// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_DEBUGFS_PLUGIN_H
#define NETDATA_DEBUGFS_PLUGIN_H 1

#include "libnetdata/libnetdata.h"
#include "collectors/all.h"
#include "database/rrd.h"

extern netdata_mutex_t stdout_mutex;

void *libsensors_thread(void *ptr);

int do_module_numa_extfrag(int update_every, const char *name);
int do_module_zswap(int update_every, const char *name);
int do_module_devices_powercap(int update_every, const char *name);
int do_module_libsensors(int update_every, const char *name);

void module_libsensors_cleanup(void);

void debugfs2lower(char *name);
const char *debugfs_rrdset_type_name(RRDSET_TYPE chart_type);
const char *debugfs_rrd_algorithm_name(RRD_ALGORITHM algorithm);

#endif // NETDATA_DEBUGFS_PLUGIN_H
