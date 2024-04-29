package remotedialer

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/sirupsen/logrus"
)

func (s *Session) serveMessage(ctx context.Context, reader io.Reader) error {
	message, err := newServerMessage(reader)
	if err != nil {
		return err
	}

	if PrintTunnelData {
		logrus.Debug("REQUEST ", message)
	}

	if message.messageType == Connect {
		if s.auth == nil || !s.auth(message.proto, message.address) {
			return errors.New("connect not allowed")
		}
		s.clientConnect(ctx, message)
		return nil
	}

	s.Lock()
	if message.messageType == AddClient && s.remoteClientKeys != nil {
		err := s.addRemoteClient(message.address)
		s.Unlock()
		return err
	} else if message.messageType == RemoveClient {
		err := s.removeRemoteClient(message.address)
		s.Unlock()
		return err
	}
	conn := s.conns[message.connID]
	s.Unlock()

	if conn == nil {
		if message.messageType == Data {
			err := fmt.Errorf("connection not found %s/%d/%d", s.clientKey, s.sessionKey, message.connID)
			newErrorMessage(message.connID, err).WriteTo(defaultDeadline(), s.conn)
		}
		return nil
	}

	switch message.messageType {
	case Data:
		if err := conn.OnData(message); err != nil {
			s.closeConnection(message.connID, err)
		}
	case Pause:
		conn.OnPause()
	case Resume:
		conn.OnResume()
	case Error:
		s.closeConnection(message.connID, message.Err())
	}

	return nil
}

func (s *Session) clientConnect(ctx context.Context, message *message) {
	conn := newConnection(message.connID, s, message.proto, message.address)

	s.Lock()
	s.conns[message.connID] = conn
	if PrintTunnelData {
		logrus.Debugf("CONNECTIONS %d %d", s.sessionKey, len(s.conns))
	}
	s.Unlock()

	go clientDial(ctx, s.dialer, conn, message)
}

func (s *Session) addRemoteClient(address string) error {
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

func (s *Session) removeRemoteClient(address string) error {
	clientKey, sessionKey, err := parseAddress(address)
	if err != nil {
		return fmt.Errorf("invalid remote Session %s: %v", address, err)
	}

	keys := s.remoteClientKeys[clientKey]
	delete(keys, int(sessionKey))
	if len(keys) == 0 {
		delete(s.remoteClientKeys, clientKey)
	}

	if PrintTunnelData {
		logrus.Debugf("REMOVE REMOTE CLIENT %s, SESSION %d", address, s.sessionKey)
	}

	return nil
}

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
