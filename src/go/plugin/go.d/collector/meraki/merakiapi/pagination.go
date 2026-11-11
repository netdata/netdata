// SPDX-License-Identifier: GPL-3.0-or-later

package merakiapi

import (
	"errors"
	"maps"
	"net"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Meraki paginates with Link headers (RFC 8288): the server owns the cursor, and
// the last page is the one without rel="next". The client never builds a cursor
// itself or infers the end from a short page.

// pageURL checks that a page URL stays on a trusted host, on the operation's
// path, with only documented pagination parameters and an unchanged win. It
// returns the URL to request, with a canonical query that carries win even
// where the link left it out, and a canonical key of it to detect cycles.
func (c *Client) pageURL(u *url.URL, operationPath string, maxPageSize int, win *window) (*url.URL, string, error) {
	if !c.trustedTarget(u) || u.EscapedPath() != operationPath {
		return nil, "", errors.New("unsafe pagination scope")
	}
	query, err := url.ParseQuery(u.RawQuery)
	if err != nil {
		return nil, "", errors.New("invalid pagination query")
	}
	for name, values := range query {
		if len(values) != 1 {
			return nil, "", errors.New("ambiguous pagination query")
		}
		switch name {
		case "startingAfter", "endingBefore":
		case "perPage":
			n, err := strconv.Atoi(values[0])
			if err != nil || n < minPageSize || n > maxPageSize {
				return nil, "", errors.New("invalid pagination page size")
			}
			query.Set(name, strconv.Itoa(n))
		case "t0", "t1":
			// A link may spell the window's instants differently, but never move them.
			if win == nil || !sameInstant(values[0], win.instant(name)) {
				return nil, "", errors.New("changed pagination time window")
			}
		case "interval":
			// Meraki derives the interval from the window and can echo it. The
			// echo is dropped, so every page lets the same window decide it.
			if win == nil {
				return nil, "", errors.New("unexpected pagination query parameter")
			}
			query.Del(name)
		default:
			return nil, "", errors.New("unexpected pagination query parameter")
		}
	}
	if win != nil {
		maps.Copy(query, win.query())
	}
	canonical := *u
	canonical.RawQuery = query.Encode()
	host := net.JoinHostPort(strings.ToLower(u.Hostname()), effectivePort(u))
	return &canonical, u.Scheme + "://" + host + operationPath + "?" + canonical.RawQuery, nil
}

// instant returns the window's value of the t0 or t1 parameter.
func (w window) instant(param string) time.Time {
	if param == "t0" {
		return w.start
	}
	return w.end
}

// sameInstant reports whether value is an RFC 3339 spelling of t.
func sameInstant(value string, t time.Time) bool {
	parsed, err := time.Parse(time.RFC3339Nano, value)
	return err == nil && parsed.Equal(t)
}

// nextLink returns the rel="next" target from Link header values. Targets and
// quoted parameters can contain commas, so it parses the RFC 8288 framing instead
// of splitting on commas. Malformed or conflicting links are an error rather than
// an apparent last page.
func nextLink(headers []string) (string, bool, error) {
	var next string
	var found bool
	for _, header := range headers {
		for rest := strings.TrimSpace(header); rest != ""; {
			if rest[0] != '<' {
				return "", false, errors.New("invalid Link target")
			}
			end := strings.IndexByte(rest, '>')
			if end < 0 {
				return "", false, errors.New("unterminated Link target")
			}
			target := rest[1:end]
			rest = strings.TrimSpace(rest[end+1:])
			isNext := false
			hasRelation := false
			hasAnchor := false
			for rest != "" && rest[0] == ';' {
				rest = strings.TrimSpace(rest[1:])
				nameEnd := 0
				for nameEnd < len(rest) && isTokenChar(rest[nameEnd]) {
					nameEnd++
				}
				if nameEnd == 0 {
					return "", false, errors.New("invalid Link parameter")
				}
				name := rest[:nameEnd]
				rest = strings.TrimSpace(rest[nameEnd:])
				if rest == "" || rest[0] != '=' {
					if strings.EqualFold(name, "rel") || strings.EqualFold(name, "anchor") {
						return "", false, errors.New("missing Link parameter value")
					}
					continue // extension parameters may have no value
				}
				value, remaining, err := linkParamValue(strings.TrimSpace(rest[1:]))
				if err != nil {
					return "", false, err
				}
				rest = strings.TrimSpace(remaining)
				hasAnchor = hasAnchor || strings.EqualFold(name, "anchor")
				if strings.EqualFold(name, "rel") && !hasRelation {
					// RFC 8288: occurrences of rel after the first are ignored.
					hasRelation = true
					relations := strings.Fields(value)
					if len(relations) == 0 {
						return "", false, errors.New("empty Link relation")
					}
					for _, relation := range relations {
						isNext = isNext || strings.EqualFold(relation, "next")
					}
				}
			}
			if isNext {
				// An anchor changes the link context; it is not this page's next.
				if hasAnchor {
					return "", false, errors.New("unsupported pagination context")
				}
				if target == "" || found && target != next {
					return "", false, errors.New("ambiguous next Link")
				}
				next, found = target, true
			}
			if rest != "" {
				if rest[0] != ',' || strings.TrimSpace(rest[1:]) == "" {
					return "", false, errors.New("invalid Link separator")
				}
				rest = strings.TrimSpace(rest[1:])
			}
		}
	}
	return next, found, nil
}

// linkParamValue reads one token or quoted-string parameter value and returns it
// with the unparsed remainder.
func linkParamValue(value string) (string, string, error) {
	if value == "" {
		return "", "", errors.New("empty Link parameter")
	}
	if value[0] != '"' {
		end := 0
		for end < len(value) && (isTokenChar(value[end]) || strings.ContainsRune("()/:<=>?@[]{}", rune(value[end]))) {
			end++
		}
		if end == 0 {
			return "", "", errors.New("invalid Link value")
		}
		return value[:end], value[end:], nil
	}
	var text strings.Builder
	for i := 1; i < len(value); i++ {
		switch value[i] {
		case '"':
			return text.String(), value[i+1:], nil
		case '\\':
			i++
			if i == len(value) {
				return "", "", errors.New("unterminated quoted Link value")
			}
		}
		if value[i] < ' ' && value[i] != '\t' || value[i] == 127 {
			return "", "", errors.New("invalid quoted Link value")
		}
		text.WriteByte(value[i])
	}
	return "", "", errors.New("unterminated quoted Link value")
}

// isTokenChar reports whether b is an RFC 9110 token character.
func isTokenChar(b byte) bool {
	return b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z' || b >= '0' && b <= '9' ||
		strings.ContainsRune("!#$%&'*+-.^_`|~", rune(b))
}
