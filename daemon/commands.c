// SPDX-License-Identifier: GPL-3.0-or-later

#include "common.h"

static uv_thread_t thread;
static uv_loop_t* loop;
static uv_async_t async;
static struct completion completion;
static uv_pipe_t server_pipe;
static uv_pipe_t client_pipe;
#ifdef _WIN32
# define PIPENAME "\\\\?\\pipe\\netdata-cli"
#else
# define PIPENAME "/tmp/netdata-cli"
#endif

static int command_thread_error = 0;
static int command_thread_shutdown = 0;
static unsigned clients = 0;
#define MAX_COMMAND_LENGTH 4096
char command_string[MAX_COMMAND_LENGTH];
unsigned command_string_size;

char claiming_token[MAX_COMMAND_LENGTH] = {'\0', };
unsigned claiming_status = 0;

static void pipe_write_cb(uv_write_t* req, int status)
{
    //assert(status == 0);
    uv_close((uv_handle_t *)&client_pipe, NULL);
    --clients;
    info("Command Clients = %u\n", clients);
}

static inline void add_string_to_command_reply(char *reply_string, unsigned *reply_string_size, char *str)
{
    unsigned len;

    len = strlen(str);
    strncpyz(reply_string + *reply_string_size, str, len);
    *reply_string_size += len;
}

static void send_command_reply(uv_stream_t *client, int success, char *message)
{
    int ret;
    char reply_string[MAX_COMMAND_LENGTH] = {'\0', };
    unsigned reply_string_size = 0;
    uv_buf_t write_buf;
    uv_write_t write_req;

    add_string_to_command_reply(reply_string, &reply_string_size, (success) ? "SUCCESS" : "FAILURE");
    if (message) {
        add_string_to_command_reply(reply_string, &reply_string_size, ": ");
        add_string_to_command_reply(reply_string, &reply_string_size, message);
    }
    add_string_to_command_reply(reply_string, &reply_string_size, "\n");

    write_buf.base = reply_string;
    write_buf.len = strlen((char *) write_buf.base);
    ret = uv_write(&write_req, (uv_stream_t *) client, &write_buf, 1, pipe_write_cb);
    if (ret) {
        error("uv_write(): %s", uv_strerror(ret));
    }
    info("COMMAND: Sending reply: \"%s\"", reply_string);
}

static void parse_commands(uv_stream_t *client)
{
    char *message = NULL, *pos;
    int success = 0;

    if (strstr(command_string, "help")) {
        /* Show commands */
        success = 1;
        message = strdupz("The commands are (arguments are in brackets):\n"
                  "\thelp\n"
                  "\tclaiming status\n"
                  "\tset claiming token [token]\n"
                  "\tclear claiming token\n"
                  "\treload health\n"
                  "\tsave database\n"
                  "\tlog rotate\n"
                  "\texit\n"
                  "\tfatal");
    } else if (strstr(command_string, "claiming status")) {
        /* Agent Claiming */
        message = mallocz(MAX_COMMAND_LENGTH);
        snprintfz(message, MAX_COMMAND_LENGTH - 1, "claiming status=%s token=\"%s\"",
                  claiming_status ? "claimed" : "unclaimed", claiming_token);
        success = 1;
    } else if (pos = strstr(command_string, "set claiming token ")) {
        if (strlen(claiming_token)) {
            success = 0;
        } else {
            strncpyz(claiming_token, pos + strlen("set claiming token "), MAX_COMMAND_LENGTH - 1);
            success = 1;
        }
    } else if (strstr(command_string, "clear claiming token")) {
        claiming_token[0] = '\0';
        success = 1;
    } else if (strstr(command_string, "reload health")) {
        info("COMMAND: Reloading HEALTH configuration.");
        health_reload();
        error_log_limit_reset();
        success = 1;
    } else if (strstr(command_string, "save database")) {
        error_log_limit_unlimited();
        info("COMMAND: Saving databases.");
        rrdhost_save_all();
        info("Databases saved.");
        error_log_limit_reset();
        success = 1;
    } else if (strstr(command_string, "log rotate")) {
        error_log_limit_unlimited();
        info("COMMAND: Reopening all log files.");
        reopen_all_log_files();
        error_log_limit_reset();
        success = 1;
    } else if (strstr(command_string, "exit")) {
        error_log_limit_unlimited();
        info("COMMAND: Cleaning up to exit.");
        netdata_cleanup_and_exit(0);
        exit(0);
    } else if (strstr(command_string, "fatal")) {
        fatal("COMMAND: netdata now exits.");
    }
    send_command_reply(client, success, message);
    if (message)
        freez(message);
}

static void pipe_read_cb(uv_stream_t *client, ssize_t nread, const uv_buf_t *buf)
{
    int ret;

    if (0 == nread) {
        info("%s: Zero bytes read by command pipe.", __func__);
    } else if (UV_EOF == nread) {
        info("EOF found in command pipe.");
        parse_commands(client);
    } else if (nread < 0) {
        error("%s: %s", __func__, uv_strerror(nread));
    }

    if (nread < 0) { /* stop stream due to EOF or error */
        ret = uv_read_stop((uv_stream_t *)client);
    } else if (nread) {
        size_t to_copy;

        to_copy = MIN(buf->len, MAX_COMMAND_LENGTH - 1 - command_string_size);
        strncpyz(command_string + command_string_size, buf->base, to_copy);
    }
    if (buf && buf->len) {
        freez(buf->base);
    }

    if (nread < 0 && UV_EOF != nread) {
        uv_close((uv_handle_t*)client, NULL);
        --clients;
        info("Command Clients = %u\n", clients);
    }
}

static void alloc_cb(uv_handle_t *handle, size_t suggested_size, uv_buf_t *buf)
{
    buf->base = mallocz(suggested_size);
    buf->len = suggested_size;
}

static void connection_cb(uv_stream_t *server, int status)
{
    int ret;
    assert(status == 0);

    ret = uv_pipe_init(server->loop, &client_pipe, 1);
    if (ret) {
        error("uv_pipe_init(): %s", uv_strerror(ret));
        return;
    }
    ret = uv_accept(server, (uv_stream_t *)&client_pipe);
    if (ret) {
        error("uv_accept(): %s", uv_strerror(ret));
        return;
    }

    ++clients;
    info("Command Clients = %u\n", clients);
    /* Start parsing a new command */
    command_string_size = 0;
    command_string[0] = '\0';

    ret = uv_read_start((uv_stream_t*)&client_pipe, alloc_cb, pipe_read_cb);
    if (ret) {
        error("uv_read_start(): %s", uv_strerror(ret));
        uv_close((uv_handle_t *)&client_pipe, NULL);
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
    uv_timer_t timer_req;
    uv_fs_t req;

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
    if ((ret = uv_listen((uv_stream_t *)&server_pipe, 1, connection_cb))) {
        error("uv_listen(): %s", uv_strerror(ret));
        goto error_after_uv_listen;
    }

    command_thread_error = 0;
    command_thread_shutdown = 0;
    /* wake up initialization thread */
    complete(&completion);

//    assert(0 == uv_timer_start(&timer_req, timer_cb, TIMER_PERIOD_MS, TIMER_PERIOD_MS));
    while (command_thread_shutdown == 0 || uv_loop_alive(loop)) {
        uv_run(loop, UV_RUN_DEFAULT);
    }
    uv_close((uv_handle_t *)&async, NULL);
    /* cleanup operations of the event loop */
    info("Shutting down command event loop.");
    uv_run(loop, UV_RUN_DEFAULT);

    info("Shutting down command loop complete.");
    assert(0 == uv_loop_close(loop));
    freez(loop);

    return;

error_after_async_init:
error_after_uv_listen:
error_after_pipe_bind:
    uv_close((uv_handle_t *)&async, NULL);
error_after_pipe_init:
    assert(0 == uv_loop_close(loop));
error_after_loop_init:
    freez(loop);

    /* wake up initialization thread */
    complete(&completion);
}


/*
 * Returns 0 on success, negative on error
 */
int commands_init(void)
{
    struct rrdengine_instance *ctx;
    int error;

    init_completion(&completion);
    assert(0 == uv_thread_create(&thread, command_thread, NULL));
    /* wait for worker thread to initialize */
    wait_for_completion(&completion);
    destroy_completion(&completion);

    if (command_thread_error) {
        assert(0 == uv_thread_join(&thread));
    }
    return command_thread_error;
}

/*
 * Returns 0 on success, 1 on error
 */
int commands_exit(void)
{
    command_thread_shutdown = 1;
    /* wake up event loop */
    assert(0 == uv_async_send(&async));

    assert(0 == uv_thread_join(&thread));

    return 0;
}