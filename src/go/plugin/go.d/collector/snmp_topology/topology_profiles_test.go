// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"testing"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAppendTopologyProfiles_AutoprobeEnabled(t *testing.T) {
	log := logger.New()

	profiles := appendTopologyProfiles(nil, log)
	require.NotEmpty(t, profiles)

	assert.True(t, profilesHaveExtension(profiles, topologyLldpProfileName))
	assert.True(t, profilesHaveExtension(profiles, cdpProfileName))
	assert.True(t, profilesHaveExtension(profiles, fdbArpProfileName))
	assert.True(t, profilesHaveExtension(profiles, qBridgeProfileName))
	assert.True(t, profilesHaveExtension(profiles, stpProfileName))
	assert.True(t, profilesHaveExtension(profiles, vtpProfileName))
}

func TestAppendTopologyProfiles_AutoprobeDisabled(t *testing.T) {
	seed := []*ddsnmp.Profile{{SourceFile: "_custom.yaml"}}

	// When not called (autoprobe disabled), profiles are unchanged.
	require.Len(t, seed, 1)
	assert.Equal(t, "_custom.yaml", seed[0].SourceFile)
}
