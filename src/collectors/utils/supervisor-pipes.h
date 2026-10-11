// SPDX-License-Identifier: GPL-3.0-or-later
#ifndef NETDATA_SUPERVISOR_PIPES_H
#define NETDATA_SUPERVISOR_PIPES_H
#include <dirent.h>
#include <errno.h>
#include <fcntl.h>
#include <limits.h>
#include <stdbool.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <sys/stat.h>
#include <unistd.h>

static inline bool supervisor_protocol_pipes(int control_fd, int status_fd) {
    struct stat control, status;
    if (control_fd < 3 || status_fd < 3 || control_fd == status_fd ||
        fstat(control_fd, &control) || fstat(status_fd, &status) ||
        !S_ISFIFO(control.st_mode) || !S_ISFIFO(status.st_mode) ||
        (control.st_dev == status.st_dev && control.st_ino == status.st_ino))
        return false;
    int control_flags = fcntl(control_fd, F_GETFL);
    int status_flags = fcntl(status_fd, F_GETFL);
    if (control_flags < 0 || status_flags < 0 ||
        (control_flags & O_ACCMODE) != O_RDONLY || (status_flags & O_ACCMODE) != O_WRONLY)
        return false;

    // Every protocol copy must stay with its owner. An inherited opposite end
    // can hide parent EOF, and an alias can let the payload retain the protocol.
    // Inspect only our descriptors, before any children or concurrent mutation.
    DIR *fds = opendir("/proc/thread-self/fd");
    if (!fds)
        return false;
    bool private = true;
    struct dirent *entry;
    errno = 0;
    while ((entry = readdir(fds))) {
        char *end;
        long fd = strtol(entry->d_name, &end, 10);
        if (*end || fd < 0 || fd > INT_MAX || fd == control_fd || fd == status_fd)
            continue;
        struct stat other;
        if (fstat((int)fd, &other) != 0 ||
            (other.st_dev == control.st_dev && other.st_ino == control.st_ino) ||
            (other.st_dev == status.st_dev && other.st_ino == status.st_ino)) {
            private = false;
            break;
        }
        errno = 0;
    }
    if (errno)
        private = false;
    closedir(fds);
    return private;
}

#endif
