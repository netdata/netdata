// SPDX-License-Identifier: GPL-3.0-or-later

package redis

import (
	"context"
	"strings"
	"time"
)

// garnetFieldMap renames Garnet INFO fields to their Redis equivalents.
// Only semantics-equivalent fields are mapped:
//   - proc_physical_memory_size is Process.WorkingSet64 (Garnet SystemMetrics),
//     the same "resident set size as seen by the OS" value Redis reports as
//     used_memory_rss.
//   - total_found/total_notfound are the store lookup hit/miss counters Garnet
//     computes garnet_hit_rate from, equivalent to keyspace_hits/keyspace_misses.
var garnetFieldMap = map[string]string{
	"proc_physical_memory_size": "used_memory_rss",
	"total_found":               "keyspace_hits",
	"total_notfound":            "keyspace_misses",
}

// garnetKeyspaceRefreshInterval is the default value of the collector's
// garnetKeyspaceRefreshInterval field: Garnet populates the keyspace section
// with a full per-database store scan (it has no O(1) key counter) and
// excludes it from INFO all for that reason. Key counts change slowly, so a
// value that is up to one interval old is acceptable.
const garnetKeyspaceRefreshInterval = 30 * time.Second

// appendInfoSection appends an INFO section to an INFO document, joining the
// two with exactly one newline. INFO replies do not guarantee a trailing
// newline, and the parser needs every field on its own line.
func appendInfoSection(info, section string) string {
	if section == "" {
		return info
	}
	if info != "" && !strings.HasSuffix(info, "\n") {
		info += "\n"
	}
	return info + strings.TrimRight(section, "\n") + "\n"
}

// collectGarnetExtraInfo appends the INFO sections Garnet excludes from `all`
// and returns the extended INFO text:
//   - keyspace: db/key counts, requested at most once per refresh interval
//     (also on error, so a failed scan waits out the interval); a failed
//     refresh clears the cached section so stale key counts are not reported;
//   - commandstats: redis-format per-command stats, requested every cycle,
//     populated only when the Garnet server runs with --commandstats-monitor;
//     the disabled response contains no parseable fields.
func (c *Collector) collectGarnetExtraInfo(info string) string {
	if time.Since(c.garnetKeyspaceRefreshedAt) >= c.garnetKeyspaceRefreshInterval {
		c.garnetKeyspaceRefreshedAt = time.Now()
		keyspace, err := c.rdb.Info(context.Background(), "keyspace").Result()
		if err != nil {
			c.Warningf("garnet: error on INFO keyspace: %v", err)
			c.garnetKeyspaceInfo = ""
		} else {
			c.garnetKeyspaceInfo = keyspace
		}
	}
	info = appendInfoSection(info, c.garnetKeyspaceInfo)

	commandstats, err := c.rdb.Info(context.Background(), "commandstats").Result()
	if err != nil {
		// Requested every cycle (unlike the throttled keyspace scan), so a
		// persistent failure must not log a warning on every collection.
		c.Debugf("garnet: error on INFO commandstats: %v", err)
		return info
	}
	return appendInfoSection(info, commandstats)
}
