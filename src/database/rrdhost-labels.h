// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_RRDHOST_LABELS_H
#define NETDATA_RRDHOST_LABELS_H

#include "libnetdata/libnetdata.h"
#include "rrdlabels.h"

const char *rrdhost_os_label_value(RRDLABELS *labels, const char *host_os, char *value, size_t value_size);
void reload_host_labels(void);
void rrdhost_set_is_parent_label(void);
int rrdhost_labels_unittest(void);

#endif //NETDATA_RRDHOST_LABELS_H
