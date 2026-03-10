// SPDX-License-Identifier: GPL-3.0-or-later

#include "nd_log-internals.h"

void chown_open_file(int fd, uid_t uid, gid_t gid) {
    if(fd == -1) return;

    struct stat buf;

    if(fstat(fd, &buf) == -1) {
        netdata_log_error("Cannot fstat() fd %d", fd);
        return;
    }

    if((buf.st_uid != uid || buf.st_gid != gid) && S_ISREG(buf.st_mode)) {
        if(fchown(fd, uid, gid) == -1)
            netdata_log_error("Cannot fchown() fd %d.", fd);
    }
}

void nd_log_chown_log_files(uid_t uid, gid_t gid) {
    for(size_t i = 0 ; i < _NDLS_MAX ; i++) {
        if(nd_log.sources[i].fd != -1 && nd_log.sources[i].fd != STDIN_FILENO)
            chown_open_file(nd_log.sources[i].fd, uid, gid);
    }
}

bool nd_logger_file(FILE *fp, netdata_mutex_t *mutex, ND_LOG_FORMAT format, struct log_field *fields, size_t fields_max) {
    BUFFER *wb = buffer_create(1024, NULL);

    if(format == NDLF_JSON)
        nd_logger_json(wb, fields, fields_max);
    else
        nd_logger_logfmt(wb, fields, fields_max);

    buffer_strcat(wb, "\n");

    // Serialize writes with a Netdata-owned mutex and use write() on the raw fd.
    //
    // We avoid libc's stdio locking (flockfile/funlockfile) because spawn-server
    // children inherit FILE* state across fork(). In those post-fork children we
    // disable logger mutexes entirely, since they are single-threaded at that point.
    //
    // A netdata_mutex_t sleeps on contention rather than busy-waiting, so blocked
    // I/O (full pipe, slow disk) does not burn CPU in other logging threads.

    int fd = fileno(fp);
    const char *buf = buffer_tostring(wb);
    size_t remaining = buffer_strlen(wb);

    if(mutex)
        netdata_mutex_lock(mutex);

    while(remaining > 0) {
        ssize_t written = write(fd, buf, remaining);
        if(written >= 0) {
            buf += written;
            remaining -= written;
        }
        else if(errno != EINTR)
            break;
    }

    if(mutex)
        netdata_mutex_unlock(mutex);

    buffer_free(wb);
    return remaining == 0;
}
