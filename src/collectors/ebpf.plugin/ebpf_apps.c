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
        netdata_log_error("Number of elements given is too small, adjusting it for %d", NETDATA_EBPF_ALLOC_MIN_ELEMENTS);
        max_elements = NETDATA_EBPF_ALLOC_MIN_ELEMENTS;
    }

    ebpf_aral_apps_pid_stat = ebpf_allocate_pid_aral("ebpf_pid_stat", sizeof(struct ebpf_pid_stat));

#ifdef NETDATA_DEV_MODE
    netdata_log_info("Plugin is using ARAL with values %d", NETDATA_EBPF_ALLOC_MAX_PID);
#endif
}

/**
 * eBPF pid stat get
 *
 * Get a ebpf_pid_stat entry to be used with a specific PID.
 *
 * @return it returns the address on success.
 */
struct ebpf_pid_stat *ebpf_pid_stat_get(void)
{
    struct ebpf_pid_stat *target = aral_mallocz(ebpf_aral_apps_pid_stat);
    memset(target, 0, sizeof(struct ebpf_pid_stat));
    return target;
}

/**
 * eBPF target release
 *
 * @param stat Release a target after usage.
 */
void ebpf_pid_stat_release(struct ebpf_pid_stat *stat)
{
    aral_freez(ebpf_aral_apps_pid_stat, stat);
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
struct ebpf_target *get_apps_groups_target(struct ebpf_target **agrt, const char *id, struct ebpf_target *target, const char *name)
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
            "Internal Error: request to link process '%s' to target '%s' which is linked to target '%s'", id,
            target->id, target->target->id);

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

struct ebpf_pid_stat **ebpf_all_pids = NULL;    // to avoid allocations, we pre-allocate the entire pid space.
struct ebpf_pid_stat *ebpf_vector_pids = NULL; //
struct ebpf_pid_stat *ebpf_root_of_pids = NULL; // global list of all processes running
ebpf_pid_data_t *ebpf_pids = NULL;

size_t ebpf_all_pids_count = 0; // the number of processes running

struct ebpf_target
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
static inline int managed_log(ebpf_pid_data_t *p, uint32_t log, int status)
{
    if (unlikely(!status)) {
        // netdata_log_error("command failed log %u, errno %d", log, errno);

        if (unlikely(debug_enabled || errno != ENOENT)) {
            if (unlikely(debug_enabled || !(p->log_thrown & log))) {
                p->log_thrown |= log;
                switch (log) {
                    case PID_LOG_IO:
                        netdata_log_error(
                            "Cannot process %s/proc/%u/io (command '%s')", netdata_configured_host_prefix, p->pid,
                            p->comm);
                        break;

                    case PID_LOG_STATUS:
                        netdata_log_error(
                            "Cannot process %s/proc/%u/status (command '%s')", netdata_configured_host_prefix, p->pid,
                            p->comm);
                        break;

                    case PID_LOG_CMDLINE:
                        netdata_log_error(
                            "Cannot process %s/proc/%u/cmdline (command '%s')", netdata_configured_host_prefix, p->pid,
                            p->comm);
                        break;

                    case PID_LOG_FDS:
                        netdata_log_error(
                            "Cannot process entries in %s/proc/%u/fd (command '%s')", netdata_configured_host_prefix,
                            p->pid, p->comm);
                        break;

                    case PID_LOG_STAT:
                        break;

                    default:
                        netdata_log_error("unhandled error for pid %u, command '%s'", p->pid, p->comm);
                        break;
                }
            }
        }
        errno_clear();
    } else if (unlikely(p->log_thrown & log)) {
        // netdata_log_error("unsetting log %u on pid %d", log, p->pid);
        p->log_thrown &= ~log;
    }

    return status;
}

/**
 * Get PID entry
 *
 * Get or allocate the PID entry for the specified pid.
 *
 * @param pid   the pid to search the data.
 * @param tgid  the task group id
 *
 * @return It returns the pid entry structure
 */
ebpf_pid_stat_t *ebpf_get_pid_entry(pid_t pid, pid_t tgid)
{
    struct ebpf_pid_stat *p;
    if (unlikely(!ebpf_vector_pids)) {
        ebpf_pid_stat_t *ptr = ebpf_all_pids[pid];
        if (unlikely(ptr)) {
            if (!ptr->ppid && tgid)
                ptr->ppid = tgid;
            return ebpf_all_pids[pid];
        }

        p = ebpf_pid_stat_get();
        ebpf_all_pids[pid] = p;
    } else {
        p = &ebpf_vector_pids[pid];

        if (p->pid == pid && p->ppid == tgid)
            return p;

        memset(p, 0, sizeof(*p));
    }

    if (likely(ebpf_root_of_pids))
        ebpf_root_of_pids->prev = p;

    p->next = ebpf_root_of_pids;
    ebpf_root_of_pids = p;

    p->pid = pid;
    p->ppid = tgid;

    ebpf_all_pids_count++;

    return p;
}

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
                 (!w->starts_with && w->ends_with && pclen >= w->comparelen && !strcmp(w->compare, &p->comm[pclen - w->comparelen])) ||
                 (proc_pid_cmdline_is_needed && w->starts_with && w->ends_with && p->cmdline && strstr(p->cmdline, w->compare))))) {
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

/**
 * Get PID and link
 *
 * @param pid  the current PID
 * @param tgid The parent PID
 * @param name The process name
 *
 * @return It returns the pid_stat already associated to a target.
 */
ebpf_pid_stat_t *ebpf_get_pid_and_link(pid_t pid, pid_t tgid, char *name)
{
    ebpf_pid_stat_t *pe = ebpf_get_pid_entry(pid, tgid);

    // mark it as updated
    pe->updated = 1;
    pe->keep = 0;
    pe->keeploops = 0;

    strncpyz(pe->comm, name, sizeof(pe->comm) -1);

    /*
    if (!pe->target)
        assign_target_to_pid(pe);
        */

    return pe;
}

/**
 * Release and Ulink PID stat
 *
 * Release the PID stat pid and also remove from apps group.
 *
 * @param eps a structure with data to test and update.
 * @param fd  the file descriptor where data is stored inside kernel ring.
 * @param key the key (pid value) of the hash table.
 * @param idx the thread index.
 */
void ebpf_release_and_unlink_pid_stat(ebpf_pid_stat_t *eps, int fd, uint32_t key, uint32_t idx)
{
    bpf_map_delete_elem(fd, &key);
    eps->not_updated = 0;
    eps->updated = 0;
    eps->thread_collecting &= ~(1<<idx);
    if (!eps->thread_collecting)
        ebpf_del_pid_entry((pid_t)key);
}

// ----------------------------------------------------------------------------
// update pids from proc

/**
 * Read cmd line from /proc/PID/cmdline
 *
 * @param p  the ebpf_pid_stat_structure.
 *
 * @return It returns 1 on success and 0 otherwise.
 */
static inline int read_proc_pid_cmdline(ebpf_pid_data_t *p)
{
    static char cmdline[MAX_CMDLINE + 1];
    char filename[FILENAME_MAX + 1];
    snprintfz(filename, FILENAME_MAX, "%s/proc/%d/cmdline", netdata_configured_host_prefix, p->pid);

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
    // copy the command to the command line
    if (p->cmdline)
        freez(p->cmdline);
    p->cmdline = strdupz(p->comm);

    if (collect_pids & EBPF_MODULE_SOCKET_IDX) {
        rw_spinlock_write_lock(&ebpf_judy_pid.index.rw_spinlock);
        netdata_ebpf_judy_pid_stats_t *pid_ptr = ebpf_get_pid_from_judy_unsafe(&ebpf_judy_pid.index.JudyLArray, p->pid);
        if (pid_ptr)
            pid_ptr->cmdline = p->cmdline;
        rw_spinlock_write_unlock(&ebpf_judy_pid.index.rw_spinlock);
    }

    return ret;
}

/**
 * Read information from /proc/PID/stat and /proc/PID/cmdline
 * Assign target to pid
 *
 * @param p the pid stat structure to store the data.
 * @param ptr an useless argument.
 */
static inline int read_proc_pid_stat(ebpf_pid_data_t *p, void *ptr)
{
    UNUSED(ptr);

    procfile *ff;

    char filename[FILENAME_MAX + 1];
    snprintfz(filename, FILENAME_MAX, "%s/proc/%d/stat", netdata_configured_host_prefix, p->pid);

    struct stat statbuf;
    if (stat(filename, &statbuf))
        return 0;

    ff = procfile_open(filename, NULL, PROCFILE_FLAG_NO_ERROR_ON_FILE_IO);
    if (unlikely(!ff))
        return 0;

    procfile_set_open_close(ff, "(", ")");

    ff = procfile_readall(ff);
    if (unlikely(!ff))
        return 0;

    char *comm = procfile_lineword(ff, 0, 1);
    p->ppid = (int32_t)str2pid_t(procfile_lineword(ff, 0, 3));

    read_proc_pid_cmdline(p);
    if (strcmp(p->comm, comm) != 0) {
        if (unlikely(debug_enabled)) {
            if (p->comm[0])
                debug_log("\tpid %d (%s) changed name to '%s'", p->pid, p->comm, comm);
            else
                debug_log("\tJust added %d (%s)", p->pid, comm);
        }

        strncpyz(p->comm, comm, EBPF_MAX_COMPARE_NAME);

        assign_target_to_pid(p);
    }

    if (unlikely(debug_enabled || (p->target && p->target->debug_enabled)))
        debug_log_int(
            "READ PROC/PID/STAT: %s/proc/%d/stat, process: '%s' on target '%s'",
            netdata_configured_host_prefix, p->pid, p->comm, (p->target) ? p->target->name : "UNSET");

    procfile_close(ff);

    return 1;
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
    void *ptr = NULL;
    if (unlikely(pid < 0 || pid > pid_max)) {
        netdata_log_error("Invalid pid %d read (expected %d to %d). Ignoring process.", pid, 0, pid_max);
        return 0;
    }

    ebpf_pid_data_t *p = ebpf_get_pid_data((uint32_t)pid, 0, NULL, 0);
    if (p->comm[0])
        return 1;

    if (unlikely(!managed_log(p, PID_LOG_STAT, read_proc_pid_stat(p, ptr))))
        // there is no reason to proceed if we cannot get its status
        return 0;

    // check its parent pid
    if (unlikely(p->ppid < 0 || p->ppid > pid_max)) {
        netdata_log_error("Pid %u (command '%s') states invalid parent pid %u. Using 0.", pid, p->comm, p->ppid);
        p->ppid = 0;
    }

    return 1;
}

/**
 * Fill link list of parents with children PIDs
 */
static inline void link_all_processes_to_their_parents(void)
{
    struct ebpf_pid_stat *p, *pp;

    // link all children to their parents
    // and update children count on parents
    for (p = ebpf_root_of_pids; p; p = p->next) {
        // for each process found

        p->sortlist = 0;
        p->parent = NULL;

        if (unlikely(!p->ppid)) {
            p->parent = NULL;
            continue;
        }

        pp = ebpf_all_pids[p->ppid];
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
    struct ebpf_pid_stat *p = NULL;

    // children that do not have a target
    // inherit their target from their parent
    int found = 1, loops = 0;
    while (found) {
        if (unlikely(debug_enabled))
            loops++;
        found = 0;
        for (p = ebpf_root_of_pids; p; p = p->next) {
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

        for (p = ebpf_root_of_pids; p; p = p->next) {
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
    if (ebpf_all_pids[INIT_PID])
        ebpf_all_pids[INIT_PID]->target = apps_groups_default_target;

    // pid 0 goes always to default target
    if (ebpf_all_pids[0])
        ebpf_all_pids[0]->target = apps_groups_default_target;

    // give a default target on all top level processes
    if (unlikely(debug_enabled))
        loops++;
    for (p = ebpf_root_of_pids; p; p = p->next) {
        // if the process is not merged itself
        // then is is a top level process
        if (unlikely(!p->merged && !p->target))
            p->target = apps_groups_default_target;

        // make sure all processes have a sortlist
        if (unlikely(!p->sortlist))
            p->sortlist = sortlist++;
    }

    if (ebpf_all_pids[1])
        ebpf_all_pids[1]->sortlist = sortlist++;

    // give a target to all merged child processes
    found = 1;
    while (found) {
        if (unlikely(debug_enabled))
            loops++;
        found = 0;
        for (p = ebpf_root_of_pids; p; p = p->next) {
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
    struct ebpf_pid_stat *p = (ebpf_all_pids) ? ebpf_all_pids[pid] : &ebpf_vector_pids[pid];

    if (unlikely(!p)) {
        netdata_log_error("attempted to free pid %d that is not allocated.", pid);
        return;
    }

    debug_log("process %d %s exited, deleting it.", pid, p->comm);

    if (ebpf_root_of_pids == p)
        ebpf_root_of_pids = p->next;

    if (p->next)
        p->next->prev = p->prev;
    if (p->prev)
        p->prev->next = p->next;

    freez(p->stat_filename);
    freez(p->status_filename);
    freez(p->io_filename);
    freez(p->cmdline_filename);

    rw_spinlock_write_lock(&ebpf_judy_pid.index.rw_spinlock);
    netdata_ebpf_judy_pid_stats_t *pid_ptr = ebpf_get_pid_from_judy_unsafe(&ebpf_judy_pid.index.JudyLArray, p->pid);
    if (pid_ptr) {
        if (pid_ptr->socket_stats.JudyLArray) {
            Word_t local_socket = 0;
            Pvoid_t *socket_value;
            bool first_socket = true;
            while ((socket_value = JudyLFirstThenNext(pid_ptr->socket_stats.JudyLArray, &local_socket, &first_socket))) {
                netdata_socket_plus_t *socket_clean = *socket_value;
                aral_freez(aral_socket_table, socket_clean);
            }
            JudyLFreeArray(&pid_ptr->socket_stats.JudyLArray, PJE0);
        }
        aral_freez(ebpf_judy_pid.pid_table, pid_ptr);
        JudyLDel(&ebpf_judy_pid.index.JudyLArray, p->pid, PJE0);
    }
    rw_spinlock_write_unlock(&ebpf_judy_pid.index.rw_spinlock);

    freez(p->cmdline);
    if (ebpf_all_pids) {
        ebpf_pid_stat_release(p);
        ebpf_all_pids[pid] = NULL;
    } else if (ebpf_vector_pids) {
        memset(&ebpf_vector_pids[pid], 0, sizeof(struct ebpf_pid_stat));
    }

    ebpf_all_pids_count--;
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
    struct ebpf_pid_stat *stat;

    stat = ebpf_all_pids[pid];
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
 * Remove PIDs when they are not running more.
 */
void ebpf_cleanup_exited_pids(int max)
{
    struct ebpf_pid_stat *p = NULL;

    for (p = ebpf_root_of_pids; p;) {
        if (p->not_updated >= max) {
            if (unlikely(debug_enabled && (p->keep || p->keeploops)))
                debug_log(" > CLEANUP cannot keep exited process %d (%s) anymore - removing it.", p->pid, p->comm);

            pid_t r = p->pid;
            p = p->next;

            ebpf_del_pid_entry(r);
        }
        p = p->next;
    }
}

/**
 * Read proc filesystem for the first time.
 *
 * @return It returns 0 on success and -1 otherwise.
 */
void ebpf_read_proc_filesystem()
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

        ebpf_collect_data_for_pid(pid);
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
static inline void aggregate_pid_on_target(struct ebpf_target *w, struct ebpf_pid_stat *p, struct ebpf_target *o)
{
    UNUSED(o);

    if (unlikely(!p->updated)) {
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
 * Process Accumulator
 *
 * Sum all values read from kernel and store in the first address.
 *
 * @param out the vector with read values.
 * @param maps_per_core do I need to read all cores?
 */
void ebpf_process_apps_accumulator(ebpf_process_stat_t *out, int maps_per_core)
{
    int i, end = (maps_per_core) ? ebpf_nprocs : 1;
    ebpf_process_stat_t *total = &out[0];
    for (i = 1; i < end; i++) {
        ebpf_process_stat_t *w = &out[i];
        total->exit_call += w->exit_call;
        total->task_err += w->task_err;
        total->create_thread += w->create_thread;
        total->create_process += w->create_process;
        total->release_call += w->release_call;
    }
}

/**
 * Sum values for pid
 *
 * @param structure to store result.
 * @param root the structure with all available PIDs
 */
void ebpf_process_sum_values_for_pids(ebpf_process_stat_t *process, struct ebpf_pid_on_target *root)
{
    memset(process, 0, sizeof(ebpf_process_stat_t));
    while (root) {
        int32_t pid = root->pid;
        ebpf_pid_stat_t *local_pid = ebpf_get_pid_entry(pid, 0);
        if (local_pid) {
            ebpf_process_stat_t *in = &local_pid->process;
            process->task_err += in->task_err;
            process->release_call += in->release_call;
            process->exit_call += in->exit_call;
            process->create_thread += in->create_thread;
            process->create_process += in->create_process;
        }

        root = root->next;
    }
}

/**
 * Collect data for all process
 *
 * Read data from hash table and store it in appropriate vectors.
 * It also creates the link between targets and PIDs.
 *
 * @param tbl_pid_stats_fd      The mapped file descriptor for the hash table.
 * @param maps_per_core         do I have hash maps per core?
 */
void collect_data_for_all_processes(int tbl_pid_stats_fd, int maps_per_core)
{
    if (unlikely(!ebpf_all_pids))
        return;

    struct ebpf_pid_stat *pids = ebpf_root_of_pids; // global list of all processes running
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

    ebpf_read_proc_filesystem();

    pids = ebpf_root_of_pids; // global list of all processes running

    if (tbl_pid_stats_fd != -1) {
        size_t length =  sizeof(ebpf_process_stat_t);
        if (maps_per_core)
            length *= ebpf_nprocs;

        uint32_t key = 0, next_key = 0;
        while (bpf_map_get_next_key(tbl_pid_stats_fd, &key, &next_key) == 0) {
            ebpf_pid_stat_t *local_pid = ebpf_get_pid_entry(key, 0);
            if (!local_pid)
                goto end_process_loop;

            ebpf_process_stat_t *w = &local_pid->process;
            if (bpf_map_lookup_elem(tbl_pid_stats_fd, &key, process_stat_vector)) {
                goto end_process_loop;
            }

            ebpf_process_apps_accumulator(process_stat_vector, maps_per_core);

            memcpy(w, process_stat_vector, sizeof(ebpf_process_stat_t));

end_process_loop:
            memset(process_stat_vector, 0, length);
            key = next_key;
        }
    }

    link_all_processes_to_their_parents();

    apply_apps_groups_targets_inheritance();

    apps_groups_targets_count = zero_all_targets(apps_groups_root_target);

    // this has to be done, before the cleanup
    // // concentrate everything on the targets
    for (pids = ebpf_root_of_pids; pids; pids = pids->next)
        aggregate_pid_on_target(pids->target, pids, NULL);

    post_aggregate_targets(apps_groups_root_target);

    struct ebpf_target *w;
    for (w = apps_groups_root_target; w; w = w->next) {
        if (unlikely(!(w->processes)))
            continue;

        ebpf_process_sum_values_for_pids(&w->process, w->root_pid);
    }
}
