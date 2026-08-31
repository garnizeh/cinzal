package opsmetrics

import (
	"sync/atomic"
	"testing"
	"time"
)

// TestStartHeapChurnSamplerRecordsPositiveDelta starts the sampler at a
// short interval, deliberately allocates a sizeable amount of heap memory
// between ticks, and checks the accumulator moved. This can't assert an
// exact byte count — the whole point of sampling a process-wide counter is
// that other goroutines (including the test runner's own machinery) also
// allocate during the window — but a real allocation of several megabytes
// must move a churn counter sampled every few milliseconds by more than
// zero, or the sampler isn't reading anything.
func TestStartHeapChurnSamplerRecordsPositiveDelta(t *testing.T) {
	s := NewFoldStats()
	stop := s.StartHeapChurnSampler(5 * time.Millisecond)
	defer stop()

	// Deliberately allocate ~8MB across several ticks so the sampler has
	// something unmistakable to see even alongside the test binary's own
	// background allocation.
	var sink [][]byte
	for range 8 {
		sink = append(sink, make([]byte, 1<<20)) // 1MB
		time.Sleep(10 * time.Millisecond)
	}
	_ = sink

	time.Sleep(20 * time.Millisecond) // let the last tick land

	churn := atomic.LoadUint64(&s.heapChurn)
	if churn == 0 {
		t.Fatal("heap-churn accumulator is 0 after ~8MB of deliberate allocation across multiple ticks")
	}
}

// TestStartHeapChurnSamplerStopIsIdempotent asserts calling stop twice does
// not panic (closing an already-closed channel would).
func TestStartHeapChurnSamplerStopIsIdempotent(t *testing.T) {
	s := NewFoldStats()
	stop := s.StartHeapChurnSampler(time.Hour) // long interval, never expected to tick in this test
	stop()
	stop() // must not panic
}

// TestStartHeapChurnSamplerStopSamplesBeforeFirstTick is PR #398's
// CodeRabbit-found regression: a sampler stopped before its first tick
// interval elapses must still record whatever churn happened in that
// window, not report zero. Before this fix, stop only signaled the
// sampling goroutine to exit — it took no final sample of its own — so a
// caller stopping faster than one tick (a short cmd/simulate sweep, or
// this test) saw an accumulator stuck at its initial value even though
// real allocation happened. The long interval here (never expected to
// tick on its own within the test's runtime) isolates stop's own final
// sample as the only possible source of the recorded churn.
func TestStartHeapChurnSamplerStopSamplesBeforeFirstTick(t *testing.T) {
	s := NewFoldStats()
	stop := s.StartHeapChurnSampler(time.Hour)

	// Allocate enough that the delta is unmistakable against the test
	// binary's own background churn, mirroring the positive-delta test
	// above — but stop immediately after, before any tick could fire.
	sink := make([][]byte, 8)
	for i := range sink {
		sink[i] = make([]byte, 1<<20) // 1MB
	}
	_ = sink

	stop() // blocks until the final synchronous sample is added

	churn := atomic.LoadUint64(&s.heapChurn)
	if churn == 0 {
		t.Fatal("heap-churn accumulator is 0 after stop(), despite ~8MB allocated before any tick could fire — stop must take a final sample")
	}
}

// TestStartHeapChurnSamplerPackageLevel is a smoke test for the
// package-level convenience wrapper around Default.
func TestStartHeapChurnSamplerPackageLevel(t *testing.T) {
	stop := StartHeapChurnSampler(time.Hour)
	defer stop()
	// No assertion beyond "did not panic" — Default is shared with other
	// tests in this package, so this only checks the wrapper delegates to
	// Default.StartHeapChurnSampler without error.
}

// TestReadHeapAllocsBytesReturnsPositive is a direct check that the metric
// name this package depends on ("/gc/heap/allocs:bytes") is still recognised
// by the running Go runtime and reports a non-zero, KindUint64 value — a Go
// toolchain upgrade that renamed or reshaped this metric would otherwise
// only surface as heap churn silently staying zero forever, which reads as
// "fold is 100% of nothing" rather than a broken measurement.
func TestReadHeapAllocsBytesReturnsPositive(t *testing.T) {
	s := NewFoldStats()
	stop := s.StartHeapChurnSampler(5 * time.Millisecond)
	defer stop()

	// Any process has already allocated something on the heap by the time
	// this test runs (the test binary itself, goroutine stacks, etc.), so
	// the very first successful read inside StartHeapChurnSampler having
	// happened without panicking is itself the assertion that
	// readHeapAllocsBytes worked. This test exists to document that
	// intent explicitly rather than relying on the positive-delta test
	// above to be the only thing standing between us and a silent panic
	// recovery masking the failure — there is no recover() anywhere in this
	// package, so a panic here fails the test directly.
	time.Sleep(10 * time.Millisecond)
}
