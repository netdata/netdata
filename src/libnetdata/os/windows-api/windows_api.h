// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_WINDOWS_API_H
#define NETDATA_WINDOWS_API_H

#if defined(OS_WINDOWS)

char *netdata_win_local_interface();
char *netdata_win_local_ip();

#endif

#endif //NETDATA_WINDOWS_API_H
