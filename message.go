package remotedialer

import (
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"strings"
)

const (
	Connect messageType = iota + 1
	AddClient
	RemoveClient
)

type messageType byte

type message struct {
	messageType messageType
	bytes       []byte
	conn        net.Conn
	proto       string
	address     string
}

func newConnect(proto, address string) *message {
	return &message{
		messageType: Connect,
		bytes:       []byte(fmt.Sprintf("%s/%s", proto, address)),
		proto:       proto,
		address:     address,
	}
}

func newAddClient(client string) *message {
	return &message{
		messageType: AddClient,
		address:     client,
		bytes:       []byte(client),
	}
}

func newRemoveClient(client string) *message {
	return &message{
		messageType: RemoveClient,
		address:     client,
		bytes:       []byte(client),
	}
}

func newServerMessage(conn net.Conn) (*message, error) {
	byteReader := &byteReader{conn}
	mType, err := byteReader.ReadByte()
	if err != nil {
		return nil, err
	}

	messageLength, err := binary.ReadVarint(byteReader)
	if err != nil {
		return nil, err
	}

	m := &message{
		messageType: messageType(mType),
		conn:        conn,
	}

	if m.messageType == Connect {
		bytes, err := io.ReadAll(io.LimitReader(conn, messageLength))
		if err != nil {
			return nil, err
		}
		parts := strings.SplitN(string(bytes), "/", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("failed to parse connect address")
		}
		m.proto = parts[0]
		m.address = parts[1]
	} else if m.messageType == AddClient || m.messageType == RemoveClient {
		bytes, err := io.ReadAll(io.LimitReader(conn, messageLength))
		if err != nil {
			return nil, err
		}
		m.address = string(bytes)
	}

	return m, nil
}

func (m *message) Bytes() []byte {
	buf := []byte{}
	buf = append(buf, byte(m.messageType))
	buf = binary.AppendVarint(buf, int64(len(m.bytes)))
	buf = append(buf, m.bytes...)
	return buf
}

func (m *message) String() string {
	switch m.messageType {
	case Connect:
		return fmt.Sprintf("CONNECT      : %s/%s", m.proto, m.address)
	case AddClient:
		return fmt.Sprintf("ADDCLIENT    [%s]", m.address)
	case RemoveClient:
		return fmt.Sprintf("REMOVECLIENT [%s]", m.address)
	}
	return fmt.Sprintf("UNKNOWN: %d", m.messageType)
}
