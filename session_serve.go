package remotedialer

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/sirupsen/logrus"
)

// serveMessage accepts an incoming message from the underlying websocket connection and processes the request based on its messageType
func (s *Session) serveMessage(ctx context.Context, reader io.Reader) error {
	message, err := newServerMessage(reader)
	if err != nil {
		return err
	}

	if PrintTunnelData {
		logrus.Debug("REQUEST ", message)
	}

	switch message.messageType {
	case Connect:
		return s.clientConnect(ctx, message)
	case AddClient:
		return s.addRemoteClient(message.address)
	case RemoveClient:
		return s.removeRemoteClient(message.address)
	case Data:
		s.connectionData(message.connID, message.body)
	case Pause:
		s.pauseConnection(message.connID)
	case Resume:
		s.resumeConnection(message.connID)
	case Error:
		s.closeConnection(message.connID, message.Err())
	}
	return nil
}

// clientConnect accepts a new connection request, dialing back to establish the connection
func (s *Session) clientConnect(ctx context.Context, message *message) error {
	if s.auth == nil || !s.auth(message.proto, message.address) {
		return errors.New("connect not allowed")
	}

	conn := newConnection(message.connID, s, message.proto, message.address)

	s.Lock()
	s.conns[message.connID] = conn
	if PrintTunnelData {
		logrus.Debugf("CONNECTIONS %d %d", s.sessionKey, len(s.conns))
	}
	s.Unlock()

	go clientDial(ctx, s.dialer, conn, message)

	return nil
}

// / addRemoteClient registers a new remote client, making it accessible for requests
func (s *Session) addRemoteClient(address string) error {
	s.Lock()
	defer s.Unlock()

	if s.remoteClientKeys == nil {
		return nil
	}

	clientKey, sessionKey, err := parseAddress(address)
	if err != nil {
		return fmt.Errorf("invalid remote Session %s: %v", address, err)
	}

	keys := s.remoteClientKeys[clientKey]
	if keys == nil {
		keys = map[int]bool{}
		s.remoteClientKeys[clientKey] = keys
	}
	keys[sessionKey] = true

	if PrintTunnelData {
		logrus.Debugf("ADD REMOTE CLIENT %s, SESSION %d", address, s.sessionKey)
	}

	return nil
}

// / addRemoteClient removes a given client from a session
func (s *Session) removeRemoteClient(address string) error {
	s.Lock()
	defer s.Unlock()

	clientKey, sessionKey, err := parseAddress(address)
	if err != nil {
		return fmt.Errorf("invalid remote Session %s: %v", address, err)
	}

	keys := s.remoteClientKeys[clientKey]
	delete(keys, sessionKey)
	if len(keys) == 0 {
		delete(s.remoteClientKeys, clientKey)
	}

	if PrintTunnelData {
		logrus.Debugf("REMOVE REMOTE CLIENT %s, SESSION %d", address, s.sessionKey)
	}

	return nil
}

// closeConnection removes a connection for a given ID from the session, sending an error message to communicate the closing to the other end.
// If an error is not provided, io.EOF will be used instead.
func (s *Session) closeConnection(connID int64, err error) {
	s.Lock()
	conn := s.conns[connID]
	delete(s.conns, connID)
	if PrintTunnelData {
		logrus.Debugf("CONNECTIONS %d %d", s.sessionKey, len(s.conns))
	}
	s.Unlock()

	if conn != nil {
		conn.tunnelClose(err)
	}
}

// connectionData process incoming data from connection by reading the body into an internal readBuffer
func (s *Session) connectionData(connID int64, body io.Reader) {
	conn := s.getConnection(connID)
	if conn == nil {
		errMsg := newErrorMessage(connID, fmt.Errorf("connection not found %s/%d/%d", s.clientKey, s.sessionKey, connID))
		_, _ = errMsg.WriteTo(defaultDeadline(), s.conn)
		return
	}

	if err := conn.OnData(body); err != nil {
		s.closeConnection(connID, err)
	}
}

// pauseConnection activates backPressure for a given connection ID
func (s *Session) pauseConnection(connID int64) {
	if conn := s.getConnection(connID); conn != nil {
		conn.OnPause()
	}
}

// resumeConnection deactivates backPressure for a given connection ID
func (s *Session) resumeConnection(connID int64) {
	if conn := s.getConnection(connID); conn != nil {
		conn.OnResume()
	}
}
