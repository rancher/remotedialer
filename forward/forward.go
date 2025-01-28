package forward

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"math/rand"
	"net/http"
	"net/url"
	"strings"
	"time"

	v1 "github.com/rancher/wrangler/v3/pkg/generated/controllers/core/v1"
	"github.com/sirupsen/logrus"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/portforward"
	"k8s.io/client-go/transport/spdy"
)

type PortForward struct {
	restConfig    *rest.Config
	podClient     v1.PodController
	namespace     string
	labelSelector string
	Ports         []string

	readyCh chan struct{}
	stopCh  chan struct{}
	cancel  context.CancelFunc
}

func New(restConfig *rest.Config, podClient v1.PodController, namespace string, labelSelector string, ports []string) (*PortForward, error) {
	if restConfig == nil {
		return nil, fmt.Errorf("restConfig must not be nil")
	}
	if podClient == nil {
		return nil, fmt.Errorf("podClient must not be nil")
	}
	if labelSelector == "" {
		return nil, fmt.Errorf("labelSelector must not be empty")
	}
	if len(ports) == 0 {
		return nil, fmt.Errorf("ports must not be empty")
	}
	if namespace == "" {
		return nil, fmt.Errorf("namespace must not be empty")
	}

	for _, p := range ports {
		if strings.HasPrefix(p, "0:") {
			return nil, fmt.Errorf("cannot bind port zero", p)
		}
	}

	return &PortForward{
		restConfig:    restConfig,
		podClient:     podClient,
		namespace:     namespace,
		labelSelector: labelSelector,
		Ports:         ports,
		readyCh:       make(chan struct{}, 1),
		stopCh:        make(chan struct{}, 1),
	}, nil
}

func (r *PortForward) Stop() {
	r.cancel()
	r.stopCh <- struct{}{}
}

func (r *PortForward) Start() error {
	var failed bool
	var err error

	r.readyCh = make(chan struct{}, 1)

	ctx, cancel := context.WithCancel(context.Background())
	r.cancel = cancel

	go func() {
		for {
			select {
			case <-ctx.Done():
				logrus.Infoln("Goroutine stopped.")
				return
			default:
				err = r.runForwarder(ctx, r.readyCh, r.stopCh, r.Ports)
				if err != nil {
					if errors.Is(err, portforward.ErrLostConnectionToPod) {
						logrus.Errorf("Lost connection to pod (no automatic retry in this refactor): %v", err)
					} else {
						logrus.Errorf("Non-restartable error: %v", err)
						failed = true
						r.readyCh <- struct{}{}
						return
					}
				}
			}
		}
	}()

	// wait for the port forward to be ready if not failed
	<-r.readyCh

	if failed {
		return err
	}

	return nil
}

func (r *PortForward) runForwarder(ctx context.Context, readyCh, stopCh chan struct{}, ports []string) error {
	podName, err := findPodName(ctx, r.namespace, r.labelSelector, r.podClient)
	if err != nil {
		return err
	}
	logrus.Infof("Selected pod %q for label %q", podName, r.labelSelector)

	path := fmt.Sprintf("/api/v1/namespaces/%s/pods/%s/portforward", r.namespace, podName)
	hostIP := strings.TrimPrefix(r.restConfig.Host, "https://")
	serverURL := url.URL{
		Scheme: "https",
		Path:   path,
		Host:   hostIP,
	}

	roundTripper, upgrader, err := spdy.RoundTripperFor(r.restConfig)
	if err != nil {
		return err
	}
	dialer := spdy.NewDialer(upgrader, &http.Client{
		Transport: roundTripper,
	}, http.MethodPost, &serverURL)

	stdout, stderr := new(bytes.Buffer), new(bytes.Buffer)
	forwarder, err := portforward.New(dialer, ports, stopCh, readyCh, stdout, stderr)
	if err != nil {
		return err
	}

	return forwarder.ForwardPorts()
}

func findPodName(ctx context.Context, namespace, labelSelector string, podClient v1.PodClient) (string, error) {
	for {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		default:
			pods, err := podClient.List(namespace, metav1.ListOptions{
				LabelSelector: labelSelector,
			})
			if err != nil {
				return "", err
			}
			if len(pods.Items) < 1 {
				logrus.Debugf("no pod found with label selector %q, retrying in 1s", labelSelector)
				time.Sleep(time.Second)
				continue
			}
			i := rand.Intn(len(pods.Items))
			return pods.Items[i].Name, nil
		}
	}
}
