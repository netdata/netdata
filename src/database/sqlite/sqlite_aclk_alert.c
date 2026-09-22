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
static int insert_alert_to_submit_queue(nd_uuid_t *host_id, int64_t health_log_id, uint32_t unique_id, RRDCALC_STATUS status)
{
    static __thread sqlite3_stmt *compiled_res = NULL;
    sqlite3_stmt *res = NULL;

    if (cloud_status_matches(health_log_id, status)) {
        update_alert_version_transition(health_log_id, unique_id);
        return 1;
    }

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

    int param = 0;
    SQLITE_BIND_FAIL(done, sqlite3_bind_blob(res, ++param, host_id, sizeof(*host_id), SQLITE_STATIC));
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
// The caller owns the statement: this runs once per row sent, and re-preparing it each time would take
// the global sqlite_spinlock in simple_prepare_statement() - and a full SQL parse - up to
// ACLK_MAX_ALERT_UPDATES times per host per tick, on a lock every web, API and metadata thread contends on.
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

static void aclk_push_alert_event(RRDHOST *host, sqlite3_stmt **res, sqlite3_stmt **res_version, sqlite3_stmt **res_delete)
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
        aclk_send_alarm_log_entry(&alarm_log);

        nd_uuid_t hash_id;
        if (alarm_log.config_hash && !uuid_parse(alarm_log.config_hash, hash_id))
            alert_hash_mark_sent(&hash_id);

        aclk_host_config->alert_count++;

        sent++;
        if (!first_seq)
            first_seq = alarm_log.sequence_id;
        last_seq = alarm_log.sequence_id;

        // The statement to set the version will be compiled once and reset when done
        // our caller will finalize the statement to release resources
        //
        // Ordered version-then-delete on purpose: cloud_status_matches() reads alert_version, so until it is
        // updated a concurrent insert still sees the pre-send status. Deleting first would widen that window.
        sql_update_alert_version(alarm_log.health_log_id, alarm_log.unique_id, status, alarm_log.version, res_version);

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

        // Mark to do one more check
        rrdhost_flag_set(host, RRDHOST_FLAG_ACLK_STREAM_ALERTS);
    }

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
    if (!REQUIRE_HEALTH_DB_OPEN())
        return false;

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

        struct aclk_sync_cfg_t *aclk_host_config = __atomic_load_n(&host->aclk_host_config, __ATOMIC_ACQUIRE);
        if (aclk_host_config) {
            int ret = insert_alert_to_submit_queue(&host->host_id.uuid, health_log_id, unique_id, new_status);
            if (ret == 0)
                added++;
        }

        delete_alert_from_pending_queue(&host->host_id.uuid, row, (int64_t)unique_id);

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
        if (!rrdhost_flag_check(host, RRDHOST_FLAG_ACLK_STREAM_ALERTS) ||
            rrdhost_flag_check(host, RRDHOST_FLAG_PENDING_CONTEXT_LOAD))
            continue;

        rrdhost_flag_clear(host, RRDHOST_FLAG_ACLK_STREAM_ALERTS);

        struct aclk_sync_cfg_t *aclk_host_config = __atomic_load_n(&host->aclk_host_config, __ATOMIC_ACQUIRE);
        if (!aclk_host_config || !aclk_alert_streaming_enabled(aclk_host_config) || rrdhost_flag_check(host, RRDHOST_FLAG_ARCHIVED)) {
            (void)process_alert_pending_queue(host);
            if (commit_alert_events(host))
                rrdhost_flag_set(host, RRDHOST_FLAG_ACLK_STREAM_ALERTS);
            continue;
        }

        enum aclk_alert_snapshot_state snapshot_state = aclk_alert_snapshot_state_get(aclk_host_config);
        if (snapshot_state != ACLK_ALERT_SNAPSHOT_IDLE) {
            rrdhost_flag_set(host, RRDHOST_FLAG_ACLK_STREAM_ALERTS);
            if (snapshot_state == ACLK_ALERT_SNAPSHOT_PENDING)
                continue;
            (void)process_alert_pending_queue(host);
            // the snapshot itself is rebuilt from the health log, not from what is still queued, so a capped
            // drain cannot make it stale; the flag is already set on this path for the remainder
            (void)commit_alert_events(host);
            rebuild_host_alert_version_table(host);
            send_alert_snapshot_to_cloud(host);
            aclk_host_config->snapshot_count++;
            aclk_alert_snapshot_complete(aclk_host_config);
        }
        else
            aclk_push_alert_event(host, &res, &res_version, &res_delete);
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
        "ACLK REQ [%s (%s)]: Sending %d alerts snapshot, snapshot_uuid %s",
        node_id, rrdhost_hostname(host),
        cnt, snapshot_uuid);

    uint32_t chunks;
    chunks = (cnt / ALARM_EVENTS_PER_CHUNK) + (cnt % ALARM_EVENTS_PER_CHUNK != 0);

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
            else
                destroy_alarm_snapshot_proto(snapshot_proto);
            snapshot_proto = NULL;
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
    else
        destroy_alarm_snapshot_proto(snapshot_proto);

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
