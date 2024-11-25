// SPDX-License-Identifier: GPL-3.0-or-later

package fluentd

import "github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"

type (
	// Charts is an alias for module.Charts
	Charts = module.Charts
	// Dim is an alias for module.Dim
	Dim = module.Dim
)

// TODO: units for buffer charts
var charts = Charts{
	{
		ID:    "retry_count",
		Title: "Plugin Retry Count",
		Units: "count",
		Fam:   "retry count",
		Ctx:   "fluentd.retry_count",
	},
	{
		ID:    "buffer_queue_length",
		Title: "Plugin Buffer Queue Length",
		Units: "queue length",
		Fam:   "buffer",
		Ctx:   "fluentd.buffer_queue_length",
	},
	{
		ID:    "buffer_total_queued_size",
		Title: "Plugin Buffer Total Size",
		Units: "buffer total size",
		Fam:   "buffer",
		Ctx:   "fluentd.buffer_total_queued_size",
	},
}
