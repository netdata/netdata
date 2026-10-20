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

	tests := map[string]struct {
		tags map[string]string
		want bool
	}{
		"first OR value":       {tags: map[string]string{"team": "sre", "environment": "production"}, want: true},
		"second OR value":      {tags: map[string]string{"team": "platform", "environment": "production"}, want: true},
		"case-sensitive value": {tags: map[string]string{"team": "SRE", "environment": "production"}},
		"every key required":   {tags: map[string]string{"team": "sre"}},
		"exact value required": {tags: map[string]string{"team": "sre", "environment": "production-east"}},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, tc.want, resourceMatchesFilters(tc.tags, filters))
		})
	}
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
