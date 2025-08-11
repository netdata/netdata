// SPDX-License-Identifier: GPL-3.0-or-later

#include "common.h"

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

static int command_server_initialized = 0;
static int command_thread_error;
static int command_thread_shutdown;
static unsigned clients = 0;

struct command_context {
    /* embedded client pipe structure at address 0 */
    uv_pipe_t client;

    uv_work_t work;
    uv_write_t write_req;
    cmd_t idx;
    char *args;
    char *message;
    cmd_status_t status;
    char command_string[MAX_COMMAND_LENGTH];
    unsigned command_string_size;
};

/* Forward declarations */
static cmd_status_t cmd_help_execute(char *args, char **message);
static cmd_status_t cmd_reload_health_execute(char *args, char **message);
static cmd_status_t cmd_reopen_logs_execute(char *args, char **message);
static cmd_status_t cmd_exit_execute(char *args, char **message);
static cmd_status_t cmd_fatal_execute(char *args, char **message);
static cmd_status_t cmd_reload_claiming_state_execute(char *args, char **message);
static cmd_status_t cmd_reload_labels_execute(char *args, char **message);
static cmd_status_t cmd_read_config_execute(char *args, char **message);
static cmd_status_t cmd_write_config_execute(char *args, char **message);
static cmd_status_t cmd_ping_execute(char *args, char **message);
static cmd_status_t cmd_aclk_state(char *args, char **message);
static cmd_status_t cmd_version(char *args, char **message);
static cmd_status_t cmd_dumpconfig(char *args, char **message);
static cmd_status_t cmd_remove_stale_node(char *args, char **message);
static cmd_status_t cmd_mark_stale_nodes_ephemeral(char *args, char **message);
static cmd_status_t cmd_update_node_info(char *args, char **message);

static command_info_t command_info_array[] = {
    {"help", "", "Show this help menu.", cmd_help_execute, CMD_TYPE_HIGH_PRIORITY},                // show help menu
    {"reload-health", "", "Reload health configuration.", cmd_reload_health_execute, CMD_TYPE_ORTHOGONAL}, // reload health configuration
    {"reopen-logs", "", "Close and reopen log files.", cmd_reopen_logs_execute, CMD_TYPE_ORTHOGONAL},     // Close and reopen log files
    {"shutdown-agent", "", "Cleanup and exit the netdata agent.", cmd_exit_execute, CMD_TYPE_EXCLUSIVE},          // exit cleanly
    {"fatal-agent", "", "Log the state and halt the netdata agent.", cmd_fatal_execute, CMD_TYPE_HIGH_PRIORITY},        // exit with fatal error
    {"reload-claiming-state", "", "Reload agent claiming state from disk.", cmd_reload_claiming_state_execute, CMD_TYPE_ORTHOGONAL}, // reload claiming state
    {"reload-labels", "", "Reload all localhost labels.", cmd_reload_labels_execute, CMD_TYPE_ORTHOGONAL},                 // reload the labels
    {"read-config", "", "", cmd_read_config_execute, CMD_TYPE_CONCURRENT},
    {"write-config", "", "", cmd_write_config_execute, CMD_TYPE_ORTHOGONAL},
    {"ping", "", "Return with 'pong' if agent is alive.", cmd_ping_execute, CMD_TYPE_ORTHOGONAL},
    {"aclk-state", "[json]",  "Returns current state of ACLK and Netdata Cloud connection. (optionally in json).", cmd_aclk_state, CMD_TYPE_ORTHOGONAL},
    {"version", "", "Returns the netdata version.", cmd_version, CMD_TYPE_ORTHOGONAL},
    {"dumpconfig", "", "Returns the current netdata.conf on stdout.", cmd_dumpconfig, CMD_TYPE_ORTHOGONAL},
    {"mark-stale-nodes-ephemeral", "<node_id | machine_guid | hostname | ALL_NODES>",
        "Marks one or all disconnected nodes as ephemeral, while keeping their retention\n      available for queries on both this Netdata Agent dashboard and Netdata Cloud", cmd_mark_stale_nodes_ephemeral, CMD_TYPE_ORTHOGONAL},
    {"remove-stale-node", "<node_id | machine_guid | hostname | ALL_NODES>",
     "Marks one or all disconnected nodes as ephemeral, and removes them\n      so that they are no longer available for queries, from both this\n      Netdata Agent dashboard and Netdata Cloud.", cmd_remove_stale_node, CMD_TYPE_ORTHOGONAL},
    {"update-node-info", "", "Schedules an node update message for localhost to Netdata Cloud.", cmd_update_node_info, CMD_TYPE_ORTHOGONAL},
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
    CLEAN_BUFFER *wb = buffer_create(0, NULL);

    buffer_strcat(wb, "The commands are:\n\n");
    for(size_t i = 0; i < _countof(command_info_array); i++) {
        const command_info_t *t = &command_info_array[i];
        if(!t->help || !t->help[0]) continue;

        buffer_strcat(wb, "  ");
        buffer_strcat(wb, t->cmd_str);
        if(t->params && t->params[0]) {
            buffer_putc(wb, ' ');
            buffer_strcat(wb, t->params);
        }
        buffer_putc(wb, '\n');
        buffer_strcat(wb, "      ");
        buffer_strcat(wb, t->help);
        buffer_strcat(wb, "\n\n");
    }

    *message = strdupz(buffer_tostring(wb));
    return CMD_STATUS_SUCCESS;
}

static cmd_status_t cmd_reload_health_execute(char *args, char **message)
{
    (void)args;
    (void)message;

    nd_log_limits_unlimited();
    netdata_log_info("COMMAND: Reloading HEALTH configuration.");
    health_plugin_reload();
    nd_log_limits_reset();

    return CMD_STATUS_SUCCESS;
}

static cmd_status_t cmd_reopen_logs_execute(char *args, char **message)
{
    (void)args;
    (void)message;

    nd_log_limits_unlimited();
    nd_log_reopen_log_files(true);
    nd_log_limits_reset();

    return CMD_STATUS_SUCCESS;
}

static cmd_status_t cmd_exit_execute(char *args, char **message)
{
    (void)args;
    (void)message;

    nd_log_limits_unlimited();
    netdata_log_info("COMMAND: Cleaning up to exit.");
    netdata_exit_gracefully(EXIT_REASON_CMD_EXIT, true);
    return CMD_STATUS_SUCCESS;
}

static cmd_status_t cmd_fatal_execute(char *args, char **message)
{
    (void)args;
    (void)message;

    fatal("COMMAND: netdata now exits.");

    return CMD_STATUS_SUCCESS;
}

static cmd_status_t cmd_reload_claiming_state_execute(char *args __maybe_unused, char **message) {
    char msg[1024];

    CLOUD_STATUS status = claim_reload_and_wait_online();
    switch(status) {
        case CLOUD_STATUS_ONLINE:
            snprintfz(msg, sizeof(msg),
                      "Netdata Agent is claimed to Netdata Cloud and is currently online.");
            break;

        case CLOUD_STATUS_BANNED:
            snprintfz(msg, sizeof(msg),
                      "Netdata Agent is claimed to Netdata Cloud, but it is banned.");
            break;

        default:
        case CLOUD_STATUS_AVAILABLE:
            snprintfz(msg, sizeof(msg),
                      "Netdata Agent is not claimed to Netdata Cloud: %s",
                      claim_agent_failure_reason_get());
            break;

        case CLOUD_STATUS_OFFLINE:
            snprintfz(msg, sizeof(msg),
                      "Netdata Agent is claimed to Netdata Cloud, but it is currently offline: %s",
                      cloud_status_aclk_offline_reason());
            break;

        case CLOUD_STATUS_INDIRECT:
            snprintfz(msg, sizeof(msg),
                      "Netdata Agent is not claimed to Netdata Cloud, but it is currently online via parent.");
            break;
    }

    *message = strdupz(msg);

    return CMD_STATUS_SUCCESS;
}

static cmd_status_t cmd_reload_labels_execute(char *args, char **message)
{
    (void)args;
    netdata_log_info("COMMAND: reloading host labels.");
    reload_host_labels();
    aclk_queue_node_info(localhost, 1);

    BUFFER *wb = buffer_create(10, NULL);
    rrdlabels_log_to_buffer(localhost->rrdlabels, wb);
    (*message)=strdupz(buffer_tostring(wb));
    buffer_free(wb);

    return CMD_STATUS_SUCCESS;
}

static cmd_status_t cmd_read_config_execute(char *args, char **message)
{
    size_t n = strlen(args);
    char *separator = strchr(args,'|');
    if (separator == NULL)
        return CMD_STATUS_FAILURE;
    char *separator2 = strchr(separator + 1,'|');
    if (separator2 == NULL)
        return CMD_STATUS_FAILURE;

    char *temp = callocz(n + 1, 1);
    strcpy(temp, args);
    size_t offset = separator - args;
    temp[offset] = 0;
    size_t offset2 = separator2 - args;
    temp[offset2] = 0;

    const char *conf_file = temp; /* "cloud" is cloud.conf, otherwise netdata.conf */
    struct config *tmp_config = strcmp(conf_file, "cloud") ? &netdata_config : &cloud_config;

    const char *value = inicfg_get(tmp_config, temp + offset + 1, temp + offset2 + 1, NULL);
    if (value == NULL) {
        netdata_log_error("Cannot execute read-config conf_file=%s section=%s / key=%s because no value set",
                          conf_file,
                          temp + offset + 1,
                          temp + offset2 + 1);
        freez(temp);
        return CMD_STATUS_FAILURE;
    }
    else {
        (*message) = strdupz(value);
        freez(temp);
        return CMD_STATUS_SUCCESS;
    }
}

static cmd_status_t cmd_write_config_execute(char *args, char **message)
{
    UNUSED(message);
    netdata_log_info("write-config %s", args);
    size_t n = strlen(args);
    char *separator = strchr(args,'|');
    if (separator == NULL)
        return CMD_STATUS_FAILURE;
    char *separator2 = strchr(separator + 1,'|');
    if (separator2 == NULL)
        return CMD_STATUS_FAILURE;
    char *separator3 = strchr(separator2 + 1,'|');
    if (separator3 == NULL)
        return CMD_STATUS_FAILURE;
    char *temp = callocz(n + 1, 1);
    strcpy(temp, args);
    size_t offset = separator - args;
    temp[offset] = 0;
    size_t offset2 = separator2 - args;
    temp[offset2] = 0;
    size_t offset3 = separator3 - args;
    temp[offset3] = 0;

    const char *conf_file = temp; /* "cloud" is cloud.conf, otherwise netdata.conf */
    struct config *tmp_config = strcmp(conf_file, "cloud") ? &netdata_config : &cloud_config;

    inicfg_set(tmp_config, temp + offset + 1, temp + offset2 + 1, temp + offset3 + 1);
    netdata_log_info("write-config conf_file=%s section=%s key=%s value=%s",conf_file, temp + offset + 1, temp + offset2 + 1,
         temp + offset3 + 1);
    freez(temp);
    return CMD_STATUS_SUCCESS;
}

static cmd_status_t cmd_ping_execute(char *args, char **message)
{
    (void)args;

    *message = strdupz("pong");

    return CMD_STATUS_SUCCESS;
}

static cmd_status_t cmd_aclk_state(char *args, char **message)
{
    netdata_log_info("COMMAND: Reopening aclk/cloud state.");
    if (strstr(args, "json"))
        *message = aclk_state_json();
    else
        *message = aclk_state();

    return CMD_STATUS_SUCCESS;
}

static cmd_status_t cmd_version(char *args, char **message)
{
    (void)args;

    char version[MAX_COMMAND_LENGTH];
    snprintfz(version, MAX_COMMAND_LENGTH -1, "%s %s", program_name, NETDATA_VERSION);

    *message = strdupz(version);

    return CMD_STATUS_SUCCESS;
}

static cmd_status_t cmd_dumpconfig(char *args, char **message)
{
    (void)args;

    BUFFER *wb = buffer_create(1024, NULL);
    inicfg_generate(&netdata_config, wb, 0, true);
    *message = strdupz(buffer_tostring(wb));
    buffer_free(wb);
    return CMD_STATUS_SUCCESS;
}

static int remove_ephemeral_host(BUFFER *wb, RRDHOST *host, bool report_error, bool unregister)
{
    if (host == localhost) {
        if (report_error)
            buffer_sprintf(wb, "Node '%s' (machine guid: %s) is our localhost - not changing it",
                           rrdhost_hostname(host), host->machine_guid);
        return 0;
    }

    if (rrdhost_is_online(host)) {
        if (report_error)
            buffer_sprintf(wb, "Node '%s' (machine guid: %s) is online - not changing it",
                           rrdhost_hostname(host), host->machine_guid);
        return 0;
    }

    bool marked = false;
    if (!rrdhost_option_check(host, RRDHOST_OPTION_EPHEMERAL_HOST)) {
        rrdhost_option_set(host, RRDHOST_OPTION_EPHEMERAL_HOST);
        marked = true;
    }

    sql_set_host_label(&host->host_id.uuid, "_is_ephemeral", "true");
    pulse_host_status(host, 0, 0);

    if(unregister) {
        aclk_host_state_update(host, 0, 0);
        unregister_node(host->machine_guid);
        host->node_id = UUID_ZERO;
        buffer_sprintf(wb, "Node '%s' (machine guid: %s) has been unregistered",
                       rrdhost_hostname(host), host->machine_guid);
        rrd_wrlock();
        rrdhost_free___while_having_rrd_wrlock(host);
        rrd_wrunlock();
        return 1;
    }
    else if(marked) {
        buffer_sprintf(wb, "Node '%s' (machine guid: %s) has been marked ephemeral",
                       rrdhost_hostname(host), host->machine_guid);
        return 1;
    }
    else if (report_error) {
        buffer_sprintf(wb, "Node '%s' (machine guid: %s) is already ephemeral - not changing it",
                       rrdhost_hostname(host), host->machine_guid);
    }

    return 0;
}

#define SQL_HOSTNAME_TO_REMOVE "SELECT host_id FROM host WHERE (hostname = @hostname OR @hostname = 'ALL_NODES')"

static cmd_status_t cmd_remove_stale_node_internal(char *args, char **message, bool unregister)
{
    (void)args;

    BUFFER *wb = buffer_create(1024, NULL);
    if (strlen(args) == 0) {
        buffer_sprintf(wb, "Please specify a machine or node UUID or hostname");
        goto done;
    }

    RRDHOST *host = NULL;
    host = rrdhost_find_by_guid(args);
    if (!host)
        host = rrdhost_find_by_node_id(args);

    if (!host) {
        sqlite3_stmt *res = NULL;

        bool report_error = strcmp(args, "ALL_NODES") != 0;

        if (!PREPARE_STATEMENT(db_meta, SQL_HOSTNAME_TO_REMOVE, &res)) {
            buffer_sprintf(wb, "Failed to prepare database statement to check for stale nodes");
            goto done;
        }

        int param = 0;
        SQLITE_BIND_FAIL(done0, sqlite3_bind_text(res, ++param, args, -1, SQLITE_STATIC));

        param = 0;
        int cnt = 0;
        while (sqlite3_step_monitored(res) == SQLITE_ROW) {
            char guid[UUID_STR_LEN];
            uuid_unparse_lower(*(nd_uuid_t *)sqlite3_column_blob(res, 0), guid);
            host = rrdhost_find_by_guid(guid);
            if (host) {
                int rc = remove_ephemeral_host(wb, host, report_error, unregister);
                if(rc) {
                    cnt += rc;
                    buffer_fast_strcat(wb, "\n", 1);
                }
            }
        }
        if (!cnt && buffer_strlen(wb) == 0) {
            if (report_error)
                buffer_sprintf(wb, "No match for \"%s\"", args);
            else
                buffer_sprintf(wb, "No stale nodes found");
        }
    done0:
        REPORT_BIND_FAIL(res, param);
        SQLITE_FINALIZE(res);
    }
    else
        (void) remove_ephemeral_host(wb, host, true, unregister);

done:
    *message = strdupz(buffer_tostring(wb));
    buffer_free(wb);
    return CMD_STATUS_SUCCESS;
}

static cmd_status_t cmd_update_node_info(char *args __maybe_unused, char **message)
{
    if (aclk_online()) {
        schedule_node_state_update(localhost, 1000);
        CLEAN_BUFFER *wb = buffer_create(1024, NULL);
        buffer_sprintf(wb, "Node info update scheduled for \"%s\"", rrdhost_hostname(localhost));
        *message = strdupz(buffer_tostring(wb));
    }
    else
        *message = strdupz("Agent is not connected to Netdata Cloud");
    return CMD_STATUS_SUCCESS;
}

static cmd_status_t cmd_remove_stale_node(char *args, char **message)
{
    return cmd_remove_stale_node_internal(args, message, true);
}

static cmd_status_t cmd_mark_stale_nodes_ephemeral(char *args, char **message)
{
    return cmd_remove_stale_node_internal(args, message, false);
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

static void pipe_close_cb(uv_handle_t* handle)
{
    /* Also frees command context */
    freez(handle);
}

static void pipe_write_cb(uv_write_t* req, int status)
{
    (void)status;
    uv_pipe_t *client = req->data;

    uv_close((uv_handle_t *)client, pipe_close_cb);
    --clients;
    buffer_free(client->data);
    // netdata_log_info("Command Clients = %u", clients);
}

static inline void add_char_to_command_reply(BUFFER *reply_string, unsigned *reply_string_size, char character)
{
    buffer_putc(reply_string, character);
    *reply_string_size +=1;
}

static inline void add_string_to_command_reply(BUFFER *reply_string, unsigned *reply_string_size, char *str)
{
    unsigned len;

    len = strlen(str);
    buffer_fast_strcat(reply_string, str, len);
    *reply_string_size += len;
}

static void send_command_reply(struct command_context *cmd_ctx, cmd_status_t status, char *message)
{
    int ret;
    BUFFER *reply_string = buffer_create(128, NULL);

    char exit_status_string[MAX_EXIT_STATUS_LENGTH + 1] = {'\0', };
    unsigned reply_string_size = 0;
    uv_buf_t write_buf;
    uv_stream_t *client = (uv_stream_t *)(uv_pipe_t *)cmd_ctx;

    snprintfz(exit_status_string, MAX_EXIT_STATUS_LENGTH, "%u", status);
    add_char_to_command_reply(reply_string, &reply_string_size, CMD_PREFIX_EXIT_CODE);
    add_string_to_command_reply(reply_string, &reply_string_size, exit_status_string);
    add_char_to_command_reply(reply_string, &reply_string_size, '\0');

    if (message) {
        add_char_to_command_reply(reply_string, &reply_string_size, cmd_prefix_by_status[status]);
        add_string_to_command_reply(reply_string, &reply_string_size, message);
    }

    cmd_ctx->write_req.data = client;
    client->data = reply_string;
    write_buf.base = reply_string->buffer;
    write_buf.len = reply_string_size;
    ret = uv_write(&cmd_ctx->write_req, (uv_stream_t *)client, &write_buf, 1, pipe_write_cb);
    if (ret) {
        netdata_log_error("uv_write(): %s", uv_strerror(ret));
    }
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

    send_command_reply(cmd_ctx, cmd_ctx->status, cmd_ctx->message);
    if (cmd_ctx->message)
        freez(cmd_ctx->message);
}

static void schedule_command(uv_work_t *req)
{
    register_libuv_worker_jobs();
    worker_is_busy(UV_EVENT_SCHEDULE_CMD);

    struct command_context *cmd_ctx = req->data;
    cmd_ctx->status = execute_command(cmd_ctx->idx, cmd_ctx->args, &cmd_ctx->message);

    worker_is_idle();
}

/* This will alter the state of the command_info_array.cmd_str
*/
static void parse_commands(struct command_context *cmd_ctx)
{
    char *message = NULL, *pos, *lstrip, *rstrip;
    cmd_t i;
    cmd_status_t status;

    status = CMD_STATUS_FAILURE;

    /* Skip white-space characters */
    for (pos = cmd_ctx->command_string ; isspace((uint8_t)*pos) && ('\0' != *pos) ; ++pos) ;
    for (i = 0 ; i < CMD_TOTAL_COMMANDS ; ++i) {
        if (!strncmp(pos, command_info_array[i].cmd_str, strlen(command_info_array[i].cmd_str))) {
            if (CMD_EXIT == i) {
                /* musl C does not like libuv workqueues calling exit() */
                execute_command(CMD_EXIT, NULL, NULL);
            }
            for (lstrip=pos + strlen(command_info_array[i].cmd_str); isspace((uint8_t)*lstrip) && ('\0' != *lstrip); ++lstrip) ;
            for (rstrip=lstrip+strlen(lstrip)-1; rstrip>lstrip && isspace((uint8_t)*rstrip); *(rstrip--) = 0 ) ;

            cmd_ctx->work.data = cmd_ctx;
            cmd_ctx->idx = i;
            cmd_ctx->args = lstrip;
            cmd_ctx->message = NULL;

            fatal_assert(0 == uv_queue_work(loop, &cmd_ctx->work, schedule_command, after_schedule_command));
            break;
        }
    }
    if (CMD_TOTAL_COMMANDS == i) {
        /* no command found */
        message = strdupz("Illegal command. Please type \"help\" for instructions.");
        send_command_reply(cmd_ctx, status, message);
        freez(message);
    }
}

static void pipe_read_cb(uv_stream_t *client, ssize_t nread, const uv_buf_t *buf)
{
    struct command_context *cmd_ctx = (struct command_context *)client;

    if (0 == nread) {
        netdata_log_info("%s: Zero bytes read by command pipe.", __func__);
    } else if (UV_EOF == nread) {
        parse_commands(cmd_ctx);
    } else if (nread < 0) {
        netdata_log_error("%s: %s", __func__, uv_strerror(nread));
    }

    if (nread < 0) { /* stop stream due to EOF or error */
        (void)uv_read_stop((uv_stream_t *)client);
    } else if (nread) {
        size_t to_copy;

        to_copy = MIN((size_t) nread, MAX_COMMAND_LENGTH - 1 - cmd_ctx->command_string_size);
        memcpy(cmd_ctx->command_string + cmd_ctx->command_string_size, buf->base, to_copy);
        cmd_ctx->command_string_size += to_copy;
        cmd_ctx->command_string[cmd_ctx->command_string_size] = '\0';
    }
    if (buf && buf->len) {
        freez(buf->base);
    }

    if (nread < 0 && UV_EOF != nread) {
        uv_close((uv_handle_t *)client, pipe_close_cb);
        --clients;
        // netdata_log_info("Command Clients = %u", clients);
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
    struct command_context *cmd_ctx;
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
        uv_close((uv_handle_t *)client, pipe_close_cb);
        return;
    }

    ++clients;
    // netdata_log_info("Command Clients = %u", clients);
    /* Start parsing a new command */
    cmd_ctx->command_string_size = 0;
    cmd_ctx->command_string[0] = '\0';

    ret = uv_read_start((uv_stream_t*)client, alloc_cb, pipe_read_cb);
    if (ret) {
        netdata_log_error("uv_read_start(): %s", uv_strerror(ret));
        uv_close((uv_handle_t *)client, pipe_close_cb);
        --clients;
        // netdata_log_info("Command Clients = %u", clients);
        return;
    }
}

static void async_cb(uv_async_t *handle)
{
    uv_stop(handle->loop);
}

static void command_thread(void *arg) {
    uv_thread_set_name_np("DAEMON_COMMAND");

    int ret;
    uv_fs_t req;

    (void) arg;
    loop = mallocz(sizeof(uv_loop_t));
    ret = uv_loop_init(loop);
    if (ret) {
        netdata_log_error("uv_loop_init(): %s", uv_strerror(ret));
        command_thread_error = ret;
        goto error_after_loop_init;
    }
    loop->data = NULL;

    ret = uv_async_init(loop, &async, async_cb);
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

    const char *pipename = daemon_pipename();

    (void)uv_fs_unlink(loop, &req, pipename, NULL);
    uv_fs_req_cleanup(&req);
    ret = uv_pipe_bind(&server_pipe, pipename);
    if (ret) {
        netdata_log_error("uv_pipe_bind(): %s", uv_strerror(ret));
        command_thread_error = ret;
        goto error_after_pipe_bind;
    }

    ret = uv_listen((uv_stream_t *)&server_pipe, SOMAXCONN, connection_cb);
    if (ret) {
        /* Fallback to backlog of 1 */
        netdata_log_info("uv_listen() failed with backlog = %d, falling back to backlog = 1.", SOMAXCONN);
        ret = uv_listen((uv_stream_t *)&server_pipe, 1, connection_cb);
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
    /* cleanup operations of the event loop */
    netdata_log_info("Shutting down command event loop.");
    uv_close((uv_handle_t *)&async, NULL);
    uv_close((uv_handle_t*)&server_pipe, NULL);
    uv_run(loop, UV_RUN_DEFAULT); /* flush all libuv handles */

    netdata_log_info("Shutting down command loop complete.");
    fatal_assert(0 == uv_loop_close(loop));
    freez(loop);

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
    if (command_server_initialized)
        return;

    netdata_log_info("Initializing command server.");
    for (i = 0 ; i < CMD_TOTAL_COMMANDS ; ++i) {
        fatal_assert(0 == uv_mutex_init(&command_lock_array[i]));
    }
    fatal_assert(0 == uv_rwlock_init(&exclusive_rwlock));

    completion_init(&completion);
    error = uv_thread_create(&thread, command_thread, NULL);
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

void commands_exit(void)
{
    cmd_t i;

    if (!command_server_initialized)
        return;

    command_thread_shutdown = 1;
    netdata_log_info("Shutting down command server.");
    /* wake up event loop */
    fatal_assert(0 == uv_async_send(&async));
    fatal_assert(0 == uv_thread_join(&thread));

    for (i = 0 ; i < CMD_TOTAL_COMMANDS ; ++i) {
        uv_mutex_destroy(&command_lock_array[i]);
    }
    uv_rwlock_destroy(&exclusive_rwlock);
    netdata_log_info("Command server has stopped.");
    command_server_initialized = 0;
}
