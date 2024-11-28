#include "CLI/CLI.hpp"
#include "daemon/pipename.h"
#include "daemon/common.h"
#include "libnetdata/required_dummies.h"
#include <iostream>
#include <string>

static uv_pipe_t client_pipe;
static uv_write_t write_req;
static uv_shutdown_t shutdown_req;
static char command_string[MAX_COMMAND_LENGTH];
static unsigned command_string_size;
static int exit_status;

static void parse_command_reply(BUFFER *buf)
{
    char *response_string = (char *)buffer_tostring(buf);
    unsigned response_string_size = buffer_strlen(buf);
    FILE *stream = NULL;
    char *pos;
    int syntax_error = 0;

    for (pos = response_string; pos < response_string + response_string_size && !syntax_error; ++pos) {
        /* Skip white-space characters */
        for (; isspace(*pos) && ('\0' != *pos); ++pos)
            ;

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
    BUFFER *response = static_cast<BUFFER *>(client->data);

    if (0 == nread)
        fprintf(stderr, "%s: Zero bytes read by command pipe.\n", __func__);
    else if (UV_EOF == nread)
        parse_command_reply(response);
    else if (nread < 0) {
        fprintf(stderr, "%s: %s\n", __func__, uv_strerror(nread));
        (void)uv_read_stop((uv_stream_t *)client);
    } else
        buffer_fast_rawcat(response, buf->base, nread);

    if (buf && buf->len)
        free(buf->base);
}

static void alloc_cb(uv_handle_t *handle, size_t suggested_size, uv_buf_t *buf)
{
    (void)handle;
    buf->base = (char *)malloc(suggested_size);
    buf->len = suggested_size;
}

static void shutdown_cb(uv_shutdown_t *req, int status)
{
    int ret;
    (void)status;

    client_pipe.data = req->data;
    ret = uv_read_start((uv_stream_t *)&client_pipe, alloc_cb, pipe_read_cb);
    if (ret) {
        fprintf(stderr, "uv_read_start(): %s\n", uv_strerror(ret));
        uv_close((uv_handle_t *)&client_pipe, NULL);
        return;
    }
}

static void pipe_write_cb(uv_write_t *req, int status)
{
    int ret;
    (void)status;

    uv_pipe_t *clientp = (uv_pipe_t *)req->data;
    shutdown_req.data = clientp->data;

    ret = uv_shutdown(&shutdown_req, (uv_stream_t *)&client_pipe, shutdown_cb);
    if (ret) {
        fprintf(stderr, "uv_shutdown(): %s\n", uv_strerror(ret));
        uv_close((uv_handle_t *)&client_pipe, NULL);
        return;
    }
}

static void connect_cb(uv_connect_t *req, int status)
{
    int ret;
    uv_buf_t write_buf;

    if (status) {
        fprintf(stderr, "uv_pipe_connect(): %s\n", uv_strerror(status));
        fprintf(stderr, "Make sure the netdata service is running.\n");
        exit(-1);
    }

    client_pipe.data = req->data;
    write_req.data = &client_pipe;
    write_buf.base = command_string;
    write_buf.len = command_string_size;
    ret = uv_write(&write_req, (uv_stream_t *)&client_pipe, &write_buf, 1, pipe_write_cb);
    if (ret) {
        fprintf(stderr, "uv_write(): %s\n", uv_strerror(ret));
    }
}

int main(int argc, char **argv)
{
    nd_log_initialize_for_external_plugins("netdatacli");

    CLI::App app{"Netdata CLI Tool"};

    // Disable default help flag since we'll handle it ourselves
    app.set_help_flag("");

    // Variables to store option states
    bool reload_health = false;
    bool reload_labels = false;
    bool reopen_logs = false;
    bool shutdown_agent = false;
    bool fatal_agent = false;
    bool reload_claiming_state = false;
    bool ping = false;
    bool dumpconfig = false;
    bool version = false;
    std::string aclk_state;
    std::string remove_stale_node;

    app.set_help_flag("--help", "Show this help message and exit");
    app.add_flag("--reload-health", reload_health, "Reload health configuration");
    app.add_flag("--reload-labels", reload_labels, "Reload all labels");
    app.add_flag("--reopen-logs", reopen_logs, "Close and reopen log files");
    app.add_flag("--shutdown-agent", shutdown_agent, "Cleanup and exit the netdata agent");
    app.add_flag("--fatal-agent", fatal_agent, "Log the state and halt the netdata agent");
    app.add_flag("--reload-claiming-state", reload_claiming_state, "Reload agent claiming state from disk");
    app.add_flag("--ping", ping, "Return with 'pong' if agent is alive");
    app.add_option(
        "--aclk-state", aclk_state, "Returns current state of ACLK and Cloud connection. Use 'json' for JSON format");
    app.add_flag("--dump-config", dumpconfig, "Returns the current netdata.conf on stdout");
    app.add_option(
        "--remove-stale-node",
        remove_stale_node,
        "Unregisters and removes a node from the cloud. Specify node_id, machine_guid, hostname, or ALL_NODES");
    app.add_flag("--version", version, "Returns the netdata version");

    // Parse command line
    try {
        app.parse(argc, argv);
    } catch (const CLI::ParseError &e) {
        return app.exit(e);
    }

    if (argc == 1) {
        std::cout << app.help() << std::endl;
        return 0;
    }

    // Convert options to command string
    if (reload_health) {
        command_string_size = snprintf(command_string, MAX_COMMAND_LENGTH, "reload-health");
    } else if (reload_labels) {
        command_string_size = snprintf(command_string, MAX_COMMAND_LENGTH, "reload-labels");
    } else if (reopen_logs) {
        command_string_size = snprintf(command_string, MAX_COMMAND_LENGTH, "reopen-logs");
    } else if (shutdown_agent) {
        command_string_size = snprintf(command_string, MAX_COMMAND_LENGTH, "shutdown-agent");
    } else if (fatal_agent) {
        command_string_size = snprintf(command_string, MAX_COMMAND_LENGTH, "fatal-agent");
    } else if (reload_claiming_state) {
        command_string_size = snprintf(command_string, MAX_COMMAND_LENGTH, "reload-claiming-state");
    } else if (ping) {
        command_string_size = snprintf(command_string, MAX_COMMAND_LENGTH, "ping");
    } else if (!aclk_state.empty()) {
        command_string_size = snprintf(command_string, MAX_COMMAND_LENGTH, "aclk-state %s", aclk_state.c_str());
    } else if (dumpconfig) {
        command_string_size = snprintf(command_string, MAX_COMMAND_LENGTH, "dumpconfig");
    } else if (!remove_stale_node.empty()) {
        command_string_size =
            snprintf(command_string, MAX_COMMAND_LENGTH, "remove-stale-node %s", remove_stale_node.c_str());
    } else if (version) {
        command_string_size = snprintf(command_string, MAX_COMMAND_LENGTH, "version");
    }

    // Initialize event loop and pipe
    exit_status = -1;
    uv_loop_t *loop = uv_default_loop();
    int ret = uv_pipe_init(loop, &client_pipe, 1);
    if (ret) {
        fprintf(stderr, "uv_pipe_init(): %s\n", uv_strerror(ret));
        return exit_status;
    }

    // Connect and send command
    uv_connect_t connect_req;
    connect_req.data = buffer_create(128, NULL);
    const char *pipename = daemon_pipename();
    uv_pipe_connect(&connect_req, &client_pipe, pipename, connect_cb);

    uv_run(loop, UV_RUN_DEFAULT);

    // Cleanup
    uv_close((uv_handle_t *)&client_pipe, NULL);
    buffer_free(static_cast<BUFFER *>(client_pipe.data));

    return exit_status;
}
