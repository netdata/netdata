// SPDX-License-Identifier: GPL-3.0-or-later

package fail2ban

import (
	"fmt"
	"strings"

	"github.com/netdata/netdata/go/go.d.plugin/agent/module"
)

const (
	prioJailBannedIPs = module.Priority + iota
	prioJailActiveFailures
)

var jailChartsTmpl = module.Charts{
	jailCurrentBannedIPs.Copy(),
	jailActiveFailures.Copy(),
}

var (
	jailCurrentBannedIPs = module.Chart{
		ID:       "jail_%s_banned_ips",
		Title:    "Fail2Ban Jail banned IPs",
		Units:    "addresses",
		Fam:      "bans",
		Ctx:      "fail2ban.jail_banned_ips",
		Type:     module.Line,
		Priority: prioJailBannedIPs,
		Dims: module.Dims{
			{ID: "jail_%s_currently_banned", Name: "banned"},
		},
	}
	jailActiveFailures = module.Chart{
		ID:       "jail_%s_active_failures",
		Title:    "Fail2Ban Jail active failures",
		Units:    "failures",
		Fam:      "failures",
		Ctx:      "fail2ban.jail_active_failures",
		Type:     module.Line,
		Priority: prioJailActiveFailures,
		Dims: module.Dims{
			{ID: "jail_%s_currently_failed", Name: "active_failures"},
		},
	}
)

func (f *Fail2Ban) addJailCharts(jail string) {
	charts := jailChartsTmpl.Copy()

	for _, chart := range *charts {
		chart.ID = fmt.Sprintf(chart.ID, jail)
		chart.Labels = []module.Label{
			{Key: "jail", Value: jail},
		}
		for _, dim := range chart.Dims {
			dim.ID = fmt.Sprintf(dim.ID, jail)
		}
	}

	if err := f.Charts().Add(*charts...); err != nil {
		f.Warning(err)
	}
}

func (f *Fail2Ban) removeJailCharts(jail string) {
	px := fmt.Sprintf("jail_%s_", jail)
	for _, chart := range *f.Charts() {
		if strings.HasPrefix(chart.ID, px) {
			chart.MarkRemove()
			chart.MarkNotCreated()
		}
	}
}
