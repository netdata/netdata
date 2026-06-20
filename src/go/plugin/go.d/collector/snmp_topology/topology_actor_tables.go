// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import "strings"

func attachTopologyActorTableRows(data *topologyData, tableName string, rowsByActor map[string][]map[string]any, sortRows func([]map[string]any)) {
	if data == nil || tableName == "" || len(rowsByActor) == 0 {
		return
	}
	for i := range data.Actors {
		actor := &data.Actors[i]
		rows := rowsByActor[strings.TrimSpace(actor.ActorID)]
		if len(rows) == 0 {
			continue
		}
		if sortRows != nil {
			sortRows(rows)
		}
		if actor.Tables == nil {
			actor.Tables = make(map[string][]map[string]any)
		}
		actor.Tables[tableName] = rows
	}
}
