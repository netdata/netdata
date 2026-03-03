// SPDX-License-Identifier: GPL-3.0-or-later

package ids

import (
	"crypto/sha1"
	"fmt"
	"strings"
)

// Sanitize converts a job name into a lowercase alphanumeric identifier with underscores.
func Sanitize(name string) string {
	lower := strings.ToLower(name)
	var b strings.Builder
	b.Grow(len(lower))
	lastUnderscore := false
	hasAlnum := false
	for _, r := range lower {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			lastUnderscore = false
			hasAlnum = true
			continue
		}
		if r == '_' || r == '-' || isWhitespace(r) {
			if !lastUnderscore {
				b.WriteRune('_')
				lastUnderscore = true
			}
			continue
		}
		if !lastUnderscore {
			b.WriteRune('_')
			lastUnderscore = true
		}
	}
	result := b.String()
	if hasAlnum && result != "" {
		return result
	}
	sum := sha1.Sum([]byte(name))
	return fmt.Sprintf("id_%x", sum[:6])
}

func isWhitespace(r rune) bool {
	switch r {
	case ' ', '\t', '\n', '\r':
		return true
	}
	return false
}
