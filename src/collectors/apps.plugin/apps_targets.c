// SPDX-License-Identifier: GPL-3.0-or-later

#include "apps_plugin.h"

// ----------------------------------------------------------------------------
// apps_groups.conf
// aggregate all processes in groups, to have a limited number of dimensions

struct target *get_users_target(uid_t uid) {
    struct target *w;
    for(w = users_root_target ; w ; w = w->next)
        if(w->uid == uid) return w;

    w = callocz(sizeof(struct target), 1);
    snprintfz(w->compare, sizeof(w->compare), "%u", uid);
    w->comparehash = simple_hash(w->compare);
    w->comparelen = strlen(w->compare);

    snprintfz(w->id, MAX_NAME, "%u", uid);
    w->idhash = simple_hash(w->id);

    struct user_or_group_id user_id_to_find = {
        .id = {
            .uid = uid,
        }
    };
    struct user_or_group_id *user_or_group_id = user_id_find(&user_id_to_find);

    if(user_or_group_id && user_or_group_id->name && *user_or_group_id->name)
        snprintfz(w->name, MAX_NAME, "%s", user_or_group_id->name);

    else {
        struct passwd *pw = getpwuid(uid);
        if(!pw || !pw->pw_name || !*pw->pw_name)
            snprintfz(w->name, MAX_NAME, "%u", uid);
        else
            snprintfz(w->name, MAX_NAME, "%s", pw->pw_name);
    }

    strncpyz(w->clean_name, w->name, MAX_NAME);
    netdata_fix_chart_name(w->clean_name);

    w->uid = uid;

    w->next = users_root_target;
    users_root_target = w;

    debug_log("added uid %u ('%s') target", w->uid, w->name);

    return w;
}

struct target *get_groups_target(gid_t gid) {
    struct target *w;
    for(w = groups_root_target ; w ; w = w->next)
        if(w->gid == gid) return w;

    w = callocz(sizeof(struct target), 1);
    snprintfz(w->compare, sizeof(w->compare), "%u", gid);
    w->comparehash = simple_hash(w->compare);
    w->comparelen = strlen(w->compare);

    snprintfz(w->id, MAX_NAME, "%u", gid);
    w->idhash = simple_hash(w->id);

    struct user_or_group_id group_id_to_find = {
        .id = {
            .gid = gid,
        }
    };
    struct user_or_group_id *group_id = group_id_find(&group_id_to_find);

    if(group_id && group_id->name && *group_id->name) {
        snprintfz(w->name, MAX_NAME, "%s", group_id->name);
    }
    else {
        struct group *gr = getgrgid(gid);
        if(!gr || !gr->gr_name || !*gr->gr_name)
            snprintfz(w->name, MAX_NAME, "%u", gid);
        else
            snprintfz(w->name, MAX_NAME, "%s", gr->gr_name);
    }

    strncpyz(w->clean_name, w->name, MAX_NAME);
    netdata_fix_chart_name(w->clean_name);

    w->gid = gid;

    w->next = groups_root_target;
    groups_root_target = w;

    debug_log("added gid %u ('%s') target", w->gid, w->name);

    return w;
}

// find or create a new target
// there are targets that are just aggregated to other target (the second argument)
static struct target *get_apps_groups_target(const char *id, struct target *target, const char *name) {
    bool tdebug = false, thidden = target ? target->hidden : false, ends_with = false;
    const char *nid = id;

    // extract the options
    while(nid[0] == '-' || nid[0] == '+' || nid[0] == '*') {
        if(nid[0] == '-') thidden = 1;
        if(nid[0] == '+') tdebug = 1;
        if(nid[0] == '*') ends_with = 1;
        nid++;
    }
    uint32_t hash = simple_hash(id);

    // find if it already exists
    struct target *w, *last = apps_groups_root_target;
    for(w = apps_groups_root_target ; w ; w = w->next) {
        if(w->idhash == hash && strncmp(nid, w->id, MAX_NAME) == 0)
            return w;

        last = w;
    }

    // find an existing target
    if(unlikely(!target)) {
        while(*name == '-') {
            if(*name == '-') thidden = true;
            name++;
        }

        for(target = apps_groups_root_target ; target != NULL ; target = target->next) {
            if(!target->target && strcmp(name, target->name) == 0)
                break;
        }

        if(unlikely(debug_enabled)) {
            if(unlikely(target))
                debug_log("REUSING TARGET NAME '%s' on ID '%s'", target->name, target->id);
            else
                debug_log("NEW TARGET NAME '%s' on ID '%s'", name, id);
        }
    }

    if(target && target->target)
        fatal("Internal Error: request to link process '%s' to target '%s' which is linked to target '%s'", id, target->id, target->target->id);

    w = callocz(sizeof(struct target), 1);
    strncpyz(w->id, nid, MAX_NAME);
    w->idhash = simple_hash(w->id);

    if(unlikely(!target))
        // copy the name
        strncpyz(w->name, name, MAX_NAME);
    else
        // copy the id
        strncpyz(w->name, nid, MAX_NAME);

    // dots are used to distinguish chart type and id in streaming, so we should replace them
    strncpyz(w->clean_name, w->name, MAX_NAME);
    netdata_fix_chart_name(w->clean_name);
    for (char *d = w->clean_name; *d; d++) {
        if (*d == '.')
            *d = '_';
    }

    strncpyz(w->compare, nid, sizeof(w->compare) - 1);
    size_t len = strlen(w->compare);
    if(w->compare[len - 1] == '*') {
        w->compare[len - 1] = '\0';
        w->starts_with = true;
    }
    w->ends_with = ends_with;

    if(w->starts_with && w->ends_with)
        proc_pid_cmdline_is_needed = true;

    w->comparehash = simple_hash(w->compare);
    w->comparelen = strlen(w->compare);

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
              , w->id
              , w->compare, (w->starts_with && w->ends_with)?"substring":((w->starts_with)?"prefix":((w->ends_with)?"suffix":"exact"))
                  , w->target?w->target->name:w->name
              , (w->hidden)?"hidden":"-"
              , (w->debug_enabled)?"debug":"-"
    );

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
