// SPDX-License-Identifier: GPL-3.0-or-later

#include "systemd-internals.h"

#define SYSTEMD_JOURNAL_MAX_SOURCE_LEN 64
#define VAR_LOG_JOURNAL_MAX_DEPTH 10

struct journal_directory journal_directories[MAX_JOURNAL_DIRECTORIES] = { 0 };
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

static bool journal_sd_id128_parse(const char *in, sd_id128_t *ret) {
    while(isspace(*in))
        in++;

    char uuid[33];
    strncpyz(uuid, in, 32);
    uuid[32] = '\0';

    if(strlen(uuid) == 32) {
        sd_id128_t read;
        if(sd_id128_from_string(uuid, &read) == 0) {
            *ret = read;
            return true;
        }
    }

    return false;
}

//static void journal_file_get_header_from_journalctl(const char *filename, struct journal_file *jf) {
//    // unfortunately, our capabilities are not inheritted by journalctl
//    // so, it fails to give us the information we need.
//
//    bool read_writer = false, read_head = false, read_tail = false;
//
//    char cmd[FILENAME_MAX * 2];
//    snprintfz(cmd, sizeof(cmd), "journalctl --header --file '%s'", filename);
//    CLEAN_BUFFER *wb = run_command_and_get_output_to_buffer(cmd, 1024);
//    if(wb) {
//        const char *s = buffer_tostring(wb);
//
//        const char *sequential_id_header = "Sequential Number ID:";
//        const char *sequential_id_data = strcasestr(s, sequential_id_header);
//        if(sequential_id_data) {
//            sequential_id_data += strlen(sequential_id_header);
//            if(journal_sd_id128_parse(sequential_id_data, &jf->first_writer_id))
//                read_writer = true;
//        }
//
//        const char *head_sequential_number_header = "Head sequential number:";
//        const char *head_sequential_number_data = strcasestr(s, head_sequential_number_header);
//        if(head_sequential_number_data) {
//            head_sequential_number_data += strlen(head_sequential_number_header);
//
//            while(isspace(*head_sequential_number_data))
//                head_sequential_number_data++;
//
//            if(isdigit(*head_sequential_number_data)) {
//                jf->first_seqnum = strtoul(head_sequential_number_data, NULL, 10);
//                if(jf->first_seqnum)
//                    read_head = true;
//            }
//        }
//
//        const char *tail_sequential_number_header = "Tail sequential number:";
//        const char *tail_sequential_number_data = strcasestr(s, tail_sequential_number_header);
//        if(tail_sequential_number_data) {
//            tail_sequential_number_data += strlen(tail_sequential_number_header);
//
//            while(isspace(*tail_sequential_number_data))
//                tail_sequential_number_data++;
//
//            if(isdigit(*tail_sequential_number_data)) {
//                jf->last_seqnum = strtoul(tail_sequential_number_data, NULL, 10);
//                if(jf->last_seqnum)
//                    read_tail = true;
//            }
//        }
//
//        if(read_head && read_tail && jf->last_seqnum > jf->first_seqnum)
//            jf->messages_in_file = jf->last_seqnum - jf->first_seqnum;
//    }
//
//    if(!jf->logged_journalctl_failure && (!read_head || !read_tail)) {
//
//        nd_log(NDLS_COLLECTORS, NDLP_NOTICE,
//               "Failed to read %s%s%s from journalctl's output on filename '%s', using the command: %s",
//               read_writer?"":"writer id,",
//               read_head?"":"head id,",
//               read_tail?"":"tail id,",
//               filename, cmd);
//
//        jf->logged_journalctl_failure = true;
//    }
//}

usec_t journal_file_update_annotation_boot_id(sd_journal *j, struct journal_file *jf __maybe_unused, const char *boot_id) {
    usec_t ut = UINT64_MAX;
    int r;

    char m[100];
    size_t len = snprintfz(m, sizeof(m), "_BOOT_ID=%s", boot_id);

    sd_journal_flush_matches(j);

    r = sd_journal_add_match(j, m, len);
    if(r < 0) {
        errno = -r;
        internal_error(true,
                       "JOURNAL: while looking for the first timestamp of boot_id '%s', "
                       "sd_journal_add_match('%s') on file '%s' returned %d",
                       boot_id, m, jf->filename, r);
        return UINT64_MAX;
    }

    r = sd_journal_seek_head(j);
    if(r < 0) {
        errno = -r;
        internal_error(true,
                       "JOURNAL: while looking for the first timestamp of boot_id '%s', "
                       "sd_journal_seek_head() on file '%s' returned %d",
                       boot_id, jf->filename, r);
        return UINT64_MAX;
    }

    r = sd_journal_next(j);
    if(r < 0) {
        errno = -r;
        internal_error(true,
                       "JOURNAL: while looking for the first timestamp of boot_id '%s', "
                       "sd_journal_next() on file '%s' returned %d",
                       boot_id, jf->filename, r);
        return UINT64_MAX;
    }

    r = sd_journal_get_realtime_usec(j, &ut);
    if(r < 0 || !ut || ut == UINT64_MAX) {
        errno = -r;
        internal_error(r != -EADDRNOTAVAIL,
                       "JOURNAL: while looking for the first timestamp of boot_id '%s', "
                       "sd_journal_get_realtime_usec() on file '%s' returned %d",
                       boot_id, jf->filename, r);
        return UINT64_MAX;
    }

    if(ut && ut != UINT64_MAX) {
        dictionary_set(boot_ids_to_first_ut, boot_id, &ut, sizeof(ut));
        return ut;
    }

    return UINT64_MAX;
}

static void journal_file_get_boot_id_annotations(sd_journal *j __maybe_unused, struct journal_file *jf __maybe_unused) {
#ifdef HAVE_SD_JOURNAL_RESTART_FIELDS
    sd_journal_flush_matches(j);

    int r = sd_journal_query_unique(j, "_BOOT_ID");
    if (r < 0) {
        errno = -r;
        internal_error(true,
                       "JOURNAL: while querying for the unique _BOOT_ID values, "
                       "sd_journal_query_unique() on file '%s' returned %d",
                       jf->filename, r);
        errno = -r;
        return;
    }

    const void *data = NULL;
    size_t data_length;

    DICTIONARY *dict = dictionary_create(DICT_OPTION_SINGLE_THREADED);

    SD_JOURNAL_FOREACH_UNIQUE(j, data, data_length) {
        const char *key, *value;
        size_t key_length, value_length;

        if(!parse_journal_field(data, data_length, &key, &key_length, &value, &value_length))
            continue;

        if(value_length != 32)
            continue;

        char buf[33];
        memcpy(buf, value, 32);
        buf[32] = '\0';

        dictionary_set(dict, buf, NULL, 0);
    }

    void *nothing;
    dfe_start_read(dict, nothing){
        journal_file_update_annotation_boot_id(j, jf, nothing_dfe.name);
    }
    dfe_done(nothing);

    dictionary_destroy(dict);
#endif
}

void journal_file_update_header(const char *filename, struct journal_file *jf) {
    if(jf->last_scan_header_vs_last_modified_ut == jf->file_last_modified_ut)
        return;

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
        jf->last_scan_header_vs_last_modified_ut = jf->file_last_modified_ut;
        return;
    }

    usec_t first_ut = 0, last_ut = 0;
    uint64_t first_seqnum = 0, last_seqnum = 0;
    sd_id128_t first_writer_id = SD_ID128_NULL, last_writer_id = SD_ID128_NULL;

    if(sd_journal_seek_head(j) < 0 || sd_journal_next(j) < 0 || sd_journal_get_realtime_usec(j, &first_ut) < 0 || !first_ut) {
        internal_error(true, "cannot find the timestamp of the first message in '%s'", filename);
        first_ut = 0;
    }
#ifdef HAVE_SD_JOURNAL_GET_SEQNUM
    else {
        if(sd_journal_get_seqnum(j, &first_seqnum, &first_writer_id) < 0 || !first_seqnum) {
            internal_error(true, "cannot find the first seqnums of the first message in '%s'", filename);
            first_seqnum = 0;
            memset(&first_writer_id, 0, sizeof(first_writer_id));
        }
    }
#endif

    if(sd_journal_seek_tail(j) < 0 || sd_journal_previous(j) < 0 || sd_journal_get_realtime_usec(j, &last_ut) < 0 || !last_ut) {
        internal_error(true, "cannot find the timestamp of the last message in '%s'", filename);
        last_ut = jf->file_last_modified_ut;
    }
#ifdef HAVE_SD_JOURNAL_GET_SEQNUM
    else {
        if(sd_journal_get_seqnum(j, &last_seqnum, &last_writer_id) < 0 || !last_seqnum) {
            internal_error(true, "cannot find the last seqnums of the first message in '%s'", filename);
            last_seqnum = 0;
            memset(&last_writer_id, 0, sizeof(last_writer_id));
        }
    }
#endif

    if(first_ut > last_ut) {
        internal_error(true, "timestamps are flipped in file '%s'", filename);
        usec_t t = first_ut;
        first_ut = last_ut;
        last_ut = t;
    }

    if(!first_seqnum || !first_ut) {
        // extract these from the filename - if possible

        const char *at = strchr(filename, '@');
        if(at) {
            const char *dash_seqnum = strchr(at + 1, '-');
            if(dash_seqnum) {
                const char *dash_first_msg_ut = strchr(dash_seqnum + 1, '-');
                if(dash_first_msg_ut) {
                    const char *dot_journal = NULL;
                    if(is_journal_file(filename, -1, &dot_journal) && dot_journal && dot_journal > dash_first_msg_ut) {
                        if(dash_seqnum - at - 1 == 32 &&
                            dash_first_msg_ut - dash_seqnum - 1 == 16 &&
                            dot_journal - dash_first_msg_ut - 1 == 16) {
                            sd_id128_t writer;
                            if(journal_sd_id128_parse(at + 1, &writer)) {
                                char *endptr = NULL;
                                uint64_t seqnum = strtoul(dash_seqnum + 1, &endptr, 16);
                                if(endptr == dash_first_msg_ut) {
                                    uint64_t ts = strtoul(dash_first_msg_ut + 1, &endptr, 16);
                                    if(endptr == dot_journal) {
                                        first_seqnum = seqnum;
                                        first_writer_id = writer;
                                        first_ut = ts;
                                    }
                                }
                            }
                        }
                    }
                }
            }
        }
    }

    jf->first_seqnum = first_seqnum;
    jf->last_seqnum = last_seqnum;

    jf->first_writer_id = first_writer_id;
    jf->last_writer_id = last_writer_id;

    jf->msg_first_ut = first_ut;
    jf->msg_last_ut = last_ut;

    if(!jf->msg_last_ut)
        jf->msg_last_ut = jf->file_last_modified_ut;

    if(last_seqnum > first_seqnum) {
        if(!sd_id128_equal(first_writer_id, last_writer_id)) {
            jf->messages_in_file = 0;
            nd_log(NDLS_COLLECTORS, NDLP_NOTICE,
                    "The writers of the first and the last message in file '%s' differ."
                   , filename);
        }
        else
            jf->messages_in_file = last_seqnum - first_seqnum + 1;
    }
    else
        jf->messages_in_file = 0;

//    if(!jf->messages_in_file)
//        journal_file_get_header_from_journalctl(filename, jf);

    journal_file_get_boot_id_annotations(j, jf);
    sd_journal_close(j);
    fstat_cache_disable_on_thread();

    jf->last_scan_header_vs_last_modified_ut = jf->file_last_modified_ut;

//    nd_log(NDLS_COLLECTORS, NDLP_DEBUG,
//           "Journal file header updated '%s'",
//           jf->filename);
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
        if(!is_netdata_api_valid_character(buf[i])) buf[i] = '_';

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
                    is_journal_file(s, -1, (const char **)&e);

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

    jf->msg_last_ut = jf->file_last_modified_ut;

    nd_log(NDLS_COLLECTORS, NDLP_DEBUG,
           "Journal file added to the journal files registry: '%s'",
           jf->filename);
}

static bool files_registry_conflict_cb(const DICTIONARY_ITEM *item __maybe_unused, void *old_value, void *new_value, void *data __maybe_unused) {
    struct journal_file *jf = old_value;
    struct journal_file *njf = new_value;

    if(njf->last_scan_monotonic_ut > jf->last_scan_monotonic_ut)
        jf->last_scan_monotonic_ut = njf->last_scan_monotonic_ut;

    if(njf->file_last_modified_ut > jf->file_last_modified_ut) {
        jf->file_last_modified_ut = njf->file_last_modified_ut;
        jf->size = njf->size;

        jf->msg_last_ut = jf->file_last_modified_ut;

//        nd_log(NDLS_COLLECTORS, NDLP_DEBUG,
//               "Journal file updated to the journal files registry '%s'",
//               jf->filename);
    }

    return false;
}

struct journal_file_source {
    usec_t first_ut;
    usec_t last_ut;
    size_t count;
    uint64_t size;
};

#define print_duration(dst, dst_len, pos, remaining, duration, one, many, printed) do { \
    if((remaining) > (duration)) {                                                      \
        uint64_t _count = (remaining) / (duration);                                     \
        uint64_t _rem = (remaining) - (_count * (duration));                            \
        (pos) += snprintfz(&(dst)[pos], (dst_len) - (pos), "%s%s%"PRIu64" %s", (printed) ? ", " : "", _rem ? "" : "and ", _count, _count > 1 ? (many) : (one));  \
        (remaining) = _rem;                                                             \
        (printed) = true;                                                               \
    } \
} while(0)

static int journal_file_to_json_array_cb(const DICTIONARY_ITEM *item, void *entry, void *data) {
    struct journal_file_source *jfs = entry;
    BUFFER *wb = data;

    const char *name = dictionary_acquired_item_name(item);

    buffer_json_add_array_item_object(wb);
    {
        char size_for_humans[128];
        size_snprintf(size_for_humans, sizeof(size_for_humans), jfs->size, "B", false);

        char duration_for_humans[128];
        duration_snprintf(duration_for_humans, sizeof(duration_for_humans),
                          (time_t)((jfs->last_ut - jfs->first_ut) / USEC_PER_SEC), "s", true);

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

#define EXT_DOT_JOURNAL ".journal"
#define EXT_DOT_JOURNAL_TILDA ".journal~"

static struct {
    const char *ext;
    ssize_t len;
} valid_journal_extension[] = {
    { .ext = EXT_DOT_JOURNAL, .len = sizeof(EXT_DOT_JOURNAL) - 1 },
    { .ext = EXT_DOT_JOURNAL_TILDA, .len = sizeof(EXT_DOT_JOURNAL_TILDA) - 1 },
};

bool is_journal_file(const char *filename, ssize_t len, const char **start_of_extension) {
    if(len < 0)
        len = (ssize_t)strlen(filename);

    for(size_t i = 0; i < _countof(valid_journal_extension) ;i++) {
        const char *ext = valid_journal_extension[i].ext;
        ssize_t elen = valid_journal_extension[i].len;

        if(len > elen && strcmp(filename + len - elen, ext) == 0) {
            if(start_of_extension)
                *start_of_extension = filename + len - elen;
            return true;
        }
    }

    if(start_of_extension)
        *start_of_extension = NULL;

    return false;
}

void journal_directory_scan_recursively(DICTIONARY *files, DICTIONARY *dirs, const char *dirname, int depth) {
    if (depth > VAR_LOG_JOURNAL_MAX_DEPTH)
        return;

    DIR *dir;
    struct dirent *entry;
    char full_path[FILENAME_MAX];

    // Open the directory.
    if ((dir = opendir(dirname)) == NULL) {
        if(errno != ENOENT && errno != ENOTDIR)
            netdata_log_error("Cannot opendir() '%s'", dirname);
        return;
    }

    bool existing = false;
    bool *found = dictionary_set(dirs, dirname, &existing, sizeof(existing));
    if(*found) return;
    *found = true;

    // Read each entry in the directory.
    while ((entry = readdir(dir)) != NULL) {
        if (strcmp(entry->d_name, ".") == 0 || strcmp(entry->d_name, "..") == 0)
            continue;

        ssize_t len = snprintfz(full_path, sizeof(full_path), "%s/%s", dirname, entry->d_name);

        if (entry->d_type == DT_DIR) {
            journal_directory_scan_recursively(files, dirs, full_path, depth++);
        }
        else if (entry->d_type == DT_REG && is_journal_file(full_path, len, NULL)) {
            if(files)
                dictionary_set(files, full_path, NULL, 0);

            send_newline_and_flush(&stdout_mutex);
        }
        else if (entry->d_type == DT_LNK) {
            struct stat info;
            if (stat(full_path, &info) == -1)
                continue;

            if (S_ISDIR(info.st_mode)) {
                // The symbolic link points to a directory
                char resolved_path[FILENAME_MAX + 1];
                if (realpath(full_path, resolved_path) != NULL) {
                    journal_directory_scan_recursively(files, dirs, resolved_path, depth++);
                }
            }
            else if(S_ISREG(info.st_mode) && is_journal_file(full_path, len, NULL)) {
                if(files)
                    dictionary_set(files, full_path, NULL, 0);

                send_newline_and_flush(&stdout_mutex);
            }
        }
    }

    closedir(dir);
}

static size_t journal_files_scans = 0;
bool journal_files_completed_once(void) {
    return journal_files_scans > 0;
}

int filenames_compar(const void *a, const void *b) {
    const char *p1 = *(const char **)a;
    const char *p2 = *(const char **)b;

    const char *at1 = strchr(p1, '@');
    const char *at2 = strchr(p2, '@');

    if(!at1 && at2)
        return -1;

    if(at1 && !at2)
        return 1;

    if(!at1 && !at2)
        return strcmp(p1, p2);

    const char *dash1 = strrchr(at1, '-');
    const char *dash2 = strrchr(at2, '-');

    if(!dash1 || !dash2)
        return strcmp(p1, p2);

    uint64_t ts1 = strtoul(dash1 + 1, NULL, 16);
    uint64_t ts2 = strtoul(dash2 + 1, NULL, 16);

    if(ts1 > ts2)
        return -1;

    if(ts1 < ts2)
        return 1;

    return -strcmp(p1, p2);
}

void journal_files_registry_update(void) {
    static SPINLOCK spinlock = SPINLOCK_INITIALIZER;

    if(spinlock_trylock(&spinlock)) {
        usec_t scan_monotonic_ut = now_monotonic_usec();

        DICTIONARY *files = dictionary_create(DICT_OPTION_SINGLE_THREADED | DICT_OPTION_DONT_OVERWRITE_VALUE);
        DICTIONARY *dirs = dictionary_create(DICT_OPTION_SINGLE_THREADED|DICT_OPTION_DONT_OVERWRITE_VALUE);

        for(unsigned i = 0; i < MAX_JOURNAL_DIRECTORIES; i++) {
            if(!journal_directories[i].path) break;
            journal_directory_scan_recursively(files, dirs, string2str(journal_directories[i].path), 0);
        }

        const char **array = mallocz(sizeof(const char *) * dictionary_entries(files));
        size_t used = 0;

        void *x;
        dfe_start_read(files, x) {
            if(used >= dictionary_entries(files)) continue;
            array[used++] = x_dfe.name;
        }
        dfe_done(x);

        qsort(array, used, sizeof(const char *), filenames_compar);

        for(size_t i = 0; i < used ;i++) {
            const char *full_path = array[i];

            struct stat info;
            if (stat(full_path, &info) == -1)
                continue;

            struct journal_file t = {
                    .file_last_modified_ut = info.st_mtim.tv_sec * USEC_PER_SEC + info.st_mtim.tv_nsec / NSEC_PER_USEC,
                    .last_scan_monotonic_ut = scan_monotonic_ut,
                    .size = info.st_size,
                    .max_journal_vs_realtime_delta_ut = JOURNAL_VS_REALTIME_DELTA_DEFAULT_UT,
            };
            struct journal_file *jf = dictionary_set(journal_files_registry, full_path, &t, sizeof(t));
            journal_file_update_header(jf->filename, jf);
        }
        freez(array);
        dictionary_destroy(files);
        dictionary_destroy(dirs);

        struct journal_file *jf;
        dfe_start_write(journal_files_registry, jf){
            if(jf->last_scan_monotonic_ut < scan_monotonic_ut)
                dictionary_del(journal_files_registry, jf_dfe.name);
        }
        dfe_done(jf);
        dictionary_garbage_collect(journal_files_registry);

        journal_files_scans++;
        spinlock_unlock(&spinlock);

        internal_error(true,
               "Journal library scan completed in %.3f ms",
                       (double)(now_monotonic_usec() - scan_monotonic_ut) / (double)USEC_PER_MS);
    }
}

// ----------------------------------------------------------------------------

int journal_file_dict_items_backward_compar(const void *a, const void *b) {
    const DICTIONARY_ITEM **ad = (const DICTIONARY_ITEM **)a, **bd = (const DICTIONARY_ITEM **)b;
    struct journal_file *jfa = dictionary_acquired_item_value(*ad);
    struct journal_file *jfb = dictionary_acquired_item_value(*bd);

    // compare the last message timestamps
    if(jfa->msg_last_ut < jfb->msg_last_ut)
        return 1;

    if(jfa->msg_last_ut > jfb->msg_last_ut)
        return -1;

    // compare the file last modification timestamps
    if(jfa->file_last_modified_ut < jfb->file_last_modified_ut)
        return 1;

    if(jfa->file_last_modified_ut > jfb->file_last_modified_ut)
        return -1;

    // compare the first message timestamps
    if(jfa->msg_first_ut < jfb->msg_first_ut)
        return 1;

    if(jfa->msg_first_ut > jfb->msg_first_ut)
        return -1;

    return 0;
}

int journal_file_dict_items_forward_compar(const void *a, const void *b) {
    return -journal_file_dict_items_backward_compar(a, b);
}

static bool boot_id_conflict_cb(const DICTIONARY_ITEM *item __maybe_unused, void *old_value, void *new_value, void *data __maybe_unused) {
    usec_t *old_usec = old_value;
    usec_t *new_usec = new_value;

    if(*new_usec < *old_usec) {
        *old_usec = *new_usec;
        return true;
    }

    return false;
}

void journal_init_files_and_directories(void) {
    unsigned d = 0;

    // ------------------------------------------------------------------------
    // setup the journal directories

    journal_directories[d++].path = string_strdupz("/run/log/journal");
    journal_directories[d++].path = string_strdupz("/var/log/journal");

    if(*netdata_configured_host_prefix) {
        char path[PATH_MAX];
        snprintfz(path, sizeof(path), "%s/var/log/journal", netdata_configured_host_prefix);
        journal_directories[d++].path = string_strdupz(path);
        snprintfz(path, sizeof(path), "%s/run/log/journal", netdata_configured_host_prefix);
        journal_directories[d++].path = string_strdupz(path);
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

    dictionary_register_conflict_callback(boot_ids_to_first_ut, boot_id_conflict_cb, NULL);

}
