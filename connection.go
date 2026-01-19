package remotedialer

import (
	"context"
	"io"
	"net"
	"sync/atomic"
	"time"

	"github.com/rancher/remotedialer/metrics"
	"github.com/sirupsen/logrus"
)

type connection struct {
	parentCtx     context.Context
	closed        atomic.Bool
	err           error
	writeDeadline time.Time
	backPressure  *backPressure
	buffer        *readBuffer
	addr          addr
	session       *Session
	connID        int64
}

func newConnection(ctx context.Context, connID int64, session *Session, proto, address string) *connection {
	c := &connection{
		parentCtx: ctx,
		addr: addr{
			proto:   proto,
			address: address,
		},
		connID:  connID,
		session: session,
	}
	c.backPressure = newBackPressure(c)
	c.buffer = newReadBuffer(connID, c.backPressure)
	metrics.IncSMTotalAddConnectionsForWS(session.clientKey, proto, address)
	return c
}

func (c *connection) tunnelClose(err error) {
	c.writeErr(err)
	c.doTunnelClose(err)
}

func (c *connection) doTunnelClose(err error) {
	if !c.closed.CompareAndSwap(false, true) {
		return
	}

	metrics.IncSMTotalRemoveConnectionsForWS(c.session.clientKey, c.addr.Network(), c.addr.String())
	c.err = err
	if c.err == nil {
		c.err = io.ErrClosedPipe
	}

	c.buffer.Close(c.err)
}

func (c *connection) OnData(r io.Reader) error {
	if PrintTunnelData {
		defer func() {
			logrus.Debugf("ONDATA  [%d] %s", c.connID, c.buffer.Status())
		}()
	}
	return c.buffer.Offer(r)
}

func (c *connection) Close() error {
	c.session.closeConnection(c.connID, io.EOF)
	c.backPressure.Close()
	return nil
}

func (c *connection) Read(b []byte) (int, error) {
	ctx := c.parentCtx
	if ctx == nil {
		ctx = context.Background()
	}

	if !c.buffer.deadline.IsZero() {
		var cancel context.CancelFunc
		ctx, cancel = context.WithDeadline(ctx, c.buffer.deadline)
		defer cancel()
	}

	n, err := c.buffer.readWithContext(ctx, b)
	metrics.AddSMTotalReceiveBytesOnWS(c.session.clientKey, float64(n))
	if PrintTunnelData {
		logrus.Debugf("READ    [%d] %s %d %v", c.connID, c.buffer.Status(), n, err)
	}
	return n, err
}

func (c *connection) Write(b []byte) (int, error) {
	if c.closed.Load() {
		return 0, io.ErrClosedPipe
	}
	ctx := c.parentCtx
	if ctx == nil {
		ctx = context.Background()
	}
	cancel := func() {}
	if !c.writeDeadline.IsZero() {
		ctx, cancel = context.WithDeadline(ctx, c.writeDeadline)
		go func(ctx context.Context) {
			select {
			case <-ctx.Done():
				if ctx.Err() == context.DeadlineExceeded {
					c.Close()
				}
				return
			}
		}(ctx)
	}

	c.backPressure.Wait(ctx, cancel)
	msg := newMessage(c.connID, b)
	metrics.AddSMTotalTransmitBytesOnWS(c.session.clientKey, float64(len(msg.Bytes())))
	writeCtx := c.parentCtx
	if writeCtx == nil {
		writeCtx = context.Background()
	}
	if !c.writeDeadline.IsZero() {
		var writeCancel context.CancelFunc
		writeCtx, writeCancel = context.WithDeadline(writeCtx, c.writeDeadline)
		defer writeCancel()
	}
	return c.session.writeMessage(writeCtx, msg)
}

func (c *connection) OnPause() {
	c.backPressure.OnPause()
}

func (c *connection) OnResume() {
	c.backPressure.OnResume()
}

func (c *connection) Pause() {
	msg := newPause(c.connID)
	ctx := c.parentCtx
	if ctx == nil {
		ctx = context.Background()
	}
	if !c.writeDeadline.IsZero() {
		var cancel context.CancelFunc
		ctx, cancel = context.WithDeadline(ctx, c.writeDeadline)
		defer cancel()
	}
	_, _ = c.session.writeMessage(ctx, msg)
}

func (c *connection) Resume() {
	msg := newResume(c.connID)
	ctx := c.parentCtx
	if ctx == nil {
		ctx = context.Background()
	}
	if !c.writeDeadline.IsZero() {
		var cancel context.CancelFunc
		ctx, cancel = context.WithDeadline(ctx, c.writeDeadline)
		defer cancel()
	}
	_, _ = c.session.writeMessage(ctx, msg)
}

func (c *connection) writeErr(err error) {
	if err != nil {
		msg := newErrorMessage(c.connID, err)
		metrics.AddSMTotalTransmitErrorBytesOnWS(c.session.clientKey, float64(len(msg.Bytes())))
		deadline := time.Now().Add(SendErrorTimeout)
		ctx := c.parentCtx
		if ctx == nil {
			ctx = context.Background()
		}
		if !deadline.IsZero() {
			var cancel context.CancelFunc
			ctx, cancel = context.WithDeadline(ctx, deadline)
			defer cancel()
		}
		if _, err2 := c.session.writeMessage(ctx, msg); err2 != nil {
			logrus.Warnf("[%d] encountered error %q while writing error %q to close remotedialer", c.connID, err2, err)
		}
	}
}

func (c *connection) LocalAddr() net.Addr {
	return c.addr
}

func (c *connection) RemoteAddr() net.Addr {
	return c.addr
}

func (c *connection) SetDeadline(t time.Time) error {
	if err := c.SetReadDeadline(t); err != nil {
		return err
	}
	return c.SetWriteDeadline(t)
}

func (c *connection) SetReadDeadline(t time.Time) error {
	c.buffer.deadline = t
	return nil
}

func (c *connection) SetWriteDeadline(t time.Time) error {
	c.writeDeadline = t
	return nil
}

type addr struct {
	proto   string
	address string
}

func (a addr) Network() string {
	return a.proto
}

func (a addr) String() string {
	return a.address
}
