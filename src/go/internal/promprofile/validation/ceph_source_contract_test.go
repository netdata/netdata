// SPDX-License-Identifier: GPL-3.0-or-later

package promvalidation

import (
	"testing"

	"github.com/stretchr/testify/require"

	promsemantics "github.com/netdata/netdata/go/plugins/internal/promprofile/semantics"
	"github.com/netdata/netdata/go/plugins/internal/promprofile/testutil"
)

func TestCephTentacleHardwareFanRPMSourceUnit(t *testing.T) {
	source, err := promsemantics.LoadSourceSemantics(
		promtestutil.Require(t, "prometheus/profiles/ceph/SOURCE-SEMANTICS.yaml"),
	)
	require.NoError(t, err)

	signal, ok := source.Signals["hardware_fan_rpm"]
	require.True(t, ok)
	component, ok := signal.Components["value"]
	require.True(t, ok)
	require.Equal(t, "count", component.Unit.Quantity)
	require.Equal(t, "one", component.Unit.Base)
	require.Equal(t, "per_minute", component.Unit.Rate)
	require.Equal(t, "fan_revolutions", component.Unit.Object)
}
