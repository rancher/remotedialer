package main

import (
	"bytes"
	"context"
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
	stopCh        chan struct{}
	started       bool
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
		stopCh:        make(chan struct{}, 1),
	}, nil
}

// Stop releases the resources of the running port-forwarder.
// If the port forwarder is not already running, this method is a no-op.
func (r *PortForward) Stop() {
	if r.started {
		r.started = false
		r.stopCh <- struct{}{}
	}
}

// Start waits for a port forwarder to start in the background.
// It returns an error in case the forwarder failed to start.
func (r *PortForward) Start() error {
	if r.started {
		return fmt.Errorf("port-forward is already running")
	}
	errCh := make(chan error, 1)
	readyCh := make(chan struct{}, 1)

	go r.runForwarder(readyCh, r.stopCh, errCh, r.Ports)
	select {
	case err := <-errCh:
		return fmt.Errorf("error setting up port forwarding: %w", err)
	case <-readyCh:
		logrus.Info("port forwarding ready")
		r.started = true
		return nil
	}
}

// runForwarder starts a port forwarder and blocks until it is stopped when it receives a value on the stopCh.
// This method is meant to be called asynchronously.
func (r *PortForward) runForwarder(readyCh chan struct{}, stopCh chan struct{}, errCh chan error, ports []string) {
	podName, err := r.podName()
	if err != nil {
		errCh <- err
		return
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
		errCh <- err
		return
	}
	dialer := spdy.NewDialer(upgrader, &http.Client{
		Transport: roundTripper,
	}, http.MethodPost, &serverURL)

	stdout, stderr := new(bytes.Buffer), new(bytes.Buffer)
	forwarder, err := portforward.New(dialer, ports, stopCh, readyCh, stdout, stderr)
	if err != nil {
		errCh <- err
		return
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

	if err := forwarder.ForwardPorts(); err != nil {
		errCh <- err
		return
	}
	fmt.Println("forwarder done")
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
