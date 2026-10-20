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
		// The case that motivated the fix: on a post-refactor kernel close_fd
		// still exists (so it resolves and attaches) but the syscall no longer
		// calls it.  The wrapper must win, or the close counters read zero.
		"syscall wrapper wins over the stale inner helper": {
			kallsyms: `
ffffffff81000000 T do_sys_openat2
ffffffff81000010 T close_fd
ffffffff81000020 T file_close_fd
ffffffff81000030 T __x64_sys_close
`,
			wantOpen:  "do_sys_openat2",
			wantClose: "__x64_sys_close",
		},
		"arm64 wrapper": {
			kallsyms: `
ffffffff81000000 T do_sys_openat2
ffffffff81000010 T close_fd
ffffffff81000020 T __arm64_sys_close
`,
			wantOpen:  "do_sys_openat2",
			wantClose: "__arm64_sys_close",
		},
		// powerpc and 32-bit arm have no CONFIG_ARCH_HAS_SYSCALL_WRAPPER, so the
		// syscall symbol is the unprefixed one.
		"unwrapped arch uses sys_close": {
			kallsyms: `
ffffffff81000000 T do_sys_openat2
ffffffff81000010 T close_fd
ffffffff81000020 T sys_close
`,
			wantOpen:  "do_sys_openat2",
			wantClose: "sys_close",
		},
		// Pre-wrapper kernel: the historical helpers are all that exist, and the
		// collector must still resolve rather than refuse to load.
		"pre-wrapper kernel falls back to close_fd": {
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
		// file_close_fd is deliberately NOT a candidate: it returns struct file *,
		// and the shipped BPF program classifies errors with `(int)ret < 0`, which
		// is meaningless on a pointer and never true for the NULL error case.
		"file_close_fd is never selected": {
			kallsyms: `
ffffffff81000000 T do_sys_openat2
ffffffff81000010 T file_close_fd
ffffffff81000020 T close_fd
`,
			wantOpen:  "do_sys_openat2",
			wantClose: "close_fd",
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
		"both-close-helpers-present": {
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

// TestFDCandidateOrder pins the preference order itself.  Open traces the inner
// function that every entry point funnels through; close traces the syscall
// wrapper, and the two stale inner helpers must remain strictly last so a
// post-refactor kernel can never select one of them.
func TestFDCandidateOrder(t *testing.T) {
	if fdOpenCandidates[0] != "do_sys_openat2" {
		t.Errorf("fdOpenCandidates = %v, want do_sys_openat2 first", fdOpenCandidates)
	}

	tail := fdCloseCandidates[len(fdCloseCandidates)-2:]
	if tail[0] != "close_fd" || tail[1] != "__close_fd" {
		t.Fatalf("fdCloseCandidates = %v, want it to end with close_fd, __close_fd", fdCloseCandidates)
	}

	for i, candidate := range fdCloseCandidates[:len(fdCloseCandidates)-2] {
		if candidate != "sys_close" && !strings.HasSuffix(candidate, "_sys_close") {
			t.Errorf("fdCloseCandidates[%d] = %q, want a syscall symbol before the inner helpers",
				i, candidate)
		}
	}

	// file_close_fd() is in the close(2) path but returns struct file *, which the
	// shipped program's `(int)ret < 0` error test cannot interpret.
	for _, candidate := range fdCloseCandidates {
		if candidate == "file_close_fd" {
			t.Error("file_close_fd must not be a candidate: its struct file * return breaks error counting")
		}
	}
}
