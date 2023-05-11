// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_DEBUGFS_PLUGIN_H
#define NETDATA_DEBUGFS_PLUGIN_H 1

#include "libnetdata/libnetdata.h"
#include "collectors/all.h"
#include "database/rrd.h"

int debugfs_parse_extfrag_index(int update_every, const char *name);
int debugfs_zswap(int update_every, const char *name);
void debugfs2lower(char *name);
const char *debugfs_rrdset_type_name(RRDSET_TYPE chart_type);

#endif // NETDATA_DEBUGFS_PLUGIN_H
