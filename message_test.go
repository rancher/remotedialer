package remotedialer

import (
	"bytes"
	"encoding/binary"
	"strings"
	"testing"
)

func TestNewServerMessage_266(t *testing.T) {
	var buf bytes.Buffer

	writeVarint := func(n int64) {
		b := make([]byte, binary.MaxVarintLen64)
		written := binary.PutVarint(b, n)
		buf.Write(b[:written])
	}

	writeVarint(1)
	writeVarint(1)
	writeVarint(int64(Connect))
	writeVarint(0)

	hostnameLen := 255
	hostname := strings.Repeat("h", hostnameLen)
	connectString := "tcp/" + hostname + ":65500"
	buf.WriteString(connectString)

	msg, err := newServerMessage(&buf)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if msg.id != 1 {
		t.Errorf("Expected id 1, got: %d", msg.id)
	}
	if msg.connID != 1 {
		t.Errorf("Expected connID 1, got: %d", msg.connID)
	}
	if msg.messageType != Connect {
		t.Errorf("Expected messageType Connect (%d), got: %d", Connect, msg.messageType)
	}
	if string(msg.bytes) != connectString {
		t.Errorf("Expected msg.bytes to equal connectString bytes, got: %v", string(msg.bytes))
	}
}
