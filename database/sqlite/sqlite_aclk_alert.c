// SPDX-License-Identifier: GPL-3.0-or-later

#include "sqlite_functions.h"
#include "sqlite_aclk_alert.h"

#ifdef ENABLE_ACLK
#include "../../aclk/aclk_alarm_api.h"
#endif

#define SQLITE3_COLUMN_STRDUPZ_OR_NULL(res, param)                                                                     \
    ({                                                                                                                 \
        int _param = (param);                                                                                          \
        sqlite3_column_bytes((res), (_param)) ? strdupz((char *)sqlite3_column_text((res), (_param))) : NULL;          \
    })

#define SQL_UPDATE_FILTERED_ALERT                                                                                      \
    "UPDATE aclk_alert_%s SET filtered_alert_unique_id = @new_alert, date_created = UNIXEPOCH() "                      \
    "WHERE filtered_alert_unique_id = @old_alert"

static void update_filtered(ALARM_ENTRY *ae, int64_t unique_id, char *uuid_str)
{
    sqlite3_stmt *res = NULL;

    char sql[ACLK_SYNC_QUERY_SIZE];
    snprintfz(sql, sizeof(sql) - 1, SQL_UPDATE_FILTERED_ALERT, uuid_str);
    int rc = sqlite3_prepare_v2(db_meta, sql, -1, &res, 0);
    if (rc != SQLITE_OK) {
        error_report("Failed to prepare statement when trying to update_filtered");
        return;
    }

    rc = sqlite3_bind_int64(res, 1,  ae->unique_id);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to bind ae unique_id for update_filtered");
        goto done;
    }

    rc = sqlite3_bind_int64(res, 2,  unique_id);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to bind unique_id for update_filtered");
        goto done;
    }

    rc = sqlite3_step_monitored(res);
    if (likely(rc == SQLITE_DONE))
        ae->flags |= HEALTH_ENTRY_FLAG_ACLK_QUEUED;

done:
    rc = sqlite3_finalize(res);
    if (unlikely(rc != SQLITE_OK))
        error_report("Failed to finalize statement when trying to update_filtered, rc = %d", rc);
}

#define SQL_SELECT_VARIABLE_ALERT_BY_UNIQUE_ID                                                                         \
    "SELECT hld.unique_id FROM health_log hl, alert_hash ah, health_log_detail hld "                                   \
    "WHERE hld.unique_id = @unique_id AND hl.config_hash_id = ah.hash_id AND hld.health_log_id = hl.health_log_id "    \
    "AND hl.host_id = @host_id AND ah.warn IS NULL AND ah.crit IS NULL"

static inline bool is_event_from_alert_variable_config(int64_t unique_id, uuid_t *host_id)
{
    sqlite3_stmt *res = NULL;

    int rc = sqlite3_prepare_v2(db_meta, SQL_SELECT_VARIABLE_ALERT_BY_UNIQUE_ID, -1, &res, 0);
    if (rc != SQLITE_OK) {
        error_report("Failed to prepare statement when trying to check for alert variables.");
        return false;
    }

    bool ret = false;

    rc = sqlite3_bind_int64(res, 1, unique_id);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to bind unique_id for checking alert variable.");
        goto done;
    }

    rc = sqlite3_bind_blob(res, 2, host_id, sizeof(*host_id), SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to bind host_id for checking alert variable.");
        goto done;
    }

    rc = sqlite3_step_monitored(res);
    if (likely(rc == SQLITE_ROW))
        ret = true;

done:
    rc = sqlite3_finalize(res);
    if (unlikely(rc != SQLITE_OK))
        error_report("Failed to finalize statement when trying to check for alert variables, rc = %d", rc);

    return ret;
}

#define MAX_REMOVED_PERIOD 604800 //a week

//decide if some events should be sent or not
#define SQL_SELECT_ALERT_BY_ID                                                                                             \
    "SELECT hld.new_status, hl.config_hash_id, hld.unique_id FROM health_log hl, aclk_alert_%s aa, health_log_detail hld " \
    "WHERE hl.host_id = @host_id AND hld.unique_id = aa.filtered_alert_unique_id "                                         \
    "AND hld.alarm_id = @alarm_id AND hl.health_log_id = hld.health_log_id "                                               \
    "ORDER BY hld.rowid DESC LIMIT 1"

static bool should_send_to_cloud(RRDHOST *host, ALARM_ENTRY *ae)
{
    sqlite3_stmt *res = NULL;

    if (ae->new_status == RRDCALC_STATUS_REMOVED || ae->new_status == RRDCALC_STATUS_UNINITIALIZED)
        return 0;

    if (unlikely(uuid_is_null(ae->config_hash_id) || !host->aclk_config))
        return 0;

    char sql[ACLK_SYNC_QUERY_SIZE];

    //get the previous sent event of this alarm_id
    //base the search on the last filtered event
    snprintfz(sql, sizeof(sql) - 1, SQL_SELECT_ALERT_BY_ID, host->aclk_config->uuid_str);

    int rc = sqlite3_prepare_v2(db_meta, sql, -1, &res, 0);
    if (rc != SQLITE_OK) {
        error_report("Failed to prepare statement when trying should_send_to_cloud.");
        return true;
    }

    bool send = false;

    rc = sqlite3_bind_blob(res, 1, &host->host_uuid, sizeof(host->host_uuid), SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to bind host_id for checking should_send_to_cloud");
        goto done;
    }

    rc = sqlite3_bind_int(res, 2, (int) ae->alarm_id);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to bind alarm_id for checking should_send_to_cloud");
        goto done;
    }

    rc = sqlite3_step_monitored(res);

    if (likely(rc == SQLITE_ROW)) {
        uuid_t config_hash_id;
        RRDCALC_STATUS status = (RRDCALC_STATUS)sqlite3_column_int(res, 0);

        if (sqlite3_column_type(res, 1) != SQLITE_NULL)
            uuid_copy(config_hash_id, *((uuid_t *)sqlite3_column_blob(res, 1)));

        int64_t unique_id = sqlite3_column_int64(res, 2);

        if (ae->new_status != (RRDCALC_STATUS)status || uuid_memcmp(&ae->config_hash_id, &config_hash_id))
            send = true;
        else
            update_filtered(ae, unique_id, host->aclk_config->uuid_str);
    } else
        send = true;

done:
    rc = sqlite3_finalize(res);
    if (unlikely(rc != SQLITE_OK))
        error_report("Failed to finalize statement when trying should_send_to_cloud, rc = %d", rc);

    return send;
}

#define SQL_QUEUE_ALERT_TO_CLOUD                                                                                       \
    "INSERT INTO aclk_alert_%s (alert_unique_id, date_created, filtered_alert_unique_id) "                             \
    "VALUES (@alert_unique_id, UNIXEPOCH(), @alert_unique_id) ON CONFLICT (alert_unique_id) DO NOTHING"

void sql_queue_alarm_to_aclk(RRDHOST *host, ALARM_ENTRY *ae, bool skip_filter)
{
    sqlite3_stmt *res_alert = NULL;
    char sql[ACLK_SYNC_QUERY_SIZE];

    if (!service_running(SERVICE_ACLK))
        return;

    if (!claimed() || ae->flags & HEALTH_ENTRY_FLAG_ACLK_QUEUED)
        return;

    if (false == skip_filter && !should_send_to_cloud(host, ae))
            return;

    if (is_event_from_alert_variable_config(ae->unique_id, &host->host_uuid))
        return;

    snprintfz(sql, sizeof(sql) - 1, SQL_QUEUE_ALERT_TO_CLOUD, host->aclk_config->uuid_str);

    int rc = sqlite3_prepare_v2(db_meta, sql, -1, &res_alert, 0);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to prepare statement to store alert event");
        return;
    }

    rc = sqlite3_bind_int64(res_alert, 1, ae->unique_id);
    if (unlikely(rc != SQLITE_OK))
        goto done;

    rc = execute_insert(res_alert);
    if (unlikely(rc == SQLITE_DONE)) {
        ae->flags |= HEALTH_ENTRY_FLAG_ACLK_QUEUED;
        rrdhost_flag_set(host, RRDHOST_FLAG_ACLK_STREAM_ALERTS);
    } else
        error_report("Failed to store alert event %"PRIu32", rc = %d", ae->unique_id, rc);

done:
    if (unlikely(sqlite3_finalize(res_alert) != SQLITE_OK))
        error_report("Failed to reset statement in store alert event, rc = %d", rc);
}

int rrdcalc_status_to_proto_enum(RRDCALC_STATUS status)
{
#ifdef ENABLE_ACLK
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
#else
    UNUSED(status);
    return 1;
#endif
}

static inline char *sqlite3_uuid_unparse_strdupz(sqlite3_stmt *res, int iCol) {
    char uuid_str[UUID_STR_LEN];

    if(sqlite3_column_type(res, iCol) == SQLITE_NULL)
        uuid_str[0] = '\0';
    else
        uuid_unparse_lower(*((uuid_t *) sqlite3_column_blob(res, iCol)), uuid_str);

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


static void aclk_push_alert_event(struct aclk_sync_cfg_t *wc __maybe_unused)
{
#ifdef ENABLE_ACLK
    int rc;

    if (unlikely(!wc->alert_updates)) {
        nd_log(NDLS_ACCESS, NDLP_NOTICE,
            "ACLK STA [%s (%s)]: Ignoring alert push event, updates have been turned off for this node.",
            wc->node_id,
            wc->host ? rrdhost_hostname(wc->host) : "N/A");
        return;
    }

    char *claim_id = get_agent_claimid();
    if (unlikely(!claim_id))
        return;

    if (unlikely(!wc->host)) {
        freez(claim_id);
        return;
    }

    BUFFER *sql = buffer_create(1024, &netdata_buffers_statistics.buffers_sqlite);

    sqlite3_stmt *res = NULL;

    buffer_sprintf(
        sql,
        "SELECT aa.sequence_id, hld.unique_id, hld.alarm_id, hl.config_hash_id, hld.updated_by_id, hld.when_key, "
        " hld.duration, hld.non_clear_duration, hld.flags, hld.exec_run_timestamp, hld.delay_up_to_timestamp, hl.name,  "
        " hl.chart, hl.exec, hl.recipient, ha.source, hl.units, hld.info, hld.exec_code, hld.new_status,  "
        " hld.old_status, hld.delay, hld.new_value, hld.old_value, hld.last_repeat, hl.chart_context, hld.transition_id, "
        " hld.alarm_event_id, hl.chart_name, hld.summary  "
        " FROM health_log hl, aclk_alert_%s aa, alert_hash ha, health_log_detail hld "
        " WHERE hld.unique_id = aa.alert_unique_id AND hl.config_hash_id = ha.hash_id AND aa.date_submitted IS NULL "
        " AND hl.host_id = @host_id AND hl.health_log_id = hld.health_log_id "
        " ORDER BY aa.sequence_id ASC LIMIT "ACLK_MAX_ALERT_UPDATES,
        wc->uuid_str);

    rc = sqlite3_prepare_v2(db_meta, buffer_tostring(sql), -1, &res, 0);
    if (rc != SQLITE_OK) {

        BUFFER *sql_fix = buffer_create(1024, &netdata_buffers_statistics.buffers_sqlite);
        buffer_sprintf(sql_fix, TABLE_ACLK_ALERT, wc->uuid_str);

        rc = db_execute(db_meta, buffer_tostring(sql_fix));
        if (unlikely(rc))
            error_report("Failed to create ACLK alert table for host %s", rrdhost_hostname(wc->host));
        buffer_free(sql_fix);

        // Try again
        rc = sqlite3_prepare_v2(db_meta, buffer_tostring(sql), -1, &res, 0);
        if (rc != SQLITE_OK) {
            error_report("Failed to prepare statement when trying to send an alert update via ACLK");

            buffer_free(sql);
            freez(claim_id);
            return;
        }
    }

    rc = sqlite3_bind_blob(res, 1, &wc->host->host_uuid, sizeof(wc->host->host_uuid), SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to bind host_id for pushing alert event.");
        goto done;
    }

    uint64_t first_sequence_id = 0;
    uint64_t last_sequence_id = 0;

    while (sqlite3_step_monitored(res) == SQLITE_ROW) {
        struct alarm_log_entry alarm_log;
        char old_value_string[100 + 1];
        char new_value_string[100 + 1];

        alarm_log.node_id = wc->node_id;
        alarm_log.claim_id = claim_id;
        alarm_log.chart = strdupz((char *)sqlite3_column_text(res, 12));
        alarm_log.name = strdupz((char *)sqlite3_column_text(res, 11));
        alarm_log.when = (time_t) sqlite3_column_int64(res, 5);
        alarm_log.config_hash = sqlite3_uuid_unparse_strdupz(res, 3);
        alarm_log.utc_offset = wc->host->utc_offset;
        alarm_log.timezone = strdupz(rrdhost_abbrev_timezone(wc->host));
        alarm_log.exec_path = sqlite3_column_bytes(res, 13) > 0 ? strdupz((char *)sqlite3_column_text(res, 13)) :
                                                                  strdupz((char *)string2str(wc->host->health.health_default_exec));
        alarm_log.conf_source = sqlite3_column_bytes(res, 15) > 0 ? strdupz((char *)sqlite3_column_text(res, 15)) : strdupz("");

        char *edit_command = sqlite3_column_bytes(res, 15) > 0 ?
                                 health_edit_command_from_source((char *)sqlite3_column_text(res, 15)) :
                                 strdupz("UNKNOWN=0=UNKNOWN");
        alarm_log.command = strdupz(edit_command);

        alarm_log.duration = (time_t) sqlite3_column_int64(res, 6);
        alarm_log.non_clear_duration = (time_t) sqlite3_column_int64(res, 7);
        alarm_log.status = rrdcalc_status_to_proto_enum((RRDCALC_STATUS) sqlite3_column_int(res, 19));
        alarm_log.old_status = rrdcalc_status_to_proto_enum((RRDCALC_STATUS) sqlite3_column_int(res, 20));
        alarm_log.delay = (int) sqlite3_column_int(res, 21);
        alarm_log.delay_up_to_timestamp = (time_t) sqlite3_column_int64(res, 10);
        alarm_log.last_repeat = (time_t) sqlite3_column_int64(res, 24);
        alarm_log.silenced = ((sqlite3_column_int64(res, 8) & HEALTH_ENTRY_FLAG_SILENCED) ||
                              (sqlite3_column_type(res, 14) != SQLITE_NULL &&
                               !strncmp((char *)sqlite3_column_text(res, 14), "silent", 6))) ?
                                 1 :
                                 0;
        alarm_log.value_string =
            sqlite3_column_type(res, 22) == SQLITE_NULL ?
                strdupz((char *)"-") :
                strdupz((char *)format_value_and_unit(
                    new_value_string, 100, sqlite3_column_double(res, 22), (char *)sqlite3_column_text(res, 16), -1));
        alarm_log.old_value_string =
            sqlite3_column_type(res, 23) == SQLITE_NULL ?
                strdupz((char *)"-") :
                strdupz((char *)format_value_and_unit(
                    old_value_string, 100, sqlite3_column_double(res, 23), (char *)sqlite3_column_text(res, 16), -1));
        alarm_log.value = (NETDATA_DOUBLE) sqlite3_column_double(res, 22);
        alarm_log.old_value = (NETDATA_DOUBLE) sqlite3_column_double(res, 23);
        alarm_log.updated = (sqlite3_column_int64(res, 8) & HEALTH_ENTRY_FLAG_UPDATED) ? 1 : 0;
        alarm_log.rendered_info = sqlite3_text_strdupz_empty(res, 17);
        alarm_log.chart_context = sqlite3_text_strdupz_empty(res, 25);
        alarm_log.transition_id = sqlite3_uuid_unparse_strdupz(res, 26);
        alarm_log.event_id = (time_t) sqlite3_column_int64(res, 27);
        alarm_log.chart_name = sqlite3_text_strdupz_empty(res, 28);
        alarm_log.summary = sqlite3_text_strdupz_empty(res, 29);

        aclk_send_alarm_log_entry(&alarm_log);

        if (first_sequence_id == 0)
            first_sequence_id  = (uint64_t) sqlite3_column_int64(res, 0);

        if (wc->alerts_log_first_sequence_id == 0)
            wc->alerts_log_first_sequence_id  = (uint64_t) sqlite3_column_int64(res, 0);

        last_sequence_id = (uint64_t) sqlite3_column_int64(res, 0);
        wc->alerts_log_last_sequence_id = (uint64_t) sqlite3_column_int64(res, 0);

        destroy_alarm_log_entry(&alarm_log);
        freez(edit_command);
    }

    if (first_sequence_id) {
        buffer_flush(sql);
        buffer_sprintf(
            sql,
            "UPDATE aclk_alert_%s SET date_submitted=unixepoch() "
            "WHERE +date_submitted IS NULL AND sequence_id BETWEEN %" PRIu64 " AND %" PRIu64,
            wc->uuid_str,
            first_sequence_id,
            last_sequence_id);

        if (unlikely(db_execute(db_meta, buffer_tostring(sql))))
            error_report("Failed to mark ACLK alert entries as submitted for host %s", rrdhost_hostname(wc->host));

        // Mark to do one more check
        rrdhost_flag_set(wc->host, RRDHOST_FLAG_ACLK_STREAM_ALERTS);

    } else {
        if (wc->alerts_log_first_sequence_id)
            nd_log(NDLS_ACCESS, NDLP_DEBUG,
                "ACLK RES [%s (%s)]: ALERTS SENT from %" PRIu64 " to %" PRIu64 "",
                wc->node_id,
                wc->host ? rrdhost_hostname(wc->host) : "N/A",
                wc->alerts_log_first_sequence_id,
                wc->alerts_log_last_sequence_id);
        wc->alerts_log_first_sequence_id = 0;
        wc->alerts_log_last_sequence_id = 0;
    }

done:
    rc = sqlite3_finalize(res);
    if (unlikely(rc != SQLITE_OK))
        error_report("Failed to finalize statement to send alert entries from the database, rc = %d", rc);

    freez(claim_id);
    buffer_free(sql);
#endif
}

void aclk_push_alert_events_for_all_hosts(void)
{
    RRDHOST *host;

    dfe_start_reentrant(rrdhost_root_index, host) {
        if (rrdhost_flag_check(host, RRDHOST_FLAG_ARCHIVED) ||
            !rrdhost_flag_check(host, RRDHOST_FLAG_ACLK_STREAM_ALERTS))
            continue;

        rrdhost_flag_clear(host, RRDHOST_FLAG_ACLK_STREAM_ALERTS);

        struct aclk_sync_cfg_t *wc = host->aclk_config;
        if (likely(wc))
            aclk_push_alert_event(wc);
    }
    dfe_done(host);
}

void sql_queue_existing_alerts_to_aclk(RRDHOST *host)
{
    sqlite3_stmt *res = NULL;
    int rc;

    struct aclk_sync_cfg_t *wc = host->aclk_config;

    BUFFER *sql = buffer_create(1024, &netdata_buffers_statistics.buffers_sqlite);

    rw_spinlock_write_lock(&host->health_log.spinlock);

    buffer_sprintf(sql, "DELETE FROM aclk_alert_%s", wc->uuid_str);
    if (unlikely(db_execute(db_meta, buffer_tostring(sql))))
        goto skip;

    buffer_flush(sql);

    buffer_sprintf(
        sql,
        "INSERT INTO aclk_alert_%s (alert_unique_id, date_created, filtered_alert_unique_id) "
        "SELECT hld.unique_id alert_unique_id, unixepoch(), hld.unique_id alert_unique_id FROM health_log_detail hld, health_log hl "
        "WHERE hld.new_status <> 0 AND hld.new_status <> -2 AND hl.health_log_id = hld.health_log_id AND hl.config_hash_id IS NOT NULL "
        "AND hld.updated_by_id = 0 AND hl.host_id = @host_id ORDER BY hld.unique_id ASC ON CONFLICT (alert_unique_id) DO NOTHING",
        wc->uuid_str);

    rc = sqlite3_prepare_v2(db_meta, buffer_tostring(sql), -1, &res, 0);
    if (rc != SQLITE_OK) {
        error_report("Failed to prepare statement when trying to queue existing alerts.");
        goto skip;
    }

    rc = sqlite3_bind_blob(res, 1, &host->host_uuid, sizeof(host->host_uuid), SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to bind host_id for when trying to queue existing alerts.");
        goto done;
    }

    rc = execute_insert(res);
    if (unlikely(rc != SQLITE_DONE))
        error_report("Failed to queue existing alerts, rc = %d", rc);
    else
        rrdhost_flag_set(host, RRDHOST_FLAG_ACLK_STREAM_ALERTS);
done:
    rc = sqlite3_finalize(res);
    if (unlikely(rc != SQLITE_OK))
        error_report("Failed to finalize statement to queue existing alerts, rc = %d", rc);

skip:
    rw_spinlock_write_unlock(&host->health_log.spinlock);
    buffer_free(sql);
}

void aclk_send_alarm_configuration(char *config_hash)
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
#ifdef ENABLE_ACLK
    int rc;

    sqlite3_stmt *res = NULL;
    struct aclk_sync_cfg_t *wc;

    RRDHOST *host = find_host_by_node_id(node_id);

    if (unlikely(!host || !(wc = host->aclk_config))) {
        freez(config_hash);
        freez(node_id);
        return;
    }

    rc = sqlite3_prepare_v2(db_meta, SQL_SELECT_ALERT_CONFIG, -1, &res, 0);
    if (rc != SQLITE_OK) {
        error_report("Failed to prepare statement when trying to fetch an alarm hash configuration");
        return;
    }

    uuid_t hash_uuid;
    if (uuid_parse(config_hash, hash_uuid))
        return;

    rc = sqlite3_bind_blob(res, 1, &hash_uuid , sizeof(hash_uuid), SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    struct aclk_alarm_configuration alarm_config;
    struct provide_alarm_configuration p_alarm_config;
    p_alarm_config.cfg_hash = NULL;

    if (sqlite3_step_monitored(res) == SQLITE_ROW) {

        int param = 0;
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
            buffer_data_options2string(tmp_buf, sqlite3_column_int(res, 28));
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

    if (likely(p_alarm_config.cfg_hash)) {
        nd_log(NDLS_ACCESS, NDLP_DEBUG, "ACLK RES [%s (%s)]: Sent alert config %s.", wc->node_id, wc->host ? rrdhost_hostname(wc->host) : "N/A", config_hash);
        aclk_send_provide_alarm_cfg(&p_alarm_config);
        freez(p_alarm_config.cfg_hash);
        destroy_aclk_alarm_configuration(&alarm_config);
    }
    else
        nd_log(NDLS_ACCESS, NDLP_WARNING, "ACLK STA [%s (%s)]: Alert config for %s not found.", wc->node_id, wc->host ? rrdhost_hostname(wc->host) : "N/A", config_hash);

bind_fail:
    rc = sqlite3_finalize(res);
    if (unlikely(rc != SQLITE_OK))
        error_report("Failed to reset statement when pushing alarm config hash, rc = %d", rc);

    freez(config_hash);
    freez(node_id);
#endif
}


// Start streaming alerts
void aclk_start_alert_streaming(char *node_id, bool resets)
{
    uuid_t node_uuid;

    if (unlikely(!node_id || uuid_parse(node_id, node_uuid)))
        return;

    struct aclk_sync_cfg_t *wc;

    RRDHOST *host = find_host_by_node_id(node_id);
    if (unlikely(!host || !(wc = host->aclk_config)))
        return;

    if (unlikely(!host->health.health_enabled)) {
        nd_log(NDLS_ACCESS, NDLP_NOTICE, "ACLK STA [%s (N/A)]: Ignoring request to stream alert state changes, health is disabled.", node_id);
        return;
    }

    if (resets) {
        nd_log(NDLS_ACCESS, NDLP_DEBUG, "ACLK REQ [%s (%s)]: STREAM ALERTS ENABLED (RESET REQUESTED)", node_id, wc->host ? rrdhost_hostname(wc->host) : "N/A");
        sql_queue_existing_alerts_to_aclk(host);
    } else
        nd_log(NDLS_ACCESS, NDLP_DEBUG, "ACLK REQ [%s (%s)]: STREAM ALERTS ENABLED", node_id, wc->host ? rrdhost_hostname(wc->host) : "N/A");

    wc->alert_updates = 1;
    wc->alert_queue_removed = SEND_REMOVED_AFTER_HEALTH_LOOPS;
}

#define SQL_QUEUE_REMOVE_ALERTS                                                                                                   \
    "INSERT INTO aclk_alert_%s (alert_unique_id, date_created, filtered_alert_unique_id) "                                        \
    "SELECT hld.unique_id alert_unique_id, UNIXEPOCH(), hld.unique_id alert_unique_id FROM health_log hl, health_log_detail hld " \
    "WHERE hl.host_id = @host_id AND hl.health_log_id = hld.health_log_id AND hld.new_status = -2 AND hld.updated_by_id = 0 "     \
    "AND hld.unique_id NOT IN (SELECT alert_unique_id FROM aclk_alert_%s) "                                                       \
    "AND hl.config_hash_id NOT IN (SELECT hash_id FROM alert_hash WHERE warn IS NULL AND crit IS NULL) "                          \
    "AND hl.name || hl.chart NOT IN (select name || chart FROM health_log WHERE name = hl.name AND "                              \
    "chart = hl.chart AND alarm_id > hl.alarm_id AND host_id = hl.host_id) "                                                      \
    "ORDER BY hld.unique_id ASC ON CONFLICT (alert_unique_id) DO NOTHING"

void sql_process_queue_removed_alerts_to_aclk(char *node_id)
{
    struct aclk_sync_cfg_t *wc;
    RRDHOST *host = find_host_by_node_id(node_id);
    freez(node_id);

    if (unlikely(!host || !(wc = host->aclk_config)))
        return;

    char sql[ACLK_SYNC_QUERY_SIZE * 2];
    sqlite3_stmt *res = NULL;

    snprintfz(sql, sizeof(sql) - 1, SQL_QUEUE_REMOVE_ALERTS, wc->uuid_str, wc->uuid_str);

    int rc = sqlite3_prepare_v2(db_meta, sql, -1, &res, 0);
    if (rc != SQLITE_OK) {
        error_report("Failed to prepare statement when trying to queue removed alerts.");
        return;
    }

    rc = sqlite3_bind_blob(res, 1, &host->host_uuid, sizeof(host->host_uuid), SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to bind host_id for when trying to queue remvoed alerts.");
        goto skip;
    }

    rc = execute_insert(res);
    if (likely(rc == SQLITE_DONE)) {
        nd_log(NDLS_ACCESS, NDLP_DEBUG, "ACLK STA [%s (%s)]: QUEUED REMOVED ALERTS", wc->node_id, rrdhost_hostname(wc->host));
        rrdhost_flag_set(wc->host, RRDHOST_FLAG_ACLK_STREAM_ALERTS);
        wc->alert_queue_removed = 0;
    }

skip:
    rc = sqlite3_finalize(res);
    if (unlikely(rc != SQLITE_OK))
        error_report("Failed to finalize statement to queue removed alerts, rc = %d", rc);
}

void sql_queue_removed_alerts_to_aclk(RRDHOST *host)
{
    if (unlikely(!host->aclk_config || !claimed() || !host->node_id))
        return;

    char node_id[UUID_STR_LEN];
    uuid_unparse_lower(*host->node_id, node_id);

    aclk_push_node_removed_alerts(node_id);
}

void aclk_process_send_alarm_snapshot(char *node_id, char *claim_id __maybe_unused, char *snapshot_uuid)
{
    uuid_t node_uuid;

    if (unlikely(!node_id || uuid_parse(node_id, node_uuid)))
        return;

    struct aclk_sync_cfg_t *wc;

    RRDHOST *host = find_host_by_node_id(node_id);
    if (unlikely(!host || !(wc = host->aclk_config))) {
        nd_log(NDLS_ACCESS, NDLP_WARNING, "ACLK STA [%s (N/A)]: ACLK node id does not exist", node_id);
        return;
    }

    nd_log(NDLS_ACCESS, NDLP_DEBUG,
            "IN [%s (%s)]: Request to send alerts snapshot, snapshot_uuid %s",
            node_id,
            wc->host ? rrdhost_hostname(wc->host) : "N/A",
            snapshot_uuid);

    if (wc->alerts_snapshot_uuid && !strcmp(wc->alerts_snapshot_uuid,snapshot_uuid))
        return;

    wc->alerts_snapshot_uuid = strdupz(snapshot_uuid);

    aclk_push_node_alert_snapshot(node_id);
}

#ifdef ENABLE_ACLK
void health_alarm_entry2proto_nolock(struct alarm_log_entry *alarm_log, ALARM_ENTRY *ae, RRDHOST *host)
{
    char *edit_command = ae->source ? health_edit_command_from_source(ae_source(ae)) : strdupz("UNKNOWN=0=UNKNOWN");
    char config_hash_id[UUID_STR_LEN];
    uuid_unparse_lower(ae->config_hash_id, config_hash_id);
    char transition_id[UUID_STR_LEN];
    uuid_unparse_lower(ae->transition_id, transition_id);

    alarm_log->chart = strdupz(ae_chart_id(ae));
    alarm_log->name = strdupz(ae_name(ae));

    alarm_log->when = ae->when;

    alarm_log->config_hash = strdupz((char *)config_hash_id);

    alarm_log->utc_offset = host->utc_offset;
    alarm_log->timezone = strdupz(rrdhost_abbrev_timezone(host));
    alarm_log->exec_path = ae->exec ? strdupz(ae_exec(ae)) : strdupz((char *)string2str(host->health.health_default_exec));
    alarm_log->conf_source = ae->source ? strdupz(ae_source(ae)) : strdupz((char *)"");

    alarm_log->command = strdupz((char *)edit_command);

    alarm_log->duration = (time_t)ae->duration;
    alarm_log->non_clear_duration = (time_t)ae->non_clear_duration;
    alarm_log->status = rrdcalc_status_to_proto_enum((RRDCALC_STATUS)ae->new_status);
    alarm_log->old_status = rrdcalc_status_to_proto_enum((RRDCALC_STATUS)ae->old_status);
    alarm_log->delay = ae->delay;
    alarm_log->delay_up_to_timestamp = (time_t)ae->delay_up_to_timestamp;
    alarm_log->last_repeat = (time_t)ae->last_repeat;

    alarm_log->silenced =
        ((ae->flags & HEALTH_ENTRY_FLAG_SILENCED) || (ae->recipient && !strncmp(ae_recipient(ae), "silent", 6))) ?
            1 :
            0;

    alarm_log->value_string = strdupz(ae_new_value_string(ae));
    alarm_log->old_value_string = strdupz(ae_old_value_string(ae));

    alarm_log->value = (!isnan(ae->new_value)) ? (NETDATA_DOUBLE)ae->new_value : 0;
    alarm_log->old_value = (!isnan(ae->old_value)) ? (NETDATA_DOUBLE)ae->old_value : 0;

    alarm_log->updated = (ae->flags & HEALTH_ENTRY_FLAG_UPDATED) ? 1 : 0;
    alarm_log->rendered_info = strdupz(ae_info(ae));
    alarm_log->chart_context = strdupz(ae_chart_context(ae));
    alarm_log->chart_name = strdupz(ae_chart_name(ae));

    alarm_log->transition_id = strdupz((char *)transition_id);
    alarm_log->event_id = (uint64_t) ae->alarm_event_id;

    alarm_log->summary = strdupz(ae_summary(ae));

    freez(edit_command);
}
#endif

#ifdef ENABLE_ACLK
static bool have_recent_alarm_unsafe(RRDHOST *host, int64_t alarm_id, int64_t mark)
{
    ALARM_ENTRY *ae = host->health_log.alarms;

    while (ae) {
        if (ae->alarm_id == alarm_id && ae->unique_id >mark &&
            (ae->new_status != RRDCALC_STATUS_WARNING && ae->new_status != RRDCALC_STATUS_CRITICAL))
            return true;
        ae = ae->next;
    }

    return false;
}
#endif

#define ALARM_EVENTS_PER_CHUNK 1000
void aclk_push_alert_snapshot_event(char *node_id __maybe_unused)
{
#ifdef ENABLE_ACLK
    RRDHOST *host = find_host_by_node_id(node_id);

    if (unlikely(!host)) {
    nd_log(NDLS_ACCESS, NDLP_WARNING, "AC [%s (N/A)]: Node id not found", node_id);
        freez(node_id);
        return;
    }
    freez(node_id);

    struct aclk_sync_cfg_t *wc = host->aclk_config;

    // we perhaps we don't need this for snapshots
    if (unlikely(!wc->alert_updates)) {
        nd_log(NDLS_ACCESS, NDLP_NOTICE,
            "ACLK STA [%s (%s)]: Ignoring alert snapshot event, updates have been turned off for this node.",
            wc->node_id,
            wc->host ? rrdhost_hostname(wc->host) : "N/A");
        return;
    }

    if (unlikely(!wc->alerts_snapshot_uuid))
        return;

    char *claim_id = get_agent_claimid();
    if (unlikely(!claim_id))
        return;

    nd_log(NDLS_ACCESS, NDLP_DEBUG, "ACLK REQ [%s (%s)]: Sending alerts snapshot, snapshot_uuid %s", wc->node_id, rrdhost_hostname(wc->host), wc->alerts_snapshot_uuid);

    uint32_t cnt = 0;

    rw_spinlock_read_lock(&host->health_log.spinlock);

    ALARM_ENTRY *ae = host->health_log.alarms;

    for (; ae; ae = ae->next) {
        if (likely(ae->updated_by_id))
            continue;

        if (unlikely(ae->new_status == RRDCALC_STATUS_UNINITIALIZED))
            continue;

        if (have_recent_alarm_unsafe(host, ae->alarm_id, ae->unique_id))
            continue;

        if (is_event_from_alert_variable_config(ae->unique_id, &host->host_uuid))
            continue;

        cnt++;
    }

    if (cnt) {
        uint32_t chunks;

        chunks = (cnt / ALARM_EVENTS_PER_CHUNK) + (cnt % ALARM_EVENTS_PER_CHUNK != 0);
        ae = host->health_log.alarms;

        cnt = 0;
        struct alarm_snapshot alarm_snap;
        alarm_snap.node_id = wc->node_id;
        alarm_snap.claim_id = claim_id;
        alarm_snap.snapshot_uuid = wc->alerts_snapshot_uuid;
        alarm_snap.chunks = chunks;
        alarm_snap.chunk = 1;

        alarm_snapshot_proto_ptr_t snapshot_proto = NULL;

        for (; ae; ae = ae->next) {
            if (likely(ae->updated_by_id) || unlikely(ae->new_status == RRDCALC_STATUS_UNINITIALIZED))
                continue;

            if (have_recent_alarm_unsafe(host, ae->alarm_id, ae->unique_id))
                continue;

            if (is_event_from_alert_variable_config(ae->unique_id, &host->host_uuid))
                continue;

            cnt++;

            struct alarm_log_entry alarm_log;
            alarm_log.node_id = wc->node_id;
            alarm_log.claim_id = claim_id;

            if (!snapshot_proto)
                snapshot_proto = generate_alarm_snapshot_proto(&alarm_snap);

            health_alarm_entry2proto_nolock(&alarm_log, ae, host);
            add_alarm_log_entry2snapshot(snapshot_proto, &alarm_log);

            if (cnt == ALARM_EVENTS_PER_CHUNK) {
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
    }

    rw_spinlock_read_unlock(&host->health_log.spinlock);
    wc->alerts_snapshot_uuid = NULL;

    freez(claim_id);
#endif
}

#define SQL_DELETE_ALERT_ENTRIES "DELETE FROM aclk_alert_%s WHERE date_created < UNIXEPOCH() - @period"

void sql_aclk_alert_clean_dead_entries(RRDHOST *host)
{
    struct aclk_sync_cfg_t *wc = host->aclk_config;
    if (unlikely(!wc))
        return;

    char sql[ACLK_SYNC_QUERY_SIZE];
    snprintfz(sql, sizeof(sql) - 1, SQL_DELETE_ALERT_ENTRIES, wc->uuid_str);

    sqlite3_stmt *res = NULL;
    int rc = sqlite3_prepare_v2(db_meta, sql, -1, &res, 0);
    if (rc != SQLITE_OK) {
        error_report("Failed to prepare statement for cleaning stale ACLK alert entries.");
        return;
    }

    rc = sqlite3_bind_int64(res, 1, MAX_REMOVED_PERIOD);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to bind MAX_REMOVED_PERIOD parameter.");
        goto skip;
    }

    rc = sqlite3_step_monitored(res);
    if (rc != SQLITE_DONE)
        error_report("Failed to execute DELETE query for cleaning stale ACLK alert entries.");

skip:
    rc = sqlite3_finalize(res);
    if (unlikely(rc != SQLITE_OK))
        error_report("Failed to finalize statement for cleaning stale ACLK alert entries.");
}

#define SQL_GET_MIN_MAX_ALERT_SEQ "SELECT MIN(sequence_id), MAX(sequence_id), " \
                                  "(SELECT MAX(sequence_id) FROM aclk_alert_%s WHERE date_submitted IS NOT NULL) " \
                                  "FROM aclk_alert_%s WHERE date_submitted IS NULL"
int get_proto_alert_status(RRDHOST *host, struct proto_alert_status *proto_alert_status)
{

    struct aclk_sync_cfg_t *wc = host->aclk_config;
    if (!wc)
        return 1;

    proto_alert_status->alert_updates = wc->alert_updates;

    char sql[ACLK_SYNC_QUERY_SIZE];

    sqlite3_stmt *res = NULL;
    snprintfz(sql, sizeof(sql) - 1, SQL_GET_MIN_MAX_ALERT_SEQ, wc->uuid_str, wc->uuid_str);

    int rc = sqlite3_prepare_v2(db_meta, sql, -1, &res, 0);
    if (rc != SQLITE_OK) {
        error_report("Failed to prepare statement to get alert log status from the database.");
        return 1;
    }

    while (sqlite3_step_monitored(res) == SQLITE_ROW) {
        proto_alert_status->pending_min_sequence_id =
            sqlite3_column_bytes(res, 0) > 0 ? (uint64_t)sqlite3_column_int64(res, 0) : 0;
        proto_alert_status->pending_max_sequence_id =
            sqlite3_column_bytes(res, 1) > 0 ? (uint64_t)sqlite3_column_int64(res, 1) : 0;
        proto_alert_status->last_submitted_sequence_id =
            sqlite3_column_bytes(res, 2) > 0 ? (uint64_t)sqlite3_column_int64(res, 2) : 0;
    }

    rc = sqlite3_finalize(res);
    if (unlikely(rc != SQLITE_OK))
        error_report("Failed to finalize statement to get alert log status from the database, rc = %d", rc);

    return 0;
}

void aclk_send_alarm_checkpoint(char *node_id, char *claim_id __maybe_unused)
{
    if (unlikely(!node_id))
        return;

    struct aclk_sync_cfg_t *wc;
    RRDHOST *host = find_host_by_node_id(node_id);

    if (unlikely(!host || !(wc = host->aclk_config)))
        nd_log(NDLS_ACCESS, NDLP_WARNING, "ACLK REQ [%s (N/A)]: ALERTS CHECKPOINT REQUEST RECEIVED FOR INVALID NODE", node_id);
    else {
        nd_log(NDLS_ACCESS, NDLP_DEBUG, "ACLK REQ [%s (%s)]: ALERTS CHECKPOINT REQUEST RECEIVED", node_id, rrdhost_hostname(host));
        wc->alert_checkpoint_req = SEND_CHECKPOINT_AFTER_HEALTH_LOOPS;
    }
}

typedef struct active_alerts {
    char *name;
    char *chart;
    RRDCALC_STATUS status;
} active_alerts_t;

static inline int compare_active_alerts(const void *a, const void *b)
{
    active_alerts_t *active_alerts_a = (active_alerts_t *)a;
    active_alerts_t *active_alerts_b = (active_alerts_t *)b;

    if (!(strcmp(active_alerts_a->name, active_alerts_b->name))) {
        return strcmp(active_alerts_a->chart, active_alerts_b->chart);
    } else
        return strcmp(active_alerts_a->name, active_alerts_b->name);
}

#define BATCH_ALLOCATED 10
void aclk_push_alarm_checkpoint(RRDHOST *host __maybe_unused)
{
#ifdef ENABLE_ACLK
    struct aclk_sync_cfg_t *wc = host->aclk_config;
    if (unlikely(!wc)) {
        nd_log(NDLS_ACCESS, NDLP_WARNING, "ACLK REQ [%s (N/A)]: ALERTS CHECKPOINT REQUEST RECEIVED FOR INVALID NODE", rrdhost_hostname(host));
        return;
    }

    if (rrdhost_flag_check(host, RRDHOST_FLAG_ACLK_STREAM_ALERTS)) {
        //postpone checkpoint send
        wc->alert_checkpoint_req += 3;
        nd_log(NDLS_ACCESS, NDLP_NOTICE, "ACLK REQ [%s (N/A)]: ALERTS CHECKPOINT POSTPONED", rrdhost_hostname(host));
        return;
    }

    RRDCALC *rc;
    uint32_t cnt = 0;
    size_t len = 0;

    active_alerts_t *active_alerts = callocz(BATCH_ALLOCATED, sizeof(active_alerts_t));
    foreach_rrdcalc_in_rrdhost_read(host, rc) {
        if(unlikely(!rc->rrdset || !rc->rrdset->last_collected_time.tv_sec))
            continue;

        if (rc->status == RRDCALC_STATUS_WARNING ||
            rc->status == RRDCALC_STATUS_CRITICAL) {

            if (cnt && !(cnt % BATCH_ALLOCATED)) {
                active_alerts = reallocz(active_alerts, (BATCH_ALLOCATED * ((cnt / BATCH_ALLOCATED) + 1)) * sizeof(active_alerts_t));
            }

            active_alerts[cnt].name = (char *)rrdcalc_name(rc);
            len += string_strlen(rc->config.name);
            active_alerts[cnt].chart = (char *)rrdcalc_chart_name(rc);
            len += string_strlen(rc->chart);
            active_alerts[cnt].status = rc->status;
            len++;
            cnt++;
        }
    }
    foreach_rrdcalc_in_rrdhost_done(rc);

    BUFFER *alarms_to_hash;
    if (cnt) {
        qsort(active_alerts, cnt, sizeof(active_alerts_t), compare_active_alerts);

        alarms_to_hash = buffer_create(len, NULL);
        for (uint32_t i = 0; i < cnt; i++) {
            buffer_strcat(alarms_to_hash, active_alerts[i].name);
            buffer_strcat(alarms_to_hash, active_alerts[i].chart);
            if (active_alerts[i].status == RRDCALC_STATUS_WARNING)
                buffer_fast_strcat(alarms_to_hash, "W", 1);
            else if (active_alerts[i].status == RRDCALC_STATUS_CRITICAL)
                buffer_fast_strcat(alarms_to_hash, "C", 1);
        }
    } else {
        alarms_to_hash = buffer_create(1, NULL);
        buffer_strcat(alarms_to_hash, "");
        len = 0;
    }
    freez(active_alerts);

    char hash[SHA256_DIGEST_LENGTH + 1];
    if (hash256_string((const unsigned char *)buffer_tostring(alarms_to_hash), len, hash)) {
        hash[SHA256_DIGEST_LENGTH] = 0;

        struct alarm_checkpoint alarm_checkpoint;
        char *claim_id = get_agent_claimid();
        alarm_checkpoint.claim_id = claim_id;
        alarm_checkpoint.node_id = wc->node_id;
        alarm_checkpoint.checksum = (char *)hash;

        aclk_send_provide_alarm_checkpoint(&alarm_checkpoint);
        freez(claim_id);
        nd_log(NDLS_ACCESS, NDLP_DEBUG, "ACLK RES [%s (%s)]: ALERTS CHECKPOINT SENT", wc->node_id, rrdhost_hostname(host));
    } else
        nd_log(NDLS_ACCESS, NDLP_ERR, "ACLK RES [%s (%s)]: FAILED TO CREATE ALERTS CHECKPOINT HASH", wc->node_id, rrdhost_hostname(host));

    wc->alert_checkpoint_req = 0;
    buffer_free(alarms_to_hash);
#endif
}
