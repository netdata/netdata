// SPDX-License-Identifier: GPL-3.0-or-later

package postfix

import (
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
)

const (
	prioPostfixQueueEmailsCount = module.Priority + iota
	prioPostfixQueueSize
)

var charts = module.Charts{
	queueEmailsCountChart.Copy(),
	queueSizeChart.Copy(),
}

var (
	queueEmailsCountChart = module.Chart{
		ID:       "postfix_queue_emails",
		Title:    "Postfix Queue Emails",
		Units:    "emails",
		Fam:      "queue",
		Ctx:      "postfix.qemails",
		Type:     module.Line,
		Priority: prioPostfixQueueEmailsCount,
		Dims: module.Dims{
			{ID: "emails"},
		},
	}
	queueSizeChart = module.Chart{
		ID:       "postfix_queue_size",
		Title:    "Postfix Queue Size",
		Units:    "KiB",
		Fam:      "queue",
		Ctx:      "postfix.qsize",
		Type:     module.Area,
		Priority: prioPostfixQueueSize,
		Dims: module.Dims{
			{ID: "size"},
		},
	}
)
