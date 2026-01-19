// SPDX-License-Identifier: GPL-3.0-or-later

// Package strmutil provides string manipulation utilities.
package strmutil

import "unicode/utf8"

// TruncateText limits text length to maxLen bytes.
// UTF-8 safe - does not split multi-byte characters.
// Appends "..." when truncation occurs.
func TruncateText(text string, maxLen int) string {
	if maxLen <= 0 {
		return ""
	}
	if len(text) <= maxLen {
		return text
	}
	// Handle edge case where maxLen is too small for ellipsis
	if maxLen < 3 {
		return text[:maxLen]
	}
	// UTF-8 safe truncation
	cutoff := 0
	for i := 0; i < len(text); {
		_, size := utf8.DecodeRuneInString(text[i:])
		if cutoff+size > maxLen-3 {
			break
		}
		cutoff += size
		i += size
	}
	return text[:cutoff] + "..."
}
