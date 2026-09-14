// SPDX-License-Identifier: GPL-3.0-or-later

package journal

import "unicode/utf8"

func journalFieldNeedsBinary(value []byte) bool {
	if len(value) == 0 {
		return false
	}
	for _, b := range value {
		if b == '\n' || b == 0 || b == 0x7f {
			return true
		}
		if b < 0x20 && b != '\t' && b != ' ' {
			return true
		}
	}
	if !utf8.Valid(value) {
		return true
	}
	return false
}
