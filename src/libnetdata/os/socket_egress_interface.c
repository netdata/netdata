// SPDX-License-Identifier: GPL-3.0-or-later

#include "libnetdata/libnetdata.h"

#if defined(HAVE_NET_IF_H) && !defined(OS_WINDOWS)
#include <ifaddrs.h>
#include <arpa/inet.h>

// pure address->interface match, separated from the syscalls so it can be unit-tested with
// synthetic inputs (see os_socket_egress_interface_unittest). Writes the first matching interface
// name into out (capacity out_len) and returns true on match. Handles plain IPv4, native IPv6, and
// the dual-stack case where getsockname reports the local IPv4 as ::ffff:a.b.c.d.
static bool match_egress_interface(const struct sockaddr_storage *local,
                                   const struct ifaddrs *ifaddr, char *out, size_t out_len) {
    for (const struct ifaddrs *ifa = ifaddr; ifa; ifa = ifa->ifa_next) {
        if (!ifa->ifa_addr)
            continue;

        bool match = false;
        if (local->ss_family == AF_INET && ifa->ifa_addr->sa_family == AF_INET)
            match = ((const struct sockaddr_in *)(const void *)local)->sin_addr.s_addr ==
                    ((const struct sockaddr_in *)(void *)ifa->ifa_addr)->sin_addr.s_addr;
        else if (local->ss_family == AF_INET6) {
            const struct sockaddr_in6 *local6 = (const struct sockaddr_in6 *)(const void *)local;
            if (IN6_IS_ADDR_V4MAPPED(&local6->sin6_addr) && ifa->ifa_addr->sa_family == AF_INET) {
                // a dual-stack socket reports its local IPv4 as ::ffff:a.b.c.d - match it against the
                // interface's plain AF_INET address
                struct in_addr mapped4;
                memcpy(&mapped4, &local6->sin6_addr.s6_addr[12], sizeof(mapped4));
                match = mapped4.s_addr ==
                        ((const struct sockaddr_in *)(void *)ifa->ifa_addr)->sin_addr.s_addr;
            }
            else if (ifa->ifa_addr->sa_family == AF_INET6)
                match = memcmp(&local6->sin6_addr,
                               &((const struct sockaddr_in6 *)(void *)ifa->ifa_addr)->sin6_addr,
                               sizeof(struct in6_addr)) == 0;
        }

        if (match) {
            strncpyz(out, ifa->ifa_name, out_len - 1);
            return true;
        }
    }
    return false;
}

bool os_socket_egress_interface(int fd, char *out, size_t out_len) {
    if (fd < 0 || !out || out_len == 0)
        return false;

    out[0] = '\0';

    struct sockaddr_storage local;
    socklen_t local_len = sizeof(local);
    if (getsockname(fd, (struct sockaddr *)&local, &local_len) != 0)
        return false;

    struct ifaddrs *ifaddr;
    if (getifaddrs(&ifaddr) != 0)
        return false;

    bool found = match_egress_interface(&local, ifaddr, out, out_len);
    freeifaddrs(ifaddr);
    return found;
}

int os_socket_egress_interface_unittest(void) {
    fprintf(stderr, "%s() running...\n", __FUNCTION__);

    struct sockaddr_in lo4 = { .sin_family = AF_INET };
    inet_pton(AF_INET, "127.0.0.1", &lo4.sin_addr);
    struct sockaddr_in eth0_4 = { .sin_family = AF_INET };
    inet_pton(AF_INET, "10.20.4.5", &eth0_4.sin_addr);
    struct sockaddr_in6 eth1_6 = { .sin6_family = AF_INET6 };
    inet_pton(AF_INET6, "2001:db8::5", &eth1_6.sin6_addr);

    struct ifaddrs if_eth1 = { .ifa_next = NULL,     .ifa_name = (char *)"eth1", .ifa_addr = (struct sockaddr *)&eth1_6 };
    struct ifaddrs if_eth0 = { .ifa_next = &if_eth1, .ifa_name = (char *)"eth0", .ifa_addr = (struct sockaddr *)&eth0_4 };
    struct ifaddrs if_lo   = { .ifa_next = &if_eth0, .ifa_name = (char *)"lo",   .ifa_addr = (struct sockaddr *)&lo4   };

    static const struct {
        const char *name;
        int family;
        const char *addr;    // an IPv4 string when mapped==true
        bool mapped;         // build ::ffff:addr (a dual-stack local)
        const char *expect;  // NULL = expect no match
    } cases[] = {
        { "ipv4 direct",      AF_INET,  "10.20.4.5",    false, "eth0" },
        { "ipv6 native",      AF_INET6, "2001:db8::5",  false, "eth1" },
        { "ipv4-mapped ipv6", AF_INET6, "10.20.4.5",    true,  "eth0" },
        { "no match ipv4",    AF_INET,  "192.0.2.1",    false, NULL   },
        { "no match ipv6",    AF_INET6, "2001:db8::99", false, NULL   },
    };

    int errors = 0;
    for (size_t i = 0; i < sizeof(cases) / sizeof(cases[0]); i++) {
        struct sockaddr_storage local;
        memset(&local, 0, sizeof(local));

        if (cases[i].family == AF_INET) {
            struct sockaddr_in *l = (struct sockaddr_in *)&local;
            l->sin_family = AF_INET;
            inet_pton(AF_INET, cases[i].addr, &l->sin_addr);
        }
        else {
            struct sockaddr_in6 *l = (struct sockaddr_in6 *)&local;
            l->sin6_family = AF_INET6;
            if (cases[i].mapped) {
                struct in_addr v4;
                inet_pton(AF_INET, cases[i].addr, &v4);
                l->sin6_addr.s6_addr[10] = 0xff;
                l->sin6_addr.s6_addr[11] = 0xff;
                memcpy(&l->sin6_addr.s6_addr[12], &v4, sizeof(v4));
            }
            else
                inet_pton(AF_INET6, cases[i].addr, &l->sin6_addr);
        }

        char iface[OS_IFNAME_MAX] = "";
        bool found = match_egress_interface(&local, &if_lo, iface, sizeof(iface));

        bool ok = cases[i].expect ? (found && strcmp(iface, cases[i].expect) == 0) : !found;
        if (!ok) {
            fprintf(stderr, "  FAILED %s: expected %s, got %s\n", cases[i].name,
                    cases[i].expect ? cases[i].expect : "(none)", found ? iface : "(none)");
            errors++;
        }
    }

    fprintf(stderr, "%s() %s\n", __FUNCTION__, errors ? "FAILED" : "passed");
    return errors;
}

#else // unsupported platform (no getifaddrs / Windows)

bool os_socket_egress_interface(int fd __maybe_unused, char *out, size_t out_len) {
    if (out && out_len)
        out[0] = '\0';
    return false;
}

int os_socket_egress_interface_unittest(void) {
    return 0;
}

#endif
