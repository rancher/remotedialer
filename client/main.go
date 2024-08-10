package main

import (
	"context"
	"flag"
	"io"
	"net/http"

	"github.com/rancher/remotedialer"
	"github.com/sirupsen/logrus"
)

var (
	addr    string
	id      string
	example string
	debug   bool
)

func main() {
	flag.StringVar(&addr, "connect", "ws://localhost:8123/connect", "Address to connect to")
	flag.StringVar(&example, "example", "localhost:8124", "example server listen address")
	flag.StringVar(&id, "id", "foo", "Client ID")
	flag.BoolVar(&debug, "debug", true, "Debug logging")
	flag.Parse()

	if debug {
		logrus.SetLevel(logrus.DebugLevel)
	}

	headers := http.Header{
		"X-Tunnel-ID": []string{id},
	}
	helloHandler := func(w http.ResponseWriter, req *http.Request) {
		io.WriteString(w, "Hello, world!\n")
	}
	http.HandleFunc("/hello", helloHandler)
	go http.ListenAndServe(example, nil)

	remotedialer.ClientConnect(context.Background(), addr, headers, nil, func(string, string) bool { return true }, nil)
}
