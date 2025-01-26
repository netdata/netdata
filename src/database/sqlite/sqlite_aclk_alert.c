// SPDX-License-Identifier: GPL-3.0-or-later

#include "sqlite_functions.h"
#include "sqlite_aclk_alert.h"

#include "../../aclk/aclk_alarm_api.h"

extern __thread bool is_health_thread;

#define SQLITE3_COLUMN_STRDUPZ_OR_NULL(res, param)                                                                     \
    ({                                                                                                                 \
        int _param = (param);                                                                                          \
        sqlite3_column_bytes((res), (_param)) ? strdupz((char *)sqlite3_column_text((res), (_param))) : NULL;          \
    })

#define SQL_SELECT_VARIABLE_ALERT_BY_UNIQUE_ID                                                                         \
    "SELECT hld.unique_id FROM health_log hl, alert_hash ah, health_log_detail hld "                                   \
    "WHERE hld.unique_id = @unique_id AND hl.config_hash_id = ah.hash_id AND hld.health_log_id = hl.health_log_id "    \
    "AND hl.host_id = @host_id AND ah.warn IS NULL AND ah.crit IS NULL"

static inline bool is_event_from_alert_variable_config(int64_t unique_id, nd_uuid_t *host_id)
{
    static __thread sqlite3_stmt *compiled_res = NULL;
    sqlite3_stmt *res = NULL;

    if (is_health_thread) {
        if (!compiled_res) {
            if (!PREPARE_COMPILED_STATEMENT(db_meta, SQL_SELECT_VARIABLE_ALERT_BY_UNIQUE_ID, &compiled_res))
                return false;
        }
        res = compiled_res;
    } else {
        if (!PREPARE_STATEMENT(db_meta, SQL_SELECT_VARIABLE_ALERT_BY_UNIQUE_ID, &res))
            return false;
    }

    bool ret = false;

    int param = 0;
    SQLITE_BIND_FAIL(done, sqlite3_bind_int64(res, ++param, unique_id));
    SQLITE_BIND_FAIL(done, sqlite3_bind_blob(res, ++param, host_id, sizeof(*host_id), SQLITE_STATIC));

    param = 0;
    ret = (sqlite3_step_monitored(res) == SQLITE_ROW);

done:
    REPORT_BIND_FAIL(res, param);
    if (is_health_thread)
        SQLITE_RESET(res);
    else
        SQLITE_FINALIZE(res);
    return ret;
}

#define SQL_UPDATE_ALERT_VERSION_TRANSITION                                                                            \
    "UPDATE alert_version SET unique_id = @unique_id WHERE health_log_id = @health_log_id"

static void update_alert_version_transition(int64_t health_log_id, int64_t unique_id)
{
    static __thread sqlite3_stmt *compiled_res = NULL;
    sqlite3_stmt *res = NULL;

    if (is_health_thread) {
        if (!compiled_res) {
            if (!PREPARE_COMPILED_STATEMENT(db_meta, SQL_UPDATE_ALERT_VERSION_TRANSITION, &compiled_res))
                return;
        }
        res = compiled_res;
    } else {
        if (!PREPARE_STATEMENT(db_meta, SQL_UPDATE_ALERT_VERSION_TRANSITION, &res))
            return;
    }

    int param = 0;
    SQLITE_BIND_FAIL(done, sqlite3_bind_int64(res, ++param, unique_id));
    SQLITE_BIND_FAIL(done, sqlite3_bind_int64(res, ++param, health_log_id));

    param = 0;
    int rc = sqlite3_step_monitored(res);
    if (rc != SQLITE_DONE)
        error_report("Failed to update alert_version to latest transition");

done:
    REPORT_BIND_FAIL(res, param);
    if (is_health_thread)
        SQLITE_RESET(res);
    else
        SQLITE_FINALIZE(res);
}

//decide if some events should be sent or not

#define SQL_SELECT_LAST_ALERT_STATUS "SELECT status FROM alert_version WHERE health_log_id = @health_log_id "

static bool cloud_status_matches(int64_t health_log_id, RRDCALC_STATUS status)
{
    static __thread sqlite3_stmt *compiled_res = NULL;
    sqlite3_stmt *res = NULL;

    if (is_health_thread) {
        if (!compiled_res) {
            if (!PREPARE_COMPILED_STATEMENT(db_meta, SQL_SELECT_LAST_ALERT_STATUS, &compiled_res))
                return true;
        }
        res = compiled_res;
    } else {
        if (!PREPARE_STATEMENT(db_meta, SQL_SELECT_LAST_ALERT_STATUS, &res))
            return true;
    }

    bool send = false;

    int param = 0;
    SQLITE_BIND_FAIL(done, sqlite3_bind_int(res, ++param, health_log_id));

    param = 0;
    int rc = sqlite3_step_monitored(res);
    if (likely(rc == SQLITE_ROW)) {
        RRDCALC_STATUS current_status = (RRDCALC_STATUS)sqlite3_column_int(res, 0);
        send = (current_status == status);
    }

done:
    REPORT_BIND_FAIL(res, param);
    if (is_health_thread)
        SQLITE_RESET(res);
    else
        SQLITE_FINALIZE(res);
    return send;
}

#define SQL_QUEUE_ALERT_TO_CLOUD                                                                                       \
    "INSERT INTO aclk_queue (host_id, health_log_id, unique_id, date_created)"                                         \
    " VALUES (@host_id, @health_log_id, @unique_id, UNIXEPOCH())"                                                      \
    " ON CONFLICT(host_id, health_log_id) DO UPDATE SET unique_id=excluded.unique_id, "                                \
    " date_created=excluded.date_created"

//
// Attempt to insert an alert to the submit queue to reach the cloud
//
// The alert will NOT be added in the submit queue if
// - Cloud is already aware of the alert status
// - The transition refers to a variable
//
static int insert_alert_to_submit_queue(RRDHOST *host, int64_t health_log_id, uint32_t unique_id, RRDCALC_STATUS status)
{
    static __thread sqlite3_stmt *compiled_res = NULL;
    sqlite3_stmt *res = NULL;

    if (cloud_status_matches(health_log_id, status)) {
        update_alert_version_transition(health_log_id, unique_id);
        return 1;
    }

    if (is_event_from_alert_variable_config(unique_id, &host->host_id.uuid))
        return 2;

    if (is_health_thread) {
        if (!compiled_res) {
            if (!PREPARE_COMPILED_STATEMENT(db_meta, SQL_QUEUE_ALERT_TO_CLOUD, &compiled_res))
                return -1;
        }
        res = compiled_res;
    } else {
        if (!PREPARE_STATEMENT(db_meta, SQL_QUEUE_ALERT_TO_CLOUD, &res))
            return -1;
    }

    int param = 0;
    SQLITE_BIND_FAIL(done, sqlite3_bind_blob(res, ++param, &host->host_id.uuid, sizeof(host->host_id.uuid), SQLITE_STATIC));
    SQLITE_BIND_FAIL(done, sqlite3_bind_int64(res, ++param, health_log_id));
    SQLITE_BIND_FAIL(done, sqlite3_bind_int64(res, ++param, (int64_t) unique_id));

    param = 0;
    int rc = sqlite3_step_monitored(res);
    if (unlikely(rc != SQLITE_DONE))
        error_report("Failed to insert alert in the submit queue %"PRIu32", rc = %d", unique_id, rc);

done:
    REPORT_BIND_FAIL(res, param);
    if (is_health_thread)
        SQLITE_RESET(res);
    else
        SQLITE_FINALIZE(res);
    return 0;
}

#define SQL_DELETE_QUEUE_ALERT_TO_CLOUD                                                                                \
    "DELETE FROM aclk_queue WHERE host_id = @host_id AND sequence_id BETWEEN @seq1 AND @seq2"

//
// Delete a range of alerts from the submit queue (after being sent to the the cloud)
//
static int delete_alert_from_submit_queue(RRDHOST *host, int64_t first_seq_id, int64_t last_seq_id)
{
    sqlite3_stmt *res = NULL;

    if (!PREPARE_STATEMENT(db_meta, SQL_DELETE_QUEUE_ALERT_TO_CLOUD, &res))
        return -1;

    int param = 0;
    SQLITE_BIND_FAIL(done, sqlite3_bind_blob(res, ++param, &host->host_id.uuid, sizeof(host->host_id.uuid), SQLITE_STATIC));
    SQLITE_BIND_FAIL(done, sqlite3_bind_int64(res, ++param, first_seq_id));
    SQLITE_BIND_FAIL(done, sqlite3_bind_int64(res, ++param, last_seq_id));

    param = 0;
    int rc = sqlite3_step_monitored(res);
    if (rc != SQLITE_DONE)
        error_report("Failed to delete submitted to ACLK");

done:
    REPORT_BIND_FAIL(res, param);
    SQLITE_FINALIZE(res);
    return 0;
}

int rrdcalc_status_to_proto_enum(RRDCALC_STATUS status)
{

    switch(status) {
        case RRDCALC_STATUS_REMOVED:
            return ALARM_STATUS_REMOVED;

        case RRDCALC_STATUS_UNDEFINED:
            return ALARM_STATUS_NOT_A_NUMBER;

        case RRDCALC_STATUS_CLEAR:
            return ALARM_STATUS_CLEAR;

        case RRDCALC_STATUS_WARNING:
            return ALARM_STATUS_WARNING;

        case RRDCALC_STATUS_CRITICAL:
            return ALARM_STATUS_CRITICAL;

        default:
            return ALARM_STATUS_UNKNOWN;
    }
}

static inline char *sqlite3_uuid_unparse_strdupz(sqlite3_stmt *res, int iCol) {
    char uuid_str[UUID_STR_LEN];

    if(sqlite3_column_type(res, iCol) == SQLITE_NULL)
        uuid_str[0] = '\0';
    else
        uuid_unparse_lower(*((nd_uuid_t *) sqlite3_column_blob(res, iCol)), uuid_str);

    return strdupz(uuid_str);
}

static inline char *sqlite3_text_strdupz_empty(sqlite3_stmt *res, int iCol) {
    char *ret;

    if(sqlite3_column_type(res, iCol) == SQLITE_NULL)
        ret = "";
    else
        ret = (char *)sqlite3_column_text(res, iCol);

    return strdupz(ret);
}

#define SQL_UPDATE_ALERT_VERSION                                                                                       \
    "INSERT INTO alert_version (health_log_id, unique_id, status, version, date_submitted)"                            \
    " VALUES (@health_log_id, @unique_id, @status, @version, UNIXEPOCH())"                                             \
    " ON CONFLICT(health_log_id) DO UPDATE SET status = excluded.status, version = excluded.version, "                 \
    " unique_id=excluded.unique_id, date_submitted=excluded.date_submitted"

//
// Store a new alert transition along with the version after sending to the cloud
//   - Update an existing alert with the updated version, status, transition and date submitted
//
static void sql_update_alert_version(
    int64_t health_log_id,
    int64_t unique_id,
    RRDCALC_STATUS status,
    uint64_t version,
    sqlite3_stmt **res)
{
    if (!*res) {
        if (!PREPARE_STATEMENT(db_meta, SQL_UPDATE_ALERT_VERSION, res))
            return;
    }

    int param = 0;
    SQLITE_BIND_FAIL(done, sqlite3_bind_int64(*res, ++param, health_log_id));
    SQLITE_BIND_FAIL(done, sqlite3_bind_int64(*res, ++param, unique_id));
    SQLITE_BIND_FAIL(done, sqlite3_bind_int(*res, ++param, status));
    SQLITE_BIND_FAIL(done, sqlite3_bind_int64(*res, ++param, version));

    param = 0;
    int rc = sqlite3_step_monitored(*res);
    if (rc != SQLITE_DONE)
        error_report("Failed to execute sql_update_alert_version");

done:
    REPORT_BIND_FAIL(*res, param);
    SQLITE_RESET(*res);
}

#define SQL_SELECT_ALERT_TO_DUMMY                                                                                      \
    "SELECT aq.sequence_id, hld.unique_id, hld.when_key, hld.new_status, hld.health_log_id"                            \
    " FROM health_log hl, aclk_queue aq, alert_hash ah, health_log_detail hld"                                         \
    " WHERE hld.unique_id = aq.unique_id AND hl.config_hash_id = ah.hash_id"                                           \
    " AND hl.host_id = @host_id AND aq.host_id = hl.host_id AND hl.health_log_id = hld.health_log_id"                  \
    " ORDER BY aq.sequence_id ASC"

//
// Check all queued alerts for a host and commit them as if they have been send to the cloud
// this will produce new versions as needed. We need this because we are about to send a
// a snapshot so we can include the latest transition.
//
static void commit_alert_events(RRDHOST *host)
{
    sqlite3_stmt *res = NULL;

    if (!PREPARE_STATEMENT(db_meta, SQL_SELECT_ALERT_TO_DUMMY, &res))
        return;

    int param = 0;
    SQLITE_BIND_FAIL(done, sqlite3_bind_blob(res, ++param, &host->host_id.uuid, sizeof(host->host_id.uuid), SQLITE_STATIC));

    int64_t first_sequence_id = 0;
    int64_t last_sequence_id = 0;

    sqlite3_stmt *res_version = NULL;
    param = 0;
    while (sqlite3_step_monitored(res) == SQLITE_ROW) {

        last_sequence_id = sqlite3_column_int64(res, 0);
        if (first_sequence_id == 0)
            first_sequence_id = last_sequence_id;

        int64_t unique_id = sqlite3_column_int(res, 1);
        int64_t version = sqlite3_column_int64(res, 2);
        RRDCALC_STATUS status = (RRDCALC_STATUS)sqlite3_column_int(res, 3);
        int64_t health_log_id = sqlite3_column_int64(res, 4);

        // Prepare the statement on the first time (res_version) then reuse it
        // finalize when we are done
        sql_update_alert_version(health_log_id, unique_id, status, version, &res_version);
    }

    if (first_sequence_id)
        delete_alert_from_submit_queue(host, first_sequence_id, last_sequence_id);

done:
    REPORT_BIND_FAIL(res, param);
    SQLITE_FINALIZE(res);
}

typedef enum {
    SEQUENCE_ID,
    UNIQUE_ID,
    ALARM_ID,
    CONFIG_HASH_ID,
    UPDATED_BY_ID,
    WHEN_KEY,
    DURATION,
    NON_CLEAR_DURATION,
    FLAGS,
    EXEC_RUN_TIMESTAMP,
    DELAY_UP_TO_TIMESTAMP,
    NAME,
    CHART,
    EXEC,
    RECIPIENT,
    SOURCE,
    UNITS,
    INFO,
    EXEC_CODE,
    NEW_STATUS,
    OLD_STATUS,
    DELAY,
    NEW_VALUE,
    OLD_VALUE,
    LAST_REPEAT,
    CHART_CONTEXT,
    TRANSITION_ID,
    ALARM_EVENT_ID,
    CHART_NAME,
    SUMMARY,
    HEALTH_LOG_ID,
    VERSION
} HealthLogDetails;

void health_alarm_log_populate(
    struct alarm_log_entry *alarm_log,
    sqlite3_stmt *res,
    RRDHOST *host,
    RRDCALC_STATUS *status)
{
    char old_value_string[100 + 1];
    char new_value_string[100 + 1];

    RRDCALC_STATUS current_status = (RRDCALC_STATUS)sqlite3_column_int(res, NEW_STATUS);
    if (status)
        *status = current_status;

    char *source = (char *) sqlite3_column_text(res, SOURCE);
    alarm_log->command = source ? health_edit_command_from_source(source) : strdupz("UNKNOWN=0=UNKNOWN");

    alarm_log->chart = strdupz((char *) sqlite3_column_text(res, CHART));
    alarm_log->name = strdupz((char *) sqlite3_column_text(res, NAME));

    alarm_log->when = sqlite3_column_int64(res, WHEN_KEY);

    alarm_log->config_hash = sqlite3_uuid_unparse_strdupz(res, CONFIG_HASH_ID);

    alarm_log->utc_offset = host->utc_offset;
    alarm_log->timezone = strdupz(rrdhost_abbrev_timezone(host));
    alarm_log->exec_path =  sqlite3_column_bytes(res, EXEC) ?
                               strdupz((char *)sqlite3_column_text(res, EXEC)) :
                               strdupz((char *)string2str(host->health.default_exec));

    alarm_log->conf_source = source ? strdupz(source) : strdupz("");

    time_t duration = sqlite3_column_int64(res, DURATION);
    alarm_log->duration =  (duration > 0) ? duration : 0;

    alarm_log->non_clear_duration = sqlite3_column_int64(res, NON_CLEAR_DURATION);

    alarm_log->status = rrdcalc_status_to_proto_enum(current_status);
    alarm_log->old_status = rrdcalc_status_to_proto_enum((RRDCALC_STATUS)sqlite3_column_int64(res, OLD_STATUS));
    alarm_log->delay = sqlite3_column_int64(res, DELAY);
    alarm_log->delay_up_to_timestamp = sqlite3_column_int64(res, DELAY_UP_TO_TIMESTAMP);
    alarm_log->last_repeat = sqlite3_column_int64(res, LAST_REPEAT);

    uint64_t flags = sqlite3_column_int64(res, FLAGS);
    char *recipient =  (char *) sqlite3_column_text(res, RECIPIENT);
    alarm_log->silenced =
        ((flags & HEALTH_ENTRY_FLAG_SILENCED) || (recipient && !strncmp(recipient, "silent", 6))) ? 1 : 0;

    double value = sqlite3_column_double(res, NEW_VALUE);
    double old_value = sqlite3_column_double(res, OLD_VALUE);

    alarm_log->value_string =
        sqlite3_column_type(res, NEW_VALUE) == SQLITE_NULL ?
            strdupz((char *)"-") :
            strdupz((char *)format_value_and_unit(
                new_value_string, 100, value, (char *)sqlite3_column_text(res, UNITS), -1));

    alarm_log->old_value_string =
        sqlite3_column_type(res, OLD_VALUE) == SQLITE_NULL ?
            strdupz((char *)"-") :
            strdupz((char *)format_value_and_unit(
                old_value_string, 100, old_value, (char *)sqlite3_column_text(res, UNITS), -1));

    alarm_log->value = (!isnan(value)) ? (NETDATA_DOUBLE)value : 0;
    alarm_log->old_value = (!isnan(old_value)) ? (NETDATA_DOUBLE)old_value : 0;

    alarm_log->updated = (flags & HEALTH_ENTRY_FLAG_UPDATED) ? 1 : 0;
    alarm_log->rendered_info = sqlite3_text_strdupz_empty(res, INFO);
    alarm_log->chart_context = sqlite3_text_strdupz_empty(res, CHART_CONTEXT);
    alarm_log->chart_name = sqlite3_text_strdupz_empty(res, CHART_NAME);

    alarm_log->transition_id = sqlite3_uuid_unparse_strdupz(res, TRANSITION_ID);
    alarm_log->event_id = sqlite3_column_int64(res, ALARM_EVENT_ID);
    alarm_log->version = sqlite3_column_int64(res, VERSION);

    alarm_log->summary = sqlite3_text_strdupz_empty(res, SUMMARY);

    alarm_log->health_log_id = sqlite3_column_int64(res, HEALTH_LOG_ID);
    alarm_log->unique_id = sqlite3_column_int64(res, UNIQUE_ID);
    alarm_log->alarm_id = sqlite3_column_int64(res, ALARM_ID);
    alarm_log->sequence_id = sqlite3_column_int64(res, SEQUENCE_ID);
}

#define SQL_SELECT_ALERT_TO_PUSH                                                                                       \
    "SELECT aq.sequence_id, hld.unique_id, hld.alarm_id, hl.config_hash_id, hld.updated_by_id, hld.when_key,"          \
    " hld.duration, hld.non_clear_duration, hld.flags, hld.exec_run_timestamp, hld.delay_up_to_timestamp, hl.name,"    \
    " hl.chart, hl.exec, hl.recipient, ah.source, hl.units, hld.info, hld.exec_code, hld.new_status,"                  \
    " hld.old_status, hld.delay, hld.new_value, hld.old_value, hld.last_repeat, hl.chart_context, hld.transition_id,"  \
    " hld.alarm_event_id, hl.chart_name, hld.summary, hld.health_log_id, hld.when_key"                                 \
    " FROM health_log hl, aclk_queue aq, alert_hash ah, health_log_detail hld"                                         \
    " WHERE hld.unique_id = aq.unique_id AND hl.config_hash_id = ah.hash_id"                                           \
    " AND hl.host_id = @host_id AND aq.host_id = hl.host_id AND hl.health_log_id = hld.health_log_id"                  \
    " ORDER BY aq.sequence_id ASC LIMIT "ACLK_MAX_ALERT_UPDATES

static void aclk_push_alert_event(RRDHOST *host, sqlite3_stmt **res, sqlite3_stmt **res_version)
{
    CLAIM_ID claim_id = claim_id_get();

    if (!claim_id_is_set(claim_id) || UUIDiszero(host->node_id))
        return;

    if (!*res) {
        if (!PREPARE_STATEMENT(db_meta, SQL_SELECT_ALERT_TO_PUSH, res))
            return;
    }

    int param = 0;
    SQLITE_BIND_FAIL(done, sqlite3_bind_blob(*res, ++param, &host->host_id.uuid, sizeof(host->host_id.uuid), SQLITE_STATIC));

    char node_id_str[UUID_STR_LEN];
    uuid_unparse_lower(host->node_id.uuid, node_id_str);

    struct alarm_log_entry alarm_log;
    alarm_log.node_id = node_id_str;
    alarm_log.claim_id = claim_id.str;

    int64_t first_id = 0;
    int64_t last_id = 0;

    param = 0;
    RRDCALC_STATUS status;
    struct aclk_sync_cfg_t *wc = host->aclk_config;
    while (sqlite3_step_monitored(*res) == SQLITE_ROW) {
        health_alarm_log_populate(&alarm_log, *res, host, &status);
        aclk_send_alarm_log_entry(&alarm_log);
        wc->alert_count++;

        last_id = alarm_log.sequence_id;
        if (first_id == 0)
            first_id = last_id;

        // The statement to set the version will be compiled once and reset when done
        // out caller will finalize the statement to release resources
        sql_update_alert_version(alarm_log.health_log_id, alarm_log.unique_id, status, alarm_log.version, res_version);

        destroy_alarm_log_entry(&alarm_log);
    }

    if (first_id) {
        nd_log(
            NDLS_ACCESS,
            NDLP_DEBUG,
            "ACLK RES [%s (%s)]: ALERTS SENT from %lld - %lld",
            node_id_str,
            rrdhost_hostname(host),
            (long long)first_id,
            (long long)last_id);

        delete_alert_from_submit_queue(host, first_id, last_id);
        // Mark to do one more check
        rrdhost_flag_set(host, RRDHOST_FLAG_ACLK_STREAM_ALERTS);
    }

done:
    REPORT_BIND_FAIL(*res, param);
    SQLITE_RESET(*res);
}

#define SQL_DELETE_PROCESSED_ROWS "DELETE FROM alert_queue WHERE host_id = @host_id AND rowid = @row"

static void delete_alert_from_pending_queue(RRDHOST *host, int64_t row)
{
    static __thread sqlite3_stmt *compiled_res = NULL;
    sqlite3_stmt *res = NULL;

    if (is_health_thread) {
        if (!compiled_res) {
            if (!PREPARE_COMPILED_STATEMENT(db_meta, SQL_DELETE_PROCESSED_ROWS, &compiled_res))
                return;
        }
        res = compiled_res;
    } else {
        if (!PREPARE_STATEMENT(db_meta, SQL_DELETE_PROCESSED_ROWS, &res))
            return;
    }

    int param = 0;
    SQLITE_BIND_FAIL(done, sqlite3_bind_blob(res, ++param, &host->host_id.uuid, sizeof(host->host_id.uuid), SQLITE_STATIC));
    SQLITE_BIND_FAIL(done, sqlite3_bind_int64(res, ++param, row));

    param = 0;
    int rc = sqlite3_step_monitored(res);
    if (rc != SQLITE_DONE)
        error_report("Failed to delete processed rows, rc = %d", rc);

done:
    REPORT_BIND_FAIL(res, param);
    if (is_health_thread)
        SQLITE_RESET(res);
    else
        SQLITE_FINALIZE(res);
}

#define SQL_REBUILD_HOST_ALERT_VERSION_TABLE                                                                           \
    "INSERT INTO alert_version (health_log_id, unique_id, status, version, date_submitted) "                           \
    " SELECT hl.health_log_id, hld.unique_id, hld.new_status, hld.when_key, UNIXEPOCH() "                              \
    " FROM health_log hl, health_log_detail hld WHERE "                                                                \
    "  hl.host_id = @host_id AND hld.health_log_id = hl.health_log_id AND hld.transition_id = hl.last_transition_id"

#define SQL_DELETE_HOST_ALERT_VERSION_TABLE                                                                            \
    "DELETE FROM alert_version WHERE health_log_id IN (SELECT health_log_id FROM health_log WHERE host_id = @host_id)"

void rebuild_host_alert_version_table(RRDHOST *host)
{
    sqlite3_stmt *res = NULL;

    if (!PREPARE_STATEMENT(db_meta, SQL_DELETE_HOST_ALERT_VERSION_TABLE, &res))
        return;

    int param = 0;
    SQLITE_BIND_FAIL(done, sqlite3_bind_blob(res, ++param, &host->host_id.uuid, sizeof(host->host_id.uuid), SQLITE_STATIC));

    param = 0;
    int rc = execute_insert(res);
    if (rc != SQLITE_DONE) {
        netdata_log_error("Failed to delete the host alert version table");
        goto done;
    }

    SQLITE_FINALIZE(res);
    if (!PREPARE_STATEMENT(db_meta, SQL_REBUILD_HOST_ALERT_VERSION_TABLE, &res))
        return;

    param = 0;
    SQLITE_BIND_FAIL(done, sqlite3_bind_blob(res, ++param, &host->host_id.uuid, sizeof(host->host_id.uuid), SQLITE_STATIC));

    param = 0;
    rc = execute_insert(res);
    if (rc != SQLITE_DONE)
        netdata_log_error("Failed to rebuild the host alert version table");

done:
    REPORT_BIND_FAIL(res, param);
    SQLITE_FINALIZE(res);
}

#define SQL_PROCESS_ALERT_PENDING_QUEUE                                                                                \
    "SELECT health_log_id, unique_id, status, rowid"                                                                   \
    " FROM alert_queue WHERE host_id = @host_id AND date_scheduled <= UNIXEPOCH() ORDER BY rowid ASC"

bool process_alert_pending_queue(RRDHOST *host)
{
    static __thread sqlite3_stmt *compiled_res = NULL;
    sqlite3_stmt *res = NULL;

    if (is_health_thread) {
        if (!compiled_res) {
            if (!PREPARE_COMPILED_STATEMENT(db_meta, SQL_PROCESS_ALERT_PENDING_QUEUE, &compiled_res))
                return false;
        }
        res = compiled_res;
    } else {
        if (!PREPARE_STATEMENT(db_meta, SQL_PROCESS_ALERT_PENDING_QUEUE, &res))
            return false;
    }

    int param = 0;
    int added =0, count = 0;
    SQLITE_BIND_FAIL(done, sqlite3_bind_blob(res, ++param, &host->host_id.uuid, sizeof(host->host_id.uuid), SQLITE_STATIC));

    param = 0;
    while (sqlite3_step_monitored(res) == SQLITE_ROW) {

        int64_t health_log_id = sqlite3_column_int64(res, 0);
        uint32_t unique_id = sqlite3_column_int64(res, 1);
        RRDCALC_STATUS new_status = sqlite3_column_int(res, 2);
        int64_t row = sqlite3_column_int64(res, 3);

        if (host->aclk_config) {
            int ret = insert_alert_to_submit_queue(host, health_log_id, unique_id, new_status);
            if (ret == 0)
                added++;
        }

        delete_alert_from_pending_queue(host, row);

        count++;
    }

    if(count)
        nd_log(NDLS_ACCESS, NDLP_NOTICE, "ACLK STA [%s (N/A)]: Processed %d entries, queued %d", rrdhost_hostname(host), count, added);
done:
    REPORT_BIND_FAIL(res, param);
    if (is_health_thread)
        SQLITE_RESET(res);
    else
        SQLITE_FINALIZE(res);
    return added > 0;
}

void aclk_push_alert_events_for_all_hosts(void)
{
    RRDHOST *host;

    sqlite3_stmt *res = NULL;               // used to scan pending alerts to send
    sqlite3_stmt *res_version = NULL;       // used to update the alert version
    dfe_start_reentrant(rrdhost_root_index, host) {
        if (!rrdhost_flag_check(host, RRDHOST_FLAG_ACLK_STREAM_ALERTS) ||
            rrdhost_flag_check(host, RRDHOST_FLAG_PENDING_CONTEXT_LOAD))
            continue;

        rrdhost_flag_clear(host, RRDHOST_FLAG_ACLK_STREAM_ALERTS);

        struct aclk_sync_cfg_t *wc = host->aclk_config;
        if (!wc || false == wc->stream_alerts || rrdhost_flag_check(host, RRDHOST_FLAG_ARCHIVED)) {
            (void)process_alert_pending_queue(host);
            commit_alert_events(host);
            continue;
        }

        if (wc->send_snapshot) {
            rrdhost_flag_set(host, RRDHOST_FLAG_ACLK_STREAM_ALERTS);
            if (wc->send_snapshot == 1)
                continue;
            (void)process_alert_pending_queue(host);
            commit_alert_events(host);
            rebuild_host_alert_version_table(host);
            send_alert_snapshot_to_cloud(host);
            wc->snapshot_count++;
            wc->send_snapshot = 0;
        }
        else
            aclk_push_alert_event(host, &res, &res_version);
    }
    dfe_done(host);
    SQLITE_FINALIZE(res);
    SQLITE_FINALIZE(res_version);
}

void aclk_send_alert_configuration(char *config_hash)
{
    if (unlikely(!config_hash))
        return;

    struct aclk_sync_cfg_t *wc = localhost->aclk_config;

    if (unlikely(!wc))
        return;

    nd_log(NDLS_ACCESS, NDLP_DEBUG,
        "ACLK REQ [%s (%s)]: Request to send alert config %s.",
        wc->node_id,
        wc->host ? rrdhost_hostname(wc->host) : "N/A",
        config_hash);

    aclk_push_alert_config(wc->node_id, config_hash);
}

#define SQL_SELECT_ALERT_CONFIG                                                                                        \
    "SELECT alarm, template, on_key, class, type, component, os, hosts, plugin,"                                       \
    "module, charts, lookup, every, units, green, red, calc, warn, crit, to_key, exec, delay, repeat, info,"           \
    "options, host_labels, p_db_lookup_dimensions, p_db_lookup_method, p_db_lookup_options, p_db_lookup_after,"        \
    "p_db_lookup_before, p_update_every, chart_labels, summary FROM alert_hash WHERE hash_id = @hash_id"

void aclk_push_alert_config_event(char *node_id __maybe_unused, char *config_hash __maybe_unused)
{
    sqlite3_stmt *res = NULL;
    struct aclk_sync_cfg_t *wc;

    RRDHOST *host = find_host_by_node_id(node_id);

    if (unlikely(!host || !(wc = host->aclk_config))) {
        freez(config_hash);
        freez(node_id);
        return;
    }

    if (!PREPARE_STATEMENT(db_meta, SQL_SELECT_ALERT_CONFIG, &res))
        return;

    nd_uuid_t hash_uuid;
    if (uuid_parse(config_hash, hash_uuid))
        return;

    int param = 0;
    SQLITE_BIND_FAIL(done, sqlite3_bind_blob(res, ++param, &hash_uuid , sizeof(hash_uuid), SQLITE_STATIC));

    struct aclk_alarm_configuration alarm_config;
    struct provide_alarm_configuration p_alarm_config;
    p_alarm_config.cfg_hash = NULL;

    param = 0;
    if (sqlite3_step_monitored(res) == SQLITE_ROW) {
        alarm_config.alarm = SQLITE3_COLUMN_STRDUPZ_OR_NULL(res, param++);
        alarm_config.tmpl = SQLITE3_COLUMN_STRDUPZ_OR_NULL(res, param++);
        alarm_config.on_chart = SQLITE3_COLUMN_STRDUPZ_OR_NULL(res, param++);
        alarm_config.classification = SQLITE3_COLUMN_STRDUPZ_OR_NULL(res, param++);
        alarm_config.type = SQLITE3_COLUMN_STRDUPZ_OR_NULL(res, param++);
        alarm_config.component = SQLITE3_COLUMN_STRDUPZ_OR_NULL(res, param++);
        alarm_config.os = SQLITE3_COLUMN_STRDUPZ_OR_NULL(res, param++);
        alarm_config.hosts = SQLITE3_COLUMN_STRDUPZ_OR_NULL(res, param++);
        alarm_config.plugin = SQLITE3_COLUMN_STRDUPZ_OR_NULL(res, param++);
        alarm_config.module = SQLITE3_COLUMN_STRDUPZ_OR_NULL(res, param++);
        alarm_config.charts = SQLITE3_COLUMN_STRDUPZ_OR_NULL(res, param++);
        alarm_config.lookup = SQLITE3_COLUMN_STRDUPZ_OR_NULL(res, param++);
        alarm_config.every = SQLITE3_COLUMN_STRDUPZ_OR_NULL(res, param++);
        alarm_config.units = SQLITE3_COLUMN_STRDUPZ_OR_NULL(res, param++);
        alarm_config.green = SQLITE3_COLUMN_STRDUPZ_OR_NULL(res, param++);
        alarm_config.red = SQLITE3_COLUMN_STRDUPZ_OR_NULL(res, param++);
        alarm_config.calculation_expr = SQLITE3_COLUMN_STRDUPZ_OR_NULL(res, param++);
        alarm_config.warning_expr = SQLITE3_COLUMN_STRDUPZ_OR_NULL(res, param++);
        alarm_config.critical_expr = SQLITE3_COLUMN_STRDUPZ_OR_NULL(res, param++);
        alarm_config.recipient = SQLITE3_COLUMN_STRDUPZ_OR_NULL(res, param++);
        alarm_config.exec = SQLITE3_COLUMN_STRDUPZ_OR_NULL(res, param++);
        alarm_config.delay = SQLITE3_COLUMN_STRDUPZ_OR_NULL(res, param++);
        alarm_config.repeat = SQLITE3_COLUMN_STRDUPZ_OR_NULL(res, param++);
        alarm_config.info = SQLITE3_COLUMN_STRDUPZ_OR_NULL(res, param++);
        alarm_config.options = SQLITE3_COLUMN_STRDUPZ_OR_NULL(res, param++);
        alarm_config.host_labels = SQLITE3_COLUMN_STRDUPZ_OR_NULL(res, param++);  // Current param 25

        alarm_config.p_db_lookup_dimensions = NULL;
        alarm_config.p_db_lookup_method = NULL;
        alarm_config.p_db_lookup_options = NULL;
        alarm_config.p_db_lookup_after = 0;
        alarm_config.p_db_lookup_before = 0;

        if (sqlite3_column_bytes(res, 29) > 0) {

            alarm_config.p_db_lookup_dimensions = SQLITE3_COLUMN_STRDUPZ_OR_NULL(res, param++);  // Current param 26
            alarm_config.p_db_lookup_method = SQLITE3_COLUMN_STRDUPZ_OR_NULL(res, param++);      // Current param 27
            if (param != 28)
                netdata_log_error("aclk_push_alert_config_event: Unexpected param number %d", param);

            BUFFER *tmp_buf = buffer_create(1024, &netdata_buffers_statistics.buffers_sqlite);
            rrdr_options_to_buffer(tmp_buf, sqlite3_column_int(res, 28));
            alarm_config.p_db_lookup_options = strdupz((char *)buffer_tostring(tmp_buf));
            buffer_free(tmp_buf);

            alarm_config.p_db_lookup_after = sqlite3_column_int(res, 29);
            alarm_config.p_db_lookup_before = sqlite3_column_int(res, 30);
        }

        alarm_config.p_update_every = sqlite3_column_int(res, 31);

        alarm_config.chart_labels = SQLITE3_COLUMN_STRDUPZ_OR_NULL(res, 32);
        alarm_config.summary = SQLITE3_COLUMN_STRDUPZ_OR_NULL(res, 33);

        p_alarm_config.cfg_hash = strdupz((char *) config_hash);
        p_alarm_config.cfg = alarm_config;
    }

    param = 0;

    if (likely(p_alarm_config.cfg_hash)) {
        nd_log(NDLS_ACCESS, NDLP_DEBUG, "ACLK RES [%s (%s)]: Sent alert config %s.", wc->node_id, wc->host ? rrdhost_hostname(wc->host) : "N/A", config_hash);
        aclk_send_provide_alarm_cfg(&p_alarm_config);
        freez(p_alarm_config.cfg_hash);
        destroy_aclk_alarm_configuration(&alarm_config);
    }
    else
        nd_log(NDLS_ACCESS, NDLP_WARNING, "ACLK STA [%s (%s)]: Alert config for %s not found.", wc->node_id, wc->host ? rrdhost_hostname(wc->host) : "N/A", config_hash);

done:
    REPORT_BIND_FAIL(res, param);
    SQLITE_FINALIZE(res);
    freez(config_hash);
    freez(node_id);
}

#define SQL_ALERT_VERSION_CALC                                                                                         \
    "SELECT SUM(version) FROM health_log hl, alert_version av"                                                         \
    " WHERE hl.host_id = @host_uuid AND hl.health_log_id = av.health_log_id AND av.status <> -2"

uint64_t calculate_node_alert_version(RRDHOST *host)
{
    sqlite3_stmt *res = NULL;

    if (!PREPARE_STATEMENT(db_meta, SQL_ALERT_VERSION_CALC, &res))
        return 0;

    uint64_t version = 0;
    int param = 0;
    SQLITE_BIND_FAIL(done, sqlite3_bind_blob(res, ++param, &host->host_id.uuid, sizeof(host->host_id.uuid), SQLITE_STATIC));

    param = 0;
    while (sqlite3_step_monitored(res) == SQLITE_ROW) {
        version = (uint64_t)sqlite3_column_int64(res, 0);
    }

done:
    REPORT_BIND_FAIL(res, param);
    SQLITE_FINALIZE(res);
    return version;
}

static void schedule_alert_snapshot_if_needed(struct aclk_sync_cfg_t *wc, uint64_t cloud_version)
{
    if (cloud_version == 1) {
        nd_log(
            NDLS_ACCESS,
            NDLP_NOTICE,
            "Cloud requested to skip alert version verification for host \"%s\", node \"%s\"",
            rrdhost_hostname(wc->host),
            wc->node_id);
        return;
    }

    uint64_t local_version = calculate_node_alert_version(wc->host);
    if (local_version != cloud_version) {
        nd_log(
            NDLS_ACCESS,
            NDLP_NOTICE,
            "Scheduling alert snapshot for host \"%s\", node \"%s\" (version: cloud %llu, local %llu)",
            rrdhost_hostname(wc->host),
            wc->node_id,
            (long long unsigned)cloud_version,
            (long long unsigned)local_version);

        wc->send_snapshot = 1;
        rrdhost_flag_set(wc->host, RRDHOST_FLAG_ACLK_STREAM_ALERTS);
    }
    else
        nd_log(
            NDLS_ACCESS,
            NDLP_DEBUG,
            "Alert check on \"%s\", node \"%s\" (version: cloud %llu, local %llu)",
            rrdhost_hostname(wc->host),
            wc->node_id,
            (unsigned long long)cloud_version,
            (unsigned long long)local_version);
    wc->checkpoint_count++;
}

#define SQL_COUNT_SNAPSHOT_ENTRIES                                                                                     \
    "SELECT COUNT(1) FROM alert_version av, health_log hl "                                                            \
    "WHERE hl.host_id = @host_id AND hl.health_log_id = av.health_log_id AND av.status <> -2"

static int calculate_alert_snapshot_entries(nd_uuid_t *host_uuid)
{
    int count = 0;

    sqlite3_stmt *res = NULL;

    if (!PREPARE_STATEMENT(db_meta, SQL_COUNT_SNAPSHOT_ENTRIES, &res))
        return 0;

    int param = 0;
    SQLITE_BIND_FAIL(done, sqlite3_bind_blob(res, ++param, host_uuid, sizeof(*host_uuid), SQLITE_STATIC));

    param = 0;
    int rc = sqlite3_step_monitored(res);
    if (rc == SQLITE_ROW)
        count = sqlite3_column_int(res, 0);
    else
        error_report("Failed to select snapshot count");

done:
    REPORT_BIND_FAIL(res, param);
    SQLITE_FINALIZE(res);

    return count;
}

#define SQL_GET_SNAPSHOT_ENTRIES                                                                                       \
    " SELECT 0, hld.unique_id, hld.alarm_id, hl.config_hash_id, hld.updated_by_id, hld.when_key, "                     \
    " hld.duration, hld.non_clear_duration, hld.flags, hld.exec_run_timestamp, hld.delay_up_to_timestamp, hl.name,  "  \
    " hl.chart, hl.exec, hl.recipient, ah.source, hl.units, hld.info, hld.exec_code, hld.new_status,  "                \
    " hld.old_status, hld.delay, hld.new_value, hld.old_value, hld.last_repeat, hl.chart_context, hld.transition_id, " \
    " hld.alarm_event_id, hl.chart_name, hld.summary, hld.health_log_id, av.version "                                  \
    " FROM health_log hl, alert_hash ah, health_log_detail hld, alert_version av "                                     \
    " WHERE hl.config_hash_id = ah.hash_id"                                                                            \
    " AND hl.host_id = @host_id AND hl.health_log_id = hld.health_log_id "                                             \
    " AND hld.health_log_id = av.health_log_id AND av.unique_id = hld.unique_id AND av.status <> -2"

#define ALARM_EVENTS_PER_CHUNK 1000
void send_alert_snapshot_to_cloud(RRDHOST *host __maybe_unused)
{
    struct aclk_sync_cfg_t *wc = host->aclk_config;

    if (unlikely(!host)) {
        nd_log(NDLS_ACCESS, NDLP_WARNING, "AC [%s (N/A)]: Node id not found", wc->node_id);
        return;
    }

    CLAIM_ID claim_id = claim_id_get();
    if (unlikely(!claim_id_is_set(claim_id)))
        return;

    // Check the database for this node to see how many alerts we will need to put in the snapshot
    int cnt = calculate_alert_snapshot_entries(&host->host_id.uuid);
    if (!cnt)
        return;

    sqlite3_stmt *res = NULL;
    if (!PREPARE_STATEMENT(db_meta, SQL_GET_SNAPSHOT_ENTRIES, &res))
        return;

    int param = 0;
    SQLITE_BIND_FAIL(done, sqlite3_bind_blob(res, ++param, &host->host_id.uuid, sizeof(host->host_id.uuid), SQLITE_STATIC));

    nd_uuid_t local_snapshot_uuid;
    char snapshot_uuid_str[UUID_STR_LEN];
    uuid_generate_random(local_snapshot_uuid);
    uuid_unparse_lower(local_snapshot_uuid, snapshot_uuid_str);
    char *snapshot_uuid = &snapshot_uuid_str[0];

    nd_log(NDLS_ACCESS, NDLP_DEBUG,
        "ACLK REQ [%s (%s)]: Sending %d alerts snapshot, snapshot_uuid %s", wc->node_id, rrdhost_hostname(host),
        cnt, snapshot_uuid);

    uint32_t chunks;
    chunks = (cnt / ALARM_EVENTS_PER_CHUNK) + (cnt % ALARM_EVENTS_PER_CHUNK != 0);

    alarm_snapshot_proto_ptr_t snapshot_proto = NULL;
    struct alarm_snapshot alarm_snap;
    struct alarm_log_entry alarm_log;

    alarm_snap.node_id = wc->node_id;
    alarm_snap.claim_id = claim_id.str;
    alarm_snap.snapshot_uuid = snapshot_uuid;
    alarm_snap.chunks = chunks;
    alarm_snap.chunk = 1;

    alarm_log.node_id = wc->node_id;
    alarm_log.claim_id = claim_id.str;

    cnt = 0;
    param = 0;
    uint64_t version = 0;
    int total_count = 0;
    while (sqlite3_step_monitored(res) == SQLITE_ROW) {
        cnt++;
        total_count++;

        if (!snapshot_proto)
            snapshot_proto = generate_alarm_snapshot_proto(&alarm_snap);

        health_alarm_log_populate(&alarm_log, res, host, NULL);

        add_alarm_log_entry2snapshot(snapshot_proto, &alarm_log);
        version += alarm_log.version;

        if (cnt == ALARM_EVENTS_PER_CHUNK) {
            if (aclk_online_for_alerts())
                aclk_send_alarm_snapshot(snapshot_proto);
            cnt = 0;
            if (alarm_snap.chunk < chunks) {
                alarm_snap.chunk++;
                snapshot_proto = generate_alarm_snapshot_proto(&alarm_snap);
            }
        }
        destroy_alarm_log_entry(&alarm_log);
    }
    if (cnt)
        aclk_send_alarm_snapshot(snapshot_proto);

    nd_log(
        NDLS_ACCESS,
        NDLP_DEBUG,
        "ACLK REQ [%s (%s)]: Sent! %d alerts snapshot, snapshot_uuid %s  (version = %llu)",
        wc->node_id,
        rrdhost_hostname(host),
        cnt,
        snapshot_uuid,
        (long long unsigned)version);

done:
    REPORT_BIND_FAIL(res, param);
    SQLITE_FINALIZE(res);
}

// Start streaming alerts
void aclk_start_alert_streaming(char *node_id, uint64_t cloud_version)
{
    nd_uuid_t node_uuid;

    if (unlikely(!node_id || uuid_parse(node_id, node_uuid)))
        return;

    struct aclk_sync_cfg_t *wc;
    RRDHOST *host = find_host_by_node_id(node_id);

    if (unlikely(!host || !(wc = host->aclk_config))) {
        nd_log(NDLS_ACCESS, NDLP_NOTICE, "ACLK STA [%s (N/A)]: Ignoring request to stream alert state changes, invalid node.", node_id);
        return;
    }

    if (unlikely(!host->health.enabled)) {
        nd_log(NDLS_ACCESS, NDLP_NOTICE, "ACLK STA [%s (N/A)]: Ignoring request to stream alert state changes, health is disabled.", node_id);
        return;
    }

    nd_log(NDLS_ACCESS, NDLP_DEBUG, "ACLK REQ [%s (%s)]: STREAM ALERTS ENABLED", node_id, wc->host ? rrdhost_hostname(wc->host) : "N/A");
    schedule_alert_snapshot_if_needed(wc, cloud_version);
    wc->stream_alerts = true;
}

// Do checkpoint alert version check
void aclk_alert_version_check(char *node_id, char *claim_id, uint64_t cloud_version)
{
    nd_uuid_t node_uuid;

    if (unlikely(!node_id || !claim_id || !is_agent_claimed() || uuid_parse(node_id, node_uuid)))
        return;

    CLAIM_ID agent_claim_id = claim_id_get();
    if (claim_id && claim_id_is_set(agent_claim_id) && strcmp(agent_claim_id.str, claim_id) != 0) {
        nd_log(NDLS_ACCESS, NDLP_NOTICE,
               "ACLK REQ [%s (N/A)]: ALERTS CHECKPOINT VALIDATION REQUEST RECEIVED WITH INVALID CLAIM ID",
               node_id);
        return;
    }

    struct aclk_sync_cfg_t *wc;
    RRDHOST *host = find_host_by_node_id(node_id);

    if ((!host || !(wc = host->aclk_config)))
        nd_log(NDLS_ACCESS, NDLP_NOTICE,
               "ACLK REQ [%s (N/A)]: ALERTS CHECKPOINT VALIDATION REQUEST RECEIVED FOR INVALID NODE",
               node_id);
    else
        schedule_alert_snapshot_if_needed(wc, cloud_version);
}
