// SPDX-License-Identifier: GPL-3.0-or-later

package snmp

import (
	"math"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/snmputils"
)

// RFC 3416 bounds max-repetitions at 2147483647.
func TestCollector_ValidateConfigMaxRepetitions(t *testing.T) {
	if strconv.IntSize < 64 {
		t.Skip("max_repetitions cannot exceed the bound on this platform")
	}
	limit := int64(math.MaxInt32)
	for name, tc := range map[string]struct {
		maxRepetitions int
		wantFailure    snmputils.Failure
	}{
		"disabled":     {maxRepetitions: 0},
		"negative":     {maxRepetitions: -1},
		"at the bound": {maxRepetitions: int(limit)},
		"over the bound": {
			maxRepetitions: int(limit + 1),
			wantFailure:    snmputils.Failure{Operation: "configuration", Reason: "invalid_configuration"},
		},
	} {
		t.Run(name, func(t *testing.T) {
			c := newTestSNMPCollector()
			c.Config = prepareV2Config()
			c.Options.MaxRepetitions = tc.maxRepetitions
			err := c.validateConfig()
			if tc.wantFailure.Reason == "" {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.Equal(t, tc.wantFailure, snmputils.ClassifyFailure(err))
		})
	}
}
