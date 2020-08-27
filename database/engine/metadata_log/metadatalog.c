// SPDX-License-Identifier: GPL-3.0-or-later
#define NETDATA_RRD_INTERNALS

#include "metadatalog.h"

static void sanity_check(void)
{
    /* Magic numbers must fit in the super-blocks */
    BUILD_BUG_ON(strlen(RRDENG_METALOG_MAGIC) > RRDENG_MAGIC_SZ);

    /* Metadata log file super-block cannot be larger than RRDENG_BLOCK_SIZE */
    BUILD_BUG_ON(RRDENG_METALOG_SB_PADDING_SZ < 0);

    /* Object duplication factor cannot be less than 1, or too close to 1 */
    BUILD_BUG_ON(MAX_DUPLICATION_PERCENTAGE < 110);
}

char *get_metalog_statistics(struct metalog_instance *ctx, char *str, size_t size)
{
    snprintfz(str, size,
              "io_write_bytes: %ld\n"
              "io_write_requests: %ld\n"
              "io_read_bytes: %ld\n"
              "io_read_requests: %ld\n"
              "io_write_record_bytes: %ld\n"
              "io_write_records: %ld\n"
              "io_read_record_bytes: %ld\n"
              "io_read_records: %ld\n"
              "metadata_logfile_creations: %ld\n"
              "metadata_logfile_deletions: %ld\n"
              "io_errors: %ld\n"
              "fs_errors: %ld\n",
              (long)ctx->stats.io_write_bytes,
              (long)ctx->stats.io_write_requests,
              (long)ctx->stats.io_read_bytes,
              (long)ctx->stats.io_read_requests,
              (long)ctx->stats.io_write_record_bytes,
              (long)ctx->stats.io_write_records,
              (long)ctx->stats.io_read_record_bytes,
              (long)ctx->stats.io_read_records,
              (long)ctx->stats.metadata_logfile_creations,
              (long)ctx->stats.metadata_logfile_deletions,
              (long)ctx->stats.io_errors,
              (long)ctx->stats.fs_errors
    );
    return str;
}

/* The buffer must not be empty */
void metalog_commit_record(struct metalog_instance *ctx, BUFFER *buffer, enum metalog_opcode opcode, uuid_t *uuid,
                           int compacting)
{
    struct metalog_cmd cmd;

    fatal_assert(buffer_strlen(buffer));
    fatal_assert(opcode == METALOG_COMMIT_CREATION_RECORD || opcode == METALOG_COMMIT_DELETION_RECORD);

    cmd.opcode = opcode;
    cmd.record_io_descr.buffer = buffer;
    cmd.record_io_descr.compacting = compacting;
    if (!uuid)
        uuid_clear(cmd.record_io_descr.uuid);
    else
        uuid_copy(cmd.record_io_descr.uuid, *uuid);
    metalog_enq_cmd(&ctx->worker_config, &cmd);
}

static void commit_record(struct metalog_worker_config* wc, struct metalog_record_io_descr *io_descr, uint8_t type)
{
    struct metalog_instance *ctx = wc->ctx;
    unsigned payload_length, size_bytes;
    void *buf, *mlf_payload;
    /* persistent structures */
    struct rrdeng_metalog_record_header *mlf_header;
    struct rrdeng_metalog_record_trailer *mlf_trailer;
    uLong crc;

    payload_length = buffer_strlen(io_descr->buffer);
    size_bytes = sizeof(*mlf_header) + payload_length + sizeof(*mlf_trailer);

    if (io_descr->compacting)
        buf = mlf_get_records_buffer(wc, &ctx->compaction_state.records_log,
                                     &ctx->compaction_state.new_metadata_logfiles, size_bytes);
    else
        buf = mlf_get_records_buffer(wc, &ctx->records_log, &ctx->metadata_logfiles, size_bytes);

    mlf_header = buf;
    mlf_header->type = type;
    mlf_header->header_length = sizeof(*mlf_header);
    mlf_header->payload_length = payload_length;

    mlf_payload = buf + sizeof(*mlf_header);
    memcpy(mlf_payload, buffer_tostring(io_descr->buffer), payload_length);

    mlf_trailer = buf + sizeof(*mlf_header) + payload_length;
    crc = crc32(0L, Z_NULL, 0);
    crc = crc32(crc, buf, sizeof(*mlf_header) + payload_length);
    crc32set(mlf_trailer->checksum, crc);

    buffer_free(io_descr->buffer);
}

static void do_commit_record(struct metalog_worker_config* wc, uint8_t type, void *data)
{
    struct metalog_record_io_descr *io_descr = (struct metalog_record_io_descr *)data;
    switch (type) {
    case METALOG_CREATE_OBJECT:
        if (!uuid_is_null(io_descr->uuid)) { /* It's a valid object */
            struct metalog_record record;

            uuid_copy(record.uuid, io_descr->uuid);
            if (io_descr->compacting)
                mlf_record_insert(wc->ctx->compaction_state.new_metadata_logfiles.last, &record);
            else
                mlf_record_insert(wc->ctx->metadata_logfiles.last, &record);
        } /* fall through */
        case METALOG_DELETE_OBJECT:
        commit_record(wc, (struct metalog_record_io_descr *)data, type);
        break;
    default:
        fatal("Unknown metadata log file record type, possible memory corruption.");
        break;
    }
}

/* Only creates a new metadata file and links it to the metadata log if the last one is non empty. */
void metalog_try_link_new_metadata_logfile(struct metalog_worker_config *wc)
{
    struct metalog_instance *ctx = wc->ctx;
    struct metadata_logfile *metalogfile;
    int ret;

    metalogfile = ctx->metadata_logfiles.last;
    if (metalogfile->records.first) { /* it has records */
        /* Finalize metadata log file and create a new one */
        mlf_flush_records_buffer(wc, &ctx->records_log, &ctx->metadata_logfiles);
        fsync_metadata_logfile(ctx->metadata_logfiles.last);
        ret = add_new_metadata_logfile(ctx, &ctx->metadata_logfiles, 0, ctx->last_fileno + 1);
        if (likely(!ret)) {
            ++ctx->last_fileno;
        }
    }
}

void metalog_test_quota(struct metalog_worker_config *wc)
{
    struct metalog_instance *ctx = wc->ctx;
    struct metadata_logfile *metalogfile;
    unsigned current_size;
    uint8_t only_one_metalogfile;

    metalogfile = ctx->metadata_logfiles.last;
    current_size = metalogfile->pos;
    if (unlikely(current_size >= MAX_METALOGFILE_SIZE)) {
        metalog_try_link_new_metadata_logfile(wc);
    }

    metalogfile = ctx->metadata_logfiles.last;
    only_one_metalogfile = (metalogfile == ctx->metadata_logfiles.first) ? 1 : 0;
    debug(D_METADATALOG, "records=%lu objects=%lu", (long unsigned)ctx->records_nr,
           (long unsigned)ctx->objects_nr);
    if (unlikely(!only_one_metalogfile &&
                 ctx->records_nr > (ctx->objects_nr * (uint64_t)MAX_DUPLICATION_PERCENTAGE) / 100) &&
                NO_QUIESCE == ctx->quiesce) {
        metalog_do_compaction(wc);
    }
}

static inline int metalog_threads_alive(struct metalog_worker_config* wc)
{
    if (wc->cleanup_thread_compacting_files) {
        return 1;
    }

    return 0;
}

static void metalog_cleanup_finished_threads(struct metalog_worker_config *wc)
{
    struct metalog_instance *ctx = wc->ctx;

    if (unlikely(wc->cleanup_thread_compacting_files)) {
        after_compact_old_records(wc);
    }
    if (unlikely(SET_QUIESCE == ctx->quiesce && !metalog_threads_alive(wc))) {
        ctx->quiesce = QUIESCED;
        complete(&ctx->metalog_completion);
    }
}

static void metalog_init_cmd_queue(struct metalog_worker_config *wc)
{
    wc->cmd_queue.head = wc->cmd_queue.tail = 0;
    wc->queue_size = 0;
    fatal_assert(0 == uv_cond_init(&wc->cmd_cond));
    fatal_assert(0 == uv_mutex_init(&wc->cmd_mutex));
}

void metalog_enq_cmd(struct metalog_worker_config *wc, struct metalog_cmd *cmd)
{
    unsigned queue_size;

    /* wait for free space in queue */
    uv_mutex_lock(&wc->cmd_mutex);
    while ((queue_size = wc->queue_size) == METALOG_CMD_Q_MAX_SIZE) {
        uv_cond_wait(&wc->cmd_cond, &wc->cmd_mutex);
    }
    fatal_assert(queue_size < METALOG_CMD_Q_MAX_SIZE);
    /* enqueue command */
    wc->cmd_queue.cmd_array[wc->cmd_queue.tail] = *cmd;
    wc->cmd_queue.tail = wc->cmd_queue.tail != METALOG_CMD_Q_MAX_SIZE - 1 ?
                         wc->cmd_queue.tail + 1 : 0;
    wc->queue_size = queue_size + 1;
    uv_mutex_unlock(&wc->cmd_mutex);

    /* wake up event loop */
    fatal_assert(0 == uv_async_send(&wc->async));
}

struct metalog_cmd metalog_deq_cmd(struct metalog_worker_config *wc)
{
    struct metalog_cmd ret;
    unsigned queue_size;

    uv_mutex_lock(&wc->cmd_mutex);
    queue_size = wc->queue_size;
    if (queue_size == 0) {
        ret.opcode = METALOG_NOOP;
    } else {
        /* dequeue command */
        ret = wc->cmd_queue.cmd_array[wc->cmd_queue.head];
        if (queue_size == 1) {
            wc->cmd_queue.head = wc->cmd_queue.tail = 0;
        } else {
            wc->cmd_queue.head = wc->cmd_queue.head != RRDENG_CMD_Q_MAX_SIZE - 1 ?
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

/* Flushes metadata log when timer expires */
#define TIMER_PERIOD_MS (5000)

static void timer_cb(uv_timer_t* handle)
{
    struct metalog_worker_config* wc = handle->data;
    struct metalog_instance *ctx = wc->ctx;

    uv_stop(handle->loop);
    uv_update_time(handle->loop);
    metalog_test_quota(wc);
    debug(D_METADATALOG, "%s: timeout reached.", __func__);
#ifdef NETDATA_INTERNAL_CHECKS
    {
        char buf[4096];
        debug(D_METADATALOG, "%s", get_metalog_statistics(wc->ctx, buf, sizeof(buf)));
    }
#endif
    mlf_flush_records_buffer(wc, &ctx->records_log, &ctx->metadata_logfiles);
}

#define MAX_CMD_BATCH_SIZE (256)

void metalog_worker(void* arg)
{
    struct metalog_worker_config *wc = arg;
    struct metalog_instance *ctx = wc->ctx;
    uv_loop_t* loop;
    int shutdown, ret;
    enum metalog_opcode opcode;
    uv_timer_t timer_req;
    struct metalog_cmd cmd;
    unsigned cmd_batch_size;

    sanity_check();
    metalog_init_cmd_queue(wc);

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

    wc->now_compacting_files = NULL;
    wc->cleanup_thread_compacting_files = 0;

    /* quota check timer */
    ret = uv_timer_init(loop, &timer_req);
    if (ret) {
        error("uv_timer_init(): %s", uv_strerror(ret));
        goto error_after_timer_init;
    }
    timer_req.data = wc;

    wc->error = 0;
    /* wake up initialization thread */
    complete(&ctx->metalog_completion);

    fatal_assert(0 == uv_timer_start(&timer_req, timer_cb, TIMER_PERIOD_MS, TIMER_PERIOD_MS));
    shutdown = 0;
    while (likely(shutdown == 0 || metalog_threads_alive(wc))) {
        uv_run(loop, UV_RUN_DEFAULT);
        metalog_cleanup_finished_threads(wc);

        /* wait for commands */
        cmd_batch_size = 0;
        do {
            /*
             * Avoid starving the loop when there are too many commands coming in.
             * timer_cb will interrupt the loop again to allow serving more commands.
             */
            if (unlikely(cmd_batch_size >= MAX_CMD_BATCH_SIZE))
                break;

            cmd = metalog_deq_cmd(wc);
            opcode = cmd.opcode;
            ++cmd_batch_size;

            switch (opcode) {
            case METALOG_NOOP:
                /* the command queue was empty, do nothing */
                break;
            case METALOG_SHUTDOWN:
                shutdown = 1;
                break;
            case METALOG_QUIESCE:
                ctx->quiesce = SET_QUIESCE;
                fatal_assert(0 == uv_timer_stop(&timer_req));
                uv_close((uv_handle_t *)&timer_req, NULL);
                mlf_flush_records_buffer(wc, &ctx->records_log, &ctx->metadata_logfiles);
                if (!metalog_threads_alive(wc)) {
                    ctx->quiesce = QUIESCED;
                    complete(&ctx->metalog_completion);
                }
                break;
            case METALOG_COMMIT_CREATION_RECORD:
                do_commit_record(wc, METALOG_CREATE_OBJECT, &cmd.record_io_descr);
                break;
            case METALOG_COMMIT_DELETION_RECORD:
                do_commit_record(wc, METALOG_DELETE_OBJECT, &cmd.record_io_descr);
                break;
            case METALOG_COMPACTION_FLUSH:
                mlf_flush_records_buffer(wc, &ctx->compaction_state.records_log,
                                         &ctx->compaction_state.new_metadata_logfiles);
                fsync_metadata_logfile(ctx->compaction_state.new_metadata_logfiles.last);
                complete(cmd.record_io_descr.completion);
                break;
                default:
                debug(D_METADATALOG, "%s: default.", __func__);
                break;
            }
        } while (opcode != METALOG_NOOP);
    }

    /* cleanup operations of the event loop */
    info("Shutting down RRD metadata log event loop.");

    /*
     * uv_async_send after uv_close does not seem to crash in linux at the moment,
     * it is however undocumented behaviour and we need to be aware if this becomes
     * an issue in the future.
     */
    uv_close((uv_handle_t *)&wc->async, NULL);

    mlf_flush_records_buffer(wc, &ctx->records_log, &ctx->metadata_logfiles);
    uv_run(loop, UV_RUN_DEFAULT);

    info("Shutting down RRD metadata log loop complete.");
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
    /* wake up initialization thread */
    complete(&ctx->metalog_completion);
}

void error_with_guid(uuid_t *uuid, char *reason)
{
    char  uuid_str[37];

    uuid_unparse_lower(*uuid, uuid_str);
    errno = 0;
    error("%s (GUID = %s)", reason, uuid_str);
}

void info_with_guid(uuid_t *uuid, char *reason)
{
    char  uuid_str[37];

    uuid_unparse_lower(*uuid, uuid_str);
    info("%s (GUID = %s)", reason, uuid_str);
}
