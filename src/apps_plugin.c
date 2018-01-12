
/*
 * netdata apps.plugin
 * (C) Copyright 2016-2017 Costa Tsaousis <costa@tsaousis.gr>
 * Released under GPL v3+
 */

#include "common.h"

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
// to avoid reallocating too frequently, we can increase the number of spare
// file descriptors used by processes.
// IMPORTANT:
// having a lot of spares, increases the CPU utilization of the plugin.
#define MAX_SPARE_FDS 1


// ----------------------------------------------------------------------------
// command line options

static int
        debug = 0,
        update_every = 1,
        enable_guest_charts = 0,
#ifdef __FreeBSD__
        enable_file_charts = 0,
#else
        enable_file_charts = 1,
#endif
        enable_users_charts = 1,
        enable_groups_charts = 1,
        include_exited_childs = 1;


// will be changed to getenv(NETDATA_CONFIG_DIR) if it exists
static char *config_dir = CONFIG_DIR;

// ----------------------------------------------------------------------------
// internal flags
// handled in code (automatically set)

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
// With normalization we align the per-process utilization, to the total of
// the system. We first consume the exited children utilization and it the
// collected values is above the total, we proportionally scale each reported
// metric.

// the total system time, as reported by /proc/stat
#if (ALL_PIDS_ARE_READ_INSTANTLY == 0)
static kernel_uint_t
        global_utime = 0,
        global_stime = 0,
        global_gtime = 0;
#endif

// the normalization ratios, as calculated by normalize_utilization()
double  utime_fix_ratio = 1.0,
        stime_fix_ratio = 1.0,
        gtime_fix_ratio = 1.0,
        minflt_fix_ratio = 1.0,
        majflt_fix_ratio = 1.0,
        cutime_fix_ratio = 1.0,
        cstime_fix_ratio = 1.0,
        cgtime_fix_ratio = 1.0,
        cminflt_fix_ratio = 1.0,
        cmajflt_fix_ratio = 1.0;

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

    uid_t uid;
    gid_t gid;

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

    kernel_uint_t statm_size;
    kernel_uint_t statm_resident;
    kernel_uint_t statm_share;
    // kernel_uint_t statm_text;
    // kernel_uint_t statm_lib;
    // kernel_uint_t statm_data;
    // kernel_uint_t statm_dirty;

    kernel_uint_t io_logical_bytes_read;
    kernel_uint_t io_logical_bytes_written;
    // kernel_uint_t io_read_calls;
    // kernel_uint_t io_write_calls;
    kernel_uint_t io_storage_bytes_read;
    kernel_uint_t io_storage_bytes_written;
    // kernel_uint_t io_cancelled_write_bytes;

    int *target_fds;
    int target_fds_size;

    kernel_uint_t openfiles;
    kernel_uint_t openpipes;
    kernel_uint_t opensockets;
    kernel_uint_t openinotifies;
    kernel_uint_t openeventfds;
    kernel_uint_t opentimerfds;
    kernel_uint_t opensignalfds;
    kernel_uint_t openeventpolls;
    kernel_uint_t openother;

    unsigned int processes; // how many processes have been merged to this
    int exposed;            // if set, we have sent this to netdata
    int hidden;             // if set, we set the hidden flag on the dimension
    int debug;
    int ends_with;
    int starts_with;        // if set, the compare string matches only the
                            // beginning of the command

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

struct pid_stat {
    int32_t pid;
    char comm[MAX_COMPARE_NAME + 1];
    char *cmdline;

    uint32_t log_thrown;

    // char state;
    int32_t ppid;
    // int32_t pgrp;
    // int32_t session;
    // int32_t tty_nr;
    // int32_t tpgid;
    // uint64_t flags;

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
    // kernel_uint_t starttime;
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

    kernel_uint_t statm_size;
    kernel_uint_t statm_resident;
    kernel_uint_t statm_share;
    // kernel_uint_t statm_text;
    // kernel_uint_t statm_lib;
    // kernel_uint_t statm_data;
    // kernel_uint_t statm_dirty;

    kernel_uint_t io_logical_bytes_read_raw;
    kernel_uint_t io_logical_bytes_written_raw;
    // kernel_uint_t io_read_calls_raw;
    // kernel_uint_t io_write_calls_raw;
    kernel_uint_t io_storage_bytes_read_raw;
    kernel_uint_t io_storage_bytes_written_raw;
    // kernel_uint_t io_cancelled_write_bytes_raw;

    kernel_uint_t io_logical_bytes_read;
    kernel_uint_t io_logical_bytes_written;
    // kernel_uint_t io_read_calls;
    // kernel_uint_t io_write_calls;
    kernel_uint_t io_storage_bytes_read;
    kernel_uint_t io_storage_bytes_written;
    // kernel_uint_t io_cancelled_write_bytes;

    int *fds;                       // array of fds it uses
    int fds_size;                   // the size of the fds array

    int children_count;             // number of processes directly referencing this
    char keep:1;                    // 1 when we need to keep this process in memory even after it exited
    int keeploops;                  // increases by 1 every time keep is 1 and updated 0
    char updated:1;                 // 1 when the process is currently running
    char merged:1;                  // 1 when it has been merged to its parent
    char read:1;                    // 1 when we have already read this process for this iteration

    int sortlist;                   // higher numbers = top on the process tree
                                    // each process gets a unique number

    struct target *target;          // app_groups.conf targets
    struct target *user_target;     // uid based targets
    struct target *group_target;    // gid based targets

    usec_t stat_collected_usec;
    usec_t last_stat_collected_usec;

    usec_t io_collected_usec;
    usec_t last_io_collected_usec;

    char *fds_dirname;              // the full directory name in /proc/PID/fd

    char *stat_filename;
    char *statm_filename;
    char *io_filename;
    char *cmdline_filename;

    struct pid_stat *parent;
    struct pid_stat *prev;
    struct pid_stat *next;
};

// log each problem once per process
// log flood protection flags (log_thrown)
#define PID_LOG_IO      0x00000001
#define PID_LOG_STATM   0x00000002
#define PID_LOG_CMDLINE 0x00000004
#define PID_LOG_FDS     0x00000008
#define PID_LOG_STAT    0x00000010

static struct pid_stat
        *root_of_pids = NULL,   // global list of all processes running
        **all_pids = NULL;      // to avoid allocations, we pre-allocate the
                                // the entire pid space.

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
    avl avl;

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
// callback required by fatal()

void netdata_cleanup_and_exit(int ret) {
    exit(ret);
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

    struct passwd *pw = getpwuid(uid);
    if(!pw || !pw->pw_name || !*pw->pw_name)
        snprintfz(w->name, MAX_NAME, "%u", uid);
    else
        snprintfz(w->name, MAX_NAME, "%s", pw->pw_name);

    netdata_fix_chart_name(w->name);

    w->uid = uid;

    w->next = users_root_target;
    users_root_target = w;

    if(unlikely(debug))
        fprintf(stderr, "apps.plugin: added uid %u ('%s') target\n", w->uid, w->name);

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

    struct group *gr = getgrgid(gid);
    if(!gr || !gr->gr_name || !*gr->gr_name)
        snprintfz(w->name, MAX_NAME, "%u", gid);
    else
        snprintfz(w->name, MAX_NAME, "%s", gr->gr_name);

    netdata_fix_chart_name(w->name);

    w->gid = gid;

    w->next = groups_root_target;
    groups_root_target = w;

    if(unlikely(debug))
        fprintf(stderr, "apps.plugin: added gid %u ('%s') target\n", w->gid, w->name);

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

        if(unlikely(debug)) {
            if(unlikely(target))
                fprintf(stderr, "apps.plugin: REUSING TARGET NAME '%s' on ID '%s'\n", target->name, target->id);
            else
                fprintf(stderr, "apps.plugin: NEW TARGET NAME '%s' on ID '%s'\n", name, id);
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
    w->debug = tdebug;
    w->target = target;

    // append it, to maintain the order in apps_groups.conf
    if(last) last->next = w;
    else apps_groups_root_target = w;

    if(unlikely(debug))
        fprintf(stderr, "apps.plugin: ADDING TARGET ID '%s', process name '%s' (%s), aggregated on target '%s', options: %s %s\n"
                , w->id
                , w->compare, (w->starts_with && w->ends_with)?"substring":((w->starts_with)?"prefix":((w->ends_with)?"suffix":"exact"))
                , w->target?w->target->name:w->name
                , (w->hidden)?"hidden":"-"
                , (w->debug)?"debug":"-"
        );

    return w;
}

// read the apps_groups.conf file
static int read_apps_groups_conf(const char *file)
{
    char filename[FILENAME_MAX + 1];

    snprintfz(filename, FILENAME_MAX, "%s/apps_%s.conf", config_dir, file);

    if(unlikely(debug))
        fprintf(stderr, "apps.plugin: process groups file: '%s'\n", filename);

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
                error("Cannot create target '%s' (line %zu, word %zu)", s, line, word);
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

    // allow the user to override group 'other'
    if(apps_groups_default_target->target)
        apps_groups_default_target = apps_groups_default_target->target;

    return 0;
}


// ----------------------------------------------------------------------------
// struct pid_stat management

static inline struct pid_stat *get_pid_entry(pid_t pid) {
    if(unlikely(all_pids[pid]))
        return all_pids[pid];

    struct pid_stat *p = callocz(sizeof(struct pid_stat), 1);
    p->fds = callocz(sizeof(int), MAX_SPARE_FDS);
    p->fds_size = MAX_SPARE_FDS;

    if(likely(root_of_pids))
        root_of_pids->prev = p;

    p->next = root_of_pids;
    root_of_pids = p;

    p->pid = pid;

    all_pids[pid] = p;
    all_pids_count++;

    return p;
}

static inline void del_pid_entry(pid_t pid) {
    struct pid_stat *p = all_pids[pid];

    if(unlikely(!p)) {
        error("attempted to free pid %d that is not allocated.", pid);
        return;
    }

    if(unlikely(debug))
        fprintf(stderr, "apps.plugin: process %d %s exited, deleting it.\n", pid, p->comm);

    if(root_of_pids == p)
        root_of_pids = p->next;

    if(p->next) p->next->prev = p->prev;
    if(p->prev) p->prev->next = p->next;

    freez(p->fds);
    freez(p->fds_dirname);
    freez(p->stat_filename);
    freez(p->statm_filename);
    freez(p->io_filename);
    freez(p->cmdline_filename);
    freez(p->cmdline);
    freez(p);

    all_pids[pid] = NULL;
    all_pids_count--;
}

// ----------------------------------------------------------------------------

static inline int managed_log(struct pid_stat *p, uint32_t log, int status) {
    if(unlikely(!status)) {
        // error("command failed log %u, errno %d", log, errno);

        if(unlikely(debug || errno != ENOENT)) {
            if(unlikely(debug || !(p->log_thrown & log))) {
                p->log_thrown |= log;
                switch(log) {
                    case PID_LOG_IO:
                        error("Cannot process %s/proc/%d/io (command '%s')", netdata_configured_host_prefix, p->pid, p->comm);
                        break;

                    case PID_LOG_STATM:
                        error("Cannot process %s/proc/%d/statm (command '%s')", netdata_configured_host_prefix, p->pid, p->comm);
                        break;

                    case PID_LOG_CMDLINE:
                        error("Cannot process %s/proc/%d/cmdline (command '%s')", netdata_configured_host_prefix, p->pid, p->comm);
                        break;

                    case PID_LOG_FDS:
                        error("Cannot process entries in %s/proc/%d/fd (command '%s')", netdata_configured_host_prefix, p->pid, p->comm);
                        break;

                    case PID_LOG_STAT:
                        break;

                    default:
                        error("unhandled error for pid %d, command '%s'", p->pid, p->comm);
                        break;
                }
            }
        }
        errno = 0;
    }
    else if(unlikely(p->log_thrown & log)) {
        // error("unsetting log %u on pid %d", log, p->pid);
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
        // if(debug || (p->target && p->target->debug)) fprintf(stderr, "apps.plugin: \t\tcomparing '%s' with '%s'\n", w->compare, p->comm);

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

            if(w->target) p->target = w->target;
            else p->target = w;

            if(debug || (p->target && p->target->debug))
                fprintf(stderr, "apps.plugin: \t\t%s linked to target %s\n", p->comm, p->target->name);

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

    int fd = open(p->cmdline_filename, O_RDONLY, 0666);
    if(unlikely(fd == -1)) goto cleanup;

    ssize_t i, bytes = read(fd, cmdline, MAX_CMDLINE);
    close(fd);

    if(unlikely(bytes < 0)) goto cleanup;
#endif

    cmdline[bytes] = '\0';
    for(i = 0; i < bytes ; i++)
        if(unlikely(!cmdline[i])) cmdline[i] = ' ';

    p->cmdline = strdupz(cmdline);

    if(unlikely(debug))
        fprintf(stderr, "Read file '%s' contents: %s\n", p->cmdline_filename, p->cmdline);

    return 1;

cleanup:
    // copy the command to the command line
    p->cmdline = strdupz(p->comm);
    return 0;
}

static inline int read_proc_pid_ownership(struct pid_stat *p, void *ptr) {
    (void)ptr;
#ifdef __FreeBSD__
    struct kinfo_proc *proc_info = (struct kinfo_proc *)ptr;

    p->uid = proc_info->ki_uid;
    p->gid = proc_info->ki_groups[0];

    return 1;
#else
    if(unlikely(!p->stat_filename)) {
        error("pid %d does not have a stat_filename", p->pid);
        return 0;
    }

    // ----------------------------------------
    // read uid and gid

    struct stat st;
    if(stat(p->stat_filename, &st) != 0) {
        error("Cannot stat file '%s'", p->stat_filename);
        return 1;
    }

    p->uid = st.st_uid;
    p->gid = st.st_gid;

    return 1;
#endif
}

// ----------------------------------------------------------------------------
// macro to calculate the incremental rate of a value
// each parameter is accessed only ONCE - so it is safe to pass function calls
// or other macros as parameters

#define incremental_rate(rate_variable, last_kernel_variable, new_kernel_value, collected_usec, last_collected_usec) { \
        kernel_uint_t _new_tmp = new_kernel_value; \
        (rate_variable) = (_new_tmp - (last_kernel_variable)) * (USEC_PER_SEC * RATES_DETAIL) / ((collected_usec) - (last_collected_usec)); \
        (last_kernel_variable) = _new_tmp; \
    }

// the same macro for struct pid members
#define pid_incremental_rate(type, var, value) \
    incremental_rate(var, var##_raw, value, p->type##_collected_usec, p->last_##type##_collected_usec)


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
    // p->state         = *(procfile_lineword(ff, 0, 2));
    p->ppid             = (int32_t)str2pid_t(procfile_lineword(ff, 0, 3));
    // p->pgrp          = (int32_t)str2pid_t(procfile_lineword(ff, 0, 4));
    // p->session       = (int32_t)str2pid_t(procfile_lineword(ff, 0, 5));
    // p->tty_nr        = (int32_t)str2pid_t(procfile_lineword(ff, 0, 6));
    // p->tpgid         = (int32_t)str2pid_t(procfile_lineword(ff, 0, 7));
    // p->flags         = str2uint64_t(procfile_lineword(ff, 0, 8));
#endif

    if(strcmp(p->comm, comm) != 0) {
        if(unlikely(debug)) {
            if(p->comm[0])
                fprintf(stderr, "apps.plugin: \tpid %d (%s) changed name to '%s'\n", p->pid, p->comm, comm);
            else
                fprintf(stderr, "apps.plugin: \tJust added %d (%s)\n", p->pid, comm);
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
        info("Guest charts aren't supported by FreeBSD");
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
    p->num_threads      = (int32_t)str2uint32_t(procfile_lineword(ff, 0, 19));
    // p->itrealvalue   = str2kernel_uint_t(procfile_lineword(ff, 0, 20));
    // p->starttime     = str2kernel_uint_t(procfile_lineword(ff, 0, 21));
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

    if(unlikely(debug || (p->target && p->target->debug)))
        fprintf(stderr, "apps.plugin: READ PROC/PID/STAT: %s/proc/%d/stat, process: '%s' on target '%s' (dt=%llu) VALUES: utime=" KERNEL_UINT_FORMAT ", stime=" KERNEL_UINT_FORMAT ", cutime=" KERNEL_UINT_FORMAT ", cstime=" KERNEL_UINT_FORMAT ", minflt=" KERNEL_UINT_FORMAT ", majflt=" KERNEL_UINT_FORMAT ", cminflt=" KERNEL_UINT_FORMAT ", cmajflt=" KERNEL_UINT_FORMAT ", threads=%d\n", netdata_configured_host_prefix, p->pid, p->comm, (p->target)?p->target->name:"UNSET", p->stat_collected_usec - p->last_stat_collected_usec, p->utime, p->stime, p->cutime, p->cstime, p->minflt, p->majflt, p->cminflt, p->cmajflt, p->num_threads);

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

static inline int read_proc_pid_statm(struct pid_stat *p, void *ptr) {
    (void)ptr;
#ifdef __FreeBSD__
    struct kinfo_proc *proc_info = (struct kinfo_proc *)ptr;
#else
    static procfile *ff = NULL;

    if(unlikely(!p->statm_filename)) {
        char filename[FILENAME_MAX + 1];
        snprintfz(filename, FILENAME_MAX, "%s/proc/%d/statm", netdata_configured_host_prefix, p->pid);
        p->statm_filename = strdupz(filename);
    }

    ff = procfile_reopen(ff, p->statm_filename, NULL, PROCFILE_FLAG_NO_ERROR_ON_FILE_IO);
    if(unlikely(!ff)) goto cleanup;

    ff = procfile_readall(ff);
    if(unlikely(!ff)) goto cleanup;
#endif

    calls_counter++;

#ifdef __FreeBSD__
    p->statm_size           = proc_info->ki_size / sysconf(_SC_PAGESIZE);
    p->statm_resident       = proc_info->ki_rssize;
    p->statm_share          = 0; // do we have to use ru_ixrss here?
#else
    p->statm_size           = str2kernel_uint_t(procfile_lineword(ff, 0, 0));
    p->statm_resident       = str2kernel_uint_t(procfile_lineword(ff, 0, 1));
    p->statm_share          = str2kernel_uint_t(procfile_lineword(ff, 0, 2));
    // p->statm_text           = str2kernel_uint_t(procfile_lineword(ff, 0, 3));
    // p->statm_lib            = str2kernel_uint_t(procfile_lineword(ff, 0, 4));
    // p->statm_data           = str2kernel_uint_t(procfile_lineword(ff, 0, 5));
    // p->statm_dirty          = str2kernel_uint_t(procfile_lineword(ff, 0, 6));
#endif

    return 1;

#ifndef __FreeBSD__
cleanup:
    p->statm_size           = 0;
    p->statm_resident       = 0;
    p->statm_share          = 0;
    // p->statm_text           = 0;
    // p->statm_lib            = 0;
    // p->statm_data           = 0;
    // p->statm_dirty          = 0;
    return 0;
#endif
}

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
    // pid_incremental_rate(io, p->io_read_calls,               str2kernel_uint_t(procfile_lineword(ff, 2,  1)));
    // pid_incremental_rate(io, p->io_write_calls,              str2kernel_uint_t(procfile_lineword(ff, 3,  1)));
    pid_incremental_rate(io, p->io_storage_bytes_read,       str2kernel_uint_t(procfile_lineword(ff, 4,  1)));
    pid_incremental_rate(io, p->io_storage_bytes_written,    str2kernel_uint_t(procfile_lineword(ff, 5,  1)));
    // pid_incremental_rate(io, p->io_cancelled_write_bytes,    str2kernel_uint_t(procfile_lineword(ff, 6,  1)));
#endif

    if(unlikely(global_iterations_counter == 1)) {
        p->io_logical_bytes_read        = 0;
        p->io_logical_bytes_written     = 0;
        // p->io_read_calls             = 0;
        // p->io_write_calls            = 0;
        p->io_storage_bytes_read        = 0;
        p->io_storage_bytes_written     = 0;
        // p->io_cancelled_write_bytes  = 0;
    }

    return 1;

#ifndef __FreeBSD__
cleanup:
    p->io_logical_bytes_read        = 0;
    p->io_logical_bytes_written     = 0;
    // p->io_read_calls             = 0;
    // p->io_write_calls            = 0;
    p->io_storage_bytes_read        = 0;
    p->io_storage_bytes_written     = 0;
    // p->io_cancelled_write_bytes  = 0;
    return 0;
#endif
}

#if (ALL_PIDS_ARE_READ_INSTANTLY == 0)
static inline int read_proc_stat() {
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
static inline int read_proc_stat() {
    return 0;
}
#endif

// ----------------------------------------------------------------------------

int file_descriptor_compare(void* a, void* b) {
#ifdef NETDATA_INTERNAL_CHECKS
    if(((struct file_descriptor *)a)->magic != 0x0BADCAFE || ((struct file_descriptor *)b)->magic != 0x0BADCAFE)
        error("Corrupted index data detected. Please report this.");
#endif /* NETDATA_INTERNAL_CHECKS */

    if(((struct file_descriptor *)a)->hash < ((struct file_descriptor *)b)->hash)
        return -1;

    else if(((struct file_descriptor *)a)->hash > ((struct file_descriptor *)b)->hash)
        return 1;

    else
        return strcmp(((struct file_descriptor *)a)->name, ((struct file_descriptor *)b)->name);
}

int file_descriptor_iterator(avl *a) { if(a) {}; return 0; }

avl_tree all_files_index = {
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

    return (struct file_descriptor *)avl_search(&all_files_index, (avl *) &tmp);
}

#define file_descriptor_add(fd) avl_insert(&all_files_index, (avl *)(fd))
#define file_descriptor_remove(fd) avl_remove(&all_files_index, (avl *)(fd))

// ----------------------------------------------------------------------------

static inline void file_descriptor_not_used(int id)
{
    if(id > 0 && id < all_files_size) {

#ifdef NETDATA_INTERNAL_CHECKS
        if(all_files[id].magic != 0x0BADCAFE) {
            error("Ignoring request to remove empty file id %d.", id);
            return;
        }
#endif /* NETDATA_INTERNAL_CHECKS */

        if(unlikely(debug))
            fprintf(stderr, "apps.plugin: decreasing slot %d (count = %d).\n", id, all_files[id].count);

        if(all_files[id].count > 0) {
            all_files[id].count--;

            if(!all_files[id].count) {
                if(unlikely(debug))
                    fprintf(stderr, "apps.plugin:   >> slot %d is empty.\n", id);

                if(unlikely(file_descriptor_remove(&all_files[id]) != (void *)&all_files[id]))
                    error("INTERNAL ERROR: removal of unused fd from index, removed a different fd");

#ifdef NETDATA_INTERNAL_CHECKS
                all_files[id].magic = 0x00000000;
#endif /* NETDATA_INTERNAL_CHECKS */
                all_files_len--;
            }
        }
        else
            error("Request to decrease counter of fd %d (%s), while the use counter is 0", id, all_files[id].name);
    }
    else    error("Request to decrease counter of fd %d, which is outside the array size (1 to %d)", id, all_files_size);
}

static inline void all_files_grow() {
    void *old = all_files;
    int i;

    // there is no empty slot
    if(unlikely(debug))
        fprintf(stderr, "apps.plugin: extending fd array to %d entries\n", all_files_size + FILE_DESCRIPTORS_INCREASE_STEP);

    all_files = reallocz(all_files, (all_files_size + FILE_DESCRIPTORS_INCREASE_STEP) * sizeof(struct file_descriptor));

    // if the address changed, we have to rebuild the index
    // since all pointers are now invalid

    if(unlikely(old && old != (void *)all_files)) {
        if(unlikely(debug))
            fprintf(stderr, "apps.plugin:   >> re-indexing.\n");

        all_files_index.root = NULL;
        for(i = 0; i < all_files_size; i++) {
            if(!all_files[i].count) continue;
            if(unlikely(file_descriptor_add(&all_files[i]) != (void *)&all_files[i]))
                error("INTERNAL ERROR: duplicate indexing of fd during realloc.");
        }

        if(unlikely(debug))
            fprintf(stderr, "apps.plugin:   >> re-indexing done.\n");
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

    if(unlikely(debug))
        fprintf(stderr, "apps.plugin:   >> searching for empty slot.\n");

    // search for an empty slot

    static int last_pos = 0;
    int i, c;
    for(i = 0, c = last_pos ; i < all_files_size ; i++, c++) {
        if(c >= all_files_size) c = 0;
        if(c == 0) continue;

        if(!all_files[c].count) {
            if(unlikely(debug))
                fprintf(stderr, "apps.plugin:   >> Examining slot %d.\n", c);

#ifdef NETDATA_INTERNAL_CHECKS
            if(all_files[c].magic == 0x0BADCAFE && all_files[c].name && file_descriptor_find(all_files[c].name, all_files[c].hash))
                error("fd on position %d is not cleared properly. It still has %s in it.\n", c, all_files[c].name);
#endif /* NETDATA_INTERNAL_CHECKS */

            if(unlikely(debug))
                fprintf(stderr, "apps.plugin:   >> %s fd position %d for %s (last name: %s)\n", all_files[c].name?"re-using":"using", c, name, all_files[c].name);

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

    if(unlikely(debug))
        fprintf(stderr, "apps.plugin:   >> updating slot %d.\n", c);

    all_files[c].name = strdupz(name);
    all_files[c].hash = hash;
    all_files[c].type = type;
    all_files[c].pos  = c;
    all_files[c].count = 1;
#ifdef NETDATA_INTERNAL_CHECKS
    all_files[c].magic = 0x0BADCAFE;
#endif /* NETDATA_INTERNAL_CHECKS */
    if(unlikely(file_descriptor_add(&all_files[c]) != (void *)&all_files[c]))
        error("INTERNAL ERROR: duplicate indexing of fd.");

    if(unlikely(debug))
        fprintf(stderr, "apps.plugin: using fd position %d (name: %s)\n", c, all_files[c].name);

    return c;
}

static inline int file_descriptor_find_or_add(const char *name)
{
    uint32_t hash = simple_hash(name);

    if(unlikely(debug))
        fprintf(stderr, "apps.plugin: adding or finding name '%s' with hash %u\n", name, hash);

    struct file_descriptor *fd = file_descriptor_find(name, hash);
    if(fd) {
        // found
        if(unlikely(debug))
            fprintf(stderr, "apps.plugin:   >> found on slot %d\n", fd->pos);

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
            if(unlikely(debug))
                fprintf(stderr, "apps.plugin: FIXME: unknown anonymous inode: %s\n", name);

            type = FILETYPE_OTHER;
        }
    }
    else if(likely(strcmp(name, "inotify") == 0)) type = FILETYPE_INOTIFY;
    else {
        if(unlikely(debug))
            fprintf(stderr, "apps.plugin: FIXME: cannot understand linkname: %s\n", name);

        type = FILETYPE_OTHER;
    }

    return file_descriptor_set_on_empty_slot(name, hash, type);
}

static inline void make_all_pid_fds_negative(struct pid_stat *p) {
    int *fd = p->fds, *end = &p->fds[p->fds_size];
    while(fd < end) {
        *fd = -(*fd);
        fd++;
    }
}

static inline void cleanup_negative_pid_fds(struct pid_stat *p) {
    int *fd = p->fds, *fdend = &p->fds[p->fds_size];

    while(fd < fdend) {
        if(unlikely(*fd < 0)) {
            file_descriptor_not_used(-(*fd));
            *fd++ = 0;
        }
        else
            fd++;
    }
}

static inline void zero_pid_fds(struct pid_stat *p, int first, int size) {
    int *fd = &p->fds[first], *end = &p->fds[first + size];
    while(fd < end) *fd++ = 0;
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

    // we make all pid fds negative, so that
    // we can detect unused file descriptors
    // at the end, to free them
    make_all_pid_fds_negative(p);

    mib[0] = CTL_KERN;
    mib[1] = KERN_PROC;
    mib[2] = KERN_PROC_FILEDESC;
    mib[3] = p->pid;

    if (unlikely(sysctl(mib, 4, NULL, &size, NULL, 0))) {
        error("sysctl error: Can't get file descriptors data size for pid %d", p->pid);
        return 0;
    }
    if (likely(size > 0))
        fdsbuf = reallocz(fdsbuf, size);
    if (unlikely(sysctl(mib, 4, fdsbuf, &size, NULL, 0))) {
        error("sysctl error: Can't get file descriptors data for pid %d", p->pid);
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
        int fdid = fds->kf_fd;

        // check if the fds array is small
        if (unlikely(fdid >= p->fds_size)) {
            // it is small, extend it

            if (unlikely(debug))
                fprintf(stderr, "apps.plugin: extending fd memory slots for %s from %d to %d\n", p->comm, p->fds_size, fdid + MAX_SPARE_FDS);

            p->fds = reallocz(p->fds, (fdid + MAX_SPARE_FDS) * sizeof(int));

            // and initialize it
            zero_pid_fds(p, p->fds_size, (fdid + MAX_SPARE_FDS) - p->fds_size);
            p->fds_size = fdid + MAX_SPARE_FDS;
        }

        if (unlikely(p->fds[fdid] == 0)) {
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
                    sprintf(fdsname, "other: shm: %s size: %lu", fds->kf_path, fds->kf_un.kf_file.kf_file_size);
                    break;
                case KF_TYPE_SEM:
                    sprintf(fdsname, "other: sem: %u", fds->kf_un.kf_sem.kf_sem_value);
                    break;
                default:
                    sprintf(fdsname, "other: pid: %d fd: %d", fds->kf_un.kf_proc.kf_pid, fds->kf_fd);
            }

            // if another process already has this, we will get
            // the same id
            p->fds[fdid] = file_descriptor_find_or_add(fdsname);
        }

            // else make it positive again, we need it
            // of course, the actual file may have changed, but we don't care so much
            // FIXME: we could compare the inode as returned by readdir dirent structure

        else
            p->fds[fdid] = -p->fds[fdid];

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
    char fdname[FILENAME_MAX + 1];
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
        if(unlikely(fdid >= p->fds_size)) {
            // it is small, extend it

            if(unlikely(debug))
                fprintf(stderr
                        , "apps.plugin: extending fd memory slots for %s from %d to %d\n"
                        , p->comm
                        , p->fds_size
                        , fdid + MAX_SPARE_FDS
                );

            p->fds = reallocz(p->fds, (fdid + MAX_SPARE_FDS) * sizeof(int));

            // and initialize it
            zero_pid_fds(p, p->fds_size, (fdid + MAX_SPARE_FDS) - p->fds_size);
            p->fds_size = fdid + MAX_SPARE_FDS;
        }

        if(unlikely(p->fds[fdid] == 0)) {
            // we don't know this fd, get it

            sprintf(fdname, "%s/proc/%d/fd/%s", netdata_configured_host_prefix, p->pid, de->d_name);
            ssize_t l = readlink(fdname, linkname, FILENAME_MAX);
            if(unlikely(l == -1)) {
                if(debug || (p->target && p->target->debug)) {
                    if(debug || (p->target && p->target->debug))
                        error("Cannot read link %s", fdname);
                }
                continue;
            }
            else
                linkname[l] = '\0';

            file_counter++;

            // if another process already has this, we will get
            // the same id
            p->fds[fdid] = file_descriptor_find_or_add(linkname);
        }

            // else make it positive again, we need it
            // of course, the actual file may have changed, but we don't care so much
            // FIXME: we could compare the inode as returned by readdir dirent structure
            // UPDATE: no we cannot use inodes - under /proc inodes don't change when the link is changed

        else
            p->fds[fdid] = -p->fds[fdid];
    }

    closedir(fds);
#endif
    cleanup_negative_pid_fds(p);

    return 1;
}

// ----------------------------------------------------------------------------

static inline int print_process_and_parents(struct pid_stat *p, usec_t time) {
    char *prefix = "\\_ ";
    int indent = 0;

    if(p->parent)
        indent = print_process_and_parents(p->parent, p->stat_collected_usec);
    else
        prefix = " > ";

    char buffer[indent + 1];
    int i;

    for(i = 0; i < indent ;i++) buffer[i] = ' ';
    buffer[i] = '\0';

    fprintf(stderr, "  %s %s%s (%d %s %llu"
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

static inline void print_process_tree(struct pid_stat *p, char *msg) {
    fprintf(stderr, "%s: process %s (%d, %s) with parents:\n", msg, p->comm, p->pid, p->updated?"running":"exited");
    print_process_and_parents(p, p->stat_collected_usec);
}

static inline void find_lost_child_debug(struct pid_stat *pe, kernel_uint_t lost, int type) {
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

        if(unlikely(debug)) {
            fprintf(stderr, "Absorb %s (%d %s total resources: utime=" KERNEL_UINT_FORMAT " stime=" KERNEL_UINT_FORMAT " gtime=" KERNEL_UINT_FORMAT " minflt=" KERNEL_UINT_FORMAT " majflt=" KERNEL_UINT_FORMAT ")\n"
                , p->comm
                , p->pid
                , p->updated?"running":"exited"
                , utime
                , stime
                , gtime
                , minflt
                , majflt
                );
            print_process_tree(p, "Searching parents");
        }

        struct pid_stat *pp;
        for(pp = p->parent; pp ; pp = pp->parent) {
            if(!pp->updated) continue;

            kernel_uint_t absorbed;
            absorbed = remove_exited_child_from_parent(&utime,  &pp->cutime);
            if(unlikely(debug && absorbed))
                fprintf(stderr, " > process %s (%d %s) absorbed " KERNEL_UINT_FORMAT " utime (remaining: " KERNEL_UINT_FORMAT ")\n", pp->comm, pp->pid, pp->updated?"running":"exited", absorbed, utime);

            absorbed = remove_exited_child_from_parent(&stime,  &pp->cstime);
            if(unlikely(debug && absorbed))
                fprintf(stderr, " > process %s (%d %s) absorbed " KERNEL_UINT_FORMAT " stime (remaining: " KERNEL_UINT_FORMAT ")\n", pp->comm, pp->pid, pp->updated?"running":"exited", absorbed, stime);

            absorbed = remove_exited_child_from_parent(&gtime,  &pp->cgtime);
            if(unlikely(debug && absorbed))
                fprintf(stderr, " > process %s (%d %s) absorbed " KERNEL_UINT_FORMAT " gtime (remaining: " KERNEL_UINT_FORMAT ")\n", pp->comm, pp->pid, pp->updated?"running":"exited", absorbed, gtime);

            absorbed = remove_exited_child_from_parent(&minflt, &pp->cminflt);
            if(unlikely(debug && absorbed))
                fprintf(stderr, " > process %s (%d %s) absorbed " KERNEL_UINT_FORMAT " minflt (remaining: " KERNEL_UINT_FORMAT ")\n", pp->comm, pp->pid, pp->updated?"running":"exited", absorbed, minflt);

            absorbed = remove_exited_child_from_parent(&majflt, &pp->cmajflt);
            if(unlikely(debug && absorbed))
                fprintf(stderr, " > process %s (%d %s) absorbed " KERNEL_UINT_FORMAT " majflt (remaining: " KERNEL_UINT_FORMAT ")\n", pp->comm, pp->pid, pp->updated?"running":"exited", absorbed, majflt);
        }

        if(unlikely(utime + stime + gtime + minflt + majflt > 0)) {
            if(unlikely(debug)) {
                if(utime)  find_lost_child_debug(p, utime,  3);
                if(stime)  find_lost_child_debug(p, stime,  4);
                if(gtime)  find_lost_child_debug(p, gtime,  5);
                if(minflt) find_lost_child_debug(p, minflt, 1);
                if(majflt) find_lost_child_debug(p, majflt, 2);
            }

            p->keep = 1;

            if(unlikely(debug))
                fprintf(stderr, " > remaining resources - KEEP - for another loop: %s (%d %s total resources: utime=" KERNEL_UINT_FORMAT " stime=" KERNEL_UINT_FORMAT " gtime=" KERNEL_UINT_FORMAT " minflt=" KERNEL_UINT_FORMAT " majflt=" KERNEL_UINT_FORMAT ")\n"
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
                pp->keep = 1;

                if(unlikely(debug))
                    fprintf(stderr, " > - KEEP - parent for another loop: %s (%d %s)\n"
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

            if(unlikely(debug))
                fprintf(stderr, "\n");
        }
        else if(unlikely(debug)) {
            fprintf(stderr, " > totally absorbed - DONE - %s (%d %s)\n"
                , p->comm
                , p->pid
                , p->updated?"running":"exited"
                );
        }
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
            p->parent = NULL;
            continue;
        }

        pp = all_pids[p->ppid];
        if(likely(pp)) {
            p->parent = pp;
            pp->children_count++;

            if(unlikely(debug || (p->target && p->target->debug)))
                fprintf(stderr, "apps.plugin: \tchild %d (%s, %s) on target '%s' has parent %d (%s, %s). Parent: utime=" KERNEL_UINT_FORMAT ", stime=" KERNEL_UINT_FORMAT ", gtime=" KERNEL_UINT_FORMAT ", minflt=" KERNEL_UINT_FORMAT ", majflt=" KERNEL_UINT_FORMAT ", cutime=" KERNEL_UINT_FORMAT ", cstime=" KERNEL_UINT_FORMAT ", cgtime=" KERNEL_UINT_FORMAT ", cminflt=" KERNEL_UINT_FORMAT ", cmajflt=" KERNEL_UINT_FORMAT "\n", p->pid, p->comm, p->updated?"running":"exited", (p->target)?p->target->name:"UNSET", pp->pid, pp->comm, pp->updated?"running":"exited", pp->utime, pp->stime, pp->gtime, pp->minflt, pp->majflt, pp->cutime, pp->cstime, pp->cgtime, pp->cminflt, pp->cmajflt);
        }
        else {
            p->parent = NULL;
            error("pid %d %s states parent %d, but the later does not exist.", p->pid, p->comm, p->ppid);
        }
    }
}

// ----------------------------------------------------------------------------

// 1. read all files in /proc
// 2. for each numeric directory:
//    i.   read /proc/pid/stat
//    ii.  read /proc/pid/statm
//    iii. read /proc/pid/io (requires root access)
//    iii. read the entries in directory /proc/pid/fd (requires root access)
//         for each entry:
//         a. find or create a struct file_descriptor
//         b. cleanup any old/unused file_descriptors

// after all these, some pids may be linked to targets, while others may not

// in case of errors, only 1 every 1000 errors is printed
// to avoid filling up all disk space
// if debug is enabled, all errors are printed

#ifndef __FreeBSD__
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
        error("Invalid pid %d read (expected %d to %d). Ignoring process.", pid, 0, pid_max);
        return 0;
    }

    struct pid_stat *p = get_pid_entry(pid);
    if(unlikely(!p || p->read)) return 0;
    p->read = 1;

    // fprintf(stderr, "Reading process %d (%s), sortlist %d\n", p->pid, p->comm, p->sortlist);

    // --------------------------------------------------------------------
    // /proc/<pid>/stat

    if(unlikely(!managed_log(p, PID_LOG_STAT, read_proc_pid_stat(p, ptr))))
        // there is no reason to proceed if we cannot get its status
        return 0;

    read_proc_pid_ownership(p, ptr);

    // check its parent pid
    if(unlikely(p->ppid < 0 || p->ppid > pid_max)) {
        error("Pid %d (command '%s') states invalid parent pid %d. Using 0.", pid, p->comm, p->ppid);
        p->ppid = 0;
    }

    // --------------------------------------------------------------------
    // /proc/<pid>/io

    managed_log(p, PID_LOG_IO, read_proc_pid_io(p, ptr));

    // --------------------------------------------------------------------
    // /proc/<pid>/statm

    if(unlikely(!managed_log(p, PID_LOG_STATM, read_proc_pid_statm(p, ptr))))
        // there is no reason to proceed if we cannot get its memory status
        return 0;

    // --------------------------------------------------------------------
    // /proc/<pid>/fd

    if(enable_file_charts)
            managed_log(p, PID_LOG_FDS, read_pid_file_descriptors(p, ptr));

    // --------------------------------------------------------------------
    // done!

    if(unlikely(debug && include_exited_childs && all_pids_count && p->ppid && all_pids[p->ppid] && !all_pids[p->ppid]->read))
        fprintf(stderr, "Read process %d (%s) sortlisted %d, but its parent %d (%s) sortlisted %d, is not read\n", p->pid, p->comm, p->sortlist, all_pids[p->ppid]->pid, all_pids[p->ppid]->comm, all_pids[p->ppid]->sortlist);

    // mark it as updated
    p->updated = 1;
    p->keep = 0;
    p->keeploops = 0;

    return 1;
}

static int collect_data_for_all_processes(void) {
    struct pid_stat *p = NULL;

#ifdef __FreeBSD__
    int i, procnum;
    size_t procbase_size;
    static struct kinfo_proc *procbase;

    int mib[3];

    mib[0] = CTL_KERN;
    mib[1] = KERN_PROC;
    mib[2] = KERN_PROC_PROC;
    if (unlikely(sysctl(mib, 3, NULL, &procbase_size, NULL, 0))) {
        error("sysctl error: Can't get processes data size");
        return 0;
    }
    procbase = reallocz(procbase, procbase_size);
    if (unlikely(sysctl(mib, 3, procbase, &procbase_size, NULL, 0))) {
        error("sysctl error: Can't get processes data");
        return 0;
    }
    procnum = procbase_size / sizeof(struct kinfo_proc);
#endif

    if(all_pids_count) {
#if (ALL_PIDS_ARE_READ_INSTANTLY == 0)
        size_t slc = 0;
#endif
        for(p = root_of_pids; p ; p = p->next) {
            p->read             = 0; // mark it as not read, so that collect_data_for_pid() will read it
            p->updated          = 0;
            p->merged           = 0;
            p->children_count   = 0;
            p->parent           = NULL;

#if (ALL_PIDS_ARE_READ_INSTANTLY == 0)
            all_pids_sortlist[slc++] = p->pid;
#endif
        }

#if (ALL_PIDS_ARE_READ_INSTANTLY == 0)
        if(unlikely(slc != all_pids_count)) {
            error("Internal error: I was thinking I had %zu processes in my arrays, but it seems there are %zu.", all_pids_count, slc);
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
            for(slc = 0; slc < all_pids_count; slc++)
                collect_data_for_pid(all_pids_sortlist[slc], NULL);
        }
#endif
    }

#ifdef __FreeBSD__
    for (i = 0 ; i < procnum ; ++i) {
        pid_t pid = procbase[i].ki_pid;
        collect_data_for_pid(pid, &procbase[i]);
    }
#else
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
    read_proc_stat();

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
    int c;
    struct pid_stat *p = NULL;

    for(p = root_of_pids; p ;) {
        if(!p->updated && (!p->keep || p->keeploops > 0)) {
            if(unlikely(debug && (p->keep || p->keeploops)))
                fprintf(stderr, " > CLEANUP cannot keep exited process %d (%s) anymore - removing it.\n", p->pid, p->comm);

            for(c = 0; c < p->fds_size; c++)
                if(p->fds[c] > 0) {
                    file_descriptor_not_used(p->fds[c]);
                    p->fds[c] = 0;
                }

            pid_t r = p->pid;
            p = p->next;
            del_pid_entry(r);
        }
        else {
            if(unlikely(p->keep)) p->keeploops++;
            p->keep = 0;
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
        if(unlikely(debug)) loops++;
        found = 0;
        for(p = root_of_pids; p ; p = p->next) {
            // if this process does not have a target
            // and it has a parent
            // and its parent has a target
            // then, set the parent's target to this process
            if(unlikely(!p->target && p->parent && p->parent->target)) {
                p->target = p->parent->target;
                found++;

                if(debug || (p->target && p->target->debug))
                    fprintf(stderr, "apps.plugin: \t\tTARGET INHERITANCE: %s is inherited by %d (%s) from its parent %d (%s).\n", p->target->name, p->pid, p->comm, p->parent->pid, p->parent->comm);
            }
        }
    }

    // find all the procs with 0 childs and merge them to their parents
    // repeat, until nothing more can be done.
    int sortlist = 1;
    found = 1;
    while(found) {
        if(unlikely(debug)) loops++;
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
                p->merged = 1;

                // the parent inherits the child's target, if it does not have a target itself
                if(unlikely(p->target && !p->parent->target)) {
                    p->parent->target = p->target;

                    if(debug || (p->target && p->target->debug))
                        fprintf(stderr, "apps.plugin: \t\tTARGET INHERITANCE: %s is inherited by %d (%s) from its child %d (%s).\n", p->target->name, p->parent->pid, p->parent->comm, p->pid, p->comm);
                }

                found++;
            }
        }

        if(unlikely(debug))
            fprintf(stderr, "apps.plugin: TARGET INHERITANCE: merged %d processes\n", found);
    }

    // init goes always to default target
    if(all_pids[INIT_PID])
        all_pids[INIT_PID]->target = apps_groups_default_target;

    // pid 0 goes always to default target
    if(all_pids[0])
        all_pids[0]->target = apps_groups_default_target;

    // give a default target on all top level processes
    if(unlikely(debug)) loops++;
    for(p = root_of_pids; p ; p = p->next) {
        // if the process is not merged itself
        // then is is a top level process
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
        if(unlikely(debug)) loops++;
        found = 0;
        for(p = root_of_pids; p ; p = p->next) {
            if(unlikely(!p->target && p->merged && p->parent && p->parent->target)) {
                p->target = p->parent->target;
                found++;

                if(debug || (p->target && p->target->debug))
                    fprintf(stderr, "apps.plugin: \t\tTARGET INHERITANCE: %s is inherited by %d (%s) from its parent %d (%s) at phase 2.\n", p->target->name, p->pid, p->comm, p->parent->pid, p->parent->comm);
            }
        }
    }

    if(unlikely(debug))
        fprintf(stderr, "apps.plugin: apply_apps_groups_targets_inheritance() made %d loops on the process tree\n", loops);
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

        w->statm_size = 0;
        w->statm_resident = 0;
        w->statm_share = 0;
        // w->statm_text = 0;
        // w->statm_lib = 0;
        // w->statm_data = 0;
        // w->statm_dirty = 0;

        w->io_logical_bytes_read = 0;
        w->io_logical_bytes_written = 0;
        // w->io_read_calls = 0;
        // w->io_write_calls = 0;
        w->io_storage_bytes_read = 0;
        w->io_storage_bytes_written = 0;
        // w->io_cancelled_write_bytes = 0;

        // zero file counters
        if(w->target_fds) {
            memset(w->target_fds, 0, sizeof(int) * w->target_fds_size);
            w->openfiles = 0;
            w->openpipes = 0;
            w->opensockets = 0;
            w->openinotifies = 0;
            w->openeventfds = 0;
            w->opentimerfds = 0;
            w->opensignalfds = 0;
            w->openeventpolls = 0;
            w->openother = 0;
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

    switch(all_files[fd].type) {
        case FILETYPE_FILE:
            w->openfiles++;
            break;

        case FILETYPE_PIPE:
            w->openpipes++;
            break;

        case FILETYPE_SOCKET:
            w->opensockets++;
            break;

        case FILETYPE_INOTIFY:
            w->openinotifies++;
            break;

        case FILETYPE_EVENTFD:
            w->openeventfds++;
            break;

        case FILETYPE_TIMERFD:
            w->opentimerfds++;
            break;

        case FILETYPE_SIGNALFD:
            w->opensignalfds++;
            break;

        case FILETYPE_EVENTPOLL:
            w->openeventpolls++;
            break;

        case FILETYPE_OTHER:
            w->openother++;
            break;
    }
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

    int c, size = p->fds_size, *fds = p->fds;
    for(c = 0; c < size ;c++) {
        int fd = fds[c];

        if(likely(fd <= 0 || fd >= all_files_size))
            continue;

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
        error("pid %d %s was left without a target!", p->pid, p->comm);
        return;
    }

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

    w->statm_size += p->statm_size;
    w->statm_resident += p->statm_resident;
    w->statm_share += p->statm_share;
    // w->statm_text += p->statm_text;
    // w->statm_lib += p->statm_lib;
    // w->statm_data += p->statm_data;
    // w->statm_dirty += p->statm_dirty;

    w->io_logical_bytes_read    += p->io_logical_bytes_read;
    w->io_logical_bytes_written += p->io_logical_bytes_written;
    // w->io_read_calls            += p->io_read_calls;
    // w->io_write_calls           += p->io_write_calls;
    w->io_storage_bytes_read    += p->io_storage_bytes_read;
    w->io_storage_bytes_written += p->io_storage_bytes_written;
    // w->io_cancelled_write_bytes += p->io_cancelled_write_bytes;

    w->processes++;
    w->num_threads += p->num_threads;

    if(unlikely(debug || w->debug))
        fprintf(stderr, "apps.plugin: \taggregating '%s' pid %d on target '%s' utime=" KERNEL_UINT_FORMAT ", stime=" KERNEL_UINT_FORMAT ", gtime=" KERNEL_UINT_FORMAT ", cutime=" KERNEL_UINT_FORMAT ", cstime=" KERNEL_UINT_FORMAT ", cgtime=" KERNEL_UINT_FORMAT ", minflt=" KERNEL_UINT_FORMAT ", majflt=" KERNEL_UINT_FORMAT ", cminflt=" KERNEL_UINT_FORMAT ", cmajflt=" KERNEL_UINT_FORMAT "\n", p->comm, p->pid, w->name, p->utime, p->stime, p->gtime, p->cutime, p->cstime, p->cgtime, p->minflt, p->majflt, p->cminflt, p->cmajflt);
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
            if(unlikely(debug && p->user_target))
                    fprintf(stderr, "apps.plugin: \t\tpid %d (%s) switched user from %u (%s) to %u.\n", p->pid, p->comm, p->user_target->uid, p->user_target->name, p->uid);

            w = p->user_target = get_users_target(p->uid);
        }

        aggregate_pid_on_target(w, p, o);


        // --------------------------------------------------------------------
        // user group target

        o = p->group_target;
        if(likely(p->group_target && p->group_target->gid == p->gid))
            w = p->group_target;
        else {
            if(unlikely(debug && p->group_target))
                    fprintf(stderr, "apps.plugin: \t\tpid %d (%s) switched group from %u (%s) to %u.\n", p->pid, p->comm, p->group_target->gid, p->group_target->name, p->gid);

            w = p->group_target = get_groups_target(p->gid);
        }

        aggregate_pid_on_target(w, p, o);


        // --------------------------------------------------------------------
        // aggregate all file descriptors

        if(enable_file_charts)
            aggregate_pid_fds_on_targets(p);
    }

    cleanup_exited_pids();
}

// ----------------------------------------------------------------------------
// update chart dimensions

int print_calculated_number(char *str, calculated_number value) { (void)str; (void)value; return 0; }

static inline void send_BEGIN(const char *type, const char *id, usec_t usec) {
    fprintf(stdout, "BEGIN %s.%s %llu\n", type, id, usec);
}

static inline void send_SET(const char *name, kernel_uint_t value) {
    fprintf(stdout, "SET %s = " KERNEL_UINT_FORMAT "\n", name, value);
}

static inline void send_END(void) {
    fprintf(stdout, "END\n");
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
                "DIMENSION pids '' absolute 1 1\n"
                "DIMENSION fds '' absolute 1 1\n"
                "DIMENSION targets '' absolute 1 1\n"
                "DIMENSION new_pids 'new pids' incremental 1 1\n"
                , update_every
        );

#if (ALL_PIDS_ARE_READ_INSTANTLY == 0)
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
#endif

    }

    fprintf(stdout,
        "BEGIN netdata.apps_cpu %llu\n"
        "SET user = %llu\n"
        "SET system = %llu\n"
        "END\n"
        "BEGIN netdata.apps_sizes %llu\n"
        "SET calls = %zu\n"
        "SET files = %zu\n"
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
        , all_pids_count
        , all_files_len
        , apps_groups_targets_count
        , targets_assignment_counter
        );

#if (ALL_PIDS_ARE_READ_INSTANTLY == 0)
    fprintf(stdout,
            "BEGIN netdata.apps_fix %llu\n"
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
            "BEGIN netdata.apps_children_fix %llu\n"
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
#endif
}

#if (ALL_PIDS_ARE_READ_INSTANTLY == 0)
static void normalize_utilization(struct target *root) {
    struct target *w;

    // childs processing introduces spikes
    // here we try to eliminate them by disabling childs processing either for specific dimensions
    // or entirely. Of course, either way, we disable it just a single iteration.

    kernel_uint_t max_time = processors * hz * RATES_DETAIL;
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

    if((global_utime || global_stime || global_gtime) && (utime || stime || gtime)) {
        if(global_utime + global_stime + global_gtime > utime + cutime + stime + cstime + gtime + cgtime) {
            // everything we collected fits
            utime_fix_ratio  =
            stime_fix_ratio  =
            gtime_fix_ratio  =
            cutime_fix_ratio =
            cstime_fix_ratio =
            cgtime_fix_ratio = 1.0; //(double)(global_utime + global_stime) / (double)(utime + cutime + stime + cstime);
        }
        else if(global_utime + global_stime > utime + stime) {
            // childrens resources are too high
            // lower only the children resources
            utime_fix_ratio  =
            stime_fix_ratio  =
            gtime_fix_ratio  = 1.0;
            cutime_fix_ratio =
            cstime_fix_ratio =
            cgtime_fix_ratio = (double)((global_utime + global_stime) - (utime + stime)) / (double)(cutime + cstime);
        }
        else {
            // even running processes are unrealistic
            // zero the children resources
            // lower the running processes resources
            utime_fix_ratio  =
            stime_fix_ratio  =
            gtime_fix_ratio  = (double)(global_utime + global_stime) / (double)(utime + stime);
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

    // FIXME
    // we use cpu time to normalize page faults
    // the problem is that to find the proper max values
    // for page faults we have to parse /proc/vmstat
    // which is quite big to do it again (netdata does it already)
    //
    // a better solution could be to somehow have netdata
    // do this normalization for us

    if(utime || stime || gtime)
        majflt_fix_ratio =
        minflt_fix_ratio = (double)(utime * utime_fix_ratio + stime * stime_fix_ratio + gtime * gtime_fix_ratio) / (double)(utime + stime + gtime);
    else
        minflt_fix_ratio =
        majflt_fix_ratio = 1.0;

    if(cutime || cstime || cgtime)
        cmajflt_fix_ratio =
        cminflt_fix_ratio = (double)(cutime * cutime_fix_ratio + cstime * cstime_fix_ratio + cgtime * cgtime_fix_ratio) / (double)(cutime + cstime + cgtime);
    else
        cminflt_fix_ratio =
        cmajflt_fix_ratio = 1.0;

    // the report

    if(unlikely(debug)) {
        fprintf(stderr,
            "SYSTEM: u=" KERNEL_UINT_FORMAT " s=" KERNEL_UINT_FORMAT " g=" KERNEL_UINT_FORMAT " "
            "COLLECTED: u=" KERNEL_UINT_FORMAT " s=" KERNEL_UINT_FORMAT " g=" KERNEL_UINT_FORMAT " cu=" KERNEL_UINT_FORMAT " cs=" KERNEL_UINT_FORMAT " cg=" KERNEL_UINT_FORMAT " "
            "DELTA: u=" KERNEL_UINT_FORMAT " s=" KERNEL_UINT_FORMAT " g=" KERNEL_UINT_FORMAT " "
            "FIX: u=%0.2f s=%0.2f g=%0.2f cu=%0.2f cs=%0.2f cg=%0.2f "
            "FINALLY: u=" KERNEL_UINT_FORMAT " s=" KERNEL_UINT_FORMAT " g=" KERNEL_UINT_FORMAT " cu=" KERNEL_UINT_FORMAT " cs=" KERNEL_UINT_FORMAT " cg=" KERNEL_UINT_FORMAT " "
            "\n"
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
}
#else // ALL_PIDS_ARE_READ_INSTANTLY == 1
static void normalize_utilization(struct target *root) {
    (void)root;
}
#endif // ALL_PIDS_ARE_READ_INSTANTLY

static void send_collected_data_to_netdata(struct target *root, const char *type, usec_t dt) {
    struct target *w;

    send_BEGIN(type, "cpu", dt);
    for (w = root; w ; w = w->next) {
        if(unlikely(w->exposed))
            send_SET(w->name, (kernel_uint_t)(w->utime * utime_fix_ratio) + (kernel_uint_t)(w->stime * stime_fix_ratio) + (kernel_uint_t)(w->gtime * gtime_fix_ratio) + (include_exited_childs?((kernel_uint_t)(w->cutime * cutime_fix_ratio) + (kernel_uint_t)(w->cstime * cstime_fix_ratio) + (kernel_uint_t)(w->cgtime * cgtime_fix_ratio)):0ULL));
    }
    send_END();

    send_BEGIN(type, "cpu_user", dt);
    for (w = root; w ; w = w->next) {
        if(unlikely(w->exposed))
            send_SET(w->name, (kernel_uint_t)(w->utime * utime_fix_ratio) + (include_exited_childs?((kernel_uint_t)(w->cutime * cutime_fix_ratio)):0ULL));
    }
    send_END();

    send_BEGIN(type, "cpu_system", dt);
    for (w = root; w ; w = w->next) {
        if(unlikely(w->exposed))
            send_SET(w->name, (kernel_uint_t)(w->stime * stime_fix_ratio) + (include_exited_childs?((kernel_uint_t)(w->cstime * cstime_fix_ratio)):0ULL));
    }
    send_END();

    if(show_guest_time) {
        send_BEGIN(type, "cpu_guest", dt);
        for (w = root; w ; w = w->next) {
            if(unlikely(w->exposed))
                send_SET(w->name, (kernel_uint_t)(w->gtime * gtime_fix_ratio) + (include_exited_childs?((kernel_uint_t)(w->cgtime * cgtime_fix_ratio)):0ULL));
        }
        send_END();
    }

    send_BEGIN(type, "threads", dt);
    for (w = root; w ; w = w->next) {
        if(unlikely(w->exposed))
            send_SET(w->name, w->num_threads);
    }
    send_END();

    send_BEGIN(type, "processes", dt);
    for (w = root; w ; w = w->next) {
        if(unlikely(w->exposed))
            send_SET(w->name, w->processes);
    }
    send_END();

    send_BEGIN(type, "mem", dt);
    for (w = root; w ; w = w->next) {
        if(unlikely(w->exposed))
            send_SET(w->name, (w->statm_resident > w->statm_share)?(w->statm_resident - w->statm_share):0ULL);
    }
    send_END();

    send_BEGIN(type, "vmem", dt);
    for (w = root; w ; w = w->next) {
        if(unlikely(w->exposed))
            send_SET(w->name, w->statm_size);
    }
    send_END();

    send_BEGIN(type, "minor_faults", dt);
    for (w = root; w ; w = w->next) {
        if(unlikely(w->exposed))
            send_SET(w->name, (kernel_uint_t)(w->minflt * minflt_fix_ratio) + (include_exited_childs?((kernel_uint_t)(w->cminflt * cminflt_fix_ratio)):0ULL));
    }
    send_END();

    send_BEGIN(type, "major_faults", dt);
    for (w = root; w ; w = w->next) {
        if(unlikely(w->exposed))
            send_SET(w->name, (kernel_uint_t)(w->majflt * majflt_fix_ratio) + (include_exited_childs?((kernel_uint_t)(w->cmajflt * cmajflt_fix_ratio)):0ULL));
    }
    send_END();

#ifndef __FreeBSD__
    send_BEGIN(type, "lreads", dt);
    for (w = root; w ; w = w->next) {
        if(unlikely(w->exposed))
            send_SET(w->name, w->io_logical_bytes_read);
    }
    send_END();

    send_BEGIN(type, "lwrites", dt);
    for (w = root; w ; w = w->next) {
        if(unlikely(w->exposed))
            send_SET(w->name, w->io_logical_bytes_written);
    }
    send_END();
#endif

    send_BEGIN(type, "preads", dt);
    for (w = root; w ; w = w->next) {
        if(unlikely(w->exposed))
            send_SET(w->name, w->io_storage_bytes_read);
    }
    send_END();

    send_BEGIN(type, "pwrites", dt);
    for (w = root; w ; w = w->next) {
        if(unlikely(w->exposed))
            send_SET(w->name, w->io_storage_bytes_written);
    }
    send_END();

    if(enable_file_charts) {
        send_BEGIN(type, "files", dt);
        for (w = root; w; w = w->next) {
            if (unlikely(w->exposed))
                send_SET(w->name, w->openfiles);
        }
        send_END();

        send_BEGIN(type, "sockets", dt);
        for (w = root; w; w = w->next) {
            if (unlikely(w->exposed))
                send_SET(w->name, w->opensockets);
        }
        send_END();

        send_BEGIN(type, "pipes", dt);
        for (w = root; w; w = w->next) {
            if (unlikely(w->exposed))
                send_SET(w->name, w->openpipes);
        }
        send_END();
    }
}


// ----------------------------------------------------------------------------
// generate the charts

static void send_charts_updates_to_netdata(struct target *root, const char *type, const char *title)
{
    struct target *w;
    int newly_added = 0;

    for(w = root ; w ; w = w->next) {
        if (w->target) continue;

        if (!w->exposed && w->processes) {
            newly_added++;
            w->exposed = 1;
            if (debug || w->debug) fprintf(stderr, "apps.plugin: %s just added - regenerating charts.\n", w->name);
        }
    }

    // nothing more to show
    if(!newly_added && show_guest_time == show_guest_time_old) return;

    // we have something new to show
    // update the charts
    fprintf(stdout, "CHART %s.cpu '' '%s CPU Time (%d%% = %d core%s)' 'cpu time %%' cpu %s.cpu stacked 20001 %d\n", type, title, (processors * 100), processors, (processors>1)?"s":"", type, update_every);
    for (w = root; w ; w = w->next) {
        if(unlikely(w->exposed))
            fprintf(stdout, "DIMENSION %s '' absolute 1 %llu %s\n", w->name, hz * RATES_DETAIL / 100, w->hidden ? "hidden" : "");
    }

    fprintf(stdout, "CHART %s.mem '' '%s Real Memory (w/o shared)' 'MB' mem %s.mem stacked 20003 %d\n", type, title, type, update_every);
    for (w = root; w ; w = w->next) {
        if(unlikely(w->exposed))
            fprintf(stdout, "DIMENSION %s '' absolute %ld %ld\n", w->name, sysconf(_SC_PAGESIZE), 1024L*1024L);
    }

    fprintf(stdout, "CHART %s.vmem '' '%s Virtual Memory Size' 'MB' mem %s.vmem stacked 20004 %d\n", type, title, type, update_every);
    for (w = root; w ; w = w->next) {
        if(unlikely(w->exposed))
            fprintf(stdout, "DIMENSION %s '' absolute %ld %ld\n", w->name, sysconf(_SC_PAGESIZE), 1024L*1024L);
    }

    fprintf(stdout, "CHART %s.threads '' '%s Threads' 'threads' processes %s.threads stacked 20005 %d\n", type, title, type, update_every);
    for (w = root; w ; w = w->next) {
        if(unlikely(w->exposed))
            fprintf(stdout, "DIMENSION %s '' absolute 1 1\n", w->name);
    }

    fprintf(stdout, "CHART %s.processes '' '%s Processes' 'processes' processes %s.processes stacked 20004 %d\n", type, title, type, update_every);
    for (w = root; w ; w = w->next) {
        if(unlikely(w->exposed))
            fprintf(stdout, "DIMENSION %s '' absolute 1 1\n", w->name);
    }

    fprintf(stdout, "CHART %s.cpu_user '' '%s CPU User Time (%d%% = %d core%s)' 'cpu time %%' cpu %s.cpu_user stacked 20020 %d\n", type, title, (processors * 100), processors, (processors>1)?"s":"", type, update_every);
    for (w = root; w ; w = w->next) {
        if(unlikely(w->exposed))
            fprintf(stdout, "DIMENSION %s '' absolute 1 %llu\n", w->name, hz * RATES_DETAIL / 100LLU);
    }

    fprintf(stdout, "CHART %s.cpu_system '' '%s CPU System Time (%d%% = %d core%s)' 'cpu time %%' cpu %s.cpu_system stacked 20021 %d\n", type, title, (processors * 100), processors, (processors>1)?"s":"", type, update_every);
    for (w = root; w ; w = w->next) {
        if(unlikely(w->exposed))
            fprintf(stdout, "DIMENSION %s '' absolute 1 %llu\n", w->name, hz * RATES_DETAIL / 100LLU);
    }

    if(show_guest_time) {
        fprintf(stdout, "CHART %s.cpu_guest '' '%s CPU Guest Time (%d%% = %d core%s)' 'cpu time %%' cpu %s.cpu_system stacked 20022 %d\n", type, title, (processors * 100), processors, (processors > 1) ? "s" : "", type, update_every);
        for (w = root; w; w = w->next) {
            if(unlikely(w->exposed))
                fprintf(stdout, "DIMENSION %s '' absolute 1 %llu\n", w->name, hz * RATES_DETAIL / 100LLU);
        }
    }

    fprintf(stdout, "CHART %s.major_faults '' '%s Major Page Faults (swap read)' 'page faults/s' swap %s.major_faults stacked 20010 %d\n", type, title, type, update_every);
    for (w = root; w ; w = w->next) {
        if(unlikely(w->exposed))
            fprintf(stdout, "DIMENSION %s '' absolute 1 %llu\n", w->name, RATES_DETAIL);
    }

    fprintf(stdout, "CHART %s.minor_faults '' '%s Minor Page Faults' 'page faults/s' mem %s.minor_faults stacked 20011 %d\n", type, title, type, update_every);
    for (w = root; w ; w = w->next) {
        if(unlikely(w->exposed))
            fprintf(stdout, "DIMENSION %s '' absolute 1 %llu\n", w->name, RATES_DETAIL);
    }

#ifdef __FreeBSD__
    fprintf(stdout, "CHART %s.preads '' '%s Disk Reads' 'blocks/s' disk %s.preads stacked 20002 %d\n", type, title, type, update_every);
    for (w = root; w ; w = w->next) {
        if(unlikely(w->exposed))
            fprintf(stdout, "DIMENSION %s '' absolute 1 %llu\n", w->name, RATES_DETAIL);
    }

    fprintf(stdout, "CHART %s.pwrites '' '%s Disk Writes' 'blocks/s' disk %s.pwrites stacked 20002 %d\n", type, title, type, update_every);
    for (w = root; w ; w = w->next) {
        if(unlikely(w->exposed))
            fprintf(stdout, "DIMENSION %s '' absolute 1 %llu\n", w->name, RATES_DETAIL);
    }
#else
    fprintf(stdout, "CHART %s.preads '' '%s Disk Reads' 'kilobytes/s' disk %s.preads stacked 20002 %d\n", type, title, type, update_every);
    for (w = root; w ; w = w->next) {
        if(unlikely(w->exposed))
            fprintf(stdout, "DIMENSION %s '' absolute 1 %llu\n", w->name, 1024LLU * RATES_DETAIL);
    }

    fprintf(stdout, "CHART %s.pwrites '' '%s Disk Writes' 'kilobytes/s' disk %s.pwrites stacked 20002 %d\n", type, title, type, update_every);
    for (w = root; w ; w = w->next) {
        if(unlikely(w->exposed))
            fprintf(stdout, "DIMENSION %s '' absolute 1 %llu\n", w->name, 1024LLU * RATES_DETAIL);
    }

    fprintf(stdout, "CHART %s.lreads '' '%s Disk Logical Reads' 'kilobytes/s' disk %s.lreads stacked 20042 %d\n", type, title, type, update_every);
    for (w = root; w ; w = w->next) {
        if(unlikely(w->exposed))
            fprintf(stdout, "DIMENSION %s '' absolute 1 %llu\n", w->name, 1024LLU * RATES_DETAIL);
    }

    fprintf(stdout, "CHART %s.lwrites '' '%s I/O Logical Writes' 'kilobytes/s' disk %s.lwrites stacked 20042 %d\n", type, title, type, update_every);
    for (w = root; w ; w = w->next) {
        if(unlikely(w->exposed))
            fprintf(stdout, "DIMENSION %s '' absolute 1 %llu\n", w->name, 1024LLU * RATES_DETAIL);
    }
#endif

    if(enable_file_charts) {
        fprintf(stdout, "CHART %s.files '' '%s Open Files' 'open files' disk %s.files stacked 20050 %d\n", type,
                       title, type, update_every);
        for (w = root; w; w = w->next) {
            if (unlikely(w->exposed))
                fprintf(stdout, "DIMENSION %s '' absolute 1 1\n", w->name);
        }

        fprintf(stdout, "CHART %s.sockets '' '%s Open Sockets' 'open sockets' net %s.sockets stacked 20051 %d\n",
                       type, title, type, update_every);
        for (w = root; w; w = w->next) {
            if (unlikely(w->exposed))
                fprintf(stdout, "DIMENSION %s '' absolute 1 1\n", w->name);
        }

        fprintf(stdout, "CHART %s.pipes '' '%s Pipes' 'open pipes' processes %s.pipes stacked 20053 %d\n", type,
                       title, type, update_every);
        for (w = root; w; w = w->next) {
            if (unlikely(w->exposed))
                fprintf(stdout, "DIMENSION %s '' absolute 1 1\n", w->name);
        }
    }
}


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
    char *name = NULL;

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
            debug = 1;
            // debug_flags = 0xffffffff;
            continue;
        }

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
                    " SECONDS           set the data collection frequency\n"
                    "\n"
                    " debug             enable debugging (lot of output)\n"
                    "\n"
                    " with-childs\n"
                    " without-childs    enable / disable aggregating exited\n"
                    "                   children resources into parents\n"
                    "                   (default is enabled)\n"
                    "\n"
                    " with-guest\n"
                    " without-guest     enable / disable reporting guest charts\n"
                    "                   (default is disabled)\n"
                    "\n"
                    " with-files\n"
                    " without-files     enable / disable reporting files, sockets, pipes\n"
                    "                   (default is enabled)\n"
                    "\n"
                    " NAME              read apps_NAME.conf instead of\n"
                    "                   apps_groups.conf\n"
                    "                   (default NAME=groups)\n"
                    "\n"
                    " version or -v or -V print program version and exit\n"
                    "\n"
                    , VERSION
            );
            exit(1);
        }

        if(!name) {
            name = argv[i];
            continue;
        }

        error("Cannot understand option %s", argv[i]);
        exit(1);
    }

    if(freq > 0) update_every = freq;
    if(!name) name = "groups";

    if(read_apps_groups_conf(name)) {
        error("Cannot read process groups '%s/apps_%s.conf'. There are no internal defaults. Failing.", config_dir, name);
        exit(1);
    }
}

static int am_i_running_as_root() {
    uid_t uid = getuid(), euid = geteuid();

    if(uid == 0 || euid == 0) {
        if(debug) info("I am running with escalated privileges, uid = %u, euid = %u.", uid, euid);
        return 1;
    }

    if(debug) info("I am not running with escalated privileges, uid = %u, euid = %u.", uid, euid);
    return 0;
}

#ifdef HAVE_CAPABILITY
static int check_capabilities() {
    cap_t caps = cap_get_proc();
    if(!caps) {
        error("Cannot get current capabilities.");
        return 0;
    }
    else if(debug)
        info("Received my capabilities from the system.");

    int ret = 1;

    cap_flag_value_t cfv = CAP_CLEAR;
    if(cap_get_flag(caps, CAP_DAC_READ_SEARCH, CAP_EFFECTIVE, &cfv) == -1) {
        error("Cannot find if CAP_DAC_READ_SEARCH is effective.");
        ret = 0;
    }
    else {
        if(cfv != CAP_SET) {
            error("apps.plugin should run with CAP_DAC_READ_SEARCH.");
            ret = 0;
        }
        else if(debug)
            info("apps.plugin runs with CAP_DAC_READ_SEARCH.");
    }

    cfv = CAP_CLEAR;
    if(cap_get_flag(caps, CAP_SYS_PTRACE, CAP_EFFECTIVE, &cfv) == -1) {
        error("Cannot find if CAP_SYS_PTRACE is effective.");
        ret = 0;
    }
    else {
        if(cfv != CAP_SET) {
            error("apps.plugin should run with CAP_SYS_PTRACE.");
            ret = 0;
        }
        else if(debug)
            info("apps.plugin runs with CAP_SYS_PTRACE.");
    }

    cap_free(caps);

    return ret;
}
#else
static int check_capabilities() {
    return 0;
}
#endif

int main(int argc, char **argv) {
    // debug_flags = D_PROCFILE;

    // set the name for logging
    program_name = "apps.plugin";

    // disable syslog for apps.plugin
    error_log_syslog = 0;

    // set errors flood protection to 100 logs per hour
    error_log_errors_per_period = 100;
    error_log_throttle_period = 3600;

    netdata_configured_host_prefix = getenv("NETDATA_HOST_PREFIX");
    if(netdata_configured_host_prefix == NULL) {
        // info("NETDATA_HOST_PREFIX is not passed from netdata");
        netdata_configured_host_prefix = "";
    }
    // else info("Found NETDATA_HOST_PREFIX='%s'", netdata_configured_host_prefix);

    config_dir = getenv("NETDATA_CONFIG_DIR");
    if(config_dir == NULL) {
        // info("NETDATA_CONFIG_DIR is not passed from netdata");
        config_dir = CONFIG_DIR;
    }
    // else info("Found NETDATA_CONFIG_DIR='%s'", config_dir);

#ifdef NETDATA_INTERNAL_CHECKS
    if(debug_flags != 0) {
        struct rlimit rl = { RLIM_INFINITY, RLIM_INFINITY };
        if(setrlimit(RLIMIT_CORE, &rl) != 0)
            info("Cannot request unlimited core dumps for debugging... Proceeding anyway...");
#ifdef HAVE_SYS_PRCTL_H
        prctl(PR_SET_DUMPABLE, 1, 0, 0, 0);
#endif
    }
#endif /* NETDATA_INTERNAL_CHECKS */

    procfile_adaptive_initial_allocation = 1;

    time_t started_t = now_monotonic_sec();
    get_system_HZ();
    get_system_pid_max();
    get_system_cpus();

    parse_args(argc, argv);

    if(!check_capabilities() && !am_i_running_as_root() && !check_proc_1_io()) {
        uid_t uid = getuid(), euid = geteuid();
#ifdef HAVE_CAPABILITY
        error("apps.plugin should either run as root (now running with uid %u, euid %u) or have special capabilities. "
                      "Without these, apps.plugin cannot report disk I/O utilization of other processes. "
                      "To enable capabilities run: sudo setcap cap_dac_read_search,cap_sys_ptrace+ep %s; "
                      "To enable setuid to root run: sudo chown root %s; sudo chmod 4755 %s; "
              , uid, euid, argv[0], argv[0], argv[0]
        );
#else
        error("apps.plugin should either run as root (now running with uid %u, euid %u) or have special capabilities. "
                      "Without these, apps.plugin cannot report disk I/O utilization of other processes. "
                      "Your system does not support capabilities. "
                      "To enable setuid to root run: sudo chown root %s; sudo chmod 4755 %s; "
              , uid, euid, argv[0], argv[0]
        );
#endif
    }

    info("started on pid %d", getpid());

#if (ALL_PIDS_ARE_READ_INSTANTLY == 0)
    all_pids_sortlist = callocz(sizeof(pid_t), (size_t)pid_max);
#endif

    all_pids          = callocz(sizeof(struct pid_stat *), (size_t) pid_max);

    usec_t step = update_every * USEC_PER_SEC;
    global_iterations_counter = 1;
    heartbeat_t hb;
    heartbeat_init(&hb);
    for(;1; global_iterations_counter++) {

#ifdef NETDATA_PROFILING
#warning "compiling for profiling"
        static int profiling_count=0;
        profiling_count++;
        if(unlikely(profiling_count > 1000)) exit(0);
        usec_t dt = update_every * USEC_PER_SEC;
#else
        usec_t dt = heartbeat_next(&hb, step);
#endif

        if(!collect_data_for_all_processes()) {
            error("Cannot collect /proc data for running processes. Disabling apps.plugin...");
            printf("DISABLE\n");
            exit(1);
        }

        calculate_netdata_statistics();
        normalize_utilization(apps_groups_root_target);

        send_resource_usage_to_netdata(dt);

        // this is smart enough to show only newly added apps, when needed
        send_charts_updates_to_netdata(apps_groups_root_target, "apps", "Apps");

        if(likely(enable_users_charts))
            send_charts_updates_to_netdata(users_root_target, "users", "Users");

        if(likely(enable_groups_charts))
            send_charts_updates_to_netdata(groups_root_target, "groups", "User Groups");

        send_collected_data_to_netdata(apps_groups_root_target, "apps", dt);

        if(likely(enable_users_charts))
            send_collected_data_to_netdata(users_root_target, "users", dt);

        if(likely(enable_groups_charts))
            send_collected_data_to_netdata(groups_root_target, "groups", dt);

        fflush(stdout);

        show_guest_time_old = show_guest_time;

        if(unlikely(debug))
            fprintf(stderr, "apps.plugin: done Loop No %zu\n", global_iterations_counter);

        // restart check (14400 seconds)
        if(now_monotonic_sec() - started_t > 14400) exit(0);
    }
}
