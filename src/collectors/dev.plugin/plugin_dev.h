// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_PLUGIN_PROC_H
#define NETDATA_PLUGIN_PROC_H 1

#include "database/rrd.h"

#define PLUGIN_DEV_CONFIG_NAME "dev"
#define PLUGIN_DEV_NAME PLUGIN_DEV_CONFIG_NAME ".plugin"

void dev_main(void *ptr);

#endif /* NETDATA_PLUGIN_PROC_H */
