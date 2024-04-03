package remotedialer

import (
	"fmt"
	"math/rand"
	"reflect"
	"testing"
)

func Test_encodeConnectionIDs(t *testing.T) {
	tests := []struct {
		size int
	}{
		{0},
		{1},
		{2},
		{10},
		{100},
		{1000},
	}
	for _, tt := range tests {
		t.Run(fmt.Sprintf("%d_ids", tt.size), func(t *testing.T) {
			ids := generateIDs(tt.size)
			encoded := encodeConnectionIDs(ids)
			decoded, err := decodeConnectionIDs(encoded)
			if err != nil {
				t.Error(err)
			}
			if got, want := decoded, ids; !reflect.DeepEqual(got, want) {
				t.Errorf("encoding and decoding differs from original data, got: %v, want: %v", got, want)
			}
		})
	}
}

func generateIDs(n int) []int64 {
	ids := make([]int64, n)
	for x := range ids {
		ids[x] = rand.Int63()
	}
	return ids
}
