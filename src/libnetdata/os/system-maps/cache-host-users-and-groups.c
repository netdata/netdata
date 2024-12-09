// SPDX-License-Identifier: GPL-3.0-or-later

#include "libnetdata/libnetdata.h"

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

static size_t read_passwd_or_group(const char *filename, struct timespec *last_modification_time, void (*cb)(uint32_t gid, const char *name, uint32_t version), uint32_t version) {
    struct stat statbuf;
    if(unlikely(stat(filename, &statbuf) || !file_changed(&statbuf, last_modification_time)))
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

        cb(id, name, version);
        added++;
    }

    procfile_close(ff);
    return added;
}

void update_cached_host_users(void) {
    if(!netdata_configured_host_prefix || !*netdata_configured_host_prefix) return;

    static SPINLOCK spinlock = SPINLOCK_INITIALIZER;
    if(!spinlock_trylock(&spinlock)) return;

    char filename[FILENAME_MAX];
    static bool initialized = false;

    size_t added = 0;

    if(!initialized) {
        initialized = true;
        cached_usernames_init();
    }

    static uint32_t passwd_version = 0;
    static struct timespec passwd_ts = { 0 };
    snprintfz(filename, FILENAME_MAX, "%s/etc/passwd", netdata_configured_host_prefix);
    added = read_passwd_or_group(filename, &passwd_ts, cached_username_populate_by_uid, ++passwd_version);
    if(added) cached_usernames_delete_old_versions(passwd_version);

    spinlock_unlock(&spinlock);
}

void update_cached_host_groups(void) {
    if(!netdata_configured_host_prefix || !*netdata_configured_host_prefix) return;
    
    static SPINLOCK spinlock = SPINLOCK_INITIALIZER;
    if(!spinlock_trylock(&spinlock)) return;

    char filename[FILENAME_MAX];
    static bool initialized = false;

    size_t added = 0;

    if(!initialized) {
        initialized = true;
        cached_groupnames_init();
    }

    static uint32_t group_version = 0;
    static struct timespec group_ts = { 0 };
    snprintfz(filename, FILENAME_MAX, "%s/etc/group", netdata_configured_host_prefix);
    added = read_passwd_or_group(filename, &group_ts, cached_groupname_populate_by_gid, ++group_version);
    if(added) cached_groupnames_delete_old_versions(group_version);

    spinlock_unlock(&spinlock);
}
