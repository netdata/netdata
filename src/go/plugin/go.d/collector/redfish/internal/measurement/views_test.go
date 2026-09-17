// SPDX-License-Identifier: GPL-3.0-or-later

package measurement_test

import (
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/redfish/internal/measurement"
	"github.com/stretchr/testify/require"
)

func TestProjectComponentViewsUseCurrentSourceMetadata(t *testing.T) {
	observed := time.Unix(100, 0)
	resources := []*measurement.Resource{
		{Kind: "firmware", Key: "firmware", AcquisitionState: "readable", Data: map[string]any{"Version": "1.2.3"}},
		{Kind: "software", Key: "software", AcquisitionState: "readable", Data: map[string]any{"Version": "4.5.6"}},
		{Kind: "system", Key: "system", AcquisitionState: "readable", Data: map[string]any{"BiosVersion": "7.8.9"}},
		{Kind: "drive", Key: "drive", AcquisitionState: "readable", Data: map[string]any{"FirmwareVersion": "A1"}},
		{
			Kind:             "firmware",
			Key:              "unreadable",
			URI:              "/redfish/v1/UpdateService/FirmwareInventory/1",
			AcquisitionState: "unreadable",
		},
		{Kind: "service", Key: "service", AcquisitionState: "readable", Data: map[string]any{}},
	}
	projector := measurement.New("https://fixture.example", "hardware", nil)
	result, err := projector.Project(resources, true, observed)
	require.NoError(t, err)
	require.Len(t, result.Components, 5, "service root is not hardware")
	for i, version := range []string{"1.2.3", "4.5.6", "7.8.9", "A1"} {
		require.Equal(t, version, result.Components[i].Firmware)
		require.Equal(t, observed, result.Components[i].ObservedAt)
	}
	missing := result.Components[4]
	require.Equal(t, "unreadable", missing.Availability)
	require.Equal(t, resources[4].URI, missing.Name)
	require.Empty(t, missing.Health)
	require.Empty(t, missing.Firmware)
	require.True(t, missing.ObservedAt.IsZero())
	resources[0].Data["Version"] = "changed"
	require.Equal(t, "1.2.3", result.Components[0].Firmware, "published facts are independent of input documents")
}
