// SPDX-License-Identifier: GPL-3.0-or-later

package ddsnmp

import (
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/netdata/netdata/go/plugins/pkg/multipath"
)

func Test_AlcatelBGPProfileUsesTypedRows(t *testing.T) {
	dir, _ := filepath.Abs("../../../config/go.d/snmp.profiles/default")

	profiles, err := loadProfilesFromDir(dir, multipath.New(dir))
	require.NoError(t, err)
	require.NotEmpty(t, profiles)

	matched := FindProfiles("1.3.6.1.4.1.6486.801.1.1.2.1.1", "", nil)
	index := slices.IndexFunc(matched, func(p *Profile) bool {
		return strings.HasSuffix(p.SourceFile, "alcatel-lucent-ent.yaml")
	})
	require.NotEqual(t, -1, index, "expected alcatel-lucent-ent.yaml profile to match")

	profile := matched[index]
	for _, table := range []string{"bgpPeerTable", "alaBgpPeerTable", "alaBgpPeer6Table"} {
		assertNoMetricTable(t, profile, table)
	}
	for _, name := range []string{"bgpPeerAvailability", "bgpPeerUpdates", "alcatel.bgpPeerAvailability", "alcatel.bgpPeerUpdates"} {
		assertNoVirtualMetric(t, profile, name)
	}
	assertBGPTableForRowID(t, profile, "alcatel-bgp4-peer", "bgpPeerTable")
	assertBGPTableForRowID(t, profile, "alcatel-bgp4-peer-family", "alaBgpPeerTable")
	assertBGPTableForRowID(t, profile, "alcatel-bgp6-peer", "alaBgpPeer6Table")
	assertBGPTableForRowID(t, profile, "alcatel-bgp6-peer-family", "alaBgpPeer6Table")
	assertBGPSixStateMapping(t, requireBGPRowByID(t, profile, "alcatel-bgp4-peer").State)
	assertBGPSixStateMapping(t, requireBGPRowByID(t, profile, "alcatel-bgp6-peer").State)
}
