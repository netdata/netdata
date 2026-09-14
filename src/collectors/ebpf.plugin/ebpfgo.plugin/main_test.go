package main

import (
	"errors"
	"testing"
)

func TestResolveFDConfigForSelection(t *testing.T) {
	resolveErr := errors.New("invalid fd config")
	resolver := func() (FDLegacyConfig, error) {
		return FDLegacyConfig{}, resolveErr
	}

	tests := map[string]struct {
		only        moduleSelection
		wantSkipped bool
		wantErr     error
	}{
		"normal startup keeps the FD configuration error": {
			only:    moduleSelectionNone,
			wantErr: resolveErr,
		},
		"FD-only startup keeps the FD configuration error": {
			only:    moduleSelectionFD,
			wantErr: resolveErr,
		},
		"DCStat-only startup skips the unrelated FD configuration error": {
			only:        moduleSelectionDCStat,
			wantSkipped: true,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			cfg, skipped, err := resolveFDConfigForSelection(tc.only, resolver)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("resolveFDConfigForSelection() error = %v, want %v", err, tc.wantErr)
			}
			if skipped != tc.wantSkipped {
				t.Fatalf("resolveFDConfigForSelection() skipped = %t, want %t", skipped, tc.wantSkipped)
			}
			if skipped && cfg.Enabled {
				t.Fatal("skipped FD config must leave fd disabled")
			}
		})
	}
}

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
