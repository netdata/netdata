// SPDX-License-Identifier: GPL-3.0-or-later

#include "cgroup-internals.h"
#include "cgroup-netipc.h"
#include "libnetdata/netipc/netipc_netdata.h"

#if defined(OS_LINUX) && defined(ENABLE_CGROUPS_LOOKUP_SERVER)

#define CGROUP_NETIPC_LOOKUP_SERVICE_NAME "cgroups-lookup"
#define CGROUP_NETIPC_LOOKUP_WORKER_COUNT 2

// Lock ordering invariant: lookup handlers release cgroup_root_mutex before signalling discovery_thread.
// Never hold cgroup_root_mutex while acquiring discovery_thread.mutex.

struct cgroup_lookup_reaped_entry {
    char *path;
    uint32_t hash;
    struct cgroup_lookup_reaped_entry *prev;
    struct cgroup_lookup_reaped_entry *next;
};

struct cgroup_lookup_copied_label {
    char *key;
    char *value;
};

struct cgroup_lookup_label_copy_ctx {
    struct cgroup_lookup_copied_label *labels;
    nipc_lookup_label_view_t *views;
    uint16_t count;
    uint16_t capacity;
    bool overflow;
};

struct cgroup_lookup_response_item {
    uint16_t status;
    uint16_t orchestrator;
    char *path;
    char *name;
    struct cgroup_lookup_copied_label *labels;
    nipc_lookup_label_view_t *label_views;
    uint16_t label_count;
};

static nipc_managed_server_t cgroup_netipc_lookup_server;
static ND_THREAD *cgroup_netipc_lookup_thread = NULL;
static bool cgroup_netipc_lookup_server_initialized = false;
static bool cgroup_lookup_reaped_accepting = false;

static struct cgroup_lookup_reaped_entry *cgroup_lookup_reaped_head = NULL;
static size_t cgroup_lookup_reaped_entries = 0;

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

static char *cgroup_lookup_strndupz(const char *src, uint32_t len)
{
    char *dst = mallocz((size_t)len + 1);
    if (len)
        memcpy(dst, src, len);
    dst[len] = '\0';
    return dst;
}

static void cgroup_lookup_reaped_unlink(struct cgroup_lookup_reaped_entry *entry)
{
    DOUBLE_LINKED_LIST_REMOVE_ITEM_UNSAFE(cgroup_lookup_reaped_head, entry, prev, next);
    cgroup_lookup_reaped_entries--;
}

static void cgroup_lookup_reaped_link_head(struct cgroup_lookup_reaped_entry *entry)
{
    DOUBLE_LINKED_LIST_PREPEND_ITEM_UNSAFE(cgroup_lookup_reaped_head, entry, prev, next);
    cgroup_lookup_reaped_entries++;
}

static void cgroup_lookup_reaped_free_entry(struct cgroup_lookup_reaped_entry *entry)
{
    if (!entry)
        return;

    freez(entry->path);
    freez(entry);
}

static struct cgroup_lookup_reaped_entry *cgroup_lookup_reaped_find_locked(const char *path, uint32_t hash)
{
    struct cgroup_lookup_reaped_entry *entry;

    for (entry = cgroup_lookup_reaped_head; entry; entry = entry->next) {
        if (entry->hash == hash && strcmp(entry->path, path) == 0)
            return entry;
    }

    return NULL;
}

static bool cgroup_lookup_reaped_contains_locked(const char *path)
{
    if (!path)
        return false;

    uint32_t hash = simple_hash(path);
    return cgroup_lookup_reaped_find_locked(path, hash) != NULL;
}

static void cgroup_lookup_reaped_set_insert_locked(const char *path)
{
    if (!path || !*path || cgroup_lookup_reaped_set_size <= 0)
        return;

    uint32_t hash = simple_hash(path);
    struct cgroup_lookup_reaped_entry *entry = cgroup_lookup_reaped_find_locked(path, hash);
    if (entry) {
        cgroup_lookup_reaped_unlink(entry);
        cgroup_lookup_reaped_link_head(entry);
        return;
    }

    entry = callocz(1, sizeof(*entry));
    entry->path = strdupz(path);
    entry->hash = hash;
    cgroup_lookup_reaped_link_head(entry);

    while (cgroup_lookup_reaped_entries > (size_t)cgroup_lookup_reaped_set_size) {
        struct cgroup_lookup_reaped_entry *oldest = cgroup_lookup_reaped_head->prev;
        cgroup_lookup_reaped_unlink(oldest);
        cgroup_lookup_reaped_free_entry(oldest);
    }
}

static void cgroup_lookup_reaped_set_destroy_locked(void)
{
    struct cgroup_lookup_reaped_entry *entry = cgroup_lookup_reaped_head;
    while (entry) {
        struct cgroup_lookup_reaped_entry *next = entry->next;
        cgroup_lookup_reaped_free_entry(entry);
        entry = next;
    }

    cgroup_lookup_reaped_head = NULL;
    cgroup_lookup_reaped_entries = 0;
}

void cgroup_netipc_lookup_reaped_path_add(const char *path)
{
    if (!cgroup_lookup_reaped_accepting)
        return;

    cgroup_lookup_reaped_set_insert_locked(path);
}

#ifdef NETDATA_INTERNAL_CHECKS
void cgroup_netipc_lookup_reaped_set_insert(const char *path)
{
    netdata_mutex_lock(&cgroup_root_mutex);
    cgroup_lookup_reaped_set_insert_locked(path);
    netdata_mutex_unlock(&cgroup_root_mutex);
}

void cgroup_lookup_set_cgroup_root_for_testing(struct cgroup *root)
{
    netdata_mutex_lock(&cgroup_root_mutex);
    cgroup_root = root;
    netdata_mutex_unlock(&cgroup_root_mutex);
}
#endif

static void cgroup_lookup_free_labels(
    struct cgroup_lookup_copied_label *labels,
    nipc_lookup_label_view_t *views,
    uint16_t count)
{
    for (uint16_t i = 0; i < count; i++) {
        freez(labels[i].key);
        freez(labels[i].value);
    }

    freez(labels);
    freez(views);
}

static void cgroup_lookup_free_items(struct cgroup_lookup_response_item *items, uint32_t count)
{
    if (!items)
        return;

    for (uint32_t i = 0; i < count; i++) {
        freez(items[i].path);
        freez(items[i].name);
        cgroup_lookup_free_labels(items[i].labels, items[i].label_views, items[i].label_count);
    }

    freez(items);
}

static int cgroup_lookup_copy_label_cb(
    const char *name,
    const char *value,
    RRDLABEL_SRC ls __maybe_unused,
    void *data)
{
    struct cgroup_lookup_label_copy_ctx *ctx = data;

    if (ctx->count == UINT16_MAX) {
        ctx->overflow = true;
        return -1;
    }

    if (ctx->count == ctx->capacity) {
        uint16_t new_capacity = ctx->capacity ? ctx->capacity * 2 : 8;
        if (new_capacity < ctx->capacity || new_capacity == 0) {
            ctx->overflow = true;
            return -1;
        }

        ctx->labels = reallocz(ctx->labels, (size_t)new_capacity * sizeof(*ctx->labels));
        ctx->views = reallocz(ctx->views, (size_t)new_capacity * sizeof(*ctx->views));
        ctx->capacity = new_capacity;
    }

    struct cgroup_lookup_copied_label *label = &ctx->labels[ctx->count];
    label->key = strdupz(name ? name : "");
    label->value = strdupz(value ? value : "");
    ctx->views[ctx->count].key.ptr = label->key;
    ctx->views[ctx->count].key.len = (uint32_t)strlen(label->key);
    ctx->views[ctx->count].value.ptr = label->value;
    ctx->views[ctx->count].value.len = (uint32_t)strlen(label->value);
    ctx->count++;

    return 0;
}

static bool cgroup_lookup_copy_labels_locked(
    struct cgroup *cg,
    struct cgroup_lookup_response_item *item)
{
    struct cgroup_lookup_label_copy_ctx ctx = { 0 };

    if (cg->chart_labels)
        rrdlabels_walkthrough_read(cg->chart_labels, cgroup_lookup_copy_label_cb, &ctx);

    if (ctx.overflow) {
        cgroup_lookup_free_labels(ctx.labels, ctx.views, ctx.count);
        return false;
    }

    item->labels = ctx.labels;
    item->label_views = ctx.views;
    item->label_count = ctx.count;
    return true;
}

static struct cgroup *cgroup_lookup_find_locked(const char *path, uint32_t path_len)
{
    struct cgroup *cg;

    for (cg = cgroup_root; cg; cg = cg->next) {
        if (strlen(cg->id) == path_len && memcmp(cg->id, path, path_len) == 0)
            return cg;
    }

    return NULL;
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
    static _Atomic uint64_t last_logged_overflow_generation = 0;
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
    struct cgroup_lookup_response_item *items = NULL;
    uint64_t generation;
    bool should_signal_lookup_miss = false;
    bool success = false;

    if (request->item_count)
        items = callocz(request->item_count, sizeof(*items));

    netdata_mutex_lock(&cgroup_root_mutex);

    generation = __atomic_load_n(&cgroup_discovery_generation, __ATOMIC_ACQUIRE);

    for (uint32_t i = 0; i < request->item_count; i++) {
        nipc_cgroups_lookup_req_item_t request_item;
        nipc_error_t err = nipc_cgroups_lookup_req_item(request, i, &request_item);
        if (err != NIPC_OK) {
            builder->error = err;
            goto cleanup_locked;
        }

        items[i].path = cgroup_lookup_strndupz(request_item.path.ptr, request_item.path.len);

        if (generation == 0) {
            items[i].status = NIPC_CGROUP_LOOKUP_UNKNOWN_RETRY_LATER;
            should_signal_lookup_miss = true;
            continue;
        }

        struct cgroup *cg = cgroup_lookup_find_locked(request_item.path.ptr, request_item.path.len);
        if (cg) {
            if (cg->processed && cg->pending_renames == 0) {
                discovery_classify_orchestrator(cg);
                items[i].status = NIPC_CGROUP_LOOKUP_KNOWN;
                items[i].orchestrator = (uint16_t)cg->container_orchestrator;
                items[i].name = strdupz(cg->name ? cg->name : "");
                if (!cgroup_lookup_copy_labels_locked(cg, &items[i])) {
                    builder->error = NIPC_ERR_OVERFLOW;
                    goto cleanup_locked;
                }
            } else {
                items[i].status = NIPC_CGROUP_LOOKUP_UNKNOWN_RETRY_LATER;
            }
        } else if (cgroup_lookup_reaped_contains_locked(items[i].path)) {
            items[i].status = NIPC_CGROUP_LOOKUP_UNKNOWN_PERMANENT;
        } else {
            items[i].status = NIPC_CGROUP_LOOKUP_UNKNOWN_RETRY_LATER;
            should_signal_lookup_miss = true;
        }
    }

cleanup_locked:
    netdata_mutex_unlock(&cgroup_root_mutex);

    nipc_cgroups_lookup_builder_set_generation(builder, generation);

    if (builder->error != NIPC_OK)
        goto cleanup;

    for (uint32_t i = 0; i < request->item_count; i++) {
        nipc_error_t err = nipc_cgroups_lookup_builder_add(
            builder,
            items[i].status,
            items[i].orchestrator,
            items[i].path,
            (uint32_t)strlen(items[i].path),
            items[i].name ? items[i].name : "",
            items[i].name ? (uint32_t)strlen(items[i].name) : 0,
            items[i].label_views,
            items[i].label_count);

        if (err != NIPC_OK)
            goto cleanup;
    }

    if (should_signal_lookup_miss)
        cgroup_lookup_record_signal(cgroup_discovery_signal_if_unknown());

    success = true;

cleanup:
    if (!success && builder->error == NIPC_ERR_OVERFLOW)
        cgroup_lookup_log_overflow_once(generation);

    cgroup_lookup_record_request(success, now_monotonic_usec() - started_ut);
    cgroup_lookup_free_items(items, request->item_count);

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

    cgroup_lookup_reaped_accepting = true;
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
    cgroup_lookup_reaped_accepting = false;

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

    netdata_mutex_lock(&cgroup_root_mutex);
    cgroup_lookup_reaped_set_destroy_locked();
    netdata_mutex_unlock(&cgroup_root_mutex);
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
