// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_SOCKET_EGRESS_INTERFACE_H
#define NETDATA_SOCKET_EGRESS_INTERFACE_H

#include "libnetdata/libnetdata.h"

// portable upper bound for an interface name (IF_NAMESIZE is 16 on Linux/BSD/macOS); kept here so
// callers do not need <net/if.h>, which is not available on every platform.
#define OS_IFNAME_MAX 32

/**
 * Find the local network interface a connected socket egresses on.
 *
 * Resolves the socket's local address (getsockname) and matches it against the system interface
 * addresses (getifaddrs), handling plain IPv4, native IPv6, and the dual-stack case where the local
 * IPv4 is reported as an IPv6-mapped address (::ffff:a.b.c.d).
 *
 * @param fd       a connected socket file descriptor
 * @param out      buffer receiving the null-terminated interface name (set empty on failure)
 * @param out_len  capacity of out (use OS_IFNAME_MAX)
 * @return true if an interface was found, false on unsupported platforms, errors, or no match
 */
bool os_socket_egress_interface(int fd, char *out, size_t out_len);

// unit test for the address->interface matching (incl. the IPv6-mapped-IPv4 dual-stack case)
int os_socket_egress_interface_unittest(void);

#endif //NETDATA_SOCKET_EGRESS_INTERFACE_H
