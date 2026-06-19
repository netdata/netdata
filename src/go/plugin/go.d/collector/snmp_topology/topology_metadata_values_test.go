// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestTopologyMetadataValue_CanonicalizesAliasKeys(t *testing.T) {
	labels := map[string]string{
		"Serial Number":    "SN-123",
		"software.version": "17.9.4",
		"sys-location":     "dc1",
	}

	tests := map[string]struct {
		keys []string
		want string
	}{
		"serial-number":    {keys: []string{"serial_number"}, want: "SN-123"},
		"software-version": {keys: []string{"software_version"}, want: "17.9.4"},
		"sys-location":     {keys: []string{"sys_location"}, want: "dc1"},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			require.Equal(t, tc.want, topologyMetadataValue(labels, tc.keys))
		})
	}
}

func TestSetTopologyMetadataLabelIfMissing_PreservesExistingValue(t *testing.T) {
	labels := map[string]string{"serial_number": "SN-123"}

	setTopologyMetadataLabelIfMissing(labels, "serial_number", "SN-999")
	setTopologyMetadataLabelIfMissing(labels, "firmware_version", "1.2.3")

	require.Equal(t, "SN-123", labels["serial_number"])
	require.Equal(t, "1.2.3", labels["firmware_version"])
}

func TestTopologyMetadataValue_DeterministicAcrossCanonicalKeyCollisions(t *testing.T) {
	tests := map[string]struct {
		labels map[string]string
	}{
		"underscore-first": {
			labels: map[string]string{
				"serial_number": "SN-200",
				"serial-number": "SN-100",
			},
		},
		"dash-first": {
			labels: map[string]string{
				"serial-number": "SN-100",
				"serial_number": "SN-200",
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			require.Equal(t, "SN-100", topologyMetadataValue(tc.labels, []string{"serial_number"}))
		})
	}
}
