// SPDX-License-Identifier: GPL-3.0-or-later


#ifndef NETDATA_PLUGIN_MACOS_H
#define NETDATA_PLUGIN_MACOS_H 1

#include "../../common.h"

#if (TARGET_OS == OS_MACOS)

#define NETDATA_PLUGIN_HOOK_MACOS \
    { \
        .name = "PLUGIN[macos]", \
        .config_section = CONFIG_SECTION_PLUGINS, \
        .config_name = "macos", \
        .enabled = 1, \
        .thread = NULL, \
        .init_routine = NULL, \
        .start_routine = macos_main \
    },

void *macos_main(void *ptr);

#define GETSYSCTL_BY_NAME(name, var) getsysctl_by_name(name, &(var), sizeof(var))

extern int getsysctl_by_name(const char *name, void *ptr, size_t len);

extern int do_macos_sysctl(int update_every, usec_t dt);
extern int do_macos_mach_smi(int update_every, usec_t dt);
extern int do_macos_iokit(int update_every, usec_t dt);


#else // (TARGET_OS == OS_MACOS)

#define NETDATA_PLUGIN_HOOK_MACOS

#endif // (TARGET_OS == OS_MACOS)





#endif /* NETDATA_PLUGIN_MACOS_H */
