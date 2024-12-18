// SPDX-License-Identifier: GPL-3.0-or-later

#include "ebpf-ipc.h"
#include "libnetdata/libnetdata.h"

#if defined(OS_LINUX)
#define NETDATA_EBPF_INTEGRATION_CMD_LENGTH 128

netdata_ebpf_pid_stats_t *integration_shm;
int shm_fd_ebpf_integration = -1;
sem_t *shm_mutex_ebpf_integration = SEM_FAILED;

struct integration_ebpf_context {
    /* embedded client pipe structure at address 0 */
    uv_pipe_t client;

    uv_work_t work;
    uv_write_t write_req;
    ebpf_cmds_t idx;
    char *args;
    integration_cmd_status_t status;
    char *message;
    char command_string[NETDATA_EBPF_INTEGRATION_CMD_LENGTH];
    unsigned command_string_size;
};

uint32_t plot_intercommunication_charts = 0;

static integration_cmd_status_t cmd_ping_execute(char *args, char **message);

static integration_command_info_t command_info_array[] = {
    {"ping", cmd_ping_execute}
};

static int command_server_initialized = 0;

static uv_thread_t thread;
static uv_loop_t* loop;
static uv_async_t async;
static struct completion completion;
static uv_pipe_t server_pipe;

static int command_thread_error;
static int command_thread_shutdown;

const char *pipes = "NETDATA_EBPF_INTEGRATION_PIPENAME";

static integration_cmd_status_t cmd_ping_execute(char *args, char **message)
{
    (void)args;

    *message = strdupz("pong");

    return INTEGRATION_CMD_STATUS_SUCCESS;
}

static void netdata_integration_pipe_close_cb(uv_handle_t* handle)
{
    /* Also frees command context */
    freez(handle);
}

static void netdata_integration_pipe_write_cb(uv_write_t* req, int status)
{
    (void)status;
    uv_pipe_t *client = req->data;

    uv_close((uv_handle_t *)client, netdata_integration_pipe_close_cb);
    buffer_free(client->data);
}

integration_cmd_status_t netdata_integration_execute_command(ebpf_cmds_t idx, char *args, char **message)
{
    // TODO: ADD PROPER LOCK AFTER TESTS
    integration_cmd_status_t status;

    status = command_info_array[idx].func(args, message);

    return status;
}

static void netdata_integration_alloc_cb(uv_handle_t *handle, size_t suggested_size, uv_buf_t *buf)
{
    (void)handle;

    buf->base = mallocz(suggested_size);
    buf->len = suggested_size;
}

static inline void netdata_integration_add_string_to_command_reply(BUFFER *reply_string,
                                                                   unsigned *reply_string_size,
                                                                   char *str)
{
    unsigned len;

    len = strlen(str);
    buffer_fast_strcat(reply_string, str, len);
    *reply_string_size += len;
}

static void netdata_integration_send_command_reply(struct integration_ebpf_context *cmd_ctx,
                                                   integration_cmd_status_t status,
                                                   char *message)
{
    (void)status;
    int ret;
    uv_buf_t write_buf;
    unsigned reply_string_size = 0;
    BUFFER *reply_string = buffer_create(128, NULL);
    uv_stream_t *client = (uv_stream_t *)(uv_pipe_t *)cmd_ctx;

    netdata_integration_add_string_to_command_reply(reply_string, &reply_string_size, message);

    cmd_ctx->write_req.data = client;
    write_buf.base = reply_string->buffer;
    write_buf.len = reply_string_size;
    ret = uv_write(&cmd_ctx->write_req, (uv_stream_t *)client, &write_buf, 1, netdata_integration_pipe_write_cb);
    if (ret) {
        netdata_log_error("uv_write(): %s", uv_strerror(ret));
    }
}

static void netdata_integration_after_schedule_command(uv_work_t *req, int status)
{
    struct integration_ebpf_context *cmd_ctx = req->data;
    (void)status;

    if (cmd_ctx->message) {
        netdata_integration_send_command_reply(cmd_ctx, cmd_ctx->status, cmd_ctx->message);
        freez(cmd_ctx->message);
    }
}

static void netdata_integration_schedule_command(uv_work_t *req)
{
    struct integration_ebpf_context *cmd_ctx = req->data;
    cmd_ctx->status = netdata_integration_execute_command(cmd_ctx->idx, cmd_ctx->args, &cmd_ctx->message);
}

static void netdata_integration_parse_commands(struct integration_ebpf_context *cmd_ctx)
{
    char *pos, *lstrip, *rstrip;
    ebpf_cmds_t i;

    /* Skip white-space characters */
    for (pos = cmd_ctx->command_string ; isspace((uint8_t)*pos) && ('\0' != *pos) ; ++pos) ;
    // TODO: ADD ALL COMMANDS
    // for (i = 0 ; i < NETDATA_EBPF_CMDS_TOTAL ; ++i) {
    for (i = 0 ; i < NETDATA_EBPF_CMD_COLLECT ; ++i) {
        if (!strncmp(pos, command_info_array[i].cmd_str, strlen(command_info_array[i].cmd_str))) {
            for (lstrip=pos + strlen(command_info_array[i].cmd_str); isspace((uint8_t)*lstrip) && ('\0' != *lstrip); ++lstrip) ;
            for (rstrip=lstrip+strlen(lstrip)-1; rstrip>lstrip && isspace((uint8_t)*rstrip); *(rstrip--) = 0 ) ;

            cmd_ctx->work.data = cmd_ctx;
            cmd_ctx->idx = i;
            cmd_ctx->args = lstrip;
            cmd_ctx->message = NULL;

            fatal_assert(0 == uv_queue_work(loop,
                                            &cmd_ctx->work,
                                            netdata_integration_schedule_command,
                                            netdata_integration_after_schedule_command));
            break;
        }
    }
    if (NETDATA_EBPF_CMDS_TOTAL == i) {
        /* no command found */
        netdata_log_error("%s: Illegal command.", __func__);
    }
}

static void netdata_integration_pipe_read_cb(uv_stream_t *client, ssize_t nread, const uv_buf_t *buf)
{
    struct integration_ebpf_context *cmd_ctx = (struct integration_ebpf_context *)client;

    if (0 == nread) {
        netdata_log_info("%s: Zero bytes read by command pipe.", __func__);
    } else if (UV_EOF == nread) {
        netdata_log_info("EOF found in command pipe.");
        netdata_integration_parse_commands(cmd_ctx);
    } else if (nread < 0) {
        netdata_log_error("%s: %s", __func__, uv_strerror(nread));
    }

    if (nread < 0) { /* stop stream due to EOF or error */
        (void)uv_read_stop((uv_stream_t *)client);
    } else if (nread) {
        size_t to_copy;

        to_copy = MIN((size_t) nread, NETDATA_EBPF_INTEGRATION_CMD_LENGTH - 1 - cmd_ctx->command_string_size);
        memcpy(cmd_ctx->command_string + cmd_ctx->command_string_size, buf->base, to_copy);
        cmd_ctx->command_string_size += to_copy;
        cmd_ctx->command_string[cmd_ctx->command_string_size] = '\0';
    }
    if (buf && buf->len) {
        freez(buf->base);
    }

    if (nread < 0 && UV_EOF != nread) {
        uv_close((uv_handle_t *)client, netdata_integration_pipe_close_cb);
    }
}

static void netdata_integration_connection_cb(uv_stream_t *server, int status)
{
    int ret;
    uv_pipe_t *client;
    struct integration_ebpf_context *cmd_ctx;
    fatal_assert(status == 0);

    /* combined allocation of client pipe and command context */
    cmd_ctx = mallocz(sizeof(*cmd_ctx));
    client = (uv_pipe_t *)cmd_ctx;
    ret = uv_pipe_init(server->loop, client, 1);
    if (ret) {
        netdata_log_error("uv_pipe_init(): %s", uv_strerror(ret));
        freez(cmd_ctx);
        return;
    }
    ret = uv_accept(server, (uv_stream_t *)client);
    if (ret) {
        netdata_log_error("uv_accept(): %s", uv_strerror(ret));
        uv_close((uv_handle_t *)client, netdata_integration_pipe_close_cb);
        return;
    }

    /* Start parsing a new command */
    cmd_ctx->command_string_size = 0;
    cmd_ctx->command_string[0] = '\0';

    ret = uv_read_start((uv_stream_t*)client, netdata_integration_alloc_cb, netdata_integration_pipe_read_cb);
    if (ret) {
        netdata_log_error("uv_read_start(): %s", uv_strerror(ret));
        uv_close((uv_handle_t *)client, netdata_integration_pipe_close_cb);
        return;
    }
}

static void netdata_integration_async_cb(uv_async_t *handle)
{
    uv_stop(handle->loop);
}

static void netdata_integration_parser_thread(void *arg)
{
    (void)arg;
    uv_fs_t req;
    loop = mallocz(sizeof(uv_loop_t));
    int ret = uv_loop_init(loop);
    if (ret) {
        netdata_log_error("uv_loop_init(): %s", uv_strerror(ret));
        command_thread_error = ret;
        goto error_after_loop_init;
    }
    loop->data = NULL;

    ret = uv_async_init(loop, &async, netdata_integration_async_cb);
    if (ret) {
        netdata_log_error("uv_async_init(): %s", uv_strerror(ret));
        command_thread_error = ret;
        goto error_after_async_init;
    }
    async.data = NULL;

    ret = uv_pipe_init(loop, &server_pipe, 0);
    if (ret) {
        netdata_log_error("uv_pipe_init(): %s", uv_strerror(ret));
        command_thread_error = ret;
        goto error_after_pipe_init;
    }

    const char *pipename = netdata_integration_pipename();

    (void)uv_fs_unlink(loop, &req, pipename, NULL);
    uv_fs_req_cleanup(&req);
    ret = uv_pipe_bind(&server_pipe, pipename);
    if (ret) {
        netdata_log_error("uv_pipe_bind(): %s", uv_strerror(ret));
        command_thread_error = ret;
        goto error_after_pipe_bind;
    }

    ret = uv_listen((uv_stream_t *)&server_pipe, SOMAXCONN, netdata_integration_connection_cb);
    if (ret) {
        /* Fallback to backlog of 1 */
        netdata_log_info("uv_listen() failed with backlog = %d, falling back to backlog = 1.", SOMAXCONN);
        ret = uv_listen((uv_stream_t *)&server_pipe, 1, netdata_integration_connection_cb);
    }
    if (ret) {
        netdata_log_error("uv_listen(): %s", uv_strerror(ret));
        command_thread_error = ret;
        goto error_after_uv_listen;
    }

    command_thread_error = 0;
    command_thread_shutdown = 0;
    /* wake up initialization thread */
    completion_mark_complete(&completion);

    while (command_thread_shutdown == 0) {
        uv_run(loop, UV_RUN_DEFAULT);
    }

    return;

error_after_uv_listen:
error_after_pipe_bind:
    uv_close((uv_handle_t*)&server_pipe, NULL);
error_after_pipe_init:
    uv_close((uv_handle_t *)&async, NULL);
error_after_async_init:
    uv_run(loop, UV_RUN_DEFAULT); /* flush all libuv handles */
    fatal_assert(0 == uv_loop_close(loop));
error_after_loop_init:
    freez(loop);

    /* wake up initialization thread */
    completion_mark_complete(&completion);
}

void netdata_integration_cleanup_shm()
{
    /* cleanup operations of the event loop */
    netdata_log_info("Shutting down pipe %s event loop.", pipes);
    uv_close((uv_handle_t *)&async, NULL);
    uv_close((uv_handle_t*)&server_pipe, NULL);
    uv_run(loop, UV_RUN_DEFAULT); /* flush all libuv handles */

    netdata_log_info("Shutting down pipe %s loop complete.", pipes);
    fatal_assert(0 == uv_loop_close(loop));
    freez(loop);

    if (shm_mutex_ebpf_integration != SEM_FAILED) {
        sem_close(shm_mutex_ebpf_integration);
    }

    if (integration_shm) {
        size_t length  = os_get_system_pid_max() * sizeof(netdata_ebpf_pid_stats_t);
        munmap(integration_shm, length);
    }

    if (shm_fd_ebpf_integration > 0) {
        close(shm_fd_ebpf_integration);
    }
}

static int netdata_integration_initialize_shm()
{
    shm_fd_ebpf_integration = shm_open(NETDATA_EBPF_INTEGRATION_NAME, O_CREAT | O_RDWR, 0660);
    if (shm_fd_ebpf_integration < 0) {
        nd_log(NDLS_COLLECTORS, NDLP_ERR, "Cannot initialize shared memory. Integration won't happen.");
        return -1;
    }

    size_t length = os_get_system_pid_max() * sizeof(netdata_ebpf_pid_stats_t);
    if (ftruncate(shm_fd_ebpf_integration, (off_t)length)) {
        nd_log(NDLS_COLLECTORS, NDLP_ERR, "Cannot set size for shared memory.");
        goto end_shm;
    }

    integration_shm = mmap(NULL, length, PROT_READ | PROT_WRITE, MAP_SHARED, shm_fd_ebpf_integration, 0);
    if (unlikely(MAP_FAILED == integration_shm)) {
        integration_shm = NULL;
        nd_log(NDLS_COLLECTORS, NDLP_ERR, "Cannot map shared memory used between cgroup and eBPF, integration won't happen");
        goto end_shm;
    }

    shm_mutex_ebpf_integration = sem_open(NETDATA_EBPF_SHM_INTEGRATION_NAME, O_CREAT,
                                          S_IRUSR | S_IWUSR | S_IRGRP | S_IWGRP | S_IROTH | S_IWOTH,
                                          1);
    if (shm_mutex_ebpf_integration != SEM_FAILED) {
        return 0;
    }

    nd_log(NDLS_COLLECTORS, NDLP_ERR, "Cannot create semaphore, integration between won't happen");
    munmap(integration_shm, length);
    integration_shm = NULL;

end_shm:
    return -1;
}

void netdata_integration_init(enum netdata_integration_selector idx)
{
    if (command_server_initialized)
        return;

    if (netdata_integration_initialize_shm())
        return;

    completion_init(&completion);

    int error = uv_thread_create(&thread, netdata_integration_parser_thread, NULL);;
    if (error) {
        netdata_log_error("uv_thread_create(): %s", uv_strerror(error));
        goto after_error;
    }
    /* wait for worker thread to initialize */
    completion_wait_for(&completion);
    completion_destroy(&completion);

    if (command_thread_error) {
        error = uv_thread_join(&thread);
        if (error) {
            netdata_log_error("uv_thread_create(): %s", uv_strerror(error));
        }
        goto after_error;
    }

    command_server_initialized = 1;
    return;

after_error:
    netdata_log_error("Failed to initialize command server. The netdata cli tool will be unable to send commands.");
}

const char *netdata_integration_pipename()
{
    const char *pipename = getenv(pipes);
    if (pipename)
        return pipename;

#ifdef _WIN32
    return "\\\\?\\pipe\\netdata-ebpf-integration";
#else
    return "/tmp/netdata-ebpf-integration";
#endif
}



#endif // defined(OS_LINUX)
