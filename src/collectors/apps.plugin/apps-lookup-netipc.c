// SPDX-License-Identifier: GPL-3.0-or-later

#include "apps-lookup-netipc.h"
#include "apps-cgroups-lookup-client.h"

#ifdef OS_LINUX

#include "libnetdata/netipc/netipc_netdata.h"

struct apps_lookup_label_view_ctx {
    nipc_lookup_label_view_t *views;
    uint16_t count;
};

static nipc_managed_server_t apps_lookup_server;
static ND_THREAD *apps_lookup_netipc_thread = NULL;

static uint64_t apps_lookup_requests_responded = 0;
static uint64_t apps_lookup_requests_error = 0;
static uint64_t apps_lookup_duration_le_1ms = 0;
static uint64_t apps_lookup_duration_le_5ms = 0;
static uint64_t apps_lookup_duration_le_10ms = 0;
static uint64_t apps_lookup_duration_le_50ms = 0;
static uint64_t apps_lookup_duration_le_100ms = 0;
static uint64_t apps_lookup_duration_le_500ms = 0;
static uint64_t apps_lookup_duration_le_1000ms = 0;
static uint64_t apps_lookup_duration_gt_1000ms = 0;

static void apps_lookup_counter_inc(uint64_t *counter)
{
    __atomic_add_fetch(counter, 1, __ATOMIC_RELAXED);
}

static uint64_t apps_lookup_counter_get(uint64_t *counter)
{
    return __atomic_load_n(counter, __ATOMIC_RELAXED);
}

static void apps_lookup_duration_observe(usec_t duration_ut)
{
    uint64_t duration_ms = duration_ut / USEC_PER_MS;

    if (duration_ms <= 1)
        apps_lookup_counter_inc(&apps_lookup_duration_le_1ms);
    else if (duration_ms <= 5)
        apps_lookup_counter_inc(&apps_lookup_duration_le_5ms);
    else if (duration_ms <= 10)
        apps_lookup_counter_inc(&apps_lookup_duration_le_10ms);
    else if (duration_ms <= 50)
        apps_lookup_counter_inc(&apps_lookup_duration_le_50ms);
    else if (duration_ms <= 100)
        apps_lookup_counter_inc(&apps_lookup_duration_le_100ms);
    else if (duration_ms <= 500)
        apps_lookup_counter_inc(&apps_lookup_duration_le_500ms);
    else if (duration_ms <= 1000)
        apps_lookup_counter_inc(&apps_lookup_duration_le_1000ms);
    else
        apps_lookup_counter_inc(&apps_lookup_duration_gt_1000ms);
}

static void apps_lookup_free_label_views(struct apps_lookup_label_view_ctx *ctx)
{
    freez(ctx->views);
    memset(ctx, 0, sizeof(*ctx));
}

static void apps_lookup_label_views_from_entry(
    const struct cgroup_lookup_entry *entry,
    struct apps_lookup_label_view_ctx *ctx)
{
    if (!entry || entry->cgroup_label_count == 0)
        return;

    ctx->views = callocz(entry->cgroup_label_count, sizeof(*ctx->views));
    ctx->count = entry->cgroup_label_count;

    for (uint16_t i = 0; i < entry->cgroup_label_count; i++) {
        ctx->views[i].key.ptr = string2str(entry->cgroup_labels[i].key);
        ctx->views[i].key.len = (uint32_t)string_strlen(entry->cgroup_labels[i].key);
        ctx->views[i].value.ptr = string2str(entry->cgroup_labels[i].value);
        ctx->views[i].value.len = (uint32_t)string_strlen(entry->cgroup_labels[i].value);
    }
}

static nipc_error_t apps_lookup_builder_add_unknown(nipc_apps_lookup_builder_t *builder, uint32_t pid)
{
    return nipc_apps_lookup_builder_add(
        builder,
        NIPC_PID_LOOKUP_UNKNOWN,
        0,
        0,
        pid,
        0,
        NIPC_UID_UNSET,
        0,
        NULL,
        0,
        NULL,
        0,
        NULL,
        0,
        NULL,
        0);
}

static nipc_error_t apps_lookup_builder_add_known(nipc_apps_lookup_builder_t *builder, struct pid_stat *p)
{
    const char *comm = string2str(p->comm);
    uint32_t comm_len = (uint32_t)MIN(strlen(comm), 15);
    if (comm_len == 0) {
        comm = "-";
        comm_len = 1;
    }

    uint32_t uid = NIPC_UID_UNSET;
#if (PROCESSES_HAVE_UID == 1)
    uid = (uint32_t)p->uid;
#endif

    if (!p->cgroup_path) {
        return nipc_apps_lookup_builder_add(
            builder,
            NIPC_PID_LOOKUP_KNOWN,
            NIPC_APPS_CGROUP_UNKNOWN_RETRY_LATER,
            NIPC_ORCHESTRATOR_UNKNOWN,
            (uint32_t)p->pid,
            (uint32_t)p->ppid,
            uid,
            p->starttime,
            comm,
            comm_len,
            NULL,
            0,
            NULL,
            0,
            NULL,
            0);
    }

    const char *path = string2str(p->cgroup_path);

    if (apps_cgroups_lookup_is_host_root_path(path)) {
        return nipc_apps_lookup_builder_add(
            builder,
            NIPC_PID_LOOKUP_KNOWN,
            NIPC_APPS_CGROUP_HOST_ROOT,
            NIPC_ORCHESTRATOR_UNKNOWN,
            (uint32_t)p->pid,
            (uint32_t)p->ppid,
            uid,
            p->starttime,
            comm,
            comm_len,
            NULL,
            0,
            NULL,
            0,
            NULL,
            0);
    }

    if (!p->cgroup_cache) {
        return nipc_apps_lookup_builder_add(
            builder,
            NIPC_PID_LOOKUP_KNOWN,
            NIPC_APPS_CGROUP_UNKNOWN_RETRY_LATER,
            NIPC_ORCHESTRATOR_UNKNOWN,
            (uint32_t)p->pid,
            (uint32_t)p->ppid,
            uid,
            p->starttime,
            comm,
            comm_len,
            path,
            (uint32_t)string_strlen(p->cgroup_path),
            NULL,
            0,
            NULL,
            0);
    }

    struct cgroup_lookup_entry *entry = p->cgroup_cache;
    uint16_t cgroup_status = NIPC_APPS_CGROUP_UNKNOWN_RETRY_LATER;
    uint16_t orchestrator = NIPC_ORCHESTRATOR_UNKNOWN;
    const char *cgroup_name = NULL;
    uint32_t cgroup_name_len = 0;
    struct apps_lookup_label_view_ctx labels = { 0 };

    if (entry->cgroup_status == NIPC_CGROUP_LOOKUP_KNOWN) {
        cgroup_status = NIPC_APPS_CGROUP_KNOWN;
        orchestrator = entry->orchestrator;
        cgroup_name = string2str(entry->cgroup_name);
        cgroup_name_len = (uint32_t)string_strlen(entry->cgroup_name);
        apps_lookup_label_views_from_entry(entry, &labels);
    }
    else if (entry->cgroup_status == NIPC_CGROUP_LOOKUP_UNKNOWN_PERMANENT) {
        cgroup_status = NIPC_APPS_CGROUP_UNKNOWN_PERMANENT;
    }

    nipc_error_t err = nipc_apps_lookup_builder_add(
        builder,
        NIPC_PID_LOOKUP_KNOWN,
        cgroup_status,
        orchestrator,
        (uint32_t)p->pid,
        (uint32_t)p->ppid,
        uid,
        p->starttime,
        comm,
        comm_len,
        path,
        (uint32_t)string_strlen(p->cgroup_path),
        cgroup_name,
        cgroup_name_len,
        labels.views,
        labels.count);

    apps_lookup_free_label_views(&labels);
    return err;
}

static bool apps_lookup_handler(
    void *user __maybe_unused,
    const nipc_apps_lookup_req_view_t *request,
    nipc_apps_lookup_builder_t *builder)
{
    usec_t started_ut = now_monotonic_usec();

    if (request->item_count > APPS_LOOKUP_MAX_PIDS_PER_REQUEST) {
        builder->error = NIPC_ERR_BAD_ITEM_COUNT;
        apps_lookup_counter_inc(&apps_lookup_requests_error);
        return false;
    }

    bool ok = true;
    netdata_mutex_lock(&apps_pids_mutex);
    nipc_apps_lookup_builder_set_generation(
        builder,
        __atomic_load_n(&apps_collection_generation, __ATOMIC_ACQUIRE));

    for (uint32_t i = 0; i < request->item_count; i++) {
        nipc_apps_lookup_req_item_t item;
        nipc_error_t err = nipc_apps_lookup_req_item(request, i, &item);
        if (err != NIPC_OK) {
            builder->error = err;
            ok = false;
            break;
        }

        struct pid_stat *p = find_pid_entry((pid_t)item.pid);
        err = p ? apps_lookup_builder_add_known(builder, p) : apps_lookup_builder_add_unknown(builder, item.pid);
        if (err != NIPC_OK) {
            ok = false;
            break;
        }
    }

    netdata_mutex_unlock(&apps_pids_mutex);

    apps_lookup_duration_observe(now_monotonic_usec() - started_ut);

    if (ok)
        apps_lookup_counter_inc(&apps_lookup_requests_responded);
    else
        apps_lookup_counter_inc(&apps_lookup_requests_error);

    return ok;
}

static void apps_lookup_netipc_server_thread(void *arg)
{
    nipc_server_run((nipc_managed_server_t *)arg);
}

bool apps_lookup_netipc_init(void)
{
    if (apps_lookup_netipc_thread)
        return true;

    nipc_server_config_t config = {
        .supported_profiles = NIPC_PROFILE_BASELINE | NIPC_PROFILE_SHM_HYBRID | NIPC_PROFILE_SHM_FUTEX,
        .preferred_profiles = NIPC_PROFILE_SHM_FUTEX,
        .auth_token = netipc_auth_token(),
    };

    nipc_apps_lookup_service_handler_t handler = {
        .handle = apps_lookup_handler,
        .user = NULL,
    };

    nipc_error_t err = nipc_server_init_apps_lookup(
        &apps_lookup_server,
        os_run_dir(true),
        APPS_NETIPC_LOOKUP_SERVICE_NAME,
        &config,
        APPS_NETIPC_LOOKUP_WORKER_COUNT,
        &handler);

    if (err != NIPC_OK) {
        netdata_log_error("apps.plugin APPS_LOOKUP server init failed (error %u)", (unsigned int)err);
        return false;
    }

    apps_lookup_netipc_thread = nd_thread_create(
        "P[appslookupipc]",
        NETDATA_THREAD_OPTION_DONT_LOG_STARTUP,
        apps_lookup_netipc_server_thread,
        &apps_lookup_server);

    if (!apps_lookup_netipc_thread) {
        netdata_log_error("apps.plugin failed to create APPS_LOOKUP server thread");
        nipc_server_destroy(&apps_lookup_server);
        return false;
    }

    netdata_log_info("apps.plugin APPS_LOOKUP server started on '%s/%s.sock'",
                     os_run_dir(true), APPS_NETIPC_LOOKUP_SERVICE_NAME);
    return true;
}

void apps_lookup_netipc_cleanup(void)
{
    if (!apps_lookup_netipc_thread)
        return;

    nipc_server_stop(&apps_lookup_server);
    nd_thread_join(apps_lookup_netipc_thread);
    apps_lookup_netipc_thread = NULL;

    nipc_server_drain(&apps_lookup_server, 5000);
    nipc_server_destroy(&apps_lookup_server);

    netdata_log_info("apps.plugin APPS_LOOKUP server stopped");
}

void apps_lookup_netipc_send_charts_to_netdata(usec_t dt)
{
    static bool charts_created = false;

    if (!charts_created) {
        charts_created = true;
        fprintf(stdout,
                "CHART netdata.collector_ipc_apps_lookup_server_requests '' 'Apps Plugin APPS_LOOKUP Server Requests' 'requests/s' apps.plugin netdata.collector.ipc.apps_lookup.server.requests line 140030 %d\n"
                "DIMENSION requests_responded '' incremental 1 1\n"
                "DIMENSION requests_error '' incremental 1 1\n"
                "CHART netdata.collector_ipc_apps_lookup_server_duration '' 'Apps Plugin APPS_LOOKUP Server Request Duration' 'requests/s' apps.plugin netdata.collector.ipc.apps_lookup.server.request_duration_ms stacked 140031 %d\n"
                "DIMENSION le_1ms '' incremental 1 1\n"
                "DIMENSION le_5ms '' incremental 1 1\n"
                "DIMENSION le_10ms '' incremental 1 1\n"
                "DIMENSION le_50ms '' incremental 1 1\n"
                "DIMENSION le_100ms '' incremental 1 1\n"
                "DIMENSION le_500ms '' incremental 1 1\n"
                "DIMENSION le_1000ms '' incremental 1 1\n"
                "DIMENSION gt_1000ms '' incremental 1 1\n",
                update_every, update_every);
    }

    fprintf(stdout,
            "BEGIN netdata.collector_ipc_apps_lookup_server_requests %" PRIu64 "\n"
            "SET requests_responded = %" PRIu64 "\n"
            "SET requests_error = %" PRIu64 "\n"
            "END\n"
            "BEGIN netdata.collector_ipc_apps_lookup_server_duration %" PRIu64 "\n"
            "SET le_1ms = %" PRIu64 "\n"
            "SET le_5ms = %" PRIu64 "\n"
            "SET le_10ms = %" PRIu64 "\n"
            "SET le_50ms = %" PRIu64 "\n"
            "SET le_100ms = %" PRIu64 "\n"
            "SET le_500ms = %" PRIu64 "\n"
            "SET le_1000ms = %" PRIu64 "\n"
            "SET gt_1000ms = %" PRIu64 "\n"
            "END\n",
            dt,
            apps_lookup_counter_get(&apps_lookup_requests_responded),
            apps_lookup_counter_get(&apps_lookup_requests_error),
            dt,
            apps_lookup_counter_get(&apps_lookup_duration_le_1ms),
            apps_lookup_counter_get(&apps_lookup_duration_le_5ms),
            apps_lookup_counter_get(&apps_lookup_duration_le_10ms),
            apps_lookup_counter_get(&apps_lookup_duration_le_50ms),
            apps_lookup_counter_get(&apps_lookup_duration_le_100ms),
            apps_lookup_counter_get(&apps_lookup_duration_le_500ms),
            apps_lookup_counter_get(&apps_lookup_duration_le_1000ms),
            apps_lookup_counter_get(&apps_lookup_duration_gt_1000ms));
}

#endif /* OS_LINUX */
