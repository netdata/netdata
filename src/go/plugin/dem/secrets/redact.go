// SPDX-License-Identifier: GPL-3.0-or-later

package secrets

import (
	"net/url"
	"regexp"
	"strings"
)

// Redactor removes secret values and common credential shapes from text
// before egress. It applies exact supplied values first, then pattern matches.
type Redactor struct {
	values     []string
	standalone *regexp.Regexp // whole match → [REDACTED]
	keyvalue   *regexp.Regexp // keep the key, redact the value
}

var standalonePattern = regexp.MustCompile(
	`(?i)(` +
		`bearer\s+[a-z0-9._~+/-]+=*` + // Bearer tokens
		`|akia[0-9a-z]{16}` + // AWS access keys
		`|-----begin [a-z ]+-----[\s\S]*?-----end [a-z ]+-----` + // PEM blocks
		`)`,
)

const credentialKeys = `api_key|apikey|access_token|client_secret|token|password|secret`

var keyValuePattern = regexp.MustCompile(`(?i)\b((?:` + credentialKeys + `))=[^&\s"']*`)
var queryKeyPattern = regexp.MustCompile(`(?i)^(?:` + credentialKeys + `)$`)

func NewRedactor(values ...string) *Redactor {
	return &Redactor{
		values:     append([]string(nil), values...),
		standalone: standalonePattern,
		keyvalue:   keyValuePattern,
	}
}

const redacted = "[REDACTED]"

// Apply redacts from a copy of s.
func (r *Redactor) Apply(s string) string {
	out := s
	for _, v := range r.values {
		if len(v) >= 4 { // avoid redacting trivially short values everywhere
			out = strings.ReplaceAll(out, v, redacted)
		}
	}
	out = r.standalone.ReplaceAllString(out, redacted)
	out = r.keyvalue.ReplaceAllString(out, "$1="+redacted)
	return out
}

// HeaderDenylist identifies sensitive headers for HAR and trace redaction.
var HeaderDenylist = map[string]bool{
	"authorization":       true,
	"cookie":              true,
	"set-cookie":          true,
	"proxy-authorization": true,
	"x-api-key":           true,
}

// ApplyURL recognizes query names after URL decoding, including repeated keys.
// Only the display/history target changes; navigation uses the original URL.
func (r *Redactor) ApplyURL(value string) string {
	u, err := url.Parse(value)
	if err != nil {
		return r.Apply(value)
	}
	parts := strings.Split(u.RawQuery, "&")
	for i, part := range parts {
		key, _, hasValue := strings.Cut(part, "=")
		decoded, err := url.QueryUnescape(key)
		if err == nil && hasValue && queryKeyPattern.MatchString(decoded) {
			parts[i] = key + "=" + url.QueryEscape(redacted)
		}
	}
	u.RawQuery = strings.Join(parts, "&")
	return r.Apply(u.String())
}
