// SPDX-License-Identifier: GPL-3.0-or-later

#include "libnetdata/netipc/netipc_netdata.h"

static const char *test_run_dir = NULL;

static const char *test_os_run_dir(bool rw __maybe_unused)
{
    return test_run_dir;
}

netdata_mutex_t apps_pids_mutex;
uint64_t apps_collection_generation = 0;
int update_every = 1;
size_t global_iterations_counter = 1;

#define os_run_dir(rw) test_os_run_dir(rw)
#include "../apps-cgroups-lookup-client.c"
#undef os_run_dir

static struct pid_stat test_pid = { 0 };
static uint64_t blocking_mock_requests = 0;
static bool blocking_mock_release = false;

struct pid_stat *root_of_pids(void)
{
    return &test_pid;
}

static bool expect_ok(bool condition, const char *message)
{
    if (condition)
        return true;

    fprintf(stderr, "%s\n", message);
    return false;
}

static bool wait_for_counter(uint64_t *counter, uint64_t value)
{
    for (size_t i = 0; i < 500; i++) {
        if (__atomic_load_n(counter, __ATOMIC_ACQUIRE) >= value)
            return true;

        sleep_usec(10000);
    }

    return false;
}

static bool test_cache_allows_more_than_legacy_fixed_cap(void)
{
    const size_t legacy_fixed_cap = 4096;
    bool ok = true;

    cgroups_lookup_cache_create();

    for (size_t i = 0; i <= legacy_fixed_cap; i++) {
        char path[128];
        snprintfz(path, sizeof(path), "/kubepods.slice/test-%zu.scope", i);

        STRING *path_string = string_strdupz(path);
        struct cgroup_lookup_entry *entry = cgroups_lookup_cache_get_or_create(path_string);
        string_freez(path_string);

        if (!expect_ok(entry != NULL, "active cgroup lookup cache entry was rejected")) {
            ok = false;
            break;
        }

        entry->refcount = 1;
    }

    ok = expect_ok(dictionary_entries(cgroups_lookup_cache) == legacy_fixed_cap + 1,
                   "active cgroup lookup cache did not retain all active entries") && ok;

    cgroups_lookup_cache_destroy();
    return ok;
}

static bool test_retry_later_generation_reset(void)
{
    bool ok = true;

    cgroups_lookup_cache_create();
    cgroups_lookup_latest_generation = 42;

    STRING *retry_path = string_strdupz("/kubepods.slice/retry.scope");
    struct cgroup_lookup_entry *retry = cgroups_lookup_cache_get_or_create(retry_path);
    retry->refcount = 1;
    retry->cgroup_status = NIPC_CGROUP_LOOKUP_UNKNOWN_RETRY_LATER;
    retry->generation = 42;
    retry->pending = true;

    STRING *known_path = string_strdupz("/kubepods.slice/known.scope");
    struct cgroup_lookup_entry *known = cgroups_lookup_cache_get_or_create(known_path);
    known->refcount = 1;
    known->cgroup_status = NIPC_CGROUP_LOOKUP_KNOWN;
    known->generation = 42;
    known->pending = false;

    cgroups_lookup_note_peer_generation_reset();

    ok =
        expect_ok(cgroups_lookup_latest_generation == 0, "latest cgroups generation was not reset") &&
        expect_ok(retry->generation == 0, "retry-later entry generation was not reset") &&
        expect_ok(retry->pending, "retry-later pending state was unexpectedly changed") &&
        expect_ok(known->generation == 42, "known entry generation was unexpectedly reset") &&
        ok;

    retry->refcount = 0;
    known->refcount = 0;
    string_freez(retry_path);
    string_freez(known_path);
    cgroups_lookup_cache_destroy();
    return ok;
}

static bool test_retry_later_same_generation_not_requeued(void)
{
    bool ok = true;
    bool queue_mutex_initialized = false;
    bool queue_cond_initialized = false;

    cgroups_lookup_cache_create();
    cgroups_lookup_latest_generation = 7;

    if (!expect_ok(netdata_mutex_init(&cgroups_lookup_queue_mutex) == 0, "queue mutex init failed"))
        goto cleanup;
    queue_mutex_initialized = true;

    if (!expect_ok(netdata_cond_init(&cgroups_lookup_queue_cond) == 0, "queue cond init failed"))
        goto cleanup;
    queue_cond_initialized = true;

    cgroups_lookup_queue_initialized = true;
    cgroups_lookup_worker_thread = (ND_THREAD *)0x1;

    test_pid = (struct pid_stat){ 0 };
    test_pid.pid = 22345;
    test_pid.cgroup_path = string_strdupz(
        "/../../kubepods.slice/pod-a/cri-containerd-0123456789abcdef.scope");
    test_pid.cgroup_cache = cgroups_lookup_cache_get_or_create(test_pid.cgroup_path);
    test_pid.cgroup_cache->refcount = 1;
    test_pid.cgroup_cache->cgroup_status = NIPC_CGROUP_LOOKUP_UNKNOWN_RETRY_LATER;
    test_pid.cgroup_cache->generation = 7;
    test_pid.cgroup_cache->pending = false;

    apps_cgroups_lookup_scan_pids();
    ok =
        expect_ok(cgroups_lookup_queue_count == 0, "same-generation retry-later path was requeued") &&
        expect_ok(!test_pid.cgroup_cache->pending, "same-generation retry-later entry was marked pending") &&
        ok;

    cgroups_lookup_latest_generation = 8;
    apps_cgroups_lookup_scan_pids();
    ok =
        expect_ok(cgroups_lookup_queue_count == 1, "new-generation retry-later path was not requeued") &&
        expect_ok(test_pid.cgroup_cache->pending, "new-generation retry-later entry was not marked pending") &&
        ok;

cleanup:
    if (cgroups_lookup_queue_initialized)
        cgroups_lookup_queue_drain();
    cgroups_lookup_queue_initialized = false;
    cgroups_lookup_worker_thread = NULL;
    if (queue_cond_initialized)
        netdata_cond_destroy(&cgroups_lookup_queue_cond);
    if (queue_mutex_initialized)
        netdata_mutex_destroy(&cgroups_lookup_queue_mutex);

    test_pid.cgroup_cache = NULL;
    string_freez(test_pid.cgroup_path);
    test_pid.cgroup_path = NULL;
    cgroups_lookup_latest_generation = 0;
    cgroups_lookup_cache_destroy();
    return ok;
}

static bool blocking_cgroups_lookup_handler(
    void *user __maybe_unused,
    const nipc_cgroups_lookup_req_view_t *request,
    nipc_cgroups_lookup_builder_t *builder)
{
    __atomic_add_fetch(&blocking_mock_requests, 1, __ATOMIC_RELEASE);
    while (!__atomic_load_n(&blocking_mock_release, __ATOMIC_ACQUIRE))
        sleep_usec(10000);

    nipc_cgroups_lookup_builder_set_generation(builder, 7);
    for (uint32_t i = 0; i < request->item_count; i++) {
        nipc_cgroups_lookup_req_item_t req_item;
        nipc_error_t err = nipc_cgroups_lookup_req_item(request, i, &req_item);
        if (err != NIPC_OK) {
            builder->error = err;
            return false;
        }

        err = nipc_cgroups_lookup_builder_add(
            builder,
            NIPC_CGROUP_LOOKUP_UNKNOWN_RETRY_LATER,
            NIPC_ORCHESTRATOR_UNKNOWN,
            req_item.path.ptr,
            req_item.path.len,
            NULL,
            0,
            NULL,
            0);
        if (err != NIPC_OK) {
            builder->error = err;
            return false;
        }
    }

    return true;
}

static void mock_server_thread(void *arg)
{
    nipc_server_run((nipc_managed_server_t *)arg);
}

int main(void)
{
    if (nd_environment_init() != 0 || nd_environment_freeze_process() != 0) {
        fprintf(stderr, "failed to initialize and freeze the process environment: %s\n", strerror(errno));
        return 1;
    }

    char temp_dir[] = "./apps-cgroups-lookup-abort-test.XXXXXX";
    nipc_managed_server_t server = { 0 };
    ND_THREAD *server_thread = NULL;
    bool server_initialized = false;
    bool client_initialized = false;
    bool mutex_initialized = false;
    bool ok = false;

    netdata_configured_host_prefix = "";
    if (!mkdtemp(temp_dir)) {
        perror("mkdtemp");
        return 1;
    }
    test_run_dir = temp_dir;

    if (!expect_ok(netdata_mutex_init(&apps_pids_mutex) == 0, "failed to initialize apps_pids_mutex"))
        goto cleanup_dir;
    mutex_initialized = true;

    if (!expect_ok(test_cache_allows_more_than_legacy_fixed_cap(), "active cgroup cache cardinality test failed"))
        goto cleanup_fixture;
    if (!expect_ok(test_retry_later_generation_reset(), "retry-later generation reset test failed"))
        goto cleanup_fixture;
    if (!expect_ok(
            test_retry_later_same_generation_not_requeued(),
            "retry-later same-generation requeue test failed"))
        goto cleanup_fixture;

    test_pid.pid = 12345;
    test_pid.cgroup_path = string_strdupz("/docker/apps-cgroups-abort");

    nipc_server_config_t config = {
        .supported_profiles = NIPC_PROFILE_BASELINE,
        .preferred_profiles = NIPC_PROFILE_BASELINE,
        .auth_token = netipc_auth_token(),
    };
    nipc_cgroups_lookup_service_handler_t handler = {
        .handle = blocking_cgroups_lookup_handler,
        .user = NULL,
    };

    nipc_error_t err = nipc_server_init_cgroups_lookup(
        &server,
        temp_dir,
        APPS_CGROUPS_LOOKUP_SERVICE_NAME,
        &config,
        1,
        &handler);
    if (!expect_ok(err == NIPC_OK, "blocking CGROUPS_LOOKUP server init failed"))
        goto cleanup_fixture;
    server_initialized = true;

    server_thread = nd_thread_create("cglookup-b", 0, mock_server_thread, &server);
    if (!expect_ok(server_thread != NULL, "blocking CGROUPS_LOOKUP server thread creation failed"))
        goto cleanup_server;

    apps_cgroups_lookup_init();
    client_initialized = true;
    apps_cgroups_lookup_scan_pids();

    if (!expect_ok(wait_for_counter(&blocking_mock_requests, 1), "worker did not enter blocking CGROUPS_LOOKUP call"))
        goto cleanup_client;

    usec_t started_ut = now_monotonic_usec();
    apps_cgroups_lookup_cleanup();
    usec_t elapsed_ut = now_monotonic_usec() - started_ut;
    client_initialized = false;

    fprintf(
        stderr,
        "CGROUPS_LOOKUP abort-during-call cleanup elapsed=%" PRIu64 "us requests=%" PRIu64 "\n",
        (uint64_t)elapsed_ut,
        __atomic_load_n(&blocking_mock_requests, __ATOMIC_ACQUIRE));

    ok =
        expect_ok(cgroups_lookup_worker_thread == NULL, "CGROUPS_LOOKUP worker thread was not cleared") &&
        expect_ok(!__atomic_load_n(&cgroups_lookup_client_initialized, __ATOMIC_ACQUIRE),
                  "CGROUPS_LOOKUP client context remained initialized") &&
        expect_ok(elapsed_ut < 2 * USEC_PER_SEC, "CGROUPS_LOOKUP cleanup waited for timeout instead of aborting the call");

cleanup_client:
    __atomic_store_n(&blocking_mock_release, true, __ATOMIC_RELEASE);
    if (client_initialized)
        apps_cgroups_lookup_cleanup();
cleanup_server:
    __atomic_store_n(&blocking_mock_release, true, __ATOMIC_RELEASE);
    if (server_initialized)
        nipc_server_stop(&server);
    if (server_thread)
        nd_thread_join(server_thread);
    if (server_initialized) {
        nipc_server_drain(&server, 5000);
        nipc_server_destroy(&server);
    }
cleanup_fixture:
    test_pid.cgroup_cache = NULL;
    string_freez(test_pid.cgroup_path);
    test_pid.cgroup_path = NULL;
    if (mutex_initialized)
        netdata_mutex_destroy(&apps_pids_mutex);
cleanup_dir:
    (void)rmdir(temp_dir);
    return ok ? 0 : 1;
}
