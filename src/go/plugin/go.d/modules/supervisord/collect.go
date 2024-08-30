// SPDX-License-Identifier: GPL-3.0-or-later

package supervisord

import (
	"fmt"
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
)

func (s *Supervisord) collect() (map[string]int64, error) {
	info, err := s.client.getAllProcessInfo()
	if err != nil {
		return nil, err
	}

	ms := make(map[string]int64)
	s.collectAllProcessInfo(ms, info)

	return ms, nil
}

func (s *Supervisord) collectAllProcessInfo(ms map[string]int64, info []processStatus) {
	s.resetCache()
	ms["running_processes"] = 0
	ms["non_running_processes"] = 0
	for _, p := range info {
		if _, ok := s.cache[p.group]; !ok {
			s.cache[p.group] = make(map[string]bool)
			s.addProcessGroupCharts(p)
		}
		if _, ok := s.cache[p.group][p.name]; !ok {
			s.addProcessToCharts(p)
		}
		s.cache[p.group][p.name] = true

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
	s.cleanupCache()
}

func (s *Supervisord) resetCache() {
	for _, procs := range s.cache {
		for name := range procs {
			procs[name] = false
		}
	}
}

func (s *Supervisord) cleanupCache() {
	for group, procs := range s.cache {
		for name, ok := range procs {
			if !ok {
				s.removeProcessFromCharts(group, name)
				delete(s.cache[group], name)
			}
		}
		if len(s.cache[group]) == 0 {
			s.removeProcessGroupCharts(group)
			delete(s.cache, group)
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

func (s *Supervisord) addProcessGroupCharts(p processStatus) {
	charts := newProcGroupCharts(p.group)
	if err := s.Charts().Add(*charts...); err != nil {
		s.Warning(err)
	}
}

func (s *Supervisord) addProcessToCharts(p processStatus) {
	id := procID(p)
	for _, c := range *s.Charts() {
		var dimID string
		switch c.ID {
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
		if err := c.AddDim(dim); err != nil {
			s.Warning(err)
			return
		}
		c.MarkNotCreated()
	}
}

func (s *Supervisord) removeProcessGroupCharts(group string) {
	prefix := "group_" + group
	for _, c := range *s.Charts() {
		if strings.HasPrefix(c.ID, prefix) {
			c.MarkRemove()
			c.MarkNotCreated()
		}
	}
}

func (s *Supervisord) removeProcessFromCharts(group, name string) {
	id := procID(processStatus{name: name, group: group})
	for _, c := range *s.Charts() {
		var dimID string
		switch c.ID {
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
		if err := c.MarkDimRemove(dimID, true); err != nil {
			s.Warning(err)
			return
		}
		c.MarkNotCreated()
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
