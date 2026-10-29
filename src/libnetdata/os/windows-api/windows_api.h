// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_WINDOWS_API_H
#define NETDATA_WINDOWS_API_H

#include "libnetdata/libnetdata-platform-fwd.h"

#if defined(OS_WINDOWS)

#include <stdbool.h>

// Caller frees returned strings. 1 = value, 0 = absent, -1 = failed.
int netdata_win_default_network(char **iface, char **ipaddr);

struct netdata_win_network_interface {
    char *description;
    char *friendly_name;
    const char *oper_status;
    unsigned int mtu;
    struct netdata_win_network_interface *next;
};

// Returns 1 for a complete snapshot, 0 for no adapters, and -1 on failure.
int netdata_win_network_interfaces_get(struct netdata_win_network_interface **interfaces);
void netdata_win_network_interfaces_free(struct netdata_win_network_interface *interfaces);

#endif

#endif //NETDATA_WINDOWS_API_H
