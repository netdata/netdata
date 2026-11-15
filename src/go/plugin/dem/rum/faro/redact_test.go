// SPDX-License-Identifier: GPL-3.0-or-later

package faro

import (
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRedactPath(t *testing.T) {
	custom := []PathRule{{Re: regexp.MustCompile(`^/u/[^/]+`), Replace: "/u/:user"}}
	for in, want := range map[string]string{
		"/users/jane@example.com/orders":               "/users/:email/orders",
		"/users/jane%40example.com":                    "/users/:email",
		"/orders/0d6c0e7a-3f3b-4e59-9f4e-8c2d8c1f2a11": "/orders/:id",
		"/account/1234567/settings":                    "/account/:id/settings",
		"/page/42":                                     "/page/42",
		"/reset/eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxIn0":  "/reset/:token",
		"/blog/top-10-tips-for-summer-2026":            "/blog/top-10-tips-for-summer-2026",
		"/u/jane/profile":                              "/u/:user/profile",
		"/":                                            "/",
	} {
		if got := redactPath(in, custom, false); got != want {
			t.Errorf("redactPath(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestRedactURL(t *testing.T) {
	var o Options
	assert.Equal(
		t,
		"https://api.example.com/users/:email",
		o.redactURL("https://api.example.com/users/jane@example.com"),
	)
	assert.Equal(t, "https://api.example.com", o.redactURL("https://api.example.com"), "host only")
}

// Removing control characters from a relative path must not turn it into a
// network-path reference, and escapes the redactor keeps stay escaped.
func TestRedactURLKeepsRelativeURLsHostless(t *testing.T) {
	var o Options
	for raw, want := range map[string]string{
		"/%2Fevil.com/Bearer%20abcdef.ghi":    "/%2Fevil.com/[REDACTED]",
		"/%09/tok%65n%3Dx":                    "/%2Ftok%65n%3D[REDACTED]",
		"https://example.org/%09/tok%65n%3Dx": "https://example.org//tok%65n%3D[REDACTED]",
		"a%3Ab/Bearer%20abcdef.ghi":           "a%3Ab/[REDACTED]",
	} {
		t.Run(raw, func(t *testing.T) { assert.Equal(t, want, o.redactURL(raw)) })
	}
}

// Site path rules run on the escaped path; text they insert is escaped before
// the decoded reading, so a rule cannot bypass it.
func TestRedactURLReadsSitePathRuleOutputDecoded(t *testing.T) {
	o := Options{
		PathRules: []PathRule{{Re: regexp.MustCompile(`_`), Replace: " "}},
	}
	assert.Equal(t, "https://example.org/a/[REDACTED]", o.redactURL("https://example.org/a/Bearer_abcdef.ghi"))
}

// Received escapes stay attached to the bytes they encode: a credential
// removed elsewhere in the path cannot lend its literal characters to retained
// text, and retained marker text is not a marker. cleanURL adds the final pass
// over the escaped URL.
func TestRedactURLKeepsEscapesOfRetainedBytes(t *testing.T) {
	var o Options
	for raw, want := range map[string]string{
		"https://example.org/k/-----BEGIN%20KEY-----/password=x&tail/z/-----END%20KEY-----/password=a%26tail/z/Bearer%20abcdef.ghi": "https://example.org/k/[REDACTED]/password=[REDACTED]",
		"https://example.org/%5BREDACTED%5D/x%00": "https://example.org/%5BREDACTED%5D/x",
	} {
		t.Run(raw, func(t *testing.T) { assert.Equal(t, want, o.cleanURL(raw)) })
	}
}

// A site path rule that inserts a malformed escape cannot stop the decoded
// reading of the rest of the path.
func TestRedactURLRepairsEscapesSitePathRulesBreak(t *testing.T) {
	o := Options{
		PathRules: []PathRule{{Re: regexp.MustCompile(`/sale/`), Replace: "/sale%/"}},
	}
	assert.Equal(
		t,
		"https://example.org/sale%25/[REDACTED]",
		o.cleanURL("https://example.org/sale/Bearer%20abcdef.ghi"),
	)
}

// Bytes a site path rule inserts that an escaped path cannot hold are escaped
// alone, so the rest of the path keeps its received escapes and the hostless
// guard. cleanURL adds the final pass over the escaped URL.
func TestRedactURLEscapesSitePathRuleBytesAlone(t *testing.T) {
	for name, tc := range map[string]struct {
		rule, replace string
		raw, want     string
	}{
		"space": {
			rule:    `_`,
			replace: " ",
			raw:     "https://example.org/a_b/password=a%26tail/z",
			want:    "https://example.org/a%20b/password=[REDACTED]",
		},
		"braces": {
			rule:    `/users/\d+`,
			replace: "/users/{id}",
			raw:     "https://example.org/users/42/password=a%26tail/z",
			want:    "https://example.org/users/%7Bid%7D/password=[REDACTED]",
		},
		"relative path joined by a removed control": {
			rule:    `_`,
			replace: " ",
			raw:     "/%00/evil.com/a_b",
			want:    "/%2Fevil.com/a%20b",
		},
	} {
		t.Run(name, func(t *testing.T) {
			o := Options{
				PathRules: []PathRule{{Re: regexp.MustCompile(tc.rule), Replace: tc.replace}},
			}
			assert.Equal(t, tc.want, o.cleanURL(tc.raw))
		})
	}
}

// Removing a control character can join invalid UTF-8 bytes into another
// control character; the decoded reading repeats until none is left.
func TestRedactURLRemovesControlsJoinedByRemoval(t *testing.T) {
	var o Options
	assert.Equal(t, "https://example.org/token=[REDACTED]", o.cleanURL("https://example.org/tok%C2%00%85en=hunter2"))
}

func TestRedactSelector(t *testing.T) {
	if got := redactSelector("div#row-jane@example.com > button.buy-1234567"); got != "div#:email > button.buy-:id" {
		t.Fatalf("got %q", got)
	}
	if got := redactSelector("#hero img"); got != "#hero img" {
		t.Fatalf("plain selector changed: %q", got)
	}
}

func TestRedactURLMalformedEscapes(t *testing.T) {
	for raw, want := range map[string]string{
		"https://user:p%ss@example.org/sale/50%-off?secret=private#fragment": "https://example.org/sale/50%25-off",
		"https://example.org/a%20b/50%?secret=private#bad%":                  "https://example.org/a%20b/50%25",
		"/sale/%ab/%x1/%1x/%1":                               "/sale/%ab/%25x1/%251x/%251",
		"https://example.org/users/person%40example.org/50%": "https://example.org/users/:email/50%25",
		"data:text/plain,50%":                                "",
		"https://example.org:bad/50%":                        "",
		"https://example.org/path\n50%":                      "",
	} {
		t.Run(raw, func(t *testing.T) { assert.Equal(t, want, (Options{}).redactURL(raw)) })
	}
}

func TestRedactSourceURLKeepsOnlyScriptBasename(t *testing.T) {
	const chunk = "framework-2c79e2a64abdb08b.js"
	const raw = "https://user:password@example.org/eyJhbGciOiJIUzI1NiJ9/" + chunk + "?secret=private#fragment"
	var o Options
	assert.Equal(t, "https://example.org/:token/:token", o.redactURL(raw))
	assert.Equal(t, "https://example.org/:token/"+chunk, o.redactSourceURL(raw))
	ruled := Options{
		PathRules: []PathRule{{Re: regexp.MustCompile(regexp.QuoteMeta(chunk)), Replace: ":file"}},
	}
	assert.Equal(t, "https://example.org/:token/:file", ruled.redactSourceURL(raw))
	assert.Equal(t, "https://example.org/:email", o.redactSourceURL("https://example.org/person@example.org.js"))
	assert.Equal(
		t,
		"https://example.org/:id",
		o.redactSourceURL("https://example.org/123e4567-e89b-12d3-a456-426614174000"),
	)
	assert.Equal(t, "https://example.org/:token", o.redactSourceURL("https://example.org/eyJhbGciOiJIUzI1NiJ9.txt"))
}
