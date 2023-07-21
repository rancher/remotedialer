package remotedialer

import (
	"io"

	"github.com/gorilla/websocket"
)

func newConn(wsConn *websocket.Conn) io.ReadWriteCloser {
	return &conn{
		wsConn: wsConn,
	}
}

type conn struct {
	wsConn *websocket.Conn

	currentReader io.Reader
}

func (c *conn) Read(b []byte) (n int, err error) {
	if c.currentReader != nil {
		cont, n, err := c.read(b)
		if !cont || err != nil {
			return n, err
		}
	}

	for {
		messageType, reader, err := c.wsConn.NextReader()
		switch {
		case err != nil:
			return 0, err
		case messageType != websocket.BinaryMessage && messageType == websocket.CloseMessage:
			_ = c.wsConn.Close()
			return 0, io.EOF
		default:
			c.currentReader = reader
			cont, n, err := c.read(b)
			if !cont || err != nil {
				return n, err
			}
		}
	}
}

func (c *conn) read(b []byte) (bool, int, error) {
	n, err := c.currentReader.Read(b)
	if err != nil && err == io.EOF {
		c.currentReader = nil
		if n == 0 {
			return true, 0, nil
		}
	}

	return false, n, err
}

func (c *conn) Write(b []byte) (int, error) {
	err := c.wsConn.WriteMessage(websocket.BinaryMessage, b)
	if err != nil {
		return 0, err
	}

	return len(b), nil
}

func (c *conn) Close() error {
	return c.wsConn.Close()
}
