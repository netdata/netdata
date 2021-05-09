// SPDX-License-Identifier: GPL-3.0-or-later

#include "sqlite_functions.h"
#include "sqlite_aclk.h"

#ifndef ACLK_NG
#include "../../aclk/legacy/agent_cloud_link.h"
#else
#include "../../aclk/aclk.h"
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
                case ACLK_DATABASE_RESET_CHART:
                    // Fetch one or more charts
                    info("Resetting chart to  sequence id %d", cmd.count);
                    aclk_reset_chart_event(wc, cmd);
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
                case ACLK_DATABASE_TIMER:
                    //sql_maint_database();
                    //info("Cleanup for %s", wc->uuid_str);
                    break;
                case ACLK_DATABASE_SHUTDOWN:
                    shutdown = 1;
                    break;
                default:
                    debug(D_METADATALOG, "%s: default.", __func__);
                    break;
            }
        } while (opcode != ACLK_DATABASE_NOOP);
    }

    /* cleanup operations of the event loop */
    info("Shutting down ACLK_DATABASE  engine event loop.");

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


int aclk_add_chart_event(RRDSET *st, char *payload_type, struct completion *completion)
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

    uuid_unparse_lower(st->rrdhost->host_uuid, uuid_str);
    uuid_str[8] = '_';
    uuid_str[13] = '_';
    uuid_str[18] = '_';
    uuid_str[23] = '_';

    uuid_t unique_uuid;
    uuid_generate(unique_uuid);
    BUFFER *tmp_buffer = NULL;
    tmp_buffer = buffer_create(4096);
    rrdset2json(st, tmp_buffer, NULL, NULL, 1);

    rc = aclk_add_chart_payload(
        uuid_str, &unique_uuid, st->chart_uuid, payload_type, buffer_tostring(tmp_buffer), strlen(buffer_tostring(tmp_buffer)));

    buffer_free(tmp_buffer);
    //info("Added %s completed", st->name);

    if (completion)
       complete(completion);

    return rc;
}

void aclk_reset_chart_event(struct aclk_database_worker_config *wc, struct aclk_database_cmd cmd)
{
//    int rc;

    BUFFER *sql = buffer_create(1024);

    buffer_sprintf(sql, "update aclk_chart_%s set date_submitted = NULL where sequence_id >= %d",
                        wc->uuid_str, cmd.count < 0 ? 0 : cmd.count);
    db_execute(buffer_tostring(sql));

    buffer_free(sql);
    if (cmd.completion)
        complete(cmd.completion);

    return;
}

void aclk_reset_node_event(struct aclk_database_worker_config *wc, struct aclk_database_cmd cmd)
{
    int rc;
    uuid_t  host_id;

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
    if (!aclk_architecture)
        aclk_update_chart(st->rrdhost, st->id, ACLK_CMD_CHART);

    if (unlikely(!st->rrdhost->dbsync_worker))
        return;

    struct aclk_database_cmd cmd;
    cmd.opcode = ACLK_DATABASE_ADD_CHART;
    cmd.data = st;
    cmd.data_param = strdupz("JSON");
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


void sql_create_aclk_table(RRDHOST *host)
{
    char uuid_str[GUID_LEN + 1];

    uuid_unparse_lower(host->host_uuid, uuid_str);
    uuid_str[8] = '_';
    uuid_str[13] = '_';
    uuid_str[18] = '_';
    uuid_str[23] = '_';

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

    buffer_sprintf(sql,TABLE_ACLK_ALERT_PAYLOAD, uuid_str);
    db_execute(buffer_tostring(sql));


    buffer_free(sql);

    // Spawn db thread for processing (event loop)
    if (unlikely(host->dbsync_worker))
        return;

    struct aclk_database_worker_config *wc = NULL;
    host->dbsync_worker = mallocz(sizeof(struct aclk_database_worker_config));
    wc = (struct aclk_database_worker_config *) host->dbsync_worker;
    strcpy(wc->uuid_str, uuid_str);
    fatal_assert(0 == uv_thread_create(&(wc->thread), aclk_database_worker, wc));
}

void sql_drop_aclk_table_list(RRDHOST *host)
{
    char uuid_str[GUID_LEN + 1];

    uuid_unparse_lower(host->host_uuid, uuid_str);
    uuid_str[8] = '_';
    uuid_str[13] = '_';
    uuid_str[18] = '_';
    uuid_str[23] = '_';

    BUFFER *sql = buffer_create(1024);

    buffer_sprintf(sql, "drop table if exists aclk_chart_%s;", uuid_str);
    db_execute(buffer_tostring(sql));

    buffer_sprintf(sql, "drop table if exists aclk_chart_payload_%s;", uuid_str);
    db_execute(buffer_tostring(sql));

    buffer_sprintf(sql,"drop trigger if exists aclk_tr_chart_payload_%s;", uuid_str);
    db_execute(buffer_tostring(sql));

    buffer_sprintf(sql,"drop table if exists aclk_alert_%s;", uuid_str);
    db_execute(buffer_tostring(sql));

    buffer_sprintf(sql,"drop table if exists aclk_alert_payload_%s;", uuid_str);
    db_execute(buffer_tostring(sql));
    buffer_free(sql);
}
