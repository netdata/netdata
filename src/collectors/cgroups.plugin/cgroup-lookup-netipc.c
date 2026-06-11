// SPDX-License-Identifier: GPL-3.0-or-later

#include "cgroup-internals.h"
#include "cgroup-netipc.h"
#include "cgroup-snapshot-store.h"
#include "libnetdata/netipc/netipc_netdata.h"

#if defined(OS_LINUX) && defined(ENABLE_CGROUPS_LOOKUP_SERVER)

#define CGROUP_NETIPC_LOOKUP_SERVICE_NAME "cgroups-lookup"
#define CGROUP_NETIPC_LOOKUP_WORKER_COUNT 2

// The handler reads the discovery-owned snapshot store under its read lock and
// never takes cgroup_root_mutex. Lock ordering: it releases the store read lock
// before signalling discovery_thread; it never holds the store lock while
// acquiring discovery_thread.mutex.

static nipc_managed_server_t cgroup_netipc_lookup_server;
static ND_THREAD *cgroup_netipc_lookup_thread = NULL;
static bool cgroup_netipc_lookup_server_initialized = false;

static uint64_t cgroup_lookup_requests_responded = 0;
static uint64_t cgroup_lookup_requests_error = 0;
static uint64_t cgroup_lookup_miss_signals_sent = 0;
static uint64_t cgroup_lookup_miss_signals_coalesced = 0;
static uint64_t cgroup_lookup_duration_le_1ms = 0;
static uint64_t cgroup_lookup_duration_le_5ms = 0;
static uint64_t cgroup_lookup_duration_le_10ms = 0;
static uint64_t cgroup_lookup_duration_le_50ms = 0;
static uint64_t cgroup_lookup_duration_le_100ms = 0;
static uint64_t cgroup_lookup_duration_le_500ms = 0;
static uint64_t cgroup_lookup_duration_le_1000ms = 0;
static uint64_t cgroup_lookup_duration_gt_1000ms = 0;

static RRDSET *cgroup_lookup_requests_st = NULL;
static RRDDIM *cgroup_lookup_requests_responded_rd = NULL;
static RRDDIM *cgroup_lookup_requests_error_rd = NULL;
static RRDDIM *cgroup_lookup_miss_signals_sent_rd = NULL;
static RRDDIM *cgroup_lookup_miss_signals_coalesced_rd = NULL;

static RRDSET *cgroup_lookup_duration_st = NULL;
static RRDDIM *cgroup_lookup_duration_le_1ms_rd = NULL;
static RRDDIM *cgroup_lookup_duration_le_5ms_rd = NULL;
static RRDDIM *cgroup_lookup_duration_le_10ms_rd = NULL;
static RRDDIM *cgroup_lookup_duration_le_50ms_rd = NULL;
static RRDDIM *cgroup_lookup_duration_le_100ms_rd = NULL;
static RRDDIM *cgroup_lookup_duration_le_500ms_rd = NULL;
static RRDDIM *cgroup_lookup_duration_le_1000ms_rd = NULL;
static RRDDIM *cgroup_lookup_duration_gt_1000ms_rd = NULL;

// Mirror of discovery's directory-walk admission test (cgroup-discovery.c:
// discovery_find_walkdir): a cgroup P is enumerated only if every strict
// ancestor directory matches matches_search_cgroup_paths() and P's depth is
// within cgroup_max_depth. A lookup MISS for an unreachable path is therefore
// permanent -- discovery will never add it -- so the peer must stop re-asking;
// a reachable miss is transient (discovery has just not seen it yet). Using the
// per-level ancestor test (not a single full-path match) is required for the
// default exact-anchored exclusions to behave like the walk for any pattern.
static bool cgroup_lookup_path_reachable(const char *path, uint32_t path_len)
{
    if (!path || !path_len || path[0] != '/')
        return false;

    if (cgroup_max_depth > 0) {
        int depth = 0;
        for (uint32_t i = 0; i < path_len; i++)
            depth += (path[i] == '/');
        if (depth > cgroup_max_depth)
            return false;
    }

    char ancestor[FILENAME_MAX + 1];
    for (uint32_t k = 0; k < path_len; k++) {
        if (path[k] != '/')
            continue;

        if (k > FILENAME_MAX)
            return false;

        if (k == 0) {
            ancestor[0] = '/';
            ancestor[1] = '\0';
        } else {
            memcpy(ancestor, path, k);
            ancestor[k] = '\0';
        }

        if (!matches_search_cgroup_paths(ancestor))
            return false;
    }

    return true;
}

static void cgroup_lookup_record_request(bool success, usec_t duration_ut)
{
    if (success)
        __atomic_add_fetch(&cgroup_lookup_requests_responded, 1, __ATOMIC_RELAXED);
    else
        __atomic_add_fetch(&cgroup_lookup_requests_error, 1, __ATOMIC_RELAXED);

    if (duration_ut <= 1000)
        __atomic_add_fetch(&cgroup_lookup_duration_le_1ms, 1, __ATOMIC_RELAXED);
    if (duration_ut <= 5000)
        __atomic_add_fetch(&cgroup_lookup_duration_le_5ms, 1, __ATOMIC_RELAXED);
    if (duration_ut <= 10000)
        __atomic_add_fetch(&cgroup_lookup_duration_le_10ms, 1, __ATOMIC_RELAXED);
    if (duration_ut <= 50000)
        __atomic_add_fetch(&cgroup_lookup_duration_le_50ms, 1, __ATOMIC_RELAXED);
    if (duration_ut <= 100000)
        __atomic_add_fetch(&cgroup_lookup_duration_le_100ms, 1, __ATOMIC_RELAXED);
    if (duration_ut <= 500000)
        __atomic_add_fetch(&cgroup_lookup_duration_le_500ms, 1, __ATOMIC_RELAXED);
    if (duration_ut <= USEC_PER_SEC)
        __atomic_add_fetch(&cgroup_lookup_duration_le_1000ms, 1, __ATOMIC_RELAXED);
    else
        __atomic_add_fetch(&cgroup_lookup_duration_gt_1000ms, 1, __ATOMIC_RELAXED);
}

static void cgroup_lookup_record_signal(bool sent)
{
    if (sent)
        __atomic_add_fetch(&cgroup_lookup_miss_signals_sent, 1, __ATOMIC_RELAXED);
    else
        __atomic_add_fetch(&cgroup_lookup_miss_signals_coalesced, 1, __ATOMIC_RELAXED);
}

bool cgroup_discovery_signal_if_unknown(void)
{
    bool was_pending = __atomic_exchange_n(&discovery_signal_pending, true, __ATOMIC_RELEASE);
    if (was_pending)
        return false;

    netdata_mutex_lock(&discovery_thread.mutex);
    netdata_cond_signal(&discovery_thread.cond_var);
    netdata_mutex_unlock(&discovery_thread.mutex);

    return true;
}

static void cgroup_lookup_log_overflow_once(uint64_t generation)
{
    static uint64_t last_logged_overflow_generation = 0;
    uint64_t last = __atomic_load_n(&last_logged_overflow_generation, __ATOMIC_RELAXED);

    if (last == generation)
        return;

    if (!__atomic_compare_exchange_n(
            &last_logged_overflow_generation, &last, generation, false, __ATOMIC_RELAXED, __ATOMIC_RELAXED))
        return;

    collector_error(
        "CGROUP: netipc lookup generation=%llu failed due to response size limits",
        (unsigned long long)generation);
}

static bool cgroups_lookup_handler(
    void *user __maybe_unused,
    const nipc_cgroups_lookup_req_view_t *request,
    nipc_cgroups_lookup_builder_t *builder)
{
    usec_t started_ut = now_monotonic_usec();
    bool should_signal_lookup_miss = false;
    bool success = false;

    // grown on demand and reused across items; views point into the store
    // entries' STRINGs, valid only while the read lock is held below
    nipc_lookup_label_view_t *label_views = NULL;
    uint16_t label_views_capacity = 0;

    // Read under the store read lock for the whole response build: the builder
    // copies each item's bytes, and discovery's publish (a brief O(1) pointer
    // swap under the write lock) waits for in-flight readers, so the entries we
    // dereference here cannot be freed mid-response. No cgroup_root_mutex.
    const CGROUP_SNAPSHOT_STORE *store = cgroup_snapshot_store_acquire();
    uint64_t generation = cgroup_snapshot_store_generation(store);

    nipc_cgroups_lookup_builder_set_generation(builder, generation);

    for (uint32_t i = 0; i < request->item_count; i++) {
        nipc_cgroups_lookup_req_item_t request_item;
        nipc_error_t err = nipc_cgroups_lookup_req_item(request, i, &request_item);
        if (err != NIPC_OK) {
            builder->error = err;
            break;
        }

        const char *path = request_item.path.ptr;
        uint32_t path_len = request_item.path.len;

        uint16_t status = NIPC_CGROUP_LOOKUP_UNKNOWN_RETRY_LATER;
        uint16_t orchestrator = 0;
        const char *name = "";
        uint32_t name_len = 0;
        uint16_t label_count = 0;

        if (generation == 0) {
            // discovery has not published a snapshot yet
            should_signal_lookup_miss = true;
        } else {
            const CGROUP_SNAPSHOT_ENTRY *e = cgroup_snapshot_store_find(store, path, path_len);
            if (e) {
                if (e->known) {
                    status = NIPC_CGROUP_LOOKUP_KNOWN;
                    orchestrator = e->orchestrator;
                    name = string2str(e->name);
                    name_len = string_strlen(e->name);

                    if (e->label_count > label_views_capacity) {
                        label_views = reallocz(label_views, (size_t)e->label_count * sizeof(*label_views));
                        label_views_capacity = e->label_count;
                    }
                    for (uint16_t l = 0; l < e->label_count; l++) {
                        label_views[l].key.ptr = string2str(e->labels[l].key);
                        label_views[l].key.len = string_strlen(e->labels[l].key);
                        label_views[l].value.ptr = string2str(e->labels[l].value);
                        label_views[l].value.len = string_strlen(e->labels[l].value);
                    }
                    label_count = e->label_count;
                }
                // else: discovery knows the cgroup but has not resolved its name
                // yet -- transient, retry without waking discovery (it is aware)
            } else if (cgroup_snapshot_reaped_contains(path)) {
                // discovered then removed -- it will never come back
                status = NIPC_CGROUP_LOOKUP_UNKNOWN_PERMANENT;
            } else if (cgroup_lookup_path_reachable(path, path_len)) {
                // discovery could enumerate this path but has not seen it yet
                should_signal_lookup_miss = true;
            } else {
                // discovery's walk would never descend to this path
                status = NIPC_CGROUP_LOOKUP_UNKNOWN_PERMANENT;
            }
        }

        err = nipc_cgroups_lookup_builder_add(
            builder, status, orchestrator,
            path, path_len,
            name, name_len,
            label_count ? label_views : NULL, label_count);

        if (err != NIPC_OK) {
            builder->error = err;
            break;
        }
    }

    cgroup_snapshot_store_release();

    if (builder->error == NIPC_OK) {
        if (should_signal_lookup_miss)
            cgroup_lookup_record_signal(cgroup_discovery_signal_if_unknown());
        success = true;
    } else if (builder->error == NIPC_ERR_OVERFLOW)
        cgroup_lookup_log_overflow_once(generation);

    cgroup_lookup_record_request(success, now_monotonic_usec() - started_ut);
    freez(label_views);

    return success;
}

static void cgroup_netipc_lookup_server_thread(void *arg)
{
    nipc_server_run((nipc_managed_server_t *)arg);
}

static void cgroup_netipc_lookup_init_at(const char *run_dir, const char *service_name)
{
    uint64_t auth = netipc_auth_token();

    nipc_server_config_t config = {
        .supported_profiles = NIPC_PROFILE_BASELINE | NIPC_PROFILE_SHM_HYBRID | NIPC_PROFILE_SHM_FUTEX,
        .preferred_profiles = NIPC_PROFILE_SHM_FUTEX,
        .auth_token = auth,
    };

    nipc_cgroups_lookup_service_handler_t handler = {
        .handle = cgroups_lookup_handler,
        .user = NULL,
    };

    nipc_error_t err = nipc_server_init_cgroups_lookup(
        &cgroup_netipc_lookup_server,
        run_dir,
        service_name,
        &config,
        CGROUP_NETIPC_LOOKUP_WORKER_COUNT,
        &handler);

    if (err != NIPC_OK) {
        collector_error("CGROUP: netipc lookup server init failed (error %u), lookup IPC disabled", (unsigned int)err);
        return;
    }

    cgroup_netipc_lookup_server_initialized = true;
    cgroup_netipc_lookup_thread = nd_thread_create(
        "P[cglkupipc]", NETDATA_THREAD_OPTION_DONT_LOG_STARTUP,
        cgroup_netipc_lookup_server_thread, &cgroup_netipc_lookup_server);

    if (!cgroup_netipc_lookup_thread) {
        collector_error("CGROUP: failed to create netipc lookup server thread");
        nipc_server_destroy(&cgroup_netipc_lookup_server);
        cgroup_netipc_lookup_server_initialized = false;
        return;
    }

    errno_clear();
    collector_info("CGROUP: netipc lookup server started on '%s/%s.sock'", run_dir, service_name);
}

void cgroup_netipc_lookup_init(void)
{
    cgroup_netipc_lookup_init_at(os_run_dir(true), CGROUP_NETIPC_LOOKUP_SERVICE_NAME);
}

#ifdef NETDATA_INTERNAL_CHECKS
void cgroup_netipc_lookup_init_for_testing(const char *run_dir, const char *service_name)
{
    cgroup_netipc_lookup_init_at(run_dir, service_name);
}
#endif

void cgroup_netipc_lookup_cleanup(void)
{
    if (cgroup_netipc_lookup_thread) {
        nipc_server_stop(&cgroup_netipc_lookup_server);
        nd_thread_join(cgroup_netipc_lookup_thread);
        cgroup_netipc_lookup_thread = NULL;

        nipc_server_drain(&cgroup_netipc_lookup_server, 5000);
    }

    if (cgroup_netipc_lookup_server_initialized) {
        nipc_server_destroy(&cgroup_netipc_lookup_server);
        cgroup_netipc_lookup_server_initialized = false;
        errno_clear();
        collector_info("CGROUP: netipc lookup server stopped");
    }
}

void cgroup_netipc_lookup_update_charts(int update_every)
{
    if (unlikely(!cgroup_lookup_requests_st)) {
        cgroup_lookup_requests_st = rrdset_create_localhost(
            "netdata",
            "collector_ipc_cgroups_lookup_server_requests",
            NULL,
            "ipc",
            "netdata.collector.ipc.cgroups_lookup.server.requests",
            "CGROUPS_LOOKUP Server Requests",
            "requests/s",
            PLUGIN_CGROUPS_NAME,
            PLUGIN_CGROUPS_MODULE_CGROUPS_NAME,
            NETDATA_CHART_PRIO_CGROUPS_CONTAINERS + 3000,
            update_every,
            RRDSET_TYPE_LINE);

        cgroup_lookup_requests_responded_rd = rrddim_add(
            cgroup_lookup_requests_st, "requests_responded", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
        cgroup_lookup_requests_error_rd = rrddim_add(
            cgroup_lookup_requests_st, "requests_error", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
        cgroup_lookup_miss_signals_sent_rd = rrddim_add(
            cgroup_lookup_requests_st, "lookup_miss_signals_sent", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
        cgroup_lookup_miss_signals_coalesced_rd = rrddim_add(
            cgroup_lookup_requests_st, "lookup_miss_signals_coalesced", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
    }

    rrddim_set_by_pointer(
        cgroup_lookup_requests_st,
        cgroup_lookup_requests_responded_rd,
        (collected_number)__atomic_load_n(&cgroup_lookup_requests_responded, __ATOMIC_RELAXED));
    rrddim_set_by_pointer(
        cgroup_lookup_requests_st,
        cgroup_lookup_requests_error_rd,
        (collected_number)__atomic_load_n(&cgroup_lookup_requests_error, __ATOMIC_RELAXED));
    rrddim_set_by_pointer(
        cgroup_lookup_requests_st,
        cgroup_lookup_miss_signals_sent_rd,
        (collected_number)__atomic_load_n(&cgroup_lookup_miss_signals_sent, __ATOMIC_RELAXED));
    rrddim_set_by_pointer(
        cgroup_lookup_requests_st,
        cgroup_lookup_miss_signals_coalesced_rd,
        (collected_number)__atomic_load_n(&cgroup_lookup_miss_signals_coalesced, __ATOMIC_RELAXED));
    rrdset_done(cgroup_lookup_requests_st);

    if (unlikely(!cgroup_lookup_duration_st)) {
        cgroup_lookup_duration_st = rrdset_create_localhost(
            "netdata",
            "collector_ipc_cgroups_lookup_server_request_duration_ms",
            NULL,
            "ipc",
            "netdata.collector.ipc.cgroups_lookup.server.request_duration_ms",
            "CGROUPS_LOOKUP Server Request Duration Histogram",
            "requests/s",
            PLUGIN_CGROUPS_NAME,
            PLUGIN_CGROUPS_MODULE_CGROUPS_NAME,
            NETDATA_CHART_PRIO_CGROUPS_CONTAINERS + 3001,
            update_every,
            RRDSET_TYPE_LINE);

        cgroup_lookup_duration_le_1ms_rd = rrddim_add(
            cgroup_lookup_duration_st, "le_1ms", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
        cgroup_lookup_duration_le_5ms_rd = rrddim_add(
            cgroup_lookup_duration_st, "le_5ms", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
        cgroup_lookup_duration_le_10ms_rd = rrddim_add(
            cgroup_lookup_duration_st, "le_10ms", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
        cgroup_lookup_duration_le_50ms_rd = rrddim_add(
            cgroup_lookup_duration_st, "le_50ms", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
        cgroup_lookup_duration_le_100ms_rd = rrddim_add(
            cgroup_lookup_duration_st, "le_100ms", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
        cgroup_lookup_duration_le_500ms_rd = rrddim_add(
            cgroup_lookup_duration_st, "le_500ms", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
        cgroup_lookup_duration_le_1000ms_rd = rrddim_add(
            cgroup_lookup_duration_st, "le_1000ms", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
        cgroup_lookup_duration_gt_1000ms_rd = rrddim_add(
            cgroup_lookup_duration_st, "gt_1000ms", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
    }

    rrddim_set_by_pointer(
        cgroup_lookup_duration_st,
        cgroup_lookup_duration_le_1ms_rd,
        (collected_number)__atomic_load_n(&cgroup_lookup_duration_le_1ms, __ATOMIC_RELAXED));
    rrddim_set_by_pointer(
        cgroup_lookup_duration_st,
        cgroup_lookup_duration_le_5ms_rd,
        (collected_number)__atomic_load_n(&cgroup_lookup_duration_le_5ms, __ATOMIC_RELAXED));
    rrddim_set_by_pointer(
        cgroup_lookup_duration_st,
        cgroup_lookup_duration_le_10ms_rd,
        (collected_number)__atomic_load_n(&cgroup_lookup_duration_le_10ms, __ATOMIC_RELAXED));
    rrddim_set_by_pointer(
        cgroup_lookup_duration_st,
        cgroup_lookup_duration_le_50ms_rd,
        (collected_number)__atomic_load_n(&cgroup_lookup_duration_le_50ms, __ATOMIC_RELAXED));
    rrddim_set_by_pointer(
        cgroup_lookup_duration_st,
        cgroup_lookup_duration_le_100ms_rd,
        (collected_number)__atomic_load_n(&cgroup_lookup_duration_le_100ms, __ATOMIC_RELAXED));
    rrddim_set_by_pointer(
        cgroup_lookup_duration_st,
        cgroup_lookup_duration_le_500ms_rd,
        (collected_number)__atomic_load_n(&cgroup_lookup_duration_le_500ms, __ATOMIC_RELAXED));
    rrddim_set_by_pointer(
        cgroup_lookup_duration_st,
        cgroup_lookup_duration_le_1000ms_rd,
        (collected_number)__atomic_load_n(&cgroup_lookup_duration_le_1000ms, __ATOMIC_RELAXED));
    rrddim_set_by_pointer(
        cgroup_lookup_duration_st,
        cgroup_lookup_duration_gt_1000ms_rd,
        (collected_number)__atomic_load_n(&cgroup_lookup_duration_gt_1000ms, __ATOMIC_RELAXED));
    rrdset_done(cgroup_lookup_duration_st);
}

#endif // OS_LINUX && ENABLE_CGROUPS_LOOKUP_SERVER
