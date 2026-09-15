package metrics

import (
	"testing"
	"time"
)

func TestSummarizePercentiles(t *testing.T) {
	var tr Tracker
	for _, ms := range []int{10, 20, 30, 40, 100} {
		tr.Add(Sample{Total: time.Duration(ms) * time.Millisecond, Invalid: ms == 100, Timeout: ms == 100})
	}
	sum := tr.Summarize(time.Minute)
	if sum.Decisions != 5 || sum.InvalidOutputs != 1 || sum.Timeouts != 1 {
		t.Fatalf("%+v", sum)
	}
	if sum.Median != 30*time.Millisecond {
		t.Fatalf("median %s", sum.Median)
	}
	if sum.P95 != 100*time.Millisecond {
		t.Fatalf("p95 %s", sum.P95)
	}
	if sum.ActionsPerMin != 5 {
		t.Fatalf("apm %v", sum.ActionsPerMin)
	}
}
