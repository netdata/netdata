// SPDX-License-Identifier: GPL-3.0-or-later

#include "sqlite_functions.h"
#include "sqlite_aclk.h"

void sanity_check(void) {
    // make sure the compiler will stop on misconfigurations
    BUILD_BUG_ON(WORKER_UTILIZATION_MAX_JOB_TYPES < ACLK_MAX_ENUMERATIONS_DEFINED);
}

#include "sqlite_aclk_node.h"
#include "../aclk_query_queue.h"
#include "../aclk_query.h"

struct aclk_sync_config_s {
    uv_thread_t thread;
    uv_loop_t loop;
    uv_timer_t timer_req;
    uv_async_t async;
    bool initialized;
    mqtt_wss_client client;
    int aclk_queries_running;
    SPINLOCK cmd_queue_lock;
    struct aclk_database_cmd *cmd_base;
} aclk_sync_config = { 0 };

static struct aclk_database_cmd aclk_database_deq_cmd(void)
{
    struct aclk_database_cmd ret = { 0 };

    spinlock_lock(&aclk_sync_config.cmd_queue_lock);
    if(aclk_sync_config.cmd_base) {
        struct aclk_database_cmd *t = aclk_sync_config.cmd_base;
        DOUBLE_LINKED_LIST_REMOVE_ITEM_UNSAFE(aclk_sync_config.cmd_base, t, prev, next);
        ret = *t;
        freez(t);
    }
    else {
        ret.opcode = ACLK_DATABASE_NOOP;
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
    uuid_unparse_lower(*(nd_uuid_t *)argv[IDX_HOST_ID], guid);

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

    sql_build_host_system_info((nd_uuid_t *)argv[IDX_HOST_ID], system_info);

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

        host->rrdlabels = sql_load_host_labels((nd_uuid_t *)argv[IDX_HOST_ID]);
        host->last_connected = last_connected;
    }

    (*number_of_chidren)++;

#ifdef NETDATA_INTERNAL_CHECKS
    char node_str[UUID_STR_LEN] = "<none>";
    if (likely(!UUIDiszero(host->node_id)))
        uuid_unparse_lower(host->node_id.uuid, node_str);
    internal_error(true, "Adding archived host \"%s\" with GUID \"%s\" node id = \"%s\"  ephemeral=%d",
                   rrdhost_hostname(host), host->machine_guid, node_str, is_ephemeral);
#endif
    return 0;
}


#define SQL_SELECT_ACLK_ALERT_TABLES                                                                                   \
    "SELECT 'DROP '||type||' IF EXISTS '||name||';' FROM sqlite_schema WHERE name LIKE 'aclk_alert_%' AND type IN ('table', 'trigger', 'index')"

static void sql_delete_aclk_table_list(void)
{
    sqlite3_stmt *res = NULL;

    BUFFER *sql = buffer_create(ACLK_SYNC_QUERY_SIZE, NULL);

    if (!PREPARE_STATEMENT(db_meta, SQL_SELECT_ACLK_ALERT_TABLES, &res))
        goto fail;

    while (sqlite3_step_monitored(res) == SQLITE_ROW)
        buffer_strcat(sql, (char *) sqlite3_column_text(res, 0));

    SQLITE_FINALIZE(res);

    int rc = db_execute(db_meta, buffer_tostring(sql));
    if (unlikely(rc))
        netdata_log_error("Failed to drop unused ACLK tables");

fail:
    buffer_free(sql);
}

#define SQL_INVALIDATE_HOST_LAST_CONNECTED "UPDATE host SET last_connected = 1 WHERE host_id = @host_id"

static void invalidate_host_last_connected(nd_uuid_t *host_uuid)
{
    sqlite3_stmt *res = NULL;
    if (!host_uuid)
        return;

    if (!PREPARE_STATEMENT(db_meta, SQL_INVALIDATE_HOST_LAST_CONNECTED, &res))
        return;

    int param = 0;
    SQLITE_BIND_FAIL(bind_fail, sqlite3_bind_blob(res, ++param, host_uuid, sizeof(*host_uuid), SQLITE_STATIC));

    param = 0;
    int rc = sqlite3_step_monitored(res);
    if (unlikely(rc != SQLITE_DONE)) {
        char wstr[UUID_STR_LEN];
        uuid_unparse_lower(*host_uuid, wstr);
        error_report("Failed invalidate last_connected time for host with GUID %s, rc = %d", wstr, rc);
    }

bind_fail:
    REPORT_BIND_FAIL(res, param);
    SQLITE_FINALIZE(res);
}


// OPCODE: ACLK_DATABASE_NODE_UNREGISTER
static void sql_unregister_node(char *machine_guid)
{
    int rc;
    nd_uuid_t host_uuid;

    if (unlikely(!machine_guid))
        return;

    rc = uuid_parse(machine_guid, host_uuid);
    if (rc)
        goto skip;

    sqlite3_stmt *res = NULL;

    if (!PREPARE_STATEMENT(db_meta, "UPDATE node_instance SET node_id = NULL WHERE host_id = @host_id", &res))
        goto skip;

    int param = 0;
    SQLITE_BIND_FAIL(done, sqlite3_bind_blob(res, ++param, &host_uuid, sizeof(host_uuid), SQLITE_STATIC));
    param = 0;

    rc = sqlite3_step_monitored(res);
    if (unlikely(rc != SQLITE_DONE)) {
        error_report("Failed to execute command to remove host node id");
    } else {
       // node: machine guid will be freed after processing
       invalidate_host_last_connected(&host_uuid);
       metadata_delete_host_chart_labels(machine_guid);
       machine_guid = NULL;
    }

done:
    REPORT_BIND_FAIL(res, param);
    SQLITE_FINALIZE(res);
skip:
    freez(machine_guid);
}

static int aclk_config_parameters(void *data __maybe_unused, int argc __maybe_unused, char **argv, char **column __maybe_unused)
{
    char uuid_str[UUID_STR_LEN];
    uuid_unparse_lower(*((nd_uuid_t *) argv[0]), uuid_str);

    RRDHOST *host = rrdhost_find_by_guid(uuid_str);
    if (host != localhost)
        create_aclk_config(host, (nd_uuid_t *)argv[0], (nd_uuid_t *)argv[1]);
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

    struct aclk_database_cmd cmd = { 0 };
    if (aclk_online_for_alerts()) {
        cmd.opcode = ACLK_DATABASE_PUSH_ALERT;
        aclk_database_enq_cmd(&cmd);
        aclk_check_node_info_and_collectors();
    }
}

struct aclk_query_payload {
    uv_work_t request;
    void *data;
    struct aclk_sync_config_s *config;
};

static void after_aclk_run_query_job(uv_work_t *req, int status __maybe_unused)
{
    worker_is_busy(ACLK_QUERY_EXECUTE);
    struct aclk_query_payload *payload = req->data;
    struct aclk_sync_config_s *config = payload->config;
    config->aclk_queries_running--;
    freez(payload);
}

static void aclk_run_query(struct aclk_sync_config_s *config, aclk_query_t query)
{
    if (query->type == UNKNOWN || query->type >= ACLK_QUERY_TYPE_COUNT) {
        error_report("Unknown query in query queue. %u", query->type);
        return;
    }

    if (query->type == HTTP_API_V2) {
        http_api_v2(config->client, query);
    } else {
        send_bin_msg(config->client, query);
    }
    aclk_query_free(query);
}

static void aclk_run_query_job(uv_work_t *req)
{
    struct aclk_query_payload *payload =  req->data;
    struct aclk_sync_config_s *config = payload->config;
    aclk_query_t query = (aclk_query_t) payload->data;

    aclk_run_query(config, query);
}

static int read_query_thread_count()
{
    int threads = MIN(get_netdata_cpus()/2, 6);
    threads = MAX(threads, 2);
    threads = config_get_number(CONFIG_SECTION_CLOUD, "query thread count", threads);
    if(threads < 1) {
        netdata_log_error("You need at least one query thread. Overriding configured setting of \"%d\"", threads);
        threads = 1;
        config_set_number(CONFIG_SECTION_CLOUD, "query thread count", threads);
    }
    else {
        if (threads > libuv_worker_threads / 2) {
            threads = MAX(libuv_worker_threads / 2, 2);
            config_set_number(CONFIG_SECTION_CLOUD, "query thread count", threads);
        }
    }
    return threads;
}

static void aclk_synchronization(void *arg)
{
    struct aclk_sync_config_s *config = arg;
    uv_thread_set_name_np("ACLKSYNC");
    worker_register("ACLKSYNC");
    service_register(SERVICE_THREAD_TYPE_EVENT_LOOP, NULL, NULL, NULL, true);

    worker_register_job_name(ACLK_DATABASE_NOOP,                 "noop");
    worker_register_job_name(ACLK_DATABASE_NODE_STATE,           "node state");
    worker_register_job_name(ACLK_DATABASE_PUSH_ALERT,           "alert push");
    worker_register_job_name(ACLK_DATABASE_PUSH_ALERT_CONFIG,    "alert conf push");
    worker_register_job_name(ACLK_QUERY_EXECUTE,                 "query execute");
    worker_register_job_name(ACLK_QUERY_EXECUTE_SYNC,            "query execute sync");
    worker_register_job_name(ACLK_DATABASE_TIMER,                "timer");

    uv_loop_t *loop = &config->loop;
    fatal_assert(0 == uv_loop_init(loop));
    fatal_assert(0 == uv_async_init(loop, &config->async, async_cb));

    fatal_assert(0 == uv_timer_init(loop, &config->timer_req));
    config->timer_req.data = config;
    fatal_assert(0 == uv_timer_start(&config->timer_req, timer_cb, TIMER_PERIOD_MS, TIMER_PERIOD_MS));

    netdata_log_info("Starting ACLK synchronization thread");

    config->initialized = true;

    sql_delete_aclk_table_list();

    int query_thread_count = read_query_thread_count();
    netdata_log_info("Starting ACLK synchronization thread with %d parallel query threads", query_thread_count);

    while (likely(service_running(SERVICE_ACLK))) {
        enum aclk_database_opcode opcode;
        worker_is_idle();
        uv_run(loop, UV_RUN_DEFAULT);

        /* wait for commands */
        do {
            struct aclk_database_cmd cmd = aclk_database_deq_cmd();

            if (unlikely(!service_running(SERVICE_ACLK)))
                break;

            opcode = cmd.opcode;

            if(likely(opcode != ACLK_DATABASE_NOOP && opcode != ACLK_QUERY_EXECUTE))
                worker_is_busy(opcode);

            switch (opcode) {
                case ACLK_DATABASE_NOOP:
                    /* the command queue was empty, do nothing */
                    break;
// NODE STATE
                case ACLK_DATABASE_NODE_STATE:;
                    RRDHOST *host = cmd.param[0];
                    int live = (host == localhost || host->receiver || !(rrdhost_flag_check(host, RRDHOST_FLAG_ORPHAN))) ? 1 : 0;
                    struct aclk_sync_cfg_t *ahc = host->aclk_config;
                    if (unlikely(!ahc))
                        create_aclk_config(host, &host->host_id.uuid, &host->node_id.uuid);
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

                case ACLK_MQTT_WSS_CLIENT:
                    config->client = (mqtt_wss_client) cmd.param[0];
                    break;

                case ACLK_QUERY_EXECUTE:;
                    aclk_query_t query = (aclk_query_t)cmd.param[0];

                    struct aclk_query_payload *payload = NULL;
                    config->aclk_queries_running++;
                    bool execute_now = (config->aclk_queries_running > query_thread_count);
                    if (!execute_now) {
                        payload = mallocz(sizeof(*payload));
                        payload->request.data = payload;
                        payload->config = config;
                        payload->data = query;
                        execute_now = uv_queue_work(loop, &payload->request, aclk_run_query_job, after_aclk_run_query_job);
                    }

                    if (execute_now) {
                        worker_is_busy(ACLK_QUERY_EXECUTE_SYNC);
                        aclk_run_query(config, query);
                        freez(payload);
                        config->aclk_queries_running--;
                    }
                    break;

                default:
                    break;
            }
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

// -------------------------------------------------------------

void create_aclk_config(RRDHOST *host __maybe_unused, nd_uuid_t *host_uuid __maybe_unused, nd_uuid_t *node_id __maybe_unused)
{

    if (!host || host->aclk_config)
        return;

    struct aclk_sync_cfg_t *wc = callocz(1, sizeof(struct aclk_sync_cfg_t));
    if (node_id && !uuid_is_null(*node_id))
        uuid_unparse_lower(*node_id, wc->node_id);

    host->aclk_config = wc;
    if (node_id && UUIDiszero(host->node_id)) {
        uuid_copy(host->node_id.uuid, *node_id);
    }

    wc->host = host;
    wc->stream_alerts = false;
    time_t now = now_realtime_sec();
    wc->node_info_send_time = (host == localhost || NULL == localhost) ? now - 25 : now;
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

    REQUIRE_DB(db_meta);

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

    if (!number_of_children)
        aclk_queue_node_info(localhost, true);

    rc = sqlite3_exec_monitored(db_meta, SQL_FETCH_ALL_INSTANCES, aclk_config_parameters, NULL, &err_msg);

    if (rc != SQLITE_OK) {
        error_report("SQLite error when configuring host ACLK synchonization parameters, rc = %d (%s)", rc, err_msg);
        sqlite3_free(err_msg);
    }
    aclk_synchronization_init();

    netdata_log_info("ACLK sync initialization completed");
}

static inline void queue_aclk_sync_cmd(enum aclk_database_opcode opcode, const void *param0, const void *param1)
{
    struct aclk_database_cmd cmd;
    cmd.opcode = opcode;
    cmd.param[0] = (void *) param0;
    cmd.param[1] = (void *) param1;
    aclk_database_enq_cmd(&cmd);
}

// Public
void aclk_push_alert_config(const char *node_id, const char *config_hash)
{
    if (unlikely(!aclk_sync_config.initialized))
        return;

    queue_aclk_sync_cmd(ACLK_DATABASE_PUSH_ALERT_CONFIG, strdupz(node_id), strdupz(config_hash));
}

void aclk_execute_query(aclk_query_t query)
{
    if (unlikely(!aclk_sync_config.initialized))
        return;

    queue_aclk_sync_cmd(ACLK_QUERY_EXECUTE, query, NULL);
}

void aclk_query_init(mqtt_wss_client client) {

    queue_aclk_sync_cmd(ACLK_MQTT_WSS_CLIENT, client, NULL);
}

void schedule_node_info_update(RRDHOST *host __maybe_unused)
{
    if (unlikely(!host))
        return;
    queue_aclk_sync_cmd(ACLK_DATABASE_NODE_STATE, host, NULL);
}

void unregister_node(const char *machine_guid)
{
    if (unlikely(!machine_guid))
        return;
    queue_aclk_sync_cmd(ACLK_DATABASE_NODE_UNREGISTER, strdupz(machine_guid), NULL);
}
