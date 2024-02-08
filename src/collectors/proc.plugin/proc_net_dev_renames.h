// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_PROC_NET_DEV_RENAMES_H
#define NETDATA_PROC_NET_DEV_RENAMES_H

#include "plugin_proc.h"

extern DICTIONARY *netdev_renames;

struct rename_task {
    const char *container_device;
    const char *container_name;
    const char *ctx_prefix;
    RRDLABELS *chart_labels;
    const DICTIONARY_ITEM *cgroup_netdev_link;
};

void netdev_renames_init(void);

void cgroup_netdev_reset_all(void);
void cgroup_netdev_release(const DICTIONARY_ITEM *link);
const void *cgroup_netdev_dup(const DICTIONARY_ITEM *link);
void cgroup_netdev_add_bandwidth(const DICTIONARY_ITEM *link, NETDATA_DOUBLE received, NETDATA_DOUBLE sent);


#endif //NETDATA_PROC_NET_DEV_RENAMES_H
