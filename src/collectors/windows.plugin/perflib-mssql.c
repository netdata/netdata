// SPDX-License-Identifier: GPL-3.0-or-later

#include "windows_plugin.h"
#include "windows-internals.h"

// https://learn.microsoft.com/en-us/sql/sql-server/install/instance-configuration?view=sql-server-ver16
#define NETDATA_MAX_INSTANCE_NAME 32
#define NETDATA_MAX_INSTANCE_OBJECT 128

enum netdata_mssql_metrics {
    NETDATA_MSSQL_GENERAL_STATS,
    NETDATA_MSSQL_SQL_ERRORS,
    NETDATA_MSSQL_DATABASE,
    NETDATA_MSSQL_LOCKS,
    NETDATA_MSSQL_MEMORY,
    NETDATA_MSSQL_BUFFER_MANAGEMENT,
    NETDATA_MSSQL_SQL_STATS,
    NETDATA_MSSQL_TRANSACTIONS
};

struct mssql_instance {
    char *instanceID;

    char *genStats;
    char *sqlErrors;
    char *databases;
    char *transactions;
    char *sqlStatistics;
    char *bufMan;
    char *memmgr;
    char *locks;

    RRDSET *st_user_connections;
    RRDDIM *rd_user_connections;

    RRDSET *st_process_blocked;
    RRDDIM *rd_process_blocked;

    COUNTER_DATA MSSQLAccessMethodPageSplits;
    COUNTER_DATA MSSQLBufferCacheHits;
    COUNTER_DATA MSSQLBufferCacheLookups;
    COUNTER_DATA MSSQLBufferCheckpointPages;
    COUNTER_DATA MSSQLBufferPageLifeExpectancy;
    COUNTER_DATA MSSQLBufferPageReads;
    COUNTER_DATA MSSQLBufferPageWrites;
    COUNTER_DATA MSSQLBlockedProcesses;
    COUNTER_DATA MSSQLUserConnections;
    COUNTER_DATA MSSQLLockWait;
    COUNTER_DATA MSSQLDeadlocks;
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

    COUNTER_DATA MSSQLDatabaseActiveTransactions;
    COUNTER_DATA MSSQLDatabaseBackupRestoreOperations;
    COUNTER_DATA MSSQLDatabaseDataFileSize;
    COUNTER_DATA MSSQLDatabaseLogFlushed;
    COUNTER_DATA MSSQLDatabaseLogFlushes;
    COUNTER_DATA MSSQLDatabaseTransactions;
    COUNTER_DATA MSSQLDatabaseWriteTransactions;
};

static DICTIONARY *mssql_instances = NULL;

static inline void initialize_mssql_objects(struct mssql_instance *p, const char *instance) {
    char prefix[NETDATA_MAX_INSTANCE_NAME];
    if (!strcmp(instance, "MSSQLSERVER")) {
        strncpyz(prefix, "SQLServer:", sizeof(prefix) -1);
    } else {
        snprintfz(prefix, sizeof(prefix) -1, "MSSQL$:%s:", instance);
    }

    size_t length = strlen(prefix);
    char name[NETDATA_MAX_INSTANCE_OBJECT];
    snprintfz(name, sizeof(name) -1, "%s%s", prefix, "General Statistics");
    p->genStats = strdup(name);

    strncpyz(&name[length], "SQL Errors", sizeof(name) - length);
    p->sqlErrors = strdup(name);

    strncpyz(&name[length], "Databases", sizeof(name) - length);
    p->databases = strdup(name);

    strncpyz(&name[length], "Transactions", sizeof(name) - length);
    p->transactions = strdup(name);

    strncpyz(&name[length], "SQL Statistics", sizeof(name) - length);
    p->sqlStatistics = strdup(name);

    strncpyz(&name[length], "Buffer Manager", sizeof(name) - length);
    p->bufMan = strdup(name);

    strncpyz(&name[length], "Memory Manager", sizeof(name) - length);
    p->memmgr = strdup(name);

    strncpyz(&name[length], "Locks", sizeof(name) - length);
    p->locks = strdup(name);

    p->instanceID = strdup(instance);
    netdata_fix_chart_name(p->instanceID);
}

static inline void initialize_mssql_keys(struct mssql_instance *p) {
    // General Statistics
    p->MSSQLUserConnections.key = "User Connections";
    p->MSSQLBlockedProcesses.key = "Processes blocked";

    /*
    p->MSSQLAccessMethodPageSplits.key = "";
    p->MSSQLBufferCacheHits.key = "";
    p->MSSQLBufferCacheLookups.key = "";
    p->MSSQLBufferCheckpointPages.key = "";
    p->MSSQLBufferPageLifeExpectancy.key = "";
    p->MSSQLBufferPageReads.key = "";
    p->MSSQLBufferPageWrites.key = "";
    p->MSSQLLockWait.key = "";
    p->MSSQLDeadlocks.key = "";
    p->MSSQLConnectionMemoryBytes.key = "";
    p->MSSQLExternalBenefitOfMemory.key = "";
    p->MSSQLPendingMemoryGrants.key = "";
    p->MSSQLSQLErrorsTotal.key = "";
    p->MSSQLTotalServerMemory.key = "";
    p->MSSQLStatsAutoParameterization.key = "";
    p->MSSQLStatsBatchRequests.key = "";
    p->MSSQLStatSafeAutoParameterization.key = "";
    p->MSSQLCompilations.key = "";
    p->MSSQLRecompilations.key = "";

    p->MSSQLDatabaseActiveTransactions.key = "";
    p->MSSQLDatabaseBackupRestoreOperations.key = "";
    p->MSSQLDatabaseDataFileSize.key = "";
    p->MSSQLDatabaseLogFlushed.key = "";
    p->MSSQLDatabaseLogFlushes.key = "";
    p->MSSQLDatabaseTransactions.key = "";
    p->MSSQLDatabaseWriteTransactions.key = "";
     */
}

void dict_mssql_insert_cb(const DICTIONARY_ITEM *item __maybe_unused, void *value, void *data __maybe_unused) {
    struct mssql_instance *p = value;
    const char *instance = dictionary_acquired_item_name((DICTIONARY_ITEM *)item);

    initialize_mssql_objects(p, instance);
    initialize_mssql_keys(p);
}

static int mssql_fill_dictionary() {
    HKEY hKey;
    LSTATUS ret = RegOpenKeyExA(HKEY_LOCAL_MACHINE,
                                "SOFTWARE\\Microsoft\\Microsoft SQL Server\\Instance Names\\SQL",
                                0,
                                KEY_READ,
                                &hKey);
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
    DWORD length = REGISTRY_MAX_VALUE;
    for (i = 0; i < values; i++) {
        avalue[0] = '\0';

        ret = RegEnumValue(hKey, i, avalue, &length, NULL, NULL, NULL, NULL);
        if (ret != ERROR_SUCCESS)
            continue;

        struct mssql_instance *p = dictionary_set(mssql_instances, avalue, NULL, sizeof(*p));
    }

endMSSQLFillDict:
    RegCloseKey(hKey);

    return (ret == ERROR_SUCCESS) ? 0 : -1;
}

static int initialize(void) {
    mssql_instances = dictionary_create_advanced(DICT_OPTION_DONT_OVERWRITE_VALUE |
                                                     DICT_OPTION_FIXED_SIZE, NULL, sizeof(struct mssql_instance));

    dictionary_register_insert_callback(mssql_instances, dict_mssql_insert_cb, NULL);

    if (mssql_fill_dictionary()) {
        return -1;
    }

    return 0;
}

static inline void do_MSSQL_GENERAL_STATS(PERF_DATA_BLOCK *pDataBlock, struct mssql_instance *p, int update_every)
{
    char id[RRD_ID_LENGTH_MAX + 1];
    PERF_OBJECT_TYPE *pObjectType = perflibFindObjectTypeByName(pDataBlock, p->genStats);
    if (!pObjectType)
        return;

    if (perflibGetObjectCounter(pDataBlock, pObjectType, &p->MSSQLUserConnections)) {
        snprintfz(id, RRD_ID_LENGTH_MAX, "instance_%s_user_connection", p->instanceID);
        if (!p->st_user_connections) {
            p->st_user_connections = rrdset_create_localhost("mssql"
                                                             , id, NULL
                                                             , "connections"
                                                             , "mssql.instance_user_connection"
                                                             , "User connections"
                                                             , "connections"
                                                             , PLUGIN_WINDOWS_NAME
                                                             , "MSSQL"
                                                             , PRIO_MSSQL_USER_CONNECTIONS
                                                             , update_every
                                                             , RRDSET_TYPE_LINE
                                                             );

            snprintfz(id, RRD_ID_LENGTH_MAX, "mssql_instance_%s_genstats_user_connections", p->instanceID);
            p->rd_user_connections  = rrddim_add(p->st_user_connections,
                                                id,
                                                "user",
                                                1,
                                                1,
                                                RRD_ALGORITHM_ABSOLUTE);

            rrdlabels_add(p->st_file_transfer->rrdlabels, "mssql_instance", p->instanceID, RRDLABEL_SRC_AUTO);
        }

        rrddim_set_by_pointer(p->st_user_connections,
                              p->rd_user_connections,
                              (collected_number)p->MSSQLUserConnections.current.Data);
        rrdset_done(p->st_user_connections);
    }

    if (perflibGetObjectCounter(pDataBlock, pObjectType, &p->MSSQLBlockedProcesses)) {
        snprintfz(id, RRD_ID_LENGTH_MAX, "instance_%s_blocked_process", p->instanceID);
        if (!p->st_process_blocked) {
            p->st_process_blocked = rrdset_create_localhost("mssql"
                                                            , id, NULL
                                                            , "processes"
                                                            , "mssql.instance_blocked_processes"
                                                            , "Blocked processes"
                                                            , "process"
                                                            , PLUGIN_WINDOWS_NAME
                                                            , "MSSQL"
                                                            , PRIO_MSSQL_BLOCKED_PROCESSES
                                                            , update_every
                                                            , RRDSET_TYPE_LINE
                                                            );

            snprintfz(id, RRD_ID_LENGTH_MAX, "mssql_instance_%s_genstats_blocked_processes", p->instanceID);
            p->rd_process_blocked  = rrddim_add(p->st_process_blocked,
                                                 id,
                                                 "blocked",
                                                 1,
                                                 1,
                                                 RRD_ALGORITHM_ABSOLUTE);

            rrdlabels_add(p->st_file_transfer->rrdlabels, "mssql_instance", p->instanceID, RRDLABEL_SRC_AUTO);
        }

        rrddim_set_by_pointer(p->st_process_blocked,
                              p->rd_process_blocked,
                              (collected_number)p->MSSQLBlockedProcesses.current.Data);
        rrdset_done(p->st_process_blocked);
    }
}

static bool do_MSSQL(PERF_DATA_BLOCK *pDataBlock, struct mssql_instance *instance, int update_every, int caller_idx)
{
    switch(caller_idx) {
        case NETDATA_MSSQL_GENERAL_STATS: {
            do_MSSQL_GENERAL_STATS(pDataBlock, instance, update_every);
            break;
        }
        case NETDATA_MSSQL_SQL_ERRORS: {
            break;
        }
        case NETDATA_MSSQL_DATABASE: {
            break;
        }
        case NETDATA_MSSQL_LOCKS: {
            break;
        }
        case NETDATA_MSSQL_MEMORY: {
            break;
        }
        case NETDATA_MSSQL_BUFFER_MANAGEMENT: {
            break;
        }
        case NETDATA_MSSQL_SQL_STATS: {
            break;
        }
        case NETDATA_MSSQL_TRANSACTIONS: {
            break;
        }
    }

    return true;
}

int dict_mssql_charts_cb(const DICTIONARY_ITEM *item __maybe_unused, void *value, void *data __maybe_unused) {
    struct mssql_instance *p = value;
    int *update_every = data;

    char *loop_through_data[] = {
        p->genStats,
        p->sqlErrors,
        p->databases,
        p->locks,
        p->memmgr,
        p->bufMan,
        p->sqlStatistics,
        p->transactions,
        NULL
    };

    DWORD i;
    for (i = 0; loop_through_data[i] ; i++) {
        DWORD id = RegistryFindIDByName(loop_through_data[i]);
        if(id == PERFLIB_REGISTRY_NAME_NOT_FOUND)
            return -1;

        PERF_DATA_BLOCK *pDataBlock = perflibGetPerformanceData(id);
        if(!pDataBlock) return -1;

        do_MSSQL(pDataBlock, p, *update_every, i);
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
