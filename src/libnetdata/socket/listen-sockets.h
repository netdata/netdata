// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_LISTEN_SOCKETS_H
#define NETDATA_LISTEN_SOCKETS_H

#include "libnetdata/common.h"

#ifndef MAX_LISTEN_FDS
#define MAX_LISTEN_FDS 50
#endif

typedef struct listen_sockets {
    struct config *config;              // the config file to use
    const char *config_section;         // the netdata configuration section to read settings from
    const char *default_bind_to;        // the default bind to configuration string
    uint16_t default_port;              // the default port to use
    int backlog;                        // the default listen backlog to use

    size_t opened;                      // the number of sockets opened
    size_t failed;                      // the number of sockets attempted to open, but failed
    int fds[MAX_LISTEN_FDS];            // the open sockets
    char *fds_names[MAX_LISTEN_FDS];    // descriptions for the open sockets
    int fds_types[MAX_LISTEN_FDS];      // the socktype for the open sockets (SOCK_STREAM, SOCK_DGRAM)
    int fds_families[MAX_LISTEN_FDS];   // the family of the open sockets (AF_UNIX, AF_INET, AF_INET6)
    HTTP_ACL fds_acl_flags[MAX_LISTEN_FDS];  // the acl to apply to the open sockets (dashboard, badges, streaming, netdata.conf, management)
} LISTEN_SOCKETS;

int listen_sockets_setup(LISTEN_SOCKETS *sockets);
void listen_sockets_close(LISTEN_SOCKETS *sockets);

#endif //NETDATA_LISTEN_SOCKETS_H
