// SPDX-License-Identifier: GPL-3.0-or-later

package beacon

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
		if got := RedactPath(in, custom); got != want {
			t.Errorf("RedactPath(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestRedactURL(t *testing.T) {
	if got := RedactURL("https://api.example.com/users/jane@example.com", nil); got != "https://api.example.com/users/:email" {
		t.Fatalf("got %q", got)
	}
	if got := RedactURL("https://api.example.com", nil); got != "https://api.example.com" {
		t.Fatalf("host only: %q", got)
	}
}

func TestRedactSelector(t *testing.T) {
	if got := RedactSelector("div#row-jane@example.com > button.buy-1234567"); got != "div#:email > button.buy-:id" {
		t.Fatalf("got %q", got)
	}
	if got := RedactSelector("#hero img"); got != "#hero img" {
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
		t.Run(raw, func(t *testing.T) { assert.Equal(t, want, RedactURL(raw, nil)) })
	}
}

func TestRedactSourceURLKeepsOnlyScriptBasename(t *testing.T) {
	const chunk = "framework-2c79e2a64abdb08b.js"
	const raw = "https://user:password@example.org/eyJhbGciOiJIUzI1NiJ9/" + chunk + "?secret=private#fragment"
	assert.Equal(t, "https://example.org/:token/:token", RedactURL(raw, nil))
	assert.Equal(t, "https://example.org/:token/"+chunk, RedactSourceURL(raw, nil))
	assert.Equal(t, "https://example.org/:token/:file", RedactSourceURL(raw, []PathRule{{Re: regexp.MustCompile(regexp.QuoteMeta(chunk)), Replace: ":file"}}))
	assert.Equal(t, "https://example.org/:email", RedactSourceURL("https://example.org/person@example.org.js", nil))
	assert.Equal(t, "https://example.org/:id", RedactSourceURL("https://example.org/123e4567-e89b-12d3-a456-426614174000", nil))
	assert.Equal(t, "https://example.org/:token", RedactSourceURL("https://example.org/eyJhbGciOiJIUzI1NiJ9.txt", nil))
}
