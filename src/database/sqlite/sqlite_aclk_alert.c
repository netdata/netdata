// SPDX-License-Identifier: GPL-3.0-or-later

#include "sqlite_functions.h"
#include "sqlite_aclk_alert.h"

#include "../../aclk/aclk_alarm_api.h"
#include "../../health/health-alert-entry.h"

extern __thread bool is_health_thread;

static bool alert_snapshot_held(RRDHOST *host);
static void send_snapshot_if_online(alarm_snapshot_proto_ptr_t snapshot);
static void send_alert_snapshot_to_cloud(RRDHOST *host, void (*send_snapshot)(alarm_snapshot_proto_ptr_t));

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
                return false;
        }
        res = compiled_res;
    } else {
        if (!PREPARE_STATEMENT(db_meta, SQL_SELECT_LAST_ALERT_STATUS, &res))
            return false;
    }

    bool send = false;

    int param = 0;
    SQLITE_BIND_FAIL(done, sqlite3_bind_int64(res, ++param, health_log_id));

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

// Decide suppression and replacement atomically. A recovery that matches Cloud must still replace a queued
// REMOVED: the sender may already have read it. Its version-before-delete ordering and unique_id guard ensure
// this statement sees either that row or its acknowledged status, and preserve recovery after an in-flight send.
#define SQL_QUEUE_ALERT_TO_CLOUD                                                                                       \
    "INSERT INTO aclk_queue (host_id, health_log_id, unique_id, date_created)"                                         \
    " SELECT @host_id, @health_log_id, @unique_id, UNIXEPOCH()"                                                        \
    " WHERE NOT EXISTS (SELECT 1 FROM alert_version WHERE health_log_id = @health_log_id AND status = @status)"      \
    " OR EXISTS (SELECT 1 FROM aclk_queue WHERE host_id = @host_id AND health_log_id = @health_log_id)"               \
    " ON CONFLICT(host_id, health_log_id) DO UPDATE SET unique_id=excluded.unique_id, "                                \
    " date_created=excluded.date_created RETURNING sequence_id"

//
// Attempt to insert an alert to the submit queue to reach the cloud
//
// The alert will NOT be added in the submit queue if
// - Cloud is already aware of the alert status and no older transition is queued
// - The transition refers to a variable
//
// Returns 0 when queued, 1 or 2 when there is nothing to queue, -1 when the queue could not be written.
//
static int insert_alert_to_submit_queue(nd_uuid_t *host_id, int64_t health_log_id, uint32_t unique_id, RRDCALC_STATUS status)
{
    static __thread sqlite3_stmt *compiled_res = NULL;
    sqlite3_stmt *res = NULL;

    if (is_event_from_alert_variable_config(unique_id, host_id))
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

    int ret = -1;
    int param = 0;
    SQLITE_BIND_FAIL(done, sqlite3_bind_blob(res, ++param, host_id, sizeof(*host_id), SQLITE_STATIC));
    SQLITE_BIND_FAIL(done, sqlite3_bind_int64(res, ++param, health_log_id));
    SQLITE_BIND_FAIL(done, sqlite3_bind_int64(res, ++param, (int64_t) unique_id));
    SQLITE_BIND_FAIL(done, sqlite3_bind_int(res, ++param, status));

    param = 0;
    int rc = sqlite3_step_monitored(res);
    if (likely(rc == SQLITE_ROW))
        ret = 0;
    else if (rc == SQLITE_DONE) {
        update_alert_version_transition(health_log_id, unique_id);
        ret = 1;
    } else
        error_report("Failed to insert alert in the submit queue %"PRIu32", rc = %d", unique_id, rc);

done:
    REPORT_BIND_FAIL(res, param);
    if (is_health_thread)
        SQLITE_RESET(res);
    else
        SQLITE_FINALIZE(res);
    return ret;
}

#define SQL_DELETE_QUEUE_ALERT_TO_CLOUD                                                                                \
    "DELETE FROM aclk_queue WHERE host_id = @host_id AND sequence_id = @sequence_id AND unique_id = @unique_id"        \
    " RETURNING sequence_id"

//
// Remove one alert from the submit queue, after that transition was sent to the cloud.
//
// The unique_id is part of the predicate on purpose. The queue coalesces by re-pointing an existing row
// (ON CONFLICT ... DO UPDATE SET unique_id=...), and DO UPDATE keeps sequence_id, so a transition queued
// while this batch was in flight sits on a row we have already read and sent. Deleting by position alone
// would destroy it unsent and leave the cloud on a status the agent has already left. Guarded this way, a
// re-pointed row simply fails the predicate, survives, and goes out on the next pass.
//
// The caller owns the statement: this runs once per row sent, and re-preparing it each time would cost a
// full SQL parse (and a lifetime-lease round trip in simple_prepare_statement()) up to
// ACLK_MAX_ALERT_UPDATES times per host per tick.
// Returns true when the row was actually removed. A false means the row was re-pointed after we read it - the
// guard did its job and the newer transition is still queued, which the caller must not mistake for "done".
static bool delete_alert_from_submit_queue(nd_uuid_t *host_id, int64_t sequence_id, int64_t unique_id, sqlite3_stmt **res)
{
    bool deleted = false;

    if (!*res) {
        if (!PREPARE_STATEMENT(db_meta, SQL_DELETE_QUEUE_ALERT_TO_CLOUD, res))
            return false;
    }

    int param = 0;
    SQLITE_BIND_FAIL(done, sqlite3_bind_blob(*res, ++param, host_id, sizeof(*host_id), SQLITE_STATIC));
    SQLITE_BIND_FAIL(done, sqlite3_bind_int64(*res, ++param, sequence_id));
    SQLITE_BIND_FAIL(done, sqlite3_bind_int64(*res, ++param, unique_id));

    param = 0;
    // RETURNING: one row back means this exact transition was consumed, none means it was re-pointed
    int rc = sqlite3_step_monitored(*res);
    if (rc == SQLITE_ROW)
        deleted = true;
    else if (rc != SQLITE_DONE)
        error_report("Failed to delete submitted to ACLK");

done:
    REPORT_BIND_FAIL(*res, param);
    SQLITE_RESET(*res);
    return deleted;
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
    else if (!sqlite3_column_uuid_unparse_lower(res, iCol, uuid_str)) {
        error_report("ACLK ALERT: Got invalid UUID blob at column %d. Returning empty string.", iCol);
        uuid_str[0] = '\0';
    }

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

// Only the ACLK sender decides whether a queued replacement still needs a message. A recovery may replace
// an unsent REMOVED, or one already in flight: the acknowledged version distinguishes these cases. Suppression
// updates only the transition id, preserving the version Cloud already knows and avoiding checkpoint churn.
static bool send_alert_transition_if_needed(
    struct alarm_log_entry *entry, RRDCALC_STATUS status,
    ACLK_ALARM_LOG_SEND_RESULT (*send_entry)(struct alarm_log_entry *), sqlite3_stmt **res_version, bool *sent)
{
    *sent = false;
    if (cloud_status_matches(entry->health_log_id, status)) {
        update_alert_version_transition(entry->health_log_id, entry->unique_id);
        return true;
    }

    if (send_entry(entry) == ACLK_ALARM_LOG_SEND_FAILED)
        return false;

    // Publish the acknowledged version before the caller's unique_id-guarded queue deletion.
    sql_update_alert_version(entry->health_log_id, entry->unique_id, status, entry->version, res_version);
    *sent = true;
    return true;
}

#define SQL_SELECT_ALERT_TO_DUMMY                                                                                      \
    "SELECT aq.sequence_id, hld.unique_id, hld.when_key, hld.new_status, hld.health_log_id"                            \
    " FROM health_log hl, aclk_queue aq, alert_hash ah, health_log_detail hld"                                         \
    " WHERE hld.unique_id = aq.unique_id AND hld.health_log_id = aq.health_log_id"                                     \
    " AND hl.config_hash_id = ah.hash_id"                                                                              \
    " AND hl.host_id = @host_id AND aq.host_id = hl.host_id AND hl.health_log_id = hld.health_log_id"                  \
    " ORDER BY aq.sequence_id ASC LIMIT "ACLK_MAX_ALERT_UPDATES

// how many batches one commit_alert_events() call may drain before yielding to the next tick
#define ACLK_COMMIT_MAX_PASSES   10

//
// Check all queued alerts for a host and commit them as if they have been send to the cloud
// this will produce new versions as needed. We need this because we are about to send a
// a snapshot so we can include the latest transition.
//
// Returns true when it stopped on the pass cap with rows still queued, so the caller can re-arm the host:
// RRDHOST_FLAG_ACLK_STREAM_ALERTS is cleared before this runs, and on the non-streaming/archived path nothing
// else would set it again - an archived host generates no new transitions to re-arm it, so the remainder
// would sit in the queue indefinitely.
static bool commit_alert_events(RRDHOST *host)
{
    sqlite3_stmt *res = NULL;
    sqlite3_stmt *res_version = NULL;
    sqlite3_stmt *res_delete = NULL;
    bool more_queued = false;

    if (!PREPARE_STATEMENT(db_meta, SQL_SELECT_ALERT_TO_DUMMY, &res))
        return false;

    int param = 0;
    SQLITE_BIND_FAIL(done, sqlite3_bind_blob(res, ++param, &host->host_id.uuid, sizeof(host->host_id.uuid), SQLITE_STATIC));

    // Same reason as the push path: the deletes happen after the statement is reset, never while this joined
    // SELECT is being stepped. The LIMIT bounds one pass, and the pass repeats while it is full - each pass
    // deletes what it committed, so the next one sees the rest and the loop terminates.
    int64_t consumed_seq[ACLK_MAX_ALERT_UPDATES_N];
    int64_t consumed_uid[ACLK_MAX_ALERT_UPDATES_N];
    size_t consumed;
    int passes = 0;
    bool missed_delete = false;

    param = 0;
    do {
        consumed = 0;
        missed_delete = false;

        while (sqlite3_step_monitored(res) == SQLITE_ROW) {

            int64_t sequence_id = sqlite3_column_int64(res, 0);
            int64_t unique_id = sqlite3_column_int64(res, 1);
            int64_t version = sqlite3_column_int64(res, 2);
            RRDCALC_STATUS status = (RRDCALC_STATUS)sqlite3_column_int(res, 3);
            int64_t health_log_id = sqlite3_column_int64(res, 4);

            // Prepare the statement on the first time (res_version) then reuse it
            // finalize when we are done
            sql_update_alert_version(health_log_id, unique_id, status, version, &res_version);

            if (consumed < ACLK_MAX_ALERT_UPDATES_N) {
                consumed_seq[consumed] = sequence_id;
                consumed_uid[consumed] = unique_id;
                consumed++;
            }
        }

        SQLITE_RESET(res);

        // same guard as the push path: consume exactly the transitions we committed
        for (size_t i = 0; i < consumed; i++)
            if (!delete_alert_from_submit_queue(&host->host_id.uuid, consumed_seq[i], consumed_uid[i], &res_delete))
                missed_delete = true;

        // A row re-pointed during the pass fails its guarded delete by design and stays queued, so a pass can
        // come back full without making progress. That resolves itself - the next pass sees the newer
        // transition and consumes that - but do not spin the ACLK worker on it: stop after a bounded number of
        // passes and let the next tick continue, exactly as the push path does.
        // A full batch means there is more to read. A delete that matched nothing means a row we just
        // committed was re-pointed mid-pass and is still queued - both are remaining work, and on the
        // non-streaming path nothing but our return value will re-arm this host.
        more_queued = (consumed == ACLK_MAX_ALERT_UPDATES_N) || missed_delete;

    } while (more_queued && ++passes < ACLK_COMMIT_MAX_PASSES);

done:
    REPORT_BIND_FAIL(res, param);
    SQLITE_FINALIZE(res);
    SQLITE_FINALIZE(res_version);
    SQLITE_FINALIZE(res_delete);
    return more_queued;
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

    alarm_log->chart = sqlite3_text_strdupz_empty(res, CHART);
    alarm_log->name = sqlite3_text_strdupz_empty(res, NAME);

    alarm_log->when = sqlite3_column_int64(res, WHEN_KEY);

    alarm_log->config_hash = sqlite3_uuid_unparse_strdupz(res, CONFIG_HASH_ID);

    {
        RRDHOST_TZ host_tz = rrdhost_tz_get(host);
        alarm_log->utc_offset = host_tz.utc_offset;
        // Transfer ownership of the strdup'd copy instead of duplicating again
        alarm_log->timezone = host_tz.abbrev_timezone;
        host_tz.abbrev_timezone = NULL;
        rrdhost_tz_free(&host_tz);
    }
    alarm_log->exec_path =  sqlite3_column_bytes(res, EXEC) ?
                               strdupz((char *)sqlite3_column_text(res, EXEC)) :
                               strdupz((char *)string2str(host->health.default_exec));

    alarm_log->conf_source = source ? strdupz(source) : strdupz("");

    int64_t duration = sqlite3_column_int64(res, DURATION);
    int64_t non_clear_duration = sqlite3_column_int64(res, NON_CLEAR_DURATION);
    alarm_log->duration = nd_duration_to_uint32_saturating(duration);
    alarm_log->non_clear_duration = nd_duration_to_uint32_saturating(non_clear_duration);

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
    " WHERE hld.unique_id = aq.unique_id AND hld.health_log_id = aq.health_log_id"                                     \
    " AND hl.config_hash_id = ah.hash_id"                                                                              \
    " AND hl.host_id = @host_id AND aq.host_id = hl.host_id AND hl.health_log_id = hld.health_log_id"                  \
    " ORDER BY aq.sequence_id ASC LIMIT "ACLK_MAX_ALERT_UPDATES

static void aclk_push_alert_event(
    RRDHOST *host, sqlite3_stmt **res, sqlite3_stmt **res_version, sqlite3_stmt **res_delete,
    ACLK_ALARM_LOG_SEND_RESULT (*send_entry)(struct alarm_log_entry *))
{
    CLAIM_ID claim_id = claim_id_get();

    if (!claim_id_is_set(claim_id) || UUIDiszero(host->node_id))
        return;

    if (!*res) {
        if (!PREPARE_STATEMENT(db_meta, SQL_SELECT_ALERT_TO_PUSH, res))
            return;
    }

    // Deleting from aclk_queue while this SELECT is still being stepped would be relying on undefined
    // behaviour: SQLite only guarantees that deleting the current or a prior row is safe for a SELECT over a
    // SINGLE table, and this is a four-way join. What a joined SELECT does when the same connection modifies
    // one of its tables mid-scan "depends on which release of SQLite is running, the schema of the database
    // file, whether or not ANALYZE has been run, and the details of the query" (sqlite.org/isolation.html) -
    // and a row skipped that way would be a transition that is never sent, which is the very bug this guard
    // exists to prevent. So buffer what we consumed, reset the statement, then delete. One batch is bounded
    // by the LIMIT, so the buffer is too. Declared before the first bind: `done:` runs this loop, so a
    // SQLITE_BIND_FAIL goto must not be able to jump over the initialisation.
    int64_t consumed_seq[ACLK_MAX_ALERT_UPDATES_N];
    int64_t consumed_uid[ACLK_MAX_ALERT_UPDATES_N];
    size_t consumed = 0;

    int param = 0;
    SQLITE_BIND_FAIL(done, sqlite3_bind_blob(*res, ++param, &host->host_id.uuid, sizeof(host->host_id.uuid), SQLITE_STATIC));

    char node_id_str[UUID_STR_LEN];
    uuid_unparse_lower(host->node_id.uuid, node_id_str);

    struct alarm_log_entry alarm_log;
    alarm_log.node_id = node_id_str;
    alarm_log.claim_id = claim_id.str;

    size_t sent = 0;
    int64_t first_seq = 0, last_seq = 0;


    param = 0;
    RRDCALC_STATUS status;
    struct aclk_sync_cfg_t *aclk_host_config = __atomic_load_n(&host->aclk_host_config, __ATOMIC_ACQUIRE);
    while (sqlite3_step_monitored(*res) == SQLITE_ROW) {
        health_alarm_log_populate(&alarm_log, *res, host, &status);

        // A message the MQTT layer did not accept right now is not a delivery: leave this row and the ones after it
        // queued, with their version untouched, and retry on the next pass. A message that can never be sent
        // (cannot be built, too big for the server) is consumed as before, so one bad row cannot block the queue.
        bool emitted;
        if (!send_alert_transition_if_needed(&alarm_log, status, send_entry, res_version, &emitted)) {
            destroy_alarm_log_entry(&alarm_log);
            rrdhost_flag_set(host, RRDHOST_FLAG_ACLK_STREAM_ALERTS);
            break;
        }

        if (emitted) {
            nd_uuid_t hash_id;
            if (alarm_log.config_hash && !uuid_parse(alarm_log.config_hash, hash_id))
                alert_hash_mark_sent(&hash_id);

            aclk_host_config->alert_count++;
            sent++;
            if (!first_seq)
                first_seq = alarm_log.sequence_id;
            last_seq = alarm_log.sequence_id;
        }

        // the rows are consumed after this statement is reset - see the comment on the buffer
        if (consumed < ACLK_MAX_ALERT_UPDATES_N) {
            consumed_seq[consumed] = alarm_log.sequence_id;
            consumed_uid[consumed] = (int64_t)alarm_log.unique_id;
            consumed++;
        }

        destroy_alarm_log_entry(&alarm_log);
    }

    if (sent) {
        nd_log(
            NDLS_ACCESS,
            NDLP_DEBUG,
            "ACLK RES [%s (%s)]: ALERTS SENT %zu (seq span %lld - %lld)",
            node_id_str,
            rrdhost_hostname(host),
            sent,
            (long long)first_seq,
            (long long)last_seq);
    }
    // Suppressed rows also consume the bounded batch; check again even when none needed a message.
    if (consumed)
        rrdhost_flag_set(host, RRDHOST_FLAG_ACLK_STREAM_ALERTS);

done:
    REPORT_BIND_FAIL(*res, param);
    SQLITE_RESET(*res);

    // the cursor is closed now, so these cannot perturb it
    for (size_t i = 0; i < consumed; i++)
        if (!delete_alert_from_submit_queue(&host->host_id.uuid, consumed_seq[i], consumed_uid[i], res_delete))
            rrdhost_flag_set(host, RRDHOST_FLAG_ACLK_STREAM_ALERTS);
}

// The unique_id guard is what makes this safe against the same re-pointing the submit queue does:
// insert_alert_queue() coalesces with ON CONFLICT ... DO UPDATE SET status, unique_id, keeping the rowid.
#define SQL_DELETE_PROCESSED_ROWS                                                                                      \
    "DELETE FROM alert_queue WHERE host_id = @host_id AND rowid = @row AND unique_id = @unique_id"

static void delete_alert_from_pending_queue(nd_uuid_t *host_id, int64_t row, int64_t unique_id)
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
    SQLITE_BIND_FAIL(done, sqlite3_bind_blob(res, ++param, host_id, sizeof(*host_id), SQLITE_STATIC));
    SQLITE_BIND_FAIL(done, sqlite3_bind_int64(res, ++param, row));
    SQLITE_BIND_FAIL(done, sqlite3_bind_int64(res, ++param, unique_id));

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
    "INSERT OR IGNORE INTO alert_version (health_log_id, unique_id, status, version, date_submitted) "                 \
    " SELECT hl.health_log_id, hld.unique_id, hld.new_status, hld.when_key, UNIXEPOCH() "                              \
    " FROM health_log hl, health_log_detail hld WHERE "                                                                \
    "  hl.host_id = @host_id AND hld.health_log_id = hl.health_log_id AND hld.transition_id = hl.last_transition_id"

#define SQL_DELETE_HOST_ALERT_VERSION_TABLE                                                                            \
    "DELETE FROM alert_version WHERE health_log_id IN (SELECT health_log_id FROM health_log WHERE host_id = @host_id)"

#define SQL_DELETE_SNAPSHOT_COVERED_SUBMIT_ROWS                                                                         \
    "DELETE FROM aclk_queue WHERE host_id = @host_id AND EXISTS ("                                                     \
    " SELECT 1 FROM alert_version av WHERE av.health_log_id = aclk_queue.health_log_id"                                 \
    " AND aclk_queue.unique_id <= av.unique_id)"

// The DELETE and the INSERT must look like one step to the other threads using db_meta:
// cloud_status_matches() and calculate_node_alert_version() read alert_version on the same connection, and seeing it
// empty would re-queue transitions or schedule a needless snapshot. A transaction does not isolate threads that share
// a connection, so hold the connection mutex across both statements; the savepoint makes them all-or-nothing.
// When another transaction is already open on db_meta (metadata_scan_host), a savepoint would only nest inside it, so
// the rebuild is deferred to the next pass. Snapshot eligibility and health's decision to start evaluating share
// this mutex. A newly held checkpoint rolls back its speculative versions; RELEASE captures an eligible state
// that remains publishable even if a new transition starts afterward.
//
// Returns true when the table was rebuilt.
static bool rebuild_host_alert_version_table(RRDHOST *host)
{
    sqlite3_stmt *res_delete = NULL;
    sqlite3_stmt *res_insert = NULL;
    sqlite3_stmt *res_queue = NULL;
    bool rebuilt = false;
    int param = 0;

    if (!PREPARE_STATEMENT(db_meta, SQL_DELETE_HOST_ALERT_VERSION_TABLE, &res_delete))
        return false;

    if (!PREPARE_STATEMENT(db_meta, SQL_REBUILD_HOST_ALERT_VERSION_TABLE, &res_insert))
        goto done;
    if (!PREPARE_STATEMENT(db_meta, SQL_DELETE_SNAPSHOT_COVERED_SUBMIT_ROWS, &res_queue))
        goto done;

    SQLITE_BIND_FAIL(done, sqlite3_bind_blob(res_delete, ++param, &host->host_id.uuid, sizeof(host->host_id.uuid), SQLITE_STATIC));
    param = 0;
    SQLITE_BIND_FAIL(done, sqlite3_bind_blob(res_insert, ++param, &host->host_id.uuid, sizeof(host->host_id.uuid), SQLITE_STATIC));
    param = 0;
    SQLITE_BIND_FAIL(done, sqlite3_bind_blob(res_queue, ++param, &host->host_id.uuid, sizeof(host->host_id.uuid), SQLITE_STATIC));
    param = 0;

    sqlite3_mutex *db_mutex = sqlite3_db_mutex(db_meta);
    sqlite3_mutex_enter(db_mutex);

    if (!sqlite3_get_autocommit(db_meta) || alert_snapshot_held(host)) {
        sqlite3_mutex_leave(db_mutex);
        goto done;
    }

    if (sqlite3_exec(db_meta, "SAVEPOINT rebuild_alert_version", NULL, NULL, NULL) != SQLITE_OK) {
        sqlite3_mutex_leave(db_mutex);
        error_report("Failed to start rebuilding the host alert version table");
        goto done;
    }

    int rc = execute_insert(res_delete);
    if (rc == SQLITE_DONE)
        rc = execute_insert(res_insert);
    // The captured checkpoint supersedes these submit rows. Consume them in the same savepoint so an older
    // REMOVED cannot follow the recovered snapshot, and a new hold restores both versions and queued rows.
    if (rc == SQLITE_DONE)
        rc = execute_insert(res_queue);

    bool held = rc == SQLITE_DONE && alert_snapshot_held(host);
    if (rc == SQLITE_DONE && !held && sqlite3_exec(db_meta, "RELEASE rebuild_alert_version", NULL, NULL, NULL) == SQLITE_OK)
        rebuilt = true;
    else {
        (void)sqlite3_exec(db_meta, "ROLLBACK TO rebuild_alert_version", NULL, NULL, NULL);
        (void)sqlite3_exec(db_meta, "RELEASE rebuild_alert_version", NULL, NULL, NULL);
        if (!held)
            error_report("Failed to rebuild the host alert version table, rc = %d", rc);
    }

    sqlite3_mutex_leave(db_mutex);

done:
    REPORT_BIND_FAIL(res_delete, param);
    SQLITE_FINALIZE(res_delete);
    SQLITE_FINALIZE(res_insert);
    SQLITE_FINALIZE(res_queue);
    return rebuilt;
}

#define SQL_PROCESS_ALERT_PENDING_QUEUE                                                                                \
    "SELECT health_log_id, unique_id, status, rowid"                                                                   \
    " FROM alert_queue WHERE host_id = @host_id AND date_scheduled <= UNIXEPOCH() ORDER BY rowid ASC"

#define SQL_ALERT_PENDING_QUEUE_NEXT_DUE "SELECT MIN(date_scheduled) FROM alert_queue WHERE host_id = @host_id"

// a drain that failed, or kept a row it could not submit, looks at the queue again after this many seconds
#define ALERT_QUEUE_RETRY_S (10)

#define SQL_PENDING_ALERT_STILL_CURRENT                                                                                \
    "SELECT 1 FROM alert_queue WHERE host_id = @host_id AND health_log_id = @health_log_id"                            \
    " AND rowid = @row AND unique_id = @unique_id"

// A pending cursor can cache a transition before a recovery replaces it or a snapshot captures that recovery.
// Check ownership and submit under one connection lock; stale candidates must not queue or rewind version ids.
static int submit_pending_alert_if_current(
    nd_uuid_t *host_id, int64_t health_log_id, int64_t row, uint32_t unique_id, RRDCALC_STATUS status)
{
    static __thread sqlite3_stmt *compiled_res = NULL;
    sqlite3_stmt *res = NULL;
    if (is_health_thread) {
        if (!compiled_res && !PREPARE_COMPILED_STATEMENT(db_meta, SQL_PENDING_ALERT_STILL_CURRENT, &compiled_res))
            return -1;
        res = compiled_res;
    } else if (!PREPARE_STATEMENT(db_meta, SQL_PENDING_ALERT_STILL_CURRENT, &res))
        return -1;

    int ret = -1;
    int param = 0;
    SQLITE_BIND_FAIL(done, sqlite3_bind_blob(res, ++param, host_id, sizeof(*host_id), SQLITE_STATIC));
    SQLITE_BIND_FAIL(done, sqlite3_bind_int64(res, ++param, health_log_id));
    SQLITE_BIND_FAIL(done, sqlite3_bind_int64(res, ++param, row));
    SQLITE_BIND_FAIL(done, sqlite3_bind_int64(res, ++param, unique_id));
    param = 0;

    sqlite3_mutex *db_mutex = sqlite3_db_mutex(db_meta);
    sqlite3_mutex_enter(db_mutex);
    int rc = sqlite3_step_monitored(res);
    SQLITE_RESET(res);
    if (rc == SQLITE_ROW)
        ret = insert_alert_to_submit_queue(host_id, health_log_id, unique_id, status);
    else if (rc == SQLITE_DONE)
        ret = 1;
    else
        error_report("Failed to verify pending alert ownership, rc = %d", rc);
    sqlite3_mutex_leave(db_mutex);

done:
    REPORT_BIND_FAIL(res, param);
    if (is_health_thread)
        SQLITE_RESET(res);
    else
        SQLITE_FINALIZE(res);
    return ret;
}

// The pending queue is only read when something in it can be due: every writer lowers the host's next-due time
// to the date_scheduled it wrote, and each drain sets it to the earliest date_scheduled still queued.
void alert_queue_next_due_lower(RRDHOST *host, time_t when)
{
    time_t *next_due = &host->health.alert_queue_next_due_s;
    time_t current = __atomic_load_n(next_due, __ATOMIC_RELAXED);
    while (when < current &&
           !__atomic_compare_exchange_n(next_due, &current, when, false, __ATOMIC_RELEASE, __ATOMIC_RELAXED))
        ;
}

#define SQL_ALERT_PENDING_QUEUE_NOT_DUE                                                                                \
    "SELECT 1 FROM alert_queue WHERE host_id = @host_id AND date_scheduled > @now LIMIT 1"

// True while the host has an alert_queue row that is not due yet. The next-due gate cannot answer this: it is the
// earliest due time only, a row left due by a drain hides the later ones, and a drain sets it to nd_time_t_max() while
// it runs, so the queue itself is read. A failed read answers false, so a snapshot is never held by it.
static bool alert_queue_has_rows_not_due(RRDHOST *host, time_t now)
{
    sqlite3_stmt *res = NULL;

    if (!PREPARE_STATEMENT(db_meta, SQL_ALERT_PENDING_QUEUE_NOT_DUE, &res))
        return false;

    bool not_due = false;
    int param = 0;
    SQLITE_BIND_FAIL(done, sqlite3_bind_blob(res, ++param, &host->host_id.uuid, sizeof(host->host_id.uuid), SQLITE_STATIC));
    SQLITE_BIND_FAIL(done, sqlite3_bind_int64(res, ++param, (sqlite3_int64)now));

    param = 0;
    not_due = (sqlite3_step_monitored(res) == SQLITE_ROW);

done:
    REPORT_BIND_FAIL(res, param);
    SQLITE_FINALIZE(res);
    return not_due;
}

// Snapshots wait for queued transition deadlines as well as writes still in progress. Otherwise the health log's
// latest REMOVED or non-runnable UNINITIALIZED state would bypass the grace period, including after reconnection.
// The queue row is written before pending_transitions drops, so a transition being saved cannot escape both gates.
static bool alert_snapshot_held(RRDHOST *host)
{
    if (__atomic_load_n(&host->health.pending_transitions, __ATOMIC_ACQUIRE))
        return true;

    // Online health must finish its first recovery pass too. While any gate holds, health keeps evaluating
    // even with a READY snapshot; a raised recovery can then replace the delayed initialization transition.
    if (rrdhost_flag_check(host, RRDHOST_FLAG_COLLECTOR_ONLINE) && host->health.enabled &&
        !__atomic_load_n(&host->health.alert_status_snapshot.valid, __ATOMIC_ACQUIRE))
        return true;

    return alert_queue_has_rows_not_due(host, now_realtime_sec());
}

bool alert_snapshot_freezes_health(RRDHOST *host)
{
    struct aclk_sync_cfg_t *config = __atomic_load_n(&host->aclk_host_config, __ATOMIC_ACQUIRE);
    if (!config || aclk_alert_snapshot_state_get(config) != ACLK_ALERT_SNAPSHOT_READY)
        return false;

    // Serialize admission of another health pass with snapshot capture. Invalidate before releasing the
    // mutex so an expiring queue deadline cannot expose partially evaluated alerts to the snapshot builder.
    sqlite3_mutex *db_mutex = sqlite3_db_mutex(db_meta);
    sqlite3_mutex_enter(db_mutex);
    bool freeze = false;
    if (aclk_alert_snapshot_state_get(config) == ACLK_ALERT_SNAPSHOT_READY) {
        freeze = !alert_snapshot_held(host);
        if (!freeze)
            __atomic_store_n(&host->health.alert_status_snapshot.valid, 0, __ATOMIC_RELEASE);
    }
    sqlite3_mutex_leave(db_mutex);
    return freeze;
}

// the earliest date_scheduled queued for the host, nd_time_t_max() when nothing is
static bool alert_queue_get_next_due(RRDHOST *host, time_t *next_due)
{
    static __thread sqlite3_stmt *compiled_res = NULL;
    sqlite3_stmt *res = NULL;

    if (is_health_thread) {
        if (!compiled_res) {
            if (!PREPARE_COMPILED_STATEMENT(db_meta, SQL_ALERT_PENDING_QUEUE_NEXT_DUE, &compiled_res))
                return false;
        }
        res = compiled_res;
    } else {
        if (!PREPARE_STATEMENT(db_meta, SQL_ALERT_PENDING_QUEUE_NEXT_DUE, &res))
            return false;
    }

    bool ok = false;
    int param = 0;
    SQLITE_BIND_FAIL(done, sqlite3_bind_blob(res, ++param, &host->host_id.uuid, sizeof(host->host_id.uuid), SQLITE_STATIC));

    param = 0;
    if (sqlite3_step_monitored(res) == SQLITE_ROW) {
        *next_due = sqlite3_column_type(res, 0) == SQLITE_NULL ? nd_time_t_max() : (time_t)sqlite3_column_int64(res, 0);
        ok = true;
    }

done:
    REPORT_BIND_FAIL(res, param);
    if (is_health_thread)
        SQLITE_RESET(res);
    else
        SQLITE_FINALIZE(res);
    return ok;
}

bool process_alert_pending_queue(RRDHOST *host)
{
    if (!REQUIRE_HEALTH_DB_OPEN())
        return false;

    time_t now = now_realtime_sec();
    if (now < __atomic_load_n(&host->health.alert_queue_next_due_s, __ATOMIC_ACQUIRE))
        return false;

    // Clear the gate before reading: a row inserted from here on lowers it again, so it cannot be missed.
    (void)__atomic_exchange_n(&host->health.alert_queue_next_due_s, nd_time_t_max(), __ATOMIC_ACQ_REL);

    static __thread sqlite3_stmt *compiled_res = NULL;
    sqlite3_stmt *res = NULL;

    if (is_health_thread) {
        if (!compiled_res) {
            if (!PREPARE_COMPILED_STATEMENT(db_meta, SQL_PROCESS_ALERT_PENDING_QUEUE, &compiled_res)) {
                alert_queue_next_due_lower(host, nd_time_t_add_saturating(now, ALERT_QUEUE_RETRY_S));
                return false;
            }
        }
        res = compiled_res;
    } else {
        if (!PREPARE_STATEMENT(db_meta, SQL_PROCESS_ALERT_PENDING_QUEUE, &res)) {
            alert_queue_next_due_lower(host, nd_time_t_add_saturating(now, ALERT_QUEUE_RETRY_S));
            return false;
        }
    }

    int param = 0;
    int added = 0, count = 0, kept = 0;
    bool drained = false;
    SQLITE_BIND_FAIL(done, sqlite3_bind_blob(res, ++param, &host->host_id.uuid, sizeof(host->host_id.uuid), SQLITE_STATIC));

    param = 0;
    int rc;
    while ((rc = sqlite3_step_monitored(res)) == SQLITE_ROW) {

        int64_t health_log_id = sqlite3_column_int64(res, 0);
        uint32_t unique_id = sqlite3_column_int64(res, 1);
        RRDCALC_STATUS new_status = sqlite3_column_int(res, 2);
        int64_t row = sqlite3_column_int64(res, 3);
        count++;

        struct aclk_sync_cfg_t *aclk_host_config = __atomic_load_n(&host->aclk_host_config, __ATOMIC_ACQUIRE);
        if (aclk_host_config) {
            int ret = submit_pending_alert_if_current(&host->host_id.uuid, health_log_id, row, unique_id, new_status);
            if (ret == 0)
                added++;
            else if (ret < 0) {
                // not submitted: keep the row, it is retried
                kept++;
                continue;
            }
        }

        delete_alert_from_pending_queue(&host->host_id.uuid, row, (int64_t)unique_id);
    }
    drained = (rc == SQLITE_DONE && !kept);

    if(count)
        nd_log(NDLS_ACCESS, NDLP_NOTICE, "ACLK STA [%s (N/A)]: Processed %d entries, queued %d, kept %d", rrdhost_hostname(host), count, added, kept);
done:
    REPORT_BIND_FAIL(res, param);
    if (is_health_thread)
        SQLITE_RESET(res);
    else
        SQLITE_FINALIZE(res);

    // A row left behind (re-pointed during the drain, or a failed delete) is already due, so it brings the gate
    // back to the past and the next call drains it.
    time_t next_due;
    if (drained && alert_queue_get_next_due(host, &next_due))
        alert_queue_next_due_lower(host, next_due);
    else
        alert_queue_next_due_lower(host, nd_time_t_add_saturating(now, ALERT_QUEUE_RETRY_S));

    return added > 0;
}

// A submit-queue row is consumed by naming its exact transition, so a row whose transition no longer exists
// can never be consumed: the push SELECT joins health_log_detail and simply does not return it. That happens
// when health-log retention removes a superseded transition (SQL_CLEANUP_HEALTH_LOG_DETAIL deletes rows with
// updated_by_id <> 0) that was still queued. The old range delete swept such rows incidentally; now they need
// removing on purpose, or they accumulate for the lifetime of the database.
//
// The age guard is not a sendability timeout - an unsendable row is unsendable however long we wait, and a
// legitimately pending row must survive an ACLK outage of any length. It only keeps us from racing a row that
// was just inserted.
#define ACLK_QUEUE_REAP_AGE_S    3600
#define ACLK_QUEUE_REAP_EVERY_S  3600
#define ACLK_QUEUE_REAP_RETRY_S  60

// The predicate mirrors the push SELECT: a row is unsendable if ANY of the joins that query needs cannot be
// satisfied - the transition itself (health_log_detail), its alert (health_log), or its config (alert_hash).
// RETURNING gives an exact count from the statement itself; sqlite3_changes() would be a separate read of a
// per-connection counter that the health thread shares and can clobber between our step and our read.
// The row set is bounded per call: SQLite materialises all RETURNING output during the first step, so an
// unbounded DELETE would allocate in proportion to the whole backlog. Anything left waits for the next hour.
// The LIMIT is deliberately unordered: every row it selects is deleted by this same statement, so a backlog
// drains regardless of which rows each pass picks. An ORDER BY would buy determinism nobody needs at the cost
// of a sort on an hourly path over a potentially large set.
#define ACLK_QUEUE_REAP_MAX_ROWS 1000

#define SQL_REAP_UNSENDABLE_QUEUE_ROWS                                                                                 \
    "DELETE FROM aclk_queue WHERE sequence_id IN"                                                                      \
    " (SELECT q.sequence_id FROM aclk_queue q WHERE q.date_created < UNIXEPOCH() - @age AND NOT EXISTS"                 \
    "   (SELECT 1 FROM health_log hl, health_log_detail hld, alert_hash ah"                                            \
    "     WHERE hl.health_log_id = q.health_log_id AND hl.host_id = q.host_id"                                         \
    "       AND hld.health_log_id = q.health_log_id AND hld.unique_id = q.unique_id"                                   \
    "       AND ah.hash_id = hl.config_hash_id)"                                                                       \
    "  LIMIT @limit)"                                                                                                  \
    " RETURNING sequence_id"

// Returns the number of rows reaped, or -1 if the reap could not be completed. The caller schedules the next
// attempt from that: advancing the hourly timer after a failure would hide a broken cleanup for an hour.
static int aclk_queue_reap_unsendable(int64_t age_s)
{
    sqlite3_stmt *res = NULL;
    int reaped = 0;
    bool failed = false;

    if (!PREPARE_STATEMENT(db_meta, SQL_REAP_UNSENDABLE_QUEUE_ROWS, &res))
        return -1;

    int param = 0;
    SQLITE_BIND_FAIL(done, sqlite3_bind_int64(res, ++param, age_s));
    SQLITE_BIND_FAIL(done, sqlite3_bind_int64(res, ++param, ACLK_QUEUE_REAP_MAX_ROWS));

    param = 0;
    int rc;
    while ((rc = sqlite3_step_monitored(res)) == SQLITE_ROW)
        reaped++;

    if (rc != SQLITE_DONE) {
        error_report("Failed to reap unsendable alert submit queue rows, rc = %d", rc);
        failed = true;
    }

    if (reaped)
        nd_log(NDLS_DAEMON, NDLP_NOTICE, "ACLK: reaped %d alert submit queue rows whose transition no longer exists", reaped);

done:
    // param is non-zero here only when a bind failed, which is the other way this reap did not run
    if (param)
        failed = true;

    REPORT_BIND_FAIL(res, param);
    SQLITE_FINALIZE(res);
    return failed ? -1 : reaped;
}

static void aclk_push_alert_events_for_host(
    RRDHOST *host, sqlite3_stmt **res, sqlite3_stmt **res_version, sqlite3_stmt **res_delete,
    ACLK_ALARM_LOG_SEND_RESULT (*send_entry)(struct alarm_log_entry *),
    void (*send_snapshot)(alarm_snapshot_proto_ptr_t))
{
    if (!rrdhost_flag_check(host, RRDHOST_FLAG_ACLK_STREAM_ALERTS) ||
        rrdhost_flag_check(host, RRDHOST_FLAG_PENDING_CONTEXT_LOAD))
        return;

    rrdhost_flag_clear(host, RRDHOST_FLAG_ACLK_STREAM_ALERTS);

    struct aclk_sync_cfg_t *aclk_host_config = __atomic_load_n(&host->aclk_host_config, __ATOMIC_ACQUIRE);
    // alert_queue is drained only by the health thread, for every host; here only what it submitted is consumed
    if (!aclk_host_config || !aclk_alert_streaming_enabled(aclk_host_config) || rrdhost_flag_check(host, RRDHOST_FLAG_ARCHIVED)) {
        if (commit_alert_events(host))
            rrdhost_flag_set(host, RRDHOST_FLAG_ACLK_STREAM_ALERTS);
        return;
    }

    enum aclk_alert_snapshot_state snapshot_state = aclk_alert_snapshot_state_get(aclk_host_config);
    if (snapshot_state != ACLK_ALERT_SNAPSHOT_IDLE) {
        rrdhost_flag_set(host, RRDHOST_FLAG_ACLK_STREAM_ALERTS);
        if (snapshot_state == ACLK_ALERT_SNAPSHOT_PENDING)
            return;
        // A held/deferred checkpoint must not delay unrelated due alerts. Failed preparation rolls back its
        // speculative versions, so normal sender dedup still compares against what Cloud actually knows.
        if (!rebuild_host_alert_version_table(host)) {
            aclk_push_alert_event(host, res, res_version, res_delete, send_entry);
            return;
        }
        // Successful preparation captured an eligible state under the DB mutex. A later hold belongs to
        // newer transitions and must not veto publication after those captured versions were committed.
        send_alert_snapshot_to_cloud(host, send_snapshot);
        aclk_host_config->snapshot_count++;
        aclk_alert_snapshot_complete(aclk_host_config);
    }
    else
        aclk_push_alert_event(host, res, res_version, res_delete, send_entry);
}

void aclk_push_alert_events_for_all_hosts(void)
{
    RRDHOST *host;

    sqlite3_stmt *res = NULL;               // used to scan pending alerts to send
    sqlite3_stmt *res_version = NULL;       // used to update the alert version
    sqlite3_stmt *res_delete = NULL;        // used to consume each sent row

    // one push runs at a time (aclk_sync_config.alert_push_running), but successive pushes are different
    // threadpool workers, so read and write this gate atomically rather than relying on that ordering
    static time_t next_reap_s = 0;
    time_t now_s = now_realtime_sec();
    time_t next_s = __atomic_load_n(&next_reap_s, __ATOMIC_RELAXED);
    if (now_s >= next_s) {
        time_t again_in_s = ACLK_QUEUE_REAP_EVERY_S;

        // A failed reap must not buy the failure an hour of silence. Retry sooner instead - but not on the
        // next tick: this worker runs every second, and a persistently failing reap would then hammer db_meta.
        //
        // This runs on the first deadline too. Skipping the first pass bought nothing: what keeps the reaper
        // off a row that is merely waiting for connectivity is the age guard and the unjoinable predicate, not
        // the pass number - so rows left unsendable by a previous run had to wait an hour for no reason.
        if (aclk_queue_reap_unsendable(ACLK_QUEUE_REAP_AGE_S) < 0)
            again_in_s = ACLK_QUEUE_REAP_RETRY_S;

        __atomic_store_n(&next_reap_s, nd_time_t_add_saturating(now_s, again_in_s), __ATOMIC_RELAXED);
    }

    dfe_start_reentrant(rrdhost_root_index, host) {
        aclk_push_alert_events_for_host(host, &res, &res_version, &res_delete,
                                        aclk_send_alarm_log_entry, send_snapshot_if_online);
    }
    dfe_done(host);
    SQLITE_FINALIZE(res);
    SQLITE_FINALIZE(res_version);
    SQLITE_FINALIZE(res_delete);
}

#define SQL_SELECT_ALERT_HASH_CLOUD "SELECT 1 FROM alert_hash_cloud WHERE hash_id = @hash_id"
#define SQL_INSERT_ALERT_HASH_CLOUD "INSERT OR IGNORE INTO alert_hash_cloud (hash_id) VALUES (@hash_id)"

void alert_hash_mark_sent(nd_uuid_t *hash_id)
{
    if (!hash_id)
        return;

    static __thread sqlite3_stmt *compiled_res = NULL;
    sqlite3_stmt *res = NULL;

    if (is_health_thread) {
        if (!compiled_res) {
            if (!PREPARE_COMPILED_STATEMENT(db_meta, SQL_INSERT_ALERT_HASH_CLOUD, &compiled_res))
                return;
        }
        res = compiled_res;
    } else {
        if (!PREPARE_STATEMENT(db_meta, SQL_INSERT_ALERT_HASH_CLOUD, &res))
            return;
    }

    int param = 0;
    SQLITE_BIND_FAIL(done, sqlite3_bind_blob(res, ++param, hash_id, sizeof(*hash_id), SQLITE_STATIC));

    param = 0;
    (void)sqlite3_step_monitored(res);

done:
    REPORT_BIND_FAIL(res, param);
    if (is_health_thread)
        SQLITE_RESET(res);
    else
        SQLITE_FINALIZE(res);
}

bool alert_hash_has_transitioned(nd_uuid_t *hash_id)
{
    if (!hash_id)
        return false;

    static __thread sqlite3_stmt *compiled_res = NULL;
    sqlite3_stmt *res = NULL;

    if (is_health_thread) {
        if (!compiled_res) {
            if (!PREPARE_COMPILED_STATEMENT(db_meta, SQL_SELECT_ALERT_HASH_CLOUD, &compiled_res))
                return false;
        }
        res = compiled_res;
    } else {
        if (!PREPARE_STATEMENT(db_meta, SQL_SELECT_ALERT_HASH_CLOUD, &res))
            return false;
    }

    int param = 0;
    bool found = true;
    SQLITE_BIND_FAIL(done, sqlite3_bind_blob(res, ++param, hash_id, sizeof(*hash_id), SQLITE_STATIC));

    param = 0;
    found = (sqlite3_step_monitored(res) == SQLITE_ROW);

done:
    REPORT_BIND_FAIL(res, param);
    if (is_health_thread)
        SQLITE_RESET(res);
    else
        SQLITE_FINALIZE(res);
    return found;
}

void aclk_send_alert_configuration(char *config_hash)
{
    if (unlikely(!config_hash))
        return;

    struct aclk_sync_cfg_t *aclk_host_config = __atomic_load_n(&localhost->aclk_host_config, __ATOMIC_ACQUIRE);

    if (unlikely(!aclk_host_config))
        return;

    char node_id[UUID_STR_LEN];
    aclk_node_id_copy(aclk_host_config, node_id);
    nd_log(NDLS_ACCESS, NDLP_DEBUG,
        "ACLK REQ [%s (%s)]: Request to send alert config %s.",
        node_id,
        aclk_host_config->host ? rrdhost_hostname(aclk_host_config->host) : "N/A",
        config_hash);

    aclk_push_alert_config(node_id, config_hash);
}

#define SQL_SELECT_ALERT_CONFIG                                                                                        \
    "SELECT alarm, template, on_key, class, type, component, os, hosts, plugin,"                                       \
    "module, charts, lookup, every, units, green, red, calc, warn, crit, to_key, exec, delay, repeat, info,"           \
    "options, host_labels, p_db_lookup_dimensions, p_db_lookup_method, p_db_lookup_options, p_db_lookup_after,"        \
    "p_db_lookup_before, p_update_every, chart_labels, summary FROM alert_hash WHERE hash_id = @hash_id"

void aclk_push_alert_config_event(char *node_id __maybe_unused, char *config_hash __maybe_unused)
{
    sqlite3_stmt *res = NULL;
    struct aclk_sync_cfg_t *aclk_host_config;

    RRDHOST *host = rrdhost_find_by_node_id(node_id);

    if (!host || !(aclk_host_config = __atomic_load_n(&host->aclk_host_config, __ATOMIC_ACQUIRE))) {
        freez(config_hash);
        freez(node_id);
        return;
    }

    char current_node_id[UUID_STR_LEN];
    aclk_node_id_copy(aclk_host_config, current_node_id);

    nd_uuid_t hash_uuid;
    if (uuid_parse(config_hash, hash_uuid)) {
        freez(config_hash);
        freez(node_id);
        return;
    }

    if (!PREPARE_STATEMENT(db_meta, SQL_SELECT_ALERT_CONFIG, &res)) {
        freez(config_hash);
        freez(node_id);
        return;
    }

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
        nd_log(NDLS_ACCESS, NDLP_DEBUG, "ACLK RES [%s (%s)]: Sent alert config %s.",
            current_node_id,
            aclk_host_config->host ? rrdhost_hostname(aclk_host_config->host) : "N/A", config_hash);
        aclk_send_provide_alarm_cfg(&p_alarm_config);
        alert_hash_mark_sent(&hash_uuid);
        freez(p_alarm_config.cfg_hash);
        destroy_aclk_alarm_configuration(&alarm_config);
    }
    else
        nd_log(NDLS_ACCESS, NDLP_WARNING, "ACLK STA [%s (%s)]: Alert config for %s not found.",
            current_node_id,
            aclk_host_config->host ? rrdhost_hostname(aclk_host_config->host) : "N/A", config_hash);

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

static void schedule_alert_snapshot_if_needed(struct aclk_sync_cfg_t *aclk_host_config, uint64_t cloud_version)
{
    char node_id[UUID_STR_LEN];
    aclk_node_id_copy(aclk_host_config, node_id);

    if (cloud_version == 1) {
        nd_log(
            NDLS_ACCESS,
            NDLP_NOTICE,
            "Cloud requested to skip alert version verification for host \"%s\", node \"%s\"",
            rrdhost_hostname(aclk_host_config->host),
            node_id);
        return;
    }

    uint64_t local_version = calculate_node_alert_version(aclk_host_config->host);
    if (local_version != cloud_version) {
        nd_log(
            NDLS_ACCESS,
            NDLP_NOTICE,
            "Scheduling alert snapshot for host \"%s\", node \"%s\" (version: cloud %llu, local %llu)",
            rrdhost_hostname(aclk_host_config->host),
            node_id,
            (long long unsigned)cloud_version,
            (long long unsigned)local_version);

        aclk_alert_snapshot_request(aclk_host_config);
        rrdhost_flag_set(aclk_host_config->host, RRDHOST_FLAG_ACLK_STREAM_ALERTS);
    }
    else
        nd_log(
            NDLS_ACCESS,
            NDLP_DEBUG,
            "Alert check on \"%s\", node \"%s\" (version: cloud %llu, local %llu)",
            rrdhost_hostname(aclk_host_config->host),
            node_id,
            (unsigned long long)cloud_version,
            (unsigned long long)local_version);
    aclk_host_config->checkpoint_count++;
}

// The snapshot rows and their count come from ONE statement: two statements on the shared db_meta connection are not
// isolated from the other threads writing to it, so a separate COUNT could disagree with the rows sent, telling the
// cloud to expect chunks it never receives (or sending a chunk number twice).
#define SQL_GET_SNAPSHOT_ENTRIES                                                                                       \
    " WITH snapshot AS MATERIALIZED ("                                                                                 \
    " SELECT 0, hld.unique_id, hld.alarm_id, hl.config_hash_id, hld.updated_by_id, hld.when_key, "                     \
    " hld.duration, hld.non_clear_duration, hld.flags, hld.exec_run_timestamp, hld.delay_up_to_timestamp, hl.name,  "  \
    " hl.chart, hl.exec, hl.recipient, ah.source, hl.units, hld.info, hld.exec_code, hld.new_status,  "                \
    " hld.old_status, hld.delay, hld.new_value, hld.old_value, hld.last_repeat, hl.chart_context, hld.transition_id, " \
    " hld.alarm_event_id, hl.chart_name, hld.summary, hld.health_log_id, av.version "                                  \
    " FROM health_log hl, alert_hash ah, health_log_detail hld, alert_version av "                                     \
    " WHERE hl.config_hash_id = ah.hash_id"                                                                            \
    " AND hl.host_id = @host_id AND hl.health_log_id = hld.health_log_id "                                             \
    " AND hld.health_log_id = av.health_log_id AND av.unique_id = hld.unique_id AND av.status <> -2)"                  \
    " SELECT snapshot.*, COUNT(*) OVER () FROM snapshot"

#define SNAPSHOT_ENTRIES_COUNT_COLUMN 32

// True when the host has alerts in memory (a disconnected child has none: they are deleted at the disconnect).
// Archiving a host destroys the alert index under the write lock, so the try-read keeps this count away from it;
// when the lock is busy, assume there are alerts.
static bool host_has_alerts(RRDHOST *host)
{
    if (!rw_spinlock_tryread_lock(&host->metadata_lifetime_lock))
        return true;

    bool has_alerts = dictionary_entries(host->rrdcalc_root_index) > 0;
    rw_spinlock_read_unlock(&host->metadata_lifetime_lock);
    return has_alerts;
}

#define ALARM_EVENTS_PER_CHUNK 1000
static void send_snapshot_if_online(alarm_snapshot_proto_ptr_t snapshot)
{
    if (aclk_online_for_alerts())
        aclk_send_alarm_snapshot(snapshot);
    else
        destroy_alarm_snapshot_proto(snapshot);
}

static void send_alert_snapshot_to_cloud(RRDHOST *host, void (*send_snapshot)(alarm_snapshot_proto_ptr_t))
{
    if (unlikely(!host)) {
        nd_log(NDLS_ACCESS, NDLP_WARNING, "AC [N/A (N/A)]: Node id not found");
        return;
    }

    struct aclk_sync_cfg_t *aclk_host_config = __atomic_load_n(&host->aclk_host_config, __ATOMIC_ACQUIRE);
    if (unlikely(!aclk_host_config))
        return;

    CLAIM_ID claim_id = claim_id_get();
    if (unlikely(!claim_id_is_set(claim_id)))
        return;

    char node_id[UUID_STR_LEN];
    aclk_node_id_copy(aclk_host_config, node_id);

    sqlite3_stmt *res = NULL;
    if (!PREPARE_STATEMENT(db_meta, SQL_GET_SNAPSHOT_ENTRIES, &res))
        return;

    int param = 0;
    SQLITE_BIND_FAIL(done, sqlite3_bind_blob(res, ++param, &host->host_id.uuid, sizeof(host->host_id.uuid), SQLITE_STATIC));

    param = 0;
    // the first row carries the number of alerts in the snapshot; no row means the host has no alert to report,
    // which is sent too: the cloud takes a snapshot as the node's complete alert state, so an empty one clears it
    // (e.g. a disconnected child whose alerts are all REMOVED). A failed query sends nothing.
    int rc = sqlite3_step_monitored(res);
    if (rc != SQLITE_ROW && rc != SQLITE_DONE) {
        error_report("Failed to read the alert snapshot of host %s, rc = %d", rrdhost_hostname(host), rc);
        goto done;
    }

    // An empty health log view proves nothing while the host still has alerts in memory: a REMOVED saved after a
    // re-created alert's first transition leaves the live alert REMOVED in the health log. Clearing the cloud would
    // hide it, so send nothing then. The caller still completes the request, as it always did for an empty view:
    // retrying would rebuild the version table on every pass for as long as the health log stays stale, and the next
    // cloud checkpoint requests a new snapshot if the versions still differ.
    if (rc == SQLITE_DONE && host_has_alerts(host))
        goto done;

    int cnt = (rc == SQLITE_ROW) ? sqlite3_column_int(res, SNAPSHOT_ENTRIES_COUNT_COLUMN) : 0;

    nd_uuid_t local_snapshot_uuid;
    char snapshot_uuid_str[UUID_STR_LEN];
    uuid_generate_random(local_snapshot_uuid);
    uuid_unparse_lower(local_snapshot_uuid, snapshot_uuid_str);
    char *snapshot_uuid = &snapshot_uuid_str[0];

    nd_log(NDLS_ACCESS, NDLP_DEBUG,
        "ACLK REQ [%s (%s)]: Sending %d alerts snapshot, snapshot_uuid %s",
        node_id, rrdhost_hostname(host),
        cnt, snapshot_uuid);

    // an empty snapshot is one chunk without alerts
    uint32_t chunks = cnt ? (cnt / ALARM_EVENTS_PER_CHUNK) + (cnt % ALARM_EVENTS_PER_CHUNK != 0) : 1;

    alarm_snapshot_proto_ptr_t snapshot_proto = NULL;
    struct alarm_snapshot alarm_snap;
    struct alarm_log_entry alarm_log;

    alarm_snap.node_id = node_id;
    alarm_snap.claim_id = claim_id.str;
    alarm_snap.snapshot_uuid = snapshot_uuid;
    alarm_snap.chunks = chunks;
    alarm_snap.chunk = 1;

    alarm_log.node_id = node_id;
    alarm_log.claim_id = claim_id.str;

    uint64_t version = 0;
    int total_count = 0;

    // no alerts: send the single empty chunk
    if (rc == SQLITE_DONE) {
        snapshot_proto = generate_alarm_snapshot_proto(&alarm_snap);
        send_snapshot(snapshot_proto);
        goto created;
    }

    cnt = 0;
    do {
        cnt++;
        total_count++;

        if (!snapshot_proto)
            snapshot_proto = generate_alarm_snapshot_proto(&alarm_snap);

        health_alarm_log_populate(&alarm_log, res, host, NULL);

        add_alarm_log_entry2snapshot(snapshot_proto, &alarm_log);
        version += alarm_log.version;

        if (cnt == ALARM_EVENTS_PER_CHUNK) {
            send_snapshot(snapshot_proto);
            snapshot_proto = NULL;
            cnt = 0;
            if (alarm_snap.chunk < chunks) {
                alarm_snap.chunk++;
                snapshot_proto = generate_alarm_snapshot_proto(&alarm_snap);
            }
        }
        destroy_alarm_log_entry(&alarm_log);
    } while (sqlite3_step_monitored(res) == SQLITE_ROW);

    if (cnt)
        send_snapshot(snapshot_proto);
    else
        destroy_alarm_snapshot_proto(snapshot_proto);

created:
    nd_log(
        NDLS_ACCESS,
        NDLP_DEBUG,
        "ACLK REQ [%s (%s)]: Created snapshot %s with %d alerts (version = %llu)",
        node_id,
        rrdhost_hostname(host),
        snapshot_uuid,
        total_count,
        (long long unsigned)version);

done:
    REPORT_BIND_FAIL(res, param);
    SQLITE_FINALIZE(res);
}

// ---------------------------------------------------------------------------
// Unit test: netdata -W alertqueuetest
//
// The submit queue coalesces by re-pointing an existing row: the insert is
// ON CONFLICT(host_id, health_log_id) DO UPDATE SET unique_id=..., and DO UPDATE
// does not move sequence_id. So a transition that is queued while a batch is in
// flight lands on a row the push has already read and sent. Whatever removes the
// sent rows afterwards MUST therefore name the transition it actually sent, or it
// destroys the newer one and the cloud keeps a status the agent has already left.

static int aq_test_queue(nd_uuid_t *host_id, int64_t *sequence_id, int64_t *unique_id)
{
    sqlite3_stmt *res = NULL;
    int count = 0;

    if (!PREPARE_STATEMENT(
            db_meta, "SELECT sequence_id, unique_id FROM aclk_queue WHERE host_id = @host_id ORDER BY sequence_id", &res))
        return -1;

    int param = 0;
    SQLITE_BIND_FAIL(done, sqlite3_bind_blob(res, ++param, host_id, sizeof(*host_id), SQLITE_STATIC));

    param = 0;
    while (sqlite3_step_monitored(res) == SQLITE_ROW) {
        if (!count) {
            if (sequence_id)
                *sequence_id = sqlite3_column_int64(res, 0);
            if (unique_id)
                *unique_id = sqlite3_column_int64(res, 1);
        }
        count++;
    }

done:
    REPORT_BIND_FAIL(res, param);
    SQLITE_FINALIZE(res);
    return count;
}

// queue a pending transition the way SQL_INSERT_ALERT_PENDING_QUEUE does, due `due_in_s` from now, without
// touching the host's next-due gate
static bool aq_pending_seed(nd_uuid_t *host_id, int64_t health_log_id, int64_t unique_id, int64_t due_in_s)
{
    sqlite3_stmt *res = NULL;
    bool ok = false;

    if (!PREPARE_STATEMENT(
            db_meta,
            "INSERT INTO alert_queue (host_id, health_log_id, unique_id, alarm_id, status, date_scheduled)"
            " VALUES (@host_id, @health_log_id, @unique_id, @health_log_id, 3, UNIXEPOCH() + @due_in_s)",
            &res))
        return false;

    int param = 0;
    SQLITE_BIND_FAIL(done, sqlite3_bind_blob(res, ++param, host_id, sizeof(*host_id), SQLITE_STATIC));
    SQLITE_BIND_FAIL(done, sqlite3_bind_int64(res, ++param, health_log_id));
    SQLITE_BIND_FAIL(done, sqlite3_bind_int64(res, ++param, unique_id));
    SQLITE_BIND_FAIL(done, sqlite3_bind_int64(res, ++param, due_in_s));

    param = 0;
    ok = (sqlite3_step_monitored(res) == SQLITE_DONE);

done:
    REPORT_BIND_FAIL(res, param);
    SQLITE_FINALIZE(res);
    return ok;
}

static int aq_pending_rows(nd_uuid_t *host_id)
{
    sqlite3_stmt *res = NULL;
    int count = -1;

    if (!PREPARE_STATEMENT(db_meta, "SELECT COUNT(*) FROM alert_queue WHERE host_id = @host_id", &res))
        return -1;

    int param = 0;
    SQLITE_BIND_FAIL(done, sqlite3_bind_blob(res, ++param, host_id, sizeof(*host_id), SQLITE_STATIC));

    param = 0;
    if (sqlite3_step_monitored(res) == SQLITE_ROW)
        count = sqlite3_column_int(res, 0);

done:
    REPORT_BIND_FAIL(res, param);
    SQLITE_FINALIZE(res);
    return count;
}

static time_t aq_pending_due(nd_uuid_t *host_id, int64_t health_log_id)
{
    sqlite3_stmt *res = NULL;
    time_t due = 0;

    if (!PREPARE_STATEMENT(
            db_meta,
            "SELECT date_scheduled FROM alert_queue WHERE host_id = @host_id AND health_log_id = @health_log_id",
            &res))
        return 0;

    int param = 0;
    SQLITE_BIND_FAIL(done, sqlite3_bind_blob(res, ++param, host_id, sizeof(*host_id), SQLITE_STATIC));
    SQLITE_BIND_FAIL(done, sqlite3_bind_int64(res, ++param, health_log_id));

    param = 0;
    if (sqlite3_step_monitored(res) == SQLITE_ROW)
        due = (time_t)sqlite3_column_int64(res, 0);

done:
    REPORT_BIND_FAIL(res, param);
    SQLITE_FINALIZE(res);
    return due;
}

static int64_t aq_cached_pending_row(RRDHOST *host, int64_t *unique_id)
{
    sqlite3_stmt *res = NULL;
    int64_t row = 0;
    if (PREPARE_STATEMENT(db_meta, SQL_PROCESS_ALERT_PENDING_QUEUE, &res)) {
        if (sqlite3_bind_blob(res, 1, &host->host_id.uuid, sizeof(host->host_id.uuid), SQLITE_STATIC) == SQLITE_OK &&
            sqlite3_step_monitored(res) == SQLITE_ROW) {
            *unique_id = sqlite3_column_int64(res, 1);
            row = sqlite3_column_int64(res, 3);
        }
        SQLITE_FINALIZE(res);
    }
    return row;
}

static int64_t aq_version_unique_id(int64_t health_log_id)
{
    sqlite3_stmt *res = NULL;
    int64_t unique_id = 0;
    if (PREPARE_STATEMENT(db_meta, "SELECT unique_id FROM alert_version WHERE health_log_id = ?1", &res)) {
        if (sqlite3_bind_int64(res, 1, health_log_id) == SQLITE_OK && sqlite3_step_monitored(res) == SQLITE_ROW)
            unique_id = sqlite3_column_int64(res, 0);
        SQLITE_FINALIZE(res);
    }
    return unique_id;
}

// A failed seed used to surface as a confusing queue-state mismatch three assertions later, so every
// bind/step in the fixtures reports itself with its rc and the sqlite message.
#define AQ_SQL(expr, what)                                                                                             \
    do {                                                                                                               \
        int _rc = (expr);                                                                                              \
        if (_rc != SQLITE_OK && _rc != SQLITE_DONE && _rc != SQLITE_ROW) {                                             \
            netdata_log_error(                                                                                         \
                "ALERT QUEUE TEST: fixture step \"%s\" failed: rc=%d (%s)", what, _rc, sqlite3_errmsg(db_meta));       \
            failures++;                                                                                                \
        }                                                                                                              \
    } while (0)

#define AQ_CHECK(condition, fmt, ...)                                                                                  \
    do {                                                                                                               \
        if (!(condition)) {                                                                                            \
            netdata_log_error("ALERT QUEUE TEST FAILED: " fmt, ##__VA_ARGS__);                                         \
            failures++;                                                                                                \
        }                                                                                                              \
        else                                                                                                           \
            netdata_log_info("ALERT QUEUE TEST ok: " fmt, ##__VA_ARGS__);                                              \
    } while (0)

// Persist through the production writer: this also queues the transition and advances last_transition_id,
// so the snapshot tests observe the same health-log state as a real removal/recovery.
static int64_t aq_save_transition_for_alarm(
    RRDHOST *host, uint32_t alarm_id, uint32_t unique_id, RRDCALC_STATUS old_status, RRDCALC_STATUS new_status,
    uint32_t flags, time_t when)
{
    ALARM_ENTRY ae = {
        .alarm_id = alarm_id,
        .unique_id = unique_id,
        .alarm_event_id = unique_id,
        .old_status = old_status,
        .new_status = new_status,
        .flags = flags,
        .when = when,
        .name = string_strdupz("bounce_alert"),
        .chart = string_strdupz("bounce.chart"),
    };
    uuid_copy(ae.config_hash_id, host->host_id.uuid);
    uuid_generate(ae.transition_id);
    sql_health_alarm_log_save(host, &ae);
    string_freez(ae.name);
    string_freez(ae.chart);
    if (!(ae.flags & HEALTH_ENTRY_FLAG_SAVED))
        return 0;

    sqlite3_stmt *res = NULL;
    int64_t id = 0;
    if (PREPARE_STATEMENT(
            db_meta, "SELECT health_log_id FROM health_log WHERE host_id = ?1 AND alarm_id = ?2", &res)) {
        if (sqlite3_bind_blob(res, 1, &host->host_id.uuid, sizeof(host->host_id.uuid), SQLITE_STATIC) == SQLITE_OK &&
            sqlite3_bind_int64(res, 2, alarm_id) == SQLITE_OK &&
            sqlite3_step_monitored(res) == SQLITE_ROW)
            id = sqlite3_column_int64(res, 0);
        SQLITE_FINALIZE(res);
    }
    return id;
}

static int64_t aq_save_transition(
    RRDHOST *host, uint32_t unique_id, RRDCALC_STATUS old_status, RRDCALC_STATUS new_status,
    uint32_t flags, time_t when)
{
    return aq_save_transition_for_alarm(host, 100, unique_id, old_status, new_status, flags, when);
}

static bool aq_seed_alert_config(RRDHOST *host)
{
    sqlite3_stmt *res = NULL;
    bool ok = false;
    if (PREPARE_STATEMENT(db_meta, "INSERT INTO alert_hash (hash_id, warn) VALUES (?1, '$this > 1')", &res)) {
        ok = sqlite3_bind_blob(res, 1, &host->host_id.uuid, sizeof(host->host_id.uuid), SQLITE_STATIC) == SQLITE_OK &&
             sqlite3_step_monitored(res) == SQLITE_DONE;
        SQLITE_FINALIZE(res);
    }
    return ok;
}

// Read the exact snapshot query without sending to ACLK; zero rows is the empty snapshot that clears Cloud.
static int aq_snapshot_rows(RRDHOST *host)
{
    sqlite3_stmt *res = NULL;
    int rows = -1;
    if (PREPARE_STATEMENT(db_meta, SQL_GET_SNAPSHOT_ENTRIES, &res)) {
        if (sqlite3_bind_blob(res, 1, &host->host_id.uuid, sizeof(host->host_id.uuid), SQLITE_STATIC) == SQLITE_OK) {
            rows = 0;
            int rc;
            while ((rc = sqlite3_step_monitored(res)) == SQLITE_ROW)
                rows++;
            if (rc != SQLITE_DONE)
                rows = -1;
        }
        SQLITE_FINALIZE(res);
    }
    return rows;
}

static unsigned aq_emitted_events;
static unsigned aq_emitted_snapshots;
static int64_t aq_last_alarm_id;
static enum aclk_alarm_status aq_last_status;

static ACLK_ALARM_LOG_SEND_RESULT aq_record_send(struct alarm_log_entry *entry)
{
    aq_emitted_events++;
    aq_last_alarm_id = entry->alarm_id;
    aq_last_status = entry->status;
    return ACLK_ALARM_LOG_SENT;
}

static void aq_record_snapshot(alarm_snapshot_proto_ptr_t snapshot)
{
    aq_emitted_snapshots++;
    destroy_alarm_snapshot_proto(snapshot);
}

static int aq_deny_alert_version_read(
    void *data __maybe_unused, int action, const char *table, const char *column __maybe_unused,
    const char *database __maybe_unused, const char *source __maybe_unused)
{
    return action == SQLITE_READ && table && !strcmp(table, "alert_version") ? SQLITE_DENY : SQLITE_OK;
}

static void aq_hold_snapshot_during_rebuild(sqlite3_context *context, int argc __maybe_unused, sqlite3_value **argv __maybe_unused)
{
    RRDHOST *host = sqlite3_user_data(context);
    // Model a metadata save starting after the initial eligibility check but before the checkpoint commits.
    __atomic_store_n(&host->health.pending_transitions, 1, __ATOMIC_RELEASE);
    sqlite3_result_int(context, 0);
}

// Each bounce fixture has one alert. Exercise the production sender decision and acknowledgement with a
// recording transport; an empty queue or a suppressed duplicate emits zero messages.
static int aq_send_one(RRDHOST *host)
{
    sqlite3_stmt *res = NULL, *version = NULL, *remove = NULL;
    if (!PREPARE_STATEMENT(db_meta, SQL_SELECT_ALERT_TO_DUMMY, &res))
        return -1;
    if (sqlite3_bind_blob(res, 1, &host->host_id.uuid, sizeof(host->host_id.uuid), SQLITE_STATIC) != SQLITE_OK) {
        SQLITE_FINALIZE(res);
        return -1;
    }
    int rc = sqlite3_step_monitored(res);
    struct alarm_log_entry entry = {0};
    RRDCALC_STATUS status = RRDCALC_STATUS_UNINITIALIZED;
    if (rc == SQLITE_ROW) {
        entry.sequence_id = sqlite3_column_int64(res, 0);
        entry.unique_id = sqlite3_column_int64(res, 1);
        entry.version = sqlite3_column_int64(res, 2);
        status = sqlite3_column_int(res, 3);
        entry.health_log_id = sqlite3_column_int64(res, 4);
    }
    SQLITE_FINALIZE(res);
    if (rc != SQLITE_ROW)
        return rc == SQLITE_DONE ? 0 : -1;

    bool sent;
    unsigned before = aq_emitted_events;
    bool ok = send_alert_transition_if_needed(&entry, status, aq_record_send, &version, &sent);
    if (ok)
        ok = delete_alert_from_submit_queue(&host->host_id.uuid, entry.sequence_id, entry.unique_id, &remove);
    SQLITE_FINALIZE(version);
    SQLITE_FINALIZE(remove);
    return ok ? (int)(aq_emitted_events - before) : -1;
}

int alert_queue_unittest(void)
{
    int failures = 0;

    // -W options are parsed before any database is opened, so db_meta is NULL here and this test owns the
    // sqlite lifecycle. The save/restore is kept for the case where that stops being true - but note that
    // sql_init_meta_database() would then overwrite the global without closing what it replaced.
    sqlite3 *saved_db_meta = db_meta;
    bool owns_sqlite_lifecycle = !saved_db_meta;

    if (sqlite_library_init())
        return 1;

    // the second argument is `memory`: this opens ":memory:" (sqlite_metadata.c), never the on-disk
    // netdata-meta.db. That is what makes the unqualified DELETEs further down safe.
    if (sql_init_meta_database(DB_CHECK_NONE, 1) != SQLITE_OK) {
        sql_close_database(db_meta, "METADATA");
        db_meta = saved_db_meta;
        if (owns_sqlite_lifecycle)
            sqlite_library_shutdown();
        return 1;
    }

    nd_uuid_t host_id;
    uuid_generate(host_id);

    const int64_t health_log_id = 1;
    const uint32_t sent_unique_id = 1001;
    const uint32_t queued_during_send_unique_id = 1002;

    int64_t sequence_id = 0, unique_id = 0;
    sqlite3_stmt *res_delete = NULL;

    // the health loop queues a transition
    (void)insert_alert_to_submit_queue(&host_id, health_log_id, sent_unique_id, RRDCALC_STATUS_CLEAR);
    AQ_CHECK(
        aq_test_queue(&host_id, &sequence_id, &unique_id) == 1 && unique_id == (int64_t)sent_unique_id,
        "one row is queued, holding the transition to send (unique_id=%" PRId64 ", expected %u)",
        unique_id, sent_unique_id);

    // the push reads and sends it; while it is in flight the alert transitions
    // again, and the insert re-points the very same row
    (void)insert_alert_to_submit_queue(&host_id, health_log_id, queued_during_send_unique_id, RRDCALC_STATUS_WARNING);
    int64_t re_pointed_sequence_id = 0;
    AQ_CHECK(
        aq_test_queue(&host_id, &re_pointed_sequence_id, &unique_id) == 1 &&
            unique_id == (int64_t)queued_during_send_unique_id && re_pointed_sequence_id == sequence_id,
        "the newer transition re-points the same row, keeping its sequence_id "
        "(unique_id=%" PRId64 " expected %u, sequence_id=%" PRId64 " expected %" PRId64 ")",
        unique_id, queued_during_send_unique_id, re_pointed_sequence_id, sequence_id);

    // the push now removes what it sent
    (void)delete_alert_from_submit_queue(&host_id, sequence_id, sent_unique_id, &res_delete);

    // THE CONTRACT: only the transition that was sent may be removed
    int rows = aq_test_queue(&host_id, NULL, &unique_id);
    AQ_CHECK(
        rows == 1 && unique_id == (int64_t)queued_during_send_unique_id,
        "the transition queued during the send survives and is still pending "
        "(rows=%d, unique_id=%" PRId64 ", expected %u)",
        rows, unique_id, queued_during_send_unique_id);

    // and the row does go away once its own transition has been sent
    (void)delete_alert_from_submit_queue(&host_id, sequence_id, queued_during_send_unique_id, &res_delete);
    rows = aq_test_queue(&host_id, NULL, NULL);
    AQ_CHECK(rows == 0, "the queue is empty once the newer transition is sent too (rows=%d)", rows);

    // a row whose transition no longer exists can never be sent, so it must be reaped - but only once it is
    // old enough that we cannot be racing its insert
    (void)insert_alert_to_submit_queue(&host_id, health_log_id, 2001, RRDCALC_STATUS_CLEAR);
    AQ_CHECK(aq_test_queue(&host_id, NULL, NULL) == 1, "an unsendable row is queued");
    AQ_CHECK(
        aclk_queue_reap_unsendable(ACLK_QUEUE_REAP_AGE_S) == 0 && aq_test_queue(&host_id, NULL, NULL) == 1,
        "the reaper leaves a freshly inserted row alone");

    (void)db_execute(db_meta, "UPDATE aclk_queue SET date_created = UNIXEPOCH() - 7200", NULL);
    AQ_CHECK(
        aclk_queue_reap_unsendable(ACLK_QUEUE_REAP_AGE_S) == 1 && aq_test_queue(&host_id, NULL, NULL) == 0,
        "the reaper removes an aged row whose transition no longer exists");


    // --- the pending queue (alert_queue) carries the same defect and the same guard. Its conflict key is
    // (host_id, health_log_id, alarm_id), NOT the submit queue's key, so it needs its own case. The seeding
    // INSERT mirrors SQL_INSERT_ALERT_PENDING_QUEUE in sqlite_health.c.
    {
        const char *pending_insert =
            "INSERT INTO alert_queue (host_id, health_log_id, unique_id, alarm_id, status, date_scheduled)"
            "  VALUES (@host_id, 7, @unique_id, 7, 3, UNIXEPOCH())"
            " ON CONFLICT (host_id, health_log_id, alarm_id)"
            " DO UPDATE SET status = excluded.status, unique_id = excluded.unique_id,"
            " date_scheduled = MIN(date_scheduled, excluded.date_scheduled)";
        sqlite3_stmt *ins = NULL;
        int64_t rowid = 0, pending_unique = 0;

        for (int pass = 0; pass < 2; pass++) {
            if (!PREPARE_STATEMENT(db_meta, pending_insert, &ins))
                break;
            AQ_SQL(sqlite3_bind_blob(ins, 1, &host_id, sizeof(host_id), SQLITE_STATIC), "pending seed host_id");
            AQ_SQL(sqlite3_bind_int64(ins, 2, pass ? 3002 : 3001), "pending seed unique_id");
            AQ_SQL(sqlite3_step_monitored(ins), "pending seed insert");
            SQLITE_FINALIZE(ins);
            ins = NULL;
        }

        sqlite3_stmt *sel = NULL;
        if (PREPARE_STATEMENT(db_meta, "SELECT rowid, unique_id FROM alert_queue WHERE host_id = @host_id", &sel)) {
            AQ_SQL(sqlite3_bind_blob(sel, 1, &host_id, sizeof(host_id), SQLITE_STATIC), "pending read host_id");
            if (sqlite3_step_monitored(sel) == SQLITE_ROW) {
                rowid = sqlite3_column_int64(sel, 0);
                pending_unique = sqlite3_column_int64(sel, 1);
            }
            SQLITE_FINALIZE(sel);
        }
        AQ_CHECK(
            pending_unique == 3002,
            "pending queue: the newer transition re-points the same row (unique_id=%" PRId64 ", expected 3002)",
            pending_unique);

        // drain the row as process_alert_pending_queue() would, naming the transition it read (the older one)
        delete_alert_from_pending_queue(&host_id, rowid, 3001);

        int64_t still = 0;
        if (PREPARE_STATEMENT(db_meta, "SELECT COUNT(*) FROM alert_queue WHERE host_id = @host_id", &sel)) {
            AQ_SQL(sqlite3_bind_blob(sel, 1, &host_id, sizeof(host_id), SQLITE_STATIC), "pending count host_id");
            if (sqlite3_step_monitored(sel) == SQLITE_ROW)
                still = sqlite3_column_int64(sel, 0);
            SQLITE_FINALIZE(sel);
        }
        AQ_CHECK(
            still == 1, "pending queue: the transition queued during the drain survives (rows=%" PRId64 ", expected 1)",
            still);
        (void)db_execute(db_meta, "DELETE FROM alert_queue", NULL);
    }

    // --- the push SELECT must never hand back more than one batch, whatever is queued. This is the runtime
    // assertion SOW-20260921-aclk-alert-push-rate-25 could not make for itself: nothing reaches
    // aclk_push_alert_event() on an unclaimed agent, so the batch limit is only observable from here.
    {
        const int limit = atoi(ACLK_MAX_ALERT_UPDATES);
        const int seed = limit + 7;
        nd_uuid_t config_hash;
        uuid_generate(config_hash);
        sqlite3_stmt *st = NULL;
        int queued_rows = 0, returned = 0;

        (void)db_execute(db_meta, "DELETE FROM aclk_queue", NULL);

        // warn must be non-NULL: a config with neither warn nor crit is a variable config, and
        // insert_alert_to_submit_queue() deliberately never queues those
        if (PREPARE_STATEMENT(db_meta, "INSERT INTO alert_hash (hash_id, warn) VALUES (@hash, '$this > 1')", &st)) {
            AQ_SQL(sqlite3_bind_blob(st, 1, &config_hash, sizeof(config_hash), SQLITE_STATIC), "alert_hash seed");
            AQ_SQL(sqlite3_step_monitored(st), "alert_hash insert");
            SQLITE_FINALIZE(st);
            st = NULL;
        }

        for (int i = 1; i <= seed; i++) {
            if (PREPARE_STATEMENT(
                    db_meta,
                    "INSERT INTO health_log (health_log_id, host_id, alarm_id, config_hash_id, name, chart)"
                    " VALUES (@id, @host_id, @id, @hash, 'alert', 'chart')", &st)) {
                AQ_SQL(sqlite3_bind_int64(st, 1, i), "health_log seed id");
                AQ_SQL(sqlite3_bind_blob(st, 2, &host_id, sizeof(host_id), SQLITE_STATIC), "health_log seed host_id");
                AQ_SQL(sqlite3_bind_blob(st, 3, &config_hash, sizeof(config_hash), SQLITE_STATIC), "health_log seed hash");
                AQ_SQL(sqlite3_step_monitored(st), "health_log insert");
                SQLITE_FINALIZE(st);
                st = NULL;
            }
            if (PREPARE_STATEMENT(
                    db_meta,
                    "INSERT INTO health_log_detail (health_log_id, unique_id, alarm_id, alarm_event_id,"
                    " updated_by_id, when_key, new_status, old_status) "
                    " VALUES (@id, @uid, @id, 1, 0, UNIXEPOCH(), 3, 1)", &st)) {
                AQ_SQL(sqlite3_bind_int64(st, 1, i), "health_log_detail seed id");
                AQ_SQL(sqlite3_bind_int64(st, 2, 5000 + i), "health_log_detail seed unique_id");
                AQ_SQL(sqlite3_step_monitored(st), "health_log_detail insert");
                SQLITE_FINALIZE(st);
                st = NULL;
            }
            if (!insert_alert_to_submit_queue(&host_id, i, (uint32_t)(5000 + i), RRDCALC_STATUS_WARNING))
                queued_rows++;
        }
        AQ_CHECK(queued_rows == seed, "seeded %d queue rows (one batch is %d)", queued_rows, limit);

        if (PREPARE_STATEMENT(db_meta, SQL_SELECT_ALERT_TO_PUSH, &st)) {
            AQ_SQL(sqlite3_bind_blob(st, 1, &host_id, sizeof(host_id), SQLITE_STATIC), "push SELECT host_id");
            while (sqlite3_step_monitored(st) == SQLITE_ROW)
                returned++;
            SQLITE_FINALIZE(st);
            st = NULL;
        }
        AQ_CHECK(
            returned == limit,
            "the push SELECT returns exactly one batch of %d with %d queued (got %d)", limit, seed, returned);

        (void)db_execute(db_meta, "DELETE FROM aclk_queue", NULL);
    }

    // --- the pending queue is read only when something in it can be due (health.alert_queue_next_due_s).
    // The health loop calls process_alert_pending_queue() for every host on every pass; an idle host must not
    // reach SQLite, and no queued row may be stranded by that.
    {
        RRDHOST *h = callocz(1, sizeof(*h));
        uuid_generate(h->host_id.uuid);
        nd_uuid_t *hid = &h->host_id.uuid;
        time_t *gate = &h->health.alert_queue_next_due_s;
        time_t now = now_realtime_sec();

        (void)db_execute(db_meta, "DELETE FROM alert_queue", NULL);
        (void)db_execute(db_meta, "DELETE FROM aclk_queue", NULL);

        // a row left by a previous run: a new host has never been checked, so the first call reads the queue
        AQ_SQL(aq_pending_seed(hid, 11, 4001, 0) ? SQLITE_DONE : SQLITE_ERROR, "gate seed 11");
        (void)process_alert_pending_queue(h);
        rows = aq_pending_rows(hid);
        AQ_CHECK(
            rows == 0 && *gate == nd_time_t_max(),
            "gate: a row persisted by a previous run is drained on the first call, then the gate parks (rows=%d)", rows);

        // written behind the gate's back: an idle host does not read the queue
        AQ_SQL(aq_pending_seed(hid, 12, 4002, 0) ? SQLITE_DONE : SQLITE_ERROR, "gate seed 12");
        (void)process_alert_pending_queue(h);
        rows = aq_pending_rows(hid);
        AQ_CHECK(rows == 1, "gate: an idle host does not read the queue (rows=%d, expected 1)", rows);

        // a writer lowers the gate: the due row drains, the future one stays and becomes the next due time
        AQ_SQL(aq_pending_seed(hid, 13, 4003, 600) ? SQLITE_DONE : SQLITE_ERROR, "gate seed 13");
        alert_queue_next_due_lower(h, now + 600);
        alert_queue_next_due_lower(h, now);
        (void)process_alert_pending_queue(h);
        rows = aq_pending_rows(hid);
        AQ_CHECK(
            rows == 1 && *gate >= now + 600 && *gate != nd_time_t_max(),
            "gate: due rows drain and the next due time is the earliest one still queued (rows=%d, due in %lld s)",
            rows, (long long)(*gate - now));

        // ... and is not read again before then
        AQ_SQL(aq_pending_seed(hid, 14, 4004, 0) ? SQLITE_DONE : SQLITE_ERROR, "gate seed 14");
        (void)process_alert_pending_queue(h);
        rows = aq_pending_rows(hid);
        AQ_CHECK(rows == 2, "gate: the queue is not read before its next due time (rows=%d, expected 2)", rows);

        // a due row the drain could not remove (e.g. re-pointed while it was read) keeps the gate due
        (void)db_execute(db_meta, "DELETE FROM alert_queue", NULL);
        (void)db_execute(db_meta, "CREATE TRIGGER aq_keep BEFORE DELETE ON alert_queue BEGIN SELECT RAISE(IGNORE); END", NULL);
        AQ_SQL(aq_pending_seed(hid, 15, 4005, 0) ? SQLITE_DONE : SQLITE_ERROR, "gate seed 15");
        alert_queue_next_due_lower(h, now);
        (void)process_alert_pending_queue(h);
        rows = aq_pending_rows(hid);
        AQ_CHECK(
            rows == 1 && *gate <= now_realtime_sec(),
            "gate: a row left behind by the drain keeps the gate due (rows=%d, due in %lld s)",
            rows, (long long)(*gate - now_realtime_sec()));
        (void)db_execute(db_meta, "DROP TRIGGER aq_keep", NULL);
        (void)process_alert_pending_queue(h);
        rows = aq_pending_rows(hid);
        AQ_CHECK(
            rows == 0 && *gate == nd_time_t_max(),
            "gate: the row left behind is drained on the next call (rows=%d)", rows);

        // a transition that could not be submitted is kept and retried after ALERT_QUEUE_RETRY_S
        h->aclk_host_config = callocz(1, sizeof(*h->aclk_host_config));
        (void)db_execute(db_meta, "CREATE TRIGGER aq_fail BEFORE INSERT ON aclk_queue BEGIN SELECT RAISE(ABORT, 'test'); END", NULL);
        AQ_SQL(aq_pending_seed(hid, 16, 4006, 0) ? SQLITE_DONE : SQLITE_ERROR, "gate seed 16");
        now = now_realtime_sec();
        alert_queue_next_due_lower(h, now);
        bool queued = process_alert_pending_queue(h);
        rows = aq_pending_rows(hid);
        AQ_CHECK(
            !queued && rows == 1 && *gate > now && *gate <= now + ALERT_QUEUE_RETRY_S + 1,
            "gate: a transition the submit queue refused is kept and retried later (rows=%d, due in %lld s)",
            rows, (long long)(*gate - now));
        (void)db_execute(db_meta, "DROP TRIGGER aq_fail", NULL);
        *gate = 0; // the retry time has come
        queued = process_alert_pending_queue(h);
        rows = aq_pending_rows(hid);
        int submitted = aq_test_queue(hid, NULL, NULL);
        AQ_CHECK(
            queued && rows == 0 && submitted == 1,
            "gate: the kept transition is submitted on retry (pending=%d, submitted=%d)", rows, submitted);

        (void)db_execute(db_meta, "DELETE FROM alert_queue", NULL);
        (void)db_execute(db_meta, "DELETE FROM aclk_queue", NULL);
        freez(h->aclk_host_config);
        freez(h);
    }

    // --- a child that disconnects is usually back within minutes (restart, update): its REMOVED transitions get the
    // long delay, so a reconnect overwrites them before they are due; other removals keep their delays
    {
        AQ_CHECK(
            calculate_delay(RRDCALC_STATUS_WARNING, RRDCALC_STATUS_REMOVED, 0) == ALERT_TRANSITION_DELAY_SHORT,
            "delay: a raised alert that is removed keeps the short delay");
        AQ_CHECK(
            calculate_delay(RRDCALC_STATUS_WARNING, RRDCALC_STATUS_REMOVED, HEALTH_ENTRY_FLAG_CHILD_DISCONNECTED) ==
                ALERT_TRANSITION_DELAY_LONG &&
            calculate_delay(RRDCALC_STATUS_CRITICAL, RRDCALC_STATUS_REMOVED, HEALTH_ENTRY_FLAG_CHILD_DISCONNECTED) ==
                ALERT_TRANSITION_DELAY_LONG,
            "delay: a raised alert of a disconnected child gets the long delay");
        AQ_CHECK(
            calculate_delay(RRDCALC_STATUS_CLEAR, RRDCALC_STATUS_REMOVED, HEALTH_ENTRY_FLAG_CHILD_DISCONNECTED) ==
                ALERT_TRANSITION_DELAY_LONG,
            "delay: a clear alert of a disconnected child keeps the long delay");
        AQ_CHECK(
            calculate_delay(RRDCALC_STATUS_REMOVED, RRDCALC_STATUS_WARNING, HEALTH_ENTRY_FLAG_CHILD_DISCONNECTED) ==
                ALERT_TRANSITION_DELAY_NONE,
            "delay: the flag changes only the delay of REMOVED");
    }

    // --- a REMOVED of a disconnected child takes its own due time, not the earlier one of the transition it replaces;
    // every other transition keeps the earlier due time, so a reconnect overwrites a queued REMOVED as soon as it is due
    {
        RRDHOST *h = callocz(1, sizeof(*h));
        uuid_generate(h->host_id.uuid);
        h->aclk_host_config = callocz(1, sizeof(*h->aclk_host_config));
        h->health.alert_queue_next_due_s = nd_time_t_max();
        nd_uuid_t *hid = &h->host_id.uuid;
        time_t now = now_realtime_sec();

        (void)db_execute(db_meta, "DELETE FROM alert_queue", NULL);

        // raised just before the disconnect, not drained yet
        insert_alert_queue(h, 31, 6001, 31, RRDCALC_STATUS_CLEAR, RRDCALC_STATUS_WARNING, 0, now);
        AQ_CHECK(aq_pending_due(hid, 31) == now, "replace: the raise is due at once");

        insert_alert_queue(
            h, 31, 6002, 31, RRDCALC_STATUS_WARNING, RRDCALC_STATUS_REMOVED, HEALTH_ENTRY_FLAG_CHILD_DISCONNECTED, now);
        AQ_CHECK(
            aq_pending_due(hid, 31) == now + ALERT_TRANSITION_DELAY_LONG,
            "replace: the REMOVED of a disconnect keeps its long delay (due in %lld s)",
            (long long)(aq_pending_due(hid, 31) - now));

        // the child is back: the reconnect transition is due at once again
        insert_alert_queue(h, 31, 6003, 31, RRDCALC_STATUS_REMOVED, RRDCALC_STATUS_WARNING, 0, now);
        AQ_CHECK(aq_pending_due(hid, 31) == now, "replace: the reconnect overwrites the REMOVED and is due at once");

        // any other REMOVED keeps the earlier due time
        insert_alert_queue(h, 31, 6004, 31, RRDCALC_STATUS_WARNING, RRDCALC_STATUS_REMOVED, 0, now);
        AQ_CHECK(aq_pending_due(hid, 31) == now, "replace: another REMOVED keeps the earlier due time");

        (void)db_execute(db_meta, "DELETE FROM alert_queue", NULL);
        freez(h->aclk_host_config);
        freez(h);
    }

    // --- repeated short outages coalesce in the real pending/submit queues. Cloud already knows the raised
    // status, so recovery before removal is submitted must generate no extra Cloud transition.
    for (int child = 0; child < 2; child++) {
        for (int critical = 0; critical < 2; critical++) {
            RRDHOST *h = callocz(1, sizeof(*h));
            uuid_generate(h->host_id.uuid);
            h->aclk_host_config = callocz(1, sizeof(*h->aclk_host_config));
            h->health.alert_queue_next_due_s = nd_time_t_max();
            nd_uuid_t *hid = &h->host_id.uuid;
            RRDCALC_STATUS raised = critical ? RRDCALC_STATUS_CRITICAL : RRDCALC_STATUS_WARNING;
            uint32_t flags = child ? HEALTH_ENTRY_FLAG_CHILD_DISCONNECTED : 0;
            uint32_t uid = 10000 + child * 100 + critical * 20;
            time_t now = now_realtime_sec();

            AQ_SQL(aq_seed_alert_config(h) ? SQLITE_DONE : SQLITE_ERROR, "bounce alert config");
            int64_t id = aq_save_transition(h, ++uid, RRDCALC_STATUS_REMOVED, raised, 0, now - 1000);
            AQ_CHECK(id > 0 && process_alert_pending_queue(h) && aq_test_queue(hid, NULL, NULL) == 1,
                     "bounce: initial raised status reaches the submit queue (child=%d critical=%d)", child, critical);
            AQ_CHECK(aq_send_one(h) == 1, "bounce: initial raise emits one message");
            AQ_CHECK(cloud_status_matches(id, raised) && aq_test_queue(hid, NULL, NULL) == 0,
                     "bounce: Cloud starts with the raised status (child=%d critical=%d)", child, critical);

            for (int cycle = 0; cycle < 5; cycle++) {
                rrdhost_flag_clear(h, RRDHOST_FLAG_COLLECTOR_ONLINE);
                AQ_CHECK(aq_save_transition(h, ++uid, raised, RRDCALC_STATUS_REMOVED, flags, now) == id,
                         "bounce: removal retains the alert identity (child=%d cycle=%d)", child, cycle);
                (void)process_alert_pending_queue(h);
                AQ_CHECK(aq_pending_rows(hid) == 1 && aq_test_queue(hid, NULL, NULL) == 0 && alert_snapshot_held(h),
                         "bounce: pending removal cannot clear Cloud (child=%d cycle=%d)", child, cycle);

                rrdhost_flag_set(h, RRDHOST_FLAG_COLLECTOR_ONLINE);
                AQ_CHECK(aq_save_transition(h, ++uid, RRDCALC_STATUS_REMOVED, raised, 0, now) == id,
                         "bounce: recovery retains the alert identity (child=%d cycle=%d)", child, cycle);
                (void)process_alert_pending_queue(h);
                AQ_CHECK(aq_pending_rows(hid) == 0 && aq_test_queue(hid, NULL, NULL) == 0 &&
                             cloud_status_matches(id, raised),
                         "bounce: recovery replaces removal without duplicate/stale submissions (child=%d cycle=%d)",
                         child, cycle);
                AQ_CHECK(aq_send_one(h) == 0,
                         "bounce: quick recovery emits no message (child=%d cycle=%d)", child, cycle);
                AQ_CHECK(rebuild_host_alert_version_table(h) && aq_snapshot_rows(h) == 1,
                         "bounce: recovered snapshot keeps the raised alert (child=%d cycle=%d)", child, cycle);
            }

            // Eleven seconds is beyond ordinary raised-alert removal but within the child's 600-second grace.
            rrdhost_flag_clear(h, RRDHOST_FLAG_COLLECTOR_ONLINE);
            AQ_CHECK(aq_save_transition(h, ++uid, raised, RRDCALC_STATUS_REMOVED, flags, now - 11) == id,
                     "bounce: persisted removal before grace check (child=%d)", child);
            (void)process_alert_pending_queue(h);
            int64_t submitted_uid = 0;
            AQ_CHECK(child ? (aq_pending_rows(hid) == 1 && aq_test_queue(hid, NULL, NULL) == 0) :
                             (aq_pending_rows(hid) == 0 && aq_test_queue(hid, NULL, &submitted_uid) == 1 &&
                              submitted_uid == uid),
                     "bounce: ordinary vnode removal is submitted while child grace is preserved (child=%d)", child);
            if (child) {
                AQ_CHECK(aq_save_transition(h, ++uid, raised, RRDCALC_STATUS_REMOVED, flags, now - 601) == id,
                         "bounce: persisted child removal beyond grace");
                (void)process_alert_pending_queue(h);
                AQ_CHECK(aq_pending_rows(hid) == 0 && aq_test_queue(hid, NULL, &submitted_uid) == 1 &&
                             submitted_uid == uid,
                         "bounce: child removal is eventually submitted after grace");
            }
            AQ_CHECK(aq_send_one(h) == 1, "bounce: sustained outage emits one removal (child=%d)", child);
            AQ_CHECK(aq_test_queue(hid, NULL, NULL) == 0 && rebuild_host_alert_version_table(h) &&
                         aq_snapshot_rows(h) == 0,
                     "bounce: sustained outage has one removal and an empty snapshot (child=%d)", child);
            freez(h->aclk_host_config);
            freez(h);
        }
    }

    // --- recovery after the pending removal became due, but before ACLK consumed it, must supersede that
    // submit-queue entry too. Otherwise the delayed REMOVED is sent after recovery with no raise left to follow it.
    {
        RRDHOST *h = callocz(1, sizeof(*h));
        uuid_generate(h->host_id.uuid);
        h->aclk_host_config = callocz(1, sizeof(*h->aclk_host_config));
        AQ_SQL(aq_seed_alert_config(h) ? SQLITE_DONE : SQLITE_ERROR, "submitted bounce alert config");
        time_t now = now_realtime_sec();
        int64_t id = aq_save_transition(h, 12001, RRDCALC_STATUS_REMOVED, RRDCALC_STATUS_WARNING, 0, now - 1000);
        AQ_CHECK(id > 0 && process_alert_pending_queue(h), "submitted bounce: initial raise is submitted");
        AQ_CHECK(aq_send_one(h) == 1, "submitted bounce: initial raise emits one message");
        uint64_t raised_version = calculate_node_alert_version(h);
        AQ_CHECK(aq_save_transition(h, 12002, RRDCALC_STATUS_WARNING, RRDCALC_STATUS_REMOVED, 0, now - 11) == id &&
                     process_alert_pending_queue(h),
                 "submitted bounce: expired removal reaches the submit queue");
        AQ_CHECK(aq_save_transition(h, 12003, RRDCALC_STATUS_REMOVED, RRDCALC_STATUS_WARNING, 0, now) == id,
                 "submitted bounce: recovery is persisted before ACLK sends removal");
        (void)process_alert_pending_queue(h);
        int64_t uid = 0;
        int submitted = aq_test_queue(&h->host_id.uuid, NULL, &uid);
        AQ_CHECK(submitted == 1 && uid == 12003,
                 "submitted bounce: recovery replaces stale removal (rows=%d uid=%" PRId64 ")",
                 submitted, uid);
        AQ_CHECK(aq_send_one(h) == 0 && calculate_node_alert_version(h) == raised_version,
                 "submitted bounce: unsent removal/recovery emits no message and preserves Cloud checksum");

        AQ_CHECK(aq_save_transition(h, 12004, RRDCALC_STATUS_WARNING, RRDCALC_STATUS_REMOVED, 0, now - 11) == id &&
                     process_alert_pending_queue(h),
                 "submitted bounce: another removal reaches ACLK");
        int64_t removed_sequence = 0, removed_uid = 0;
        AQ_CHECK(aq_test_queue(&h->host_id.uuid, &removed_sequence, &removed_uid) == 1 && removed_uid == 12004,
                 "submitted bounce: ACLK reads the removal before recovery");
        AQ_CHECK(aq_save_transition(h, 12005, RRDCALC_STATUS_REMOVED, RRDCALC_STATUS_WARNING, 0, now) == id,
                 "submitted bounce: another recovery is persisted while removal is in flight");
        (void)process_alert_pending_queue(h);
        // The send was already in flight: its version update is allowed, but its guarded delete must retain
        // the recovery. Cancelling the old queue row without retaining recovery would lose this race.
        sqlite3_stmt *version = NULL;
        sql_update_alert_version(id, removed_uid, RRDCALC_STATUS_REMOVED, now, &version);
        SQLITE_FINALIZE(version);
        (void)delete_alert_from_submit_queue(&h->host_id.uuid, removed_sequence, removed_uid, &res_delete);
        AQ_CHECK(aq_test_queue(&h->host_id.uuid, NULL, &uid) == 1 && uid == 12005,
                 "submitted bounce: in-flight removal cannot consume recovery");
        AQ_CHECK(aq_send_one(h) == 1, "submitted bounce: acknowledged removal needs exactly one recovery message");
        AQ_CHECK(cloud_status_matches(id, RRDCALC_STATUS_WARNING) && aq_test_queue(&h->host_id.uuid, NULL, NULL) == 0,
                 "submitted bounce: draining after recovery leaves Cloud raised");
        freez(h->aclk_host_config);
        freez(h);
    }

    // --- a snapshot requested between reconnect and the completed health pass must not publish the old
    // REMOVED health-log state. The completed-pass marker and persistence barrier cover distinct windows.
    {
        RRDHOST *h = callocz(1, sizeof(*h));
        uuid_generate(h->host_id.uuid);
        h->aclk_host_config = callocz(1, sizeof(*h->aclk_host_config));
        h->health.enabled = true;
        AQ_SQL(aq_seed_alert_config(h) ? SQLITE_DONE : SQLITE_ERROR, "recovery snapshot alert config");
        time_t now = now_realtime_sec();
        int64_t id = aq_save_transition(h, 13001, RRDCALC_STATUS_REMOVED, RRDCALC_STATUS_WARNING, 0, now - 1000);
        AQ_CHECK(id > 0 && process_alert_pending_queue(h), "recovery snapshot: initial raise is submitted");
        (void)commit_alert_events(h);
        AQ_CHECK(aq_save_transition(h, 13002, RRDCALC_STATUS_WARNING, RRDCALC_STATUS_REMOVED, 0, now) == id,
                 "recovery snapshot: offline removal is persisted");

        rrdhost_flag_set(h, RRDHOST_FLAG_COLLECTOR_ONLINE | RRDHOST_FLAG_PENDING_HEALTH_INITIALIZATION);
        h->health.pending_transitions = 1;
        AQ_CHECK(alert_snapshot_held(h), "recovery snapshot: reconnect cannot bypass pending writes");
        h->health.pending_transitions = 0;
        rrdhost_flag_clear(h, RRDHOST_FLAG_PENDING_HEALTH_INITIALIZATION);
        AQ_CHECK(alert_snapshot_held(h), "recovery snapshot: attaching charts is not a completed health pass");
        if (!alert_snapshot_held(h))
            (void)rebuild_host_alert_version_table(h);
        AQ_CHECK(aq_snapshot_rows(h) == 1,
                 "recovery snapshot: requested snapshot cannot clear the raised alert before evaluation");

        AQ_CHECK(aq_save_transition(h, 13003, RRDCALC_STATUS_REMOVED, RRDCALC_STATUS_UNINITIALIZED, 0, now) == id,
                 "recovery snapshot: chart attachment persists initialization before data is runnable");
        h->health.alert_status_snapshot.valid = 1;
        AQ_CHECK(alert_snapshot_held(h),
                 "recovery snapshot: completed non-runnable pass preserves the queued initialization delay");
        if (!alert_snapshot_held(h))
            (void)rebuild_host_alert_version_table(h);
        AQ_CHECK(cloud_status_matches(id, RRDCALC_STATUS_WARNING),
                 "recovery snapshot: non-runnable initialization cannot replace Cloud's raised status early");

        AQ_CHECK(aq_save_transition(h, 13004, RRDCALC_STATUS_UNINITIALIZED, RRDCALC_STATUS_WARNING, 0, now) == id,
                 "recovery snapshot: first evaluated raise is persisted");
        h->health.alert_status_snapshot.valid = 1;
        h->health.pending_transitions = 1;
        AQ_CHECK(alert_snapshot_held(h), "recovery snapshot: completed evaluation still waits for persistence");
        h->health.pending_transitions = 0;
        AQ_CHECK(!alert_snapshot_held(h) && rebuild_host_alert_version_table(h) && aq_snapshot_rows(h) == 1,
                 "recovery snapshot: completed persisted recovery sends one raised alert");
        (void)process_alert_pending_queue(h);
        AQ_CHECK(aq_pending_rows(&h->host_id.uuid) == 0 && aq_test_queue(&h->host_id.uuid, NULL, NULL) == 0,
                 "recovery snapshot: subsequent drain generates no redundant raise");

        h->health.enabled = false;
        h->health.alert_status_snapshot.valid = 0;
        AQ_CHECK(!alert_snapshot_held(h), "recovery snapshot: health-disabled host does not wait for evaluation");
        freez(h->aclk_host_config);
        freez(h);
    }

    // --- a reconnect that never becomes runnable cannot hold snapshots forever. Its real initialization
    // transition keeps the ordinary 600-second delay, then becomes eligible even though its status never changed.
    {
        RRDHOST *h = callocz(1, sizeof(*h));
        uuid_generate(h->host_id.uuid);
        h->aclk_host_config = callocz(1, sizeof(*h->aclk_host_config));
        h->health.enabled = true;
        AQ_SQL(aq_seed_alert_config(h) ? SQLITE_DONE : SQLITE_ERROR, "non-runnable recovery alert config");
        time_t now = now_realtime_sec();
        int64_t id = aq_save_transition(h, 14001, RRDCALC_STATUS_REMOVED, RRDCALC_STATUS_WARNING, 0, now - 2000);
        AQ_CHECK(id > 0 && process_alert_pending_queue(h) && aq_send_one(h) == 1,
                 "non-runnable recovery: Cloud starts with a raised alert");
        AQ_CHECK(aq_save_transition(h, 14002, RRDCALC_STATUS_WARNING, RRDCALC_STATUS_REMOVED,
                                   HEALTH_ENTRY_FLAG_CHILD_DISCONNECTED, now - 11) == id,
                 "non-runnable recovery: child removal is persisted");
        rrdhost_flag_set(h, RRDHOST_FLAG_COLLECTOR_ONLINE);
        AQ_CHECK(aq_save_transition(h, 14003, RRDCALC_STATUS_REMOVED, RRDCALC_STATUS_UNINITIALIZED, 0, now - 11) == id,
                 "non-runnable recovery: initialization is persisted");
        h->health.alert_status_snapshot.valid = 1;
        (void)process_alert_pending_queue(h);
        AQ_CHECK(aq_pending_rows(&h->host_id.uuid) == 1 && aq_pending_due(&h->host_id.uuid, id) == now + 589 &&
                     aq_test_queue(&h->host_id.uuid, NULL, NULL) == 0 &&
                     alert_snapshot_held(h),
                 "non-runnable recovery: online snapshot honors the ordinary 600-second initialization delay");

        // Advance only this fixture's scheduler deadline instead of sleeping for its remaining grace.
        sqlite3_stmt *expire = NULL;
        if (PREPARE_STATEMENT(db_meta,
                              "UPDATE alert_queue SET date_scheduled = UNIXEPOCH() - 1 WHERE host_id = ?1", &expire)) {
            AQ_SQL(sqlite3_bind_blob(expire, 1, &h->host_id.uuid, sizeof(h->host_id.uuid), SQLITE_STATIC),
                   "non-runnable recovery expire host");
            AQ_SQL(sqlite3_step_monitored(expire), "non-runnable recovery expire deadline");
            SQLITE_FINALIZE(expire);
        }
        h->health.alert_queue_next_due_s = 0;
        AQ_CHECK(!alert_snapshot_held(h) && process_alert_pending_queue(h) && aq_send_one(h) == 1 &&
                     cloud_status_matches(id, RRDCALC_STATUS_UNINITIALIZED),
                 "non-runnable recovery: initialization is eventually sent after its deadline");

        AQ_SQL(sqlite3_set_authorizer(db_meta, aq_deny_alert_version_read, NULL), "deny version read");
        bool matches_on_error = cloud_status_matches(id, RRDCALC_STATUS_UNINITIALIZED);
        AQ_SQL(sqlite3_set_authorizer(db_meta, NULL, NULL), "restore version read");
        AQ_CHECK(!matches_on_error, "sender dedup: a failed version read cannot establish a matching Cloud status");
        freez(h->aclk_host_config);
        freez(h);
    }

    // --- a READY snapshot held by one delayed initialization must not block another alert's due raise.
    // Run the production per-host dispatcher and real joined push query, recording only the transport call.
    {
        ND_UUID saved_claim = claim_id_get_uuid();
        AQ_CHECK(claim_id_set_str("00000000-0000-4000-8000-000000000111"), "mixed alerts: synthetic claim is valid");
        for (int critical = 0; critical < 2; critical++) {
            RRDHOST *h = callocz(1, sizeof(*h));
            uuid_generate(h->host_id.uuid);
            uuid_generate(h->node_id.uuid);
            h->hostname = string_strdupz("mixed-alert-host");
            h->health.default_exec = string_strdupz("/bin/true");
            h->aclk_host_config = callocz(1, sizeof(*h->aclk_host_config));
            h->health.enabled = true;
            h->health.alert_status_snapshot.valid = 1;
            rrdhost_flag_set(h, RRDHOST_FLAG_COLLECTOR_ONLINE | RRDHOST_FLAG_ACLK_STREAM_ALERTS);
            aclk_alert_streaming_set(h->aclk_host_config, true);
            aclk_alert_snapshot_request(h->aclk_host_config);
            AQ_CHECK(aclk_alert_snapshot_mark_ready(h->aclk_host_config), "mixed alerts: snapshot becomes READY");
            AQ_SQL(aq_seed_alert_config(h) ? SQLITE_DONE : SQLITE_ERROR, "mixed alerts config");
            time_t now = now_realtime_sec();
            RRDCALC_STATUS raised = critical ? RRDCALC_STATUS_CRITICAL : RRDCALC_STATUS_WARNING;
            uint32_t uid = 15000 + critical * 10;
            AQ_CHECK(aq_save_transition(h, uid, RRDCALC_STATUS_REMOVED, RRDCALC_STATUS_UNINITIALIZED, 0, now) > 0,
                     "mixed alerts: first alert has a future initialization");
            int64_t raised_id = aq_save_transition_for_alarm(h, 101, uid + 1, RRDCALC_STATUS_CLEAR, raised, 0, now);
            AQ_CHECK(raised_id > 0 && process_alert_pending_queue(h) &&
                         aq_pending_rows(&h->host_id.uuid) == 1 && aq_test_queue(&h->host_id.uuid, NULL, NULL) == 1,
                     "mixed alerts: second alert is submitted while initialization remains pending");

            sqlite3_stmt *push = NULL, *version = NULL, *remove = NULL;
            unsigned before = aq_emitted_events;
            aclk_push_alert_events_for_host(h, &push, &version, &remove, aq_record_send, aq_record_snapshot);
            AQ_CHECK(aq_emitted_events == before + 1 && aq_last_alarm_id == 101 &&
                         aq_last_status == rrdcalc_status_to_proto_enum(raised) &&
                         cloud_status_matches(raised_id, raised) && aq_test_queue(&h->host_id.uuid, NULL, NULL) == 0,
                     "mixed alerts: READY dispatch emits due %s despite another alert's future row",
                     critical ? "CRITICAL" : "WARNING");
            AQ_CHECK(aclk_alert_snapshot_state_get(h->aclk_host_config) == ACLK_ALERT_SNAPSHOT_READY &&
                         !h->aclk_host_config->snapshot_count && aq_pending_rows(&h->host_id.uuid) == 1 &&
                         rrdhost_flag_check(h, RRDHOST_FLAG_ACLK_STREAM_ALERTS),
                     "mixed alerts: dispatch retains the held snapshot and delayed transition");
            SQLITE_FINALIZE(push);
            SQLITE_FINALIZE(version);
            SQLITE_FINALIZE(remove);
            string_freez(h->health.default_exec);
            string_freez(h->hostname);
            freez(h->aclk_host_config);
            freez(h);
        }
        // A hold emerging during checkpoint preparation must roll back speculative versions, including
        // alerts beyond one normal-send batch. Otherwise sender dedup would consume unsent raises.
        {
            RRDHOST *h = callocz(1, sizeof(*h));
            uuid_generate(h->host_id.uuid);
            uuid_generate(h->node_id.uuid);
            h->hostname = string_strdupz("snapshot-race-host");
            h->health.default_exec = string_strdupz("/bin/true");
            h->aclk_host_config = callocz(1, sizeof(*h->aclk_host_config));
            h->health.enabled = true;
            h->health.alert_status_snapshot.valid = 1;
            rrdhost_flag_set(h, RRDHOST_FLAG_COLLECTOR_ONLINE | RRDHOST_FLAG_ACLK_STREAM_ALERTS);
            aclk_alert_streaming_set(h->aclk_host_config, true);
            aclk_alert_snapshot_request(h->aclk_host_config);
            AQ_CHECK(aclk_alert_snapshot_mark_ready(h->aclk_host_config), "snapshot race: request becomes READY");
            AQ_SQL(aq_seed_alert_config(h) ? SQLITE_DONE : SQLITE_ERROR, "snapshot race config");
            const int batch = atoi(ACLK_MAX_ALERT_UPDATES);
            const int count = batch + 7;
            time_t now = now_realtime_sec();
            int64_t last_id = 0;
            for (int i = 0; i < count; i++) {
                last_id = aq_save_transition_for_alarm(h, 200 + i, 16000 + i,
                                                      RRDCALC_STATUS_CLEAR, RRDCALC_STATUS_WARNING, 0, now);
                AQ_CHECK(last_id > 0, "snapshot race: persisted due raise %d", i);
            }
            AQ_CHECK(process_alert_pending_queue(h) && aq_test_queue(&h->host_id.uuid, NULL, NULL) == count,
                     "snapshot race: more than one due batch is submitted");
            AQ_SQL(sqlite3_create_function_v2(db_meta, "aq_hold_snapshot_during_rebuild", 0, SQLITE_UTF8,
                                               h, aq_hold_snapshot_during_rebuild, NULL, NULL, NULL),
                   "snapshot race register hold callback");
            AQ_SQL(sqlite3_exec(db_meta,
                               "CREATE TEMP TRIGGER aq_snapshot_hold AFTER INSERT ON alert_version"
                               " BEGIN SELECT aq_hold_snapshot_during_rebuild(); END", NULL, NULL, NULL),
                   "snapshot race register hold trigger");
            sqlite3_stmt *push = NULL, *version = NULL, *remove = NULL;
            unsigned before = aq_emitted_events;
            aclk_push_alert_events_for_host(h, &push, &version, &remove, aq_record_send, aq_record_snapshot);
            AQ_CHECK(aq_emitted_events == before + batch &&
                         aq_test_queue(&h->host_id.uuid, NULL, NULL) == count - batch &&
                         !cloud_status_matches(last_id, RRDCALC_STATUS_WARNING),
                     "snapshot race: new hold rolls back speculative versions and sends only one real batch");
            aclk_push_alert_events_for_host(h, &push, &version, &remove, aq_record_send, aq_record_snapshot);
            AQ_CHECK(aq_emitted_events == before + count && aq_test_queue(&h->host_id.uuid, NULL, NULL) == 0 &&
                         cloud_status_matches(last_id, RRDCALC_STATUS_WARNING) &&
                         aclk_alert_snapshot_state_get(h->aclk_host_config) == ACLK_ALERT_SNAPSHOT_READY &&
                         !h->aclk_host_config->snapshot_count,
                     "snapshot race: next held dispatch sends the remainder without losing the snapshot request");
            SQLITE_FINALIZE(push);
            SQLITE_FINALIZE(version);
            SQLITE_FINALIZE(remove);
            AQ_SQL(sqlite3_exec(db_meta, "DROP TRIGGER aq_snapshot_hold", NULL, NULL, NULL),
                   "snapshot race remove hold trigger");
            AQ_SQL(sqlite3_create_function_v2(db_meta, "aq_hold_snapshot_during_rebuild", 0, SQLITE_UTF8,
                                               NULL, NULL, NULL, NULL, NULL),
                   "snapshot race remove hold callback");
            string_freez(h->health.default_exec);
            string_freez(h->hostname);
            freez(h->aclk_host_config);
            freez(h);
        }
        // Snapshot capture can overtake the pending-queue drain on recovery. The snapshot already contains
        // the raised recovery, so an older submitted REMOVED must never be emitted on the following tick.
        {
            RRDHOST *h = callocz(1, sizeof(*h));
            uuid_generate(h->host_id.uuid);
            uuid_generate(h->node_id.uuid);
            h->hostname = string_strdupz("snapshot-covered-alert-host");
            h->health.default_exec = string_strdupz("/bin/true");
            h->aclk_host_config = callocz(1, sizeof(*h->aclk_host_config));
            aclk_node_id_set(h->aclk_host_config, h->node_id.uuid);
            h->health.enabled = true;
            h->health.alert_status_snapshot.valid = 1;
            rrdhost_flag_set(h, RRDHOST_FLAG_COLLECTOR_ONLINE | RRDHOST_FLAG_ACLK_STREAM_ALERTS);
            aclk_alert_streaming_set(h->aclk_host_config, true);
            AQ_SQL(aq_seed_alert_config(h) ? SQLITE_DONE : SQLITE_ERROR, "snapshot covered alert config");
            time_t now = now_realtime_sec();
            int64_t id = aq_save_transition(h, 18001, RRDCALC_STATUS_REMOVED, RRDCALC_STATUS_WARNING, 0, now - 2000);
            AQ_CHECK(id > 0 && process_alert_pending_queue(h) && aq_send_one(h) == 1,
                     "snapshot covered alert: initial WARNING is sent");
            AQ_CHECK(aq_save_transition(h, 18002, RRDCALC_STATUS_WARNING, RRDCALC_STATUS_REMOVED, 0, now - 11) == id &&
                         process_alert_pending_queue(h),
                     "snapshot covered alert: older REMOVED is submitted");
            AQ_CHECK(aq_save_transition(h, 18003, RRDCALC_STATUS_REMOVED, RRDCALC_STATUS_WARNING, 0, now) == id,
                     "snapshot covered alert: recovery is persisted before its pending row drains");
            aclk_alert_snapshot_request(h->aclk_host_config);
            AQ_CHECK(aclk_alert_snapshot_mark_ready(h->aclk_host_config), "snapshot covered alert: request is READY");

            sqlite3_stmt *push = NULL, *version = NULL, *remove = NULL;
            unsigned before = aq_emitted_events, snapshots_before = aq_emitted_snapshots;
            aclk_push_alert_events_for_host(h, &push, &version, &remove, aq_record_send, aq_record_snapshot);
            AQ_CHECK(aq_emitted_snapshots == snapshots_before + 1 && aq_emitted_events == before &&
                         cloud_status_matches(id, RRDCALC_STATUS_WARNING) &&
                         aclk_alert_snapshot_state_get(h->aclk_host_config) == ACLK_ALERT_SNAPSHOT_IDLE,
                     "snapshot covered alert: production dispatch publishes the recovered WARNING snapshot");
            aclk_push_alert_events_for_host(h, &push, &version, &remove, aq_record_send, aq_record_snapshot);
            AQ_CHECK(aq_emitted_events == before && cloud_status_matches(id, RRDCALC_STATUS_WARNING) &&
                         aq_test_queue(&h->host_id.uuid, NULL, NULL) == 0,
                     "snapshot covered alert: no older REMOVED follows the recovered snapshot");
            (void)process_alert_pending_queue(h);
            rrdhost_flag_set(h, RRDHOST_FLAG_ACLK_STREAM_ALERTS);
            aclk_push_alert_events_for_host(h, &push, &version, &remove, aq_record_send, aq_record_snapshot);
            AQ_CHECK(aq_emitted_events == before && aq_pending_rows(&h->host_id.uuid) == 0 &&
                         cloud_status_matches(id, RRDCALC_STATUS_WARNING),
                     "snapshot covered alert: later pending drain does not duplicate the snapshot recovery");
            AQ_CHECK(insert_alert_to_submit_queue(&h->host_id.uuid, id, 18004, RRDCALC_STATUS_CRITICAL) == 0 &&
                         rebuild_host_alert_version_table(h),
                     "snapshot covered alert: a newer submit identity remains outside the captured checkpoint");
            int64_t newer_uid = 0;
            AQ_CHECK(aq_test_queue(&h->host_id.uuid, NULL, &newer_uid) == 1 && newer_uid == 18004,
                     "snapshot covered alert: snapshot cleanup preserves a submit row newer than capture");
            SQLITE_FINALIZE(push);
            SQLITE_FINALIZE(version);
            SQLITE_FINALIZE(remove);
            string_freez(h->health.default_exec);
            string_freez(h->hostname);
            freez(h->aclk_host_config);
            freez(h);
        }
        // A pending cursor can have cached both an old removal and an old recovery before later writes
        // replace that row. Snapshot capture must not let either cached owner submit or rewind its version.
        {
            RRDHOST *h = callocz(1, sizeof(*h));
            uuid_generate(h->host_id.uuid);
            uuid_generate(h->node_id.uuid);
            h->hostname = string_strdupz("snapshot-pending-owner-host");
            h->health.default_exec = string_strdupz("/bin/true");
            h->aclk_host_config = callocz(1, sizeof(*h->aclk_host_config));
            aclk_node_id_set(h->aclk_host_config, h->node_id.uuid);
            h->health.enabled = true;
            h->health.alert_status_snapshot.valid = 1;
            rrdhost_flag_set(h, RRDHOST_FLAG_COLLECTOR_ONLINE | RRDHOST_FLAG_ACLK_STREAM_ALERTS);
            aclk_alert_streaming_set(h->aclk_host_config, true);
            AQ_SQL(aq_seed_alert_config(h) ? SQLITE_DONE : SQLITE_ERROR, "pending owner config");
            time_t now = now_realtime_sec();
            int64_t id = aq_save_transition(h, 19000, RRDCALC_STATUS_REMOVED, RRDCALC_STATUS_WARNING, 0, now - 2000);
            AQ_CHECK(id > 0 && process_alert_pending_queue(h) && aq_send_one(h) == 1,
                     "pending owner: initial raise is acknowledged");
            AQ_CHECK(aq_save_transition(h, 19001, RRDCALC_STATUS_WARNING, RRDCALC_STATUS_REMOVED, 0, now - 11) == id,
                     "pending owner: removal is persisted");
            int64_t removed_uid = 0, recovery_uid = 0;
            int64_t removed_row = aq_cached_pending_row(h, &removed_uid);
            AQ_CHECK(removed_row && removed_uid == 19001, "pending owner: cursor caches the due removal");
            AQ_CHECK(aq_save_transition(h, 19002, RRDCALC_STATUS_REMOVED, RRDCALC_STATUS_WARNING, 0, now) == id,
                     "pending owner: first recovery replaces removal");
            int64_t recovery_row = aq_cached_pending_row(h, &recovery_uid);
            AQ_CHECK(recovery_row == removed_row && recovery_uid == 19002,
                     "pending owner: cursor caches the due recovery on the same row");
            AQ_CHECK(aq_save_transition(h, 19003, RRDCALC_STATUS_WARNING, RRDCALC_STATUS_REMOVED, 0, now) == id &&
                         aq_save_transition(h, 19004, RRDCALC_STATUS_REMOVED, RRDCALC_STATUS_WARNING, 0, now) == id,
                     "pending owner: another bounce replaces both cached owners");
            aclk_alert_snapshot_request(h->aclk_host_config);
            AQ_CHECK(aclk_alert_snapshot_mark_ready(h->aclk_host_config), "pending owner: snapshot request is READY");
            sqlite3_stmt *push = NULL, *version = NULL, *remove = NULL;
            unsigned before = aq_emitted_events;
            aclk_push_alert_events_for_host(h, &push, &version, &remove, aq_record_send, aq_record_snapshot);
            AQ_CHECK(aq_version_unique_id(id) == 19004 && cloud_status_matches(id, RRDCALC_STATUS_WARNING),
                     "pending owner: actual snapshot captures the newest recovery");
            AQ_CHECK(submit_pending_alert_if_current(&h->host_id.uuid, id, removed_row, removed_uid,
                                                     RRDCALC_STATUS_REMOVED) == 1 &&
                         submit_pending_alert_if_current(&h->host_id.uuid, id, recovery_row, recovery_uid,
                                                         RRDCALC_STATUS_WARNING) == 1 &&
                         aq_test_queue(&h->host_id.uuid, NULL, NULL) == 0 && aq_version_unique_id(id) == 19004,
                     "pending owner: cached transitions cannot submit or rewind the captured version");
            int64_t current_uid = 0;
            AQ_CHECK(aq_cached_pending_row(h, &current_uid) == recovery_row && current_uid == 19004 &&
                         aq_pending_rows(&h->host_id.uuid) == 1,
                     "pending owner: rejecting stale cursors preserves the current pending row");
            (void)process_alert_pending_queue(h);
            AQ_CHECK(aq_pending_rows(&h->host_id.uuid) == 0 && aq_version_unique_id(id) == 19004 &&
                         aq_emitted_events == before,
                     "pending owner: the current row drains without a duplicate event");
            SQLITE_FINALIZE(push);
            SQLITE_FINALIZE(version);
            SQLITE_FINALIZE(remove);
            string_freez(h->health.default_exec);
            string_freez(h->hostname);
            freez(h->aclk_host_config);
            freez(h);
        }
        claim_id_set(saved_claim);
        (void)db_execute(db_meta, "DELETE FROM alert_queue", NULL);
        (void)db_execute(db_meta, "DELETE FROM aclk_queue", NULL);
    }

    // --- only an eligible READY checkpoint freezes health. Admitting evaluation invalidates the completed
    // pass under the same mutex used by snapshot preparation.
    {
        RRDHOST *h = callocz(1, sizeof(*h));
        uuid_generate(h->host_id.uuid);
        h->aclk_host_config = callocz(1, sizeof(*h->aclk_host_config));
        h->health.enabled = true;
        h->health.alert_status_snapshot.valid = 1;
        rrdhost_flag_set(h, RRDHOST_FLAG_COLLECTOR_ONLINE);
        AQ_CHECK(!alert_snapshot_freezes_health(h) && h->health.alert_status_snapshot.valid,
                 "health freeze: IDLE allows evaluation");
        aclk_alert_snapshot_request(h->aclk_host_config);
        AQ_CHECK(!alert_snapshot_freezes_health(h) && h->health.alert_status_snapshot.valid,
                 "health freeze: PENDING allows evaluation");
        AQ_CHECK(aclk_alert_snapshot_mark_ready(h->aclk_host_config) && alert_snapshot_freezes_health(h) &&
                     h->health.alert_status_snapshot.valid,
                 "health freeze: eligible READY preserves the completed pass");
        h->health.pending_transitions = 1;
        AQ_CHECK(!alert_snapshot_freezes_health(h) && !h->health.alert_status_snapshot.valid,
                 "health freeze: held READY admits and invalidates evaluation");
        h->health.pending_transitions = 0;
        AQ_CHECK(!alert_snapshot_freezes_health(h), "health freeze: incomplete evaluation keeps READY held");
        h->health.alert_status_snapshot.valid = 1;
        AQ_CHECK(alert_snapshot_freezes_health(h), "health freeze: completed evaluation makes READY eligible again");
        freez(h->aclk_host_config);
        freez(h);
    }

    // --- non-raised vnode removals keep the ordinary long-delay policy.
    {
        const RRDCALC_STATUS statuses[] = {
            RRDCALC_STATUS_CLEAR, RRDCALC_STATUS_UNDEFINED, RRDCALC_STATUS_UNINITIALIZED,
        };
        for (size_t i = 0; i < sizeof(statuses) / sizeof(statuses[0]); i++) {
            RRDHOST *h = callocz(1, sizeof(*h));
            uuid_generate(h->host_id.uuid);
            h->aclk_host_config = callocz(1, sizeof(*h->aclk_host_config));
            AQ_SQL(aq_seed_alert_config(h) ? SQLITE_DONE : SQLITE_ERROR, "non-raised alert config");
            AQ_CHECK(aq_save_transition(h, 11000 + i, statuses[i], RRDCALC_STATUS_REMOVED, 0,
                                       now_realtime_sec() - 11) > 0,
                     "ordinary removal: persisted status %d", statuses[i]);
            (void)process_alert_pending_queue(h);
            AQ_CHECK(aq_pending_rows(&h->host_id.uuid) == 1 && aq_test_queue(&h->host_id.uuid, NULL, NULL) == 0 &&
                         alert_snapshot_held(h),
                     "ordinary removal: non-raised status %d retains its grace", statuses[i]);
            freez(h->aclk_host_config);
            freez(h);
        }
        (void)db_execute(db_meta, "DELETE FROM alert_queue", NULL);
    }

    // --- a snapshot of a host that is not collecting is held while the host has rows that are not due yet, or
    // transitions still being saved
    {
        RRDHOST *h = callocz(1, sizeof(*h));
        uuid_generate(h->host_id.uuid);
        nd_uuid_t *hid = &h->host_id.uuid;

        (void)db_execute(db_meta, "DELETE FROM alert_queue", NULL);
        (void)db_execute(db_meta, "DELETE FROM aclk_queue", NULL);

        h->health.alert_queue_next_due_s = nd_time_t_max();
        AQ_CHECK(!alert_snapshot_held(h), "held: an empty queue holds nothing");

        // the gate is nd_time_t_max() while a drain runs, and 0 on a new host with rows persisted by a previous run:
        // neither hides a row due later
        AQ_SQL(aq_pending_seed(hid, 21, 5001, 600) ? SQLITE_DONE : SQLITE_ERROR, "held seed 21");
        AQ_CHECK(alert_snapshot_held(h), "held: a row due later holds the snapshot while the gate is max");
        h->health.alert_queue_next_due_s = 0;
        AQ_CHECK(alert_snapshot_held(h), "held: a row due later holds the snapshot while the gate is 0");

        rrdhost_flag_set(h, RRDHOST_FLAG_COLLECTOR_ONLINE);
        AQ_CHECK(alert_snapshot_held(h), "held: a collecting host also honors queued transition deadlines");
        rrdhost_flag_clear(h, RRDHOST_FLAG_COLLECTOR_ONLINE);

        // a due row (e.g. left behind by a drain) does not hide the row due later
        AQ_SQL(aq_pending_seed(hid, 22, 5002, 0) ? SQLITE_DONE : SQLITE_ERROR, "held seed 22");
        h->health.alert_queue_next_due_s = now_realtime_sec();
        AQ_CHECK(alert_snapshot_held(h), "held: a due row does not release the snapshot while a row due later is queued");

        // the due row drains, the later one still holds; once only due rows are left, nothing holds
        (void)process_alert_pending_queue(h);
        AQ_CHECK(
            aq_pending_rows(hid) == 1 && alert_snapshot_held(h),
            "held: after the drain the row due later still holds the snapshot");
        (void)db_execute(db_meta, "UPDATE alert_queue SET date_scheduled = UNIXEPOCH() - 1", NULL);
        AQ_CHECK(!alert_snapshot_held(h), "held: a due row releases the snapshot");

        // a REMOVED still being saved is not in the queue yet
        h->health.pending_transitions = 1;
        AQ_CHECK(alert_snapshot_held(h), "held: transitions still being saved hold the snapshot");
        h->health.pending_transitions = 0;

        (void)db_execute(db_meta, "DELETE FROM alert_queue", NULL);
        (void)db_execute(db_meta, "DELETE FROM aclk_queue", NULL);
        freez(h);
    }

    SQLITE_FINALIZE(res_delete);
    sql_close_database(db_meta, "METADATA");
    db_meta = saved_db_meta;
    if (owns_sqlite_lifecycle)
        sqlite_library_shutdown();

    netdata_log_info("ALERT QUEUE TEST: %d failure(s)", failures);
    return failures ? 1 : 0;
}

// Start streaming alerts
void aclk_start_alert_streaming(char *node_id, uint64_t cloud_version)
{
    nd_uuid_t node_uuid;

    if (unlikely(!node_id || uuid_parse(node_id, node_uuid)))
        return;

    struct aclk_sync_cfg_t *aclk_host_config;
    RRDHOST *host = rrdhost_find_by_node_id(node_id);

    if (!host || !(aclk_host_config = __atomic_load_n(&host->aclk_host_config, __ATOMIC_ACQUIRE))) {
        nd_log(NDLS_ACCESS, NDLP_NOTICE, "ACLK STA [%s (N/A)]: Ignoring request to stream alert state changes, invalid node.", node_id);
        return;
    }

    if (unlikely(!host->health.enabled)) {
        nd_log(NDLS_ACCESS, NDLP_NOTICE, "ACLK STA [%s (N/A)]: Ignoring request to stream alert state changes, health is disabled.", node_id);
        return;
    }

    nd_log(NDLS_ACCESS, NDLP_DEBUG, "ACLK REQ [%s (%s)]: STREAM ALERTS ENABLED", node_id,
        aclk_host_config->host ? rrdhost_hostname(aclk_host_config->host) : "N/A");
    schedule_alert_snapshot_if_needed(aclk_host_config, cloud_version);
    aclk_alert_streaming_set(aclk_host_config, true);
    if (aclk_alert_snapshot_state_get(aclk_host_config) != ACLK_ALERT_SNAPSHOT_IDLE)
        rrdhost_flag_set(aclk_host_config->host, RRDHOST_FLAG_ACLK_STREAM_ALERTS);
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

    struct aclk_sync_cfg_t *aclk_host_config;
    RRDHOST *host = rrdhost_find_by_node_id(node_id);

    if (!host || !(aclk_host_config = __atomic_load_n(&host->aclk_host_config, __ATOMIC_ACQUIRE)))
        nd_log(NDLS_ACCESS, NDLP_NOTICE,
               "ACLK REQ [%s (N/A)]: ALERTS CHECKPOINT VALIDATION REQUEST RECEIVED FOR INVALID NODE",
               node_id);
    else
        schedule_alert_snapshot_if_needed(aclk_host_config, cloud_version);
}
