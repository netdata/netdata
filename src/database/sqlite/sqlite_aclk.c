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

// Lifecycle of the ACLK sync event loop, as seen by a thread that is NOT the loop.
//
// It exists because "the loop is not accepting commands" and "the loop owns no handles of mine" are
// different questions, and destroy_aclk_config() needs the second one. The per-host node-update
// uv_timer_t is registered by the loop and lives inside the host's aclk config, so a teardown that
// frees the config while the loop still holds that handle corrupts the loop.
typedef enum {
    ACLK_SYNC_NEVER_STARTED = 0,  // zero value: no loop, so no host timer was ever registered
    ACLK_SYNC_RUNNING,            // accepting and consuming commands
    ACLK_SYNC_CLOSING,            // refusing commands, host timers may still be registered
    ACLK_SYNC_HANDLES_CLOSED,     // uv_walk() done: no host timer is registered any more
} ACLK_SYNC_PHASE;

struct aclk_sync_config_s {
    ND_THREAD *thread;
    pid_t event_loop_tid; // atomic: the event loop publishes it, other threads compare against it
    uv_loop_t loop;
    uv_timer_t timer_req;
    uv_async_t async;
    ACLK_SYNC_PHASE phase;            // atomic
    struct completion handles_closed; // released when phase reaches ACLK_SYNC_HANDLES_CLOSED
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
    if(unlikely(__atomic_load_n(&aclk_sync_config.phase, __ATOMIC_ACQUIRE) != ACLK_SYNC_RUNNING))
        return false;

    // The notify is part of the enqueue, not an afterthought: uv_async_send() touches a handle the
    // loop closes on its way out, so being counted as a producer only for the duration of push_cmd()
    // would let the loop finish closing that handle while we are still between the two calls. The
    // admission ref holds the pool - and with it the loop's teardown - open across both.
    if (!cmd_pool_producer_enter(&aclk_sync_config.cmd_pool))
        return false;

    bool added = push_cmd(&aclk_sync_config.cmd_pool, (void *)cmd, wait_on_full);
    if (added)
        (void) uv_async_send(&aclk_sync_config.async);

    cmd_pool_producer_leave(&aclk_sync_config.cmd_pool);
    return added;
}

struct children {
    int vnodes;
    int normal;
};

// Column indices for SQL_FETCH_ALL_HOSTS — keep in lock-step with the SELECT list.
enum {
    COL_FETCH_HOST_ID = 0,
    COL_FETCH_HOSTNAME,
    COL_FETCH_REGISTRY,
    COL_FETCH_UPDATE_EVERY,
    COL_FETCH_OS,
    COL_FETCH_TIMEZONE,
    COL_FETCH_HOPS,
    COL_FETCH_ABBREV_TIMEZONE,
    COL_FETCH_UTC_OFFSET,
    COL_FETCH_PROGRAM_NAME,
    COL_FETCH_PROGRAM_VERSION,
    COL_FETCH_ENTRIES,
    COL_FETCH_LAST_CONNECTED,
    COL_FETCH_IS_EPHEMERAL,
    COL_FETCH_IS_REGISTERED,
};

// Materialise one archived host row from SQL_FETCH_ALL_HOSTS into rrdhost_root_index.
// Returns the host (or NULL if creation skipped/failed) so the caller can update counters.
static RRDHOST *load_archived_host_from_row(sqlite3_stmt *res)
{
    // The COL_FETCH_* enum is in lock-step with SQL_FETCH_ALL_HOSTS' SELECT list.
    // Catch drift early in debug builds; release builds compile this out.
    internal_fatal(sqlite3_column_count(res) != COL_FETCH_IS_REGISTERED + 1,
                   "SQL_FETCH_ALL_HOSTS column count (%d) does not match COL_FETCH_* enum (%d)",
                   sqlite3_column_count(res), COL_FETCH_IS_REGISTERED + 1);

    nd_uuid_t host_uuid;
    if (!sqlite3_column_uuid_copy(res, COL_FETCH_HOST_ID, host_uuid)) {
        nd_log_daemon(
            NDLP_ERR,
            "Skipping archived host: host_id column is not a valid 16-byte UUID blob (type=%d, bytes=%d). Possible DB corruption.",
            sqlite3_column_type(res, COL_FETCH_HOST_ID),
            sqlite3_column_bytes(res, COL_FETCH_HOST_ID));
        return NULL;
    }

    char guid[UUID_STR_LEN];
    uuid_unparse_lower(host_uuid, guid);

    const char *hostname     = (const char *)sqlite3_column_text(res, COL_FETCH_HOSTNAME);
    const char *registry     = (const char *)sqlite3_column_text(res, COL_FETCH_REGISTRY);
    const char *os           = (const char *)sqlite3_column_text(res, COL_FETCH_OS);
    const char *host_tz      = (const char *)sqlite3_column_text(res, COL_FETCH_TIMEZONE);
    const char *abbrev_tz    = (const char *)sqlite3_column_text(res, COL_FETCH_ABBREV_TIMEZONE);
    const char *prog_name    = (const char *)sqlite3_column_text(res, COL_FETCH_PROGRAM_NAME);
    const char *prog_version = (const char *)sqlite3_column_text(res, COL_FETCH_PROGRAM_VERSION);
    int hops          = sqlite3_column_int(res, COL_FETCH_HOPS);
    int utc_offset    = sqlite3_column_int(res, COL_FETCH_UTC_OFFSET);
    int entries       = sqlite3_column_int(res, COL_FETCH_ENTRIES);
    // update_every defaults to 1 only when the column is SQL NULL — preserves
    // the pre-refactor `argv[i] ? str2i(argv[i]) : 1` fallback exactly. A
    // stored 0 stays 0 (matches the original str2i path).
    int update_every = (sqlite3_column_type(res, COL_FETCH_UPDATE_EVERY) == SQLITE_NULL)
        ? 1
        : sqlite3_column_int(res, COL_FETCH_UPDATE_EVERY);
    int64_t last_connected_db = sqlite3_column_int64(res, COL_FETCH_LAST_CONNECTED);
    int is_ephemeral  = sqlite3_column_int(res, COL_FETCH_IS_EPHEMERAL);
    int is_registered = sqlite3_column_int(res, COL_FETCH_IS_REGISTERED);

    time_t last_connected = (time_t)last_connected_db;
    if (!last_connected)
        last_connected = now_realtime_sec();

    time_t age = now_realtime_sec() - last_connected;

    if (is_ephemeral && ((!is_registered && last_connected == 1) ||
                         (rrdhost_free_ephemeral_time_s && age > rrdhost_free_ephemeral_time_s))) {
        netdata_log_info(
            "%s ephemeral hostname \"%s\" with GUID \"%s\", age = %" PRId64 " seconds (limit %" PRId64 " seconds)",
            is_registered ? "Loading registered" : "Skipping unregistered",
            hostname,
            guid,
            (int64_t)age,
            (int64_t)rrdhost_free_ephemeral_time_s);

        if (!is_registered)
           return NULL;
    }

    struct rrdhost_system_info *system_info = rrdhost_system_info_create();
    rrdhost_system_info_hops_set(system_info, (int16_t)hops);
    sql_build_host_system_info(&host_uuid, system_info);

    RRDHOST *host = rrdhost_find_or_create(
        hostname,
        registry,
        guid,
        os,
        host_tz,
        abbrev_tz,
        (int32_t)utc_offset,
        prog_name    ? prog_name    : "unknown",
        prog_version ? prog_version : "unknown",
        update_every,
        entries,
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

    if (unlikely(!host))
        return NULL;

    if (is_ephemeral) {
        rrdhost_option_set(host, RRDHOST_OPTION_EPHEMERAL_HOST);
        host->stream.rcv.status.last_disconnected = now_realtime_sec();
    }

    host->rrdlabels = sql_load_host_labels(&host_uuid);
    host->stream.snd.status.last_connected = last_connected;

    pulse_host_status(host, 0, 0); // this will detect the receiver status

#ifdef NETDATA_INTERNAL_CHECKS
    char node_str[UUID_STR_LEN] = "<none>";
    if (likely(!UUIDiszero(host->node_id)))
        uuid_unparse_lower(host->node_id.uuid, node_str);
    internal_error(true, "Adding archived host \"%s\" with GUID \"%s\" node id = \"%s\"  ephemeral=%d",
                   rrdhost_hostname(host), host->machine_guid, node_str, is_ephemeral);
#endif

    return host;
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

struct judy_list_t {
    Pvoid_t JudyL;
    Word_t count;
};

static void async_cb(uv_async_t *handle __maybe_unused)
{
    ;
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
        aclk_query_free(query);
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
            // the cloud asks for the node instances after connecting, so this is also where the
            // manifest gets its one request per ACLK session - see aclk_arm_node_manifest_all_hosts()
            aclk_arm_node_manifest_all_hosts();
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
        case UPDATE_NODE_MANIFEST:
            worker_is_busy(UV_EVENT_UPDATE_NODE_MANIFEST);
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
        if (client) {
            bool sent = (send_bin_msg(client, query) == 0);

            // aclk_query_free() reports the outcome to whoever tracks what the cloud has been told;
            // leaving it unset here is what makes a dropped message recoverable. Only the manifest
            // carries a publication record, so it is the only type with an outcome to record here.
            if (query->type == UPDATE_NODE_MANIFEST)
                query->manifest.published = sent;
        }
        else
            nd_log_daemon(NDLP_ERR, "No client to send message %u", query->type);
    }

    aclk_query_free(query);
}

static void aclk_run_query_job(uv_work_t *req)
{
    register_libuv_worker_jobs();

    worker_data_t *worker = req->data;
    struct aclk_sync_config_s *config = worker->config;
    aclk_query_t *query = (aclk_query_t *)worker->payload;

    // aclk_run_query() frees the query; if we're shutting down we must still free it here
    if (unlikely(__atomic_load_n(&config->shutdown_requested, __ATOMIC_RELAXED)))
        aclk_query_free(query);
    else
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

    Word_t Index = 0;
    bool first = true;
    Pvoid_t *Pvalue;
    while ((Pvalue = JudyLFirstThenNext(aclk_query_batch->JudyL, &Index, &first))) {
        if (!*Pvalue)
            continue;

        aclk_query_t *query = *Pvalue;
        // Shutdown may be requested while this batch is already running, so
        // re-check before each query instead of relying on a stale snapshot.
        if (unlikely(__atomic_load_n(&config->shutdown_requested, __ATOMIC_RELAXED)))
            aclk_query_free(query);
        else
            aclk_run_query(config, query);
    }

    (void) JudyLFreeArray(&aclk_query_batch->JudyL, PJE0);
    freez(aclk_query_batch);

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

// Retires a host's node-update timer and answers `compl` once the handle has actually left libuv.
//
// Called from the live ACLK_CANCEL_NODE_UPDATE_TIMER handler and from the shutdown drain, which MUST
// behave identically: destroy_aclk_config() frees the config - and with it the embedded uv_timer_t -
// the moment its completion fires, so the completion may only be marked from the close callback,
// never inline while the handle is still registered on the loop.
//
// Runs on the ACLK sync event loop thread only.
static void aclk_cancel_node_update_timer(struct aclk_sync_cfg_t *cfg, void *payload, struct completion *compl)
{
    if (!cfg || !cfg->timer_initialized) {
        // nothing registered on the loop: the waiter can go
        if (compl)
            completion_mark_complete(compl);
        return;
    }

    if (uv_is_active((uv_handle_t *)&cfg->timer))
        uv_timer_stop(&cfg->timer);

    cfg->timer_initialized = false;

    struct notify_timer_cb_data *timer_cb_data = mallocz(sizeof(*timer_cb_data));
    timer_cb_data->payload = payload;
    timer_cb_data->completion = compl;
    cfg->timer.data = timer_cb_data;
    uv_close((uv_handle_t *)&cfg->timer, notify_timer_close_callback);
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
static void start_alert_push(uv_work_t *req)
{
    register_libuv_worker_jobs();

    struct worker_data *worker = req->data;
    struct aclk_sync_config_s *config = worker->config;

    if (unlikely(__atomic_load_n(&config->shutdown_requested, __ATOMIC_RELAXED)))
        return;

    worker_is_busy(UV_EVENT_ACLK_NODE_INFO);
    aclk_check_node_info_collectors_and_manifest();
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

#define SHUTDOWN_SLEEP_INTERVAL_MS (100)
#define ACLK_SHUTDOWN_WATCHDOG_TIMEOUT_SECONDS (15)
#define CMD_POOL_SIZE (2048)

// uv_close() completes in the loop pass after it is requested, so this only ever needs a couple of
// iterations. The cap only bounds an impossible state - see the fatal() at its only use.
#define ACLK_HANDLE_CLOSE_MAX_PASSES (100)

static void aclk_count_open_handle(uv_handle_t *handle __maybe_unused, void *data)
{
    (*(size_t *)data)++;
}

// Whether any handle is still registered on the loop - including one that is mid-close, since
// uv_walk() keeps visiting it until its close callback has run.
static bool aclk_loop_has_open_handles(uv_loop_t *loop)
{
    size_t open_handles = 0;
    uv_walk(loop, aclk_count_open_handle, &open_handles);
    return open_handles > 0;
}

#define ACLK_JOBS_ARE_RUNNING                                                                                          \
    (config->aclk_queries_running || config->alert_push_running || config->aclk_batch_job_is_running)

static void aclk_synchronization_event_loop(void *arg)
{
    struct aclk_sync_config_s *config = arg;
    uv_thread_set_name_np("ACLKSYNC");

    // published so code that can run on either this thread or a worker can tell which it is on -
    // this thread must never block on a lock a host teardown may be holding while it waits here
    // (destroy_aclk_config()). See aclk_sync_on_event_loop_thread().
    __atomic_store_n(&config->event_loop_tid, gettid_cached(), __ATOMIC_RELEASE);

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

    // This holds queries that need to be executed one by one
    struct judy_list_t *aclk_query_batch = NULL;

    // This holds queries that can be dispatched in parallel in ACLK QUERY worker threads
    struct judy_list_t *aclk_query_execute = callocz(1, sizeof(*aclk_query_execute));
    size_t pending_queries = 0;

    Pvoid_t *Pvalue;
    worker_data_t  *worker;

    __atomic_store_n(&config->shutdown_requested, false, __ATOMIC_RELAXED);
    __atomic_store_n(&config->phase, ACLK_SYNC_RUNNING, __ATOMIC_RELEASE);
    completion_mark_complete(&config->start_stop_complete);

    while (likely(!__atomic_load_n(&config->shutdown_requested, __ATOMIC_RELAXED)))  {
        enum aclk_database_opcode opcode;
        RRDHOST *host;
        struct aclk_sync_cfg_t *aclk_host_config;
        aclk_query_t *query;
        worker_is_idle();
        uv_run(loop, UV_RUN_ONCE);

        do {
            cmd_data_t cmd;

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

            // pending_queries is an accounting alias for the number of queries held in
            // aclk_query_execute->JudyL. Drift in EITHER direction is harmful:
            //  - too high: the NOOP -> ACLK_QUERY_EXECUTE rewrite below fires every iteration
            //    with nothing to drain and the inner UV_RUN_NOWAIT loop never blocks -> a
            //    silent one-core 100% CPU spin that cannot self-recover;
            //  - too low (e.g. 0 while the queue still holds work): the rewrite never fires and
            //    those queries stall until an unrelated enqueue happens to nudge execution.
            // Reconcile the alias against the Judy array (the source of truth) on every idle
            // pass so drift heals immediately instead of becoming a lockup or a stalled queue.
            // The field trigger is unproven (suspected Judy/memory corruption on some
            // runtimes); log it if seen.
            if (opcode == ACLK_DATABASE_NOOP) {
                size_t queued = (size_t)JudyLCount(aclk_query_execute->JudyL, 0, -1, PJE0);
                if (unlikely(queued != pending_queries)) {
                    nd_log_limit_static_global_var(erl, 1, 0);
                    nd_log_limit(&erl, NDLS_DAEMON, NDLP_WARNING,
                                 "ACLK: pending query counter (%zu) disagrees with the queued-query count (%zu); "
                                 "reconciling to prevent a CPU spin or a stalled queue (possible memory corruption)",
                                 pending_queries, queued);
                    pending_queries = queued;
                }
            }

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
                    aclk_host_config = __atomic_load_n(&host->aclk_host_config, __ATOMIC_ACQUIRE);
                    if (unlikely(!aclk_host_config)) {
                        create_aclk_config(host, &host->host_id.uuid, &host->node_id.uuid);
                        aclk_host_config = __atomic_load_n(&host->aclk_host_config, __ATOMIC_ACQUIRE);
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
                    aclk_host_config = __atomic_load_n(&host->aclk_host_config, __ATOMIC_ACQUIRE);
                    if (unlikely(!aclk_host_config)) {
                        create_aclk_config(host, &host->host_id.uuid, &host->node_id.uuid);
                        aclk_host_config = __atomic_load_n(&host->aclk_host_config, __ATOMIC_ACQUIRE);
                    }
                    aclk_send_timestamp_set(
                        &aclk_host_config->node_info_send_time,
                        (host == localhost || immediate) ? 1 : now_realtime_sec());
                    // the manifest is re-armed by build_node_info() itself, so every path that
                    // sends node info re-arms it, not only this opcode
                    break;
                case ACLK_CANCEL_NODE_UPDATE_TIMER:
                    host = cmd.param[0];
                    struct completion *compl = cmd.param[1];
                    aclk_host_config = __atomic_load_n(&host->aclk_host_config, __ATOMIC_ACQUIRE);
                    aclk_cancel_node_update_timer(aclk_host_config, host, compl);
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
                        } else {
                            nd_log_daemon(NDLP_ERR, "Failed to add ACLK command to the pending commands Judy");
                            aclk_query_free(query);
                        }
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
                        else {
                            nd_log_daemon(NDLP_ERR, "Failed to add ACLK command to the pending commands Judy");
                            aclk_query_free(query);
                        }
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
                    if (Pvalue != PJERR)
                        *Pvalue = query;
                    else {
                        aclk_query_free(query);
                        aclk_query_batch->count--;
                        
                        // Clean up the batch structure if this was the first entry that failed
                        if (aclk_query_batch->count == 0) {
                            freez(aclk_query_batch);
                            aclk_query_batch = NULL;
                        }
                        break;
                    }

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
                    __atomic_store_n(&config->shutdown_requested, true, __ATOMIC_RELAXED);
                    mark_pending_req_cancel_all();
                    break;
                default:
                    break;
            }

            if (opcode != ACLK_DATABASE_NOOP)
                uv_run(loop, UV_RUN_NOWAIT);

        } while (opcode != ACLK_DATABASE_NOOP);
    }

    // Stop accepting commands, then answer what is still queued. A waiter that got its command in
    // before the close is entitled to its completion - abandoning the queue here is what used to
    // leave a host teardown blocked forever in destroy_aclk_config(). close_cmd_pool() makes the
    // refusal and the insertion mutually exclusive, so after this point a producer either has a
    // command we are about to drain, or was told no and is not waiting.
    //
    // CLOSING is published first, before the close, so a producer that IS refused can tell which
    // refusal it got: from here it means "the loop may still own this host's timer", which
    // destroy_aclk_config() has to wait out. It MUST NOT be read as "there is nothing of mine on the
    // loop" - that assumption is what let a teardown free a config with its uv_timer_t registered.
    __atomic_store_n(&config->phase, ACLK_SYNC_CLOSING, __ATOMIC_RELEASE);
    close_cmd_pool(&config->cmd_pool);

    {
        cmd_data_t cmd;
        size_t abandoned = 0;
        while (pop_cmd(&config->cmd_pool, &cmd)) {
            abandoned++;
            // Every command whose producer waits on us MUST be completed here, or that producer
            // blocks forever. Note the completion is in a different parameter for each.
            switch (cmd.opcode) {
                case ACLK_CANCEL_NODE_UPDATE_TIMER: {
                    // destroy_aclk_config() is waiting, and it frees the host's config the moment we
                    // complete it. Same helper as the live handler, so the completion is marked from
                    // the close callback during the uv_run() below and the waiter only proceeds once
                    // the embedded handle is off the loop.
                    RRDHOST *dying = cmd.param[0];
                    struct completion *compl = cmd.param[1];
                    struct aclk_sync_cfg_t *cfg =
                        dying ? __atomic_load_n(&dying->aclk_host_config, __ATOMIC_ACQUIRE) : NULL;

                    aclk_cancel_node_update_timer(cfg, dying, compl);
                    break;
                }

                case ACLK_MQTT_WSS_CLIENT_RESET:
                    // aclk_mqtt_client_reset(): same state change as the live handler, in the same
                    // order - the client is cleared before the waiter is released, so a waiter that
                    // observes its completion never observes the stale client.
                    __atomic_store_n(&config->client, NULL, __ATOMIC_RELEASE);
                    if (cmd.param[0])
                        completion_mark_complete((struct completion *)cmd.param[0]);
                    break;

                // The rest have no waiter, but they do own their payload: the live handlers consume
                // it, so dropping the command here without the matching release leaks it. For a
                // query that is worse than a leak - aclk_query_free() is also what signals
                // query->sync_completion and returns the pooled slot, so skipping it strands a
                // send_node_info_with_wait() caller for its full timeout.
                case ACLK_QUERY_EXECUTE:
                case ACLK_QUERY_BATCH_ADD:
                    if (cmd.param[0])
                        aclk_query_free((aclk_query_t *)cmd.param[0]);
                    break;

                case ACLK_DATABASE_PUSH_ALERT_CONFIG:
                    freez(cmd.param[0]);
                    freez(cmd.param[1]);
                    break;

                case ACLK_DATABASE_NODE_UNREGISTER:
                    freez(cmd.param[0]);
                    break;

                default:
                    break;
            }
        }

        if (abandoned)
            nd_log_daemon(NDLP_INFO, "ACLK: sync event loop stopped with %zu queued commands", abandoned);
    }

    if (!uv_timer_stop(&config->timer_req))
        uv_close((uv_handle_t *)&config->timer_req, NULL);

    uv_close((uv_handle_t *)&config->async, NULL);
    uv_walk(loop, libuv_close_callback, NULL);

    // Run the loop until those closes have actually completed, then release every teardown that is
    // blocked waiting to hear it. This MUST happen before the outstanding-jobs wait below: a caller
    // parked in destroy_aclk_config() may be holding rrd_wrlock(), and worker jobs - which are what
    // that wait is for - do not own host timers, so making the barrier wait for them would stall the
    // whole agent behind a query for up to the watchdog timeout for no lifetime benefit.
    //
    // The wait for the walk to empty MUST NOT give up early and publish the barrier anyway: the
    // waiters it releases free configs with an embedded uv_timer_t, so releasing them while libuv
    // still owns a handle is exactly the corruption this barrier exists to prevent - a timeout would
    // trade a hang for a use-after-free. The cap therefore ends the process rather than the wait, and
    // is not reachable in practice: uv_walk() above requested a close on every handle, work requests
    // are not handles, and one uv_run() pass drains the whole closing queue.
    size_t handle_close_passes = 0;
    while (aclk_loop_has_open_handles(loop)) {
        if (unlikely(++handle_close_passes > ACLK_HANDLE_CLOSE_MAX_PASSES))
            fatal("ACLK: libuv handles still open after %zu close passes", handle_close_passes - 1);

        (void)uv_run(loop, UV_RUN_NOWAIT);
    }

    __atomic_store_n(&config->phase, ACLK_SYNC_HANDLES_CLOSED, __ATOMIC_RELEASE);
    completion_mark_complete(&config->handles_closed);

    size_t shutdown_wait_iterations = 0;
    const size_t log_every_iterations = (10 * MSEC_PER_SEC) / SHUTDOWN_SLEEP_INTERVAL_MS;
    const size_t watchdog_iterations = (ACLK_SHUTDOWN_WATCHDOG_TIMEOUT_SECONDS * MSEC_PER_SEC) / SHUTDOWN_SLEEP_INTERVAL_MS;

    while (ACLK_JOBS_ARE_RUNNING || uv_loop_alive(loop)) {
        (void)uv_run(loop, UV_RUN_NOWAIT);

        shutdown_wait_iterations++;

        if (shutdown_wait_iterations >= watchdog_iterations) {
            nd_log_daemon(
                NDLP_ERR,
                "ACLK: shutdown watchdog timeout (%d seconds) exceeded, abandoning outstanding libuv jobs "
                "(queries_running=%d, alert_push_running=%d, batch_job_running=%d)",
                ACLK_SHUTDOWN_WATCHDOG_TIMEOUT_SECONDS,
                config->aclk_queries_running,
                config->alert_push_running,
                config->aclk_batch_job_is_running);

            // Same reasoning as the metadata loop: these jobs run on libuv threadpool
            // threads that nothing joins, and they use db_meta. Suppress the SQLite
            // teardown so their handles and statements outlive this shutdown.
            sqlite_mark_teardown_unsafe(
                "an ACLK libuv job outlived the shutdown watchdog and may still be using the databases");
            break;
        }

        if ((shutdown_wait_iterations % log_every_iterations) == 0) {
            nd_log_daemon(
                NDLP_WARNING,
                "ACLK: waiting for outstanding libuv jobs during shutdown "
                "(queries_running=%d, alert_push_running=%d, batch_job_running=%d)",
                config->aclk_queries_running,
                config->alert_push_running,
                config->aclk_batch_job_is_running);
        }

        sleep_usec(SHUTDOWN_SLEEP_INTERVAL_MS * USEC_PER_MS);
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
}

static void aclk_initialize_event_loop(void)
{
    memset(&aclk_sync_config, 0, sizeof(aclk_sync_config));
    completion_init(&aclk_sync_config.start_stop_complete);

    // One-shot barrier, many waiters: every host teardown refused during shutdown parks here until
    // the loop has closed its handles. Initialized before the thread exists, so a waiter can never
    // reach it uninitialized.
    completion_init(&aclk_sync_config.handles_closed);

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

    if (!host || __atomic_load_n(&host->aclk_host_config, __ATOMIC_ACQUIRE))
        return;

    struct aclk_sync_cfg_t *aclk_host_config = callocz(1, sizeof(struct aclk_sync_cfg_t));
    spinlock_init(&aclk_host_config->pending_ctx_spinlock);
    spinlock_init(&aclk_host_config->node_id_spinlock);
    if (node_id && !uuid_is_null(*node_id))
        aclk_node_id_set(aclk_host_config, *node_id);

    // Initialize every field BEFORE publishing the pointer via CAS; the RELEASE on
    // the CAS pairs with ACQUIRE loads of host->aclk_host_config in readers so they
    // cannot observe the pointer with zero-initialized fields (host == NULL etc.).
    aclk_host_config->host = host;
    aclk_alert_streaming_set(aclk_host_config, false);
    time_t now = now_realtime_sec();
    aclk_send_timestamp_set(
        &aclk_host_config->node_info_send_time,
        (host == localhost || NULL == localhost) ? nd_time_t_add_saturating(now, -25) : now);
    // node_manifest_send_time is deliberately NOT armed here: build_node_info() arms it, so it is
    // armed after that host's node info was built rather than alongside it. That orders the ARMING
    // only - both messages are dispatched to parallel query workers, so it is no guarantee of
    // publish order at the cloud. What keeps the manifest from describing a node the cloud does not
    // know is the node_id check in aclk_check_node_info_collectors_and_manifest().
    // (set_host_node_id() also arms right after creating a config, so this is not the only path.)

    struct aclk_sync_cfg_t *expected = NULL;
    if (__atomic_compare_exchange_n(&host->aclk_host_config, &expected, aclk_host_config, false, __ATOMIC_RELEASE, __ATOMIC_RELAXED)) {
        if (node_id && UUIDiszero(host->node_id))
            uuid_copy(host->node_id.uuid, *node_id);
    }
    else {
        freez(aclk_host_config);
        return;
    }
}

// Replaces two correlated subqueries (host_label, node_instance) with LEFT JOINs
// so the planner does the lookup once per row instead of 2N extra index probes.
// memory_mode and health_enabled are intentionally omitted — they were SELECTed
// in the previous shape but never read by the consumer.
//
// Both LEFT JOINs are guaranteed to match at most one row per host by the
// schema (sqlite_metadata.c database_config[]): host_label has
// PRIMARY KEY (host_id, label_key), and node_instance has host_id PRIMARY KEY.
// Row multiplication is therefore impossible here without a SQLite invariant
// violation; no DISTINCT / GROUP BY / EXISTS wrapper needed.
#define SQL_FETCH_ALL_HOSTS                                                                                            \
    "SELECT h.host_id, h.hostname, h.registry_hostname, h.update_every, h.os, "                                        \
    "h.timezone, h.hops, h.abbrev_timezone, h.utc_offset, h.program_name, "                                            \
    "h.program_version, h.entries, h.last_connected, "                                                                 \
    "CASE WHEN hl.label_value = 'true' THEN 1 ELSE 0 END, "                                                            \
    "CASE WHEN ni.node_id IS NULL THEN 0 ELSE 1 END "                                                                  \
    "FROM host h "                                                                                                     \
    "LEFT JOIN host_label hl ON hl.host_id = h.host_id AND hl.label_key = '_is_ephemeral' "                            \
    "LEFT JOIN node_instance ni ON ni.host_id = h.host_id "                                                            \
    "WHERE h.hops > 0"

#define SQL_FETCH_ALL_INSTANCES                                                                                        \
    "SELECT ni.host_id, ni.node_id FROM host h, node_instance ni "                                                     \
    "WHERE h.host_id = ni.host_id AND ni.node_id IS NOT NULL"


uv_sem_t ctx_sem;

void aclk_synchronization_init(void)
{
    nd_log_daemon(NDLP_INFO, "Creating archived hosts");
    struct children node_data = { 0, 0 };

    sqlite3_stmt *res = NULL;
    if (PREPARE_STATEMENT(db_meta, SQL_FETCH_ALL_HOSTS, &res)) {
        int step_rc;
        while ((step_rc = sqlite3_step_monitored(res)) == SQLITE_ROW) {
            RRDHOST *host = load_archived_host_from_row(res);
            if (!host)
                continue;
            if (IS_VIRTUAL_HOST_OS(host))
                node_data.vnodes++;
            else
                node_data.normal++;
        }
        if (step_rc != SQLITE_DONE)
            nd_log_daemon(
                NDLP_ERR,
                "SQLite error while loading archived hosts, rc = %d (%s); load may be partial",
                step_rc,
                sqlite3_errmsg(db_meta));
        SQLITE_FINALIZE(res);
    }
    else
        nd_log_daemon(NDLP_ERR,
                      "SQLite error when preparing statement to load archived hosts: %s",
                      sqlite3_errmsg(db_meta));

    nd_log_daemon(
        NDLP_INFO,
        "Created %d archived hosts (%d children and %d vnodes)",
        node_data.normal + node_data.vnodes,
        node_data.normal,
        node_data.vnodes);

    bool sem_init = true;
    uv_sem_init(&ctx_sem, 0);

    // Trigger host context load for hosts that have been created
    if (unlikely(!metadata_queue_load_host_context())) {
        nd_log_daemon(NDLP_WARNING, "Failed to queue command to load contexts for archived hosts");
        // Reset context load flag so that contexts will be loaded on demand
        reset_host_context_load_flag();
        uv_sem_destroy(&ctx_sem);
        sem_init = false;
    }

    sqlite3_stmt *res_inst = NULL;
    if (PREPARE_STATEMENT(db_meta, SQL_FETCH_ALL_INSTANCES, &res_inst)) {
        int step_rc;
        while ((step_rc = sqlite3_step_monitored(res_inst)) == SQLITE_ROW) {
            nd_uuid_t host_uuid, node_uuid;
            if (!sqlite3_column_uuid_copy(res_inst, 0, host_uuid)) {
                nd_log_daemon(
                    NDLP_ERR,
                    "Skipping node_instance row: host_id (col 0) is not a valid 16-byte UUID blob (type=%d, bytes=%d). ACLK config not configured for this host.",
                    sqlite3_column_type(res_inst, 0),
                    sqlite3_column_bytes(res_inst, 0));
                continue;
            }
            if (!sqlite3_column_uuid_copy(res_inst, 1, node_uuid)) {
                nd_log_daemon(
                    NDLP_ERR,
                    "Skipping node_instance row: node_id (col 1) is not a valid 16-byte UUID blob (type=%d, bytes=%d). ACLK config not configured for this host.",
                    sqlite3_column_type(res_inst, 1),
                    sqlite3_column_bytes(res_inst, 1));
                continue;
            }

            char uuid_str[UUID_STR_LEN];
            uuid_unparse_lower(host_uuid, uuid_str);
            RRDHOST *host = rrdhost_find_by_guid(uuid_str);
            // create_aclk_config() already null-checks `host`, but the explicit
            // guard makes the intent clear and skips the call for unknown GUIDs.
            if (host && host != localhost)
                create_aclk_config(host, &host_uuid, &node_uuid);
        }
        if (step_rc != SQLITE_DONE)
            nd_log_daemon(
                NDLP_ERR,
                "SQLite error while configuring host ACLK synchronization parameters, rc = %d (%s); some configs may be missing",
                step_rc,
                sqlite3_errmsg(db_meta));
        SQLITE_FINALIZE(res_inst);
    }
    else
        nd_log_daemon(NDLP_ERR,
                      "SQLite error when preparing statement to configure host ACLK synchronization parameters: %s",
                      sqlite3_errmsg(db_meta));

    aclk_initialize_event_loop();

    if (!(node_data.normal + node_data.vnodes))
        aclk_queue_node_info(localhost, true);

    if (sem_init) {
        int finished_vnodes = 0;
        time_t deadline = nd_time_t_add_saturating(now_realtime_sec(), 60);  // hard timeput to avoid infinite block
        while (finished_vnodes < node_data.vnodes) {
            if (uv_sem_trywait(&ctx_sem) == 0) {
                finished_vnodes++;
                continue;
            }

            if (now_realtime_sec() >= deadline) {
                nd_log_daemon(NDLP_WARNING, "Vnodes context load still in progress, continue with agent start");
                break;
            }
            sleep_usec(100 * USEC_PER_MS);
        }
        if (finished_vnodes == node_data.vnodes) {
            uv_sem_destroy(&ctx_sem);
        }
    }
    nd_log_daemon(NDLP_INFO, "ACLK sync initialization completed");
}

static inline bool queue_aclk_sync_cmd(enum aclk_database_opcode opcode, const void *param0, const void *param1)
{
    cmd_data_t cmd;
    cmd.opcode = opcode;
    cmd.param[0] = (void *) param0;
    cmd.param[1] = (void *) param1;

    // The event loop MUST NOT wait for queue capacity: it is the only consumer, so blocking on a full
    // pool parks the thread that has to drain it, and nothing ever frees a slot. That matters more now
    // that destroy_aclk_config() waits on this loop unboundedly while holding rrd_wrlock() - a loop
    // parked on its own not_full condition would wedge the global rrd lock behind it.
    //
    // Reachable from here: node_update_timer_cb() and the ACLK_DATABASE_NODE_STATE fallback both call
    // aclk_host_state_update_auto(), which enqueues a job. Every caller already handles a refusal by
    // releasing whatever it owns, so dropping the command is safe where deadlocking is not.
    bool wait_on_full = !aclk_sync_on_event_loop_thread();

    return aclk_database_enq_cmd(&cmd, wait_on_full);
}

void aclk_synchronization_shutdown(void)
{
    if (!aclk_sync_config.thread)
        return;

    // Send shutdown command, note that the completion is initialized
    // on init and still valid
    aclk_mqtt_client_reset();

    // NOTE the asymmetry with metadata_sync_shutdown(), which DOES latch when its command cannot
    // be queued: that function returns immediately without joining, so nothing else establishes
    // that its thread is gone. Here we fall through to nd_thread_join() regardless, and a
    // successful join is proof the thread finished. Latching on the enqueue failure as well
    // would leave the one-way latch set on a shutdown where no ACLK worker remains, leaking the
    // metadata and context handles and skipping sqlite3_shutdown() for nothing. The failed-join
    // branch below is the one that matters.
    if (queue_aclk_sync_cmd(ACLK_SYNC_SHUTDOWN, NULL, NULL))
        completion_wait_for(&aclk_sync_config.start_stop_complete);

    completion_destroy(&aclk_sync_config.start_stop_complete);
    int rc = nd_thread_join(aclk_sync_config.thread);
    if (rc) {
        nd_log_daemon(NDLP_ERR, "ACLK: Failed to join synchronization thread");
        sqlite_mark_teardown_unsafe("the ACLK synchronization thread could not be joined");
    }
    else
        nd_log_daemon(NDLP_INFO, "ACLK: synchronization thread shutdown completed");
}

// Public
void aclk_push_alert_config(const char *node_id, const char *config_hash)
{
    if (unlikely(!node_id || !config_hash))
        return;

    char *node_id_dup = strdupz(node_id);
    char *config_hash_dup = strdupz(config_hash);
    bool queued = queue_aclk_sync_cmd(ACLK_DATABASE_PUSH_ALERT_CONFIG, node_id_dup, config_hash_dup);
    if (unlikely(!queued)) {
        nd_log_daemon(NDLP_WARNING, "ACLK: Failed to queue alert config push for node %s (config hash %s)", node_id, config_hash);
        freez(node_id_dup);
        freez(config_hash_dup);
    }
}

// Whether the caller is running on the ACLK sync event loop. That thread has one hard constraint the
// workers do not: a host teardown can be blocked inside destroy_aclk_config() waiting for it while
// holding rrd_wrlock(), so anything reachable from this thread that waits for rrd_rdlock() closes a
// cycle. A worker blocking on it is safe - the event loop stays free to satisfy that wait.
//
// Returns false before the loop has published its id, which is the safe answer: nothing can be
// waiting on a loop that has not started.
bool aclk_sync_on_event_loop_thread(void)
{
    pid_t tid = __atomic_load_n(&aclk_sync_config.event_loop_tid, __ATOMIC_ACQUIRE);
    return tid != 0 && tid == gettid_cached();
}

void aclk_execute_query(aclk_query_t *query)
{
    if (unlikely(!query))
        return;

    bool queued = queue_aclk_sync_cmd(ACLK_QUERY_EXECUTE, query, NULL);
    if (unlikely(!queued)) {
        nd_log_daemon(NDLP_WARNING, "ACLK: Failed to queue query execution");
        aclk_query_free(query);
    }
}

void aclk_add_job(aclk_query_t *query)
{
    if (unlikely(!query))
        return;

    bool queued = queue_aclk_sync_cmd(ACLK_QUERY_BATCH_ADD, query, NULL);
    if (unlikely(!queued)) {
        nd_log_daemon(NDLP_WARNING, "ACLK: Failed to queue query job");
        aclk_query_free(query);
    }
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

    char *machine_guid_dup = strdupz(machine_guid);
    bool queued = queue_aclk_sync_cmd(ACLK_DATABASE_NODE_UNREGISTER, machine_guid_dup, NULL);
    if (unlikely(!queued)) {
        nd_log_daemon(NDLP_WARNING, "ACLK: Failed to queue unregister node command for %s", machine_guid);
        freez(machine_guid_dup);
    }
}

// Blocks until no per-host uv_timer_t can still be registered on the sync loop.
//
// Only CLOSING has to wait. NEVER_STARTED means the loop never ran, so it never registered a timer
// for anybody; HANDLES_CLOSED means uv_walk() has already retired them. RUNNING cannot reach here:
// a refusal means the pool was closed, which happens after the phase leaves RUNNING - and if it is
// read stale the pool is closed anyway, so the next phase read resolves it.
static void aclk_sync_wait_for_handles_closed(void)
{
    if (__atomic_load_n(&aclk_sync_config.phase, __ATOMIC_ACQUIRE) == ACLK_SYNC_CLOSING)
        completion_wait_for(&aclk_sync_config.handles_closed);
}

// Waited for before the caller frees the host. The command pool is FIFO, so once the event loop
// has run this cancel, every command queued earlier for this host has already been consumed - and
// those commands carry a raw RRDHOST * (schedule_node_state_update(), aclk_queue_node_info()).
// Without this barrier the loop would dereference the host after it was freed.
void destroy_aclk_config(RRDHOST *host)
{
    if (!host)
        return;

    // The drain is NOT conditional on this host having a config: a node-state command queued for a
    // host whose config has not been created yet is exactly the case that used to skip it, and it
    // is the command most likely to still be in the queue - the handler creates the config, so a
    // NULL config means the loop has not consumed it.
    //
    // Never from the event loop itself: we would be waiting on the thread that has to run the
    // command. No caller does this today - rrdhost_free_unlinked() and the nrpc unittest, where
    // the loop is not running - so this is an invariant made explicit rather than a case to handle.
    if(!aclk_sync_on_event_loop_thread()) {
        struct completion compl;
        completion_init(&compl);

        // The wait MUST NOT be bounded. `compl` lives on this stack and the queued command holds a
        // pointer to it, so returning early would let the loop complete a destroyed completion and
        // dereference a host this caller is about to free - the very lifetime bug this barrier
        // exists to prevent, and reachable whenever the loop is merely slow rather than gone.
        //
        // Waiting forever is safe because the loop answers everything it accepts: on the way out it
        // closes the pool - making refusal and insertion mutually exclusive - and then completes
        // any cancel still queued. The loop also never waits for queue capacity itself
        // (queue_aclk_sync_cmd()), so it cannot park on its own full pool with this barrier behind it.
        if (queue_aclk_sync_cmd(ACLK_CANCEL_NODE_UPDATE_TIMER, (void *)host, (void *)&compl))
            completion_wait_for(&compl);
        else
            // Refused. That answers "my command will not run", NOT "the loop holds nothing of mine":
            // the per-host uv_timer_t is registered by the loop, inside the config freed below, so a
            // refusal issued while the loop is still closing its handles would free that timer out
            // from under libuv. Wait for the point where that is provably no longer true.
            aclk_sync_wait_for_handles_closed();

        completion_destroy(&compl);
    }

    struct aclk_sync_cfg_t *aclk_host_config = __atomic_load_n(&host->aclk_host_config, __ATOMIC_ACQUIRE);
    if (!aclk_host_config)
        return;

    struct aclk_sync_cfg_t *old_aclk_host_config = __atomic_exchange_n(&host->aclk_host_config, NULL, __ATOMIC_ACQUIRE);
    if (!old_aclk_host_config)
        return;

    // detach pending checkpoint strings under lock, to avoid racing with save/replay
    spinlock_lock(&old_aclk_host_config->pending_ctx_spinlock);
    char *pending_claim_id = old_aclk_host_config->pending_ctx_claim_id;
    char *pending_node_id = old_aclk_host_config->pending_ctx_node_id;
    old_aclk_host_config->pending_ctx_claim_id = NULL;
    old_aclk_host_config->pending_ctx_node_id = NULL;
    old_aclk_host_config->pending_ctx_version_hash = 0;
    old_aclk_host_config->pending_ctx_saved_monotonic_s = 0;
    __atomic_store_n(&old_aclk_host_config->pending_ctx_checkpoint, false, __ATOMIC_RELEASE);
    spinlock_unlock(&old_aclk_host_config->pending_ctx_spinlock);

    freez(pending_claim_id);
    freez(pending_node_id);
    freez(old_aclk_host_config);
}

void aclk_queue_node_info(RRDHOST *host, bool immediate)
{
    (void) queue_aclk_sync_cmd(ACLK_QUEUE_NODE_INFO, (void *)host, (void *)(uintptr_t)immediate);
}

// Requests a refresh of the cloud function manifest for this host.
//
// Arming is a single atomic CAS on the host's aclk config, so - unlike node info, which needs
// the event loop to create a missing config - this does NOT go through the command queue. That
// keeps a burst of function registrations (a parent's children reconnecting, a plugin
// restarting) from flooding the ACLK sync command pool, and avoids publishing a borrowed host
// pointer to another thread.
//
// A change made before the arm is always reported: build_node_manifest() reads the live function
// list rather than a snapshot taken here, so an already-armed request covers this change too,
// and a change racing with the send re-arms (the claim resets the timestamp to 0).
void aclk_arm_node_manifest(RRDHOST *host)
{
    if (!host)
        return;

    // No config: nothing to arm. build_node_info() arms the first manifest for every host that
    // gets one, which covers every function registered until then.
    //
    // The load below is lock-free and destroy_aclk_config() frees what it returns, so what keeps
    // this safe is teardown ordering, not the NULL check. destroy_aclk_config() has exactly one
    // caller - rrdhost_free_unlinked() - and everything this function's callers depend on is torn
    // down earlier in it:
    //   1. rrdhost_index_del_by_guid()          - the host stops being findable
    //   2. stream_receiver_signal_to_stop_and_wait() - waits for host->receiver to become NULL,
    //      but the wait is BOUNDED (~2s) and gives up on a stalled receiver thread, so step 2 is
    //      best-effort, not a guarantee (pre-existing residual, tracked separately)
    //   3. nrpc_registry_destroy()         - the host's registry entry is synchronously DISARMED
    //      (owner callbacks cleared under the entry's lock, so the component can no longer call
    //      this function for that host) and leaves the component index
    //   4. destroy_aclk_config()                - only now is the config freed
    // So the function-registry paths (which reach this only through the owner callback the disarm
    // cleared in step 3) and rrdhost_clear_receiver() (which runs before host->receiver is
    // cleared) cannot still be running here, and a caller that reached this host through
    // rrdhost_find_by_guid() did so before step 1. An owner-callback invocation that snapshotted
    // the callback JUST before the disarm is bounded by the owner's thread lifecycle: the sender
    // is joined and the receiver stopped (best-effort, step 2) before step 3 runs.
    // Do NOT "fix" this by routing the arm back through the ACLK event loop: that
    // publishes a borrowed RRDHOST pointer to another thread (a wider window on a longer-lived
    // object) and blocks the caller in push_cmd() when the command pool is full, sometimes while
    // holding the host functions lock.
    //
    // One caller does NOT reach the host any of those ways: aclk_arm_node_manifest_all_hosts()
    // walks rrdhost_root_index. That walk only yields hosts still indexed when it reaches them, so
    // step 1 bounds it - but the index links the host without owning it, so the walk's reference
    // does not stop rrdhost_free_unlinked() from freeing a host it already unlinked. That caller
    // therefore carries the same pre-existing teardown exposure as the alert-push scan, which
    // dereferences hosts from the same index (see build_node_manifest() in sqlite_aclk_node.c).
    struct aclk_sync_cfg_t *aclk_host_config = __atomic_load_n(&host->aclk_host_config, __ATOMIC_ACQUIRE);
    if (!aclk_host_config)
        return;

    aclk_send_timestamp_arm(&aclk_host_config->node_manifest_send_time, now_realtime_sec());
}

// Re-arms the manifest of every host. Called when the cloud asks the agent to re-announce its node
// instances (the SendNodeInstances message), which it does after connecting - so this is where the
// manifest gets its request for a new ACLK session. How often the cloud repeats that ask within one
// session is server-side behaviour this repository cannot verify, so treat "once per session" as an
// assumption, not a guarantee; the suppression below is what makes repeats cheap either way.
//
// It is needed because a successful send is NOT a delivery. send_bin_msg() returning 0 means only
// that the PUBLISH was appended to mqtt_ng's transaction buffer, and that is exactly when the
// publication record is kept (published = true). Three windows then lose the message with no
// agent-side signal at all:
//   1. disconnect before the bytes reach the socket - the fragment is still queued, and
//      mqtt_ng_connect() purges the whole tx buffer on the next connect (buffer_purge()).
//   2. disconnect after the write but before the PUBACK - QoS1, but mqtt_ng_connect() also calls
//      destroy_timeout_monitor_list(), so nothing is retransmitted and there is no MQTT session
//      continuation.
//   3. no PUBACK within PACKET_ACK_TIMEOUT_SECS (60s) while still connected -
//      check_packet_monitor_list_for_timeouts() calls mark_packet_acked(), the same path a real
//      PUBACK takes, and the message is garbage collected as if it had been delivered.
// Nothing correlates an ack back to a query in any case: send_bin_msg() passes NULL for the packet
// id and puback_callback() only counts pubacks. build_node_manifest() scopes its suppression to one
// session for exactly this reason, so the pair publishes one manifest per host per session. A
// redundant arm costs one manifest build (rrd_rdlock plus a dictionary of string copies) and one
// hash; the content hash then drops the publish.
//
// Note what this does NOT cover: a new session lets a rebuild through, but it does not cause one.
// Something still has to arm the host. Every other arm is event-driven - a function registration,
// pluginsd, a child reconnecting, host creation, an ingestion-status change, or a node info send
// (build_node_info() arms too, and aclk_queue_node_info() is reached independently of this message,
// from metadata load, label updates, streaming reconnect and pluginsd). None of them is tied to an
// ACLK reconnect, so on a node whose functions and labels are stable this ask is what triggers the
// rebuild, and without it a manifest lost above stays lost for the whole session.
void aclk_arm_node_manifest_all_hosts(void)
{
    RRDHOST *host;
    dfe_start_reentrant(rrdhost_root_index, host)
    {
        aclk_arm_node_manifest(host);
    }
    dfe_done(host);
}
