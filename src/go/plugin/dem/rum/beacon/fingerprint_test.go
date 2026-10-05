// SPDX-License-Identifier: GPL-3.0-or-later

package beacon

import "testing"

func TestNormalizeMessage(t *testing.T) {
	tests := map[string]struct {
		in   string
		want string
	}{
		"decimal number":   {in: "timeout after 42 ms", want: "timeout after # ms"},
		"multiple numbers": {in: "item 1 of 200", want: "item # of #"},
		"hex id 6+":        {in: "object deadbe missing", want: "object # missing"},
		"hex shorter kept": {in: "code ab12", want: "code ab12"},
		"double quoted": {
			in:   `Cannot read property "foo" of undefined`,
			want: `Cannot read property "…" of undefined`,
		},
		"single quoted": {
			in:   "Cannot read property 'foo' of undefined",
			want: `Cannot read property "…" of undefined`,
		},
		"url": {
			in:   "failed to fetch https://api.example.com/v1/users/42?x=1",
			want: "failed to fetch URL",
		},
		"whitespace": {in: "a\n\tb   c", want: "a b c"},
		"combined":   {in: `GET "https://x/y/123" failed with code 500`, want: `GET "…" failed with code #`},
		"empty":      {in: "", want: ""},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			if got := NormalizeMessage(tc.in); got != tc.want {
				t.Fatalf("NormalizeMessage(%q) = %q want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestFingerprintStability(t *testing.T) {
	// Same shape, volatile details differ: must fingerprint identically.
	a := Fingerprint("TypeError", "Cannot read property 'x' of undefined (id 42)", "onClick@app.js")
	b := Fingerprint("TypeError", "Cannot read property 'x' of undefined (id 99)", "onClick@app.js")
	if a != b {
		t.Fatalf("expected stable fingerprint across volatile numbers: %q != %q", a, b)
	}
	if len(a) != fingerprintLen {
		t.Fatalf("fingerprint length = %d want %d", len(a), fingerprintLen)
	}
}

func TestFingerprintDistinguishesShape(t *testing.T) {
	tests := map[string]struct{ typ, msg, frame string }{
		"base":        {"TypeError", "x is undefined", "a@f.js"},
		"other type":  {"RangeError", "x is undefined", "a@f.js"},
		"other msg":   {"TypeError", "y is undefined", "a@f.js"},
		"other frame": {"TypeError", "x is undefined", "b@f.js"},
		"empty frame": {"TypeError", "x is undefined", ""},
	}
	base := Fingerprint(tests["base"].typ, tests["base"].msg, tests["base"].frame)
	for name, tc := range tests {
		if name == "base" {
			continue
		}
		if got := Fingerprint(tc.typ, tc.msg, tc.frame); got == base {
			t.Fatalf("%s: expected a different fingerprint from base, got the same %q", name, got)
		}
	}
}
