package opsmetrics

import (
	"runtime/metrics"
	"sync/atomic"
	"time"
)

// heapAllocsBytesMetric is the runtime/metrics sample name for cumulative
// bytes allocated to the heap — D45's chosen denominator, "cheap to read
// repeatedly" per the standard library's own documentation, unlike
// runtime.ReadMemStats, which briefly stops the world.
const heapAllocsBytesMetric = "/gc/heap/allocs:bytes"

// StartHeapChurnSampler starts a ticker that samples the cumulative
// /gc/heap/allocs:bytes counter every interval and adds the delta since the
// previous tick into s's heap-churn accumulator — the allocation-share
// denominator in Snapshot. Never started implicitly (no package init): a
// caller opts in explicitly, per D45, because a background goroutine that
// starts itself is exactly the kind of thing a test or a short-lived CLI
// invocation does not want running unasked.
//
// Returns a stop function; call it to end sampling. Safe to call more than
// once — later calls after the first are no-ops.
func (s *FoldStats) StartHeapChurnSampler(interval time.Duration) (stop func()) {
	sample := []metrics.Sample{{Name: heapAllocsBytesMetric}}
	metrics.Read(sample)
	last := readHeapAllocsBytes(sample)

	ticker := time.NewTicker(interval)
	done := make(chan struct{})
	var stopped int32

	go func() {
		for {
			select {
			case <-ticker.C:
				metrics.Read(sample)
				cur := readHeapAllocsBytes(sample)
				if cur >= last {
					atomic.AddUint64(&s.heapChurn, cur-last)
				}
				// cur < last cannot happen — this is a cumulative counter —
				// but if the runtime ever changes that guarantee, skipping
				// the add rather than underflowing an unsigned delta is the
				// fail-safe direction: heap churn only ever undercounts,
				// never wraps to a huge false spike.
				last = cur
			case <-done:
				ticker.Stop()
				return
			}
		}
	}()

	return func() {
		if atomic.CompareAndSwapInt32(&stopped, 0, 1) {
			close(done)
		}
	}
}

// StartHeapChurnSampler starts the sampler on the package-level Default
// instance. See FoldStats.StartHeapChurnSampler.
func StartHeapChurnSampler(interval time.Duration) (stop func()) {
	return Default.StartHeapChurnSampler(interval)
}

// readHeapAllocsBytes extracts the uint64 value from a single-element
// sample slice populated for heapAllocsBytesMetric. Panics if the runtime
// no longer reports this metric as a Uint64 — a silently-zero heap-churn
// denominator would make every allocation-share computation divide by zero
// and read as "fold is the entire heap," the opposite failure direction
// from the one this package exists to avoid.
func readHeapAllocsBytes(sample []metrics.Sample) uint64 {
	v := sample[0].Value
	if v.Kind() != metrics.KindUint64 {
		panic("opsmetrics: " + heapAllocsBytesMetric + " is not a KindUint64 metric (runtime/metrics shape changed)")
	}
	return v.Uint64()
}
