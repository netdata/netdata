// SPDX-License-Identifier: GPL-3.0-or-later

package redis

import (
	"bufio"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/metrix"
)

const (
	infoSectionServer       = "# Server"
	infoSectionData         = "# Data"
	infoSectionClients      = "# Clients"
	infoSectionStats        = "# Stats"
	infoSectionCommandstats = "# Commandstats"
	infoSectionCPU          = "# CPU"
	infoSectionRepl         = "# Replication"
	infoSectionKeyspace     = "# Keyspace"
)

var infoSections = map[string]struct{}{
	infoSectionServer:       {},
	infoSectionData:         {},
	infoSectionClients:      {},
	infoSectionStats:        {},
	infoSectionCommandstats: {},
	infoSectionCPU:          {},
	infoSectionRepl:         {},
	infoSectionKeyspace:     {},
}

func isInfoSection(line string) bool { _, ok := infoSections[line]; return ok }

func (c *Collector) collectInfo(mx map[string]int64, info string) {
	// https://redis.io/commands/info
	// Lines can contain a section name (starting with a # character) or a property.
	// All the properties are in the form of field:value terminated by \r\n.

	var curSection string
	sc := bufio.NewScanner(strings.NewReader(info))
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if len(line) == 0 {
			curSection = ""
			continue
		}
		if strings.HasPrefix(line, "#") {
			if isInfoSection(line) {
				curSection = line
			}
			continue
		}

		field, value, ok := parseProperty(line)
		if !ok {
			continue
		}

		switch {
		case curSection == infoSectionCommandstats:
			c.collectInfoCommandstatsProperty(mx, field, value)
		case curSection == infoSectionKeyspace:
			c.collectInfoKeyspaceProperty(mx, field, value)
		case field == "rdb_last_bgsave_status":
			collectNumericValue(mx, field, convertBgSaveStatus(value))
		case field == "rdb_current_bgsave_time_sec" && value == "-1":
			// TODO: https://github.com/netdata/dashboard/issues/198
			// "-1" means there is no on-going bgsave operation;
			// netdata has 'Convert seconds to time' feature (enabled by default),
			// looks like it doesn't respect negative values and does abs().
			// "-1" => "00:00:01".
			collectNumericValue(mx, field, "0")
		case field == "rdb_last_save_time":
			v, _ := strconv.ParseInt(value, 10, 64)
			mx[field] = int64(time.Since(time.Unix(v, 0)).Seconds())
		case field == "aof_enabled" && value == "1":
			c.addAOFChartsOnce.Do(c.addAOFCharts)
		case field == "master_link_status":
			mx["master_link_status_up"] = metrix.Bool(value == "up")
			mx["master_link_status_down"] = metrix.Bool(value == "down")
		default:
			collectNumericValue(mx, field, value)
		}
	}

	if has(mx, "keyspace_hits", "keyspace_misses") {
		mx["keyspace_hit_rate"] = int64(calcKeyspaceHitRate(mx) * precision)
	}
	if has(mx, "master_last_io_seconds_ago") {
		c.addReplSlaveChartsOnce.Do(c.addReplSlaveCharts)
		if !has(mx, "master_link_down_since_seconds") {
			mx["master_link_down_since_seconds"] = 0
		}
	}
}

var reKeyspaceValue = regexp.MustCompile(`^keys=(\d+),expires=(\d+)`)

func (c *Collector) collectInfoKeyspaceProperty(ms map[string]int64, field, value string) {
	match := reKeyspaceValue.FindStringSubmatch(value)
	if match == nil {
		return
	}

	keys, expires := match[1], match[2]
	collectNumericValue(ms, field+"_keys", keys)
	collectNumericValue(ms, field+"_expires_keys", expires)

	if !c.collectedDbs[field] {
		c.collectedDbs[field] = true
		c.addDbToKeyspaceCharts(field)
	}
}

var reCommandstatsValue = regexp.MustCompile(`^calls=(\d+),usec=(\d+),usec_per_call=([\d.]+)`)

func (c *Collector) collectInfoCommandstatsProperty(ms map[string]int64, field, value string) {
	if !strings.HasPrefix(field, "cmdstat_") {
		return
	}
	cmd := field[len("cmdstat_"):]

	match := reCommandstatsValue.FindStringSubmatch(value)
	if match == nil {
		return
	}

	calls, usec, usecPerCall := match[1], match[2], match[3]
	collectNumericValue(ms, "cmd_"+cmd+"_calls", calls)
	collectNumericValue(ms, "cmd_"+cmd+"_usec", usec)
	collectNumericValue(ms, "cmd_"+cmd+"_usec_per_call", usecPerCall)

	if !c.collectedCommands[cmd] {
		c.collectedCommands[cmd] = true
		c.addCmdToCommandsCharts(cmd)
	}
}

func collectNumericValue(ms map[string]int64, field, value string) {
	v, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return
	}
	if strings.IndexByte(value, '.') == -1 {
		ms[field] = int64(v)
	} else {
		ms[field] = int64(v * precision)
	}
}

func convertBgSaveStatus(status string) string {
	// https://github.com/redis/redis/blob/unstable/src/server.c
	// "ok" or "err"
	if status == "ok" {
		return "0"
	}
	return "1"
}

func parseProperty(prop string) (field, value string, ok bool) {
	i := strings.IndexByte(prop, ':')
	if i == -1 {
		return "", "", false
	}
	field, value = prop[:i], prop[i+1:]
	return field, value, field != "" && value != ""
}

func calcKeyspaceHitRate(ms map[string]int64) float64 {
	hits := ms["keyspace_hits"]
	misses := ms["keyspace_misses"]
	if hits+misses == 0 {
		return 0
	}
	return float64(hits) * 100 / float64(hits+misses)
}

func (c *Collector) addCmdToCommandsCharts(cmd string) {
	c.addDimToChart(chartCommandsCalls.ID, &module.Dim{
		ID:   "cmd_" + cmd + "_calls",
		Name: strings.ToUpper(cmd),
		Algo: module.Incremental,
	})
	c.addDimToChart(chartCommandsUsec.ID, &module.Dim{
		ID:   "cmd_" + cmd + "_usec",
		Name: strings.ToUpper(cmd),
		Algo: module.Incremental,
	})
	c.addDimToChart(chartCommandsUsecPerSec.ID, &module.Dim{
		ID:   "cmd_" + cmd + "_usec_per_call",
		Name: strings.ToUpper(cmd),
		Div:  precision,
	})
}

func (c *Collector) addDbToKeyspaceCharts(db string) {
	c.addDimToChart(chartKeys.ID, &module.Dim{
		ID:   db + "_keys",
		Name: db,
	})
	c.addDimToChart(chartExpiresKeys.ID, &module.Dim{
		ID:   db + "_expires_keys",
		Name: db,
	})
}

func (c *Collector) addDimToChart(chartID string, dim *module.Dim) {
	chart := c.Charts().Get(chartID)
	if chart == nil {
		c.Warningf("error on adding '%s' dimension: can not find '%s' chart", dim.ID, chartID)
		return
	}
	if err := chart.AddDim(dim); err != nil {
		c.Warning(err)
		return
	}
	chart.MarkNotCreated()
}

func (c *Collector) addAOFCharts() {
	err := c.Charts().Add(chartPersistenceAOFSize.Copy())
	if err != nil {
		c.Warningf("error on adding '%s' chart", chartPersistenceAOFSize.ID)
	}
}

func (c *Collector) addReplSlaveCharts() {
	if err := c.Charts().Add(masterLinkStatusChart.Copy()); err != nil {
		c.Warningf("error on adding '%s' chart", masterLinkStatusChart.ID)
	}
	if err := c.Charts().Add(masterLastIOSinceTimeChart.Copy()); err != nil {
		c.Warningf("error on adding '%s' chart", masterLastIOSinceTimeChart.ID)
	}
	if err := c.Charts().Add(masterLinkDownSinceTimeChart.Copy()); err != nil {
		c.Warningf("error on adding '%s' chart", masterLinkDownSinceTimeChart.ID)
	}
}

func has(m map[string]int64, key string, keys ...string) bool {
	switch _, ok := m[key]; len(keys) {
	case 0:
		return ok
	default:
		return ok && has(m, keys[0], keys[1:]...)
	}
}
