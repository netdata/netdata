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
#include "daemon/main.h"

#include "ebpf_apps.h"

typedef enum {
    MODE_RETURN = 0, // This attaches kprobe when the function returns
    MODE_DEVMODE,    // This stores log given description about the errors raised
    MODE_ENTRY       // This attaches kprobe when the function is called
} netdata_run_mode_t;

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

typedef struct ebpf_module {
    const char *thread_name;
    const char *config_name;
    int enabled;
    void *(*start_routine)(void *);
    int update_time;
    int global_charts;
    int apps_charts;
    netdata_run_mode_t mode;
    netdata_ebpf_events_t *probes;
    uint32_t thread_id;
    int optional;
} ebpf_module_t;

extern ebpf_module_t ebpf_modules[];
#define EBPF_MODULE_PROCESS_IDX 0
#define EBPF_MODULE_SOCKET_IDX 1

// Copied from musl header
#ifndef offsetof
#if __GNUC__ > 3
#define offsetof(type, member) __builtin_offsetof(type, member)
#else
#define offsetof(type, member) ((size_t)((char *)&(((type *)0)->member) - (char *)0))
#endif
#endif

// Chart defintions
#define NETDATA_EBPF_FAMILY "ebpf"

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

// Threads
extern void *ebpf_process_thread(void *ptr);
extern void *ebpf_socket_thread(void *ptr);

// Common variables
extern pthread_mutex_t lock;
extern int close_ebpf_plugin;
extern int ebpf_nprocs;
extern int running_on_kernel;
extern char *ebpf_plugin_dir;
extern char kernel_string[64];
extern netdata_ebpf_events_t process_probes[];
extern netdata_ebpf_events_t socket_probes[];

extern pthread_mutex_t collect_data_mutex;
extern pthread_cond_t collect_data_cond_var;

// Common functions
extern void ebpf_global_labels(netdata_syscall_stat_t *is,
                               netdata_publish_syscall_t *pio,
                               char **dim,
                               char **name,
                               int end);

extern void ebpf_write_chart_cmd(char *type,
                                 char *id,
                                 char *title,
                                 char *units,
                                 char *family,
                                 char *charttype,
                                 int order);

extern void ebpf_write_global_dimension(char *n, char *d);

extern void ebpf_create_global_dimension(void *ptr, int end);

extern void ebpf_create_chart(char *type,
                              char *id,
                              char *title,
                              char *units,
                              char *family,
                              int order,
                              void (*ncd)(void *, int),
                              void *move,
                              int end);

extern void write_begin_chart(char *family, char *name);

extern void write_chart_dimension(char *dim, long long value);

extern void write_count_chart(char *name, char *family, netdata_publish_syscall_t *move, uint32_t end);

extern void write_err_chart(char *name, char *family, netdata_publish_syscall_t *move, int end);

extern void write_io_chart(char *chart, char *family, char *dwrite, char *dread, netdata_publish_vfs_common_t *pvc);

extern void fill_ebpf_data(ebpf_data_t *ef);

extern void ebpf_create_charts_on_apps(char *name,
                                       char *title,
                                       char *units,
                                       char *family,
                                       int order,
                                       struct target *root);

extern void write_end_chart();

#define EBPF_GLOBAL_SECTION "global"
#define EBPF_PROGRAMS_SECTION "ebpf programs"
#define EBPF_NETWORK_VIEWER_SECTION "network viewer"
#define EBPF_SERVICE_NAME_SECTION "service name"

#define EBPF_COMMON_DIMENSION_CALL "calls"
#define EBPF_COMMON_DIMENSION_BYTESS "bytes/s"
#define EBPF_COMMON_DIMENSION_DIFFERENCE "difference"
#define EBPF_COMMON_DIMENSION_PACKETS "packets"

// Common variables
extern char *ebpf_user_config_dir;
extern char *ebpf_stock_config_dir;
extern pid_t *pid_index;
extern int debug_enabled;

// Socket functions and variables
// Common functions
extern void ebpf_socket_create_apps_charts(ebpf_module_t *em, struct target *root);
extern collected_number get_value_from_structure(char *basis, size_t offset);
extern struct pid_stat *root_of_pids;
extern ebpf_process_stat_t *global_process_stat;
extern size_t all_pids_count;
extern int update_every;

#define EBPF_MAX_SYNCHRONIZATION_TIME 300

// External functions
extern void change_socket_event();
extern void change_process_event();

#endif /* NETDATA_COLLECTOR_EBPF_H */
