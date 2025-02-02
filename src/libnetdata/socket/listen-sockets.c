// SPDX-License-Identifier: GPL-3.0-or-later

#include "libnetdata/libnetdata.h"
#include "listen-sockets.h"

static HTTP_ACL socket_ssl_acl(char *acl) {
    char *ssl = strchr(acl,'^');
    if(ssl) {
        //Due the format of the SSL command it is always the last command,
        //we finish it here to avoid problems with the ACLs
        *ssl = '\0';
        ssl++;
        if (!strncmp("SSL=",ssl,4)) {
            ssl += 4;
            if (!strcmp(ssl,"optional")) {
                return HTTP_ACL_SSL_OPTIONAL;
            }
            else if (!strcmp(ssl,"force")) {
                return HTTP_ACL_SSL_FORCE;
            }
        }
    }

    return HTTP_ACL_NONE;
}

static HTTP_ACL read_acl(char *st) {
    HTTP_ACL ret = socket_ssl_acl(st);

    if (!strcmp(st,"dashboard")) ret |= HTTP_ACL_DASHBOARD;
    if (!strcmp(st,"registry")) ret |= HTTP_ACL_REGISTRY;
    if (!strcmp(st,"badges")) ret |= HTTP_ACL_BADGES;
    if (!strcmp(st,"management")) ret |= HTTP_ACL_MANAGEMENT;
    if (!strcmp(st,"streaming")) ret |= HTTP_ACL_STREAMING;
    if (!strcmp(st,"netdata.conf")) ret |= HTTP_ACL_NETDATACONF;

    return ret;
}

static char *strdup_client_description(int family, const char *protocol, const char *ip, uint16_t port) {
    char buffer[100 + 1];

    switch(family) {
        case AF_INET:
            snprintfz(buffer, sizeof(buffer) - 1, "%s:%s:%d", protocol, ip, port);
            break;

        case AF_INET6:
        default:
            snprintfz(buffer, sizeof(buffer) - 1, "%s:[%s]:%d", protocol, ip, port);
            break;

        case AF_UNIX:
            snprintfz(buffer, sizeof(buffer) - 1, "%s:%s", protocol, ip);
            break;
    }

    return strdupz(buffer);
}

static int create_listen_socket_unix(const char *path, int listen_backlog) {
    int sock;

    sock = socket(AF_UNIX, SOCK_STREAM | DEFAULT_SOCKET_FLAGS, 0);
    if(sock < 0) {
        nd_log(NDLS_DAEMON, NDLP_ERR,
               "LISTENER: UNIX socket() on path '%s' failed.",
               path);

        return -1;
    }

    if(sock_setnonblock(sock, true) != 1)
        nd_log(NDLS_DAEMON, NDLP_ERR,
               "LISTENER: UNIX socket on path '%s' failed to set non-blocking mode.",
               path);

    sock_setcloexec(sock, true);
    sock_enlarge_rcv_buf(sock);

    struct sockaddr_un name;
    memset(&name, 0, sizeof(struct sockaddr_un));
    name.sun_family = AF_UNIX;
    strncpyz(name.sun_path, path, sizeof(name.sun_path) - 1);

    errno_clear();
    if (unlink(path) == -1 && errno != ENOENT)
        nd_log(NDLS_DAEMON, NDLP_ERR,
               "LISTENER: failed to remove existing (probably obsolete or left-over) file on UNIX socket path '%s'.",
               path);

    if(bind (sock, (struct sockaddr *) &name, sizeof (name)) < 0) {
        close(sock);
        nd_log(NDLS_DAEMON, NDLP_ERR,
               "LISTENER: UNIX bind() on path '%s' failed.",
               path);

        return -1;
    }

    // we have to chmod this to 0777 so that the client will be able
    // to read from and write to this socket.
    if(chmod(path, 0777) == -1)
        nd_log(NDLS_DAEMON, NDLP_ERR,
               "LISTENER: failed to chmod() socket file '%s'.",
               path);

    if(listen(sock, listen_backlog) < 0) {
        close(sock);
        nd_log(NDLS_DAEMON, NDLP_ERR,
               "LISTENER: UNIX listen() on path '%s' failed.",
               path);

        return -1;
    }

    return sock;
}

static int create_listen_socket4(int socktype, const char *ip, uint16_t port, int listen_backlog) {
    int sock;

    sock = socket(AF_INET, socktype | DEFAULT_SOCKET_FLAGS, 0);
    if(sock < 0) {
        nd_log(NDLS_DAEMON, NDLP_ERR,
               "LISTENER: IPv4 socket() on ip '%s' port %d, socktype %d failed.",
               ip, port, socktype);

        return -1;
    }

    if(sock_setreuse_addr(sock, true) != 1)
        nd_log(NDLS_DAEMON, NDLP_ERR,
               "LISTENER: IPv4 socket on ip '%s' port %d, socktype %d failed to enable reuse address.",
               ip, port, socktype);

    if(sock_setreuse_port(sock, false) == 1) // -1 means not supported
        nd_log(NDLS_DAEMON, NDLP_ERR,
               "LISTENER: IPv4 socket on ip '%s' port %d, socktype %d failed to disable reuse port.",
               ip, port, socktype);

    if(sock_setnonblock(sock, true) != 1)
        nd_log(NDLS_DAEMON, NDLP_ERR,
               "LISTENER: IPv4 socket on ip '%s' port %d, socktype %d failed to set non-blocking mode.",
               ip, port, socktype);

    sock_setcloexec(sock, true);
    sock_enlarge_rcv_buf(sock);

    struct sockaddr_in name;
    memset(&name, 0, sizeof(struct sockaddr_in));
    name.sin_family = AF_INET;
    name.sin_port = htons (port);

    int ret = inet_pton(AF_INET, ip, (void *)&name.sin_addr.s_addr);
    if(ret != 1) {
        nd_log(NDLS_DAEMON, NDLP_ERR,
               "LISTENER: Failed to convert IP '%s' to a valid IPv4 address.",
               ip);

        close(sock);
        return -1;
    }

    if(bind (sock, (struct sockaddr *) &name, sizeof (name)) < 0) {
        close(sock);
        nd_log(NDLS_DAEMON, NDLP_ERR,
               "LISTENER: IPv4 bind() on ip '%s' port %d, socktype %d failed.",
               ip, port, socktype);

        return -1;
    }

    if(socktype == SOCK_STREAM && listen(sock, listen_backlog) < 0) {
        close(sock);
        nd_log(NDLS_DAEMON, NDLP_ERR,
               "LISTENER: IPv4 listen() on ip '%s' port %d, socktype %d failed.",
               ip, port, socktype);

        return -1;
    }

    // Add TCP_DEFER_ACCEPT for TCP sockets
    if(socktype == SOCK_STREAM)
        sock_set_tcp_defer_accept(sock, true);

    nd_log(NDLS_DAEMON, NDLP_DEBUG,
           "LISTENER: Listening on IPv4 ip '%s' port %d, socktype %d",
           ip, port, socktype);

    return sock;
}

static int create_listen_socket6(int socktype, uint32_t scope_id, const char *ip, int port, int listen_backlog) {
    int sock;
    int ipv6only = 1;

    sock = socket(AF_INET6, socktype | DEFAULT_SOCKET_FLAGS, 0);
    if (sock < 0) {
        nd_log(NDLS_DAEMON, NDLP_ERR,
               "LISTENER: IPv6 socket() on ip '%s' port %d, socktype %d, failed.",
               ip, port, socktype);

        return -1;
    }

    if(sock_setreuse_addr(sock, true) != 1)
        nd_log(NDLS_DAEMON, NDLP_ERR,
               "LISTENER: IPv6 socket on ip '%s' port %d, socktype %d failed to set reuse address.",
               ip, port, socktype);

    if(sock_setreuse_port(sock, false) == 1) // -1 means not supported
        nd_log(NDLS_DAEMON, NDLP_ERR,
               "LISTENER: IPv6 socket on ip '%s' port %d, socktype %d failed to disable reuse port.",
               ip, port, socktype);

    if(sock_setnonblock(sock, true) != 1)
        nd_log(NDLS_DAEMON, NDLP_ERR,
               "LISTENER: IPv6 socket on ip '%s' port %d, socktype %d, failed to set non-blocking mode.",
               ip, port, socktype);

    sock_setcloexec(sock, true);
    sock_enlarge_rcv_buf(sock);

    /* IPv6 only */
    if(setsockopt(sock, IPPROTO_IPV6, IPV6_V6ONLY, (void*)&ipv6only, sizeof(ipv6only)) != 0)
        nd_log(NDLS_DAEMON, NDLP_ERR,
               "LISTENER: Cannot set IPV6_V6ONLY on ip '%s' port %d, socktype %d.",
               ip, port, socktype);

    struct sockaddr_in6 name;
    memset(&name, 0, sizeof(struct sockaddr_in6));
    name.sin6_family = AF_INET6;
    name.sin6_port = htons ((uint16_t) port);
    name.sin6_scope_id = scope_id;

    int ret = inet_pton(AF_INET6, ip, (void *)&name.sin6_addr.s6_addr);
    if(ret != 1) {
        nd_log(NDLS_DAEMON, NDLP_ERR,
               "LISTENER: Failed to convert IP '%s' to a valid IPv6 address.",
               ip);

        close(sock);
        return -1;
    }

    name.sin6_scope_id = scope_id;

    if (bind (sock, (struct sockaddr *) &name, sizeof (name)) < 0) {
        close(sock);
        nd_log(NDLS_DAEMON, NDLP_ERR,
               "LISTENER: IPv6 bind() on ip '%s' port %d, socktype %d failed.",
               ip, port, socktype);

        return -1;
    }

    if (socktype == SOCK_STREAM && listen(sock, listen_backlog) < 0) {
        close(sock);
        nd_log(NDLS_DAEMON, NDLP_ERR,
               "LISTENER: IPv6 listen() on ip '%s' port %d, socktype %d failed.",
               ip, port, socktype);

        return -1;
    }

    // Add TCP_DEFER_ACCEPT for TCP sockets
    if(socktype == SOCK_STREAM)
        sock_set_tcp_defer_accept(sock, true);

    nd_log(NDLS_DAEMON, NDLP_DEBUG,
           "LISTENER: Listening on IPv6 ip '%s' port %d, socktype %d",
           ip, port, socktype);

    return sock;
}

static inline int listen_sockets_add(LISTEN_SOCKETS *sockets, int fd, int family, int socktype, const char *protocol, const char *ip, uint16_t port, int acl_flags) {
    if(sockets->opened >= MAX_LISTEN_FDS) {
        nd_log(NDLS_DAEMON, NDLP_ERR,
               "LISTENER: Too many listening sockets. Failed to add listening %s socket at ip '%s' port %d, protocol %s, socktype %d",
               protocol, ip, port, protocol, socktype);

        close(fd);
        return -1;
    }

    sockets->fds[sockets->opened] = fd;
    sockets->fds_types[sockets->opened] = socktype;
    sockets->fds_families[sockets->opened] = family;
    sockets->fds_names[sockets->opened] = strdup_client_description(family, protocol, ip, port);
    sockets->fds_acl_flags[sockets->opened] = acl_flags;

    sockets->opened++;
    return 0;
}

static inline int listen_sockets_check_is_member(LISTEN_SOCKETS *sockets, int fd) {
    size_t i;
    for(i = 0; i < sockets->opened ;i++)
        if(sockets->fds[i] == fd) return 1;

    return 0;
}

static inline void listen_sockets_init(LISTEN_SOCKETS *sockets) {
    size_t i;
    for(i = 0; i < MAX_LISTEN_FDS ;i++) {
        sockets->fds[i] = -1;
        sockets->fds_names[i] = NULL;
        sockets->fds_types[i] = -1;
    }

    sockets->opened = 0;
    sockets->failed = 0;
}

void listen_sockets_close(LISTEN_SOCKETS *sockets) {
    size_t i;
    for(i = 0; i < sockets->opened ;i++) {
        close(sockets->fds[i]);
        sockets->fds[i] = -1;

        freez(sockets->fds_names[i]);
        sockets->fds_names[i] = NULL;

        sockets->fds_types[i] = -1;
    }

    sockets->opened = 0;
    sockets->failed = 0;
}

static inline int bind_to_this(LISTEN_SOCKETS *sockets, const char *definition, uint16_t default_port, int listen_backlog) {
    int added = 0;
    HTTP_ACL acl_flags = HTTP_ACL_NONE;

    struct addrinfo hints;
    struct addrinfo *result = NULL, *rp = NULL;

    char buffer[strlen(definition) + 1];
    strcpy(buffer, definition);

    char buffer2[10 + 1];
    snprintfz(buffer2, 10, "%d", default_port);

    char *ip = buffer, *port = buffer2, *iface = "", *portconfig;

    int protocol = IPPROTO_TCP, socktype = SOCK_STREAM;
    const char *protocol_str = "tcp";

    if(strncmp(ip, "tcp:", 4) == 0) {
        ip += 4;
        protocol = IPPROTO_TCP;
        socktype = SOCK_STREAM;
        protocol_str = "tcp";
        acl_flags |= HTTP_ACL_API;
    }
    else if(strncmp(ip, "udp:", 4) == 0) {
        ip += 4;
        protocol = IPPROTO_UDP;
        socktype = SOCK_DGRAM;
        protocol_str = "udp";
        acl_flags |= HTTP_ACL_API_UDP;
    }
    else if(strncmp(ip, "unix:", 5) == 0) {
        char *path = ip + 5;
        socktype = SOCK_STREAM;
        protocol_str = "unix";
        int fd = create_listen_socket_unix(path, listen_backlog);
        if (fd == -1) {
            nd_log(NDLS_DAEMON, NDLP_ERR,
                   "LISTENER: Cannot create unix socket '%s'",
                   path);

            sockets->failed++;
        } else {
            acl_flags = HTTP_ACL_API_UNIX | HTTP_ACL_DASHBOARD | HTTP_ACL_REGISTRY | HTTP_ACL_BADGES |
                        HTTP_ACL_MANAGEMENT | HTTP_ACL_NETDATACONF | HTTP_ACL_STREAMING | HTTP_ACL_SSL_DEFAULT;
            listen_sockets_add(sockets, fd, AF_UNIX, socktype, protocol_str, path, 0, acl_flags);
            added++;
        }
        return added;
    }

    char *e = ip;
    if(*e == '[') {
        e = ++ip;
        while(*e && *e != ']') e++;
        if(*e == ']') {
            *e = '\0';
            e++;
        }
    }
    else {
        while(*e && *e != ':' && *e != '%' && *e != '=') e++;
    }

    if(*e == '%') {
        *e = '\0';
        e++;
        iface = e;
        while(*e && *e != ':' && *e != '=') e++;
    }

    if(*e == ':') {
        port = e + 1;
        *e = '\0';
        e++;
        while(*e && *e != '=') e++;
    }

    if(*e == '=') {
        *e='\0';
        e++;
        portconfig = e;
        while (*e != '\0') {
            if (*e == '|') {
                *e = '\0';
                acl_flags |= read_acl(portconfig);
                e++;
                portconfig = e;
                continue;
            }
            e++;
        }
        acl_flags |= read_acl(portconfig);
    } else {
        acl_flags |= HTTP_ACL_DASHBOARD | HTTP_ACL_REGISTRY | HTTP_ACL_BADGES | HTTP_ACL_MANAGEMENT | HTTP_ACL_NETDATACONF | HTTP_ACL_STREAMING | HTTP_ACL_SSL_DEFAULT;
    }

    //Case the user does not set the option SSL in the "bind to", but he has
    //the certificates, I must redirect, so I am assuming here the default option
    if(!(acl_flags & HTTP_ACL_SSL_OPTIONAL) && !(acl_flags & HTTP_ACL_SSL_FORCE)) {
        acl_flags |= HTTP_ACL_SSL_DEFAULT;
    }

    uint32_t scope_id = 0;
    if(*iface) {
        scope_id = if_nametoindex(iface);
        if(!scope_id)
            nd_log(NDLS_DAEMON, NDLP_ERR,
                   "LISTENER: Cannot find a network interface named '%s'. "
                   "Continuing with limiting the network interface",
                   iface);
    }

    if(!*ip || *ip == '*' || !strcmp(ip, "any") || !strcmp(ip, "all"))
        ip = NULL;

    if(!*port)
        port = buffer2;

    memset(&hints, 0, sizeof(hints));
    hints.ai_family = AF_UNSPEC;  /* Allow IPv4 or IPv6 */
    hints.ai_socktype = socktype;
    hints.ai_flags = AI_PASSIVE;  /* For wildcard IP address */
    hints.ai_protocol = protocol;
    hints.ai_canonname = NULL;
    hints.ai_addr = NULL;
    hints.ai_next = NULL;

    int r = getaddrinfo(ip, port, &hints, &result);
    if (r != 0) {
        nd_log(NDLS_DAEMON, NDLP_ERR,
               "LISTENER: getaddrinfo('%s', '%s'): %s\n",
               ip, port, gai_strerror(r));

        return -1;
    }

    for (rp = result; rp != NULL; rp = rp->ai_next) {
        int fd = -1;
        int family;

        char rip[INET_ADDRSTRLEN + INET6_ADDRSTRLEN] = "INVALID";
        uint16_t rport = default_port;

        family = rp->ai_addr->sa_family;
        switch (family) {
            case AF_INET: {
                struct sockaddr_in *sin = (struct sockaddr_in *) rp->ai_addr;
                inet_ntop(AF_INET, &sin->sin_addr, rip, INET_ADDRSTRLEN);
                rport = ntohs(sin->sin_port);
                fd = create_listen_socket4(socktype, rip, rport, listen_backlog);
                break;
            }

            case AF_INET6: {
                struct sockaddr_in6 *sin6 = (struct sockaddr_in6 *) rp->ai_addr;
                inet_ntop(AF_INET6, &sin6->sin6_addr, rip, INET6_ADDRSTRLEN);
                rport = ntohs(sin6->sin6_port);
                fd = create_listen_socket6(socktype, scope_id, rip, rport, listen_backlog);
                break;
            }

            default:
                nd_log(NDLS_DAEMON, NDLP_DEBUG,
                       "LISTENER: Unknown socket family %d",
                       family);

                break;
        }

        if (fd == -1) {
            nd_log(NDLS_DAEMON, NDLP_ERR,
                   "LISTENER: Cannot bind to ip '%s', port %d",
                   rip, rport);

            sockets->failed++;
        }
        else {
            listen_sockets_add(sockets, fd, family, socktype, protocol_str, rip, rport, acl_flags);
            added++;
        }
    }

    freeaddrinfo(result);

    return added;
}

int listen_sockets_setup(LISTEN_SOCKETS *sockets) {
    listen_sockets_init(sockets);

    sockets->backlog = (int) inicfg_get_number(sockets->config, sockets->config_section, "listen backlog", sockets->backlog);

    long long int old_port = sockets->default_port;
    long long int new_port = inicfg_get_number(sockets->config, sockets->config_section, "default port", sockets->default_port);
    if(new_port < 1 || new_port > 65535) {
        nd_log(NDLS_DAEMON, NDLP_ERR,
               "LISTENER: Invalid listen port %lld given. Defaulting to %lld.",
               new_port, old_port);

        sockets->default_port = (uint16_t) inicfg_set_number(sockets->config, sockets->config_section, "default port", old_port);
    }
    else sockets->default_port = (uint16_t)new_port;

    const char *s = inicfg_get(sockets->config, sockets->config_section, "bind to", sockets->default_bind_to);
    while(*s) {
        const char *e = s;

        // skip separators, moving both s(tart) and e(nd)
        while(isspace((uint8_t)*e) || *e == ',') s = ++e;

        // move e(nd) to the first separator
        while(*e && !isspace((uint8_t)*e) && *e != ',') e++;

        // is there anything?
        if(!*s || s == e) break;

        char buf[e - s + 1];
        strncpyz(buf, s, e - s);
        bind_to_this(sockets, buf, sockets->default_port, sockets->backlog);

        s = e;
    }

    if(sockets->failed) {
        size_t i;
        for(i = 0; i < sockets->opened ;i++)
            nd_log(NDLS_DAEMON, NDLP_DEBUG,
                   "LISTENER: Listen socket %s opened successfully.",
                   sockets->fds_names[i]);
    }

    return (int)sockets->opened;
}
