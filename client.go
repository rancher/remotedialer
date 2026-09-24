package remotedialer

import (
	"context"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
	"github.com/sirupsen/logrus"
)

// ConnectAuthorizer custom for authorization
type ConnectAuthorizer func(proto, address string) bool

// ConnectOpts holds optional settings for a client connection.
type ConnectOpts struct {
	// Backoff controls the wait between attempts. Zero means fixed DefaultRetryMin.
	Backoff Backoff
}

// dialError marks a failed handshake rather than a dropped tunnel.
type dialError struct{ error }

// Unwrap keeps errors.Is and errors.As working through the wrapper.
func (e dialError) Unwrap() error { return e.error }

// ClientConnect connect to WS and wait 5 seconds when error
func ClientConnect(ctx context.Context, wsURL string, headers http.Header, dialer *websocket.Dialer,
	auth ConnectAuthorizer, onConnect func(context.Context, *Session) error) error {
	if err := ConnectToProxy(ctx, wsURL, headers, auth, dialer, onConnect); err != nil {
		if !errors.Is(err, context.Canceled) {
			logrus.WithError(err).Error("Remotedialer proxy error")
			time.Sleep(time.Duration(5) * time.Second)
		}
		return err
	}
	return nil
}

// ClientConnectWithOpts reconnects until ctx is cancelled, backing off between attempts.
func ClientConnectWithOpts(ctx context.Context, wsURL string, headers http.Header, dialer *websocket.Dialer,
	auth ConnectAuthorizer, onConnect func(context.Context, *Session) error, opts *ConnectOpts) error {
	var backoff Backoff
	if opts != nil {
		backoff = opts.Backoff
	}

	for attempt := 0; ; attempt++ {
		err := ConnectToProxy(ctx, wsURL, headers, auth, dialer, onConnect)
		if ctx.Err() != nil {
			return ctx.Err()
		}

		// A non-dial error means the tunnel was up.
		var de dialError
		if !errors.As(err, &de) {
			attempt = 0
		}

		d := backoff.delay(attempt)
		logrus.WithError(err).Errorf("Remotedialer proxy error, reconnecting to %s in %s", wsURL, d)
		if err := sleep(ctx, d); err != nil {
			return err
		}
	}
}

// ConnectToProxy connects to the websocket server.
// Local connections on behalf of the remote host will be dialed using a default net.Dialer.
func ConnectToProxy(rootCtx context.Context, proxyURL string, headers http.Header, auth ConnectAuthorizer, dialer *websocket.Dialer, onConnect func(context.Context, *Session) error) error {
	return ConnectToProxyWithDialer(rootCtx, proxyURL, headers, auth, dialer, nil, onConnect)
}

// ConnectToProxyWithDialer connects to the websocket server.
// Local connections on behalf of the remote host will be dialed using the provided Dialer function.
func ConnectToProxyWithDialer(rootCtx context.Context, proxyURL string, headers http.Header, auth ConnectAuthorizer, dialer *websocket.Dialer, localDialer Dialer, onConnect func(context.Context, *Session) error) error {
	logrus.WithField("url", proxyURL).Info("Connecting to proxy")

	if dialer == nil {
		dialer = &websocket.Dialer{Proxy: http.ProxyFromEnvironment, HandshakeTimeout: HandshakeTimeOut}
	}
	ws, resp, err := dialer.DialContext(rootCtx, proxyURL, headers)
	if err != nil {
		if resp == nil {
			if !errors.Is(err, context.Canceled) {
				logrus.WithError(err).Errorf("Failed to connect to proxy. Empty dialer response")
			}
		} else {
			rb, err2 := ioutil.ReadAll(resp.Body)
			if err2 != nil {
				logrus.WithError(err).Errorf("Failed to connect to proxy. Response status: %v - %v. Couldn't read response body (err: %v)", resp.StatusCode, resp.Status, err2)
			} else {
				logrus.WithError(err).Errorf("Failed to connect to proxy. Response status: %v - %v. Response body: %s", resp.StatusCode, resp.Status, rb)
			}
		}
		return dialError{err}
	}
	defer ws.Close()

	result := make(chan error, 2)

	ctx, cancel := context.WithCancel(rootCtx)
	defer cancel()
	ctx = context.WithValue(ctx, ContextKeyCaller, fmt.Sprintf("ConnectToProxy: url: %s", proxyURL))

	session := NewClientSessionWithDialer(auth, ws, localDialer)
	defer session.Close()

	if onConnect != nil {
		go func() {
			if err := onConnect(ctx, session); err != nil {
				result <- err
			}
		}()
	}

	go func() {
		_, err = session.Serve(ctx)
		result <- err
	}()

	logrus.WithField("url", proxyURL).Info("Connected to proxy")

	select {
	case <-ctx.Done():
		logrus.WithField("url", proxyURL).WithField("err", ctx.Err()).Info("Proxy done")
		return nil
	case err := <-result:
		return err
	}
}
