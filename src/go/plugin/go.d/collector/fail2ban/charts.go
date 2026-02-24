// SPDX-License-Identifier: GPL-3.0-or-later

//go:build linux

package fail2ban

import (
	"fmt"
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
)

const (
	prioJailBannedIPs = collectorapi.Priority + iota
	prioJailActiveFailures
)

var jailChartsTmpl = collectorapi.Charts{
	jailCurrentBannedIPs.Copy(),
	jailActiveFailures.Copy(),
}

var (
	jailCurrentBannedIPs = collectorapi.Chart{
		ID:       "jail_%s_banned_ips",
		Title:    "Fail2Ban Jail banned IPs",
		Units:    "addresses",
		Fam:      "bans",
		Ctx:      "fail2ban.jail_banned_ips",
		Type:     collectorapi.Line,
		Priority: prioJailBannedIPs,
		Dims: collectorapi.Dims{
			{ID: "jail_%s_currently_banned", Name: "banned"},
		},
	}
	jailActiveFailures = collectorapi.Chart{
		ID:       "jail_%s_active_failures",
		Title:    "Fail2Ban Jail active failures",
		Units:    "failures",
		Fam:      "failures",
		Ctx:      "fail2ban.jail_active_failures",
		Type:     collectorapi.Line,
		Priority: prioJailActiveFailures,
		Dims: collectorapi.Dims{
			{ID: "jail_%s_currently_failed", Name: "active_failures"},
		},
	}
)

func (c *Collector) addJailCharts(jail string) {
	charts := jailChartsTmpl.Copy()

	for _, chart := range *charts {
		chart.ID = fmt.Sprintf(chart.ID, jail)
		chart.Labels = []collectorapi.Label{
			{Key: "jail", Value: jail},
		}
		for _, dim := range chart.Dims {
			dim.ID = fmt.Sprintf(dim.ID, jail)
		}
	}

	if err := c.Charts().Add(*charts...); err != nil {
		c.Warning(err)
	}
}

func (c *Collector) removeJailCharts(jail string) {
	px := fmt.Sprintf("jail_%s_", jail)
	for _, chart := range *c.Charts() {
		if strings.HasPrefix(chart.ID, px) {
			chart.MarkRemove()
			chart.MarkNotCreated()
		}
	}
}
