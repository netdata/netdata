// SPDX-License-Identifier: GPL-3.0-or-later

#include "cgroup-internals.h"
#include "libnetdata/netipc/netipc_netdata.h"

#ifdef OS_LINUX

#define CGROUP_NETIPC_SERVICE_NAME "cgroups-snapshot"
#define CGROUP_NETIPC_WORKER_COUNT 2

static nipc_managed_server_t cgroup_netipc_server;
static ND_THREAD *cgroup_netipc_thread = NULL;

// find the cgroup.procs path for a given cgroup (v1 hierarchy)
// writes into path_buf, returns true if found
static bool cgroup_find_procs_path_v1(char *path_buf, size_t path_buf_size, const char *cg_id) {
    struct stat buf;

    snprintfz(path_buf, path_buf_size - 1, "%s%s/cgroup.procs", cgroup_cpuset_base, cg_id);
    if (stat(path_buf, &buf) == 0)
        return true;

    snprintfz(path_buf, path_buf_size - 1, "%s%s/cgroup.procs", cgroup_blkio_base, cg_id);
    if (stat(path_buf, &buf) == 0)
        return true;

    snprintfz(path_buf, path_buf_size - 1, "%s%s/cgroup.procs", cgroup_memory_base, cg_id);
    if (stat(path_buf, &buf) == 0)
        return true;

    path_buf[0] = '\0';
    return false;
}

// handler callback invoked by netipc worker threads when a client requests a snapshot
static bool cgroups_snapshot_handler(void *user __maybe_unused,
                                     const nipc_cgroups_req_t *request __maybe_unused,
                                     nipc_cgroups_builder_t *builder) {
    static uint64_t generation = 0;
    char name_buf[256];
    char path_buf[FILENAME_MAX + 1];

    netdata_mutex_lock(&cgroup_root_mutex);

    // set snapshot header — systemd is always enabled in this codebase
    nipc_cgroups_builder_set_header(builder, CONFIG_BOOLEAN_YES, ++generation);

    struct cgroup *cg;
    int count;
    for (cg = cgroup_root, count = 0; cg && count < cgroup_root_max; cg = cg->next, count++) {
        const char *prefix = is_cgroup_systemd_service(cg)
            ? services_chart_id_prefix
            : cgroup_chart_id_prefix;

        snprintfz(name_buf, sizeof(name_buf) - 1, "%s%s", prefix, cg->chart_id);
        uint32_t hash = simple_hash(name_buf);
        uint32_t options = cg->options;
        uint32_t enabled = cg->enabled;

        // find the cgroup.procs path
        if (cgroup_use_unified_cgroups) {
            struct stat buf;
            snprintfz(path_buf, FILENAME_MAX, "%s%s/cgroup.procs", cgroup_unified_base, cg->id);
            if (stat(path_buf, &buf) == -1) {
                path_buf[0] = '\0';
                enabled = 0;
            }
        } else {
            if (!cgroup_find_procs_path_v1(path_buf, sizeof(path_buf), cg->id))
                enabled = 0;
        }

        nipc_error_t err = nipc_cgroups_builder_add(
            builder, hash, options, enabled,
            name_buf, (uint32_t)strlen(name_buf),
            path_buf, (uint32_t)strlen(path_buf));

        if (err == NIPC_ERR_OVERFLOW)
            break; // buffer full — send what we have
    }

    netdata_mutex_unlock(&cgroup_root_mutex);
    return true;
}

// thread entry point for the netipc accept loop
static void cgroup_netipc_server_thread(void *arg) {
    nipc_server_run((nipc_managed_server_t *)arg);
}

void cgroup_netipc_init(void) {
    uint64_t auth = netipc_auth_token();

    nipc_server_config_t config = {
        .supported_profiles = NIPC_PROFILE_BASELINE | NIPC_PROFILE_SHM_HYBRID | NIPC_PROFILE_SHM_FUTEX,
        .preferred_profiles = NIPC_PROFILE_SHM_FUTEX,
        .auth_token = auth,
    };

    nipc_cgroups_service_handler_t handler = {
        .handle = cgroups_snapshot_handler,
        .snapshot_max_items = 0, // auto-estimate from negotiated limits
        .user = NULL,
    };

    nipc_error_t err = nipc_server_init_typed(
        &cgroup_netipc_server,
        os_run_dir(true),
        CGROUP_NETIPC_SERVICE_NAME,
        &config,
        CGROUP_NETIPC_WORKER_COUNT,
        &handler);

    if (err != NIPC_OK) {
        collector_error("CGROUP: netipc server init failed (error %d), IPC sharing disabled", err);
        return;
    }

    cgroup_netipc_thread = nd_thread_create(
        "P[cgroupsipc]", NETDATA_THREAD_OPTION_DONT_LOG_STARTUP,
        cgroup_netipc_server_thread, &cgroup_netipc_server);

    if (!cgroup_netipc_thread) {
        collector_error("CGROUP: failed to create netipc server thread");
        nipc_server_destroy(&cgroup_netipc_server);
        return;
    }

    collector_info("CGROUP: netipc server started on '%s/%s'",
                   os_run_dir(true), CGROUP_NETIPC_SERVICE_NAME);
}

void cgroup_netipc_cleanup(void) {
    if (!cgroup_netipc_thread)
        return;

    nipc_server_stop(&cgroup_netipc_server);
    nipc_server_drain(&cgroup_netipc_server, 5000);
    nipc_server_destroy(&cgroup_netipc_server);
    cgroup_netipc_thread = NULL;

    collector_info("CGROUP: netipc server stopped");
}

#endif // OS_LINUX
