package db2

import (
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/confopt"
	"github.com/netdata/netdata/go/plugins/plugin/ibm.d/framework"
)

// Config defines the user-facing configuration for the DB2 collector.
type Config struct {
	framework.Config `yaml:",inline" json:",inline"`

	// Vnode allows binding the collector to a virtual node.
	Vnode string `yaml:"vnode,omitempty" json:"vnode"`

	// DSN provides a full DB2 connection string when manual control is required.
	DSN string `yaml:"dsn" json:"dsn"`

	// Timeout controls how long DB2 RPCs may run before cancellation.
	Timeout confopt.Duration `yaml:"timeout,omitempty" json:"timeout"`

	// MaxDbConns limits the connection pool size.
	MaxDbConns int `yaml:"max_db_conns,omitempty" json:"max_db_conns"`

	// MaxDbLifeTime forces pooled connections to be recycled after the specified duration.
	MaxDbLifeTime confopt.Duration `yaml:"max_db_life_time,omitempty" json:"max_db_life_time"`

	// CollectDatabaseMetrics toggles high-level database status metrics.
	CollectDatabaseMetrics *bool `yaml:"collect_database_metrics,omitempty" json:"collect_database_metrics"`

	// CollectBufferpoolMetrics toggles buffer pool efficiency metrics.
	CollectBufferpoolMetrics *bool `yaml:"collect_bufferpool_metrics,omitempty" json:"collect_bufferpool_metrics"`

	// CollectTablespaceMetrics toggles tablespace capacity metrics.
	CollectTablespaceMetrics *bool `yaml:"collect_tablespace_metrics,omitempty" json:"collect_tablespace_metrics"`

	// CollectConnectionMetrics toggles per-connection activity metrics.
	CollectConnectionMetrics *bool `yaml:"collect_connection_metrics,omitempty" json:"collect_connection_metrics"`

	// CollectLockMetrics toggles lock contention metrics.
	CollectLockMetrics *bool `yaml:"collect_lock_metrics,omitempty" json:"collect_lock_metrics"`

	// CollectTableMetrics toggles table-level size and row metrics.
	CollectTableMetrics *bool `yaml:"collect_table_metrics,omitempty" json:"collect_table_metrics"`

	// CollectIndexMetrics toggles index usage metrics.
	CollectIndexMetrics *bool `yaml:"collect_index_metrics,omitempty" json:"collect_index_metrics"`

	// MaxDatabases caps the number of databases charted.
	MaxDatabases int `yaml:"max_databases,omitempty" json:"max_databases"`

	// MaxBufferpools caps the number of buffer pools charted.
	MaxBufferpools int `yaml:"max_bufferpools,omitempty" json:"max_bufferpools"`

	// MaxTablespaces caps the number of tablespaces charted.
	MaxTablespaces int `yaml:"max_tablespaces,omitempty" json:"max_tablespaces"`

	// MaxConnections caps the number of connection instances charted.
	MaxConnections int `yaml:"max_connections,omitempty" json:"max_connections"`

	// MaxTables caps the number of tables charted.
	MaxTables int `yaml:"max_tables,omitempty" json:"max_tables"`

	// MaxIndexes caps the number of indexes charted.
	MaxIndexes int `yaml:"max_indexes,omitempty" json:"max_indexes"`

	// BackupHistoryDays controls how many days of backup history are retrieved.
	BackupHistoryDays int `yaml:"backup_history_days,omitempty" json:"backup_history_days"`

	// CollectMemoryMetrics enables memory pool statistics.
	CollectMemoryMetrics bool `yaml:"collect_memory_metrics,omitempty" json:"collect_memory_metrics"`

	// CollectWaitMetrics enables wait time statistics (locks, logs, I/O).
	CollectWaitMetrics bool `yaml:"collect_wait_metrics,omitempty" json:"collect_wait_metrics"`

	// CollectTableIOMetrics enables table I/O statistics when available.
	CollectTableIOMetrics bool `yaml:"collect_table_io_metrics,omitempty" json:"collect_table_io_metrics"`

	// CollectDatabasesMatching filters databases by name using glob patterns.
	CollectDatabasesMatching string `yaml:"collect_databases_matching,omitempty" json:"collect_databases_matching"`

	// CollectBufferpoolsMatching filters buffer pools by name using glob patterns.
	CollectBufferpoolsMatching string `yaml:"collect_bufferpools_matching,omitempty" json:"collect_bufferpools_matching"`

	// CollectTablespacesMatching filters tablespaces by name using glob patterns.
	CollectTablespacesMatching string `yaml:"collect_tablespaces_matching,omitempty" json:"collect_tablespaces_matching"`

	// CollectConnectionsMatching filters monitored connections by application ID.
	CollectConnectionsMatching string `yaml:"collect_connections_matching,omitempty" json:"collect_connections_matching"`

	// CollectTablesMatching filters tables by schema/name.
	CollectTablesMatching string `yaml:"collect_tables_matching,omitempty" json:"collect_tables_matching"`

	// CollectIndexesMatching filters indexes by schema/name.
	CollectIndexesMatching string `yaml:"collect_indexes_matching,omitempty" json:"collect_indexes_matching"`
}
