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
#include "aclk/aclk_capas.h"

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
    uv_thread_t thread;
    uv_loop_t loop;
    uv_timer_t timer_req;
    uv_async_t async;
    bool initialized;
    mqtt_wss_client client;
    int aclk_queries_running;
    bool alert_push_running;
    bool aclk_batch_job_is_running;
    SPINLOCK cmd_queue_lock;
    uint32_t aclk_jobs_pending;
    struct completion start_stop_complete;
    struct aclk_database_cmd *cmd_base;
    ARAL *ar;
} aclk_sync_config = { 0 };

static struct aclk_database_cmd aclk_database_deq_cmd(void)
{
    struct aclk_database_cmd ret = { 0 };
    struct aclk_database_cmd *to_free = NULL;

    spinlock_lock(&aclk_sync_config.cmd_queue_lock);
    if(aclk_sync_config.cmd_base) {
        struct aclk_database_cmd *t = aclk_sync_config.cmd_base;
        DOUBLE_LINKED_LIST_REMOVE_ITEM_UNSAFE(aclk_sync_config.cmd_base, t, prev, next);
        ret = *t;
        to_free = t;
    }
    else {
        ret.opcode = ACLK_DATABASE_NOOP;
    }
    spinlock_unlock(&aclk_sync_config.cmd_queue_lock);
    aral_freez(aclk_sync_config.ar, to_free);

    return ret;
}

static void aclk_database_enq_cmd(struct aclk_database_cmd *cmd)
{
    if(unlikely(!aclk_sync_config.initialized))
        return;

    struct aclk_database_cmd *t = aral_mallocz(aclk_sync_config.ar);
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

static void timer_cb(uv_timer_t *handle)
{
    uv_stop(handle->loop);
    uv_update_time(handle->loop);
    struct aclk_sync_config_s *config = handle->data;

    struct aclk_database_cmd cmd = { 0 };
    if (aclk_online_for_alerts()) {
        cmd.opcode = ACLK_DATABASE_PUSH_ALERT;
        aclk_database_enq_cmd(&cmd);
    }

    if (config->aclk_jobs_pending > 0) {
        cmd.opcode = ACLK_QUERY_BATCH_EXECUTE;
        aclk_database_enq_cmd(&cmd);
    }
}

struct aclk_query_payload {
    uv_work_t request;
    void *data;
    struct aclk_sync_config_s *config;
};

static void after_aclk_run_query_job(uv_work_t *req, int status __maybe_unused)
{
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

    bool ok_to_send = true;

    switch (query->type) {

// Incoming : cloud -> agent
        case HTTP_API_V2:
            worker_is_busy(UV_EVENT_ACLK_QUERY_EXECUTE);
            http_api_v2(config->client, query);
            ok_to_send = false;
            break;
        case CTX_CHECKPOINT:;
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

    if (ok_to_send)
        send_bin_msg(config->client, query);

    aclk_query_free(query);
}

static void aclk_run_query_job(uv_work_t *req)
{
    register_libuv_worker_jobs();

    struct aclk_query_payload *payload =  req->data;
    struct aclk_sync_config_s *config = payload->config;
    aclk_query_t query = (aclk_query_t) payload->data;

    aclk_run_query(config, query);
    worker_is_idle();
}

static void after_aclk_execute_batch(uv_work_t *req, int status __maybe_unused)
{
    struct aclk_query_payload *payload = req->data;
    struct aclk_sync_config_s *config = payload->config;
    config->aclk_batch_job_is_running = false;
    freez(payload);
}

static void aclk_execute_batch(uv_work_t *req)
{
    register_libuv_worker_jobs();

    struct aclk_query_payload *payload =  req->data;
    struct aclk_sync_config_s *config = payload->config;
    struct judy_list_t *aclk_query_batch = payload->data;

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

        aclk_query_t query = *Pvalue;
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

struct worker_data {
    uv_work_t request;
    void *payload;
    struct aclk_sync_config_s *config;
};

static void after_do_unregister_node(uv_work_t *req, int status __maybe_unused)
{
    struct worker_data *data = req->data;
    freez(data);
}

static void do_unregister_node(uv_work_t *req)
{
    register_libuv_worker_jobs();

    struct worker_data *data =  req->data;

    worker_is_busy(UV_EVENT_UNREGISTER_NODE);

    sql_unregister_node(data->payload);

    worker_is_idle();
}

static void node_update_timer_cb(uv_timer_t *handle)
{
    struct aclk_sync_cfg_t *ahc = handle->data;
    RRDHOST *host = ahc->host;

    if(!host || aclk_host_state_update_auto(host))
        uv_timer_stop(&ahc->timer);
}

static void close_callback(uv_handle_t *handle, void *data __maybe_unused)
{
    if (handle->type == UV_TIMER) {
        uv_timer_stop((uv_timer_t *)handle);
    }

    uv_close(handle, NULL);  // Automatically close and free the handle
}

static void after_start_alert_push(uv_work_t *req, int status __maybe_unused)
{
    struct worker_data *data = req->data;
    struct aclk_sync_config_s *config = data->config;

    config->alert_push_running = false;
    freez(data);
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

int schedule_query_in_worker(uv_loop_t *loop, struct aclk_sync_config_s *config, aclk_query_t query) {
    struct aclk_query_payload *payload = mallocz(sizeof(*payload));
    payload->request.data = payload;
    payload->config = config;
    payload->data = query;
    config->aclk_queries_running++;
    int rc = uv_queue_work(loop, &payload->request, aclk_run_query_job, after_aclk_run_query_job);
    if (rc) {
        config->aclk_queries_running--;
        freez(payload);
    }
    return rc;
}

static void free_query_list(Pvoid_t JudyL)
{
    bool first = true;
    Pvoid_t *Pvalue;
    Word_t Index = 0;
    aclk_query_t query;
    while ((Pvalue = JudyLFirstThenNext(JudyL, &Index, &first))) {
        if (!*Pvalue)
            continue;
        query = *Pvalue;
        aclk_query_free(query);
    }
}

#define MAX_SHUTDOWN_TIMEOUT_SECONDS (5)

#define ACLK_SYNC_SHOULD_BE_RUNNING                                                                                    \
    (!shutdown_requested || config->aclk_queries_running || config->alert_push_running ||                              \
     config->aclk_batch_job_is_running)

static void aclk_synchronization_event_loop(void *arg)
{
    struct aclk_sync_config_s *config = arg;
    uv_thread_set_name_np("ACLKSYNC");
    config->ar = aral_by_size_acquire(sizeof(struct aclk_database_cmd));
    worker_register("ACLKSYNC");

    service_register(SERVICE_THREAD_TYPE_EVENT_LOOP, NULL, NULL, NULL, true);

    worker_register_job_name(ACLK_DATABASE_NOOP,                "noop");
    worker_register_job_name(ACLK_DATABASE_NODE_STATE,          "node state");
    worker_register_job_name(ACLK_DATABASE_PUSH_ALERT,          "alert push");
    worker_register_job_name(ACLK_DATABASE_PUSH_ALERT_CONFIG,   "alert conf push");
    worker_register_job_name(ACLK_QUERY_EXECUTE_SYNC,           "aclk query execute sync");
    worker_register_job_name(ACLK_QUERY_BATCH_EXECUTE,          "aclk batch execute");
    worker_register_job_name(ACLK_QUERY_BATCH_ADD,              "aclk batch add");
    worker_register_job_name(ACLK_MQTT_WSS_CLIENT,              "config mqtt client");
    worker_register_job_name(ACLK_DATABASE_NODE_UNREGISTER,     "unregister node");

    uv_loop_t *loop = &config->loop;
    fatal_assert(0 == uv_loop_init(loop));
    fatal_assert(0 == uv_async_init(loop, &config->async, async_cb));

    fatal_assert(0 == uv_timer_init(loop, &config->timer_req));
    config->timer_req.data = config;
    fatal_assert(0 == uv_timer_start(&config->timer_req, timer_cb, TIMER_PERIOD_MS, TIMER_PERIOD_MS));

    netdata_log_info("Starting ACLK synchronization thread");

    config->initialized = true;

    sql_delete_aclk_table_list();

    int query_thread_count = netdata_conf_cloud_query_threads();
    netdata_log_info("Starting ACLK synchronization thread with %d parallel query threads", query_thread_count);

    struct worker_data *data;
    aclk_query_t query;

    // This holds queries that need to be executed one by one
    struct judy_list_t *aclk_query_batch = NULL;
    // This holds queries that can be dispatched in parallel in ACLK QUERY worker threads
    struct judy_list_t *aclk_query_execute = callocz(1, sizeof(*aclk_query_execute));
    size_t pending_queries = 0;

    Pvoid_t *Pvalue;
    struct aclk_query_payload *payload;

    unsigned cmd_batch_size;

    completion_mark_complete(&config->start_stop_complete);
    int shutdown_requested = 0;
    time_t shutdown_initiated = 0;

    while (likely(ACLK_SYNC_SHOULD_BE_RUNNING)) {
        enum aclk_database_opcode opcode;
        worker_is_idle();
        uv_run(loop, UV_RUN_DEFAULT);

        if (unlikely(shutdown_requested)) {
            nd_log_limit_static_thread_var(erl, 1, 0);
            nd_log_limit(&erl, NDLS_DAEMON, NDLP_INFO, "ACLKSYNC: Waiting for pending queries to finish before shutdown");
            if (now_realtime_sec() - shutdown_initiated > MAX_SHUTDOWN_TIMEOUT_SECONDS) {
                nd_log_daemon(NDLP_INFO, "ACLKSYNC: Shutdown timeout, forcing exit");
                break;
            }
            continue;
        }

        /* wait for commands */
        cmd_batch_size = 0;
        do {
            if (unlikely(++cmd_batch_size >= MAX_BATCH_SIZE))
                break;

            struct aclk_database_cmd cmd = aclk_database_deq_cmd();
            opcode = cmd.opcode;

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
                case ACLK_DATABASE_NODE_STATE:;
                    RRDHOST *host = cmd.param[0];
                    struct aclk_sync_cfg_t *ahc = host->aclk_config;
                    if (unlikely(!ahc)) {
                        create_aclk_config(host, &host->host_id.uuid, &host->node_id.uuid);
                        ahc = host->aclk_config;
                    }

                    if (ahc) {
                        uint64_t schedule_time = (uint64_t)(uintptr_t)cmd.param[1];
                        if (!ahc->timer_initialized) {
                            int rc = uv_timer_init(loop, &ahc->timer);
                            if (!rc) {
                                ahc->timer_initialized = true;
                                ahc->timer.data = ahc;
                            }
                        }

                        if (ahc->timer_initialized) {
                            if (uv_is_active((uv_handle_t *)&ahc->timer))
                                uv_timer_stop(&ahc->timer);

                            ahc->timer.data = ahc;
                            int rc = uv_timer_start(&ahc->timer, node_update_timer_cb, schedule_time, 5000);
                            if (!rc)
                                break; // Timer started, exit
                        }
                    }

                    // This is fallback if timer fails
                    aclk_host_state_update_auto(host);
                    break;

                case ACLK_DATABASE_NODE_UNREGISTER:
                    data = mallocz(sizeof(*data));
                    data->request.data = data;
                    data->config = config;
                    data->payload = cmd.param[0];

                    if (uv_queue_work(loop, &data->request, do_unregister_node, after_do_unregister_node)) {
                        freez(data->payload);
                        freez(data);
                    }
                    break;
                    // ALERTS
                case ACLK_DATABASE_PUSH_ALERT_CONFIG:
                    aclk_push_alert_config_event(cmd.param[0], cmd.param[1]);
                    break;
                case ACLK_DATABASE_PUSH_ALERT:

                    if (config->alert_push_running)
                        break;

                    config->alert_push_running = true;

                    data = mallocz(sizeof(*data));
                    data->request.data = data;
                    data->config = config;

                    if (uv_queue_work(loop, &data->request, start_alert_push, after_start_alert_push)) {
                        freez(data);
                        config->alert_push_running = false;
                    }
                    break;
                case ACLK_MQTT_WSS_CLIENT:
                    config->client = (mqtt_wss_client)cmd.param[0];
                    break;

                case ACLK_QUERY_EXECUTE:
                    query = (aclk_query_t)cmd.param[0];

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
                        aclk_query_t query_in_queue = *Pvalue;

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
                    query = (aclk_query_t)cmd.param[0];
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

                    payload = mallocz(sizeof(*payload));
                    payload->request.data = payload;
                    payload->config = config;
                    payload->data = aclk_query_batch;

                    config->aclk_batch_job_is_running = true;
                    config->aclk_jobs_pending -= aclk_query_batch->count;
                    aclk_query_batch = NULL;

                    if (uv_queue_work(loop, &payload->request, aclk_execute_batch, after_aclk_execute_batch)) {
                        aclk_query_batch = payload->data;
                        config->aclk_jobs_pending += aclk_query_batch->count;
                        freez(payload);
                        config->aclk_batch_job_is_running = false;
                    }
                    break;
                case ACLK_SYNC_SHUTDOWN:
                    shutdown_requested = 1;
                    shutdown_initiated = now_realtime_sec();
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
    uv_run(loop, UV_RUN_NOWAIT);

    uv_walk(loop, (uv_walk_cb) close_callback, NULL);
    uv_run(loop, UV_RUN_NOWAIT);

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

    aral_by_size_release(config->ar);
    completion_mark_complete(&config->start_stop_complete);

    worker_unregister();
    service_exits();
    netdata_log_info("ACLK SYNC: Shutting down ACLK synchronization event loop");
}

static void aclk_initialize_event_loop(void)
{
    memset(&aclk_sync_config, 0, sizeof(aclk_sync_config));
    completion_init(&aclk_sync_config.start_stop_complete);

    int retries = 0;
    int create_uv_thread_rc = create_uv_thread(&aclk_sync_config.thread, aclk_synchronization_event_loop, &aclk_sync_config, &retries);
    if (create_uv_thread_rc)
        nd_log_daemon(NDLP_ERR, "Failed to create ACLK synchronization thread, error %s, after %d retries", uv_err_name(create_uv_thread_rc), retries);

    fatal_assert(0 == create_uv_thread_rc);

    if (retries)
        nd_log_daemon(NDLP_WARNING, "ACLK synchronization thread was created after %d attempts", retries);
    completion_wait_for(&aclk_sync_config.start_stop_complete);

    // Keep completion, just reset it for next use during shutdown
    completion_reset(&aclk_sync_config.start_stop_complete);
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

void destroy_aclk_config(RRDHOST *host)
{
    if (!host || !host->aclk_config)
        return;

    freez(host->aclk_config);
    host->aclk_config = NULL;
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
    metadata_queue_load_host_context();

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

static inline void queue_aclk_sync_cmd(enum aclk_database_opcode opcode, const void *param0, const void *param1)
{
    struct aclk_database_cmd cmd;
    cmd.opcode = opcode;
    cmd.param[0] = (void *) param0;
    cmd.param[1] = (void *) param1;
    aclk_database_enq_cmd(&cmd);
}

void aclk_synchronization_shutdown(void)
{
    // Send shutdown command, not that the completion is initialized
    // on init and still valid
    queue_aclk_sync_cmd(ACLK_SYNC_SHUTDOWN, NULL, NULL);

    completion_wait_for(&aclk_sync_config.start_stop_complete);
    completion_destroy(&aclk_sync_config.start_stop_complete);
    int rc = uv_thread_join(&aclk_sync_config.thread);
    if (rc)
        nd_log_daemon(NDLP_ERR, "ACLK: Failed to join synchronization thread, error %s", uv_err_name(rc));
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

void aclk_execute_query(aclk_query_t query)
{
    if (unlikely(!query))
        return;

    queue_aclk_sync_cmd(ACLK_QUERY_EXECUTE, query, NULL);
}

void aclk_add_job(aclk_query_t query)
{
    if (unlikely(!query))
        return;

    queue_aclk_sync_cmd(ACLK_QUERY_BATCH_ADD, query, NULL);
}

void aclk_query_init(mqtt_wss_client client)
{
    queue_aclk_sync_cmd(ACLK_MQTT_WSS_CLIENT, client, NULL);
}

void schedule_node_state_update(RRDHOST *host, uint64_t delay)
{
    if (unlikely(!host))
        return;

    queue_aclk_sync_cmd(ACLK_DATABASE_NODE_STATE, host, (void *)(uintptr_t)delay);
}

void unregister_node(const char *machine_guid)
{
    if (unlikely(!machine_guid))
        return;
    queue_aclk_sync_cmd(ACLK_DATABASE_NODE_UNREGISTER, strdupz(machine_guid), NULL);
}
