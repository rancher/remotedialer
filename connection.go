package remotedialer

import (
	"context"
	"errors"
	"io"
	"net"
	"sync"
	"time"

	"github.com/rancher/remotedialer/metrics"
	"github.com/sirupsen/logrus"
)

type connection struct {
	lock          sync.RWMutex
	err           error
	writeDeadline time.Time
	backPressure  *backPressure
	buffer        *readBuffer
	addr          addr
	session       *Session
	connID        int64
}

func newConnection(connID int64, session *Session, proto, address string) *connection {
	c := &connection{
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
	err, didSet := c.setErrIfNotSet(err)
	if !didSet {
		return
	}

	metrics.IncSMTotalRemoveConnectionsForWS(c.session.clientKey, c.addr.Network(), c.addr.String())
	c.buffer.Close(err)
}

// setErrIfNotSet sets the error if it is not already set.
// Returns true if the error was set, false if it was already set.
// Regardless, the error is returned.
func (c *connection) setErrIfNotSet(err error) (error, bool) {
	if err == nil {
		err = io.ErrClosedPipe
	}

	c.lock.Lock()
	defer c.lock.Unlock()
	if c.err == nil {
		c.err = err
		return c.err, true
	}
	return c.err, false
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
	n, err := c.buffer.Read(b)
	metrics.AddSMTotalReceiveBytesOnWS(c.session.clientKey, float64(n))
	if PrintTunnelData {
		logrus.Debugf("READ    [%d] %s %d %v", c.connID, c.buffer.Status(), n, err)
	}
	return n, err
}

func (c *connection) Write(b []byte) (int, error) {
	c.lock.RLock()
	err := c.err
	writeDeadline := c.writeDeadline
	c.lock.RUnlock()

	if err != nil {
		return 0, errors.Join(err, io.ErrClosedPipe)
	}

	ctx, cancel := context.WithCancel(context.Background())
	if !writeDeadline.IsZero() {
		ctx, cancel = context.WithDeadline(ctx, writeDeadline)
		go func(ctx context.Context) {
			<-ctx.Done()
			if ctx.Err() == context.DeadlineExceeded {
				c.Close()
			}
		}(ctx)
	}

	c.backPressure.Wait(cancel)
	msg := newMessage(c.connID, b)
	metrics.AddSMTotalTransmitBytesOnWS(c.session.clientKey, float64(len(msg.Bytes())))
	return c.session.writeMessage(writeDeadline, msg)
}

func (c *connection) OnPause() {
	c.backPressure.OnPause()
}

func (c *connection) OnResume() {
	c.backPressure.OnResume()
}

func (c *connection) Pause() {
	msg := newPause(c.connID)
	_, _ = c.session.writeMessage(c.getWriteDeadline(), msg)
}

func (c *connection) Resume() {
	msg := newResume(c.connID)
	_, _ = c.session.writeMessage(c.getWriteDeadline(), msg)
}

func (c *connection) writeErr(err error) {
	if err != nil {
		msg := newErrorMessage(c.connID, err)
		metrics.AddSMTotalTransmitErrorBytesOnWS(c.session.clientKey, float64(len(msg.Bytes())))
		deadline := time.Now().Add(SendErrorTimeout)
		if _, err2 := c.session.writeMessage(deadline, msg); err2 != nil {
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
	c.lock.Lock()
	defer c.lock.Unlock()
	c.writeDeadline = t
	return nil
}

func (c *connection) getWriteDeadline() time.Time {
	c.lock.RLock()
	defer c.lock.RUnlock()
	return c.writeDeadline
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
