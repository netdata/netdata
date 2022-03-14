// SPDX-License-Identifier: GPL-3.0-or-later

#include "ebpf.h"
#include "ebpf_socket.h"
#include "ebpf_apps.h"

// ----------------------------------------------------------------------------
// internal flags
// handled in code (automatically set)

static int proc_pid_cmdline_is_needed = 0; // 1 when we need to read /proc/cmdline

/*****************************************************************
 *
 *  FUNCTIONS USED TO READ HASH TABLES
 *
 *****************************************************************/

/**
 * Read statistic hash table.
 *
 * @param ep                    the output structure.
 * @param fd                    the file descriptor mapped from kernel ring.
 * @param pid                   the index used to select the data.
 * @param bpf_map_lookup_elem   a pointer for the function used to read data.
 *
 * @return It returns 0 when the data was copied and -1 otherwise
 */
int ebpf_read_hash_table(void *ep, int fd, uint32_t pid)
{
    if (!ep)
        return -1;

    if (!bpf_map_lookup_elem(fd, &pid, ep))
        return 0;

    return -1;
}

/**
 * Read socket statistic
 *
 * Read information from kernel ring to user ring.
 *
 * @param ep    the table with all process stats values.
 * @param fd    the file descriptor mapped from kernel
 * @param ef    a pointer for the functions mapped from dynamic library
 * @param pids  the list of pids associated to a target.
 *
 * @return
 */
size_t read_bandwidth_statistic_using_pid_on_target(ebpf_bandwidth_t **ep, int fd, struct pid_on_target *pids)
{
    size_t count = 0;
    while (pids) {
        uint32_t current_pid = pids->pid;
        if (!ebpf_read_hash_table(ep[current_pid], fd, current_pid))
            count++;

        pids = pids->next;
    }

    return count;
}

/**
 * Read bandwidth statistic using hash table
 *
 * @param out                   the output tensor that will receive the information.
 * @param fd                    the file descriptor that has the data
 * @param bpf_map_lookup_elem   a pointer for the function to read the data
 * @param bpf_map_get_next_key  a pointer fo the function to read the index.
 */
size_t read_bandwidth_statistic_using_hash_table(ebpf_bandwidth_t **out, int fd)
{
    size_t count = 0;
    uint32_t key = 0;
    uint32_t next_key = 0;

    while (bpf_map_get_next_key(fd, &key, &next_key) == 0) {
        ebpf_bandwidth_t *eps = out[next_key];
        if (!eps) {
            eps = callocz(1, sizeof(ebpf_process_stat_t));
            out[next_key] = eps;
        }
        ebpf_read_hash_table(eps, fd, next_key);
    }

    return count;
}

/*****************************************************************
 *
 *  FUNCTIONS CALLED FROM COLLECTORS
 *
 *****************************************************************/

/**
 * Am I running as Root
 *
 * Verify the user that is running the collector.
 *
 * @return It returns 1 for root and 0 otherwise.
 */
int am_i_running_as_root()
{
    uid_t uid = getuid(), euid = geteuid();

    if (uid == 0 || euid == 0) {
        return 1;
    }

    return 0;
}

/**
 * Reset the target values
 *
 * @param root the pointer to the chain that will be reset.
 *
 * @return it returns the number of structures that was reset.
 */
size_t zero_all_targets(struct target *root)
{
    struct target *w;
    size_t count = 0;

    for (w = root; w; w = w->next) {
        count++;

        if (unlikely(w->root_pid)) {
            struct pid_on_target *pid_on_target = w->root_pid;

            while (pid_on_target) {
                struct pid_on_target *pid_on_target_to_free = pid_on_target;
                pid_on_target = pid_on_target->next;
                free(pid_on_target_to_free);
            }

            w->root_pid = NULL;
        }
    }

    return count;
}

/**
 * Clean the allocated structures
 *
 * @param agrt the pointer to be cleaned.
 */
void clean_apps_groups_target(struct target *agrt)
{
    struct target *current_target;
    while (agrt) {
        current_target = agrt;
        agrt = current_target->target;

        freez(current_target);
    }
}

/**
 * Find or create a new target
 * there are targets that are just aggregated to other target (the second argument)
 *
 * @param id
 * @param target
 * @param name
 *
 * @return It returns the target on success and NULL otherwise
 */
struct target *get_apps_groups_target(struct target **agrt, const char *id, struct target *target, const char *name)
{
    int tdebug = 0, thidden = target ? target->hidden : 0, ends_with = 0;
    const char *nid = id;

    // extract the options
    while (nid[0] == '-' || nid[0] == '+' || nid[0] == '*') {
        if (nid[0] == '-')
            thidden = 1;
        if (nid[0] == '+')
            tdebug = 1;
        if (nid[0] == '*')
            ends_with = 1;
        nid++;
    }
    uint32_t hash = simple_hash(id);

    // find if it already exists
    struct target *w, *last = *agrt;
    for (w = *agrt; w; w = w->next) {
        if (w->idhash == hash && strncmp(nid, w->id, MAX_NAME) == 0)
            return w;

        last = w;
    }

    // find an existing target
    if (unlikely(!target)) {
        while (*name == '-') {
            if (*name == '-')
                thidden = 1;
            name++;
        }

        for (target = *agrt; target != NULL; target = target->next) {
            if (!target->target && strcmp(name, target->name) == 0)
                break;
        }
    }

    if (target && target->target)
        fatal(
            "Internal Error: request to link process '%s' to target '%s' which is linked to target '%s'", id,
            target->id, target->target->id);

    w = callocz(1, sizeof(struct target));
    strncpyz(w->id, nid, MAX_NAME);
    w->idhash = simple_hash(w->id);

    if (unlikely(!target))
        // copy the name
        strncpyz(w->name, name, MAX_NAME);
    else
        // copy the id
        strncpyz(w->name, nid, MAX_NAME);

    strncpyz(w->compare, nid, MAX_COMPARE_NAME);
    size_t len = strlen(w->compare);
    if (w->compare[len - 1] == '*') {
        w->compare[len - 1] = '\0';
        w->starts_with = 1;
    }
    w->ends_with = ends_with;

    if (w->starts_with && w->ends_with)
        proc_pid_cmdline_is_needed = 1;

    w->comparehash = simple_hash(w->compare);
    w->comparelen = strlen(w->compare);

    w->hidden = thidden;
#ifdef NETDATA_INTERNAL_CHECKS
    w->debug_enabled = tdebug;
#else
    if (tdebug)
        fprintf(stderr, "apps.plugin has been compiled without debugging\n");
#endif
    w->target = target;

    // append it, to maintain the order in apps_groups.conf
    if (last)
        last->next = w;
    else
        *agrt = w;

    return w;
}

/**
 * Read the apps_groups.conf file
 *
 * @param agrt a pointer to apps_group_root_target
 * @param path the directory to search apps_%s.conf
 * @param file the word to complement the file name.
 *
 * @return It returns 0 on success and -1 otherwise
 */
int ebpf_read_apps_groups_conf(struct target **agdt, struct target **agrt, const char *path, const char *file)
{
    char filename[FILENAME_MAX + 1];

    snprintfz(filename, FILENAME_MAX, "%s/apps_%s.conf", path, file);

    // ----------------------------------------

    procfile *ff = procfile_open_no_log(filename, " :\t", PROCFILE_FLAG_DEFAULT);
    if (!ff)
        return -1;

    procfile_set_quotes(ff, "'\"");

    ff = procfile_readall(ff);
    if (!ff)
        return -1;

    size_t line, lines = procfile_lines(ff);

    for (line = 0; line < lines; line++) {
        size_t word, words = procfile_linewords(ff, line);
        if (!words)
            continue;

        char *name = procfile_lineword(ff, line, 0);
        if (!name || !*name)
            continue;

        // find a possibly existing target
        struct target *w = NULL;

        // loop through all words, skipping the first one (the name)
        for (word = 0; word < words; word++) {
            char *s = procfile_lineword(ff, line, word);
            if (!s || !*s)
                continue;
            if (*s == '#')
                break;

            // is this the first word? skip it
            if (s == name)
                continue;

            // add this target
            struct target *n = get_apps_groups_target(agrt, s, w, name);
            if (!n) {
                error("Cannot create target '%s' (line %zu, word %zu)", s, line, word);
                continue;
            }

            // just some optimization
            // to avoid searching for a target for each process
            if (!w)
                w = n->target ? n->target : n;
        }
    }

    procfile_close(ff);

    *agdt = get_apps_groups_target(agrt, "p+!o@w#e$i^r&7*5(-i)l-o_", NULL, "other"); // match nothing
    if (!*agdt)
        fatal("Cannot create default target");

    struct target *ptr = *agdt;
    if (ptr->target)
        *agdt = ptr->target;

    return 0;
}

// the minimum PID of the system
// this is also the pid of the init process
#define INIT_PID 1

// ----------------------------------------------------------------------------
// string lengths

#define MAX_COMPARE_NAME 100
#define MAX_NAME 100
#define MAX_CMDLINE 16384

struct pid_stat **all_pids = NULL;    // to avoid allocations, we pre-allocate the
                                      // the entire pid space.
struct pid_stat *root_of_pids = NULL; // global list of all processes running

size_t all_pids_count = 0; // the number of processes running

struct target
    *apps_groups_default_target = NULL, // the default target
    *apps_groups_root_target = NULL,    // apps_groups.conf defined
    *users_root_target = NULL,          // users
    *groups_root_target = NULL;         // user groups

size_t apps_groups_targets_count = 0; // # of apps_groups.conf targets

// ----------------------------------------------------------------------------
// internal counters

static size_t
    // global_iterations_counter = 1,
    calls_counter = 0,
    // file_counter = 0,
    // filenames_allocated_counter = 0,
    // inodes_changed_counter = 0,
    // links_changed_counter = 0,
    targets_assignment_counter = 0;

// ----------------------------------------------------------------------------
// debugging

// log each problem once per process
// log flood protection flags (log_thrown)
#define PID_LOG_IO 0x00000001
#define PID_LOG_STATUS 0x00000002
#define PID_LOG_CMDLINE 0x00000004
#define PID_LOG_FDS 0x00000008
#define PID_LOG_STAT 0x00000010

int debug_enabled = 0;

#ifdef NETDATA_INTERNAL_CHECKS

#define debug_log(fmt, args...)                                                                                        \
    do {                                                                                                               \
        if (unlikely(debug_enabled))                                                                                   \
            debug_log_int(fmt, ##args);                                                                                \
    } while (0)

#else

static inline void debug_log_dummy(void)
{
}
#define debug_log(fmt, args...) debug_log_dummy()

#endif

/**
 * Managed log
 *
 * Store log information if it is necessary.
 *
 * @param p         the pid stat structure
 * @param log       the log id
 * @param status    the return from a function.
 *
 * @return It returns the status value.
 */
static inline int managed_log(struct pid_stat *p, uint32_t log, int status)
{
    if (unlikely(!status)) {
        // error("command failed log %u, errno %d", log, errno);

        if (unlikely(debug_enabled || errno != ENOENT)) {
            if (unlikely(debug_enabled || !(p->log_thrown & log))) {
                p->log_thrown |= log;
                switch (log) {
                    case PID_LOG_IO:
                        error(
                            "Cannot process %s/proc/%d/io (command '%s')", netdata_configured_host_prefix, p->pid,
                            p->comm);
                        break;

                    case PID_LOG_STATUS:
                        error(
                            "Cannot process %s/proc/%d/status (command '%s')", netdata_configured_host_prefix, p->pid,
                            p->comm);
                        break;

                    case PID_LOG_CMDLINE:
                        error(
                            "Cannot process %s/proc/%d/cmdline (command '%s')", netdata_configured_host_prefix, p->pid,
                            p->comm);
                        break;

                    case PID_LOG_FDS:
                        error(
                            "Cannot process entries in %s/proc/%d/fd (command '%s')", netdata_configured_host_prefix,
                            p->pid, p->comm);
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
    } else if (unlikely(p->log_thrown & log)) {
        // error("unsetting log %u on pid %d", log, p->pid);
        p->log_thrown &= ~log;
    }

    return status;
}

/**
 * Get PID entry
 *
 * Get or allocate the PID entry for the specified pid.
 *
 * @param pid the pid to search the data.
 *
 * @return It returns the pid entry structure
 */
static inline struct pid_stat *get_pid_entry(pid_t pid)
{
    if (unlikely(all_pids[pid]))
        return all_pids[pid];

    struct pid_stat *p = callocz(1, sizeof(struct pid_stat));

    if (likely(root_of_pids))
        root_of_pids->prev = p;

    p->next = root_of_pids;
    root_of_pids = p;

    p->pid = pid;

    all_pids[pid] = p;
    all_pids_count++;

    return p;
}

/**
 * Assign the PID to a target.
 *
 * @param p the pid_stat structure to assign for a target.
 */
static inline void assign_target_to_pid(struct pid_stat *p)
{
    targets_assignment_counter++;

    uint32_t hash = simple_hash(p->comm);
    size_t pclen = strlen(p->comm);

    struct target *w;
    for (w = apps_groups_root_target; w; w = w->next) {
        // if(debug_enabled || (p->target && p->target->debug_enabled)) debug_log_int("\t\tcomparing '%s' with '%s'", w->compare, p->comm);

        // find it - 4 cases:
        // 1. the target is not a pattern
        // 2. the target has the prefix
        // 3. the target has the suffix
        // 4. the target is something inside cmdline

        if (unlikely(
                ((!w->starts_with && !w->ends_with && w->comparehash == hash && !strcmp(w->compare, p->comm)) ||
                 (w->starts_with && !w->ends_with && !strncmp(w->compare, p->comm, w->comparelen)) ||
                 (!w->starts_with && w->ends_with && pclen >= w->comparelen && !strcmp(w->compare, &p->comm[pclen - w->comparelen])) ||
                 (proc_pid_cmdline_is_needed && w->starts_with && w->ends_with && p->cmdline && strstr(p->cmdline, w->compare))))) {
            if (w->target)
                p->target = w->target;
            else
                p->target = w;

            if (debug_enabled || (p->target && p->target->debug_enabled))
                debug_log_int("%s linked to target %s", p->comm, p->target->name);

            break;
        }
    }
}

// ----------------------------------------------------------------------------
// update pids from proc

/**
 * Read cmd line from /proc/PID/cmdline
 *
 * @param p  the pid_stat_structure.
 *
 * @return It returns 1 on success and 0 otherwise.
 */
static inline int read_proc_pid_cmdline(struct pid_stat *p)
{
    static char cmdline[MAX_CMDLINE + 1];

    if (unlikely(!p->cmdline_filename)) {
        char filename[FILENAME_MAX + 1];
        snprintfz(filename, FILENAME_MAX, "%s/proc/%d/cmdline", netdata_configured_host_prefix, p->pid);
        p->cmdline_filename = strdupz(filename);
    }

    int fd = open(p->cmdline_filename, procfile_open_flags, 0666);
    if (unlikely(fd == -1))
        goto cleanup;

    ssize_t i, bytes = read(fd, cmdline, MAX_CMDLINE);
    close(fd);

    if (unlikely(bytes < 0))
        goto cleanup;

    cmdline[bytes] = '\0';
    for (i = 0; i < bytes; i++) {
        if (unlikely(!cmdline[i]))
            cmdline[i] = ' ';
    }

    if (p->cmdline)
        freez(p->cmdline);
    p->cmdline = strdupz(cmdline);

    debug_log("Read file '%s' contents: %s", p->cmdline_filename, p->cmdline);

    return 1;

cleanup:
    // copy the command to the command line
    if (p->cmdline)
        freez(p->cmdline);
    p->cmdline = strdupz(p->comm);
    return 0;
}

/**
 * Read information from /proc/PID/stat and /proc/PID/cmdline
 * Assign target to pid
 *
 * @param p the pid stat structure to store the data.
 * @param ptr an useless argument.
 */
static inline int read_proc_pid_stat(struct pid_stat *p, void *ptr)
{
    UNUSED(ptr);

    static procfile *ff = NULL;

    if (unlikely(!p->stat_filename)) {
        char filename[FILENAME_MAX + 1];
        snprintfz(filename, FILENAME_MAX, "%s/proc/%d/stat", netdata_configured_host_prefix, p->pid);
        p->stat_filename = strdupz(filename);
    }

    int set_quotes = (!ff) ? 1 : 0;

    struct stat statbuf;
    if (stat(p->stat_filename, &statbuf))
        return 0;

    ff = procfile_reopen(ff, p->stat_filename, NULL, PROCFILE_FLAG_NO_ERROR_ON_FILE_IO);
    if (unlikely(!ff))
        return 0;

    if (unlikely(set_quotes))
        procfile_set_open_close(ff, "(", ")");

    ff = procfile_readall(ff);
    if (unlikely(!ff))
        return 0;

    p->last_stat_collected_usec = p->stat_collected_usec;
    p->stat_collected_usec = now_monotonic_usec();
    calls_counter++;

    char *comm = procfile_lineword(ff, 0, 1);
    p->ppid = (int32_t)str2pid_t(procfile_lineword(ff, 0, 3));

    if (strcmp(p->comm, comm) != 0) {
        if (unlikely(debug_enabled)) {
            if (p->comm[0])
                debug_log("\tpid %d (%s) changed name to '%s'", p->pid, p->comm, comm);
            else
                debug_log("\tJust added %d (%s)", p->pid, comm);
        }

        strncpyz(p->comm, comm, MAX_COMPARE_NAME);

        // /proc/<pid>/cmdline
        if (likely(proc_pid_cmdline_is_needed))
            managed_log(p, PID_LOG_CMDLINE, read_proc_pid_cmdline(p));

        assign_target_to_pid(p);
    }

    if (unlikely(debug_enabled || (p->target && p->target->debug_enabled)))
        debug_log_int(
            "READ PROC/PID/STAT: %s/proc/%d/stat, process: '%s' on target '%s' (dt=%llu)",
            netdata_configured_host_prefix, p->pid, p->comm, (p->target) ? p->target->name : "UNSET",
            p->stat_collected_usec - p->last_stat_collected_usec);

    return 1;
}

/**
 * Collect data for PID
 *
 * @param pid the current pid that we are working
 * @param ptr a NULL value
 *
 * @return It returns 1 on success and 0 otherwise
 */
static inline int collect_data_for_pid(pid_t pid, void *ptr)
{
    if (unlikely(pid < 0 || pid > pid_max)) {
        error("Invalid pid %d read (expected %d to %d). Ignoring process.", pid, 0, pid_max);
        return 0;
    }

    struct pid_stat *p = get_pid_entry(pid);
    if (unlikely(!p || p->read))
        return 0;
    p->read = 1;

    if (unlikely(!managed_log(p, PID_LOG_STAT, read_proc_pid_stat(p, ptr))))
        // there is no reason to proceed if we cannot get its status
        return 0;

    // check its parent pid
    if (unlikely(p->ppid < 0 || p->ppid > pid_max)) {
        error("Pid %d (command '%s') states invalid parent pid %d. Using 0.", pid, p->comm, p->ppid);
        p->ppid = 0;
    }

    // mark it as updated
    p->updated = 1;
    p->keep = 0;
    p->keeploops = 0;

    return 1;
}

/**
 * Fill link list of parents with children PIDs
 */
static inline void link_all_processes_to_their_parents(void)
{
    struct pid_stat *p, *pp;

    // link all children to their parents
    // and update children count on parents
    for (p = root_of_pids; p; p = p->next) {
        // for each process found

        p->sortlist = 0;
        p->parent = NULL;

        if (unlikely(!p->ppid)) {
            p->parent = NULL;
            continue;
        }

        pp = all_pids[p->ppid];
        if (likely(pp)) {
            p->parent = pp;
            pp->children_count++;

            if (unlikely(debug_enabled || (p->target && p->target->debug_enabled)))
                debug_log_int(
                    "child %d (%s, %s) on target '%s' has parent %d (%s, %s).", p->pid, p->comm,
                    p->updated ? "running" : "exited", (p->target) ? p->target->name : "UNSET", pp->pid, pp->comm,
                    pp->updated ? "running" : "exited");
        } else {
            p->parent = NULL;
            debug_log("pid %d %s states parent %d, but the later does not exist.", p->pid, p->comm, p->ppid);
        }
    }
}

/**
 * Aggregate PIDs to targets.
 */
static void apply_apps_groups_targets_inheritance(void)
{
    struct pid_stat *p = NULL;

    // children that do not have a target
    // inherit their target from their parent
    int found = 1, loops = 0;
    while (found) {
        if (unlikely(debug_enabled))
            loops++;
        found = 0;
        for (p = root_of_pids; p; p = p->next) {
            // if this process does not have a target
            // and it has a parent
            // and its parent has a target
            // then, set the parent's target to this process
            if (unlikely(!p->target && p->parent && p->parent->target)) {
                p->target = p->parent->target;
                found++;

                if (debug_enabled || (p->target && p->target->debug_enabled))
                    debug_log_int(
                        "TARGET INHERITANCE: %s is inherited by %d (%s) from its parent %d (%s).", p->target->name,
                        p->pid, p->comm, p->parent->pid, p->parent->comm);
            }
        }
    }

    // find all the procs with 0 childs and merge them to their parents
    // repeat, until nothing more can be done.
    int sortlist = 1;
    found = 1;
    while (found) {
        if (unlikely(debug_enabled))
            loops++;
        found = 0;

        for (p = root_of_pids; p; p = p->next) {
            if (unlikely(!p->sortlist && !p->children_count))
                p->sortlist = sortlist++;

            if (unlikely(
                    !p->children_count           // if this process does not have any children
                    && !p->merged                // and is not already merged
                    && p->parent                 // and has a parent
                    && p->parent->children_count // and its parent has children
                                                 // and the target of this process and its parent is the same,
                                                 // or the parent does not have a target
                    && (p->target == p->parent->target || !p->parent->target) &&
                    p->ppid != INIT_PID // and its parent is not init
                    )) {
                // mark it as merged
                p->parent->children_count--;
                p->merged = 1;

                // the parent inherits the child's target, if it does not have a target itself
                if (unlikely(p->target && !p->parent->target)) {
                    p->parent->target = p->target;

                    if (debug_enabled || (p->target && p->target->debug_enabled))
                        debug_log_int(
                            "TARGET INHERITANCE: %s is inherited by %d (%s) from its child %d (%s).", p->target->name,
                            p->parent->pid, p->parent->comm, p->pid, p->comm);
                }

                found++;
            }
        }

        debug_log("TARGET INHERITANCE: merged %d processes", found);
    }

    // init goes always to default target
    if (all_pids[INIT_PID])
        all_pids[INIT_PID]->target = apps_groups_default_target;

    // pid 0 goes always to default target
    if (all_pids[0])
        all_pids[0]->target = apps_groups_default_target;

    // give a default target on all top level processes
    if (unlikely(debug_enabled))
        loops++;
    for (p = root_of_pids; p; p = p->next) {
        // if the process is not merged itself
        // then is is a top level process
        if (unlikely(!p->merged && !p->target))
            p->target = apps_groups_default_target;

        // make sure all processes have a sortlist
        if (unlikely(!p->sortlist))
            p->sortlist = sortlist++;
    }

    if (all_pids[1])
        all_pids[1]->sortlist = sortlist++;

    // give a target to all merged child processes
    found = 1;
    while (found) {
        if (unlikely(debug_enabled))
            loops++;
        found = 0;
        for (p = root_of_pids; p; p = p->next) {
            if (unlikely(!p->target && p->merged && p->parent && p->parent->target)) {
                p->target = p->parent->target;
                found++;

                if (debug_enabled || (p->target && p->target->debug_enabled))
                    debug_log_int(
                        "TARGET INHERITANCE: %s is inherited by %d (%s) from its parent %d (%s) at phase 2.",
                        p->target->name, p->pid, p->comm, p->parent->pid, p->parent->comm);
            }
        }
    }

    debug_log("apply_apps_groups_targets_inheritance() made %d loops on the process tree", loops);
}

/**
 * Update target timestamp.
 *
 * @param root the targets that will be updated.
 */
static inline void post_aggregate_targets(struct target *root)
{
    struct target *w;
    for (w = root; w; w = w->next) {
        if (w->collected_starttime) {
            if (!w->starttime || w->collected_starttime < w->starttime) {
                w->starttime = w->collected_starttime;
            }
        } else {
            w->starttime = 0;
        }
    }
}

/**
 * Remove PID from the link list.
 *
 * @param pid the PID that will be removed.
 */
static inline void del_pid_entry(pid_t pid)
{
    struct pid_stat *p = all_pids[pid];

    if (unlikely(!p)) {
        error("attempted to free pid %d that is not allocated.", pid);
        return;
    }

    debug_log("process %d %s exited, deleting it.", pid, p->comm);

    if (root_of_pids == p)
        root_of_pids = p->next;

    if (p->next)
        p->next->prev = p->prev;
    if (p->prev)
        p->prev->next = p->next;

    freez(p->stat_filename);
    freez(p->status_filename);
    freez(p->io_filename);
    freez(p->cmdline_filename);
    freez(p->cmdline);
    freez(p);

    all_pids[pid] = NULL;
    all_pids_count--;
}

/**
 * Get command string associated with a PID.
 * This can only safely be used when holding the `collect_data_mutex` lock.
 *
 * @param pid the pid to search the data.
 * @param n the maximum amount of bytes to copy into dest.
 *          if this is greater than the size of the command, it is clipped.
 * @param dest the target memory buffer to write the command into.
 * @return -1 if the PID hasn't been scraped yet, 0 otherwise.
 */
int get_pid_comm(pid_t pid, size_t n, char *dest)
{
    struct pid_stat *stat;

    stat = all_pids[pid];
    if (unlikely(stat == NULL)) {
        return -1;
    }

    if (unlikely(n > sizeof(stat->comm))) {
        n = sizeof(stat->comm);
    }

    strncpyz(dest, stat->comm, n);
    return 0;
}

/**
 * Cleanup variable from other threads
 *
 * @param pid current pid.
 */
void cleanup_variables_from_other_threads(uint32_t pid)
{
    // Clean socket structures
    if (socket_bandwidth_curr) {
        freez(socket_bandwidth_curr[pid]);
        socket_bandwidth_curr[pid] = NULL;
    }

    // Clean cachestat structure
    if (cachestat_pid) {
        freez(cachestat_pid[pid]);
        cachestat_pid[pid] = NULL;
    }

    // Clean directory cache structure
    if (dcstat_pid) {
        freez(dcstat_pid[pid]);
        dcstat_pid[pid] = NULL;
    }

    // Clean swap structure
    if (swap_pid) {
        freez(swap_pid[pid]);
        swap_pid[pid] = NULL;
    }

    // Clean vfs structure
    if (vfs_pid) {
        freez(vfs_pid[pid]);
        vfs_pid[pid] = NULL;
    }

    // Clean fd structure
    if (fd_pid) {
        freez(fd_pid[pid]);
        fd_pid[pid] = NULL;
    }

    // Clean shm structure
    if (shm_pid) {
        freez(shm_pid[pid]);
        shm_pid[pid] = NULL;
    }
}

/**
 * Remove PIDs when they are not running more.
 */
void cleanup_exited_pids()
{
    struct pid_stat *p = NULL;

    for (p = root_of_pids; p;) {
        if (!p->updated && (!p->keep || p->keeploops > 0)) {
            if (unlikely(debug_enabled && (p->keep || p->keeploops)))
                debug_log(" > CLEANUP cannot keep exited process %d (%s) anymore - removing it.", p->pid, p->comm);

            pid_t r = p->pid;
            p = p->next;

            // Clean process structure
            freez(global_process_stats[r]);
            global_process_stats[r] = NULL;

            freez(current_apps_data[r]);
            current_apps_data[r] = NULL;

            cleanup_variables_from_other_threads(r);

            del_pid_entry(r);
        } else {
            if (unlikely(p->keep))
                p->keeploops++;
            p->keep = 0;
            p = p->next;
        }
    }
}

/**
 * Read proc filesystem for the first time.
 *
 * @return It returns 0 on success and -1 otherwise.
 */
static inline void read_proc_filesystem()
{
    char dirname[FILENAME_MAX + 1];

    snprintfz(dirname, FILENAME_MAX, "%s/proc", netdata_configured_host_prefix);
    DIR *dir = opendir(dirname);
    if (!dir)
        return;

    struct dirent *de = NULL;

    while ((de = readdir(dir))) {
        char *endptr = de->d_name;

        if (unlikely(de->d_type != DT_DIR || de->d_name[0] < '0' || de->d_name[0] > '9'))
            continue;

        pid_t pid = (pid_t)strtoul(de->d_name, &endptr, 10);

        // make sure we read a valid number
        if (unlikely(endptr == de->d_name || *endptr != '\0'))
            continue;

        collect_data_for_pid(pid, NULL);
    }
    closedir(dir);
}

/**
 * Aggregated PID on target
 *
 * @param w the target output
 * @param p the pid with information to update
 * @param o never used
 */
static inline void aggregate_pid_on_target(struct target *w, struct pid_stat *p, struct target *o)
{
    UNUSED(o);

    if (unlikely(!p->updated)) {
        // the process is not running
        return;
    }

    if (unlikely(!w)) {
        error("pid %d %s was left without a target!", p->pid, p->comm);
        return;
    }

    w->processes++;
    struct pid_on_target *pid_on_target = mallocz(sizeof(struct pid_on_target));
    pid_on_target->pid = p->pid;
    pid_on_target->next = w->root_pid;
    w->root_pid = pid_on_target;
}

/**
 * Collect data for all process
 *
 * Read data from hash table and store it in appropriate vectors.
 * It also creates the link between targets and PIDs.
 *
 * @param tbl_pid_stats_fd      The mapped file descriptor for the hash table.
 */
void collect_data_for_all_processes(int tbl_pid_stats_fd)
{
    struct pid_stat *pids = root_of_pids; // global list of all processes running
    while (pids) {
        if (pids->updated_twice) {
            pids->read = 0; // mark it as not read, so that collect_data_for_pid() will read it
            pids->updated = 0;
            pids->merged = 0;
            pids->children_count = 0;
            pids->parent = NULL;
        } else {
            if (pids->updated)
                pids->updated_twice = 1;
        }

        pids = pids->next;
    }

    read_proc_filesystem();

    uint32_t key;
    pids = root_of_pids; // global list of all processes running
    // while (bpf_map_get_next_key(tbl_pid_stats_fd, &key, &next_key) == 0) {
    while (pids) {
        key = pids->pid;
        ebpf_process_stat_t *w = global_process_stats[key];
        if (!w) {
            w = mallocz(sizeof(ebpf_process_stat_t));
            global_process_stats[key] = w;
        }

        if (bpf_map_lookup_elem(tbl_pid_stats_fd, &key, w)) {
            // Clean Process structures
            freez(w);
            global_process_stats[key] = NULL;

            freez(current_apps_data[key]);
            current_apps_data[key] = NULL;

            cleanup_variables_from_other_threads(key);

            pids = pids->next;
            continue;
        }

        pids = pids->next;
    }

    link_all_processes_to_their_parents();

    apply_apps_groups_targets_inheritance();

    apps_groups_targets_count = zero_all_targets(apps_groups_root_target);

    // this has to be done, before the cleanup
    // // concentrate everything on the targets
    for (pids = root_of_pids; pids; pids = pids->next)
        aggregate_pid_on_target(pids->target, pids, NULL);

    post_aggregate_targets(apps_groups_root_target);
}
