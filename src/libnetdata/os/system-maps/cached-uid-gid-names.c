// SPDX-License-Identifier: GPL-3.0-or-later

#include "cached-uid-gid-names.h"

// --------------------------------------------------------------------------------------------------------------------
// one implementation of an id -> name cache, used for uid -> username and gid -> groupname
// key is the id, value is the name (STRING)

// /etc/passwd and /etc/group are parsed into uint32_t ids, and the caches are keyed by uint32_t
_Static_assert(sizeof(uid_t) == sizeof(uint32_t) && (uid_t)-1 > 0, "uid_t must be an unsigned 32-bit integer");
_Static_assert(sizeof(gid_t) == sizeof(uint32_t) && (gid_t)-1 > 0, "gid_t must be an unsigned 32-bit integer");

typedef struct {
    uint32_t version;       // version 0 entries survive population and old-version cleanup
    uint32_t id;
    STRING *name;
} ID_NAME;

#define SIMPLE_HASHTABLE_KEY_TYPE uint32_t
#define SIMPLE_HASHTABLE_VALUE_TYPE ID_NAME *
#define SIMPLE_HASHTABLE_VALUE2KEY_FUNCTION id_name_to_id_ptr
#define SIMPLE_HASHTABLE_COMPARE_KEYS_FUNCTION compar_id_ptr
#define SIMPLE_HASHTABLE_NAME _ID_NAMES_CACHE
#include "libnetdata/simple_hashtable/simple_hashtable.h"

static uint32_t *id_name_to_id_ptr(ID_NAME *in) {
    return &in->id;
}

static bool compar_id_ptr(uint32_t *a, uint32_t *b) {
    return *a == *b;
}

// returns the name of the id, stored in buf, or NULL when it cannot be resolved
typedef const char *(*id_name_lookup_t)(uint32_t id, char *buf, size_t buf_size);

typedef struct {
    const char *what;               // for error messages
    id_name_lookup_t lookup;

    bool initialized;
    SPINLOCK spinlock;
    SIMPLE_HASHTABLE_ID_NAMES_CACHE ht;
    char lookup_buf[1024];          // used only under spinlock

    // refreshing from the host file
    struct {
        const char *filename;       // appended to the host prefix
        SPINLOCK spinlock;
        uint32_t version;
        struct timespec last_modification_time;
    } host_file;
} ID_NAMES_CACHE;

static void id_names_populate(ID_NAMES_CACHE *c, uint32_t id, const char *name, uint32_t version) {
    internal_fatal(!c->initialized, "%s cache needs to be initialized", c->what);
    if(!name || !*name) return;

    spinlock_lock(&c->spinlock);

    XXH64_hash_t hash = XXH3_64bits(&id, sizeof(id));
    SIMPLE_HASHTABLE_SLOT_ID_NAMES_CACHE *sl = simple_hashtable_get_slot_ID_NAMES_CACHE(&c->ht, hash, &id, true);
    ID_NAME *in = SIMPLE_HASHTABLE_SLOT_DATA(sl);
    if(!in || (in->version && version > in->version)) {
        internal_fatal(in && in->id != id, "invalid %s id matched from cache", c->what);

        if(in)
            string_freez(in->name);
        else
            in = callocz(1, sizeof(*in));

        in->version = version;
        in->id = id;
        in->name = string_strdupz(name);
        simple_hashtable_set_slot_ID_NAMES_CACHE(&c->ht, sl, hash, in);
    }

    spinlock_unlock(&c->spinlock);
}

// returns the cached entry, with its name acquired (string_dup) for the caller
static ID_NAME id_names_get(ID_NAMES_CACHE *c, uint32_t id) {
    internal_fatal(!c->initialized, "%s cache needs to be initialized", c->what);

    spinlock_lock(&c->spinlock);

    XXH64_hash_t hash = XXH3_64bits(&id, sizeof(id));
    SIMPLE_HASHTABLE_SLOT_ID_NAMES_CACHE *sl = simple_hashtable_get_slot_ID_NAMES_CACHE(&c->ht, hash, &id, true);
    ID_NAME *in = SIMPLE_HASHTABLE_SLOT_DATA(sl);
    if(!in) {
        in = callocz(1, sizeof(*in));

        const char *name = c->lookup(id, c->lookup_buf, sizeof(c->lookup_buf));
        if(!name || !*name) {
            char id_str[UINT64_MAX_LENGTH];
            print_uint64(id_str, id);
            in->name = string_strdupz(id_str);
        }
        else
            in->name = string_strdupz(name);

        in->id = id;
        simple_hashtable_set_slot_ID_NAMES_CACHE(&c->ht, sl, hash, in);
    }

    internal_fatal(in->id != id, "invalid %s id matched from cache", c->what);

    ID_NAME rc = {
        .version = in->version,
        .id = in->id,
        .name = string_dup(in->name),
    };

    spinlock_unlock(&c->spinlock);
    return rc;
}

static void id_names_init(ID_NAMES_CACHE *c) {
    if(c->initialized) return;
    c->initialized = true;

    spinlock_init(&c->spinlock);
    simple_hashtable_init_ID_NAMES_CACHE(&c->ht, 100);
}

static void id_names_destroy(ID_NAMES_CACHE *c) {
    if(!c->initialized) return;

    spinlock_lock(&c->spinlock);

    for(SIMPLE_HASHTABLE_SLOT_ID_NAMES_CACHE *sl = simple_hashtable_first_read_only_ID_NAMES_CACHE(&c->ht);
         sl;
         sl = simple_hashtable_next_read_only_ID_NAMES_CACHE(&c->ht, sl)) {
        ID_NAME *in = SIMPLE_HASHTABLE_SLOT_DATA(sl);
        if(in) {
            string_freez(in->name);
            freez(in);
        }
    }

    simple_hashtable_destroy_ID_NAMES_CACHE(&c->ht);
    c->initialized = false;

    spinlock_unlock(&c->spinlock);
}

static void id_names_delete_old_versions(ID_NAMES_CACHE *c, uint32_t version) {
    if(!c->initialized) return;

    spinlock_lock(&c->spinlock);

    for(SIMPLE_HASHTABLE_SLOT_ID_NAMES_CACHE *sl = simple_hashtable_first_read_only_ID_NAMES_CACHE(&c->ht);
         sl;
         sl = simple_hashtable_next_read_only_ID_NAMES_CACHE(&c->ht, sl)) {
        ID_NAME *in = SIMPLE_HASHTABLE_SLOT_DATA(sl);
        if(in && in->version && in->version < version) {
            string_freez(in->name);
            freez(in);
            simple_hashtable_del_slot_ID_NAMES_CACHE(&c->ht, sl);
        }
    }

    spinlock_unlock(&c->spinlock);
}

// --------------------------------------------------------------------------------------------------------------------
// refreshing a cache from the host's /etc/passwd or /etc/group

static bool file_changed(const struct stat *statbuf __maybe_unused, struct timespec *last_modification_time __maybe_unused) {
#if defined(OS_MACOS) || defined(OS_WINDOWS)
    return false;
#else
    if(likely(statbuf->st_mtim.tv_sec == last_modification_time->tv_sec &&
               statbuf->st_mtim.tv_nsec == last_modification_time->tv_nsec)) return false;

    last_modification_time->tv_sec = statbuf->st_mtim.tv_sec;
    last_modification_time->tv_nsec = statbuf->st_mtim.tv_nsec;

    return true;
#endif
}

// populates the cache from name:x:id lines; returns the number of lines added
static size_t read_passwd_or_group(ID_NAMES_CACHE *c, const char *filename, uint32_t version) {
    struct stat statbuf;
    if(unlikely(stat(filename, &statbuf) || !file_changed(&statbuf, &c->host_file.last_modification_time)))
        return 0;

    procfile *ff = procfile_open(filename, " :\t", PROCFILE_FLAG_DEFAULT);
    if(unlikely(!ff)) return 0;

    ff = procfile_readall(ff);
    if(unlikely(!ff)) return 0;

    size_t line, lines = procfile_lines(ff);

    size_t added = 0;
    for(line = 0; line < lines ;line++) {
        size_t words = procfile_linewords(ff, line);
        if(unlikely(words < 3)) continue;

        char *name = procfile_lineword(ff, line, 0);
        if(unlikely(!name || !*name)) continue;

        char *id_string = procfile_lineword(ff, line, 2);
        if(unlikely(!id_string || !*id_string)) continue;

        uint32_t id = str2ull(id_string, NULL);

        id_names_populate(c, id, name, version);
        added++;
    }

    procfile_close(ff);
    return added;
}

static void id_names_update_from_host_file(ID_NAMES_CACHE *c) {
    if(!netdata_configured_host_prefix || !*netdata_configured_host_prefix) return;

    if(!spinlock_trylock(&c->host_file.spinlock)) return;

    id_names_init(c);

    char filename[FILENAME_MAX];
    snprintfz(filename, FILENAME_MAX, "%s%s", netdata_configured_host_prefix, c->host_file.filename);
    uint32_t version = ++c->host_file.version;
    size_t added = read_passwd_or_group(c, filename, version);
    if(added) id_names_delete_old_versions(c, version);

    spinlock_unlock(&c->host_file.spinlock);
}

// --------------------------------------------------------------------------------------------------------------------
// the two caches

static const char *lookup_username(uint32_t id, char *buf, size_t buf_size) {
    struct passwd pw, *result = NULL;
    if(getpwuid_r((uid_t)id, &pw, buf, buf_size, &result) != 0 || !result || !pw.pw_name)
        return NULL;

    return pw.pw_name; // points into buf
}

static const char *lookup_groupname(uint32_t id, char *buf, size_t buf_size) {
    struct group gr, *result = NULL;
    if(getgrgid_r((gid_t)id, &gr, buf, buf_size, &result) != 0 || !result || !gr.gr_name)
        return NULL;

    return gr.gr_name; // points into buf
}

static ID_NAMES_CACHE users_cache = {
    .what = "system-users",
    .lookup = lookup_username,
    .spinlock = SPINLOCK_INITIALIZER,
    .host_file = {
        .filename = "/etc/passwd",
        .spinlock = SPINLOCK_INITIALIZER,
    },
};

static ID_NAMES_CACHE groups_cache = {
    .what = "system-groups",
    .lookup = lookup_groupname,
    .spinlock = SPINLOCK_INITIALIZER,
    .host_file = {
        .filename = "/etc/group",
        .spinlock = SPINLOCK_INITIALIZER,
    },
};

// --------------------------------------------------------------------------------------------------------------------
// uid -> username

void cached_username_populate_by_uid(uid_t uid, const char *username, uint32_t version) {
    id_names_populate(&users_cache, uid, username, version);
}

CACHED_USERNAME cached_username_get_by_uid(uid_t uid) {
    ID_NAME in = id_names_get(&users_cache, uid);
    return (CACHED_USERNAME) {
        .version = in.version,
        .uid = in.id,
        .username = in.name,
    };
}

void cached_username_release(CACHED_USERNAME cu) {
    string_freez(cu.username);
}

void cached_usernames_init(void) {
    id_names_init(&users_cache);
}

void cached_usernames_destroy(void) {
    id_names_destroy(&users_cache);
}

void cached_usernames_delete_old_versions(uint32_t version) {
    id_names_delete_old_versions(&users_cache, version);
}

void update_cached_host_users(void) {
    id_names_update_from_host_file(&users_cache);
}

// --------------------------------------------------------------------------------------------------------------------
// gid -> groupname

void cached_groupname_populate_by_gid(gid_t gid, const char *groupname, uint32_t version) {
    id_names_populate(&groups_cache, gid, groupname, version);
}

CACHED_GROUPNAME cached_groupname_get_by_gid(gid_t gid) {
    ID_NAME in = id_names_get(&groups_cache, gid);
    return (CACHED_GROUPNAME) {
        .version = in.version,
        .gid = in.id,
        .groupname = in.name,
    };
}

void cached_groupname_release(CACHED_GROUPNAME cg) {
    string_freez(cg.groupname);
}

void cached_groupnames_init(void) {
    id_names_init(&groups_cache);
}

void cached_groupnames_destroy(void) {
    id_names_destroy(&groups_cache);
}

void cached_groupnames_delete_old_versions(uint32_t version) {
    id_names_delete_old_versions(&groups_cache, version);
}

void update_cached_host_groups(void) {
    id_names_update_from_host_file(&groups_cache);
}
