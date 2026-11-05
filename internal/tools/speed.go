package tools

import (
	"bytes"
	"context"
	"crypto/rand"
	"fmt"
	"io"
	"net/http"
	"sort"
	"sync"
	"sync/atomic"
	"time"
)

// The speed test downloads from and uploads to Cloudflare's speed test
// servers through the proxy, as speed.cloudflare.com does.
const (
	speedHost    = "https://speed.cloudflare.com"
	speedStreams = 4
	// speedPhase is how long the download, and the upload, last.
	speedPhase = 8 * time.Second
)

// SpeedSample is how a speed test goes: Phase is latency, download,
// upload or done; the speeds are in bits per second and the times in
// milliseconds.
type SpeedSample struct {
	Phase    string  `json:"phase"`
	Progress float64 `json:"progress"` // of the phase, 0 to 1
	Latency  float64 `json:"latency"`
	Jitter   float64 `json:"jitter"`
	Download float64 `json:"download"`
	Upload   float64 `json:"upload"`
	// Bytes is what moved in all.
	Bytes int64  `json:"bytes"`
	Error string `json:"error,omitempty"`
	// Colo is the Cloudflare data center that answered, and Country where
	// the test came from, as Cloudflare saw it.
	Colo    string `json:"colo,omitempty"`
	Country string `json:"country,omitempty"`
}

// SpeedTest measures the latency, then the download and upload speeds
// through the proxy, calling report as it goes. The last sample has the
// phase "done".
func SpeedTest(ctx context.Context, proxy string, report func(SpeedSample)) SpeedSample {
	var s SpeedSample
	c := Client(proxy, 15*time.Second, true)
	send := func(phase string, progress float64) {
		s.Phase, s.Progress = phase, progress
		report(s)
	}

	// Latency: the time to the headers of an empty download, a few times.
	var rtts []float64
	for i := 0; i < 8 && ctx.Err() == nil; i++ {
		start := time.Now()
		resp, err := get0(ctx, c, speedHost+"/__down?bytes=0")
		if err != nil {
			if i == 0 {
				s.Error = shortError(err)
				send("done", 1)
				return s
			}
			continue
		}
		rtts = append(rtts, float64(time.Since(start).Microseconds())/1000)
		if s.Colo == "" {
			s.Colo, s.Country = resp.Header.Get("cf-meta-colo"), resp.Header.Get("cf-meta-country")
			if ray := resp.Header.Get("cf-ray"); s.Colo == "" && len(ray) > 4 && ray[len(ray)-4] == '-' {
				s.Colo = ray[len(ray)-3:]
			}
		}
		resp.Body.Close()
		s.Latency, s.Jitter = latencyOf(rtts)
		send("latency", float64(i+1)/8)
	}
	if ctx.Err() != nil {
		return s
	}

	// Download.
	s.Download = s.measure(ctx, "download", func(ctx context.Context, moved *atomic.Int64) error {
		buf := make([]byte, 64<<10)
		for ctx.Err() == nil {
			// The servers refuse more than about 100 MB at once.
			resp, err := get0(ctx, c, fmt.Sprintf("%s/__down?bytes=%d", speedHost, 50_000_000))
			if err != nil {
				return err
			}
			for {
				n, err := resp.Body.Read(buf)
				moved.Add(int64(n))
				if err != nil {
					break
				}
			}
			resp.Body.Close()
		}
		return nil
	}, send)

	// Upload.
	chunk := make([]byte, 4<<20)
	_, _ = rand.Read(chunk)
	s.Upload = s.measure(ctx, "upload", func(ctx context.Context, moved *atomic.Int64) error {
		for ctx.Err() == nil {
			body := &countingReader{r: bytes.NewReader(chunk), n: moved}
			req, err := http.NewRequestWithContext(ctx, http.MethodPost, speedHost+"/__up", body)
			if err != nil {
				return err
			}
			req.ContentLength = int64(len(chunk))
			req.Header.Set("Content-Type", "application/octet-stream")
			resp, err := c.Do(req)
			if err != nil {
				return err
			}
			_, _ = io.Copy(io.Discard, resp.Body)
			resp.Body.Close()
		}
		return nil
	}, send)
	send("done", 1)
	return s
}

// measure runs streams of work for speedPhase and returns the speed, in
// bits per second, of the second half: the first lets the connections
// open and grow their windows.
func (s *SpeedSample) measure(ctx context.Context, phase string, work func(context.Context, *atomic.Int64) error, send func(string, float64)) float64 {
	ctx, cancel := context.WithTimeout(ctx, speedPhase)
	defer cancel()
	var moved atomic.Int64
	var wg sync.WaitGroup
	var errOnce sync.Once
	for i := 0; i < speedStreams; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := work(ctx, &moved); err != nil && ctx.Err() == nil {
				errOnce.Do(func() { s.Error = shortError(err) })
			}
		}()
	}
	start := time.Now()
	tick := time.NewTicker(250 * time.Millisecond)
	defer tick.Stop()
	var markAt time.Time
	var markBytes int64
	speed := 0.0
	before := s.Bytes
	for {
		select {
		case <-ctx.Done():
			wg.Wait()
			s.Bytes = before + moved.Load()
			return speed
		case now := <-tick.C:
			el := now.Sub(start)
			cur := moved.Load()
			if markAt.IsZero() && el >= speedPhase/4 {
				markAt, markBytes = now, cur
			}
			if !markAt.IsZero() && now.Sub(markAt) > 0 {
				speed = float64(cur-markBytes) * 8 / now.Sub(markAt).Seconds()
			} else if el > 0 {
				speed = float64(cur) * 8 / el.Seconds()
			}
			s.Bytes = before + cur
			if phase == "download" {
				s.Download = speed
			} else {
				s.Upload = speed
			}
			send(phase, min(1, el.Seconds()/speedPhase.Seconds()))
		}
	}
}

type countingReader struct {
	r *bytes.Reader
	n *atomic.Int64
}

func (c *countingReader) Read(p []byte) (int, error) {
	n, err := c.r.Read(p)
	c.n.Add(int64(n))
	return n, err
}

func get0(ctx context.Context, c *http.Client, url string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	return resp, nil
}

// latencyOf is the median of the round trips, and their mean deviation
// from one to the next.
func latencyOf(rtts []float64) (median, jitter float64) {
	if len(rtts) == 0 {
		return 0, 0
	}
	sorted := append([]float64(nil), rtts...)
	sort.Float64s(sorted)
	median = sorted[len(sorted)/2]
	if len(sorted)%2 == 0 {
		median = (sorted[len(sorted)/2-1] + sorted[len(sorted)/2]) / 2
	}
	for i := 1; i < len(rtts); i++ {
		d := rtts[i] - rtts[i-1]
		if d < 0 {
			d = -d
		}
		jitter += d
	}
	if len(rtts) > 1 {
		jitter /= float64(len(rtts) - 1)
	}
	return median, jitter
}
