package remotedialer

import (
	"context"
	"io"
	"sync"
	"testing"
	"time"
)

// blockingWSConn is the least conn that lets Serve run: NextReader blocks
// until the conn is closed, so the read loop parks exactly the way it does on
// a healthy websocket with no traffic.
type blockingWSConn struct {
	closed chan struct{}
	once   sync.Once
}

func (c *blockingWSConn) Close() error {
	c.once.Do(func() { close(c.closed) })
	return nil
}

func (c *blockingWSConn) NextReader() (int, io.Reader, error) {
	<-c.closed
	return 0, nil, io.EOF
}

func (c *blockingWSConn) WriteControl(int, time.Time, []byte) error { return nil }
func (c *blockingWSConn) WriteMessage(int, time.Time, []byte) error { return nil }

// TestSessionCloseDoesNotRaceServeStartup pins a data race between
// Session.Serve and Session.Close on s.pingCancel.
//
// The concurrency here is not invented by the test: it is the shape
// ConnectToProxyWithDialer itself creates —
//
//	go func() { session.Serve(ctx) }()   // Serve → startPings: s.pingCancel = cancel
//	defer session.Close()                // Close → stopPings:  if s.pingCancel == nil
//
// When the caller's context is cancelled just after connecting, the deferred
// Close runs concurrently with Serve's startup and the two touch s.pingCancel
// with no synchronization, even though Session carries a sync.RWMutex that
// every neighbouring field already uses.
//
// In production the two are separated by the lifetime of a session, which is
// why this survived: the window only closes to nothing when sessions are
// opened and torn down rapidly — a reconnect storm, or a test suite driving
// real tunnels. It was found by `go test -race` doing exactly that.
//
// The loop gives the detector repeated chances at the window; a single
// iteration can park the two goroutines apart by chance. 200 iterations
// trips it reliably while staying well under a second of wall time.
func TestSessionCloseDoesNotRaceServeStartup(t *testing.T) {
	for i := 0; i < 200; i++ {
		conn := &blockingWSConn{closed: make(chan struct{})}
		s := newSession(int64(i), "race-probe", conn)
		// What NewClientSession sets: only the client side sends pings, so
		// only the client side runs startPings.
		s.client = true

		ctx, cancel := context.WithCancel(context.Background())
		served := make(chan struct{})
		go func() {
			defer close(served)
			_, _ = s.Serve(ctx)
		}()

		// The deferred Close from ConnectToProxyWithDialer, arriving while
		// Serve is still starting up.
		s.Close()

		// Unwind: release the read loop, stop the ping goroutine if it did
		// start, and wait for both so iterations cannot bleed into each other.
		_ = conn.Close()
		cancel()
		<-served
		s.pingWait.Wait()
	}
}
