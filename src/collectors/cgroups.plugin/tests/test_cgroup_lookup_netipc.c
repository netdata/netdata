// SPDX-License-Identifier: GPL-3.0-or-later

#include "../cgroup-internals.h"
#include "../cgroup-netipc.h"
#include "libnetdata/netipc/netipc_netdata.h"

SIMPLE_PATTERN *systemd_services_cgroups = NULL;
RRDHOST *localhost = NULL;
struct cgroup *cgroup_root = NULL;
netdata_mutex_t cgroup_root_mutex;
int cgroup_lookup_reaped_set_size = 4;
_Atomic uint64_t cgroup_discovery_generation = 0;

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

int main(void)
{
    char temp_dir[] = "/tmp/cgroup-lookup-netipc-test.XXXXXX";
    char service_name[64];
    const char *known_path = "/docker/0123456789abcdef";
    const char *retry_path = "/docker/aaaaaaaaaaaaaaaa";
    const char *gone_path = "/docker/bbbbbbbbbbbbbbbb";
    const char *missing_path = "/docker/cccccccccccccccc";
    int rc = 1;

    netdata_configured_host_prefix = "";
    if (!mkdtemp(temp_dir)) {
        perror("mkdtemp");
        return 1;
    }

    snprintfz(service_name, sizeof(service_name) - 1, "cgroups-lookup-test-%d", getpid());

    if (netdata_mutex_init(&cgroup_root_mutex) != 0) {
        fprintf(stderr, "failed to initialize cgroup_root_mutex\n");
        goto cleanup_dir;
    }

    struct cgroup retry_cgroup = {
        .id = (char *)retry_path,
        .name = (char *)"not-ready",
        .processed = 0,
        .pending_renames = 0,
    };
    struct cgroup known_cgroup = {
        .id = (char *)known_path,
        .name = (char *)"known-container",
        .processed = 1,
        .pending_renames = 0,
        .next = &retry_cgroup,
    };
    known_cgroup.chart_labels = (RRDLABELS *)&known_cgroup;

    cgroup_lookup_set_cgroup_root_for_testing(&known_cgroup);
    __atomic_store_n(&cgroup_discovery_generation, 1, __ATOMIC_RELEASE);

    cgroup_netipc_lookup_init_for_testing(temp_dir, service_name);
    cgroup_netipc_lookup_reaped_set_insert(gone_path);

    nipc_client_config_t config = {
        .supported_profiles = NIPC_PROFILE_BASELINE,
        .preferred_profiles = NIPC_PROFILE_BASELINE,
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
        gone_path,
        missing_path,
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
        !expect_item(&response, 2, gone_path, NIPC_CGROUP_LOOKUP_UNKNOWN_PERMANENT, NIPC_ORCHESTRATOR_UNKNOWN, "", 0) ||
        !expect_item(&response, 3, missing_path, NIPC_CGROUP_LOOKUP_UNKNOWN_RETRY_LATER, NIPC_ORCHESTRATOR_UNKNOWN, "", 0))
        goto cleanup_client;

    rc = 0;

cleanup_client:
    nipc_client_close(&client);
cleanup_server:
    cgroup_lookup_set_cgroup_root_for_testing(NULL);
    cgroup_netipc_lookup_cleanup();
    netdata_mutex_destroy(&cgroup_root_mutex);
cleanup_dir:
    (void)rmdir(temp_dir);
    return rc;
}
