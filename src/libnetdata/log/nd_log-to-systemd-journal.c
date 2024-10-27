// SPDX-License-Identifier: GPL-3.0-or-later

#include "nd_log-internals.h"

bool nd_log_journal_systemd_init(void) {
#ifdef HAVE_SYSTEMD
    nd_log.journal.initialized = true;
#else
    nd_log.journal.initialized = false;
#endif

    return nd_log.journal.initialized;
}

static int nd_log_journal_direct_fd_find_and_open(char *filename, size_t size) {
    int fd;

    if(netdata_configured_host_prefix && *netdata_configured_host_prefix) {
        journal_construct_path(filename, size, netdata_configured_host_prefix, "netdata");
        if (is_path_unix_socket(filename) && (fd = journal_direct_fd(filename)) != -1)
            return fd;

        journal_construct_path(filename, size, netdata_configured_host_prefix, NULL);
        if (is_path_unix_socket(filename) && (fd = journal_direct_fd(filename)) != -1)
            return fd;
    }

    journal_construct_path(filename, size, NULL, "netdata");
    if (is_path_unix_socket(filename) && (fd = journal_direct_fd(filename)) != -1)
        return fd;

    journal_construct_path(filename, size, NULL, NULL);
    if (is_path_unix_socket(filename) && (fd = journal_direct_fd(filename)) != -1)
        return fd;

    return -1;
}

bool nd_log_journal_socket_available(void) {
    char filename[FILENAME_MAX];
    int fd = nd_log_journal_direct_fd_find_and_open(filename, sizeof(filename));
    if(fd == -1) return false;
    close(fd);
    return true;
}

static void nd_log_journal_direct_set_env(void) {
    if(nd_log.sources[NDLS_COLLECTORS].method == NDLM_JOURNAL)
        nd_setenv("NETDATA_SYSTEMD_JOURNAL_PATH", nd_log.journal_direct.filename, 1);
}

bool nd_log_journal_direct_init(const char *path) {
    if(nd_log.journal_direct.initialized) {
        nd_log_journal_direct_set_env();
        return true;
    }

    int fd;
    char filename[FILENAME_MAX];
    if(!is_path_unix_socket(path))
        fd = nd_log_journal_direct_fd_find_and_open(filename, sizeof(filename));
    else {
        snprintfz(filename, sizeof(filename), "%s", path);
        fd = journal_direct_fd(filename);
    }

    if(fd < 0)
        return false;

    nd_log.journal_direct.fd = fd;
    nd_log.journal_direct.initialized = true;

    strncpyz(nd_log.journal_direct.filename, filename, sizeof(nd_log.journal_direct.filename) - 1);
    nd_log_journal_direct_set_env();

    return true;
}

bool nd_logger_journal_libsystemd(struct log_field *fields __maybe_unused, size_t fields_max __maybe_unused) {
#ifdef HAVE_SYSTEMD

    //  --- FIELD_PARSER_VERSIONS ---
    //
    // IMPORTANT:
    // THERE ARE 6 VERSIONS OF THIS CODE
    //
    // 1. journal (direct socket API),
    // 2. journal (libsystemd API),
    // 3. logfmt,
    // 4. json,
    // 5. convert to uint64
    // 6. convert to int64
    //
    // UPDATE ALL OF THEM FOR NEW FEATURES OR FIXES

    struct iovec iov[fields_max];
    int iov_count = 0;

    memset(iov, 0, sizeof(iov));

    CLEAN_BUFFER *tmp = NULL;

    for (size_t i = 0; i < fields_max; i++) {
        if (!fields[i].entry.set || !fields[i].journal)
            continue;

        const char *key = fields[i].journal;
        char *value = NULL;
        int rc = 0;
        switch (fields[i].entry.type) {
            case NDFT_TXT:
                if(*fields[i].entry.txt)
                    rc = asprintf(&value, "%s=%s", key, fields[i].entry.txt);
                break;
            case NDFT_STR:
                rc = asprintf(&value, "%s=%s", key, string2str(fields[i].entry.str));
                break;
            case NDFT_BFR:
                if(buffer_strlen(fields[i].entry.bfr))
                    rc = asprintf(&value, "%s=%s", key, buffer_tostring(fields[i].entry.bfr));
                break;
            case NDFT_U64:
                rc = asprintf(&value, "%s=%" PRIu64, key, fields[i].entry.u64);
                break;
            case NDFT_I64:
                rc = asprintf(&value, "%s=%" PRId64, key, fields[i].entry.i64);
                break;
            case NDFT_DBL:
                rc = asprintf(&value, "%s=%f", key, fields[i].entry.dbl);
                break;
            case NDFT_UUID:
                if(!uuid_is_null(*fields[i].entry.uuid)) {
                    char u[UUID_COMPACT_STR_LEN];
                    uuid_unparse_lower_compact(*fields[i].entry.uuid, u);
                    rc = asprintf(&value, "%s=%s", key, u);
                }
                break;
            case NDFT_CALLBACK: {
                if(!tmp)
                    tmp = buffer_create(1024, NULL);
                else
                    buffer_flush(tmp);
                if(fields[i].entry.cb.formatter(tmp, fields[i].entry.cb.formatter_data))
                    rc = asprintf(&value, "%s=%s", key, buffer_tostring(tmp));
            }
            break;
            default:
                rc = asprintf(&value, "%s=%s", key, "UNHANDLED");
                break;
        }

        if (rc != -1 && value) {
            iov[iov_count].iov_base = value;
            iov[iov_count].iov_len = strlen(value);
            iov_count++;
        }
    }

    static bool sockets_before[1024];
    bool detect_systemd_socket = __atomic_load_n(&nd_log.journal.first_msg, __ATOMIC_RELAXED) == false;
    if(detect_systemd_socket) {
        for(int i = 3 ; (size_t)i < _countof(sockets_before); i++)
            sockets_before[i] = fd_is_socket(i);
    }

    int r = sd_journal_sendv(iov, iov_count);

    if(r == 0 && detect_systemd_socket) {
        __atomic_store_n(&nd_log.journal.first_msg, true, __ATOMIC_RELAXED);

        // this is the first successful libsystemd log
        // let's detect its fd number (we need it for the spawn server)

        for(int i = 3 ; (size_t)i < _countof(sockets_before); i++) {
            if (!sockets_before[i] && fd_is_socket(i)) {
                nd_log.journal.fd = i;
                break;
            }
        }
    }

    // Clean up allocated memory
    for (int i = 0; i < iov_count; i++) {
        if (iov[i].iov_base != NULL) {
            free(iov[i].iov_base);
        }
    }

    return r == 0;
#else
    return false;
#endif
}

bool nd_logger_journal_direct(struct log_field *fields, size_t fields_max) {
    if(!nd_log.journal_direct.initialized)
        return false;

    //  --- FIELD_PARSER_VERSIONS ---
    //
    // IMPORTANT:
    // THERE ARE 6 VERSIONS OF THIS CODE
    //
    // 1. journal (direct socket API),
    // 2. journal (libsystemd API),
    // 3. logfmt,
    // 4. json,
    // 5. convert to uint64
    // 6. convert to int64
    //
    // UPDATE ALL OF THEM FOR NEW FEATURES OR FIXES

    CLEAN_BUFFER *wb = buffer_create(4096, NULL);
    CLEAN_BUFFER *tmp = NULL;

    for (size_t i = 0; i < fields_max; i++) {
        if (!fields[i].entry.set || !fields[i].journal)
            continue;

        const char *key = fields[i].journal;

        const char *s = NULL;
        switch(fields[i].entry.type) {
            case NDFT_TXT:
                s = fields[i].entry.txt;
                break;
            case NDFT_STR:
                s = string2str(fields[i].entry.str);
                break;
            case NDFT_BFR:
                s = buffer_tostring(fields[i].entry.bfr);
                break;
            case NDFT_U64:
                buffer_strcat(wb, key);
                buffer_putc(wb, '=');
                buffer_print_uint64(wb, fields[i].entry.u64);
                buffer_putc(wb, '\n');
                break;
            case NDFT_I64:
                buffer_strcat(wb, key);
                buffer_putc(wb, '=');
                buffer_print_int64(wb, fields[i].entry.i64);
                buffer_putc(wb, '\n');
                break;
            case NDFT_DBL:
                buffer_strcat(wb, key);
                buffer_putc(wb, '=');
                buffer_print_netdata_double(wb, fields[i].entry.dbl);
                buffer_putc(wb, '\n');
                break;
            case NDFT_UUID:
                if(!uuid_is_null(*fields[i].entry.uuid)) {
                    char u[UUID_COMPACT_STR_LEN];
                    uuid_unparse_lower_compact(*fields[i].entry.uuid, u);
                    buffer_strcat(wb, key);
                    buffer_putc(wb, '=');
                    buffer_fast_strcat(wb, u, sizeof(u) - 1);
                    buffer_putc(wb, '\n');
                }
                break;
            case NDFT_CALLBACK: {
                if(!tmp)
                    tmp = buffer_create(1024, NULL);
                else
                    buffer_flush(tmp);
                if(fields[i].entry.cb.formatter(tmp, fields[i].entry.cb.formatter_data))
                    s = buffer_tostring(tmp);
                else
                    s = NULL;
            }
            break;
            default:
                s = "UNHANDLED";
                break;
        }

        if(s && *s) {
            buffer_strcat(wb, key);
            if(!strchr(s, '\n')) {
                buffer_putc(wb, '=');
                buffer_strcat(wb, s);
                buffer_putc(wb, '\n');
            }
            else {
                buffer_putc(wb, '\n');
                size_t size = strlen(s);
                uint64_t le_size = htole64(size);
                buffer_memcat(wb, &le_size, sizeof(le_size));
                buffer_memcat(wb, s, size);
                buffer_putc(wb, '\n');
            }
        }
    }

    return journal_direct_send(nd_log.journal_direct.fd, buffer_tostring(wb), buffer_strlen(wb));
}
