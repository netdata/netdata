// SPDX-License-Identifier: GPL-3.0-or-later

#include "sqlite_functions.h"
#include "sqlite_aclk_alert.h"

#include "../../aclk/aclk_alarm_api.h"

// will replace call to aclk_update_alarm in health/health_log.c
// and handle both cases
void sql_queue_alarm_to_aclk(RRDHOST *host, ALARM_ENTRY *ae)
{
    //check aclk architecture and handle old json alarm update to cloud
    //include also the valid statuses for this case
    /* if (!aclk_architecture)
         aclk_update_alarm(host, ae); */

    if (ae->flags & HEALTH_ENTRY_FLAG_ACLK_QUEUED)
        return;

    if (ae->new_status == RRDCALC_STATUS_REMOVED || ae->new_status == RRDCALC_STATUS_UNINITIALIZED)
        return;

    if (unlikely(!host->dbsync_worker))
        return;

    if (unlikely(uuid_is_null(ae->config_hash_id)))
        return;

    struct aclk_database_cmd cmd;
    memset(&cmd, 0, sizeof(cmd));
    cmd.opcode = ACLK_DATABASE_ADD_ALERT;
    cmd.data = ae;
    cmd.completion = NULL;
    aclk_database_enq_cmd((struct aclk_database_worker_config *) host->dbsync_worker, &cmd);
    ae->flags |= HEALTH_ENTRY_FLAG_ACLK_QUEUED;
    return;
}

// stores an alert entry to aclk_alert_ table
int aclk_add_alert_event(struct aclk_database_worker_config *wc, struct aclk_database_cmd cmd)
{
    int rc = 0;

    CHECK_SQLITE_CONNECTION(db_meta);

    sqlite3_stmt *res_alert = NULL;
    ALARM_ENTRY *ae = cmd.data;

    BUFFER *sql = buffer_create(1024);

    buffer_sprintf(
        sql,
        "INSERT INTO aclk_alert_%s (alert_unique_id, date_created) "
        "VALUES (@alert_unique_id, strftime('%%s')); ",
        wc->uuid_str);

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
    if (unlikely(rc != SQLITE_DONE))
        error_report("Failed to store alert event %u, rc = %d", ae->unique_id, rc);

bind_fail:
    if (unlikely(sqlite3_finalize(res_alert) != SQLITE_OK))
        error_report("Failed to reset statement in store alert event, rc = %d", rc);

    buffer_free(sql);
    return (rc != SQLITE_DONE);
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

void aclk_push_alert_event(struct aclk_database_worker_config *wc, struct aclk_database_cmd cmd)
{
#ifndef ENABLE_NEW_CLOUD_PROTOCOL
    UNUSED(wc);
    UNUSED(cmd);
#else
    int rc;

    if (unlikely(!wc->alert_updates)) {
        debug(D_ACLK_SYNC,"Ignoring alert push event, updates have been turned off for node %s", wc->node_id);
        return;
    }

    char *claim_id = is_agent_claimed();
    if (unlikely(!claim_id))
        return;

    BUFFER *sql = buffer_create(1024);

    if (wc->alerts_start_seq_id != 0) {
        buffer_sprintf(
            sql,
            "UPDATE aclk_alert_%s SET date_submitted = NULL, date_cloud_ack = NULL WHERE sequence_id >= %" PRIu64
            "; UPDATE aclk_alert_%s SET date_cloud_ack = strftime('%%s','now') WHERE sequence_id < %" PRIu64
            " and date_cloud_ack is null",
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
                   hl.old_status, hl.delay, hl.new_value, hl.old_value, hl.last_repeat \
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

    while (sqlite3_step(res) == SQLITE_ROW) {
        struct alarm_log_entry alarm_log;
        char old_value_string[100 + 1];
        char new_value_string[100 + 1];

        alarm_log.node_id = strdupz(wc->node_id);
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
        alarm_log.timezone = strdupz((char *)wc->host->abbrev_timezone);
        alarm_log.exec_path = sqlite3_column_bytes(res, 14) > 0 ? strdupz((char *)sqlite3_column_text(res, 14)) : strdupz((char *)wc->host->health_default_exec);
        alarm_log.conf_source = strdupz((char *)sqlite3_column_text(res, 16));

        char *edit_command = sqlite3_column_bytes(res, 16) > 0 ? health_edit_command_from_source((char *)sqlite3_column_text(res, 16)) : strdupz("UNKNOWN=0");
        alarm_log.command = strdupz(edit_command);

        alarm_log.duration = (time_t) sqlite3_column_int64(res, 6);
        alarm_log.non_clear_duration = (time_t) sqlite3_column_int64(res, 7);
        alarm_log.status = rrdcalc_status_to_proto_enum((RRDCALC_STATUS) sqlite3_column_int(res, 20));
        alarm_log.old_status = rrdcalc_status_to_proto_enum((RRDCALC_STATUS) sqlite3_column_int(res, 21));
        alarm_log.delay = (int) sqlite3_column_int(res, 22);
        alarm_log.delay_up_to_timestamp = (time_t) sqlite3_column_int64(res, 10);
        alarm_log.last_repeat = (time_t) sqlite3_column_int64(res, 25);

        alarm_log.silenced = ( (sqlite3_column_int64(res, 8) & HEALTH_ENTRY_FLAG_SILENCED)  || ( sqlite3_column_type(res, 15) != SQLITE_NULL && !strncmp((char *)sqlite3_column_text(res,15), "silent", 6)) ) ? 1 : 0;

        alarm_log.value_string = sqlite3_column_type(res, 23) == SQLITE_NULL ? strdupz((char *)"-") : strdupz((char *)format_value_and_unit(new_value_string, 100, sqlite3_column_double(res, 23), (char *) sqlite3_column_text(res, 17), -1));
        alarm_log.old_value_string = sqlite3_column_type(res, 24) == SQLITE_NULL ? strdupz((char *)"-") : strdupz((char *)format_value_and_unit(old_value_string, 100, sqlite3_column_double(res, 24), (char *) sqlite3_column_text(res, 17), -1));

        alarm_log.value = (calculated_number) sqlite3_column_double(res, 23);
        alarm_log.old_value = (calculated_number) sqlite3_column_double(res, 24);

        alarm_log.updated = (sqlite3_column_int64(res, 8) & HEALTH_ENTRY_FLAG_UPDATED) ? 1 : 0;
        alarm_log.rendered_info = strdupz((char *)sqlite3_column_text(res, 18));

        info("DEBUG: %s pushing alert seq %" PRIu64 " - %" PRIu64"", wc->uuid_str, (uint64_t) sqlite3_column_int64(res, 0), (uint64_t) sqlite3_column_int64(res, 1));
        aclk_send_alarm_log_entry(&alarm_log);

        if (first_sequence_id == 0)
            first_sequence_id  = (uint64_t) sqlite3_column_int64(res, 0);
        last_sequence_id = (uint64_t) sqlite3_column_int64(res, 0);

        destroy_alarm_log_entry(&alarm_log);
        freez(edit_command);
    }
    buffer_flush(sql);

    buffer_sprintf(sql, "UPDATE aclk_alert_%s SET date_submitted=strftime('%%s') "
                        "WHERE date_submitted IS NULL AND sequence_id BETWEEN %" PRIu64 " AND %" PRIu64 ";",
                   wc->uuid_str, first_sequence_id, last_sequence_id);
    db_execute(buffer_tostring(sql));

    rc = sqlite3_finalize(res);
    if (unlikely(rc != SQLITE_OK))
        error_report("Failed to finalize statement to send alert entries from the database, rc = %d", rc);

    freez(claim_id);
    buffer_free(sql);
#endif

    return;
}

void aclk_send_alarm_health_log(char *node_id)
{
    if (unlikely(!node_id))
        return;

    struct aclk_database_worker_config *wc  = NULL;
    struct aclk_database_cmd cmd;
    memset(&cmd, 0, sizeof(cmd));
    cmd.opcode = ACLK_DATABASE_ALARM_HEALTH_LOG;

    rrd_wrlock();
    RRDHOST *host = find_host_by_node_id(node_id);
    if (likely(host))
        wc = (struct aclk_database_worker_config *)host->dbsync_worker;
    rrd_unlock();
    if (wc)
        aclk_database_enq_cmd(wc, &cmd);
    else {
        if (aclk_worker_enq_cmd(node_id, &cmd))
            error_report("ACLK synchronization thread is not active for node id %s", node_id);
    }
    return;
}

void aclk_push_alarm_health_log(struct aclk_database_worker_config *wc, struct aclk_database_cmd cmd)
{
    UNUSED(cmd);
#ifndef ENABLE_NEW_CLOUD_PROTOCOL
    UNUSED(wc);
#else
    int rc;

    char *claim_id = is_agent_claimed();
    if (unlikely(!claim_id))
        return;

    uint64_t first_sequence = 0;
    uint64_t last_sequence = 0;
    struct timeval first_timestamp;
    struct timeval last_timestamp;

    BUFFER *sql = buffer_create(1024);

    sqlite3_stmt *res = NULL;

    //TODO: make this better: include info from health log too
    buffer_sprintf(sql, "select aa.sequence_id, aa.date_created, \
                         (select laa.sequence_id from aclk_alert_%s laa \
                         order by laa.sequence_id desc limit 1), \
                         (select laa.date_created from aclk_alert_%s laa \
                         order by laa.sequence_id desc limit 1) \
                         from aclk_alert_%s aa order by aa.sequence_id asc limit 1;", wc->uuid_str, wc->uuid_str, wc->uuid_str);

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

    while (sqlite3_step(res) == SQLITE_ROW) {
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
    alarm_log.node_id = strdupz(wc->node_id);
    alarm_log.log_entries = log_entries;
    alarm_log.status = wc->alert_updates == 0 ? 2 : 1;

    wc->alert_sequence_id = last_sequence;

    aclk_send_alarm_log_health(&alarm_log);

    rc = sqlite3_finalize(res);
    if (unlikely(rc != SQLITE_OK))
        error_report("Failed to reset statement to get health log statistics from the database, rc = %d", rc);

    freez((char *)alarm_log.node_id);
    freez(claim_id);
    buffer_free(sql);
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

    struct aclk_database_cmd cmd;
    memset(&cmd, 0, sizeof(cmd));
    cmd.opcode = ACLK_DATABASE_PUSH_ALERT_CONFIG;
    cmd.data_param = (void *) strdupz(config_hash);
    cmd.completion = NULL;
    aclk_database_enq_cmd(wc, &cmd);

    return;
}

int aclk_push_alert_config_event(struct aclk_database_worker_config *wc, struct aclk_database_cmd cmd)
{
    UNUSED(wc);
#ifndef ENABLE_NEW_CLOUD_PROTOCOL
    UNUSED(cmd);
#else
    int rc = 0;

    CHECK_SQLITE_CONNECTION(db_meta);

    sqlite3_stmt *res = NULL;

    char *config_hash = (char *) cmd.data_param;
    BUFFER *sql = buffer_create(1024);
    buffer_sprintf(
        sql,
        "SELECT alarm, template, on_key, class, type, component, os, hosts, plugin, module, charts, families, lookup, every, units, green, red, calc, warn, crit, to_key, exec, delay, repeat, info, options, host_labels, p_db_lookup_dimensions, p_db_lookup_method, p_db_lookup_options, p_db_lookup_after, p_db_lookup_before, p_update_every FROM alert_hash WHERE hash_id = @hash_id;");

    rc = sqlite3_prepare_v2(db_meta, buffer_tostring(sql), -1, &res, 0);
    if (rc != SQLITE_OK) {
        error_report("Failed to prepare statement when trying to fetch a chart hash configuration");
        goto fail;
    }

    uuid_t hash_uuid;
    if (uuid_parse(config_hash, hash_uuid))
        goto fail;

    rc = sqlite3_bind_blob(res, 1, &hash_uuid , sizeof(hash_uuid), SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    struct aclk_alarm_configuration alarm_config;
    struct provide_alarm_configuration p_alarm_config;
    p_alarm_config.cfg_hash = NULL;

    if (sqlite3_step(res) == SQLITE_ROW) {

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
        debug(D_ACLK_SYNC, "Sending alert config for %s", config_hash);
        aclk_send_provide_alarm_cfg(&p_alarm_config);
        freez((char *) cmd.data_param);
        freez(p_alarm_config.cfg_hash);
        destroy_aclk_alarm_configuration(&alarm_config);
    }
    else
        info("DEBUG: Alert config for %s not found", config_hash);

    bind_fail:
    rc = sqlite3_finalize(res);
    if (unlikely(rc != SQLITE_OK))
        error_report("Failed to reset statement when pushing alarm config hash, rc = %d", rc);

    fail:
    buffer_free(sql);

    return rc;
#endif
    return 0;
}


// Start streaming alerts
void aclk_start_alert_streaming(char *node_id, uint64_t batch_id, uint64_t start_seq_id)
{
#ifdef ENABLE_NEW_CLOUD_PROTOCOL
    if (unlikely(!node_id))
        return;

    uuid_t node_uuid;
    if (uuid_parse(node_id, node_uuid))
        return;

    struct aclk_database_worker_config *wc  = NULL;
    rrd_wrlock();
    RRDHOST *host = find_host_by_node_id(node_id);
    if (likely(host))
        wc = (struct aclk_database_worker_config *)host->dbsync_worker;
    rrd_unlock();

    if (likely(wc)) {
        info("START streaming alerts for %s enabled with batch_id %"PRIu64" and start_seq_id %"PRIu64, node_id, batch_id, start_seq_id);
        __sync_synchronize();
        wc->alerts_batch_id = batch_id;
        wc->alerts_start_seq_id = start_seq_id;
        wc->alert_updates = 1;
        __sync_synchronize();
    }
    else
        error("ACLK synchronization thread is not active for host %s", host->hostname);

#else
    UNUSED(node_id);
    UNUSED(start_seq_id);
    UNUSED(batch_id);
#endif
    return;
}

int sql_queue_removed_alerts_to_aclk(RRDHOST *host)
{
    CHECK_SQLITE_CONNECTION(db_meta);

    struct aclk_database_worker_config *wc = (struct aclk_database_worker_config *) host->dbsync_worker;
    if (unlikely(!wc)) {
        return 1;
    }

    BUFFER *sql = buffer_create(1024);

    buffer_sprintf(sql,"insert into aclk_alert_%s (alert_unique_id, date_created) " \
                   "select unique_id alert_unique_id, strftime('%%s') date_created from health_log_%s where new_status = -2 and updated_by_id = 0 and unique_id not in (select alert_unique_id from aclk_alert_%s) order by unique_id asc on conflict (alert_unique_id) do nothing;", wc->uuid_str, wc->uuid_str, wc->uuid_str);

    db_execute(buffer_tostring(sql));

    buffer_free(sql);

    return 0;
}
