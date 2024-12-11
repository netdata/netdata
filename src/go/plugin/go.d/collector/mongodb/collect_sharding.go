// SPDX-License-Identifier: GPL-3.0-or-later

package mongo

import (
	"fmt"
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
)

func (c *Collector) collectSharding(mx map[string]int64) error {
	nodes, err := c.conn.shardNodes()
	if err != nil {
		return err
	}

	mx["shard_nodes_aware"] = nodes.ShardAware
	mx["shard_nodes_unaware"] = nodes.ShardUnaware

	dbPart, err := c.conn.shardDatabasesPartitioning()
	if err != nil {
		return err
	}

	mx["shard_databases_partitioned"] = dbPart.Partitioned
	mx["shard_databases_unpartitioned"] = dbPart.UnPartitioned

	collPart, err := c.conn.shardCollectionsPartitioning()
	if err != nil {
		return err
	}

	mx["shard_collections_partitioned"] = collPart.Partitioned
	mx["shard_collections_unpartitioned"] = collPart.UnPartitioned

	chunksPerShard, err := c.conn.shardChunks()
	if err != nil {
		return err
	}

	seen := make(map[string]bool)

	for shard, count := range chunksPerShard {
		seen[shard] = true
		mx["shard_id_"+shard+"_chunks"] = count
	}

	for id := range seen {
		if !c.shards[id] {
			c.shards[id] = true
			c.addShardCharts(id)
		}
	}

	for id := range c.shards {
		if !seen[id] {
			delete(c.shards, id)
			c.removeShardCharts(id)
		}
	}

	return nil
}

func (c *Collector) addShardCharts(id string) {
	charts := chartsTmplShardingShard.Copy()

	for _, chart := range *charts {
		chart.ID = fmt.Sprintf(chart.ID, id)
		chart.Labels = []module.Label{
			{Key: "shard_id", Value: id},
		}
		for _, dim := range chart.Dims {
			dim.ID = fmt.Sprintf(dim.ID, id)
		}
	}

	if err := c.Charts().Add(*charts...); err != nil {
		c.Warning(err)
	}

}

func (c *Collector) removeShardCharts(id string) {
	px := fmt.Sprintf("%s%s_", chartPxShard, id)

	for _, chart := range *c.Charts() {
		if strings.HasPrefix(chart.ID, px) {
			chart.MarkRemove()
			chart.MarkNotCreated()
		}
	}
}

func (c *Collector) addShardingCharts() {
	charts := chartsSharding.Copy()

	if err := c.Charts().Add(*charts...); err != nil {
		c.Warning(err)
	}
}
