package remotedialer

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"sync"

	"github.com/gorilla/websocket"
	"github.com/hashicorp/yamux"
	"github.com/loft-sh/remotedialer/log"
	"k8s.io/klog/v2"
)

type Session struct {
	sync.Mutex

	clientKey  string
	sessionKey int64

	wsConn           *websocket.Conn
	session          *yamux.Session
	remoteClientKeys map[string]map[int]bool
	auth             ConnectAuthorizer
	dialer           Dialer
	client           bool
}

// PrintTunnelData No tunnel logging by default
var PrintTunnelData bool

func init() {
	if os.Getenv("CATTLE_TUNNEL_DATA_DEBUG") == "true" {
		PrintTunnelData = true
	}
}

func NewClientSession(ctx context.Context, auth ConnectAuthorizer, conn *websocket.Conn) (*Session, error) {
	return NewClientSessionWithDialer(ctx, auth, conn, nil)
}

func NewClientSessionWithDialer(ctx context.Context, auth ConnectAuthorizer, conn *websocket.Conn, dialer Dialer) (*Session, error) {
	config := yamux.DefaultConfig()

	// Override the default logger with our own
	config.LogOutput = nil
	config.Logger = &log.YamuxLogr{Logger: klog.FromContext(ctx)}

	session, err := yamux.Client(newConn(conn), config)
	if err != nil {
		return nil, err
	}

	return &Session{
		clientKey: "client",
		session:   session,
		wsConn:    conn,
		auth:      auth,
		client:    true,
		dialer:    dialer,
	}, nil
}

func newSession(ctx context.Context, sessionKey int64, clientKey string, conn *websocket.Conn) (*Session, error) {
	config := yamux.DefaultConfig()

	// Override the default logger with our own
	config.LogOutput = nil
	config.Logger = &log.YamuxLogr{Logger: klog.FromContext(ctx)}

	session, err := yamux.Server(newConn(conn), config)
	if err != nil {
		return nil, err
	}

	return &Session{
		clientKey:        clientKey,
		sessionKey:       sessionKey,
		session:          session,
		wsConn:           conn,
		remoteClientKeys: map[string]map[int]bool{},
	}, nil
}

func (s *Session) Serve(ctx context.Context) (int, error) {
	for {
		stream, err := s.session.Accept()
		if err != nil {
			return 400, err
		}

		if err := s.serveMessage(ctx, stream); err != nil {
			return 500, err
		}
	}
}

func (s *Session) serveMessage(ctx context.Context, conn net.Conn) error {
	message, err := newServerMessage(conn)
	if err != nil {
		return err
	}

	if PrintTunnelData {
		klog.FromContext(ctx).V(1).Info("REQUEST", "message", message)
	}

	if message.messageType == Connect {
		if s.auth == nil || !s.auth(message.proto, message.address) {
			return errors.New("connect not allowed")
		}

		go clientDial(ctx, s.dialer, message)
		return nil
	}

	s.Lock()
	defer s.Unlock()

	if message.messageType == AddClient && s.remoteClientKeys != nil {
		err := s.addRemoteClient(ctx, message.address)
		return err
	} else if message.messageType == RemoveClient {
		err := s.removeRemoteClient(ctx, message.address)
		return err
	}

	return nil
}

func parseAddress(address string) (string, int, error) {
	parts := strings.SplitN(address, "/", 2)
	if len(parts) != 2 {
		return "", 0, errors.New("not / separated")
	}
	v, err := strconv.Atoi(parts[1])
	return parts[0], v, err
}

func (s *Session) addRemoteClient(ctx context.Context, address string) error {
	clientKey, sessionKey, err := parseAddress(address)
	if err != nil {
		return fmt.Errorf("invalid remote Session %s: %w", address, err)
	}

	keys := s.remoteClientKeys[clientKey]
	if keys == nil {
		keys = map[int]bool{}
		s.remoteClientKeys[clientKey] = keys
	}
	keys[sessionKey] = true

	if PrintTunnelData {
		klog.FromContext(ctx).V(1).Info("ADD REMOTE CLIENT", "address", address, "session", s.sessionKey)
	}

	return nil
}

func (s *Session) removeRemoteClient(ctx context.Context, address string) error {
	clientKey, sessionKey, err := parseAddress(address)
	if err != nil {
		return fmt.Errorf("invalid remote Session %s: %w", address, err)
	}

	keys := s.remoteClientKeys[clientKey]
	delete(keys, sessionKey)
	if len(keys) == 0 {
		delete(s.remoteClientKeys, clientKey)
	}

	if PrintTunnelData {
		klog.FromContext(ctx).V(1).Info("REMOVE REMOTE CLIENT", "address", address, "session", s.sessionKey)
	}

	return nil
}

type connResult struct {
	conn net.Conn
	err  error
}

func (s *Session) Dial(ctx context.Context, proto, address string) (net.Conn, error) {
	return s.serverConnectContext(ctx, proto, address)
}

func (s *Session) serverConnectContext(ctx context.Context, proto, address string) (net.Conn, error) {
	result := make(chan connResult, 1)
	go func() {
		c, err := s.serverConnect(proto, address)
		result <- connResult{conn: c, err: err}
	}()

	select {
	case <-ctx.Done():
		// We don't want to orphan an open connection so we wait for the result and immediately close it
		go func() {
			r := <-result
			if r.err == nil {
				r.conn.Close()
			}
		}()
		return nil, ctx.Err()
	case r := <-result:
		return r.conn, r.err
	}
}

func (s *Session) serverConnect(proto, address string) (net.Conn, error) {
	conn, err := s.session.Open()
	if err != nil {
		return nil, err
	}

	connectMessage := newConnect(proto, address).Bytes()
	n, err := conn.Write(connectMessage)
	if err != nil {
		_ = conn.Close()
		return nil, err
	} else if n != len(connectMessage) {
		_ = conn.Close()
		return nil, fmt.Errorf("short write, expected %d bytes to be written, got %d", len(connectMessage), n)
	}

	return conn, err
}

func (s *Session) Close() {
	s.Lock()
	defer s.Unlock()

	_ = s.session.Close()
}

func (s *Session) sessionAdded(clientKey string, sessionKey int64) error {
	client := fmt.Sprintf("%s/%d", clientKey, sessionKey)
	conn, err := s.session.Open()
	if err != nil {
		return err
	}
	defer conn.Close()

	addClientBytes := newAddClient(client).Bytes()
	n, err := conn.Write(addClientBytes)
	if err != nil {
		return err
	} else if n != len(addClientBytes) {
		return fmt.Errorf("short write, expected %d bytes to be written, got %d", len(addClientBytes), n)
	}

	return nil
}

func (s *Session) sessionRemoved(clientKey string, sessionKey int64) error {
	client := fmt.Sprintf("%s/%d", clientKey, sessionKey)
	conn, err := s.session.Open()
	if err != nil {
		return err
	}
	defer conn.Close()

	removeClientBytes := newRemoveClient(client).Bytes()
	n, err := conn.Write(removeClientBytes)
	if err != nil {
		return err
	} else if n != len(removeClientBytes) {
		return fmt.Errorf("short write, expected %d bytes to be written, got %d", len(removeClientBytes), n)
	}

	return nil
}
