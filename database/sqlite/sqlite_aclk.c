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
    /* FIFO command queue */
    uv_mutex_t cmd_mutex;
    uv_cond_t cmd_cond;
    bool initialized;
    volatile unsigned queue_size;
    struct aclk_database_cmdqueue cmd_queue;
} aclk_sync_config = { 0 };


void sanity_check(void) {
    // make sure the compiler will stop on misconfigurations
    BUILD_BUG_ON(WORKER_UTILIZATION_MAX_JOB_TYPES < ACLK_MAX_ENUMERATIONS_DEFINED);
}

#define DB_ACLK_VERSION 1

const char *database_aclk_config[] = {
    "VACUUM;",
    NULL
};

sqlite3 *db_aclk = NULL;

/*
 * Initialize the SQLite database
 * Return 0 on success
 */

#ifndef ENABLE_ACLK
int sql_init_aclk_database(int memory __maybe_unused)
{
    return 0;
}
#else
int sql_init_aclk_database(int memory)
{
    char sqlite_database[FILENAME_MAX + 1];
    int rc;

    if (likely(!memory))
        snprintfz(sqlite_database, FILENAME_MAX, "%s/netdata-aclk.db", netdata_configured_cache_dir);
    else
        strcpy(sqlite_database, ":memory:");

    rc = sqlite3_open(sqlite_database, &db_aclk);
    if (rc != SQLITE_OK) {
        error_report("Failed to initialize database at %s, due to \"%s\"", sqlite_database, sqlite3_errstr(rc));
        sqlite3_close(db_aclk);
        db_aclk = NULL;
        return 1;
    }

    info("SQLite aclk database %s initialization", sqlite_database);

    int target_version = DB_ACLK_VERSION;
    if (likely(!memory))
        target_version = perform_aclk_database_migration(db_health, DB_ACLK_VERSION);

    if (configure_database_params(db_aclk, target_version))
        return 1;

    if (init_database_batch(db_aclk, &database_aclk_config[0]))
        return 1;

    if (attach_database(db_aclk, "netdata-meta.db", "meta"))
        return 1;

    if (attach_database(db_aclk, "netdata-health.db", "health"))
        return 1;

    info("SQLite aclk database initialization completed");

    return 0;
}
#endif

#define SQL_SELECT_HOST_BY_NODE_ID  "select host_id from node_instance where node_id = @node_id;"

int get_host_id(uuid_t *node_id, uuid_t *host_id)
{
    static __thread sqlite3_stmt *res = NULL;
    int rc;

    if (unlikely(!db_meta)) {
        if (default_rrd_memory_mode == RRD_MEMORY_MODE_DBENGINE)
            error_report("Database has not been initialized");
        return 1;
    }

    if (unlikely(!res)) {
        rc = prepare_statement(db_meta, SQL_SELECT_HOST_BY_NODE_ID, &res);
        if (unlikely(rc != SQLITE_OK)) {
            error_report("Failed to prepare statement to select node instance information for a node");
            return 1;
        }
    }

    rc = sqlite3_bind_blob(res, 1, node_id, sizeof(*node_id), SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to bind host_id parameter to select node instance information");
        goto failed;
    }

    rc = sqlite3_step_monitored(res);
    if (likely(rc == SQLITE_ROW && host_id))
        uuid_copy(*host_id, *((uuid_t *) sqlite3_column_blob(res, 0)));

failed:
    if (unlikely(sqlite3_reset(res) != SQLITE_OK))
        error_report("Failed to reset the prepared statement when selecting node instance information");

    return (rc == SQLITE_ROW) ? 0 : -1;
}

#define SQL_SELECT_NODE_ID  "select node_id from node_instance where host_id = @host_id and node_id not null;"

int get_node_id(uuid_t *host_id, uuid_t *node_id)
{
    sqlite3_stmt *res = NULL;
    int rc;

    if (unlikely(!db_meta)) {
        if (default_rrd_memory_mode == RRD_MEMORY_MODE_DBENGINE)
            error_report("Database has not been initialized");
        return 1;
    }

    rc = sqlite3_prepare_v2(db_meta, SQL_SELECT_NODE_ID, -1, &res, 0);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to prepare statement to select node instance information for a host");
        return 1;
    }

    rc = sqlite3_bind_blob(res, 1, host_id, sizeof(*host_id), SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to bind host_id parameter to select node instance information");
        goto failed;
    }

    rc = sqlite3_step_monitored(res);
    if (likely(rc == SQLITE_ROW && node_id))
        uuid_copy(*node_id, *((uuid_t *) sqlite3_column_blob(res, 0)));

failed:
    if (unlikely(sqlite3_finalize(res) != SQLITE_OK))
        error_report("Failed to finalize the prepared statement when selecting node instance information");

    return (rc == SQLITE_ROW) ? 0 : -1;
}

#define SQL_INVALIDATE_NODE_INSTANCES "update node_instance set node_id = NULL where exists " \
    "(select host_id from node_instance where host_id = @host_id and (@claim_id is null or claim_id <> @claim_id));"

void invalidate_node_instances(uuid_t *host_id, uuid_t *claim_id)
{
    sqlite3_stmt *res = NULL;
    int rc;

    if (unlikely(!db_meta)) {
        if (default_rrd_memory_mode == RRD_MEMORY_MODE_DBENGINE)
            error_report("Database has not been initialized");
        return;
    }

    rc = sqlite3_prepare_v2(db_meta, SQL_INVALIDATE_NODE_INSTANCES, -1, &res, 0);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to prepare statement to invalidate node instance ids");
        return;
    }

    rc = sqlite3_bind_blob(res, 1, host_id, sizeof(*host_id), SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to bind host_id parameter to invalidate node instance information");
        goto failed;
    }

    if (claim_id)
        rc = sqlite3_bind_blob(res, 2, claim_id, sizeof(*claim_id), SQLITE_STATIC);
    else
        rc = sqlite3_bind_null(res, 2);

    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to bind claim_id parameter to invalidate node instance information");
        goto failed;
    }

    rc = execute_insert(res);
    if (unlikely(rc != SQLITE_DONE))
        error_report("Failed to invalidate node instance information, rc = %d", rc);

failed:
    if (unlikely(sqlite3_finalize(res) != SQLITE_OK))
        error_report("Failed to finalize the prepared statement when invalidating node instance information");
}

#define SQL_GET_NODE_INSTANCE_LIST "select ni.node_id, ni.host_id, h.hostname " \
    "from node_instance ni, host h where ni.host_id = h.host_id;"

struct node_instance_list *get_node_list(void)
{
    struct node_instance_list *node_list = NULL;
    sqlite3_stmt *res = NULL;
    int rc;

    if (unlikely(!db_meta)) {
        if (default_rrd_memory_mode == RRD_MEMORY_MODE_DBENGINE)
            error_report("Database has not been initialized");
        return NULL;
    }

    rc = sqlite3_prepare_v2(db_meta, SQL_GET_NODE_INSTANCE_LIST, -1, &res, 0);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to prepare statement to get node instance information");
        return NULL;
    };

    int row = 0;
    char host_guid[37];
    while (sqlite3_step_monitored(res) == SQLITE_ROW)
        row++;

    if (sqlite3_reset(res) != SQLITE_OK) {
        error_report("Failed to reset the prepared statement while fetching node instance information");
        goto failed;
    }
    node_list = callocz(row + 1, sizeof(*node_list));
    int max_rows = row;
    row = 0;
    // TODO: Check to remove lock
    rrd_rdlock();
    while (sqlite3_step_monitored(res) == SQLITE_ROW) {
        if (sqlite3_column_bytes(res, 0) == sizeof(uuid_t))
            uuid_copy(node_list[row].node_id, *((uuid_t *)sqlite3_column_blob(res, 0)));
        if (sqlite3_column_bytes(res, 1) == sizeof(uuid_t)) {
            uuid_t *host_id = (uuid_t *)sqlite3_column_blob(res, 1);
            uuid_copy(node_list[row].host_id, *host_id);
            node_list[row].queryable = 1;
            uuid_unparse_lower(*host_id, host_guid);
            RRDHOST *host = rrdhost_find_by_guid(host_guid);
            node_list[row].live = host && (host == localhost || host->receiver) ? 1 : 0;
            node_list[row].hops = (host && host->system_info) ? host->system_info->hops :
                                  uuid_compare(*host_id, localhost->host_uuid) ? 1 : 0;
            node_list[row].hostname =
                sqlite3_column_bytes(res, 2) ? strdupz((char *)sqlite3_column_text(res, 2)) : NULL;
        }
        row++;
        if (row == max_rows)
            break;
    }
    rrd_unlock();

failed:
    if (unlikely(sqlite3_finalize(res) != SQLITE_OK))
        error_report("Failed to finalize the prepared statement when fetching node instance information");

    return node_list;
};

static inline void set_host_node_id(RRDHOST *host, uuid_t *node_id)
{
    if (unlikely(!host))
        return;

    if (unlikely(!node_id)) {
        freez(host->node_id);
        host->node_id = NULL;
        return;
    }

    if (unlikely(!host->node_id))
        host->node_id = mallocz(sizeof(*host->node_id));
    uuid_copy(*(host->node_id), *node_id);
    sql_create_aclk_table(host, &host->host_uuid, node_id);
}

#define SQL_UPDATE_NODE_ID  "update node_instance set node_id = @node_id where host_id = @host_id;"
int update_node_id(uuid_t *host_id, uuid_t *node_id)
{
    sqlite3_stmt *res = NULL;
    RRDHOST *host = NULL;
    int rc = 2;

    if (unlikely(!db_meta)) {
        if (default_rrd_memory_mode == RRD_MEMORY_MODE_DBENGINE)
            error_report("Database has not been initialized");
        return 1;
    }

    rc = sqlite3_prepare_v2(db_meta, SQL_UPDATE_NODE_ID, -1, &res, 0);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to prepare statement to store node instance information");
        return 1;
    }

    rc = sqlite3_bind_blob(res, 1, node_id, sizeof(*node_id), SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to bind host_id parameter to store node instance information");
        goto failed;
    }

    rc = sqlite3_bind_blob(res, 2, host_id, sizeof(*host_id), SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to bind host_id parameter to store node instance information");
        goto failed;
    }

    rc = execute_insert(res);
    if (unlikely(rc != SQLITE_DONE))
        error_report("Failed to store node instance information, rc = %d", rc);
    rc = sqlite3_changes(db_meta);

    char host_guid[GUID_LEN + 1];
    uuid_unparse_lower(*host_id, host_guid);
    rrd_wrlock();
    host = rrdhost_find_by_guid(host_guid);
    if (likely(host))
        set_host_node_id(host, node_id);
    rrd_unlock();

failed:
    if (unlikely(sqlite3_finalize(res) != SQLITE_OK))
        error_report("Failed to finalize the prepared statement when storing node instance information");

    return rc - 1;
}

#define SQL_GET_HOST_NODE_ID "select node_id from node_instance where host_id = @host_id;"

void sql_load_node_id(RRDHOST *host)
{
    static __thread sqlite3_stmt *res = NULL;
    int rc;

    if (unlikely(!db_meta)) {
        if (default_rrd_memory_mode == RRD_MEMORY_MODE_DBENGINE)
            error_report("Database has not been initialized");
        return;
    }

    if (unlikely(!res)) {
        rc = prepare_statement(db_meta, SQL_GET_HOST_NODE_ID, &res);
        if (unlikely(rc != SQLITE_OK)) {
            error_report("Failed to prepare statement to fetch node id");
            return;
        };
    }

    rc = sqlite3_bind_blob(res, 1, &host->host_uuid, sizeof(host->host_uuid), SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to bind host_id parameter to load node instance information");
        goto failed;
    }

    rc = sqlite3_step_monitored(res);
    if (likely(rc == SQLITE_ROW)) {
        if (likely(sqlite3_column_bytes(res, 0) == sizeof(uuid_t)))
            set_host_node_id(host, (uuid_t *)sqlite3_column_blob(res, 0));
        else
            set_host_node_id(host, NULL);
    }

failed:
    if (unlikely(sqlite3_reset(res) != SQLITE_OK))
        error_report("Failed to reset the prepared statement when loading node instance information");
};


#ifdef ENABLE_ACLK
#define SQL_SELECT_HOST_BY_UUID  "SELECT host_id FROM host WHERE host_id = @host_id;"

static int is_host_available(uuid_t *host_id)
{
    sqlite3_stmt *res = NULL;
    int rc;

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

int aclk_database_enq_cmd_noblock(struct aclk_database_cmd *cmd)
{
    unsigned queue_size;

    /* wait for free space in queue */
    uv_mutex_lock(&aclk_sync_config.cmd_mutex);
    if ((queue_size = aclk_sync_config.queue_size) == ACLK_DATABASE_CMD_Q_MAX_SIZE) {
        uv_mutex_unlock(&aclk_sync_config.cmd_mutex);
        return 1;
    }

    fatal_assert(queue_size < ACLK_DATABASE_CMD_Q_MAX_SIZE);
    /* enqueue command */
    aclk_sync_config.cmd_queue.cmd_array[aclk_sync_config.cmd_queue.tail] = *cmd;
    aclk_sync_config.cmd_queue.tail = aclk_sync_config.cmd_queue.tail != ACLK_DATABASE_CMD_Q_MAX_SIZE - 1 ?
                                          aclk_sync_config.cmd_queue.tail + 1 : 0;
    aclk_sync_config.queue_size = queue_size + 1;
    uv_mutex_unlock(&aclk_sync_config.cmd_mutex);
    return 0;
}

static void aclk_database_enq_cmd(struct aclk_database_cmd *cmd)
{
    unsigned queue_size;

    /* wait for free space in queue */
    uv_mutex_lock(&aclk_sync_config.cmd_mutex);
    while ((queue_size = aclk_sync_config.queue_size) == ACLK_DATABASE_CMD_Q_MAX_SIZE) {
        uv_cond_wait(&aclk_sync_config.cmd_cond, &aclk_sync_config.cmd_mutex);
    }
    fatal_assert(queue_size < ACLK_DATABASE_CMD_Q_MAX_SIZE);
    /* enqueue command */
    aclk_sync_config.cmd_queue.cmd_array[aclk_sync_config.cmd_queue.tail] = *cmd;
    aclk_sync_config.cmd_queue.tail = aclk_sync_config.cmd_queue.tail != ACLK_DATABASE_CMD_Q_MAX_SIZE - 1 ?
                                          aclk_sync_config.cmd_queue.tail + 1 : 0;
    aclk_sync_config.queue_size = queue_size + 1;
    uv_mutex_unlock(&aclk_sync_config.cmd_mutex);

    /* wake up event loop */
    int rc = uv_async_send(&aclk_sync_config.async);
    if (unlikely(rc))
        debug(D_ACLK_SYNC, "Failed to wake up event loop");
}
#endif

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
    int *number_of_chidren = data;
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
        , (int32_t) (argv[IDX_UTC_OFFSET] ? str2uint32_t(argv[IDX_UTC_OFFSET], NULL) : 0)
        , (const char *) argv[IDX_TAGS]
        , (const char *) (argv[IDX_PROGRAM_NAME] ? argv[IDX_PROGRAM_NAME] : "unknown")
        , (const char *) (argv[IDX_PROGRAM_VERSION] ? argv[IDX_PROGRAM_VERSION] : "unknown")
        , argv[IDX_UPDATE_EVERY] ? str2i(argv[IDX_UPDATE_EVERY]) : 1
        , argv[IDX_ENTRIES] ? str2i(argv[IDX_ENTRIES]) : 0
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

    (*number_of_chidren)++;

#ifdef NETDATA_INTERNAL_CHECKS
    char node_str[UUID_STR_LEN] = "<none>";
    if (likely(host->node_id))
        uuid_unparse_lower(*host->node_id, node_str);
    internal_error(true, "Adding archived host \"%s\" with GUID \"%s\" node id = \"%s\"", rrdhost_hostname(host), host->machine_guid, node_str);
#endif
    return 0;
}
#ifdef ENABLE_ACLK

static struct aclk_database_cmd aclk_database_deq_cmd(void)
{
    struct aclk_database_cmd ret;
    unsigned queue_size;

    uv_mutex_lock(&aclk_sync_config.cmd_mutex);
    queue_size = aclk_sync_config.queue_size;
    if (queue_size == 0) {
        memset(&ret, 0, sizeof(ret));
        ret.opcode = ACLK_DATABASE_NOOP;
        ret.completion = NULL;

    } else {
        /* dequeue command */
        ret = aclk_sync_config.cmd_queue.cmd_array[aclk_sync_config.cmd_queue.head];
        if (queue_size == 1) {
            aclk_sync_config.cmd_queue.head = aclk_sync_config.cmd_queue.tail = 0;
        } else {
            aclk_sync_config.cmd_queue.head = aclk_sync_config.cmd_queue.head != ACLK_DATABASE_CMD_Q_MAX_SIZE - 1 ?
                                                  aclk_sync_config.cmd_queue.head + 1 : 0;
        }
        aclk_sync_config.queue_size = queue_size - 1;
        /* wake up producers */
        uv_cond_signal(&aclk_sync_config.cmd_cond);
    }
    uv_mutex_unlock(&aclk_sync_config.cmd_mutex);

    return ret;
}

// OPCODE: ACLK_DATABASE_DELETE_HOST
static void sql_delete_aclk_table_list(char *host_guid)
{
    char uuid_str[UUID_STR_LEN];
    char host_uuid_str[UUID_STR_LEN];

    int rc;
    uuid_t host_uuid;

    if (unlikely(!host_guid))
        return;

    rc = uuid_parse(host_guid, host_uuid);
    freez(host_guid);
    if (rc || is_host_available(&host_uuid))
        return;

    uuid_unparse_lower(host_uuid, host_uuid_str);
    uuid_unparse_lower_fix(&host_uuid, uuid_str);

    info("ACLKSYNC: Host with UUID %s does NOT exist dropping ACLK sync tables", host_uuid_str);

    sqlite3_stmt *res = NULL;
    BUFFER *sql = buffer_create(ACLK_SYNC_QUERY_SIZE, &netdata_buffers_statistics.buffers_sqlite);

    buffer_sprintf(sql,"SELECT 'drop '||type||' IF EXISTS '||name||';' FROM sqlite_schema " \
                        "WHERE name LIKE 'aclk_%%_%s' AND type IN ('table', 'trigger', 'index');", uuid_str);

    rc = sqlite3_prepare_v2(db_aclk, buffer_tostring(sql), -1, &res, 0);
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

    rc = db_execute(db_aclk,buffer_tostring(sql));
    if (unlikely(rc))
        error("Failed to drop unused ACLK tables for host with UUID %s", host_uuid_str);

fail:
    buffer_free(sql);
}

static int sql_check_aclk_table(void *data __maybe_unused, int argc __maybe_unused, char **argv __maybe_unused, char **column __maybe_unused)
{
    debug(D_ACLK_SYNC,"Scheduling aclk sync table check for node %s", (char *) argv[0]);
    struct aclk_database_cmd cmd;
    memset(&cmd, 0, sizeof(cmd));
    cmd.opcode = ACLK_DATABASE_DELETE_HOST;
    cmd.param[0] = strdupz((char *) argv[0]);
    aclk_database_enq_cmd_noblock(&cmd);
    return 0;
}

#define SQL_SELECT_ACLK_ACTIVE_LIST "SELECT REPLACE(SUBSTR(name,12),'_','-') FROM sqlite_schema WHERE name LIKE 'aclk_alert_%' AND type IN ('table');"

static void sql_check_aclk_table_list(void)
{
    char *err_msg = NULL;
    int rc = sqlite3_exec_monitored(db_meta, SQL_SELECT_ACLK_ACTIVE_LIST, sql_check_aclk_table, NULL, &err_msg);
    if (rc != SQLITE_OK) {
        error_report("Query failed when trying to check for nodes that do not exist, %s", err_msg);
        sqlite3_free(err_msg);
    }
}

#define SQL_ALERT_CLEANUP "DELETE FROM aclk_alert_%s WHERE date_submitted IS NOT NULL AND CAST(date_cloud_ack AS INT) < unixepoch()-%d;"

static int sql_maint_aclk_sync_database(void *data __maybe_unused, int argc __maybe_unused, char **argv, char **column __maybe_unused)
{
    char sql[512];
    snprintfz(sql,511, SQL_ALERT_CLEANUP, (char *) argv[0], ACLK_DELETE_ACK_ALERTS_INTERNAL);
    if (unlikely(db_execute(db_aclk, sql)))
        error_report("Failed to clean stale ACLK alert entries");
    return 0;
}


#define SQL_SELECT_ACLK_ALERT_LIST "SELECT SUBSTR(name,12) FROM sqlite_schema WHERE name LIKE 'aclk_alert_%' AND type IN ('table');"

static void sql_maint_aclk_sync_database_all(void)
{
    char *err_msg = NULL;
    debug(D_ACLK_SYNC,"Cleaning tables for nodes that do not exist");
    int rc = sqlite3_exec_monitored(db_aclk, SQL_SELECT_ACLK_ALERT_LIST, sql_maint_aclk_sync_database, NULL, &err_msg);
    if (rc != SQLITE_OK) {
        error_report("Query failed when trying to check for obsolete ACLK sync tables, %s", err_msg);
        sqlite3_free(err_msg);
    }
}

static int aclk_config_parameters(void *data __maybe_unused, int argc __maybe_unused, char **argv, char **column __maybe_unused)
{
    char uuid_str[GUID_LEN + 1];
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

    time_t now =  now_realtime_sec();

    if (config->cleanup_after && config->cleanup_after < now) {
        cmd.opcode = ACLK_DATABASE_CLEANUP;
        if (!aclk_database_enq_cmd_noblock(&cmd))
            config->cleanup_after += ACLK_DATABASE_CLEANUP_INTERVAL;
    }

    if (aclk_connected) {
        cmd.opcode = ACLK_DATABASE_PUSH_ALERT;
        aclk_database_enq_cmd_noblock(&cmd);

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

    info("Starting ACLK synchronization thread");

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
                    struct aclk_sync_host_config *ahc = host->aclk_sync_host_config;
                    if (unlikely(!ahc))
                        sql_create_aclk_table(host, &host->host_uuid, host->node_id);
                    aclk_host_state_update(host, live);
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
                    debug(D_ACLK_SYNC, "%s: default.", __func__);
                    break;
            }
            if (cmd.completion)
                completion_mark_complete(cmd.completion);
        } while (opcode != ACLK_DATABASE_NOOP);
    }

    if (!uv_timer_stop(&config->timer_req))
        uv_close((uv_handle_t *)&config->timer_req, NULL);

    uv_close((uv_handle_t *)&config->async, NULL);
//    uv_close((uv_handle_t *)&config->async_exit, NULL);
    uv_cond_destroy(&config->cmd_cond);
    (void) uv_loop_close(loop);

    worker_unregister();
    service_exits();
    info("ACLK SYNC: Shutting down ACLK synchronization event loop");
}

static void aclk_synchronization_init(void)
{
    aclk_sync_config.cmd_queue.head = aclk_sync_config.cmd_queue.tail = 0;
    aclk_sync_config.queue_size = 0;
    fatal_assert(0 == uv_cond_init(&aclk_sync_config.cmd_cond));
    fatal_assert(0 == uv_mutex_init(&aclk_sync_config.cmd_mutex));

    fatal_assert(0 == uv_thread_create(&aclk_sync_config.thread, aclk_synchronization, &aclk_sync_config));
}
#endif

// -------------------------------------------------------------

void sql_create_aclk_table(RRDHOST *host __maybe_unused, uuid_t *host_uuid __maybe_unused, uuid_t *node_id __maybe_unused)
{
#ifdef ENABLE_ACLK
    char uuid_str[GUID_LEN + 1];
    char host_guid[GUID_LEN + 1];
    int rc;

    uuid_unparse_lower_fix(host_uuid, uuid_str);
    uuid_unparse_lower(*host_uuid, host_guid);

    char sql[ACLK_SYNC_QUERY_SIZE];

    snprintfz(sql, ACLK_SYNC_QUERY_SIZE-1, TABLE_ACLK_ALERT, uuid_str);
    rc = db_execute(db_aclk, sql);
    if (unlikely(rc))
        error_report("Failed to create ACLK alert table for host %s", host ? rrdhost_hostname(host) : host_guid);
    else {
        snprintfz(sql, ACLK_SYNC_QUERY_SIZE -1, INDEX_ACLK_ALERT, uuid_str, uuid_str);
        rc = db_execute(db_aclk, sql);
        if (unlikely(rc))
            error_report("Failed to create ACLK alert table index for host %s", host ? string2str(host->hostname) : host_guid);
    }
    if (likely(host) && unlikely(host->aclk_sync_host_config))
        return;
    snprintfz(sql, ACLK_SYNC_QUERY_SIZE-1, INDEX_ACLK_ALERT, uuid_str, uuid_str);
    rc = db_execute(db_aclk, sql);

    if (unlikely(!host))
        return;

    struct aclk_sync_host_config *wc = callocz(1, sizeof(struct aclk_sync_host_config));
    if (node_id && !uuid_is_null(*node_id))
        uuid_unparse_lower(*node_id, wc->node_id);

    host->aclk_sync_host_config = (void *)wc;
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

#define SQL_FETCH_ALL_HOSTS "SELECT host_id, hostname, registry_hostname, update_every, os, " \
    "timezone, tags, hops, memory_mode, abbrev_timezone, utc_offset, program_name, " \
    "program_version, entries, health_enabled FROM host WHERE hops >0;"

#define SQL_FETCH_ALL_INSTANCES "SELECT ni.host_id, ni.node_id FROM host h, node_instance ni " \
                                "WHERE h.host_id = ni.host_id AND ni.node_id IS NOT NULL; "
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
    int number_of_children = 0;
    rc = sqlite3_exec_monitored(db_meta, SQL_FETCH_ALL_HOSTS, create_host_callback, &number_of_children, &err_msg);

    if (rc != SQLITE_OK) {
        error_report("SQLite error when loading archived hosts, rc = %d (%s)", rc, err_msg);
        sqlite3_free(err_msg);
    }

    info("Created %d archived hosts", number_of_children);
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

    info("ACLK sync initialization completed");
#endif
}

// Public

#ifdef ENABLE_ACLK
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
#endif

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
