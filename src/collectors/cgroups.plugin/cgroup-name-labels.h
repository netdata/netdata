// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_CGROUP_NAME_LABELS_H
#define NETDATA_CGROUP_NAME_LABELS_H

#include "database/rrdlabels.h"

#define CGROUP_NAME_LINE_MAX 8190

typedef enum {
    CGROUP_NAME_READ_ERROR = -1,
    CGROUP_NAME_READ_TIMEOUT = 0,
    CGROUP_NAME_READ_COMPLETE = 1,
    CGROUP_NAME_READ_EMPTY = 2,
    CGROUP_NAME_READ_INVALID = 3,
} CGROUP_NAME_READ_RESULT;

// Read one bounded newline-terminated record without letting a partial writer
// reset or bypass the caller's deadline. A negative timeout is unbounded.
CGROUP_NAME_READ_RESULT cgroup_name_read_response(int fd, char *buffer, size_t size, int timeout_ms);

// Parse a resolved "<name> key=\"value\",..." line produced by the cgroup-name
// helper. Adds the label pairs to `labels`, returns the chart name (a pointer
// into `data`, which is modified in place), and sets *ignored to true when an
// ignore="true"/"yes" label requests the cgroup be excluded. `ignored` may be NULL.
char *cgroup_parse_name_and_labels(RRDLABELS *labels, char *data, bool *ignored);

#endif // NETDATA_CGROUP_NAME_LABELS_H
