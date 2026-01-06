// SPDX-License-Identifier: GPL-3.0-or-later

#include "windows_plugin.h"
#include "windows-internals.h"

#include "perflib-mssql-queries.h"

DICTIONARY *conn_options;

static inline bool netdata_mssql_check_result(SQLRETURN ret)
{
    return (ret != SQL_SUCCESS && ret != SQL_SUCCESS_WITH_INFO);
}

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
    if (likely(SQL_SUCCESS == SQLGetDiagRec((short)type, handle, 1, state, NULL, message, 1024, NULL))) {
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

static inline void netdata_MSSQL_release_results(SQLHSTMT stmt)
{
    if (stmt == SQL_NULL_HSTMT)
        return;

    SQLCloseCursor(stmt);
    SQLFreeStmt(stmt, SQL_UNBIND);
    SQLFreeStmt(stmt, SQL_RESET_PARAMS);
}

static ULONGLONG netdata_MSSQL_fill_long_value(SQLHSTMT stmt, const char *mask, const char *dbname, char *instance)
{
    long db_size = 0;
    SQLLEN col_data_len = 0;

    SQLCHAR query[512];
    snprintfz((char *)query, 511, mask, dbname);

    SQLRETURN ret;

    ret = SQLExecDirect(stmt, query, SQL_NTS);
    if (likely(netdata_mssql_check_result(ret))) {
        netdata_MSSQL_error(SQL_HANDLE_STMT, stmt, NETDATA_MSSQL_ODBC_QUERY, instance);
        netdata_MSSQL_release_results(stmt);
        return (ULONGLONG)ULONG_LONG_MAX;
    }

    ret = SQLBindCol(stmt, 1, SQL_C_LONG, &db_size, sizeof(long), &col_data_len);
    if (likely(netdata_mssql_check_result(ret))) {
        netdata_MSSQL_error(SQL_HANDLE_STMT, stmt, NETDATA_MSSQL_ODBC_PREPARE, instance);
        netdata_MSSQL_release_results(stmt);
        return (ULONGLONG)ULONG_LONG_MAX;
    }

    ret = SQLFetch(stmt);
    if (likely(netdata_mssql_check_result(ret))) {
        netdata_MSSQL_error(SQL_HANDLE_STMT, stmt, NETDATA_MSSQL_ODBC_FETCH, instance);
        netdata_MSSQL_release_results(stmt);
        return (ULONGLONG)ULONG_LONG_MAX;
    }

    if (col_data_len == SQL_NULL_DATA)
        db_size = 0;

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

bool netdata_mssql_counter_buffer(struct mssql_db_instance *mdi, char *inst_obj, long value)
{
    bool ret = false;
    if (!strncmp(inst_obj,
                 NETDATA_MSSQL_STATS_COMPILATIONS_METRIC,
                 sizeof(NETDATA_MSSQL_STATS_COMPILATIONS_METRIC) - 1)) {
        mdi->MSSQLCompilations.current.Data = (ULONGLONG)value;
        ret = true;
    } else if (!strncmp(inst_obj,
                        NETDATA_MSSQL_STATS_RECOMPILATIONS_METRIC,
                        sizeof(NETDATA_MSSQL_STATS_RECOMPILATIONS_METRIC) - 1)) {
        mdi->MSSQLRecompilations.current.Data = (ULONGLONG)value;
        ret = true;
    } else if (!strncmp(inst_obj,
                      NETDATA_MSSQL_BUFFER_PAGE_READS_METRIC,
                      sizeof(NETDATA_MSSQL_BUFFER_PAGE_READS_METRIC) - 1)) {
        mdi->MSSQLBufferPageReads.current.Data = (ULONGLONG)value;
        ret = true;
    } else if (!strncmp(inst_obj,
                      NETDATA_MSSQL_BUFFER_PAGE_WRITES_METRIC,
                      sizeof(NETDATA_MSSQL_BUFFER_PAGE_WRITES_METRIC) - 1)) {
        mdi->MSSQLBufferPageWrites.current.Data = (ULONGLONG)value;
        ret = true;
    } else if (!strncmp(inst_obj,
                      NETDATA_MSSQL_BUFFER_PAGE_CACHE_METRIC,
                      sizeof(NETDATA_MSSQL_BUFFER_PAGE_CACHE_METRIC) - 1)) {
        mdi->MSSQLBufferCacheHits.current.Data = (ULONGLONG)value;
        ret = true;
    } else if (!strncmp(inst_obj,
                      NETDATA_MSSQL_BUFFER_CHECKPOINT_METRIC,
                      sizeof(NETDATA_MSSQL_BUFFER_CHECKPOINT_METRIC) - 1)) {
        mdi->MSSQLBufferCheckpointPages.current.Data = (ULONGLONG)value;
        ret = true;
    } else if (!strncmp(inst_obj,
                        NETDATA_MSSQL_BUFFER_PAGE_LIFE_METRIC,
                        sizeof(NETDATA_MSSQL_BUFFER_PAGE_LIFE_METRIC) - 1)) {
        mdi->MSSQLBufferPageLifeExpectancy.current.Data = (ULONGLONG)value;
        ret = true;
    } else if (!strncmp(inst_obj,
                        NETDATA_MSSQL_BUFFER_LAZY_WRITES_METRIC,
                        sizeof(NETDATA_MSSQL_BUFFER_LAZY_WRITES_METRIC) - 1)) {
        mdi->MSSQLBufferLazyWrite.current.Data = (ULONGLONG)value;
        ret = true;
    } else if (!strncmp(inst_obj,
                        NETDATA_MSSQL_BUFFER_PAGE_LOOKUPS_METRIC,
                        sizeof(NETDATA_MSSQL_BUFFER_PAGE_LOOKUPS_METRIC) - 1)) {
        mdi->MSSQLBufferPageLookups.current.Data = (ULONGLONG)value;
        ret = true;
    }

    return ret;
}

bool netdata_mssql_counter_transaction(struct mssql_db_instance *mdi, char *object_name, long value)
{
    bool ret = false;
    if (unlikely(!strncmp(object_name,
                          NETDATA_MSSQL_ACTIVE_TRANSACTIONS_METRIC,
                          sizeof(NETDATA_MSSQL_ACTIVE_TRANSACTIONS_METRIC) - 1))) {
        mdi->MSSQLDatabaseActiveTransactions.current.Data = (ULONGLONG)value;
        ret = true;
    } else if (unlikely(!strncmp(object_name,
                                 NETDATA_MSSQL_TRANSACTION_PER_SEC_METRIC,
                                 sizeof(NETDATA_MSSQL_TRANSACTION_PER_SEC_METRIC) - 1))) {
        mdi->MSSQLDatabaseTransactions.current.Data = (ULONGLONG)value;
        ret = true;
    } else if (unlikely(!strncmp(object_name,
                                 NETDATA_MSSQL_WRITE_TRANSACTIONS_METRIC,
                                 sizeof(NETDATA_MSSQL_WRITE_TRANSACTIONS_METRIC) - 1))) {
        mdi->MSSQLDatabaseWriteTransactions.current.Data = (ULONGLONG)value;
        ret = true;
    }

    return ret;
}

bool netdata_mssql_counter_lock_and_log(struct mssql_db_instance *mdi, char *object_name, long value)
{
    bool ret = false;
    if (unlikely(!strncmp(object_name,
                          NETDATA_MSSQL_BACKUP_RESTORE_METRIC,
                          sizeof(NETDATA_MSSQL_BACKUP_RESTORE_METRIC) - 1))) {
        mdi->MSSQLDatabaseBackupRestoreOperations.current.Data = (ULONGLONG)value;
        ret = true;
    } else if (unlikely(!strncmp(object_name,
                                 NETDATA_MSSQL_LOG_FLUSHED_METRIC,
                                 sizeof(NETDATA_MSSQL_LOG_FLUSHED_METRIC) - 1))) {
        mdi->MSSQLDatabaseLogFlushed.current.Data = (ULONGLONG)value;
        ret = true;
    } else if (unlikely(!strncmp(object_name,
                                 NETDATA_MSSQL_LOG_FLUSHES_METRIC,
                                 sizeof(NETDATA_MSSQL_LOG_FLUSHES_METRIC) - 1))) {
        mdi->MSSQLDatabaseLogFlushes.current.Data = (ULONGLONG)value;
        ret = true;
    } else if (unlikely(!strncmp(object_name,
                                 NETDATA_MSSQL_NUMBER_DEADLOCKS_METRIC,
                                 sizeof(NETDATA_MSSQL_NUMBER_DEADLOCKS_METRIC) - 1))) {
        mdi->MSSQLDatabaseDeadLockSec.current.Data = (ULONGLONG)value;
        ret = true;
    } else if (unlikely(!strncmp(object_name,
                                 NETDATA_MSSQL_LOCK_WAITS_METRIC,
                                 sizeof(NETDATA_MSSQL_LOCK_WAITS_METRIC) - 1))) {
        mdi->MSSQLDatabaseLockWaitSec.current.Data = (ULONGLONG)value;
        ret = true;
    } else if (unlikely(!strncmp(object_name,
                                 NETDATA_MSSQL_LOCK_TIMEOUTS_METRIC,
                                 sizeof(NETDATA_MSSQL_LOCK_TIMEOUTS_METRIC) - 1))) {
        mdi->MSSQLDatabaseLockTimeoutsSec.current.Data = (ULONGLONG)value;
        ret = true;
    } else if (unlikely(!strncmp(object_name,
                                 NETDATA_MSSQL_LOCK_REQUESTS_METRIC,
                                 sizeof(NETDATA_MSSQL_LOCK_REQUESTS_METRIC) - 1))) {
        mdi->MSSQLDatabaseLockRequestsSec.current.Data = (ULONGLONG)value;
        ret = true;
    }

    return ret;
}

void dict_mssql_fill_performance_counters(struct mssql_db_instance *mdi, const char *dbname, const char *instance_name)
{
    char object_name[NETDATA_MAX_INSTANCE_OBJECT + 1] = {};
    long value = 0;
    SQLLEN col_object_len = 0, col_value_len = 0;

    if (unlikely(!mdi->parent->conn->collect_transactions &&
        !mdi->parent->conn->collect_buffer &&
        !mdi->collect_instance))
        return;

    SQLCHAR query[sizeof(NETDATA_QUERY_PERFORMANCE_COUNTER) + 2 * NETDATA_MAX_INSTANCE_OBJECT + 1];
    snprintfz(
            (char *)query,
            sizeof(NETDATA_QUERY_PERFORMANCE_COUNTER) + 2 * NETDATA_MAX_INSTANCE_OBJECT,
            NETDATA_QUERY_PERFORMANCE_COUNTER,
            dbname,
            dbname);

    SQLRETURN ret = SQLExecDirect(mdi->parent->conn->dbPerfCounterSTMT, (SQLCHAR *)query, SQL_NTS);
    if (likely(netdata_mssql_check_result(ret))) {
        mdi->collecting_data = false;
        netdata_MSSQL_error(
                SQL_HANDLE_STMT, mdi->parent->conn->dbPerfCounterSTMT, NETDATA_MSSQL_ODBC_QUERY, mdi->parent->instanceID);
        goto endcounters;
    }

    ret = SQLBindCol(
            mdi->parent->conn->dbPerfCounterSTMT, 1, SQL_C_CHAR, object_name, sizeof(object_name), &col_object_len);
    if (likely(netdata_mssql_check_result(ret))) {
        netdata_MSSQL_error(
                SQL_HANDLE_STMT, mdi->parent->conn->dbPerfCounterSTMT, NETDATA_MSSQL_ODBC_PREPARE, mdi->parent->instanceID);
        goto endcounters;
    }

    ret = SQLBindCol(mdi->parent->conn->dbPerfCounterSTMT, 2, SQL_C_LONG, &value, sizeof(value), &col_value_len);
    if (likely(netdata_mssql_check_result(ret))) {
        netdata_MSSQL_error(
                SQL_HANDLE_STMT, mdi->parent->conn->dbPerfCounterSTMT, NETDATA_MSSQL_ODBC_PREPARE, mdi->parent->instanceID);
        goto endcounters;
    }

    do {
        ret = SQLFetch(mdi->parent->conn->dbPerfCounterSTMT);
        switch (ret) {
            case SQL_SUCCESS:
            case SQL_SUCCESS_WITH_INFO:
                break;
            case SQL_NO_DATA:
            default:
                goto endcounters;
        }

        if (col_object_len == SQL_NULL_DATA)
            continue;
        if (col_value_len == SQL_NULL_DATA)
            value = 0;

        if (netdata_mssql_counter_buffer(mdi, object_name, value))
            continue;

        if (netdata_mssql_counter_transaction(mdi, object_name, value))
            continue;

        if (netdata_mssql_counter_lock_and_log(mdi, object_name, value))
            continue;

    } while (true);

endcounters:
    netdata_MSSQL_release_results(mdi->parent->conn->dbPerfCounterSTMT);
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
        sizeof(NETDATA_QUERY_LOCKS_MASK) + 2 * NETDATA_MAX_INSTANCE_OBJECT,
        NETDATA_QUERY_LOCKS_MASK,
        dbname,
        dbname);

    SQLRETURN ret = SQLExecDirect(mdi->parent->conn->dbLocksSTMT, (SQLCHAR *)query, SQL_NTS);
    if (likely(netdata_mssql_check_result(ret))) {
        mdi->collecting_data = false;
        netdata_MSSQL_error(
            SQL_HANDLE_STMT, mdi->parent->conn->dbLocksSTMT, NETDATA_MSSQL_ODBC_QUERY, mdi->parent->instanceID);
        goto endlocks;
    }

    ret = SQLBindCol(
        mdi->parent->conn->dbLocksSTMT, 1, SQL_C_CHAR, resource_type, sizeof(resource_type), &col_object_len);
    if (likely(netdata_mssql_check_result(ret))) {
        netdata_MSSQL_error(
            SQL_HANDLE_STMT, mdi->parent->conn->dbLocksSTMT, NETDATA_MSSQL_ODBC_PREPARE, mdi->parent->instanceID);
        goto endlocks;
    }

    ret = SQLBindCol(mdi->parent->conn->dbLocksSTMT, 2, SQL_C_LONG, &value, sizeof(value), &col_value_len);
    if (likely(netdata_mssql_check_result(ret))) {
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

        if (col_object_len == SQL_NULL_DATA)
            continue;
        if (col_value_len == SQL_NULL_DATA)
            value = 0;

        char *space = strchr(resource_type, ' ');
        if (likely(space))
            *space = '\0';

        struct mssql_lock_instance *mli =
            dictionary_set(mdi->parent->locks_instances, resource_type, NULL, sizeof(*mli));
        if (unlikely(!mli))
            continue;

        mli->lockWait.current.Data = value;
    } while (true);

endlocks:
    netdata_MSSQL_release_results(mdi->parent->conn->dbLocksSTMT);
}

int dict_mssql_fill_waits(struct mssql_instance *mi)
{
    if (unlikely(!mi->conn->collect_waits))
        return 1;

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
    if (likely(netdata_mssql_check_result(ret))) {
        netdata_MSSQL_error(SQL_HANDLE_STMT, mi->conn->dbWaitsSTMT, NETDATA_MSSQL_ODBC_QUERY, mi->instanceID);
        goto endwait;
    }

    ret = SQLBindCol(mi->conn->dbWaitsSTMT, 1, SQL_C_CHAR, wait_type, sizeof(wait_type), &col_wait_type_len);
    if (likely(netdata_mssql_check_result(ret))) {
        netdata_MSSQL_error(SQL_HANDLE_STMT, mi->conn->dbWaitsSTMT, NETDATA_MSSQL_ODBC_PREPARE, mi->instanceID);
        goto endwait;
    }

    ret = SQLBindCol(mi->conn->dbWaitsSTMT, 2, SQL_C_SBIGINT, &total_wait, sizeof(total_wait), &col_total_wait_len);
    if (likely(netdata_mssql_check_result(ret))) {
        netdata_MSSQL_error(SQL_HANDLE_STMT, mi->conn->dbWaitsSTMT, NETDATA_MSSQL_ODBC_PREPARE, mi->instanceID);
        goto endwait;
    }

    ret =
        SQLBindCol(mi->conn->dbWaitsSTMT, 3, SQL_C_SBIGINT, &resource_wait, sizeof(resource_wait), &col_resource_wait_len);
    if (likely(netdata_mssql_check_result(ret))) {
        netdata_MSSQL_error(SQL_HANDLE_STMT, mi->conn->dbWaitsSTMT, NETDATA_MSSQL_ODBC_PREPARE, mi->instanceID);
        goto endwait;
    }

    ret = SQLBindCol(mi->conn->dbWaitsSTMT, 4, SQL_C_SBIGINT, &signal_wait, sizeof(signal_wait), &col_signal_wait_len);
    if (likely(netdata_mssql_check_result(ret))) {
        netdata_MSSQL_error(SQL_HANDLE_STMT, mi->conn->dbWaitsSTMT, NETDATA_MSSQL_ODBC_PREPARE, mi->instanceID);
        goto endwait;
    }

    ret = SQLBindCol(mi->conn->dbWaitsSTMT, 5, SQL_C_SBIGINT, &max_wait, sizeof(max_wait), &col_max_wait_len);
    if (likely(netdata_mssql_check_result(ret))) {
        netdata_MSSQL_error(SQL_HANDLE_STMT, mi->conn->dbWaitsSTMT, NETDATA_MSSQL_ODBC_PREPARE, mi->instanceID);
        goto endwait;
    }

    ret =
        SQLBindCol(mi->conn->dbWaitsSTMT, 6, SQL_C_SBIGINT, &waiting_tasks, sizeof(waiting_tasks), &col_waiting_tasks_len);
    if (likely(netdata_mssql_check_result(ret))) {
        netdata_MSSQL_error(SQL_HANDLE_STMT, mi->conn->dbWaitsSTMT, NETDATA_MSSQL_ODBC_PREPARE, mi->instanceID);
        goto endwait;
    }

    ret =
        SQLBindCol(mi->conn->dbWaitsSTMT, 7, SQL_C_CHAR, wait_category, sizeof(wait_category), &col_wait_category_len);
    if (likely(netdata_mssql_check_result(ret))) {
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

        if (col_wait_type_len == SQL_NULL_DATA)
            continue;
        if (col_total_wait_len == SQL_NULL_DATA)
            total_wait = 0;
        if (col_resource_wait_len == SQL_NULL_DATA)
            resource_wait = 0;
        if (col_signal_wait_len == SQL_NULL_DATA)
            signal_wait = 0;
        if (col_max_wait_len == SQL_NULL_DATA)
            max_wait = 0;
        if (col_waiting_tasks_len == SQL_NULL_DATA)
            waiting_tasks = 0;
        if (col_wait_category_len == SQL_NULL_DATA)
            wait_category[0] = '\0';

        struct mssql_db_waits *mdw = dictionary_set(mi->waits, wait_type, NULL, sizeof(*mdw));
        if (!mdw)
            continue;

        mdw->MSSQLDatabaseTotalWait.current.Data = (ULONGLONG)total_wait;
        if (unlikely(resource_wait < 0))
            resource_wait = 0;
        mdw->MSSQLDatabaseResourceWaitMSec.current.Data = (ULONGLONG)resource_wait;
        mdw->MSSQLDatabaseSignalWaitMSec.current.Data = (ULONGLONG)signal_wait;
        mdw->MSSQLDatabaseMaxWaitTimeMSec.current.Data = (ULONGLONG)max_wait;
        mdw->MSSQLDatabaseWaitingTasks.current.Data = (ULONGLONG)waiting_tasks;

        if (unlikely(!mdw->wait_category))
            mdw->wait_category = strdupz(wait_category);
    } while (true);

endwait:
    netdata_MSSQL_release_results(mi->conn->dbWaitsSTMT);

    return success;
}

int netdata_select_db(SQLHDBC hdbc, const char *database)
{
    SQLHSTMT hstmt;
    SQLRETURN ret;

    ret = SQLAllocHandle(SQL_HANDLE_STMT, hdbc, &hstmt);
    if (likely(netdata_mssql_check_result(ret))) {
        return -1;
    }

    char query[512];
    snprintfz(query, sizeof(query), "USE %s", database);
    ret = SQLExecDirect(hstmt, (SQLCHAR *)query, SQL_NTS);
    int result = 0;
    if (likely(netdata_mssql_check_result(ret))) {
        result = -1;
    }
    SQLFreeHandle(SQL_HANDLE_STMT, hstmt);
    return result;
}

void dict_mssql_fill_replication(struct mssql_db_instance *mdi)
{
    char publisher[NETDATA_MAX_INSTANCE_OBJECT + 1] = {};
    char publisher_db[NETDATA_MAX_INSTANCE_OBJECT + 1] = {};
    char publication[NETDATA_MAX_INSTANCE_OBJECT + 1] = {};
    char key[NETDATA_MAX_INSTANCE_OBJECT * 2 + 2];
    int type = 0, status = 0, warning = 0, avg_latency = 0, retention = 0, subscriptioncount = 0,
        runningdistagentcount = 0, average_runspeedPerf = 0;
    SQLLEN publisher_len = 0, publisherdb_len = 0, publication_len = 0, type_len = 0, status_len = 0, warning_len = 0,
           avg_latency_len = 0, retention_len = 0, subscriptioncount_len = 0, runningagentcount_len = 0,
           average_runspeedperf_len = 0;

    if (likely(netdata_select_db(mdi->parent->conn->netdataSQLHDBc, NETDATA_REPLICATION_DB))) {
        return;
    }

    SQLRETURN ret =
        SQLExecDirect(mdi->parent->conn->dbReplicationPublisher, (SQLCHAR *)NETDATA_REPLICATION_MONITOR_QUERY, SQL_NTS);
    if (likely(netdata_mssql_check_result(ret))) {
        mdi->collecting_data = false;
        netdata_MSSQL_error(
            SQL_HANDLE_STMT,
            mdi->parent->conn->dbReplicationPublisher,
            NETDATA_MSSQL_ODBC_QUERY,
            mdi->parent->instanceID);
        goto endreplication;
    }

    ret = SQLBindCol(
        mdi->parent->conn->dbReplicationPublisher, 1, SQL_C_CHAR, publisher_db, sizeof(publisher_db), &publisherdb_len);
    if (likely(netdata_mssql_check_result(ret))) {
        netdata_MSSQL_error(
            SQL_HANDLE_STMT,
            mdi->parent->conn->dbReplicationPublisher,
            NETDATA_MSSQL_ODBC_PREPARE,
            mdi->parent->instanceID);
        goto endreplication;
    }

    ret = SQLBindCol(
        mdi->parent->conn->dbReplicationPublisher, 2, SQL_C_CHAR, publication, sizeof(publication), &publication_len);
    if (likely(netdata_mssql_check_result(ret))) {
        netdata_MSSQL_error(
            SQL_HANDLE_STMT,
            mdi->parent->conn->dbReplicationPublisher,
            NETDATA_MSSQL_ODBC_PREPARE,
            mdi->parent->instanceID);
        goto endreplication;
    }

    ret = SQLBindCol(mdi->parent->conn->dbReplicationPublisher, 4, SQL_C_LONG, &type, sizeof(type), &type_len);
    if (likely(netdata_mssql_check_result(ret))) {
        netdata_MSSQL_error(
            SQL_HANDLE_STMT,
            mdi->parent->conn->dbReplicationPublisher,
            NETDATA_MSSQL_ODBC_PREPARE,
            mdi->parent->instanceID);
        goto endreplication;
    }

    ret = SQLBindCol(mdi->parent->conn->dbReplicationPublisher, 5, SQL_C_LONG, &status, sizeof(status), &status_len);
    if (likely(netdata_mssql_check_result(ret))) {
        netdata_MSSQL_error(
            SQL_HANDLE_STMT,
            mdi->parent->conn->dbReplicationPublisher,
            NETDATA_MSSQL_ODBC_PREPARE,
            mdi->parent->instanceID);
        goto endreplication;
    }

    ret = SQLBindCol(mdi->parent->conn->dbReplicationPublisher, 6, SQL_C_LONG, &warning, sizeof(warning), &warning_len);
    if (likely(netdata_mssql_check_result(ret))) {
        netdata_MSSQL_error(
            SQL_HANDLE_STMT,
            mdi->parent->conn->dbReplicationPublisher,
            NETDATA_MSSQL_ODBC_PREPARE,
            mdi->parent->instanceID);
        goto endreplication;
    }

    ret = SQLBindCol(
        mdi->parent->conn->dbReplicationPublisher, 9, SQL_C_LONG, &avg_latency, sizeof(avg_latency), &avg_latency_len);
    if (likely(netdata_mssql_check_result(ret))) {
        // It is still a NULL value
        avg_latency = 0;
    }

    ret = SQLBindCol(
        mdi->parent->conn->dbReplicationPublisher, 11, SQL_C_LONG, &retention, sizeof(retention), &retention_len);
    if (likely(netdata_mssql_check_result(ret))) {
        netdata_MSSQL_error(
            SQL_HANDLE_STMT,
            mdi->parent->conn->dbReplicationPublisher,
            NETDATA_MSSQL_ODBC_PREPARE,
            mdi->parent->instanceID);
        goto endreplication;
    }

    ret = SQLBindCol(
        mdi->parent->conn->dbReplicationPublisher,
        15,
        SQL_C_LONG,
        &subscriptioncount,
        sizeof(subscriptioncount),
        &subscriptioncount_len);
    if (likely(netdata_mssql_check_result(ret))) {
        netdata_MSSQL_error(
            SQL_HANDLE_STMT,
            mdi->parent->conn->dbReplicationPublisher,
            NETDATA_MSSQL_ODBC_PREPARE,
            mdi->parent->instanceID);
        goto endreplication;
    }

    ret = SQLBindCol(
        mdi->parent->conn->dbReplicationPublisher,
        16,
        SQL_C_LONG,
        &runningdistagentcount,
        sizeof(runningdistagentcount),
        &runningagentcount_len);
    if (likely(netdata_mssql_check_result(ret))) {
        netdata_MSSQL_error(
            SQL_HANDLE_STMT,
            mdi->parent->conn->dbReplicationPublisher,
            NETDATA_MSSQL_ODBC_PREPARE,
            mdi->parent->instanceID);
        goto endreplication;
    }

    ret = SQLBindCol(
        mdi->parent->conn->dbReplicationPublisher,
        22,
        SQL_C_LONG,
        &average_runspeedPerf,
        sizeof(average_runspeedPerf),
        &average_runspeedperf_len);
    if (likely(netdata_mssql_check_result(ret))) {
        netdata_MSSQL_error(
            SQL_HANDLE_STMT,
            mdi->parent->conn->dbReplicationPublisher,
            NETDATA_MSSQL_ODBC_PREPARE,
            mdi->parent->instanceID);
        goto endreplication;
    }

    ret = SQLBindCol(
        mdi->parent->conn->dbReplicationPublisher, 24, SQL_C_CHAR, publisher, sizeof(publisher), &publisher_len);
    if (likely(netdata_mssql_check_result(ret))) {
        netdata_MSSQL_error(
            SQL_HANDLE_STMT,
            mdi->parent->conn->dbReplicationPublisher,
            NETDATA_MSSQL_ODBC_PREPARE,
            mdi->parent->instanceID);
        goto endreplication;
    }

    do {
        ret = SQLFetch(mdi->parent->conn->dbReplicationPublisher);
        switch (ret) {
            case SQL_SUCCESS:
            case SQL_SUCCESS_WITH_INFO:
                break;
            case SQL_NO_DATA:
            default:
                goto endreplication;
        }

        if (publisherdb_len == SQL_NULL_DATA)
            publisher_db[0] = '\0';
        if (publication_len == SQL_NULL_DATA)
            publication[0] = '\0';
        if (type_len == SQL_NULL_DATA)
            type = 0;
        if (status_len == SQL_NULL_DATA)
            status = 0;
        if (warning_len == SQL_NULL_DATA)
            warning = 0;
        if (avg_latency_len == SQL_NULL_DATA)
            avg_latency = 0;
        if (retention_len == SQL_NULL_DATA)
            retention = 0;
        if (subscriptioncount_len == SQL_NULL_DATA)
            subscriptioncount = 0;
        if (runningagentcount_len == SQL_NULL_DATA)
            runningdistagentcount = 0;
        if (average_runspeedperf_len == SQL_NULL_DATA)
            average_runspeedPerf = 0;
        if (publisher_len == SQL_NULL_DATA)
            publisher[0] = '\0';

        if(unlikely(!publisher_db[0] || !publication[0]))
            continue;

        snprintfz(key, sizeof(key) - 1, "%s:%s", publisher_db, publication);
        struct mssql_publisher_publication *mpp =
            dictionary_set(mdi->parent->publisher_publication, key, NULL, sizeof(*mpp));

        if (unlikely(!mpp->publisher)) {
            mpp->publisher = strdupz(publisher);
            mpp->parent = mdi->parent;
        }

        if (unlikely(!mpp->publication))
            mpp->publication = strdupz(publication);

        if (unlikely(!mpp->db))
            mpp->db = strdupz(publisher_db);

        mpp->type = type;
        mpp->status = status;
        mpp->warning = warning;

        mpp->avg_latency = avg_latency;

        mpp->retention = retention;

        mpp->subscriptioncount = subscriptioncount;
        mpp->runningdistagentcount = runningdistagentcount;

        mpp->average_runspeedPerf = average_runspeedPerf;
    } while (true);

endreplication:
    (void)netdata_select_db(mdi->parent->conn->netdataSQLHDBc, "master");
    netdata_MSSQL_release_results(mdi->parent->conn->dbReplicationPublisher);
}
int dict_mssql_databases_run_queries(const DICTIONARY_ITEM *item __maybe_unused, void *value, void *data __maybe_unused)
{
    struct mssql_db_instance *mdi = value;
    const char *instance_name = data;
    const char *dbname = dictionary_acquired_item_name((DICTIONARY_ITEM *)item);

    if (unlikely(!mdi->collecting_data || !mdi->parent || !mdi->parent->conn)) {
        goto enddrunquery;
    }

    // We failed to collect this for the database, so we are not going to try again
    if (unlikely(mdi->MSSQLDatabaseDataFileSize.current.Data != ULONG_LONG_MAX)) {
        if (likely(mdi->parent->conn->collect_data_size))
            mdi->MSSQLDatabaseDataFileSize.current.Data = netdata_MSSQL_fill_long_value(
                mdi->parent->conn->dataFileSizeSTMT, NETDATA_QUERY_DATA_FILE_SIZE_MASK, dbname, mdi->parent->instanceID);
    } else {
        mdi->collecting_data = false;
        goto enddrunquery;
    }

    dict_mssql_fill_performance_counters(mdi, dbname, instance_name);
    dict_mssql_fill_locks(mdi, dbname);

    if (likely(mdi->running_replication && mdi->parent->conn->collect_replication))
        dict_mssql_fill_replication(mdi);

enddrunquery:
    return 1;
}

long netdata_mssql_check_permission(struct mssql_instance *mi)
{
    static int next_try = NETDATA_MSSQL_NEXT_TRY - 1;
    long perm = 0;
    SQLLEN col_data_len = 0;

    if (++next_try != NETDATA_MSSQL_NEXT_TRY)
        return 1;

    next_try = 0;

    SQLRETURN ret;

    ret = SQLExecDirect(mi->conn->checkPermSTMT, (SQLCHAR *)NETDATA_QUERY_CHECK_PERM, SQL_NTS);
    if (likely(netdata_mssql_check_result(ret))) {
        netdata_MSSQL_error(SQL_HANDLE_STMT, mi->conn->checkPermSTMT, NETDATA_MSSQL_ODBC_QUERY, mi->instanceID);
        perm = LONG_MAX;
        goto endperm;
    }

    ret = SQLBindCol(mi->conn->checkPermSTMT, 1, SQL_C_LONG, &perm, sizeof(perm), &col_data_len);

    if (likely(netdata_mssql_check_result(ret))) {
        netdata_MSSQL_error(SQL_HANDLE_STMT, mi->conn->checkPermSTMT, NETDATA_MSSQL_ODBC_PREPARE, mi->instanceID);
        perm = LONG_MAX;
        goto endperm;
    }

    ret = SQLFetch(mi->conn->checkPermSTMT);
    if (likely(netdata_mssql_check_result(ret))) {
        netdata_MSSQL_error(SQL_HANDLE_STMT, mi->conn->checkPermSTMT, NETDATA_MSSQL_ODBC_FETCH, mi->instanceID);
        perm = LONG_MAX;
        goto endperm;
    }

    if (col_data_len == SQL_NULL_DATA)
        perm = 0;

endperm:
    netdata_MSSQL_release_results(mi->conn->checkPermSTMT);
    return perm;
}

void netdata_mssql_fill_mssql_status(struct mssql_instance *mi)
{
    char dbname[SQLSERVER_MAX_NAME_LENGTH + 1];
    int readonly = 0;
    BYTE state = 0;
    SQLLEN col_data_len = 0, col_state_len = 0, col_readonly_len = 0;
    static int next_try = NETDATA_MSSQL_NEXT_TRY - 1;

    if (unlikely(++next_try != NETDATA_MSSQL_NEXT_TRY))
        return;

    next_try = 0;

    SQLRETURN ret;

    ret = SQLExecDirect(mi->conn->dbSQLState, (SQLCHAR *)NETDATA_QUERY_DATABASE_STATUS, SQL_NTS);
    if (likely(netdata_mssql_check_result(ret))) {
        netdata_MSSQL_error(SQL_HANDLE_STMT, mi->conn->dbSQLState, NETDATA_MSSQL_ODBC_QUERY, mi->instanceID);
        goto enddbstate;
    }

    ret = SQLBindCol(mi->conn->dbSQLState, 1, SQL_C_CHAR, dbname, sizeof(dbname), &col_data_len);
    if (likely(netdata_mssql_check_result(ret))) {
        netdata_MSSQL_error(SQL_HANDLE_STMT, mi->conn->dbSQLState, NETDATA_MSSQL_ODBC_PREPARE, mi->instanceID);
        goto enddbstate;
    }

    ret = SQLBindCol(mi->conn->dbSQLState, 2, SQL_C_TINYINT, &state, sizeof(state), &col_state_len);
    if (likely(netdata_mssql_check_result(ret))) {
        netdata_MSSQL_error(SQL_HANDLE_STMT, mi->conn->dbSQLState, NETDATA_MSSQL_ODBC_PREPARE, mi->instanceID);
        goto enddbstate;
    }

    ret = SQLBindCol(mi->conn->dbSQLState, 3, SQL_C_BIT, &readonly, sizeof(readonly), &col_readonly_len);
    if (likely(netdata_mssql_check_result(ret))) {
        netdata_MSSQL_error(SQL_HANDLE_STMT, mi->conn->dbSQLState, NETDATA_MSSQL_ODBC_PREPARE, mi->instanceID);
        goto enddbstate;
    }

    do {
        ret = SQLFetch(mi->conn->dbSQLState);
        if (likely(netdata_mssql_check_result(ret))) {
            goto enddbstate;
        }

        if (col_data_len == SQL_NULL_DATA)
            continue;
        if (col_state_len == SQL_NULL_DATA)
            state = 0;
        if (col_readonly_len == SQL_NULL_DATA)
            readonly = 0;

        struct mssql_db_instance *mdi = dictionary_set(mi->databases, dbname, NULL, sizeof(*mdi));
        if (unlikely(!mdi))
            continue;

        mdi->MSSQLDBState.current.Data = (ULONGLONG)state;
        mdi->MSSQLDBIsReadonly.current.Data = (ULONGLONG)readonly;
    } while (true);

enddbstate:
    netdata_MSSQL_release_results(mi->conn->dbSQLState);
}

void netdata_mssql_fill_job_status(struct mssql_instance *mi)
{
    char job[SQLSERVER_MAX_NAME_LENGTH + 1];
    BYTE state = 0;
    SQLLEN col_job_len = 0;
    SQLLEN col_state_len = 0;

    static int next_try = NETDATA_MSSQL_NEXT_TRY - 1;

    if (unlikely(++next_try != NETDATA_MSSQL_NEXT_TRY))
        return;

    next_try = 0;

    SQLRETURN ret;

    ret = SQLExecDirect(mi->conn->dbSQLJobs, (SQLCHAR *)NETDATA_QUERY_JOBS_STATUS, SQL_NTS);
    if (likely(netdata_mssql_check_result(ret))) {
        netdata_MSSQL_error(SQL_HANDLE_STMT, mi->conn->dbSQLJobs, NETDATA_MSSQL_ODBC_QUERY, mi->instanceID);
        goto enddbjobs;
    }

    ret = SQLBindCol(mi->conn->dbSQLJobs, 1, SQL_C_CHAR, job, sizeof(job), &col_job_len);
    if (likely(netdata_mssql_check_result(ret))) {
        netdata_MSSQL_error(SQL_HANDLE_STMT, mi->conn->dbSQLJobs, NETDATA_MSSQL_ODBC_PREPARE, mi->instanceID);
        goto enddbjobs;
    }

    ret = SQLBindCol(mi->conn->dbSQLJobs, 2, SQL_C_TINYINT, &state, sizeof(state), &col_state_len);
    if (likely(netdata_mssql_check_result(ret))) {
        netdata_MSSQL_error(SQL_HANDLE_STMT, mi->conn->dbSQLJobs, NETDATA_MSSQL_ODBC_PREPARE, mi->instanceID);
        goto enddbjobs;
    }

    do {
        ret = SQLFetch(mi->conn->dbSQLJobs);
        if (likely(netdata_mssql_check_result(ret))) {
            goto enddbjobs;
        }

        if (col_job_len == SQL_NULL_DATA)
            continue;
        if (col_state_len == SQL_NULL_DATA)
            state = 0;

        struct mssql_db_jobs *mdj = dictionary_set(mi->sysjobs, job, NULL, sizeof(*mdj));
        if (unlikely(!mdj))
            continue;

        mdj->MSSQLJOBState.current.Data = (ULONGLONG)state;
    } while (true);

enddbjobs:
    netdata_MSSQL_release_results(mi->conn->dbSQLJobs);
}

void netdata_mssql_fill_user_connection(struct mssql_instance *mi)
{
    if (unlikely(!mi->conn->collect_user_connections))
        return;

    mi->MSSQLUserConnections.current.Data = 0;
    mi->MSSQLSessionConnections.current.Data = 0;

    collected_number connections = 0;
    unsigned char is_user;
    SQLLEN col_user_connections_len = 0;
    SQLLEN col_user_bit_len = 0;

    SQLRETURN ret;

    ret = SQLExecDirect(mi->conn->dbSQLConnections, (SQLCHAR *)NETDATA_QUERY_CONNECTIONS, SQL_NTS);
    if (likely(netdata_mssql_check_result(ret))) {
        netdata_MSSQL_error(SQL_HANDLE_STMT, mi->conn->dbSQLConnections, NETDATA_MSSQL_ODBC_QUERY, mi->instanceID);
        goto enduserconn;
    }

    ret = SQLBindCol(
        mi->conn->dbSQLConnections, 1, SQL_C_LONG, &connections, sizeof(connections), &col_user_connections_len);
    if (likely(netdata_mssql_check_result(ret))) {
        netdata_MSSQL_error(SQL_HANDLE_STMT, mi->conn->dbSQLConnections, NETDATA_MSSQL_ODBC_PREPARE, mi->instanceID);
        goto enduserconn;
    }

    ret = SQLBindCol(mi->conn->dbSQLConnections, 2, SQL_C_BIT, &is_user, sizeof(is_user), &col_user_bit_len);
    if (likely(netdata_mssql_check_result(ret))) {
        netdata_MSSQL_error(SQL_HANDLE_STMT, mi->conn->dbSQLConnections, NETDATA_MSSQL_ODBC_PREPARE, mi->instanceID);
        goto enduserconn;
    }

    do {
        ret = SQLFetch(mi->conn->dbSQLConnections);
        if (likely(netdata_mssql_check_result(ret))) {
            goto enduserconn;
        }

        if (col_user_connections_len == SQL_NULL_DATA)
            connections = 0;
        if (col_user_bit_len == SQL_NULL_DATA)
            is_user = 0;

        if (is_user)
            mi->MSSQLUserConnections.current.Data = (ULONGLONG)connections;
        else
            mi->MSSQLSessionConnections.current.Data = (ULONGLONG)connections;
    } while (true);

enduserconn:
    netdata_MSSQL_release_results(mi->conn->dbSQLConnections);
}

static void netdata_mssql_fill_blocked_processes_query(struct mssql_instance *mi)
{
    if (unlikely(!mi || !mi->conn || mi->conn->dbSQLBlockedProcesses == SQL_NULL_HSTMT))
        return;

    long blocked_processes = 0;
    SQLLEN col_len = 0;
    mi->MSSQLBlockedProcesses.current.Data = 0;

    SQLRETURN ret = SQLExecDirect(mi->conn->dbSQLBlockedProcesses, (SQLCHAR *)NETDATA_QUERY_BLOCKED_PROCESSES, SQL_NTS);
    if (likely(netdata_mssql_check_result(ret))) {
        netdata_MSSQL_error(SQL_HANDLE_STMT, mi->conn->dbSQLBlockedProcesses, NETDATA_MSSQL_ODBC_QUERY, mi->instanceID);
        goto end_blocked_processes;
    }

    ret = SQLBindCol(
            mi->conn->dbSQLBlockedProcesses,
            1,
            SQL_C_LONG,
            &blocked_processes,
            sizeof(blocked_processes),
            &col_len);
    if (likely(netdata_mssql_check_result(ret))) {
        netdata_MSSQL_error(SQL_HANDLE_STMT, mi->conn->dbSQLBlockedProcesses, NETDATA_MSSQL_ODBC_PREPARE, mi->instanceID);
        goto end_blocked_processes;
    }

    ret = SQLFetch(mi->conn->dbSQLBlockedProcesses);
    if (likely(netdata_mssql_check_result(ret)))
        goto end_blocked_processes;

    mi->MSSQLBlockedProcesses.current.Data = blocked_processes;
end_blocked_processes:
    netdata_MSSQL_release_results(mi->conn->dbSQLBlockedProcesses);
}

void netdata_mssql_fill_dictionary_from_db(struct mssql_instance *mi)
{
    char dbname[SQLSERVER_MAX_NAME_LENGTH + 1];
    SQLLEN col_data_len = 0;

    static int next_try = NETDATA_MSSQL_NEXT_TRY - 1;

    if (unlikely(++next_try != NETDATA_MSSQL_NEXT_TRY))
        return;

    next_try = 0;

    SQLRETURN ret;

    ret = SQLExecDirect(mi->conn->databaseListSTMT, (SQLCHAR *)NETDATA_QUERY_LIST_DB, SQL_NTS);
    if (likely(netdata_mssql_check_result(ret))) {
        netdata_MSSQL_error(SQL_HANDLE_STMT, mi->conn->databaseListSTMT, NETDATA_MSSQL_ODBC_QUERY, mi->instanceID);
        goto enddblist;
    }

    ret = SQLBindCol(mi->conn->databaseListSTMT, 1, SQL_C_CHAR, dbname, sizeof(dbname), &col_data_len);

    if (likely(netdata_mssql_check_result(ret))) {
        netdata_MSSQL_error(SQL_HANDLE_STMT, mi->conn->databaseListSTMT, NETDATA_MSSQL_ODBC_PREPARE, mi->instanceID);
        goto enddblist;
    }

    int i = 0;
    do {
        ret = SQLFetch(mi->conn->databaseListSTMT);
        if (likely(netdata_mssql_check_result(ret))) {
            goto enddblist;
        }

        if (col_data_len == SQL_NULL_DATA)
            continue;

        struct mssql_db_instance *mdi = dictionary_set(mi->databases, dbname, NULL, sizeof(*mdi));
        if (!mdi)
            continue;

        mdi->updated = 0;
        if (unlikely(!mdi->parent)) {
            mdi->parent = mi;
        }

        if (mi->conn && !strncmp(dbname, NETDATA_REPLICATION_DB, sizeof(NETDATA_REPLICATION_DB) - 1)) {
            mdi->running_replication = true;
        }

        if (unlikely(!i)) {
            mdi->collect_instance = true;
        }
        i++;
    } while (true);

enddblist:
    netdata_MSSQL_release_results(mi->conn->databaseListSTMT);
}

static bool netdata_MSSQL_initialize_connection(struct netdata_mssql_conn *nmc)
{
    SQLRETURN ret;
    if (unlikely(!nmc->netdataSQLEnv)) {
        ret = SQLAllocHandle(SQL_HANDLE_ENV, SQL_NULL_HANDLE, &nmc->netdataSQLEnv);
        if (likely(netdata_mssql_check_result(ret)))
            return FALSE;

        ret = SQLSetEnvAttr(nmc->netdataSQLEnv, SQL_ATTR_ODBC_VERSION, (SQLPOINTER)SQL_OV_ODBC3, 0);
        if (likely(netdata_mssql_check_result(ret))) {
            return FALSE;
        }
    }

    ret = SQLAllocHandle(SQL_HANDLE_DBC, nmc->netdataSQLEnv, &nmc->netdataSQLHDBc);
    if (likely(netdata_mssql_check_result(ret))) {
        return FALSE;
    }

    ret = SQLSetConnectAttr(nmc->netdataSQLHDBc, SQL_LOGIN_TIMEOUT, (SQLPOINTER)5, 0);
    if (likely(netdata_mssql_check_result(ret))) {
        return FALSE;
    }

    ret = SQLSetConnectAttr(nmc->netdataSQLHDBc, SQL_ATTR_AUTOCOMMIT, (SQLPOINTER)TRUE, 0);
    if (likely(netdata_mssql_check_result(ret))) {
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

    if (likely(retConn)) {
        SQLSetConnectAttr(nmc->netdataSQLHDBc, SQL_ATTR_AUTOCOMMIT, (SQLPOINTER)SQL_AUTOCOMMIT_ON, 0);

        ret = SQLAllocHandle(SQL_HANDLE_STMT, nmc->netdataSQLHDBc, &nmc->checkPermSTMT);
        if (likely(netdata_mssql_check_result(ret))) {
            retConn = FALSE;
            goto endMSSQLInitializationConnection;
        }

        ret = SQLAllocHandle(SQL_HANDLE_STMT, nmc->netdataSQLHDBc, &nmc->databaseListSTMT);
        if (likely(netdata_mssql_check_result(ret))) {
            retConn = FALSE;
            goto endMSSQLInitializationConnection;
        }

        ret = SQLAllocHandle(SQL_HANDLE_STMT, nmc->netdataSQLHDBc, &nmc->dataFileSizeSTMT);
        if (likely(netdata_mssql_check_result(ret))) {
            retConn = FALSE;
            goto endMSSQLInitializationConnection;
        }

        ret = SQLAllocHandle(SQL_HANDLE_STMT, nmc->netdataSQLHDBc, &nmc->dbPerfCounterSTMT);
        if (likely(netdata_mssql_check_result(ret))) {
            retConn = FALSE;
            goto endMSSQLInitializationConnection;
        }

        ret = SQLAllocHandle(SQL_HANDLE_STMT, nmc->netdataSQLHDBc, &nmc->dbInstanceTransactionSTMT);
        if (likely(netdata_mssql_check_result(ret))) {
            retConn = FALSE;
            goto endMSSQLInitializationConnection;
        }

        ret = SQLAllocHandle(SQL_HANDLE_STMT, nmc->netdataSQLHDBc, &nmc->dbLocksSTMT);
        if (likely(netdata_mssql_check_result(ret))) {
            retConn = FALSE;
            goto endMSSQLInitializationConnection;
        }

        ret = SQLAllocHandle(SQL_HANDLE_STMT, nmc->netdataSQLHDBc, &nmc->dbWaitsSTMT);
        if (likely(netdata_mssql_check_result(ret))) {
            retConn = FALSE;
            goto endMSSQLInitializationConnection;
        }

        ret = SQLAllocHandle(SQL_HANDLE_STMT, nmc->netdataSQLHDBc, &nmc->dbSQLState);
        if (likely(netdata_mssql_check_result(ret))) {
            retConn = FALSE;
            goto endMSSQLInitializationConnection;
        }

        ret = SQLAllocHandle(SQL_HANDLE_STMT, nmc->netdataSQLHDBc, &nmc->dbSQLJobs);
        if (likely(netdata_mssql_check_result(ret))) {
            retConn = FALSE;
            goto endMSSQLInitializationConnection;
        }

        ret = SQLAllocHandle(SQL_HANDLE_STMT, nmc->netdataSQLHDBc, &nmc->dbSQLConnections);
        if (likely(netdata_mssql_check_result(ret))) {
            retConn = FALSE;
            goto endMSSQLInitializationConnection;
        }

        ret = SQLAllocHandle(SQL_HANDLE_STMT, nmc->netdataSQLHDBc, &nmc->dbSQLBlockedProcesses);
        if (likely(netdata_mssql_check_result(ret))) {
            retConn = FALSE;
            goto endMSSQLInitializationConnection;
        }

        ret = SQLAllocHandle(SQL_HANDLE_STMT, nmc->netdataSQLHDBc, &nmc->dbReplicationPublisher);
        if (likely(netdata_mssql_check_result(ret))) {
            retConn = FALSE;
            goto endMSSQLInitializationConnection;
        }
    }

endMSSQLInitializationConnection:
    return retConn;
}

// Dictionary
static DICTIONARY *mssql_instances = NULL;

static void initialize_mssql_objects(struct mssql_instance *mi, const char *instance)
{
    char prefix[NETDATA_MAX_INSTANCE_NAME];
    if (unlikely(!strcmp(instance, "MSSQLSERVER"))) {
        strncpyz(prefix, "SQLServer:", sizeof(prefix) - 1);
    } else if (unlikely(!strcmp(instance, "SQLEXPRESS"))) {
        strncpyz(prefix, "MSSQL$SQLEXPRESS:", sizeof(prefix) - 1);
        if (mi->conn)
            mi->conn->is_sqlexpress = true;
    } else {
        char *express = (mi->conn && mi->conn->is_sqlexpress) ? "SQLEXPRESS:" : "";
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

    strncpyz(&name[length], "SystemJobs", sizeof(name) - length);
    mi->objectName[NETDATA_MSSQL_JOBS] = strdupz(name);

    mi->objectName[NETDATA_USER_CONNECTIONS] = NULL;

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

void dict_mssql_insert_replication_cb(const DICTIONARY_ITEM *item __maybe_unused, void *value, void *data __maybe_unused)
{
    UNUSED(value);
}

void dict_mssql_insert_jobs_cb(const DICTIONARY_ITEM *item __maybe_unused, void *value, void *data __maybe_unused)
{
    UNUSED(value);
}

// Options
void netdata_mount_mssql_connection_string(struct netdata_mssql_conn *dbInput)
{
    SQLCHAR conn[1024];
    const char *serverAddress;
    const char *serverAddressArg;
    if (unlikely(!dbInput)) {
        return;
    }

    char auth[512];
    if (likely(dbInput->server && dbInput->address)) {
        nd_log(
            NDLS_COLLECTORS,
            NDLP_ERR,
            "Collector is not expecting server and address defined together, please, select one of them.");
        dbInput->connectionString = NULL;
        return;
    }

    if (likely(dbInput->server)) {
        serverAddress = "Server";
        serverAddressArg = dbInput->server;
    } else {
        serverAddressArg = "Address";
        serverAddress = dbInput->address;
    }

    if (likely(dbInput->windows_auth))
        snprintfz(auth, sizeof(auth) - 1, "Trusted_Connection = yes");
    else if (unlikely(!dbInput->username || !dbInput->password)) {
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
        if (likely(total_instances)) {
            snprintfz(&section_name[sizeof(NETDATA_DEFAULT_MSSQL_SECTION) - 1], 5, "%d", total_instances);
        }
        const char *instance = inicfg_get(&netdata_config, section_name, "instance", NULL);
        int additional_instances = (int)inicfg_get_number(&netdata_config, section_name, "additional instances", 0);
        if (!instance || strlen(instance) > NETDATA_MAX_INSTANCE_OBJECT) {
            nd_log(
                NDLS_COLLECTORS,
                NDLP_ERR,
                "You must specify a valid 'instance' name to collect data from database in section %s.",
                section_name);
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
        dbconn->collect_transactions = inicfg_get_boolean(&netdata_config, section_name, "collect transactions", true);
        dbconn->collect_waits = inicfg_get_boolean(&netdata_config, section_name, "collect waits", true);
        dbconn->collect_locks = inicfg_get_boolean(&netdata_config, section_name, "collect lock metrics", true);
        dbconn->collect_replication = inicfg_get_boolean(&netdata_config, section_name, "collect replication", true);
        dbconn->collect_jobs = inicfg_get_boolean(&netdata_config, section_name, "collect jobs", true);
        dbconn->collect_buffer = inicfg_get_boolean(&netdata_config, section_name, "collect buffer stats", true);
        dbconn->collect_data_size = inicfg_get_boolean(&netdata_config, section_name, "collect database size", true);
        dbconn->collect_user_connections =
            inicfg_get_boolean(&netdata_config, section_name, "collect user connections", true);
        dbconn->collect_blocked_processes = inicfg_get_boolean(
                &netdata_config, section_name, "collect blocked processes", true);
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

    if (unlikely(!mi->locks_instances)) {
        mi->locks_instances = dictionary_create_advanced(
            DICT_OPTION_DONT_OVERWRITE_VALUE | DICT_OPTION_FIXED_SIZE, NULL, sizeof(struct mssql_lock_instance));
        dictionary_register_insert_callback(mi->locks_instances, dict_mssql_insert_locks_cb, NULL);
        mssql_fill_initial_instances(mi);
    }


    if (unlikely(!mi->databases)) {
        mi->databases = dictionary_create_advanced(
            DICT_OPTION_DONT_OVERWRITE_VALUE | DICT_OPTION_FIXED_SIZE, NULL, sizeof(struct mssql_db_instance));
        dictionary_register_insert_callback(mi->databases, dict_mssql_insert_databases_cb, NULL);
    }

    if (unlikely(!mi->publisher_publication)) {
        mi->publisher_publication = dictionary_create_advanced(
            DICT_OPTION_DONT_OVERWRITE_VALUE | DICT_OPTION_FIXED_SIZE,
            NULL,
            sizeof(struct mssql_publisher_publication));
        dictionary_register_insert_callback(mi->publisher_publication, dict_mssql_insert_replication_cb, NULL);
    }

    if (unlikely(!mi->sysjobs)) {
        mi->sysjobs = dictionary_create_advanced(
            DICT_OPTION_DONT_OVERWRITE_VALUE | DICT_OPTION_FIXED_SIZE, NULL, sizeof(struct mssql_db_jobs));
        dictionary_register_insert_callback(mi->sysjobs, dict_mssql_insert_jobs_cb, NULL);
    }

    if (unlikely(!mi->waits)) {
        mi->waits = dictionary_create_advanced(
            DICT_OPTION_DONT_OVERWRITE_VALUE | DICT_OPTION_FIXED_SIZE, NULL, sizeof(struct mssql_db_waits));
        dictionary_register_insert_callback(mi->waits, dict_mssql_insert_wait_cb, NULL);
    }

    initialize_mssql_objects(mi, instance);
    initialize_mssql_keys(mi);
    mi->conn = netdata_mssql_get_conn_option(instance);

    if (likely(mi->conn && mi->conn->connectionString)) {
        mi->conn->is_connected = netdata_MSSQL_initialize_connection(mi->conn);
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
    if (likely(ret != ERROR_SUCCESS)) {
        goto endMSSQLFillDict;
    }

    if (unlikely(!values)) {
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
        if (likely(ret != ERROR_SUCCESS))
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
    const char *instance_name = dictionary_acquired_item_name((DICTIONARY_ITEM *)item);
    static long collecting = 1;

    if (likely(mi->conn && mi->conn->is_connected && collecting)) {
        collecting = netdata_mssql_check_permission(mi);
        if (!collecting) {
            nd_log(
                NDLS_COLLECTORS,
                NDLP_ERR,
                "User %s does not have permission to run queries on %s",
                mi->conn->username,
                mi->instanceID);
        } else {
            netdata_mssql_fill_dictionary_from_db(mi);
            netdata_mssql_fill_mssql_status(mi);
            netdata_mssql_fill_job_status(mi);
            netdata_mssql_fill_user_connection(mi);
            if (likely(mi->conn->collect_blocked_processes))
                netdata_mssql_fill_blocked_processes_query(mi);
            dictionary_sorted_walkthrough_read(mi->databases, dict_mssql_databases_run_queries, (void *)instance_name);
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

    if (likely(create_thread)) {
        mssql_queries_thread =
            nd_thread_create("mssql_queries", NETDATA_THREAD_OPTION_DEFAULT, netdata_mssql_queries, &update_every);
    }

    return 0;
}

// Charts
void netdata_mssql_blocked_processes_chart(struct mssql_instance *mi, int update_every)
{
    if (unlikely(!mi->st_process_blocked)) {
        char id[RRD_ID_LENGTH_MAX + 1];
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

static void do_mssql_blocked_processes(PERF_DATA_BLOCK *pDataBlock __maybe_unused, struct mssql_instance *mi, int update_every)
{
    if (unlikely(!mi || !mi->conn || !mi->conn->collect_blocked_processes))
        return;

    netdata_mssql_blocked_processes_chart(mi, update_every);
}

void dict_mssql_locks_wait_charts(struct mssql_instance *mi, struct mssql_lock_instance *mli, const char *resource)
{
    if (unlikely(!mli->st_lockWait)) {
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
    if (unlikely(!mli->st_deadLocks)) {
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
    if (unlikely(!pDataBlock))
        goto end_mssql_locks;

    PERF_OBJECT_TYPE *pObjectType = perflibFindObjectTypeByName(pDataBlock, mi->objectName[NETDATA_MSSQL_LOCKS]);
    if (likely(pObjectType)) {
        if (likely(pObjectType->NumInstances)) {
            PERF_INSTANCE_DEFINITION *pi = NULL;
            for (LONG i = 0; i < pObjectType->NumInstances; i++) {
                pi = perflibForEachInstance(pDataBlock, pObjectType, pi);
                if (unlikely(!pi))
                    break;

                if (unlikely(!getInstanceName(
                        pDataBlock, pObjectType, pi, windows_shared_buffer, sizeof(windows_shared_buffer))))
                    strncpyz(windows_shared_buffer, "[unknown]", sizeof(windows_shared_buffer) - 1);

                if (unlikely(!strcasecmp(windows_shared_buffer, "_Total")))
                    continue;

                struct mssql_lock_instance *mli =
                    dictionary_set(mi->locks_instances, windows_shared_buffer, NULL, sizeof(*mli));
                if (unlikely(!mli))
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
    if (unlikely(!mdw->st_total_wait)) {
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
    if (unlikely(!mdw->st_resource_wait_msec)) {
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
    if (unlikely(!mdw->st_signal_wait_msec)) {
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
    if (unlikely(!mdw->st_max_wait_time_msec)) {
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
    if (unlikely(!mdw->st_waiting_tasks)) {
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
    if (unlikely(!mi->conn->collect_waits))
        return;

    dictionary_sorted_walkthrough_read(mi->waits, dict_mssql_waits_charts_cb, mi);
}

void mssql_buffman_iops_chart(struct mssql_db_instance *mdi, struct mssql_instance *mi)
{
    if (unlikely(!mdi->st_buff_page_iops)) {
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
    if (unlikely(!mdi->st_buff_cache_hits)) {
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

        mdi->rd_buff_cache_hits = rrddim_add(mdi->st_buff_cache_hits, "hit_ratio", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);

        rrdlabels_add(mdi->st_buff_cache_hits->rrdlabels, "mssql_instance", mi->instanceID, RRDLABEL_SRC_AUTO);
    }

    rrddim_set_by_pointer(
        mdi->st_buff_cache_hits, mdi->rd_buff_cache_hits, (collected_number)mdi->MSSQLBufferCacheHits.current.Data);
    rrdset_done(mdi->st_buff_cache_hits);
}

void mssql_buffman_checkpoints_pages_chart(struct mssql_db_instance *mdi, struct mssql_instance *mi)
{
    if (unlikely(!mdi->st_buff_checkpoint_pages)) {
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
    if (unlikely(!mdi->st_buff_cache_page_life_expectancy)) {
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

        mdi->rd_buff_cache_page_life_expectancy =
            rrddim_add(mdi->st_buff_cache_page_life_expectancy, "life_expectancy", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);

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
    if (unlikely(!mdi->st_buff_lazy_write)) {
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
            "writes/s",
            PLUGIN_WINDOWS_NAME,
            "PerflibMSSQL",
            PRIO_MSSQL_BUFF_LAZY_WRITE,
            mi->update_every,
            RRDSET_TYPE_LINE);

        mdi->rd_buff_lazy_write = rrddim_add(mdi->st_buff_lazy_write, "writes", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);

        rrdlabels_add(mdi->st_buff_lazy_write->rrdlabels, "mssql_instance", mi->instanceID, RRDLABEL_SRC_AUTO);
    }

    rrddim_set_by_pointer(
        mdi->st_buff_lazy_write, mdi->rd_buff_lazy_write, (collected_number)mdi->MSSQLBufferLazyWrite.current.Data);
    rrdset_done(mdi->st_buff_lazy_write);
}

void mssql_buffman_page_lookups_chart(struct mssql_db_instance *mdi, struct mssql_instance *mi)
{
    if (unlikely(!mdi->st_buff_page_lookups)) {
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
            "lookups/s",
            PLUGIN_WINDOWS_NAME,
            "PerflibMSSQL",
            PRIO_MSSQL_BUFF_PAGE_LOOKUPS,
            mi->update_every,
            RRDSET_TYPE_LINE);

        mdi->rd_buff_page_lookups =
            rrddim_add(mdi->st_buff_page_lookups, "lookups", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);

        rrdlabels_add(mdi->st_buff_page_lookups->rrdlabels, "mssql_instance", mi->instanceID, RRDLABEL_SRC_AUTO);
    }

    rrddim_set_by_pointer(
        mdi->st_buff_page_lookups,
        mdi->rd_buff_page_lookups,
        (collected_number)mdi->MSSQLBufferPageLookups.current.Data);
    rrdset_done(mdi->st_buff_page_lookups);
}

static void netdata_mssql_compilations(struct mssql_db_instance *mdi, struct mssql_instance *mi)
{
    if (unlikely(!mdi->st_stats_compilation)) {
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
        mdi->st_stats_compilation, mdi->rd_stats_compilation, (collected_number)mdi->MSSQLCompilations.current.Data);
    rrdset_done(mdi->st_stats_compilation);
}

static void netdata_mssql_recompilations(struct mssql_db_instance *mdi, struct mssql_instance *mi)
{
    if (unlikely(!mdi->st_stats_recompiles)) {
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
            "recompiles/s",
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

int dict_mssql_buffman_stats_charts_cb(
    const DICTIONARY_ITEM *item __maybe_unused,
    void *value,
    void *data __maybe_unused)
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
    if (unlikely(!mi->conn->collect_buffer))
        return;

    dictionary_sorted_walkthrough_read(mi->databases, dict_mssql_buffman_stats_charts_cb, mi);
}

static void netdata_mssql_jobs_status(struct mssql_db_jobs *mdj, struct mssql_instance *mi, const char *job)
{
    if (unlikely(!mdj->st_status)) {
        char id[RRD_ID_LENGTH_MAX + 1];
        snprintfz(id, RRD_ID_LENGTH_MAX, "job_%s_instance_%s_status", job, mi->instanceID);
        netdata_fix_chart_name(id);
        mdj->st_status = rrdset_create_localhost(
            "mssql",
            id,
            NULL,
            "jobs",
            "mssql.instance_jobs_status",
            "Jobs running",
            "status",
            PLUGIN_WINDOWS_NAME,
            "PerflibMSSQL",
            PRIO_MSSQL_DATABASE_JOBS_STATUS,
            mi->update_every,
            RRDSET_TYPE_LINE);

        mdj->rd_status_enabled = rrddim_add(mdj->st_status, "enabled", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);

        mdj->rd_status_disabled = rrddim_add(mdj->st_status, "disabled", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);

        rrdlabels_add(mdj->st_status->rrdlabels, "mssql_instance", mi->instanceID, RRDLABEL_SRC_AUTO);
        rrdlabels_add(mdj->st_status->rrdlabels, "job_name", job, RRDLABEL_SRC_AUTO);
    }

    rrddim_set_by_pointer(
        mdj->st_status, mdj->rd_status_enabled, (collected_number)(mdj->MSSQLJOBState.current.Data == 1));
    rrddim_set_by_pointer(
        mdj->st_status, mdj->rd_status_disabled, (collected_number)(mdj->MSSQLJOBState.current.Data == 0));
    rrdset_done(mdj->st_status);
}

int dict_mssql_sysjobs_chart_cb(const DICTIONARY_ITEM *item __maybe_unused, void *value, void *data __maybe_unused)
{
    const char *job = dictionary_acquired_item_name((DICTIONARY_ITEM *)item);
    struct mssql_db_jobs *mdj = value;
    struct mssql_instance *mi = data;

    netdata_mssql_jobs_status(mdj, mi, job);
}

static void do_mssql_job_status_sql(PERF_DATA_BLOCK *pDataBlock, struct mssql_instance *mi, int update_every)
{
    if (unlikely(!mi->conn->collect_jobs))
        return;

    dictionary_sorted_walkthrough_read(mi->sysjobs, dict_mssql_sysjobs_chart_cb, mi);
}

static void do_mssql_user_connection(PERF_DATA_BLOCK *pDataBlock, struct mssql_instance *mi, int update_every)
{
    if (unlikely(!mi->conn->collect_user_connections))
        return;

    do_mssql_user_connections(mi, update_every);
    do_mssql_sessions_connections(mi, update_every);
}

void dict_mssql_replication_status(struct mssql_publisher_publication *mpp, int update_every)
{
    if (unlikely(!mpp->st_publisher_status)) {
        char id[RRD_ID_LENGTH_MAX + 1];

        snprintfz(
            id,
            RRD_ID_LENGTH_MAX,
            "instance_%s_replication_%s_%s_status",
            mpp->parent->instanceID,
            mpp->publication,
            mpp->db);
        netdata_fix_chart_name(id);
        mpp->st_publisher_status = rrdset_create_localhost(
            "mssql",
            id,
            NULL,
            "replication",
            "mssql.replication_status",
            "Current replication status",
            "status",
            PLUGIN_WINDOWS_NAME,
            "PerflibMSSQL",
            PRIO_MSSQL_REPLICATION_STATUS,
            update_every,
            RRDSET_TYPE_LINE);

        rrdlabels_add(
            mpp->st_publisher_status->rrdlabels, "mssql_instance", mpp->parent->instanceID, RRDLABEL_SRC_AUTO);
        rrdlabels_add(mpp->st_publisher_status->rrdlabels, "publisher", mpp->publisher, RRDLABEL_SRC_AUTO);
        rrdlabels_add(mpp->st_publisher_status->rrdlabels, "database", mpp->db, RRDLABEL_SRC_AUTO);
        rrdlabels_add(mpp->st_publisher_status->rrdlabels, "publication", mpp->publication, RRDLABEL_SRC_AUTO);

        mpp->rd_publisher_status_started =
            rrddim_add(mpp->st_publisher_status, "started", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
        mpp->rd_publisher_status_successed =
            rrddim_add(mpp->st_publisher_status, "succeeded", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
        mpp->rd_publisher_status_in_progress =
            rrddim_add(mpp->st_publisher_status, "in_progress", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
        mpp->rd_publisher_status_idle =
            rrddim_add(mpp->st_publisher_status, "idle", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
        mpp->rd_publisher_status_retrying =
            rrddim_add(mpp->st_publisher_status, "retrying", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
        mpp->rd_publisher_status_failed =
            rrddim_add(mpp->st_publisher_status, "failed", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
    }

    int status = mpp->status;
    rrddim_set_by_pointer(mpp->st_publisher_status, mpp->rd_publisher_status_started, (collected_number)(status == 1));
    rrddim_set_by_pointer(
        mpp->st_publisher_status, mpp->rd_publisher_status_successed, (collected_number)(status == 2));
    rrddim_set_by_pointer(
        mpp->st_publisher_status, mpp->rd_publisher_status_in_progress, (collected_number)(status == 3));
    rrddim_set_by_pointer(mpp->st_publisher_status, mpp->rd_publisher_status_idle, (collected_number)(status == 4));
    rrddim_set_by_pointer(mpp->st_publisher_status, mpp->rd_publisher_status_retrying, (collected_number)(status == 5));
    rrddim_set_by_pointer(mpp->st_publisher_status, mpp->rd_publisher_status_failed, (collected_number)(status == 6));
    rrdset_done(mpp->st_publisher_status);
}

void dict_mssql_replication_warning(struct mssql_publisher_publication *mpp, int update_every)
{
    if (unlikely(!mpp->st_warning)) {
        char id[RRD_ID_LENGTH_MAX + 1];

        snprintfz(
            id,
            RRD_ID_LENGTH_MAX,
            "instance_%s_replication_%s_%s_warning",
            mpp->parent->instanceID,
            mpp->publication,
            mpp->db);
        netdata_fix_chart_name(id);
        mpp->st_warning = rrdset_create_localhost(
            "mssql",
            id,
            NULL,
            "replication",
            "mssql.replication_warning",
            "Maximum threshold warning.",
            "status",
            PLUGIN_WINDOWS_NAME,
            "PerflibMSSQL",
            PRIO_MSSQL_REPLICATION_WARNING,
            update_every,
            RRDSET_TYPE_LINE);

        rrdlabels_add(mpp->st_warning->rrdlabels, "mssql_instance", mpp->parent->instanceID, RRDLABEL_SRC_AUTO);
        rrdlabels_add(mpp->st_warning->rrdlabels, "publisher", mpp->publisher, RRDLABEL_SRC_AUTO);
        rrdlabels_add(mpp->st_warning->rrdlabels, "database", mpp->db, RRDLABEL_SRC_AUTO);
        rrdlabels_add(mpp->st_warning->rrdlabels, "publication", mpp->publication, RRDLABEL_SRC_AUTO);

        mpp->rd_warning_expiration = rrddim_add(mpp->st_warning, "expiration", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
        mpp->rd_warning_latency = rrddim_add(mpp->st_warning, "latency", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
        mpp->rd_warning_mergeeexpiration =
            rrddim_add(mpp->st_warning, "merge expiration", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
        mpp->rd_warning_mergefastduration =
            rrddim_add(mpp->st_warning, "fast duration", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
        mpp->rd_warning_mergelowduration =
            rrddim_add(mpp->st_warning, "low duration", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
        mpp->rd_warning_mergefastrunspeed =
            rrddim_add(mpp->st_warning, "fast run speed", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
        mpp->rd_warning_mergelowrunspeed =
            rrddim_add(mpp->st_warning, "low run speed", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
    }

    int warning = mpp->warning;
    rrddim_set_by_pointer(
        mpp->st_warning, mpp->rd_warning_expiration, (collected_number)(warning & MSSQL_REPLICATON_EXPIRATION));
    rrddim_set_by_pointer(
        mpp->st_warning, mpp->rd_warning_latency, (collected_number)(warning & MSSQL_REPLICATON_LATENCY));
    rrddim_set_by_pointer(
        mpp->st_warning,
        mpp->rd_warning_mergeeexpiration,
        (collected_number)(warning & MSSQL_REPLICATON_MERGEEXPIRATION));
    rrddim_set_by_pointer(
        mpp->st_warning,
        mpp->rd_warning_mergefastduration,
        (collected_number)(warning & MSSQL_REPLICATON_MERGEFASTDURATION));
    rrddim_set_by_pointer(
        mpp->st_warning,
        mpp->rd_warning_mergelowduration,
        (collected_number)(warning & MSSQL_REPLICATON_MERGELOWDURATION));
    rrddim_set_by_pointer(
        mpp->st_warning,
        mpp->rd_warning_mergefastrunspeed,
        (collected_number)(warning & MSSQL_REPLICATON_MERGEFASTRUNSPEED));
    rrddim_set_by_pointer(
        mpp->st_warning,
        mpp->rd_warning_mergelowrunspeed,
        (collected_number)(warning & MSSQL_REPLICATON_MERGELOWRUNSPEED));
    rrdset_done(mpp->st_warning);
}

void dict_mssql_replication_avg_latency(struct mssql_publisher_publication *mpp, int update_every)
{
    if (unlikely(!mpp->st_avg_latency)) {
        char id[RRD_ID_LENGTH_MAX + 1];

        snprintfz(
            id,
            RRD_ID_LENGTH_MAX,
            "instance_%s_replication_%s_%s_avg_latency",
            mpp->parent->instanceID,
            mpp->publication,
            mpp->db);
        netdata_fix_chart_name(id);
        mpp->st_avg_latency = rrdset_create_localhost(
            "mssql",
            id,
            NULL,
            "replication",
            "mssql.replication_avg_latency",
            "Average latency for a transactional publication.",
            "seconds",
            PLUGIN_WINDOWS_NAME,
            "PerflibMSSQL",
            PRIO_MSSQL_REPLICATION_AVG_LATENCY,
            update_every,
            RRDSET_TYPE_LINE);

        rrdlabels_add(mpp->st_avg_latency->rrdlabels, "mssql_instance", mpp->parent->instanceID, RRDLABEL_SRC_AUTO);
        rrdlabels_add(mpp->st_avg_latency->rrdlabels, "publisher", mpp->publisher, RRDLABEL_SRC_AUTO);
        rrdlabels_add(mpp->st_avg_latency->rrdlabels, "database", mpp->db, RRDLABEL_SRC_AUTO);
        rrdlabels_add(mpp->st_avg_latency->rrdlabels, "publication", mpp->publication, RRDLABEL_SRC_AUTO);

        mpp->rd_avg_latency = rrddim_add(mpp->st_avg_latency, "latency", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
    }

    rrddim_set_by_pointer(mpp->st_avg_latency, mpp->rd_avg_latency, (collected_number)mpp->avg_latency);
    rrdset_done(mpp->st_avg_latency);
}

void dict_mssql_replication_subscription(struct mssql_publisher_publication *mpp, int update_every)
{
    if (unlikely(!mpp->st_subscription_count)) {
        char id[RRD_ID_LENGTH_MAX + 1];

        snprintfz(
            id,
            RRD_ID_LENGTH_MAX,
            "instance_%s_replication_%s_%s_subscription",
            mpp->parent->instanceID,
            mpp->publication,
            mpp->db);
        netdata_fix_chart_name(id);
        mpp->st_subscription_count = rrdset_create_localhost(
            "mssql",
            id,
            NULL,
            "replication",
            "mssql.replication_subscription",
            "Number of subscriptions to a publication.",
            "subscription",
            PLUGIN_WINDOWS_NAME,
            "PerflibMSSQL",
            PRIO_MSSQL_REPLICATION_SUBSCRIPTION_COUNT,
            update_every,
            RRDSET_TYPE_LINE);

        rrdlabels_add(
            mpp->st_subscription_count->rrdlabels, "mssql_instance", mpp->parent->instanceID, RRDLABEL_SRC_AUTO);
        rrdlabels_add(mpp->st_subscription_count->rrdlabels, "publisher", mpp->publisher, RRDLABEL_SRC_AUTO);
        rrdlabels_add(mpp->st_subscription_count->rrdlabels, "database", mpp->db, RRDLABEL_SRC_AUTO);
        rrdlabels_add(mpp->st_subscription_count->rrdlabels, "publication", mpp->publication, RRDLABEL_SRC_AUTO);

        mpp->rd_subscription_count =
            rrddim_add(mpp->st_subscription_count, "subscription", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
    }

    rrddim_set_by_pointer(
        mpp->st_subscription_count, mpp->rd_subscription_count, (collected_number)mpp->subscriptioncount);
    rrdset_done(mpp->st_subscription_count);
}

void dict_mssql_replication_dist_agent_running(struct mssql_publisher_publication *mpp, int update_every)
{
    if (unlikely(!mpp->st_running_agent)) {
        char id[RRD_ID_LENGTH_MAX + 1];

        snprintfz(
            id,
            RRD_ID_LENGTH_MAX,
            "instance_%s_replication_%s_%s_agent_running",
            mpp->parent->instanceID,
            mpp->publication,
            mpp->db);
        netdata_fix_chart_name(id);
        mpp->st_running_agent = rrdset_create_localhost(
            "mssql",
            id,
            NULL,
            "replication",
            "mssql.replication_agent_running",
            "Distribution agents running.",
            "agents",
            PLUGIN_WINDOWS_NAME,
            "PerflibMSSQL",
            PRIO_MSSQL_REPLICATION_AGENT_RUNNING,
            update_every,
            RRDSET_TYPE_LINE);

        rrdlabels_add(mpp->st_running_agent->rrdlabels, "mssql_instance", mpp->parent->instanceID, RRDLABEL_SRC_AUTO);
        rrdlabels_add(mpp->st_running_agent->rrdlabels, "publisher", mpp->publisher, RRDLABEL_SRC_AUTO);
        rrdlabels_add(mpp->st_running_agent->rrdlabels, "database", mpp->db, RRDLABEL_SRC_AUTO);
        rrdlabels_add(mpp->st_running_agent->rrdlabels, "publication", mpp->publication, RRDLABEL_SRC_AUTO);

        mpp->rd_running_agent = rrddim_add(mpp->st_running_agent, "agents", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
    }

    rrddim_set_by_pointer(mpp->st_running_agent, mpp->rd_running_agent, (collected_number)mpp->runningdistagentcount);
    rrdset_done(mpp->st_running_agent);
}

void dict_mssql_replication_sync_time(struct mssql_publisher_publication *mpp, int update_every)
{
    if (unlikely(!mpp->st_synchronization_time)) {
        char id[RRD_ID_LENGTH_MAX + 1];

        snprintfz(
            id,
            RRD_ID_LENGTH_MAX,
            "instance_%s_replication_%s_%s_synchronization",
            mpp->parent->instanceID,
            mpp->publication,
            mpp->db);
        netdata_fix_chart_name(id);
        mpp->st_synchronization_time = rrdset_create_localhost(
            "mssql",
            id,
            NULL,
            "replication",
            "mssql.replication_synchronization",
            "The shortest synchronization.",
            "seconds",
            PLUGIN_WINDOWS_NAME,
            "PerflibMSSQL",
            PRIO_MSSQL_REPLICATION_SYNC_TIME,
            update_every,
            RRDSET_TYPE_LINE);

        rrdlabels_add(
            mpp->st_synchronization_time->rrdlabels, "mssql_instance", mpp->parent->instanceID, RRDLABEL_SRC_AUTO);
        rrdlabels_add(mpp->st_synchronization_time->rrdlabels, "publisher", mpp->publisher, RRDLABEL_SRC_AUTO);
        rrdlabels_add(mpp->st_synchronization_time->rrdlabels, "database", mpp->db, RRDLABEL_SRC_AUTO);
        rrdlabels_add(mpp->st_synchronization_time->rrdlabels, "publication", mpp->publication, RRDLABEL_SRC_AUTO);

        mpp->rd_synchronization_time =
            rrddim_add(mpp->st_synchronization_time, "seconds", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
    }

    rrddim_set_by_pointer(
        mpp->st_synchronization_time, mpp->rd_synchronization_time, (collected_number)mpp->runningdistagentcount);
    rrdset_done(mpp->st_synchronization_time);
}

int dict_mssql_replication_chart_cb(const DICTIONARY_ITEM *item __maybe_unused, void *value, void *data __maybe_unused)
{
    struct mssql_publisher_publication *mpp = value;
    int *update_every = data;

    dict_mssql_replication_status(mpp, *update_every);
    dict_mssql_replication_warning(mpp, *update_every);
    dict_mssql_replication_avg_latency(mpp, *update_every);
    dict_mssql_replication_subscription(mpp, *update_every);
    dict_mssql_replication_dist_agent_running(mpp, *update_every);
    dict_mssql_replication_sync_time(mpp, *update_every);
}

static void do_mssql_replication(struct mssql_instance *mi, int update_every)
{
    if (unlikely(!mi->conn->collect_replication))
        return;

    dictionary_sorted_walkthrough_read(mi->publisher_publication, dict_mssql_replication_chart_cb, &update_every);
}

static void mssql_database_backup_restore_chart(struct mssql_db_instance *mdi, const char *db, int update_every)
{
    if (unlikely(!mdi->st_db_backup_restore_operations)) {
        char id[RRD_ID_LENGTH_MAX + 1];
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
    if (unlikely(!mdi->st_db_log_flushes)) {
        char id[RRD_ID_LENGTH_MAX + 1];
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

    if (unlikely(!mdi->rd_db_log_flushes)) {
        mdi->rd_db_log_flushes = rrddim_add(mdi->st_db_log_flushes, "flushes", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
    }

    rrddim_set_by_pointer(
        mdi->st_db_log_flushes, mdi->rd_db_log_flushes, (collected_number)mdi->MSSQLDatabaseLogFlushes.current.Data);

    rrdset_done(mdi->st_db_log_flushes);
}

static void mssql_database_log_flushed_chart(struct mssql_db_instance *mdi, const char *db, int update_every)
{
    if (unlikely(!mdi->st_db_log_flushed)) {
        char id[RRD_ID_LENGTH_MAX + 1];
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
    if (unlikely(!mdi->st_db_transactions)) {
        char id[RRD_ID_LENGTH_MAX + 1];
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
    if (unlikely(!mdi->st_db_write_transactions)) {
        char id[RRD_ID_LENGTH_MAX + 1];
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
    if (unlikely(!mdi->st_db_lockwait)) {
        char id[RRD_ID_LENGTH_MAX + 1];
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
    if (unlikely(!mdi->st_db_deadlock)) {
        char id[RRD_ID_LENGTH_MAX + 1];
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

static void mssql_is_readonly_chart(struct mssql_db_instance *mdi, const char *db, int update_every)
{
    if (unlikely(!mdi->st_db_readonly)) {
        char id[RRD_ID_LENGTH_MAX + 1];
        snprintfz(id, RRD_ID_LENGTH_MAX, "db_%s_instance_%s_readonly", db, mdi->parent->instanceID);
        netdata_fix_chart_name(id);
        mdi->st_db_readonly = rrdset_create_localhost(
            "mssql",
            id,
            NULL,
            "locks",
            "mssql.database_readonly",
            "Current database write status.",
            "status",
            PLUGIN_WINDOWS_NAME,
            "PerflibMSSQL",
            PRIO_MSSQL_DATABASE_READONLY,
            update_every,
            RRDSET_TYPE_LINE);

        rrdlabels_add(mdi->st_db_readonly->rrdlabels, "mssql_instance", mdi->parent->instanceID, RRDLABEL_SRC_AUTO);
        rrdlabels_add(mdi->st_db_readonly->rrdlabels, "database", db, RRDLABEL_SRC_AUTO);

        mdi->rd_db_readonly_yes = rrddim_add(mdi->st_db_readonly, "writable", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
        mdi->rd_db_readonly_no = rrddim_add(mdi->st_db_readonly, "readonly", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
    }

    rrddim_set_by_pointer(
        mdi->st_db_readonly, mdi->rd_db_readonly_no, (collected_number)mdi->MSSQLDBIsReadonly.current.Data);

    collected_number opposite = (mdi->MSSQLDBIsReadonly.current.Data) ? 0 : 1;
    rrddim_set_by_pointer(mdi->st_db_readonly, mdi->rd_db_readonly_yes, (collected_number)opposite);

    rrdset_done(mdi->st_db_readonly);
}

static void mssql_db_states_chart(struct mssql_db_instance *mdi, const char *db, int update_every)
{
    if (unlikely(!mdi->st_db_state)) {
        char id[RRD_ID_LENGTH_MAX + 1];
        snprintfz(id, RRD_ID_LENGTH_MAX, "db_%s_instance_%s_state", db, mdi->parent->instanceID);
        netdata_fix_chart_name(id);
        mdi->st_db_state = rrdset_create_localhost(
            "mssql",
            id,
            NULL,
            "locks",
            "mssql.database_state",
            "Current database state.",
            "status",
            PLUGIN_WINDOWS_NAME,
            "PerflibMSSQL",
            PRIO_MSSQL_DATABASE_STATE,
            update_every,
            RRDSET_TYPE_LINE);

        rrdlabels_add(mdi->st_db_state->rrdlabels, "mssql_instance", mdi->parent->instanceID, RRDLABEL_SRC_AUTO);
        rrdlabels_add(mdi->st_db_state->rrdlabels, "database", db, RRDLABEL_SRC_AUTO);

        mdi->rd_db_state[0] = rrddim_add(mdi->st_db_state, "online", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
        mdi->rd_db_state[1] = rrddim_add(mdi->st_db_state, "restoring", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
        mdi->rd_db_state[2] = rrddim_add(mdi->st_db_state, "recovering", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
        mdi->rd_db_state[3] = rrddim_add(mdi->st_db_state, "recovering_pending", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
        mdi->rd_db_state[4] = rrddim_add(mdi->st_db_state, "suspect", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
        mdi->rd_db_state[5] = rrddim_add(mdi->st_db_state, "offline", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
    }
}

static void mssql_db_state_chart_loop(struct mssql_db_instance *mdi, const char *db, int update_every)
{
    collected_number set_value =
        (mdi->MSSQLDBState.current.Data < 5) ? (collected_number)mdi->MSSQLDBState.current.Data : 5;
    mssql_db_states_chart(mdi, db, update_every);
    for (collected_number i = 0; i < NETDATA_DB_STATES; i++) {
        rrddim_set_by_pointer(mdi->st_db_state, mdi->rd_db_state[i], i == set_value);
    }
    rrdset_done(mdi->st_db_state);
}

static void mssql_lock_request_chart(struct mssql_db_instance *mdi, const char *db, int update_every)
{
    if (unlikely(!mdi->st_lock_requests)) {
        char id[RRD_ID_LENGTH_MAX + 1];
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
    if (unlikely(!mdi->st_lock_timeouts)) {
        char id[RRD_ID_LENGTH_MAX + 1];
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
    if (unlikely(!mdi->st_db_active_transactions)) {
        char id[RRD_ID_LENGTH_MAX + 1];
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
    if (unlikely(!mdi->st_db_data_file_size)) {
        char id[RRD_ID_LENGTH_MAX + 1];
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

    if (unlikely(!mdi->collecting_data)) {
        goto endchartcb;
    }

    int *update_every = data;
    struct mssql_instance *mi = mdi->parent;
    if (unlikely(!mi))
        goto endchartcb;

    struct netdata_mssql_conn *conn = mi->conn;
    if (unlikely(!conn))
        goto endchartcb;

    if (likely(conn->collect_data_size))
        mssql_data_file_size_chart(mdi, db, *update_every);

    if (likely(conn->collect_transactions)) {
        mssql_transactions_chart(mdi, db, *update_every);
        mssql_active_transactions_chart(mdi, db, *update_every);
        mssql_write_transactions_chart(mdi, db, *update_every);
    }

    if (likely(conn->collect_waits)) {
        mssql_lockwait_chart(mdi, db, *update_every);
    }

    if (likely(conn->collect_locks)) {
        mssql_deadlock_chart(mdi, db, *update_every);
        mssql_lock_timeout_chart(mdi, db, *update_every);
        mssql_lock_request_chart(mdi, db, *update_every);
    }

    mssql_is_readonly_chart(mdi, db, *update_every);
    mssql_db_state_chart_loop(mdi, db, *update_every);
    mssql_database_log_flushed_chart(mdi, db, *update_every);
    mssql_database_log_flushes_chart(mdi, db, *update_every);
    mssql_database_backup_restore_chart(mdi, db, *update_every);

endchartcb:
    return 1;
}

static void do_mssql_databases(PERF_DATA_BLOCK *pDataBlock, struct mssql_instance *mi, int update_every)
{
    if (unlikely(!pDataBlock))
        goto end_mssql_databases;

    PERF_OBJECT_TYPE *pObjectType = perflibFindObjectTypeByName(pDataBlock, mi->objectName[NETDATA_MSSQL_DATABASE]);
    if (unlikely(!pObjectType))
        return;

    PERF_INSTANCE_DEFINITION *pi = NULL;
    for (LONG i = 0; i < pObjectType->NumInstances; i++) {
        pi = perflibForEachInstance(pDataBlock, pObjectType, pi);
        if (unlikely(!pi))
            break;

        if (unlikely(
                !getInstanceName(pDataBlock, pObjectType, pi, windows_shared_buffer, sizeof(windows_shared_buffer))))
            strncpyz(windows_shared_buffer, "[unknown]", sizeof(windows_shared_buffer) - 1);

        if (unlikely(!strcasecmp(windows_shared_buffer, "_Total")))
            continue;

        struct mssql_db_instance *mdi = dictionary_set(mi->databases, windows_shared_buffer, NULL, sizeof(*mdi));
        if (unlikely(!mdi))
            continue;

        if (unlikely(!mdi->parent)) {
            mdi->parent = mi;
        }

        if (unlikely(!i))
            mdi->collect_instance = true;
    }

end_mssql_databases:
    dictionary_sorted_walkthrough_read(mi->databases, dict_mssql_databases_charts_cb, &update_every);
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
        do_mssql_job_status_sql,
        do_mssql_user_connection,
        do_mssql_blocked_processes,

        NULL};

    DWORD i;
    PERF_DATA_BLOCK *pDataBlock;
    static bool collect_perflib[NETDATA_MSSQL_METRICS_END] = {
        true, true, true, true, true, true, true, true, true, true, false, false};
    for (i = 0; i < NETDATA_MSSQL_ACCESS_METHODS; i++) {
        if (unlikely(!collect_perflib[i]))
            continue;

        pDataBlock = netdata_mssql_get_perf_data_block(collect_perflib, mi, i);
        if (unlikely(!pDataBlock))
            continue;

        doMSSQL[i](pDataBlock, mi, *update_every);
    }

    if (unlikely(unlikely(!mi->conn || !mi->conn->is_connected)))
        return 1;

    for (i = NETDATA_MSSQL_DATABASE; doMSSQL[i]; i++) {
        pDataBlock = (collect_perflib[i]) ? netdata_mssql_get_perf_data_block(collect_perflib, mi, i) : NULL;

        doMSSQL[i](pDataBlock, mi, *update_every);
    }

    do_mssql_replication(mi, *update_every);

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
    if (likely(nd_thread_join(mssql_queries_thread)))
        nd_log_daemon(NDLP_ERR, "Failed to join mssql queries thread");
}
