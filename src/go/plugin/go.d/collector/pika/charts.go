// SPDX-License-Identifier: GPL-3.0-or-later

package pika

import "github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"

var pikaCharts = collectorapi.Charts{
	chartConnections.Copy(),
	chartClients.Copy(),

	chartMemory.Copy(),

	chartConnectedReplicas.Copy(),

	chartCommands.Copy(),
	chartCommandsCalls.Copy(),

	chartDbStringsKeys.Copy(),
	chartDbStringsExpiresKeys.Copy(),
	chartDbStringsInvalidKeys.Copy(),
	chartDbHashesKeys.Copy(),
	chartDbHashesExpiresKeys.Copy(),
	chartDbHashesInvalidKeys.Copy(),
	chartDbListsKeys.Copy(),
	chartDbListsExpiresKeys.Copy(),
	chartDbListsInvalidKeys.Copy(),
	chartDbZsetsKeys.Copy(),
	chartDbZsetsExpiresKeys.Copy(),
	chartDbZsetsInvalidKeys.Copy(),
	chartDbSetsKeys.Copy(),
	chartDbSetsExpiresKeys.Copy(),
	chartDbSetsInvalidKeys.Copy(),

	chartUptime.Copy(),
}

var (
	chartConnections = collectorapi.Chart{
		ID:    "connections",
		Title: "Connections",
		Units: "connections/s",
		Fam:   "connections",
		Ctx:   "pika.connections",
		Dims: collectorapi.Dims{
			{ID: "total_connections_received", Name: "accepted", Algo: collectorapi.Incremental},
		},
	}
	chartClients = collectorapi.Chart{
		ID:    "clients",
		Title: "Clients",
		Units: "clients",
		Fam:   "connections",
		Ctx:   "pika.clients",
		Dims: collectorapi.Dims{
			{ID: "connected_clients", Name: "connected"},
		},
	}
)

var (
	chartMemory = collectorapi.Chart{
		ID:    "memory",
		Title: "Memory usage",
		Units: "bytes",
		Fam:   "memory",
		Ctx:   "pika.memory",
		Type:  collectorapi.Area,
		Dims: collectorapi.Dims{
			{ID: "used_memory", Name: "used"},
		},
	}
)

var (
	chartConnectedReplicas = collectorapi.Chart{
		ID:    "connected_replicas",
		Title: "Connected replicas",
		Units: "replicas",
		Fam:   "replication",
		Ctx:   "pika.connected_replicas",
		Dims: collectorapi.Dims{
			{ID: "connected_slaves", Name: "connected"},
		},
	}
)

var (
	chartCommands = collectorapi.Chart{
		ID:    "commands",
		Title: "Processed commands",
		Units: "commands/s",
		Fam:   "commands",
		Ctx:   "pika.commands",
		Dims: collectorapi.Dims{
			{ID: "total_commands_processed", Name: "processed", Algo: collectorapi.Incremental},
		},
	}
	chartCommandsCalls = collectorapi.Chart{
		ID:    "commands_calls",
		Title: "Calls per command",
		Units: "calls/s",
		Fam:   "commands",
		Ctx:   "pika.commands_calls",
		Type:  collectorapi.Stacked,
	}
)

var (
	chartDbStringsKeys = collectorapi.Chart{
		ID:    "database_strings_keys",
		Title: "Strings type keys per database",
		Units: "keys",
		Fam:   "keyspace strings",
		Ctx:   "pika.database_strings_keys",
		Type:  collectorapi.Stacked,
	}
	chartDbStringsExpiresKeys = collectorapi.Chart{
		ID:    "database_strings_expires_keys",
		Title: "Strings type expires keys per database",
		Units: "keys",
		Fam:   "keyspace strings",
		Ctx:   "pika.database_strings_expires_keys",
		Type:  collectorapi.Stacked,
	}
	chartDbStringsInvalidKeys = collectorapi.Chart{
		ID:    "database_strings_invalid_keys",
		Title: "Strings type invalid keys per database",
		Units: "keys",
		Fam:   "keyspace strings",
		Ctx:   "pika.database_strings_invalid_keys",
		Type:  collectorapi.Stacked,
	}

	chartDbHashesKeys = collectorapi.Chart{
		ID:    "database_hashes_keys",
		Title: "Hashes type keys per database",
		Units: "keys",
		Fam:   "keyspace hashes",
		Ctx:   "pika.database_hashes_keys",
		Type:  collectorapi.Stacked,
	}
	chartDbHashesExpiresKeys = collectorapi.Chart{
		ID:    "database_hashes_expires_keys",
		Title: "Hashes type expires keys per database",
		Units: "keys",
		Fam:   "keyspace hashes",
		Ctx:   "pika.database_hashes_expires_keys",
		Type:  collectorapi.Stacked,
	}
	chartDbHashesInvalidKeys = collectorapi.Chart{
		ID:    "database_hashes_invalid_keys",
		Title: "Hashes type invalid keys per database",
		Units: "keys",
		Fam:   "keyspace hashes",
		Ctx:   "pika.database_hashes_invalid_keys",
		Type:  collectorapi.Stacked,
	}

	chartDbListsKeys = collectorapi.Chart{
		ID:    "database_lists_keys",
		Title: "Lists type keys per database",
		Units: "keys",
		Fam:   "keyspace lists",
		Ctx:   "pika.database_lists_keys",
		Type:  collectorapi.Stacked,
	}
	chartDbListsExpiresKeys = collectorapi.Chart{
		ID:    "database_lists_expires_keys",
		Title: "Lists type expires keys per database",
		Units: "keys",
		Fam:   "keyspace lists",
		Ctx:   "pika.database_lists_expires_keys",
		Type:  collectorapi.Stacked,
	}
	chartDbListsInvalidKeys = collectorapi.Chart{
		ID:    "database_lists_invalid_keys",
		Title: "Lists type invalid keys per database",
		Units: "keys",
		Fam:   "keyspace lists",
		Ctx:   "pika.database_lists_invalid_keys",
		Type:  collectorapi.Stacked,
	}

	chartDbZsetsKeys = collectorapi.Chart{
		ID:    "database_zsets_keys",
		Title: "Zsets type keys per database",
		Units: "keys",
		Fam:   "keyspace zsets",
		Ctx:   "pika.database_zsets_keys",
		Type:  collectorapi.Stacked,
	}
	chartDbZsetsExpiresKeys = collectorapi.Chart{
		ID:    "database_zsets_expires_keys",
		Title: "Zsets type expires keys per database",
		Units: "keys",
		Fam:   "keyspace zsets",
		Ctx:   "pika.database_zsets_expires_keys",
		Type:  collectorapi.Stacked,
	}
	chartDbZsetsInvalidKeys = collectorapi.Chart{
		ID:    "database_zsets_invalid_keys",
		Title: "Zsets type invalid keys per database",
		Units: "keys",
		Fam:   "keyspace zsets",
		Ctx:   "pika.database_zsets_invalid_keys",
		Type:  collectorapi.Stacked,
	}

	chartDbSetsKeys = collectorapi.Chart{
		ID:    "database_sets_keys",
		Title: "Sets type keys per database",
		Units: "keys",
		Fam:   "keyspace sets",
		Ctx:   "pika.database_sets_keys",
		Type:  collectorapi.Stacked,
	}
	chartDbSetsExpiresKeys = collectorapi.Chart{
		ID:    "database_sets_expires_keys",
		Title: "Sets type expires keys per database",
		Units: "keys",
		Fam:   "keyspace sets",
		Ctx:   "pika.database_sets_expires_keys",
		Type:  collectorapi.Stacked,
	}
	chartDbSetsInvalidKeys = collectorapi.Chart{
		ID:    "database_sets_invalid_keys",
		Title: "Sets invalid keys per database",
		Units: "keys",
		Fam:   "keyspace sets",
		Ctx:   "pika.database_sets_invalid_keys",
		Type:  collectorapi.Stacked,
	}
)

var (
	chartUptime = collectorapi.Chart{
		ID:    "uptime",
		Title: "Uptime",
		Units: "seconds",
		Fam:   "uptime",
		Ctx:   "pika.uptime",
		Dims: collectorapi.Dims{
			{ID: "uptime_in_seconds", Name: "uptime"},
		},
	}
)
