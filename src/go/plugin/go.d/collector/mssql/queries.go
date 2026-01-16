// SPDX-License-Identifier: GPL-3.0-or-later

package mssql

// queryVersion retrieves SQL Server version info
const queryVersion = `
SELECT SERVERPROPERTY('ProductVersion') AS version;
`

// queryUserConnections counts user vs system connections
const queryUserConnections = `
SELECT
  SUM(CASE WHEN is_user_process = 1 THEN 1 ELSE 0 END) AS user_connections,
  SUM(CASE WHEN is_user_process = 0 THEN 1 ELSE 0 END) AS system_connections
FROM sys.dm_exec_sessions;
`

// queryBlockedProcesses counts blocked sessions
const queryBlockedProcesses = `
SELECT COUNT(DISTINCT session_id) AS blocked_sessions
FROM sys.dm_exec_requests
WHERE blocking_session_id <> 0;
`

// queryBatchRequests gets batch request rate from performance counters
const queryBatchRequests = `
SELECT cntr_value
FROM sys.dm_os_performance_counters
WHERE counter_name = 'Batch Requests/sec'
  AND object_name LIKE '%SQL Statistics%';
`

// queryCompilations gets SQL compilation metrics
const queryCompilations = `
SELECT counter_name, cntr_value
FROM sys.dm_os_performance_counters
WHERE object_name LIKE '%SQL Statistics%'
  AND counter_name IN (
    'SQL Compilations/sec',
    'SQL Re-Compilations/sec',
    'Auto-Param Attempts/sec',
    'Safe Auto-Params/sec',
    'Failed Auto-Params/sec'
  );
`

// queryBufferManager gets buffer manager metrics
const queryBufferManager = `
SELECT counter_name, cntr_value
FROM sys.dm_os_performance_counters
WHERE object_name LIKE '%Buffer Manager%'
  AND counter_name IN (
    'Page reads/sec',
    'Page writes/sec',
    'Buffer cache hit ratio',
    'Buffer cache hit ratio base',
    'Checkpoint pages/sec',
    'Page life expectancy',
    'Lazy writes/sec',
    'Page lookups/sec'
  );
`

// queryMemoryManager gets memory manager metrics
const queryMemoryManager = `
SELECT counter_name, cntr_value
FROM sys.dm_os_performance_counters
WHERE object_name LIKE '%Memory Manager%'
  AND counter_name IN (
    'Total Server Memory (KB)',
    'Connection Memory (KB)',
    'Memory Grants Pending',
    'External benefit of memory'
  );
`

// queryAccessMethods gets access method metrics
const queryAccessMethods = `
SELECT cntr_value
FROM sys.dm_os_performance_counters
WHERE object_name LIKE '%Access Methods%'
  AND counter_name = 'Page Splits/sec';
`

// queryDatabaseCounters gets per-database performance counters
const queryDatabaseCounters = `
SELECT
  RTRIM(instance_name) AS database_name,
  counter_name,
  cntr_value
FROM sys.dm_os_performance_counters
WHERE object_name LIKE '%Databases%'
  AND instance_name NOT IN ('_Total', 'mssqlsystemresource')
  AND counter_name IN (
    'Active Transactions',
    'Transactions/sec',
    'Write Transactions/sec',
    'Backup/Restore Throughput/sec',
    'Log Bytes Flushed/sec',
    'Log Flushes/sec'
  );
`

// queryDatabaseLocks gets per-database lock metrics
const queryDatabaseLocks = `
SELECT
  RTRIM(instance_name) AS database_name,
  counter_name,
  cntr_value
FROM sys.dm_os_performance_counters
WHERE object_name LIKE '%Locks%'
  AND instance_name NOT IN ('_Total')
  AND counter_name IN (
    'Number of Deadlocks/sec',
    'Lock Waits/sec',
    'Lock Timeouts/sec',
    'Lock Requests/sec'
  );
`

// queryDatabaseSize gets the size of data files for each database
const queryDatabaseSize = `
SELECT
  DB_NAME(database_id) AS database_name,
  SUM(size) * 8 * 1024 AS size_bytes
FROM sys.master_files
WHERE type = 0 -- data files only
  AND database_id > 4 -- exclude system databases
GROUP BY database_id;
`

// queryLocksByResource gets lock counts grouped by resource type
const queryLocksByResource = `
SELECT
  resource_type,
  COUNT(*) AS lock_count
FROM sys.dm_tran_locks
WHERE resource_database_id > 4 -- exclude system databases
GROUP BY resource_type;
`

// queryWaitStats gets wait statistics with category mapping
const queryWaitStats = `
SELECT
  ws.[wait_type],
  ws.[wait_time_ms] AS total_wait_ms,
  ws.[wait_time_ms] - ws.[signal_wait_time_ms] AS resource_wait_ms,
  ws.[signal_wait_time_ms] AS signal_wait_ms,
  ws.[max_wait_time_ms] AS max_wait_ms,
  ws.[waiting_tasks_count] AS waiting_tasks
FROM sys.dm_os_wait_stats AS ws WITH(NOLOCK)
WHERE ws.[waiting_tasks_count] > 0
  AND ws.[wait_time_ms] > 100
  AND ws.[wait_type] NOT IN (
    'BROKER_EVENTHANDLER', 'BROKER_RECEIVE_WAITFOR', 'BROKER_TASK_STOP',
    'BROKER_TO_FLUSH', 'BROKER_TRANSMITTER', 'CHECKPOINT_QUEUE',
    'CHKPT', 'CLR_AUTO_EVENT', 'CLR_MANUAL_EVENT', 'CLR_SEMAPHORE',
    'DBMIRROR_DBM_EVENT', 'DBMIRROR_EVENTS_QUEUE', 'DBMIRROR_WORKER_QUEUE',
    'DBMIRRORING_CMD', 'DIRTY_PAGE_POLL', 'DISPATCHER_QUEUE_SEMAPHORE',
    'EXECSYNC', 'FSAGENT', 'FT_IFTS_SCHEDULER_IDLE_WAIT',
    'FT_IFTSHC_MUTEX', 'HADR_CLUSAPI_CALL', 'HADR_FILESTREAM_IOMGR_IOCOMPLETION',
    'HADR_LOGCAPTURE_WAIT', 'HADR_NOTIFICATION_DEQUEUE', 'HADR_TIMER_TASK',
    'HADR_WORK_QUEUE', 'KSOURCE_WAKEUP', 'LAZYWRITER_SLEEP',
    'LOGMGR_QUEUE', 'MEMORY_ALLOCATION_EXT', 'ONDEMAND_TASK_QUEUE',
    'PREEMPTIVE_OS_AUTHENTICATIONOPS', 'PREEMPTIVE_OS_GETPROCADDRESS',
    'PREEMPTIVE_XE_CALLBACKEXECUTE', 'PREEMPTIVE_XE_DISPATCHER',
    'PREEMPTIVE_XE_GETTARGETSTATE', 'PREEMPTIVE_XE_SESSIONCOMMIT',
    'PREEMPTIVE_XE_TARGETFINALIZE', 'PREEMPTIVE_XE_TARGETINIT',
    'PWAIT_ALL_COMPONENTS_INITIALIZED', 'PWAIT_DIRECTLOGCONSUMER_GETNEXT',
    'QDS_ASYNC_QUEUE', 'QDS_CLEANUP_STALE_QUERIES_TASK_MAIN_LOOP_SLEEP',
    'QDS_PERSIST_TASK_MAIN_LOOP_SLEEP', 'QDS_SHUTDOWN_QUEUE',
    'REDO_THREAD_PENDING_WORK', 'REQUEST_FOR_DEADLOCK_SEARCH',
    'RESOURCE_QUEUE', 'SERVER_IDLE_CHECK', 'SLEEP_BPOOL_FLUSH',
    'SLEEP_DBSTARTUP', 'SLEEP_DCOMSTARTUP', 'SLEEP_MASTERDBREADY',
    'SLEEP_MASTERMDREADY', 'SLEEP_MASTERUPGRADED', 'SLEEP_MSDBSTARTUP',
    'SLEEP_SYSTEMTASK', 'SLEEP_TASK', 'SLEEP_TEMPDBSTARTUP',
    'SNI_HTTP_ACCEPT', 'SP_SERVER_DIAGNOSTICS_SLEEP',
    'SQLTRACE_BUFFER_FLUSH', 'SQLTRACE_INCREMENTAL_FLUSH_SLEEP',
    'SQLTRACE_WAIT_ENTRIES', 'UCS_SESSION_REGISTRATION',
    'WAIT_FOR_RESULTS', 'WAIT_XTP_CKPT_CLOSE', 'WAIT_XTP_HOST_WAIT',
    'WAIT_XTP_OFFLINE_CKPT_NEW_LOG', 'WAIT_XTP_RECOVERY',
    'WAITFOR', 'WAITFOR_TASKSHUTDOWN', 'XE_BUFFERMGR_ALLPROCESSED_EVENT',
    'XE_DISPATCHER_JOIN', 'XE_DISPATCHER_WAIT', 'XE_LIVE_TARGET_TVF',
    'XE_TIMER_EVENT'
  )
ORDER BY ws.[wait_time_ms] DESC;
`

// queryJobs gets SQL Agent job status
const queryJobs = `
SELECT name, enabled
FROM msdb.dbo.sysjobs;
`

// querySQLErrors gets SQL error counts from performance counters
const querySQLErrors = `
SELECT cntr_value
FROM sys.dm_os_performance_counters
WHERE object_name LIKE '%SQL Errors%'
  AND instance_name = '_Total'
  AND counter_name = 'Errors/sec';
`

// queryDatabaseStatus gets database state and read-only status
const queryDatabaseStatus = `
SELECT name, state, is_read_only
FROM sys.databases;
`

// queryReplicationStatus gets replication publication status (if configured)
// Groups by publication to aggregate across agent types and excludes 'ALL' placeholder
const queryReplicationStatus = `
SELECT
  publisher_db,
  publication,
  MAX(status) AS status,
  MAX(warning) AS warning,
  MAX(ISNULL(worst_latency, 0)) AS worst_latency,
  MIN(CASE WHEN best_latency IS NULL OR best_latency = 0 THEN 999999 ELSE best_latency END) AS best_latency,
  AVG(ISNULL(avg_latency, 0)) AS avg_latency,
  SUM(CASE WHEN isagentrunningnow = 1 THEN 1 ELSE 0 END) AS running_agents
FROM distribution.dbo.MSreplication_monitordata
WHERE publication != 'ALL'
GROUP BY publisher_db, publication;
`

// querySubscriptionCount gets subscription counts per publication
const querySubscriptionCount = `
SELECT
  p.publisher_db,
  p.publication,
  COUNT(s.subscription_type) AS subscription_count
FROM distribution.dbo.MSpublications p
LEFT JOIN distribution.dbo.MSsubscriptions s
  ON p.publication_id = s.publication_id
GROUP BY p.publisher_db, p.publication;
`

// waitTypeCategories maps wait types to their categories
var waitTypeCategories = map[string]string{
	"ASYNC_IO_COMPLETION":              "Other Disk IO",
	"ASYNC_NETWORK_IO":                 "Network IO",
	"BACKUPIO":                         "Other Disk IO",
	"BACKUPBUFFER":                     "Other Disk IO",
	"BROKER_DISPATCHER":                "Service Broker",
	"BROKER_FORWARDER":                 "Service Broker",
	"BROKER_INIT":                      "Service Broker",
	"BROKER_MASTERSTART":               "Service Broker",
	"BROKER_REGISTERALLENDPOINTS":      "Service Broker",
	"BROKER_SERVICE":                   "Service Broker",
	"BROKER_SHUTDOWN":                  "Service Broker",
	"CXPACKET":                         "Parallelism",
	"CXCONSUMER":                       "Parallelism",
	"DBMIRROR_DBM_MUTEX":               "Mirroring",
	"DBMIRROR_SEND":                    "Mirroring",
	"DTC":                              "Transaction",
	"DTC_ABORT_REQUEST":                "Transaction",
	"DTC_RESOLVE":                      "Transaction",
	"DTC_STATE":                        "Transaction",
	"DTC_TMDOWN_REQUEST":               "Transaction",
	"DTC_WAITFOR_OUTCOME":              "Transaction",
	"FT_COMPROWSET_RWLOCK":             "Full Text Search",
	"FT_IFTS_RWLOCK":                   "Full Text Search",
	"FT_IFTS_SCHEDULER_IDLE_WAIT":      "Full Text Search",
	"FT_IFTSHC_MUTEX":                  "Full Text Search",
	"FT_MASTER_MERGE":                  "Full Text Search",
	"IO_COMPLETION":                    "Other Disk IO",
	"IO_QUEUE_LIMIT":                   "Other Disk IO",
	"IO_RETRY":                         "Other Disk IO",
	"LATCH_DT":                         "Latch",
	"LATCH_EX":                         "Latch",
	"LATCH_KP":                         "Latch",
	"LATCH_NL":                         "Latch",
	"LATCH_SH":                         "Latch",
	"LATCH_UP":                         "Latch",
	"LCK_M_BU":                         "Lock",
	"LCK_M_IS":                         "Lock",
	"LCK_M_IU":                         "Lock",
	"LCK_M_IX":                         "Lock",
	"LCK_M_RIn_NL":                     "Lock",
	"LCK_M_RIn_S":                      "Lock",
	"LCK_M_RIn_U":                      "Lock",
	"LCK_M_RIn_X":                      "Lock",
	"LCK_M_RS_S":                       "Lock",
	"LCK_M_RS_U":                       "Lock",
	"LCK_M_RX_S":                       "Lock",
	"LCK_M_RX_U":                       "Lock",
	"LCK_M_RX_X":                       "Lock",
	"LCK_M_S":                          "Lock",
	"LCK_M_SCH_M":                      "Lock",
	"LCK_M_SCH_S":                      "Lock",
	"LCK_M_SIU":                        "Lock",
	"LCK_M_SIX":                        "Lock",
	"LCK_M_U":                          "Lock",
	"LCK_M_UIX":                        "Lock",
	"LCK_M_X":                          "Lock",
	"LOGBUFFER":                        "Tran Log IO",
	"LOGMGR":                           "Tran Log IO",
	"LOGMGR_FLUSH":                     "Tran Log IO",
	"LOGMGR_PMM_LOG":                   "Tran Log IO",
	"LOGMGR_RESERVE_APPEND":            "Tran Log IO",
	"MSQL_DQ":                          "Network IO",
	"MSQL_XP":                          "Network IO",
	"NET_WAITFOR_PACKET":               "Network IO",
	"OLEDB":                            "Network IO",
	"PAGELATCH_DT":                     "Buffer Latch",
	"PAGELATCH_EX":                     "Buffer Latch",
	"PAGELATCH_KP":                     "Buffer Latch",
	"PAGELATCH_NL":                     "Buffer Latch",
	"PAGELATCH_SH":                     "Buffer Latch",
	"PAGELATCH_UP":                     "Buffer Latch",
	"PAGEIOLATCH_DT":                   "Buffer IO",
	"PAGEIOLATCH_EX":                   "Buffer IO",
	"PAGEIOLATCH_KP":                   "Buffer IO",
	"PAGEIOLATCH_NL":                   "Buffer IO",
	"PAGEIOLATCH_SH":                   "Buffer IO",
	"PAGEIOLATCH_UP":                   "Buffer IO",
	"PARALLEL_BACKUP_QUEUE":            "Backup",
	"PARALLEL_REDO_DRAIN_WORKER":       "Backup",
	"PARALLEL_REDO_LOG_CACHE":          "Backup",
	"PARALLEL_REDO_TRAN_LIST":          "Backup",
	"PARALLEL_REDO_WORKER_SYNC":        "Backup",
	"PARALLEL_REDO_WORKER_WAIT_WORK":   "Backup",
	"PREEMPTIVE_ABR":                   "Preemptive",
	"PREEMPTIVE_AUDIT_ACCESS_EVENTLOG": "Preemptive",
	"PREEMPTIVE_AUDIT_ACCESS_SECLOG":   "Preemptive",
	"PREEMPTIVE_CLOSEBACKUPMEDIA":      "Preemptive",
	"PREEMPTIVE_CLOSEBACKUPTAPE":       "Preemptive",
	"PREEMPTIVE_CLOSEBACKUPVDIDEVICE":  "Preemptive",
	"PREEMPTIVE_CLUSAPI_CLUSTERRESOURCECONTROL":    "Preemptive",
	"PREEMPTIVE_COM_COCREATEINSTANCE":              "Preemptive",
	"PREEMPTIVE_COM_COGETCLASSOBJECT":              "Preemptive",
	"PREEMPTIVE_COM_CREATEACCESSOR":                "Preemptive",
	"PREEMPTIVE_COM_DELETEROWS":                    "Preemptive",
	"PREEMPTIVE_COM_GETCOMMANDTEXT":                "Preemptive",
	"PREEMPTIVE_COM_GETDATA":                       "Preemptive",
	"PREEMPTIVE_COM_GETNEXTROWS":                   "Preemptive",
	"PREEMPTIVE_COM_GETRESULT":                     "Preemptive",
	"PREEMPTIVE_COM_GETROWSBYBOOKMARK":             "Preemptive",
	"PREEMPTIVE_COM_LBFLUSH":                       "Preemptive",
	"PREEMPTIVE_COM_LBLOCKREGION":                  "Preemptive",
	"PREEMPTIVE_COM_LBREADAT":                      "Preemptive",
	"PREEMPTIVE_COM_LBSETSIZE":                     "Preemptive",
	"PREEMPTIVE_COM_LBSTAT":                        "Preemptive",
	"PREEMPTIVE_COM_LBUNLOCKREGION":                "Preemptive",
	"PREEMPTIVE_COM_LBWRITEAT":                     "Preemptive",
	"PREEMPTIVE_COM_QUERYINTERFACE":                "Preemptive",
	"PREEMPTIVE_COM_RELEASE":                       "Preemptive",
	"PREEMPTIVE_COM_RELEASEACCESSOR":               "Preemptive",
	"PREEMPTIVE_COM_RELEASEROWS":                   "Preemptive",
	"PREEMPTIVE_COM_RELEASESESSION":                "Preemptive",
	"PREEMPTIVE_COM_RESTARTPOSITION":               "Preemptive",
	"PREEMPTIVE_COM_SEQITHROW":                     "Preemptive",
	"PREEMPTIVE_COM_SETDATAFAILURE":                "Preemptive",
	"PREEMPTIVE_COM_SETPARAMETERINFO":              "Preemptive",
	"PREEMPTIVE_COM_SETPARAMETERPROPERTIES":        "Preemptive",
	"PREEMPTIVE_CONSOLEWRITE":                      "Preemptive",
	"PREEMPTIVE_CREATEPARAM":                       "Preemptive",
	"PREEMPTIVE_DEBUG":                             "Preemptive",
	"PREEMPTIVE_DFSADDLINK":                        "Preemptive",
	"PREEMPTIVE_DFSLINKEXISTCHECK":                 "Preemptive",
	"PREEMPTIVE_DFSLINKHEALTHCHECK":                "Preemptive",
	"PREEMPTIVE_DFSREMOVELINK":                     "Preemptive",
	"PREEMPTIVE_DFSREMOVEROOT":                     "Preemptive",
	"PREEMPTIVE_DFSROOTFOLDERCHECK":                "Preemptive",
	"PREEMPTIVE_DFSROOTINIT":                       "Preemptive",
	"PREEMPTIVE_DFSROOTSHARECHECK":                 "Preemptive",
	"PREEMPTIVE_DTC_ABORT":                         "Preemptive",
	"PREEMPTIVE_DTC_ABORTREQUESTDONE":              "Preemptive",
	"PREEMPTIVE_DTC_BEGINTRANSACTION":              "Preemptive",
	"PREEMPTIVE_DTC_COMMITREQUESTDONE":             "Preemptive",
	"PREEMPTIVE_DTC_ENLIST":                        "Preemptive",
	"PREEMPTIVE_DTC_PREPAREREQUESTDONE":            "Preemptive",
	"PREEMPTIVE_FILESIZEGET":                       "Preemptive",
	"PREEMPTIVE_FSAREATEFILLBACKUP":                "Preemptive",
	"PREEMPTIVE_FSACREATERESTORE":                  "Preemptive",
	"PREEMPTIVE_FSAFREEHEAP":                       "Preemptive",
	"PREEMPTIVE_FSGETTARGETCOMPLETE":               "Preemptive",
	"PREEMPTIVE_FSGETTARGETPREPARE":                "Preemptive",
	"PREEMPTIVE_FSQUERYALLOCATEDRANGES":            "Preemptive",
	"PREEMPTIVE_GETRMIDENTITY":                     "Preemptive",
	"PREEMPTIVE_LOCKMONITOR":                       "Preemptive",
	"PREEMPTIVE_MSS_RELEASE":                       "Preemptive",
	"PREEMPTIVE_ODBCOPS":                           "Preemptive",
	"PREEMPTIVE_OLE_UNINIT":                        "Preemptive",
	"PREEMPTIVE_OTHER_ABORT":                       "Preemptive",
	"PREEMPTIVE_OTHER_ALERTREGISTER":               "Preemptive",
	"PREEMPTIVE_OTHER_ALERTSIGNAL":                 "Preemptive",
	"PREEMPTIVE_OTHER_ALERTWAIT":                   "Preemptive",
	"PREEMPTIVE_OTHER_ABORTTRANSACTION":            "Preemptive",
	"PREEMPTIVE_OTHER_CREATETHREAD":                "Preemptive",
	"PREEMPTIVE_OTHER_PREPARETOENLIST":             "Preemptive",
	"PREEMPTIVE_OTHER_RECOVER":                     "Preemptive",
	"PREEMPTIVE_OTHER_SCHEMAUNLOCK":                "Preemptive",
	"PREEMPTIVE_OTHER_TRANSIMPORT":                 "Preemptive",
	"PREEMPTIVE_OS_ACCEPTSECURITYCONTEXT":          "Preemptive",
	"PREEMPTIVE_OS_ACQUIRECREDENTIALSHANDLE":       "Preemptive",
	"PREEMPTIVE_OS_AUTHZGETINFORMATIONFROMCONTEXT": "Preemptive",
	"PREEMPTIVE_OS_AUTHZINITIALIZERESOURCEMANAGER": "Preemptive",
	"PREEMPTIVE_OS_BACKUPREAD":                     "Preemptive",
	"PREEMPTIVE_OS_CLOSEHANDLE":                    "Preemptive",
	"PREEMPTIVE_OS_CLUSTEROPS":                     "Preemptive",
	"PREEMPTIVE_OS_COMOPS":                         "Preemptive",
	"PREEMPTIVE_OS_COMPLETEAUTHTOKEN":              "Preemptive",
	"PREEMPTIVE_OS_COPYFILE":                       "Preemptive",
	"PREEMPTIVE_OS_CREATEDIRECTORY":                "Preemptive",
	"PREEMPTIVE_OS_CREATEFILE":                     "Preemptive",
	"PREEMPTIVE_OS_CRYPTOPS":                       "Preemptive",
	"PREEMPTIVE_OS_DECRYPTMESSAGE":                 "Preemptive",
	"PREEMPTIVE_OS_DELETEFILE":                     "Preemptive",
	"PREEMPTIVE_OS_DELETESECURITYCONTEXT":          "Preemptive",
	"PREEMPTIVE_OS_DEVICEIOCONTROL":                "Preemptive",
	"PREEMPTIVE_OS_DEVICEOPS":                      "Preemptive",
	"PREEMPTIVE_OS_DIABORPC":                       "Preemptive",
	"PREEMPTIVE_OS_DOMAINSERVICEOPS":               "Preemptive",
	"PREEMPTIVE_OS_DSGETDCNAME":                    "Preemptive",
	"PREEMPTIVE_OS_DTCOPS":                         "Preemptive",
	"PREEMPTIVE_OS_ENCRYPTMESSAGE":                 "Preemptive",
	"PREEMPTIVE_OS_FILEOPS":                        "Preemptive",
	"PREEMPTIVE_OS_FINDFILE":                       "Preemptive",
	"PREEMPTIVE_OS_FLUSHFILEBUFFERS":               "Preemptive",
	"PREEMPTIVE_OS_FORMATMESSAGE":                  "Preemptive",
	"PREEMPTIVE_OS_FREECREDENTIALSHANDLE":          "Preemptive",
	"PREEMPTIVE_OS_FREELIBRARY":                    "Preemptive",
	"PREEMPTIVE_OS_GENERICOPS":                     "Preemptive",
	"PREEMPTIVE_OS_GETADDRINFO":                    "Preemptive",
	"PREEMPTIVE_OS_GETCOMPRESSEDFILESIZE":          "Preemptive",
	"PREEMPTIVE_OS_GETDISKFREESPACE":               "Preemptive",
	"PREEMPTIVE_OS_GETFILEATTRIBUTES":              "Preemptive",
	"PREEMPTIVE_OS_GETFILESIZE":                    "Preemptive",
	"PREEMPTIVE_OS_GETFINALFILEPATHBYHANDLE":       "Preemptive",
	"PREEMPTIVE_OS_GETLONGPATHNAME":                "Preemptive",
	"PREEMPTIVE_OS_GETTEMPPATHNAME":                "Preemptive",
	"PREEMPTIVE_OS_GETVOLUMEPATHNAMEOFNAME":        "Preemptive",
	"PREEMPTIVE_OS_INITIALIZESECURITYCONTEXT":      "Preemptive",
	"PREEMPTIVE_OS_LIBRARYOPS":                     "Preemptive",
	"PREEMPTIVE_OS_LOADLIBRARY":                    "Preemptive",
	"PREEMPTIVE_OS_LOGONUSER":                      "Preemptive",
	"PREEMPTIVE_OS_LOOKUPACCOUNTSID":               "Preemptive",
	"PREEMPTIVE_OS_MESSAGEQUEUEOPS":                "Preemptive",
	"PREEMPTIVE_OS_MOVEFILE":                       "Preemptive",
	"PREEMPTIVE_OS_NETGROUPGETUSERS":               "Preemptive",
	"PREEMPTIVE_OS_NETLOCALGROUPGETMEMBERS":        "Preemptive",
	"PREEMPTIVE_OS_NETUSERGETGROUPS":               "Preemptive",
	"PREEMPTIVE_OS_NETUSERGETLOCALGROUPS":          "Preemptive",
	"PREEMPTIVE_OS_NETUSERMODALSGET":               "Preemptive",
	"PREEMPTIVE_OS_NETVALIDATEPASSWORDPOLICY":      "Preemptive",
	"PREEMPTIVE_OS_NETVALIDATEPASSWORDPOLICYFREE":  "Preemptive",
	"PREEMPTIVE_OS_OPENDIRECTORY":                  "Preemptive",
	"PREEMPTIVE_OS_PABORPC":                        "Preemptive",
	"PREEMPTIVE_OS_PIPEOPS":                        "Preemptive",
	"PREEMPTIVE_OS_PROCESSOPS":                     "Preemptive",
	"PREEMPTIVE_OS_QUERYREGISTRY":                  "Preemptive",
	"PREEMPTIVE_OS_QUERYSECURITYCONTEXTTOKEN":      "Preemptive",
	"PREEMPTIVE_OS_READFILE":                       "Preemptive",
	"PREEMPTIVE_OS_REMOVEDIRECTORY":                "Preemptive",
	"PREEMPTIVE_OS_REPORTEVENT":                    "Preemptive",
	"PREEMPTIVE_OS_REVERTTOSELF":                   "Preemptive",
	"PREEMPTIVE_OS_RABORPC":                        "Preemptive",
	"PREEMPTIVE_OS_SABORPC":                        "Preemptive",
	"PREEMPTIVE_OS_SECURITYOPS":                    "Preemptive",
	"PREEMPTIVE_OS_SERVICEOPS":                     "Preemptive",
	"PREEMPTIVE_OS_SETENDOFFILE":                   "Preemptive",
	"PREEMPTIVE_OS_SETFILEPOINTER":                 "Preemptive",
	"PREEMPTIVE_OS_SETFILEVALIDDATA":               "Preemptive",
	"PREEMPTIVE_OS_SQLTHREADOPS":                   "Preemptive",
	"PREEMPTIVE_OS_VERIFYSIGNATURE":                "Preemptive",
	"PREEMPTIVE_OS_WAITFORSINGLEOBJECT":            "Preemptive",
	"PREEMPTIVE_OS_WINSOCKOPS":                     "Preemptive",
	"PREEMPTIVE_OS_WRITEFILE":                      "Preemptive",
	"PREEMPTIVE_OS_WRITEFILEGATHER":                "Preemptive",
	"PREEMPTIVE_REENLIST":                          "Preemptive",
	"PREEMPTIVE_RESIZELOG":                         "Preemptive",
	"PREEMPTIVE_ROLLFORWARDREDO":                   "Preemptive",
	"PREEMPTIVE_ROLLFORWARDUNDO":                   "Preemptive",
	"PREEMPTIVE_SB_STOPENDPOINT":                   "Preemptive",
	"PREEMPTIVE_SERVER_STARTUP":                    "Preemptive",
	"PREEMPTIVE_SETRMIDENTITY":                     "Preemptive",
	"PREEMPTIVE_SHAREDMEM_GETDATA":                 "Preemptive",
	"PREEMPTIVE_SNIOPEN":                           "Preemptive",
	"PREEMPTIVE_SOSHOST":                           "Preemptive",
	"PREEMPTIVE_SOSTESTING":                        "Preemptive",
	"PREEMPTIVE_SP_SERVER_DIAGNOSTICS":             "Preemptive",
	"PREEMPTIVE_STARTRM":                           "Preemptive",
	"PREEMPTIVE_STREAMFCB_CHECKPOINT":              "Preemptive",
	"PREEMPTIVE_STREAMFCB_RECOVER":                 "Preemptive",
	"PREEMPTIVE_STRESSDRIVER":                      "Preemptive",
	"PREEMPTIVE_TESTING":                           "Preemptive",
	"PREEMPTIVE_TRANSIMPORT":                       "Preemptive",
	"PREEMPTIVE_UNMARSHALPROPAGATIONTOKEN":         "Preemptive",
	"PREEMPTIVE_VSS_CREATESNAPSHOT":                "Preemptive",
	"PREEMPTIVE_VSS_CREATEVOLUMESNAPSHOT":          "Preemptive",
	"PREEMPTIVE_XE_CALLBACKEXECUTE":                "Preemptive",
	"PREEMPTIVE_XE_CX_FILE_OPEN":                   "Preemptive",
	"PREEMPTIVE_XE_CX_HTTP_CALL":                   "Preemptive",
	"PREEMPTIVE_XE_DISPATCHER":                     "Preemptive",
	"PREEMPTIVE_XE_ENGINEINIT":                     "Preemptive",
	"PREEMPTIVE_XE_GETTARGETSTATE":                 "Preemptive",
	"PREEMPTIVE_XE_SESSIONCOMMIT":                  "Preemptive",
	"PREEMPTIVE_XE_TARGETFINALIZE":                 "Preemptive",
	"PREEMPTIVE_XE_TARGETINIT":                     "Preemptive",
	"PREEMPTIVE_XE_TIMERRUN":                       "Preemptive",
	"PREEMPTIVE_XETESTING":                         "Preemptive",
	"PWAIT_HADR_ACTION_COMPLETED":                  "Replication",
	"PWAIT_HADR_CHANGE_NOTIFIER_TERMINATION_SYNC":  "Replication",
	"PWAIT_HADR_CLUSTER_INTEGRATION":               "Replication",
	"PWAIT_HADR_FAILOVER_COMPLETED":                "Replication",
	"PWAIT_HADR_JOIN":                              "Replication",
	"PWAIT_HADR_OFFLINE_COMPLETED":                 "Replication",
	"PWAIT_HADR_ONLINE_COMPLETED":                  "Replication",
	"PWAIT_HADR_POST_ONLINE_COMPLETED":             "Replication",
	"PWAIT_HADR_SERVER_READY_CONNECTIONS":          "Replication",
	"PWAIT_HADR_WORKITEM_COMPLETED":                "Replication",
	"REPLICA_WRITES":                               "Replication",
	"RESOURCE_SEMAPHORE":                           "Memory",
	"RESOURCE_SEMAPHORE_MUTEX":                     "Memory",
	"RESOURCE_SEMAPHORE_QUERY_COMPILE":             "Compilation",
	"RESOURCE_SEMAPHORE_SMALL_QUERY":               "Memory",
	"SOS_PHYS_PAGE_CACHE":                          "Memory",
	"SOS_RESERVEDMEMBLOCKLIST":                     "Memory",
	"SOS_SCHEDULER_YIELD":                          "CPU",
	"SOS_VIRTUALMEMORY_LOW":                        "Memory",
	"SOS_WORK_DISPATCHER":                          "Worker Thread",
	"SQLCLR_APPDOMAIN":                             "SQL CLR",
	"SQLCLR_ASSEMBLY":                              "SQL CLR",
	"SQLCLR_DEADLOCK_DETECTION":                    "SQL CLR",
	"SQLCLR_QUANTUM_PUNISHMENT":                    "SQL CLR",
	"THREADPOOL":                                   "Worker Thread",
	"TRACEWRITE":                                   "Tracing",
	"TRAN_MARKLATCH_DT":                            "Transaction",
	"TRAN_MARKLATCH_EX":                            "Transaction",
	"TRAN_MARKLATCH_KP":                            "Transaction",
	"TRAN_MARKLATCH_NL":                            "Transaction",
	"TRAN_MARKLATCH_SH":                            "Transaction",
	"TRAN_MARKLATCH_UP":                            "Transaction",
	"TRANSACTION_MUTEX":                            "Transaction",
	"WRITELOG":                                     "Tran Log IO",
	"XACTLOCKINFO":                                 "Transaction",
	"XACT_OWN_TRANSACTION":                         "Transaction",
	"XACT_RECLAIM_SESSION":                         "Transaction",
	"XACT_SNAPSHOT":                                "Transaction",
}

// getWaitCategory returns the category for a wait type, or "OTHER" if unknown
func getWaitCategory(waitType string) string {
	if cat, ok := waitTypeCategories[waitType]; ok {
		return cat
	}
	return "OTHER"
}
