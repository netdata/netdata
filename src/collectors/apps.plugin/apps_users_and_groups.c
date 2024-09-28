// SPDX-License-Identifier: GPL-3.0-or-later

#include "apps_plugin.h"

// ----------------------------------------------------------------------------
// read users and groups from files

enum user_or_group_id_type {
    USER_ID,
    GROUP_ID
};

struct user_or_group_ids {
    enum user_or_group_id_type type;

    avl_tree_type index;
    struct user_or_group_id *root;

    char filename[FILENAME_MAX + 1];
};

#if (PROCESSES_HAVE_UID == 1)
static int user_id_compare(void* a, void* b) {
    if(((struct user_or_group_id *)a)->id.uid < ((struct user_or_group_id *)b)->id.uid)
        return -1;

    else if(((struct user_or_group_id *)a)->id.uid > ((struct user_or_group_id *)b)->id.uid)
        return 1;

    else
        return 0;
}

static struct user_or_group_ids all_user_ids = {
    .type = USER_ID,

    .index = {
        NULL,
        user_id_compare
    },

    .root = NULL,

    .filename = "",
};
#endif

#if (PROCESSES_HAVE_GID == 1)
static int group_id_compare(void* a, void* b) {
    if(((struct user_or_group_id *)a)->id.gid < ((struct user_or_group_id *)b)->id.gid)
        return -1;

    else if(((struct user_or_group_id *)a)->id.gid > ((struct user_or_group_id *)b)->id.gid)
        return 1;

    else
        return 0;
}

static struct user_or_group_ids all_group_ids = {
    .type = GROUP_ID,

    .index = {
        NULL,
        group_id_compare
    },

    .root = NULL,

    .filename = "",
};
#endif

int file_changed(const struct stat *statbuf __maybe_unused, struct timespec *last_modification_time __maybe_unused) {
#if defined(OS_MACOS)
    return 0;
#else
    if(likely(statbuf->st_mtim.tv_sec == last_modification_time->tv_sec &&
               statbuf->st_mtim.tv_nsec == last_modification_time->tv_nsec)) return 0;

    last_modification_time->tv_sec = statbuf->st_mtim.tv_sec;
    last_modification_time->tv_nsec = statbuf->st_mtim.tv_nsec;

    return 1;
#endif
}

#if (PROCESSES_HAVE_UID == 1) || (PROCESSES_HAVE_GID == 1)
static int read_user_or_group_ids(struct user_or_group_ids *ids, struct timespec *last_modification_time) {
    struct stat statbuf;
    if(unlikely(stat(ids->filename, &statbuf)))
        return 1;
    else
        if(likely(!file_changed(&statbuf, last_modification_time))) return 0;

    procfile *ff = procfile_open(ids->filename, " :\t", PROCFILE_FLAG_DEFAULT);
    if(unlikely(!ff)) return 1;

    ff = procfile_readall(ff);
    if(unlikely(!ff)) return 1;

    size_t line, lines = procfile_lines(ff);

    for(line = 0; line < lines ;line++) {
        size_t words = procfile_linewords(ff, line);
        if(unlikely(words < 3)) continue;

        char *name = procfile_lineword(ff, line, 0);
        if(unlikely(!name || !*name)) continue;

        char *id_string = procfile_lineword(ff, line, 2);
        if(unlikely(!id_string || !*id_string)) continue;


        struct user_or_group_id *user_or_group_id = callocz(1, sizeof(struct user_or_group_id));

#if (PROCESSES_HAVE_UID == 1)
        if(ids->type == USER_ID)
            user_or_group_id->id.uid = (uid_t) str2ull(id_string, NULL);
#endif
#if (PROCESSES_HAVE_GID == 1)
        if(ids->type == GROUP_ID)
            user_or_group_id->id.gid = (uid_t) str2ull(id_string, NULL);
#endif

        user_or_group_id->name = strdupz(name);
        user_or_group_id->updated = 1;

        struct user_or_group_id *existing_user_id = NULL;

        if(likely(ids->root))
            existing_user_id = (struct user_or_group_id *)avl_search(&ids->index, (avl_t *) user_or_group_id);

        if(unlikely(existing_user_id)) {
            freez(existing_user_id->name);
            existing_user_id->name = user_or_group_id->name;
            existing_user_id->updated = 1;
            freez(user_or_group_id);
        }
        else {
            if(unlikely(avl_insert(&ids->index, (avl_t *) user_or_group_id) != (void *) user_or_group_id)) {
                netdata_log_error("INTERNAL ERROR: duplicate indexing of id during realloc");
            }

            user_or_group_id->next = ids->root;
            ids->root = user_or_group_id;
        }
    }

    procfile_close(ff);

    // remove unused ids
    struct user_or_group_id *user_or_group_id = ids->root, *prev_user_id = NULL;

    while(user_or_group_id) {
        if(unlikely(!user_or_group_id->updated)) {
            if(unlikely((struct user_or_group_id *)avl_remove(&ids->index, (avl_t *) user_or_group_id) != user_or_group_id))
                netdata_log_error("INTERNAL ERROR: removal of unused id from index, removed a different id");

            if(prev_user_id)
                prev_user_id->next = user_or_group_id->next;
            else
                ids->root = user_or_group_id->next;

            freez(user_or_group_id->name);
            freez(user_or_group_id);

            if(prev_user_id)
                user_or_group_id = prev_user_id->next;
            else
                user_or_group_id = ids->root;
        }
        else {
            user_or_group_id->updated = 0;

            prev_user_id = user_or_group_id;
            user_or_group_id = user_or_group_id->next;
        }
    }

    return 0;
}
#endif

#if (PROCESSES_HAVE_UID == 1)
struct user_or_group_id *user_id_find(struct user_or_group_id *user_id_to_find) {
    if(*netdata_configured_host_prefix) {
        static struct timespec last_passwd_modification_time;
        int ret = read_user_or_group_ids(&all_user_ids, &last_passwd_modification_time);

        if(likely(!ret && all_user_ids.index.root))
            return (struct user_or_group_id *)avl_search(&all_user_ids.index, (avl_t *)user_id_to_find);
    }

    return NULL;
}
#endif

#if (PROCESSES_HAVE_GID == 1)
struct user_or_group_id *group_id_find(struct user_or_group_id *group_id_to_find) {
    if(*netdata_configured_host_prefix) {
        static struct timespec last_group_modification_time;
        int ret = read_user_or_group_ids(&all_group_ids, &last_group_modification_time);

        if(likely(!ret && all_group_ids.index.root))
            return (struct user_or_group_id *)avl_search(&all_group_ids.index, (avl_t *) &group_id_to_find);
    }

    return NULL;
}
#endif

void apps_users_and_groups_init(void) {
#if (PROCESSES_HAVE_UID == 1)
    snprintfz(all_user_ids.filename, FILENAME_MAX, "%s/etc/passwd", netdata_configured_host_prefix);
    debug_log("passwd file: '%s'", all_user_ids.filename);
#endif

#if (PROCESSES_HAVE_GID == 1)
    snprintfz(all_group_ids.filename, FILENAME_MAX, "%s/etc/group", netdata_configured_host_prefix);
    debug_log("group file: '%s'", all_group_ids.filename);
#endif
}

