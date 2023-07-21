package remotedialer

import (
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/pkg/errors"
	"k8s.io/klog/v2"
)

var (
	errFailedAuth = errors.New("failed authentication")
)

type PeerAuthorizer func(req *http.Request, id, token string) bool
type Authorizer func(req *http.Request) (clientKey string, authed bool, err error)
type ErrorWriter func(rw http.ResponseWriter, req *http.Request, code int, err error)

func DefaultErrorWriter(rw http.ResponseWriter, _ *http.Request, code int, err error) {
	rw.WriteHeader(code)
	_, _ = rw.Write([]byte(err.Error()))
}

type Server struct {
	PeerID                  string
	PeerToken               string
	PeerAuthorizer          PeerAuthorizer
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
	clientKey, authed, peer, err := s.auth(req)
	if err != nil {
		s.errorWriter(rw, req, 400, err)
		return
	}
	if !authed {
		s.errorWriter(rw, req, 401, errFailedAuth)
		return
	}

	logger := klog.FromContext(req.Context())

	logger.Info("Handling backend connection request", "clientKey", clientKey)

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

	session, err := s.sessions.add(req.Context(), clientKey, wsConn, peer)
	if err != nil {
		s.errorWriter(rw, req, 400, errors.Wrapf(err, "Error during creating session for host [%v]", clientKey))
		return
	}
	session.auth = s.ClientConnectAuthorizer
	defer s.sessions.remove(session)

	code, err := session.Serve(req.Context())
	if err != nil {
		// Hijacked so we can't write to the client
		logger.Error(err, "Error in remotedialer server", "code", code)
	}
}

func (s *Server) auth(req *http.Request) (clientKey string, authed, peer bool, err error) {
	id := req.Header.Get(ID)
	token := req.Header.Get(Token)
	if id != "" && token != "" {
		// peer authentication
		s.peerLock.Lock()
		p, ok := s.peers[id]
		s.peerLock.Unlock()
		if ok {
			if s.PeerAuthorizer != nil && s.PeerAuthorizer(req, id, token) {
				return id, true, true, nil
			} else if p.token != "" && p.token == token {
				return id, true, true, nil
			}
		}
	}

	id, authed, err = s.authorizer(req)
	return id, authed, false, err
}
