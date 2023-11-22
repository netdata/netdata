// SPDX-License-Identifier: GPL-3.0-or-later

#include "systemd-internals.h"

#define SYSTEMD_JOURNAL_MAX_SOURCE_LEN 64
#define VAR_LOG_JOURNAL_MAX_DEPTH 10
#define MAX_JOURNAL_DIRECTORIES 100

struct journal_directory {
    char *path;
    bool logged_failure;
};

static struct journal_directory journal_directories[MAX_JOURNAL_DIRECTORIES] = { 0 };
DICTIONARY *journal_files_registry = NULL;
DICTIONARY *used_hashes_registry = NULL;

static usec_t systemd_journal_session = 0;

void buffer_json_journal_versions(BUFFER *wb) {
    buffer_json_member_add_object(wb, "versions");
    {
        buffer_json_member_add_uint64(wb, "sources",
                systemd_journal_session + dictionary_version(journal_files_registry));
    }
    buffer_json_object_close(wb);
}

static void journal_file_update_msg_ut(const char *filename, struct journal_file *jf) {
    fstat_cache_enable_on_thread();

    const char *files[2] = {
            [0] = filename,
            [1] = NULL,
    };

    sd_journal *j = NULL;
    if(sd_journal_open_files(&j, files, ND_SD_JOURNAL_OPEN_FLAGS) < 0 || !j) {
        netdata_log_error("JOURNAL: cannot open file '%s' to update msg_ut", filename);
        fstat_cache_disable_on_thread();

        if(!jf->logged_failure) {
            netdata_log_error("cannot open journal file '%s', using file timestamps to understand time-frame.", filename);
            jf->logged_failure = true;
        }

        jf->msg_first_ut = 0;
        jf->msg_last_ut = jf->file_last_modified_ut;
        return;
    }

    usec_t first_ut = 0, last_ut = 0;

    if(sd_journal_seek_head(j) < 0 || sd_journal_next(j) < 0 || sd_journal_get_realtime_usec(j, &first_ut) < 0 || !first_ut) {
        internal_error(true, "cannot find the timestamp of the first message in '%s'", filename);
        first_ut = 0;
    }

    if(sd_journal_seek_tail(j) < 0 || sd_journal_previous(j) < 0 || sd_journal_get_realtime_usec(j, &last_ut) < 0 || !last_ut) {
        internal_error(true, "cannot find the timestamp of the last message in '%s'", filename);
        last_ut = jf->file_last_modified_ut;
    }

    sd_journal_close(j);
    fstat_cache_disable_on_thread();

    if(first_ut > last_ut) {
        internal_error(true, "timestamps are flipped in file '%s'", filename);
        usec_t t = first_ut;
        first_ut = last_ut;
        last_ut = t;
    }

    jf->msg_first_ut = first_ut;
    jf->msg_last_ut = last_ut;
}

static STRING *string_strdupz_source(const char *s, const char *e, size_t max_len, const char *prefix) {
    char buf[max_len];
    size_t len;
    char *dst = buf;

    if(prefix) {
        len = strlen(prefix);
        memcpy(buf, prefix, len);
        dst = &buf[len];
        max_len -= len;
    }

    len = e - s;
    if(len >= max_len)
        len = max_len - 1;
    memcpy(dst, s, len);
    dst[len] = '\0';
    buf[max_len - 1] = '\0';

    for(size_t i = 0; buf[i] ;i++)
        if(!isalnum(buf[i]) && buf[i] != '-' && buf[i] != '.' && buf[i] != ':')
            buf[i] = '_';

    return string_strdupz(buf);
}

static void files_registry_insert_cb(const DICTIONARY_ITEM *item, void *value, void *data __maybe_unused) {
    struct journal_file *jf = value;
    jf->filename = dictionary_acquired_item_name(item);
    jf->filename_len = strlen(jf->filename);
    jf->source_type = SDJF_ALL;

    // based on the filename
    // decide the source to show to the user
    const char *s = strrchr(jf->filename, '/');
    if(s) {
        if(strstr(jf->filename, "/remote/")) {
            jf->source_type |= SDJF_REMOTE_ALL;

            if(strncmp(s, "/remote-", 8) == 0) {
                s = &s[8]; // skip "/remote-"

                char *e = strchr(s, '@');
                if(!e)
                    e = strstr(s, ".journal");

                if(e) {
                    const char *d = s;
                    for(; d < e && (isdigit(*d) || *d == '.' || *d == ':') ; d++) ;
                    if(d == e) {
                        // a valid IP address
                        char ip[e - s + 1];
                        memcpy(ip, s, e - s);
                        ip[e - s] = '\0';
                        char buf[SYSTEMD_JOURNAL_MAX_SOURCE_LEN];
                        if(ip_to_hostname(ip, buf, sizeof(buf)))
                            jf->source = string_strdupz_source(buf, &buf[strlen(buf)], SYSTEMD_JOURNAL_MAX_SOURCE_LEN, "remote-");
                        else {
                            internal_error(true, "Cannot find the hostname for IP '%s'", ip);
                            jf->source = string_strdupz_source(s, e, SYSTEMD_JOURNAL_MAX_SOURCE_LEN, "remote-");
                        }
                    }
                    else
                        jf->source = string_strdupz_source(s, e, SYSTEMD_JOURNAL_MAX_SOURCE_LEN, "remote-");
                }
            }
        }
        else {
            jf->source_type |= SDJF_LOCAL_ALL;

            const char *t = s - 1;
            while(t >= jf->filename && *t != '.' && *t != '/')
                t--;

            if(t >= jf->filename && *t == '.') {
                jf->source_type |= SDJF_LOCAL_NAMESPACE;
                jf->source = string_strdupz_source(t + 1, s, SYSTEMD_JOURNAL_MAX_SOURCE_LEN, "namespace-");
            }
            else if(strncmp(s, "/system", 7) == 0)
                jf->source_type |= SDJF_LOCAL_SYSTEM;

            else if(strncmp(s, "/user", 5) == 0)
                jf->source_type |= SDJF_LOCAL_USER;

            else
                jf->source_type |= SDJF_LOCAL_OTHER;
        }
    }
    else
        jf->source_type |= SDJF_LOCAL_ALL | SDJF_LOCAL_OTHER;

    journal_file_update_msg_ut(jf->filename, jf);

    internal_error(true,
            "found journal file '%s', type %d, source '%s', "
            "file modified: %"PRIu64", "
            "msg {first: %"PRIu64", last: %"PRIu64"}",
            jf->filename, jf->source_type, jf->source ? string2str(jf->source) : "<unset>",
            jf->file_last_modified_ut,
            jf->msg_first_ut, jf->msg_last_ut);
}

static bool files_registry_conflict_cb(const DICTIONARY_ITEM *item, void *old_value, void *new_value, void *data __maybe_unused) {
    struct journal_file *jf = old_value;
    struct journal_file *njf = new_value;

    if(njf->last_scan_ut > jf->last_scan_ut)
        jf->last_scan_ut = njf->last_scan_ut;

    if(njf->file_last_modified_ut > jf->file_last_modified_ut) {
        jf->file_last_modified_ut = njf->file_last_modified_ut;
        jf->size = njf->size;

        const char *filename = dictionary_acquired_item_name(item);
        journal_file_update_msg_ut(filename, jf);

//        internal_error(true,
//                "updated journal file '%s', type %d, "
//                "file modified: %"PRIu64", "
//                                        "msg {first: %"PRIu64", last: %"PRIu64"}",
//                filename, jf->source_type,
//                jf->file_last_modified_ut,
//                jf->msg_first_ut, jf->msg_last_ut);
    }

    return false;
}

struct journal_file_source {
    usec_t first_ut;
    usec_t last_ut;
    size_t count;
    uint64_t size;
};

static void human_readable_size_ib(uint64_t size, char *dst, size_t dst_len) {
    if(size > 1024ULL * 1024 * 1024 * 1024)
        snprintfz(dst, dst_len, "%0.2f TiB", (double)size / 1024.0 / 1024.0 / 1024.0 / 1024.0);
    else if(size > 1024ULL * 1024 * 1024)
        snprintfz(dst, dst_len, "%0.2f GiB", (double)size / 1024.0 / 1024.0 / 1024.0);
    else if(size > 1024ULL * 1024)
        snprintfz(dst, dst_len, "%0.2f MiB", (double)size / 1024.0 / 1024.0);
    else if(size > 1024ULL)
        snprintfz(dst, dst_len, "%0.2f KiB", (double)size / 1024.0);
    else
        snprintfz(dst, dst_len, "%"PRIu64" B", size);
}

#define print_duration(dst, dst_len, pos, remaining, duration, one, many, printed) do { \
    if((remaining) > (duration)) {                                                      \
        uint64_t _count = (remaining) / (duration);                                     \
        uint64_t _rem = (remaining) - (_count * (duration));                            \
        (pos) += snprintfz(&(dst)[pos], (dst_len) - (pos), "%s%s%"PRIu64" %s", (printed) ? ", " : "", _rem ? "" : "and ", _count, _count > 1 ? (many) : (one));  \
        (remaining) = _rem;                                                             \
        (printed) = true;                                                               \
    } \
} while(0)

static void human_readable_duration_s(time_t duration_s, char *dst, size_t dst_len) {
    if(duration_s < 0)
        duration_s = -duration_s;

    size_t pos = 0;
    dst[0] = 0 ;

    bool printed = false;
    print_duration(dst, dst_len, pos, duration_s, 86400 * 365, "year", "years", printed);
    print_duration(dst, dst_len, pos, duration_s, 86400 * 30, "month", "months", printed);
    print_duration(dst, dst_len, pos, duration_s, 86400 * 1, "day", "days", printed);
    print_duration(dst, dst_len, pos, duration_s, 3600 * 1, "hour", "hours", printed);
    print_duration(dst, dst_len, pos, duration_s, 60 * 1, "min", "mins", printed);
    print_duration(dst, dst_len, pos, duration_s, 1, "sec", "secs", printed);
}

static int journal_file_to_json_array_cb(const DICTIONARY_ITEM *item, void *entry, void *data) {
    struct journal_file_source *jfs = entry;
    BUFFER *wb = data;

    const char *name = dictionary_acquired_item_name(item);

    buffer_json_add_array_item_object(wb);
    {
        char size_for_humans[100];
        human_readable_size_ib(jfs->size, size_for_humans, sizeof(size_for_humans));

        char duration_for_humans[1024];
        human_readable_duration_s((time_t)((jfs->last_ut - jfs->first_ut) / USEC_PER_SEC),
                duration_for_humans, sizeof(duration_for_humans));

        char info[1024];
        snprintfz(info, sizeof(info), "%zu files, with a total size of %s, covering %s",
                jfs->count, size_for_humans, duration_for_humans);

        buffer_json_member_add_string(wb, "id", name);
        buffer_json_member_add_string(wb, "name", name);
        buffer_json_member_add_string(wb, "pill", size_for_humans);
        buffer_json_member_add_string(wb, "info", info);
    }
    buffer_json_object_close(wb); // options object

    return 1;
}

static bool journal_file_merge_sizes(const DICTIONARY_ITEM *item __maybe_unused, void *old_value, void *new_value , void *data __maybe_unused) {
    struct journal_file_source *jfs = old_value, *njfs = new_value;
    jfs->count += njfs->count;
    jfs->size += njfs->size;

    if(njfs->first_ut && njfs->first_ut < jfs->first_ut)
        jfs->first_ut = njfs->first_ut;

    if(njfs->last_ut && njfs->last_ut > jfs->last_ut)
        jfs->last_ut = njfs->last_ut;

    return false;
}

void available_journal_file_sources_to_json_array(BUFFER *wb) {
    DICTIONARY *dict = dictionary_create(DICT_OPTION_SINGLE_THREADED|DICT_OPTION_NAME_LINK_DONT_CLONE|DICT_OPTION_DONT_OVERWRITE_VALUE);
    dictionary_register_conflict_callback(dict, journal_file_merge_sizes, NULL);

    struct journal_file_source t = { 0 };

    struct journal_file *jf;
    dfe_start_read(journal_files_registry, jf) {
        t.first_ut = jf->msg_first_ut;
        t.last_ut = jf->msg_last_ut;
        t.count = 1;
        t.size = jf->size;

        dictionary_set(dict, SDJF_SOURCE_ALL_NAME, &t, sizeof(t));

        if(jf->source_type & SDJF_LOCAL_ALL)
            dictionary_set(dict, SDJF_SOURCE_LOCAL_NAME, &t, sizeof(t));
        if(jf->source_type & SDJF_LOCAL_SYSTEM)
            dictionary_set(dict, SDJF_SOURCE_LOCAL_SYSTEM_NAME, &t, sizeof(t));
        if(jf->source_type & SDJF_LOCAL_USER)
            dictionary_set(dict, SDJF_SOURCE_LOCAL_USERS_NAME, &t, sizeof(t));
        if(jf->source_type & SDJF_LOCAL_OTHER)
            dictionary_set(dict, SDJF_SOURCE_LOCAL_OTHER_NAME, &t, sizeof(t));
        if(jf->source_type & SDJF_LOCAL_NAMESPACE)
            dictionary_set(dict, SDJF_SOURCE_NAMESPACES_NAME, &t, sizeof(t));
        if(jf->source_type & SDJF_REMOTE_ALL)
            dictionary_set(dict, SDJF_SOURCE_REMOTES_NAME, &t, sizeof(t));
        if(jf->source)
            dictionary_set(dict, string2str(jf->source), &t, sizeof(t));
    }
    dfe_done(jf);

    dictionary_sorted_walkthrough_read(dict, journal_file_to_json_array_cb, wb);

    dictionary_destroy(dict);
}

static void files_registry_delete_cb(const DICTIONARY_ITEM *item, void *value, void *data __maybe_unused) {
    struct journal_file *jf = value; (void)jf;
    const char *filename = dictionary_acquired_item_name(item); (void)filename;

    internal_error(true, "removed journal file '%s'", filename);
    string_freez(jf->source);
}

void journal_directory_scan(const char *dirname, int depth, usec_t last_scan_ut) {
    static const char *ext = ".journal";
    static const size_t ext_len = sizeof(".journal") - 1;

    if (depth > VAR_LOG_JOURNAL_MAX_DEPTH)
        return;

    DIR *dir;
    struct dirent *entry;
    struct stat info;
    char absolute_path[FILENAME_MAX];

    // Open the directory.
    if ((dir = opendir(dirname)) == NULL) {
        if(errno != ENOENT && errno != ENOTDIR)
            netdata_log_error("Cannot opendir() '%s'", dirname);
        return;
    }

    // Read each entry in the directory.
    while ((entry = readdir(dir)) != NULL) {
        snprintfz(absolute_path, sizeof(absolute_path), "%s/%s", dirname, entry->d_name);
        if (stat(absolute_path, &info) != 0) {
            netdata_log_error("Failed to stat() '%s", absolute_path);
            continue;
        }

        if (S_ISDIR(info.st_mode)) {
            // If entry is a directory, call traverse recursively.
            if (strcmp(entry->d_name, ".") != 0 && strcmp(entry->d_name, "..") != 0)
                journal_directory_scan(absolute_path, depth + 1, last_scan_ut);

        }
        else if (S_ISREG(info.st_mode)) {
            // If entry is a regular file, check if it ends with .journal.
            char *filename = entry->d_name;
            size_t len = strlen(filename);

            if (len > ext_len && strcmp(filename + len - ext_len, ext) == 0) {
                struct journal_file t = {
                        .file_last_modified_ut = info.st_mtim.tv_sec * USEC_PER_SEC + info.st_mtim.tv_nsec / NSEC_PER_USEC,
                        .last_scan_ut = last_scan_ut,
                        .size = info.st_size,
                        .max_journal_vs_realtime_delta_ut = JOURNAL_VS_REALTIME_DELTA_DEFAULT_UT,
                };
                dictionary_set(journal_files_registry, absolute_path, &t, sizeof(t));
                send_newline_and_flush();
            }
        }
    }

    closedir(dir);
}

void journal_files_registry_update(void) {
    static SPINLOCK spinlock = NETDATA_SPINLOCK_INITIALIZER;

    if(spinlock_trylock(&spinlock)) {
        usec_t scan_ut = now_monotonic_usec();

        for(unsigned i = 0; i < MAX_JOURNAL_DIRECTORIES; i++) {
            if(!journal_directories[i].path)
                break;

            journal_directory_scan(journal_directories[i].path, 0, scan_ut);
        }

        struct journal_file *jf;
        dfe_start_write(journal_files_registry, jf){
                    if(jf->last_scan_ut < scan_ut)
                        dictionary_del(journal_files_registry, jf_dfe.name);
                }
        dfe_done(jf);

        spinlock_unlock(&spinlock);
    }
}

// ----------------------------------------------------------------------------

int journal_file_dict_items_backward_compar(const void *a, const void *b) {
    const DICTIONARY_ITEM **ad = (const DICTIONARY_ITEM **)a, **bd = (const DICTIONARY_ITEM **)b;
    struct journal_file *jfa = dictionary_acquired_item_value(*ad);
    struct journal_file *jfb = dictionary_acquired_item_value(*bd);

    if(jfa->msg_last_ut < jfb->msg_last_ut)
        return 1;

    if(jfa->msg_last_ut > jfb->msg_last_ut)
        return -1;

    if(jfa->msg_first_ut < jfb->msg_first_ut)
        return 1;

    if(jfa->msg_first_ut > jfb->msg_first_ut)
        return -1;

    return 0;
}

int journal_file_dict_items_forward_compar(const void *a, const void *b) {
    return -journal_file_dict_items_backward_compar(a, b);
}

void journal_init_files_and_directories(void) {
    unsigned d = 0;

    // ------------------------------------------------------------------------
    // setup the journal directories

    journal_directories[d++].path = strdupz("/var/log/journal");
    journal_directories[d++].path = strdupz("/run/log/journal");

    if(*netdata_configured_host_prefix) {
        char path[PATH_MAX];
        snprintfz(path, sizeof(path), "%s/var/log/journal", netdata_configured_host_prefix);
        journal_directories[d++].path = strdupz(path);
        snprintfz(path, sizeof(path), "%s/run/log/journal", netdata_configured_host_prefix);
        journal_directories[d++].path = strdupz(path);
    }

    // terminate the list
    journal_directories[d].path = NULL;

    // ------------------------------------------------------------------------
    // initialize the used hashes files registry

    used_hashes_registry = dictionary_create(DICT_OPTION_DONT_OVERWRITE_VALUE);

    systemd_journal_session = (now_realtime_usec() / USEC_PER_SEC) * USEC_PER_SEC;

    journal_files_registry = dictionary_create_advanced(
            DICT_OPTION_DONT_OVERWRITE_VALUE | DICT_OPTION_FIXED_SIZE,
            NULL, sizeof(struct journal_file));

    dictionary_register_insert_callback(journal_files_registry, files_registry_insert_cb, NULL);
    dictionary_register_delete_callback(journal_files_registry, files_registry_delete_cb, NULL);
    dictionary_register_conflict_callback(journal_files_registry, files_registry_conflict_cb, NULL);

    boot_ids_to_first_ut = dictionary_create_advanced(
            DICT_OPTION_DONT_OVERWRITE_VALUE | DICT_OPTION_FIXED_SIZE,
            NULL, sizeof(usec_t));
}
