package remotedialer

import (
	"context"
	"errors"
	"math"
	"testing"
	"time"
)

// assertJittered checks got is want give or take jitterPercent.
func assertJittered(t *testing.T, got, want time.Duration) {
	t.Helper()

	delta := time.Duration(int64(want) / 100 * jitterPercent)
	if got < want-delta || got > want+delta {
		t.Errorf("delay = %s, want %s +/-%s", got, want, delta)
	}
}

func TestBackoffDelay(t *testing.T) {
	exponential := Backoff{Min: 5 * time.Second, Max: 5 * time.Minute}

	tests := []struct {
		name    string
		backoff Backoff
		attempt int
		want    time.Duration
	}{
		{"zero value uses the default", Backoff{}, 0, DefaultRetryMin},
		{"zero value never grows", Backoff{}, 100, DefaultRetryMin},
		{"negative Min falls back to the default", Backoff{Min: -time.Second}, 3, DefaultRetryMin},
		{"negative attempt yields Min", exponential, -1, 5 * time.Second},

		{"Min alone never grows", Backoff{Min: 30 * time.Second}, 7, 30 * time.Second},
		{"negative Max never grows", Backoff{Min: 5 * time.Second, Max: -time.Second}, 3, 5 * time.Second},
		{"Max equal to Min never grows", Backoff{Min: time.Minute, Max: time.Minute}, 4, time.Minute},
		{"Max below Min never grows", Backoff{Min: time.Minute, Max: time.Second}, 5, time.Minute},

		{"first retry waits Min", exponential, 0, 5 * time.Second},
		{"second retry doubles", exponential, 1, 10 * time.Second},
		{"third retry doubles", exponential, 2, 20 * time.Second},
		{"fourth retry doubles", exponential, 3, 40 * time.Second},
		{"fifth retry doubles", exponential, 4, 80 * time.Second},
		{"sixth retry doubles", exponential, 5, 160 * time.Second},
		{"seventh retry saturates at Max", exponential, 6, 5 * time.Minute},
		{"later retries stay at Max", exponential, 100, 5 * time.Minute},

		{"saturates rather than overshooting Max", Backoff{Min: 5 * time.Second, Max: 6 * time.Second}, 1, 6 * time.Second},
		{"lands exactly on Max", Backoff{Min: 5 * time.Second, Max: 10 * time.Second}, 1, 10 * time.Second},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertJittered(t, tt.backoff.delay(tt.attempt), tt.want)
		})
	}
}

// A non-positive delay would busy-loop the caller, so guard the extremes.
func TestBackoffDelayAlwaysPositive(t *testing.T) {
	maxes := []time.Duration{
		time.Duration(math.MaxInt64),
		time.Duration(math.MaxInt64) / 2,
		292 * 365 * 24 * time.Hour,
		100 * 365 * 24 * time.Hour,
	}

	for _, max := range maxes {
		b := Backoff{Min: 5 * time.Second, Max: max}
		for attempt := range 80 {
			if got := b.delay(attempt); got <= 0 {
				t.Fatalf("Backoff{Max: %v}.delay(%d) = %v, want a positive duration", max, attempt, got)
			}
		}
	}
}

// Jitter must vary delays or clients reconnect in lockstep.
func TestBackoffDelayJitterVaries(t *testing.T) {
	b := Backoff{Min: 5 * time.Second}

	seen := map[time.Duration]bool{}
	for range 100 {
		seen[b.delay(0)] = true
	}

	if len(seen) < 2 {
		t.Errorf("got %d distinct delay(s) over 100 calls, want jitter to vary them", len(seen))
	}
}

// ClientConnect needs context.Canceled visible through the wrapper.
func TestDialErrorUnwraps(t *testing.T) {
	if !errors.Is(error(dialError{context.Canceled}), context.Canceled) {
		t.Error("errors.Is(dialError{context.Canceled}, context.Canceled) = false, want true")
	}
}
