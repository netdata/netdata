// SPDX-License-Identifier: GPL-3.0-or-later

package activemq

import "github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"

type (
	// Charts is an alias for collectorapi.Charts
	Charts = collectorapi.Charts
	// Dims is an alias for collectorapi.Dims
	Dims = collectorapi.Dims
)

var charts = Charts{
	{
		ID:    "%s_%s_messages",
		Title: "%s Messages",
		Units: "messages/s",
		Fam:   "",
		Ctx:   "activemq.messages",
		Dims: Dims{
			{ID: "%s_%s_enqueued", Name: "enqueued", Algo: collectorapi.Incremental},
			{ID: "%s_%s_dequeued", Name: "dequeued", Algo: collectorapi.Incremental},
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
