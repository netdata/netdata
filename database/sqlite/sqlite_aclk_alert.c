// SPDX-License-Identifier: GPL-3.0-or-later

#include "sqlite_functions.h"
#include "sqlite_aclk_alert.h"

#ifdef ENABLE_ACLK
#include "../../aclk/aclk_alarm_api.h"
#endif

#define SQL_GET_ALERT_REMOVE_TIME "SELECT when_key FROM health_log_%s WHERE alarm_id = %u " \
                                  "AND unique_id > %u AND unique_id < %u " \
                                  "AND new_status = -2;"

time_t removed_when(uint32_t alarm_id, uint32_t before_unique_id, uint32_t after_unique_id, char *uuid_str) {
    sqlite3_stmt *res = NULL;
    time_t when = 0;
    char sql[ACLK_SYNC_QUERY_SIZE];

    snprintfz(sql,ACLK_SYNC_QUERY_SIZE-1, SQL_GET_ALERT_REMOVE_TIME, uuid_str, alarm_id, after_unique_id, before_unique_id);

    int rc = sqlite3_prepare_v2(db_meta, sql, -1, &res, 0);
    if (rc != SQLITE_OK) {
        error_report("Failed to prepare statement when trying to find removed gap.");
        return 0;
    }

    rc = sqlite3_step_monitored(res);
    if (likely(rc == SQLITE_ROW)) {
        when = (time_t) sqlite3_column_int64(res, 0);
    }

    rc = sqlite3_finalize(res);
    if (unlikely(rc != SQLITE_OK))
        error_report("Failed to finalize statement when trying to find removed gap, rc = %d", rc);

    return when;
}

#define SQL_UPDATE_FILTERED_ALERT "UPDATE aclk_alert_%s SET filtered_alert_unique_id = %u where filtered_alert_unique_id = %u"

void update_filtered(ALARM_ENTRY *ae, uint32_t unique_id, char *uuid_str) {
    char sql[ACLK_SYNC_QUERY_SIZE];
    snprintfz(sql, ACLK_SYNC_QUERY_SIZE-1, SQL_UPDATE_FILTERED_ALERT, uuid_str, ae->unique_id, unique_id);
    sqlite3_exec_monitored(db_meta, sql, 0, 0, NULL);
    ae->flags |= HEALTH_ENTRY_FLAG_ACLK_QUEUED;
}

#define SQL_SELECT_ALERT_BY_UNIQUE_ID "SELECT hl.unique_id FROM health_log_%s hl, alert_hash ah WHERE hl.unique_id = %u " \
                            "AND hl.config_hash_id = ah.hash_id " \
                            "AND ah.warn IS NULL AND ah.crit IS NULL;"

static inline bool is_event_from_alert_variable_config(uint32_t unique_id, char *uuid_str) {
    sqlite3_stmt *res = NULL;
    int rc = 0;
    bool ret = false;

    char sql[ACLK_SYNC_QUERY_SIZE];
    snprintfz(sql,ACLK_SYNC_QUERY_SIZE-1, SQL_SELECT_ALERT_BY_UNIQUE_ID, uuid_str, unique_id);

    rc = sqlite3_prepare_v2(db_meta, sql, -1, &res, 0);
    if (rc != SQLITE_OK) {
        error_report("Failed to prepare statement when trying to check for alert variables.");
        return false;
    }

    rc = sqlite3_step_monitored(res);
    if (likely(rc == SQLITE_ROW)) {
        ret = true;
    }

    rc = sqlite3_finalize(res);
    if (unlikely(rc != SQLITE_OK))
        error_report("Failed to finalize statement when trying to check for alert variables, rc = %d", rc);

    return ret;
}

#define MAX_REMOVED_PERIOD 604800 //a week
//decide if some events should be sent or not

#define SQL_SELECT_ALERT_BY_ID  "SELECT hl.new_status, hl.config_hash_id, hl.unique_id FROM health_log_%s hl, aclk_alert_%s aa " \
                                "WHERE hl.unique_id = aa.filtered_alert_unique_id " \
                                "AND hl.alarm_id = %u " \
                                "ORDER BY alarm_event_id DESC LIMIT 1;"

int should_send_to_cloud(RRDHOST *host, ALARM_ENTRY *ae)
{
    sqlite3_stmt *res = NULL;
    char uuid_str[UUID_STR_LEN];
    uuid_unparse_lower_fix(&host->host_uuid, uuid_str);
    int send = 1;

    if (ae->new_status == RRDCALC_STATUS_REMOVED || ae->new_status == RRDCALC_STATUS_UNINITIALIZED) {
        return 0;
    }

    if (unlikely(uuid_is_null(ae->config_hash_id))) 
        return 0;

    char sql[ACLK_SYNC_QUERY_SIZE];
    uuid_t config_hash_id;
    RRDCALC_STATUS status;
    uint32_t unique_id;

    //get the previous sent event of this alarm_id
    //base the search on the last filtered event
    snprintfz(sql,ACLK_SYNC_QUERY_SIZE-1, SQL_SELECT_ALERT_BY_ID, uuid_str, uuid_str, ae->alarm_id);

    int rc = sqlite3_prepare_v2(db_meta, sql, -1, &res, 0);
    if (rc != SQLITE_OK) {
        error_report("Failed to prepare statement when trying to filter alert events.");
        send = 1;
        return send;
    }

    rc = sqlite3_step_monitored(res);
    if (likely(rc == SQLITE_ROW)) {
        status  = (RRDCALC_STATUS) sqlite3_column_int(res, 0);
        if (sqlite3_column_type(res, 1) != SQLITE_NULL)
            uuid_copy(config_hash_id, *((uuid_t *) sqlite3_column_blob(res, 1)));
        unique_id = (uint32_t) sqlite3_column_int64(res, 2);
        
    } else {
        send = 1;
        goto done;
    }

    if (ae->new_status != (RRDCALC_STATUS)status) {
        send = 1;
        goto done;
    }

    if (uuid_memcmp(&ae->config_hash_id, &config_hash_id)) {
        send = 1;
        goto done;
    }

    //same status, same config
    if (ae->new_status == RRDCALC_STATUS_CLEAR || ae->new_status == RRDCALC_STATUS_UNDEFINED) {
        send = 0;
        update_filtered(ae, unique_id, uuid_str);
        goto done;
    }

    //detect a long off period of the agent, TODO make global
    if (ae->new_status == RRDCALC_STATUS_WARNING || ae->new_status == RRDCALC_STATUS_CRITICAL) {
        time_t when = removed_when(ae->alarm_id, ae->unique_id, unique_id, uuid_str);

        if (when && (when + (time_t)MAX_REMOVED_PERIOD) < ae->when) {
            send = 1;
            goto done;
        } else {
            send = 0;
            update_filtered(ae, unique_id, uuid_str);
            goto done;
        }
    }
     
done:
    rc = sqlite3_finalize(res);
    if (unlikely(rc != SQLITE_OK))
        error_report("Failed to finalize statement when trying to filter alert events, rc = %d", rc);

    return send;
}

// will replace call to aclk_update_alarm in health/health_log.c
// and handle both cases

#define SQL_QUEUE_ALERT_TO_CLOUD "INSERT INTO aclk_alert_%s (alert_unique_id, date_created, filtered_alert_unique_id) " \
                            "VALUES (@alert_unique_id, unixepoch(), @alert_unique_id) ON CONFLICT (alert_unique_id) do nothing;"

int sql_queue_alarm_to_aclk(RRDHOST *host, ALARM_ENTRY *ae, int skip_filter)
{
    if(!service_running(SERVICE_ACLK))
        return 0;

    if (!claimed())
        return 0;

    if (ae->flags & HEALTH_ENTRY_FLAG_ACLK_QUEUED) {
        return 0;
    }

    CHECK_SQLITE_CONNECTION(db_meta);

    if (!skip_filter) {
        if (!should_send_to_cloud(host, ae)) {
            return 0;
        }
    }

    char uuid_str[UUID_STR_LEN];
    uuid_unparse_lower_fix(&host->host_uuid, uuid_str);

    if (is_event_from_alert_variable_config(ae->unique_id, uuid_str))
        return 0;

    sqlite3_stmt *res_alert = NULL;
    char sql[ACLK_SYNC_QUERY_SIZE];

    snprintfz(sql, ACLK_SYNC_QUERY_SIZE - 1, SQL_QUEUE_ALERT_TO_CLOUD, uuid_str);

    int rc = sqlite3_prepare_v2(db_meta, sql, -1, &res_alert, 0);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to prepare statement to store alert event");
        return 1;
    }

    rc = sqlite3_bind_int(res_alert, 1, (int) ae->unique_id);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    rc = execute_insert(res_alert);
    if (unlikely(rc != SQLITE_DONE)) {
        error_report("Failed to store alert event %u, rc = %d", ae->unique_id, rc);
        goto bind_fail;
    }

    ae->flags |= HEALTH_ENTRY_FLAG_ACLK_QUEUED;
    rrdhost_flag_set(host, RRDHOST_FLAG_ACLK_STREAM_ALERTS);

bind_fail:
    if (unlikely(sqlite3_finalize(res_alert) != SQLITE_OK))
        error_report("Failed to reset statement in store alert event, rc = %d", rc);

    return 0;
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


void aclk_push_alert_event(struct aclk_sync_host_config *wc)
{
#ifndef ENABLE_ACLK
    UNUSED(wc);
#else
    int rc;

    if (unlikely(!wc->alert_updates)) {
        log_access("ACLK STA [%s (%s)]: Ignoring alert push event, updates have been turned off for this node.", wc->node_id, wc->host ? rrdhost_hostname(wc->host) : "N/A");
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

    int limit = ACLK_MAX_ALERT_UPDATES;

    sqlite3_stmt *res = NULL;

    buffer_sprintf(sql, "select aa.sequence_id, hl.unique_id, hl.alarm_id, hl.config_hash_id, hl.updated_by_id, hl.when_key, " \
        " hl.duration, hl.non_clear_duration, hl.flags, hl.exec_run_timestamp, hl.delay_up_to_timestamp, hl.name,  " \
        " hl.chart, hl.family, hl.exec, hl.recipient, hl.source, hl.units, hl.info, hl.exec_code, hl.new_status,  " \
        " hl.old_status, hl.delay, hl.new_value, hl.old_value, hl.last_repeat, hl.chart_context, hl.transition_id, hl.alarm_event_id  " \
        " from health_log_%s hl, aclk_alert_%s aa " \
        " where hl.unique_id = aa.alert_unique_id and aa.date_submitted is null " \
        " order by aa.sequence_id asc limit %d;", wc->uuid_str, wc->uuid_str, limit);

    rc = sqlite3_prepare_v2(db_meta, buffer_tostring(sql), -1, &res, 0);
    if (rc != SQLITE_OK) {

        // Try to create tables
        if (wc->host)
            sql_create_health_log_table(wc->host);

        BUFFER *sql_fix = buffer_create(1024, &netdata_buffers_statistics.buffers_sqlite);
        buffer_sprintf(sql_fix, TABLE_ACLK_ALERT, wc->uuid_str);
        rc = db_execute(db_meta, buffer_tostring(sql_fix));
        if (unlikely(rc))
            error_report("Failed to create ACLK alert table for host %s", rrdhost_hostname(wc->host));
        else {
            buffer_flush(sql_fix);
            buffer_sprintf(sql_fix, INDEX_ACLK_ALERT, wc->uuid_str, wc->uuid_str);
            if (unlikely(db_execute(db_meta, buffer_tostring(sql_fix))))
                error_report("Failed to create ACLK alert table for host %s", rrdhost_hostname(wc->host));
        }
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

    uint64_t  first_sequence_id = 0;
    uint64_t  last_sequence_id = 0;
    static __thread uint64_t log_first_sequence_id = 0;
    static __thread uint64_t log_last_sequence_id = 0;

    while (sqlite3_step_monitored(res) == SQLITE_ROW) {
        struct alarm_log_entry alarm_log;
        char old_value_string[100 + 1];
        char new_value_string[100 + 1];

        alarm_log.node_id = wc->node_id;
        alarm_log.claim_id = claim_id;

        alarm_log.chart = strdupz((char *)sqlite3_column_text(res, 12));
        alarm_log.name = strdupz((char *)sqlite3_column_text(res, 11));
        alarm_log.family = sqlite3_column_bytes(res, 13) > 0 ? strdupz((char *)sqlite3_column_text(res, 13)) : NULL;

        //alarm_log.batch_id = wc->alerts_batch_id;
        //alarm_log.sequence_id = (uint64_t) sqlite3_column_int64(res, 0);
        alarm_log.when = (time_t) sqlite3_column_int64(res, 5);

        alarm_log.config_hash = sqlite3_uuid_unparse_strdupz(res, 3);

        alarm_log.utc_offset = wc->host->utc_offset;
        alarm_log.timezone = strdupz(rrdhost_abbrev_timezone(wc->host));
        alarm_log.exec_path = sqlite3_column_bytes(res, 14) > 0 ? strdupz((char *)sqlite3_column_text(res, 14)) :
                                                                  strdupz((char *)string2str(wc->host->health.health_default_exec));
        alarm_log.conf_source = strdupz((char *)sqlite3_column_text(res, 16));

        char *edit_command = sqlite3_column_bytes(res, 16) > 0 ?
                                 health_edit_command_from_source((char *)sqlite3_column_text(res, 16)) :
                                 strdupz("UNKNOWN=0=UNKNOWN");
        alarm_log.command = strdupz(edit_command);

        alarm_log.duration = (time_t) sqlite3_column_int64(res, 6);
        alarm_log.non_clear_duration = (time_t) sqlite3_column_int64(res, 7);
        alarm_log.status = rrdcalc_status_to_proto_enum((RRDCALC_STATUS) sqlite3_column_int(res, 20));
        alarm_log.old_status = rrdcalc_status_to_proto_enum((RRDCALC_STATUS) sqlite3_column_int(res, 21));
        alarm_log.delay = (int) sqlite3_column_int(res, 22);
        alarm_log.delay_up_to_timestamp = (time_t) sqlite3_column_int64(res, 10);
        alarm_log.last_repeat = (time_t) sqlite3_column_int64(res, 25);

        alarm_log.silenced = ((sqlite3_column_int64(res, 8) & HEALTH_ENTRY_FLAG_SILENCED) ||
                              (sqlite3_column_type(res, 15) != SQLITE_NULL &&
                               !strncmp((char *)sqlite3_column_text(res, 15), "silent", 6))) ?
                                 1 :
                                 0;

        alarm_log.value_string =
            sqlite3_column_type(res, 23) == SQLITE_NULL ?
                strdupz((char *)"-") :
                strdupz((char *)format_value_and_unit(
                    new_value_string, 100, sqlite3_column_double(res, 23), (char *)sqlite3_column_text(res, 17), -1));

        alarm_log.old_value_string =
            sqlite3_column_type(res, 24) == SQLITE_NULL ?
                strdupz((char *)"-") :
                strdupz((char *)format_value_and_unit(
                    old_value_string, 100, sqlite3_column_double(res, 24), (char *)sqlite3_column_text(res, 17), -1));

        alarm_log.value = (NETDATA_DOUBLE) sqlite3_column_double(res, 23);
        alarm_log.old_value = (NETDATA_DOUBLE) sqlite3_column_double(res, 24);

        alarm_log.updated = (sqlite3_column_int64(res, 8) & HEALTH_ENTRY_FLAG_UPDATED) ? 1 : 0;
        alarm_log.rendered_info = sqlite3_text_strdupz_empty(res, 18);

        alarm_log.chart_context = sqlite3_text_strdupz_empty(res, 26);
        alarm_log.transition_id = sqlite3_uuid_unparse_strdupz(res, 27);

        alarm_log.event_id = (time_t) sqlite3_column_int64(res, 28);

        aclk_send_alarm_log_entry(&alarm_log);

        if (first_sequence_id == 0)
            first_sequence_id  = (uint64_t) sqlite3_column_int64(res, 0);

        if (log_first_sequence_id == 0)
            log_first_sequence_id  = (uint64_t) sqlite3_column_int64(res, 0);

        last_sequence_id = (uint64_t) sqlite3_column_int64(res, 0);
        log_last_sequence_id = (uint64_t) sqlite3_column_int64(res, 0);

        destroy_alarm_log_entry(&alarm_log);
        freez(edit_command);
    }

    if (first_sequence_id) {
        buffer_flush(sql);
        buffer_sprintf(sql, "UPDATE aclk_alert_%s SET date_submitted=unixepoch() "
                            "WHERE date_submitted IS NULL AND sequence_id BETWEEN %" PRIu64 " AND %" PRIu64 ";",
                       wc->uuid_str, first_sequence_id, last_sequence_id);

        if (unlikely(db_execute(db_meta, buffer_tostring(sql))))
            error_report("Failed to mark ACLK alert entries as submitted for host %s", rrdhost_hostname(wc->host));

        // Mark to do one more check
        rrdhost_flag_set(wc->host, RRDHOST_FLAG_ACLK_STREAM_ALERTS);

    } else {
        if (log_first_sequence_id)
            log_access(
                "ACLK RES [%s (%s)]: ALERTS SENT from %" PRIu64 " to %" PRIu64 "",
                wc->node_id,
                wc->host ? rrdhost_hostname(wc->host) : "N/A",
                log_first_sequence_id,
                log_last_sequence_id);
        log_first_sequence_id = 0;
        log_last_sequence_id = 0;
    }

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
        if (rrdhost_flag_check(host, RRDHOST_FLAG_ARCHIVED) || !rrdhost_flag_check(host, RRDHOST_FLAG_ACLK_STREAM_ALERTS))
            continue;

        internal_error(true, "ACLK SYNC: Scanning host %s", rrdhost_hostname(host));
        rrdhost_flag_clear(host, RRDHOST_FLAG_ACLK_STREAM_ALERTS);

        struct aclk_sync_host_config *wc = host->aclk_sync_host_config;
        if (likely(wc))
            aclk_push_alert_event(wc);
    }
    dfe_done(host);
}

void sql_queue_existing_alerts_to_aclk(RRDHOST *host)
{
    char uuid_str[UUID_STR_LEN];
    uuid_unparse_lower_fix(&host->host_uuid, uuid_str);
    BUFFER *sql = buffer_create(1024, &netdata_buffers_statistics.buffers_sqlite);

    buffer_sprintf(sql,"delete from aclk_alert_%s; " \
                       "insert into aclk_alert_%s (alert_unique_id, date_created, filtered_alert_unique_id) " \
                       "select unique_id alert_unique_id, unixepoch(), unique_id alert_unique_id from health_log_%s " \
                       "where new_status <> 0 and new_status <> -2 and config_hash_id is not null and updated_by_id = 0 " \
                       "order by unique_id asc on conflict (alert_unique_id) do nothing;", uuid_str, uuid_str, uuid_str);

    netdata_rwlock_rdlock(&host->health_log.alarm_log_rwlock);

    if (unlikely(db_execute(db_meta, buffer_tostring(sql))))
        error_report("Failed to queue existing ACLK alert events for host %s", rrdhost_hostname(host));

    netdata_rwlock_unlock(&host->health_log.alarm_log_rwlock);

    buffer_free(sql);
    rrdhost_flag_set(host, RRDHOST_FLAG_ACLK_STREAM_ALERTS);
}

void aclk_send_alarm_configuration(char *config_hash)
{
    if (unlikely(!config_hash))
        return;

    struct aclk_sync_host_config *wc = (struct aclk_sync_host_config *) localhost->aclk_sync_host_config;

    if (unlikely(!wc))
        return;

    log_access("ACLK REQ [%s (%s)]: Request to send alert config %s.", wc->node_id, wc->host ? rrdhost_hostname(wc->host) : "N/A", config_hash);

    aclk_push_alert_config(wc->node_id, config_hash);
}

#define SQL_SELECT_ALERT_CONFIG "SELECT alarm, template, on_key, class, type, component, os, hosts, plugin," \
    "module, charts, families, lookup, every, units, green, red, calc, warn, crit, to_key, exec, delay, repeat, info," \
    "options, host_labels, p_db_lookup_dimensions, p_db_lookup_method, p_db_lookup_options, p_db_lookup_after," \
    "p_db_lookup_before, p_update_every FROM alert_hash WHERE hash_id = @hash_id;"

int aclk_push_alert_config_event(char *node_id __maybe_unused, char *config_hash __maybe_unused)
{
    int rc = 0;

#ifdef ENABLE_ACLK

    CHECK_SQLITE_CONNECTION(db_meta);

    sqlite3_stmt *res = NULL;

    struct aclk_sync_host_config *wc = NULL;
    RRDHOST *host = find_host_by_node_id(node_id);

    if (unlikely(!host)) {
        freez(config_hash);
        freez(node_id);
        return 1;
    }

    wc = (struct aclk_sync_host_config *)host->aclk_sync_host_config;
    if (unlikely(!wc)) {
        freez(config_hash);
        freez(node_id);
        return 1;
    }

    rc = sqlite3_prepare_v2(db_meta, SQL_SELECT_ALERT_CONFIG, -1, &res, 0);
    if (rc != SQLITE_OK) {
        error_report("Failed to prepare statement when trying to fetch an alarm hash configuration");
        return 1;
    }

    uuid_t hash_uuid;
    if (uuid_parse(config_hash, hash_uuid))
        return 1;

    rc = sqlite3_bind_blob(res, 1, &hash_uuid , sizeof(hash_uuid), SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    struct aclk_alarm_configuration alarm_config;
    struct provide_alarm_configuration p_alarm_config;
    p_alarm_config.cfg_hash = NULL;

    if (sqlite3_step_monitored(res) == SQLITE_ROW) {

        alarm_config.alarm = sqlite3_column_bytes(res, 0) > 0 ? strdupz((char *)sqlite3_column_text(res, 0)) : NULL;
        alarm_config.tmpl = sqlite3_column_bytes(res, 1) > 0 ? strdupz((char *)sqlite3_column_text(res, 1)) : NULL;
        alarm_config.on_chart = sqlite3_column_bytes(res, 2) > 0 ? strdupz((char *)sqlite3_column_text(res, 2)) : NULL;
        alarm_config.classification = sqlite3_column_bytes(res, 3) > 0 ? strdupz((char *)sqlite3_column_text(res, 3)) : NULL;
        alarm_config.type = sqlite3_column_bytes(res, 4) > 0 ? strdupz((char *)sqlite3_column_text(res, 4)) : NULL;
        alarm_config.component = sqlite3_column_bytes(res, 5) > 0 ? strdupz((char *)sqlite3_column_text(res, 5)) : NULL;

        alarm_config.os = sqlite3_column_bytes(res, 6) > 0 ? strdupz((char *)sqlite3_column_text(res, 6)) : NULL;
        alarm_config.hosts = sqlite3_column_bytes(res, 7) > 0 ? strdupz((char *)sqlite3_column_text(res, 7)) : NULL;
        alarm_config.plugin = sqlite3_column_bytes(res, 8) > 0 ? strdupz((char *)sqlite3_column_text(res, 8)) : NULL;
        alarm_config.module = sqlite3_column_bytes(res, 9) > 0 ? strdupz((char *)sqlite3_column_text(res, 9)) : NULL;
        alarm_config.charts = sqlite3_column_bytes(res, 10) > 0 ? strdupz((char *)sqlite3_column_text(res, 10)) : NULL;
        alarm_config.families = sqlite3_column_bytes(res, 11) > 0 ? strdupz((char *)sqlite3_column_text(res, 11)) : NULL;
        alarm_config.lookup = sqlite3_column_bytes(res, 12) > 0 ? strdupz((char *)sqlite3_column_text(res, 12)) : NULL;
        alarm_config.every = sqlite3_column_bytes(res, 13) > 0 ? strdupz((char *)sqlite3_column_text(res, 13)) : NULL;
        alarm_config.units = sqlite3_column_bytes(res, 14) > 0 ? strdupz((char *)sqlite3_column_text(res, 14)) : NULL;

        alarm_config.green = sqlite3_column_bytes(res, 15) > 0 ? strdupz((char *)sqlite3_column_text(res, 15)) : NULL;
        alarm_config.red = sqlite3_column_bytes(res, 16) > 0 ? strdupz((char *)sqlite3_column_text(res, 16)) : NULL;

        alarm_config.calculation_expr = sqlite3_column_bytes(res, 17) > 0 ? strdupz((char *)sqlite3_column_text(res, 17)) : NULL;
        alarm_config.warning_expr = sqlite3_column_bytes(res, 18) > 0 ? strdupz((char *)sqlite3_column_text(res, 18)) : NULL;
        alarm_config.critical_expr = sqlite3_column_bytes(res, 19) > 0 ? strdupz((char *)sqlite3_column_text(res, 19)) : NULL;

        alarm_config.recipient = sqlite3_column_bytes(res, 20) > 0 ? strdupz((char *)sqlite3_column_text(res, 20)) : NULL;
        alarm_config.exec = sqlite3_column_bytes(res, 21) > 0 ? strdupz((char *)sqlite3_column_text(res, 21)) : NULL;
        alarm_config.delay = sqlite3_column_bytes(res, 22) > 0 ? strdupz((char *)sqlite3_column_text(res, 22)) : NULL;
        alarm_config.repeat = sqlite3_column_bytes(res, 23) > 0 ? strdupz((char *)sqlite3_column_text(res, 23)) : NULL;
        alarm_config.info = sqlite3_column_bytes(res, 24) > 0 ? strdupz((char *)sqlite3_column_text(res, 24)) : NULL;
        alarm_config.options = sqlite3_column_bytes(res, 25) > 0 ? strdupz((char *)sqlite3_column_text(res, 25)) : NULL;
        alarm_config.host_labels = sqlite3_column_bytes(res, 26) > 0 ? strdupz((char *)sqlite3_column_text(res, 26)) : NULL;

        alarm_config.p_db_lookup_dimensions = NULL;
        alarm_config.p_db_lookup_method = NULL;
        alarm_config.p_db_lookup_options = NULL;
        alarm_config.p_db_lookup_after = 0;
        alarm_config.p_db_lookup_before = 0;

        if (sqlite3_column_bytes(res, 30) > 0) {

            alarm_config.p_db_lookup_dimensions = sqlite3_column_bytes(res, 27) > 0 ? strdupz((char *)sqlite3_column_text(res, 27)) : NULL;
            alarm_config.p_db_lookup_method = sqlite3_column_bytes(res, 28) > 0 ? strdupz((char *)sqlite3_column_text(res, 28)) : NULL;

            BUFFER *tmp_buf = buffer_create(1024, &netdata_buffers_statistics.buffers_sqlite);
            buffer_data_options2string(tmp_buf, sqlite3_column_int(res, 29));
            alarm_config.p_db_lookup_options = strdupz((char *)buffer_tostring(tmp_buf));
            buffer_free(tmp_buf);

            alarm_config.p_db_lookup_after = sqlite3_column_int(res, 30);
            alarm_config.p_db_lookup_before = sqlite3_column_int(res, 31);
        }

        alarm_config.p_update_every = sqlite3_column_int(res, 32);

        p_alarm_config.cfg_hash = strdupz((char *) config_hash);
        p_alarm_config.cfg = alarm_config;
    }

    if (likely(p_alarm_config.cfg_hash)) {
        log_access("ACLK RES [%s (%s)]: Sent alert config %s.", wc->node_id, wc->host ? rrdhost_hostname(wc->host) : "N/A", config_hash);
        aclk_send_provide_alarm_cfg(&p_alarm_config);
        freez(p_alarm_config.cfg_hash);
        destroy_aclk_alarm_configuration(&alarm_config);
    }
    else
        log_access("ACLK STA [%s (%s)]: Alert config for %s not found.", wc->node_id, wc->host ? rrdhost_hostname(wc->host) : "N/A", config_hash);

bind_fail:
    rc = sqlite3_finalize(res);
    if (unlikely(rc != SQLITE_OK))
        error_report("Failed to reset statement when pushing alarm config hash, rc = %d", rc);

    freez(config_hash);
    freez(node_id);
#endif
    return rc;
}


// Start streaming alerts
void aclk_start_alert_streaming(char *node_id, bool resets)
{
    if (unlikely(!node_id))
        return;

    uuid_t node_uuid;
    if (uuid_parse(node_id, node_uuid))
        return;

    RRDHOST *host = find_host_by_node_id(node_id);

    if (unlikely(!host))
        return;

    struct aclk_sync_host_config *wc  = host->aclk_sync_host_config;

    if (unlikely(!wc))
        return;

    if (unlikely(!host->health.health_enabled)) {
        log_access("ACLK STA [%s (N/A)]: Ignoring request to stream alert state changes, health is disabled.", node_id);
        return;
    }

    if (resets) {
        log_access("ACLK REQ [%s (%s)]: STREAM ALERTS ENABLED (RESET REQUESTED)", node_id, wc->host ? rrdhost_hostname(wc->host) : "N/A");
        sql_queue_existing_alerts_to_aclk(host);
    } else
        log_access("ACLK REQ [%s (%s)]: STREAM ALERTS ENABLED", node_id, wc->host ? rrdhost_hostname(wc->host) : "N/A");

    wc->alert_updates = 1;
    wc->alert_queue_removed = SEND_REMOVED_AFTER_HEALTH_LOOPS;
}

#define SQL_QUEUE_REMOVE_ALERTS "INSERT INTO aclk_alert_%s (alert_unique_id, date_created, filtered_alert_unique_id) " \
                                "SELECT unique_id alert_unique_id, UNIXEPOCH(), unique_id alert_unique_id FROM health_log_%s " \
                                "WHERE new_status = -2 AND updated_by_id = 0 AND unique_id NOT IN " \
                                "(SELECT alert_unique_id FROM aclk_alert_%s) " \
                                "AND config_hash_id NOT IN (select hash_id from alert_hash where warn is null and crit is null) " \
                                "ORDER BY unique_id ASC " \
                                "ON CONFLICT (alert_unique_id) DO NOTHING;"

void sql_process_queue_removed_alerts_to_aclk(char *node_id)
{
    struct aclk_sync_host_config *wc;
    RRDHOST *host = find_host_by_node_id(node_id);
    freez(node_id);

    if (unlikely(!host || !(wc = host->aclk_sync_host_config)))
        return;

    char sql[ACLK_SYNC_QUERY_SIZE * 2];

    snprintfz(sql,ACLK_SYNC_QUERY_SIZE * 2 - 1, SQL_QUEUE_REMOVE_ALERTS, wc->uuid_str, wc->uuid_str, wc->uuid_str);

    if (unlikely(db_execute(db_meta, sql))) {
        log_access("ACLK STA [%s (%s)]: QUEUED REMOVED ALERTS FAILED", wc->node_id, rrdhost_hostname(wc->host));
        error_report("Failed to queue ACLK alert removed entries for host %s", rrdhost_hostname(wc->host));
    }
    else
        log_access("ACLK STA [%s (%s)]: QUEUED REMOVED ALERTS", wc->node_id, rrdhost_hostname(wc->host));

    rrdhost_flag_set(wc->host, RRDHOST_FLAG_ACLK_STREAM_ALERTS);
    wc->alert_queue_removed = 0;
}

void sql_queue_removed_alerts_to_aclk(RRDHOST *host)
{
    if (unlikely(!host->aclk_sync_host_config))
        return;

    if (!claimed() || !host->node_id)
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

    RRDHOST *host = find_host_by_node_id(node_id);
    if (unlikely(!host)) {
        log_access("ACLK STA [%s (N/A)]: ACLK node id does not exist", node_id);
        return;
    }

    struct aclk_sync_host_config *wc = (struct aclk_sync_host_config *)host->aclk_sync_host_config;

    if (unlikely(!wc)) {
       log_access("ACLK STA [%s (N/A)]: ACLK node id does not exist", node_id);
       return;
    }

    log_access(
            "IN [%s (%s)]: Request to send alerts snapshot, snapshot_uuid %s",
            node_id,
            wc->host ? rrdhost_hostname(wc->host) : "N/A",
            snapshot_uuid);
    if (wc->alerts_snapshot_uuid && !strcmp(wc->alerts_snapshot_uuid,snapshot_uuid))
        return;
    __sync_synchronize();
    wc->alerts_snapshot_uuid = strdupz(snapshot_uuid);
    __sync_synchronize();

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

    alarm_log->chart = strdupz(ae_chart_name(ae));
    alarm_log->name = strdupz(ae_name(ae));
    alarm_log->family = strdupz(ae_family(ae));

    alarm_log->batch_id = 0;
    alarm_log->sequence_id = 0;
    alarm_log->when = (time_t)ae->when;

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
    alarm_log->delay = (int)ae->delay;
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

    alarm_log->transition_id = strdupz((char *)transition_id);
    alarm_log->event_id = (uint64_t) ae->alarm_event_id;

    freez(edit_command);
}
#endif

#ifdef ENABLE_ACLK
static int have_recent_alarm(RRDHOST *host, uint32_t alarm_id, uint32_t mark)
{
    ALARM_ENTRY *ae = host->health_log.alarms;

    while (ae) {
        if (ae->alarm_id == alarm_id && ae->unique_id >mark &&
            (ae->new_status != RRDCALC_STATUS_WARNING && ae->new_status != RRDCALC_STATUS_CRITICAL))
            return 1;
        ae = ae->next;
    }

    return 0;
}
#endif

#define ALARM_EVENTS_PER_CHUNK 10
void aclk_push_alert_snapshot_event(char *node_id __maybe_unused)
{
#ifdef ENABLE_ACLK
    RRDHOST *host = find_host_by_node_id(node_id);

    if (unlikely(!host)) {
        log_access("AC [%s (N/A)]: Node id not found", node_id);
        freez(node_id);
        return;
    }
    freez(node_id);

    struct aclk_sync_host_config *wc = host->aclk_sync_host_config;

    // we perhaps we don't need this for snapshots
    if (unlikely(!wc->alert_updates)) {
        log_access(
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

    log_access("ACLK REQ [%s (%s)]: Sending alerts snapshot, snapshot_uuid %s", wc->node_id, rrdhost_hostname(wc->host), wc->alerts_snapshot_uuid);

    uint32_t cnt = 0;
    char uuid_str[UUID_STR_LEN];
    uuid_unparse_lower_fix(&host->host_uuid, uuid_str);

    netdata_rwlock_rdlock(&host->health_log.alarm_log_rwlock);

    ALARM_ENTRY *ae = host->health_log.alarms;

    for (; ae; ae = ae->next) {
        if (likely(ae->updated_by_id))
            continue;

        if (unlikely(ae->new_status == RRDCALC_STATUS_UNINITIALIZED))
            continue;

        if (have_recent_alarm(host, ae->alarm_id, ae->unique_id))
            continue;

        if (is_event_from_alert_variable_config(ae->unique_id, uuid_str))
            continue;

        cnt++;
    }

    if (cnt) {
        uint32_t chunk = 1, chunks = 0;

        chunks = (cnt / ALARM_EVENTS_PER_CHUNK) + (cnt % ALARM_EVENTS_PER_CHUNK != 0);
        ae = host->health_log.alarms;

        cnt = 0;
        struct alarm_snapshot alarm_snap;
        alarm_snap.node_id = wc->node_id;
        alarm_snap.claim_id = claim_id;
        alarm_snap.snapshot_uuid = wc->alerts_snapshot_uuid;
        alarm_snap.chunks = chunks;
        alarm_snap.chunk = chunk;

        alarm_snapshot_proto_ptr_t snapshot_proto = NULL;

        for (; ae; ae = ae->next) {
            if (likely(ae->updated_by_id))
                continue;

            if (unlikely(ae->new_status == RRDCALC_STATUS_UNINITIALIZED))
                continue;

            if (have_recent_alarm(host, ae->alarm_id, ae->unique_id))
                continue;

            if (is_event_from_alert_variable_config(ae->unique_id, uuid_str))
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

                if (chunk < chunks) {
                    chunk++;

                    struct alarm_snapshot alarm_snap;
                    alarm_snap.node_id = wc->node_id;
                    alarm_snap.claim_id = claim_id;
                    alarm_snap.snapshot_uuid = wc->alerts_snapshot_uuid;
                    alarm_snap.chunks = chunks;
                    alarm_snap.chunk = chunk;

                    snapshot_proto = generate_alarm_snapshot_proto(&alarm_snap);
                }
            }
            destroy_alarm_log_entry(&alarm_log);
        }
        if (cnt)
            aclk_send_alarm_snapshot(snapshot_proto);
    }

    netdata_rwlock_unlock(&host->health_log.alarm_log_rwlock);
    wc->alerts_snapshot_uuid = NULL;

    freez(claim_id);
#endif
}

#define SQL_DELETE_ALERT_ENTRIES "DELETE FROM aclk_alert_%s WHERE filtered_alert_unique_id + %d < UNIXEPOCH();"
void sql_aclk_alert_clean_dead_entries(RRDHOST *host)
{
    char uuid_str[UUID_STR_LEN];
    uuid_unparse_lower_fix(&host->host_uuid, uuid_str);

    char sql[ACLK_SYNC_QUERY_SIZE];
    snprintfz(sql, ACLK_SYNC_QUERY_SIZE - 1, SQL_DELETE_ALERT_ENTRIES, uuid_str, MAX_REMOVED_PERIOD);

    char *err_msg = NULL;
    int rc = sqlite3_exec_monitored(db_meta, sql, NULL, NULL, &err_msg);
    if (rc != SQLITE_OK) {
        error_report("Failed when trying to clean stale ACLK alert entries from aclk_alert_%s, error message \"%s\"", uuid_str, err_msg);
        sqlite3_free(err_msg);
    }
}

#define SQL_GET_MIN_MAX_ALERT_SEQ "SELECT MIN(sequence_id), MAX(sequence_id), " \
                                  "(SELECT MAX(sequence_id) FROM aclk_alert_%s WHERE date_submitted IS NOT NULL) " \
                                  "FROM aclk_alert_%s WHERE date_submitted IS NULL;"

int get_proto_alert_status(RRDHOST *host, struct proto_alert_status *proto_alert_status)
{
    int rc;
    struct aclk_sync_host_config *wc  = NULL;
    wc = (struct aclk_sync_host_config *)host->aclk_sync_host_config;
    if (!wc)
        return 1;

    proto_alert_status->alert_updates = wc->alert_updates;

    char sql[ACLK_SYNC_QUERY_SIZE];
    sqlite3_stmt *res = NULL;

    snprintfz(sql, ACLK_SYNC_QUERY_SIZE - 1, SQL_GET_MIN_MAX_ALERT_SEQ, wc->uuid_str, wc->uuid_str);

    rc = sqlite3_prepare_v2(db_meta, sql, -1, &res, 0);
    if (rc != SQLITE_OK) {
        error_report("Failed to prepare statement to get alert log status from the database.");
        return 1;
    }

    while (sqlite3_step_monitored(res) == SQLITE_ROW) {
        proto_alert_status->pending_min_sequence_id = sqlite3_column_bytes(res, 0) > 0 ? (uint64_t) sqlite3_column_int64(res, 0) : 0;
        proto_alert_status->pending_max_sequence_id = sqlite3_column_bytes(res, 1) > 0 ? (uint64_t) sqlite3_column_int64(res, 1) : 0;
        proto_alert_status->last_submitted_sequence_id = sqlite3_column_bytes(res, 2) > 0 ? (uint64_t) sqlite3_column_int64(res, 2) : 0;
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

    struct aclk_sync_host_config *wc = NULL;
    RRDHOST *host = find_host_by_node_id(node_id);

    if (unlikely(!host))
        return;

    wc = (struct aclk_sync_host_config *)host->aclk_sync_host_config;
    if (unlikely(!wc)) {
        log_access("ACLK REQ [%s (N/A)]: ALERTS CHECKPOINT REQUEST RECEIVED FOR INVALID NODE", node_id);
        return;
    }

    log_access("ACLK REQ [%s (%s)]: ALERTS CHECKPOINT REQUEST RECEIVED", node_id, rrdhost_hostname(host));

    wc->alert_checkpoint_req = SEND_CHECKPOINT_AFTER_HEALTH_LOOPS;
}

typedef struct active_alerts {
    char *name;
    char *chart;
    RRDCALC_STATUS status;
} active_alerts_t;

static inline int compare_active_alerts(const void * a, const void * b) {
    active_alerts_t *active_alerts_a = (active_alerts_t *)a;
    active_alerts_t *active_alerts_b = (active_alerts_t *)b;

    if( !(strcmp(active_alerts_a->name, active_alerts_b->name)) )
        {
            return strcmp(active_alerts_a->chart, active_alerts_b->chart);
        }
    else
        return strcmp(active_alerts_a->name, active_alerts_b->name);
}

#define BATCH_ALLOCATED 10
void aclk_push_alarm_checkpoint(RRDHOST *host __maybe_unused)
{
#ifdef ENABLE_ACLK
    struct aclk_sync_host_config *wc = host->aclk_sync_host_config;
    if (unlikely(!wc)) {
        log_access("ACLK REQ [%s (N/A)]: ALERTS CHECKPOINT REQUEST RECEIVED FOR INVALID NODE", rrdhost_hostname(host));
        return;
    }

    if (rrdhost_flag_check(host, RRDHOST_FLAG_ACLK_STREAM_ALERTS)) {
        //postpone checkpoint send
        wc->alert_checkpoint_req+=3;
        log_access("ACLK REQ [%s (N/A)]: ALERTS CHECKPOINT POSTPONED", rrdhost_hostname(host));
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
            len += string_strlen(rc->name);
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
        qsort (active_alerts, cnt, sizeof(active_alerts_t), compare_active_alerts);

        alarms_to_hash = buffer_create(len, NULL);
        for (uint32_t i=0;i<cnt;i++) {
            buffer_strcat(alarms_to_hash, active_alerts[i].name);
            buffer_strcat(alarms_to_hash, active_alerts[i].chart);
            if (active_alerts[i].status == RRDCALC_STATUS_WARNING)
                buffer_strcat(alarms_to_hash, "W");
            else if (active_alerts[i].status == RRDCALC_STATUS_CRITICAL)
                buffer_strcat(alarms_to_hash, "C");
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
        log_access("ACLK RES [%s (%s)]: ALERTS CHECKPOINT SENT", wc->node_id, rrdhost_hostname(host));
    } else {
        log_access("ACLK RES [%s (%s)]: FAILED TO CREATE ALERTS CHECKPOINT HASH", wc->node_id, rrdhost_hostname(host));
    }
    wc->alert_checkpoint_req = 0;
    buffer_free(alarms_to_hash);
#endif
}
