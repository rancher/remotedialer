package remotedialer

import (
	"context"
	"math/rand/v2"
	"time"
)

// DefaultRetryMin is the delay used when Backoff.Min is unset.
const DefaultRetryMin = 5 * time.Second

// jitterPercent spreads delays so clients do not reconnect in lockstep.
const jitterPercent = 10

// Backoff controls how long a client waits between connection attempts.
type Backoff struct {
	// Min is the first retry delay. Zero means DefaultRetryMin.
	Min time.Duration

	// Max caps the delay; unset keeps it fixed at Min.
	Max time.Duration
}

// delay returns the jittered wait before the given retry.
func (b Backoff) delay(attempt int) time.Duration {
	d := b.Min
	if d <= 0 {
		d = DefaultRetryMin
	}

	if b.Max > d {
		for i := 0; i < attempt && d < b.Max; i++ {
			// Saturate instead of doubling past Max, which would overflow.
			if d > b.Max/2 {
				d = b.Max
				break
			}
			d *= 2
		}
	}

	return jitter(d)
}

// jitter randomises d by plus or minus jitterPercent.
func jitter(d time.Duration) time.Duration {
	// Divide before multiplying so a huge d cannot overflow.
	delta := int64(d) / 100 * jitterPercent
	if delta <= 0 {
		return d
	}

	j := d - time.Duration(delta) + time.Duration(rand.Int64N(2*delta+1))
	if j < 0 {
		return d
	}
	return j
}

// sleep waits for d or until ctx is done.
func sleep(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}
