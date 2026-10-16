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

// The first two descriptors are the control/read and status/write pipes. Any
// remaining descriptors are private protocol resources that must not be aliased.
static inline bool supervisor_protocol_pipes(const int *descriptors, size_t count) {
    if (count < 2)
        return false;
    struct stat objects[count];
    for (size_t i = 0; i < count; i++) {
        if (descriptors[i] < 3 || fstat(descriptors[i], &objects[i]) != 0)
            return false;
        for (size_t j = 0; j < i; j++)
            if (objects[i].st_dev == objects[j].st_dev && objects[i].st_ino == objects[j].st_ino)
                return false;
    }
    if (!S_ISFIFO(objects[0].st_mode) || !S_ISFIFO(objects[1].st_mode))
        return false;
    int control_flags = fcntl(descriptors[0], F_GETFL);
    int status_flags = fcntl(descriptors[1], F_GETFL);
    // O_PATH reports O_RDONLY access bits but cannot provide pipe I/O.
    if (control_flags < 0 || status_flags < 0 ||
        ((control_flags | status_flags) & O_PATH) ||
        (control_flags & O_ACCMODE) != O_RDONLY || (status_flags & O_ACCMODE) != O_WRONLY)
        return false;

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
        if (*end || fd < 0 || fd > INT_MAX)
            continue;
        bool owned = false;
        for (size_t i = 0; i < count; i++)
            owned = owned || fd == descriptors[i];
        if (owned)
            continue;
        struct stat other;
        if (fstat((int)fd, &other) != 0) {
            private = false;
            break;
        }
        for (size_t i = 0; i < count; i++)
            if (other.st_dev == objects[i].st_dev && other.st_ino == objects[i].st_ino)
                private = false;
        if (!private)
            break;
        errno = 0;
    }
    if (errno)
        private = false;
    closedir(fds);
    return private;
}

#endif
