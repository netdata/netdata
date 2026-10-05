// SPDX-License-Identifier: GPL-3.0-or-later

package beacon

import (
	"regexp"
	"testing"
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
