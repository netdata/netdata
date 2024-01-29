// SPDX-License-Identifier: GPL-3.0-or-later

/*
 * netdata apps.plugin
 * (C) Copyright 2023 Netdata Inc.
 * Released under GPL v3+
 */

#include "collectors/all.h"
#include "libnetdata/libnetdata.h"
#include "libnetdata/required_dummies.h"

#define APPS_PLUGIN_PROCESSES_FUNCTION_DESCRIPTION "Detailed information on the currently running processes."

#define APPS_PLUGIN_FUNCTIONS() do { \
    fprintf(stdout, PLUGINSD_KEYWORD_FUNCTION " \"processes\" %d \"%s\" \"top\" "HTTP_ACCESS_FORMAT" %d\n",         \
            PLUGINS_FUNCTIONS_TIMEOUT_DEFAULT, APPS_PLUGIN_PROCESSES_FUNCTION_DESCRIPTION,                          \
            (HTTP_ACCESS_FORMAT_CAST)(HTTP_ACCESS_SIGNED_ID|HTTP_ACCESS_SAME_SPACE|HTTP_ACCESS_SENSITIVE_DATA),     \
            RRDFUNCTIONS_PRIORITY_DEFAULT / 10);                                                                    \
} while(0)

#define APPS_PLUGIN_GLOBAL_FUNCTIONS() do { \
    fprintf(stdout, PLUGINSD_KEYWORD_FUNCTION " GLOBAL \"processes\" %d \"%s\" \"top\" "HTTP_ACCESS_FORMAT" %d\n",  \
            PLUGINS_FUNCTIONS_TIMEOUT_DEFAULT, APPS_PLUGIN_PROCESSES_FUNCTION_DESCRIPTION,                          \
            (HTTP_ACCESS_FORMAT_CAST)(HTTP_ACCESS_SIGNED_ID|HTTP_ACCESS_SAME_SPACE|HTTP_ACCESS_SENSITIVE_DATA),     \
            RRDFUNCTIONS_PRIORITY_DEFAULT / 10);                                                                    \
} while(0)

// ----------------------------------------------------------------------------
// debugging

static int debug_enabled = 0;
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


// ----------------------------------------------------------------------------

#ifdef __FreeBSD__
#include <sys/user.h>
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
#ifdef __FreeBSD__
#define ALL_PIDS_ARE_READ_INSTANTLY 1
#else
#define ALL_PIDS_ARE_READ_INSTANTLY 0
#endif

// ----------------------------------------------------------------------------
// string lengths

#define MAX_COMPARE_NAME 100
#define MAX_NAME 100
#define MAX_CMDLINE 16384

// ----------------------------------------------------------------------------
// the rates we are going to send to netdata will have this detail a value of:
//  - 1 will send just integer parts to netdata
//  - 100 will send 2 decimal points
//  - 1000 will send 3 decimal points
// etc.
#define RATES_DETAIL 10000ULL

// ----------------------------------------------------------------------------
// factor for calculating correct CPU time values depending on units of raw data
static unsigned int time_factor = 0;

// ----------------------------------------------------------------------------
// to avoid reallocating too frequently, we can increase the number of spare
// file descriptors used by processes.
// IMPORTANT:
// having a lot of spares, increases the CPU utilization of the plugin.
#define MAX_SPARE_FDS 1

// ----------------------------------------------------------------------------
// command line options

static int
        update_every = 1,
        enable_guest_charts = 0,
#ifdef __FreeBSD__
        enable_file_charts = 0,
#else
        enable_file_charts = 1,
        max_fds_cache_seconds = 60,
#endif
        enable_function_cmdline = 0,
        enable_detailed_uptime_charts = 0,
        enable_users_charts = 1,
        enable_groups_charts = 1,
        include_exited_childs = 1;

// will be changed to getenv(NETDATA_USER_CONFIG_DIR) if it exists
static char *user_config_dir = CONFIG_DIR;
static char *stock_config_dir = LIBCONFIG_DIR;

// some variables for keeping track of processes count by states
typedef enum {
    PROC_STATUS_RUNNING = 0,
    PROC_STATUS_SLEEPING_D, // uninterruptible sleep
    PROC_STATUS_SLEEPING,   // interruptible sleep
    PROC_STATUS_ZOMBIE,
    PROC_STATUS_STOPPED,
    PROC_STATUS_END, //place holder for ending enum fields
} proc_state;

#ifndef __FreeBSD__
static proc_state proc_state_count[PROC_STATUS_END];
static const char *proc_states[] = {
    [PROC_STATUS_RUNNING] = "running",
    [PROC_STATUS_SLEEPING] = "sleeping_interruptible",
    [PROC_STATUS_SLEEPING_D] = "sleeping_uninterruptible",
    [PROC_STATUS_ZOMBIE] = "zombie",
    [PROC_STATUS_STOPPED] = "stopped",
    };
#endif

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

static int
        show_guest_time = 0,            // 1 when guest values are collected
        show_guest_time_old = 0,
        proc_pid_cmdline_is_needed = 0; // 1 when we need to read /proc/cmdline


// ----------------------------------------------------------------------------
// internal counters

static size_t
        global_iterations_counter = 1,
        calls_counter = 0,
        file_counter = 0,
        filenames_allocated_counter = 0,
        inodes_changed_counter = 0,
        links_changed_counter = 0,
        targets_assignment_counter = 0;


// ----------------------------------------------------------------------------
// Normalization
//
// With normalization we lower the collected metrics by a factor to make them
// match the total utilization of the system.
// The discrepancy exists because apps.plugin needs some time to collect all
// the metrics. This results in utilization that exceeds the total utilization
// of the system.
//
// During normalization, we align the per-process utilization, to the total of
// the system. We first consume the exited children utilization and it the
// collected values is above the total, we proportionally scale each reported
// metric.

// the total system time, as reported by /proc/stat
static kernel_uint_t
        global_utime = 0,
        global_stime = 0,
        global_gtime = 0;

// the normalization ratios, as calculated by normalize_utilization()
NETDATA_DOUBLE
        utime_fix_ratio = 1.0,
        stime_fix_ratio = 1.0,
        gtime_fix_ratio = 1.0,
        minflt_fix_ratio = 1.0,
        majflt_fix_ratio = 1.0,
        cutime_fix_ratio = 1.0,
        cstime_fix_ratio = 1.0,
        cgtime_fix_ratio = 1.0,
        cminflt_fix_ratio = 1.0,
        cmajflt_fix_ratio = 1.0;


struct pid_on_target {
    int32_t pid;
    struct pid_on_target *next;
};

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

// ----------------------------------------------------------------------------
// target
//
// target is the structure that processes are aggregated to be reported
// to netdata.
//
// - Each entry in /etc/apps_groups.conf creates a target.
// - Each user and group used by a process in the system, creates a target.

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

    kernel_uint_t starttime;
    kernel_uint_t collected_starttime;
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

struct target
        *apps_groups_default_target = NULL, // the default target
        *apps_groups_root_target = NULL,    // apps_groups.conf defined
        *users_root_target = NULL,          // users
        *groups_root_target = NULL;         // user groups

size_t
        apps_groups_targets_count = 0;       // # of apps_groups.conf targets


// ----------------------------------------------------------------------------
// pid_stat
//
// structure to store data for each process running
// see: man proc for the description of the fields

struct pid_fd {
    int fd;

#ifndef __FreeBSD__
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
    kernel_uint_t collected_starttime;
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

size_t pagesize;

kernel_uint_t global_uptime;

static struct pid_stat
        *root_of_pids = NULL,   // global list of all processes running
        **all_pids = NULL;      // to avoid allocations, we pre-allocate
                                // a pointer for each pid in the entire pid space.

static size_t
        all_pids_count = 0;     // the number of processes running

#if (ALL_PIDS_ARE_READ_INSTANTLY == 0)
// Another pre-allocated list of all possible pids.
// We need it to pids and assign them a unique sortlist id, so that we
// read parents before children. This is needed to prevent a situation where
// a child is found running, but until we read its parent, it has exited and
// its parent has accumulated its resources.
static pid_t
        *all_pids_sortlist = NULL;
#endif

// ----------------------------------------------------------------------------
// file descriptor
//
// this is used to keep a global list of all open files of the system.
// it is needed in order to calculate the unique files processes have open.

#define FILE_DESCRIPTORS_INCREASE_STEP 100

// types for struct file_descriptor->type
typedef enum fd_filetype {
    FILETYPE_OTHER,
    FILETYPE_FILE,
    FILETYPE_PIPE,
    FILETYPE_SOCKET,
    FILETYPE_INOTIFY,
    FILETYPE_EVENTFD,
    FILETYPE_EVENTPOLL,
    FILETYPE_TIMERFD,
    FILETYPE_SIGNALFD
} FD_FILETYPE;

struct file_descriptor {
    avl_t avl;

#ifdef NETDATA_INTERNAL_CHECKS
    uint32_t magic;
#endif /* NETDATA_INTERNAL_CHECKS */

    const char *name;
    uint32_t hash;

    FD_FILETYPE type;
    int count;
    int pos;
} *all_files = NULL;

static int
        all_files_len = 0,
        all_files_size = 0;

// ----------------------------------------------------------------------------
// read users and groups from files

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

enum user_or_group_id_type {
    USER_ID,
    GROUP_ID
};

struct user_or_group_ids{
    enum user_or_group_id_type type;

    avl_tree_type index;
    struct user_or_group_id *root;

    char filename[FILENAME_MAX + 1];
};

int user_id_compare(void* a, void* b) {
    if(((struct user_or_group_id *)a)->id.uid < ((struct user_or_group_id *)b)->id.uid)
        return -1;

    else if(((struct user_or_group_id *)a)->id.uid > ((struct user_or_group_id *)b)->id.uid)
        return 1;

    else
        return 0;
}

struct user_or_group_ids all_user_ids = {
    .type = USER_ID,

    .index = {
        NULL,
        user_id_compare
    },

    .root = NULL,

    .filename = "",
};

int group_id_compare(void* a, void* b) {
    if(((struct user_or_group_id *)a)->id.gid < ((struct user_or_group_id *)b)->id.gid)
        return -1;

    else if(((struct user_or_group_id *)a)->id.gid > ((struct user_or_group_id *)b)->id.gid)
        return 1;

    else
        return 0;
}

struct user_or_group_ids all_group_ids = {
    .type = GROUP_ID,

    .index = {
        NULL,
        group_id_compare
    },

    .root = NULL,

    .filename = "",
};

int file_changed(const struct stat *statbuf, struct timespec *last_modification_time) {
    if(likely(statbuf->st_mtim.tv_sec == last_modification_time->tv_sec &&
       statbuf->st_mtim.tv_nsec == last_modification_time->tv_nsec)) return 0;

    last_modification_time->tv_sec = statbuf->st_mtim.tv_sec;
    last_modification_time->tv_nsec = statbuf->st_mtim.tv_nsec;

    return 1;
}

int read_user_or_group_ids(struct user_or_group_ids *ids, struct timespec *last_modification_time) {
    struct stat statbuf;
    if(unlikely(stat(ids->filename, &statbuf)))
        return 1;
    else
        if(likely(!file_changed(&statbuf, last_modification_time))) return 0;

    procfile *ff = procfile_open(ids->filename, " :\t", PROCFILE_FLAG_DEFAULT);
    if(unlikely(!ff)) return 1;

    ff = procfile_readall(ff);
    if(unlikely(!ff)) return 1;

    size_t line, lines = procfile_lines(ff);

    for(line = 0; line < lines ;line++) {
        size_t words = procfile_linewords(ff, line);
        if(unlikely(words < 3)) continue;

        char *name = procfile_lineword(ff, line, 0);
        if(unlikely(!name || !*name)) continue;

        char *id_string = procfile_lineword(ff, line, 2);
        if(unlikely(!id_string || !*id_string)) continue;


        struct user_or_group_id *user_or_group_id = callocz(1, sizeof(struct user_or_group_id));

        if(ids->type == USER_ID)
            user_or_group_id->id.uid = (uid_t) str2ull(id_string, NULL);
        else
            user_or_group_id->id.gid = (uid_t) str2ull(id_string, NULL);

        user_or_group_id->name = strdupz(name);
        user_or_group_id->updated = 1;

        struct user_or_group_id *existing_user_id = NULL;

        if(likely(ids->root))
            existing_user_id = (struct user_or_group_id *)avl_search(&ids->index, (avl_t *) user_or_group_id);

        if(unlikely(existing_user_id)) {
            freez(existing_user_id->name);
            existing_user_id->name = user_or_group_id->name;
            existing_user_id->updated = 1;
            freez(user_or_group_id);
        }
        else {
            if(unlikely(avl_insert(&ids->index, (avl_t *) user_or_group_id) != (void *) user_or_group_id)) {
                netdata_log_error("INTERNAL ERROR: duplicate indexing of id during realloc");
            }

            user_or_group_id->next = ids->root;
            ids->root = user_or_group_id;
        }
    }

    procfile_close(ff);

    // remove unused ids
    struct user_or_group_id *user_or_group_id = ids->root, *prev_user_id = NULL;

    while(user_or_group_id) {
        if(unlikely(!user_or_group_id->updated)) {
            if(unlikely((struct user_or_group_id *)avl_remove(&ids->index, (avl_t *) user_or_group_id) != user_or_group_id))
                netdata_log_error("INTERNAL ERROR: removal of unused id from index, removed a different id");

            if(prev_user_id)
                prev_user_id->next = user_or_group_id->next;
            else
                ids->root = user_or_group_id->next;

            freez(user_or_group_id->name);
            freez(user_or_group_id);

            if(prev_user_id)
                user_or_group_id = prev_user_id->next;
            else
                user_or_group_id = ids->root;
        }
        else {
            user_or_group_id->updated = 0;

            prev_user_id = user_or_group_id;
            user_or_group_id = user_or_group_id->next;
        }
    }

    return 0;
}

// ----------------------------------------------------------------------------
// apps_groups.conf
// aggregate all processes in groups, to have a limited number of dimensions

static struct target *get_users_target(uid_t uid) {
    struct target *w;
    for(w = users_root_target ; w ; w = w->next)
        if(w->uid == uid) return w;

    w = callocz(sizeof(struct target), 1);
    snprintfz(w->compare, MAX_COMPARE_NAME, "%u", uid);
    w->comparehash = simple_hash(w->compare);
    w->comparelen = strlen(w->compare);

    snprintfz(w->id, MAX_NAME, "%u", uid);
    w->idhash = simple_hash(w->id);

    struct user_or_group_id user_id_to_find, *user_or_group_id = NULL;
    user_id_to_find.id.uid = uid;

    if(*netdata_configured_host_prefix) {
        static struct timespec last_passwd_modification_time;
        int ret = read_user_or_group_ids(&all_user_ids, &last_passwd_modification_time);

        if(likely(!ret && all_user_ids.index.root))
                user_or_group_id = (struct user_or_group_id *)avl_search(&all_user_ids.index, (avl_t *) &user_id_to_find);
    }

    if(user_or_group_id && user_or_group_id->name && *user_or_group_id->name) {
        snprintfz(w->name, MAX_NAME, "%s", user_or_group_id->name);
    }
    else {
        struct passwd *pw = getpwuid(uid);
        if(!pw || !pw->pw_name || !*pw->pw_name)
            snprintfz(w->name, MAX_NAME, "%u", uid);
        else
            snprintfz(w->name, MAX_NAME, "%s", pw->pw_name);
    }

    strncpyz(w->clean_name, w->name, MAX_NAME);
    netdata_fix_chart_name(w->clean_name);

    w->uid = uid;

    w->next = users_root_target;
    users_root_target = w;

    debug_log("added uid %u ('%s') target", w->uid, w->name);

    return w;
}

struct target *get_groups_target(gid_t gid)
{
    struct target *w;
    for(w = groups_root_target ; w ; w = w->next)
        if(w->gid == gid) return w;

    w = callocz(sizeof(struct target), 1);
    snprintfz(w->compare, MAX_COMPARE_NAME, "%u", gid);
    w->comparehash = simple_hash(w->compare);
    w->comparelen = strlen(w->compare);

    snprintfz(w->id, MAX_NAME, "%u", gid);
    w->idhash = simple_hash(w->id);

    struct user_or_group_id group_id_to_find, *group_id = NULL;
    group_id_to_find.id.gid = gid;

    if(*netdata_configured_host_prefix) {
        static struct timespec last_group_modification_time;
        int ret = read_user_or_group_ids(&all_group_ids, &last_group_modification_time);

        if(likely(!ret && all_group_ids.index.root))
                group_id = (struct user_or_group_id *)avl_search(&all_group_ids.index, (avl_t *) &group_id_to_find);
    }

    if(group_id && group_id->name && *group_id->name) {
        snprintfz(w->name, MAX_NAME, "%s", group_id->name);
    }
    else {
        struct group *gr = getgrgid(gid);
        if(!gr || !gr->gr_name || !*gr->gr_name)
            snprintfz(w->name, MAX_NAME, "%u", gid);
        else
            snprintfz(w->name, MAX_NAME, "%s", gr->gr_name);
    }

    strncpyz(w->clean_name, w->name, MAX_NAME);
    netdata_fix_chart_name(w->clean_name);

    w->gid = gid;

    w->next = groups_root_target;
    groups_root_target = w;

    debug_log("added gid %u ('%s') target", w->gid, w->name);

    return w;
}

// find or create a new target
// there are targets that are just aggregated to other target (the second argument)
static struct target *get_apps_groups_target(const char *id, struct target *target, const char *name) {
    int tdebug = 0, thidden = target?target->hidden:0, ends_with = 0;
    const char *nid = id;

    // extract the options
    while(nid[0] == '-' || nid[0] == '+' || nid[0] == '*') {
        if(nid[0] == '-') thidden = 1;
        if(nid[0] == '+') tdebug = 1;
        if(nid[0] == '*') ends_with = 1;
        nid++;
    }
    uint32_t hash = simple_hash(id);

    // find if it already exists
    struct target *w, *last = apps_groups_root_target;
    for(w = apps_groups_root_target ; w ; w = w->next) {
        if(w->idhash == hash && strncmp(nid, w->id, MAX_NAME) == 0)
            return w;

        last = w;
    }

    // find an existing target
    if(unlikely(!target)) {
        while(*name == '-') {
            if(*name == '-') thidden = 1;
            name++;
        }

        for(target = apps_groups_root_target ; target != NULL ; target = target->next) {
            if(!target->target && strcmp(name, target->name) == 0)
                break;
        }

        if(unlikely(debug_enabled)) {
            if(unlikely(target))
                debug_log("REUSING TARGET NAME '%s' on ID '%s'", target->name, target->id);
            else
                debug_log("NEW TARGET NAME '%s' on ID '%s'", name, id);
        }
    }

    if(target && target->target)
        fatal("Internal Error: request to link process '%s' to target '%s' which is linked to target '%s'", id, target->id, target->target->id);

    w = callocz(sizeof(struct target), 1);
    strncpyz(w->id, nid, MAX_NAME);
    w->idhash = simple_hash(w->id);

    if(unlikely(!target))
        // copy the name
        strncpyz(w->name, name, MAX_NAME);
    else
        // copy the id
        strncpyz(w->name, nid, MAX_NAME);
    
    // dots are used to distinguish chart type and id in streaming, so we should replace them
    strncpyz(w->clean_name, w->name, MAX_NAME);
    netdata_fix_chart_name(w->clean_name);
    for (char *d = w->clean_name; *d; d++) {
        if (*d == '.')
            *d = '_';
    }

    strncpyz(w->compare, nid, MAX_COMPARE_NAME);
    size_t len = strlen(w->compare);
    if(w->compare[len - 1] == '*') {
        w->compare[len - 1] = '\0';
        w->starts_with = 1;
    }
    w->ends_with = ends_with;

    if(w->starts_with && w->ends_with)
        proc_pid_cmdline_is_needed = 1;

    w->comparehash = simple_hash(w->compare);
    w->comparelen = strlen(w->compare);

    w->hidden = thidden;
#ifdef NETDATA_INTERNAL_CHECKS
    w->debug_enabled = tdebug;
#else
    if(tdebug)
        fprintf(stderr, "apps.plugin has been compiled without debugging\n");
#endif
    w->target = target;

    // append it, to maintain the order in apps_groups.conf
    if(last) last->next = w;
    else apps_groups_root_target = w;

    debug_log("ADDING TARGET ID '%s', process name '%s' (%s), aggregated on target '%s', options: %s %s"
            , w->id
            , w->compare, (w->starts_with && w->ends_with)?"substring":((w->starts_with)?"prefix":((w->ends_with)?"suffix":"exact"))
            , w->target?w->target->name:w->name
            , (w->hidden)?"hidden":"-"
            , (w->debug_enabled)?"debug":"-"
    );

    return w;
}

// read the apps_groups.conf file
static int read_apps_groups_conf(const char *path, const char *file)
{
    char filename[FILENAME_MAX + 1];

    snprintfz(filename, FILENAME_MAX, "%s/apps_%s.conf", path, file);

    debug_log("process groups file: '%s'", filename);

    // ----------------------------------------

    procfile *ff = procfile_open(filename, " :\t", PROCFILE_FLAG_DEFAULT);
    if(!ff) return 1;

    procfile_set_quotes(ff, "'\"");

    ff = procfile_readall(ff);
    if(!ff)
        return 1;

    size_t line, lines = procfile_lines(ff);

    for(line = 0; line < lines ;line++) {
        size_t word, words = procfile_linewords(ff, line);
        if(!words) continue;

        char *name = procfile_lineword(ff, line, 0);
        if(!name || !*name) continue;

        // find a possibly existing target
        struct target *w = NULL;

        // loop through all words, skipping the first one (the name)
        for(word = 0; word < words ;word++) {
            char *s = procfile_lineword(ff, line, word);
            if(!s || !*s) continue;
            if(*s == '#') break;

            // is this the first word? skip it
            if(s == name) continue;

            // add this target
            struct target *n = get_apps_groups_target(s, w, name);
            if(!n) {
                netdata_log_error("Cannot create target '%s' (line %zu, word %zu)", s, line, word);
                continue;
            }

            // just some optimization
            // to avoid searching for a target for each process
            if(!w) w = n->target?n->target:n;
        }
    }

    procfile_close(ff);

    apps_groups_default_target = get_apps_groups_target("p+!o@w#e$i^r&7*5(-i)l-o_", NULL, "other"); // match nothing
    if(!apps_groups_default_target)
        fatal("Cannot create default target");
    apps_groups_default_target->is_other = true;

    // allow the user to override group 'other'
    if(apps_groups_default_target->target)
        apps_groups_default_target = apps_groups_default_target->target;

    return 0;
}


// ----------------------------------------------------------------------------
// struct pid_stat management
static inline void init_pid_fds(struct pid_stat *p, size_t first, size_t size);

static inline struct pid_stat *get_pid_entry(pid_t pid) {
    if(unlikely(all_pids[pid]))
        return all_pids[pid];

    struct pid_stat *p = callocz(sizeof(struct pid_stat), 1);
    p->fds = mallocz(sizeof(struct pid_fd) * MAX_SPARE_FDS);
    p->fds_size = MAX_SPARE_FDS;
    init_pid_fds(p, 0, p->fds_size);
    p->pid = pid;

    DOUBLE_LINKED_LIST_APPEND_ITEM_UNSAFE(root_of_pids, p, prev, next);

    all_pids[pid] = p;
    all_pids_count++;

    return p;
}

static inline void del_pid_entry(pid_t pid) {
    struct pid_stat *p = all_pids[pid];

    if(unlikely(!p)) {
        netdata_log_error("attempted to free pid %d that is not allocated.", pid);
        return;
    }

    debug_log("process %d %s exited, deleting it.", pid, p->comm);

    DOUBLE_LINKED_LIST_REMOVE_ITEM_UNSAFE(root_of_pids, p, prev, next);

    // free the filename
#ifndef __FreeBSD__
    {
        size_t i;
        for(i = 0; i < p->fds_size; i++)
            if(p->fds[i].filename)
                freez(p->fds[i].filename);
    }
#endif
    freez(p->fds);

    freez(p->fds_dirname);
    freez(p->stat_filename);
    freez(p->status_filename);
    freez(p->limits_filename);
#ifndef __FreeBSD__
    arl_free(p->status_arl);
#endif
    freez(p->io_filename);
    freez(p->cmdline_filename);
    freez(p->cmdline);
    freez(p);

    all_pids[pid] = NULL;
    all_pids_count--;
}

// ----------------------------------------------------------------------------

static inline int managed_log(struct pid_stat *p, PID_LOG log, int status) {
    if(unlikely(!status)) {
        // netdata_log_error("command failed log %u, errno %d", log, errno);

        if(unlikely(debug_enabled || errno != ENOENT)) {
            if(unlikely(debug_enabled || !(p->log_thrown & log))) {
                p->log_thrown |= log;
                switch(log) {
                    case PID_LOG_IO:
                        #ifdef __FreeBSD__
                        netdata_log_error("Cannot fetch process %d I/O info (command '%s')", p->pid, p->comm);
                        #else
                        netdata_log_error("Cannot process %s/proc/%d/io (command '%s')", netdata_configured_host_prefix, p->pid, p->comm);
                        #endif
                        break;

                    case PID_LOG_STATUS:
                        #ifdef __FreeBSD__
                        netdata_log_error("Cannot fetch process %d status info (command '%s')", p->pid, p->comm);
                        #else
                        netdata_log_error("Cannot process %s/proc/%d/status (command '%s')", netdata_configured_host_prefix, p->pid, p->comm);
                        #endif
                        break;

                    case PID_LOG_CMDLINE:
                        #ifdef __FreeBSD__
                        netdata_log_error("Cannot fetch process %d command line (command '%s')", p->pid, p->comm);
                        #else
                        netdata_log_error("Cannot process %s/proc/%d/cmdline (command '%s')", netdata_configured_host_prefix, p->pid, p->comm);
                        #endif
                        break;

                    case PID_LOG_FDS:
                        #ifdef __FreeBSD__
                        netdata_log_error("Cannot fetch process %d files (command '%s')", p->pid, p->comm);
                        #else
                        netdata_log_error("Cannot process entries in %s/proc/%d/fd (command '%s')", netdata_configured_host_prefix, p->pid, p->comm);
                        #endif
                        break;

                    case PID_LOG_LIMITS:
                        #ifdef __FreeBSD__
                        ;
                        #else
                        netdata_log_error("Cannot process %s/proc/%d/limits (command '%s')", netdata_configured_host_prefix, p->pid, p->comm);
                        #endif

                    case PID_LOG_STAT:
                        break;

                    default:
                        netdata_log_error("unhandled error for pid %d, command '%s'", p->pid, p->comm);
                        break;
                }
            }
        }
        errno = 0;
    }
    else if(unlikely(p->log_thrown & log)) {
        // netdata_log_error("unsetting log %u on pid %d", log, p->pid);
        p->log_thrown &= ~log;
    }

    return status;
}

static inline void assign_target_to_pid(struct pid_stat *p) {
    targets_assignment_counter++;

    uint32_t hash = simple_hash(p->comm);
    size_t pclen  = strlen(p->comm);

    struct target *w;
    for(w = apps_groups_root_target; w ; w = w->next) {
        // if(debug_enabled || (p->target && p->target->debug_enabled)) debug_log_int("\t\tcomparing '%s' with '%s'", w->compare, p->comm);

        // find it - 4 cases:
        // 1. the target is not a pattern
        // 2. the target has the prefix
        // 3. the target has the suffix
        // 4. the target is something inside cmdline

        if(unlikely(( (!w->starts_with && !w->ends_with && w->comparehash == hash && !strcmp(w->compare, p->comm))
                      || (w->starts_with && !w->ends_with && !strncmp(w->compare, p->comm, w->comparelen))
                      || (!w->starts_with && w->ends_with && pclen >= w->comparelen && !strcmp(w->compare, &p->comm[pclen - w->comparelen]))
                      || (proc_pid_cmdline_is_needed && w->starts_with && w->ends_with && p->cmdline && strstr(p->cmdline, w->compare))
                    ))) {

            p->matched_by_config = true;
            if(w->target) p->target = w->target;
            else p->target = w;

            if(debug_enabled || (p->target && p->target->debug_enabled))
                debug_log_int("%s linked to target %s", p->comm, p->target->name);

            break;
        }
    }
}


// ----------------------------------------------------------------------------
// update pids from proc

static inline int read_proc_pid_cmdline(struct pid_stat *p) {
    static char cmdline[MAX_CMDLINE + 1];

#ifdef __FreeBSD__
    size_t i, bytes = MAX_CMDLINE;
    int mib[4];

    mib[0] = CTL_KERN;
    mib[1] = KERN_PROC;
    mib[2] = KERN_PROC_ARGS;
    mib[3] = p->pid;
    if (unlikely(sysctl(mib, 4, cmdline, &bytes, NULL, 0)))
        goto cleanup;
#else
    if(unlikely(!p->cmdline_filename)) {
        char filename[FILENAME_MAX + 1];
        snprintfz(filename, FILENAME_MAX, "%s/proc/%d/cmdline", netdata_configured_host_prefix, p->pid);
        p->cmdline_filename = strdupz(filename);
    }

    int fd = open(p->cmdline_filename, procfile_open_flags, 0666);
    if(unlikely(fd == -1)) goto cleanup;

    ssize_t i, bytes = read(fd, cmdline, MAX_CMDLINE);
    close(fd);

    if(unlikely(bytes < 0)) goto cleanup;
#endif

    cmdline[bytes] = '\0';
    for(i = 0; i < bytes ; i++) {
        if(unlikely(!cmdline[i])) cmdline[i] = ' ';
    }

    if(p->cmdline) freez(p->cmdline);
    p->cmdline = strdupz(cmdline);

    debug_log("Read file '%s' contents: %s", p->cmdline_filename, p->cmdline);

    return 1;

cleanup:
    // copy the command to the command line
    if(p->cmdline) freez(p->cmdline);
    p->cmdline = strdupz(p->comm);
    return 0;
}

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


// ----------------------------------------------------------------------------

#ifndef __FreeBSD__
struct arl_callback_ptr {
    struct pid_stat *p;
    procfile *ff;
    size_t line;
};

void arl_callback_status_uid(const char *name, uint32_t hash, const char *value, void *dst) {
    (void)name; (void)hash; (void)value;
    struct arl_callback_ptr *aptr = (struct arl_callback_ptr *)dst;
    if(unlikely(procfile_linewords(aptr->ff, aptr->line) < 5)) return;

    //const char *real_uid = procfile_lineword(aptr->ff, aptr->line, 1);
    const char *effective_uid = procfile_lineword(aptr->ff, aptr->line, 2);
    //const char *saved_uid = procfile_lineword(aptr->ff, aptr->line, 3);
    //const char *filesystem_uid = procfile_lineword(aptr->ff, aptr->line, 4);

    if(likely(effective_uid && *effective_uid))
        aptr->p->uid = (uid_t)str2l(effective_uid);
}

void arl_callback_status_gid(const char *name, uint32_t hash, const char *value, void *dst) {
    (void)name; (void)hash; (void)value;
    struct arl_callback_ptr *aptr = (struct arl_callback_ptr *)dst;
    if(unlikely(procfile_linewords(aptr->ff, aptr->line) < 5)) return;

    //const char *real_gid = procfile_lineword(aptr->ff, aptr->line, 1);
    const char *effective_gid = procfile_lineword(aptr->ff, aptr->line, 2);
    //const char *saved_gid = procfile_lineword(aptr->ff, aptr->line, 3);
    //const char *filesystem_gid = procfile_lineword(aptr->ff, aptr->line, 4);

    if(likely(effective_gid && *effective_gid))
        aptr->p->gid = (uid_t)str2l(effective_gid);
}

void arl_callback_status_vmsize(const char *name, uint32_t hash, const char *value, void *dst) {
    (void)name; (void)hash; (void)value;
    struct arl_callback_ptr *aptr = (struct arl_callback_ptr *)dst;
    if(unlikely(procfile_linewords(aptr->ff, aptr->line) < 3)) return;

    aptr->p->status_vmsize = str2kernel_uint_t(procfile_lineword(aptr->ff, aptr->line, 1));
}

void arl_callback_status_vmswap(const char *name, uint32_t hash, const char *value, void *dst) {
    (void)name; (void)hash; (void)value;
    struct arl_callback_ptr *aptr = (struct arl_callback_ptr *)dst;
    if(unlikely(procfile_linewords(aptr->ff, aptr->line) < 3)) return;

    aptr->p->status_vmswap = str2kernel_uint_t(procfile_lineword(aptr->ff, aptr->line, 1));
}

void arl_callback_status_vmrss(const char *name, uint32_t hash, const char *value, void *dst) {
    (void)name; (void)hash; (void)value;
    struct arl_callback_ptr *aptr = (struct arl_callback_ptr *)dst;
    if(unlikely(procfile_linewords(aptr->ff, aptr->line) < 3)) return;

    aptr->p->status_vmrss = str2kernel_uint_t(procfile_lineword(aptr->ff, aptr->line, 1));
}

void arl_callback_status_rssfile(const char *name, uint32_t hash, const char *value, void *dst) {
    (void)name; (void)hash; (void)value;
    struct arl_callback_ptr *aptr = (struct arl_callback_ptr *)dst;
    if(unlikely(procfile_linewords(aptr->ff, aptr->line) < 3)) return;

    aptr->p->status_rssfile = str2kernel_uint_t(procfile_lineword(aptr->ff, aptr->line, 1));
}

void arl_callback_status_rssshmem(const char *name, uint32_t hash, const char *value, void *dst) {
    (void)name; (void)hash; (void)value;
    struct arl_callback_ptr *aptr = (struct arl_callback_ptr *)dst;
    if(unlikely(procfile_linewords(aptr->ff, aptr->line) < 3)) return;

    aptr->p->status_rssshmem = str2kernel_uint_t(procfile_lineword(aptr->ff, aptr->line, 1));
}

void arl_callback_status_voluntary_ctxt_switches(const char *name, uint32_t hash, const char *value, void *dst) {
    (void)name; (void)hash; (void)value;
    struct arl_callback_ptr *aptr = (struct arl_callback_ptr *)dst;
    if(unlikely(procfile_linewords(aptr->ff, aptr->line) < 2)) return;

    struct pid_stat *p = aptr->p;
    pid_incremental_rate(stat, p->status_voluntary_ctxt_switches, str2kernel_uint_t(procfile_lineword(aptr->ff, aptr->line, 1)));
}

void arl_callback_status_nonvoluntary_ctxt_switches(const char *name, uint32_t hash, const char *value, void *dst) {
    (void)name; (void)hash; (void)value;
    struct arl_callback_ptr *aptr = (struct arl_callback_ptr *)dst;
    if(unlikely(procfile_linewords(aptr->ff, aptr->line) < 2)) return;

    struct pid_stat *p = aptr->p;
    pid_incremental_rate(stat, p->status_nonvoluntary_ctxt_switches, str2kernel_uint_t(procfile_lineword(aptr->ff, aptr->line, 1)));
}

static void update_proc_state_count(char proc_stt) {
    switch (proc_stt) {
        case 'S':
            proc_state_count[PROC_STATUS_SLEEPING] += 1;
            break;
        case 'R':
            proc_state_count[PROC_STATUS_RUNNING] += 1;
            break;
        case 'D':
            proc_state_count[PROC_STATUS_SLEEPING_D] += 1;
            break;
        case 'Z':
            proc_state_count[PROC_STATUS_ZOMBIE] += 1;
            break;
        case 'T':
            proc_state_count[PROC_STATUS_STOPPED] += 1;
            break;
        default:
            break;
    }
}
#endif // !__FreeBSD__

#define MAX_PROC_PID_LIMITS 8192
#define PROC_PID_LIMITS_MAX_OPEN_FILES_KEY "\nMax open files "

static inline kernel_uint_t get_proc_pid_limits_limit(char *buf, const char *key, size_t key_len, kernel_uint_t def) {
    char *line = strstr(buf, key);
    if(!line)
        return def;

    char *v = &line[key_len];
    while(isspace(*v)) v++;

    if(strcmp(v, "unlimited") == 0)
        return 0;

    return str2ull(v, NULL);
}

static inline int read_proc_pid_limits(struct pid_stat *p, void *ptr) {
    (void)ptr;

#ifdef __FreeBSD__
    return 0;
#else
    static char proc_pid_limits_buffer[MAX_PROC_PID_LIMITS + 1];
    int ret = 0;
    bool read_limits = false;

    errno = 0;
    proc_pid_limits_buffer[0] = '\0';

    kernel_uint_t all_fds = pid_openfds_sum(p);
    if(all_fds < p->limits.max_open_files / 2 && p->io_collected_usec > p->last_limits_collected_usec && p->io_collected_usec - p->last_limits_collected_usec <= 60 * USEC_PER_SEC) {
        // too frequent, we want to collect limits once per minute
        ret = 1;
        goto cleanup;
    }

    if(unlikely(!p->limits_filename)) {
        char filename[FILENAME_MAX + 1];
        snprintfz(filename, FILENAME_MAX, "%s/proc/%d/limits", netdata_configured_host_prefix, p->pid);
        p->limits_filename = strdupz(filename);
    }

    int fd = open(p->limits_filename, procfile_open_flags, 0666);
    if(unlikely(fd == -1)) goto cleanup;

    ssize_t bytes = read(fd, proc_pid_limits_buffer, MAX_PROC_PID_LIMITS);
    close(fd);

    if(bytes <= 0)
        goto cleanup;

    // make it '\0' terminated
    if(bytes < MAX_PROC_PID_LIMITS)
        proc_pid_limits_buffer[bytes] = '\0';
    else
        proc_pid_limits_buffer[MAX_PROC_PID_LIMITS - 1] = '\0';

    p->limits.max_open_files = get_proc_pid_limits_limit(proc_pid_limits_buffer, PROC_PID_LIMITS_MAX_OPEN_FILES_KEY, sizeof(PROC_PID_LIMITS_MAX_OPEN_FILES_KEY) - 1, 0);
    if(p->limits.max_open_files == 1) {
        // it seems a bug in the kernel or something similar
        // it sets max open files to 1 but the number of files
        // the process has open are more than 1...
        // https://github.com/netdata/netdata/issues/15443
        p->limits.max_open_files = 0;
        ret = 1;
        goto cleanup;
    }

    p->last_limits_collected_usec = p->io_collected_usec;
    read_limits = true;

    ret = 1;

cleanup:
    if(p->limits.max_open_files)
        p->openfds_limits_percent = (NETDATA_DOUBLE)all_fds * 100.0 / (NETDATA_DOUBLE)p->limits.max_open_files;
    else
        p->openfds_limits_percent = 0.0;

    if(p->openfds_limits_percent > 100.0) {
        if(!(p->log_thrown & PID_LOG_LIMITS_DETAIL)) {
            char *line;

            if(!read_limits) {
                proc_pid_limits_buffer[0] = '\0';
                line = "NOT READ";
            }
            else {
                line = strstr(proc_pid_limits_buffer, PROC_PID_LIMITS_MAX_OPEN_FILES_KEY);
                if (line) {
                    line++; // skip the initial newline

                    char *end = strchr(line, '\n');
                    if (end)
                        *end = '\0';
                }
            }

            netdata_log_info(
                    "FDS_LIMITS: PID %d (%s) is using "
                    "%0.2f %% of its fds limits, "
                    "open fds = %"PRIu64 "("
                    "files = %"PRIu64 ", "
                    "pipes = %"PRIu64 ", "
                    "sockets = %"PRIu64", "
                    "inotifies = %"PRIu64", "
                    "eventfds = %"PRIu64", "
                    "timerfds = %"PRIu64", "
                    "signalfds = %"PRIu64", "
                    "eventpolls = %"PRIu64" "
                    "other = %"PRIu64" "
                    "), open fds limit = %"PRIu64", "
                    "%s, "
                    "original line [%s]",
                    p->pid, p->comm, p->openfds_limits_percent, all_fds,
                    p->openfds.files,
                    p->openfds.pipes,
                    p->openfds.sockets,
                    p->openfds.inotifies,
                    p->openfds.eventfds,
                    p->openfds.timerfds,
                    p->openfds.signalfds,
                    p->openfds.eventpolls,
                    p->openfds.other,
                    p->limits.max_open_files,
                    read_limits ? "and we have read the limits AFTER counting the fds"
                                : "but we have read the limits BEFORE counting the fds",
                    line);

            p->log_thrown |= PID_LOG_LIMITS_DETAIL;
        }
    }
    else
        p->log_thrown &= ~PID_LOG_LIMITS_DETAIL;

    return ret;
#endif
}

static inline int read_proc_pid_status(struct pid_stat *p, void *ptr) {
    p->status_vmsize           = 0;
    p->status_vmrss            = 0;
    p->status_vmshared         = 0;
    p->status_rssfile          = 0;
    p->status_rssshmem         = 0;
    p->status_vmswap           = 0;
    p->status_voluntary_ctxt_switches = 0;
    p->status_nonvoluntary_ctxt_switches = 0;

#ifdef __FreeBSD__
    struct kinfo_proc *proc_info = (struct kinfo_proc *)ptr;

    p->uid                  = proc_info->ki_uid;
    p->gid                  = proc_info->ki_groups[0];
    p->status_vmsize        = proc_info->ki_size / 1024; // in KiB
    p->status_vmrss         = proc_info->ki_rssize * pagesize / 1024; // in KiB
    // TODO: what about shared and swap memory on FreeBSD?
    return 1;
#else
    (void)ptr;

    static struct arl_callback_ptr arl_ptr;
    static procfile *ff = NULL;

    if(unlikely(!p->status_arl)) {
        p->status_arl = arl_create("/proc/pid/status", NULL, 60);
        arl_expect_custom(p->status_arl, "Uid", arl_callback_status_uid, &arl_ptr);
        arl_expect_custom(p->status_arl, "Gid", arl_callback_status_gid, &arl_ptr);
        arl_expect_custom(p->status_arl, "VmSize", arl_callback_status_vmsize, &arl_ptr);
        arl_expect_custom(p->status_arl, "VmRSS", arl_callback_status_vmrss, &arl_ptr);
        arl_expect_custom(p->status_arl, "RssFile", arl_callback_status_rssfile, &arl_ptr);
        arl_expect_custom(p->status_arl, "RssShmem", arl_callback_status_rssshmem, &arl_ptr);
        arl_expect_custom(p->status_arl, "VmSwap", arl_callback_status_vmswap, &arl_ptr);
        arl_expect_custom(p->status_arl, "voluntary_ctxt_switches", arl_callback_status_voluntary_ctxt_switches, &arl_ptr);
        arl_expect_custom(p->status_arl, "nonvoluntary_ctxt_switches", arl_callback_status_nonvoluntary_ctxt_switches, &arl_ptr);
    }


    if(unlikely(!p->status_filename)) {
        char filename[FILENAME_MAX + 1];
        snprintfz(filename, FILENAME_MAX, "%s/proc/%d/status", netdata_configured_host_prefix, p->pid);
        p->status_filename = strdupz(filename);
    }

    ff = procfile_reopen(ff, p->status_filename, (!ff)?" \t:,-()/":NULL, PROCFILE_FLAG_NO_ERROR_ON_FILE_IO);
    if(unlikely(!ff)) return 0;

    ff = procfile_readall(ff);
    if(unlikely(!ff)) return 0;

    calls_counter++;

    // let ARL use this pid
    arl_ptr.p = p;
    arl_ptr.ff = ff;

    size_t lines = procfile_lines(ff), l;
    arl_begin(p->status_arl);

    for(l = 0; l < lines ;l++) {
        // debug_log("CHECK: line %zu of %zu, key '%s' = '%s'", l, lines, procfile_lineword(ff, l, 0), procfile_lineword(ff, l, 1));
        arl_ptr.line = l;
        if(unlikely(arl_check(p->status_arl,
                procfile_lineword(ff, l, 0),
                procfile_lineword(ff, l, 1)))) break;
    }

    p->status_vmshared = p->status_rssfile + p->status_rssshmem;

    // debug_log("%s uid %d, gid %d, VmSize %zu, VmRSS %zu, RssFile %zu, RssShmem %zu, shared %zu", p->comm, (int)p->uid, (int)p->gid, p->status_vmsize, p->status_vmrss, p->status_rssfile, p->status_rssshmem, p->status_vmshared);

    return 1;
#endif
}


// ----------------------------------------------------------------------------

static inline int read_proc_pid_stat(struct pid_stat *p, void *ptr) {
    (void)ptr;

#ifdef __FreeBSD__
    struct kinfo_proc *proc_info = (struct kinfo_proc *)ptr;
    if (unlikely(proc_info->ki_tdflags & TDF_IDLETD))
        goto cleanup;
#else
    static procfile *ff = NULL;

    if(unlikely(!p->stat_filename)) {
        char filename[FILENAME_MAX + 1];
        snprintfz(filename, FILENAME_MAX, "%s/proc/%d/stat", netdata_configured_host_prefix, p->pid);
        p->stat_filename = strdupz(filename);
    }

    int set_quotes = (!ff)?1:0;

    ff = procfile_reopen(ff, p->stat_filename, NULL, PROCFILE_FLAG_NO_ERROR_ON_FILE_IO);
    if(unlikely(!ff)) goto cleanup;

    // if(set_quotes) procfile_set_quotes(ff, "()");
    if(unlikely(set_quotes))
        procfile_set_open_close(ff, "(", ")");

    ff = procfile_readall(ff);
    if(unlikely(!ff)) goto cleanup;
#endif

    p->last_stat_collected_usec = p->stat_collected_usec;
    p->stat_collected_usec = now_monotonic_usec();
    calls_counter++;

#ifdef __FreeBSD__
    char *comm          = proc_info->ki_comm;
    p->ppid             = proc_info->ki_ppid;
#else
    // p->pid           = str2pid_t(procfile_lineword(ff, 0, 0));
    char *comm          = procfile_lineword(ff, 0, 1);
    p->state            = *(procfile_lineword(ff, 0, 2));
    p->ppid             = (int32_t)str2pid_t(procfile_lineword(ff, 0, 3));
    // p->pgrp          = (int32_t)str2pid_t(procfile_lineword(ff, 0, 4));
    // p->session       = (int32_t)str2pid_t(procfile_lineword(ff, 0, 5));
    // p->tty_nr        = (int32_t)str2pid_t(procfile_lineword(ff, 0, 6));
    // p->tpgid         = (int32_t)str2pid_t(procfile_lineword(ff, 0, 7));
    // p->flags         = str2uint64_t(procfile_lineword(ff, 0, 8));
#endif
    if(strcmp(p->comm, comm) != 0) {
        if(unlikely(debug_enabled)) {
            if(p->comm[0])
                debug_log("\tpid %d (%s) changed name to '%s'", p->pid, p->comm, comm);
            else
                debug_log("\tJust added %d (%s)", p->pid, comm);
        }

        strncpyz(p->comm, comm, MAX_COMPARE_NAME);

        // /proc/<pid>/cmdline
        if(likely(proc_pid_cmdline_is_needed))
            managed_log(p, PID_LOG_CMDLINE, read_proc_pid_cmdline(p));

        assign_target_to_pid(p);
    }

#ifdef __FreeBSD__
    pid_incremental_rate(stat, p->minflt,  (kernel_uint_t)proc_info->ki_rusage.ru_minflt);
    pid_incremental_rate(stat, p->cminflt, (kernel_uint_t)proc_info->ki_rusage_ch.ru_minflt);
    pid_incremental_rate(stat, p->majflt,  (kernel_uint_t)proc_info->ki_rusage.ru_majflt);
    pid_incremental_rate(stat, p->cmajflt, (kernel_uint_t)proc_info->ki_rusage_ch.ru_majflt);
    pid_incremental_rate(stat, p->utime,   (kernel_uint_t)proc_info->ki_rusage.ru_utime.tv_sec * 100 + proc_info->ki_rusage.ru_utime.tv_usec / 10000);
    pid_incremental_rate(stat, p->stime,   (kernel_uint_t)proc_info->ki_rusage.ru_stime.tv_sec * 100 + proc_info->ki_rusage.ru_stime.tv_usec / 10000);
    pid_incremental_rate(stat, p->cutime,  (kernel_uint_t)proc_info->ki_rusage_ch.ru_utime.tv_sec * 100 + proc_info->ki_rusage_ch.ru_utime.tv_usec / 10000);
    pid_incremental_rate(stat, p->cstime,  (kernel_uint_t)proc_info->ki_rusage_ch.ru_stime.tv_sec * 100 + proc_info->ki_rusage_ch.ru_stime.tv_usec / 10000);

    p->num_threads      = proc_info->ki_numthreads;

    if(enable_guest_charts) {
        enable_guest_charts = 0;
        netdata_log_info("Guest charts aren't supported by FreeBSD");
    }
#else
    pid_incremental_rate(stat, p->minflt,  str2kernel_uint_t(procfile_lineword(ff, 0,  9)));
    pid_incremental_rate(stat, p->cminflt, str2kernel_uint_t(procfile_lineword(ff, 0, 10)));
    pid_incremental_rate(stat, p->majflt,  str2kernel_uint_t(procfile_lineword(ff, 0, 11)));
    pid_incremental_rate(stat, p->cmajflt, str2kernel_uint_t(procfile_lineword(ff, 0, 12)));
    pid_incremental_rate(stat, p->utime,   str2kernel_uint_t(procfile_lineword(ff, 0, 13)));
    pid_incremental_rate(stat, p->stime,   str2kernel_uint_t(procfile_lineword(ff, 0, 14)));
    pid_incremental_rate(stat, p->cutime,  str2kernel_uint_t(procfile_lineword(ff, 0, 15)));
    pid_incremental_rate(stat, p->cstime,  str2kernel_uint_t(procfile_lineword(ff, 0, 16)));
    // p->priority      = str2kernel_uint_t(procfile_lineword(ff, 0, 17));
    // p->nice          = str2kernel_uint_t(procfile_lineword(ff, 0, 18));
    p->num_threads      = (int32_t) str2uint32_t(procfile_lineword(ff, 0, 19), NULL);
    // p->itrealvalue   = str2kernel_uint_t(procfile_lineword(ff, 0, 20));
    p->collected_starttime        = str2kernel_uint_t(procfile_lineword(ff, 0, 21)) / system_hz;
    p->uptime           = (global_uptime > p->collected_starttime)?(global_uptime - p->collected_starttime):0;
    // p->vsize         = str2kernel_uint_t(procfile_lineword(ff, 0, 22));
    // p->rss           = str2kernel_uint_t(procfile_lineword(ff, 0, 23));
    // p->rsslim        = str2kernel_uint_t(procfile_lineword(ff, 0, 24));
    // p->starcode      = str2kernel_uint_t(procfile_lineword(ff, 0, 25));
    // p->endcode       = str2kernel_uint_t(procfile_lineword(ff, 0, 26));
    // p->startstack    = str2kernel_uint_t(procfile_lineword(ff, 0, 27));
    // p->kstkesp       = str2kernel_uint_t(procfile_lineword(ff, 0, 28));
    // p->kstkeip       = str2kernel_uint_t(procfile_lineword(ff, 0, 29));
    // p->signal        = str2kernel_uint_t(procfile_lineword(ff, 0, 30));
    // p->blocked       = str2kernel_uint_t(procfile_lineword(ff, 0, 31));
    // p->sigignore     = str2kernel_uint_t(procfile_lineword(ff, 0, 32));
    // p->sigcatch      = str2kernel_uint_t(procfile_lineword(ff, 0, 33));
    // p->wchan         = str2kernel_uint_t(procfile_lineword(ff, 0, 34));
    // p->nswap         = str2kernel_uint_t(procfile_lineword(ff, 0, 35));
    // p->cnswap        = str2kernel_uint_t(procfile_lineword(ff, 0, 36));
    // p->exit_signal   = str2kernel_uint_t(procfile_lineword(ff, 0, 37));
    // p->processor     = str2kernel_uint_t(procfile_lineword(ff, 0, 38));
    // p->rt_priority   = str2kernel_uint_t(procfile_lineword(ff, 0, 39));
    // p->policy        = str2kernel_uint_t(procfile_lineword(ff, 0, 40));
    // p->delayacct_blkio_ticks = str2kernel_uint_t(procfile_lineword(ff, 0, 41));

    if(enable_guest_charts) {

        pid_incremental_rate(stat, p->gtime,  str2kernel_uint_t(procfile_lineword(ff, 0, 42)));
        pid_incremental_rate(stat, p->cgtime, str2kernel_uint_t(procfile_lineword(ff, 0, 43)));

        if (show_guest_time || p->gtime || p->cgtime) {
            p->utime -= (p->utime >= p->gtime) ? p->gtime : p->utime;
            p->cutime -= (p->cutime >= p->cgtime) ? p->cgtime : p->cutime;
            show_guest_time = 1;
        }
    }
#endif

    if(unlikely(debug_enabled || (p->target && p->target->debug_enabled)))
        debug_log_int("READ PROC/PID/STAT: %s/proc/%d/stat, process: '%s' on target '%s' (dt=%llu) VALUES: utime=" KERNEL_UINT_FORMAT ", stime=" KERNEL_UINT_FORMAT ", cutime=" KERNEL_UINT_FORMAT ", cstime=" KERNEL_UINT_FORMAT ", minflt=" KERNEL_UINT_FORMAT ", majflt=" KERNEL_UINT_FORMAT ", cminflt=" KERNEL_UINT_FORMAT ", cmajflt=" KERNEL_UINT_FORMAT ", threads=%d", netdata_configured_host_prefix, p->pid, p->comm, (p->target)?p->target->name:"UNSET", p->stat_collected_usec - p->last_stat_collected_usec, p->utime, p->stime, p->cutime, p->cstime, p->minflt, p->majflt, p->cminflt, p->cmajflt, p->num_threads);

    if(unlikely(global_iterations_counter == 1)) {
        p->minflt           = 0;
        p->cminflt          = 0;
        p->majflt           = 0;
        p->cmajflt          = 0;
        p->utime            = 0;
        p->stime            = 0;
        p->gtime            = 0;
        p->cutime           = 0;
        p->cstime           = 0;
        p->cgtime           = 0;
    }
#ifndef __FreeBSD__
    update_proc_state_count(p->state);
#endif
    return 1;

cleanup:
    p->minflt           = 0;
    p->cminflt          = 0;
    p->majflt           = 0;
    p->cmajflt          = 0;
    p->utime            = 0;
    p->stime            = 0;
    p->gtime            = 0;
    p->cutime           = 0;
    p->cstime           = 0;
    p->cgtime           = 0;
    p->num_threads      = 0;
    // p->rss              = 0;
    return 0;
}

// ----------------------------------------------------------------------------

static inline int read_proc_pid_io(struct pid_stat *p, void *ptr) {
    (void)ptr;
#ifdef __FreeBSD__
    struct kinfo_proc *proc_info = (struct kinfo_proc *)ptr;
#else
    static procfile *ff = NULL;

    if(unlikely(!p->io_filename)) {
        char filename[FILENAME_MAX + 1];
        snprintfz(filename, FILENAME_MAX, "%s/proc/%d/io", netdata_configured_host_prefix, p->pid);
        p->io_filename = strdupz(filename);
    }

    // open the file
    ff = procfile_reopen(ff, p->io_filename, NULL, PROCFILE_FLAG_NO_ERROR_ON_FILE_IO);
    if(unlikely(!ff)) goto cleanup;

    ff = procfile_readall(ff);
    if(unlikely(!ff)) goto cleanup;
#endif

    calls_counter++;

    p->last_io_collected_usec = p->io_collected_usec;
    p->io_collected_usec = now_monotonic_usec();

#ifdef __FreeBSD__
    pid_incremental_rate(io, p->io_storage_bytes_read,       proc_info->ki_rusage.ru_inblock);
    pid_incremental_rate(io, p->io_storage_bytes_written,    proc_info->ki_rusage.ru_oublock);
#else
    pid_incremental_rate(io, p->io_logical_bytes_read,       str2kernel_uint_t(procfile_lineword(ff, 0,  1)));
    pid_incremental_rate(io, p->io_logical_bytes_written,    str2kernel_uint_t(procfile_lineword(ff, 1,  1)));
    pid_incremental_rate(io, p->io_read_calls,               str2kernel_uint_t(procfile_lineword(ff, 2,  1)));
    pid_incremental_rate(io, p->io_write_calls,              str2kernel_uint_t(procfile_lineword(ff, 3,  1)));
    pid_incremental_rate(io, p->io_storage_bytes_read,       str2kernel_uint_t(procfile_lineword(ff, 4,  1)));
    pid_incremental_rate(io, p->io_storage_bytes_written,    str2kernel_uint_t(procfile_lineword(ff, 5,  1)));
    pid_incremental_rate(io, p->io_cancelled_write_bytes,    str2kernel_uint_t(procfile_lineword(ff, 6,  1)));
#endif

    if(unlikely(global_iterations_counter == 1)) {
        p->io_logical_bytes_read        = 0;
        p->io_logical_bytes_written     = 0;
        p->io_read_calls                = 0;
        p->io_write_calls               = 0;
        p->io_storage_bytes_read        = 0;
        p->io_storage_bytes_written     = 0;
        p->io_cancelled_write_bytes     = 0;
    }

    return 1;

#ifndef __FreeBSD__
cleanup:
    p->io_logical_bytes_read        = 0;
    p->io_logical_bytes_written     = 0;
    p->io_read_calls                = 0;
    p->io_write_calls               = 0;
    p->io_storage_bytes_read        = 0;
    p->io_storage_bytes_written     = 0;
    p->io_cancelled_write_bytes     = 0;
    return 0;
#endif
}

#ifndef __FreeBSD__
static inline int read_global_time() {
    static char filename[FILENAME_MAX + 1] = "";
    static procfile *ff = NULL;
    static kernel_uint_t utime_raw = 0, stime_raw = 0, gtime_raw = 0, gntime_raw = 0, ntime_raw = 0;
    static usec_t collected_usec = 0, last_collected_usec = 0;

    if(unlikely(!ff)) {
        snprintfz(filename, FILENAME_MAX, "%s/proc/stat", netdata_configured_host_prefix);
        ff = procfile_open(filename, " \t:", PROCFILE_FLAG_DEFAULT);
        if(unlikely(!ff)) goto cleanup;
    }

    ff = procfile_readall(ff);
    if(unlikely(!ff)) goto cleanup;

    last_collected_usec = collected_usec;
    collected_usec = now_monotonic_usec();

    calls_counter++;

    // temporary - it is added global_ntime;
    kernel_uint_t global_ntime = 0;

    incremental_rate(global_utime, utime_raw, str2kernel_uint_t(procfile_lineword(ff, 0,  1)), collected_usec, last_collected_usec);
    incremental_rate(global_ntime, ntime_raw, str2kernel_uint_t(procfile_lineword(ff, 0,  2)), collected_usec, last_collected_usec);
    incremental_rate(global_stime, stime_raw, str2kernel_uint_t(procfile_lineword(ff, 0,  3)), collected_usec, last_collected_usec);
    incremental_rate(global_gtime, gtime_raw, str2kernel_uint_t(procfile_lineword(ff, 0, 10)), collected_usec, last_collected_usec);

    global_utime += global_ntime;

    if(enable_guest_charts) {
        // temporary - it is added global_ntime;
        kernel_uint_t global_gntime = 0;

        // guest nice time, on guest time
        incremental_rate(global_gntime, gntime_raw, str2kernel_uint_t(procfile_lineword(ff, 0, 11)), collected_usec, last_collected_usec);

        global_gtime += global_gntime;

        // remove guest time from user time
        global_utime -= (global_utime > global_gtime) ? global_gtime : global_utime;
    }

    if(unlikely(global_iterations_counter == 1)) {
        global_utime = 0;
        global_stime = 0;
        global_gtime = 0;
    }

    return 1;

cleanup:
    global_utime = 0;
    global_stime = 0;
    global_gtime = 0;
    return 0;
}
#else
static inline int read_global_time() {
    static kernel_uint_t utime_raw = 0, stime_raw = 0, ntime_raw = 0;
    static usec_t collected_usec = 0, last_collected_usec = 0;
    long cp_time[CPUSTATES];

    if (unlikely(CPUSTATES != 5)) {
        goto cleanup;
    } else {
        static int mib[2] = {0, 0};

        if (unlikely(GETSYSCTL_SIMPLE("kern.cp_time", mib, cp_time))) {
            goto cleanup;
        }
    }

    last_collected_usec = collected_usec;
    collected_usec = now_monotonic_usec();

    calls_counter++;

    // temporary - it is added global_ntime;
    kernel_uint_t global_ntime = 0;

    incremental_rate(global_utime, utime_raw, cp_time[0] * 100LLU / system_hz, collected_usec, last_collected_usec);
    incremental_rate(global_ntime, ntime_raw, cp_time[1] * 100LLU / system_hz, collected_usec, last_collected_usec);
    incremental_rate(global_stime, stime_raw, cp_time[2] * 100LLU / system_hz, collected_usec, last_collected_usec);

    global_utime += global_ntime;

    if(unlikely(global_iterations_counter == 1)) {
        global_utime = 0;
        global_stime = 0;
        global_gtime = 0;
    }

    return 1;

cleanup:
    global_utime = 0;
    global_stime = 0;
    global_gtime = 0;
    return 0;
}
#endif /* !__FreeBSD__ */

// ----------------------------------------------------------------------------

int file_descriptor_compare(void* a, void* b) {
#ifdef NETDATA_INTERNAL_CHECKS
    if(((struct file_descriptor *)a)->magic != 0x0BADCAFE || ((struct file_descriptor *)b)->magic != 0x0BADCAFE)
        netdata_log_error("Corrupted index data detected. Please report this.");
#endif /* NETDATA_INTERNAL_CHECKS */

    if(((struct file_descriptor *)a)->hash < ((struct file_descriptor *)b)->hash)
        return -1;

    else if(((struct file_descriptor *)a)->hash > ((struct file_descriptor *)b)->hash)
        return 1;

    else
        return strcmp(((struct file_descriptor *)a)->name, ((struct file_descriptor *)b)->name);
}

// int file_descriptor_iterator(avl_t *a) { if(a) {}; return 0; }

avl_tree_type all_files_index = {
        NULL,
        file_descriptor_compare
};

static struct file_descriptor *file_descriptor_find(const char *name, uint32_t hash) {
    struct file_descriptor tmp;
    tmp.hash = (hash)?hash:simple_hash(name);
    tmp.name = name;
    tmp.count = 0;
    tmp.pos = 0;
#ifdef NETDATA_INTERNAL_CHECKS
    tmp.magic = 0x0BADCAFE;
#endif /* NETDATA_INTERNAL_CHECKS */

    return (struct file_descriptor *)avl_search(&all_files_index, (avl_t *) &tmp);
}

#define file_descriptor_add(fd) avl_insert(&all_files_index, (avl_t *)(fd))
#define file_descriptor_remove(fd) avl_remove(&all_files_index, (avl_t *)(fd))

// ----------------------------------------------------------------------------

static inline void file_descriptor_not_used(int id)
{
    if(id > 0 && id < all_files_size) {

#ifdef NETDATA_INTERNAL_CHECKS
        if(all_files[id].magic != 0x0BADCAFE) {
            netdata_log_error("Ignoring request to remove empty file id %d.", id);
            return;
        }
#endif /* NETDATA_INTERNAL_CHECKS */

        debug_log("decreasing slot %d (count = %d).", id, all_files[id].count);

        if(all_files[id].count > 0) {
            all_files[id].count--;

            if(!all_files[id].count) {
                debug_log("  >> slot %d is empty.", id);

                if(unlikely(file_descriptor_remove(&all_files[id]) != (void *)&all_files[id]))
                    netdata_log_error("INTERNAL ERROR: removal of unused fd from index, removed a different fd");

#ifdef NETDATA_INTERNAL_CHECKS
                all_files[id].magic = 0x00000000;
#endif /* NETDATA_INTERNAL_CHECKS */
                all_files_len--;
            }
        }
        else
            netdata_log_error("Request to decrease counter of fd %d (%s), while the use counter is 0",
                              id,
                              all_files[id].name);
    }
    else
        netdata_log_error("Request to decrease counter of fd %d, which is outside the array size (1 to %d)",
                          id,
                          all_files_size);
}

static inline void all_files_grow() {
    void *old = all_files;
    int i;

    // there is no empty slot
    debug_log("extending fd array to %d entries", all_files_size + FILE_DESCRIPTORS_INCREASE_STEP);

    all_files = reallocz(all_files, (all_files_size + FILE_DESCRIPTORS_INCREASE_STEP) * sizeof(struct file_descriptor));

    // if the address changed, we have to rebuild the index
    // since all pointers are now invalid

    if(unlikely(old && old != (void *)all_files)) {
        debug_log("  >> re-indexing.");

        all_files_index.root = NULL;
        for(i = 0; i < all_files_size; i++) {
            if(!all_files[i].count) continue;
            if(unlikely(file_descriptor_add(&all_files[i]) != (void *)&all_files[i]))
                netdata_log_error("INTERNAL ERROR: duplicate indexing of fd during realloc.");
        }

        debug_log("  >> re-indexing done.");
    }

    // initialize the newly added entries

    for(i = all_files_size; i < (all_files_size + FILE_DESCRIPTORS_INCREASE_STEP); i++) {
        all_files[i].count = 0;
        all_files[i].name = NULL;
#ifdef NETDATA_INTERNAL_CHECKS
        all_files[i].magic = 0x00000000;
#endif /* NETDATA_INTERNAL_CHECKS */
        all_files[i].pos = i;
    }

    if(unlikely(!all_files_size)) all_files_len = 1;
    all_files_size += FILE_DESCRIPTORS_INCREASE_STEP;
}

static inline int file_descriptor_set_on_empty_slot(const char *name, uint32_t hash, FD_FILETYPE type) {
    // check we have enough memory to add it
    if(!all_files || all_files_len == all_files_size)
        all_files_grow();

    debug_log("  >> searching for empty slot.");

    // search for an empty slot

    static int last_pos = 0;
    int i, c;
    for(i = 0, c = last_pos ; i < all_files_size ; i++, c++) {
        if(c >= all_files_size) c = 0;
        if(c == 0) continue;

        if(!all_files[c].count) {
            debug_log("  >> Examining slot %d.", c);

#ifdef NETDATA_INTERNAL_CHECKS
            if(all_files[c].magic == 0x0BADCAFE && all_files[c].name && file_descriptor_find(all_files[c].name, all_files[c].hash))
                netdata_log_error("fd on position %d is not cleared properly. It still has %s in it.", c, all_files[c].name);
#endif /* NETDATA_INTERNAL_CHECKS */

            debug_log("  >> %s fd position %d for %s (last name: %s)", all_files[c].name?"re-using":"using", c, name, all_files[c].name);

            freez((void *)all_files[c].name);
            all_files[c].name = NULL;
            last_pos = c;
            break;
        }
    }

    all_files_len++;

    if(i == all_files_size) {
        fatal("We should find an empty slot, but there isn't any");
        exit(1);
    }
    // else we have an empty slot in 'c'

    debug_log("  >> updating slot %d.", c);

    all_files[c].name = strdupz(name);
    all_files[c].hash = hash;
    all_files[c].type = type;
    all_files[c].pos  = c;
    all_files[c].count = 1;
#ifdef NETDATA_INTERNAL_CHECKS
    all_files[c].magic = 0x0BADCAFE;
#endif /* NETDATA_INTERNAL_CHECKS */
    if(unlikely(file_descriptor_add(&all_files[c]) != (void *)&all_files[c]))
        netdata_log_error("INTERNAL ERROR: duplicate indexing of fd.");

    debug_log("using fd position %d (name: %s)", c, all_files[c].name);

    return c;
}

static inline int file_descriptor_find_or_add(const char *name, uint32_t hash) {
    if(unlikely(!hash))
        hash = simple_hash(name);

    debug_log("adding or finding name '%s' with hash %u", name, hash);

    struct file_descriptor *fd = file_descriptor_find(name, hash);
    if(fd) {
        // found
        debug_log("  >> found on slot %d", fd->pos);

        fd->count++;
        return fd->pos;
    }
    // not found

    FD_FILETYPE type;
    if(likely(name[0] == '/')) type = FILETYPE_FILE;
    else if(likely(strncmp(name, "pipe:", 5) == 0)) type = FILETYPE_PIPE;
    else if(likely(strncmp(name, "socket:", 7) == 0)) type = FILETYPE_SOCKET;
    else if(likely(strncmp(name, "anon_inode:", 11) == 0)) {
        const char *t = &name[11];

        if(strcmp(t, "inotify") == 0) type = FILETYPE_INOTIFY;
        else if(strcmp(t, "[eventfd]") == 0) type = FILETYPE_EVENTFD;
        else if(strcmp(t, "[eventpoll]") == 0) type = FILETYPE_EVENTPOLL;
        else if(strcmp(t, "[timerfd]") == 0) type = FILETYPE_TIMERFD;
        else if(strcmp(t, "[signalfd]") == 0) type = FILETYPE_SIGNALFD;
        else {
            debug_log("UNKNOWN anonymous inode: %s", name);
            type = FILETYPE_OTHER;
        }
    }
    else if(likely(strcmp(name, "inotify") == 0)) type = FILETYPE_INOTIFY;
    else {
        debug_log("UNKNOWN linkname: %s", name);
        type = FILETYPE_OTHER;
    }

    return file_descriptor_set_on_empty_slot(name, hash, type);
}

static inline void clear_pid_fd(struct pid_fd *pfd) {
    pfd->fd = 0;

    #ifndef __FreeBSD__
    pfd->link_hash = 0;
    pfd->inode = 0;
    pfd->cache_iterations_counter = 0;
    pfd->cache_iterations_reset = 0;
#endif
}

static inline void make_all_pid_fds_negative(struct pid_stat *p) {
    struct pid_fd *pfd = p->fds, *pfdend = &p->fds[p->fds_size];
    while(pfd < pfdend) {
        pfd->fd = -(pfd->fd);
        pfd++;
    }
}

static inline void cleanup_negative_pid_fds(struct pid_stat *p) {
    struct pid_fd *pfd = p->fds, *pfdend = &p->fds[p->fds_size];

    while(pfd < pfdend) {
        int fd = pfd->fd;

        if(unlikely(fd < 0)) {
            file_descriptor_not_used(-(fd));
            clear_pid_fd(pfd);
        }

        pfd++;
    }
}

static inline void init_pid_fds(struct pid_stat *p, size_t first, size_t size) {
    struct pid_fd *pfd = &p->fds[first], *pfdend = &p->fds[first + size];

    while(pfd < pfdend) {
#ifndef __FreeBSD__
        pfd->filename = NULL;
#endif
        clear_pid_fd(pfd);
        pfd++;
    }
}

static inline int read_pid_file_descriptors(struct pid_stat *p, void *ptr) {
    (void)ptr;
#ifdef __FreeBSD__
    int mib[4];
    size_t size;
    struct kinfo_file *fds;
    static char *fdsbuf;
    char *bfdsbuf, *efdsbuf;
    char fdsname[FILENAME_MAX + 1];
#define SHM_FORMAT_LEN 31 // format: 21 + size: 10
    char shm_name[FILENAME_MAX - SHM_FORMAT_LEN + 1];

    // we make all pid fds negative, so that
    // we can detect unused file descriptors
    // at the end, to free them
    make_all_pid_fds_negative(p);

    mib[0] = CTL_KERN;
    mib[1] = KERN_PROC;
    mib[2] = KERN_PROC_FILEDESC;
    mib[3] = p->pid;

    if (unlikely(sysctl(mib, 4, NULL, &size, NULL, 0))) {
        netdata_log_error("sysctl error: Can't get file descriptors data size for pid %d", p->pid);
        return 0;
    }
    if (likely(size > 0))
        fdsbuf = reallocz(fdsbuf, size);
    if (unlikely(sysctl(mib, 4, fdsbuf, &size, NULL, 0))) {
        netdata_log_error("sysctl error: Can't get file descriptors data for pid %d", p->pid);
        return 0;
    }

    bfdsbuf = fdsbuf;
    efdsbuf = fdsbuf + size;
    while (bfdsbuf < efdsbuf) {
        fds = (struct kinfo_file *)(uintptr_t)bfdsbuf;
        if (unlikely(fds->kf_structsize == 0))
            break;

        // do not process file descriptors for current working directory, root directory,
        // jail directory, ktrace vnode, text vnode and controlling terminal
        if (unlikely(fds->kf_fd < 0)) {
            bfdsbuf += fds->kf_structsize;
            continue;
        }

        // get file descriptors array index
        size_t fdid = fds->kf_fd;

        // check if the fds array is small
        if (unlikely(fdid >= p->fds_size)) {
            // it is small, extend it

            debug_log("extending fd memory slots for %s from %d to %d", p->comm, p->fds_size, fdid + MAX_SPARE_FDS);

            p->fds = reallocz(p->fds, (fdid + MAX_SPARE_FDS) * sizeof(struct pid_fd));

            // and initialize it
            init_pid_fds(p, p->fds_size, (fdid + MAX_SPARE_FDS) - p->fds_size);
            p->fds_size = fdid + MAX_SPARE_FDS;
        }

        if (unlikely(p->fds[fdid].fd == 0)) {
            // we don't know this fd, get it

            switch (fds->kf_type) {
                case KF_TYPE_FIFO:
                case KF_TYPE_VNODE:
                    if (unlikely(!fds->kf_path[0])) {
                        sprintf(fdsname, "other: inode: %lu", fds->kf_un.kf_file.kf_file_fileid);
                        break;
                    }
                    sprintf(fdsname, "%s", fds->kf_path);
                    break;
                case KF_TYPE_SOCKET:
                    switch (fds->kf_sock_domain) {
                        case AF_INET:
                        case AF_INET6:
                            if (fds->kf_sock_protocol == IPPROTO_TCP)
                                sprintf(fdsname, "socket: %d %lx", fds->kf_sock_protocol, fds->kf_un.kf_sock.kf_sock_inpcb);
                            else
                                sprintf(fdsname, "socket: %d %lx", fds->kf_sock_protocol, fds->kf_un.kf_sock.kf_sock_pcb);
                            break;
                        case AF_UNIX:
                            /* print address of pcb and connected pcb */
                            sprintf(fdsname, "socket: %lx %lx", fds->kf_un.kf_sock.kf_sock_pcb, fds->kf_un.kf_sock.kf_sock_unpconn);
                            break;
                        default:
                            /* print protocol number and socket address */
#if __FreeBSD_version < 1200031
                            sprintf(fdsname, "socket: other: %d %s %s", fds->kf_sock_protocol, fds->kf_sa_local.__ss_pad1, fds->kf_sa_local.__ss_pad2);
#else
                            sprintf(fdsname, "socket: other: %d %s %s", fds->kf_sock_protocol, fds->kf_un.kf_sock.kf_sa_local.__ss_pad1, fds->kf_un.kf_sock.kf_sa_local.__ss_pad2);
#endif
                    }
                    break;
                case KF_TYPE_PIPE:
                    sprintf(fdsname, "pipe: %lu %lu", fds->kf_un.kf_pipe.kf_pipe_addr, fds->kf_un.kf_pipe.kf_pipe_peer);
                    break;
                case KF_TYPE_PTS:
#if __FreeBSD_version < 1200031
                    sprintf(fdsname, "other: pts: %u", fds->kf_un.kf_pts.kf_pts_dev);
#else
                    sprintf(fdsname, "other: pts: %lu", fds->kf_un.kf_pts.kf_pts_dev);
#endif
                    break;
                case KF_TYPE_SHM:
                    strncpyz(shm_name, fds->kf_path, FILENAME_MAX - SHM_FORMAT_LEN);
                    sprintf(fdsname, "other: shm: %s size: %lu", shm_name, fds->kf_un.kf_file.kf_file_size);
                    break;
                case KF_TYPE_SEM:
                    sprintf(fdsname, "other: sem: %u", fds->kf_un.kf_sem.kf_sem_value);
                    break;
                default:
                    sprintf(fdsname, "other: pid: %d fd: %d", fds->kf_un.kf_proc.kf_pid, fds->kf_fd);
            }

            // if another process already has this, we will get
            // the same id
            p->fds[fdid].fd = file_descriptor_find_or_add(fdsname, 0);
        }

            // else make it positive again, we need it
            // of course, the actual file may have changed

        else
            p->fds[fdid].fd = -p->fds[fdid].fd;

        bfdsbuf += fds->kf_structsize;
    }
#else
    if(unlikely(!p->fds_dirname)) {
        char dirname[FILENAME_MAX+1];
        snprintfz(dirname, FILENAME_MAX, "%s/proc/%d/fd", netdata_configured_host_prefix, p->pid);
        p->fds_dirname = strdupz(dirname);
    }

    DIR *fds = opendir(p->fds_dirname);
    if(unlikely(!fds)) return 0;

    struct dirent *de;
    char linkname[FILENAME_MAX + 1];

    // we make all pid fds negative, so that
    // we can detect unused file descriptors
    // at the end, to free them
    make_all_pid_fds_negative(p);

    while((de = readdir(fds))) {
        // we need only files with numeric names

        if(unlikely(de->d_name[0] < '0' || de->d_name[0] > '9'))
            continue;

        // get its number
        int fdid = (int) str2l(de->d_name);
        if(unlikely(fdid < 0)) continue;

        // check if the fds array is small
        if(unlikely((size_t)fdid >= p->fds_size)) {
            // it is small, extend it

            debug_log("extending fd memory slots for %s from %d to %d"
                    , p->comm
                    , p->fds_size
                    , fdid + MAX_SPARE_FDS
            );

            p->fds = reallocz(p->fds, (fdid + MAX_SPARE_FDS) * sizeof(struct pid_fd));

            // and initialize it
            init_pid_fds(p, p->fds_size, (fdid + MAX_SPARE_FDS) - p->fds_size);
            p->fds_size = (size_t)fdid + MAX_SPARE_FDS;
        }

        if(unlikely(p->fds[fdid].fd < 0 && de->d_ino != p->fds[fdid].inode)) {
            // inodes do not match, clear the previous entry
            inodes_changed_counter++;
            file_descriptor_not_used(-p->fds[fdid].fd);
            clear_pid_fd(&p->fds[fdid]);
        }

        if(p->fds[fdid].fd < 0 && p->fds[fdid].cache_iterations_counter > 0) {
            p->fds[fdid].fd = -p->fds[fdid].fd;
            p->fds[fdid].cache_iterations_counter--;
            continue;
        }

        if(unlikely(!p->fds[fdid].filename)) {
            filenames_allocated_counter++;
            char fdname[FILENAME_MAX + 1];
            snprintfz(fdname, FILENAME_MAX, "%s/proc/%d/fd/%s", netdata_configured_host_prefix, p->pid, de->d_name);
            p->fds[fdid].filename = strdupz(fdname);
        }

        file_counter++;
        ssize_t l = readlink(p->fds[fdid].filename, linkname, FILENAME_MAX);
        if(unlikely(l == -1)) {
            // cannot read the link

            if(debug_enabled || (p->target && p->target->debug_enabled))
                netdata_log_error("Cannot read link %s", p->fds[fdid].filename);

            if(unlikely(p->fds[fdid].fd < 0)) {
                file_descriptor_not_used(-p->fds[fdid].fd);
                clear_pid_fd(&p->fds[fdid]);
            }

            continue;
        }
        else
            linkname[l] = '\0';

        uint32_t link_hash = simple_hash(linkname);

        if(unlikely(p->fds[fdid].fd < 0 && p->fds[fdid].link_hash != link_hash)) {
            // the link changed
            links_changed_counter++;
            file_descriptor_not_used(-p->fds[fdid].fd);
            clear_pid_fd(&p->fds[fdid]);
        }

        if(unlikely(p->fds[fdid].fd == 0)) {
            // we don't know this fd, get it

            // if another process already has this, we will get
            // the same id
            p->fds[fdid].fd = file_descriptor_find_or_add(linkname, link_hash);
            p->fds[fdid].inode = de->d_ino;
            p->fds[fdid].link_hash = link_hash;
        }
        else {
            // else make it positive again, we need it
            p->fds[fdid].fd = -p->fds[fdid].fd;
        }

        // caching control
        // without this we read all the files on every iteration
        if(max_fds_cache_seconds > 0) {
            size_t spread = ((size_t)max_fds_cache_seconds > 10) ? 10 : (size_t)max_fds_cache_seconds;

            // cache it for a few iterations
            size_t max = ((size_t) max_fds_cache_seconds + (fdid % spread)) / (size_t) update_every;
            p->fds[fdid].cache_iterations_reset++;

            if(unlikely(p->fds[fdid].cache_iterations_reset % spread == (size_t) fdid % spread))
                p->fds[fdid].cache_iterations_reset++;

            if(unlikely((fdid <= 2 && p->fds[fdid].cache_iterations_reset > 5) ||
                        p->fds[fdid].cache_iterations_reset > max)) {
                // for stdin, stdout, stderr (fdid <= 2) we have checked a few times, or if it goes above the max, goto max
                p->fds[fdid].cache_iterations_reset = max;
            }

            p->fds[fdid].cache_iterations_counter = p->fds[fdid].cache_iterations_reset;
        }
    }

    closedir(fds);
#endif
    cleanup_negative_pid_fds(p);

    return 1;
}

// ----------------------------------------------------------------------------

static inline int debug_print_process_and_parents(struct pid_stat *p, usec_t time) {
    char *prefix = "\\_ ";
    int indent = 0;

    if(p->parent)
        indent = debug_print_process_and_parents(p->parent, p->stat_collected_usec);
    else
        prefix = " > ";

    char buffer[indent + 1];
    int i;

    for(i = 0; i < indent ;i++) buffer[i] = ' ';
    buffer[i] = '\0';

    fprintf(stderr, "  %s %s%s (%d %s %"PRIu64""
        , buffer
        , prefix
        , p->comm
        , p->pid
        , p->updated?"running":"exited"
        , p->stat_collected_usec - time
        );

    if(p->utime)   fprintf(stderr, " utime=" KERNEL_UINT_FORMAT,   p->utime);
    if(p->stime)   fprintf(stderr, " stime=" KERNEL_UINT_FORMAT,   p->stime);
    if(p->gtime)   fprintf(stderr, " gtime=" KERNEL_UINT_FORMAT,   p->gtime);
    if(p->cutime)  fprintf(stderr, " cutime=" KERNEL_UINT_FORMAT,  p->cutime);
    if(p->cstime)  fprintf(stderr, " cstime=" KERNEL_UINT_FORMAT,  p->cstime);
    if(p->cgtime)  fprintf(stderr, " cgtime=" KERNEL_UINT_FORMAT,  p->cgtime);
    if(p->minflt)  fprintf(stderr, " minflt=" KERNEL_UINT_FORMAT,  p->minflt);
    if(p->cminflt) fprintf(stderr, " cminflt=" KERNEL_UINT_FORMAT, p->cminflt);
    if(p->majflt)  fprintf(stderr, " majflt=" KERNEL_UINT_FORMAT,  p->majflt);
    if(p->cmajflt) fprintf(stderr, " cmajflt=" KERNEL_UINT_FORMAT, p->cmajflt);
    fprintf(stderr, ")\n");

    return indent + 1;
}

static inline void debug_print_process_tree(struct pid_stat *p, char *msg __maybe_unused) {
    debug_log("%s: process %s (%d, %s) with parents:", msg, p->comm, p->pid, p->updated?"running":"exited");
    debug_print_process_and_parents(p, p->stat_collected_usec);
}

static inline void debug_find_lost_child(struct pid_stat *pe, kernel_uint_t lost, int type) {
    int found = 0;
    struct pid_stat *p = NULL;

    for(p = root_of_pids; p ; p = p->next) {
        if(p == pe) continue;

        switch(type) {
            case 1:
                if(p->cminflt > lost) {
                    fprintf(stderr, " > process %d (%s) could use the lost exited child minflt " KERNEL_UINT_FORMAT " of process %d (%s)\n", p->pid, p->comm, lost, pe->pid, pe->comm);
                    found++;
                }
                break;

            case 2:
                if(p->cmajflt > lost) {
                    fprintf(stderr, " > process %d (%s) could use the lost exited child majflt " KERNEL_UINT_FORMAT " of process %d (%s)\n", p->pid, p->comm, lost, pe->pid, pe->comm);
                    found++;
                }
                break;

            case 3:
                if(p->cutime > lost) {
                    fprintf(stderr, " > process %d (%s) could use the lost exited child utime " KERNEL_UINT_FORMAT " of process %d (%s)\n", p->pid, p->comm, lost, pe->pid, pe->comm);
                    found++;
                }
                break;

            case 4:
                if(p->cstime > lost) {
                    fprintf(stderr, " > process %d (%s) could use the lost exited child stime " KERNEL_UINT_FORMAT " of process %d (%s)\n", p->pid, p->comm, lost, pe->pid, pe->comm);
                    found++;
                }
                break;

            case 5:
                if(p->cgtime > lost) {
                    fprintf(stderr, " > process %d (%s) could use the lost exited child gtime " KERNEL_UINT_FORMAT " of process %d (%s)\n", p->pid, p->comm, lost, pe->pid, pe->comm);
                    found++;
                }
                break;
        }
    }

    if(!found) {
        switch(type) {
            case 1:
                fprintf(stderr, " > cannot find any process to use the lost exited child minflt " KERNEL_UINT_FORMAT " of process %d (%s)\n", lost, pe->pid, pe->comm);
                break;

            case 2:
                fprintf(stderr, " > cannot find any process to use the lost exited child majflt " KERNEL_UINT_FORMAT " of process %d (%s)\n", lost, pe->pid, pe->comm);
                break;

            case 3:
                fprintf(stderr, " > cannot find any process to use the lost exited child utime " KERNEL_UINT_FORMAT " of process %d (%s)\n", lost, pe->pid, pe->comm);
                break;

            case 4:
                fprintf(stderr, " > cannot find any process to use the lost exited child stime " KERNEL_UINT_FORMAT " of process %d (%s)\n", lost, pe->pid, pe->comm);
                break;

            case 5:
                fprintf(stderr, " > cannot find any process to use the lost exited child gtime " KERNEL_UINT_FORMAT " of process %d (%s)\n", lost, pe->pid, pe->comm);
                break;
        }
    }
}

static inline kernel_uint_t remove_exited_child_from_parent(kernel_uint_t *field, kernel_uint_t *pfield) {
    kernel_uint_t absorbed = 0;

    if(*field > *pfield) {
        absorbed += *pfield;
        *field -= *pfield;
        *pfield = 0;
    }
    else {
        absorbed += *field;
        *pfield -= *field;
        *field = 0;
    }

    return absorbed;
}

static inline void process_exited_processes() {
    struct pid_stat *p;

    for(p = root_of_pids; p ; p = p->next) {
        if(p->updated || !p->stat_collected_usec)
            continue;

        kernel_uint_t utime  = (p->utime_raw + p->cutime_raw)   * (USEC_PER_SEC * RATES_DETAIL) / (p->stat_collected_usec - p->last_stat_collected_usec);
        kernel_uint_t stime  = (p->stime_raw + p->cstime_raw)   * (USEC_PER_SEC * RATES_DETAIL) / (p->stat_collected_usec - p->last_stat_collected_usec);
        kernel_uint_t gtime  = (p->gtime_raw + p->cgtime_raw)   * (USEC_PER_SEC * RATES_DETAIL) / (p->stat_collected_usec - p->last_stat_collected_usec);
        kernel_uint_t minflt = (p->minflt_raw + p->cminflt_raw) * (USEC_PER_SEC * RATES_DETAIL) / (p->stat_collected_usec - p->last_stat_collected_usec);
        kernel_uint_t majflt = (p->majflt_raw + p->cmajflt_raw) * (USEC_PER_SEC * RATES_DETAIL) / (p->stat_collected_usec - p->last_stat_collected_usec);

        if(utime + stime + gtime + minflt + majflt == 0)
            continue;

        if(unlikely(debug_enabled)) {
            debug_log("Absorb %s (%d %s total resources: utime=" KERNEL_UINT_FORMAT " stime=" KERNEL_UINT_FORMAT " gtime=" KERNEL_UINT_FORMAT " minflt=" KERNEL_UINT_FORMAT " majflt=" KERNEL_UINT_FORMAT ")"
                , p->comm
                , p->pid
                , p->updated?"running":"exited"
                , utime
                , stime
                , gtime
                , minflt
                , majflt
                );
            debug_print_process_tree(p, "Searching parents");
        }

        struct pid_stat *pp;
        for(pp = p->parent; pp ; pp = pp->parent) {
            if(!pp->updated) continue;

            kernel_uint_t absorbed;
            absorbed = remove_exited_child_from_parent(&utime,  &pp->cutime);
            if(unlikely(debug_enabled && absorbed))
                debug_log(" > process %s (%d %s) absorbed " KERNEL_UINT_FORMAT " utime (remaining: " KERNEL_UINT_FORMAT ")", pp->comm, pp->pid, pp->updated?"running":"exited", absorbed, utime);

            absorbed = remove_exited_child_from_parent(&stime,  &pp->cstime);
            if(unlikely(debug_enabled && absorbed))
                debug_log(" > process %s (%d %s) absorbed " KERNEL_UINT_FORMAT " stime (remaining: " KERNEL_UINT_FORMAT ")", pp->comm, pp->pid, pp->updated?"running":"exited", absorbed, stime);

            absorbed = remove_exited_child_from_parent(&gtime,  &pp->cgtime);
            if(unlikely(debug_enabled && absorbed))
                debug_log(" > process %s (%d %s) absorbed " KERNEL_UINT_FORMAT " gtime (remaining: " KERNEL_UINT_FORMAT ")", pp->comm, pp->pid, pp->updated?"running":"exited", absorbed, gtime);

            absorbed = remove_exited_child_from_parent(&minflt, &pp->cminflt);
            if(unlikely(debug_enabled && absorbed))
                debug_log(" > process %s (%d %s) absorbed " KERNEL_UINT_FORMAT " minflt (remaining: " KERNEL_UINT_FORMAT ")", pp->comm, pp->pid, pp->updated?"running":"exited", absorbed, minflt);

            absorbed = remove_exited_child_from_parent(&majflt, &pp->cmajflt);
            if(unlikely(debug_enabled && absorbed))
                debug_log(" > process %s (%d %s) absorbed " KERNEL_UINT_FORMAT " majflt (remaining: " KERNEL_UINT_FORMAT ")", pp->comm, pp->pid, pp->updated?"running":"exited", absorbed, majflt);
        }

        if(unlikely(utime + stime + gtime + minflt + majflt > 0)) {
            if(unlikely(debug_enabled)) {
                if(utime) debug_find_lost_child(p, utime, 3);
                if(stime) debug_find_lost_child(p, stime, 4);
                if(gtime) debug_find_lost_child(p, gtime, 5);
                if(minflt) debug_find_lost_child(p, minflt, 1);
                if(majflt) debug_find_lost_child(p, majflt, 2);
            }

            p->keep = true;

            debug_log(" > remaining resources - KEEP - for another loop: %s (%d %s total resources: utime=" KERNEL_UINT_FORMAT " stime=" KERNEL_UINT_FORMAT " gtime=" KERNEL_UINT_FORMAT " minflt=" KERNEL_UINT_FORMAT " majflt=" KERNEL_UINT_FORMAT ")"
                , p->comm
                , p->pid
                , p->updated?"running":"exited"
                , utime
                , stime
                , gtime
                , minflt
                , majflt
            );

            for(pp = p->parent; pp ; pp = pp->parent) {
                if(pp->updated) break;
                pp->keep = true;

                debug_log(" > - KEEP - parent for another loop: %s (%d %s)"
                    , pp->comm
                    , pp->pid
                    , pp->updated?"running":"exited"
                );
            }

            p->utime_raw   = utime  * (p->stat_collected_usec - p->last_stat_collected_usec) / (USEC_PER_SEC * RATES_DETAIL);
            p->stime_raw   = stime  * (p->stat_collected_usec - p->last_stat_collected_usec) / (USEC_PER_SEC * RATES_DETAIL);
            p->gtime_raw   = gtime  * (p->stat_collected_usec - p->last_stat_collected_usec) / (USEC_PER_SEC * RATES_DETAIL);
            p->minflt_raw  = minflt * (p->stat_collected_usec - p->last_stat_collected_usec) / (USEC_PER_SEC * RATES_DETAIL);
            p->majflt_raw  = majflt * (p->stat_collected_usec - p->last_stat_collected_usec) / (USEC_PER_SEC * RATES_DETAIL);
            p->cutime_raw = p->cstime_raw = p->cgtime_raw = p->cminflt_raw = p->cmajflt_raw = 0;

            debug_log(" ");
        }
        else
            debug_log(" > totally absorbed - DONE - %s (%d %s)"
                , p->comm
                , p->pid
                , p->updated?"running":"exited"
            );
    }
}

static inline void link_all_processes_to_their_parents(void) {
    struct pid_stat *p, *pp;

    // link all children to their parents
    // and update children count on parents
    for(p = root_of_pids; p ; p = p->next) {
        // for each process found

        p->sortlist = 0;
        p->parent = NULL;

        if(unlikely(!p->ppid)) {
            //unnecessary code from apps_plugin.c
            //p->parent = NULL;
            continue;
        }

        pp = all_pids[p->ppid];
        if(likely(pp)) {
            p->parent = pp;
            pp->children_count++;

            if(unlikely(debug_enabled || (p->target && p->target->debug_enabled)))
                debug_log_int("child %d (%s, %s) on target '%s' has parent %d (%s, %s). Parent: utime=" KERNEL_UINT_FORMAT ", stime=" KERNEL_UINT_FORMAT ", gtime=" KERNEL_UINT_FORMAT ", minflt=" KERNEL_UINT_FORMAT ", majflt=" KERNEL_UINT_FORMAT ", cutime=" KERNEL_UINT_FORMAT ", cstime=" KERNEL_UINT_FORMAT ", cgtime=" KERNEL_UINT_FORMAT ", cminflt=" KERNEL_UINT_FORMAT ", cmajflt=" KERNEL_UINT_FORMAT "", p->pid, p->comm, p->updated?"running":"exited", (p->target)?p->target->name:"UNSET", pp->pid, pp->comm, pp->updated?"running":"exited", pp->utime, pp->stime, pp->gtime, pp->minflt, pp->majflt, pp->cutime, pp->cstime, pp->cgtime, pp->cminflt, pp->cmajflt);
        }
        else {
            p->parent = NULL;
            netdata_log_error("pid %d %s states parent %d, but the later does not exist.", p->pid, p->comm, p->ppid);
        }
    }
}

// ----------------------------------------------------------------------------

// 1. read all files in /proc
// 2. for each numeric directory:
//    i.   read /proc/pid/stat
//    ii.  read /proc/pid/status
//    iii. read /proc/pid/io (requires root access)
//    iii. read the entries in directory /proc/pid/fd (requires root access)
//         for each entry:
//         a. find or create a struct file_descriptor
//         b. cleanup any old/unused file_descriptors

// after all these, some pids may be linked to targets, while others may not

// in case of errors, only 1 every 1000 errors is printed
// to avoid filling up all disk space
// if debug is enabled, all errors are printed

#if (ALL_PIDS_ARE_READ_INSTANTLY == 0)
static int compar_pid(const void *pid1, const void *pid2) {

    struct pid_stat *p1 = all_pids[*((pid_t *)pid1)];
    struct pid_stat *p2 = all_pids[*((pid_t *)pid2)];

    if(p1->sortlist > p2->sortlist)
        return -1;
    else
        return 1;
}
#endif

static inline int collect_data_for_pid(pid_t pid, void *ptr) {
    if(unlikely(pid < 0 || pid > pid_max)) {
        netdata_log_error("Invalid pid %d read (expected %d to %d). Ignoring process.", pid, 0, pid_max);
        return 0;
    }

    struct pid_stat *p = get_pid_entry(pid);
    if(unlikely(!p || p->read)) return 0;
    p->read = true;

    // debug_log("Reading process %d (%s), sortlist %d", p->pid, p->comm, p->sortlist);

    // --------------------------------------------------------------------
    // /proc/<pid>/stat

    if(unlikely(!managed_log(p, PID_LOG_STAT, read_proc_pid_stat(p, ptr))))
        // there is no reason to proceed if we cannot get its status
        return 0;

    // check its parent pid
    if(unlikely(p->ppid < 0 || p->ppid > pid_max)) {
        netdata_log_error("Pid %d (command '%s') states invalid parent pid %d. Using 0.", pid, p->comm, p->ppid);
        p->ppid = 0;
    }

    // --------------------------------------------------------------------
    // /proc/<pid>/io

    managed_log(p, PID_LOG_IO, read_proc_pid_io(p, ptr));

    // --------------------------------------------------------------------
    // /proc/<pid>/status

    if(unlikely(!managed_log(p, PID_LOG_STATUS, read_proc_pid_status(p, ptr))))
        // there is no reason to proceed if we cannot get its status
        return 0;

    // --------------------------------------------------------------------
    // /proc/<pid>/fd

    if(enable_file_charts) {
        managed_log(p, PID_LOG_FDS, read_pid_file_descriptors(p, ptr));
        managed_log(p, PID_LOG_LIMITS, read_proc_pid_limits(p, ptr));
    }

    // --------------------------------------------------------------------
    // done!

    if(unlikely(debug_enabled && include_exited_childs && all_pids_count && p->ppid && all_pids[p->ppid] && !all_pids[p->ppid]->read))
        debug_log("Read process %d (%s) sortlisted %d, but its parent %d (%s) sortlisted %d, is not read", p->pid, p->comm, p->sortlist, all_pids[p->ppid]->pid, all_pids[p->ppid]->comm, all_pids[p->ppid]->sortlist);

    // mark it as updated
    p->updated = true;
    p->keep = false;
    p->keeploops = 0;

    return 1;
}

static int collect_data_for_all_processes(void) {
    struct pid_stat *p = NULL;

#ifndef __FreeBSD__
    // clear process state counter
    memset(proc_state_count, 0, sizeof proc_state_count);
#else
    int i, procnum;

    static size_t procbase_size = 0;
    static struct kinfo_proc *procbase = NULL;

    size_t new_procbase_size;

    int mib[3] = { CTL_KERN, KERN_PROC, KERN_PROC_PROC };
    if (unlikely(sysctl(mib, 3, NULL, &new_procbase_size, NULL, 0))) {
        netdata_log_error("sysctl error: Can't get processes data size");
        return 0;
    }

    // give it some air for processes that may be started
    // during this little time.
    new_procbase_size += 100 * sizeof(struct kinfo_proc);

    // increase the buffer if needed
    if(new_procbase_size > procbase_size) {
        procbase_size = new_procbase_size;
        procbase = reallocz(procbase, procbase_size);
    }

    // sysctl() gets from new_procbase_size the buffer size
    // and also returns to it the amount of data filled in
    new_procbase_size = procbase_size;

    // get the processes from the system
    if (unlikely(sysctl(mib, 3, procbase, &new_procbase_size, NULL, 0))) {
        netdata_log_error("sysctl error: Can't get processes data");
        return 0;
    }

    // based on the amount of data filled in
    // calculate the number of processes we got
    procnum = new_procbase_size / sizeof(struct kinfo_proc);

#endif

    if(all_pids_count) {
#if (ALL_PIDS_ARE_READ_INSTANTLY == 0)
        size_t slc = 0;
#endif
        for(p = root_of_pids; p ; p = p->next) {
            p->read             = false; // mark it as not read, so that collect_data_for_pid() will read it
            p->updated          = false;
            p->merged           = false;
            p->children_count   = 0;
            p->parent           = NULL;

#if (ALL_PIDS_ARE_READ_INSTANTLY == 0)
            all_pids_sortlist[slc++] = p->pid;
#endif
        }

#if (ALL_PIDS_ARE_READ_INSTANTLY == 0)
        if(unlikely(slc != all_pids_count)) {
            netdata_log_error("Internal error: I was thinking I had %zu processes in my arrays, but it seems there are %zu.", all_pids_count, slc);
            all_pids_count = slc;
        }

        if(include_exited_childs) {
            // Read parents before childs
            // This is needed to prevent a situation where
            // a child is found running, but until we read
            // its parent, it has exited and its parent
            // has accumulated its resources.

            qsort((void *)all_pids_sortlist, (size_t)all_pids_count, sizeof(pid_t), compar_pid);

            // we forward read all running processes
            // collect_data_for_pid() is smart enough,
            // not to read the same pid twice per iteration
            for(slc = 0; slc < all_pids_count; slc++) {
                collect_data_for_pid(all_pids_sortlist[slc], NULL);
            }
        }
#endif
    }

#ifdef __FreeBSD__
    for (i = 0 ; i < procnum ; ++i) {
        pid_t pid = procbase[i].ki_pid;
        collect_data_for_pid(pid, &procbase[i]);
    }
#else
    static char uptime_filename[FILENAME_MAX + 1] = "";
    if(*uptime_filename == '\0')
        snprintfz(uptime_filename, FILENAME_MAX, "%s/proc/uptime", netdata_configured_host_prefix);

    global_uptime = (kernel_uint_t)(uptime_msec(uptime_filename) / MSEC_PER_SEC);

    char dirname[FILENAME_MAX + 1];

    snprintfz(dirname, FILENAME_MAX, "%s/proc", netdata_configured_host_prefix);
    DIR *dir = opendir(dirname);
    if(!dir) return 0;

    struct dirent *de = NULL;

    while((de = readdir(dir))) {
        char *endptr = de->d_name;

        if(unlikely(de->d_type != DT_DIR || de->d_name[0] < '0' || de->d_name[0] > '9'))
            continue;

        pid_t pid = (pid_t) strtoul(de->d_name, &endptr, 10);

        // make sure we read a valid number
        if(unlikely(endptr == de->d_name || *endptr != '\0'))
            continue;

        collect_data_for_pid(pid, NULL);
    }
    closedir(dir);
#endif

    if(!all_pids_count)
        return 0;

    // we need /proc/stat to normalize the cpu consumption of the exited childs
    read_global_time();

    // build the process tree
    link_all_processes_to_their_parents();

    // normally this is done
    // however we may have processes exited while we collected values
    // so let's find the exited ones
    // we do this by collecting the ownership of process
    // if we manage to get the ownership, the process still runs
    process_exited_processes();
    return 1;
}

// ----------------------------------------------------------------------------
// update statistics on the targets

// 1. link all childs to their parents
// 2. go from bottom to top, marking as merged all childs to their parents
//    this step links all parents without a target to the child target, if any
// 3. link all top level processes (the ones not merged) to the default target
// 4. go from top to bottom, linking all childs without a target, to their parent target
//    after this step, all processes have a target
// [5. for each killed pid (updated = 0), remove its usage from its target]
// 6. zero all apps_groups_targets
// 7. concentrate all values on the apps_groups_targets
// 8. remove all killed processes
// 9. find the unique file count for each target
// check: update_apps_groups_statistics()

static void cleanup_exited_pids(void) {
    size_t c;
    struct pid_stat *p = NULL;

    for(p = root_of_pids; p ;) {
        if(!p->updated && (!p->keep || p->keeploops > 0)) {
            if(unlikely(debug_enabled && (p->keep || p->keeploops)))
                debug_log(" > CLEANUP cannot keep exited process %d (%s) anymore - removing it.", p->pid, p->comm);

            for(c = 0; c < p->fds_size; c++)
                if(p->fds[c].fd > 0) {
                    file_descriptor_not_used(p->fds[c].fd);
                    clear_pid_fd(&p->fds[c]);
                }

            pid_t r = p->pid;
            p = p->next;
            del_pid_entry(r);
        }
        else {
            if(unlikely(p->keep)) p->keeploops++;
            p->keep = false;
            p = p->next;
        }
    }
}

static void apply_apps_groups_targets_inheritance(void) {
    struct pid_stat *p = NULL;

    // children that do not have a target
    // inherit their target from their parent
    int found = 1, loops = 0;
    while(found) {
        if(unlikely(debug_enabled)) loops++;
        found = 0;
        for(p = root_of_pids; p ; p = p->next) {
            // if this process does not have a target,
            // and it has a parent
            // and its parent has a target
            // then, set the parent's target to this process
            if(unlikely(!p->target && p->parent && p->parent->target)) {
                p->target = p->parent->target;
                found++;

                if(debug_enabled || (p->target && p->target->debug_enabled))
                    debug_log_int("TARGET INHERITANCE: %s is inherited by %d (%s) from its parent %d (%s).", p->target->name, p->pid, p->comm, p->parent->pid, p->parent->comm);
            }
        }
    }

    // find all the procs with 0 childs and merge them to their parents
    // repeat, until nothing more can be done.
    int sortlist = 1;
    found = 1;
    while(found) {
        if(unlikely(debug_enabled)) loops++;
        found = 0;

        for(p = root_of_pids; p ; p = p->next) {
            if(unlikely(!p->sortlist && !p->children_count))
                p->sortlist = sortlist++;

            if(unlikely(
                    !p->children_count            // if this process does not have any children
                    && !p->merged                 // and is not already merged
                    && p->parent                  // and has a parent
                    && p->parent->children_count  // and its parent has children
                                                  // and the target of this process and its parent is the same,
                                                  // or the parent does not have a target
                    && (p->target == p->parent->target || !p->parent->target)
                    && p->ppid != INIT_PID        // and its parent is not init
                )) {
                // mark it as merged
                p->parent->children_count--;
                p->merged = true;

                // the parent inherits the child's target, if it does not have a target itself
                if(unlikely(p->target && !p->parent->target)) {
                    p->parent->target = p->target;

                    if(debug_enabled || (p->target && p->target->debug_enabled))
                        debug_log_int("TARGET INHERITANCE: %s is inherited by %d (%s) from its child %d (%s).", p->target->name, p->parent->pid, p->parent->comm, p->pid, p->comm);
                }

                found++;
            }
        }

        debug_log("TARGET INHERITANCE: merged %d processes", found);
    }

    // init goes always to default target
    if(all_pids[INIT_PID] && !all_pids[INIT_PID]->matched_by_config)
        all_pids[INIT_PID]->target = apps_groups_default_target;

    // pid 0 goes always to default target
    if(all_pids[0] && !all_pids[INIT_PID]->matched_by_config)
        all_pids[0]->target = apps_groups_default_target;

    // give a default target on all top level processes
    if(unlikely(debug_enabled)) loops++;
    for(p = root_of_pids; p ; p = p->next) {
        // if the process is not merged itself
        // then it is a top level process
        if(unlikely(!p->merged && !p->target))
            p->target = apps_groups_default_target;

        // make sure all processes have a sortlist
        if(unlikely(!p->sortlist))
            p->sortlist = sortlist++;
    }

    if(all_pids[1])
        all_pids[1]->sortlist = sortlist++;

    // give a target to all merged child processes
    found = 1;
    while(found) {
        if(unlikely(debug_enabled)) loops++;
        found = 0;
        for(p = root_of_pids; p ; p = p->next) {
            if(unlikely(!p->target && p->merged && p->parent && p->parent->target)) {
                p->target = p->parent->target;
                found++;

                if(debug_enabled || (p->target && p->target->debug_enabled))
                    debug_log_int("TARGET INHERITANCE: %s is inherited by %d (%s) from its parent %d (%s) at phase 2.", p->target->name, p->pid, p->comm, p->parent->pid, p->parent->comm);
            }
        }
    }

    debug_log("apply_apps_groups_targets_inheritance() made %d loops on the process tree", loops);
}

static size_t zero_all_targets(struct target *root) {
    struct target *w;
    size_t count = 0;

    for (w = root; w ; w = w->next) {
        count++;

        w->minflt = 0;
        w->majflt = 0;
        w->utime = 0;
        w->stime = 0;
        w->gtime = 0;
        w->cminflt = 0;
        w->cmajflt = 0;
        w->cutime = 0;
        w->cstime = 0;
        w->cgtime = 0;
        w->num_threads = 0;
        // w->rss = 0;
        w->processes = 0;

        w->status_vmsize = 0;
        w->status_vmrss = 0;
        w->status_vmshared = 0;
        w->status_rssfile = 0;
        w->status_rssshmem = 0;
        w->status_vmswap = 0;
        w->status_voluntary_ctxt_switches = 0;
        w->status_nonvoluntary_ctxt_switches = 0;

        w->io_logical_bytes_read = 0;
        w->io_logical_bytes_written = 0;
        w->io_read_calls = 0;
        w->io_write_calls = 0;
        w->io_storage_bytes_read = 0;
        w->io_storage_bytes_written = 0;
        w->io_cancelled_write_bytes = 0;

        // zero file counters
        if(w->target_fds) {
            memset(w->target_fds, 0, sizeof(int) * w->target_fds_size);
            w->openfds.files = 0;
            w->openfds.pipes = 0;
            w->openfds.sockets = 0;
            w->openfds.inotifies = 0;
            w->openfds.eventfds = 0;
            w->openfds.timerfds = 0;
            w->openfds.signalfds = 0;
            w->openfds.eventpolls = 0;
            w->openfds.other = 0;

            w->max_open_files_percent = 0.0;
        }

        w->collected_starttime = 0;
        w->uptime_min = 0;
        w->uptime_sum = 0;
        w->uptime_max = 0;

        if(unlikely(w->root_pid)) {
            struct pid_on_target *pid_on_target_to_free, *pid_on_target = w->root_pid;

            while(pid_on_target) {
                pid_on_target_to_free = pid_on_target;
                pid_on_target = pid_on_target->next;
                freez(pid_on_target_to_free);
            }

            w->root_pid = NULL;
        }
    }

    return count;
}

static inline void reallocate_target_fds(struct target *w) {
    if(unlikely(!w))
        return;

    if(unlikely(!w->target_fds || w->target_fds_size < all_files_size)) {
        w->target_fds = reallocz(w->target_fds, sizeof(int) * all_files_size);
        memset(&w->target_fds[w->target_fds_size], 0, sizeof(int) * (all_files_size - w->target_fds_size));
        w->target_fds_size = all_files_size;
    }
}

static void aggregage_fd_type_on_openfds(FD_FILETYPE type, struct openfds *openfds) {
    switch(type) {
        case FILETYPE_FILE:
            openfds->files++;
            break;

        case FILETYPE_PIPE:
            openfds->pipes++;
            break;

        case FILETYPE_SOCKET:
            openfds->sockets++;
            break;

        case FILETYPE_INOTIFY:
            openfds->inotifies++;
            break;

        case FILETYPE_EVENTFD:
            openfds->eventfds++;
            break;

        case FILETYPE_TIMERFD:
            openfds->timerfds++;
            break;

        case FILETYPE_SIGNALFD:
            openfds->signalfds++;
            break;

        case FILETYPE_EVENTPOLL:
            openfds->eventpolls++;
            break;

        case FILETYPE_OTHER:
            openfds->other++;
            break;
    }
}

static inline void aggregate_fd_on_target(int fd, struct target *w) {
    if(unlikely(!w))
        return;

    if(unlikely(w->target_fds[fd])) {
        // it is already aggregated
        // just increase its usage counter
        w->target_fds[fd]++;
        return;
    }

    // increase its usage counter
    // so that we will not add it again
    w->target_fds[fd]++;

    aggregage_fd_type_on_openfds(all_files[fd].type, &w->openfds);
}

static inline void aggregate_pid_fds_on_targets(struct pid_stat *p) {

    if(unlikely(!p->updated)) {
        // the process is not running
        return;
    }

    struct target *w = p->target, *u = p->user_target, *g = p->group_target;

    reallocate_target_fds(w);
    reallocate_target_fds(u);
    reallocate_target_fds(g);

    p->openfds.files = 0;
    p->openfds.pipes = 0;
    p->openfds.sockets = 0;
    p->openfds.inotifies = 0;
    p->openfds.eventfds = 0;
    p->openfds.timerfds = 0;
    p->openfds.signalfds = 0;
    p->openfds.eventpolls = 0;
    p->openfds.other = 0;

    long currentfds = 0;
    size_t c, size = p->fds_size;
    struct pid_fd *fds = p->fds;
    for(c = 0; c < size ;c++) {
        int fd = fds[c].fd;

        if(likely(fd <= 0 || fd >= all_files_size))
            continue;

        currentfds++;
        aggregage_fd_type_on_openfds(all_files[fd].type, &p->openfds);

        aggregate_fd_on_target(fd, w);
        aggregate_fd_on_target(fd, u);
        aggregate_fd_on_target(fd, g);
    }
}

static inline void aggregate_pid_on_target(struct target *w, struct pid_stat *p, struct target *o) {
    (void)o;

    if(unlikely(!p->updated)) {
        // the process is not running
        return;
    }

    if(unlikely(!w)) {
        netdata_log_error("pid %d %s was left without a target!", p->pid, p->comm);
        return;
    }

    if(p->openfds_limits_percent > w->max_open_files_percent)
        w->max_open_files_percent = p->openfds_limits_percent;

    w->cutime  += p->cutime;
    w->cstime  += p->cstime;
    w->cgtime  += p->cgtime;
    w->cminflt += p->cminflt;
    w->cmajflt += p->cmajflt;

    w->utime  += p->utime;
    w->stime  += p->stime;
    w->gtime  += p->gtime;
    w->minflt += p->minflt;
    w->majflt += p->majflt;

    // w->rss += p->rss;

    w->status_vmsize   += p->status_vmsize;
    w->status_vmrss    += p->status_vmrss;
    w->status_vmshared += p->status_vmshared;
    w->status_rssfile  += p->status_rssfile;
    w->status_rssshmem += p->status_rssshmem;
    w->status_vmswap   += p->status_vmswap;
    w->status_voluntary_ctxt_switches += p->status_voluntary_ctxt_switches;
    w->status_nonvoluntary_ctxt_switches += p->status_nonvoluntary_ctxt_switches;

    w->io_logical_bytes_read    += p->io_logical_bytes_read;
    w->io_logical_bytes_written += p->io_logical_bytes_written;
    w->io_read_calls            += p->io_read_calls;
    w->io_write_calls           += p->io_write_calls;
    w->io_storage_bytes_read    += p->io_storage_bytes_read;
    w->io_storage_bytes_written += p->io_storage_bytes_written;
    w->io_cancelled_write_bytes += p->io_cancelled_write_bytes;

    w->processes++;
    w->num_threads += p->num_threads;

    if(!w->collected_starttime || p->collected_starttime < w->collected_starttime) w->collected_starttime = p->collected_starttime;
    if(!w->uptime_min || p->uptime < w->uptime_min) w->uptime_min = p->uptime;
    w->uptime_sum += p->uptime;
    if(!w->uptime_max || w->uptime_max < p->uptime) w->uptime_max = p->uptime;

    if(unlikely(debug_enabled || w->debug_enabled)) {
        debug_log_int("aggregating '%s' pid %d on target '%s' utime=" KERNEL_UINT_FORMAT ", stime=" KERNEL_UINT_FORMAT ", gtime=" KERNEL_UINT_FORMAT ", cutime=" KERNEL_UINT_FORMAT ", cstime=" KERNEL_UINT_FORMAT ", cgtime=" KERNEL_UINT_FORMAT ", minflt=" KERNEL_UINT_FORMAT ", majflt=" KERNEL_UINT_FORMAT ", cminflt=" KERNEL_UINT_FORMAT ", cmajflt=" KERNEL_UINT_FORMAT "", p->comm, p->pid, w->name, p->utime, p->stime, p->gtime, p->cutime, p->cstime, p->cgtime, p->minflt, p->majflt, p->cminflt, p->cmajflt);

        struct pid_on_target *pid_on_target = mallocz(sizeof(struct pid_on_target));
        pid_on_target->pid = p->pid;
        pid_on_target->next = w->root_pid;
        w->root_pid = pid_on_target;
    }
}

static inline void post_aggregate_targets(struct target *root) {
    struct target *w;
    for (w = root; w ; w = w->next) {
        if(w->collected_starttime) {
            if (!w->starttime || w->collected_starttime < w->starttime) {
                w->starttime = w->collected_starttime;
            }
        } else {
            w->starttime = 0;
        }
    }
}

static void calculate_netdata_statistics(void) {

    apply_apps_groups_targets_inheritance();

    zero_all_targets(users_root_target);
    zero_all_targets(groups_root_target);
    apps_groups_targets_count = zero_all_targets(apps_groups_root_target);

    // this has to be done, before the cleanup
    struct pid_stat *p = NULL;
    struct target *w = NULL, *o = NULL;

    // concentrate everything on the targets
    for(p = root_of_pids; p ; p = p->next) {

        // --------------------------------------------------------------------
        // apps_groups target

        aggregate_pid_on_target(p->target, p, NULL);


        // --------------------------------------------------------------------
        // user target

        o = p->user_target;
        if(likely(p->user_target && p->user_target->uid == p->uid))
            w = p->user_target;
        else {
            if(unlikely(debug_enabled && p->user_target))
                debug_log("pid %d (%s) switched user from %u (%s) to %u.", p->pid, p->comm, p->user_target->uid, p->user_target->name, p->uid);

            w = p->user_target = get_users_target(p->uid);
        }

        aggregate_pid_on_target(w, p, o);


        // --------------------------------------------------------------------
        // user group target

        o = p->group_target;
        if(likely(p->group_target && p->group_target->gid == p->gid))
            w = p->group_target;
        else {
            if(unlikely(debug_enabled && p->group_target))
                debug_log("pid %d (%s) switched group from %u (%s) to %u.", p->pid, p->comm, p->group_target->gid, p->group_target->name, p->gid);

            w = p->group_target = get_groups_target(p->gid);
        }

        aggregate_pid_on_target(w, p, o);


        // --------------------------------------------------------------------
        // aggregate all file descriptors

        if(enable_file_charts)
            aggregate_pid_fds_on_targets(p);
    }

    post_aggregate_targets(apps_groups_root_target);
    post_aggregate_targets(users_root_target);
    post_aggregate_targets(groups_root_target);

    cleanup_exited_pids();
}

// ----------------------------------------------------------------------------
// update chart dimensions

static inline void send_BEGIN(const char *type, const char *name,const char *metric,  usec_t usec) {
    fprintf(stdout, "BEGIN %s.%s_%s %" PRIu64 "\n", type, name, metric, usec);
}

static inline void send_SET(const char *name, kernel_uint_t value) {
    fprintf(stdout, "SET %s = " KERNEL_UINT_FORMAT "\n", name, value);
}

static inline void send_END(void) {
    fprintf(stdout, "END\n\n");
}

void send_resource_usage_to_netdata(usec_t dt) {
    static struct timeval last = { 0, 0 };
    static struct rusage me_last;

    struct timeval now;
    struct rusage me;

    usec_t cpuuser;
    usec_t cpusyst;

    if(!last.tv_sec) {
        now_monotonic_timeval(&last);
        getrusage(RUSAGE_SELF, &me_last);

        cpuuser = 0;
        cpusyst = 0;
    }
    else {
        now_monotonic_timeval(&now);
        getrusage(RUSAGE_SELF, &me);

        cpuuser = me.ru_utime.tv_sec * USEC_PER_SEC + me.ru_utime.tv_usec;
        cpusyst = me.ru_stime.tv_sec * USEC_PER_SEC + me.ru_stime.tv_usec;

        memmove(&last, &now, sizeof(struct timeval));
        memmove(&me_last, &me, sizeof(struct rusage));
    }

    static char created_charts = 0;
    if(unlikely(!created_charts)) {
        created_charts = 1;

        fprintf(stdout,
                "CHART netdata.apps_cpu '' 'Apps Plugin CPU' 'milliseconds/s' apps.plugin netdata.apps_cpu stacked 140000 %1$d\n"
                "DIMENSION user '' incremental 1 1000\n"
                "DIMENSION system '' incremental 1 1000\n"
                "CHART netdata.apps_sizes '' 'Apps Plugin Files' 'files/s' apps.plugin netdata.apps_sizes line 140001 %1$d\n"
                "DIMENSION calls '' incremental 1 1\n"
                "DIMENSION files '' incremental 1 1\n"
                "DIMENSION filenames '' incremental 1 1\n"
                "DIMENSION inode_changes '' incremental 1 1\n"
                "DIMENSION link_changes '' incremental 1 1\n"
                "DIMENSION pids '' absolute 1 1\n"
                "DIMENSION fds '' absolute 1 1\n"
                "DIMENSION targets '' absolute 1 1\n"
                "DIMENSION new_pids 'new pids' incremental 1 1\n"
                , update_every
        );

        fprintf(stdout,
                "CHART netdata.apps_fix '' 'Apps Plugin Normalization Ratios' 'percentage' apps.plugin netdata.apps_fix line 140002 %1$d\n"
                "DIMENSION utime '' absolute 1 %2$llu\n"
                "DIMENSION stime '' absolute 1 %2$llu\n"
                "DIMENSION gtime '' absolute 1 %2$llu\n"
                "DIMENSION minflt '' absolute 1 %2$llu\n"
                "DIMENSION majflt '' absolute 1 %2$llu\n"
                , update_every
                , RATES_DETAIL
        );

        if(include_exited_childs)
            fprintf(stdout,
                    "CHART netdata.apps_children_fix '' 'Apps Plugin Exited Children Normalization Ratios' 'percentage' apps.plugin netdata.apps_children_fix line 140003 %1$d\n"
                    "DIMENSION cutime '' absolute 1 %2$llu\n"
                    "DIMENSION cstime '' absolute 1 %2$llu\n"
                    "DIMENSION cgtime '' absolute 1 %2$llu\n"
                    "DIMENSION cminflt '' absolute 1 %2$llu\n"
                    "DIMENSION cmajflt '' absolute 1 %2$llu\n"
                    , update_every
                    , RATES_DETAIL
            );

    }

    fprintf(stdout,
        "BEGIN netdata.apps_cpu %"PRIu64"\n"
        "SET user = %"PRIu64"\n"
        "SET system = %"PRIu64"\n"
        "END\n"
        "BEGIN netdata.apps_sizes %"PRIu64"\n"
        "SET calls = %zu\n"
        "SET files = %zu\n"
        "SET filenames = %zu\n"
        "SET inode_changes = %zu\n"
        "SET link_changes = %zu\n"
        "SET pids = %zu\n"
        "SET fds = %d\n"
        "SET targets = %zu\n"
        "SET new_pids = %zu\n"
        "END\n"
        , dt
        , cpuuser
        , cpusyst
        , dt
        , calls_counter
        , file_counter
        , filenames_allocated_counter
        , inodes_changed_counter
        , links_changed_counter
        , all_pids_count
        , all_files_len
        , apps_groups_targets_count
        , targets_assignment_counter
        );

    fprintf(stdout,
            "BEGIN netdata.apps_fix %"PRIu64"\n"
            "SET utime = %u\n"
            "SET stime = %u\n"
            "SET gtime = %u\n"
            "SET minflt = %u\n"
            "SET majflt = %u\n"
            "END\n"
            , dt
            , (unsigned int)(utime_fix_ratio   * 100 * RATES_DETAIL)
            , (unsigned int)(stime_fix_ratio   * 100 * RATES_DETAIL)
            , (unsigned int)(gtime_fix_ratio   * 100 * RATES_DETAIL)
            , (unsigned int)(minflt_fix_ratio  * 100 * RATES_DETAIL)
            , (unsigned int)(majflt_fix_ratio  * 100 * RATES_DETAIL)
    );

    if(include_exited_childs)
        fprintf(stdout,
            "BEGIN netdata.apps_children_fix %"PRIu64"\n"
            "SET cutime = %u\n"
            "SET cstime = %u\n"
            "SET cgtime = %u\n"
            "SET cminflt = %u\n"
            "SET cmajflt = %u\n"
            "END\n"
            , dt
            , (unsigned int)(cutime_fix_ratio  * 100 * RATES_DETAIL)
            , (unsigned int)(cstime_fix_ratio  * 100 * RATES_DETAIL)
            , (unsigned int)(cgtime_fix_ratio  * 100 * RATES_DETAIL)
            , (unsigned int)(cminflt_fix_ratio * 100 * RATES_DETAIL)
            , (unsigned int)(cmajflt_fix_ratio * 100 * RATES_DETAIL)
            );
}

static void normalize_utilization(struct target *root) {
    struct target *w;

    // childs processing introduces spikes
    // here we try to eliminate them by disabling childs processing either for specific dimensions
    // or entirely. Of course, either way, we disable it just a single iteration.

    kernel_uint_t max_time = get_system_cpus() * time_factor * RATES_DETAIL;
    kernel_uint_t utime = 0, cutime = 0, stime = 0, cstime = 0, gtime = 0, cgtime = 0, minflt = 0, cminflt = 0, majflt = 0, cmajflt = 0;

    if(global_utime > max_time) global_utime = max_time;
    if(global_stime > max_time) global_stime = max_time;
    if(global_gtime > max_time) global_gtime = max_time;

    for(w = root; w ; w = w->next) {
        if(w->target || (!w->processes && !w->exposed)) continue;

        utime   += w->utime;
        stime   += w->stime;
        gtime   += w->gtime;
        cutime  += w->cutime;
        cstime  += w->cstime;
        cgtime  += w->cgtime;

        minflt  += w->minflt;
        majflt  += w->majflt;
        cminflt += w->cminflt;
        cmajflt += w->cmajflt;
    }

    if(global_utime || global_stime || global_gtime) {
        if(global_utime + global_stime + global_gtime > utime + cutime + stime + cstime + gtime + cgtime) {
            // everything we collected fits
            utime_fix_ratio  =
            stime_fix_ratio  =
            gtime_fix_ratio  =
            cutime_fix_ratio =
            cstime_fix_ratio =
            cgtime_fix_ratio = 1.0; //(NETDATA_DOUBLE)(global_utime + global_stime) / (NETDATA_DOUBLE)(utime + cutime + stime + cstime);
        }
        else if((global_utime + global_stime > utime + stime) && (cutime || cstime)) {
            // children resources are too high
            // lower only the children resources
            utime_fix_ratio  =
            stime_fix_ratio  =
            gtime_fix_ratio  = 1.0;
            cutime_fix_ratio =
            cstime_fix_ratio =
            cgtime_fix_ratio = (NETDATA_DOUBLE)((global_utime + global_stime) - (utime + stime)) / (NETDATA_DOUBLE)(cutime + cstime);
        }
        else if(utime || stime) {
            // even running processes are unrealistic
            // zero the children resources
            // lower the running processes resources
            utime_fix_ratio  =
            stime_fix_ratio  =
            gtime_fix_ratio  = (NETDATA_DOUBLE)(global_utime + global_stime) / (NETDATA_DOUBLE)(utime + stime);
            cutime_fix_ratio =
            cstime_fix_ratio =
            cgtime_fix_ratio = 0.0;
        }
        else {
            utime_fix_ratio  =
            stime_fix_ratio  =
            gtime_fix_ratio  =
            cutime_fix_ratio =
            cstime_fix_ratio =
            cgtime_fix_ratio = 0.0;
        }
    }
    else {
        utime_fix_ratio  =
        stime_fix_ratio  =
        gtime_fix_ratio  =
        cutime_fix_ratio =
        cstime_fix_ratio =
        cgtime_fix_ratio = 0.0;
    }

    if(utime_fix_ratio  > 1.0) utime_fix_ratio  = 1.0;
    if(cutime_fix_ratio > 1.0) cutime_fix_ratio = 1.0;
    if(stime_fix_ratio  > 1.0) stime_fix_ratio  = 1.0;
    if(cstime_fix_ratio > 1.0) cstime_fix_ratio = 1.0;
    if(gtime_fix_ratio  > 1.0) gtime_fix_ratio  = 1.0;
    if(cgtime_fix_ratio > 1.0) cgtime_fix_ratio = 1.0;

    // if(utime_fix_ratio  < 0.0) utime_fix_ratio  = 0.0;
    // if(cutime_fix_ratio < 0.0) cutime_fix_ratio = 0.0;
    // if(stime_fix_ratio  < 0.0) stime_fix_ratio  = 0.0;
    // if(cstime_fix_ratio < 0.0) cstime_fix_ratio = 0.0;
    // if(gtime_fix_ratio  < 0.0) gtime_fix_ratio  = 0.0;
    // if(cgtime_fix_ratio < 0.0) cgtime_fix_ratio = 0.0;

    // TODO
    // we use cpu time to normalize page faults
    // the problem is that to find the proper max values
    // for page faults we have to parse /proc/vmstat
    // which is quite big to do it again (netdata does it already)
    //
    // a better solution could be to somehow have netdata
    // do this normalization for us

    if(utime || stime || gtime)
        majflt_fix_ratio =
        minflt_fix_ratio = (NETDATA_DOUBLE)(utime * utime_fix_ratio + stime * stime_fix_ratio + gtime * gtime_fix_ratio) / (NETDATA_DOUBLE)(utime + stime + gtime);
    else
        minflt_fix_ratio =
        majflt_fix_ratio = 1.0;

    if(cutime || cstime || cgtime)
        cmajflt_fix_ratio =
        cminflt_fix_ratio = (NETDATA_DOUBLE)(cutime * cutime_fix_ratio + cstime * cstime_fix_ratio + cgtime * cgtime_fix_ratio) / (NETDATA_DOUBLE)(cutime + cstime + cgtime);
    else
        cminflt_fix_ratio =
        cmajflt_fix_ratio = 1.0;

    // the report

    debug_log(
            "SYSTEM: u=" KERNEL_UINT_FORMAT " s=" KERNEL_UINT_FORMAT " g=" KERNEL_UINT_FORMAT " "
            "COLLECTED: u=" KERNEL_UINT_FORMAT " s=" KERNEL_UINT_FORMAT " g=" KERNEL_UINT_FORMAT " cu=" KERNEL_UINT_FORMAT " cs=" KERNEL_UINT_FORMAT " cg=" KERNEL_UINT_FORMAT " "
            "DELTA: u=" KERNEL_UINT_FORMAT " s=" KERNEL_UINT_FORMAT " g=" KERNEL_UINT_FORMAT " "
            "FIX: u=%0.2f s=%0.2f g=%0.2f cu=%0.2f cs=%0.2f cg=%0.2f "
            "FINALLY: u=" KERNEL_UINT_FORMAT " s=" KERNEL_UINT_FORMAT " g=" KERNEL_UINT_FORMAT " cu=" KERNEL_UINT_FORMAT " cs=" KERNEL_UINT_FORMAT " cg=" KERNEL_UINT_FORMAT " "
            , global_utime
            , global_stime
            , global_gtime
            , utime
            , stime
            , gtime
            , cutime
            , cstime
            , cgtime
            , utime + cutime - global_utime
            , stime + cstime - global_stime
            , gtime + cgtime - global_gtime
            , utime_fix_ratio
            , stime_fix_ratio
            , gtime_fix_ratio
            , cutime_fix_ratio
            , cstime_fix_ratio
            , cgtime_fix_ratio
            , (kernel_uint_t)(utime * utime_fix_ratio)
            , (kernel_uint_t)(stime * stime_fix_ratio)
            , (kernel_uint_t)(gtime * gtime_fix_ratio)
            , (kernel_uint_t)(cutime * cutime_fix_ratio)
            , (kernel_uint_t)(cstime * cstime_fix_ratio)
            , (kernel_uint_t)(cgtime * cgtime_fix_ratio)
    );
}

static void send_collected_data_to_netdata(struct target *root, const char *type, usec_t dt) {
    struct target *w;

    for (w = root; w ; w = w->next) {
        if (unlikely(!w->exposed))
            continue;

        send_BEGIN(type, w->clean_name, "processes", dt);
        send_SET("processes", w->processes);
        send_END();

        send_BEGIN(type, w->clean_name, "threads", dt);
        send_SET("threads", w->num_threads);
        send_END();

        if (unlikely(!w->processes && !w->is_other))
            continue;

        send_BEGIN(type, w->clean_name, "cpu_utilization", dt);
        send_SET("user", (kernel_uint_t)(w->utime * utime_fix_ratio) + (include_exited_childs ? ((kernel_uint_t)(w->cutime * cutime_fix_ratio)) : 0ULL));
        send_SET("system", (kernel_uint_t)(w->stime * stime_fix_ratio) + (include_exited_childs ? ((kernel_uint_t)(w->cstime * cstime_fix_ratio)) : 0ULL));
        send_END();

#ifndef __FreeBSD__
        if (enable_guest_charts) {
            send_BEGIN(type, w->clean_name, "cpu_guest_utilization", dt);
            send_SET("guest", (kernel_uint_t)(w->gtime * gtime_fix_ratio) + (include_exited_childs ? ((kernel_uint_t)(w->cgtime * cgtime_fix_ratio)) : 0ULL));
            send_END();
        }

        send_BEGIN(type, w->clean_name, "cpu_context_switches", dt);
        send_SET("voluntary", w->status_voluntary_ctxt_switches);
        send_SET("involuntary", w->status_nonvoluntary_ctxt_switches);
        send_END();

        send_BEGIN(type, w->clean_name, "mem_private_usage", dt);
        send_SET("mem", (w->status_vmrss > w->status_vmshared)?(w->status_vmrss - w->status_vmshared) : 0ULL);
        send_END();
#endif

        send_BEGIN(type, w->clean_name, "mem_usage", dt);
        send_SET("rss", w->status_vmrss);
        send_END();

        send_BEGIN(type, w->clean_name, "vmem_usage", dt);
        send_SET("vmem", w->status_vmsize);
        send_END();

        send_BEGIN(type, w->clean_name, "mem_page_faults", dt);
        send_SET("minor", (kernel_uint_t)(w->minflt * minflt_fix_ratio) + (include_exited_childs ? ((kernel_uint_t)(w->cminflt * cminflt_fix_ratio)) : 0ULL));
        send_SET("major", (kernel_uint_t)(w->majflt * majflt_fix_ratio) + (include_exited_childs ? ((kernel_uint_t)(w->cmajflt * cmajflt_fix_ratio)) : 0ULL));
        send_END();

#ifndef __FreeBSD__
        send_BEGIN(type, w->clean_name, "swap_usage", dt);
        send_SET("swap", w->status_vmswap);
        send_END();
#endif

#ifndef __FreeBSD__
        if (w->processes == 0) {
            send_BEGIN(type, w->clean_name, "uptime", dt);
            send_SET("uptime", 0);
            send_END();

            if (enable_detailed_uptime_charts) {
                send_BEGIN(type, w->clean_name, "uptime_summary", dt);
                send_SET("min", 0);
                send_SET("avg", 0);
                send_SET("max", 0);
                send_END();
            }
        } else {
            send_BEGIN(type, w->clean_name, "uptime", dt);
            send_SET("uptime", (global_uptime > w->starttime) ? (global_uptime - w->starttime) : 0);
            send_END();

            if (enable_detailed_uptime_charts) {
                send_BEGIN(type, w->clean_name, "uptime_summary", dt);
                send_SET("min", w->uptime_min);
                send_SET("avg", w->processes > 0 ? w->uptime_sum / w->processes : 0);
                send_SET("max", w->uptime_max);
                send_END();
            }
        }
#endif

        send_BEGIN(type, w->clean_name, "disk_physical_io", dt);
        send_SET("reads", w->io_storage_bytes_read);
        send_SET("writes", w->io_storage_bytes_written);
        send_END();

#ifndef __FreeBSD__
        send_BEGIN(type, w->clean_name, "disk_logical_io", dt);
        send_SET("reads", w->io_logical_bytes_read);
        send_SET("writes", w->io_logical_bytes_written);
        send_END();
#endif
        if (enable_file_charts) {
            send_BEGIN(type, w->clean_name, "fds_open_limit", dt);
            send_SET("limit", w->max_open_files_percent * 100.0);
            send_END();

            send_BEGIN(type, w->clean_name, "fds_open", dt);
            send_SET("files", w->openfds.files);
            send_SET("sockets", w->openfds.sockets);
            send_SET("pipes", w->openfds.sockets);
            send_SET("inotifies", w->openfds.inotifies);
            send_SET("event", w->openfds.eventfds);
            send_SET("timer", w->openfds.timerfds);
            send_SET("signal", w->openfds.signalfds);
            send_SET("eventpolls", w->openfds.eventpolls);
            send_SET("other", w->openfds.other);
            send_END();
        }
    }
}


// ----------------------------------------------------------------------------
// generate the charts

static void send_charts_updates_to_netdata(struct target *root, const char *type, const char *lbl_name, const char *title)
{
    struct target *w;

    if (debug_enabled) {
        for (w = root; w; w = w->next) {
            if (unlikely(!w->target && w->processes)) {
                struct pid_on_target *pid_on_target;
                fprintf(stderr, "apps.plugin: target '%s' has aggregated %u process(es):", w->name, w->processes);
                for (pid_on_target = w->root_pid; pid_on_target; pid_on_target = pid_on_target->next) {
                    fprintf(stderr, " %d", pid_on_target->pid);
                }
                fputc('\n', stderr);
            }
        }
    }

    for (w = root; w; w = w->next) {
        if (likely(w->exposed || (!w->processes && !w->is_other)))
            continue;

        w->exposed = 1;

        fprintf(stdout, "CHART %s.%s_cpu_utilization '' '%s CPU utilization (100%% = 1 core)' 'percentage' cpu %s.cpu_utilization stacked 20001 %d\n", type, w->clean_name, title, type, update_every);
        fprintf(stdout, "CLABEL '%s' '%s' 1\n", lbl_name, w->name);
        fprintf(stdout, "CLABEL_COMMIT\n");
        fprintf(stdout, "DIMENSION user '' absolute 1 %llu\n", time_factor * RATES_DETAIL / 100LLU);
        fprintf(stdout, "DIMENSION system '' absolute 1 %llu\n", time_factor * RATES_DETAIL / 100LLU);

#ifndef __FreeBSD__
        if (enable_guest_charts) {
            fprintf(stdout, "CHART %s.%s_cpu_guest_utilization '' '%s CPU guest utlization (100%% = 1 core)' 'percentage' cpu %s.cpu_guest_utilization line 20005 %d\n", type, w->clean_name, title, type, update_every);
            fprintf(stdout, "CLABEL '%s' '%s' 1\n", lbl_name, w->name);
            fprintf(stdout, "CLABEL_COMMIT\n");
            fprintf(stdout, "DIMENSION guest '' absolute 1 %llu\n", time_factor * RATES_DETAIL / 100LLU);
        }

        fprintf(stdout, "CHART %s.%s_cpu_context_switches '' '%s CPU context switches' 'switches/s' cpu %s.cpu_context_switches stacked 20010 %d\n", type, w->clean_name, title, type, update_every);
        fprintf(stdout, "CLABEL '%s' '%s' 1\n", lbl_name, w->name);
        fprintf(stdout, "CLABEL_COMMIT\n");
        fprintf(stdout, "DIMENSION voluntary '' absolute 1 %llu\n", RATES_DETAIL);
        fprintf(stdout, "DIMENSION involuntary '' absolute 1 %llu\n", RATES_DETAIL);

        fprintf(stdout, "CHART %s.%s_mem_private_usage '' '%s memory usage without shared' 'MiB' mem %s.mem_private_usage area 20050 %d\n", type, w->clean_name, title, type, update_every);
        fprintf(stdout, "CLABEL '%s' '%s' 1\n", lbl_name, w->name);
        fprintf(stdout, "CLABEL_COMMIT\n");
        fprintf(stdout, "DIMENSION mem '' absolute %ld %ld\n", 1L, 1024L);
#endif

        fprintf(stdout, "CHART %s.%s_mem_usage '' '%s memory RSS usage' 'MiB' mem %s.mem_usage area 20055 %d\n", type, w->clean_name, title, type, update_every);
        fprintf(stdout, "CLABEL '%s' '%s' 1\n", lbl_name, w->name);
        fprintf(stdout, "CLABEL_COMMIT\n");
        fprintf(stdout, "DIMENSION rss '' absolute %ld %ld\n", 1L, 1024L);

        fprintf(stdout, "CHART %s.%s_mem_page_faults '' '%s memory page faults' 'pgfaults/s' mem %s.mem_page_faults stacked 20060 %d\n", type, w->clean_name, title, type, update_every);
        fprintf(stdout, "CLABEL '%s' '%s' 1\n", lbl_name, w->name);
        fprintf(stdout, "CLABEL_COMMIT\n");
        fprintf(stdout, "DIMENSION major '' absolute 1 %llu\n", RATES_DETAIL);
        fprintf(stdout, "DIMENSION minor '' absolute 1 %llu\n", RATES_DETAIL);

        fprintf(stdout, "CHART %s.%s_vmem_usage '' '%s virtual memory size' 'MiB' mem %s.vmem_usage line 20065 %d\n", type, w->clean_name, title, type, update_every);
        fprintf(stdout, "CLABEL '%s' '%s' 1\n", lbl_name, w->name);
        fprintf(stdout, "CLABEL_COMMIT\n");
        fprintf(stdout, "DIMENSION vmem '' absolute %ld %ld\n", 1L, 1024L);

#ifndef __FreeBSD__
        fprintf(stdout, "CHART %s.%s_swap_usage '' '%s swap usage' 'MiB' mem %s.swap_usage area 20065 %d\n", type, w->clean_name, title, type, update_every);
        fprintf(stdout, "CLABEL '%s' '%s' 1\n", lbl_name, w->name);
        fprintf(stdout, "CLABEL_COMMIT\n");
        fprintf(stdout, "DIMENSION swap '' absolute %ld %ld\n", 1L, 1024L);
#endif

#ifndef __FreeBSD__
       fprintf(stdout, "CHART %s.%s_disk_physical_io '' '%s disk physical IO' 'KiB/s' disk %s.disk_physical_io area 20100 %d\n", type, w->clean_name, title, type, update_every);
       fprintf(stdout, "CLABEL '%s' '%s' 1\n", lbl_name, w->name);
       fprintf(stdout, "CLABEL_COMMIT\n");
       fprintf(stdout, "DIMENSION reads '' absolute 1 %llu\n", 1024LLU * RATES_DETAIL);
       fprintf(stdout, "DIMENSION writes '' absolute -1 %llu\n", 1024LLU * RATES_DETAIL);

       fprintf(stdout, "CHART %s.%s_disk_logical_io '' '%s disk logical IO' 'KiB/s' disk %s.disk_logical_io area 20105 %d\n", type, w->clean_name, title, type, update_every);
       fprintf(stdout, "CLABEL '%s' '%s' 1\n", lbl_name, w->name);
       fprintf(stdout, "CLABEL_COMMIT\n");
       fprintf(stdout, "DIMENSION reads '' absolute 1 %llu\n", 1024LLU * RATES_DETAIL);
       fprintf(stdout, "DIMENSION writes '' absolute -1 %llu\n", 1024LLU * RATES_DETAIL);
#else
       fprintf(stdout, "CHART %s.%s_disk_physical_io '' '%s disk physical IO' 'blocks/s' disk %s.disk_physical_block_io area 20100 %d\n", type, w->clean_name, title, type, update_every);
       fprintf(stdout, "CLABEL '%s' '%s' 1\n", lbl_name, w->name);
       fprintf(stdout, "CLABEL_COMMIT\n");
       fprintf(stdout, "DIMENSION reads '' absolute 1 %llu\n", RATES_DETAIL);
       fprintf(stdout, "DIMENSION writes '' absolute -1 %llu\n", RATES_DETAIL);
#endif

        fprintf(stdout, "CHART %s.%s_processes '' '%s processes' 'processes' processes %s.processes line 20150 %d\n", type, w->clean_name, title, type, update_every);
        fprintf(stdout, "CLABEL '%s' '%s' 1\n", lbl_name, w->name);
        fprintf(stdout, "CLABEL_COMMIT\n");
        fprintf(stdout, "DIMENSION processes '' absolute 1 1\n");

        fprintf(stdout, "CHART %s.%s_threads '' '%s threads' 'threads' processes %s.threads line 20155 %d\n", type, w->clean_name, title, type, update_every);
        fprintf(stdout, "CLABEL '%s' '%s' 1\n", lbl_name, w->name);
        fprintf(stdout, "CLABEL_COMMIT\n");
        fprintf(stdout, "DIMENSION threads '' absolute 1 1\n");

       if (enable_file_charts) {
           fprintf(stdout, "CHART %s.%s_fds_open_limit '' '%s open file descriptors limit' '%%' fds %s.fds_open_limit line 20200 %d\n", type, w->clean_name, title, type, update_every);
           fprintf(stdout, "CLABEL '%s' '%s' 1\n", lbl_name, w->name);
           fprintf(stdout, "CLABEL_COMMIT\n");
           fprintf(stdout, "DIMENSION limit '' absolute 1 100\n");

           fprintf(stdout, "CHART %s.%s_fds_open '' '%s open files descriptors' 'fds' fds %s.fds_open stacked 20210 %d\n", type, w->clean_name, title, type, update_every);
           fprintf(stdout, "CLABEL '%s' '%s' 1\n", lbl_name, w->name);
           fprintf(stdout, "CLABEL_COMMIT\n");
           fprintf(stdout, "DIMENSION files '' absolute 1 1\n");
           fprintf(stdout, "DIMENSION sockets '' absolute 1 1\n");
           fprintf(stdout, "DIMENSION pipes '' absolute 1 1\n");
           fprintf(stdout, "DIMENSION inotifies '' absolute 1 1\n");
           fprintf(stdout, "DIMENSION event '' absolute 1 1\n");
           fprintf(stdout, "DIMENSION timer '' absolute 1 1\n");
           fprintf(stdout, "DIMENSION signal '' absolute 1 1\n");
           fprintf(stdout, "DIMENSION eventpolls '' absolute 1 1\n");
           fprintf(stdout, "DIMENSION other '' absolute 1 1\n");
       }

#ifndef __FreeBSD__
       fprintf(stdout, "CHART %s.%s_uptime '' '%s uptime' 'seconds' uptime %s.uptime line 20250 %d\n", type, w->clean_name, title, type, update_every);
       fprintf(stdout, "CLABEL '%s' '%s' 1\n", lbl_name, w->name);
       fprintf(stdout, "CLABEL_COMMIT\n");
       fprintf(stdout, "DIMENSION uptime '' absolute 1 1\n");

       if (enable_detailed_uptime_charts) {
           fprintf(stdout, "CHART %s.%s_uptime_summary '' '%s uptime summary' 'seconds' uptime %s.uptime_summary area 20255 %d\n", type, w->clean_name, title, type, update_every);
           fprintf(stdout, "CLABEL '%s' '%s' 1\n", lbl_name, w->name);
           fprintf(stdout, "CLABEL_COMMIT\n");
           fprintf(stdout, "DIMENSION min '' absolute 1 1\n");
           fprintf(stdout, "DIMENSION avg '' absolute 1 1\n");
           fprintf(stdout, "DIMENSION max '' absolute 1 1\n");
       }
#endif
    }
}

#ifndef __FreeBSD__
static void send_proc_states_count(usec_t dt)
{
    static bool chart_added = false;
    // create chart for count of processes in different states
    if (!chart_added) {
        fprintf(
                stdout,
                "CHART system.processes_state '' 'System Processes State' 'processes' processes system.processes_state line %d %d\n",
                NETDATA_CHART_PRIO_SYSTEM_PROCESS_STATES,
                update_every);
        for (proc_state i = PROC_STATUS_RUNNING; i < PROC_STATUS_END; i++) {
          fprintf(stdout, "DIMENSION %s '' absolute 1 1\n", proc_states[i]);
        }
        chart_added = true;
    }

    // send process state count
    fprintf(stdout, "BEGIN system.processes_state %" PRIu64 "\n", dt);
    for (proc_state i = PROC_STATUS_RUNNING; i < PROC_STATUS_END; i++) {
        send_SET(proc_states[i], proc_state_count[i]);
    }
    send_END();
}
#endif

// ----------------------------------------------------------------------------
// parse command line arguments

int check_proc_1_io() {
    int ret = 0;

    procfile *ff = procfile_open("/proc/1/io", NULL, PROCFILE_FLAG_NO_ERROR_ON_FILE_IO);
    if(!ff) goto cleanup;

    ff = procfile_readall(ff);
    if(!ff) goto cleanup;

    ret = 1;

cleanup:
    procfile_close(ff);
    return ret;
}

static void parse_args(int argc, char **argv)
{
    int i, freq = 0;

    for(i = 1; i < argc; i++) {
        if(!freq) {
            int n = (int)str2l(argv[i]);
            if(n > 0) {
                freq = n;
                continue;
            }
        }

        if(strcmp("version", argv[i]) == 0 || strcmp("-version", argv[i]) == 0 || strcmp("--version", argv[i]) == 0 || strcmp("-v", argv[i]) == 0 || strcmp("-V", argv[i]) == 0) {
            printf("apps.plugin %s\n", VERSION);
            exit(0);
        }

        if(strcmp("test-permissions", argv[i]) == 0 || strcmp("-t", argv[i]) == 0) {
            if(!check_proc_1_io()) {
                perror("Tried to read /proc/1/io and it failed");
                exit(1);
            }
            printf("OK\n");
            exit(0);
        }

        if(strcmp("debug", argv[i]) == 0) {
            debug_enabled = 1;
#ifndef NETDATA_INTERNAL_CHECKS
            fprintf(stderr, "apps.plugin has been compiled without debugging\n");
#endif
            continue;
        }

#ifndef __FreeBSD__
        if(strcmp("fds-cache-secs", argv[i]) == 0) {
            if(argc <= i + 1) {
                fprintf(stderr, "Parameter 'fds-cache-secs' requires a number as argument.\n");
                exit(1);
            }
            i++;
            max_fds_cache_seconds = str2i(argv[i]);
            if(max_fds_cache_seconds < 0) max_fds_cache_seconds = 0;
            continue;
        }
#endif

        if(strcmp("no-childs", argv[i]) == 0 || strcmp("without-childs", argv[i]) == 0) {
            include_exited_childs = 0;
            continue;
        }

        if(strcmp("with-childs", argv[i]) == 0) {
            include_exited_childs = 1;
            continue;
        }

        if(strcmp("with-guest", argv[i]) == 0) {
            enable_guest_charts = 1;
            continue;
        }

        if(strcmp("no-guest", argv[i]) == 0 || strcmp("without-guest", argv[i]) == 0) {
            enable_guest_charts = 0;
            continue;
        }

        if(strcmp("with-files", argv[i]) == 0) {
            enable_file_charts = 1;
            continue;
        }

        if(strcmp("no-files", argv[i]) == 0 || strcmp("without-files", argv[i]) == 0) {
            enable_file_charts = 0;
            continue;
        }

        if(strcmp("no-users", argv[i]) == 0 || strcmp("without-users", argv[i]) == 0) {
            enable_users_charts = 0;
            continue;
        }

        if(strcmp("no-groups", argv[i]) == 0 || strcmp("without-groups", argv[i]) == 0) {
            enable_groups_charts = 0;
            continue;
        }

        if(strcmp("with-detailed-uptime", argv[i]) == 0) {
            enable_detailed_uptime_charts = 1;
            continue;
        }
        if(strcmp("with-function-cmdline", argv[i]) == 0) {
            enable_function_cmdline = 1;
            continue;
        }

        if(strcmp("-h", argv[i]) == 0 || strcmp("--help", argv[i]) == 0) {
            fprintf(stderr,
                    "\n"
                    " netdata apps.plugin %s\n"
                    " Copyright (C) 2016-2017 Costa Tsaousis <costa@tsaousis.gr>\n"
                    " Released under GNU General Public License v3 or later.\n"
                    " All rights reserved.\n"
                    "\n"
                    " This program is a data collector plugin for netdata.\n"
                    "\n"
                    " Available command line options:\n"
                    "\n"
                    " SECONDS                set the data collection frequency\n"
                    "\n"
                    " debug                  enable debugging (lot of output)\n"
                    "\n"
                    " with-function-cmdline  enable reporting the complete command line for processes\n"
                    "                        it includes the command and passed arguments\n"
                    "                        it may include sensitive data such as passwords and tokens\n"
                    "                        enabling this could be a security risk\n"
                    "\n"
                    " with-childs\n"
                    " without-childs         enable / disable aggregating exited\n"
                    "                        children resources into parents\n"
                    "                        (default is enabled)\n"
                    "\n"
                    " with-guest\n"
                    " without-guest          enable / disable reporting guest charts\n"
                    "                        (default is disabled)\n"
                    "\n"
                    " with-files\n"
                    " without-files          enable / disable reporting files, sockets, pipes\n"
                    "                        (default is enabled)\n"
                    "\n"
                    " without-users          disable reporting per user charts\n"
                    "\n"
                    " without-groups         disable reporting per user group charts\n"
                    "\n"
                    " with-detailed-uptime   enable reporting min/avg/max uptime charts\n"
                    "\n"
#ifndef __FreeBSD__
                    " fds-cache-secs N       cache the files of processed for N seconds\n"
                    "                        caching is adaptive per file (when a file\n"
                    "                        is found, it starts at 0 and while the file\n"
                    "                        remains open, it is incremented up to the\n"
                    "                        max given)\n"
                    "                        (default is %d seconds)\n"
                    "\n"
#endif
                    " version or -v or -V print program version and exit\n"
                    "\n"
                    , VERSION
#ifndef __FreeBSD__
                    , max_fds_cache_seconds
#endif
            );
            exit(1);
        }

        netdata_log_error("Cannot understand option %s", argv[i]);
        exit(1);
    }

    if(freq > 0) update_every = freq;

    if(read_apps_groups_conf(user_config_dir, "groups")) {
        netdata_log_info("Cannot read process groups configuration file '%s/apps_groups.conf'. Will try '%s/apps_groups.conf'", user_config_dir, stock_config_dir);

        if(read_apps_groups_conf(stock_config_dir, "groups")) {
            netdata_log_error("Cannot read process groups '%s/apps_groups.conf'. There are no internal defaults. Failing.", stock_config_dir);
            exit(1);
        }
        else
            netdata_log_info("Loaded config file '%s/apps_groups.conf'", stock_config_dir);
    }
    else
        netdata_log_info("Loaded config file '%s/apps_groups.conf'", user_config_dir);
}

static int am_i_running_as_root() {
    uid_t uid = getuid(), euid = geteuid();

    if(uid == 0 || euid == 0) {
        if(debug_enabled) netdata_log_info("I am running with escalated privileges, uid = %u, euid = %u.", uid, euid);
        return 1;
    }

    if(debug_enabled) netdata_log_info("I am not running with escalated privileges, uid = %u, euid = %u.", uid, euid);
    return 0;
}

#ifdef HAVE_SYS_CAPABILITY_H
static int check_capabilities() {
    cap_t caps = cap_get_proc();
    if(!caps) {
        netdata_log_error("Cannot get current capabilities.");
        return 0;
    }
    else if(debug_enabled)
        netdata_log_info("Received my capabilities from the system.");

    int ret = 1;

    cap_flag_value_t cfv = CAP_CLEAR;
    if(cap_get_flag(caps, CAP_DAC_READ_SEARCH, CAP_EFFECTIVE, &cfv) == -1) {
        netdata_log_error("Cannot find if CAP_DAC_READ_SEARCH is effective.");
        ret = 0;
    }
    else {
        if(cfv != CAP_SET) {
            netdata_log_error("apps.plugin should run with CAP_DAC_READ_SEARCH.");
            ret = 0;
        }
        else if(debug_enabled)
            netdata_log_info("apps.plugin runs with CAP_DAC_READ_SEARCH.");
    }

    cfv = CAP_CLEAR;
    if(cap_get_flag(caps, CAP_SYS_PTRACE, CAP_EFFECTIVE, &cfv) == -1) {
        netdata_log_error("Cannot find if CAP_SYS_PTRACE is effective.");
        ret = 0;
    }
    else {
        if(cfv != CAP_SET) {
            netdata_log_error("apps.plugin should run with CAP_SYS_PTRACE.");
            ret = 0;
        }
        else if(debug_enabled)
            netdata_log_info("apps.plugin runs with CAP_SYS_PTRACE.");
    }

    cap_free(caps);

    return ret;
}
#else
static int check_capabilities() {
    return 0;
}
#endif

static netdata_mutex_t apps_and_stdout_mutex = NETDATA_MUTEX_INITIALIZER;

#define PROCESS_FILTER_CATEGORY "category:"
#define PROCESS_FILTER_USER "user:"
#define PROCESS_FILTER_GROUP "group:"
#define PROCESS_FILTER_PROCESS "process:"
#define PROCESS_FILTER_PID "pid:"
#define PROCESS_FILTER_UID "uid:"
#define PROCESS_FILTER_GID "gid:"

static struct target *find_target_by_name(struct target *base, const char *name) {
    struct target *t;
    for(t = base; t ; t = t->next) {
        if (strcmp(t->name, name) == 0)
            return t;
    }

    return NULL;
}

static kernel_uint_t MemTotal = 0;

static void get_MemTotal(void) {
#ifdef __FreeBSD__
    // TODO - fix this for FreeBSD
     return;
#else
    char filename[FILENAME_MAX + 1];
    snprintfz(filename, FILENAME_MAX, "%s/proc/meminfo", netdata_configured_host_prefix);

    procfile *ff = procfile_open(filename, ": \t", PROCFILE_FLAG_DEFAULT);
    if(!ff)
        return;

    ff = procfile_readall(ff);
    if(!ff)
        return;

    size_t line, lines = procfile_lines(ff);

    for(line = 0; line < lines ;line++) {
        size_t words = procfile_linewords(ff, line);
        if(words == 3 && strcmp(procfile_lineword(ff, line, 0), "MemTotal") == 0 && strcmp(procfile_lineword(ff, line, 2), "kB") == 0) {
            kernel_uint_t n = str2ull(procfile_lineword(ff, line, 1), NULL);
            if(n) MemTotal = n;
            break;
        }
    }

    procfile_close(ff);
#endif
}

static void apps_plugin_function_processes_help(const char *transaction) {
    BUFFER *wb = buffer_create(0, NULL);
    buffer_sprintf(wb, "%s",
            "apps.plugin / processes\n"
            "\n"
            "Function `processes` presents all the currently running processes of the system.\n"
            "\n"
            "The following filters are supported:\n"
            "\n"
            "   category:NAME\n"
            "      Shows only processes that are assigned the category `NAME` in apps_groups.conf\n"
            "\n"
            "   user:NAME\n"
            "      Shows only processes that are running as user name `NAME`.\n"
            "\n"
            "   group:NAME\n"
            "      Shows only processes that are running as group name `NAME`.\n"
            "\n"
            "   process:NAME\n"
            "      Shows only processes that their Command is `NAME` or their parent's Command is `NAME`.\n"
            "\n"
            "   pid:NUMBER\n"
            "      Shows only processes that their PID is `NUMBER` or their parent's PID is `NUMBER`\n"
            "\n"
            "   uid:NUMBER\n"
            "      Shows only processes that their UID is `NUMBER`\n"
            "\n"
            "   gid:NUMBER\n"
            "      Shows only processes that their GID is `NUMBER`\n"
            "\n"
            "Filters can be combined. Each filter can be given only one time.\n"
            );

    pluginsd_function_result_to_stdout(transaction, HTTP_RESP_OK, "text/plain", now_realtime_sec() + 3600, wb);
    buffer_free(wb);
}

#define add_value_field_llu_with_max(wb, key, value) do {                                                       \
    unsigned long long _tmp = (value);                                                                          \
    key ## _max = (rows == 0) ? (_tmp) : MAX(key ## _max, _tmp);                                                \
    buffer_json_add_array_item_uint64(wb, _tmp);                                                                \
} while(0)

#define add_value_field_ndd_with_max(wb, key, value) do {                                                       \
    NETDATA_DOUBLE _tmp = (value);                                                                              \
    key ## _max = (rows == 0) ? (_tmp) : MAX(key ## _max, _tmp);                                                \
    buffer_json_add_array_item_double(wb, _tmp);                                                                \
} while(0)

static void function_processes(const char *transaction, char *function __maybe_unused,
                               usec_t *stop_monotonic_ut __maybe_unused, bool *cancelled __maybe_unused,
                               BUFFER *payload __maybe_unused, HTTP_ACCESS access,
                               const char *source __maybe_unused, void *data __maybe_unused) {
    struct pid_stat *p;

    bool show_cmdline = http_access_user_has_enough_access_level_for_endpoint(
                            access, HTTP_ACCESS_SIGNED_ID | HTTP_ACCESS_SAME_SPACE |
                                        HTTP_ACCESS_SENSITIVE_DATA | HTTP_ACCESS_VIEW_AGENT_CONFIG) ||
                        enable_function_cmdline;

    char *words[PLUGINSD_MAX_WORDS] = { NULL };
    size_t num_words = quoted_strings_splitter_pluginsd(function, words, PLUGINSD_MAX_WORDS);

    struct target *category = NULL, *user = NULL, *group = NULL;
    const char *process_name = NULL;
    pid_t pid = 0;
    uid_t uid = 0;
    gid_t gid = 0;

    bool filter_pid = false, filter_uid = false, filter_gid = false;

    for(int i = 1; i < PLUGINSD_MAX_WORDS ;i++) {
        const char *keyword = get_word(words, num_words, i);
        if(!keyword) break;

        if(!category && strncmp(keyword, PROCESS_FILTER_CATEGORY, strlen(PROCESS_FILTER_CATEGORY)) == 0) {
            category = find_target_by_name(apps_groups_root_target, &keyword[strlen(PROCESS_FILTER_CATEGORY)]);
            if(!category) {
                pluginsd_function_json_error_to_stdout(transaction, HTTP_RESP_BAD_REQUEST,
                                                       "No category with that name found.");
                return;
            }
        }
        else if(!user && strncmp(keyword, PROCESS_FILTER_USER, strlen(PROCESS_FILTER_USER)) == 0) {
            user = find_target_by_name(users_root_target, &keyword[strlen(PROCESS_FILTER_USER)]);
            if(!user) {
                pluginsd_function_json_error_to_stdout(transaction, HTTP_RESP_BAD_REQUEST,
                                                       "No user with that name found.");
                return;
            }
        }
        else if(strncmp(keyword, PROCESS_FILTER_GROUP, strlen(PROCESS_FILTER_GROUP)) == 0) {
            group = find_target_by_name(groups_root_target, &keyword[strlen(PROCESS_FILTER_GROUP)]);
            if(!group) {
                pluginsd_function_json_error_to_stdout(transaction, HTTP_RESP_BAD_REQUEST,
                                                       "No group with that name found.");
                return;
            }
        }
        else if(!process_name && strncmp(keyword, PROCESS_FILTER_PROCESS, strlen(PROCESS_FILTER_PROCESS)) == 0) {
            process_name = &keyword[strlen(PROCESS_FILTER_PROCESS)];
        }
        else if(!pid && strncmp(keyword, PROCESS_FILTER_PID, strlen(PROCESS_FILTER_PID)) == 0) {
            pid = str2i(&keyword[strlen(PROCESS_FILTER_PID)]);
            filter_pid = true;
        }
        else if(!uid && strncmp(keyword, PROCESS_FILTER_UID, strlen(PROCESS_FILTER_UID)) == 0) {
            uid = str2i(&keyword[strlen(PROCESS_FILTER_UID)]);
            filter_uid = true;
        }
        else if(!gid && strncmp(keyword, PROCESS_FILTER_GID, strlen(PROCESS_FILTER_GID)) == 0) {
            gid = str2i(&keyword[strlen(PROCESS_FILTER_GID)]);
            filter_gid = true;
        }
        else if(strcmp(keyword, "help") == 0) {
            apps_plugin_function_processes_help(transaction);
            return;
        }
        else {
            char msg[1024];
            snprintfz(msg, sizeof(msg), "Invalid parameter '%s'", keyword);
            pluginsd_function_json_error_to_stdout(transaction, HTTP_RESP_BAD_REQUEST, msg);
            return;
        }
    }

    time_t expires = now_realtime_sec() + update_every;

    unsigned int cpu_divisor = time_factor * RATES_DETAIL / 100;
    unsigned int memory_divisor = 1024;
    unsigned int io_divisor = 1024 * RATES_DETAIL;

    BUFFER *wb = buffer_create(4096, NULL);
    buffer_json_initialize(wb, "\"", "\"", 0, true, BUFFER_JSON_OPTIONS_NEWLINE_ON_ARRAY_ITEMS);
    buffer_json_member_add_uint64(wb, "status", HTTP_RESP_OK);
    buffer_json_member_add_string(wb, "type", "table");
    buffer_json_member_add_time_t(wb, "update_every", update_every);
    buffer_json_member_add_string(wb, "help", APPS_PLUGIN_PROCESSES_FUNCTION_DESCRIPTION);
    buffer_json_member_add_array(wb, "data");

    NETDATA_DOUBLE
              UserCPU_max = 0.0
            , SysCPU_max = 0.0
            , GuestCPU_max = 0.0
            , CUserCPU_max = 0.0
            , CSysCPU_max = 0.0
            , CGuestCPU_max = 0.0
            , CPU_max = 0.0
            , VMSize_max = 0.0
            , RSS_max = 0.0
            , Shared_max = 0.0
            , Swap_max = 0.0
            , Memory_max = 0.0
            , FDsLimitPercent_max = 0.0
            ;

    unsigned long long
              Processes_max = 0
            , Threads_max = 0
            , VoluntaryCtxtSwitches_max = 0
            , NonVoluntaryCtxtSwitches_max = 0
            , Uptime_max = 0
            , MinFlt_max = 0
            , CMinFlt_max = 0
            , TMinFlt_max = 0
            , MajFlt_max = 0
            , CMajFlt_max = 0
            , TMajFlt_max = 0
            , PReads_max = 0
            , PWrites_max = 0
            , RCalls_max = 0
            , WCalls_max = 0
            , Files_max = 0
            , Pipes_max = 0
            , Sockets_max = 0
            , iNotiFDs_max = 0
            , EventFDs_max = 0
            , TimerFDs_max = 0
            , SigFDs_max = 0
            , EvPollFDs_max = 0
            , OtherFDs_max = 0
            , FDs_max = 0
            ;

#ifndef __FreeBSD__
    unsigned long long
              LReads_max = 0
            , LWrites_max = 0
            ;
#endif

    int rows= 0;
    for(p = root_of_pids; p ; p = p->next) {
        if(!p->updated)
            continue;

        if(category && p->target != category)
            continue;

        if(user && p->user_target != user)
            continue;

        if(group && p->group_target != group)
            continue;

        if(process_name && ((strcmp(p->comm, process_name) != 0 && !p->parent) || (p->parent && strcmp(p->comm, process_name) != 0 && strcmp(p->parent->comm, process_name) != 0)))
            continue;

        if(filter_pid && p->pid != pid && p->ppid != pid)
            continue;

        if(filter_uid && p->uid != uid)
            continue;

        if(filter_gid && p->gid != gid)
            continue;

        rows++;

        buffer_json_add_array_item_array(wb); // for each pid

        // IMPORTANT!
        // THE ORDER SHOULD BE THE SAME WITH THE FIELDS!

        // pid
        buffer_json_add_array_item_uint64(wb, p->pid);

        // cmd
        buffer_json_add_array_item_string(wb, p->comm);

        // cmdline
        if (show_cmdline) {
            buffer_json_add_array_item_string(wb, (p->cmdline && *p->cmdline) ? p->cmdline : p->comm);
        }

        // ppid
        buffer_json_add_array_item_uint64(wb, p->ppid);

        // category
        buffer_json_add_array_item_string(wb, p->target ? p->target->name : "-");

        // user
        buffer_json_add_array_item_string(wb, p->user_target ? p->user_target->name : "-");

        // uid
        buffer_json_add_array_item_uint64(wb, p->uid);

        // group
        buffer_json_add_array_item_string(wb, p->group_target ? p->group_target->name : "-");

        // gid
        buffer_json_add_array_item_uint64(wb, p->gid);

        // CPU utilization %
        add_value_field_ndd_with_max(wb, CPU, (NETDATA_DOUBLE)(p->utime + p->stime + p->gtime + p->cutime + p->cstime + p->cgtime) / cpu_divisor);
        add_value_field_ndd_with_max(wb, UserCPU, (NETDATA_DOUBLE)(p->utime) / cpu_divisor);
        add_value_field_ndd_with_max(wb, SysCPU, (NETDATA_DOUBLE)(p->stime) / cpu_divisor);
        add_value_field_ndd_with_max(wb, GuestCPU, (NETDATA_DOUBLE)(p->gtime) / cpu_divisor);
        add_value_field_ndd_with_max(wb, CUserCPU, (NETDATA_DOUBLE)(p->cutime) / cpu_divisor);
        add_value_field_ndd_with_max(wb, CSysCPU, (NETDATA_DOUBLE)(p->cstime) / cpu_divisor);
        add_value_field_ndd_with_max(wb, CGuestCPU, (NETDATA_DOUBLE)(p->cgtime) / cpu_divisor);

        add_value_field_llu_with_max(wb, VoluntaryCtxtSwitches, p->status_voluntary_ctxt_switches / RATES_DETAIL);
        add_value_field_llu_with_max(wb, NonVoluntaryCtxtSwitches, p->status_nonvoluntary_ctxt_switches / RATES_DETAIL);

        // memory MiB
        if(MemTotal)
            add_value_field_ndd_with_max(wb, Memory, (NETDATA_DOUBLE)p->status_vmrss * 100.0 / (NETDATA_DOUBLE)MemTotal);

        add_value_field_ndd_with_max(wb, RSS, (NETDATA_DOUBLE)p->status_vmrss / memory_divisor);
        add_value_field_ndd_with_max(wb, Shared, (NETDATA_DOUBLE)p->status_vmshared / memory_divisor);
        add_value_field_ndd_with_max(wb, VMSize, (NETDATA_DOUBLE)p->status_vmsize / memory_divisor);
        add_value_field_ndd_with_max(wb, Swap, (NETDATA_DOUBLE)p->status_vmswap / memory_divisor);

        // Physical I/O
        add_value_field_llu_with_max(wb, PReads, p->io_storage_bytes_read / io_divisor);
        add_value_field_llu_with_max(wb, PWrites, p->io_storage_bytes_written / io_divisor);

        // Logical I/O
#ifndef __FreeBSD__
        add_value_field_llu_with_max(wb, LReads, p->io_logical_bytes_read / io_divisor);
        add_value_field_llu_with_max(wb, LWrites, p->io_logical_bytes_written / io_divisor);
#endif

        // I/O calls
        add_value_field_llu_with_max(wb, RCalls, p->io_read_calls / RATES_DETAIL);
        add_value_field_llu_with_max(wb, WCalls, p->io_write_calls / RATES_DETAIL);

        // minor page faults
        add_value_field_llu_with_max(wb, MinFlt, p->minflt / RATES_DETAIL);
        add_value_field_llu_with_max(wb, CMinFlt, p->cminflt / RATES_DETAIL);
        add_value_field_llu_with_max(wb, TMinFlt, (p->minflt + p->cminflt) / RATES_DETAIL);

        // major page faults
        add_value_field_llu_with_max(wb, MajFlt, p->majflt / RATES_DETAIL);
        add_value_field_llu_with_max(wb, CMajFlt, p->cmajflt / RATES_DETAIL);
        add_value_field_llu_with_max(wb, TMajFlt, (p->majflt + p->cmajflt) / RATES_DETAIL);

        // open file descriptors
        add_value_field_ndd_with_max(wb, FDsLimitPercent, p->openfds_limits_percent);
        add_value_field_llu_with_max(wb, FDs, pid_openfds_sum(p));
        add_value_field_llu_with_max(wb, Files, p->openfds.files);
        add_value_field_llu_with_max(wb, Pipes, p->openfds.pipes);
        add_value_field_llu_with_max(wb, Sockets, p->openfds.sockets);
        add_value_field_llu_with_max(wb, iNotiFDs, p->openfds.inotifies);
        add_value_field_llu_with_max(wb, EventFDs, p->openfds.eventfds);
        add_value_field_llu_with_max(wb, TimerFDs, p->openfds.timerfds);
        add_value_field_llu_with_max(wb, SigFDs, p->openfds.signalfds);
        add_value_field_llu_with_max(wb, EvPollFDs, p->openfds.eventpolls);
        add_value_field_llu_with_max(wb, OtherFDs, p->openfds.other);


        // processes, threads, uptime
        add_value_field_llu_with_max(wb, Processes, p->children_count);
        add_value_field_llu_with_max(wb, Threads, p->num_threads);
        add_value_field_llu_with_max(wb, Uptime, p->uptime);

        buffer_json_array_close(wb); // for each pid
    }

    buffer_json_array_close(wb); // data
    buffer_json_member_add_object(wb, "columns");

    {
        int field_id = 0;

        // IMPORTANT!
        // THE ORDER SHOULD BE THE SAME WITH THE VALUES!
        // wb, key, name, visible, type, visualization, transform, decimal_points, units, max, sort, sortable, sticky, unique_key, pointer_to, summary, range
        buffer_rrdf_table_add_field(wb, field_id++, "PID", "Process ID", RRDF_FIELD_TYPE_INTEGER,
                                    RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NUMBER, 0, NULL, NAN,
                                    RRDF_FIELD_SORT_ASCENDING, NULL, RRDF_FIELD_SUMMARY_COUNT,
                                    RRDF_FIELD_FILTER_MULTISELECT,
                                    RRDF_FIELD_OPTS_VISIBLE | RRDF_FIELD_OPTS_STICKY |
                                    RRDF_FIELD_OPTS_UNIQUE_KEY, NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "Cmd", "Process Name", RRDF_FIELD_TYPE_STRING,
                                    RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NONE, 0, NULL, NAN,
                                    RRDF_FIELD_SORT_ASCENDING, NULL, RRDF_FIELD_SUMMARY_COUNT,
                                    RRDF_FIELD_FILTER_MULTISELECT,
                                    RRDF_FIELD_OPTS_VISIBLE | RRDF_FIELD_OPTS_STICKY, NULL);

        if (show_cmdline) {
            buffer_rrdf_table_add_field(wb, field_id++, "CmdLine", "Command Line", RRDF_FIELD_TYPE_STRING,
                                        RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NONE, 0,
                                        NULL, NAN, RRDF_FIELD_SORT_ASCENDING, NULL, RRDF_FIELD_SUMMARY_COUNT,
                                        RRDF_FIELD_FILTER_MULTISELECT,
                                        RRDF_FIELD_OPTS_NONE, NULL);
        }

        buffer_rrdf_table_add_field(wb, field_id++, "PPID", "Parent Process ID", RRDF_FIELD_TYPE_INTEGER,
                                    RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NUMBER, 0, NULL,
                                    NAN, RRDF_FIELD_SORT_ASCENDING, "PID", RRDF_FIELD_SUMMARY_COUNT,
                                    RRDF_FIELD_FILTER_MULTISELECT,
                                    RRDF_FIELD_OPTS_NONE, NULL);
        buffer_rrdf_table_add_field(wb, field_id++, "Category", "Category (apps_groups.conf)", RRDF_FIELD_TYPE_STRING,
                                    RRDF_FIELD_VISUAL_VALUE,
                                    RRDF_FIELD_TRANSFORM_NONE,
                                    0, NULL, NAN, RRDF_FIELD_SORT_ASCENDING, NULL, RRDF_FIELD_SUMMARY_COUNT,
                                    RRDF_FIELD_FILTER_MULTISELECT,
                                    RRDF_FIELD_OPTS_VISIBLE | RRDF_FIELD_OPTS_STICKY, NULL);
        buffer_rrdf_table_add_field(wb, field_id++, "User", "User Owner", RRDF_FIELD_TYPE_STRING,
                                    RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NONE, 0, NULL, NAN,
                                    RRDF_FIELD_SORT_ASCENDING, NULL, RRDF_FIELD_SUMMARY_COUNT,
                                    RRDF_FIELD_FILTER_MULTISELECT,
                                    RRDF_FIELD_OPTS_VISIBLE, NULL);
        buffer_rrdf_table_add_field(wb, field_id++, "Uid", "User ID", RRDF_FIELD_TYPE_INTEGER, RRDF_FIELD_VISUAL_VALUE,
                                    RRDF_FIELD_TRANSFORM_NUMBER, 0, NULL, NAN,
                                    RRDF_FIELD_SORT_ASCENDING, NULL, RRDF_FIELD_SUMMARY_COUNT,
                                    RRDF_FIELD_FILTER_MULTISELECT,
                                    RRDF_FIELD_OPTS_NONE, NULL);
        buffer_rrdf_table_add_field(wb, field_id++, "Group", "Group Owner", RRDF_FIELD_TYPE_STRING,
                                    RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NONE, 0, NULL, NAN,
                                    RRDF_FIELD_SORT_ASCENDING, NULL, RRDF_FIELD_SUMMARY_COUNT,
                                    RRDF_FIELD_FILTER_MULTISELECT,
                                    RRDF_FIELD_OPTS_NONE, NULL);
        buffer_rrdf_table_add_field(wb, field_id++, "Gid", "Group ID", RRDF_FIELD_TYPE_INTEGER, RRDF_FIELD_VISUAL_VALUE,
                                    RRDF_FIELD_TRANSFORM_NUMBER, 0, NULL, NAN,
                                    RRDF_FIELD_SORT_ASCENDING, NULL, RRDF_FIELD_SUMMARY_COUNT,
                                    RRDF_FIELD_FILTER_MULTISELECT,
                                    RRDF_FIELD_OPTS_NONE, NULL);

        // CPU utilization
        buffer_rrdf_table_add_field(wb, field_id++, "CPU", "Total CPU Time (100% = 1 core)",
                                    RRDF_FIELD_TYPE_BAR_WITH_INTEGER, RRDF_FIELD_VISUAL_BAR,
                                    RRDF_FIELD_TRANSFORM_NUMBER, 2, "%", CPU_max, RRDF_FIELD_SORT_DESCENDING, NULL,
                                    RRDF_FIELD_SUMMARY_SUM, RRDF_FIELD_FILTER_RANGE,
                                    RRDF_FIELD_OPTS_VISIBLE, NULL);
        buffer_rrdf_table_add_field(wb, field_id++, "UserCPU", "User CPU time (100% = 1 core)",
                                    RRDF_FIELD_TYPE_BAR_WITH_INTEGER,
                                    RRDF_FIELD_VISUAL_BAR, RRDF_FIELD_TRANSFORM_NUMBER, 2, "%", UserCPU_max,
                                    RRDF_FIELD_SORT_DESCENDING, NULL, RRDF_FIELD_SUMMARY_SUM, RRDF_FIELD_FILTER_RANGE,
                                    RRDF_FIELD_OPTS_NONE, NULL);
        buffer_rrdf_table_add_field(wb, field_id++, "SysCPU", "System CPU Time (100% = 1 core)",
                                    RRDF_FIELD_TYPE_BAR_WITH_INTEGER,
                                    RRDF_FIELD_VISUAL_BAR, RRDF_FIELD_TRANSFORM_NUMBER, 2, "%", SysCPU_max,
                                    RRDF_FIELD_SORT_DESCENDING, NULL, RRDF_FIELD_SUMMARY_SUM, RRDF_FIELD_FILTER_RANGE,
                                    RRDF_FIELD_OPTS_NONE, NULL);
        buffer_rrdf_table_add_field(wb, field_id++, "GuestCPU", "Guest CPU Time (100% = 1 core)",
                                    RRDF_FIELD_TYPE_BAR_WITH_INTEGER,
                                    RRDF_FIELD_VISUAL_BAR, RRDF_FIELD_TRANSFORM_NUMBER, 2, "%", GuestCPU_max,
                                    RRDF_FIELD_SORT_DESCENDING, NULL, RRDF_FIELD_SUMMARY_SUM, RRDF_FIELD_FILTER_RANGE,
                                    RRDF_FIELD_OPTS_NONE, NULL);
        buffer_rrdf_table_add_field(wb, field_id++, "CUserCPU", "Children User CPU Time (100% = 1 core)",
                                    RRDF_FIELD_TYPE_BAR_WITH_INTEGER, RRDF_FIELD_VISUAL_BAR,
                                    RRDF_FIELD_TRANSFORM_NUMBER, 2, "%", CUserCPU_max, RRDF_FIELD_SORT_DESCENDING, NULL,
                                    RRDF_FIELD_SUMMARY_SUM, RRDF_FIELD_FILTER_RANGE,
                                    RRDF_FIELD_OPTS_NONE, NULL);
        buffer_rrdf_table_add_field(wb, field_id++, "CSysCPU", "Children System CPU Time (100% = 1 core)",
                                    RRDF_FIELD_TYPE_BAR_WITH_INTEGER, RRDF_FIELD_VISUAL_BAR,
                                    RRDF_FIELD_TRANSFORM_NUMBER, 2, "%", CSysCPU_max, RRDF_FIELD_SORT_DESCENDING, NULL,
                                    RRDF_FIELD_SUMMARY_SUM, RRDF_FIELD_FILTER_RANGE,
                                    RRDF_FIELD_OPTS_NONE, NULL);
        buffer_rrdf_table_add_field(wb, field_id++, "CGuestCPU", "Children Guest CPU Time (100% = 1 core)",
                                    RRDF_FIELD_TYPE_BAR_WITH_INTEGER, RRDF_FIELD_VISUAL_BAR,
                                    RRDF_FIELD_TRANSFORM_NUMBER, 2, "%", CGuestCPU_max, RRDF_FIELD_SORT_DESCENDING,
                                    NULL,
                                    RRDF_FIELD_SUMMARY_SUM, RRDF_FIELD_FILTER_RANGE, RRDF_FIELD_OPTS_NONE, NULL);

        // CPU context switches
        buffer_rrdf_table_add_field(wb, field_id++, "vCtxSwitch", "Voluntary Context Switches",
                                    RRDF_FIELD_TYPE_BAR_WITH_INTEGER,
                                    RRDF_FIELD_VISUAL_BAR, RRDF_FIELD_TRANSFORM_NUMBER, 2, "switches/s",
                                    VoluntaryCtxtSwitches_max, RRDF_FIELD_SORT_DESCENDING, NULL,
                                    RRDF_FIELD_SUMMARY_SUM, RRDF_FIELD_FILTER_RANGE, RRDF_FIELD_OPTS_NONE, NULL);
        buffer_rrdf_table_add_field(wb, field_id++, "iCtxSwitch", "Involuntary Context Switches",
                                    RRDF_FIELD_TYPE_BAR_WITH_INTEGER,
                                    RRDF_FIELD_VISUAL_BAR, RRDF_FIELD_TRANSFORM_NUMBER, 2, "switches/s",
                                    NonVoluntaryCtxtSwitches_max, RRDF_FIELD_SORT_DESCENDING, NULL,
                                    RRDF_FIELD_SUMMARY_SUM, RRDF_FIELD_FILTER_RANGE, RRDF_FIELD_OPTS_NONE, NULL);

        // memory
        if (MemTotal)
            buffer_rrdf_table_add_field(wb, field_id++, "Memory", "Memory Percentage", RRDF_FIELD_TYPE_BAR_WITH_INTEGER,
                                        RRDF_FIELD_VISUAL_BAR,
                                        RRDF_FIELD_TRANSFORM_NUMBER, 2, "%", 100.0, RRDF_FIELD_SORT_DESCENDING, NULL,
                                        RRDF_FIELD_SUMMARY_SUM, RRDF_FIELD_FILTER_RANGE,
                                        RRDF_FIELD_OPTS_VISIBLE, NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "Resident", "Resident Set Size", RRDF_FIELD_TYPE_BAR_WITH_INTEGER,
                                    RRDF_FIELD_VISUAL_BAR,
                                    RRDF_FIELD_TRANSFORM_NUMBER,
                                    2, "MiB", RSS_max, RRDF_FIELD_SORT_DESCENDING, NULL, RRDF_FIELD_SUMMARY_SUM,
                                    RRDF_FIELD_FILTER_RANGE,
                                    RRDF_FIELD_OPTS_VISIBLE, NULL);
        buffer_rrdf_table_add_field(wb, field_id++, "Shared", "Shared Pages", RRDF_FIELD_TYPE_BAR_WITH_INTEGER,
                                    RRDF_FIELD_VISUAL_BAR, RRDF_FIELD_TRANSFORM_NUMBER, 2,
                                    "MiB", Shared_max, RRDF_FIELD_SORT_DESCENDING, NULL, RRDF_FIELD_SUMMARY_SUM,
                                    RRDF_FIELD_FILTER_RANGE,
                                    RRDF_FIELD_OPTS_VISIBLE, NULL);
        buffer_rrdf_table_add_field(wb, field_id++, "Virtual", "Virtual Memory Size", RRDF_FIELD_TYPE_BAR_WITH_INTEGER,
                                    RRDF_FIELD_VISUAL_BAR,
                                    RRDF_FIELD_TRANSFORM_NUMBER, 2, "MiB", VMSize_max, RRDF_FIELD_SORT_DESCENDING, NULL,
                                    RRDF_FIELD_SUMMARY_SUM, RRDF_FIELD_FILTER_RANGE,
                                    RRDF_FIELD_OPTS_VISIBLE, NULL);
        buffer_rrdf_table_add_field(wb, field_id++, "Swap", "Swap Memory", RRDF_FIELD_TYPE_BAR_WITH_INTEGER,
                                    RRDF_FIELD_VISUAL_BAR, RRDF_FIELD_TRANSFORM_NUMBER, 2,
                                    "MiB",
                                    Swap_max, RRDF_FIELD_SORT_DESCENDING, NULL, RRDF_FIELD_SUMMARY_SUM,
                                    RRDF_FIELD_FILTER_RANGE,
                                    RRDF_FIELD_OPTS_NONE, NULL);

        // Physical I/O
        buffer_rrdf_table_add_field(wb, field_id++, "PReads", "Physical I/O Reads", RRDF_FIELD_TYPE_BAR_WITH_INTEGER,
                                    RRDF_FIELD_VISUAL_BAR, RRDF_FIELD_TRANSFORM_NUMBER,
                                    2, "KiB/s", PReads_max, RRDF_FIELD_SORT_DESCENDING, NULL, RRDF_FIELD_SUMMARY_SUM,
                                    RRDF_FIELD_FILTER_RANGE,
                                    RRDF_FIELD_OPTS_VISIBLE, NULL);
        buffer_rrdf_table_add_field(wb, field_id++, "PWrites", "Physical I/O Writes", RRDF_FIELD_TYPE_BAR_WITH_INTEGER,
                                    RRDF_FIELD_VISUAL_BAR,
                                    RRDF_FIELD_TRANSFORM_NUMBER, 2, "KiB/s", PWrites_max, RRDF_FIELD_SORT_DESCENDING,
                                    NULL, RRDF_FIELD_SUMMARY_SUM, RRDF_FIELD_FILTER_RANGE,
                                    RRDF_FIELD_OPTS_VISIBLE, NULL);

        // Logical I/O
#ifndef __FreeBSD__
        buffer_rrdf_table_add_field(wb, field_id++, "LReads", "Logical I/O Reads", RRDF_FIELD_TYPE_BAR_WITH_INTEGER,
                                    RRDF_FIELD_VISUAL_BAR, RRDF_FIELD_TRANSFORM_NUMBER,
                                    2, "KiB/s", LReads_max, RRDF_FIELD_SORT_DESCENDING, NULL, RRDF_FIELD_SUMMARY_SUM,
                                    RRDF_FIELD_FILTER_RANGE,
                                    RRDF_FIELD_OPTS_NONE, NULL);
        buffer_rrdf_table_add_field(wb, field_id++, "LWrites", "Logical I/O Writes", RRDF_FIELD_TYPE_BAR_WITH_INTEGER,
                                    RRDF_FIELD_VISUAL_BAR,
                                    RRDF_FIELD_TRANSFORM_NUMBER,
                                    2, "KiB/s", LWrites_max, RRDF_FIELD_SORT_DESCENDING, NULL, RRDF_FIELD_SUMMARY_SUM,
                                    RRDF_FIELD_FILTER_RANGE,
                                    RRDF_FIELD_OPTS_NONE, NULL);
#endif

        // I/O calls
        buffer_rrdf_table_add_field(wb, field_id++, "RCalls", "I/O Read Calls", RRDF_FIELD_TYPE_BAR_WITH_INTEGER,
                                    RRDF_FIELD_VISUAL_BAR, RRDF_FIELD_TRANSFORM_NUMBER, 2,
                                    "calls/s", RCalls_max, RRDF_FIELD_SORT_DESCENDING, NULL, RRDF_FIELD_SUMMARY_SUM,
                                    RRDF_FIELD_FILTER_RANGE,
                                    RRDF_FIELD_OPTS_NONE, NULL);
        buffer_rrdf_table_add_field(wb, field_id++, "WCalls", "I/O Write Calls", RRDF_FIELD_TYPE_BAR_WITH_INTEGER,
                                    RRDF_FIELD_VISUAL_BAR, RRDF_FIELD_TRANSFORM_NUMBER, 2,
                                    "calls/s", WCalls_max, RRDF_FIELD_SORT_DESCENDING, NULL, RRDF_FIELD_SUMMARY_SUM,
                                    RRDF_FIELD_FILTER_RANGE,
                                    RRDF_FIELD_OPTS_NONE, NULL);

        // minor page faults
        buffer_rrdf_table_add_field(wb, field_id++, "MinFlt", "Minor Page Faults/s", RRDF_FIELD_TYPE_BAR_WITH_INTEGER,
                                    RRDF_FIELD_VISUAL_BAR,
                                    RRDF_FIELD_TRANSFORM_NUMBER,
                                    2, "pgflts/s", MinFlt_max, RRDF_FIELD_SORT_DESCENDING, NULL, RRDF_FIELD_SUMMARY_SUM,
                                    RRDF_FIELD_FILTER_RANGE,
                                    RRDF_FIELD_OPTS_NONE, NULL);
        buffer_rrdf_table_add_field(wb, field_id++, "CMinFlt", "Children Minor Page Faults/s",
                                    RRDF_FIELD_TYPE_BAR_WITH_INTEGER,
                                    RRDF_FIELD_VISUAL_BAR,
                                    RRDF_FIELD_TRANSFORM_NUMBER, 2, "pgflts/s", CMinFlt_max, RRDF_FIELD_SORT_DESCENDING,
                                    NULL, RRDF_FIELD_SUMMARY_SUM, RRDF_FIELD_FILTER_RANGE,
                                    RRDF_FIELD_OPTS_NONE, NULL);
        buffer_rrdf_table_add_field(wb, field_id++, "TMinFlt", "Total Minor Page Faults/s",
                                    RRDF_FIELD_TYPE_BAR_WITH_INTEGER, RRDF_FIELD_VISUAL_BAR,
                                    RRDF_FIELD_TRANSFORM_NUMBER, 2, "pgflts/s", TMinFlt_max, RRDF_FIELD_SORT_DESCENDING,
                                    NULL, RRDF_FIELD_SUMMARY_SUM, RRDF_FIELD_FILTER_RANGE,
                                    RRDF_FIELD_OPTS_NONE, NULL);

        // major page faults
        buffer_rrdf_table_add_field(wb, field_id++, "MajFlt", "Major Page Faults/s", RRDF_FIELD_TYPE_BAR_WITH_INTEGER,
                                    RRDF_FIELD_VISUAL_BAR,
                                    RRDF_FIELD_TRANSFORM_NUMBER,
                                    2, "pgflts/s", MajFlt_max, RRDF_FIELD_SORT_DESCENDING, NULL, RRDF_FIELD_SUMMARY_SUM,
                                    RRDF_FIELD_FILTER_RANGE,
                                    RRDF_FIELD_OPTS_NONE, NULL);
        buffer_rrdf_table_add_field(wb, field_id++, "CMajFlt", "Children Major Page Faults/s",
                                    RRDF_FIELD_TYPE_BAR_WITH_INTEGER,
                                    RRDF_FIELD_VISUAL_BAR,
                                    RRDF_FIELD_TRANSFORM_NUMBER, 2, "pgflts/s", CMajFlt_max, RRDF_FIELD_SORT_DESCENDING,
                                    NULL, RRDF_FIELD_SUMMARY_SUM, RRDF_FIELD_FILTER_RANGE,
                                    RRDF_FIELD_OPTS_NONE, NULL);
        buffer_rrdf_table_add_field(wb, field_id++, "TMajFlt", "Total Major Page Faults/s",
                                    RRDF_FIELD_TYPE_BAR_WITH_INTEGER, RRDF_FIELD_VISUAL_BAR,
                                    RRDF_FIELD_TRANSFORM_NUMBER, 2, "pgflts/s", TMajFlt_max, RRDF_FIELD_SORT_DESCENDING,
                                    NULL, RRDF_FIELD_SUMMARY_SUM, RRDF_FIELD_FILTER_RANGE,
                                    RRDF_FIELD_OPTS_NONE, NULL);

        // open file descriptors
        buffer_rrdf_table_add_field(wb, field_id++, "FDsLimitPercent", "Percentage of Open Descriptors vs Limits",
                                    RRDF_FIELD_TYPE_BAR_WITH_INTEGER, RRDF_FIELD_VISUAL_BAR,
                                    RRDF_FIELD_TRANSFORM_NUMBER, 2, "%", FDsLimitPercent_max, RRDF_FIELD_SORT_DESCENDING, NULL,
                                    RRDF_FIELD_SUMMARY_MAX, RRDF_FIELD_FILTER_RANGE,
                                    RRDF_FIELD_OPTS_NONE, NULL);
        buffer_rrdf_table_add_field(wb, field_id++, "FDs", "All Open File Descriptors",
                                    RRDF_FIELD_TYPE_BAR_WITH_INTEGER, RRDF_FIELD_VISUAL_BAR,
                                    RRDF_FIELD_TRANSFORM_NUMBER, 0, "fds", FDs_max, RRDF_FIELD_SORT_DESCENDING, NULL,
                                    RRDF_FIELD_SUMMARY_SUM, RRDF_FIELD_FILTER_RANGE,
                                    RRDF_FIELD_OPTS_NONE, NULL);
        buffer_rrdf_table_add_field(wb, field_id++, "Files", "Open Files", RRDF_FIELD_TYPE_BAR_WITH_INTEGER,
                                    RRDF_FIELD_VISUAL_BAR, RRDF_FIELD_TRANSFORM_NUMBER, 0,
                                    "fds",
                                    Files_max, RRDF_FIELD_SORT_DESCENDING, NULL, RRDF_FIELD_SUMMARY_SUM,
                                    RRDF_FIELD_FILTER_RANGE,
                                    RRDF_FIELD_OPTS_NONE, NULL);
        buffer_rrdf_table_add_field(wb, field_id++, "Pipes", "Open Pipes", RRDF_FIELD_TYPE_BAR_WITH_INTEGER,
                                    RRDF_FIELD_VISUAL_BAR, RRDF_FIELD_TRANSFORM_NUMBER, 0,
                                    "fds",
                                    Pipes_max, RRDF_FIELD_SORT_DESCENDING, NULL, RRDF_FIELD_SUMMARY_SUM,
                                    RRDF_FIELD_FILTER_RANGE,
                                    RRDF_FIELD_OPTS_NONE, NULL);
        buffer_rrdf_table_add_field(wb, field_id++, "Sockets", "Open Sockets", RRDF_FIELD_TYPE_BAR_WITH_INTEGER,
                                    RRDF_FIELD_VISUAL_BAR, RRDF_FIELD_TRANSFORM_NUMBER, 0,
                                    "fds", Sockets_max, RRDF_FIELD_SORT_DESCENDING, NULL, RRDF_FIELD_SUMMARY_SUM,
                                    RRDF_FIELD_FILTER_RANGE,
                                    RRDF_FIELD_OPTS_NONE, NULL);
        buffer_rrdf_table_add_field(wb, field_id++, "iNotiFDs", "Open iNotify Descriptors",
                                    RRDF_FIELD_TYPE_BAR_WITH_INTEGER, RRDF_FIELD_VISUAL_BAR,
                                    RRDF_FIELD_TRANSFORM_NUMBER, 0, "fds", iNotiFDs_max, RRDF_FIELD_SORT_DESCENDING,
                                    NULL, RRDF_FIELD_SUMMARY_SUM, RRDF_FIELD_FILTER_RANGE,
                                    RRDF_FIELD_OPTS_NONE, NULL);
        buffer_rrdf_table_add_field(wb, field_id++, "EventFDs", "Open Event Descriptors",
                                    RRDF_FIELD_TYPE_BAR_WITH_INTEGER, RRDF_FIELD_VISUAL_BAR,
                                    RRDF_FIELD_TRANSFORM_NUMBER, 0, "fds", EventFDs_max, RRDF_FIELD_SORT_DESCENDING,
                                    NULL, RRDF_FIELD_SUMMARY_SUM, RRDF_FIELD_FILTER_RANGE,
                                    RRDF_FIELD_OPTS_NONE, NULL);
        buffer_rrdf_table_add_field(wb, field_id++, "TimerFDs", "Open Timer Descriptors",
                                    RRDF_FIELD_TYPE_BAR_WITH_INTEGER, RRDF_FIELD_VISUAL_BAR,
                                    RRDF_FIELD_TRANSFORM_NUMBER, 0, "fds", TimerFDs_max, RRDF_FIELD_SORT_DESCENDING,
                                    NULL, RRDF_FIELD_SUMMARY_SUM, RRDF_FIELD_FILTER_RANGE,
                                    RRDF_FIELD_OPTS_NONE, NULL);
        buffer_rrdf_table_add_field(wb, field_id++, "SigFDs", "Open Signal Descriptors",
                                    RRDF_FIELD_TYPE_BAR_WITH_INTEGER, RRDF_FIELD_VISUAL_BAR,
                                    RRDF_FIELD_TRANSFORM_NUMBER, 0, "fds", SigFDs_max, RRDF_FIELD_SORT_DESCENDING, NULL,
                                    RRDF_FIELD_SUMMARY_SUM, RRDF_FIELD_FILTER_RANGE,
                                    RRDF_FIELD_OPTS_NONE, NULL);
        buffer_rrdf_table_add_field(wb, field_id++, "EvPollFDs", "Open Event Poll Descriptors",
                                    RRDF_FIELD_TYPE_BAR_WITH_INTEGER,
                                    RRDF_FIELD_VISUAL_BAR, RRDF_FIELD_TRANSFORM_NUMBER, 0, "fds", EvPollFDs_max,
                                    RRDF_FIELD_SORT_DESCENDING, NULL, RRDF_FIELD_SUMMARY_SUM, RRDF_FIELD_FILTER_RANGE,
                                    RRDF_FIELD_OPTS_NONE, NULL);
        buffer_rrdf_table_add_field(wb, field_id++, "OtherFDs", "Other Open Descriptors",
                                    RRDF_FIELD_TYPE_BAR_WITH_INTEGER, RRDF_FIELD_VISUAL_BAR,
                                    RRDF_FIELD_TRANSFORM_NUMBER, 0, "fds", OtherFDs_max, RRDF_FIELD_SORT_DESCENDING,
                                    NULL, RRDF_FIELD_SUMMARY_SUM, RRDF_FIELD_FILTER_RANGE,
                                    RRDF_FIELD_OPTS_NONE, NULL);

        // processes, threads, uptime
        buffer_rrdf_table_add_field(wb, field_id++, "Processes", "Processes", RRDF_FIELD_TYPE_BAR_WITH_INTEGER,
                                    RRDF_FIELD_VISUAL_BAR, RRDF_FIELD_TRANSFORM_NUMBER, 0,
                                    "processes", Processes_max, RRDF_FIELD_SORT_DESCENDING, NULL,
                                    RRDF_FIELD_SUMMARY_SUM, RRDF_FIELD_FILTER_RANGE,
                                    RRDF_FIELD_OPTS_NONE, NULL);
        buffer_rrdf_table_add_field(wb, field_id++, "Threads", "Threads", RRDF_FIELD_TYPE_BAR_WITH_INTEGER,
                                    RRDF_FIELD_VISUAL_BAR, RRDF_FIELD_TRANSFORM_NUMBER, 0,
                                    "threads", Threads_max, RRDF_FIELD_SORT_DESCENDING, NULL, RRDF_FIELD_SUMMARY_SUM,
                                    RRDF_FIELD_FILTER_RANGE,
                                    RRDF_FIELD_OPTS_NONE, NULL);
        buffer_rrdf_table_add_field(wb, field_id++, "Uptime", "Uptime in seconds", RRDF_FIELD_TYPE_DURATION,
                                    RRDF_FIELD_VISUAL_BAR, RRDF_FIELD_TRANSFORM_DURATION_S, 2,
                                    "seconds", Uptime_max, RRDF_FIELD_SORT_DESCENDING, NULL, RRDF_FIELD_SUMMARY_MAX,
                                    RRDF_FIELD_FILTER_RANGE,
                                    RRDF_FIELD_OPTS_VISIBLE, NULL);
    }
    buffer_json_object_close(wb); // columns

    buffer_json_member_add_string(wb, "default_sort_column", "CPU");

    buffer_json_member_add_object(wb, "charts");
    {
        // CPU chart
        buffer_json_member_add_object(wb, "CPU");
        {
            buffer_json_member_add_string(wb, "name", "CPU Utilization");
            buffer_json_member_add_string(wb, "type", "stacked-bar");
            buffer_json_member_add_array(wb, "columns");
            {
                buffer_json_add_array_item_string(wb, "UserCPU");
                buffer_json_add_array_item_string(wb, "SysCPU");
                buffer_json_add_array_item_string(wb, "GuestCPU");
                buffer_json_add_array_item_string(wb, "CUserCPU");
                buffer_json_add_array_item_string(wb, "CSysCPU");
                buffer_json_add_array_item_string(wb, "CGuestCPU");
            }
            buffer_json_array_close(wb);
        }
        buffer_json_object_close(wb);

        buffer_json_member_add_object(wb, "CPUCtxSwitches");
        {
            buffer_json_member_add_string(wb, "name", "CPU Context Switches");
            buffer_json_member_add_string(wb, "type", "stacked-bar");
            buffer_json_member_add_array(wb, "columns");
            {
                buffer_json_add_array_item_string(wb, "vCtxSwitch");
                buffer_json_add_array_item_string(wb, "iCtxSwitch");
            }
            buffer_json_array_close(wb);
        }
        buffer_json_object_close(wb);

        // Memory chart
        buffer_json_member_add_object(wb, "Memory");
        {
            buffer_json_member_add_string(wb, "name", "Memory");
            buffer_json_member_add_string(wb, "type", "stacked-bar");
            buffer_json_member_add_array(wb, "columns");
            {
                buffer_json_add_array_item_string(wb, "Virtual");
                buffer_json_add_array_item_string(wb, "Resident");
                buffer_json_add_array_item_string(wb, "Shared");
                buffer_json_add_array_item_string(wb, "Swap");
            }
            buffer_json_array_close(wb);
        }
        buffer_json_object_close(wb);

        if(MemTotal) {
            // Memory chart
            buffer_json_member_add_object(wb, "MemoryPercent");
            {
                buffer_json_member_add_string(wb, "name", "Memory Percentage");
                buffer_json_member_add_string(wb, "type", "stacked-bar");
                buffer_json_member_add_array(wb, "columns");
                {
                    buffer_json_add_array_item_string(wb, "Memory");
                }
                buffer_json_array_close(wb);
            }
            buffer_json_object_close(wb);
        }

#ifndef __FreeBSD__
        // I/O Reads chart
        buffer_json_member_add_object(wb, "Reads");
        {
            buffer_json_member_add_string(wb, "name", "I/O Reads");
            buffer_json_member_add_string(wb, "type", "stacked-bar");
            buffer_json_member_add_array(wb, "columns");
            {
                buffer_json_add_array_item_string(wb, "LReads");
                buffer_json_add_array_item_string(wb, "PReads");
            }
            buffer_json_array_close(wb);
        }
        buffer_json_object_close(wb);

        // I/O Writes chart
        buffer_json_member_add_object(wb, "Writes");
        {
            buffer_json_member_add_string(wb, "name", "I/O Writes");
            buffer_json_member_add_string(wb, "type", "stacked-bar");
            buffer_json_member_add_array(wb, "columns");
            {
                buffer_json_add_array_item_string(wb, "LWrites");
                buffer_json_add_array_item_string(wb, "PWrites");
            }
            buffer_json_array_close(wb);
        }
        buffer_json_object_close(wb);

        // Logical I/O chart
        buffer_json_member_add_object(wb, "LogicalIO");
        {
            buffer_json_member_add_string(wb, "name", "Logical I/O");
            buffer_json_member_add_string(wb, "type", "stacked-bar");
            buffer_json_member_add_array(wb, "columns");
            {
                buffer_json_add_array_item_string(wb, "LReads");
                buffer_json_add_array_item_string(wb, "LWrites");
            }
            buffer_json_array_close(wb);
        }
        buffer_json_object_close(wb);
#endif

        // Physical I/O chart
        buffer_json_member_add_object(wb, "PhysicalIO");
        {
            buffer_json_member_add_string(wb, "name", "Physical I/O");
            buffer_json_member_add_string(wb, "type", "stacked-bar");
            buffer_json_member_add_array(wb, "columns");
            {
                buffer_json_add_array_item_string(wb, "PReads");
                buffer_json_add_array_item_string(wb, "PWrites");
            }
            buffer_json_array_close(wb);
        }
        buffer_json_object_close(wb);

        // I/O Calls chart
        buffer_json_member_add_object(wb, "IOCalls");
        {
            buffer_json_member_add_string(wb, "name", "I/O Calls");
            buffer_json_member_add_string(wb, "type", "stacked-bar");
            buffer_json_member_add_array(wb, "columns");
            {
                buffer_json_add_array_item_string(wb, "RCalls");
                buffer_json_add_array_item_string(wb, "WCalls");
            }
            buffer_json_array_close(wb);
        }
        buffer_json_object_close(wb);

        // Minor Page Faults chart
        buffer_json_member_add_object(wb, "MinFlt");
        {
            buffer_json_member_add_string(wb, "name", "Minor Page Faults");
            buffer_json_member_add_string(wb, "type", "stacked-bar");
            buffer_json_member_add_array(wb, "columns");
            {
                buffer_json_add_array_item_string(wb, "MinFlt");
                buffer_json_add_array_item_string(wb, "CMinFlt");
            }
            buffer_json_array_close(wb);
        }
        buffer_json_object_close(wb);

        // Major Page Faults chart
        buffer_json_member_add_object(wb, "MajFlt");
        {
            buffer_json_member_add_string(wb, "name", "Major Page Faults");
            buffer_json_member_add_string(wb, "type", "stacked-bar");
            buffer_json_member_add_array(wb, "columns");
            {
                buffer_json_add_array_item_string(wb, "MajFlt");
                buffer_json_add_array_item_string(wb, "CMajFlt");
            }
            buffer_json_array_close(wb);
        }
        buffer_json_object_close(wb);

        // Threads chart
        buffer_json_member_add_object(wb, "Threads");
        {
            buffer_json_member_add_string(wb, "name", "Threads");
            buffer_json_member_add_string(wb, "type", "stacked-bar");
            buffer_json_member_add_array(wb, "columns");
            {
                buffer_json_add_array_item_string(wb, "Threads");
            }
            buffer_json_array_close(wb);
        }
        buffer_json_object_close(wb);

        // Processes chart
        buffer_json_member_add_object(wb, "Processes");
        {
            buffer_json_member_add_string(wb, "name", "Processes");
            buffer_json_member_add_string(wb, "type", "stacked-bar");
            buffer_json_member_add_array(wb, "columns");
            {
                buffer_json_add_array_item_string(wb, "Processes");
            }
            buffer_json_array_close(wb);
        }
        buffer_json_object_close(wb);

        // FDs chart
        buffer_json_member_add_object(wb, "FDs");
        {
            buffer_json_member_add_string(wb, "name", "File Descriptors");
            buffer_json_member_add_string(wb, "type", "stacked-bar");
            buffer_json_member_add_array(wb, "columns");
            {
                buffer_json_add_array_item_string(wb, "Files");
                buffer_json_add_array_item_string(wb, "Pipes");
                buffer_json_add_array_item_string(wb, "Sockets");
                buffer_json_add_array_item_string(wb, "iNotiFDs");
                buffer_json_add_array_item_string(wb, "EventFDs");
                buffer_json_add_array_item_string(wb, "TimerFDs");
                buffer_json_add_array_item_string(wb, "SigFDs");
                buffer_json_add_array_item_string(wb, "EvPollFDs");
                buffer_json_add_array_item_string(wb, "OtherFDs");
            }
            buffer_json_array_close(wb);
        }
        buffer_json_object_close(wb);
    }
    buffer_json_object_close(wb); // charts

    buffer_json_member_add_array(wb, "default_charts");
    {
        buffer_json_add_array_item_array(wb);
        buffer_json_add_array_item_string(wb, "CPU");
        buffer_json_add_array_item_string(wb, "Category");
        buffer_json_array_close(wb);

        buffer_json_add_array_item_array(wb);
        buffer_json_add_array_item_string(wb, "Memory");
        buffer_json_add_array_item_string(wb, "Category");
        buffer_json_array_close(wb);
    }
    buffer_json_array_close(wb);

    buffer_json_member_add_object(wb, "group_by");
    {
        // group by PID
        buffer_json_member_add_object(wb, "PID");
        {
            buffer_json_member_add_string(wb, "name", "Process Tree by PID");
            buffer_json_member_add_array(wb, "columns");
            {
                buffer_json_add_array_item_string(wb, "PPID");
            }
            buffer_json_array_close(wb);
        }
        buffer_json_object_close(wb);

        // group by Category
        buffer_json_member_add_object(wb, "Category");
        {
            buffer_json_member_add_string(wb, "name", "Process Tree by Category");
            buffer_json_member_add_array(wb, "columns");
            {
                buffer_json_add_array_item_string(wb, "Category");
                buffer_json_add_array_item_string(wb, "PPID");
            }
            buffer_json_array_close(wb);
        }
        buffer_json_object_close(wb);

        // group by User
        buffer_json_member_add_object(wb, "User");
        {
            buffer_json_member_add_string(wb, "name", "Process Tree by User");
            buffer_json_member_add_array(wb, "columns");
            {
                buffer_json_add_array_item_string(wb, "User");
                buffer_json_add_array_item_string(wb, "PPID");
            }
            buffer_json_array_close(wb);
        }
        buffer_json_object_close(wb);

        // group by Group
        buffer_json_member_add_object(wb, "Group");
        {
            buffer_json_member_add_string(wb, "name", "Process Tree by Group");
            buffer_json_member_add_array(wb, "columns");
            {
                buffer_json_add_array_item_string(wb, "Group");
                buffer_json_add_array_item_string(wb, "PPID");
            }
            buffer_json_array_close(wb);
        }
        buffer_json_object_close(wb);
    }
    buffer_json_object_close(wb); // group_by

    buffer_json_member_add_time_t(wb, "expires", expires);
    buffer_json_finalize(wb);

    pluginsd_function_result_to_stdout(transaction, HTTP_RESP_OK, "application/json", expires, wb);

    buffer_free(wb);
}

static bool apps_plugin_exit = false;

int main(int argc, char **argv) {
    clocks_init();
    nd_log_initialize_for_external_plugins("apps.plugin");

    pagesize = (size_t)sysconf(_SC_PAGESIZE);

    bool send_resource_usage = true;
    {
        const char *s = getenv("NETDATA_INTERNALS_MONITORING");
        if(s && *s && strcmp(s, "NO") == 0)
            send_resource_usage = false;
    }

    // since apps.plugin runs as root, prevent it from opening symbolic links
    procfile_open_flags = O_RDONLY|O_NOFOLLOW;

    netdata_configured_host_prefix = getenv("NETDATA_HOST_PREFIX");
    if(verify_netdata_host_prefix(true) == -1) exit(1);

    user_config_dir = getenv("NETDATA_USER_CONFIG_DIR");
    if(user_config_dir == NULL) {
        // netdata_log_info("NETDATA_CONFIG_DIR is not passed from netdata");
        user_config_dir = CONFIG_DIR;
    }
    // else netdata_log_info("Found NETDATA_USER_CONFIG_DIR='%s'", user_config_dir);

    stock_config_dir = getenv("NETDATA_STOCK_CONFIG_DIR");
    if(stock_config_dir == NULL) {
        // netdata_log_info("NETDATA_CONFIG_DIR is not passed from netdata");
        stock_config_dir = LIBCONFIG_DIR;
    }
    // else netdata_log_info("Found NETDATA_USER_CONFIG_DIR='%s'", user_config_dir);

#ifdef NETDATA_INTERNAL_CHECKS
    if(debug_flags != 0) {
        struct rlimit rl = { RLIM_INFINITY, RLIM_INFINITY };
        if(setrlimit(RLIMIT_CORE, &rl) != 0)
            netdata_log_info("Cannot request unlimited core dumps for debugging... Proceeding anyway...");
#ifdef HAVE_SYS_PRCTL_H
        prctl(PR_SET_DUMPABLE, 1, 0, 0, 0);
#endif
    }
#endif /* NETDATA_INTERNAL_CHECKS */

    procfile_adaptive_initial_allocation = 1;

    get_system_HZ();
#ifdef __FreeBSD__
    time_factor = 1000000ULL / RATES_DETAIL; // FreeBSD uses usecs
#else
    time_factor = system_hz; // Linux uses clock ticks
#endif

    get_system_pid_max();
    get_system_cpus_uncached();

    parse_args(argc, argv);

    if(!check_capabilities() && !am_i_running_as_root() && !check_proc_1_io()) {
        uid_t uid = getuid(), euid = geteuid();
#ifdef HAVE_SYS_CAPABILITY_H
        netdata_log_error("apps.plugin should either run as root (now running with uid %u, euid %u) or have special capabilities. "
                      "Without these, apps.plugin cannot report disk I/O utilization of other processes. "
                      "To enable capabilities run: sudo setcap cap_dac_read_search,cap_sys_ptrace+ep %s; "
                      "To enable setuid to root run: sudo chown root:netdata %s; sudo chmod 4750 %s; "
              , uid, euid, argv[0], argv[0], argv[0]
        );
#else
        netdata_log_error("apps.plugin should either run as root (now running with uid %u, euid %u) or have special capabilities. "
                      "Without these, apps.plugin cannot report disk I/O utilization of other processes. "
                      "Your system does not support capabilities. "
                      "To enable setuid to root run: sudo chown root:netdata %s; sudo chmod 4750 %s; "
              , uid, euid, argv[0], argv[0]
        );
#endif
    }

    netdata_log_info("started on pid %d", getpid());

    snprintfz(all_user_ids.filename, FILENAME_MAX, "%s/etc/passwd", netdata_configured_host_prefix);
    debug_log("passwd file: '%s'", all_user_ids.filename);

    snprintfz(all_group_ids.filename, FILENAME_MAX, "%s/etc/group", netdata_configured_host_prefix);
    debug_log("group file: '%s'", all_group_ids.filename);

#if (ALL_PIDS_ARE_READ_INSTANTLY == 0)
    all_pids_sortlist = callocz(sizeof(pid_t), (size_t)pid_max + 1);
#endif

    all_pids          = callocz(sizeof(struct pid_stat *), (size_t) pid_max + 1);

    // ------------------------------------------------------------------------
    // the event loop for functions

    struct functions_evloop_globals *wg =
            functions_evloop_init(1, "APPS", &apps_and_stdout_mutex, &apps_plugin_exit);

    functions_evloop_add_function(wg, "processes", function_processes, PLUGINS_FUNCTIONS_TIMEOUT_DEFAULT, NULL);

    // ------------------------------------------------------------------------

    netdata_mutex_lock(&apps_and_stdout_mutex);
    APPS_PLUGIN_GLOBAL_FUNCTIONS();

    usec_t step = update_every * USEC_PER_SEC;
    global_iterations_counter = 1;
    heartbeat_t hb;
    heartbeat_init(&hb);
    for(; !apps_plugin_exit ; global_iterations_counter++) {
        netdata_mutex_unlock(&apps_and_stdout_mutex);

#ifdef NETDATA_PROFILING
#warning "compiling for profiling"
        static int profiling_count=0;
        profiling_count++;
        if(unlikely(profiling_count > 2000)) exit(0);
        usec_t dt = update_every * USEC_PER_SEC;
#else
        usec_t dt = heartbeat_next(&hb, step);
#endif
        netdata_mutex_lock(&apps_and_stdout_mutex);

        struct pollfd pollfd = { .fd = fileno(stdout), .events = POLLERR };
        if (unlikely(poll(&pollfd, 1, 0) < 0)) {
            netdata_mutex_unlock(&apps_and_stdout_mutex);
            fatal("Cannot check if a pipe is available");
        }
        if (unlikely(pollfd.revents & POLLERR)) {
            netdata_mutex_unlock(&apps_and_stdout_mutex);
            fatal("Received error on read pipe.");
        }

        if(global_iterations_counter % 10 == 0)
            get_MemTotal();

        if(!collect_data_for_all_processes()) {
            netdata_log_error("Cannot collect /proc data for running processes. Disabling apps.plugin...");
            printf("DISABLE\n");
            netdata_mutex_unlock(&apps_and_stdout_mutex);
            exit(1);
        }

        calculate_netdata_statistics();
        normalize_utilization(apps_groups_root_target);

        if(send_resource_usage)
            send_resource_usage_to_netdata(dt);

#ifndef __FreeBSD__
        send_proc_states_count(dt);
#endif

        send_charts_updates_to_netdata(apps_groups_root_target, "app", "app_group", "Apps");
        send_collected_data_to_netdata(apps_groups_root_target, "app", dt);

        if (enable_users_charts) {
            send_charts_updates_to_netdata(users_root_target, "user", "user", "Users");
            send_collected_data_to_netdata(users_root_target, "user", dt);
        }

        if (enable_groups_charts) {
            send_charts_updates_to_netdata(groups_root_target, "usergroup", "user_group", "User Groups");
            send_collected_data_to_netdata(groups_root_target, "usergroup", dt);
        }

        fflush(stdout);

        show_guest_time_old = show_guest_time;

        debug_log("done Loop No %zu", global_iterations_counter);
    }
    netdata_mutex_unlock(&apps_and_stdout_mutex);
}
