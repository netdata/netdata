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

	require.Equal(t, "SN-123", topologyMetadataValue(labels, []string{"serial_number"}))
	require.Equal(t, "17.9.4", topologyMetadataValue(labels, []string{"software_version"}))
	require.Equal(t, "dc1", topologyMetadataValue(labels, []string{"sys_location"}))
}

func TestSetTopologyMetadataLabelIfMissing_PreservesExistingValue(t *testing.T) {
	labels := map[string]string{"serial_number": "SN-123"}

	setTopologyMetadataLabelIfMissing(labels, "serial_number", "SN-999")
	setTopologyMetadataLabelIfMissing(labels, "firmware_version", "1.2.3")

	require.Equal(t, "SN-123", labels["serial_number"])
	require.Equal(t, "1.2.3", labels["firmware_version"])
}

func TestTopologyMetadataValue_DeterministicAcrossCanonicalKeyCollisions(t *testing.T) {
	first := map[string]string{
		"serial_number": "SN-200",
		"serial-number": "SN-100",
	}
	second := map[string]string{
		"serial-number": "SN-100",
		"serial_number": "SN-200",
	}

	require.Equal(t, "SN-100", topologyMetadataValue(first, []string{"serial_number"}))
	require.Equal(t, "SN-100", topologyMetadataValue(second, []string{"serial_number"}))
}
