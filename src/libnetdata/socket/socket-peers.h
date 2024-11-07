// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_SOCKET_PEERS_H
#define NETDATA_SOCKET_PEERS_H

#ifndef INET6_ADDRSTRLEN
#define INET6_ADDRSTRLEN 46
#endif

typedef struct {
    struct {
        char ip[INET6_ADDRSTRLEN];
        int port;
    } local;

    struct {
        char ip[INET6_ADDRSTRLEN];
        int port;
    } peer;
} SOCKET_PEERS;

SOCKET_PEERS socket_peers(int sock_fd);

#endif //NETDATA_SOCKET_PEERS_H
