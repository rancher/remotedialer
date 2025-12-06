package remotedialer

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/coder/websocket"
	"github.com/sirupsen/logrus"
)

// websocketConn is a minimal interface for websocket connections
type websocketConn interface {
	Read(ctx context.Context) (websocket.MessageType, []byte, error)
	Write(ctx context.Context, typ websocket.MessageType, p []byte) error
	Close(code websocket.StatusCode, reason string) error
	SetReadLimit(limit int64)
}

type Session struct {
	sync.RWMutex

	nextConnID       int64
	clientKey        string
	sessionKey       int64
	conn             websocketConn
	conns            map[int64]*connection
	remoteClientKeys map[string]map[int]bool
	auth             ConnectAuthorizer
	syncCancel       context.CancelFunc
	syncWait         sync.WaitGroup
	dialer           Dialer
	client           bool
}

// Use this defined type so we can share context between remotedialer and its clients
type ContextKey struct{}

var ContextKeyCaller = ContextKey{}

func ValueFromContext(ctx context.Context) string {
	v := ctx.Value(ContextKeyCaller)
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

// PrintTunnelData No tunnel logging by default
var PrintTunnelData bool

func init() {
	if os.Getenv("CATTLE_TUNNEL_DATA_DEBUG") == "true" {
		PrintTunnelData = true
	}
}

func NewClientSession(auth ConnectAuthorizer, conn *websocket.Conn) *Session {
	return NewClientSessionWithDialer(auth, conn, nil)
}

func NewClientSessionWithDialer(auth ConnectAuthorizer, conn *websocket.Conn, dialer Dialer) *Session {
	// Set unlimited read limit to match gorilla/websocket behavior
	conn.SetReadLimit(-1)
	return &Session{
		clientKey: "client",
		conn:      conn,
		conns:     map[int64]*connection{},
		auth:      auth,
		client:    true,
		dialer:    dialer,
	}
}

func newSession(sessionKey int64, clientKey string, conn *websocket.Conn) *Session {
	// Set unlimited read limit to match gorilla/websocket behavior
	// (skip if conn is nil, which only happens in tests)
	if conn != nil {
		conn.SetReadLimit(-1)
	}
	return &Session{
		nextConnID:       1,
		clientKey:        clientKey,
		sessionKey:       sessionKey,
		conn:             conn,
		conns:            map[int64]*connection{},
		remoteClientKeys: map[string]map[int]bool{},
	}
}

// addConnection safely registers a new connection in the connections map
func (s *Session) addConnection(connID int64, conn *connection) {
	s.Lock()
	defer s.Unlock()

	s.conns[connID] = conn
	if PrintTunnelData {
		logrus.Debugf("CONNECTIONS %d %d", s.sessionKey, len(s.conns))
	}
}

// removeConnection safely removes a connection by ID, returning the connection object
func (s *Session) removeConnection(connID int64) *connection {
	s.Lock()
	defer s.Unlock()

	conn := s.removeConnectionLocked(connID)
	if PrintTunnelData {
		defer logrus.Debugf("CONNECTIONS %d %d", s.sessionKey, len(s.conns))
	}
	return conn
}

// removeConnectionLocked removes a given connection from the session.
// The session lock must be held by the caller when calling this method
func (s *Session) removeConnectionLocked(connID int64) *connection {
	conn := s.conns[connID]
	delete(s.conns, connID)
	return conn
}

// getConnection retrieves a connection by ID
func (s *Session) getConnection(connID int64) *connection {
	s.RLock()
	defer s.RUnlock()

	return s.conns[connID]
}

// activeConnectionIDs returns an ordered list of IDs for the currently active connections
func (s *Session) activeConnectionIDs() []int64 {
	s.RLock()
	defer s.RUnlock()

	res := make([]int64, 0, len(s.conns))
	for id := range s.conns {
		res = append(res, id)
	}
	sort.Slice(res, func(i, j int) bool { return res[i] < res[j] })
	return res
}

// addSessionKey registers a new session key for a given client key
func (s *Session) addSessionKey(clientKey string, sessionKey int) {
	s.Lock()
	defer s.Unlock()

	keys := s.remoteClientKeys[clientKey]
	if keys == nil {
		keys = map[int]bool{}
		s.remoteClientKeys[clientKey] = keys
	}
	keys[sessionKey] = true
}

// removeSessionKey removes a specific session key for a client key
func (s *Session) removeSessionKey(clientKey string, sessionKey int) {
	s.Lock()
	defer s.Unlock()

	keys := s.remoteClientKeys[clientKey]
	delete(keys, sessionKey)
	if len(keys) == 0 {
		delete(s.remoteClientKeys, clientKey)
	}
}

// getSessionKeys retrieves all session keys for a given client key
func (s *Session) getSessionKeys(clientKey string) map[int]bool {
	s.RLock()
	defer s.RUnlock()
	return s.remoteClientKeys[clientKey]
}

func (s *Session) startPeriodicSync(rootCtx context.Context) {
	ctx, cancel := context.WithCancel(rootCtx)
	s.syncCancel = cancel
	s.syncWait.Add(1)

	go func() {
		defer s.syncWait.Done()

		ticker := time.NewTicker(SyncConnectionsInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := s.sendSyncConnections(); err != nil {
					logrus.WithError(err).Error("Error syncing connections")
				}
			}
		}
	}()
}

func (s *Session) stopPeriodicSync() {
	if s.syncCancel == nil {
		return
	}

	s.syncCancel()
	s.syncWait.Wait()
}

func (s *Session) Serve(ctx context.Context) (int, error) {
	if s.client {
		s.startPeriodicSync(ctx)
	}

	for {
		msgType, data, err := s.conn.Read(ctx)
		if err != nil {
			return 400, err
		}

		if msgType != websocket.MessageBinary {
			return 400, errWrongMessageType
		}

		if err := s.serveMessage(ctx, bytes.NewReader(data)); err != nil {
			return 500, err
		}
	}
}

func defaultDeadline() time.Time {
	return time.Now().Add(time.Minute)
}

func parseAddress(address string) (string, int, error) {
	parts := strings.SplitN(address, "/", 2)
	if len(parts) != 2 {
		return "", 0, errors.New("not / separated")
	}
	v, err := strconv.Atoi(parts[1])
	return parts[0], v, err
}

type connResult struct {
	conn net.Conn
	err  error
}

func (s *Session) Dial(ctx context.Context, proto, address string) (net.Conn, error) {
	return s.serverConnectContext(ctx, proto, address)
}

func (s *Session) serverConnectContext(ctx context.Context, proto, address string) (net.Conn, error) {
	deadline, ok := ctx.Deadline()
	if ok {
		return s.serverConnect(ctx, deadline, proto, address)
	}

	result := make(chan connResult, 1)
	go func() {
		c, err := s.serverConnect(ctx, defaultDeadline(), proto, address)
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

func (s *Session) serverConnect(ctx context.Context, deadline time.Time, proto, address string) (net.Conn, error) {
	connID := atomic.AddInt64(&s.nextConnID, 1)
	conn := newConnection(ctx, connID, s, proto, address)

	s.addConnection(connID, conn)

	writeCtx := ctx
	if writeCtx == nil {
		writeCtx = context.Background()
	}
	if !deadline.IsZero() {
		var cancel context.CancelFunc
		writeCtx, cancel = context.WithDeadline(writeCtx, deadline)
		defer cancel()
	}
	_, err := s.writeMessage(writeCtx, newConnect(connID, proto, address))
	if err != nil {
		s.closeConnection(connID, err)
		return nil, err
	}

	return conn, err
}

func (s *Session) writeMessage(ctx context.Context, message *message) (int, error) {
	if PrintTunnelData {
		logrus.Debug("WRITE ", message)
	}
	return message.WriteTo(ctx, s.conn)
}

func (s *Session) Close() {
	s.stopPeriodicSync()

	s.Lock()
	defer s.Unlock()
	for _, connection := range s.conns {
		connection.tunnelClose(errors.New("tunnel disconnect"))
	}

	s.conns = map[int64]*connection{}
}

func (s *Session) sessionAdded(ctx context.Context, clientKey string, sessionKey int64) {
	if ctx == nil {
		ctx = context.Background()
	}
	client := fmt.Sprintf("%s/%d", clientKey, sessionKey)
	_, err := s.writeMessage(ctx, newAddClient(client))
	if err != nil {
		s.conn.Close(websocket.StatusInternalError, "failed to add client")
	}
}

func (s *Session) sessionRemoved(ctx context.Context, clientKey string, sessionKey int64) {
	if ctx == nil {
		ctx = context.Background()
	}
	client := fmt.Sprintf("%s/%d", clientKey, sessionKey)
	_, err := s.writeMessage(ctx, newRemoveClient(client))
	if err != nil {
		s.conn.Close(websocket.StatusInternalError, "failed to remove client")
	}
}
