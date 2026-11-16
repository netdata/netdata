// SPDX-License-Identifier: GPL-3.0-or-later

package policy

import (
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/framework/confgroup"
	"github.com/stretchr/testify/require"
)

func TestSecretReferencesAllowed(t *testing.T) {
	tests := map[string]struct {
		sourceType string
		stamp      any
		want       bool
	}{
		"stock":                {sourceType: confgroup.TypeStock, want: true},
		"user":                 {sourceType: confgroup.TypeUser, want: true},
		"dyncfg":               {sourceType: confgroup.TypeDyncfg, want: true},
		"discovered":           {sourceType: confgroup.TypeDiscovered},
		"trusted discovered":   {sourceType: confgroup.TypeDiscovered, stamp: true, want: true},
		"untrusted discovered": {sourceType: confgroup.TypeDiscovered, stamp: false},
		"string stamp":         {sourceType: confgroup.TypeDiscovered, stamp: "true"},
		"numeric stamp":        {sourceType: confgroup.TypeDiscovered, stamp: 1},
		"unknown stamped":      {sourceType: "future-source", stamp: true},
		"empty stamped":        {stamp: true},
		"empty":                {},
		"unknown":              {sourceType: "future-source"},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			config := confgroup.Config{"trust_discovered_targets": true, "__trust_discovered_targets__": test.stamp}.
				SetSourceType(test.sourceType)
			require.Equal(t, test.want, SecretReferencesAllowed(config))
		})
	}
}
