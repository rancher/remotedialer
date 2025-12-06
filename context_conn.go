package remotedialer

import (
	"context"
	"net"
	"time"
)

// contextConn wraps net.Conn to make it context-aware
type contextConn struct {
	net.Conn
	ctx context.Context
}

func newContextConn(ctx context.Context, conn net.Conn) *contextConn {
	return &contextConn{
		Conn: conn,
		ctx:  ctx,
	}
}

func (c *contextConn) Read(b []byte) (int, error) {
	stopc := make(chan struct{})
	stop := context.AfterFunc(c.ctx, func() {
		c.Conn.SetReadDeadline(time.Now())
		close(stopc)
	})

	n, err := c.Conn.Read(b)

	if !stop() {
		<-stopc
		c.Conn.SetReadDeadline(time.Time{})
		return n, c.ctx.Err()
	}

	return n, err
}

func (c *contextConn) Write(b []byte) (int, error) {
	stopc := make(chan struct{})
	stop := context.AfterFunc(c.ctx, func() {
		c.Conn.SetWriteDeadline(time.Now())
		close(stopc)
	})

	n, err := c.Conn.Write(b)

	if !stop() {
		<-stopc
		c.Conn.SetWriteDeadline(time.Time{})
		return n, c.ctx.Err()
	}

	return n, err
}
