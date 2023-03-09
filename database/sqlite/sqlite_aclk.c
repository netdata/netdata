// SPDX-License-Identifier: GPL-3.0-or-later

#include "sqlite_functions.h"
#include "sqlite_aclk.h"

#include "sqlite_aclk_node.h"

void sanity_check(void) {
    // make sure the compiler will stop on misconfigurations
    BUILD_BUG_ON(WORKER_UTILIZATION_MAX_JOB_TYPES < ACLK_MAX_ENUMERATIONS_DEFINED);
}

void schedule_node_info_update(RRDHOST *host)
{
    if (unlikely(!host))
        return;

    struct aclk_database_worker_config *wc = host->dbsync_worker;

    if (unlikely(!wc))
        return;

    struct aclk_database_cmd cmd;
    memset(&cmd, 0, sizeof(cmd));
    cmd.opcode = ACLK_DATABASE_NODE_STATE;
    cmd.completion = NULL;
    aclk_database_enq_cmd(wc, &cmd);
}

#ifdef ENABLE_ACLK
static int sql_check_aclk_table(void *data, int argc, char **argv, char **column)
{
    struct aclk_database_worker_config *wc = data;
    UNUSED(argc);
    UNUSED(column);

    debug(D_ACLK_SYNC,"Scheduling aclk sync table check for node %s", (char *) argv[0]);
    struct aclk_database_cmd cmd;
    memset(&cmd, 0, sizeof(cmd));
    cmd.opcode = ACLK_DATABASE_DELETE_HOST;
    cmd.data = strdupz((char *) argv[0]);
    aclk_database_enq_cmd_noblock(wc, &cmd);
    return 0;
}

#define SQL_SELECT_ACLK_ACTIVE_LIST "SELECT REPLACE(SUBSTR(name,19),'_','-') FROM sqlite_schema " \
        "WHERE name LIKE 'aclk_chart_latest_%' AND type IN ('table');"

static void sql_check_aclk_table_list(struct aclk_database_worker_config *wc)
{
    char *err_msg = NULL;
    debug(D_ACLK_SYNC,"Cleaning tables for nodes that do not exist");
    int rc = sqlite3_exec_monitored(db_meta, SQL_SELECT_ACLK_ACTIVE_LIST, sql_check_aclk_table, (void *) wc, &err_msg);
    if (rc != SQLITE_OK) {
        error_report("Query failed when trying to check for obsolete ACLK sync tables, %s", err_msg);
        sqlite3_free(err_msg);
    }
}

static void sql_maint_aclk_sync_database(struct aclk_database_worker_config *wc, struct aclk_database_cmd cmd)
{
    UNUSED(cmd);

    debug(D_ACLK, "Checking database for %s", wc->host_guid);

    BUFFER *sql = buffer_create(ACLK_SYNC_QUERY_SIZE, &netdata_buffers_statistics.buffers_sqlite);

    buffer_sprintf(sql,"DELETE FROM aclk_alert_%s WHERE date_submitted IS NOT NULL AND "
                        "CAST(date_cloud_ack AS INT) < unixepoch()-%d;", wc->uuid_str, ACLK_DELETE_ACK_ALERTS_INTERNAL);
    db_execute(buffer_tostring(sql));

    buffer_free(sql);
}


#define SQL_SELECT_HOST_BY_UUID  "SELECT host_id FROM host WHERE host_id = @host_id;"

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
        error_report("Failed to bind host_id parameter to select node instance information");
        goto failed;
    }
    rc = sqlite3_step_monitored(res);

failed:
    if (unlikely(sqlite3_finalize(res) != SQLITE_OK))
        error_report("Failed to finalize the prepared statement when checking host existence");

    return (rc == SQLITE_ROW);
}

// OPCODE: ACLK_DATABASE_DELETE_HOST
void sql_delete_aclk_table_list(struct aclk_database_worker_config *wc, struct aclk_database_cmd cmd)
{
    UNUSED(wc);
    char uuid_str[GUID_LEN + 1];
    char host_str[GUID_LEN + 1];

    int rc;
    uuid_t host_uuid;
    char *host_guid = (char *)cmd.data;

    if (unlikely(!host_guid))
        return;

    rc = uuid_parse(host_guid, host_uuid);
    freez(host_guid);
    if (rc)
        return;

    uuid_unparse_lower(host_uuid, host_str);
    uuid_unparse_lower_fix(&host_uuid, uuid_str);

    debug(D_ACLK_SYNC, "Checking if I should delete aclk tables for node %s", host_str);

    if (is_host_available(&host_uuid)) {
        debug(D_ACLK_SYNC, "Host %s exists, not deleting aclk sync tables", host_str);
        return;
    }

    debug(D_ACLK_SYNC, "Host %s does NOT exist, can delete aclk sync tables", host_str);

    sqlite3_stmt *res = NULL;
    BUFFER *sql = buffer_create(ACLK_SYNC_QUERY_SIZE, &netdata_buffers_statistics.buffers_sqlite);

    buffer_sprintf(sql,"SELECT 'drop '||type||' IF EXISTS '||name||';' FROM sqlite_schema " \
                        "WHERE name LIKE 'aclk_%%_%s' AND type IN ('table', 'trigger', 'index');", uuid_str);

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

    db_execute(buffer_tostring(sql));

fail:
    buffer_free(sql);
}
#endif

uv_mutex_t aclk_async_lock;
struct aclk_database_worker_config  *aclk_thread_head = NULL;

int claimed()
{
    int rc;
    rrdhost_aclk_state_lock(localhost);
    rc = (localhost->aclk_state.claimed_id != NULL);
    rrdhost_aclk_state_unlock(localhost);
    return rc;
}

void aclk_add_worker_thread(struct aclk_database_worker_config *wc)
{
    if (unlikely(!wc))
        return;

    uv_mutex_lock(&aclk_async_lock);
    if (unlikely(!wc->host)) {
        wc->next = aclk_thread_head;
        aclk_thread_head = wc;
    }
    uv_mutex_unlock(&aclk_async_lock);
}

void aclk_del_worker_thread(struct aclk_database_worker_config *wc)
{
    if (unlikely(!wc))
        return;

    uv_mutex_lock(&aclk_async_lock);
    struct aclk_database_worker_config **tmp = &aclk_thread_head;
    while (*tmp && (*tmp) != wc)
        tmp = &(*tmp)->next;
    if (*tmp)
        *tmp = wc->next;
    uv_mutex_unlock(&aclk_async_lock);
}

int aclk_worker_thread_exists(char *guid)
{
    int rc = 0;
    uv_mutex_lock(&aclk_async_lock);

    struct aclk_database_worker_config *tmp = aclk_thread_head;

    while (tmp && !rc) {
        rc = strcmp(tmp->uuid_str, guid) == 0;
        tmp = tmp->next;
    }
    uv_mutex_unlock(&aclk_async_lock);
    return rc;
}

void aclk_database_init_cmd_queue(struct aclk_database_worker_config *wc)
{
    wc->cmd_queue.head = wc->cmd_queue.tail = 0;
    wc->queue_size = 0;
    fatal_assert(0 == uv_cond_init(&wc->cmd_cond));
    fatal_assert(0 == uv_mutex_init(&wc->cmd_mutex));
}

int aclk_database_enq_cmd_noblock(struct aclk_database_worker_config *wc, struct aclk_database_cmd *cmd)
{
    unsigned queue_size;

    /* wait for free space in queue */
    uv_mutex_lock(&wc->cmd_mutex);
    if ((queue_size = wc->queue_size) == ACLK_DATABASE_CMD_Q_MAX_SIZE || wc->is_shutting_down) {
        uv_mutex_unlock(&wc->cmd_mutex);
        return 1;
    }

    fatal_assert(queue_size < ACLK_DATABASE_CMD_Q_MAX_SIZE);
    /* enqueue command */
    wc->cmd_queue.cmd_array[wc->cmd_queue.tail] = *cmd;
    wc->cmd_queue.tail = wc->cmd_queue.tail != ACLK_DATABASE_CMD_Q_MAX_SIZE - 1 ?
                         wc->cmd_queue.tail + 1 : 0;
    wc->queue_size = queue_size + 1;
    uv_mutex_unlock(&wc->cmd_mutex);
    return 0;
}

void aclk_database_enq_cmd(struct aclk_database_worker_config *wc, struct aclk_database_cmd *cmd)
{
    unsigned queue_size;

    /* wait for free space in queue */
    uv_mutex_lock(&wc->cmd_mutex);
    if (wc->is_shutting_down) {
        uv_mutex_unlock(&wc->cmd_mutex);
        return;
    }
    while ((queue_size = wc->queue_size) == ACLK_DATABASE_CMD_Q_MAX_SIZE) {
        uv_cond_wait(&wc->cmd_cond, &wc->cmd_mutex);
    }
    fatal_assert(queue_size < ACLK_DATABASE_CMD_Q_MAX_SIZE);
    /* enqueue command */
    wc->cmd_queue.cmd_array[wc->cmd_queue.tail] = *cmd;
    wc->cmd_queue.tail = wc->cmd_queue.tail != ACLK_DATABASE_CMD_Q_MAX_SIZE - 1 ?
                         wc->cmd_queue.tail + 1 : 0;
    wc->queue_size = queue_size + 1;
    uv_mutex_unlock(&wc->cmd_mutex);

    /* wake up event loop */
    int rc = uv_async_send(&wc->async);
    if (unlikely(rc))
        debug(D_ACLK_SYNC, "Failed to wake up event loop");
}

struct aclk_database_cmd aclk_database_deq_cmd(struct aclk_database_worker_config* wc)
{
    struct aclk_database_cmd ret;
    unsigned queue_size;

    uv_mutex_lock(&wc->cmd_mutex);
    queue_size = wc->queue_size;
    if (queue_size == 0 || wc->is_shutting_down) {
        memset(&ret, 0, sizeof(ret));
        ret.opcode = ACLK_DATABASE_NOOP;
        ret.completion = NULL;
        if (wc->is_shutting_down)
            uv_cond_signal(&wc->cmd_cond);
    } else {
        /* dequeue command */
        ret = wc->cmd_queue.cmd_array[wc->cmd_queue.head];
        if (queue_size == 1) {
            wc->cmd_queue.head = wc->cmd_queue.tail = 0;
        } else {
            wc->cmd_queue.head = wc->cmd_queue.head != ACLK_DATABASE_CMD_Q_MAX_SIZE - 1 ?
                                 wc->cmd_queue.head + 1 : 0;
        }
        wc->queue_size = queue_size - 1;
        /* wake up producers */
        uv_cond_signal(&wc->cmd_cond);
    }
    uv_mutex_unlock(&wc->cmd_mutex);

    return ret;
}

struct aclk_database_worker_config *find_inactive_wc_by_node_id(char *node_id)
{
    if (unlikely(!node_id))
        return NULL;

    uv_mutex_lock(&aclk_async_lock);
    struct aclk_database_worker_config *wc = aclk_thread_head;

    while (wc) {
        if (!strcmp(wc->node_id, node_id))
            break;
        wc = wc->next;
    }
    uv_mutex_unlock(&aclk_async_lock);

    return (wc);
}

void aclk_sync_exit_all()
{
    rrd_rdlock();
    RRDHOST *host;
    rrdhost_foreach_read(host) {
        struct aclk_database_worker_config *wc = host->dbsync_worker;
        if (wc) {
            wc->is_shutting_down = 1;
            (void) aclk_database_deq_cmd(wc);
            uv_cond_signal(&wc->cmd_cond);
        }
    }
    rrd_unlock();

    uv_mutex_lock(&aclk_async_lock);
    struct aclk_database_worker_config *wc = aclk_thread_head;
    while (wc) {
        wc->is_shutting_down = 1;
        wc = wc->next;
    }
    uv_mutex_unlock(&aclk_async_lock);
}

enum {
    IDX_HOST_ID,
    IDX_HOSTNAME,
    IDX_REGISTRY,
    IDX_UPDATE_EVERY,
    IDX_OS,
    IDX_TIMEZONE,
    IDX_TAGS,
    IDX_HOPS,
    IDX_MEMORY_MODE,
    IDX_ABBREV_TIMEZONE,
    IDX_UTC_OFFSET,
    IDX_PROGRAM_NAME,
    IDX_PROGRAM_VERSION,
    IDX_ENTRIES,
    IDX_HEALTH_ENABLED,
};

static int create_host_callback(void *data, int argc, char **argv, char **column)
{
    UNUSED(data);
    UNUSED(argc);
    UNUSED(column);

    char guid[UUID_STR_LEN];
    uuid_unparse_lower(*(uuid_t *)argv[IDX_HOST_ID], guid);

    struct rrdhost_system_info *system_info = callocz(1, sizeof(struct rrdhost_system_info));
    __atomic_sub_fetch(&netdata_buffers_statistics.rrdhost_allocations_size, sizeof(struct rrdhost_system_info), __ATOMIC_RELAXED);

    system_info->hops = str2i((const char *) argv[IDX_HOPS]);

    sql_build_host_system_info((uuid_t *)argv[IDX_HOST_ID], system_info);

    RRDHOST *host = rrdhost_find_or_create(
          (const char *) argv[IDX_HOSTNAME]
        , (const char *) argv[IDX_REGISTRY]
        , guid
        , (const char *) argv[IDX_OS]
        , (const char *) argv[IDX_TIMEZONE]
        , (const char *) argv[IDX_ABBREV_TIMEZONE]
        , argv[IDX_UTC_OFFSET] ? str2uint32_t(argv[IDX_UTC_OFFSET], NULL) : 0
        , (const char *) argv[IDX_TAGS]
        , (const char *) (argv[IDX_PROGRAM_NAME] ? argv[IDX_PROGRAM_NAME] : "unknown")
        , (const char *) (argv[IDX_PROGRAM_VERSION] ? argv[IDX_PROGRAM_VERSION] : "unknown")
        , argv[3] ? str2i(argv[IDX_UPDATE_EVERY]) : 1
        , argv[13] ? str2i(argv[IDX_ENTRIES]) : 0
        , default_rrd_memory_mode
        , 0 // health
        , 0 // rrdpush enabled
        , NULL  //destination
        , NULL  // api key
        , NULL  // send charts matching
        , false // rrdpush_enable_replication
        , 0     // rrdpush_seconds_to_replicate
        , 0     // rrdpush_replication_step
        , system_info
        , 1
    );
    if (likely(host))
        host->rrdlabels = sql_load_host_labels((uuid_t *)argv[IDX_HOST_ID]);

#ifdef NETDATA_INTERNAL_CHECKS
    char node_str[UUID_STR_LEN] = "<none>";
    if (likely(host->node_id))
        uuid_unparse_lower(*host->node_id, node_str);
    internal_error(true, "Adding archived host \"%s\" with GUID \"%s\" node id = \"%s\"", rrdhost_hostname(host), host->machine_guid, node_str);
#endif
    return 0;
}

#ifdef ENABLE_ACLK
static int aclk_start_sync_thread(void *data, int argc, char **argv, char **column)
{
    char uuid_str[GUID_LEN + 1];
    UNUSED(data);
    UNUSED(argc);
    UNUSED(column);

    uuid_unparse_lower(*((uuid_t *) argv[0]), uuid_str);

    RRDHOST *host = rrdhost_find_by_guid(uuid_str);
    if (host == localhost)
        return 0;

    sql_create_aclk_table(host, (uuid_t *) argv[0], (uuid_t *) argv[1]);
    return 0;
}
#endif
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

    info("Creating archived hosts");
    rc = sqlite3_exec_monitored(db_meta, "SELECT host_id, hostname, registry_hostname, update_every, os, "
       "timezone, tags, hops, memory_mode, abbrev_timezone, utc_offset, program_name, "
       "program_version, entries, health_enabled FROM host WHERE hops >0;",
          create_host_callback, NULL, &err_msg);
    if (rc != SQLITE_OK) {
        error_report("SQLite error when loading archived hosts, rc = %d (%s)", rc, err_msg);
        sqlite3_free(err_msg);
    }

#ifdef ENABLE_ACLK
    fatal_assert(0 == uv_mutex_init(&aclk_async_lock));
    rc = sqlite3_exec_monitored(db_meta, "SELECT ni.host_id, ni.node_id FROM host h, node_instance ni WHERE "
        "h.host_id = ni.host_id AND ni.node_id IS NOT NULL;", aclk_start_sync_thread, NULL, &err_msg);
    if (rc != SQLITE_OK) {
        error_report("SQLite error when starting ACLK sync threads, rc = %d (%s)", rc, err_msg);
        sqlite3_free(err_msg);
    }
    info("ACLK sync initialization completed");
#endif
}
#ifdef ENABLE_ACLK
static void async_cb(uv_async_t *handle)
{
    uv_stop(handle->loop);
    uv_update_time(handle->loop);
    debug(D_ACLK_SYNC, "%s called, active=%d.", __func__, uv_is_active((uv_handle_t *)handle));
}

#define TIMER_PERIOD_MS (1000)

static void timer_cb(uv_timer_t* handle)
{
    uv_stop(handle->loop);
    uv_update_time(handle->loop);

    struct aclk_database_worker_config *wc = handle->data;
    struct aclk_database_cmd cmd;
    memset(&cmd, 0, sizeof(cmd));
    cmd.opcode = ACLK_DATABASE_TIMER;
    aclk_database_enq_cmd_noblock(wc, &cmd);

    time_t now =  now_realtime_sec();

    if (wc->cleanup_after && wc->cleanup_after < now) {
        cmd.opcode = ACLK_DATABASE_CLEANUP;
        if (!aclk_database_enq_cmd_noblock(wc, &cmd))
            wc->cleanup_after += ACLK_DATABASE_CLEANUP_INTERVAL;
    }

    if (aclk_connected) {
        if (wc->alert_updates && !wc->pause_alert_updates) {
            cmd.opcode = ACLK_DATABASE_PUSH_ALERT;
            cmd.count = ACLK_MAX_ALERT_UPDATES;
            aclk_database_enq_cmd_noblock(wc, &cmd);
        }
    }
}

static void aclk_database_worker(void *arg)
{
    service_register(SERVICE_THREAD_TYPE_EVENT_LOOP, NULL, NULL, NULL, true);
    worker_register("ACLKSYNC");
    worker_register_job_name(ACLK_DATABASE_NOOP,                 "noop");
    worker_register_job_name(ACLK_DATABASE_ORPHAN_HOST,          "node orphan");
    worker_register_job_name(ACLK_DATABASE_ALARM_HEALTH_LOG,     "alert log");
    worker_register_job_name(ACLK_DATABASE_CLEANUP,              "cleanup");
    worker_register_job_name(ACLK_DATABASE_DELETE_HOST,          "node delete");
    worker_register_job_name(ACLK_DATABASE_NODE_INFO,            "node info");
    worker_register_job_name(ACLK_DATABASE_NODE_STATE,           "node state");
    worker_register_job_name(ACLK_DATABASE_NODE_COLLECTORS,      "node collectors");
    worker_register_job_name(ACLK_DATABASE_PUSH_ALERT,           "alert push");
    worker_register_job_name(ACLK_DATABASE_PUSH_ALERT_CONFIG,    "alert conf push");
    worker_register_job_name(ACLK_DATABASE_PUSH_ALERT_SNAPSHOT,  "alert snapshot");
    worker_register_job_name(ACLK_DATABASE_QUEUE_REMOVED_ALERTS, "alerts check");
    worker_register_job_name(ACLK_DATABASE_TIMER,                "timer");

    struct aclk_database_worker_config *wc = arg;
    uv_loop_t *loop;
    int ret;
    enum aclk_database_opcode opcode;
    uv_timer_t timer_req;
    struct aclk_database_cmd cmd;

    char threadname[NETDATA_THREAD_NAME_MAX+1];
    if (wc->host)
        snprintfz(threadname, NETDATA_THREAD_NAME_MAX, "ACLK[%s]", rrdhost_hostname(wc->host));
    else {
        snprintfz(threadname, NETDATA_THREAD_NAME_MAX, "ACLK[%s]", wc->uuid_str);
        threadname[11] = '\0';
    }
    uv_thread_set_name_np(wc->thread, threadname);

    loop = wc->loop = mallocz(sizeof(uv_loop_t));
    ret = uv_loop_init(loop);
    if (ret) {
        error("uv_loop_init(): %s", uv_strerror(ret));
        goto error_after_loop_init;
    }
    loop->data = wc;

    ret = uv_async_init(wc->loop, &wc->async, async_cb);
    if (ret) {
        error("uv_async_init(): %s", uv_strerror(ret));
        goto error_after_async_init;
    }
    wc->async.data = wc;

    ret = uv_timer_init(loop, &timer_req);
    if (ret) {
        error("uv_timer_init(): %s", uv_strerror(ret));
        goto error_after_timer_init;
    }
    timer_req.data = wc;
    fatal_assert(0 == uv_timer_start(&timer_req, timer_cb, TIMER_PERIOD_MS, TIMER_PERIOD_MS));

    wc->node_info_send = 1;
    info("Starting ACLK sync thread for host %s -- scratch area %lu bytes", wc->host_guid, (unsigned long int) sizeof(*wc));

    memset(&cmd, 0, sizeof(cmd));

    wc->startup_time = now_realtime_sec();
    wc->cleanup_after = wc->startup_time + ACLK_DATABASE_CLEANUP_FIRST;

    debug(D_ACLK_SYNC,"Node %s reports pending message count = %u", wc->node_id, wc->chart_payload_count);

    while (likely(!netdata_exit)) {
        worker_is_idle();
        uv_run(loop, UV_RUN_DEFAULT);

        /* wait for commands */
        do {
            cmd = aclk_database_deq_cmd(wc);

            if (netdata_exit)
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
                    debug(D_ACLK_SYNC, "Database cleanup for %s", wc->host_guid);

                    if (wc->startup_time + ACLK_DATABASE_CLEANUP_FIRST + 2 < now_realtime_sec() && claimed() && aclk_connected) {
                        cmd.opcode = ACLK_DATABASE_NODE_INFO;
                        cmd.completion = NULL;
                        (void) aclk_database_enq_cmd_noblock(wc, &cmd);
                    }

                    sql_maint_aclk_sync_database(wc, cmd);
                    if (wc->host == localhost)
                        sql_check_aclk_table_list(wc);
                    break;

                case ACLK_DATABASE_DELETE_HOST:
                    debug(D_ACLK_SYNC,"Cleaning ACLK tables for %s", (char *) cmd.data);
                    sql_delete_aclk_table_list(wc, cmd);
                    break;

// ALERTS
                case ACLK_DATABASE_PUSH_ALERT_CONFIG:
                    debug(D_ACLK_SYNC,"Pushing chart config info to the cloud for %s", wc->host_guid);
                    aclk_push_alert_config_event(wc, cmd);
                    break;
                case ACLK_DATABASE_PUSH_ALERT:
                    debug(D_ACLK_SYNC, "Pushing alert info to the cloud for %s", wc->host_guid);
                    aclk_push_alert_event(wc, cmd);
                    break;
                case ACLK_DATABASE_ALARM_HEALTH_LOG:
                    debug(D_ACLK_SYNC, "Pushing alarm health log to the cloud for %s", wc->host_guid);
                    aclk_push_alarm_health_log(wc, cmd);
                    break;
                case ACLK_DATABASE_PUSH_ALERT_SNAPSHOT:
                    debug(D_ACLK_SYNC, "Pushing alert snapshot to the cloud for node %s", wc->host_guid);
                    aclk_push_alert_snapshot_event(wc, cmd);
                    break;
                case ACLK_DATABASE_QUEUE_REMOVED_ALERTS:
                    debug(D_ACLK_SYNC, "Queueing removed alerts for node %s", wc->host_guid);
                    sql_process_queue_removed_alerts_to_aclk(wc, cmd);
                    break;
// NODE OPERATIONS
                case ACLK_DATABASE_NODE_STATE:
                    debug(D_ACLK_SYNC,"Sending state update for %s", wc->uuid_str);
                    if (wc->host) {
                        RRDHOST *host = wc->host;
                        int live = (host == localhost || host->receiver || !(rrdhost_flag_check(host, RRDHOST_FLAG_ORPHAN))) ? 1 : 0;
                        aclk_host_state_update(wc->host, live);
                    }
                    break;
                case ACLK_DATABASE_NODE_INFO:
                    debug(D_ACLK_SYNC,"Sending node info for %s", wc->uuid_str);
                    sql_build_node_info(wc, cmd);
                    break;
                case ACLK_DATABASE_NODE_COLLECTORS:
                    debug(D_ACLK_SYNC,"Sending node collectors info for %s", wc->uuid_str);
                    sql_build_node_collectors(wc);
                    break;
// NODE_INSTANCE DETECTION
                case ACLK_DATABASE_ORPHAN_HOST:
                    wc->host = NULL;
                    wc->is_orphan = 1;
                    aclk_add_worker_thread(wc);
                    break;
                case ACLK_DATABASE_TIMER:
                    if (unlikely(localhost && !wc->host && !wc->is_orphan)) {
                        if (claimed()) {
                            wc->host = rrdhost_find_by_guid(wc->host_guid);
                            if (wc->host) {
                                info("HOST %s (%s) detected as active", rrdhost_hostname(wc->host), wc->host_guid);
                                snprintfz(threadname, NETDATA_THREAD_NAME_MAX, "ACLK[%s]", rrdhost_hostname(wc->host));
                                uv_thread_set_name_np(wc->thread, threadname);
                                wc->host->dbsync_worker = wc;
                                if (unlikely(!wc->hostname))
                                    wc->hostname = strdupz(rrdhost_hostname(wc->host));
                                aclk_del_worker_thread(wc);
                                wc->node_info_send = 1;
                            }
                        }
                    }
                    if (wc->node_info_send && localhost && claimed() && aclk_connected) {
                        cmd.opcode = ACLK_DATABASE_NODE_INFO;
                        cmd.completion = NULL;
                        wc->node_info_send = aclk_database_enq_cmd_noblock(wc, &cmd);
                    }
                    if (wc->node_collectors_send && wc->node_collectors_send + 30 < now_realtime_sec()) {
                        cmd.opcode = ACLK_DATABASE_NODE_COLLECTORS;
                        cmd.completion = NULL;
                        wc->node_collectors_send = aclk_database_enq_cmd_noblock(wc, &cmd);
                    }
                    if (localhost == wc->host)
                        (void) sqlite3_wal_checkpoint(db_meta, NULL);
                    break;
                default:
                    debug(D_ACLK_SYNC, "%s: default.", __func__);
                    break;
            }
            if (cmd.completion)
                aclk_complete(cmd.completion);
        } while (opcode != ACLK_DATABASE_NOOP);
    }

    if (!uv_timer_stop(&timer_req))
        uv_close((uv_handle_t *)&timer_req, NULL);

    /* cleanup operations of the event loop */
    //info("Shutting down ACLK sync event loop for %s", wc->host_guid);

    /*
     * uv_async_send after uv_close does not seem to crash in linux at the moment,
     * it is however undocumented behaviour we need to be aware if this becomes
     * an issue in the future.
     */
    uv_close((uv_handle_t *)&wc->async, NULL);
    uv_run(loop, UV_RUN_DEFAULT);

    info("Shutting down ACLK sync event loop complete for host %s", wc->host_guid);
    /* TODO: don't let the API block by waiting to enqueue commands */
    uv_cond_destroy(&wc->cmd_cond);

    int rc;
    do {
        rc = uv_loop_close(loop);
    } while (rc != UV_EBUSY);

    freez(loop);

    rrd_rdlock();
    if (likely(wc->host))
        wc->host->dbsync_worker = NULL;
    freez(wc->hostname);
    freez(wc);
    rrd_unlock();

    worker_unregister();
    return;

error_after_timer_init:
    uv_close((uv_handle_t *)&wc->async, NULL);
error_after_async_init:
    fatal_assert(0 == uv_loop_close(loop));
error_after_loop_init:
    freez(loop);
    worker_unregister();
}
#endif

// -------------------------------------------------------------

void sql_create_aclk_table(RRDHOST *host, uuid_t *host_uuid, uuid_t *node_id)
{
#ifdef ENABLE_ACLK
    char uuid_str[GUID_LEN + 1];
    char host_guid[GUID_LEN + 1];

    uuid_unparse_lower_fix(host_uuid, uuid_str);

    if (aclk_worker_thread_exists(uuid_str))
        return;

    uuid_unparse_lower(*host_uuid, host_guid);

    BUFFER *sql = buffer_create(ACLK_SYNC_QUERY_SIZE, &netdata_buffers_statistics.buffers_sqlite);

    buffer_sprintf(sql, TABLE_ACLK_ALERT, uuid_str);
    db_execute(buffer_tostring(sql));
    buffer_flush(sql);

    buffer_sprintf(sql, INDEX_ACLK_ALERT, uuid_str, uuid_str);
    db_execute(buffer_tostring(sql));

    buffer_free(sql);

    if (likely(host) && unlikely(host->dbsync_worker))
        return;

    struct aclk_database_worker_config *wc = callocz(1, sizeof(struct aclk_database_worker_config));
    if (node_id && !uuid_is_null(*node_id))
        uuid_unparse_lower(*node_id, wc->node_id);
    if (likely(host)) {
        host->dbsync_worker = (void *)wc;
        wc->hostname = strdupz(rrdhost_hostname(host));
        if (node_id && !host->node_id) {
            host->node_id = mallocz(sizeof(*host->node_id));
            uuid_copy(*host->node_id, *node_id);
        }
    }
    else
        wc->hostname = get_hostname_by_node_id(wc->node_id);
    wc->host = host;
    strcpy(wc->uuid_str, uuid_str);
    strcpy(wc->host_guid, host_guid);
    wc->alert_updates = 0;
    aclk_database_init_cmd_queue(wc);
    aclk_add_worker_thread(wc);
    fatal_assert(0 == uv_thread_create(&(wc->thread), aclk_database_worker, wc));
#else
    UNUSED(host);
    UNUSED(host_uuid);
    UNUSED(node_id);
#endif
}