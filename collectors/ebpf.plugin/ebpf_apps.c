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
struct ebpf_target *ebpf_get_apps_groups_target(struct ebpf_target **agrt, const char *id, struct ebpf_target *target, const char *name)
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

    w->pid_list.JudyLArray = NULL;
    rw_spinlock_init(&w->pid_list.rw_spinlock);

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
            struct ebpf_target *n = ebpf_get_apps_groups_target(agrt, s, w, name);
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

    *agdt = ebpf_get_apps_groups_target(agrt, "p+!o@w#e$i^r&7*5(-i)l-o_", NULL, "other"); // match nothing
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

struct ebpf_pid_stat **ebpf_all_pids = NULL;    // to avoid allocations, we pre-allocate the
                                      // the entire pid space.
struct ebpf_pid_stat *ebpf_root_of_pids = NULL; // global list of all processes running

size_t ebpf_all_pids_count = 0; // the number of processes running

struct ebpf_target
    *ebpf_apps_groups_default_target = NULL, // the default target
    *ebpf_apps_groups_root_target = NULL,    // apps_groups.conf defined
    *users_root_target = NULL,          // users
    *groups_root_target = NULL;         // user groups

size_t apps_groups_targets_count = 0; // # of apps_groups.conf targets

// ----------------------------------------------------------------------------
// internal counters

static size_t
    // global_iterations_counter = 1,
    // calls_counter = 0,
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
static inline int managed_log(struct ebpf_pid_stat *p, uint32_t log, int status)
{
    if (unlikely(!status)) {
        // netdata_log_error("command failed log %u, errno %d", log, errno);

        if (unlikely(debug_enabled || errno != ENOENT)) {
            if (unlikely(debug_enabled || !(p->log_thrown & log))) {
                p->log_thrown |= log;
                switch (log) {
                    case PID_LOG_IO:
                        netdata_log_error(
                            "Cannot process %s/proc/%d/io (command '%s')", netdata_configured_host_prefix, p->pid,
                            p->comm);
                        break;

                    case PID_LOG_STATUS:
                        netdata_log_error(
                            "Cannot process %s/proc/%d/status (command '%s')", netdata_configured_host_prefix, p->pid,
                            p->comm);
                        break;

                    case PID_LOG_CMDLINE:
                        netdata_log_error(
                            "Cannot process %s/proc/%d/cmdline (command '%s')", netdata_configured_host_prefix, p->pid,
                            p->comm);
                        break;

                    case PID_LOG_FDS:
                        netdata_log_error(
                            "Cannot process entries in %s/proc/%d/fd (command '%s')", netdata_configured_host_prefix,
                            p->pid, p->comm);
                        break;

                    case PID_LOG_STAT:
                        break;

                    default:
                        netdata_log_error("unhandled error for pid %d, command '%s'", p->pid, p->comm);
                        break;
                }
            }
        }
        errno = 0;
    } else if (unlikely(p->log_thrown & log)) {
        // netdata_log_error("unsetting log %u on pid %d", log, p->pid);
        p->log_thrown &= ~log;
    }

    return status;
}

/**
 * Select target
 *
 * @param name   the process name from kernel ring.
 * @param length the name size.
 * @param hash   the calculated hash for the name.
 * @param pid    the pid value.
 */
struct ebpf_target *ebpf_select_target(char *name, uint32_t length, uint32_t hash, uint32_t pid)
{
    targets_assignment_counter++;

    if (!length)
        goto ret_default_target;

    struct ebpf_target *w;
    for (w = ebpf_apps_groups_root_target; w; w = w->next) {
        if (unlikely(
                ((!w->starts_with && !w->ends_with && w->comparehash == hash && !strcmp(w->compare, name)) ||
                 (w->starts_with && !w->ends_with && !strncmp(w->compare, name, w->comparelen)) ||
                 (!w->starts_with && w->ends_with && length >= w->comparelen &&
                  !strcmp(w->compare, &name[length - w->comparelen]))))) {
            return w;
        }
    }

    if (!proc_pid_cmdline_is_needed)
        goto ret_default_target;

    static char cmdline[MAX_CMDLINE + 1];
    char filename[FILENAME_MAX + 1];
    snprintfz(filename, FILENAME_MAX, "%s/proc/%u/cmdline", netdata_configured_host_prefix, pid);
    int fd = open(filename, procfile_open_flags, 0666);
    if (unlikely(fd == -1))
        goto ret_default_target;

    ssize_t i, bytes = read(fd, cmdline, MAX_CMDLINE);
    close(fd);

    if (bytes < 0)
        goto ret_default_target;

    cmdline[bytes] = '\0';
    for (i = 0; i < bytes; i++) {
        if (unlikely(!cmdline[i]))
            cmdline[i] = ' ';
    }

    hash = simple_hash(cmdline);
    size_t pclen = strlen(cmdline);

    for (w = ebpf_apps_groups_root_target; w; w = w->next) {
        if (unlikely(
            ((!w->starts_with && !w->ends_with && w->comparehash == hash && !strcmp(w->compare, cmdline)) ||
             (w->starts_with && !w->ends_with && !strncmp(w->compare, cmdline, w->comparelen)) ||
             (!w->starts_with && w->ends_with && pclen >= w->comparelen && !strcmp(w->compare, &cmdline[pclen - w->comparelen])) ||
             (proc_pid_cmdline_is_needed && w->starts_with && w->ends_with && strstr(cmdline, w->compare))))) {
            return w;
        }
    }

ret_default_target:
    return ebpf_apps_groups_default_target;
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
static inline int read_proc_pid_cmdline(struct ebpf_pid_stat *p)
{
    static char cmdline[MAX_CMDLINE + 1];

    int ret = 0;
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

    debug_log("Read file '%s' contents: %s", p->cmdline_filename, p->cmdline);

    ret = 1;

cleanup:
    // copy the command to the command line
    if (p->cmdline)
        freez(p->cmdline);
    p->cmdline = strdupz(p->comm);

    rw_spinlock_write_lock(&ebpf_judy_pid.index.rw_spinlock);
    // This code exists to create an entry while the PR is developed.
    netdata_ebpf_judy_pid_stats_t *pid_ptr = ebpf_get_pid_from_judy_unsafe(&ebpf_judy_pid.index.JudyLArray,
                                                                           p->pid,
                                                                           p->cmdline,
                                                                           NULL);
    (void)pid_ptr;
    rw_spinlock_write_unlock(&ebpf_judy_pid.index.rw_spinlock);

    return ret;
}
