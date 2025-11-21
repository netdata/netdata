package db2

import (
	"github.com/netdata/netdata/go/plugins/pkg/confopt"
	"github.com/netdata/netdata/go/plugins/plugin/ibm.d/framework"
)

// Config defines the user-facing configuration for the DB2 collector.
type Config struct {
	framework.Config `yaml:",inline" json:",inline"`

	// Vnode allows binding the collector to a virtual node.
	Vnode string `yaml:"vnode,omitempty" json:"vnode" ui:"group:Connection"`

	// DSN provides a full DB2 connection string when manual control is required.
	DSN string `yaml:"dsn" json:"dsn" ui:"group:Connection"`

	// Timeout controls how long DB2 RPCs may run before cancellation.
	Timeout confopt.Duration `yaml:"timeout,omitempty" json:"timeout" ui:"group:Connection"`

	// MaxDbConns limits the connection pool size.
	MaxDbConns int `yaml:"max_db_conns,omitempty" json:"max_db_conns" ui:"group:Advanced"`

	// MaxDbLifeTime forces pooled connections to be recycled after the specified duration.
	MaxDbLifeTime confopt.Duration `yaml:"max_db_life_time,omitempty" json:"max_db_life_time" ui:"group:Advanced"`

	// CollectDatabaseMetrics toggles high-level database status metrics.
	CollectDatabaseMetrics confopt.AutoBool `yaml:"collect_database_metrics,omitempty" json:"collect_database_metrics" ui:"group:Databases"`

	// CollectBufferpoolMetrics toggles buffer pool efficiency metrics.
	CollectBufferpoolMetrics confopt.AutoBool `yaml:"collect_bufferpool_metrics,omitempty" json:"collect_bufferpool_metrics" ui:"group:Buffer Pools"`

	// CollectTablespaceMetrics toggles tablespace capacity metrics.
	CollectTablespaceMetrics confopt.AutoBool `yaml:"collect_tablespace_metrics,omitempty" json:"collect_tablespace_metrics" ui:"group:Tablespaces"`

	// CollectConnectionMetrics toggles per-connection activity metrics.
	CollectConnectionMetrics confopt.AutoBool `yaml:"collect_connection_metrics,omitempty" json:"collect_connection_metrics" ui:"group:Connections Monitoring"`

	// CollectLockMetrics toggles lock contention metrics.
	CollectLockMetrics confopt.AutoBool `yaml:"collect_lock_metrics,omitempty" json:"collect_lock_metrics" ui:"group:Other Metrics"`

	// CollectTableMetrics toggles table-level size and row metrics.
	CollectTableMetrics confopt.AutoBool `yaml:"collect_table_metrics,omitempty" json:"collect_table_metrics" ui:"group:Tables"`

	// CollectIndexMetrics toggles index usage metrics.
	CollectIndexMetrics confopt.AutoBool `yaml:"collect_index_metrics,omitempty" json:"collect_index_metrics" ui:"group:Indexes"`

	// MaxDatabases caps the number of databases charted.
	MaxDatabases int `yaml:"max_databases,omitempty" json:"max_databases" ui:"group:Databases"`

	// MaxBufferpools caps the number of buffer pools charted.
	MaxBufferpools int `yaml:"max_bufferpools,omitempty" json:"max_bufferpools" ui:"group:Buffer Pools"`

	// MaxTablespaces caps the number of tablespaces charted.
	MaxTablespaces int `yaml:"max_tablespaces,omitempty" json:"max_tablespaces" ui:"group:Tablespaces"`

	// MaxConnections caps the number of connection instances charted.
	MaxConnections int `yaml:"max_connections,omitempty" json:"max_connections" ui:"group:Connections Monitoring"`

	// MaxTables caps the number of tables charted.
	MaxTables int `yaml:"max_tables,omitempty" json:"max_tables" ui:"group:Tables"`

	// MaxIndexes caps the number of indexes charted.
	MaxIndexes int `yaml:"max_indexes,omitempty" json:"max_indexes" ui:"group:Indexes"`

	// BackupHistoryDays controls how many days of backup history are retrieved.
	BackupHistoryDays int `yaml:"backup_history_days,omitempty" json:"backup_history_days" ui:"group:Advanced"`

	// CollectMemoryMetrics enables memory pool statistics.
	CollectMemoryMetrics bool `yaml:"collect_memory_metrics,omitempty" json:"collect_memory_metrics" ui:"group:Other Metrics"`

	// CollectWaitMetrics enables wait time statistics (locks, logs, I/O).
	CollectWaitMetrics bool `yaml:"collect_wait_metrics,omitempty" json:"collect_wait_metrics" ui:"group:Other Metrics"`

	// CollectTableIOMetrics enables table I/O statistics when available.
	CollectTableIOMetrics bool `yaml:"collect_table_io_metrics,omitempty" json:"collect_table_io_metrics" ui:"group:Other Metrics"`

	// CollectDatabasesMatching filters databases by name using glob patterns.
	CollectDatabasesMatching string `yaml:"collect_databases_matching,omitempty" json:"collect_databases_matching" ui:"group:Databases"`

	// IncludeConnections filters monitored connections by application ID or application name (wildcards supported).
	IncludeConnections []string `yaml:"include_connections,omitempty" json:"include_connections" ui:"group:Connections Monitoring"`
	// ExcludeConnections excludes connections after inclusion matching.
	ExcludeConnections []string `yaml:"exclude_connections,omitempty" json:"exclude_connections" ui:"group:Connections Monitoring"`

	// IncludeBufferpools filters buffer pools by name.
	IncludeBufferpools []string `yaml:"include_bufferpools,omitempty" json:"include_bufferpools" ui:"group:Buffer Pools"`
	// ExcludeBufferpools excludes buffer pools after inclusion.
	ExcludeBufferpools []string `yaml:"exclude_bufferpools,omitempty" json:"exclude_bufferpools" ui:"group:Buffer Pools"`

	// IncludeTablespaces filters tablespaces by name.
	IncludeTablespaces []string `yaml:"include_tablespaces,omitempty" json:"include_tablespaces" ui:"group:Tablespaces"`
	// ExcludeTablespaces excludes tablespaces after inclusion.
	ExcludeTablespaces []string `yaml:"exclude_tablespaces,omitempty" json:"exclude_tablespaces" ui:"group:Tablespaces"`

	// IncludeTables filters tables by schema/name.
	IncludeTables []string `yaml:"include_tables,omitempty" json:"include_tables" ui:"group:Tables"`
	// ExcludeTables excludes tables after inclusion.
	ExcludeTables []string `yaml:"exclude_tables,omitempty" json:"exclude_tables" ui:"group:Tables"`

	// IncludeIndexes filters indexes by schema/name.
	IncludeIndexes []string `yaml:"include_indexes,omitempty" json:"include_indexes" ui:"group:Indexes"`
	// ExcludeIndexes excludes indexes after inclusion.
	ExcludeIndexes []string `yaml:"exclude_indexes,omitempty" json:"exclude_indexes" ui:"group:Indexes"`
}
