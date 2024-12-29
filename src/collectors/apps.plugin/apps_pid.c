// SPDX-License-Identifier: GPL-3.0-or-later

#include "apps_plugin.h"

static inline void link_pid_to_its_parent(struct pid_stat *p);

// --------------------------------------------------------------------------------------------------------------------
// The index of all pids

#define SIMPLE_HASHTABLE_NAME _PID
#define SIMPLE_HASHTABLE_VALUE_TYPE struct pid_stat *
#define SIMPLE_HASHTABLE_KEY_TYPE int32_t
#define SIMPLE_HASHTABLE_VALUE2KEY_FUNCTION pid_stat_to_pid_ptr
#define SIMPLE_HASHTABLE_COMPARE_KEYS_FUNCTION pid_ptr_eq
#define SIMPLE_HASHTABLE_SAMPLE_IMPLEMENTATION 0
#include "libnetdata/simple_hashtable/simple_hashtable.h"

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

void apps_pids_init(void) {
    pids.all_pids.aral = aral_create("pid_stat", sizeof(struct pid_stat),
                                     1, 0, NULL, NULL, NULL,
                                     false, true, false);
    simple_hashtable_init_PID(&pids.all_pids.ht, 1024);
}

static inline uint64_t pid_hash(pid_t pid) {
    return XXH3_64bits(&pid, sizeof(pid));
}

inline struct pid_stat *find_pid_entry(pid_t pid) {
    if(pid < INIT_PID) return NULL;

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

#if (PROCESSES_HAVE_FDS == 1)
    p->fds = mallocz(sizeof(struct pid_fd) * 3); // stdin, stdout, stderr
    p->fds_size = 3;
    init_pid_fds(p, 0, p->fds_size);
#endif

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

    freez(p->fds_dirname);
    freez(p->stat_filename);
    freez(p->status_filename);
    freez(p->limits_filename);
    freez(p->io_filename);
    freez(p->cmdline_filename);
#endif

#if (PROCESSES_HAVE_FDS == 1)
    freez(p->fds);
#endif

#if (PROCESSES_HAVE_SID == 1)
    string_freez(p->sid_name);
#endif

    string_freez(p->comm_orig);
    string_freez(p->comm);
    string_freez(p->cmdline);
    aral_freez(pids.all_pids.aral, p);

    pids.all_pids.count--;
}

// --------------------------------------------------------------------------------------------------------------------

static __thread pid_t current_pid;
static __thread kernel_uint_t current_pid_values[PDF_MAX];

void pid_collection_started(struct pid_stat *p) {
    fatal_assert(sizeof(current_pid_values) == sizeof(p->values));
    current_pid = p->pid;
    memcpy(current_pid_values, p->values, sizeof(current_pid_values));
    memset(p->values, 0, sizeof(p->values));
    p->values[PDF_PROCESSES] = 1;
    p->read = true;
}

void pid_collection_failed(struct pid_stat *p) {
    fatal_assert(current_pid == p->pid);
    fatal_assert(sizeof(current_pid_values) == sizeof(p->values));
    memcpy(p->values, current_pid_values, sizeof(p->values));
}

void pid_collection_completed(struct pid_stat *p) {
    p->updated = true;
    p->keep = false;
    p->keeploops = 0;
}

// --------------------------------------------------------------------------------------------------------------------
// preloading of parents before their children

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
        pids.sorted.array[slc++] = p;

        // assign a sortlist id to all it and its parents
        for (struct pid_stat *pp = p; pp ; pp = pp->parent)
            pp->sortlist = sortlist++;
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
        // incrementally_collect_data_for_pid() is smart enough,
        // not to read the same pid twice per iteration
        for (slc = 0; slc < sorted; slc++) {
            p = pids.sorted.array[slc];
            incrementally_collect_data_for_pid_stat(p, NULL);
        }
    }

    return true;
}
#endif

// --------------------------------------------------------------------------------------------------------------------

static void log_parent_loop(struct pid_stat *p) {
    CLEAN_BUFFER *wb = buffer_create(0, NULL);
    buffer_sprintf(wb, "original pid %d (%s)", p->pid, string2str(p->comm));

    size_t loops = 0;
    for(struct pid_stat *t = p->parent; t && loops < 2 ;t = t->parent) {
        buffer_sprintf(wb, " => %d (%s)", t->pid, string2str(t->comm));
        if(t == p->parent) loops++;
    }

    buffer_sprintf(wb, " : broke loop at %d (%s)", p->pid, string2str(p->comm));

    errno_clear();
    nd_log(NDLS_COLLECTORS, NDLP_WARNING, "Parents loop detected: %s", buffer_tostring(wb));
}

static inline bool is_already_a_parent(struct pid_stat *p, struct pid_stat *pp) {
    for(struct pid_stat *t = pp; t ;t = t->parent)
        if(t == p) return true;

    return false;
}

static inline void link_pid_to_its_parent(struct pid_stat *p) {
    p->parent = NULL;
    if(unlikely(!p->ppid))
        return;

    if(unlikely(p->ppid == p->pid)) {
        nd_log(NDLS_COLLECTORS, NDLP_WARNING,
               "Process %d (%s) states parent %d, which is the same PID. Ignoring it.",
               p->pid, string2str(p->comm), p->ppid);
        p->ppid = 0;
        return;
    }

    struct pid_stat *pp = find_pid_entry(p->ppid);
    if(likely(pp)) {
        fatal_assert(pp->pid == p->ppid);

        if(!is_already_a_parent(p, pp)) {
            p->parent = pp;
            pp->children_count++;
        }
        else {
            p->parent = pp;
            log_parent_loop(p);
            p->parent = NULL;
            p->ppid = 0;
        }
    }
#if (PPID_SHOULD_BE_RUNNING == 1)
    else {
        nd_log(NDLS_COLLECTORS, NDLP_WARNING,
               "pid %d %s states parent %d, but the later does not exist.",
               p->pid, pid_stat_comm(p), p->ppid);
    }
#endif
}

static inline void link_all_processes_to_their_parents(void) {
    // link all children to their parents
    // and update children count on parents
    for(struct pid_stat *p = root_of_pids(); p ; p = p->next)
        link_pid_to_its_parent(p);
}

// --------------------------------------------------------------------------------------------------------------------

static bool is_filename(const char *s) {
    if(!s || !*s) return false;

#if defined(OS_WINDOWS)
    if( (isalpha((uint8_t)*s) || (s[1] == ':' && s[2] == '\\')) ||                  // windows native "x:\"
        (isalpha((uint8_t)*s) || (s[1] == ':' && s[2] == '/')) ||                   // windows native "x:/"
        (*s == '\\' && s[1] == '\\' && isalpha((uint8_t)s[2]) && s[3] == '\\') ||   // windows native "\\x\"
        (*s == '/' && s[1] == '/' && isalpha((uint8_t)s[2]) && s[3] == '/')) {      // windows native "//x/"

        WCHAR ws[FILENAME_MAX];
        if(utf8_to_utf16(ws, _countof(ws), s, -1) > 0) {
            DWORD attributes = GetFileAttributesW(ws);
            if (attributes != INVALID_FILE_ATTRIBUTES)
                return true;
        }
    }
#endif

    // for: sh -c "exec /path/to/command parameters"
    if(strncmp(s, "exec ", 5) == 0 && s[5]) {
        s += 5;
        char look_for = ' ';
        if(*s == '\'') { look_for = '\''; s++; }
        if(*s == '"') { look_for = '"'; s++; }
        char *end = strchr(s, look_for);
        if(end) *end = '\0';
    }

    // linux, freebsd, macos, msys, cygwin
    if(*s == '/') {
        struct statvfs stat;
        return statvfs(s, &stat) == 0;
    }

    return false;
}

static const char *extensions_to_strip[] = {
        ".sh", // shell scripts
        ".py", // python scripts
        ".pl", // perl scripts
        ".js", // node.js
#if defined(OS_WINDOWS)
        ".exe",
#endif
        NULL,
};

// strip extensions we don't want to show
static void remove_extension(char *name) {
    size_t name_len = strlen(name);
    for(size_t i = 0; extensions_to_strip[i] != NULL; i++) {
        const char *ext = extensions_to_strip[i];
        size_t ext_len = strlen(ext);
        if(name_len > ext_len) {
            char *check = &name[name_len - ext_len];
            if(strcmp(check, ext) == 0) {
                *check = '\0';
                break;
            }
        }
    }
}

static inline STRING *comm_from_cmdline_param_sanitized(STRING *cmdline) {
    if(!cmdline) return NULL;

    char buf[string_strlen(cmdline) + 1];
    memcpy(buf, string2str(cmdline), sizeof(buf));

    char *words[100];
    size_t num_words = quoted_strings_splitter_whitespace(buf, words, 100);
    for(size_t word = 1; word < num_words ;word++) {
        char *s = words[word];
        if(is_filename(s)) {
            char *name = strrchr(s, '/');

#if defined(OS_WINDOWS)
            if(!name)
                name = strrchr(s, '\\');
#endif

            if(name && *name) {
                name++;
                remove_extension(name);
                sanitize_apps_plugin_chart_meta(name);
                return string_strdupz(name);
            }
        }
    }

    return NULL;
}

static inline STRING *comm_from_cmdline_sanitized(STRING *comm, STRING *cmdline) {
    if(!cmdline) return NULL;

    char buf[string_strlen(cmdline) + 1];
    memcpy(buf, string2str(cmdline), sizeof(buf));

    size_t comm_len = string_strlen(comm);
    char *start = strstr(buf, string2str(comm));
    while (start) {
        char *end = start + comm_len;
        while (*end &&
               !isspace((uint8_t) *end) &&
               *end != '/' &&    // path separator - linux
               *end != '\\' &&   // path separator - windows
               *end != '"' &&    // closing double quotes
               *end != '\'' &&   // closing single quotes
               *end != ')' &&    // sometimes process add ) at their end
               *end != ':')      // sometimes process add : at their end
            end++;

        *end = '\0';

        remove_extension(start);
        sanitize_apps_plugin_chart_meta(start);
        return string_strdupz(start);
    }

    return NULL;
}

static void update_pid_comm_from_cmdline(struct pid_stat *p) {
    bool updated = false;

    STRING *new_comm = comm_from_cmdline_sanitized(p->comm, p->cmdline);
    if(new_comm) {
        string_freez(p->comm);
        p->comm = new_comm;
        updated = true;
    }

    if(is_process_an_interpreter(p)) {
        new_comm = comm_from_cmdline_param_sanitized(p->cmdline);
        if(new_comm) {
            string_freez(p->comm);
            p->comm = new_comm;
            updated = true;
        }
    }

    if(updated) {
        p->is_manager = is_process_a_manager(p);
        p->is_aggregator = is_process_an_aggregator(p);
    }
}

void update_pid_cmdline(struct pid_stat *p, const char *cmdline) {
    string_freez(p->cmdline);
    p->cmdline = cmdline ? string_strdupz(cmdline) : NULL;

    if(p->cmdline)
        update_pid_comm_from_cmdline(p);
}

void update_pid_comm(struct pid_stat *p, const char *comm) {
    if(p->comm_orig && string_strcmp(p->comm_orig, comm) == 0)
        // no change
        return;

    string_freez(p->comm_orig);
    p->comm_orig = string_strdupz(comm);

    // some process names have ( and ), remove the parenthesis
    size_t len = strlen(comm);
    char buf[len + 1];
    if(comm[0] == '(' && comm[len - 1] == ')') {
        memcpy(buf, &comm[1], len - 2);
        buf[len - 2] = '\0';
    }
    else
        memcpy(buf, comm, sizeof(buf));

    sanitize_apps_plugin_chart_meta(buf);
    p->comm = string_strdupz(buf);
    p->is_manager = is_process_a_manager(p);
    p->is_aggregator = is_process_an_aggregator(p);

#if (PROCESSES_HAVE_CMDLINE == 1)
    if(likely(proc_pid_cmdline_is_needed && !p->cmdline))
        managed_log(p, PID_LOG_CMDLINE, read_proc_pid_cmdline(p));
#else
    update_pid_comm_from_cmdline(p);
#endif

    // the process changed comm, we may have to reassign it to
    // an apps_groups.conf target.
    p->target = NULL;
}

// --------------------------------------------------------------------------------------------------------------------

#if (PROCESSES_HAVE_CPU_CHILDREN_TIME == 1) || (PROCESSES_HAVE_CHILDREN_FLTS == 1)
//static inline int debug_print_process_and_parents(struct pid_stat *p, usec_t time) {
//    char *prefix = "\\_ ";
//    int indent = 0;
//
//    if(p->parent)
//        indent = debug_print_process_and_parents(p->parent, p->stat_collected_usec);
//    else
//        prefix = " > ";
//
//    char buffer[indent + 1];
//    int i;
//
//    for(i = 0; i < indent ;i++) buffer[i] = ' ';
//    buffer[i] = '\0';
//
//    fprintf(stderr, "  %s %s%s (%d %s %"PRIu64""
//            , buffer
//            , prefix
//            , pid_stat_comm(p)
//            , p->pid
//            , p->updated?"running":"exited"
//            , p->stat_collected_usec - time
//    );
//
//    if(p->values[PDF_UTIME])   fprintf(stderr, " utime=" KERNEL_UINT_FORMAT,   p->values[PDF_UTIME]);
//    if(p->values[PDF_STIME])   fprintf(stderr, " stime=" KERNEL_UINT_FORMAT,   p->values[PDF_STIME]);
//#if (PROCESSES_HAVE_CPU_GUEST_TIME == 1)
//    if(p->values[PDF_GTIME])   fprintf(stderr, " gtime=" KERNEL_UINT_FORMAT,   p->values[PDF_GTIME]);
//#endif
//#if (PROCESSES_HAVE_CPU_CHILDREN_TIME == 1)
//    if(p->values[PDF_CUTIME])  fprintf(stderr, " cutime=" KERNEL_UINT_FORMAT,  p->values[PDF_CUTIME]);
//    if(p->values[PDF_CSTIME])  fprintf(stderr, " cstime=" KERNEL_UINT_FORMAT,  p->values[PDF_CSTIME]);
//#if (PROCESSES_HAVE_CPU_GUEST_TIME == 1)
//    if(p->values[PDF_CGTIME])  fprintf(stderr, " cgtime=" KERNEL_UINT_FORMAT,  p->values[PDF_CGTIME]);
//#endif
//#endif
//    if(p->values[PDF_MINFLT])  fprintf(stderr, " minflt=" KERNEL_UINT_FORMAT,  p->values[PDF_MINFLT]);
//#if (PROCESSES_HAVE_MAJFLT == 1)
//    if(p->values[PDF_MAJFLT])  fprintf(stderr, " majflt=" KERNEL_UINT_FORMAT,  p->values[PDF_MAJFLT]);
//#endif
//#if (PROCESSES_HAVE_CHILDREN_FLTS == 1)
//    if(p->values[PDF_CMINFLT]) fprintf(stderr, " cminflt=" KERNEL_UINT_FORMAT, p->values[PDF_CMINFLT]);
//    if(p->values[PDF_CMAJFLT]) fprintf(stderr, " cmajflt=" KERNEL_UINT_FORMAT, p->values[PDF_CMAJFLT]);
//#endif
//    fprintf(stderr, ")\n");
//
//    return indent + 1;
//}
//
//static inline void debug_print_process_tree(struct pid_stat *p, char *msg __maybe_unused) {
//    debug_log("%s: process %s (%d, %s) with parents:", msg, pid_stat_comm(p), p->pid, p->updated?"running":"exited");
//    debug_print_process_and_parents(p, p->stat_collected_usec);
//}
//
//static inline void debug_find_lost_child(struct pid_stat *pe, kernel_uint_t lost, int type) {
//    int found = 0;
//    struct pid_stat *p = NULL;
//
//    for(p = root_of_pids(); p ; p = p->next) {
//        if(p == pe) continue;
//
//        switch(type) {
//            case 1:
//#if (PROCESSES_HAVE_CHILDREN_FLTS == 1)
//                if(p->values[PDF_CMINFLT] > lost) {
//                    fprintf(stderr, " > process %d (%s) could use the lost exited child minflt " KERNEL_UINT_FORMAT " of process %d (%s)\n",
//                            p->pid, pid_stat_comm(p), lost, pe->pid, pid_stat_comm(pe));
//                    found++;
//                }
//#endif
//                break;
//
//            case 2:
//#if (PROCESSES_HAVE_CHILDREN_FLTS == 1)
//                if(p->values[PDF_CMAJFLT] > lost) {
//                    fprintf(stderr, " > process %d (%s) could use the lost exited child majflt " KERNEL_UINT_FORMAT " of process %d (%s)\n",
//                            p->pid, pid_stat_comm(p), lost, pe->pid, pid_stat_comm(pe));
//                    found++;
//                }
//#endif
//                break;
//
//            case 3:
//#if (PROCESSES_HAVE_CPU_CHILDREN_TIME == 1)
//                if(p->values[PDF_CUTIME] > lost) {
//                    fprintf(stderr, " > process %d (%s) could use the lost exited child utime " KERNEL_UINT_FORMAT " of process %d (%s)\n",
//                            p->pid, pid_stat_comm(p), lost, pe->pid, pid_stat_comm(pe));
//                    found++;
//                }
//#endif
//                break;
//
//            case 4:
//#if (PROCESSES_HAVE_CPU_CHILDREN_TIME == 1)
//                if(p->values[PDF_CSTIME] > lost) {
//                    fprintf(stderr, " > process %d (%s) could use the lost exited child stime " KERNEL_UINT_FORMAT " of process %d (%s)\n",
//                            p->pid, pid_stat_comm(p), lost, pe->pid, pid_stat_comm(pe));
//                    found++;
//                }
//#endif
//                break;
//
//            case 5:
//#if (PROCESSES_HAVE_CPU_CHILDREN_TIME == 1) && (PROCESSES_HAVE_CPU_GUEST_TIME == 1)
//                if(p->values[PDF_CGTIME] > lost) {
//                    fprintf(stderr, " > process %d (%s) could use the lost exited child gtime " KERNEL_UINT_FORMAT " of process %d (%s)\n",
//                            p->pid, pid_stat_comm(p), lost, pe->pid, pid_stat_comm(pe));
//                    found++;
//                }
//#endif
//                break;
//        }
//    }
//
//    if(!found) {
//        switch(type) {
//            case 1:
//                fprintf(stderr, " > cannot find any process to use the lost exited child minflt " KERNEL_UINT_FORMAT " of process %d (%s)\n",
//                        lost, pe->pid, pid_stat_comm(pe));
//                break;
//
//            case 2:
//                fprintf(stderr, " > cannot find any process to use the lost exited child majflt " KERNEL_UINT_FORMAT " of process %d (%s)\n",
//                        lost, pe->pid, pid_stat_comm(pe));
//                break;
//
//            case 3:
//                fprintf(stderr, " > cannot find any process to use the lost exited child utime " KERNEL_UINT_FORMAT " of process %d (%s)\n",
//                        lost, pe->pid, pid_stat_comm(pe));
//                break;
//
//            case 4:
//                fprintf(stderr, " > cannot find any process to use the lost exited child stime " KERNEL_UINT_FORMAT " of process %d (%s)\n",
//                        lost, pe->pid, pid_stat_comm(pe));
//                break;
//
//            case 5:
//                fprintf(stderr, " > cannot find any process to use the lost exited child gtime " KERNEL_UINT_FORMAT " of process %d (%s)\n",
//                        lost, pe->pid, pid_stat_comm(pe));
//                break;
//        }
//    }
//}

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

static inline void process_exited_pids(void) {
    /*
     * WHY WE NEED THIS?
     *
     * When a child process exits in Linux, its accumulated user time (utime) and its children's accumulated
     * user time (cutime) are added to the parent's cutime. This means the parent process's cutime reflects
     * the total user time spent by its exited children and their descendants
     *
     * This results in spikes in the charts.
     * In this function we remove the exited children resources from the parent's cutime, but only for the
     * children we have been monitoring and to the degree we have data for them. Since previously running
     * children have already been reported by us, removing them is the right thing to do.
     *
     */

    for(struct pid_stat *p = root_of_pids(); p ; p = p->next) {
        if(p->updated || !p->stat_collected_usec)
            continue;

        bool have_work = false;

#if (PROCESSES_HAVE_CPU_CHILDREN_TIME == 1)
        kernel_uint_t utime  = (p->raw[PDF_UTIME] + p->raw[PDF_CUTIME]) * CPU_TO_NANOSECONDCORES;
        kernel_uint_t stime  = (p->raw[PDF_STIME] + p->raw[PDF_CSTIME]) * CPU_TO_NANOSECONDCORES;
        if(utime + stime) have_work = true;
#if (PROCESSES_HAVE_CPU_GUEST_TIME == 1)
        kernel_uint_t gtime  = (p->raw[PDF_GTIME] + p->raw[PDF_CGTIME]) * CPU_TO_NANOSECONDCORES;
        if(gtime) have_work = true;
#endif
#endif

#if (PROCESSES_HAVE_CHILDREN_FLTS == 1)
        kernel_uint_t minflt = (p->raw[PDF_MINFLT] + p->raw[PDF_CMINFLT]) * RATES_DETAIL;
        if(minflt) have_work = true;
#if (PROCESSES_HAVE_MAJFLT == 1)
        kernel_uint_t majflt = (p->raw[PDF_MAJFLT] + p->raw[PDF_CMAJFLT]) * RATES_DETAIL;
        if(majflt) have_work = true;
#endif
#endif

        if(!have_work)
            continue;

//        if(unlikely(debug_enabled)) {
//            debug_log("Absorb %s (%d %s total resources: utime=" KERNEL_UINT_FORMAT " stime=" KERNEL_UINT_FORMAT " gtime=" KERNEL_UINT_FORMAT " minflt=" KERNEL_UINT_FORMAT " majflt=" KERNEL_UINT_FORMAT ")"
//                      , pid_stat_comm(p)
//                      , p->pid
//                      , p->updated?"running":"exited"
//                      , utime
//                      , stime
//                      , gtime
//                      , minflt
//                      , majflt
//            );
//            debug_print_process_tree(p, "Searching parents");
//        }

        for(struct pid_stat *pp = p->parent; pp ; pp = pp->parent) {
            if(!pp->updated) continue;

            kernel_uint_t absorbed;
#if (PROCESSES_HAVE_CPU_CHILDREN_TIME == 1)
            absorbed = remove_exited_child_from_parent(&utime,  &pp->values[PDF_CUTIME]);
//            if(unlikely(debug_enabled && absorbed))
//                debug_log(" > process %s (%d %s) absorbed " KERNEL_UINT_FORMAT " utime (remaining: " KERNEL_UINT_FORMAT ")",
//                          pid_stat_comm(pp), pp->pid, pp->updated?"running":"exited", absorbed, utime);

            absorbed = remove_exited_child_from_parent(&stime,  &pp->values[PDF_CSTIME]);
//            if(unlikely(debug_enabled && absorbed))
//                debug_log(" > process %s (%d %s) absorbed " KERNEL_UINT_FORMAT " stime (remaining: " KERNEL_UINT_FORMAT ")",
//                          pid_stat_comm(pp), pp->pid, pp->updated?"running":"exited", absorbed, stime);

#if (PROCESSES_HAVE_CPU_GUEST_TIME == 1)
            absorbed = remove_exited_child_from_parent(&gtime,  &pp->values[PDF_CGTIME]);
//            if(unlikely(debug_enabled && absorbed))
//                debug_log(" > process %s (%d %s) absorbed " KERNEL_UINT_FORMAT " gtime (remaining: " KERNEL_UINT_FORMAT ")",
//                          pid_stat_comm(pp), pp->pid, pp->updated?"running":"exited", absorbed, gtime);
#endif
#endif

#if (PROCESSES_HAVE_CHILDREN_FLTS == 1)
            absorbed = remove_exited_child_from_parent(&minflt, &pp->values[PDF_CMINFLT]);
//            if(unlikely(debug_enabled && absorbed))
//                debug_log(" > process %s (%d %s) absorbed " KERNEL_UINT_FORMAT " minflt (remaining: " KERNEL_UINT_FORMAT ")",
//                          pid_stat_comm(pp), pp->pid, pp->updated?"running":"exited", absorbed, minflt);

#if (PROCESSES_HAVE_MAJFLT == 1)
            absorbed = remove_exited_child_from_parent(&majflt, &pp->values[PDF_CMAJFLT]);
//            if(unlikely(debug_enabled && absorbed))
//                debug_log(" > process %s (%d %s) absorbed " KERNEL_UINT_FORMAT " majflt (remaining: " KERNEL_UINT_FORMAT ")",
//                          pid_stat_comm(pp), pp->pid, pp->updated?"running":"exited", absorbed, majflt);
#endif
#endif

            (void)absorbed;
            break;
        }

//        if(unlikely(debug_enabled)) {
//            if(utime) debug_find_lost_child(p, utime, 3);
//            if(stime) debug_find_lost_child(p, stime, 4);
//            if(gtime) debug_find_lost_child(p, gtime, 5);
//            if(minflt) debug_find_lost_child(p, minflt, 1);
//            if(majflt) debug_find_lost_child(p, majflt, 2);
//        }

//        debug_log(" > remaining resources - KEEP - for another loop: %s (%d %s total resources: utime=" KERNEL_UINT_FORMAT " stime=" KERNEL_UINT_FORMAT " gtime=" KERNEL_UINT_FORMAT " minflt=" KERNEL_UINT_FORMAT " majflt=" KERNEL_UINT_FORMAT ")"
//                  , pid_stat_comm(p)
//                  , p->pid
//                  , p->updated?"running":"exited"
//                  , utime
//                  , stime
//                  , gtime
//                  , minflt
//                  , majflt
//        );

        bool done = true;

#if (PROCESSES_HAVE_CPU_CHILDREN_TIME == 1)
        p->values[PDF_UTIME]  = utime / CPU_TO_NANOSECONDCORES;
        p->values[PDF_STIME]  = stime / CPU_TO_NANOSECONDCORES;
        p->values[PDF_CUTIME] = 0;
        p->values[PDF_CSTIME] = 0;
        if(utime + stime) done = false;
#if (PROCESSES_HAVE_CPU_GUEST_TIME == 1)
        p->values[PDF_GTIME]  = gtime / CPU_TO_NANOSECONDCORES;
        p->values[PDF_CGTIME] = 0;
        if(gtime) done = false;
#endif
#endif

#if (PROCESSES_HAVE_CHILDREN_FLTS == 1)
        p->values[PDF_MINFLT]  = minflt / RATES_DETAIL;
        p->values[PDF_CMINFLT] = 0;
        if(minflt) done = false;
#if (PROCESSES_HAVE_MAJFLT == 1)
        p->values[PDF_MAJFLT]  = majflt / RATES_DETAIL;
        p->values[PDF_CMAJFLT] = 0;
        if(majflt) done = false;
#endif
#endif

        p->keep = !done;

        if(p->keep) {
            // we need to keep its exited parents too, to ensure we will have
            // the information to reach the running parent at the next iteration
            for (struct pid_stat *pp = p->parent; pp; pp = pp->parent) {
                if (pp->updated) break;
                pp->keep = true;
            }
        }
    }
}
#endif

// --------------------------------------------------------------------------------------------------------------------
// the main loop for collecting process data

static inline void clear_pid_rates(struct pid_stat *p) {
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

#if (PROCESSES_HAVE_LOGICAL_IO == 1)
    p->values[PDF_LREAD]   = 0;
    p->values[PDF_LWRITE]  = 0;
#endif

#if (PROCESSES_HAVE_PHYSICAL_IO == 1)
    p->values[PDF_PREAD]   = 0;
    p->values[PDF_PWRITE]  = 0;
#endif

#if (PROCESSES_HAVE_IO_CALLS == 1)
    p->values[PDF_OREAD]   = 0;
    p->values[PDF_OWRITE]  = 0;
#endif

#if (PROCESSES_HAVE_VOLCTX == 1)
    p->values[PDF_VOLCTX]  = 0;
#endif

#if (PROCESSES_HAVE_NVOLCTX == 1)
    p->values[PDF_NVOLCTX]  = 0;
#endif
}

bool collect_data_for_all_pids(void) {
    // mark all pids as unread
#if (INCREMENTAL_DATA_COLLECTION == 0)
    usec_t now_mon_ut = now_monotonic_usec();
#endif

    for(struct pid_stat *p = root_of_pids(); p ; p = p->next) {
        p->read = p->updated = p->merged = false;
        p->children_count = 0;

#if (INCREMENTAL_DATA_COLLECTION == 0)
        p->last_stat_collected_usec = p->stat_collected_usec;
        p->last_io_collected_usec = p->io_collected_usec;
        p->stat_collected_usec = p->io_collected_usec = now_mon_ut;
#endif
    }

    // collect data for all pids
    if(!OS_FUNCTION(apps_os_collect_all_pids)())
        return false;

    // build the process tree
    link_all_processes_to_their_parents();

#if (PROCESSES_HAVE_CPU_CHILDREN_TIME == 1) || (PROCESSES_HAVE_CHILDREN_FLTS == 1)
    // merge exited pids to their parents
    process_exited_pids();
#endif

    // the first iteration needs to be eliminated
    // since we are looking for rates
    if(unlikely(global_iterations_counter == 1)) {
        for(struct pid_stat *p = root_of_pids(); p ; p = p->next)
            if(p->read) clear_pid_rates(p);
    }

    return true;
}
