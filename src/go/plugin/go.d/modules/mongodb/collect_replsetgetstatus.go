// SPDX-License-Identifier: GPL-3.0-or-later

package mongo

import (
	"fmt"
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/metrix"
)

// https://www.mongodb.com/docs/manual/reference/replica-states/#replica-set-member-states
var replicaSetMemberStates = map[string]int{
	"startup":    0,
	"primary":    1,
	"secondary":  2,
	"recovering": 3,
	"startup2":   5,
	"unknown":    6,
	"arbiter":    7,
	"down":       8,
	"rollback":   9,
	"removed":    10,
}

// TODO: deal with duplicates if we collect metrics from all cluster nodes
// should we only collect ReplSetStatus (at least by default) from primary nodes? (db.runCommand( { isMaster: 1 } ))
func (m *Mongo) collectReplSetStatus(mx map[string]int64) error {
	s, err := m.conn.replSetGetStatus()
	if err != nil {
		return fmt.Errorf("error get status of the replica set from mongo: %s", err)
	}

	seen := make(map[string]documentReplSetMember)

	for _, member := range s.Members {
		seen[member.Name] = member

		px := fmt.Sprintf("repl_set_member_%s_", member.Name)

		mx[px+"replication_lag"] = s.Date.Sub(member.OptimeDate).Milliseconds()

		for k, v := range replicaSetMemberStates {
			mx[px+"state_"+k] = metrix.Bool(member.State == v)
		}

		mx[px+"health_status_up"] = metrix.Bool(member.Health == 1)
		mx[px+"health_status_down"] = metrix.Bool(member.Health == 0)

		if member.Self == nil {
			mx[px+"uptime"] = member.Uptime
			if v := member.LastHeartbeatRecv; v != nil && !v.IsZero() {
				mx[px+"heartbeat_latency"] = s.Date.Sub(*v).Milliseconds()
			}
			if v := member.PingMs; v != nil {
				mx[px+"ping_rtt"] = *v
			}
		}
	}

	for name, member := range seen {
		if !m.replSetMembers[name] {
			m.replSetMembers[name] = true
			m.Debugf("new replica set member '%s': adding charts", name)
			m.addReplSetMemberCharts(member)
		}
	}

	for name := range m.replSetMembers {
		if _, ok := seen[name]; !ok {
			delete(m.replSetMembers, name)
			m.Debugf("stale replica set member '%s': removing charts", name)
			m.removeReplSetMemberCharts(name)
		}
	}

	return nil
}

func (m *Mongo) addReplSetMemberCharts(v documentReplSetMember) {
	charts := chartsTmplReplSetMember.Copy()

	if v.Self != nil {
		_ = charts.Remove(chartTmplReplSetMemberHeartbeatLatencyTime.ID)
		_ = charts.Remove(chartTmplReplSetMemberPingRTTTime.ID)
		_ = charts.Remove(chartTmplReplSetMemberUptime.ID)
	}

	for _, chart := range *charts {
		chart.ID = fmt.Sprintf(chart.ID, v.Name)
		chart.Labels = []module.Label{
			{Key: "repl_set_member", Value: v.Name},
		}
		for _, dim := range chart.Dims {
			dim.ID = fmt.Sprintf(dim.ID, v.Name)
		}
	}

	if err := m.Charts().Add(*charts...); err != nil {
		m.Warning(err)
	}
}

func (m *Mongo) removeReplSetMemberCharts(name string) {
	px := fmt.Sprintf("%s%s_", chartPxReplSetMember, name)

	for _, chart := range *m.Charts() {
		if strings.HasPrefix(chart.ID, px) {
			chart.MarkRemove()
			chart.MarkNotCreated()
		}
	}
}
