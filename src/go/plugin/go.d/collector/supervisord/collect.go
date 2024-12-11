// SPDX-License-Identifier: GPL-3.0-or-later

package supervisord

import (
	"fmt"
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
)

func (c *Collector) collect() (map[string]int64, error) {
	info, err := c.client.getAllProcessInfo()
	if err != nil {
		return nil, err
	}

	ms := make(map[string]int64)
	c.collectAllProcessInfo(ms, info)

	return ms, nil
}

func (c *Collector) collectAllProcessInfo(ms map[string]int64, info []processStatus) {
	c.resetCache()
	ms["running_processes"] = 0
	ms["non_running_processes"] = 0
	for _, p := range info {
		if _, ok := c.cache[p.group]; !ok {
			c.cache[p.group] = make(map[string]bool)
			c.addProcessGroupCharts(p)
		}
		if _, ok := c.cache[p.group][p.name]; !ok {
			c.addProcessToCharts(p)
		}
		c.cache[p.group][p.name] = true

		ms["group_"+p.group+"_running_processes"] += 0
		ms["group_"+p.group+"_non_running_processes"] += 0
		if isProcRunning(p) {
			ms["running_processes"] += 1
			ms["group_"+p.group+"_running_processes"] += 1
		} else {
			ms["non_running_processes"] += 1
			ms["group_"+p.group+"_non_running_processes"] += 1
		}
		id := procID(p)
		ms[id+"_state_code"] = int64(p.state)
		ms[id+"_exit_status"] = int64(p.exitStatus)
		ms[id+"_uptime"] = calcProcessUptime(p)
		ms[id+"_downtime"] = calcProcessDowntime(p)
	}
	c.cleanupCache()
}

func (c *Collector) resetCache() {
	for _, procs := range c.cache {
		for name := range procs {
			procs[name] = false
		}
	}
}

func (c *Collector) cleanupCache() {
	for group, procs := range c.cache {
		for name, ok := range procs {
			if !ok {
				c.removeProcessFromCharts(group, name)
				delete(c.cache[group], name)
			}
		}
		if len(c.cache[group]) == 0 {
			c.removeProcessGroupCharts(group)
			delete(c.cache, group)
		}
	}
}

func calcProcessUptime(p processStatus) int64 {
	if !isProcRunning(p) {
		return 0
	}
	return int64(p.now - p.start)
}

func calcProcessDowntime(p processStatus) int64 {
	if isProcRunning(p) || p.stop == 0 {
		return 0
	}
	return int64(p.now - p.stop)
}

func (c *Collector) addProcessGroupCharts(p processStatus) {
	charts := newProcGroupCharts(p.group)
	if err := c.Charts().Add(*charts...); err != nil {
		c.Warning(err)
	}
}

func (c *Collector) addProcessToCharts(p processStatus) {
	id := procID(p)
	for _, chart := range *c.Charts() {
		var dimID string
		switch chart.ID {
		case fmt.Sprintf(groupProcessesStateCodeChartTmpl.ID, p.group):
			dimID = id + "_state_code"
		case fmt.Sprintf(groupProcessesExitStatusChartTmpl.ID, p.group):
			dimID = id + "_exit_status"
		case fmt.Sprintf(groupProcessesUptimeChartTmpl.ID, p.group):
			dimID = id + "_uptime"
		case fmt.Sprintf(groupProcessesDowntimeChartTmpl.ID, p.group):
			dimID = id + "_downtime"
		default:
			continue
		}
		dim := &module.Dim{ID: dimID, Name: p.name}
		if err := chart.AddDim(dim); err != nil {
			c.Warning(err)
			return
		}
		chart.MarkNotCreated()
	}
}

func (c *Collector) removeProcessGroupCharts(group string) {
	prefix := "group_" + group
	for _, c := range *c.Charts() {
		if strings.HasPrefix(c.ID, prefix) {
			c.MarkRemove()
			c.MarkNotCreated()
		}
	}
}

func (c *Collector) removeProcessFromCharts(group, name string) {
	id := procID(processStatus{name: name, group: group})
	for _, chart := range *c.Charts() {
		var dimID string
		switch chart.ID {
		case fmt.Sprintf(groupProcessesStateCodeChartTmpl.ID, group):
			dimID = id + "_state_code"
		case fmt.Sprintf(groupProcessesExitStatusChartTmpl.ID, group):
			dimID = id + "_exit_status"
		case fmt.Sprintf(groupProcessesUptimeChartTmpl.ID, group):
			dimID = id + "_uptime"
		case fmt.Sprintf(groupProcessesDowntimeChartTmpl.ID, group):
			dimID = id + "_downtime"
		default:
			continue
		}
		if err := chart.MarkDimRemove(dimID, true); err != nil {
			c.Warning(err)
			return
		}
		chart.MarkNotCreated()
	}
}

func procID(p processStatus) string {
	return fmt.Sprintf("group_%s_process_%s", p.group, p.name)
}

func isProcRunning(p processStatus) bool {
	// http://supervisord.org/subprocess.html#process-states
	// STOPPED  (0)
	// STARTING (10)
	// RUNNING (20)
	// BACKOFF (30)
	// STOPPING (40)
	// EXITED (100)
	// FATAL (200)
	// UNKNOWN (1000)
	return p.state == 20
}
