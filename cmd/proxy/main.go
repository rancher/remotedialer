package main

import (
	"github.com/sirupsen/logrus"
	"k8s.io/client-go/rest"

	"github.com/rancher/lasso/pkg/log"
	"github.com/rancher/remotedialer/proxy"
)

func main() {
	logrus.Info("Starting Remote Dialer Proxy")

	cfg, err := proxy.ConfigFromEnvironment()
	if err != nil {
		logrus.Fatalf("fatal configuration error: %v", err)
	}

	// Initializing Wrangler
	restConfig, err := rest.InClusterConfig()
	if err != nil {
		log.Errorf("failed to get in-cluster config: %w", err)
		return
	}

	err = proxy.Start(cfg, restConfig)
	if err != nil {
		logrus.Fatal(err)
	}
}
