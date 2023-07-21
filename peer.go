package remotedialer

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/gorilla/websocket"
	"github.com/loft-sh/remotedialer/metrics"
	"k8s.io/klog/v2"
)

var (
	Token = "X-API-Tunnel-Token"
	ID    = "X-API-Tunnel-ID"
)

func (s *Server) AddPeer(url, id, token string) {
	if s.PeerID == "" || s.PeerToken == "" {
		return
	}

	ctx, cancel := context.WithCancel(context.Background())
	peer := peer{
		url:    url,
		id:     id,
		token:  token,
		cancel: cancel,
	}

	klog.FromContext(ctx).Info("Adding peer", "url", url, "id", id)

	s.peerLock.Lock()
	defer s.peerLock.Unlock()

	if p, ok := s.peers[id]; ok {
		if p.equals(peer) {
			return
		}
		p.cancel()
	}

	s.peers[id] = peer
	go peer.start(ctx, s)
}

func (s *Server) RemovePeer(ctx context.Context, id string) {
	s.peerLock.Lock()
	defer s.peerLock.Unlock()

	if p, ok := s.peers[id]; ok {
		klog.FromContext(ctx).Info("Removing peer", "id", id)
		p.cancel()
	}
	delete(s.peers, id)
}

type peer struct {
	url, id, token string
	cancel         func()
}

func (p peer) equals(other peer) bool {
	return p.url == other.url &&
		p.id == other.id &&
		p.token == other.token
}

func (p *peer) start(ctx context.Context, s *Server) {
	headers := http.Header{
		ID:    {s.PeerID},
		Token: {s.PeerToken},
	}

	dialer := &websocket.Dialer{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: true,
		},
		HandshakeTimeout: HandshakeTimeOut,
	}

	logger := klog.FromContext(ctx)

outer:
	for {
		select {
		case <-ctx.Done():
			break outer
		default:
		}

		metrics.IncSMTotalAddPeerAttempt(p.id)
		ws, _, err := dialer.Dial(p.url, headers)
		if err != nil {
			logger.Error(err, "Failed to connect to peer", "url", p.url, "id", s.PeerID)
			time.Sleep(5 * time.Second)
			continue
		}
		metrics.IncSMTotalPeerConnected(p.id)

		session, err := NewClientSession(ctx, func(string, string) bool { return true }, ws)
		if err != nil {
			logger.Error(err, "Failed to connect to peer", "url", p.url)

			time.Sleep(5 * time.Second)
			continue
		}

		session.dialer = func(ctx context.Context, network, address string) (net.Conn, error) {
			parts := strings.SplitN(network, "::", 2)
			if len(parts) != 2 {
				return nil, fmt.Errorf("invalid clientKey/proto: %s", network)
			}
			d := s.Dialer(parts[0])
			return d(ctx, parts[1], address)
		}

		s.sessions.addListener(session)
		_, err = session.Serve(ctx)
		s.sessions.removeListener(session)
		session.Close()

		if err != nil {
			logger.Error(err, "Failed to serve peer connection", "id", p.id)
		}

		ws.Close()
		time.Sleep(5 * time.Second)
	}
}
