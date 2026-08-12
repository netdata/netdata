package main

import (
	"errors"
	"io"
	"os"
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

// errKallsyms is a symbol table whose read always fails, so the resolver's
// scanner-error branch is reachable without depending on the host's /proc.
type errKallsyms struct{ closed bool }

func (e *errKallsyms) Read([]byte) (int, error) { return 0, errors.New("read failed") }
func (e *errKallsyms) Close() error             { e.closed = true; return nil }

type nopCloser struct{ io.Reader }

func (nopCloser) Close() error { return nil }

func TestResolveDCStatTargetsFrom(t *testing.T) {
	tests := map[string]struct {
		open kallsymsOpener
		want string
	}{
		"adopts the suffixed symbol": {
			open: func() (io.ReadCloser, error) {
				return nopCloser{strings.NewReader("ffffffff81000000 t lookup_fast.isra.0\n")}, nil
			},
			want: "lookup_fast.isra.0",
		},
		"keeps the default when the symbol is absent": {
			open: func() (io.ReadCloser, error) {
				return nopCloser{strings.NewReader("ffffffff81000000 T d_lookup\n")}, nil
			},
			want: "lookup_fast",
		},
		"keeps the default when the table cannot be opened": {
			// The P1 case: /proc/kallsyms missing or permission-denied must not
			// fail dcstat configuration, because that would take down every other
			// collector in this process.
			open: func() (io.ReadCloser, error) { return nil, os.ErrPermission },
			want: "lookup_fast",
		},
		"keeps the default when the table cannot be read": {
			open: func() (io.ReadCloser, error) { return &errKallsyms{}, nil },
			want: "lookup_fast",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got := resolveDCStatTargetsFrom(tc.open)
			if got.LookupFast.Name != tc.want {
				t.Fatalf("LookupFast.Name = %q, want %q", got.LookupFast.Name, tc.want)
			}
			if got.DLookup.Name != "d_lookup" {
				t.Fatalf("DLookup.Name = %q, want d_lookup", got.DLookup.Name)
			}
		})
	}
}

// TestResolveDCStatTargetsFromClosesTheTable guards against leaking the
// symbol-table handle on the read-error path.
func TestResolveDCStatTargetsFromClosesTheTable(t *testing.T) {
	table := &errKallsyms{}
	resolveDCStatTargetsFrom(func() (io.ReadCloser, error) { return table, nil })
	if !table.closed {
		t.Fatal("resolveDCStatTargetsFrom did not close the symbol table")
	}
}

// TestResolveDCStatTargetsLiveHost is a smoke test over the real /proc: it only
// asserts the resolver returns usable targets on this machine.  The degrade and
// adopt branches are covered by TestResolveDCStatTargetsFrom above.
func TestResolveDCStatTargetsLiveHost(t *testing.T) {
	got := resolveDCStatTargets()

	if !strings.HasPrefix(got.LookupFast.Name, "lookup_fast") {
		t.Fatalf("LookupFast.Name = %q, want a lookup_fast* symbol", got.LookupFast.Name)
	}
	if got.DLookup.Name != "d_lookup" {
		t.Fatalf("DLookup.Name = %q, want d_lookup", got.DLookup.Name)
	}
}
