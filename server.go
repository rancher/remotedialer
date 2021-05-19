package remotedialer

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

var (
	errFailedAuth       = errors.New("failed authentication")
	errWrongMessageType = errors.New("wrong websocket message type")
)
var Proxy = "X-API-Tunnel-Proxy"

type Authorizer func(req *http.Request) (clientKey string, authed bool, err error)
type ErrorWriter func(rw http.ResponseWriter, req *http.Request, code int, err error)

func DefaultErrorWriter(rw http.ResponseWriter, req *http.Request, code int, err error) {
	rw.WriteHeader(code)
	rw.Write([]byte(err.Error()))
}

type Server struct {
	PeerID                  string
	PeerToken               string
	ClientConnectAuthorizer ConnectAuthorizer
	authorizer              Authorizer
	errorWriter             ErrorWriter
	sessions                *sessionManager
	peers                   map[string]peer
	peerLock                sync.Mutex
}

func New(auth Authorizer, errorWriter ErrorWriter) *Server {
	return &Server{
		peers:       map[string]peer{},
		authorizer:  auth,
		errorWriter: errorWriter,
		sessions:    newSessionManager(),
	}
}

func (s *Server) ServeHTTP(rw http.ResponseWriter, req *http.Request) {
	clientKey, authed, peer, proxy, err := s.auth(req)
	if err != nil {
		s.errorWriter(rw, req, 400, err)
		return
	}
	if !authed {
		s.errorWriter(rw, req, 401, errFailedAuth)
		return
	}

	logrus.Infof("Handling backend connection request [%s]", clientKey)

	upgrader := websocket.Upgrader{
		HandshakeTimeout: 5 * time.Second,
		CheckOrigin:      func(r *http.Request) bool { return true },
		Error:            s.errorWriter,
	}

	wsConn, err := upgrader.Upgrade(rw, req, nil)
	if err != nil {
		s.errorWriter(rw, req, 400, errors.Wrapf(err, "Error during upgrade for host [%v]", clientKey))
		return
	}
	var session *Session
	if !proxy {
		session = s.sessions.add(clientKey, wsConn, peer)
		defer s.sessions.remove(session)
	} else {
		session = NewProxySession(func(string, string) bool { return true }, wsConn)
		session.dialer = func(ctx context.Context, network, address string) (net.Conn, error) {
			parts := strings.SplitN(network, "::", 2)
			if len(parts) != 2 {
				return nil, fmt.Errorf("invalid clientKey/proto: %s", network)
			}
			d := s.Dialer(parts[0])
			return d(ctx, parts[1], address)
		}
		defer session.Close()
	}
	session.auth = s.ClientConnectAuthorizer
	code, err := session.Serve(req.Context())
	if err != nil {
		// Hijacked so we can't write to the client
		logrus.Infof("error in remotedialer server [%d]: %v", code, err)
	}
}

func (s *Server) auth(req *http.Request) (clientKey string, authed, peer, proxy bool, err error) {
	id := req.Header.Get(ID)
	token := req.Header.Get(Token)
	isProxy := req.Header.Get(Proxy)
	if id != "" && token != "" {
		// peer authentication
		s.peerLock.Lock()
		p, ok := s.peers[id]
		s.peerLock.Unlock()

		if ok && p.token == token {
			return id, true, true, false, nil
		}
	}
	id, authed, err = s.authorizer(req)
	if id != "" && isProxy != "" {
		return id, true, false, true, nil
	}
	return id, authed, false, false, err
}
