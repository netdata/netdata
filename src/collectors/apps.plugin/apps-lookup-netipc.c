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

#ifdef NETDATA_APPS_LOOKUP_TEST_HOOKS
static usec_t apps_lookup_test_lock_started_ut = 0;
static usec_t apps_lookup_test_max_lock_hold_ut = 0;
static uint64_t apps_lookup_test_lock_acquisitions = 0;
static uint64_t apps_lookup_test_emit_calls = 0;
static uint64_t apps_lookup_test_emit_calls_while_locked = 0;
static bool apps_lookup_test_lock_held = false;

static void apps_lookup_test_lock_acquired(void)
{
    apps_lookup_test_lock_held = true;
    apps_lookup_test_lock_started_ut = now_monotonic_usec();
    apps_lookup_test_lock_acquisitions++;
}

static void apps_lookup_test_lock_released(void)
{
    usec_t now_ut = now_monotonic_usec();
    if (apps_lookup_test_lock_started_ut && now_ut >= apps_lookup_test_lock_started_ut) {
        usec_t held_ut = now_ut - apps_lookup_test_lock_started_ut;
        if (held_ut > apps_lookup_test_max_lock_hold_ut)
            apps_lookup_test_max_lock_hold_ut = held_ut;
    }

    apps_lookup_test_lock_held = false;
    apps_lookup_test_lock_started_ut = 0;
}

static void apps_lookup_test_before_emit(void)
{
    apps_lookup_test_emit_calls++;
    if (apps_lookup_test_lock_held)
        apps_lookup_test_emit_calls_while_locked++;
}
#else
#define apps_lookup_test_lock_acquired() do {} while(0)
#define apps_lookup_test_lock_released() do {} while(0)
#define apps_lookup_test_before_emit() do {} while(0)
#endif

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

struct apps_lookup_staged_label {
    STRING *key;
    STRING *value;
};

// A request PID's response fields, snapshotted under apps_pids_mutex so the wire
// encoding (the real cost on the 8192-PID network-viewer path) runs after the
// lock is released. STRINGs are string_dup'd (O(1) refcount) so they stay valid
// once the lock is dropped and the collector may mutate or evict the originals.
struct apps_lookup_staged_pid {
    bool found;               // PID exists
    uint16_t cgroup_status;
    uint16_t orchestrator;
    uint32_t pid;
    uint32_t ppid;
    uint32_t uid;
    uint64_t starttime;
    STRING *comm;             // dup; NULL when the PID was not found
    STRING *cgroup_path;      // dup; NULL means "do not encode a path"
    STRING *cgroup_name;      // dup; NULL means none
    struct apps_lookup_staged_label *labels;
    uint16_t label_count;
};

// Phase 1 (under apps_pids_mutex): find the PID and refcount-dup the strings the
// response needs. No wire encoding happens here.
static void apps_lookup_stage_pid(uint32_t req_pid, struct apps_lookup_staged_pid *staged)
{
    *staged = (struct apps_lookup_staged_pid){ 0 };

    struct pid_stat *p = find_pid_entry((pid_t)req_pid);
    if (!p) {
        staged->pid = req_pid;
        return;
    }

    staged->found = true;
    staged->pid = (uint32_t)p->pid;
    staged->ppid = (uint32_t)p->ppid;
    staged->uid = NIPC_UID_UNSET;
#if (PROCESSES_HAVE_UID == 1)
    staged->uid = (uint32_t)p->uid;
#endif
    staged->starttime = p->starttime;
    staged->comm = string_dup(p->comm);
    staged->cgroup_status = NIPC_APPS_CGROUP_UNKNOWN_RETRY_LATER;
    staged->orchestrator = NIPC_ORCHESTRATOR_UNKNOWN;

    if (!p->cgroup_path)
        return; // RETRY_LATER, no path encoded

    if (apps_cgroups_lookup_is_host_root_path(string2str(p->cgroup_path))) {
        staged->cgroup_status = NIPC_APPS_CGROUP_HOST_ROOT; // no path encoded
        return;
    }

    staged->cgroup_path = string_dup(p->cgroup_path); // path encoded

    const struct cgroup_lookup_entry *entry = p->cgroup_cache;
    if (!entry)
        return; // RETRY_LATER, with path

    if (entry->cgroup_status == NIPC_CGROUP_LOOKUP_KNOWN) {
        staged->cgroup_status = NIPC_APPS_CGROUP_KNOWN;
        staged->orchestrator = entry->orchestrator;
        staged->cgroup_name = string_dup(entry->cgroup_name);
        if (entry->cgroup_label_count) {
            staged->label_count = entry->cgroup_label_count;
            staged->labels = callocz(entry->cgroup_label_count, sizeof(*staged->labels));
            for (uint16_t i = 0; i < entry->cgroup_label_count; i++) {
                staged->labels[i].key = string_dup(entry->cgroup_labels[i].key);
                staged->labels[i].value = string_dup(entry->cgroup_labels[i].value);
            }
        }
    }
    else if (entry->cgroup_status == NIPC_CGROUP_LOOKUP_UNKNOWN_PERMANENT)
        staged->cgroup_status = NIPC_APPS_CGROUP_UNKNOWN_PERMANENT;
}

static void apps_lookup_free_staged_pid(struct apps_lookup_staged_pid *staged)
{
    string_freez(staged->comm);
    string_freez(staged->cgroup_path);
    string_freez(staged->cgroup_name);
    for (uint16_t i = 0; i < staged->label_count; i++) {
        string_freez(staged->labels[i].key);
        string_freez(staged->labels[i].value);
    }
    freez(staged->labels);
}

// Phase 2 (no lock): wire-encode a staged PID into the response.
static nipc_error_t apps_lookup_emit_staged(
    nipc_apps_lookup_builder_t *builder, const struct apps_lookup_staged_pid *staged)
{
    apps_lookup_test_before_emit();

    if (!staged->found)
        return nipc_apps_lookup_builder_add(
            builder, NIPC_PID_LOOKUP_UNKNOWN, 0, 0, staged->pid, 0, NIPC_UID_UNSET, 0,
            NULL, 0, NULL, 0, NULL, 0, NULL, 0);

    const char *comm = string2str(staged->comm);
    uint32_t comm_len = (uint32_t)MIN(strlen(comm), 15);
    if (comm_len == 0) {
        comm = "-";
        comm_len = 1;
    }

    const char *path = staged->cgroup_path ? string2str(staged->cgroup_path) : NULL;
    uint32_t path_len = staged->cgroup_path ? (uint32_t)string_strlen(staged->cgroup_path) : 0;
    const char *cgroup_name = staged->cgroup_name ? string2str(staged->cgroup_name) : NULL;
    uint32_t cgroup_name_len = staged->cgroup_name ? (uint32_t)string_strlen(staged->cgroup_name) : 0;

    nipc_lookup_label_view_t *views = NULL;
    if (staged->label_count) {
        views = callocz(staged->label_count, sizeof(*views));
        for (uint16_t i = 0; i < staged->label_count; i++) {
            views[i].key.ptr = string2str(staged->labels[i].key);
            views[i].key.len = (uint32_t)string_strlen(staged->labels[i].key);
            views[i].value.ptr = string2str(staged->labels[i].value);
            views[i].value.len = (uint32_t)string_strlen(staged->labels[i].value);
        }
    }

    nipc_error_t err = nipc_apps_lookup_builder_add(
        builder,
        NIPC_PID_LOOKUP_KNOWN,
        staged->cgroup_status,
        staged->orchestrator,
        staged->pid,
        staged->ppid,
        staged->uid,
        staged->starttime,
        comm,
        comm_len,
        path,
        path_len,
        cgroup_name,
        cgroup_name_len,
        views,
        staged->label_count);

    freez(views);
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

    struct apps_lookup_staged_pid *staged = NULL;
    if (request->item_count)
        staged = callocz(request->item_count, sizeof(*staged));
    uint32_t staged_count = 0;
    bool ok = true;

    // Phase 1: snapshot each requested PID under the lock (find + refcount-dup).
    netdata_mutex_lock(&apps_pids_mutex);
    apps_lookup_test_lock_acquired();
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

        apps_lookup_stage_pid((uint32_t)item.pid, &staged[staged_count++]);
    }

    apps_lookup_test_lock_released();
    netdata_mutex_unlock(&apps_pids_mutex);

    // Phase 2: wire-encode the snapshot with the lock released (the real cost,
    // up to 8192 PIDs on the network-viewer path).
    for (uint32_t i = 0; ok && i < staged_count; i++) {
        if (apps_lookup_emit_staged(builder, &staged[i]) != NIPC_OK)
            ok = false;
    }

    for (uint32_t i = 0; i < staged_count; i++)
        apps_lookup_free_staged_pid(&staged[i]);
    freez(staged);

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
