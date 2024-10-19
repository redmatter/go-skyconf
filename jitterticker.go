package skyconf

import (
	cfclock "code.cloudfoundry.org/clock"
	"time"
)

// newJitterTickerClock returns a new jitterTickerClock, which is a clock that adds a random jitter to the ticker to
// prevent thundering herd. The jitter is at most 1/10th of the duration.
func newJitterTickerClock() cfclock.Clock {
	return &jitterTickerClock{
		clock: cfclock.NewClock(),
	}
}

type jitterTickerClock struct {
	clock cfclock.Clock
}

func (j *jitterTickerClock) Now() time.Time {
	return j.clock.Now()
}

func (j *jitterTickerClock) Sleep(d time.Duration) {
	j.clock.Sleep(d)
}

func (j *jitterTickerClock) Since(t time.Time) time.Duration {
	return j.clock.Since(t)
}

func (j *jitterTickerClock) After(d time.Duration) <-chan time.Time {
	return j.clock.After(d)
}

func (j *jitterTickerClock) NewTimer(d time.Duration) cfclock.Timer {
	return j.clock.NewTimer(d)
}

func (j *jitterTickerClock) NewTicker(d time.Duration) cfclock.Ticker {
	// Add a random jitter to the ticker to prevent thundering herd, with a max jitter of 1/10th of the duration
	jitter := time.Duration(j.clock.Now().UnixNano() % int64(d/10))
	return j.clock.NewTicker(d + jitter)
}
