// SPDX-License-Identifier: GPL-3.0-or-later

#include "network-viewer-apps-lookup-client.h"
#include "libnetdata/netipc/netipc_netdata.h"

#include <poll.h>
#include <sys/eventfd.h>
#include <sys/socket.h>
#include <unistd.h>

#ifndef NV_APPS_LOOKUP_SERVICE_NAME
#define NV_APPS_LOOKUP_SERVICE_NAME "apps-lookup"
#endif
#define NV_APPS_LOOKUP_BATCH_MAX 8192U
#define NV_APPS_LOOKUP_INTAKE_MAX 16384U
#define NV_APPS_LOOKUP_CACHE_DEFAULT 8192U
#define NV_APPS_LOOKUP_CACHE_MIN 1U
#define NV_APPS_LOOKUP_CACHE_MAX 65536U
#define NV_APPS_LOOKUP_REFRESH_DEFAULT 30U
#define NV_APPS_LOOKUP_REFRESH_MIN 5U
#define NV_APPS_LOOKUP_REFRESH_MAX 300U
#define NV_APPS_LOOKUP_CONNECT_RETRY_SEC 1U

typedef struct {
    char *key;
    char *value;
} NV_APPS_LOOKUP_LABEL;

typedef struct {
    char *key;
    uint32_t pid;
    uint32_t ppid;
    uint32_t uid;
    uint16_t status;
    uint16_t cgroup_status;
    uint16_t orchestrator;
    uint64_t starttime;
    uint64_t observed_generation;
    usec_t last_used_usec;
    char *comm;
    char *cgroup_path;
    char *cgroup_name;
    NV_APPS_LOOKUP_LABEL *labels;
    uint16_t label_count;
} NV_APPS_LOOKUP_CACHE_ENTRY;

typedef struct {
    uint32_t pid;
    uint64_t sequence;
} NV_APPS_LOOKUP_PID_ENTRY;

static DICTIONARY *apps_lookup_cache = NULL;
static DICTIONARY *apps_lookup_intake = NULL;
static DICTIONARY *apps_lookup_last_seen_pids = NULL;

static netdata_mutex_t apps_lookup_cache_mutex;
static netdata_mutex_t apps_lookup_intake_mutex;
static netdata_mutex_t apps_lookup_client_mutex;

static uint32_t apps_lookup_max_cache_size = NV_APPS_LOOKUP_CACHE_DEFAULT;
static uint32_t apps_lookup_refresh_seconds = NV_APPS_LOOKUP_REFRESH_DEFAULT;
static uint32_t apps_lookup_cache_size = 0;
static uint32_t apps_lookup_intake_size = 0;
static uint32_t apps_lookup_last_seen_size = 0;
static uint64_t apps_lookup_intake_sequence = 0;
static uint64_t apps_lookup_last_seen_sequence = 0;
static uint64_t apps_lookup_last_observed_generation = 0;

static int apps_lookup_intake_eventfd = -1;
static ND_THREAD *apps_lookup_worker_thread = NULL;
static bool *apps_lookup_plugin_should_exit = NULL;
static nipc_client_ctx_t apps_lookup_client_ctx;
static bool apps_lookup_client_initialized = false;
static bool apps_lookup_peer_was_ready = false;
static usec_t apps_lookup_next_retry_ut = 0;

static _Atomic bool apps_lookup_worker_stop = false;
static _Atomic bool apps_lookup_worker_thread_exited = false;
static _Atomic int apps_lookup_active_client_fd = -1;

static _Atomic uint64_t apps_lookup_requests_sent = 0;
static _Atomic uint64_t apps_lookup_requests_responded = 0;
static _Atomic uint64_t apps_lookup_requests_failed = 0;
static _Atomic uint64_t apps_lookup_cache_hits = 0;
static _Atomic uint64_t apps_lookup_cache_misses_unknown = 0;
static _Atomic uint64_t apps_lookup_cache_misses_intake_dropped = 0;
static _Atomic uint64_t apps_lookup_cache_evictions_pid_reuse = 0;
static _Atomic uint64_t apps_lookup_cache_evictions_lru = 0;
static _Atomic uint64_t apps_lookup_cache_evictions_generation_bump = 0;
static _Atomic uint64_t apps_lookup_cache_evictions_unknown_permanent = 0;
static _Atomic uint64_t apps_lookup_peer_connect_attempts = 0;
static _Atomic uint64_t apps_lookup_peer_disconnects = 0;
static _Atomic uint64_t apps_lookup_worker_refresh_probes = 0;
static _Atomic uint64_t apps_lookup_intake_depth = 0;
static _Atomic uint64_t apps_lookup_worker_duration_le_1ms = 0;
static _Atomic uint64_t apps_lookup_worker_duration_le_5ms = 0;
static _Atomic uint64_t apps_lookup_worker_duration_le_10ms = 0;
static _Atomic uint64_t apps_lookup_worker_duration_le_50ms = 0;
static _Atomic uint64_t apps_lookup_worker_duration_le_100ms = 0;
static _Atomic uint64_t apps_lookup_worker_duration_le_500ms = 0;
static _Atomic uint64_t apps_lookup_worker_duration_le_1000ms = 0;
static _Atomic uint64_t apps_lookup_worker_duration_gt_1000ms = 0;
static _Atomic uint64_t apps_lookup_handler_overhead_le_1ms = 0;
static _Atomic uint64_t apps_lookup_handler_overhead_le_5ms = 0;
static _Atomic uint64_t apps_lookup_handler_overhead_le_10ms = 0;
static _Atomic uint64_t apps_lookup_handler_overhead_le_50ms = 0;
static _Atomic uint64_t apps_lookup_handler_overhead_le_100ms = 0;
static _Atomic uint64_t apps_lookup_handler_overhead_le_500ms = 0;
static _Atomic uint64_t apps_lookup_handler_overhead_le_1000ms = 0;
static _Atomic uint64_t apps_lookup_handler_overhead_gt_1000ms = 0;

static void nv_apps_lookup_counter_inc(_Atomic uint64_t *counter)
{
    __atomic_add_fetch(counter, 1, __ATOMIC_RELAXED);
}

static uint64_t nv_apps_lookup_counter_get(_Atomic uint64_t *counter)
{
    return __atomic_load_n(counter, __ATOMIC_RELAXED);
}

static void nv_apps_lookup_duration_observe(usec_t duration_ut, _Atomic uint64_t *buckets[8])
{
    uint64_t duration_ms = duration_ut / USEC_PER_MS;

    if (duration_ms <= 1)
        nv_apps_lookup_counter_inc(buckets[0]);
    else if (duration_ms <= 5)
        nv_apps_lookup_counter_inc(buckets[1]);
    else if (duration_ms <= 10)
        nv_apps_lookup_counter_inc(buckets[2]);
    else if (duration_ms <= 50)
        nv_apps_lookup_counter_inc(buckets[3]);
    else if (duration_ms <= 100)
        nv_apps_lookup_counter_inc(buckets[4]);
    else if (duration_ms <= 500)
        nv_apps_lookup_counter_inc(buckets[5]);
    else if (duration_ms <= 1000)
        nv_apps_lookup_counter_inc(buckets[6]);
    else
        nv_apps_lookup_counter_inc(buckets[7]);
}

static void nv_apps_lookup_worker_duration_observe(usec_t duration_ut)
{
    _Atomic uint64_t *buckets[8] = {
        &apps_lookup_worker_duration_le_1ms,
        &apps_lookup_worker_duration_le_5ms,
        &apps_lookup_worker_duration_le_10ms,
        &apps_lookup_worker_duration_le_50ms,
        &apps_lookup_worker_duration_le_100ms,
        &apps_lookup_worker_duration_le_500ms,
        &apps_lookup_worker_duration_le_1000ms,
        &apps_lookup_worker_duration_gt_1000ms,
    };

    nv_apps_lookup_duration_observe(duration_ut, buckets);
}

static void nv_apps_lookup_handler_overhead_observe(usec_t duration_ut)
{
    _Atomic uint64_t *buckets[8] = {
        &apps_lookup_handler_overhead_le_1ms,
        &apps_lookup_handler_overhead_le_5ms,
        &apps_lookup_handler_overhead_le_10ms,
        &apps_lookup_handler_overhead_le_50ms,
        &apps_lookup_handler_overhead_le_100ms,
        &apps_lookup_handler_overhead_le_500ms,
        &apps_lookup_handler_overhead_le_1000ms,
        &apps_lookup_handler_overhead_gt_1000ms,
    };

    nv_apps_lookup_duration_observe(duration_ut, buckets);
}

static void nv_apps_lookup_pid_key(uint32_t pid, char key[16])
{
    snprintfz(key, 16, "%u", pid);
}

static char *nv_apps_lookup_strndupz(const char *src, uint32_t len)
{
    char *dst = mallocz((size_t)len + 1);
    if (len)
        memcpy(dst, src, len);
    dst[len] = '\0';
    return dst;
}

static void nv_apps_lookup_cache_entry_clear(NV_APPS_LOOKUP_CACHE_ENTRY *entry)
{
    if (!entry)
        return;

    freez(entry->comm);
    entry->comm = NULL;
    freez(entry->cgroup_path);
    entry->cgroup_path = NULL;
    freez(entry->cgroup_name);
    entry->cgroup_name = NULL;

    for (uint16_t i = 0; i < entry->label_count; i++) {
        freez(entry->labels[i].key);
        freez(entry->labels[i].value);
    }
    freez(entry->labels);
    entry->labels = NULL;
    entry->label_count = 0;
}

static void nv_apps_lookup_cache_entry_delete_cb(
    const DICTIONARY_ITEM *item __maybe_unused,
    void *value,
    void *data __maybe_unused)
{
    NV_APPS_LOOKUP_CACHE_ENTRY *entry = value;
    if (!entry)
        return;

    freez(entry->key);
    entry->key = NULL;
    nv_apps_lookup_cache_entry_clear(entry);
}

static void nv_apps_lookup_cache_create(void)
{
    if (apps_lookup_cache)
        return;

    apps_lookup_cache = dictionary_create_advanced(
        DICT_OPTION_SINGLE_THREADED | DICT_OPTION_DONT_OVERWRITE_VALUE | DICT_OPTION_FIXED_SIZE,
        NULL,
        sizeof(NV_APPS_LOOKUP_CACHE_ENTRY));
    dictionary_register_delete_callback(apps_lookup_cache, nv_apps_lookup_cache_entry_delete_cb, NULL);
}

static void nv_apps_lookup_pid_sets_create(void)
{
    if (!apps_lookup_intake)
        apps_lookup_intake = dictionary_create_advanced(
            DICT_OPTION_SINGLE_THREADED | DICT_OPTION_DONT_OVERWRITE_VALUE | DICT_OPTION_FIXED_SIZE,
            NULL,
            sizeof(NV_APPS_LOOKUP_PID_ENTRY));

    if (!apps_lookup_last_seen_pids)
        apps_lookup_last_seen_pids = dictionary_create_advanced(
            DICT_OPTION_SINGLE_THREADED | DICT_OPTION_DONT_OVERWRITE_VALUE | DICT_OPTION_FIXED_SIZE,
            NULL,
            sizeof(NV_APPS_LOOKUP_PID_ENTRY));
}

static void nv_apps_lookup_cache_destroy(void)
{
    if (apps_lookup_cache)
        dictionary_destroy(apps_lookup_cache);
    apps_lookup_cache = NULL;
    apps_lookup_cache_size = 0;
}

static void nv_apps_lookup_pid_sets_destroy(void)
{
    if (apps_lookup_intake)
        dictionary_destroy(apps_lookup_intake);
    apps_lookup_intake = NULL;
    apps_lookup_intake_size = 0;
    __atomic_store_n(&apps_lookup_intake_depth, 0, __ATOMIC_RELAXED);

    if (apps_lookup_last_seen_pids)
        dictionary_destroy(apps_lookup_last_seen_pids);
    apps_lookup_last_seen_pids = NULL;
    apps_lookup_last_seen_size = 0;
}

static bool nv_apps_lookup_pid_set_evict_oldest(DICTIONARY *dict, uint32_t *size)
{
    if (!dict || !size || *size == 0)
        return false;

    NV_APPS_LOOKUP_PID_ENTRY *victim = NULL;
    NV_APPS_LOOKUP_PID_ENTRY *entry;
    dfe_start_read(dict, entry) {
        if (!victim || entry->sequence < victim->sequence)
            victim = entry;
    }
    dfe_done(entry);

    if (!victim)
        return false;

    char key[16];
    nv_apps_lookup_pid_key(victim->pid, key);
    dictionary_del(dict, key);
    (*size)--;
    return true;
}

static bool nv_apps_lookup_pid_set_add(
    DICTIONARY *dict,
    uint32_t *size,
    uint32_t max_size,
    uint64_t *sequence,
    uint32_t pid,
    bool count_eviction)
{
    if (!dict || !size || !sequence || pid == 0)
        return false;

    char key[16];
    nv_apps_lookup_pid_key(pid, key);
    NV_APPS_LOOKUP_PID_ENTRY *existing = dictionary_get(dict, key);
    if (existing) {
        existing->sequence = ++(*sequence);
        return false;
    }

    while (*size >= max_size) {
        if (!nv_apps_lookup_pid_set_evict_oldest(dict, size))
            return false;
        if (count_eviction)
            nv_apps_lookup_counter_inc(&apps_lookup_cache_misses_intake_dropped);
    }

    NV_APPS_LOOKUP_PID_ENTRY tmp = {
        .pid = pid,
        .sequence = ++(*sequence),
    };

    dictionary_set(dict, key, &tmp, sizeof(tmp));
    (*size)++;
    return true;
}

static void nv_apps_lookup_signal_worker(void)
{
    if (apps_lookup_intake_eventfd < 0)
        return;

    eventfd_t value = 1;
    if (eventfd_write(apps_lookup_intake_eventfd, value) < 0 && errno != EAGAIN)
        netdata_log_error("NETWORK-VIEWER: APPS_LOOKUP eventfd write failed: %s", strerror(errno));
}

static void nv_apps_lookup_drain_eventfd(void)
{
    if (apps_lookup_intake_eventfd < 0)
        return;

    eventfd_t value;
    while (eventfd_read(apps_lookup_intake_eventfd, &value) == 0) {
        ;
    }
}

static uint32_t *nv_apps_lookup_drain_intake(uint32_t *count)
{
    *count = 0;
    bool intake_has_more = false;

    netdata_mutex_lock(&apps_lookup_intake_mutex);
    if (!apps_lookup_intake || apps_lookup_intake_size == 0) {
        __atomic_store_n(&apps_lookup_intake_depth, 0, __ATOMIC_RELAXED);
        netdata_mutex_unlock(&apps_lookup_intake_mutex);
        return NULL;
    }

    uint32_t capacity = apps_lookup_intake_size < NV_APPS_LOOKUP_BATCH_MAX ?
        apps_lookup_intake_size : NV_APPS_LOOKUP_BATCH_MAX;
    uint32_t *pids = mallocz(sizeof(*pids) * capacity);
    NV_APPS_LOOKUP_PID_ENTRY *entry;
    dfe_start_read(apps_lookup_intake, entry) {
        if (*count < capacity)
            pids[(*count)++] = entry->pid;
    }
    dfe_done(entry);

    for (uint32_t i = 0; i < *count; i++) {
        char key[16];
        nv_apps_lookup_pid_key(pids[i], key);
        dictionary_del(apps_lookup_intake, key);
        if (apps_lookup_intake_size > 0)
            apps_lookup_intake_size--;
    }

    intake_has_more = apps_lookup_intake_size > 0;
    __atomic_store_n(&apps_lookup_intake_depth, apps_lookup_intake_size, __ATOMIC_RELAXED);
    netdata_mutex_unlock(&apps_lookup_intake_mutex);

    if (intake_has_more)
        nv_apps_lookup_signal_worker();

    return pids;
}

static bool nv_apps_lookup_cache_lowest_pid(uint32_t *pid)
{
    if (!pid)
        return false;

    bool found = false;
    netdata_mutex_lock(&apps_lookup_cache_mutex);
    NV_APPS_LOOKUP_CACHE_ENTRY *entry;
    dfe_start_read(apps_lookup_cache, entry) {
        if (!found || entry->pid < *pid) {
            *pid = entry->pid;
            found = true;
        }
    }
    dfe_done(entry);
    netdata_mutex_unlock(&apps_lookup_cache_mutex);

    return found;
}

static void nv_apps_lookup_merge_last_seen_into_intake(void)
{
    netdata_mutex_lock(&apps_lookup_intake_mutex);

    NV_APPS_LOOKUP_PID_ENTRY *entry;
    dfe_start_read(apps_lookup_last_seen_pids, entry) {
        nv_apps_lookup_pid_set_add(
            apps_lookup_intake,
            &apps_lookup_intake_size,
            NV_APPS_LOOKUP_INTAKE_MAX,
            &apps_lookup_intake_sequence,
            entry->pid,
            true);
    }
    dfe_done(entry);

    bool intake_has_entries = apps_lookup_intake_size > 0;
    __atomic_store_n(&apps_lookup_intake_depth, apps_lookup_intake_size, __ATOMIC_RELAXED);
    netdata_mutex_unlock(&apps_lookup_intake_mutex);

    if (intake_has_entries)
        nv_apps_lookup_signal_worker();
}

static bool nv_apps_lookup_cache_evict_lru(void)
{
    if (!apps_lookup_cache || apps_lookup_cache_size < apps_lookup_max_cache_size)
        return true;

    NV_APPS_LOOKUP_CACHE_ENTRY *victim = NULL;
    NV_APPS_LOOKUP_CACHE_ENTRY *entry;
    dfe_start_read(apps_lookup_cache, entry) {
        if (!victim || entry->last_used_usec < victim->last_used_usec)
            victim = entry;
    }
    dfe_done(entry);

    if (!victim)
        return false;

    // cache_size is worker-private; if a future SOW adds another cache writer, re-evaluate this scan.
    char key[16];
    nv_apps_lookup_pid_key(victim->pid, key);
    dictionary_del(apps_lookup_cache, key);
    if (apps_lookup_cache_size > 0)
        apps_lookup_cache_size--;
    nv_apps_lookup_counter_inc(&apps_lookup_cache_evictions_lru);

    return true;
}

static NV_APPS_LOOKUP_CACHE_ENTRY *nv_apps_lookup_cache_insert(uint32_t pid)
{
    if (!nv_apps_lookup_cache_evict_lru())
        return NULL;

    char key[16];
    nv_apps_lookup_pid_key(pid, key);
    NV_APPS_LOOKUP_CACHE_ENTRY *entry = dictionary_set(apps_lookup_cache, key, NULL, sizeof(*entry));
    entry->key = strdupz(key);
    entry->pid = pid;
    apps_lookup_cache_size++;

    return entry;
}

static void nv_apps_lookup_cache_fill_from_item(
    NV_APPS_LOOKUP_CACHE_ENTRY *entry,
    const nipc_apps_lookup_item_view_t *item,
    uint64_t generation)
{
    nv_apps_lookup_cache_entry_clear(entry);

    entry->pid = item->pid;
    entry->ppid = item->ppid;
    entry->uid = item->uid;
    entry->status = item->status;
    entry->cgroup_status = item->cgroup_status;
    entry->orchestrator = item->orchestrator;
    entry->starttime = item->starttime;
    entry->observed_generation = generation;
    entry->last_used_usec = now_monotonic_usec();
    entry->comm = nv_apps_lookup_strndupz(item->comm.ptr, item->comm.len);
    entry->cgroup_path = nv_apps_lookup_strndupz(item->cgroup_path.ptr, item->cgroup_path.len);
    entry->cgroup_name = nv_apps_lookup_strndupz(item->cgroup_name.ptr, item->cgroup_name.len);

    if (item->label_count) {
        entry->labels = callocz(item->label_count, sizeof(*entry->labels));
        entry->label_count = item->label_count;
        for (uint16_t i = 0; i < item->label_count; i++) {
            nipc_lookup_label_view_t label;
            if (nipc_apps_lookup_item_label(item, i, &label) != NIPC_OK) {
                entry->label_count = i;
                return;
            }

            entry->labels[i].key = nv_apps_lookup_strndupz(label.key.ptr, label.key.len);
            entry->labels[i].value = nv_apps_lookup_strndupz(label.value.ptr, label.value.len);
        }
    }
}

static bool nv_apps_lookup_apply_response(
    const uint32_t *request_pids,
    uint32_t request_count,
    const nipc_apps_lookup_resp_view_t *response)
{
    bool generation_bumped = false;

    netdata_mutex_lock(&apps_lookup_cache_mutex);

    if (response->generation > apps_lookup_last_observed_generation) {
        uint32_t evicted = apps_lookup_cache_size;
        dictionary_flush(apps_lookup_cache);
        apps_lookup_cache_size = 0;
        apps_lookup_last_observed_generation = response->generation;
        __atomic_add_fetch(&apps_lookup_cache_evictions_generation_bump, evicted, __ATOMIC_RELAXED);
        generation_bumped = true;
    }

    for (uint32_t i = 0; i < response->item_count; i++) {
        nipc_apps_lookup_item_view_t item;
        if (nipc_apps_lookup_resp_item(response, i, &item) != NIPC_OK) {
            nv_apps_lookup_counter_inc(&apps_lookup_requests_failed);
            continue;
        }

        if (item.status == NIPC_PID_LOOKUP_UNKNOWN) {
            nv_apps_lookup_counter_inc(&apps_lookup_cache_misses_unknown);
            continue;
        }

        char key[16];
        nv_apps_lookup_pid_key(item.pid, key);
        NV_APPS_LOOKUP_CACHE_ENTRY *entry = dictionary_get(apps_lookup_cache, key);

        if (item.cgroup_status == NIPC_APPS_CGROUP_UNKNOWN_PERMANENT) {
            if (entry) {
                dictionary_del(apps_lookup_cache, key);
                if (apps_lookup_cache_size > 0)
                    apps_lookup_cache_size--;
                nv_apps_lookup_counter_inc(&apps_lookup_cache_evictions_unknown_permanent);
            }
            continue;
        }

        if (item.cgroup_status == NIPC_APPS_CGROUP_UNKNOWN_RETRY_LATER) {
            nv_apps_lookup_counter_inc(&apps_lookup_cache_misses_unknown);
            continue;
        }

        if (item.cgroup_status != NIPC_APPS_CGROUP_KNOWN && item.cgroup_status != NIPC_APPS_CGROUP_HOST_ROOT) {
            nv_apps_lookup_counter_inc(&apps_lookup_requests_failed);
            continue;
        }

        if (!entry)
            entry = nv_apps_lookup_cache_insert(item.pid);
        else if (entry->starttime != item.starttime) {
            dictionary_del(apps_lookup_cache, key);
            if (apps_lookup_cache_size > 0)
                apps_lookup_cache_size--;
            nv_apps_lookup_counter_inc(&apps_lookup_cache_evictions_pid_reuse);
            entry = nv_apps_lookup_cache_insert(item.pid);
        }

        if (entry)
            nv_apps_lookup_cache_fill_from_item(entry, &item, response->generation);
    }

    netdata_mutex_unlock(&apps_lookup_cache_mutex);

    (void)request_pids;
    (void)request_count;
    return generation_bumped;
}

static bool nv_apps_lookup_client_ensure_ready(void)
{
    if (nipc_client_ready(&apps_lookup_client_ctx)) {
        apps_lookup_peer_was_ready = true;
        return true;
    }

    usec_t now_ut = now_monotonic_usec();
    if (now_ut < apps_lookup_next_retry_ut)
        return false;

    nv_apps_lookup_counter_inc(&apps_lookup_peer_connect_attempts);
    (void)nipc_client_refresh(&apps_lookup_client_ctx);
    if (nipc_client_ready(&apps_lookup_client_ctx)) {
        if (!apps_lookup_peer_was_ready)
            netdata_log_info("network-viewer.plugin connected to APPS_LOOKUP service");
        apps_lookup_peer_was_ready = true;
        return true;
    }

    if (apps_lookup_peer_was_ready) {
        nv_apps_lookup_counter_inc(&apps_lookup_peer_disconnects);
        netdata_log_info("network-viewer.plugin lost APPS_LOOKUP service; cache warming will retry");
    }
    else {
        static bool logged_absent = false;
        if (!logged_absent) {
            logged_absent = true;
            netdata_log_info("network-viewer.plugin APPS_LOOKUP service unavailable; cache warming will retry");
        }
    }

    apps_lookup_peer_was_ready = false;
    apps_lookup_next_retry_ut = now_ut + NV_APPS_LOOKUP_CONNECT_RETRY_SEC * USEC_PER_SEC;
    return false;
}

static void nv_apps_lookup_worker_cancel(void *data __maybe_unused)
{
    __atomic_store_n(&apps_lookup_worker_stop, true, __ATOMIC_RELEASE);
    nv_apps_lookup_signal_worker();

#if !defined(_WIN32) && !defined(__MSYS__)
    int fd = __atomic_load_n(&apps_lookup_active_client_fd, __ATOMIC_ACQUIRE);
    if (fd >= 0)
        shutdown(fd, SHUT_RDWR);
#endif
}

static void nv_apps_lookup_worker_main(void *arg __maybe_unused)
{
    nd_thread_register_canceller(nv_apps_lookup_worker_cancel, NULL);

    while (!__atomic_load_n(&apps_lookup_worker_stop, __ATOMIC_ACQUIRE) &&
           !nd_thread_signaled_to_cancel() &&
           !__atomic_load_n(apps_lookup_plugin_should_exit, __ATOMIC_ACQUIRE)) {
        struct pollfd pfd = {
            .fd = apps_lookup_intake_eventfd,
            .events = POLLIN,
        };
        int timeout_ms = (int)apps_lookup_refresh_seconds * MSEC_PER_SEC;
        int ret = poll(&pfd, 1, timeout_ms);
        bool timer_fired = ret == 0;

        if (ret < 0) {
            if (errno == EINTR)
                continue;

            nv_apps_lookup_counter_inc(&apps_lookup_requests_failed);
            continue;
        }

        if (ret > 0)
            nv_apps_lookup_drain_eventfd();

        uint32_t pid_count = 0;
        uint32_t *pids = nv_apps_lookup_drain_intake(&pid_count);

        if (pid_count == 0 && timer_fired) {
            uint32_t probe_pid = 0;
            if (nv_apps_lookup_cache_lowest_pid(&probe_pid)) {
                pids = mallocz(sizeof(*pids));
                pids[0] = probe_pid;
                pid_count = 1;
                nv_apps_lookup_counter_inc(&apps_lookup_worker_refresh_probes);
            }
        }

        if (pid_count == 0) {
            freez(pids);
            continue;
        }

        netdata_mutex_lock(&apps_lookup_client_mutex);

        if (!nv_apps_lookup_client_ensure_ready()) {
            netdata_mutex_unlock(&apps_lookup_client_mutex);
            freez(pids);
            continue;
        }

        nipc_apps_lookup_resp_view_t response;
        nv_apps_lookup_counter_inc(&apps_lookup_requests_sent);
        usec_t started_ut = now_monotonic_usec();
#if !defined(_WIN32) && !defined(__MSYS__)
        int active_fd = apps_lookup_client_ctx.session_valid ? nipc_uds_session_fd(&apps_lookup_client_ctx.session) : -1;
        __atomic_store_n(&apps_lookup_active_client_fd, active_fd, __ATOMIC_RELEASE);
#endif
        nipc_error_t err = nipc_client_call_apps_lookup(&apps_lookup_client_ctx, pids, pid_count, &response);
#if !defined(_WIN32) && !defined(__MSYS__)
        __atomic_store_n(&apps_lookup_active_client_fd, -1, __ATOMIC_RELEASE);
#endif
        nv_apps_lookup_worker_duration_observe(now_monotonic_usec() - started_ut);

        if (err != NIPC_OK) {
            nv_apps_lookup_counter_inc(&apps_lookup_requests_failed);
            nv_apps_lookup_counter_inc(&apps_lookup_peer_disconnects);
            netdata_mutex_unlock(&apps_lookup_client_mutex);
            freez(pids);
            continue;
        }

        nv_apps_lookup_counter_inc(&apps_lookup_requests_responded);
        if (response.item_count != pid_count) {
            nv_apps_lookup_counter_inc(&apps_lookup_requests_failed);
            netdata_mutex_unlock(&apps_lookup_client_mutex);
            freez(pids);
            continue;
        }

        bool generation_bumped = nv_apps_lookup_apply_response(pids, pid_count, &response);
        netdata_mutex_unlock(&apps_lookup_client_mutex);
        if (generation_bumped)
            nv_apps_lookup_merge_last_seen_into_intake();
        freez(pids);
    }

    __atomic_store_n(&apps_lookup_worker_thread_exited, true, __ATOMIC_RELEASE);
}

static uint32_t nv_apps_lookup_clamp_u32(uint32_t value, uint32_t min, uint32_t max)
{
    if (value < min)
        return min;
    if (value > max)
        return max;
    return value;
}

void nv_apps_lookup_init(bool *plugin_should_exit)
{
    apps_lookup_plugin_should_exit = plugin_should_exit;

    apps_lookup_max_cache_size = nv_apps_lookup_clamp_u32(
        (uint32_t)inicfg_get_number(
            &netdata_config,
            "plugin:network-viewer",
            "apps lookup cache size",
            NV_APPS_LOOKUP_CACHE_DEFAULT),
        NV_APPS_LOOKUP_CACHE_MIN,
        NV_APPS_LOOKUP_CACHE_MAX);

    apps_lookup_refresh_seconds = nv_apps_lookup_clamp_u32(
        (uint32_t)inicfg_get_duration_seconds(
            &netdata_config,
            "plugin:network-viewer",
            "apps lookup generation refresh seconds",
            NV_APPS_LOOKUP_REFRESH_DEFAULT),
        NV_APPS_LOOKUP_REFRESH_MIN,
        NV_APPS_LOOKUP_REFRESH_MAX);

    if (netdata_mutex_init(&apps_lookup_cache_mutex) != 0 ||
        netdata_mutex_init(&apps_lookup_intake_mutex) != 0 ||
        netdata_mutex_init(&apps_lookup_client_mutex) != 0)
        fatal("NETWORK-VIEWER: cannot initialize APPS_LOOKUP mutexes");

    nv_apps_lookup_cache_create();
    nv_apps_lookup_pid_sets_create();

    apps_lookup_intake_eventfd = eventfd(0, EFD_NONBLOCK | EFD_CLOEXEC);
    if (apps_lookup_intake_eventfd < 0)
        fatal("NETWORK-VIEWER: cannot create APPS_LOOKUP eventfd: %s", strerror(errno));

    nipc_client_config_t config = {
        .supported_profiles = NIPC_PROFILE_BASELINE | NIPC_PROFILE_SHM_HYBRID | NIPC_PROFILE_SHM_FUTEX,
        .preferred_profiles = NIPC_PROFILE_SHM_FUTEX,
        .auth_token = netipc_auth_token(),
    };
    nipc_client_init(&apps_lookup_client_ctx, os_run_dir(true), NV_APPS_LOOKUP_SERVICE_NAME, &config);
    apps_lookup_client_initialized = true;
}

void nv_apps_lookup_start(void)
{
    if (apps_lookup_worker_thread)
        return;

    __atomic_store_n(&apps_lookup_worker_stop, false, __ATOMIC_RELEASE);
    __atomic_store_n(&apps_lookup_worker_thread_exited, false, __ATOMIC_RELEASE);
    apps_lookup_worker_thread = nd_thread_create(
        "nv-applkup", 0, nv_apps_lookup_worker_main, NULL);

    if (!apps_lookup_worker_thread)
        fatal("NETWORK-VIEWER: cannot create APPS_LOOKUP worker thread");
}

void nv_apps_lookup_stop(void)
{
    __atomic_store_n(&apps_lookup_worker_stop, true, __ATOMIC_RELEASE);

    if (apps_lookup_worker_thread) {
        nd_thread_signal_cancel(apps_lookup_worker_thread);
        nv_apps_lookup_signal_worker();
        nd_thread_join(apps_lookup_worker_thread);
        apps_lookup_worker_thread = NULL;
    }

    if (apps_lookup_client_initialized) {
        nipc_client_close(&apps_lookup_client_ctx);
        apps_lookup_client_initialized = false;
    }

    if (apps_lookup_intake_eventfd >= 0) {
        close(apps_lookup_intake_eventfd);
        apps_lookup_intake_eventfd = -1;
    }

    nv_apps_lookup_cache_destroy();
    nv_apps_lookup_pid_sets_destroy();

    netdata_mutex_destroy(&apps_lookup_client_mutex);
    netdata_mutex_destroy(&apps_lookup_intake_mutex);
    netdata_mutex_destroy(&apps_lookup_cache_mutex);
}

bool nv_apps_lookup_worker_exited(void)
{
    return __atomic_load_n(&apps_lookup_worker_thread_exited, __ATOMIC_ACQUIRE);
}

void nv_apps_lookup_warm_pids(const uint32_t *pids, size_t pid_count)
{
    if (!pids || pid_count == 0 || !apps_lookup_cache || !apps_lookup_intake || apps_lookup_intake_eventfd < 0)
        return;

    usec_t started_ut = now_monotonic_usec();
    uint32_t *misses = mallocz(sizeof(*misses) * pid_count);
    size_t miss_count = 0;
    usec_t now_ut = now_monotonic_usec();

    netdata_mutex_lock(&apps_lookup_cache_mutex);
    for (size_t i = 0; i < pid_count; i++) {
        if (pids[i] == 0)
            continue;

        char key[16];
        nv_apps_lookup_pid_key(pids[i], key);
        NV_APPS_LOOKUP_CACHE_ENTRY *entry = dictionary_get(apps_lookup_cache, key);
        if (entry) {
            entry->last_used_usec = now_ut;
            nv_apps_lookup_counter_inc(&apps_lookup_cache_hits);
        }
        else {
            misses[miss_count++] = pids[i];
        }
    }
    netdata_mutex_unlock(&apps_lookup_cache_mutex);

    netdata_mutex_lock(&apps_lookup_intake_mutex);
    for (size_t i = 0; i < pid_count; i++)
        nv_apps_lookup_pid_set_add(
            apps_lookup_last_seen_pids,
            &apps_lookup_last_seen_size,
            apps_lookup_max_cache_size,
            &apps_lookup_last_seen_sequence,
            pids[i],
            false);

    for (size_t i = 0; i < miss_count; i++)
        nv_apps_lookup_pid_set_add(
            apps_lookup_intake,
            &apps_lookup_intake_size,
            NV_APPS_LOOKUP_INTAKE_MAX,
            &apps_lookup_intake_sequence,
            misses[i],
            true);

    __atomic_store_n(&apps_lookup_intake_depth, apps_lookup_intake_size, __ATOMIC_RELAXED);
    netdata_mutex_unlock(&apps_lookup_intake_mutex);

    if (miss_count)
        nv_apps_lookup_signal_worker();

    nv_apps_lookup_handler_overhead_observe(now_monotonic_usec() - started_ut);
    freez(misses);
}

void nv_apps_lookup_send_charts_to_netdata(usec_t dt)
{
    static bool charts_created = false;

    if (!charts_created) {
        charts_created = true;
        fprintf(stdout,
                "CHART netdata.collector_ipc_apps_lookup_client_requests '' 'Network Viewer APPS_LOOKUP Client Requests' 'requests/s' network-viewer.plugin netdata.collector.ipc.apps_lookup.client.requests line 140040 1\n"
                "DIMENSION requests_sent '' incremental 1 1\n"
                "DIMENSION requests_responded '' incremental 1 1\n"
                "DIMENSION requests_failed '' incremental 1 1\n"
                "CHART netdata.collector_ipc_apps_lookup_client_cache '' 'Network Viewer APPS_LOOKUP Client Cache' 'events/s' network-viewer.plugin netdata.collector.ipc.apps_lookup.client.cache line 140041 1\n"
                "DIMENSION cache_hits '' incremental 1 1\n"
                "DIMENSION cache_misses_unknown '' incremental 1 1\n"
                "DIMENSION cache_misses_intake_dropped '' incremental 1 1\n"
                "DIMENSION cache_evictions_pid_reuse '' incremental 1 1\n"
                "DIMENSION cache_evictions_lru '' incremental 1 1\n"
                "DIMENSION cache_evictions_generation_bump '' incremental 1 1\n"
                "DIMENSION cache_evictions_unknown_permanent '' incremental 1 1\n"
                "CHART netdata.collector_ipc_apps_lookup_client_peer '' 'Network Viewer APPS_LOOKUP Client Peer' 'events/s' network-viewer.plugin netdata.collector.ipc.apps_lookup.client.peer line 140042 1\n"
                "DIMENSION peer_connect_attempts '' incremental 1 1\n"
                "DIMENSION peer_disconnects '' incremental 1 1\n"
                "DIMENSION worker_refresh_probes '' incremental 1 1\n"
                "CHART netdata.collector_ipc_apps_lookup_client_worker_duration '' 'Network Viewer APPS_LOOKUP Worker Request Duration' 'requests/s' network-viewer.plugin netdata.collector.ipc.apps_lookup.client.worker_request_duration_ms stacked 140043 1\n"
                "DIMENSION le_1ms '' incremental 1 1\n"
                "DIMENSION le_5ms '' incremental 1 1\n"
                "DIMENSION le_10ms '' incremental 1 1\n"
                "DIMENSION le_50ms '' incremental 1 1\n"
                "DIMENSION le_100ms '' incremental 1 1\n"
                "DIMENSION le_500ms '' incremental 1 1\n"
                "DIMENSION le_1000ms '' incremental 1 1\n"
                "DIMENSION gt_1000ms '' incremental 1 1\n"
                "CHART netdata.collector_ipc_apps_lookup_client_handler_overhead '' 'Network Viewer APPS_LOOKUP Function Handler Overhead' 'calls/s' network-viewer.plugin netdata.collector.ipc.apps_lookup.client.function_handler_overhead_ms stacked 140044 1\n"
                "DIMENSION le_1ms '' incremental 1 1\n"
                "DIMENSION le_5ms '' incremental 1 1\n"
                "DIMENSION le_10ms '' incremental 1 1\n"
                "DIMENSION le_50ms '' incremental 1 1\n"
                "DIMENSION le_100ms '' incremental 1 1\n"
                "DIMENSION le_500ms '' incremental 1 1\n"
                "DIMENSION le_1000ms '' incremental 1 1\n"
                "DIMENSION gt_1000ms '' incremental 1 1\n"
                "CHART netdata.collector_ipc_apps_lookup_client_intake '' 'Network Viewer APPS_LOOKUP Intake Depth' 'pids' network-viewer.plugin netdata.collector.ipc.apps_lookup.client.intake_depth line 140045 1\n"
                "DIMENSION intake_depth '' absolute 1 1\n");
    }

    fprintf(stdout,
            "BEGIN netdata.collector_ipc_apps_lookup_client_requests %" PRIu64 "\n"
            "SET requests_sent = %" PRIu64 "\n"
            "SET requests_responded = %" PRIu64 "\n"
            "SET requests_failed = %" PRIu64 "\n"
            "END\n"
            "BEGIN netdata.collector_ipc_apps_lookup_client_cache %" PRIu64 "\n"
            "SET cache_hits = %" PRIu64 "\n"
            "SET cache_misses_unknown = %" PRIu64 "\n"
            "SET cache_misses_intake_dropped = %" PRIu64 "\n"
            "SET cache_evictions_pid_reuse = %" PRIu64 "\n"
            "SET cache_evictions_lru = %" PRIu64 "\n"
            "SET cache_evictions_generation_bump = %" PRIu64 "\n"
            "SET cache_evictions_unknown_permanent = %" PRIu64 "\n"
            "END\n"
            "BEGIN netdata.collector_ipc_apps_lookup_client_peer %" PRIu64 "\n"
            "SET peer_connect_attempts = %" PRIu64 "\n"
            "SET peer_disconnects = %" PRIu64 "\n"
            "SET worker_refresh_probes = %" PRIu64 "\n"
            "END\n"
            "BEGIN netdata.collector_ipc_apps_lookup_client_worker_duration %" PRIu64 "\n"
            "SET le_1ms = %" PRIu64 "\n"
            "SET le_5ms = %" PRIu64 "\n"
            "SET le_10ms = %" PRIu64 "\n"
            "SET le_50ms = %" PRIu64 "\n"
            "SET le_100ms = %" PRIu64 "\n"
            "SET le_500ms = %" PRIu64 "\n"
            "SET le_1000ms = %" PRIu64 "\n"
            "SET gt_1000ms = %" PRIu64 "\n"
            "END\n"
            "BEGIN netdata.collector_ipc_apps_lookup_client_handler_overhead %" PRIu64 "\n"
            "SET le_1ms = %" PRIu64 "\n"
            "SET le_5ms = %" PRIu64 "\n"
            "SET le_10ms = %" PRIu64 "\n"
            "SET le_50ms = %" PRIu64 "\n"
            "SET le_100ms = %" PRIu64 "\n"
            "SET le_500ms = %" PRIu64 "\n"
            "SET le_1000ms = %" PRIu64 "\n"
            "SET gt_1000ms = %" PRIu64 "\n"
            "END\n"
            "BEGIN netdata.collector_ipc_apps_lookup_client_intake %" PRIu64 "\n"
            "SET intake_depth = %" PRIu64 "\n"
            "END\n",
            dt,
            nv_apps_lookup_counter_get(&apps_lookup_requests_sent),
            nv_apps_lookup_counter_get(&apps_lookup_requests_responded),
            nv_apps_lookup_counter_get(&apps_lookup_requests_failed),
            dt,
            nv_apps_lookup_counter_get(&apps_lookup_cache_hits),
            nv_apps_lookup_counter_get(&apps_lookup_cache_misses_unknown),
            nv_apps_lookup_counter_get(&apps_lookup_cache_misses_intake_dropped),
            nv_apps_lookup_counter_get(&apps_lookup_cache_evictions_pid_reuse),
            nv_apps_lookup_counter_get(&apps_lookup_cache_evictions_lru),
            nv_apps_lookup_counter_get(&apps_lookup_cache_evictions_generation_bump),
            nv_apps_lookup_counter_get(&apps_lookup_cache_evictions_unknown_permanent),
            dt,
            nv_apps_lookup_counter_get(&apps_lookup_peer_connect_attempts),
            nv_apps_lookup_counter_get(&apps_lookup_peer_disconnects),
            nv_apps_lookup_counter_get(&apps_lookup_worker_refresh_probes),
            dt,
            nv_apps_lookup_counter_get(&apps_lookup_worker_duration_le_1ms),
            nv_apps_lookup_counter_get(&apps_lookup_worker_duration_le_5ms),
            nv_apps_lookup_counter_get(&apps_lookup_worker_duration_le_10ms),
            nv_apps_lookup_counter_get(&apps_lookup_worker_duration_le_50ms),
            nv_apps_lookup_counter_get(&apps_lookup_worker_duration_le_100ms),
            nv_apps_lookup_counter_get(&apps_lookup_worker_duration_le_500ms),
            nv_apps_lookup_counter_get(&apps_lookup_worker_duration_le_1000ms),
            nv_apps_lookup_counter_get(&apps_lookup_worker_duration_gt_1000ms),
            dt,
            nv_apps_lookup_counter_get(&apps_lookup_handler_overhead_le_1ms),
            nv_apps_lookup_counter_get(&apps_lookup_handler_overhead_le_5ms),
            nv_apps_lookup_counter_get(&apps_lookup_handler_overhead_le_10ms),
            nv_apps_lookup_counter_get(&apps_lookup_handler_overhead_le_50ms),
            nv_apps_lookup_counter_get(&apps_lookup_handler_overhead_le_100ms),
            nv_apps_lookup_counter_get(&apps_lookup_handler_overhead_le_500ms),
            nv_apps_lookup_counter_get(&apps_lookup_handler_overhead_le_1000ms),
            nv_apps_lookup_counter_get(&apps_lookup_handler_overhead_gt_1000ms),
            dt,
            nv_apps_lookup_counter_get(&apps_lookup_intake_depth));
}
