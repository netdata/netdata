// SPDX-License-Identifier: GPL-3.0-or-later

package snmp

import (
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAppendTopologyProfiles_AutoprobeEnabled(t *testing.T) {
	coll := &Collector{
		Config: Config{
			Topology: TopologyConfig{Autoprobe: true},
		},
	}

	profiles := coll.appendTopologyProfiles(nil)
	require.NotEmpty(t, profiles)

	assert.True(t, profilesHaveExtension(profiles, topologyLldpProfileName))
	assert.True(t, profilesHaveExtension(profiles, cdpProfileName))
	assert.True(t, profilesHaveExtension(profiles, fdbArpProfileName))
}

func TestAppendTopologyProfiles_AutoprobeDisabled(t *testing.T) {
	coll := &Collector{
		Config: Config{
			Topology: TopologyConfig{Autoprobe: false},
		},
	}

	seed := []*ddsnmp.Profile{{SourceFile: "_custom.yaml"}}
	out := coll.appendTopologyProfiles(seed)
	require.Len(t, out, 1)
	assert.Equal(t, "_custom.yaml", out[0].SourceFile)
}
