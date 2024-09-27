// SPDX-License-Identifier: GPL-3.0-or-later

#include "apps_plugin.h"

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

// ----------------------------------------------------------------------------
// apps_groups.conf
// aggregate all processes in groups, to have a limited number of dimensions

#if (PROCESSES_HAVE_UID == 1)
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

#if (PROCESSES_HAVE_GID == 1)
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

static void id_cleanup_txt(char *buf) {
    char *s = buf;
    while(*s) {
        if (!isalnum((uint8_t)*s) && *s != '-' && *s != '_')
            *s = ' ';
        s++;
    }

    trim_all(buf);

    s = buf;
    while(*s) {
        if (isspace((uint8_t)*s))
            *s = '-';
        s++;
    }
}

static STRING *id_cleanup_string(STRING *s) {
    char buf[string_strlen(s) + 1];
    memcpy(buf, string2str(s), sizeof(buf));
    id_cleanup_txt(buf);
    return string_strdupz(buf);
}

static STRING *comm_from_cmdline(STRING *comm, STRING *cmdline) {
    if(!cmdline) return id_cleanup_string(comm);

    char buf_cmd[string_strlen(cmdline) + 1];
    memcpy(buf_cmd, string2str(cmdline), sizeof(buf_cmd));

    char *start = strstr(buf_cmd, string2str(comm));
    if(start) {
        char *end = start + string_strlen(comm);
        while(*end && !isspace((uint8_t)*end) && *end != '/' && *end != '\\') end++;
        *end = '\0';

        id_cleanup_txt(start);
        return string_strdupz(start);
    }

    return id_cleanup_string(comm);
}

struct {
    const char *txt;
    STRING *comm;
} process_orchestrators[] = {
#if defined(OS_LINUX)
    { "systemd", NULL, },
#endif
#if defined(OS_WINDOWS)
    { "System", NULL, },
    { "services", NULL, },
    { "wininit", NULL, },
#else
    { "init", NULL, },
#endif

    // terminator
    { NULL, NULL }
};

struct {
    const char *txt;
    STRING *comm;
} process_aggregators[] = {
#if defined(OS_LINUX)
    { "kthread", NULL, },
#endif

    // terminator
    { NULL, NULL }
};

static bool is_orchestrator(struct pid_stat *p) {
    for(size_t i = 0; process_orchestrators[i].comm ;i++) {
        if(p->comm == process_orchestrators[i].comm)
            return true;
    }

    return false;
}

static bool is_aggregator(struct pid_stat *p) {
    for(size_t i = 0; process_aggregators[i].comm ;i++) {
        if(p->comm == process_aggregators[i].comm)
            return true;
    }

    return false;
}

void orchestrators_and_aggregators_init(void) {
    for(size_t i = 0; process_orchestrators[i].txt ;i++)
        process_orchestrators[i].comm = string_strdupz(process_orchestrators[i].txt);

    for(size_t i = 0; process_aggregators[i].txt ;i++)
        process_aggregators[i].comm = string_strdupz(process_aggregators[i].txt);
}

struct target *get_tree_target(struct pid_stat *p) {
    // skip fast all the children that are more than 3 levels down
    while(p->parent && p->parent->parent && p->parent->parent->parent)
        p = p->parent;

    // keep the children of INIT_PID, and process orchestrators
    while(p->parent && p->parent->pid != INIT_PID && !is_orchestrator(p->parent))
        p = p->parent;

    // merge all processes into process aggregators
    if(p->parent && is_aggregator(p->parent))
        p = p->parent;

    struct target *w;
    for(w = tree_root_target; w ; w = w->next)
        if(w->pid_comm == p->comm) return w;

    w = callocz(sizeof(struct target), 1);
    w->type = TARGET_TYPE_TREE;
    w->pid_comm = string_dup(p->comm);
    w->id = string_dup(p->comm);
    w->name = comm_from_cmdline(p->comm, p->cmdline);
    w->clean_name = get_clean_name(w->name);

    w->next = tree_root_target;
    tree_root_target = w;

    return w;
}

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
                netdata_log_error("Cannot create target '%s' (line %zu, word %zu)", s, line, word);
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
    apps_groups_default_target->is_other = true;

    // allow the user to override group 'other'
    if(apps_groups_default_target->target)
        apps_groups_default_target = apps_groups_default_target->target;

    return 0;
}

struct target *find_target_by_name(struct target *base, const char *name) {
    struct target *t;
    for(t = base; t ; t = t->next) {
        if (string_strcmp(t->name, name) == 0)
            return t;
    }

    return NULL;
}
