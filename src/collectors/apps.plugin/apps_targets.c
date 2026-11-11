// SPDX-License-Identifier: GPL-3.0-or-later

#include "apps_plugin.h"

pid_t INIT_PID = OS_INIT_PID;

static STRING *get_clean_name(STRING *name) {
    char *buf = strdupz(string2str(name));
    netdata_fix_chart_name(buf);

    for (char *d = buf; *d ; d++)
        if (*d == '.') *d = '_';

    STRING *clean = string_strdupz(buf);
    freez(buf);
    return clean;
}

static inline STRING *get_numeric_string(uint64_t n) {
    char buf[UINT64_MAX_LENGTH];
    print_uint64(buf, n);
    return string_strdupz(buf);
}

// --------------------------------------------------------------------------------------------------------------------
// Dynamic target lifecycle: admission, overflow, retirement

time_t retire_targets_after_s = RETIRE_TARGETS_AFTER_DEFAULT_S;
size_t max_targets_per_family = MAX_TARGETS_PER_FAMILY_DEFAULT;

#if (PROCESSES_HAVE_UID == 1)
struct target *users_root_target = NULL;
#endif
#if (PROCESSES_HAVE_GID == 1)
struct target *groups_root_target = NULL;
#endif
#if (PROCESSES_HAVE_SID == 1)
struct target *sids_root_target = NULL;
#endif
#if (PROCESSES_HAVE_SERVICE == 1)
struct target *services_root_target = NULL;
#endif
struct target *apps_groups_root_target = NULL;

struct target_family target_families[TARGET_FAMILY_MAX] = {
    [TARGET_FAMILY_APPS] = {
        .root = &apps_groups_root_target, .type = "app", .lbl_name = "app_group",
        .title = "Applications Groups", .enabled = NULL,
    },
#if (PROCESSES_HAVE_UID == 1)
    [TARGET_FAMILY_UID] = {
        .root = &users_root_target, .type = "user", .lbl_name = "user",
        .title = "User", .enabled = &enable_users_charts,
    },
#endif
#if (PROCESSES_HAVE_GID == 1)
    [TARGET_FAMILY_GID] = {
        .root = &groups_root_target, .type = "usergroup", .lbl_name = "user_group",
        .title = "User Group", .enabled = &enable_groups_charts,
    },
#endif
#if (PROCESSES_HAVE_SID == 1)
    [TARGET_FAMILY_SID] = {
        .root = &sids_root_target, .type = "user", .lbl_name = "user",
        .title = "User Processes", .enabled = &enable_users_charts,
    },
#endif
#if (PROCESSES_HAVE_SERVICE == 1)
    [TARGET_FAMILY_SERVICE] = {
        .root = &services_root_target, .type = "service", .lbl_name = "service",
        .title = "Windows Service", .enabled = &enable_services_charts,
    },
#endif
};

// Chart ids are built from clean_name, and get_clean_name() is lossy, so two
// identities can map to the same id. Give a new dynamic target a clean name no
// other reported target of its family uses, so they never write the same chart.
static void target_set_unique_clean_name(struct target *root, struct target *w) {
    STRING *base = get_clean_name(w->name);
    STRING *candidate = string_dup(base);

    for(size_t suffix = 2; ; suffix++) {
        bool taken = false;
        for(struct target *t = root; t ; t = t->next) {
            if(t != w && !t->target && t->clean_name == candidate) {
                taken = true;
                break;
            }
        }

        if(!taken)
            break;

        size_t len = string_strlen(base) + 1 + UINT64_MAX_LENGTH;
        char *buf = mallocz(len);
        snprintfz(buf, len - 1, "%s_%zu", string2str(base), suffix);
        string_freez(candidate);
        candidate = string_strdupz(buf);
        freez(buf);
    }

    string_freez(base);
    w->clean_name = candidate;
}

// Link a newly created dynamic target into its family. Stamps it active, so a
// target is never retired before it had the chance to be referenced.
static struct target *target_family_link(TARGET_FAMILY family, struct target *w) {
    struct target_family *f = &target_families[family];

    target_set_unique_clean_name(*f->root, w);

    w->referenced_iteration = global_iterations_counter;
    w->last_active_ut = now_monotonic_usec();

    w->next = *f->root;
    *f->root = w;

    if(!w->overflow) {
        f->dynamic++;
        f->generation++;
    }

    return w;
}

// The family is at its cap: new identities aggregate into a single target that
// identity lookups never match, so it cannot capture a real uid 0 or a real
// process called "other".
static struct target *target_family_overflow(TARGET_FAMILY family, TARGET_TYPE type) {
    struct target_family *f = &target_families[family];

    // logged once per family: under sustained churn the family stays at the cap,
    // and every retirement would otherwise log it again
    if(!f->cap_logged) {
        f->cap_logged = true;
        netdata_log_info("APPS: '%s' charts reached the limit of %zu targets; "
                         "new identities are aggregated into '%s.other' until idle targets are retired "
                         "(plugin options 'max-targets' and 'retire-targets-after')",
                         f->type, max_targets_per_family, f->type);
    }

    if(f->overflow)
        return f->overflow;

    struct target *w = callocz(sizeof(struct target), 1);
    w->type = type;
    w->overflow = true;
    w->id = string_strdupz("other");
    w->name = string_strdupz("other");
#if (PROCESSES_HAVE_SID == 1)
    if(type == TARGET_TYPE_SID)
        w->sid_name = string_dup(w->name);
#endif
#if (PROCESSES_HAVE_SERVICE == 1)
    if(type == TARGET_TYPE_SERVICE)
        w->service_name = string_dup(w->name);
#endif

    f->overflow = target_family_link(family, w);
    return w;
}

static void target_free(struct target *w) {
    string_freez(w->id);
    string_freez(w->name);
    string_freez(w->clean_name);
    pid_match_cleanup(&w->match);
#if (PROCESSES_HAVE_SID == 1)
    string_freez(w->sid_name);
#endif
#if (PROCESSES_HAVE_SERVICE == 1)
    string_freez(w->service_name);
#endif
#if (PROCESSES_HAVE_FDS == 1)
    freez(w->target_fds);
#endif

    struct pid_on_target *pot = w->root_pid;
    while(pot) {
        struct pid_on_target *next = pot->next;
        freez(pot);
        pot = next;
    }

    freez(w);
}

static inline void target_mark_referenced(struct target *w, usec_t now_ut) {
    if(w) {
        w->referenced_iteration = global_iterations_counter;
        w->last_active_ut = now_ut;
    }
}

// Called after the exited processes are cleaned up: every target a remaining
// process points to - including the previous-iteration one - is in use.
void apps_targets_mark_referenced(void) {
    usec_t now_ut = now_monotonic_usec();

    for(struct pid_stat *p = root_of_pids(); p ; p = p->next) {
        target_mark_referenced(p->target, now_ut);
#if (PROCESSES_HAVE_SMAPS_ROLLUP == 1)
        target_mark_referenced(p->prev_target, now_ut);
#endif
#if (PROCESSES_HAVE_UID == 1)
        target_mark_referenced(p->uid_target, now_ut);
#endif
#if (PROCESSES_HAVE_GID == 1)
        target_mark_referenced(p->gid_target, now_ut);
#endif
#if (PROCESSES_HAVE_SID == 1)
        target_mark_referenced(p->sid_target, now_ut);
#endif
#if (PROCESSES_HAVE_SERVICE == 1)
        target_mark_referenced(p->service_target, now_ut);
#endif
    }
}

// Retire the dynamic targets no process has referenced for retire_targets_after_s:
// obsolete the charts they announced, unlink and free them. apps_groups.conf
// targets are permanent. Must run under apps_and_stdout_mutex, after
// apps_targets_mark_referenced(): no process can point to a retired target.
void apps_targets_retire(void) {
    usec_t now_ut = now_monotonic_usec();
    usec_t retire_after_ut = (usec_t)retire_targets_after_s * USEC_PER_SEC;

    for(size_t fi = 0; fi < TARGET_FAMILY_MAX ; fi++) {
        struct target_family *f = &target_families[fi];

        struct target **link = f->root;
        while(*link) {
            struct target *w = *link;

            if(w->type == TARGET_TYPE_APP_GROUP ||
                w->referenced_iteration == global_iterations_counter ||
                now_ut - w->last_active_ut < retire_after_ut) {
                link = &w->next;
                continue;
            }

            if(w->exposed)
                send_target_charts_obsolete(w, f);

            *link = w->next;

            if(w->overflow)
                f->overflow = NULL;
            else {
                f->dynamic--;
                f->generation++;
            }

            f->retired++;

            debug_log("retired '%s' target '%s'", f->type, string2str(w->name));
            target_free(w);
        }
    }
}

// A real target wins over its family's overflow target, which is also named
// "other": the overflow one is returned only when no real target matches.
struct target *find_target_by_name(struct target *base, const char *name) {
    struct target *overflow = NULL;
    for(struct target *t = base; t ; t = t->next) {
        if (string_strcmp(t->name, name) == 0) {
            if(!t->overflow)
                return t;
            overflow = t;
        }
    }

    return overflow;
}

// --------------------------------------------------------------------------------------------------------------------
// Process managers and aggregators

struct comm_list {
    APPS_MATCH match;
};

struct managed_list {
    size_t used;
    size_t size;
    struct comm_list *array;
};

static struct {
    struct managed_list managers;
    struct managed_list aggregators;
    struct managed_list interpreters;
} tree = {
    .managers = {
        .array = NULL,
        .size = 0,
        .used = 0,
    },
    .aggregators = {
        .array = NULL,
        .size = 0,
        .used = 0,
    }
};

static void managed_list_clear(struct managed_list *list) {
    for(size_t c = 0; c < list->used ; c++)
        pid_match_cleanup(&list->array[c].match);

    freez(list->array);
    list->array = NULL;
    list->used = 0;
    list->size = 0;
}

static void managed_list_add(struct managed_list *list, const char *s) {
    if(list->used >= list->size) {
        if(!list->size)
            list->size = 16;
        else
            list->size *= 2;
        list->array = reallocz(list->array, sizeof(*list->array) * list->size);
    }

    list->array[list->used++].match = pid_match_create(s);
}

static STRING *KernelAggregator = NULL;

void apps_managers_and_aggregators_init(void) {
    KernelAggregator = string_strdupz("kernel");

    managed_list_clear(&tree.managers);
#if defined(OS_LINUX)
    managed_list_add(&tree.managers, "init");                       // linux systems
    managed_list_add(&tree.managers, "systemd");                    // lxc containers and host systems (this also catches "systemd --user")
    managed_list_add(&tree.managers, "containerd-shim-runc-v2");    // docker containers
    managed_list_add(&tree.managers, "docker-init");                // docker containers
    managed_list_add(&tree.managers, "tini");                       // docker containers (https://github.com/krallin/tini)
    managed_list_add(&tree.managers, "dumb-init");                  // some docker containers use this
    managed_list_add(&tree.managers, "openrc-run.sh");              // openrc
    managed_list_add(&tree.managers, "crond");                      // linux crond
    managed_list_add(&tree.managers, "gnome-shell");                // gnome user applications
    managed_list_add(&tree.managers, "plasmashell");                // kde user applications
    managed_list_add(&tree.managers, "xfwm4");                      // xfce4 user applications
    managed_list_add(&tree.managers, "*python3*bin/yugabyted*");    // https://docs.yugabyte.com/preview/tutorials/quick-start/docker/#create-a-local-cluster
#elif defined(OS_WINDOWS)
    managed_list_add(&tree.managers, "wininit");
    managed_list_add(&tree.managers, "services");
    managed_list_add(&tree.managers, "explorer");
    managed_list_add(&tree.managers, "System");
#elif defined(OS_FREEBSD)
    managed_list_add(&tree.managers, "init");
#elif defined(OS_MACOS)
    managed_list_add(&tree.managers, "launchd");
    managed_list_add(&tree.managers, "sshd-session");   // per-connection ssh session managers
#endif

#if defined(OS_WINDOWS)
    managed_list_add(&tree.managers, "netdata");
#else
    managed_list_add(&tree.managers, "spawn-plugins");
#endif

    managed_list_clear(&tree.aggregators);
#if defined(OS_LINUX)
    managed_list_add(&tree.aggregators, "kthread");
#elif defined(OS_WINDOWS)
#elif defined(OS_FREEBSD)
    managed_list_add(&tree.aggregators, "kernel");
#elif defined(OS_MACOS)
#endif

    managed_list_clear(&tree.interpreters);
    managed_list_add(&tree.interpreters, "python");
    managed_list_add(&tree.interpreters, "python2");
    managed_list_add(&tree.interpreters, "python3");
    managed_list_add(&tree.interpreters, "sh");
    managed_list_add(&tree.interpreters, "bash");
    // the remaining login shells shipped with macOS (/etc/shells) - a shell
    // that is a tree root (e.g. an SSH session) must group by its comm, not
    // by the path-derived Apple system bucket
    managed_list_add(&tree.interpreters, "zsh");
    managed_list_add(&tree.interpreters, "dash");
    managed_list_add(&tree.interpreters, "csh");
    managed_list_add(&tree.interpreters, "tcsh");
    managed_list_add(&tree.interpreters, "ksh");
    managed_list_add(&tree.interpreters, "node");
    managed_list_add(&tree.interpreters, "perl");
}

bool is_process_a_manager(struct pid_stat *p) {
    for(size_t c = 0; c < tree.managers.used ; c++) {
        if(pid_match_check(p, &tree.managers.array[c].match))
            return true;
    }

    return false;
}

bool is_process_an_aggregator(struct pid_stat *p) {
    for(size_t c = 0; c < tree.aggregators.used ; c++) {
        if(pid_match_check(p, &tree.aggregators.array[c].match))
            return true;
    }

    return false;
}

bool is_process_an_interpreter(struct pid_stat *p) {
    for(size_t c = 0; c < tree.interpreters.used ; c++) {
        if(pid_match_check(p, &tree.interpreters.array[c].match))
            return true;
    }

    return false;
}

// --------------------------------------------------------------------------------------------------------------------
// Tree

struct target *get_tree_target(struct pid_stat *p) {
//    // skip fast all the children that are more than 3 levels down
//    while(p->parent && p->parent->pid != INIT_PID && p->parent->parent && p->parent->parent->parent)
//        p = p->parent;

    // keep the children of INIT_PID, and process orchestrators
    while(p->parent && p->parent->pid != INIT_PID && p->parent->pid != 0 && !p->parent->is_manager)
        p = p->parent;

    // merge all processes into process aggregators
    STRING *search_for = NULL;
    if((p->ppid == 0 && p->pid != INIT_PID) || (p->parent && p->parent->is_aggregator)) {
        search_for = string_dup(KernelAggregator);
    }
    else {
        // OS-specific naming (macOS: derived from the executable path)
        search_for = apps_os_tree_target_name(p);

        if(!search_for) {
#if (PROCESSES_HAVE_COMM_AND_NAME == 1)
            search_for = string_dup(p->name ? p->name : p->comm);
#else
            search_for = string_dup(p->comm);
#endif
        }
    }

    // find an existing target with the required name
    struct target *w;
    for(w = apps_groups_root_target; w ; w = w->next) {
        if (w->name == search_for && !w->overflow) {
            string_freez(search_for);
            return w;
        }
    }

    if(target_family_at_cap(TARGET_FAMILY_APPS)) {
        string_freez(search_for);
        return target_family_overflow(TARGET_FAMILY_APPS, TARGET_TYPE_TREE);
    }

    w = callocz(sizeof(struct target), 1);
    w->type = TARGET_TYPE_TREE;
    w->match.starts_with = w->match.ends_with = false;
    w->match.compare = string_dup(search_for);
    w->match.pattern = NULL;
    w->id = search_for;
    w->name = string_dup(search_for);

    return target_family_link(TARGET_FAMILY_APPS, w);
}

// --------------------------------------------------------------------------------------------------------------------
// Users

#if (PROCESSES_HAVE_UID == 1)
struct target *get_uid_target(uid_t uid) {
    struct target *w;
    for(w = users_root_target ; w ; w = w->next)
        if(w->uid == uid && !w->overflow) return w;

    if(target_family_at_cap(TARGET_FAMILY_UID))
        return target_family_overflow(TARGET_FAMILY_UID, TARGET_TYPE_UID);

    w = callocz(sizeof(struct target), 1);
    w->type = TARGET_TYPE_UID;
    w->uid = uid;
    w->id = get_numeric_string(uid);

    CACHED_USERNAME cu = cached_username_get_by_uid(uid);
    w->name = string_dup(cu.username);
    cached_username_release(cu);

    target_family_link(TARGET_FAMILY_UID, w);

    debug_log("added uid %u ('%s') target", w->uid, string2str(w->name));

    return w;
}
#endif

// --------------------------------------------------------------------------------------------------------------------
// Groups

#if (PROCESSES_HAVE_GID == 1)
struct target *get_gid_target(gid_t gid) {
    struct target *w;
    for(w = groups_root_target ; w ; w = w->next)
        if(w->gid == gid && !w->overflow) return w;

    if(target_family_at_cap(TARGET_FAMILY_GID))
        return target_family_overflow(TARGET_FAMILY_GID, TARGET_TYPE_GID);

    w = callocz(sizeof(struct target), 1);
    w->type = TARGET_TYPE_GID;
    w->gid = gid;
    w->id = get_numeric_string(gid);

    CACHED_GROUPNAME cg = cached_groupname_get_by_gid(gid);
    w->name = string_dup(cg.groupname);
    cached_groupname_release(cg);

    target_family_link(TARGET_FAMILY_GID, w);

    debug_log("added gid %u ('%s') target", w->gid, string2str(w->name));

    return w;
}
#endif

// --------------------------------------------------------------------------------------------------------------------
// SID

#if (PROCESSES_HAVE_SID == 1)
struct target *get_sid_target(STRING *sid_name) {
    struct target *w;
    for(w = sids_root_target ; w ; w = w->next)
        if(w->sid_name == sid_name && !w->overflow) return w;

    if(target_family_at_cap(TARGET_FAMILY_SID))
        return target_family_overflow(TARGET_FAMILY_SID, TARGET_TYPE_SID);

    w = callocz(sizeof(struct target), 1);
    w->type = TARGET_TYPE_SID;
    w->sid_name = string_dup(sid_name);
    w->id = string_dup(sid_name);
    w->name = string_dup(sid_name);

    target_family_link(TARGET_FAMILY_SID, w);

    debug_log("added sid %s ('%s') target", string2str(w->sid_name), string2str(w->name));

    return w;
}
#endif

// --------------------------------------------------------------------------------------------------------------------
// Service

#if (PROCESSES_HAVE_SERVICE == 1)
struct target *get_service_target(STRING *service_name) {
    struct target *w;
    for(w = services_root_target ; w ; w = w->next)
        if(w->service_name == service_name && !w->overflow) return w;

    if(target_family_at_cap(TARGET_FAMILY_SERVICE))
        return target_family_overflow(TARGET_FAMILY_SERVICE, TARGET_TYPE_SERVICE);

    w = callocz(sizeof(struct target), 1);
    w->type = TARGET_TYPE_SERVICE;
    w->service_name = string_dup(service_name);
    w->id = string_dup(service_name);
    w->name = string_dup(service_name);

    target_family_link(TARGET_FAMILY_SERVICE, w);

    debug_log("added service '%s' target", string2str(w->service_name));

    return w;
}
#endif

// --------------------------------------------------------------------------------------------------------------------
// apps_groups.conf

// find or create a new target
// there are targets that are just aggregated to other target (the second argument)
static struct target *get_apps_groups_target(const char *comm, struct target *target, const char *name) {
    APPS_MATCH match = pid_match_create(comm);
    STRING *name_lookup = string_strdupz(name);

    // find if it already exists
    struct target *w, *last = apps_groups_root_target;
    for(w = apps_groups_root_target ; w ; w = w->next) {
        if(w->id == match.compare) {
            pid_match_cleanup(&match);
            string_freez(name_lookup);
            return w;
        }

        last = w;
    }

    // find an existing target
    if(unlikely(!target)) {
        for(target = apps_groups_root_target ; target ; target = target->next) {
            if(!target->target && name_lookup == target->name)
                break;
        }
    }

    if(target && target->target)
        fatal("Internal Error: request to link process '%s' to target '%s' which is linked to target '%s'",
            comm, string2str(target->id), string2str(target->target->id));

    w = callocz(sizeof(struct target), 1);
    w->type = TARGET_TYPE_APP_GROUP;
    w->match = match;
    w->id = string_dup(w->match.compare);

    if(unlikely(!target))
        w->name = string_dup(name_lookup); // copy the name
    else
        w->name = string_dup(w->id); // copy the id

    // dots are used to distinguish chart type and id in streaming, so we should replace them
    w->clean_name = get_clean_name(w->name);

    if(w->match.starts_with && w->match.ends_with)
        proc_pid_cmdline_is_needed = true;

    w->target = target;

    // append it, to maintain the order in apps_groups.conf
    if(last) last->next = w;
    else apps_groups_root_target = w;

    debug_log("ADDING TARGET ID '%s', process name '%s' (%s), aggregated on target '%s'"
              , string2str(w->id)
              , string2str(w->match.compare)
              , (w->match.starts_with && w->match.ends_with) ? "substring" : ((w->match.starts_with) ? "prefix" : ((w->match.ends_with) ? "suffix" : "exact"))
              , w->target?w->target->name:w->name
    );

    string_freez(name_lookup);

    return w;
}

// read the apps_groups.conf file
int read_apps_groups_conf(const char *path, const char *file) {
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
        if(!name || !*name || *name == '#') continue;

        if(strcmp(name, "managers") == 0) {
            if(words == 2 && strcmp(procfile_lineword(ff, line, 1), "clear") == 0)
                managed_list_clear(&tree.managers);

            for(word = 1; word < words ;word++) {
                char *s = procfile_lineword(ff, line, word);
                if (!s || !*s) continue;
                if (*s == '#') break;

                managed_list_add(&tree.managers, s);
            }

            // done with managers, proceed to next line
            continue;
        }

        if(strcmp(name, "interpreters") == 0) {
            if(words == 2 && strcmp(procfile_lineword(ff, line, 1), "clear") == 0)
                managed_list_clear(&tree.interpreters);

            for(word = 1; word < words ;word++) {
                char *s = procfile_lineword(ff, line, word);
                if (!s || !*s) continue;
                if (*s == '#') break;

                managed_list_add(&tree.interpreters, s);
            }

            // done with managers, proceed to the next line
            continue;
        }

        // find a possibly existing target
        struct target *w = NULL;

        // loop through all words, skipping the first one (the name)
        for(word = 1; word < words ;word++) {
            char *s = procfile_lineword(ff, line, word);
            if(!s || !*s) continue;
            if(*s == '#') break;

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
    return 0;
}
