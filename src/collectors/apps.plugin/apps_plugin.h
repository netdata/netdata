// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_APPS_PLUGIN_H
#define NETDATA_APPS_PLUGIN_H

#include "collectors/all.h"
#include "libnetdata/libnetdata.h"

#define OS_FUNC_CONCAT(a, b) a##b

#if defined(OS_FREEBSD)
#include <sys/user.h>

#define OS_INIT_PID                          1
#define ALL_PIDS_ARE_READ_INSTANTLY          1
#define PROCESSES_HAVE_CPU_GUEST_TIME        0
#define PROCESSES_HAVE_CPU_CHILDREN_TIME     1
#define PROCESSES_HAVE_VOLCTX                0
#define PROCESSES_HAVE_NVOLCTX               0
#define PROCESSES_HAVE_PHYSICAL_IO           0
#define PROCESSES_HAVE_LOGICAL_IO            1
#define PROCESSES_HAVE_IO_CALLS              0
#define PROCESSES_HAVE_UID                   1
#define PROCESSES_HAVE_GID                   1
#define PROCESSES_HAVE_SID                   0
#define PROCESSES_HAVE_MAJFLT                1
#define PROCESSES_HAVE_CHILDREN_FLTS         1
#define PROCESSES_HAVE_VMSWAP                0
#define PROCESSES_HAVE_VMSHARED              0
#define PROCESSES_HAVE_RSSFILE               0
#define PROCESSES_HAVE_RSSSHMEM              0
#define PROCESSES_HAVE_FDS                   1
#define PROCESSES_HAVE_HANDLES               0
#define PROCESSES_HAVE_CMDLINE               1
#define PROCESSES_HAVE_PID_LIMITS            0
#define PROCESSES_HAVE_COMM_AND_NAME         0
#define PROCESSES_HAVE_STATE                 0
#define PPID_SHOULD_BE_RUNNING               1
#define INCREMENTAL_DATA_COLLECTION          1
#define CPU_TO_NANOSECONDCORES (1)
#define OS_FUNCTION(func) OS_FUNC_CONCAT(func, _freebsd)

#elif defined(OS_MACOS)
#include <mach/mach.h>
#include <mach/mach_host.h>
#include <libproc.h>
#include <sys/proc_info.h>
#include <sys/sysctl.h>
#include <mach/mach_time.h> // For mach_timebase_info_data_t and mach_timebase_info
#include <AvailabilityMacros.h>

struct pid_info {
    struct kinfo_proc proc;
    struct proc_taskinfo taskinfo;
    struct proc_bsdinfo bsdinfo;
#if MAC_OS_X_VERSION_MIN_REQUIRED >= 110000
    struct rusage_info_v4 rusageinfo;
#endif
};

#define OS_INIT_PID                          1
#define ALL_PIDS_ARE_READ_INSTANTLY          1
#define PROCESSES_HAVE_CPU_GUEST_TIME        0
#define PROCESSES_HAVE_CPU_CHILDREN_TIME     0
#define PROCESSES_HAVE_VOLCTX                1
#define PROCESSES_HAVE_NVOLCTX               0
#define PROCESSES_HAVE_PHYSICAL_IO           0
#define PROCESSES_HAVE_LOGICAL_IO            1
#define PROCESSES_HAVE_IO_CALLS              0
#define PROCESSES_HAVE_UID                   1
#define PROCESSES_HAVE_GID                   1
#define PROCESSES_HAVE_SID                   0
#define PROCESSES_HAVE_MAJFLT                1
#define PROCESSES_HAVE_CHILDREN_FLTS         0
#define PROCESSES_HAVE_VMSWAP                0
#define PROCESSES_HAVE_VMSHARED              0
#define PROCESSES_HAVE_RSSFILE               0
#define PROCESSES_HAVE_RSSSHMEM              0
#define PROCESSES_HAVE_SMAPS_ROLLUP          0
#define PROCESSES_HAVE_FDS                   1
#define PROCESSES_HAVE_HANDLES               0
#define PROCESSES_HAVE_CMDLINE               1
#define PROCESSES_HAVE_PID_LIMITS            0
#define PROCESSES_HAVE_COMM_AND_NAME         0
#define PROCESSES_HAVE_STATE                 0
#define PPID_SHOULD_BE_RUNNING               1
#define INCREMENTAL_DATA_COLLECTION          1
#define CPU_TO_NANOSECONDCORES (1) // already in nanoseconds
#define OS_FUNCTION(func) OS_FUNC_CONCAT(func, _macos)

#elif defined(OS_WINDOWS)
#define OS_INIT_PID                          0 // dynamic, is set during data collection
#define ALL_PIDS_ARE_READ_INSTANTLY          1
#define PROCESSES_HAVE_CPU_GUEST_TIME        0
#define PROCESSES_HAVE_CPU_CHILDREN_TIME     0
#define PROCESSES_HAVE_VOLCTX                0
#define PROCESSES_HAVE_NVOLCTX               0
#define PROCESSES_HAVE_PHYSICAL_IO           0
#define PROCESSES_HAVE_LOGICAL_IO            1
#define PROCESSES_HAVE_IO_CALLS              1
#define PROCESSES_HAVE_UID                   0
#define PROCESSES_HAVE_GID                   0
#define PROCESSES_HAVE_SID                   1
#define PROCESSES_HAVE_MAJFLT                0
#define PROCESSES_HAVE_CHILDREN_FLTS         0
#define PROCESSES_HAVE_VMSWAP                1
#define PROCESSES_HAVE_VMSHARED              0
#define PROCESSES_HAVE_RSSFILE               0
#define PROCESSES_HAVE_RSSSHMEM              0
#define PROCESSES_HAVE_SMAPS_ROLLUP          0
#define PROCESSES_HAVE_FDS                   0
#define PROCESSES_HAVE_HANDLES               1
#define PROCESSES_HAVE_CMDLINE               0
#define PROCESSES_HAVE_PID_LIMITS            0
#define PROCESSES_HAVE_COMM_AND_NAME         1
#define PROCESSES_HAVE_STATE                 0
#define PPID_SHOULD_BE_RUNNING               0
#define INCREMENTAL_DATA_COLLECTION          0
#define CPU_TO_NANOSECONDCORES (100) // convert 100ns to ns
#define OS_FUNCTION(func) OS_FUNC_CONCAT(func, _windows)

#elif defined(OS_LINUX)
#define OS_INIT_PID                          1
#define ALL_PIDS_ARE_READ_INSTANTLY          0
#define PROCESSES_HAVE_CPU_GUEST_TIME        1
#define PROCESSES_HAVE_CPU_CHILDREN_TIME     1
#define PROCESSES_HAVE_VOLCTX                1
#define PROCESSES_HAVE_NVOLCTX               1
#define PROCESSES_HAVE_PHYSICAL_IO           1
#define PROCESSES_HAVE_LOGICAL_IO            1
#define PROCESSES_HAVE_IO_CALLS              1
#define PROCESSES_HAVE_UID                   1
#define PROCESSES_HAVE_GID                   1
#define PROCESSES_HAVE_SID                   0
#define PROCESSES_HAVE_MAJFLT                1
#define PROCESSES_HAVE_CHILDREN_FLTS         1
#define PROCESSES_HAVE_VMSWAP                1
#define PROCESSES_HAVE_VMSHARED              1
#define PROCESSES_HAVE_RSSFILE               1
#define PROCESSES_HAVE_RSSSHMEM              1
#define PROCESSES_HAVE_SMAPS_ROLLUP          1
#define PROCESSES_HAVE_FDS                   1
#define PROCESSES_HAVE_HANDLES               0
#define PROCESSES_HAVE_CMDLINE               1
#define PROCESSES_HAVE_PID_LIMITS            1
#define PROCESSES_HAVE_COMM_AND_NAME         0
#define PROCESSES_HAVE_STATE                 1
#define PPID_SHOULD_BE_RUNNING               1
#define USE_APPS_GROUPS_CONF                 1
#define INCREMENTAL_DATA_COLLECTION          1
#define CPU_TO_NANOSECONDCORES (NSEC_PER_SEC / system_hz)
#define OS_FUNCTION(func) OS_FUNC_CONCAT(func, _linux)

extern int max_fds_cache_seconds;

#else
#error "Unsupported operating system"
#endif

#if (PROCESSES_HAVE_UID == 1) && (PROCESSES_HAVE_SID == 1)
#error "Do not enable SID and UID at the same time"
#endif

// --------------------------------------------------------------------------------------------------------------------

#define MAX_SYSTEM_FD_TO_ALLOW_FILES_PROCESSING 100000

extern pid_t INIT_PID;

extern bool debug_enabled;

extern bool enable_detailed_uptime_charts;
extern bool enable_users_charts;
extern bool enable_groups_charts;
extern bool include_exited_childs;
extern bool enable_function_cmdline;
extern bool proc_pid_cmdline_is_needed;
extern int enable_file_charts;
extern bool obsolete_file_charts;

extern size_t
    global_iterations_counter,
    calls_counter,
    file_counter,
    filenames_allocated_counter,
    inodes_changed_counter,
    links_changed_counter,
    targets_assignment_counter,
    apps_groups_targets_count;

#if (PROCESSES_HAVE_CPU_GUEST_TIME == 1)
extern bool enable_guest_charts;
extern bool show_guest_time;
#endif

#if (ALL_PIDS_ARE_READ_INSTANTLY == 0)
extern kernel_uint_t
    global_utime,
    global_stime,
    global_gtime;
#endif

// the normalization ratios, as calculated by normalize_utilization()
extern NETDATA_DOUBLE
    utime_fix_ratio,
    stime_fix_ratio,
    gtime_fix_ratio,
    minflt_fix_ratio,
    majflt_fix_ratio,
    cutime_fix_ratio,
    cstime_fix_ratio,
    cgtime_fix_ratio,
    cminflt_fix_ratio,
    cmajflt_fix_ratio;

extern size_t pagesize;

extern netdata_mutex_t apps_and_stdout_mutex;

// --------------------------------------------------------------------------------------------------------------------
// string lengths

#define MAX_CMDLINE 65536

// --------------------------------------------------------------------------------------------------------------------
// to avoid reallocating too frequently when we add file descriptors,
// we double the allocation at every increase request.

static inline uint32_t fds_new_size(uint32_t old_size, uint32_t new_fd) {
    return MAX(old_size * 2, new_fd + 1); // 1 space always
}

// --------------------------------------------------------------------------------------------------------------------
// some variables for keeping track of processes count by states
#if (PROCESSES_HAVE_STATE == 1)
typedef enum {
    PROC_STATUS_RUNNING = 0,
    PROC_STATUS_SLEEPING_D, // uninterruptible sleep
    PROC_STATUS_SLEEPING,   // interruptible sleep
    PROC_STATUS_ZOMBIE,
    PROC_STATUS_STOPPED,
    PROC_STATUS_END, //place holder for ending enum fields
} proc_state;

extern proc_state proc_state_count[PROC_STATUS_END];
extern const char *proc_states[];
#endif

// --------------------------------------------------------------------------------------------------------------------
// the rates we are going to send to netdata will have this detail a value of:
//  - 1 will send just integer parts to netdata
//  - 100 will send 2 decimal points
//  - 1000 will send 3 decimal points
// etc.
#define RATES_DETAIL 10000ULL

#if (PROCESSES_HAVE_FDS == 1)
struct openfds {
    kernel_uint_t files;
    kernel_uint_t pipes;
    kernel_uint_t sockets;
    kernel_uint_t inotifies;
    kernel_uint_t eventfds;
    kernel_uint_t timerfds;
    kernel_uint_t signalfds;
    kernel_uint_t eventpolls;
    kernel_uint_t other;
};
#define pid_openfds_sum(p) ((p)->openfds.files + (p)->openfds.pipes + (p)->openfds.sockets + (p)->openfds.inotifies + (p)->openfds.eventfds + (p)->openfds.timerfds + (p)->openfds.signalfds + (p)->openfds.eventpolls + (p)->openfds.other)
#endif

// --------------------------------------------------------------------------------------------------------------------
// target
//
// target is the structure that processes are aggregated to be reported
// to netdata.
//
// - Each entry in /etc/apps_groups.conf creates a target.
// - Each user and group used by a process in the system, creates a target.

struct pid_on_target {
    int32_t pid;
    struct pid_on_target *next;
};

typedef enum __attribute__((packed)) {
    TARGET_TYPE_APP_GROUP = 1,
#if (PROCESSES_HAVE_UID == 1)
    TARGET_TYPE_UID,
#endif
#if (PROCESSES_HAVE_GID == 1)
    TARGET_TYPE_GID,
#endif
#if (PROCESSES_HAVE_SID == 1)
    TARGET_TYPE_SID,
#endif
    TARGET_TYPE_TREE,
} TARGET_TYPE;

typedef enum __attribute__((packed)) {
    // CPU utilization time
    // The values are expressed in "NANOSECONDCORES".
    // 1 x "NANOSECONDCORE" = 1 x NSEC_PER_SEC (1 billion).
    PDF_UTIME,      // CPU user time
    PDF_STIME,      // CPU system time
#if (PROCESSES_HAVE_CPU_GUEST_TIME == 1)
    PDF_GTIME,      // CPU guest time
#endif
#if (PROCESSES_HAVE_CPU_CHILDREN_TIME == 1)
    PDF_CUTIME,     // exited children CPU user time
    PDF_CSTIME,     // exited children CPU system time
#if (PROCESSES_HAVE_CPU_GUEST_TIME == 1)
    PDF_CGTIME,     // exited children CPU guest time
#endif
#endif

    PDF_MINFLT,     // rate, unit: faults * RATES_DETAIL

#if (PROCESSES_HAVE_MAJFLT == 1)
    PDF_MAJFLT,     // rate, unit: faults * RATES_DETAIL
#endif

#if (PROCESSES_HAVE_CHILDREN_FLTS == 1)
    PDF_CMINFLT,    // rate, unit: faults * RATES_DETAIL
    PDF_CMAJFLT,    // rate, unit: faults * RATES_DETAIL
#endif

    PDF_VMSIZE,     // the current virtual memory used by the process, in bytes
    PDF_VMRSS,      // the resident memory used by the process, in bytes

#if (PROCESSES_HAVE_VMSHARED == 1)
    PDF_VMSHARED,   // the shared memory used by the process, in bytes
#endif

#if (PROCESSES_HAVE_SMAPS_ROLLUP == 1)
    PDF_MEM_ESTIMATED, // estimated memory usage using smaps ratios, in bytes
    PDF_PSS,        // proportional set size, in bytes
#endif

#if (PROCESSES_HAVE_RSSFILE == 1)
    PDF_RSSFILE,    // unit: bytes
#endif

#if (PROCESSES_HAVE_RSSSHMEM == 1)
    PDF_RSSSHMEM,   // unit: bytes
#endif

#if (PROCESSES_HAVE_VMSWAP == 1)
    PDF_VMSWAP,     // the swap memory used by the process, in bytes
#endif

#if (PROCESSES_HAVE_VOLCTX == 1)
    PDF_VOLCTX,     // rate, unit: switches * RATES_DETAIL
#endif

#if (PROCESSES_HAVE_NVOLCTX == 1)
    PDF_NVOLCTX,    // rate, unit: switches * RATES_DETAIL
#endif

#if (PROCESSES_HAVE_LOGICAL_IO == 1)
    PDF_LREAD,      // rate, logical reads in bytes/sec * RATES_DETAIL
    PDF_LWRITE,     // rate, logical writes in bytes/sec * RATES_DETAIL
#endif

#if (PROCESSES_HAVE_PHYSICAL_IO == 1)
    PDF_PREAD,      // rate, physical reads in bytes/sec * RATES_DETAIL
    PDF_PWRITE,     // rate, physical writes in bytes/sec * RATES_DETAIL
#endif

#if (PROCESSES_HAVE_IO_CALLS == 1)
    PDF_OREAD,      // rate, read ops/sec * RATES_DETAIL
    PDF_OWRITE,     // rate, write ops/sec * RATES_DETAIL
#endif

    PDF_UPTIME,     // the process uptime in seconds
    PDF_THREADS,    // the number of threads
    PDF_PROCESSES,  // the number of processes

#if (PROCESSES_HAVE_HANDLES == 1)
    PDF_HANDLES,    // the number of handles the process maintains
#endif

    // terminator
    PDF_MAX
} PID_FIELD;

typedef struct apps_match {
    bool starts_with:1;
    bool ends_with:1;
    STRING *compare;
    SIMPLE_PATTERN *pattern;
} APPS_MATCH;

struct target {
    STRING *id;
    STRING *name;
    STRING *clean_name;

    TARGET_TYPE type;
    APPS_MATCH match;
#if (PROCESSES_HAVE_UID == 1)
    uid_t uid;
#endif
#if (PROCESSES_HAVE_GID == 1)
    gid_t gid;
#endif
#if (PROCESSES_HAVE_SID == 1)
    STRING *sid_name;
#endif

    kernel_uint_t values[PDF_MAX];

    kernel_uint_t uptime_min;
    kernel_uint_t uptime_max;

#if (PROCESSES_HAVE_FDS == 1)
    struct openfds openfds;
    NETDATA_DOUBLE max_open_files_percent;
    int *target_fds;
    uint32_t target_fds_size;
#endif

    bool exposed:1;         // if set, we have sent this to netdata
    bool needs_smaps_update:1;

    struct pid_on_target *root_pid; // list of aggregated pids for target debugging

    struct target *target;  // the one that will be reported to netdata
    struct target *next;
};

// --------------------------------------------------------------------------------------------------------------------
// internal flags
// handled in code (automatically set)

// log each problem once per process
// log flood protection flags (log_thrown)
typedef enum __attribute__((packed)) {
    PID_LOG_IO              = (1 << 0),
    PID_LOG_STATUS          = (1 << 1),
    PID_LOG_CMDLINE         = (1 << 2),
    PID_LOG_FDS             = (1 << 3),
    PID_LOG_STAT            = (1 << 4),
    PID_LOG_LIMITS          = (1 << 5),
    PID_LOG_LIMITS_DETAIL   = (1 << 6),
#if (PROCESSES_HAVE_SMAPS_ROLLUP == 1)
    PID_LOG_SMAPS           = (1 << 7),
#endif
} PID_LOG;

// --------------------------------------------------------------------------------------------------------------------
// pid_stat
//
// structure to store data for each process running
// see: man proc for the description of the fields

#if (PROCESSES_HAVE_PID_LIMITS == 1)
struct pid_limits {
    //    kernel_uint_t max_cpu_time;
    //    kernel_uint_t max_file_size;
    //    kernel_uint_t max_data_size;
    //    kernel_uint_t max_stack_size;
    //    kernel_uint_t max_core_file_size;
    //    kernel_uint_t max_resident_set;
    //    kernel_uint_t max_processes;
    kernel_uint_t max_open_files;
    //    kernel_uint_t max_locked_memory;
    //    kernel_uint_t max_address_space;
    //    kernel_uint_t max_file_locks;
    //    kernel_uint_t max_pending_signals;
    //    kernel_uint_t max_msgqueue_size;
    //    kernel_uint_t max_nice_priority;
    //    kernel_uint_t max_realtime_priority;
    //    kernel_uint_t max_realtime_timeout;
};
#endif

struct pid_fd {
    int fd;

#if defined(OS_LINUX)
    ino_t inode;
    char *filename;
    uint32_t link_hash;
    size_t cache_iterations_counter;
    size_t cache_iterations_reset;
#endif
};

#define pid_stat_comm(p) (string2str((p)->comm))
#define pid_stat_cmdline(p) (string2str((p)->cmdline))
uint32_t all_files_len_get(void);

struct pid_stat {
    int32_t pid;
    int32_t ppid;
    // int32_t pgrp;
    // int32_t session;
    // int32_t tty_nr;
    // int32_t tpgid;
    // uint64_t flags;

    struct pid_stat *parent;
    struct pid_stat *next;
    struct pid_stat *prev;

    struct target *target;          // app_groups.conf/tree targets
    struct target *prev_target;

#if (PROCESSES_HAVE_UID == 1)
    struct target *uid_target;      // uid based targets
#endif
#if (PROCESSES_HAVE_GID == 1)
    struct target *gid_target;      // gid based targets
#endif
#if (PROCESSES_HAVE_SID == 1)
    struct target *sid_target;      // sid based targets
#endif

    STRING *comm_orig;              // the command, as-collected
    STRING *comm;                   // the command, sanitized
    STRING *name;                   // the command name, if any, sanitized
    STRING *cmdline;                // the full command line of the program

#if defined(OS_WINDOWS)
    COUNTER_DATA perflib[PDF_MAX];
#else
    kernel_uint_t raw[PDF_MAX];
#endif

    kernel_uint_t values[PDF_MAX];

#if (PROCESSES_HAVE_UID == 1)
    uid_t uid;
#endif
#if (PROCESSES_HAVE_GID == 1)
    gid_t gid;
#endif
#if (PROCESSES_HAVE_SID == 1)
    STRING *sid_name;
#endif

#if (ALL_PIDS_ARE_READ_INSTANTLY == 0)
    uint32_t sortlist;  // higher numbers = top on the process tree
                        // each process gets a unique number (non-sequential though)
#endif

#if (PROCESSES_HAVE_FDS == 1)
    struct openfds openfds;
#if (PROCESSES_HAVE_PID_LIMITS == 1)
    struct pid_limits limits;
    NETDATA_DOUBLE openfds_limits_percent;
#endif
    struct pid_fd *fds;             // array of fds it uses
    uint32_t fds_size;              // the size of the fds array
#endif

    uint32_t children_count;        // the number of processes directly referencing this.
                                    // used internally for apps_groups.conf inheritance.
                                    // don't rely on it for anything else.

    uint32_t keeploops;             // increases by 1 every time keep is 1 and updated 0

    PID_LOG log_thrown;

    bool read:1;                    // true when we have already read this process for this iteration
    bool updated:1;                 // true when the process is currently running
    bool merged:1;                  // true when it has been merged to its parent
    bool keep:1;                    // true when we need to keep this process in memory even after it exited
    bool is_manager:1;              // true when this pid is a process manager
    bool is_aggregator:1;           // true when this pid is a process aggregator

    bool matched_by_config:1;

#if (PROCESSES_HAVE_STATE == 1)
    char state;
#endif

#if defined(OS_WINDOWS)
    bool got_info:1;
    bool got_service:1;
    bool initialized:1;
#endif

    usec_t stat_collected_usec;
    usec_t last_stat_collected_usec;

    usec_t io_collected_usec;
    usec_t last_io_collected_usec;
    usec_t last_limits_collected_usec;

#if defined(OS_LINUX)
    ARL_BASE *status_arl;
    char *fds_dirname;              // the full directory name in /proc/PID/fd
    char *stat_filename;
    char *status_filename;
    char *io_filename;
    char *cmdline_filename;
    char *limits_filename;
#if (PROCESSES_HAVE_SMAPS_ROLLUP == 1)
    char *smaps_rollup_filename;
    ARL_BASE *smaps_rollup_arl;
    kernel_uint_t vmshared_delta;
    NETDATA_DOUBLE pss_total_ratio;
    size_t last_pss_iteration;
    kernel_uint_t pss_bytes;
#endif
#endif
};

// --------------------------------------------------------------------------------------------------------------------

extern int update_every;

// --------------------------------------------------------------------------------------------------------------------
// debugging

static inline void debug_log_int(const char *fmt, ... ) {
    va_list args;

    fprintf( stderr, "apps.plugin: ");
    va_start( args, fmt );
    vfprintf( stderr, fmt, args );
    va_end( args );

    fputc('\n', stderr);
}

#ifdef NETDATA_INTERNAL_CHECKS

#define debug_log(fmt, args...) do { if(unlikely(debug_enabled)) debug_log_int(fmt, ##args); } while(0)

#else

static inline void debug_log_dummy(void) {}
#define debug_log(fmt, args...) debug_log_dummy()

#endif
bool managed_log(struct pid_stat *p, PID_LOG log, bool status);
void sanitize_apps_plugin_chart_meta(char *buf);

// --------------------------------------------------------------------------------------------------------------------
// macro to calculate the incremental rate of a value
// each parameter is accessed only ONCE - so it is safe to pass function calls
// or other macros as parameters

#define incremental_rate(rate_variable, last_kernel_variable, new_kernel_value, collected_usec, last_collected_usec, multiplier) do { \
        kernel_uint_t _new_tmp = new_kernel_value; \
        (rate_variable) = (_new_tmp - (last_kernel_variable)) * (USEC_PER_SEC * (multiplier)) / ((collected_usec) - (last_collected_usec)); \
        (last_kernel_variable) = _new_tmp; \
    } while(0)

// the same macro for struct pid members
#define pid_incremental_rate(type, idx, value) \
    incremental_rate(p->values[idx], p->raw[idx], value, p->type##_collected_usec, p->last_##type##_collected_usec, RATES_DETAIL)

#define pid_incremental_cpu(type, idx, value) \
    incremental_rate(p->values[idx], p->raw[idx], value, p->type##_collected_usec, p->last_##type##_collected_usec, CPU_TO_NANOSECONDCORES)

void apps_managers_and_aggregators_init(void);
void apps_pids_init(void);

#if (PROCESSES_HAVE_CMDLINE == 1)
int read_proc_pid_cmdline(struct pid_stat *p);
#endif

#if (PROCESSES_HAVE_FDS == 1)
void clear_pid_fd(struct pid_fd *pfd);
void file_descriptor_not_used(int id);
void init_pid_fds(struct pid_stat *p, size_t first, size_t size);
void aggregate_pid_fds_on_targets(struct pid_stat *p);
int read_pid_file_descriptors(struct pid_stat *p, void *ptr);
void make_all_pid_fds_negative(struct pid_stat *p);
uint32_t file_descriptor_find_or_add(const char *name, uint32_t hash);
#endif

// --------------------------------------------------------------------------------------------------------------------
// data collection management

bool pid_match_check(struct pid_stat *p, APPS_MATCH *match);
APPS_MATCH pid_match_create(const char *comm);
void pid_match_cleanup(APPS_MATCH *m);

bool collect_data_for_all_pids(void);

void pid_collection_started(struct pid_stat *p);
void pid_collection_failed(struct pid_stat *p);
void pid_collection_completed(struct pid_stat *p);

#if (INCREMENTAL_DATA_COLLECTION == 1)
bool collect_parents_before_children(void);
int incrementally_collect_data_for_pid(pid_t pid, void *ptr);
int incrementally_collect_data_for_pid_stat(struct pid_stat *p, void *ptr);
#endif

// --------------------------------------------------------------------------------------------------------------------
// pid management

struct pid_stat *root_of_pids(void);
size_t all_pids_count(void);

struct pid_stat *get_or_allocate_pid_entry(pid_t pid);
struct pid_stat *find_pid_entry(pid_t pid);
void del_pid_entry(pid_t pid);
void update_pid_comm(struct pid_stat *p, const char *comm);
void update_pid_cmdline(struct pid_stat *p, const char *cmdline);

bool is_process_a_manager(struct pid_stat *p);
bool is_process_an_aggregator(struct pid_stat *p);
bool is_process_an_interpreter(struct pid_stat *p);

// --------------------------------------------------------------------------------------------------------------------
// targets management

struct target *find_target_by_name(struct target *base, const char *name);
struct target *get_tree_target(struct pid_stat *p);

void aggregate_processes_to_targets(void);

#if (PROCESSES_HAVE_UID == 1)
extern struct target *users_root_target;
struct target *get_uid_target(uid_t uid);
#endif

#if (PROCESSES_HAVE_GID == 1)
extern struct target *groups_root_target;
struct target *get_gid_target(gid_t gid);
#endif

#if (PROCESSES_HAVE_SID == 1)
extern struct target *sids_root_target;
struct target *get_sid_target(STRING *sid_name);
#endif

extern struct target *apps_groups_root_target;
int read_apps_groups_conf(const char *path, const char *file);

// --------------------------------------------------------------------------------------------------------------------
// output

void send_charts_updates_to_netdata(struct target *root, const char *type, const char *lbl_name, const char *title);
void send_collected_data_to_netdata(struct target *root, const char *type, usec_t dt);
void send_resource_usage_to_netdata(usec_t dt);

#if (PROCESSES_HAVE_STATE == 1)
void send_proc_states_count(usec_t dt);
#endif

#define APPS_PLUGIN_PROCESSES_FUNCTION_DESCRIPTION "Provides detailed process information including CPU usage, memory consumption, I/O statistics, file descriptors, page faults, and parent-child relationships (PPID) for all processes including those in containers."
void function_processes(const char *transaction, char *function,
                        usec_t *stop_monotonic_ut __maybe_unused, bool *cancelled __maybe_unused,
                        BUFFER *payload __maybe_unused, HTTP_ACCESS access,
                        const char *source __maybe_unused, void *data __maybe_unused);

// --------------------------------------------------------------------------------------------------------------------
// operating system functions

// one time initialization per operating system
void OS_FUNCTION(apps_os_init)(void);

// collect all the available information for all processes running
bool OS_FUNCTION(apps_os_collect_all_pids)(void);

bool OS_FUNCTION(apps_os_read_pid_status)(struct pid_stat *p, void *ptr);
bool OS_FUNCTION(apps_os_read_pid_stat)(struct pid_stat *p, void *ptr);
bool OS_FUNCTION(apps_os_read_pid_io)(struct pid_stat *p, void *ptr);

#if (PROCESSES_HAVE_PID_LIMITS == 1)
bool OS_FUNCTION(apps_os_read_pid_limits)(struct pid_stat *p, void *ptr);
#endif

#if (PROCESSES_HAVE_CMDLINE == 1)
bool OS_FUNCTION(apps_os_get_pid_cmdline)(struct pid_stat *p, char *cmdline, size_t bytes);
#endif

#if (PROCESSES_HAVE_FDS == 1)
bool OS_FUNCTION(apps_os_read_pid_fds)(struct pid_stat *p, void *ptr);
#endif

#if (PROCESSES_HAVE_SMAPS_ROLLUP == 1)
bool OS_FUNCTION(apps_os_read_pid_smaps_rollup)(struct pid_stat *p, void *ptr);
bool OS_FUNCTION(apps_os_have_smaps_rollup)(void);
static inline bool apps_os_have_smaps_rollup(void) {
    return OS_FUNCTION(apps_os_have_smaps_rollup)();
}
extern int pss_refresh_period;
#else
static inline bool OS_FUNCTION(apps_os_read_pid_smaps_rollup)(struct pid_stat *p __maybe_unused, void *ptr __maybe_unused) { return false; }
static inline bool OS_FUNCTION(apps_os_have_smaps_rollup)(void) { return false; }
static inline bool apps_os_have_smaps_rollup(void) { return false; }
#endif

#if (ALL_PIDS_ARE_READ_INSTANTLY == 0)
bool OS_FUNCTION(apps_os_read_global_cpu_utilization)(void);
#endif

// return the total physical memory of the system, in bytes
uint64_t OS_FUNCTION(apps_os_get_total_memory)(void);

#endif //NETDATA_APPS_PLUGIN_H
