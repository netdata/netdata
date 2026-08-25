package main

import "testing"

func TestParsePluginArgs(t *testing.T) {
	tests := map[string]struct {
		args         []string
		wantInterval int
		wantOnly     moduleSelection
		wantErrors   *bool
		wantLoad     *LoadMethod
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
		"fd legacy filedescriptor alias": {
			args:     []string{"--filedescriptor"},
			wantOnly: moduleSelectionFD,
		},
		"fd short legacy filedescriptor alias": {
			args:     []string{"-filedescriptor"},
			wantOnly: moduleSelectionFD,
		},
		"return enables fd error charts": {
			args:       []string{"--filedescriptor", "--return"},
			wantOnly:   moduleSelectionFD,
			wantErrors: new(true),
		},
		"legacy forces fd legacy object": {
			args:     []string{"--fd", "--legacy"},
			wantOnly: moduleSelectionFD,
			wantLoad: new(LoadLegacy),
		},
		"core overrides legacy when last": {
			args:     []string{"--legacy", "--core"},
			wantLoad: new(LoadCore),
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
			gotInterval, gotOnly, gotFD := parsePluginArgs(tc.args)
			if gotInterval != tc.wantInterval || gotOnly != tc.wantOnly {
				t.Fatalf("parsePluginArgs(%q) = (%d, %d), want (%d, %d)",
					tc.args, gotInterval, gotOnly, tc.wantInterval, tc.wantOnly)
			}
			if (gotFD.reportErrors == nil) != (tc.wantErrors == nil) ||
				gotFD.reportErrors != nil && *gotFD.reportErrors != *tc.wantErrors {
				t.Fatalf("parsePluginArgs(%q) reportErrors = %#v, want %#v", tc.args, gotFD.reportErrors, tc.wantErrors)
			}
			if (gotFD.loadMethod == nil) != (tc.wantLoad == nil) ||
				gotFD.loadMethod != nil && *gotFD.loadMethod != *tc.wantLoad {
				t.Fatalf("parsePluginArgs(%q) loadMethod = %#v, want %#v", tc.args, gotFD.loadMethod, tc.wantLoad)
			}
		})
	}
}
