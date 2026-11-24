package remotedialer

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"
)

// dropWrites models an agent-to-Rancher blackhole while TCP remains writable
// from Rancher's point of view. It is enabled only after a successful request.
type dropWrites struct {
	net.Conn
	drop atomic.Bool
}

func (c *dropWrites) Write(p []byte) (int, error) {
	if c.drop.Load() {
		return len(p), nil
	}
	return c.Conn.Write(p)
}

func TestFreshDialAvoidsHalfOpenSession(t *testing.T) {
	const cluster = "stv-cluster-fixture"
	var requests atomic.Int32
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		fmt.Fprintln(w, "fixture")
	}))
	defer backend.Close()
	target, err := url.Parse(backend.URL)
	require.NoError(t, err)

	server := New(func(*http.Request) (string, bool, error) {
		return cluster, true, nil
	}, DefaultErrorWriter)
	tunnel := httptest.NewServer(server)
	defer tunnel.Close()

	sessions := func() []*Session {
		server.sessions.Lock()
		defer server.sessions.Unlock()
		return append([]*Session(nil), server.sessions.clients[cluster]...)
	}
	connect := func(wantSessions int) *dropWrites {
		ctx, cancel := context.WithCancel(context.Background())
		var conn *dropWrites
		dialer := websocket.Dialer{
			NetDialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
				tcp, err := (&net.Dialer{}).DialContext(ctx, network, address)
				if err != nil {
					return nil, err
				}
				conn = &dropWrites{Conn: tcp}
				return conn, nil
			},
		}
		ws, _, err := dialer.DialContext(ctx, "ws"+strings.TrimPrefix(tunnel.URL, "http")+"/v3/connect", nil)
		if err != nil {
			cancel()
			t.Fatal(err)
		}
		agent := NewClientSession(func(network, addr string) bool {
			return network == "tcp" && addr == target.Host
		}, ws)
		done := make(chan struct{})
		go func() {
			defer close(done)
			_, _ = agent.Serve(ctx)
			agent.Close()
		}()
		t.Cleanup(func() {
			cancel()
			_ = ws.Close()
			<-done
		})
		require.Eventually(t, func() bool { return len(sessions()) == wantSessions }, time.Second, time.Millisecond)
		return conn
	}

	newProxy := func(dial Dialer) *httptest.Server {
		transport := &http.Transport{DialContext: dial, DisableKeepAlives: true}
		proxy := httputil.NewSingleHostReverseProxy(target)
		proxy.Transport = transport
		proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
		instance := httptest.NewServer(proxy)
		t.Cleanup(instance.Close)
		return instance
	}

	type result struct {
		status int
		body   string
		err    error
	}
	request := func(proxy *httptest.Server, timeout time.Duration) result {
		client := &http.Client{Timeout: timeout}
		resp, err := client.Get(proxy.URL + "/v1")
		if err != nil {
			return result{err: err}
		}
		defer resp.Body.Close()
		body, err := io.ReadAll(resp.Body)
		return result{resp.StatusCode, strings.TrimSpace(string(body)), err}
	}
	wantOK := result{status: http.StatusOK, body: "fixture"}

	oldWire := connect(1)
	normal := newProxy(server.Dialer(cluster))
	require.Equal(t, wantOK, request(normal, 2*time.Second))
	oldWire.drop.Store(true)
	start := time.Now()

	connect(2)
	current := sessions()
	require.Len(t, current, 2)
	// The same HTTP request succeeds if routed explicitly to the replacement
	// session. Normal requests still go through the unmodified selector.
	control := newProxy(toDialer(current[1], ""))
	require.Equal(t, wantOK, request(control, 2*time.Second))
	t.Logf("replacement session served the control request after %s", time.Since(start).Round(time.Millisecond))

	waiting := make(chan result, 1)
	go func() { waiting <- request(normal, PingWaitDuration+5*time.Second) }()
	// The old agent received the request, so the stall is on its response path.
	require.Eventually(t, func() bool { return requests.Load() == 3 }, 2*time.Second, time.Millisecond)
	select {
	case got := <-waiting:
		t.Fatalf("normal request returned before the old session expired: %+v", got)
	case <-time.After(300 * time.Millisecond):
	}
	// Allow the replacement to send its heartbeats and the old session to
	// miss two intervals. The old session is still registered at this point.
	time.Sleep(time.Until(start.Add(2*PingWriteInterval + time.Second)))
	require.Len(t, sessions(), 2)
	require.Equal(t, wantOK, request(normal, 2*time.Second),
		"a fresh dial should use the healthy replacement before stale cleanup")
	t.Logf("fresh dial served after %s while the old session still existed", time.Since(start).Round(time.Millisecond))

	// Natural cleanup still closes a request already on the old session.
	require.Equal(t, result{status: http.StatusInternalServerError, body: "tunnel disconnect"}, <-waiting)
	require.Eventually(t, func() bool { return len(sessions()) == 1 }, time.Second, time.Millisecond)
	require.Equal(t, wantOK, request(normal, 2*time.Second))
	t.Logf("confirmed stale session cleanup after %s", time.Since(start).Round(time.Millisecond))
}
