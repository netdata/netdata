// SPDX-License-Identifier: GPL-3.0-or-later

package cloudwatch

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestStructuralID_LengthPrefixAndDomainSeparation(t *testing.T) {
	base := structuralIDFromStrings("query", "a", "bc")
	assert.Equal(t, base, structuralIDFromStrings("query", "a", "bc"))
	assert.NotEqual(t, base, structuralIDFromStrings("query", "ab", "c"))
	assert.NotEqual(t, base, structuralIDFromStrings("billing", "a", "bc"))
}
