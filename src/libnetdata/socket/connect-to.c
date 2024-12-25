// SPDX-License-Identifier: GPL-3.0-or-later

#include "libnetdata/libnetdata.h"

// --------------------------------------------------------------------------------------------------------------------
// connect to another host/port

// connect_to_this_unix()
// path        the path of the unix socket
// timeout     the timeout for establishing a connection

static inline int connect_to_unix(const char *path, struct timeval *timeout) {
    int fd = socket(AF_UNIX, SOCK_STREAM | DEFAULT_SOCKET_FLAGS, 0);
    if(fd == -1) {
        nd_log(NDLS_DAEMON, NDLP_ERR,
               "Failed to create UNIX socket() for '%s'",
               path);

        return -1;
    }

    if(timeout) {
        if(setsockopt(fd, SOL_SOCKET, SO_SNDTIMEO, (char *) timeout, sizeof(struct timeval)) < 0)
            nd_log(NDLS_DAEMON, NDLP_ERR,
                   "Failed to set timeout on UNIX socket '%s'",
                   path);
    }

    sock_setcloexec(fd, true);

    struct sockaddr_un addr;
    memset(&addr, 0, sizeof(addr));
    addr.sun_family = AF_UNIX;
    strncpyz(addr.sun_path, path, sizeof(addr.sun_path) - 1);

    if (connect(fd, (struct sockaddr*)&addr, sizeof(addr)) == -1) {
        nd_log(NDLS_DAEMON, NDLP_ERR,
               "Cannot connect to UNIX socket on path '%s'.",
               path);

        close(fd);
        return -1;
    }

    nd_log(NDLS_DAEMON, NDLP_DEBUG,
           "Connected to UNIX socket on path '%s'.",
           path);

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

int connect_to_this_ip46(
    int protocol,
    int socktype,
    const char *host,
    uint32_t scope_id,
    const char *service,
    struct timeval *timeout,
    bool *fallback_ipv4)
{
    struct addrinfo hints;
    struct addrinfo *ai_head = NULL, *ai = NULL;

    memset(&hints, 0, sizeof(hints));
    hints.ai_family   = PF_UNSPEC;   /* Allow IPv4 or IPv6 */
    hints.ai_socktype = socktype;
    hints.ai_protocol = protocol;

    int ai_err = getaddrinfo(host, service, &hints, &ai_head);
    if (ai_err != 0) {

        nd_log(NDLS_DAEMON, NDLP_ERR,
               "Cannot resolve host '%s', port '%s': %s",
               host, service, gai_strerror(ai_err));

        return -ND_SOCK_ERR_CANNOT_RESOLVE_HOSTNAME;
    }

    char hostBfr[NI_MAXHOST + 1];
    char servBfr[NI_MAXSERV + 1];

    ND_LOG_STACK lgs[] = {
        ND_LOG_FIELD_TXT(NDF_DST_IP, hostBfr),
        ND_LOG_FIELD_TXT(NDF_DST_PORT, servBfr),
        ND_LOG_FIELD_END(),
    };
    ND_LOG_STACK_PUSH(lgs);

    int fd = -1;
    for (ai = ai_head; ai != NULL && fd == -1; ai = ai->ai_next) {
        if(nd_thread_signaled_to_cancel()) break;

        if (fallback_ipv4 && *fallback_ipv4 && ai->ai_family == PF_INET6)
            continue;

        if (ai->ai_family == PF_INET6) {
            struct sockaddr_in6 *pSadrIn6 = (struct sockaddr_in6 *) ai->ai_addr;
            if(pSadrIn6->sin6_scope_id == 0) {
                pSadrIn6->sin6_scope_id = scope_id;
            }
        }

        getnameinfo(ai->ai_addr,
                    ai->ai_addrlen,
                    hostBfr,
                    sizeof(hostBfr),
                    servBfr,
                    sizeof(servBfr),
                    NI_NUMERICHOST | NI_NUMERICSERV);

        switch (ai->ai_addr->sa_family) {
            case PF_INET: {
                struct sockaddr_in *pSadrIn = (struct sockaddr_in *)ai->ai_addr;
                (void)pSadrIn;
                break;
            }

            case PF_INET6: {
                struct sockaddr_in6 *pSadrIn6 = (struct sockaddr_in6 *) ai->ai_addr;
                (void)pSadrIn6;
                break;
            }

            default: {
                // Unknown protocol family
                continue;
            }
        }

        fd = socket(ai->ai_family, ai->ai_socktype | DEFAULT_SOCKET_FLAGS, ai->ai_protocol);
        if(fd != -1) {
            if(timeout) {
                if(setsockopt(fd, SOL_SOCKET, SO_SNDTIMEO, (char *) timeout, sizeof(struct timeval)) < 0)
                    nd_log(NDLS_DAEMON, NDLP_ERR,
                           "Failed to set timeout on the socket to ip '%s' port '%s'",
                           hostBfr, servBfr);
            }
            sock_setcloexec(fd, true);

            errno_clear();
            if(connect(fd, ai->ai_addr, ai->ai_addrlen) < 0) {
                if(errno == EALREADY || errno == EINPROGRESS) {
                    nd_log(NDLS_DAEMON, NDLP_DEBUG,
                           "Waiting for connection to ip %s port %s to be established",
                           hostBfr, servBfr);

                    // Convert 'struct timeval' to milliseconds for poll():
                    int timeout_ms = timeout ? (timeout->tv_sec * 1000 + timeout->tv_usec / 1000) : 1000;

                    switch(wait_on_socket_or_cancel_with_timeout(NULL, fd, timeout_ms, POLLOUT, NULL)) {
                        case  0: // proceed
                            nd_log(NDLS_DAEMON, NDLP_DEBUG,
                                   "connect() to ip %s port %s completed successfully",
                                   hostBfr, servBfr);
                            break;

                        case -1: // thread cancelled
                            nd_log(NDLS_DAEMON, NDLP_ERR,
                                   "Thread is cancelled while connecting to '%s', port '%s'.",
                                   hostBfr, servBfr);

                            close(fd);
                            fd = -ND_SOCK_ERR_THREAD_CANCELLED;
                            break;

                        case  1: // timeout
                            nd_log(NDLS_DAEMON, NDLP_ERR,
                                   "Timed out while connecting to '%s', port '%s'.",
                                   hostBfr, servBfr);

                            close(fd);
                            fd = -ND_SOCK_ERR_TIMEOUT;

                            if (fallback_ipv4 && ai->ai_family == PF_INET6)
                                *fallback_ipv4 = true;
                            break;

                        default:
                        case  2: // poll error
                            nd_log(NDLS_DAEMON, NDLP_ERR,
                                   "Failed to connect to '%s', port '%s'.",
                                   hostBfr, servBfr);

                            close(fd);
                            fd = -ND_SOCK_ERR_POLL_ERROR;
                            break;
                    }
                }
                else {
                    nd_log(NDLS_DAEMON, NDLP_ERR,
                           "Failed to connect to '%s', port '%s'",
                           hostBfr, servBfr);

                    close(fd);
                    fd = -ND_SOCK_ERR_CONNECTION_REFUSED;
                }
            }
        }
        else {
            nd_log(NDLS_DAEMON, NDLP_ERR, "Failed to socket() to '%s', port '%s'", hostBfr, servBfr);
            fd = -ND_SOCK_ERR_FAILED_TO_CREATE_SOCKET;
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

    char *host = buffer, *service = default_service, *iface = "";
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
        iface = e;
        while(*e && *e != ':') e++;
    }

    if(*e == ':') {
        *e = '\0';
        e++;
        service = e;
    }

    if(!*host) {
        nd_log(NDLS_DAEMON, NDLP_ERR,
               "Definition '%s' does not specify a host.",
               definition);

        return -ND_SOCK_ERR_NO_HOST_IN_DEFINITION;
    }

    if(*iface) {
        scope_id = if_nametoindex(iface);
        if(!scope_id)
            nd_log(NDLS_DAEMON, NDLP_ERR,
                   "Cannot find a network interface named '%s'. Continuing with limiting the network interface",
                   iface);
    }

    if(!*service)
        service = default_service;


    return connect_to_this_ip46(protocol, socktype, host, scope_id, service, timeout, NULL);
}

void foreach_entry_in_connection_string(const char *destination, bool (*callback)(char *entry, void *data), void *data) {
    const char *s = destination;
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
