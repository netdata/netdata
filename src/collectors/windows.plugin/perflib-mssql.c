// SPDX-License-Identifier: GPL-3.0-or-later

#include "windows_plugin.h"
#include "windows-internals.h"

#include "perflib-mssql-queries.h"

#include <sqlext.h>
#include <sqltypes.h>
#include <sql.h>

#define MEGA_FACTOR (1048576) // 1024 * 1024
// https://learn.microsoft.com/en-us/sql/sql-server/install/instance-configuration?view=sql-server-ver16
#define NETDATA_MAX_INSTANCE_NAME (32)
#define NETDATA_MAX_INSTANCE_OBJECT (128)
// https://learn.microsoft.com/en-us/previous-versions/sql/sql-server-2008-r2/ms191240(v=sql.105)#sysname
#define SQLSERVER_MAX_NAME_LENGTH NETDATA_MAX_INSTANCE_OBJECT
#define NETDATA_MSSQL_NEXT_TRY (60)

struct netdata_mssql_conn {
    const char *instance;
    const char *driver;
    const char *server;
    const char *address;
    const char *username;
    const char *password;
    int instances;
    bool windows_auth;
    bool is_sqlexpress;

    SQLCHAR *connectionString;

    SQLHENV netdataSQLEnv;
    SQLHDBC netdataSQLHDBc;

    SQLHSTMT checkPermSTMT;
    SQLHSTMT databaseListSTMT;
    SQLHSTMT dataFileSizeSTMT;
    SQLHSTMT dbTransactionSTMT;
    SQLHSTMT dbInstanceTransactionSTMT;
    SQLHSTMT dbWaitsSTMT;
    SQLHSTMT dbLocksSTMT;
    SQLHSTMT dbSQLState;

    BOOL is_connected;
};

DICTIONARY *conn_options;

enum netdata_mssql_metrics {
    NETDATA_MSSQL_GENERAL_STATS,
    NETDATA_MSSQL_SQL_ERRORS,
    NETDATA_MSSQL_MEMORY,
    NETDATA_MSSQL_SQL_STATS,
    NETDATA_MSSQL_ACCESS_METHODS,

    NETDATA_MSSQL_DATABASE,
    NETDATA_MSSQL_LOCKS,
    NETDATA_MSSQL_WAITS,
    NETDATA_MSSQL_BUFFER_MANAGEMENT,

    NETDATA_MSSQL_METRICS_END
};

struct mssql_db_waits {
    const char *wait_type;
    const char *wait_category;

    RRDSET *st_total_wait;
    RRDDIM *rd_total_wait;

    RRDSET *st_resource_wait_msec;
    RRDDIM *rd_resource_wait_msec;

    RRDSET *st_signal_wait_msec;
    RRDDIM *rd_signal_wait_msec;

    RRDSET *st_max_wait_time_msec;
    RRDDIM *rd_max_wait_time_msec;

    RRDSET *st_waiting_tasks;
    RRDDIM *rd_waiting_tasks;

    COUNTER_DATA MSSQLDatabaseTotalWait;
    COUNTER_DATA MSSQLDatabaseResourceWaitMSec;
    COUNTER_DATA MSSQLDatabaseSignalWaitMSec;
    COUNTER_DATA MSSQLDatabaseMaxWaitTimeMSec;
    COUNTER_DATA MSSQLDatabaseWaitingTasks;
};

struct mssql_instance {
    char *instanceID;
    int update_every;

    struct netdata_mssql_conn *conn;

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

    RRDSET *st_access_method_page_splits;
    RRDDIM *rd_access_method_page_splits;

    RRDSET *st_sql_errors;
    RRDDIM *rd_sql_errors;

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

    DICTIONARY *waits;

    COUNTER_DATA MSSQLAccessMethodPageSplits;
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
};

struct mssql_lock_instance {
    struct mssql_instance *parent;

    char *resourceID;

    COUNTER_DATA lockWait;
    COUNTER_DATA deadLocks;

    RRDSET *st_deadLocks;
    RRDDIM *rd_lockWait;

    RRDSET *st_lockWait;
    RRDDIM *rd_deadLocks;
};

struct mssql_db_instance {
    struct mssql_instance *parent;

    bool collecting_data;
    bool collect_instance;

    RRDSET *st_db_data_file_size;
    RRDSET *st_db_active_transactions;
    RRDSET *st_db_backup_restore_operations;
    RRDSET *st_db_log_flushed;
    RRDSET *st_db_log_flushes;
    RRDSET *st_db_transactions;
    RRDSET *st_db_write_transactions;
    RRDSET *st_db_lockwait;
    RRDSET *st_db_deadlock;
    RRDSET *st_lock_timeouts;
    RRDSET *st_lock_requests;
    RRDSET *st_buff_page_iops;
    RRDSET *st_buff_cache_hits;
    RRDSET *st_buff_checkpoint_pages;
    RRDSET *st_buff_cache_page_life_expectancy;
    RRDSET *st_buff_lazy_write;
    RRDSET *st_buff_page_lookups;

    RRDSET *st_stats_compilation;
    RRDSET *st_stats_recompiles;

    RRDDIM *rd_db_data_file_size;
    RRDDIM *rd_db_active_transactions;
    RRDDIM *rd_db_backup_restore_operations;
    RRDDIM *rd_db_log_flushed;
    RRDDIM *rd_db_log_flushes;
    RRDDIM *rd_db_transactions;
    RRDDIM *rd_db_write_transactions;
    RRDDIM *rd_db_lockwait;
    RRDDIM *rd_db_deadlock;
    RRDDIM *rd_lock_timeouts;
    RRDDIM *rd_lock_requests;
    RRDDIM *rd_buff_page_reads;
    RRDDIM *rd_buff_page_writes;
    RRDDIM *rd_buff_cache_hits;
    RRDDIM *rd_buff_checkpoint_pages;
    RRDDIM *rd_buff_cache_page_life_expectancy;
    RRDDIM *rd_buff_lazy_write;
    RRDDIM *rd_buff_page_lookups;

    RRDDIM *rd_stats_compilation;
    RRDDIM *rd_stats_recompiles;

    COUNTER_DATA MSSQLDatabaseDataFileSize;

    COUNTER_DATA MSSQLDatabaseActiveTransactions;
    COUNTER_DATA MSSQLDatabaseBackupRestoreOperations;
    COUNTER_DATA MSSQLDatabaseLogFlushed;
    COUNTER_DATA MSSQLDatabaseLogFlushes;
    COUNTER_DATA MSSQLDatabaseTransactions;
    COUNTER_DATA MSSQLDatabaseWriteTransactions;

    COUNTER_DATA MSSQLDatabaseLockWaitSec;
    COUNTER_DATA MSSQLDatabaseDeadLockSec;
    COUNTER_DATA MSSQLDatabaseLockTimeoutsSec;
    COUNTER_DATA MSSQLDatabaseLockRequestsSec;

    // Buffer Management (https://learn.microsoft.com/en-us/sql/relational-databases/performance-monitor/sql-server-buffer-manager-object)
    COUNTER_DATA MSSQLBufferPageReads;
    COUNTER_DATA MSSQLBufferPageWrites;
    COUNTER_DATA MSSQLBufferCacheHits;
    COUNTER_DATA MSSQLBufferCheckpointPages;
    COUNTER_DATA MSSQLBufferPageLifeExpectancy;
    COUNTER_DATA MSSQLBufferLazyWrite;
    COUNTER_DATA MSSQLBufferPageLookups;

    COUNTER_DATA MSSQLCompilations;
    COUNTER_DATA MSSQLRecompilations;

    COUNTER_DATA MSSQLDBIsReadonly;
    COUNTER_DATA MSSQLDBState;

    uint32_t updated;
};

enum netdata_mssql_odbc_errors {
    NETDATA_MSSQL_ODBC_NO_ERROR,
    NETDATA_MSSQL_ODBC_CONNECT,
    NETDATA_MSSQL_ODBC_BIND,
    NETDATA_MSSQL_ODBC_PREPARE,
    NETDATA_MSSQL_ODBC_QUERY,
    NETDATA_MSSQL_ODBC_FETCH
};

static char *netdata_MSSQL_error_text(enum netdata_mssql_odbc_errors val)
{
    switch (val) {
        case NETDATA_MSSQL_ODBC_NO_ERROR:
            return "NO ERROR";
        case NETDATA_MSSQL_ODBC_CONNECT:
            return "CONNECTION";
        case NETDATA_MSSQL_ODBC_BIND:
            return "BIND PARAMETER";
        case NETDATA_MSSQL_ODBC_PREPARE:
            return "PREPARE PARAMETER";
        case NETDATA_MSSQL_ODBC_QUERY:
            return "QUERY PARAMETER";
        case NETDATA_MSSQL_ODBC_FETCH:
        default:
            return "QUERY FETCH";
    }
}

static char *netdata_MSSQL_type_text(uint32_t type)
{
    switch (type) {
        case SQL_HANDLE_STMT:
            return "STMT";
        case SQL_HANDLE_DBC:
        default:
            return "DBC";
    }
}

// Connection and SQL
static void netdata_MSSQL_error(uint32_t type, SQLHANDLE handle, enum netdata_mssql_odbc_errors step, char *instance)
{
    SQLCHAR state[1024];
    SQLCHAR message[1024];
    if (SQL_SUCCESS == SQLGetDiagRec((short)type, handle, 1, state, NULL, message, 1024, NULL)) {
        char *str_step = netdata_MSSQL_error_text(step);
        char *str_type = netdata_MSSQL_type_text(type);
        char *use_instance = (!instance) ? "no instance" : instance;
        nd_log(
            NDLS_COLLECTORS,
            NDLP_INFO,
            "MSSQL server error on %s using the handle %s running %s :  %s, %s",
            use_instance,
            str_type,
            str_step,
            message,
            state);
    }
}

static inline void netdata_MSSQL_release_results(SQLHSTMT *stmt)
{
    SQLFreeStmt(stmt, SQL_CLOSE);
    SQLFreeStmt(stmt, SQL_UNBIND);
    SQLFreeStmt(stmt, SQL_RESET_PARAMS);
}

static ULONGLONG netdata_MSSQL_fill_long_value(SQLHSTMT *stmt, const char *mask, const char *dbname, char *instance)
{
    long db_size = 0;
    SQLLEN col_data_len = 0;

    SQLCHAR query[512];
    snprintfz((char *)query, 511, mask, dbname);

    SQLRETURN ret;

    ret = SQLExecDirect(stmt, query, SQL_NTS);
    if (ret != SQL_SUCCESS) {
        netdata_MSSQL_error(SQL_HANDLE_STMT, stmt, NETDATA_MSSQL_ODBC_QUERY, instance);
        return (ULONGLONG)ULONG_LONG_MAX;
    }

    ret = SQLBindCol(stmt, 1, SQL_C_LONG, &db_size, sizeof(long), &col_data_len);

    if (ret != SQL_SUCCESS) {
        netdata_MSSQL_error(SQL_HANDLE_STMT, stmt, NETDATA_MSSQL_ODBC_PREPARE, instance);
        return (ULONGLONG)ULONG_LONG_MAX;
    }

    ret = SQLFetch(stmt);
    if (ret != SQL_SUCCESS) {
        netdata_MSSQL_error(SQL_HANDLE_STMT, stmt, NETDATA_MSSQL_ODBC_FETCH, instance);
        return (ULONGLONG)ULONG_LONG_MAX;
    }

    netdata_MSSQL_release_results(stmt);
    return (ULONGLONG)(db_size * MEGA_FACTOR);
}

#define NETDATA_MSSQL_BUFFER_PAGE_READS_METRIC "Page reads/sec"
#define NETDATA_MSSQL_BUFFER_PAGE_WRITES_METRIC "Page writes/sec"
#define NETDATA_MSSQL_BUFFER_PAGE_CACHE_METRIC "Buffer cache hit ratio"
#define NETDATA_MSSQL_BUFFER_CHECKPOINT_METRIC "Checkpoint pages/sec"
#define NETDATA_MSSQL_BUFFER_PAGE_LIFE_METRIC "Page life expectancy"
#define NETDATA_MSSQL_BUFFER_LAZY_WRITES_METRIC "Lazy writes/sec"
#define NETDATA_MSSQL_BUFFER_PAGE_LOOKUPS_METRIC "Page Lookups/sec"

#define NETDATA_MSSQL_STATS_COMPILATIONS_METRIC "SQL Compilations/sec"
#define NETDATA_MSSQL_STATS_RECOMPILATIONS_METRIC "SQL Re-Compilations/sec"

void dict_mssql_fill_instance_transactions(struct mssql_db_instance *mdi)
{
    char object_name[NETDATA_MAX_INSTANCE_OBJECT + 1] = {};
    long value = 0;
    SQLLEN col_object_len = 0, col_value_len = 0;

    SQLCHAR query[sizeof(NETDATA_QUERY_TRANSACTIONS_PER_INSTANCE_MASK) + NETDATA_MAX_INSTANCE_OBJECT + 1];
    snprintfz(
            (char *)query,
            sizeof(NETDATA_QUERY_TRANSACTIONS_PER_INSTANCE_MASK) + NETDATA_MAX_INSTANCE_OBJECT,
            NETDATA_QUERY_TRANSACTIONS_PER_INSTANCE_MASK,
            mdi->parent->instanceID);

    SQLRETURN ret = SQLExecDirect(mdi->parent->conn->dbInstanceTransactionSTMT, (SQLCHAR *)query, SQL_NTS);
    if (ret != SQL_SUCCESS) {
        mdi->collecting_data = false;
        netdata_MSSQL_error(
                SQL_HANDLE_STMT, mdi->parent->conn->dbInstanceTransactionSTMT, NETDATA_MSSQL_ODBC_QUERY, mdi->parent->instanceID);
        goto enditransactions;
    }

    ret = SQLBindCol(
            mdi->parent->conn->dbInstanceTransactionSTMT, 1, SQL_C_CHAR, object_name, sizeof(object_name), &col_object_len);
    if (ret != SQL_SUCCESS) {
        netdata_MSSQL_error(
                SQL_HANDLE_STMT, mdi->parent->conn->dbInstanceTransactionSTMT, NETDATA_MSSQL_ODBC_PREPARE, mdi->parent->instanceID);
        goto enditransactions;
    }

    ret = SQLBindCol(mdi->parent->conn->dbInstanceTransactionSTMT, 2, SQL_C_LONG, &value, sizeof(value), &col_value_len);
    if (ret != SQL_SUCCESS) {
        netdata_MSSQL_error(
                SQL_HANDLE_STMT, mdi->parent->conn->dbInstanceTransactionSTMT, NETDATA_MSSQL_ODBC_PREPARE, mdi->parent->instanceID);
        goto enditransactions;
    }

    do {
        ret = SQLFetch(mdi->parent->conn->dbInstanceTransactionSTMT);
        switch (ret) {
            case SQL_SUCCESS:
            case SQL_SUCCESS_WITH_INFO:
                break;
            case SQL_NO_DATA:
            default:
                goto enditransactions;
        }

        // We cannot use strcmp, because buffer is filled with spaces instead NULL.
        if (!strncmp(
                object_name, NETDATA_MSSQL_BUFFER_PAGE_READS_METRIC, sizeof(NETDATA_MSSQL_BUFFER_PAGE_READS_METRIC) - 1))
            mdi->MSSQLBufferPageReads.current.Data = (ULONGLONG)value;
        else if (!strncmp(
                object_name, NETDATA_MSSQL_BUFFER_PAGE_WRITES_METRIC, sizeof(NETDATA_MSSQL_BUFFER_PAGE_WRITES_METRIC) - 1))
            mdi->MSSQLBufferPageWrites.current.Data = (ULONGLONG)value;
        else if (!strncmp(
                object_name, NETDATA_MSSQL_BUFFER_PAGE_CACHE_METRIC, sizeof(NETDATA_MSSQL_BUFFER_PAGE_CACHE_METRIC) - 1))
            mdi->MSSQLBufferCacheHits.current.Data = (ULONGLONG)value;
        else if (!strncmp(
                object_name, NETDATA_MSSQL_BUFFER_CHECKPOINT_METRIC, sizeof(NETDATA_MSSQL_BUFFER_CHECKPOINT_METRIC) - 1))
            mdi->MSSQLBufferCheckpointPages.current.Data = (ULONGLONG)value;
        else if (!strncmp(
                object_name, NETDATA_MSSQL_BUFFER_PAGE_LIFE_METRIC, sizeof(NETDATA_MSSQL_BUFFER_PAGE_LIFE_METRIC) - 1))
            mdi->MSSQLBufferPageLifeExpectancy.current.Data = (ULONGLONG)value;
        else if (!strncmp(
                object_name, NETDATA_MSSQL_BUFFER_LAZY_WRITES_METRIC, sizeof(NETDATA_MSSQL_BUFFER_LAZY_WRITES_METRIC) - 1))
            mdi->MSSQLBufferLazyWrite.current.Data = (ULONGLONG)value;
        else if (!strncmp(
                object_name, NETDATA_MSSQL_BUFFER_PAGE_LOOKUPS_METRIC, sizeof(NETDATA_MSSQL_BUFFER_PAGE_LOOKUPS_METRIC) - 1))
            mdi->MSSQLBufferPageLookups.current.Data = (ULONGLONG)value;
        else if (!strncmp(
                object_name, NETDATA_MSSQL_STATS_COMPILATIONS_METRIC, sizeof(NETDATA_MSSQL_STATS_COMPILATIONS_METRIC) - 1))
            mdi->MSSQLCompilations.current.Data = (ULONGLONG)value;
        else if (!strncmp(
                object_name, NETDATA_MSSQL_STATS_RECOMPILATIONS_METRIC, sizeof(NETDATA_MSSQL_STATS_RECOMPILATIONS_METRIC) - 1))
            mdi->MSSQLRecompilations.current.Data = (ULONGLONG)value;
    } while (true);

    enditransactions:
    netdata_MSSQL_release_results(mdi->parent->conn->dbInstanceTransactionSTMT);
}

#define NETDATA_MSSQL_ACTIVE_TRANSACTIONS_METRIC "Active Transactions"
#define NETDATA_MSSQL_TRANSACTION_PER_SEC_METRIC "Transactions/sec"
#define NETDATA_MSSQL_WRITE_TRANSACTIONS_METRIC "Write Transactions/sec"
#define NETDATA_MSSQL_BACKUP_RESTORE_METRIC "Backup/Restore Throughput/sec"
#define NETDATA_MSSQL_LOG_FLUSHED_METRIC "Log Bytes Flushed/sec"
#define NETDATA_MSSQL_LOG_FLUSHES_METRIC "Log Flushes/sec"
#define NETDATA_MSSQL_NUMBER_DEADLOCKS_METRIC "Number of Deadlocks/sec"
#define NETDATA_MSSQL_LOCK_WAITS_METRIC "Lock Waits/sec"
#define NETDATA_MSSQL_LOCK_TIMEOUTS_METRIC "Lock Timeouts/sec"
#define NETDATA_MSSQL_LOCK_REQUESTS_METRIC "Lock Requests/sec"

void dict_mssql_fill_transactions(struct mssql_db_instance *mdi, const char *dbname)
{
    char object_name[NETDATA_MAX_INSTANCE_OBJECT + 1] = {};
    long value = 0;
    SQLLEN col_object_len = 0, col_value_len = 0;

    if (mdi->collect_instance)
        dict_mssql_fill_instance_transactions(mdi);

    SQLCHAR query[sizeof(NETDATA_QUERY_TRANSACTIONS_MASK) + 2 * NETDATA_MAX_INSTANCE_OBJECT + 1];
    snprintfz(
        (char *)query,
        sizeof(NETDATA_QUERY_TRANSACTIONS_MASK) + 2 * NETDATA_MAX_INSTANCE_OBJECT,
        NETDATA_QUERY_TRANSACTIONS_MASK,
        dbname,
        dbname);

    SQLRETURN ret = SQLExecDirect(mdi->parent->conn->dbTransactionSTMT, (SQLCHAR *)query, SQL_NTS);
    if (ret != SQL_SUCCESS) {
        mdi->collecting_data = false;
        netdata_MSSQL_error(
            SQL_HANDLE_STMT, mdi->parent->conn->dbTransactionSTMT, NETDATA_MSSQL_ODBC_QUERY, mdi->parent->instanceID);
        goto endtransactions;
    }

    ret = SQLBindCol(
        mdi->parent->conn->dbTransactionSTMT, 1, SQL_C_CHAR, object_name, sizeof(object_name), &col_object_len);
    if (ret != SQL_SUCCESS) {
        netdata_MSSQL_error(
            SQL_HANDLE_STMT, mdi->parent->conn->dbTransactionSTMT, NETDATA_MSSQL_ODBC_PREPARE, mdi->parent->instanceID);
        goto endtransactions;
    }

    ret = SQLBindCol(mdi->parent->conn->dbTransactionSTMT, 2, SQL_C_LONG, &value, sizeof(value), &col_value_len);
    if (ret != SQL_SUCCESS) {
        netdata_MSSQL_error(
            SQL_HANDLE_STMT, mdi->parent->conn->dbTransactionSTMT, NETDATA_MSSQL_ODBC_PREPARE, mdi->parent->instanceID);
        goto endtransactions;
    }

    do {
        ret = SQLFetch(mdi->parent->conn->dbTransactionSTMT);
        switch (ret) {
            case SQL_SUCCESS:
            case SQL_SUCCESS_WITH_INFO:
                break;
            case SQL_NO_DATA:
            default:
                goto endtransactions;
        }

        // We cannot use strcmp, because buffer is filled with spaces instead NULL.
        if (!strncmp(
                object_name,
                NETDATA_MSSQL_ACTIVE_TRANSACTIONS_METRIC,
                sizeof(NETDATA_MSSQL_ACTIVE_TRANSACTIONS_METRIC) - 1))
            mdi->MSSQLDatabaseActiveTransactions.current.Data = (ULONGLONG)value;
        else if (!strncmp(
                     object_name,
                     NETDATA_MSSQL_TRANSACTION_PER_SEC_METRIC,
                     sizeof(NETDATA_MSSQL_TRANSACTION_PER_SEC_METRIC) - 1))
            mdi->MSSQLDatabaseTransactions.current.Data = (ULONGLONG)value;
        else if (!strncmp(
                     object_name,
                     NETDATA_MSSQL_WRITE_TRANSACTIONS_METRIC,
                     sizeof(NETDATA_MSSQL_WRITE_TRANSACTIONS_METRIC) - 1))
            mdi->MSSQLDatabaseWriteTransactions.current.Data = (ULONGLONG)value;
        else if (!strncmp(
                     object_name, NETDATA_MSSQL_BACKUP_RESTORE_METRIC, sizeof(NETDATA_MSSQL_BACKUP_RESTORE_METRIC) - 1))
            mdi->MSSQLDatabaseBackupRestoreOperations.current.Data = (ULONGLONG)value;
        else if (!strncmp(object_name, NETDATA_MSSQL_LOG_FLUSHED_METRIC, sizeof(NETDATA_MSSQL_LOG_FLUSHED_METRIC) - 1))
            mdi->MSSQLDatabaseLogFlushed.current.Data = (ULONGLONG)value;
        else if (!strncmp(object_name, NETDATA_MSSQL_LOG_FLUSHES_METRIC, sizeof(NETDATA_MSSQL_LOG_FLUSHES_METRIC) - 1))
            mdi->MSSQLDatabaseLogFlushes.current.Data = (ULONGLONG)value;
        else if (!strncmp(
                     object_name,
                     NETDATA_MSSQL_NUMBER_DEADLOCKS_METRIC,
                     sizeof(NETDATA_MSSQL_NUMBER_DEADLOCKS_METRIC) - 1))
            mdi->MSSQLDatabaseDeadLockSec.current.Data = (ULONGLONG)value;
        else if (!strncmp(object_name, NETDATA_MSSQL_LOCK_WAITS_METRIC, sizeof(NETDATA_MSSQL_LOCK_WAITS_METRIC) - 1))
            mdi->MSSQLDatabaseLockWaitSec.current.Data = (ULONGLONG)value;
        else if (!strncmp(
                     object_name, NETDATA_MSSQL_LOCK_TIMEOUTS_METRIC, sizeof(NETDATA_MSSQL_LOCK_TIMEOUTS_METRIC) - 1))
            mdi->MSSQLDatabaseLockTimeoutsSec.current.Data = (ULONGLONG)value;
        else if (!strncmp(
                     object_name, NETDATA_MSSQL_LOCK_REQUESTS_METRIC, sizeof(NETDATA_MSSQL_LOCK_REQUESTS_METRIC) - 1))
            mdi->MSSQLDatabaseLockRequestsSec.current.Data = (ULONGLONG)value;

    } while (true);

endtransactions:
    netdata_MSSQL_release_results(mdi->parent->conn->dbTransactionSTMT);
}

void dict_mssql_fill_locks(struct mssql_db_instance *mdi, const char *dbname)
{
#define NETDATA_MSSQL_MAX_RESOURCE_TYPE (60)
    char resource_type[NETDATA_MSSQL_MAX_RESOURCE_TYPE + 1] = {};
    long value = 0;
    SQLLEN col_object_len = 0, col_value_len = 0;

    SQLCHAR query[sizeof(NETDATA_QUERY_LOCKS_MASK) + 2 * NETDATA_MAX_INSTANCE_OBJECT + 1];
    snprintfz(
        (char *)query,
        sizeof(NETDATA_QUERY_TRANSACTIONS_MASK) + 2 * NETDATA_MAX_INSTANCE_OBJECT,
        NETDATA_QUERY_LOCKS_MASK,
        dbname,
        dbname);

    SQLRETURN ret = SQLExecDirect(mdi->parent->conn->dbLocksSTMT, (SQLCHAR *)query, SQL_NTS);
    if (ret != SQL_SUCCESS) {
        mdi->collecting_data = false;
        netdata_MSSQL_error(
            SQL_HANDLE_STMT, mdi->parent->conn->dbLocksSTMT, NETDATA_MSSQL_ODBC_QUERY, mdi->parent->instanceID);
        goto endlocks;
    }

    ret = SQLBindCol(
        mdi->parent->conn->dbLocksSTMT, 1, SQL_C_CHAR, resource_type, sizeof(resource_type), &col_object_len);
    if (ret != SQL_SUCCESS) {
        netdata_MSSQL_error(
            SQL_HANDLE_STMT, mdi->parent->conn->dbLocksSTMT, NETDATA_MSSQL_ODBC_PREPARE, mdi->parent->instanceID);
        goto endlocks;
    }

    ret = SQLBindCol(mdi->parent->conn->dbLocksSTMT, 2, SQL_C_LONG, &value, sizeof(value), &col_value_len);
    if (ret != SQL_SUCCESS) {
        netdata_MSSQL_error(
            SQL_HANDLE_STMT, mdi->parent->conn->dbLocksSTMT, NETDATA_MSSQL_ODBC_PREPARE, mdi->parent->instanceID);
        goto endlocks;
    }

    do {
        ret = SQLFetch(mdi->parent->conn->dbLocksSTMT);
        switch (ret) {
            case SQL_SUCCESS:
            case SQL_SUCCESS_WITH_INFO:
                break;
            case SQL_NO_DATA:
            default:
                goto endlocks;
        }

        char *space = strchr(resource_type, ' ');
        if (space)
            *space = '\0';

        struct mssql_lock_instance *mli =
            dictionary_set(mdi->parent->locks_instances, resource_type, NULL, sizeof(*mli));
        if (!mli)
            continue;

        mli->lockWait.current.Data = value;
    } while (true);

endlocks:
    netdata_MSSQL_release_results(mdi->parent->conn->dbLocksSTMT);
}

int dict_mssql_fill_waits(struct mssql_instance *mi)
{
    char wait_type[NETDATA_MAX_INSTANCE_OBJECT + 1] = {};
    char wait_category[NETDATA_MAX_INSTANCE_OBJECT + 1] = {};
    SQLBIGINT total_wait = 0;
    SQLBIGINT resource_wait = 0;
    SQLBIGINT signal_wait = 0;
    SQLBIGINT max_wait = 0;
    SQLBIGINT waiting_tasks = 0;
    int success = 0;
    SQLLEN col_wait_type_len = 0, col_total_wait_len = 0, col_resource_wait_len = 0, col_signal_wait_len = 0,
           col_max_wait_len = 0, col_waiting_tasks_len = 0, col_wait_category_len = 0;

    SQLRETURN ret = SQLExecDirect(mi->conn->dbWaitsSTMT, (SQLCHAR *)NETDATA_QUERY_CHECK_WAITS, SQL_NTS);
    if (ret != SQL_SUCCESS) {
        netdata_MSSQL_error(SQL_HANDLE_STMT, mi->conn->dbWaitsSTMT, NETDATA_MSSQL_ODBC_QUERY, mi->instanceID);
        goto endwait;
    }

    ret = SQLBindCol(mi->conn->dbWaitsSTMT, 1, SQL_C_CHAR, wait_type, sizeof(wait_type), &col_wait_type_len);
    if (ret != SQL_SUCCESS) {
        netdata_MSSQL_error(SQL_HANDLE_STMT, mi->conn->dbWaitsSTMT, NETDATA_MSSQL_ODBC_PREPARE, mi->instanceID);
        goto endwait;
    }

    ret = SQLBindCol(mi->conn->dbWaitsSTMT, 2, SQL_C_LONG, &total_wait, sizeof(total_wait), &col_total_wait_len);
    if (ret != SQL_SUCCESS) {
        netdata_MSSQL_error(SQL_HANDLE_STMT, mi->conn->dbWaitsSTMT, NETDATA_MSSQL_ODBC_PREPARE, mi->instanceID);
        goto endwait;
    }

    ret =
        SQLBindCol(mi->conn->dbWaitsSTMT, 3, SQL_C_LONG, &resource_wait, sizeof(resource_wait), &col_resource_wait_len);
    if (ret != SQL_SUCCESS) {
        netdata_MSSQL_error(SQL_HANDLE_STMT, mi->conn->dbWaitsSTMT, NETDATA_MSSQL_ODBC_PREPARE, mi->instanceID);
        goto endwait;
    }

    ret = SQLBindCol(mi->conn->dbWaitsSTMT, 4, SQL_C_LONG, &signal_wait, sizeof(signal_wait), &col_signal_wait_len);
    if (ret != SQL_SUCCESS) {
        netdata_MSSQL_error(SQL_HANDLE_STMT, mi->conn->dbWaitsSTMT, NETDATA_MSSQL_ODBC_PREPARE, mi->instanceID);
        goto endwait;
    }

    ret = SQLBindCol(mi->conn->dbWaitsSTMT, 5, SQL_C_LONG, &max_wait, sizeof(max_wait), &col_max_wait_len);
    if (ret != SQL_SUCCESS) {
        netdata_MSSQL_error(SQL_HANDLE_STMT, mi->conn->dbWaitsSTMT, NETDATA_MSSQL_ODBC_PREPARE, mi->instanceID);
        goto endwait;
    }

    ret =
        SQLBindCol(mi->conn->dbWaitsSTMT, 6, SQL_C_LONG, &waiting_tasks, sizeof(waiting_tasks), &col_waiting_tasks_len);
    if (ret != SQL_SUCCESS) {
        netdata_MSSQL_error(SQL_HANDLE_STMT, mi->conn->dbWaitsSTMT, NETDATA_MSSQL_ODBC_PREPARE, mi->instanceID);
        goto endwait;
    }

    ret =
        SQLBindCol(mi->conn->dbWaitsSTMT, 7, SQL_C_CHAR, wait_category, sizeof(wait_category), &col_wait_category_len);
    if (ret != SQL_SUCCESS) {
        netdata_MSSQL_error(SQL_HANDLE_STMT, mi->conn->dbWaitsSTMT, NETDATA_MSSQL_ODBC_PREPARE, mi->instanceID);
        goto endwait;
    }

    do {
        ret = SQLFetch(mi->conn->dbWaitsSTMT);
        switch (ret) {
            case SQL_SUCCESS:
            case SQL_SUCCESS_WITH_INFO:
                break;
            case SQL_NO_DATA:
            default:
                success = 1;
                goto endwait;
        }

        struct mssql_db_waits *mdw = dictionary_set(mi->waits, wait_type, NULL, sizeof(*mdw));
        if (!mdw)
            continue;

        mdw->MSSQLDatabaseTotalWait.current.Data = (ULONGLONG)total_wait;
        // Variable mdw->MSSQLDatabaseResourceWaitMSec.current.Data stores a mathematical operation
        // that can be negative sometimes. This is the reason we have this if
        if (resource_wait > mdw->MSSQLDatabaseResourceWaitMSec.current.Data)
            mdw->MSSQLDatabaseResourceWaitMSec.current.Data = (ULONGLONG)resource_wait;
        mdw->MSSQLDatabaseSignalWaitMSec.current.Data = (ULONGLONG)signal_wait;
        mdw->MSSQLDatabaseMaxWaitTimeMSec.current.Data = (ULONGLONG)max_wait;
        mdw->MSSQLDatabaseWaitingTasks.current.Data = (ULONGLONG)waiting_tasks;

        if (!mdw->wait_category)
            mdw->wait_category = strdupz(wait_category);
    } while (true);

endwait:
    netdata_MSSQL_release_results(mi->conn->dbWaitsSTMT);

    return success;
}

int dict_mssql_databases_run_queries(const DICTIONARY_ITEM *item __maybe_unused, void *value, void *data __maybe_unused)
{
    struct mssql_db_instance *mdi = value;
    const char *dbname = dictionary_acquired_item_name((DICTIONARY_ITEM *)item);

    if (!mdi->collecting_data || !mdi->parent || !mdi->parent->conn) {
        goto enddrunquery;
    }

    // We failed to collect this for the database, so we are not going to try again
    if (mdi->MSSQLDatabaseDataFileSize.current.Data != ULONG_LONG_MAX)
        mdi->MSSQLDatabaseDataFileSize.current.Data = netdata_MSSQL_fill_long_value(
            mdi->parent->conn->dataFileSizeSTMT, NETDATA_QUERY_DATA_FILE_SIZE_MASK, dbname, mdi->parent->instanceID);
    else {
        mdi->collecting_data = false;
        goto enddrunquery;
    }

    dict_mssql_fill_transactions(mdi, dbname);
    dict_mssql_fill_locks(mdi, dbname);

enddrunquery:
    return 1;
}

long metdata_mssql_check_permission(struct mssql_instance *mi)
{
    static int next_try = NETDATA_MSSQL_NEXT_TRY - 1;
    long perm = 0;
    SQLLEN col_data_len = 0;

    if (++next_try != NETDATA_MSSQL_NEXT_TRY)
        return 1;

    next_try = 0;

    SQLRETURN ret;

    ret = SQLExecDirect(mi->conn->checkPermSTMT, (SQLCHAR *)NETDATA_QUERY_CHECK_PERM, SQL_NTS);
    if (ret != SQL_SUCCESS) {
        netdata_MSSQL_error(SQL_HANDLE_STMT, mi->conn->checkPermSTMT, NETDATA_MSSQL_ODBC_QUERY, mi->instanceID);
        perm = LONG_MAX;
        goto endperm;
    }

    ret = SQLBindCol(mi->conn->checkPermSTMT, 1, SQL_C_LONG, &perm, sizeof(perm), &col_data_len);

    if (ret != SQL_SUCCESS) {
        netdata_MSSQL_error(SQL_HANDLE_STMT, mi->conn->checkPermSTMT, NETDATA_MSSQL_ODBC_PREPARE, mi->instanceID);
        perm = LONG_MAX;
        goto endperm;
    }

    ret = SQLFetch(mi->conn->checkPermSTMT);
    if (ret != SQL_SUCCESS) {
        netdata_MSSQL_error(SQL_HANDLE_STMT, mi->conn->checkPermSTMT, NETDATA_MSSQL_ODBC_FETCH, mi->instanceID);
        perm = LONG_MAX;
        goto endperm;
    }

endperm:
    netdata_MSSQL_release_results(mi->conn->checkPermSTMT);
    return perm;
}

void metdata_mssql_fill_mssql_status(struct mssql_instance *mi)
{
    char dbname[SQLSERVER_MAX_NAME_LENGTH + 1];
    int readonly = 0;
    BYTE state = 0;
    SQLLEN col_data_len = 0;

    static int next_try = NETDATA_MSSQL_NEXT_TRY - 1;

    if (++next_try != NETDATA_MSSQL_NEXT_TRY)
        return;

    next_try = 0;

    SQLRETURN ret;

    ret = SQLExecDirect(mi->conn->dbSQLState, (SQLCHAR *)NETDATA_QUERY_DATABASE_STATUS, SQL_NTS);
    if (ret != SQL_SUCCESS) {
        netdata_MSSQL_error(SQL_HANDLE_STMT, mi->conn->dbSQLState, NETDATA_MSSQL_ODBC_QUERY, mi->instanceID);
        goto enddbstate;
    }

    ret = SQLBindCol(mi->conn->dbSQLState, 1, SQL_C_TINYINT, &state, sizeof(state), &col_data_len);
    if (ret != SQL_SUCCESS) {
        netdata_MSSQL_error(SQL_HANDLE_STMT, mi->conn->dbSQLState, NETDATA_MSSQL_ODBC_PREPARE, mi->instanceID);
        goto enddbstate;
    }

    ret = SQLBindCol(mi->conn->dbSQLState, 3, SQL_C_BIT, &readonly, sizeof(readonly), &col_data_len);
    if (ret != SQL_SUCCESS) {
        netdata_MSSQL_error(SQL_HANDLE_STMT, mi->conn->dbWaitsSTMT, NETDATA_MSSQL_ODBC_PREPARE, mi->instanceID);
        goto enddbstate;
    }

    int i = 0;
    do {
        ret = SQLFetch(mi->conn->dbSQLState);
        if (ret != SQL_SUCCESS && ret != SQL_SUCCESS_WITH_INFO) {
            goto enddbstate;
        }

        struct mssql_db_instance *mdi = dictionary_set(mi->databases, dbname, NULL, sizeof(*mdi));
        if (!mdi)
            continue;

        mdi->MSSQLDBState.current.Data = (ULONGLONG)state;
        mdi->MSSQLDBIsReadonly.current.Data = (ULONGLONG)readonly;
    } while (true);

enddbstate:
    netdata_MSSQL_release_results(mi->conn->dbSQLState);
}

void metdata_mssql_fill_dictionary_from_db(struct mssql_instance *mi)
{
    char dbname[SQLSERVER_MAX_NAME_LENGTH + 1];
    SQLLEN col_data_len = 0;

    static int next_try = NETDATA_MSSQL_NEXT_TRY - 1;

    if (++next_try != NETDATA_MSSQL_NEXT_TRY)
        return;

    next_try = 0;

    SQLRETURN ret;

    ret = SQLExecDirect(mi->conn->databaseListSTMT, (SQLCHAR *)NETDATA_QUERY_LIST_DB, SQL_NTS);
    if (ret != SQL_SUCCESS) {
        netdata_MSSQL_error(SQL_HANDLE_STMT, mi->conn->databaseListSTMT, NETDATA_MSSQL_ODBC_QUERY, mi->instanceID);
        goto enddblist;
    }

    ret = SQLBindCol(mi->conn->databaseListSTMT, 1, SQL_C_CHAR, dbname, sizeof(dbname), &col_data_len);

    if (ret != SQL_SUCCESS) {
        netdata_MSSQL_error(SQL_HANDLE_STMT, mi->conn->databaseListSTMT, NETDATA_MSSQL_ODBC_PREPARE, mi->instanceID);
        goto enddblist;
    }

    int i = 0;
    do {
        ret = SQLFetch(mi->conn->databaseListSTMT);
        if (ret != SQL_SUCCESS && ret != SQL_SUCCESS_WITH_INFO) {
            goto enddblist;
        }

        struct mssql_db_instance *mdi = dictionary_set(mi->databases, dbname, NULL, sizeof(*mdi));
        if (!mdi)
            continue;

        mdi->updated = 0;
        if (!mdi->parent) {
            mdi->parent = mi;
        }

        if (!i) {
            mdi->collect_instance = true;
        }
    } while (true);

enddblist:
    netdata_MSSQL_release_results(mi->conn->databaseListSTMT);
}

static bool netdata_MSSQL_initialize_conection(struct netdata_mssql_conn *nmc)
{
    SQLRETURN ret;
    if (nmc->netdataSQLEnv == NULL) {
        ret = SQLAllocHandle(SQL_HANDLE_ENV, SQL_NULL_HANDLE, &nmc->netdataSQLEnv);
        if (ret != SQL_SUCCESS && ret != SQL_SUCCESS_WITH_INFO)
            return FALSE;

        ret = SQLSetEnvAttr(nmc->netdataSQLEnv, SQL_ATTR_ODBC_VERSION, (SQLPOINTER)SQL_OV_ODBC3, 0);
        if (ret != SQL_SUCCESS && ret != SQL_SUCCESS_WITH_INFO) {
            return FALSE;
        }
    }

    ret = SQLAllocHandle(SQL_HANDLE_DBC, nmc->netdataSQLEnv, &nmc->netdataSQLHDBc);
    if (ret != SQL_SUCCESS && ret != SQL_SUCCESS_WITH_INFO) {
        return FALSE;
    }

    ret = SQLSetConnectAttr(nmc->netdataSQLHDBc, SQL_LOGIN_TIMEOUT, (SQLPOINTER)5, 0);
    if (ret != SQL_SUCCESS && ret != SQL_SUCCESS_WITH_INFO) {
        return FALSE;
    }

    ret = SQLSetConnectAttr(nmc->netdataSQLHDBc, SQL_ATTR_AUTOCOMMIT, (SQLPOINTER)TRUE, 0);
    if (ret != SQL_SUCCESS && ret != SQL_SUCCESS_WITH_INFO) {
        return FALSE;
    }

    SQLCHAR ret_conn_str[1024];
    ret = SQLDriverConnect(
        nmc->netdataSQLHDBc, NULL, nmc->connectionString, SQL_NTS, ret_conn_str, 1024, NULL, SQL_DRIVER_NOPROMPT);

    BOOL retConn;
    switch (ret) {
        case SQL_NO_DATA_FOUND:
        case SQL_INVALID_HANDLE:
        case SQL_ERROR:
        default:
            netdata_MSSQL_error(SQL_HANDLE_DBC, nmc->netdataSQLHDBc, NETDATA_MSSQL_ODBC_CONNECT, NULL);
            retConn = FALSE;
            break;
        case SQL_SUCCESS:
        case SQL_SUCCESS_WITH_INFO:
            retConn = TRUE;
            break;
    }

    if (retConn) {
        ret = SQLAllocHandle(SQL_HANDLE_STMT, nmc->netdataSQLHDBc, &nmc->checkPermSTMT);
        if (ret != SQL_SUCCESS && ret != SQL_SUCCESS_WITH_INFO)
            retConn = FALSE;

        ret = SQLAllocHandle(SQL_HANDLE_STMT, nmc->netdataSQLHDBc, &nmc->databaseListSTMT);
        if (ret != SQL_SUCCESS && ret != SQL_SUCCESS_WITH_INFO)
            retConn = FALSE;

        ret = SQLAllocHandle(SQL_HANDLE_STMT, nmc->netdataSQLHDBc, &nmc->dataFileSizeSTMT);
        if (ret != SQL_SUCCESS && ret != SQL_SUCCESS_WITH_INFO)
            retConn = FALSE;

        ret = SQLAllocHandle(SQL_HANDLE_STMT, nmc->netdataSQLHDBc, &nmc->dbTransactionSTMT);
        if (ret != SQL_SUCCESS && ret != SQL_SUCCESS_WITH_INFO)
            retConn = FALSE;

        ret = SQLAllocHandle(SQL_HANDLE_STMT, nmc->netdataSQLHDBc, &nmc->dbInstanceTransactionSTMT);
        if (ret != SQL_SUCCESS && ret != SQL_SUCCESS_WITH_INFO)
            retConn = FALSE;

        ret = SQLAllocHandle(SQL_HANDLE_STMT, nmc->netdataSQLHDBc, &nmc->dbLocksSTMT);
        if (ret != SQL_SUCCESS && ret != SQL_SUCCESS_WITH_INFO)
            retConn = FALSE;

        ret = SQLAllocHandle(SQL_HANDLE_STMT, nmc->netdataSQLHDBc, &nmc->dbWaitsSTMT);
        if (ret != SQL_SUCCESS && ret != SQL_SUCCESS_WITH_INFO)
            retConn = FALSE;

        ret = SQLAllocHandle(SQL_HANDLE_STMT, nmc->netdataSQLHDBc, &nmc->dbSQLState);
        if (ret != SQL_SUCCESS && ret != SQL_SUCCESS_WITH_INFO)
            retConn = FALSE;
    }

    return retConn;
}

// Dictionary
static DICTIONARY *mssql_instances = NULL;

static void initialize_mssql_objects(struct mssql_instance *mi, const char *instance)
{
    char prefix[NETDATA_MAX_INSTANCE_NAME];
    if (!strcmp(instance, "MSSQLSERVER")) {
        strncpyz(prefix, "SQLServer:", sizeof(prefix) - 1);
    } else if (!strcmp(instance, "SQLEXPRESS")) {
        strncpyz(prefix, "MSSQL$SQLEXPRESS:", sizeof(prefix) - 1);
        if (mi->conn)
            mi->conn->is_sqlexpress = true;
    } else {
        char *express = (mi->conn && mi->conn->is_sqlexpress) ? "SQLEXPRESS:": "";
        snprintfz(prefix, sizeof(prefix) - 1, "MSSQL$%s%s:", express, instance);
    }

    size_t length = strlen(prefix);
    char name[NETDATA_MAX_INSTANCE_OBJECT];
    snprintfz(name, sizeof(name) - 1, "%s%s", prefix, "General Statistics");
    mi->objectName[NETDATA_MSSQL_GENERAL_STATS] = strdupz(name);

    strncpyz(&name[length], "SQL Errors", sizeof(name) - length);
    mi->objectName[NETDATA_MSSQL_SQL_ERRORS] = strdupz(name);

    strncpyz(&name[length], "Databases", sizeof(name) - length);
    mi->objectName[NETDATA_MSSQL_DATABASE] = strdupz(name);

    strncpyz(&name[length], "SQL Statistics", sizeof(name) - length);
    mi->objectName[NETDATA_MSSQL_SQL_STATS] = strdupz(name);

    strncpyz(&name[length], "Buffer Manager", sizeof(name) - length);
    mi->objectName[NETDATA_MSSQL_BUFFER_MANAGEMENT] = strdupz(name);

    strncpyz(&name[length], "Memory Manager", sizeof(name) - length);
    mi->objectName[NETDATA_MSSQL_MEMORY] = strdupz(name);

    strncpyz(&name[length], "Locks", sizeof(name) - length);
    mi->objectName[NETDATA_MSSQL_LOCKS] = strdupz(name);

    strncpyz(&name[length], "Wait Statistics", sizeof(name) - length);
    mi->objectName[NETDATA_MSSQL_WAITS] = strdupz(name);

    strncpyz(&name[length], "Access Methods", sizeof(name) - length);
    mi->objectName[NETDATA_MSSQL_ACCESS_METHODS] = strdupz(name);

    mi->instanceID = strdupz(instance);
}

static inline void initialize_mssql_keys(struct mssql_instance *mi)
{
    // General Statistics (https://learn.microsoft.com/en-us/sql/relational-databases/performance-monitor/sql-server-general-statistics-object)
    mi->MSSQLUserConnections.key = "User Connections";
    mi->MSSQLBlockedProcesses.key = "Processes blocked";

    // SQL Statistics (https://learn.microsoft.com/en-us/sql/relational-databases/performance-monitor/sql-server-sql-statistics-object)
    mi->MSSQLStatsAutoParameterization.key = "Auto-Param Attempts/sec";
    mi->MSSQLStatsBatchRequests.key = "Batch Requests/sec";
    mi->MSSQLStatSafeAutoParameterization.key = "Safe Auto-Params/sec";

    // Access Methods (https://learn.microsoft.com/en-us/sql/relational-databases/performance-monitor/sql-server-access-methods-object)
    mi->MSSQLAccessMethodPageSplits.key = "Page Splits/sec";

    // Errors (https://learn.microsoft.com/en-us/sql/relational-databases/performance-monitor/sql-server-sql-errors-object)
    mi->MSSQLSQLErrorsTotal.key = "Errors/sec";

    // Memory Management (https://learn.microsoft.com/en-us/sql/relational-databases/performance-monitor/sql-server-memory-manager-object)
    mi->MSSQLConnectionMemoryBytes.key = "Connection Memory (KB)";
    mi->MSSQLExternalBenefitOfMemory.key = "External benefit of memory";
    mi->MSSQLPendingMemoryGrants.key = "Memory Grants Pending";
    mi->MSSQLTotalServerMemory.key = "Total Server Memory (KB)";
}

void dict_mssql_insert_locks_cb(const DICTIONARY_ITEM *item __maybe_unused, void *value, void *data __maybe_unused)
{
    const char *resource = dictionary_acquired_item_name((DICTIONARY_ITEM *)item);

    // https://learn.microsoft.com/en-us/sql/relational-databases/performance-monitor/sql-server-locks-object
    struct mssql_lock_instance *ptr = value;
    ptr->resourceID = strdupz(resource);
    ptr->deadLocks.key = "Number of Deadlocks/sec";
    ptr->lockWait.key = "Lock Waits/sec";
}

void dict_mssql_insert_wait_cb(const DICTIONARY_ITEM *item __maybe_unused, void *value, void *data __maybe_unused)
{
    const char *type = dictionary_acquired_item_name((DICTIONARY_ITEM *)item);

    struct mssql_db_waits *mdw = value;

    mdw->wait_type = strdupz(type);
    mdw->wait_category = NULL;
    mdw->rd_total_wait = mdw->rd_max_wait_time_msec = mdw->rd_resource_wait_msec = mdw->rd_signal_wait_msec =
        mdw->rd_waiting_tasks = NULL;
}

void dict_mssql_insert_databases_cb(const DICTIONARY_ITEM *item __maybe_unused, void *value, void *data __maybe_unused)
{
    struct mssql_db_instance *mdi = value;

    mdi->collecting_data = true;
}

// Options
void netdata_mount_mssql_connection_string(struct netdata_mssql_conn *dbInput)
{
    SQLCHAR conn[1024];
    const char *serverAddress;
    const char *serverAddressArg;
    if (!dbInput) {
        return;
    }

    char auth[512];
    if (dbInput->server && dbInput->address) {
        nd_log(
            NDLS_COLLECTORS,
            NDLP_ERR,
            "Collector is not expecting server and address defined together, please, select one of them.");
        dbInput->connectionString = NULL;
        return;
    }

    if (dbInput->server) {
        serverAddress = "Server";
        serverAddressArg = dbInput->server;
    } else {
        serverAddressArg = "Address";
        serverAddress = dbInput->address;
    }

    if (dbInput->windows_auth)
        snprintfz(auth, sizeof(auth) - 1, "Trusted_Connection = yes");
    else if (!dbInput->username || !dbInput->password) {
        nd_log(
            NDLS_COLLECTORS,
            NDLP_ERR,
            "You are not using Windows Authentication. Thus, it is necessary to specify user and password.");
        dbInput->connectionString = NULL;
        return;
    } else {
        snprintfz(auth, sizeof(auth) - 1, "UID=%s;PWD=%s;", dbInput->username, dbInput->password);
    }

    snprintfz(
        (char *)conn, sizeof(conn) - 1, "Driver={%s};%s=%s;%s", dbInput->driver, serverAddress, serverAddressArg, auth);
    dbInput->connectionString = (SQLCHAR *)strdupz((char *)conn);
}

static void netdata_read_config_options()
{
#define NETDATA_MAX_MSSSQL_SECTION_LENGTH (40)
#define NETDATA_DEFAULT_MSSQL_SECTION "plugin:windows:PerflibMSSQL"
    uint16_t expected_instances = 1;
    uint16_t total_instances = 0;
    for (; total_instances < expected_instances; total_instances++) {
        char section_name[NETDATA_MAX_MSSSQL_SECTION_LENGTH + 1];
        char upper_instance[NETDATA_MAX_INSTANCE_OBJECT + 1];
        strncpyz(section_name, NETDATA_DEFAULT_MSSQL_SECTION, sizeof(NETDATA_DEFAULT_MSSQL_SECTION));
        if (total_instances) {
            snprintfz(&section_name[sizeof(NETDATA_DEFAULT_MSSQL_SECTION) - 1], 5, "%d", total_instances);
        }
        const char *instance = inicfg_get(&netdata_config, section_name, "instance", NULL);
        int additional_instances = (int)inicfg_get_number(&netdata_config, section_name, "additional instances", 0);
        if (!instance || strlen(instance) > NETDATA_MAX_INSTANCE_OBJECT) {
            nd_log(NDLS_COLLECTORS, NDLP_ERR, "You must specify a valid 'instance' name to collect data from database in section %s.", section_name);
            continue;
        }

        if (!total_instances && additional_instances) {
            if (additional_instances > 64) {
                nd_log(NDLS_COLLECTORS, NDLP_ERR, "Number of instances is bigger than expected (64)");
                expected_instances = 64;
            }
            expected_instances = additional_instances + 1;
        }

        const char *move = instance;
        int i;
        for (i = 0; *move; move++, i++) {
            upper_instance[i] = toupper(*move);
        }
        upper_instance[i] = '\0';

        struct netdata_mssql_conn *dbconn = dictionary_set(conn_options, upper_instance, NULL, sizeof(*dbconn));

        dbconn->instance = strdupz(upper_instance);
        dbconn->driver = inicfg_get(&netdata_config, section_name, "driver", "SQL Server");
        dbconn->server = inicfg_get(&netdata_config, section_name, "server", NULL);
        dbconn->address = inicfg_get(&netdata_config, section_name, "address", NULL);
        dbconn->username = inicfg_get(&netdata_config, section_name, "uid", NULL);
        dbconn->password = inicfg_get(&netdata_config, section_name, "pwd", NULL);
        dbconn->instances = additional_instances;
        dbconn->windows_auth = inicfg_get_boolean(&netdata_config, section_name, "windows authentication", false);
        dbconn->is_sqlexpress = inicfg_get_boolean(&netdata_config, section_name, "express", false);
        dbconn->is_connected = FALSE;

        netdata_mount_mssql_connection_string(dbconn);
    }
}

static inline struct netdata_mssql_conn *netdata_mssql_get_conn_option(const char *instance)
{
    return (struct netdata_mssql_conn *)dictionary_get(conn_options, instance);
}

void mssql_fill_initial_instances(struct mssql_instance *mi)
{
    // https://learn.microsoft.com/en-us/sql/relational-databases/performance-monitor/sql-server-locks-object?view=sql-server-ver17
    char *keys[] = {
        "AllocUnit",
        "Application",
        "Database",
        "Extent",
        "File",
        "HoBT",
        "Key",
        "Metadata",
        "OIB",
        "Object",
        "Page",
        "RID",
        "RowGroup",
        "Xact",
        NULL};
    for (int i = 0; keys[i]; i++) {
        (void)dictionary_set(mi->locks_instances, keys[i], NULL, sizeof(struct mssql_lock_instance));
    }
}

void dict_mssql_insert_cb(const DICTIONARY_ITEM *item __maybe_unused, void *value, void *data __maybe_unused)
{
    struct mssql_instance *mi = value;
    const char *instance = dictionary_acquired_item_name((DICTIONARY_ITEM *)item);
    bool *create_thread = data;

    if (!mi->locks_instances) {
        mi->locks_instances = dictionary_create_advanced(
            DICT_OPTION_DONT_OVERWRITE_VALUE | DICT_OPTION_FIXED_SIZE, NULL, sizeof(struct mssql_lock_instance));
        dictionary_register_insert_callback(mi->locks_instances, dict_mssql_insert_locks_cb, NULL);
        mssql_fill_initial_instances(mi);
    }

    if (!mi->databases) {
        mi->databases = dictionary_create_advanced(
            DICT_OPTION_DONT_OVERWRITE_VALUE | DICT_OPTION_FIXED_SIZE, NULL, sizeof(struct mssql_db_instance));
        dictionary_register_insert_callback(mi->databases, dict_mssql_insert_databases_cb, NULL);
    }

    if (!mi->waits) {
        mi->waits = dictionary_create_advanced(
            DICT_OPTION_DONT_OVERWRITE_VALUE | DICT_OPTION_FIXED_SIZE, NULL, sizeof(struct mssql_db_waits));
        dictionary_register_insert_callback(mi->waits, dict_mssql_insert_wait_cb, NULL);
    }

    initialize_mssql_objects(mi, instance);
    initialize_mssql_keys(mi);
    mi->conn = netdata_mssql_get_conn_option(instance);

    if (mi->conn && mi->conn->connectionString) {
        mi->conn->is_connected = netdata_MSSQL_initialize_conection(mi->conn);
        if (mi->conn->is_connected)
            *create_thread = true;
    }
}

void dict_mssql_insert_conn_option(const DICTIONARY_ITEM *item __maybe_unused, void *value, void *data __maybe_unused)
{
    ;
}

static void mssql_fill_dictionary(int update_every)
{
    HKEY hKey;
    LSTATUS ret = RegOpenKeyExA(
        HKEY_LOCAL_MACHINE, "SOFTWARE\\Microsoft\\Microsoft SQL Server\\Instance Names\\SQL", 0, KEY_READ, &hKey);
    if (ret != ERROR_SUCCESS)
        return;

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

        struct mssql_instance *p = dictionary_set(mssql_instances, avalue, NULL, sizeof(*p));
        p->update_every = update_every;
    }

endMSSQLFillDict:
    RegCloseKey(hKey);
}

int netdata_mssql_reset_value(const DICTIONARY_ITEM *item __maybe_unused, void *value, void *data __maybe_unused)
{
    struct mssql_db_instance *mdi = value;

    mdi->collecting_data = false;

    return 1;
}

int dict_mssql_query_cb(const DICTIONARY_ITEM *item __maybe_unused, void *value, void *data __maybe_unused)
{
    struct mssql_instance *mi = value;
    static long collecting = 1;

    if (mi->conn && mi->conn->is_connected && collecting) {
        collecting = metdata_mssql_check_permission(mi);
        if (!collecting) {
            nd_log(
                NDLS_COLLECTORS,
                NDLP_ERR,
                "User %s does not have permission to run queries on %s",
                mi->conn->username,
                mi->instanceID);
        } else {
            metdata_mssql_fill_dictionary_from_db(mi);
            metdata_mssql_fill_mssql_status(mi);
            dictionary_sorted_walkthrough_read(mi->databases, dict_mssql_databases_run_queries, NULL);
        }

        collecting = dict_mssql_fill_waits(mi);
    } else {
        dictionary_sorted_walkthrough_read(mi->databases, netdata_mssql_reset_value, NULL);
    }

    return 1;
}

static void netdata_mssql_queries(void *ptr __maybe_unused)
{
    heartbeat_t hb;
    heartbeat_init(&hb, USEC_PER_SEC);
    int update_every = UPDATE_EVERY_MIN;

    while (service_running(SERVICE_COLLECTORS)) {
        (void)heartbeat_next(&hb);

        if (unlikely(!service_running(SERVICE_COLLECTORS)))
            break;

        dictionary_sorted_walkthrough_read(mssql_instances, dict_mssql_query_cb, &update_every);
    }
}

static ND_THREAD *mssql_queries_thread = NULL;

static int initialize(int update_every)
{
    static bool create_thread = false;
    mssql_instances = dictionary_create_advanced(
        DICT_OPTION_DONT_OVERWRITE_VALUE | DICT_OPTION_FIXED_SIZE, NULL, sizeof(struct mssql_instance));

    dictionary_register_insert_callback(mssql_instances, dict_mssql_insert_cb, &create_thread);

    conn_options = dictionary_create_advanced(
        DICT_OPTION_DONT_OVERWRITE_VALUE | DICT_OPTION_FIXED_SIZE, NULL, sizeof(struct netdata_mssql_conn));
    dictionary_register_insert_callback(conn_options, dict_mssql_insert_conn_option, NULL);

    netdata_read_config_options();
    mssql_fill_dictionary(update_every);

    if (create_thread)
        mssql_queries_thread = nd_thread_create("mssql_queries", NETDATA_THREAD_OPTION_DEFAULT, netdata_mssql_queries, &update_every);

    return 0;
}

static void do_mssql_general_stats(PERF_DATA_BLOCK *pDataBlock, struct mssql_instance *mi, int update_every)
{
    char id[RRD_ID_LENGTH_MAX + 1];

    PERF_OBJECT_TYPE *pObjectType =
        perflibFindObjectTypeByName(pDataBlock, mi->objectName[NETDATA_MSSQL_GENERAL_STATS]);
    if (!pObjectType)
        return;

    if (perflibGetObjectCounter(pDataBlock, pObjectType, &mi->MSSQLUserConnections)) {
        if (!mi->st_user_connections) {
            snprintfz(id, RRD_ID_LENGTH_MAX, "instance_%s_user_connections", mi->instanceID);
            netdata_fix_chart_name(id);
            mi->st_user_connections = rrdset_create_localhost(
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

            mi->rd_user_connections = rrddim_add(mi->st_user_connections, "user", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);

            rrdlabels_add(mi->st_user_connections->rrdlabels, "mssql_instance", mi->instanceID, RRDLABEL_SRC_AUTO);
        }

        rrddim_set_by_pointer(
            mi->st_user_connections, mi->rd_user_connections, (collected_number)mi->MSSQLUserConnections.current.Data);
        rrdset_done(mi->st_user_connections);
    }

    if (perflibGetObjectCounter(pDataBlock, pObjectType, &mi->MSSQLBlockedProcesses)) {
        if (!mi->st_process_blocked) {
            snprintfz(id, RRD_ID_LENGTH_MAX, "instance_%s_blocked_process", mi->instanceID);
            netdata_fix_chart_name(id);
            mi->st_process_blocked = rrdset_create_localhost(
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

            mi->rd_process_blocked = rrddim_add(mi->st_process_blocked, "blocked", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);

            rrdlabels_add(mi->st_process_blocked->rrdlabels, "mssql_instance", mi->instanceID, RRDLABEL_SRC_AUTO);
        }

        rrddim_set_by_pointer(
            mi->st_process_blocked, mi->rd_process_blocked, (collected_number)mi->MSSQLBlockedProcesses.current.Data);
        rrdset_done(mi->st_process_blocked);
    }
}

static void do_mssql_statistics_perflib(PERF_DATA_BLOCK *pDataBlock, struct mssql_instance *mi, int update_every)
{
    char id[RRD_ID_LENGTH_MAX + 1];

    PERF_OBJECT_TYPE *pObjectType = perflibFindObjectTypeByName(pDataBlock, mi->objectName[NETDATA_MSSQL_SQL_STATS]);
    if (!pObjectType)
        return;

    if (perflibGetObjectCounter(pDataBlock, pObjectType, &mi->MSSQLStatsAutoParameterization)) {
        if (!mi->st_stats_auto_param) {
            snprintfz(id, RRD_ID_LENGTH_MAX, "instance_%s_sqlstats_auto_parameterization_attempts", mi->instanceID);
            netdata_fix_chart_name(id);
            mi->st_stats_auto_param = rrdset_create_localhost(
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

            mi->rd_stats_auto_param =
                rrddim_add(mi->st_stats_auto_param, "failed", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);

            rrdlabels_add(mi->st_stats_auto_param->rrdlabels, "mssql_instance", mi->instanceID, RRDLABEL_SRC_AUTO);
        }

        rrddim_set_by_pointer(
            mi->st_stats_auto_param,
            mi->rd_stats_auto_param,
            (collected_number)mi->MSSQLStatsAutoParameterization.current.Data);
        rrdset_done(mi->st_stats_auto_param);
    }

    if (perflibGetObjectCounter(pDataBlock, pObjectType, &mi->MSSQLStatsBatchRequests)) {
        if (!mi->st_stats_batch_request) {
            snprintfz(id, RRD_ID_LENGTH_MAX, "instance_%s_sqlstats_batch_requests", mi->instanceID);
            netdata_fix_chart_name(id);
            mi->st_stats_batch_request = rrdset_create_localhost(
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

            mi->rd_stats_batch_request =
                rrddim_add(mi->st_stats_batch_request, "batch", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);

            rrdlabels_add(mi->st_stats_batch_request->rrdlabels, "mssql_instance", mi->instanceID, RRDLABEL_SRC_AUTO);
        }

        rrddim_set_by_pointer(
            mi->st_stats_batch_request,
            mi->rd_stats_batch_request,
            (collected_number)mi->MSSQLStatsBatchRequests.current.Data);
        rrdset_done(mi->st_stats_batch_request);
    }

    if (perflibGetObjectCounter(pDataBlock, pObjectType, &mi->MSSQLStatSafeAutoParameterization)) {
        if (!mi->st_stats_safe_auto) {
            snprintfz(
                id, RRD_ID_LENGTH_MAX, "instance_%s_sqlstats_safe_auto_parameterization_attempts", mi->instanceID);
            netdata_fix_chart_name(id);
            mi->st_stats_safe_auto = rrdset_create_localhost(
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

            mi->rd_stats_safe_auto = rrddim_add(mi->st_stats_safe_auto, "safe", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);

            rrdlabels_add(mi->st_stats_safe_auto->rrdlabels, "mssql_instance", mi->instanceID, RRDLABEL_SRC_AUTO);
        }

        rrddim_set_by_pointer(
            mi->st_stats_safe_auto,
            mi->rd_stats_safe_auto,
            (collected_number)mi->MSSQLStatSafeAutoParameterization.current.Data);
        rrdset_done(mi->st_stats_safe_auto);
    }
}

static void do_mssql_access_methods(PERF_DATA_BLOCK *pDataBlock, struct mssql_instance *mi, int update_every)
{
    char id[RRD_ID_LENGTH_MAX + 1];

    PERF_OBJECT_TYPE *pObjectType =
        perflibFindObjectTypeByName(pDataBlock, mi->objectName[NETDATA_MSSQL_ACCESS_METHODS]);
    if (!pObjectType)
        return;

    if (perflibGetObjectCounter(pDataBlock, pObjectType, &mi->MSSQLAccessMethodPageSplits)) {
        if (!mi->st_access_method_page_splits) {
            snprintfz(id, RRD_ID_LENGTH_MAX, "instance_%s_accessmethods_page_splits", mi->instanceID);
            netdata_fix_chart_name(id);
            mi->st_access_method_page_splits = rrdset_create_localhost(
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

            mi->rd_access_method_page_splits =
                rrddim_add(mi->st_access_method_page_splits, "page", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);

            rrdlabels_add(
                mi->st_access_method_page_splits->rrdlabels, "mssql_instance", mi->instanceID, RRDLABEL_SRC_AUTO);
        }

        rrddim_set_by_pointer(
            mi->st_access_method_page_splits,
            mi->rd_access_method_page_splits,
            (collected_number)mi->MSSQLAccessMethodPageSplits.current.Data);
        rrdset_done(mi->st_access_method_page_splits);
    }
}

static void do_mssql_errors(PERF_DATA_BLOCK *pDataBlock, struct mssql_instance *mi, int update_every)
{
    char id[RRD_ID_LENGTH_MAX + 1];

    PERF_OBJECT_TYPE *pObjectType = perflibFindObjectTypeByName(pDataBlock, mi->objectName[NETDATA_MSSQL_SQL_ERRORS]);
    if (!pObjectType)
        return;

    if (perflibGetObjectCounter(pDataBlock, pObjectType, &mi->MSSQLSQLErrorsTotal)) {
        if (!mi->st_sql_errors) {
            snprintfz(id, RRD_ID_LENGTH_MAX, "instance_%s_sql_errors_total", mi->instanceID);
            netdata_fix_chart_name(id);
            mi->st_sql_errors = rrdset_create_localhost(
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

            mi->rd_sql_errors = rrddim_add(mi->st_sql_errors, "errors", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);

            rrdlabels_add(mi->st_sql_errors->rrdlabels, "mssql_instance", mi->instanceID, RRDLABEL_SRC_AUTO);
        }

        rrddim_set_by_pointer(
            mi->st_sql_errors, mi->rd_sql_errors, (collected_number)mi->MSSQLAccessMethodPageSplits.current.Data);
        rrdset_done(mi->st_sql_errors);
    }
}

void dict_mssql_locks_wait_charts(struct mssql_instance *mi, struct mssql_lock_instance *mli, const char *resource)
{
    if (!mli->st_lockWait) {
        char id[RRD_ID_LENGTH_MAX + 1];

        snprintfz(id, RRD_ID_LENGTH_MAX, "instance_%s_resource_%s_lock_wait", mi->instanceID, resource);
        netdata_fix_chart_name(id);
        mli->st_lockWait = rrdset_create_localhost(
            "mssql",
            id,
            NULL,
            "locks",
            "mssql.instance_resource_lock_waits",
            "Lock requests that required the caller to wait per resource",
            "locks/s",
            PLUGIN_WINDOWS_NAME,
            "PerflibMSSQL",
            PRIO_MSSQL_LOCKS_WAIT,
            mi->update_every,
            RRDSET_TYPE_LINE);

        rrdlabels_add(mli->st_lockWait->rrdlabels, "mssql_instance", mi->instanceID, RRDLABEL_SRC_AUTO);
        rrdlabels_add(mli->st_lockWait->rrdlabels, "resource", resource, RRDLABEL_SRC_AUTO);
        mli->rd_lockWait = rrddim_add(mli->st_lockWait, "locks", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
    }

    rrddim_set_by_pointer(mli->st_lockWait, mli->rd_lockWait, (collected_number)mli->lockWait.current.Data);
    rrdset_done(mli->st_lockWait);
}

void dict_mssql_dead_locks_charts(struct mssql_instance *mi, struct mssql_lock_instance *mli, const char *resource)
{
    if (!mli->st_deadLocks) {
        char id[RRD_ID_LENGTH_MAX + 1];

        snprintfz(id, RRD_ID_LENGTH_MAX, "instance_%s_resource_%s_deadlocks", mi->instanceID, resource);
        netdata_fix_chart_name(id);
        mli->st_deadLocks = rrdset_create_localhost(
            "mssql",
            id,
            NULL,
            "locks",
            "mssql.instance_resource_deadlocks",
            "Active lock requests that resulted in deadlock per resource",
            "deadlocks/s",
            PLUGIN_WINDOWS_NAME,
            "PerflibMSSQL",
            PRIO_MSSQL_LOCKS_DEADLOCK_PER_RESOURCE,
            mi->update_every,
            RRDSET_TYPE_LINE);

        rrdlabels_add(mli->st_deadLocks->rrdlabels, "mssql_instance", mi->instanceID, RRDLABEL_SRC_AUTO);
        rrdlabels_add(mli->st_deadLocks->rrdlabels, "resource", resource, RRDLABEL_SRC_AUTO);
        mli->rd_deadLocks = rrddim_add(mli->st_deadLocks, "deadlocks", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
    }

    rrddim_set_by_pointer(mli->st_deadLocks, mli->rd_deadLocks, (collected_number)mli->deadLocks.current.Data);
    rrdset_done(mli->st_deadLocks);
}

int dict_mssql_locks_charts_cb(const DICTIONARY_ITEM *item __maybe_unused, void *value, void *data __maybe_unused)
{
    const char *dimension = dictionary_acquired_item_name((DICTIONARY_ITEM *)item);
    struct mssql_lock_instance *mli = value;
    struct mssql_instance *mi = data;

    dict_mssql_locks_wait_charts(mi, mli, dimension);
    dict_mssql_dead_locks_charts(mi, mli, dimension);

    return 1;
}

static void do_mssql_locks(PERF_DATA_BLOCK *pDataBlock, struct mssql_instance *mi, int update_every __maybe_unused)
{
    if (!pDataBlock)
        goto end_mssql_locks;

    PERF_OBJECT_TYPE *pObjectType = perflibFindObjectTypeByName(pDataBlock, mi->objectName[NETDATA_MSSQL_LOCKS]);
    if (pObjectType) {
        if (pObjectType->NumInstances) {
            PERF_INSTANCE_DEFINITION *pi = NULL;
            for (LONG i = 0; i < pObjectType->NumInstances; i++) {
                pi = perflibForEachInstance(pDataBlock, pObjectType, pi);
                if (!pi)
                    break;

                if (!getInstanceName(pDataBlock, pObjectType, pi, windows_shared_buffer, sizeof(windows_shared_buffer)))
                    strncpyz(windows_shared_buffer, "[unknown]", sizeof(windows_shared_buffer) - 1);

                if (!strcasecmp(windows_shared_buffer, "_Total"))
                    continue;

                struct mssql_lock_instance *mli =
                    dictionary_set(mi->locks_instances, windows_shared_buffer, NULL, sizeof(*mli));
                if (!mli)
                    continue;

                perflibGetInstanceCounter(pDataBlock, pObjectType, pi, &mli->deadLocks);
                perflibGetInstanceCounter(pDataBlock, pObjectType, pi, &mli->lockWait);
            }
        }
    }

end_mssql_locks:
    dictionary_sorted_walkthrough_read(mi->locks_instances, dict_mssql_locks_charts_cb, mi);
}

void mssql_total_wait_charts(struct mssql_instance *mi, struct mssql_db_waits *mdw, const char *type)
{
    if (!mdw->st_total_wait) {
        char id[RRD_ID_LENGTH_MAX + 1];

        snprintfz(id, RRD_ID_LENGTH_MAX, "instance_%s_%s_total_wait", mi->instanceID, type);
        netdata_fix_chart_name(id);
        mdw->st_total_wait = rrdset_create_localhost(
            "mssql",
            id,
            NULL,
            "locks",
            "mssql.instance_total_wait_time",
            "Wait time for each wait type and category",
            "ms",
            PLUGIN_WINDOWS_NAME,
            "PerflibMSSQL",
            PRIO_MSSQL_WAITS_TOTAL,
            mi->update_every,
            RRDSET_TYPE_LINE);

        rrdlabels_add(mdw->st_total_wait->rrdlabels, "mssql_instance", mi->instanceID, RRDLABEL_SRC_AUTO);
        rrdlabels_add(mdw->st_total_wait->rrdlabels, "wait_type", mdw->wait_type, RRDLABEL_SRC_AUTO);
        rrdlabels_add(mdw->st_total_wait->rrdlabels, "wait_category", mdw->wait_category, RRDLABEL_SRC_AUTO);
        mdw->rd_total_wait = rrddim_add(mdw->st_total_wait, "duration", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
    }

    rrddim_set_by_pointer(
        mdw->st_total_wait, mdw->rd_total_wait, (collected_number)mdw->MSSQLDatabaseTotalWait.current.Data);

    rrdset_done(mdw->st_total_wait);
}

void mssql_resource_wait_charts(struct mssql_instance *mi, struct mssql_db_waits *mdw, const char *type)
{
    if (!mdw->st_resource_wait_msec) {
        char id[RRD_ID_LENGTH_MAX + 1];

        snprintfz(id, RRD_ID_LENGTH_MAX, "instance_%s_%s_resource_wait", mi->instanceID, type);
        netdata_fix_chart_name(id);
        mdw->st_resource_wait_msec = rrdset_create_localhost(
            "mssql",
            id,
            NULL,
            "locks",
            "mssql.instance_resource_wait_time",
            "Wait time for threads waiting on specific resource types for each wait type and category",
            "ms",
            PLUGIN_WINDOWS_NAME,
            "PerflibMSSQL",
            PRIO_MSSQL_RESOURCE_WAIT,
            mi->update_every,
            RRDSET_TYPE_LINE);

        rrdlabels_add(mdw->st_resource_wait_msec->rrdlabels, "mssql_instance", mi->instanceID, RRDLABEL_SRC_AUTO);
        rrdlabels_add(mdw->st_resource_wait_msec->rrdlabels, "wait_type", mdw->wait_type, RRDLABEL_SRC_AUTO);
        rrdlabels_add(mdw->st_resource_wait_msec->rrdlabels, "wait_category", mdw->wait_category, RRDLABEL_SRC_AUTO);
        mdw->rd_resource_wait_msec =
            rrddim_add(mdw->st_resource_wait_msec, "duration", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
    }

    rrddim_set_by_pointer(
        mdw->st_resource_wait_msec,
        mdw->rd_resource_wait_msec,
        (collected_number)mdw->MSSQLDatabaseResourceWaitMSec.current.Data);

    rrdset_done(mdw->st_resource_wait_msec);
}

void mssql_signal_wait_charts(struct mssql_instance *mi, struct mssql_db_waits *mdw, const char *type)
{
    if (!mdw->st_signal_wait_msec) {
        char id[RRD_ID_LENGTH_MAX + 1];

        snprintfz(id, RRD_ID_LENGTH_MAX, "instance_%s_%s_signal_wait", mi->instanceID, type);
        netdata_fix_chart_name(id);
        mdw->st_signal_wait_msec = rrdset_create_localhost(
            "mssql",
            id,
            NULL,
            "locks",
            "mssql.instance_signal_wait_time",
            "Delay between thread wakeup signal and actual execution start for each wait type and category",
            "ms",
            PLUGIN_WINDOWS_NAME,
            "PerflibMSSQL",
            PRIO_MSSQL_SIGNAL_WAIT,
            mi->update_every,
            RRDSET_TYPE_LINE);

        rrdlabels_add(mdw->st_signal_wait_msec->rrdlabels, "mssql_instance", mi->instanceID, RRDLABEL_SRC_AUTO);
        rrdlabels_add(mdw->st_signal_wait_msec->rrdlabels, "wait_type", mdw->wait_type, RRDLABEL_SRC_AUTO);
        rrdlabels_add(mdw->st_signal_wait_msec->rrdlabels, "wait_category", mdw->wait_category, RRDLABEL_SRC_AUTO);
        mdw->rd_signal_wait_msec =
            rrddim_add(mdw->st_signal_wait_msec, "duration", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
    }

    rrddim_set_by_pointer(
        mdw->st_signal_wait_msec,
        mdw->rd_signal_wait_msec,
        (collected_number)mdw->MSSQLDatabaseSignalWaitMSec.current.Data);

    rrdset_done(mdw->st_signal_wait_msec);
}

void mssql_max_wait_charts(struct mssql_instance *mi, struct mssql_db_waits *mdw, const char *type)
{
    if (!mdw->st_max_wait_time_msec) {
        char id[RRD_ID_LENGTH_MAX + 1];

        snprintfz(id, RRD_ID_LENGTH_MAX, "instance_%s_%s_max_wait", mi->instanceID, type);
        netdata_fix_chart_name(id);
        mdw->st_max_wait_time_msec = rrdset_create_localhost(
            "mssql",
            id,
            NULL,
            "locks",
            "mssql.instance_max_wait_time",
            "Maximum wait time for each wait type and category",
            "ms",
            PLUGIN_WINDOWS_NAME,
            "PerflibMSSQL",
            PRIO_MSSQL_MAX_WAIT_TIME,
            mi->update_every,
            RRDSET_TYPE_LINE);

        rrdlabels_add(mdw->st_max_wait_time_msec->rrdlabels, "mssql_instance", mi->instanceID, RRDLABEL_SRC_AUTO);
        rrdlabels_add(mdw->st_max_wait_time_msec->rrdlabels, "wait_type", mdw->wait_type, RRDLABEL_SRC_AUTO);
        rrdlabels_add(mdw->st_max_wait_time_msec->rrdlabels, "wait_category", mdw->wait_category, RRDLABEL_SRC_AUTO);
        mdw->rd_max_wait_time_msec =
            rrddim_add(mdw->st_max_wait_time_msec, "duration", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
    }

    rrddim_set_by_pointer(
        mdw->st_max_wait_time_msec,
        mdw->rd_max_wait_time_msec,
        (collected_number)mdw->MSSQLDatabaseMaxWaitTimeMSec.current.Data);

    rrdset_done(mdw->st_max_wait_time_msec);
}

void mssql_waiting_count_charts(struct mssql_instance *mi, struct mssql_db_waits *mdw, const char *type)
{
    if (!mdw->st_waiting_tasks) {
        char id[RRD_ID_LENGTH_MAX + 1];

        snprintfz(id, RRD_ID_LENGTH_MAX, "instance_%s_%s_waiting_count", mi->instanceID, type);
        netdata_fix_chart_name(id);
        mdw->st_waiting_tasks = rrdset_create_localhost(
            "mssql",
            id,
            NULL,
            "locks",
            "mssql.instance_waits",
            "Number of waits for each wait type and category",
            "waits/s",
            PLUGIN_WINDOWS_NAME,
            "PerflibMSSQL",
            PRIO_MSSQL_WAITING_COUNT,
            mi->update_every,
            RRDSET_TYPE_LINE);

        rrdlabels_add(mdw->st_waiting_tasks->rrdlabels, "mssql_instance", mi->instanceID, RRDLABEL_SRC_AUTO);
        rrdlabels_add(mdw->st_waiting_tasks->rrdlabels, "wait_type", mdw->wait_type, RRDLABEL_SRC_AUTO);
        rrdlabels_add(mdw->st_waiting_tasks->rrdlabels, "wait_category", mdw->wait_category, RRDLABEL_SRC_AUTO);
        mdw->rd_waiting_tasks = rrddim_add(mdw->st_waiting_tasks, "waits", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
    }

    rrddim_set_by_pointer(
        mdw->st_waiting_tasks, mdw->rd_waiting_tasks, (collected_number)mdw->MSSQLDatabaseWaitingTasks.current.Data);

    rrdset_done(mdw->st_waiting_tasks);
}

int dict_mssql_waits_charts_cb(const DICTIONARY_ITEM *item __maybe_unused, void *value, void *data __maybe_unused)
{
    const char *dimension = dictionary_acquired_item_name((DICTIONARY_ITEM *)item);
    struct mssql_db_waits *mdw = value;
    struct mssql_instance *mi = data;

    mssql_total_wait_charts(mi, mdw, dimension);
    mssql_resource_wait_charts(mi, mdw, dimension);
    mssql_signal_wait_charts(mi, mdw, dimension);
    mssql_max_wait_charts(mi, mdw, dimension);
    mssql_waiting_count_charts(mi, mdw, dimension);

    return 1;
}

static void do_mssql_waits(PERF_DATA_BLOCK *pDataBlock, struct mssql_instance *mi, int update_every)
{
    dictionary_sorted_walkthrough_read(mi->waits, dict_mssql_waits_charts_cb, mi);
}

void mssql_buffman_iops_chart(struct mssql_db_instance *mdi, struct mssql_instance *mi)
{
    if (!mdi->st_buff_page_iops) {
        char id[RRD_ID_LENGTH_MAX + 1];
        snprintfz(id, RRD_ID_LENGTH_MAX, "instance_%s_bufman_iops", mi->instanceID);
        netdata_fix_chart_name(id);
        mdi->st_buff_page_iops = rrdset_create_localhost(
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
                mi->update_every,
                RRDSET_TYPE_LINE);

        mdi->rd_buff_page_reads = rrddim_add(mdi->st_buff_page_iops, "read", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
        mdi->rd_buff_page_writes =
                rrddim_add(mdi->st_buff_page_iops, "written", NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);

        rrdlabels_add(mdi->st_buff_page_iops->rrdlabels, "mssql_instance", mi->instanceID, RRDLABEL_SRC_AUTO);
    }

    rrddim_set_by_pointer(
            mdi->st_buff_page_iops, mdi->rd_buff_page_reads, (collected_number)mdi->MSSQLBufferPageReads.current.Data);
    rrddim_set_by_pointer(
            mdi->st_buff_page_iops, mdi->rd_buff_page_writes, (collected_number)mdi->MSSQLBufferPageWrites.current.Data);

    rrdset_done(mdi->st_buff_page_iops);
}

void mssql_buffman_cache_hit_ratio_chart(struct mssql_db_instance *mdi, struct mssql_instance *mi)
{
    if (!mdi->st_buff_cache_hits) {
        char id[RRD_ID_LENGTH_MAX + 1];
        snprintfz(id, RRD_ID_LENGTH_MAX, "instance_%s_cache_hit_ratio", mi->instanceID);
        netdata_fix_chart_name(id);
        mdi->st_buff_cache_hits = rrdset_create_localhost(
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
                mi->update_every,
                RRDSET_TYPE_LINE);

        mdi->rd_buff_cache_hits =
                rrddim_add(mdi->st_buff_cache_hits, "hit_ratio", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);

        rrdlabels_add(mdi->st_buff_cache_hits->rrdlabels, "mssql_instance", mi->instanceID, RRDLABEL_SRC_AUTO);
    }

    rrddim_set_by_pointer(
            mdi->st_buff_cache_hits, mdi->rd_buff_cache_hits, (collected_number)mdi->MSSQLBufferCacheHits.current.Data);
    rrdset_done(mdi->st_buff_cache_hits);
}

void mssql_buffman_checkpoints_pages_chart(struct mssql_db_instance *mdi, struct mssql_instance *mi)
{
    if (!mdi->st_buff_checkpoint_pages) {
        char id[RRD_ID_LENGTH_MAX + 1];
        snprintfz(id, RRD_ID_LENGTH_MAX, "instance_%s_bufman_checkpoint_pages", mi->instanceID);
        netdata_fix_chart_name(id);
        mdi->st_buff_checkpoint_pages = rrdset_create_localhost(
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
                mi->update_every,
                RRDSET_TYPE_LINE);

        mdi->rd_buff_checkpoint_pages =
                rrddim_add(mdi->st_buff_checkpoint_pages, "log", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);

        rrdlabels_add(mdi->st_buff_checkpoint_pages->rrdlabels, "mssql_instance", mi->instanceID, RRDLABEL_SRC_AUTO);
    }

    rrddim_set_by_pointer(
            mdi->st_buff_checkpoint_pages,
            mdi->rd_buff_checkpoint_pages,
            (collected_number)mdi->MSSQLBufferCheckpointPages.current.Data);
    rrdset_done(mdi->st_buff_checkpoint_pages);
}

void mssql_buffman_page_life_expectancy_chart(struct mssql_db_instance *mdi, struct mssql_instance *mi)
{
    if (!mdi->st_buff_cache_page_life_expectancy) {
        char id[RRD_ID_LENGTH_MAX + 1];
        snprintfz(id, RRD_ID_LENGTH_MAX, "instance_%s_bufman_page_life_expectancy", mi->instanceID);
        netdata_fix_chart_name(id);
        mdi->st_buff_cache_page_life_expectancy = rrdset_create_localhost(
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
                mi->update_every,
                RRDSET_TYPE_LINE);

        mdi->rd_buff_cache_page_life_expectancy = rrddim_add(
                mdi->st_buff_cache_page_life_expectancy, "life_expectancy", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);

        rrdlabels_add(
                mdi->st_buff_cache_page_life_expectancy->rrdlabels, "mssql_instance", mi->instanceID, RRDLABEL_SRC_AUTO);
    }

    rrddim_set_by_pointer(
            mdi->st_buff_cache_page_life_expectancy,
            mdi->rd_buff_cache_page_life_expectancy,
            (collected_number)mdi->MSSQLBufferPageLifeExpectancy.current.Data);
    rrdset_done(mdi->st_buff_cache_page_life_expectancy);
}

void mssql_buffman_lazy_write_chart(struct mssql_db_instance *mdi, struct mssql_instance *mi)
{
    if (!mdi->st_buff_lazy_write) {
        char id[RRD_ID_LENGTH_MAX + 1];
        snprintfz(id, RRD_ID_LENGTH_MAX, "instance_%s_bufman_lazy_write", mi->instanceID);
        netdata_fix_chart_name(id);
        mdi->st_buff_lazy_write = rrdset_create_localhost(
                "mssql",
                id,
                NULL,
                "buffer cache",
                "mssql.instance_bufman_lazy_write",
                "Buffers written by buffer manager's lazy writer",
                "Lazy writes/sec",
                PLUGIN_WINDOWS_NAME,
                "PerflibMSSQL",
                PRIO_MSSQL_BUFF_LAZY_WRITE,
                mi->update_every,
                RRDSET_TYPE_LINE);

        mdi->rd_buff_lazy_write = rrddim_add(
                mdi->st_buff_lazy_write, "lazy_write", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);

        rrdlabels_add(
                mdi->st_buff_lazy_write->rrdlabels, "mssql_instance", mi->instanceID, RRDLABEL_SRC_AUTO);
    }

    rrddim_set_by_pointer(
            mdi->st_buff_lazy_write,
            mdi->rd_buff_lazy_write,
            (collected_number)mdi->MSSQLBufferLazyWrite.current.Data);
    rrdset_done(mdi->st_buff_lazy_write);
}

void mssql_buffman_page_lookups_chart(struct mssql_db_instance *mdi, struct mssql_instance *mi) {
    if (!mdi->st_buff_page_lookups) {
        char id[RRD_ID_LENGTH_MAX + 1];
        snprintfz(id, RRD_ID_LENGTH_MAX, "instance_%s_bufman_page_lookups", mi->instanceID);
        netdata_fix_chart_name(id);
        mdi->st_buff_page_lookups = rrdset_create_localhost(
                "mssql",
                id,
                NULL,
                "buffer cache",
                "mssql.instance_bufman_page_lookups",
                "Requests to find a page in the buffer pool.",
                "Page lookups/sec",
                PLUGIN_WINDOWS_NAME,
                "PerflibMSSQL",
                PRIO_MSSQL_BUFF_PAGE_LOOKUPS,
                mi->update_every,
                RRDSET_TYPE_LINE);

        mdi->rd_buff_page_lookups = rrddim_add(
                mdi->st_buff_page_lookups, "page_lookups", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);

        rrdlabels_add(
                mdi->st_buff_page_lookups->rrdlabels, "mssql_instance", mi->instanceID, RRDLABEL_SRC_AUTO);
    }

    rrddim_set_by_pointer(
            mdi->st_buff_page_lookups,
            mdi->rd_buff_page_lookups,
            (collected_number) mdi->MSSQLBufferPageLookups.current.Data);
    rrdset_done(mdi->st_buff_page_lookups);
}

static void netdata_mssql_compilations(struct mssql_db_instance *mdi, struct mssql_instance *mi) {
    if (!mdi->st_stats_compilation) {
        char id[RRD_ID_LENGTH_MAX + 1];
        snprintfz(id, RRD_ID_LENGTH_MAX, "instance_%s_sqlstats_sql_compilations", mi->instanceID);
        netdata_fix_chart_name(id);
        mdi->st_stats_compilation = rrdset_create_localhost(
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
                mi->update_every,
                RRDSET_TYPE_LINE);

        mdi->rd_stats_compilation =
                rrddim_add(mdi->st_stats_compilation, "compilations", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);

        rrdlabels_add(mdi->st_stats_compilation->rrdlabels, "mssql_instance", mi->instanceID, RRDLABEL_SRC_AUTO);
    }

    rrddim_set_by_pointer(
            mdi->st_stats_compilation, mdi->rd_stats_compilation,
            (collected_number) mdi->MSSQLCompilations.current.Data);
    rrdset_done(mdi->st_stats_compilation);
}

static void netdata_mssql_recompilations(struct mssql_db_instance *mdi, struct mssql_instance *mi)
{
    if (!mdi->st_stats_recompiles) {
        char id[RRD_ID_LENGTH_MAX + 1];
        snprintfz(id, RRD_ID_LENGTH_MAX, "instance_%s_sqlstats_sql_recompilations", mi->instanceID);
        netdata_fix_chart_name(id);
        mdi->st_stats_recompiles = rrdset_create_localhost(
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
                mi->update_every,
                RRDSET_TYPE_LINE);

        mdi->rd_stats_recompiles =
                rrddim_add(mdi->st_stats_recompiles, "recompiles", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);

        rrdlabels_add(mdi->st_stats_recompiles->rrdlabels, "mssql_instance", mi->instanceID, RRDLABEL_SRC_AUTO);
    }

    rrddim_set_by_pointer(
            mdi->st_stats_recompiles, mdi->rd_stats_recompiles, (collected_number)mdi->MSSQLRecompilations.current.Data);
    rrdset_done(mdi->st_stats_recompiles);
}

int dict_mssql_buffman_stats_charts_cb(const DICTIONARY_ITEM *item __maybe_unused, void *value, void *data __maybe_unused)
{
    struct mssql_db_instance *mdi = value;
    struct mssql_instance *mi = data;

    if (unlikely(!mdi->collect_instance))
        return 1;

    mssql_buffman_iops_chart(mdi, mi);
    mssql_buffman_cache_hit_ratio_chart(mdi, mi);
    mssql_buffman_checkpoints_pages_chart(mdi, mi);
    mssql_buffman_page_life_expectancy_chart(mdi, mi);
    mssql_buffman_lazy_write_chart(mdi, mi);
    mssql_buffman_page_lookups_chart(mdi, mi);

    netdata_mssql_compilations(mdi, mi);
    netdata_mssql_recompilations(mdi, mi);

    return 1;
}

static void do_mssql_bufferman_stats_sql(PERF_DATA_BLOCK *pDataBlock, struct mssql_instance *mi, int update_every)
{
    dictionary_sorted_walkthrough_read(mi->databases, dict_mssql_buffman_stats_charts_cb, mi);
}

static void mssql_database_backup_restore_chart(struct mssql_db_instance *mdi, const char *db, int update_every)
{
    char id[RRD_ID_LENGTH_MAX + 1];

    if (!mdi->st_db_backup_restore_operations) {
        snprintfz(id, RRD_ID_LENGTH_MAX, "db_%s_instance_%s_backup_restore_operations", db, mdi->parent->instanceID);
        netdata_fix_chart_name(id);
        mdi->st_db_backup_restore_operations = rrdset_create_localhost(
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
            mdi->st_db_backup_restore_operations->rrdlabels,
            "mssql_instance",
            mdi->parent->instanceID,
            RRDLABEL_SRC_AUTO);
        rrdlabels_add(mdi->st_db_backup_restore_operations->rrdlabels, "database", db, RRDLABEL_SRC_AUTO);

        mdi->rd_db_backup_restore_operations =
            rrddim_add(mdi->st_db_backup_restore_operations, "backup", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
    }

    rrddim_set_by_pointer(
        mdi->st_db_backup_restore_operations,
        mdi->rd_db_backup_restore_operations,
        (collected_number)mdi->MSSQLDatabaseBackupRestoreOperations.current.Data);

    rrdset_done(mdi->st_db_backup_restore_operations);
}

static void mssql_database_log_flushes_chart(struct mssql_db_instance *mdi, const char *db, int update_every)
{
    char id[RRD_ID_LENGTH_MAX + 1];

    if (!mdi->st_db_log_flushes) {
        snprintfz(id, RRD_ID_LENGTH_MAX, "db_%s_instance_%s_log_flushes", db, mdi->parent->instanceID);
        netdata_fix_chart_name(id);
        mdi->st_db_log_flushes = rrdset_create_localhost(
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

        rrdlabels_add(mdi->st_db_log_flushes->rrdlabels, "mssql_instance", mdi->parent->instanceID, RRDLABEL_SRC_AUTO);
        rrdlabels_add(mdi->st_db_log_flushes->rrdlabels, "database", db, RRDLABEL_SRC_AUTO);
    }

    if (!mdi->rd_db_log_flushes) {
        mdi->rd_db_log_flushes = rrddim_add(mdi->st_db_log_flushes, "flushes", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
    }

    rrddim_set_by_pointer(
        mdi->st_db_log_flushes, mdi->rd_db_log_flushes, (collected_number)mdi->MSSQLDatabaseLogFlushes.current.Data);

    rrdset_done(mdi->st_db_log_flushes);
}

static void mssql_database_log_flushed_chart(struct mssql_db_instance *mdi, const char *db, int update_every)
{
    char id[RRD_ID_LENGTH_MAX + 1];

    if (!mdi->st_db_log_flushed) {
        snprintfz(id, RRD_ID_LENGTH_MAX, "db_%s_instance_%s_log_flushed", db, mdi->parent->instanceID);
        netdata_fix_chart_name(id);
        mdi->st_db_log_flushed = rrdset_create_localhost(
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

        rrdlabels_add(mdi->st_db_log_flushed->rrdlabels, "mssql_instance", mdi->parent->instanceID, RRDLABEL_SRC_AUTO);
        rrdlabels_add(mdi->st_db_log_flushed->rrdlabels, "database", db, RRDLABEL_SRC_AUTO);

        mdi->rd_db_log_flushed = rrddim_add(mdi->st_db_log_flushed, "flushed", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
    }

    rrddim_set_by_pointer(
        mdi->st_db_log_flushed, mdi->rd_db_log_flushed, (collected_number)mdi->MSSQLDatabaseLogFlushed.current.Data);

    rrdset_done(mdi->st_db_log_flushed);
}

static void mssql_transactions_chart(struct mssql_db_instance *mdi, const char *db, int update_every)
{
    char id[RRD_ID_LENGTH_MAX + 1];

    if (!mdi->st_db_transactions) {
        snprintfz(id, RRD_ID_LENGTH_MAX, "db_%s_instance_%s_transactions", db, mdi->parent->instanceID);
        netdata_fix_chart_name(id);
        mdi->st_db_transactions = rrdset_create_localhost(
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

        rrdlabels_add(mdi->st_db_transactions->rrdlabels, "mssql_instance", mdi->parent->instanceID, RRDLABEL_SRC_AUTO);
        rrdlabels_add(mdi->st_db_transactions->rrdlabels, "database", db, RRDLABEL_SRC_AUTO);

        mdi->rd_db_transactions =
            rrddim_add(mdi->st_db_transactions, "transactions", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
    }

    rrddim_set_by_pointer(
        mdi->st_db_transactions,
        mdi->rd_db_transactions,
        (collected_number)mdi->MSSQLDatabaseTransactions.current.Data);

    rrdset_done(mdi->st_db_transactions);
}

static void mssql_write_transactions_chart(struct mssql_db_instance *mdi, const char *db, int update_every)
{
    char id[RRD_ID_LENGTH_MAX + 1];

    if (!mdi->st_db_write_transactions) {
        snprintfz(id, RRD_ID_LENGTH_MAX, "db_%s_instance_%s_write_transactions", db, mdi->parent->instanceID);
        netdata_fix_chart_name(id);
        mdi->st_db_write_transactions = rrdset_create_localhost(
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
            mdi->st_db_write_transactions->rrdlabels, "mssql_instance", mdi->parent->instanceID, RRDLABEL_SRC_AUTO);
        rrdlabels_add(mdi->st_db_write_transactions->rrdlabels, "database", db, RRDLABEL_SRC_AUTO);

        mdi->rd_db_write_transactions =
            rrddim_add(mdi->st_db_write_transactions, "write", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
    }

    rrddim_set_by_pointer(
        mdi->st_db_write_transactions,
        mdi->rd_db_write_transactions,
        (collected_number)mdi->MSSQLDatabaseWriteTransactions.current.Data);

    rrdset_done(mdi->st_db_write_transactions);
}

static void mssql_lockwait_chart(struct mssql_db_instance *mdi, const char *db, int update_every)
{
    char id[RRD_ID_LENGTH_MAX + 1];

    if (!mdi->st_db_lockwait) {
        snprintfz(id, RRD_ID_LENGTH_MAX, "db_%s_instance_%s_lockwait", db, mdi->parent->instanceID);
        netdata_fix_chart_name(id);
        mdi->st_db_lockwait = rrdset_create_localhost(
            "mssql",
            id,
            NULL,
            "locks",
            "mssql.database_lockwait",
            "Lock requests that required the caller to wait.",
            "locks/s",
            PLUGIN_WINDOWS_NAME,
            "PerflibMSSQL",
            PRIO_MSSQL_DATABASE_LOCKWAIT_PER_SECOND,
            update_every,
            RRDSET_TYPE_LINE);

        rrdlabels_add(mdi->st_db_lockwait->rrdlabels, "mssql_instance", mdi->parent->instanceID, RRDLABEL_SRC_AUTO);
        rrdlabels_add(mdi->st_db_lockwait->rrdlabels, "database", db, RRDLABEL_SRC_AUTO);

        mdi->rd_db_lockwait = rrddim_add(mdi->st_db_lockwait, "lock", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
    }

    rrddim_set_by_pointer(
        mdi->st_db_lockwait, mdi->rd_db_lockwait, (collected_number)mdi->MSSQLDatabaseLockWaitSec.current.Data);

    rrdset_done(mdi->st_db_lockwait);
}

static void mssql_deadlock_chart(struct mssql_db_instance *mdi, const char *db, int update_every)
{
    char id[RRD_ID_LENGTH_MAX + 1];

    if (!mdi->st_db_deadlock) {
        snprintfz(id, RRD_ID_LENGTH_MAX, "db_%s_instance_%s_deadlocks", db, mdi->parent->instanceID);
        netdata_fix_chart_name(id);
        mdi->st_db_deadlock = rrdset_create_localhost(
            "mssql",
            id,
            NULL,
            "locks",
            "mssql.database_deadlocks",
            "Lock requests that resulted in deadlock.",
            "deadlocks/s",
            PLUGIN_WINDOWS_NAME,
            "PerflibMSSQL",
            PRIO_MSSQL_DATABASE_DEADLOCKS_PER_SECOND,
            update_every,
            RRDSET_TYPE_LINE);

        rrdlabels_add(mdi->st_db_deadlock->rrdlabels, "mssql_instance", mdi->parent->instanceID, RRDLABEL_SRC_AUTO);
        rrdlabels_add(mdi->st_db_deadlock->rrdlabels, "database", db, RRDLABEL_SRC_AUTO);

        mdi->rd_db_deadlock = rrddim_add(mdi->st_db_deadlock, "deadlocks", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
    }

    rrddim_set_by_pointer(
        mdi->st_db_deadlock, mdi->rd_db_deadlock, (collected_number)mdi->MSSQLDatabaseDeadLockSec.current.Data);

    rrdset_done(mdi->st_db_deadlock);
}

static void mssql_lock_request_chart(struct mssql_db_instance *mdi, const char *db, int update_every)
{
    char id[RRD_ID_LENGTH_MAX + 1];

    if (!mdi->st_lock_requests) {
        snprintfz(id, RRD_ID_LENGTH_MAX, "db_%s_instance_%s_lock_requests", db, mdi->parent->instanceID);
        netdata_fix_chart_name(id);
        mdi->st_lock_requests = rrdset_create_localhost(
            "mssql",
            id,
            NULL,
            "locks",
            "mssql.database_lock_requests",
            "Number of new locks and lock conversions requested.",
            "requests/s",
            PLUGIN_WINDOWS_NAME,
            "PerflibMSSQL",
            PRIO_MSSQL_DATABASE_LOCK_REQUESTS_SEC,
            update_every,
            RRDSET_TYPE_LINE);

        rrdlabels_add(mdi->st_lock_requests->rrdlabels, "mssql_instance", mdi->parent->instanceID, RRDLABEL_SRC_AUTO);
        rrdlabels_add(mdi->st_lock_requests->rrdlabels, "database", db, RRDLABEL_SRC_AUTO);

        mdi->rd_lock_requests = rrddim_add(mdi->st_lock_requests, "requests", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
    }

    rrddim_set_by_pointer(
        mdi->st_lock_requests, mdi->rd_lock_requests, (collected_number)mdi->MSSQLDatabaseLockRequestsSec.current.Data);

    rrdset_done(mdi->st_lock_requests);
}

static void mssql_lock_timeout_chart(struct mssql_db_instance *mdi, const char *db, int update_every)
{
    char id[RRD_ID_LENGTH_MAX + 1];

    if (!mdi->st_lock_timeouts) {
        snprintfz(id, RRD_ID_LENGTH_MAX, "db_%s_instance_%s_lock_timeouts", db, mdi->parent->instanceID);
        netdata_fix_chart_name(id);
        mdi->st_lock_timeouts = rrdset_create_localhost(
            "mssql",
            id,
            NULL,
            "locks",
            "mssql.database_lock_timeouts",
            "Lock that timed out.",
            "timeouts/s",
            PLUGIN_WINDOWS_NAME,
            "PerflibMSSQL",
            PRIO_MSSQL_DATABASE_LOCKS_TIMEOUT_PER_SECOND,
            update_every,
            RRDSET_TYPE_LINE);

        rrdlabels_add(mdi->st_lock_timeouts->rrdlabels, "mssql_instance", mdi->parent->instanceID, RRDLABEL_SRC_AUTO);
        rrdlabels_add(mdi->st_lock_timeouts->rrdlabels, "database", db, RRDLABEL_SRC_AUTO);

        mdi->rd_lock_timeouts = rrddim_add(mdi->st_lock_timeouts, "timeouts", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
    }

    rrddim_set_by_pointer(
        mdi->st_lock_timeouts, mdi->rd_lock_timeouts, (collected_number)mdi->MSSQLDatabaseLockTimeoutsSec.current.Data);

    rrdset_done(mdi->st_lock_timeouts);
}

static void mssql_active_transactions_chart(struct mssql_db_instance *mdi, const char *db, int update_every)
{
    char id[RRD_ID_LENGTH_MAX + 1];

    if (!mdi->st_db_active_transactions) {
        snprintfz(id, RRD_ID_LENGTH_MAX, "db_%s_instance_%s_active_transactions", db, mdi->parent->instanceID);
        netdata_fix_chart_name(id);
        mdi->st_db_active_transactions = rrdset_create_localhost(
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
            mdi->st_db_active_transactions->rrdlabels, "mssql_instance", mdi->parent->instanceID, RRDLABEL_SRC_AUTO);
        rrdlabels_add(mdi->st_db_active_transactions->rrdlabels, "database", db, RRDLABEL_SRC_AUTO);

        mdi->rd_db_active_transactions =
            rrddim_add(mdi->st_db_active_transactions, "active", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
    }

    rrddim_set_by_pointer(
        mdi->st_db_active_transactions,
        mdi->rd_db_active_transactions,
        (collected_number)mdi->MSSQLDatabaseActiveTransactions.current.Data);

    rrdset_done(mdi->st_db_active_transactions);
}

static inline void mssql_data_file_size_chart(struct mssql_db_instance *mdi, const char *db, int update_every)
{
    char id[RRD_ID_LENGTH_MAX + 1];

    if (unlikely(!mdi->st_db_data_file_size)) {
        snprintfz(id, RRD_ID_LENGTH_MAX, "db_%s_instance_%s_data_files_size", db, mdi->parent->instanceID);
        netdata_fix_chart_name(id);
        mdi->st_db_data_file_size = rrdset_create_localhost(
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
            mdi->st_db_data_file_size->rrdlabels, "mssql_instance", mdi->parent->instanceID, RRDLABEL_SRC_AUTO);
        rrdlabels_add(mdi->st_db_data_file_size->rrdlabels, "database", db, RRDLABEL_SRC_AUTO);

        mdi->rd_db_data_file_size = rrddim_add(mdi->st_db_data_file_size, "size", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
    }

    collected_number data = mdi->MSSQLDatabaseDataFileSize.current.Data;
    rrddim_set_by_pointer(mdi->st_db_data_file_size, mdi->rd_db_data_file_size, data);

    rrdset_done(mdi->st_db_data_file_size);
}

int dict_mssql_databases_charts_cb(const DICTIONARY_ITEM *item __maybe_unused, void *value, void *data __maybe_unused)
{
    struct mssql_db_instance *mdi = value;
    const char *db = dictionary_acquired_item_name((DICTIONARY_ITEM *)item);

    if (!mdi->collecting_data) {
        goto endchartcb;
    }

    int *update_every = data;

    void (*transaction_chart[])(struct mssql_db_instance *, const char *, int) = {
        mssql_data_file_size_chart,
        mssql_transactions_chart,
        mssql_database_backup_restore_chart,
        mssql_database_log_flushed_chart,
        mssql_database_log_flushes_chart,
        mssql_active_transactions_chart,
        mssql_write_transactions_chart,
        mssql_lockwait_chart,
        mssql_deadlock_chart,
        mssql_lock_timeout_chart,
        mssql_lock_request_chart,

        // Last function pointer must be NULL
        NULL};

    int i;
    for (i = 0; transaction_chart[i]; i++) {
        transaction_chart[i](mdi, db, *update_every);
    }

endchartcb:
    return 1;
}

static void do_mssql_databases(PERF_DATA_BLOCK *pDataBlock, struct mssql_instance *mi, int update_every)
{
    if (!pDataBlock)
        goto end_mssql_databases;

    PERF_OBJECT_TYPE *pObjectType = perflibFindObjectTypeByName(pDataBlock, mi->objectName[NETDATA_MSSQL_DATABASE]);
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

        struct mssql_db_instance *mdi = dictionary_set(mi->databases, windows_shared_buffer, NULL, sizeof(*mdi));
        if (!mdi)
            continue;

        if (!mdi->parent) {
            mdi->parent = mi;
        }

        if (!i)
            mdi->collect_instance = true;
    }

end_mssql_databases:
    dictionary_sorted_walkthrough_read(mi->databases, dict_mssql_databases_charts_cb, &update_every);
}

static void do_mssql_memory_mgr(PERF_DATA_BLOCK *pDataBlock, struct mssql_instance *mi, int update_every)
{
    char id[RRD_ID_LENGTH_MAX + 1];

    PERF_OBJECT_TYPE *pObjectType = perflibFindObjectTypeByName(pDataBlock, mi->objectName[NETDATA_MSSQL_MEMORY]);
    if (!pObjectType)
        return;

    if (perflibGetObjectCounter(pDataBlock, pObjectType, &mi->MSSQLConnectionMemoryBytes)) {
        if (!mi->st_conn_memory) {
            snprintfz(id, RRD_ID_LENGTH_MAX, "instance_%s_memmgr_connection_memory_bytes", mi->instanceID);
            netdata_fix_chart_name(id);
            mi->st_conn_memory = rrdset_create_localhost(
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

            mi->rd_conn_memory = rrddim_add(mi->st_conn_memory, "memory", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);

            rrdlabels_add(mi->st_conn_memory->rrdlabels, "mssql_instance", mi->instanceID, RRDLABEL_SRC_AUTO);
        }

        rrddim_set_by_pointer(
            mi->st_conn_memory,
            mi->rd_conn_memory,
            (collected_number)(mi->MSSQLConnectionMemoryBytes.current.Data * 1024));
        rrdset_done(mi->st_conn_memory);
    }

    if (perflibGetObjectCounter(pDataBlock, pObjectType, &mi->MSSQLExternalBenefitOfMemory)) {
        if (!mi->st_ext_benefit_mem) {
            snprintfz(id, RRD_ID_LENGTH_MAX, "instance_%s_memmgr_external_benefit_of_memory", mi->instanceID);
            netdata_fix_chart_name(id);
            mi->st_ext_benefit_mem = rrdset_create_localhost(
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

            mi->rd_ext_benefit_mem = rrddim_add(mi->st_ext_benefit_mem, "benefit", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);

            rrdlabels_add(mi->st_ext_benefit_mem->rrdlabels, "mssql_instance", mi->instanceID, RRDLABEL_SRC_AUTO);
        }

        rrddim_set_by_pointer(
            mi->st_ext_benefit_mem,
            mi->rd_ext_benefit_mem,
            (collected_number)mi->MSSQLExternalBenefitOfMemory.current.Data);
        rrdset_done(mi->st_ext_benefit_mem);
    }

    if (perflibGetObjectCounter(pDataBlock, pObjectType, &mi->MSSQLPendingMemoryGrants)) {
        if (!mi->st_pending_mem_grant) {
            snprintfz(id, RRD_ID_LENGTH_MAX, "instance_%s_memmgr_pending_memory_grants", mi->instanceID);
            netdata_fix_chart_name(id);
            mi->st_pending_mem_grant = rrdset_create_localhost(
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

            mi->rd_pending_mem_grant =
                rrddim_add(mi->st_pending_mem_grant, "pending", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);

            rrdlabels_add(mi->st_pending_mem_grant->rrdlabels, "mssql_instance", mi->instanceID, RRDLABEL_SRC_AUTO);
        }

        rrddim_set_by_pointer(
            mi->st_pending_mem_grant,
            mi->rd_pending_mem_grant,
            (collected_number)mi->MSSQLPendingMemoryGrants.current.Data);

        rrdset_done(mi->st_pending_mem_grant);
    }

    if (perflibGetObjectCounter(pDataBlock, pObjectType, &mi->MSSQLTotalServerMemory)) {
        if (!mi->st_mem_tot_server) {
            snprintfz(id, RRD_ID_LENGTH_MAX, "instance_%s_memmgr_server_memory", mi->instanceID);
            netdata_fix_chart_name(id);
            mi->st_mem_tot_server = rrdset_create_localhost(
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

            mi->rd_mem_tot_server = rrddim_add(mi->st_mem_tot_server, "memory", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);

            rrdlabels_add(mi->st_mem_tot_server->rrdlabels, "mssql_instance", mi->instanceID, RRDLABEL_SRC_AUTO);
        }

        rrddim_set_by_pointer(
            mi->st_mem_tot_server,
            mi->rd_mem_tot_server,
            (collected_number)(mi->MSSQLTotalServerMemory.current.Data * 1024));

        rrdset_done(mi->st_mem_tot_server);
    }
}

static inline PERF_DATA_BLOCK *
netdata_mssql_get_perf_data_block(bool *collect_perflib, struct mssql_instance *mi, DWORD idx)
{
    DWORD id = RegistryFindIDByName(mi->objectName[idx]);
    if (id == PERFLIB_REGISTRY_NAME_NOT_FOUND) {
        collect_perflib[idx] = false;
        return NULL;
    }

    PERF_DATA_BLOCK *pDataBlock = perflibGetPerformanceData(id);
    if (!pDataBlock) {
        collect_perflib[idx] = true;
        return NULL;
    }

    return pDataBlock;
}

int dict_mssql_charts_cb(const DICTIONARY_ITEM *item __maybe_unused, void *value, void *data __maybe_unused)
{
    struct mssql_instance *mi = value;
    int *update_every = data;

    static void (*doMSSQL[])(PERF_DATA_BLOCK *, struct mssql_instance *, int) = {
            do_mssql_general_stats,
            do_mssql_errors,
            do_mssql_memory_mgr,
            do_mssql_statistics_perflib,
            do_mssql_access_methods,

            do_mssql_databases,
            do_mssql_locks,
            do_mssql_waits,
            do_mssql_bufferman_stats_sql,

            NULL};

    DWORD i;
    PERF_DATA_BLOCK *pDataBlock;
    static bool collect_perflib[NETDATA_MSSQL_METRICS_END] = {true, true, true, true, true, true, true, true, true};
    for (i = 0; i < NETDATA_MSSQL_ACCESS_METHODS; i++) {
        if (!collect_perflib[i])
            continue;

        pDataBlock = netdata_mssql_get_perf_data_block(collect_perflib, mi, i);
        if (!pDataBlock)
            continue;

        doMSSQL[i](pDataBlock, mi, *update_every);
    }

    if (unlikely(!mi->conn || !mi->conn->is_connected))
        return 1;

    for (i = NETDATA_MSSQL_DATABASE; doMSSQL[i]; i++) {
        pDataBlock = (collect_perflib[i]) ? netdata_mssql_get_perf_data_block(collect_perflib, mi, i): NULL;

        doMSSQL[i](pDataBlock, mi, *update_every);
    }

    return 1;
}

// Entry point

int do_PerflibMSSQL(int update_every, usec_t dt __maybe_unused)
{
    static bool initialized = false;

    if (unlikely(!initialized)) {
        if (initialize(update_every))
            return -1;

        initialized = true;
    }

    dictionary_sorted_walkthrough_read(mssql_instances, dict_mssql_charts_cb, &update_every);

    return 0;
}

void do_PerflibMSSQL_cleanup()
{
    if (nd_thread_join(mssql_queries_thread))
        nd_log_daemon(NDLP_ERR, "Failed to join mssql queries thread");
}
