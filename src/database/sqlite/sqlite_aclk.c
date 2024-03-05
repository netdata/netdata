// SPDX-License-Identifier: GPL-3.0-or-later

#include "sqlite_functions.h"
#include "sqlite_aclk.h"

#include "sqlite_aclk_node.h"

struct aclk_sync_config_s {
    uv_thread_t thread;
    uv_loop_t loop;
    uv_timer_t timer_req;
    time_t cleanup_after;          // Start a cleanup after this timestamp
    uv_async_t async;
    bool initialized;
    SPINLOCK cmd_queue_lock;
    struct aclk_database_cmd *cmd_base;
} aclk_sync_config = { 0 };

void sanity_check(void) {
    // make sure the compiler will stop on misconfigurations
    BUILD_BUG_ON(WORKER_UTILIZATION_MAX_JOB_TYPES < ACLK_MAX_ENUMERATIONS_DEFINED);
}

static struct aclk_database_cmd aclk_database_deq_cmd(void)
{
    struct aclk_database_cmd ret;

    spinlock_lock(&aclk_sync_config.cmd_queue_lock);
    if(aclk_sync_config.cmd_base) {
        struct aclk_database_cmd *t = aclk_sync_config.cmd_base;
        DOUBLE_LINKED_LIST_REMOVE_ITEM_UNSAFE(aclk_sync_config.cmd_base, t, prev, next);
        ret = *t;
        freez(t);
    }
    else {
        ret.opcode = ACLK_DATABASE_NOOP;
        ret.completion = NULL;
    }
    spinlock_unlock(&aclk_sync_config.cmd_queue_lock);

    return ret;
}

static void aclk_database_enq_cmd(struct aclk_database_cmd *cmd)
{
    struct aclk_database_cmd *t = mallocz(sizeof(*t));
    *t = *cmd;
    t->prev = t->next = NULL;

    spinlock_lock(&aclk_sync_config.cmd_queue_lock);
    DOUBLE_LINKED_LIST_APPEND_ITEM_UNSAFE(aclk_sync_config.cmd_base, t, prev, next);
    spinlock_unlock(&aclk_sync_config.cmd_queue_lock);

    (void) uv_async_send(&aclk_sync_config.async);
}

enum {
    IDX_HOST_ID,
    IDX_HOSTNAME,
    IDX_REGISTRY,
    IDX_UPDATE_EVERY,
    IDX_OS,
    IDX_TIMEZONE,
    IDX_HOPS,
    IDX_MEMORY_MODE,
    IDX_ABBREV_TIMEZONE,
    IDX_UTC_OFFSET,
    IDX_PROGRAM_NAME,
    IDX_PROGRAM_VERSION,
    IDX_ENTRIES,
    IDX_HEALTH_ENABLED,
    IDX_LAST_CONNECTED,
    IDX_IS_EPHEMERAL,
    IDX_IS_REGISTERED,
};

static int create_host_callback(void *data, int argc, char **argv, char **column)
{
    int *number_of_chidren = data;
    UNUSED(argc);
    UNUSED(column);

    time_t last_connected =
        (time_t)(argv[IDX_LAST_CONNECTED] ? str2uint64_t(argv[IDX_LAST_CONNECTED], NULL) : 0);

    if (!last_connected)
        last_connected = now_realtime_sec();

    time_t age = now_realtime_sec() - last_connected;
    int is_ephemeral = 0;
    int is_registered = 0;

    if (argv[IDX_IS_EPHEMERAL])
        is_ephemeral = str2i(argv[IDX_IS_EPHEMERAL]);

    if (argv[IDX_IS_REGISTERED])
        is_registered = str2i(argv[IDX_IS_REGISTERED]);

    char guid[UUID_STR_LEN];
    uuid_unparse_lower(*(uuid_t *)argv[IDX_HOST_ID], guid);

    if (is_ephemeral && age > rrdhost_free_ephemeral_time_s) {
        netdata_log_info(
            "%s ephemeral hostname \"%s\" with GUID \"%s\", age = %ld seconds (limit %ld seconds)",
            is_registered ? "Loading registered" : "Skipping unregistered",
            (const char *)argv[IDX_HOSTNAME],
            guid,
            age,
            rrdhost_free_ephemeral_time_s);
        if (!is_registered)
            return 0;
    }

    struct rrdhost_system_info *system_info = callocz(1, sizeof(struct rrdhost_system_info));
    __atomic_sub_fetch(&netdata_buffers_statistics.rrdhost_allocations_size, sizeof(struct rrdhost_system_info), __ATOMIC_RELAXED);

    system_info->hops = str2i((const char *) argv[IDX_HOPS]);

    sql_build_host_system_info((uuid_t *)argv[IDX_HOST_ID], system_info);

    RRDHOST *host = rrdhost_find_or_create(
        (const char *)argv[IDX_HOSTNAME],
        (const char *)argv[IDX_REGISTRY],
        guid,
        (const char *)argv[IDX_OS],
        (const char *)argv[IDX_TIMEZONE],
        (const char *)argv[IDX_ABBREV_TIMEZONE],
        (int32_t)(argv[IDX_UTC_OFFSET] ? str2uint32_t(argv[IDX_UTC_OFFSET], NULL) : 0),
        (const char *)(argv[IDX_PROGRAM_NAME] ? argv[IDX_PROGRAM_NAME] : "unknown"),
        (const char *)(argv[IDX_PROGRAM_VERSION] ? argv[IDX_PROGRAM_VERSION] : "unknown"),
        argv[IDX_UPDATE_EVERY] ? str2i(argv[IDX_UPDATE_EVERY]) : 1,
        argv[IDX_ENTRIES] ? str2i(argv[IDX_ENTRIES]) : 0,
        default_rrd_memory_mode,
        0 // health
        ,
        0 // rrdpush enabled
        ,
        NULL //destination
        ,
        NULL // api key
        ,
        NULL // send charts matching
        ,
        false // rrdpush_enable_replication
        ,
        0 // rrdpush_seconds_to_replicate
        ,
        0 // rrdpush_replication_step
        ,
        system_info,
        1);

    if (likely(host)) {
        if (is_ephemeral)
            rrdhost_option_set(host, RRDHOST_OPTION_EPHEMERAL_HOST);

        if (is_ephemeral)
            host->child_disconnected_time = now_realtime_sec();

        host->rrdlabels = sql_load_host_labels((uuid_t *)argv[IDX_HOST_ID]);
        host->last_connected = last_connected;
    }

    (*number_of_chidren)++;

#ifdef NETDATA_INTERNAL_CHECKS
    char node_str[UUID_STR_LEN] = "<none>";
    if (likely(host->node_id))
        uuid_unparse_lower(*host->node_id, node_str);
    internal_error(true, "Adding archived host \"%s\" with GUID \"%s\" node id = \"%s\"  ephemeral=%d", rrdhost_hostname(host), host->machine_guid, node_str, is_ephemeral);
#endif
    return 0;
}

#ifdef ENABLE_ACLK

#define SQL_SELECT_HOST_BY_UUID  "SELECT host_id FROM host WHERE host_id = @host_id"
static int is_host_available(uuid_t *host_id)
{
    sqlite3_stmt *res = NULL;
    int rc;

    if (unlikely(!db_meta)) {
        if (default_rrd_memory_mode == RRD_MEMORY_MODE_DBENGINE)
            error_report("Database has not been initialized");
        return 1;
    }

    rc = sqlite3_prepare_v2(db_meta, SQL_SELECT_HOST_BY_UUID, -1, &res, 0);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to prepare statement to select node instance information for a node");
        return 1;
    }

    rc = sqlite3_bind_blob(res, 1, host_id, sizeof(*host_id), SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to bind host_id parameter to check host existence");
        goto failed;
    }
    rc = sqlite3_step_monitored(res);

failed:
    if (unlikely(sqlite3_finalize(res) != SQLITE_OK))
        error_report("Failed to finalize the prepared statement when checking host existence");

    return (rc == SQLITE_ROW);
}

// OPCODE: ACLK_DATABASE_DELETE_HOST
static void sql_delete_aclk_table_list(char *host_guid)
{
    char uuid_str[UUID_STR_LEN];
    char host_str[UUID_STR_LEN];

    int rc;
    uuid_t host_uuid;

    if (unlikely(!host_guid))
        return;

    rc = uuid_parse(host_guid, host_uuid);
    freez(host_guid);
    if (rc)
        return;

    uuid_unparse_lower(host_uuid, host_str);
    uuid_unparse_lower_fix(&host_uuid, uuid_str);

    if (is_host_available(&host_uuid))
        return;

    sqlite3_stmt *res = NULL;
    BUFFER *sql = buffer_create(ACLK_SYNC_QUERY_SIZE, &netdata_buffers_statistics.buffers_sqlite);

    buffer_sprintf(sql,"SELECT 'drop '||type||' IF EXISTS '||name||';' FROM sqlite_schema " \
                        "WHERE name LIKE 'aclk_%%_%s' AND type IN ('table', 'trigger', 'index')", uuid_str);

    rc = sqlite3_prepare_v2(db_meta, buffer_tostring(sql), -1, &res, 0);
    if (rc != SQLITE_OK) {
        error_report("Failed to prepare statement to clean up aclk tables");
        goto fail;
    }
    buffer_flush(sql);

    while (sqlite3_step_monitored(res) == SQLITE_ROW)
        buffer_strcat(sql, (char *) sqlite3_column_text(res, 0));

    rc = sqlite3_finalize(res);
    if (unlikely(rc != SQLITE_OK))
        error_report("Failed to finalize statement to clean up aclk tables, rc = %d", rc);

    rc = db_execute(db_meta, buffer_tostring(sql));
    if (unlikely(rc))
        netdata_log_error("Failed to drop unused ACLK tables");

fail:
    buffer_free(sql);
}

// OPCODE: ACLK_DATABASE_NODE_UNREGISTER
static void sql_unregister_node(char *machine_guid)
{
    int rc;
    uuid_t host_uuid;

    if (unlikely(!machine_guid))
        return;

    rc = uuid_parse(machine_guid, host_uuid);
    if (rc) {
        freez(machine_guid);
        return;
    }

    sqlite3_stmt *res = NULL;

    rc = sqlite3_prepare_v2(db_meta, "UPDATE node_instance SET node_id = NULL WHERE host_id = @host_id", -1, &res, 0);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to prepare statement to remove the host node id");
        freez(machine_guid);
        return;
    }

    rc = sqlite3_bind_blob(res, 1, &host_uuid, sizeof(host_uuid), SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to bind host_id parameter to remove host node id");
        goto skip;
    }
    rc = sqlite3_step_monitored(res);
    if (unlikely(rc != SQLITE_DONE)) {
        error_report("Failed to execute command to remove host node id");
    } else {
       // node: machine guid will be freed after processing
       metadata_delete_host_chart_labels(machine_guid);
       machine_guid = NULL;
    }

skip:
    if (unlikely(sqlite3_finalize(res) != SQLITE_OK))
        error_report("Failed to finalize statement to remove host node id");
    freez(machine_guid);
}


static int sql_check_aclk_table(void *data __maybe_unused, int argc __maybe_unused, char **argv __maybe_unused, char **column __maybe_unused)
{
    struct aclk_database_cmd cmd;
    memset(&cmd, 0, sizeof(cmd));
    cmd.opcode = ACLK_DATABASE_DELETE_HOST;
    cmd.param[0] = strdupz((char *) argv[0]);
    aclk_database_enq_cmd(&cmd);
    return 0;
}

#define SQL_SELECT_ACLK_ACTIVE_LIST "SELECT REPLACE(SUBSTR(name,19),'_','-') FROM sqlite_schema " \
        "WHERE name LIKE 'aclk_chart_latest_%' AND type IN ('table')"

static void sql_check_aclk_table_list(void)
{
    char *err_msg = NULL;
    int rc = sqlite3_exec_monitored(db_meta, SQL_SELECT_ACLK_ACTIVE_LIST, sql_check_aclk_table, NULL, &err_msg);
    if (rc != SQLITE_OK) {
        error_report("Query failed when trying to check for obsolete ACLK sync tables, %s", err_msg);
        sqlite3_free(err_msg);
    }
}

#define SQL_ALERT_CLEANUP "DELETE FROM aclk_alert_%s WHERE date_submitted IS NOT NULL AND CAST(date_cloud_ack AS INT) < unixepoch()-%d"

static int sql_maint_aclk_sync_database(void *data __maybe_unused, int argc __maybe_unused, char **argv, char **column __maybe_unused)
{
    char sql[ACLK_SYNC_QUERY_SIZE];
    snprintfz(sql,sizeof(sql) - 1, SQL_ALERT_CLEANUP, (char *) argv[0], ACLK_DELETE_ACK_ALERTS_INTERNAL);
    if (unlikely(db_execute(db_meta, sql)))
        error_report("Failed to clean stale ACLK alert entries");
    return 0;
}

#define SQL_SELECT_ACLK_ALERT_LIST "SELECT SUBSTR(name,12) FROM sqlite_schema WHERE name LIKE 'aclk_alert_%' AND type IN ('table')"

static void sql_maint_aclk_sync_database_all(void)
{
    char *err_msg = NULL;
    int rc = sqlite3_exec_monitored(db_meta, SQL_SELECT_ACLK_ALERT_LIST, sql_maint_aclk_sync_database, NULL, &err_msg);
    if (rc != SQLITE_OK) {
        error_report("Query failed when trying to check for obsolete ACLK sync tables, %s", err_msg);
        sqlite3_free(err_msg);
    }
}

static int aclk_config_parameters(void *data __maybe_unused, int argc __maybe_unused, char **argv, char **column __maybe_unused)
{
    char uuid_str[UUID_STR_LEN];
    uuid_unparse_lower(*((uuid_t *) argv[0]), uuid_str);

    RRDHOST *host = rrdhost_find_by_guid(uuid_str);
    if (host == localhost)
        return 0;

    sql_create_aclk_table(host, (uuid_t *) argv[0], (uuid_t *) argv[1]);
    return 0;
}

static void async_cb(uv_async_t *handle)
{
    uv_stop(handle->loop);
    uv_update_time(handle->loop);
}

#define TIMER_PERIOD_MS (1000)

static void timer_cb(uv_timer_t *handle)
{
    uv_stop(handle->loop);
    uv_update_time(handle->loop);

    struct aclk_sync_config_s *config = handle->data;
    struct aclk_database_cmd cmd;
    memset(&cmd, 0, sizeof(cmd));

    if (config->cleanup_after < now_realtime_sec()) {
        cmd.opcode = ACLK_DATABASE_CLEANUP;
        aclk_database_enq_cmd(&cmd);
        config->cleanup_after += ACLK_DATABASE_CLEANUP_INTERVAL;
    }

    if (aclk_connected) {
        cmd.opcode = ACLK_DATABASE_PUSH_ALERT;
        aclk_database_enq_cmd(&cmd);
        aclk_check_node_info_and_collectors();
    }
}

static void aclk_synchronization(void *arg __maybe_unused)
{
    struct aclk_sync_config_s *config = arg;
    uv_thread_set_name_np(config->thread,  "ACLKSYNC");
    worker_register("ACLKSYNC");
    service_register(SERVICE_THREAD_TYPE_EVENT_LOOP, NULL, NULL, NULL, true);

    worker_register_job_name(ACLK_DATABASE_NOOP,                 "noop");
    worker_register_job_name(ACLK_DATABASE_CLEANUP,              "cleanup");
    worker_register_job_name(ACLK_DATABASE_DELETE_HOST,          "node delete");
    worker_register_job_name(ACLK_DATABASE_NODE_STATE,           "node state");
    worker_register_job_name(ACLK_DATABASE_PUSH_ALERT,           "alert push");
    worker_register_job_name(ACLK_DATABASE_PUSH_ALERT_CONFIG,    "alert conf push");
    worker_register_job_name(ACLK_DATABASE_PUSH_ALERT_CHECKPOINT,"alert checkpoint");
    worker_register_job_name(ACLK_DATABASE_PUSH_ALERT_SNAPSHOT,  "alert snapshot");
    worker_register_job_name(ACLK_DATABASE_QUEUE_REMOVED_ALERTS, "alerts check");
    worker_register_job_name(ACLK_DATABASE_TIMER,                "timer");

    uv_loop_t *loop = &config->loop;
    fatal_assert(0 == uv_loop_init(loop));
    fatal_assert(0 == uv_async_init(loop, &config->async, async_cb));

    fatal_assert(0 == uv_timer_init(loop, &config->timer_req));
    config->timer_req.data = config;
    fatal_assert(0 == uv_timer_start(&config->timer_req, timer_cb, TIMER_PERIOD_MS, TIMER_PERIOD_MS));

    netdata_log_info("Starting ACLK synchronization thread");

    config->cleanup_after = now_realtime_sec() + ACLK_DATABASE_CLEANUP_FIRST;
    config->initialized = true;

    while (likely(service_running(SERVICE_ACLKSYNC))) {
        enum aclk_database_opcode opcode;
        worker_is_idle();
        uv_run(loop, UV_RUN_DEFAULT);

        /* wait for commands */
        do {
            struct aclk_database_cmd cmd = aclk_database_deq_cmd();

            if (unlikely(!service_running(SERVICE_ACLKSYNC)))
                break;

            opcode = cmd.opcode;

            if(likely(opcode != ACLK_DATABASE_NOOP))
                worker_is_busy(opcode);

            switch (opcode) {
                case ACLK_DATABASE_NOOP:
                    /* the command queue was empty, do nothing */
                    break;
// MAINTENANCE
                case ACLK_DATABASE_CLEANUP:
                    // Scan all aclk_alert_ tables and cleanup as needed
                    sql_maint_aclk_sync_database_all();
                    sql_check_aclk_table_list();
                    break;

                case ACLK_DATABASE_DELETE_HOST:
                    sql_delete_aclk_table_list(cmd.param[0]);
                    break;
// NODE STATE
                case ACLK_DATABASE_NODE_STATE:;
                    RRDHOST *host = cmd.param[0];
                    int live = (host == localhost || host->receiver || !(rrdhost_flag_check(host, RRDHOST_FLAG_ORPHAN))) ? 1 : 0;
                    struct aclk_sync_cfg_t *ahc = host->aclk_config;
                    if (unlikely(!ahc))
                        sql_create_aclk_table(host, &host->host_uuid, host->node_id);
                    aclk_host_state_update(host, live, 1);
                    break;
                case ACLK_DATABASE_NODE_UNREGISTER:
                    sql_unregister_node(cmd.param[0]);

                    break;
                    // ALERTS
                case ACLK_DATABASE_PUSH_ALERT_CONFIG:
                    aclk_push_alert_config_event(cmd.param[0], cmd.param[1]);
                    break;
                case ACLK_DATABASE_PUSH_ALERT:
                    aclk_push_alert_events_for_all_hosts();
                    break;
                case ACLK_DATABASE_PUSH_ALERT_SNAPSHOT:;
                    aclk_push_alert_snapshot_event(cmd.param[0]);
                    break;
                case ACLK_DATABASE_QUEUE_REMOVED_ALERTS:
                    sql_process_queue_removed_alerts_to_aclk(cmd.param[0]);
                    break;
                default:
                    break;
            }
            if (cmd.completion)
                completion_mark_complete(cmd.completion);
        } while (opcode != ACLK_DATABASE_NOOP);
    }

    if (!uv_timer_stop(&config->timer_req))
        uv_close((uv_handle_t *)&config->timer_req, NULL);

    uv_close((uv_handle_t *)&config->async, NULL);
    (void) uv_loop_close(loop);

    worker_unregister();
    service_exits();
    netdata_log_info("ACLK SYNC: Shutting down ACLK synchronization event loop");
}

static void aclk_synchronization_init(void)
{
    memset(&aclk_sync_config, 0, sizeof(aclk_sync_config));
    fatal_assert(0 == uv_thread_create(&aclk_sync_config.thread, aclk_synchronization, &aclk_sync_config));
}
#endif

// -------------------------------------------------------------

void sql_create_aclk_table(RRDHOST *host __maybe_unused, uuid_t *host_uuid __maybe_unused, uuid_t *node_id __maybe_unused)
{
#ifdef ENABLE_ACLK
    char uuid_str[UUID_STR_LEN];
    char host_guid[UUID_STR_LEN];
    int rc;

    uuid_unparse_lower_fix(host_uuid, uuid_str);
    uuid_unparse_lower(*host_uuid, host_guid);

    char sql[ACLK_SYNC_QUERY_SIZE];

    snprintfz(sql, sizeof(sql) - 1, TABLE_ACLK_ALERT, uuid_str);
    rc = db_execute(db_meta, sql);
    if (unlikely(rc))
        error_report("Failed to create ACLK alert table for host %s", host ? rrdhost_hostname(host) : host_guid);
    else {
        snprintfz(sql, sizeof(sql) - 1, INDEX_ACLK_ALERT1, uuid_str, uuid_str);
        rc = db_execute(db_meta, sql);
        if (unlikely(rc))
            error_report(
                "Failed to create ACLK alert table index 1 for host %s", host ? string2str(host->hostname) : host_guid);

        snprintfz(sql, sizeof(sql) - 1, INDEX_ACLK_ALERT2, uuid_str, uuid_str);
        rc = db_execute(db_meta, sql);
        if (unlikely(rc))
            error_report(
                "Failed to create ACLK alert table index 2 for host %s", host ? string2str(host->hostname) : host_guid);
    }
    if (likely(host) && unlikely(host->aclk_config))
        return;

    if (unlikely(!host))
        return;

    struct aclk_sync_cfg_t *wc = callocz(1, sizeof(struct aclk_sync_cfg_t));
    if (node_id && !uuid_is_null(*node_id))
        uuid_unparse_lower(*node_id, wc->node_id);

    host->aclk_config = wc;
    if (node_id && !host->node_id) {
        host->node_id = mallocz(sizeof(*host->node_id));
        uuid_copy(*host->node_id, *node_id);
    }

    wc->host = host;
    strcpy(wc->uuid_str, uuid_str);
    wc->alert_updates = 0;
    time_t now = now_realtime_sec();
    wc->node_info_send_time = (host == localhost || NULL == localhost) ? now - 25 : now;
#endif
}

#define SQL_FETCH_ALL_HOSTS                                                                                            \
    "SELECT host_id, hostname, registry_hostname, update_every, os, "                                                  \
    "timezone, hops, memory_mode, abbrev_timezone, utc_offset, program_name, "                                         \
    "program_version, entries, health_enabled, last_connected, "                                                       \
    "(SELECT CASE WHEN hl.label_value = 'true' THEN 1 ELSE 0 END FROM "                                                \
    "host_label hl WHERE hl.host_id = h.host_id AND hl.label_key = '_is_ephemeral'),  "                                \
    "(SELECT CASE WHEN ni.node_id is NULL THEN 0 ELSE 1 END FROM "                                                     \
    "node_instance ni WHERE ni.host_id = h.host_id) FROM host h WHERE hops > 0"

#define SQL_FETCH_ALL_INSTANCES                                                                                        \
    "SELECT ni.host_id, ni.node_id FROM host h, node_instance ni "                                                     \
    "WHERE h.host_id = ni.host_id AND ni.node_id IS NOT NULL"

void sql_aclk_sync_init(void)
{
    char *err_msg = NULL;
    int rc;

    if (unlikely(!db_meta)) {
        if (default_rrd_memory_mode != RRD_MEMORY_MODE_DBENGINE) {
            return;
        }
        error_report("Database has not been initialized");
        return;
    }

    netdata_log_info("Creating archived hosts");
    int number_of_children = 0;
    rc = sqlite3_exec_monitored(db_meta, SQL_FETCH_ALL_HOSTS, create_host_callback, &number_of_children, &err_msg);

    if (rc != SQLITE_OK) {
        error_report("SQLite error when loading archived hosts, rc = %d (%s)", rc, err_msg);
        sqlite3_free(err_msg);
    }

    netdata_log_info("Created %d archived hosts", number_of_children);
    // Trigger host context load for hosts that have been created
    metadata_queue_load_host_context(NULL);

#ifdef ENABLE_ACLK
    if (!number_of_children)
        aclk_queue_node_info(localhost, true);

    rc = sqlite3_exec_monitored(db_meta, SQL_FETCH_ALL_INSTANCES,aclk_config_parameters, NULL,&err_msg);

    if (rc != SQLITE_OK) {
        error_report("SQLite error when configuring host ACLK synchonization parameters, rc = %d (%s)", rc, err_msg);
        sqlite3_free(err_msg);
    }
    aclk_synchronization_init();

    netdata_log_info("ACLK sync initialization completed");
#endif
}

// Public

static inline void queue_aclk_sync_cmd(enum aclk_database_opcode opcode, const void *param0, const void *param1)
{
    struct aclk_database_cmd cmd;
    cmd.opcode = opcode;
    cmd.param[0] = (void *) param0;
    cmd.param[1] = (void *) param1;
    cmd.completion = NULL;
    aclk_database_enq_cmd(&cmd);
}

// Public
void aclk_push_alert_config(const char *node_id, const char *config_hash)
{
    if (unlikely(!aclk_sync_config.initialized))
        return;

    queue_aclk_sync_cmd(ACLK_DATABASE_PUSH_ALERT_CONFIG, strdupz(node_id), strdupz(config_hash));
}

void aclk_push_node_alert_snapshot(const char *node_id)
{
    if (unlikely(!aclk_sync_config.initialized))
        return;

    queue_aclk_sync_cmd(ACLK_DATABASE_PUSH_ALERT_SNAPSHOT, strdupz(node_id), NULL);
}


void aclk_push_node_removed_alerts(const char *node_id)
{
    if (unlikely(!aclk_sync_config.initialized))
        return;

    queue_aclk_sync_cmd(ACLK_DATABASE_QUEUE_REMOVED_ALERTS, strdupz(node_id), NULL);
}

void schedule_node_info_update(RRDHOST *host __maybe_unused)
{
#ifdef ENABLE_ACLK
    if (unlikely(!host))
        return;

    struct aclk_database_cmd cmd;
    memset(&cmd, 0, sizeof(cmd));
    cmd.opcode = ACLK_DATABASE_NODE_STATE;
    cmd.param[0] = host;
    cmd.completion = NULL;
    aclk_database_enq_cmd(&cmd);
#endif
}

#ifdef ENABLE_ACLK
void unregister_node(const char *machine_guid)
{
    if (unlikely(!machine_guid))
        return;

    struct aclk_database_cmd cmd;
    memset(&cmd, 0, sizeof(cmd));
    cmd.opcode = ACLK_DATABASE_NODE_UNREGISTER;
    cmd.param[0] = strdupz(machine_guid);
    cmd.completion = NULL;
    aclk_database_enq_cmd(&cmd);
}
#endif
