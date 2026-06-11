// SPDX-License-Identifier: GPL-3.0-or-later

#include "cgroup-internals.h"
#include "cgroup-snapshot-store.h"
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

static int cgroup_snapshot_label_cb(
    const char *name, const char *value, RRDLABEL_SRC ls __maybe_unused, void *data) {
    cgroup_snapshot_entry_add_label((CGROUP_SNAPSHOT_ENTRY *)data, name, value);
    return 0;
}

// Build the immutable snapshot store from cgroup_root and publish it. Called by
// the discovery thread right after the splice: discovery is the only writer of
// the fields read here and is not splicing again until next cycle, so this runs
// lock-free against the collection loop (which only reads its own metric fields)
// and moves the per-cgroup cgroup.procs stat() off the snapshot handler's hot
// path and off cgroup_root_mutex.
void cgroup_snapshot_rebuild_and_publish(void) {
    uint64_t generation = __atomic_load_n(&cgroup_discovery_generation, __ATOMIC_ACQUIRE);
    CGROUP_SNAPSHOT_BUILDER *sb = cgroup_snapshot_builder_create(generation, (size_t)cgroup_root_count);

    char name_buf[256];
    char path_buf[FILENAME_MAX + 1];

    for (struct cgroup *cg = cgroup_root; cg; cg = cg->next) {
        CGROUP_SNAPSHOT_ENTRY *e = cgroup_snapshot_builder_add_entry(sb, cg->id);
        if (!e)
            continue; // duplicate id, first wins

        discovery_classify_orchestrator(cg);

        const char *prefix = is_cgroup_systemd_service(cg) ? services_chart_id_prefix : cgroup_chart_id_prefix;
        snprintfz(name_buf, sizeof(name_buf) - 1, "%s%s", prefix, cg->chart_id);

        uint32_t enabled = cg->enabled;
        if (cgroup_use_unified_cgroups) {
            struct stat st;
            snprintfz(path_buf, FILENAME_MAX, "%s%s/cgroup.procs", cgroup_unified_base, cg->id);
            if (stat(path_buf, &st) == -1) {
                path_buf[0] = '\0';
                enabled = 0;
            }
        } else if (!cgroup_find_procs_path_v1(path_buf, sizeof(path_buf), cg->id)) {
            enabled = 0;
        }

        e->known = (cg->processed && cg->pending_renames == 0);
        e->orchestrator = (uint16_t)cg->container_orchestrator;
        e->name = string_strdupz(cg->name ? cg->name : "");
        e->chart_name = string_strdupz(name_buf);
        e->hash = simple_hash(name_buf);
        e->options = cg->options;
        e->enabled = enabled;
        e->procs_path = string_strdupz(path_buf);

        if (cg->chart_labels)
            rrdlabels_walkthrough_read(cg->chart_labels, cgroup_snapshot_label_cb, e);
    }

    cgroup_snapshot_store_publish(cgroup_snapshot_builder_finalize(sb));
}

// handler callback invoked by netipc worker threads when a client requests a snapshot
static bool cgroups_snapshot_handler(void *user __maybe_unused,
                                     const nipc_cgroups_req_t *request __maybe_unused,
                                     nipc_cgroups_builder_t *builder) {
    static uint64_t last_logged_zero_generation = 0;
    static uint64_t last_logged_truncated_generation = 0;

    const CGROUP_SNAPSHOT_STORE *store = cgroup_snapshot_store_acquire();

    uint64_t snapshot_generation = cgroup_snapshot_store_generation(store);
    nipc_cgroups_builder_set_header(builder, CONFIG_BOOLEAN_YES, snapshot_generation);

    size_t total = cgroup_snapshot_store_count(store);
    int count = 0;
    int enabled_count = 0;
    bool truncated = false;
    for (size_t i = 0; i < total && count < cgroup_root_max; i++, count++) {
        const CGROUP_SNAPSHOT_ENTRY *e = cgroup_snapshot_store_at(store, i);

        if (e->enabled)
            enabled_count++;

        nipc_error_t err = nipc_cgroups_builder_add(
            builder, e->hash, e->options, e->enabled,
            string2str(e->chart_name), string_strlen(e->chart_name),
            string2str(e->procs_path), string_strlen(e->procs_path));

        if (err == NIPC_ERR_OVERFLOW) {
            truncated = true;
            break; // buffer full — send what we have
        }
    }

    cgroup_snapshot_store_release();

    bool log_zero_generation = false;
    bool log_truncated_generation = false;

    if (count == 0 && last_logged_zero_generation != snapshot_generation) {
        last_logged_zero_generation = snapshot_generation;
        log_zero_generation = true;
    }

    if (truncated && last_logged_truncated_generation != snapshot_generation) {
        last_logged_truncated_generation = snapshot_generation;
        log_truncated_generation = true;
    }

    if (log_zero_generation) {
        collector_info("CGROUP: netipc snapshot generation=%llu returned zero items",
                       (unsigned long long)snapshot_generation);
    }

    if (log_truncated_generation) {
        collector_error(
            "CGROUP: netipc snapshot generation=%llu truncated after %d items (%d enabled) due to response size limits",
            (unsigned long long)snapshot_generation,
            count,
            enabled_count);
    }

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
        collector_error("CGROUP: netipc server init failed (error %u), IPC sharing disabled", (unsigned int)err);
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

    collector_info("CGROUP: netipc server started on '%s/%s.sock'",
                   os_run_dir(true), CGROUP_NETIPC_SERVICE_NAME);
}

void cgroup_netipc_cleanup(void) {
    if (!cgroup_netipc_thread)
        return;

    nipc_server_stop(&cgroup_netipc_server);
    nd_thread_join(cgroup_netipc_thread);
    cgroup_netipc_thread = NULL;

    nipc_server_drain(&cgroup_netipc_server, 5000);
    nipc_server_destroy(&cgroup_netipc_server);

    collector_info("CGROUP: netipc server stopped");
}

#endif // OS_LINUX
