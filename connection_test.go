package remotedialer

import (
	"errors"
	"sync"
	"testing"
)

// TestConnection_TunnelCloseRace exercises concurrent access to connection.err:
// doTunnelClose can be reached simultaneously from Session.Close,
// Session.closeConnection and the client pipe, while Write checks err from the
// data path. Run with -race to detect regressions.
func TestConnection_TunnelCloseRace(t *testing.T) {
	s := setupDummySession(t, 0)
	s.conn = &fakeWSConn{}

	conn := newConnection(getDummyConnectionID(), s, "test", "test")

	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			conn.doTunnelClose(errors.New("tunnel disconnect"))
		}()
		go func() {
			defer wg.Done()
			_, _ = conn.Write([]byte("data"))
		}()
	}
	wg.Wait()

	if conn.getErr() == nil {
		t.Fatal("expected connection to be closed with an error")
	}
}
