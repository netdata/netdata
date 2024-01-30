// SPDX-License-Identifier: GPL-3.0-or-later

#include "sqlite_health.h"
#include "sqlite_functions.h"
#include "sqlite_db_migration.h"
#include "../../health/health_internals.h"

#define MAX_HEALTH_SQL_SIZE 2048
#define SQLITE3_BIND_STRING_OR_NULL(res, key, param)                                                                   \
    ((key) ? sqlite3_bind_text(res, param, string2str(key), -1, SQLITE_STATIC) : sqlite3_bind_null(res, param))

#define SQLITE3_COLUMN_STRINGDUP_OR_NULL(res, param)                                                                   \
    ({                                                                                                                 \
        int _param = (param);                                                                                          \
        sqlite3_column_type((res), (_param)) != SQLITE_NULL ?                                                          \
            string_strdupz((char *)sqlite3_column_text((res), (_param))) :                                             \
            NULL;                                                                                                      \
    })

/* Health related SQL queries
   Updates an entry in the table
*/
#define SQL_UPDATE_HEALTH_LOG                                                                                          \
    "UPDATE health_log_detail SET updated_by_id = @updated_by, flags = @flags, exec_run_timestamp = @exec_time, "      \
    "exec_code = @exec_code WHERE unique_id = @unique_id AND alarm_id = @alarm_id AND transition_id = @transaction"

static void sql_health_alarm_log_update(RRDHOST *host, ALARM_ENTRY *ae)
{
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

#define SQL_INSERT_HEALTH_LOG                                                                                          \
    "INSERT INTO health_log (host_id, alarm_id, "                                                                      \
    "config_hash_id, name, chart, exec, recipient, units, chart_context, last_transition_id, chart_name) "             \
    "VALUES (@host_id,@alarm_id, @config_hash_id,@name,@chart,@exec,@recipient,@units,@chart_context,"                 \
    "@last_transition_id,@chart_name) ON CONFLICT (host_id, alarm_id) DO UPDATE "                                      \
    "SET last_transition_id = excluded.last_transition_id, chart_name = excluded.chart_name, "                         \
    "config_hash_id=excluded.config_hash_id RETURNING health_log_id"

#define SQL_INSERT_HEALTH_LOG_DETAIL                                                                                         \
    "INSERT INTO health_log_detail (health_log_id, unique_id, alarm_id, alarm_event_id, "                                    \
    "updated_by_id, updates_id, when_key, duration, non_clear_duration, flags, exec_run_timestamp, delay_up_to_timestamp, "  \
    "info, exec_code, new_status, old_status, delay, new_value, old_value, last_repeat, transition_id, global_id, summary) " \
    "VALUES (@health_log_id,@unique_id,@alarm_id,@alarm_event_id,@updated_by_id,@updates_id,@when_key,@duration,"            \
    "@non_clear_duration,@flags,@exec_run_timestamp,@delay_up_to_timestamp, @info,@exec_code,@new_status,@old_status,"       \
    "@delay,@new_value,@old_value,@last_repeat,@transition_id,@global_id,@summary)"

static void sql_health_alarm_log_insert(RRDHOST *host, ALARM_ENTRY *ae) {
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

    rc = SQLITE3_BIND_STRING_OR_NULL(res, ae->name, 4);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to bind name parameter for SQL_INSERT_HEALTH_LOG");
        goto failed;
    }

    rc = SQLITE3_BIND_STRING_OR_NULL(res, ae->chart, 5);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to bind chart parameter for SQL_INSERT_HEALTH_LOG");
        goto failed;
    }

    rc = SQLITE3_BIND_STRING_OR_NULL(res, ae->exec, 6);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to bind exec parameter for SQL_INSERT_HEALTH_LOG");
        goto failed;
    }

    rc = SQLITE3_BIND_STRING_OR_NULL(res, ae->recipient, 7);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to bind recipient parameter for SQL_INSERT_HEALTH_LOG");
        goto failed;
    }

    rc = SQLITE3_BIND_STRING_OR_NULL(res, ae->units, 8);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to bind host_id parameter to store node instance information");
        goto failed;
    }

    rc = SQLITE3_BIND_STRING_OR_NULL(res, ae->chart_context, 9);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to bind chart_context parameter for SQL_INSERT_HEALTH_LOG");
        goto failed;
    }

    rc = sqlite3_bind_blob(res, 10, &ae->transition_id, sizeof(ae->transition_id), SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to bind transition_id parameter for SQL_INSERT_HEALTH_LOG");
        goto failed;
    }

    rc = SQLITE3_BIND_STRING_OR_NULL(res, ae->chart_name, 11);
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

    rc = SQLITE3_BIND_STRING_OR_NULL(res, ae->info, 13);
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

    rc = SQLITE3_BIND_STRING_OR_NULL(res, ae->summary, 23);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to bind summary parameter for SQL_INSERT_HEALTH_LOG_DETAIL");
        goto failed;
    }

    rc = execute_insert(res);
    if (unlikely(rc != SQLITE_DONE)) {
        error_report("HEALTH [%s]: Failed to execute SQL_INSERT_HEALTH_LOG_DETAIL, rc = %d", rrdhost_hostname(host), rc);
        goto failed;
    }

    ae->flags |= HEALTH_ENTRY_FLAG_SAVED;

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
            sql_queue_alarm_to_aclk(host, ae, false);
        }
#endif
    }
}

/*
 *
 * Health related SQL queries
 * Cleans up the health_log_detail table on a non-claimed or claimed host
 *
 */

#define SQL_CLEANUP_HEALTH_LOG_DETAIL_NOT_CLAIMED                                                                      \
    "DELETE FROM health_log_detail WHERE health_log_id IN "                                                            \
    "(SELECT health_log_id FROM health_log WHERE host_id = @host_id) AND when_key < UNIXEPOCH() - @history "           \
    "AND updated_by_id <> 0 AND transition_id NOT IN "                                                                 \
    "(SELECT last_transition_id FROM health_log hl WHERE hl.host_id = @host_id)"

#define SQL_CLEANUP_HEALTH_LOG_DETAIL_CLAIMED(guid)                                                                    \
    "DELETE from health_log_detail WHERE unique_id NOT IN "                                                            \
    "(SELECT filtered_alert_unique_id FROM aclk_alert_%s) "                                                            \
    "AND unique_id IN (SELECT hld.unique_id FROM health_log hl, health_log_detail hld WHERE "                          \
    "hl.host_id = @host_id AND hl.health_log_id = hld.health_log_id) "                                                 \
    "AND health_log_id IN (SELECT health_log_id FROM health_log WHERE host_id = @host_id) "                            \
    "AND when_key < unixepoch() - @history "                                                                           \
    "AND updated_by_id <> 0 AND transition_id NOT IN "                                                                 \
    "(SELECT last_transition_id FROM health_log hl WHERE hl.host_id = @host_id)",                                      \
        guid

void sql_health_alarm_log_cleanup(RRDHOST *host, bool claimed) {
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
    snprintfz(command, sizeof(command) - 1, "aclk_alert_%s", uuid_str);

    bool aclk_table_exists = table_exists_in_database(db_meta, command);

    char *sql = SQL_CLEANUP_HEALTH_LOG_DETAIL_NOT_CLAIMED;

    if (claimed && aclk_table_exists) {
        snprintfz(command, sizeof(command) - 1, SQL_CLEANUP_HEALTH_LOG_DETAIL_CLAIMED(uuid_str));
        sql = command;
    }

    rc = sqlite3_prepare_v2(db_meta, sql, -1, &res, 0);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to prepare statement to cleanup health log detail table (claimed)");
        return;
    }

    rc = sqlite3_bind_blob(res, 1, &host->host_uuid, sizeof(host->host_uuid), SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to bind first host_id for sql_health_alarm_log_cleanup.");
        goto done;
    }

    rc = sqlite3_bind_int64(res, 2, (sqlite3_int64)host->health_log.health_log_history);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to bind health log history for sql_health_alarm_log_cleanup.");
        goto done;
    }

    rc = sqlite3_step_monitored(res);
    if (unlikely(rc != SQLITE_DONE))
        error_report("Failed to cleanup health log detail table, rc = %d", rc);

    if (aclk_table_exists)
        sql_aclk_alert_clean_dead_entries(host);

done:
    rc = sqlite3_finalize(res);
    if (unlikely(rc != SQLITE_OK))
        error_report("Failed to finalize the prepared statement to cleanup health log detail table (claimed)");
}

#define SQL_INJECT_REMOVED                                                                                                      \
    "INSERT INTO health_log_detail (health_log_id, unique_id, alarm_id, alarm_event_id, updated_by_id, updates_id, when_key, "  \
    "duration, non_clear_duration, flags, exec_run_timestamp, delay_up_to_timestamp, info, exec_code, new_status, old_status, " \
    "delay, new_value, old_value, last_repeat, transition_id, global_id, summary) "                                             \
    "SELECT health_log_id, ?1, ?2, ?3, 0, ?4, UNIXEPOCH(), 0, 0, flags, exec_run_timestamp, UNIXEPOCH(), info, exec_code, -2, " \
    "new_status, delay, NULL, new_value, 0, ?5, NOW_USEC(0), summary FROM health_log_detail WHERE unique_id = ?6 AND transition_id = ?7"

#define SQL_INJECT_REMOVED_UPDATE_DETAIL                                                                               \
    "UPDATE health_log_detail SET flags = flags | ?1, updated_by_id = ?2 WHERE unique_id = ?3 AND transition_id = ?4"

#define SQL_INJECT_REMOVED_UPDATE_LOG                                                                                  \
    "UPDATE health_log SET last_transition_id = ?1 WHERE alarm_id = ?2 AND last_transition_id = ?3 AND host_id = ?4"

void sql_inject_removed_status(
    RRDHOST *host,
    uint32_t alarm_id,
    uint32_t alarm_event_id,
    uint32_t unique_id,
    uint32_t max_unique_id,
    uuid_t *prev_transition_id)
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
    if (unlikely(rc != SQLITE_DONE))
        error_report("HEALTH [N/A]: Failed to execute SQL_INJECT_REMOVED_UPDATE_DETAIL, rc = %d", rc);

failed:
    if (unlikely(sqlite3_finalize(res) != SQLITE_OK))
        error_report("HEALTH [N/A]: Failed to finalize the prepared statement for injecting removed event.");
}

#define SQL_SELECT_MAX_UNIQUE_ID                                                                                       \
    "SELECT MAX(hld.unique_id) FROM health_log_detail hld, health_log hl "                                             \
    "WHERE hl.host_id = @host_id AND hl.health_log_id = hld.health_log_id"

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

#define SQL_SELECT_LAST_STATUSES                                                                                       \
     "SELECT hld.new_status, hld.unique_id, hld.alarm_id, hld.alarm_event_id, hld.transition_id FROM health_log hl, "  \
     "health_log_detail hld WHERE hl.host_id = @host_id AND hl.last_transition_id = hld.transition_id"

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

        status = (RRDCALC_STATUS)sqlite3_column_int(res, 0);
        unique_id = (uint32_t)sqlite3_column_int64(res, 1);
        alarm_id = (uint32_t)sqlite3_column_int64(res, 2);
        alarm_event_id = (uint32_t)sqlite3_column_int64(res, 3);
        uuid_copy(transition_id, *((uuid_t *)sqlite3_column_blob(res, 4)));

        if (unlikely(status != RRDCALC_STATUS_REMOVED)) {
           if (unlikely(!max_unique_id))
               max_unique_id = sql_get_max_unique_id(host);

           sql_inject_removed_status(host, alarm_id, alarm_event_id, unique_id, ++max_unique_id, &transition_id);
        }
    }

    rc = sqlite3_finalize(res);
    if (unlikely(rc != SQLITE_OK))
        error_report("Failed to finalize the statement");
}

/* Health related SQL queries
   Load from the health log table
*/
#define SQL_LOAD_HEALTH_LOG                                                                                            \
    "SELECT hld.unique_id, hld.alarm_id, hld.alarm_event_id, hl.config_hash_id, hld.updated_by_id, "                   \
    "hld.updates_id, hld.when_key, hld.duration, hld.non_clear_duration, hld.flags, hld.exec_run_timestamp, "          \
    "hld.delay_up_to_timestamp, hl.name, hl.chart, hl.exec, hl.recipient, ah.source, hl.units, "                       \
    "hld.info, hld.exec_code, hld.new_status, hld.old_status, hld.delay, hld.new_value, hld.old_value, "               \
    "hld.last_repeat, ah.class, ah.component, ah.type, hl.chart_context, hld.transition_id, hld.global_id, "           \
    "hl.chart_name, hld.summary FROM health_log hl, alert_hash ah, health_log_detail hld "                             \
    "WHERE hl.config_hash_id = ah.hash_id and hl.host_id = @host_id and hl.last_transition_id = hld.transition_id"

void sql_health_alarm_log_load(RRDHOST *host)
{
    sqlite3_stmt *res = NULL;
    int ret;
    ssize_t errored = 0, loaded = 0;

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

        //need name and chart
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

        // Check if we got last_repeat field
        time_t last_repeat = (time_t)sqlite3_column_int64(res, 25);

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

        ae->exec = SQLITE3_COLUMN_STRINGDUP_OR_NULL(res, 14);
        ae->recipient = SQLITE3_COLUMN_STRINGDUP_OR_NULL(res, 15);
        ae->source = SQLITE3_COLUMN_STRINGDUP_OR_NULL(res, 16);
        ae->units = SQLITE3_COLUMN_STRINGDUP_OR_NULL(res, 17);
        ae->info = SQLITE3_COLUMN_STRINGDUP_OR_NULL(res, 18);

        ae->exec_code   = (int) sqlite3_column_int(res, 19);
        ae->new_status  = (RRDCALC_STATUS) sqlite3_column_int(res, 20);
        ae->old_status  = (RRDCALC_STATUS)sqlite3_column_int(res, 21);
        ae->delay       = (int) sqlite3_column_int(res, 22);

        ae->new_value   = (NETDATA_DOUBLE) sqlite3_column_double(res, 23);
        ae->old_value   = (NETDATA_DOUBLE) sqlite3_column_double(res, 24);

        ae->last_repeat = last_repeat;

        ae->classification = SQLITE3_COLUMN_STRINGDUP_OR_NULL(res, 26);
        ae->component = SQLITE3_COLUMN_STRINGDUP_OR_NULL(res, 27);
        ae->type = SQLITE3_COLUMN_STRINGDUP_OR_NULL(res, 28);
        ae->chart_context = SQLITE3_COLUMN_STRINGDUP_OR_NULL(res, 29);

        if (sqlite3_column_type(res, 30) != SQLITE_NULL)
            uuid_copy(ae->transition_id, *((uuid_t *)sqlite3_column_blob(res, 30)));

        if (sqlite3_column_type(res, 31) != SQLITE_NULL)
            ae->global_id = sqlite3_column_int64(res, 31);

        ae->chart_name = SQLITE3_COLUMN_STRINGDUP_OR_NULL(res, 32);
        ae->summary = SQLITE3_COLUMN_STRINGDUP_OR_NULL(res, 33);

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

    nd_log(NDLS_DAEMON, errored ? NDLP_WARNING : NDLP_DEBUG,
           "[%s]: Table health_log, loaded %zd alarm entries, errors in %zd entries.",
           rrdhost_hostname(host), loaded, errored);

    ret = sqlite3_finalize(res);
    if (unlikely(ret != SQLITE_OK))
        error_report("Failed to finalize the health log read statement");
}

/*
 * Store an alert config hash in the database
 */
#define SQL_STORE_ALERT_CONFIG_HASH                                                                                     \
    "insert or replace into alert_hash (hash_id, date_updated, alarm, template, "                                       \
    "on_key, class, component, type, os, hosts, lookup, every, units, calc, plugin, module, "                           \
    "charts, green, red, warn, crit, exec, to_key, info, delay, options, repeat, host_labels, "                         \
    "p_db_lookup_dimensions, p_db_lookup_method, p_db_lookup_options, p_db_lookup_after, "                              \
    "p_db_lookup_before, p_update_every, source, chart_labels, summary) values (@hash_id,UNIXEPOCH(),@alarm,@template," \
    "@on_key,@class,@component,@type,@os,@hosts,@lookup,@every,@units,@calc,@plugin,@module,"                           \
    "@charts,@green,@red,@warn,@crit,@exec,@to_key,@info,@delay,@options,@repeat,@host_labels,"                         \
    "@p_db_lookup_dimensions,@p_db_lookup_method,@p_db_lookup_options,@p_db_lookup_after,"                              \
    "@p_db_lookup_before,@p_update_every,@source,@chart_labels,@summary)"

int sql_alert_store_config(RRD_ALERT_PROTOTYPE *ap __maybe_unused)
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
    BUFFER *buf = buffer_create(128, NULL);

    rc = sqlite3_bind_blob(res, ++param, &ap->config.hash_id, sizeof(ap->config.hash_id), SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    if (ap->match.is_template)
        rc = SQLITE3_BIND_STRING_OR_NULL(res, NULL, ++param);
    else
        rc = SQLITE3_BIND_STRING_OR_NULL(res, ap->config.name, ++param);

    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    if (ap->match.is_template)
        rc = SQLITE3_BIND_STRING_OR_NULL(res, ap->config.name, ++param);
    else
        rc = SQLITE3_BIND_STRING_OR_NULL(res, NULL, ++param);

    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    if (ap->match.is_template)
        rc = SQLITE3_BIND_STRING_OR_NULL(res, ap->match.on.context, ++param);
    else
        rc = SQLITE3_BIND_STRING_OR_NULL(res, ap->match.on.chart, ++param);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    rc = SQLITE3_BIND_STRING_OR_NULL(res, ap->config.classification, ++param);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    rc = SQLITE3_BIND_STRING_OR_NULL(res, ap->config.component, ++param);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    rc = SQLITE3_BIND_STRING_OR_NULL(res, ap->config.type, ++param);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    rc = SQLITE3_BIND_STRING_OR_NULL(res, ap->match.os, ++param);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    rc = SQLITE3_BIND_STRING_OR_NULL(res, ap->match.host, ++param);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    rc = SQLITE3_BIND_STRING_OR_NULL(res, ap->config.lookup, ++param);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    rc = sqlite3_bind_int(res,  ++param, ap->config.update_every);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    rc = SQLITE3_BIND_STRING_OR_NULL(res, ap->config.units, ++param);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    if (ap->config.calculation)
        rc = sqlite3_bind_text(res, ++param, expression_source(ap->config.calculation), -1, SQLITE_STATIC);
    else
        rc = sqlite3_bind_null(res, ++param);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    rc = SQLITE3_BIND_STRING_OR_NULL(res, ap->match.plugin, ++param);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    rc = SQLITE3_BIND_STRING_OR_NULL(res, ap->match.module, ++param);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    rc = SQLITE3_BIND_STRING_OR_NULL(res, ap->match.charts, ++param);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    rc = sqlite3_bind_double(res, ++param, ap->config.green);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    rc = sqlite3_bind_double(res, ++param, ap->config.red);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    if (ap->config.warning)
        rc = sqlite3_bind_text(res, ++param, expression_source(ap->config.warning), -1, SQLITE_STATIC);
    else
        rc = sqlite3_bind_null(res, ++param);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    if (ap->config.critical)
        rc = sqlite3_bind_text(res, ++param, expression_source(ap->config.critical), -1, SQLITE_STATIC);
    else
        rc = sqlite3_bind_null(res, ++param);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    rc = SQLITE3_BIND_STRING_OR_NULL(res, ap->config.exec, ++param);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    rc = SQLITE3_BIND_STRING_OR_NULL(res, ap->config.recipient, ++param);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    rc = SQLITE3_BIND_STRING_OR_NULL(res, ap->config.info, ++param);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    if (ap->config.delay_up_duration)
        buffer_sprintf(buf, "up %ds ", ap->config.delay_up_duration);

    if (ap->config.delay_down_duration)
        buffer_sprintf(buf, "down %ds ", ap->config.delay_down_duration);

    if (ap->config.delay_multiplier)
        buffer_sprintf(buf, "multiplier %.1f ", ap->config.delay_multiplier);

    if (ap->config.delay_max_duration)
        buffer_sprintf(buf, "max %ds", ap->config.delay_max_duration);

    // delay
    rc = sqlite3_bind_text(res, ++param, buffer_tostring(buf), -1, SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    if (ap->config.alert_action_options & ALERT_ACTION_OPTION_NO_CLEAR_NOTIFICATION)
        rc = sqlite3_bind_text(res, ++param, "no-clear-notification", -1, SQLITE_STATIC);
    else
        rc = sqlite3_bind_null(res, ++param);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    rc = sqlite3_bind_int(res, ++param, ap->config.update_every);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    rc = SQLITE3_BIND_STRING_OR_NULL(res, ap->match.host_labels, ++param);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    if (ap->config.after) {
        rc = SQLITE3_BIND_STRING_OR_NULL(res, ap->config.dimensions, ++param);
        if (unlikely(rc != SQLITE_OK))
            goto bind_fail;

        rc = sqlite3_bind_text(res, ++param, time_grouping_id2txt(ap->config.group), -1, SQLITE_STATIC);
        if (unlikely(rc != SQLITE_OK))
            goto bind_fail;

        rc = sqlite3_bind_int(res, ++param, (int) ap->config.options);
        if (unlikely(rc != SQLITE_OK))
            goto bind_fail;

        rc = sqlite3_bind_int64(res, ++param, (int) ap->config.after);
        if (unlikely(rc != SQLITE_OK))
            goto bind_fail;

        rc = sqlite3_bind_int64(res, ++param, (int) ap->config.before);
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

    rc = sqlite3_bind_int(res, ++param, ap->config.update_every);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    rc = SQLITE3_BIND_STRING_OR_NULL(res, ap->config.source, ++param);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    rc = SQLITE3_BIND_STRING_OR_NULL(res, ap->match.chart_labels, ++param);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    rc = SQLITE3_BIND_STRING_OR_NULL(res, ap->config.summary, ++param);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    rc = execute_insert(res);
    if (unlikely(rc != SQLITE_DONE))
        error_report("Failed to store alert config, rc = %d", rc);

    rc = sqlite3_reset(res);
    if (unlikely(rc != SQLITE_OK))
        error_report("Failed to reset statement in alert hash_id store function, rc = %d", rc);

    buffer_free(buf);
    return 0;

bind_fail:
    buffer_free(buf);
    error_report("Failed to bind parameter %d to store alert hash_id, rc = %d", param, rc);
    rc = sqlite3_reset(res);
    if (unlikely(rc != SQLITE_OK))
        error_report("Failed to reset statement in alert hash_id store function, rc = %d", rc);
    return 1;
}

#define SQL_SELECT_HEALTH_LAST_EXECUTED_EVENT                                                                          \
    "SELECT hld.new_status FROM health_log hl, health_log_detail hld "                                                 \
    "WHERE hl.host_id = @host_id AND hl.alarm_id = @alarm_id AND hld.unique_id != @unique_id AND hld.flags & @flags "  \
    "AND hl.health_log_id = hld.health_log_id ORDER BY hld.unique_id DESC LIMIT 1"

int sql_health_get_last_executed_event(RRDHOST *host, ALARM_ENTRY *ae, RRDCALC_STATUS *last_executed_status)
{
    int rc = 0, ret = -1;
    sqlite3_stmt *res = NULL;

    rc = sqlite3_prepare_v2(db_meta, SQL_SELECT_HEALTH_LAST_EXECUTED_EVENT, -1, &res, 0);
    if (rc != SQLITE_OK) {
        error_report("Failed to prepare statement when trying to get last executed status");
        return ret;
    }

    rc = sqlite3_bind_blob(res, 1, &host->host_uuid, sizeof(host->host_uuid), SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to bind host_id parameter for SQL_SELECT_HEALTH_LAST_EXECUTED_EVENT.");
        goto done;
    }

    rc = sqlite3_bind_int(res, 2, (int) ae->alarm_id);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to bind alarm_id parameter for SQL_SELECT_HEALTH_LAST_EXECUTED_EVENT.");
        goto done;
    }

    rc = sqlite3_bind_int(res, 3, (int) ae->unique_id);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to bind unique_id parameter for SQL_SELECT_HEALTH_LAST_EXECUTED_EVENT.");
        goto done;
    }

    rc = sqlite3_bind_int(res, 4, (uint32_t) HEALTH_ENTRY_FLAG_EXEC_RUN);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to bind unique_id parameter for SQL_SELECT_HEALTH_LAST_EXECUTED_EVENT.");
        goto done;
    }

    ret = 0;
    while (sqlite3_step_monitored(res) == SQLITE_ROW) {
        *last_executed_status  = (RRDCALC_STATUS) sqlite3_column_int(res, 0);
        ret = 1;
    }

done:
     rc = sqlite3_finalize(res);
     if (unlikely(rc != SQLITE_OK))
         error_report("Failed to finalize the statement.");

     return ret;
}

#define SQL_SELECT_HEALTH_LOG                                                                                          \
     "SELECT hld.unique_id, hld.alarm_id, hld.alarm_event_id, hl.config_hash_id, hld.updated_by_id, hld.updates_id, "  \
     "hld.when_key, hld.duration, hld.non_clear_duration, hld.flags, hld.exec_run_timestamp, "                         \
     "hld.delay_up_to_timestamp, hl.name, hl.chart, hl.exec, hl.recipient, ah.source, "                                \
     "hl.units, hld.info, hld.exec_code, hld.new_status, hld.old_status, hld.delay, hld.new_value, hld.old_value, "    \
     "hld.last_repeat, ah.class, ah.component, ah.type, hl.chart_context, hld.transition_id, hld.summary "             \
     "FROM health_log hl, alert_hash ah, health_log_detail hld WHERE hl.config_hash_id = ah.hash_id and "              \
     "hl.health_log_id = hld.health_log_id and hl.host_id = @host_id AND hld.unique_id > @after "

void sql_health_alarm_log2json(RRDHOST *host, BUFFER *wb, time_t after, const char *chart)
{
     unsigned int max = host->health_log.max;

     static __thread sqlite3_stmt *stmt_no_chart = NULL;
     static __thread sqlite3_stmt *stmt_with_chart = NULL;

     sqlite3_stmt **active_stmt;
     sqlite3_stmt *stmt_query;

     int rc;

     active_stmt = chart ? &stmt_with_chart : &stmt_no_chart;

     if (!*active_stmt) {

         BUFFER *command = buffer_create(MAX_HEALTH_SQL_SIZE, NULL);
         buffer_sprintf(command, SQL_SELECT_HEALTH_LOG);

         if (chart)
            buffer_strcat(command, " AND hl.chart = @chart ");

         buffer_strcat(command, " ORDER BY hld.unique_id DESC LIMIT @limit");

         rc = prepare_statement(db_meta, buffer_tostring(command), active_stmt);
         buffer_free(command);

         if (unlikely(rc != SQLITE_OK)) {
            error_report("Failed to prepare statement SQL_SELECT_HEALTH_LOG");
            return;
         }
     }

     stmt_query = *active_stmt;

     int param = 0;
     rc = sqlite3_bind_blob(stmt_query, ++param, &host->host_uuid, sizeof(host->host_uuid), SQLITE_STATIC);
     if (unlikely(rc != SQLITE_OK)) {
         error_report("Failed to bind host_id for SQL_SELECT_HEALTH_LOG.");
         goto finish;
     }

     rc = sqlite3_bind_int64(stmt_query, ++param, after);
     if (unlikely(rc != SQLITE_OK)) {
         error_report("Failed to bind after for SQL_SELECT_HEALTH_LOG.");
         goto finish;
     }

     if (chart) {
         rc = sqlite3_bind_text(stmt_query, ++param, chart, -1, SQLITE_STATIC);
         if (unlikely(rc != SQLITE_OK)) {
            error_report("Failed to bind after for SQL_SELECT_HEALTH_LOG.");
            goto finish;
         }
     }

     rc = sqlite3_bind_int64(stmt_query, ++param, max);
     if (unlikely(rc != SQLITE_OK)) {
         error_report("Failed to bind max lines for SQL_SELECT_HEALTH_LOG.");
         goto finish;
     }

     buffer_json_initialize(wb, "\"", "\"", 0, false, BUFFER_JSON_OPTIONS_DEFAULT);
     buffer_json_member_add_array(wb, NULL);

     while (sqlite3_step(stmt_query) == SQLITE_ROW) {
         char old_value_string[100 + 1];
         char new_value_string[100 + 1];

         char config_hash_id[UUID_STR_LEN];
         uuid_unparse_lower(*((uuid_t *)sqlite3_column_blob(stmt_query, 3)), config_hash_id);

         char transition_id[UUID_STR_LEN] = {0};
         if (sqlite3_column_type(stmt_query, 30) != SQLITE_NULL)
            uuid_unparse_lower(*((uuid_t *)sqlite3_column_blob(stmt_query, 30)), transition_id);

         char *edit_command = sqlite3_column_bytes(stmt_query, 16) > 0 ?
                                  health_edit_command_from_source((char *)sqlite3_column_text(stmt_query, 16)) :
                                  strdupz("UNKNOWN=0=UNKNOWN");

        buffer_json_add_array_item_object(wb); // this node

        buffer_json_member_add_string_or_empty(wb, "hostname",             rrdhost_hostname(host));
                  buffer_json_member_add_int64(wb, "utc_offset",           (int64_t)host->utc_offset);
        buffer_json_member_add_string_or_empty(wb, "timezone",             rrdhost_abbrev_timezone(host));
                  buffer_json_member_add_int64(wb, "unique_id",            (int64_t) sqlite3_column_int64(stmt_query, 0));
                  buffer_json_member_add_int64(wb, "alarm_id",             (int64_t) sqlite3_column_int64(stmt_query, 1));
                  buffer_json_member_add_int64(wb, "alarm_event_id",       (int64_t) sqlite3_column_int64(stmt_query, 2));
        buffer_json_member_add_string_or_empty(wb, "config_hash_id",       config_hash_id);
        buffer_json_member_add_string_or_empty(wb, "transition_id",        transition_id);
        buffer_json_member_add_string_or_empty(wb, "name",                 (const char *) sqlite3_column_text(stmt_query, 12));
        buffer_json_member_add_string_or_empty(wb, "chart",                (const char *) sqlite3_column_text(stmt_query, 13));
        buffer_json_member_add_string_or_empty(wb, "context",              (const char *) sqlite3_column_text(stmt_query, 29));
        buffer_json_member_add_string_or_empty(wb, "class",                sqlite3_column_text(stmt_query, 26) ? (const char *) sqlite3_column_text(stmt_query, 26) : (char *) "Unknown");
        buffer_json_member_add_string_or_empty(wb, "component",            sqlite3_column_text(stmt_query, 27) ? (const char *) sqlite3_column_text(stmt_query, 27) : (char *) "Unknown");
        buffer_json_member_add_string_or_empty(wb, "type",                 sqlite3_column_text(stmt_query, 28) ? (const char *) sqlite3_column_text(stmt_query, 28) : (char *) "Unknown");
                buffer_json_member_add_boolean(wb, "processed",            (sqlite3_column_int64(stmt_query, 9) & HEALTH_ENTRY_FLAG_PROCESSED));
                buffer_json_member_add_boolean(wb, "updated",              (sqlite3_column_int64(stmt_query, 9) & HEALTH_ENTRY_FLAG_UPDATED));
                  buffer_json_member_add_int64(wb, "exec_run",             (int64_t)sqlite3_column_int64(stmt_query, 10));
                buffer_json_member_add_boolean(wb, "exec_failed",          (sqlite3_column_int64(stmt_query, 9) & HEALTH_ENTRY_FLAG_EXEC_FAILED));
        buffer_json_member_add_string_or_empty(wb, "exec",                 sqlite3_column_text(stmt_query, 14) ? (const char *) sqlite3_column_text(stmt_query, 14) : string2str(host->health.health_default_exec));
        buffer_json_member_add_string_or_empty(wb, "recipient",            sqlite3_column_text(stmt_query, 15) ? (const char *) sqlite3_column_text(stmt_query, 15) : string2str(host->health.health_default_recipient));
                  buffer_json_member_add_int64(wb, "exec_code",            sqlite3_column_int(stmt_query, 19));
        buffer_json_member_add_string_or_empty(wb, "source",               sqlite3_column_text(stmt_query, 16) ? (const char *) sqlite3_column_text(stmt_query, 16) : (char *) "Unknown");
        buffer_json_member_add_string_or_empty(wb, "command",              edit_command);
        buffer_json_member_add_string_or_empty(wb, "units",                (const char *) sqlite3_column_text(stmt_query, 17));
                  buffer_json_member_add_int64(wb, "when",                 (int64_t)sqlite3_column_int64(stmt_query, 6));
                  buffer_json_member_add_int64(wb, "duration",             (int64_t)sqlite3_column_int64(stmt_query, 7));
                  buffer_json_member_add_int64(wb, "non_clear_duration",   (int64_t)sqlite3_column_int64(stmt_query, 8));
        buffer_json_member_add_string_or_empty(wb, "status",               rrdcalc_status2string(sqlite3_column_int(stmt_query, 20)));
        buffer_json_member_add_string_or_empty(wb, "old_status",           rrdcalc_status2string(sqlite3_column_int(stmt_query, 21)));
                  buffer_json_member_add_int64(wb, "delay",                sqlite3_column_int(stmt_query, 22));
                  buffer_json_member_add_int64(wb, "delay_up_to_timestamp",(int64_t)sqlite3_column_int64(stmt_query, 11));
                  buffer_json_member_add_int64(wb, "updated_by_id",        (unsigned int)sqlite3_column_int64(stmt_query, 4));
                  buffer_json_member_add_int64(wb, "updates_id",           (unsigned int)sqlite3_column_int64(stmt_query, 5));
        buffer_json_member_add_string_or_empty(wb, "value_string",         sqlite3_column_type(stmt_query, 23) == SQLITE_NULL ? "-" :
                                                                                      format_value_and_unit(new_value_string, 100, sqlite3_column_double(stmt_query, 23), (char *) sqlite3_column_text(stmt_query, 17), -1));
        buffer_json_member_add_string_or_empty(wb, "old_value_string",     sqlite3_column_type(stmt_query, 24) == SQLITE_NULL ? "-" :
                                                                                      format_value_and_unit(old_value_string, 100, sqlite3_column_double(stmt_query, 24), (char *) sqlite3_column_text(stmt_query, 17), -1));
                  buffer_json_member_add_int64(wb, "last_repeat",          (int64_t)sqlite3_column_int64(stmt_query, 25));
                buffer_json_member_add_boolean(wb, "silenced",             (sqlite3_column_int64(stmt_query, 9) & HEALTH_ENTRY_FLAG_SILENCED));
        buffer_json_member_add_string_or_empty(wb, "summary",              (const char *) sqlite3_column_text(stmt_query, 31));
        buffer_json_member_add_string_or_empty(wb, "info",                 (const char *) sqlite3_column_text(stmt_query, 18));
                buffer_json_member_add_boolean(wb, "no_clear_notification",(sqlite3_column_int64(stmt_query, 9) & HEALTH_ENTRY_FLAG_NO_CLEAR_NOTIFICATION));

        if (sqlite3_column_type(stmt_query, 23) == SQLITE_NULL)
            buffer_json_member_add_string(wb, "value", NULL);
        else
            buffer_json_member_add_double(wb, "value", sqlite3_column_double(stmt_query, 23));

        if (sqlite3_column_type(stmt_query, 24) == SQLITE_NULL)
            buffer_json_member_add_string(wb, "old_value", NULL);
        else
            buffer_json_member_add_double(wb, "old_value", sqlite3_column_double(stmt_query, 23));

        freez(edit_command);

        buffer_json_object_close(wb);
    }

    buffer_json_array_close(wb);
    buffer_json_finalize(wb);

finish:
     rc = sqlite3_reset(stmt_query);
     if (unlikely(rc != SQLITE_OK))
         error_report("Failed to reset statement for SQL_SELECT_HEALTH_LOG");
}

#define SQL_COPY_HEALTH_LOG(table) "INSERT OR IGNORE INTO health_log (host_id, alarm_id, config_hash_id, name, chart, family, exec, recipient, units, chart_context) SELECT ?1, alarm_id, config_hash_id, name, chart, family, exec, recipient, units, chart_context from %s", table
#define SQL_COPY_HEALTH_LOG_DETAIL(table) "INSERT INTO health_log_detail (unique_id, alarm_id, alarm_event_id, updated_by_id, updates_id, when_key, duration, non_clear_duration, flags, exec_run_timestamp, delay_up_to_timestamp, info, exec_code, new_status, old_status, delay, new_value, old_value, last_repeat, transition_id, global_id, host_id) SELECT unique_id, alarm_id, alarm_event_id, updated_by_id, updates_id, when_key, duration, non_clear_duration, flags, exec_run_timestamp, delay_up_to_timestamp, info, exec_code, new_status, old_status, delay, new_value, old_value, last_repeat, transition_id, now_usec(1), ?1 from %s", table
#define SQL_UPDATE_HEALTH_LOG_DETAIL_TRANSITION_ID "update health_log_detail set transition_id = uuid_random() where transition_id is null"
#define SQL_UPDATE_HEALTH_LOG_DETAIL_HEALTH_LOG_ID "update health_log_detail set health_log_id = (select health_log_id from health_log where host_id = ?1 and alarm_id = health_log_detail.alarm_id) where health_log_id is null and host_id = ?2"
#define SQL_UPDATE_HEALTH_LOG_LAST_TRANSITION_ID "update health_log set last_transition_id = (select transition_id from health_log_detail where health_log_id = health_log.health_log_id and alarm_id = health_log.alarm_id group by (alarm_id) having max(alarm_event_id)) where host_id = ?1"
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
    snprintfz(command, sizeof(command) - 1, SQL_COPY_HEALTH_LOG(table));
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
    snprintfz(command, sizeof(command) - 1, SQL_COPY_HEALTH_LOG_DETAIL(table));
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

#define SQL_GET_EVENT_ID                                                                                               \
    "SELECT MAX(alarm_event_id)+1 FROM health_log_detail WHERE health_log_id = @health_log_id AND alarm_id = @alarm_id"

static uint32_t get_next_alarm_event_id(uint64_t health_log_id, uint32_t alarm_id)
{
    int rc;
    sqlite3_stmt *res = NULL;
    uint32_t next_event_id = 0;

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
        next_event_id = (uint32_t) sqlite3_column_int64(res, 0);
    }

    rc = sqlite3_finalize(res);
    if (unlikely(rc != SQLITE_OK))
        error_report("Failed to finalize the statement while getting an alarm id.");

    return next_event_id;
}

#define SQL_GET_ALARM_ID                                                                                               \
    "SELECT alarm_id, health_log_id FROM health_log WHERE host_id = @host_id AND chart = @chart AND name = @name"

uint32_t sql_get_alarm_id(RRDHOST *host, STRING *chart, STRING *name, uint32_t *next_event_id)
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

    rc = SQLITE3_BIND_STRING_OR_NULL(res, chart, 2);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to bind char parameter for SQL_GET_ALARM_ID.");
        sqlite3_finalize(res);
        return alarm_id;
    }

    rc = SQLITE3_BIND_STRING_OR_NULL(res, name, 3);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to bind name parameter for SQL_GET_ALARM_ID.");
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

     if (alarm_id)
           *next_event_id = get_next_alarm_event_id(health_log_id, alarm_id);

     return alarm_id;
}

#define SQL_GET_ALARM_ID_FROM_TRANSITION_ID                                                                            \
     "SELECT hld.alarm_id, hl.host_id, hl.chart_context FROM health_log_detail hld, health_log hl "                    \
     "WHERE hld.transition_id = @transition_id "                                                                       \
     "AND hld.health_log_id = hl.health_log_id"

bool sql_find_alert_transition(
    const char *transition,
    void (*cb)(const char *machine_guid, const char *context, time_t alert_id, void *data),
    void *data)
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
        goto done;
    }

    while (sqlite3_step_monitored(res) == SQLITE_ROW) {
        ok = true;
        uuid_unparse_lower(*(uuid_t *) sqlite3_column_blob(res, 1), machine_guid);
        cb(machine_guid, (const char *) sqlite3_column_text(res, 2), sqlite3_column_int(res, 0), data);
    }

done:
    rc = sqlite3_reset(res);
    if (unlikely(rc != SQLITE_OK))
        error_report("Failed to reset the statement when trying to find transition");

    return ok;
}

#define SQL_BUILD_ALERT_TRANSITION "CREATE TEMP TABLE IF NOT EXISTS v_%p (host_id blob)"

#define SQL_POPULATE_TEMP_ALERT_TRANSITION_TABLE "INSERT INTO v_%p (host_id) VALUES (@host_id)"

#define SQL_SEARCH_ALERT_TRANSITION_SELECT                                                                                    \
    "SELECT h.host_id, h.alarm_id, h.config_hash_id, h.name, h.chart, h.chart_name, h.family, h.recipient, h.units, h.exec, " \
    "h.chart_context,  d.when_key, d.duration, d.non_clear_duration, d.flags, d.delay_up_to_timestamp, "                      \
    "d.info, d.exec_code, d.new_status, d.old_status, d.delay, d.new_value, d.old_value, d.last_repeat, "                     \
    "d.transition_id, d.global_id, ah.class, ah.type, ah.component, d.exec_run_timestamp, d.summary"

#define SQL_SEARCH_ALERT_TRANSITION_COMMON_WHERE "h.config_hash_id = ah.hash_id AND h.health_log_id = d.health_log_id"

#define SQL_SEARCH_ALERT_TRANSITION                                                                                    \
    SQL_SEARCH_ALERT_TRANSITION_SELECT                                                                                 \
        " FROM health_log h, health_log_detail d, v_%p t, alert_hash ah "                                              \
        " WHERE h.host_id = t.host_id AND " SQL_SEARCH_ALERT_TRANSITION_COMMON_WHERE                                   \
        " AND ( d.new_status > 2 OR d.old_status > 2 ) AND d.global_id BETWEEN @after AND @before "

#define SQL_SEARCH_ALERT_TRANSITION_DIRECT                                                                             \
    SQL_SEARCH_ALERT_TRANSITION_SELECT " FROM health_log h, health_log_detail d, alert_hash ah "                       \
                                       " WHERE " SQL_SEARCH_ALERT_TRANSITION_COMMON_WHERE                              \
                                       " AND transition_id = @transition "

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
            goto done;
        }
        goto run_query;
    }

    snprintfz(sql, sizeof(sql) - 1, SQL_BUILD_ALERT_TRANSITION, nodes);
    rc = db_execute(db_meta, sql);
    if (rc)
        return;

    snprintfz(sql, sizeof(sql) - 1, SQL_POPULATE_TEMP_ALERT_TRANSITION_TABLE, nodes);

    // Prepare statement to add things
    rc = sqlite3_prepare_v2(db_meta, sql, -1, &res, 0);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to prepare statement to INSERT into v_%p", nodes);
        goto done_only_drop;
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
        goto done_only_drop;
    }

    int param = 1;
    rc = sqlite3_bind_int64(res, param++, (sqlite3_int64)(after * USEC_PER_SEC));
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to bind after parameter");
        goto done;
    }

    rc = sqlite3_bind_int64(res, param++, (sqlite3_int64)(before * USEC_PER_SEC));
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to bind before parameter");
        goto done;
    }

    if (context) {
        rc = sqlite3_bind_text(res, param++, context, -1, SQLITE_STATIC);
        if (unlikely(rc != SQLITE_OK)) {
            error_report("Failed to bind context parameter");
            goto done;
        }
    }

    if (alert_name) {
        rc = sqlite3_bind_text(res, param++, alert_name, -1, SQLITE_STATIC);
        if (unlikely(rc != SQLITE_OK)) {
            error_report("Failed to bind alert_name parameter");
            goto done;
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
        atd.summary = (const char *) sqlite3_column_text(res, 30);

        cb(&atd, data);
    }

done:
    rc = sqlite3_finalize(res);
    if (unlikely(rc != SQLITE_OK))
        error_report("Failed to finalize statement for sql_alert_transitions");

done_only_drop:
    if (likely(!transition)) {
        (void)snprintfz(sql, sizeof(sql) - 1, "DROP TABLE IF EXISTS v_%p", nodes);
        (void)db_execute(db_meta, sql);
        buffer_free(command);
    }
}

#define SQL_BUILD_CONFIG_TARGET_LIST "CREATE TEMP TABLE IF NOT EXISTS c_%p (hash_id blob)"

#define SQL_POPULATE_TEMP_CONFIG_TARGET_TABLE "INSERT INTO c_%p (hash_id) VALUES (@hash_id)"

#define SQL_SEARCH_CONFIG_LIST                                                                                         \
    "SELECT ah.hash_id, alarm, template, on_key, class, component, type, os, hosts, lookup, every, "                   \
    " units, calc, families, plugin, module, charts, green, red, warn, crit, "                                         \
    " exec, to_key, info, delay, options, repeat, host_labels, p_db_lookup_dimensions, p_db_lookup_method, "           \
    " p_db_lookup_options, p_db_lookup_after, p_db_lookup_before, p_update_every, source, chart_labels, summary "      \
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

    snprintfz(sql, sizeof(sql) - 1, SQL_BUILD_CONFIG_TARGET_LIST, configs);
    rc = db_execute(db_meta, sql);
    if (rc)
        return added;

    snprintfz(sql, sizeof(sql) - 1, SQL_POPULATE_TEMP_CONFIG_TARGET_TABLE, configs);

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
        acd.summary = (const char *) sqlite3_column_text(res, param++);

        cb(&acd, data);
        added++;
    }

    rc = sqlite3_finalize(res);
    if (unlikely(rc != SQLITE_OK))
        error_report("Failed to finalize statement for sql_get_alert_configuration");

fail_only_drop:
    (void)snprintfz(sql, sizeof(sql) - 1, "DROP TABLE IF EXISTS c_%p", configs);
    (void)db_execute(db_meta, sql);
    buffer_free(command);
    return added;
}
