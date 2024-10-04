package main

import (
	"context"
	"fmt"
	"io"
	"math/rand"
	"net"
	"net/http"
	"time"

	"github.com/gorilla/mux"
	"github.com/rancher/dynamiclistener"
	"github.com/rancher/dynamiclistener/server"
	"github.com/sirupsen/logrus"

	"github.com/rancher/remotedialer"
)

const (
	namespace  = "cattle-system"
	tlsName    = "apiserver-poc.default.svc"
	certName   = "cattle-apiextension-tls"
	caName     = "cattle-apiextension-ca"
	tcpPort    = 6666
	extAPIPort = 3333
)

func runProxyListener(ctx context.Context, server *remotedialer.Server) error {
	l, err := net.Listen("tcp", fmt.Sprintf("0.0.0.0:%d", tcpPort)) //this RDP app starts only once and always running
	if err != nil {
		return err
	}
	defer l.Close()

	for {
		conn, err := l.Accept() // the client of 6666 is kube-apiserver, according to the APIService object spec, just to this TCP 6666
		if err != nil {
			logrus.Errorf("proxy TCP connection accept failed: %v", err)
			continue
		}

		go func() {
			var client string
			for client == "" {
				clients := server.ListClients()
				if len(clients) == 0 {
					logrus.Info("proxy TCP connection failed: no clients")
					time.Sleep(time.Second)
					continue
				}
				client = clients[rand.Intn(len(clients))]
			}

			peerAddr := fmt.Sprintf(":%d", extAPIPort) // rancher's special https server for imperative API
			clientConn, err := server.Dialer(client)(ctx, "tcp", peerAddr)
			if err != nil {
				logrus.Errorf("proxy dialing %s failed: %v", peerAddr, err)
				conn.Close()
				return
			}

			go pipe(conn, clientConn)
			go pipe(clientConn, conn)
		}()

	}
}

func pipe(a, b net.Conn) {
	defer func(a net.Conn) {
		if err := a.Close(); err != nil {
			logrus.Errorf("proxy TCP connection close failed: %v", err)
		}
	}(a)
	defer func(b net.Conn) {
		if err := b.Close(); err != nil {
			logrus.Errorf("proxy TCP connection close failed: %v", err)
		}
	}(b)
	n, err := io.Copy(a, b)
	if err != nil {
		logrus.Errorf("proxy copy failed: %v", err)
		return
	}
	logrus.Debugf("proxy copied %d bytes to %v from %v", n, a.LocalAddr(), b.LocalAddr())
}

func main() {
	logrus.SetLevel(logrus.DebugLevel)
	ctx := context.Background()
	router := mux.NewRouter()
	authorizer := func(req *http.Request) (string, bool, error) {
		// TODO: Actually do authorization here with a shared Secret
		id := req.Header.Get("X-Tunnel-ID")
		if id == "" {
			return "", false, fmt.Errorf("X-Tunnel-ID not specified in request header")
		}
		return id, true, nil
	}
	remoteDialerServer := remotedialer.New(authorizer, remotedialer.DefaultErrorWriter)

	// rancher will connect via its remotedialer port-forwarder
	router.Handle("/connect", http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		remoteDialerServer.ServeHTTP(w, req)
	}))

	go func() {
		if err := runProxyListener(ctx, remoteDialerServer); err != nil {
			logrus.Fatalf("proxy listener failed to start in the background: %v", err)
		}
	}()

	// the secret will be created in the cluster.
	// only started once, always running, RDP pod, this app
	if err := server.ListenAndServe(ctx, 0, 5555, router, &server.ListenOpts{
		//Secrets:       wContext.Core.Secret(),
		CAName:        caName,
		CANamespace:   namespace,
		CertName:      certName,
		CertNamespace: namespace,
		TLSListenerConfig: dynamiclistener.Config{
			SANs: []string{tlsName},
			FilterCN: func(cns ...string) []string {
				return []string{tlsName}
			},
		},
	}); err != nil {
		logrus.Errorf("extension server exited with an error: %v", err)
	}
	<-ctx.Done()
}
