// SPDX-License-Identifier: GPL-3.0-or-later

#include "libnetdata/netipc/netipc_netdata.h"

#include <sys/stat.h>

static const char *test_run_dir = NULL;

static const char *test_os_run_dir(bool rw __maybe_unused)
{
    return test_run_dir;
}

#define os_run_dir(rw) test_os_run_dir(rw)
#include "../network-viewer-apps-lookup-client.c"
#undef os_run_dir

static uint64_t mock_requests = 0;
static uint64_t blocking_mock_requests = 0;
static bool blocking_mock_release = false;

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

static bool wait_for_cache_entries(uint32_t value)
{
    for (size_t i = 0; i < 500; i++) {
        netdata_mutex_lock(&apps_lookup_cache_mutex);
        bool ok = apps_lookup_cache_size >= value;
        netdata_mutex_unlock(&apps_lookup_cache_mutex);
        if (ok)
            return true;

        sleep_usec(10000);
    }

    return false;
}

static uint64_t counter_value(uint64_t *counter)
{
    return __atomic_load_n(counter, __ATOMIC_ACQUIRE);
}

static bool mock_apps_lookup_handler(
    void *user __maybe_unused,
    const nipc_apps_lookup_req_view_t *request,
    nipc_apps_lookup_builder_t *builder)
{
    __atomic_add_fetch(&mock_requests, 1, __ATOMIC_RELEASE);
    nipc_apps_lookup_builder_set_generation(builder, 1);

    for (uint32_t i = 0; i < request->item_count; i++) {
        nipc_apps_lookup_req_item_t req_item;
        nipc_error_t err = nipc_apps_lookup_req_item(request, i, &req_item);
        if (err != NIPC_OK) {
            builder->error = err;
            return false;
        }

        if (req_item.pid == 100) {
            nipc_lookup_label_view_t labels[1] = {
                {
                    .key = { .ptr = "image", .len = 5 },
                    .value = { .ptr = "netdata/test", .len = 12 },
                },
            };

            err = nipc_apps_lookup_builder_add(
                builder,
                NIPC_PID_LOOKUP_KNOWN,
                NIPC_APPS_CGROUP_KNOWN,
                NIPC_ORCHESTRATOR_DOCKER,
                req_item.pid,
                1,
                1000,
                123456789,
                "proc-a",
                6,
                "/docker/test",
                12,
                "test-container",
                14,
                labels,
                1);
        }
        else if (req_item.pid == 200) {
            err = nipc_apps_lookup_builder_add(
                builder,
                NIPC_PID_LOOKUP_KNOWN,
                NIPC_APPS_CGROUP_UNKNOWN_RETRY_LATER,
                NIPC_ORCHESTRATOR_UNKNOWN,
                req_item.pid,
                1,
                1000,
                223456789,
                "proc-b",
                6,
                NULL,
                0,
                NULL,
                0,
                NULL,
                0);
        }
        else if (req_item.pid == 300) {
            err = nipc_apps_lookup_builder_add(
                builder,
                NIPC_PID_LOOKUP_KNOWN,
                NIPC_APPS_CGROUP_UNKNOWN_PERMANENT,
                NIPC_ORCHESTRATOR_UNKNOWN,
                req_item.pid,
                1,
                955,
                323456789,
                "systemd-journal",
                15,
                "/system.slice/systemd-journal-upload.service",
                44,
                NULL,
                0,
                NULL,
                0);
        }
        else if (req_item.pid == 400) {
            err = nipc_apps_lookup_builder_add(
                builder,
                NIPC_PID_LOOKUP_KNOWN,
                NIPC_APPS_CGROUP_KNOWN,
                NIPC_ORCHESTRATOR_DOCKER,
                req_item.pid,
                1,
                1000,
                423456789,
                "proc-d",
                6,
                "/docker/other",
                13,
                "other-container",
                15,
                NULL,
                0);
        }
        else {
            err = nipc_apps_lookup_builder_add(
                builder,
                NIPC_PID_LOOKUP_UNKNOWN,
                0,
                0,
                req_item.pid,
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

static bool blocking_apps_lookup_handler(
    void *user __maybe_unused,
    const nipc_apps_lookup_req_view_t *request,
    nipc_apps_lookup_builder_t *builder)
{
    __atomic_add_fetch(&blocking_mock_requests, 1, __ATOMIC_RELEASE);
    while (!__atomic_load_n(&blocking_mock_release, __ATOMIC_ACQUIRE))
        sleep_usec(10000);

    nipc_apps_lookup_builder_set_generation(builder, 2);
    for (uint32_t i = 0; i < request->item_count; i++) {
        nipc_apps_lookup_req_item_t req_item;
        nipc_error_t err = nipc_apps_lookup_req_item(request, i, &req_item);
        if (err != NIPC_OK) {
            builder->error = err;
            return false;
        }

        err = nipc_apps_lookup_builder_add(
            builder,
            NIPC_PID_LOOKUP_UNKNOWN,
            0,
            0,
            req_item.pid,
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
        if (err != NIPC_OK) {
            builder->error = err;
            return false;
        }
    }

    return true;
}

static bool run_roundtrip_test(const char *run_dir)
{
    int rc = 1;
    bool plugin_should_exit = false;
    nipc_managed_server_t server = { 0 };
    ND_THREAD *server_thread = NULL;
    bool server_initialized = false;
    bool client_initialized = false;

    nipc_server_config_t config = {
        .supported_profiles = NIPC_PROFILE_BASELINE,
        .preferred_profiles = NIPC_PROFILE_BASELINE,
        .auth_token = netipc_auth_token(),
    };
    nipc_apps_lookup_service_handler_t handler = {
        .handle = mock_apps_lookup_handler,
        .user = NULL,
    };

    nipc_error_t err = nipc_server_init_apps_lookup(
        &server,
        run_dir,
        NV_APPS_LOOKUP_SERVICE_NAME,
        &config,
        1,
        &handler);
    if (!expect_ok(err == NIPC_OK, "mock APPS_LOOKUP server init failed"))
        goto cleanup_server;
    server_initialized = true;

    char socket_path[FILENAME_MAX + 1];
    snprintfz(socket_path, sizeof(socket_path), "%s/%s.sock", run_dir, NV_APPS_LOOKUP_SERVICE_NAME);
    struct stat socket_stat;
    if (!expect_ok(stat(socket_path, &socket_stat) == 0, "mock APPS_LOOKUP socket stat failed"))
        goto cleanup_server;
    if (!expect_ok((socket_stat.st_mode & 0777) == 0600, "mock APPS_LOOKUP socket mode is not owner-only"))
        goto cleanup_server;

    server_thread = nd_thread_create("appslookup-t", 0, mock_server_thread, &server);
    if (!expect_ok(server_thread != NULL, "mock APPS_LOOKUP server thread creation failed"))
        goto cleanup_server;

    nv_apps_lookup_init(&plugin_should_exit);
    nv_apps_lookup_start();
    client_initialized = true;

    uint32_t pids[] = { 100, 200, 300 };
    nv_apps_lookup_warm_pids(pids, _countof(pids));

    if (!expect_ok(wait_for_counter(&mock_requests, 1), "worker did not call APPS_LOOKUP mock"))
        goto cleanup_client;
    if (!expect_ok(wait_for_cache_entries(3), "known, retry-later, and permanent PIDs were not cached"))
        goto cleanup_client;
    NV_APPS_LOOKUP_FIELDS cached;
    if (!expect_ok(nv_cache_lookup_pid(100, 123456789, &cached), "known PID cache accessor missed"))
        goto cleanup_client;
    bool cached_ok =
        expect_ok(cached.cgroup_status == NIPC_APPS_CGROUP_KNOWN, "cached cgroup status mismatch") &&
        expect_ok(cached.orchestrator == NIPC_ORCHESTRATOR_DOCKER, "cached orchestrator mismatch") &&
        expect_ok(strcmp(cached.cgroup_path, "/docker/test") == 0, "cached cgroup path mismatch") &&
        expect_ok(strcmp(cached.cgroup_name, "test-container") == 0, "cached cgroup name mismatch") &&
        expect_ok(cached.cgroup_label_count == 1, "cached label count mismatch") &&
        expect_ok(strcmp(cached.cgroup_labels[0].key, "image") == 0, "cached label key mismatch") &&
        expect_ok(strcmp(cached.cgroup_labels[0].value, "netdata/test") == 0, "cached label value mismatch");
    nv_cache_lookup_fields_free(&cached);
    if (!cached_ok)
        goto cleanup_client;
    if (!expect_ok(nv_cache_lookup_pid(200, 223456789, &cached), "retry-later PID partial cache accessor missed"))
        goto cleanup_client;
    bool partial_ok =
        expect_ok(cached.cgroup_status == NIPC_APPS_CGROUP_UNKNOWN_RETRY_LATER, "retry-later PID cgroup status mismatch") &&
        expect_ok(cached.orchestrator == NIPC_ORCHESTRATOR_UNKNOWN, "retry-later PID orchestrator mismatch") &&
        expect_ok(cached.cgroup_path && cached.cgroup_path[0] == '\0', "retry-later PID cgroup path should be empty") &&
        expect_ok(cached.cgroup_name && cached.cgroup_name[0] == '\0', "retry-later PID cgroup name should be empty");
    nv_cache_lookup_fields_free(&cached);
    if (!partial_ok)
        goto cleanup_client;
    if (!expect_ok(nv_cache_lookup_pid(300, 323456789, &cached), "permanent PID partial cache accessor missed"))
        goto cleanup_client;
    bool permanent_ok =
        expect_ok(cached.cgroup_status == NIPC_APPS_CGROUP_UNKNOWN_PERMANENT, "permanent PID cgroup status mismatch") &&
        expect_ok(cached.orchestrator == NIPC_ORCHESTRATOR_UNKNOWN, "permanent PID orchestrator mismatch") &&
        expect_ok(strcmp(cached.cgroup_path, "/system.slice/systemd-journal-upload.service") == 0,
                  "permanent PID cgroup path mismatch") &&
        expect_ok(cached.cgroup_name && cached.cgroup_name[0] == '\0', "permanent PID cgroup name should be empty") &&
        expect_ok(cached.cgroup_label_count == 0, "permanent PID label count mismatch");
    nv_cache_lookup_fields_free(&cached);
    if (!permanent_ok)
        goto cleanup_client;
    if (!expect_ok(wait_for_counter(&apps_lookup_cache_misses_unknown, 1), "retry-later PID was not counted as unknown miss"))
        goto cleanup_client;

    nv_apps_lookup_warm_pids(pids, _countof(pids));

    if (!expect_ok(wait_for_counter(&apps_lookup_cache_hits, 1), "cached known PID was not served as a hit"))
        goto cleanup_client;
    sleep_usec(100000);
    if (!expect_ok(counter_value(&apps_lookup_requests_responded) == 1, "retry-later PID retried before backoff elapsed"))
        goto cleanup_client;
    bool retry_entry_found = false;
    netdata_mutex_lock(&apps_lookup_cache_mutex);
    NV_APPS_LOOKUP_CACHE_ENTRY *retry_entry = dictionary_get(apps_lookup_cache, "200");
    if (retry_entry) {
        retry_entry->retry_after_usec = 0;
        retry_entry_found = true;
    }
    netdata_mutex_unlock(&apps_lookup_cache_mutex);
    if (!expect_ok(retry_entry_found, "retry-later PID cache entry missing"))
        goto cleanup_client;
    nv_apps_lookup_warm_pids(pids, _countof(pids));
    if (!expect_ok(wait_for_counter(&apps_lookup_cache_misses_unknown, 2), "retry-later PID was not retried after backoff elapsed"))
        goto cleanup_client;
    if (!expect_ok(wait_for_counter(&apps_lookup_requests_responded, 2), "worker did not complete the retry-later refresh"))
        goto cleanup_client;
    if (!expect_ok(__atomic_load_n(&apps_lookup_requests_failed, __ATOMIC_ACQUIRE) == 0, "client recorded a failed request"))
        goto cleanup_client;

    uint32_t disjoint_pid[] = { 400 };
    nv_apps_lookup_warm_pids(disjoint_pid, _countof(disjoint_pid));

    if (!expect_ok(wait_for_counter(&mock_requests, 3), "worker did not warm the disjoint PID"))
        goto cleanup_client;
    if (!expect_ok(wait_for_cache_entries(4), "disjoint PID warm pruned existing cache entries"))
        goto cleanup_client;
    if (!expect_ok(nv_cache_lookup_pid(100, 123456789, &cached), "known PID was evicted by disjoint warm"))
        goto cleanup_client;
    nv_cache_lookup_fields_free(&cached);
    if (!expect_ok(nv_cache_lookup_pid(400, 423456789, &cached), "disjoint known PID cache accessor missed"))
        goto cleanup_client;
    bool disjoint_ok =
        expect_ok(cached.cgroup_status == NIPC_APPS_CGROUP_KNOWN, "disjoint PID cgroup status mismatch") &&
        expect_ok(strcmp(cached.cgroup_path, "/docker/other") == 0, "disjoint PID cgroup path mismatch") &&
        expect_ok(strcmp(cached.cgroup_name, "other-container") == 0, "disjoint PID cgroup name mismatch");
    nv_cache_lookup_fields_free(&cached);
    if (!disjoint_ok)
        goto cleanup_client;

    uint64_t worker_le_50ms =
        counter_value(&apps_lookup_worker_duration_le_1ms) +
        counter_value(&apps_lookup_worker_duration_le_5ms) +
        counter_value(&apps_lookup_worker_duration_le_10ms) +
        counter_value(&apps_lookup_worker_duration_le_50ms);
    if (!expect_ok(worker_le_50ms >= 2, "mock worker requests exceeded the local 50ms bucket"))
        goto cleanup_client;

    uint32_t known_pid[] = { 100 };
    uint64_t requests_before_known_hits = counter_value(&mock_requests);
    for (size_t i = 0; i < 10; i++)
        nv_apps_lookup_warm_pids(known_pid, _countof(known_pid));

    if (!expect_ok(wait_for_counter(&apps_lookup_cache_hits, 11), "known PID cache hit ratio check failed"))
        goto cleanup_client;
    sleep_usec(100000);
    if (!expect_ok(
            counter_value(&mock_requests) == requests_before_known_hits,
            "stable known PID cache hits triggered extra APPS_LOOKUP requests"))
        goto cleanup_client;
    uint64_t handler_le_5ms =
        counter_value(&apps_lookup_handler_overhead_le_1ms) +
        counter_value(&apps_lookup_handler_overhead_le_5ms);
    if (!expect_ok(handler_le_5ms >= 12, "Function handler overhead exceeded the local 5ms bucket"))
        goto cleanup_client;

    uint64_t pid_reuse_evictions_before = counter_value(&apps_lookup_cache_evictions_pid_reuse);
    if (!expect_ok(!nv_cache_lookup_pid(100, 999999, &cached), "PID reuse cache accessor returned stale entry"))
        goto cleanup_client;
    if (!expect_ok(
            counter_value(&apps_lookup_cache_evictions_pid_reuse) == pid_reuse_evictions_before + 1,
            "PID reuse cache accessor did not record eviction"))
        goto cleanup_client;

    rc = 0;

cleanup_client:
    if (client_initialized) {
        __atomic_store_n(&plugin_should_exit, true, __ATOMIC_RELEASE);
        nv_apps_lookup_stop();
    }
cleanup_server:
    if (server_initialized)
        nipc_server_stop(&server);
    if (server_thread)
        nd_thread_join(server_thread);
    if (server_initialized) {
        nipc_server_drain(&server, 5000);
        nipc_server_destroy(&server);
    }
    return rc == 0;
}

static bool run_abort_during_call_test(const char *run_dir)
{
    bool plugin_should_exit = false;
    nipc_managed_server_t server = { 0 };
    ND_THREAD *server_thread = NULL;
    bool server_initialized = false;
    bool client_initialized = false;
    bool ok = false;

    __atomic_store_n(&blocking_mock_requests, 0, __ATOMIC_RELEASE);
    __atomic_store_n(&blocking_mock_release, false, __ATOMIC_RELEASE);

    nipc_server_config_t config = {
        .supported_profiles = NIPC_PROFILE_BASELINE,
        .preferred_profiles = NIPC_PROFILE_BASELINE,
        .auth_token = netipc_auth_token(),
    };
    nipc_apps_lookup_service_handler_t handler = {
        .handle = blocking_apps_lookup_handler,
        .user = NULL,
    };

    nipc_error_t err = nipc_server_init_apps_lookup(
        &server,
        run_dir,
        NV_APPS_LOOKUP_SERVICE_NAME,
        &config,
        1,
        &handler);
    if (!expect_ok(err == NIPC_OK, "blocking APPS_LOOKUP server init failed"))
        goto cleanup_server;
    server_initialized = true;

    server_thread = nd_thread_create("appslookup-b", 0, mock_server_thread, &server);
    if (!expect_ok(server_thread != NULL, "blocking APPS_LOOKUP server thread creation failed"))
        goto cleanup_server;

    nv_apps_lookup_init(&plugin_should_exit);
    nv_apps_lookup_start();
    client_initialized = true;

    uint32_t pid = 300;
    nv_apps_lookup_warm_pids(&pid, 1);

    if (!expect_ok(wait_for_counter(&blocking_mock_requests, 1), "worker did not enter blocking APPS_LOOKUP call"))
        goto cleanup_client;

    __atomic_store_n(&plugin_should_exit, true, __ATOMIC_RELEASE);
    usec_t started_ut = now_monotonic_usec();
    nv_apps_lookup_stop();
    usec_t elapsed_ut = now_monotonic_usec() - started_ut;
    client_initialized = false;

    fprintf(
        stderr,
        "APPS_LOOKUP abort-during-call stop elapsed=%" PRIu64 "us requests=%" PRIu64 "\n",
        (uint64_t)elapsed_ut,
        __atomic_load_n(&blocking_mock_requests, __ATOMIC_ACQUIRE));

    ok =
        expect_ok(nv_apps_lookup_worker_exited(), "APPS_LOOKUP worker did not report exit") &&
        expect_ok(elapsed_ut < 2 * USEC_PER_SEC, "APPS_LOOKUP stop waited for timeout instead of aborting the call");

cleanup_client:
    __atomic_store_n(&blocking_mock_release, true, __ATOMIC_RELEASE);
    if (client_initialized) {
        __atomic_store_n(&plugin_should_exit, true, __ATOMIC_RELEASE);
        nv_apps_lookup_stop();
    }
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

    return ok;
}

int main(void)
{
    if (nd_environment_init() != 0 || nd_environment_freeze_process() != 0) {
        fprintf(stderr, "failed to initialize and freeze the process environment: %s\n", strerror(errno));
        return 1;
    }

    char temp_dir[] = "./network-viewer-apps-lookup-test.XXXXXX";
    bool ok = false;

    netdata_configured_host_prefix = "";
    if (!mkdtemp(temp_dir)) {
        perror("mkdtemp");
        return 1;
    }
    test_run_dir = temp_dir;

    ok = run_roundtrip_test(temp_dir) && run_abort_during_call_test(temp_dir);

    (void)rmdir(temp_dir);
    return ok ? 0 : 1;
}
