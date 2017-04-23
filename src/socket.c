#include "common.h"

// --------------------------------------------------------------------------------------------------------------------
// listening sockets

int create_listen_socket4(int socktype, const char *ip, int port, int listen_backlog) {
    int sock;
    int sockopt = 1;

    debug(D_LISTENER, "IPv4 creating new listening socket on ip '%s' port %d", ip, port);

    sock = socket(AF_INET, socktype, 0);
    if(sock < 0) {
        error("IPv4 socket() on ip '%s' port %d failed.", ip, port);
        return -1;
    }

    /* avoid "address already in use" */
    if(setsockopt(sock, SOL_SOCKET, SO_REUSEADDR, (void*)&sockopt, sizeof(sockopt)) != 0)
        error("Cannot set SO_REUSEADDR on ip '%s' port's %d.", ip, port);

    struct sockaddr_in name;
    memset(&name, 0, sizeof(struct sockaddr_in));
    name.sin_family = AF_INET;
    name.sin_port = htons (port);

    int ret = inet_pton(AF_INET, ip, (void *)&name.sin_addr.s_addr);
    if(ret != 1) {
        error("Failed to convert IP '%s' to a valid IPv4 address.", ip);
        close(sock);
        return -1;
    }

    if(bind (sock, (struct sockaddr *) &name, sizeof (name)) < 0) {
        close(sock);
        error("IPv4 bind() on ip '%s' port %d failed.", ip, port);
        return -1;
    }

    if(listen(sock, listen_backlog) < 0) {
        close(sock);
        error("IPv4 listen() on ip '%s' port %d failed.", ip, port);
        return -1;
    }

    debug(D_LISTENER, "Listening on IPv4 ip '%s' port %d", ip, port);
    return sock;
}

int create_listen_socket6(int socktype, uint32_t scope_id, const char *ip, int port, int listen_backlog) {
    int sock = -1;
    int sockopt = 1;
    int ipv6only = 1;

    debug(D_LISTENER, "IPv6 creating new listening socket on ip '%s' port %d", ip, port);

    sock = socket(AF_INET6, socktype, 0);
    if (sock < 0) {
        error("IPv6 socket() on ip '%s' port %d failed.", ip, port);
        return -1;
    }

    /* avoid "address already in use" */
    if(setsockopt(sock, SOL_SOCKET, SO_REUSEADDR, (void*)&sockopt, sizeof(sockopt)) != 0)
        error("Cannot set SO_REUSEADDR on ip '%s' port's %d.", ip, port);

    /* IPv6 only */
    if(setsockopt(sock, IPPROTO_IPV6, IPV6_V6ONLY, (void*)&ipv6only, sizeof(ipv6only)) != 0)
        error("Cannot set IPV6_V6ONLY on ip '%s' port's %d.", ip, port);

    struct sockaddr_in6 name;
    memset(&name, 0, sizeof(struct sockaddr_in6));
    name.sin6_family = AF_INET6;
    name.sin6_port = htons ((uint16_t) port);
    name.sin6_scope_id = scope_id;

    int ret = inet_pton(AF_INET6, ip, (void *)&name.sin6_addr.s6_addr);
    if(ret != 1) {
        error("Failed to convert IP '%s' to a valid IPv6 address.", ip);
        close(sock);
        return -1;
    }

    name.sin6_scope_id = scope_id;

    if (bind (sock, (struct sockaddr *) &name, sizeof (name)) < 0) {
        close(sock);
        error("IPv6 bind() on ip '%s' port %d failed.", ip, port);
        return -1;
    }

    if (listen(sock, listen_backlog) < 0) {
        close(sock);
        error("IPv6 listen() on ip '%s' port %d failed.", ip, port);
        return -1;
    }

    debug(D_LISTENER, "Listening on IPv6 ip '%s' port %d", ip, port);
    return sock;
}

static inline int listen_sockets_add(LISTEN_SOCKETS *sockets, int fd, const char *protocol, const char *ip, int port) {
    if(sockets->opened >= MAX_LISTEN_FDS) {
        error("Too many listening sockets. Failed to add listening %s socket at ip '%s' port %d", protocol, ip, port);
        close(fd);
        return -1;
    }

    sockets->fds[sockets->opened] = fd;

    char buffer[100 + 1];
    snprintfz(buffer, 100, "%s:[%s]:%d", protocol, ip, port);
    sockets->fds_names[sockets->opened] = strdupz(buffer);

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
    }

    sockets->opened = 0;
    sockets->failed = 0;
}

static inline int bind_to_one(LISTEN_SOCKETS *sockets, const char *definition, int default_port, int listen_backlog) {
    int added = 0;
    struct addrinfo hints;
    struct addrinfo *result = NULL, *rp = NULL;

    char buffer[strlen(definition) + 1];
    strcpy(buffer, definition);

    char buffer2[10 + 1];
    snprintfz(buffer2, 10, "%d", default_port);

    char *ip = buffer, *port = buffer2, *interface = "";;

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
        while(*e && *e != ':' && *e != '%') e++;
    }

    if(*e == '%') {
        *e = '\0';
        e++;
        interface = e;
        while(*e && *e != ':') e++;
    }

    if(*e == ':') {
        port = e + 1;
        *e = '\0';
    }

    uint32_t scope_id = 0;
    if(*interface) {
        scope_id = if_nametoindex(interface);
        if(!scope_id)
            error("Cannot find a network interface named '%s'. Continuing with limiting the network interface", interface);
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
        error("getaddrinfo('%s', '%s'): %s\n", ip, port, gai_strerror(r));
        return -1;
    }

    for (rp = result; rp != NULL; rp = rp->ai_next) {
        int fd = -1;

        char rip[INET_ADDRSTRLEN + INET6_ADDRSTRLEN] = "INVALID";
        int rport = default_port;

        switch (rp->ai_addr->sa_family) {
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
                debug(D_LISTENER, "Unknown socket family %d", rp->ai_addr->sa_family);
                break;
        }

        if (fd == -1) {
            error("Cannot bind to ip '%s', port %d", rip, rport);
            sockets->failed++;
        }
        else {
            listen_sockets_add(sockets, fd, protocol_str, rip, rport);
            added++;
        }
    }

    freeaddrinfo(result);

    return added;
}

int listen_sockets_setup(LISTEN_SOCKETS *sockets) {
    listen_sockets_init(sockets);

    sockets->backlog = (int) config_get_number(sockets->config_section, "listen backlog", sockets->backlog);

    int old_port = sockets->default_port;
    sockets->default_port = (int) config_get_number(sockets->config_section, "default port", sockets->default_port);
    if(sockets->default_port < 1 || sockets->default_port > 65535) {
        error("Invalid listen port %d given. Defaulting to %d.", sockets->default_port, old_port);
        sockets->default_port = (int) config_set_number(sockets->config_section, "default port", old_port);
    }
    debug(D_OPTIONS, "Default listen port set to %d.", sockets->default_port);

    char *s = config_get(sockets->config_section, "bind to", "*");
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
        bind_to_one(sockets, buf, sockets->default_port, sockets->backlog);

        s = e;
    }

    if(!sockets->opened)
        fatal("Cannot listen on any socket. Exiting...");

    else if(sockets->failed) {
        size_t i;
        for(i = 0; i < sockets->opened ;i++)
            info("Listen socket %s opened successfully.", sockets->fds_names[i]);
    }

    return (int)sockets->opened;
}


// --------------------------------------------------------------------------------------------------------------------
// connect to another host/port

// _connect_to()
// protocol    IPPROTO_TCP, IPPROTO_UDP
// socktype    SOCK_STREAM, SOCK_DGRAM
// host        the destination hostname or IP address (IPv4 or IPv6) to connect to
//             if it resolves to many IPs, all are tried (IPv4 and IPv6)
// scope_id    the if_index id of the interface to use for connecting (0 = any)
//             (used only under IPv6)
// service     the service name or port to connect to
// timeout     the timeout for establishing a connection

static inline int _connect_to(int protocol, int socktype, const char *host, uint32_t scope_id, const char *service, struct timeval *timeout) {
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

            if(connect(fd, ai->ai_addr, ai->ai_addrlen) < 0) {
                error("Failed to connect to '%s', port '%s'", hostBfr, servBfr);
                close(fd);
                fd = -1;
            }

            debug(D_CONNECT_TO, "Connected to '%s' on port '%s'.", hostBfr, servBfr);
        }
    }

    freeaddrinfo(ai_head);

    return fd;
}

// connect_to()
//
// definition format:
//
//    [PROTOCOL:]IP[%INTERFACE][:PORT]
//
// PROTOCOL  = tcp or udp
// IP        = IPv4 or IPv6 IP or hostname, optionally enclosed in [] (required for IPv6)
// INTERFACE = for IPv6 only, the network interface to use
// PORT      = port number or service name

int connect_to(const char *definition, int default_port, struct timeval *timeout) {
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


    return _connect_to(protocol, socktype, host, scope_id, service, timeout);
}

int connect_to_one_of(const char *destination, int default_port, struct timeval *timeout, size_t *reconnects_counter, char *connected_to, size_t connected_to_size) {
    int sock = -1;

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
        if(reconnects_counter) *reconnects_counter += 1;
        sock = connect_to(buf, default_port, timeout);
        if(sock != -1) {
            if(connected_to && connected_to_size) {
                strncpy(connected_to, buf, connected_to_size);
                connected_to[connected_to_size - 1] = '\0';
            }
            break;
        }
        s = e;
    }

    return sock;
}


// --------------------------------------------------------------------------------------------------------------------
// helpers to send/receive data in one call, in blocking mode, with a timeout

ssize_t recv_timeout(int sockfd, void *buf, size_t len, int flags, int timeout) {
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

    return recv(sockfd, buf, len, flags);
}

ssize_t send_timeout(int sockfd, void *buf, size_t len, int flags, int timeout) {
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
        errno = -EINVAL;
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

