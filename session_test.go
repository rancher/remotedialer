package remotedialer

import (
	"context"
	"errors"
	"math/rand"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/coder/websocket"
)

var dummyConnectionsNextID int64 = 1

func getDummyConnectionID() int64 {
	return atomic.AddInt64(&dummyConnectionsNextID, 1)
}

func setupDummySession(t *testing.T, nConnections int) *Session {
	t.Helper()

	s := newSession(rand.Int63(), "", nil)
	s.conn = &fakeWSConn{
		writeCallback: func(ctx context.Context, typ websocket.MessageType, data []byte) (err error) {
			if ctx.Err() != nil {
				return errors.New("context canceled")
			}
			return nil
		},
	}

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
	t.Parallel()

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
	t.Parallel()

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

func TestSession_activeConnectionIDs(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		conns    map[int64]*connection
		expected []int64
	}{
		{
			name:     "no connections",
			conns:    map[int64]*connection{},
			expected: []int64{},
		},
		{
			name: "single",
			conns: map[int64]*connection{
				1234: nil,
			},
			expected: []int64{1234},
		},
		{
			name: "multiple connections",
			conns: map[int64]*connection{
				5:  nil,
				20: nil,
				3:  nil,
			},
			expected: []int64{3, 5, 20},
		},
	}
	for x := range tests {
		tt := tests[x]
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			session := Session{conns: tt.conns}
			if got, want := session.activeConnectionIDs(), tt.expected; !reflect.DeepEqual(got, want) {
				t.Errorf("incorrect result, got: %v, want: %v", got, want)
			}
		})
	}
}

// This test is to ensure that there is no deadlock if Close()
// is called while startPeriodicSync goroutine is running. We are not
// calling startPeriodicSync directly, but simulating its state where the
// deadlock could occur.
func TestSession_CloseDeadlock(t *testing.T) {
	t.Parallel()

	s := setupDummySession(t, 0)

	_, s.syncCancel = context.WithCancel(context.Background())
	s.syncWait.Add(1)

	go func() {
		s.Close()
	}()

	time.Sleep(1 * time.Second)
	done := make(chan struct{})
	go func() {
		s.Lock()
		s.syncWait.Done()
		s.Unlock()
		close(done)
	}()

	select {
	case <-done:
		// Close returned. Test passed.
	case <-time.After(2 * time.Second):
		t.Fatal("Close() did not return within 2s, possible deadlock")
	}
}
