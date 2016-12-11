#include "common.h"

#define MAX_COMPARE_NAME 100
#define MAX_NAME 100
#define MAX_CMDLINE 1024

// the rates we are going to send to netdata
// will have this detail
// a value of:
// 1 will send just integer parts to netdata
// 100 will send 2 decimal points
// 1000 will send 3 decimal points
// etc.
#define RATES_DETAIL 10000ULL

int debug = 0;

int update_every = 1;
unsigned long long global_iterations_counter = 1;
unsigned long long file_counter = 0;
int proc_pid_cmdline_is_needed = 0;
int include_exited_childs = 1;
char *config_dir = CONFIG_DIR;

pid_t *all_pids_sortlist = NULL;

// will be automatically set to 1, if guest values are collected
int show_guest_time = 0;
int show_guest_time_old = 0;

int enable_guest_charts = 0;
int enable_file_charts = 1;
int enable_users_charts = 1;
int enable_groups_charts = 1;

// ----------------------------------------------------------------------------

void netdata_cleanup_and_exit(int ret) {
    exit(ret);
}


// ----------------------------------------------------------------------------
// target
// target is the structure that process data are aggregated

struct target {
    char compare[MAX_COMPARE_NAME + 1];
    uint32_t comparehash;
    size_t comparelen;

    char id[MAX_NAME + 1];
    uint32_t idhash;

    char name[MAX_NAME + 1];

    uid_t uid;
    gid_t gid;

    unsigned long long minflt;
    unsigned long long cminflt;
    unsigned long long majflt;
    unsigned long long cmajflt;
    unsigned long long utime;
    unsigned long long stime;
    unsigned long long gtime;
    unsigned long long cutime;
    unsigned long long cstime;
    unsigned long long cgtime;
    unsigned long long num_threads;
    // unsigned long long rss;

    unsigned long long statm_size;
    unsigned long long statm_resident;
    unsigned long long statm_share;
    // unsigned long long statm_text;
    // unsigned long long statm_lib;
    // unsigned long long statm_data;
    // unsigned long long statm_dirty;

    unsigned long long io_logical_bytes_read;
    unsigned long long io_logical_bytes_written;
    // unsigned long long io_read_calls;
    // unsigned long long io_write_calls;
    unsigned long long io_storage_bytes_read;
    unsigned long long io_storage_bytes_written;
    // unsigned long long io_cancelled_write_bytes;

    int *fds;
    unsigned long long openfiles;
    unsigned long long openpipes;
    unsigned long long opensockets;
    unsigned long long openinotifies;
    unsigned long long openeventfds;
    unsigned long long opentimerfds;
    unsigned long long opensignalfds;
    unsigned long long openeventpolls;
    unsigned long long openother;

    unsigned long processes;    // how many processes have been merged to this
    int exposed;                // if set, we have sent this to netdata
    int hidden;                 // if set, we set the hidden flag on the dimension
    int debug;
    int ends_with;
    int starts_with;            // if set, the compare string matches only the
                                // beginning of the command

    struct target *target;      // the one that will be reported to netdata
    struct target *next;
};


// ----------------------------------------------------------------------------
// apps_groups.conf
// aggregate all processes in groups, to have a limited number of dimensions

struct target *apps_groups_root_target = NULL;
struct target *apps_groups_default_target = NULL;
long apps_groups_targets = 0;

struct target *users_root_target = NULL;
struct target *groups_root_target = NULL;

struct target *get_users_target(uid_t uid)
{
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
    if(!pw)
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
    if(!gr)
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
struct target *get_apps_groups_target(const char *id, struct target *target) {
    int tdebug = 0, thidden = 0, ends_with = 0;
    const char *nid = id;

    while(nid[0] == '-' || nid[0] == '+' || nid[0] == '*') {
        if(nid[0] == '-') thidden = 1;
        if(nid[0] == '+') tdebug = 1;
        if(nid[0] == '*') ends_with = 1;
        nid++;
    }
    uint32_t hash = simple_hash(id);

    struct target *w, *last = apps_groups_root_target;
    for(w = apps_groups_root_target ; w ; w = w->next) {
        if(w->idhash == hash && strncmp(nid, w->id, MAX_NAME) == 0)
            return w;

        last = w;
    }

    w = callocz(sizeof(struct target), 1);
    strncpyz(w->id, nid, MAX_NAME);
    w->idhash = simple_hash(w->id);

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
                , w->target?w->target->id:w->id
                , (w->hidden)?"hidden":"-"
                , (w->debug)?"debug":"-"
        );

    return w;
}

// read the apps_groups.conf file
int read_apps_groups_conf(const char *name)
{
    char filename[FILENAME_MAX + 1];

    snprintfz(filename, FILENAME_MAX, "%s/apps_%s.conf", config_dir, name);

    if(unlikely(debug))
        fprintf(stderr, "apps.plugin: process groups file: '%s'\n", filename);

    // ----------------------------------------

    procfile *ff = procfile_open(filename, " :\t", PROCFILE_FLAG_DEFAULT);
    if(!ff) return 1;

    procfile_set_quotes(ff, "'\"");

    ff = procfile_readall(ff);
    if(!ff)
        return 1;

    unsigned long line, lines = procfile_lines(ff);

    for(line = 0; line < lines ;line++) {
        unsigned long word, words = procfile_linewords(ff, line);
        struct target *w = NULL;

        char *t = procfile_lineword(ff, line, 0);
        if(!t || !*t) continue;

        for(word = 0; word < words ;word++) {
            char *s = procfile_lineword(ff, line, word);
            if(!s || !*s) continue;
            if(*s == '#') break;

            if(t == s) continue;

            struct target *n = get_apps_groups_target(s, w);
            if(!n) {
                error("Cannot create target '%s' (line %lu, word %lu)", s, line, word);
                continue;
            }

            if(!w) w = n;
        }

        if(w) {
            int tdebug = 0, thidden = 0;

            while(t[0] == '-' || t[0] == '+') {
                if(t[0] == '-') thidden = 1;
                if(t[0] == '+') tdebug = 1;
                t++;
            }

            strncpyz(w->name, t, MAX_NAME);
            w->hidden = thidden;
            w->debug = tdebug;

            if(unlikely(debug))
                fprintf(stderr, "apps.plugin: AGGREGATION TARGET NAME '%s' on ID '%s', process name '%s' (%s), aggregated on target '%s', options: %s %s\n"
                        , w->name
                        , w->id
                        , w->compare, (w->starts_with && w->ends_with)?"substring":((w->starts_with)?"prefix":((w->ends_with)?"suffix":"exact"))
                        , w->target?w->target->id:w->id
                        , (w->hidden)?"hidden":"-"
                        , (w->debug)?"debug":"-"
                );
        }
    }

    procfile_close(ff);

    apps_groups_default_target = get_apps_groups_target("p+!o@w#e$i^r&7*5(-i)l-o_", NULL); // match nothing
    if(!apps_groups_default_target)
        error("Cannot create default target");
    else
        strncpyz(apps_groups_default_target->name, "other", MAX_NAME);

    return 0;
}


// ----------------------------------------------------------------------------
// data to store for each pid
// see: man proc

#define PID_LOG_IO      0x00000001
#define PID_LOG_STATM   0x00000002
#define PID_LOG_CMDLINE 0x00000004
#define PID_LOG_FDS     0x00000008
#define PID_LOG_STAT    0x00000010

struct pid_stat {
    int32_t pid;
    char comm[MAX_COMPARE_NAME + 1];
    char cmdline[MAX_CMDLINE + 1];

    uint32_t log_thrown;

    // char state;
    int32_t ppid;
    // int32_t pgrp;
    // int32_t session;
    // int32_t tty_nr;
    // int32_t tpgid;
    // uint64_t flags;

    // these are raw values collected
    unsigned long long minflt_raw;
    unsigned long long cminflt_raw;
    unsigned long long majflt_raw;
    unsigned long long cmajflt_raw;
    unsigned long long utime_raw;
    unsigned long long stime_raw;
    unsigned long long gtime_raw; // guest_time
    unsigned long long cutime_raw;
    unsigned long long cstime_raw;
    unsigned long long cgtime_raw; // cguest_time

    // these are rates
    unsigned long long minflt;
    unsigned long long cminflt;
    unsigned long long majflt;
    unsigned long long cmajflt;
    unsigned long long utime;
    unsigned long long stime;
    unsigned long long gtime;
    unsigned long long cutime;
    unsigned long long cstime;
    unsigned long long cgtime;

    // int64_t priority;
    // int64_t nice;
    int32_t num_threads;
    // int64_t itrealvalue;
    // unsigned long long starttime;
    // unsigned long long vsize;
    // unsigned long long rss;
    // unsigned long long rsslim;
    // unsigned long long starcode;
    // unsigned long long endcode;
    // unsigned long long startstack;
    // unsigned long long kstkesp;
    // unsigned long long kstkeip;
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
    // unsigned long long delayacct_blkio_ticks;

    uid_t uid;
    gid_t gid;

    unsigned long long statm_size;
    unsigned long long statm_resident;
    unsigned long long statm_share;
    // unsigned long long statm_text;
    // unsigned long long statm_lib;
    // unsigned long long statm_data;
    // unsigned long long statm_dirty;

    unsigned long long io_logical_bytes_read_raw;
    unsigned long long io_logical_bytes_written_raw;
    // unsigned long long io_read_calls_raw;
    // unsigned long long io_write_calls_raw;
    unsigned long long io_storage_bytes_read_raw;
    unsigned long long io_storage_bytes_written_raw;
    // unsigned long long io_cancelled_write_bytes_raw;

    unsigned long long io_logical_bytes_read;
    unsigned long long io_logical_bytes_written;
    // unsigned long long io_read_calls;
    // unsigned long long io_write_calls;
    unsigned long long io_storage_bytes_read;
    unsigned long long io_storage_bytes_written;
    // unsigned long long io_cancelled_write_bytes;

    int *fds;                       // array of fds it uses
    int fds_size;                   // the size of the fds array

    int children_count;             // number of processes directly referencing this
    int keep;                       // 1 when we need to keep this process in memory even after it exited
    int keeploops;                  // increases by 1 every time keep is 1 and updated 0
    int updated;                    // 1 when the process is currently running
    int merged;                     // 1 when it has been merged to its parent
    int new_entry;                  // 1 when this is a new process, just saw for the first time
    int read;                       // 1 when we have already read this process for this iteration
    int sortlist;                   // higher numbers = top on the process tree
                                    // each process gets a unique number

    struct target *target;          // app_groups.conf targets
    struct target *user_target;     // uid based targets
    struct target *group_target;    // gid based targets

    unsigned long long stat_collected_usec;
    unsigned long long last_stat_collected_usec;

    unsigned long long io_collected_usec;
    unsigned long long last_io_collected_usec;

    char *stat_filename;
    char *statm_filename;
    char *io_filename;
    char *cmdline_filename;

    struct pid_stat *parent;
    struct pid_stat *prev;
    struct pid_stat *next;
} *root_of_pids = NULL, **all_pids;

long all_pids_count = 0;

struct pid_stat *get_pid_entry(pid_t pid) {
    if(all_pids[pid]) {
        all_pids[pid]->new_entry = 0;
        return all_pids[pid];
    }

    all_pids[pid] = callocz(sizeof(struct pid_stat), 1);
    all_pids[pid]->fds = callocz(sizeof(int), 100);
    all_pids[pid]->fds_size = 100;

    if(root_of_pids) root_of_pids->prev = all_pids[pid];
    all_pids[pid]->next = root_of_pids;
    root_of_pids = all_pids[pid];

    all_pids[pid]->pid = pid;
    all_pids[pid]->new_entry = 1;

    all_pids_count++;

    return all_pids[pid];
}

void del_pid_entry(pid_t pid) {
    if(!all_pids[pid]) {
        error("attempted to free pid %d that is not allocated.", pid);
        return;
    }

    if(unlikely(debug))
        fprintf(stderr, "apps.plugin: process %d %s exited, deleting it.\n", pid, all_pids[pid]->comm);

    if(root_of_pids == all_pids[pid]) root_of_pids = all_pids[pid]->next;
    if(all_pids[pid]->next) all_pids[pid]->next->prev = all_pids[pid]->prev;
    if(all_pids[pid]->prev) all_pids[pid]->prev->next = all_pids[pid]->next;

    if(all_pids[pid]->fds) freez(all_pids[pid]->fds);
    if(all_pids[pid]->stat_filename) freez(all_pids[pid]->stat_filename);
    if(all_pids[pid]->statm_filename) freez(all_pids[pid]->statm_filename);
    if(all_pids[pid]->io_filename) freez(all_pids[pid]->io_filename);
    if(all_pids[pid]->cmdline_filename) freez(all_pids[pid]->cmdline_filename);
    freez(all_pids[pid]);

    all_pids[pid] = NULL;
    all_pids_count--;
}


// ----------------------------------------------------------------------------
// update pids from proc

int read_proc_pid_cmdline(struct pid_stat *p) {

    if(unlikely(!p->cmdline_filename)) {
        char filename[FILENAME_MAX + 1];
        snprintfz(filename, FILENAME_MAX, "%s/proc/%d/cmdline", global_host_prefix, p->pid);
        p->cmdline_filename = strdupz(filename);
    }

    int fd = open(p->cmdline_filename, O_RDONLY, 0666);
    if(unlikely(fd == -1)) goto cleanup;

    ssize_t i, bytes = read(fd, p->cmdline, MAX_CMDLINE);
    close(fd);

    if(unlikely(bytes < 0)) goto cleanup;

    p->cmdline[bytes] = '\0';
    for(i = 0; i < bytes ; i++)
        if(unlikely(!p->cmdline[i])) p->cmdline[i] = ' ';

    if(unlikely(debug))
        fprintf(stderr, "Read file '%s' contents: %s\n", p->cmdline_filename, p->cmdline);

    return 1;

cleanup:
    // copy the command to the command line
    strncpyz(p->cmdline, p->comm, MAX_CMDLINE);
    return 0;
}

int read_proc_pid_ownership(struct pid_stat *p) {
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
}

int read_proc_pid_stat(struct pid_stat *p) {
    static procfile *ff = NULL;

    if(unlikely(!p->stat_filename)) {
        char filename[FILENAME_MAX + 1];
        snprintfz(filename, FILENAME_MAX, "%s/proc/%d/stat", global_host_prefix, p->pid);
        p->stat_filename = strdupz(filename);
    }

    int set_quotes = (!ff)?1:0;

    ff = procfile_reopen(ff, p->stat_filename, NULL, PROCFILE_FLAG_NO_ERROR_ON_FILE_IO);
    if(unlikely(!ff)) goto cleanup;

    // if(set_quotes) procfile_set_quotes(ff, "()");
    if(set_quotes) procfile_set_open_close(ff, "(", ")");

    ff = procfile_readall(ff);
    if(unlikely(!ff)) goto cleanup;

    p->last_stat_collected_usec = p->stat_collected_usec;
    p->stat_collected_usec = now_realtime_usec();
    file_counter++;

    // p->pid           = atol(procfile_lineword(ff, 0, 0+i));

    strncpyz(p->comm, procfile_lineword(ff, 0, 1), MAX_COMPARE_NAME);

    // p->state         = *(procfile_lineword(ff, 0, 2));
    p->ppid             = (int32_t) atol(procfile_lineword(ff, 0, 3));
    // p->pgrp          = atol(procfile_lineword(ff, 0, 4));
    // p->session       = atol(procfile_lineword(ff, 0, 5));
    // p->tty_nr        = atol(procfile_lineword(ff, 0, 6));
    // p->tpgid         = atol(procfile_lineword(ff, 0, 7));
    // p->flags         = strtoull(procfile_lineword(ff, 0, 8), NULL, 10);

    unsigned long long last;

    last = p->minflt_raw;
    p->minflt_raw       = strtoull(procfile_lineword(ff, 0, 9), NULL, 10);
    p->minflt = (p->minflt_raw - last) * (USEC_PER_SEC * RATES_DETAIL) / (p->stat_collected_usec - p->last_stat_collected_usec);

    last = p->cminflt_raw;
    p->cminflt_raw      = strtoull(procfile_lineword(ff, 0, 10), NULL, 10);
    p->cminflt = (p->cminflt_raw - last) * (USEC_PER_SEC * RATES_DETAIL) / (p->stat_collected_usec - p->last_stat_collected_usec);

    last = p->majflt_raw;
    p->majflt_raw       = strtoull(procfile_lineword(ff, 0, 11), NULL, 10);
    p->majflt = (p->majflt_raw - last) * (USEC_PER_SEC * RATES_DETAIL) / (p->stat_collected_usec - p->last_stat_collected_usec);

    last = p->cmajflt_raw;
    p->cmajflt_raw      = strtoull(procfile_lineword(ff, 0, 12), NULL, 10);
    p->cmajflt = (p->cmajflt_raw - last) * (USEC_PER_SEC * RATES_DETAIL) / (p->stat_collected_usec - p->last_stat_collected_usec);

    last = p->utime_raw;
    p->utime_raw        = strtoull(procfile_lineword(ff, 0, 13), NULL, 10);
    p->utime = (p->utime_raw - last) * (USEC_PER_SEC * RATES_DETAIL) / (p->stat_collected_usec - p->last_stat_collected_usec);

    last = p->stime_raw;
    p->stime_raw        = strtoull(procfile_lineword(ff, 0, 14), NULL, 10);
    p->stime = (p->stime_raw - last) * (USEC_PER_SEC * RATES_DETAIL) / (p->stat_collected_usec - p->last_stat_collected_usec);

    last = p->cutime_raw;
    p->cutime_raw       = strtoull(procfile_lineword(ff, 0, 15), NULL, 10);
    p->cutime = (p->cutime_raw - last) * (USEC_PER_SEC * RATES_DETAIL) / (p->stat_collected_usec - p->last_stat_collected_usec);

    last = p->cstime_raw;
    p->cstime_raw       = strtoull(procfile_lineword(ff, 0, 16), NULL, 10);
    p->cstime = (p->cstime_raw - last) * (USEC_PER_SEC * RATES_DETAIL) / (p->stat_collected_usec - p->last_stat_collected_usec);

    // p->priority      = strtoull(procfile_lineword(ff, 0, 17), NULL, 10);
    // p->nice          = strtoull(procfile_lineword(ff, 0, 18), NULL, 10);
    p->num_threads      = (int32_t) atol(procfile_lineword(ff, 0, 19));
    // p->itrealvalue   = strtoull(procfile_lineword(ff, 0, 20), NULL, 10);
    // p->starttime     = strtoull(procfile_lineword(ff, 0, 21), NULL, 10);
    // p->vsize         = strtoull(procfile_lineword(ff, 0, 22), NULL, 10);
    // p->rss           = strtoull(procfile_lineword(ff, 0, 23), NULL, 10);
    // p->rsslim        = strtoull(procfile_lineword(ff, 0, 24), NULL, 10);
    // p->starcode      = strtoull(procfile_lineword(ff, 0, 25), NULL, 10);
    // p->endcode       = strtoull(procfile_lineword(ff, 0, 26), NULL, 10);
    // p->startstack    = strtoull(procfile_lineword(ff, 0, 27), NULL, 10);
    // p->kstkesp       = strtoull(procfile_lineword(ff, 0, 28), NULL, 10);
    // p->kstkeip       = strtoull(procfile_lineword(ff, 0, 29), NULL, 10);
    // p->signal        = strtoull(procfile_lineword(ff, 0, 30), NULL, 10);
    // p->blocked       = strtoull(procfile_lineword(ff, 0, 31), NULL, 10);
    // p->sigignore     = strtoull(procfile_lineword(ff, 0, 32), NULL, 10);
    // p->sigcatch      = strtoull(procfile_lineword(ff, 0, 33), NULL, 10);
    // p->wchan         = strtoull(procfile_lineword(ff, 0, 34), NULL, 10);
    // p->nswap         = strtoull(procfile_lineword(ff, 0, 35), NULL, 10);
    // p->cnswap        = strtoull(procfile_lineword(ff, 0, 36), NULL, 10);
    // p->exit_signal   = atol(procfile_lineword(ff, 0, 37));
    // p->processor     = atol(procfile_lineword(ff, 0, 38));
    // p->rt_priority   = strtoul(procfile_lineword(ff, 0, 39), NULL, 10);
    // p->policy        = strtoul(procfile_lineword(ff, 0, 40), NULL, 10);
    // p->delayacct_blkio_ticks = strtoull(procfile_lineword(ff, 0, 41), NULL, 10);

    if(enable_guest_charts) {
        last = p->gtime_raw;
        p->gtime_raw        = strtoull(procfile_lineword(ff, 0, 42), NULL, 10);
        p->gtime = (p->gtime_raw - last) * (USEC_PER_SEC * RATES_DETAIL) / (p->stat_collected_usec - p->last_stat_collected_usec);

        last = p->cgtime_raw;
        p->cgtime_raw       = strtoull(procfile_lineword(ff, 0, 43), NULL, 10);
        p->cgtime = (p->cgtime_raw - last) * (USEC_PER_SEC * RATES_DETAIL) / (p->stat_collected_usec - p->last_stat_collected_usec);

        if (show_guest_time || p->gtime || p->cgtime) {
            p->utime -= (p->utime >= p->gtime) ? p->gtime : p->utime;
            p->cutime -= (p->cutime >= p->cgtime) ? p->cgtime : p->cutime;
            show_guest_time = 1;
        }
    }

    if(unlikely(debug || (p->target && p->target->debug)))
        fprintf(stderr, "apps.plugin: READ PROC/PID/STAT: %s/proc/%d/stat, process: '%s' on target '%s' (dt=%llu) VALUES: utime=%llu, stime=%llu, cutime=%llu, cstime=%llu, minflt=%llu, majflt=%llu, cminflt=%llu, cmajflt=%llu, threads=%d\n", global_host_prefix, p->pid, p->comm, (p->target)?p->target->name:"UNSET", p->stat_collected_usec - p->last_stat_collected_usec, p->utime, p->stime, p->cutime, p->cstime, p->minflt, p->majflt, p->cminflt, p->cmajflt, p->num_threads);

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

int read_proc_pid_statm(struct pid_stat *p) {
    static procfile *ff = NULL;

    if(unlikely(!p->statm_filename)) {
        char filename[FILENAME_MAX + 1];
        snprintfz(filename, FILENAME_MAX, "%s/proc/%d/statm", global_host_prefix, p->pid);
        p->statm_filename = strdupz(filename);
    }

    ff = procfile_reopen(ff, p->statm_filename, NULL, PROCFILE_FLAG_NO_ERROR_ON_FILE_IO);
    if(unlikely(!ff)) goto cleanup;

    ff = procfile_readall(ff);
    if(unlikely(!ff)) goto cleanup;

    file_counter++;

    p->statm_size           = strtoull(procfile_lineword(ff, 0, 0), NULL, 10);
    p->statm_resident       = strtoull(procfile_lineword(ff, 0, 1), NULL, 10);
    p->statm_share          = strtoull(procfile_lineword(ff, 0, 2), NULL, 10);
    // p->statm_text           = strtoull(procfile_lineword(ff, 0, 3), NULL, 10);
    // p->statm_lib            = strtoull(procfile_lineword(ff, 0, 4), NULL, 10);
    // p->statm_data           = strtoull(procfile_lineword(ff, 0, 5), NULL, 10);
    // p->statm_dirty          = strtoull(procfile_lineword(ff, 0, 6), NULL, 10);

    return 1;

cleanup:
    p->statm_size           = 0;
    p->statm_resident       = 0;
    p->statm_share          = 0;
    // p->statm_text           = 0;
    // p->statm_lib            = 0;
    // p->statm_data           = 0;
    // p->statm_dirty          = 0;
    return 0;
}

int read_proc_pid_io(struct pid_stat *p) {
    static procfile *ff = NULL;

    if(unlikely(!p->io_filename)) {
        char filename[FILENAME_MAX + 1];
        snprintfz(filename, FILENAME_MAX, "%s/proc/%d/io", global_host_prefix, p->pid);
        p->io_filename = strdupz(filename);
    }

    // open the file
    ff = procfile_reopen(ff, p->io_filename, NULL, PROCFILE_FLAG_NO_ERROR_ON_FILE_IO);
    if(unlikely(!ff)) goto cleanup;

    ff = procfile_readall(ff);
    if(unlikely(!ff)) goto cleanup;

    file_counter++;

    p->last_io_collected_usec = p->io_collected_usec;
    p->io_collected_usec = now_realtime_usec();

    unsigned long long last;

    last = p->io_logical_bytes_read_raw;
    p->io_logical_bytes_read_raw = strtoull(procfile_lineword(ff, 0, 1), NULL, 10);
    p->io_logical_bytes_read = (p->io_logical_bytes_read_raw - last) * (USEC_PER_SEC * RATES_DETAIL) / (p->io_collected_usec - p->last_io_collected_usec);

    last = p->io_logical_bytes_written_raw;
    p->io_logical_bytes_written_raw = strtoull(procfile_lineword(ff, 1, 1), NULL, 10);
    p->io_logical_bytes_written = (p->io_logical_bytes_written_raw - last) * (USEC_PER_SEC * RATES_DETAIL) / (p->io_collected_usec - p->last_io_collected_usec);

    // last = p->io_read_calls_raw;
    // p->io_read_calls_raw = strtoull(procfile_lineword(ff, 2, 1), NULL, 10);
    // p->io_read_calls = (p->io_read_calls_raw - last) * (USEC_PER_SEC * RATES_DETAIL) / (p->io_collected_usec - p->last_io_collected_usec);

    // last = p->io_write_calls_raw;
    // p->io_write_calls_raw = strtoull(procfile_lineword(ff, 3, 1), NULL, 10);
    // p->io_write_calls = (p->io_write_calls_raw - last) * (USEC_PER_SEC * RATES_DETAIL) / (p->io_collected_usec - p->last_io_collected_usec);

    last = p->io_storage_bytes_read_raw;
    p->io_storage_bytes_read_raw = strtoull(procfile_lineword(ff, 4, 1), NULL, 10);
    p->io_storage_bytes_read = (p->io_storage_bytes_read_raw - last) * (USEC_PER_SEC * RATES_DETAIL) / (p->io_collected_usec - p->last_io_collected_usec);

    last = p->io_storage_bytes_written_raw;
    p->io_storage_bytes_written_raw = strtoull(procfile_lineword(ff, 5, 1), NULL, 10);
    p->io_storage_bytes_written = (p->io_storage_bytes_written_raw - last) * (USEC_PER_SEC * RATES_DETAIL) / (p->io_collected_usec - p->last_io_collected_usec);

    // last = p->io_cancelled_write_bytes_raw;
    // p->io_cancelled_write_bytes_raw = strtoull(procfile_lineword(ff, 6, 1), NULL, 10);
    // p->io_cancelled_write_bytes = (p->io_cancelled_write_bytes_raw - last) * (USEC_PER_SEC * RATES_DETAIL) / (p->io_collected_usec - p->last_io_collected_usec);

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

cleanup:
    p->io_logical_bytes_read        = 0;
    p->io_logical_bytes_written     = 0;
    // p->io_read_calls             = 0;
    // p->io_write_calls            = 0;
    p->io_storage_bytes_read        = 0;
    p->io_storage_bytes_written     = 0;
    // p->io_cancelled_write_bytes  = 0;
    return 0;
}

unsigned long long global_utime = 0;
unsigned long long global_stime = 0;
unsigned long long global_gtime = 0;

int read_proc_stat() {
    static char filename[FILENAME_MAX + 1] = "";
    static procfile *ff = NULL;
    static unsigned long long utime_raw = 0, stime_raw = 0, gtime_raw = 0, gntime_raw = 0, ntime_raw = 0;
    static usec_t collected_usec = 0, last_collected_usec = 0;

    if(unlikely(!ff)) {
        snprintfz(filename, FILENAME_MAX, "%s/proc/stat", global_host_prefix);
        ff = procfile_open(filename, " \t:", PROCFILE_FLAG_DEFAULT);
        if(unlikely(!ff)) goto cleanup;
    }

    ff = procfile_readall(ff);
    if(unlikely(!ff)) goto cleanup;

    last_collected_usec = collected_usec;
    collected_usec = now_realtime_usec();

    file_counter++;

    unsigned long long last;

    last = utime_raw;
    utime_raw = strtoull(procfile_lineword(ff, 0, 1), NULL, 10);
    global_utime = (utime_raw - last) * (USEC_PER_SEC * RATES_DETAIL) / (collected_usec - last_collected_usec);

    // nice time, on user time
    last = ntime_raw;
    ntime_raw = strtoull(procfile_lineword(ff, 0, 2), NULL, 10);
    global_utime += (ntime_raw - last) * (USEC_PER_SEC * RATES_DETAIL) / (collected_usec - last_collected_usec);

    last = stime_raw;
    stime_raw = strtoull(procfile_lineword(ff, 0, 3), NULL, 10);
    global_stime = (stime_raw - last) * (USEC_PER_SEC * RATES_DETAIL) / (collected_usec - last_collected_usec);

    last = gtime_raw;
    gtime_raw = strtoull(procfile_lineword(ff, 0, 10), NULL, 10);
    global_gtime = (gtime_raw - last) * (USEC_PER_SEC * RATES_DETAIL) / (collected_usec - last_collected_usec);

    if(enable_guest_charts) {
        // guest nice time, on guest time
        last = gntime_raw;
        gntime_raw = strtoull(procfile_lineword(ff, 0, 11), NULL, 10);
        global_gtime += (gntime_raw - last) * (USEC_PER_SEC * RATES_DETAIL) / (collected_usec - last_collected_usec);

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


// ----------------------------------------------------------------------------
// file descriptor
// this is used to keep a global list of all open files of the system
// it is needed in order to calculate the unique files processes have open

#define FILE_DESCRIPTORS_INCREASE_STEP 100

struct file_descriptor {
    avl avl;
#ifdef NETDATA_INTERNAL_CHECKS
    uint32_t magic;
#endif /* NETDATA_INTERNAL_CHECKS */
    uint32_t hash;
    const char *name;
    int type;
    int count;
    int pos;
} *all_files = NULL;

int all_files_len = 0;
int all_files_size = 0;

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

#define FILETYPE_OTHER 0
#define FILETYPE_FILE 1
#define FILETYPE_PIPE 2
#define FILETYPE_SOCKET 3
#define FILETYPE_INOTIFY 4
#define FILETYPE_EVENTFD 5
#define FILETYPE_EVENTPOLL 6
#define FILETYPE_TIMERFD 7
#define FILETYPE_SIGNALFD 8

void file_descriptor_not_used(int id)
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

                file_descriptor_remove(&all_files[id]);
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

int file_descriptor_find_or_add(const char *name)
{
    static int last_pos = 0;
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

    // check we have enough memory to add it
    if(!all_files || all_files_len == all_files_size) {
        void *old = all_files;
        int i;

        // there is no empty slot
        if(unlikely(debug))
            fprintf(stderr, "apps.plugin: extending fd array to %d entries\n", all_files_size + FILE_DESCRIPTORS_INCREASE_STEP);

        all_files = reallocz(all_files, (all_files_size + FILE_DESCRIPTORS_INCREASE_STEP) * sizeof(struct file_descriptor));

        // if the address changed, we have to rebuild the index
        // since all pointers are now invalid
        if(old && old != (void *)all_files) {
            if(unlikely(debug))
                fprintf(stderr, "apps.plugin:   >> re-indexing.\n");

            all_files_index.root = NULL;
            for(i = 0; i < all_files_size; i++) {
                if(!all_files[i].count) continue;
                file_descriptor_add(&all_files[i]);
            }

            if(unlikely(debug))
                fprintf(stderr, "apps.plugin:   >> re-indexing done.\n");
        }

        for(i = all_files_size; i < (all_files_size + FILE_DESCRIPTORS_INCREASE_STEP); i++) {
            all_files[i].count = 0;
            all_files[i].name = NULL;
#ifdef NETDATA_INTERNAL_CHECKS
            all_files[i].magic = 0x00000000;
#endif /* NETDATA_INTERNAL_CHECKS */
            all_files[i].pos = i;
        }

        if(!all_files_size) all_files_len = 1;
        all_files_size += FILE_DESCRIPTORS_INCREASE_STEP;
    }

    if(unlikely(debug))
        fprintf(stderr, "apps.plugin:   >> searching for empty slot.\n");

    // search for an empty slot
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

            if(all_files[c].name) freez((void *)all_files[c].name);
            all_files[c].name = NULL;
            last_pos = c;
            break;
        }
    }
    if(i == all_files_size) {
        fatal("We should find an empty slot, but there isn't any");
        exit(1);
    }

    if(unlikely(debug))
        fprintf(stderr, "apps.plugin:   >> updating slot %d.\n", c);

    all_files_len++;

    // else we have an empty slot in 'c'

    int type;
    if(name[0] == '/') type = FILETYPE_FILE;
    else if(strncmp(name, "pipe:", 5) == 0) type = FILETYPE_PIPE;
    else if(strncmp(name, "socket:", 7) == 0) type = FILETYPE_SOCKET;
    else if(strcmp(name, "anon_inode:inotify") == 0 || strcmp(name, "inotify") == 0) type = FILETYPE_INOTIFY;
    else if(strcmp(name, "anon_inode:[eventfd]") == 0) type = FILETYPE_EVENTFD;
    else if(strcmp(name, "anon_inode:[eventpoll]") == 0) type = FILETYPE_EVENTPOLL;
    else if(strcmp(name, "anon_inode:[timerfd]") == 0) type = FILETYPE_TIMERFD;
    else if(strcmp(name, "anon_inode:[signalfd]") == 0) type = FILETYPE_SIGNALFD;
    else if(strncmp(name, "anon_inode:", 11) == 0) {
        if(unlikely(debug))
            fprintf(stderr, "apps.plugin: FIXME: unknown anonymous inode: %s\n", name);

        type = FILETYPE_OTHER;
    }
    else {
        if(unlikely(debug))
            fprintf(stderr, "apps.plugin: FIXME: cannot understand linkname: %s\n", name);

        type = FILETYPE_OTHER;
    }

    all_files[c].name = strdupz(name);
    all_files[c].hash = hash;
    all_files[c].type = type;
    all_files[c].pos  = c;
    all_files[c].count = 1;
#ifdef NETDATA_INTERNAL_CHECKS
    all_files[c].magic = 0x0BADCAFE;
#endif /* NETDATA_INTERNAL_CHECKS */
    file_descriptor_add(&all_files[c]);

    if(unlikely(debug))
        fprintf(stderr, "apps.plugin: using fd position %d (name: %s)\n", c, all_files[c].name);

    return c;
}

int read_pid_file_descriptors(struct pid_stat *p) {
    char dirname[FILENAME_MAX+1];

    snprintfz(dirname, FILENAME_MAX, "%s/proc/%d/fd", global_host_prefix, p->pid);
    DIR *fds = opendir(dirname);
    if(fds) {
        int c;
        struct dirent *de;
        char fdname[FILENAME_MAX + 1];
        char linkname[FILENAME_MAX + 1];

        // make the array negative
        for(c = 0 ; c < p->fds_size ; c++)
            p->fds[c] = -p->fds[c];

        while((de = readdir(fds))) {
            if(strcmp(de->d_name, ".") == 0 || strcmp(de->d_name, "..") == 0)
                continue;

            // check if the fds array is small
            int fdid = atoi(de->d_name);
            if(fdid < 0) continue;
            if(fdid >= p->fds_size) {
                // it is small, extend it
                if(unlikely(debug))
                    fprintf(stderr, "apps.plugin: extending fd memory slots for %s from %d to %d\n", p->comm, p->fds_size, fdid + 100);

                p->fds = reallocz(p->fds, (fdid + 100) * sizeof(int));
                if(!p->fds) {
                    fatal("Cannot re-allocate fds for %s", p->comm);
                    break;
                }

                // and initialize it
                for(c = p->fds_size ; c < (fdid + 100) ; c++) p->fds[c] = 0;
                p->fds_size = fdid + 100;
            }

            if(p->fds[fdid] == 0) {
                // we don't know this fd, get it

                sprintf(fdname, "%s/proc/%d/fd/%s", global_host_prefix, p->pid, de->d_name);
                ssize_t l = readlink(fdname, linkname, FILENAME_MAX);
                if(l == -1) {
                    if(debug || (p->target && p->target->debug)) {
                        if(debug || (p->target && p->target->debug))
                            error("Cannot read link %s", fdname);
                    }
                    continue;
                }
                linkname[l] = '\0';
                file_counter++;

                // if another process already has this, we will get
                // the same id
                p->fds[fdid] = file_descriptor_find_or_add(linkname);
            }

            // else make it positive again, we need it
            // of course, the actual file may have changed, but we don't care so much
            // FIXME: we could compare the inode as returned by readdir dirent structure
            else p->fds[fdid] = -p->fds[fdid];
        }
        closedir(fds);

        // remove all the negative file descriptors
        for(c = 0 ; c < p->fds_size ; c++) if(p->fds[c] < 0) {
            file_descriptor_not_used(-p->fds[c]);
            p->fds[c] = 0;
        }
    }
    else return 0;

    return 1;
}

// ----------------------------------------------------------------------------

int print_process_and_parents(struct pid_stat *p, unsigned long long time) {
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

    fprintf(stderr, "  %s %s%s (%d %s %lld"
        , buffer
        , prefix
        , p->comm
        , p->pid
        , p->updated?"running":"exited"
        , (long long)p->stat_collected_usec - (long long)time
        );

    if(p->utime)   fprintf(stderr, " utime=%llu",   p->utime);
    if(p->stime)   fprintf(stderr, " stime=%llu",   p->stime);
    if(p->gtime)   fprintf(stderr, " gtime=%llu",   p->gtime);
    if(p->cutime)  fprintf(stderr, " cutime=%llu",  p->cutime);
    if(p->cstime)  fprintf(stderr, " cstime=%llu",  p->cstime);
    if(p->cgtime)  fprintf(stderr, " cgtime=%llu",  p->cgtime);
    if(p->minflt)  fprintf(stderr, " minflt=%llu",  p->minflt);
    if(p->cminflt) fprintf(stderr, " cminflt=%llu", p->cminflt);
    if(p->majflt)  fprintf(stderr, " majflt=%llu",  p->majflt);
    if(p->cmajflt) fprintf(stderr, " cmajflt=%llu", p->cmajflt);
    fprintf(stderr, ")\n");

    return indent + 1;
}

void print_process_tree(struct pid_stat *p, char *msg) {
    log_date(stderr);
    fprintf(stderr, "%s: process %s (%d, %s) with parents:\n", msg, p->comm, p->pid, p->updated?"running":"exited");
    print_process_and_parents(p, p->stat_collected_usec);
}

void find_lost_child_debug(struct pid_stat *pe, unsigned long long lost, int type) {
    int found = 0;
    struct pid_stat *p = NULL;

    for(p = root_of_pids; p ; p = p->next) {
        if(p == pe) continue;

        switch(type) {
            case 1:
                if(p->cminflt > lost) {
                    fprintf(stderr, " > process %d (%s) could use the lost exited child minflt %llu of process %d (%s)\n", p->pid, p->comm, lost, pe->pid, pe->comm);
                    found++;
                }
                break;

            case 2:
                if(p->cmajflt > lost) {
                    fprintf(stderr, " > process %d (%s) could use the lost exited child majflt %llu of process %d (%s)\n", p->pid, p->comm, lost, pe->pid, pe->comm);
                    found++;
                }
                break;

            case 3:
                if(p->cutime > lost) {
                    fprintf(stderr, " > process %d (%s) could use the lost exited child utime %llu of process %d (%s)\n", p->pid, p->comm, lost, pe->pid, pe->comm);
                    found++;
                }
                break;

            case 4:
                if(p->cstime > lost) {
                    fprintf(stderr, " > process %d (%s) could use the lost exited child stime %llu of process %d (%s)\n", p->pid, p->comm, lost, pe->pid, pe->comm);
                    found++;
                }
                break;

            case 5:
                if(p->cgtime > lost) {
                    fprintf(stderr, " > process %d (%s) could use the lost exited child gtime %llu of process %d (%s)\n", p->pid, p->comm, lost, pe->pid, pe->comm);
                    found++;
                }
                break;
        }
    }

    if(!found) {
        switch(type) {
            case 1:
                fprintf(stderr, " > cannot find any process to use the lost exited child minflt %llu of process %d (%s)\n", lost, pe->pid, pe->comm);
                break;

            case 2:
                fprintf(stderr, " > cannot find any process to use the lost exited child majflt %llu of process %d (%s)\n", lost, pe->pid, pe->comm);
                break;

            case 3:
                fprintf(stderr, " > cannot find any process to use the lost exited child utime %llu of process %d (%s)\n", lost, pe->pid, pe->comm);
                break;

            case 4:
                fprintf(stderr, " > cannot find any process to use the lost exited child stime %llu of process %d (%s)\n", lost, pe->pid, pe->comm);
                break;

            case 5:
                fprintf(stderr, " > cannot find any process to use the lost exited child gtime %llu of process %d (%s)\n", lost, pe->pid, pe->comm);
                break;
        }
    }
}

unsigned long long remove_exited_child_from_parent(unsigned long long *field, unsigned long long *pfield) {
    unsigned long long absorbed = 0;

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

void process_exited_processes() {
    struct pid_stat *p;

    for(p = root_of_pids; p ; p = p->next) {
        if(p->updated || !p->stat_collected_usec)
            continue;

        struct pid_stat *pp = p->parent;

        unsigned long long utime  = (p->utime_raw + p->cutime_raw)   * (1000000ULL * RATES_DETAIL) / (p->stat_collected_usec - p->last_stat_collected_usec);
        unsigned long long stime  = (p->stime_raw + p->cstime_raw)   * (1000000ULL * RATES_DETAIL) / (p->stat_collected_usec - p->last_stat_collected_usec);
        unsigned long long gtime  = (p->gtime_raw + p->cgtime_raw)   * (1000000ULL * RATES_DETAIL) / (p->stat_collected_usec - p->last_stat_collected_usec);
        unsigned long long minflt = (p->minflt_raw + p->cminflt_raw) * (1000000ULL * RATES_DETAIL) / (p->stat_collected_usec - p->last_stat_collected_usec);
        unsigned long long majflt = (p->majflt_raw + p->cmajflt_raw) * (1000000ULL * RATES_DETAIL) / (p->stat_collected_usec - p->last_stat_collected_usec);

        if(utime + stime + gtime + minflt + majflt == 0)
            continue;

        if(unlikely(debug)) {
            log_date(stderr);
            fprintf(stderr, "Absorb %s (%d %s total resources: utime=%llu stime=%llu gtime=%llu minflt=%llu majflt=%llu)\n"
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

        for(pp = p->parent; pp ; pp = pp->parent) {
            if(!pp->updated) continue;

            unsigned long long absorbed;
            absorbed = remove_exited_child_from_parent(&utime,  &pp->cutime);
            if(unlikely(debug && absorbed))
                fprintf(stderr, " > process %s (%d %s) absorbed %llu utime (remaining: %llu)\n", pp->comm, pp->pid, pp->updated?"running":"exited", absorbed, utime);

            absorbed = remove_exited_child_from_parent(&stime,  &pp->cstime);
            if(unlikely(debug && absorbed))
                fprintf(stderr, " > process %s (%d %s) absorbed %llu stime (remaining: %llu)\n", pp->comm, pp->pid, pp->updated?"running":"exited", absorbed, stime);

            absorbed = remove_exited_child_from_parent(&gtime,  &pp->cgtime);
            if(unlikely(debug && absorbed))
                fprintf(stderr, " > process %s (%d %s) absorbed %llu gtime (remaining: %llu)\n", pp->comm, pp->pid, pp->updated?"running":"exited", absorbed, gtime);

            absorbed = remove_exited_child_from_parent(&minflt, &pp->cminflt);
            if(unlikely(debug && absorbed))
                fprintf(stderr, " > process %s (%d %s) absorbed %llu minflt (remaining: %llu)\n", pp->comm, pp->pid, pp->updated?"running":"exited", absorbed, minflt);

            absorbed = remove_exited_child_from_parent(&majflt, &pp->cmajflt);
            if(unlikely(debug && absorbed))
                fprintf(stderr, " > process %s (%d %s) absorbed %llu majflt (remaining: %llu)\n", pp->comm, pp->pid, pp->updated?"running":"exited", absorbed, majflt);
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
                fprintf(stderr, " > remaining resources - KEEP - for another loop: %s (%d %s total resources: utime=%llu stime=%llu gtime=%llu minflt=%llu majflt=%llu)\n"
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

void link_all_processes_to_their_parents(void) {
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
                fprintf(stderr, "apps.plugin: \tchild %d (%s, %s) on target '%s' has parent %d (%s, %s). Parent: utime=%llu, stime=%llu, gtime=%llu, minflt=%llu, majflt=%llu, cutime=%llu, cstime=%llu, cgtime=%llu, cminflt=%llu, cmajflt=%llu\n", p->pid, p->comm, p->updated?"running":"exited", (p->target)?p->target->name:"UNSET", pp->pid, pp->comm, pp->updated?"running":"exited", pp->utime, pp->stime, pp->gtime, pp->minflt, pp->majflt, pp->cutime, pp->cstime, pp->cgtime, pp->cminflt, pp->cmajflt);
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

static int compar_pid(const void *pid1, const void *pid2) {

    struct pid_stat *p1 = all_pids[*((pid_t *)pid1)];
    struct pid_stat *p2 = all_pids[*((pid_t *)pid2)];

    if(p1->sortlist > p2->sortlist)
        return -1;
    else
        return 1;
}

static inline int managed_log(struct pid_stat *p, uint32_t log, int status) {
    if(unlikely(!status)) {
        // error("command failed log %u, errno %d", log, errno);

        if(unlikely(debug || errno != ENOENT)) {
            if(unlikely(debug || !(p->log_thrown & log))) {
                p->log_thrown |= log;
                switch(log) {
                    case PID_LOG_IO:
                        error("Cannot process %s/proc/%d/io (command '%s')", global_host_prefix, p->pid, p->comm);
                        break;

                    case PID_LOG_STATM:
                        error("Cannot process %s/proc/%d/statm (command '%s')", global_host_prefix, p->pid, p->comm);
                        break;

                    case PID_LOG_CMDLINE:
                        error("Cannot process %s/proc/%d/cmdline (command '%s')", global_host_prefix, p->pid, p->comm);
                        break;

                    case PID_LOG_FDS:
                        error("Cannot process entries in %s/proc/%d/fd (command '%s')", global_host_prefix, p->pid, p->comm);
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

void collect_data_for_pid(pid_t pid) {
    if(unlikely(pid <= 0 || pid > pid_max)) {
        error("Invalid pid %d read (expected 1 to %d). Ignoring process.", pid, pid_max);
        return;
    }

    struct pid_stat *p = get_pid_entry(pid);
    if(unlikely(!p || p->read)) return;
    p->read             = 1;

    // fprintf(stderr, "Reading process %d (%s), sortlist %d\n", p->pid, p->comm, p->sortlist);

    // --------------------------------------------------------------------
    // /proc/<pid>/stat

    if(unlikely(!managed_log(p, PID_LOG_STAT, read_proc_pid_stat(p))))
        // there is no reason to proceed if we cannot get its status
        return;

    read_proc_pid_ownership(p);

    // check its parent pid
    if(unlikely(p->ppid < 0 || p->ppid > pid_max)) {
        error("Pid %d (command '%s') states invalid parent pid %d. Using 0.", pid, p->comm, p->ppid);
        p->ppid = 0;
    }

    // --------------------------------------------------------------------
    // /proc/<pid>/io

    managed_log(p, PID_LOG_IO, read_proc_pid_io(p));

    // --------------------------------------------------------------------
    // /proc/<pid>/statm

    if(unlikely(!managed_log(p, PID_LOG_STATM, read_proc_pid_statm(p))))
        // there is no reason to proceed if we cannot get its memory status
        return;

    // --------------------------------------------------------------------
    // link it

    // check if it is target
    // we do this only once, the first time this pid is loaded
    if(unlikely(p->new_entry)) {
        // /proc/<pid>/cmdline
        if(likely(proc_pid_cmdline_is_needed))
            managed_log(p, PID_LOG_CMDLINE, read_proc_pid_cmdline(p));

        if(unlikely(debug))
            fprintf(stderr, "apps.plugin: \tJust added %d (%s)\n", pid, p->comm);

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
            if( (!w->starts_with && !w->ends_with && w->comparehash == hash && !strcmp(w->compare, p->comm))
                   || (w->starts_with && !w->ends_with && !strncmp(w->compare, p->comm, w->comparelen))
                   || (!w->starts_with && w->ends_with && pclen >= w->comparelen && !strcmp(w->compare, &p->comm[pclen - w->comparelen]))
                   || (proc_pid_cmdline_is_needed && w->starts_with && w->ends_with && strstr(p->cmdline, w->compare))
                    ) {
                if(w->target) p->target = w->target;
                else p->target = w;

                if(debug || (p->target && p->target->debug))
                    fprintf(stderr, "apps.plugin: \t\t%s linked to target %s\n", p->comm, p->target->name);

                break;
            }
        }
    }

    // --------------------------------------------------------------------
    // /proc/<pid>/fd

    if(enable_file_charts)
            managed_log(p, PID_LOG_FDS, read_pid_file_descriptors(p));

    // --------------------------------------------------------------------
    // done!

    if(unlikely(debug && include_exited_childs && all_pids_count && p->ppid && all_pids[p->ppid] && !all_pids[p->ppid]->read))
        fprintf(stderr, "Read process %d (%s) sortlisted %d, but its parent %d (%s) sortlisted %d, is not read\n", p->pid, p->comm, p->sortlist, all_pids[p->ppid]->pid, all_pids[p->ppid]->comm, all_pids[p->ppid]->sortlist);

    // mark it as updated
    p->updated = 1;
    p->keep = 0;
    p->keeploops = 0;
}

int collect_data_for_all_processes_from_proc(void) {
    struct pid_stat *p = NULL;

    if(all_pids_count) {
        // read parents before childs
        // this is needed to prevent a situation where
        // a child is found running, but until we read
        // its parent, it has exited and its parent
        // has accumulated its resources

        long slc = 0;
        for(p = root_of_pids; p ; p = p->next) {
            p->read             = 0;
            p->updated          = 0;
            p->new_entry        = 0;
            p->merged           = 0;
            p->children_count   = 0;
            p->parent           = NULL;

            all_pids_sortlist[slc++] = p->pid;
        }

        if(unlikely(slc != all_pids_count)) {
            error("Internal error: I was thinking I had %ld processes in my arrays, but it seems there are more.", all_pids_count);
            all_pids_count = slc;
        }

        if(include_exited_childs) {
            qsort((void *)all_pids_sortlist, all_pids_count, sizeof(pid_t), compar_pid);
            for(slc = 0; slc < all_pids_count; slc++)
                collect_data_for_pid(all_pids_sortlist[slc]);
        }
    }

    char dirname[FILENAME_MAX + 1];

    snprintfz(dirname, FILENAME_MAX, "%s/proc", global_host_prefix);
    DIR *dir = opendir(dirname);
    if(!dir) return 0;

    struct dirent *file = NULL;

    while((file = readdir(dir))) {
        char *endptr = file->d_name;
        pid_t pid = (pid_t) strtoul(file->d_name, &endptr, 10);

        // make sure we read a valid number
        if(unlikely(endptr == file->d_name || *endptr != '\0'))
            continue;

        collect_data_for_pid(pid);
    }
    closedir(dir);

    // normally this is done
    // however we may have processes exited while we collected values
    // so let's find the exited ones
    // we do this by collecting the ownership of process
    // if we manage to get the ownership, the process still runs

    read_proc_stat();
    link_all_processes_to_their_parents();
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

void cleanup_exited_pids(void) {
    int c;
    struct pid_stat *p = NULL;

    for(p = root_of_pids; p ;) {
        if(!p->updated && (!p->keep || p->keeploops > 0)) {
//          fprintf(stderr, "\tEXITED %d %s [parent %d %s, target %s] utime=%llu, stime=%llu, gtime=%llu, cutime=%llu, cstime=%llu, cgtime=%llu, minflt=%llu, majflt=%llu, cminflt=%llu, cmajflt=%llu\n", p->pid, p->comm, p->parent->pid, p->parent->comm, p->target->name,  p->utime, p->stime, p->gtime, p->cutime, p->cstime, p->cgtime, p->minflt, p->majflt, p->cminflt, p->cmajflt);

            if(unlikely(debug && (p->keep || p->keeploops)))
                fprintf(stderr, " > CLEANUP cannot keep exited process %d (%s) anymore - removing it.\n", p->pid, p->comm);

            for(c = 0 ; c < p->fds_size ; c++) if(p->fds[c] > 0) {
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

void apply_apps_groups_targets_inheritance(void) {
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

            // if this process does not have any children
            // and is not already merged
            // and has a parent
            // and its parent has children
            // and the target of this process and its parent is the same, or the parent does not have a target
            // and its parent is not init
            // then, mark them as merged.
            if(unlikely(
                    !p->children_count
                    && !p->merged
                    && p->parent
                    && p->parent->children_count
                    && (p->target == p->parent->target || !p->parent->target)
                    && p->ppid != 1
                )) {
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
    if(all_pids[1])
        all_pids[1]->target = apps_groups_default_target;

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

long zero_all_targets(struct target *root) {
    struct target *w;
    long count = 0;

    for (w = root; w ; w = w->next) {
        count++;

        if(w->fds) freez(w->fds);
        w->fds = NULL;

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
    }

    return count;
}

void aggregate_pid_on_target(struct target *w, struct pid_stat *p, struct target *o) {
    (void)o;

    if(unlikely(!w->fds))
        w->fds = callocz(sizeof(int), (size_t) all_files_size);

    if(likely(p->updated)) {
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

        if(likely(w->fds)) {
            int c;
            for(c = 0; c < p->fds_size ;c++) {
                if(p->fds[c] == 0) continue;

                if(likely(p->fds[c] < all_files_size)) {
                    if(w->fds) w->fds[p->fds[c]]++;
                }
                else
                    error("Invalid fd number %d", p->fds[c]);
            }
        }

        if(unlikely(debug || w->debug))
            fprintf(stderr, "apps.plugin: \taggregating '%s' pid %d on target '%s' utime=%llu, stime=%llu, gtime=%llu, cutime=%llu, cstime=%llu, cgtime=%llu, minflt=%llu, majflt=%llu, cminflt=%llu, cmajflt=%llu\n", p->comm, p->pid, w->name, p->utime, p->stime, p->gtime, p->cutime, p->cstime, p->cgtime, p->minflt, p->majflt, p->cminflt, p->cmajflt);
    }
}

void count_targets_fds(struct target *root) {
    int c;
    struct target *w;

    for (w = root; w ; w = w->next) {
        if(!w->fds) continue;

        w->openfiles = 0;
        w->openpipes = 0;
        w->opensockets = 0;
        w->openinotifies = 0;
        w->openeventfds = 0;
        w->opentimerfds = 0;
        w->opensignalfds = 0;
        w->openeventpolls = 0;
        w->openother = 0;

        for(c = 1; c < all_files_size ;c++) {
            if(w->fds[c] > 0)
                switch(all_files[c].type) {
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

                default:
                    w->openother++;
            }
        }

        freez(w->fds);
        w->fds = NULL;
    }
}

void calculate_netdata_statistics(void) {
    apply_apps_groups_targets_inheritance();

    zero_all_targets(users_root_target);
    zero_all_targets(groups_root_target);
    apps_groups_targets = zero_all_targets(apps_groups_root_target);

    // this has to be done, before the cleanup
    struct pid_stat *p = NULL;
    struct target *w = NULL, *o = NULL;

    // concentrate everything on the apps_groups_targets
    for(p = root_of_pids; p ; p = p->next) {

        // --------------------------------------------------------------------
        // apps_groups targets
        if(likely(p->target))
            aggregate_pid_on_target(p->target, p, NULL);
        else
            error("pid %d %s was left without a target!", p->pid, p->comm);


        // --------------------------------------------------------------------
        // user targets
        o = p->user_target;
        if(likely(p->user_target && p->user_target->uid == p->uid))
            w = p->user_target;
        else {
            if(unlikely(debug && p->user_target))
                    fprintf(stderr, "apps.plugin: \t\tpid %d (%s) switched user from %u (%s) to %u.\n", p->pid, p->comm, p->user_target->uid, p->user_target->name, p->uid);

            w = p->user_target = get_users_target(p->uid);
        }

        if(likely(w))
            aggregate_pid_on_target(w, p, o);
        else
            error("pid %d %s was left without a user target!", p->pid, p->comm);


        // --------------------------------------------------------------------
        // group targets
        o = p->group_target;
        if(likely(p->group_target && p->group_target->gid == p->gid))
            w = p->group_target;
        else {
            if(unlikely(debug && p->group_target))
                    fprintf(stderr, "apps.plugin: \t\tpid %d (%s) switched group from %u (%s) to %u.\n", p->pid, p->comm, p->group_target->gid, p->group_target->name, p->gid);

            w = p->group_target = get_groups_target(p->gid);
        }

        if(likely(w))
            aggregate_pid_on_target(w, p, o);
        else
            error("pid %d %s was left without a group target!", p->pid, p->comm);

    }

    count_targets_fds(apps_groups_root_target);
    count_targets_fds(users_root_target);
    count_targets_fds(groups_root_target);

    cleanup_exited_pids();
}

// ----------------------------------------------------------------------------
// update chart dimensions

BUFFER *output = NULL;
int print_calculated_number(char *str, calculated_number value) { (void)str; (void)value; return 0; }

static inline void send_BEGIN(const char *type, const char *id, unsigned long long usec) {
    // fprintf(stdout, "BEGIN %s.%s %llu\n", type, id, usec);
    buffer_strcat(output, "BEGIN ");
    buffer_strcat(output, type);
    buffer_strcat(output, ".");
    buffer_strcat(output, id);
    buffer_strcat(output, " ");
    buffer_print_llu(output, usec);
    buffer_strcat(output, "\n");
}

static inline void send_SET(const char *name, unsigned long long value) {
    // fprintf(stdout, "SET %s = %llu\n", name, value);
    buffer_strcat(output, "SET ");
    buffer_strcat(output, name);
    buffer_strcat(output, " = ");
    buffer_print_llu(output, value);
    buffer_strcat(output, "\n");
}

static inline void send_END(void) {
    // fprintf(stdout, "END\n");
    buffer_strcat(output, "END\n");
}

double utime_fix_ratio = 1.0, stime_fix_ratio = 1.0, gtime_fix_ratio = 1.0, cutime_fix_ratio = 1.0, cstime_fix_ratio = 1.0, cgtime_fix_ratio = 1.0;
double minflt_fix_ratio = 1.0, majflt_fix_ratio = 1.0, cminflt_fix_ratio = 1.0, cmajflt_fix_ratio = 1.0;

usec_t send_resource_usage_to_netdata() {
    static struct timeval last = { 0, 0 };
    static struct rusage me_last;

    struct timeval now;
    struct rusage me;

    usec_t usec;
    usec_t cpuuser;
    usec_t cpusyst;

    if(!last.tv_sec) {
        now_realtime_timeval(&last);
        getrusage(RUSAGE_SELF, &me_last);

        // the first time, give a zero to allow
        // netdata calibrate to the current time
        // usec = update_every * USEC_PER_SEC;
        usec = 0ULL;
        cpuuser = 0;
        cpusyst = 0;
    }
    else {
        now_realtime_timeval(&now);
        getrusage(RUSAGE_SELF, &me);

        usec = dt_usec(&now, &last);
        cpuuser = me.ru_utime.tv_sec * USEC_PER_SEC + me.ru_utime.tv_usec;
        cpusyst = me.ru_stime.tv_sec * USEC_PER_SEC + me.ru_stime.tv_usec;

        memmove(&last, &now, sizeof(struct timeval));
        memmove(&me_last, &me, sizeof(struct rusage));
    }

    buffer_sprintf(output,
        "BEGIN netdata.apps_cpu %llu\n"
        "SET user = %llu\n"
        "SET system = %llu\n"
        "END\n"
        "BEGIN netdata.apps_files %llu\n"
        "SET files = %llu\n"
        "SET pids = %ld\n"
        "SET fds = %d\n"
        "SET targets = %ld\n"
        "END\n"
        "BEGIN netdata.apps_fix %llu\n"
        "SET utime = %llu\n"
        "SET stime = %llu\n"
        "SET gtime = %llu\n"
        "SET minflt = %llu\n"
        "SET majflt = %llu\n"
        "END\n"
        , usec
        , cpuuser
        , cpusyst
        , usec
        , file_counter
        , all_pids_count
        , all_files_len
        , apps_groups_targets
        , usec
        , (unsigned long long)(utime_fix_ratio   * 100 * RATES_DETAIL)
        , (unsigned long long)(stime_fix_ratio   * 100 * RATES_DETAIL)
        , (unsigned long long)(gtime_fix_ratio   * 100 * RATES_DETAIL)
        , (unsigned long long)(minflt_fix_ratio  * 100 * RATES_DETAIL)
        , (unsigned long long)(majflt_fix_ratio  * 100 * RATES_DETAIL)
        );

    if(include_exited_childs)
        buffer_sprintf(output,
            "BEGIN netdata.apps_children_fix %llu\n"
            "SET cutime = %llu\n"
            "SET cstime = %llu\n"
            "SET cgtime = %llu\n"
            "SET cminflt = %llu\n"
            "SET cmajflt = %llu\n"
            "END\n"
            , usec
            , (unsigned long long)(cutime_fix_ratio  * 100 * RATES_DETAIL)
            , (unsigned long long)(cstime_fix_ratio  * 100 * RATES_DETAIL)
            , (unsigned long long)(cgtime_fix_ratio  * 100 * RATES_DETAIL)
            , (unsigned long long)(cminflt_fix_ratio * 100 * RATES_DETAIL)
            , (unsigned long long)(cmajflt_fix_ratio * 100 * RATES_DETAIL)
            );

    return usec;
}

void normalize_data(struct target *root) {
    struct target *w;

    // childs processing introduces spikes
    // here we try to eliminate them by disabling childs processing either for specific dimensions
    // or entirely. Of course, either way, we disable it just a single iteration.

    unsigned long long max = processors * hz * RATES_DETAIL;
    unsigned long long utime = 0, cutime = 0, stime = 0, cstime = 0, gtime = 0, cgtime = 0, minflt = 0, cminflt = 0, majflt = 0, cmajflt = 0;

    if(global_utime > max) global_utime = max;
    if(global_stime > max) global_stime = max;
    if(global_gtime > max) global_gtime = max;

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
            "SYSTEM: u=%llu s=%llu g=%llu "
            "COLLECTED: u=%llu s=%llu g=%llu cu=%llu cs=%llu cg=%llu "
            "DELTA: u=%lld s=%lld g=%lld "
            "FIX: u=%0.2f s=%0.2f g=%0.2f cu=%0.2f cs=%0.2f cg=%0.2f "
            "FINALLY: u=%llu s=%llu g=%llu cu=%llu cs=%llu cg=%llu "
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
            , (long long)utime + (long long)cutime - (long long)global_utime
            , (long long)stime + (long long)cstime - (long long)global_stime
            , (long long)gtime + (long long)cgtime - (long long)global_gtime
            , utime_fix_ratio
            , stime_fix_ratio
            , gtime_fix_ratio
            , cutime_fix_ratio
            , cstime_fix_ratio
            , cgtime_fix_ratio
            , (unsigned long long)(utime * utime_fix_ratio)
            , (unsigned long long)(stime * stime_fix_ratio)
            , (unsigned long long)(gtime * gtime_fix_ratio)
            , (unsigned long long)(cutime * cutime_fix_ratio)
            , (unsigned long long)(cstime * cstime_fix_ratio)
            , (unsigned long long)(cgtime * cgtime_fix_ratio)
            );
    }
}

void send_collected_data_to_netdata(struct target *root, const char *type, usec_t usec) {
    struct target *w;

    send_BEGIN(type, "cpu", usec);
    for (w = root; w ; w = w->next) {
        if(unlikely(w->exposed))
            send_SET(w->name, (unsigned long long)(w->utime * utime_fix_ratio) + (unsigned long long)(w->stime * stime_fix_ratio) + (unsigned long long)(w->gtime * gtime_fix_ratio) + (include_exited_childs?((unsigned long long)(w->cutime * cutime_fix_ratio) + (unsigned long long)(w->cstime * cstime_fix_ratio) + (unsigned long long)(w->cgtime * cgtime_fix_ratio)):0ULL));
    }
    send_END();

    send_BEGIN(type, "cpu_user", usec);
    for (w = root; w ; w = w->next) {
        if(unlikely(w->exposed))
            send_SET(w->name, (unsigned long long)(w->utime * utime_fix_ratio) + (include_exited_childs?((unsigned long long)(w->cutime * cutime_fix_ratio)):0ULL));
    }
    send_END();

    send_BEGIN(type, "cpu_system", usec);
    for (w = root; w ; w = w->next) {
        if(unlikely(w->exposed))
            send_SET(w->name, (unsigned long long)(w->stime * stime_fix_ratio) + (include_exited_childs?((unsigned long long)(w->cstime * cstime_fix_ratio)):0ULL));
    }
    send_END();

    if(show_guest_time) {
        send_BEGIN(type, "cpu_guest", usec);
        for (w = root; w ; w = w->next) {
            if(unlikely(w->exposed))
                send_SET(w->name, (unsigned long long)(w->gtime * gtime_fix_ratio) + (include_exited_childs?((unsigned long long)(w->cgtime * cgtime_fix_ratio)):0ULL));
        }
        send_END();
    }

    send_BEGIN(type, "threads", usec);
    for (w = root; w ; w = w->next) {
        if(unlikely(w->exposed))
            send_SET(w->name, w->num_threads);
    }
    send_END();

    send_BEGIN(type, "processes", usec);
    for (w = root; w ; w = w->next) {
        if(unlikely(w->exposed))
            send_SET(w->name, w->processes);
    }
    send_END();

    send_BEGIN(type, "mem", usec);
    for (w = root; w ; w = w->next) {
        if(unlikely(w->exposed))
            send_SET(w->name, (w->statm_resident > w->statm_share)?(w->statm_resident - w->statm_share):0ULL);
    }
    send_END();

    send_BEGIN(type, "vmem", usec);
    for (w = root; w ; w = w->next) {
        if(unlikely(w->exposed))
            send_SET(w->name, w->statm_size);
    }
    send_END();

    send_BEGIN(type, "minor_faults", usec);
    for (w = root; w ; w = w->next) {
        if(unlikely(w->exposed))
            send_SET(w->name, (unsigned long long)(w->minflt * minflt_fix_ratio) + (include_exited_childs?((unsigned long long)(w->cminflt * cminflt_fix_ratio)):0ULL));
    }
    send_END();

    send_BEGIN(type, "major_faults", usec);
    for (w = root; w ; w = w->next) {
        if(unlikely(w->exposed))
            send_SET(w->name, (unsigned long long)(w->majflt * majflt_fix_ratio) + (include_exited_childs?((unsigned long long)(w->cmajflt * cmajflt_fix_ratio)):0ULL));
    }
    send_END();

    send_BEGIN(type, "lreads", usec);
    for (w = root; w ; w = w->next) {
        if(unlikely(w->exposed))
            send_SET(w->name, w->io_logical_bytes_read);
    }
    send_END();

    send_BEGIN(type, "lwrites", usec);
    for (w = root; w ; w = w->next) {
        if(unlikely(w->exposed))
            send_SET(w->name, w->io_logical_bytes_written);
    }
    send_END();

    send_BEGIN(type, "preads", usec);
    for (w = root; w ; w = w->next) {
        if(unlikely(w->exposed))
            send_SET(w->name, w->io_storage_bytes_read);
    }
    send_END();

    send_BEGIN(type, "pwrites", usec);
    for (w = root; w ; w = w->next) {
        if(unlikely(w->exposed))
            send_SET(w->name, w->io_storage_bytes_written);
    }
    send_END();

    if(enable_file_charts) {
        send_BEGIN(type, "files", usec);
        for (w = root; w; w = w->next) {
            if (unlikely(w->exposed))
                send_SET(w->name, w->openfiles);
        }
        send_END();

        send_BEGIN(type, "sockets", usec);
        for (w = root; w; w = w->next) {
            if (unlikely(w->exposed))
                send_SET(w->name, w->opensockets);
        }
        send_END();

        send_BEGIN(type, "pipes", usec);
        for (w = root; w; w = w->next) {
            if (unlikely(w->exposed))
                send_SET(w->name, w->openpipes);
        }
        send_END();
    }
}


// ----------------------------------------------------------------------------
// generate the charts

void send_charts_updates_to_netdata(struct target *root, const char *type, const char *title)
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
    buffer_sprintf(output, "CHART %s.cpu '' '%s CPU Time (%d%% = %d core%s)' 'cpu time %%' cpu %s.cpu stacked 20001 %d\n", type, title, (processors * 100), processors, (processors>1)?"s":"", type, update_every);
    for (w = root; w ; w = w->next) {
        if(unlikely(w->exposed))
            buffer_sprintf(output, "DIMENSION %s '' absolute 1 %llu %s\n", w->name, hz * RATES_DETAIL / 100, w->hidden ? "hidden" : "");
    }

    buffer_sprintf(output, "CHART %s.mem '' '%s Real Memory (w/o shared)' 'MB' mem %s.mem stacked 20003 %d\n", type, title, type, update_every);
    for (w = root; w ; w = w->next) {
        if(unlikely(w->exposed))
            buffer_sprintf(output, "DIMENSION %s '' absolute %ld %ld\n", w->name, sysconf(_SC_PAGESIZE), 1024L*1024L);
    }

    buffer_sprintf(output, "CHART %s.vmem '' '%s Virtual Memory Size' 'MB' mem %s.vmem stacked 20004 %d\n", type, title, type, update_every);
    for (w = root; w ; w = w->next) {
        if(unlikely(w->exposed))
            buffer_sprintf(output, "DIMENSION %s '' absolute %ld %ld\n", w->name, sysconf(_SC_PAGESIZE), 1024L*1024L);
    }

    buffer_sprintf(output, "CHART %s.threads '' '%s Threads' 'threads' processes %s.threads stacked 20005 %d\n", type, title, type, update_every);
    for (w = root; w ; w = w->next) {
        if(unlikely(w->exposed))
            buffer_sprintf(output, "DIMENSION %s '' absolute 1 1\n", w->name);
    }

    buffer_sprintf(output, "CHART %s.processes '' '%s Processes' 'processes' processes %s.processes stacked 20004 %d\n", type, title, type, update_every);
    for (w = root; w ; w = w->next) {
        if(unlikely(w->exposed))
            buffer_sprintf(output, "DIMENSION %s '' absolute 1 1\n", w->name);
    }

    buffer_sprintf(output, "CHART %s.cpu_user '' '%s CPU User Time (%d%% = %d core%s)' 'cpu time %%' cpu %s.cpu_user stacked 20020 %d\n", type, title, (processors * 100), processors, (processors>1)?"s":"", type, update_every);
    for (w = root; w ; w = w->next) {
        if(unlikely(w->exposed))
            buffer_sprintf(output, "DIMENSION %s '' absolute 1 %llu\n", w->name, hz * RATES_DETAIL / 100LLU);
    }

    buffer_sprintf(output, "CHART %s.cpu_system '' '%s CPU System Time (%d%% = %d core%s)' 'cpu time %%' cpu %s.cpu_system stacked 20021 %d\n", type, title, (processors * 100), processors, (processors>1)?"s":"", type, update_every);
    for (w = root; w ; w = w->next) {
        if(unlikely(w->exposed))
            buffer_sprintf(output, "DIMENSION %s '' absolute 1 %llu\n", w->name, hz * RATES_DETAIL / 100LLU);
    }

    if(show_guest_time) {
        buffer_sprintf(output, "CHART %s.cpu_guest '' '%s CPU Guest Time (%d%% = %d core%s)' 'cpu time %%' cpu %s.cpu_system stacked 20022 %d\n", type, title, (processors * 100), processors, (processors > 1) ? "s" : "", type, update_every);
        for (w = root; w; w = w->next) {
            if(unlikely(w->exposed))
                buffer_sprintf(output, "DIMENSION %s '' absolute 1 %llu\n", w->name, hz * RATES_DETAIL / 100LLU);
        }
    }

    buffer_sprintf(output, "CHART %s.major_faults '' '%s Major Page Faults (swap read)' 'page faults/s' swap %s.major_faults stacked 20010 %d\n", type, title, type, update_every);
    for (w = root; w ; w = w->next) {
        if(unlikely(w->exposed))
            buffer_sprintf(output, "DIMENSION %s '' absolute 1 %llu\n", w->name, RATES_DETAIL);
    }

    buffer_sprintf(output, "CHART %s.minor_faults '' '%s Minor Page Faults' 'page faults/s' mem %s.minor_faults stacked 20011 %d\n", type, title, type, update_every);
    for (w = root; w ; w = w->next) {
        if(unlikely(w->exposed))
            buffer_sprintf(output, "DIMENSION %s '' absolute 1 %llu\n", w->name, RATES_DETAIL);
    }

    buffer_sprintf(output, "CHART %s.lreads '' '%s Disk Logical Reads' 'kilobytes/s' disk %s.lreads stacked 20042 %d\n", type, title, type, update_every);
    for (w = root; w ; w = w->next) {
        if(unlikely(w->exposed))
            buffer_sprintf(output, "DIMENSION %s '' absolute 1 %llu\n", w->name, 1024LLU * RATES_DETAIL);
    }

    buffer_sprintf(output, "CHART %s.lwrites '' '%s I/O Logical Writes' 'kilobytes/s' disk %s.lwrites stacked 20042 %d\n", type, title, type, update_every);
    for (w = root; w ; w = w->next) {
        if(unlikely(w->exposed))
            buffer_sprintf(output, "DIMENSION %s '' absolute 1 %llu\n", w->name, 1024LLU * RATES_DETAIL);
    }

    buffer_sprintf(output, "CHART %s.preads '' '%s Disk Reads' 'kilobytes/s' disk %s.preads stacked 20002 %d\n", type, title, type, update_every);
    for (w = root; w ; w = w->next) {
        if(unlikely(w->exposed))
            buffer_sprintf(output, "DIMENSION %s '' absolute 1 %llu\n", w->name, 1024LLU * RATES_DETAIL);
    }

    buffer_sprintf(output, "CHART %s.pwrites '' '%s Disk Writes' 'kilobytes/s' disk %s.pwrites stacked 20002 %d\n", type, title, type, update_every);
    for (w = root; w ; w = w->next) {
        if(unlikely(w->exposed))
            buffer_sprintf(output, "DIMENSION %s '' absolute 1 %llu\n", w->name, 1024LLU * RATES_DETAIL);
    }

    if(enable_file_charts) {
        buffer_sprintf(output, "CHART %s.files '' '%s Open Files' 'open files' disk %s.files stacked 20050 %d\n", type,
                       title, type, update_every);
        for (w = root; w; w = w->next) {
            if (unlikely(w->exposed))
                buffer_sprintf(output, "DIMENSION %s '' absolute 1 1\n", w->name);
        }

        buffer_sprintf(output, "CHART %s.sockets '' '%s Open Sockets' 'open sockets' net %s.sockets stacked 20051 %d\n",
                       type, title, type, update_every);
        for (w = root; w; w = w->next) {
            if (unlikely(w->exposed))
                buffer_sprintf(output, "DIMENSION %s '' absolute 1 1\n", w->name);
        }

        buffer_sprintf(output, "CHART %s.pipes '' '%s Pipes' 'open pipes' processes %s.pipes stacked 20053 %d\n", type,
                       title, type, update_every);
        for (w = root; w; w = w->next) {
            if (unlikely(w->exposed))
                buffer_sprintf(output, "DIMENSION %s '' absolute 1 1\n", w->name);
        }
    }
}


// ----------------------------------------------------------------------------
// parse command line arguments

void parse_args(int argc, char **argv)
{
    int i, freq = 0;
    char *name = NULL;

    for(i = 1; i < argc; i++) {
        if(!freq) {
            int n = atoi(argv[i]);
            if(n > 0) {
                freq = n;
                continue;
            }
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
                    "apps.plugin\n"
                    "(C) 2016 Costa Tsaousis"
                    "GPL v3+\n"
                    "This program is a data collector plugin for netdata.\n"
                    "\n"
                    "Valid command line options:\n"
                    "\n"
                    "SECONDS           set the data collection frequency\n"
                    "\n"
                    "debug             enable debugging (lot of output)\n"
                    "\n"
                    "with-childs\n"
                    "without-childs    enable / disable aggregating exited\n"
                    "                  children resources into parents\n"
                    "                  (default is enabled)\n"
                    "\n"
                    "with-guest\n"
                    "without-guest     enable / disable reporting guest charts\n"
                    "                  (default is disabled)\n"
                    "\n"
                    "with-files\n"
                    "without-files     enable / disable reporting files, sockets, pipes\n"
                    "                  (default is enabled)\n"
                    "\n"
                    "NAME              read apps_NAME.conf instead of\n"
                    "                  apps_groups.conf\n"
                    "                  (default NAME=groups)\n"
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
        error("Cannot read process groups %s", name);
        exit(1);
    }
}

int main(int argc, char **argv)
{
    // debug_flags = D_PROCFILE;

    // set the name for logging
    program_name = "apps.plugin";

    info("started on pid %d", getpid());

    // disable syslog for apps.plugin
    error_log_syslog = 0;

    // set errors flood protection to 100 logs per hour
    error_log_errors_per_period = 100;
    error_log_throttle_period = 3600;

    global_host_prefix = getenv("NETDATA_HOST_PREFIX");
    if(global_host_prefix == NULL) {
        // info("NETDATA_HOST_PREFIX is not passed from netdata");
        global_host_prefix = "";
    }
    // else info("Found NETDATA_HOST_PREFIX='%s'", global_host_prefix);

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
        prctl(PR_SET_DUMPABLE, 1, 0, 0, 0);
    }
#endif /* NETDATA_INTERNAL_CHECKS */

    procfile_adaptive_initial_allocation = 1;

    time_t started_t = now_realtime_sec();
    get_system_HZ();
    get_system_pid_max();
    get_system_cpus();

    parse_args(argc, argv);

    all_pids_sortlist = callocz(sizeof(pid_t), (size_t)pid_max);
    all_pids = callocz(sizeof(struct pid_stat *), (size_t) pid_max);

    output = buffer_create(1024);
    buffer_sprintf(output,
        "CHART netdata.apps_cpu '' 'Apps Plugin CPU' 'milliseconds/s' apps.plugin netdata.apps_cpu stacked 140000 %1$d\n"
        "DIMENSION user '' incremental 1 1000\n"
        "DIMENSION system '' incremental 1 1000\n"
        "CHART netdata.apps_files '' 'Apps Plugin Files' 'files/s' apps.plugin netdata.apps_files line 140001 %1$d\n"
        "DIMENSION files '' incremental 1 1\n"
        "DIMENSION pids '' absolute 1 1\n"
        "DIMENSION fds '' absolute 1 1\n"
        "DIMENSION targets '' absolute 1 1\n"
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
        buffer_sprintf(output,
            "CHART netdata.apps_children_fix '' 'Apps Plugin Exited Children Normalization Ratios' 'percentage' apps.plugin netdata.apps_children_fix line 140003 %1$d\n"
            "DIMENSION cutime '' absolute 1 %2$llu\n"
            "DIMENSION cstime '' absolute 1 %2$llu\n"
            "DIMENSION cgtime '' absolute 1 %2$llu\n"
            "DIMENSION cminflt '' absolute 1 %2$llu\n"
            "DIMENSION cmajflt '' absolute 1 %2$llu\n"
            , update_every
            , RATES_DETAIL
            );

    usec_t step = update_every * USEC_PER_SEC;
    global_iterations_counter = 1;
    for(;1; global_iterations_counter++) {
        usec_t now = now_realtime_usec();
        usec_t next = now - (now % step) + step;

        while(now < next) {
            sleep_usec(next - now);
            now = now_realtime_usec();
        }

        if(!collect_data_for_all_processes_from_proc()) {
            error("Cannot collect /proc data for running processes. Disabling apps.plugin...");
            printf("DISABLE\n");
            exit(1);
        }

        calculate_netdata_statistics();
        normalize_data(apps_groups_root_target);

        usec_t dt = send_resource_usage_to_netdata();

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

        show_guest_time_old = show_guest_time;

        if(write(STDOUT_FILENO, buffer_tostring(output), buffer_strlen(output)) == -1)
            fatal("Cannot send chart values to netdata.");

        // fflush(stdout);
        buffer_flush(output);

        if(unlikely(debug))
            fprintf(stderr, "apps.plugin: done Loop No %llu\n", global_iterations_counter);

        time_t current_t = now_realtime_sec();

        // restart check (14400 seconds)
        if(current_t - started_t > 14400) exit(0);
    }
}
