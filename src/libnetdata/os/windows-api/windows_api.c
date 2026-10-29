// SPDX-License-Identifier: GPL-3.0-or-later

#include "windows_api.h"

#include <winsock2.h>
#include <ws2tcpip.h>
#include <iphlpapi.h>
#include <stdlib.h>
#include <string.h>
#include <stdbool.h>

static int netdata_win_get_adapter_addresses(ULONG family, ULONG flags, ULONG max_length, PIP_ADAPTER_ADDRESSES *result)
{
    ULONG length = 15000;
    *result = NULL;

    for (unsigned attempt = 0; attempt < 3; attempt++) {
        if (max_length && length > max_length)
            return -1;

        PIP_ADAPTER_ADDRESSES adapters = malloc(length);
        if (!adapters)
            return -1;

        DWORD rc = GetAdaptersAddresses(family, flags, NULL, adapters, &length);
        if (rc == NO_ERROR) {
            *result = adapters;
            return 1;
        }

        free(adapters);
        if (rc == ERROR_NO_DATA)
            return 0;
        if (rc != ERROR_BUFFER_OVERFLOW)
            return -1;
    }

    return -1;
}

static char *netdata_win_utf16_to_utf8(const WCHAR *value)
{
    if (!value)
        return NULL;

    int bytes = WideCharToMultiByte(CP_UTF8, 0, value, -1, NULL, 0, NULL, NULL);
    if (bytes <= 0)
        return NULL;

    char *text = malloc((size_t)bytes);
    if (!text)
        return NULL;

    if (!WideCharToMultiByte(CP_UTF8, 0, value, -1, text, bytes, NULL, NULL)) {
        free(text);
        return NULL;
    }

    return text;
}

static const char *netdata_win_oper_status(IF_OPER_STATUS status)
{
    switch (status) {
        case IfOperStatusUp:
            return "up";
        case IfOperStatusDown:
            return "down";
        case IfOperStatusTesting:
            return "testing";
        case IfOperStatusUnknown:
            return "unknown";
        case IfOperStatusDormant:
            return "dormant";
        case IfOperStatusNotPresent:
            return "notpresent";
        case IfOperStatusLowerLayerDown:
            return "lowerlayerdown";
        default:
            return NULL;
    }
}

void netdata_win_network_interfaces_free(struct netdata_win_network_interface *interfaces)
{
    while (interfaces) {
        struct netdata_win_network_interface *next = interfaces->next;
        free(interfaces->description);
        free(interfaces->friendly_name);
        free(interfaces);
        interfaces = next;
    }
}

int netdata_win_network_interfaces_get(struct netdata_win_network_interface **interfaces)
{
    *interfaces = NULL;
    PIP_ADAPTER_ADDRESSES adapters = NULL;
    // Addresses are not consumed; skipping them keeps the snapshot small.
    int status = netdata_win_get_adapter_addresses(
        AF_UNSPEC,
        GAA_FLAG_SKIP_UNICAST | GAA_FLAG_SKIP_ANYCAST | GAA_FLAG_SKIP_MULTICAST | GAA_FLAG_SKIP_DNS_SERVER |
            GAA_FLAG_INCLUDE_ALL_INTERFACES,
        16 * 1024 * 1024,
        &adapters);
    if (status != 1)
        return status;

    struct netdata_win_network_interface **tail = interfaces;
    unsigned int count = 0;
    for (PIP_ADAPTER_ADDRESSES aa = adapters; aa; aa = aa->Next) {
        if (++count > 4096) {
            status = -1;
            break;
        }

        struct netdata_win_network_interface *entry = calloc(1, sizeof(*entry));
        if (!entry) {
            status = -1;
            break;
        }

        entry->description = netdata_win_utf16_to_utf8(aa->Description);
        entry->friendly_name = netdata_win_utf16_to_utf8(aa->FriendlyName);
        // Tunnel adapters may lack a description; callers then match by friendly name.
        if ((aa->Description && !entry->description) || (aa->FriendlyName && !entry->friendly_name)) {
            free(entry->description);
            free(entry->friendly_name);
            free(entry);
            status = -1;
            break;
        }

        entry->oper_status = netdata_win_oper_status(aa->OperStatus);
        entry->mtu = aa->Mtu;
        *tail = entry;
        tail = &entry->next;
    }

    free(adapters);
    if (status != 1) {
        netdata_win_network_interfaces_free(*interfaces);
        *interfaces = NULL;
    }
    return status;
}

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

    PIP_ADAPTER_ADDRESSES adapters = NULL;
    int snapshot_status = netdata_win_get_adapter_addresses(AF_INET, GAA_FLAG_INCLUDE_PREFIX, 0, &adapters);
    int result = -1;
    if (snapshot_status == 0) {
        result = 0;
        goto done;
    }
    if (snapshot_status != 1)
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
            *iface = netdata_win_utf16_to_utf8(aa->FriendlyName);
            if (!*iface || !**iface)
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
