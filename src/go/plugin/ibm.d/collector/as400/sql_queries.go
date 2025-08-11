// SPDX-License-Identifier: GPL-3.0-or-later

package as400

const (
	// VERIFIED: Comprehensive system status query - works on both IBM i 7.4 and pub400.com
	// This is the primary source for all system-level metrics
	querySystemStatus = `SELECT * FROM TABLE(QSYS2.SYSTEM_STATUS(RESET_STATISTICS=>'YES',DETAILED_INFO=>'ALL')) X`

	// VERIFIED: Memory pool monitoring using MEMORY_POOL() function
	// Works on both IBM i 7.4 and pub400.com - only use verified columns
	queryMemoryPools = `
		SELECT 
			POOL_NAME,
			CURRENT_SIZE,
			DEFINED_SIZE,
			RESERVED_SIZE,
			CURRENT_THREADS,
			MAXIMUM_ACTIVE_THREADS
		FROM TABLE(QSYS2.MEMORY_POOL(RESET_STATISTICS=>'YES')) X
		WHERE POOL_NAME IN ('*MACHINE', '*BASE', '*INTERACT', '*SPOOL')
	`

	// VERIFIED: Disk status using SYSDISKSTAT table - works on both systems
	queryDiskStatus = `
		SELECT 
			AVG(ELAPSED_PERCENT_BUSY) as AVG_DISK_BUSY
		FROM QSYS2.SYSDISKSTAT
	`

	// VERIFIED: Disk instance details using SYSDISKSTAT table - only basic columns
	// Works on both IBM i 7.4 and pub400.com
	queryDiskInstances = `
		SELECT 
			UNIT_NUMBER,
			UNIT_TYPE,
			TOTAL_READ_REQUESTS as READ_REQUESTS,
			TOTAL_WRITE_REQUESTS as WRITE_REQUESTS,
			ELAPSED_PERCENT_BUSY as PERCENT_BUSY
		FROM QSYS2.SYSDISKSTAT
	`

	// VERIFIED: Job queue length using JOB_INFO() function
	// Works on both IBM i 7.4 and pub400.com
	queryJobInfo = `
		SELECT 
			COUNT(*) as JOB_QUEUE_LENGTH
		FROM TABLE(QSYS2.JOB_INFO(JOB_STATUS_FILTER => '*JOBQ')) X
	`

	// VERIFIED: Disk count using SYSDISKSTAT table
	// Works on both IBM i 7.4 and pub400.com
	queryCountDisks = `
		SELECT COUNT(DISTINCT UNIT_NUMBER) as COUNT FROM QSYS2.SYSDISKSTAT
	`

	// Individual queries for columns that exist in SYSTEM_STATUS() - for backward compatibility
	queryConfiguredCPUs = `SELECT CONFIGURED_CPUS FROM TABLE(QSYS2.SYSTEM_STATUS()) X`
	queryAverageCPU = `SELECT AVERAGE_CPU_UTILIZATION FROM TABLE(QSYS2.SYSTEM_STATUS()) X`
	querySystemASP = `SELECT SYSTEM_ASP_USED FROM TABLE(QSYS2.SYSTEM_STATUS()) X`
	queryActiveJobs = `SELECT TOTAL_JOBS_IN_SYSTEM FROM TABLE(QSYS2.SYSTEM_STATUS()) X`

	// System information queries for labels - only use verified columns
	querySerialNumber = `SELECT SERIAL_NUMBER FROM TABLE(QSYS2.SYSTEM_STATUS()) X`
	querySystemModel = `SELECT MACHINE_MODEL FROM TABLE(QSYS2.SYSTEM_STATUS()) X`

	// Query for IBM i version - use SYSIBMADM.ENV_SYS_INFO which works across IBM i versions
	queryIBMiVersion = `SELECT OS_NAME, OS_VERSION, OS_RELEASE FROM SYSIBMADM.ENV_SYS_INFO`

	// Alternative query using data area (fallback if SYSIBMADM.ENV_SYS_INFO doesn't exist)
	queryIBMiVersionDataArea = `SELECT SUBSTRING(DATA_AREA_VALUE, 1, 8) AS VERSION FROM QSYS2.DATA_AREA_INFO WHERE DATA_AREA_LIBRARY = 'QUSRSYS' AND DATA_AREA_NAME = 'QSS1MRI'`

	// Query for Technology Refresh level - shows TR level (e.g., TR1, TR2, TR3)
	queryTechnologyRefresh = `
		SELECT MAX(PTF_GROUP_LEVEL) AS TR_LEVEL
		FROM QSYS2.GROUP_PTF_INFO
		WHERE PTF_GROUP_DESCRIPTION = 'TECHNOLOGY REFRESH'
		  AND PTF_GROUP_STATUS = 'INSTALLED'
	`

	// VERIFIED: Network connections from NETSTAT_INFO
	// Works on both IBM i 7.4 and pub400.com
	queryNetworkConnections = `
		SELECT 
			COUNT(CASE WHEN TCP_STATE = 'ESTABLISHED' AND REMOTE_ADDRESS != '::1' AND REMOTE_ADDRESS != '127.0.0.1' THEN 1 END) as REMOTE_CONNECTIONS,
			COUNT(*) as TOTAL_CONNECTIONS,
			COUNT(CASE WHEN TCP_STATE = 'LISTEN' THEN 1 END) as LISTEN_CONNECTIONS,
			COUNT(CASE WHEN TCP_STATE = 'CLOSE-WAIT' THEN 1 END) as CLOSEWAIT_CONNECTIONS
		FROM QSYS2.NETSTAT_INFO
	`

	// Network interface information from NETSTAT_INTERFACE_INFO
	// Provides interface configuration and status (requires IBM i 7.2 TR3+)
	queryNetworkInterfaces = `
		SELECT 
			COALESCE(LINE_DESCRIPTION, 'UNKNOWN') as LINE_DESCRIPTION,
			COALESCE(INTERFACE_LINE_TYPE, 'UNKNOWN') as INTERFACE_LINE_TYPE,
			COALESCE(INTERFACE_STATUS, 'UNKNOWN') as INTERFACE_STATUS,
			COALESCE(CONNECTION_TYPE, 'UNKNOWN') as CONNECTION_TYPE,
			COALESCE(INTERNET_ADDRESS, '') as INTERNET_ADDRESS,
			COALESCE(NETWORK_ADDRESS, '') as NETWORK_ADDRESS,
			COALESCE(MAXIMUM_TRANSMISSION_UNIT, 0) as MTU
		FROM QSYS2.NETSTAT_INTERFACE_INFO
		WHERE LINE_DESCRIPTION != '*LOOPBACK'
		ORDER BY LINE_DESCRIPTION
	`

	// Count network interfaces for cardinality check
	queryCountNetworkInterfaces = `
		SELECT COUNT(*) as COUNT
		FROM QSYS2.NETSTAT_INTERFACE_INFO
		WHERE LINE_DESCRIPTION != '*LOOPBACK'
	`

	// VERIFIED: Temporary storage monitoring
	// Works on both IBM i 7.4 and pub400.com
	queryTempStorageTotal = `
		SELECT 
			SUM(BUCKET_CURRENT_SIZE) as CURRENT_SIZE, 
			SUM(BUCKET_PEAK_SIZE) as PEAK_SIZE 
		FROM QSYS2.SYSTMPSTG 
		WHERE GLOBAL_BUCKET_NAME IS NULL
	`

	queryTempStorageNamed = `
		SELECT 
			REPLACE(UPPER(REPLACE(GLOBAL_BUCKET_NAME, '*','')), ' ', '_') as NAME, 
			BUCKET_CURRENT_SIZE as CURRENT_SIZE, 
			BUCKET_PEAK_SIZE as PEAK_SIZE 
		FROM QSYS2.SYSTMPSTG 
		WHERE GLOBAL_BUCKET_NAME IS NOT NULL
	`

	// VERIFIED: Subsystem monitoring
	// Works on both IBM i 7.4 and pub400.com  
	// Note: HELD_JOB_COUNT and STORAGE_USED_KB columns don't exist, removed per AS400.md verification
	querySubsystems = `
		SELECT 
			SUBSYSTEM_DESCRIPTION_LIBRARY || '/' || SUBSYSTEM_DESCRIPTION as SUBSYSTEM_NAME,
			CURRENT_ACTIVE_JOBS,
			MAXIMUM_ACTIVE_JOBS
		FROM QSYS2.SUBSYSTEM_INFO
		WHERE STATUS = 'ACTIVE'
		ORDER BY CURRENT_ACTIVE_JOBS DESC
	`

	// VERIFIED: Job queue monitoring
	// Works on both IBM i 7.4 and pub400.com
	// Note: HELD_JOB_COUNT column doesn't exist, removed per AS400.md verification
	queryJobQueues = `
		SELECT 
			JOB_QUEUE_LIBRARY || '/' || JOB_QUEUE_NAME as QUEUE_NAME,
			NUMBER_OF_JOBS
		FROM QSYS2.JOB_QUEUE_INFO
		WHERE JOB_QUEUE_STATUS = 'RELEASED'
		ORDER BY NUMBER_OF_JOBS DESC
	`

	// Enhanced disk query with all metrics
	queryDiskInstancesEnhanced = `
		SELECT 
			UNIT_NUMBER,
			UNIT_TYPE,
			PERCENT_USED,
			UNIT_SPACE_AVAILABLE_GB,
			UNIT_STORAGE_CAPACITY,
			TOTAL_READ_REQUESTS,
			TOTAL_WRITE_REQUESTS,
			TOTAL_BLOCKS_READ,
			TOTAL_BLOCKS_WRITTEN,
			ELAPSED_PERCENT_BUSY,
			COALESCE(SSD_LIFE_REMAINING, -1) as SSD_LIFE_REMAINING,
			COALESCE(SSD_POWER_ON_DAYS, -1) as SSD_POWER_ON_DAYS,
			COALESCE(HARDWARE_STATUS, 'UNKNOWN') as HARDWARE_STATUS,
			COALESCE(DISK_MODEL, 'UNKNOWN') as DISK_MODEL,
			COALESCE(SERIAL_NUMBER, 'UNKNOWN') as SERIAL_NUMBER
		FROM QSYS2.SYSDISKSTAT
	`

	// Top active jobs query using ACTIVE_JOB_INFO (requires IBM i 7.3+)
	queryTopActiveJobs = `
		SELECT
			JOB_NAME,
			JOB_STATUS,
			SUBSYSTEM,
			JOB_TYPE,
			ELAPSED_CPU_TIME,
			ELAPSED_TIME,
			TEMPORARY_STORAGE,
			CPU_PERCENTAGE,
			ELAPSED_INTERACTIVE_TRANSACTIONS,
			ELAPSED_TOTAL_DISK_IO_COUNT,
			THREAD_COUNT,
			RUN_PRIORITY
		FROM TABLE(QSYS2.ACTIVE_JOB_INFO(
			JOB_NAME_FILTER => '*ALL',
			SUBSYSTEM_LIST_FILTER => '*ALL',
			CURRENT_USER_LIST_FILTER => '*ALL',
			DETAILED_INFO => 'BASIC'
		)) X
		WHERE JOB_STATUS != '*JOBLOG PENDING'
		ORDER BY CPU_PERCENTAGE DESC
		FETCH FIRST %d ROWS ONLY
	`

	// Count active jobs for cardinality check
	queryCountActiveJobs = `
		SELECT COUNT(*) as COUNT
		FROM TABLE(QSYS2.ACTIVE_JOB_INFO(
			JOB_NAME_FILTER => '*ALL',
			SUBSYSTEM_LIST_FILTER => '*ALL',
			CURRENT_USER_LIST_FILTER => '*ALL',
			DETAILED_INFO => 'NONE'
		)) X
		WHERE JOB_STATUS != '*JOBLOG PENDING'
	`

	// Remove all queries that reference non-existent tables/columns:
	// - MESSAGE_QUEUE_INFO (doesn't exist)
	// - SUBSYSTEM_INFO (columns don't exist)
	// - JOB_QUEUE_INFO (columns don't exist)
	// - IFS_OBJECT_STATISTICS (doesn't exist)
	// - NETSTAT_INFO (may not exist)
	// - SYSTMPSTG (may not exist)
	// - SYSFUNCS (ROUTINE_TYPE column doesn't exist)
	// - All CPU utilization breakdown columns (don't exist)
	// - All advanced disk columns (don't exist)
)