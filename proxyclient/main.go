package main

import (
	"context"
	"crypto/tls"
	"database/sql"
	"flag"
	"math/rand"
	"net"
	"net/http"
	"time"

	"github.com/go-sql-driver/mysql"
	"github.com/gorilla/websocket"
	"github.com/rancher/remotedialer"
	"github.com/sirupsen/logrus"
)

var (
	addr    string
	id      string
	debug   bool
	session *remotedialer.Session
)

const HandshakeTimeOut = 10 * time.Second

func getDialer(id string) remotedialer.Dialer {
	latestSesion := session
	return remotedialer.ToDialer(latestSesion, id)
}

func getSqlDialer(id string) mysql.DialContextFunc {
	return func(ctx context.Context, addr string) (net.Conn, error) {
		logrus.Debug("use remote sql dialer")
		return getDialer(id)(ctx, "tcp", addr)
	}
}

var ready = make(chan int)

func init() {
	flag.StringVar(&addr, "connect", "ws://localhost:8123/connect", "Address to connect to")
	flag.StringVar(&id, "id", "foo", "Client ID")
	flag.BoolVar(&debug, "debug", true, "Debug logging")
	flag.Parse()

	if debug {
		logrus.SetLevel(logrus.DebugLevel)
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
				logrus.Errorf("Failed to connect to proxy server %s [local ID=%s]: %v", addr, id, err)
				time.Sleep(time.Duration(rand.Int()%10) * time.Second)
				continue
			}
			session = remotedialer.NewClientSession(func(string, string) bool { return true }, ws)
			ready <- 1
			_, err = session.Serve(context.Background())
			if err != nil {
				logrus.Errorf("Failed to serve proxy connection %s: %v", id, err)
			}
			session.Close()
			ws.Close()
			// retry connect after sleep a random time
			time.Sleep(time.Duration(rand.Int()%10) * time.Second)
		}
	}()
}

func main() {
	<-ready
	mysql.RegisterDialContext("tcp", getSqlDialer(id))
	db, err := sql.Open("mysql", "root@tcp(127.0.0.1:3306)/")
	if err != nil {
		logrus.Error(err)
		return
	}
	defer db.Close()
	row := db.QueryRow("select version()")
	var version string
	row.Scan(&version)
	logrus.Infof("sql version is:%v", version)
	ch := make(chan int)
	<-ch
}
