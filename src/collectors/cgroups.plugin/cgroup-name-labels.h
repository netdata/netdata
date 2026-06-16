// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_CGROUP_NAME_LABELS_H
#define NETDATA_CGROUP_NAME_LABELS_H

#include "database/rrdlabels.h"

// Parse a resolved "<name> key=\"value\",..." line produced by the cgroup-name
// helper. Adds the label pairs to `labels`, returns the chart name (a pointer
// into `data`, which is modified in place), and sets *ignored to true when an
// ignore="true"/"yes" label requests the cgroup be excluded. `ignored` may be NULL.
char *cgroup_parse_name_and_labels(RRDLABELS *labels, char *data, bool *ignored);

#endif // NETDATA_CGROUP_NAME_LABELS_H
