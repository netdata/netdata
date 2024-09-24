// SPDX-License-Identifier: GPL-3.0-or-later

#include "apps_plugin.h"

#define SIMPLE_HASHTABLE_NAME _PID
#define SIMPLE_HASHTABLE_VALUE_TYPE struct pid_stat
#define SIMPLE_HASHTABLE_KEY_TYPE int32_t
#define SIMPLE_HASHTABLE_VALUE2KEY_FUNCTION pid_stat_to_pid_ptr
#define SIMPLE_HASHTABLE_COMPARE_KEYS_FUNCTION pid_ptr_eq
#define SIMPLE_HASHTABLE_SAMPLE_IMPLEMENTATION 0
#include "libnetdata/simple_hashtable.h"

static inline int32_t *pid_stat_to_pid_ptr(struct pid_stat *p) {
    return &p->pid;
}

static inline bool pid_ptr_eq(int32_t *a, int32_t *b) {
    return *a == *b;
}

struct {
#if (ALL_PIDS_ARE_READ_INSTANTLY == 0)
    // Another pre-allocated list of all possible pids.
    // We need it to assign them a unique sortlist id, so that we
    // read parents before children. This is needed to prevent a situation where
    // a child is found running, but until we read its parent, it has exited and
    // its parent has accumulated its resources.
    struct {
        size_t size;
        struct pid_stat **array;
    } sorted;
#endif

    struct {
        size_t count;               // the number of processes running
        struct pid_stat *root;
        SIMPLE_HASHTABLE_PID ht;
        ARAL *aral;
    } all_pids;
} pids = { 0 };

struct pid_stat *root_of_pids(void) {
    return pids.all_pids.root;
}

size_t all_pids_count(void) {
    return pids.all_pids.count;
}

void pids_init(void) {
    pids.all_pids.aral = aral_create("pid_stat", sizeof(struct pid_stat), 1, 65536, NULL, NULL, NULL, false, true);
    simple_hashtable_init_PID(&pids.all_pids.ht, 1024);
}

static inline uint64_t pid_hash(pid_t pid) {
    return ((uint64_t)pid << 31) + (uint64_t)pid; // we remove 1 bit when shifting to make it different
}

inline struct pid_stat *find_pid_entry(pid_t pid) {
    if(pid <= 0) return NULL;

    uint64_t hash = pid_hash(pid);
    int32_t key = pid;
    SIMPLE_HASHTABLE_SLOT_PID *sl = simple_hashtable_get_slot_PID(&pids.all_pids.ht, hash, &key, true);
    return(SIMPLE_HASHTABLE_SLOT_DATA(sl));
}

struct pid_stat *get_or_allocate_pid_entry(pid_t pid) {
    uint64_t hash = pid_hash(pid);
    int32_t key = pid;
    SIMPLE_HASHTABLE_SLOT_PID *sl = simple_hashtable_get_slot_PID(&pids.all_pids.ht, hash, &key, true);
    struct pid_stat *p = SIMPLE_HASHTABLE_SLOT_DATA(sl);
    if(likely(p))
        return p;

    p = aral_callocz(pids.all_pids.aral);
    p->fds = mallocz(sizeof(struct pid_fd) * MAX_SPARE_FDS);
    p->fds_size = MAX_SPARE_FDS;
    init_pid_fds(p, 0, p->fds_size);
    p->pid = pid;
    p->values[PDF_PROCESSES] = 1;

    DOUBLE_LINKED_LIST_APPEND_ITEM_UNSAFE(pids.all_pids.root, p, prev, next);
    simple_hashtable_set_slot_PID(&pids.all_pids.ht, sl, hash, p);
    pids.all_pids.count++;

    return p;
}

void del_pid_entry(pid_t pid) {
    uint64_t hash = pid_hash(pid);
    int32_t key = pid;
    SIMPLE_HASHTABLE_SLOT_PID *sl = simple_hashtable_get_slot_PID(&pids.all_pids.ht, hash, &key, true);
    struct pid_stat *p = SIMPLE_HASHTABLE_SLOT_DATA(sl);

    if(unlikely(!p)) {
        netdata_log_error("attempted to free pid %d that is not allocated.", pid);
        return;
    }

    debug_log("process %d %s exited, deleting it.", pid, pid_stat_comm(p));

    DOUBLE_LINKED_LIST_REMOVE_ITEM_UNSAFE(pids.all_pids.root, p, prev, next);
    simple_hashtable_del_slot_PID(&pids.all_pids.ht, sl);

#if defined(OS_LINUX)
    {
        size_t i;
        for(i = 0; i < p->fds_size; i++)
            if(p->fds[i].filename)
                freez(p->fds[i].filename);
    }
    arl_free(p->status_arl);
#endif

    freez(p->fds);
    freez(p->fds_dirname);
    freez(p->stat_filename);
    freez(p->status_filename);
    freez(p->limits_filename);
    freez(p->io_filename);
    freez(p->cmdline_filename);
    string_freez(p->comm);
    string_freez(p->cmdline);
    aral_freez(pids.all_pids.aral, p);

    pids.all_pids.count--;
}

int collect_data_for_pid_stat(struct pid_stat *p, void *ptr) {
    if(unlikely(p->read)) return 0;
    p->read = true;

    // --------------------------------------------------------------------
    // /proc/<pid>/stat

    if(unlikely(!managed_log(p, PID_LOG_STAT, read_proc_pid_stat(p, ptr))))
        // there is no reason to proceed if we cannot get its status
        return 0;

    // check its parent pid
    if(unlikely(p->ppid < 0 || p->ppid > pid_max)) {
        netdata_log_error("Pid %d (command '%s') states invalid parent pid %d. Using 0.", p->pid, pid_stat_comm(p), p->ppid);
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

#if defined(NETDATA_INTERNAL_CHECKS) && (ALL_PIDS_ARE_READ_INSTANTLY == 0)
    struct pid_stat *pp = p->parent;
    if(unlikely(include_exited_childs && pp && !pp->read))
        nd_log(NDLS_COLLECTORS, NDLP_WARNING,
               "Read process %d (%s) sortlisted %"PRIu32", but its parent %d (%s) sortlisted %"PRIu32", is not read",
               p->pid, pid_stat_comm(p), p->sortlist, pp->pid, pid_stat_comm(pp), pp->sortlist);
#endif

    // mark it as updated
    p->updated = true;
    p->keep = false;
    p->keeploops = 0;

    return 1;
}

int collect_data_for_pid(pid_t pid, void *ptr) {
    if(unlikely(pid < 0 || pid > pid_max)) {
        netdata_log_error("Invalid pid %d read (expected %d to %d). Ignoring process.", pid, 0, pid_max);
        return 0;
    }

    struct pid_stat *p = get_or_allocate_pid_entry(pid);
    if(unlikely(!p)) return 0;

    return collect_data_for_pid_stat(p, ptr);
}

#if (ALL_PIDS_ARE_READ_INSTANTLY == 0)
static inline size_t compute_new_sorted_size(size_t old_size, size_t required_size) {
    size_t size = (required_size % 1024 == 0) ? required_size : required_size + 1024;
    size = (size / 1024) * 1024;

    if(size < old_size * 2)
        size = old_size * 2;

    return size;
}

static int compar_pid_sortlist(const void *a, const void *b) {
    const struct pid_stat *p1 = *(struct pid_stat **)a;
    const struct pid_stat *p2 = *(struct pid_stat **)b;

    if(p1->sortlist > p2->sortlist)
        return -1;
    else
        return 1;
}

bool collect_parents_before_children(void) {
    if (!pids.all_pids.count) return false;

    if (pids.all_pids.count > pids.sorted.size) {
        size_t new_size = compute_new_sorted_size(pids.sorted.size, pids.all_pids.count);
        freez(pids.sorted.array);
        pids.sorted.array = mallocz(new_size * sizeof(struct pid_stat *));
        pids.sorted.size = new_size;
    }

    size_t slc = 0;
    struct pid_stat *p = NULL;
    uint32_t sortlist = 1;
    for (p = root_of_pids(); p && slc < pids.sorted.size; p = p->next) {
        mark_pid_as_unread(p);
        pids.sorted.array[slc++] = p;

        // assign a sortlist id to all it and its parents
        for (struct pid_stat *pp = p; pp; pp = pp->parent) {
            pp->sortlist = sortlist++;
            if (pp->ppid && !pp->parent)
                pp->parent = find_pid_entry(pp->ppid);
        }
    }
    size_t sorted = slc;

    static bool logged = false;
    if (unlikely(p && !logged)) {
        nd_log(
            NDLS_COLLECTORS,
            NDLP_ERR,
            "Internal error: I was thinking I had %zu processes in my arrays, but it seems there are more.",
            pids.all_pids.count);
        logged = true;
    }

    if (include_exited_childs && sorted) {
        // Read parents before childs
        // This is needed to prevent a situation where
        // a child is found running, but until we read
        // its parent, it has exited and its parent
        // has accumulated its resources.

        qsort((void *)pids.sorted.array, sorted, sizeof(struct pid_stat *), compar_pid_sortlist);

        // we forward read all running processes
        // collect_data_for_pid() is smart enough,
        // not to read the same pid twice per iteration
        for (slc = 0; slc < sorted; slc++) {
            p = pids.sorted.array[slc];
            collect_data_for_pid_stat(p, NULL);
        }
    }

    return true;
}
#endif

// ----------------------------------------------------------------------------

static inline void link_all_processes_to_their_parents(void) {
    // link all children to their parents
    // and update children count on parents
    for(struct pid_stat *p = root_of_pids(); p ; p = p->next) {
        // for each process found

        p->parent = NULL;
        if(unlikely(!p->ppid))
            continue;

        struct pid_stat *pp = find_pid_entry(p->ppid);
        if(likely(pp)) {
            p->parent = pp;
            pp->children_count++;
        }
        else {
            p->parent = NULL;
            netdata_log_error("pid %d %s states parent %d, but the later does not exist.", p->pid, pid_stat_comm(p), p->ppid);
        }
    }
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
            , pid_stat_comm(p)
            , p->pid
            , p->updated?"running":"exited"
            , p->stat_collected_usec - time
    );

    if(p->values[PDF_UTIME])   fprintf(stderr, " utime=" KERNEL_UINT_FORMAT,   p->values[PDF_UTIME]);
    if(p->values[PDF_STIME])   fprintf(stderr, " stime=" KERNEL_UINT_FORMAT,   p->values[PDF_STIME]);
#if (PROCESSES_HAVE_CPU_GUEST_TIME == 1)
    if(p->values[PDF_GTIME])   fprintf(stderr, " gtime=" KERNEL_UINT_FORMAT,   p->values[PDF_GTIME]);
#endif
#if (PROCESSES_HAVE_CPU_CHILDREN_TIME == 1)
    if(p->values[PDF_CUTIME])  fprintf(stderr, " cutime=" KERNEL_UINT_FORMAT,  p->values[PDF_CUTIME]);
    if(p->values[PDF_CSTIME])  fprintf(stderr, " cstime=" KERNEL_UINT_FORMAT,  p->values[PDF_CSTIME]);
#if (PROCESSES_HAVE_CPU_GUEST_TIME == 1)
    if(p->values[PDF_CGTIME])  fprintf(stderr, " cgtime=" KERNEL_UINT_FORMAT,  p->values[PDF_CGTIME]);
#endif
#endif
    if(p->values[PDF_MINFLT])  fprintf(stderr, " minflt=" KERNEL_UINT_FORMAT,  p->values[PDF_MINFLT]);
#if (PROCESSES_HAVE_MAJFLT == 1)
    if(p->values[PDF_MAJFLT])  fprintf(stderr, " majflt=" KERNEL_UINT_FORMAT,  p->values[PDF_MAJFLT]);
#endif
#if (PROCESSES_HAVE_CHILDREN_FLTS == 1)
    if(p->values[PDF_CMINFLT]) fprintf(stderr, " cminflt=" KERNEL_UINT_FORMAT, p->values[PDF_CMINFLT]);
    if(p->values[PDF_CMAJFLT]) fprintf(stderr, " cmajflt=" KERNEL_UINT_FORMAT, p->values[PDF_CMAJFLT]);
#endif
    fprintf(stderr, ")\n");

    return indent + 1;
}

static inline void debug_print_process_tree(struct pid_stat *p, char *msg __maybe_unused) {
    debug_log("%s: process %s (%d, %s) with parents:", msg, pid_stat_comm(p), p->pid, p->updated?"running":"exited");
    debug_print_process_and_parents(p, p->stat_collected_usec);
}

static inline void debug_find_lost_child(struct pid_stat *pe, kernel_uint_t lost, int type) {
    int found = 0;
    struct pid_stat *p = NULL;

    for(p = root_of_pids(); p ; p = p->next) {
        if(p == pe) continue;

        switch(type) {
            case 1:
#if (PROCESSES_HAVE_CHILDREN_FLTS == 1)
                if(p->values[PDF_CMINFLT] > lost) {
                    fprintf(stderr, " > process %d (%s) could use the lost exited child minflt " KERNEL_UINT_FORMAT " of process %d (%s)\n",
                            p->pid, pid_stat_comm(p), lost, pe->pid, pid_stat_comm(pe));
                    found++;
                }
#endif
                break;

            case 2:
#if (PROCESSES_HAVE_CHILDREN_FLTS == 1)
                if(p->values[PDF_CMAJFLT] > lost) {
                    fprintf(stderr, " > process %d (%s) could use the lost exited child majflt " KERNEL_UINT_FORMAT " of process %d (%s)\n",
                            p->pid, pid_stat_comm(p), lost, pe->pid, pid_stat_comm(pe));
                    found++;
                }
#endif
                break;

            case 3:
#if (PROCESSES_HAVE_CPU_CHILDREN_TIME == 1)
                if(p->values[PDF_CUTIME] > lost) {
                    fprintf(stderr, " > process %d (%s) could use the lost exited child utime " KERNEL_UINT_FORMAT " of process %d (%s)\n",
                            p->pid, pid_stat_comm(p), lost, pe->pid, pid_stat_comm(pe));
                    found++;
                }
#endif
                break;

            case 4:
#if (PROCESSES_HAVE_CPU_CHILDREN_TIME == 1)
                if(p->values[PDF_CSTIME] > lost) {
                    fprintf(stderr, " > process %d (%s) could use the lost exited child stime " KERNEL_UINT_FORMAT " of process %d (%s)\n",
                            p->pid, pid_stat_comm(p), lost, pe->pid, pid_stat_comm(pe));
                    found++;
                }
#endif
                break;

            case 5:
#if (PROCESSES_HAVE_CPU_CHILDREN_TIME == 1) && (PROCESSES_HAVE_CPU_GUEST_TIME == 1)
                if(p->values[PDF_CGTIME] > lost) {
                    fprintf(stderr, " > process %d (%s) could use the lost exited child gtime " KERNEL_UINT_FORMAT " of process %d (%s)\n",
                            p->pid, pid_stat_comm(p), lost, pe->pid, pid_stat_comm(pe));
                    found++;
                }
#endif
                break;
        }
    }

    if(!found) {
        switch(type) {
            case 1:
                fprintf(stderr, " > cannot find any process to use the lost exited child minflt " KERNEL_UINT_FORMAT " of process %d (%s)\n",
                        lost, pe->pid, pid_stat_comm(pe));
                break;

            case 2:
                fprintf(stderr, " > cannot find any process to use the lost exited child majflt " KERNEL_UINT_FORMAT " of process %d (%s)\n",
                        lost, pe->pid, pid_stat_comm(pe));
                break;

            case 3:
                fprintf(stderr, " > cannot find any process to use the lost exited child utime " KERNEL_UINT_FORMAT " of process %d (%s)\n",
                        lost, pe->pid, pid_stat_comm(pe));
                break;

            case 4:
                fprintf(stderr, " > cannot find any process to use the lost exited child stime " KERNEL_UINT_FORMAT " of process %d (%s)\n",
                        lost, pe->pid, pid_stat_comm(pe));
                break;

            case 5:
                fprintf(stderr, " > cannot find any process to use the lost exited child gtime " KERNEL_UINT_FORMAT " of process %d (%s)\n",
                        lost, pe->pid, pid_stat_comm(pe));
                break;
        }
    }
}

void clear_pid_stat(struct pid_stat *p, bool threads) {
    p->values[PDF_UTIME]    = 0;
    p->values[PDF_STIME]    = 0;
#if (PROCESSES_HAVE_CPU_GUEST_TIME == 1)
    p->values[PDF_GTIME]    = 0;
#endif
#if (PROCESSES_HAVE_CPU_CHILDREN_TIME == 1)
    p->values[PDF_CUTIME]   = 0;
    p->values[PDF_CSTIME]   = 0;
#if (PROCESSES_HAVE_CPU_GUEST_TIME == 1)
    p->values[PDF_CGTIME]   = 0;
#endif
#endif

    p->values[PDF_MINFLT]   = 0;
#if (PROCESSES_HAVE_MAJFLT == 1)
    p->values[PDF_MAJFLT]   = 0;
#endif
#if (PROCESSES_HAVE_CHILDREN_FLTS == 1)
    p->values[PDF_CMINFLT]  = 0;
    p->values[PDF_CMAJFLT]  = 0;
#endif

    if(threads)
        p->values[PDF_THREADS] = 0;
}

void clear_pid_io(struct pid_stat *p) {
#if (PROCESSES_HAVE_LOGICAL_IO == 1)
    p->values[PDF_LREAD]   = 0;
    p->values[PDF_LWRITE]  = 0;
#endif
#if (PROCESSES_HAVE_PHYSICAL_IO == 1)
    p->values[PDF_PREAD]   = 0;
    p->values[PDF_PWRITE]  = 0;
#endif
#if (PROCESSES_HAVE_IO_CALLS == 1)
    p->values[PDF_CREAD]   = 0;
    p->values[PDF_CWRITE]  = 0;
#endif
}

static inline void assign_app_group_target_to_pid(struct pid_stat *p) {
    targets_assignment_counter++;

    for(struct target *w = apps_groups_root_target; w ; w = w->next) {
        // find it - 4 cases:
        // 1. the target is not a pattern
        // 2. the target has the prefix
        // 3. the target has the suffix
        // 4. the target is something inside cmdline

        if(unlikely(( (!w->starts_with && !w->ends_with && w->compare == p->comm)
                      || (w->starts_with && !w->ends_with && string_starts_with_string(p->comm, w->compare))
                      || (!w->starts_with && w->ends_with && string_ends_with_string(p->comm, w->compare))
                      || (proc_pid_cmdline_is_needed && w->starts_with && w->ends_with && strstr(pid_stat_cmdline(p), string2str(w->compare)))
                          ))) {

            p->matched_by_config = true;
            if(w->target) p->target = w->target;
            else p->target = w;

            if(debug_enabled || (p->target && p->target->debug_enabled))
                debug_log_int("%s linked to target %s",
                              pid_stat_comm(p), string2str(p->target->name));

            break;
        }
    }
}

void update_pid_comm(struct pid_stat *p, const char *comm) {
    if(strcmp(pid_stat_comm(p), comm) != 0) {
        if(unlikely(debug_enabled)) {
            if(string_strlen(p->comm))
                debug_log("\tpid %d (%s) changed name to '%s'", p->pid, pid_stat_comm(p), comm);
            else
                debug_log("\tJust added %d (%s)", p->pid, comm);
        }

        string_freez(p->comm);
        p->comm = string_strdupz(comm);

        // /proc/<pid>/cmdline
        if(likely(proc_pid_cmdline_is_needed))
            managed_log(p, PID_LOG_CMDLINE, read_proc_pid_cmdline(p));

        assign_app_group_target_to_pid(p);
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

static inline void process_exited_pids() {
    struct pid_stat *p;

    for(p = root_of_pids(); p ; p = p->next) {
        if(p->updated || !p->stat_collected_usec)
            continue;

#if (PROCESSES_HAVE_CPU_CHILDREN_TIME == 1)
        kernel_uint_t utime  = (p->raw[PDF_UTIME] + p->raw[PDF_CUTIME])   * (USEC_PER_SEC * RATES_DETAIL) / (p->stat_collected_usec - p->last_stat_collected_usec);
        kernel_uint_t stime  = (p->raw[PDF_STIME] + p->raw[PDF_CSTIME])   * (USEC_PER_SEC * RATES_DETAIL) / (p->stat_collected_usec - p->last_stat_collected_usec);
#if (PROCESSES_HAVE_CPU_GUEST_TIME == 1)
        kernel_uint_t gtime  = (p->raw[PDF_GTIME] + p->raw[PDF_CGTIME])   * (USEC_PER_SEC * RATES_DETAIL) / (p->stat_collected_usec - p->last_stat_collected_usec);
#else
        kernel_uint_t gtime = 0;
#endif
#else
        kernel_uint_t utime  = (p->raw[PDF_UTIME])   * (USEC_PER_SEC * RATES_DETAIL) / (p->stat_collected_usec - p->last_stat_collected_usec);
        kernel_uint_t stime  = (p->raw[PDF_STIME])   * (USEC_PER_SEC * RATES_DETAIL) / (p->stat_collected_usec - p->last_stat_collected_usec);
#if (PROCESSES_HAVE_CPU_GUEST_TIME == 1)
        kernel_uint_t gtime  = (p->raw[PDF_GTIME])   * (USEC_PER_SEC * RATES_DETAIL) / (p->stat_collected_usec - p->last_stat_collected_usec);
#else
        kernel_uint_t gtime = 0;
#endif
#endif

#if (PROCESSES_HAVE_CHILDREN_FLTS == 1)
        kernel_uint_t minflt = (p->raw[PDF_MINFLT] + p->raw[PDF_CMINFLT]) * (USEC_PER_SEC * RATES_DETAIL) / (p->stat_collected_usec - p->last_stat_collected_usec);
#if (PROCESSES_HAVE_MAJFLT == 1)
        kernel_uint_t majflt = (p->raw[PDF_MAJFLT] + p->raw[PDF_CMAJFLT]) * (USEC_PER_SEC * RATES_DETAIL) / (p->stat_collected_usec - p->last_stat_collected_usec);
#else
        kernel_uint_t majflt = 0;
#endif
#else
        kernel_uint_t minflt = (p->raw[PDF_MINFLT]) * (USEC_PER_SEC * RATES_DETAIL) / (p->stat_collected_usec - p->last_stat_collected_usec);
#if (PROCESSES_HAVE_MAJFLT == 1)
        kernel_uint_t majflt = (p->raw[PDF_MAJFLT]) * (USEC_PER_SEC * RATES_DETAIL) / (p->stat_collected_usec - p->last_stat_collected_usec);
#else
        kernel_uint_t majflt = 0;
#endif
#endif

        if(utime + stime + gtime + minflt + majflt == 0)
            continue;

        if(unlikely(debug_enabled)) {
            debug_log("Absorb %s (%d %s total resources: utime=" KERNEL_UINT_FORMAT " stime=" KERNEL_UINT_FORMAT " gtime=" KERNEL_UINT_FORMAT " minflt=" KERNEL_UINT_FORMAT " majflt=" KERNEL_UINT_FORMAT ")"
                      , pid_stat_comm(p)
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
#if (PROCESSES_HAVE_CPU_CHILDREN_TIME == 1)
            absorbed = remove_exited_child_from_parent(&utime,  &pp->values[PDF_CUTIME]);
            if(unlikely(debug_enabled && absorbed))
                debug_log(" > process %s (%d %s) absorbed " KERNEL_UINT_FORMAT " utime (remaining: " KERNEL_UINT_FORMAT ")",
                          pid_stat_comm(pp), pp->pid, pp->updated?"running":"exited", absorbed, utime);

            absorbed = remove_exited_child_from_parent(&stime,  &pp->values[PDF_CSTIME]);
            if(unlikely(debug_enabled && absorbed))
                debug_log(" > process %s (%d %s) absorbed " KERNEL_UINT_FORMAT " stime (remaining: " KERNEL_UINT_FORMAT ")",
                          pid_stat_comm(pp), pp->pid, pp->updated?"running":"exited", absorbed, stime);

#if (PROCESSES_HAVE_CPU_GUEST_TIME == 1)
            absorbed = remove_exited_child_from_parent(&gtime,  &pp->values[PDF_CGTIME]);
            if(unlikely(debug_enabled && absorbed))
                debug_log(" > process %s (%d %s) absorbed " KERNEL_UINT_FORMAT " gtime (remaining: " KERNEL_UINT_FORMAT ")",
                          pid_stat_comm(pp), pp->pid, pp->updated?"running":"exited", absorbed, gtime);
#endif
#endif

            absorbed = remove_exited_child_from_parent(&minflt, &pp->values[PDF_CMINFLT]);
            if(unlikely(debug_enabled && absorbed))
                debug_log(" > process %s (%d %s) absorbed " KERNEL_UINT_FORMAT " minflt (remaining: " KERNEL_UINT_FORMAT ")",
                          pid_stat_comm(pp), pp->pid, pp->updated?"running":"exited", absorbed, minflt);

            absorbed = remove_exited_child_from_parent(&majflt, &pp->values[PDF_CMAJFLT]);
            if(unlikely(debug_enabled && absorbed))
                debug_log(" > process %s (%d %s) absorbed " KERNEL_UINT_FORMAT " majflt (remaining: " KERNEL_UINT_FORMAT ")",
                          pid_stat_comm(pp), pp->pid, pp->updated?"running":"exited", absorbed, majflt);
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
                      , pid_stat_comm(p)
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
                          , pid_stat_comm(pp)
                          , pp->pid
                          , pp->updated?"running":"exited"
                );
            }

            p->raw[PDF_UTIME]   = utime  * (p->stat_collected_usec - p->last_stat_collected_usec) / (USEC_PER_SEC * RATES_DETAIL);
            p->raw[PDF_STIME]   = stime  * (p->stat_collected_usec - p->last_stat_collected_usec) / (USEC_PER_SEC * RATES_DETAIL);
#if (PROCESSES_HAVE_CPU_GUEST_TIME == 1)
            p->raw[PDF_GTIME]   = gtime  * (p->stat_collected_usec - p->last_stat_collected_usec) / (USEC_PER_SEC * RATES_DETAIL);
#endif
            p->raw[PDF_MINFLT]  = minflt * (p->stat_collected_usec - p->last_stat_collected_usec) / (USEC_PER_SEC * RATES_DETAIL);
            p->raw[PDF_MAJFLT]  = majflt * (p->stat_collected_usec - p->last_stat_collected_usec) / (USEC_PER_SEC * RATES_DETAIL);
#if (PROCESSES_HAVE_CPU_CHILDREN_TIME == 1)
            p->raw[PDF_CUTIME] = p->raw[PDF_CSTIME] =
#if (PROCESSES_HAVE_CPU_GUEST_TIME == 1)
                    p->raw[PDF_CGTIME] =
#endif
                    p->raw[PDF_CMINFLT] = p->raw[PDF_CMAJFLT] = 0;
#endif

            debug_log(" ");
        }
        else
            debug_log(" > totally absorbed - DONE - %s (%d %s)"
                      , pid_stat_comm(p)
                      , p->pid
                      , p->updated?"running":"exited"
            );
    }
}

void mark_pid_as_unread(struct pid_stat *p) {
    p->read = false; // mark it as not read, so that collect_data_for_pid() will read it
    p->updated = false;
    p->merged = false;
    p->children_count = 0;
    p->parent = NULL;
}

int read_proc_pid_stat(struct pid_stat *p, void *ptr) {
    p->last_stat_collected_usec = p->stat_collected_usec;
    p->stat_collected_usec = now_monotonic_usec();
    calls_counter++;

    if(!read_proc_pid_stat_per_os(p, ptr))
        return 0;

    return 1;
}

int read_proc_pid_status(struct pid_stat *p, void *ptr) {
    p->values[PDF_VMSIZE]   = 0;
    p->values[PDF_VMRSS]    = 0;

#if (PROCESSES_HAVE_VMSHARED == 1)
    p->values[PDF_VMSHARED] = 0;
#endif

#if (PROCESSES_HAVE_RSSFILE == 1)
    p->values[PDF_RSSFILE]  = 0;
#endif

#if (PROCESSES_HAVE_RSSSHMEM == 1)
    p->values[PDF_RSSSHMEM] = 0;
#endif

#if (PROCESSES_HAVE_VMSWAP == 1)
    p->values[PDF_VMSWAP]   = 0;
#endif

#if (PROCESSES_HAVE_VOLCTX == 1)
    p->values[PDF_VOLCTX]   = 0;
#endif

#if (PROCESSES_HAVE_NVOLCTX == 1)
    p->values[PDF_NVOLCTX]  = 0;
#endif

    return read_proc_pid_status_per_os(p, ptr) ? 1 : 0;
}

int read_proc_pid_limits(struct pid_stat *p, void *ptr) {
    return read_proc_pid_limits_per_os(p, ptr) ? 1 : 0;
}

int read_proc_pid_io(struct pid_stat *p, void *ptr) {
    p->last_io_collected_usec = p->io_collected_usec;
    p->io_collected_usec = now_monotonic_usec();
    calls_counter++;

    bool ret = read_proc_pid_io_per_os(p, ptr);

    if(unlikely(global_iterations_counter == 1))
        clear_pid_io(p);

    return ret ? 1 : 0;
}

int read_proc_pid_cmdline(struct pid_stat *p) {
    static char cmdline[MAX_CMDLINE];

    if(unlikely(!get_cmdline_per_os(p, cmdline, sizeof(cmdline))))
        goto cleanup;

    string_freez(p->cmdline);
    p->cmdline = string_strdupz(cmdline);

    return 1;

cleanup:
    // copy the command to the command line
    string_freez(p->cmdline);
    p->cmdline = string_dup(p->comm);
    return 0;
}

kernel_uint_t MemTotal = 0;

void get_MemTotal(void) {
    if(!get_MemTotal_per_os())
        MemTotal = 0;
}

bool collect_data_for_all_pids(void) {
    if(!apps_os_collect())
        return false;

    if(!pids.all_pids.count)
        return false;

    // we need /proc/stat to normalize the cpu consumption of the exited childs
    apps_os_read_global_cpu_utilization();

    // build the process tree
    link_all_processes_to_their_parents();

    // normally this is done
    // however we may have processes exited while we collected values
    // so let's find the exited ones
    // we do this by collecting the ownership of process
    // if we manage to get the ownership, the process still runs
    process_exited_pids();

    return true;
}
