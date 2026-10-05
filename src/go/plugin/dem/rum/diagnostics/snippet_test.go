// SPDX-License-Identifier: GPL-3.0-or-later

package diagnostics

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/config"
)

const collectorBase = "https://abc123.collectors.example.net"

func TestCheckSnippet(t *testing.T) {
	tag := `<script async src="` + collectorBase + `/rum/shop.js"></script>`
	for name, tc := range map[string]struct {
		status int
		csp    string
		body   string
		want   string
		detail string
	}{
		"found, no policy":           {200, "", "<head>" + tag + "</head>", SnippetFound, ""},
		"found, policy allows":       {200, "default-src 'self'; script-src 'self' https://cdn.jsdelivr.net *.collectors.example.net; connect-src " + collectorBase, tag, SnippetFound, ""},
		"missing":                    {200, "", "<head></head>", SnippetMissing, "not in the HTML"},
		"connect-src blocks":         {200, "script-src * ; connect-src 'self'", tag, SnippetCSPBlocked, "connect-src does not allow " + collectorBase},
		"sdk blocked by default":     {200, "default-src 'self' " + collectorBase, tag, SnippetCSPBlocked, "script-src does not allow https://cdn.jsdelivr.net"},
		"scheme source allows":       {200, "default-src https:", tag, SnippetFound, ""},
		"meta policy blocks":         {200, "", `<meta http-equiv="Content-Security-Policy" content="connect-src 'self'">` + tag, SnippetCSPBlocked, "connect-src"},
		"home page errors":           {500, "", "oops", SnippetUnreachable, "HTTP 500"},
		"wildcard needs a subdomain": {200, "default-src https://cdn.jsdelivr.net *.example.org", tag, SnippetCSPBlocked, "script-src does not allow " + collectorBase},
	} {
		t.Run(name, func(t *testing.T) {
			site := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if tc.csp != "" {
					w.Header().Set("Content-Security-Policy", tc.csp)
				}
				w.WriteHeader(tc.status)
				_, _ = io.WriteString(w, tc.body)
			}))
			defer site.Close()
			got := CheckSnippet(context.Background(), site.Client(),
				config.Site{
					Name:           "shop",
					AllowedOrigins: []string{site.URL},
				}, collectorBase)
			if got.State != tc.want || !strings.Contains(got.Detail, tc.detail) {
				t.Fatalf("got %s %q, want %s containing %q", got.State, got.Detail, tc.want, tc.detail)
			}
		})
	}
}

func TestCheckSnippetWithoutOrigins(t *testing.T) {
	if got := CheckSnippet(context.Background(), http.DefaultClient, config.Site{
		Name: "shop",
	}, collectorBase); got.State != SnippetUnchecked {
		t.Fatalf("state = %s", got.State)
	}
}

func TestHomePagePrefersHTTPS(t *testing.T) {
	if got := homePage([]string{"http://localhost:3000", "https://www.example.com"}); got != "https://www.example.com/" {
		t.Fatalf("home page = %q", got)
	}
}
