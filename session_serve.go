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
		logrus.Debugf("Failed to create server message: %v", err)
		return err
	}

	if PrintTunnelData {
		logrus.Debug("REQUEST ", message)
	}

	logrus.Debugf("Processing message type: %s", message.String())

	switch message.messageType {
	case Connect:
		logrus.Debugf("Handling Connect for connID: %d, proto: %s, address: %s", message.connID, message.proto, message.address)
		return s.clientConnect(ctx, message)
	case AddClient:
		logrus.Debugf("Handling AddClient for address: %s", message.address)
		return s.addRemoteClient(message.address)
	case RemoveClient:
		logrus.Debugf("Handling RemoveClient for address: %s", message.address)
		return s.removeRemoteClient(message.address)
	case SyncConnections:
		logrus.Debug("Handling SyncConnections")
		return s.syncConnections(message.body)
	case Data:
		logrus.Debugf("Handling Data for connID: %d", message.connID)
		s.connectionData(message.connID, message.body)
	case Pause:
		logrus.Debugf("Handling Pause for connID: %d", message.connID)
		s.pauseConnection(message.connID)
	case Resume:
		logrus.Debugf("Handling Resume for connID: %d", message.connID)
		s.resumeConnection(message.connID)
	case Error:
		logrus.Debugf("Handling Error for connID: %d, error: %v", message.connID, message.Err())
		s.closeConnection(message.connID, message.Err())
	}
	return nil
}

// clientConnect accepts a new connection request, dialing back to establish the connection
func (s *Session) clientConnect(ctx context.Context, message *message) error {
	if s.auth == nil || !s.auth(message.proto, message.address) {
		logrus.Debugf("Connection not allowed for proto: %s, address: %s", message.proto, message.address)
		return errors.New("connect not allowed")
	}

	logrus.Debugf("Establishing new connection for connID: %d, proto: %s, address: %s", message.connID, message.proto, message.address)
	conn := newConnection(message.connID, s, message.proto, message.address)
	s.addConnection(message.connID, conn)

	go clientDial(ctx, s.dialer, conn, message)

	return nil
}

// addRemoteClient registers a new remote client, making it accessible for requests
func (s *Session) addRemoteClient(address string) error {
	if s.remoteClientKeys == nil {
		logrus.Debug("remoteClientKeys is nil, skipping addRemoteClient")
		return nil
	}

	clientKey, sessionKey, err := parseAddress(address)
	if err != nil {
		logrus.Debugf("Invalid remote Session %s: %v", address, err)
		return fmt.Errorf("invalid remote Session %s: %v", address, err)
	}
	s.addSessionKey(clientKey, sessionKey)

	if PrintTunnelData {
		logrus.Debugf("ADD REMOTE CLIENT %s, SESSION %d", address, s.sessionKey)
	}
	logrus.Debugf("Added remote client: %s (clientKey: %s, sessionKey: %d)", address, clientKey, sessionKey)

	return nil
}

// removeRemoteClient removes a given client from a session
func (s *Session) removeRemoteClient(address string) error {
	clientKey, sessionKey, err := parseAddress(address)
	if err != nil {
		logrus.Debugf("Invalid remote Session %s: %v", address, err)
		return fmt.Errorf("invalid remote Session %s: %v", address, err)
	}
	s.removeSessionKey(clientKey, sessionKey)

	if PrintTunnelData {
		logrus.Debugf("REMOVE REMOTE CLIENT %s, SESSION %d", address, s.sessionKey)
	}
	logrus.Debugf("Removed remote client: %s (clientKey: %s, sessionKey: %d)", address, clientKey, sessionKey)

	return nil
}

// syncConnections closes any session connection that is not present in the IDs received from the client
func (s *Session) syncConnections(r io.Reader) error {
	payload, err := io.ReadAll(r)
	if err != nil {
		logrus.Debugf("Error reading syncConnections message body: %v", err)
		return fmt.Errorf("reading message body: %w", err)
	}
	clientActiveConnections, err := decodeConnectionIDs(payload)
	if err != nil {
		logrus.Debugf("Error decoding syncConnections payload: %v", err)
		return fmt.Errorf("decoding sync connections payload: %w", err)
	}

	logrus.Debugf("Syncing connections, active client connections: %v", clientActiveConnections)
	s.compareAndCloseStaleConnections(clientActiveConnections)
	return nil
}

// closeConnection removes a connection for a given ID from the session, sending an error message to communicate the closing to the other end.
// If an error is not provided, io.EOF will be used instead.
func (s *Session) closeConnection(connID int64, err error) {
	logrus.Debugf("Closing connection for connID: %d, error: %v", connID, err)
	if conn := s.removeConnection(connID); conn != nil {
		conn.tunnelClose(err)
	}
}

// connectionData process incoming data from connection by reading the body into an internal readBuffer
func (s *Session) connectionData(connID int64, body io.Reader) {
	logrus.Debugf("Processing connection data for connID: %d", connID)
	conn := s.getConnection(connID)
	if conn == nil {
		logrus.Debugf("Connection not found for connID: %d", connID)
		errMsg := newErrorMessage(connID, fmt.Errorf("connection not found %s/%d/%d", s.clientKey, s.sessionKey, connID))
		_, _ = errMsg.WriteTo(defaultDeadline(), s.conn)
		return
	}

	if err := conn.OnData(body); err != nil {
		logrus.Debugf("Error processing data for connID: %d: %v", connID, err)
		s.closeConnection(connID, err)
	}
}

// pauseConnection activates backPressure for a given connection ID
func (s *Session) pauseConnection(connID int64) {
	logrus.Debugf("Pausing connection for connID: %d", connID)
	if conn := s.getConnection(connID); conn != nil {
		conn.OnPause()
	}
}

// resumeConnection deactivates backPressure for a given connection ID
func (s *Session) resumeConnection(connID int64) {
	logrus.Debugf("Resuming connection for connID: %d", connID)
	if conn := s.getConnection(connID); conn != nil {
		conn.OnResume()
	}
}
