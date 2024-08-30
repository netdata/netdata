// SPDX-License-Identifier: GPL-3.0-or-later

package activemq

import "github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"

type (
	// Charts is an alias for module.Charts
	Charts = module.Charts
	// Dims is an alias for module.Dims
	Dims = module.Dims
)

var charts = Charts{
	{
		ID:    "%s_%s_messages",
		Title: "%s Messages",
		Units: "messages/s",
		Fam:   "",
		Ctx:   "activemq.messages",
		Dims: Dims{
			{ID: "%s_%s_enqueued", Name: "enqueued", Algo: module.Incremental},
			{ID: "%s_%s_dequeued", Name: "dequeued", Algo: module.Incremental},
		},
	},
	{
		ID:    "%s_%s_unprocessed_messages",
		Title: "%s Unprocessed Messages",
		Units: "messages",
		Fam:   "",
		Ctx:   "activemq.unprocessed_messages",
		Dims: Dims{
			{ID: "%s_%s_unprocessed", Name: "unprocessed"},
		},
	},
	{
		ID:    "%s_%s_consumers",
		Title: "%s Consumers",
		Units: "consumers",
		Fam:   "",
		Ctx:   "activemq.consumers",
		Dims: Dims{
			{ID: "%s_%s_consumers", Name: "consumers"},
		},
	},
}
