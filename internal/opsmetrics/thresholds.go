package opsmetrics

import "time"

// FoldDurationP99Threshold is RFC-001 §7.3's falsifiability trigger for the
// no-snapshot decision: "if p99 fold duration passes 50 ms... it gets
// built." A fold covers everything RFC §7.1's fold(Resolve, initial(seed,
// cfg), orderLog) means by "one fold" — the initial state construction plus
// every Resolve call it replays — not a single Resolve in isolation, which
// RFC §7.3's own arithmetic table treats as a third the size and would read
// as comfortably under threshold for the wrong reason.
const FoldDurationP99Threshold = 50 * time.Millisecond

// FoldAllocShareThreshold is RFC-001 §7.3's second falsifiability trigger:
// "or fold allocation exceeds 20% of total heap churn... it gets built."
//
// This constant exists for the day the comparison is meaningful — once
// cmd/server (M5) mixes fold work with the rest of a real request. In M3,
// per D51, neither cmd/replay's nor cmd/simulate's own allocation share is
// compared against this constant: FoldSnapshot.WriteHTML renders both as
// labeled reference measurements with no pass/fail mark. See this package's
// doc comment and D51 for why a process-wide ratio from either M3 process
// answers a different question than the one this threshold was chosen for.
const FoldAllocShareThreshold = 0.20
