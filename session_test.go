package remotedialer

import (
	"math/rand"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"
)

var dummyConnectionsNextID int64 = 1

func getDummyConnectionID() int64 {
	return atomic.AddInt64(&dummyConnectionsNextID, 1)
}

func setupDummySession(t *testing.T, nConnections int) *Session {
	t.Helper()

	s := newSession(rand.Int63(), "", nil)

	var wg sync.WaitGroup
	ready := make(chan struct{})
	for i := 0; i < nConnections; i++ {
		connID := getDummyConnectionID()
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-ready
			s.addConnection(connID, &connection{})
		}()
	}
	close(ready)
	wg.Wait()

	if got, want := len(s.conns), nConnections; got != want {
		t.Fatalf("incorrect number of connections, got: %d, want %d", got, want)
	}

	return s
}

func TestSession_connections(t *testing.T) {
	const n = 10
	s := setupDummySession(t, n)

	connID, conn := getDummyConnectionID(), &connection{}
	s.addConnection(connID, conn)
	if got, want := len(s.conns), n+1; got != want {
		t.Errorf("incorrect number of connections, got: %d, want %d", got, want)
	}
	if got, want := s.getConnection(connID), conn; got != want {
		t.Errorf("incorrect result from getConnection, got: %v, want %v", got, want)
	}
	if got, want := s.removeConnection(connID), conn; got != want {
		t.Errorf("incorrect result from removeConnection, got: %v, want %v", got, want)
	}
}

func TestSession_sessionKeys(t *testing.T) {
	s := setupDummySession(t, 0)

	clientKey, sessionKey := "testkey", rand.Int()
	s.addSessionKey(clientKey, sessionKey)
	if got, want := len(s.remoteClientKeys), 1; got != want {
		t.Errorf("incorrect number of remote client keys, got: %d, want %d", got, want)
	}

	if got, want := s.getSessionKeys(clientKey), map[int]bool{sessionKey: true}; !reflect.DeepEqual(got, want) {
		t.Errorf("incorrect result from getSessionKeys, got: %v, want %v", got, want)
	}

	s.removeSessionKey(clientKey, sessionKey)
	if got, want := len(s.remoteClientKeys), 0; got != want {
		t.Errorf("incorrect number of remote client keys after removal, got: %d, want %d", got, want)
	}
}
