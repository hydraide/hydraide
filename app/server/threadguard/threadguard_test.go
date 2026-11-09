package threadguard

import (
	"testing"
	"time"
)

func TestWarner_OnlyNewHighsAboveThreshold(t *testing.T) {
	w := &warner{threshold: 100}
	t0 := time.Now()

	if w.shouldWarn(99, t0) {
		t.Fatal("warned below the threshold")
	}
	if !w.shouldWarn(150, t0) {
		t.Fatal("did not warn above the threshold")
	}
	if w.shouldWarn(150, t0.Add(2*minWarnGap)) {
		t.Fatal("warned again for a count that did not grow")
	}
	if w.shouldWarn(200, t0.Add(minWarnGap/2)) {
		t.Fatal("warned again inside the rate-limit gap")
	}
	if !w.shouldWarn(200, t0.Add(minWarnGap+time.Second)) {
		t.Fatal("did not warn for a new high after the gap")
	}
}

func TestThreadCount_Positive(t *testing.T) {
	if n := ThreadCount(); n <= 0 {
		t.Fatalf("ThreadCount = %d", n)
	}
}

func TestSetMaxThreads_ReturnsPrevious(t *testing.T) {
	prev := SetMaxThreads(20000)
	t.Cleanup(func() { SetMaxThreads(prev) })
	if got := SetMaxThreads(0); got != 20000 {
		t.Fatalf("previous limit = %d, want 20000", got)
	}
	if got := SetMaxThreads(prev); got != DefaultMaxThreads {
		t.Fatalf("SetMaxThreads(0) set %d, want %d", got, DefaultMaxThreads)
	}
}
