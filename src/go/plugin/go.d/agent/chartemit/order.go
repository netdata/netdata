// SPDX-License-Identifier: GPL-3.0-or-later

package chartemit

import "strings"

func makeDimensionOptions(hidden, obsolete bool) string {
	var parts []string
	if hidden {
		parts = append(parts, "hidden")
	}
	if obsolete {
		parts = append(parts, "obsolete")
	}
	return strings.Join(parts, " ")
}

func handleZero(v int) int {
	if v == 0 {
		return 1
	}
	return v
}
