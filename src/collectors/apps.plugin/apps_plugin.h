// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_APPS_PLUGIN_H
#define NETDATA_APPS_PLUGIN_H

#include "collectors/all.h"
#include "libnetdata/libnetdata.h"

#ifdef __FreeBSD__
#include <sys/user.h>
#endif

#ifdef __APPLE__
#include <mach/mach.h>
#include <mach/mach_host.h>
#include <libproc.h>
#include <sys/proc_info.h>
#include <sys/sysctl.h>
#include <mach/mach_time.h> // For mach_timebase_info_data_t and mach_timebase_info

extern mach_timebase_info_data_t mach_info;
#endif

// ----------------------------------------------------------------------------
// per O/S configuration

// the minimum PID of the system
// this is also the pid of the init process
#define INIT_PID 1

// if the way apps.plugin will work, will read the entire process list,
// including the resource utilization of each process, instantly
// set this to 1
// when set to 0, apps.plugin builds a sort list of processes, in order
// to process children processes, before parent processes
#if defined(__FreeBSD__) || defined(__APPLE__)
#define ALL_PIDS_ARE_READ_INSTANTLY 1
#else
#define ALL_PIDS_ARE_READ_INSTANTLY 0
#endif

#if defined(__APPLE__)
struct pid_info {
    struct kinfo_proc proc;
    struct proc_taskinfo taskinfo;
    struct proc_bsdinfo bsdinfo;
    struct rusage_info_v4 rusageinfo;
};
#endif

// ----------------------------------------------------------------------------

extern bool debug_enabled;
extern bool enable_guest_charts;
extern bool enable_detailed_uptime_charts;
extern bool enable_users_charts;
extern bool enable_groups_charts;
extern bool include_exited_childs;
extern bool enable_function_cmdline;
extern bool proc_pid_cmdline_is_needed;
extern bool enable_file_charts;

extern size_t
    global_iterations_counter,
    calls_counter,
    file_counter,
    filenames_allocated_counter,
    inodes_changed_counter,
    links_changed_counter,
    targets_assignment_counter,
    all_pids_count,
    apps_groups_targets_count;

extern int
    all_files_len,
    all_files_size,
    show_guest_time,
    show_guest_time_old;

extern kernel_uint_t
    global_utime,
    global_stime,
    global_gtime;

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

#if defined(__FreeBSD__) || defined(__APPLE__)
extern usec_t system_current_time_ut;
#else
extern kernel_uint_t system_uptime_secs;
#endif

extern size_t pagesize;

// ----------------------------------------------------------------------------
// string lengths

#define MAX_COMPARE_NAME 100
#define MAX_NAME 100
#define MAX_CMDLINE 65536

// ----------------------------------------------------------------------------
// to avoid reallocating too frequently, we can increase the number of spare
// file descriptors used by processes.
// IMPORTANT:
// having a lot of spares, increases the CPU utilization of the plugin.
#define MAX_SPARE_FDS 1

#if !defined(__FreeBSD__) && !defined(__APPLE__)
extern int max_fds_cache_seconds;
#endif

// ----------------------------------------------------------------------------
// some variables for keeping track of processes count by states

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

// ----------------------------------------------------------------------------
// the rates we are going to send to netdata will have this detail a value of:
//  - 1 will send just integer parts to netdata
//  - 100 will send 2 decimal points
//  - 1000 will send 3 decimal points
// etc.
#define RATES_DETAIL 10000ULL

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

// ----------------------------------------------------------------------------
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

struct target {
    char compare[MAX_COMPARE_NAME + 1];
    uint32_t comparehash;
    size_t comparelen;

    char id[MAX_NAME + 1];
    uint32_t idhash;

    char name[MAX_NAME + 1];
    char clean_name[MAX_NAME + 1]; // sanitized name used in chart id (need to replace at least dots)
    uid_t uid;
    gid_t gid;

    bool is_other;

    kernel_uint_t minflt;
    kernel_uint_t cminflt;
    kernel_uint_t majflt;
    kernel_uint_t cmajflt;
    kernel_uint_t utime;
    kernel_uint_t stime;
    kernel_uint_t gtime;
    kernel_uint_t cutime;
    kernel_uint_t cstime;
    kernel_uint_t cgtime;
    kernel_uint_t num_threads;
    // kernel_uint_t rss;

    kernel_uint_t status_vmsize;
    kernel_uint_t status_vmrss;
    kernel_uint_t status_vmshared;
    kernel_uint_t status_rssfile;
    kernel_uint_t status_rssshmem;
    kernel_uint_t status_vmswap;
    kernel_uint_t status_voluntary_ctxt_switches;
    kernel_uint_t status_nonvoluntary_ctxt_switches;

    kernel_uint_t io_logical_bytes_read;
    kernel_uint_t io_logical_bytes_written;
    kernel_uint_t io_read_calls;
    kernel_uint_t io_write_calls;
    kernel_uint_t io_storage_bytes_read;
    kernel_uint_t io_storage_bytes_written;
    kernel_uint_t io_cancelled_write_bytes;

    int *target_fds;
    int target_fds_size;

    struct openfds openfds;

    NETDATA_DOUBLE max_open_files_percent;

    kernel_uint_t uptime_min;
    kernel_uint_t uptime_sum;
    kernel_uint_t uptime_max;

    unsigned int processes; // how many processes have been merged to this
    int exposed;            // if set, we have sent this to netdata
    int hidden;             // if set, we set the hidden flag on the dimension
    int debug_enabled;
    int ends_with;
    int starts_with;        // if set, the compare string matches only the
                     // beginning of the command

    struct pid_on_target *root_pid; // list of aggregated pids for target debugging

    struct target *target;  // the one that will be reported to netdata
    struct target *next;
};

// ----------------------------------------------------------------------------
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
} PID_LOG;

// ----------------------------------------------------------------------------
// pid_stat
//
// structure to store data for each process running
// see: man proc for the description of the fields

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

struct pid_fd {
    int fd;

#if !defined(__FreeBSD__) && !defined(__APPLE__)
    ino_t inode;
    char *filename;
    uint32_t link_hash;
    size_t cache_iterations_counter;
    size_t cache_iterations_reset;
#endif
};

struct pid_stat {
    int32_t pid;
    int32_t ppid;
    // int32_t pgrp;
    // int32_t session;
    // int32_t tty_nr;
    // int32_t tpgid;
    // uint64_t flags;

    char state;

    char comm[MAX_COMPARE_NAME + 1];
    char *cmdline;

    // these are raw values collected
    kernel_uint_t minflt_raw;
    kernel_uint_t cminflt_raw;
    kernel_uint_t majflt_raw;
    kernel_uint_t cmajflt_raw;
    kernel_uint_t utime_raw;
    kernel_uint_t stime_raw;
    kernel_uint_t gtime_raw; // guest_time
    kernel_uint_t cutime_raw;
    kernel_uint_t cstime_raw;
    kernel_uint_t cgtime_raw; // cguest_time

    // these are rates
    kernel_uint_t minflt;
    kernel_uint_t cminflt;
    kernel_uint_t majflt;
    kernel_uint_t cmajflt;
    kernel_uint_t utime;
    kernel_uint_t stime;
    kernel_uint_t gtime;
    kernel_uint_t cutime;
    kernel_uint_t cstime;
    kernel_uint_t cgtime;

    // int64_t priority;
    // int64_t nice;
    int32_t num_threads;
    // int64_t itrealvalue;
    // kernel_uint_t collected_starttime;
    // kernel_uint_t vsize;
    // kernel_uint_t rss;
    // kernel_uint_t rsslim;
    // kernel_uint_t starcode;
    // kernel_uint_t endcode;
    // kernel_uint_t startstack;
    // kernel_uint_t kstkesp;
    // kernel_uint_t kstkeip;
    // uint64_t signal;
    // uint64_t blocked;
    // uint64_t sigignore;
    // uint64_t sigcatch;
    // uint64_t wchan;
    // uint64_t nswap;
    // uint64_t cnswap;
    // int32_t exit_signal;
    // int32_t processor;
    // uint32_t rt_priority;
    // uint32_t policy;
    // kernel_uint_t delayacct_blkio_ticks;

    uid_t uid;
    gid_t gid;

    kernel_uint_t status_voluntary_ctxt_switches_raw;
    kernel_uint_t status_nonvoluntary_ctxt_switches_raw;

    kernel_uint_t status_vmsize;
    kernel_uint_t status_vmrss;
    kernel_uint_t status_vmshared;
    kernel_uint_t status_rssfile;
    kernel_uint_t status_rssshmem;
    kernel_uint_t status_vmswap;
    kernel_uint_t status_voluntary_ctxt_switches;
    kernel_uint_t status_nonvoluntary_ctxt_switches;
#ifndef __FreeBSD__
    ARL_BASE *status_arl;
#endif

    kernel_uint_t io_logical_bytes_read_raw;
    kernel_uint_t io_logical_bytes_written_raw;
    kernel_uint_t io_read_calls_raw;
    kernel_uint_t io_write_calls_raw;
    kernel_uint_t io_storage_bytes_read_raw;
    kernel_uint_t io_storage_bytes_written_raw;
    kernel_uint_t io_cancelled_write_bytes_raw;

    kernel_uint_t io_logical_bytes_read;
    kernel_uint_t io_logical_bytes_written;
    kernel_uint_t io_read_calls;
    kernel_uint_t io_write_calls;
    kernel_uint_t io_storage_bytes_read;
    kernel_uint_t io_storage_bytes_written;
    kernel_uint_t io_cancelled_write_bytes;

    kernel_uint_t uptime;

    struct pid_fd *fds;             // array of fds it uses
    size_t fds_size;                // the size of the fds array

    struct openfds openfds;
    struct pid_limits limits;

    NETDATA_DOUBLE openfds_limits_percent;

    int sortlist;                   // higher numbers = top on the process tree
                  // each process gets a unique number

    int children_count;             // number of processes directly referencing this
    int keeploops;                  // increases by 1 every time keep is 1 and updated 0

    PID_LOG log_thrown;

    bool keep;                      // true when we need to keep this process in memory even after it exited
    bool updated;                   // true when the process is currently running
    bool merged;                    // true when it has been merged to its parent
    bool read;                      // true when we have already read this process for this iteration
    bool matched_by_config;

    struct target *target;          // app_groups.conf targets
    struct target *user_target;     // uid based targets
    struct target *group_target;    // gid based targets

    usec_t stat_collected_usec;
    usec_t last_stat_collected_usec;

    usec_t io_collected_usec;
    usec_t last_io_collected_usec;
    usec_t last_limits_collected_usec;

    char *fds_dirname;              // the full directory name in /proc/PID/fd

    char *stat_filename;
    char *status_filename;
    char *io_filename;
    char *cmdline_filename;
    char *limits_filename;

    struct pid_stat *parent;
    struct pid_stat *prev;
    struct pid_stat *next;
};

// ----------------------------------------------------------------------------

struct user_or_group_id {
    avl_t avl;

    union {
        uid_t uid;
        gid_t gid;
    } id;

    char *name;

    int updated;

    struct user_or_group_id * next;
};

extern struct target
    *apps_groups_default_target,
    *apps_groups_root_target,
    *users_root_target,
    *groups_root_target;

extern struct pid_stat *root_of_pids;

extern int update_every;
extern unsigned int time_factor;
extern kernel_uint_t MemTotal;

#if (ALL_PIDS_ARE_READ_INSTANTLY == 0)
extern pid_t *all_pids_sortlist;
#endif

#define APPS_PLUGIN_PROCESSES_FUNCTION_DESCRIPTION "Detailed information on the currently running processes."

void function_processes(const char *transaction, char *function,
                        usec_t *stop_monotonic_ut __maybe_unused, bool *cancelled __maybe_unused,
                        BUFFER *payload __maybe_unused, HTTP_ACCESS access,
                        const char *source __maybe_unused, void *data __maybe_unused);

struct target *find_target_by_name(struct target *base, const char *name);

struct target *get_users_target(uid_t uid);
struct target *get_groups_target(gid_t gid);
int read_apps_groups_conf(const char *path, const char *file);

void users_and_groups_init(void);
struct user_or_group_id *user_id_find(struct user_or_group_id *user_id_to_find);
struct user_or_group_id *group_id_find(struct user_or_group_id *group_id_to_find);

// ----------------------------------------------------------------------------
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
int managed_log(struct pid_stat *p, PID_LOG log, int status);

// ----------------------------------------------------------------------------
// macro to calculate the incremental rate of a value
// each parameter is accessed only ONCE - so it is safe to pass function calls
// or other macros as parameters

#define incremental_rate(rate_variable, last_kernel_variable, new_kernel_value, collected_usec, last_collected_usec) do { \
        kernel_uint_t _new_tmp = new_kernel_value; \
        (rate_variable) = (_new_tmp - (last_kernel_variable)) * (USEC_PER_SEC * RATES_DETAIL) / ((collected_usec) - (last_collected_usec)); \
        (last_kernel_variable) = _new_tmp; \
    } while(0)

// the same macro for struct pid members
#define pid_incremental_rate(type, var, value) \
    incremental_rate(var, var##_raw, value, p->type##_collected_usec, p->last_##type##_collected_usec)

int read_proc_pid_stat(struct pid_stat *p, void *ptr);
int read_proc_pid_limits(struct pid_stat *p, void *ptr);
int read_proc_pid_status(struct pid_stat *p, void *ptr);
int read_proc_pid_cmdline(struct pid_stat *p);
int read_proc_pid_io(struct pid_stat *p, void *ptr);
int read_pid_file_descriptors(struct pid_stat *p, void *ptr);
int read_global_time(void);
void get_MemTotal(void);

bool collect_data_for_all_pids(void);
void cleanup_exited_pids(void);

void clear_pid_fd(struct pid_fd *pfd);
void file_descriptor_not_used(int id);
void init_pid_fds(struct pid_stat *p, size_t first, size_t size);
void aggregate_pid_fds_on_targets(struct pid_stat *p);

void send_proc_states_count(usec_t dt);
void send_charts_updates_to_netdata(struct target *root, const char *type, const char *lbl_name, const char *title);
void send_collected_data_to_netdata(struct target *root, const char *type, usec_t dt);
void send_resource_usage_to_netdata(usec_t dt);

void pids_init(void);
struct pid_stat *find_pid_entry(pid_t pid);

#endif //NETDATA_APPS_PLUGIN_H
