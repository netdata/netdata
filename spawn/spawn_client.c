// SPDX-License-Identifier: GPL-3.0-or-later

#include "spawn.h"

static uv_process_t process;
static uv_pipe_t spawn_channel;
static uv_loop_t *loop;
uv_async_t spawn_async;

static char prot_buffer[MAX_COMMAND_LENGTH];
static unsigned prot_buffer_len = 0;

static void async_cb(uv_async_t *handle)
{
    uv_stop(handle->loop);
}

static void after_pipe_write(uv_write_t* req, int status)
{
    (void)status;
#ifdef SPAWN_DEBUG
    info("CLIENT %s called status=%d", __func__, status);
#endif
    void **data = req->data;
    freez(data[0]);
    freez(data[1]);
    freez(data);
}

static void client_parse_spawn_protocol(unsigned source_len, char *source)
{
    unsigned required_len;
    struct spawn_prot_header *header;
    struct spawn_prot_spawn_result *spawn_result;
    struct spawn_prot_cmd_exit_status *exit_status;
    struct spawn_cmd_info *cmdinfo;

    while (source_len) {
        required_len = sizeof(*header);
        if (prot_buffer_len < required_len)
            copy_to_prot_buffer(prot_buffer, &prot_buffer_len, required_len - prot_buffer_len, &source, &source_len);
        if (prot_buffer_len < required_len)
            return; /* Source buffer ran out */

        header = (struct spawn_prot_header *)prot_buffer;
        cmdinfo = (struct spawn_cmd_info *)header->handle;
        fatal_assert(NULL != cmdinfo);

        switch(header->opcode) {
        case SPAWN_PROT_SPAWN_RESULT:
            required_len += sizeof(*spawn_result);
            if (prot_buffer_len < required_len)
                copy_to_prot_buffer(prot_buffer, &prot_buffer_len, required_len - prot_buffer_len, &source, &source_len);
            if (prot_buffer_len < required_len)
                return; /* Source buffer ran out */

            spawn_result = (struct spawn_prot_spawn_result *)(header + 1);
            uv_mutex_lock(&cmdinfo->mutex);
            cmdinfo->pid = spawn_result->exec_pid;
            if (0 == cmdinfo->pid) { /* Failed to spawn */
#ifdef SPAWN_DEBUG
                info("CLIENT %s SPAWN_PROT_SPAWN_RESULT failed to spawn.", __func__);
#endif
                cmdinfo->flags |= SPAWN_CMD_FAILED_TO_SPAWN | SPAWN_CMD_DONE;
                uv_cond_signal(&cmdinfo->cond);
            } else {
                cmdinfo->exec_run_timestamp = spawn_result->exec_run_timestamp;
                cmdinfo->flags |= SPAWN_CMD_IN_PROGRESS;
#ifdef SPAWN_DEBUG
                info("CLIENT %s SPAWN_PROT_SPAWN_RESULT in progress.", __func__);
#endif
            }
            uv_mutex_unlock(&cmdinfo->mutex);
            prot_buffer_len = 0;
            break;
        case SPAWN_PROT_CMD_EXIT_STATUS:
            required_len += sizeof(*exit_status);
            if (prot_buffer_len < required_len)
                copy_to_prot_buffer(prot_buffer, &prot_buffer_len, required_len - prot_buffer_len, &source, &source_len);
            if (prot_buffer_len < required_len)
                return; /* Source buffer ran out */

            exit_status = (struct spawn_prot_cmd_exit_status *)(header + 1);
            uv_mutex_lock(&cmdinfo->mutex);
            cmdinfo->exit_status = exit_status->exec_exit_status;
#ifdef SPAWN_DEBUG
            info("CLIENT %s SPAWN_PROT_CMD_EXIT_STATUS %d.", __func__, exit_status->exec_exit_status);
#endif
            cmdinfo->flags |= SPAWN_CMD_DONE;
            uv_cond_signal(&cmdinfo->cond);
            uv_mutex_unlock(&cmdinfo->mutex);
            prot_buffer_len = 0;
            break;
        default:
            fatal_assert(0);
            break;
        }

    }
}

static void on_pipe_read(uv_stream_t* pipe, ssize_t nread, const uv_buf_t* buf)
{
    if (0 == nread) {
        info("%s: Zero bytes read from spawn pipe.", __func__);
    } else if (UV_EOF == nread) {
        info("EOF found in spawn pipe.");
    } else if (nread < 0) {
        error("%s: %s", __func__, uv_strerror(nread));
    }

    if (nread < 0) { /* stop stream due to EOF or error */
        (void)uv_read_stop((uv_stream_t *)pipe);
    } else if (nread) {
#ifdef SPAWN_DEBUG
        info("CLIENT %s read %u", __func__, (unsigned)nread);
#endif
        client_parse_spawn_protocol(nread, buf->base);
    }
    if (buf && buf->len) {
        freez(buf->base);
    }

    if (nread < 0) {
        uv_close((uv_handle_t *)pipe, NULL);
    }
}

static void on_read_alloc(uv_handle_t* handle,
                          size_t suggested_size,
                          uv_buf_t* buf)
{
    (void)handle;
    buf->base = mallocz(suggested_size);
    buf->len = suggested_size;
}

static void spawn_process_cmd(struct spawn_cmd_info *cmdinfo)
{
    int ret;
    uv_buf_t *writebuf;
    struct write_context *write_ctx;

    void **data = callocz(2, sizeof(void *));
    writebuf = callocz(3, sizeof(uv_buf_t));
    write_ctx = callocz(1, sizeof(*write_ctx));

    data[0] = write_ctx;
    data[1] = writebuf;
    write_ctx->write_req.data = data;

    uv_mutex_lock(&cmdinfo->mutex);
    cmdinfo->flags |= SPAWN_CMD_PROCESSED;
    uv_mutex_unlock(&cmdinfo->mutex);

    write_ctx->header.opcode = SPAWN_PROT_EXEC_CMD;
    write_ctx->header.handle = cmdinfo;
    write_ctx->payload.command_length = strlen(cmdinfo->command_to_run);

    writebuf[0] = uv_buf_init((char *)&write_ctx->header, sizeof(write_ctx->header));
    writebuf[1] = uv_buf_init((char *)&write_ctx->payload, sizeof(write_ctx->payload));
    writebuf[2] = uv_buf_init((char *)cmdinfo->command_to_run, write_ctx->payload.command_length);

#ifdef SPAWN_DEBUG
    info("CLIENT %s SPAWN_PROT_EXEC_CMD %u", __func__, (unsigned)cmdinfo->serial);
#endif
    ret = uv_write(&write_ctx->write_req, (uv_stream_t *)&spawn_channel, writebuf, 3, after_pipe_write);
    fatal_assert(ret == 0);
}

void spawn_client(void *arg)
{
    int ret;
    struct completion *completion = (struct completion *)arg;

    loop = mallocz(sizeof(uv_loop_t));
    ret = uv_loop_init(loop);
    if (ret) {
        error("uv_loop_init(): %s", uv_strerror(ret));
        spawn_thread_error = ret;
        goto error_after_loop_init;
    }
    loop->data = NULL;

    spawn_async.data = NULL;
    ret = uv_async_init(loop, &spawn_async, async_cb);
    if (ret) {
        error("uv_async_init(): %s", uv_strerror(ret));
        spawn_thread_error = ret;
        goto error_after_async_init;
    }

    ret = uv_pipe_init(loop, &spawn_channel, 1);
    if (ret) {
        error("uv_pipe_init(): %s", uv_strerror(ret));
        spawn_thread_error = ret;
        goto error_after_pipe_init;
    }
    fatal_assert(spawn_channel.ipc);

    ret = create_spawn_server(loop, &spawn_channel, &process);
    if (ret) {
        error("Failed to fork spawn server process.");
        spawn_thread_error = ret;
        goto error_after_spawn_server;
    }

    spawn_thread_error = 0;
    spawn_thread_shutdown = 0;
    /* wake up initialization thread */
    completion_mark_complete(completion);

    prot_buffer_len = 0;
    ret = uv_read_start((uv_stream_t *)&spawn_channel, on_read_alloc, on_pipe_read);
    fatal_assert(ret == 0);

    while (spawn_thread_shutdown == 0) {
        struct spawn_cmd_info *cmdinfo;

        uv_run(loop, UV_RUN_DEFAULT);
        while (NULL != (cmdinfo = spawn_get_unprocessed_cmd())) {
            spawn_process_cmd(cmdinfo);
        }
    }
    /* cleanup operations of the event loop */
    info("Shutting down spawn client event loop.");
    uv_close((uv_handle_t *)&spawn_channel, NULL);
    uv_close((uv_handle_t *)&spawn_async, NULL);
    uv_run(loop, UV_RUN_DEFAULT); /* flush all libuv handles */

    info("Shutting down spawn client loop complete.");
    fatal_assert(0 == uv_loop_close(loop));

    return;

error_after_spawn_server:
    uv_close((uv_handle_t *)&spawn_channel, NULL);
error_after_pipe_init:
    uv_close((uv_handle_t *)&spawn_async, NULL);
error_after_async_init:
    uv_run(loop, UV_RUN_DEFAULT); /* flush all libuv handles */
    fatal_assert(0 == uv_loop_close(loop));
error_after_loop_init:
    freez(loop);

    /* wake up initialization thread */
    completion_mark_complete(completion);
}
