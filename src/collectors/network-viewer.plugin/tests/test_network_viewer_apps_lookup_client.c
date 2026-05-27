// SPDX-License-Identifier: GPL-3.0-or-later

#include "libnetdata/netipc/netipc_netdata.h"

static const char *test_run_dir = NULL;

static const char *test_os_run_dir(bool rw __maybe_unused)
{
    return test_run_dir;
}

#define os_run_dir(rw) test_os_run_dir(rw)
#include "../network-viewer-apps-lookup-client.c"
#undef os_run_dir

static _Atomic uint64_t mock_requests = 0;

static bool expect_ok(bool condition, const char *message)
{
    if (condition)
        return true;

    fprintf(stderr, "%s\n", message);
    return false;
}

static bool wait_for_counter(_Atomic uint64_t *counter, uint64_t value)
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

static uint64_t counter_value(_Atomic uint64_t *counter)
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

int main(void)
{
    char temp_dir[] = "/tmp/network-viewer-apps-lookup-test.XXXXXX";
    int rc = 1;
    bool plugin_should_exit = false;
    nipc_managed_server_t server;
    ND_THREAD *server_thread = NULL;

    netdata_configured_host_prefix = "";
    if (!mkdtemp(temp_dir)) {
        perror("mkdtemp");
        return 1;
    }
    test_run_dir = temp_dir;

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
        temp_dir,
        NV_APPS_LOOKUP_SERVICE_NAME,
        &config,
        1,
        &handler);
    if (!expect_ok(err == NIPC_OK, "mock APPS_LOOKUP server init failed"))
        goto cleanup_dir;

    server_thread = nd_thread_create("appslookup-t", 0, mock_server_thread, &server);
    if (!expect_ok(server_thread != NULL, "mock APPS_LOOKUP server thread creation failed"))
        goto cleanup_server;

    nv_apps_lookup_init(&plugin_should_exit);
    nv_apps_lookup_start();

    uint32_t pids[] = { 100, 200 };
    nv_apps_lookup_warm_pids(pids, _countof(pids));

    if (!expect_ok(wait_for_counter(&mock_requests, 1), "worker did not call APPS_LOOKUP mock"))
        goto cleanup_client;
    if (!expect_ok(wait_for_cache_entries(1), "known PID was not cached"))
        goto cleanup_client;
    NV_APPS_LOOKUP_FIELDS cached;
    if (!expect_ok(nv_cache_lookup_pid(100, &cached), "known PID cache accessor missed"))
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
    if (!expect_ok(!nv_cache_lookup_pid(200, &cached), "retry-later PID should not be cached"))
        goto cleanup_client;
    if (!expect_ok(wait_for_counter(&apps_lookup_cache_misses_unknown, 1), "retry-later PID was not counted as unknown miss"))
        goto cleanup_client;

    nv_apps_lookup_warm_pids(pids, _countof(pids));

    if (!expect_ok(wait_for_counter(&apps_lookup_cache_hits, 1), "cached known PID was not served as a hit"))
        goto cleanup_client;
    if (!expect_ok(wait_for_counter(&apps_lookup_cache_misses_unknown, 2), "retry-later PID was cached or not retried"))
        goto cleanup_client;
    if (!expect_ok(wait_for_counter(&apps_lookup_requests_responded, 2), "worker did not complete the retry-later refresh"))
        goto cleanup_client;
    if (!expect_ok(__atomic_load_n(&apps_lookup_requests_failed, __ATOMIC_ACQUIRE) == 0, "client recorded a failed request"))
        goto cleanup_client;
    uint64_t worker_le_50ms =
        counter_value(&apps_lookup_worker_duration_le_1ms) +
        counter_value(&apps_lookup_worker_duration_le_5ms) +
        counter_value(&apps_lookup_worker_duration_le_10ms) +
        counter_value(&apps_lookup_worker_duration_le_50ms);
    if (!expect_ok(worker_le_50ms >= 2, "mock worker requests exceeded the local 50ms bucket"))
        goto cleanup_client;

    uint32_t known_pid[] = { 100 };
    for (size_t i = 0; i < 10; i++)
        nv_apps_lookup_warm_pids(known_pid, _countof(known_pid));

    if (!expect_ok(wait_for_counter(&apps_lookup_cache_hits, 11), "known PID cache hit ratio check failed"))
        goto cleanup_client;
    uint64_t handler_le_5ms =
        counter_value(&apps_lookup_handler_overhead_le_1ms) +
        counter_value(&apps_lookup_handler_overhead_le_5ms);
    if (!expect_ok(handler_le_5ms >= 12, "Function handler overhead exceeded the local 5ms bucket"))
        goto cleanup_client;

    rc = 0;

cleanup_client:
    __atomic_store_n(&plugin_should_exit, true, __ATOMIC_RELEASE);
    nv_apps_lookup_stop();
cleanup_server:
    nipc_server_stop(&server);
    if (server_thread)
        nd_thread_join(server_thread);
    nipc_server_drain(&server, 5000);
    nipc_server_destroy(&server);
cleanup_dir:
    (void)rmdir(temp_dir);
    return rc;
}
