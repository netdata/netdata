// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_PLUGIN_PROC_H
#define NETDATA_PLUGIN_PROC_H 1

#include "collectors/all.h"
#include "libnetdata/libnetdata.h"

#define PLUGIN_DEV_CONFIG_NAME "dev"
#define PLUGIN_DEV_NAME PLUGIN_DEV_CONFIG_NAME ".plugin"

static inline bool dev_plugin_stop(void)
{
    return nd_thread_signaled_to_cancel();
}

#endif /* NETDATA_PLUGIN_PROC_H */
