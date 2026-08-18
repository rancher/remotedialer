package remotedialer

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// A failed handshake must be retried, and cancellation must end the loop.
func TestClientConnectWithOptsRetriesUntilCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var mu sync.Mutex
	attempts := 0

	// Refusing the upgrade makes every attempt fail its handshake.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		attempts++
		reached := attempts
		mu.Unlock()

		if reached == 3 {
			cancel()
		}
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http")
	err := ClientConnectWithOpts(ctx, wsURL, nil, nil, nil, nil,
		&ConnectOpts{Backoff: Backoff{Min: time.Millisecond}})

	if !errors.Is(err, context.Canceled) {
		t.Errorf("ClientConnectWithOpts() = %v, want context.Canceled", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if attempts < 3 {
		t.Errorf("server saw %d attempt(s), want at least 3", attempts)
	}
}
