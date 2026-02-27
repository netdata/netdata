package ids

import (
	"strings"
	"testing"
)

func TestSanitizePreservesBoundaryUnderscores(t *testing.T) {
	cases := []struct {
		name   string
		input  string
		expect string
	}{
		{name: "plain", input: "Disk usage", expect: "disk_usage"},
		{name: "trailing punctuation", input: "Disk usage /", expect: "disk_usage_"},
	}

	for _, tc := range cases {
		if got := Sanitize(tc.input); got != tc.expect {
			t.Fatalf("%s: expected %q, got %q", tc.name, tc.expect, got)
		}
	}
}

func TestSanitizeFallsBackForNonAlnum(t *testing.T) {
	got := Sanitize("///")
	if !strings.HasPrefix(got, "id_") {
		t.Fatalf("expected fallback id_*, got %q", got)
	}
}
