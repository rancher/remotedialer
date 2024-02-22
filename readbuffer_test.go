package remotedialer

import (
	"bytes"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func Test_readBuffer_Read(t *testing.T) {
	type args struct {
		b []byte
	}
	tests := []struct {
		name     string
		args     args
		deadline time.Duration
		timeout  time.Duration
		offer    []byte
		want     int
		wantErr  assert.ErrorAssertionFunc
	}{
		{
			name: "Read",
			args: args{
				b: make([]byte, 10),
			},
			offer:   []byte("test"),
			want:    4,
			wantErr: assert.NoError,
		},
		{
			name: "Read with timeout",
			args: args{
				b: make([]byte, 10),
			},
			timeout: 100 * time.Millisecond,
			want:    0,
			wantErr: func(t assert.TestingT, err error, msgAndArgs ...interface{}) bool {
				return assert.Equal(t, err, ErrReadTimeoutExceeded)
			},
		},
		{
			name: "Read with deadline",
			args: args{
				b: make([]byte, 10),
			},
			deadline: 100 * time.Millisecond,
			offer:    nil,
			want:     0,
			wantErr: func(t assert.TestingT, err error, msgAndArgs ...interface{}) bool {
				return assert.Equal(t, err, ErrReadDeadlineExceeded)
			},
		},
		{
			name: "Read with timeout and deadline",
			args: args{
				b: make([]byte, 10),
			},
			deadline: 50 * time.Millisecond,
			offer:    nil,
			want:     0,
			wantErr: func(t assert.TestingT, err error, msgAndArgs ...interface{}) bool {
				return assert.Equal(t, err, ErrReadDeadlineExceeded)
			},
		},
	}

	t.Parallel()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &readBuffer{
				id:           1,
				backPressure: newBackPressure(nil),
				cond: sync.Cond{
					L: &sync.Mutex{},
				},
			}
			if tt.deadline > 0 {
				r.deadline = time.Now().Add(tt.deadline)
			}
			if tt.timeout > 0 {
				r.readTimeout = tt.timeout
			}
			if tt.offer != nil {
				err := r.Offer(bytes.NewReader(tt.offer))
				if err != nil {
					t.Errorf("Offer() returned error: %v", err)
				}
			}
			got, err := r.Read(tt.args.b)
			if !tt.wantErr(t, err, fmt.Sprintf("Read(%v)", tt.args.b)) {
				return
			}
			assert.Equalf(t, tt.want, got, "Read(%v)", tt.args.b)
		})
	}
}
