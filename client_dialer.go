package remotedialer

import (
	"context"
	"io"
	"net"
	"sync"
	"time"
)

func clientDial(ctx context.Context, dialer Dialer, message *message) {
	defer message.conn.Close()
	var (
		netConn net.Conn
		err     error
	)

	ctx, cancel := context.WithDeadline(ctx, time.Now().Add(time.Minute))
	if dialer == nil {
		d := net.Dialer{}
		netConn, err = d.DialContext(ctx, message.proto, message.address)
	} else {
		netConn, err = dialer(ctx, message.proto, message.address)
	}
	cancel()
	if err != nil {
		return
	}

	defer netConn.Close()
	pipe(netConn, message.conn)
}

func pipe(client net.Conn, server net.Conn) {
	wg := sync.WaitGroup{}
	wg.Add(1)

	closeOnError := func(err error) error {
		if err == nil {
			err = io.EOF
		}
		client.Close()
		server.Close()
		return err
	}

	go func() {
		defer wg.Done()
		_, err := io.Copy(server, client)
		_ = closeOnError(err)
	}()

	_, err := io.Copy(client, server)
	_ = closeOnError(err)
	wg.Wait()
}
