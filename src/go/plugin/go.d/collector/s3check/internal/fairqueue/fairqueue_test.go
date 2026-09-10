// SPDX-License-Identifier: GPL-3.0-or-later

package fairqueue

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSelectRotatesBoundedWorkAndSkipsActive(t *testing.T) {
	keys := []string{"blocked-a", "active", "blocked-b", "ready"}

	first, cursor := Select(keys, "active", 0, 2)
	second, cursor := Select(keys, "active", cursor, 2)
	third, _ := Select(keys, "active", cursor, 2)

	assert.Equal(t, []string{"blocked-a", "blocked-b"}, first)
	assert.Equal(t, []string{"ready", "blocked-a"}, second)
	assert.Equal(t, []string{"blocked-b", "ready"}, third)
}

func TestSelectHandlesEmptyAndOutOfRangeCursor(t *testing.T) {
	selected, cursor := Select(nil, "", 100, 2)
	assert.Empty(t, selected)
	assert.Zero(t, cursor)

	selected, cursor = Select([]string{"a"}, "", 100, 1)
	assert.Equal(t, []string{"a"}, selected)
	assert.Zero(t, cursor)
}
