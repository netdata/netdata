// SPDX-License-Identifier: GPL-3.0-or-later

package lifecycle

func checkedSum[T ~int | ~int64](maximum T, values ...T) (T, bool) {
	var total T
	for _, value := range values {
		if value < 0 || total > maximum-value {
			return 0, false
		}
		total += value
	}
	return total, true
}
