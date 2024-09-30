// SPDX-License-Identifier: GPL-3.0-or-later

#include "apps_plugin.h"

pid_t INIT_PID = OS_INIT_PID;

static STRING *get_clean_name(STRING *name) {
    char buf[string_strlen(name) + 1];
    memcpy(buf, string2str(name), string_strlen(name) + 1);
    netdata_fix_chart_name(buf);
    for (char *d = buf; *d ; d++) {
        if (*d == '.') *d = '_';
    }
    return string_strdupz(buf);
}

static inline STRING *get_numeric_string(uint64_t n) {
    char buf[UINT64_MAX_LENGTH];
    print_uint64(buf, n);
    return string_strdupz(buf);
}

struct target *find_target_by_name(struct target *base, const char *name) {
    struct target *t;
    for(t = base; t ; t = t->next) {
        if (string_strcmp(t->name, name) == 0)
            return t;
    }

    return NULL;
}

// --------------------------------------------------------------------------------------------------------------------
// Tree

static inline STRING *comm_from_cmdline(STRING *comm, STRING *cmdline) {
    if(!cmdline) return sanitize_chart_meta_string(comm);

    const char *cl = string2str(cmdline);
    size_t len = string_strlen(cmdline);

    char buf_cmd[len + 1];
    // if it is enclosed in (), remove the parenthesis
    if(cl[0] == '(' && cl[len - 1] == ')') {
        memcpy(buf_cmd, &cl[1], len - 2);
        buf_cmd[len - 2] = '\0';
    }
    else
        memcpy(buf_cmd, cl, sizeof(buf_cmd));

    char *start = strstr(buf_cmd, string2str(comm));
    if(start) {
        char *end = start + string_strlen(comm);
        while(*end && !isspace((uint8_t)*end) && *end != '/' && *end != '\\') end++;
        *end = '\0';

        sanitize_chart_meta(start);
        return string_strdupz(start);
    }

    return sanitize_chart_meta_string(comm);
}

struct comm_list {
    STRING *comm;
};

struct managed_list {
    size_t used;
    size_t size;
    struct comm_list *array;
};

static struct {
    struct managed_list managers;
    struct managed_list aggregators;
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
        string_freez(list->array[c].comm);

    freez(list->array);
    list->array = NULL;
    list->used = 0;
    list->size = 0;
}

static void managed_list_add(struct managed_list *list, const char *s) {
    if(list->used >= list->size) {
        if(!list->size)
            list->size = 10;
        else
            list->size *= 2;
        list->array = reallocz(list->array, sizeof(*list->array) * list->size);
    }

    list->array[list->used++].comm = string_strdupz(s);
}

static STRING *KernelAggregator = NULL;

void apps_orchestrators_and_aggregators_init(void) {
    KernelAggregator = string_strdupz("kernel");

    managed_list_clear(&tree.managers);
#if defined(OS_LINUX)
    managed_list_add(&tree.managers, "init");               // linux systems
    managed_list_add(&tree.managers, "systemd");            // lxc containers and host systems (this also catches "systemd --user")
    managed_list_add(&tree.managers, "containerd-shim");    // docker containers
    managed_list_add(&tree.managers, "dumb-init");          // some docker containers use this
    managed_list_add(&tree.managers, "gnome-shell");        // gnome user applications
#elif defined(OS_WINDOWS)
    managed_list_add(&tree.managers, "System");
    managed_list_add(&tree.managers, "services");
    managed_list_add(&tree.managers, "wininit");
#elif defined(OS_FREEBSD)
    managed_list_add(&tree.managers, "init");
#elif defined(OS_MACOS)
    managed_list_add(&tree.managers, "launchd");
#endif

    managed_list_clear(&tree.aggregators);
#if defined(OS_LINUX)
    managed_list_add(&tree.aggregators, "kthread");
#elif defined(OS_WINDOWS)
#elif defined(OS_FREEBSD)
    managed_list_add(&tree.aggregators, "kernel");
#elif defined(OS_MACOS)
#endif
}

static inline bool is_orchestrator(struct pid_stat *p) {
    for(size_t c = 0; c < tree.managers.used ; c++) {
        if(p->comm == tree.managers.array[c].comm)
            return true;
    }

    return false;
}

static inline bool is_aggregator(struct pid_stat *p) {
    for(size_t c = 0; c < tree.aggregators.used ; c++) {
        if(p->comm == tree.aggregators.array[c].comm)
            return true;
    }

    return false;
}

struct target *get_tree_target(struct pid_stat *p) {
//    // skip fast all the children that are more than 3 levels down
//    while(p->parent && p->parent->pid != INIT_PID && p->parent->parent && p->parent->parent->parent)
//        p = p->parent;

    // keep the children of INIT_PID, and process orchestrators
    while(p->parent && p->parent->pid != INIT_PID && p->parent->pid != 0 && !is_orchestrator(p->parent))
        p = p->parent;

    // merge all processes into process aggregators
    STRING *search_for = string_dup(p->comm);
    bool aggregator = false;
    if((p->ppid == 0 && p->pid != INIT_PID) || (p->parent && is_aggregator(p->parent))) {
        aggregator = true;
        search_for = string_dup(KernelAggregator);
    }

    if(!aggregator) {
#if (PROCESSES_HAVE_COMM_AND_NAME == 1)
        search_for = sanitize_chart_meta_string(p->name ? p->name : p->comm);
#else
        search_for = comm_from_cmdline(p->comm, p->cmdline);
#endif
    }

    struct target *w;
    for(w = apps_groups_root_target; w ; w = w->next) {
        if (w->name == search_for) {
            string_freez(search_for);
            return w;
        }
    }

    w = callocz(sizeof(struct target), 1);
    w->type = TARGET_TYPE_TREE;
    w->starts_with = w->ends_with = false;
    w->compare = string_dup(p->comm);
    w->id = search_for;
    w->name = string_dup(search_for);
    w->clean_name = get_clean_name(w->name);

    w->next = apps_groups_root_target;
    apps_groups_root_target = w;

    return w;
}

// --------------------------------------------------------------------------------------------------------------------
// Users

#if (PROCESSES_HAVE_UID == 1)
struct target *users_root_target = NULL;

struct target *get_uid_target(uid_t uid) {
    struct target *w;
    for(w = users_root_target ; w ; w = w->next)
        if(w->uid == uid) return w;

    w = callocz(sizeof(struct target), 1);
    w->type = TARGET_TYPE_UID;
    w->uid = uid;
    w->id = get_numeric_string(uid);

    struct user_or_group_id user_id_to_find = {
        .id = {
            .uid = uid,
        }
    };
    struct user_or_group_id *user_or_group_id = user_id_find(&user_id_to_find);

    if(user_or_group_id && user_or_group_id->name && *user_or_group_id->name)
        w->name = string_strdupz(user_or_group_id->name);
    else {
        struct passwd *pw = getpwuid(uid);
        if(!pw || !pw->pw_name || !*pw->pw_name)
            w->name = get_numeric_string(uid);
        else
            w->name = string_strdupz(pw->pw_name);
    }

    w->clean_name = get_clean_name(w->name);

    w->next = users_root_target;
    users_root_target = w;

    debug_log("added uid %u ('%s') target", w->uid, string2str(w->name));

    return w;
}
#endif

// --------------------------------------------------------------------------------------------------------------------
// Groups

#if (PROCESSES_HAVE_GID == 1)
struct target *groups_root_target = NULL;

struct target *get_gid_target(gid_t gid) {
    struct target *w;
    for(w = groups_root_target ; w ; w = w->next)
        if(w->gid == gid) return w;

    w = callocz(sizeof(struct target), 1);
    w->type = TARGET_TYPE_GID;
    w->gid = gid;
    w->id = get_numeric_string(gid);

    struct user_or_group_id group_id_to_find = {
        .id = {
            .gid = gid,
        }
    };
    struct user_or_group_id *group_id = group_id_find(&group_id_to_find);

    if(group_id && group_id->name)
        w->name = string_strdupz(group_id->name);
    else {
        struct group *gr = getgrgid(gid);
        if(!gr || !gr->gr_name || !*gr->gr_name)
            w->name = get_numeric_string(gid);
        else
            w->name = string_strdupz(gr->gr_name);
    }

    w->clean_name = get_clean_name(w->name);

    w->next = groups_root_target;
    groups_root_target = w;

    debug_log("added gid %u ('%s') target", w->gid, w->name);

    return w;
}
#endif

// --------------------------------------------------------------------------------------------------------------------
// apps_groups.conf

struct target *apps_groups_root_target = NULL;

// find or create a new target
// there are targets that are just aggregated to other target (the second argument)
static struct target *get_apps_groups_target(const char *id, struct target *target, const char *name) {
    bool tdebug = false, thidden = target ? target->hidden : false, ends_with = false, starts_with = false;

    STRING *id_lookup = NULL;
    STRING *name_lookup = NULL;

    // extract the options from the id
    {
        size_t len = strlen(id);
        char buf[len + 1];
        memcpy(buf, id, sizeof(buf));

        if(buf[len - 1] == '*') {
            buf[--len] = '\0';
            starts_with = true;
        }

        const char *nid = buf;
        while (nid[0] == '-' || nid[0] == '+' || nid[0] == '*') {
            if (nid[0] == '-') thidden = true;
            if (nid[0] == '+') tdebug = true;
            if (nid[0] == '*') ends_with = true;
            nid++;
        }

        id_lookup = string_strdupz(nid);
    }

    // extract the options from the name
    {
        size_t len = strlen(name);
        char buf[len + 1];
        memcpy(buf, name, sizeof(buf));

        const char *nn = buf;
        while (nn[0] == '-' || nn[0] == '+') {
            if (nn[0] == '-') thidden = true;
            if (nn[0] == '+') tdebug = true;
            nn++;
        }

        name_lookup = string_strdupz(nn);
    }

    // find if it already exists
    struct target *w, *last = apps_groups_root_target;
    for(w = apps_groups_root_target ; w ; w = w->next) {
        if(w->id == id_lookup) {
            string_freez(id_lookup);
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
              id, string2str(target->id), string2str(target->target->id));

    w = callocz(sizeof(struct target), 1);
    w->type = TARGET_TYPE_APP_GROUP;
    w->compare = string_dup(id_lookup);
    w->starts_with = starts_with;
    w->ends_with = ends_with;
    w->id = string_dup(id_lookup);

    if(unlikely(!target))
        w->name = string_dup(name_lookup); // copy the name
    else
        w->name = string_dup(id_lookup); // copy the id

    // dots are used to distinguish chart type and id in streaming, so we should replace them
    w->clean_name = get_clean_name(w->name);

    if(w->starts_with && w->ends_with)
        proc_pid_cmdline_is_needed = true;

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
              , string2str(w->id)
              , string2str(w->compare)
              , (w->starts_with && w->ends_with)?"substring":((w->starts_with)?"prefix":((w->ends_with)?"suffix":"exact"))
              , w->target?w->target->name:w->name
              , (w->hidden)?"hidden":"-"
              , (w->debug_enabled)?"debug":"-"
    );

    string_freez(id_lookup);
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

    bool managers_reset_done = false;

    for(line = 0; line < lines ;line++) {
        size_t word, words = procfile_linewords(ff, line);
        if(!words) continue;

        char *name = procfile_lineword(ff, line, 0);
        if(!name || !*name) continue;

        if(strcmp(name, "managers") == 0) {
            if(!managers_reset_done) {
                managers_reset_done = true;
                managed_list_clear(&tree.managers);
            }

            for(word = 0; word < words ;word++) {
                char *s = procfile_lineword(ff, line, word);
                if (!s || !*s) continue;
                if (*s == '#') break;

                // is this the first word? skip it
                if(s == name) continue;

                managed_list_add(&tree.managers, s);
            }

            // done with managers, proceed to next line
            continue;
        }

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
    return 0;
}
