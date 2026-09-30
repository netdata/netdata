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
		want       bool
	}{
		"stock":      {sourceType: confgroup.TypeStock, want: true},
		"user":       {sourceType: confgroup.TypeUser, want: true},
		"dyncfg":     {sourceType: confgroup.TypeDyncfg, want: true},
		"discovered": {sourceType: confgroup.TypeDiscovered},
		"empty":      {},
		"unknown":    {sourceType: "future-source"},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			require.Equal(t, test.want, SecretReferencesAllowed(test.sourceType))
		})
	}
}
