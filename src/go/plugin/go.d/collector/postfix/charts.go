// SPDX-License-Identifier: GPL-3.0-or-later

package postfix

import (
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
)

const (
	prioPostfixQueueEmailsCount = collectorapi.Priority + iota
	prioPostfixQueueSize
)

var charts = collectorapi.Charts{
	queueEmailsCountChart.Copy(),
	queueSizeChart.Copy(),
}

var (
	queueEmailsCountChart = collectorapi.Chart{
		ID:       "postfix_queue_emails",
		Title:    "Postfix Queue Emails",
		Units:    "emails",
		Fam:      "queue",
		Ctx:      "postfix.qemails",
		Type:     collectorapi.Line,
		Priority: prioPostfixQueueEmailsCount,
		Dims: collectorapi.Dims{
			{ID: "emails"},
		},
	}
	queueSizeChart = collectorapi.Chart{
		ID:       "postfix_queue_size",
		Title:    "Postfix Queue Size",
		Units:    "KiB",
		Fam:      "queue",
		Ctx:      "postfix.qsize",
		Type:     collectorapi.Area,
		Priority: prioPostfixQueueSize,
		Dims: collectorapi.Dims{
			{ID: "size"},
		},
	}
)
