package main

import (
	"errors"
	"io"
	"strings"
	"testing"
)

// nopReadCloser adapts a reader for the kallsymsOpener seam and records whether
// the caller closed it — resolveFDTargetsFrom must not leak the file handle.
type nopReadCloser struct {
	io.Reader
	closed bool
}

func (r *nopReadCloser) Close() error {
	r.closed = true
	return nil
}

func TestResolveFDTargetsFromReader(t *testing.T) {
	tests := map[string]struct {
		kallsyms  string
		wantOpen  string
		wantClose string
		wantErr   bool
	}{
		// Modern kernel: both newest symbols present.
		"5-11-and-newer": {
			kallsyms: `
ffffffff81000000 T do_sys_openat2
ffffffff81000010 T close_fd
`,
			wantOpen:  "do_sys_openat2",
			wantClose: "close_fd",
		},
		// Older kernel: only the legacy symbols exist.
		"pre-5-6": {
			kallsyms: `
ffffffff81000000 T do_sys_open
ffffffff81000010 T __close_fd
`,
			wantOpen:  "do_sys_open",
			wantClose: "__close_fd",
		},
		// Transitional kernel exporting both open symbols: candidate order must
		// pick do_sys_openat2, because that is what the shipped objects trace.
		"both-open-symbols-present": {
			kallsyms: `
ffffffff81000000 T do_sys_open
ffffffff81000010 T do_sys_openat2
ffffffff81000020 T __close_fd
`,
			wantOpen:  "do_sys_openat2",
			wantClose: "__close_fd",
		},
		"both-close-symbols-present": {
			kallsyms: `
ffffffff81000000 T __close_fd
ffffffff81000010 T close_fd
ffffffff81000020 T do_sys_openat2
`,
			wantOpen:  "do_sys_openat2",
			wantClose: "close_fd",
		},
		"non-probeable-symbol-types-are-skipped": {
			kallsyms: `
ffffffff81000000 d close_fd
ffffffff81000010 T __close_fd
ffffffff81000020 T do_sys_openat2
`,
			wantOpen:  "do_sys_openat2",
			wantClose: "__close_fd",
		},
		// Unlike dcstat, an unresolved target is fatal: there is no fallback name
		// and attaching to a symbol that does not exist fails less legibly.
		"missing-open-target-is-an-error": {
			kallsyms: "ffffffff81000000 T close_fd\n",
			wantErr:  true,
		},
		"missing-close-target-is-an-error": {
			kallsyms: "ffffffff81000000 T do_sys_openat2\n",
			wantErr:  true,
		},
		"empty-symbol-table-is-an-error": {
			kallsyms: "",
			wantErr:  true,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got, err := resolveFDTargetsFromReader(strings.NewReader(tc.kallsyms))
			if tc.wantErr {
				if err == nil {
					t.Fatalf("resolveFDTargetsFromReader() = %+v, want error", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("resolveFDTargetsFromReader: %v", err)
			}
			if got.Open != tc.wantOpen || got.Close != tc.wantClose {
				t.Fatalf("resolveFDTargetsFromReader() = %+v, want {Open:%s Close:%s}",
					got, tc.wantOpen, tc.wantClose)
			}
		})
	}
}

// TestResolveFDTargetsFromSingleScan documents the reason both symbols are
// resolved together: /proc/kallsyms is multi-MB and not seekable, so the
// resolver gets exactly one pass over it.
func TestResolveFDTargetsFromSingleScan(t *testing.T) {
	table := &nopReadCloser{Reader: strings.NewReader(
		"ffffffff81000000 T do_sys_openat2\nffffffff81000010 T close_fd\n")}

	reads := 0
	got, err := resolveFDTargetsFrom(func() (io.ReadCloser, error) {
		reads++
		return table, nil
	})
	if err != nil {
		t.Fatalf("resolveFDTargetsFrom: %v", err)
	}
	if reads != 1 {
		t.Fatalf("opened the symbol table %d times, want 1", reads)
	}
	if !table.closed {
		t.Fatal("resolveFDTargetsFrom did not close the symbol table")
	}
	if got != (FDTargets{Open: "do_sys_openat2", Close: "close_fd"}) {
		t.Fatalf("resolveFDTargetsFrom() = %+v", got)
	}
}

func TestResolveFDTargetsFromOpenFailure(t *testing.T) {
	wantErr := errors.New("permission denied")
	_, err := resolveFDTargetsFrom(func() (io.ReadCloser, error) { return nil, wantErr })
	if err == nil {
		t.Fatal("resolveFDTargetsFrom: want error when the symbol table cannot be opened")
	}
	if !errors.Is(err, wantErr) {
		t.Fatalf("resolveFDTargetsFrom error = %v, want it to wrap %v", err, wantErr)
	}
}

func TestResolveFDTargetsFromReadFailure(t *testing.T) {
	_, err := resolveFDTargetsFromReader(&errReader{prefix: "ffffffff81000000 T other\n"})
	if err == nil {
		t.Fatal("resolveFDTargetsFromReader: want error on a failed read, not 'target not found'")
	}
}

// TestFDCandidateOrder pins the preference order itself: the shipped BPF objects
// trace the newest symbol, so degrading to the older one when both exist would
// attach the wrong probe.
func TestFDCandidateOrder(t *testing.T) {
	if fdOpenCandidates[0] != "do_sys_openat2" {
		t.Errorf("fdOpenCandidates = %v, want do_sys_openat2 first", fdOpenCandidates)
	}
	if fdCloseCandidates[0] != "close_fd" {
		t.Errorf("fdCloseCandidates = %v, want close_fd first", fdCloseCandidates)
	}
}
