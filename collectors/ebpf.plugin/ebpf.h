// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_COLLECTOR_EBPF_H
#define NETDATA_COLLECTOR_EBPF_H 1

#ifndef __FreeBSD__
#include <linux/perf_event.h>
#endif
#include <stdint.h>
#include <errno.h>
#include <signal.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <unistd.h>
#include <dlfcn.h>

#include <fcntl.h>
#include <ctype.h>
#include <dirent.h>

// From libnetdata.h
#include "libnetdata/threads/threads.h"
#include "libnetdata/locks/locks.h"
#include "libnetdata/avl/avl.h"
#include "libnetdata/clocks/clocks.h"
#include "libnetdata/config/appconfig.h"
#include "libnetdata/ebpf/ebpf.h"
#include "libnetdata/procfile/procfile.h"
#include "collectors/cgroups.plugin/sys_fs_cgroup.h"
#include "daemon/main.h"

#include "ebpf_apps.h"
#include "ebpf_cgroup.h"

#define NETDATA_EBPF_OLD_CONFIG_FILE "ebpf.conf"
#define NETDATA_EBPF_CONFIG_FILE "ebpf.d.conf"

typedef struct netdata_syscall_stat {
    unsigned long bytes;               // total number of bytes
    uint64_t call;                     // total number of calls
    uint64_t ecall;                    // number of calls that returned error
    struct netdata_syscall_stat *next; // Link list
} netdata_syscall_stat_t;

typedef uint64_t netdata_idx_t;

typedef struct netdata_publish_syscall {
    char *dimension;
    char *name;
    char *algorithm;
    unsigned long nbyte;
    unsigned long pbyte;
    uint64_t ncall;
    uint64_t pcall;
    uint64_t nerr;
    uint64_t perr;
    struct netdata_publish_syscall *next;
} netdata_publish_syscall_t;

typedef struct netdata_publish_vfs_common {
    long write;
    long read;

    long running;
    long zombie;
} netdata_publish_vfs_common_t;

typedef struct netdata_error_report {
    char comm[16];
    __u32 pid;

    int type;
    int err;
} netdata_error_report_t;

extern ebpf_module_t ebpf_modules[];
enum ebpf_main_index {
    EBPF_MODULE_PROCESS_IDX,
    EBPF_MODULE_SOCKET_IDX,
    EBPF_MODULE_CACHESTAT_IDX,
    EBPF_MODULE_SYNC_IDX,
    EBPF_MODULE_DCSTAT_IDX,
    EBPF_MODULE_SWAP_IDX,
    EBPF_MODULE_VFS_IDX,
    EBPF_MODULE_FILESYSTEM_IDX,
    EBPF_MODULE_DISK_IDX,
    EBPF_MODULE_MOUNT_IDX,
    EBPF_MODULE_FD_IDX,
    EBPF_MODULE_HARDIRQ_IDX,
    EBPF_MODULE_SOFTIRQ_IDX,
    EBPF_MODULE_OOMKILL_IDX,
    EBPF_MODULE_SHM_IDX,
    EBPF_MODULE_MDFLUSH_IDX,
    /* THREADS MUST BE INCLUDED BEFORE THIS COMMENT */
    EBPF_OPTION_ALL_CHARTS,
    EBPF_OPTION_VERSION,
    EBPF_OPTION_HELP,
    EBPF_OPTION_GLOBAL_CHART,
    EBPF_OPTION_RETURN_MODE,
    EBPF_OPTION_LEGACY,
    EBPF_OPTION_CORE
};

typedef struct ebpf_tracepoint {
    bool enabled;
    char *class;
    char *event;
} ebpf_tracepoint_t;

// Copied from musl header
#ifndef offsetof
#if __GNUC__ > 3
#define offsetof(type, member) __builtin_offsetof(type, member)
#else
#define offsetof(type, member) ((size_t)((char *)&(((type *)0)->member) - (char *)0))
#endif
#endif

// Chart definitions
#define NETDATA_EBPF_FAMILY "ebpf"
#define NETDATA_EBPF_IP_FAMILY "ip"
#define NETDATA_FILESYSTEM_FAMILY "filesystem"
#define NETDATA_EBPF_MOUNT_GLOBAL_FAMILY "mount_points"
#define NETDATA_EBPF_CHART_TYPE_LINE "line"
#define NETDATA_EBPF_CHART_TYPE_STACKED "stacked"
#define NETDATA_EBPF_MEMORY_GROUP "mem"
#define NETDATA_EBPF_SYSTEM_GROUP "system"
#define NETDATA_SYSTEM_SWAP_SUBMENU "swap"
#define NETDATA_SYSTEM_CGROUP_SWAP_SUBMENU "swap (eBPF)"
#define NETDATA_SYSTEM_IPC_SHM_SUBMENU "ipc shared memory"
#define NETDATA_MONITORING_FAMILY "netdata"

// Statistics charts
#define NETDATA_EBPF_THREADS "ebpf_threads"
#define NETDATA_EBPF_LOAD_METHOD "ebpf_load_methods"

// Log file
#define NETDATA_DEVELOPER_LOG_FILE "developer.log"

// Maximum number of processors monitored on perf events
#define NETDATA_MAX_PROCESSOR 512

// Kernel versions calculated with the formula:
// R = MAJOR*65536 + MINOR*256 + PATCH
#define NETDATA_KERNEL_V5_3 328448
#define NETDATA_KERNEL_V4_15 265984

#define EBPF_SYS_CLONE_IDX 11
#define EBPF_MAX_MAPS 32

#define EBPF_DEFAULT_UPDATE_EVERY 10

enum ebpf_algorithms_list {
    NETDATA_EBPF_ABSOLUTE_IDX,
    NETDATA_EBPF_INCREMENTAL_IDX
};

// Threads
extern void *ebpf_process_thread(void *ptr);
extern void *ebpf_socket_thread(void *ptr);

// Common variables
extern pthread_mutex_t lock;
extern int close_ebpf_plugin;
extern int ebpf_nprocs;
extern int running_on_kernel;
extern int isrh;
extern char *ebpf_plugin_dir;

extern pthread_mutex_t collect_data_mutex;
extern pthread_cond_t collect_data_cond_var;

// Common functions
extern void ebpf_global_labels(netdata_syscall_stat_t *is,
                               netdata_publish_syscall_t *pio,
                               char **dim,
                               char **name,
                               int *algorithm,
                               int end);

extern void ebpf_write_chart_cmd(char *type,
                                 char *id,
                                 char *title,
                                 char *units,
                                 char *family,
                                 char *charttype,
                                 char *context,
                                 int order,
                                 int update_every,
                                 char *module);

extern void ebpf_write_global_dimension(char *name, char *id, char *algorithm);

extern void ebpf_create_global_dimension(void *ptr, int end);

extern void ebpf_create_chart(char *type,
                              char *id,
                              char *title,
                              char *units,
                              char *family,
                              char *context,
                              char *charttype,
                              int order,
                              void (*ncd)(void *, int),
                              void *move,
                              int end,
                              int update_every,
                              char *module);

extern void write_begin_chart(char *family, char *name);

extern void write_chart_dimension(char *dim, long long value);

extern void write_count_chart(char *name, char *family, netdata_publish_syscall_t *move, uint32_t end);

extern void write_err_chart(char *name, char *family, netdata_publish_syscall_t *move, int end);

extern void write_io_chart(char *chart, char *family, char *dwrite, long long vwrite,
                           char *dread, long long vread);

extern void ebpf_create_charts_on_apps(char *name,
                                       char *title,
                                       char *units,
                                       char *family,
                                       char *charttype,
                                       int order,
                                       char *algorithm,
                                       struct target *root,
                                       int update_every,
                                       char *module);

extern void write_end_chart();

extern void ebpf_cleanup_publish_syscall(netdata_publish_syscall_t *nps);

extern int ebpf_enable_tracepoint(ebpf_tracepoint_t *tp);
extern int ebpf_disable_tracepoint(ebpf_tracepoint_t *tp);
extern uint32_t ebpf_enable_tracepoints(ebpf_tracepoint_t *tps);

extern void ebpf_pid_file(char *filename, size_t length);

#define EBPF_PROGRAMS_SECTION "ebpf programs"

#define EBPF_COMMON_DIMENSION_PERCENTAGE "%"
#define EBPF_COMMON_DIMENSION_CALL "calls/s"
#define EBPF_COMMON_DIMENSION_CONNECTIONS "connections/s"
#define EBPF_COMMON_DIMENSION_BITS "kilobits/s"
#define EBPF_COMMON_DIMENSION_BYTES "bytes/s"
#define EBPF_COMMON_DIMENSION_DIFFERENCE "difference"
#define EBPF_COMMON_DIMENSION_PACKETS "packets"
#define EBPF_COMMON_DIMENSION_FILES "files"
#define EBPF_COMMON_DIMENSION_MILLISECONDS "milliseconds"
#define EBPF_COMMON_DIMENSION_KILLS "kills"

// Common variables
extern int debug_enabled;
extern struct pid_stat *root_of_pids;
extern ebpf_cgroup_target_t *ebpf_cgroup_pids;
extern char *ebpf_algorithms[];
extern struct config collector_config;
extern ebpf_process_stat_t *global_process_stat;
extern netdata_ebpf_cgroup_shm_t shm_ebpf_cgroup;
extern int shm_fd_ebpf_cgroup;
extern sem_t *shm_sem_ebpf_cgroup;
extern pthread_mutex_t mutex_cgroup_shm;
extern size_t all_pids_count;
extern uint32_t finalized_threads;
extern ebpf_plugin_stats_t plugin_statistics;
extern struct btf *default_btf;

// Socket functions and variables
// Common functions
extern void ebpf_process_create_apps_charts(struct ebpf_module *em, void *ptr);
extern void ebpf_socket_create_apps_charts(struct ebpf_module *em, void *ptr);
extern void ebpf_cachestat_create_apps_charts(struct ebpf_module *em, void *root);
extern void ebpf_one_dimension_write_charts(char *family, char *chart, char *dim, long long v1);
extern collected_number get_value_from_structure(char *basis, size_t offset);
extern void ebpf_update_pid_table(ebpf_local_maps_t *pid, ebpf_module_t *em);
extern void ebpf_write_chart_obsolete(char *type, char *id, char *title, char *units, char *family,
                                      char *charttype, char *context, int order, int update_every);
extern void write_histogram_chart(char *family, char *name, const netdata_idx_t *hist, char **dimensions, uint32_t end);
void ebpf_update_disabled_plugin_stats(ebpf_module_t *em);

#define EBPF_MAX_SYNCHRONIZATION_TIME 300

#endif /* NETDATA_COLLECTOR_EBPF_H */
