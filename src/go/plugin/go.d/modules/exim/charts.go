// SPDX-License-Identifier: GPL-3.0-or-later

package exim

import (
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
)

const (
	prioQueueEmailsCount = module.Priority + iota
)

var charts = module.Charts{
	queueEmailsCountChart.Copy(),
}

var queueEmailsCountChart = module.Chart{
	ID:       "qemails",
	Title:    "Exim Queue Emails",
	Units:    "emails",
	Fam:      "queue",
	Ctx:      "exim.qemails",
	Priority: prioQueueEmailsCount,
	Dims: module.Dims{
		{ID: "emails"},
	},
}
