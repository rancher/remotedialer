package remotedialer

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"time"

	"github.com/coder/websocket"
	"github.com/sirupsen/logrus"
)

// ConnectAuthorizer custom for authorization
type ConnectAuthorizer func(proto, address string) bool

// ClientConnect connect to WS and wait 5 seconds when error
func ClientConnect(ctx context.Context, wsURL string, headers http.Header, dialOpts *websocket.DialOptions,
	auth ConnectAuthorizer, onConnect func(context.Context, *Session) error) error {
	if err := ConnectToProxy(ctx, wsURL, headers, auth, dialOpts, onConnect); err != nil {
		if !errors.Is(err, context.Canceled) {
			logrus.WithError(err).Error("Remotedialer proxy error")
			time.Sleep(time.Duration(5) * time.Second)
		}
		return err
	}
	return nil
}

// ConnectToProxy connects to the websocket server.
// Local connections on behalf of the remote host will be dialed using a default net.Dialer.
func ConnectToProxy(rootCtx context.Context, proxyURL string, headers http.Header, auth ConnectAuthorizer, dialOpts *websocket.DialOptions, onConnect func(context.Context, *Session) error) error {
	return ConnectToProxyWithDialer(rootCtx, proxyURL, headers, auth, dialOpts, nil, onConnect)
}

// ConnectToProxyWithDialer connects to the websocket server.
// Local connections on behalf of the remote host will be dialed using the provided Dialer function.
func ConnectToProxyWithDialer(rootCtx context.Context, proxyURL string, headers http.Header, auth ConnectAuthorizer, dialOpts *websocket.DialOptions, localDialer Dialer, onConnect func(context.Context, *Session) error) error {
	logrus.WithField("url", proxyURL).Info("Connecting to proxy")

	// If no dial options provided, create default with headers and proxy settings
	if dialOpts == nil {
		dialOpts = &websocket.DialOptions{
			HTTPHeader: headers,
			HTTPClient: &http.Client{
				Timeout: HandshakeTimeOut,
				Transport: &http.Transport{
					Proxy: http.ProxyFromEnvironment,
					DialContext: (&net.Dialer{
						Timeout: HandshakeTimeOut,
					}).DialContext,
				},
			},
		}
	} else if len(headers) > 0 {
		// Merge headers (dialOpts take precedence)
		if dialOpts.HTTPHeader == nil {
			dialOpts.HTTPHeader = make(http.Header)
		}
		for key, values := range headers {
			if _, exists := dialOpts.HTTPHeader[http.CanonicalHeaderKey(key)]; !exists {
				dialOpts.HTTPHeader[http.CanonicalHeaderKey(key)] = values
			}
		}
	}

	ws, resp, err := websocket.Dial(rootCtx, proxyURL, dialOpts)
	if err != nil {
		if resp == nil {
			if !errors.Is(err, context.Canceled) {
				logrus.WithError(err).Errorf("Failed to connect to proxy. Empty dialer response")
			}
		} else {
			rb, err2 := io.ReadAll(resp.Body)
			if err2 != nil {
				logrus.WithError(err).Errorf("Failed to connect to proxy. Response status: %v - %v. Couldn't read response body (err: %v)", resp.StatusCode, resp.Status, err2)
			} else {
				logrus.WithError(err).Errorf("Failed to connect to proxy. Response status: %v - %v. Response body: %s", resp.StatusCode, resp.Status, rb)
			}
		}
		return err
	}
	defer ws.Close(websocket.StatusNormalClosure, "")

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
