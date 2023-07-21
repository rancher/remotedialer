package remotedialer

import (
	"context"
	"errors"
	"io"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
	"k8s.io/klog/v2"
)

// ConnectAuthorizer custom for authorization
type ConnectAuthorizer func(proto, address string) bool

// ClientConnect connect to WS and wait 5 seconds when error
func ClientConnect(ctx context.Context, wsURL string, headers http.Header, dialer *websocket.Dialer,
	auth ConnectAuthorizer, onConnect func(context.Context, *Session) error) error {
	if err := ConnectToProxy(ctx, wsURL, headers, auth, dialer, onConnect); err != nil {
		if !errors.Is(err, context.Canceled) {
			klog.FromContext(ctx).Error(err, "Remotedialer proxy error")
			time.Sleep(time.Duration(5) * time.Second)
		}
		return err
	}
	return nil
}

// ConnectToProxy connect to websocket server
func ConnectToProxy(rootCtx context.Context, proxyURL string, headers http.Header, auth ConnectAuthorizer, dialer *websocket.Dialer, onConnect func(context.Context, *Session) error) error {
	klog.FromContext(rootCtx).Info("Connecting to proxy", "url", proxyURL)

	if dialer == nil {
		dialer = &websocket.Dialer{Proxy: http.ProxyFromEnvironment, HandshakeTimeout: HandshakeTimeOut}
	}
	ws, resp, err := dialer.DialContext(rootCtx, proxyURL, headers)
	if err != nil {
		if resp == nil {
			if !errors.Is(err, context.Canceled) {
				klog.FromContext(rootCtx).Error(err, "Failed to connect to proxy. Empty dialer response")
			}
		} else {
			rb, err2 := io.ReadAll(resp.Body)
			if err2 != nil {
				klog.FromContext(rootCtx).Error(err, "Failed to connect to proxy. Empty dialer response", "statusCode", resp.StatusCode, "status", resp.Status, "err2", err2)
			} else {
				klog.FromContext(rootCtx).Error(err, "Failed to connect to proxy", "statusCode", resp.StatusCode, "status", resp.Status, "body", string(rb))
			}
		}
		return err
	}
	defer ws.Close()

	result := make(chan error, 2)

	ctx, cancel := context.WithCancel(rootCtx)
	defer cancel()

	session, err := NewClientSession(ctx, auth, ws)
	if err != nil {
		return err
	}
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

	select {
	case <-ctx.Done():
		klog.FromContext(ctx).Info("Proxy done", "url", proxyURL, "err", ctx.Err())
		return nil
	case err := <-result:
		return err
	}
}
