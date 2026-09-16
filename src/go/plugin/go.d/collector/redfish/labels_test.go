// SPDX-License-Identifier: GPL-3.0-or-later

package redfish

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestComponentFamilyLabelUsesKnownKindsOnly(t *testing.T) {
	client := &protocolClient{}
	for kind := range sourceStatusByKind {
		require.Equal(t, kind, observationLabel(client.metricLabels(&graphNode{
			Kind: kind,
		}, nil), "component_family"))
	}
	require.Empty(
		t,
		observationLabel(client.metricLabels(&graphNode{
			Kind: "vendor_extension",
		}, nil), "component_family"),
	)
}
