// SPDX-License-Identifier: GPL-3.0-or-later

#include "sqlite_functions.h"
#include "sqlite_aclk_chart.h"

#include "../../aclk/aclk_charts_api.h"

#define CHECK_SQLITE_CONNECTION(db_meta)                                                                               \
    if (unlikely(!db_meta)) {                                                                                          \
        if (default_rrd_memory_mode != RRD_MEMORY_MODE_DBENGINE) {                                                     \
            return 1;                                                                                                  \
        }                                                                                                              \
        error_report("Database has not been initialized");                                                             \
        return 1;                                                                                                      \
    }

static inline int sql_queue_chart_payload(struct aclk_database_worker_config *wc,
                                          void *data, enum aclk_database_opcode opcode)
{
    if (unlikely(!wc))
        return 1;

    struct aclk_database_cmd cmd;
    memset(&cmd, 0, sizeof(cmd));
    cmd.opcode = opcode;
    cmd.data = data;
    aclk_database_enq_cmd(wc, &cmd);
    return 0;
}

static int payload_sent(char *uuid_str, uuid_t *uuid, void *payload, size_t payload_size)
{
    static __thread sqlite3_stmt *res = NULL;
    int rc;
    int send_status = 0;

    if (unlikely(!res)) {
        BUFFER *sql = buffer_create(1024);
        buffer_sprintf(sql,"SELECT 1 FROM aclk_chart_latest_%s acl, aclk_chart_payload_%s acp "
                            "WHERE acl.unique_id = acp.unique_id AND acl.uuid = @uuid AND acp.payload = @payload;",
                       uuid_str, uuid_str);
        rc = prepare_statement(db_meta, (char *) buffer_tostring(sql), &res);
        buffer_free(sql);
        if (rc != SQLITE_OK) {
            error_report("Failed to prepare statement to check payload data");
            return 0;
        }
    }

    rc = sqlite3_bind_blob(res, 1, uuid , sizeof(*uuid), SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    rc = sqlite3_bind_blob(res, 2, payload , payload_size, SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    while (sqlite3_step(res) == SQLITE_ROW) {
        send_status = sqlite3_column_int(res, 0);
    }

bind_fail:
    if (unlikely(sqlite3_reset(res) != SQLITE_OK))
        error_report("Failed to reset statement in check payload, rc = %d", rc);
    return send_status;
}

static int aclk_add_chart_payload(char *uuid_str, uuid_t *uuid, char *claim_id, ACLK_PAYLOAD_TYPE payload_type,
                           void *payload, size_t payload_size)
{
    static __thread sqlite3_stmt *res_chart = NULL;
    int rc;

    rc = payload_sent(uuid_str, uuid, payload, payload_size);
    if (rc == 1)
        return 0;

    if (unlikely(!res_chart)) {
        BUFFER *sql = buffer_create(1024);

        buffer_sprintf(sql,"INSERT INTO aclk_chart_payload_%s (unique_id, uuid, claim_id, date_created, type, payload) " \
                            "VALUES (@unique_id, @uuid, @claim_id, strftime('%%s','now'), @type, @payload);", uuid_str);

        rc = prepare_statement(db_meta, (char *) buffer_tostring(sql), &res_chart);
        buffer_free(sql);

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

    rc = sqlite3_bind_blob(res_chart, 1, &unique_uuid , sizeof(unique_uuid), SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    rc = sqlite3_bind_blob(res_chart, 2, uuid , sizeof(*uuid), SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    rc = sqlite3_bind_blob(res_chart, 3, &claim_uuid , sizeof(claim_uuid), SQLITE_STATIC);
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

bind_fail:
    if (unlikely(sqlite3_reset(res_chart) != SQLITE_OK))
        error_report("Failed to reset statement in store chart payload, rc = %d", rc);
    return (rc != SQLITE_DONE);
}

int aclk_add_chart_event(struct aclk_database_worker_config *wc, struct aclk_database_cmd cmd)
{
    int rc = 0;
    CHECK_SQLITE_CONNECTION(db_meta);

#ifdef ACLK_NG
    char *claim_id = is_agent_claimed();

    RRDSET *st = cmd.data;

    if (likely(claim_id)) {
        struct chart_instance_updated chart_payload;
        memset(&chart_payload, 0, sizeof(chart_payload));
        chart_payload.config_hash = get_str_from_uuid(&st->state->hash_id);
        chart_payload.update_every = st->update_every;
        chart_payload.memory_mode = st->rrd_memory_mode;
        chart_payload.name = strdupz((char *)st->name);
        chart_payload.node_id = strdupz(wc->node_id);
        chart_payload.claim_id = claim_id;
        chart_payload.id = strdupz(st->id);

        struct label_index *labels = &st->state->labels;
        netdata_rwlock_wrlock(&labels->labels_rwlock);
        struct label *label_list = labels->head;
        struct label *chart_label = NULL;
        while (label_list) {
            chart_label = add_label_to_list(chart_label, label_list->key, label_list->value, label_list->label_source);
            label_list = label_list->next;
        }
        netdata_rwlock_unlock(&labels->labels_rwlock);
        chart_payload.label_head = chart_label;

        size_t size;
        char *payload = generate_chart_instance_updated(&size, &chart_payload);
        if (likely(payload))
            rc = aclk_add_chart_payload(wc->uuid_str, st->chart_uuid, claim_id, ACLK_PAYLOAD_CHART, (void *) payload, size);
        freez(payload);
        chart_instance_updated_destroy(&chart_payload);
    }
#else
    UNUSED(wc);
    UNUSED(cmd);
#endif
    return rc;
}

int aclk_add_dimension_event(struct aclk_database_worker_config *wc, struct aclk_database_cmd cmd)
{
    int rc = 0;
    CHECK_SQLITE_CONNECTION(db_meta);

#ifdef ACLK_NG
    char *claim_id = is_agent_claimed();

    RRDDIM *rd = cmd.data;

    if (likely(claim_id)) {
        time_t now = now_realtime_sec();

        time_t first_t = rd->state->query_ops.oldest_time(rd);
        time_t last_t  = rd->state->query_ops.latest_time(rd);

        int live = ((now - last_t) < (RRDSET_MINIMUM_LIVE_COUNT * rd->update_every));

        struct chart_dimension_updated dim_payload;
        size_t size;

        memset(&dim_payload, 0, sizeof(dim_payload));
        dim_payload.node_id = strdupz(wc->node_id);
        dim_payload.claim_id = claim_id;
        dim_payload.name = strdupz(rd->name);
        dim_payload.id = strdupz(rd->id);

        dim_payload.chart_id = strdupz(rd->rrdset->name);
        dim_payload.created_at.tv_sec = first_t;
        if (unlikely(!live))
            dim_payload.last_timestamp.tv_sec = last_t;

        char *payload = generate_chart_dimension_updated(&size, &dim_payload);
        if (likely(payload))
            rc = aclk_add_chart_payload(wc->uuid_str, &rd->state->metric_uuid, claim_id, ACLK_PAYLOAD_DIMENSION, (void *)payload, size);
        freez((char *)dim_payload.node_id);
        freez((char *)dim_payload.chart_id);
        freez((char *)dim_payload.name);
        freez((char *)dim_payload.id);
        freez(payload);
        freez(claim_id);
    }
    rrddim_flag_clear(rd, RRDDIM_FLAG_ACLK);
#else
    UNUSED(wc);
    UNUSED(cmd);
#endif
    return rc;
}

void aclk_send_chart_event(struct aclk_database_worker_config *wc, struct aclk_database_cmd cmd)
{
#ifdef ACLK_NG
    int rc;

    wc->chart_pending = 0;
    if (unlikely(!wc->chart_updates)) {
        debug(D_ACLK_SYNC,"Ignoring chart push event, updates have been turned off for node %s", wc->node_id);
        return;
    }

    char *claim_id = is_agent_claimed();
    if (unlikely(!claim_id))
        return;

    int limit = cmd.count > 0 ? cmd.count : 1;

    uint64_t first_sequence;
    uint64_t last_sequence;
    time_t last_timestamp;

    BUFFER *sql = buffer_create(1024);

    sqlite3_stmt *res = NULL;

    buffer_sprintf(sql, "SELECT ac.sequence_id, acp.payload, ac.date_created, ac.type, ac.uuid  " \
    "FROM aclk_chart_%s ac, aclk_chart_payload_%s acp " \
    "WHERE ac.date_submitted IS NULL AND ac.unique_id = acp.unique_id AND ac.update_count > 0 " \
    "AND acp.claim_id = @claim_id ORDER BY ac.sequence_id ASC LIMIT %d;", wc->uuid_str, wc->uuid_str, limit);

    rc = sqlite3_prepare_v2(db_meta, buffer_tostring(sql), -1, &res, 0);
    if (rc != SQLITE_OK) {
        error_report("Failed to prepare statement when trying to send a chart update via ACLK");
        buffer_free(sql);
        freez(claim_id);
        return;
    }

    rc = sqlite3_bind_text(res, 1, claim_id , -1, SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    char **payload_list = callocz(limit+1, sizeof(char *));
    size_t *payload_list_size = callocz(limit+1, sizeof(size_t));
    size_t *payload_list_max_size = callocz(limit+1, sizeof(size_t));
    struct aclk_message_position *position_list =  callocz(limit+1, sizeof(*position_list));
    int *is_dim = callocz(limit+1, sizeof(*is_dim));

    int loop = cmd.param1;

    while (loop > 0) {
        uint64_t previous_sequence_id = wc->chart_sequence_id;
        int count = 0;
        first_sequence = 0;
        last_sequence = 0;
        while (count < limit && sqlite3_step(res) == SQLITE_ROW) {
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
        }
        freez(payload_list[count]);
        payload_list_max_size[count] = 0;
        payload_list[count] = NULL;

        rc = sqlite3_reset(res);
        if (unlikely(rc != SQLITE_OK))
            error_report("Failed to reset statement when pushing chart events, rc = %d", rc);

        if (likely(first_sequence)) {
            buffer_flush(sql);

            db_lock();
            buffer_sprintf(sql, "UPDATE aclk_chart_%s SET status = NULL, date_submitted=strftime('%%s','now') "
                                "WHERE date_submitted IS NULL AND sequence_id BETWEEN %" PRIu64 " AND %" PRIu64 ";",
                                wc->uuid_str, first_sequence, last_sequence);
            db_execute(buffer_tostring(sql));

            buffer_flush(sql);
            buffer_sprintf(sql, "INSERT OR REPLACE INTO aclk_chart_latest_%s (uuid, unique_id, date_submitted) "
                                " SELECT uuid, unique_id, date_submitted FROM aclk_chart_%s s "
                                " WHERE date_submitted IS NOT NULL AND sequence_id BETWEEN %" PRIu64 " AND %" PRIu64
                                " ;",
                                wc->uuid_str, wc->uuid_str, first_sequence, last_sequence);
            db_execute(buffer_tostring(sql));
            db_unlock();

            aclk_chart_inst_and_dim_update(payload_list, payload_list_size, is_dim, position_list, wc->batch_id);
            wc->chart_sequence_id = last_sequence;
            wc->chart_timestamp = last_timestamp;
        }
        --loop;
    }

    for (int i = 0; i <= limit; ++i)
        freez(payload_list[i]);

    freez(payload_list);
    freez(payload_list_size);
    freez(payload_list_max_size);
    freez(position_list);
    freez(is_dim);

bind_fail:
    rc = sqlite3_finalize(res);
    if (unlikely(rc != SQLITE_OK))
        error_report("Failed to finalize statement when pushing chart events, rc = %d", rc);

    buffer_free(sql);
    freez(claim_id);
#else
    UNUSED(wc);
    UNUSED(cmd);
#endif
    return;
}


// Push one chart config to the cloud
int aclk_send_chart_config(struct aclk_database_worker_config *wc, struct aclk_database_cmd cmd)
{
    UNUSED(wc);
#ifdef ACLK_NG

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

    while (sqlite3_step(res) == SQLITE_ROW) {
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
        debug(D_ACLK_SYNC, "Sending chart config for %s", hash_id);
        aclk_chart_config_updated(&chart_config, 1);
        destroy_chart_config_updated(&chart_config);
    }
    else
        info("DEBUG: Chart config for %s not found", hash_id);

    bind_fail:
        rc = sqlite3_finalize(res);
        if (unlikely(rc != SQLITE_OK))
            error_report("Failed to reset statement when pushing chart config hash, rc = %d", rc);
        fail:
        freez((char *) cmd.data_param);
        buffer_free(sql);
        return rc;
#else
        UNUSED(cmd);
        return 0;
#endif
}

void aclk_receive_chart_ack(struct aclk_database_worker_config *wc, struct aclk_database_cmd cmd)
{
    int rc;
    sqlite3_stmt *res = NULL;

    BUFFER *sql = buffer_create(1024);

    buffer_sprintf(sql, "UPDATE aclk_chart_%s SET date_updated=strftime('%%s','now') WHERE sequence_id <= @sequence_id "
                        "AND date_submitted IS NOT NULL AND date_updated IS NULL;", wc->uuid_str);

    rc = sqlite3_prepare_v2(db_meta, buffer_tostring(sql), -1, &res, 0);
    if (rc != SQLITE_OK) {
        error_report("Failed to prepare statement count sequence ids in the database");
        goto prepare_fail;
    }

    rc = sqlite3_bind_int64(res, 1, (uint64_t) cmd.param1);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    rc = execute_insert(res);
    if (rc != SQLITE_DONE)
        error_report("Failed to ACK sequence id, rc = %d", rc);

    bind_fail:
        if (unlikely(sqlite3_finalize(res) != SQLITE_OK))
            error_report("Failed to finalize statement to ACK older sequence ids, rc = %d", rc);

        prepare_fail:
        buffer_free(sql);
        return;
}

void aclk_receive_chart_reset(struct aclk_database_worker_config *wc, struct aclk_database_cmd cmd)
{
    BUFFER *sql = buffer_create(1024);
    buffer_sprintf(sql, "UPDATE aclk_chart_%s SET status = NULL, date_submitted = NULL WHERE sequence_id >= %"PRIu64";",
                   wc->uuid_str, cmd.param1);
    db_execute(buffer_tostring(sql));
    if (cmd.param1 == 1) {
        db_lock();
        buffer_flush(sql);
        info("DEBUG: Deleting all data for %s", wc->uuid_str);
        buffer_sprintf(sql, "DELETE FROM aclk_chart_payload_%s; DELETE FROM aclk_chart_%s; DELETE FROM aclk_chart_latest_%s;",
                       wc->uuid_str, wc->uuid_str, wc->uuid_str);
        db_execute(buffer_tostring(sql));
        db_unlock();
        wc->chart_sequence_id = 0;
        wc->chart_timestamp = 0;

#ifdef ACLK_NG
        RRDHOST *host = wc->host;
        rrdhost_rdlock(host);
        RRDSET *st;
        rrdset_foreach_read(st, host) {
            rrdset_rdlock(st);
            RRDDIM *rd;
            rrddim_foreach_read(rd, st) {
                rd->state->aclk_live_status = (rd->state->aclk_live_status == 0);
            }
            rrdset_flag_clear(st, RRDSET_FLAG_ACLK);
            rrdset_unlock(st);
        }
        rrdhost_unlock(host);
#endif
    }
    else {
        //sql_chart_deduplicate(wc, cmd);
        sql_get_last_chart_sequence(wc, cmd);
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
        debug(D_ACLK_SYNC,"Request %d for chart config with hash [%s] received", i, hash_id[i]);
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

    struct aclk_database_worker_config *wc  = NULL;
    struct aclk_database_cmd cmd;
    memset(&cmd, 0, sizeof(cmd));
    cmd.opcode = aclk_command;
    cmd.param1 = param;

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

void aclk_ack_chart_sequence_id(char *node_id, uint64_t last_sequence_id)
{
    if (unlikely(!node_id))
        return;

    debug(D_ACLK_SYNC, "NODE %s reports last sequence id received %"PRIu64, node_id, last_sequence_id);
    aclk_submit_param_command(node_id, ACLK_DATABASE_CHART_ACK, last_sequence_id);
    return;
}

void aclk_reset_chart_event(char *node_id, uint64_t last_sequence_id)
{
    if (unlikely(!node_id))
        return;

    debug(D_ACLK_SYNC, "NODE %s wants to resync from %"PRIu64, node_id, last_sequence_id);
    aclk_submit_param_command(node_id, ACLK_DATABASE_RESET_CHART, last_sequence_id);
    return;
}

// ST is read locked
int sql_queue_chart_to_aclk(RRDSET *st)
{
    return sql_queue_chart_payload((struct aclk_database_worker_config *) st->rrdhost->dbsync_worker,
                                   st, ACLK_DATABASE_ADD_CHART);
}

int sql_queue_dimension_to_aclk(RRDDIM *rd)
{
    int rc = sql_queue_chart_payload((struct aclk_database_worker_config *) rd->rrdset->rrdhost->dbsync_worker,
                                     rd, ACLK_DATABASE_ADD_DIMENSION);
    if (likely(!rc))
        rrddim_flag_set(rd, RRDDIM_FLAG_ACLK);
    return rc;
}

void sql_chart_deduplicate(struct aclk_database_worker_config *wc, struct aclk_database_cmd cmd)
{
    UNUSED(cmd);

    BUFFER *sql = buffer_create(1024);

    db_lock();
    buffer_sprintf(sql, "DROP TABLE IF EXISTS t_%s;", wc->uuid_str);
    db_execute(buffer_tostring(sql));

    buffer_flush(sql);
    buffer_sprintf(sql, "CREATE TABLE t_%s AS SELECT * FROM aclk_chart_payload_%s WHERE unique_id IN "
        "(SELECT unique_id from aclk_chart_%s WHERE date_submitted IS NULL AND update_count > 0);",
        wc->uuid_str, wc->uuid_str, wc->uuid_str);
    db_execute(buffer_tostring(sql));

    buffer_flush(sql);
    buffer_sprintf(sql, "DELETE FROM aclk_chart_payload_%s WHERE unique_id IN (SELECT unique_id FROM t_%s); " ,
       wc->uuid_str, wc->uuid_str);
    db_execute(buffer_tostring(sql));

    buffer_flush(sql);
    buffer_sprintf(sql, "DELETE FROM aclk_chart_%s WHERE unique_id IN (SELECT unique_id FROM t_%s);",
       wc->uuid_str, wc->uuid_str);
    db_execute(buffer_tostring(sql));

    buffer_flush(sql);
    buffer_sprintf(sql, "DELETE FROM aclk_chart_latest_%s WHERE unique_id IN (SELECT unique_id FROM t_%s);",
       wc->uuid_str, wc->uuid_str);
    db_execute(buffer_tostring(sql));

    buffer_flush(sql);
    buffer_sprintf(sql, "INSERT INTO aclk_chart_payload_%s SELECT * FROM t_%s ORDER BY DATE_CREATED ASC;",
                   wc->uuid_str, wc->uuid_str);
    db_execute(buffer_tostring(sql));

    buffer_flush(sql);
    buffer_sprintf(sql, "INSERT OR REPLACE INTO aclk_chart_latest_%s (uuid, unique_id, date_submitted) "
                        "SELECT uuid, unique_id, date_submitted FROM aclk_chart_%s where sequence_id IN "
                        "(SELECT sequence_id FROM aclk_chart_%s WHERE date_submitted IS NOT NULL "
                        "GROUP BY uuid HAVING sequence_id = MAX(sequence_id));"
                        , wc->uuid_str, wc->uuid_str, wc->uuid_str);
    db_execute(buffer_tostring(sql));

    buffer_flush(sql);
    buffer_sprintf(sql, "DROP TABLE IF EXISTS t_%s;", wc->uuid_str);
    db_execute(buffer_tostring(sql));
    db_unlock();

    sql_get_last_chart_sequence(wc, cmd);

    buffer_free(sql);
    return;
}

void sql_get_last_chart_sequence(struct aclk_database_worker_config *wc, struct aclk_database_cmd cmd)
{
    UNUSED(cmd);

    BUFFER *sql = buffer_create(1024);

    buffer_sprintf(sql,"SELECT ac.sequence_id, ac.date_created FROM aclk_chart_%s ac " \
                        "WHERE ac.date_submitted IS NOT NULL ORDER BY ac.sequence_id DESC LIMIT 1;", wc->uuid_str);

    int rc;
    sqlite3_stmt *res = NULL;
    rc = sqlite3_prepare_v2(db_meta, buffer_tostring(sql), -1, &res, 0);
    if (rc != SQLITE_OK) {
        error_report("Failed to prepare statement to find last chart sequence id");
        goto fail;
    }

    wc->chart_sequence_id = 0;
    wc->chart_timestamp = 0;
    while (sqlite3_step(res) == SQLITE_ROW) {
        wc->chart_sequence_id = (uint64_t) sqlite3_column_int64(res, 0);
        wc->chart_timestamp  = (time_t) sqlite3_column_int64(res, 1);
    }

    debug(D_ACLK_SYNC,"Node %s reports last sequence_id=%"PRIu64, wc->node_id, wc->chart_sequence_id);

    rc = sqlite3_finalize(res);
    if (unlikely(rc != SQLITE_OK))
        error_report("Failed to reset statement when fetching chart sequence info, rc = %d", rc);

fail:
    buffer_free(sql);
    return;
}

// Start streaming charts / dimensions for node_id
void aclk_start_streaming(char *node_id, uint64_t sequence_id, time_t created_at, uint64_t batch_id)
{
#ifdef ACLK_NG
    if (unlikely(!node_id))
        return;

    debug(D_ACLK_SYNC,"START streaming charts for node %s from sequence %"PRIu64" t=%ld, batch=%"PRIu64, node_id,
          sequence_id, created_at, batch_id);
    uuid_t node_uuid;
    if (uuid_parse(node_id, node_uuid))
        return;

    struct aclk_database_worker_config *wc  = NULL;
    rrd_wrlock();
    RRDHOST *host = localhost;
    while(host) {
        if (host->node_id && !(uuid_compare(*host->node_id, node_uuid))) {
            rrd_unlock();
            wc = (struct aclk_database_worker_config *)host->dbsync_worker;
            if (likely(wc)) {
                //                if (unlikely(!wc->chart_updates)) {
                //                    struct aclk_database_cmd cmd;
                //                    cmd.opcode = ACLK_DATABASE_NODE_INFO;
                //                    cmd.completion = NULL;
                //                    aclk_database_enq_cmd(wc, &cmd);
                //                }

                wc->chart_reset_count++;
                wc->chart_updates = 0;
                wc->batch_id = batch_id;
                wc->batch_created = now_realtime_sec();
                info("DEBUG: START streaming charts for %s (%s) enabled -- last streamed sequence %"PRIu64" t=%ld  (reset count=%d)", node_id, wc->uuid_str,
                     wc->chart_sequence_id, wc->chart_timestamp, wc->chart_reset_count);
                // If mismatch detected
                if (sequence_id > wc->chart_sequence_id || wc->chart_reset_count > 10) {
                    info("DEBUG: Full resync requested -- reset_count=%d", wc->chart_reset_count);
                    chart_reset_t chart_reset;
                    chart_reset.node_id = strdupz(node_id);
                    chart_reset.claim_id = is_agent_claimed();
                    chart_reset.reason = SEQ_ID_NOT_EXISTS;
                    aclk_chart_reset(chart_reset);
//                    wc->chart_updates = 0;
                    wc->chart_reset_count = -1;
                    return;
                } else {
                    struct aclk_database_cmd cmd;
                    memset(&cmd, 0, sizeof(cmd));
                    // TODO: handle timestamp
//                    if (!wc->chart_reset_count)
//                        wc->chart_delay = now_realtime_sec() + 60;
//                    else
//                        wc->chart_delay = 0;

                    if (sequence_id < wc->chart_sequence_id) { // || created_at != wc->chart_timestamp) {
//                        wc->chart_updates = 0;
                        if (sequence_id)
                            info("DEBUG: Synchonization mismatch detected");
                        else
                            info("DEBUG: Synchonization mismatch detected; full resync ACKed from the cloud");
                        cmd.opcode = ACLK_DATABASE_RESET_CHART;
                        cmd.param1 = sequence_id + 1;
                        cmd.completion = NULL;
                        aclk_database_enq_cmd(wc, &cmd);
                    }
                    else
                        wc->chart_updates = 1;
                }
            }
            else
                error("ACLK synchronization thread is not active for host %s", host->hostname);
            return;
        }
        host = host->next;
    }
    rrd_unlock();
#else
    UNUSED(node_id);
    UNUSED(sequence_id);
    UNUSED(created_at);
    UNUSED(batch_id);
#endif
    return;
}
