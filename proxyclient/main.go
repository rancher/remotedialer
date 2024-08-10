package main

import (
	"context"
	"crypto/tls"
	"errors"
	"flag"
	"io/ioutil"
	"math/rand"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/rancher/remotedialer"
	"github.com/sirupsen/logrus"
)

var (
	addr    string
	id      string
	example string
	debug   bool
	session *remotedialer.Session
	lock    sync.Mutex
)

const HandshakeTimeOut = 10 * time.Second

func getClientDialer(ctx context.Context, clientKey string) remotedialer.Dialer {
	var s *remotedialer.Session
	start := time.Now()
	for {
		lock.Lock()
		s = session
		lock.Unlock()
		if s != nil {
			break
		}
		select {
		case <-ctx.Done():
			logrus.Errorf("get session failed, cost %.3fs", time.Since(start).Seconds())
			return nil
		case <-time.After(1 * time.Second):
			logrus.Infof("waiting fo session ready... ")
		}
	}
	return remotedialer.ToDialer(s, clientKey)
}

type DialContextFunc func(ctx context.Context, network, address string) (net.Conn, error)

func DialContext(clientKey string) DialContextFunc {
	return func(ctx context.Context, network, addr string) (net.Conn, error) {
		logrus.Debugf("use client dialer, key:%s", clientKey)
		f := getClientDialer(ctx, clientKey)
		if f == nil {
			return nil, errors.New("get client dialer failed")
		}
		return f(ctx, network, addr)
	}
}

func main() {
	flag.StringVar(&addr, "connect", "ws://localhost:8123/connect", "Address to connect to")
	flag.StringVar(&example, "example", "localhost:8124", "example server listen address")
	flag.StringVar(&id, "id", "foo", "Client ID")
	flag.BoolVar(&debug, "debug", true, "Debug logging")
	flag.Parse()

	if debug {
		logrus.SetLevel(logrus.DebugLevel)
		remotedialer.PrintTunnelData = true
	}

	headers := http.Header{
		"X-Tunnel-ID":        []string{"proxy"},
		"X-API-Tunnel-Proxy": []string{"on"},
	}
	dialer := &websocket.Dialer{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: true,
		},
		HandshakeTimeout: HandshakeTimeOut,
	}
	go func() {
		for {
			ws, _, err := dialer.Dial(addr, headers)
			if err != nil {
				logrus.Errorf("Failed to connect to proxy server %s, err: %v", addr, err)
				time.Sleep(time.Duration(rand.Int()%10) * time.Second)
				continue
			}
			lock.Lock()
			session = remotedialer.NewClientSession(func(string, string) bool { return true }, ws)
			lock.Unlock()
			_, err = session.Serve(context.Background())
			if err != nil {
				logrus.Errorf("Failed to serve proxy connection err: %v", err)
			}
			session.Close()
			lock.Lock()
			session = nil
			lock.Unlock()
			ws.Close()
			// retry connect after sleep a random time
			time.Sleep(time.Duration(rand.Int()%10) * time.Second)
		}
	}()
	hc := http.Client{
		Transport: &http.Transport{
			DialContext: DialContext(id),
		},
	}
	req, err := http.NewRequest("GET", "http://"+example+"/hello", nil)
	if err != nil {
		panic(err)
	}
	for {
		time.Sleep(time.Duration(1 * time.Second))
		resp, err := hc.Do(req)
		if err != nil {
			logrus.Errorf("request example server failed, err:%v", err)
			continue
		}
		defer resp.Body.Close()
		respBody, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			logrus.Errorf("read body failed, err:%v", err)
			continue
		}
		logrus.Infof("status:%d respBody:%s", resp.StatusCode, respBody)
	}

}
