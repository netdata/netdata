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

// Accessed from the command loop thread, the libuv workers, the signal thread and the shutdown
// thread, so every read and write goes through __atomic_* - see commands_exit().
static cmd_init_status_t command_server_initialized = CMD_INIT_STATUS_OFF;
static int command_thread_error;
static int command_thread_shutdown;
static unsigned clients = 0;

struct command_context {
    /* embedded client pipe structure at address 0 */
    uv_pipe_t client;

    // Will a libuv callback close this handle?
    //
    // The shutdown teardown closes IDLE clients - ones sitting on an open connection with nothing
    // outstanding - so that the final uv_run() can drain. It must NOT close a handle that a
    // callback chain is going to close, because closing frees the whole context (the pipe is
    // embedded at offset 0 and pipe_close_cb() frees it) and the later callback would then
    // double-close it and touch freed memory.
    //
    // Set at BOTH points where a callback chain takes ownership, and NEVER cleared:
    //   - when a command is queued (uv_queue_work): schedule_command() and
    //     after_schedule_command() use the context, and the latter starts the reply write;
    //   - when any reply write is started (send_command_reply): pipe_write_cb() closes the
    //     handle once that write completes. This covers the invalid-command reply, which is sent
    //     directly from parse_commands() with no work item at all.
    // If the uv_write fails to start, send_command_reply() closes the handle itself there and
    // then, so the flag staying set costs nothing - uv_is_closing() covers that case.
    bool close_by_callback;

    uv_work_t work;
    uv_write_t write_req;
    cmd_t idx;
    char *args;
    char *message;
    cmd_status_t status;
    char command_string[MAX_COMMAND_LENGTH];
    unsigned command_string_size;
};

static inline char command_reply_prefix(const struct command_context *cmd_ctx, cmd_status_t status)
{
    if (cmd_ctx->idx == CMD_PING)
        return CMD_PREFIX_INFO;

    return cmd_prefix_by_status[status];
}

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
    {"help", "", "Show this help menu.", cmd_help_execute, CMD_TYPE_HIGH_PRIORITY, CMD_INIT_STATUS_INIT},                // show help menu
    {"reload-health", "", "Reload health configuration.", cmd_reload_health_execute, CMD_TYPE_ORTHOGONAL, CMD_INIT_STATUS_FULL}, // reload health configuration
    {"reopen-logs", "", "Close and reopen log files.", cmd_reopen_logs_execute, CMD_TYPE_ORTHOGONAL, CMD_INIT_STATUS_FULL},     // Close and reopen log files
    {"shutdown-agent", "", "Cleanup and exit the netdata agent.", cmd_exit_execute, CMD_TYPE_EXCLUSIVE, CMD_INIT_STATUS_FULL},          // exit cleanly
    {"fatal-agent", "", "Log the state and halt the netdata agent.", cmd_fatal_execute, CMD_TYPE_HIGH_PRIORITY, CMD_INIT_STATUS_FULL},        // exit with fatal error
    {"reload-claiming-state", "", "Reload agent claiming state from disk.", cmd_reload_claiming_state_execute, CMD_TYPE_ORTHOGONAL, CMD_INIT_STATUS_FULL}, // reload claiming state
    {"reload-labels", "", "Reload all localhost labels.", cmd_reload_labels_execute, CMD_TYPE_ORTHOGONAL, CMD_INIT_STATUS_FULL},                 // reload the labels
    {"read-config", "", "", cmd_read_config_execute, CMD_TYPE_CONCURRENT, CMD_INIT_STATUS_FULL},
    {"write-config", "", "", cmd_write_config_execute, CMD_TYPE_ORTHOGONAL, CMD_INIT_STATUS_FULL},
    {"ping", "", "Return with 'pong'; exit 0 when ready, 1 while initializing, 255 if the agent cannot be contacted.", cmd_ping_execute, CMD_TYPE_ORTHOGONAL, CMD_INIT_STATUS_INIT},     // ping command
    {"aclk-state", "[json]",  "Returns current state of ACLK and Netdata Cloud connection. (optionally in json).", cmd_aclk_state, CMD_TYPE_ORTHOGONAL, CMD_INIT_STATUS_FULL},
    {"version", "", "Returns the netdata version.", cmd_version, CMD_TYPE_ORTHOGONAL, CMD_INIT_STATUS_INIT},
    {"dumpconfig", "", "Returns the current netdata.conf on stdout.", cmd_dumpconfig, CMD_TYPE_ORTHOGONAL, CMD_INIT_STATUS_FULL},
    {"mark-stale-nodes-ephemeral", "<node_id | machine_guid | hostname | ALL_NODES>",
        "Marks one or all disconnected nodes as ephemeral, while keeping their retention\n      available for queries on both this Netdata Agent dashboard and Netdata Cloud", cmd_mark_stale_nodes_ephemeral, CMD_TYPE_ORTHOGONAL, CMD_INIT_STATUS_FULL},
    {"remove-stale-node", "<node_id | machine_guid | hostname | ALL_NODES>",
     "Marks one or all disconnected nodes as ephemeral, and removes them\n      so that they are no longer available for queries, from both this\n      Netdata Agent dashboard and Netdata Cloud.", cmd_remove_stale_node, CMD_TYPE_ORTHOGONAL, CMD_INIT_STATUS_FULL},
    {"update-node-info", "", "Schedules an node update message for localhost to Netdata Cloud.", cmd_update_node_info, CMD_TYPE_ORTHOGONAL, CMD_INIT_STATUS_FULL},
};

/* Mutexes for commands of type CMD_TYPE_ORTHOGONAL */
static netdata_mutex_t command_lock_array[CMD_TOTAL_COMMANDS];
/* Commands of type CMD_TYPE_EXCLUSIVE are writers */
static netdata_rwlock_t exclusive_rwlock;
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

    return netdata_ready_load() ? CMD_STATUS_SUCCESS : CMD_STATUS_FAILURE;
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

static int remove_ephemeral_host(BUFFER *wb, const char *machine_guid, bool report_error, bool unregister)
{
    rrd_rdlock();
    RRDHOST *host = rrdhost_find_by_guid(machine_guid);
    if (!host) {
        rrd_rdunlock();
        return 0;
    }

    if (host == localhost) {
        if (report_error)
            buffer_sprintf(wb, "Node '%s' (machine guid: %s) is our localhost - not changing it",
                           rrdhost_hostname(host), host->machine_guid);
        rrd_rdunlock();
        return 0;
    }

    // the context-load workers (sqlite_metadata.c) hold raw RRDHOST pointers to
    // every host carrying this flag; freeing such a host is a use-after-free
    // (marking ephemeral without unregistering does not free it, so it may proceed)
    if (unregister && rrdhost_flag_check(host, RRDHOST_FLAG_PENDING_CONTEXT_LOAD)) {
        if (report_error)
            buffer_sprintf(wb, "Node '%s' (machine guid: %s) is busy loading contexts - try again",
                           rrdhost_hostname(host), host->machine_guid);
        rrd_rdunlock();
        return -1;
    }

    bool locked = unregister ? rw_spinlock_trywrite_lock(&host->metadata_lifetime_lock)
                             : rw_spinlock_tryread_lock(&host->metadata_lifetime_lock);
    if (!locked) {
        if (report_error)
            buffer_sprintf(wb, "Node '%s' (machine guid: %s) is busy - try again",
                           rrdhost_hostname(host), host->machine_guid);
        rrd_rdunlock();
        return -1;
    }
    rrd_rdunlock();

    if (rrdhost_is_online(host)) {
        if (report_error)
            buffer_sprintf(wb, "Node '%s' (machine guid: %s) is online - not changing it",
                           rrdhost_hostname(host), host->machine_guid);
        if (unregister)
            rw_spinlock_write_unlock(&host->metadata_lifetime_lock);
        else
            rw_spinlock_read_unlock(&host->metadata_lifetime_lock);
        return 0;
    }

    bool marked = false;
    if (!rrdhost_option_check(host, RRDHOST_OPTION_EPHEMERAL_HOST)) {
        rrdhost_option_set(host, RRDHOST_OPTION_EPHEMERAL_HOST);
        marked = true;
    }

    // host->rrdlabels is the copy that matters: build_node_info() transmits it as the
    // node's complete label set, and meta_store_host_labels() rewrites the host_label
    // rows from it. Updating only the row below leaves the cloud believing the node is
    // permanent, and lets a later metadata store revert that row. Update the dictionary
    // in place - RRDLABELS is internally locked, while replacing the pointer would race
    // readers that assume it stays valid for the lifetime of the host.
    bool label_changed = host->rrdlabels &&
        rrdlabels_add_changed(host->rrdlabels, HOST_LABEL_IS_EPHEMERAL, "true", RRDLABEL_SRC_CONFIG);
    marked |= label_changed;

    sql_set_host_label(&host->host_id.uuid, HOST_LABEL_IS_EPHEMERAL, "true");
    pulse_host_status(host, 0, 0);

    // only a change in the transmitted labels needs an update; a host that has none in
    // memory gets no update at all, because that would publish an empty set and drop
    // every other label this node has
    if (label_changed)
        send_node_info_with_wait(host);
    else if (!host->rrdlabels)
        nd_log_daemon(NDLP_WARNING,
                      "Node '%s' (machine guid: %s) has no labels in memory - "
                      "could not inform Netdata Cloud that it is ephemeral",
                      rrdhost_hostname(host), host->machine_guid);

    if (unregister) {
        send_node_update_with_wait(host, 0, 0);

        unregister_node(host->machine_guid);
        host->node_id = UUID_ZERO;
        buffer_sprintf(wb, "Node '%s' (machine guid: %s) has been unregistered",
                       rrdhost_hostname(host), host->machine_guid);
        rrdhost_free___consume_metadata_lifetime_writelock(host);
        return 1;
    }

    if (marked) {
        buffer_sprintf(wb, "Node '%s' (machine guid: %s) has been marked ephemeral",
                       rrdhost_hostname(host), host->machine_guid);
        rw_spinlock_read_unlock(&host->metadata_lifetime_lock);
        return 1;
    }

    if (report_error) {
        buffer_sprintf(wb, "Node '%s' (machine guid: %s) is already ephemeral - not changing it",
                       rrdhost_hostname(host), host->machine_guid);
    }

    rw_spinlock_read_unlock(&host->metadata_lifetime_lock);
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

    char machine_guid[UUID_STR_LEN] = "";
    rrd_rdlock();
    RRDHOST *host = rrdhost_find_by_guid(args);
    if (!host)
        host = rrdhost_find_by_node_id(args);
    if (host)
        strncpyz(machine_guid, host->machine_guid, sizeof(machine_guid));
    rrd_rdunlock();

    if (!machine_guid[0]) {
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
        int busy = 0;
        while (sqlite3_step_monitored(res) == SQLITE_ROW) {
            char guid[UUID_STR_LEN];
            if (!sqlite3_column_uuid_unparse_lower(res, 0, guid))
                continue;
            int rc = remove_ephemeral_host(wb, guid, report_error, unregister);
            if (rc > 0) {
                cnt += rc;
                buffer_fast_strcat(wb, "\n", 1);
            }
            else if (rc < 0) {
                busy++;
                if (report_error)
                    buffer_fast_strcat(wb, "\n", 1);
            }
        }
        if (!cnt && !busy && buffer_strlen(wb) == 0) {
            if (report_error)
                buffer_sprintf(wb, "No match for \"%s\"", args);
            else
                buffer_sprintf(wb, "No stale nodes found");
        }
        else if (busy && !report_error)
            buffer_sprintf(wb, "%d node%s %s busy - try again", busy, busy == 1 ? "" : "s", busy == 1 ? "is" : "are");
    done0:
        REPORT_BIND_FAIL(res, param);
        SQLITE_FINALIZE(res);
    }
    else
        (void) remove_ephemeral_host(wb, machine_guid, true, unregister);

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

    netdata_rwlock_wrlock(&exclusive_rwlock);
}

static void cmd_lock_orthogonal(unsigned index)
{
    netdata_rwlock_rdlock(&exclusive_rwlock);
    netdata_mutex_lock(&command_lock_array[index]);
}

static void cmd_lock_idempotent(unsigned index)
{
    (void)index;

    netdata_rwlock_rdlock(&exclusive_rwlock);
}

static void cmd_lock_high_priority(unsigned index)
{
    (void)index;
}

static void cmd_unlock_exclusive(unsigned index)
{
    (void)index;

    netdata_rwlock_wrunlock(&exclusive_rwlock);
}

static void cmd_unlock_orthogonal(unsigned index)
{
    netdata_rwlock_rdunlock(&exclusive_rwlock);
    netdata_mutex_unlock(&command_lock_array[index]);
}

static void cmd_unlock_idempotent(unsigned index)
{
    (void)index;

    netdata_rwlock_rdunlock(&exclusive_rwlock);
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

    // From here on pipe_write_cb() owns closing this handle - see close_by_callback. This must be
    // set for EVERY reply, not just those that came from a queued command: the invalid-command
    // reply is sent straight from parse_commands() with no work item.
    cmd_ctx->close_by_callback = true;

    char exit_status_string[MAX_EXIT_STATUS_LENGTH + 1] = {'\0', };
    unsigned reply_string_size = 0;
    uv_buf_t write_buf;
    uv_stream_t *client = (uv_stream_t *)(uv_pipe_t *)cmd_ctx;

    snprintfz(exit_status_string, MAX_EXIT_STATUS_LENGTH, "%u", status);
    add_char_to_command_reply(reply_string, &reply_string_size, CMD_PREFIX_EXIT_CODE);
    add_string_to_command_reply(reply_string, &reply_string_size, exit_status_string);
    add_char_to_command_reply(reply_string, &reply_string_size, '\0');

    if (message) {
        add_char_to_command_reply(reply_string, &reply_string_size, command_reply_prefix(cmd_ctx, status));
        add_string_to_command_reply(reply_string, &reply_string_size, message);
    }

    cmd_ctx->write_req.data = client;
    client->data = reply_string;
    write_buf.base = reply_string->buffer;
    write_buf.len = reply_string_size;
    ret = uv_write(&cmd_ctx->write_req, (uv_stream_t *)client, &write_buf, 1, pipe_write_cb);
    if (ret) {
        netdata_log_error("uv_write(): %s", uv_strerror(ret));
        buffer_free(reply_string);
        uv_close((uv_handle_t *)client, pipe_close_cb);
        --clients;
    }
}

cmd_status_t execute_command(cmd_t idx, char *args, char **message)
{
    cmd_status_t status;
    cmd_type_t type = command_info_array[idx].type;

    cmd_lock_by_type[type](idx);
    if (__atomic_load_n(&command_server_initialized, __ATOMIC_ACQUIRE) >= command_info_array[idx].init_status)
        status = command_info_array[idx].func(args, message);
    else {
        if (message)
            *message = strdupz("Agent is initializing");
        status = CMD_STATUS_SUCCESS;
    }
    cmd_unlock_by_type[type](idx);

    return status;
}

// Command handlers dispatched to the libuv threadpool, and whether THIS thread is inside one.
//
// SCOPE, so the count is not over-trusted: this covers the QUEUED commands only. The signal
// path calls execute_command(CMD_RELOAD_HEALTH / CMD_REOPEN_LOGS) directly on its own thread
// (signal-handler.c), and CMD_EXIT is executed inline by parse_commands(); neither is counted.
// CMD_RELOAD_HEALTH does reach SQLite (health_plugin_reload() -> ... -> sql_alert_store_config()),
// so "no commands in flight" means no netdatacli WORK ITEM is in flight - it is not a statement
// about every SQLite user.
//
// commands_exit() needs both. A handler can terminate the process from where it runs -
// cmd_fatal_execute() calls fatal(), which runs the whole shutdown sequence on the threadpool
// worker - and shutdown then reaches commands_exit(). Joining the command loop from there
// deadlocks: the loop's own teardown runs uv_run(loop, UV_RUN_DEFAULT) to flush, and the work
// request it is waiting to flush is the one this thread is still inside.
static uint32_t commands_in_flight = 0;
static __thread bool this_thread_runs_a_command = false;

// commands_exit() has TWO callers now - the signal path and the shutdown sequence - and they
// can run on different threads at the same time (a systemd PrepareForShutdown, an API quit, or
// a fatal() on any thread can be inside netdata_cleanup_and_exit() when SIGTERM arrives).
// command_server_initialized only becomes CMD_INIT_STATUS_OFF after the join completes, so it
// cannot by itself keep two callers apart: both would pass it and both would reach
// uv_thread_join() on the same thread, which is undefined behaviour, and then destroy the same
// mutexes twice.
static bool commands_exit_in_progress = false;

// Has uv_thread_create() actually produced `thread`?
//
// This is NOT the same question as the init status. commands_init() sets CMD_INIT_STATUS_INIT
// BEFORE it creates the thread, so the status cannot tell us whether `thread` is safe to touch,
// and CMD_INIT_STATUS_FULL is too strict in the other direction: at INIT the loop is already
// running and serving commands, so a shutdown there still has to drain it.
static bool command_thread_created = false;

static void after_schedule_command(uv_work_t *req, int status)
{
    struct command_context *cmd_ctx = req->data;

    (void)status;

    send_command_reply(cmd_ctx, cmd_ctx->status, cmd_ctx->message);
    if (cmd_ctx->message)
        freez(cmd_ctx->message);

    __atomic_sub_fetch(&commands_in_flight, 1, __ATOMIC_ACQ_REL);
}

static void schedule_command(uv_work_t *req)
{
    register_libuv_worker_jobs();
    worker_is_busy(UV_EVENT_SCHEDULE_CMD);

    struct command_context *cmd_ctx = req->data;
    this_thread_runs_a_command = true;
    cmd_ctx->status = execute_command(cmd_ctx->idx, cmd_ctx->args, &cmd_ctx->message);
    this_thread_runs_a_command = false;

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

            cmd_ctx->close_by_callback = true;
            __atomic_add_fetch(&commands_in_flight, 1, __ATOMIC_ACQ_REL);
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
    cmd_ctx->idx = CMD_HELP;
    cmd_ctx->close_by_callback = false;
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

// Close the client connections that are merely sitting there.
//
// The teardown below closes the listener and the async handle, then flushes with
// uv_run(loop, UV_RUN_DEFAULT). That flush does not return while ANY handle is still active -
// including an accepted client that connected and then sent nothing, which keeps its read
// handle open indefinitely. Since commands_exit() joins this thread, such a client would stall
// the whole shutdown until the 135 s watchdog SIGABRTs it - and it would also trip the
// fatal_assert(0 == uv_loop_close(loop)) below.
//
// Only IDLE clients are closed here - those that never had a command dispatched. A client the
// loop is working for is left alone: its context is freed by pipe_close_cb(), so closing it now
// would pull the memory out from under schedule_command(), after_schedule_command(), or the
// uv_write they start. Each closes itself in pipe_write_cb() once its reply is written, and this
// same flush collects it.
static void close_idle_clients_cb(uv_handle_t *handle, void *arg __maybe_unused)
{
    if (handle->type != UV_NAMED_PIPE || handle == (uv_handle_t *)&server_pipe)
        return;

    if (uv_is_closing(handle))
        return;

    struct command_context *cmd_ctx = (struct command_context *)handle;
    if (cmd_ctx->close_by_callback)
        return;

    uv_close(handle, pipe_close_cb);
    --clients;
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

    /* stop accepting new connections first, then drop the idle ones, then flush */
    uv_close((uv_handle_t*)&server_pipe, NULL);
    uv_walk(loop, close_idle_clients_cb, NULL);

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

    // Never (re)start the command server once shutdown has begun. Without this, a SIGTERM during
    // startup could have commands_exit() stop the loop while main() is still initializing, and
    // the next commands_init() would bring a fresh loop up - dispatching commands that touch
    // db_meta while the shutdown sequence is closing it.
    if (exit_initiated_get()) {
        netdata_log_info("Not initializing the command server: shutdown has already begun.");
        return;
    }

    if (__atomic_load_n(&command_server_initialized, __ATOMIC_ACQUIRE) == CMD_INIT_STATUS_FULL)
        return;

    if (__atomic_load_n(&command_server_initialized, __ATOMIC_ACQUIRE) == CMD_INIT_STATUS_OFF) {
        netdata_log_info("Initializing command server for liveness CHECK");
        // Re-arm, so a second init/exit cycle in one process does not latch instead of
        // joining. sqlite_library_init() does the same for its own flags.
        __atomic_store_n(&commands_exit_in_progress, false, __ATOMIC_RELEASE);
        __atomic_store_n(&command_server_initialized, CMD_INIT_STATUS_INIT, __ATOMIC_RELEASE);
    }
    else {
        netdata_log_info("Initializing full command server.");
        __atomic_store_n(&command_server_initialized, CMD_INIT_STATUS_FULL, __ATOMIC_RELEASE);
        return;
    }

    // Initialize the command locks ONCE per process.
    //
    // commands_exit() deliberately does not destroy them (another thread may hold one; see the
    // note there), so re-running netdata_mutex_init() on an already-initialized mutex would be
    // undefined behaviour on a second init/exit cycle. Guarding here keeps both halves safe:
    // nothing is destroyed while it may be held, and nothing is re-initialized while it is live.
    //
    // Only ever reached from commands_init(), which main() calls during single-threaded startup,
    // so a plain flag is sufficient.
    static bool command_locks_initialized = false;
    if (!command_locks_initialized) {
        for (i = 0 ; i < CMD_TOTAL_COMMANDS ; ++i) {
            fatal_assert(0 == netdata_mutex_init(&command_lock_array[i]));
        }
        fatal_assert(0 == netdata_rwlock_init(&exclusive_rwlock));
        command_locks_initialized = true;
    }

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

    // Publish ONLY here - after the initialization handshake, never right after
    // uv_thread_create(). Between those two points the worker has not yet initialized `async`
    // and `server_pipe`, and it ends its setup with `command_thread_shutdown = 0`. A
    // commands_exit() that passed the gate in that window would (a) uv_async_send() an
    // uninitialized handle and (b) have its `command_thread_shutdown = 1` overwritten by the
    // worker, so the loop would never exit and uv_thread_join() would block until the shutdown
    // watchdog aborted. Publishing after the handshake makes the gate mean "the loop is up and
    // can be told to stop", which is what commands_exit() actually needs.
    __atomic_store_n(&command_thread_created, true, __ATOMIC_RELEASE);

    // Shutdown may have begun while we were completing that handshake (on Windows the service
    // reports itself running before netdata_main(), so a stop can arrive during startup). The
    // commands_exit() that ran then saw no thread and returned without stopping anything, so
    // stop it now rather than leaving a live loop dispatching commands into a teardown.
    if (exit_initiated_get()) {
        netdata_log_info("Command server came up during shutdown; stopping it again.");
        commands_exit();
    }

    return;

after_error:
    netdata_log_error("Failed to initialize command server. The netdata cli tool will be unable to send commands.");
    __atomic_store_n(&command_server_initialized, CMD_INIT_STATUS_OFF, __ATOMIC_RELEASE);
}

void commands_exit(void)
{
    // Already stopped, by whichever caller got here first. Nothing to drain and nothing to
    // suppress: that caller completed the join before setting this.
    if (__atomic_load_n(&command_server_initialized, __ATOMIC_ACQUIRE) == CMD_INIT_STATUS_OFF)
        return;

    // Two shutdown paths originate INSIDE this subsystem, and neither can join the loop:
    //
    //  - `netdatacli shutdown-agent`: parse_commands() executes CMD_EXIT inline rather than
    //    through the workqueue (musl does not tolerate a libuv worker calling exit()), and
    //    cmd_exit_execute() -> netdata_exit_gracefully() -> netdata_cleanup_and_exit() runs
    //    the whole shutdown ON THE LOOP THREAD. Joining ourselves trips the fatal_assert below.
    //  - `netdatacli fatal-agent` (and any handler that calls fatal()): those DO go through the
    //    workqueue, so shutdown runs on a threadpool WORKER. That worker is not the loop
    //    thread, so a naive thread comparison lets it through to the join - and then it
    //    deadlocks, because the loop's teardown flushes with uv_run(loop, UV_RUN_DEFAULT) and
    //    the request it waits on is the one this worker is still inside. Shutdown then hangs
    //    until the watchdog SIGABRTs it.
    //
    // So: skip the join whenever we are the loop thread OR we are inside a command handler.
    //
    // Whether to suppress the SQLite teardown as well is a SEPARATE question, and must not be
    // answered by the shape of the path. `netdatacli shutdown-agent` is the stop command used
    // by the installer and the documented manual stop, and CMD_EXIT itself touches no SQLite;
    // latching there unconditionally would skip the database close, PRAGMA optimize and WAL
    // checkpoint on ordinary restarts. Latch only when a command handler is actually in flight
    // and may be holding SQLite state - which for the fatal-agent path includes ourselves.
    // Do not touch `thread` until it exists - but do not demand CMD_INIT_STATUS_FULL either.
    // At CMD_INIT_STATUS_INIT the loop is already running (only help/ping/version are
    // dispatchable, none of which touch SQLite, but the loop must still be stopped), so gating
    // on FULL would leave it running straight through the SQLite teardown.
    if (!__atomic_load_n(&command_thread_created, __ATOMIC_ACQUIRE)) {
        netdata_log_info("Command server thread was never created; nothing to stop.");
        return;
    }

    uv_thread_t self = uv_thread_self();
    if (uv_thread_equal(&thread, &self) || this_thread_runs_a_command) {
        uint32_t in_flight = __atomic_load_n(&commands_in_flight, __ATOMIC_ACQUIRE);

        if (in_flight) {
            sqlite_mark_teardown_unsafe(
                "shutdown started inside the netdatacli command subsystem while a command was still "
                "running, so the command loop cannot be drained");
            netdata_log_info(
                "Command server cannot be joined from this thread; %u command(s) still in flight.", in_flight);
        }
        else
            netdata_log_info(
                "Command server cannot be joined from this thread; no netdatacli command is in flight.");

        return;
    }

    // Someone else is inside this function right now (see commands_exit_in_progress). We must
    // not join a thread they are already joining, and we cannot wait for them without risking a
    // deadlock of our own - so we fall back to the other half of the contract and suppress the
    // teardown, because we cannot establish that the command loop was drained.
    if (__atomic_exchange_n(&commands_exit_in_progress, true, __ATOMIC_ACQ_REL)) {
        sqlite_mark_teardown_unsafe(
            "another thread is already stopping the netdatacli command loop, so this one cannot "
            "confirm it was drained");
        return;
    }

    command_thread_shutdown = 1;
    netdata_log_info("Shutting down command server.");
    /* wake up event loop */
    fatal_assert(0 == uv_async_send(&async));
    fatal_assert(0 == uv_thread_join(&thread));

    // Deliberately NOT destroying command_lock_array[] / exclusive_rwlock.
    //
    // This function used to have a single caller on the signal thread - the same thread that
    // runs the signal path's direct execute_command() calls - so a destroy could never race a
    // held lock. It now also runs from the shutdown sequence, which can be on a different
    // thread (systemd PrepareForShutdown, an API quit, or any fatal()), so destroying here
    // could tear down a mutex that the signal thread is holding inside execute_command().
    // POSIX says that is undefined; glibc happens to return EBUSY.
    //
    // There is nothing to gain by destroying them: the process exits immediately after, and
    // the OS reclaims everything. Leaking them is the safe half of the trade.
    netdata_log_info("Command server has stopped.");
    __atomic_store_n(&command_thread_created, false, __ATOMIC_RELEASE);
    __atomic_store_n(&command_server_initialized, CMD_INIT_STATUS_OFF, __ATOMIC_RELEASE);
}
