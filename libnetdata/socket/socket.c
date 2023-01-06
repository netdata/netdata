// SPDX-License-Identifier: GPL-3.0-or-later

#include "../libnetdata.h"

// --------------------------------------------------------------------------------------------------------------------
// various library calls

#ifdef __gnu_linux__
#define LARGE_SOCK_SIZE 33554431 // don't ask why - I found it at brubeck source - I guess it is just a large number
#else
#define LARGE_SOCK_SIZE 4096
#endif

int sock_setnonblock(int fd) {
    int flags;

    flags = fcntl(fd, F_GETFL);
    flags |= O_NONBLOCK;

    int ret = fcntl(fd, F_SETFL, flags);
    if(ret < 0)
        error("Failed to set O_NONBLOCK on socket %d", fd);

    return ret;
}

int sock_delnonblock(int fd) {
    int flags;

    flags = fcntl(fd, F_GETFL);
    flags &= ~O_NONBLOCK;

    int ret = fcntl(fd, F_SETFL, flags);
    if(ret < 0)
        error("Failed to remove O_NONBLOCK on socket %d", fd);

    return ret;
}

int sock_setreuse(int fd, int reuse) {
    int ret = setsockopt(fd, SOL_SOCKET, SO_REUSEADDR, &reuse, sizeof(reuse));

    if(ret == -1)
        error("Failed to set SO_REUSEADDR on socket %d", fd);

    return ret;
}

int sock_setreuse_port(int fd, int reuse) {
    int ret;

#ifdef SO_REUSEPORT
    ret = setsockopt(fd, SOL_SOCKET, SO_REUSEPORT, &reuse, sizeof(reuse));
    if(ret == -1 && errno != ENOPROTOOPT)
        error("failed to set SO_REUSEPORT on socket %d", fd);
#else
    ret = -1;
#endif

    return ret;
}

int sock_enlarge_in(int fd) {
    int ret, bs = LARGE_SOCK_SIZE;

    ret = setsockopt(fd, SOL_SOCKET, SO_RCVBUF, &bs, sizeof(bs));

    if(ret == -1)
        error("Failed to set SO_RCVBUF on socket %d", fd);

    return ret;
}

int sock_enlarge_out(int fd) {
    int ret, bs = LARGE_SOCK_SIZE;
    ret = setsockopt(fd, SOL_SOCKET, SO_SNDBUF, &bs, sizeof(bs));

    if(ret == -1)
        error("Failed to set SO_SNDBUF on socket %d", fd);

    return ret;
}


// --------------------------------------------------------------------------------------------------------------------

char *strdup_client_description(int family, const char *protocol, const char *ip, uint16_t port) {
    char buffer[100 + 1];

    switch(family) {
        case AF_INET:
            snprintfz(buffer, 100, "%s:%s:%d", protocol, ip, port);
            break;

        case AF_INET6:
        default:
            snprintfz(buffer, 100, "%s:[%s]:%d", protocol, ip, port);
            break;

        case AF_UNIX:
            snprintfz(buffer, 100, "%s:%s", protocol, ip);
            break;
    }

    return strdupz(buffer);
}

// --------------------------------------------------------------------------------------------------------------------
// listening sockets

int create_listen_socket_unix(const char *path, int listen_backlog) {
    int sock;

    debug(D_LISTENER, "LISTENER: UNIX creating new listening socket on path '%s'", path);

    sock = socket(AF_UNIX, SOCK_STREAM, 0);
    if(sock < 0) {
        error("LISTENER: UNIX socket() on path '%s' failed.", path);
        return -1;
    }

    sock_setnonblock(sock);
    sock_enlarge_in(sock);

    struct sockaddr_un name;
    memset(&name, 0, sizeof(struct sockaddr_un));
    name.sun_family = AF_UNIX;
    strncpy(name.sun_path, path, sizeof(name.sun_path)-1);

    errno = 0;
    if (unlink(path) == -1 && errno != ENOENT)
        error("LISTENER: failed to remove existing (probably obsolete or left-over) file on UNIX socket path '%s'.", path);

    if(bind (sock, (struct sockaddr *) &name, sizeof (name)) < 0) {
        close(sock);
        error("LISTENER: UNIX bind() on path '%s' failed.", path);
        return -1;
    }

    // we have to chmod this to 0777 so that the client will be able
    // to read from and write to this socket.
    if(chmod(path, 0777) == -1)
        error("LISTENER: failed to chmod() socket file '%s'.", path);

    if(listen(sock, listen_backlog) < 0) {
        close(sock);
        error("LISTENER: UNIX listen() on path '%s' failed.", path);
        return -1;
    }

    debug(D_LISTENER, "LISTENER: Listening on UNIX path '%s'", path);
    return sock;
}

int create_listen_socket4(int socktype, const char *ip, uint16_t port, int listen_backlog) {
    int sock;

    debug(D_LISTENER, "LISTENER: IPv4 creating new listening socket on ip '%s' port %d, socktype %d", ip, port, socktype);

    sock = socket(AF_INET, socktype, 0);
    if(sock < 0) {
        error("LISTENER: IPv4 socket() on ip '%s' port %d, socktype %d failed.", ip, port, socktype);
        return -1;
    }

    sock_setreuse(sock, 1);
    sock_setreuse_port(sock, 1);
    sock_setnonblock(sock);
    sock_enlarge_in(sock);

    struct sockaddr_in name;
    memset(&name, 0, sizeof(struct sockaddr_in));
    name.sin_family = AF_INET;
    name.sin_port = htons (port);

    int ret = inet_pton(AF_INET, ip, (void *)&name.sin_addr.s_addr);
    if(ret != 1) {
        error("LISTENER: Failed to convert IP '%s' to a valid IPv4 address.", ip);
        close(sock);
        return -1;
    }

    if(bind (sock, (struct sockaddr *) &name, sizeof (name)) < 0) {
        close(sock);
        error("LISTENER: IPv4 bind() on ip '%s' port %d, socktype %d failed.", ip, port, socktype);
        return -1;
    }

    if(socktype == SOCK_STREAM && listen(sock, listen_backlog) < 0) {
        close(sock);
        error("LISTENER: IPv4 listen() on ip '%s' port %d, socktype %d failed.", ip, port, socktype);
        return -1;
    }

    debug(D_LISTENER, "LISTENER: Listening on IPv4 ip '%s' port %d, socktype %d", ip, port, socktype);
    return sock;
}

int create_listen_socket6(int socktype, uint32_t scope_id, const char *ip, int port, int listen_backlog) {
    int sock;
    int ipv6only = 1;

    debug(D_LISTENER, "LISTENER: IPv6 creating new listening socket on ip '%s' port %d, socktype %d", ip, port, socktype);

    sock = socket(AF_INET6, socktype, 0);
    if (sock < 0) {
        error("LISTENER: IPv6 socket() on ip '%s' port %d, socktype %d, failed.", ip, port, socktype);
        return -1;
    }

    sock_setreuse(sock, 1);
    sock_setreuse_port(sock, 1);
    sock_setnonblock(sock);
    sock_enlarge_in(sock);

    /* IPv6 only */
    if(setsockopt(sock, IPPROTO_IPV6, IPV6_V6ONLY, (void*)&ipv6only, sizeof(ipv6only)) != 0)
        error("LISTENER: Cannot set IPV6_V6ONLY on ip '%s' port %d, socktype %d.", ip, port, socktype);

    struct sockaddr_in6 name;
    memset(&name, 0, sizeof(struct sockaddr_in6));
    name.sin6_family = AF_INET6;
    name.sin6_port = htons ((uint16_t) port);
    name.sin6_scope_id = scope_id;

    int ret = inet_pton(AF_INET6, ip, (void *)&name.sin6_addr.s6_addr);
    if(ret != 1) {
        error("LISTENER: Failed to convert IP '%s' to a valid IPv6 address.", ip);
        close(sock);
        return -1;
    }

    name.sin6_scope_id = scope_id;

    if (bind (sock, (struct sockaddr *) &name, sizeof (name)) < 0) {
        close(sock);
        error("LISTENER: IPv6 bind() on ip '%s' port %d, socktype %d failed.", ip, port, socktype);
        return -1;
    }

    if (socktype == SOCK_STREAM && listen(sock, listen_backlog) < 0) {
        close(sock);
        error("LISTENER: IPv6 listen() on ip '%s' port %d, socktype %d failed.", ip, port, socktype);
        return -1;
    }

    debug(D_LISTENER, "LISTENER: Listening on IPv6 ip '%s' port %d, socktype %d", ip, port, socktype);
    return sock;
}

static inline int listen_sockets_add(LISTEN_SOCKETS *sockets, int fd, int family, int socktype, const char *protocol, const char *ip, uint16_t port, int acl_flags) {
    if(sockets->opened >= MAX_LISTEN_FDS) {
        error("LISTENER: Too many listening sockets. Failed to add listening %s socket at ip '%s' port %d, protocol %s, socktype %d", protocol, ip, port, protocol, socktype);
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

int listen_sockets_check_is_member(LISTEN_SOCKETS *sockets, int fd) {
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

/*
 *  SSL ACL
 *
 *  Search the SSL acl and apply it case it is set.
 *
 *  @param acl is the acl given by the user.
 */
WEB_CLIENT_ACL socket_ssl_acl(char *acl) {
    char *ssl = strchr(acl,'^');
    if(ssl) {
        //Due the format of the SSL command it is always the last command,
        //we finish it here to avoid problems with the ACLs
        *ssl = '\0';
#ifdef ENABLE_HTTPS
        ssl++;
        if (!strncmp("SSL=",ssl,4)) {
            ssl += 4;
            if (!strcmp(ssl,"optional")) {
                return WEB_CLIENT_ACL_SSL_OPTIONAL;
            }
            else if (!strcmp(ssl,"force")) {
                return WEB_CLIENT_ACL_SSL_FORCE;
            }
        }
#endif
    }

    return WEB_CLIENT_ACL_NONE;
}

WEB_CLIENT_ACL read_acl(char *st) {
    WEB_CLIENT_ACL ret = socket_ssl_acl(st);

    if (!strcmp(st,"dashboard")) ret |= WEB_CLIENT_ACL_DASHBOARD;
    if (!strcmp(st,"registry")) ret |= WEB_CLIENT_ACL_REGISTRY;
    if (!strcmp(st,"badges")) ret |= WEB_CLIENT_ACL_BADGE;
    if (!strcmp(st,"management")) ret |= WEB_CLIENT_ACL_MGMT;
    if (!strcmp(st,"streaming")) ret |= WEB_CLIENT_ACL_STREAMING;
    if (!strcmp(st,"netdata.conf")) ret |= WEB_CLIENT_ACL_NETDATACONF;

    return ret;
}

static inline int bind_to_this(LISTEN_SOCKETS *sockets, const char *definition, uint16_t default_port, int listen_backlog) {
    int added = 0;
    WEB_CLIENT_ACL acl_flags = WEB_CLIENT_ACL_NONE;

    struct addrinfo hints;
    struct addrinfo *result = NULL, *rp = NULL;

    char buffer[strlen(definition) + 1];
    strcpy(buffer, definition);

    char buffer2[10 + 1];
    snprintfz(buffer2, 10, "%d", default_port);

    char *ip = buffer, *port = buffer2, *interface = "", *portconfig;;

    int protocol = IPPROTO_TCP, socktype = SOCK_STREAM;
    const char *protocol_str = "tcp";

    if(strncmp(ip, "tcp:", 4) == 0) {
        ip += 4;
        protocol = IPPROTO_TCP;
        socktype = SOCK_STREAM;
        protocol_str = "tcp";
    }
    else if(strncmp(ip, "udp:", 4) == 0) {
        ip += 4;
        protocol = IPPROTO_UDP;
        socktype = SOCK_DGRAM;
        protocol_str = "udp";
    }
    else if(strncmp(ip, "unix:", 5) == 0) {
        char *path = ip + 5;
        socktype = SOCK_STREAM;
        protocol_str = "unix";
        int fd = create_listen_socket_unix(path, listen_backlog);
        if (fd == -1) {
            error("LISTENER: Cannot create unix socket '%s'", path);
            sockets->failed++;
        } else {
            acl_flags = WEB_CLIENT_ACL_DASHBOARD | WEB_CLIENT_ACL_REGISTRY | WEB_CLIENT_ACL_BADGE | WEB_CLIENT_ACL_MGMT | WEB_CLIENT_ACL_NETDATACONF | WEB_CLIENT_ACL_STREAMING | WEB_CLIENT_ACL_SSL_DEFAULT;
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
        interface = e;
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
        acl_flags = WEB_CLIENT_ACL_DASHBOARD | WEB_CLIENT_ACL_REGISTRY | WEB_CLIENT_ACL_BADGE | WEB_CLIENT_ACL_MGMT | WEB_CLIENT_ACL_NETDATACONF | WEB_CLIENT_ACL_STREAMING | WEB_CLIENT_ACL_SSL_DEFAULT;
    }

    //Case the user does not set the option SSL in the "bind to", but he has
    //the certificates, I must redirect, so I am assuming here the default option
    if(!(acl_flags & WEB_CLIENT_ACL_SSL_OPTIONAL) && !(acl_flags & WEB_CLIENT_ACL_SSL_FORCE)) {
        acl_flags |= WEB_CLIENT_ACL_SSL_DEFAULT;
    }

    uint32_t scope_id = 0;
    if(*interface) {
        scope_id = if_nametoindex(interface);
        if(!scope_id)
            error("LISTENER: Cannot find a network interface named '%s'. Continuing with limiting the network interface", interface);
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
        error("LISTENER: getaddrinfo('%s', '%s'): %s\n", ip, port, gai_strerror(r));
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
                // info("Attempting to listen on IPv4 '%s' ('%s'), port %d ('%s'), socktype %d", rip, ip, rport, port, socktype);
                fd = create_listen_socket4(socktype, rip, rport, listen_backlog);
                break;
            }

            case AF_INET6: {
                struct sockaddr_in6 *sin6 = (struct sockaddr_in6 *) rp->ai_addr;
                inet_ntop(AF_INET6, &sin6->sin6_addr, rip, INET6_ADDRSTRLEN);
                rport = ntohs(sin6->sin6_port);
                // info("Attempting to listen on IPv6 '%s' ('%s'), port %d ('%s'), socktype %d", rip, ip, rport, port, socktype);
                fd = create_listen_socket6(socktype, scope_id, rip, rport, listen_backlog);
                break;
            }

            default:
                debug(D_LISTENER, "LISTENER: Unknown socket family %d", family);
                break;
        }

        if (fd == -1) {
            error("LISTENER: Cannot bind to ip '%s', port %d", rip, rport);
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

    sockets->backlog = (int) appconfig_get_number(sockets->config, sockets->config_section, "listen backlog", sockets->backlog);

    long long int old_port = sockets->default_port;
    long long int new_port = appconfig_get_number(sockets->config, sockets->config_section, "default port", sockets->default_port);
    if(new_port < 1 || new_port > 65535) {
        error("LISTENER: Invalid listen port %lld given. Defaulting to %lld.", new_port, old_port);
        sockets->default_port = (uint16_t) appconfig_set_number(sockets->config, sockets->config_section, "default port", old_port);
    }
    else sockets->default_port = (uint16_t)new_port;

    debug(D_OPTIONS, "LISTENER: Default listen port set to %d.", sockets->default_port);

    char *s = appconfig_get(sockets->config, sockets->config_section, "bind to", sockets->default_bind_to);
    while(*s) {
        char *e = s;

        // skip separators, moving both s(tart) and e(nd)
        while(isspace(*e) || *e == ',') s = ++e;

        // move e(nd) to the first separator
        while(*e && !isspace(*e) && *e != ',') e++;

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
            info("LISTENER: Listen socket %s opened successfully.", sockets->fds_names[i]);
    }

    return (int)sockets->opened;
}


// --------------------------------------------------------------------------------------------------------------------
// connect to another host/port

// connect_to_this_unix()
// path        the path of the unix socket
// timeout     the timeout for establishing a connection

static inline int connect_to_unix(const char *path, struct timeval *timeout) {
    int fd = socket(AF_UNIX, SOCK_STREAM, 0);
    if(fd == -1) {
        error("Failed to create UNIX socket() for '%s'", path);
        return -1;
    }

    if(timeout) {
        if(setsockopt(fd, SOL_SOCKET, SO_SNDTIMEO, (char *) timeout, sizeof(struct timeval)) < 0)
            error("Failed to set timeout on UNIX socket '%s'", path);
    }

    struct sockaddr_un addr;
    memset(&addr, 0, sizeof(addr));
    addr.sun_family = AF_UNIX;
    strncpy(addr.sun_path, path, sizeof(addr.sun_path)-1);

    if (connect(fd, (struct sockaddr*)&addr, sizeof(addr)) == -1) {
        error("Cannot connect to UNIX socket on path '%s'.", path);
        close(fd);
        return -1;
    }

    debug(D_CONNECT_TO, "Connected to UNIX socket on path '%s'.", path);

    return fd;
}

// connect_to_this_ip46()
// protocol    IPPROTO_TCP, IPPROTO_UDP
// socktype    SOCK_STREAM, SOCK_DGRAM
// host        the destination hostname or IP address (IPv4 or IPv6) to connect to
//             if it resolves to many IPs, all are tried (IPv4 and IPv6)
// scope_id    the if_index id of the interface to use for connecting (0 = any)
//             (used only under IPv6)
// service     the service name or port to connect to
// timeout     the timeout for establishing a connection

int connect_to_this_ip46(int protocol, int socktype, const char *host, uint32_t scope_id, const char *service, struct timeval *timeout) {
    struct addrinfo hints;
    struct addrinfo *ai_head = NULL, *ai = NULL;

    memset(&hints, 0, sizeof(hints));
    hints.ai_family   = PF_UNSPEC;   /* Allow IPv4 or IPv6 */
    hints.ai_socktype = socktype;
    hints.ai_protocol = protocol;

    int ai_err = getaddrinfo(host, service, &hints, &ai_head);
    if (ai_err != 0) {
        error("Cannot resolve host '%s', port '%s': %s", host, service, gai_strerror(ai_err));
        return -1;
    }

    int fd = -1;
    for (ai = ai_head; ai != NULL && fd == -1; ai = ai->ai_next) {

        if (ai->ai_family == PF_INET6) {
            struct sockaddr_in6 *pSadrIn6 = (struct sockaddr_in6 *) ai->ai_addr;
            if(pSadrIn6->sin6_scope_id == 0) {
                pSadrIn6->sin6_scope_id = scope_id;
            }
        }

        char hostBfr[NI_MAXHOST + 1];
        char servBfr[NI_MAXSERV + 1];

        getnameinfo(ai->ai_addr,
                    ai->ai_addrlen,
                    hostBfr,
                    sizeof(hostBfr),
                    servBfr,
                    sizeof(servBfr),
                    NI_NUMERICHOST | NI_NUMERICSERV);

        debug(D_CONNECT_TO, "Address info: host = '%s', service = '%s', ai_flags = 0x%02X, ai_family = %d (PF_INET = %d, PF_INET6 = %d), ai_socktype = %d (SOCK_STREAM = %d, SOCK_DGRAM = %d), ai_protocol = %d (IPPROTO_TCP = %d, IPPROTO_UDP = %d), ai_addrlen = %lu (sockaddr_in = %lu, sockaddr_in6 = %lu)",
              hostBfr,
              servBfr,
              (unsigned int)ai->ai_flags,
              ai->ai_family,
              PF_INET,
              PF_INET6,
              ai->ai_socktype,
              SOCK_STREAM,
              SOCK_DGRAM,
              ai->ai_protocol,
              IPPROTO_TCP,
              IPPROTO_UDP,
              (unsigned long)ai->ai_addrlen,
              (unsigned long)sizeof(struct sockaddr_in),
              (unsigned long)sizeof(struct sockaddr_in6));

        switch (ai->ai_addr->sa_family) {
            case PF_INET: {
                struct sockaddr_in *pSadrIn = (struct sockaddr_in *)ai->ai_addr;
                (void)pSadrIn;

                debug(D_CONNECT_TO, "ai_addr = sin_family: %d (AF_INET = %d, AF_INET6 = %d), sin_addr: '%s', sin_port: '%s'",
                      pSadrIn->sin_family,
                      AF_INET,
                      AF_INET6,
                      hostBfr,
                      servBfr);
                break;
            }

            case PF_INET6: {
                struct sockaddr_in6 *pSadrIn6 = (struct sockaddr_in6 *) ai->ai_addr;
                (void)pSadrIn6;

                debug(D_CONNECT_TO,"ai_addr = sin6_family: %d (AF_INET = %d, AF_INET6 = %d), sin6_addr: '%s', sin6_port: '%s', sin6_flowinfo: %u, sin6_scope_id: %u",
                      pSadrIn6->sin6_family,
                      AF_INET,
                      AF_INET6,
                      hostBfr,
                      servBfr,
                      pSadrIn6->sin6_flowinfo,
                      pSadrIn6->sin6_scope_id);
                break;
            }

            default: {
                debug(D_CONNECT_TO, "Unknown protocol family %d.", ai->ai_family);
                continue;
            }
        }

        fd = socket(ai->ai_family, ai->ai_socktype, ai->ai_protocol);
        if(fd != -1) {
            if(timeout) {
                if(setsockopt(fd, SOL_SOCKET, SO_SNDTIMEO, (char *) timeout, sizeof(struct timeval)) < 0)
                    error("Failed to set timeout on the socket to ip '%s' port '%s'", hostBfr, servBfr);
            }

            errno = 0;
            if(connect(fd, ai->ai_addr, ai->ai_addrlen) < 0) {
                if(errno == EALREADY || errno == EINPROGRESS) {
                    info("Waiting for connection to ip %s port %s to be established", hostBfr, servBfr);

                    fd_set fds;
                    FD_ZERO(&fds);
                    FD_SET(0, &fds);
                    int rc = select (1, NULL, &fds, NULL, timeout);

                    if(rc > 0 && FD_ISSET(fd, &fds)) {
                        info("connect() to ip %s port %s completed successfully", hostBfr, servBfr);
                    }
                    else if(rc == -1) {
                        error("Failed to connect to '%s', port '%s'. select() returned %d", hostBfr, servBfr, rc);
                        close(fd);
                        fd = -1;
                    }
                    else {
                        error("Timed out while connecting to '%s', port '%s'. select() returned %d", hostBfr, servBfr, rc);
                        close(fd);
                        fd = -1;
                    }
                }
                else {
                    error("Failed to connect to '%s', port '%s'", hostBfr, servBfr);
                    close(fd);
                    fd = -1;
                }
            }

            if(fd != -1)
                debug(D_CONNECT_TO, "Connected to '%s' on port '%s'.", hostBfr, servBfr);
        }
    }

    freeaddrinfo(ai_head);

    return fd;
}

// connect_to_this()
//
// definition format:
//
//    [PROTOCOL:]IP[%INTERFACE][:PORT]
//
// PROTOCOL  = tcp or udp
// IP        = IPv4 or IPv6 IP or hostname, optionally enclosed in [] (required for IPv6)
// INTERFACE = for IPv6 only, the network interface to use
// PORT      = port number or service name

int connect_to_this(const char *definition, int default_port, struct timeval *timeout) {
    char buffer[strlen(definition) + 1];
    strcpy(buffer, definition);

    char default_service[10 + 1];
    snprintfz(default_service, 10, "%d", default_port);

    char *host = buffer, *service = default_service, *interface = "";
    int protocol = IPPROTO_TCP, socktype = SOCK_STREAM;
    uint32_t scope_id = 0;

    if(strncmp(host, "tcp:", 4) == 0) {
        host += 4;
        protocol = IPPROTO_TCP;
        socktype = SOCK_STREAM;
    }
    else if(strncmp(host, "udp:", 4) == 0) {
        host += 4;
        protocol = IPPROTO_UDP;
        socktype = SOCK_DGRAM;
    }
    else if(strncmp(host, "unix:", 5) == 0) {
        char *path = host + 5;
        return connect_to_unix(path, timeout);
    }
    else if(*host == '/') {
        char *path = host;
        return connect_to_unix(path, timeout);
    }

    char *e = host;
    if(*e == '[') {
        e = ++host;
        while(*e && *e != ']') e++;
        if(*e == ']') {
            *e = '\0';
            e++;
        }
    }
    else {
        while(*e && *e != ':' && *e != '%') e++;
    }

    if(*e == '%') {
        *e = '\0';
        e++;
        interface = e;
        while(*e && *e != ':') e++;
    }

    if(*e == ':') {
        *e = '\0';
        e++;
        service = e;
    }

    debug(D_CONNECT_TO, "Attempting connection to host = '%s', service = '%s', interface = '%s', protocol = %d (tcp = %d, udp = %d)", host, service, interface, protocol, IPPROTO_TCP, IPPROTO_UDP);

    if(!*host) {
        error("Definition '%s' does not specify a host.", definition);
        return -1;
    }

    if(*interface) {
        scope_id = if_nametoindex(interface);
        if(!scope_id)
            error("Cannot find a network interface named '%s'. Continuing with limiting the network interface", interface);
    }

    if(!*service)
        service = default_service;


    return connect_to_this_ip46(protocol, socktype, host, scope_id, service, timeout);
}

void foreach_entry_in_connection_string(const char *destination, bool (*callback)(char *entry, void *data), void *data) {
    const char *s = destination;
    while(*s) {
        const char *e = s;

        // skip separators, moving both s(tart) and e(nd)
        while(isspace(*e) || *e == ',') s = ++e;

        // move e(nd) to the first separator
        while(*e && !isspace(*e) && *e != ',') e++;

        // is there anything?
        if(!*s || s == e) break;

        char buf[e - s + 1];
        strncpyz(buf, s, e - s);

        if(callback(buf, data)) break;

        s = e;
    }
}

struct connect_to_one_of_data {
    int default_port;
    struct timeval *timeout;
    size_t *reconnects_counter;
    char *connected_to;
    size_t connected_to_size;
    int sock;
};

static bool connect_to_one_of_callback(char *entry, void *data) {
    struct connect_to_one_of_data *t = data;

    if(t->reconnects_counter)
        t->reconnects_counter++;

    t->sock = connect_to_this(entry, t->default_port, t->timeout);
    if(t->sock != -1) {
        if(t->connected_to && t->connected_to_size) {
            strncpyz(t->connected_to, entry, t->connected_to_size);
            t->connected_to[t->connected_to_size - 1] = '\0';
        }

        return true;
    }

    return false;
}

int connect_to_one_of(const char *destination, int default_port, struct timeval *timeout, size_t *reconnects_counter, char *connected_to, size_t connected_to_size) {
    struct connect_to_one_of_data t = {
        .default_port = default_port,
        .timeout = timeout,
        .reconnects_counter = reconnects_counter,
        .connected_to = connected_to,
        .connected_to_size = connected_to_size,
        .sock = -1,
    };

    foreach_entry_in_connection_string(destination, connect_to_one_of_callback, &t);

    return t.sock;
}

static bool connect_to_one_of_urls_callback(char *entry, void *data) {
    char *s = strchr(entry, '/');
    if(s) *s = '\0';

    return connect_to_one_of_callback(entry, data);
}

int connect_to_one_of_urls(const char *destination, int default_port, struct timeval *timeout, size_t *reconnects_counter, char *connected_to, size_t connected_to_size) {
    struct connect_to_one_of_data t = {
        .default_port = default_port,
        .timeout = timeout,
        .reconnects_counter = reconnects_counter,
        .connected_to = connected_to,
        .connected_to_size = connected_to_size,
        .sock = -1,
    };

    foreach_entry_in_connection_string(destination, connect_to_one_of_urls_callback, &t);

    return t.sock;
}


#ifdef ENABLE_HTTPS
ssize_t netdata_ssl_read(SSL *ssl, void *buf, size_t num) {
    error_limit_static_thread_var(erl, 1, 0);

    int bytes, err, retries = 0;

    //do {
        bytes = SSL_read(ssl, buf, (int)num);
        err = SSL_get_error(ssl, bytes);
        retries++;
    //} while (bytes <= 0 && (err == SSL_ERROR_WANT_READ));

    if(unlikely(bytes <= 0))
        error("SSL_read() returned %d bytes, SSL error %d", bytes, err);

    if(retries > 1)
        error_limit(&erl, "SSL_read() retried %d times", retries);

    return bytes;
}

ssize_t netdata_ssl_write(SSL *ssl, const void *buf, size_t num) {
    error_limit_static_thread_var(erl, 1, 0);

    int bytes, err, retries = 0;
    size_t total = 0;

    //do {
        bytes = SSL_write(ssl, (uint8_t *)buf + total, (int)(num - total));
        err = SSL_get_error(ssl, bytes);
        retries++;

        if(bytes > 0)
            total += bytes;

    //} while ((bytes <= 0 && (err == SSL_ERROR_WANT_WRITE)) || (bytes > 0 && total < num));

    if(unlikely(bytes <= 0))
        error("SSL_write() returned %d bytes, SSL error %d", bytes, err);

    if(retries > 1)
        error_limit(&erl, "SSL_write() retried %d times", retries);

    return bytes;
}
#endif

// --------------------------------------------------------------------------------------------------------------------
// helpers to send/receive data in one call, in blocking mode, with a timeout

#ifdef ENABLE_HTTPS
ssize_t recv_timeout(struct netdata_ssl *ssl,int sockfd, void *buf, size_t len, int flags, int timeout) {
#else
ssize_t recv_timeout(int sockfd, void *buf, size_t len, int flags, int timeout) {
#endif

    for(;;) {
        struct pollfd fd = {
                .fd = sockfd,
                .events = POLLIN,
                .revents = 0
        };

        errno = 0;
        int retval = poll(&fd, 1, timeout * 1000);

        if(retval == -1) {
            // failed

            if(errno == EINTR || errno == EAGAIN)
                continue;

            return -1;
        }

        if(!retval) {
            // timeout
            return 0;
        }

        if(fd.events & POLLIN) break;
    }

#ifdef ENABLE_HTTPS
    if (ssl->conn && ssl->flags == NETDATA_SSL_HANDSHAKE_COMPLETE)
        return netdata_ssl_read(ssl->conn, buf, len);
#endif

    return recv(sockfd, buf, len, flags);
}

#ifdef ENABLE_HTTPS
ssize_t send_timeout(struct netdata_ssl *ssl,int sockfd, void *buf, size_t len, int flags, int timeout) {
#else
ssize_t send_timeout(int sockfd, void *buf, size_t len, int flags, int timeout) {
#endif

    for(;;) {
        struct pollfd fd = {
                .fd = sockfd,
                .events = POLLOUT,
                .revents = 0
        };

        errno = 0;
        int retval = poll(&fd, 1, timeout * 1000);

        if(retval == -1) {
            // failed

            if(errno == EINTR || errno == EAGAIN)
                continue;

            return -1;
        }

        if(!retval) {
            // timeout
            return 0;
        }

        if(fd.events & POLLOUT) break;
    }

#ifdef ENABLE_HTTPS
    if(ssl->conn) {
        if (ssl->flags == NETDATA_SSL_HANDSHAKE_COMPLETE) {
            return netdata_ssl_write(ssl->conn, buf, len);
        }
        else {
            error("cannot write to SSL connection - connection is not ready.");
            return -1;
        }
    }
#endif
    return send(sockfd, buf, len, flags);
}


// --------------------------------------------------------------------------------------------------------------------
// accept4() replacement for systems that do not have one

#ifndef HAVE_ACCEPT4
int accept4(int sock, struct sockaddr *addr, socklen_t *addrlen, int flags) {
    int fd = accept(sock, addr, addrlen);
    int newflags = 0;

    if (fd < 0) return fd;

    if (flags & SOCK_NONBLOCK) {
        newflags |= O_NONBLOCK;
        flags &= ~SOCK_NONBLOCK;
    }

#ifdef SOCK_CLOEXEC
#ifdef O_CLOEXEC
    if (flags & SOCK_CLOEXEC) {
        newflags |= O_CLOEXEC;
        flags &= ~SOCK_CLOEXEC;
    }
#endif
#endif

    if (flags) {
        close(fd);
        errno = EINVAL;
        return -1;
    }

    if (fcntl(fd, F_SETFL, newflags) < 0) {
        int saved_errno = errno;
        close(fd);
        errno = saved_errno;
        return -1;
    }

    return fd;
}
#endif

/*
 * ---------------------------------------------------------------------------------------------------------------------
 * connection_allowed() - if there is an access list then check the connection matches a pattern.
 *                        Numeric patterns are checked against the IP address first, only if they
 *                        do not match is the hostname resolved (reverse-DNS) and checked. If the
 *                        hostname matches then we perform forward DNS resolution to check the IP
 *                        is really associated with the DNS record. This call is repeatable: the
 *                        web server may check more refined matches against the connection. Will
 *                        update the client_host if uninitialized - ensure the hostsize is the number
 *                        of *writable* bytes (i.e. be aware of the strdup used to compact the pollinfo).
 */
extern int connection_allowed(int fd, char *client_ip, char *client_host, size_t hostsize, SIMPLE_PATTERN *access_list,
                              const char *patname, int allow_dns)
{
    debug(D_LISTENER,"checking %s... (allow_dns=%d)", patname, allow_dns);
    if (!access_list)
        return 1;
    if (simple_pattern_matches(access_list, client_ip))
        return 1;
    // If the hostname is unresolved (and needed) then attempt the DNS lookups.
    //if (client_host[0]==0 && simple_pattern_is_potential_name(access_list))
    if (client_host[0]==0 && allow_dns)
    {
        struct sockaddr_storage sadr;
        socklen_t addrlen = sizeof(sadr);
        int err = getpeername(fd, (struct sockaddr*)&sadr, &addrlen);
        if (err != 0 ||
            (err = getnameinfo((struct sockaddr *)&sadr, addrlen, client_host, (socklen_t)hostsize,
                              NULL, 0, NI_NAMEREQD)) != 0) {
            error("Incoming %s on '%s' does not match a numeric pattern, and host could not be resolved (err=%s)",
                  patname, client_ip, gai_strerror(err));
            if (hostsize >= 8)
                strcpy(client_host,"UNKNOWN");
            return 0;
        }
        struct addrinfo *addr_infos = NULL;
        if (getaddrinfo(client_host, NULL, NULL, &addr_infos) !=0 ) {
            error("LISTENER: cannot validate hostname '%s' from '%s' by resolving it",
                  client_host, client_ip);
            if (hostsize >= 8)
                strcpy(client_host,"UNKNOWN");
            return 0;
        }
        struct addrinfo *scan = addr_infos;
        int    validated = 0;
        while (scan) {
            char address[INET6_ADDRSTRLEN];
            address[0] = 0;
            switch (scan->ai_addr->sa_family) {
                case AF_INET:
                    inet_ntop(AF_INET, &((struct sockaddr_in*)(scan->ai_addr))->sin_addr, address, INET6_ADDRSTRLEN);
                    break;
                case AF_INET6:
                    inet_ntop(AF_INET6, &((struct sockaddr_in6*)(scan->ai_addr))->sin6_addr, address, INET6_ADDRSTRLEN);
                    break;
            }
            debug(D_LISTENER, "Incoming ip %s rev-resolved onto %s, validating against forward-resolution %s",
                  client_ip, client_host, address);
            if (!strcmp(client_ip, address)) {
                validated = 1;
                break;
            }
            scan = scan->ai_next;
        }
        if (!validated) {
            error("LISTENER: Cannot validate '%s' as ip of '%s', not listed in DNS", client_ip, client_host);
            if (hostsize >= 8)
                strcpy(client_host,"UNKNOWN");
        }
        if (addr_infos!=NULL)
            freeaddrinfo(addr_infos);
    }
    if (!simple_pattern_matches(access_list, client_host)) {
        debug(D_LISTENER, "Incoming connection on '%s' (%s) does not match allowed pattern for %s",
              client_ip, client_host, patname);
        return 0;
    }
    return 1;
}

// --------------------------------------------------------------------------------------------------------------------
// accept_socket() - accept a socket and store client IP and port
int accept_socket(int fd, int flags, char *client_ip, size_t ipsize, char *client_port, size_t portsize,
                  char *client_host, size_t hostsize, SIMPLE_PATTERN *access_list, int allow_dns) {
    struct sockaddr_storage sadr;
    socklen_t addrlen = sizeof(sadr);

    int nfd = accept4(fd, (struct sockaddr *)&sadr, &addrlen, flags);
    if (likely(nfd >= 0)) {
        if (getnameinfo((struct sockaddr *)&sadr, addrlen, client_ip, (socklen_t)ipsize,
                        client_port, (socklen_t)portsize, NI_NUMERICHOST | NI_NUMERICSERV) != 0) {
            error("LISTENER: cannot getnameinfo() on received client connection.");
            strncpyz(client_ip, "UNKNOWN", ipsize);
            strncpyz(client_port, "UNKNOWN", portsize);
        }
        if (!strcmp(client_ip, "127.0.0.1") || !strcmp(client_ip, "::1")) {
            strncpyz(client_ip, "localhost", ipsize);
        }

#ifdef __FreeBSD__
        if(((struct sockaddr *)&sadr)->sa_family == AF_LOCAL)
            strncpyz(client_ip, "localhost", ipsize);
#endif

        client_ip[ipsize - 1] = '\0';
        client_port[portsize - 1] = '\0';

        switch (((struct sockaddr *)&sadr)->sa_family) {
            case AF_UNIX:
                debug(D_LISTENER, "New UNIX domain web client from %s on socket %d.", client_ip, fd);
                // set the port - certain versions of libc return garbage on unix sockets
                strncpyz(client_port, "UNIX", portsize);
                break;

            case AF_INET:
                debug(D_LISTENER, "New IPv4 web client from %s port %s on socket %d.", client_ip, client_port, fd);
                break;

            case AF_INET6:
                if (strncmp(client_ip, "::ffff:", 7) == 0) {
                    memmove(client_ip, &client_ip[7], strlen(&client_ip[7]) + 1);
                    debug(D_LISTENER, "New IPv4 web client from %s port %s on socket %d.", client_ip, client_port, fd);
                }
                else
                    debug(D_LISTENER, "New IPv6 web client from %s port %s on socket %d.", client_ip, client_port, fd);
                break;

            default:
                debug(D_LISTENER, "New UNKNOWN web client from %s port %s on socket %d.", client_ip, client_port, fd);
                break;
        }
        if (!connection_allowed(nfd, client_ip, client_host, hostsize, access_list, "connection", allow_dns)) {
            errno = 0;
            error("Permission denied for client '%s', port '%s'", client_ip, client_port);
            close(nfd);
            nfd = -1;
            errno = EPERM;
        }
    }
#ifdef HAVE_ACCEPT4
    else if (errno == ENOSYS)
        error("netdata has been compiled with the assumption that the system has the accept4() call, but it is not here. Recompile netdata like this: ./configure --disable-accept4 ...");
#endif

    return nfd;
}


// --------------------------------------------------------------------------------------------------------------------
// poll() based listener
// this should be the fastest possible listener for up to 100 sockets
// above 100, an epoll() interface is needed on Linux

#define POLL_FDS_INCREASE_STEP 10

inline POLLINFO *poll_add_fd(POLLJOB *p
                             , int fd
                             , int socktype
                             , WEB_CLIENT_ACL port_acl
                             , uint32_t flags
                             , const char *client_ip
                             , const char *client_port
                             , const char *client_host
                             , void *(*add_callback)(POLLINFO * /*pi*/, short int * /*events*/, void * /*data*/)
                             , void  (*del_callback)(POLLINFO * /*pi*/)
                             , int   (*rcv_callback)(POLLINFO * /*pi*/, short int * /*events*/)
                             , int   (*snd_callback)(POLLINFO * /*pi*/, short int * /*events*/)
                             , void *data
) {
    debug(D_POLLFD, "POLLFD: ADD: request to add fd %d, slots = %zu, used = %zu, min = %zu, max = %zu, next free = %zd", fd, p->slots, p->used, p->min, p->max, p->first_free?(ssize_t)p->first_free->slot:(ssize_t)-1);

    if(unlikely(fd < 0)) return NULL;

    //if(p->limit && p->used >= p->limit) {
    //    info("Max sockets limit reached (%zu sockets), dropping connection", p->used);
    //    close(fd);
    //    return NULL;
    //}

    if(unlikely(!p->first_free)) {
        size_t new_slots = p->slots + POLL_FDS_INCREASE_STEP;
        debug(D_POLLFD, "POLLFD: ADD: increasing size (current = %zu, new = %zu, used = %zu, min = %zu, max = %zu)", p->slots, new_slots, p->used, p->min, p->max);

        p->fds = reallocz(p->fds, sizeof(struct pollfd) * new_slots);
        p->inf = reallocz(p->inf, sizeof(POLLINFO) * new_slots);

        // reset all the newly added slots
        ssize_t i;
        for(i = new_slots - 1; i >= (ssize_t)p->slots ; i--) {
            debug(D_POLLFD, "POLLFD: ADD: resetting new slot %zd", i);
            p->fds[i].fd = -1;
            p->fds[i].events = 0;
            p->fds[i].revents = 0;

            p->inf[i].p = p;
            p->inf[i].slot = (size_t)i;
            p->inf[i].flags = 0;
            p->inf[i].socktype = -1;
            p->inf[i].port_acl = -1;

            p->inf[i].client_ip = NULL;
            p->inf[i].client_port = NULL;
            p->inf[i].client_host = NULL;
            p->inf[i].del_callback = p->del_callback;
            p->inf[i].rcv_callback = p->rcv_callback;
            p->inf[i].snd_callback = p->snd_callback;
            p->inf[i].data = NULL;

            // link them so that the first free will be earlier in the array
            // (we loop decrementing i)
            p->inf[i].next = p->first_free;
            p->first_free = &p->inf[i];
        }

        p->slots = new_slots;
    }

    POLLINFO *pi = p->first_free;
    p->first_free = p->first_free->next;

    debug(D_POLLFD, "POLLFD: ADD: selected slot %zu, next free is %zd", pi->slot, p->first_free?(ssize_t)p->first_free->slot:(ssize_t)-1);

    struct pollfd *pf = &p->fds[pi->slot];
    pf->fd = fd;
    pf->events = POLLIN;
    pf->revents = 0;

    pi->fd = fd;
    pi->p = p;
    pi->socktype = socktype;
    pi->port_acl = port_acl;
    pi->flags = flags;
    pi->next = NULL;
    pi->client_ip   = strdupz(client_ip);
    pi->client_port = strdupz(client_port);
    pi->client_host = strdupz(client_host);

    pi->del_callback = del_callback;
    pi->rcv_callback = rcv_callback;
    pi->snd_callback = snd_callback;

    pi->connected_t = now_boottime_sec();
    pi->last_received_t = 0;
    pi->last_sent_t = 0;
    pi->last_sent_t = 0;
    pi->recv_count = 0;
    pi->send_count = 0;

    netdata_thread_disable_cancelability();
    p->used++;
    if(unlikely(pi->slot > p->max))
        p->max = pi->slot;

    if(pi->flags & POLLINFO_FLAG_CLIENT_SOCKET) {
        pi->data = add_callback(pi, &pf->events, data);
    }

    if(pi->flags & POLLINFO_FLAG_SERVER_SOCKET) {
        p->min = pi->slot;
    }
    netdata_thread_enable_cancelability();

    debug(D_POLLFD, "POLLFD: ADD: completed, slots = %zu, used = %zu, min = %zu, max = %zu, next free = %zd", p->slots, p->used, p->min, p->max, p->first_free?(ssize_t)p->first_free->slot:(ssize_t)-1);

    return pi;
}

inline void poll_close_fd(POLLINFO *pi) {
    POLLJOB *p = pi->p;

    struct pollfd *pf = &p->fds[pi->slot];
    debug(D_POLLFD, "POLLFD: DEL: request to clear slot %zu (fd %d), old next free was %zd", pi->slot, pf->fd, p->first_free?(ssize_t)p->first_free->slot:(ssize_t)-1);

    if(unlikely(pf->fd == -1)) return;

    netdata_thread_disable_cancelability();

    if(pi->flags & POLLINFO_FLAG_CLIENT_SOCKET) {
        pi->del_callback(pi);

        if(likely(!(pi->flags & POLLINFO_FLAG_DONT_CLOSE))) {
            if(close(pf->fd) == -1)
                error("Failed to close() poll_events() socket %d", pf->fd);
        }
    }

    pf->fd = -1;
    pf->events = 0;
    pf->revents = 0;

    pi->fd = -1;
    pi->socktype = -1;
    pi->flags = 0;
    pi->data = NULL;

    pi->del_callback = NULL;
    pi->rcv_callback = NULL;
    pi->snd_callback = NULL;

    freez(pi->client_ip);
    pi->client_ip = NULL;

    freez(pi->client_port);
    pi->client_port = NULL;

    freez(pi->client_host);
    pi->client_host = NULL;

    pi->next = p->first_free;
    p->first_free = pi;

    p->used--;
    if(unlikely(p->max == pi->slot)) {
        p->max = p->min;
        ssize_t i;
        for(i = (ssize_t)pi->slot; i > (ssize_t)p->min ;i--) {
            if (unlikely(p->fds[i].fd != -1)) {
                p->max = (size_t)i;
                break;
            }
        }
    }
    netdata_thread_enable_cancelability();

    debug(D_POLLFD, "POLLFD: DEL: completed, slots = %zu, used = %zu, min = %zu, max = %zu, next free = %zd", p->slots, p->used, p->min, p->max, p->first_free?(ssize_t)p->first_free->slot:(ssize_t)-1);
}

void *poll_default_add_callback(POLLINFO *pi, short int *events, void *data) {
    (void)pi;
    (void)events;
    (void)data;

    // error("POLLFD: internal error: poll_default_add_callback() called");

    return NULL;
}

void poll_default_del_callback(POLLINFO *pi) {
    if(pi->data)
        error("POLLFD: internal error: del_callback_default() called with data pointer - possible memory leak");
}

int poll_default_rcv_callback(POLLINFO *pi, short int *events) {
    *events |= POLLIN;

    char buffer[1024 + 1];

    ssize_t rc;
    do {
        rc = recv(pi->fd, buffer, 1024, MSG_DONTWAIT);
        if (rc < 0) {
            // read failed
            if (errno != EWOULDBLOCK && errno != EAGAIN) {
                error("POLLFD: poll_default_rcv_callback(): recv() failed with %zd.", rc);
                return -1;
            }
        } else if (rc) {
            // data received
            info("POLLFD: internal error: poll_default_rcv_callback() is discarding %zd bytes received on socket %d", rc, pi->fd);
        }
    } while (rc != -1);

    return 0;
}

int poll_default_snd_callback(POLLINFO *pi, short int *events) {
    *events &= ~POLLOUT;

    info("POLLFD: internal error: poll_default_snd_callback(): nothing to send on socket %d", pi->fd);
    return 0;
}

void poll_default_tmr_callback(void *timer_data) {
    (void)timer_data;
}

static void poll_events_cleanup(void *data) {
    POLLJOB *p = (POLLJOB *)data;

    size_t i;
    for(i = 0 ; i <= p->max ; i++) {
        POLLINFO *pi = &p->inf[i];
        poll_close_fd(pi);
    }

    freez(p->fds);
    freez(p->inf);
}

static int poll_process_error(POLLINFO *pi, struct pollfd *pf, short int revents) {
    error("POLLFD: LISTENER: received %s %s %s on socket at slot %zu (fd %d) client '%s' port '%s' expecting %s %s %s, having %s %s %s"
          , revents & POLLERR  ? "POLLERR" : ""
          , revents & POLLHUP  ? "POLLHUP" : ""
          , revents & POLLNVAL ? "POLLNVAL" : ""
          , pi->slot
          , pi->fd
          , pi->client_ip ? pi->client_ip : "<undefined-ip>"
          , pi->client_port ? pi->client_port : "<undefined-port>"
          , pf->events & POLLIN ? "POLLIN" : "", pf->events & POLLOUT ? "POLLOUT" : "", pf->events & POLLPRI ? "POLLPRI" : ""
          , revents & POLLIN ? "POLLIN" : "", revents & POLLOUT ? "POLLOUT" : "", revents & POLLPRI ? "POLLPRI" : ""
          );

    pf->events = 0;
    poll_close_fd(pi);
    return 1;
}

static inline int poll_process_send(POLLJOB *p, POLLINFO *pi, struct pollfd *pf, time_t now) {
    pi->last_sent_t = now;
    pi->send_count++;

    debug(D_POLLFD, "POLLFD: LISTENER: sending data to socket on slot %zu (fd %d)", pi->slot, pf->fd);

    pf->events = 0;

    // remember the slot, in case we need to close it later
    // the callback may manipulate the socket list and our pf and pi pointers may be invalid after that call
    size_t slot = pi->slot;

    if (unlikely(pi->snd_callback(pi, &pf->events) == -1))
        poll_close_fd(&p->inf[slot]);

    // IMPORTANT:
    // pf and pi may be invalid below this point, they may have been reallocated.

    return 1;
}

static inline int poll_process_tcp_read(POLLJOB *p, POLLINFO *pi, struct pollfd *pf, time_t now) {
    pi->last_received_t = now;
    pi->recv_count++;

    debug(D_POLLFD, "POLLFD: LISTENER: reading data from TCP client slot %zu (fd %d)", pi->slot, pf->fd);

    pf->events = 0;

    // remember the slot, in case we need to close it later
    // the callback may manipulate the socket list and our pf and pi pointers may be invalid after that call
    size_t slot = pi->slot;

    if (pi->rcv_callback(pi, &pf->events) == -1)
        poll_close_fd(&p->inf[slot]);

    // IMPORTANT:
    // pf and pi may be invalid below this point, they may have been reallocated.

    return 1;
}

static inline int poll_process_udp_read(POLLINFO *pi, struct pollfd *pf, time_t now __maybe_unused) {
    pi->last_received_t = now;
    pi->recv_count++;

    debug(D_POLLFD, "POLLFD: LISTENER: reading data from UDP slot %zu (fd %d)", pi->slot, pf->fd);

    // TODO: access_list is not applied to UDP
    // but checking the access list on every UDP packet will destroy
    // performance, especially for statsd.

    pf->events = 0;
    if(pi->rcv_callback(pi, &pf->events) == -1)
        return 0;

    // IMPORTANT:
    // pf and pi may be invalid below this point, they may have been reallocated.

    return 1;
}

static int poll_process_new_tcp_connection(POLLJOB *p, POLLINFO *pi, struct pollfd *pf, time_t now) {
    pi->last_received_t = now;
    pi->recv_count++;

    debug(D_POLLFD, "POLLFD: LISTENER: accepting connections from slot %zu (fd %d)", pi->slot, pf->fd);

    char client_ip[INET6_ADDRSTRLEN] = "";
    char client_port[NI_MAXSERV] = "";
    char client_host[NI_MAXHOST] = "";

    debug(D_POLLFD, "POLLFD: LISTENER: calling accept4() slot %zu (fd %d)", pi->slot, pf->fd);

    int nfd = accept_socket(
        pf->fd,SOCK_NONBLOCK,
        client_ip, INET6_ADDRSTRLEN, client_port,NI_MAXSERV, client_host, NI_MAXHOST,
        p->access_list, p->allow_dns
        );

    if (unlikely(nfd < 0)) {
        // accept failed

        debug(D_POLLFD, "POLLFD: LISTENER: accept4() slot %zu (fd %d) failed.", pi->slot, pf->fd);

        if(unlikely(errno == EMFILE)) {
            error_limit_static_global_var(erl, 10, 1000);
            error_limit(&erl, "POLLFD: LISTENER: too many open files - used by this thread %zu, max for this thread %zu",
                      p->used, p->limit);
        }
        else if(unlikely(errno != EWOULDBLOCK && errno != EAGAIN))
            error("POLLFD: LISTENER: accept() failed.");

    }
    else {
        // accept ok

        poll_add_fd(p
                    , nfd
                    , SOCK_STREAM
                    , pi->port_acl
                    , POLLINFO_FLAG_CLIENT_SOCKET
                    , client_ip
                    , client_port
                    , client_host
                    , p->add_callback
                    , p->del_callback
                    , p->rcv_callback
                    , p->snd_callback
                    , NULL
        );

        // IMPORTANT:
        // pf and pi may be invalid below this point, they may have been reallocated.

        return 1;
    }

    return 0;
}

void poll_events(LISTEN_SOCKETS *sockets
        , void *(*add_callback)(POLLINFO * /*pi*/, short int * /*events*/, void * /*data*/)
        , void  (*del_callback)(POLLINFO * /*pi*/)
        , int   (*rcv_callback)(POLLINFO * /*pi*/, short int * /*events*/)
        , int   (*snd_callback)(POLLINFO * /*pi*/, short int * /*events*/)
        , void  (*tmr_callback)(void * /*timer_data*/)
        , SIMPLE_PATTERN *access_list
        , int allow_dns
        , void *data
        , time_t tcp_request_timeout_seconds
        , time_t tcp_idle_timeout_seconds
        , time_t timer_milliseconds
        , void *timer_data
        , size_t max_tcp_sockets
) {
    if(!sockets || !sockets->opened) {
        error("POLLFD: internal error: no listening sockets are opened");
        return;
    }

    if(timer_milliseconds <= 0) timer_milliseconds = 0;

    int retval;

    POLLJOB p = {
            .slots = 0,
            .used = 0,
            .max = 0,
            .limit = max_tcp_sockets,
            .fds = NULL,
            .inf = NULL,
            .first_free = NULL,

            .complete_request_timeout = tcp_request_timeout_seconds,
            .idle_timeout = tcp_idle_timeout_seconds,
            .checks_every = (tcp_idle_timeout_seconds / 3) + 1,

            .access_list = access_list,
            .allow_dns   = allow_dns,

            .timer_milliseconds = timer_milliseconds,
            .timer_data = timer_data,

            .add_callback = add_callback?add_callback:poll_default_add_callback,
            .del_callback = del_callback?del_callback:poll_default_del_callback,
            .rcv_callback = rcv_callback?rcv_callback:poll_default_rcv_callback,
            .snd_callback = snd_callback?snd_callback:poll_default_snd_callback,
            .tmr_callback = tmr_callback?tmr_callback:poll_default_tmr_callback
    };

    size_t i;
    for(i = 0; i < sockets->opened ;i++) {

        POLLINFO *pi = poll_add_fd(&p
                                   , sockets->fds[i]
                                   , sockets->fds_types[i]
                                   , sockets->fds_acl_flags[i]
                                   , POLLINFO_FLAG_SERVER_SOCKET
                                   , (sockets->fds_names[i])?sockets->fds_names[i]:"UNKNOWN"
                                   , ""
                                   , ""
                                   , p.add_callback
                                   , p.del_callback
                                   , p.rcv_callback
                                   , p.snd_callback
                                   , NULL
        );

        pi->data = data;
        info("POLLFD: LISTENER: listening on '%s'", (sockets->fds_names[i])?sockets->fds_names[i]:"UNKNOWN");
    }

    int listen_sockets_active = 1;

    int timeout_ms = 1000; // in milliseconds
    time_t last_check = now_boottime_sec();

    usec_t timer_usec = timer_milliseconds * USEC_PER_MS;
    usec_t now_usec = 0, next_timer_usec = 0, last_timer_usec = 0;
    (void)last_timer_usec;

    if(unlikely(timer_usec)) {
        now_usec = now_boottime_usec();
        next_timer_usec = now_usec - (now_usec % timer_usec) + timer_usec;
    }

    netdata_thread_cleanup_push(poll_events_cleanup, &p);

    while(!netdata_exit) {
        if(unlikely(timer_usec)) {
            now_usec = now_boottime_usec();

            if(unlikely(timer_usec && now_usec >= next_timer_usec)) {
                debug(D_POLLFD, "Calling timer callback after %zu usec", (size_t)(now_usec - last_timer_usec));
                last_timer_usec = now_usec;
                p.tmr_callback(p.timer_data);
                now_usec = now_boottime_usec();
                next_timer_usec = now_usec - (now_usec % timer_usec) + timer_usec;
            }

            usec_t dt_usec = next_timer_usec - now_usec;
            if(dt_usec < 1000 * USEC_PER_MS)
                timeout_ms = 1000;
            else
                timeout_ms = (int)(dt_usec / USEC_PER_MS);
        }

        // enable or disable the TCP listening sockets, based on the current number of sockets used and the limit set
        if((listen_sockets_active && (p.limit && p.used >= p.limit)) || (!listen_sockets_active && (!p.limit || p.used < p.limit))) {
            listen_sockets_active = !listen_sockets_active;
            info("%s listening sockets (used TCP sockets %zu, max allowed for this worker %zu)", (listen_sockets_active)?"ENABLING":"DISABLING", p.used, p.limit);
            for (i = 0; i <= p.max; i++) {
                if(p.inf[i].flags & POLLINFO_FLAG_SERVER_SOCKET && p.inf[i].socktype == SOCK_STREAM) {
                    p.fds[i].events = (short int) ((listen_sockets_active) ? POLLIN : 0);
                }
            }
        }

        debug(D_POLLFD, "POLLFD: LISTENER: Waiting on %zu sockets for %zu ms...", p.max + 1, (size_t)timeout_ms);
        retval = poll(p.fds, p.max + 1, timeout_ms);
        time_t now = now_boottime_sec();

        if(unlikely(retval == -1)) {
            error("POLLFD: LISTENER: poll() failed while waiting on %zu sockets.", p.max + 1);
            break;
        }
        else if(unlikely(!retval)) {
            debug(D_POLLFD, "POLLFD: LISTENER: poll() timeout.");
        }
        else {
            POLLINFO *pi;
            struct pollfd *pf;
            size_t idx, processed = 0;
            short int revents;

            // keep fast lookup arrays per function
            // to avoid looping through the entire list every time
            size_t sends[p.max + 1], sends_max = 0;
            size_t reads[p.max + 1], reads_max = 0;
            size_t conns[p.max + 1], conns_max = 0;
            size_t udprd[p.max + 1], udprd_max = 0;

            for (i = 0; i <= p.max; i++) {
                pi = &p.inf[i];
                pf = &p.fds[i];
                revents = pf->revents;

                if(unlikely(revents == 0 || pf->fd == -1))
                    continue;

                if (unlikely(revents & (POLLERR|POLLHUP|POLLNVAL))) {
                    // something is wrong to one of our sockets

                    pf->revents = 0;
                    processed += poll_process_error(pi, pf, revents);
                }
                else if (likely(revents & POLLOUT)) {
                    // a client is ready to receive data

                    sends[sends_max++] = i;
                }
                else if (likely(revents & (POLLIN|POLLPRI))) {
                    if (pi->flags & POLLINFO_FLAG_CLIENT_SOCKET) {
                        // a client sent data to us

                        reads[reads_max++] = i;
                    }
                    else if (pi->flags & POLLINFO_FLAG_SERVER_SOCKET) {
                        // something is coming to our server sockets

                        if(pi->socktype == SOCK_DGRAM) {
                            // UDP receive, directly on our listening socket

                            udprd[udprd_max++] = i;
                        }
                        else if(pi->socktype == SOCK_STREAM) {
                            // new TCP connection

                            conns[conns_max++] = i;
                        }
                        else
                            error("POLLFD: LISTENER: server slot %zu (fd %d) connection from %s port %s using unhandled socket type %d."
                                 , i
                                 , pi->fd
                                 , pi->client_ip ? pi->client_ip : "<undefined-ip>"
                                 , pi->client_port ? pi->client_port : "<undefined-port>"
                                 , pi->socktype
                            );
                    }
                    else
                        error("POLLFD: LISTENER: client slot %zu (fd %d) data from %s port %s using flags %08X is neither client nor server."
                              , i
                              , pi->fd
                              , pi->client_ip ? pi->client_ip : "<undefined-ip>"
                              , pi->client_port ? pi->client_port : "<undefined-port>"
                              , pi->flags
                        );
                }
                else
                    error("POLLFD: LISTENER: socket slot %zu (fd %d) client %s port %s unhandled event id %d."
                      , i
                      , pi->fd
                      , pi->client_ip ? pi->client_ip : "<undefined-ip>"
                      , pi->client_port ? pi->client_port : "<undefined-port>"
                      , revents
                );
            }

            // process sends
            for (idx = 0; idx < sends_max; idx++) {
                i = sends[idx];
                pi = &p.inf[i];
                pf = &p.fds[i];
                pf->revents = 0;
                processed += poll_process_send(&p, pi, pf, now);
            }

            // process UDP reads
            for (idx = 0; idx < udprd_max; idx++) {
                i = udprd[idx];
                pi = &p.inf[i];
                pf = &p.fds[i];
                pf->revents = 0;
                processed += poll_process_udp_read(pi, pf, now);
            }

            // process TCP reads
            for (idx = 0; idx < reads_max; idx++) {
                i = reads[idx];
                pi = &p.inf[i];
                pf = &p.fds[i];
                pf->revents = 0;
                processed += poll_process_tcp_read(&p, pi, pf, now);
            }

            if(!processed && (!p.limit || p.used < p.limit)) {
                // nothing processed above (rcv, snd) and we have room for another TCP connection
                // so, accept one TCP connection
                for (idx = 0; idx < conns_max; idx++) {
                    i = conns[idx];
                    pi = &p.inf[i];
                    pf = &p.fds[i];
                    pf->revents = 0;
                    if (poll_process_new_tcp_connection(&p, pi, pf, now))
                        break;
                }
            }
        }

        if(unlikely(p.checks_every > 0 && now - last_check > p.checks_every)) {
            last_check = now;

            // cleanup old sockets
            for(i = 0; i <= p.max; i++) {
                POLLINFO *pi = &p.inf[i];

                if(likely(pi->flags & POLLINFO_FLAG_CLIENT_SOCKET)) {
                    if (unlikely(pi->send_count == 0 && p.complete_request_timeout > 0 && (now - pi->connected_t) >= p.complete_request_timeout)) {
                        info("POLLFD: LISTENER: client slot %zu (fd %d) from %s port %s has not sent a complete request in %zu seconds - closing it. "
                              , i
                              , pi->fd
                              , pi->client_ip ? pi->client_ip : "<undefined-ip>"
                              , pi->client_port ? pi->client_port : "<undefined-port>"
                              , (size_t) p.complete_request_timeout
                        );
                        poll_close_fd(pi);
                    }
                    else if(unlikely(pi->recv_count && p.idle_timeout > 0 && now - ((pi->last_received_t > pi->last_sent_t) ? pi->last_received_t : pi->last_sent_t) >= p.idle_timeout )) {
                        info("POLLFD: LISTENER: client slot %zu (fd %d) from %s port %s is idle for more than %zu seconds - closing it. "
                              , i
                              , pi->fd
                              , pi->client_ip ? pi->client_ip : "<undefined-ip>"
                              , pi->client_port ? pi->client_port : "<undefined-port>"
                              , (size_t) p.idle_timeout
                        );
                        poll_close_fd(pi);
                    }
                }
            }
        }
    }

    netdata_thread_cleanup_pop(1);
    debug(D_POLLFD, "POLLFD: LISTENER: cleanup completed");
}
