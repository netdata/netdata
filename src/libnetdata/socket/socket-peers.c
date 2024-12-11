// SPDX-License-Identifier: GPL-3.0-or-later

#include "libnetdata/libnetdata.h"

SOCKET_PEERS socket_peers(int sock_fd) {
    SOCKET_PEERS peers;

    if(sock_fd < 0) {
        strncpyz(peers.peer.ip, "not connected", sizeof(peers.peer.ip) - 1);
        peers.peer.port = 0;

        strncpyz(peers.local.ip, "not connected", sizeof(peers.local.ip) - 1);
        peers.local.port = 0;

        return peers;
    }

    struct sockaddr_storage addr;
    socklen_t addr_len = sizeof(addr);

    // Get peer info
    if (getpeername(sock_fd, (struct sockaddr *)&addr, &addr_len) == 0) {
        if (addr.ss_family == AF_INET) {  // IPv4
            struct sockaddr_in *s = (struct sockaddr_in *)&addr;
            inet_ntop(AF_INET, &s->sin_addr, peers.peer.ip, sizeof(peers.peer.ip));
            peers.peer.port = ntohs(s->sin_port);
        }
        else {  // IPv6
            struct sockaddr_in6 *s = (struct sockaddr_in6 *)&addr;
            inet_ntop(AF_INET6, &s->sin6_addr, peers.peer.ip, sizeof(peers.peer.ip));
            peers.peer.port = ntohs(s->sin6_port);
        }
    }
    else {
        strncpyz(peers.peer.ip, "unknown", sizeof(peers.peer.ip) - 1);
        peers.peer.port = 0;
    }

    // Get local info
    addr_len = sizeof(addr);
    if (getsockname(sock_fd, (struct sockaddr *)&addr, &addr_len) == 0) {
        if (addr.ss_family == AF_INET) {  // IPv4
            struct sockaddr_in *s = (struct sockaddr_in *) &addr;
            inet_ntop(AF_INET, &s->sin_addr, peers.local.ip, sizeof(peers.local.ip));
            peers.local.port = ntohs(s->sin_port);
        } else {  // IPv6
            struct sockaddr_in6 *s = (struct sockaddr_in6 *) &addr;
            inet_ntop(AF_INET6, &s->sin6_addr, peers.local.ip, sizeof(peers.local.ip));
            peers.local.port = ntohs(s->sin6_port);
        }
    }
    else {
        strncpyz(peers.local.ip, "unknown", sizeof(peers.local.ip) - 1);
        peers.local.port = 0;
    }

    return peers;
}
