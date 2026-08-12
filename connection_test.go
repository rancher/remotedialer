package remotedialer

import (
	"sync"
	"testing"
	"time"
)

func TestConnectionSetWriteDeadlineConcurrentWrite(t *testing.T) {
	t.Parallel()

	s := setupDummySession(t, 0)
	s.conn = &fakeWSConn{
		writeMessageCallback: func(int, time.Time, []byte) error {
			return nil
		},
	}
	conn := newConnection(getDummyConnectionID(), s, "test", "test")

	const iterations = 1000
	start := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		<-start
		for i := 0; i < iterations; i++ {
			if _, err := conn.Write([]byte("test")); err != nil {
				t.Errorf("Write() error = %v", err)
				return
			}
		}
	}()

	go func() {
		defer wg.Done()
		<-start
		for range iterations {
			if err := conn.SetWriteDeadline(time.Time{}); err != nil {
				t.Errorf("SetWriteDeadline() error = %v", err)
				return
			}
		}
	}()

	close(start)
	wg.Wait()
}
