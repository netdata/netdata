// SPDX-License-Identifier: GPL-3.0-or-later

#include "sqlite_health.h"
#include "sqlite_functions.h"

#define MAX_HEALTH_SQL_SIZE 2048

/* Health related SQL queries
   Creates a health log table in sqlite, one per host guid
*/
#define SQL_CREATE_HEALTH_LOG_TABLE(guid) "CREATE TABLE IF NOT EXISTS health_log_%s(hostname text, unique_id int, alarm_id int, alarm_event_id int, config_hash_id blob, updated_by_id int, updates_id int, when_key int, duration int, non_clear_duration int, flags int, exec_run_timestamp int, delay_up_to_timestamp int, name text, chart text, family text, exec text, recipient text, source text, units text, info text, exec_code int, new_status real, old_status real, delay int, new_value double, old_value double, last_repeat int, class text, component text, type text);", guid
int sql_create_health_log_table(RRDHOST *host) {
    int rc;
    char *err_msg = NULL, command[MAX_HEALTH_SQL_SIZE + 1];

    char uuid_str[GUID_LEN + 1];
    uuid_unparse_lower(host->host_uuid, uuid_str);
    uuid_str[8] = '_';
    uuid_str[13] = '_';
    uuid_str[18] = '_';
    uuid_str[23] = '_';

    snprintfz(command, MAX_HEALTH_SQL_SIZE, SQL_CREATE_HEALTH_LOG_TABLE(uuid_str));

    if (unlikely(!db_meta)) {
        if (default_rrd_memory_mode == RRD_MEMORY_MODE_DBENGINE)
            error_report("HEALTH [%s]: Database has not been initialized", host->hostname);
        return 1;
    }

    rc = sqlite3_exec(db_meta, command, 0, 0, &err_msg);
    if (rc != SQLITE_OK) {
        error_report("HEALTH [%s]: SQLite error during creation of health log table, rc = %d (%s)", host->hostname, rc, err_msg);
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
            error_report("HEALTH [%s]: Database has not been initialized", host->hostname);
        return;
    }

    char uuid_str[GUID_LEN + 1];
    uuid_unparse_lower(host->host_uuid, uuid_str);
    uuid_str[8] = '_';
    uuid_str[13] = '_';
    uuid_str[18] = '_';
    uuid_str[23] = '_';

    sprintf(command, SQL_UPDATE_HEALTH_LOG(uuid_str));

    rc = sqlite3_prepare_v2(db_meta, command, -1, &res, 0);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("HEALTH [%s]: Failed to prepare statement for SQL_UPDATE_HEALTH_LOG", host->hostname);
        goto failed;
    }

    rc = sqlite3_bind_int(res, 1, ae->updated_by_id);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to bind updated_by_id parameter for SQL_UPDATE_HEALTH_LOG");
        goto failed;
    }

    rc = sqlite3_bind_int64(res, 2, (uint64_t)ae->flags);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to bind flags parameter for SQL_UPDATE_HEALTH_LOG");
        goto failed;
    }

    rc = sqlite3_bind_int(res, 3, ae->exec_run_timestamp);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to bind exec_run_timestamp parameter for SQL_UPDATE_HEALTH_LOG");
        goto failed;
    }

    rc = sqlite3_bind_int(res, 4, ae->exec_code);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to bind exec_code parameter for SQL_UPDATE_HEALTH_LOG");
        goto failed;
    }

    rc = sqlite3_bind_int(res, 5, ae->unique_id);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to bind unique_id parameter for SQL_UPDATE_HEALTH_LOG");
        goto failed;
    }

    rc = execute_insert(res);
    if (unlikely(rc != SQLITE_DONE)) {
        error_report("HEALTH [%s]: Failed to update health log, rc = %d", host->hostname, rc);
    }

    failed:
    if (unlikely(sqlite3_finalize(res) != SQLITE_OK))
        error_report("HEALTH [%s]: Failed to finalize the prepared statement for updating health log.", host->hostname);

    return;
}

/* Health related SQL queries
   Inserts an entry in the table
*/
#define SQL_INSERT_HEALTH_LOG(guid) "INSERT INTO health_log_%s(hostname, unique_id, alarm_id, alarm_event_id, config_hash_id, updated_by_id, updates_id, when_key, duration, non_clear_duration, flags, exec_run_timestamp, delay_up_to_timestamp, name, chart, family, exec, recipient, source, units, info, exec_code, new_status, old_status, delay, new_value, old_value, last_repeat, class, component, type) values (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?);", guid
void sql_health_alarm_log_insert(RRDHOST *host, ALARM_ENTRY *ae) {
    sqlite3_stmt *res = NULL;
    int rc;
    char command[MAX_HEALTH_SQL_SIZE + 1];

    if (unlikely(!db_meta)) {
        if (default_rrd_memory_mode == RRD_MEMORY_MODE_DBENGINE)
            error_report("HEALTH [%s]: Database has not been initialized", host->hostname);
        return;
    }

    char uuid_str[GUID_LEN + 1];
    uuid_unparse_lower(host->host_uuid, uuid_str);
    uuid_str[8] = '_';
    uuid_str[13] = '_';
    uuid_str[18] = '_';
    uuid_str[23] = '_';

    snprintfz(command, MAX_HEALTH_SQL_SIZE, SQL_INSERT_HEALTH_LOG(uuid_str));

    rc = sqlite3_prepare_v2(db_meta, command, -1, &res, 0);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("HEALTH [%s]: Failed to prepare statement for SQL_INSERT_HEALTH_LOG", host->hostname);
        return;
    }

    rc = sqlite3_bind_text(res, 1, host->hostname, -1, SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to bind hostname parameter for SQL_INSERT_HEALTH_LOG");
        goto failed;
    }

    rc = sqlite3_bind_int(res, 2, ae->unique_id);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to bind unique_id parameter for SQL_INSERT_HEALTH_LOG");
        goto failed;
    }

    rc = sqlite3_bind_int(res, 3, ae->alarm_id);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to bind alarm_id parameter for SQL_INSERT_HEALTH_LOG");
        goto failed;
    }

    rc = sqlite3_bind_int(res, 4, ae->alarm_event_id);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to bind alarm_event_id parameter for SQL_INSERT_HEALTH_LOG");
        goto failed;
    }

    rc = sqlite3_bind_blob(res, 5, ae->config_hash_id, sizeof(ae->config_hash_id), SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to bind config_hash_id parameter for SQL_INSERT_HEALTH_LOG");
        goto failed;
    }

    rc = sqlite3_bind_int(res, 6, ae->updated_by_id);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to bind updated_by_id parameter for SQL_INSERT_HEALTH_LOG");
        goto failed;
    }

    rc = sqlite3_bind_int(res, 7, ae->updates_id);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to bind updates_id parameter for SQL_INSERT_HEALTH_LOG");
        goto failed;
    }

    rc = sqlite3_bind_int(res, 8, ae->when);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to bind when parameter for SQL_INSERT_HEALTH_LOG");
        goto failed;
    }

    rc = sqlite3_bind_int(res, 9, ae->duration);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to bind duration parameter for SQL_INSERT_HEALTH_LOG");
        goto failed;
    }

    rc = sqlite3_bind_int(res, 10, ae->non_clear_duration);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to bind non_clear_duration parameter for SQL_INSERT_HEALTH_LOG");
        goto failed;
    }

    rc = sqlite3_bind_int64(res, 11, (uint64_t)ae->flags);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to bind flags parameter for SQL_INSERT_HEALTH_LOG");
        goto failed;
    }

    rc = sqlite3_bind_int(res, 12, ae->exec_run_timestamp);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to bind exec_run_timestamp parameter for SQL_INSERT_HEALTH_LOG");
        goto failed;
    }

    rc = sqlite3_bind_int(res, 13, ae->delay_up_to_timestamp);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to bind delay_up_to_timestamp parameter for SQL_INSERT_HEALTH_LOG");
        goto failed;
    }

    rc = sqlite3_bind_text(res, 14, ae->name, -1, SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to bind name parameter for SQL_INSERT_HEALTH_LOG");
        goto failed;
    }

    rc = sqlite3_bind_text(res, 15, ae->chart, -1, SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to bind chart parameter for SQL_INSERT_HEALTH_LOG");
        goto failed;
    }

    rc = sqlite3_bind_text(res, 16, ae->family, -1, SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to bind family parameter for SQL_INSERT_HEALTH_LOG");
        goto failed;
    }

    rc = sqlite3_bind_text(res, 17, ae->exec, -1, SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to bind exec parameter for SQL_INSERT_HEALTH_LOG");
        goto failed;
    }

    rc = sqlite3_bind_text(res, 18, ae->recipient, -1, SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to bind recipient parameter for SQL_INSERT_HEALTH_LOG");
        goto failed;
    }

    rc = sqlite3_bind_text(res, 19, ae->source, -1, SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to bind source parameter for SQL_INSERT_HEALTH_LOG");
        goto failed;
    }

    rc = sqlite3_bind_text(res, 20, ae->units, -1, SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to bind host_id parameter to store node instance information");
        goto failed;
    }

    rc = sqlite3_bind_text(res, 21, ae->info, -1, SQLITE_STATIC);
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

    rc = sqlite3_bind_int(res, 28, ae->last_repeat);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to bind last_repeat parameter for SQL_INSERT_HEALTH_LOG");
        goto failed;
    }

    rc = sqlite3_bind_text(res, 29, ae->classification, -1, SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to bind classification parameter for SQL_INSERT_HEALTH_LOG");
        goto failed;
    }

    rc = sqlite3_bind_text(res, 30, ae->component, -1, SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to bind component parameter for SQL_INSERT_HEALTH_LOG");
        goto failed;
    }

    rc = sqlite3_bind_text(res, 31, ae->type, -1, SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to bind type parameter for SQL_INSERT_HEALTH_LOG");
        goto failed;
    }

    rc = execute_insert(res);
    if (unlikely(rc != SQLITE_DONE)) {
        error_report("HEALTH [%s]: Failed to execute SQL_INSERT_HEALTH_LOG, rc = %d", host->hostname, rc);
        goto failed;
    }

    ae->flags |= HEALTH_ENTRY_FLAG_SAVED;
    host->health_log_entries_written++;

    failed:
    if (unlikely(sqlite3_finalize(res) != SQLITE_OK))
        error_report("HEALTH [%s]: Failed to finalize the prepared statement for inserting to health log.", host->hostname);

    return;
}

void sql_health_alarm_log_save(RRDHOST *host, ALARM_ENTRY *ae)
{
    if (ae->flags & HEALTH_ENTRY_FLAG_SAVED)
        sql_health_alarm_log_update(host, ae);
    else
        sql_health_alarm_log_insert(host, ae);
}


#define SQL_SELECT_HEALTH_LOG(guid) "SELECT hostname, unique_id, alarm_id, alarm_event_id, config_hash_id, updated_by_id, updates_id, when_key, duration, non_clear_duration, flags, exec_run_timestamp, delay_up_to_timestamp, name, chart, family, exec, recipient, source, units, info, exec_code, new_status, old_status, delay, new_value, old_value, last_repeat, class, component, type FROM health_log_%s where unique_id = ? and alarm_id = ?;", guid
void health_alarm_entry_sql2json(BUFFER *wb, uint32_t unique_id, uint32_t alarm_id, RRDHOST *host) {
    sqlite3_stmt *res = NULL;
    int rc;
    char *guid = NULL, command[1000];

    if (unlikely(!db_meta)) {
        if (default_rrd_memory_mode == RRD_MEMORY_MODE_DBENGINE)
            error_report("Database has not been initialized");
        return;
    }

    guid = strdupz(host->machine_guid);

    guid[8]='_';
    guid[13]='_';
    guid[18]='_';
    guid[23]='_';

    sprintf(command, SQL_SELECT_HEALTH_LOG(guid));

    rc = sqlite3_prepare_v2(db_meta, command, -1, &res, 0);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to prepare statement SQL_SELECT_HEALTH_LOG");
        return;
    }

    rc = sqlite3_bind_int(res, 1, unique_id);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to bind unique_id parameter to SQL_SELECT_HEALTH_LOG");
        debug(D_HEALTH, "GREPME2: Failed to bind 1");
        goto failed;
    }

    rc = sqlite3_bind_int(res, 2, alarm_id);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to bind alarm_id parameter to SQL_SELECT_HEALTH_LOG");
        goto failed;
    }

    while (sqlite3_step(res) == SQLITE_ROW) {

        char old_value_string[100 + 1];
        char new_value_string[100 + 1];

        char uuid_str[GUID_LEN + 1];
        uuid_unparse_lower(*((uuid_t *) sqlite3_column_blob(res, 4)), uuid_str);

        char *edit_command = health_edit_command_from_source((char *)sqlite3_column_text(res, 18));

        buffer_sprintf(
            wb,
            "\n\t{\n"
            "\t\t\"hostname\": \"%s\",\n"
            "\t\t\"utc_offset\": %d,\n"
            "\t\t\"timezone\": \"%s\",\n"
            "\t\t\"unique_id\": %u,\n"
            "\t\t\"alarm_id\": %u,\n"
            "\t\t\"alarm_event_id\": %u,\n"
            //"\t\t\"config_hash_id\": \"%s\",\n"
            "\t\t\"name\": \"%s\",\n"
            "\t\t\"chart\": \"%s\",\n"
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
            sqlite3_column_text(res, 0),
            host->utc_offset,
            host->abbrev_timezone,
            (unsigned int) sqlite3_column_int(res, 1),
            (unsigned int) sqlite3_column_int(res, 2),
            (unsigned int) sqlite3_column_int(res, 3),
            //uuid_str,
            sqlite3_column_text(res, 13),
            sqlite3_column_text(res, 14),
            sqlite3_column_text(res, 15),
            sqlite3_column_text(res, 28) ? (const char *) sqlite3_column_text(res, 28) : (char *) "Unknown",
            sqlite3_column_text(res, 29) ? (const char *) sqlite3_column_text(res, 29) : (char *) "Unknown",
            sqlite3_column_text(res, 30) ? (const char *) sqlite3_column_text(res, 30) : (char *) "Unknown",
            (sqlite3_column_int(res, 10) & HEALTH_ENTRY_FLAG_PROCESSED)?"true":"false",
            (sqlite3_column_int(res, 10) & HEALTH_ENTRY_FLAG_UPDATED)?"true":"false",
            (long unsigned int)sqlite3_column_int(res, 11),
            (sqlite3_column_int(res, 10) & HEALTH_ENTRY_FLAG_EXEC_FAILED)?"true":"false",
            sqlite3_column_text(res, 16) ? (const char *) sqlite3_column_text(res, 16) : host->health_default_exec,
            sqlite3_column_text(res, 17) ? (const char *) sqlite3_column_text(res, 17) : host->health_default_recipient,
            sqlite3_column_int(res, 21),
            sqlite3_column_text(res, 18),
            edit_command,
            sqlite3_column_text(res, 19),
            (long unsigned int)sqlite3_column_int(res, 7),
            (long unsigned int)sqlite3_column_int(res, 8),
            (long unsigned int)sqlite3_column_int(res, 9),
            rrdcalc_status2string(sqlite3_column_int(res, 22)),
            rrdcalc_status2string(sqlite3_column_int(res, 23)),
            sqlite3_column_int(res, 24),
            (long unsigned int)sqlite3_column_int(res, 12),
            (unsigned int)sqlite3_column_int(res, 5),
            (unsigned int)sqlite3_column_int(res, 6),
            sqlite3_column_type(res, 25) == SQLITE_NULL ? "-" : format_value_and_unit(new_value_string, 100, sqlite3_column_double(res, 25), (char *) sqlite3_column_text(res, 19), -1), //int instead of double, alarm_log rounds?

            sqlite3_column_type(res, 26) == SQLITE_NULL ? "-" : format_value_and_unit(old_value_string, 100, sqlite3_column_double(res, 26), (char *) sqlite3_column_text(res, 19), -1), //int instead of double, alarm_log rounds?
            (long unsigned int)sqlite3_column_int(res, 27),
            (sqlite3_column_int(res, 10) & HEALTH_ENTRY_FLAG_SILENCED)?"true":"false");

        //not needed, it's already stored with replaced info
        char *replaced_info = NULL;
        if (likely(sqlite3_column_text(res, 20))) {
            char *m = NULL;
            replaced_info = strdupz((char *) sqlite3_column_text(res, 20));
            size_t pos = 0;
            while ((m = strstr(replaced_info + pos, "$family"))) {
                char *buf = NULL;
                pos = m - replaced_info;
                buf = find_and_replace(replaced_info, "$family", (char *) sqlite3_column_text(res, 20) ? (const char *)sqlite3_column_text(res, 20) : "", m);
                freez(replaced_info);
                replaced_info = strdupz(buf);
                freez(buf);
            }
        }

        buffer_strcat(wb, "\t\t\"info\":\"");
        buffer_strcat(wb, replaced_info);
        buffer_strcat(wb, "\",\n");

        if(unlikely(sqlite3_column_int(res, 10) & HEALTH_ENTRY_FLAG_NO_CLEAR_NOTIFICATION)) {
            buffer_strcat(wb, "\t\t\"no_clear_notification\": true,\n");
        }

        buffer_strcat(wb, "\t\t\"value\":");
        if (sqlite3_column_type(res, 25) == SQLITE_NULL)
            buffer_strcat(wb, "null");
        else
            buffer_rrd_value(wb, sqlite3_column_double(res, 25)); //int instead of double, alarm_log rounds?
        buffer_strcat(wb, ",\n");

        buffer_strcat(wb, "\t\t\"old_value\":");
        if (sqlite3_column_type(res, 26) == SQLITE_NULL)
            buffer_strcat(wb, "null");
        else
            buffer_rrd_value(wb, sqlite3_column_double(res, 26)); //int instead of double, alarm_log rounds?
        buffer_strcat(wb, "\n");

        buffer_strcat(wb, "\t}");

        freez(replaced_info);
    }

    failed:
    rc = sqlite3_finalize(res);
    if (unlikely(rc != SQLITE_OK))
        error_report("Failed to finalize statement for SQL_SELECT_HEALTH_LOG");
}

#define SQL_SELECT_ALL_HEALTH_LOG(guid,limit) "SELECT unique_id, alarm_id FROM health_log_%s order by unique_id desc LIMIT %d;", guid,limit
void sql_health_alarm_log_select_all(BUFFER *wb, RRDHOST *host) {
    sqlite3_stmt *res = NULL;
    int rc;
    char *guid = NULL, command[1000];

    if (unlikely(!db_meta)) {
        if (default_rrd_memory_mode == RRD_MEMORY_MODE_DBENGINE)
            error_report("Database has not been initialized");
        return;
    }

    guid = strdupz(host->machine_guid);

    guid[8]='_';
    guid[13]='_';
    guid[18]='_';
    guid[23]='_';

    sprintf(command, SQL_SELECT_ALL_HEALTH_LOG(guid, host->health_log.max));

    rc = sqlite3_prepare_v2(db_meta, command, -1, &res, 0);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to prepare statement for SQL_SELECT_ALL_HEALTH_LOG");
        return;
    }

    int count=0;
    while (sqlite3_step(res) == SQLITE_ROW) {
        if (count) buffer_strcat(wb, ",");
        health_alarm_entry_sql2json(wb, sqlite3_column_int(res, 0), sqlite3_column_int(res, 1), host);
        count++;
    }

    rc = sqlite3_finalize(res);
    if (unlikely(rc != SQLITE_OK))
        error_report("Failed to finalize statement for SQL_SELECT_ALL_HEALTH_LOG");
}

/* Health related SQL queries
   Cleans up the health_log table.
*/
#define SQL_CLEANUP_HEALTH_LOG(guid,guid2,limit) "DELETE from health_log_%s where unique_id in (SELECT unique_id from health_log_%s order by unique_id asc LIMIT %ld);", guid, guid2, limit
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
    uuid_unparse_lower(host->host_uuid, uuid_str);
    uuid_str[8] = '_';
    uuid_str[13] = '_';
    uuid_str[18] = '_';
    uuid_str[23] = '_';

    snprintfz(command, MAX_HEALTH_SQL_SIZE, SQL_CLEANUP_HEALTH_LOG(uuid_str, uuid_str, host->health_log_entries_written - rotate_every));

    rc = sqlite3_prepare_v2(db_meta, command, -1, &res, 0);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to prepare statement to cleanup health log table");
        return;
    }

    rc = sqlite3_step(res);
    if (unlikely(rc != SQLITE_DONE))
        error_report("Failed to cleanup health log table, rc = %d", rc);

    host->health_log_entries_written = rotate_every;
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
    uuid_unparse_lower(host->host_uuid, uuid_str);
    uuid_str[8] = '_';
    uuid_str[13] = '_';
    uuid_str[18] = '_';
    uuid_str[23] = '_';

    snprintfz(command, MAX_HEALTH_SQL_SIZE, SQL_COUNT_HEALTH_LOG(uuid_str));

    rc = sqlite3_prepare_v2(db_meta, command, -1, &res, 0);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to prepare statement to count health log entries from db");
        return;
    }
    rc = sqlite3_step(res);

    host->health_log_entries_written = sqlite3_column_int(res, 0);

    rc = sqlite3_finalize(res);
    if (unlikely(rc != SQLITE_OK))
        error_report("Failed to finalize the prepared statement to count health log entries from db");
}

static uint32_t is_valid_alarm_id(RRDHOST *host, const char *chart, const char *name, uint32_t alarm_id)
{
    uint32_t hash_chart = simple_hash(chart);
    uint32_t hash_name = simple_hash(name);

    ALARM_ENTRY *ae;
    for(ae = host->health_log.alarms; ae ;ae = ae->next) {
        if (unlikely(
            ae->alarm_id == alarm_id && (!(ae->hash_name == hash_name && ae->hash_chart == hash_chart &&
                                           !strcmp(name, ae->name) && !strcmp(chart, ae->chart))))) {
            return 0;
        }
    }
    return 1;
}

/* Health related SQL queries
   Load from the health log table
*/
#define SQL_LOAD_HEALTH_LOG(guid,limit) "SELECT hostname, unique_id, alarm_id, alarm_event_id, config_hash_id, updated_by_id, updates_id, when_key, duration, non_clear_duration, flags, exec_run_timestamp, delay_up_to_timestamp, name, chart, family, exec, recipient, source, units, info, exec_code, new_status, old_status, delay, new_value, old_value, last_repeat, class, component, type FROM (SELECT hostname, unique_id, alarm_id, alarm_event_id, config_hash_id, updated_by_id, updates_id, when_key, duration, non_clear_duration, flags, exec_run_timestamp, delay_up_to_timestamp, name, chart, family, exec, recipient, source, units, info, exec_code, new_status, old_status, delay, new_value, old_value, last_repeat, class, component, type FROM health_log_%s order by unique_id desc limit %d) order by unique_id asc;", guid, limit
void sql_health_alarm_log_load(RRDHOST *host) {
    sqlite3_stmt *res = NULL;
    int rc;
    char command[MAX_HEALTH_SQL_SIZE + 1];

    host->health_log_entries_written = 0;

    if (unlikely(!db_meta)) {
        if (default_rrd_memory_mode == RRD_MEMORY_MODE_DBENGINE)
            error_report("HEALTH [%s]: Database has not been initialized", host->hostname);
        return;
    }

    netdata_rwlock_rdlock(&host->health_log.alarm_log_rwlock);

    char uuid_str[GUID_LEN + 1];
    uuid_unparse_lower(host->host_uuid, uuid_str);
    uuid_str[8] = '_';
    uuid_str[13] = '_';
    uuid_str[18] = '_';
    uuid_str[23] = '_';

    snprintfz(command, MAX_HEALTH_SQL_SIZE, SQL_LOAD_HEALTH_LOG(uuid_str, host->health_log.max));

    rc = sqlite3_prepare_v2(db_meta, command, -1, &res, 0);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("HEALTH [%s]: Failed to prepare sql statement to load health log.", host->hostname);
        return;
    }

    while (sqlite3_step(res) == SQLITE_ROW) {
        errno = 0;
        ssize_t errored = 0;

        ALARM_ENTRY *ae = NULL;

        // check that we have valid ids
        uint32_t unique_id = sqlite3_column_int(res, 1);
        if(!unique_id) {
            error_report("HEALTH [%s]: Got invalid unique id. Ignoring it.", host->hostname);
            errored++;
            continue;
        }

        uint32_t alarm_id = sqlite3_column_int(res, 2);
        if(!alarm_id) {
            error_report("HEALTH [%s]: Got invalid alarm id. Ignoring it.", host->hostname);
            errored++;
            continue;
        }

        // Check if we got last_repeat field
        time_t last_repeat = 0;
        char* alarm_name = strdupz((char *) sqlite3_column_text(res, 14));
        last_repeat = (time_t)sqlite3_column_int(res, 27);

        RRDCALC *rc = alarm_max_last_repeat(host, alarm_name,simple_hash(alarm_name));
        if (!rc) {
            for(rc = host->alarms; rc ; rc = rc->next) {
                RRDCALC *rdcmp  = (RRDCALC *) avl_insert_lock(&(host)->alarms_idx_name, (avl_t *)rc);
                if(rdcmp != rc) {
                    error("Cannot insert the alarm index ID using log %s", rc->name);
                }
            }

            rc = alarm_max_last_repeat(host, alarm_name,simple_hash(alarm_name));
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

        freez(alarm_name);

        // check for a possible host mismatch
        if(strcmp((char *) sqlite3_column_text(res, 0), host->hostname)) {
            error("HEALTH [%s]: Unique id %u provides an alarm for host '%s' but this is named '%s'. Ignoring it.", host->hostname, unique_id, (char *) sqlite3_column_text(res, 0), host->hostname);
            errored++;
            continue;
        }

        ae = callocz(1, sizeof(ALARM_ENTRY));

        ae->unique_id = unique_id;

        if (!is_valid_alarm_id(host, (const char *) sqlite3_column_text(res, 14), (const char *) sqlite3_column_text(res, 13), alarm_id))
            alarm_id = rrdcalc_get_unique_id(host, (const char *) sqlite3_column_text(res, 14), (const char *) sqlite3_column_text(res, 13), NULL);
        ae->alarm_id = alarm_id;

        uuid_copy(ae->config_hash_id, *((uuid_t *) sqlite3_column_blob(res, 4)));

        ae->alarm_event_id = sqlite3_column_int(res, 3);
        ae->updated_by_id = sqlite3_column_int(res, 5);
        ae->updates_id = sqlite3_column_int(res, 6);

        ae->when = sqlite3_column_int(res, 7);
        ae->duration = sqlite3_column_int(res, 8);
        ae->non_clear_duration = sqlite3_column_int(res, 9);

        ae->flags = sqlite3_column_int64(res, 10);
        ae->flags |= HEALTH_ENTRY_FLAG_SAVED;

        ae->exec_run_timestamp = sqlite3_column_int(res, 11);
        ae->delay_up_to_timestamp = sqlite3_column_int(res, 12);

        ae->name = strdupz((char *) sqlite3_column_text(res, 13));
        ae->hash_name = simple_hash(ae->name);

        ae->chart = strdupz((char *) sqlite3_column_text(res, 14));
        ae->hash_chart = simple_hash(ae->chart);

        ae->family = strdupz((char *) sqlite3_column_text(res, 15));

        if (sqlite3_column_type(res, 16) != SQLITE_NULL)
            ae->exec = strdupz((char *) sqlite3_column_text(res, 16));
        else
            ae->exec = NULL;

        if (sqlite3_column_type(res, 17) != SQLITE_NULL)
            ae->recipient = strdupz((char *) sqlite3_column_text(res, 17));
        else
            ae->recipient = NULL;

        if (sqlite3_column_type(res, 18) != SQLITE_NULL)
            ae->source = strdupz((char *) sqlite3_column_text(res, 18));
        else
            ae->source = NULL;

        if (sqlite3_column_type(res, 19) != SQLITE_NULL)
            ae->units = strdupz((char *) sqlite3_column_text(res, 19));
        else
            ae->units = NULL;

        if (sqlite3_column_type(res, 20) != SQLITE_NULL)
            ae->info = strdupz((char *) sqlite3_column_text(res, 20));
        else
            ae->info = NULL;

        ae->exec_code   = sqlite3_column_int(res, 21);
        ae->new_status  = sqlite3_column_int(res, 22);
        ae->old_status  = sqlite3_column_int(res, 23);
        ae->delay       = sqlite3_column_int(res, 24);

        ae->new_value   = sqlite3_column_double(res, 25);
        ae->old_value   = sqlite3_column_double(res, 26);

        ae->last_repeat = last_repeat;

        if (sqlite3_column_type(res, 28) != SQLITE_NULL)
            ae->classification = strdupz((char *) sqlite3_column_text(res, 28));
        else
            ae->classification = NULL;

        if (sqlite3_column_type(res, 29) != SQLITE_NULL)
            ae->component = strdupz((char *) sqlite3_column_text(res, 29));
        else
            ae->component = NULL;

        if (sqlite3_column_type(res, 30) != SQLITE_NULL)
            ae->type = strdupz((char *) sqlite3_column_text(res, 30));
        else
            ae->type = NULL;

        char value_string[100 + 1];
        freez(ae->old_value_string);
        freez(ae->new_value_string);
        ae->old_value_string = strdupz(format_value_and_unit(value_string, 100, ae->old_value, ae->units, -1));
        ae->new_value_string = strdupz(format_value_and_unit(value_string, 100, ae->new_value, ae->units, -1));

        ae->next = host->health_log.alarms;
        host->health_log.alarms = ae;

        if(unlikely(ae->unique_id > host->health_max_unique_id))
            host->health_max_unique_id = ae->unique_id;

        if(unlikely(ae->alarm_id >= host->health_max_alarm_id))
            host->health_max_alarm_id = ae->alarm_id;
    }

    netdata_rwlock_unlock(&host->health_log.alarm_log_rwlock);

    if(!host->health_max_unique_id) host->health_max_unique_id = (uint32_t)now_realtime_sec();
    if(!host->health_max_alarm_id)  host->health_max_alarm_id  = (uint32_t)now_realtime_sec();

    host->health_log.next_log_id = host->health_max_unique_id + 1;
    if (unlikely(!host->health_log.next_alarm_id || host->health_log.next_alarm_id <= host->health_max_alarm_id))
        host->health_log.next_alarm_id = host->health_max_alarm_id + 1;

    //debug(D_HEALTH, "HEALTH [%s]: loaded file '%s' with %zd new alarm entries, updated %zd alarms, errors %zd entries, duplicate %zd", host->hostname, filename, loaded, updated, errored, duplicate);

    rc = sqlite3_finalize(res);
    if (unlikely(rc != SQLITE_OK))
        error_report("Failed to finalize the health log read statement");

    sql_health_alarm_log_count(host);
}

/*
 * Store an alert config hash in the database
 */
#define SQL_STORE_ALERT_CONFIG_HASH "insert into alert_hash (hash_id, alarm, template, on_key, class, component, type, os, hosts, lookup, every, units, calc, families, plugin, module, charts, green, red, warn, crit, exec, to_key, info, delay, options, repeat, host_labels, p_db_lookup_dimensions, p_db_lookup_method, p_db_lookup_options, p_db_lookup_after, p_db_lookup_before, p_update_every) values (?1,?2,?3,?4,?5,?6,?7,?8,?9,?10,?11,?12,?13,?14,?15,?16,?17,?18,?19,?20,?21,?22,?23,?24,?25,?26,?27,?28,?29,?30,?31,?32,?33,?34) on conflict(hash_id) do nothing;"
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
    if (cfg->alarm && *cfg->alarm)
        rc = sqlite3_bind_text(res, 2, cfg->alarm, -1, SQLITE_STATIC);
    else
        rc = sqlite3_bind_null(res, 2);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    param++;
    if (cfg->template_key && *cfg->template_key)
        rc = sqlite3_bind_text(res, 3, cfg->template_key, -1, SQLITE_STATIC);
    else
        rc = sqlite3_bind_null(res, 3);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    param++;
    rc = sqlite3_bind_text(res, 4, cfg->on, -1, SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    param++;
    rc = sqlite3_bind_text(res, 5, cfg->classification, -1, SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    param++;
    rc = sqlite3_bind_text(res, 6, cfg->component, -1, SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    param++;
    rc = sqlite3_bind_text(res, 7, cfg->type, -1, SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    param++;
    rc = sqlite3_bind_text(res, 8, cfg->os, -1, SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    param++;
    rc = sqlite3_bind_text(res, 9, cfg->host, -1, SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    param++;
    rc = sqlite3_bind_text(res, 10, cfg->lookup, -1, SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    param++;
    rc = sqlite3_bind_text(res, 11, cfg->every, -1, SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    param++;
    rc = sqlite3_bind_text(res, 12, cfg->units, -1, SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    param++;
    rc = sqlite3_bind_text(res, 13, cfg->calc, -1, SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    param++;
    rc = sqlite3_bind_text(res, 14, cfg->families, -1, SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    param++;
    rc = sqlite3_bind_text(res, 15, cfg->plugin, -1, SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    param++;
    rc = sqlite3_bind_text(res, 16, cfg->module, -1, SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    param++;
    rc = sqlite3_bind_text(res, 17, cfg->charts, -1, SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    param++;
    rc = sqlite3_bind_text(res, 18, cfg->green, -1, SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    param++;
    rc = sqlite3_bind_text(res, 19, cfg->red, -1, SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    param++;
    rc = sqlite3_bind_text(res, 20, cfg->warn, -1, SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    param++;
    rc = sqlite3_bind_text(res, 21, cfg->crit, -1, SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    param++;
    rc = sqlite3_bind_text(res, 22, cfg->exec, -1, SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    param++;
    rc = sqlite3_bind_text(res, 23, cfg->to, -1, SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    param++;
    rc = sqlite3_bind_text(res, 24, cfg->info, -1, SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    param++;
    rc = sqlite3_bind_text(res, 25, cfg->delay, -1, SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    param++;
    rc = sqlite3_bind_text(res, 26, cfg->options, -1, SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    param++;
    rc = sqlite3_bind_text(res, 27, cfg->repeat, -1, SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    param++;
    rc = sqlite3_bind_text(res, 28, cfg->host_labels, -1, SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    if (cfg->p_db_lookup_after) {
        param++;
        rc = sqlite3_bind_text(res, 29, cfg->p_db_lookup_dimensions, -1, SQLITE_STATIC);
        if (unlikely(rc != SQLITE_OK))
            goto bind_fail;

        param++;
        rc = sqlite3_bind_text(res, 30, cfg->p_db_lookup_method, -1, SQLITE_STATIC);
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
 * Select an alert config
 */
#define SQL_SELECT_ALERT_CONFIG_WITH_HASH "select hash_id, alarm, template, on_key, class, component, type, os, hosts, lookup, every, units, calc, families, plugin, module, green, red, warn, crit, exec, to_key, info, delay, options, repeat, host_labels, p_db_lookup_dimensions, p_db_lookup_method, p_db_lookup_options, p_db_lookup_after, p_db_lookup_before, p_update_every from alert_hash where hash_id = @hash_id;"
#define SQL_SELECT_ALERT_CONFIG "select hash_id, alarm, template, on_key, class, component, type, os, hosts, lookup, every, units, calc, families, plugin, module, green, red, warn, crit, exec, to_key, info, delay, options, repeat, host_labels, p_db_lookup_dimensions, p_db_lookup_method, p_db_lookup_options, p_db_lookup_after, p_db_lookup_before, p_update_every from alert_hash;"
void sql_select_alert_config(char *hash_str, BUFFER *wb)
{
    int rc;
    sqlite3_stmt *res_alert = NULL;

    uuid_t hash_id;
    if (hash_str)
        uuid_parse(hash_str, hash_id);

    if (hash_str)
        rc = sqlite3_prepare_v2(db_meta, SQL_SELECT_ALERT_CONFIG_WITH_HASH, -1, &res_alert, 0);
    else
        rc = sqlite3_prepare_v2(db_meta, SQL_SELECT_ALERT_CONFIG, -1, &res_alert, 0);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to prepare statement to fetch chart config with hash");
        return;
    }

    if (hash_str) {
        rc = sqlite3_bind_blob(res_alert, 1, &hash_id, sizeof(hash_id), SQLITE_STATIC);
        if (unlikely(rc != SQLITE_OK)) {
            error_report("Failed to bind host parameter to fetch chart config with hash");
            goto failed;
        }
    }

    buffer_sprintf(wb, "[\n");

    size_t c = 0;

    while (sqlite3_step(res_alert) == SQLITE_ROW) {
        /* char id[512]; */
        /* sprintf(id, "%s.%s", sqlite3_column_text(res_alert, 3), sqlite3_column_text(res_alert, 1)); */

        if (c)
            buffer_strcat(wb, ",\t\t\n");
        else
            buffer_strcat(wb, "\t\n");
        c++;

        char uuid_str[36 + 1];
        uuid_unparse_lower(*((uuid_t *)sqlite3_column_blob(res_alert, 0)), uuid_str);

        buffer_sprintf(wb, "\t{\n");
        buffer_sprintf(wb, "\t\t\"config_hash_id\": \"%s\"", uuid_str);

        if (sqlite3_column_type(res_alert, 1) != SQLITE_NULL) {
            buffer_sprintf(wb, ",\n\t\t\"alarm\": \"%s\"", sqlite3_column_text(res_alert, 1));
        }
        if (sqlite3_column_type(res_alert, 2) != SQLITE_NULL) {
            buffer_sprintf(wb, ",\n\t\t\"template\": \"%s\"", sqlite3_column_text(res_alert, 2));
        }
        if (sqlite3_column_type(res_alert, 3) != SQLITE_NULL) {
            buffer_sprintf(wb, ",\n\t\t\"on\": \"%s\"", sqlite3_column_text(res_alert, 3));
        }
        if (sqlite3_column_type(res_alert, 4) != SQLITE_NULL) {
            buffer_sprintf(wb, ",\n\t\t\"class\": \"%s\"", sqlite3_column_text(res_alert, 4));
        }
        if (sqlite3_column_type(res_alert, 5) != SQLITE_NULL) {
            buffer_sprintf(wb, ",\n\t\t\"component\": \"%s\"", sqlite3_column_text(res_alert, 5));
        }
        if (sqlite3_column_type(res_alert, 6) != SQLITE_NULL) {
            buffer_sprintf(wb, ",\n\t\t\"type\": \"%s\"", sqlite3_column_text(res_alert, 6));
        }
        if (sqlite3_column_type(res_alert, 7) != SQLITE_NULL) {
            buffer_sprintf(wb, ",\n\t\t\"os\": \"%s\"", sqlite3_column_text(res_alert, 7));
        }
        if (sqlite3_column_type(res_alert, 8) != SQLITE_NULL) {
            buffer_sprintf(wb, ",\n\t\t\"hosts\": \"%s\"", sqlite3_column_text(res_alert, 8));
        }
        if (sqlite3_column_type(res_alert, 9) != SQLITE_NULL) {
            buffer_sprintf(wb, ",\n\t\t\"lookup\": \"%s\"", sqlite3_column_text(res_alert, 9));
        }
        if (sqlite3_column_type(res_alert, 10) != SQLITE_NULL) {
            buffer_sprintf(wb, ",\n\t\t\"every\": \"%s\"", sqlite3_column_text(res_alert, 10));
        }
        if (sqlite3_column_type(res_alert, 11) != SQLITE_NULL) {
            buffer_sprintf(wb, ",\n\t\t\"units\": \"%s\"", sqlite3_column_text(res_alert, 11));
        }
        if (sqlite3_column_type(res_alert, 12) != SQLITE_NULL) {
            buffer_sprintf(wb, ",\n\t\t\"calc\": \"%s\"", sqlite3_column_text(res_alert, 12));
        }
        if (sqlite3_column_type(res_alert, 13) != SQLITE_NULL) {
            buffer_sprintf(wb, ",\n\t\t\"families\": \"%s\"", sqlite3_column_text(res_alert, 13));
        }
        if (sqlite3_column_type(res_alert, 14) != SQLITE_NULL) {
            buffer_sprintf(wb, ",\n\t\t\"plugin\": \"%s\"", sqlite3_column_text(res_alert, 14));
        }
        if (sqlite3_column_type(res_alert, 15) != SQLITE_NULL) {
            buffer_sprintf(wb, ",\n\t\t\"module\": \"%s\"", sqlite3_column_text(res_alert, 15));
        }
        if (sqlite3_column_type(res_alert, 16) != SQLITE_NULL) {
            buffer_sprintf(wb, ",\n\t\t\"green\": \"%s\"", sqlite3_column_text(res_alert, 16));
        }
        if (sqlite3_column_type(res_alert, 17) != SQLITE_NULL) {
            buffer_sprintf(wb, ",\n\t\t\"red\": \"%s\"", sqlite3_column_text(res_alert, 17));
        }
        if (sqlite3_column_type(res_alert, 18) != SQLITE_NULL) {
            buffer_sprintf(wb, ",\n\t\t\"warn\": \"%s\"", sqlite3_column_text(res_alert, 18));
        }
        if (sqlite3_column_type(res_alert, 19) != SQLITE_NULL) {
            buffer_sprintf(wb, ",\n\t\t\"crit\": \"%s\"", sqlite3_column_text(res_alert, 19));
        }
        if (sqlite3_column_type(res_alert, 20) != SQLITE_NULL) {
            buffer_sprintf(wb, ",\n\t\t\"exec\": \"%s\"", sqlite3_column_text(res_alert, 20));
        }
        if (sqlite3_column_type(res_alert, 21) != SQLITE_NULL) {
            buffer_sprintf(wb, ",\n\t\t\"to\": \"%s\"", sqlite3_column_text(res_alert, 21));
        }
        if (sqlite3_column_type(res_alert, 22) != SQLITE_NULL) {
            buffer_sprintf(wb, ",\n\t\t\"info\": \"%s\"", sqlite3_column_text(res_alert, 22));
        }
        if (sqlite3_column_type(res_alert, 23) != SQLITE_NULL) {
            buffer_sprintf(wb, ",\n\t\t\"delay\": \"%s\"", sqlite3_column_text(res_alert, 23));
        }
        if (sqlite3_column_type(res_alert, 24) != SQLITE_NULL) {
            buffer_sprintf(wb, ",\n\t\t\"options\": \"%s\"", sqlite3_column_text(res_alert, 24));
        }
        if (sqlite3_column_type(res_alert, 25) != SQLITE_NULL) {
            buffer_sprintf(wb, ",\n\t\t\"repeat\": \"%s\"", sqlite3_column_text(res_alert, 25));
        }
        if (sqlite3_column_type(res_alert, 26) != SQLITE_NULL) {
            buffer_sprintf(wb, ",\n\t\t\"host_labels\": \"%s\"", sqlite3_column_text(res_alert, 26));
        }
        if (sqlite3_column_type(res_alert, 27) != SQLITE_NULL) {
            buffer_sprintf(wb, ",\n\t\t\"p_db_lookup_dimensions\": \"%s\"", sqlite3_column_text(res_alert, 27));
        }
        if (sqlite3_column_type(res_alert, 28) != SQLITE_NULL) {
            buffer_sprintf(wb, ",\n\t\t\"p_db_lookup_method\": \"%s\"", sqlite3_column_text(res_alert, 28));
        }
        if (sqlite3_column_type(res_alert, 29) != SQLITE_NULL) {
            buffer_sprintf(wb, ",\n\t\t\"p_db_lookup_options\": \"");
            buffer_data_options2string(wb, sqlite3_column_int(res_alert, 29));
            buffer_strcat(wb, "\"");
        }
        if (sqlite3_column_type(res_alert, 30) != SQLITE_NULL) {
            buffer_sprintf(wb, ",\n\t\t\"p_db_lookup_after\": %d", sqlite3_column_int(res_alert, 30));
        }
        if (sqlite3_column_type(res_alert, 31) != SQLITE_NULL) {
            buffer_sprintf(wb, ",\n\t\t\"p_db_lookup_before\": %d", sqlite3_column_int(res_alert, 31));
        }
        if (sqlite3_column_type(res_alert, 32) != SQLITE_NULL) {
            buffer_sprintf(wb, ",\n\t\t\"p_update_every\": %d", sqlite3_column_int(res_alert, 32));
        }

        if (unlikely(rc != SQLITE_OK))
            error_report("Failed to reset the prepared statement when reading chart config with hash");
        buffer_strcat(wb, "\n\t}");
    }

    buffer_sprintf(wb, "\n]");

    failed:
    rc = sqlite3_finalize(res_alert);
    if (unlikely(rc != SQLITE_OK))
        error_report("Failed to finalize the prepared statement when reading chart config with hash");

    return;
}

int alert_hash_and_store_config(
    uuid_t hash_id,
    struct alert_config *cfg)
{
    EVP_MD_CTX *evpctx;
    unsigned char hash_value[EVP_MAX_MD_SIZE];
    unsigned int hash_len;
    evpctx = EVP_MD_CTX_create();
    EVP_DigestInit_ex(evpctx, EVP_sha256(), NULL);

    error("HASH asked to hash [%s]", cfg->alarm ? cfg->alarm : cfg->template_key);

    if (cfg->alarm) {
        EVP_DigestUpdate(evpctx, cfg->alarm, strlen(cfg->alarm));
    } else if (cfg->template_key)
        EVP_DigestUpdate(evpctx, cfg->template_key, strlen(cfg->template_key));

    if (cfg->os)
        EVP_DigestUpdate(evpctx, cfg->os, strlen(cfg->os));
    if (cfg->host)
        EVP_DigestUpdate(evpctx, cfg->host, strlen(cfg->host));
    if (cfg->on)
        EVP_DigestUpdate(evpctx, cfg->on, strlen(cfg->on));
    if (cfg->families)
        EVP_DigestUpdate(evpctx, cfg->families, strlen(cfg->families));
    if (cfg->plugin)
        EVP_DigestUpdate(evpctx, cfg->plugin, strlen(cfg->plugin));
    if (cfg->module)
        EVP_DigestUpdate(evpctx, cfg->module, strlen(cfg->module));
    if (cfg->charts)
        EVP_DigestUpdate(evpctx, cfg->charts, strlen(cfg->charts));
    if (cfg->lookup)
        EVP_DigestUpdate(evpctx, cfg->lookup, strlen(cfg->lookup));
    if (cfg->calc)
        EVP_DigestUpdate(evpctx, cfg->calc, strlen(cfg->calc));
    if (cfg->every)
        EVP_DigestUpdate(evpctx, cfg->every, strlen(cfg->every));
    if (cfg->green)
        EVP_DigestUpdate(evpctx, cfg->green, strlen(cfg->green));
    if (cfg->red)
        EVP_DigestUpdate(evpctx, cfg->red, strlen(cfg->red));
    if (cfg->warn)
        EVP_DigestUpdate(evpctx, cfg->warn, strlen(cfg->warn));
    if (cfg->crit)
        EVP_DigestUpdate(evpctx, cfg->crit, strlen(cfg->crit));
    if (cfg->exec)
        EVP_DigestUpdate(evpctx, cfg->exec, strlen(cfg->exec));
    if (cfg->to)
        EVP_DigestUpdate(evpctx, cfg->to, strlen(cfg->to));
    if (cfg->units)
        EVP_DigestUpdate(evpctx, cfg->units, strlen(cfg->units));
    if (cfg->info)
        EVP_DigestUpdate(evpctx, cfg->info, strlen(cfg->info));
    if (cfg->classification)
        EVP_DigestUpdate(evpctx, cfg->classification, strlen(cfg->classification));
    if (cfg->component)
        EVP_DigestUpdate(evpctx, cfg->component, strlen(cfg->component));
    if (cfg->type)
        EVP_DigestUpdate(evpctx, cfg->type, strlen(cfg->type));
    if (cfg->delay)
        EVP_DigestUpdate(evpctx, cfg->delay, strlen(cfg->delay));
    if (cfg->options)
        EVP_DigestUpdate(evpctx, cfg->options, strlen(cfg->options));
    if (cfg->repeat)
        EVP_DigestUpdate(evpctx, cfg->repeat, strlen(cfg->repeat));
    if (cfg->host_labels)
        EVP_DigestUpdate(evpctx, cfg->host_labels, strlen(cfg->host_labels));

    EVP_DigestFinal_ex(evpctx, hash_value, &hash_len);
    EVP_MD_CTX_destroy(evpctx);
    fatal_assert(hash_len > sizeof(uuid_t));

    char uuid_str[36 + 1];
    uuid_unparse_lower(*((uuid_t *)&hash_value), uuid_str);
    uuid_copy(hash_id, *((uuid_t *)&hash_value));

    /* store everything, so it can be recreated when not in memory or just a subset ? */
    (void)sql_store_alert_config_hash( (uuid_t *)&hash_value, cfg);

    return 1;
}
