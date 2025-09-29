package remotedialer

import (
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

	"github.com/gorilla/websocket"
	"github.com/sirupsen/logrus"
)

type Session struct {
	sync.RWMutex

	nextConnID       int64
	clientKey        string
	sessionKey       int64
	conn             wsConn
	conns            map[int64]*connection
	remoteClientKeys map[string]map[int]bool
	auth             ConnectAuthorizer
	pingCancel       context.CancelFunc
	pingWait         sync.WaitGroup
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

func NewClientSession(auth ConnectAuthorizer, conn *websocket.Conn) *Session {
	return NewClientSessionWithDialer(auth, conn, nil)
}

func NewClientSessionWithDialer(auth ConnectAuthorizer, conn *websocket.Conn, dialer Dialer) *Session {
	logrus.Debugf("Creating new client session: clientKey=client, dialer=%v", dialer)
	return &Session{
		clientKey: "client",
		conn:      newWSConn(conn),
		conns:     map[int64]*connection{},
		auth:      auth,
		client:    true,
		dialer:    dialer,
	}
}

func newSession(sessionKey int64, clientKey string, conn wsConn) *Session {
	logrus.Debugf("Creating new session: sessionKey=%d, clientKey=%s", sessionKey, clientKey)
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
	logrus.Debugf("Session %d: Added connection connID=%d, total connections=%d", s.sessionKey, connID, len(s.conns))
	if PrintTunnelData {
		logrus.Debugf("CONNECTIONS %d %d", s.sessionKey, len(s.conns))
	}
}

// removeConnection safely removes a connection by ID, returning the connection object
func (s *Session) removeConnection(connID int64) *connection {
	s.Lock()
	defer s.Unlock()

	conn := s.removeConnectionLocked(connID)
	logrus.Debugf("Session %d: Removed connection connID=%d, remaining connections=%d", s.sessionKey, connID, len(s.conns))
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

	logrus.Debugf("Session %d: Get connection connID=%d", s.sessionKey, connID)
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
	logrus.Debugf("Session %d: Active connection IDs: %v", s.sessionKey, res)
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
	logrus.Debugf("Session %d: Added sessionKey=%d for clientKey=%s, remoteClientKeys=%v", s.sessionKey, sessionKey, clientKey, s.remoteClientKeys)
}

// removeSessionKey removes a specific session key for a client key
func (s *Session) removeSessionKey(clientKey string, sessionKey int) {
	s.Lock()
	defer s.Unlock()

	keys := s.remoteClientKeys[clientKey]
	delete(keys, sessionKey)
	logrus.Debugf("Session %d: Removed sessionKey=%d for clientKey=%s, remaining keys=%v", s.sessionKey, sessionKey, clientKey, keys)
	if len(keys) == 0 {
		delete(s.remoteClientKeys, clientKey)
		logrus.Debugf("Session %d: Removed clientKey=%s from remoteClientKeys", s.sessionKey, clientKey)
	}
}

// getSessionKeys retrieves all session keys for a given client key
func (s *Session) getSessionKeys(clientKey string) map[int]bool {
	s.RLock()
	defer s.RUnlock()
	logrus.Debugf("Session %d: Get session keys for clientKey=%s, keys=%v", s.sessionKey, clientKey, s.remoteClientKeys[clientKey])
	return s.remoteClientKeys[clientKey]
}

func (s *Session) startPings(rootCtx context.Context) {
	logrus.Debugf("Session %d: Initializing startPings for clientKey=%s", s.sessionKey, s.clientKey)
	ctx, cancel := context.WithCancel(rootCtx)
	s.pingCancel = cancel
	s.pingWait.Add(1)

	logrus.Debugf("Session %d: Starting pings goroutine for clientKey=%s", s.sessionKey, s.clientKey)
	go func() {
		defer func() {
			logrus.Debugf("Session %d: Exiting pings goroutine for clientKey=%s", s.sessionKey, s.clientKey)
			s.pingWait.Done()
		}()

		t := time.NewTicker(PingWriteInterval)
		defer func() {
			logrus.Debugf("Session %d: Stopping ping ticker for clientKey=%s", s.sessionKey, s.clientKey)
			t.Stop()
		}()

		syncConnections := time.NewTicker(SyncConnectionsInterval)
		defer func() {
			logrus.Debugf("Session %d: Stopping syncConnections ticker for clientKey=%s", s.sessionKey, s.clientKey)
			syncConnections.Stop()
		}()

		for {
			select {
			case <-ctx.Done():
				logrus.Debugf("Session %d: Received context done, stopping pings for clientKey=%s", s.sessionKey, s.clientKey)
				return
			case <-syncConnections.C:
				logrus.Debugf("Session %d: Syncing connections for clientKey=%s", s.sessionKey, s.clientKey)
				if err := s.sendSyncConnections(); err != nil {
					logrus.WithError(err).Error("Error syncing connections")
				} else {
					logrus.Debugf("Session %d: Successfully synced connections for clientKey=%s", s.sessionKey, s.clientKey)
				}
			case <-t.C:
				logrus.Debugf("Session %d: Sending ping for clientKey=%s", s.sessionKey, s.clientKey)
				if err := s.sendPing(); err != nil {
					logrus.WithError(err).Error("Error writing ping")
				} else {
					logrus.Debugf("Session %d: Successfully wrote ping for clientKey=%s", s.sessionKey, s.clientKey)
				}
				logrus.Debug("Wrote ping")
			}
		}
	}()
}

// sendPing sends a Ping control message to the peer
func (s *Session) sendPing() error {
	logrus.Debugf("Session %d: Sending ping", s.sessionKey)
	return s.conn.WriteControl(websocket.PingMessage, time.Now().Add(PingWaitDuration), []byte(""))
}

func (s *Session) stopPings() {
	if s.pingCancel == nil {
		return
	}

	logrus.Debugf("Session %d: Stopping pings", s.sessionKey)
	s.pingCancel()
	s.pingWait.Wait()
}

func (s *Session) Serve(ctx context.Context) (int, error) {
	if s.client {
		s.startPings(ctx)
	}

	logrus.Debugf("Session %d: Serving for clientKey=%s", s.sessionKey, s.clientKey)
	for {
		msType, reader, err := s.conn.NextReader()
		if err != nil {
			logrus.WithError(err).Errorf("Session %d: Error getting next reader", s.sessionKey)
			return 400, err
		}

		if msType != websocket.BinaryMessage {
			logrus.Errorf("Session %d: Wrong message type: %d", s.sessionKey, msType)
			return 400, errWrongMessageType
		}

		if err := s.serveMessage(ctx, reader); err != nil {
			logrus.WithError(err).Errorf("Session %d: Error serving message", s.sessionKey)
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
	logrus.Debugf("Session %d: Dialing proto=%s, address=%s", s.sessionKey, proto, address)
	return s.serverConnectContext(ctx, proto, address)
}

func (s *Session) serverConnectContext(ctx context.Context, proto, address string) (net.Conn, error) {
	deadline, ok := ctx.Deadline()
	if ok {
		logrus.Debugf("Session %d: serverConnect with deadline=%v, proto=%s, address=%s", s.sessionKey, deadline, proto, address)
		return s.serverConnect(deadline, proto, address)
	}

	result := make(chan connResult, 1)
	go func() {
		c, err := s.serverConnect(defaultDeadline(), proto, address)
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
		logrus.Debugf("Session %d: Context done while connecting proto=%s, address=%s", s.sessionKey, proto, address)
		return nil, ctx.Err()
	case r := <-result:
		logrus.Debugf("Session %d: serverConnect result for proto=%s, address=%s, err=%v", s.sessionKey, proto, address, r.err)
		return r.conn, r.err
	}
}

func (s *Session) serverConnect(deadline time.Time, proto, address string) (net.Conn, error) {
	connID := atomic.AddInt64(&s.nextConnID, 1)
	logrus.Debugf("Session %d: serverConnect connID=%d, proto=%s, address=%s, deadline=%v", s.sessionKey, connID, proto, address, deadline)
	conn := newConnection(connID, s, proto, address)

	s.addConnection(connID, conn)

	_, err := s.writeMessage(deadline, newConnect(connID, proto, address))
	if err != nil {
		logrus.WithError(err).Errorf("Session %d: Error writing connect message for connID=%d", s.sessionKey, connID)
		s.closeConnection(connID, err)
		return nil, err
	}

	return conn, err
}

func (s *Session) writeMessage(deadline time.Time, message *message) (int, error) {
	if PrintTunnelData {
		logrus.Debug("WRITE ", message)
	}
	logrus.Debugf("Session %d: Writing message: %v, deadline=%v", s.sessionKey, message, deadline)
	return message.WriteTo(deadline, s.conn)
}

func (s *Session) Close() {
	s.stopPings()

	s.Lock()
	defer s.Unlock()
	logrus.Debugf("Session %d: Closing session, closing %d connections", s.sessionKey, len(s.conns))
	for _, connection := range s.conns {
		connection.tunnelClose(errors.New("tunnel disconnect"))
	}

	s.conns = map[int64]*connection{}
}

func (s *Session) sessionAdded(clientKey string, sessionKey int64) {
	client := fmt.Sprintf("%s/%d", clientKey, sessionKey)
	logrus.Debugf("Session %d: sessionAdded client=%s", s.sessionKey, client)
	_, err := s.writeMessage(time.Time{}, newAddClient(client))
	if err != nil {
		logrus.WithError(err).Errorf("Session %d: Error adding client %s", s.sessionKey, client)
		s.conn.Close()
	}
}

func (s *Session) sessionRemoved(clientKey string, sessionKey int64) {
	client := fmt.Sprintf("%s/%d", clientKey, sessionKey)
	logrus.Debugf("Session %d: sessionRemoved client=%s", s.sessionKey, client)
	_, err := s.writeMessage(time.Time{}, newRemoveClient(client))
	if err != nil {
		logrus.WithError(err).Errorf("Session %d: Error removing client %s", s.sessionKey, client)
		s.conn.Close()
	}
}
