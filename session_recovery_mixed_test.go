package remotedialer

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestRecoveryAgentProcess(t *testing.T) {
	tunnel := os.Getenv("RECOVERY_TEST_TUNNEL")
	if tunnel == "" {
		t.Skip("agent process helper")
	}
	target := os.Getenv("RECOVERY_TEST_TARGET")
	require.True(t, strings.HasPrefix(tunnel, "ws://127.0.0.1:"))
	require.True(t, strings.HasPrefix(target, "127.0.0.1:"))
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	err := ConnectToProxy(ctx, tunnel, nil, func(network, address string) bool {
		return network == "tcp" && address == target
	}, nil, nil)
	require.NoError(t, err)
}

func TestRecoveryMixedVersion(t *testing.T) {
	agentBinary := os.Getenv("RECOVERY_TEST_AGENT_BINARY")
	if agentBinary == "" {
		t.Skip("requires test binary compiled from the other version")
	}
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1":
			fmt.Fprintln(w, "fixture")
		case "/watch":
			fmt.Fprintln(w, "event 1")
			w.(http.Flusher).Flush()
			<-r.Context().Done()
		default:
			http.NotFound(w, r)
		}
	}))
	defer backend.Close()
	target, err := url.Parse(backend.URL)
	require.NoError(t, err)
	server := New(func(*http.Request) (string, bool, error) {
		return "mixed-version-fixture", true, nil
	}, DefaultErrorWriter)
	tunnel := httptest.NewServer(server)
	defer tunnel.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	agent := exec.CommandContext(ctx, agentBinary, "-test.run=^TestRecoveryAgentProcess$")
	agent.Env = append(os.Environ(),
		"RECOVERY_TEST_TUNNEL=ws"+strings.TrimPrefix(tunnel.URL, "http"),
		"RECOVERY_TEST_TARGET="+target.Host)
	require.NoError(t, agent.Start())
	defer func() {
		cancel()
		_ = agent.Wait()
	}()
	require.Eventually(t, func() bool { return server.HasSession("mixed-version-fixture") }, 4*time.Second, 10*time.Millisecond)
	transport := &http.Transport{DialContext: server.Dialer("mixed-version-fixture")}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, Timeout: 8 * time.Second}
	stream, err := client.Get(backend.URL + "/watch")
	require.NoError(t, err)
	defer stream.Body.Close()
	event, err := bufio.NewReader(stream.Body).ReadString('\n')
	require.NoError(t, err)
	require.Equal(t, "event 1\n", event)
	response, err := client.Get(backend.URL + "/v1")
	require.NoError(t, err)
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, response.StatusCode)
	require.Equal(t, "fixture\n", string(body))
}
