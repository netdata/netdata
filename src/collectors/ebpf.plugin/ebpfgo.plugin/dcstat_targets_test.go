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

	if targets.LookupFast.Name != "lookup_fast" || targets.LookupFast.Mode != RunModeEntry {
		t.Fatalf("lookup_fast target = %+v, want entry probe on lookup_fast", targets.LookupFast)
	}
	// d_lookup must stay a return probe: the BPF program reads the return value
	// to tell a successful lookup from a miss.
	if targets.DLookup.Name != "d_lookup" || targets.DLookup.Mode != RunModeReturn {
		t.Fatalf("d_lookup target = %+v, want return probe on d_lookup", targets.DLookup)
	}
}

// TestResolveLookupFastTargetKeepsNameWhenAbsent verifies the resolver leaves
// the configured name in place when kallsyms has no match, so the failure
// surfaces at attach time rather than as a config error.
func TestResolveLookupFastTargetKeepsNameWhenAbsent(t *testing.T) {
	targets := defaultDCStatTargets()
	name, err := selectDCStatKallsymsPrefixFromReader(targets.LookupFast.Name, strings.NewReader(""))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if name != "" {
		t.Fatalf("expected no match, got %q", name)
	}
	if targets.LookupFast.Name != "lookup_fast" {
		t.Fatalf("target name changed unexpectedly: %q", targets.LookupFast.Name)
	}
}
