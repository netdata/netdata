// SPDX-License-Identifier: GPL-3.0-or-later

package cloudwatch

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestStructuralID_LengthPrefixAndDomainSeparation(t *testing.T) {
	base := structuralIDFromStrings("query", "a", "bc")
	tests := map[string]struct {
		domain    string
		fields    []string
		wantEqual bool
	}{
		"stable":           {domain: "query", fields: []string{"a", "bc"}, wantEqual: true},
		"length-prefixed":  {domain: "query", fields: []string{"ab", "c"}},
		"domain-separated": {domain: "billing", fields: []string{"a", "bc"}},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got := structuralIDFromStrings(tc.domain, tc.fields...)
			if tc.wantEqual {
				assert.Equal(t, base, got)
			} else {
				assert.NotEqual(t, base, got)
			}
		})
	}
}
