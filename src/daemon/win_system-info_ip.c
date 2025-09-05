// SPDX-License-Identifier: GPL-3.0-or-later

#ifdef OS_WINDOWS

#include <winsock2.h>
#include <ws2tcpip.h>
#include <iphlpapi.h>

static struct windows_ip_labels {
    char *interface;
    char *ipaddr;
    bool initialized;
} default_ip = {
    .interface = NULL,
    .ipaddr = NULL,
    .initialized = false;
};

static unsigned int netdata_fill_default_ip()
{
    if (default_ip.initialized)
        return 0;

    default_ip.initialized = true;

    return 0;
}

char *netdata_win_local_interface()
{
    if (!default_ip.initialized)
        netdata_fill_default_ip();

    return default_ip.interface;
}

char *netdata_win_local_ip()
{
    if (!default_ip.initialized)
        netdata_fill_default_ip();

    return default_ip.ipaddr;
}

#endif

