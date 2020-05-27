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

/**
 * Am I running as Root
 *
 * Verify the user that is running the collector.
 *
 * @return It returns 1 for root and 0 otherwise.
 */
int am_i_running_as_root() {
    uid_t uid = getuid(), euid = geteuid();

    if(uid == 0 || euid == 0) {
        return 1;
    }

    return 0;
}

/**
 * Reset the target values
 *
 * @param root the pointer to the chain that will be reseted.
 *
 * @return it returns the number of structures that was reseted.
 */
size_t zero_all_targets(struct target *root) {
    struct target *w;
    size_t count = 0;

    for (w = root; w ; w = w->next) {
        count++;

        w->minflt = 0;
        w->majflt = 0;
        w->utime = 0;
        w->stime = 0;
        w->gtime = 0;
        w->cminflt = 0;
        w->cmajflt = 0;
        w->cutime = 0;
        w->cstime = 0;
        w->cgtime = 0;
        w->num_threads = 0;
        // w->rss = 0;
        w->processes = 0;

        w->status_vmsize = 0;
        w->status_vmrss = 0;
        w->status_vmshared = 0;
        w->status_rssfile = 0;
        w->status_rssshmem = 0;
        w->status_vmswap = 0;

        w->io_logical_bytes_read = 0;
        w->io_logical_bytes_written = 0;
        // w->io_read_calls = 0;
        // w->io_write_calls = 0;
        w->io_storage_bytes_read = 0;
        w->io_storage_bytes_written = 0;
        // w->io_cancelled_write_bytes = 0;

        // zero file counters
        if(w->target_fds) {
            memset(w->target_fds, 0, sizeof(int) * w->target_fds_size);
            w->openfiles = 0;
            w->openpipes = 0;
            w->opensockets = 0;
            w->openinotifies = 0;
            w->openeventfds = 0;
            w->opentimerfds = 0;
            w->opensignalfds = 0;
            w->openeventpolls = 0;
            w->openother = 0;
        }

        w->collected_starttime = 0;
        w->uptime_min = 0;
        w->uptime_sum = 0;
        w->uptime_max = 0;

        if(unlikely(w->root_pid)) {
            struct pid_on_target *pid_on_target_to_free, *pid_on_target = w->root_pid;

            while(pid_on_target) {
                pid_on_target_to_free = pid_on_target;
                pid_on_target = pid_on_target->next;
                free(pid_on_target_to_free);
            }

            w->root_pid = NULL;
        }
    }

    return count;
}

/**
 * Clean the allocated structures
 *
 * @param apps_groups_root_target the pointer to be cleaned.
 */
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
