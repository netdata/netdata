// SPDX-License-Identifier: GPL-3.0-or-later

#include "apps-cgroups-lookup-client.h"

#ifdef OS_LINUX

#define APPS_CGROUPS_LOOKUP_SERVICE_NAME "cgroups-lookup"
#define APPS_CGROUPS_LOOKUP_BATCH_MAX 256U
#define APPS_CGROUPS_LOOKUP_CONNECT_RETRY_SEC 30ULL

struct cgroup_lookup_queue_entry {
    STRING *path;
    struct cgroup_lookup_queue_entry *next;
};

static DICTIONARY *cgroups_lookup_cache = NULL;
static size_t cgroups_lookup_cache_entries = 0;
static uint64_t cgroups_lookup_cache_iteration = 0;

static netdata_mutex_t cgroups_lookup_queue_mutex;
static netdata_cond_t cgroups_lookup_queue_cond;
static bool cgroups_lookup_queue_initialized = false;
static bool cgroups_lookup_worker_stop = false;
static struct cgroup_lookup_queue_entry *cgroups_lookup_queue_head = NULL;
static struct cgroup_lookup_queue_entry *cgroups_lookup_queue_tail = NULL;
static size_t cgroups_lookup_queue_count = 0;
static ND_THREAD *cgroups_lookup_worker_thread = NULL;

static _Atomic uint64_t cgroups_lookup_requests_sent = 0;
static _Atomic uint64_t cgroups_lookup_requests_responded = 0;
static _Atomic uint64_t cgroups_lookup_requests_timeout = 0;
static _Atomic uint64_t cgroups_lookup_requests_error = 0;
static _Atomic uint64_t cgroups_lookup_cache_hits = 0;
static _Atomic uint64_t cgroups_lookup_cache_misses_retry = 0;
static _Atomic uint64_t cgroups_lookup_cache_misses_permanent = 0;
static _Atomic uint64_t cgroups_lookup_cache_evictions = 0;
static _Atomic uint64_t cgroups_lookup_peer_connect_attempts = 0;
static _Atomic uint64_t cgroups_lookup_peer_disconnects = 0;
static _Atomic uint64_t cgroups_lookup_queue_depth = 0;
static _Atomic uint64_t cgroups_lookup_duration_le_1ms = 0;
static _Atomic uint64_t cgroups_lookup_duration_le_5ms = 0;
static _Atomic uint64_t cgroups_lookup_duration_le_10ms = 0;
static _Atomic uint64_t cgroups_lookup_duration_le_50ms = 0;
static _Atomic uint64_t cgroups_lookup_duration_le_100ms = 0;
static _Atomic uint64_t cgroups_lookup_duration_le_500ms = 0;
static _Atomic uint64_t cgroups_lookup_duration_le_1000ms = 0;
static _Atomic uint64_t cgroups_lookup_duration_gt_1000ms = 0;

static void cgroups_lookup_counter_inc(_Atomic uint64_t *counter)
{
    __atomic_add_fetch(counter, 1, __ATOMIC_RELAXED);
}

static uint64_t cgroups_lookup_counter_get(_Atomic uint64_t *counter)
{
    return __atomic_load_n(counter, __ATOMIC_RELAXED);
}

static void cgroups_lookup_duration_observe(usec_t duration_ut)
{
    uint64_t duration_ms = duration_ut / USEC_PER_MS;

    if (duration_ms <= 1)
        cgroups_lookup_counter_inc(&cgroups_lookup_duration_le_1ms);
    else if (duration_ms <= 5)
        cgroups_lookup_counter_inc(&cgroups_lookup_duration_le_5ms);
    else if (duration_ms <= 10)
        cgroups_lookup_counter_inc(&cgroups_lookup_duration_le_10ms);
    else if (duration_ms <= 50)
        cgroups_lookup_counter_inc(&cgroups_lookup_duration_le_50ms);
    else if (duration_ms <= 100)
        cgroups_lookup_counter_inc(&cgroups_lookup_duration_le_100ms);
    else if (duration_ms <= 500)
        cgroups_lookup_counter_inc(&cgroups_lookup_duration_le_500ms);
    else if (duration_ms <= 1000)
        cgroups_lookup_counter_inc(&cgroups_lookup_duration_le_1000ms);
    else
        cgroups_lookup_counter_inc(&cgroups_lookup_duration_gt_1000ms);
}

bool apps_cgroups_lookup_is_host_root_path(const char *path)
{
    return path && (strcmp(path, "/") == 0 || strcmp(path, "/init.scope") == 0);
}

static void cgroups_lookup_entry_clear_payload(struct cgroup_lookup_entry *entry)
{
    if (!entry)
        return;

    string_freez(entry->cgroup_name);
    entry->cgroup_name = NULL;

    for (uint16_t i = 0; i < entry->cgroup_label_count; i++) {
        string_freez(entry->cgroup_labels[i].key);
        string_freez(entry->cgroup_labels[i].value);
    }
    freez(entry->cgroup_labels);
    entry->cgroup_labels = NULL;
    entry->cgroup_label_count = 0;

    entry->orchestrator = NIPC_ORCHESTRATOR_UNKNOWN;
}

static void cgroups_lookup_entry_delete_cb(const DICTIONARY_ITEM *item __maybe_unused, void *value, void *data __maybe_unused)
{
    struct cgroup_lookup_entry *entry = value;

    if (!entry)
        return;

    string_freez(entry->key);
    cgroups_lookup_entry_clear_payload(entry);

    if (cgroups_lookup_cache_entries > 0)
        cgroups_lookup_cache_entries--;
}

static void cgroups_lookup_cache_destroy(void)
{
    if (!cgroups_lookup_cache)
        return;

    dictionary_destroy(cgroups_lookup_cache);
    cgroups_lookup_cache = NULL;
    cgroups_lookup_cache_entries = 0;
}

static void cgroups_lookup_cache_create(void)
{
    if (cgroups_lookup_cache)
        return;

    cgroups_lookup_cache = dictionary_create_advanced(
        DICT_OPTION_SINGLE_THREADED | DICT_OPTION_DONT_OVERWRITE_VALUE | DICT_OPTION_FIXED_SIZE,
        NULL,
        sizeof(struct cgroup_lookup_entry));
    dictionary_register_delete_callback(cgroups_lookup_cache, cgroups_lookup_entry_delete_cb, NULL);
}

static bool cgroups_lookup_cache_evict_one(void)
{
    if (!cgroups_lookup_cache || cgroups_lookup_cache_entries < APPS_CGROUPS_LOOKUP_CACHE_MAX)
        return true;

    struct cgroup_lookup_entry *victim = NULL;
    struct cgroup_lookup_entry *entry;
    dfe_start_read(cgroups_lookup_cache, entry) {
        if (entry->refcount != 0)
            continue;

        if (!victim) {
            victim = entry;
            continue;
        }

        if (entry->last_used_iteration < victim->last_used_iteration)
            victim = entry;
    }
    dfe_done(entry);

    if (!victim) {
        static uint64_t last_logged_iteration = 0;
        if (last_logged_iteration != global_iterations_counter) {
            last_logged_iteration = global_iterations_counter;
            netdata_log_error(
                "apps.plugin cgroups lookup cache is full (%u entries) and all entries are referenced",
                APPS_CGROUPS_LOOKUP_CACHE_MAX);
        }
        return false;
    }

    STRING *victim_key = string_dup(victim->key);
    dictionary_del(cgroups_lookup_cache, string2str(victim_key));
    string_freez(victim_key);
    cgroups_lookup_counter_inc(&cgroups_lookup_cache_evictions);
    return cgroups_lookup_cache_entries < APPS_CGROUPS_LOOKUP_CACHE_MAX;
}

static struct cgroup_lookup_entry *cgroups_lookup_cache_get_or_create(STRING *path)
{
    if (!path)
        return NULL;

    cgroups_lookup_cache_create();

    struct cgroup_lookup_entry *entry = dictionary_get(cgroups_lookup_cache, string2str(path));
    if (entry)
        return entry;

    if (!cgroups_lookup_cache_evict_one())
        return NULL;

    entry = dictionary_set(cgroups_lookup_cache, string2str(path), NULL, sizeof(*entry));
    entry->key = string_dup(path);
    entry->cgroup_status = NIPC_CGROUP_LOOKUP_UNKNOWN_RETRY_LATER;
    entry->orchestrator = NIPC_ORCHESTRATOR_UNKNOWN;
    entry->last_used_iteration = ++cgroups_lookup_cache_iteration;
    cgroups_lookup_cache_entries++;

    return entry;
}

void apps_cgroups_lookup_unlink_pid(struct pid_stat *p)
{
    if (!p || !p->cgroup_cache)
        return;

    struct cgroup_lookup_entry *entry = p->cgroup_cache;

    if (entry->refcount > 0)
        entry->refcount--;
    else
        netdata_log_error("apps.plugin cgroups lookup cache refcount underflow for '%s'",
                          string2str(entry->key));

    p->cgroup_cache = NULL;

    if (entry->refcount == 0 && cgroups_lookup_cache) {
        STRING *key = string_dup(entry->key);
        dictionary_del(cgroups_lookup_cache, string2str(key));
        string_freez(key);
        cgroups_lookup_counter_inc(&cgroups_lookup_cache_evictions);
    }
}

void apps_cgroups_lookup_set_pid_cgroup_path(struct pid_stat *p, const char *path)
{
    if (!p || !path || !*path)
        return;

    if (p->cgroup_path && strcmp(string2str(p->cgroup_path), path) == 0)
        return;

    STRING *new_path = string_strdupz(path);
    apps_cgroups_lookup_unlink_pid(p);
    string_freez(p->cgroup_path);
    p->cgroup_path = new_path;
}

static bool cgroups_lookup_queue_push(STRING *path)
{
    if (!path || !cgroups_lookup_queue_initialized || !cgroups_lookup_worker_thread)
        return false;

    bool pushed = false;
    netdata_mutex_lock(&cgroups_lookup_queue_mutex);

    if (cgroups_lookup_queue_count < APPS_CGROUPS_LOOKUP_QUEUE_MAX) {
        struct cgroup_lookup_queue_entry *entry = callocz(1, sizeof(*entry));
        entry->path = string_dup(path);

        if (cgroups_lookup_queue_tail)
            cgroups_lookup_queue_tail->next = entry;
        else
            cgroups_lookup_queue_head = entry;

        cgroups_lookup_queue_tail = entry;
        cgroups_lookup_queue_count++;
        __atomic_store_n(&cgroups_lookup_queue_depth, cgroups_lookup_queue_count, __ATOMIC_RELAXED);
        netdata_cond_signal(&cgroups_lookup_queue_cond);
        pushed = true;
    }

    netdata_mutex_unlock(&cgroups_lookup_queue_mutex);
    return pushed;
}

static uint32_t cgroups_lookup_queue_pop_batch(STRING **paths, uint32_t capacity)
{
    uint32_t count = 0;
    size_t encoded_size = NIPC_CGROUPS_LOOKUP_REQ_HDR_SIZE;

    netdata_mutex_lock(&cgroups_lookup_queue_mutex);

    while (count < capacity && cgroups_lookup_queue_head) {
        STRING *path = cgroups_lookup_queue_head->path;
        size_t path_len = string_strlen(path);
        size_t next_dir_size = ((size_t)count + 1) * NIPC_LOOKUP_DIR_ENTRY_SIZE;
        size_t data_start = NIPC_CGROUPS_LOOKUP_REQ_HDR_SIZE + next_dir_size;
        size_t item_start = nipc_align8(MAX(encoded_size, data_start));
        size_t next_size = item_start + path_len + 1;

        if (next_size > NIPC_MAX_PAYLOAD_CAP && count > 0)
            break;

        struct cgroup_lookup_queue_entry *entry = cgroups_lookup_queue_head;
        cgroups_lookup_queue_head = entry->next;
        if (!cgroups_lookup_queue_head)
            cgroups_lookup_queue_tail = NULL;

        cgroups_lookup_queue_count--;
        paths[count++] = entry->path;
        freez(entry);
        encoded_size = next_size;
    }

    __atomic_store_n(&cgroups_lookup_queue_depth, cgroups_lookup_queue_count, __ATOMIC_RELAXED);
    netdata_mutex_unlock(&cgroups_lookup_queue_mutex);
    return count;
}

static void cgroups_lookup_queue_wait_for_work(void)
{
    netdata_mutex_lock(&cgroups_lookup_queue_mutex);
    while (!cgroups_lookup_worker_stop && !cgroups_lookup_queue_head)
        netdata_cond_wait(&cgroups_lookup_queue_cond, &cgroups_lookup_queue_mutex);
    netdata_mutex_unlock(&cgroups_lookup_queue_mutex);
}

static void cgroups_lookup_queue_drain(void)
{
    netdata_mutex_lock(&cgroups_lookup_queue_mutex);
    struct cgroup_lookup_queue_entry *entry = cgroups_lookup_queue_head;
    cgroups_lookup_queue_head = NULL;
    cgroups_lookup_queue_tail = NULL;
    cgroups_lookup_queue_count = 0;
    __atomic_store_n(&cgroups_lookup_queue_depth, 0, __ATOMIC_RELAXED);
    netdata_mutex_unlock(&cgroups_lookup_queue_mutex);

    while (entry) {
        struct cgroup_lookup_queue_entry *next = entry->next;
        string_freez(entry->path);
        freez(entry);
        entry = next;
    }
}

static bool cgroups_lookup_copy_labels_from_item(
    struct cgroup_lookup_entry *entry,
    const nipc_cgroups_lookup_item_view_t *item)
{
    if (!entry || !item || item->label_count == 0)
        return true;

    entry->cgroup_labels = callocz(item->label_count, sizeof(*entry->cgroup_labels));
    entry->cgroup_label_count = item->label_count;

    for (uint32_t i = 0; i < item->label_count; i++) {
        nipc_lookup_label_view_t label;
        if (nipc_cgroups_lookup_item_label(item, i, &label) != NIPC_OK) {
            cgroups_lookup_entry_clear_payload(entry);
            return false;
        }

        entry->cgroup_labels[i].key = string_strndupz(label.key.ptr, label.key.len);
        entry->cgroup_labels[i].value = string_strndupz(label.value.ptr, label.value.len);
    }

    return true;
}

static void cgroups_lookup_commit_response_item(const nipc_cgroups_lookup_item_view_t *item, uint64_t generation)
{
    struct cgroup_lookup_entry *entry =
        dictionary_get_advanced(cgroups_lookup_cache, item->path.ptr, item->path.len);
    if (!entry)
        return;

    cgroups_lookup_entry_clear_payload(entry);
    entry->cgroup_status = item->status;
    entry->generation = generation;
    entry->pending = false;
    entry->last_used_iteration = ++cgroups_lookup_cache_iteration;

    if (item->status == NIPC_CGROUP_LOOKUP_KNOWN) {
        entry->orchestrator = item->orchestrator;
        entry->cgroup_name = string_strndupz(item->name.ptr, item->name.len);
        if (!cgroups_lookup_copy_labels_from_item(entry, item))
            entry->cgroup_status = NIPC_CGROUP_LOOKUP_UNKNOWN_RETRY_LATER;
        cgroups_lookup_counter_inc(&cgroups_lookup_cache_hits);
    }
    else if (item->status == NIPC_CGROUP_LOOKUP_UNKNOWN_PERMANENT) {
        cgroups_lookup_counter_inc(&cgroups_lookup_cache_misses_permanent);
    }
    else {
        cgroups_lookup_counter_inc(&cgroups_lookup_cache_misses_retry);
    }
}

static void cgroups_lookup_clear_pending_for_paths(STRING **paths, uint32_t count)
{
    netdata_mutex_lock(&apps_pids_mutex);
    for (uint32_t i = 0; i < count; i++) {
        struct cgroup_lookup_entry *entry = dictionary_get(cgroups_lookup_cache, string2str(paths[i]));
        if (entry)
            entry->pending = false;
    }
    netdata_mutex_unlock(&apps_pids_mutex);
}

static bool cgroups_lookup_client_ready(nipc_client_ctx_t *client, bool *was_ready, usec_t *next_retry_ut)
{
    if (nipc_client_ready(client)) {
        *was_ready = true;
        return true;
    }

    usec_t now_ut = now_monotonic_usec();
    if (now_ut < *next_retry_ut)
        return false;

    cgroups_lookup_counter_inc(&cgroups_lookup_peer_connect_attempts);
    (void)nipc_client_refresh(client);
    if (nipc_client_ready(client)) {
        if (!*was_ready)
            netdata_log_info("apps.plugin connected to CGROUPS_LOOKUP service");
        *was_ready = true;
        return true;
    }

    if (*was_ready) {
        cgroups_lookup_counter_inc(&cgroups_lookup_peer_disconnects);
        netdata_log_error("apps.plugin lost CGROUPS_LOOKUP service; cgroup enrichment will retry");
    }
    else {
        static bool logged_absent = false;
        if (!logged_absent) {
            logged_absent = true;
            netdata_log_error("apps.plugin CGROUPS_LOOKUP service unavailable; cgroup enrichment will retry");
        }
    }

    *was_ready = false;
    *next_retry_ut = now_ut + APPS_CGROUPS_LOOKUP_CONNECT_RETRY_SEC * USEC_PER_SEC;
    return false;
}

static void cgroups_lookup_worker(void *arg __maybe_unused)
{
    nipc_client_config_t config = {
        .supported_profiles = NIPC_PROFILE_BASELINE | NIPC_PROFILE_SHM_HYBRID | NIPC_PROFILE_SHM_FUTEX,
        .preferred_profiles = NIPC_PROFILE_SHM_FUTEX,
        .auth_token = netipc_auth_token(),
    };

    nipc_client_ctx_t client = { 0 };
    nipc_client_init(&client, os_run_dir(true), APPS_CGROUPS_LOOKUP_SERVICE_NAME, &config);

    bool was_ready = false;
    usec_t next_retry_ut = 0;
    uint64_t last_observed_generation = 0;
    bool request_failure_logged = false;

    while (!__atomic_load_n(&cgroups_lookup_worker_stop, __ATOMIC_ACQUIRE)) {
        cgroups_lookup_queue_wait_for_work();
        if (__atomic_load_n(&cgroups_lookup_worker_stop, __ATOMIC_ACQUIRE))
            break;

        if (!cgroups_lookup_client_ready(&client, &was_ready, &next_retry_ut)) {
            sleep_usec(USEC_PER_SEC);
            continue;
        }

        STRING *paths[APPS_CGROUPS_LOOKUP_BATCH_MAX] = { 0 };
        uint32_t count = cgroups_lookup_queue_pop_batch(paths, APPS_CGROUPS_LOOKUP_BATCH_MAX);
        if (count == 0)
            continue;

        nipc_str_view_t views[APPS_CGROUPS_LOOKUP_BATCH_MAX];
        for (uint32_t i = 0; i < count; i++) {
            views[i].ptr = string2str(paths[i]);
            views[i].len = (uint32_t)string_strlen(paths[i]);
        }

        nipc_cgroups_lookup_resp_view_t response = { 0 };
        cgroups_lookup_counter_inc(&cgroups_lookup_requests_sent);
        usec_t started_ut = now_monotonic_usec();
        nipc_error_t err = nipc_client_call_cgroups_lookup(&client, views, count, &response);
        cgroups_lookup_duration_observe(now_monotonic_usec() - started_ut);

        if (err != NIPC_OK) {
            cgroups_lookup_counter_inc(&cgroups_lookup_requests_error);
            if (!request_failure_logged) {
                netdata_log_error("apps.plugin CGROUPS_LOOKUP request failed (error %u); cgroup enrichment will retry",
                                  (unsigned int)err);
                request_failure_logged = true;
            }
            cgroups_lookup_clear_pending_for_paths(paths, count);
            was_ready = false;
        }
        else if (response.item_count != count) {
            cgroups_lookup_counter_inc(&cgroups_lookup_requests_error);
            if (!request_failure_logged) {
                netdata_log_error(
                    "apps.plugin CGROUPS_LOOKUP response item count mismatch (requested %u, received %u); cgroup enrichment will retry",
                    count,
                    response.item_count);
                request_failure_logged = true;
            }
            cgroups_lookup_clear_pending_for_paths(paths, count);
            was_ready = false;
        }
        else {
            nipc_cgroups_lookup_item_view_t items[APPS_CGROUPS_LOOKUP_BATCH_MAX];
            bool malformed = false;
            for (uint32_t i = 0; i < response.item_count; i++) {
                if (nipc_cgroups_lookup_resp_item(&response, i, &items[i]) != NIPC_OK) {
                    malformed = true;
                    break;
                }
            }

            if (malformed) {
                cgroups_lookup_counter_inc(&cgroups_lookup_requests_error);
                if (!request_failure_logged) {
                    netdata_log_error("apps.plugin CGROUPS_LOOKUP response decode failed; cgroup enrichment will retry");
                    request_failure_logged = true;
                }
                cgroups_lookup_clear_pending_for_paths(paths, count);
                was_ready = false;
                goto free_paths;
            }

            request_failure_logged = false;
            cgroups_lookup_counter_inc(&cgroups_lookup_requests_responded);

            netdata_mutex_lock(&apps_pids_mutex);
            if (response.generation > last_observed_generation)
                last_observed_generation = response.generation;

            for (uint32_t i = 0; i < response.item_count; i++)
                cgroups_lookup_commit_response_item(&items[i], response.generation);
            netdata_mutex_unlock(&apps_pids_mutex);
        }

free_paths:
        for (uint32_t i = 0; i < count; i++)
            string_freez(paths[i]);
    }

    nipc_client_close(&client);
}

void apps_cgroups_lookup_scan_pids(void)
{
    if (!cgroups_lookup_cache)
        cgroups_lookup_cache_create();

    for (struct pid_stat *p = root_of_pids(); p; p = p->next) {
        if (!p->cgroup_path)
            continue;

        if (apps_cgroups_lookup_is_host_root_path(string2str(p->cgroup_path))) {
            apps_cgroups_lookup_unlink_pid(p);
            continue;
        }

        if (p->cgroup_cache && p->cgroup_cache->key != p->cgroup_path &&
            strcmp(string2str(p->cgroup_cache->key), string2str(p->cgroup_path)) != 0)
            apps_cgroups_lookup_unlink_pid(p);

        if (!p->cgroup_cache) {
            struct cgroup_lookup_entry *entry = cgroups_lookup_cache_get_or_create(p->cgroup_path);
            if (!entry)
                continue;

            p->cgroup_cache = entry;
            entry->refcount++;
            entry->last_used_iteration = ++cgroups_lookup_cache_iteration;
        }

        if (p->cgroup_cache->cgroup_status == NIPC_CGROUP_LOOKUP_UNKNOWN_RETRY_LATER &&
            !p->cgroup_cache->pending) {
            p->cgroup_cache->pending = true;
            if (!cgroups_lookup_queue_push(p->cgroup_path))
                p->cgroup_cache->pending = false;
        }
    }
}

void apps_cgroups_lookup_init(void)
{
    cgroups_lookup_cache_create();

    if (!cgroups_lookup_queue_initialized) {
        netdata_mutex_init(&cgroups_lookup_queue_mutex);
        netdata_cond_init(&cgroups_lookup_queue_cond);
        cgroups_lookup_queue_initialized = true;
    }

    if (cgroups_lookup_worker_thread)
        return;

    __atomic_store_n(&cgroups_lookup_worker_stop, false, __ATOMIC_RELEASE);
    cgroups_lookup_worker_thread = nd_thread_create(
        "P[appscgroupipc]",
        NETDATA_THREAD_OPTION_DONT_LOG_STARTUP,
        cgroups_lookup_worker,
        NULL);

    if (!cgroups_lookup_worker_thread)
        netdata_log_error("apps.plugin failed to create CGROUPS_LOOKUP async worker");
}

void apps_cgroups_lookup_cleanup(void)
{
    if (cgroups_lookup_worker_thread) {
        __atomic_store_n(&cgroups_lookup_worker_stop, true, __ATOMIC_RELEASE);
        netdata_mutex_lock(&cgroups_lookup_queue_mutex);
        netdata_cond_broadcast(&cgroups_lookup_queue_cond);
        netdata_mutex_unlock(&cgroups_lookup_queue_mutex);
        nd_thread_join(cgroups_lookup_worker_thread);
        cgroups_lookup_worker_thread = NULL;
    }

    if (cgroups_lookup_queue_initialized) {
        cgroups_lookup_queue_drain();
        netdata_cond_destroy(&cgroups_lookup_queue_cond);
        netdata_mutex_destroy(&cgroups_lookup_queue_mutex);
        cgroups_lookup_queue_initialized = false;
    }

    cgroups_lookup_cache_destroy();
}

void apps_cgroups_lookup_send_charts_to_netdata(usec_t dt)
{
    static bool charts_created = false;

    if (!charts_created) {
        charts_created = true;
        fprintf(stdout,
                "CHART netdata.collector_ipc_cgroups_lookup_client_requests '' 'Apps Plugin CGROUPS_LOOKUP Client Requests' 'requests/s' apps.plugin netdata.collector.ipc.cgroups_lookup.client.requests line 140020 %d\n"
                "DIMENSION requests_sent '' incremental 1 1\n"
                "DIMENSION requests_responded '' incremental 1 1\n"
                "DIMENSION requests_timeout '' incremental 1 1\n"
                "DIMENSION requests_error '' incremental 1 1\n"
                "CHART netdata.collector_ipc_cgroups_lookup_client_cache '' 'Apps Plugin CGROUPS_LOOKUP Client Cache' 'events/s' apps.plugin netdata.collector.ipc.cgroups_lookup.client.cache line 140021 %d\n"
                "DIMENSION cache_hits '' incremental 1 1\n"
                "DIMENSION cache_misses_retry '' incremental 1 1\n"
                "DIMENSION cache_misses_permanent '' incremental 1 1\n"
                "DIMENSION cache_evictions '' incremental 1 1\n"
                "CHART netdata.collector_ipc_cgroups_lookup_client_peer '' 'Apps Plugin CGROUPS_LOOKUP Client Peer' 'events/s' apps.plugin netdata.collector.ipc.cgroups_lookup.client.peer line 140022 %d\n"
                "DIMENSION peer_connect_attempts '' incremental 1 1\n"
                "DIMENSION peer_disconnects '' incremental 1 1\n"
                "CHART netdata.collector_ipc_cgroups_lookup_client_duration '' 'Apps Plugin CGROUPS_LOOKUP Client Request Duration' 'requests/s' apps.plugin netdata.collector.ipc.cgroups_lookup.client.request_duration_ms stacked 140023 %d\n"
                "DIMENSION le_1ms '' incremental 1 1\n"
                "DIMENSION le_5ms '' incremental 1 1\n"
                "DIMENSION le_10ms '' incremental 1 1\n"
                "DIMENSION le_50ms '' incremental 1 1\n"
                "DIMENSION le_100ms '' incremental 1 1\n"
                "DIMENSION le_500ms '' incremental 1 1\n"
                "DIMENSION le_1000ms '' incremental 1 1\n"
                "DIMENSION gt_1000ms '' incremental 1 1\n"
                "CHART netdata.collector_ipc_cgroups_lookup_client_queue '' 'Apps Plugin CGROUPS_LOOKUP Client Queue Depth' 'paths' apps.plugin netdata.collector.ipc.cgroups_lookup.client.queue_depth line 140024 %d\n"
                "DIMENSION queue_depth '' absolute 1 1\n",
                update_every, update_every, update_every, update_every, update_every);
    }

    fprintf(stdout,
            "BEGIN netdata.collector_ipc_cgroups_lookup_client_requests %" PRIu64 "\n"
            "SET requests_sent = %" PRIu64 "\n"
            "SET requests_responded = %" PRIu64 "\n"
            "SET requests_timeout = %" PRIu64 "\n"
            "SET requests_error = %" PRIu64 "\n"
            "END\n"
            "BEGIN netdata.collector_ipc_cgroups_lookup_client_cache %" PRIu64 "\n"
            "SET cache_hits = %" PRIu64 "\n"
            "SET cache_misses_retry = %" PRIu64 "\n"
            "SET cache_misses_permanent = %" PRIu64 "\n"
            "SET cache_evictions = %" PRIu64 "\n"
            "END\n"
            "BEGIN netdata.collector_ipc_cgroups_lookup_client_peer %" PRIu64 "\n"
            "SET peer_connect_attempts = %" PRIu64 "\n"
            "SET peer_disconnects = %" PRIu64 "\n"
            "END\n"
            "BEGIN netdata.collector_ipc_cgroups_lookup_client_duration %" PRIu64 "\n"
            "SET le_1ms = %" PRIu64 "\n"
            "SET le_5ms = %" PRIu64 "\n"
            "SET le_10ms = %" PRIu64 "\n"
            "SET le_50ms = %" PRIu64 "\n"
            "SET le_100ms = %" PRIu64 "\n"
            "SET le_500ms = %" PRIu64 "\n"
            "SET le_1000ms = %" PRIu64 "\n"
            "SET gt_1000ms = %" PRIu64 "\n"
            "END\n"
            "BEGIN netdata.collector_ipc_cgroups_lookup_client_queue %" PRIu64 "\n"
            "SET queue_depth = %" PRIu64 "\n"
            "END\n",
            dt,
            cgroups_lookup_counter_get(&cgroups_lookup_requests_sent),
            cgroups_lookup_counter_get(&cgroups_lookup_requests_responded),
            cgroups_lookup_counter_get(&cgroups_lookup_requests_timeout),
            cgroups_lookup_counter_get(&cgroups_lookup_requests_error),
            dt,
            cgroups_lookup_counter_get(&cgroups_lookup_cache_hits),
            cgroups_lookup_counter_get(&cgroups_lookup_cache_misses_retry),
            cgroups_lookup_counter_get(&cgroups_lookup_cache_misses_permanent),
            cgroups_lookup_counter_get(&cgroups_lookup_cache_evictions),
            dt,
            cgroups_lookup_counter_get(&cgroups_lookup_peer_connect_attempts),
            cgroups_lookup_counter_get(&cgroups_lookup_peer_disconnects),
            dt,
            cgroups_lookup_counter_get(&cgroups_lookup_duration_le_1ms),
            cgroups_lookup_counter_get(&cgroups_lookup_duration_le_5ms),
            cgroups_lookup_counter_get(&cgroups_lookup_duration_le_10ms),
            cgroups_lookup_counter_get(&cgroups_lookup_duration_le_50ms),
            cgroups_lookup_counter_get(&cgroups_lookup_duration_le_100ms),
            cgroups_lookup_counter_get(&cgroups_lookup_duration_le_500ms),
            cgroups_lookup_counter_get(&cgroups_lookup_duration_le_1000ms),
            cgroups_lookup_counter_get(&cgroups_lookup_duration_gt_1000ms),
            dt,
            cgroups_lookup_counter_get(&cgroups_lookup_queue_depth));
}

#endif /* OS_LINUX */
