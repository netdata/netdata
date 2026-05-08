// SPDX-License-Identifier: GPL-3.0-or-later

#include "windows_api.h"

#include <winsock2.h>
#include <ws2tcpip.h>
#include <iphlpapi.h>
#include <stdlib.h>
#include <string.h>
#include <stdbool.h>

static inline int netdata_win_tcp_state_to_index(DWORD state)
{
    switch(state) {
    case MIB_TCP_STATE_CLOSED:
        return NETDATA_WIN_TCP_STATE_CLOSED;
    case MIB_TCP_STATE_LISTEN:
        return NETDATA_WIN_TCP_STATE_LISTENING;
    case MIB_TCP_STATE_SYN_SENT:
        return NETDATA_WIN_TCP_STATE_SYN_SENT;
    case MIB_TCP_STATE_SYN_RCVD:
        return NETDATA_WIN_TCP_STATE_SYN_RECEIVED;
    case MIB_TCP_STATE_ESTAB:
        return NETDATA_WIN_TCP_STATE_ESTABLISHED;
    case MIB_TCP_STATE_FIN_WAIT1:
        return NETDATA_WIN_TCP_STATE_FIN_WAIT1;
    case MIB_TCP_STATE_FIN_WAIT2:
        return NETDATA_WIN_TCP_STATE_FIN_WAIT2;
    case MIB_TCP_STATE_CLOSE_WAIT:
        return NETDATA_WIN_TCP_STATE_CLOSE_WAIT;
    case MIB_TCP_STATE_CLOSING:
        return NETDATA_WIN_TCP_STATE_CLOSING;
    case MIB_TCP_STATE_LAST_ACK:
        return NETDATA_WIN_TCP_STATE_LAST_ACK;
    case MIB_TCP_STATE_TIME_WAIT:
        return NETDATA_WIN_TCP_STATE_TIME_WAIT;
    case MIB_TCP_STATE_DELETE_TCB:
        return NETDATA_WIN_TCP_STATE_DELETE_TCB;
    default:
        return -1;
    }
}

struct netdata_windows_ip_labels {
    char *local_iface;
    char *ipaddr;
    bool initialized;
} default_ip = {
    .local_iface = NULL,
    .ipaddr = NULL,
    .initialized = false
};

int netdata_fill_default_ip()
{
    if (default_ip.initialized)
        return 0;

    default_ip.initialized = true;

    MIB_IPFORWARDROW route;
    DWORD dest = 0;
    if (GetBestRoute(dest, 0, &route) != NO_ERROR) {
        return -1;
    }

    DWORD ifIndex = route.dwForwardIfIndex;

    ULONG bufLen = 15000;
    PIP_ADAPTER_ADDRESSES adapters = (PIP_ADAPTER_ADDRESSES)malloc(bufLen);
    if (!adapters) {
        return 1;
    }

    int ret = GetAdaptersAddresses(AF_INET, GAA_FLAG_INCLUDE_PREFIX, NULL, adapters, &bufLen);
    if (ret != NO_ERROR) {
        goto end_ip_detection;
    }

    PIP_ADAPTER_ADDRESSES aa = adapters;
    while (aa) {
        if (aa->IfIndex == ifIndex) {
            char iface[1024];
            size_t required_size = wcstombs(NULL , aa->FriendlyName, 0) + 1;
            wcstombs(iface, aa->FriendlyName, required_size);
            default_ip.local_iface = strdup(iface);

            PIP_ADAPTER_UNICAST_ADDRESS ua = aa->FirstUnicastAddress;
            while (ua) {
                if (ua->Address.lpSockaddr->sa_family == AF_INET) {
                    char ipstr[INET_ADDRSTRLEN];
                    struct sockaddr_in *sa_in = (struct sockaddr_in *)ua->Address.lpSockaddr;
                    inet_ntop(AF_INET, &(sa_in->sin_addr), ipstr, sizeof(ipstr));
                    default_ip.ipaddr = strdup(ipstr);
                    goto end_ip_detection;
                }
                ua = ua->Next;
            }
            break;
        }
        aa = aa->Next;
    }

    ret = NO_ERROR;
end_ip_detection:
    free(adapters);
    return ret;
}

char *netdata_win_local_interface()
{
    if (!default_ip.initialized)
        netdata_fill_default_ip();

    return default_ip.local_iface;
}

char *netdata_win_local_ip()
{
    if (!default_ip.initialized)
        netdata_fill_default_ip();

    return default_ip.ipaddr;
}

bool netdata_win_collect_tcp_state_counts(uint32_t af, uint32_t state_counts[])
{
    DWORD size = 0;
    DWORD ret = GetExtendedTcpTable(NULL, &size, FALSE, (DWORD)af, TCP_TABLE_OWNER_PID_ALL, 0);
    if (ret != ERROR_INSUFFICIENT_BUFFER)
        return false;

    void *table = malloc(size);
    if (!table)
        return false;

    ret = GetExtendedTcpTable(table, &size, FALSE, (DWORD)af, TCP_TABLE_OWNER_PID_ALL, 0);
    if (ret != NO_ERROR) {
        free(table);
        return false;
    }

    memset(state_counts, 0, sizeof(uint32_t) * NETDATA_WIN_TCP_STATE_COUNT);

    if (af == AF_INET) {
        PMIB_TCPTABLE_OWNER_PID tcp4 = table;
        for (DWORD i = 0; i < tcp4->dwNumEntries; i++) {
            int state = netdata_win_tcp_state_to_index(tcp4->table[i].dwState);
            if (state >= 0)
                state_counts[state]++;
        }
    }
    else if (af == AF_INET6) {
        PMIB_TCP6TABLE_OWNER_PID tcp6 = table;
        for (DWORD i = 0; i < tcp6->dwNumEntries; i++) {
            int state = netdata_win_tcp_state_to_index(tcp6->table[i].dwState);
            if (state >= 0)
                state_counts[state]++;
        }
    }

    free(table);
    return true;
}
