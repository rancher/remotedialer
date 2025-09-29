package remotedialer

import (
	"context"
	"errors"
	"io/ioutil"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
	"github.com/sirupsen/logrus"
)

// ConnectAuthorizer custom for authorization
type ConnectAuthorizer func(proto, address string) bool

// ClientConnect connect to WS and wait 5 seconds when error
func ClientConnect(ctx context.Context, wsURL string, headers http.Header, dialer *websocket.Dialer,
	auth ConnectAuthorizer, onConnect func(context.Context, *Session) error) error {
	logrus.Debugf("ClientConnect called with wsURL: %s", wsURL)
	if err := ConnectToProxy(ctx, wsURL, headers, auth, dialer, onConnect); err != nil {
		if !errors.Is(err, context.Canceled) {
			logrus.WithError(err).Error("Remotedialer proxy error")
			time.Sleep(time.Duration(5) * time.Second)
		}
		return err
	}
	logrus.Debug("ClientConnect succeeded")
	return nil
}

// ConnectToProxy connects to the websocket server.
// Local connections on behalf of the remote host will be dialed using a default net.Dialer.
func ConnectToProxy(rootCtx context.Context, proxyURL string, headers http.Header, auth ConnectAuthorizer, dialer *websocket.Dialer, onConnect func(context.Context, *Session) error) error {
	logrus.Debugf("ConnectToProxy called with proxyURL: %s", proxyURL)
	return ConnectToProxyWithDialer(rootCtx, proxyURL, headers, auth, dialer, nil, onConnect)
}

// ConnectToProxyWithDialer connects to the websocket server.
// Local connections on behalf of the remote host will be dialed using the provided Dialer function.
func ConnectToProxyWithDialer(rootCtx context.Context, proxyURL string, headers http.Header, auth ConnectAuthorizer, dialer *websocket.Dialer, localDialer Dialer, onConnect func(context.Context, *Session) error) error {
	logrus.WithField("url", proxyURL).Info("Connecting to proxy")
	logrus.Debugf("ConnectToProxyWithDialer called with proxyURL: %s, headers: %v", proxyURL, headers)

	if dialer == nil {
		logrus.Debug("Dialer is nil, creating default websocket.Dialer")
		dialer = &websocket.Dialer{Proxy: http.ProxyFromEnvironment, HandshakeTimeout: HandshakeTimeOut}
	}
	ws, resp, err := dialer.DialContext(rootCtx, proxyURL, headers)
	if err != nil {
		logrus.Debugf("DialContext failed for proxyURL: %s", proxyURL)
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
		return err
	}
	defer func() {
		logrus.Debug("Closing websocket connection")
		ws.Close()
	}()

	result := make(chan error, 2)

	ctx, cancel := context.WithCancel(rootCtx)
	defer func() {
		logrus.Debug("Cancelling context in ConnectToProxyWithDialer")
		cancel()
	}()

	session := NewClientSessionWithDialer(auth, ws, localDialer)
	defer func() {
		logrus.Debug("Closing session in ConnectToProxyWithDialer")
		session.Close()
	}()

	if onConnect != nil {
		logrus.Debug("Starting onConnect goroutine")
		go func() {
			if err := onConnect(ctx, session); err != nil {
				logrus.Debugf("onConnect returned error: %v", err)
				result <- err
			}
		}()
	}

	logrus.Debug("Starting session.Serve goroutine")
	go func() {
		_, err = session.Serve(ctx)
		logrus.Debugf("session.Serve returned error: %v", err)
		result <- err
	}()

	select {
	case <-ctx.Done():
		logrus.WithField("url", proxyURL).WithField("err", ctx.Err()).Info("Proxy done")
		logrus.Debug("Context done in ConnectToProxyWithDialer")
		return nil
	case err := <-result:
		logrus.Debugf("Received error from result channel: %v", err)
		return err
	}
}
