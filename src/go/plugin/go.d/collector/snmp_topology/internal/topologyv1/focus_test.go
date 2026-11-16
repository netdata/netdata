// SPDX-License-Identifier: GPL-3.0-or-later

package topologyv1

import (
	"testing"

	topologyapi "github.com/netdata/netdata/go/plugins/pkg/topology/v1"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologymodel"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologyoptions"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologyv1test"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRenderFocusBindingsResolveFinalActorRows(t *testing.T) {
	data := snmpTopologyV1GoldenInput(t)
	first, second := data.Actors[0].ActorHandle, data.Actors[1].ActorHandle
	data.FocusRootBindings = []topologymodel.FocusRootBinding{
		{OptionID: "choice-a", ActorHandle: first},
		{OptionID: "choice-a", ActorHandle: second},
		{OptionID: "choice-b", ActorHandle: first},
	}
	data.Actors[0], data.Actors[1] = data.Actors[1], data.Actors[0]
	payload, err := Render(data)
	require.NoError(t, err)
	require.NoError(t, topologyv1test.ValidateData(payload))
	table := payload.Tables.Actor[topologyoptions.ManagedFocusBindingsTable]
	assert.Equal(t, topologyoptions.ManagedFocusBindingsType, table.Type)
	assert.Equal(t, 3, table.Table.Rows)
	assert.Equal(t, topologyapi.Values("choice-a", "choice-a", "choice-b"), table.Table.Values[0])
	assert.Equal(t, topologyapi.Values(1, 0, 1), table.Table.Values[1], "private handles must be remapped after actor ordering")
	definition := payload.Types.TableTypes[table.Type]
	assert.Equal(t, "actor", definition.Owner)
	assert.Equal(t, "actor_detail", definition.Role)
	assert.Equal(t, "append", definition.Aggregation)
	require.NotNil(t, definition.Presentation)
	assert.Equal(t, "hidden", definition.Presentation.DefaultVisibility)

	data.FocusRootBindings = nil
	without, err := Render(data)
	require.NoError(t, err)
	assert.Equal(t, payload.Actors, without.Actors)
	assert.Equal(t, payload.Links, without.Links)
	assert.Equal(t, payload.Evidence, without.Evidence)
	assert.Equal(t, payload.Presentation, without.Presentation)
	assert.Zero(t, without.Tables.Actor[topologyoptions.ManagedFocusBindingsTable].Table.Rows)
}

func TestRenderFocusBindingsRejectInvalidSourceMapping(t *testing.T) {
	for _, binding := range []topologymodel.FocusRootBinding{
		{OptionID: "choice"},
		{OptionID: "choice", ActorHandle: topologyV1TestActorHandle("missing-focus-actor")},
		{OptionID: "", ActorHandle: topologyV1TestActorHandle("switch-a")},
	} {
		data := snmpTopologyV1GoldenInput(t)
		data.FocusRootBindings = []topologymodel.FocusRootBinding{binding}
		_, err := Render(data)
		require.ErrorContains(t, err, "focus binding")
	}
}
