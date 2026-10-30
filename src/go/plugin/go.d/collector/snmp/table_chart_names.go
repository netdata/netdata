// SPDX-License-Identifier: GPL-3.0-or-later

package snmp

import (
	"strings"
	"unicode/utf8"

	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
)

const tagIfAlias = "_if_alias"

// tableChartState tracks what a table chart was last sent with, so label and name changes are re-sent only when
// the underlying underscore-prefixed tag values (which never affect the chart ID) change.
type tableChartState struct {
	chart      *collectorapi.Chart
	privTags   map[string]string
	aliasName  string
	uniqueName string
	name       string
}

func newTableChartState(chart *collectorapi.Chart, m ddsnmp.Metric) *tableChartState {
	st := &tableChartState{chart: chart}
	st.update(m)
	return st
}

func (st *tableChartState) update(m ddsnmp.Metric) {
	st.privTags = make(map[string]string)
	for k, v := range m.Tags {
		if strings.HasPrefix(k, "_") {
			st.privTags[k] = v
		}
	}

	st.aliasName, st.uniqueName = "", ""
	iface := m.Tags[tagInterface]
	if alias := strings.TrimSpace(m.Tags[tagIfAlias]); alias != "" && iface != "" {
		st.aliasName = sanitizeChartName(tableMetricKeyWith(m, tagInterface, alias))
		st.uniqueName = sanitizeChartName(tableMetricKeyWith(m, tagInterface, alias+"_"+iface))
	}
}

func (st *tableChartState) privateTagsChanged(tags map[string]string) bool {
	n := 0
	for k, v := range tags {
		if !strings.HasPrefix(k, "_") {
			continue
		}
		n++
		if old, ok := st.privTags[k]; !ok || old != v {
			return true
		}
	}
	return n != len(st.privTags)
}

// updateTableChartNames sets each interface chart's display name from its alias. The agent silently refuses a name
// another chart of the device still holds, so names are made unique here, a name is never sent while another chart
// holds it, and a chart moving between two names is reset to its ID first (two ports swapping aliases would
// otherwise block each other forever). Deferred names are retried on the next collection.
func (c *Collector) updateTableChartNames() {
	counts := make(map[string]int)
	held := make(map[string]bool)
	for _, st := range c.tableCharts {
		if st.aliasName != "" {
			counts[st.aliasName]++
		}
		if st.name != "" {
			held[st.name] = true
		}
	}

	c.tableChartNamesPending = false
	for _, st := range c.tableCharts {
		want := st.aliasName
		if want != "" && counts[want] > 1 {
			want = st.uniqueName
		}
		if want == st.name || st.chart == nil {
			continue
		}

		switch {
		case want != "" && st.name != "":
			want = ""
			c.tableChartNamesPending = true
		case want != "" && held[want]:
			c.tableChartNamesPending = true
			continue
		}

		st.name = want
		st.chart.OverID = want
		st.chart.MarkNotCreated()
	}
}

// sanitizeChartName applies the agent's chart name character rules (src/libnetdata/sanitizers/chart_id_and_name.c)
// so that names compared for uniqueness here are the names the agent stores, then maps dots to underscores like chart
// IDs. The result is also safe for the single-quoted, line-based plugins.d protocol.
func sanitizeChartName(s string) string {
	var sb strings.Builder
	sb.Grow(len(s))

	pendingSpace := false
	for _, r := range s {
		if r >= utf8.RuneSelf {
			if r == utf8.RuneError {
				pendingSpace = true
				continue
			}
			if pendingSpace && sb.Len() > 0 {
				sb.WriteByte('_')
			}
			pendingSpace = false
			sb.WriteRune(r)
			continue
		}

		b := byte(r)
		switch {
		case b <= ' ' || b == 0x7f:
			pendingSpace = true
			continue
		case b >= 'a' && b <= 'z', b >= 'A' && b <= 'Z', b >= '0' && b <= '9', strings.IndexByte("!#()-/:;<>[]{}~_", b) >= 0:
		case b == '\\':
			b = '/'
		default:
			b = '_'
		}

		if pendingSpace && sb.Len() > 0 {
			sb.WriteByte('_')
		}
		pendingSpace = false
		sb.WriteByte(b)
	}

	return sb.String()
}
