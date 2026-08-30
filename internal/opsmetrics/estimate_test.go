package opsmetrics

import "testing"

func TestEstimateFoldBytes(t *testing.T) {
	tests := []struct {
		name         string
		resolveCalls int
		want         uint64
	}{
		{"zero calls (empty log, initial() only)", 0, BytesPerInitialCall},
		{"negative calls treated as zero", -3, BytesPerInitialCall},
		{"one call", 1, BytesPerInitialCall + BytesPerResolveCall},
		{"a full 15-round match", 15, BytesPerInitialCall + 15*BytesPerResolveCall},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := EstimateFoldBytes(tc.resolveCalls)
			if got != tc.want {
				t.Errorf("EstimateFoldBytes(%d) = %d, want %d", tc.resolveCalls, got, tc.want)
			}
		})
	}
}
