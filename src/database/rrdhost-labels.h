// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_RRDHOST_LABELS_H
#define NETDATA_RRDHOST_LABELS_H

#include "libnetdata/libnetdata.h"
#include "rrdlabels.h"

struct rrdhost;
struct rrdhost_system_info;

void reload_host_labels(void);
void rrdhost_set_is_parent_label(void);
void rrdhost_labels_changed(struct rrdhost *host);
bool rrdhost_refresh_system_info(struct rrdhost *host, struct rrdhost_system_info *candidate);
struct rrdhost_system_info *rrdhost_system_info_labels_snapshot(struct rrdhost *host, RRDLABELS **labels);
int rrdhost_labels_unittest(void);

#endif //NETDATA_RRDHOST_LABELS_H
