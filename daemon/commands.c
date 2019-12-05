// SPDX-License-Identifier: GPL-3.0-or-later

#include "common.h"
#include "../database/engine/rrdenginelib.h"

static uv_thread_t thread;
static uv_loop_t* loop;
static uv_async_t async;
static struct completion completion;
static uv_pipe_t server_pipe;

char cmd_prefix_by_status[] = {
        CMD_PREFIX_INFO,
        CMD_PREFIX_ERROR,
        CMD_PREFIX_ERROR
};

static int command_thread_error;
static int command_thread_shutdown;
static unsigned clients = 0;
static char command_string[MAX_COMMAND_LENGTH];
static unsigned command_string_size;

struct command_context {
    uv_work_t work;
    uv_stream_t *client;
    cmd_t idx;
    char *args;
    char *message;
    cmd_status_t status;
};

/* Forward declarations */
static cmd_status_t cmd_help_execute(char *args, char **message);
static cmd_status_t cmd_reload_health_execute(char *args, char **message);
static cmd_status_t cmd_save_database_execute(char *args, char **message);
static cmd_status_t cmd_reopen_logs_execute(char *args, char **message);
static cmd_status_t cmd_exit_execute(char *args, char **message);
static cmd_status_t cmd_fatal_execute(char *args, char **message);

static command_info_t command_info_array[] = {
        {"help", cmd_help_execute, CMD_TYPE_HIGH_PRIORITY},                  // show help menu
        {"reload-health", cmd_reload_health_execute, CMD_TYPE_ORTHOGONAL},   // reload health configuration
        {"save-database", cmd_save_database_execute, CMD_TYPE_ORTHOGONAL},   // save database for memory mode save
        {"reopen-logs", cmd_reopen_logs_execute, CMD_TYPE_ORTHOGONAL},       // Close and reopen log files
        {"shutdown-agent", cmd_exit_execute, CMD_TYPE_EXCLUSIVE},            // exit cleanly
        {"fatal-agent", cmd_fatal_execute, CMD_TYPE_HIGH_PRIORITY},          // exit with fatal error
};

/* Mutexes for commands of type CMD_TYPE_ORTHOGONAL */
static uv_mutex_t command_lock_array[CMD_TOTAL_COMMANDS];
/* Commands of type CMD_TYPE_EXCLUSIVE are writers */
static uv_rwlock_t exclusive_rwlock;
/*
 * Locking order:
 * 1. exclusive_rwlock
 * 2. command_lock_array[]
 */

/* Forward declarations */
static void cmd_lock_exclusive(unsigned index);
static void cmd_lock_orthogonal(unsigned index);
static void cmd_lock_idempotent(unsigned index);
static void cmd_lock_high_priority(unsigned index);

static command_lock_t *cmd_lock_by_type[] = {
        cmd_lock_exclusive,
        cmd_lock_orthogonal,
        cmd_lock_idempotent,
        cmd_lock_high_priority
};

/* Forward declarations */
static void cmd_unlock_exclusive(unsigned index);
static void cmd_unlock_orthogonal(unsigned index);
static void cmd_unlock_idempotent(unsigned index);
static void cmd_unlock_high_priority(unsigned index);

static command_lock_t *cmd_unlock_by_type[] = {
        cmd_unlock_exclusive,
        cmd_unlock_orthogonal,
        cmd_unlock_idempotent,
        cmd_unlock_high_priority
};

static cmd_status_t cmd_help_execute(char *args, char **message)
{
    (void)args;

    *message = mallocz(MAX_COMMAND_LENGTH);
    strncpyz(*message,
             "\nThe commands are (arguments are in brackets):\n"
             "help\n"
             "    Show this help menu.\n"
             "reload-health\n"
             "    Reload health configuration.\n"
             "save-database\n"
             "    Save internal DB to disk for memory mode save.\n"
             "reopen-logs\n"
             "    Close and reopen log files.\n"
             "shutdown-agent\n"
             "    Cleanup and exit the netdata agent.\n"
             "fatal-agent\n"
             "    Log the state and halt the netdata agent.\n",
             MAX_COMMAND_LENGTH - 1);
    return CMD_STATUS_SUCCESS;
}

static cmd_status_t cmd_reload_health_execute(char *args, char **message)
{
    (void)args;
    (void)message;

    error_log_limit_unlimited();
    info("COMMAND: Reloading HEALTH configuration.");
    health_reload();
    error_log_limit_reset();

    return CMD_STATUS_SUCCESS;
}

static cmd_status_t cmd_save_database_execute(char *args, char **message)
{
    (void)args;
    (void)message;

    error_log_limit_unlimited();
    info("COMMAND: Saving databases.");
    rrdhost_save_all();
    info("COMMAND: Databases saved.");
    error_log_limit_reset();

    return CMD_STATUS_SUCCESS;
}

static cmd_status_t cmd_reopen_logs_execute(char *args, char **message)
{
    (void)args;
    (void)message;

    error_log_limit_unlimited();
    info("COMMAND: Reopening all log files.");
    reopen_all_log_files();
    error_log_limit_reset();

    return CMD_STATUS_SUCCESS;
}

static cmd_status_t cmd_exit_execute(char *args, char **message)
{
    (void)args;
    (void)message;

    error_log_limit_unlimited();
    info("COMMAND: Cleaning up to exit.");
    netdata_cleanup_and_exit(0);
    exit(0);

    return CMD_STATUS_SUCCESS;
}

static cmd_status_t cmd_fatal_execute(char *args, char **message)
{
    (void)args;
    (void)message;

    fatal("COMMAND: netdata now exits.");

    return CMD_STATUS_SUCCESS;
}

static void cmd_lock_exclusive(unsigned index)
{
    (void)index;

    uv_rwlock_wrlock(&exclusive_rwlock);
}

static void cmd_lock_orthogonal(unsigned index)
{
    uv_rwlock_rdlock(&exclusive_rwlock);
    uv_mutex_lock(&command_lock_array[index]);
}

static void cmd_lock_idempotent(unsigned index)
{
    (void)index;

    uv_rwlock_rdlock(&exclusive_rwlock);
}

static void cmd_lock_high_priority(unsigned index)
{
    (void)index;
}

static void cmd_unlock_exclusive(unsigned index)
{
    (void)index;

    uv_rwlock_wrunlock(&exclusive_rwlock);
}

static void cmd_unlock_orthogonal(unsigned index)
{
    uv_rwlock_rdunlock(&exclusive_rwlock);
    uv_mutex_unlock(&command_lock_array[index]);
}

static void cmd_unlock_idempotent(unsigned index)
{
    (void)index;

    uv_rwlock_rdunlock(&exclusive_rwlock);
}

static void cmd_unlock_high_priority(unsigned index)
{
    (void)index;
}

static void pipe_write_cb(uv_write_t* req, int status)
{
    (void)status;
    uv_pipe_t *client = req->data;

    uv_close((uv_handle_t *)client, NULL);
    freez(client);
    --clients;
    info("Command Clients = %u\n", clients);
}

static inline void add_char_to_command_reply(char *reply_string, unsigned *reply_string_size, char character)
{
    reply_string[(*reply_string_size)++] = character;
}

static inline void add_string_to_command_reply(char *reply_string, unsigned *reply_string_size, char *str)
{
    unsigned len;

    len = strlen(str);
    strncpyz(reply_string + *reply_string_size, str, len);
    *reply_string_size += len;
}

static void send_command_reply(uv_stream_t *client, cmd_status_t status, char *message)
{
    int ret;
    char reply_string[MAX_COMMAND_LENGTH] = {'\0', };
    char exit_status_string[MAX_EXIT_STATUS_LENGTH + 1] = {'\0', };
    unsigned reply_string_size = 0;
    uv_buf_t write_buf;
    uv_write_t write_req;

    snprintfz(exit_status_string, MAX_EXIT_STATUS_LENGTH, "%u", status);
    add_char_to_command_reply(reply_string, &reply_string_size, CMD_PREFIX_EXIT_CODE);
    add_string_to_command_reply(reply_string, &reply_string_size, exit_status_string);
    add_char_to_command_reply(reply_string, &reply_string_size, '\0');

    if (message) {
        add_char_to_command_reply(reply_string, &reply_string_size, cmd_prefix_by_status[status]);
        add_string_to_command_reply(reply_string, &reply_string_size, message);
    }

    write_req.data = client;
    write_buf.base = reply_string;
    write_buf.len = reply_string_size;
    ret = uv_write(&write_req, (uv_stream_t *)client, &write_buf, 1, pipe_write_cb);
    if (ret) {
        error("uv_write(): %s", uv_strerror(ret));
    }
    info("COMMAND: Sending reply: \"%s\"", reply_string);
}

cmd_status_t execute_command(cmd_t idx, char *args, char **message)
{
    cmd_status_t status;
    cmd_type_t type = command_info_array[idx].type;

    cmd_lock_by_type[type](idx);
    status = command_info_array[idx].func(args, message);
    cmd_unlock_by_type[type](idx);

    return status;
}

static void after_schedule_command(uv_work_t *req, int status)
{
    struct command_context *cmd_ctx = req->data;

    (void)status;

    send_command_reply(cmd_ctx->client, cmd_ctx->status, cmd_ctx->message);
    if (cmd_ctx->message)
        freez(cmd_ctx->message);
}

static void schedule_command(uv_work_t *req)
{
    struct command_context *cmd_ctx = req->data;

    cmd_ctx->status = execute_command(cmd_ctx->idx, cmd_ctx->args, &cmd_ctx->message);
}

static void parse_commands(uv_stream_t *client)
{
    char *message = NULL, *pos;
    cmd_t i;
    cmd_status_t status;
    struct command_context *cmd_ctx;

    status = CMD_STATUS_FAILURE;

    /* Skip white-space characters */
    for (pos = command_string ; isspace(*pos) && ('\0' != *pos) ; ++pos) {;}
    for (i = 0 ; i < CMD_TOTAL_COMMANDS ; ++i) {
        if (!strncmp(pos, command_info_array[i].cmd_str, strlen(command_info_array[i].cmd_str))) {
            cmd_ctx = mallocz(sizeof(*cmd_ctx));
            cmd_ctx->work.data = cmd_ctx;
            cmd_ctx->client = client;
            cmd_ctx->idx = i;
            cmd_ctx->args = pos + strlen(command_info_array[i].cmd_str);
            cmd_ctx->message = NULL;

            assert(0 == uv_queue_work(loop, &cmd_ctx->work, schedule_command, after_schedule_command));
            break;
        }
    }
    if (CMD_TOTAL_COMMANDS == i) {
        /* no command found */
        message = strdupz("Illegal command. Please type \"help\" for instructions.");
        send_command_reply(client, status, message);
        freez(message);
    }
}

static void pipe_read_cb(uv_stream_t *client, ssize_t nread, const uv_buf_t *buf)
{
    if (0 == nread) {
        info("%s: Zero bytes read by command pipe.", __func__);
    } else if (UV_EOF == nread) {
        info("EOF found in command pipe.");
        parse_commands(client);
    } else if (nread < 0) {
        error("%s: %s", __func__, uv_strerror(nread));
    }

    if (nread < 0) { /* stop stream due to EOF or error */
        (void)uv_read_stop((uv_stream_t *)client);
    } else if (nread) {
        size_t to_copy;

        to_copy = MIN(nread, MAX_COMMAND_LENGTH - 1 - command_string_size);
        strncpyz(command_string + command_string_size, buf->base, to_copy);
        command_string_size += to_copy;
    }
    if (buf && buf->len) {
        freez(buf->base);
    }

    if (nread < 0 && UV_EOF != nread) {
        uv_close((uv_handle_t *)client, NULL);
        freez(client);
        --clients;
        info("Command Clients = %u\n", clients);
    }
}

static void alloc_cb(uv_handle_t *handle, size_t suggested_size, uv_buf_t *buf)
{
    (void)handle;

    buf->base = mallocz(suggested_size);
    buf->len = suggested_size;
}

static void connection_cb(uv_stream_t *server, int status)
{
    int ret;
    uv_pipe_t *client;
    assert(status == 0);

    client = mallocz(sizeof(*client));
    ret = uv_pipe_init(server->loop, client, 1);
    if (ret) {
        error("uv_pipe_init(): %s", uv_strerror(ret));
        freez(client);
        return;
    }
    ret = uv_accept(server, (uv_stream_t *)client);
    if (ret) {
        error("uv_accept(): %s", uv_strerror(ret));
        uv_close((uv_handle_t *)client, NULL);
        freez(client);
        return;
    }

    ++clients;
    info("Command Clients = %u\n", clients);
    /* Start parsing a new command */
    command_string_size = 0;
    command_string[0] = '\0';

    ret = uv_read_start((uv_stream_t*)client, alloc_cb, pipe_read_cb);
    if (ret) {
        error("uv_read_start(): %s", uv_strerror(ret));
        uv_close((uv_handle_t *)client, NULL);
        freez(client);
        --clients;
        info("Command Clients = %u\n", clients);
        return;
    }
}

static void async_cb(uv_async_t *handle)
{
    uv_stop(handle->loop);
}

static void command_thread(void *arg)
{
    int ret;
    uv_fs_t req;

    (void) arg;
    loop = mallocz(sizeof(uv_loop_t));
    ret = uv_loop_init(loop);
    if (ret) {
        error("uv_loop_init(): %s", uv_strerror(ret));
        command_thread_error = ret;
        goto error_after_loop_init;
    }
    loop->data = NULL;

    ret = uv_async_init(loop, &async, async_cb);
    if (ret) {
        error("uv_async_init(): %s", uv_strerror(ret));
        command_thread_error = ret;
        goto error_after_async_init;
    }
    async.data = NULL;

    ret = uv_pipe_init(loop, &server_pipe, 1);
    if (ret) {
        error("uv_pipe_init(): %s", uv_strerror(ret));
        command_thread_error = ret;
        goto error_after_pipe_init;
    }
    (void)uv_fs_unlink(loop, &req, PIPENAME, NULL);
    uv_fs_req_cleanup(&req);
    ret = uv_pipe_bind(&server_pipe, PIPENAME);
    if (ret) {
        error("uv_pipe_bind(): %s", uv_strerror(ret));
        command_thread_error = ret;
        goto error_after_pipe_bind;
    }
    if ((ret = uv_listen((uv_stream_t *)&server_pipe, SOMAXCONN, connection_cb))) {
        error("uv_listen(): %s", uv_strerror(ret));
        command_thread_error = ret;
        goto error_after_uv_listen;
    }

    command_thread_error = 0;
    command_thread_shutdown = 0;
    /* wake up initialization thread */
    complete(&completion);

    while (command_thread_shutdown == 0) {
        uv_run(loop, UV_RUN_DEFAULT);
    }
    /* cleanup operations of the event loop */
    info("Shutting down command event loop.");
    uv_close((uv_handle_t *)&async, NULL);
    uv_close((uv_handle_t*)&server_pipe, NULL);
    uv_run(loop, UV_RUN_DEFAULT);

    info("Shutting down command loop complete.");
    assert(0 == uv_loop_close(loop));
    freez(loop);

    return;

error_after_uv_listen:
error_after_pipe_bind:
    uv_close((uv_handle_t*)&server_pipe, NULL);
error_after_pipe_init:
    uv_close((uv_handle_t *)&async, NULL);
error_after_async_init:
    assert(0 == uv_loop_close(loop));
error_after_loop_init:
    freez(loop);

    /* wake up initialization thread */
    complete(&completion);
}

static void sanity_check(void)
{
    /* The size of command_info_array must be CMD_TOTAL_COMMANDS elements */
    BUILD_BUG_ON(CMD_TOTAL_COMMANDS != sizeof(command_info_array) / sizeof(command_info_array[0]));
}

void commands_init(void)
{
    cmd_t i;
    int error;

    sanity_check();
    info("Initializing command server.");
    for (i = 0 ; i < CMD_TOTAL_COMMANDS ; ++i) {
        uv_mutex_init(&command_lock_array[i]);
    }
    assert(0 == uv_rwlock_init(&exclusive_rwlock));

    init_completion(&completion);
    error = uv_thread_create(&thread, command_thread, NULL);
    if (error) {
        error("uv_thread_create(): %s", uv_strerror(error));
        goto after_error;
    }
    /* wait for worker thread to initialize */
    wait_for_completion(&completion);
    destroy_completion(&completion);

    if (command_thread_error) {
        error = uv_thread_join(&thread);
        if (error) {
            error("uv_thread_create(): %s", uv_strerror(error));
        }
        goto after_error;
    }
    return;

after_error:
    error("Failed to initialize command server.");
}

void commands_exit(void)
{
    cmd_t i;

    command_thread_shutdown = 1;
    info("Shutting down command server.");
    /* wake up event loop */
    assert(0 == uv_async_send(&async));
    assert(0 == uv_thread_join(&thread));

    for (i = 0 ; i < CMD_TOTAL_COMMANDS ; ++i) {
        uv_mutex_destroy(&command_lock_array[i]);
    }
    uv_rwlock_destroy(&exclusive_rwlock);
    info("Command server has stopped.");
}
