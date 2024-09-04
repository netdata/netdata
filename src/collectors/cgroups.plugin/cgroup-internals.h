#include "sys_fs_cgroup.h"

#ifndef NETDATA_CGROUP_INTERNALS_H
#define NETDATA_CGROUP_INTERNALS_H 1

#ifdef NETDATA_INTERNAL_CHECKS
#define CGROUP_PROCFILE_FLAG PROCFILE_FLAG_DEFAULT
#else
#define CGROUP_PROCFILE_FLAG PROCFILE_FLAG_NO_ERROR_ON_FILE_IO
#endif

struct blkio {
    char *filename;
    bool staterr;

    int updated;

    unsigned long long Read;
    unsigned long long Write;
};

struct pids {
    char *filename;
    bool staterr;

    int updated;

    unsigned long long pids_current;
};

// https://www.kernel.org/doc/Documentation/cgroup-v1/memory.txt
struct memory {
    ARL_BASE *arl_base;
    ARL_ENTRY *arl_dirty;
    ARL_ENTRY *arl_swap;

    char *filename_usage_in_bytes;
    char *filename_detailed;
    char *filename_msw_usage_in_bytes;
    char *filename_failcnt;

    bool staterr_mem_current;
    bool staterr_mem_stat;
    bool staterr_failcnt;
    bool staterr_swap;

    int updated_usage_in_bytes;
    int updated_detailed;
    int updated_msw_usage_in_bytes;
    int updated_failcnt;

    int detailed_has_dirty;
    int detailed_has_swap;

    unsigned long long anon;
    unsigned long long kernel_stack;
    unsigned long long slab;
    unsigned long long sock;
    unsigned long long anon_thp;

    unsigned long long total_cache;
    unsigned long long total_rss;
    unsigned long long total_rss_huge;
    unsigned long long total_mapped_file;
    unsigned long long total_writeback;
    unsigned long long total_dirty;
    unsigned long long total_swap;
    unsigned long long total_pgpgin;
    unsigned long long total_pgpgout;
    unsigned long long total_pgfault;
    unsigned long long total_pgmajfault;

    unsigned long long total_inactive_file;

    // single file metrics
    unsigned long long usage_in_bytes;
    unsigned long long msw_usage_in_bytes;
    unsigned long long failcnt;
};

// https://www.kernel.org/doc/Documentation/cgroup-v1/cpuacct.txt
struct cpuacct_stat {
    char *filename;
    bool staterr;

    int updated;

    unsigned long long user;           // v1, v2(user_usec)
    unsigned long long system;         // v1, v2(system_usec)
};

// https://www.kernel.org/doc/Documentation/cgroup-v1/cpuacct.txt
struct cpuacct_usage {
    char *filename;
    bool disabled;
    int updated;

    unsigned int cpus;
    unsigned long long *cpu_percpu;
};

// represents cpuacct/cpu.stat, for v2 'cpuacct_stat' is used for 'user_usec', 'system_usec'
struct cpuacct_cpu_throttling {
    char *filename;
    bool staterr;

    int updated;

    unsigned long long nr_periods;
    unsigned long long nr_throttled;
    unsigned long long throttled_time;

    unsigned long long nr_throttled_perc;
};

// https://access.redhat.com/documentation/en-us/red_hat_enterprise_linux/6/html/resource_management_guide/sec-cpu#sect-cfs
// https://access.redhat.com/documentation/en-us/red_hat_enterprise_linux/8/html/managing_monitoring_and_updating_the_kernel/using-cgroups-v2-to-control-distribution-of-cpu-time-for-applications_managing-monitoring-and-updating-the-kernel#proc_controlling-distribution-of-cpu-time-for-applications-by-adjusting-cpu-weight_using-cgroups-v2-to-control-distribution-of-cpu-time-for-applications
struct cpuacct_cpu_shares {
    char *filename;
    bool staterr;

    int updated;

    unsigned long long shares;
};

struct cgroup_network_interface {
    const char *host_device;
    const char *container_device;
    struct cgroup_network_interface *next;
};

enum cgroups_container_orchestrator {
    CGROUPS_ORCHESTRATOR_UNSET,
    CGROUPS_ORCHESTRATOR_UNKNOWN,
    CGROUPS_ORCHESTRATOR_K8S
};


// *** WARNING *** The fields are not thread safe. Take care of safe usage.
struct cgroup {
    uint32_t options;

    int first_time_seen; // first time seen by the discoverer
    int processed;       // the discoverer is done processing a cgroup (resolved name, set 'enabled' option)

    char available;      // found in the filesystem
    char enabled;        // enabled in the config

    bool function_ready; // true after the first iteration of chart creation/update

    char pending_renames;

    char *id;
    uint32_t hash;

    char *intermediate_id; // TODO: remove it when the renaming script is fixed

    char *chart_id;
    uint32_t hash_chart_id;

    // 'cgroup_name' label value.
    // by default this is the *id (path), later changed to the resolved name (cgroup-name.sh) or systemd service name.
    char *name;

    RRDLABELS *chart_labels;

    int container_orchestrator;

    struct cpuacct_stat cpuacct_stat;
    struct cpuacct_usage cpuacct_usage;
    struct cpuacct_cpu_throttling cpuacct_cpu_throttling;
    struct cpuacct_cpu_shares cpuacct_cpu_shares;

    struct memory memory;

    struct blkio io_service_bytes;              // bytes
    struct blkio io_serviced;                   // operations

    struct blkio throttle_io_service_bytes;     // bytes
    struct blkio throttle_io_serviced;          // operations

    struct blkio io_merged;                     // operations
    struct blkio io_queued;                     // operations

    struct pids pids_current;

    struct cgroup_network_interface *interfaces;

    struct pressure cpu_pressure;
    struct pressure io_pressure;
    struct pressure memory_pressure;
    struct pressure irq_pressure;

    // Cpu
    RRDSET *st_cpu;
    RRDDIM *st_cpu_rd_user;
    RRDDIM *st_cpu_rd_system;

    RRDSET *st_cpu_limit;
    RRDSET *st_cpu_per_core;
    RRDSET *st_cpu_nr_throttled;
    RRDSET *st_cpu_throttled_time;
    RRDSET *st_cpu_shares;

    // Memory
    RRDSET *st_mem;
    RRDDIM *st_mem_rd_ram;
    RRDDIM *st_mem_rd_swap;

    RRDSET *st_mem_utilization;
    RRDSET *st_writeback;
    RRDSET *st_mem_activity;
    RRDSET *st_pgfaults;
    RRDSET *st_mem_usage;
    RRDSET *st_mem_usage_limit;
    RRDSET *st_mem_failcnt;

    // Blkio
    RRDSET *st_io;
    RRDDIM *st_io_rd_read;
    RRDDIM *st_io_rd_written;

    RRDSET *st_serviced_ops;

    RRDSET *st_throttle_io;
    RRDDIM *st_throttle_io_rd_read;
    RRDDIM *st_throttle_io_rd_written;

    RRDSET *st_throttle_serviced_ops;

    RRDSET *st_queued_ops;
    RRDSET *st_merged_ops;

    // Pids
    RRDSET *st_pids;
    RRDDIM *st_pids_rd_pids_current;

    // per cgroup chart variables
    char *filename_cpuset_cpus;
    unsigned long long cpuset_cpus;

    char *filename_cpu_cfs_period;
    unsigned long long cpu_cfs_period;

    char *filename_cpu_cfs_quota;
    unsigned long long cpu_cfs_quota;

    const RRDVAR_ACQUIRED *chart_var_cpu_limit;
    NETDATA_DOUBLE prev_cpu_usage;

    char *filename_memory_limit;
    unsigned long long memory_limit;
    const RRDVAR_ACQUIRED *chart_var_memory_limit;

    char *filename_memoryswap_limit;
    unsigned long long memoryswap_limit;
    const RRDVAR_ACQUIRED *chart_var_memoryswap_limit;

    const DICTIONARY_ITEM *cgroup_netdev_link;

    struct cgroup *next;
    struct cgroup *discovered_next;

};

struct discovery_thread {
    uv_thread_t thread;
    uv_mutex_t mutex;
    uv_cond_t cond_var;
    int exited;
};

extern struct discovery_thread discovery_thread;

extern const char *cgroups_rename_script;
extern char cgroup_chart_id_prefix[];
extern char services_chart_id_prefix[];
extern uv_mutex_t cgroup_root_mutex;

void cgroup_discovery_worker(void *ptr);

extern bool is_inside_k8s;
extern long system_page_size;

extern int cgroup_use_unified_cgroups;
extern bool cgroup_unified_exist;

extern bool cgroup_enable_cpuacct;
extern bool cgroup_enable_memory;
extern bool cgroup_enable_blkio;
extern bool cgroup_enable_pressure;
extern bool cgroup_enable_cpuacct_cpu_shares;

extern int cgroup_check_for_new_every;
extern int cgroup_update_every;

extern char *cgroup_cpuacct_base;
extern char *cgroup_cpuset_base;
extern char *cgroup_blkio_base;
extern char *cgroup_memory_base;
extern char *cgroup_pids_base;
extern char *cgroup_unified_base;

extern int cgroup_root_count;
extern int cgroup_root_max;
extern int cgroup_max_depth;

extern SIMPLE_PATTERN *enabled_cgroup_paths;
extern SIMPLE_PATTERN *enabled_cgroup_names;
extern SIMPLE_PATTERN *search_cgroup_paths;
extern SIMPLE_PATTERN *enabled_cgroup_renames;
extern SIMPLE_PATTERN *systemd_services_cgroups;
extern SIMPLE_PATTERN *entrypoint_parent_process_comm;

extern const char *cgroups_network_interface_script;

extern int cgroups_check;

extern uint32_t Read_hash;
extern uint32_t Write_hash;
extern uint32_t user_hash;
extern uint32_t system_hash;
extern uint32_t user_usec_hash;
extern uint32_t system_usec_hash;
extern uint32_t nr_periods_hash;
extern uint32_t nr_throttled_hash;
extern uint32_t throttled_time_hash;
extern uint32_t throttled_usec_hash;

extern struct cgroup *cgroup_root;

enum cgroups_type { CGROUPS_AUTODETECT_FAIL, CGROUPS_V1, CGROUPS_V2 };

enum cgroups_systemd_setting {
    SYSTEMD_CGROUP_ERR,
    SYSTEMD_CGROUP_LEGACY,
    SYSTEMD_CGROUP_HYBRID,
    SYSTEMD_CGROUP_UNIFIED
};

struct cgroups_systemd_config_setting {
    char *name;
    enum cgroups_systemd_setting setting;
};

extern struct cgroups_systemd_config_setting cgroups_systemd_options[];

static inline int matches_enabled_cgroup_paths(char *id) {
    return simple_pattern_matches(enabled_cgroup_paths, id);
}

static inline int matches_enabled_cgroup_names(char *name) {
    return simple_pattern_matches(enabled_cgroup_names, name);
}

static inline int matches_enabled_cgroup_renames(char *id) {
    return simple_pattern_matches(enabled_cgroup_renames, id);
}

static inline int matches_systemd_services_cgroups(char *id) {
    return simple_pattern_matches(systemd_services_cgroups, id);
}

static inline int matches_search_cgroup_paths(const char *dir) {
    return simple_pattern_matches(search_cgroup_paths, dir);
}

static inline int matches_entrypoint_parent_process_comm(const char *comm) {
    return simple_pattern_matches(entrypoint_parent_process_comm, comm);
}

static inline int is_cgroup_systemd_service(struct cgroup *cg) {
    return (int)(cg->options & CGROUP_OPTIONS_SYSTEM_SLICE_SERVICE);
}

static inline int k8s_is_kubepod(struct cgroup *cg) {
    return cg->container_orchestrator == CGROUPS_ORCHESTRATOR_K8S;
}

static inline char *cgroup_chart_type(char *buffer, struct cgroup *cg) {
    buffer[0] = '\0';

    if (cg->chart_id[0] == '\0' || (cg->chart_id[0] == '/' && cg->chart_id[1] == '\0'))
        strncpy(buffer, "cgroup_root", RRD_ID_LENGTH_MAX);
    else if (is_cgroup_systemd_service(cg))
        snprintfz(buffer, RRD_ID_LENGTH_MAX, "%s%s", services_chart_id_prefix, cg->chart_id);
    else
        snprintfz(buffer, RRD_ID_LENGTH_MAX, "%s%s", cgroup_chart_id_prefix, cg->chart_id);

    return buffer;
}

#define RRDFUNCTIONS_CGTOP_HELP "View running containers"
#define RRDFUNCTIONS_SYSTEMD_SERVICES_HELP "View systemd services"

int cgroup_function_cgroup_top(BUFFER *wb, const char *function, BUFFER *payload, const char *source);
int cgroup_function_systemd_top(BUFFER *wb, const char *function, BUFFER *payload, const char *source);

void cgroup_netdev_link_init(void);
const DICTIONARY_ITEM *cgroup_netdev_get(struct cgroup *cg);
void cgroup_netdev_delete(struct cgroup *cg);

void update_cpu_utilization_chart(struct cgroup *cg);
void update_cpu_utilization_limit_chart(struct cgroup *cg, NETDATA_DOUBLE cpu_limit);
void update_cpu_throttled_chart(struct cgroup *cg);
void update_cpu_throttled_duration_chart(struct cgroup *cg);
void update_cpu_shares_chart(struct cgroup *cg);
void update_cpu_per_core_usage_chart(struct cgroup *cg);

void update_mem_usage_limit_chart(struct cgroup *cg, unsigned long long memory_limit);
void update_mem_utilization_chart(struct cgroup *cg, unsigned long long memory_limit);
void update_mem_usage_detailed_chart(struct cgroup *cg);
void update_mem_writeback_chart(struct cgroup *cg);
void update_mem_activity_chart(struct cgroup *cg);
void update_mem_pgfaults_chart(struct cgroup *cg);
void update_mem_failcnt_chart(struct cgroup *cg);
void update_mem_usage_chart(struct cgroup *cg);

void update_io_serviced_bytes_chart(struct cgroup *cg);
void update_io_serviced_ops_chart(struct cgroup *cg);
void update_throttle_io_serviced_bytes_chart(struct cgroup *cg);
void update_throttle_io_serviced_ops_chart(struct cgroup *cg);
void update_io_queued_ops_chart(struct cgroup *cg);
void update_io_merged_ops_chart(struct cgroup *cg);

void update_pids_current_chart(struct cgroup *cg);

void update_cpu_some_pressure_chart(struct cgroup *cg);
void update_cpu_some_pressure_stall_time_chart(struct cgroup *cg);
void update_cpu_full_pressure_chart(struct cgroup *cg);
void update_cpu_full_pressure_stall_time_chart(struct cgroup *cg);

void update_mem_some_pressure_chart(struct cgroup *cg);
void update_mem_some_pressure_stall_time_chart(struct cgroup *cg);
void update_mem_full_pressure_chart(struct cgroup *cg);
void update_mem_full_pressure_stall_time_chart(struct cgroup *cg);

void update_irq_some_pressure_chart(struct cgroup *cg);
void update_irq_some_pressure_stall_time_chart(struct cgroup *cg);
void update_irq_full_pressure_chart(struct cgroup *cg);
void update_irq_full_pressure_stall_time_chart(struct cgroup *cg);

void update_io_some_pressure_chart(struct cgroup *cg);
void update_io_some_pressure_stall_time_chart(struct cgroup *cg);
void update_io_full_pressure_chart(struct cgroup *cg);
void update_io_full_pressure_stall_time_chart(struct cgroup *cg);

#endif // NETDATA_CGROUP_INTERNALS_H