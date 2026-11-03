// SPDX-License-Identifier: GPL-3.0-or-later

package redis

import (
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/oldmetrix"
)

// INFO section headers, lowercased: Redis-compatible servers may use different casing (e.g. Kvrocks emits
// "# CommandStats").
const (
	infoSectionClients      = "# clients"
	infoSectionStats        = "# stats"
	infoSectionCommandstats = "# commandstats"
	infoSectionKeyspace     = "# keyspace"
)

var (
	reKeyspaceValue     = regexp.MustCompile(`^keys=(\d+),expires=(\d+)`)
	reCommandstatsValue = regexp.MustCompile(`^calls=(\d+),usec=(\d+),usec_per_call=([\d.]+)`)
)

// collectInfo parses an INFO response (https://redis.io/commands/info). Every line is either a section
// header ("# Name") or a "field:value" property.
func (c *Collector) collectInfo(mx map[string]int64, info string) {
	var section string
	var garnetSamplingDisabled bool

	for line := range strings.Lines(info) {
		line = strings.TrimSpace(line)
		if line == "" {
			section = ""
			continue
		}
		if strings.HasPrefix(line, "#") {
			section = strings.ToLower(line)
			continue
		}

		field, value, ok := strings.Cut(line, ":")
		if !ok || field == "" || value == "" {
			continue
		}

		switch {
		case c.server == "garnet" && field == "monitor_task":
			// Garnet's Server section precedes Stats/Clients. Without periodic sampling those
			// sections contain placeholders, even with commandstats enabled.
			garnetSamplingDisabled = value == "disabled"
		case garnetSamplingDisabled && (section == infoSectionStats || section == infoSectionClients):
			continue
		case section == infoSectionCommandstats:
			c.collectCommandstatsProperty(mx, field, value)
		case section == infoSectionKeyspace:
			c.collectKeyspaceProperty(mx, field, value)
		case field == "rejected_connections" && value == "-1":
			// Dragonfly uses -1 when this counter is unsupported.
			continue
		case field == "rdb_last_bgsave_status":
			mx[field] = oldmetrix.Bool(value != "ok")
		case field == "rdb_current_bgsave_time_sec" && value == "-1":
			// -1 means no bgsave is in progress.
			mx[field] = 0
		case field == "rdb_last_save_time":
			if v, err := strconv.ParseInt(value, 10, 64); err == nil {
				mx[field] = int64(time.Since(time.Unix(v, 0)).Seconds())
			}
		case field == "aof_enabled" && value == "1":
			c.addAOFChartsOnce.Do(c.addAOFCharts)
		case field == "master_link_status":
			mx["master_link_status_up"] = oldmetrix.Bool(value == "up")
			mx["master_link_status_down"] = oldmetrix.Bool(value == "down")
		default:
			if c.server == "garnet" {
				if mapped, ok := garnetFieldMap[field]; ok {
					field = mapped
				}
			}
			collectNumericValue(mx, field, value)
		}
	}

	hits, okHits := mx["keyspace_hits"]
	misses, okMisses := mx["keyspace_misses"]
	if okHits && okMisses {
		mx["keyspace_hit_rate"] = int64(keyspaceHitRate(hits, misses) * precision)
	}

	if _, ok := mx["master_last_io_seconds_ago"]; ok {
		c.addReplicaChartsOnce.Do(c.addReplicaCharts)
		if _, ok := mx["master_link_down_since_seconds"]; !ok && mx["master_link_status_up"] == 1 {
			// A missing duration is zero only for an explicitly healthy link.
			mx["master_link_down_since_seconds"] = 0
		}
	}
}

func (c *Collector) collectKeyspaceProperty(mx map[string]int64, db, value string) {
	match := reKeyspaceValue.FindStringSubmatch(value)
	if match == nil {
		return
	}

	keys, expires := match[1], match[2]
	collectNumericValue(mx, db+"_keys", keys)
	collectNumericValue(mx, db+"_expires_keys", expires)

	if !c.collectedDBs[db] {
		c.collectedDBs[db] = true
		c.addDBToKeyspaceCharts(db)
	}
}

func (c *Collector) collectCommandstatsProperty(mx map[string]int64, field, value string) {
	cmd, ok := strings.CutPrefix(field, "cmdstat_")
	if !ok {
		return
	}

	match := reCommandstatsValue.FindStringSubmatch(value)
	if match == nil {
		return
	}

	calls, usec, usecPerCall := match[1], match[2], match[3]
	collectNumericValue(mx, "cmd_"+cmd+"_calls", calls)
	// Garnet reports literal zero timing fields, not measured execution times.
	if c.server != "garnet" {
		collectNumericValue(mx, "cmd_"+cmd+"_usec", usec)
		collectNumericValue(mx, "cmd_"+cmd+"_usec_per_call", usecPerCall)
	}

	if !c.collectedCommands[cmd] {
		c.collectedCommands[cmd] = true
		c.addCmdToCommandsCharts(cmd)
	}
}

// collectNumericValue stores a numeric INFO value, multiplied by precision when it is fractional.
// Non-numeric values are skipped.
func collectNumericValue(mx map[string]int64, key, value string) {
	v, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return
	}
	if strings.Contains(value, ".") {
		v *= precision
	}
	mx[key] = int64(v)
}

func keyspaceHitRate(hits, misses int64) float64 {
	if hits+misses == 0 {
		return 0
	}
	return float64(hits) * 100 / float64(hits+misses)
}
