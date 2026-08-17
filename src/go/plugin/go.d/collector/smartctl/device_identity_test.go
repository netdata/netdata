// SPDX-License-Identifier: GPL-3.0-or-later

package smartctl

import (
	"strings"
	"testing"
	"unicode"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewDeviceIdentity(t *testing.T) {
	tests := map[string]struct {
		name           string
		wantNamePrefix string
		wantExact      string
	}{
		"safe identity is unchanged": {
			name:      "IOService:/Example_Controller(Example)@0/Namespace@1",
			wantExact: "device_IOService:/Example_Controller(Example)@0/Namespace@1_type_nvme_",
		},
		"ASCII space": {
			name:           "IOService:/Example Controller(Example)@0/Namespace@1",
			wantNamePrefix: "device_IOService:/Example_Controller(Example)@0/Namespace@1_",
		},
		"tab": {
			name:           "IOService:/Example\tController(Example)@0/Namespace@1",
			wantNamePrefix: "device_IOService:/Example_Controller(Example)@0/Namespace@1_",
		},
		"Unicode non-breaking space": {
			name:           "IOService:/Example\u00a0Controller(Example)@0/Namespace@1",
			wantNamePrefix: "device_IOService:/Example_Controller(Example)@0/Namespace@1_",
		},
	}

	identities := make(map[string]deviceIdentity)
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			id := newDeviceIdentity(test.name, "nvme")
			assert.Equal(t, -1, strings.IndexFunc(id.prefix, unicode.IsSpace))
			assert.Equal(t, id, newDeviceIdentity(test.name, "nvme"), "identity must be deterministic")
			if test.wantExact != "" {
				assert.Equal(t, test.wantExact, id.prefix)
			} else {
				assert.True(t, strings.HasPrefix(id.prefix, test.wantNamePrefix), id.prefix)
				assert.True(t, strings.HasSuffix(id.prefix, "_type_nvme_"), id.prefix)
			}
			identities[name] = id
		})
	}

	require.Len(t, identities, len(tests))
	for leftName, left := range identities {
		for rightName, right := range identities {
			if leftName != rightName {
				assert.NotEqual(t, left.prefix, right.prefix, "%s and %s", leftName, rightName)
			}
		}
	}
}
