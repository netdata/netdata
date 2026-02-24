// SPDX-License-Identifier: GPL-3.0-or-later

package redis

import "github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"

const (
	prioConnections = collectorapi.Priority + iota
	prioClients

	prioPingLatency
	prioCommands
	prioKeyLookupHitRate

	prioMemory
	prioMemoryFragmentationRatio
	prioKeyEviction

	prioNet

	prioConnectedReplicas
	prioMasterLinkStatus
	prioMasterLastIOSinceTime
	prioMasterLinkDownSinceTime

	prioPersistenceRDBChanges
	prioPersistenceRDBBgSaveNow
	prioPersistenceRDBBgSaveHealth
	prioPersistenceRDBBgSaveLastSaveSinceTime
	prioPersistenceAOFSize

	prioCommandsCalls
	prioCommandsUsec
	prioCommandsUsecPerSec

	prioKeyExpiration
	prioKeys
	prioExpiresKeys

	prioUptime
)

var redisCharts = collectorapi.Charts{
	chartConnections.Copy(),
	chartClients.Copy(),

	pingLatencyCommands.Copy(),
	chartCommands.Copy(),
	chartKeyLookupHitRate.Copy(),

	chartMemory.Copy(),
	chartMemoryFragmentationRatio.Copy(),
	chartKeyEviction.Copy(),

	chartNet.Copy(),

	chartConnectedReplicas.Copy(),

	chartPersistenceRDBChanges.Copy(),
	chartPersistenceRDBBgSaveNow.Copy(),
	chartPersistenceRDBBgSaveHealth.Copy(),
	chartPersistenceRDBLastSaveSinceTime.Copy(),

	chartCommandsCalls.Copy(),
	chartCommandsUsec.Copy(),
	chartCommandsUsecPerSec.Copy(),

	chartKeyExpiration.Copy(),
	chartKeys.Copy(),
	chartExpiresKeys.Copy(),

	chartUptime.Copy(),
}

var (
	chartConnections = collectorapi.Chart{
		ID:       "connections",
		Title:    "Accepted and rejected (maxclients limit) connections",
		Units:    "connections/s",
		Fam:      "connections",
		Ctx:      "redis.connections",
		Priority: prioConnections,
		Dims: collectorapi.Dims{
			{ID: "total_connections_received", Name: "accepted", Algo: collectorapi.Incremental},
			{ID: "rejected_connections", Name: "rejected", Algo: collectorapi.Incremental},
		},
	}
	chartClients = collectorapi.Chart{
		ID:       "clients",
		Title:    "Clients",
		Units:    "clients",
		Fam:      "connections",
		Ctx:      "redis.clients",
		Priority: prioClients,
		Dims: collectorapi.Dims{
			{ID: "connected_clients", Name: "connected"},
			{ID: "blocked_clients", Name: "blocked"},
			{ID: "tracking_clients", Name: "tracking"},
			{ID: "clients_in_timeout_table", Name: "in_timeout_table"},
		},
	}
)

var (
	pingLatencyCommands = collectorapi.Chart{
		ID:       "ping_latency",
		Title:    "Ping latency",
		Units:    "seconds",
		Fam:      "performance",
		Ctx:      "redis.ping_latency",
		Priority: prioPingLatency,
		Type:     collectorapi.Area,
		Dims: collectorapi.Dims{
			{ID: "ping_latency_min", Name: "min", Div: 1e6},
			{ID: "ping_latency_max", Name: "max", Div: 1e6},
			{ID: "ping_latency_avg", Name: "avg", Div: 1e6},
		},
	}
	chartCommands = collectorapi.Chart{
		ID:       "commands",
		Title:    "Processed commands",
		Units:    "commands/s",
		Fam:      "performance",
		Ctx:      "redis.commands",
		Priority: prioCommands,
		Dims: collectorapi.Dims{
			{ID: "total_commands_processed", Name: "processed", Algo: collectorapi.Incremental},
		},
	}
	chartKeyLookupHitRate = collectorapi.Chart{
		ID:       "key_lookup_hit_rate",
		Title:    "Keys lookup hit rate",
		Units:    "percentage",
		Fam:      "performance",
		Ctx:      "redis.keyspace_lookup_hit_rate",
		Priority: prioKeyLookupHitRate,
		Dims: collectorapi.Dims{
			{ID: "keyspace_hit_rate", Name: "lookup_hit_rate", Div: precision},
		},
	}
)

var (
	chartMemory = collectorapi.Chart{
		ID:       "memory",
		Title:    "Memory usage",
		Units:    "bytes",
		Fam:      "memory",
		Ctx:      "redis.memory",
		Type:     collectorapi.Area,
		Priority: prioMemory,
		Dims: collectorapi.Dims{
			{ID: "maxmemory", Name: "max"},
			{ID: "used_memory", Name: "used"},
			{ID: "used_memory_rss", Name: "rss"},
			{ID: "used_memory_peak", Name: "peak"},
			{ID: "used_memory_dataset", Name: "dataset"},
			{ID: "used_memory_lua", Name: "lua"},
			{ID: "used_memory_scripts", Name: "scripts"},
		},
	}
	chartMemoryFragmentationRatio = collectorapi.Chart{
		ID:       "mem_fragmentation_ratio",
		Title:    "Ratio between used_memory_rss and used_memory",
		Units:    "ratio",
		Fam:      "memory",
		Ctx:      "redis.mem_fragmentation_ratio",
		Priority: prioMemoryFragmentationRatio,
		Dims: collectorapi.Dims{
			{ID: "mem_fragmentation_ratio", Name: "mem_fragmentation", Div: precision},
		},
	}
	chartKeyEviction = collectorapi.Chart{
		ID:       "key_eviction_events",
		Title:    "Evicted keys due to maxmemory limit",
		Units:    "keys/s",
		Fam:      "memory",
		Ctx:      "redis.key_eviction_events",
		Priority: prioKeyEviction,
		Dims: collectorapi.Dims{
			{ID: "evicted_keys", Name: "evicted", Algo: collectorapi.Incremental},
		},
	}
)

var (
	chartNet = collectorapi.Chart{
		ID:       "net",
		Title:    "Bandwidth",
		Units:    "kilobits/s",
		Fam:      "network",
		Ctx:      "redis.net",
		Type:     collectorapi.Area,
		Priority: prioNet,
		Dims: collectorapi.Dims{
			{ID: "total_net_input_bytes", Name: "received", Mul: 8, Div: 1024, Algo: collectorapi.Incremental},
			{ID: "total_net_output_bytes", Name: "sent", Mul: -8, Div: 1024, Algo: collectorapi.Incremental},
		},
	}
)

var (
	chartPersistenceRDBChanges = collectorapi.Chart{
		ID:       "persistence",
		Title:    "Operations that produced changes since the last SAVE or BGSAVE",
		Units:    "operations",
		Fam:      "persistence",
		Ctx:      "redis.rdb_changes",
		Priority: prioPersistenceRDBChanges,
		Dims: collectorapi.Dims{
			{ID: "rdb_changes_since_last_save", Name: "changes"},
		},
	}
	chartPersistenceRDBBgSaveNow = collectorapi.Chart{
		ID:       "bgsave_now",
		Title:    "Duration of the on-going RDB save operation if any",
		Units:    "seconds",
		Fam:      "persistence",
		Ctx:      "redis.bgsave_now",
		Priority: prioPersistenceRDBBgSaveNow,
		Dims: collectorapi.Dims{
			{ID: "rdb_current_bgsave_time_sec", Name: "current_bgsave_time"},
		},
	}
	chartPersistenceRDBBgSaveHealth = collectorapi.Chart{
		ID:       "bgsave_health",
		Title:    "Status of the last RDB save operation (0: ok, 1: err)",
		Units:    "status",
		Fam:      "persistence",
		Ctx:      "redis.bgsave_health",
		Priority: prioPersistenceRDBBgSaveHealth,
		Dims: collectorapi.Dims{
			{ID: "rdb_last_bgsave_status", Name: "last_bgsave"},
		},
	}
	chartPersistenceRDBLastSaveSinceTime = collectorapi.Chart{
		ID:       "bgsave_last_rdb_save_since_time",
		Title:    "Time elapsed since the last successful RDB save",
		Units:    "seconds",
		Fam:      "persistence",
		Ctx:      "redis.bgsave_last_rdb_save_since_time",
		Priority: prioPersistenceRDBBgSaveLastSaveSinceTime,
		Dims: collectorapi.Dims{
			{ID: "rdb_last_save_time", Name: "last_bgsave_time"},
		},
	}

	chartPersistenceAOFSize = collectorapi.Chart{
		ID:       "persistence_aof_size",
		Title:    "AOF file size",
		Units:    "bytes",
		Fam:      "persistence",
		Ctx:      "redis.aof_file_size",
		Priority: prioPersistenceAOFSize,
		Dims: collectorapi.Dims{
			{ID: "aof_current_size", Name: "current"},
			{ID: "aof_base_size", Name: "base"},
		},
	}
)

var (
	chartCommandsCalls = collectorapi.Chart{
		ID:       "commands_calls",
		Title:    "Calls per command",
		Units:    "calls/s",
		Fam:      "commands",
		Ctx:      "redis.commands_calls",
		Type:     collectorapi.Stacked,
		Priority: prioCommandsCalls,
	}
	chartCommandsUsec = collectorapi.Chart{
		ID:       "commands_usec",
		Title:    "Total CPU time consumed by the commands",
		Units:    "microseconds",
		Fam:      "commands",
		Ctx:      "redis.commands_usec",
		Type:     collectorapi.Stacked,
		Priority: prioCommandsUsec,
	}
	chartCommandsUsecPerSec = collectorapi.Chart{
		ID:       "commands_usec_per_sec",
		Title:    "Average CPU consumed per command execution",
		Units:    "microseconds/s",
		Fam:      "commands",
		Ctx:      "redis.commands_usec_per_sec",
		Priority: prioCommandsUsecPerSec,
	}
)

var (
	chartKeyExpiration = collectorapi.Chart{
		ID:       "key_expiration_events",
		Title:    "Expired keys",
		Units:    "keys/s",
		Fam:      "keyspace",
		Ctx:      "redis.key_expiration_events",
		Priority: prioKeyExpiration,
		Dims: collectorapi.Dims{
			{ID: "expired_keys", Name: "expired", Algo: collectorapi.Incremental},
		},
	}
	chartKeys = collectorapi.Chart{
		ID:       "keys",
		Title:    "Keys per database",
		Units:    "keys",
		Fam:      "keyspace",
		Ctx:      "redis.database_keys",
		Type:     collectorapi.Stacked,
		Priority: prioKeys,
	}
	chartExpiresKeys = collectorapi.Chart{
		ID:       "expires_keys",
		Title:    "Keys with an expiration per database",
		Units:    "keys",
		Fam:      "keyspace",
		Ctx:      "redis.database_expires_keys",
		Type:     collectorapi.Stacked,
		Priority: prioExpiresKeys,
	}
)

var (
	chartConnectedReplicas = collectorapi.Chart{
		ID:       "connected_replicas",
		Title:    "Connected replicas",
		Units:    "replicas",
		Fam:      "replication",
		Ctx:      "redis.connected_replicas",
		Priority: prioConnectedReplicas,
		Dims: collectorapi.Dims{
			{ID: "connected_slaves", Name: "connected"},
		},
	}
	masterLinkStatusChart = collectorapi.Chart{
		ID:       "master_last_status",
		Title:    "Master link status",
		Units:    "status",
		Fam:      "replication",
		Ctx:      "redis.master_link_status",
		Priority: prioMasterLinkStatus,
		Dims: collectorapi.Dims{
			{ID: "master_link_status_up", Name: "up"},
			{ID: "master_link_status_down", Name: "down"},
		},
	}
	masterLastIOSinceTimeChart = collectorapi.Chart{
		ID:       "master_last_io_since_time",
		Title:    "Time elapsed since the last interaction with master",
		Units:    "seconds",
		Fam:      "replication",
		Ctx:      "redis.master_last_io_since_time",
		Priority: prioMasterLastIOSinceTime,
		Dims: collectorapi.Dims{
			{ID: "master_last_io_seconds_ago", Name: "time"},
		},
	}
	masterLinkDownSinceTimeChart = collectorapi.Chart{
		ID:       "master_link_down_since_stime",
		Title:    "Time elapsed since the link between master and slave is down",
		Units:    "seconds",
		Fam:      "replication",
		Ctx:      "redis.master_link_down_since_time",
		Priority: prioMasterLinkDownSinceTime,
		Dims: collectorapi.Dims{
			{ID: "master_link_down_since_seconds", Name: "time"},
		},
	}
)

var (
	chartUptime = collectorapi.Chart{
		ID:       "uptime",
		Title:    "Uptime",
		Units:    "seconds",
		Fam:      "uptime",
		Ctx:      "redis.uptime",
		Priority: prioUptime,
		Dims: collectorapi.Dims{
			{ID: "uptime_in_seconds", Name: "uptime"},
		},
	}
)
