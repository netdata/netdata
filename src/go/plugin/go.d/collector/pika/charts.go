// SPDX-License-Identifier: GPL-3.0-or-later

package pika

import "github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"

var pikaCharts = module.Charts{
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
	chartConnections = module.Chart{
		ID:    "connections",
		Title: "Connections",
		Units: "connections/s",
		Fam:   "connections",
		Ctx:   "pika.connections",
		Dims: module.Dims{
			{ID: "total_connections_received", Name: "accepted", Algo: module.Incremental},
		},
	}
	chartClients = module.Chart{
		ID:    "clients",
		Title: "Clients",
		Units: "clients",
		Fam:   "connections",
		Ctx:   "pika.clients",
		Dims: module.Dims{
			{ID: "connected_clients", Name: "connected"},
		},
	}
)

var (
	chartMemory = module.Chart{
		ID:    "memory",
		Title: "Memory usage",
		Units: "bytes",
		Fam:   "memory",
		Ctx:   "pika.memory",
		Type:  module.Area,
		Dims: module.Dims{
			{ID: "used_memory", Name: "used"},
		},
	}
)

var (
	chartConnectedReplicas = module.Chart{
		ID:    "connected_replicas",
		Title: "Connected replicas",
		Units: "replicas",
		Fam:   "replication",
		Ctx:   "pika.connected_replicas",
		Dims: module.Dims{
			{ID: "connected_slaves", Name: "connected"},
		},
	}
)

var (
	chartCommands = module.Chart{
		ID:    "commands",
		Title: "Processed commands",
		Units: "commands/s",
		Fam:   "commands",
		Ctx:   "pika.commands",
		Dims: module.Dims{
			{ID: "total_commands_processed", Name: "processed", Algo: module.Incremental},
		},
	}
	chartCommandsCalls = module.Chart{
		ID:    "commands_calls",
		Title: "Calls per command",
		Units: "calls/s",
		Fam:   "commands",
		Ctx:   "pika.commands_calls",
		Type:  module.Stacked,
	}
)

var (
	chartDbStringsKeys = module.Chart{
		ID:    "database_strings_keys",
		Title: "Strings type keys per database",
		Units: "keys",
		Fam:   "keyspace strings",
		Ctx:   "pika.database_strings_keys",
		Type:  module.Stacked,
	}
	chartDbStringsExpiresKeys = module.Chart{
		ID:    "database_strings_expires_keys",
		Title: "Strings type expires keys per database",
		Units: "keys",
		Fam:   "keyspace strings",
		Ctx:   "pika.database_strings_expires_keys",
		Type:  module.Stacked,
	}
	chartDbStringsInvalidKeys = module.Chart{
		ID:    "database_strings_invalid_keys",
		Title: "Strings type invalid keys per database",
		Units: "keys",
		Fam:   "keyspace strings",
		Ctx:   "pika.database_strings_invalid_keys",
		Type:  module.Stacked,
	}

	chartDbHashesKeys = module.Chart{
		ID:    "database_hashes_keys",
		Title: "Hashes type keys per database",
		Units: "keys",
		Fam:   "keyspace hashes",
		Ctx:   "pika.database_hashes_keys",
		Type:  module.Stacked,
	}
	chartDbHashesExpiresKeys = module.Chart{
		ID:    "database_hashes_expires_keys",
		Title: "Hashes type expires keys per database",
		Units: "keys",
		Fam:   "keyspace hashes",
		Ctx:   "pika.database_hashes_expires_keys",
		Type:  module.Stacked,
	}
	chartDbHashesInvalidKeys = module.Chart{
		ID:    "database_hashes_invalid_keys",
		Title: "Hashes type invalid keys per database",
		Units: "keys",
		Fam:   "keyspace hashes",
		Ctx:   "pika.database_hashes_invalid_keys",
		Type:  module.Stacked,
	}

	chartDbListsKeys = module.Chart{
		ID:    "database_lists_keys",
		Title: "Lists type keys per database",
		Units: "keys",
		Fam:   "keyspace lists",
		Ctx:   "pika.database_lists_keys",
		Type:  module.Stacked,
	}
	chartDbListsExpiresKeys = module.Chart{
		ID:    "database_lists_expires_keys",
		Title: "Lists type expires keys per database",
		Units: "keys",
		Fam:   "keyspace lists",
		Ctx:   "pika.database_lists_expires_keys",
		Type:  module.Stacked,
	}
	chartDbListsInvalidKeys = module.Chart{
		ID:    "database_lists_invalid_keys",
		Title: "Lists type invalid keys per database",
		Units: "keys",
		Fam:   "keyspace lists",
		Ctx:   "pika.database_lists_invalid_keys",
		Type:  module.Stacked,
	}

	chartDbZsetsKeys = module.Chart{
		ID:    "database_zsets_keys",
		Title: "Zsets type keys per database",
		Units: "keys",
		Fam:   "keyspace zsets",
		Ctx:   "pika.database_zsets_keys",
		Type:  module.Stacked,
	}
	chartDbZsetsExpiresKeys = module.Chart{
		ID:    "database_zsets_expires_keys",
		Title: "Zsets type expires keys per database",
		Units: "keys",
		Fam:   "keyspace zsets",
		Ctx:   "pika.database_zsets_expires_keys",
		Type:  module.Stacked,
	}
	chartDbZsetsInvalidKeys = module.Chart{
		ID:    "database_zsets_invalid_keys",
		Title: "Zsets type invalid keys per database",
		Units: "keys",
		Fam:   "keyspace zsets",
		Ctx:   "pika.database_zsets_invalid_keys",
		Type:  module.Stacked,
	}

	chartDbSetsKeys = module.Chart{
		ID:    "database_sets_keys",
		Title: "Sets type keys per database",
		Units: "keys",
		Fam:   "keyspace sets",
		Ctx:   "pika.database_sets_keys",
		Type:  module.Stacked,
	}
	chartDbSetsExpiresKeys = module.Chart{
		ID:    "database_sets_expires_keys",
		Title: "Sets type expires keys per database",
		Units: "keys",
		Fam:   "keyspace sets",
		Ctx:   "pika.database_sets_expires_keys",
		Type:  module.Stacked,
	}
	chartDbSetsInvalidKeys = module.Chart{
		ID:    "database_sets_invalid_keys",
		Title: "Sets invalid keys per database",
		Units: "keys",
		Fam:   "keyspace sets",
		Ctx:   "pika.database_sets_invalid_keys",
		Type:  module.Stacked,
	}
)

var (
	chartUptime = module.Chart{
		ID:    "uptime",
		Title: "Uptime",
		Units: "seconds",
		Fam:   "uptime",
		Ctx:   "pika.uptime",
		Dims: module.Dims{
			{ID: "uptime_in_seconds", Name: "uptime"},
		},
	}
)
