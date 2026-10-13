// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_WINDOWS_API_H
#define NETDATA_WINDOWS_API_H

#if defined(OS_WINDOWS)

#include <stdbool.h>

// Caller frees returned strings. 1 = value, 0 = absent, -1 = failed.
int netdata_win_default_network(char **iface, char **ipaddr);

#endif

#endif //NETDATA_WINDOWS_API_H
