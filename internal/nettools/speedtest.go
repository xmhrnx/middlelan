package nettools

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strconv"
	"sync"
	"sync/atomic"
	"time"
)

// SpeedPhase identifies the stage of a speed test.
type SpeedPhase int

const (
	// PhaseLatency measures idle round-trip latency.
	PhaseLatency SpeedPhase = iota
	// PhaseDownload measures download throughput.
	PhaseDownload
	// PhaseUpload measures upload throughput.
	PhaseUpload
	// PhaseDone signals that the test has finished.
	PhaseDone
)

// String returns a short label for the phase.
func (p SpeedPhase) String() string {
	switch p {
	case PhaseLatency:
		return "latency"
	case PhaseDownload:
		return "download"
	case PhaseUpload:
		return "upload"
	case PhaseDone:
		return "done"
	}
	return "unknown"
}

// SpeedUpdate is a progress report emitted during SpeedTest.
type SpeedUpdate struct {
	// Phase is the current stage.
	Phase SpeedPhase
	// Mbps is the current throughput estimate (download/upload phases).
	Mbps float64
	// Progress is the fraction of the current phase completed, 0..1.
	Progress float64
	// LatencyMs and JitterMs are the latency figures measured so far.
	LatencyMs, JitterMs float64
}

// SpeedResult is the final outcome of SpeedTest.
type SpeedResult struct {
	// DownloadMbps and UploadMbps are average throughputs in megabits/s.
	DownloadMbps, UploadMbps float64
	// LatencyMs and JitterMs describe idle latency in milliseconds.
	LatencyMs, JitterMs float64
	// Server names the test endpoint (and data centre when known).
	Server string
	// When is the time the test started.
	When time.Time
}

const (
	speedBase         = "https://speed.cloudflare.com"
	speedPhaseLen     = 10 * time.Second
	speedStreams      = 4
	speedLatencyCount = 12
	speedDownChunk    = 25 << 20
	speedUpChunk      = 8 << 20
	speedTick         = 250 * time.Millisecond
)

// SpeedTest measures latency, download and upload speed against Cloudflare's
// public speed test endpoints (about 10 seconds per throughput phase, using a
// few parallel streams). onUpdate may be nil.
func SpeedTest(ctx context.Context, onUpdate func(SpeedUpdate)) (SpeedResult, error) {
	res := SpeedResult{Server: "speed.cloudflare.com", When: time.Now()}
	emit := func(u SpeedUpdate) {
		if onUpdate != nil {
			onUpdate(u)
		}
	}
	tr := &http.Transport{
		Proxy:               http.ProxyFromEnvironment,
		MaxIdleConnsPerHost: speedStreams * 2,
		IdleConnTimeout:     30 * time.Second,
		TLSHandshakeTimeout: 10 * time.Second,
		ForceAttemptHTTP2:   false, // separate TCP streams measure better
	}
	defer tr.CloseIdleConnections()
	client := &http.Client{Transport: tr}

	lat, jit, colo, err := speedLatency(ctx, client, func(done int, l, j float64) {
		emit(SpeedUpdate{Phase: PhaseLatency, Progress: float64(done) / speedLatencyCount, LatencyMs: l, JitterMs: j})
	})
	if err != nil {
		return res, err
	}
	res.LatencyMs, res.JitterMs = lat, jit
	if colo != "" {
		res.Server += " (" + colo + ")"
	}

	down, err := speedThroughput(ctx, func(ctx context.Context, counter *atomic.Int64) error {
		return speedDownloadStream(ctx, client, counter)
	}, func(mbps, p float64) {
		emit(SpeedUpdate{Phase: PhaseDownload, Mbps: mbps, Progress: p, LatencyMs: lat, JitterMs: jit})
	})
	if err != nil {
		return res, fmt.Errorf("download: %w", err)
	}
	res.DownloadMbps = down

	up, err := speedThroughput(ctx, func(ctx context.Context, counter *atomic.Int64) error {
		return speedUploadStream(ctx, client, counter)
	}, func(mbps, p float64) {
		emit(SpeedUpdate{Phase: PhaseUpload, Mbps: mbps, Progress: p, LatencyMs: lat, JitterMs: jit})
	})
	if err != nil {
		return res, fmt.Errorf("upload: %w", err)
	}
	res.UploadMbps = up
	emit(SpeedUpdate{Phase: PhaseDone, Progress: 1, Mbps: up, LatencyMs: lat, JitterMs: jit})
	return res, nil
}

// speedLatency times small requests and returns median latency, jitter and
// the serving data centre.
func speedLatency(ctx context.Context, client *http.Client, progress func(done int, lat, jit float64)) (float64, float64, string, error) {
	var samples []float64
	var colo string
	var lastErr error
	for i := 0; i < speedLatencyCount; i++ {
		if err := ctx.Err(); err != nil {
			return 0, 0, "", err
		}
		rctx, cancel := context.WithTimeout(ctx, 3*time.Second)
		req, err := http.NewRequestWithContext(rctx, http.MethodGet, speedBase+"/__down?bytes=0", nil)
		if err != nil {
			cancel()
			return 0, 0, "", err
		}
		start := time.Now()
		resp, err := client.Do(req)
		if err != nil {
			cancel()
			lastErr = err
			continue
		}
		_, cerr := io.Copy(io.Discard, resp.Body)
		if err := resp.Body.Close(); err != nil && cerr == nil {
			cerr = err
		}
		cancel()
		if cerr != nil {
			lastErr = cerr
			continue
		}
		if resp.StatusCode != http.StatusOK {
			lastErr = fmt.Errorf("HTTP %s", resp.Status)
			continue
		}
		if colo == "" {
			colo = resp.Header.Get("cf-meta-colo")
		}
		if i == 0 {
			continue // first request includes TCP/TLS setup
		}
		samples = append(samples, float64(time.Since(start))/float64(time.Millisecond))
		l, j := latencyStats(samples)
		progress(i+1, l, j)
	}
	if len(samples) == 0 {
		if lastErr == nil {
			lastErr = errors.New("no latency samples")
		}
		return 0, 0, "", fmt.Errorf("speed test server unreachable: %w", lastErr)
	}
	l, j := latencyStats(samples)
	return l, j, colo, nil
}

// latencyStats returns the median and the mean absolute successive
// difference of samples (in ms).
func latencyStats(samples []float64) (float64, float64) {
	if len(samples) == 0 {
		return 0, 0
	}
	s := append([]float64(nil), samples...)
	sort.Float64s(s)
	med := s[len(s)/2]
	if len(s)%2 == 0 {
		med = (s[len(s)/2-1] + s[len(s)/2]) / 2
	}
	var j float64
	for i := 1; i < len(samples); i++ {
		d := samples[i] - samples[i-1]
		if d < 0 {
			d = -d
		}
		j += d
	}
	if len(samples) > 1 {
		j /= float64(len(samples) - 1)
	}
	return med, j
}

// speedThroughput runs stream in parallel for speedPhaseLen, reporting the
// running rate, and returns the average Mbps.
func speedThroughput(ctx context.Context, stream func(context.Context, *atomic.Int64) error, report func(mbps, progress float64)) (float64, error) {
	pctx, cancel := context.WithTimeout(ctx, speedPhaseLen)
	defer cancel()
	var counter atomic.Int64
	var wg sync.WaitGroup
	errCh := make(chan error, speedStreams)
	start := time.Now()
	for i := 0; i < speedStreams; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for pctx.Err() == nil {
				if err := stream(pctx, &counter); err != nil {
					if pctx.Err() == nil {
						errCh <- err
						return
					}
				}
			}
		}()
	}
	finished := make(chan struct{})
	go func() {
		wg.Wait()
		close(finished)
	}()
	tick := time.NewTicker(speedTick)
	defer tick.Stop()
loop:
	for {
		select {
		case <-finished:
			break loop
		case <-tick.C:
			el := time.Since(start)
			report(mbps(counter.Load(), el), min(1, float64(el)/float64(speedPhaseLen)))
		}
	}
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	total := counter.Load()
	el := time.Since(start)
	if total == 0 {
		select {
		case err := <-errCh:
			return 0, err
		default:
			return 0, errors.New("no data transferred")
		}
	}
	rate := mbps(total, el)
	report(rate, 1)
	return rate, nil
}

func mbps(bytes int64, d time.Duration) float64 {
	if d <= 0 {
		return 0
	}
	return float64(bytes) * 8 / d.Seconds() / 1e6
}

// countingReader counts bytes passing through it.
type countingReader struct {
	r io.Reader
	n *atomic.Int64
}

// Read implements io.Reader, adding the bytes read to the counter.
func (c *countingReader) Read(p []byte) (int, error) {
	n, err := c.r.Read(p)
	c.n.Add(int64(n))
	return n, err
}

// zeroReader yields zero bytes forever.
type zeroReader struct{}

// Read implements io.Reader by filling p with zeros.
func (zeroReader) Read(p []byte) (int, error) {
	clear(p)
	return len(p), nil
}

func speedDownloadStream(ctx context.Context, client *http.Client, counter *atomic.Int64) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, speedBase+"/__down?bytes="+strconv.Itoa(speedDownChunk), nil)
	if err != nil {
		return err
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	_, cerr := io.Copy(io.Discard, &countingReader{r: resp.Body, n: counter})
	if err := resp.Body.Close(); err != nil && cerr == nil {
		cerr = err
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %s", resp.Status)
	}
	return cerr
}

func speedUploadStream(ctx context.Context, client *http.Client, counter *atomic.Int64) error {
	body := &countingReader{r: io.LimitReader(zeroReader{}, speedUpChunk), n: counter}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, speedBase+"/__up", body)
	if err != nil {
		return err
	}
	req.ContentLength = speedUpChunk
	req.Header.Set("Content-Type", "application/octet-stream")
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	_, cerr := io.Copy(io.Discard, resp.Body)
	if err := resp.Body.Close(); err != nil && cerr == nil {
		cerr = err
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %s", resp.Status)
	}
	return cerr
}
