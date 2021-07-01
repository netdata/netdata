// SPDX-License-Identifier: GPL-3.0-or-later

#include "sqlite_functions.h"
#include "sqlite_aclk.h"

#ifndef ACLK_NG
#include "../../aclk/legacy/agent_cloud_link.h"
#else
#include "../../aclk/aclk.h"
#include "../../aclk/aclk_charts_api.h"
#endif

int aclk_architecture = 0;

void aclk_database_init_cmd_queue(struct aclk_database_worker_config *wc)
{
    wc->cmd_queue.head = wc->cmd_queue.tail = 0;
    wc->queue_size = 0;
    fatal_assert(0 == uv_cond_init(&wc->cmd_cond));
    fatal_assert(0 == uv_mutex_init(&wc->cmd_mutex));
}

void aclk_database_enq_cmd_nowake(struct aclk_database_worker_config *wc, struct aclk_database_cmd *cmd)
{
    unsigned queue_size;

    /* wait for free space in queue */
    uv_mutex_lock(&wc->cmd_mutex);
    while ((queue_size = wc->queue_size) == ACLK_DATABASE_CMD_Q_MAX_SIZE) {
        uv_cond_wait(&wc->cmd_cond, &wc->cmd_mutex);
    }
    fatal_assert(queue_size < ACLK_DATABASE_CMD_Q_MAX_SIZE);
    /* enqueue command */
    wc->cmd_queue.cmd_array[wc->cmd_queue.tail] = *cmd;
    wc->cmd_queue.tail = wc->cmd_queue.tail != ACLK_DATABASE_CMD_Q_MAX_SIZE - 1 ?
                         wc->cmd_queue.tail + 1 : 0;
    wc->queue_size = queue_size + 1;
    uv_mutex_unlock(&wc->cmd_mutex);
}

void aclk_database_enq_cmd(struct aclk_database_worker_config *wc, struct aclk_database_cmd *cmd)
{
    unsigned queue_size;

    /* wait for free space in queue */
    uv_mutex_lock(&wc->cmd_mutex);
    while ((queue_size = wc->queue_size) == ACLK_DATABASE_CMD_Q_MAX_SIZE) {
        uv_cond_wait(&wc->cmd_cond, &wc->cmd_mutex);
    }
    fatal_assert(queue_size < ACLK_DATABASE_CMD_Q_MAX_SIZE);
    /* enqueue command */
    wc->cmd_queue.cmd_array[wc->cmd_queue.tail] = *cmd;
    wc->cmd_queue.tail = wc->cmd_queue.tail != ACLK_DATABASE_CMD_Q_MAX_SIZE - 1 ?
                         wc->cmd_queue.tail + 1 : 0;
    wc->queue_size = queue_size + 1;
    uv_mutex_unlock(&wc->cmd_mutex);

    /* wake up event loop */
    fatal_assert(0 == uv_async_send(&wc->async));
}

struct aclk_database_cmd aclk_database_deq_cmd(struct aclk_database_worker_config* wc)
{
    struct aclk_database_cmd ret;
    unsigned queue_size;

    uv_mutex_lock(&wc->cmd_mutex);
    queue_size = wc->queue_size;
    if (queue_size == 0) {
        ret.opcode = ACLK_DATABASE_NOOP;
        ret.completion = NULL;
    } else {
        /* dequeue command */
        ret = wc->cmd_queue.cmd_array[wc->cmd_queue.head];
        if (queue_size == 1) {
            wc->cmd_queue.head = wc->cmd_queue.tail = 0;
        } else {
            wc->cmd_queue.head = wc->cmd_queue.head != ACLK_DATABASE_CMD_Q_MAX_SIZE - 1 ?
                                 wc->cmd_queue.head + 1 : 0;
        }
        wc->queue_size = queue_size - 1;

        /* wake up producers */
        uv_cond_signal(&wc->cmd_cond);
    }
    uv_mutex_unlock(&wc->cmd_mutex);

    return ret;
}

static void async_cb(uv_async_t *handle)
{
    uv_stop(handle->loop);
    uv_update_time(handle->loop);
    debug(D_ACLK_SYNC, "%s called, active=%d.", __func__, uv_is_active((uv_handle_t *)handle));
}

#define TIMER_PERIOD_MS (5000)

static void timer_cb(uv_timer_t* handle)
{
    struct aclk_database_worker_config *wc = handle->data;
    uv_stop(handle->loop);
    uv_update_time(handle->loop);

    struct aclk_database_cmd cmd;
    cmd.opcode = ACLK_DATABASE_TIMER;
    cmd.completion = NULL;
    aclk_database_enq_cmd(wc, &cmd);

    if (wc->chart_updates) {
        cmd.opcode = ACLK_DATABASE_PUSH_CHART;
        cmd.count = ACLK_MAX_CHART_UPDATES;
        cmd.completion = NULL;
        aclk_database_enq_cmd(wc, &cmd);
    }
}

#define MAX_CMD_BATCH_SIZE (256)

void aclk_database_worker(void *arg)
{
    struct aclk_database_worker_config *wc = arg;
    uv_loop_t *loop;
    int shutdown, ret;
    enum aclk_database_opcode opcode;
    uv_timer_t timer_req;
    struct aclk_database_cmd cmd;
    unsigned cmd_batch_size;

    wc->chart_updates = 0;
    wc->alert_updates = 0;

    aclk_database_init_cmd_queue(wc);
    uv_thread_set_name_np(wc->thread, wc->uuid_str);

    loop = wc->loop = mallocz(sizeof(uv_loop_t));
    ret = uv_loop_init(loop);
    if (ret) {
        error("uv_loop_init(): %s", uv_strerror(ret));
        goto error_after_loop_init;
    }
    loop->data = wc;

    ret = uv_async_init(wc->loop, &wc->async, async_cb);
    if (ret) {
        error("uv_async_init(): %s", uv_strerror(ret));
        goto error_after_async_init;
    }
    wc->async.data = wc;

    ret = uv_timer_init(loop, &timer_req);
    if (ret) {
        error("uv_timer_init(): %s", uv_strerror(ret));
        goto error_after_timer_init;
    }
    timer_req.data = wc;

    wc->error = 0;
    fatal_assert(0 == uv_timer_start(&timer_req, timer_cb, TIMER_PERIOD_MS, TIMER_PERIOD_MS));
    shutdown = 0;

//    cmd.opcode = ACLK_DATABASE_SYNC_CHART_SEQ;
//    aclk_database_enq_cmd(wc, &cmd);

    info("Starting ACLK sync event loop for host with GUID %s (Host is '%s')", wc->host_guid, wc->host ? "connected" : "not connected");
    sql_get_last_chart_sequence(wc, cmd);
    while (likely(shutdown == 0)) {
        uv_run(loop, UV_RUN_DEFAULT);

        if (netdata_exit)
            shutdown = 1;

        /* wait for commands */
        cmd_batch_size = 0;
        do {
            if (unlikely(cmd_batch_size >= MAX_CMD_BATCH_SIZE))
                break;
            cmd = aclk_database_deq_cmd(wc);
            opcode = cmd.opcode;
            ++cmd_batch_size;
            db_lock();
            switch (opcode) {
                case ACLK_DATABASE_NOOP:
                    /* the command queue was empty, do nothing */
                    break;
                case ACLK_DATABASE_CLEANUP:
                    debug(D_ACLK_SYNC, "Database cleanup for %s", wc->uuid_str);
                    //sql_maint_database();
                    break;
                case ACLK_DATABASE_PUSH_CHART:
                    debug(D_ACLK_SYNC, "Pushing chart info to the cloud for node %s", wc->uuid_str);
                    aclk_push_chart_event(wc, cmd);
                    break;
                case ACLK_DATABASE_PUSH_CHART_CONFIG:
                    debug(D_ACLK_SYNC, "Pushing chart config info to the cloud for node %s", wc->uuid_str);
                    aclk_push_chart_config_event(wc, cmd);
                    break;
                case ACLK_DATABASE_CHART_ACK:
                    debug(D_ACLK_SYNC, "ACK chart SEQ for %s to %"PRIu64, wc->uuid_str, (uint64_t) cmd.param1);
                    sql_set_chart_ack(wc, cmd);
                    break;
                case ACLK_DATABASE_RESET_CHART:
                    debug(D_ACLK_SYNC, "RESET chart SEQ for %s to %"PRIu64, wc->uuid_str, (uint64_t) cmd.param1);

                    sql_reset_chart_event(wc, cmd);
                    break;
                case ACLK_DATABASE_RESET_NODE:
                    debug(D_ACLK_SYNC,"Resetting the node instance id of host %s", (char *) cmd.data);
                    aclk_reset_node_event(wc, cmd);
                    break;
                case ACLK_DATABASE_STATUS_CHART:
                    debug(D_ACLK_SYNC,"Requesting chart status for %s", wc->uuid_str);
                    aclk_status_chart_event(wc, cmd);
                    break;
                case ACLK_DATABASE_ADD_CHART:
                    debug(D_ACLK_SYNC,"Adding chart event for %s", wc->uuid_str);
                    aclk_add_chart_event(wc, cmd);
                    break;
                case ACLK_DATABASE_ADD_ALERT:
                    debug(D_ACLK_SYNC,"Adding alert event for %s", wc->uuid_str);
                    aclk_add_alert_event(wc, cmd);
                    break;
                case ACLK_DATABASE_ADD_DIMENSION:
                    debug(D_ACLK_SYNC,"Adding dimension event for %s", wc->uuid_str);
                    aclk_add_dimension_event(wc, cmd);
                    break;
                case ACLK_DATABASE_NODE_INFO:
                    debug(D_ACLK_SYNC,"Sending node info for %s", wc->uuid_str);
                    sql_build_node_info(wc, cmd);
                    break;
                case ACLK_DATABASE_UPD_STATS:
                    sql_update_metric_statistics(wc, cmd);
                    break;
                case ACLK_DATABASE_TIMER:
                    if (unlikely(localhost && !wc->host)) {
                        char *agent_id = is_agent_claimed();
                        if (agent_id) {
                            wc->host = rrdhost_find_by_guid(wc->host_guid, 0);
                            if (wc->host) {
                                info("HOST %s detected as active and claimed !!!", wc->host->hostname);
                                wc->host->dbsync_worker = wc;
                                if (wc->host->node_id) {
                                    cmd.opcode = ACLK_DATABASE_NODE_INFO;
                                    cmd.completion = NULL;
                                    aclk_database_enq_cmd(wc, &cmd);
                                }
                                cmd.opcode = ACLK_DATABASE_UPD_STATS;
                                cmd.completion = NULL;
                                aclk_database_enq_cmd(wc, &cmd);
                            }
                            freez(agent_id);
                        }
                    }
                    break;
                case ACLK_DATABASE_DEDUP_CHART:
                    debug(D_ACLK_SYNC,"Running chart deduplication for %s", wc->uuid_str);
                    sql_chart_deduplicate(wc, cmd);
                    break;
                case ACLK_DATABASE_SYNC_CHART_SEQ:
                    debug(D_ACLK_SYNC,"Calculatting chart sequence for %s", wc->uuid_str);
                    sql_get_last_chart_sequence(wc, cmd);
                    break;
                case ACLK_DATABASE_SHUTDOWN:
                    shutdown = 1;
                    fatal_assert(0 == uv_timer_stop(&timer_req));
                    uv_close((uv_handle_t *)&timer_req, NULL);
                    break;
                default:
                    debug(D_ACLK_SYNC, "%s: default.", __func__);
                    break;
            }
            db_unlock();
            if (cmd.completion)
                complete(cmd.completion);
        } while (opcode != ACLK_DATABASE_NOOP);
    }

    /* cleanup operations of the event loop */
    info("Shutting down ACLK_DATABASE engine event loop.");

    /*
     * uv_async_send after uv_close does not seem to crash in linux at the moment,
     * it is however undocumented behaviour and we need to be aware if this becomes
     * an issue in the future.
     */
    uv_close((uv_handle_t *)&wc->async, NULL);
    uv_run(loop, UV_RUN_DEFAULT);

    info("Shutting down ACLK_DATABASE engine event loop complete.");
    /* TODO: don't let the API block by waiting to enqueue commands */
    uv_cond_destroy(&wc->cmd_cond);
/*  uv_mutex_destroy(&wc->cmd_mutex); */
    //fatal_assert(0 == uv_loop_close(loop));
    int rc;

    do {
        rc = uv_loop_close(loop);
//        info("DEBUG: LOOP returns %d", rc);
    } while (rc != UV_EBUSY);

    freez(loop);

    rrd_wrlock();
    if (likely(wc->host))
        wc->host->dbsync_worker = NULL;
    freez(wc);
    rrd_unlock();
    return;

    error_after_timer_init:
    uv_close((uv_handle_t *)&wc->async, NULL);
    error_after_async_init:
    fatal_assert(0 == uv_loop_close(loop));
    error_after_loop_init:
    freez(loop);

    wc->error = UV_EAGAIN;
}

// -------------------------------------------------------------

void aclk_set_architecture(int mode)
{
    aclk_architecture = mode;
}

int aclk_add_chart_payload(char *uuid_str, uuid_t *uuid, char *claim_id, char *payload_type, void *payload, size_t payload_size)
{
    sqlite3_stmt *res_chart = NULL;
    int rc;

    BUFFER *sql = buffer_create(1024);

    //uuid, claim_id, type, date_created, payload

    buffer_sprintf(sql,"INSERT INTO aclk_chart_payload_%s (unique_id, uuid, claim_id, date_created, type, payload) " \
                 "VALUES (@unique_id, @uuid, @claim_id, strftime('%%s'), @type, @payload);", uuid_str);

    rc = sqlite3_prepare_v2(db_meta, buffer_tostring(sql), -1, &res_chart, 0);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to prepare statement to store chart payload data");
        buffer_free(sql);
        return 1;
    }

    uuid_t unique_uuid;
    uuid_generate(unique_uuid);

    uuid_t claim_uuid;
    uuid_parse(claim_id, claim_uuid);

    rc = sqlite3_bind_blob(res_chart, 1, &unique_uuid , sizeof(unique_uuid), SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    rc = sqlite3_bind_blob(res_chart, 2, uuid , sizeof(*uuid), SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    rc = sqlite3_bind_blob(res_chart, 3, &claim_uuid , sizeof(claim_uuid), SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    rc = sqlite3_bind_text(res_chart, 4, payload_type ,-1, SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    rc = sqlite3_bind_blob(res_chart, 5, payload, payload_size, SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    rc = execute_insert(res_chart);
    if (unlikely(rc != SQLITE_DONE))
        error_report("Failed store chart payload event, rc = %d", rc);

bind_fail:
    if (unlikely(sqlite3_finalize(res_chart) != SQLITE_OK))
        error_report("Failed to reset statement in store chart payload, rc = %d", rc);
    buffer_free(sql);
    return (rc != SQLITE_DONE);
}

int aclk_add_chart_event(struct aclk_database_worker_config *wc, struct aclk_database_cmd cmd)
{
    int rc = 0;
    if (unlikely(!db_meta)) {
        freez(cmd.data_param);
        if (default_rrd_memory_mode != RRD_MEMORY_MODE_DBENGINE) {
            return 1;
        }
        error_report("Database has not been initialized");
        return 1;
    }

#ifdef ACLK_NG
    char *claim_id = is_agent_claimed();

    RRDSET *st = cmd.data;
    char *payload_type = cmd.data_param;

    if (likely(claim_id)) {
        struct chart_instance_updated chart_payload;
        memset(&chart_payload, 0, sizeof(chart_payload));
        chart_payload.config_hash = get_str_from_uuid(&st->state->hash_id);
        chart_payload.update_every = st->update_every;
        chart_payload.memory_mode = st->rrd_memory_mode;
        chart_payload.name = strdupz((char *)st->name);
        chart_payload.node_id = get_str_from_uuid(st->rrdhost->node_id);
        chart_payload.claim_id = claim_id;
        chart_payload.id = strdupz(st->id);

        size_t size;
        char *payload = generate_chart_instance_updated(&size, &chart_payload);
        if (likely(payload))
            rc = aclk_add_chart_payload(wc->uuid_str, st->chart_uuid, claim_id, "0", (void *) payload, size);
        freez(payload);
        chart_instance_updated_destroy(&chart_payload);
    }
#else
    UNUSED(wc);
    UNUSED(cmd);
#endif
    freez(cmd.data_param);
    return rc;
}

int aclk_add_dimension_event(struct aclk_database_worker_config *wc, struct aclk_database_cmd cmd)
{
    int rc = 0;
    if (unlikely(!db_meta)) {
        freez(cmd.data_param);
        if (default_rrd_memory_mode != RRD_MEMORY_MODE_DBENGINE) {
            return 1;
        }
        error_report("Database has not been initialized");
        return 1;
    }

#ifdef ACLK_NG
    char *claim_id = is_agent_claimed();

    RRDDIM *rd = cmd.data;
    char *payload_type = cmd.data_param;

    time_t now = now_realtime_sec();

    if (likely(claim_id)) {
        const char *node_id = NULL;
        node_id = get_str_from_uuid(rd->rrdset->rrdhost->node_id);
        struct chart_dimension_updated dim_payload;
        memset(&dim_payload, 0, sizeof(dim_payload));
        dim_payload.node_id = node_id;
        dim_payload.claim_id = claim_id;

        dim_payload.chart_id = strdupz(rd->rrdset->name);
        dim_payload.created_at = rd->last_collected_time; //TODO: Fix with creation time
        if ((now - rd->last_collected_time.tv_sec) < (RRDSET_MINIMUM_LIVE_COUNT * rd->update_every)) {
            dim_payload.last_timestamp.tv_usec = 0;
            dim_payload.last_timestamp.tv_sec = 0;
        }
        else
            dim_payload.last_timestamp = rd->last_collected_time;

        dim_payload.name = strdupz(rd->name);
        dim_payload.id = strdupz(rd->id);

        size_t size;
        char *payload = generate_chart_dimension_updated(&size, &dim_payload);
        if (likely(payload))
            rc = aclk_add_chart_payload(wc->uuid_str, rd->state->metric_uuid, claim_id, "1", (void *) payload, size);
        freez(payload);
        //chart_instance_updated_destroy(&dim_payload);
    }
#else
    UNUSED(wc);
    UNUSED(cmd);
#endif
    freez(cmd.data_param);
    return rc;
}

void sql_reset_chart_event(struct aclk_database_worker_config *wc, struct aclk_database_cmd cmd)
{
    BUFFER *sql = buffer_create(1024);
    buffer_sprintf(sql, "UPDATE aclk_chart_%s SET status = NULL, date_submitted = NULL WHERE sequence_id >= %"PRIu64";",
                        wc->uuid_str, cmd.param1);
    db_execute(buffer_tostring(sql));
    buffer_free(sql);
    sql_chart_deduplicate(wc, cmd);
    return;
}

void aclk_reset_node_event(struct aclk_database_worker_config *wc, struct aclk_database_cmd cmd)
{
    int rc;
    uuid_t  host_id;

    UNUSED(wc);

    rc = uuid_parse((char *) cmd.data, host_id);
    if (unlikely(rc))
        goto fail1;

    BUFFER *sql = buffer_create(1024);

    buffer_sprintf(sql, "UPDATE node_instance set node_id = null WHERE host_id = @host_id;");

    sqlite3_stmt *res = NULL;
    rc = sqlite3_prepare_v2(db_meta, buffer_tostring(sql), -1, &res, 0);
    if (rc != SQLITE_OK) {
        error_report("Failed to prepare statement to reset node id in the database");
        goto fail;
    }

    rc = sqlite3_bind_blob(res, 1, &host_id , sizeof(host_id), SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    rc = execute_insert(res);
    if (unlikely(rc != SQLITE_DONE))
        error_report("Failed to reset the node id of host %s, rc = %d", (char *) cmd.data, rc);

bind_fail:
    rc = sqlite3_finalize(res);
    if (unlikely(rc != SQLITE_OK))
        error_report("Failed to reset statement when doing a node reset, rc = %d", rc);

fail:
    buffer_free(sql);

fail1:
    return;
}


void aclk_status_chart_event(struct aclk_database_worker_config *wc, struct aclk_database_cmd cmd)
{
    int rc;

    BUFFER *sql = buffer_create(1024);
    BUFFER *resp = buffer_create(1024);

    buffer_sprintf(sql, "SELECT CASE WHEN status IS NULL AND date_submitted IS NULL THEN 'resync' " \
        "WHEN status IS NULL THEN 'submitted' ELSE status END, " \
        "COUNT(*), MIN(sequence_id), MAX(sequence_id) FROM " \
        "aclk_chart_%s GROUP BY 1;", wc->uuid_str);

    sqlite3_stmt *res = NULL;
    rc = sqlite3_prepare_v2(db_meta, buffer_tostring(sql), -1, &res, 0);
    if (rc != SQLITE_OK) {
        error_report("Failed to prepare statement to lookup chart UUID in the database");
        goto fail;
    }

    struct aclk_chart_payload_t *head = NULL;
    while (sqlite3_step(res) == SQLITE_ROW) {
        struct aclk_chart_payload_t *chart_payload = callocz(1, sizeof(*chart_payload));
        buffer_flush(resp);
        buffer_sprintf(resp, "Status: %s\n Count: %lld\n Min sequence_id: %lld\n Max sequence_id: %lld\n",
                       (char *) sqlite3_column_text(res, 0),
                       sqlite3_column_int64(res, 1), sqlite3_column_int64(res, 2), sqlite3_column_int64(res, 3));

        chart_payload->payload = strdupz(buffer_tostring(resp));

        chart_payload->next =  head;
        head = chart_payload;
    }
    *(struct aclk_chart_payload_t **) cmd.data = head;

    rc = sqlite3_finalize(res);
    if (unlikely(rc != SQLITE_OK))
        error_report("Failed to reset statement when searching for a chart UUID, rc = %d", rc);

fail:
    buffer_free(sql);
    buffer_free(resp);
    return;
}

// ST is read locked
void sql_queue_chart_to_aclk(RRDSET *st, int mode)
{
    UNUSED(mode);

    if (!aclk_architecture)
        aclk_update_chart(st->rrdhost, st->id, ACLK_CMD_CHART);

    if (unlikely(!st->rrdhost->dbsync_worker))
        return;

    struct aclk_database_cmd cmd;
    cmd.opcode = ACLK_DATABASE_ADD_CHART;
    cmd.data = st;
    cmd.data_param = strdupz("BINARY");
    cmd.completion = NULL;
    aclk_database_enq_cmd((struct aclk_database_worker_config *) st->rrdhost->dbsync_worker, &cmd);
    return;
}

void sql_queue_dimension_to_aclk(RRDDIM *rd, int mode)
{
    UNUSED(mode);

    if (unlikely(!rd->rrdset->rrdhost->dbsync_worker))
        return;

    struct aclk_database_cmd cmd;
    cmd.opcode = ACLK_DATABASE_ADD_DIMENSION;
    cmd.data = rd;
    cmd.data_param = strdupz("BINARY");
    cmd.completion = NULL;
    aclk_database_enq_cmd((struct aclk_database_worker_config *) rd->rrdset->rrdhost->dbsync_worker, &cmd);
    return;
}

void sql_queue_alarm_to_aclk(RRDHOST *host, ALARM_ENTRY *ae)
{
    if (!aclk_architecture) //{ //queue_chart_to_aclk continues after that, should we too ?
        aclk_update_alarm(host, ae);

    if (unlikely(!host->dbsync_worker))
       return;

    struct aclk_database_cmd cmd;
    cmd.opcode = ACLK_DATABASE_ADD_ALERT;
    cmd.data = ae;
    //cmd.data1 = ae;
    //cmd.data_param = ae;
    cmd.completion = NULL;
    aclk_database_enq_cmd((struct aclk_database_worker_config *) host->dbsync_worker, &cmd);
    return;
}

void sql_drop_host_aclk_table_list(uuid_t *host_uuid)
{
    int rc;
    char uuid_str[GUID_LEN + 1];

    uuid_unparse_lower_fix(host_uuid, uuid_str);

    BUFFER *sql = buffer_create(1024);
    buffer_sprintf(
        sql,"SELECT 'drop '||type||' IF EXISTS '||name||';' FROM sqlite_schema " \
        "WHERE name LIKE 'aclk_%%_%s' AND type IN ('table', 'trigger', 'index');", uuid_str);

    sqlite3_stmt *res = NULL;

    rc = sqlite3_prepare_v2(db_meta, buffer_tostring(sql), -1, &res, 0);
    if (rc != SQLITE_OK) {
        error_report("Failed to prepare statement to clean up aclk tables");
        goto fail;
    }
    buffer_flush(sql);

    while (sqlite3_step(res) == SQLITE_ROW)
        buffer_strcat(sql, (char *) sqlite3_column_text(res, 0));

    rc = sqlite3_finalize(res);
    if (unlikely(rc != SQLITE_OK))
        error_report("Failed to finalize statement to clean up aclk tables, rc = %d", rc);

    db_execute(buffer_tostring(sql));

fail:
    buffer_free(sql);
}

void sql_create_aclk_table(RRDHOST *host, uuid_t *host_uuid)
{
    char uuid_str[GUID_LEN + 1];
    char host_guid[GUID_LEN + 1];

    uuid_unparse_lower(*host_uuid, host_guid);
    uuid_unparse_lower_fix(host_uuid, uuid_str);

    BUFFER *sql = buffer_create(1024);

    buffer_sprintf(sql, TABLE_ACLK_CHART, uuid_str);
    db_execute(buffer_tostring(sql));
    buffer_flush(sql);

    buffer_sprintf(sql, TABLE_ACLK_CHART_PAYLOAD, uuid_str);
    db_execute(buffer_tostring(sql));
    buffer_flush(sql);

    buffer_sprintf(sql,TRIGGER_ACLK_CHART_PAYLOAD, uuid_str, uuid_str, uuid_str);
    db_execute(buffer_tostring(sql));
    buffer_flush(sql);

    buffer_sprintf(sql,TABLE_ACLK_ALERT, uuid_str);
    db_execute(buffer_tostring(sql));
    buffer_flush(sql);
//
//    buffer_sprintf(sql,TABLE_ACLK_ALERT_PAYLOAD, uuid_str);
//    db_execute(buffer_tostring(sql));
//
//    buffer_sprintf(sql,TRIGGER_ACLK_ALERT_PAYLOAD, uuid_str, uuid_str, uuid_str);
//    db_execute(buffer_tostring(sql));
//    buffer_flush(sql);

    buffer_free(sql);

    // Spawn db thread for processing (event loop)
    if (likely(host) && unlikely(host->dbsync_worker))
        return;

    struct aclk_database_worker_config *wc = callocz(1, sizeof(struct aclk_database_worker_config));
    if (likely(host))
         host->dbsync_worker = (void *) wc;
    wc->host = host;
    strcpy(wc->uuid_str, uuid_str);
    strcpy(wc->host_guid, host_guid);
    fatal_assert(0 == uv_thread_create(&(wc->thread), aclk_database_worker, wc));
}

void sql_aclk_drop_all_table_list()
{
    int rc;

    BUFFER *sql = buffer_create(1024);
    buffer_strcat(sql, "SELECT host_id FROM host;");
    sqlite3_stmt *res = NULL;

    rc = sqlite3_prepare_v2(db_meta, buffer_tostring(sql), -1, &res, 0);
    if (rc != SQLITE_OK) {
        error_report("Failed to prepare statement to clean up aclk tables");
        goto fail;
    }
    while (sqlite3_step(res) == SQLITE_ROW) {
        sql_drop_host_aclk_table_list((uuid_t *)sqlite3_column_blob(res, 0));
    }

    rc = sqlite3_finalize(res);
    if (unlikely(rc != SQLITE_OK))
        error_report("Failed to finalize statement to clean up aclk tables, rc = %d", rc);

fail:
    buffer_free(sql);
    return;
}

// Start streaming charts / dimensions for node_id
void aclk_start_streaming(char *node_id, uint64_t sequence_id, time_t created_at, uint64_t batch_id)
{
    if (unlikely(!node_id))
        return;

    info("DEBUG: START streaming charts for %s received from sequence %"PRIu64" t=%ld", node_id, sequence_id, created_at);
    uuid_t node_uuid;
    uuid_parse(node_id, node_uuid);

    struct aclk_database_worker_config *wc  = NULL;
    rrd_wrlock();
    RRDHOST *host = localhost;
    while(host) {
        if (host->node_id && !(uuid_compare(*host->node_id, node_uuid))) {
            wc = (struct aclk_database_worker_config *)host->dbsync_worker;
            if (likely(wc)) {

                if (unlikely(!wc->chart_updates)) {
                    struct aclk_database_cmd cmd;
                    cmd.opcode = ACLK_DATABASE_NODE_INFO;
                    cmd.completion = NULL;
                    aclk_database_enq_cmd(wc, &cmd);
                }

                wc->chart_updates = 1;
                wc->batch_id = batch_id;
                info("DEBUG: START streaming charts for %s (%s) enabled -- last streamed sequence %"PRIu64" t=%ld", node_id, wc->uuid_str,
                     wc->chart_sequence_id, wc->chart_timestamp);
                // If mismatch detected
                if (sequence_id > wc->chart_sequence_id) {
                    info("DEBUG: Full resync requested");
                    chart_reset_t chart_reset;
                    chart_reset.node_id = strdupz(node_id);
                    chart_reset.claim_id = is_agent_claimed();
                    chart_reset.reason = SEQ_ID_NOT_EXISTS;
                    aclk_chart_reset(chart_reset);
                } else {
                    struct aclk_database_cmd cmd;
                    // TODO: handle timestamp
                    if (sequence_id < wc->chart_sequence_id) { // || created_at != wc->chart_timestamp) {
                        info("DEBUG: Synchonization mismatch detected");
                        cmd.opcode = ACLK_DATABASE_RESET_CHART;
                        cmd.param1 = sequence_id + 1;
                        cmd.completion = NULL;
                        aclk_database_enq_cmd(wc, &cmd);
                    }
                }
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

// Start streaming alerts
void aclk_start_alert_streaming(char *node_id)
{
    if (unlikely(!node_id))
        return;

    info("START streaming alerts for %s received but not yet supported", node_id);
    return;
//
//    uuid_t node_uuid;
//    uuid_parse(node_id, node_uuid);
//
//    struct aclk_database_worker_config *wc  = NULL;
//    rrd_wrlock();
//    RRDHOST *host = localhost;
//    while(host) {
//        if (host->node_id && !(uuid_compare(*host->node_id, node_uuid))) {
//            wc = (struct aclk_database_worker_config *)host->dbsync_worker;
//            if (likely(wc)) {
//                wc->alert_updates = 1;
//                info("START streaming alerts for %s enabled", node_id);
//            }
//            else
//                error("ACLK synchronization thread is not active for host %s", host->hostname);
//            break;
//        }
//        host = host->next;
//    }
//    rrd_unlock();
//
//    return;
}



// READ dimensions of RRDSET
// Build complete list
// RRDSET is locked
char **build_dimension_payload_list(RRDSET *st, size_t **payload_list_size, size_t *total)
{
   int count = 0;
   RRDDIM *rd;
   const char *node_id = NULL;
   rrddim_foreach_read(rd, st) {
       if (unlikely(!count))
           node_id = get_str_from_uuid(rd->rrdset->rrdhost->node_id);
       count++;
   }
   char **payload_list = callocz(count + 1 ,  sizeof(char *));
   *payload_list_size = callocz(count + 1,  sizeof(size_t));
//   struct aclk_message_position *position_list = callocz(count + 1, sizeof(*position_list));
   int i = 0;
   struct chart_dimension_updated dim_payload;
   memset(&dim_payload, 0, sizeof(dim_payload));

   char *claim_id = is_agent_claimed();
   dim_payload.node_id = node_id;
   dim_payload.claim_id = claim_id;
   time_t now = now_realtime_sec();
   rrddim_foreach_read(rd, st)
   {
       //int find_dimension_first_last_t(char *machine_guid, char *chart_id, char *dim_id,
       //       uuid_t *uuid, time_t *first_entry_t, time_t *last_entry_t, uuid_t *rrdeng_uuid)
       dim_payload.chart_id = rd->rrdset->name;
       dim_payload.created_at = rd->last_collected_time; //TODO: Fix with creation time
       if ((now - rd->last_collected_time.tv_sec) < (RRDSET_MINIMUM_LIVE_COUNT * rd->update_every)) {
           dim_payload.last_timestamp.tv_usec = 0;
           dim_payload.last_timestamp.tv_sec = 0;
       }
       else
            dim_payload.last_timestamp = rd->last_collected_time;
       dim_payload.name = rd->name;
       dim_payload.id = rd->id;
       payload_list[i] = generate_chart_dimension_updated(&((*payload_list_size)[i]), &dim_payload);
       ++i;
   }
   *total += count;
   freez((char *) claim_id);
   freez((char *) node_id);
   return payload_list;
}

RRDSET *find_rrdset_by_uuid(RRDHOST *host, uuid_t  *chart_uuid)
{
    int rc;
    RRDSET *st = NULL;

    sqlite3_stmt *res = NULL;

    rc = sqlite3_prepare_v2(db_meta,  "SELECT type||'.'||id FROM chart WHERE chart_id = @chart_id;", -1, &res, 0);
    if (rc != SQLITE_OK) {
        error_report("Failed to prepare statement to find a chart in the database");
        goto fail;
    }

    rc = sqlite3_bind_blob(res, 1, chart_uuid , sizeof(*chart_uuid), SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    while (sqlite3_step(res) == SQLITE_ROW) {
        st = rrdset_find(host, (const char *) sqlite3_column_text(res, 0));
        if (!st)
            st = rrdset_find_byname(host, (const char *) sqlite3_column_text(res, 0));
    }

bind_fail:
    rc = sqlite3_finalize(res);
    if (unlikely(rc != SQLITE_OK))
        error_report("Failed to reset statement when pushing chart events, rc = %d", rc);

fail:
    return st;
}

void aclk_push_chart_event(struct aclk_database_worker_config *wc, struct aclk_database_cmd cmd)
{

#ifndef ACLK_NG
    UNUSED (wc);
    UNUSED(cmd);
#else
    int rc;

    int limit = cmd.count > 0 ? cmd.count : 1;
    int available = 0;
    uint64_t first_sequence = 0;
    uint64_t last_sequence = 0;
    size_t total_dimension_count = 0;
    time_t last_timestamp;

    struct dim_list {
       char **dimension_list;
       size_t *dim_size;
       size_t count;
       size_t position_index;
       struct dim_list *next;
    } *dim_head = NULL, *tmp_dim;

    BUFFER *sql = buffer_create(1024);

    sqlite3_stmt *res = NULL;

    buffer_sprintf(sql, "select count(*) from aclk_chart_%s where case when status is null then 'processing' " \
                        "else status end = 'processing' and date_submitted is null;", wc->uuid_str);
    rc = sqlite3_prepare_v2(db_meta, buffer_tostring(sql), -1, &res, 0);
    if (rc != SQLITE_OK) {
        error_report("Failed to prepare statement count sequence ids in the database");
        goto fail_complete;
    }
    while (sqlite3_step(res) == SQLITE_ROW) {
        available = sqlite3_column_int64(res, 0);
    }
    rc = sqlite3_finalize(res);
    if (unlikely(rc != SQLITE_OK))
        error_report("Failed to reset statement counting pending events, rc = %d", rc);
    buffer_flush(sql);

    info("Available %d limit = %d", available, limit);

    if (limit > available) {
        limit = limit - available;

        buffer_sprintf(sql, "update aclk_chart_%s set status = 'processing' where status = 'pending' "
                            "order by sequence_id limit %d;", wc->uuid_str, limit);
        db_execute(buffer_tostring(sql));
        buffer_flush(sql);
    }

    buffer_sprintf(sql, "select ac.sequence_id, (select sequence_id from aclk_chart_%s " \
        "lac where lac.sequence_id < ac.sequence_id and (status is NULL or status = 'processing')  " \
        "order by lac.sequence_id desc limit 1), " \
        "acp.payload, ac.date_created, ac.uuid, ac.type " \
        "from aclk_chart_%s ac, " \
        "aclk_chart_payload_%s acp " \
        "where (ac.status = 'processing' or (ac.status is NULL and ac.date_submitted is null)) " \
        "and ac.unique_id = acp.unique_id " \
        "order by ac.sequence_id asc limit %d;",
                   wc->uuid_str, wc->uuid_str, wc->uuid_str, limit);

    rc = sqlite3_prepare_v2(db_meta, buffer_tostring(sql), -1, &res, 0);
    if (rc != SQLITE_OK) {
        error_report("Failed to prepare statement when trying to send a chart update via ACLK");
        goto fail;
    }

    char **payload_list = callocz(limit+1, sizeof(char *));
    size_t *payload_list_size = callocz(limit+1, sizeof(size_t));
    struct aclk_message_position *position_list =  callocz(limit+1, sizeof(*position_list));
    int *is_dim = callocz(limit+1, sizeof(*is_dim));

    int count = 0;
    while (sqlite3_step(res) == SQLITE_ROW) {
        size_t  payload_size = sqlite3_column_bytes(res, 2);
        payload_list_size[count] = payload_size;
        payload_list[count] = mallocz(payload_size);
        memcpy(payload_list[count], sqlite3_column_blob(res, 2), payload_size);
        position_list[count].sequence_id = (uint64_t) sqlite3_column_int64(res, 0);
        position_list[count].previous_sequence_id = sqlite3_column_bytes(res, 1) > 0 ? sqlite3_column_int64(res, 1) : 0;
        position_list[count].seq_id_creation_time.tv_sec = sqlite3_column_int64(res, 3);
        position_list[count].seq_id_creation_time.tv_usec = 0;
        if (!first_sequence)
            first_sequence = position_list[count].sequence_id;
        last_sequence = position_list[count].sequence_id;
        last_timestamp = position_list[count].seq_id_creation_time.tv_sec;
        is_dim[count] = sqlite3_column_int(res, 5);  // 0 chart , 1 dimension

//        RRDSET *st = find_rrdset_by_uuid(wc->host, (uuid_t *) sqlite3_column_blob(res, 4));
//        if (st) {
//            rrdset_rdlock(st);
//            tmp_dim = callocz(1, sizeof(*tmp_dim));
//            tmp_dim->position_index = count;
//            tmp_dim->dimension_list = build_dimension_payload_list(st, &tmp_dim->dim_size, &tmp_dim->count);
//            tmp_dim->next = dim_head;
//            dim_head = tmp_dim;
//            total_dimension_count += tmp_dim->count;
//            rrdset_unlock(st);
//        }
        count++;
    }

    rc = sqlite3_finalize(res);
    if (unlikely(rc != SQLITE_OK))
        error_report("Failed to reset statement when pushing chart events, rc = %d", rc);

fail:

    if (likely(first_sequence)) {
        buffer_flush(sql);
        buffer_sprintf(
            sql,
            "update aclk_chart_%s set status = NULL, date_submitted=strftime('%%s') "
            "where (status = 'processing' or (status is NULL and date_submitted is NULL)) "
            "and sequence_id between %" PRIu64 " and %" PRIu64 ";",
            wc->uuid_str,
            first_sequence,
            last_sequence);
        info("DEBUG: %s pushing chart seq %" PRIu64 " - %" PRIu64", t=%ld", wc->uuid_str, first_sequence, last_sequence, last_timestamp);
        db_execute(buffer_tostring(sql));
    }
    else
        info("DEBUG: %s no chart changes detected", wc->uuid_str);
    if  (count == 0) {
        freez(payload_list);
        freez(payload_list_size);
        freez(position_list);
        freez(is_dim);
//        payload_list = NULL;
    }
    else {
        aclk_chart_inst_and_dim_update(payload_list, payload_list_size, is_dim, position_list, wc->batch_id);
            wc->chart_sequence_id = last_sequence;
            wc->chart_timestamp = last_timestamp;
    }

//    if (payload_list) {
//        payload_list = realloc(payload_list, (count + total_dimension_count + 1) * sizeof(char *));
//        payload_list_size = realloc(payload_list_size, (count + total_dimension_count + 1) * sizeof(size_t));
//        position_list = realloc(position_list, (count + total_dimension_count + 1) * sizeof(*position_list));
//        payload_list[count + total_dimension_count] = NULL;
//        int *is_dim = callocz(count + total_dimension_count + 1, sizeof(*is_dim));
//
//        while (dim_head) {
//            tmp_dim = dim_head->next;
//            memcpy(payload_list + count, dim_head->dimension_list, dim_head->count * sizeof(char *));
//            memcpy(payload_list_size + count, dim_head->dim_size, dim_head->count * sizeof(size_t));
//            for (size_t i = 0; i < dim_head->count; ++i) {
//                position_list[count + i] = position_list[dim_head->position_index];
//                is_dim[count + i] = 1;
//            }
//            count += dim_head->count;
//            freez(dim_head->dimension_list);
//            freez(dim_head->dim_size);
//            freez(dim_head);
//            dim_head = tmp_dim;
//        }
//        aclk_chart_inst_and_dim_update(payload_list, payload_list_size, is_dim, position_list, wc->batch_id);
//        wc->chart_sequence_id = last_sequence;
//        wc->chart_timestamp = last_timestamp;
//    }

fail_complete:
    buffer_free(sql);
#endif

    return;
}

void aclk_get_chart_config(char **hash_id)
{
    if (unlikely(!hash_id))
        return;

    struct aclk_database_worker_config *wc = (struct aclk_database_worker_config *) localhost->dbsync_worker;

    if (unlikely(!wc)) {
        return;
    }

    struct aclk_database_cmd cmd;
    cmd.opcode = ACLK_DATABASE_PUSH_CHART_CONFIG;

    for (int i=0; hash_id[i]; ++i) {
        info("Request for chart config %d -- %s received", i, hash_id[i]);
        cmd.data_param = (void *) strdupz(hash_id[i]);
        cmd.count = 1;
        cmd.completion = NULL;
        aclk_database_enq_cmd(wc, &cmd);
    }

    return;
}

int aclk_push_chart_config_event(struct aclk_database_worker_config *wc, struct aclk_database_cmd cmd)
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

    char *hash_id = (char *) cmd.data_param;
    BUFFER *sql = buffer_create(1024);
    buffer_sprintf(sql, "SELECT type, family, context, title, priority, plugin, module, unit, chart_type " \
                        "FROM chart_hash WHERE hash_id = @hash_id;");

    rc = sqlite3_prepare_v2(db_meta, buffer_tostring(sql), -1, &res, 0);
    if (rc != SQLITE_OK) {
        error_report("Failed to prepare statement when trying to fetch a chart hash configuration");
        goto fail;
    }

    uuid_t hash_uuid;
    uuid_parse(hash_id, hash_uuid);
    rc = sqlite3_bind_blob(res, 1, &hash_uuid , sizeof(hash_uuid), SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    struct chart_config_updated chart_config;

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

    debug(D_ACLK_SYNC,"Sending chart config for %s", hash_id);

    aclk_chart_config_updated(&chart_config, 1);
    destroy_chart_config_updated(&chart_config);
    freez((char *) cmd.data_param);

bind_fail:
    rc = sqlite3_finalize(res);
    if (unlikely(rc != SQLITE_OK))
        error_report("Failed to reset statement when pushing chart config hash, rc = %d", rc);

fail:
    buffer_free(sql);

    return rc;
}

void sql_set_chart_ack(struct aclk_database_worker_config *wc, struct aclk_database_cmd cmd)
{
    int rc;
    sqlite3_stmt *res = NULL;

    BUFFER *sql = buffer_create(1024);

    buffer_sprintf(sql, "DELETE FROM aclk_chart_%s WHERE sequence_id <= @sequence_id AND date_submitted IS NOT NULL;",
                   wc->uuid_str);
    rc = sqlite3_prepare_v2(db_meta, buffer_tostring(sql), -1, &res, 0);
    if (rc != SQLITE_OK) {
        error_report("Failed to prepare statement count sequence ids in the database");
        goto prepare_fail;
    }

    rc = sqlite3_bind_int64(res, 1, (uint64_t) cmd.param1);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    rc = execute_insert(res);
    if (rc)
        error_report("Failed to delete sequence ids, rc = %d", rc);

bind_fail:
    rc = sqlite3_finalize(res);
    if (unlikely(rc != SQLITE_OK))
        error_report("Failed to finalize statement to delete older sequence ids, rc = %d", rc);

prepare_fail:
    buffer_free(sql);

    return;
}


void aclk_submit_param_command(char *node_id, enum aclk_database_opcode aclk_command, uint64_t param)
{
    if (unlikely(!node_id))
        return;

    struct aclk_database_worker_config *wc  = NULL;
    rrd_wrlock();
    RRDHOST *host = find_host_by_node_id(node_id);
    if (likely(host)) {
        wc = (struct aclk_database_worker_config *)host->dbsync_worker;
        rrd_unlock();
        if (likely(wc)) {
            struct aclk_database_cmd cmd;
            cmd.opcode = aclk_command;
            cmd.param1 = param;
            cmd.completion = NULL;
            aclk_database_enq_cmd(wc, &cmd);
        } else
            error("ACLK synchronization thread is not active for host %s", host->hostname);
    }
    else
        rrd_unlock();
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

void sql_chart_deduplicate(struct aclk_database_worker_config *wc, struct aclk_database_cmd cmd)
{
    UNUSED(cmd);

    BUFFER *sql = buffer_create(1024);

    buffer_sprintf(sql, "DROP TABLE IF EXISTS t_%s;", wc->uuid_str);
    db_execute(buffer_tostring(sql));
    buffer_reset(sql);

    buffer_sprintf(sql, "CREATE TABLE t_%s AS SELECT * FROM aclk_chart_payload_%s WHERE unique_id IN "
       "(SELECT unique_id from aclk_chart_%s WHERE date_submitted IS NULL);", wc->uuid_str, wc->uuid_str, wc->uuid_str);
    db_execute(buffer_tostring(sql));
    buffer_reset(sql);

    buffer_sprintf(sql, "DELETE FROM aclk_chart_payload_%s WHERE unique_id IN (SELECT unique_id FROM t_%s);",
                   wc->uuid_str, wc->uuid_str);
    db_execute(buffer_tostring(sql));
    buffer_reset(sql);

    buffer_sprintf(sql, "DELETE FROM aclk_chart_%s WHERE unique_id IN (SELECT unique_id FROM t_%s);",
                   wc->uuid_str, wc->uuid_str);
    db_execute(buffer_tostring(sql));
    buffer_reset(sql);

    buffer_sprintf(sql, "INSERT INTO aclk_chart_payload_%s SELECT * FROM t_%s ORDER BY DATE_CREATED ASC;",
                   wc->uuid_str, wc->uuid_str);
    db_execute(buffer_tostring(sql));
    buffer_reset(sql);

    buffer_sprintf(sql, "DROP TABLE IF EXISTS t_%s;", wc->uuid_str);
    db_execute(buffer_tostring(sql));

    sql_get_last_chart_sequence(wc, cmd);

    buffer_free(sql);
    return;
}

void sql_get_last_chart_sequence(struct aclk_database_worker_config *wc, struct aclk_database_cmd cmd)
{
    UNUSED(cmd);

    BUFFER *sql = buffer_create(1024);

    buffer_sprintf(sql,"select ac.sequence_id, ac.date_created from aclk_chart_%s ac " \
                        "where ac.date_submitted is null order by ac.sequence_id asc limit 1;", wc->uuid_str);

    int rc;
    sqlite3_stmt *res = NULL;
    rc = sqlite3_prepare_v2(db_meta, buffer_tostring(sql), -1, &res, 0);
    if (rc != SQLITE_OK) {
        error_report("Failed to prepare statement to find last chart sequence id");
        goto fail;
    }

    while (sqlite3_step(res) == SQLITE_ROW) {
        wc->chart_sequence_id = (uint64_t) sqlite3_column_int64(res, 0);
        wc->chart_timestamp  = (time_t) sqlite3_column_int64(res, 1);
    }

    debug(D_ACLK_SYNC,"Chart %s reports last seq=%"PRIu64" t=%ld", wc->uuid_str,
          wc->chart_sequence_id, wc->chart_timestamp);

    rc = sqlite3_finalize(res);
    if (unlikely(rc != SQLITE_OK))
        error_report("Failed to reset statement when fetching chart sequence info, rc = %d", rc);

fail:
    buffer_free(sql);
    return;
}

void sql_build_node_info(struct aclk_database_worker_config *wc, struct aclk_database_cmd cmd)
{
    UNUSED(cmd);

    struct update_node_info node_info;

    rrd_wrlock();
    node_info.node_id = get_str_from_uuid(wc->host->node_id);
    node_info.claim_id = is_agent_claimed();
    node_info.machine_guid = strdupz(wc->host_guid);
    node_info.child = (wc->host != localhost);
    now_realtime_timeval(&node_info.updated_at);

    RRDHOST *host = wc->host;

    node_info.data.name = strdupz(host->hostname);
    node_info.data.os = strdupz(host->os);
    node_info.data.os_name = strdupz(host->system_info->host_os_name);
    node_info.data.os_version = strdupz(host->system_info->host_os_version);
    node_info.data.kernel_name = strdupz(host->system_info->kernel_name);
    node_info.data.kernel_version = strdupz(host->system_info->kernel_version);
    node_info.data.architecture = strdupz(host->system_info->architecture);
    node_info.data.cpus = str2uint32_t(host->system_info->host_cores);
    node_info.data.cpu_frequency = strdupz(host->system_info->host_cpu_freq);
    node_info.data.memory = strdupz(host->system_info->host_ram_total);
    node_info.data.disk_space = strdupz(host->system_info->host_disk_space);
    node_info.data.version = strdupz(VERSION);
    node_info.data.release_channel = strdupz("nightly");
    node_info.data.timezone = strdupz("UTC");
    node_info.data.virtualization_type = strdupz(host->system_info->virtualization);
    node_info.data.container_type = strdupz(host->system_info->container);
    node_info.data.custom_info = config_get(CONFIG_SECTION_WEB, "custom dashboard_info.js", "");
    node_info.data.services = NULL;   // char **
    node_info.data.service_count = 0;
    node_info.data.machine_guid = strdupz(wc->host_guid);
    node_info.data.host_labels_head = NULL;     //struct label *host_labels_head;

    rrd_unlock();
    aclk_update_node_info(&node_info);
    return;
}

#define SELECT_HOST_DIMENSION_LIST  "SELECT d.dim_id, c.update_every FROM chart c, dimension d, host h " \
        "where d.chart_id = c.chart_id and c.host_id = h.host_id and c.host_id = @host_id order by c.update_every;"

void sql_update_metric_statistics(struct aclk_database_worker_config *wc, struct aclk_database_cmd cmd)
{
    UNUSED(cmd);
#ifdef ENABLE_DBENGINE
    int rc;

    sqlite3_stmt *res = NULL;

    RRDHOST *host = wc->host;

    rc = sqlite3_prepare_v2(db_meta, SELECT_HOST_DIMENSION_LIST, -1, &res, 0);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to prepare statement to fetch host dimensions");
        return;
    }

    rc = sqlite3_bind_blob(res, 1, &host->host_uuid, sizeof(host->host_uuid), SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to bind host parameter to fetch host dimensions");
        goto failed;
    }

    time_t  start_time = LONG_MAX;
    time_t  first_entry_t;
    time_t  last_entry_t;
    int update_every = 0;

    while (sqlite3_step(res) == SQLITE_ROW) {
        if (!update_every || update_every != sqlite3_column_int(res, 1)) {
            if (update_every)
                debug(D_ACLK_SYNC,"Update %s for %d oldest time = %ld", host->machine_guid, update_every, start_time);
            update_every = sqlite3_column_int(res, 1);
            start_time = LONG_MAX;
        }
        rrdeng_metric_latest_time_by_uuid((uuid_t *)sqlite3_column_blob(res, 0), &first_entry_t, &last_entry_t);
        start_time = MIN(start_time, first_entry_t);
    }
    if (update_every)
        debug(D_ACLK_SYNC,"Update %s for %d oldest time = %ld", host->machine_guid, update_every, start_time);

failed:
    rc = sqlite3_finalize(res);
    if (unlikely(rc != SQLITE_OK))
        error_report("Failed to finalize the prepared statement when reading archived charts");
#else
    UNUSED(wc);
#endif
    return;
}

int aclk_add_alert_event(struct aclk_database_worker_config *wc, struct aclk_database_cmd cmd)
{
    int rc = 0;
    if (unlikely(!db_meta)) {
        freez(cmd.data_param);
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
        error_report("Failed to store alert event, rc = %d", rc);

bind_fail:
    if (unlikely(sqlite3_finalize(res_alert) != SQLITE_OK))
        error_report("Failed to reset statement in store chart payload, rc = %d", rc);
    buffer_free(sql);
    return (rc != SQLITE_DONE);

    freez(cmd.data);
    return rc;
}
