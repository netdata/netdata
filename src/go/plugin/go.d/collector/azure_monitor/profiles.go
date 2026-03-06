// SPDX-License-Identifier: GPL-3.0-or-later

package azure_monitor

import "strings"

var builtInProfiles = map[string]ProfileConfig{
	"sql_managed_instance": {
		Name:         "Azure SQL Managed Instance",
		ResourceType: "Microsoft.Sql/managedInstances",
		Metrics: []MetricConfig{
			{Name: "avg_cpu_percent", Units: "percentage", Aggregations: []string{"average"}, TimeGrain: "PT1M"},
			{Name: "io_bytes_read", Units: "bytes/s", Aggregations: []string{"average"}, TimeGrain: "PT1M"},
			{Name: "io_bytes_written", Units: "bytes/s", Aggregations: []string{"average"}, TimeGrain: "PT1M"},
			{Name: "io_requests", Units: "requests/s", Aggregations: []string{"average"}, TimeGrain: "PT1M"},
			{Name: "reserved_storage_mb", Units: "MiB", Aggregations: []string{"average"}, TimeGrain: "PT1M"},
			{Name: "storage_space_used_mb", Units: "MiB", Aggregations: []string{"average"}, TimeGrain: "PT1M"},
			{Name: "virtual_core_count", Units: "cores", Aggregations: []string{"average"}, TimeGrain: "PT1M"},
		},
	},
	"sql_database": {
		Name:         "Azure SQL Database",
		ResourceType: "Microsoft.Sql/servers/databases",
		Metrics: []MetricConfig{
			{Name: "cpu_percent", Units: "percentage", Aggregations: []string{"average"}, TimeGrain: "PT1M"},
			{Name: "dtu_consumption_percent", Units: "percentage", Aggregations: []string{"average"}, TimeGrain: "PT1M"},
			{Name: "storage_percent", Units: "percentage", Aggregations: []string{"average"}, TimeGrain: "PT1M"},
			{Name: "sessions_percent", Units: "percentage", Aggregations: []string{"average"}, TimeGrain: "PT1M"},
			{Name: "workers_percent", Units: "percentage", Aggregations: []string{"average"}, TimeGrain: "PT1M"},
			{Name: "physical_data_read_percent", Units: "percentage", Aggregations: []string{"average"}, TimeGrain: "PT1M"},
			{Name: "log_write_percent", Units: "percentage", Aggregations: []string{"average"}, TimeGrain: "PT1M"},
		},
	},
	"postgres_flexible": {
		Name:         "Azure PostgreSQL Flexible Server",
		ResourceType: "Microsoft.DBforPostgreSQL/flexibleServers",
		Metrics: []MetricConfig{
			{Name: "cpu_percent", Units: "percentage", Aggregations: []string{"average"}, TimeGrain: "PT1M"},
			{Name: "memory_percent", Units: "percentage", Aggregations: []string{"average"}, TimeGrain: "PT1M"},
			{Name: "storage_percent", Units: "percentage", Aggregations: []string{"average"}, TimeGrain: "PT1M"},
			{Name: "iops", Units: "operations/s", Aggregations: []string{"average"}, TimeGrain: "PT1M"},
			{Name: "read_iops", Units: "operations/s", Aggregations: []string{"average"}, TimeGrain: "PT1M"},
			{Name: "write_iops", Units: "operations/s", Aggregations: []string{"average"}, TimeGrain: "PT1M"},
			{Name: "active_connections", Units: "connections", Aggregations: []string{"average"}, TimeGrain: "PT1M"},
			{Name: "tps", Units: "transactions/s", Aggregations: []string{"average"}, TimeGrain: "PT1M"},
			{Name: "deadlocks", Units: "deadlocks/s", Aggregations: []string{"total"}, TimeGrain: "PT1M"},
			{Name: "xact_commit", Units: "transactions/s", Aggregations: []string{"total"}, TimeGrain: "PT1M"},
			{Name: "xact_rollback", Units: "transactions/s", Aggregations: []string{"total"}, TimeGrain: "PT1M"},
			{Name: "is_db_alive", Units: "state", Aggregations: []string{"average"}, TimeGrain: "PT1M"},
			{Name: "network_bytes_egress", Units: "bytes/s", Aggregations: []string{"total"}, TimeGrain: "PT1M"},
			{Name: "network_bytes_ingress", Units: "bytes/s", Aggregations: []string{"total"}, TimeGrain: "PT1M"},
			{Name: "backup_storage_used", Units: "bytes", Aggregations: []string{"average"}, TimeGrain: "PT30M"},
			{Name: "physical_replication_delay_in_seconds", Units: "seconds", Aggregations: []string{"average"}, TimeGrain: "PT30M"},
		},
	},
	"cosmos_db": {
		Name:         "Azure Cosmos DB",
		ResourceType: "Microsoft.DocumentDB/databaseAccounts",
		Metrics: []MetricConfig{
			{Name: "TotalRequests", Units: "requests/s", Aggregations: []string{"total"}, TimeGrain: "PT1M"},
			{Name: "TotalRequestUnits", Units: "RU/s", Aggregations: []string{"total"}, TimeGrain: "PT1M"},
			{Name: "NormalizedRUConsumption", Units: "percentage", Aggregations: []string{"average"}, TimeGrain: "PT1M"},
			{Name: "ServerSideLatencyDirect", Units: "milliseconds", Aggregations: []string{"average"}, TimeGrain: "PT5M"},
			{Name: "ServerSideLatencyGateway", Units: "milliseconds", Aggregations: []string{"average"}, TimeGrain: "PT5M"},
			{Name: "ReplicationLatency", Units: "milliseconds", Aggregations: []string{"average"}, TimeGrain: "PT5M"},
			{Name: "DataUsage", Units: "bytes", Aggregations: []string{"maximum"}, TimeGrain: "PT1H"},
			{Name: "DocumentCount", Units: "documents", Aggregations: []string{"maximum"}, TimeGrain: "PT1H"},
			{Name: "ProvisionedThroughput", Units: "RU/s", Aggregations: []string{"maximum"}, TimeGrain: "PT1H"},
			{Name: "ServiceAvailability", Units: "percentage", Aggregations: []string{"average"}, TimeGrain: "PT5M"},
			{Name: "MongoRequests", Units: "requests/s", Aggregations: []string{"total"}, TimeGrain: "PT1M"},
			{Name: "CassandraRequests", Units: "requests/s", Aggregations: []string{"total"}, TimeGrain: "PT1M"},
		},
	},
	"logic_apps": {
		Name:         "Azure Logic Apps",
		ResourceType: "Microsoft.Logic/workflows",
		Metrics: []MetricConfig{
			{Name: "RunsStarted", Units: "runs/s", Aggregations: []string{"total"}, TimeGrain: "PT1M"},
			{Name: "RunsSucceeded", Units: "runs/s", Aggregations: []string{"total"}, TimeGrain: "PT1M"},
			{Name: "RunsFailed", Units: "runs/s", Aggregations: []string{"total"}, TimeGrain: "PT1M"},
			{Name: "RunLatency", Units: "milliseconds", Aggregations: []string{"average"}, TimeGrain: "PT1M"},
			{Name: "RunSuccessLatency", Units: "milliseconds", Aggregations: []string{"average"}, TimeGrain: "PT1M"},
			{Name: "RunFailurePercentage", Units: "percentage", Aggregations: []string{"average"}, TimeGrain: "PT1M"},
			{Name: "ActionsStarted", Units: "actions/s", Aggregations: []string{"total"}, TimeGrain: "PT1M"},
			{Name: "ActionsSucceeded", Units: "actions/s", Aggregations: []string{"total"}, TimeGrain: "PT1M"},
			{Name: "ActionsFailed", Units: "actions/s", Aggregations: []string{"total"}, TimeGrain: "PT1M"},
			{Name: "TriggersStarted", Units: "triggers/s", Aggregations: []string{"total"}, TimeGrain: "PT1M"},
			{Name: "TriggersFired", Units: "triggers/s", Aggregations: []string{"total"}, TimeGrain: "PT1M"},
			{Name: "TriggersFailed", Units: "triggers/s", Aggregations: []string{"total"}, TimeGrain: "PT1M"},
			{Name: "TotalBillableExecutions", Units: "executions/s", Aggregations: []string{"total"}, TimeGrain: "PT1M"},
			{Name: "RunThrottledEvents", Units: "events/s", Aggregations: []string{"total"}, TimeGrain: "PT1M"},
		},
	},
	"virtual_machines": {
		Name:         "Azure Virtual Machines",
		ResourceType: "Microsoft.Compute/virtualMachines",
		Metrics: []MetricConfig{
			{Name: "Percentage CPU", Units: "percentage", Aggregations: []string{"average"}, TimeGrain: "PT1M"},
			{Name: "Network In Total", Units: "bytes/s", Aggregations: []string{"total"}, TimeGrain: "PT1M"},
			{Name: "Network Out Total", Units: "bytes/s", Aggregations: []string{"total"}, TimeGrain: "PT1M"},
			{Name: "Disk Read Bytes", Units: "bytes/s", Aggregations: []string{"total"}, TimeGrain: "PT1M"},
			{Name: "Disk Write Bytes", Units: "bytes/s", Aggregations: []string{"total"}, TimeGrain: "PT1M"},
		},
	},
	"aks": {
		Name:         "Azure Kubernetes Service",
		ResourceType: "Microsoft.ContainerService/managedClusters",
		Metrics: []MetricConfig{
			{Name: "node_cpu_usage_millicores", Units: "millicores", Aggregations: []string{"average"}, TimeGrain: "PT1M"},
			{Name: "node_memory_working_set_bytes", Units: "bytes", Aggregations: []string{"average"}, TimeGrain: "PT1M"},
			{Name: "node_disk_usage_bytes", Units: "bytes", Aggregations: []string{"average"}, TimeGrain: "PT1M"},
			{Name: "node_network_in_bytes", Units: "bytes/s", Aggregations: []string{"total"}, TimeGrain: "PT1M"},
			{Name: "node_network_out_bytes", Units: "bytes/s", Aggregations: []string{"total"}, TimeGrain: "PT1M"},
		},
	},
	"storage_accounts": {
		Name:         "Azure Storage Accounts",
		ResourceType: "Microsoft.Storage/storageAccounts",
		Metrics: []MetricConfig{
			{Name: "Availability", Units: "percentage", Aggregations: []string{"average"}, TimeGrain: "PT1M"},
			{Name: "Transactions", Units: "transactions/s", Aggregations: []string{"total"}, TimeGrain: "PT1M"},
			{Name: "Ingress", Units: "bytes/s", Aggregations: []string{"total"}, TimeGrain: "PT1M"},
			{Name: "Egress", Units: "bytes/s", Aggregations: []string{"total"}, TimeGrain: "PT1M"},
			{Name: "UsedCapacity", Units: "bytes", Aggregations: []string{"average"}, TimeGrain: "PT1H"},
		},
	},
	"load_balancers": {
		Name:         "Azure Load Balancers",
		ResourceType: "Microsoft.Network/loadBalancers",
		Metrics: []MetricConfig{
			{Name: "VipAvailability", Units: "percentage", Aggregations: []string{"average"}, TimeGrain: "PT1M"},
			{Name: "ByteCount", Units: "bytes/s", Aggregations: []string{"total"}, TimeGrain: "PT1M"},
			{Name: "PacketCount", Units: "packets/s", Aggregations: []string{"total"}, TimeGrain: "PT1M"},
			{Name: "SYNCount", Units: "requests/s", Aggregations: []string{"total"}, TimeGrain: "PT1M"},
			{Name: "SnatConnectionCount", Units: "connections", Aggregations: []string{"average"}, TimeGrain: "PT1M"},
		},
	},
}

func defaultBuiltInProfileNames() []string {
	return []string{
		"sql_managed_instance",
		"sql_database",
		"postgres_flexible",
		"cosmos_db",
		"logic_apps",
		"virtual_machines",
		"aks",
		"storage_accounts",
		"load_balancers",
	}
}

func hasBuiltInProfile(name string) bool {
	_, ok := builtInProfiles[strings.ToLower(strings.TrimSpace(name))]
	return ok
}
