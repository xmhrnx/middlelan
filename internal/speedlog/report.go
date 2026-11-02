package speedlog

import (
	"math"
	"sort"
	"time"
)

// Plan is what the internet plan promises, in Mbps; zero means unknown.
type Plan struct {
	DownloadMbps float64 `json:"down"`
	UploadMbps   float64 `json:"up"`
}

// Report grades results against a plan.
type Report struct {
	Tests int
	// Median and slowest-tenth speeds, which say more than averages: a
	// connection is judged by its bad evenings.
	MedianDown, P10Down, MedianUp, P10Up, MedianPing float64
	// DownPct and UpPct are the median as a share of the plan (0 when
	// there's no plan).
	DownPct, UpPct float64
	// Grade is A to F from the share of tests reaching 80% of the plan's
	// download speed (or, without a plan, from absolute speed).
	Grade string
	// Summary says it in a sentence.
	Summary string
	// SlowHours lists the hours of the day (0-23) whose median download
	// is well under the overall median, busiest first.
	SlowHours []int
}

// BuildReport summarizes results.
func BuildReport(results []Result, plan Plan) Report {
	r := Report{Tests: len(results)}
	if len(results) == 0 {
		r.Grade, r.Summary = "–", "Run a few speed tests to see how your connection holds up."
		return r
	}
	var down, up, ping []float64
	byHour := map[int][]float64{}
	for _, t := range results {
		down = append(down, t.DownloadMbps)
		up = append(up, t.UploadMbps)
		ping = append(ping, t.LatencyMs)
		byHour[t.When.Hour()] = append(byHour[t.When.Hour()], t.DownloadMbps)
	}
	r.MedianDown, r.P10Down = quantile(down, 0.5), quantile(down, 0.1)
	r.MedianUp, r.P10Up = quantile(up, 0.5), quantile(up, 0.1)
	r.MedianPing = quantile(ping, 0.5)
	if plan.DownloadMbps > 0 {
		r.DownPct = r.MedianDown / plan.DownloadMbps * 100
	}
	if plan.UploadMbps > 0 {
		r.UpPct = r.MedianUp / plan.UploadMbps * 100
	}
	r.Grade = grade(down, plan)
	for h, v := range byHour {
		if len(v) >= 2 && quantile(v, 0.5) < r.MedianDown*0.7 {
			r.SlowHours = append(r.SlowHours, h)
		}
	}
	sort.Ints(r.SlowHours)
	r.Summary = summary(r, plan)
	return r
}

func grade(down []float64, plan Plan) string {
	if plan.DownloadMbps <= 0 {
		m := quantile(down, 0.5)
		switch {
		case m >= 100:
			return "A"
		case m >= 50:
			return "B"
		case m >= 25:
			return "C"
		case m >= 10:
			return "D"
		}
		return "F"
	}
	ok := 0
	for _, d := range down {
		if d >= plan.DownloadMbps*0.8 {
			ok++
		}
	}
	share := float64(ok) / float64(len(down))
	switch {
	case share >= 0.9:
		return "A"
	case share >= 0.75:
		return "B"
	case share >= 0.5:
		return "C"
	case share >= 0.25:
		return "D"
	}
	return "F"
}

func summary(r Report, plan Plan) string {
	if plan.DownloadMbps <= 0 {
		return "Set your plan's speed to see whether your provider delivers it."
	}
	switch {
	case r.DownPct >= 90:
		return "Your provider delivers what you pay for."
	case r.DownPct >= 70:
		return "Mostly close to your plan, with some slow periods."
	}
	return "Well below your plan. Worth raising with your provider, with this report as evidence."
}

// quantile returns the q-th quantile (0..1) by linear interpolation.
func quantile(v []float64, q float64) float64 {
	if len(v) == 0 {
		return 0
	}
	s := append([]float64(nil), v...)
	sort.Float64s(s)
	pos := q * float64(len(s)-1)
	lo := int(math.Floor(pos))
	hi := int(math.Ceil(pos))
	return s[lo] + (s[hi]-s[lo])*(pos-float64(lo))
}

// Due reports whether a scheduled test should run: none ran in the last
// `every`.
func Due(last time.Time, every time.Duration, now time.Time) bool {
	return last.IsZero() || now.Sub(last) >= every
}
