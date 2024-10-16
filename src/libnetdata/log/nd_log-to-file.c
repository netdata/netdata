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

bool nd_logger_file(FILE *fp, ND_LOG_FORMAT format, struct log_field *fields, size_t fields_max) {
    BUFFER *wb = buffer_create(1024, NULL);

    if(format == NDLF_JSON)
        nd_logger_json(wb, fields, fields_max);
    else
        nd_logger_logfmt(wb, fields, fields_max);

    int r = fprintf(fp, "%s\n", buffer_tostring(wb));
    fflush(fp);

    buffer_free(wb);
    return r > 0;
}
