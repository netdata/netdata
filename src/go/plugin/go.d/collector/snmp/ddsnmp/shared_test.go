// SPDX-License-Identifier: GPL-3.0-or-later

package ddsnmp

import (
	"fmt"
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSharedMappingsCompleteness(t *testing.T) {
	// Verify all ifType entries have a corresponding group
	missingGroups := []string{}

	for ifTypeID := range sharedMappings.ifType {
		if _, exists := sharedMappings.ifTypeGroup[ifTypeID]; !exists {
			missingGroups = append(missingGroups, fmt.Sprintf("%s (%s)", ifTypeID, sharedMappings.ifType[ifTypeID]))
		}
	}

	assert.Empty(t, missingGroups, "All interface types should have groups")
	if len(missingGroups) == 0 {
		t.Logf("✓ All %d interface types have groups", len(sharedMappings.ifType))
	}
}

func TestSharedMappingsCount(t *testing.T) {
	t.Logf("ifType entries: %d", len(sharedMappings.ifType))
	t.Logf("ifTypeGroup entries: %d", len(sharedMappings.ifTypeGroup))

	// Count items per group
	groups := make(map[string]int)
	for _, group := range sharedMappings.ifTypeGroup {
		groups[group]++
	}

	// Sort by count descending
	type groupCount struct {
		name  string
		count int
	}
	var sorted []groupCount
	for name, count := range groups {
		sorted = append(sorted, groupCount{name, count})
	}
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].count > sorted[j].count
	})

	t.Logf("\nGroup distribution (%d groups):", len(groups))
	for _, gc := range sorted {
		t.Logf("  %3d  %s", gc.count, gc.name)
	}

	// Verify group count doesn't exceed ifType count
	assert.LessOrEqual(t, len(sharedMappings.ifTypeGroup), len(sharedMappings.ifType),
		"ifTypeGroup should not have more entries than ifType")
}
