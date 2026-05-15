// SPDX-License-Identifier: GPL-3.0-or-later

package secretstore

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestStoreKindIsValid(t *testing.T) {
	tests := map[string]struct {
		kind StoreKind
		want bool
	}{
		"vault":    {kind: KindVault, want: true},
		"aws-sm":   {kind: KindAWSSM, want: true},
		"azure-kv": {kind: KindAzureKV, want: true},
		"gcp-sm":   {kind: KindGCPSM, want: true},
		"empty":    {kind: "", want: false},
		"unknown":  {kind: "foobar", want: false},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, tc.want, tc.kind.IsValid())
		})
	}
}
