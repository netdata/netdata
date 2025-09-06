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
            default_ip.interface = strdup(aa->FriendlyName);

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

    return default_ip.interface;
}

char *netdata_win_local_ip()
{
    if (!default_ip.initialized)
        netdata_fill_default_ip();

    return default_ip.ipaddr;
}

#endif

