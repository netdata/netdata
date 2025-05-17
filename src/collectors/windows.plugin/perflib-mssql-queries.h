// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_PERFLIB_MSSQL_QUERIES_H
#define NETDATA_PERFLIB_MSSQL_QUERIES_H

// https://learn.microsoft.com/en-us/sql/relational-databases/system-dynamic-management-views/sys-dm-tran-locks-transact-sql?view=sql-server-ver16
#define NETDATA_QUERY_LOCKS_MASK                                                                                       \
"SELECT resource_type, count(*) FROM %s.sys.dm_tran_locks WHERE DB_NAME(resource_database_id) = '%s' group by resource_type;"

#define NETDATA_QUERY_LIST_DB "SELECT name FROM sys.databases;"

// https://learn.microsoft.com/en-us/sql/relational-databases/system-catalog-views/sys-database-files-transact-sql?view=sql-server-ver16
#define NETDATA_QUERY_DATA_FILE_SIZE_MASK "SELECT size * 8/1024 FROM %s.sys.database_files WHERE type = 0;"

// https://learn.microsoft.com/en-us/sql/relational-databases/system-compatibility-views/sys-sysprocesses-transact-sql?view=sql-server-ver16
// SQL SERVER BEFORE 2008 DOES NOT HAVE DATA IN THIS TABLE
// https://github.com/influxdata/telegraf/blob/081dfa26e80d8764fb7f9aac5230e81584b62b56/plugins/inputs/sqlserver/sqlqueriesV2.go#L1259
#define NETDATA_QUERY_TRANSACTIONS_MASK                                                                                \
"SELECT counter_name, cntr_value FROM %s.sys.dm_os_performance_counters WHERE instance_name = '%s' AND counter_name IN ('Active Transactions', 'Transactions/sec', 'Write Transactions/sec', 'Backup/Restore Throughput/sec', 'Log Bytes Flushed/sec', 'Log Flushes/sec', 'Number of Deadlocks/sec', 'Lock Waits/sec', 'Lock Timeouts/sec', 'Lock Requests/sec');"

#define NETDATA_QUERY_CHECK_PERM                                                                                       \
"SELECT CASE WHEN IS_SRVROLEMEMBER('sysadmin') = 1 OR HAS_PERMS_BY_NAME(null, null, 'VIEW SERVER STATE') = 1 THEN 1 ELSE 0 END AS has_permission;"

#endif

