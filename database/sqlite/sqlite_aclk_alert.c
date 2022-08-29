// SPDX-License-Identifier: GPL-3.0-or-later

#include "sqlite_functions.h"
#include "sqlite_aclk_alert.h"

#ifdef ENABLE_ACLK
#include "../../aclk/aclk_alarm_api.h"
#include "../../aclk/aclk.h"
#endif

time_t removed_when(uint32_t alarm_id, uint32_t before_unique_id, uint32_t after_unique_id, char *uuid_str) {
    sqlite3_stmt *res = NULL;
    int rc = 0;
    time_t when = 0;
    char sql[ACLK_SYNC_QUERY_SIZE];

    snprintfz(sql,ACLK_SYNC_QUERY_SIZE-1, "select when_key from health_log_%s where alarm_id = %u " \
                                          "and unique_id > %u and unique_id < %u " \
                                          "and new_status = -2;", uuid_str, alarm_id, after_unique_id, before_unique_id);

    rc = sqlite3_prepare_v2(db_meta, sql, -1, &res, 0);
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

#define MAX_REMOVED_PERIOD 900
//decide if some events should be sent or not
int should_send_to_cloud(RRDHOST *host, ALARM_ENTRY *ae)
{
    sqlite3_stmt *res = NULL;
    char uuid_str[GUID_LEN + 1];
    uuid_unparse_lower_fix(&host->host_uuid, uuid_str);
    int send = 1, rc = 0;

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
    snprintfz(sql,ACLK_SYNC_QUERY_SIZE-1, "select hl.new_status, hl.config_hash_id, hl.unique_id from health_log_%s hl, aclk_alert_%s aa \
                         where hl.unique_id = aa.alert_unique_id        \
                         and hl.alarm_id = %u and hl.unique_id <> %u \
                         order by alarm_event_id desc LIMIT 1;", uuid_str, uuid_str, ae->alarm_id, ae->unique_id);

    rc = sqlite3_prepare_v2(db_meta, sql, -1, &res, 0);
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

    if (uuid_compare(ae->config_hash_id, config_hash_id)) {
        send = 1;
        goto done;
    }

    //same status, same config
    if (ae->new_status == RRDCALC_STATUS_CLEAR) {
        send = 0;
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
int sql_queue_alarm_to_aclk(RRDHOST *host, ALARM_ENTRY *ae, int skip_filter)
{
    if (!claimed())
        return 0;

    if (ae->flags & HEALTH_ENTRY_FLAG_ACLK_QUEUED) {
        return 0;
    }

    if (!skip_filter) {
        if (!should_send_to_cloud(host, ae)) {
            return 0;
        }
    }

    int rc = 0;

    CHECK_SQLITE_CONNECTION(db_meta);

    sqlite3_stmt *res_alert = NULL;
    char uuid_str[GUID_LEN + 1];
    uuid_unparse_lower_fix(&host->host_uuid, uuid_str);

    BUFFER *sql = buffer_create(1024);

    buffer_sprintf(
        sql,
        "INSERT INTO aclk_alert_%s (alert_unique_id, date_created) "
        "VALUES (@alert_unique_id, unixepoch()) on conflict (alert_unique_id) do nothing; ",
        uuid_str);

    rc = sqlite3_prepare_v2(db_meta, buffer_tostring(sql), -1, &res_alert, 0);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to prepare statement to store alert event");
        buffer_free(sql);
        return 1;
    }

    rc = sqlite3_bind_int(res_alert, 1, ae->unique_id);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    rc = execute_insert(res_alert);
    if (unlikely(rc != SQLITE_DONE)) {
        error_report("Failed to store alert event %u, rc = %d", ae->unique_id, rc);
        goto bind_fail;
    }

    ae->flags |= HEALTH_ENTRY_FLAG_ACLK_QUEUED;
    struct aclk_database_worker_config *wc  = (struct aclk_database_worker_config *)host->dbsync_worker;
    if (wc) {
        wc->pause_alert_updates = 0;
    }

bind_fail:
    if (unlikely(sqlite3_finalize(res_alert) != SQLITE_OK))
        error_report("Failed to reset statement in store alert event, rc = %d", rc);

    buffer_free(sql);
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

void aclk_push_alert_event(struct aclk_database_worker_config *wc, struct aclk_database_cmd cmd)
{
#ifndef ENABLE_ACLK
    UNUSED(wc);
    UNUSED(cmd);
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

    BUFFER *sql = buffer_create(1024);

    if (wc->alerts_start_seq_id != 0) {
        buffer_sprintf(
            sql,
            "UPDATE aclk_alert_%s SET date_submitted = NULL, date_cloud_ack = NULL WHERE sequence_id >= %"PRIu64
            "; UPDATE aclk_alert_%s SET date_cloud_ack = unixepoch() WHERE sequence_id < %"PRIu64
            " and date_cloud_ack is null "
            "; UPDATE aclk_alert_%s SET date_submitted = unixepoch() WHERE sequence_id < %"PRIu64
            " and date_submitted is null",
            wc->uuid_str,
            wc->alerts_start_seq_id,
            wc->uuid_str,
            wc->alerts_start_seq_id,
            wc->uuid_str,
            wc->alerts_start_seq_id);
        db_execute(buffer_tostring(sql));
        buffer_reset(sql);
        wc->alerts_start_seq_id = 0;
    }

    int limit = cmd.count > 0 ? cmd.count : 1;

    sqlite3_stmt *res = NULL;

    buffer_sprintf(sql, "select aa.sequence_id, hl.unique_id, hl.alarm_id, hl.config_hash_id, hl.updated_by_id, hl.when_key, \
                   hl.duration, hl.non_clear_duration, hl.flags, hl.exec_run_timestamp, hl.delay_up_to_timestamp, hl.name, \
                   hl.chart, hl.family, hl.exec, hl.recipient, hl.source, hl.units, hl.info, hl.exec_code, hl.new_status, \
                   hl.old_status, hl.delay, hl.new_value, hl.old_value, hl.last_repeat, hl.chart_context \
                         from health_log_%s hl, aclk_alert_%s aa \
                         where hl.unique_id = aa.alert_unique_id and aa.date_submitted is null \
                         order by aa.sequence_id asc limit %d;", wc->uuid_str, wc->uuid_str, limit);

    rc = sqlite3_prepare_v2(db_meta, buffer_tostring(sql), -1, &res, 0);
    if (rc != SQLITE_OK) {
        error_report("Failed to prepare statement when trying to send an alert update via ACLK");
        buffer_free(sql);
        freez(claim_id);
        return;
    }

    char uuid_str[GUID_LEN + 1];
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

        alarm_log.batch_id = wc->alerts_batch_id;
        alarm_log.sequence_id = (uint64_t) sqlite3_column_int64(res, 0);
        alarm_log.when = (time_t) sqlite3_column_int64(res, 5);

        uuid_unparse_lower(*((uuid_t *) sqlite3_column_blob(res, 3)), uuid_str);
        alarm_log.config_hash = strdupz((char *)uuid_str);

        alarm_log.utc_offset = wc->host->utc_offset;
        alarm_log.timezone = strdupz(rrdhost_abbrev_timezone(wc->host));
        alarm_log.exec_path = sqlite3_column_bytes(res, 14) > 0 ? strdupz((char *)sqlite3_column_text(res, 14)) :
                                                                  strdupz((char *)string2str(wc->host->health_default_exec));
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
        alarm_log.rendered_info = sqlite3_column_type(res, 18) == SQLITE_NULL ?
                                      strdupz((char *)"") :
                                      strdupz((char *)sqlite3_column_text(res, 18));

        alarm_log.chart_context = sqlite3_column_type(res, 26) == SQLITE_NULL ?
                                      strdupz((char *)"") :
                                      strdupz((char *)sqlite3_column_text(res, 26));

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
        db_execute(buffer_tostring(sql));
    } else {
        if (log_first_sequence_id)
            log_access(
                "ACLK RES [%s (%s)]: ALERTS SENT from %" PRIu64 " to %" PRIu64 " batch=%" PRIu64,
                wc->node_id,
                wc->host ? rrdhost_hostname(wc->host) : "N/A",
                log_first_sequence_id,
                log_last_sequence_id,
                wc->alerts_batch_id);
        log_first_sequence_id = 0;
        log_last_sequence_id = 0;
        wc->pause_alert_updates = 1;
    }

    rc = sqlite3_finalize(res);
    if (unlikely(rc != SQLITE_OK))
        error_report("Failed to finalize statement to send alert entries from the database, rc = %d", rc);

    freez(claim_id);
    buffer_free(sql);
#endif

    return;
}

void sql_queue_existing_alerts_to_aclk(RRDHOST *host)
{
    char uuid_str[GUID_LEN + 1];
    uuid_unparse_lower_fix(&host->host_uuid, uuid_str);
    BUFFER *sql = buffer_create(1024);

    buffer_sprintf(sql,"delete from aclk_alert_%s; " \
                       "insert into aclk_alert_%s (alert_unique_id, date_created) " \
                       "select unique_id alert_unique_id, unixepoch() from health_log_%s " \
                       "where new_status <> 0 and new_status <> -2 and config_hash_id is not null and updated_by_id = 0 " \
                       "order by unique_id asc on conflict (alert_unique_id) do nothing;", uuid_str, uuid_str, uuid_str);

    db_execute(buffer_tostring(sql));

    buffer_free(sql);

    struct aclk_database_worker_config *wc  = (struct aclk_database_worker_config *)host->dbsync_worker;
    if (wc) {
        wc->pause_alert_updates = 0;
    }
}

void aclk_send_alarm_health_log(char *node_id)
{
    if (unlikely(!node_id))
        return;

    struct aclk_database_worker_config *wc = find_inactive_wc_by_node_id(node_id);

    if (likely(!wc)) {
        rrd_rdlock();
        RRDHOST *host = find_host_by_node_id(node_id);
        rrd_unlock();
        if (likely(host))
            wc = (struct aclk_database_worker_config *)host->dbsync_worker;
    }

    if (!wc) {
        log_access("ACLK REQ [%s (N/A)]: HEALTH LOG REQUEST RECEIVED FOR INVALID NODE", node_id);
        return;
    }

    log_access("ACLK REQ [%s (%s)]: HEALTH LOG REQUEST RECEIVED", node_id, wc->hostname ? wc->hostname : "N/A");

    struct aclk_database_cmd cmd;
    memset(&cmd, 0, sizeof(cmd));
    cmd.opcode = ACLK_DATABASE_ALARM_HEALTH_LOG;

    aclk_database_enq_cmd(wc, &cmd);
    return;
}

void aclk_push_alarm_health_log(struct aclk_database_worker_config *wc, struct aclk_database_cmd cmd)
{
    UNUSED(cmd);
#ifndef ENABLE_ACLK
    UNUSED(wc);
#else
    int rc;

    char *claim_id = get_agent_claimid();
    if (unlikely(!claim_id))
        return;

    RRDHOST *host = wc->host;
    if (unlikely(!host)) {
        rrd_rdlock();
        host = find_host_by_node_id(wc->node_id);
        rrd_unlock();

        if (unlikely(!host)) {
            log_access(
                "AC [%s (N/A)]: ACLK synchronization thread for %s is not yet linked to HOST.",
                wc->node_id,
                wc->host_guid);
            freez(claim_id);
            return;
        }
    }

    uint64_t first_sequence = 0;
    uint64_t last_sequence = 0;
    struct timeval first_timestamp;
    struct timeval last_timestamp;

    BUFFER *sql = buffer_create(1024);

    sqlite3_stmt *res = NULL;

    //TODO: make this better: include info from health log too
    buffer_sprintf(sql, "SELECT MIN(sequence_id), MIN(date_created), MAX(sequence_id), MAX(date_created) " \
         "FROM aclk_alert_%s;", wc->uuid_str);

    rc = sqlite3_prepare_v2(db_meta, buffer_tostring(sql), -1, &res, 0);
    if (rc != SQLITE_OK) {
        error_report("Failed to prepare statement to get health log statistics from the database");
        buffer_free(sql);
        freez(claim_id);
        return;
    }

    first_timestamp.tv_sec = 0;
    first_timestamp.tv_usec = 0;
    last_timestamp.tv_sec = 0;
    last_timestamp.tv_usec = 0;

    while (sqlite3_step_monitored(res) == SQLITE_ROW) {
        first_sequence = sqlite3_column_bytes(res, 0) > 0 ? (uint64_t) sqlite3_column_int64(res, 0) : 0;
        if (sqlite3_column_bytes(res, 1) > 0) {
            first_timestamp.tv_sec = sqlite3_column_int64(res, 1);
        }

        last_sequence = sqlite3_column_bytes(res, 2) > 0 ? (uint64_t) sqlite3_column_int64(res, 2) : 0;
        if (sqlite3_column_bytes(res, 3) > 0) {
            last_timestamp.tv_sec = sqlite3_column_int64(res, 3);
        }
    }

    struct alarm_log_entries log_entries;
    log_entries.first_seq_id = first_sequence;
    log_entries.first_when = first_timestamp;
    log_entries.last_seq_id = last_sequence;
    log_entries.last_when = last_timestamp;

    struct alarm_log_health alarm_log;
    alarm_log.claim_id = claim_id;
    alarm_log.node_id =  wc->node_id;
    alarm_log.log_entries = log_entries;
    alarm_log.status = wc->alert_updates == 0 ? 2 : 1;
    alarm_log.enabled = (int)host->health_enabled;

    wc->alert_sequence_id = last_sequence;

    aclk_send_alarm_log_health(&alarm_log);
    log_access("ACLK RES [%s (%s)]: HEALTH LOG SENT from %"PRIu64" to %"PRIu64, wc->node_id, wc->hostname ? wc->hostname : "N/A", first_sequence, last_sequence);

    rc = sqlite3_finalize(res);
    if (unlikely(rc != SQLITE_OK))
        error_report("Failed to reset statement to get health log statistics from the database, rc = %d", rc);

    freez(claim_id);
    buffer_free(sql);

    aclk_alert_reloaded = 1;
#endif

    return;
}

void aclk_send_alarm_configuration(char *config_hash)
{
    if (unlikely(!config_hash))
        return;

    struct aclk_database_worker_config *wc = (struct aclk_database_worker_config *) localhost->dbsync_worker;

    if (unlikely(!wc)) {
        return;
    }

    log_access("ACLK REQ [%s (%s)]: Request to send alert config %s.", wc->node_id, wc->host ? rrdhost_hostname(wc->host) : "N/A", config_hash);

    struct aclk_database_cmd cmd;
    memset(&cmd, 0, sizeof(cmd));
    cmd.opcode = ACLK_DATABASE_PUSH_ALERT_CONFIG;
    cmd.data_param = (void *) strdupz(config_hash);
    cmd.completion = NULL;
    aclk_database_enq_cmd(wc, &cmd);

    return;
}

#define SQL_SELECT_ALERT_CONFIG "SELECT alarm, template, on_key, class, type, component, os, hosts, plugin," \
    "module, charts, families, lookup, every, units, green, red, calc, warn, crit, to_key, exec, delay, repeat, info," \
    "options, host_labels, p_db_lookup_dimensions, p_db_lookup_method, p_db_lookup_options, p_db_lookup_after," \
    "p_db_lookup_before, p_update_every FROM alert_hash WHERE hash_id = @hash_id;"

int aclk_push_alert_config_event(struct aclk_database_worker_config *wc, struct aclk_database_cmd cmd)
{
    UNUSED(wc);
#ifndef ENABLE_ACLK
    UNUSED(cmd);
#else
    int rc = 0;

    CHECK_SQLITE_CONNECTION(db_meta);

    sqlite3_stmt *res = NULL;

    char *config_hash = (char *) cmd.data_param;

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

            BUFFER *tmp_buf = buffer_create(1024);
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
        freez((char *) cmd.data_param);
        freez(p_alarm_config.cfg_hash);
        destroy_aclk_alarm_configuration(&alarm_config);
    }
    else
        log_access("ACLK STA [%s (%s)]: Alert config for %s not found.", wc->node_id, wc->host ? rrdhost_hostname(wc->host) : "N/A", config_hash);

bind_fail:
    rc = sqlite3_finalize(res);
    if (unlikely(rc != SQLITE_OK))
        error_report("Failed to reset statement when pushing alarm config hash, rc = %d", rc);

    return rc;
#endif
    return 0;
}


// Start streaming alerts
void aclk_start_alert_streaming(char *node_id, uint64_t batch_id, uint64_t start_seq_id)
{
    if (unlikely(!node_id))
        return;

    //log_access("ACLK REQ [%s (N/A)]: ALERTS STREAM from %"PRIu64" batch=%"PRIu64".", node_id, start_seq_id, batch_id);

    uuid_t node_uuid;
    if (uuid_parse(node_id, node_uuid))
        return;

    struct aclk_database_worker_config *wc  = NULL;
    rrd_rdlock();
    RRDHOST *host = find_host_by_node_id(node_id);
    rrd_unlock();
    if (likely(host)) {
        wc = (struct aclk_database_worker_config *)host->dbsync_worker ?
                 (struct aclk_database_worker_config *)host->dbsync_worker :
                 (struct aclk_database_worker_config *)find_inactive_wc_by_node_id(node_id);

        if (unlikely(!host->health_enabled)) {
            log_access("ACLK STA [%s (N/A)]: Ignoring request to stream alert state changes, health is disabled.", node_id);
            return;
        }

        if (unlikely(batch_id == 1) && unlikely(start_seq_id == 1))
            sql_queue_existing_alerts_to_aclk(host);
    } else
        wc = (struct aclk_database_worker_config *)find_inactive_wc_by_node_id(node_id);

    if (likely(wc)) {
        log_access("ACLK REQ [%s (%s)]: ALERTS STREAM from %"PRIu64" batch=%"PRIu64, node_id, wc->host ? rrdhost_hostname(wc->host) : "N/A", start_seq_id, batch_id);
        __sync_synchronize();
        wc->alerts_batch_id = batch_id;
        wc->alerts_start_seq_id = start_seq_id;
        wc->alert_updates = 1;
        wc->pause_alert_updates = 0;
        __sync_synchronize();
    }
    else
        log_access("ACLK STA [%s (N/A)]: ACLK synchronization thread is not active.", node_id);

    return;
}

void sql_process_queue_removed_alerts_to_aclk(struct aclk_database_worker_config *wc, struct aclk_database_cmd cmd)
{
    UNUSED(cmd);

    BUFFER *sql = buffer_create(1024);

    buffer_sprintf(sql,"insert into aclk_alert_%s (alert_unique_id, date_created) " \
        "select unique_id alert_unique_id, unixepoch() from health_log_%s " \
        "where new_status = -2 and updated_by_id = 0 and unique_id not in " \
        "(select alert_unique_id from aclk_alert_%s) order by unique_id asc " \
        "on conflict (alert_unique_id) do nothing;", wc->uuid_str, wc->uuid_str, wc->uuid_str);

    db_execute(buffer_tostring(sql));

    log_access("ACLK STA [%s (%s)]: QUEUED REMOVED ALERTS", wc->node_id, wc->host ? rrdhost_hostname(wc->host) : "N/A");

    buffer_free(sql);

    wc->pause_alert_updates = 0;
    return;
}

void sql_queue_removed_alerts_to_aclk(RRDHOST *host)
{
    if (unlikely(!host->dbsync_worker))
        return;

    if (!claimed())
        return;

    struct aclk_database_cmd cmd;
    memset(&cmd, 0, sizeof(cmd));
    cmd.opcode = ACLK_DATABASE_QUEUE_REMOVED_ALERTS;
    cmd.data = NULL;
    cmd.data_param = NULL;
    cmd.completion = NULL;
    aclk_database_enq_cmd((struct aclk_database_worker_config *) host->dbsync_worker, &cmd);
}

void aclk_process_send_alarm_snapshot(char *node_id, char *claim_id, uint64_t snapshot_id, uint64_t sequence_id)
{
    UNUSED(claim_id);
    if (unlikely(!node_id))
        return;

    uuid_t node_uuid;
    if (uuid_parse(node_id, node_uuid))
        return;

    struct aclk_database_worker_config *wc = NULL;
    rrd_rdlock();
    RRDHOST *host = find_host_by_node_id(node_id);
    if (likely(host))
        wc = (struct aclk_database_worker_config *)host->dbsync_worker;
    rrd_unlock();

    if (likely(wc)) {
        log_access(
            "IN [%s (%s)]: Request to send alerts snapshot, snapshot_id %" PRIu64 " and ack_sequence_id %" PRIu64,
            wc->node_id,
            wc->host ? rrdhost_hostname(wc->host) : "N/A",
            snapshot_id,
            sequence_id);
        if (wc->alerts_snapshot_id == snapshot_id)
            return;
        __sync_synchronize();
        wc->alerts_snapshot_id = snapshot_id;
        wc->alerts_ack_sequence_id = sequence_id;
        __sync_synchronize();

        struct aclk_database_cmd cmd;
        memset(&cmd, 0, sizeof(cmd));
        cmd.opcode = ACLK_DATABASE_PUSH_ALERT_SNAPSHOT;
        cmd.data_param = NULL;
        cmd.completion = NULL;
        aclk_database_enq_cmd(wc, &cmd);
    } else
        log_access("ACLK STA [%s (N/A)]: ACLK synchronization thread is not active.", node_id);

    return;
}

void aclk_mark_alert_cloud_ack(char *uuid_str, uint64_t alerts_ack_sequence_id)
{
    BUFFER *sql = buffer_create(1024);

    if (alerts_ack_sequence_id != 0) {
        buffer_sprintf(
            sql,
            "UPDATE aclk_alert_%s SET date_cloud_ack = unixepoch() WHERE sequence_id <= %" PRIu64 "",
            uuid_str,
            alerts_ack_sequence_id);
        db_execute(buffer_tostring(sql));
    }

    buffer_free(sql);
}

#ifdef ENABLE_ACLK
void health_alarm_entry2proto_nolock(struct alarm_log_entry *alarm_log, ALARM_ENTRY *ae, RRDHOST *host)
{
    char *edit_command = ae->source ? health_edit_command_from_source(ae_source(ae)) : strdupz("UNKNOWN=0=UNKNOWN");
    char config_hash_id[GUID_LEN + 1];
    uuid_unparse_lower(ae->config_hash_id, config_hash_id);

    alarm_log->chart = strdupz(ae_chart_name(ae));
    alarm_log->name = strdupz(ae_name(ae));
    alarm_log->family = strdupz(ae_family(ae));

    alarm_log->batch_id = 0;
    alarm_log->sequence_id = 0;
    alarm_log->when = (time_t)ae->when;

    alarm_log->config_hash = strdupz((char *)config_hash_id);

    alarm_log->utc_offset = host->utc_offset;
    alarm_log->timezone = strdupz(rrdhost_abbrev_timezone(host));
    alarm_log->exec_path = ae->exec ? strdupz(ae_exec(ae)) : strdupz((char *)string2str(host->health_default_exec));
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

    freez(edit_command);
}
#endif

#ifdef ENABLE_ACLK
static int have_recent_alarm(RRDHOST *host, uint32_t alarm_id, time_t mark)
{
    ALARM_ENTRY *ae = host->health_log.alarms;

    while (ae) {
        if (ae->alarm_id == alarm_id && ae->unique_id > mark &&
            (ae->new_status != RRDCALC_STATUS_WARNING && ae->new_status != RRDCALC_STATUS_CRITICAL))
            return 1;
        ae = ae->next;
    }

    return 0;
}
#endif

#define ALARM_EVENTS_PER_CHUNK 10
void aclk_push_alert_snapshot_event(struct aclk_database_worker_config *wc, struct aclk_database_cmd cmd)
{
#ifndef ENABLE_ACLK
    UNUSED(wc);
    UNUSED(cmd);
#else
    UNUSED(cmd);
    // we perhaps we don't need this for snapshots
    if (unlikely(!wc->alert_updates)) {
        log_access("ACLK STA [%s (%s)]: Ignoring alert snapshot event, updates have been turned off for this node.", wc->node_id, wc->host ? rrdhost_hostname(wc->host) : "N/A");
        return;
    }

    if (unlikely(!wc->host)) {
        error_report("ACLK synchronization thread for %s is not linked to HOST", wc->host_guid);
        return;
    }

    if (unlikely(!wc->alerts_snapshot_id))
        return;

    char *claim_id = get_agent_claimid();
    if (unlikely(!claim_id))
        return;

    log_access("ACLK REQ [%s (%s)]: Sending alerts snapshot, snapshot_id %" PRIu64, wc->node_id, wc->host ? rrdhost_hostname(wc->host) : "N/A", wc->alerts_snapshot_id);

    aclk_mark_alert_cloud_ack(wc->uuid_str, wc->alerts_ack_sequence_id);

    RRDHOST *host = wc->host;
    uint32_t cnt = 0;

    netdata_rwlock_rdlock(&host->health_log.alarm_log_rwlock);

    ALARM_ENTRY *ae = host->health_log.alarms;

    for (; ae; ae = ae->next) {
        if (likely(ae->updated_by_id))
            continue;

        if (unlikely(ae->new_status == RRDCALC_STATUS_UNINITIALIZED))
            continue;

        if (have_recent_alarm(host, ae->alarm_id, ae->unique_id))
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
        alarm_snap.snapshot_id = wc->alerts_snapshot_id;
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
                    alarm_snap.snapshot_id = wc->alerts_snapshot_id;
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
    wc->alerts_snapshot_id = 0;

    freez(claim_id);
#endif
    return;
}

void sql_aclk_alert_clean_dead_entries(RRDHOST *host)
{
    if (!claimed())
        return;

    char uuid_str[GUID_LEN + 1];
    uuid_unparse_lower_fix(&host->host_uuid, uuid_str);

    BUFFER *sql = buffer_create(1024);

    buffer_sprintf(sql,"delete from aclk_alert_%s where alert_unique_id not in "
                   " (select unique_id from health_log_%s); ", uuid_str, uuid_str);
    
    char *err_msg = NULL;
    int rc = sqlite3_exec_monitored(db_meta, buffer_tostring(sql), NULL, NULL, &err_msg);
    if (rc != SQLITE_OK) {
        error_report("Failed when trying to clean stale ACLK alert entries from aclk_alert_%s, error message \"%s""",
                     uuid_str, err_msg);
        sqlite3_free(err_msg);
    }
    buffer_free(sql);
}

int get_proto_alert_status(RRDHOST *host, struct proto_alert_status *proto_alert_status)
{
    int rc;
    struct aclk_database_worker_config *wc  = NULL;
    wc = (struct aclk_database_worker_config *)host->dbsync_worker;
    if (!wc)
        return 1;

    proto_alert_status->alert_updates = wc->alert_updates;
    proto_alert_status->alerts_batch_id = wc->alerts_batch_id;

    BUFFER *sql = buffer_create(1024);
    sqlite3_stmt *res = NULL;

    buffer_sprintf(sql, "SELECT MIN(sequence_id), MAX(sequence_id), " \
                   "(select MAX(sequence_id) from aclk_alert_%s where date_cloud_ack is not NULL), " \
                   "(select MAX(sequence_id) from aclk_alert_%s where date_submitted is not NULL) " \
                   "FROM aclk_alert_%s where date_submitted is null;", wc->uuid_str, wc->uuid_str, wc->uuid_str);

    rc = sqlite3_prepare_v2(db_meta, buffer_tostring(sql), -1, &res, 0);
    if (rc != SQLITE_OK) {
        error_report("Failed to prepare statement to get alert log status from the database.");
        buffer_free(sql);
        return 1;
    }

    while (sqlite3_step_monitored(res) == SQLITE_ROW) {
        proto_alert_status->pending_min_sequence_id = sqlite3_column_bytes(res, 0) > 0 ? (uint64_t) sqlite3_column_int64(res, 0) : 0;
        proto_alert_status->pending_max_sequence_id = sqlite3_column_bytes(res, 1) > 0 ? (uint64_t) sqlite3_column_int64(res, 1) : 0;
        proto_alert_status->last_acked_sequence_id = sqlite3_column_bytes(res, 2) > 0 ? (uint64_t) sqlite3_column_int64(res, 2) : 0;
        proto_alert_status->last_submitted_sequence_id = sqlite3_column_bytes(res, 3) > 0 ? (uint64_t) sqlite3_column_int64(res, 3) : 0;
    }

    rc = sqlite3_finalize(res);
    if (unlikely(rc != SQLITE_OK))
        error_report("Failed to finalize statement to get alert log status from the database, rc = %d", rc);

    buffer_free(sql);
    return 0;
}
