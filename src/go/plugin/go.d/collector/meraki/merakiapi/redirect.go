// SPDX-License-Identifier: GPL-3.0-or-later

package merakiapi

import (
	"errors"
	"net/http"
	"net/url"
	"strings"
)

// Meraki can redirect a GET to a regional shard host and requires authentication
// there too. Go drops Authorization when a redirect leaves the original host, so
// the client authenticates again, but only after checking that the destination
// is a Meraki host it trusts. Pagination links pass the same trust check.

// regionalRoots maps the official API hosts to their regional root domains.
var regionalRoots = map[string]string{
	"api.meraki.com":     "meraki.com",
	"api.meraki.ca":      "meraki.ca",
	"api.meraki.cn":      "meraki.cn",
	"api.meraki.in":      "meraki.in",
	"api.gov-meraki.com": "gov-meraki.com",
}

// merakiRegion returns the regional root of an official API base URL. Any other
// origin, such as a proxy or private endpoint, returns "" and trusts only itself.
func merakiRegion(u *url.URL) string {
	if effectivePort(u) != "443" {
		return ""
	}
	return regionalRoots[strings.ToLower(u.Hostname())]
}

// trustedTarget reports whether credentials may be sent to u: the configured
// origin, or for an official regional base, that region's API and shard hosts.
func (c *Client) trustedTarget(u *url.URL) bool {
	if u.Scheme != "https" || u.Hostname() == "" || u.User != nil || u.Opaque != "" || u.Fragment != "" {
		return false
	}
	if sameOrigin(u, c.baseURL) {
		return true
	}
	if c.regionRoot == "" || effectivePort(u) != "443" {
		return false
	}
	host := strings.ToLower(u.Hostname())
	if host == "api."+c.regionRoot {
		return true
	}
	// Cisco documents shard hosts such as n23.meraki.com and
	// n23.dashboard.meraki.com. Accept only that numeric form, not every
	// subdomain of the regional root.
	for _, suffix := range []string{"." + c.regionRoot, ".dashboard." + c.regionRoot} {
		if prefix, ok := strings.CutSuffix(host, suffix); ok && isShardName(prefix) {
			return true
		}
	}
	return false
}

// isShardName reports whether s is "n" followed by one or more digits.
func isShardName(s string) bool {
	digits, ok := strings.CutPrefix(s, "n")
	return ok && digits != "" && strings.Trim(digits, "0123456789") == ""
}

func sameOrigin(a, b *url.URL) bool {
	return a.Scheme == b.Scheme && strings.EqualFold(a.Hostname(), b.Hostname()) &&
		effectivePort(a) == effectivePort(b)
}

func effectivePort(u *url.URL) string {
	if port := u.Port(); port != "" {
		return port
	}
	return "443"
}

// checkRedirect follows a redirect only to a trusted host for the same operation
// and query, and builds fresh authentication for it.
func (c *Client) checkRedirect(req *http.Request, via []*http.Request) error {
	// Setting CheckRedirect replaces Go's default ten-hop limit; keep that limit.
	if len(via) == 0 || len(via) >= 10 || req.Method != http.MethodGet || !c.trustedTarget(req.URL) {
		return errors.New("Meraki redirect is not permitted")
	}
	initial := via[0].URL
	query, err := url.ParseQuery(req.URL.RawQuery)
	initialQuery, initialErr := url.ParseQuery(initial.RawQuery)
	if err != nil || initialErr != nil || req.URL.EscapedPath() != initial.EscapedPath() ||
		query.Encode() != initialQuery.Encode() {
		return errors.New("Meraki redirect changed the operation")
	}
	authenticated, err := c.newRequest(req.Context(), req.URL)
	if err != nil {
		return safeError("Meraki redirect", "cannot prepare authentication", err)
	}
	req.Header = authenticated.Header
	return nil
}
