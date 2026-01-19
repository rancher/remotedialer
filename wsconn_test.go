package remotedialer

import (
	"context"
	"errors"

	"github.com/coder/websocket"
)

type fakeWSConn struct {
	writeCallback func(context.Context, websocket.MessageType, []byte) error
}

func (f *fakeWSConn) Close(code websocket.StatusCode, reason string) error {
	return nil
}

func (f *fakeWSConn) Read(ctx context.Context) (websocket.MessageType, []byte, error) {
	return 0, nil, errors.New("not implemented")
}

func (f *fakeWSConn) Write(ctx context.Context, typ websocket.MessageType, data []byte) error {
	if cb := f.writeCallback; cb != nil {
		return cb(ctx, typ, data)
	}
	return errors.New("callback not provided")
}

func (f *fakeWSConn) SetReadLimit(limit int64) {
	// no-op for fake
}
