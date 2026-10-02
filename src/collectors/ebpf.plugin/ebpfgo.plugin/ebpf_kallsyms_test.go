package main

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestOpenProcKallsymsPath(t *testing.T) {
	original := openKallsymsFile
	t.Cleanup(func() { openKallsymsFile = original })

	tests := map[string]struct {
		prefix string
		want   string
	}{
		"without host prefix": {want: "/proc/kallsyms"},
		"with host prefix": {
			prefix: t.TempDir(),
			want:   "", // set below to keep the fixture path local to the test
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			if tc.prefix != "" {
				tc.want = filepath.Join(tc.prefix, "/proc/kallsyms")
			}
			t.Setenv("NETDATA_HOST_PREFIX", tc.prefix)

			var got string
			openKallsymsFile = func(path string) (*os.File, error) {
				got = path
				return nil, errors.New("test open")
			}
			if _, err := openProcKallsyms(); err == nil {
				t.Fatal("openProcKallsyms() succeeded, want test open error")
			}
			if got != tc.want {
				t.Fatalf("openProcKallsyms() opened %q, want %q", got, tc.want)
			}
		})
	}
}

func TestIsProbeableKallsymsType(t *testing.T) {
	cases := map[string]bool{
		"T": true,
		"t": true,
		"W": true,
		"w": true,
		"D": false,
		"d": false,
		"R": false,
		"r": false,
		"B": false,
		"":  false,
	}

	for value, want := range cases {
		if got := isProbeableKallsymsType(value); got != want {
			t.Fatalf("isProbeableKallsymsType(%q) = %v, want %v", value, got, want)
		}
	}
}

func TestSelectKallsymsCandidate(t *testing.T) {
	cases := map[string]struct {
		candidates []string
		kallsyms   string
		want       string
		wantErr    bool
	}{
		// The whole table is scanned before choosing, so a lower-priority
		// candidate appearing at a lower address never shadows a higher-priority
		// one.  This is the property the fd open/close lists depend on.
		"first candidate wins regardless of table order": {
			candidates: []string{"do_sys_openat2", "do_sys_open"},
			kallsyms: `
ffffffff81000000 T do_sys_open
ffffffff81000010 T do_sys_openat2
`,
			want: "do_sys_openat2",
		},
		"falls through to the next candidate": {
			candidates: []string{"close_fd", "__close_fd"},
			kallsyms:   "ffffffff81000000 T __close_fd\n",
			want:       "__close_fd",
		},
		"non-probeable types do not count as present": {
			candidates: []string{"close_fd", "__close_fd"},
			kallsyms: `
ffffffff81000000 d close_fd
ffffffff81000010 T __close_fd
`,
			want: "__close_fd",
		},
		"short lines are ignored": {
			candidates: []string{"close_fd"},
			kallsyms: `
ffffffff81000000
ffffffff81000010 T
ffffffff81000020 T close_fd
`,
			want: "close_fd",
		},
		"no match returns empty without error": {
			candidates: []string{"close_fd", "__close_fd"},
			kallsyms:   "ffffffff81000000 T something_else\n",
			want:       "",
		},
		"empty candidate names are ignored": {
			candidates: []string{"", "close_fd"},
			kallsyms:   "ffffffff81000000 T close_fd\n",
			want:       "close_fd",
		},
		"no candidates is an error": {
			candidates: nil,
			kallsyms:   "ffffffff81000000 T close_fd\n",
			wantErr:    true,
		},
		"only empty candidates is an error": {
			candidates: []string{"", ""},
			kallsyms:   "ffffffff81000000 T close_fd\n",
			wantErr:    true,
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			got, err := selectKallsymsCandidate(tc.candidates, strings.NewReader(tc.kallsyms))
			if tc.wantErr {
				if err == nil {
					t.Fatalf("selectKallsymsCandidate() = %q, want error", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("selectKallsymsCandidate: %v", err)
			}
			if got != tc.want {
				t.Fatalf("selectKallsymsCandidate() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestSelectKallsymsPrefix(t *testing.T) {
	cases := map[string]struct {
		prefix   string
		kallsyms string
		want     string
		wantErr  bool
	}{
		"exact name": {
			prefix:   "lookup_fast",
			kallsyms: "ffffffff81000000 t lookup_fast\n",
			want:     "lookup_fast",
		},
		"compiler suffix": {
			prefix:   "lookup_fast",
			kallsyms: "ffffffff81000000 t lookup_fast.isra.0\n",
			want:     "lookup_fast.isra.0",
		},
		"non-probeable types are skipped": {
			prefix: "lookup_fast",
			kallsyms: `
ffffffff81000000 d lookup_fast.cold
ffffffff81000010 t lookup_fast.constprop.0
`,
			want: "lookup_fast.constprop.0",
		},
		"no match returns empty without error": {
			prefix:   "lookup_fast",
			kallsyms: "ffffffff81000000 T d_lookup\n",
			want:     "",
		},
		"empty prefix is an error": {
			prefix:   "",
			kallsyms: "ffffffff81000000 t lookup_fast\n",
			wantErr:  true,
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			got, err := selectKallsymsPrefix(tc.prefix, strings.NewReader(tc.kallsyms))
			if tc.wantErr {
				if err == nil {
					t.Fatalf("selectKallsymsPrefix() = %q, want error", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("selectKallsymsPrefix: %v", err)
			}
			if got != tc.want {
				t.Fatalf("selectKallsymsPrefix() = %q, want %q", got, tc.want)
			}
		})
	}
}

// errReader fails partway through so the scanner-error path is exercised: a
// truncated /proc read must surface as an error, not as "symbol not found".
type errReader struct{ prefix string }

func (r *errReader) Read(p []byte) (int, error) {
	if r.prefix != "" {
		n := copy(p, r.prefix)
		r.prefix = r.prefix[n:]
		return n, nil
	}
	return 0, errors.New("read failed")
}

func TestScanKallsymsPropagatesReadErrors(t *testing.T) {
	if _, err := selectKallsymsPrefix("lookup_fast", &errReader{prefix: "ffffffff81000000 T other\n"}); err == nil {
		t.Fatal("selectKallsymsPrefix: want read error")
	}

	if _, err := selectKallsymsCandidate([]string{"close_fd"}, &errReader{prefix: "ffffffff81000000 T other\n"}); err == nil {
		t.Fatal("selectKallsymsCandidate: want read error")
	}
}

func TestOpenProcKallsymsReturnsReadCloser(t *testing.T) {
	// Guards the seam's contract rather than the host's /proc contents: callers
	// defer Close() on the returned value, so it must be an io.ReadCloser.
	var open kallsymsOpener = openProcKallsyms
	symbols, err := open()
	if err != nil {
		t.Skipf("cannot read /proc/kallsyms on this host: %v", err)
	}
	defer symbols.Close()

	var _ io.ReadCloser = symbols
}
