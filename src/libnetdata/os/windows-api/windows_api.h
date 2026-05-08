// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_WINDOWS_API_H
#define NETDATA_WINDOWS_API_H

#define NETDATA_WIN_TCP_STATE_COUNT 12

#if defined(OS_WINDOWS)

#include <stdbool.h>
#include <stdint.h>

char *netdata_win_local_interface();
char *netdata_win_local_ip();
bool netdata_win_collect_tcp_state_counts(uint32_t af, uint32_t state_counts[]);

#endif

#endif //NETDATA_WINDOWS_API_H
