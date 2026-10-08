// SPDX-License-Identifier: GPL-3.0-or-later

package fairqueue

// Select returns at most limit non-active keys, scanning from the persisted
// cursor. The returned cursor resumes after the last inspected position.
func Select(keys []string, active string, cursor, limit int) ([]string, int) {
	if len(keys) == 0 || limit <= 0 {
		return nil, 0
	}
	if cursor < 0 {
		cursor = 0
	}
	cursor %= len(keys)
	selected := make([]string, 0, limit)
	inspected := 0
	index := cursor
	for inspected < len(keys) && len(selected) < limit {
		if keys[index] != active {
			selected = append(selected, keys[index])
		}
		index = (index + 1) % len(keys)
		inspected++
	}
	return selected, index
}
