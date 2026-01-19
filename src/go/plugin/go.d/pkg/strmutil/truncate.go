// SPDX-License-Identifier: GPL-3.0-or-later

// Package strmutil provides string manipulation utilities.
package strmutil

import "unicode/utf8"

// TruncateText limits text length to maxLen characters.
// UTF-8 safe - does not split multi-byte characters.
// Appends "..." when truncation occurs.
func TruncateText(text string, maxLen int) string {
	if len(text) <= maxLen {
		return text
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
