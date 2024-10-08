package main

import (
	"github.com/sirupsen/logrus"

	"github.com/rancher/remotedialer/proxy"
)

func main() {
	cfg, err := proxy.ConfigFromEnvironment()
	if err != nil {
		logrus.Fatal(err)
	}
	err = proxy.Start(cfg)
	if err != nil {
		logrus.Fatal(err)
	}
}
