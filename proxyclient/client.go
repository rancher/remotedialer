package proxyclient

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/rancher/remotedialer"
	"github.com/rancher/wrangler/v3/pkg/generated/controllers/core"
	v1 "github.com/rancher/wrangler/v3/pkg/generated/controllers/core/v1"

	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
)

const (
	defaultServerAddr        = "wss://127.0.0.1"
	defaultServerPort        = 5555
	defaultServerPath        = "/connect"
	retryTimeout             = 1 * time.Second
	certificateWatchInterval = 10 * time.Second
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

	dialer    *websocket.Dialer
	dialerMtx sync.Mutex

	secretController v1.SecretController
	namespace        string
	certSecretName   string
	certServerName   string

	onConnect func(ctx context.Context, session *remotedialer.Session) error
}

func New(ctx context.Context, serverSharedSecret, namespace, certSecretName, certServerName string, restConfig *rest.Config, forwarder PortForwarder, opts ...ProxyClientOpt) (*ProxyClient, error) {
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

	client := &ProxyClient{
		serverUrl:           serverUrl,
		forwarder:           forwarder,
		serverConnectSecret: serverSharedSecret,
		certSecretName:      certSecretName,
		certServerName:      certServerName,
		namespace:           namespace,
	}

	if err := client.buildDialer(ctx, restConfig); err != nil {
		return nil, fmt.Errorf("dialer build failed %w: ", err)
	}

	for _, opt := range opts {
		opt(client)
	}

	return client, nil
}

func (c *ProxyClient) buildDialer(ctx context.Context, restConfig *rest.Config) error {
	core, err := core.NewFactoryFromConfigWithOptions(restConfig, nil)
	if err != nil {
		return fmt.Errorf("build secret controller failed: %w", err)
	}

	secretController := core.Core().V1().Secret()
	secretController.Informer().AddEventHandler(
		cache.ResourceEventHandlerFuncs{
			UpdateFunc: func(oldSecret, newSecret interface{}) {
				updatedSecretCert, ok := newSecret.(*corev1.Secret)
				if ok {
					if updatedSecretCert.Name == c.certSecretName {
						rootCAs, err := buildCertFromSecret(c.namespace, c.certSecretName, updatedSecretCert)
						if err != nil {
							logrus.Errorf("build certificate failed: %s", err.Error())
							return
						}

						c.dialerMtx.Lock()
						c.dialer = &websocket.Dialer{
							TLSClientConfig: &tls.Config{
								RootCAs:    rootCAs,
								ServerName: c.certServerName,
							},
						}
						c.dialerMtx.Unlock()
						logrus.Infof("certificate updated successfully")
					}
				}
			},
		})

	if err := core.Start(ctx, 1); err != nil {
		return fmt.Errorf("secret controller factory start failed: %w", err)
	}

	secret, err := secretController.Get(c.namespace, c.certSecretName, metav1.GetOptions{})
	if err != nil {
		return err
	}

	rootCAs, err := buildCertFromSecret(c.namespace, c.certSecretName, secret)
	if err != nil {
		return fmt.Errorf("build certificate failed: %w", err)
	}

	c.dialerMtx.Lock()
	c.dialer = &websocket.Dialer{
		TLSClientConfig: &tls.Config{
			RootCAs:    rootCAs,
			ServerName: c.certServerName,
		},
	}
	c.dialerMtx.Unlock()

	return nil
}

func buildCertFromSecret(namespace, certSecretName string, secret *corev1.Secret) (*x509.CertPool, error) {
	crtData, exists := secret.Data["tls.crt"]
	if !exists {
		return nil, fmt.Errorf("secret %s/%s missing tls.crt field", namespace, certSecretName)
	}

	rootCAs := x509.NewCertPool()
	if ok := rootCAs.AppendCertsFromPEM(crtData); !ok {
		return nil, fmt.Errorf("failed to parse tls.crt from secret into a CA pool")
	}

	return rootCAs, nil
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

				c.dialerMtx.Lock()
				dialer := c.dialer
				c.dialerMtx.Unlock()

				if err := remotedialer.ClientConnect(ctx, c.serverUrl, headers, dialer, onConnectAuth, onConnect); err != nil {
					logrus.Errorf("remotedialer.ClientConnect error: %s", err.Error())
					c.forwarder.Stop()
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
