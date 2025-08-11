// SPDX-License-Identifier: GPL-3.0-or-later

#include "sqlite_functions.h"
#include "sqlite_aclk.h"

void sanity_check(void) {
    // make sure the compiler will stop on misconfigurations
    BUILD_BUG_ON(WORKER_UTILIZATION_MAX_JOB_TYPES < ACLK_MAX_ENUMERATIONS_DEFINED);
}

#include "sqlite_aclk_node.h"
#include "aclk/aclk_query_queue.h"
#include "aclk/aclk_query.h"

static void create_node_instance_result_job(const char *machine_guid, const char *node_id)
{
    nd_uuid_t host_uuid, node_uuid;

    if (uuid_parse(machine_guid, host_uuid)) {
        netdata_log_error("Error parsing machine_guid provided by CreateNodeInstanceResult");
        return;
    }

    if (uuid_parse(node_id, node_uuid)) {
        netdata_log_error("Error parsing node_id provided by CreateNodeInstanceResult");
        return;
    }

    RRDHOST *host = rrdhost_find_by_guid(machine_guid);
    if (unlikely(!host)) {
        netdata_log_error("Cannot find machine_guid provided by CreateNodeInstanceResult");
        return;
    }
    sql_update_node_id(&host_uuid, &node_uuid);
    schedule_node_state_update(host, 1000);
}

struct aclk_sync_config_s {
    ND_THREAD *thread;
    uv_loop_t loop;
    uv_timer_t timer_req;
    uv_async_t async;
    bool initialized;
    bool shutdown_requested;
    mqtt_wss_client client;
    int aclk_queries_running;
    bool run_query_batch;
    bool alert_push_running;
    bool aclk_batch_job_is_running;
    uint32_t aclk_jobs_pending;
    struct completion start_stop_complete;
    CmdPool cmd_pool;
    WorkerPool worker_pool;
} aclk_sync_config = { 0 };

static cmd_data_t aclk_database_deq_cmd(void)
{
    cmd_data_t ret = { 0 };
    ret.opcode = ACLK_DATABASE_NOOP;
    (void) pop_cmd(&aclk_sync_config.cmd_pool, (cmd_data_t *) &ret);
    return ret;
}

static bool aclk_database_enq_cmd(cmd_data_t *cmd, bool wait_on_full)
{
    if(unlikely(!__atomic_load_n(&aclk_sync_config.initialized, __ATOMIC_RELAXED)))
        return false;

    bool added = push_cmd(&aclk_sync_config.cmd_pool, (void *)cmd, wait_on_full);
    if (added)
        (void) uv_async_send(&aclk_sync_config.async);
    return added;
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

    if (is_ephemeral && ((!is_registered && last_connected == 1) || (rrdhost_free_ephemeral_time_s && age > rrdhost_free_ephemeral_time_s))) {
        netdata_log_info(
            "%s ephemeral hostname \"%s\" with GUID \"%s\", age = %ld seconds (limit %ld seconds)",
            is_registered ? "Loading registered" : "Skipping unregistered",
            (const char *)argv[IDX_HOSTNAME],
            guid,
            age,
            rrdhost_free_ephemeral_time_s);

        if (!is_registered)
           goto done;
    }

    struct rrdhost_system_info *system_info = rrdhost_system_info_create();

    rrdhost_system_info_hops_set(system_info, (int16_t)str2i((const char *) argv[IDX_HOPS]));

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
        0,              // health
        0,              // rrdpush enabled
        NULL,           // destination
        NULL,           // api key
        NULL,           // send charts matching
        false,          // rrdpush_enable_replication
        0,              // rrdpush_seconds_to_replicate
        0,              // rrdpush_replication_step
        system_info,
        1);

    rrdhost_system_info_free(system_info);

    if (likely(host)) {
        if (is_ephemeral)
            rrdhost_option_set(host, RRDHOST_OPTION_EPHEMERAL_HOST);

        if (is_ephemeral)
            host->stream.rcv.status.last_disconnected = now_realtime_sec();

        host->rrdlabels = sql_load_host_labels((nd_uuid_t *)argv[IDX_HOST_ID]);
        host->stream.snd.status.last_connected = last_connected;

        if (argv[IDX_OS] && strcmp(argv[IDX_OS], NETDATA_VIRTUAL_HOST) == 0)
            rrdhost_option_set(host, RRDHOST_OPTION_VIRTUAL_HOST);

        pulse_host_status(host, 0, 0); // this will detect the receiver status
    }

    (*number_of_chidren)++;

#ifdef NETDATA_INTERNAL_CHECKS
    char node_str[UUID_STR_LEN] = "<none>";
    if (likely(!UUIDiszero(host->node_id)))
        uuid_unparse_lower(host->node_id.uuid, node_str);
    internal_error(true, "Adding archived host \"%s\" with GUID \"%s\" node id = \"%s\"  ephemeral=%d",
                   rrdhost_hostname(host), host->machine_guid, node_str, is_ephemeral);
#endif

done:
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

    int rc = db_execute(db_meta, buffer_tostring(sql), NULL);
    if (unlikely(rc))
        netdata_log_error("Failed to drop unused ACLK tables");

fail:
    buffer_free(sql);
}

#define SQL_INVALIDATE_HOST_LAST_CONNECTED "UPDATE host SET last_connected = 1 WHERE host_id = @host_id"

static void invalidate_host_last_connected(nd_uuid_t *host_uuid)
{
    sqlite3_stmt *res = NULL;

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
    if (unlikely(rc != SQLITE_DONE))
        error_report("Failed to execute command to remove host node id");
    else
       invalidate_host_last_connected(&host_uuid);

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

struct judy_list_t {
    Pvoid_t JudyL;
    Word_t count;
};

static void async_cb(uv_async_t *handle)
{
    uv_stop(handle->loop);
    uv_update_time(handle->loop);
}

#define TIMER_PERIOD_MS (1000)

static void after_aclk_run_query_job(uv_work_t *req, int status __maybe_unused)
{
    worker_data_t *worker = req->data;
    struct aclk_sync_config_s *config = worker->config;
    config->aclk_queries_running--;
    return_worker(&config->worker_pool, worker);
}

static void aclk_run_query(struct aclk_sync_config_s *config, aclk_query_t *query)
{
    if (query->type == UNKNOWN || query->type >= ACLK_QUERY_TYPE_COUNT) {
        error_report("Unknown query in query queue. %u", query->type);
        return;
    }

    bool ok_to_send = true;
    mqtt_wss_client client = __atomic_load_n(&config->client, __ATOMIC_RELAXED);

    switch (query->type) {

// Incoming : cloud -> agent
        case HTTP_API_V2:
            worker_is_busy(UV_EVENT_ACLK_QUERY_EXECUTE);
            if (client)
                http_api_v2(client, query);
            ok_to_send = false;
            break;
        case CTX_CHECKPOINT:
            worker_is_busy(UV_EVENT_CTX_CHECKPOINT);
            rrdcontext_hub_checkpoint_command(query->data.payload);
            ok_to_send = false;
            break;
        case CTX_STOP_STREAMING:
            worker_is_busy(UV_EVENT_CTX_STOP_STREAMING);
            rrdcontext_hub_stop_streaming_command(query->data.payload);
            ok_to_send = false;
            break;
        case SEND_NODE_INSTANCES:
            worker_is_busy(UV_EVENT_SEND_NODE_INSTANCES);
            aclk_send_node_instances();
            ok_to_send = false;
            break;
        case ALERT_START_STREAMING:
            worker_is_busy(UV_EVENT_ALERT_START_STREAMING);
            aclk_start_alert_streaming(query->data.node_id, query->version);
            ok_to_send = false;
            break;
        case ALERT_CHECKPOINT:
            worker_is_busy(UV_EVENT_ALERT_CHECKPOINT);
            aclk_alert_version_check(query->data.node_id, query->claim_id, query->version);
            ok_to_send = false;
            break;
        case CREATE_NODE_INSTANCE:
            worker_is_busy(UV_EVENT_CREATE_NODE_INSTANCE);
            create_node_instance_result_job(query->machine_guid, query->data.node_id);
            ok_to_send = false;
            break;

// Outgoing: agent -> cloud
        case ALARM_PROVIDE_CFG:
            worker_is_busy(UV_EVENT_ALARM_PROVIDE_CFG);
            break;
        case ALARM_SNAPSHOT:
            worker_is_busy(UV_EVENT_ALARM_SNAPSHOT);
            break;
        case REGISTER_NODE:
            worker_is_busy(UV_EVENT_REGISTER_NODE);
            break;
        case UPDATE_NODE_COLLECTORS:
            worker_is_busy(UV_EVENT_UPDATE_NODE_COLLECTORS);
            break;
        case UPDATE_NODE_INFO:
            worker_is_busy(UV_EVENT_UPDATE_NODE_INFO);
            break;
        case CTX_SEND_SNAPSHOT:
            worker_is_busy(UV_EVENT_CTX_SEND_SNAPSHOT);
            break;
        case CTX_SEND_SNAPSHOT_UPD:
            worker_is_busy(UV_EVENT_CTX_SEND_SNAPSHOT_UPD);
            break;
        case NODE_STATE_UPDATE:
            worker_is_busy(UV_EVENT_NODE_STATE_UPDATE);
            break;
        default:
            nd_log_daemon(NDLP_ERR, "Unknown msg type %u; ignoring", query->type);
            ok_to_send = false;
            break;
    }

    if (ok_to_send) {
        if (client)
            send_bin_msg(client, query);
        else {
            freez(query->data.bin_payload.payload);
            nd_log_daemon(NDLP_ERR, "No client to send message %u", query->type);
        }
    }

    aclk_query_free(query);
}

static void aclk_run_query_job(uv_work_t *req)
{
    register_libuv_worker_jobs();

    worker_data_t *worker = req->data;
    struct aclk_sync_config_s *config = worker->config;
    aclk_query_t *query = (aclk_query_t *)worker->payload;

    aclk_run_query(config, query);
    worker_is_idle();
}

static void after_aclk_execute_batch(uv_work_t *req, int status __maybe_unused)
{
    worker_data_t *worker = req->data;
    struct aclk_sync_config_s *config = worker->config;
    config->aclk_batch_job_is_running = false;
    return_worker(&config->worker_pool, worker);
}

static void aclk_execute_batch(uv_work_t *req)
{
    register_libuv_worker_jobs();

    worker_data_t *worker = req->data;
    struct aclk_sync_config_s *config = worker->config;
    struct judy_list_t *aclk_query_batch = worker->payload;

    if (!aclk_query_batch)
        return;

    usec_t started_ut = now_monotonic_usec(); (void)started_ut;

    size_t entries = aclk_query_batch->count;
    Word_t Index = 0;
    bool first = true;
    Pvoid_t *Pvalue;
    while ((Pvalue = JudyLFirstThenNext(aclk_query_batch->JudyL, &Index, &first))) {
        if (!*Pvalue)
            continue;

        aclk_query_t *query = *Pvalue;
        aclk_run_query(config, query);
    }

    (void) JudyLFreeArray(&aclk_query_batch->JudyL, PJE0);
    freez(aclk_query_batch);

    usec_t ended_ut = now_monotonic_usec();
    (void)ended_ut;
    nd_log_daemon(
        NDLP_DEBUG, "Processed %zu ACLK commands in %0.2f ms", entries, (double)(ended_ut - started_ut) / USEC_PER_MS);

    worker_is_idle();
}

struct notify_timer_cb_data {
    void *payload;
    struct completion *completion;
};

static void after_do_unregister_node(uv_work_t *req, int status __maybe_unused)
{
    worker_data_t *worker = req->data;
    struct aclk_sync_config_s *config = worker->config;
    return_worker(&config->worker_pool, worker);
}

static void do_unregister_node(uv_work_t *req)
{
    register_libuv_worker_jobs();

    worker_data_t *worker =  req->data;

    worker_is_busy(UV_EVENT_UNREGISTER_NODE);

    sql_unregister_node(worker->payload);

    worker_is_idle();
}

static void notify_timer_close_callback(uv_handle_t *handle)
{
    struct notify_timer_cb_data *data = handle->data;
    if (data->completion) {
        completion_mark_complete(data->completion);
    }
    freez(data);
}

static void node_update_timer_cb(uv_timer_t *handle)
{
    struct aclk_sync_cfg_t *aclk_host_config = handle->data;
    if (unlikely(!aclk_host_config))
        return;

    RRDHOST *host = aclk_host_config->host;

    if(!host || aclk_host_state_update_auto(host))
        uv_timer_stop(&aclk_host_config->timer);
}

static void after_start_alert_push(uv_work_t *req, int status __maybe_unused)
{
    struct worker_data *worker = req->data;
    struct aclk_sync_config_s *config = worker->config;

    config->alert_push_running = false;
    return_worker(&config->worker_pool, worker);
}

// Worker thread to scan hosts for pending metadata to store
static void start_alert_push(uv_work_t *req __maybe_unused)
{
    register_libuv_worker_jobs();

    worker_is_busy(UV_EVENT_ACLK_NODE_INFO);
    aclk_check_node_info_and_collectors();
    worker_is_idle();

    worker_is_busy(UV_EVENT_ACLK_ALERT_PUSH);
    aclk_push_alert_events_for_all_hosts();
    worker_is_idle();
}

#define MAX_ACLK_BATCH_JOBS_IN_QUEUE (20)

#define MAX_BATCH_SIZE (64)

// Take a query, and try to schedule it in a worker
// Update config->aclk_queries_running if success
// config->aclk_queries_running is only accessed from the vent loop
// On failure: free the payload

int schedule_query_in_worker(uv_loop_t *loop, struct aclk_sync_config_s *config, aclk_query_t *query) {

    worker_data_t *worker = get_worker(&config->worker_pool);
    worker->payload = query;
    worker->config = config;

    config->aclk_queries_running++;
    int rc = uv_queue_work(loop, &worker->request, aclk_run_query_job, after_aclk_run_query_job);
    if (rc) {
        config->aclk_queries_running--;
        return_worker(&config->worker_pool, worker);
    }
    return rc;
}

static void free_query_list(Pvoid_t JudyL)
{
    bool first = true;
    Pvoid_t *Pvalue;
    Word_t Index = 0;
    aclk_query_t *query;
    while ((Pvalue = JudyLFirstThenNext(JudyL, &Index, &first))) {
        if (!*Pvalue)
            continue;
        query = *Pvalue;
        aclk_query_free(query);
    }
}

static void timer_cb(uv_timer_t *handle)
{
    uv_stop(handle->loop);
    uv_update_time(handle->loop);
    struct aclk_sync_config_s *config = handle->data;

    if (aclk_online_for_alerts()) {
        worker_data_t *worker;
        if (!config->alert_push_running) {
            worker = get_worker(&config->worker_pool);
            worker->config = config;
            config->alert_push_running = true;
            if (uv_queue_work(handle->loop, &worker->request, start_alert_push, after_start_alert_push)) {
                config->alert_push_running = false;
                return_worker(&config->worker_pool, worker);
            }
        }
    }

    if (config->aclk_jobs_pending > 0)
        config->run_query_batch = true;
}

#define MAX_SHUTDOWN_TIMEOUT_SECONDS (5)
#define SHUTDOWN_SLEEP_INTERVAL_MS (100)
#define CMD_POOL_SIZE (2048)

#define ACLK_JOBS_ARE_RUNNING                                                                                          \
    (config->aclk_queries_running || config->alert_push_running || config->aclk_batch_job_is_running)

static void *aclk_synchronization_event_loop(void *arg)
{
    struct aclk_sync_config_s *config = arg;
    uv_thread_set_name_np("ACLKSYNC");
    init_cmd_pool(&config->cmd_pool, CMD_POOL_SIZE);

    worker_register("ACLKSYNC");

    service_register(NULL, NULL, NULL);

    worker_register_job_name(ACLK_DATABASE_NOOP,                "noop");
    worker_register_job_name(ACLK_DATABASE_NODE_STATE,          "node state");
    worker_register_job_name(ACLK_DATABASE_PUSH_ALERT_CONFIG,   "alert conf push");
    worker_register_job_name(ACLK_QUERY_BATCH_EXECUTE,          "aclk batch execute");
    worker_register_job_name(ACLK_QUERY_BATCH_ADD,              "aclk batch add");
    worker_register_job_name(ACLK_MQTT_WSS_CLIENT_SET,          "config mqtt client");
    worker_register_job_name(ACLK_MQTT_WSS_CLIENT_RESET,        "reset mqtt client");
    worker_register_job_name(ACLK_DATABASE_NODE_UNREGISTER,     "unregister node");
    worker_register_job_name(ACLK_CANCEL_NODE_UPDATE_TIMER,     "cancel node update timer");
    worker_register_job_name(ACLK_QUEUE_NODE_INFO,              "queue node info");

    uv_loop_t *loop = &config->loop;
    fatal_assert(0 == uv_loop_init(loop));
    fatal_assert(0 == uv_async_init(loop, &config->async, async_cb));

    fatal_assert(0 == uv_timer_init(loop, &config->timer_req));
    config->timer_req.data = config;
    fatal_assert(0 == uv_timer_start(&config->timer_req, timer_cb, TIMER_PERIOD_MS, TIMER_PERIOD_MS));

    netdata_log_info("Starting ACLK synchronization thread");

    sql_delete_aclk_table_list();

    int query_thread_count = (int) netdata_conf_cloud_query_threads();
    netdata_log_info("Starting ACLK synchronization thread with %d parallel query threads", query_thread_count);

    struct notify_timer_cb_data *timer_cb_data;

    // This holds queries that need to be executed one by one
    struct judy_list_t *aclk_query_batch = NULL;

    // This holds queries that can be dispatched in parallel in ACLK QUERY worker threads
    struct judy_list_t *aclk_query_execute = callocz(1, sizeof(*aclk_query_execute));
    size_t pending_queries = 0;

    Pvoid_t *Pvalue;
    worker_data_t  *worker;

    unsigned cmd_batch_size;

    config->shutdown_requested = false;
    config->initialized = true;
    completion_mark_complete(&config->start_stop_complete);

    while (likely(config->shutdown_requested == false))  {
        enum aclk_database_opcode opcode;
        RRDHOST *host;
        struct aclk_sync_cfg_t *aclk_host_config;
        aclk_query_t *query;
        worker_is_idle();
        uv_run(loop, UV_RUN_DEFAULT);

        /* wait for commands */
        cmd_batch_size = 0;
        do {
            cmd_data_t cmd;

            if (unlikely(++cmd_batch_size >= MAX_BATCH_SIZE))
                break;

            if (config->run_query_batch) {
                opcode = ACLK_QUERY_BATCH_EXECUTE;
                config->run_query_batch = false;
            }
            else
            {
                cmd = aclk_database_deq_cmd();
                opcode = cmd.opcode;
            }

            if(likely(opcode != ACLK_DATABASE_NOOP && opcode != ACLK_QUERY_EXECUTE))
                worker_is_busy(opcode);

            // Check if we have pending commands to execute
            if (opcode == ACLK_DATABASE_NOOP && pending_queries && config->aclk_queries_running < query_thread_count) {
                opcode = ACLK_QUERY_EXECUTE;
                cmd.param[0] = NULL;
            }

            switch (opcode) {
                case ACLK_DATABASE_NOOP:
                    /* the command queue was empty, do nothing */
                    break;
                    // NODE STATE
                case ACLK_DATABASE_NODE_STATE:
                    host = cmd.param[0];
                    aclk_host_config = host->aclk_host_config;
                    if (unlikely(!aclk_host_config)) {
                        create_aclk_config(host, &host->host_id.uuid, &host->node_id.uuid);
                        aclk_host_config = host->aclk_host_config;
                    }

                    if (aclk_host_config) {
                        uint64_t schedule_time = (uint64_t)(uintptr_t)cmd.param[1];
                        if (!aclk_host_config->timer_initialized) {
                            int rc = uv_timer_init(loop, &aclk_host_config->timer);
                            if (!rc) {
                                aclk_host_config->timer_initialized = true;
                                aclk_host_config->timer.data = aclk_host_config;
                            }
                        }

                        if (aclk_host_config->timer_initialized) {
                            if (uv_is_active((uv_handle_t *)&aclk_host_config->timer))
                                uv_timer_stop(&aclk_host_config->timer);

                            aclk_host_config->timer.data = aclk_host_config;
                            int rc = uv_timer_start(&aclk_host_config->timer, node_update_timer_cb, schedule_time, 5000);
                            if (!rc)
                                break; // Timer started, exit
                        }
                    }

                    // This is fallback if timer fails
                    aclk_host_state_update_auto(host);
                    break;
                case ACLK_QUEUE_NODE_INFO:
                    host = cmd.param[0];
                    bool immediate = (bool)(uintptr_t)cmd.param[1];
                    aclk_host_config = host->aclk_host_config;
                    if (unlikely(!aclk_host_config)) {
                        create_aclk_config(host, &host->host_id.uuid, &host->node_id.uuid);
                        aclk_host_config = host->aclk_host_config;
                    }
                    aclk_host_config->node_info_send_time = (host == localhost ||(void *)(uintptr_t) immediate) ? 1 : now_realtime_sec();
                    break;
                case ACLK_CANCEL_NODE_UPDATE_TIMER:
                    host = cmd.param[0];
                    struct completion *compl = cmd.param[1];
                    aclk_host_config = host->aclk_host_config;
                    if (!aclk_host_config || !aclk_host_config->timer_initialized) {
                        completion_mark_complete(compl);
                        break;
                    }
                    if (uv_is_active((uv_handle_t *)&aclk_host_config->timer))
                        uv_timer_stop(&aclk_host_config->timer);

                    aclk_host_config->timer_initialized = false;
                    timer_cb_data = mallocz(sizeof(*timer_cb_data));
                    timer_cb_data->payload = host;
                    timer_cb_data->completion = compl;
                    aclk_host_config->timer.data = timer_cb_data;
                    uv_close((uv_handle_t *)&aclk_host_config->timer, notify_timer_close_callback);
                    break;

                case ACLK_DATABASE_NODE_UNREGISTER:
                    worker = get_worker(&config->worker_pool);
                    worker->config = config;
                    worker->payload = cmd.param[0];

                    if (uv_queue_work(loop, &worker->request, do_unregister_node, after_do_unregister_node)) {
                        freez(cmd.param[0]);
                        return_worker(&config->worker_pool, worker);
                    }
                    break;
                case ACLK_DATABASE_PUSH_ALERT_CONFIG:
                    aclk_push_alert_config_event(cmd.param[0], cmd.param[1]);
                    break;
                case ACLK_MQTT_WSS_CLIENT_SET:
                    config->client = (mqtt_wss_client)cmd.param[0];
                    break;
                case ACLK_MQTT_WSS_CLIENT_RESET:
                    __atomic_store_n(&config->client, NULL, __ATOMIC_RELEASE);
                    struct completion *comp = cmd.param[0];
                    completion_mark_complete(comp);
                    break;
                case ACLK_QUERY_EXECUTE:
                    query = (aclk_query_t *) cmd.param[0];

                    bool too_busy = (config->aclk_queries_running >= query_thread_count);

                    // If we are busy and it's just a ping to run, leave
                    if (too_busy && !query)
                        break;

                    // if we are busy (we have a query) store it and leave
                    if (too_busy) {
                        Pvalue = JudyLIns(&aclk_query_execute->JudyL, ++aclk_query_execute->count, PJE0);
                        if (Pvalue != PJERR) {
                            *Pvalue = query;
                            pending_queries++;
                        } else
                            nd_log_daemon(NDLP_ERR, "Failed to add ACLK command to the pending commands Judy");
                        break;
                    }

                    // Here: we are not busy
                    // If we have query it was a normal incoming command
                    // if we dont, it was a ping from the callback

                    // Lets try to queue as many of the pending commands
                    while(!too_busy && pending_queries && config->aclk_queries_running < query_thread_count) {

                        Word_t Index = 0;
                        Pvalue = JudyLFirst(aclk_query_execute->JudyL, &Index, PJE0);

                        // We have nothing, leave
                        if (Pvalue == NULL)
                            break;
                        aclk_query_t *query_in_queue = *Pvalue;

                        // Schedule it and increase running
                        too_busy = schedule_query_in_worker(loop, config, query_in_queue);

                        // It was scheduled in worker, remove it from pending
                        if (!too_busy) {
                            pending_queries--;
                            (void)JudyLDel(&aclk_query_execute->JudyL, Index, PJE0);
                        }
                    }

                    // Was it just a ping to run? leave
                    if (!query)
                        break;

                    // We have a query, if not busy lets run it
                    if (!too_busy)
                        too_busy = schedule_query_in_worker(loop, config, query);

                    // We were either busy, or failed to start worker, schedule for later
                    if (too_busy) {
                        Pvalue = JudyLIns(&aclk_query_execute->JudyL, ++aclk_query_execute->count, PJE0);
                        if (Pvalue != PJERR) {
                            *Pvalue = query;
                            pending_queries++;
                        }
                        else
                            nd_log_daemon(NDLP_ERR, "Failed to add ACLK command to the pending commands Judy");
                    }
                    break;

// Note: The following two opcodes must be in this order
                case ACLK_QUERY_BATCH_ADD:
                    query = (aclk_query_t *)cmd.param[0];
                    if (!query)
                        break;

                    if (!aclk_query_batch)
                        aclk_query_batch = callocz(1, sizeof(*aclk_query_batch));

                    Pvalue = JudyLIns(&aclk_query_batch->JudyL, ++aclk_query_batch->count, PJE0);
                    if (Pvalue)
                        *Pvalue = query;

                    config->aclk_jobs_pending++;
                    if (aclk_query_batch->count < MAX_ACLK_BATCH_JOBS_IN_QUEUE || config->aclk_batch_job_is_running)
                        break;
                    // fall through
                case ACLK_QUERY_BATCH_EXECUTE:
                    if (!aclk_query_batch || config->aclk_batch_job_is_running)
                        break;

                    worker = get_worker(&config->worker_pool);
                    worker->config = config;
                    worker->payload = aclk_query_batch;

                    config->aclk_batch_job_is_running = true;
                    config->aclk_jobs_pending -= aclk_query_batch->count;
                    aclk_query_batch = NULL;

                    if (uv_queue_work(loop, &worker->request, aclk_execute_batch, after_aclk_execute_batch)) {
                        aclk_query_batch = worker->payload;
                        config->aclk_jobs_pending += aclk_query_batch->count;
                        return_worker(&config->worker_pool, worker);
                        config->aclk_batch_job_is_running = false;
                    }
                    break;
                case ACLK_SYNC_SHUTDOWN:
                    config->shutdown_requested = true;
                    mark_pending_req_cancel_all();
                    break;
                default:
                    break;
            }
        } while (opcode != ACLK_DATABASE_NOOP);
    }
    config->initialized = false;

    if (!uv_timer_stop(&config->timer_req))
        uv_close((uv_handle_t *)&config->timer_req, NULL);

    uv_close((uv_handle_t *)&config->async, NULL);
    uv_walk(loop, libuv_close_callback, notify_timer_close_callback);

    size_t loop_count = (MAX_SHUTDOWN_TIMEOUT_SECONDS * MSEC_PER_SEC) / SHUTDOWN_SLEEP_INTERVAL_MS;

    while (ACLK_JOBS_ARE_RUNNING && loop_count > 0) {
        if (!uv_run(loop, UV_RUN_NOWAIT))
            break;  // No pending callbacks
        sleep_usec(SHUTDOWN_SLEEP_INTERVAL_MS * USEC_PER_MS);
        loop_count--;
    }

    (void) uv_loop_close(loop);

    // Free execute commands / queries
    free_query_list(aclk_query_execute->JudyL);
    (void)JudyLFreeArray(&aclk_query_execute->JudyL, PJE0);
    freez(aclk_query_execute);

    // Free batch commands
    if (aclk_query_batch) {
        free_query_list(aclk_query_batch->JudyL);
        (void)JudyLFreeArray(&aclk_query_batch->JudyL, PJE0);
        freez(aclk_query_batch);
    }

    release_cmd_pool(&config->cmd_pool);
    worker_unregister();
    service_exits();
    completion_mark_complete(&config->start_stop_complete);
    return NULL;
}

static void aclk_initialize_event_loop(void)
{
    memset(&aclk_sync_config, 0, sizeof(aclk_sync_config));
    completion_init(&aclk_sync_config.start_stop_complete);

    init_worker_pool(&aclk_sync_config.worker_pool);

    aclk_sync_config.thread = nd_thread_create("ACLKSYNC", NETDATA_THREAD_OPTION_DEFAULT, aclk_synchronization_event_loop, &aclk_sync_config);
    fatal_assert(NULL != aclk_sync_config.thread);

    completion_wait_for(&aclk_sync_config.start_stop_complete);
    // Keep completion, just reset it for next use during shutdown
    completion_reset(&aclk_sync_config.start_stop_complete);
}

// -------------------------------------------------------------

void create_aclk_config(RRDHOST *host, nd_uuid_t *host_uuid __maybe_unused, nd_uuid_t *node_id __maybe_unused)
{

    if (!host || host->aclk_host_config)
        return;

    struct aclk_sync_cfg_t *aclk_host_config = callocz(1, sizeof(struct aclk_sync_cfg_t));
    if (node_id && !uuid_is_null(*node_id))
        uuid_unparse_lower(*node_id, aclk_host_config->node_id);

    struct aclk_sync_cfg_t *expected = NULL;
    if (__atomic_compare_exchange_n(&host->aclk_host_config, &expected, aclk_host_config, false, __ATOMIC_RELAXED, __ATOMIC_RELAXED)) {
        if (node_id && UUIDiszero(host->node_id))
            uuid_copy(host->node_id.uuid, *node_id);
    }
    else {
        freez(aclk_host_config);
        return;
    }

    aclk_host_config->host = host;
    aclk_host_config->stream_alerts = false;
    time_t now = now_realtime_sec();
    aclk_host_config->node_info_send_time = (host == localhost || NULL == localhost) ? now - 25 : now;
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

void aclk_synchronization_init(void)
{
    char *err_msg = NULL;
    int rc;

    nd_log_daemon(NDLP_INFO, "Creating archived hosts");
    int number_of_children = 0;
    rc = sqlite3_exec_monitored(db_meta, SQL_FETCH_ALL_HOSTS, create_host_callback, &number_of_children, &err_msg);

    if (rc != SQLITE_OK) {
        nd_log_daemon(NDLP_ERR, "SQLite error when loading archived hosts, rc = %d (%s)", rc, err_msg);
        sqlite3_free(err_msg);
    }

    nd_log_daemon(NDLP_INFO, "Created %d archived hosts", number_of_children);
    // Trigger host context load for hosts that have been created
    if (unlikely(!metadata_queue_load_host_context())) {
        nd_log_daemon(NDLP_WARNING, "Failed to queue command to load contexts for archived hosts");
        // Reset context load flag so that contexts will be loaded on demand
        reset_host_context_load_flag();
    }

    rc = sqlite3_exec_monitored(db_meta, SQL_FETCH_ALL_INSTANCES, aclk_config_parameters, NULL, &err_msg);

    if (rc != SQLITE_OK) {
        nd_log_daemon(NDLP_ERR, "SQLite error when configuring host ACLK synchonization parameters, rc = %d (%s)", rc, err_msg);
        sqlite3_free(err_msg);
    }

    aclk_initialize_event_loop();

    if (!number_of_children)
        aclk_queue_node_info(localhost, true);

    nd_log_daemon(NDLP_INFO, "ACLK sync initialization completed");
}

static inline bool queue_aclk_sync_cmd(enum aclk_database_opcode opcode, const void *param0, const void *param1)
{
    cmd_data_t cmd;
    cmd.opcode = opcode;
    cmd.param[0] = (void *) param0;
    cmd.param[1] = (void *) param1;
    return aclk_database_enq_cmd(&cmd, true);
}

void aclk_synchronization_shutdown(void)
{
    if (!aclk_sync_config.thread)
        return;

    // Send shutdown command, note that the completion is initialized
    // on init and still valid
    aclk_mqtt_client_reset();

    if (queue_aclk_sync_cmd(ACLK_SYNC_SHUTDOWN, NULL, NULL))
        completion_wait_for(&aclk_sync_config.start_stop_complete);

    completion_destroy(&aclk_sync_config.start_stop_complete);
    int rc = nd_thread_join(aclk_sync_config.thread);
    if (rc)
        nd_log_daemon(NDLP_ERR, "ACLK: Failed to join synchronization thread");
    else
        nd_log_daemon(NDLP_INFO, "ACLK: synchronization thread shutdown completed");
}

// Public
void aclk_push_alert_config(const char *node_id, const char *config_hash)
{
    if (unlikely(!node_id || !config_hash))
        return;

    queue_aclk_sync_cmd(ACLK_DATABASE_PUSH_ALERT_CONFIG, strdupz(node_id), strdupz(config_hash));
}

void aclk_execute_query(aclk_query_t *query)
{
    if (unlikely(!query))
        return;

    (void) queue_aclk_sync_cmd(ACLK_QUERY_EXECUTE, query, NULL);
}

void aclk_add_job(aclk_query_t *query)
{
    if (unlikely(!query))
        return;

    (void) queue_aclk_sync_cmd(ACLK_QUERY_BATCH_ADD, query, NULL);
}

void aclk_mqtt_client_set(mqtt_wss_client client)
{
    (void) queue_aclk_sync_cmd(ACLK_MQTT_WSS_CLIENT_SET, client, NULL);
}

void aclk_mqtt_client_reset()
{
    if (!__atomic_load_n(&aclk_sync_config.client, __ATOMIC_RELAXED))
        return;

    struct completion compl;
    completion_init(&compl);
    if (queue_aclk_sync_cmd(ACLK_MQTT_WSS_CLIENT_RESET, &compl, NULL))
        completion_wait_for(&compl);
    completion_destroy(&compl);
}

void schedule_node_state_update(RRDHOST *host, uint64_t delay)
{
    if (unlikely(!host))
        return;

    (void) queue_aclk_sync_cmd(ACLK_DATABASE_NODE_STATE, host, (void *)(uintptr_t)delay);
}

void unregister_node(const char *machine_guid)
{
    if (unlikely(!machine_guid))
        return;

    (void) queue_aclk_sync_cmd(ACLK_DATABASE_NODE_UNREGISTER, strdupz(machine_guid), NULL);
}

void destroy_aclk_config(RRDHOST *host)
{
    if (!host)
        return;

    struct aclk_sync_cfg_t *aclk_host_config = __atomic_load_n(&host->aclk_host_config, __ATOMIC_RELAXED);
    if (!aclk_host_config)
        return;

    if(likely(__atomic_load_n(&aclk_sync_config.initialized, __ATOMIC_RELAXED))) {
        struct completion compl;
        completion_init(&compl);

        if (queue_aclk_sync_cmd(ACLK_CANCEL_NODE_UPDATE_TIMER, (void *)host, (void *)&compl))
            completion_wait_for(&compl);
        completion_destroy(&compl);
    }

    struct aclk_sync_cfg_t *old_aclk_host_config = __atomic_exchange_n(&host->aclk_host_config, NULL, __ATOMIC_RELAXED);
    freez(old_aclk_host_config);
}

void aclk_queue_node_info(RRDHOST *host, bool immediate)
{
    (void) queue_aclk_sync_cmd(ACLK_QUEUE_NODE_INFO, (void *)host, (void *)(uintptr_t)immediate);
}
