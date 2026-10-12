// SPDX-License-Identifier: GPL-3.0-or-later

package charttpl

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestChartExpiryPresenceRoundTrip(t *testing.T) {
	for name, expiry := range map[string]*int{"omitted": nil, "disabled": new(0), "positive": new(7)} {
		t.Run(name, func(t *testing.T) {
			spec := validationSpec()
			spec.Groups[0].Charts[0].Lifecycle = &Lifecycle{ExpireAfterCycles: expiry}
			for _, clone := range []bool{false, true} {
				if clone {
					spec.Groups[0] = spec.Groups[0].Clone()
				}
				data, err := spec.MarshalTemplate()
				require.NoError(t, err)
				if expiry == nil {
					assert.NotContains(t, data, "expire_after_cycles")
				} else {
					assert.Contains(t, data, fmt.Sprintf("expire_after_cycles: %d", *expiry))
				}
				decoded, err := DecodeYAML([]byte(data))
				require.NoError(t, err)
				require.NotNil(t, decoded.Groups[0].Charts[0].Lifecycle)
				assert.Equal(t, expiry, decoded.Groups[0].Charts[0].Lifecycle.ExpireAfterCycles)

				jsonData, err := json.Marshal(spec)
				require.NoError(t, err)
				if expiry == nil {
					assert.NotContains(t, string(jsonData), "expire_after_cycles")
				} else {
					assert.Contains(t, string(jsonData), fmt.Sprintf(`"expire_after_cycles":%d`, *expiry))
				}
				var fromJSON Spec
				require.NoError(t, json.Unmarshal(jsonData, &fromJSON))
				require.NotNil(t, fromJSON.Groups[0].Charts[0].Lifecycle)
				assert.Equal(t, expiry, fromJSON.Groups[0].Charts[0].Lifecycle.ExpireAfterCycles)
			}
		})
	}
}

func TestDecodeYAMLRejectsNegativeChartExpiry(t *testing.T) {
	spec := validationSpec()
	spec.Groups[0].Charts[0].Lifecycle = &Lifecycle{ExpireAfterCycles: new(0)}
	data, err := spec.MarshalTemplate()
	require.NoError(t, err)
	// Use the authored document boundary as well as the typed validation tests.
	data = strings.Replace(data, "expire_after_cycles: 0", "expire_after_cycles: -1", 1)
	_, err = DecodeYAML([]byte(data))
	require.ErrorContains(t, err, "lifecycle.expire_after_cycles")
}
