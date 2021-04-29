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
                case ACLK_DATABASE_ADD_CHART:
                    //sql_maint_database();
                    info("Adding chart %s (queue size = %d)", wc->uuid_str, wc->queue_size);
                    aclk_add_chart_event((RRDSET *) cmd.data, (char *) cmd.data_param);
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

    sprintf(sql,"insert into aclk_payload_%s (unique_id, chart_id, date_created, type, payload) " \
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


void aclk_add_chart_event(RRDSET *st, char *payload_type)
{
    char uuid_str[37];
    char sql[1024];
    sqlite3_stmt *res_chart = NULL;
    int rc;

    if (unlikely(!db_meta)) {
        if (default_rrd_memory_mode != RRD_MEMORY_MODE_DBENGINE) {
            return;
        }
        error_report("Database has not been initialized");
        return;
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

    return;
//
//    if (unlikely(rc))
//        return;
//
//    sprintf(sql,"insert into aclk_chart_%s (chart_id, unique_id, status, date_created) " \
//                 "values (@chart_uuid, @unique_id, 'pending', strftime('%%s')) " \
//                 "on conflict(chart_id, status) do update set unique_id = @unique_id, update_count = update_count + 1;" , uuid_str);
//
//    rc = sqlite3_prepare_v2(db_meta, sql, -1, &res_chart, 0);
//    if (unlikely(rc != SQLITE_OK)) {
//        error_report("Failed to prepare statement to store chart event data");
//        return;
//    }
//
//    rc = sqlite3_bind_blob(res_chart, 1, st->chart_uuid , sizeof(*st->chart_uuid), SQLITE_STATIC);
//    if (unlikely(rc != SQLITE_OK))
//        goto bind_fail;
//
//    rc = sqlite3_bind_blob(res_chart, 2, &unique_uuid , sizeof(unique_uuid), SQLITE_STATIC);
//    if (unlikely(rc != SQLITE_OK))
//        goto bind_fail;
//
//    rc = execute_insert(res_chart);
//    if (unlikely(rc != SQLITE_DONE))
//        error_report("Failed store chart event, rc = %d", rc);
//
//bind_fail:
//    rc = sqlite3_finalize(res_chart);
//    if (unlikely(rc != SQLITE_OK))
//        error_report("Failed to reset statement in store dimension, rc = %d", rc);
//
//    return;
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
    aclk_database_enq_cmd_nowake((struct aclk_database_worker_config *) st->rrdhost->dbsync_worker, &cmd);

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
    char uuid_str[37];
    char sql[2048];

    if (unlikely(host->dbsync_worker))
        return;

    uuid_unparse_lower(host->host_uuid, uuid_str);
    uuid_str[8] = '_';
    uuid_str[13] = '_';
    uuid_str[18] = '_';
    uuid_str[23] = '_';

    struct sqlite_worker_config *wc = NULL;

    host->dbsync_worker = mallocz(sizeof(struct aclk_database_worker_config));

    wc = (struct sqlite_worker_config *) host->dbsync_worker;
    strcpy(wc->uuid_str, uuid_str);
    wc->host = host;

    sprintf(sql, "create table if not exists aclk_chart_%s (sequence_id integer primary key, " \
        "date_created, date_updated, date_submitted, status, chart_id, unique_id, " \
        "update_count default 1, unique(chart_id, status));", uuid_str);

    db_execute(sql);
    sprintf(sql,"create table if not exists aclk_payload_%s (unique_id blob primary key, " \
                 "chart_id, type, date_created, payload);", uuid_str);
    db_execute(sql);

    sprintf(sql,"create trigger if not exists aclk_tr_payload_%s after insert on aclk_payload_%s begin insert into aclk_chart_%s " \
        "(chart_id, unique_id, status, date_created) " \
        " values (new.chart_id, new.unique_id, 'pending', strftime('%%s')) on conflict(chart_id, status) " \
        " do update set unique_id = new.unique_id, update_count = update_count + 1; " \
        " end;", uuid_str, uuid_str, uuid_str);

    info("%s", sql);
    db_execute(sql);

    sprintf(sql,"create table if not exists aclk_alert_%s (sequence_id integer primary key, " \
                 "date_created, date_updated, unique_id);", uuid_str);
    db_execute(sql);

    // Spawn db thread for processing (event loop)

    fatal_assert(0 == uv_thread_create(&(wc->thread), aclk_database_worker, wc));
}
