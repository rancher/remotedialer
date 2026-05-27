package remotedialer

import (
	"encoding/binary"
	"errors"
	"fmt"
	"time"
)

var errCloseSyncConnections = errors.New("sync from client")

// syncIgnoreTop disables the 'top' field guard in compareAndCloseStaleConnections.
// Used only in tests to simulate the pre-fix behavior and verify the bug is reproducible.
var syncIgnoreTop bool

// encodeConnectionIDs serializes a slice of connection IDs and the topmost connection ID seen
func encodeConnectionIDs(top int64, ids []int64) []byte {
	payload := make([]byte, 0, 8*(len(ids)+1))

	// send top to denote the latest ID this packet was send with knowledge of
	payload = binary.LittleEndian.AppendUint64(payload, uint64(top))

	for _, id := range ids {
		payload = binary.LittleEndian.AppendUint64(payload, uint64(id))
	}
	return payload
}

// decodeConnectionIDs deserializes a slice of connection IDs along with the highest seen
func decodeConnectionIDs(payload []byte) ([]int64, int64, error) {
	if len(payload)%8 != 0 {
		return nil, 0, fmt.Errorf("incorrect data format")
	}
	top := int64(binary.LittleEndian.Uint64(payload[0 : 0+8]))

	result := make([]int64, 0, (len(payload)/8)-1)
	for x := 8; x < len(payload); x += 8 {
		id := binary.LittleEndian.Uint64(payload[x : x+8])
		result = append(result, int64(id))
	}
	return result, top, nil
}

func newSyncConnectionsMessage(top int64, connectionIDs []int64) *message {
	return &message{
		id:          nextid(),
		messageType: SyncConnections,
		bytes:       encodeConnectionIDs(top, connectionIDs),
	}
}

// sendSyncConnections sends a binary message of type SyncConnections, whose payload is a list of the active connection IDs for this session
func (s *Session) sendSyncConnections() error {
	act, top := s.activeConnectionIDs()

	_, err := s.writeMessage(time.Now().Add(SyncConnectionsTimeout), newSyncConnectionsMessage(top, act))
	return err
}

// compareAndCloseStaleConnections compares the Session's activeConnectionIDs with the provided list from the client, then closing every connection not present in it
func (s *Session) compareAndCloseStaleConnections(clientIDs []int64, top int64) {
	serverIDs, _ := s.activeConnectionIDs()
	toClose := diffSortedSetsGetRemoved(serverIDs, clientIDs)
	if len(toClose) == 0 {
		return
	}

	s.Lock()
	defer s.Unlock()

	for _, id := range toClose {
		// dont close connection if packet contains id not ever seen by client
		if !syncIgnoreTop && id > top {
			break // not continue as toClose is sorted
		}

		// Connection no longer active in the client, close it server-side
		conn := s.removeConnectionLocked(id)

		if conn != nil {
			// Using doTunnelClose directly instead of tunnelClose, omitting unnecessarily sending an Error message
			conn.doTunnelClose(errCloseSyncConnections)
		}
	}
}

// diffSortedSetsGetRemoved compares two sorted slices and returns those items present in a that are not present in b
// similar to coreutil's "comm -23"
func diffSortedSetsGetRemoved(a, b []int64) []int64 {
	var res []int64
	var i, j int
	for i < len(a) && j < len(b) {
		if a[i] < b[j] { // present in "a", not in "b"
			res = append(res, a[i])
			i++
		} else if a[i] > b[j] { // present in "b", not in "a"
			j++
		} else { // present in both
			i++
			j++
		}
	}
	res = append(res, a[i:]...) // any remainders in "a" are also removed from "b"
	return res
}
