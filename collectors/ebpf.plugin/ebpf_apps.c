// SPDX-License-Identifier: GPL-3.0-or-later

#include "ebpf_apps.h"

/*****************************************************************
 *
 *  INTERNAL FUNCTIONS
 *
 *****************************************************************/


/*****************************************************************
 *
 *  FUNCTIONS CALLED FROM COLLECTORS
 *
 *****************************************************************/

void clean_apps_groups_target(struct target *apps_groups_root_target) {
    struct target *current_target;
    while (apps_groups_root_target) {
        current_target = apps_groups_root_target;
        apps_groups_root_target = current_target->target;

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
struct target *get_apps_groups_target(struct target **apps_groups_root_target, const char *id,
                                           struct target *target, const char *name) {
    int tdebug = 0, thidden = target?target->hidden:0, ends_with = 0;
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
    struct target *w, *last = *apps_groups_root_target;
    for(w = *apps_groups_root_target ; w ; w = w->next) {
        if(w->idhash == hash && strncmp(nid, w->id, MAX_NAME) == 0)
            return w;

        last = w;
    }

    // find an existing target
    if(unlikely(!target)) {
        while(*name == '-') {
            if(*name == '-') thidden = 1;
            name++;
        }

        for(target = *apps_groups_root_target ; target != NULL ; target = target->next) {
            if(!target->target && strcmp(name, target->name) == 0)
                break;
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

    strncpyz(w->compare, nid, MAX_COMPARE_NAME);
    size_t len = strlen(w->compare);
    if(w->compare[len - 1] == '*') {
        w->compare[len - 1] = '\0';
        w->starts_with = 1;
    }
    w->ends_with = ends_with;

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
    else *apps_groups_root_target = w;

    return w;
}

/**
 * Read the apps_groups.conf file
 *
 * @param path
 * @param file
 *
 * @return It returns 0 on succcess and -1 otherwise
 */
int ebpf_read_apps_groups_conf(struct target **apps_groups_default_target, struct target **apps_groups_root_target,
                               const char *path, const char *file)
{
    char filename[FILENAME_MAX + 1];

    snprintfz(filename, FILENAME_MAX, "%s/apps_%s.conf", path, file);

    // ----------------------------------------

    procfile *ff = procfile_open(filename, " :\t", PROCFILE_FLAG_DEFAULT);
    if(!ff) return -1;

    procfile_set_quotes(ff, "'\"");

    ff = procfile_readall(ff);
    if(!ff)
        return -1;

    size_t line, lines = procfile_lines(ff);

    for (line = 0; line < lines ;line++) {
        size_t word, words = procfile_linewords(ff, line);
        if(!words) continue;

        char *name = procfile_lineword(ff, line, 0);
        if (!name || !*name) continue;

        // find a possibly existing target
        struct target *w = NULL;

        // loop through all words, skipping the first one (the name)
        for (word = 0; word < words ;word++) {
            char *s = procfile_lineword(ff, line, word);
            if (!s || !*s) continue;
            if (*s == '#') break;

            // is this the first word? skip it
            if (s == name) continue;

            // add this target
            struct target *n = get_apps_groups_target(apps_groups_root_target, s, w, name);
            if (!n) {
                error("Cannot create target '%s' (line %zu, word %zu)", s, line, word);
                continue;
            }

            // just some optimization
            // to avoid searching for a target for each process
            if (!w) w = n->target?n->target:n;
        }
    }

    procfile_close(ff);

    *apps_groups_default_target = get_apps_groups_target(apps_groups_root_target, "p+!o@w#e$i^r&7*5(-i)l-o_",
                                                         NULL, "other"); // match nothing
    if(!*apps_groups_default_target)
        fatal("Cannot create default target");

    struct target *ptr = *apps_groups_default_target;
    if (ptr->target)
        *apps_groups_default_target = ptr->target;
    /*
    // allow the user to override group 'other'
    if(*apps_groups_default_target.target)
        *apps_groups_default_target = *apps_groups_default_target.target;
        */

    return 0;
}
