// SPDX-License-Identifier: GPL-3.0-or-later

package redfish

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/redfish/internal/acquisition"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/redfish/internal/identity"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/redfish/internal/measurement"
	"github.com/stretchr/testify/require"
)

func TestCollectionSkipsProjectionForUnavailableOrUntrustedAcquisition(t *testing.T) {
	for _, test := range []struct {
		name      string
		available bool
		err       error
	}{
		{"unavailable", false, errors.New("root unavailable")},
		{"identity failure", true, identity.ErrIntegrity},
	} {
		t.Run(test.name, func(t *testing.T) {
			client := &staticEndpointClient{}
			collector := New()
			collector.client = client
			collector.measurement = measurement.New("https://fixture.example", "job", nil)
			at := time.Unix(100, 0)
			collector.now = func() time.Time { return at }
			client.result = collectionTestEnergy("100")
			_, err := collector.collect(t.Context())
			require.NoError(t, err)
			at = at.Add(10 * time.Second)
			client.result = collectionTestEnergy("1000")
			client.result.Available = test.available
			client.result.Complete = false
			client.err = test.err
			rejected, err := collector.collect(t.Context())
			require.ErrorIs(t, err, test.err)
			require.Empty(t, rejected.Hardware)
			at = at.Add(10 * time.Second)
			client.result = collectionTestEnergy("300")
			client.err = nil
			recovered, err := collector.collect(t.Context())
			require.NoError(t, err)
			require.Equal(
				t,
				[]float64{10},
				collectionTestPower(recovered),
				"rejected acquisition must not advance or prune history",
			)
		})
	}
}

func TestCollectionUsesGraphCompletenessForRateHistory(t *testing.T) {
	for _, graphComplete := range []bool{false, true} {
		t.Run(fmt.Sprintf("graph_complete_%t", graphComplete), func(t *testing.T) {
			client := &staticEndpointClient{}
			collector := New()
			collector.client = client
			collector.measurement = measurement.New("https://fixture.example", "job", nil)
			at := time.Unix(100, 0)
			collector.now = func() time.Time { return at }
			client.result = collectionTestEnergy("100")
			_, err := collector.collect(t.Context())
			require.NoError(t, err)
			at = at.Add(10 * time.Second)
			client.result = acquisition.Result{
				Available:     true,
				Complete:      false,
				GraphComplete: graphComplete,
			}
			client.err = errors.New("base collection incomplete")
			partial, err := collector.collect(t.Context())
			require.Error(t, err)
			require.Equal(t, "partial", partial.Metrics.Status)
			at = at.Add(10 * time.Second)
			client.result = collectionTestEnergy("300")
			client.err = nil
			recovered, err := collector.collect(t.Context())
			require.NoError(t, err)
			if graphComplete {
				require.Empty(t, collectionTestPower(recovered))
			} else {
				require.Equal(t, []float64{10}, collectionTestPower(recovered))
			}
		})
	}
}

func TestCollectionCombinesAcquisitionAndMeasurementDiagnostics(t *testing.T) {
	const uri = "/redfish/v1/Chassis/1"
	const repeated = "Redfish reading source alarm is missing for " + uri + " chassis.PowerWatts.Reading"
	var diagnostics acquisition.Diagnostics
	diagnostics.Add(repeated)
	for index := range 255 {
		diagnostics.Add(fmt.Sprintf("acquisition diagnostic %d", index))
	}
	diagnostics.Add("omitted acquisition diagnostic")
	client := &staticEndpointClient{
		result: acquisition.Result{
			Available:     true,
			Complete:      true,
			GraphComplete: true,
			Diagnostics:   diagnostics,
			Resources: []*measurement.Resource{{Kind: "chassis", Key: "chassis", URI: uri, AcquisitionState: "readable",
				Data: map[string]any{
					"PowerWatts":  map[string]any{"Reading": 10},
					"CurrentAmps": map[string]any{"Reading": 2},
				},
			}},
		},
	}
	collector := New()
	collector.client = client
	collector.measurement = measurement.New("https://fixture.example", "job", nil)
	result, err := collector.collect(context.Background())
	require.NoError(t, err)
	require.Len(t, result.Diagnostics, 257)
	require.Equal(t, repeated, result.Diagnostics[0])
	require.Equal(
		t,
		"2 additional Redfish collection diagnostics were omitted by the fixed internal bound",
		result.Diagnostics[256],
	)
	require.True(t, result.Complete)
}

func collectionTestEnergy(value string) acquisition.Result {
	return acquisition.Result{
		Available:     true,
		Complete:      true,
		GraphComplete: true,
		Resources: []*measurement.Resource{{
			Kind: "sensor", Key: "energy", AcquisitionState: "readable", Data: map[string]any{
				"ReadingType": "EnergyJoules", "ReadingUnits": "J", "Reading": json.Number(value),
			},
		}},
	}
}
func collectionTestPower(result collectionResult) []float64 {
	var values []float64
	for _, observation := range result.Hardware {
		if observation.Metric == "reading_power_value" {
			values = append(values, observation.Value)
		}
	}
	return values
}
