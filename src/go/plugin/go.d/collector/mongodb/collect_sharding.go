// SPDX-License-Identifier: GPL-3.0-or-later

package mongo

import (
	"fmt"
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
)

func (m *Mongo) collectSharding(mx map[string]int64) error {
	nodes, err := m.conn.shardNodes()
	if err != nil {
		return err
	}

	mx["shard_nodes_aware"] = nodes.ShardAware
	mx["shard_nodes_unaware"] = nodes.ShardUnaware

	dbPart, err := m.conn.shardDatabasesPartitioning()
	if err != nil {
		return err
	}

	mx["shard_databases_partitioned"] = dbPart.Partitioned
	mx["shard_databases_unpartitioned"] = dbPart.UnPartitioned

	collPart, err := m.conn.shardCollectionsPartitioning()
	if err != nil {
		return err
	}

	mx["shard_collections_partitioned"] = collPart.Partitioned
	mx["shard_collections_unpartitioned"] = collPart.UnPartitioned

	chunksPerShard, err := m.conn.shardChunks()
	if err != nil {
		return err
	}

	seen := make(map[string]bool)

	for shard, count := range chunksPerShard {
		seen[shard] = true
		mx["shard_id_"+shard+"_chunks"] = count
	}

	for id := range seen {
		if !m.shards[id] {
			m.shards[id] = true
			m.addShardCharts(id)
		}
	}

	for id := range m.shards {
		if !seen[id] {
			delete(m.shards, id)
			m.removeShardCharts(id)
		}
	}

	return nil
}

func (m *Mongo) addShardCharts(id string) {
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

	if err := m.Charts().Add(*charts...); err != nil {
		m.Warning(err)
	}

}

func (m *Mongo) removeShardCharts(id string) {
	px := fmt.Sprintf("%s%s_", chartPxShard, id)

	for _, chart := range *m.Charts() {
		if strings.HasPrefix(chart.ID, px) {
			chart.MarkRemove()
			chart.MarkNotCreated()
		}
	}
}

func (m *Mongo) addShardingCharts() {
	charts := chartsSharding.Copy()

	if err := m.Charts().Add(*charts...); err != nil {
		m.Warning(err)
	}
}
