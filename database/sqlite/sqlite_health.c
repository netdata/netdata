// SPDX-License-Identifier: GPL-3.0-or-later

#include "sqlite_health.h"
#include "sqlite_functions.h"
#include "sqlite_db_migration.h"

#define MAX_HEALTH_SQL_SIZE 2048
#define sqlite3_bind_string_or_null(res,key,param) ((key) ? sqlite3_bind_text(res, param, string2str(key), -1, SQLITE_STATIC) : sqlite3_bind_null(res, param))

/* Health related SQL queries
   Updates an entry in the table
*/
#define SQL_UPDATE_HEALTH_LOG "UPDATE health_log_detail set updated_by_id = ?, flags = ?, exec_run_timestamp = ?, exec_code = ? where unique_id = ? AND alarm_id = ? and transition_id = ?;"
void sql_health_alarm_log_update(RRDHOST *host, ALARM_ENTRY *ae) {
    sqlite3_stmt *res = NULL;
    int rc;

    if (unlikely(!db_meta)) {
        if (default_rrd_memory_mode == RRD_MEMORY_MODE_DBENGINE)
            error_report("HEALTH [%s]: Database has not been initialized", rrdhost_hostname(host));
        return;
    }

    rc = sqlite3_prepare_v2(db_meta, SQL_UPDATE_HEALTH_LOG, -1, &res, 0);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("HEALTH [%s]: Failed to prepare statement for SQL_UPDATE_HEALTH_LOG", rrdhost_hostname(host));
        return;
    }

    rc = sqlite3_bind_int64(res, 1, (sqlite3_int64) ae->updated_by_id);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to bind updated_by_id parameter for SQL_UPDATE_HEALTH_LOG");
        goto failed;
    }

    rc = sqlite3_bind_int64(res, 2, (sqlite3_int64) ae->flags);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to bind flags parameter for SQL_UPDATE_HEALTH_LOG");
        goto failed;
    }

    rc = sqlite3_bind_int64(res, 3, (sqlite3_int64) ae->exec_run_timestamp);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to bind exec_run_timestamp parameter for SQL_UPDATE_HEALTH_LOG");
        goto failed;
    }

    rc = sqlite3_bind_int(res, 4, ae->exec_code);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to bind exec_code parameter for SQL_UPDATE_HEALTH_LOG");
        goto failed;
    }

    rc = sqlite3_bind_int64(res, 5, (sqlite3_int64) ae->unique_id);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to bind unique_id parameter for SQL_UPDATE_HEALTH_LOG");
        goto failed;
    }

    rc = sqlite3_bind_int64(res, 6, (sqlite3_int64) ae->alarm_id);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to bind unique_id parameter for SQL_UPDATE_HEALTH_LOG");
        goto failed;
    }

    rc = sqlite3_bind_blob(res, 7, &ae->transition_id, sizeof(ae->transition_id), SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to bind host_id for SQL_UPDATE_HEALTH_LOG.");
        goto failed;
    }

    rc = execute_insert(res);
    if (unlikely(rc != SQLITE_DONE)) {
        error_report("HEALTH [%s]: Failed to update health log, rc = %d", rrdhost_hostname(host), rc);
    }

failed:
    if (unlikely(sqlite3_finalize(res) != SQLITE_OK))
        error_report("HEALTH [%s]: Failed to finalize the prepared statement for updating health log.", rrdhost_hostname(host));
}

/* Health related SQL queries
   Inserts an entry in the table
*/
#define SQL_INSERT_HEALTH_LOG "INSERT INTO health_log (host_id, alarm_id, " \
    "config_hash_id, name, chart, family, exec, recipient, units, chart_context, last_transition_id, chart_name) " \
    "VALUES (?,?,?,?,?,?,?,?,?,?,?,?) " \
    "ON CONFLICT (host_id, alarm_id) DO UPDATE SET last_transition_id = excluded.last_transition_id, " \
    "chart_name = excluded.chart_name RETURNING health_log_id; "

#define SQL_INSERT_HEALTH_LOG_DETAIL "INSERT INTO health_log_detail (health_log_id, unique_id, alarm_id, alarm_event_id, " \
    "updated_by_id, updates_id, when_key, duration, non_clear_duration, flags, exec_run_timestamp, delay_up_to_timestamp, " \
    "info, exec_code, new_status, old_status, delay, new_value, old_value, last_repeat, transition_id, global_id) " \
    "VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,@global_id); "
void sql_health_alarm_log_insert(RRDHOST *host, ALARM_ENTRY *ae) {
    sqlite3_stmt *res = NULL;
    int rc;
    uint64_t health_log_id = 0;

    if (unlikely(!db_meta)) {
        if (default_rrd_memory_mode == RRD_MEMORY_MODE_DBENGINE)
            error_report("HEALTH [%s]: Database has not been initialized", rrdhost_hostname(host));
        return;
    }

    rc = sqlite3_prepare_v2(db_meta, SQL_INSERT_HEALTH_LOG, -1, &res, 0);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("HEALTH [%s]: Failed to prepare statement for SQL_INSERT_HEALTH_LOG", rrdhost_hostname(host));
        return;
    }

    rc = sqlite3_bind_blob(res, 1, &host->host_uuid, sizeof(host->host_uuid), SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to bind host_id for SQL_INSERT_HEALTH_LOG.");
        goto failed;
    }

    rc = sqlite3_bind_int64(res, 2, (sqlite3_int64) ae->alarm_id);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to bind alarm_id parameter for SQL_INSERT_HEALTH_LOG");
        goto failed;
    }

    rc = sqlite3_bind_blob(res, 3, &ae->config_hash_id, sizeof(ae->config_hash_id), SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to bind config_hash_id parameter for SQL_INSERT_HEALTH_LOG");
        goto failed;
    }

    rc = sqlite3_bind_string_or_null(res, ae->name, 4);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to bind name parameter for SQL_INSERT_HEALTH_LOG");
        goto failed;
    }

    rc = sqlite3_bind_string_or_null(res, ae->chart, 5);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to bind chart parameter for SQL_INSERT_HEALTH_LOG");
        goto failed;
    }

    rc = sqlite3_bind_string_or_null(res, ae->family, 6);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to bind family parameter for SQL_INSERT_HEALTH_LOG");
        goto failed;
    }

    rc = sqlite3_bind_string_or_null(res, ae->exec, 7);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to bind exec parameter for SQL_INSERT_HEALTH_LOG");
        goto failed;
    }

    rc = sqlite3_bind_string_or_null(res, ae->recipient, 8);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to bind recipient parameter for SQL_INSERT_HEALTH_LOG");
        goto failed;
    }

    rc = sqlite3_bind_string_or_null(res, ae->units, 9);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to bind host_id parameter to store node instance information");
        goto failed;
    }

    rc = sqlite3_bind_string_or_null(res, ae->chart_context, 10);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to bind chart_context parameter for SQL_INSERT_HEALTH_LOG");
        goto failed;
    }

    rc = sqlite3_bind_blob(res, 11, &ae->transition_id, sizeof(ae->transition_id), SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to bind transition_id parameter for SQL_INSERT_HEALTH_LOG");
        goto failed;
    }

    rc = sqlite3_bind_string_or_null(res, ae->chart_name, 12);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to bind chart_name parameter for SQL_INSERT_HEALTH_LOG");
        goto failed;
    }

    rc = sqlite3_step_monitored(res);
    if (likely(rc == SQLITE_ROW))
        health_log_id = (size_t) sqlite3_column_int64(res, 0);
    else {
        error_report("HEALTH [%s]: Failed to execute SQL_INSERT_HEALTH_LOG, rc = %d", rrdhost_hostname(host), rc);
        goto failed;
    }

    rc = sqlite3_finalize(res);
    if (unlikely(rc != SQLITE_OK))
        error_report("HEALTH [%s]: Failed to finalize the prepared statement for inserting to health log.", rrdhost_hostname(host));

    rc = sqlite3_prepare_v2(db_meta, SQL_INSERT_HEALTH_LOG_DETAIL, -1, &res, 0);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("HEALTH [%s]: Failed to prepare statement for SQL_INSERT_HEALTH_LOG_DETAIL", rrdhost_hostname(host));
        return;
    }

    rc = sqlite3_bind_int64(res, 1, (sqlite3_int64) health_log_id);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to bind unique_id parameter for SQL_INSERT_HEALTH_LOG_DETAIL");
        goto failed;
    }

    rc = sqlite3_bind_int64(res, 2, (sqlite3_int64) ae->unique_id);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to bind unique_id parameter for SQL_INSERT_HEALTH_LOG_DETAIL");
        goto failed;
    }

    rc = sqlite3_bind_int64(res, 3, (sqlite3_int64) ae->alarm_id);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to bind unique_id parameter for SQL_INSERT_HEALTH_LOG_DETAIL");
        goto failed;
    }

    rc = sqlite3_bind_int64(res, 4, (sqlite3_int64) ae->alarm_event_id);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to bind alarm_event_id parameter for SQL_INSERT_HEALTH_LOG_DETAIL");
        goto failed;
    }

    rc = sqlite3_bind_int64(res, 5, (sqlite3_int64) ae->updated_by_id);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to bind updated_by_id parameter for SQL_INSERT_HEALTH_LOG_DETAIL");
        goto failed;
    }

    rc = sqlite3_bind_int64(res, 6, (sqlite3_int64) ae->updates_id);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to bind updates_id parameter for SQL_INSERT_HEALTH_LOG_DETAIL");
        goto failed;
    }

    rc = sqlite3_bind_int64(res, 7, (sqlite3_int64) ae->when);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to bind when parameter for SQL_INSERT_HEALTH_LOG_DETAIL");
        goto failed;
    }

    rc = sqlite3_bind_int64(res, 8, (sqlite3_int64) ae->duration);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to bind duration parameter for SQL_INSERT_HEALTH_LOG_DETAIL");
        goto failed;
    }

    rc = sqlite3_bind_int64(res, 9, (sqlite3_int64) ae->non_clear_duration);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to bind non_clear_duration parameter for SQL_INSERT_HEALTH_LOG_DETAIL");
        goto failed;
    }

    rc = sqlite3_bind_int64(res, 10, (sqlite3_int64) ae->flags);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to bind flags parameter for SQL_INSERT_HEALTH_LOG_DETAIL");
        goto failed;
    }

    rc = sqlite3_bind_int64(res, 11, (sqlite3_int64) ae->exec_run_timestamp);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to bind exec_run_timestamp parameter for SQL_INSERT_HEALTH_LOG_DETAIL");
        goto failed;
    }

    rc = sqlite3_bind_int64(res, 12, (sqlite3_int64) ae->delay_up_to_timestamp);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to bind delay_up_to_timestamp parameter for SQL_INSERT_HEALTH_LOG_DETAIL");
        goto failed;
    }

    rc = sqlite3_bind_string_or_null(res, ae->info, 13);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to bind info parameter for SQL_INSERT_HEALTH_LOG_DETAIL");
        goto failed;
    }

    rc = sqlite3_bind_int(res, 14, ae->exec_code);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to bind exec_code parameter for SQL_INSERT_HEALTH_LOG_DETAIL");
        goto failed;
    }

    rc = sqlite3_bind_int(res, 15, ae->new_status);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to bind new_status parameter for SQL_INSERT_HEALTH_LOG_DETAIL");
        goto failed;
    }

    rc = sqlite3_bind_int(res, 16, ae->old_status);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to bind old_status parameter for SQL_INSERT_HEALTH_LOG_DETAIL");
        goto failed;
    }

    rc = sqlite3_bind_int(res, 17, ae->delay);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to bind delay parameter for SQL_INSERT_HEALTH_LOG_DETAIL");
        goto failed;
    }

    rc = sqlite3_bind_double(res, 18, ae->new_value);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to bind new_value parameter for SQL_INSERT_HEALTH_LOG_DETAIL");
        goto failed;
    }

    rc = sqlite3_bind_double(res, 19, ae->old_value);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to bind old_value parameter for SQL_INSERT_HEALTH_LOG_DETAIL");
        goto failed;
    }

    rc = sqlite3_bind_int64(res, 20, (sqlite3_int64) ae->last_repeat);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to bind last_repeat parameter for SQL_INSERT_HEALTH_LOG_DETAIL");
        goto failed;
    }

    rc = sqlite3_bind_blob(res, 21, &ae->transition_id, sizeof(ae->transition_id), SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to bind transition_id parameter for SQL_INSERT_HEALTH_LOG_DETAIL");
        goto failed;
    }

    rc = sqlite3_bind_int64(res, 22, (sqlite3_int64) ae->global_id);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to bind global_id parameter for SQL_INSERT_HEALTH_LOG_DETAIL");
        goto failed;
    }

    rc = execute_insert(res);
    if (unlikely(rc != SQLITE_DONE)) {
        error_report("HEALTH [%s]: Failed to execute SQL_INSERT_HEALTH_LOG_DETAIL, rc = %d", rrdhost_hostname(host), rc);
        goto failed;
    }

    ae->flags |= HEALTH_ENTRY_FLAG_SAVED;
    host->health.health_log_entries_written++;

failed:
    if (unlikely(sqlite3_finalize(res) != SQLITE_OK))
        error_report("HEALTH [%s]: Failed to finalize the prepared statement for inserting to health log.", rrdhost_hostname(host));
}

void sql_health_alarm_log_save(RRDHOST *host, ALARM_ENTRY *ae)
{
    if (ae->flags & HEALTH_ENTRY_FLAG_SAVED)
        sql_health_alarm_log_update(host, ae);
    else {
        sql_health_alarm_log_insert(host, ae);
#ifdef ENABLE_ACLK
        if (netdata_cloud_enabled) {
            sql_queue_alarm_to_aclk(host, ae, 0);
        }
#endif
    }
}

/* Health related SQL queries
   Get a count of rows from health log table
*/
#define SQL_COUNT_HEALTH_LOG_DETAIL "SELECT count(1) FROM health_log_detail hld, health_log hl where hl.host_id = @host_id and hl.health_log_id = hld.health_log_id;"
void sql_health_alarm_log_count(RRDHOST *host) {
    sqlite3_stmt *res = NULL;
    int rc;

    if (unlikely(!db_meta)) {
        if (default_rrd_memory_mode == RRD_MEMORY_MODE_DBENGINE)
            error_report("Database has not been initialized");
        return;
    }

    rc = sqlite3_prepare_v2(db_meta, SQL_COUNT_HEALTH_LOG_DETAIL, -1, &res, 0);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to prepare statement to count health log entries from db");
        return;
    }

    rc = sqlite3_bind_blob(res, 1, &host->host_uuid, sizeof(host->host_uuid), SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to bind host_id for SQL_COUNT_HEALTH_LOG.");
        sqlite3_finalize(res);
        return;
    }

    rc = sqlite3_step_monitored(res);
    if (likely(rc == SQLITE_ROW))
        host->health.health_log_entries_written = (size_t) sqlite3_column_int64(res, 0);

    rc = sqlite3_finalize(res);
    if (unlikely(rc != SQLITE_OK))
        error_report("Failed to finalize the prepared statement to count health log entries from db");

    netdata_log_info("HEALTH [%s]: Table health_log_detail contains %lu entries.", rrdhost_hostname(host), (unsigned long int) host->health.health_log_entries_written);
}

/* Health related SQL queries
   Cleans up the health_log_detail table on a non-claimed host
*/
#define SQL_CLEANUP_HEALTH_LOG_DETAIL_NOT_CLAIMED "DELETE FROM health_log_detail WHERE health_log_id IN (SELECT health_log_id FROM health_log WHERE host_id = ?1) AND when_key + ?2 < unixepoch() AND updated_by_id <> 0 AND transition_id NOT IN (SELECT last_transition_id FROM health_log hl WHERE hl.host_id = ?3);"
void sql_health_alarm_log_cleanup_not_claimed(RRDHOST *host) {
    sqlite3_stmt *res = NULL;
    int rc;
    char command[MAX_HEALTH_SQL_SIZE + 1];

    if (unlikely(!db_meta)) {
        if (default_rrd_memory_mode == RRD_MEMORY_MODE_DBENGINE)
            error_report("Database has not been initialized");
        return;
    }

    char uuid_str[UUID_STR_LEN];
    uuid_unparse_lower_fix(&host->host_uuid, uuid_str);

    rc = sqlite3_prepare_v2(db_meta, SQL_CLEANUP_HEALTH_LOG_DETAIL_NOT_CLAIMED, -1, &res, 0);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to prepare statement to cleanup health log detail table (un-claimed)");
        return;
    }

    rc = sqlite3_bind_blob(res, 1, &host->host_uuid, sizeof(host->host_uuid), SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to bind host_id for SQL_CLEANUP_HEALTH_LOG_NOT_CLAIMED.");
        sqlite3_finalize(res);
        return;
    }

    rc = sqlite3_bind_int64(res, 2, (sqlite3_int64)host->health_log.health_log_history);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to bind health log history for SQL_CLEANUP_HEALTH_LOG_NOT_CLAIMED.");
        sqlite3_finalize(res);
        return;
    }

    rc = sqlite3_bind_blob(res, 3, &host->host_uuid, sizeof(host->host_uuid), SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to bind host_id for SQL_CLEANUP_HEALTH_LOG_NOT_CLAIMED.");
        sqlite3_finalize(res);
        return;
    }

    rc = sqlite3_step_monitored(res);
    if (unlikely(rc != SQLITE_DONE))
        error_report("Failed to cleanup health log detail table, rc = %d", rc);

    rc = sqlite3_finalize(res);
    if (unlikely(rc != SQLITE_OK))
        error_report("Failed to finalize the prepared statement to cleanup health log detail table (un-claimed)");

    sql_health_alarm_log_count(host);

    snprintfz(command, MAX_HEALTH_SQL_SIZE, "aclk_alert_%s", uuid_str);
    if (unlikely(table_exists_in_database(command))) {
        sql_aclk_alert_clean_dead_entries(host);
    }
}

/* Health related SQL queries
   Cleans up the health_log_detail table on a claimed host
*/
#define SQL_CLEANUP_HEALTH_LOG_DETAIL_CLAIMED(guid) "DELETE from health_log_detail WHERE unique_id NOT IN (SELECT filtered_alert_unique_id FROM aclk_alert_%s) AND unique_id IN (SELECT hld.unique_id FROM health_log hl, health_log_detail hld WHERE hl.host_id = ?1 AND hl.health_log_id = hld.health_log_id) AND health_log_id IN (SELECT health_log_id FROM health_log WHERE host_id = ?2) AND when_key + ?3 < unixepoch() AND updated_by_id <> 0 AND transition_id NOT IN (SELECT last_transition_id FROM health_log hl WHERE hl.host_id = ?4);", guid
void sql_health_alarm_log_cleanup_claimed(RRDHOST *host) {
    sqlite3_stmt *res = NULL;
    int rc;
    char command[MAX_HEALTH_SQL_SIZE + 1];

    if (unlikely(!db_meta)) {
        if (default_rrd_memory_mode == RRD_MEMORY_MODE_DBENGINE)
            error_report("Database has not been initialized");
        return;
    }

    char uuid_str[UUID_STR_LEN];
    uuid_unparse_lower_fix(&host->host_uuid, uuid_str);
    snprintfz(command, MAX_HEALTH_SQL_SIZE, "aclk_alert_%s", uuid_str);

    if (!table_exists_in_database(command)) {
        sql_health_alarm_log_cleanup_not_claimed(host);
        return;
    }

    snprintfz(command, MAX_HEALTH_SQL_SIZE, SQL_CLEANUP_HEALTH_LOG_DETAIL_CLAIMED(uuid_str));

    rc = sqlite3_prepare_v2(db_meta, command, -1, &res, 0);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to prepare statement to cleanup health log detail table (claimed)");
        return;
    }

    rc = sqlite3_bind_blob(res, 1, &host->host_uuid, sizeof(host->host_uuid), SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to bind first host_id for SQL_CLEANUP_HEALTH_LOG_CLAIMED.");
        sqlite3_finalize(res);
        return;
    }

    rc = sqlite3_bind_blob(res, 2, &host->host_uuid, sizeof(host->host_uuid), SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to bind second host_id for SQL_CLEANUP_HEALTH_LOG_CLAIMED.");
        sqlite3_finalize(res);
        return;
    }

    rc = sqlite3_bind_int64(res, 3, (sqlite3_int64)host->health_log.health_log_history);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to bind health log history for SQL_CLEANUP_HEALTH_LOG_CLAIMED.");
        sqlite3_finalize(res);
        return;
    }

    rc = sqlite3_bind_blob(res, 4, &host->host_uuid, sizeof(host->host_uuid), SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to bind second host_id for SQL_CLEANUP_HEALTH_LOG_CLAIMED.");
        sqlite3_finalize(res);
        return;
    }

    rc = sqlite3_step_monitored(res);
    if (unlikely(rc != SQLITE_DONE))
        error_report("Failed to cleanup health log detail table, rc = %d", rc);

    rc = sqlite3_finalize(res);
    if (unlikely(rc != SQLITE_OK))
        error_report("Failed to finalize the prepared statement to cleanup health log detail table (claimed)");

    sql_health_alarm_log_count(host);

    sql_aclk_alert_clean_dead_entries(host);

}

/* Health related SQL queries
   Cleans up the health_log table.
*/
void sql_health_alarm_log_cleanup(RRDHOST *host) {
    if (!claimed()) {
        sql_health_alarm_log_cleanup_not_claimed(host);
    } else
        sql_health_alarm_log_cleanup_claimed(host);
}

#define SQL_INJECT_REMOVED "insert into health_log_detail (health_log_id, unique_id, alarm_id, alarm_event_id, updated_by_id, updates_id, when_key, duration, non_clear_duration, flags, exec_run_timestamp, delay_up_to_timestamp, info, exec_code, new_status, old_status, delay, new_value, old_value, last_repeat, transition_id, global_id) select health_log_id, ?1, ?2, ?3, 0, ?4, unixepoch(), 0, 0, flags, exec_run_timestamp, unixepoch(), info, exec_code, -2, new_status, delay, NULL, new_value, 0, ?5, now_usec(0) from health_log_detail where unique_id = ?6 and transition_id = ?7;"
#define SQL_INJECT_REMOVED_UPDATE_DETAIL "update health_log_detail set flags = flags | ?1, updated_by_id = ?2 where unique_id = ?3 and transition_id = ?4;"
#define SQL_INJECT_REMOVED_UPDATE_LOG "update health_log set last_transition_id = ?1 where alarm_id = ?2 and last_transition_id = ?3 and host_id = ?4;"
void sql_inject_removed_status(RRDHOST *host, uint32_t alarm_id, uint32_t alarm_event_id, uint32_t unique_id, uint32_t max_unique_id, uuid_t *prev_transition_id)
{
    int rc;

    if (!alarm_id || !alarm_event_id || !unique_id || !max_unique_id)
        return;

    sqlite3_stmt *res = NULL;

    rc = sqlite3_prepare_v2(db_meta, SQL_INJECT_REMOVED, -1, &res, 0);
    if (rc != SQLITE_OK) {
        error_report("Failed to prepare statement when trying to inject removed event");
        return;
    }

    rc = sqlite3_bind_int64(res, 1, (sqlite3_int64) max_unique_id);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to bind max_unique_id parameter for SQL_INJECT_REMOVED");
        goto failed;
    }

    rc = sqlite3_bind_int64(res, 2, (sqlite3_int64) alarm_id);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to bind alarm_id parameter for SQL_INJECT_REMOVED");
        goto failed;
    }

    rc = sqlite3_bind_int64(res, 3, (sqlite3_int64) alarm_event_id + 1);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to bind alarm_event_id parameter for SQL_INJECT_REMOVED");
        goto failed;
    }

    rc = sqlite3_bind_int64(res, 4, (sqlite3_int64) unique_id);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to bind unique_id parameter for SQL_INJECT_REMOVED");
        goto failed;
    }

    uuid_t transition_id;
    uuid_generate_random(transition_id);
    rc = sqlite3_bind_blob(res, 5, &transition_id, sizeof(transition_id), SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to bind config_hash_id parameter for SQL_INJECT_REMOVED");
        goto failed;
    }

    rc = sqlite3_bind_int64(res, 6, (sqlite3_int64) unique_id);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to bind unique_id parameter for SQL_INJECT_REMOVED");
        goto failed;
    }

    rc = sqlite3_bind_blob(res, 7, prev_transition_id, sizeof(*prev_transition_id), SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to bind host_id parameter for SQL_INJECT_REMOVED.");
        goto failed;
    }

    rc = execute_insert(res);
    if (unlikely(rc != SQLITE_DONE)) {
        error_report("HEALTH [N/A]: Failed to execute SQL_INJECT_REMOVED, rc = %d", rc);
        goto failed;
    }

    if (unlikely(sqlite3_finalize(res) != SQLITE_OK))
        error_report("HEALTH [N/A]: Failed to finalize the prepared statement for injecting removed event.");

    //update the old entry in health_log_detail
    rc = sqlite3_prepare_v2(db_meta, SQL_INJECT_REMOVED_UPDATE_DETAIL, -1, &res, 0);
    if (rc != SQLITE_OK) {
        error_report("Failed to prepare statement when trying to update health_log_detail during inject removed event");
        return;
    }

    rc = sqlite3_bind_int64(res, 1, (sqlite3_int64) HEALTH_ENTRY_FLAG_UPDATED);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to bind flags parameter for SQL_INJECT_REMOVED_UPDATE_DETAIL");
        goto failed;
    }

    rc = sqlite3_bind_int64(res, 2, (sqlite3_int64) max_unique_id);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to bind max_unique_id parameter for SQL_INJECT_REMOVED_UPDATE_DETAIL");
        goto failed;
    }

    rc = sqlite3_bind_int64(res, 3, (sqlite3_int64) unique_id);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to bind unique_id parameter for SQL_INJECT_REMOVED_UPDATE_DETAIL");
        goto failed;
    }

    rc = sqlite3_bind_blob(res, 4, prev_transition_id, sizeof(*prev_transition_id), SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to bind host_id parameter for SQL_INJECT_REMOVED_UPDATE_DETAIL");
        goto failed;
    }

    rc = execute_insert(res);
    if (unlikely(rc != SQLITE_DONE)) {
        error_report("HEALTH [N/A]: Failed to execute SQL_INJECT_REMOVED_UPDATE_DETAIL, rc = %d", rc);
        goto failed;
    }

    //update the health_log_table
    rc = sqlite3_prepare_v2(db_meta, SQL_INJECT_REMOVED_UPDATE_LOG, -1, &res, 0);
    if (rc != SQLITE_OK) {
        error_report("Failed to prepare statement when trying to update health_log during inject removed event");
        return;
    }

    rc = sqlite3_bind_blob(res, 1, &transition_id, sizeof(transition_id), SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to bind host_id parameter for SQL_INJECT_REMOVED_UPDATE_LOG");
        goto failed;
    }

    rc = sqlite3_bind_int64(res, 2, (sqlite3_int64) alarm_id);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to bind unique_id parameter for SQL_INJECT_REMOVED_UPDATE_DETAIL");
        goto failed;
    }

    rc = sqlite3_bind_blob(res, 3, prev_transition_id, sizeof(*prev_transition_id), SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to bind host_id parameter for SQL_INJECT_REMOVED_UPDATE_LOG");
        goto failed;
    }

    rc = sqlite3_bind_blob(res, 4, &host->host_uuid, sizeof(host->host_uuid), SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to bind host_id parameter for SQL_INJECT_REMOVED_UPDATE_DETAIL");
        goto failed;
    }

    rc = execute_insert(res);
    if (unlikely(rc != SQLITE_DONE)) {
        error_report("HEALTH [N/A]: Failed to execute SQL_INJECT_REMOVED_UPDATE_DETAIL, rc = %d", rc);
        goto failed;
    }

failed:
    if (unlikely(sqlite3_finalize(res) != SQLITE_OK))
        error_report("HEALTH [N/A]: Failed to finalize the prepared statement for injecting removed event.");
}

#define SQL_SELECT_MAX_UNIQUE_ID "SELECT MAX(hld.unique_id) from health_log_detail hld, health_log hl where hl.host_id = @host_id; and hl.health_log_id = hld.health_log_id"
uint32_t sql_get_max_unique_id (RRDHOST *host)
{
    int rc;
    uint32_t max_unique_id = 0;

    sqlite3_stmt *res = NULL;

    rc = sqlite3_prepare_v2(db_meta, SQL_SELECT_MAX_UNIQUE_ID, -1, &res, 0);
    if (rc != SQLITE_OK) {
        error_report("Failed to prepare statement when trying to get max unique id");
        return 0;
    }

    rc = sqlite3_bind_blob(res, 1, &host->host_uuid, sizeof(host->host_uuid), SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to bind host_id parameter for SQL_SELECT_MAX_UNIQUE_ID.");
        sqlite3_finalize(res);
        return 0;
    }

     while (sqlite3_step_monitored(res) == SQLITE_ROW) {
         max_unique_id = (uint32_t) sqlite3_column_int64(res, 0);
     }

     rc = sqlite3_finalize(res);
     if (unlikely(rc != SQLITE_OK))
         error_report("Failed to finalize the statement");

     return max_unique_id;
}

#define SQL_SELECT_LAST_STATUSES "SELECT hld.new_status, hld.unique_id, hld.alarm_id, hld.alarm_event_id, hld.transition_id from health_log hl, health_log_detail hld where hl.host_id = @host_id and hl.last_transition_id = hld.transition_id;"
void sql_check_removed_alerts_state(RRDHOST *host)
{
    int rc;
    uint32_t max_unique_id = 0;
    sqlite3_stmt *res = NULL;
    uuid_t transition_id;

    rc = sqlite3_prepare_v2(db_meta, SQL_SELECT_LAST_STATUSES, -1, &res, 0);
    if (rc != SQLITE_OK) {
        error_report("Failed to prepare statement when trying to check removed statuses");
        return;
    }

    rc = sqlite3_bind_blob(res, 1, &host->host_uuid, sizeof(host->host_uuid), SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to bind host_id parameter for SQL_SELECT_LAST_STATUSES.");
        sqlite3_finalize(res);
        return;
    }

    while (sqlite3_step_monitored(res) == SQLITE_ROW) {
        uint32_t alarm_id, alarm_event_id, unique_id;
        RRDCALC_STATUS status;

        status  = (RRDCALC_STATUS) sqlite3_column_int(res, 0);
        unique_id = (uint32_t) sqlite3_column_int64(res, 1);
        alarm_id = (uint32_t) sqlite3_column_int64(res, 2);
        alarm_event_id = (uint32_t) sqlite3_column_int64(res, 3);
        uuid_copy(transition_id, *((uuid_t *) sqlite3_column_blob(res, 4)));
        if (unlikely(status != RRDCALC_STATUS_REMOVED)) {
            if (unlikely(!max_unique_id))
                max_unique_id = sql_get_max_unique_id (host);
            sql_inject_removed_status (host, alarm_id, alarm_event_id, unique_id, ++max_unique_id, &transition_id);
        }
    }

     rc = sqlite3_finalize(res);
     if (unlikely(rc != SQLITE_OK))
         error_report("Failed to finalize the statement");
}

/* Health related SQL queries
   Load from the health log table
*/
#define SQL_LOAD_HEALTH_LOG "SELECT hld.unique_id, hld.alarm_id, hld.alarm_event_id, hl.config_hash_id, hld.updated_by_id, " \
            "hld.updates_id, hld.when_key, hld.duration, hld.non_clear_duration, hld.flags, hld.exec_run_timestamp, " \
            "hld.delay_up_to_timestamp, hl.name, hl.chart, hl.family, hl.exec, hl.recipient, ah.source, hl.units, " \
            "hld.info, hld.exec_code, hld.new_status, hld.old_status, hld.delay, hld.new_value, hld.old_value, " \
            "hld.last_repeat, ah.class, ah.component, ah.type, hl.chart_context, hld.transition_id, hld.global_id, hl.chart_name " \
            "FROM health_log hl, alert_hash ah, health_log_detail hld " \
            "WHERE hl.config_hash_id = ah.hash_id and hl.host_id = @host_id and hl.last_transition_id = hld.transition_id;"
void sql_health_alarm_log_load(RRDHOST *host) {
    sqlite3_stmt *res = NULL;
    int ret;
    ssize_t errored = 0, loaded = 0;

    host->health.health_log_entries_written = 0;

    if (unlikely(!db_meta)) {
        if (default_rrd_memory_mode == RRD_MEMORY_MODE_DBENGINE)
            error_report("HEALTH [%s]: Database has not been initialized", rrdhost_hostname(host));
        return;
    }

    sql_check_removed_alerts_state(host);

    ret = sqlite3_prepare_v2(db_meta, SQL_LOAD_HEALTH_LOG, -1, &res, 0);
    if (unlikely(ret != SQLITE_OK)) {
        error_report("HEALTH [%s]: Failed to prepare sql statement to load health log.", rrdhost_hostname(host));
        return;
    }

    ret = sqlite3_bind_blob(res, 1, &host->host_uuid, sizeof(host->host_uuid), SQLITE_STATIC);
    if (unlikely(ret != SQLITE_OK)) {
        error_report("Failed to bind host_id parameter for SQL_LOAD_HEALTH_LOG.");
        sqlite3_finalize(res);
        return;
    }

    DICTIONARY *all_rrdcalcs = dictionary_create(
        DICT_OPTION_NAME_LINK_DONT_CLONE | DICT_OPTION_VALUE_LINK_DONT_CLONE | DICT_OPTION_DONT_OVERWRITE_VALUE);
    RRDCALC *rc;
    foreach_rrdcalc_in_rrdhost_read(host, rc) {
        dictionary_set(all_rrdcalcs, rrdcalc_name(rc), rc, sizeof(*rc));
    }
    foreach_rrdcalc_in_rrdhost_done(rc);

    rw_spinlock_read_lock(&host->health_log.spinlock);

    while (sqlite3_step_monitored(res) == SQLITE_ROW) {
        ALARM_ENTRY *ae = NULL;

        // check that we have valid ids
        uint32_t unique_id = (uint32_t) sqlite3_column_int64(res, 0);
        if(!unique_id) {
            error_report("HEALTH [%s]: Got invalid unique id. Ignoring it.", rrdhost_hostname(host));
            errored++;
            continue;
        }

        uint32_t alarm_id = (uint32_t) sqlite3_column_int64(res, 1);
        if(!alarm_id) {
            error_report("HEALTH [%s]: Got invalid alarm id. Ignoring it.", rrdhost_hostname(host));
            errored++;
            continue;
        }

        //need name, chart and family
        if (sqlite3_column_type(res, 12) == SQLITE_NULL) {
            error_report("HEALTH [%s]: Got null name field. Ignoring it.", rrdhost_hostname(host));
            errored++;
            continue;
        }

        if (sqlite3_column_type(res, 13) == SQLITE_NULL) {
            error_report("HEALTH [%s]: Got null chart field. Ignoring it.", rrdhost_hostname(host));
            errored++;
            continue;
        }

        if (sqlite3_column_type(res, 14) == SQLITE_NULL) {
            error_report("HEALTH [%s]: Got null family field. Ignoring it.", rrdhost_hostname(host));
            errored++;
            continue;
        }

        // Check if we got last_repeat field
        time_t last_repeat = (time_t)sqlite3_column_int64(res, 26);

        rc = dictionary_get(all_rrdcalcs, (char *) sqlite3_column_text(res, 13));
        if(unlikely(rc)) {
            if (rrdcalc_isrepeating(rc)) {
                rc->last_repeat = last_repeat;
                // We iterate through repeating alarm entries only to
                // find the latest last_repeat timestamp. Otherwise,
                // there is no need to keep them in memory.
                continue;
            }
        }

        ae = callocz(1, sizeof(ALARM_ENTRY));

        ae->unique_id = unique_id;
        ae->alarm_id = alarm_id;

        if (sqlite3_column_type(res, 3) != SQLITE_NULL)
            uuid_copy(ae->config_hash_id, *((uuid_t *) sqlite3_column_blob(res, 3)));

        ae->alarm_event_id = (uint32_t) sqlite3_column_int64(res, 2);
        ae->updated_by_id = (uint32_t) sqlite3_column_int64(res, 4);
        ae->updates_id = (uint32_t) sqlite3_column_int64(res, 5);

        ae->when = (time_t) sqlite3_column_int64(res, 6);
        ae->duration = (time_t) sqlite3_column_int64(res, 7);
        ae->non_clear_duration = (time_t) sqlite3_column_int64(res, 8);

        ae->flags = (uint32_t) sqlite3_column_int64(res, 9);
        ae->flags |= HEALTH_ENTRY_FLAG_SAVED;

        ae->exec_run_timestamp = (time_t) sqlite3_column_int64(res, 10);
        ae->delay_up_to_timestamp = (time_t) sqlite3_column_int64(res, 11);

        ae->name   = string_strdupz((char *) sqlite3_column_text(res, 12));
        ae->chart  = string_strdupz((char *) sqlite3_column_text(res, 13));
        ae->family = string_strdupz((char *) sqlite3_column_text(res, 14));

        if (sqlite3_column_type(res, 15) != SQLITE_NULL)
            ae->exec = string_strdupz((char *) sqlite3_column_text(res, 15));
        else
            ae->exec = NULL;

        if (sqlite3_column_type(res, 16) != SQLITE_NULL)
            ae->recipient = string_strdupz((char *) sqlite3_column_text(res, 16));
        else
            ae->recipient = NULL;

        if (sqlite3_column_type(res, 17) != SQLITE_NULL)
            ae->source = string_strdupz((char *) sqlite3_column_text(res, 17));
        else
            ae->source = NULL;

        if (sqlite3_column_type(res, 18) != SQLITE_NULL)
            ae->units = string_strdupz((char *) sqlite3_column_text(res, 18));
        else
            ae->units = NULL;

        if (sqlite3_column_type(res, 19) != SQLITE_NULL)
            ae->info = string_strdupz((char *) sqlite3_column_text(res, 19));
        else
            ae->info = NULL;

        ae->exec_code   = (int) sqlite3_column_int(res, 20);
        ae->new_status  = (RRDCALC_STATUS) sqlite3_column_int(res, 21);
        ae->old_status  = (RRDCALC_STATUS)sqlite3_column_int(res, 22);
        ae->delay       = (int) sqlite3_column_int(res, 23);

        ae->new_value   = (NETDATA_DOUBLE) sqlite3_column_double(res, 24);
        ae->old_value   = (NETDATA_DOUBLE) sqlite3_column_double(res, 25);

        ae->last_repeat = last_repeat;

        if (sqlite3_column_type(res, 27) != SQLITE_NULL)
            ae->classification = string_strdupz((char *) sqlite3_column_text(res, 27));
        else
            ae->classification = NULL;

        if (sqlite3_column_type(res, 28) != SQLITE_NULL)
            ae->component = string_strdupz((char *) sqlite3_column_text(res, 28));
        else
            ae->component = NULL;

        if (sqlite3_column_type(res, 29) != SQLITE_NULL)
            ae->type = string_strdupz((char *) sqlite3_column_text(res, 29));
        else
            ae->type = NULL;

        if (sqlite3_column_type(res, 30) != SQLITE_NULL)
            ae->chart_context = string_strdupz((char *) sqlite3_column_text(res, 30));
        else
            ae->chart_context = NULL;

        if (sqlite3_column_type(res, 31) != SQLITE_NULL)
            uuid_copy(ae->transition_id, *((uuid_t *)sqlite3_column_blob(res, 31)));

        if (sqlite3_column_type(res, 32) != SQLITE_NULL)
            ae->global_id = sqlite3_column_int64(res, 32);

        if (sqlite3_column_type(res, 33) != SQLITE_NULL)
            ae->chart_name = string_strdupz((char *) sqlite3_column_text(res, 33));
        else
            ae->chart_name = NULL;

        char value_string[100 + 1];
        string_freez(ae->old_value_string);
        string_freez(ae->new_value_string);
        ae->old_value_string = string_strdupz(format_value_and_unit(value_string, 100, ae->old_value, ae_units(ae), -1));
        ae->new_value_string = string_strdupz(format_value_and_unit(value_string, 100, ae->new_value, ae_units(ae), -1));

        ae->next = host->health_log.alarms;
        host->health_log.alarms = ae;

        if(unlikely(ae->unique_id > host->health_max_unique_id))
            host->health_max_unique_id = ae->unique_id;

        if(unlikely(ae->alarm_id >= host->health_max_alarm_id))
            host->health_max_alarm_id = ae->alarm_id;

        loaded++;
    }

    rw_spinlock_read_unlock(&host->health_log.spinlock);

    dictionary_destroy(all_rrdcalcs);
    all_rrdcalcs = NULL;

    if(!host->health_max_unique_id) host->health_max_unique_id = (uint32_t)now_realtime_sec();
    if(!host->health_max_alarm_id)  host->health_max_alarm_id  = (uint32_t)now_realtime_sec();

    host->health_log.next_log_id = host->health_max_unique_id + 1;
    if (unlikely(!host->health_log.next_alarm_id || host->health_log.next_alarm_id <= host->health_max_alarm_id))
        host->health_log.next_alarm_id = host->health_max_alarm_id + 1;

    netdata_log_health("[%s]: Table health_log, loaded %zd alarm entries, errors in %zd entries.", rrdhost_hostname(host), loaded, errored);

    ret = sqlite3_finalize(res);
    if (unlikely(ret != SQLITE_OK))
        error_report("Failed to finalize the health log read statement");

    sql_health_alarm_log_count(host);
}

/*
 * Store an alert config hash in the database
 */
#define SQL_STORE_ALERT_CONFIG_HASH "insert or replace into alert_hash (hash_id, date_updated, alarm, template, " \
    "on_key, class, component, type, os, hosts, lookup, every, units, calc, families, plugin, module, " \
    "charts, green, red, warn, crit, exec, to_key, info, delay, options, repeat, host_labels, " \
    "p_db_lookup_dimensions, p_db_lookup_method, p_db_lookup_options, p_db_lookup_after, " \
    "p_db_lookup_before, p_update_every, source, chart_labels) values (?1,unixepoch(),?2,?3,?4,?5,?6,?7,?8,?9,?10,?11,?12," \
    "?13,?14,?15,?16,?17,?18,?19,?20,?21,?22,?23,?24,?25,?26,?27,?28,?29,?30,?31,?32,?33,?34,?35,?36);"

int sql_store_alert_config_hash(uuid_t *hash_id, struct alert_config *cfg)
{
    static __thread sqlite3_stmt *res = NULL;
    int rc, param = 0;

    if (unlikely(!db_meta)) {
        if (default_rrd_memory_mode != RRD_MEMORY_MODE_DBENGINE)
            return 0;
        error_report("Database has not been initialized");
        return 1;
    }

    if (unlikely(!res)) {
        rc = prepare_statement(db_meta, SQL_STORE_ALERT_CONFIG_HASH, &res);
        if (unlikely(rc != SQLITE_OK)) {
            error_report("Failed to prepare statement to store alert configuration, rc = %d", rc);
            return 1;
        }
    }

    rc = sqlite3_bind_blob(res, ++param, hash_id, sizeof(*hash_id), SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    rc = sqlite3_bind_string_or_null(res, cfg->alarm, ++param);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    rc = sqlite3_bind_string_or_null(res, cfg->template_key, ++param);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    rc = sqlite3_bind_string_or_null(res, cfg->on, ++param);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    rc = sqlite3_bind_string_or_null(res, cfg->classification, ++param);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    rc = sqlite3_bind_string_or_null(res, cfg->component, ++param);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    rc = sqlite3_bind_string_or_null(res, cfg->type, ++param);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    rc = sqlite3_bind_string_or_null(res, cfg->os, ++param);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    rc = sqlite3_bind_string_or_null(res, cfg->host, ++param);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    rc = sqlite3_bind_string_or_null(res, cfg->lookup, ++param);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    rc = sqlite3_bind_string_or_null(res, cfg->every, ++param);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    rc = sqlite3_bind_string_or_null(res, cfg->units, ++param);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    rc = sqlite3_bind_string_or_null(res, cfg->calc, ++param);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    rc = sqlite3_bind_string_or_null(res, cfg->families, ++param);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    rc = sqlite3_bind_string_or_null(res, cfg->plugin, ++param);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    rc = sqlite3_bind_string_or_null(res, cfg->module, ++param);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    rc = sqlite3_bind_string_or_null(res, cfg->charts, ++param);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    rc = sqlite3_bind_string_or_null(res, cfg->green, ++param);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    rc = sqlite3_bind_string_or_null(res, cfg->red, ++param);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    rc = sqlite3_bind_string_or_null(res, cfg->warn, ++param);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    rc = sqlite3_bind_string_or_null(res, cfg->crit, ++param);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    rc = sqlite3_bind_string_or_null(res, cfg->exec, ++param);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    rc = sqlite3_bind_string_or_null(res, cfg->to, ++param);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    rc = sqlite3_bind_string_or_null(res, cfg->info, ++param);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    rc = sqlite3_bind_string_or_null(res, cfg->delay, ++param);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    rc = sqlite3_bind_string_or_null(res, cfg->options, ++param);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    rc = sqlite3_bind_string_or_null(res, cfg->repeat, ++param);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    rc = sqlite3_bind_string_or_null(res, cfg->host_labels, ++param);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    if (cfg->p_db_lookup_after) {
        rc = sqlite3_bind_string_or_null(res, cfg->p_db_lookup_dimensions, ++param);
        if (unlikely(rc != SQLITE_OK))
            goto bind_fail;

        rc = sqlite3_bind_string_or_null(res, cfg->p_db_lookup_method, ++param);
        if (unlikely(rc != SQLITE_OK))
            goto bind_fail;

        rc = sqlite3_bind_int(res, ++param, (int) cfg->p_db_lookup_options);
        if (unlikely(rc != SQLITE_OK))
            goto bind_fail;

        rc = sqlite3_bind_int(res, ++param, (int) cfg->p_db_lookup_after);
        if (unlikely(rc != SQLITE_OK))
            goto bind_fail;

        rc = sqlite3_bind_int(res, ++param, (int) cfg->p_db_lookup_before);
        if (unlikely(rc != SQLITE_OK))
            goto bind_fail;
    } else {
        rc = sqlite3_bind_null(res, ++param);
        if (unlikely(rc != SQLITE_OK))
            goto bind_fail;

        rc = sqlite3_bind_null(res, ++param);
        if (unlikely(rc != SQLITE_OK))
            goto bind_fail;

        rc = sqlite3_bind_null(res, ++param);
        if (unlikely(rc != SQLITE_OK))
            goto bind_fail;

        rc = sqlite3_bind_null(res, ++param);
        if (unlikely(rc != SQLITE_OK))
            goto bind_fail;

        rc = sqlite3_bind_null(res, ++param);
        if (unlikely(rc != SQLITE_OK))
            goto bind_fail;
    }

    rc = sqlite3_bind_int(res, ++param, cfg->p_update_every);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    rc = sqlite3_bind_string_or_null(res, cfg->source, ++param);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    rc = sqlite3_bind_string_or_null(res, cfg->chart_labels, ++param);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    rc = execute_insert(res);
    if (unlikely(rc != SQLITE_DONE))
        error_report("Failed to store alert config, rc = %d", rc);

    rc = sqlite3_reset(res);
    if (unlikely(rc != SQLITE_OK))
        error_report("Failed to reset statement in alert hash_id store function, rc = %d", rc);

    return 0;

bind_fail:
    error_report("Failed to bind parameter %d to store alert hash_id, rc = %d", param, rc);
    rc = sqlite3_reset(res);
    if (unlikely(rc != SQLITE_OK))
        error_report("Failed to reset statement in alert hash_id store function, rc = %d", rc);
    return 1;
}

/*
  alert hashes are used for cloud communication.
  if cloud is disabled or openssl is not available (which will prevent cloud connectivity)
  skip hash calculations
*/
#if !defined DISABLE_CLOUD && defined ENABLE_HTTPS
#define DIGEST_ALERT_CONFIG_VAL(v) ((v) ? EVP_DigestUpdate(evpctx, (string2str(v)), string_strlen((v))) : EVP_DigestUpdate(evpctx, "", 1))
#endif
int alert_hash_and_store_config(
    uuid_t hash_id,
    struct alert_config *cfg,
    int store_hash)
{
#if !defined DISABLE_CLOUD && defined ENABLE_HTTPS
    EVP_MD_CTX *evpctx;
    unsigned char hash_value[EVP_MAX_MD_SIZE];
    unsigned int hash_len;
    evpctx = EVP_MD_CTX_create();
    EVP_DigestInit_ex(evpctx, EVP_sha256(), NULL);

    DIGEST_ALERT_CONFIG_VAL(cfg->alarm);
    DIGEST_ALERT_CONFIG_VAL(cfg->template_key);
    DIGEST_ALERT_CONFIG_VAL(cfg->os);
    DIGEST_ALERT_CONFIG_VAL(cfg->host);
    DIGEST_ALERT_CONFIG_VAL(cfg->on);
    DIGEST_ALERT_CONFIG_VAL(cfg->families);
    DIGEST_ALERT_CONFIG_VAL(cfg->plugin);
    DIGEST_ALERT_CONFIG_VAL(cfg->module);
    DIGEST_ALERT_CONFIG_VAL(cfg->charts);
    DIGEST_ALERT_CONFIG_VAL(cfg->lookup);
    DIGEST_ALERT_CONFIG_VAL(cfg->calc);
    DIGEST_ALERT_CONFIG_VAL(cfg->every);
    DIGEST_ALERT_CONFIG_VAL(cfg->green);
    DIGEST_ALERT_CONFIG_VAL(cfg->red);
    DIGEST_ALERT_CONFIG_VAL(cfg->warn);
    DIGEST_ALERT_CONFIG_VAL(cfg->crit);
    DIGEST_ALERT_CONFIG_VAL(cfg->exec);
    DIGEST_ALERT_CONFIG_VAL(cfg->to);
    DIGEST_ALERT_CONFIG_VAL(cfg->units);
    DIGEST_ALERT_CONFIG_VAL(cfg->info);
    DIGEST_ALERT_CONFIG_VAL(cfg->classification);
    DIGEST_ALERT_CONFIG_VAL(cfg->component);
    DIGEST_ALERT_CONFIG_VAL(cfg->type);
    DIGEST_ALERT_CONFIG_VAL(cfg->delay);
    DIGEST_ALERT_CONFIG_VAL(cfg->options);
    DIGEST_ALERT_CONFIG_VAL(cfg->repeat);
    DIGEST_ALERT_CONFIG_VAL(cfg->host_labels);
    DIGEST_ALERT_CONFIG_VAL(cfg->chart_labels);

    EVP_DigestFinal_ex(evpctx, hash_value, &hash_len);
    EVP_MD_CTX_destroy(evpctx);
    fatal_assert(hash_len > sizeof(uuid_t));

    char uuid_str[UUID_STR_LEN];
    uuid_unparse_lower(*((uuid_t *)&hash_value), uuid_str);
    uuid_copy(hash_id, *((uuid_t *)&hash_value));

    /* store everything, so it can be recreated when not in memory or just a subset ? */
    if (store_hash)
        (void)sql_store_alert_config_hash( (uuid_t *)&hash_value, cfg);
#else
    UNUSED(hash_id);
    UNUSED(cfg);
    UNUSED(store_hash);
#endif

    return 1;
}

#define SQL_SELECT_HEALTH_LAST_EXECUTED_EVENT "SELECT hld.new_status FROM health_log hl, health_log_detail hld WHERE hl.alarm_id = %u AND hld.unique_id != %u AND hld.flags & %u AND hl.host_id = @host_id and hl.health_log_id = hld.health_log_id ORDER BY hld.unique_id DESC LIMIT 1;"
int sql_health_get_last_executed_event(RRDHOST *host, ALARM_ENTRY *ae, RRDCALC_STATUS *last_executed_status)
{
    int rc = 0, ret = -1;
    char command[MAX_HEALTH_SQL_SIZE + 1];
    sqlite3_stmt *res = NULL;

    snprintfz(command, MAX_HEALTH_SQL_SIZE, SQL_SELECT_HEALTH_LAST_EXECUTED_EVENT, ae->alarm_id, ae->unique_id, (uint32_t) HEALTH_ENTRY_FLAG_EXEC_RUN);

    rc = sqlite3_prepare_v2(db_meta, command, -1, &res, 0);
    if (rc != SQLITE_OK) {
        error_report("Failed to prepare statement when trying to get last executed status");
        return ret;
    }

    rc = sqlite3_bind_blob(res, 1, &host->host_uuid, sizeof(host->host_uuid), SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to bind host_id parameter for SQL_SELECT_HEALTH_LAST_EXECUTED_EVENT.");
        sqlite3_finalize(res);
        return ret;
    }

    ret = 0;
    while (sqlite3_step_monitored(res) == SQLITE_ROW) {
        *last_executed_status  = (RRDCALC_STATUS) sqlite3_column_int(res, 0);
        ret = 1;
    }

     rc = sqlite3_finalize(res);
     if (unlikely(rc != SQLITE_OK))
         error_report("Failed to finalize the statement.");

     return ret;
}

#define SQL_SELECT_HEALTH_LOG "SELECT hld.unique_id, hld.alarm_id, hld.alarm_event_id, hl.config_hash_id, hld.updated_by_id, hld.updates_id, hld.when_key, hld.duration, hld.non_clear_duration, hld.flags, hld.exec_run_timestamp, hld.delay_up_to_timestamp, hl.name, hl.chart, hl.family, hl.exec, hl.recipient, ah.source, hl.units, hld.info, hld.exec_code, hld.new_status, hld.old_status, hld.delay, hld.new_value, hld.old_value, hld.last_repeat, ah.class, ah.component, ah.type, hl.chart_context, hld.transition_id FROM health_log hl, alert_hash ah, health_log_detail hld WHERE hl.config_hash_id = ah.hash_id and hl.health_log_id = hld.health_log_id and hl.host_id = @host_id "
void sql_health_alarm_log2json(RRDHOST *host, BUFFER *wb, uint32_t after, char *chart) {

    buffer_strcat(wb, "[");

    unsigned int max = host->health_log.max;
    unsigned int count = 0;

    sqlite3_stmt *res = NULL;
    int rc;

    BUFFER *command = buffer_create(MAX_HEALTH_SQL_SIZE, NULL);
    buffer_sprintf(command, SQL_SELECT_HEALTH_LOG);

    if (chart) {
        char chart_sql[MAX_HEALTH_SQL_SIZE + 1];
        snprintfz(chart_sql, MAX_HEALTH_SQL_SIZE, "AND hl.chart = '%s' ", chart);
        buffer_strcat(command, chart_sql);
    }

    if (after) {
        char after_sql[MAX_HEALTH_SQL_SIZE + 1];
        snprintfz(after_sql, MAX_HEALTH_SQL_SIZE, "AND hld.unique_id > %u ", after);
        buffer_strcat(command, after_sql);
    }

    {
        char limit_sql[MAX_HEALTH_SQL_SIZE + 1];
        snprintfz(limit_sql, MAX_HEALTH_SQL_SIZE, "ORDER BY hld.unique_id DESC LIMIT %u ", max);
        buffer_strcat(command, limit_sql);
    }

    rc = sqlite3_prepare_v2(db_meta, buffer_tostring(command), -1, &res, 0);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to prepare statement SQL_SELECT_HEALTH_LOG");
        buffer_free(command);
        return;
    }

    rc = sqlite3_bind_blob(res, 1, &host->host_uuid, sizeof(host->host_uuid), SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to bind host_id for SQL_SELECT_HEALTH_LOG.");
        sqlite3_finalize(res);
        buffer_free(command);
        return;
    }

    while (sqlite3_step(res) == SQLITE_ROW) {

        char old_value_string[100 + 1];
        char new_value_string[100 + 1];

        char config_hash_id[UUID_STR_LEN];
        uuid_unparse_lower(*((uuid_t *) sqlite3_column_blob(res, 3)), config_hash_id);

        char transition_id[UUID_STR_LEN] = {0};
        if (sqlite3_column_type(res, 31) != SQLITE_NULL)
            uuid_unparse_lower(*((uuid_t *) sqlite3_column_blob(res, 31)), transition_id);

        char *edit_command = sqlite3_column_bytes(res, 17) > 0 ? health_edit_command_from_source((char *)sqlite3_column_text(res, 17)) : strdupz("UNKNOWN=0=UNKNOWN");

        if (count)
            buffer_sprintf(wb, ",");

        count++;

        buffer_sprintf(
            wb,
            "\n\t{\n"
            "\t\t\"hostname\": \"%s\",\n"
            "\t\t\"utc_offset\": %d,\n"
            "\t\t\"timezone\": \"%s\",\n"
            "\t\t\"unique_id\": %u,\n"
            "\t\t\"alarm_id\": %u,\n"
            "\t\t\"alarm_event_id\": %u,\n"
            "\t\t\"config_hash_id\": \"%s\",\n"
            "\t\t\"transition_id\": \"%s\",\n"
            "\t\t\"name\": \"%s\",\n"
            "\t\t\"chart\": \"%s\",\n"
            "\t\t\"context\": \"%s\",\n"
            "\t\t\"family\": \"%s\",\n"
            "\t\t\"class\": \"%s\",\n"
            "\t\t\"component\": \"%s\",\n"
            "\t\t\"type\": \"%s\",\n"
            "\t\t\"processed\": %s,\n"
            "\t\t\"updated\": %s,\n"
            "\t\t\"exec_run\": %lu,\n"
            "\t\t\"exec_failed\": %s,\n"
            "\t\t\"exec\": \"%s\",\n"
            "\t\t\"recipient\": \"%s\",\n"
            "\t\t\"exec_code\": %d,\n"
            "\t\t\"source\": \"%s\",\n"
            "\t\t\"command\": \"%s\",\n"
            "\t\t\"units\": \"%s\",\n"
            "\t\t\"when\": %lu,\n"
            "\t\t\"duration\": %lu,\n"
            "\t\t\"non_clear_duration\": %lu,\n"
            "\t\t\"status\": \"%s\",\n"
            "\t\t\"old_status\": \"%s\",\n"
            "\t\t\"delay\": %d,\n"
            "\t\t\"delay_up_to_timestamp\": %lu,\n"
            "\t\t\"updated_by_id\": %u,\n"
            "\t\t\"updates_id\": %u,\n"
            "\t\t\"value_string\": \"%s\",\n"
            "\t\t\"old_value_string\": \"%s\",\n"
            "\t\t\"last_repeat\": \"%lu\",\n"
            "\t\t\"silenced\": \"%s\",\n",
            rrdhost_hostname(host),
            host->utc_offset,
            rrdhost_abbrev_timezone(host),
            (unsigned int) sqlite3_column_int64(res, 0),
            (unsigned int) sqlite3_column_int64(res, 1),
            (unsigned int) sqlite3_column_int64(res, 2),
            config_hash_id,
            transition_id,
            sqlite3_column_text(res, 12),
            sqlite3_column_text(res, 13),
            sqlite3_column_text(res, 30),
            sqlite3_column_text(res, 14),
            sqlite3_column_text(res, 27) ? (const char *) sqlite3_column_text(res, 27) : (char *) "Unknown",
            sqlite3_column_text(res, 28) ? (const char *) sqlite3_column_text(res, 28) : (char *) "Unknown",
            sqlite3_column_text(res, 29) ? (const char *) sqlite3_column_text(res, 29) : (char *) "Unknown",
            (sqlite3_column_int64(res, 9) & HEALTH_ENTRY_FLAG_PROCESSED)?"true":"false",
            (sqlite3_column_int64(res, 9) & HEALTH_ENTRY_FLAG_UPDATED)?"true":"false",
            (long unsigned int)sqlite3_column_int64(res, 10),
            (sqlite3_column_int64(res, 9) & HEALTH_ENTRY_FLAG_EXEC_FAILED)?"true":"false",
            sqlite3_column_text(res, 15) ? (const char *) sqlite3_column_text(res, 15) : string2str(host->health.health_default_exec),
            sqlite3_column_text(res, 16) ? (const char *) sqlite3_column_text(res, 16) : string2str(host->health.health_default_recipient),
            sqlite3_column_int(res, 20),
            sqlite3_column_text(res, 17) ? (const char *) sqlite3_column_text(res, 17) : (char *) "Unknown",
            edit_command,
            sqlite3_column_text(res, 18),
            (long unsigned int)sqlite3_column_int64(res, 6),
            (long unsigned int)sqlite3_column_int64(res, 7),
            (long unsigned int)sqlite3_column_int64(res, 8),
            rrdcalc_status2string(sqlite3_column_int(res, 21)),
            rrdcalc_status2string(sqlite3_column_int(res, 22)),
            sqlite3_column_int(res, 23),
            (long unsigned int)sqlite3_column_int64(res, 11),
            (unsigned int)sqlite3_column_int64(res, 4),
            (unsigned int)sqlite3_column_int64(res, 5),
            sqlite3_column_type(res, 24) == SQLITE_NULL ? "-" : format_value_and_unit(new_value_string, 100, sqlite3_column_double(res, 24), (char *) sqlite3_column_text(res, 18), -1),
            sqlite3_column_type(res, 25) == SQLITE_NULL ? "-" : format_value_and_unit(old_value_string, 100, sqlite3_column_double(res, 25), (char *) sqlite3_column_text(res, 18), -1),
            (long unsigned int)sqlite3_column_int64(res, 26),
            (sqlite3_column_int64(res, 9) & HEALTH_ENTRY_FLAG_SILENCED)?"true":"false");

        health_string2json(wb, "\t\t", "info", (char *) sqlite3_column_text(res, 19), ",\n");

        if(unlikely(sqlite3_column_int64(res, 9) & HEALTH_ENTRY_FLAG_NO_CLEAR_NOTIFICATION)) {
            buffer_strcat(wb, "\t\t\"no_clear_notification\": true,\n");
        }

        buffer_strcat(wb, "\t\t\"value\":");
        if (sqlite3_column_type(res, 24) == SQLITE_NULL)
            buffer_strcat(wb, "null");
        else
            buffer_print_netdata_double(wb, sqlite3_column_double(res, 24));
        buffer_strcat(wb, ",\n");

        buffer_strcat(wb, "\t\t\"old_value\":");
        if (sqlite3_column_type(res, 25) == SQLITE_NULL)
            buffer_strcat(wb, "null");
        else
            buffer_print_netdata_double(wb, sqlite3_column_double(res, 25));
        buffer_strcat(wb, "\n");

        buffer_strcat(wb, "\t}");

        freez(edit_command);
    }

    buffer_strcat(wb, "\n]");

    rc = sqlite3_finalize(res);
    if (unlikely(rc != SQLITE_OK))
        error_report("Failed to finalize statement for SQL_SELECT_HEALTH_LOG");

    buffer_free(command);
}

#define SQL_COPY_HEALTH_LOG(table) "INSERT OR IGNORE INTO health_log (host_id, alarm_id, config_hash_id, name, chart, family, exec, recipient, units, chart_context) SELECT ?1, alarm_id, config_hash_id, name, chart, family, exec, recipient, units, chart_context from %s;", table
#define SQL_COPY_HEALTH_LOG_DETAIL(table) "INSERT INTO health_log_detail (unique_id, alarm_id, alarm_event_id, updated_by_id, updates_id, when_key, duration, non_clear_duration, flags, exec_run_timestamp, delay_up_to_timestamp, info, exec_code, new_status, old_status, delay, new_value, old_value, last_repeat, transition_id, global_id, host_id) SELECT unique_id, alarm_id, alarm_event_id, updated_by_id, updates_id, when_key, duration, non_clear_duration, flags, exec_run_timestamp, delay_up_to_timestamp, info, exec_code, new_status, old_status, delay, new_value, old_value, last_repeat, transition_id, now_usec(1), ?1 from %s;", table
#define SQL_UPDATE_HEALTH_LOG_DETAIL_TRANSITION_ID "update health_log_detail set transition_id = uuid_random() where transition_id is null;"
#define SQL_UPDATE_HEALTH_LOG_DETAIL_HEALTH_LOG_ID "update health_log_detail set health_log_id = (select health_log_id from health_log where host_id = ?1 and alarm_id = health_log_detail.alarm_id) where health_log_id is null and host_id = ?2;"
#define SQL_UPDATE_HEALTH_LOG_LAST_TRANSITION_ID "update health_log set last_transition_id = (select transition_id from health_log_detail where health_log_id = health_log.health_log_id and alarm_id = health_log.alarm_id group by (alarm_id) having max(alarm_event_id)) where host_id = ?1;"
int health_migrate_old_health_log_table(char *table) {
    if (!table)
        return 0;

    //table should contain guid. We need to
    //keep it in the new table along with it's data
    //health_log_XXXXXXXX_XXXX_XXXX_XXXX_XXXXXXXXXXXX
    if (strnlen(table, 46) != 46) {
        return 0;
    }

    char *uuid_from_table = strdupz(table + 11);
    uuid_t uuid;
    if (uuid_parse_fix(uuid_from_table, uuid)) {
        freez(uuid_from_table);
        return 0;
    }

    int rc;
    char command[MAX_HEALTH_SQL_SIZE + 1];
    sqlite3_stmt *res = NULL;
    snprintfz(command, MAX_HEALTH_SQL_SIZE, SQL_COPY_HEALTH_LOG(table));
    rc = sqlite3_prepare_v2(db_meta, command, -1, &res, 0);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to prepare statement to copy health log, rc = %d", rc);
        freez(uuid_from_table);
        return 0;
    }

    rc = sqlite3_bind_blob(res, 1, &uuid, sizeof(uuid), SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK)) {
        rc = sqlite3_finalize(res);
        if (unlikely(rc != SQLITE_OK))
            error_report("Failed to reset statement to copy health log table, rc = %d", rc);
        freez(uuid_from_table);
        return 0;
    }

    rc = execute_insert(res);
    if (unlikely(rc != SQLITE_DONE)) {
        error_report("Failed to execute SQL_COPY_HEALTH_LOG, rc = %d", rc);
        rc = sqlite3_finalize(res);
        if (unlikely(rc != SQLITE_OK))
            error_report("Failed to reset statement to copy health log table, rc = %d", rc);
        freez(uuid_from_table);
    }

    //detail
    snprintfz(command, MAX_HEALTH_SQL_SIZE, SQL_COPY_HEALTH_LOG_DETAIL(table));
    rc = sqlite3_prepare_v2(db_meta, command, -1, &res, 0);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to prepare statement to copy health log detail, rc = %d", rc);
        return 0;
    }

    rc = sqlite3_bind_blob(res, 1, &uuid, sizeof(uuid), SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK)) {
        rc = sqlite3_finalize(res);
        if (unlikely(rc != SQLITE_OK))
            error_report("Failed to reset statement to copy health log detail, rc = %d", rc);
        return 0;
    }

    rc = execute_insert(res);
    if (unlikely(rc != SQLITE_DONE)) {
        error_report("Failed to execute SQL_COPY_HEALTH_LOG_DETAIL, rc = %d", rc);
        rc = sqlite3_finalize(res);
        if (unlikely(rc != SQLITE_OK))
            error_report("Failed to reset statement to copy health log detail table, rc = %d", rc);
        return 0;
    }

    //update transition ids
    rc = sqlite3_prepare_v2(db_meta, SQL_UPDATE_HEALTH_LOG_DETAIL_TRANSITION_ID, -1, &res, 0);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to prepare statement to update health log detail with transition ids, rc = %d", rc);
        return 0;
    }

    rc = execute_insert(res);
    if (unlikely(rc != SQLITE_DONE)) {
        error_report("Failed to execute SQL_UPDATE_HEALTH_LOG_DETAIL_TRANSITION_ID, rc = %d", rc);
        rc = sqlite3_finalize(res);
        if (unlikely(rc != SQLITE_OK))
            error_report("Failed to reset statement to update health log detail table with transition ids, rc = %d", rc);
        return 0;
    }

    //update health_log_id
    rc = sqlite3_prepare_v2(db_meta, SQL_UPDATE_HEALTH_LOG_DETAIL_HEALTH_LOG_ID, -1, &res, 0);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to prepare statement to update health log detail with health log ids, rc = %d", rc);
        return 0;
    }

    rc = sqlite3_bind_blob(res, 1, &uuid, sizeof(uuid), SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK)) {
        rc = sqlite3_finalize(res);
        if (unlikely(rc != SQLITE_OK))
            error_report("Failed to reset statement to update health log detail with health log ids, rc = %d", rc);
        return 0;
    }

    rc = sqlite3_bind_blob(res, 2, &uuid, sizeof(uuid), SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK)) {
        rc = sqlite3_finalize(res);
        if (unlikely(rc != SQLITE_OK))
            error_report("Failed to reset statement to update health log detail with health log ids, rc = %d", rc);
        return 0;
    }

    rc = execute_insert(res);
    if (unlikely(rc != SQLITE_DONE)) {
        error_report("Failed to execute SQL_UPDATE_HEALTH_LOG_DETAIL_HEALTH_LOG_ID, rc = %d", rc);
        rc = sqlite3_finalize(res);
        if (unlikely(rc != SQLITE_OK))
            error_report("Failed to reset statement to update health log detail table with health log ids, rc = %d", rc);
    }

    //update last transition id
    rc = sqlite3_prepare_v2(db_meta, SQL_UPDATE_HEALTH_LOG_LAST_TRANSITION_ID, -1, &res, 0);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to prepare statement to update health log  with last transition id, rc = %d", rc);
        return 0;
    }

    rc = sqlite3_bind_blob(res, 1, &uuid, sizeof(uuid), SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK)) {
        rc = sqlite3_finalize(res);
        if (unlikely(rc != SQLITE_OK))
            error_report("Failed to reset statement to update health log with last transition id, rc = %d", rc);
        return 0;
    }

    rc = execute_insert(res);
    if (unlikely(rc != SQLITE_DONE)) {
        error_report("Failed to execute SQL_UPDATE_HEALTH_LOG_LAST_TRANSITION_ID, rc = %d", rc);
        rc = sqlite3_finalize(res);
        if (unlikely(rc != SQLITE_OK))
            error_report("Failed to reset statement to update health log table with last transition id, rc = %d", rc);
    }

    return 1;
}

#define SQL_GET_ALARM_ID "select alarm_id, health_log_id from health_log where host_id = @host_id and chart = @chart and name = @name and config_hash_id = @config_hash_id"
#define SQL_GET_EVENT_ID "select max(alarm_event_id) + 1 from health_log_detail where health_log_id = @health_log_id and alarm_id = @alarm_id"
uint32_t sql_get_alarm_id(RRDHOST *host, STRING *chart, STRING *name, uint32_t *next_event_id, uuid_t *config_hash_id)
{
    int rc = 0;
    sqlite3_stmt *res = NULL;
    uint32_t alarm_id = 0;
    uint64_t health_log_id = 0;

    rc = sqlite3_prepare_v2(db_meta, SQL_GET_ALARM_ID, -1, &res, 0);
    if (rc != SQLITE_OK) {
        error_report("Failed to prepare statement when trying to get an alarm id");
        return alarm_id;
    }

    rc = sqlite3_bind_blob(res, 1, &host->host_uuid, sizeof(host->host_uuid), SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to bind host_id parameter for SQL_GET_ALARM_ID.");
        sqlite3_finalize(res);
        return alarm_id;
    }

    rc = sqlite3_bind_string_or_null(res, chart, 2);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to bind char parameter for SQL_GET_ALARM_ID.");
        sqlite3_finalize(res);
        return alarm_id;
    }

    rc = sqlite3_bind_string_or_null(res, name, 3);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to bind name parameter for SQL_GET_ALARM_ID.");
        sqlite3_finalize(res);
        return alarm_id;
    }

    rc = sqlite3_bind_blob(res, 4, config_hash_id, sizeof(*config_hash_id), SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to bind config_hash_id parameter for SQL_GET_ALARM_ID.");
        sqlite3_finalize(res);
        return alarm_id;
    }

    while (sqlite3_step_monitored(res) == SQLITE_ROW) {
        alarm_id = (uint32_t) sqlite3_column_int64(res, 0);
        health_log_id = (uint64_t) sqlite3_column_int64(res, 1);
    }

     rc = sqlite3_finalize(res);
     if (unlikely(rc != SQLITE_OK))
         error_report("Failed to finalize the statement while getting an alarm id.");

     if (alarm_id) {
         rc = sqlite3_prepare_v2(db_meta, SQL_GET_EVENT_ID, -1, &res, 0);
         if (rc != SQLITE_OK) {
             error_report("Failed to prepare statement when trying to get an event id");
             return alarm_id;
         }

         rc = sqlite3_bind_int64(res, 1, (sqlite3_int64) health_log_id);
         if (unlikely(rc != SQLITE_OK)) {
             error_report("Failed to bind host_id parameter for SQL_GET_EVENT_ID.");
             sqlite3_finalize(res);
             return alarm_id;
         }

         rc = sqlite3_bind_int64(res, 2, (sqlite3_int64) alarm_id);
         if (unlikely(rc != SQLITE_OK)) {
             error_report("Failed to bind char parameter for SQL_GET_EVENT_ID.");
             sqlite3_finalize(res);
             return alarm_id;
         }

         while (sqlite3_step_monitored(res) == SQLITE_ROW) {
             *next_event_id = (uint32_t) sqlite3_column_int64(res, 0);
         }

         rc = sqlite3_finalize(res);
         if (unlikely(rc != SQLITE_OK))
             error_report("Failed to finalize the statement while getting an alarm id.");
     }

     return alarm_id;
}

#define SQL_GET_ALARM_ID_FROM_TRANSITION_ID "SELECT hld.alarm_id, hl.host_id, hl.chart_context FROM " \
        "health_log_detail hld, health_log hl WHERE hld.transition_id = @transition_id " \
        "and hld.health_log_id = hl.health_log_id"

bool sql_find_alert_transition(const char *transition, void (*cb)(const char *machine_guid, const char *context, time_t alert_id, void *data), void *data)
{
    static __thread sqlite3_stmt *res = NULL;

    char machine_guid[UUID_STR_LEN];

    int rc;
    uuid_t transition_uuid;
    if (uuid_parse(transition, transition_uuid))
        return false;

    if (unlikely(!res)) {
        rc = prepare_statement(db_meta, SQL_GET_ALARM_ID_FROM_TRANSITION_ID, &res);
        if (unlikely(rc != SQLITE_OK)) {
             error_report("Failed to prepare statement when trying to get transition id");
             return false;
        }
    }

    bool ok = false;

    rc = sqlite3_bind_blob(res, 1, &transition_uuid, sizeof(transition_uuid), SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to bind transition");
        goto fail;
    }

    while (sqlite3_step_monitored(res) == SQLITE_ROW) {
        ok = true;
        uuid_unparse_lower(*(uuid_t *) sqlite3_column_blob(res, 1), machine_guid);
        cb(machine_guid, (const char *) sqlite3_column_text(res, 2), sqlite3_column_int(res, 0), data);
    }

fail:
    rc = sqlite3_reset(res);
    if (unlikely(rc != SQLITE_OK))
        error_report("Failed to reset the statement when trying to find transition");

    return ok;
}

#define SQL_BUILD_ALERT_TRANSITION "CREATE TEMP TABLE IF NOT EXISTS v_%p (host_id blob)"

#define SQL_POPULATE_TEMP_ALERT_TRANSITION_TABLE "INSERT INTO v_%p (host_id) VALUES (@host_id)"

#define SQL_SEARCH_ALERT_TRANSITION_SELECT "SELECT " \
    "h.host_id, h.alarm_id, h.config_hash_id, h.name, h.chart, h.chart_name, h.family, h.recipient, h.units, h.exec, " \
    "h.chart_context,  d.when_key, d.duration, d.non_clear_duration, d.flags, d.delay_up_to_timestamp, " \
    "d.info, d.exec_code, d.new_status, d.old_status, d.delay, d.new_value, d.old_value, d.last_repeat, " \
    "d.transition_id, d.global_id, ah.class, ah.type, ah.component, d.exec_run_timestamp"

#define SQL_SEARCH_ALERT_TRANSITION_COMMON_WHERE \
    "h.config_hash_id = ah.hash_id AND h.health_log_id = d.health_log_id"

#define SQL_SEARCH_ALERT_TRANSITION SQL_SEARCH_ALERT_TRANSITION_SELECT " FROM health_log h, health_log_detail d, v_%p t, alert_hash ah " \
    " WHERE h.host_id = t.host_id AND " SQL_SEARCH_ALERT_TRANSITION_COMMON_WHERE " AND ( d.new_status > 2 OR d.old_status > 2 ) AND d.global_id BETWEEN @after AND @before "

#define SQL_SEARCH_ALERT_TRANSITION_DIRECT SQL_SEARCH_ALERT_TRANSITION_SELECT " FROM health_log h, health_log_detail d, alert_hash ah " \
    " WHERE " SQL_SEARCH_ALERT_TRANSITION_COMMON_WHERE " AND transition_id = @transition "

void sql_alert_transitions(
    DICTIONARY *nodes,
    time_t after,
    time_t before,
    const char *context,
    const char *alert_name,
    const char *transition,
    void (*cb)(struct sql_alert_transition_data *, void *),
    void *data,
    bool debug __maybe_unused)
{
    uuid_t transition_uuid;
    char sql[512];
    int rc;
    sqlite3_stmt *res = NULL;
    BUFFER *command = NULL;

    if (unlikely(!nodes))
        return;

    if (transition) {
        if (uuid_parse(transition, transition_uuid)) {
            error_report("Invalid transition given %s", transition);
            return;
        }

        rc = sqlite3_prepare_v2(db_meta, SQL_SEARCH_ALERT_TRANSITION_DIRECT, -1, &res, 0);

        rc = sqlite3_bind_blob(res, 1, &transition_uuid, sizeof(transition_uuid), SQLITE_STATIC);
        if (unlikely(rc != SQLITE_OK)) {
            error_report("Failed to bind transition_id parameter");
            goto fail;
        }
        goto run_query;
    }

    snprintfz(sql, 511, SQL_BUILD_ALERT_TRANSITION, nodes);
    rc = db_execute(db_meta, sql);
    if (rc)
        return;

    snprintfz(sql, 511, SQL_POPULATE_TEMP_ALERT_TRANSITION_TABLE, nodes);

    // Prepare statement to add things
    rc = sqlite3_prepare_v2(db_meta, sql, -1, &res, 0);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to prepare statement to INSERT into v_%p", nodes);
        goto fail_only_drop;
    }

    void *t;
    dfe_start_read(nodes, t) {
        uuid_t host_uuid;
        uuid_parse( t_dfe.name, host_uuid);

        rc = sqlite3_bind_blob(res, 1, &host_uuid, sizeof(host_uuid), SQLITE_STATIC);
        if (unlikely(rc != SQLITE_OK))
            error_report("Failed to bind host_id parameter.");

        rc = sqlite3_step_monitored(res);
        if (rc != SQLITE_DONE)
            error_report("Error while populating temp table");

        rc = sqlite3_reset(res);
        if (rc != SQLITE_OK)
            error_report("Error while resetting parameters");
    }
    dfe_done(t);

    rc = sqlite3_finalize(res);
    if (unlikely(rc != SQLITE_OK)) {
        // log error but continue
        error_report("Failed to finalize statement for sql_alert_transitions temp table population");
    }

    command = buffer_create(MAX_HEALTH_SQL_SIZE, NULL);

    buffer_sprintf(command, SQL_SEARCH_ALERT_TRANSITION, nodes);

    if (context)
        buffer_sprintf(command, " AND h.chart_context = @context");

    if (alert_name)
        buffer_sprintf(command, " AND h.name = @alert_name");

    buffer_strcat(command, " ORDER BY d.global_id DESC");

    rc = sqlite3_prepare_v2(db_meta, buffer_tostring(command), -1, &res, 0);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to prepare statement sql_alert_transitions");
        goto fail_only_drop;
    }

    int param = 1;
    rc = sqlite3_bind_int64(res, param++, (sqlite3_int64)(after * USEC_PER_SEC));
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to bind after parameter");
        goto fail;
    }

    rc = sqlite3_bind_int64(res, param++, (sqlite3_int64)(before * USEC_PER_SEC));
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to bind before parameter");
        goto fail;
    }

    if (context) {
        rc = sqlite3_bind_text(res, param++, context, -1, SQLITE_STATIC);
        if (unlikely(rc != SQLITE_OK)) {
            error_report("Failed to bind context parameter");
            goto fail;
        }
    }

    if (alert_name) {
        rc = sqlite3_bind_text(res, param++, alert_name, -1, SQLITE_STATIC);
        if (unlikely(rc != SQLITE_OK)) {
            error_report("Failed to bind alert_name parameter");
            goto fail;
        }
    }

run_query:;

    struct sql_alert_transition_data atd = {0 };

    while (sqlite3_step(res) == SQLITE_ROW) {
        atd.host_id = (uuid_t *) sqlite3_column_blob(res, 0);
        atd.alarm_id = sqlite3_column_int64(res, 1);
        atd.config_hash_id = (uuid_t *)sqlite3_column_blob(res, 2);
        atd.alert_name = (const char *) sqlite3_column_text(res, 3);
        atd.chart = (const char *) sqlite3_column_text(res, 4);
        atd.chart_name = (const char *) sqlite3_column_text(res, 5);
        atd.family = (const char *) sqlite3_column_text(res, 6);
        atd.recipient = (const char *) sqlite3_column_text(res, 7);
        atd.units = (const char *) sqlite3_column_text(res, 8);
        atd.exec = (const char *) sqlite3_column_text(res, 9);
        atd.chart_context = (const char *) sqlite3_column_text(res, 10);
        atd.when_key = sqlite3_column_int64(res, 11);
        atd.duration = sqlite3_column_int64(res, 12);
        atd.non_clear_duration = sqlite3_column_int64(res, 13);
        atd.flags = sqlite3_column_int64(res, 14);
        atd.delay_up_to_timestamp = sqlite3_column_int64(res, 15);
        atd.info = (const char *) sqlite3_column_text(res, 16);
        atd.exec_code = sqlite3_column_int(res, 17);
        atd.new_status = sqlite3_column_int(res, 18);
        atd.old_status = sqlite3_column_int(res, 19);
        atd.delay = (int) sqlite3_column_int(res, 20);
        atd.new_value = (NETDATA_DOUBLE) sqlite3_column_double(res, 21);
        atd.old_value = (NETDATA_DOUBLE) sqlite3_column_double(res, 22);
        atd.last_repeat = sqlite3_column_int64(res, 23);
        atd.transition_id = (uuid_t *) sqlite3_column_blob(res, 24);
        atd.global_id = sqlite3_column_int64(res, 25);
        atd.classification = (const char *) sqlite3_column_text(res, 26);
        atd.type = (const char *) sqlite3_column_text(res, 27);
        atd.component = (const char *) sqlite3_column_text(res, 28);
        atd.exec_run_timestamp = sqlite3_column_int64(res, 29);

        cb(&atd, data);
    }

fail:
    rc = sqlite3_finalize(res);
    if (unlikely(rc != SQLITE_OK))
        error_report("Failed to finalize statement for sql_alert_transitions");

fail_only_drop:
    if (likely(!transition)) {
        (void)snprintfz(sql, 511, "DROP TABLE IF EXISTS v_%p", nodes);
        (void)db_execute(db_meta, sql);
        buffer_free(command);
    }
}

#define SQL_BUILD_CONFIG_TARGET_LIST "CREATE TEMP TABLE IF NOT EXISTS c_%p (hash_id blob)"

#define SQL_POPULATE_TEMP_CONFIG_TARGET_TABLE "INSERT INTO c_%p (hash_id) VALUES (@hash_id)"

#define SQL_SEARCH_CONFIG_LIST "SELECT ah.hash_id, alarm, template, on_key, class, component, type, os, hosts, lookup, every, " \
    " units, calc, families, plugin, module, charts, green, red, warn, crit, " \
    " exec, to_key, info, delay, options, repeat, host_labels, p_db_lookup_dimensions, p_db_lookup_method, " \
    " p_db_lookup_options, p_db_lookup_after, p_db_lookup_before, p_update_every, source, chart_labels " \
    " FROM alert_hash ah, c_%p t where ah.hash_id = t.hash_id"

int sql_get_alert_configuration(
    DICTIONARY *configs,
    void (*cb)(struct sql_alert_config_data *, void *),
    void *data,
    bool debug __maybe_unused)
{
    int added = -1;
    char sql[512];
    int rc;
    sqlite3_stmt *res = NULL;
    BUFFER *command = NULL;

    if (unlikely(!configs))
        return added;

    snprintfz(sql, 511, SQL_BUILD_CONFIG_TARGET_LIST, configs);
    rc = db_execute(db_meta, sql);
    if (rc)
        return added;

    snprintfz(sql, 511, SQL_POPULATE_TEMP_CONFIG_TARGET_TABLE, configs);

    // Prepare statement to add things
    rc = sqlite3_prepare_v2(db_meta, sql, -1, &res, 0);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to prepare statement to INSERT into c_%p", configs);
        goto fail_only_drop;
    }

    void *t;
    dfe_start_read(configs, t) {
        uuid_t hash_id;
        uuid_parse( t_dfe.name, hash_id);

        rc = sqlite3_bind_blob(res, 1, &hash_id, sizeof(hash_id), SQLITE_STATIC);
        if (unlikely(rc != SQLITE_OK))
            error_report("Failed to bind host_id parameter.");

        rc = sqlite3_step_monitored(res);
        if (rc != SQLITE_DONE)
            error_report("Error while populating temp table");

        rc = sqlite3_reset(res);
        if (rc != SQLITE_OK)
            error_report("Error while resetting parameters");
    }
    dfe_done(t);

    rc = sqlite3_finalize(res);
    if (unlikely(rc != SQLITE_OK)) {
        // log error but continue
        error_report("Failed to finalize statement for sql_get_alert_configuration temp table population");
    }

    command = buffer_create(MAX_HEALTH_SQL_SIZE, NULL);

    buffer_sprintf(command, SQL_SEARCH_CONFIG_LIST, configs);

    rc = sqlite3_prepare_v2(db_meta, buffer_tostring(command), -1, &res, 0);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to prepare statement sql_get_alert_configuration");
        goto fail_only_drop;
    }

    struct sql_alert_config_data acd = {0 };

    added = 0;
    int param;
    while (sqlite3_step(res) == SQLITE_ROW) {
        param = 0;
        acd.config_hash_id = (uuid_t *) sqlite3_column_blob(res, param++);
        acd.name = (const char *) sqlite3_column_text(res, param++);
        acd.selectors.on_template = (const char *) sqlite3_column_text(res, param++);
        acd.selectors.on_key = (const char *) sqlite3_column_text(res, param++);
        acd.classification = (const char *) sqlite3_column_text(res, param++);
        acd.component = (const char *) sqlite3_column_text(res, param++);
        acd.type = (const char *) sqlite3_column_text(res, param++);
        acd.selectors.os = (const char *) sqlite3_column_text(res, param++);
        acd.selectors.hosts = (const char *) sqlite3_column_text(res, param++);
        acd.value.db.lookup = (const char *) sqlite3_column_text(res, param++);
        acd.value.every = (const char *) sqlite3_column_text(res, param++);
        acd.value.units = (const char *) sqlite3_column_text(res, param++);
        acd.value.calc = (const char *) sqlite3_column_text(res, param++);
        acd.selectors.families = (const char *) sqlite3_column_text(res, param++);
        acd.selectors.plugin = (const char *) sqlite3_column_text(res, param++);
        acd.selectors.module = (const char *) sqlite3_column_text(res, param++);
        acd.selectors.charts = (const char *) sqlite3_column_text(res, param++);
        acd.status.green = (const char *) sqlite3_column_text(res, param++);
        acd.status.red = (const char *) sqlite3_column_text(res, param++);
        acd.status.warn = (const char *) sqlite3_column_text(res, param++);
        acd.status.crit = (const char *) sqlite3_column_text(res, param++);
        acd.notification.exec = (const char *) sqlite3_column_text(res, param++);
        acd.notification.to_key = (const char *) sqlite3_column_text(res, param++);
        acd.info = (const char *) sqlite3_column_text(res, param++);
        acd.notification.delay = (const char *) sqlite3_column_text(res, param++);
        acd.notification.options = (const char *) sqlite3_column_text(res, param++);
        acd.notification.repeat = (const char *) sqlite3_column_text(res, param++);
        acd.selectors.host_labels = (const char *) sqlite3_column_text(res, param++);
        acd.value.db.dimensions = (const char *) sqlite3_column_text(res, param++);
        acd.value.db.method = (const char *) sqlite3_column_text(res, param++);
        acd.value.db.options = (uint32_t) sqlite3_column_int(res, param++);
        acd.value.db.after = (int32_t) sqlite3_column_int(res, param++);
        acd.value.db.before = (int32_t) sqlite3_column_int(res, param++);
        acd.value.update_every = (int32_t) sqlite3_column_int(res, param++);
        acd.source = (const char *) sqlite3_column_text(res, param++);
        acd.selectors.chart_labels = (const char *) sqlite3_column_text(res, param++);

        cb(&acd, data);
        added++;
    }

    rc = sqlite3_finalize(res);
    if (unlikely(rc != SQLITE_OK))
        error_report("Failed to finalize statement for sql_get_alert_configuration");

fail_only_drop:
    (void)snprintfz(sql, 511, "DROP TABLE IF EXISTS c_%p", configs);
    (void)db_execute(db_meta, sql);
    buffer_free(command);
    return added;
}

#define SQL_FETCH_CHART_NAME "SELECT chart_name FROM health_log where host_id = @host_id LIMIT 1;"
bool is_chart_name_populated(uuid_t  *host_uuid)
{
    sqlite3_stmt *res = NULL;
    int rc;

    bool status = true;

    rc = sqlite3_prepare_v2(db_meta, SQL_FETCH_CHART_NAME, -1, &res, 0);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to prepare statement to check health_log chart_name");
        return true;
    }

    rc = sqlite3_bind_blob(res, 1, host_uuid, sizeof(*host_uuid), SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to bind host_id for health_log chart_name check");
        goto fail;
    }

    rc = sqlite3_step_monitored(res);
    if (likely(rc == SQLITE_ROW))
        status = sqlite3_column_type(res, 0) != SQLITE_NULL;
fail:

    rc = sqlite3_finalize(res);
    if (unlikely(rc != SQLITE_OK))
        error_report("Failed to finalize the prepared statement for health_log chart_name check");

    return status;
}

#define SQL_POPULATE_CHART_NAME " UPDATE health_log SET chart_name = upd.chart_name FROM " \
    "(SELECT c.type || '.' || IFNULL(c.name, c.id) AS chart_name, hl.host_id, hl.health_log_id FROM " \
    "chart c, health_log hl WHERE (c.type || '.' || c.id) = hl.chart AND c.host_id = hl.host_id " \
    "AND hl.host_id = @host_id) AS upd WHERE health_log.host_id = upd.host_id " \
    "AND health_log.health_log_id = upd.health_log_id"

void chart_name_populate(uuid_t *host_uuid)
{
    sqlite3_stmt *res = NULL;
    int rc;

    rc = sqlite3_prepare_v2(db_meta, SQL_POPULATE_CHART_NAME, -1, &res, 0);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to prepare statement to update health_log chart_name");
        return;
    }

    rc = sqlite3_bind_blob(res, 1, host_uuid, sizeof(*host_uuid), SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to bind host_id for health_log chart_name update");
        goto fail;
    }

    rc = execute_insert(res);
    if (unlikely(rc != SQLITE_DONE))
        error_report("Failed to update chart name in health_log, rc = %d", rc);

fail:

    rc = sqlite3_finalize(res);
    if (unlikely(rc != SQLITE_OK))
        error_report("Failed to finalize the prepared statement for health_log chart_name update");
}
