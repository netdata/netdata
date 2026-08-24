package main

import "testing"

func TestParsePluginArgs(t *testing.T) {
	tests := map[string]struct {
		args         []string
		wantInterval int
		wantOnly     moduleSelection
	}{
		"numeric pluginsd interval": {
			args:         []string{"10"},
			wantInterval: 10,
		},
		"dcstat long flag": {
			args:     []string{"--dcstat"},
			wantOnly: moduleSelectionDCStat,
		},
		"dcstat short-compatible flag with interval": {
			args:         []string{"-dcstat", "7"},
			wantInterval: 7,
			wantOnly:     moduleSelectionDCStat,
		},
		"fd long flag": {
			args:     []string{"--fd"},
			wantOnly: moduleSelectionFD,
		},
		"fd short-compatible flag with interval": {
			args:         []string{"-fd", "5"},
			wantInterval: 5,
			wantOnly:     moduleSelectionFD,
		},
		// Each flag means "run only this module", so they cannot combine; the C
		// plugin's getopt loop let the last one win and so does this parser.
		"last selection flag wins": {
			args:     []string{"--dcstat", "--fd"},
			wantOnly: moduleSelectionFD,
		},
		"unknown and invalid arguments stay ignored": {
			args: []string{"--unknown", "0", "invalid"},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			gotInterval, gotOnly := parsePluginArgs(tc.args)
			if gotInterval != tc.wantInterval || gotOnly != tc.wantOnly {
				t.Fatalf("parsePluginArgs(%q) = (%d, %d), want (%d, %d)",
					tc.args, gotInterval, gotOnly, tc.wantInterval, tc.wantOnly)
			}
		})
	}
}
