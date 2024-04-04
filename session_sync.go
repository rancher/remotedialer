package remotedialer

import (
	"encoding/binary"
	"fmt"
)

// encodeConnectionIDs serializes a slice of connection IDs
func encodeConnectionIDs(ids []int64) []byte {
	payload := make([]byte, 0, 8*len(ids))
	for _, id := range ids {
		payload = binary.LittleEndian.AppendUint64(payload, uint64(id))
	}
	return payload
}

// decodeConnectionIDs deserializes a slice of connection IDs
func decodeConnectionIDs(payload []byte) ([]int64, error) {
	if len(payload)%8 != 0 {
		return nil, fmt.Errorf("incorrect data format")
	}
	result := make([]int64, 0, len(payload)/8)
	for x := 0; x < len(payload); x += 8 {
		id := binary.LittleEndian.Uint64(payload[x : x+8])
		result = append(result, int64(id))
	}
	return result, nil
}

func newSyncConnectionsMessage(connectionIDs []int64) *message {
	return &message{
		id:          nextid(),
		messageType: SyncConnections,
		bytes:       encodeConnectionIDs(connectionIDs),
	}
}
