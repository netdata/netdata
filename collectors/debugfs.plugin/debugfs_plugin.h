// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_DEBUGFS_PLUGIN_H
#define NETDATA_DEBUGFS_PLUGIN_H 1

#include "libnetdata/libnetdata.h"
#include "collectors/all.h"
#include "database/rrd.h"

int do_debugfs_extfrag(int update_every, const char *name);
int do_debugfs_zswap(int update_every, const char *name);
void debugfs2lower(char *name);
const char *debugfs_rrdset_type_name(RRDSET_TYPE chart_type);
const char *debugfs_rrd_algorithm_name(RRD_ALGORITHM algorithm);

#endif // NETDATA_DEBUGFS_PLUGIN_H
