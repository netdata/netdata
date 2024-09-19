// SPDX-License-Identifier: GPL-3.0-or-later

#include "windows_plugin.h"
#include "windows-internals.h"

// https://learn.microsoft.com/en-us/sql/sql-server/install/instance-configuration?view=sql-server-ver16
#define NETDATA_MAX_INSTANCE_NAME 32
#define NETDATA_MAX_INSTANCE_OBJECT 128

struct mssql_instance {
    char *genStats;
    char *sqlErrors;
    char *databases;
    char *transactions;
    char *sqlStatistics;
    char *bufMan;
    char *memmgr;
    char *locks;

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

static bool do_MSSQL(PERF_DATA_BLOCK *pDataBlock, int update_every) {
    return true;
}

int do_PerflibMSSQL(int update_every, usec_t dt __maybe_unused)
{
    static bool initialized = false;

    if (unlikely(!initialized)) {
        if (initialize())
            return -1;

        initialized = true;
    }

    do_MSSQL(NULL, update_every);

    return 0;
}
