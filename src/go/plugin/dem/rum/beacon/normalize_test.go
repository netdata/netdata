// SPDX-License-Identifier: GPL-3.0-or-later

package beacon

import (
	"strings"
	"testing"
)

func TestPath(t *testing.T) {
	tests := map[string]struct {
		in   string
		want string
	}{
		"absolute url with query and fragment": {in: "https://shop.example.com/pricing?utm=x#top", want: "/pricing"},
		"root":                                 {in: "https://shop.example.com/", want: "/"},
		"empty":                                {in: "", want: "/"},
		"host only":                            {in: "https://shop.example.com", want: "/"},
		"trailing slash trimmed":               {in: "https://x/a/b/", want: "/a/b"},
		"relative path with query":             {in: "/a?b=c", want: "/a"},
		"garbage without slash":                {in: "pricing?x=1", want: "/pricing"},
		"control chars removed":                {in: "https://x/a\x00b\n", want: "/ab"},
		"encoded path kept encoded":            {in: "https://x/caf%C3%A9", want: "/caf%C3%A9"},
		"long path capped": {
			in:   "https://x/" + strings.Repeat("a", 600),
			want: "/" + strings.Repeat("a", maxPathLen-1),
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			if got := Path(tc.in); got != tc.want {
				t.Fatalf("Path(%q) = %q want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestPageGroup(t *testing.T) {
	tests := map[string]struct {
		in   string
		want string
	}{
		"root":                     {in: "/", want: "/"},
		"empty":                    {in: "", want: "/"},
		"static path unchanged":    {in: "/pricing", want: "/pricing"},
		"numeric id":               {in: "/users/42", want: "/users/:id"},
		"numeric in the middle":    {in: "/users/42/orders", want: "/users/:id/orders"},
		"uuid":                     {in: "/orders/123e4567-e89b-12d3-a456-426614174000", want: "/orders/:id"},
		"uppercase uuid":           {in: "/orders/123E4567-E89B-12D3-A456-426614174000", want: "/orders/:id"},
		"hex 8+":                   {in: "/build/deadbeef01", want: "/build/:id"},
		"hex shorter than 8 kept":  {in: "/build/abc123", want: "/build/abc123"},
		"words with digits kept":   {in: "/v2/api/item42", want: "/v2/api/item42"},
		"multiple ids":             {in: "/a/1/b/2/c", want: "/a/:id/b/:id/c"},
		"all-hex word 8+ is an id": {in: "/deadbeef", want: "/:id"},
		"long group capped": {
			in:   "/" + strings.Repeat("z", 300),
			want: "/" + strings.Repeat("z", maxGroupLen-1),
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			if got := PageGroup(tc.in); got != tc.want {
				t.Fatalf("PageGroup(%q) = %q want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestClean(t *testing.T) {
	tests := map[string]struct {
		in   string
		max  int
		want string
	}{
		"trim and control chars": {in: "  Chr\x01ome\n", max: 10, want: "Chrome"},
		"cap":                    {in: "abcdefgh", max: 3, want: "abc"},
		"empty":                  {in: "", max: 3, want: ""},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			if got := Clean(tc.in, tc.max); got != tc.want {
				t.Fatalf("Clean(%q) = %q want %q", tc.in, got, tc.want)
			}
		})
	}
}
