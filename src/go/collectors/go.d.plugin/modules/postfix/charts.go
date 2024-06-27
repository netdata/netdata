// SPDX-License-Identifier: GPL-3.0-or-later

package postfix

import (
	"github.com/netdata/netdata/go/go.d.plugin/agent/module"
)

const (
	prioPostfixQEmailsChartTmpl = 1000 + iota
	prioPostfixQSizeChartTmpl
)

var postfixChartsTmpl = module.Charts{
	postfixQEmailsChartTmpl.Copy(),
	postfixQSizeChartTmpl.Copy(),
}

var (
	postfixQEmailsChartTmpl = module.Chart{
		ID:       "postfix_queue_emails",
		Title:    "Postfix Queue Emails",
		Units:    "emails",
		Fam:      "queue",
		Ctx:      "postfix.qemails",
		Type:     module.Area,
		Priority: prioPostfixQEmailsChartTmpl,
		Dims: module.Dims{
			{ID: "emails", Name: "emails"},
		},
	}
	postfixQSizeChartTmpl = module.Chart{
		ID:       "postfix_queue_size",
		Title:    "Postfix Queue Size",
		Units:    "KiB",
		Fam:      "queue",
		Ctx:      "postfix.qsize",
		Type:     module.Area,
		Priority: prioPostfixQSizeChartTmpl,
		Dims: module.Dims{
			{ID: "size", Name: "size"},
		},
	}
)

func (p *Postfix) addPostfixCharts() {
	charts := postfixChartsTmpl.Copy()

	if err := p.Charts().Add(*charts...); err != nil {
		p.Warning(err)
	}
}

// func (p *Postfix) removePostfixCharts(name string) {
// 	for _, chart := range *p.Charts() {
// 		if strings.HasPrefix(chart.ID, name) {
// 			chart.MarkRemove()
// 			chart.MarkNotCreated()
// 		}
// 	}
// }
