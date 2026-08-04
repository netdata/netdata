// SPDX-License-Identifier: GPL-3.0-or-later

#include "../cgroup-internals.h"
#include "../cgroup-netipc.h"
#include "../cgroup-lookup-resolver.h"
#include "../cgroup-snapshot-store.h"
#include "libnetdata/netipc/netipc_netdata.h"

SIMPLE_PATTERN *systemd_services_cgroups = NULL;
SIMPLE_PATTERN *search_cgroup_paths = NULL;
RRDHOST *localhost = NULL;
struct cgroup *cgroup_root = NULL;
netdata_mutex_t cgroup_root_mutex;
struct discovery_thread discovery_thread = { 0 };
int cgroup_max_depth = 0;
bool discovery_signal_pending = false;
uint64_t cgroup_discovery_generation = 0;
int cgroup_use_unified_cgroups = 0;
char *cgroup_cpuset_base = NULL;
char *cgroup_blkio_base = NULL;
char *cgroup_memory_base = NULL;
char *cgroup_cpuacct_base = NULL;
char *cgroup_unified_base = NULL;

int rrdlabels_walkthrough_read(
    RRDLABELS *labels,
    int (*callback)(const char *name, const char *value, RRDLABEL_SRC ls, void *data),
    void *data)
{
    if (!labels || !callback)
        return 0;

    return callback("image", "netdata/test", RRDLABEL_SRC_AUTO, data);
}

RRDSET *rrdset_create_custom(
    RRDHOST *host,
    const char *type,
    const char *id,
    const char *name,
    const char *family,
    const char *context,
    const char *title,
    const char *units,
    const char *plugin,
    const char *module,
    long priority,
    int update_every,
    RRDSET_TYPE chart_type,
    RRD_DB_MODE memory_mode,
    long history_entries)
{
    (void)host;
    (void)type;
    (void)id;
    (void)name;
    (void)family;
    (void)context;
    (void)title;
    (void)units;
    (void)plugin;
    (void)module;
    (void)priority;
    (void)update_every;
    (void)chart_type;
    (void)memory_mode;
    (void)history_entries;

    return NULL;
}

RRDDIM *rrddim_add_custom(
    RRDSET *st,
    const char *id,
    const char *name,
    collected_number multiplier,
    collected_number divisor,
    RRD_ALGORITHM algorithm,
    RRD_DB_MODE memory_mode)
{
    (void)st;
    (void)id;
    (void)name;
    (void)multiplier;
    (void)divisor;
    (void)algorithm;
    (void)memory_mode;

    return NULL;
}

collected_number rrddim_set_by_pointer(RRDSET *st, RRDDIM *rd, collected_number value)
{
    (void)st;
    (void)rd;
    (void)value;

    return 0;
}

void rrdset_done(RRDSET *st)
{
    (void)st;
}

static bool wait_for_client_ready(nipc_client_ctx_t *client)
{
    for (size_t i = 0; i < 100; i++) {
        (void)nipc_client_refresh(client);
        if (nipc_client_ready(client))
            return true;

        sleep_usec(10000);
    }

    return false;
}

static bool expect_item(
    const nipc_cgroups_lookup_resp_view_t *response,
    uint32_t index,
    const char *path,
    uint16_t status,
    uint16_t orchestrator,
    const char *name,
    uint16_t label_count)
{
    nipc_cgroups_lookup_item_view_t item;
    nipc_error_t err = nipc_cgroups_lookup_resp_item(response, index, &item);
    if (err != NIPC_OK) {
        fprintf(stderr, "item %u decode failed: %u\n", index, (unsigned int)err);
        return false;
    }

    if (item.path.len != strlen(path) || memcmp(item.path.ptr, path, item.path.len) != 0) {
        fprintf(stderr, "item %u path mismatch\n", index);
        return false;
    }

    if (item.status != status) {
        fprintf(stderr, "item %u status mismatch: expected %u got %u\n", index, status, item.status);
        return false;
    }

    if (item.orchestrator != orchestrator) {
        fprintf(stderr, "item %u orchestrator mismatch: expected %u got %u\n", index, orchestrator, item.orchestrator);
        return false;
    }

    if (item.name.len != strlen(name) || memcmp(item.name.ptr, name, item.name.len) != 0) {
        fprintf(stderr, "item %u name mismatch\n", index);
        return false;
    }

    if (item.label_count != label_count) {
        fprintf(stderr, "item %u label count mismatch: expected %u got %u\n", index, label_count, item.label_count);
        return false;
    }

    return true;
}

static bool expect_true(bool condition, const char *message)
{
    if (condition)
        return true;

    fprintf(stderr, "%s\n", message);
    return false;
}

// Publish a snapshot store holding the known (resolved) and retry (discovered
// but not yet resolved) cgroups, mirroring what discovery's rebuild would build.
static void publish_test_store_with_generation(
    uint64_t generation,
    const char *known_path,
    const char *retry_path,
    const char *known_name)
{
    size_t entries = (known_path ? 1 : 0) + (retry_path ? 1 : 0);
    CGROUP_SNAPSHOT_BUILDER *builder = cgroup_snapshot_builder_create(generation, entries);

    if (known_path) {
        CGROUP_SNAPSHOT_ENTRY *known = cgroup_snapshot_builder_add_entry(builder, known_path);
        struct stat known_st = { .st_dev = 1, .st_ino = (ino_t)(10 + generation) };
        cgroup_snapshot_entry_set_dir_identity(known, &known_st);
        known->known = true;
        known->orchestrator = (uint16_t)CGROUPS_ORCHESTRATOR_DOCKER;
        known->name = string_strdupz(known_name ? known_name : "known-container");
        cgroup_snapshot_entry_add_label(known, "image", "netdata/test");
    }

    // discovered but unresolved: known stays false -> RETRY_LATER without a name
    if (retry_path)
        (void)cgroup_snapshot_builder_add_entry(builder, retry_path);

    cgroup_snapshot_store_publish(cgroup_snapshot_builder_finalize(builder));
}

static void publish_test_store(const char *known_path, const char *retry_path)
{
    publish_test_store_with_generation(1, known_path, retry_path, "known-container");
}

static void publish_duplicate_suffix_store(uint64_t generation)
{
    CGROUP_SNAPSHOT_BUILDER *builder = cgroup_snapshot_builder_create(generation, 2);

    CGROUP_SNAPSHOT_ENTRY *first = cgroup_snapshot_builder_add_entry(builder, "/left.slice/pod-a/container.scope");
    first->known = true;
    first->orchestrator = (uint16_t)CGROUPS_ORCHESTRATOR_DOCKER;
    first->name = string_strdupz("left-container");

    CGROUP_SNAPSHOT_ENTRY *second = cgroup_snapshot_builder_add_entry(builder, "/right.slice/pod-a/container.scope");
    second->known = true;
    second->orchestrator = (uint16_t)CGROUPS_ORCHESTRATOR_DOCKER;
    second->name = string_strdupz("right-container");

    cgroup_snapshot_store_publish(cgroup_snapshot_builder_finalize(builder));
}

static bool test_snapshot_lookup_helpers(void)
{
    CGROUP_SNAPSHOT_BUILDER *builder = cgroup_snapshot_builder_create(2, 3);

    CGROUP_SNAPSHOT_ENTRY *first = cgroup_snapshot_builder_add_entry(builder, "/kubepods.slice/pod-a/container.scope");
    struct stat first_st = { .st_dev = 2, .st_ino = 20 };
    cgroup_snapshot_entry_set_dir_identity(first, &first_st);

    CGROUP_SNAPSHOT_ENTRY *second = cgroup_snapshot_builder_add_entry(builder, "/other.slice/pod-a/container.scope");
    struct stat second_st = { .st_dev = 2, .st_ino = 21 };
    cgroup_snapshot_entry_set_dir_identity(second, &second_st);

    cgroup_snapshot_store_publish(cgroup_snapshot_builder_finalize(builder));

    const CGROUP_SNAPSHOT_STORE *store = cgroup_snapshot_store_acquire();
    bool duplicate = false;
    bool ok =
        expect_true(
            cgroup_snapshot_store_find_unique_identity(store, first_st.st_dev, first_st.st_ino) == first,
            "unique identity helper did not find first entry") &&
        expect_true(
            cgroup_snapshot_store_find_unique_suffix(
                store, "kubepods.slice/pod-a/container.scope",
                strlen("kubepods.slice/pod-a/container.scope"), &duplicate) == first,
            "unique suffix helper did not find first entry") &&
        expect_true(!duplicate, "unique suffix helper reported a duplicate for unique suffix");

    duplicate = false;
    ok = expect_true(
             cgroup_snapshot_store_find_unique_suffix(
                 store, "pod-a/container.scope", strlen("pod-a/container.scope"), &duplicate) == NULL,
             "duplicate suffix helper should not return an entry") && ok;
    ok = expect_true(duplicate, "duplicate suffix helper did not report duplicate") && ok;

    cgroup_snapshot_store_release();
    return ok;
}

int main(void)
{
    if (nd_environment_init() != 0 || nd_environment_freeze_process() != 0) {
        fprintf(stderr, "failed to initialize and freeze the process environment: %s\n", strerror(errno));
        return 1;
    }

    char temp_dir[] = "./cgroup-lookup-netipc-test.XXXXXX";
    char service_name[64];
    const char *known_path = "/docker/0123456789abcdef";
    const char *retry_path = "/docker/aaaaaaaaaaaaaaaa";
    const char *missing_path = "/docker/cccccccccccccccc";
    const char *unreachable_path = "/system/some.scope"; // ancestor "/system" is excluded from the walk
    int rc = 1;

    netdata_configured_host_prefix = "";
    if (!mkdtemp(temp_dir)) {
        perror("mkdtemp");
        return 1;
    }

    snprintfz(service_name, sizeof(service_name) - 1, "cgroups-lookup-test-%d", getpid());

    // classifier input: mirror the real default so /docker/* is reachable (a
    // transient miss) while /system/* is not (a permanent miss).
    search_cgroup_paths = simple_pattern_create(" !/system !/systemd !/user * ", NULL, SIMPLE_PATTERN_EXACT, true);
    if (!search_cgroup_paths) {
        fprintf(stderr, "failed to create search_cgroup_paths pattern\n");
        goto cleanup_dir;
    }

    if (netdata_mutex_init(&cgroup_root_mutex) != 0) {
        fprintf(stderr, "failed to initialize cgroup_root_mutex\n");
        goto cleanup_pattern;
    }

    if (netdata_mutex_init(&discovery_thread.mutex) != 0) {
        fprintf(stderr, "failed to initialize discovery_thread.mutex\n");
        goto cleanup_root_mutex;
    }

    if (netdata_cond_init(&discovery_thread.cond_var) != 0) {
        fprintf(stderr, "failed to initialize discovery_thread.cond_var\n");
        goto cleanup_discovery_mutex;
    }

    cgroup_snapshot_store_init();

    if (!test_snapshot_lookup_helpers())
        goto cleanup_snapshot_store;

    publish_test_store(known_path, retry_path);

    cgroup_netipc_lookup_init_for_testing(temp_dir, service_name);

    nipc_client_config_t config = {
        .supported_profiles = NIPC_PROFILE_BASELINE | NIPC_PROFILE_SHM_HYBRID | NIPC_PROFILE_SHM_FUTEX,
        .preferred_profiles = NIPC_PROFILE_SHM_FUTEX,
        .auth_token = netipc_auth_token(),
    };

    nipc_client_ctx_t client;
    nipc_client_init(&client, temp_dir, service_name, &config);
    if (!wait_for_client_ready(&client)) {
        fprintf(stderr, "client did not reach READY state\n");
        goto cleanup_server;
    }

    const char *path_strings[] = {
        known_path,
        retry_path,
        missing_path,
        unreachable_path,
    };
    nipc_str_view_t paths[4];
    for (size_t i = 0; i < 4; i++) {
        paths[i].ptr = path_strings[i];
        paths[i].len = (uint32_t)strlen(path_strings[i]);
    }

    nipc_cgroups_lookup_resp_view_t response;
    nipc_error_t err = nipc_client_call_cgroups_lookup(&client, paths, 4, &response);
    if (err != NIPC_OK) {
        fprintf(stderr, "lookup call failed: %u\n", (unsigned int)err);
        goto cleanup_client;
    }

    if (response.generation != 1 || response.item_count != 4) {
        fprintf(stderr, "response header mismatch: generation=%llu items=%u\n",
                (unsigned long long)response.generation, response.item_count);
        goto cleanup_client;
    }

    if (!expect_item(&response, 0, known_path, NIPC_CGROUP_LOOKUP_KNOWN, NIPC_ORCHESTRATOR_DOCKER, "known-container", 1) ||
        !expect_item(&response, 1, retry_path, NIPC_CGROUP_LOOKUP_UNKNOWN_RETRY_LATER, NIPC_ORCHESTRATOR_UNKNOWN, "", 0) ||
        !expect_item(&response, 2, missing_path, NIPC_CGROUP_LOOKUP_UNKNOWN_RETRY_LATER, NIPC_ORCHESTRATOR_UNKNOWN, "", 0) ||
        !expect_item(&response, 3, unreachable_path, NIPC_CGROUP_LOOKUP_UNKNOWN_PERMANENT, NIPC_ORCHESTRATOR_UNKNOWN, "", 0))
        goto cleanup_client;

    if (!__atomic_load_n(&discovery_signal_pending, __ATOMIC_ACQUIRE)) {
        fprintf(stderr, "reachable lookup miss did not arm discovery signal flag\n");
        goto cleanup_client;
    }

    __atomic_store_n(&discovery_signal_pending, false, __ATOMIC_RELEASE);

    // none of these must wake discovery: retry is known-but-unresolved and
    // unreachable is walk-excluded (permanent)
    nipc_str_view_t no_signal_paths[2] = {
        paths[1],
        paths[3],
    };
    err = nipc_client_call_cgroups_lookup(&client, no_signal_paths, 2, &response);
    if (err != NIPC_OK) {
        fprintf(stderr, "retry/unreachable lookup call failed: %u\n", (unsigned int)err);
        goto cleanup_client;
    }

    if (__atomic_load_n(&discovery_signal_pending, __ATOMIC_ACQUIRE)) {
        fprintf(stderr, "unresolved/unreachable lookup incorrectly armed discovery signal flag\n");
        goto cleanup_client;
    }

    const char *namespace_relative_known =
        "/../../docker/0123456789abcdef";
    nipc_str_view_t namespace_relative_path = {
        .ptr = namespace_relative_known,
        .len = (uint32_t)strlen(namespace_relative_known),
    };
    err = nipc_client_call_cgroups_lookup(&client, &namespace_relative_path, 1, &response);
    if (err != NIPC_OK) {
        fprintf(stderr, "namespace-relative lookup call failed: %u\n", (unsigned int)err);
        goto cleanup_client;
    }

    if (!expect_item(
            &response, 0, namespace_relative_known,
            NIPC_CGROUP_LOOKUP_KNOWN, NIPC_ORCHESTRATOR_DOCKER, "known-container", 1))
        goto cleanup_client;

    if (cgroup_lookup_resolver_suffix_scans_for_testing() == 0) {
        fprintf(stderr, "namespace-relative lookup did not use suffix resolver path\n");
        goto cleanup_client;
    }

    size_t scans_after_first_resolution = cgroup_lookup_resolver_suffix_scans_for_testing();
    const char *namespace_relative_retry_same_prefix =
        "/../../docker/aaaaaaaaaaaaaaaa";
    nipc_str_view_t namespace_relative_retry_same_prefix_path = {
        .ptr = namespace_relative_retry_same_prefix,
        .len = (uint32_t)strlen(namespace_relative_retry_same_prefix),
    };
    err = nipc_client_call_cgroups_lookup(&client, &namespace_relative_retry_same_prefix_path, 1, &response);
    if (err != NIPC_OK) {
        fprintf(stderr, "namespace-relative positive-cache lookup failed: %u\n", (unsigned int)err);
        goto cleanup_client;
    }

    if (!expect_item(
            &response, 0, namespace_relative_retry_same_prefix,
            NIPC_CGROUP_LOOKUP_UNKNOWN_RETRY_LATER, NIPC_ORCHESTRATOR_UNKNOWN, "", 0))
        goto cleanup_client;

    if (cgroup_lookup_resolver_suffix_scans_for_testing() != scans_after_first_resolution) {
        fprintf(stderr, "namespace-relative positive cache did not avoid a second suffix scan\n");
        goto cleanup_client;
    }

    __atomic_store_n(&discovery_signal_pending, false, __ATOMIC_RELEASE);
    const char *namespace_relative_missing =
        "/../../missing/not-present.scope";
    nipc_str_view_t namespace_relative_missing_path = {
        .ptr = namespace_relative_missing,
        .len = (uint32_t)strlen(namespace_relative_missing),
    };
    size_t scans_before_negative_miss = cgroup_lookup_resolver_suffix_scans_for_testing();
    err = nipc_client_call_cgroups_lookup(&client, &namespace_relative_missing_path, 1, &response);
    if (err != NIPC_OK) {
        fprintf(stderr, "namespace-relative missing lookup call failed: %u\n", (unsigned int)err);
        goto cleanup_client;
    }

    if (!expect_item(
            &response, 0, namespace_relative_missing,
            NIPC_CGROUP_LOOKUP_UNKNOWN_RETRY_LATER, NIPC_ORCHESTRATOR_UNKNOWN, "", 0))
        goto cleanup_client;

    size_t scans_after_negative_miss = cgroup_lookup_resolver_suffix_scans_for_testing();
    if (scans_after_negative_miss != scans_before_negative_miss + 1) {
        fprintf(stderr, "namespace-relative negative lookup did not perform exactly one suffix scan\n");
        goto cleanup_client;
    }

    err = nipc_client_call_cgroups_lookup(&client, &namespace_relative_missing_path, 1, &response);
    if (err != NIPC_OK) {
        fprintf(stderr, "namespace-relative negative-cache lookup call failed: %u\n", (unsigned int)err);
        goto cleanup_client;
    }

    if (!expect_item(
            &response, 0, namespace_relative_missing,
            NIPC_CGROUP_LOOKUP_UNKNOWN_RETRY_LATER, NIPC_ORCHESTRATOR_UNKNOWN, "", 0))
        goto cleanup_client;

    if (cgroup_lookup_resolver_suffix_scans_for_testing() != scans_after_negative_miss) {
        fprintf(stderr, "namespace-relative negative cache did not avoid a second suffix scan\n");
        goto cleanup_client;
    }

    if (__atomic_load_n(&discovery_signal_pending, __ATOMIC_ACQUIRE)) {
        fprintf(stderr, "namespace-relative miss incorrectly armed discovery signal flag\n");
        goto cleanup_client;
    }

    publish_test_store_with_generation(2, "/missing/not-present.scope", retry_path, "late-container");
    err = nipc_client_call_cgroups_lookup(&client, &namespace_relative_missing_path, 1, &response);
    if (err != NIPC_OK) {
        fprintf(stderr, "namespace-relative generation-invalidation lookup failed: %u\n", (unsigned int)err);
        goto cleanup_client;
    }

    if (!expect_item(
            &response, 0, namespace_relative_missing,
            NIPC_CGROUP_LOOKUP_KNOWN, NIPC_ORCHESTRATOR_DOCKER, "late-container", 1))
        goto cleanup_client;

    publish_duplicate_suffix_store(3);
    const char *namespace_relative_duplicate =
        "/../../pod-a/container.scope";
    nipc_str_view_t namespace_relative_duplicate_path = {
        .ptr = namespace_relative_duplicate,
        .len = (uint32_t)strlen(namespace_relative_duplicate),
    };
    err = nipc_client_call_cgroups_lookup(&client, &namespace_relative_duplicate_path, 1, &response);
    if (err != NIPC_OK) {
        fprintf(stderr, "namespace-relative duplicate-suffix lookup failed: %u\n", (unsigned int)err);
        goto cleanup_client;
    }

    if (!expect_item(
            &response, 0, namespace_relative_duplicate,
            NIPC_CGROUP_LOOKUP_UNKNOWN_RETRY_LATER, NIPC_ORCHESTRATOR_UNKNOWN, "", 0))
        goto cleanup_client;

    publish_test_store(known_path, retry_path);
    err = nipc_client_call_cgroups_lookup(&client, paths, 4, &response);
    if (err != NIPC_OK) {
        fprintf(stderr, "second lookup call failed: %u\n", (unsigned int)err);
        goto cleanup_client;
    }

    if (!__atomic_load_n(&discovery_signal_pending, __ATOMIC_ACQUIRE)) {
        fprintf(stderr, "second reachable lookup miss did not re-arm discovery signal flag\n");
        goto cleanup_client;
    }

    rc = 0;

cleanup_client:
    nipc_client_close(&client);
cleanup_server:
    cgroup_netipc_lookup_cleanup();
cleanup_snapshot_store:
    cgroup_snapshot_store_shutdown();
    netdata_cond_destroy(&discovery_thread.cond_var);
cleanup_discovery_mutex:
    netdata_mutex_destroy(&discovery_thread.mutex);
cleanup_root_mutex:
    netdata_mutex_destroy(&cgroup_root_mutex);
cleanup_pattern:
    simple_pattern_free(search_cgroup_paths);
cleanup_dir:
    (void)rmdir(temp_dir);
    return rc;
}
