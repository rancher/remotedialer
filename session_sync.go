package remotedialer

import (
	"encoding/binary"
	"errors"
	"fmt"
	"time"
)

var errCloseSyncConnections = errors.New("sync from client")

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

// sendSyncConnections sends a binary message of type SyncConnections, whose payload is a list of the active connection IDs for this session
func (s *Session) sendSyncConnections() error {
	_, err := s.writeMessage(time.Now().Add(SyncConnectionsTimeout), newSyncConnectionsMessage(s.activeConnectionIDs()))
	return err
}

// lockedSyncConnections closes any session connection that is not present in the IDs received from the client
// The session lock must be hold by the caller when calling this method
func (s *Session) lockedSyncConnections(payload []byte) error {
	clientIDs, err := decodeConnectionIDs(payload)
	if err != nil {
		return fmt.Errorf("decoding sync connections payload: %w", err)
	}
	for _, id := range s.activeConnectionIDs() {
		if i := sliceIndex(clientIDs, id); i >= 0 {
			// Connection is still active, do nothing
			clientIDs = append(clientIDs[:i], clientIDs[i+1:]...) // IDs are unique, skip in next iteration
			continue
		}

		// Connection no longer active in the client, close it server-side
		conn := s.lockedRemoveConnection(id)
		if conn != nil {
			// Using doTunnelClose directly instead of tunnelClose, omitting unnecessarily sending an Error message
			conn.doTunnelClose(errCloseSyncConnections)
		}
	}
	return nil
}

func sliceIndex(s []int64, id int64) int {
	for x := range s {
		if s[x] == id {
			return x
		}
	}
	return -1
}
