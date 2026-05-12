// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_PLUGIN_MACOS_H
#define NETDATA_PLUGIN_MACOS_H 1

#include "database/rrd.h"

int do_macos_sysctl(int update_every, usec_t dt);
int do_macos_mach_smi(int update_every, usec_t dt);
int do_macos_iokit(int update_every, usec_t dt);
int do_macos_power_sources(int update_every, usec_t dt);
int do_macos_powermetrics(int update_every, usec_t dt);

void macos_powermetrics_cleanup(void);

#endif /* NETDATA_PLUGIN_MACOS_H */
