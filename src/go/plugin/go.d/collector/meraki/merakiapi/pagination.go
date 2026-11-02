// SPDX-License-Identifier: GPL-3.0-or-later

package merakiapi

import (
	"errors"
	"strings"
)

// Link values can contain commas inside URI references and quoted parameters.
// Parse the RFC5988 framing rather than splitting the header on commas.
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
				for nameEnd < len(rest) && tokenByte(rest[nameEnd]) {
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
					continue // RFC5988 permits extension parameters without a value.
				}
				value, remaining, err := linkValue(strings.TrimSpace(rest[1:]))
				if err != nil {
					return "", false, err
				}
				rest = strings.TrimSpace(remaining)
				hasAnchor = hasAnchor || strings.EqualFold(name, "anchor")
				if strings.EqualFold(name, "rel") && !hasRelation {
					// RFC5988 requires ignoring later occurrences of rel.
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

func linkValue(value string) (string, string, error) {
	if value == "" {
		return "", "", errors.New("empty Link parameter")
	}
	if value[0] != '"' {
		end := 0
		for end < len(value) && (tokenByte(value[end]) || strings.ContainsRune("()/:<=>?@[]{}", rune(value[end]))) {
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

func tokenByte(b byte) bool {
	return b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z' || b >= '0' && b <= '9' ||
		strings.ContainsRune("!#$%&'*+-.^_`|~", rune(b))
}
