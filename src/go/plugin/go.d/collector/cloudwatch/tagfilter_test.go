// SPDX-License-Identifier: GPL-3.0-or-later

package cloudwatch

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestResourceMatchesFilters_ExactANDOR(t *testing.T) {
	filters := compileResourceTagFilters([]ResourceTagFilterConfig{
		{Key: "team", Values: []string{"sre", "platform"}},
		{Key: "environment", Values: []string{"production"}},
	})

	assert.True(t, resourceMatchesFilters(map[string]string{"team": "sre", "environment": "production"}, filters))
	assert.True(t, resourceMatchesFilters(map[string]string{"team": "platform", "environment": "production"}, filters))
	assert.False(t, resourceMatchesFilters(map[string]string{"team": "SRE", "environment": "production"}, filters), "values are case-sensitive")
	assert.False(t, resourceMatchesFilters(map[string]string{"team": "sre"}, filters), "every key is required")
	assert.False(t, resourceMatchesFilters(map[string]string{"team": "sre", "environment": "production-east"}, filters), "values are exact")
}

func TestResourceTagFilterSignature_CanonicalAndCollisionSafe(t *testing.T) {
	first := compileResourceTagFilters([]ResourceTagFilterConfig{
		{Key: "team", Values: []string{"sre", "platform"}},
		{Key: "environment", Values: []string{"production"}},
	})
	second := compileResourceTagFilters([]ResourceTagFilterConfig{
		{Key: "environment", Values: []string{"production"}},
		{Key: "team", Values: []string{"platform", "sre"}},
	})

	assert.Equal(t, resourceTagFilterSignature(first), resourceTagFilterSignature(second))
	assert.NotEqual(t,
		resourceTagFilterSignature([]resourceTagFilter{{key: "a", values: []string{"bc"}}}),
		resourceTagFilterSignature([]resourceTagFilter{{key: "ab", values: []string{"c"}}}),
	)
}
