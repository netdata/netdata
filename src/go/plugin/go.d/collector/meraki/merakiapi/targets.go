// SPDX-License-Identifier: GPL-3.0-or-later

package merakiapi

import (
	"errors"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

func validBaseURL(u *url.URL) bool {
	return u != nil && u.Scheme == "https" && u.Hostname() != "" && u.User == nil &&
		u.Opaque == "" && u.RawQuery == "" && !u.ForceQuery && u.Fragment == "" &&
		strings.TrimRight(u.EscapedPath(), "/") == "/api/v1"
}

func merakiRegion(u *url.URL) string {
	if effectivePort(u) != "443" {
		return ""
	}
	switch strings.ToLower(u.Hostname()) {
	case "api.meraki.com":
		return "meraki.com"
	case "api.meraki.ca":
		return "meraki.ca"
	case "api.meraki.cn":
		return "meraki.cn"
	case "api.meraki.in":
		return "meraki.in"
	case "api.gov-meraki.com":
		return "gov-meraki.com"
	}
	return ""
}

func effectivePort(u *url.URL) string {
	if port := u.Port(); port != "" {
		return port
	}
	return "443"
}

func sameOrigin(a, b *url.URL) bool {
	return a.Scheme == b.Scheme && strings.EqualFold(a.Hostname(), b.Hostname()) &&
		effectivePort(a) == effectivePort(b)
}

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
	// Cisco documents n23.meraki.com and n23.dashboard.meraki.com. This is
	// deliberately narrower than trusting every subdomain of a Meraki root.
	for _, suffix := range []string{"." + c.regionRoot, ".dashboard." + c.regionRoot} {
		prefix, found := strings.CutSuffix(host, suffix)
		if !found || len(prefix) < 2 || prefix[0] != 'n' {
			continue
		}
		if strings.IndexFunc(prefix[1:], func(r rune) bool { return r < '0' || r > '9' }) == -1 {
			return true
		}
	}
	return false
}

func (c *Client) pageKey(u *url.URL, operationPath string, maxPageSize int) (string, error) {
	if !c.trustedTarget(u) || u.EscapedPath() != operationPath {
		return "", errors.New("unsafe pagination scope")
	}
	query, err := url.ParseQuery(u.RawQuery)
	if err != nil {
		return "", errors.New("invalid pagination query")
	}
	for name, values := range query {
		if len(values) != 1 {
			return "", errors.New("ambiguous pagination query")
		}
		switch name {
		case "startingAfter", "endingBefore":
		case "perPage":
			n, err := strconv.Atoi(values[0])
			if err != nil || n < 3 || n > maxPageSize {
				return "", errors.New("invalid pagination page size")
			}
			query.Set(name, strconv.Itoa(n))
		default:
			return "", errors.New("unexpected pagination query parameter")
		}
	}
	host := net.JoinHostPort(strings.ToLower(u.Hostname()), effectivePort(u))
	return u.Scheme + "://" + host + operationPath + "?" + query.Encode(), nil
}

func (c *Client) checkRedirect(req *http.Request, via []*http.Request) error {
	// Installing CheckRedirect replaces Go's default ten-hop check.
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
	// Go strips Authorization between sibling hosts. Rebuild it only after
	// validating the target, including a fresh credential-file read if configured.
	authenticated, err := c.newRequest(req.Context(), req.URL)
	if err != nil {
		return safeError("Meraki redirect", "cannot prepare authentication", err)
	}
	req.Header = authenticated.Header
	return nil
}
