package portforward

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

	"github.com/sirupsen/logrus"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	corev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/portforward"
	"k8s.io/client-go/transport/spdy"
)

type PortForward struct {
	restConfig    *rest.Config
	podClient     corev1.PodInterface
	Namespace     string
	LabelSelector string
	Ports         []string
	readyCh       chan struct{}
	stopCh        chan struct{}
	cancel        context.CancelFunc
}

// New initializes and returns a PortForward value after validating the incoming parameters.
func New(restConfig *rest.Config, podClient corev1.PodInterface, namespace string, labelSelector string, ports []string) (*PortForward, error) {
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
	return &PortForward{
		restConfig:    restConfig,
		podClient:     podClient,
		Namespace:     namespace,
		LabelSelector: labelSelector,
		Ports:         ports,
		readyCh:       make(chan struct{}, 1),
		stopCh:        make(chan struct{}, 1),
	}, nil
}

// Stop releases the resources of the running port-forwarder.
func (r *PortForward) Stop() {
	r.cancel()
	r.stopCh <- struct{}{}
}

// Start launches a port forwarder in the background.
func (r *PortForward) Start() {
	ctx, cancel := context.WithCancel(context.Background())
	r.cancel = cancel

	go func() {
		for {
			select {
			case <-ctx.Done():
				logrus.Infoln("Goroutine stopped.")
				return
			default:
				r.readyCh = make(chan struct{}, 1)
				err := r.runForwarder(r.readyCh, r.stopCh, r.Ports)
				if err != nil {
					if errors.Is(err, portforward.ErrLostConnectionToPod) {
						logrus.Error("Lost connection to pod; restarting.")
						time.Sleep(time.Second)
						continue
					} else {
						logrus.Errorf("Non-restartable error: %v", err)
						return
					}
				}
			}
		}
	}()
	// TODO: maybe block until port forwarding is ready.
}

// runForwarder starts a port forwarder and blocks until it is stopped when it receives a value on the stopCh.
func (r *PortForward) runForwarder(readyCh, stopCh chan struct{}, ports []string) error {
	podName, err := r.podName()
	if err != nil {
		return err
	}
	path := fmt.Sprintf("/api/v1/namespaces/%s/pods/%s/portforward", r.Namespace, podName)
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

	go func() {
		for range readyCh {
		} // Wait until port forwarding is ready.

		if s := stderr.String(); s != "" {
			logrus.Error(s)
		} else if s = stdout.String(); s != "" {
			logrus.Info(s)
		}
	}()

	return forwarder.ForwardPorts()
}

// podName tries to select a random pod and return its name from the pods that the
// underlying client finds by label selector.
// The method continuously retries if there are no pods yet that match the selector.
func (r *PortForward) podName() (string, error) {
	for {
		pods, err := r.podClient.List(context.Background(), metav1.ListOptions{
			LabelSelector: r.LabelSelector,
		})
		if err != nil {
			return "", err
		}
		if len(pods.Items) < 1 {
			logrus.Debugf("no pod found with label selector %q, retrying", r.LabelSelector)
			time.Sleep(time.Second)
			continue
		}
		i := rand.Intn(len(pods.Items))
		return pods.Items[i].Name, nil
	}
}
