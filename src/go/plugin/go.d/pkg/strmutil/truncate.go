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
	// UTF-8 safe truncation
	cutoff := 0
	ellipsis := "..."
	reserveForEllipsis := 3
	if maxLen < 3 {
		// Too small for ellipsis - just truncate without it
		ellipsis = ""
		reserveForEllipsis = 0
	}
	for i := 0; i < len(text); {
		_, size := utf8.DecodeRuneInString(text[i:])
		if cutoff+size > maxLen-reserveForEllipsis {
			break
		}
		cutoff += size
		i += size
	}
	return text[:cutoff] + ellipsis
}
