// SPDX-License-Identifier: GPL-3.0-or-later

#include "sqlite_health.h"
#include "sqlite_functions.h"

#define MAX_HEALTH_SQL_SIZE 2048

/* Health related SQL queries
   Creates a health log table in sqlite, one per host guid
*/
#define SQL_CREATE_HEALTH_LOG_TABLE(guid) "CREATE TABLE IF NOT EXISTS health_log_%s(hostname text, unique_id int, alarm_id int, alarm_event_id int, config_hash_id blob, updated_by_id int, updates_id int, when_key int, duration int, non_clear_duration int, flags int, exec_run_timestamp int, delay_up_to_timestamp int, name text, chart text, family text, exec text, recipient text, source text, units text, info text, exec_code int, new_status real, old_status real, delay int, new_value double, old_value double, last_repeat int, class text, component text, type text, chart_context text);", guid
int sql_create_health_log_table(RRDHOST *host) {
    int rc;
    char *err_msg = NULL, command[MAX_HEALTH_SQL_SIZE + 1];

    if (unlikely(!db_meta)) {
        if (default_rrd_memory_mode == RRD_MEMORY_MODE_DBENGINE)
            error_report("HEALTH [%s]: Database has not been initialized", rrdhost_hostname(host));
        return 1;
    }

    char uuid_str[GUID_LEN + 1];
    uuid_unparse_lower_fix(&host->host_uuid, uuid_str);

    snprintfz(command, MAX_HEALTH_SQL_SIZE, SQL_CREATE_HEALTH_LOG_TABLE(uuid_str));

    rc = sqlite3_exec_monitored(db_meta, command, 0, 0, &err_msg);
    if (rc != SQLITE_OK) {
        error_report("HEALTH [%s]: SQLite error during creation of health log table, rc = %d (%s)", rrdhost_hostname(host), rc, err_msg);
        sqlite3_free(err_msg);
        return 1;
    }

    snprintfz(command, MAX_HEALTH_SQL_SIZE, "CREATE INDEX IF NOT EXISTS "
            "health_log_index_%s ON health_log_%s (unique_id); ", uuid_str, uuid_str);
    db_execute(command);

    return 0;
}

/* Health related SQL queries
   Updates an entry in the table
*/
#define SQL_UPDATE_HEALTH_LOG(guid) "UPDATE health_log_%s set updated_by_id = ?, flags = ?, exec_run_timestamp = ?, exec_code = ? where unique_id = ?;", guid
void sql_health_alarm_log_update(RRDHOST *host, ALARM_ENTRY *ae) {
    sqlite3_stmt *res = NULL;
    int rc;
    char command[MAX_HEALTH_SQL_SIZE + 1];

    if (unlikely(!db_meta)) {
        if (default_rrd_memory_mode == RRD_MEMORY_MODE_DBENGINE)
            error_report("HEALTH [%s]: Database has not been initialized", rrdhost_hostname(host));
        return;
    }

    char uuid_str[GUID_LEN + 1];
    uuid_unparse_lower_fix(&host->host_uuid, uuid_str);

    snprintfz(command, MAX_HEALTH_SQL_SIZE, SQL_UPDATE_HEALTH_LOG(uuid_str));

    rc = sqlite3_prepare_v2(db_meta, command, -1, &res, 0);
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

    rc = execute_insert(res);
    if (unlikely(rc != SQLITE_DONE)) {
        error_report("HEALTH [%s]: Failed to update health log, rc = %d", rrdhost_hostname(host), rc);
    }

    failed:
    if (unlikely(sqlite3_finalize(res) != SQLITE_OK))
        error_report("HEALTH [%s]: Failed to finalize the prepared statement for updating health log.", rrdhost_hostname(host));

    return;
}

/* Health related SQL queries
   Inserts an entry in the table
*/
#define SQL_INSERT_HEALTH_LOG(guid) "INSERT INTO health_log_%s(hostname, unique_id, alarm_id, alarm_event_id, " \
    "config_hash_id, updated_by_id, updates_id, when_key, duration, non_clear_duration, flags, " \
    "exec_run_timestamp, delay_up_to_timestamp, name, chart, family, exec, recipient, source, " \
    "units, info, exec_code, new_status, old_status, delay, new_value, old_value, last_repeat, " \
    "class, component, type, chart_context) values (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?);", guid

void sql_health_alarm_log_insert(RRDHOST *host, ALARM_ENTRY *ae) {
    sqlite3_stmt *res = NULL;
    int rc;
    char command[MAX_HEALTH_SQL_SIZE + 1];

    if (unlikely(!db_meta)) {
        if (default_rrd_memory_mode == RRD_MEMORY_MODE_DBENGINE)
            error_report("HEALTH [%s]: Database has not been initialized", rrdhost_hostname(host));
        return;
    }

    char uuid_str[GUID_LEN + 1];
    uuid_unparse_lower_fix(&host->host_uuid, uuid_str);

    snprintfz(command, MAX_HEALTH_SQL_SIZE, SQL_INSERT_HEALTH_LOG(uuid_str));

    rc = sqlite3_prepare_v2(db_meta, command, -1, &res, 0);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("HEALTH [%s]: Failed to prepare statement for SQL_INSERT_HEALTH_LOG", rrdhost_hostname(host));
        return;
    }

    rc = sqlite3_bind_text(res, 1, rrdhost_hostname(host), -1, SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to bind hostname parameter for SQL_INSERT_HEALTH_LOG");
        goto failed;
    }

    rc = sqlite3_bind_int64(res, 2, (sqlite3_int64) ae->unique_id);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to bind unique_id parameter for SQL_INSERT_HEALTH_LOG");
        goto failed;
    }

    rc = sqlite3_bind_int64(res, 3, (sqlite3_int64) ae->alarm_id);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to bind alarm_id parameter for SQL_INSERT_HEALTH_LOG");
        goto failed;
    }

    rc = sqlite3_bind_int64(res, 4, (sqlite3_int64) ae->alarm_event_id);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to bind alarm_event_id parameter for SQL_INSERT_HEALTH_LOG");
        goto failed;
    }

    rc = sqlite3_bind_blob(res, 5, &ae->config_hash_id, sizeof(ae->config_hash_id), SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to bind config_hash_id parameter for SQL_INSERT_HEALTH_LOG");
        goto failed;
    }

    rc = sqlite3_bind_int64(res, 6, (sqlite3_int64) ae->updated_by_id);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to bind updated_by_id parameter for SQL_INSERT_HEALTH_LOG");
        goto failed;
    }

    rc = sqlite3_bind_int64(res, 7, (sqlite3_int64) ae->updates_id);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to bind updates_id parameter for SQL_INSERT_HEALTH_LOG");
        goto failed;
    }

    rc = sqlite3_bind_int64(res, 8, (sqlite3_int64) ae->when);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to bind when parameter for SQL_INSERT_HEALTH_LOG");
        goto failed;
    }

    rc = sqlite3_bind_int64(res, 9, (sqlite3_int64) ae->duration);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to bind duration parameter for SQL_INSERT_HEALTH_LOG");
        goto failed;
    }

    rc = sqlite3_bind_int64(res, 10, (sqlite3_int64) ae->non_clear_duration);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to bind non_clear_duration parameter for SQL_INSERT_HEALTH_LOG");
        goto failed;
    }

    rc = sqlite3_bind_int64(res, 11, (sqlite3_int64) ae->flags);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to bind flags parameter for SQL_INSERT_HEALTH_LOG");
        goto failed;
    }

    rc = sqlite3_bind_int64(res, 12, (sqlite3_int64) ae->exec_run_timestamp);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to bind exec_run_timestamp parameter for SQL_INSERT_HEALTH_LOG");
        goto failed;
    }

    rc = sqlite3_bind_int64(res, 13, (sqlite3_int64) ae->delay_up_to_timestamp);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to bind delay_up_to_timestamp parameter for SQL_INSERT_HEALTH_LOG");
        goto failed;
    }

    rc = sqlite3_bind_text(res, 14, ae_name(ae), -1, SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to bind name parameter for SQL_INSERT_HEALTH_LOG");
        goto failed;
    }

    rc = sqlite3_bind_text(res, 15, ae_chart_name(ae), -1, SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to bind chart parameter for SQL_INSERT_HEALTH_LOG");
        goto failed;
    }

    rc = sqlite3_bind_text(res, 16, ae_family(ae), -1, SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to bind family parameter for SQL_INSERT_HEALTH_LOG");
        goto failed;
    }

    rc = sqlite3_bind_text(res, 17, ae_exec(ae), -1, SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to bind exec parameter for SQL_INSERT_HEALTH_LOG");
        goto failed;
    }

    rc = sqlite3_bind_text(res, 18, ae_recipient(ae), -1, SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to bind recipient parameter for SQL_INSERT_HEALTH_LOG");
        goto failed;
    }

    rc = sqlite3_bind_text(res, 19, ae_source(ae), -1, SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to bind source parameter for SQL_INSERT_HEALTH_LOG");
        goto failed;
    }

    rc = sqlite3_bind_text(res, 20, ae_units(ae), -1, SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to bind host_id parameter to store node instance information");
        goto failed;
    }

    rc = sqlite3_bind_text(res, 21, ae_info(ae), -1, SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to bind info parameter for SQL_INSERT_HEALTH_LOG");
        goto failed;
    }

    rc = sqlite3_bind_int(res, 22, ae->exec_code);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to bind exec_code parameter for SQL_INSERT_HEALTH_LOG");
        goto failed;
    }

    rc = sqlite3_bind_int(res, 23, ae->new_status);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to bind new_status parameter for SQL_INSERT_HEALTH_LOG");
        goto failed;
    }

    rc = sqlite3_bind_int(res, 24, ae->old_status);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to bind old_status parameter for SQL_INSERT_HEALTH_LOG");
        goto failed;
    }

    rc = sqlite3_bind_int(res, 25, ae->delay);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to bind delay parameter for SQL_INSERT_HEALTH_LOG");
        goto failed;
    }

    rc = sqlite3_bind_double(res, 26, ae->new_value);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to bind new_value parameter for SQL_INSERT_HEALTH_LOG");
        goto failed;
    }

    rc = sqlite3_bind_double(res, 27, ae->old_value);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to bind old_value parameter for SQL_INSERT_HEALTH_LOG");
        goto failed;
    }

    rc = sqlite3_bind_int64(res, 28, (sqlite3_int64) ae->last_repeat);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to bind last_repeat parameter for SQL_INSERT_HEALTH_LOG");
        goto failed;
    }

    rc = sqlite3_bind_text(res, 29, ae_classification(ae), -1, SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to bind classification parameter for SQL_INSERT_HEALTH_LOG");
        goto failed;
    }

    rc = sqlite3_bind_text(res, 30, ae_component(ae), -1, SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to bind component parameter for SQL_INSERT_HEALTH_LOG");
        goto failed;
    }

    rc = sqlite3_bind_text(res, 31, ae_type(ae), -1, SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to bind type parameter for SQL_INSERT_HEALTH_LOG");
        goto failed;
    }

    rc = sqlite3_bind_text(res, 32, ae_chart_context(ae), -1, SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to bind chart_context parameter for SQL_INSERT_HEALTH_LOG");
        goto failed;
    }

    rc = execute_insert(res);
    if (unlikely(rc != SQLITE_DONE)) {
        error_report("HEALTH [%s]: Failed to execute SQL_INSERT_HEALTH_LOG, rc = %d", rrdhost_hostname(host), rc);
        goto failed;
    }

    ae->flags |= HEALTH_ENTRY_FLAG_SAVED;
    host->health_log_entries_written++;

    failed:
    if (unlikely(sqlite3_finalize(res) != SQLITE_OK))
        error_report("HEALTH [%s]: Failed to finalize the prepared statement for inserting to health log.", rrdhost_hostname(host));

    return;
}

void sql_health_alarm_log_save(RRDHOST *host, ALARM_ENTRY *ae)
{
    if (ae->flags & HEALTH_ENTRY_FLAG_SAVED)
        sql_health_alarm_log_update(host, ae);
    else
        sql_health_alarm_log_insert(host, ae);
}

/* Health related SQL queries
   Cleans up the health_log table.
*/
#define SQL_CLEANUP_HEALTH_LOG(guid,guid2,limit) "DELETE from health_log_%s where unique_id in (SELECT unique_id from health_log_%s order by unique_id asc LIMIT %lu);", guid, guid2, limit
void sql_health_alarm_log_cleanup(RRDHOST *host) {
    sqlite3_stmt *res = NULL;
    static size_t rotate_every = 0;
    int rc;
    char command[MAX_HEALTH_SQL_SIZE + 1];

    if(unlikely(rotate_every == 0)) {
        rotate_every = (size_t)config_get_number(CONFIG_SECTION_HEALTH, "rotate log every lines", 2000);
        if(rotate_every < 100) rotate_every = 100;
    }

    if(likely(host->health_log_entries_written < rotate_every)) {
        return;
    }

    if (unlikely(!db_meta)) {
        if (default_rrd_memory_mode == RRD_MEMORY_MODE_DBENGINE)
            error_report("Database has not been initialized");
        return;
    }

    char uuid_str[GUID_LEN + 1];
    uuid_unparse_lower_fix(&host->host_uuid, uuid_str);

    snprintfz(command, MAX_HEALTH_SQL_SIZE, SQL_CLEANUP_HEALTH_LOG(uuid_str, uuid_str, host->health_log_entries_written - rotate_every));

    rc = sqlite3_prepare_v2(db_meta, command, -1, &res, 0);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to prepare statement to cleanup health log table");
        return;
    }

    rc = sqlite3_step_monitored(res);
    if (unlikely(rc != SQLITE_DONE))
        error_report("Failed to cleanup health log table, rc = %d", rc);

    rc = sqlite3_finalize(res);
    if (unlikely(rc != SQLITE_OK))
        error_report("Failed to finalize the prepared statement to cleanup health log table");

    host->health_log_entries_written = rotate_every;

    sql_aclk_alert_clean_dead_entries(host);
}

/* Health related SQL queries
   Get a count of rows from health log table
*/
#define SQL_COUNT_HEALTH_LOG(guid) "SELECT count(1) FROM health_log_%s;", guid
void sql_health_alarm_log_count(RRDHOST *host) {
    sqlite3_stmt *res = NULL;
    int rc;
    char command[MAX_HEALTH_SQL_SIZE + 1];

    if (unlikely(!db_meta)) {
        if (default_rrd_memory_mode == RRD_MEMORY_MODE_DBENGINE)
            error_report("Database has not been initialized");
        return;
    }

    char uuid_str[GUID_LEN + 1];
    uuid_unparse_lower_fix(&host->host_uuid, uuid_str);

    snprintfz(command, MAX_HEALTH_SQL_SIZE, SQL_COUNT_HEALTH_LOG(uuid_str));

    rc = sqlite3_prepare_v2(db_meta, command, -1, &res, 0);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to prepare statement to count health log entries from db");
        return;
    }

    rc = sqlite3_step_monitored(res);
    if (likely(rc == SQLITE_ROW))
        host->health_log_entries_written = (size_t) sqlite3_column_int64(res, 0);

    rc = sqlite3_finalize(res);
    if (unlikely(rc != SQLITE_OK))
        error_report("Failed to finalize the prepared statement to count health log entries from db");

    info("HEALTH [%s]: Table health_log_%s, contains %lu entries.", rrdhost_hostname(host), uuid_str, host->health_log_entries_written);
}

#define SQL_INJECT_REMOVED(guid, guid2) "insert into health_log_%s (hostname, unique_id, alarm_id, alarm_event_id, config_hash_id, updated_by_id, updates_id, when_key, duration, non_clear_duration, flags, exec_run_timestamp, " \
"delay_up_to_timestamp, name, chart, family, exec, recipient, source, units, info, exec_code, new_status, old_status, delay, new_value, old_value, last_repeat, class, component, type, chart_context) " \
"select hostname, ?1, ?2, ?3, config_hash_id, 0, ?4, unixepoch(), 0, 0, flags, exec_run_timestamp, " \
"unixepoch(), name, chart, family, exec, recipient, source, units, info, exec_code, -2, new_status, delay, NULL, new_value, 0, class, component, type, chart_context " \
"from health_log_%s where unique_id = ?5", guid, guid2
#define SQL_INJECT_REMOVED_UPDATE(guid) "update health_log_%s set flags = flags | ?1, updated_by_id = ?2 where unique_id = ?3; ", guid
void sql_inject_removed_status(char *uuid_str, uint32_t alarm_id, uint32_t alarm_event_id, uint32_t unique_id, uint32_t max_unique_id)
{
    int rc = 0;
    char command[MAX_HEALTH_SQL_SIZE + 1];

    if (!alarm_id || !alarm_event_id || !unique_id || !max_unique_id)
        return;

    sqlite3_stmt *res = NULL;

    snprintfz(command, MAX_HEALTH_SQL_SIZE, SQL_INJECT_REMOVED(uuid_str, uuid_str));
    rc = sqlite3_prepare_v2(db_meta, command, -1, &res, 0);
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

    rc = sqlite3_bind_int64(res, 5, (sqlite3_int64) unique_id);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to bind unique_id parameter for SQL_INJECT_REMOVED");
        goto failed;
    }

    rc = execute_insert(res);
    if (unlikely(rc != SQLITE_DONE)) {
        error_report("HEALTH [N/A]: Failed to execute SQL_INJECT_REMOVED, rc = %d", rc);
        goto failed;
    }

    if (unlikely(sqlite3_finalize(res) != SQLITE_OK))
        error_report("HEALTH [N/A]: Failed to finalize the prepared statement for injecting removed event.");

    //update the old entry
    snprintfz(command, MAX_HEALTH_SQL_SIZE, SQL_INJECT_REMOVED_UPDATE(uuid_str));
    rc = sqlite3_prepare_v2(db_meta, command, -1, &res, 0);
    if (rc != SQLITE_OK) {
        error_report("Failed to prepare statement when trying to update during inject removed event");
        return;
    }

    rc = sqlite3_bind_int64(res, 1, (sqlite3_int64) HEALTH_ENTRY_FLAG_UPDATED);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to bind flags parameter for SQL_INJECT_REMOVED (update)");
        goto failed;
    }

    rc = sqlite3_bind_int64(res, 2, (sqlite3_int64) max_unique_id);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to bind max_unique_id parameter for SQL_INJECT_REMOVED (update)");
        goto failed;
    }

    rc = sqlite3_bind_int64(res, 3, (sqlite3_int64) unique_id);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to bind unique_id parameter for SQL_INJECT_REMOVED (update)");
        goto failed;
    }

    rc = execute_insert(res);
    if (unlikely(rc != SQLITE_DONE)) {
        error_report("HEALTH [N/A]: Failed to execute SQL_INJECT_REMOVED_UPDATE, rc = %d", rc);
        goto failed;
    }

failed:
    if (unlikely(sqlite3_finalize(res) != SQLITE_OK))
        error_report("HEALTH [N/A]: Failed to finalize the prepared statement for injecting removed event.");
    return;

}

#define SQL_SELECT_MAX_UNIQUE_ID(guid) "SELECT MAX(unique_id) from health_log_%s", guid
uint32_t sql_get_max_unique_id (char *uuid_str)
{
    int rc = 0;
    char command[MAX_HEALTH_SQL_SIZE + 1];
    uint32_t max_unique_id = 0;

    sqlite3_stmt *res = NULL;

    snprintfz(command, MAX_HEALTH_SQL_SIZE, SQL_SELECT_MAX_UNIQUE_ID(uuid_str));
    rc = sqlite3_prepare_v2(db_meta, command, -1, &res, 0);
    if (rc != SQLITE_OK) {
        error_report("Failed to prepare statement when trying to get max unique id");
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

#define SQL_SELECT_LAST_STATUSES(guid) "SELECT new_status, unique_id, alarm_id, alarm_event_id from health_log_%s group by alarm_id having max(alarm_event_id)", guid
void sql_check_removed_alerts_state(char *uuid_str)
{
    int rc = 0;
    char command[MAX_HEALTH_SQL_SIZE + 1];
    RRDCALC_STATUS status;
    uint32_t alarm_id = 0, alarm_event_id = 0, unique_id = 0, max_unique_id = 0;

    sqlite3_stmt *res = NULL;

    snprintfz(command, MAX_HEALTH_SQL_SIZE, SQL_SELECT_LAST_STATUSES(uuid_str));
    rc = sqlite3_prepare_v2(db_meta, command, -1, &res, 0);
    if (rc != SQLITE_OK) {
        error_report("Failed to prepare statement when trying to check removed statuses");
        return;
    }

     while (sqlite3_step_monitored(res) == SQLITE_ROW) {
         status  = (RRDCALC_STATUS) sqlite3_column_int(res, 0);
         unique_id = (uint32_t) sqlite3_column_int64(res, 1);
         alarm_id = (uint32_t) sqlite3_column_int64(res, 2);
         alarm_event_id = (uint32_t) sqlite3_column_int64(res, 3);
         if (unlikely(status != RRDCALC_STATUS_REMOVED)) {
             if (unlikely(!max_unique_id))
                 max_unique_id = sql_get_max_unique_id (uuid_str);
             sql_inject_removed_status (uuid_str, alarm_id, alarm_event_id, unique_id, ++max_unique_id);
         }
     }

     rc = sqlite3_finalize(res);
     if (unlikely(rc != SQLITE_OK))
         error_report("Failed to finalize the statement");
}

/* Health related SQL queries
   Load from the health log table
*/
#define SQL_LOAD_HEALTH_LOG(guid,limit) "SELECT hostname, unique_id, alarm_id, alarm_event_id, config_hash_id, updated_by_id, updates_id, when_key, duration, non_clear_duration, flags, exec_run_timestamp, delay_up_to_timestamp, name, chart, family, exec, recipient, source, units, info, exec_code, new_status, old_status, delay, new_value, old_value, last_repeat, class, component, type, chart_context FROM (SELECT hostname, unique_id, alarm_id, alarm_event_id, config_hash_id, updated_by_id, updates_id, when_key, duration, non_clear_duration, flags, exec_run_timestamp, delay_up_to_timestamp, name, chart, family, exec, recipient, source, units, info, exec_code, new_status, old_status, delay, new_value, old_value, last_repeat, class, component, type, chart_context FROM health_log_%s order by unique_id desc limit %u) order by unique_id asc;", guid, limit
void sql_health_alarm_log_load(RRDHOST *host) {
    sqlite3_stmt *res = NULL;
    int ret;
    ssize_t errored = 0, loaded = 0;
    char command[MAX_HEALTH_SQL_SIZE + 1];

    host->health_log_entries_written = 0;

    if (unlikely(!db_meta)) {
        if (default_rrd_memory_mode == RRD_MEMORY_MODE_DBENGINE)
            error_report("HEALTH [%s]: Database has not been initialized", rrdhost_hostname(host));
        return;
    }

    char uuid_str[GUID_LEN + 1];
    uuid_unparse_lower_fix(&host->host_uuid, uuid_str);

    sql_check_removed_alerts_state(uuid_str);

    snprintfz(command, MAX_HEALTH_SQL_SIZE, SQL_LOAD_HEALTH_LOG(uuid_str, host->health_log.max));

    ret = sqlite3_prepare_v2(db_meta, command, -1, &res, 0);
    if (unlikely(ret != SQLITE_OK)) {
        error_report("HEALTH [%s]: Failed to prepare sql statement to load health log.", rrdhost_hostname(host));
        return;
    }

    netdata_rwlock_rdlock(&host->health_log.alarm_log_rwlock);

    while (sqlite3_step_monitored(res) == SQLITE_ROW) {
        ALARM_ENTRY *ae = NULL;

        // check that we have valid ids
        uint32_t unique_id = (uint32_t) sqlite3_column_int64(res, 1);
        if(!unique_id) {
            error_report("HEALTH [%s]: Got invalid unique id. Ignoring it.", rrdhost_hostname(host));
            errored++;
            continue;
        }

        uint32_t alarm_id = (uint32_t) sqlite3_column_int64(res, 2);
        if(!alarm_id) {
            error_report("HEALTH [%s]: Got invalid alarm id. Ignoring it.", rrdhost_hostname(host));
            errored++;
            continue;
        }

        //need name, chart and family
        if (sqlite3_column_type(res, 13) == SQLITE_NULL) {
            error_report("HEALTH [%s]: Got null name field. Ignoring it.", rrdhost_hostname(host));
            errored++;
            continue;
        }

        if (sqlite3_column_type(res, 14) == SQLITE_NULL) {
            error_report("HEALTH [%s]: Got null chart field. Ignoring it.", rrdhost_hostname(host));
            errored++;
            continue;
        }

        if (sqlite3_column_type(res, 15) == SQLITE_NULL) {
            error_report("HEALTH [%s]: Got null family field. Ignoring it.", rrdhost_hostname(host));
            errored++;
            continue;
        }

        // Check if we got last_repeat field
        time_t last_repeat = 0;
        last_repeat = (time_t)sqlite3_column_int64(res, 27);

        RRDCALC *rc = alarm_max_last_repeat(host, (char *) sqlite3_column_text(res, 14));
        if (!rc) {
            for(rc = host->alarms; rc ; rc = rc->next) {
                RRDCALC *rdcmp  = (RRDCALC *) avl_insert_lock(&(host)->alarms_idx_name, (avl_t *)rc);
                if(rdcmp != rc) {
                    error("Cannot insert the alarm index ID using log %s", rrdcalc_name(rc));
                }
            }

            rc = alarm_max_last_repeat(host, (char *) sqlite3_column_text(res, 14));
        }

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

        if (sqlite3_column_type(res, 4) != SQLITE_NULL)
            uuid_copy(ae->config_hash_id, *((uuid_t *) sqlite3_column_blob(res, 4)));

        ae->alarm_event_id = (uint32_t) sqlite3_column_int64(res, 3);
        ae->updated_by_id = (uint32_t) sqlite3_column_int64(res, 5);
        ae->updates_id = (uint32_t) sqlite3_column_int64(res, 6);

        ae->when = (time_t) sqlite3_column_int64(res, 7);
        ae->duration = (time_t) sqlite3_column_int64(res, 8);
        ae->non_clear_duration = (time_t) sqlite3_column_int64(res, 9);

        ae->flags = (uint32_t) sqlite3_column_int64(res, 10);
        ae->flags |= HEALTH_ENTRY_FLAG_SAVED;

        ae->exec_run_timestamp = (time_t) sqlite3_column_int64(res, 11);
        ae->delay_up_to_timestamp = (time_t) sqlite3_column_int64(res, 12);

        ae->name   = string_strdupz((char *) sqlite3_column_text(res, 13));
        ae->chart  = string_strdupz((char *) sqlite3_column_text(res, 14));
        ae->family = string_strdupz((char *) sqlite3_column_text(res, 15));

        if (sqlite3_column_type(res, 16) != SQLITE_NULL)
            ae->exec = string_strdupz((char *) sqlite3_column_text(res, 16));
        else
            ae->exec = NULL;

        if (sqlite3_column_type(res, 17) != SQLITE_NULL)
            ae->recipient = string_strdupz((char *) sqlite3_column_text(res, 17));
        else
            ae->recipient = NULL;

        if (sqlite3_column_type(res, 18) != SQLITE_NULL)
            ae->source = string_strdupz((char *) sqlite3_column_text(res, 18));
        else
            ae->source = NULL;

        if (sqlite3_column_type(res, 19) != SQLITE_NULL)
            ae->units = string_strdupz((char *) sqlite3_column_text(res, 19));
        else
            ae->units = NULL;

        if (sqlite3_column_type(res, 20) != SQLITE_NULL)
            ae->info = string_strdupz((char *) sqlite3_column_text(res, 20));
        else
            ae->info = NULL;

        ae->exec_code   = (int) sqlite3_column_int(res, 21);
        ae->new_status  = (RRDCALC_STATUS) sqlite3_column_int(res, 22);
        ae->old_status  = (RRDCALC_STATUS)sqlite3_column_int(res, 23);
        ae->delay       = (int) sqlite3_column_int(res, 24);

        ae->new_value   = (NETDATA_DOUBLE) sqlite3_column_double(res, 25);
        ae->old_value   = (NETDATA_DOUBLE) sqlite3_column_double(res, 26);

        ae->last_repeat = last_repeat;

        if (sqlite3_column_type(res, 28) != SQLITE_NULL)
            ae->classification = string_strdupz((char *) sqlite3_column_text(res, 28));
        else
            ae->classification = NULL;

        if (sqlite3_column_type(res, 29) != SQLITE_NULL)
            ae->component = string_strdupz((char *) sqlite3_column_text(res, 29));
        else
            ae->component = NULL;

        if (sqlite3_column_type(res, 30) != SQLITE_NULL)
            ae->type = string_strdupz((char *) sqlite3_column_text(res, 30));
        else
            ae->type = NULL;

        if (sqlite3_column_type(res, 31) != SQLITE_NULL)
            ae->chart_context = string_strdupz((char *) sqlite3_column_text(res, 31));
        else
            ae->chart_context = NULL;

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

    netdata_rwlock_unlock(&host->health_log.alarm_log_rwlock);

    if(!host->health_max_unique_id) host->health_max_unique_id = (uint32_t)now_realtime_sec();
    if(!host->health_max_alarm_id)  host->health_max_alarm_id  = (uint32_t)now_realtime_sec();

    host->health_log.next_log_id = host->health_max_unique_id + 1;
    if (unlikely(!host->health_log.next_alarm_id || host->health_log.next_alarm_id <= host->health_max_alarm_id))
        host->health_log.next_alarm_id = host->health_max_alarm_id + 1;

    info("HEALTH [%s]: Table health_log_%s, loaded %zd alarm entries, errors in %zd entries.", rrdhost_hostname(host), uuid_str, loaded, errored);

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
    "p_db_lookup_before, p_update_every) values (?1,unixepoch(),?2,?3,?4,?5,?6,?7,?8,?9,?10,?11,?12," \
    "?13,?14,?15,?16,?17,?18,?19,?20,?21,?22,?23,?24,?25,?26,?27,?28,?29,?30,?31,?32,?33,?34);"

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

    param++;
    rc = sqlite3_bind_blob(res, 1, hash_id, sizeof(*hash_id), SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    param++;
    if (cfg->alarm)
        rc = sqlite3_bind_text(res, 2, string2str(cfg->alarm), -1, SQLITE_STATIC);
    else
        rc = sqlite3_bind_null(res, 2);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    param++;
    if (cfg->template_key)
        rc = sqlite3_bind_text(res, 3, string2str(cfg->template_key), -1, SQLITE_STATIC);
    else
        rc = sqlite3_bind_null(res, 3);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    param++;
    rc = sqlite3_bind_text(res, 4, string2str(cfg->on), -1, SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    param++;
    rc = sqlite3_bind_text(res, 5, string2str(cfg->classification), -1, SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    param++;
    rc = sqlite3_bind_text(res, 6, string2str(cfg->component), -1, SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    param++;
    rc = sqlite3_bind_text(res, 7, string2str(cfg->type), -1, SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    param++;
    rc = sqlite3_bind_text(res, 8, string2str(cfg->os), -1, SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    param++;
    rc = sqlite3_bind_text(res, 9, string2str(cfg->host), -1, SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    param++;
    rc = sqlite3_bind_text(res, 10, string2str(cfg->lookup), -1, SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    param++;
    rc = sqlite3_bind_text(res, 11, string2str(cfg->every), -1, SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    param++;
    rc = sqlite3_bind_text(res, 12, string2str(cfg->units), -1, SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    param++;
    rc = sqlite3_bind_text(res, 13, string2str(cfg->calc), -1, SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    param++;
    rc = sqlite3_bind_text(res, 14, string2str(cfg->families), -1, SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    param++;
    rc = sqlite3_bind_text(res, 15, string2str(cfg->plugin), -1, SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    param++;
    rc = sqlite3_bind_text(res, 16, string2str(cfg->module), -1, SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    param++;
    rc = sqlite3_bind_text(res, 17, string2str(cfg->charts), -1, SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    param++;
    rc = sqlite3_bind_text(res, 18, string2str(cfg->green), -1, SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    param++;
    rc = sqlite3_bind_text(res, 19, string2str(cfg->red), -1, SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    param++;
    rc = sqlite3_bind_text(res, 20, string2str(cfg->warn), -1, SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    param++;
    rc = sqlite3_bind_text(res, 21, string2str(cfg->crit), -1, SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    param++;
    rc = sqlite3_bind_text(res, 22, string2str(cfg->exec), -1, SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    param++;
    rc = sqlite3_bind_text(res, 23, string2str(cfg->to), -1, SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    param++;
    rc = sqlite3_bind_text(res, 24, string2str(cfg->info), -1, SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    param++;
    rc = sqlite3_bind_text(res, 25, string2str(cfg->delay), -1, SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    param++;
    rc = sqlite3_bind_text(res, 26, string2str(cfg->options), -1, SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    param++;
    rc = sqlite3_bind_text(res, 27, string2str(cfg->repeat), -1, SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    param++;
    rc = sqlite3_bind_text(res, 28, string2str(cfg->host_labels), -1, SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    if (cfg->p_db_lookup_after) {
        param++;
        rc = sqlite3_bind_text(res, 29, string2str(cfg->p_db_lookup_dimensions), -1, SQLITE_STATIC);
        if (unlikely(rc != SQLITE_OK))
            goto bind_fail;

        param++;
        rc = sqlite3_bind_text(res, 30, string2str(cfg->p_db_lookup_method), -1, SQLITE_STATIC);
        if (unlikely(rc != SQLITE_OK))
            goto bind_fail;

        param++;
        rc = sqlite3_bind_int(res, 31, cfg->p_db_lookup_options);
        if (unlikely(rc != SQLITE_OK))
            goto bind_fail;

        param++;
        rc = sqlite3_bind_int(res, 32, cfg->p_db_lookup_after);
        if (unlikely(rc != SQLITE_OK))
            goto bind_fail;

        param++;
        rc = sqlite3_bind_int(res, 33, cfg->p_db_lookup_before);
        if (unlikely(rc != SQLITE_OK))
            goto bind_fail;
    } else {
        param++;
        rc = sqlite3_bind_null(res, 29);
        if (unlikely(rc != SQLITE_OK))
            goto bind_fail;
        param++;
        rc = sqlite3_bind_null(res, 30);
        if (unlikely(rc != SQLITE_OK))
            goto bind_fail;
        param++;
        rc = sqlite3_bind_null(res, 31);
        if (unlikely(rc != SQLITE_OK))
            goto bind_fail;
        param++;
        rc = sqlite3_bind_null(res, 32);
        if (unlikely(rc != SQLITE_OK))
            goto bind_fail;
        param++;
        rc = sqlite3_bind_null(res, 33);
        if (unlikely(rc != SQLITE_OK))
            goto bind_fail;
    }

    param++;
    rc = sqlite3_bind_int(res, 34, cfg->p_update_every);
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
#define DIGEST_ALERT_CONFIG_VAL(v) EVP_DigestUpdate(evpctx, (string2str(v)), string_length(v))
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

    EVP_DigestFinal_ex(evpctx, hash_value, &hash_len);
    EVP_MD_CTX_destroy(evpctx);
    fatal_assert(hash_len > sizeof(uuid_t));

    char uuid_str[GUID_LEN + 1];
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
