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

static inline int timeval_to_poll_timeout_ms(const struct timeval *timeout) {
    if(!timeout)
        return 1000;

    if(timeout->tv_sec <= 0 && timeout->tv_usec <= 0)
        return 0;

    uint64_t timeout_ms = 0;

    if(timeout->tv_sec > 0) {
        if((uint64_t)timeout->tv_sec > (uint64_t)INT_MAX / MSEC_PER_SEC)
            return INT_MAX;

        timeout_ms = (uint64_t)timeout->tv_sec * MSEC_PER_SEC;
    }

    if(timeout->tv_usec > 0)
        timeout_ms += (uint64_t)timeout->tv_usec / USEC_PER_MS;

    if(timeout_ms > INT_MAX)
        return INT_MAX;

    // a positive timeout below 1ms must not become 0, which means "wait forever"
    if(!timeout_ms)
        return 1;

    return (int)timeout_ms;
}

// --------------------------------------------------------------------------------------------------------------------
// connecting to the resolved addresses of a host
//
// The addresses are tried in parallel, staggered (in the spirit of RFC 8305 "Happy Eyeballs"): the next address is
// started when the earlier ones have not connected within CONNECT_ATTEMPT_DELAY_MS, or right away when one fails,
// while the earlier attempts stay pending. The first one to connect wins and the rest are closed.

#define CONNECT_ATTEMPT_DELAY_MS 250

typedef enum {
    CONNECT_ATTEMPT_FAILED,
    CONNECT_ATTEMPT_PENDING,
    CONNECT_ATTEMPT_CONNECTED,
} CONNECT_ATTEMPT_STATE;

typedef struct connect_attempt {
    const struct addrinfo *ai;
    int fd;
    char ip[NI_MAXHOST + 1];
    char port[NI_MAXSERV + 1];
} CONNECT_ATTEMPT;

static void connect_attempt_close(CONNECT_ATTEMPT *a) {
    if(a->fd != -1) {
        close(a->fd);
        a->fd = -1;
    }
}

// start a non-blocking connect() to the address of the attempt
static CONNECT_ATTEMPT_STATE connect_attempt_start(CONNECT_ATTEMPT *a, struct timeval *timeout, ND_SOCK_ERROR *err) {
    ND_LOG_STACK lgs[] = {
        ND_LOG_FIELD_TXT(NDF_DST_IP, a->ip),
        ND_LOG_FIELD_TXT(NDF_DST_PORT, a->port),
        ND_LOG_FIELD_END(),
    };
    ND_LOG_STACK_PUSH(lgs);

    a->fd = socket(a->ai->ai_family, a->ai->ai_socktype | DEFAULT_SOCKET_FLAGS, a->ai->ai_protocol);
    if(a->fd == -1) {
        nd_log(NDLS_DAEMON, NDLP_ERR, "Failed to socket() to '%s', port '%s'", a->ip, a->port);
        *err = ND_SOCK_ERR_FAILED_TO_CREATE_SOCKET;
        return CONNECT_ATTEMPT_FAILED;
    }

    if(timeout) {
        if(setsockopt(a->fd, SOL_SOCKET, SO_SNDTIMEO, (char *) timeout, sizeof(struct timeval)) < 0)
            nd_log(NDLS_DAEMON, NDLP_ERR,
                   "Failed to set timeout on the socket to ip '%s' port '%s'",
                   a->ip, a->port);
    }
    sock_setcloexec(a->fd, true);

    if(sock_setnonblock(a->fd, true) != 1) {
        nd_log(NDLS_DAEMON, NDLP_ERR,
               "Failed to set the socket to ip '%s' port '%s' to non-blocking mode",
               a->ip, a->port);

        connect_attempt_close(a);
        *err = ND_SOCK_ERR_FAILED_TO_CREATE_SOCKET;
        return CONNECT_ATTEMPT_FAILED;
    }

    errno_clear();
    if(connect(a->fd, a->ai->ai_addr, a->ai->ai_addrlen) == 0)
        return CONNECT_ATTEMPT_CONNECTED;

    if(errno == EINPROGRESS || errno == EALREADY || errno == EINTR) {
        nd_log(NDLS_DAEMON, NDLP_DEBUG,
               "Waiting for connection to ip %s port %s to be established",
               a->ip, a->port);

        return CONNECT_ATTEMPT_PENDING;
    }

    nd_log(NDLS_DAEMON, NDLP_ERR,
           "Failed to connect to '%s', port '%s': %s",
           a->ip, a->port, strerror(errno));

    connect_attempt_close(a);
    *err = ND_SOCK_ERR_CONNECTION_REFUSED;
    return CONNECT_ATTEMPT_FAILED;
}

// the socket of a pending attempt reported an event - SO_ERROR says if the connection was established
static CONNECT_ATTEMPT_STATE connect_attempt_finish(CONNECT_ATTEMPT *a, short revents, ND_SOCK_ERROR *err) {
    ND_LOG_STACK lgs[] = {
        ND_LOG_FIELD_TXT(NDF_DST_IP, a->ip),
        ND_LOG_FIELD_TXT(NDF_DST_PORT, a->port),
        ND_LOG_FIELD_END(),
    };
    ND_LOG_STACK_PUSH(lgs);

    int so_error = 0;
    socklen_t so_error_len = sizeof(so_error);
    if(getsockopt(a->fd, SOL_SOCKET, SO_ERROR, (void *)&so_error, &so_error_len) != 0) {
        nd_log(NDLS_DAEMON, NDLP_ERR,
               "Failed to get the connection status of the socket to '%s', port '%s': %s",
               a->ip, a->port, strerror(errno));

        connect_attempt_close(a);
        *err = ND_SOCK_ERR_POLL_ERROR;
        return CONNECT_ATTEMPT_FAILED;
    }

    if(so_error) {
        nd_log(NDLS_DAEMON, NDLP_ERR,
               "Failed to connect to '%s', port '%s': %s",
               a->ip, a->port, strerror(so_error));

        connect_attempt_close(a);
        *err = ND_SOCK_ERR_CONNECTION_REFUSED;
        return CONNECT_ATTEMPT_FAILED;
    }

    if(!(revents & POLLOUT)) {
        nd_log(NDLS_DAEMON, NDLP_ERR,
               "Failed to connect to '%s', port '%s'.",
               a->ip, a->port);

        connect_attempt_close(a);
        *err = ND_SOCK_ERR_POLL_ERROR;
        return CONNECT_ATTEMPT_FAILED;
    }

    nd_log(NDLS_DAEMON, NDLP_DEBUG,
           "connect() to ip %s port %s completed successfully",
           a->ip, a->port);

    return CONNECT_ATTEMPT_CONNECTED;
}

static void connect_attempt_timed_out(CONNECT_ATTEMPT *a) {
    ND_LOG_STACK lgs[] = {
        ND_LOG_FIELD_TXT(NDF_DST_IP, a->ip),
        ND_LOG_FIELD_TXT(NDF_DST_PORT, a->port),
        ND_LOG_FIELD_END(),
    };
    ND_LOG_STACK_PUSH(lgs);

    nd_log(NDLS_DAEMON, NDLP_ERR,
           "Timed out while connecting to '%s', port '%s'.",
           a->ip, a->port);

    connect_attempt_close(a);
}

// connect_to_this_ip46()
// protocol    IPPROTO_TCP, IPPROTO_UDP
// socktype    SOCK_STREAM, SOCK_DGRAM
// host        the destination hostname or IP address (IPv4 or IPv6) to connect to
//             if it resolves to many IPs, all are tried (IPv4 and IPv6, interleaved, starting with the family
//             getaddrinfo() returns first), staggered and in parallel, until one connects
// scope_id    the if_index id of the interface to use for connecting (0 = any)
//             (used only under IPv6)
// service     the service name or port to connect to
// timeout     the time allowed for connecting to the host, over all its IPs; it is also set as SO_SNDTIMEO
//             on the socket (NULL = 1 second and no SO_SNDTIMEO, zero = no limit: the IPs wait until the kernel
//             gives up)
// fallback_ipv4  when not NULL, set to true when an IPv6 address times out, and while true IPv6 addresses are skipped
//             (so it affects the calls that follow, not the one that set it)
//
// returns the connected socket (blocking), or -ND_SOCK_ERROR of the last failure

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

    // collect the addresses to try, per address family, in the order getaddrinfo() returned them
    size_t total = 0;
    for (ai = ai_head; ai != NULL; ai = ai->ai_next)
        total++;

    struct addrinfo **v4 = callocz(total, sizeof(*v4));
    struct addrinfo **v6 = callocz(total, sizeof(*v6));
    size_t v4_count = 0, v6_count = 0;
    int first_family = 0;

    for (ai = ai_head; ai != NULL; ai = ai->ai_next) {
        if (ai->ai_family == PF_INET6) {
            if (fallback_ipv4 && *fallback_ipv4)
                continue;

            struct sockaddr_in6 *pSadrIn6 = (struct sockaddr_in6 *) ai->ai_addr;
            if(pSadrIn6->sin6_scope_id == 0) {
                pSadrIn6->sin6_scope_id = scope_id;
            }

            v6[v6_count++] = ai;
        }
        else if (ai->ai_family == PF_INET)
            v4[v4_count++] = ai;
        else
            continue;

        if(!first_family)
            first_family = ai->ai_family;
    }

    // interleave the address families, starting with the first one getaddrinfo() returned
    size_t count = v4_count + v6_count;
    CONNECT_ATTEMPT *attempts = callocz(count ? count : 1, sizeof(*attempts));
    struct pollfd *pfds = callocz(count ? count : 1, sizeof(*pfds));
    size_t *pfds_attempt = callocz(count ? count : 1, sizeof(*pfds_attempt));

    for (size_t i = 0, i4 = 0, i6 = 0; i < count; i++) {
        bool want_v6 = (i % 2 == 0) == (first_family == PF_INET6);
        if ((want_v6 && i6 < v6_count) || i4 >= v4_count)
            attempts[i].ai = v6[i6++];
        else
            attempts[i].ai = v4[i4++];

        attempts[i].fd = -1;
        getnameinfo(attempts[i].ai->ai_addr,
                    attempts[i].ai->ai_addrlen,
                    attempts[i].ip,
                    sizeof(attempts[i].ip),
                    attempts[i].port,
                    sizeof(attempts[i].port),
                    NI_NUMERICHOST | NI_NUMERICSERV);
    }

    freez(v4);
    freez(v6);

    const int timeout_ms = timeval_to_poll_timeout_ms(timeout);
    usec_t now_ut = now_monotonic_usec();
    const usec_t deadline_ut = timeout_ms ? now_ut + (usec_t)timeout_ms * USEC_PER_MS : 0;
    usec_t next_start_ut = now_ut;

    int fd = -1;
    ND_SOCK_ERROR err = ND_SOCK_ERR_CONNECTION_REFUSED;
    size_t started = 0, pending = 0;

    while (fd == -1) {
        if(nd_thread_signaled_to_cancel()) {
            if(started)
                nd_log(NDLS_DAEMON, NDLP_ERR,
                       "Thread is cancelled while connecting to '%s', port '%s'.",
                       host, service);

            err = ND_SOCK_ERR_THREAD_CANCELLED;
            break;
        }

        now_ut = now_monotonic_usec();

        if(deadline_ut && now_ut >= deadline_ut) {
            for (size_t i = 0; i < started; i++) {
                if(attempts[i].fd == -1)
                    continue;

                connect_attempt_timed_out(&attempts[i]);
                err = ND_SOCK_ERR_TIMEOUT;

                if (fallback_ipv4 && attempts[i].ai->ai_family == PF_INET6)
                    *fallback_ipv4 = true;
            }

            if(started < count)
                nd_log(NDLS_DAEMON, NDLP_ERR,
                       "Not trying %zu more ips of host '%s', port '%s': the %d ms allowed for connecting have passed",
                       count - started, host, service, timeout_ms);
            break;
        }

        if(!pending && started >= count)
            break; // all the addresses failed

        // start the next address, when the pending ones are slow or none is pending
        if(started < count && (!pending || now_ut >= next_start_ut)) {
            CONNECT_ATTEMPT *a = &attempts[started++];
            switch(connect_attempt_start(a, timeout, &err)) {
                case CONNECT_ATTEMPT_CONNECTED:
                    fd = a->fd;
                    a->fd = -1;
                    break;

                case CONNECT_ATTEMPT_PENDING:
                    pending++;
                    next_start_ut = now_ut + CONNECT_ATTEMPT_DELAY_MS * USEC_PER_MS;
                    break;

                case CONNECT_ATTEMPT_FAILED:
                    break;
            }
            continue;
        }

        // wait for the pending ones, until the next address is due, the deadline, or the next cancellation check
        usec_t wait_ut = ND_CHECK_CANCELLABILITY_WHILE_WAITING_EVERY_MS * USEC_PER_MS;
        if(started < count && next_start_ut - now_ut < wait_ut)
            wait_ut = next_start_ut - now_ut;
        if(deadline_ut && deadline_ut - now_ut < wait_ut)
            wait_ut = deadline_ut - now_ut;

        size_t polled = 0;
        for (size_t i = 0; i < started; i++) {
            if(attempts[i].fd == -1)
                continue;

            pfds[polled].fd = attempts[i].fd;
            pfds[polled].events = POLLOUT;
            pfds[polled].revents = 0;
            pfds_attempt[polled] = i;
            polled++;
        }

        errno_clear();
        int ret = poll(pfds, polled, (int)((wait_ut + USEC_PER_MS - 1) / USEC_PER_MS));
        if(ret < 0) {
            if(errno == EINTR || errno == EAGAIN)
                continue;

            nd_log(NDLS_DAEMON, NDLP_ERR,
                   "Failed to poll() while connecting to '%s', port '%s'",
                   host, service);

            err = ND_SOCK_ERR_POLL_ERROR;
            break;
        }

        for (size_t i = 0; ret > 0 && i < polled && fd == -1; i++) {
            if(!pfds[i].revents)
                continue;

            CONNECT_ATTEMPT *a = &attempts[pfds_attempt[i]];
            pending--;

            if(connect_attempt_finish(a, pfds[i].revents, &err) == CONNECT_ATTEMPT_CONNECTED) {
                fd = a->fd;
                a->fd = -1;
            }
            else
                next_start_ut = now_ut; // a failure starts the next address right away
        }
    }

    // close the attempts that did not win
    for (size_t i = 0; i < started; i++)
        connect_attempt_close(&attempts[i]);

    if(fd != -1 && sock_setnonblock(fd, false) != 0) {
        nd_log(NDLS_DAEMON, NDLP_ERR,
               "Failed to restore blocking mode on the socket connected to '%s', port '%s'",
               host, service);

        close(fd);
        fd = -1;
        err = ND_SOCK_ERR_FAILED_TO_CREATE_SOCKET;
    }

    freez(pfds_attempt);
    freez(pfds);
    freez(attempts);
    freeaddrinfo(ai_head);

    return fd != -1 ? fd : -(int)err;
}

static int parse_connection_definition(
    char *definition,
    char **host,
    char **service,
    char **iface,
    int *protocol,
    int *socktype,
    char **unix_path)
{
    if(!definition || !*definition)
        return -1;

    *host = definition;
    *service = NULL;
    *iface = "";
    *protocol = IPPROTO_TCP;
    *socktype = SOCK_STREAM;
    *unix_path = NULL;

    if(strncmp(*host, "tcp:", 4) == 0) {
        *host += 4;
        *protocol = IPPROTO_TCP;
        *socktype = SOCK_STREAM;
    }
    else if(strncmp(*host, "udp:", 4) == 0) {
        *host += 4;
        *protocol = IPPROTO_UDP;
        *socktype = SOCK_DGRAM;
    }
    else if(strncmp(*host, "unix:", 5) == 0) {
        *unix_path = *host + 5;
        return 1;
    }
    else if(**host == '/') {
        *unix_path = *host;
        return 1;
    }

    char *e = *host;
    if(*e == '[') {
        e = ++(*host);
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
        *iface = e;
        while(*e && *e != ':') e++;
    }

    if(*e == ':') {
        *e = '\0';
        e++;
        *service = e;
    }

    if(!**host)
        return -1;

    return 0;
}

bool connect_to_definition_get_service(const char *definition, int default_port, char *service, size_t service_size) {
    if(!service || !service_size)
        return false;

    snprintfz(service, service_size, "%d", default_port);

    if(!definition || !*definition)
        return true;

    const char *s = definition;
    if(strncmp(s, "tcp:", 4) == 0 || strncmp(s, "udp:", 4) == 0)
        s += 4;
    else if(strncmp(s, "unix:", 5) == 0 || *s == '/')
        return true;

    const char *e = s;
    size_t host_len = 0;
    if(*e == '[') {
        const char *host_start = ++e;
        while(*e && *e != ']') e++;
        host_len = (size_t)(e - host_start);
        if(*e == ']')
            e++;
    }
    else {
        const char *host_start = e;
        while(*e && *e != ':' && *e != '%') e++;
        host_len = (size_t)(e - host_start);
    }

    if(!host_len)
        return false;

    if(*e == '%') {
        e++;
        while(*e && *e != ':') e++;
    }

    if(*e == ':' && *(e + 1))
        snprintfz(service, service_size, "%s", e + 1);

    return true;
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
    if(!definition || !*definition) {
        nd_log(NDLS_DAEMON, NDLP_ERR,
               "Definition '%s' does not specify a host.",
               definition ? definition : "(null)");

        return -ND_SOCK_ERR_NO_HOST_IN_DEFINITION;
    }

    CLEAN_CHAR_P *buffer = strdupz(definition);

    char default_service[10 + 1];
    snprintfz(default_service, 10, "%d", default_port);

    char *host = NULL, *service = NULL, *iface = NULL, *unix_path = NULL;
    int protocol = 0, socktype = 0;
    uint32_t scope_id = 0;

    int rc = parse_connection_definition(
        buffer, &host, &service, &iface, &protocol, &socktype, &unix_path);
    if(rc == 1)
        return connect_to_unix(unix_path, timeout);

    if(rc == -1) {
        nd_log(NDLS_DAEMON, NDLP_ERR,
               "Definition '%s' does not specify a host.",
               definition);

        return -ND_SOCK_ERR_NO_HOST_IN_DEFINITION;
    }

    if(iface && *iface) {
        scope_id = if_nametoindex(iface);
        if(!scope_id)
            nd_log(NDLS_DAEMON, NDLP_ERR,
                   "Cannot find a network interface named '%s'. Continuing without limiting the network interface",
                   iface);
    }

    if(!service || !*service)
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

        CLEAN_CHAR_P *buf = mallocz((size_t)(e - s) + 1);
        strncpyz(buf, s, (size_t)(e - s));

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
        (*t->reconnects_counter)++;

    t->sock = connect_to_this(entry, t->default_port, t->timeout);
    if(t->sock >= 0) {
        if(t->connected_to && t->connected_to_size) {
            strncpyz(t->connected_to, entry, t->connected_to_size - 1);
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
