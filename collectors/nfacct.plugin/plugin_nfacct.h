// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_NFACCT_H
#define NETDATA_NFACCT_H 1

#include "../../daemon/common.h"

#if defined(INTERNAL_PLUGIN_NFACCT)

#define NETDATA_PLUGIN_HOOK_LINUX_NFACCT \
    { \
        .name = "PLUGIN[nfacct]", \
        .config_section = CONFIG_SECTION_PLUGINS, \
        .config_name = "nfacct", \
        .enabled = 1, \
        .thread = NULL, \
        .init_routine = NULL, \
        .start_routine = nfacct_main \
    },

extern void *nfacct_main(void *ptr);

#else // !defined(INTERNAL_PLUGIN_NFACCT)

#define NETDATA_PLUGIN_HOOK_LINUX_NFACCT

#endif // defined(INTERNAL_PLUGIN_NFACCT)

#endif /* NETDATA_NFACCT_H */

