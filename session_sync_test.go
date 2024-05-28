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
	for _, tt := range tests {
		t.Run(fmt.Sprintf("%d_ids", tt.size), func(t *testing.T) {
			ids := generateIDs(tt.size)
			encoded := encodeConnectionIDs(ids)
			decoded, err := decodeConnectionIDs(encoded)
			if err != nil {
				t.Error(err)
			}
			if got, want := decoded, ids; !reflect.DeepEqual(got, want) {
				t.Errorf("encoding and decoding differs from original data, got: %v, want: %v", got, want)
			}
		})
	}
}

func Test_diffSortedSetsGetRemoved(t *testing.T) {
	server := []int64{3, 5, 20, 50, 100}
	client := []int64{3, 50, 100, 200}
	expected := []int64{5, 20}

	if got, want := diffSortedSetsGetRemoved(server, client), expected; !reflect.DeepEqual(got, want) {
		t.Errorf("unexpected result, got: %v, want: %v", got, want)
	}
}

func TestSession_sendSyncConnections(t *testing.T) {
	data := make(chan []byte)
	conn := testServerWS(t, data)
	session := newSession(rand.Int63(), "sync-test", newWSConn(conn))

	for _, n := range []int{0, 5, 20} {
		ids := generateIDs(n)
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
		if decoded, err := decodeConnectionIDs(payload); err != nil {
			t.Fatal(err)
		} else if got, want := decoded, session.activeConnectionIDs(); !reflect.DeepEqual(got, want) {
			t.Errorf("incorrect connections IDs, got: %v, want: %v", got, want)
		}
	}
}

func generateIDs(n int) []int64 {
	ids := make([]int64, n)
	for x := range ids {
		ids[x] = rand.Int63()
	}
	return ids
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
