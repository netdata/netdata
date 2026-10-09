// SPDX-License-Identifier: GPL-3.0-or-later

#include "windows_api.h"

#include <winsock2.h>
#include <ws2tcpip.h>
#include <iphlpapi.h>
#include <stdlib.h>
#include <string.h>
#include <stdbool.h>


// Caller owns both strings. Return 1 for a complete tuple, 0 for successful
// absence, and -1 for failure; no failed probe can poison a process-wide cache.
int netdata_win_default_network(char **iface, char **ipaddr)
{
    *iface = NULL;
    *ipaddr = NULL;
    MIB_IPFORWARDROW route;
    DWORD rc = GetBestRoute(0, 0, &route);
    if (rc == ERROR_HOST_UNREACHABLE || rc == ERROR_NETWORK_UNREACHABLE || rc == ERROR_NO_DATA)
        return 0;
    if (rc != NO_ERROR)
        return -1;

    ULONG length = 15000;
    PIP_ADAPTER_ADDRESSES adapters = NULL;
    for (unsigned attempt = 0; attempt < 3; attempt++) {
        free(adapters);
        adapters = malloc(length);
        if (!adapters)
            return -1;
        rc = GetAdaptersAddresses(AF_INET, GAA_FLAG_INCLUDE_PREFIX, NULL, adapters, &length);
        if (rc != ERROR_BUFFER_OVERFLOW)
            break;
    }
    int result = -1;
    if (rc != NO_ERROR)
        goto done;

    for (PIP_ADAPTER_ADDRESSES aa = adapters; aa; aa = aa->Next) {
        if (aa->IfIndex != route.dwForwardIfIndex)
            continue;
        // A route/adaptor race is a failed probe. A present adapter without a
        // usable IPv4 address is a successfully observed absence.
        result = 0;
        for (PIP_ADAPTER_UNICAST_ADDRESS ua = aa->FirstUnicastAddress; ua; ua = ua->Next) {
            if (!ua->Address.lpSockaddr || ua->Address.lpSockaddr->sa_family != AF_INET)
                continue;
            result = -1;
            if (!aa->FriendlyName)
                goto done;
            // FriendlyName is UTF-16; conversion must not depend on the process locale.
            int size = WideCharToMultiByte(CP_UTF8, 0, aa->FriendlyName, -1, NULL, 0, NULL, NULL);
            if (size <= 1)
                goto done;
            *iface = malloc(size);
            if (!*iface || !WideCharToMultiByte(CP_UTF8, 0, aa->FriendlyName, -1, *iface, size, NULL, NULL))
                goto done;
            char address[INET_ADDRSTRLEN];
            struct sockaddr_in *sa = (struct sockaddr_in *)ua->Address.lpSockaddr;
            if (!inet_ntop(AF_INET, &sa->sin_addr, address, sizeof(address)))
                goto done;
            *ipaddr = strdup(address);
            if (*ipaddr)
                result = 1;
            goto done;
        }
        break;
    }
done:
    free(adapters);
    if (result != 1) {
        free(*iface);
        free(*ipaddr);
        *iface = NULL;
        *ipaddr = NULL;
    }
    return result;
}
