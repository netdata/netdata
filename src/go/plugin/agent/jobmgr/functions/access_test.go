// SPDX-License-Identifier: GPL-3.0-or-later

package functions

import (
	"testing"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	"github.com/stretchr/testify/assert"
)

func TestMethodPublicationRecordAccess(t *testing.T) {
	tests := map[string]struct {
		method funcapi.FunctionConfig
		want   string
	}{
		"public": {
			method: funcapi.FunctionConfig{ID: "public"},
			want:   "0x0000",
		},
		"signed-in Space member": {
			method: funcapi.FunctionConfig{ID: "member", RequireCloud: true},
			want:   "0x0013",
		},
		"paid Space member": {
			method: funcapi.FunctionConfig{ID: "paid", RequireCloud: true, RequireCommercial: true},
			want:   "0x0017",
		},
		"paid implies signed-in": {
			method: funcapi.FunctionConfig{ID: "paid-only", RequireCommercial: true},
			want:   "0x0017",
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			record := methodPublicationRecord("module:"+tc.method.ID, "module", tc.method, 1)
			assert.Equal(t, tc.want, record.Access)
		})
	}
}
