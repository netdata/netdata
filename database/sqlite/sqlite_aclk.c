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

//#include "sqlite_event_loop.h"
//#include "sqlite_functions.h"

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
    debug(D_METADATALOG, "%s called, active=%d.", __func__, uv_is_active((uv_handle_t *)handle));
}

/* Flushes dirty pages when timer expires */
#define TIMER_PERIOD_MS (5000)

static void timer_cb(uv_timer_t* handle)
{
    struct aclk_database_worker_config *wc = handle->data;
    uv_stop(handle->loop);
    uv_update_time(handle->loop);

    struct aclk_database_cmd cmd;
    cmd.opcode = ACLK_DATABASE_TIMER;
    aclk_database_enq_cmd(wc, &cmd);

    if (wc->chart_updates) {
        cmd.opcode = ACLK_DATABASE_PUSH_CHART;
        cmd.count = 2;
        aclk_database_enq_cmd(wc, &cmd);
    }
    if (wc->alert_updates) {
        cmd.opcode = ACLK_DATABASE_PUSH_ALERT;
        cmd.count = 1;
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
    uv_thread_set_name_np(wc->thread, "Test");

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

    // RUn a chart deduplication
    cmd.opcode = ACLK_DATABASE_DEDUP_CHART;
    aclk_database_enq_cmd(wc, &cmd);

    info(
        "Starting ACLK sync event loop for host with GUID %s (Host is '%s')",
        wc->host_guid,
        wc->host ? "connected" : "not connected");
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
                    //sql_maint_database();
                    info("Cleanup for %s", wc->uuid_str);
                    break;
                case ACLK_DATABASE_FETCH_CHART:
                    // Fetch one or more charts
                    info("Fetching one chart!!!!!");
                    aclk_fetch_chart_event(wc, cmd);
                    //aclk_add_chart_event((RRDSET *) cmd.data, (char *) cmd.data_param, cmd.completion);
                    //freez(cmd.data_param);
                    break;
                case ACLK_DATABASE_PUSH_CHART:
                    // Fetch one or more charts
                    info("Pushing chart info to the cloud for node %s", wc->uuid_str);
                    aclk_push_chart_event(wc, cmd);
                    //aclk_add_chart_event((RRDSET *) cmd.data, (char *) cmd.data_param, cmd.completion);
                    //freez(cmd.data_param);
                    break;
                case ACLK_DATABASE_PUSH_CHART_CONFIG:
                    // Fetch one or more charts
                    info("Pushing chart config info to the cloud for node %s", wc->uuid_str);
                    aclk_push_chart_config_event(wc, cmd);
                    //aclk_add_chart_event((RRDSET *) cmd.data, (char *) cmd.data_param, cmd.completion);
                    //freez(cmd.data_param);
                    break;
                case ACLK_DATABASE_CHART_ACK:
                    // Fetch one or more charts
                    info("Setting last chart sequence ACK");
                    sql_set_chart_ack(wc, cmd);
                    //aclk_add_chart_event((RRDSET *) cmd.data, (char *) cmd.data_param, cmd.completion);
                    //freez(cmd.data_param);
                    if (cmd.completion)
                        complete(cmd.completion);
                    break;
                case ACLK_DATABASE_RESET_CHART:
                    // Fetch one or more charts
                    info("Resetting chart to sequence id %d", cmd.count);
                    sql_reset_chart_event(wc, cmd);
                    if (cmd.completion)
                        complete(cmd.completion);
                    break;
                case ACLK_DATABASE_PUSH_ALERT:
                    // Fetch one or more charts
                    info("Pushing alert config to the cloud");
                    aclk_push_alert_event(wc, cmd);
                    //aclk_add_chart_event((RRDSET *) cmd.data, (char *) cmd.data_param, cmd.completion);
                    //freez(cmd.data_param);
                    break;
                case ACLK_DATABASE_RESET_NODE:
                    // Fetch one or more charts
                    info("Resetting the node instance id of host with guid %s", (char *) cmd.data);
                    aclk_reset_node_event(wc, cmd);
                    //aclk_add_chart_event((RRDSET *) cmd.data, (char *) cmd.data_param, cmd.completion);
                    //freez(cmd.data_param);
                    break;
                case ACLK_DATABASE_STATUS_CHART:
                    // Fetch one or more charts
                    info("Requesting chart status for host %s", wc->uuid_str);
                    aclk_status_chart_event(wc, cmd);
                    //aclk_add_chart_event((RRDSET *) cmd.data, (char *) cmd.data_param, cmd.completion);
                    //freez(cmd.data_param);
                    break;
                case ACLK_DATABASE_ADD_CHART:
                    aclk_add_chart_event((RRDSET *) cmd.data, (char *) cmd.data_param, cmd.completion);
                    freez(cmd.data_param);
                    break;
                case ACLK_DATABASE_ADD_ALARM:
                    aclk_add_alarm_event((RRDHOST *) cmd.data, (ALARM_ENTRY *) cmd.data1, (char *) cmd.data_param, cmd.completion);
                    freez(cmd.data_param);
                    break;
                case ACLK_DATABASE_TIMER:
                    //sql_maint_database();
                    //info("Cleanup for %s", wc->uuid_str);
                    if (unlikely(!wc->host)) {
                        wc->host = rrdhost_find_by_guid(wc->host_guid, 0);
                        if (wc->host) {
                            info("HOST %s detected as active !!!", wc->host->hostname);
                            wc->host->dbsync_worker = wc;
                        }
                    }
                    break;
                case ACLK_DATABASE_DEDUP_CHART:
                    sql_chart_deduplicate(wc, cmd);
                    break;
                case ACLK_DATABASE_SHUTDOWN:
                    shutdown = 1;
                    fatal_assert(0 == uv_timer_stop(&timer_req));
                    uv_close((uv_handle_t *)&timer_req, NULL);
                    if (cmd.completion)
                        complete(cmd.completion);
                    break;
                default:
                    debug(D_METADATALOG, "%s: default.", __func__);
                    break;
            }
            db_unlock();
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
    fatal_assert(0 == uv_loop_close(loop));
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

int aclk_add_chart_payload(char *uuid_str, uuid_t *unique_id, uuid_t *chart_id, char *payload_type, const char *payload, size_t payload_size)
{
    char sql[1024];
    sqlite3_stmt *res_chart = NULL;
    int rc;

    sprintf(sql,"insert into aclk_chart_payload_%s (unique_id, chart_id, date_created, type, payload) " \
                 "values (@unique_id, @chart_id, strftime('%%s'), @type, @payload);", uuid_str);

    rc = sqlite3_prepare_v2(db_meta, sql, -1, &res_chart, 0);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to prepare statement to store chart payload data");
        return 1;
    }

    rc = sqlite3_bind_blob(res_chart, 1, unique_id , sizeof(*unique_id), SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    rc = sqlite3_bind_blob(res_chart, 2, chart_id , sizeof(*chart_id), SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    rc = sqlite3_bind_text(res_chart, 3, payload_type ,-1, SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    rc = sqlite3_bind_blob(res_chart, 4, payload, payload_size, SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    rc = execute_insert(res_chart);
    if (unlikely(rc != SQLITE_DONE))
        error_report("Failed store chart payload event, rc = %d", rc);

bind_fail:
    if (unlikely(sqlite3_finalize(res_chart) != SQLITE_OK))
        error_report("Failed to reset statement in store chart payload, rc = %d", rc);

    return (rc != SQLITE_DONE);
}

int aclk_add_chart_event(RRDSET *st, char *payload_type, struct completion *completion)
{
    int rc = 0;
    if (unlikely(!db_meta)) {
        if (default_rrd_memory_mode != RRD_MEMORY_MODE_DBENGINE) {
            return 1;
        }
        error_report("Database has not been initialized");
        return 1;
    }

#ifdef ACLK_NG
    char *claim_id = is_agent_claimed();

    if (likely(claim_id)) {
        char uuid_str[GUID_LEN + 1];
        uuid_unparse_lower_fix(&st->rrdhost->host_uuid, uuid_str);

        uuid_t unique_uuid;
        uuid_generate(unique_uuid);

        struct chart_instance_updated chart_payload;
        memset(&chart_payload, 0, sizeof(chart_payload));
        chart_payload.config_hash = get_str_from_uuid(&st->state->hash_id);
        chart_payload.update_every = st->update_every;
        chart_payload.memory_mode = st->rrd_memory_mode;
        chart_payload.name = strdupz((char *)st->name);
        chart_payload.node_id = get_str_from_uuid(st->rrdhost->node_id);
        chart_payload.claim_id = claim_id;
        chart_payload.id = strdupz(st->id);

        size_t payload_size;
        char *payload = generate_chart_instance_updated(&payload_size, &chart_payload);
        rc = aclk_add_chart_payload(uuid_str, &unique_uuid, st->chart_uuid, payload_type, payload, payload_size);
        chart_instance_updated_destroy(&chart_payload);
        freez(payload);
    }
#else
    UNUSED(st);
    UNUSED(payload_type);
#endif
    if (completion)
       complete(completion);

    return rc;
}

void sql_reset_chart_event(struct aclk_database_worker_config *wc, struct aclk_database_cmd cmd)
{
    BUFFER *sql = buffer_create(1024);

    buffer_sprintf(sql, "update aclk_chart_%s set status = NULL, date_submitted = NULL where sequence_id >= %"PRIu64";",
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

    buffer_sprintf(sql, "update node_instance set node_id = null where host_id = @host_id;");

    sqlite3_stmt *res = NULL;
    rc = sqlite3_prepare_v2(db_meta, buffer_tostring(sql), -1, &res, 0);
    if (rc != SQLITE_OK) {
        error_report("Failed to prepare statement to lookup chart UUID in the database");
        goto fail;
    }

    rc = sqlite3_bind_blob(res, 1, &host_id , sizeof(host_id), SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    rc = execute_insert(res);
    if (unlikely(rc != SQLITE_DONE))
        error_report("Failed to reset the node instance id of host %s, rc = %d", (char *) cmd.data, rc);

bind_fail:
    rc = sqlite3_finalize(res);
    if (unlikely(rc != SQLITE_OK))
        error_report("Failed to reset statement when searching for a chart UUID, rc = %d", rc);

fail:
    buffer_free(sql);

fail1:
    if (cmd.completion)
        complete(cmd.completion);

    return;
}


void aclk_status_chart_event(struct aclk_database_worker_config *wc, struct aclk_database_cmd cmd)
{
    int rc;

    BUFFER *sql = buffer_create(1024);
    BUFFER *resp = buffer_create(1024);

    buffer_sprintf(sql, "select case when status is null and date_submitted is null then 'resync' " \
        "when status is null then 'submitted' else status end, " \
        "count(*), min(sequence_id), max(sequence_id) from " \
        "aclk_chart_%s group by 1;", wc->uuid_str);

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
    if (cmd.completion)
        complete(cmd.completion);

    return;
}

void aclk_fetch_chart_event(struct aclk_database_worker_config *wc, struct aclk_database_cmd cmd)
{
    int rc;

    int limit = cmd.count > 0 ? cmd.count : 1;
    int available = 0;
    long first_sequence = 0;
    long last_sequence  = 0;

    BUFFER *sql = buffer_create(1024);

    sqlite3_stmt *res = NULL;

    buffer_sprintf(sql, "select count(*) from aclk_chart_%s where status is null and date_submitted is null;",
                   wc->uuid_str);
    rc = sqlite3_prepare_v2(db_meta, buffer_tostring(sql), -1, &res, 0);
    if (rc != SQLITE_OK) {
        error_report("Failed to prepare statement count sequence ids in the database");
        goto fail;
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
        "acp.payload from aclk_chart_%s ac, aclk_chart_payload_%s acp " \
        "where (ac.status = 'processing' or (ac.status is NULL and ac.date_submitted is null)) " \
        "and ac.unique_id = acp.unique_id order by ac.sequence_id asc limit %d;",
                   wc->uuid_str, wc->uuid_str, wc->uuid_str, limit);

    info("%s",  buffer_tostring(sql));

    //sqlite3_stmt *res = NULL;

    rc = sqlite3_prepare_v2(db_meta, buffer_tostring(sql), -1, &res, 0);
    if (rc != SQLITE_OK) {
        error_report("Failed to prepare statement to get sequence id list for charts");
        goto fail;
    }

    struct aclk_chart_payload_t *head = NULL;
    struct aclk_chart_payload_t *tail = NULL;
    while (sqlite3_step(res) == SQLITE_ROW) {
        struct aclk_chart_payload_t *chart_payload = callocz(1, sizeof(*chart_payload));
        chart_payload->sequence_id = sqlite3_column_int64(res, 0);
        if (!first_sequence)
            first_sequence = chart_payload->sequence_id;
        if (sqlite3_column_bytes(res, 1) > 0)
            chart_payload->last_sequence_id = sqlite3_column_int64(res, 1);
        else
            chart_payload->last_sequence_id = 0;
        chart_payload->payload = sqlite3_column_bytes(res, 2) ? strdupz((char *)sqlite3_column_text(res, 2)) : NULL;
        if (!head) {
            head = chart_payload;
            tail = head;
        }
        else {
            tail->next = chart_payload;
            tail = chart_payload;
        }

        last_sequence = chart_payload->sequence_id;
    }
    *(struct aclk_chart_payload_t **) cmd.data = head;

    rc = sqlite3_finalize(res);
    if (unlikely(rc != SQLITE_OK))
        error_report("Failed to reset statement when searching for a chart UUID, rc = %d", rc);

    fail:
    buffer_flush(sql);
    buffer_sprintf(sql, "update aclk_chart_%s set status = NULL, date_submitted=strftime('%%s') " \
                        "where (status = 'processing' or (status is NULL and date_submitted is NULL)) "
                        "and sequence_id between %ld and %ld;", wc->uuid_str, first_sequence, last_sequence);
    db_execute(buffer_tostring(sql));

    buffer_free(sql);
    if (cmd.completion)
        complete(cmd.completion);

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
//    struct completion compl;
    //init_completion(&compl);
    //cmd.completion = &compl;
    //info("Adding %s", st->name);
    aclk_database_enq_cmd((struct aclk_database_worker_config *) st->rrdhost->dbsync_worker, &cmd);
    //wait_for_completion(&compl);
    //destroy_completion(&compl);
    //info("Adding %s done", st->name);

    return;
}

// Hosts that have tables
// select h2u(host_id), hostname from host h, sqlite_schema s where name = "aclk_" || replace(h2u(h.host_id),"-","_") and s.type = "table";

// One query thread per host (host_id, table name)
// Prepare read / write sql statements

// Load nodes on startup (ask those that do not have node id)
// Start thread event loop (R/W)
void sql_drop_host_aclk_table_list(uuid_t *host_uuid)
{
    int rc;
    char uuid_str[GUID_LEN + 1];

    uuid_unparse_lower_fix(host_uuid, uuid_str);

    BUFFER *sql = buffer_create(1024);
    buffer_sprintf(
        sql,"select 'drop '||type||' IF EXISTS '||name||';' from sqlite_schema " \
        "where name like 'aclk_%%_%s' and type in ('table', 'trigger', 'index');", uuid_str);

    sqlite3_stmt *res = NULL;

    info("DEBUG: %s",  buffer_tostring(sql));

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

//    if (unlikely(!host))
//        host = rrdhost_find_by_guid(host_guid, 0);

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

    buffer_sprintf(sql, TABLE_ACLK_DIMENSION, uuid_str);
    db_execute(buffer_tostring(sql));
    buffer_flush(sql);

    buffer_sprintf(sql, TABLE_ACLK_DIMENSION_PAYLOAD, uuid_str);
    db_execute(buffer_tostring(sql));
    buffer_flush(sql);

    buffer_sprintf(sql,TRIGGER_ACLK_DIMENSION_PAYLOAD, uuid_str);
    db_execute(buffer_tostring(sql));
    buffer_flush(sql);

    buffer_sprintf(sql,TABLE_ACLK_ALERT, uuid_str);
    db_execute(buffer_tostring(sql));
    buffer_flush(sql);

    buffer_sprintf(sql,TABLE_ACLK_ALERT_PAYLOAD, uuid_str);
    db_execute(buffer_tostring(sql));

    buffer_sprintf(sql,TRIGGER_ACLK_ALERT_PAYLOAD, uuid_str, uuid_str, uuid_str);
    db_execute(buffer_tostring(sql));
    buffer_flush(sql);


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
    buffer_strcat(sql, "select host_id from host;");
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
void aclk_start_streaming(char *node_id)
{
    if (unlikely(!node_id))
        return;

    info("START streaming charts for %s received", node_id);
    uuid_t node_uuid;
    uuid_parse(node_id, node_uuid);

    struct aclk_database_worker_config *wc  = NULL;
    rrd_wrlock();
    RRDHOST *host = localhost;
    while(host) {
        if (host->node_id && !(uuid_compare(*host->node_id, node_uuid))) {
            wc = (struct aclk_database_worker_config *)host->dbsync_worker;
            if (likely(wc)) {
                wc->chart_updates = 1;
                info("START streaming charts for %s enabled", node_id);
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
                info("START streaming alerts for %s enabled", node_id);
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


/*
 * struct aclk_message_position {
    uint64_t sequence_id;
    struct timeval seq_id_creation_time;
    uint64_t previous_sequence_id;
};
struct chart_instance_updated {
    const char *id;
    const char *claim_id;
    const char *node_id;
    const char *name;
    struct label *label_head;
    RRD_MEMORY_MODE memory_mode;
    uint32_t update_every;
    const char * config_hash;
    struct aclk_message_position position;
};
struct chart_dimension_updated {
    const char *id;
    const char *chart_id;
    const char *node_id;
    const char *claim_id;
    const char *name;
    struct timeval created_at;
    struct timeval last_timestamp;
    struct aclk_message_position position;
};
typedef struct {
    struct chart_instance_updated *charts;
    uint16_t chart_count;
    struct chart_dimension_updated *dims;
    uint16_t dim_count;
    uint64_t batch_id;
} charts_and_dims_updated_t;
 */
int aclk_add_alarm_payload(char *uuid_str, uuid_t *unique_id, uint32_t ae_unique_id, uint32_t alarm_id, char *payload_type, const char *payload, size_t payload_size)
{
    char sql[1024];
    sqlite3_stmt *res_chart = NULL;
    int rc;

    sprintf(sql,"insert into aclk_alert_payload_%s (unique_id, ae_unique_id, alarm_id, date_created, type, payload) " \
                 "values (@unique_id, @ae_unique_id, @alarm_id, strftime('%%s'), @type, @payload);", uuid_str);

    rc = sqlite3_prepare_v2(db_meta, sql, -1, &res_chart, 0);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to prepare statement to store alert payload data [%s]", sql);
        return 1;
    }

    rc = sqlite3_bind_blob(res_chart, 1, unique_id , sizeof(*unique_id), SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    rc = sqlite3_bind_int(res_chart, 2, ae_unique_id);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    rc = sqlite3_bind_int(res_chart, 3, alarm_id);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    rc = sqlite3_bind_text(res_chart, 4, payload_type ,-1, SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    rc = sqlite3_bind_text(res_chart, 5, payload, payload_size, SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    rc = execute_insert(res_chart);
    if (unlikely(rc != SQLITE_DONE))
        error_report("Failed store chart payload event, rc = %d", rc);

bind_fail:
    if (unlikely(sqlite3_finalize(res_chart) != SQLITE_OK))
        error_report("Failed to reset statement in store dimension, rc = %d", rc);

    return (rc != SQLITE_DONE);
}


int aclk_add_alarm_event(RRDHOST *host, ALARM_ENTRY *ae, char *payload_type, struct completion *completion)
{
    char uuid_str[GUID_LEN + 1];
    int rc;

    if (unlikely(!db_meta)) {
        if (default_rrd_memory_mode != RRD_MEMORY_MODE_DBENGINE) {
            return 1;
        }
        error_report("Database has not been initialized");
        return 1;
    }

    uuid_unparse_lower_fix(&host->host_uuid, uuid_str);

    uuid_t unique_uuid;
    uuid_generate(unique_uuid);
    BUFFER *tmp_buffer = NULL;
    tmp_buffer = buffer_create(4096);
    health_alarm_entry_sql2json(tmp_buffer, ae->unique_id, ae->alarm_id, host);

    rc = aclk_add_alarm_payload(
                                uuid_str, &unique_uuid, ae->unique_id, ae->alarm_id, payload_type, buffer_tostring(tmp_buffer), strlen(buffer_tostring(tmp_buffer)));

    buffer_free(tmp_buffer);
    //info("Added %s completed", st->name);

    if (completion)
       complete(completion);

    return rc;
}

void sql_queue_alarm_to_aclk(RRDHOST *host, ALARM_ENTRY *ae)
{
    if (!aclk_architecture)
        aclk_update_alarm(host, ae);

    if (unlikely(!host->dbsync_worker))
        return;

    struct aclk_database_cmd cmd;
    cmd.opcode = ACLK_DATABASE_ADD_ALARM;
    cmd.data = host;
    cmd.data1 = ae;
    cmd.data_param = strdupz("JSON");
    cmd.completion = NULL;
//    struct completion compl;
    //init_completion(&compl);
    //cmd.completion = &compl;
    //info("Adding %s", st->name);
    aclk_database_enq_cmd((struct aclk_database_worker_config *) host->dbsync_worker, &cmd);
    //wait_for_completion(&compl);
    //destroy_completion(&compl);
    //info("Adding %s done", st->name);

    return;
}

int aclk_add_dimension_payload(char *uuid_str, uuid_t *unique_id, uuid_t *dim_id, char *payload_type, const char *payload, size_t payload_size)
{
    char sql[1024];
    sqlite3_stmt *res_chart = NULL;
    int rc;

    sprintf(sql,"insert into aclk_dimension_payload_%s (unique_id, dim_id, date_created, type, payload) " \
                 "values (@unique_id, @dim_id, strftime('%%s'), @type, @payload);", uuid_str);

    rc = sqlite3_prepare_v2(db_meta, sql, -1, &res_chart, 0);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to prepare statement to store chart payload data");
        return 1;
    }

    rc = sqlite3_bind_blob(res_chart, 1, unique_id , sizeof(*unique_id), SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    rc = sqlite3_bind_blob(res_chart, 2, dim_id , sizeof(*dim_id), SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    rc = sqlite3_bind_text(res_chart, 3, payload_type ,-1, SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    rc = sqlite3_bind_text(res_chart, 4, payload, payload_size, SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    rc = execute_insert(res_chart);
    if (unlikely(rc != SQLITE_DONE))
        error_report("Failed store chart payload event, rc = %d", rc);

    bind_fail:
    if (unlikely(sqlite3_finalize(res_chart) != SQLITE_OK))
        error_report("Failed to reset statement in store dimension, rc = %d", rc);

    return (rc != SQLITE_DONE);
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
//       info("DEBUG: Dimension %d (chart %s) --> %s live status = %d", i, rd->rrdset->name, rd->name, dim_payload.last_timestamp.tv_sec ? 0 :1);
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

    rc = sqlite3_prepare_v2(db_meta,  "select type||'.'||id from chart where chart_id = @chart_id;", -1, &res, 0);
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
    long first_sequence = 0;
    long last_sequence  = 0;
//    size_t  *dim_size = NULL;
    size_t total_dimension_count = 0;
//    char **dimension_payload_list = NULL;
//    size_t current_size = 0;

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
        "acp.payload, ac.date_created, ac.chart_id " \
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

    int count = 0;
    while (sqlite3_step(res) == SQLITE_ROW) {
        size_t  payload_size = sqlite3_column_bytes(res, 2);
        payload_list_size[count] = payload_size;
        payload_list[count] = mallocz(payload_size);
        memcpy(payload_list[count], sqlite3_column_blob(res, 2), payload_size);
        position_list[count].sequence_id = sqlite3_column_int64(res, 0);
        position_list[count].previous_sequence_id = sqlite3_column_bytes(res, 1) > 0 ? sqlite3_column_int64(res, 1) : 0;
        position_list[count].seq_id_creation_time.tv_sec = sqlite3_column_int64(res, 3);
        position_list[count].seq_id_creation_time.tv_usec = 0;
        if (!first_sequence)
            first_sequence = position_list[count].sequence_id;
        last_sequence = position_list[count].sequence_id;

        RRDSET *st = find_rrdset_by_uuid(wc->host, (uuid_t *) sqlite3_column_blob(res, 4));
        if (st) {
            rrdset_rdlock(st);
            tmp_dim = callocz(1, sizeof(*tmp_dim));
            tmp_dim->position_index = count;
            tmp_dim->dimension_list = build_dimension_payload_list(st, &tmp_dim->dim_size, &tmp_dim->count);
            tmp_dim->next = dim_head;
            dim_head = tmp_dim;
            total_dimension_count += tmp_dim->count;
            rrdset_unlock(st);
        }
        count++;

    }

    rc = sqlite3_finalize(res);
    if (unlikely(rc != SQLITE_OK))
        error_report("Failed to reset statement when pushing chart events, rc = %d", rc);

fail:
    buffer_flush(sql);
    buffer_sprintf(sql, "update aclk_chart_%s set status = NULL, date_submitted=strftime('%%s') " \
                        "where (status = 'processing' or (status is NULL and date_submitted is NULL)) "
                        "and sequence_id between %ld and %ld;", wc->uuid_str, first_sequence, last_sequence);
    db_execute(buffer_tostring(sql));

    if (payload_list) {
       payload_list = realloc(payload_list, (limit + total_dimension_count + 1) * sizeof(char *));
       payload_list_size = realloc(payload_list_size, (limit + total_dimension_count + 1) * sizeof(size_t));
       position_list = realloc(position_list, (limit + total_dimension_count + 1) *  sizeof(*position_list));
//       struct aclk_message_position *position_list =  callocz(limit+1, sizeof(*position_list));
       payload_list[limit + total_dimension_count] = NULL;
       int *is_dim = callocz(limit + total_dimension_count + 1, sizeof(*is_dim));

        //aclk_chart_inst_update(payload_list, payload_list_size, position_list);

//        int pos = limit;
        while (dim_head) {
            tmp_dim = dim_head->next;
            memcpy(payload_list + count, dim_head->dimension_list, dim_head->count * sizeof(char *));
            memcpy(payload_list_size + count, dim_head->dim_size, dim_head->count * sizeof(size_t));
            for (size_t i = 0; i < dim_head->count; ++i) {
                position_list[count + i] = position_list[dim_head->position_index];
                is_dim[count + i] = 1;
            }
            count += dim_head->count;
            freez(dim_head->dimension_list);
            freez(dim_head->dim_size);
            freez(dim_head);
            dim_head = tmp_dim;
        }
//        for (int i = 0; payload_list && payload_list[i]; ++i) {
//                info("DEBUG2: PAYLOAD %d -- payload size = %u    IS DIM = %d  SEQ=%"PRIu64"  LASTSEQ=%"PRIu64, i, payload_list_size[i], is_dim[i],
//                    position_list[i].sequence_id, position_list[i].previous_sequence_id);
//        }
       // char **dim_list = build_dimension_payload_list(*st, &dim_size_list);

//        void aclk_chart_inst_and_dim_update(char **payloads, size_t *payload_sizes, int *is_dim, struct aclk_message_position *new_positions)
        aclk_chart_inst_and_dim_update(payload_list, payload_list_size, is_dim, position_list);

    }

fail_complete:
    buffer_free(sql);
#endif

    return;
}

// Start streaming charts / dimensions for node_id
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
        aclk_database_enq_cmd(wc, &cmd);
    }

    return;
}

int aclk_push_chart_config_event(struct aclk_database_worker_config *wc, struct aclk_database_cmd cmd)
{
    int rc = 0;
    if (unlikely(!db_meta)) {
        if (default_rrd_memory_mode != RRD_MEMORY_MODE_DBENGINE) {
            return 1;
        }
        error_report("Database has not been initialized");
        return 1;
    }

    UNUSED(wc);

    char *hash_id = (char *) cmd.data_param;

    BUFFER *sql = buffer_create(1024);

    sqlite3_stmt *res = NULL;

    buffer_sprintf(sql, "select type, family, context, title, priority, plugin, module, unit, chart_type from chart_hash where hash_id = @hash_id;");

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

    info("SENDING CHART HASH CONFIG for %s", hash_id);

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

    BUFFER *sql = buffer_create(1024);

    sqlite3_stmt *res = NULL;

    buffer_sprintf(sql, "delete from aclk_chart_%s where sequence_id < @sequence_id and date_submitted is not null;",
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

    info("NODE %s reports last sequence id received %"PRIu64 , node_id, last_sequence_id);
    aclk_submit_param_command(node_id, ACLK_DATABASE_CHART_ACK, last_sequence_id);
    return;
}

void aclk_reset_chart_event(char *node_id, uint64_t last_sequence_id)
{
    if (unlikely(!node_id))
        return;

    info("NODE %s wants to resync from %"PRIu64 , node_id, last_sequence_id);
    aclk_submit_param_command(node_id, ACLK_DATABASE_RESET_CHART, last_sequence_id);
    return;
}


void aclk_push_alert_event(struct aclk_database_worker_config *wc, struct aclk_database_cmd cmd)
{

#ifndef ACLK_NG
    UNUSED (wc);
    UNUSED(cmd);
#else
    int rc;

    int limit = cmd.count > 0 ? cmd.count : 1;
    int available = 0;
    long first_sequence = 0;
    long last_sequence  = 0;
//    size_t  *dim_size = NULL;
    size_t total_dimension_count = 0;
//    char **dimension_payload_list = NULL;
//    size_t current_size = 0;

    struct dim_list {
        char **dimension_list;
        size_t *dim_size;
        size_t count;
        size_t position_index;
        struct dim_list *next;
    } *dim_head = NULL, *tmp_dim;

    BUFFER *sql = buffer_create(1024);

    sqlite3_stmt *res = NULL;

    buffer_sprintf(sql, "select count(*) from aclk_alert_%s where status is null and date_submitted is null;",
                   wc->uuid_str);
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

    info("Alerts available %d limit = %d", available, limit);

    return;

//    if (limit > available) {
//        limit = limit - available;
//
//        buffer_sprintf(sql, "update aclk_chart_%s set status = 'processing' where status = 'pending' "
//                            "order by sequence_id limit %d;", wc->uuid_str, limit);
//        db_execute(buffer_tostring(sql));
//        buffer_flush(sql);
//    }
//
//    buffer_sprintf(sql, "select ac.sequence_id, (select sequence_id from aclk_alert_%s " \
//        "lac where lac.sequence_id < ac.sequence_id and (status is NULL or status = 'processing')  " \
//        "order by lac.sequence_id desc limit 1), " \
//        "acp.payload, ac.date_created, ac.chart_id " \
//        "from aclk_alert_%s ac, " \
//        "aclk_chart_payload_%s acp " \
//        "where (ac.status = 'processing' or (ac.status is NULL and ac.date_submitted is null)) " \
//        "and ac.unique_id = acp.unique_id " \
//        "order by ac.sequence_id asc limit %d;",
//                   wc->uuid_str, wc->uuid_str, wc->uuid_str, limit);
//
//    rc = sqlite3_prepare_v2(db_meta, buffer_tostring(sql), -1, &res, 0);
//    if (rc != SQLITE_OK) {
//        error_report("Failed to prepare statement when trying to send a chart update via ACLK");
//        goto fail;
//    }
//
//    char **payload_list = callocz(limit+1, sizeof(char *));
//    size_t *payload_list_size = callocz(limit+1, sizeof(size_t));
//    struct aclk_message_position *position_list =  callocz(limit+1, sizeof(*position_list));
//
//    int count = 0;
//    while (sqlite3_step(res) == SQLITE_ROW) {
//        size_t  payload_size = sqlite3_column_bytes(res, 2);
//        payload_list_size[count] = payload_size;
//        payload_list[count] = mallocz(payload_size);
//        memcpy(payload_list[count], sqlite3_column_blob(res, 2), payload_size);
//        position_list[count].sequence_id = sqlite3_column_int64(res, 0);
//        position_list[count].previous_sequence_id = sqlite3_column_bytes(res, 1) > 0 ? sqlite3_column_int64(res, 1) : 0;
//        position_list[count].seq_id_creation_time.tv_sec = sqlite3_column_int64(res, 3);
//        position_list[count].seq_id_creation_time.tv_usec = 0;
//        if (!first_sequence)
//            first_sequence = position_list[count].sequence_id;
//        last_sequence = position_list[count].sequence_id;
//
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
//        count++;
//
//    }
//
//    rc = sqlite3_finalize(res);
//    if (unlikely(rc != SQLITE_OK))
//        error_report("Failed to reset statement when pushing chart events, rc = %d", rc);
//
//    fail:
//    buffer_flush(sql);
//    buffer_sprintf(sql, "update aclk_chart_%s set status = NULL, date_submitted=strftime('%%s') " \
//                        "where (status = 'processing' or (status is NULL and date_submitted is NULL)) "
//                        "and sequence_id between %ld and %ld;", wc->uuid_str, first_sequence, last_sequence);
//    db_execute(buffer_tostring(sql));
//
//    if (payload_list) {
//        payload_list = realloc(payload_list, (limit + total_dimension_count + 1) * sizeof(char *));
//        payload_list_size = realloc(payload_list_size, (limit + total_dimension_count + 1) * sizeof(size_t));
//        position_list = realloc(position_list, (limit + total_dimension_count + 1) *  sizeof(*position_list));
////       struct aclk_message_position *position_list =  callocz(limit+1, sizeof(*position_list));
//        payload_list[limit + total_dimension_count] = NULL;
//        int *is_dim = callocz(limit + total_dimension_count + 1, sizeof(*is_dim));
//
//        //aclk_chart_inst_update(payload_list, payload_list_size, position_list);
//
////        int pos = limit;
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
////        for (int i = 0; payload_list && payload_list[i]; ++i) {
////                info("DEBUG2: PAYLOAD %d -- payload size = %u    IS DIM = %d  SEQ=%"PRIu64"  LASTSEQ=%"PRIu64, i, payload_list_size[i], is_dim[i],
////                    position_list[i].sequence_id, position_list[i].previous_sequence_id);
////        }
//        // char **dim_list = build_dimension_payload_list(*st, &dim_size_list);
//
////        void aclk_chart_inst_and_dim_update(char **payloads, size_t *payload_sizes, int *is_dim, struct aclk_message_position *new_positions)
//        aclk_chart_inst_and_dim_update(payload_list, payload_list_size, is_dim, position_list);
//
//    }
//
fail_complete:
        buffer_free(sql);
#endif

    return;
}


void sql_chart_deduplicate(struct aclk_database_worker_config *wc, struct aclk_database_cmd cmd)
{
    UNUSED(cmd);

    BUFFER *sql = buffer_create(1024);

//    buffer_sprintf(sql, "DROP TABLE IF EXISTS t_%s; "
//                        "CREATE TABLE t_%s AS SELECT * FROM aclk_chart_payload_%s WHERE unique_id IN "
//                        "(SELECT unique_id from aclk_chart_%s WHERE date_submitted IS NULL); "
//                        "DELETE FROM aclk_chart_payload_%s WHERE "
//                        "unique_id IN (SELECT unique_id FROM t_%s); "
//                        "DELETE FROM aclk_chart_%s WHERE unique_id IN (SELECT unique_id FROM t_%s); "
//                        "INSERT INTO aclk_chart_payload_%s SELECT * FROM t_%s ORDER BY DATE_CREATED ASC; "
//                        "DROP TABLE IF EXISTS t_%s;",
//                       wc->uuid_str, wc->uuid_str, wc->uuid_str, wc->uuid_str, wc->uuid_str,
//                       wc->uuid_str, wc->uuid_str, wc->uuid_str, wc->uuid_str, wc->uuid_str, wc->uuid_str);

    buffer_sprintf(sql, "DROP TABLE IF EXISTS t_%s;", wc->uuid_str);
    db_execute(buffer_tostring(sql));
    buffer_reset(sql);

    buffer_sprintf(sql, "CREATE TABLE t_%s AS SELECT * FROM aclk_chart_payload_%s WHERE unique_id IN (SELECT unique_id from aclk_chart_%s WHERE date_submitted IS NULL);", wc->uuid_str, wc->uuid_str, wc->uuid_str);
    db_execute(buffer_tostring(sql));
    buffer_reset(sql);

    buffer_sprintf(sql, "DELETE FROM aclk_chart_payload_%s WHERE unique_id IN (SELECT unique_id FROM t_%s);", wc->uuid_str, wc->uuid_str);
    db_execute(buffer_tostring(sql));
    buffer_reset(sql);

    buffer_sprintf(sql, "DELETE FROM aclk_chart_%s WHERE unique_id IN (SELECT unique_id FROM t_%s);", wc->uuid_str, wc->uuid_str);
    db_execute(buffer_tostring(sql));
    buffer_reset(sql);

    buffer_sprintf(sql, "INSERT INTO aclk_chart_payload_%s SELECT * FROM t_%s ORDER BY DATE_CREATED ASC;", wc->uuid_str, wc->uuid_str);
    db_execute(buffer_tostring(sql));
    buffer_reset(sql);

    buffer_sprintf(sql, "DROP TABLE IF EXISTS t_%s;", wc->uuid_str);
    db_execute(buffer_tostring(sql));

    buffer_free(sql);
    return;
}
