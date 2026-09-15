package metrics

import (
	"math"
	"sort"
	"time"
)

// Sample is one decision's latency breakdown.
type Sample struct {
	Preprocess time.Duration
	Inference  time.Duration
	Parse      time.Duration
	Emulator   time.Duration
	Total      time.Duration
	Invalid    bool
	Timeout    bool
}

// Summary is a session rollup for model comparison.
type Summary struct {
	Decisions         int           `json:"decisions"`
	InvalidOutputs    int           `json:"invalid_model_outputs"`
	Timeouts          int           `json:"timeouts"`
	Runtime           time.Duration `json:"runtime_ns"`
	Average           time.Duration `json:"average_latency_ns"`
	Median            time.Duration `json:"median_latency_ns"`
	P95               time.Duration `json:"p95_latency_ns"`
	ActionsPerMin     float64       `json:"actions_per_minute"`
	AveragePreprocess time.Duration `json:"average_preprocess_ns"`
	AverageInference  time.Duration `json:"average_inference_ns"`
	AverageEmulator   time.Duration `json:"average_emulator_ns"`
}

// Tracker accumulates samples.
type Tracker struct {
	samples []Sample
}

func (t *Tracker) Add(s Sample) {
	t.samples = append(t.samples, s)
}

func (t *Tracker) Count() int { return len(t.samples) }

func (t *Tracker) Summarize(runtime time.Duration) Summary {
	n := len(t.samples)
	sum := Summary{Decisions: n, Runtime: runtime}
	if n == 0 {
		return sum
	}
	totals := make([]time.Duration, n)
	var pre, inf, emu time.Duration
	for i, s := range t.samples {
		totals[i] = s.Total
		pre += s.Preprocess
		inf += s.Inference
		emu += s.Emulator
		if s.Invalid {
			sum.InvalidOutputs++
		}
		if s.Timeout {
			sum.Timeouts++
		}
	}
	var totalSum time.Duration
	for _, d := range totals {
		totalSum += d
	}
	sum.Average = totalSum / time.Duration(n)
	sum.AveragePreprocess = pre / time.Duration(n)
	sum.AverageInference = inf / time.Duration(n)
	sum.AverageEmulator = emu / time.Duration(n)
	sum.Median = percentile(totals, 0.50)
	sum.P95 = percentile(totals, 0.95)
	if runtime > 0 {
		sum.ActionsPerMin = float64(n) / runtime.Minutes()
	}
	return sum
}

func percentile(vals []time.Duration, p float64) time.Duration {
	if len(vals) == 0 {
		return 0
	}
	cp := append([]time.Duration(nil), vals...)
	sort.Slice(cp, func(i, j int) bool { return cp[i] < cp[j] })
	if p <= 0 {
		return cp[0]
	}
	if p >= 1 {
		return cp[len(cp)-1]
	}
	idx := int(math.Round(p * float64(len(cp)-1)))
	if idx >= len(cp) {
		idx = len(cp) - 1
	}
	return cp[idx]
}
