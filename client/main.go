package main

import (
	"context"
	"flag"
	"net/http"
	"os"

	"github.com/loft-sh/remotedialer"
	"k8s.io/klog/v2"
)

var (
	addr  string
	id    string
	debug bool
)

func main() {
	flag.StringVar(&addr, "connect", "ws://localhost:8123/connect", "Address to connect to")
	flag.StringVar(&id, "id", "foo", "Client ID")
	flag.BoolVar(&debug, "debug", true, "Debug logging")
	flag.Parse()

	if debug {
		klogFlagSet := &flag.FlagSet{}
		klog.InitFlags(klogFlagSet)
		if err := klogFlagSet.Set("v", "10"); err != nil {
			klog.TODO().Error(err, "failed to set klog verbosity level")
			os.Exit(1)
		}
		if err := klogFlagSet.Parse([]string{}); err != nil {
			klog.TODO().Error(err, "failed to parse klog flags")
			os.Exit(1)
		}
	}

	headers := http.Header{
		"X-Tunnel-ID": []string{id},
	}

	ctx := context.Background()

	err := remotedialer.ClientConnect(ctx, addr, headers, nil, func(string, string) bool { return true }, nil)

	if err != nil {
		klog.FromContext(ctx).Error(err, "Failed to connect to proxy")
	}
}
