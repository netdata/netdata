#include "common.h"

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
    int ret = -1;

#ifdef SO_REUSEPORT
    ret = setsockopt(fd, SOL_SOCKET, SO_REUSEPORT, &reuse, sizeof(reuse));
    if(ret == -1 && errno != ENOPROTOOPT)
        error("failed to set SO_REUSEPORT on socket %d", fd);
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

char *strdup_client_description(int family, const char *protocol, const char *ip, int port) {
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

int create_listen_socket4(int socktype, const char *ip, int port, int listen_backlog) {
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

static inline int listen_sockets_add(LISTEN_SOCKETS *sockets, int fd, int family, int socktype, const char *protocol, const char *ip, int port) {
    if(sockets->opened >= MAX_LISTEN_FDS) {
        error("LISTENER: Too many listening sockets. Failed to add listening %s socket at ip '%s' port %d, protocol %s, socktype %d", protocol, ip, port, protocol, socktype);
        close(fd);
        return -1;
    }

    sockets->fds[sockets->opened] = fd;
    sockets->fds_types[sockets->opened] = socktype;
    sockets->fds_families[sockets->opened] = family;
    sockets->fds_names[sockets->opened] = strdup_client_description(family, protocol, ip, port);

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

static inline int bind_to_this(LISTEN_SOCKETS *sockets, const char *definition, int default_port, int listen_backlog) {
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
    else if(strncmp(ip, "unix:", 5) == 0) {
        char *path = ip + 5;
        socktype = SOCK_STREAM;
        protocol_str = "unix";

        int fd = create_listen_socket_unix(path, listen_backlog);
        if (fd == -1) {
            error("LISTENER: Cannot create unix socket '%s'", path);
            sockets->failed++;
        }
        else {
            listen_sockets_add(sockets, fd, AF_UNIX, socktype, protocol_str, path, 0);
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
        int family = -1;

        char rip[INET_ADDRSTRLEN + INET6_ADDRSTRLEN] = "INVALID";
        int rport = default_port;

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
            listen_sockets_add(sockets, fd, family, socktype, protocol_str, rip, rport);
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
        error("LISTENER: Invalid listen port %d given. Defaulting to %d.", sockets->default_port, old_port);
        sockets->default_port = (int) config_set_number(sockets->config_section, "default port", old_port);
    }
    debug(D_OPTIONS, "LISTENER: Default listen port set to %d.", sockets->default_port);

    char *s = config_get(sockets->config_section, "bind to", sockets->default_bind_to);
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

static inline int connect_to_this_ip46(int protocol, int socktype, const char *host, uint32_t scope_id, const char *service, struct timeval *timeout) {
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
        sock = connect_to_this(buf, default_port, timeout);
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


// --------------------------------------------------------------------------------------------------------------------
// accept_socket() - accept a socket and store client IP and port

int accept_socket(int fd, int flags, char *client_ip, size_t ipsize, char *client_port, size_t portsize, SIMPLE_PATTERN *access_list) {
    struct sockaddr_storage sadr;
    socklen_t addrlen = sizeof(sadr);

    int nfd = accept4(fd, (struct sockaddr *)&sadr, &addrlen, flags);
    if (likely(nfd >= 0)) {
        if (getnameinfo((struct sockaddr *)&sadr, addrlen, client_ip, (socklen_t)ipsize, client_port, (socklen_t)portsize, NI_NUMERICHOST | NI_NUMERICSERV) != 0) {
            error("LISTENER: cannot getnameinfo() on received client connection.");
            strncpyz(client_ip, "UNKNOWN", ipsize - 1);
            strncpyz(client_port, "UNKNOWN", portsize - 1);
        }

        client_ip[ipsize - 1] = '\0';
        client_port[portsize - 1] = '\0';

        switch (((struct sockaddr *)&sadr)->sa_family) {
            case AF_UNIX:
                debug(D_LISTENER, "New UNIX domain web client from %s on socket %d.", client_ip, fd);
                // set the port - certain versions of libc return garbage on unix sockets
                strncpy(client_port, "UNIX", portsize);
                client_port[portsize - 1] = '\0';
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

        if(access_list) {
            if(!strcmp(client_ip, "127.0.0.1") || !strcmp(client_ip, "::1")) {
                strncpy(client_ip, "localhost", ipsize);
                client_ip[ipsize - 1] = '\0';
            }

            if(unlikely(!simple_pattern_matches(access_list, client_ip))) {
                errno = 0;
                debug(D_LISTENER, "Permission denied for client '%s', port '%s'", client_ip, client_port);
                error("DENIED ACCESS to client '%s'", client_ip);
                close(nfd);
                nfd = -1;
                errno = EPERM;
            }
        }
    }
#ifdef HAVE_ACCEPT4
    else if(errno == ENOSYS)
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
                             , uint32_t flags
                             , const char *client_ip
                             , const char *client_port
                             , void *(*add_callback)(POLLINFO *pi, short int *events, void *data)
                             , void  (*del_callback)(POLLINFO *pi)
                             , int   (*rcv_callback)(POLLINFO *pi, short int *events)
                             , int   (*snd_callback)(POLLINFO *pi, short int *events)
                             , void *data
) {
    debug(D_POLLFD, "POLLFD: ADD: request to add fd %d, slots = %zu, used = %zu, min = %zu, max = %zu, next free = %zd", fd, p->slots, p->used, p->min, p->max, p->first_free?(ssize_t)p->first_free->slot:(ssize_t)-1);

    if(unlikely(fd < 0)) return NULL;

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
            p->inf[i].client_ip = NULL;
            p->inf[i].client_port = NULL;
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
    pi->flags = flags;
    pi->next = NULL;
    pi->client_ip = strdupz(client_ip);
    pi->client_port = strdupz(client_port);

    pi->del_callback = del_callback;
    pi->rcv_callback = rcv_callback;
    pi->snd_callback = snd_callback;

    p->used++;
    if(unlikely(pi->slot > p->max))
        p->max = pi->slot;

    if(pi->flags & POLLINFO_FLAG_CLIENT_SOCKET) {
        pi->data = add_callback(pi, &pf->events, data);
    }

    if(pi->flags & POLLINFO_FLAG_SERVER_SOCKET) {
        p->min = pi->slot;
    }

    debug(D_POLLFD, "POLLFD: ADD: completed, slots = %zu, used = %zu, min = %zu, max = %zu, next free = %zd", p->slots, p->used, p->min, p->max, p->first_free?(ssize_t)p->first_free->slot:(ssize_t)-1);

    return pi;
}

inline void poll_close_fd(POLLINFO *pi) {
    POLLJOB *p = pi->p;

    struct pollfd *pf = &p->fds[pi->slot];
    debug(D_POLLFD, "POLLFD: DEL: request to clear slot %zu (fd %d), old next free was %zd", pi->slot, pf->fd, p->first_free?(ssize_t)p->first_free->slot:(ssize_t)-1);

    if(unlikely(pf->fd == -1)) return;

    if(pi->flags & POLLINFO_FLAG_CLIENT_SOCKET) {
        pi->del_callback(pi);
    }

    // info("POLLFD: closing fd %d", pf->fd);
    close(pf->fd);
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

    debug(D_POLLFD, "POLLFD: DEL: completed, slots = %zu, used = %zu, min = %zu, max = %zu, next free = %zd", p->slots, p->used, p->min, p->max, p->first_free?(ssize_t)p->first_free->slot:(ssize_t)-1);
}

void *poll_default_add_callback(POLLINFO *pi, short int *events, void *data) {
    (void)pi;
    (void)events;
    (void)data;

    return NULL;
}

void poll_default_del_callback(POLLINFO *pi) {
    if(pi->data)
        error("POLLFD: internal error: del_callback_default() called with data pointer - possible memory leak");
}

int poll_default_rcv_callback(POLLINFO *pi, short int *events) {
    (void)events;

    char buffer[1024 + 1];

    ssize_t rc;
    do {
        rc = recv(pi->fd, buffer, 1024, MSG_DONTWAIT);
        if (rc < 0) {
            // read failed
            if (errno != EWOULDBLOCK && errno != EAGAIN) {
                error("POLLFD: recv() failed.");
                return -1;
            }
        } else if (rc) {
            // data received
            info("POLLFD: internal error: discarding %zd bytes received on socket %d", rc, pi->fd);
        }
    } while (rc != -1);

    return 0;
}

int poll_default_snd_callback(POLLINFO *pi, short int *events) {
    *events &= ~POLLOUT;

    info("POLLFD: internal error: nothing to send on socket %d", pi->fd);
    return 0;
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

static void poll_events_process(POLLJOB *p, POLLINFO *pi, struct pollfd *pf, short int revents) {
    short int events = pf->events;
    int fd = pf->fd;
    pf->revents = 0;
    size_t i = pi->slot;

    if(unlikely(fd == -1)) {
        debug(D_POLLFD, "POLLFD: LISTENER: ignoring slot %zu, it does not have an fd", i);
        return;
    }

    debug(D_POLLFD, "POLLFD: LISTENER: processing events for slot %zu (events = %d, revents = %d)", i, events, revents);

    if(revents & POLLIN || revents & POLLPRI) {
        // receiving data

        if(likely(pi->flags & POLLINFO_FLAG_SERVER_SOCKET)) {
            // new connection
            // debug(D_POLLFD, "POLLFD: LISTENER: accepting connections from slot %zu (fd %d)", i, fd);

            switch(pi->socktype) {
                case SOCK_STREAM: {
                    // a TCP socket
                    // we accept the connection

                    int nfd;
                    do {
                        char client_ip[NI_MAXHOST + 1];
                        char client_port[NI_MAXSERV + 1];

                        debug(D_POLLFD, "POLLFD: LISTENER: calling accept4() slot %zu (fd %d)", i, fd);
                        nfd = accept_socket(fd, SOCK_NONBLOCK, client_ip, NI_MAXHOST + 1, client_port, NI_MAXSERV + 1, p->access_list);
                        if (unlikely(nfd < 0)) {
                            // accept failed

                            debug(D_POLLFD, "POLLFD: LISTENER: accept4() slot %zu (fd %d) failed.", i, fd);

                            if(errno != EWOULDBLOCK && errno != EAGAIN)
                                error("POLLFD: LISTENER: accept() failed.");

                            break;
                        }
                        else {
                            // accept ok
                            // info("POLLFD: LISTENER: client '[%s]:%s' connected to '%s' on fd %d", client_ip, client_port, sockets->fds_names[i], nfd);
                            poll_add_fd(p
                                        , nfd
                                        , SOCK_STREAM
                                        , POLLINFO_FLAG_CLIENT_SOCKET
                                        , client_ip
                                        , client_port
                                        , p->add_callback
                                        , p->del_callback
                                        , p->rcv_callback
                                        , p->snd_callback
                                        , NULL
                            );

                            // it may have reallocated them, so refresh our pointers
                            pf = &p->fds[i];
                            pi = &p->inf[i];
                        }
                    } while (nfd >= 0);
                    break;
                }

                case SOCK_DGRAM: {
                    // a UDP socket
                    // we read data from the server socket

                    debug(D_POLLFD, "POLLFD: LISTENER: reading data from UDP slot %zu (fd %d)", i, fd);

                    // FIXME: access_list is not applied to UDP

                    pf->events = 0;
                    pi->rcv_callback(pi, &pf->events);
                    break;
                }

                default: {
                    error("POLLFD: LISTENER: Unknown socktype %d on slot %zu", pi->socktype, pi->slot);
                    break;
                }
            }
        }

        if(likely(pi->flags & POLLINFO_FLAG_CLIENT_SOCKET)) {
            // read data from client TCP socket
            debug(D_POLLFD, "POLLFD: LISTENER: reading data from TCP client slot %zu (fd %d)", i, fd);

            pf->events = 0;
            if (pi->rcv_callback(pi, &pf->events) == -1) {
                poll_close_fd(pi);
                return;
            }
        }
    }

    if(unlikely(revents & POLLOUT)) {
        // sending data
        debug(D_POLLFD, "POLLFD: LISTENER: sending data to socket on slot %zu (fd %d)", i, fd);

        pf->events = 0;
        if (pi->snd_callback(pi, &pf->events) == -1) {
            poll_close_fd(pi);
            return;
        }
    }

    if(unlikely(revents & POLLERR)) {
        error("POLLFD: LISTENER: processing POLLERR events for slot %zu fd %d (events = %d, revents = %d)", i, events, revents, fd);
        pf->events = 0;
        poll_close_fd(pi);
        return;
    }

    if(unlikely(revents & POLLHUP)) {
        error("POLLFD: LISTENER: processing POLLHUP events for slot %zu fd %d (events = %d, revents = %d)", i, events, revents, fd);
        pf->events = 0;
        poll_close_fd(pi);
        return;
    }

    if(unlikely(revents & POLLNVAL)) {
        error("POLLFD: LISTENER: processing POLLNVAL events for slot %zu fd %d (events = %d, revents = %d)", i, events, revents, fd);
        pf->events = 0;
        poll_close_fd(pi);
        return;
    }
}

void poll_events(LISTEN_SOCKETS *sockets
        , void *(*add_callback)(POLLINFO *pi, short int *events, void *data)
        , void  (*del_callback)(POLLINFO *pi)
        , int   (*rcv_callback)(POLLINFO *pi, short int *events)
        , int   (*snd_callback)(POLLINFO *pi, short int *events)
        , SIMPLE_PATTERN *access_list
        , void *data
) {
    int retval;

    POLLJOB p = {
            .slots = 0,
            .used = 0,
            .max = 0,
            .fds = NULL,
            .inf = NULL,
            .first_free = NULL,

            .access_list = access_list,

            .add_callback = add_callback?add_callback:poll_default_add_callback,
            .del_callback = del_callback?del_callback:poll_default_del_callback,
            .rcv_callback = rcv_callback?rcv_callback:poll_default_rcv_callback,
            .snd_callback = snd_callback?snd_callback:poll_default_snd_callback
    };

    size_t i;
    for(i = 0; i < sockets->opened ;i++) {

        POLLINFO *pi = poll_add_fd(&p
                                   , sockets->fds[i]
                                   , sockets->fds_types[i]
                                   , POLLINFO_FLAG_SERVER_SOCKET
                                   , (sockets->fds_names[i])?sockets->fds_names[i]:"UNKNOWN"
                                   , ""
                                   , poll_default_add_callback
                                   , poll_default_del_callback
                                   , poll_default_rcv_callback
                                   , poll_default_snd_callback
                                   , NULL
        );

        pi->data = data;
        info("POLLFD: LISTENER: listening on '%s'", (sockets->fds_names[i])?sockets->fds_names[i]:"UNKNOWN");
    }

    int timeout = -1; // wait forever

    netdata_thread_cleanup_push(poll_events_cleanup, &p);

    while(!netdata_exit) {
        debug(D_POLLFD, "POLLFD: LISTENER: Waiting on %zu sockets...", p.max + 1);
        retval = poll(p.fds, p.max + 1, timeout);

        if(unlikely(retval == -1)) {
            error("POLLFD: LISTENER: poll() failed.");
            continue;
        }
        else if(unlikely(!retval)) {
            debug(D_POLLFD, "POLLFD: LISTENER: poll() timeout.");
            continue;
        }

        if(unlikely(netdata_exit)) break;

        for(i = 0 ; i <= p.max ; i++) {
            struct pollfd *pf = &p.fds[i];
            short int revents = pf->revents;
            if(unlikely(revents))
                poll_events_process(&p, &p.inf[i], pf, revents);
        }
    }

    netdata_thread_cleanup_pop(1);
    debug(D_POLLFD, "POLLFD: LISTENER: cleanup completed");
}
