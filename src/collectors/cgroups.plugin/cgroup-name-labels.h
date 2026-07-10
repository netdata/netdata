// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_CGROUP_NAME_LABELS_H
#define NETDATA_CGROUP_NAME_LABELS_H

#include "database/rrdlabels.h"

#define CGROUP_NAME_LINE_MAX 8190

// A helper response is one newline-terminated record. The caller rejects a
// full buffer without a newline rather than parsing a truncated record.
bool cgroup_name_line_is_complete(const char *data);

// Parse a resolved "<name> key=\"value\",..." line produced by the cgroup-name
// helper. Adds the label pairs to `labels`, returns the chart name (a pointer
// into `data`, which is modified in place), and sets *ignored to true when an
// ignore="true"/"yes" label requests the cgroup be excluded. `ignored` may be NULL.
char *cgroup_parse_name_and_labels(RRDLABELS *labels, char *data, bool *ignored);

#endif // NETDATA_CGROUP_NAME_LABELS_H
