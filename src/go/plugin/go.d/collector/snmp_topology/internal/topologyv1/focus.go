// SPDX-License-Identifier: GPL-3.0-or-later

package topologyv1

import (
	"fmt"

	topologyapi "github.com/netdata/netdata/go/plugins/pkg/topology/v1"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologymodel"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologyoptions"
)

func buildFocusBindings(bindings []topologymodel.FocusRootBinding, actorIndex map[topologymodel.ActorHandle]int) (topologyapi.DetailTable, topologyapi.TableType, error) {
	columns := []topologyapi.Column{
		topologyapi.NewColumn("option", "string", topologyapi.WithRole("attribute")),
		topologyapi.NewColumn("actor", "actor_ref", topologyapi.WithRole("reference")),
	}
	builder := topologyapi.NewTableBuilder(columns...)
	for _, binding := range bindings {
		row, ok := actorIndex[binding.ActorHandle]
		if !ok || binding.OptionID == "" {
			return topologyapi.DetailTable{}, topologyapi.TableType{}, fmt.Errorf("focus binding has an empty option or unresolved actor handle")
		}
		builder.Add(binding.OptionID, row)
	}
	table, err := builder.Table()
	if err != nil {
		return topologyapi.DetailTable{}, topologyapi.TableType{}, fmt.Errorf("build focus bindings: %w", err)
	}
	definition := topologyapi.TableType{
		Role: "actor_detail", Owner: "actor", Aggregation: "append", Columns: columns,
		Presentation: &topologyapi.TableTypePresentation{Label: "Focus bindings", DefaultVisibility: "hidden"},
	}
	return topologyapi.DetailTable{Type: topologyoptions.ManagedFocusBindingsType, Table: table}, definition, nil
}
