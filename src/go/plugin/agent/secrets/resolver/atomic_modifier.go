// SPDX-License-Identifier: GPL-3.0-or-later

package secretresolver

import "strings"

const modifierURIEncode = "urienc"

func percentEncodeURIUnreserved(value string) string {
	const upperhex = "0123456789ABCDEF"

	var builder strings.Builder
	builder.Grow(len(value))
	for index := 0; index < len(value); index++ {
		char := value[index]
		if isURIUnreserved(char) {
			builder.WriteByte(char)
			continue
		}
		builder.WriteByte('%')
		builder.WriteByte(upperhex[char>>4])
		builder.WriteByte(upperhex[char&0x0F])
	}
	return builder.String()
}

func isURIUnreserved(char byte) bool {
	return (char >= 'A' && char <= 'Z') ||
		(char >= 'a' && char <= 'z') ||
		(char >= '0' && char <= '9') ||
		char == '-' || char == '.' || char == '_' || char == '~'
}
