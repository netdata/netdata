// SPDX-License-Identifier: GPL-3.0-or-later

package measurement

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRedundancyModeSourceEnums(t *testing.T) {
	for name, test := range map[string]struct {
		data map[string]any
		want string
	}{
		"legacy n plus m":    {map[string]any{"Mode": "N+m"}, "n_plus_m"},
		"modern n plus m":    {map[string]any{"RedundancyType": "NPlusM"}, "n_plus_m"},
		"modern wins":        {map[string]any{"RedundancyType": "NPlusM", "Mode": "Failover"}, "n_plus_m"},
		"modern null legacy": {map[string]any{"RedundancyType": nil, "Mode": "N+m"}, "n_plus_m"},
		"legacy failover":    {map[string]any{"Mode": "Failover"}, "failover"},
		"modern sharing":     {map[string]any{"RedundancyType": "Sharing"}, "sharing"},
		"modern malformed":   {map[string]any{"RedundancyType": false, "Mode": "Failover"}, "unknown"},
		"unsupported mode":   {map[string]any{"Mode": "VendorMode"}, "unknown"},
		"absent":             {map[string]any{}, ""},
	} {
		t.Run(name, func(t *testing.T) {
			observations := fixtureClient().additionalStateObservations(&Resource{
				Kind: "redundancy",
				Data: test.data,
			}, nil)
			if test.want == "" {
				require.Empty(t, observations)
				return
			}
			require.Len(t, observations, 1)
			require.Equal(t, "redundancy_mode", observations[0].Metric)
			require.Equal(t, test.want, observations[0].State)
		})
	}
}

// Counting stays O(condition records); ns/op is a local-machine trend indicator.
func BenchmarkConditionCounts(b *testing.B) {
	data := map[string]any{
		"Status": map[string]any{
			"Conditions": []any{map[string]any{"Severity": "Critical"}, map[string]any{"Severity": "Warning"}},
		},
	}
	raw, err := json.Marshal(data)
	if err != nil {
		b.Fatal(err)
	}
	var doc Document
	err = json.Unmarshal(raw, &doc)
	if err != nil {
		b.Fatal(err)
	}
	node := &Resource{
		Data: data,
		Doc:  doc,
	}
	b.ReportAllocs()
	for b.Loop() {
		_, _, readable := conditionCountsForNode(node)
		if !readable {
			b.Fatal("valid conditions unreadable")
		}
	}
}
