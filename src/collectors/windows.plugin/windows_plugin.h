// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_WINDOWS_PLUGIN_H
#define NETDATA_WINDOWS_PLUGIN_H

#include "daemon/common.h"

void *win_plugin_main(void *ptr);

int do_GetSystemTimes(int update_every, usec_t dt);

#endif //NETDATA_WINDOWS_PLUGIN_H
