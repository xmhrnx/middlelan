package threat

import (
	"context"
	"time"
)

const (
	// minEvery is the shortest update interval Run accepts; Spamhaus asks
	// clients not to fetch DROP more than once an hour.
	minEvery = time.Hour
	// maxCheck bounds how long Run takes to notice enabled() turning on.
	maxCheck = 5 * time.Minute
)

// Run keeps the lists current until ctx ends. Shortly after it starts it
// updates any list older than an hour (so frequent restarts do not
// re-download), then updates every `every` (at least an hour) while
// enabled() returns true; a nil enabled means always. After a failed
// update it retries after an hour, or `every` if shorter.
func (d *DB) Run(ctx context.Context, every time.Duration, enabled func() bool) {
	every = max(every, minEvery)
	wait := min(every, time.Hour) // threshold until the first attempt
	last := d.oldest()            // zero when a list was never loaded
	timer := time.NewTimer(d.startDelay)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
		}
		if (enabled == nil || enabled()) && time.Since(last) >= wait {
			err := d.Update(ctx)
			if ctx.Err() != nil {
				return
			}
			last, wait = time.Now(), every
			if err != nil {
				wait = min(every, time.Hour)
			}
		}
		timer.Reset(min(every, maxCheck))
	}
}
