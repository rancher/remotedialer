package remotedialer

import (
	"context"
	"fmt"
	"math/rand"
	"net"
	"sync"
	"time"

	"github.com/gorilla/websocket"

	"github.com/rancher/remotedialer/metrics"
)

type sessionListener interface {
	sessionAdded(clientKey string, sessionKey int64)
	sessionRemoved(clientKey string, sessionKey int64)
}

type sessionManager struct {
	sync.Mutex
	clients   map[string][]*Session
	peers     map[string][]*Session
	listeners map[sessionListener]bool
}

func newSessionManager() *sessionManager {
	return &sessionManager{
		clients:   map[string][]*Session{},
		peers:     map[string][]*Session{},
		listeners: map[sessionListener]bool{},
	}
}

func toDialer(s *Session, prefix string) Dialer {
	return func(ctx context.Context, proto, address string) (net.Conn, error) {
		if prefix == "" {
			return s.serverConnectContext(ctx, proto, address)
		}
		return s.serverConnectContext(ctx, prefix+"::"+proto, address)
	}
}

func (sm *sessionManager) removeListener(listener sessionListener) {
	sm.Lock()
	defer sm.Unlock()

	delete(sm.listeners, listener)
}

func (sm *sessionManager) addListener(listener sessionListener) {
	sm.Lock()
	defer sm.Unlock()

	sm.listeners[listener] = true

	for k, sessions := range sm.clients {
		for _, session := range sessions {
			listener.sessionAdded(k, session.sessionKey)
		}
	}

	for k, sessions := range sm.peers {
		for _, session := range sessions {
			listener.sessionAdded(k, session.sessionKey)
		}
	}
}

func (sm *sessionManager) listClients() []string {
	sm.Lock()
	defer sm.Unlock()
	clients := make([]string, 0, len(sm.clients))
	for c := range sm.clients {
		clients = append(clients, c)
	}
	return clients
}

func (sm *sessionManager) getDialer(clientKey string) (Dialer, error) {
	sm.Lock()
	defer sm.Unlock()

	sessions := sm.clients[clientKey]
	if len(sessions) > 0 {
		return toDialer(preferResponsiveSession(sessions), ""), nil
	}

	for _, sessions := range sm.peers {
		for _, session := range sessions {
			keys := session.getSessionKeys(clientKey)
			if len(keys) > 0 {
				return toDialer(session, clientKey), nil
			}
		}
	}

	return nil, fmt.Errorf("failed to find Session for client %s", clientKey)
}

// Keep the usual oldest-session preference while it is receiving heartbeats.
// If it misses two heartbeat intervals, a responsive replacement can accept
// new dials while the old WebSocket waits for its read deadline.
func preferResponsiveSession(sessions []*Session) *Session {
	oldest := sessions[0]
	if len(sessions) == 1 {
		return oldest
	}

	conn, ok := oldest.conn.(*wsWrapper)
	if !ok {
		return oldest
	}
	lastSeen := conn.connectedAt
	if heartbeat := conn.lastHeartbeat.Load(); heartbeat != nil {
		lastSeen = *heartbeat
	}

	cutoff := time.Now().Add(-2 * PingWriteInterval)
	if !lastSeen.Before(cutoff) {
		return oldest
	}

	for _, session := range sessions[1:] {
		if alternative, ok := session.conn.(*wsWrapper); ok {
			if heartbeat := alternative.lastHeartbeat.Load(); heartbeat != nil && !heartbeat.Before(cutoff) {
				return session
			}
		}
	}
	return oldest
}

func (sm *sessionManager) add(clientKey string, conn *websocket.Conn, peer bool) *Session {
	sessionKey := rand.Int63()
	session := newSession(sessionKey, clientKey, newWSConn(conn))

	sm.Lock()
	defer sm.Unlock()

	if peer {
		sm.peers[clientKey] = append(sm.peers[clientKey], session)
	} else {
		sm.clients[clientKey] = append(sm.clients[clientKey], session)
	}
	metrics.IncSMTotalAddWS(clientKey, peer)

	for l := range sm.listeners {
		l.sessionAdded(clientKey, session.sessionKey)
	}

	return session
}

func (sm *sessionManager) remove(s *Session) {
	var isPeer bool
	sm.Lock()
	defer sm.Unlock()

	for i, store := range []map[string][]*Session{sm.clients, sm.peers} {
		var newSessions []*Session

		for _, v := range store[s.clientKey] {
			if v.sessionKey == s.sessionKey {
				if i == 0 {
					isPeer = false
				} else {
					isPeer = true
				}
				metrics.IncSMTotalRemoveWS(s.clientKey, isPeer)
				continue
			}
			newSessions = append(newSessions, v)
		}

		if len(newSessions) == 0 {
			delete(store, s.clientKey)
		} else {
			store[s.clientKey] = newSessions
		}
	}

	for l := range sm.listeners {
		l.sessionRemoved(s.clientKey, s.sessionKey)
	}

	s.Close()
}
