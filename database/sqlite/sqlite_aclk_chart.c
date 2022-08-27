// SPDX-License-Identifier: GPL-3.0-or-later

#include "sqlite_functions.h"
#include "sqlite_aclk_chart.h"

#ifdef ENABLE_ACLK
#include "../../aclk/aclk_charts_api.h"
#include "../../aclk/aclk.h"

static inline int
sql_queue_chart_payload(struct aclk_database_worker_config *wc, void *data, enum aclk_database_opcode opcode)
{
    int rc;
    if (unlikely(!wc))
        return 1;

    struct aclk_database_cmd cmd;
    memset(&cmd, 0, sizeof(cmd));
    cmd.opcode = opcode;
    cmd.data = data;
    rc = aclk_database_enq_cmd_noblock(wc, &cmd);
    return rc;
}

static time_t payload_sent(char *uuid_str, uuid_t *uuid, void *payload, size_t payload_size)
{
    static __thread sqlite3_stmt *res = NULL;
    int rc;
    time_t send_status = 0;

    if (unlikely(!res)) {
        char sql[ACLK_SYNC_QUERY_SIZE];
        snprintfz(sql,ACLK_SYNC_QUERY_SIZE-1, "SELECT acl.date_submitted FROM aclk_chart_latest_%s acl, aclk_chart_payload_%s acp "
            "WHERE acl.unique_id = acp.unique_id AND acl.uuid = @uuid AND acp.payload = @payload;",
                  uuid_str, uuid_str);
        rc = prepare_statement(db_meta, sql, &res);
        if (rc != SQLITE_OK) {
            error_report("Failed to prepare statement to check payload data on %s", sql);
            return 0;
        }
    }

    rc = sqlite3_bind_blob(res, 1, uuid, sizeof(*uuid), SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    rc = sqlite3_bind_blob(res, 2, payload, payload_size, SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    while (sqlite3_step_monitored(res) == SQLITE_ROW) {
        send_status = (time_t) sqlite3_column_int64(res, 0);
    }

bind_fail:
    if (unlikely(sqlite3_reset(res) != SQLITE_OK))
        error_report("Failed to reset statement in check payload, rc = %d", rc);
    return send_status;
}

static int aclk_add_chart_payload(
    struct aclk_database_worker_config *wc,
    uuid_t *uuid,
    char *claim_id,
    ACLK_PAYLOAD_TYPE payload_type,
    void *payload,
    size_t payload_size,
    time_t *send_status,
    int check_sent)
{
    static __thread sqlite3_stmt *res_chart = NULL;
    int rc;
    time_t date_submitted;

    if (unlikely(!payload))
        return 0;

    if (check_sent) {
        date_submitted = payload_sent(wc->uuid_str, uuid, payload, payload_size);
        if (send_status)
            *send_status = date_submitted;
        if (date_submitted)
            return 0;
    }

    if (unlikely(!res_chart)) {
        char sql[ACLK_SYNC_QUERY_SIZE];
        snprintfz(sql,ACLK_SYNC_QUERY_SIZE-1,
                  "INSERT INTO aclk_chart_payload_%s (unique_id, uuid, claim_id, date_created, type, payload) " \
                  "VALUES (@unique_id, @uuid, @claim_id, unixepoch(), @type, @payload);", wc->uuid_str);
        rc = prepare_statement(db_meta, sql, &res_chart);
        if (rc != SQLITE_OK) {
            error_report("Failed to prepare statement to store chart payload data");
            return 1;
        }
    }

    uuid_t unique_uuid;
    uuid_generate(unique_uuid);

    uuid_t claim_uuid;
    if (uuid_parse(claim_id, claim_uuid))
        return 1;

    rc = sqlite3_bind_blob(res_chart, 1, &unique_uuid, sizeof(unique_uuid), SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    rc = sqlite3_bind_blob(res_chart, 2, uuid, sizeof(*uuid), SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    rc = sqlite3_bind_blob(res_chart, 3, &claim_uuid, sizeof(claim_uuid), SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    rc = sqlite3_bind_int(res_chart, 4, payload_type);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    rc = sqlite3_bind_blob(res_chart, 5, payload, payload_size, SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    rc = execute_insert(res_chart);
    if (unlikely(rc != SQLITE_DONE))
        error_report("Failed store chart payload event, rc = %d", rc);
    else {
        wc->chart_payload_count++;
        time_t now = now_realtime_sec();
        if (wc->rotation_after > now && wc->rotation_after < now + ACLK_DATABASE_ROTATION_DELAY)
            wc->rotation_after = now + ACLK_DATABASE_ROTATION_DELAY;
    }

bind_fail:
    if (unlikely(sqlite3_reset(res_chart) != SQLITE_OK))
        error_report("Failed to reset statement in store chart payload, rc = %d", rc);
    return (rc != SQLITE_DONE);
}

int aclk_add_chart_event(struct aclk_database_worker_config *wc, struct aclk_database_cmd cmd)
{
    int rc = 0;
    CHECK_SQLITE_CONNECTION(db_meta);

    char *claim_id = get_agent_claimid();

    RRDSET *st = cmd.data;

    if (likely(claim_id)) {
        struct chart_instance_updated chart_payload;
        memset(&chart_payload, 0, sizeof(chart_payload));
        chart_payload.config_hash = get_str_from_uuid(&st->state->hash_id);
        chart_payload.update_every = st->update_every;
        chart_payload.memory_mode = st->rrd_memory_mode;
        chart_payload.name = (char *)rrdset_name(st);
        chart_payload.node_id = wc->node_id;
        chart_payload.claim_id = claim_id;
        chart_payload.id = strdupz(rrdset_id(st));

        chart_payload.chart_labels = rrdlabels_create();
        rrdlabels_copy(chart_payload.chart_labels, st->state->chart_labels);

        size_t size;
        char *payload = generate_chart_instance_updated(&size, &chart_payload);
        if (likely(payload))
            rc = aclk_add_chart_payload(wc, st->chart_uuid, claim_id, ACLK_PAYLOAD_CHART, (void *) payload, size, NULL, 1);
        freez(payload);
        chart_instance_updated_destroy(&chart_payload);
    }
    return rc;
}

static inline int aclk_upd_dimension_event(struct aclk_database_worker_config *wc, char *claim_id, uuid_t *dim_uuid,
        const char *dim_id, const char *dim_name, const char *chart_type_id, time_t first_time, time_t last_time,
        time_t *send_status)
{
    int rc = 0;
    size_t size;

    if (unlikely(!dim_uuid || !dim_id || !dim_name || !chart_type_id))
        return 0;

    struct chart_dimension_updated dim_payload;
    memset(&dim_payload, 0, sizeof(dim_payload));

#ifdef NETDATA_INTERNAL_CHECKS
    if (!first_time)
        info("Host %s (node %s) deleting dimension id=[%s] name=[%s] chart=[%s]",
                wc->host_guid, wc->node_id, dim_id, dim_name, chart_type_id);
    if (last_time)
        info("Host %s (node %s) stopped collecting dimension id=[%s] name=[%s] chart=[%s] %ld seconds ago at %ld",
             wc->host_guid, wc->node_id, dim_id, dim_name, chart_type_id, now_realtime_sec() - last_time, last_time);
#endif

    dim_payload.node_id = wc->node_id;
    dim_payload.claim_id = claim_id;
    dim_payload.name = dim_name;
    dim_payload.id = dim_id;
    dim_payload.chart_id = chart_type_id;
    dim_payload.created_at.tv_sec = first_time;
    dim_payload.last_timestamp.tv_sec = last_time;
    char *payload = generate_chart_dimension_updated(&size, &dim_payload);
    if (likely(payload))
        rc = aclk_add_chart_payload(wc, dim_uuid, claim_id, ACLK_PAYLOAD_DIMENSION, (void *)payload, size, send_status, 1);
    freez(payload);
    return rc;
}

void aclk_process_dimension_deletion(struct aclk_database_worker_config *wc, struct aclk_database_cmd cmd)
{
    int rc = 0;
    sqlite3_stmt *res = NULL;

    if (!aclk_connected)
        return;

    if (unlikely(!db_meta))
        return;

    uuid_t host_id;
    if (uuid_parse(wc->host_guid, host_id))
        return;

    char *claim_id = get_agent_claimid();
    if (!claim_id)
        return;

    rc = sqlite3_prepare_v2(
        db_meta,
        "DELETE FROM dimension_delete where host_id = @host_id "
        "RETURNING dimension_id, dimension_name, chart_type_id, dim_id LIMIT 10;",
        -1,
        &res,
        0);

    if (rc != SQLITE_OK) {
        error_report("Failed to prepare statement when trying to delete dimension deletes");
        freez(claim_id);
        return;
    }

    rc = sqlite3_bind_blob(res, 1, &host_id, sizeof(host_id), SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    unsigned count = 0;
    while (sqlite3_step_monitored(res) == SQLITE_ROW) {
        (void) aclk_upd_dimension_event(
            wc,
            claim_id,
            (uuid_t *)sqlite3_column_text(res, 3),
            (const char *)sqlite3_column_text(res, 0),
            (const char *)sqlite3_column_text(res, 1),
            (const char *)sqlite3_column_text(res, 2),
            0,
            0,
            NULL);
        count++;
    }

    if (count) {
        memset(&cmd, 0, sizeof(cmd));
        cmd.opcode = ACLK_DATABASE_DIM_DELETION;
        if (aclk_database_enq_cmd_noblock(wc, &cmd))
            info("Failed to queue a dimension deletion message");
    }

bind_fail:
    rc = sqlite3_finalize(res);
    if (unlikely(rc != SQLITE_OK))
        error_report("Failed to finalize statement when adding dimension deletion events, rc = %d", rc);
    freez(claim_id);
    return;
}

int aclk_add_dimension_event(struct aclk_database_worker_config *wc, struct aclk_database_cmd cmd)
{
    int rc = 1;
    CHECK_SQLITE_CONNECTION(db_meta);

    struct aclk_chart_dimension_data *aclk_cd_data = cmd.data;

    char *claim_id = get_agent_claimid();
    if (!claim_id)
        goto cleanup;

    rc = aclk_add_chart_payload(wc, &aclk_cd_data->uuid, claim_id, ACLK_PAYLOAD_DIMENSION,
              (void *) aclk_cd_data->payload, aclk_cd_data->payload_size, NULL, aclk_cd_data->check_payload);

    freez(claim_id);
cleanup:
    freez(aclk_cd_data->payload);
    freez(aclk_cd_data);
    return rc;
}

void aclk_send_chart_event(struct aclk_database_worker_config *wc, struct aclk_database_cmd cmd)
{
    int rc;

    wc->chart_pending = 0;
    if (unlikely(!wc->chart_updates)) {
        log_access(
            "ACLK STA [%s (%s)]: Ignoring chart push event, updates have been turned off for this node.",
            wc->node_id,
            wc->host ? rrdhost_hostname(wc->host) : "N/A");
        return;
    }

    char *claim_id = get_agent_claimid();
    if (unlikely(!claim_id))
        return;

    uuid_t claim_uuid;
    if (uuid_parse(claim_id, claim_uuid))
        return;

    int limit = cmd.count > 0 ? cmd.count : 1;

    uint64_t first_sequence;
    uint64_t last_sequence;
    time_t last_timestamp = 0;

    char sql[ACLK_SYNC_QUERY_SIZE];
    static __thread sqlite3_stmt *res = NULL;

    if (unlikely(!res)) {
        snprintfz(sql,ACLK_SYNC_QUERY_SIZE-1,"SELECT ac.sequence_id, acp.payload, ac.date_created, ac.type, ac.uuid  " \
             "FROM aclk_chart_%s ac, aclk_chart_payload_%s acp " \
             "WHERE ac.date_submitted IS NULL AND ac.unique_id = acp.unique_id AND ac.update_count > 0 " \
             "AND acp.claim_id = @claim_id ORDER BY ac.sequence_id ASC LIMIT %d;", wc->uuid_str, wc->uuid_str, limit);
        rc = prepare_statement(db_meta, sql, &res);
        if (rc != SQLITE_OK) {
            error_report("Failed to prepare statement when trying to send a chart update via ACLK");
            freez(claim_id);
            return;
        }
    }

    rc = sqlite3_bind_blob(res, 1, claim_uuid, sizeof(claim_uuid), SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    char **payload_list = callocz(limit + 1, sizeof(char *));
    size_t *payload_list_size = callocz(limit + 1, sizeof(size_t));
    size_t *payload_list_max_size = callocz(limit + 1, sizeof(size_t));
    struct aclk_message_position *position_list = callocz(limit + 1, sizeof(*position_list));
    int *is_dim = callocz(limit + 1, sizeof(*is_dim));

    int loop = cmd.param1;

    uint64_t start_sequence_id = wc->chart_sequence_id;

    while (loop > 0) {
        uint64_t previous_sequence_id = wc->chart_sequence_id;
        int count = 0;
        first_sequence = 0;
        last_sequence = 0;
        while (count < limit && sqlite3_step_monitored(res) == SQLITE_ROW) {
            size_t payload_size = sqlite3_column_bytes(res, 1);
            if (payload_list_max_size[count] < payload_size) {
                freez(payload_list[count]);
                payload_list_max_size[count] = payload_size;
                payload_list[count] = mallocz(payload_size);
            }
            payload_list_size[count] = payload_size;
            memcpy(payload_list[count], sqlite3_column_blob(res, 1), payload_size);
            position_list[count].sequence_id = (uint64_t)sqlite3_column_int64(res, 0);
            position_list[count].previous_sequence_id = previous_sequence_id;
            position_list[count].seq_id_creation_time.tv_sec = sqlite3_column_int64(res, 2);
            position_list[count].seq_id_creation_time.tv_usec = 0;
            if (!first_sequence)
                first_sequence = position_list[count].sequence_id;
            last_sequence = position_list[count].sequence_id;
            last_timestamp = position_list[count].seq_id_creation_time.tv_sec;
            previous_sequence_id = last_sequence;
            is_dim[count] = sqlite3_column_int(res, 3) > 0;
            count++;
            if (wc->chart_payload_count)
                wc->chart_payload_count--;
        }
        freez(payload_list[count]);
        payload_list_max_size[count] = 0;
        payload_list[count] = NULL;

        rc = sqlite3_reset(res);
        if (unlikely(rc != SQLITE_OK))
            error_report("Failed to reset statement when pushing chart events, rc = %d", rc);

        if (likely(first_sequence)) {

            db_lock();
            snprintfz(sql,ACLK_SYNC_QUERY_SIZE-1, "UPDATE aclk_chart_%s SET status = NULL, date_submitted=unixepoch() "
                                "WHERE date_submitted IS NULL AND sequence_id BETWEEN %" PRIu64 " AND %" PRIu64 ";",
                                wc->uuid_str, first_sequence, last_sequence);
            db_execute(sql);
            snprintfz(sql,ACLK_SYNC_QUERY_SIZE-1, "INSERT OR REPLACE INTO aclk_chart_latest_%s (uuid, unique_id, date_submitted) "
                                " SELECT uuid, unique_id, date_submitted FROM aclk_chart_%s s "
                                " WHERE date_submitted IS NOT NULL AND sequence_id BETWEEN %" PRIu64 " AND %" PRIu64
                                " ;",
                                wc->uuid_str, wc->uuid_str, first_sequence, last_sequence);
            db_execute(sql);
            db_unlock();

            aclk_chart_inst_and_dim_update(payload_list, payload_list_size, is_dim, position_list, wc->batch_id);
            log_access(
                "ACLK RES [%s (%s)]: CHARTS SENT from %" PRIu64 " to %" PRIu64 " batch=%" PRIu64,
                wc->node_id,
                wc->hostname ? wc->hostname : "N/A",
                first_sequence,
                last_sequence,
                wc->batch_id);
            wc->chart_sequence_id = last_sequence;
            wc->chart_timestamp = last_timestamp;
        } else
            break;
        --loop;
    }

    if (start_sequence_id != wc->chart_sequence_id) {
        time_t now = now_realtime_sec();
        if (wc->rotation_after > now && wc->rotation_after < now + ACLK_DATABASE_ROTATION_DELAY)
            wc->rotation_after = now + ACLK_DATABASE_ROTATION_DELAY;
    } else {
        wc->chart_payload_count = sql_get_pending_count(wc);
        if (!wc->chart_payload_count)
            log_access(
                "ACLK STA [%s (%s)]: Sync of charts and dimensions done in %ld seconds.",
                wc->node_id,
                wc->hostname ? wc->hostname : "N/A",
                now_realtime_sec() - wc->startup_time);
    }

    for (int i = 0; i <= limit; ++i)
        freez(payload_list[i]);

    freez(payload_list);
    freez(payload_list_size);
    freez(payload_list_max_size);
    freez(position_list);
    freez(is_dim);

bind_fail:
    rc = sqlite3_reset(res);
    if (unlikely(rc != SQLITE_OK))
        error_report("Failed to reset statement when pushing chart events, rc = %d", rc);

    freez(claim_id);
    return;
}

// Push one chart config to the cloud
int aclk_send_chart_config(struct aclk_database_worker_config *wc, struct aclk_database_cmd cmd)
{
    UNUSED(wc);

    CHECK_SQLITE_CONNECTION(db_meta);

    sqlite3_stmt *res = NULL;
    int rc = 0;

    char *hash_id = (char *) cmd.data_param;

    uuid_t hash_uuid;
    rc = uuid_parse(hash_id, hash_uuid);

    if (unlikely(rc)) {
        freez((char *) cmd.data_param);
        return 1;
    }

    BUFFER *sql = buffer_create(1024);
    buffer_sprintf(sql, "SELECT type, family, context, title, priority, plugin, module, unit, chart_type " \
    "FROM chart_hash WHERE hash_id = @hash_id;");

    rc = sqlite3_prepare_v2(db_meta, buffer_tostring(sql), -1, &res, 0);
    if (rc != SQLITE_OK) {
        error_report("Failed to prepare statement when trying to fetch a chart hash configuration");
        goto fail;
    }

    rc = sqlite3_bind_blob(res, 1, &hash_uuid , sizeof(hash_uuid), SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    struct chart_config_updated chart_config;
    chart_config.config_hash  = NULL;

    while (sqlite3_step_monitored(res) == SQLITE_ROW) {
        chart_config.type = strdupz((char *)sqlite3_column_text(res, 0));
        chart_config.family = strdupz((char *)sqlite3_column_text(res, 1));
        chart_config.context = strdupz((char *)sqlite3_column_text(res, 2));
        chart_config.title = strdupz((char *)sqlite3_column_text(res, 3));
        chart_config.priority = sqlite3_column_int64(res, 4);
        chart_config.plugin = strdupz((char *)sqlite3_column_text(res, 5));
        chart_config.module = sqlite3_column_bytes(res, 6) > 0 ? strdupz((char *)sqlite3_column_text(res, 6)) : NULL;
        chart_config.chart_type = (RRDSET_TYPE) sqlite3_column_int(res,8);
        chart_config.units = strdupz((char *)sqlite3_column_text(res, 7));
        chart_config.config_hash = strdupz(hash_id);
    }

    if (likely(chart_config.config_hash)) {
        log_access(
            "ACLK REQ [%s (%s)]: Sending chart config for %s.",
            wc->node_id,
            wc->host ? rrdhost_hostname(wc->host) : "N/A",
            hash_id);
        aclk_chart_config_updated(&chart_config, 1);
        destroy_chart_config_updated(&chart_config);
    } else
        log_access(
            "ACLK STA [%s (%s)]: Chart config for %s not found.",
            wc->node_id,
            wc->host ? rrdhost_hostname(wc->host) : "N/A",
            hash_id);

bind_fail:
    rc = sqlite3_finalize(res);
    if (unlikely(rc != SQLITE_OK))
        error_report("Failed to reset statement when pushing chart config hash, rc = %d", rc);
fail:
    freez((char *)cmd.data_param);
    buffer_free(sql);
    return rc;
}

void aclk_receive_chart_ack(struct aclk_database_worker_config *wc, struct aclk_database_cmd cmd)
{
    int rc;
    sqlite3_stmt *res = NULL;

    char sql[ACLK_SYNC_QUERY_SIZE];

    snprintfz(sql,ACLK_SYNC_QUERY_SIZE-1,"UPDATE aclk_chart_%s SET date_updated=unixepoch() WHERE sequence_id <= @sequence_id "
            "AND date_submitted IS NOT NULL AND date_updated IS NULL;", wc->uuid_str);

    rc = sqlite3_prepare_v2(db_meta, sql, -1, &res, 0);
    if (rc != SQLITE_OK) {
        error_report("Failed to prepare statement to ack chart sequence ids");
        return;
    }

    rc = sqlite3_bind_int64(res, 1, (uint64_t) cmd.param1);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    rc = execute_insert(res);
    if (rc != SQLITE_DONE)
        error_report("Failed to ACK sequence id, rc = %d", rc);
    else
        log_access(
            "ACLK STA [%s (%s)]: CHARTS ACKNOWLEDGED IN THE DATABASE UP TO %" PRIu64,
            wc->node_id,
            wc->host ? rrdhost_hostname(wc->host) : "N/A",
            cmd.param1);

bind_fail:
    if (unlikely(sqlite3_finalize(res) != SQLITE_OK))
        error_report("Failed to finalize statement to ACK older sequence ids, rc = %d", rc);
    return;
}

void aclk_receive_chart_reset(struct aclk_database_worker_config *wc, struct aclk_database_cmd cmd)
{
    BUFFER *sql = buffer_create(1024);
    buffer_sprintf(
        sql,
        "UPDATE aclk_chart_%s SET status = NULL, date_submitted = NULL WHERE sequence_id >= %" PRIu64 ";",
        wc->uuid_str,
        cmd.param1);
    db_execute(buffer_tostring(sql));
    if (cmd.param1 == 1) {
        buffer_flush(sql);
        log_access("ACLK REQ [%s (%s)]: Received chart full resync.", wc->node_id, wc->hostname ? wc->hostname: "N/A");
        buffer_sprintf(sql, "DELETE FROM aclk_chart_payload_%s; DELETE FROM aclk_chart_%s; " \
                            "DELETE FROM aclk_chart_latest_%s;", wc->uuid_str, wc->uuid_str, wc->uuid_str);
        db_lock();

        db_execute("BEGIN TRANSACTION;");
        db_execute(buffer_tostring(sql));
        db_execute("COMMIT TRANSACTION;");

        db_unlock();
        wc->chart_sequence_id = 0;
        wc->chart_timestamp = 0;
        wc->chart_payload_count = 0;

        RRDHOST *host = wc->host;
        if (likely(host)) {
            rrdhost_rdlock(host);
            RRDSET *st;
            rrdset_foreach_read(st, host)
            {
                rrdset_rdlock(st);
                rrdset_flag_clear(st, RRDSET_FLAG_ACLK);
                RRDDIM *rd;
                rrddim_foreach_read(rd, st)
                {
                    rrddim_flag_clear(rd, RRDDIM_FLAG_ACLK);
                    rd->aclk_live_status = (rd->aclk_live_status == 0);
                }
                rrdset_unlock(st);
            }
            rrdhost_unlock(host);
        } else
            error_report("ACLK synchronization thread for %s is not linked to HOST", wc->host_guid);
    } else {
        log_access(
            "ACLK STA [%s (%s)]: RESTARTING CHART SYNC FROM SEQUENCE %" PRIu64,
            wc->node_id,
            wc->hostname ? wc->hostname : "N/A",
            cmd.param1);
        wc->chart_payload_count = sql_get_pending_count(wc);
        sql_get_last_chart_sequence(wc);
    }
    buffer_free(sql);
    wc->chart_updates = 1;
    return;
}

//
// Functions called directly from ACLK threads and will queue commands
//
void aclk_get_chart_config(char **hash_id)
{
    struct aclk_database_worker_config *wc = (struct aclk_database_worker_config *)localhost->dbsync_worker;

    if (unlikely(!wc || !hash_id))
        return;

    struct aclk_database_cmd cmd;
    memset(&cmd, 0, sizeof(cmd));
    cmd.opcode = ACLK_DATABASE_PUSH_CHART_CONFIG;
    for (int i = 0; hash_id[i]; ++i) {
        // TODO: Verify that we have a valid hash_id
        log_access(
            "ACLK REQ [%s (%s)]: Request %d for chart config with hash %s received.",
            wc->node_id,
            wc->host ? rrdhost_hostname(wc->host) : "N/A",
            i,
            hash_id[i]);
        cmd.data_param = (void *)strdupz(hash_id[i]);
        aclk_database_enq_cmd(wc, &cmd);
    }
    return;
}

// Send a command to a node_id
// Need to discover the thread that will handle the request
// if thread not in active hosts, then try to find in the queue
static void aclk_submit_param_command(char *node_id, enum aclk_database_opcode aclk_command, uint64_t param)
{
    if (unlikely(!node_id))
        return;

    struct aclk_database_worker_config *wc = NULL;
    struct aclk_database_cmd cmd;
    memset(&cmd, 0, sizeof(cmd));
    cmd.opcode = aclk_command;
    cmd.param1 = param;

    rrd_rdlock();
    RRDHOST *host = find_host_by_node_id(node_id);
    if (likely(host))
        wc = (struct aclk_database_worker_config *)host->dbsync_worker;
    rrd_unlock();
    if (wc)
        aclk_database_enq_cmd(wc, &cmd);
    else {
        if (aclk_worker_enq_cmd(node_id, &cmd))
            log_access("ACLK STA [%s (N/A)]: ACLK synchronization thread is not active.", node_id);
    }
    return;
}

void aclk_ack_chart_sequence_id(char *node_id, uint64_t last_sequence_id)
{
    if (unlikely(!node_id))
        return;

    char *hostname = get_hostname_by_node_id(node_id);
    log_access("ACLK REQ [%s (%s)]: CHARTS ACKNOWLEDGED upto %" PRIu64, node_id, hostname ? hostname : "N/A",
               last_sequence_id);
    freez(hostname);
    aclk_submit_param_command(node_id, ACLK_DATABASE_CHART_ACK, last_sequence_id);
    return;
}

// Start streaming charts / dimensions for node_id
void aclk_start_streaming(char *node_id, uint64_t sequence_id, time_t created_at, uint64_t batch_id)
{
    UNUSED(created_at);
    if (unlikely(!node_id))
        return;

    uuid_t node_uuid;
    if (uuid_parse(node_id, node_uuid)) {
        log_access("ACLK REQ [%s (N/A)]: CHARTS STREAM ignored, invalid node id", node_id);
        return;
    }

    struct aclk_database_worker_config *wc  = find_inactive_wc_by_node_id(node_id);
    rrd_rdlock();
    RRDHOST *host = localhost;
    while(host) {
        if (wc || (host->node_id && !(uuid_compare(*host->node_id, node_uuid)))) {
            rrd_unlock();
            if (!wc)
                wc = (struct aclk_database_worker_config *)host->dbsync_worker ?
                         (struct aclk_database_worker_config *)host->dbsync_worker :
                         (struct aclk_database_worker_config *)find_inactive_wc_by_node_id(node_id);
            if (likely(wc)) {
                wc->chart_reset_count++;
                __sync_synchronize();
                wc->chart_updates = 0;
                wc->batch_id = batch_id;
                __sync_synchronize();
                wc->batch_created = now_realtime_sec();
                log_access(
                    "ACLK REQ [%s (%s)]: CHARTS STREAM from %"PRIu64" (LOCAL %"PRIu64") t=%ld resets=%d" ,
                    wc->node_id,
                    wc->hostname ? wc->hostname : "N/A",
                    sequence_id + 1,
                    wc->chart_sequence_id,
                    wc->chart_timestamp,
                    wc->chart_reset_count);
                if (sequence_id > wc->chart_sequence_id || wc->chart_reset_count > 10) {
                    log_access(
                        "ACLK RES [%s (%s)]: CHARTS FULL RESYNC REQUEST "
                        "remote_seq=%" PRIu64 " local_seq=%" PRIu64 " resets=%d ",
                        wc->node_id,
                        wc->hostname ? wc->hostname : "N/A",
                        sequence_id,
                        wc->chart_sequence_id,
                        wc->chart_reset_count);

                    chart_reset_t chart_reset;
                    chart_reset.claim_id = get_agent_claimid();
                    if (chart_reset.claim_id) {
                        chart_reset.node_id = node_id;
                        chart_reset.reason = SEQ_ID_NOT_EXISTS;
                        aclk_chart_reset(chart_reset);
                        freez(chart_reset.claim_id);
                        wc->chart_reset_count = -1;
                    }
                } else {
                    struct aclk_database_cmd cmd;
                    memset(&cmd, 0, sizeof(cmd));
                    // TODO: handle timestamp
                    if (sequence_id < wc->chart_sequence_id ||
                        !sequence_id) { // || created_at != wc->chart_timestamp) {
                        log_access(
                            "ACLK REQ [%s (%s)]: CHART RESET from %" PRIu64 " t=%ld batch=%" PRIu64,
                            wc->node_id,
                            wc->hostname ? wc->hostname : "N/A",
                            sequence_id + 1,
                            wc->chart_timestamp,
                            wc->batch_id);
                        cmd.opcode = ACLK_DATABASE_RESET_CHART;
                        cmd.param1 = sequence_id + 1;
                        cmd.completion = NULL;
                        aclk_database_enq_cmd(wc, &cmd);
                    } else {
                        wc->chart_reset_count = 0;
                        wc->chart_updates = 1;
                    }
                }
            } else {
                log_access("ACLK STA [%s (%s)]: ACLK synchronization thread is not active.", node_id, wc->hostname ? wc->hostname : "N/A");
            }
            return;
        }
        host = host->next;
    }
    rrd_unlock();
    return;
}

#define SQL_SELECT_HOST_MEMORY_MODE "SELECT memory_mode FROM chart WHERE host_id = @host_id LIMIT 1;"

static RRD_MEMORY_MODE sql_get_host_memory_mode(uuid_t *host_id)
{
    int rc;

    RRD_MEMORY_MODE memory_mode = RRD_MEMORY_MODE_RAM;
    sqlite3_stmt *res = NULL;

    rc = sqlite3_prepare_v2(db_meta, SQL_SELECT_HOST_MEMORY_MODE, -1, &res, 0);

    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to prepare statement to read host memory mode");
        return memory_mode;
    }

    rc = sqlite3_bind_blob(res, 1, host_id, sizeof(*host_id), SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to bind host parameter to fetch host memory mode");
        goto failed;
    }

    while (sqlite3_step_monitored(res) == SQLITE_ROW) {
        memory_mode = (RRD_MEMORY_MODE)sqlite3_column_int(res, 0);
    }

failed:
    rc = sqlite3_finalize(res);
    if (unlikely(rc != SQLITE_OK))
        error_report("Failed to finalize the prepared statement when reading host memory mode");
    return memory_mode;
}

#define SELECT_HOST_DIMENSION_LIST                                                                                     \
    "SELECT d.dim_id, c.update_every, c.type||'.'||c.id, d.id, d.name FROM chart c, dimension d "                      \
    "WHERE d.chart_id = c.chart_id AND c.host_id = @host_id ORDER BY c.update_every ASC;"

#define SELECT_HOST_CHART_LIST                                                                                         \
    "SELECT distinct h.host_id, c.update_every, c.type||'.'||c.id FROM chart c, host h "                               \
    "WHERE c.host_id = h.host_id AND c.host_id = @host_id ORDER BY c.update_every ASC;"

void aclk_update_retention(struct aclk_database_worker_config *wc)
{
    int rc;

    if (!aclk_connected)
        return;

    if (wc->host && rrdhost_flag_check(wc->host, RRDHOST_FLAG_ACLK_STREAM_CONTEXTS)) {
        internal_error(true, "Skipping aclk_update_retention for host %s because context streaming is enabled", rrdhost_hostname(wc->host));
        return;
    }

    char *claim_id = get_agent_claimid();
    if (unlikely(!claim_id))
        return;

    sqlite3_stmt *res = NULL;
    RRD_MEMORY_MODE memory_mode;

    uuid_t host_uuid;
    rc = uuid_parse(wc->host_guid, host_uuid);
    if (unlikely(rc)) {
        freez(claim_id);
        return;
    }

    if (wc->host)
        memory_mode = wc->host->rrd_memory_mode;
    else
        memory_mode = sql_get_host_memory_mode(&host_uuid);

    if (memory_mode == RRD_MEMORY_MODE_DBENGINE)
        rc = sqlite3_prepare_v2(db_meta, SELECT_HOST_DIMENSION_LIST, -1, &res, 0);
    else
        rc = sqlite3_prepare_v2(db_meta, SELECT_HOST_CHART_LIST, -1, &res, 0);

    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to prepare statement to fetch host dimensions");
        freez(claim_id);
        return;
    }

    rc = sqlite3_bind_blob(res, 1, &host_uuid, sizeof(host_uuid), SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to bind host parameter to fetch host dimensions");
        goto failed;
    }

    time_t start_time = LONG_MAX;
    time_t first_entry_t;
    time_t last_entry_t;
    uint32_t update_every = 0;
    uint32_t dimension_update_count = 0;
    uint32_t total_checked = 0;
    uint32_t total_deleted= 0;
    uint32_t total_stopped= 0;
    time_t send_status;

    struct retention_updated rotate_data;

    memset(&rotate_data, 0, sizeof(rotate_data));

    int max_intervals = 32;

    rotate_data.interval_duration_count = 0;
    rotate_data.interval_durations = callocz(max_intervals, sizeof(*rotate_data.interval_durations));

    now_realtime_timeval(&rotate_data.rotation_timestamp);
    rotate_data.memory_mode = memory_mode;
    rotate_data.claim_id = claim_id;
    rotate_data.node_id = strdupz(wc->node_id);

    time_t now = now_realtime_sec();
    while (sqlite3_step_monitored(res) == SQLITE_ROW && dimension_update_count < ACLK_MAX_DIMENSION_CLEANUP) {
        if (unlikely(netdata_exit))
            break;
        if (!update_every || update_every != (uint32_t)sqlite3_column_int(res, 1)) {
            if (update_every) {
                debug(D_ACLK_SYNC, "Update %s for %u oldest time = %ld", wc->host_guid, update_every, start_time);
                if (start_time == LONG_MAX)
                    rotate_data.interval_durations[rotate_data.interval_duration_count].retention = 0;
                else
                    rotate_data.interval_durations[rotate_data.interval_duration_count].retention =
                        rotate_data.rotation_timestamp.tv_sec - start_time;
                rotate_data.interval_duration_count++;
            }
            update_every = (uint32_t)sqlite3_column_int(res, 1);
            rotate_data.interval_durations[rotate_data.interval_duration_count].update_every = update_every;
            start_time = LONG_MAX;
        }
#ifdef ENABLE_DBENGINE
        if (memory_mode == RRD_MEMORY_MODE_DBENGINE)
            rc =
                rrdeng_metric_latest_time_by_uuid((uuid_t *)sqlite3_column_blob(res, 0), &first_entry_t, &last_entry_t, 0);
        else
#endif
        {
            if (wc->host) {
                RRDSET *st = NULL;
                rc = (st = rrdset_find(wc->host, (const char *)sqlite3_column_text(res, 2))) ? 0 : 1;
                if (!rc) {
                    first_entry_t = rrdset_first_entry_t(st);
                    last_entry_t = rrdset_last_entry_t(st);
                }
            } else {
                rc = 0;
                first_entry_t = rotate_data.rotation_timestamp.tv_sec;
            }
        }

        if (likely(!rc && first_entry_t))
            start_time = MIN(start_time, first_entry_t);

        if (memory_mode == RRD_MEMORY_MODE_DBENGINE && wc->chart_updates && (dimension_update_count < ACLK_MAX_DIMENSION_CLEANUP)) {
            int live = ((now - last_entry_t) < (RRDSET_MINIMUM_DIM_LIVE_MULTIPLIER * update_every));
            if (rc) {
                first_entry_t = 0;
                last_entry_t = 0;
                live = 0;
            }
            if (!wc->host || !first_entry_t) {
                if (!first_entry_t) {
                    delete_dimension_uuid((uuid_t *)sqlite3_column_blob(res, 0));
                    total_deleted++;
                    dimension_update_count++;
                }
                else {
                    (void)aclk_upd_dimension_event(
                        wc,
                        claim_id,
                        (uuid_t *)sqlite3_column_blob(res, 0),
                        (const char *)(const char *)sqlite3_column_text(res, 3),
                        (const char *)(const char *)sqlite3_column_text(res, 4),
                        (const char *)(const char *)sqlite3_column_text(res, 2),
                        first_entry_t,
                        live ? 0 : last_entry_t,
                        &send_status);

                    if (!send_status) {
                        if (last_entry_t)
                            total_stopped++;
                        dimension_update_count++;
                    }
                }
            }
        }
        total_checked++;
    }
    if (update_every) {
        debug(D_ACLK_SYNC, "Update %s for %u oldest time = %ld", wc->host_guid, update_every, start_time);
        if (start_time == LONG_MAX)
            rotate_data.interval_durations[rotate_data.interval_duration_count].retention = 0;
        else
            rotate_data.interval_durations[rotate_data.interval_duration_count].retention =
                rotate_data.rotation_timestamp.tv_sec - start_time;
        rotate_data.interval_duration_count++;
    }

    if (dimension_update_count < ACLK_MAX_DIMENSION_CLEANUP && !netdata_exit)
        log_access("ACLK STA [%s (%s)]: UPDATES %d RETENTION MESSAGE SENT. CHECKED %u DIMENSIONS.  %u DELETED, %u STOPPED COLLECTING",
                   wc->node_id, wc->hostname ? wc->hostname : "N/A", wc->chart_updates, total_checked, total_deleted, total_stopped);
    else
        log_access("ACLK STA [%s (%s)]: UPDATES %d RETENTION MESSAGE NOT SENT. CHECKED %u DIMENSIONS.  %u DELETED, %u STOPPED COLLECTING",
                   wc->node_id, wc->hostname ? wc->hostname : "N/A", wc->chart_updates, total_checked, total_deleted, total_stopped);

#ifdef NETDATA_INTERNAL_CHECKS
    info("Retention update for %s (chart updates = %d)", wc->host_guid, wc->chart_updates);
    for (int i = 0; i < rotate_data.interval_duration_count; ++i)
        info(
            "Update for host %s (node %s) for %u Retention = %u",
            wc->host_guid,
            wc->node_id,
            rotate_data.interval_durations[i].update_every,
            rotate_data.interval_durations[i].retention);
#endif
    if (dimension_update_count < ACLK_MAX_DIMENSION_CLEANUP && !netdata_exit)
        aclk_retention_updated(&rotate_data);
    freez(rotate_data.node_id);
    freez(rotate_data.interval_durations);

failed:
    freez(claim_id);
    rc = sqlite3_finalize(res);
    if (unlikely(rc != SQLITE_OK))
        error_report("Failed to finalize the prepared statement when reading host dimensions");
    return;
}

uint32_t sql_get_pending_count(struct aclk_database_worker_config *wc)
{
    char sql[ACLK_SYNC_QUERY_SIZE];
    static __thread sqlite3_stmt *res = NULL;

    snprintfz(sql,ACLK_SYNC_QUERY_SIZE-1, "SELECT count(1) FROM aclk_chart_%s ac WHERE ac.date_submitted IS NULL;", wc->uuid_str);

    int rc;
    uint32_t chart_payload_count = 0;
    if (unlikely(!res)) {
        rc = prepare_statement(db_meta, sql, &res);
        if (rc != SQLITE_OK) {
            error_report("Failed to prepare statement to count pending messages");
            return 0;
        }
    }
    while (sqlite3_step_monitored(res) == SQLITE_ROW)
        chart_payload_count = (uint32_t) sqlite3_column_int(res, 0);

    rc = sqlite3_reset(res);
    if (unlikely(rc != SQLITE_OK))
        error_report("Failed to reset statement when fetching pending messages, rc = %d", rc);

    return chart_payload_count;
}

void sql_get_last_chart_sequence(struct aclk_database_worker_config *wc)
{
    char sql[ACLK_SYNC_QUERY_SIZE];

    snprintfz(sql,ACLK_SYNC_QUERY_SIZE-1, "SELECT ac.sequence_id, ac.date_created FROM aclk_chart_%s ac " \
        "WHERE ac.date_submitted IS NOT NULL ORDER BY ac.sequence_id DESC LIMIT 1;", wc->uuid_str);

    int rc;
    sqlite3_stmt *res = NULL;
    rc = sqlite3_prepare_v2(db_meta, sql, -1, &res, 0);
    if (rc != SQLITE_OK) {
        error_report("Failed to prepare statement to find last chart sequence id");
        return;
    }

    wc->chart_sequence_id = 0;
    wc->chart_timestamp = 0;
    while (sqlite3_step_monitored(res) == SQLITE_ROW) {
        wc->chart_sequence_id = (uint64_t)sqlite3_column_int64(res, 0);
        wc->chart_timestamp = (time_t)sqlite3_column_int64(res, 1);
    }

    debug(D_ACLK_SYNC, "Node %s reports last sequence_id=%" PRIu64, wc->node_id, wc->chart_sequence_id);

    rc = sqlite3_finalize(res);
    if (unlikely(rc != SQLITE_OK))
        error_report("Failed to reset statement when fetching chart sequence info, rc = %d", rc);

    return;
}

void queue_dimension_to_aclk(RRDDIM *rd, time_t last_updated)
{
    RRDHOST *host = rd->rrdset->rrdhost;
    if (likely(rrdhost_flag_check(host, RRDHOST_FLAG_ACLK_STREAM_CONTEXTS)))
        return;

    int live = !last_updated;

    if (likely(rd->aclk_live_status == live))
        return;

    time_t created_at = rd->tiers[0]->query_ops.oldest_time(rd->tiers[0]->db_metric_handle);

    if (unlikely(!created_at && rd->updated))
       created_at = rd->last_collected_time.tv_sec;

    rd->aclk_live_status = live;

    struct aclk_database_worker_config *wc = rd->rrdset->rrdhost->dbsync_worker;
    if (unlikely(!wc))
        return;

    char *claim_id = get_agent_claimid();
    if (unlikely(!claim_id))
        return;

    struct chart_dimension_updated dim_payload;
    memset(&dim_payload, 0, sizeof(dim_payload));
    dim_payload.node_id = wc->node_id;
    dim_payload.claim_id = claim_id;
    dim_payload.name = rrddim_name(rd);
    dim_payload.id = rrddim_id(rd);
    dim_payload.chart_id = rrdset_id(rd->rrdset);
    dim_payload.created_at.tv_sec = created_at;
    dim_payload.last_timestamp.tv_sec = last_updated;

    size_t size = 0;
    char *payload = generate_chart_dimension_updated(&size, &dim_payload);

    freez(claim_id);
    if (unlikely(!payload))
        return;

    struct aclk_chart_dimension_data *aclk_cd_data = mallocz(sizeof(*aclk_cd_data));
    uuid_copy(aclk_cd_data->uuid, rd->metric_uuid);
    aclk_cd_data->payload = payload;
    aclk_cd_data->payload_size = size;
    aclk_cd_data->check_payload = 1;

    struct aclk_database_cmd cmd;
    memset(&cmd, 0, sizeof(cmd));

    cmd.opcode = ACLK_DATABASE_ADD_DIMENSION;
    cmd.data = aclk_cd_data;
    int rc = aclk_database_enq_cmd_noblock(wc, &cmd);

    if (unlikely(rc)) {
        freez(aclk_cd_data->payload);
        freez(aclk_cd_data);
        rd->aclk_live_status = !live;
    }
    return;
}

void aclk_send_dimension_update(RRDDIM *rd)
{
    char *claim_id = get_agent_claimid();
    if (unlikely(!claim_id))
        return;

    time_t first_entry_t = rrddim_first_entry_t(rd);
    time_t last_entry_t = rrddim_last_entry_t(rd);

    time_t now = now_realtime_sec();
    int live = ((now - rd->last_collected_time.tv_sec) < (RRDSET_MINIMUM_DIM_LIVE_MULTIPLIER * rd->update_every));

    if (!live || rd->aclk_live_status != live || !first_entry_t) {
        (void)aclk_upd_dimension_event(
            rd->rrdset->rrdhost->dbsync_worker,
            claim_id,
            &rd->metric_uuid,
            rrddim_id(rd),
            rrddim_name(rd),
            rrdset_id(rd->rrdset),
            first_entry_t,
            live ? 0 : last_entry_t,
            NULL);

        if (!first_entry_t)
            debug(
                D_ACLK_SYNC,
                "%s: Update dimension chart=%s dim=%s live=%d (%ld, %ld)",
                rrdhost_hostname(rd->rrdset->rrdhost),
                rrdset_name(rd->rrdset),
                rrddim_name(rd),
                live,
                first_entry_t,
                last_entry_t);
        else
            debug(
                D_ACLK_SYNC,
                "%s: Update dimension chart=%s dim=%s live=%d (%ld, %ld) collected %ld seconds ago",
                rrdhost_hostname(rd->rrdset->rrdhost),
                rrdset_name(rd->rrdset),
                rrddim_name(rd),
                live,
                first_entry_t,
                last_entry_t,
                now - last_entry_t);
        rd->aclk_live_status = live;
    }

    freez(claim_id);
    return;
}

#define SQL_SEQ_NULL(result, n)  sqlite3_column_type(result, n) == SQLITE_NULL ? 0 : sqlite3_column_int64(result, n)

struct aclk_chart_sync_stats *aclk_get_chart_sync_stats(RRDHOST *host)
{
    struct aclk_chart_sync_stats *aclk_statistics = NULL;

    struct aclk_database_worker_config *wc  = NULL;
    wc = (struct aclk_database_worker_config *)host->dbsync_worker;
    if (!wc)
        return NULL;

    aclk_statistics = callocz(1, sizeof(struct aclk_chart_sync_stats));

    aclk_statistics->updates = wc->chart_updates;
    aclk_statistics->batch_id = wc->batch_id;

    char host_uuid_fixed[GUID_LEN + 1];

    strncpy(host_uuid_fixed, host->machine_guid, GUID_LEN);
    host_uuid_fixed[GUID_LEN] = 0;

    host_uuid_fixed[8] = '_';
    host_uuid_fixed[13] = '_';
    host_uuid_fixed[18] = '_';
    host_uuid_fixed[23] = '_';

    sqlite3_stmt *res = NULL;
    BUFFER *sql = buffer_create(1024);
    buffer_sprintf(sql, "SELECT min(sequence_id), max(sequence_id), 0 FROM aclk_chart_%s;", host_uuid_fixed);
    buffer_sprintf(sql, "SELECT min(sequence_id), max(sequence_id), 0 FROM aclk_chart_%s WHERE date_submitted IS NULL;", host_uuid_fixed);
    buffer_sprintf(sql, "SELECT min(sequence_id), max(sequence_id), 0 FROM aclk_chart_%s WHERE date_submitted IS NOT NULL;", host_uuid_fixed);
    buffer_sprintf(sql, "SELECT min(sequence_id), max(sequence_id), 0 FROM aclk_chart_%s WHERE date_updated IS NOT NULL;", host_uuid_fixed);
    buffer_sprintf(sql, "SELECT max(date_created), max(date_submitted), max(date_updated), 0 FROM aclk_chart_%s;", host_uuid_fixed);

    int rc = sqlite3_prepare_v2(db_meta, buffer_tostring(sql), -1, &res, 0);
    if (rc != SQLITE_OK) {
        buffer_free(sql);
        freez(aclk_statistics);
        return NULL;
    }

    rc = sqlite3_step_monitored(res);
    if (rc == SQLITE_ROW) {
        aclk_statistics->min_seqid = SQL_SEQ_NULL(res, 0);
        aclk_statistics->max_seqid = SQL_SEQ_NULL(res, 1);
    }

    rc = sqlite3_step_monitored(res);
    if (rc == SQLITE_ROW) {
        aclk_statistics->min_seqid_pend = SQL_SEQ_NULL(res, 0);
        aclk_statistics->max_seqid_pend = SQL_SEQ_NULL(res, 1);
    }

    rc = sqlite3_step_monitored(res);
    if (rc == SQLITE_ROW) {
        aclk_statistics->min_seqid_sent = SQL_SEQ_NULL(res, 0);
        aclk_statistics->max_seqid_sent = SQL_SEQ_NULL(res, 1);
    }

    rc = sqlite3_step_monitored(res);
    if (rc == SQLITE_ROW) {
        aclk_statistics->min_seqid_ack = SQL_SEQ_NULL(res, 0);
        aclk_statistics->max_seqid_ack = SQL_SEQ_NULL(res, 1);
    }

    rc = sqlite3_step_monitored(res);
    if (rc == SQLITE_ROW) {
        aclk_statistics->min_seqid_ack = SQL_SEQ_NULL(res, 0);
        aclk_statistics->max_seqid_ack = SQL_SEQ_NULL(res, 1);
    }

    rc = sqlite3_step_monitored(res);
    if (rc == SQLITE_ROW) {
        aclk_statistics->max_date_created = (time_t) SQL_SEQ_NULL(res, 0);
        aclk_statistics->max_date_submitted = (time_t) SQL_SEQ_NULL(res, 1);
        aclk_statistics->max_date_ack = (time_t) SQL_SEQ_NULL(res, 2);
    }

    rc = sqlite3_finalize(res);
    if (unlikely(rc != SQLITE_OK))
        error_report("Failed to finalize statement when fetching aclk sync statistics, rc = %d", rc);

    buffer_free(sql);
    return aclk_statistics;
}

void sql_check_chart_liveness(RRDSET *st) {
    RRDDIM *rd;

    if (unlikely(st->state->is_ar_chart))
        return;

    rrdset_rdlock(st);

    if (unlikely(!rrdset_flag_check(st, RRDSET_FLAG_ACLK))) {
        rrdset_unlock(st);
        return;
    }

    if (unlikely(!rrdset_flag_check(st, RRDSET_FLAG_ACLK))) {
        if (likely(st->dimensions && st->counter_done && !queue_chart_to_aclk(st))) {
            debug(D_ACLK_SYNC,"Check chart liveness [%s] submit chart definition", rrdset_name(st));
            rrdset_flag_set(st, RRDSET_FLAG_ACLK);
        }
    }
    else
        debug(D_ACLK_SYNC,"Check chart liveness [%s] chart definition already submitted", rrdset_name(st));
    time_t mark = now_realtime_sec();

    debug(D_ACLK_SYNC,"Check chart liveness [%s] scanning dimensions", rrdset_name(st));
    rrddim_foreach_read(rd, st) {
        if (!rrddim_flag_check(rd, RRDDIM_FLAG_HIDDEN))
            queue_dimension_to_aclk(rd, calc_dimension_liveness(rd, mark));
    }
    rrdset_unlock(st);
}

// ST is read locked
int queue_chart_to_aclk(RRDSET *st)
{
    RRDHOST *host = st->rrdhost;

    if (likely(rrdhost_flag_check(host, RRDHOST_FLAG_ACLK_STREAM_CONTEXTS)))
        return 0;

    return sql_queue_chart_payload((struct aclk_database_worker_config *) st->rrdhost->dbsync_worker,
                                       st, ACLK_DATABASE_ADD_CHART);
}

#endif //ENABLE_ACLK
