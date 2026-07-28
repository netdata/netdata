package main

import "testing"

func TestCgoScaffoldReady(t *testing.T) {
	if got := cgoScaffoldReady(); got != 0 {
		t.Fatalf("unexpected cgo scaffold state: %d", got)
	}
}

func TestResolveUpdateEvery(t *testing.T) {
	tests := map[string]struct {
		cliArg, cfgVal, fallback int
		want                     int
	}{
		// Config file beats argv — the primary fix for the default-install 1s regression.
		"config beats argv":       {cliArg: 1, cfgVal: 10, fallback: 10, want: 10},
		"config beats large argv": {cliArg: 30, cfgVal: 10, fallback: 10, want: 10},
		// When config is absent, argv is used as a fallback.
		"argv used when no config":      {cliArg: 5, cfgVal: 0, fallback: 10, want: 5},
		"argv beats fallback":           {cliArg: 3, cfgVal: 0, fallback: 10, want: 3},
		// Both absent — hardcoded default wins.
		"fallback when both absent": {cliArg: 0, cfgVal: 0, fallback: 10, want: 10},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			if got := resolveUpdateEvery(tc.cliArg, tc.cfgVal, tc.fallback); got != tc.want {
				t.Fatalf("resolveUpdateEvery(%d, %d, %d) = %d, want %d",
					tc.cliArg, tc.cfgVal, tc.fallback, got, tc.want)
			}
		})
	}
}
