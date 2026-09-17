// SPDX-License-Identifier: GPL-3.0-or-later

package redfish

import (
	"encoding/json"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestRateRejectsNonAdvancingSampleWithoutReplacingBaseline(t *testing.T) {
	for name, second := range map[string]int64{"equal": 10, "older": 9} {
		t.Run(name, func(t *testing.T) {
			client := fixtureClient()
			_, emit := client.rateValue("counter", "100", 1, time.Unix(10, 0), algorithmRate, "epoch")
			require.False(t, emit)
			_, emit = client.rateValue("counter", "110", 1, time.Unix(second, 0), algorithmRate, "epoch")
			require.False(t, emit)
			value, emit := client.rateValue("counter", "130", 1, time.Unix(11, 0), algorithmRate, "epoch")
			require.True(t, emit)
			require.Equal(t, float64(30), value)
		})
	}
}

func TestEnergyDerivativeRequiresCumulativeBasis(t *testing.T) {
	for basis, wantPower := range map[string]bool{"Zero": true, "": true, "Delta": false, "Headroom": false} {
		t.Run(basis, func(t *testing.T) {
			client := fixtureClient()
			node := &graphNode{
				Kind: "sensor",
				Key:  "energy",
				Data: map[string]any{
					"ReadingType":  "EnergyJoules",
					"ReadingUnits": "J",
					"ReadingBasis": basis,
					"Reading":      json.Number("100"),
				},
			}
			require.Zero(t, derivedPowerCount(client.readingsForNode(node, time.Unix(10, 0))))
			node.Data["Reading"] = json.Number("130")
			readings := client.readingsForNode(node, time.Unix(12, 0))
			require.Equal(
				t,
				float64(130),
				readings[0].Value,
				"the source reading remains available for every supported basis",
			)
			if !wantPower {
				require.Zero(t, derivedPowerCount(readings))
				return
			}
			require.Equal(t, 1, derivedPowerCount(readings))
			require.Equal(t, float64(15), readings[1].Value)
		})
	}
}

// Rate updates remain O(1) with respect to retained baselines.
func BenchmarkCounterRate(b *testing.B) {
	client := fixtureClient()
	index := int64(0)
	b.ReportAllocs()
	for b.Loop() {
		index++
		client.rateValue("counter", strconv.FormatInt(index, 10), 1, time.Unix(index, 0), algorithmRate, "epoch")
	}
}

func TestDurationRatePreservesDecodedPrecision(t *testing.T) {
	const id = "processor_processor_metrics_powerlimitthrottleduration"
	var descriptor sourceField
	for _, field := range scalarFields {
		if field.ID == id {
			descriptor = field
			break
		}
	}
	require.NotEmpty(t, descriptor.ID)
	fraction := strings.Repeat("1", 126)
	first, second := "PT0."+fraction+"S", "PT1."+fraction+"S"
	exact, _, valid := numericSourceValue(first, algorithmDurationPercent)
	require.True(t, valid)
	require.Greater(t, len(exact), maxProtocolNumericTokenBytes)
	client := fixtureClient()
	node := scalarTestNode(descriptor, descriptor.Candidates[0], first)
	at := time.Date(2026, 9, 17, 0, 0, 0, 0, time.UTC)
	initial := requireScalarValue(t, client, node, id, at)
	require.False(t, initial.Emit)
	setSourceTestPath(
		sourceTestDocument(node, descriptor.Candidates[0].Document),
		descriptor.Candidates[0].Path,
		second,
	)
	next := requireScalarValue(t, client, node, id, at.Add(10*time.Second))
	require.Empty(t, next.SourceFailures)
	require.True(t, next.Emit, "a one-second duration increase over ten seconds must emit")
	require.InDelta(t, 10.0, next.Value, 0.000001)
}
