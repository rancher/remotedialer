package remotedialer

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"testing"

	"github.com/gorilla/mux"
)

func TestBasic(t *testing.T) {
	// start a server
	PrintTunnelData = true
	handler := New(authorizer, DefaultErrorWriter)
	handler.ClientConnectAuthorizer = func(proto, address string) bool {
		return true
	}

	fmt.Println("Listening on ", "127.0.0.1:22222")
	errChan := make(chan error, 3)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	router := mux.NewRouter()
	router.Handle("/connect", handler)
	wsServer := &http.Server{
		Handler: router,
		Addr:    "127.0.0.1:22222",
	}

	go func() {
		defer cancel()

		err := wsServer.ListenAndServe()
		if err != nil {
			errChan <- err
		}
	}()

	router2 := mux.NewRouter()
	router2.Handle("/hello", http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		_, _ = fmt.Fprintf(writer, "Hi")
	}))
	helloServer := &http.Server{
		Handler: router2,
		Addr:    "127.0.0.1:22223",
	}

	go func() {
		defer cancel()

		err := helloServer.ListenAndServe()
		if err != nil {
			errChan <- err
		}
	}()

	go func() {
		err := ClientConnect(ctx, "ws://127.0.0.1:22222/connect", http.Header{
			"X-Tunnel-ID": []string{"foo"},
		}, nil, func(string, string) bool { return true }, func(ctx context.Context, session *Session) error {
			client := &http.Client{
				Transport: &http.Transport{
					DialContext: session.Dial,
				},
			}

			resp, err := client.Get("http://127.0.0.1:22223/hello")
			if err != nil {
				return err
			}
			defer resp.Body.Close()

			out, err := io.ReadAll(resp.Body)
			if err != nil {
				return fmt.Errorf("read body: %w", err)
			} else if string(out) != "Hi" {
				return fmt.Errorf("unexpected answer: %s", string(out))
			}

			errChan <- nil
			return nil
		})
		errChan <- err
	}()

	err := <-errChan
	_ = wsServer.Close()
	_ = helloServer.Close()
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
}

func authorizer(req *http.Request) (string, bool, error) {
	id := req.Header.Get("x-tunnel-id")
	return id, id != "", nil
}
