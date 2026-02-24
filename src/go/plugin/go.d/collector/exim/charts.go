// SPDX-License-Identifier: GPL-3.0-or-later

package exim

import (
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
)

const (
	prioQueueEmailsCount = collectorapi.Priority + iota
)

var charts = collectorapi.Charts{
	queueEmailsCountChart.Copy(),
}

var queueEmailsCountChart = collectorapi.Chart{
	ID:       "qemails",
	Title:    "Exim Queue Emails",
	Units:    "emails",
	Fam:      "queue",
	Ctx:      "exim.qemails",
	Priority: prioQueueEmailsCount,
	Dims: collectorapi.Dims{
		{ID: "emails"},
	},
}
