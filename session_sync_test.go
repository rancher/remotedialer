package remotedialer

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"math/rand"
	"net"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func Test_encodeConnectionIDs(t *testing.T) {
	t.Parallel()
	tests := []struct {
		size int
	}{
		{0},
		{1},
		{2},
		{10},
		{100},
		{1000},
	}
	for x := range tests {
		tt := tests[x]
		t.Run(fmt.Sprintf("%d_ids", tt.size), func(t *testing.T) {
			t.Parallel()
			ids, top := generateIDs(tt.size)
			encoded := encodeConnectionIDs(top, ids)
			decoded, decodedTop, err := decodeConnectionIDs(encoded)
			if err != nil {
				t.Error(err)
			}
			if got, want := decodedTop, top; !reflect.DeepEqual(got, want) {
				t.Errorf("encoding and decoding differs from original data, got: %v, want: %v", got, want)
			}

			if got, want := decoded, ids; !reflect.DeepEqual(got, want) {
				t.Errorf("encoding and decoding differs from original data, got: %v, want: %v", got, want)
			}
		})
	}
}

func Test_diffSortedSetsGetRemoved(t *testing.T) {
	t.Parallel()
	tests := []struct {
		server, client, expected []int64
	}{
		{
			// same ids
			server:   []int64{2, 4, 6},
			client:   []int64{2, 4, 6},
			expected: nil,
		},
		{
			// Client keeps all ids from the server, additional ids are okay
			server:   []int64{1, 2, 3},
			client:   []int64{1, 2, 3, 4, 5},
			expected: nil,
		},
		{
			// Client closed some ids kept by the server
			server:   []int64{1, 2, 3, 4, 5},
			client:   []int64{1, 2, 3},
			expected: []int64{4, 5},
		},
		{
			// Combined case
			server:   []int64{1, 2, 3, 4, 5},
			client:   []int64{3, 6},
			expected: []int64{1, 2, 4, 5},
		},
	}
	for x := range tests {
		tt := tests[x]
		t.Run(fmt.Sprintf("case_%d", x), func(t *testing.T) {
			t.Parallel()
			if got, want := diffSortedSetsGetRemoved(tt.server, tt.client), tt.expected; !reflect.DeepEqual(got, want) {
				t.Errorf("unexpected result, got: %v, want: %v", got, want)
			}
		})
	}
}

func TestSession_sendSyncConnections(t *testing.T) {
	t.Parallel()

	data := make(chan []byte)
	conn := testServerWS(t, data)
	session := newSession(rand.Int63(), "sync-test", newWSConn(conn))

	for _, n := range []int{0, 5, 20} {
		ids, _ := generateIDs(n)
		for _, id := range ids {
			session.conns[id] = nil
		}
		sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })

		if err := session.sendSyncConnections(); err != nil {
			t.Fatal(err)
		}
		message, err := newServerMessage(bytes.NewBuffer(<-data))
		if err != nil {
			t.Fatal(err)
		}
		payload, err := io.ReadAll(message.body)
		if err != nil {
			t.Fatal(err)
		}

		if got, want := message.messageType, SyncConnections; got != want {
			t.Errorf("incorrect message type, got: %v, want: %v", got, want)
		}

		decoded, decodedTop, err := decodeConnectionIDs(payload)
		if err != nil {
			t.Fatal(err)
			return
		}
		returnedIDs, returnedTop := session.activeConnectionIDs()
		if got, want := decodedTop, returnedTop; !reflect.DeepEqual(got, want) {
			t.Errorf("incorrect connections IDs, got: %v, want: %v", got, want)
		}
		if got, want := decoded, returnedIDs; !reflect.DeepEqual(got, want) {
			t.Errorf("incorrect connections IDs, got: %v, want: %v", got, want)
		}
	}
}

func generateIDs(n int) ([]int64, int64) {
	ids := make([]int64, n)
	for x := range ids {
		ids[x] = rand.Int63()
	}
	return ids, rand.Int63()
}

func testServerWS(t *testing.T, data chan<- []byte) *websocket.Conn {
	t.Helper()

	var upgrader websocket.Upgrader
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer c.Close()
		for {
			_, message, err := c.ReadMessage()
			if err != nil {
				return
			}
			if data != nil {
				data <- message
			}
		}
	}))
	t.Cleanup(server.Close)

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	url := "ws" + server.URL[4:] // http:// -> ws://
	conn, _, err := websocket.DefaultDialer.DialContext(ctx, url, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return conn
}

// TestCompareAndCloseStaleConnections tests that compareAndCloseStaleConnections
// correctly handles the 'top' field: connections with IDs greater than top should
// never be closed (the client hasn't seen them yet), while stale connections with
// IDs <= top should be closed.
func TestCompareAndCloseStaleConnections(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		serverConns    []int64
		clientConns    []int64
		top            int64
		expectedOpen   []int64
		expectedClosed []int64
	}{
		{
			name:           "no connections to close when lists match",
			serverConns:    []int64{1, 2, 3},
			clientConns:    []int64{1, 2, 3},
			top:            3,
			expectedOpen:   []int64{1, 2, 3},
			expectedClosed: nil,
		},
		{
			name:           "close stale connections at or below top",
			serverConns:    []int64{1, 2, 3, 4, 5},
			clientConns:    []int64{1, 3, 5},
			top:            5,
			expectedOpen:   []int64{1, 3, 5},
			expectedClosed: []int64{2, 4},
		},
		{
			name:           "preserve all connections above top",
			serverConns:    []int64{1, 2, 3, 4, 5},
			clientConns:    []int64{1, 2, 3},
			top:            3,
			expectedOpen:   []int64{1, 2, 3, 4, 5},
			expectedClosed: nil,
		},
		{
			name:           "mixed: close old stale and preserve new unknown",
			serverConns:    []int64{1, 2, 3, 4, 5, 10, 20},
			clientConns:    []int64{1, 3},
			top:            5,
			expectedOpen:   []int64{1, 3, 10, 20},
			expectedClosed: []int64{2, 4, 5},
		},
		{
			name:           "empty client list with low top preserves everything",
			serverConns:    []int64{5, 10, 15},
			clientConns:    []int64{},
			top:            0,
			expectedOpen:   []int64{5, 10, 15},
			expectedClosed: nil,
		},
		{
			name:           "empty client list with high top closes all",
			serverConns:    []int64{1, 2, 3},
			clientConns:    []int64{},
			top:            10,
			expectedOpen:   nil,
			expectedClosed: []int64{1, 2, 3},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			s := setupDummySession(t, 0)
			for _, id := range tt.serverConns {
				conn := newConnection(id, s, "test", "test")
				s.addConnection(id, conn)
			}

			s.compareAndCloseStaleConnections(tt.clientConns, tt.top)

			for _, id := range tt.expectedOpen {
				if s.getConnection(id) == nil {
					t.Errorf("connection %d should be open but was closed", id)
				}
			}

			for _, id := range tt.expectedClosed {
				if s.getConnection(id) != nil {
					t.Errorf("connection %d should be closed but is still open", id)
				}
			}
		})
	}
}

// TestSyncConnectionsRace is an integration test that reproduces the race condition
// described in PR #150. When SyncConnectionsInterval is very small and many concurrent
// requests are being processed, the sync mechanism can close connections that the client
// hasn't yet received. The fix adds a 'top' field to sync packets so the server knows
// not to close connections with IDs the client hasn't seen yet.
//
// The test runs two subtests:
//   - without_fix: disables the 'top' guard via syncIgnoreTop, proving the bug exists
//     (sync error rate > 10%)
//   - with_fix: runs with the 'top' guard enabled, proving the fix works
//     (sync error rate < 10%)
//
// Note: With the 'top' field fix, connections the client hasn't seen are protected.
// However, there is a separate (pre-existing) timing window where the client completes
// and removes a connection before the server does. In those cases the sync correctly
// closes the stale server-side connection, but this can briefly interrupt in-flight
// HTTP responses on the server side. The 'top' field does not address this case.
func TestSyncConnectionsRace(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	origInterval := SyncConnectionsInterval
	origTimeout := SyncConnectionsTimeout
	origIgnoreTop := syncIgnoreTop

	// Aggressive sync settings to maximize the race window
	SyncConnectionsInterval = time.Millisecond
	SyncConnectionsTimeout = time.Second

	t.Cleanup(func() {
		SyncConnectionsInterval = origInterval
		SyncConnectionsTimeout = origTimeout
		syncIgnoreTop = origIgnoreTop
	})

	const totalRequests = 500
	const maxErrorPercent = 10
	threshold := int64(totalRequests * maxErrorPercent / 100) // 50

	t.Run("without_fix", func(t *testing.T) {
		syncIgnoreTop = true
		_, syncErrs := runSyncRaceLoad(t, totalRequests)

		// Without the 'top' guard, the error rate should be high (typically >10%),
		// proving the bug is reproducible.
		if syncErrs <= threshold {
			t.Errorf("expected sync error rate >%d%% without fix, but got only %d/%d sync errors",
				maxErrorPercent, syncErrs, totalRequests)
		}
	})

	t.Run("with_fix", func(t *testing.T) {
		syncIgnoreTop = false
		_, syncErrs := runSyncRaceLoad(t, totalRequests)

		// With the 'top' guard, the error rate should be significantly lower (<10%).
		// A small number of errors are expected from the pre-existing race where the
		// client removes a completed connection before the server does.
		if syncErrs > threshold {
			t.Errorf("sync error rate too high with fix: got %d/%d sync errors (max allowed: %d)",
				syncErrs, totalRequests, threshold)
		}
	})
}

// runSyncRaceLoad sets up a remotedialer tunnel and fires totalRequests concurrent HTTP
// requests through it, returning the total error count and sync-specific error count.
// SyncConnectionsInterval and SyncConnectionsTimeout must be set by the caller.
func runSyncRaceLoad(t *testing.T, totalRequests int) (totalErrors, syncErrors int64) {
	t.Helper()

	ctx, cancel := context.WithCancel(context.Background())

	// Backend HTTP server that returns a response body.
	responseBody := strings.Repeat("x", 4096)
	backendAddr, err := newServer(ctx, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(responseBody))
	}))
	if err != nil {
		t.Fatal(err)
	}

	// remotedialer server
	serverAddr, server, err := newTestServer(ctx)
	if err != nil {
		t.Fatal(err)
	}

	// Connect client
	if err := newTestClient(ctx, "ws://"+serverAddr); err != nil {
		t.Fatal(err)
	}

	// HTTP client that routes through the remotedialer tunnel
	client := &http.Client{
		Timeout: 10 * time.Second,
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
				return server.Dialer("client")(ctx, network, address)
			},
			DisableKeepAlives: true,
		},
	}

	const concurrency = 50

	sem := make(chan struct{}, concurrency)
	var (
		wg           sync.WaitGroup
		errCount     int64
		syncErrCount int64
	)

	for i := 0; i < totalRequests; i++ {
		sem <- struct{}{}
		wg.Add(1)
		go func() {
			defer func() {
				<-sem
				wg.Done()
			}()

			resp, err := client.Get("http://" + backendAddr)
			if err != nil {
				atomic.AddInt64(&errCount, 1)
				if strings.Contains(err.Error(), errCloseSyncConnections.Error()) {
					atomic.AddInt64(&syncErrCount, 1)
				}
				return
			}
			_, _ = io.Copy(io.Discard, resp.Body)
			resp.Body.Close()
		}()
	}

	wg.Wait()

	// Stop all goroutines before returning
	cancel()
	time.Sleep(200 * time.Millisecond)

	t.Logf("completed %d requests: %d total errors, %d sync errors",
		totalRequests, errCount, syncErrCount)

	return errCount, syncErrCount
}
