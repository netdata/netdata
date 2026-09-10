package main

import (
	"strings"
	"testing"
)

func TestSelectCachestatAccountPageTargetFromReader(t *testing.T) {
	candidates := defaultCachestatTargets().AccountPage

	cases := map[string]struct {
		kallsyms string
		want     string
		wantErr  bool
	}{
		// Candidate order wins over symbol-table order.  The C module resolved
		// this by looping account_page[] and breaking on the first symbol that
		// exists (ebpf_cachestat_set_internal_value), so a kernel exporting both
		// must pick account_page_dirtied even though __set_page_dirty is listed
		// first in /proc/kallsyms.
		"candidate order beats table order": {
			kallsyms: `
ffffffff81000000 T __set_page_dirty
ffffffff81000001 T account_page_dirtied
`,
			want: "account_page_dirtied",
		},
		"only the folio symbol exists": {
			kallsyms: "ffffffff81000000 T __folio_mark_dirty\n",
			want:     "__folio_mark_dirty",
		},
		"only the legacy symbol exists": {
			kallsyms: "ffffffff81000000 T account_page_dirtied\n",
			want:     "account_page_dirtied",
		},
		"non-probeable symbol types are skipped": {
			kallsyms: `
ffffffff81000000 r __folio_mark_dirty
ffffffff81000001 T __set_page_dirty
`,
			want: "__set_page_dirty",
		},
		// Unlike dcstat, cachestat has no usable default: the target must resolve
		// or the module cannot load.
		"no candidate present is an error": {
			kallsyms: "ffffffff81000000 T some_other_symbol\n",
			wantErr:  true,
		},
		"empty symbol table is an error": {
			kallsyms: "",
			wantErr:  true,
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			got, err := selectCachestatAccountPageTargetFromReader(candidates, strings.NewReader(tc.kallsyms))
			if tc.wantErr {
				if err == nil {
					t.Fatalf("selectCachestatAccountPageTargetFromReader() = %q, want error", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("selectCachestatAccountPageTargetFromReader: %v", err)
			}
			if got != tc.want {
				t.Fatalf("selectCachestatAccountPageTargetFromReader() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestResolveCachestatAccountPageTargetFillsSlot(t *testing.T) {
	targets := defaultCachestatTargets()
	if targets.AccountPageDirtied.Name != "" {
		t.Fatalf("AccountPageDirtied ships pre-filled with %q; the resolve step is what fills it",
			targets.AccountPageDirtied.Name)
	}

	got, err := selectCachestatAccountPageTargetFromReader(
		targets.AccountPage, strings.NewReader("ffffffff81000000 T account_page_dirtied\n"))
	if err != nil {
		t.Fatalf("resolve account page target: %v", err)
	}
	targets.AccountPageDirtied.Name = got

	if targets.AccountPageDirtied.Name != "account_page_dirtied" {
		t.Fatalf("AccountPageDirtied.Name = %q, want %q", targets.AccountPageDirtied.Name, "account_page_dirtied")
	}
}
