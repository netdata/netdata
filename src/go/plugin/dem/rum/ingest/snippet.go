// SPDX-License-Identifier: GPL-3.0-or-later

package ingest

import (
	"context"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/dem/config"
)

// Snippet check states, for the Connect panel's checklist.
const (
	SnippetUnchecked   = "unchecked"   // no origin to fetch yet
	SnippetFound       = "found"       // tag present, CSP allows it
	SnippetMissing     = "missing"     // home page fetched, tag not in its HTML
	SnippetCSPBlocked  = "csp_blocked" // tag present, CSP stops the loader or its beacons
	SnippetUnreachable = "unreachable" // home page could not be fetched
)

// sdkOrigin serves the pinned Faro SDK the loader pulls in.
const sdkOrigin = "https://cdn.jsdelivr.net"

// SnippetCheck is the result of fetching a site's home page.
type SnippetCheck struct {
	State     string
	Detail    string
	Page      string
	CheckedAt time.Time
}

// CheckSnippet fetches the site's home page and reports whether the
// snippet is in its HTML and whether its Content-Security-Policy lets the
// loader, the SDK and the beacons through. Tag managers insert the tag
// after load, so "missing" is only a hint until a beacon arrives.
func CheckSnippet(ctx context.Context, client *http.Client, site config.RumSite, base string) SnippetCheck {
	page := homePage(site.AllowedOrigins)
	c := SnippetCheck{
		Page:      page,
		CheckedAt: time.Now(),
	}
	if page == "" {
		c.State, c.Detail = SnippetUnchecked, "no allowed origin to check"
		return c
	}
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, page, nil)
	if err != nil {
		c.State, c.Detail = SnippetUnreachable, err.Error()
		return c
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; NetdataRUMCheck/1.0; +https://www.netdata.cloud)")
	req.Header.Set("Accept", "text/html")
	resp, err := client.Do(req)
	if err != nil {
		c.State, c.Detail = SnippetUnreachable, err.Error()
		return c
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode >= 400 {
		c.State, c.Detail = SnippetUnreachable, fmt.Sprintf("%s answered HTTP %d", page, resp.StatusCode)
		return c
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		c.State, c.Detail = SnippetUnreachable, err.Error()
		return c
	}
	doc := string(body)
	if !strings.Contains(doc, "/rum/"+site.Key+".js") {
		c.State, c.Detail = SnippetMissing, "the snippet is not in the HTML of "+page
		return c
	}
	policies := resp.Header.Values("Content-Security-Policy")
	policies = append(policies, metaPolicies(doc)...)
	if problem := cspProblem(policies, page, strings.TrimRight(base, "/")); problem != "" {
		c.State, c.Detail = SnippetCSPBlocked, problem
		return c
	}
	c.State = SnippetFound
	return c
}

// homePage picks the origin to fetch: the first https one, else the first.
func homePage(origins []string) string {
	pick := ""
	for _, o := range origins {
		if strings.HasPrefix(o, "https://") {
			pick = o
			break
		}
		if pick == "" && strings.HasPrefix(o, "http") {
			pick = o
		}
	}
	if pick == "" {
		return ""
	}
	return strings.TrimRight(pick, "/") + "/"
}

var metaCSP = regexp.MustCompile(`(?is)<meta[^>]+http-equiv\s*=\s*["']?content-security-policy["']?[^>]*>`)
var metaContent = regexp.MustCompile(`(?is)content\s*=\s*("([^"]*)"|'([^']*)')`)

func metaPolicies(doc string) []string {
	var out []string
	for _, tag := range metaCSP.FindAllString(doc, -1) {
		if m := metaContent.FindStringSubmatch(tag); m != nil {
			out = append(out, html.UnescapeString(m[2]+m[3]))
		}
	}
	return out
}

// cspProblem returns what an enforced policy blocks, or "". Every policy
// must allow a request for the browser to make it.
func cspProblem(policies []string, page, collector string) string {
	for _, p := range policies {
		d := parseCSP(p)
		for _, need := range []struct{ directive, origin string }{
			{"script-src", collector},
			{"script-src", sdkOrigin},
			{"connect-src", collector},
		} {
			sources, ok := d[need.directive]
			if !ok {
				sources, ok = d["default-src"]
			}
			if ok && !cspAllows(sources, need.origin, page) {
				return fmt.Sprintf("%s does not allow %s", need.directive, need.origin)
			}
		}
	}
	return ""
}

func parseCSP(policy string) map[string][]string {
	out := map[string][]string{}
	for _, part := range strings.Split(policy, ";") {
		fields := strings.Fields(part)
		if len(fields) == 0 {
			continue
		}
		name := strings.ToLower(fields[0])
		if _, dup := out[name]; !dup {
			out[name] = fields[1:]
		}
	}
	return out
}

// cspAllows reports whether a source list lets the page load from origin.
func cspAllows(sources []string, origin, page string) bool {
	target, err := url.Parse(origin)
	if err != nil {
		return false
	}
	self, _ := url.Parse(page)
	for _, src := range sources {
		s := strings.ToLower(src)
		switch {
		case s == "*":
			return true
		case s == "'self'":
			if self != nil && self.Scheme == target.Scheme && self.Host == target.Host {
				return true
			}
		case s == target.Scheme+":":
			return true
		case strings.HasPrefix(s, "'"):
		default:
			if hostSourceMatches(s, target) {
				return true
			}
		}
	}
	return false
}

func hostSourceMatches(src string, target *url.URL) bool {
	scheme := ""
	if i := strings.Index(src, "://"); i >= 0 {
		scheme, src = src[:i], src[i+3:]
	}
	if i := strings.Index(src, "/"); i >= 0 {
		src = src[:i]
	}
	if scheme != "" && scheme != target.Scheme {
		return false
	}
	if scheme == "" && target.Scheme != "https" && target.Scheme != "http" {
		return false
	}
	host, port := src, ""
	if i := strings.LastIndex(src, ":"); i >= 0 {
		host, port = src[:i], src[i+1:]
	}
	if port != "" && port != "*" && port != target.Port() {
		return false
	}
	th := strings.ToLower(target.Hostname())
	if strings.HasPrefix(host, "*.") {
		return strings.HasSuffix(th, host[1:])
	}
	return th == host
}
