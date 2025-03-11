// SPDX-License-Identifier: GPL-3.0-or-later

#include "windows_plugin.h"
#include "windows-internals.h"

// https://learn.microsoft.com/en-us/sql/sql-server/install/instance-configuration?view=sql-server-ver16
#define NETDATA_MAX_INSTANCE_NAME 32
#define NETDATA_MAX_INSTANCE_OBJECT 128

BOOL is_sqlexpress = FALSE;

enum netdata_mssql_metrics {
    NETDATA_MSSQL_GENERAL_STATS,
    NETDATA_MSSQL_SQL_ERRORS,
    NETDATA_MSSQL_DATABASE,
    NETDATA_MSSQL_LOCKS,
    NETDATA_MSSQL_MEMORY,
    NETDATA_MSSQL_BUFFER_MANAGEMENT,
    NETDATA_MSSQL_SQL_STATS,
    NETDATA_MSSQL_ACCESS_METHODS,

    NETDATA_MSSQL_METRICS_END
};

struct mssql_instance {
    char *instanceID;

    char *objectName[NETDATA_MSSQL_METRICS_END];

    RRDSET *st_user_connections;
    RRDDIM *rd_user_connections;

    RRDSET *st_process_blocked;
    RRDDIM *rd_process_blocked;

    RRDSET *st_stats_auto_param;
    RRDDIM *rd_stats_auto_param;

    RRDSET *st_stats_batch_request;
    RRDDIM *rd_stats_batch_request;

    RRDSET *st_stats_safe_auto;
    RRDDIM *rd_stats_safe_auto;

    RRDSET *st_stats_compilation;
    RRDDIM *rd_stats_compilation;

    RRDSET *st_stats_recompiles;
    RRDDIM *rd_stats_recompiles;

    RRDSET *st_buff_cache_hits;
    RRDDIM *rd_buff_cache_hits;

    RRDSET *st_buff_cache_page_life_expectancy;
    RRDDIM *rd_buff_cache_page_life_expectancy;

    RRDSET *st_buff_checkpoint_pages;
    RRDDIM *rd_buff_checkpoint_pages;

    RRDSET *st_buff_page_iops;
    RRDDIM *rd_buff_page_reads;
    RRDDIM *rd_buff_page_writes;

    RRDSET *st_access_method_page_splits;
    RRDDIM *rd_access_method_page_splits;

    RRDSET *st_sql_errors;
    RRDDIM *rd_sql_errors;

    RRDSET *st_lockWait;
    RRDSET *st_deadLocks;
    DICTIONARY *locks_instances;

    DICTIONARY *databases;

    RRDSET *st_conn_memory;
    RRDDIM *rd_conn_memory;

    RRDSET *st_ext_benefit_mem;
    RRDDIM *rd_ext_benefit_mem;

    RRDSET *st_pending_mem_grant;
    RRDDIM *rd_pending_mem_grant;

    RRDSET *st_mem_tot_server;
    RRDDIM *rd_mem_tot_server;

    COUNTER_DATA MSSQLAccessMethodPageSplits;
    COUNTER_DATA MSSQLBufferCacheHits;
    COUNTER_DATA MSSQLBufferCheckpointPages;
    COUNTER_DATA MSSQLBufferPageLifeExpectancy;
    COUNTER_DATA MSSQLBufferPageReads;
    COUNTER_DATA MSSQLBufferPageWrites;
    COUNTER_DATA MSSQLBlockedProcesses;
    COUNTER_DATA MSSQLUserConnections;
    COUNTER_DATA MSSQLConnectionMemoryBytes;
    COUNTER_DATA MSSQLExternalBenefitOfMemory;
    COUNTER_DATA MSSQLPendingMemoryGrants;
    COUNTER_DATA MSSQLSQLErrorsTotal;
    COUNTER_DATA MSSQLTotalServerMemory;
    COUNTER_DATA MSSQLStatsAutoParameterization;
    COUNTER_DATA MSSQLStatsBatchRequests;
    COUNTER_DATA MSSQLStatSafeAutoParameterization;
    COUNTER_DATA MSSQLCompilations;
    COUNTER_DATA MSSQLRecompilations;
};

enum lock_instance_idx {
    NETDATA_MSSQL_ENUM_MLI_IDX_WAIT,
    NETDATA_MSSQL_ENUM_MLI_IDX_DEAD_LOCKS,

    NETDATA_MSSQL_ENUM_MLI_IDX_END
};

struct mssql_lock_instance {
    struct mssql_instance *parent;

    COUNTER_DATA lockWait;
    COUNTER_DATA deadLocks;

    RRDDIM *rd_lockWait;
    RRDDIM *rd_deadLocks;

    uint32_t updated;
};

enum db_instance_idx {
    NETDATA_MSSQL_ENUM_MDI_IDX_FILE_SIZE,
    NETDATA_MSSQL_ENUM_MDI_IDX_ACTIVE_TRANSACTIONS,
    NETDATA_MSSQL_ENUM_MDI_IDX_BACKUP_RESTORE_OP,
    NETDATA_MSSQL_ENUM_MDI_IDX_LOG_FLUSHED,
    NETDATA_MSSQL_ENUM_MDI_IDX_LOG_FLUSHES,
    NETDATA_MSSQL_ENUM_MDI_IDX_TRANSACTIONS,
    NETDATA_MSSQL_ENUM_MDI_IDX_WRITE_TRANSACTIONS,

    NETDATA_MSSQL_ENUM_MDI_IDX_END
};

struct mssql_db_instance {
    struct mssql_instance *parent;

    RRDSET *st_db_data_file_size;
    RRDSET *st_db_active_transactions;
    RRDSET *st_db_backup_restore_operations;
    RRDSET *st_db_log_flushed;
    RRDSET *st_db_log_flushes;
    RRDSET *st_db_transactions;
    RRDSET *st_db_write_transactions;

    RRDDIM *rd_db_data_file_size;
    RRDDIM *rd_db_active_transactions;
    RRDDIM *rd_db_backup_restore_operations;
    RRDDIM *rd_db_log_flushed;
    RRDDIM *rd_db_log_flushes;
    RRDDIM *rd_db_transactions;
    RRDDIM *rd_db_write_transactions;

    COUNTER_DATA MSSQLDatabaseActiveTransactions;
    COUNTER_DATA MSSQLDatabaseBackupRestoreOperations;
    COUNTER_DATA MSSQLDatabaseDataFileSize;
    COUNTER_DATA MSSQLDatabaseLogFlushed;
    COUNTER_DATA MSSQLDatabaseLogFlushes;
    COUNTER_DATA MSSQLDatabaseTransactions;
    COUNTER_DATA MSSQLDatabaseWriteTransactions;

    uint32_t updated;
};

static DICTIONARY *mssql_instances = NULL;

static void initialize_mssql_objects(struct mssql_instance *p, const char *instance)
{
    char prefix[NETDATA_MAX_INSTANCE_NAME];
    if (!strcmp(instance, "MSSQLSERVER")) {
        strncpyz(prefix, "SQLServer:", sizeof(prefix) - 1);
    } else if (!strcmp(instance, "SQLEXPRESS")) {
        strncpyz(prefix, "MSSQL$SQLEXPRESS:", sizeof(prefix) - 1);
    } else {
        char *express = (!is_sqlexpress) ? "" : "SQLEXPRESS:";
        snprintfz(prefix, sizeof(prefix) - 1, "MSSQL$%s%s:", express, instance);
    }

    size_t length = strlen(prefix);
    char name[NETDATA_MAX_INSTANCE_OBJECT];
    snprintfz(name, sizeof(name) - 1, "%s%s", prefix, "General Statistics");
    p->objectName[NETDATA_MSSQL_GENERAL_STATS] = strdup(name);

    strncpyz(&name[length], "SQL Errors", sizeof(name) - length);
    p->objectName[NETDATA_MSSQL_SQL_ERRORS] = strdup(name);

    strncpyz(&name[length], "Databases", sizeof(name) - length);
    p->objectName[NETDATA_MSSQL_DATABASE] = strdup(name);

    strncpyz(&name[length], "SQL Statistics", sizeof(name) - length);
    p->objectName[NETDATA_MSSQL_SQL_STATS] = strdup(name);

    strncpyz(&name[length], "Buffer Manager", sizeof(name) - length);
    p->objectName[NETDATA_MSSQL_BUFFER_MANAGEMENT] = strdup(name);

    strncpyz(&name[length], "Memory Manager", sizeof(name) - length);
    p->objectName[NETDATA_MSSQL_MEMORY] = strdup(name);

    strncpyz(&name[length], "Locks", sizeof(name) - length);
    p->objectName[NETDATA_MSSQL_LOCKS] = strdup(name);

    strncpyz(&name[length], "Access Methods", sizeof(name) - length);
    p->objectName[NETDATA_MSSQL_ACCESS_METHODS] = strdup(name);

    p->instanceID = strdup(instance);
}

static inline void initialize_mssql_keys(struct mssql_instance *p)
{
    // General Statistics (https://learn.microsoft.com/en-us/sql/relational-databases/performance-monitor/sql-server-general-statistics-object)
    p->MSSQLUserConnections.key = "User Connections";
    p->MSSQLBlockedProcesses.key = "Processes blocked";

    // SQL Statistics (https://learn.microsoft.com/en-us/sql/relational-databases/performance-monitor/sql-server-sql-statistics-object)
    p->MSSQLStatsAutoParameterization.key = "Auto-Param Attempts/sec";
    p->MSSQLStatsBatchRequests.key = "Batch Requests/sec";
    p->MSSQLStatSafeAutoParameterization.key = "Safe Auto-Params/sec";
    p->MSSQLCompilations.key = "SQL Compilations/sec";
    p->MSSQLRecompilations.key = "SQL Re-Compilations/sec";

    // Buffer Management (https://learn.microsoft.com/en-us/sql/relational-databases/performance-monitor/sql-server-buffer-manager-object)
    p->MSSQLBufferCacheHits.key = "Buffer cache hit ratio";
    p->MSSQLBufferPageLifeExpectancy.key = "Page life expectancy";
    p->MSSQLBufferCheckpointPages.key = "Checkpoint pages/sec";
    p->MSSQLBufferPageReads.key = "Page reads/sec";
    p->MSSQLBufferPageWrites.key = "Page writes/sec";

    // Access Methods (https://learn.microsoft.com/en-us/sql/relational-databases/performance-monitor/sql-server-access-methods-object)
    p->MSSQLAccessMethodPageSplits.key = "Page Splits/sec";

    // Errors (https://learn.microsoft.com/en-us/sql/relational-databases/performance-monitor/sql-server-sql-errors-object)
    p->MSSQLSQLErrorsTotal.key = "Errors/sec";

    // Memory Management (https://learn.microsoft.com/en-us/sql/relational-databases/performance-monitor/sql-server-memory-manager-object)
    p->MSSQLConnectionMemoryBytes.key = "Connection Memory (KB)";
    p->MSSQLExternalBenefitOfMemory.key = "External benefit of memory";
    p->MSSQLPendingMemoryGrants.key = "Memory Grants Pending";
    p->MSSQLTotalServerMemory.key = "Total Server Memory (KB)";
}

void dict_mssql_insert_locks_cb(const DICTIONARY_ITEM *item __maybe_unused, void *value, void *data __maybe_unused)
{
    // https://learn.microsoft.com/en-us/sql/relational-databases/performance-monitor/sql-server-locks-object
    struct mssql_lock_instance *ptr = value;
    ptr->deadLocks.key = "Number of Deadlocks/sec";
    ptr->lockWait.key = "Lock Waits/sec";
}

void dict_mssql_insert_databases_cb(const DICTIONARY_ITEM *item __maybe_unused, void *value, void *data __maybe_unused)
{
    struct mssql_db_instance *ptr = value;

    // https://learn.microsoft.com/en-us/sql/relational-databases/performance-monitor/sql-server-databases-object
    ptr->MSSQLDatabaseDataFileSize.key = "Data File(s) Size (KB)";
    ptr->MSSQLDatabaseActiveTransactions.key = "Active Transactions";
    ptr->MSSQLDatabaseBackupRestoreOperations.key = "Backup/Restore Throughput/sec";
    ptr->MSSQLDatabaseLogFlushed.key = "Log Bytes Flushed/sec";
    ptr->MSSQLDatabaseLogFlushes.key = "Log Flushes/sec";
    ptr->MSSQLDatabaseTransactions.key = "Transactions/sec";
    ptr->MSSQLDatabaseWriteTransactions.key = "Write Transactions/sec";
}

void dict_mssql_insert_cb(const DICTIONARY_ITEM *item __maybe_unused, void *value, void *data __maybe_unused)
{
    struct mssql_instance *p = value;
    const char *instance = dictionary_acquired_item_name((DICTIONARY_ITEM *)item);

    if (!p->locks_instances) {
        p->locks_instances = dictionary_create_advanced(
            DICT_OPTION_DONT_OVERWRITE_VALUE | DICT_OPTION_FIXED_SIZE, NULL, sizeof(struct mssql_lock_instance));
        dictionary_register_insert_callback(p->locks_instances, dict_mssql_insert_locks_cb, NULL);
    }

    if (!p->databases) {
        p->databases = dictionary_create_advanced(
            DICT_OPTION_DONT_OVERWRITE_VALUE | DICT_OPTION_FIXED_SIZE, NULL, sizeof(struct mssql_db_instance));
        dictionary_register_insert_callback(p->databases, dict_mssql_insert_databases_cb, NULL);
    }

    initialize_mssql_objects(p, instance);
    initialize_mssql_keys(p);
}

static int mssql_fill_dictionary()
{
    HKEY hKey;
    LSTATUS ret = RegOpenKeyExA(
        HKEY_LOCAL_MACHINE, "SOFTWARE\\Microsoft\\Microsoft SQL Server\\Instance Names\\SQL", 0, KEY_READ, &hKey);
    if (ret != ERROR_SUCCESS)
        return -1;

    DWORD values = 0;

    ret = RegQueryInfoKey(hKey, NULL, NULL, NULL, NULL, NULL, NULL, &values, NULL, NULL, NULL, NULL);
    if (ret != ERROR_SUCCESS) {
        goto endMSSQLFillDict;
    }

    if (!values) {
        ret = ERROR_PATH_NOT_FOUND;
        goto endMSSQLFillDict;
    }

// https://learn.microsoft.com/en-us/windows/win32/sysinfo/enumerating-registry-subkeys
#define REGISTRY_MAX_VALUE 16383

    DWORD i;
    char avalue[REGISTRY_MAX_VALUE] = {'\0'};
    for (i = 0; i < values; i++) {
        avalue[0] = '\0';
        DWORD length = REGISTRY_MAX_VALUE;

        ret = RegEnumValue(hKey, i, avalue, &length, NULL, NULL, NULL, NULL);
        if (ret != ERROR_SUCCESS)
            continue;

        if (!strcmp(avalue, "SQLEXPRESS")) {
            is_sqlexpress = TRUE;
        }

        struct mssql_instance *p = dictionary_set(mssql_instances, avalue, NULL, sizeof(*p));
    }

endMSSQLFillDict:
    RegCloseKey(hKey);

    return (ret == ERROR_SUCCESS) ? 0 : -1;
}

static int initialize(void)
{
    mssql_instances = dictionary_create_advanced(
        DICT_OPTION_DONT_OVERWRITE_VALUE | DICT_OPTION_FIXED_SIZE, NULL, sizeof(struct mssql_instance));

    dictionary_register_insert_callback(mssql_instances, dict_mssql_insert_cb, NULL);

    if (mssql_fill_dictionary()) {
        return -1;
    }

    return 0;
}

static void do_mssql_general_stats(PERF_DATA_BLOCK *pDataBlock, struct mssql_instance *p, int update_every)
{
    char id[RRD_ID_LENGTH_MAX + 1];

    PERF_OBJECT_TYPE *pObjectType = perflibFindObjectTypeByName(pDataBlock, p->objectName[NETDATA_MSSQL_GENERAL_STATS]);
    if (!pObjectType)
        return;

    if (perflibGetObjectCounter(pDataBlock, pObjectType, &p->MSSQLUserConnections)) {
        if (!p->st_user_connections) {
            snprintfz(id, RRD_ID_LENGTH_MAX, "instance_%s_user_connections", p->instanceID);
            netdata_fix_chart_name(id);
            p->st_user_connections = rrdset_create_localhost(
                "mssql",
                id,
                NULL,
                "connections",
                "mssql.instance_user_connections",
                "User connections",
                "connections",
                PLUGIN_WINDOWS_NAME,
                "PerflibMSSQL",
                PRIO_MSSQL_USER_CONNECTIONS,
                update_every,
                RRDSET_TYPE_LINE);

            p->rd_user_connections = rrddim_add(p->st_user_connections, "user", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);

            rrdlabels_add(p->st_user_connections->rrdlabels, "mssql_instance", p->instanceID, RRDLABEL_SRC_AUTO);
        }

        rrddim_set_by_pointer(
            p->st_user_connections, p->rd_user_connections, (collected_number)p->MSSQLUserConnections.current.Data);
        rrdset_done(p->st_user_connections);
    }

    if (perflibGetObjectCounter(pDataBlock, pObjectType, &p->MSSQLBlockedProcesses)) {
        if (!p->st_process_blocked) {
            snprintfz(id, RRD_ID_LENGTH_MAX, "instance_%s_blocked_process", p->instanceID);
            netdata_fix_chart_name(id);
            p->st_process_blocked = rrdset_create_localhost(
                "mssql",
                id,
                NULL,
                "processes",
                "mssql.instance_blocked_processes",
                "Blocked processes",
                "process",
                PLUGIN_WINDOWS_NAME,
                "PerflibMSSQL",
                PRIO_MSSQL_BLOCKED_PROCESSES,
                update_every,
                RRDSET_TYPE_LINE);

            p->rd_process_blocked = rrddim_add(p->st_process_blocked, "blocked", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);

            rrdlabels_add(p->st_process_blocked->rrdlabels, "mssql_instance", p->instanceID, RRDLABEL_SRC_AUTO);
        }

        rrddim_set_by_pointer(
            p->st_process_blocked, p->rd_process_blocked, (collected_number)p->MSSQLBlockedProcesses.current.Data);
        rrdset_done(p->st_process_blocked);
    }
}

static void do_mssql_sql_statistics(PERF_DATA_BLOCK *pDataBlock, struct mssql_instance *p, int update_every)
{
    char id[RRD_ID_LENGTH_MAX + 1];

    PERF_OBJECT_TYPE *pObjectType = perflibFindObjectTypeByName(pDataBlock, p->objectName[NETDATA_MSSQL_SQL_STATS]);
    if (!pObjectType)
        return;

    if (perflibGetObjectCounter(pDataBlock, pObjectType, &p->MSSQLStatsAutoParameterization)) {
        if (!p->st_stats_auto_param) {
            snprintfz(id, RRD_ID_LENGTH_MAX, "instance_%s_sqlstats_auto_parameterization_attempts", p->instanceID);
            netdata_fix_chart_name(id);
            p->st_stats_auto_param = rrdset_create_localhost(
                "mssql",
                id,
                NULL,
                "sql activity",
                "mssql.instance_sqlstats_auto_parameterization_attempts",
                "Failed auto-parameterization attempts",
                "attempts/s",
                PLUGIN_WINDOWS_NAME,
                "PerflibMSSQL",
                PRIO_MSSQL_STATS_AUTO_PARAMETRIZATION,
                update_every,
                RRDSET_TYPE_LINE);

            p->rd_stats_auto_param =
                rrddim_add(p->st_stats_auto_param, "failed", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);

            rrdlabels_add(p->st_stats_auto_param->rrdlabels, "mssql_instance", p->instanceID, RRDLABEL_SRC_AUTO);
        }

        rrddim_set_by_pointer(
            p->st_stats_auto_param,
            p->rd_stats_auto_param,
            (collected_number)p->MSSQLStatsAutoParameterization.current.Data);
        rrdset_done(p->st_stats_auto_param);
    }

    if (perflibGetObjectCounter(pDataBlock, pObjectType, &p->MSSQLStatsBatchRequests)) {
        if (!p->st_stats_batch_request) {
            snprintfz(id, RRD_ID_LENGTH_MAX, "instance_%s_sqlstats_batch_requests", p->instanceID);
            netdata_fix_chart_name(id);
            p->st_stats_batch_request = rrdset_create_localhost(
                "mssql",
                id,
                NULL,
                "sql activity",
                "mssql.instance_sqlstats_batch_requests",
                "Total of batches requests",
                "requests/s",
                PLUGIN_WINDOWS_NAME,
                "PerflibMSSQL",
                PRIO_MSSQL_STATS_BATCH_REQUEST,
                update_every,
                RRDSET_TYPE_LINE);

            p->rd_stats_batch_request =
                rrddim_add(p->st_stats_batch_request, "batch", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);

            rrdlabels_add(p->st_stats_batch_request->rrdlabels, "mssql_instance", p->instanceID, RRDLABEL_SRC_AUTO);
        }

        rrddim_set_by_pointer(
            p->st_stats_batch_request,
            p->rd_stats_batch_request,
            (collected_number)p->MSSQLStatsBatchRequests.current.Data);
        rrdset_done(p->st_stats_batch_request);
    }

    if (perflibGetObjectCounter(pDataBlock, pObjectType, &p->MSSQLStatSafeAutoParameterization)) {
        if (!p->st_stats_safe_auto) {
            snprintfz(id, RRD_ID_LENGTH_MAX, "instance_%s_sqlstats_safe_auto_parameterization_attempts", p->instanceID);
            netdata_fix_chart_name(id);
            p->st_stats_safe_auto = rrdset_create_localhost(
                "mssql",
                id,
                NULL,
                "sql activity",
                "mssql.instance_sqlstats_safe_auto_parameterization_attempts",
                "Safe auto-parameterization attempts",
                "attempts/s",
                PLUGIN_WINDOWS_NAME,
                "PerflibMSSQL",
                PRIO_MSSQL_STATS_SAFE_AUTO_PARAMETRIZATION,
                update_every,
                RRDSET_TYPE_LINE);

            p->rd_stats_safe_auto = rrddim_add(p->st_stats_safe_auto, "safe", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);

            rrdlabels_add(p->st_stats_safe_auto->rrdlabels, "mssql_instance", p->instanceID, RRDLABEL_SRC_AUTO);
        }

        rrddim_set_by_pointer(
            p->st_stats_safe_auto,
            p->rd_stats_safe_auto,
            (collected_number)p->MSSQLStatSafeAutoParameterization.current.Data);
        rrdset_done(p->st_stats_safe_auto);
    }

    if (perflibGetObjectCounter(pDataBlock, pObjectType, &p->MSSQLCompilations)) {
        if (!p->st_stats_compilation) {
            snprintfz(id, RRD_ID_LENGTH_MAX, "instance_%s_sqlstats_sql_compilations", p->instanceID);
            netdata_fix_chart_name(id);
            p->st_stats_compilation = rrdset_create_localhost(
                "mssql",
                id,
                NULL,
                "sql activity",
                "mssql.instance_sqlstats_sql_compilations",
                "SQL compilations",
                "compilations/s",
                PLUGIN_WINDOWS_NAME,
                "PerflibMSSQL",
                PRIO_MSSQL_STATS_COMPILATIONS,
                update_every,
                RRDSET_TYPE_LINE);

            p->rd_stats_compilation =
                rrddim_add(p->st_stats_compilation, "compilations", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);

            rrdlabels_add(p->st_stats_compilation->rrdlabels, "mssql_instance", p->instanceID, RRDLABEL_SRC_AUTO);
        }

        rrddim_set_by_pointer(
            p->st_stats_compilation, p->rd_stats_compilation, (collected_number)p->MSSQLCompilations.current.Data);
        rrdset_done(p->st_stats_compilation);
    }

    if (perflibGetObjectCounter(pDataBlock, pObjectType, &p->MSSQLRecompilations)) {
        if (!p->st_stats_recompiles) {
            snprintfz(id, RRD_ID_LENGTH_MAX, "instance_%s_sqlstats_sql_recompilations", p->instanceID);
            netdata_fix_chart_name(id);
            p->st_stats_recompiles = rrdset_create_localhost(
                "mssql",
                id,
                NULL,
                "sql activity",
                "mssql.instance_sqlstats_sql_recompilations",
                "SQL re-compilations",
                "recompiles/",
                PLUGIN_WINDOWS_NAME,
                "PerflibMSSQL",
                PRIO_MSSQL_STATS_RECOMPILATIONS,
                update_every,
                RRDSET_TYPE_LINE);

            p->rd_stats_recompiles =
                rrddim_add(p->st_stats_recompiles, "recompiles", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);

            rrdlabels_add(p->st_stats_recompiles->rrdlabels, "mssql_instance", p->instanceID, RRDLABEL_SRC_AUTO);
        }

        rrddim_set_by_pointer(
            p->st_stats_recompiles, p->rd_stats_recompiles, (collected_number)p->MSSQLRecompilations.current.Data);
        rrdset_done(p->st_stats_recompiles);
    }
}

static void do_mssql_buffer_management(PERF_DATA_BLOCK *pDataBlock, struct mssql_instance *p, int update_every)
{
    char id[RRD_ID_LENGTH_MAX + 1];

    PERF_OBJECT_TYPE *pObjectType =
        perflibFindObjectTypeByName(pDataBlock, p->objectName[NETDATA_MSSQL_BUFFER_MANAGEMENT]);
    if (!pObjectType)
        return;

    if (perflibGetObjectCounter(pDataBlock, pObjectType, &p->MSSQLBufferCacheHits)) {
        if (!p->st_buff_cache_hits) {
            snprintfz(id, RRD_ID_LENGTH_MAX, "instance_%s_cache_hit_ratio", p->instanceID);
            netdata_fix_chart_name(id);
            p->st_buff_cache_hits = rrdset_create_localhost(
                "mssql",
                id,
                NULL,
                "buffer cache",
                "mssql.instance_cache_hit_ratio",
                "Buffer Cache hit ratio",
                "percentage",
                PLUGIN_WINDOWS_NAME,
                "PerflibMSSQL",
                PRIO_MSSQL_BUFF_CACHE_HIT_RATIO,
                update_every,
                RRDSET_TYPE_LINE);

            p->rd_buff_cache_hits = rrddim_add(p->st_buff_cache_hits, "hit_ratio", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);

            rrdlabels_add(p->st_buff_cache_hits->rrdlabels, "mssql_instance", p->instanceID, RRDLABEL_SRC_AUTO);
        }

        rrddim_set_by_pointer(
            p->st_buff_cache_hits, p->rd_buff_cache_hits, (collected_number)p->MSSQLBufferCacheHits.current.Data);
        rrdset_done(p->st_buff_cache_hits);
    }

    if (perflibGetObjectCounter(pDataBlock, pObjectType, &p->MSSQLBufferCheckpointPages)) {
        if (!p->st_buff_checkpoint_pages) {
            snprintfz(id, RRD_ID_LENGTH_MAX, "instance_%s_bufman_checkpoint_pages", p->instanceID);
            netdata_fix_chart_name(id);
            p->st_buff_checkpoint_pages = rrdset_create_localhost(
                "mssql",
                id,
                NULL,
                "buffer cache",
                "mssql.instance_bufman_checkpoint_pages",
                "Flushed pages",
                "pages/s",
                PLUGIN_WINDOWS_NAME,
                "PerflibMSSQL",
                PRIO_MSSQL_BUFF_CHECKPOINT_PAGES,
                update_every,
                RRDSET_TYPE_LINE);

            p->rd_buff_checkpoint_pages =
                rrddim_add(p->st_buff_checkpoint_pages, "log", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);

            rrdlabels_add(p->st_buff_checkpoint_pages->rrdlabels, "mssql_instance", p->instanceID, RRDLABEL_SRC_AUTO);
        }

        rrddim_set_by_pointer(
            p->st_buff_checkpoint_pages,
            p->rd_buff_checkpoint_pages,
            (collected_number)p->MSSQLBufferCheckpointPages.current.Data);
        rrdset_done(p->st_buff_checkpoint_pages);
    }

    if (perflibGetObjectCounter(pDataBlock, pObjectType, &p->MSSQLBufferPageLifeExpectancy)) {
        if (!p->st_buff_cache_page_life_expectancy) {
            snprintfz(id, RRD_ID_LENGTH_MAX, "instance_%s_bufman_page_life_expectancy", p->instanceID);
            netdata_fix_chart_name(id);
            p->st_buff_cache_page_life_expectancy = rrdset_create_localhost(
                "mssql",
                id,
                NULL,
                "buffer cache",
                "mssql.instance_bufman_page_life_expectancy",
                "Page life expectancy",
                "seconds",
                PLUGIN_WINDOWS_NAME,
                "PerflibMSSQL",
                PRIO_MSSQL_BUFF_PAGE_LIFE_EXPECTANCY,
                update_every,
                RRDSET_TYPE_LINE);

            p->rd_buff_cache_page_life_expectancy = rrddim_add(
                p->st_buff_cache_page_life_expectancy, "life_expectancy", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);

            rrdlabels_add(
                p->st_buff_cache_page_life_expectancy->rrdlabels, "mssql_instance", p->instanceID, RRDLABEL_SRC_AUTO);
        }

        rrddim_set_by_pointer(
            p->st_buff_cache_page_life_expectancy,
            p->rd_buff_cache_page_life_expectancy,
            (collected_number)p->MSSQLBufferPageLifeExpectancy.current.Data);
        rrdset_done(p->st_buff_cache_page_life_expectancy);
    }

    if (perflibGetObjectCounter(pDataBlock, pObjectType, &p->MSSQLBufferPageReads) &&
        perflibGetObjectCounter(pDataBlock, pObjectType, &p->MSSQLBufferPageWrites)) {
        if (!p->st_buff_page_iops) {
            snprintfz(id, RRD_ID_LENGTH_MAX, "instance_%s_bufman_iops", p->instanceID);
            netdata_fix_chart_name(id);
            p->st_buff_page_iops = rrdset_create_localhost(
                "mssql",
                id,
                NULL,
                "buffer cache",
                "mssql.instance_bufman_iops",
                "Number of pages input and output",
                "pages/s",
                PLUGIN_WINDOWS_NAME,
                "PerflibMSSQL",
                PRIO_MSSQL_BUFF_MAN_IOPS,
                update_every,
                RRDSET_TYPE_LINE);

            p->rd_buff_page_reads = rrddim_add(p->st_buff_page_iops, "read", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            p->rd_buff_page_writes =
                rrddim_add(p->st_buff_page_iops, "written", NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);

            rrdlabels_add(p->st_buff_page_iops->rrdlabels, "mssql_instance", p->instanceID, RRDLABEL_SRC_AUTO);
        }

        rrddim_set_by_pointer(
            p->st_buff_page_iops, p->rd_buff_page_reads, (collected_number)p->MSSQLBufferPageReads.current.Data);
        rrddim_set_by_pointer(
            p->st_buff_page_iops, p->rd_buff_page_writes, (collected_number)p->MSSQLBufferPageWrites.current.Data);

        rrdset_done(p->st_buff_page_iops);
    }
}

static void do_mssql_access_methods(PERF_DATA_BLOCK *pDataBlock, struct mssql_instance *p, int update_every)
{
    char id[RRD_ID_LENGTH_MAX + 1];

    PERF_OBJECT_TYPE *pObjectType =
        perflibFindObjectTypeByName(pDataBlock, p->objectName[NETDATA_MSSQL_ACCESS_METHODS]);
    if (!pObjectType)
        return;

    if (perflibGetObjectCounter(pDataBlock, pObjectType, &p->MSSQLAccessMethodPageSplits)) {
        if (!p->st_access_method_page_splits) {
            snprintfz(id, RRD_ID_LENGTH_MAX, "instance_%s_accessmethods_page_splits", p->instanceID);
            netdata_fix_chart_name(id);
            p->st_access_method_page_splits = rrdset_create_localhost(
                "mssql",
                id,
                NULL,
                "buffer cache",
                "mssql.instance_accessmethods_page_splits",
                "Page splits",
                "splits/s",
                PLUGIN_WINDOWS_NAME,
                "PerflibMSSQL",
                PRIO_MSSQL_BUFF_METHODS_PAGE_SPLIT,
                update_every,
                RRDSET_TYPE_LINE);

            p->rd_access_method_page_splits =
                rrddim_add(p->st_access_method_page_splits, "page", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);

            rrdlabels_add(
                p->st_access_method_page_splits->rrdlabels, "mssql_instance", p->instanceID, RRDLABEL_SRC_AUTO);
        }

        rrddim_set_by_pointer(
            p->st_access_method_page_splits,
            p->rd_access_method_page_splits,
            (collected_number)p->MSSQLAccessMethodPageSplits.current.Data);
        rrdset_done(p->st_access_method_page_splits);
    }
}

static void do_mssql_errors(PERF_DATA_BLOCK *pDataBlock, struct mssql_instance *p, int update_every)
{
    char id[RRD_ID_LENGTH_MAX + 1];

    PERF_OBJECT_TYPE *pObjectType = perflibFindObjectTypeByName(pDataBlock, p->objectName[NETDATA_MSSQL_SQL_ERRORS]);
    if (!pObjectType)
        return;

    if (perflibGetObjectCounter(pDataBlock, pObjectType, &p->MSSQLSQLErrorsTotal)) {
        if (!p->st_sql_errors) {
            snprintfz(id, RRD_ID_LENGTH_MAX, "instance_%s_sql_errors_total", p->instanceID);
            netdata_fix_chart_name(id);
            p->st_sql_errors = rrdset_create_localhost(
                "mssql",
                id,
                NULL,
                "errors",
                "mssql.instance_sql_errors",
                "Errors",
                "errors/s",
                PLUGIN_WINDOWS_NAME,
                "PerflibMSSQL",
                PRIO_MSSQL_SQL_ERRORS,
                update_every,
                RRDSET_TYPE_LINE);

            p->rd_sql_errors = rrddim_add(p->st_sql_errors, "errors", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);

            rrdlabels_add(p->st_sql_errors->rrdlabels, "mssql_instance", p->instanceID, RRDLABEL_SRC_AUTO);
        }

        rrddim_set_by_pointer(
            p->st_sql_errors, p->rd_sql_errors, (collected_number)p->MSSQLAccessMethodPageSplits.current.Data);
        rrdset_done(p->st_sql_errors);
    }
}

int dict_mssql_locks_charts_cb(const DICTIONARY_ITEM *item __maybe_unused, void *value, void *data __maybe_unused)
{
    char id[RRD_ID_LENGTH_MAX + 1];

    struct mssql_lock_instance *mli = value;
    const char *instance = dictionary_acquired_item_name((DICTIONARY_ITEM *)item);

    int *update_every = data;

    if (!mli->parent->st_lockWait) {
        snprintfz(id, RRD_ID_LENGTH_MAX, "instance_%s_locks_lock_wait", mli->parent->instanceID);
        netdata_fix_chart_name(id);
        mli->parent->st_lockWait = rrdset_create_localhost(
            "mssql",
            id,
            NULL,
            "locks",
            "mssql.instance_locks_lock_wait",
            "Lock requests that required the caller to wait.",
            "locks/s",
            PLUGIN_WINDOWS_NAME,
            "PerflibMSSQL",
            PRIO_MSSQL_LOCKS_WAIT,
            *update_every,
            RRDSET_TYPE_LINE);

        rrdlabels_add(
            mli->parent->st_lockWait->rrdlabels, "mssql_instance", mli->parent->instanceID, RRDLABEL_SRC_AUTO);
    }

    if (!mli->rd_lockWait) {
        mli->rd_lockWait = rrddim_add(mli->parent->st_lockWait, instance, NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
    }

    if (mli->updated & (1 << NETDATA_MSSQL_ENUM_MLI_IDX_WAIT)) {
        rrddim_set_by_pointer(
            mli->parent->st_lockWait, mli->rd_lockWait, (collected_number)(mli->lockWait.current.Data));
    }

    if (!mli->parent->st_deadLocks) {
        snprintfz(id, RRD_ID_LENGTH_MAX, "instance_%s_locks_deadlocks", mli->parent->instanceID);
        netdata_fix_chart_name(id);
        mli->parent->st_deadLocks = rrdset_create_localhost(
            "mssql",
            id,
            NULL,
            "locks",
            "mssql.instance_locks_deadlocks",
            "Lock requests that resulted in deadlock.",
            "deadlocks/s",
            PLUGIN_WINDOWS_NAME,
            "PerflibMSSQL",
            PRIO_MSSQL_LOCKS_DEADLOCK,
            *update_every,
            RRDSET_TYPE_LINE);

        rrdlabels_add(
            mli->parent->st_deadLocks->rrdlabels, "mssql_instance", mli->parent->instanceID, RRDLABEL_SRC_AUTO);
    }

    if (!mli->rd_deadLocks) {
        mli->rd_deadLocks = rrddim_add(mli->parent->st_deadLocks, instance, NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
    }

    if (mli->updated & (1 << NETDATA_MSSQL_ENUM_MLI_IDX_DEAD_LOCKS)) {
        rrddim_set_by_pointer(
            mli->parent->st_deadLocks, mli->rd_deadLocks, (collected_number)mli->deadLocks.current.Data);
    }

    return 1;
}

static void do_mssql_locks(PERF_DATA_BLOCK *pDataBlock, struct mssql_instance *p, int update_every)
{
    PERF_OBJECT_TYPE *pObjectType = perflibFindObjectTypeByName(pDataBlock, p->objectName[NETDATA_MSSQL_LOCKS]);
    if (!pObjectType)
        return;

    PERF_INSTANCE_DEFINITION *pi = NULL;
    for (LONG i = 0; i < pObjectType->NumInstances; i++) {
        pi = perflibForEachInstance(pDataBlock, pObjectType, pi);
        if (!pi)
            break;

        if (!getInstanceName(pDataBlock, pObjectType, pi, windows_shared_buffer, sizeof(windows_shared_buffer)))
            strncpyz(windows_shared_buffer, "[unknown]", sizeof(windows_shared_buffer) - 1);

        if (!strcasecmp(windows_shared_buffer, "_Total"))
            continue;

        struct mssql_lock_instance *mli = dictionary_set(p->locks_instances, windows_shared_buffer, NULL, sizeof(*mli));
        if (!mli)
            continue;

        if (!mli->parent) {
            mli->parent = p;
        }

        if (perflibGetObjectCounter(pDataBlock, pObjectType, &mli->lockWait))
            mli->updated |= (1 << NETDATA_MSSQL_ENUM_MLI_IDX_WAIT);

        if (perflibGetObjectCounter(pDataBlock, pObjectType, &mli->deadLocks))
            mli->updated |= (1 << NETDATA_MSSQL_ENUM_MLI_IDX_DEAD_LOCKS);
    }

    dictionary_sorted_walkthrough_read(p->locks_instances, dict_mssql_locks_charts_cb, &update_every);
    rrdset_done(p->st_lockWait);
    rrdset_done(p->st_deadLocks);
}

static void mssql_database_backup_restore_chart(struct mssql_db_instance *mli, const char *db, int update_every)
{
    char id[RRD_ID_LENGTH_MAX + 1];

    if (!mli->st_db_backup_restore_operations) {
        snprintfz(id, RRD_ID_LENGTH_MAX, "db_%s_instance_%s_backup_restore_operations", db, mli->parent->instanceID);
        netdata_fix_chart_name(id);
        mli->st_db_backup_restore_operations = rrdset_create_localhost(
            "mssql",
            id,
            NULL,
            "transactions",
            "mssql.database_backup_restore_operations",
            "Backup IO per database",
            "operations/s",
            PLUGIN_WINDOWS_NAME,
            "PerflibMSSQL",
            PRIO_MSSQL_DATABASE_BACKUP_RESTORE_OPERATIONS,
            update_every,
            RRDSET_TYPE_LINE);

        rrdlabels_add(
            mli->st_db_backup_restore_operations->rrdlabels,
            "mssql_instance",
            mli->parent->instanceID,
            RRDLABEL_SRC_AUTO);
        rrdlabels_add(mli->st_db_backup_restore_operations->rrdlabels, "database", db, RRDLABEL_SRC_AUTO);
    }

    if (!mli->rd_db_backup_restore_operations) {
        mli->rd_db_backup_restore_operations =
            rrddim_add(mli->st_db_backup_restore_operations, "backup", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
    }

    if (mli->updated & (1 << NETDATA_MSSQL_ENUM_MDI_IDX_BACKUP_RESTORE_OP)) {
        rrddim_set_by_pointer(
            mli->st_db_backup_restore_operations,
            mli->rd_db_backup_restore_operations,
            (collected_number)mli->MSSQLDatabaseBackupRestoreOperations.current.Data);
    }

    rrdset_done(mli->st_db_backup_restore_operations);
}

static void mssql_database_log_flushes_chart(struct mssql_db_instance *mli, const char *db, int update_every)
{
    char id[RRD_ID_LENGTH_MAX + 1];

    if (!mli->st_db_log_flushes) {
        snprintfz(id, RRD_ID_LENGTH_MAX, "db_%s_instance_%s_log_flushes", db, mli->parent->instanceID);
        netdata_fix_chart_name(id);
        mli->st_db_log_flushes = rrdset_create_localhost(
            "mssql",
            id,
            NULL,
            "transactions",
            "mssql.database_log_flushes",
            "Log flushes",
            "flushes/s",
            PLUGIN_WINDOWS_NAME,
            "PerflibMSSQL",
            PRIO_MSSQL_DATABASE_LOG_FLUSHES,
            update_every,
            RRDSET_TYPE_LINE);

        rrdlabels_add(mli->st_db_log_flushes->rrdlabels, "mssql_instance", mli->parent->instanceID, RRDLABEL_SRC_AUTO);
        rrdlabels_add(mli->st_db_log_flushes->rrdlabels, "database", db, RRDLABEL_SRC_AUTO);
    }

    if (!mli->rd_db_log_flushes) {
        mli->rd_db_log_flushes = rrddim_add(mli->st_db_log_flushes, "flushes", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
    }

    if (mli->updated & (1 << NETDATA_MSSQL_ENUM_MDI_IDX_LOG_FLUSHES)) {
        rrddim_set_by_pointer(
            mli->st_db_log_flushes,
            mli->rd_db_log_flushes,
            (collected_number)mli->MSSQLDatabaseLogFlushes.current.Data);
    }

    rrdset_done(mli->st_db_log_flushes);
}

static void mssql_database_log_flushed_chart(struct mssql_db_instance *mli, const char *db, int update_every)
{
    char id[RRD_ID_LENGTH_MAX + 1];

    if (!mli->st_db_log_flushed) {
        snprintfz(id, RRD_ID_LENGTH_MAX, "db_%s_instance_%s_log_flushed", db, mli->parent->instanceID);
        netdata_fix_chart_name(id);
        mli->st_db_log_flushed = rrdset_create_localhost(
            "mssql",
            id,
            NULL,
            "transactions",
            "mssql.database_log_flushed",
            "Log flushed",
            "bytes/s",
            PLUGIN_WINDOWS_NAME,
            "PerflibMSSQL",
            PRIO_MSSQL_DATABASE_LOG_FLUSHED,
            update_every,
            RRDSET_TYPE_LINE);

        rrdlabels_add(mli->st_db_log_flushed->rrdlabels, "mssql_instance", mli->parent->instanceID, RRDLABEL_SRC_AUTO);
        rrdlabels_add(mli->st_db_log_flushed->rrdlabels, "database", db, RRDLABEL_SRC_AUTO);
    }

    if (!mli->rd_db_log_flushed) {
        mli->rd_db_log_flushed = rrddim_add(mli->st_db_log_flushed, "flushed", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
    }

    if (mli->updated & (1 << NETDATA_MSSQL_ENUM_MDI_IDX_LOG_FLUSHED)) {
        rrddim_set_by_pointer(
            mli->st_db_log_flushed,
            mli->rd_db_log_flushed,
            (collected_number)mli->MSSQLDatabaseLogFlushed.current.Data);
    }

    rrdset_done(mli->st_db_log_flushed);
}

static void mssql_transactions_chart(struct mssql_db_instance *mli, const char *db, int update_every)
{
    char id[RRD_ID_LENGTH_MAX + 1];

    if (!mli->st_db_transactions) {
        snprintfz(id, RRD_ID_LENGTH_MAX, "db_%s_instance_%s_transactions", db, mli->parent->instanceID);
        netdata_fix_chart_name(id);
        mli->st_db_transactions = rrdset_create_localhost(
            "mssql",
            id,
            NULL,
            "transactions",
            "mssql.database_transactions",
            "Transactions",
            "transactions/s",
            PLUGIN_WINDOWS_NAME,
            "PerflibMSSQL",
            PRIO_MSSQL_DATABASE_TRANSACTIONS,
            update_every,
            RRDSET_TYPE_LINE);

        rrdlabels_add(mli->st_db_transactions->rrdlabels, "mssql_instance", mli->parent->instanceID, RRDLABEL_SRC_AUTO);
        rrdlabels_add(mli->st_db_transactions->rrdlabels, "database", db, RRDLABEL_SRC_AUTO);
    }

    if (!mli->rd_db_transactions) {
        mli->rd_db_transactions =
            rrddim_add(mli->st_db_transactions, "transactions", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
    }

    if (mli->updated & (1 << NETDATA_MSSQL_ENUM_MDI_IDX_TRANSACTIONS)) {
        rrddim_set_by_pointer(
            mli->st_db_transactions,
            mli->rd_db_transactions,
            (collected_number)mli->MSSQLDatabaseTransactions.current.Data);
    }

    rrdset_done(mli->st_db_transactions);
}

static void mssql_write_transactions_chart(struct mssql_db_instance *mli, const char *db, int update_every)
{
    char id[RRD_ID_LENGTH_MAX + 1];

    if (!mli->st_db_write_transactions) {
        snprintfz(id, RRD_ID_LENGTH_MAX, "db_%s_instance_%s_write_transactions", db, mli->parent->instanceID);
        netdata_fix_chart_name(id);
        mli->st_db_write_transactions = rrdset_create_localhost(
            "mssql",
            id,
            NULL,
            "transactions",
            "mssql.database_write_transactions",
            "Write transactions",
            "transactions/s",
            PLUGIN_WINDOWS_NAME,
            "PerflibMSSQL",
            PRIO_MSSQL_DATABASE_WRITE_TRANSACTIONS,
            update_every,
            RRDSET_TYPE_LINE);

        rrdlabels_add(
            mli->st_db_write_transactions->rrdlabels, "mssql_instance", mli->parent->instanceID, RRDLABEL_SRC_AUTO);
        rrdlabels_add(mli->st_db_write_transactions->rrdlabels, "database", db, RRDLABEL_SRC_AUTO);
    }

    if (!mli->rd_db_write_transactions) {
        mli->rd_db_write_transactions =
            rrddim_add(mli->st_db_write_transactions, "write", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
    }

    if (mli->updated & (1 << NETDATA_MSSQL_ENUM_MDI_IDX_WRITE_TRANSACTIONS)) {
        rrddim_set_by_pointer(
            mli->st_db_write_transactions,
            mli->rd_db_write_transactions,
            (collected_number)mli->MSSQLDatabaseWriteTransactions.current.Data);
    }

    rrdset_done(mli->st_db_write_transactions);
}

static void mssql_active_transactions_chart(struct mssql_db_instance *mli, const char *db, int update_every)
{
    char id[RRD_ID_LENGTH_MAX + 1];

    if (!mli->st_db_active_transactions) {
        snprintfz(id, RRD_ID_LENGTH_MAX, "db_%s_instance_%s_active_transactions", db, mli->parent->instanceID);
        netdata_fix_chart_name(id);
        mli->st_db_active_transactions = rrdset_create_localhost(
            "mssql",
            id,
            NULL,
            "transactions",
            "mssql.database_active_transactions",
            "Active transactions per database",
            "transactions",
            PLUGIN_WINDOWS_NAME,
            "PerflibMSSQL",
            PRIO_MSSQL_DATABASE_ACTIVE_TRANSACTIONS,
            update_every,
            RRDSET_TYPE_LINE);

        rrdlabels_add(
            mli->st_db_active_transactions->rrdlabels, "mssql_instance", mli->parent->instanceID, RRDLABEL_SRC_AUTO);
        rrdlabels_add(mli->st_db_active_transactions->rrdlabels, "database", db, RRDLABEL_SRC_AUTO);
    }

    if (!mli->rd_db_active_transactions) {
        mli->rd_db_active_transactions =
            rrddim_add(mli->st_db_active_transactions, "active", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
    }

    if (mli->updated & (1 << NETDATA_MSSQL_ENUM_MDI_IDX_ACTIVE_TRANSACTIONS)) {
        rrddim_set_by_pointer(
            mli->st_db_active_transactions,
            mli->rd_db_active_transactions,
            (collected_number)mli->MSSQLDatabaseActiveTransactions.current.Data);
    }

    rrdset_done(mli->st_db_active_transactions);
}

static inline void mssql_data_file_size_chart(struct mssql_db_instance *mli, const char *db, int update_every)
{
    char id[RRD_ID_LENGTH_MAX + 1];

    if (!mli->st_db_data_file_size) {
        snprintfz(id, RRD_ID_LENGTH_MAX, "db_%s_instance_%s_data_files_size", db, mli->parent->instanceID);
        netdata_fix_chart_name(id);
        mli->st_db_data_file_size = rrdset_create_localhost(
            "mssql",
            id,
            NULL,
            "size",
            "mssql.database_data_files_size",
            "Current database size.",
            "bytes",
            PLUGIN_WINDOWS_NAME,
            "PerflibMSSQL",
            PRIO_MSSQL_DATABASE_DATA_FILE_SIZE,
            update_every,
            RRDSET_TYPE_LINE);

        rrdlabels_add(
            mli->st_db_data_file_size->rrdlabels, "mssql_instance", mli->parent->instanceID, RRDLABEL_SRC_AUTO);
        rrdlabels_add(mli->st_db_data_file_size->rrdlabels, "database", db, RRDLABEL_SRC_AUTO);
    }

    if (!mli->rd_db_data_file_size) {
        mli->rd_db_data_file_size = rrddim_add(mli->st_db_data_file_size, "size", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
    }

    // FIXME: If the value cannot be retrieved, remove the chart instead of displaying a 0 value.
    collected_number data =
        (mli->updated & (1 << NETDATA_MSSQL_ENUM_MDI_IDX_FILE_SIZE)) ? mli->MSSQLDatabaseDataFileSize.current.Data : 0;
    rrddim_set_by_pointer(mli->st_db_data_file_size, mli->rd_db_data_file_size, data);

    rrdset_done(mli->st_db_data_file_size);
}

int dict_mssql_databases_charts_cb(const DICTIONARY_ITEM *item __maybe_unused, void *value, void *data __maybe_unused)
{
    struct mssql_db_instance *mli = value;
    const char *db = dictionary_acquired_item_name((DICTIONARY_ITEM *)item);

    int *update_every = data;

    void (*transaction_chart[])(struct mssql_db_instance *, const char *, int) = {
        // FIXME: allegedly Netdata collects negative values (MSSQLDatabaseDataFileSize).
        // something is wrong, perflibdump shows correct values.
        // mssql_data_file_size_chart,
        mssql_transactions_chart,
        mssql_database_backup_restore_chart,
        mssql_database_log_flushed_chart,
        mssql_database_log_flushes_chart,
        mssql_active_transactions_chart,
        mssql_write_transactions_chart,

        // Last function pointer must be NULL
        NULL};

    int i;
    for (i = 0; transaction_chart[i]; i++) {
        transaction_chart[i](mli, db, *update_every);
    }

    return 1;
}

static void do_mssql_databases(PERF_DATA_BLOCK *pDataBlock, struct mssql_instance *p, int update_every)
{
    PERF_OBJECT_TYPE *pObjectType = perflibFindObjectTypeByName(pDataBlock, p->objectName[NETDATA_MSSQL_DATABASE]);
    if (!pObjectType)
        return;

    PERF_INSTANCE_DEFINITION *pi = NULL;
    for (LONG i = 0; i < pObjectType->NumInstances; i++) {
        pi = perflibForEachInstance(pDataBlock, pObjectType, pi);
        if (!pi)
            break;

        if (!getInstanceName(pDataBlock, pObjectType, pi, windows_shared_buffer, sizeof(windows_shared_buffer)))
            strncpyz(windows_shared_buffer, "[unknown]", sizeof(windows_shared_buffer) - 1);

        if (!strcasecmp(windows_shared_buffer, "_Total"))
            continue;

        struct mssql_db_instance *mdi = dictionary_set(p->databases, windows_shared_buffer, NULL, sizeof(*mdi));
        if (!mdi)
            continue;

        mdi->updated = 0;
        if (!mdi->parent) {
            mdi->parent = p;
        }

        if (perflibGetObjectCounter(pDataBlock, pObjectType, &mdi->MSSQLDatabaseDataFileSize)) {
            LONGLONG value = (LONGLONG)mdi->MSSQLDatabaseDataFileSize.current.Data;
            if (value > 0)
                mdi->updated |= (1 << NETDATA_MSSQL_ENUM_MDI_IDX_FILE_SIZE);
        }

        if (perflibGetObjectCounter(pDataBlock, pObjectType, &mdi->MSSQLDatabaseActiveTransactions))
            mdi->updated |= (1 << NETDATA_MSSQL_ENUM_MDI_IDX_ACTIVE_TRANSACTIONS);

        if (perflibGetObjectCounter(pDataBlock, pObjectType, &mdi->MSSQLDatabaseBackupRestoreOperations))
            mdi->updated |= (1 << NETDATA_MSSQL_ENUM_MDI_IDX_BACKUP_RESTORE_OP);

        if (perflibGetObjectCounter(pDataBlock, pObjectType, &mdi->MSSQLDatabaseLogFlushed))
            mdi->updated |= (1 << NETDATA_MSSQL_ENUM_MDI_IDX_LOG_FLUSHED);

        if (perflibGetObjectCounter(pDataBlock, pObjectType, &mdi->MSSQLDatabaseLogFlushes))
            mdi->updated |= (1 << NETDATA_MSSQL_ENUM_MDI_IDX_LOG_FLUSHES);

        if (perflibGetObjectCounter(pDataBlock, pObjectType, &mdi->MSSQLDatabaseTransactions))
            mdi->updated |= (1 << NETDATA_MSSQL_ENUM_MDI_IDX_TRANSACTIONS);

        if (perflibGetObjectCounter(pDataBlock, pObjectType, &mdi->MSSQLDatabaseWriteTransactions))
            mdi->updated |= (1 << NETDATA_MSSQL_ENUM_MDI_IDX_WRITE_TRANSACTIONS);
    }

    dictionary_sorted_walkthrough_read(p->databases, dict_mssql_databases_charts_cb, &update_every);
}

static void do_mssql_memory_mgr(PERF_DATA_BLOCK *pDataBlock, struct mssql_instance *p, int update_every)
{
    char id[RRD_ID_LENGTH_MAX + 1];

    PERF_OBJECT_TYPE *pObjectType = perflibFindObjectTypeByName(pDataBlock, p->objectName[NETDATA_MSSQL_MEMORY]);
    if (!pObjectType)
        return;

    if (perflibGetObjectCounter(pDataBlock, pObjectType, &p->MSSQLConnectionMemoryBytes)) {
        if (!p->st_conn_memory) {
            snprintfz(id, RRD_ID_LENGTH_MAX, "instance_%s_memmgr_connection_memory_bytes", p->instanceID);
            netdata_fix_chart_name(id);
            p->st_conn_memory = rrdset_create_localhost(
                "mssql",
                id,
                NULL,
                "memory",
                "mssql.instance_memmgr_connection_memory_bytes",
                "Amount of dynamic memory to maintain connections",
                "bytes",
                PLUGIN_WINDOWS_NAME,
                "PerflibMSSQL",
                PRIO_MSSQL_MEMMGR_CONNECTION_MEMORY_BYTES,
                update_every,
                RRDSET_TYPE_LINE);

            p->rd_conn_memory = rrddim_add(p->st_conn_memory, "memory", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);

            rrdlabels_add(p->st_conn_memory->rrdlabels, "mssql_instance", p->instanceID, RRDLABEL_SRC_AUTO);
        }

        rrddim_set_by_pointer(
            p->st_conn_memory,
            p->rd_conn_memory,
            (collected_number)(p->MSSQLConnectionMemoryBytes.current.Data * 1024));
        rrdset_done(p->st_conn_memory);
    }

    if (perflibGetObjectCounter(pDataBlock, pObjectType, &p->MSSQLExternalBenefitOfMemory)) {
        if (!p->st_ext_benefit_mem) {
            snprintfz(id, RRD_ID_LENGTH_MAX, "instance_%s_memmgr_external_benefit_of_memory", p->instanceID);
            netdata_fix_chart_name(id);
            p->st_ext_benefit_mem = rrdset_create_localhost(
                "mssql",
                id,
                NULL,
                "memory",
                "mssql.instance_memmgr_external_benefit_of_memory",
                "Performance benefit from adding memory to a specific cache",
                "bytes",
                PLUGIN_WINDOWS_NAME,
                "PerflibMSSQL",
                PRIO_MSSQL_MEMMGR_EXTERNAL_BENEFIT_OF_MEMORY,
                update_every,
                RRDSET_TYPE_LINE);

            p->rd_ext_benefit_mem = rrddim_add(p->st_ext_benefit_mem, "benefit", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);

            rrdlabels_add(p->st_ext_benefit_mem->rrdlabels, "mssql_instance", p->instanceID, RRDLABEL_SRC_AUTO);
        }

        rrddim_set_by_pointer(
            p->st_ext_benefit_mem,
            p->rd_ext_benefit_mem,
            (collected_number)p->MSSQLExternalBenefitOfMemory.current.Data);
        rrdset_done(p->st_ext_benefit_mem);
    }

    if (perflibGetObjectCounter(pDataBlock, pObjectType, &p->MSSQLPendingMemoryGrants)) {
        if (!p->st_pending_mem_grant) {
            snprintfz(id, RRD_ID_LENGTH_MAX, "instance_%s_memmgr_pending_memory_grants", p->instanceID);
            netdata_fix_chart_name(id);
            p->st_pending_mem_grant = rrdset_create_localhost(
                "mssql",
                id,
                NULL,
                "memory",
                "mssql.instance_memmgr_pending_memory_grants",
                "Process waiting for memory grant",
                "processes",
                PLUGIN_WINDOWS_NAME,
                "PerflibMSSQL",
                PRIO_MSSQL_MEMMGR_PENDING_MEMORY_GRANTS,
                update_every,
                RRDSET_TYPE_LINE);

            p->rd_pending_mem_grant =
                rrddim_add(p->st_pending_mem_grant, "pending", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);

            rrdlabels_add(p->st_pending_mem_grant->rrdlabels, "mssql_instance", p->instanceID, RRDLABEL_SRC_AUTO);
        }

        rrddim_set_by_pointer(
            p->st_pending_mem_grant,
            p->rd_pending_mem_grant,
            (collected_number)p->MSSQLPendingMemoryGrants.current.Data);

        rrdset_done(p->st_pending_mem_grant);
    }

    if (perflibGetObjectCounter(pDataBlock, pObjectType, &p->MSSQLTotalServerMemory)) {
        if (!p->st_mem_tot_server) {
            snprintfz(id, RRD_ID_LENGTH_MAX, "instance_%s_memmgr_server_memory", p->instanceID);
            netdata_fix_chart_name(id);
            p->st_mem_tot_server = rrdset_create_localhost(
                "mssql",
                id,
                NULL,
                "memory",
                "mssql.instance_memmgr_server_memory",
                "Memory committed",
                "bytes",
                PLUGIN_WINDOWS_NAME,
                "PerflibMSSQL",
                PRIO_MSSQL_MEMMGR_TOTAL_SERVER,
                update_every,
                RRDSET_TYPE_LINE);

            p->rd_mem_tot_server = rrddim_add(p->st_mem_tot_server, "memory", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);

            rrdlabels_add(p->st_mem_tot_server->rrdlabels, "mssql_instance", p->instanceID, RRDLABEL_SRC_AUTO);
        }

        rrddim_set_by_pointer(
            p->st_mem_tot_server,
            p->rd_mem_tot_server,
            (collected_number)(p->MSSQLTotalServerMemory.current.Data * 1024));

        rrdset_done(p->st_mem_tot_server);
    }
}

int dict_mssql_charts_cb(const DICTIONARY_ITEM *item __maybe_unused, void *value, void *data __maybe_unused)
{
    struct mssql_instance *p = value;
    int *update_every = data;

    static void (*doMSSQL[])(PERF_DATA_BLOCK *, struct mssql_instance *, int) = {
        do_mssql_general_stats,
        do_mssql_errors,
        do_mssql_databases,
        do_mssql_locks,
        do_mssql_memory_mgr,
        do_mssql_buffer_management,
        do_mssql_sql_statistics,
        do_mssql_access_methods};

    DWORD i;
    for (i = 0; i < NETDATA_MSSQL_METRICS_END; i++) {
        if (!doMSSQL[i])
            continue;

        DWORD id = RegistryFindIDByName(p->objectName[i]);
        if (id == PERFLIB_REGISTRY_NAME_NOT_FOUND)
            return -1;

        PERF_DATA_BLOCK *pDataBlock = perflibGetPerformanceData(id);
        if (!pDataBlock)
            return -1;

        doMSSQL[i](pDataBlock, p, *update_every);
    }

    return 1;
}

int do_PerflibMSSQL(int update_every, usec_t dt __maybe_unused)
{
    static bool initialized = false;

    if (unlikely(!initialized)) {
        if (initialize())
            return -1;

        initialized = true;
    }

    dictionary_sorted_walkthrough_read(mssql_instances, dict_mssql_charts_cb, &update_every);

    return 0;
}
