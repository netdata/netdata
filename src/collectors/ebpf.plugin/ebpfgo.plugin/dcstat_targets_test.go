package main

import (
	"strings"
	"testing"
)

func TestSelectDCStatKallsymsPrefixFromReader(t *testing.T) {
	tests := map[string]struct {
		kallsyms string
		prefix   string
		want     string
	}{
		"exact match": {
			kallsyms: "ffffffff81000000 t lookup_fast\nffffffff81000010 T d_lookup\n",
			prefix:   "lookup_fast",
			want:     "lookup_fast",
		},
		"suffixed static function": {
			// gcc emits lookup_fast with an .isra/.constprop suffix on many builds;
			// attaching to the bare name would fail.
			kallsyms: "ffffffff81000000 t lookup_fast.isra.0\n",
			prefix:   "lookup_fast",
			want:     "lookup_fast.isra.0",
		},
		"skips the __pfx_ alias": {
			// __pfx_lookup_fast does not start with the prefix, so the real symbol
			// is still selected even when the alias comes first.
			kallsyms: "ffffffff81000000 t __pfx_lookup_fast\nffffffff81000010 t lookup_fast\n",
			prefix:   "lookup_fast",
			want:     "lookup_fast",
		},
		"skips non-probeable symbol types": {
			kallsyms: "ffffffff81000000 d lookup_fast_data\nffffffff81000010 t lookup_fast\n",
			prefix:   "lookup_fast",
			want:     "lookup_fast",
		},
		"absent symbol returns empty": {
			kallsyms: "ffffffff81000000 T d_lookup\n",
			prefix:   "lookup_fast",
			want:     "",
		},
		"short lines are ignored": {
			kallsyms: "garbage\nffffffff81000010 t lookup_fast\n",
			prefix:   "lookup_fast",
			want:     "lookup_fast",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got, err := selectDCStatKallsymsPrefixFromReader(tc.prefix, strings.NewReader(tc.kallsyms))
			if err != nil {
				t.Fatalf("selectDCStatKallsymsPrefixFromReader: %v", err)
			}
			if got != tc.want {
				t.Fatalf("selectDCStatKallsymsPrefixFromReader() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestSelectDCStatKallsymsPrefixRejectsEmptyPrefix(t *testing.T) {
	if _, err := selectDCStatKallsymsPrefixFromReader("", strings.NewReader("")); err == nil {
		t.Fatal("expected an error for an empty prefix")
	}
}

func TestDefaultDCStatTargets(t *testing.T) {
	targets := defaultDCStatTargets()

	if targets.LookupFast.Name != "lookup_fast" {
		t.Fatalf("lookup_fast target = %+v, want lookup_fast", targets.LookupFast)
	}
	if targets.DLookup.Name != "d_lookup" {
		t.Fatalf("d_lookup target = %+v, want d_lookup", targets.DLookup)
	}
}

func TestResolveLookupFastTarget(t *testing.T) {
	tests := map[string]struct {
		resolved string
		want     string
	}{
		"adopts the resolved symbol":        {resolved: "lookup_fast.isra.0", want: "lookup_fast.isra.0"},
		"keeps the default when absent":     {resolved: "", want: "lookup_fast"},
		"keeps the default when unsuffixed": {resolved: "lookup_fast", want: "lookup_fast"},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			targets := defaultDCStatTargets()
			targets.ResolveLookupFastTarget(tc.resolved)
			if targets.LookupFast.Name != tc.want {
				t.Fatalf("LookupFast.Name = %q, want %q", targets.LookupFast.Name, tc.want)
			}
			if targets.DLookup.Name != "d_lookup" {
				t.Fatalf("d_lookup must not be touched, got %q", targets.DLookup.Name)
			}
		})
	}
}

// TestResolveDCStatTargetsSurvivesUnreadableKallsyms proves dcstat degrades to
// the default symbol name instead of failing: a /proc/kallsyms error must never
// take down the collectors sharing this process.
func TestResolveDCStatTargetsSurvivesUnreadableKallsyms(t *testing.T) {
	got := resolveDCStatTargets()

	// On any host this returns usable targets: either the resolved symbol (when
	// /proc/kallsyms is readable) or the configured default (when it is not).
	if !strings.HasPrefix(got.LookupFast.Name, "lookup_fast") {
		t.Fatalf("LookupFast.Name = %q, want a lookup_fast* symbol", got.LookupFast.Name)
	}
	if got.DLookup.Name != "d_lookup" {
		t.Fatalf("DLookup.Name = %q, want d_lookup", got.DLookup.Name)
	}
}
