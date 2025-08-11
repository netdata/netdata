// SPDX-License-Identifier: GPL-3.0-or-later

#include "cgroup-internals.h"

// discovery cgroup thread worker jobs
#define WORKER_DISCOVERY_INIT               0
#define WORKER_DISCOVERY_FIND               1
#define WORKER_DISCOVERY_PROCESS            2
#define WORKER_DISCOVERY_PROCESS_RENAME     3
#define WORKER_DISCOVERY_PROCESS_NETWORK    4
#define WORKER_DISCOVERY_PROCESS_FIRST_TIME 5
#define WORKER_DISCOVERY_UPDATE             6
#define WORKER_DISCOVERY_CLEANUP            7
#define WORKER_DISCOVERY_COPY               8
#define WORKER_DISCOVERY_SHARE              9
#define WORKER_DISCOVERY_LOCK              10

#if WORKER_UTILIZATION_MAX_JOB_TYPES < 11
#error WORKER_UTILIZATION_MAX_JOB_TYPES has to be at least 11
#endif

struct cgroup *discovered_cgroup_root = NULL;

char cgroup_chart_id_prefix[] = "cgroup_";
char services_chart_id_prefix[] = "systemd_";
const char *cgroups_rename_script = NULL;

// Shared memory with information from detected cgroups
netdata_ebpf_cgroup_shm_t shm_cgroup_ebpf = {NULL, NULL};
int shm_fd_cgroup_ebpf = -1;
sem_t *shm_mutex_cgroup_ebpf = SEM_FAILED;

// ----------------------------------------------------------------------------

static inline void free_pressure(struct pressure *res) {
    if (res->some.share_time.st) rrdset_is_obsolete___safe_from_collector_thread(res->some.share_time.st);
    if (res->some.total_time.st) rrdset_is_obsolete___safe_from_collector_thread(res->some.total_time.st);
    if (res->full.share_time.st) rrdset_is_obsolete___safe_from_collector_thread(res->full.share_time.st);
    if (res->full.total_time.st) rrdset_is_obsolete___safe_from_collector_thread(res->full.total_time.st);
    freez(res->filename);
}

static inline void cgroup_free_network_interfaces(struct cgroup *cg) {
    while(cg->interfaces) {
        struct cgroup_network_interface *i = cg->interfaces;
        cg->interfaces = i->next;

        // delete the registration of proc_net_dev rename
        cgroup_rename_task_device_del(i->host_device);

        freez((void *)i->host_device);
        freez((void *)i->container_device);
        freez((void *)i);
    }
}

static inline void cgroup_free(struct cgroup *cg) {
    netdata_log_debug(D_CGROUP, "Removing cgroup '%s' with chart id '%s' (was %s and %s)", cg->id, cg->chart_id, (cg->enabled)?"enabled":"disabled", (cg->available)?"available":"not available");

    if(cg->st_cpu && cg->chart_var_cpu_limit) {
        rrdvar_chart_variable_release(cg->st_cpu, cg->chart_var_cpu_limit);
        cg->chart_var_cpu_limit = NULL;
    }

    if(cg->st_mem_usage && cg->chart_var_memory_limit) {
        rrdvar_chart_variable_release(cg->st_mem_usage, cg->chart_var_memory_limit);
        cg->chart_var_memory_limit = NULL;
    }

    cgroup_netdev_delete(cg);

    if(cg->st_cpu) rrdset_is_obsolete___safe_from_collector_thread(cg->st_cpu);
    if(cg->st_cpu_limit) rrdset_is_obsolete___safe_from_collector_thread(cg->st_cpu_limit);
    if(cg->st_cpu_per_core) rrdset_is_obsolete___safe_from_collector_thread(cg->st_cpu_per_core);
    if(cg->st_cpu_nr_throttled) rrdset_is_obsolete___safe_from_collector_thread(cg->st_cpu_nr_throttled);
    if(cg->st_cpu_throttled_time) rrdset_is_obsolete___safe_from_collector_thread(cg->st_cpu_throttled_time);
    if(cg->st_cpu_shares) rrdset_is_obsolete___safe_from_collector_thread(cg->st_cpu_shares);
    if(cg->st_mem) rrdset_is_obsolete___safe_from_collector_thread(cg->st_mem);
    if(cg->st_writeback) rrdset_is_obsolete___safe_from_collector_thread(cg->st_writeback);
    if(cg->st_mem_activity) rrdset_is_obsolete___safe_from_collector_thread(cg->st_mem_activity);
    if(cg->st_pgfaults) rrdset_is_obsolete___safe_from_collector_thread(cg->st_pgfaults);
    if(cg->st_mem_usage) rrdset_is_obsolete___safe_from_collector_thread(cg->st_mem_usage);
    if(cg->st_mem_usage_limit) rrdset_is_obsolete___safe_from_collector_thread(cg->st_mem_usage_limit);
    if(cg->st_mem_utilization) rrdset_is_obsolete___safe_from_collector_thread(cg->st_mem_utilization);
    if(cg->st_mem_failcnt) rrdset_is_obsolete___safe_from_collector_thread(cg->st_mem_failcnt);
    if(cg->st_io) rrdset_is_obsolete___safe_from_collector_thread(cg->st_io);
    if(cg->st_serviced_ops) rrdset_is_obsolete___safe_from_collector_thread(cg->st_serviced_ops);
    if(cg->st_throttle_io) rrdset_is_obsolete___safe_from_collector_thread(cg->st_throttle_io);
    if(cg->st_throttle_serviced_ops) rrdset_is_obsolete___safe_from_collector_thread(cg->st_throttle_serviced_ops);
    if(cg->st_queued_ops) rrdset_is_obsolete___safe_from_collector_thread(cg->st_queued_ops);
    if(cg->st_merged_ops) rrdset_is_obsolete___safe_from_collector_thread(cg->st_merged_ops);
    if(cg->st_pids) rrdset_is_obsolete___safe_from_collector_thread(cg->st_pids);

    freez(cg->filename_cpuset_cpus);
    freez(cg->filename_cpu_cfs_period);
    freez(cg->filename_cpu_cfs_quota);
    freez(cg->filename_memory_limit);

    cgroup_free_network_interfaces(cg);

    freez(cg->cpuacct_usage.cpu_percpu);

    freez(cg->cpuacct_stat.filename);
    freez(cg->cpuacct_usage.filename);
    freez(cg->cpuacct_cpu_throttling.filename);
    freez(cg->cpuacct_cpu_shares.filename);

    arl_free(cg->memory.arl_base);
    freez(cg->memory.filename_detailed);
    freez(cg->memory.filename_failcnt);
    freez(cg->memory.filename_usage_in_bytes);
    freez(cg->memory.filename_msw_usage_in_bytes);

    freez(cg->io_service_bytes.filename);
    freez(cg->io_serviced.filename);

    freez(cg->throttle_io_service_bytes.filename);
    freez(cg->throttle_io_serviced.filename);

    freez(cg->io_merged.filename);
    freez(cg->io_queued.filename);
    freez(cg->pids_current.filename);

    free_pressure(&cg->cpu_pressure);
    free_pressure(&cg->io_pressure);
    free_pressure(&cg->memory_pressure);
    free_pressure(&cg->irq_pressure);

    freez(cg->id);
    freez(cg->intermediate_id);
    freez(cg->chart_id);
    freez(cg->name);

    rrdlabels_destroy(cg->chart_labels);

    memset(cg, 0, sizeof(*cg));
    freez(cg);

    cgroup_root_count--;
}

// ----------------------------------------------------------------------------
// add/remove/find cgroup objects

#define CGROUP_CHARTID_LINE_MAX 1024

static inline char *cgroup_chart_id_strdupz(const char *s) {
    if(!s || !*s) s = "/";

    if(*s == '/' && s[1] != '\0') s++;

    char *r = strdupz(s);
    netdata_fix_chart_id(r);

    return r;
}

// TODO: move the code to cgroup_chart_id_strdupz() when the renaming script is fixed
static inline void substitute_dots_in_id(char *s) {
    // dots are used to distinguish chart type and id in streaming, so we should replace them
    for (char *d = s; *d; d++) {
        if (*d == '.')
            *d = '-';
    }
}

// ----------------------------------------------------------------------------
// parse k8s labels

#define CGROUP_NETDATA_CLOUD_LABEL_PREFIX "netdata.cloud/"
#define CGROUP_RENAME_LABEL "cgroup.name="
#define CGROUP_IGNORE_LABEL "ignore="

static char *cgroup_parse_resolved_name_and_labels(struct cgroup *cg, char *data) {
    if (!cg->chart_labels)
        cg->chart_labels = rrdlabels_create();

    rrdlabels_unmark_all(cg->chart_labels);

    // the first word, up to the first space is the name
    char *name = strsep_skip_consecutive_separators(&data, " ");

    bool ignored = false;

    // the rest are key=value pairs separated by comma
    while(data) {
        char *pair = strsep_skip_consecutive_separators(&data, ",");

        if(strncmp(pair, CGROUP_NETDATA_CLOUD_LABEL_PREFIX, sizeof(CGROUP_NETDATA_CLOUD_LABEL_PREFIX) - 1) == 0) {
            // a netdata.cloud label
            char *key = &pair[sizeof(CGROUP_NETDATA_CLOUD_LABEL_PREFIX) - 1];

            if(strncmp(key, CGROUP_RENAME_LABEL, sizeof(CGROUP_RENAME_LABEL) - 1) == 0) {
                char *n = &key[sizeof(CGROUP_RENAME_LABEL) - 1];
                size_t len = strlen(n);
                if(n[0] == '"' && n[len - 1] == '"') {
                    n[len - 1] = '\0';
                    n++;
                }
                if(*n) name = n;

                // no need to add this label
            }
            else if(strncmp(key, CGROUP_IGNORE_LABEL, sizeof(CGROUP_IGNORE_LABEL) - 1) == 0) {
                char *v = &key[sizeof(CGROUP_IGNORE_LABEL) - 1];
                if(strcasecmp(v, "\"true\"") == 0 || strcasecmp(v, "\"yes\"") == 0)
                    ignored = true;
                else
                    ignored = false;

                // no need to add this label
            }
        }
        else
            // add the label as-is
            rrdlabels_add_pair(cg->chart_labels, pair, RRDLABEL_SRC_AUTO | RRDLABEL_SRC_K8S);
    }

    rrdlabels_remove_all_unmarked(cg->chart_labels);

    if(ignored)
        cg->options |= CGROUP_OPTIONS_DISABLED_EXCLUDED;
    else
        cg->options &= ~CGROUP_OPTIONS_DISABLED_EXCLUDED;

    return name;
}

static inline void discovery_rename_cgroup(struct cgroup *cg) {
    if (!cg->pending_renames) {
        return;
    }
    cg->pending_renames--;

    netdata_log_debug(D_CGROUP, "looking for the name of cgroup '%s' with chart id '%s'", cg->id, cg->chart_id);
    netdata_log_debug(D_CGROUP, "executing command %s \"%s\" for cgroup '%s'", cgroups_rename_script, cg->intermediate_id, cg->chart_id);

    POPEN_INSTANCE *instance = spawn_popen_run_variadic(cgroups_rename_script, cg->id, cg->intermediate_id, NULL);
    if (!instance) {
        collector_error("CGROUP: cannot popen(%s \"%s\", \"r\").", cgroups_rename_script, cg->intermediate_id);
        cg->pending_renames = 0;
        cg->processed = 1;
        return;
    }

    char buffer[8192]; // we need some size for labels
    char *new_name = fgets(buffer, sizeof(buffer), spawn_popen_stdout(instance));
    int exit_code = spawn_popen_wait(instance);

    switch (exit_code) {
        case 0:
            cg->pending_renames = 0;
            break;

        case 3:
            cg->pending_renames = 0;
            cg->processed = 1;
            break;

        default:
            break;
    }

    if (cg->pending_renames || cg->processed)
        return;
    if (!new_name || !*new_name || *new_name == '\n')
        return;
    if (!(new_name = trim(new_name)))
        return;

    char *name = cgroup_parse_resolved_name_and_labels(cg, new_name);

    freez(cg->name);
    cg->name = strdupz(name);

    freez(cg->chart_id);
    cg->chart_id = cgroup_chart_id_strdupz(name);

    substitute_dots_in_id(cg->chart_id);
    cg->hash_chart_id = simple_hash(cg->chart_id);
}

static void is_cgroup_procs_exist(netdata_ebpf_cgroup_shm_body_t *out, char *id) {
    struct stat buf;

    snprintfz(out->path, FILENAME_MAX, "%s%s/cgroup.procs", cgroup_cpuset_base, id);
    if (likely(stat(out->path, &buf) == 0)) {
        return;
    }

    snprintfz(out->path, FILENAME_MAX, "%s%s/cgroup.procs", cgroup_blkio_base, id);
    if (likely(stat(out->path, &buf) == 0)) {
        return;
    }

    snprintfz(out->path, FILENAME_MAX, "%s%s/cgroup.procs", cgroup_memory_base, id);
    if (likely(stat(out->path, &buf) == 0)) {
        return;
    }

    out->path[0] = '\0';
    out->enabled = 0;
}

static inline void convert_cgroup_to_systemd_service(struct cgroup *cg) {
    char buffer[CGROUP_CHARTID_LINE_MAX + 1];
    cg->options |= CGROUP_OPTIONS_SYSTEM_SLICE_SERVICE;
    strncpyz(buffer, cg->id, CGROUP_CHARTID_LINE_MAX);
    char *s = buffer;

    // skip to the last slash
    size_t len = strlen(s);
    while (len--) {
        if (unlikely(s[len] == '/')) {
            break;
        }
    }
    if (len) {
        s = &s[len + 1];
    }

    // remove extension
    len = strlen(s);
    while (len--) {
        if (unlikely(s[len] == '.')) {
            break;
        }
    }
    if (len) {
        s[len] = '\0';
    }

    freez(cg->name);
    cg->name = strdupz(s);

    freez(cg->chart_id);
    cg->chart_id = cgroup_chart_id_strdupz(s);
    substitute_dots_in_id(cg->chart_id);
    cg->hash_chart_id = simple_hash(cg->chart_id);
}

static inline struct cgroup *discovery_cgroup_add(const char *id) {
    netdata_log_debug(D_CGROUP, "adding to list, cgroup with id '%s'", id);

    struct cgroup *cg = callocz(1, sizeof(struct cgroup));

    cg->id = strdupz(id);
    cg->hash = simple_hash(cg->id);

    cg->name = strdupz(id);

    cg->intermediate_id = cgroup_chart_id_strdupz(id);

    cg->chart_id = cgroup_chart_id_strdupz(id);
    substitute_dots_in_id(cg->chart_id);
    cg->hash_chart_id = simple_hash(cg->chart_id);

    if (cgroup_use_unified_cgroups) {
        cg->options |= CGROUP_OPTIONS_IS_UNIFIED;
    }

    if (!discovered_cgroup_root)
        discovered_cgroup_root = cg;
    else {
        struct cgroup *t;
        for (t = discovered_cgroup_root; t->discovered_next; t = t->discovered_next) {
        }
        t->discovered_next = cg;
    }

    return cg;
}

static inline struct cgroup *discovery_cgroup_find(const char *id) {
    netdata_log_debug(D_CGROUP, "searching for cgroup '%s'", id);

    uint32_t hash = simple_hash(id);

    struct cgroup *cg;
    for(cg = discovered_cgroup_root; cg ; cg = cg->discovered_next) {
        if(hash == cg->hash && strcmp(id, cg->id) == 0)
            break;
    }

    netdata_log_debug(D_CGROUP, "cgroup '%s' %s in memory", id, (cg)?"found":"not found");
    return cg;
}

static int calc_cgroup_depth(const char *id) {
    int depth = 0;
    const char *s;
    for (s = id; *s; s++) {
        depth += unlikely(*s == '/');
    }
    return depth;
}

static inline void discovery_find_cgroup_in_dir(const char *dir) {
    if (!dir || !*dir) {
        dir = "/";
    }

    struct cgroup *cg = discovery_cgroup_find(dir);
    if (cg) {
        cg->available = 1;
        return;
    }

    if (cgroup_root_count >= cgroup_root_max) {
        nd_log_limit_static_global_var(erl, 3600, 0);
        nd_log_limit(&erl, NDLS_COLLECTORS, NDLP_WARNING, "CGROUP: maximum number of cgroups reached (%d). No more cgroups will be added.", cgroup_root_count);
        return;
    }

    if (cgroup_max_depth > 0) {
        int depth = calc_cgroup_depth(dir);
        if (depth > cgroup_max_depth) {
            nd_log_collector(NDLP_DEBUG, "CGROUP: '%s' is too deep (%d, while max is %d)", dir, depth, cgroup_max_depth);
            return;
        }
    }

    cg = discovery_cgroup_add(dir);
    cg->available = 1;
    cg->first_time_seen = 1;
    cg->function_ready = false;
    cgroup_root_count++;
}

static inline int discovery_find_walkdir(const char *base, const char *dirpath) {
    if (!dirpath)
        dirpath = base;

    netdata_log_debug(D_CGROUP, "searching for directories in '%s' (base '%s')", dirpath ? dirpath : "", base);

    size_t dirlen = strlen(dirpath), baselen = strlen(base);

    int ret = -1;
    int enabled = -1;

    const char *relative_path = &dirpath[baselen];
    if (!*relative_path)
        relative_path = "/";

    DIR *dir = opendir(dirpath);
    if(!dir) {
        collector_error("CGROUP: cannot open directory '%s'", base);
        return ret;
    }
    ret = 1;

    discovery_find_cgroup_in_dir(relative_path);

    struct dirent *de = NULL;
    while((de = readdir(dir))) {
        if (de->d_type == DT_DIR && ((de->d_name[0] == '.' && de->d_name[1] == '\0') ||
                                     (de->d_name[0] == '.' && de->d_name[1] == '.' && de->d_name[2] == '\0')))
            continue;

        if(de->d_type == DT_DIR) {
            if(enabled == -1) {
                const char *r = relative_path;
                if (*r == '\0')
                    r = "/";

                // do not decent in directories we are not interested
                enabled = matches_search_cgroup_paths(r);
            }

            if(enabled) {
                char *s = mallocz(dirlen + strlen(de->d_name) + 2);
                strcpy(s, dirpath);
                strcat(s, "/");
                strcat(s, de->d_name);
                int ret2 = discovery_find_walkdir(base, s);
                if (ret2 > 0)
                    ret += ret2;
                freez(s);
            }
        }
    }

    closedir(dir);
    return ret;
}

static inline void discovery_mark_as_unavailable_all_cgroups() {
    for (struct cgroup *cg = discovered_cgroup_root; cg; cg = cg->discovered_next) {
        cg->available = 0;
    }
}

static inline void discovery_update_filenames_cgroup_v1(struct cgroup *cg) {
    char filename[FILENAME_MAX + 1];
    struct stat buf;

    // CPU
    if (likely(cgroup_enable_cpuacct)) {
        if (unlikely(!cg->cpuacct_stat.staterr && !cg->cpuacct_stat.filename)) {
            snprintfz(filename, FILENAME_MAX, "%s%s/cpuacct.stat", cgroup_cpuacct_base, cg->id);
            if (!(cg->cpuacct_stat.staterr = stat(filename, &buf) != 0)) {
                cg->cpuacct_stat.filename = strdupz(filename);

                snprintfz(filename, FILENAME_MAX, "%s%s/cpuset.cpus", cgroup_cpuset_base, cg->id);
                cg->filename_cpuset_cpus = strdupz(filename);

                snprintfz(filename, FILENAME_MAX, "%s%s/cpu.cfs_period_us", cgroup_cpuacct_base, cg->id);
                cg->filename_cpu_cfs_period = strdupz(filename);

                snprintfz(filename, FILENAME_MAX, "%s%s/cpu.cfs_quota_us", cgroup_cpuacct_base, cg->id);
                cg->filename_cpu_cfs_quota = strdupz(filename);
            }
        }
        if (!is_cgroup_systemd_service(cg)) {
            if (unlikely(!cg->cpuacct_cpu_throttling.staterr && !cg->cpuacct_cpu_throttling.filename)) {
                snprintfz(filename, FILENAME_MAX, "%s%s/cpu.stat", cgroup_cpuacct_base, cg->id);
                if (!(cg->cpuacct_cpu_throttling.staterr = stat(filename, &buf) != 0)) {
                    cg->cpuacct_cpu_throttling.filename = strdupz(filename);
                }
            }

            if (cgroup_enable_cpuacct_cpu_shares) {
                if (unlikely(!cg->cpuacct_cpu_shares.staterr && !cg->cpuacct_cpu_shares.filename)) {
                    snprintfz(filename, FILENAME_MAX, "%s%s/cpu.shares", cgroup_cpuacct_base, cg->id);

                    if (!(cg->cpuacct_cpu_shares.staterr = stat(filename, &buf) != 0)) {
                        cg->cpuacct_cpu_shares.filename = strdupz(filename);
                    }
                }
            }
        }
    }

    // Memory
    if (likely(cgroup_enable_memory)) {
        if (unlikely(!cg->memory.staterr_mem_current && !cg->memory.filename_usage_in_bytes)) {
            snprintfz(filename, FILENAME_MAX, "%s%s/memory.usage_in_bytes", cgroup_memory_base, cg->id);
            if (!(cg->memory.staterr_mem_current = stat(filename, &buf) != 0)) {
                cg->memory.filename_usage_in_bytes = strdupz(filename);

                snprintfz(filename, FILENAME_MAX, "%s%s/memory.limit_in_bytes", cgroup_memory_base, cg->id);
                cg->filename_memory_limit = strdupz(filename);
            }
        }

        if (unlikely(!cg->memory.staterr_mem_stat && !cg->memory.filename_detailed)) {
            snprintfz(filename, FILENAME_MAX, "%s%s/memory.stat", cgroup_memory_base, cg->id);
            if (!(cg->memory.staterr_mem_stat = stat(filename, &buf) != 0)) {
                cg->memory.filename_detailed = strdupz(filename);
            }
        }

        if (unlikely(!cg->memory.staterr_failcnt && !cg->memory.filename_failcnt)) {
            snprintfz(filename, FILENAME_MAX, "%s%s/memory.failcnt", cgroup_memory_base, cg->id);
            if (!(cg->memory.staterr_failcnt = stat(filename, &buf) != 0)) {
                cg->memory.filename_failcnt = strdupz(filename);
            }
        }
    }

    // Blkio
    if (likely(cgroup_enable_blkio)) {
        if (unlikely(!cg->io_service_bytes.staterr && !cg->io_service_bytes.filename)) {
            snprintfz(filename, FILENAME_MAX, "%s%s/blkio.io_service_bytes_recursive", cgroup_blkio_base, cg->id);
            if (!(cg->io_service_bytes.staterr = stat(filename, &buf) != 0)) {
                cg->io_service_bytes.filename = strdupz(filename);
            } else {
                snprintfz(filename, FILENAME_MAX, "%s%s/blkio.io_service_bytes", cgroup_blkio_base, cg->id);
                if (!(cg->io_service_bytes.staterr = stat(filename, &buf) != 0)) {
                    cg->io_service_bytes.filename = strdupz(filename);
                }
            }
        }

        if (unlikely(!cg->io_serviced.staterr && !cg->io_serviced.filename)) {
            snprintfz(filename, FILENAME_MAX, "%s%s/blkio.io_serviced_recursive", cgroup_blkio_base, cg->id);
            if (!(cg->io_serviced.staterr = stat(filename, &buf) != 0)) {
                cg->io_serviced.filename = strdupz(filename);
            } else {
                snprintfz(filename, FILENAME_MAX, "%s%s/blkio.io_serviced", cgroup_blkio_base, cg->id);
                if (!(cg->io_serviced.staterr = stat(filename, &buf) != 0)) {
                    cg->io_serviced.filename = strdupz(filename);
                }
            }
        }

        if (unlikely(!cg->throttle_io_service_bytes.staterr && !cg->throttle_io_service_bytes.filename)) {
            snprintfz(
                filename, FILENAME_MAX, "%s%s/blkio.throttle.io_service_bytes_recursive", cgroup_blkio_base, cg->id);
            if (!(cg->throttle_io_service_bytes.staterr = stat(filename, &buf) != 0)) {
                cg->throttle_io_service_bytes.filename = strdupz(filename);
            } else {
                snprintfz(filename, FILENAME_MAX, "%s%s/blkio.throttle.io_service_bytes", cgroup_blkio_base, cg->id);
                if (!(cg->throttle_io_service_bytes.staterr = stat(filename, &buf) != 0)) {
                    cg->throttle_io_service_bytes.filename = strdupz(filename);
                }
            }
        }

        if (unlikely(!cg->throttle_io_serviced.staterr && !cg->throttle_io_serviced.filename)) {
            snprintfz(filename, FILENAME_MAX, "%s%s/blkio.throttle.io_serviced_recursive", cgroup_blkio_base, cg->id);
            if(!(cg->throttle_io_serviced.staterr = stat(filename, &buf) != 0)) {
                cg->throttle_io_serviced.filename = strdupz(filename);
            } else {
                snprintfz(filename, FILENAME_MAX, "%s%s/blkio.throttle.io_serviced", cgroup_blkio_base, cg->id);
                if (!(cg->throttle_io_serviced.staterr = stat(filename, &buf) != 0)) {
                    cg->throttle_io_serviced.filename = strdupz(filename);
                }
            }
        }

        if (unlikely(!cg->io_merged.staterr && !cg->io_merged.filename)) {
            snprintfz(filename, FILENAME_MAX, "%s%s/blkio.io_merged_recursive", cgroup_blkio_base, cg->id);
            if (!(cg->io_merged.staterr = stat(filename, &buf) != 0)) {
                cg->io_merged.filename = strdupz(filename);
            } else {
                snprintfz(filename, FILENAME_MAX, "%s%s/blkio.io_merged", cgroup_blkio_base, cg->id);
                if (!(cg->io_merged.staterr = stat(filename, &buf) != 0)) {
                    cg->io_merged.filename = strdupz(filename);
                }
            }
        }

        if (unlikely(!cg->io_queued.staterr && !cg->io_queued.filename)) {
            snprintfz(filename, FILENAME_MAX, "%s%s/blkio.io_queued_recursive", cgroup_blkio_base, cg->id);
            if (!(cg->io_queued.staterr = stat(filename, &buf) != 0)) {
                cg->io_queued.filename = strdupz(filename);
            } else {
                snprintfz(filename, FILENAME_MAX, "%s%s/blkio.io_queued", cgroup_blkio_base, cg->id);
                if (!(cg->io_queued.staterr = stat(filename, &buf) != 0)) {
                    cg->io_queued.filename = strdupz(filename);
                }
            }
        }
    }

    // Pids
    if (unlikely(!cg->pids_current.staterr && !cg->pids_current.filename)) {
        snprintfz(filename, FILENAME_MAX, "%s%s/pids.current", cgroup_pids_base, cg->id);
        if (!(cg->pids_current.staterr = stat(filename, &buf) != 0)) {
            cg->pids_current.filename = strdupz(filename);
        }
    }
}

static inline void discovery_update_filenames_cgroup_v2(struct cgroup *cg) {
    if (!cgroup_unified_exist)
        return;

    char filename[FILENAME_MAX + 1];
    struct stat buf;

    // CPU
    if (unlikely((!cg->cpuacct_stat.staterr && !cg->cpuacct_stat.filename))) {
        snprintfz(filename, FILENAME_MAX, "%s%s/cpu.stat", cgroup_unified_base, cg->id);
        if (!(cg->cpuacct_stat.staterr = stat(filename, &buf) != 0)) {
            cg->cpuacct_stat.filename = strdupz(filename);
            cg->filename_cpuset_cpus = NULL;
            cg->filename_cpu_cfs_period = NULL;

            snprintfz(filename, FILENAME_MAX, "%s%s/cpu.max", cgroup_unified_base, cg->id);
            cg->filename_cpu_cfs_quota = strdupz(filename);
        }
    }
    if (cgroup_enable_cpuacct_cpu_shares) {
        if (unlikely(!cg->cpuacct_cpu_shares.staterr && !cg->cpuacct_cpu_shares.filename)) {
            snprintfz(filename, FILENAME_MAX, "%s%s/cpu.weight", cgroup_unified_base, cg->id);
            if (!(cg->cpuacct_cpu_shares.staterr = stat(filename, &buf) != 0)) {
                cg->cpuacct_cpu_shares.filename = strdupz(filename);
            }
        }
    }

    // Memory
    if (unlikely(!cg->memory.staterr_mem_current && !cg->memory.filename_usage_in_bytes)) {
        snprintfz(filename, FILENAME_MAX, "%s%s/memory.current", cgroup_unified_base, cg->id);
        if (!(cg->memory.staterr_mem_current = stat(filename, &buf) != 0)) {
            cg->memory.filename_usage_in_bytes = strdupz(filename);

            snprintfz(filename, FILENAME_MAX, "%s%s/memory.max", cgroup_unified_base, cg->id);
            cg->filename_memory_limit = strdupz(filename);
        }
    }

    if (unlikely(!cg->memory.staterr_mem_stat && !cg->memory.filename_detailed)) {
        snprintfz(filename, FILENAME_MAX, "%s%s/memory.stat", cgroup_unified_base, cg->id);
        if (!(cg->memory.staterr_mem_stat = stat(filename, &buf) != 0)) {
            cg->memory.filename_detailed = strdupz(filename);
        }
    }

    // Blkio
    if (unlikely(!cg->io_service_bytes.staterr && !cg->io_service_bytes.filename)) {
        snprintfz(filename, FILENAME_MAX, "%s%s/io.stat", cgroup_unified_base, cg->id);
        if (!(cg->io_service_bytes.staterr = stat(filename, &buf) != 0)) {
            cg->io_service_bytes.filename = strdupz(filename);
        }
    }

    if (unlikely(!cg->io_serviced.staterr && !cg->io_serviced.filename)) {
        snprintfz(filename, FILENAME_MAX, "%s%s/io.stat", cgroup_unified_base, cg->id);
        if (!(cg->io_serviced.staterr = stat(filename, &buf) != 0)) {
            cg->io_serviced.filename = strdupz(filename);
        }
    }

    // PSI
    if (unlikely(!cg->cpu_pressure.staterr && !cg->cpu_pressure.filename)) {
        snprintfz(filename, FILENAME_MAX, "%s%s/cpu.pressure", cgroup_unified_base, cg->id);
        if (!(cg->cpu_pressure.staterr = stat(filename, &buf) != 0)) {
            cg->cpu_pressure.filename = strdupz(filename);
            cg->cpu_pressure.some.enabled = CONFIG_BOOLEAN_YES;
            cg->cpu_pressure.full.enabled = CONFIG_BOOLEAN_YES;
        }
    }

    if (unlikely(!cg->io_pressure.staterr && !cg->io_pressure.filename)) {
        snprintfz(filename, FILENAME_MAX, "%s%s/io.pressure", cgroup_unified_base, cg->id);
        if (!(cg->io_pressure.staterr = stat(filename, &buf) != 0)) {
            cg->io_pressure.filename = strdupz(filename);
            cg->io_pressure.some.enabled = CONFIG_BOOLEAN_YES;
            cg->io_pressure.full.enabled = CONFIG_BOOLEAN_YES;
        }
    }

    if (unlikely(!cg->memory_pressure.staterr && !cg->memory_pressure.filename)) {
        snprintfz(filename, FILENAME_MAX, "%s%s/memory.pressure", cgroup_unified_base, cg->id);
        if (!(cg->memory_pressure.staterr = stat(filename, &buf) != 0)) {
            cg->memory_pressure.filename = strdupz(filename);
            cg->memory_pressure.some.enabled = CONFIG_BOOLEAN_YES;
            cg->memory_pressure.full.enabled = CONFIG_BOOLEAN_YES;
        }
    }

    if (unlikely(!cg->irq_pressure.staterr && !cg->irq_pressure.filename)) {
        snprintfz(filename, FILENAME_MAX, "%s%s/irq.pressure", cgroup_unified_base, cg->id);
        if (!(cg->irq_pressure.staterr = stat(filename, &buf) != 0)) {
            cg->irq_pressure.filename = strdupz(filename);
            cg->irq_pressure.some.enabled = CONFIG_BOOLEAN_YES;
            cg->irq_pressure.full.enabled = CONFIG_BOOLEAN_YES;
        }
    }

    // Pids
    if (unlikely(!cg->pids_current.staterr && !cg->pids_current.filename)) {
        snprintfz(filename, FILENAME_MAX, "%s%s/pids.current", cgroup_unified_base, cg->id);
        if (!(cg->pids_current.staterr = stat(filename, &buf) != 0)) {
            cg->pids_current.filename = strdupz(filename);
        }
    }
}

static inline void discovery_update_filenames_all_cgroups() {
    for (struct cgroup *cg = discovered_cgroup_root; cg; cg = cg->discovered_next) {
        if (unlikely(!cg->available || !cg->enabled || cg->pending_renames))
            continue;

        if (!cgroup_use_unified_cgroups)
            discovery_update_filenames_cgroup_v1(cg);
        else
            discovery_update_filenames_cgroup_v2(cg);
    }
}

static inline void discovery_cleanup_all_cgroups() {
    struct cgroup *cg = discovered_cgroup_root, *last = NULL;

    for(; cg ;) {
        if(!cg->available) {
            // enable the first duplicate cgroup
            {
                struct cgroup *t;
                for (t = discovered_cgroup_root; t; t = t->discovered_next) {
                    if (t != cg && t->available && !t->enabled && t->options & CGROUP_OPTIONS_DISABLED_DUPLICATE &&
                        (is_cgroup_systemd_service(t) == is_cgroup_systemd_service(cg)) &&
                        t->hash_chart_id == cg->hash_chart_id && !strcmp(t->chart_id, cg->chart_id)) {
                        netdata_log_debug(D_CGROUP, "Enabling duplicate of cgroup '%s' with id '%s', because the original with id '%s' stopped.", t->chart_id, t->id, cg->id);
                        t->enabled = 1;
                        t->options &= ~CGROUP_OPTIONS_DISABLED_DUPLICATE;
                        break;
                    }
                }
            }

            if(!last)
                discovered_cgroup_root = cg->discovered_next;
            else
                last->discovered_next = cg->discovered_next;

            cgroup_free(cg);

            if(!last)
                cg = discovered_cgroup_root;
            else
                cg = last->discovered_next;
        }
        else {
            last = cg;
            cg = cg->discovered_next;
        }
    }
}

static inline void discovery_copy_discovered_cgroups_to_reader() {
    netdata_log_debug(D_CGROUP, "copy discovered cgroups to the main group list");

    struct cgroup *cg;

    for (cg = discovered_cgroup_root; cg; cg = cg->discovered_next) {
        cg->next = cg->discovered_next;
    }

    cgroup_root = discovered_cgroup_root;
}

static inline void discovery_share_cgroups_with_ebpf() {
    struct cgroup *cg;
    int count;
    struct stat buf;

    if (shm_mutex_cgroup_ebpf == SEM_FAILED) {
        return;
    }
    sem_wait(shm_mutex_cgroup_ebpf);

    for (cg = cgroup_root, count = 0; cg; cg = cg->next, count++) {
        netdata_ebpf_cgroup_shm_body_t *ptr = &shm_cgroup_ebpf.body[count];
        char *prefix = (is_cgroup_systemd_service(cg)) ? services_chart_id_prefix : cgroup_chart_id_prefix;
        snprintfz(ptr->name, CGROUP_EBPF_NAME_SHARED_LENGTH - 1, "%s%s", prefix, cg->chart_id);
        ptr->hash = simple_hash(ptr->name);
        ptr->options = cg->options;
        ptr->enabled = cg->enabled;
        if (cgroup_use_unified_cgroups) {
            snprintfz(ptr->path, FILENAME_MAX, "%s%s/cgroup.procs", cgroup_unified_base, cg->id);
            if (likely(stat(ptr->path, &buf) == -1)) {
                ptr->path[0] = '\0';
                ptr->enabled = 0;
            }
        } else {
            is_cgroup_procs_exist(ptr, cg->id);
        }

        netdata_log_debug(D_CGROUP, "cgroup shared: NAME=%s, ENABLED=%d", ptr->name, ptr->enabled);
    }

    shm_cgroup_ebpf.header->cgroup_root_count = count;
    sem_post(shm_mutex_cgroup_ebpf);
}

static inline void discovery_find_all_cgroups_v1() {
    if (cgroup_enable_cpuacct) {
        if (discovery_find_walkdir(cgroup_cpuacct_base, NULL) == -1) {
            cgroup_enable_cpuacct = false;
            collector_error("CGROUP: disabled cpu statistics.");
        }
    }

    if (cgroup_enable_blkio) {
        if (discovery_find_walkdir(cgroup_blkio_base, NULL) == -1) {
            cgroup_enable_blkio = false;
            collector_error("CGROUP: disabled blkio statistics.");
        }
    }

    if (cgroup_enable_memory) {
        if (discovery_find_walkdir(cgroup_memory_base, NULL) == -1) {
            cgroup_enable_memory = false;
            collector_error("CGROUP: disabled memory statistics.");
        }
    }
}

static inline void discovery_find_all_cgroups_v2() {
    if (cgroup_unified_exist) {
        if (discovery_find_walkdir(cgroup_unified_base, NULL) == -1) {
            cgroup_unified_exist = false;
            collector_error("CGROUP: disabled unified cgroups statistics.");
        }
    }
}

static int is_digits_only(const char *s) {
    do {
        if (!isdigit(*s++)) {
            return 0;
        }
    } while (*s);

    return 1;
}

static int is_cgroup_k8s_container(const char *id) {
    // examples:
    // https://github.com/netdata/netdata/blob/0fc101679dcd12f1cb8acdd07bb4c85d8e553e53/collectors/cgroups.plugin/cgroup-name.sh#L121-L147
    const char *p = id;
    const char *pp = NULL;
    int i = 0;
    size_t l = 3; // pod
    while ((p = strstr(p, "pod"))) {
        i++;
        p += l;
        pp = p;
    }
    return !(i < 2 || !pp || !(pp = strchr(pp, '/')) || !pp++ || !*pp);
}

#define TASK_COMM_LEN 16

static int k8s_get_container_first_proc_comm(const char *id, char *comm) {
    if (!is_cgroup_k8s_container(id)) {
        return 1;
    }

    static procfile *ff = NULL;

    char filename[FILENAME_MAX + 1];
    snprintfz(filename, FILENAME_MAX, "%s/%s/cgroup.procs", cgroup_cpuacct_base, id);

    ff = procfile_reopen(ff, filename, NULL, CGROUP_PROCFILE_FLAG);
    if (unlikely(!ff)) {
        netdata_log_debug(D_CGROUP, "CGROUP: k8s_is_pause_container(): cannot open file '%s'.", filename);
        return 1;
    }

    ff = procfile_readall(ff);
    if (unlikely(!ff)) {
        netdata_log_debug(D_CGROUP, "CGROUP: k8s_is_pause_container(): cannot read file '%s'.", filename);
        return 1;
    }

    unsigned long lines = procfile_lines(ff);
    if (likely(lines < 2)) {
        return 1;
    }

    char *pid = procfile_lineword(ff, 0, 0);
    if (!pid || !*pid) {
        return 1;
    }

    snprintfz(filename, FILENAME_MAX, "%s/proc/%s/comm", netdata_configured_host_prefix, pid);

    ff = procfile_reopen(ff, filename, NULL, PROCFILE_FLAG_DEFAULT);
    if (unlikely(!ff)) {
        netdata_log_debug(D_CGROUP, "CGROUP: k8s_is_pause_container(): cannot open file '%s'.", filename);
        return 1;
    }

    ff = procfile_readall(ff);
    if (unlikely(!ff)) {
        netdata_log_debug(D_CGROUP, "CGROUP: k8s_is_pause_container(): cannot read file '%s'.", filename);
        return 1;
    }

    lines = procfile_lines(ff);
    if (unlikely(lines != 2)) {
        return 1;
    }

    char *proc_comm = procfile_lineword(ff, 0, 0);
    if (!proc_comm || !*proc_comm) {
        return 1;
    }

    strncpyz(comm, proc_comm, TASK_COMM_LEN);
    return 0;
}

static inline void discovery_process_first_time_seen_cgroup(struct cgroup *cg) {
    if (!cg->first_time_seen) {
        return;
    }
    cg->first_time_seen = 0;

    char comm[TASK_COMM_LEN + 1];

    if (cg->container_orchestrator == CGROUPS_ORCHESTRATOR_UNSET) {
        if (strstr(cg->id, "kubepods")) {
            cg->container_orchestrator = CGROUPS_ORCHESTRATOR_K8S;
        } else {
            cg->container_orchestrator = CGROUPS_ORCHESTRATOR_UNKNOWN;
        }
    }

    if (is_inside_k8s && !k8s_get_container_first_proc_comm(cg->id, comm)) {
        // container initialization may take some time when CPU % is high
        // seen on GKE: comm is '6' before 'runc:[2:INIT]' (dunno if it could be another number)
        if (is_digits_only(comm) || matches_entrypoint_parent_process_comm(comm)) {
            cg->first_time_seen = 1;
            return;
        }
        if (!strcmp(comm, "pause")) {
            // a container that holds the network namespace for the pod
            // we don't need to collect its metrics
            cg->processed = 1;
            return;
        }
    }

    if (matches_systemd_services_cgroups(cg->id)) {
        netdata_log_debug(D_CGROUP, "cgroup '%s' (name '%s') matches 'cgroups to match as systemd services'", cg->id, cg->chart_id);
        convert_cgroup_to_systemd_service(cg);
        return;
    }

    if (matches_enabled_cgroup_renames(cg->id)) {
        netdata_log_debug(D_CGROUP, "cgroup '%s' (name '%s') matches 'run script to rename cgroups matching', will try to rename it", cg->id, cg->chart_id);
        if (is_inside_k8s && is_cgroup_k8s_container(cg->id)) {
            // it may take up to a minute for the K8s API to return data for the container
            // tested on AWS K8s cluster with 100% CPU utilization
            cg->pending_renames = 9; // 1.5 minute
        } else {
            cg->pending_renames = 2;
        }
    }
}

static int discovery_is_cgroup_duplicate(struct cgroup *cg) {
    // https://github.com/netdata/netdata/issues/797#issuecomment-241248884
    struct cgroup *c;
    for (c = discovered_cgroup_root; c; c = c->discovered_next) {
        if (c != cg && c->enabled && (is_cgroup_systemd_service(c) == is_cgroup_systemd_service(cg)) &&
            c->hash_chart_id == cg->hash_chart_id && !strcmp(c->chart_id, cg->chart_id)) {
            collector_error(
                    "CGROUP: chart id '%s' already exists with id '%s' and is enabled and available. Disabling cgroup with id '%s'.",
                    cg->chart_id,
                    c->id,
                    cg->id);
            return 1;
        }
    }
    return 0;
}

// ----------------------------------------------------------------------------
// ebpf shared memory

static void netdata_cgroup_ebpf_set_values(size_t length)
{
    sem_wait(shm_mutex_cgroup_ebpf);

    shm_cgroup_ebpf.header->cgroup_max = cgroup_root_max;
    shm_cgroup_ebpf.header->systemd_enabled = CONFIG_BOOLEAN_YES;
    shm_cgroup_ebpf.header->body_length = length;

    sem_post(shm_mutex_cgroup_ebpf);
}

static void netdata_cgroup_ebpf_initialize_shm()
{
    shm_fd_cgroup_ebpf = shm_open(NETDATA_SHARED_MEMORY_EBPF_CGROUP_NAME, O_CREAT | O_RDWR, 0660);
    if (shm_fd_cgroup_ebpf < 0) {
        collector_error("Cannot initialize shared memory used by cgroup and eBPF, integration won't happen.");
        return;
    }

    size_t length = sizeof(netdata_ebpf_cgroup_shm_header_t) + cgroup_root_max * sizeof(netdata_ebpf_cgroup_shm_body_t);
    if (ftruncate(shm_fd_cgroup_ebpf, length)) {
        collector_error("Cannot set size for shared memory.");
        goto end_init_shm;
    }

    shm_cgroup_ebpf.header = (netdata_ebpf_cgroup_shm_header_t *)
        nd_mmap(NULL, length, PROT_READ | PROT_WRITE, MAP_SHARED, shm_fd_cgroup_ebpf, 0);

    if (unlikely(MAP_FAILED == shm_cgroup_ebpf.header)) {
        shm_cgroup_ebpf.header = NULL;
        collector_error("Cannot map shared memory used between cgroup and eBPF, integration won't happen");
        goto end_init_shm;
    }
    shm_cgroup_ebpf.body = (netdata_ebpf_cgroup_shm_body_t *) ((char *)shm_cgroup_ebpf.header +
                                                               sizeof(netdata_ebpf_cgroup_shm_header_t));

    shm_mutex_cgroup_ebpf = sem_open(NETDATA_NAMED_SEMAPHORE_EBPF_CGROUP_NAME, O_CREAT,
                                     S_IRUSR | S_IWUSR | S_IRGRP | S_IWGRP | S_IROTH | S_IWOTH, 1);

    if (shm_mutex_cgroup_ebpf != SEM_FAILED) {
        netdata_cgroup_ebpf_set_values(length);
        return;
    }

    collector_error("Cannot create semaphore, integration between eBPF and cgroup won't happen");
    nd_munmap(shm_cgroup_ebpf.header, length);
    shm_cgroup_ebpf.header = NULL;

    end_init_shm:
    close(shm_fd_cgroup_ebpf);
    shm_fd_cgroup_ebpf = -1;
    shm_unlink(NETDATA_SHARED_MEMORY_EBPF_CGROUP_NAME);
}

static void cgroup_cleanup_ebpf_integration()
{
    if (shm_mutex_cgroup_ebpf != SEM_FAILED) {
        sem_close(shm_mutex_cgroup_ebpf);
    }

    if (shm_cgroup_ebpf.header) {
        shm_cgroup_ebpf.header->cgroup_root_count = 0;
        nd_munmap(shm_cgroup_ebpf.header, shm_cgroup_ebpf.header->body_length);
    }

    if (shm_fd_cgroup_ebpf > 0) {
        close(shm_fd_cgroup_ebpf);
    }
}

// ----------------------------------------------------------------------------
// cgroup network interfaces

#define CGROUP_NETWORK_INTERFACE_MAX_LINE 2048

static inline void read_cgroup_network_interfaces(struct cgroup *cg) {
    netdata_log_debug(D_CGROUP, "looking for the network interfaces of cgroup '%s' with chart id '%s'", cg->id, cg->chart_id);

    char cgroup_identifier[CGROUP_NETWORK_INTERFACE_MAX_LINE + 1];

    if(!(cg->options & CGROUP_OPTIONS_IS_UNIFIED)) {
        snprintfz(cgroup_identifier, CGROUP_NETWORK_INTERFACE_MAX_LINE, "%s%s", cgroup_cpuacct_base, cg->id);
    }
    else {
        snprintfz(cgroup_identifier, CGROUP_NETWORK_INTERFACE_MAX_LINE, "%s%s", cgroup_unified_base, cg->id);
    }

    netdata_log_debug(D_CGROUP, "executing cgroup_identifier %s --cgroup '%s' for cgroup '%s'", cgroups_network_interface_script, cgroup_identifier, cg->id);
    POPEN_INSTANCE *instance = spawn_popen_run_variadic(cgroups_network_interface_script, "--cgroup", cgroup_identifier, NULL);
    if(!instance) {
        collector_error("CGROUP: cannot popen(%s --cgroup \"%s\", \"r\").", cgroups_network_interface_script, cgroup_identifier);
        return;
    }

    char *s;
    char buffer[CGROUP_NETWORK_INTERFACE_MAX_LINE + 1];
    while((s = fgets(buffer, CGROUP_NETWORK_INTERFACE_MAX_LINE, spawn_popen_stdout(instance)))) {
        trim(s);

        if(*s && *s != '\n') {
            char *t = s;
            while(*t && *t != ' ') t++;
            if(*t == ' ') {
                *t = '\0';
                t++;
            }

            if(!*s) {
                collector_error("CGROUP: empty host interface returned by script");
                continue;
            }

            if(!*t) {
                collector_error("CGROUP: empty guest interface returned by script");
                continue;
            }

            struct cgroup_network_interface *i = callocz(1, sizeof(struct cgroup_network_interface));
            i->host_device = strdupz(s);
            i->container_device = strdupz(t);
            i->next = cg->interfaces;
            cg->interfaces = i;

            collector_info("CGROUP: cgroup '%s' has network interface '%s' as '%s'", cg->id, i->host_device, i->container_device);

            // register a device rename to proc_net_dev.c
            cgroup_rename_task_add(
                i->host_device,
                i->container_device,
                cg->chart_id,
                cg->chart_labels,
                k8s_is_kubepod(cg) ? "k8s." : "",
                cgroup_netdev_get(cg));
        }
    }

    spawn_popen_wait(instance);
}

static inline void discovery_process_cgroup(struct cgroup *cg) {
    if (!cg->available || cg->processed) {
        return;
    }

    if (cg->first_time_seen) {
        worker_is_busy(WORKER_DISCOVERY_PROCESS_FIRST_TIME);
        discovery_process_first_time_seen_cgroup(cg);
        if (unlikely(cg->first_time_seen || cg->processed)) {
            return;
        }
    }

    if (cg->pending_renames) {
        worker_is_busy(WORKER_DISCOVERY_PROCESS_RENAME);
        discovery_rename_cgroup(cg);
        if (unlikely(cg->pending_renames || cg->processed)) {
            return;
        }
    }

    cg->processed = 1;

    if ((strlen(cg->chart_id) + strlen(cgroup_chart_id_prefix)) >= RRD_ID_LENGTH_MAX) {
        collector_info("cgroup '%s' (chart id '%s') disabled because chart_id exceeds the limit (RRD_ID_LENGTH_MAX)", cg->id, cg->chart_id);
        return;
    }

    if (is_cgroup_systemd_service(cg)) {
        if (discovery_is_cgroup_duplicate(cg)) {
            cg->enabled = 0;
            cg->options |= CGROUP_OPTIONS_DISABLED_DUPLICATE;
            return;
        }
        if (!cg->chart_labels)
            cg->chart_labels = rrdlabels_create();
        rrdlabels_add(cg->chart_labels, "service_name", cg->name, RRDLABEL_SRC_AUTO);
        cg->enabled = 1;
        return;
    }

    if (cg->options & CGROUP_OPTIONS_DISABLED_EXCLUDED) {
        cg->enabled = 0;
        return;
    }

    if (!(cg->enabled = matches_enabled_cgroup_names(cg->name))) {
        netdata_log_debug(D_CGROUP, "cgroup '%s' (name '%s') disabled by 'enable by default cgroups names matching'", cg->id, cg->name);
        return;
    }

    if (!(cg->enabled = matches_enabled_cgroup_paths(cg->id))) {
        netdata_log_debug(D_CGROUP, "cgroup '%s' (name '%s') disabled by 'enable by default cgroups matching'", cg->id, cg->name);
        return;
    }

    if (discovery_is_cgroup_duplicate(cg)) {
        cg->enabled = 0;
        cg->options |= CGROUP_OPTIONS_DISABLED_DUPLICATE;
        return;
    }

    if (!cg->chart_labels)
        cg->chart_labels = rrdlabels_create();

    if (!k8s_is_kubepod(cg)) {
        rrdlabels_add(cg->chart_labels, "cgroup_name", cg->name, RRDLABEL_SRC_AUTO);
        if (!rrdlabels_exist(cg->chart_labels, "image"))
            rrdlabels_add(cg->chart_labels, "image", "", RRDLABEL_SRC_AUTO);
    }

    worker_is_busy(WORKER_DISCOVERY_PROCESS_NETWORK);
    read_cgroup_network_interfaces(cg);
}

static inline void discovery_find_all_cgroups() {
    netdata_log_debug(D_CGROUP, "searching for cgroups");

    worker_is_busy(WORKER_DISCOVERY_INIT);
    discovery_mark_as_unavailable_all_cgroups();

    worker_is_busy(WORKER_DISCOVERY_FIND);
    if (!cgroup_use_unified_cgroups) {
        discovery_find_all_cgroups_v1();
    } else {
        discovery_find_all_cgroups_v2();
    }

    for (struct cgroup *cg = discovered_cgroup_root; cg && service_running(SERVICE_COLLECTORS); cg = cg->discovered_next) {
        worker_is_busy(WORKER_DISCOVERY_PROCESS);
        discovery_process_cgroup(cg);
    }

    worker_is_busy(WORKER_DISCOVERY_UPDATE);
    discovery_update_filenames_all_cgroups();

    worker_is_busy(WORKER_DISCOVERY_LOCK);
    uv_mutex_lock(&cgroup_root_mutex);

    worker_is_busy(WORKER_DISCOVERY_CLEANUP);
    discovery_cleanup_all_cgroups();

    worker_is_busy(WORKER_DISCOVERY_COPY);
    discovery_copy_discovered_cgroups_to_reader();

    uv_mutex_unlock(&cgroup_root_mutex);

    worker_is_busy(WORKER_DISCOVERY_SHARE);
    discovery_share_cgroups_with_ebpf();

    netdata_log_debug(D_CGROUP, "done searching for cgroups");
}

void cgroup_discovery_worker(void *ptr)
{
    UNUSED(ptr);
    uv_thread_set_name_np("P[cgroupsdisc]");

    worker_register("CGROUPSDISC");
    worker_register_job_name(WORKER_DISCOVERY_INIT,               "init");
    worker_register_job_name(WORKER_DISCOVERY_FIND,               "find");
    worker_register_job_name(WORKER_DISCOVERY_PROCESS,            "process");
    worker_register_job_name(WORKER_DISCOVERY_PROCESS_RENAME,     "rename");
    worker_register_job_name(WORKER_DISCOVERY_PROCESS_NETWORK,    "network");
    worker_register_job_name(WORKER_DISCOVERY_PROCESS_FIRST_TIME, "new");
    worker_register_job_name(WORKER_DISCOVERY_UPDATE,             "update");
    worker_register_job_name(WORKER_DISCOVERY_CLEANUP,            "cleanup");
    worker_register_job_name(WORKER_DISCOVERY_COPY,               "copy");
    worker_register_job_name(WORKER_DISCOVERY_SHARE,              "share");
    worker_register_job_name(WORKER_DISCOVERY_LOCK,               "lock");

    entrypoint_parent_process_comm = simple_pattern_create(
            " runc:[* " // http://terenceli.github.io/%E6%8A%80%E6%9C%AF/2021/12/28/runc-internals-3)
            " exe ", // https://github.com/falcosecurity/falco/blob/9d41b0a151b83693929d3a9c84f7c5c85d070d3a/rules/falco_rules.yaml#L1961
            NULL,
            SIMPLE_PATTERN_EXACT, true);

    service_register(NULL, NULL, NULL);

    netdata_cgroup_ebpf_initialize_shm();

    while (service_running(SERVICE_COLLECTORS)) {
        worker_is_idle();

        uv_mutex_lock(&discovery_thread.mutex);
        uv_cond_wait(&discovery_thread.cond_var, &discovery_thread.mutex);
        uv_mutex_unlock(&discovery_thread.mutex);

        if (unlikely(!service_running(SERVICE_COLLECTORS)))
            break;

        discovery_find_all_cgroups();
    }

    // free all cgroups
    uv_mutex_lock(&cgroup_root_mutex);
    while(cgroup_root) {
        struct cgroup *cg = cgroup_root;
        cgroup_root = cg->next;
        cgroup_free(cg);
    }
    uv_mutex_unlock(&cgroup_root_mutex);

    collector_info("discovery thread stopped");
    cgroup_cleanup_ebpf_integration();
    worker_unregister();
    service_exits();
    __atomic_store_n(&discovery_thread.exited,1,__ATOMIC_RELAXED);
}
