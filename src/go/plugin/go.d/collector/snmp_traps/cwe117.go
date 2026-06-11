// SPDX-License-Identifier: GPL-3.0-or-later

package snmp_traps

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

func journalFieldNeedsBinaryString(value string) bool {
	return journalFieldNeedsBinary([]byte(value))
}

func binaryEncodedFieldCount(fields []JournalField) int {
	count := 0
	for _, f := range fields {
		if journalFieldCountsAsBinaryEncoded(f.Name, f.Value) {
			count++
		}
	}
	return count
}

func journalFieldCountsAsBinaryEncoded(name string, value []byte) bool {
	if name == "MESSAGE" {
		return false
	}
	return journalFieldNeedsBinary(value)
}
