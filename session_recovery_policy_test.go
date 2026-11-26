package remotedialer

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestPreferResponsiveSession(t *testing.T) {
	now := time.Now()
	oldSocket := &wsWrapper{connectedAt: now.Add(-time.Minute)}
	oldest := &Session{conn: oldSocket}
	newSocket := &wsWrapper{connectedAt: now}
	replacement := &Session{conn: newSocket}
	sessions := []*Session{oldest, replacement}

	require.Same(t, oldest, preferResponsiveSession(sessions[:1]))
	require.Same(t, oldest, preferResponsiveSession(sessions), "registration alone does not prove activity")

	newSocket.lastHeartbeat.Store(&now)
	require.Same(t, replacement, preferResponsiveSession(sessions))

	fresh := now.Add(-time.Second)
	oldSocket.lastHeartbeat.Store(&fresh)
	require.Same(t, oldest, preferResponsiveSession(sessions), "retain a responsive oldest session")

	expired := now.Add(-time.Minute)
	oldSocket.lastHeartbeat.Store(&expired)
	newSocket.lastHeartbeat.Store(&expired)
	require.Same(t, oldest, preferResponsiveSession(sessions), "do not prefer an equally stale session")
}
