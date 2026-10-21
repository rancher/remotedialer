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

// ConnectOpts.URL must be consulted before every attempt, so a caller that
// changes its address after the first connection reconnects to the new one.
func TestClientConnectWithOptsResolvesURLPerAttempt(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var mu sync.Mutex
	var paths []string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		paths = append(paths, r.URL.Path)
		reached := len(paths)
		mu.Unlock()

		if reached == 3 {
			cancel()
		}
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	base := "ws" + strings.TrimPrefix(srv.URL, "http")

	// Mimic a client that registers once and then uses the plain path.
	var calls int
	urlFor := func() string {
		mu.Lock()
		defer mu.Unlock()
		calls++
		if calls == 1 {
			return base + "/connect/register"
		}
		return base + "/connect"
	}

	err := ClientConnectWithOpts(ctx, base+"/unused", nil, nil, nil, nil,
		&ConnectOpts{Backoff: Backoff{Min: time.Millisecond}, URL: urlFor})

	if !errors.Is(err, context.Canceled) {
		t.Errorf("ClientConnectWithOpts() = %v, want context.Canceled", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(paths) < 2 {
		t.Fatalf("server saw %d attempt(s), want at least 2", len(paths))
	}
	if paths[0] != "/connect/register" {
		t.Errorf("first attempt path = %q, want %q", paths[0], "/connect/register")
	}
	for i, p := range paths[1:] {
		if p != "/connect" {
			t.Errorf("attempt %d path = %q, want %q", i+2, p, "/connect")
		}
	}
}

// A nil ConnectOpts.URL must keep using the wsURL argument.
func TestClientConnectWithOptsFallsBackToWsURL(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var mu sync.Mutex
	var paths []string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		paths = append(paths, r.URL.Path)
		reached := len(paths)
		mu.Unlock()

		if reached == 2 {
			cancel()
		}
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http") + "/connect"
	err := ClientConnectWithOpts(ctx, wsURL, nil, nil, nil, nil,
		&ConnectOpts{Backoff: Backoff{Min: time.Millisecond}})

	if !errors.Is(err, context.Canceled) {
		t.Errorf("ClientConnectWithOpts() = %v, want context.Canceled", err)
	}

	mu.Lock()
	defer mu.Unlock()
	for i, p := range paths {
		if p != "/connect" {
			t.Errorf("attempt %d path = %q, want %q", i+1, p, "/connect")
		}
	}
}
