package remotedialer

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sort"
	"testing"

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
