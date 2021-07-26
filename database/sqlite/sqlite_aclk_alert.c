// SPDX-License-Identifier: GPL-3.0-or-later

#include "sqlite_functions.h"
#include "sqlite_aclk_alert.h"

#ifndef ACLK_NG
#include "../../aclk/legacy/agent_cloud_link.h"
#else
#include "../../aclk/aclk.h"
#include "../../aclk/aclk_charts_api.h"
#include "../../aclk/aclk_alarm_api.h"
#endif


void sql_queue_alarm_to_aclk(RRDHOST *host, ALARM_ENTRY *ae)
{
    if (!aclk_architecture) //{ //queue_chart_to_aclk continues after that, should we too ?
        aclk_update_alarm(host, ae);

    if (ae->flags & HEALTH_ENTRY_FLAG_ACLK_QUEUED)
        return;

    if (unlikely(!host->dbsync_worker))
        return;

    //check if ae->config_hash exists, if not, don't queue it!
    if (unlikely(uuid_is_null(ae->config_hash_id)))
        return;

    struct aclk_database_cmd cmd;
    cmd.opcode = ACLK_DATABASE_ADD_ALERT;
    cmd.data = ae;
    cmd.completion = NULL;
    aclk_database_enq_cmd((struct aclk_database_worker_config *) host->dbsync_worker, &cmd);
    ae->flags |= HEALTH_ENTRY_FLAG_ACLK_QUEUED;
    return;
}


int aclk_add_alert_event(struct aclk_database_worker_config *wc, struct aclk_database_cmd cmd)
{
    int rc = 0;
    if (unlikely(!db_meta)) {
        if (default_rrd_memory_mode != RRD_MEMORY_MODE_DBENGINE) {
            return 1;
        }
        error_report("Database has not been initialized");
        return 1;
    }

    sqlite3_stmt *res_alert = NULL;
    ALARM_ENTRY *ae = cmd.data;

    BUFFER *sql = buffer_create(1024);

    buffer_sprintf(sql,"INSERT INTO aclk_alert_%s (alert_unique_id, date_created) " \
                 "VALUES (@alert_unique_id, strftime('%%s')); ", wc->uuid_str);

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

            /* case RRDCALC_STATUS_UNINITIALIZED: */
            /*     return "UNINITIALIZED"; */

        case RRDCALC_STATUS_CLEAR:
            return ALARM_STATUS_CLEAR;

            /* case RRDCALC_STATUS_RAISED: */
            /*     return "RAISED"; */

        case RRDCALC_STATUS_WARNING:
            return ALARM_STATUS_WARNING;

        case RRDCALC_STATUS_CRITICAL:
            return ALARM_STATUS_CRITICAL;

        default:
            error("Unknown alarm status %d", status);
            return ALARM_STATUS_UNKNOWN;
    }
}

/* only to help ng, this is in master already */
char *health_edit_command_from_source(const char *source)
{
    char buffer[FILENAME_MAX + 1];
    char *temp = strdupz(source);
    char *line_num = strchr(temp, '@');
    char *file_no_path = strrchr(temp, '/');

    if (likely(file_no_path && line_num)) {
        *line_num = '\0';
        snprintfz(
            buffer,
            FILENAME_MAX,
            "sudo %s/edit-config health.d/%s=%s",
            netdata_configured_user_config_dir,
            file_no_path + 1,
            temp);
    } else
        buffer[0] = '\0';

    freez(temp);
    return strdupz(buffer);
}


void aclk_push_alert_event(struct aclk_database_worker_config *wc, struct aclk_database_cmd cmd)
{
#ifndef ACLK_NG
    UNUSED (wc);
    UNUSED(cmd);
#else
    int rc;

    int limit = cmd.count > 0 ? cmd.count : 1;
    char uuid_str[GUID_LEN + 1];
    BUFFER *sql = buffer_create(1024);

    //check if we're in sync with cloud, via start_seq_id
    //if not, reset sequence ids. If not there, problem
    //this is not ideal, since we'll query the db more than needed
    if (wc->alerts_start_seq_id != 0) {
        buffer_sprintf(sql, "UPDATE aclk_alert_%s SET date_submitted = NULL WHERE sequence_id >= %"PRIu64";",
                       wc->uuid_str, wc->alerts_start_seq_id);
        db_execute(buffer_tostring(sql));
        buffer_reset(sql);
        wc->alerts_start_seq_id = 0;
    }

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
        error_report("Failed to prepare statement to get health entries from the database");
        goto fail_complete;
    }

//    int count = 0;
    char *claim_id = is_agent_claimed();
    while (sqlite3_step(res) == SQLITE_ROW) {

        struct alarm_log_entry alarm_log;
        char old_value_string[100 + 1];
        char new_value_string[100 + 1];

        uuid_unparse_lower(*((uuid_t *) sqlite3_column_blob(res, 3)), uuid_str);

        alarm_log.node_id = get_str_from_uuid(wc->host->node_id);
        alarm_log.claim_id = strdupz((char *)claim_id);

        alarm_log.chart = strdupz((char *)sqlite3_column_text(res, 12));
        alarm_log.name = strdupz((char *)sqlite3_column_text(res, 11));
        alarm_log.family = sqlite3_column_bytes(res, 13) > 0 ? strdupz((char *)sqlite3_column_text(res, 13)) : NULL;

        alarm_log.batch_id = wc->alerts_batch_id;
        alarm_log.sequence_id = (uint64_t) sqlite3_column_int64(res, 0);
        alarm_log.when = (uint64_t) sqlite3_column_int64(res, 5);

        alarm_log.config_hash = strdupz((char *)uuid_str);

        alarm_log.utc_offset = 10800; //will be there in master
        alarm_log.timezone = strdupz((char *)"EEST"); //will be there in master

        alarm_log.exec_path = sqlite3_column_bytes(res, 14) > 0 ? strdupz((char *)sqlite3_column_text(res, 14)) : strdupz((char *)wc->host->health_default_exec);
        alarm_log.conf_source = strdupz((char *)sqlite3_column_text(res, 16));

        char *edit_command = sqlite3_column_bytes(res, 16) > 0 ? health_edit_command_from_source((char *)sqlite3_column_text(res, 16)) : strdupz("UNKNOWN=0");
        alarm_log.command = strdupz(edit_command);

        alarm_log.duration = (uint32_t) sqlite3_column_int(res, 6); //correct ?
        alarm_log.non_clear_duration = (uint32_t) sqlite3_column_int(res, 7); //correct?

        alarm_log.status = rrdcalc_status_to_proto_enum((RRDCALC_STATUS) sqlite3_column_int(res, 20));
        alarm_log.old_status = rrdcalc_status_to_proto_enum((RRDCALC_STATUS) sqlite3_column_int(res, 21));

        alarm_log.delay = (uint64_t) sqlite3_column_int64(res, 22);
        alarm_log.delay_up_to_timestamp = (uint64_t) sqlite3_column_int64(res, 10);
        alarm_log.last_repeat = (uint64_t) sqlite3_column_int64(res, 25);

        alarm_log.silenced = ( (sqlite3_column_int64(res, 8) & HEALTH_ENTRY_FLAG_SILENCED)  || ( sqlite3_column_type(res, 15) != SQLITE_NULL && !strncmp((char *)sqlite3_column_text(res,15), "silent", 6)) ) ? 1 : 0;

        alarm_log.value_string = sqlite3_column_type(res, 23) == SQLITE_NULL ? strdupz((char *)"-") : strdupz((char *)format_value_and_unit(new_value_string, 100, sqlite3_column_double(res, 23), (char *) sqlite3_column_text(res, 17), -1));
        alarm_log.old_value_string = sqlite3_column_type(res, 24) == SQLITE_NULL ? strdupz((char *)"-") : strdupz((char *)format_value_and_unit(old_value_string, 100, sqlite3_column_double(res, 24), (char *) sqlite3_column_text(res, 17), -1));

        alarm_log.value = (double) sqlite3_column_double(res, 23);
        alarm_log.old_value = (double) sqlite3_column_double(res, 24);

        alarm_log.updated = (sqlite3_column_int64(res, 8) & HEALTH_ENTRY_FLAG_UPDATED) ? 1 : 0; //check when 0

        alarm_log.rendered_info = strdupz((char *)sqlite3_column_text(res, 18)); //check, should be already ok?

        info("DEBUG: %s pushing alert seq %" PRIu64 " - %" PRIu64"", wc->uuid_str, (uint64_t) sqlite3_column_int64(res, 0), (uint64_t) sqlite3_column_int64(res, 1));
        aclk_send_alarm_log_entry(&alarm_log);

        buffer_sprintf(sql, "UPDATE aclk_alert_%s SET date_submitted=strftime('%%s') "
                            "WHERE sequence_id = %" PRIu64 " AND alert_unique_id = %" PRIu64 ";",
                       wc->uuid_str, (uint64_t) sqlite3_column_int64(res, 0), (uint64_t) sqlite3_column_int64(res, 1));

        db_execute(buffer_tostring(sql));
        destroy_alarm_log_entry(&alarm_log);
        freez(edit_command);
    }

    rc = sqlite3_finalize(res);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to finalize statement to send Alarm Entries from the database, rc = %d", rc);
        goto fail_complete;
    }

    //need stuff
    freez(claim_id);

    fail_complete:
    buffer_free(sql);
#endif

    return;
}

void aclk_send_alarm_health_log(char *node_id)
{
    struct aclk_database_worker_config *wc  = NULL;

    rrd_wrlock();
    RRDHOST *host = find_host_by_node_id(node_id);
    if (likely(host)) {
        wc = (struct aclk_database_worker_config *)host->dbsync_worker;
        rrd_unlock();
        if (likely(wc)) {
            struct aclk_database_cmd cmd;
            cmd.opcode = ACLK_DATABASE_ALARM_HEALTH_LOG;
            //cmd.param1 = param;
            cmd.completion = NULL;
            aclk_database_enq_cmd(wc, &cmd);
        } else
            error("ACLK synchronization thread is not active for host %s", host->hostname);
    }
    else
        rrd_unlock(); //is this ok ???
    return;
}

void aclk_push_alarm_health_log(struct aclk_database_worker_config *wc, struct aclk_database_cmd cmd)
{
    UNUSED(cmd);
#ifndef ACLK_NG
    UNUSED (wc);
#else
    int rc;

//    int available = 0;
    uint64_t first_sequence = 0;
    uint64_t last_sequence = 0;
    struct timeval first_timestamp;
    struct timeval last_timestamp;
//    char uuid_str[GUID_LEN + 1];

    BUFFER *sql = buffer_create(1024);

    sqlite3_stmt *res = NULL;

    buffer_sprintf(sql, "select aa.sequence_id, aa.date_created, \
                         (select laa.sequence_id from aclk_alert_%s laa \
                         order by laa.sequence_id desc limit 1), \
                         (select laa.date_created from aclk_alert_%s laa \
                         order by laa.sequence_id desc limit 1) \
                         from aclk_alert_%s aa order by aa.sequence_id asc limit 1;", wc->uuid_str, wc->uuid_str, wc->uuid_str);

    rc = sqlite3_prepare_v2(db_meta, buffer_tostring(sql), -1, &res, 0);
    if (rc != SQLITE_OK) {
        error_report("Failed to prepare statement to get health log statistics from the database");
        goto fail_complete;
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

    rc = sqlite3_finalize(res);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to reset statement to get health log statistics from the database, rc = %d", rc);
        goto fail_complete;
    }

    struct alarm_log_entries log_entries;
    log_entries.first_seq_id = first_sequence;
    log_entries.first_when = first_timestamp;
    log_entries.last_seq_id = last_sequence;
    log_entries.last_when = last_timestamp;

    struct alarm_log_health alarm_log;
    char *claim_id = is_agent_claimed();
    alarm_log.claim_id = strdupz((char *)claim_id);
    alarm_log.node_id = get_str_from_uuid(wc->host->node_id);
    alarm_log.log_entries = log_entries;
    alarm_log.status = wc->alert_updates == 0 ? 2 : 1;

    wc->alert_sequence_id = last_sequence;

    aclk_send_alarm_log_health(&alarm_log);
    freez(alarm_log.claim_id);

    freez(claim_id);

    fail_complete:
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
    cmd.opcode = ACLK_DATABASE_PUSH_ALERT_CONFIG;

    info("Request for alert config %s received", config_hash);
    cmd.data_param = (void *) strdupz(config_hash);
    cmd.completion = NULL;
    aclk_database_enq_cmd(wc, &cmd);

    return;
}

int aclk_push_alert_config_event(struct aclk_database_worker_config *wc, struct aclk_database_cmd cmd)
{
    UNUSED(wc);
    sqlite3_stmt *res = NULL;

    int rc = 0;
    if (unlikely(!db_meta)) {
        if (default_rrd_memory_mode != RRD_MEMORY_MODE_DBENGINE) {
            return 1;
        }
        error_report("Database has not been initialized");
        return 1;
    }

    char *config_hash = (char *) cmd.data_param;
    BUFFER *sql = buffer_create(1024);
    buffer_sprintf(sql, "SELECT alarm, template, on_key, class, type, component, os, hosts, plugin, module, charts, families, lookup, every, units, green, red, calc, warn, crit, to_key, exec, delay, repeat, info, options, host_labels, p_db_lookup_dimensions, p_db_lookup_method, p_db_lookup_options, p_db_lookup_after, p_db_lookup_before, p_update_every FROM alert_hash WHERE hash_id = @hash_id;");

    rc = sqlite3_prepare_v2(db_meta, buffer_tostring(sql), -1, &res, 0);
    if (rc != SQLITE_OK) {
        error_report("Failed to prepare statement when trying to fetch a chart hash configuration");
        goto fail;
    }

    uuid_t hash_uuid;
    uuid_parse(config_hash, hash_uuid);
    rc = sqlite3_bind_blob(res, 1, &hash_uuid , sizeof(hash_uuid), SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    //check if we indeed have the alarm config, bail out if we dont

    struct aclk_alarm_configuration alarm_config;

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

        //get some more values from rc for this hash_id
        //this might not work... If this hash is requested from an
        //agent that does not have an rc for this hash, it will not send back
        //a full correct message..
        //perhaps it's better to run the parsers on the sql data instead of looking up rc's...
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

        struct provide_alarm_configuration p_alarm_config;

        p_alarm_config.cfg_hash = strdupz((char *) config_hash);
        p_alarm_config.cfg = alarm_config;

        info("Sending alarm config for %s", config_hash);

        aclk_send_provide_alarm_cfg(&p_alarm_config);
        freez((char *) cmd.data_param);
        freez(p_alarm_config.cfg_hash);
        destroy_aclk_alarm_configuration(&alarm_config);
    }

    bind_fail:
    rc = sqlite3_finalize(res);
    if (unlikely(rc != SQLITE_OK))
        error_report("Failed to reset statement when pushing alarm config hash, rc = %d", rc);

    fail:
    buffer_free(sql);

    return rc;
}


// Start streaming alerts
void aclk_start_alert_streaming(char *node_id, uint64_t batch_id, uint64_t start_seq_id)
{
    if (unlikely(!node_id))
        return;

    info("START streaming alerts for %s received", node_id);

    uuid_t node_uuid;
    uuid_parse(node_id, node_uuid);

    struct aclk_database_worker_config *wc  = NULL;
    rrd_wrlock();
    RRDHOST *host = localhost;
    while(host) {
        if (host->node_id && !(uuid_compare(*host->node_id, node_uuid))) {
            wc = (struct aclk_database_worker_config *)host->dbsync_worker;
            if (likely(wc)) {
                wc->alert_updates = 1;
                wc->alerts_batch_id = batch_id;
                wc->alerts_start_seq_id = start_seq_id;
                info("START streaming alerts for %s enabled with batch_id %"PRIu64" and start_seq_id %"PRIu64, node_id, batch_id, start_seq_id);
            }
            else
                error("ACLK synchronization thread is not active for host %s", host->hostname);
            break;
        }
        host = host->next;
    }
    rrd_unlock();

    return;
}
