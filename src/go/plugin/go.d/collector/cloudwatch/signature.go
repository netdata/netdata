// SPDX-License-Identifier: GPL-3.0-or-later

package cloudwatch

import (
	"strconv"
	"strings"
)

// writeLengthPrefixed appends one collision-safe string component to a signature.
func writeLengthPrefixed(b *strings.Builder, value string) {
	b.WriteString(strconv.Itoa(len(value)))
	b.WriteByte(':')
	b.WriteString(value)
}
