package main

import "testing"

func TestParsePluginArgs(t *testing.T) {
	tests := map[string]struct {
		args           []string
		wantInterval   int
		wantDCStatOnly bool
	}{
		"numeric pluginsd interval": {
			args:         []string{"10"},
			wantInterval: 10,
		},
		"dcstat long flag": {
			args:           []string{"--dcstat"},
			wantDCStatOnly: true,
		},
		"dcstat short-compatible flag with interval": {
			args:           []string{"-dcstat", "7"},
			wantInterval:   7,
			wantDCStatOnly: true,
		},
		"unknown and invalid arguments stay ignored": {
			args: []string{"--unknown", "0", "invalid"},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			gotInterval, gotDCStatOnly := parsePluginArgs(tc.args)
			if gotInterval != tc.wantInterval || gotDCStatOnly != tc.wantDCStatOnly {
				t.Fatalf("parsePluginArgs(%q) = (%d, %t), want (%d, %t)",
					tc.args, gotInterval, gotDCStatOnly, tc.wantInterval, tc.wantDCStatOnly)
			}
		})
	}
}
