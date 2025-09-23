package db2

import (
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/confopt"
	"github.com/netdata/netdata/go/plugins/plugin/ibm.d/framework"
)

// Config defines the user-facing configuration for the DB2 collector.
type Config struct {
	framework.Config `yaml:",inline" json:",inline"`

	Vnode         string           `yaml:"vnode,omitempty" json:"vnode"`
	UpdateEvery   int              `yaml:"update_every,omitempty" json:"update_every"`
	DSN           string           `yaml:"dsn" json:"dsn"`
	Timeout       confopt.Duration `yaml:"timeout,omitempty" json:"timeout"`
	MaxDbConns    int              `yaml:"max_db_conns,omitempty" json:"max_db_conns"`
	MaxDbLifeTime confopt.Duration `yaml:"max_db_life_time,omitempty" json:"max_db_life_time"`

	CollectDatabaseMetrics   *bool `yaml:"collect_database_metrics,omitempty" json:"collect_database_metrics"`
	CollectBufferpoolMetrics *bool `yaml:"collect_bufferpool_metrics,omitempty" json:"collect_bufferpool_metrics"`
	CollectTablespaceMetrics *bool `yaml:"collect_tablespace_metrics,omitempty" json:"collect_tablespace_metrics"`
	CollectConnectionMetrics *bool `yaml:"collect_connection_metrics,omitempty" json:"collect_connection_metrics"`
	CollectLockMetrics       *bool `yaml:"collect_lock_metrics,omitempty" json:"collect_lock_metrics"`
	CollectTableMetrics      *bool `yaml:"collect_table_metrics,omitempty" json:"collect_table_metrics"`
	CollectIndexMetrics      *bool `yaml:"collect_index_metrics,omitempty" json:"collect_index_metrics"`

	MaxDatabases   int `yaml:"max_databases,omitempty" json:"max_databases"`
	MaxBufferpools int `yaml:"max_bufferpools,omitempty" json:"max_bufferpools"`
	MaxTablespaces int `yaml:"max_tablespaces,omitempty" json:"max_tablespaces"`
	MaxConnections int `yaml:"max_connections,omitempty" json:"max_connections"`
	MaxTables      int `yaml:"max_tables,omitempty" json:"max_tables"`
	MaxIndexes     int `yaml:"max_indexes,omitempty" json:"max_indexes"`

	BackupHistoryDays int `yaml:"backup_history_days,omitempty" json:"backup_history_days"`

	CollectMemoryMetrics  bool `yaml:"collect_memory_metrics,omitempty" json:"collect_memory_metrics"`
	CollectWaitMetrics    bool `yaml:"collect_wait_metrics,omitempty" json:"collect_wait_metrics"`
	CollectTableIOMetrics bool `yaml:"collect_table_io_metrics,omitempty" json:"collect_table_io_metrics"`

	CollectDatabasesMatching   string `yaml:"collect_databases_matching,omitempty" json:"collect_databases_matching"`
	CollectBufferpoolsMatching string `yaml:"collect_bufferpools_matching,omitempty" json:"collect_bufferpools_matching"`
	CollectTablespacesMatching string `yaml:"collect_tablespaces_matching,omitempty" json:"collect_tablespaces_matching"`
	CollectConnectionsMatching string `yaml:"collect_connections_matching,omitempty" json:"collect_connections_matching"`
	CollectTablesMatching      string `yaml:"collect_tables_matching,omitempty" json:"collect_tables_matching"`
	CollectIndexesMatching     string `yaml:"collect_indexes_matching,omitempty" json:"collect_indexes_matching"`
}
