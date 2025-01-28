package proxyclient

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
	"github.com/rancher/remotedialer"
	v1 "github.com/rancher/wrangler/v3/pkg/generated/controllers/core/v1"
	"github.com/sirupsen/logrus"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"
)

const (
	defaultServerAddr = "wss://127.0.0.1"
	defaultServerPort = 5555
	defaultServerPath = "/connect"
)

var (
	nonTLSDialer = &websocket.Dialer{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}
	retryTimeout = 1 * time.Second
)

type PortForwarder interface {
	Start() error
	Stop()
}

type ProxyClientOpt func(*ProxyClient)

type ProxyClient struct {
	forwarder           PortForwarder
	serverUrl           string
	serverConnectSecret string
	dialer              *websocket.Dialer
	secretController    v1.SecretController

	onConnect func(ctx context.Context, session *remotedialer.Session) error
}

func New(serverSharedSecret, namespace, certSecretName, certServerName string, restConfig *rest.Config, forwarder PortForwarder, opts ...ProxyClientOpt) (*ProxyClient, error) {
	if restConfig == nil {
		return nil, fmt.Errorf("restConfig required")
	}

	if forwarder == nil {
		return nil, fmt.Errorf("a PortForwarder must be provided")
	}

	if namespace == "" {
		return nil, fmt.Errorf("namespace required")
	}

	if certSecretName == "" {
		return nil, fmt.Errorf("certSecretName required")
	}

	if serverSharedSecret == "" {
		return nil, fmt.Errorf("server shared secret must be provided")
	}

	serverUrl := fmt.Sprintf("%s:%d%s", defaultServerAddr, defaultServerPort, defaultServerPath)
	dialer, err := buildDialer(namespace, certSecretName, certServerName, restConfig)
	if err != nil {
		return nil, fmt.Errorf("couldn't build dialer: %w", err)
	}

	client := &ProxyClient{
		serverUrl:           serverUrl,
		forwarder:           forwarder,
		dialer:              dialer,
		serverConnectSecret: serverSharedSecret,
	}

	for _, opt := range opts {
		opt(client)
	}

	return client, nil
}

func buildDialer(namespace, certSecretName, certServerName string, restConfig *rest.Config) (*websocket.Dialer, error) {
	secretController, err := remotedialer.BuildSecretController(restConfig)
	if err != nil {
		logrus.Error("build secret controller failed: %w, defaulting to non TLS connection", err)
		return nonTLSDialer, nil
	}

	secret, err := secretController.Get(namespace, certSecretName, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}

	crtData, exists := secret.Data["tls.crt"]
	if !exists {
		return nil, fmt.Errorf("secret %s/%s missing tls.crt field", namespace, certSecretName)
	}

	rootCAs := x509.NewCertPool()
	if ok := rootCAs.AppendCertsFromPEM(crtData); !ok {
		return nil, fmt.Errorf("failed to parse tls.crt from secret into a CA pool")
	}

	return &websocket.Dialer{
		TLSClientConfig: &tls.Config{
			RootCAs:    rootCAs,
			ServerName: certServerName,
		},
	}, nil
}

func (c *ProxyClient) Run(ctx context.Context) {
	go func() {
		for {
			select {
			case <-ctx.Done():
				logrus.Infof("ProxyClient: ClientConnect finished. If no error, the session closed cleanly.")
				return

			default:
				if err := c.forwarder.Start(); err != nil {
					logrus.Errorf("remotedialer.ProxyClient error: %s ", err)
					time.Sleep(retryTimeout)
					continue
				}

				logrus.Infof("ProxyClient connecting to %s", c.serverUrl)

				headers := http.Header{}
				headers.Set("X-API-Tunnel-Secret", c.serverConnectSecret)

				onConnectAuth := func(proto, address string) bool { return true }
				onConnect := func(sessionCtx context.Context, session *remotedialer.Session) error {
					logrus.Infoln("ProxyClient: remotedialer session connected!")
					if c.onConnect != nil {
						return c.onConnect(sessionCtx, session)
					}
					return nil
				}

				if err := remotedialer.ClientConnect(ctx, c.serverUrl, headers, c.dialer, onConnectAuth, onConnect); err != nil {
					logrus.Errorf("remotedialer.ClientConnect error: %s", err.Error())
					time.Sleep(retryTimeout)
				}
			}
		}
	}()

	<-ctx.Done()
}

func (c *ProxyClient) Stop() {
	if c.forwarder != nil {
		c.forwarder.Stop()
		logrus.Infoln("ProxyClient: port-forward stopped.")
	}
}

func WithServerURL(serverUrl string) ProxyClientOpt {
	return func(pc *ProxyClient) {
		pc.serverUrl = serverUrl
	}
}

func WithOnConnectCallback(onConnect func(ctx context.Context, session *remotedialer.Session) error) ProxyClientOpt {
	return func(pc *ProxyClient) {
		pc.onConnect = onConnect
	}
}

func WithCustomDialer(dialer *websocket.Dialer) ProxyClientOpt {
	return func(pc *ProxyClient) {
		pc.dialer = dialer
	}
}
