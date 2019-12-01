// SPDX-License-Identifier: GPL-3.0-or-later

#include "cli.h"
#include "../libnetdata/required_dummies.h"

static uv_pipe_t client_pipe;
static uv_write_t write_req;
static uv_shutdown_t shutdown_req;

static char command_string[MAX_COMMAND_LENGTH];
static unsigned command_string_size;

static char response_string[MAX_COMMAND_LENGTH];
static unsigned response_string_size;

static int exit_status;

struct command_context {
    uv_work_t work;
    uv_stream_t *client;
    cmd_t idx;
    char *args;
    char *message;
    cmd_status_t status;
};

static void parse_command_reply(void)
{
    FILE *stream = NULL;
    char *pos;
    int syntax_error = 0;

    for (pos = response_string ;
         pos < response_string + response_string_size  && !syntax_error ;
         ++pos) {
        /* Skip white-space characters */
        for ( ; isspace(*pos) && ('\0' != *pos); ++pos) {;}

        if ('\0' == *pos)
            continue;

        switch (*pos) {
        case CMD_PREFIX_EXIT_CODE:
            exit_status = atoi(++pos);
            break;
        case CMD_PREFIX_INFO:
            stream = stdout;
            break;
        case CMD_PREFIX_ERROR:
            stream = stderr;
            break;
        default:
            syntax_error = 1;
            fprintf(stderr, "Syntax error, failed to parse command response.\n");
            break;
        }
        if (stream) {
            fprintf(stream, "%s\n", ++pos);
            pos += strlen(pos);
            stream = NULL;
        }
    }
}

static void pipe_read_cb(uv_stream_t *client, ssize_t nread, const uv_buf_t *buf)
{
    if (0 == nread) {
        fprintf(stderr, "%s: Zero bytes read by command pipe.\n", __func__);
    } else if (UV_EOF == nread) {
//      fprintf(stderr, "EOF found in command pipe.\n");
        parse_command_reply();
    } else if (nread < 0) {
        fprintf(stderr, "%s: %s\n", __func__, uv_strerror(nread));
    }

    if (nread < 0) { /* stop stream due to EOF or error */
        (void)uv_read_stop((uv_stream_t *)client);
    } else if (nread) {
        size_t to_copy;

        to_copy = MIN(nread, MAX_COMMAND_LENGTH - 1 - response_string_size);
        memcpy(response_string + response_string_size, buf->base, to_copy);
        response_string_size += to_copy;
        response_string[response_string_size] = '\0';
    }
    if (buf && buf->len) {
        free(buf->base);
    }
}

static void alloc_cb(uv_handle_t *handle, size_t suggested_size, uv_buf_t *buf)
{
    (void)handle;

    buf->base = malloc(suggested_size);
    buf->len = suggested_size;
}

static void shutdown_cb(uv_shutdown_t* req, int status)
{
    int ret;

    (void)req;
    (void)status;

    /* receive reply */
    response_string_size = 0;
    response_string[0] = '\0';

    ret = uv_read_start((uv_stream_t *)&client_pipe, alloc_cb, pipe_read_cb);
    if (ret) {
        fprintf(stderr, "uv_read_start(): %s\n", uv_strerror(ret));
        uv_close((uv_handle_t *)&client_pipe, NULL);
        return;
    }

}

static void pipe_write_cb(uv_write_t* req, int status)
{
    int ret;

    (void)req;
    (void)status;

    ret = uv_shutdown(&shutdown_req, (uv_stream_t *)&client_pipe, shutdown_cb);
    if (ret) {
        fprintf(stderr, "uv_shutdown(): %s\n", uv_strerror(ret));
        uv_close((uv_handle_t *)&client_pipe, NULL);
        return;
    }
}

static void connect_cb(uv_connect_t* req, int status)
{
    int ret;
    uv_buf_t write_buf;
    char *s;

    (void)req;
    if (status) {
        fprintf(stderr, "uv_pipe_connect(): %s\n", uv_strerror(status));
        fprintf(stderr, "Make sure the netdata service is running.\n");
        exit(-1);
    }
    if (0 == command_string_size) {
        s = fgets(command_string, MAX_COMMAND_LENGTH, stdin);
    }
    (void)s; /* We don't need input to communicate with the server */
    command_string_size = strlen(command_string);

    write_req.data = &client_pipe;
    write_buf.base = command_string;
    write_buf.len = command_string_size;
    ret = uv_write(&write_req, (uv_stream_t *)&client_pipe, &write_buf, 1, pipe_write_cb);
    if (ret) {
        fprintf(stderr, "uv_write(): %s\n", uv_strerror(ret));
    }
//  fprintf(stderr, "COMMAND: Sending command: \"%s\"\n", command_string);
}

int main(int argc, char **argv)
{
    int ret, i;
    static uv_loop_t* loop;
    uv_connect_t req;

    exit_status = -1; /* default status for when there is no command response from server */

    loop = uv_default_loop();

    ret = uv_pipe_init(loop, &client_pipe, 1);
    if (ret) {
        fprintf(stderr, "uv_pipe_init(): %s\n", uv_strerror(ret));
        return exit_status;
    }

    command_string_size = 0;
    command_string[0] = '\0';
    for (i = 1 ; i < argc ; ++i) {
        size_t to_copy;

        to_copy = MIN(strlen(argv[i]), MAX_COMMAND_LENGTH - 1 - command_string_size);
        strncpyz(command_string + command_string_size, argv[i], to_copy);
        command_string_size += to_copy;

        if (command_string_size < MAX_COMMAND_LENGTH - 1) {
            command_string[command_string_size++] = ' ';
        } else {
            break;
        }
    }

    uv_pipe_connect(&req, &client_pipe, PIPENAME, connect_cb);

    uv_run(loop, UV_RUN_DEFAULT);

    uv_close((uv_handle_t *)&client_pipe, NULL);

    return exit_status;
}