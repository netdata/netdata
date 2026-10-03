// SPDX-License-Identifier: GPL-3.0-or-later

package redis

import (
	"context"
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

// garnetKeyspaceRefreshInterval limits INFO keyspace requests: Garnet
// populates the section with a full per-database store scan (it has no O(1)
// key counter) and excludes it from INFO all for that reason. Key counts
// change slowly, so a value that is up to one interval old is acceptable.
const garnetKeyspaceRefreshInterval = 30 * time.Second

// collectGarnetExtraInfo appends the INFO sections Garnet excludes from `all`
// and returns the extended INFO text:
//   - keyspace: db/key counts, requested at most once per refresh interval
//     (also on error, so a failed scan waits out the interval);
//   - commandstats: redis-format per-command stats, populated only when the
//     Garnet server runs with --commandstats-monitor; the disabled response
//     contains no parseable fields.
func (c *Collector) collectGarnetExtraInfo(info string) string {
	if time.Since(c.garnetKeyspaceRefreshedAt) >= garnetKeyspaceRefreshInterval {
		c.garnetKeyspaceRefreshedAt = time.Now()
		keyspace, err := c.rdb.Info(context.Background(), "keyspace").Result()
		if err != nil {
			c.Debugf("garnet: error on INFO keyspace: %v", err)
		} else {
			c.garnetKeyspaceInfo = keyspace
		}
	}
	info += c.garnetKeyspaceInfo

	commandstats, err := c.rdb.Info(context.Background(), "commandstats").Result()
	if err != nil {
		c.Debugf("garnet: error on INFO commandstats: %v", err)
		return info
	}
	return info + commandstats
}
