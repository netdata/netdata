// SPDX-License-Identifier: GPL-3.0-or-later

#include "ebpf.h"
#include "ebpf_socket.h"
#include "ebpf_apps.h"

// ----------------------------------------------------------------------------
// ARAL vectors used to speed up processing
ARAL *ebpf_aral_apps_pid_stat = NULL;

/**
 * eBPF ARAL Init
 *
 * Initiallize array allocator that will be used when integration with apps and ebpf is created.
 */
void ebpf_aral_init(void)
{
    size_t max_elements = NETDATA_EBPF_ALLOC_MAX_PID;
    if (max_elements < NETDATA_EBPF_ALLOC_MIN_ELEMENTS) {
        netdata_log_error(
            "Number of elements given is too small, adjusting it for %d", NETDATA_EBPF_ALLOC_MIN_ELEMENTS);
        max_elements = NETDATA_EBPF_ALLOC_MIN_ELEMENTS;
    }

#ifdef NETDATA_DEV_MODE
    netdata_log_info("Plugin is using ARAL with values %d", NETDATA_EBPF_ALLOC_MAX_PID);
#endif
}

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

/*****************************************************************
 *
 *  FUNCTIONS CALLED FROM COLLECTORS
 *
 *****************************************************************/

/**
 * Reset the target values
 *
 * @param root the pointer to the chain that will be reset.
 *
 * @return it returns the number of structures that was reset.
 */
size_t zero_all_targets(struct ebpf_target *root)
{
    struct ebpf_target *w;
    size_t count = 0;

    for (w = root; w; w = w->next) {
        count++;

        if (unlikely(w->root_pid)) {
            struct ebpf_pid_on_target *pid_on_target = w->root_pid;

            while (pid_on_target) {
                struct ebpf_pid_on_target *pid_on_target_to_free = pid_on_target;
                pid_on_target = pid_on_target->next;
                freez(pid_on_target_to_free);
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
void clean_apps_groups_target(struct ebpf_target *agrt)
{
    struct ebpf_target *current_target;
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
struct ebpf_target *
get_apps_groups_target(struct ebpf_target **agrt, const char *id, struct ebpf_target *target, const char *name)
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
    struct ebpf_target *w, *last = *agrt;
    for (w = *agrt; w; w = w->next) {
        if (w->idhash == hash && strncmp(nid, w->id, EBPF_MAX_NAME) == 0)
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
            "Internal Error: request to link process '%s' to target '%s' which is linked to target '%s'",
            id,
            target->id,
            target->target->id);

    w = callocz(1, sizeof(struct ebpf_target));
    strncpyz(w->id, nid, EBPF_MAX_NAME);
    w->idhash = simple_hash(w->id);

    if (unlikely(!target))
        // copy the name
        strncpyz(w->name, name, EBPF_MAX_NAME);
    else
        // copy the id
        strncpyz(w->name, nid, EBPF_MAX_NAME);

    strncpyz(w->clean_name, w->name, EBPF_MAX_NAME);
    netdata_fix_chart_name(w->clean_name);
    for (char *d = w->clean_name; *d; d++) {
        if (*d == '.')
            *d = '_';
    }

    strncpyz(w->compare, nid, EBPF_MAX_COMPARE_NAME);
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
int ebpf_read_apps_groups_conf(struct ebpf_target **agdt, struct ebpf_target **agrt, const char *path, const char *file)
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
        struct ebpf_target *w = NULL;

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
            struct ebpf_target *n = get_apps_groups_target(agrt, s, w, name);
            if (!n) {
                netdata_log_error("Cannot create target '%s' (line %zu, word %zu)", s, line, word);
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

    struct ebpf_target *ptr = *agdt;
    if (ptr->target)
        *agdt = ptr->target;

    return 0;
}

// the minimum PID of the system
// this is also the pid of the init process
#define INIT_PID 1

// ----------------------------------------------------------------------------
// string lengths

#define MAX_CMDLINE 16384

Pvoid_t ebpf_pid_judyL = NULL;
SPINLOCK ebpf_pid_spinlock = SPINLOCK_INITIALIZER;

void ebpf_pid_del(pid_t pid)
{
    spinlock_lock(&ebpf_pid_spinlock);
    (void)JudyLDel(&ebpf_pid_judyL, (Word_t)pid, PJE0);
    spinlock_unlock(&ebpf_pid_spinlock);
}

static ebpf_pid_data_t *ebpf_find_pid_data_unsafe(pid_t pid)
{
    ebpf_pid_data_t *pid_data = NULL;
    Pvoid_t *Pvalue = JudyLGet(ebpf_pid_judyL, (Word_t)pid, PJE0);
    if (Pvalue)
        pid_data = *Pvalue;
    return pid_data;
}

ebpf_pid_data_t *ebpf_find_pid_data(pid_t pid)
{
    spinlock_lock(&ebpf_pid_spinlock);
    ebpf_pid_data_t *pid_data = ebpf_find_pid_data_unsafe(pid);
    spinlock_unlock(&ebpf_pid_spinlock);
    return pid_data;
}

ebpf_pid_data_t *ebpf_find_or_create_pid_data(pid_t pid)
{
    spinlock_lock(&ebpf_pid_spinlock);
    ebpf_pid_data_t *pid_data = ebpf_find_pid_data_unsafe(pid);
    if (!pid_data) {
        Pvoid_t *Pvalue = JudyLIns(&ebpf_pid_judyL, (Word_t)pid, PJE0);
        internal_fatal(!Pvalue || Pvalue == PJERR, "EBPF: pid judy array");
        if (likely(!*Pvalue))
            *Pvalue = pid_data = callocz(1, sizeof(*pid_data));
        else
            pid_data = *Pvalue;
    }
    spinlock_unlock(&ebpf_pid_spinlock);

    return pid_data;
}

//ebpf_pid_data_t *ebpf_pids = NULL;            // to avoid allocations, we pre-allocate the entire pid space.
ebpf_pid_data_t *ebpf_pids_link_list = NULL; // global list of all processes running

size_t ebpf_all_pids_count = 0;        // the number of processes running read from /proc
size_t ebpf_hash_table_pids_count = 0; // the number of tasks in our hash tables

struct ebpf_target *apps_groups_default_target = NULL, // the default target
    *apps_groups_root_target = NULL,                   // apps_groups.conf defined
        *users_root_target = NULL,                     // users
            *groups_root_target = NULL;                // user groups

size_t apps_groups_targets_count = 0; // # of apps_groups.conf targets

int pids_fd[NETDATA_EBPF_PIDS_END_IDX];

// ----------------------------------------------------------------------------
// internal counters

static size_t
    // global_iterations_counter = 1,
    //calls_counter = 0,
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
 * Assign the PID to a target.
 *
 * @param p the pid_stat structure to assign for a target.
 */
static inline void assign_target_to_pid(ebpf_pid_data_t *p)
{
    targets_assignment_counter++;

    uint32_t hash = simple_hash(p->comm);
    size_t pclen = strlen(p->comm);

    struct ebpf_target *w;
    bool assigned = false;
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
                 (!w->starts_with && w->ends_with && pclen >= w->comparelen &&
                  !strcmp(w->compare, &p->comm[pclen - w->comparelen])) ||
                 (proc_pid_cmdline_is_needed && w->starts_with && w->ends_with && p->cmdline &&
                  strstr(p->cmdline, w->compare))))) {
            if (w->target)
                p->target = w->target;
            else
                p->target = w;

            if (debug_enabled || (p->target && p->target->debug_enabled))
                debug_log_int("%s linked to target %s", p->comm, p->target->name);

            w->processes++;
            assigned = true;

            break;
        }
    }

    if (!assigned) {
        apps_groups_default_target->processes++;
        p->target = apps_groups_default_target;
    }
}

// ----------------------------------------------------------------------------
// update pids from proc

/**
 * Read cmd line from /proc/PID/cmdline
 *
 * @param p  the ebpf_pid_data structure.
 *
 * @return It returns 1 on success and 0 otherwise.
 */
static inline int read_proc_pid_cmdline(ebpf_pid_data_t *p, char *cmdline)
{
    char filename[FILENAME_MAX + 1];
    snprintfz(filename, FILENAME_MAX, "%s/proc/%u/cmdline", netdata_configured_host_prefix, p->pid);

    int ret = 0;

    int fd = open(filename, procfile_open_flags, 0666);
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

    debug_log("Read file '%s' contents: %s", filename, p->cmdline);

    ret = 1;

cleanup:
    p->cmdline[0] = '\0';

    return ret;
}

/**
 * Read information from /proc/PID/stat and /proc/PID/cmdline
 * Assign target to pid
 *
 * @param p the pid stat structure to store the data.
 */
static inline int read_proc_pid_stat(ebpf_pid_data_t *p)
{
    procfile *ff;

    char filename[FILENAME_MAX + 1];
    int ret = 0;
    snprintfz(filename, FILENAME_MAX, "%s/proc/%u/stat", netdata_configured_host_prefix, p->pid);

    struct stat statbuf;
    if (stat(filename, &statbuf)) {
        // PID ended before we stat the file
        p->has_proc_file = 0;
        return 0;
    }

    ff = procfile_open(filename, NULL, PROCFILE_FLAG_NO_ERROR_ON_FILE_IO);
    if (unlikely(!ff))
        goto cleanup_pid_stat;

    procfile_set_open_close(ff, "(", ")");

    ff = procfile_readall(ff);
    if (unlikely(!ff))
        goto cleanup_pid_stat;

    char *comm = procfile_lineword(ff, 0, 1);
    int32_t ppid = (int32_t)str2pid_t(procfile_lineword(ff, 0, 3));

    if (p->ppid == (uint32_t)ppid && p->target)
        goto without_cmdline_target;

    p->ppid = ppid;

    char cmdline[MAX_CMDLINE + 1];
    p->cmdline = cmdline;
    read_proc_pid_cmdline(p, cmdline);
    if (strcmp(p->comm, comm) != 0) {
        if (unlikely(debug_enabled)) {
            if (p->comm[0])
                debug_log("\tpid %d (%s) changed name to '%s'", p->pid, p->comm, comm);
            else
                debug_log("\tJust added %d (%s)", p->pid, comm);
        }

        strncpyz(p->comm, comm, EBPF_MAX_COMPARE_NAME);
    }
    if (!p->target)
        assign_target_to_pid(p);

    p->cmdline = NULL;

    if (unlikely(debug_enabled || (p->target && p->target->debug_enabled)))
        debug_log_int(
            "READ PROC/PID/STAT: %s/proc/%d/stat, process: '%s' on target '%s'",
            netdata_configured_host_prefix,
            p->pid,
            p->comm,
            (p->target) ? p->target->name : "UNSET");

without_cmdline_target:
    p->has_proc_file = 1;
    p->not_updated = 0;
    ret = 1;
cleanup_pid_stat:
    procfile_close(ff);

    return ret;
}

/**
 * Collect data for PID
 *
 * @param pid the current pid that we are working
 *
 * @return It returns 1 on success and 0 otherwise
 */
static inline int ebpf_collect_data_for_pid(pid_t pid)
{
    if (unlikely(pid < 0 || pid > pid_max)) {
        netdata_log_error("Invalid pid %d read (expected %d to %d). Ignoring process.", pid, 0, pid_max);
        return 0;
    }

    ebpf_pid_data_t *p = ebpf_get_pid_data((uint32_t)pid, 0, NULL, NETDATA_EBPF_PIDS_PROC_FILE);
    read_proc_pid_stat(p);

    // check its parent pid
    if (unlikely(p->ppid > (uint32_t)pid_max)) {
        netdata_log_error("Pid %d (command '%s') states invalid parent pid %u. Using 0.", pid, p->comm, p->ppid);
        p->ppid = 0;
    }

    return 1;
}

/**
 * Fill link list of parents with children PIDs
 */
static inline void link_all_processes_to_their_parents(void)
{
    ebpf_pid_data_t *p, *pp;

    // link all children to their parents
    // and update children count on parents
    for (p = ebpf_pids_link_list; p; p = p->next) {
        // for each process found

        p->parent = NULL;

        if (unlikely(!p->ppid)) {
            p->parent = NULL;
            continue;
        }

        //        pp = &ebpf_pids[p->ppid];
        pp = ebpf_find_pid_data(p->ppid);
        if (likely(pp && pp->pid)) {
            p->parent = pp;
            pp->children_count++;

            if (unlikely(debug_enabled || (p->target && p->target->debug_enabled)))
                debug_log_int(
                    "child %d (%s) on target '%s' has parent %d (%s).",
                    p->pid,
                    p->comm,
                    (p->target) ? p->target->name : "UNSET",
                    pp->pid,
                    pp->comm);
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
    struct ebpf_pid_data *p = NULL;

    // children that do not have a target
    // inherit their target from their parent
    int found = 1, loops = 0;
    while (found) {
        if (unlikely(debug_enabled))
            loops++;
        found = 0;
        for (p = ebpf_pids_link_list; p; p = p->next) {
            // if this process does not have a target
            // and it has a parent
            // and its parent has a target
            // then, set the parent's target to this process
            if (unlikely(!p->target && p->parent && p->parent->target)) {
                p->target = p->parent->target;
                found++;

                if (debug_enabled || (p->target && p->target->debug_enabled))
                    debug_log_int(
                        "TARGET INHERITANCE: %s is inherited by %u (%s) from its parent %d (%s).",
                        p->target->name,
                        p->pid,
                        p->comm,
                        p->parent->pid,
                        p->parent->comm);
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

        for (p = ebpf_pids_link_list; p; p = p->next) {
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
                            "TARGET INHERITANCE: %s is inherited by %d (%s) from its child %d (%s).",
                            p->target->name,
                            p->parent->pid,
                            p->parent->comm,
                            p->pid,
                            p->comm);
                }

                found++;
            }
        }

        debug_log("TARGET INHERITANCE: merged %d processes", found);
    }

    // init goes always to default target
    ebpf_pid_data_t *pid_entry = ebpf_find_or_create_pid_data(INIT_PID);
    pid_entry->target = apps_groups_default_target;
    //    ebpf_pids[INIT_PID].target = apps_groups_default_target;

    // pid 0 goes always to default target
    pid_entry = ebpf_find_or_create_pid_data(0);
    pid_entry->target = apps_groups_default_target;
    //ebpf_pids[0].target = apps_groups_default_target;

    // give a default target on all top level processes
    if (unlikely(debug_enabled))
        loops++;
    for (p = ebpf_pids_link_list; p; p = p->next) {
        // if the process is not merged itself
        // then is is a top level process
        if (unlikely(!p->merged && !p->target))
            p->target = apps_groups_default_target;

        // make sure all processes have a sortlist
        if (unlikely(!p->sortlist))
            p->sortlist = sortlist++;
    }

    //ebpf_pids[1].sortlist = sortlist++;
    pid_entry = ebpf_find_or_create_pid_data(1);
    pid_entry->sortlist = sortlist++;

    // give a target to all merged child processes
    found = 1;
    while (found) {
        if (unlikely(debug_enabled))
            loops++;
        found = 0;
        for (p = ebpf_pids_link_list; p; p = p->next) {
            if (unlikely(!p->target && p->merged && p->parent && p->parent->target)) {
                p->target = p->parent->target;
                found++;

                if (debug_enabled || (p->target && p->target->debug_enabled))
                    debug_log_int(
                        "TARGET INHERITANCE: %s is inherited by %d (%s) from its parent %d (%s) at phase 2.",
                        p->target->name,
                        p->pid,
                        p->comm,
                        p->parent->pid,
                        p->parent->comm);
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
static inline void post_aggregate_targets(struct ebpf_target *root)
{
    struct ebpf_target *w;
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
void ebpf_del_pid_entry(pid_t pid)
{
    //ebpf_pid_data_t *p = &ebpf_pids[pid];
    ebpf_pid_data_t *p = ebpf_find_pid_data(pid);

    debug_log("process %d %s exited, deleting it.", pid, p->comm);

    if (ebpf_pids_link_list == p)
        ebpf_pids_link_list = p->next;

    if (p->next)
        p->next->prev = p->prev;
    if (p->prev)
        p->prev->next = p->next;

    if ((p->thread_collecting & NETDATA_EBPF_PIDS_PROC_FILE) || p->has_proc_file)
        ebpf_all_pids_count--;

    rw_spinlock_write_lock(&ebpf_judy_pid.index.rw_spinlock);
    netdata_ebpf_judy_pid_stats_t *pid_ptr = ebpf_get_pid_from_judy_unsafe(&ebpf_judy_pid.index.JudyLArray, p->pid);
    if (pid_ptr) {
        if (pid_ptr->socket_stats.JudyLArray) {
            Word_t local_socket = 0;
            Pvoid_t *socket_value;
            bool first_socket = true;
            while (
                (socket_value = JudyLFirstThenNext(pid_ptr->socket_stats.JudyLArray, &local_socket, &first_socket))) {
                netdata_socket_plus_t *socket_clean = *socket_value;
                aral_freez(aral_socket_table, socket_clean);
            }
            JudyLFreeArray(&pid_ptr->socket_stats.JudyLArray, PJE0);
        }
        aral_freez(ebpf_judy_pid.pid_table, pid_ptr);
        JudyLDel(&ebpf_judy_pid.index.JudyLArray, p->pid, PJE0);
    }
    rw_spinlock_write_unlock(&ebpf_judy_pid.index.rw_spinlock);

    freez(p);
    //memset(p, 0, sizeof(ebpf_pid_data_t));
    ebpf_pid_del(pid);
}

/**
 * Remove PIDs when they are not running more.
 */
static void ebpf_cleanup_exited_pids()
{
    ebpf_pid_data_t *p = NULL;
    for (p = ebpf_pids_link_list; p; p = p->next) {
        if (!p->has_proc_file) {
            ebpf_reset_specific_pid_data(p);
        }
    }
}

/**
 * Read proc filesystem for the first time.
 *
 * @return It returns 0 on success and -1 otherwise.
 */
static int ebpf_read_proc_filesystem()
{
    char dirname[FILENAME_MAX + 1];

    snprintfz(dirname, FILENAME_MAX, "%s/proc", netdata_configured_host_prefix);
    DIR *dir = opendir(dirname);
    if (!dir)
        return -1;

    struct dirent *de = NULL;

    while ((de = readdir(dir))) {
        char *endptr = de->d_name;

        if (unlikely(de->d_type != DT_DIR || de->d_name[0] < '0' || de->d_name[0] > '9'))
            continue;

        pid_t pid = (pid_t)strtoul(de->d_name, &endptr, 10);

        // make sure we read a valid number
        if (unlikely(endptr == de->d_name || *endptr != '\0'))
            continue;

        ebpf_collect_data_for_pid(pid);
    }
    closedir(dir);

    return 0;
}

/**
 * Aggregated PID on target
 *
 * @param w the target output
 * @param p the pid with information to update
 * @param o never used
 */
static inline void aggregate_pid_on_target(struct ebpf_target *w, ebpf_pid_data_t *p, struct ebpf_target *o)
{
    UNUSED(o);

    if (unlikely(!p->has_proc_file)) {
        // the process is not running
        return;
    }

    if (unlikely(!w)) {
        netdata_log_error("pid %u %s was left without a target!", p->pid, p->comm);
        return;
    }

    w->processes++;
    struct ebpf_pid_on_target *pid_on_target = mallocz(sizeof(struct ebpf_pid_on_target));
    pid_on_target->pid = p->pid;
    pid_on_target->next = w->root_pid;
    w->root_pid = pid_on_target;
}

/**
 *
 */
void ebpf_parse_proc_files()
{
    ebpf_pid_data_t *pids;
    for (pids = ebpf_pids_link_list; pids;) {
        if (kill(pids->pid, 0)) { // No PID found
            ebpf_pid_data_t *next = pids->next;
            ebpf_reset_specific_pid_data(pids);
            pids = next;
            continue;
        }

        pids->not_updated = EBPF_CLEANUP_FACTOR;
        pids->merged = 0;
        pids->children_count = 0;
        pids = pids->next;
    }

    if (ebpf_read_proc_filesystem())
        return;

    link_all_processes_to_their_parents();

    apply_apps_groups_targets_inheritance();

    apps_groups_targets_count = zero_all_targets(apps_groups_root_target);

    for (pids = ebpf_pids_link_list; pids; pids = pids->next)
        aggregate_pid_on_target(pids->target, pids, NULL);

    ebpf_cleanup_exited_pids();
}
