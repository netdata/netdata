// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef _GNU_SOURCE
#define _GNU_SOURCE // for POLLRDHUP
#endif

#ifndef __BSD_VISIBLE
#define __BSD_VISIBLE // for POLLRDHUP
#endif

#include "../libnetdata.h"

bool ip_to_hostname(const char *ip, char *dst, size_t dst_len) {
    if(!dst || !dst_len)
        return false;

    struct sockaddr_in sa;
    struct sockaddr_in6 sa6;
    struct sockaddr *sa_ptr;
    int sa_len;

    // Try to convert the IP address to sockaddr_in (IPv4)
    if (inet_pton(AF_INET, ip, &(sa.sin_addr)) == 1) {
        sa.sin_family = AF_INET;
        sa_ptr = (struct sockaddr *)&sa;
        sa_len = sizeof(sa);
    }
        // Try to convert the IP address to sockaddr_in6 (IPv6)
    else if (inet_pton(AF_INET6, ip, &(sa6.sin6_addr)) == 1) {
        sa6.sin6_family = AF_INET6;
        sa_ptr = (struct sockaddr *)&sa6;
        sa_len = sizeof(sa6);
    }

    else {
        dst[0] = '\0';
        return false;
    }

    // Perform the reverse lookup
    int res = getnameinfo(sa_ptr, sa_len, dst, dst_len, NULL, 0, NI_NAMEREQD);
    if(res != 0)
        return false;

    return true;
}

// --------------------------------------------------------------------------------------------------------------------
// various library calls

bool fd_is_socket(int fd) {
    int type;
    socklen_t len = sizeof(type);
    if (getsockopt(fd, SOL_SOCKET, SO_TYPE, &type, &len) == -1)
        return false;

    return true;
}

#if defined(POLLRDHUP) && 0 // ktsaou: disabled because the recv() method is faster (1 syscall vs multiple by poll())
bool is_socket_closed(int fd) {
    if(fd < 0)
        return true;

//    if(!fd_is_socket(fd)) {
//        //internal_error(true, "fd %d is not a socket", fd);
//        return false;
//    }

    short int errors = POLLERR | POLLHUP | POLLNVAL | POLLRDHUP;

    struct pollfd pfd = {
            .fd = fd,
            .events = POLLOUT | errors,
            .revents = 0,
    };

    if(poll(&pfd, 1, 0) == -1) {
        //internal_error(true, "poll() failed");
        return false;
    }

    return ((pfd.revents & errors) || !(pfd.revents & POLLOUT));
}
#else
bool is_socket_closed(int fd) {
    if(fd < 0)
        return true;

    char buffer;
    ssize_t result = recv(fd, &buffer, 1, MSG_PEEK | MSG_DONTWAIT);
    if (result == 0) {
        // Connection closed
        return true;
    }
    else if (result < 0) {
        if (errno == EAGAIN || errno == EWOULDBLOCK) {
            // No data available, but socket is still open
            return false;
        } else {
            // An error occurred
            return true;
        }
    }

    // Data is available, socket is open
    return false;
}
#endif

#if defined(OS_LINUX)
// Valid from: 4 KB to 64 MB (typical range)
// Default is usually: 128 KB to 256 KB
// Maximum is controlled by: /proc/sys/net/core/rmem_max and /proc/sys/net/core/wmem_max
// Interactive applications should use: 256 KB
// High-performance applications should use: 8 MB to 64 MB
#define LARGE_SOCK_SIZE (32 * 1024 * 1024)

#elif defined(OS_FREEBSD)
// Valid from: 4 KB to 16 MB (typical range)
// Default is usually: 64 KB to 256 KB
// Maximum is controlled by: kern.ipc.maxsockbuf
// Interactive applications should use: 128 KB to 256 KB
// High-performance applications should use: 2 MB to 16 MB
#define LARGE_SOCK_SIZE (8 * 1024 * 1024)

#elif defined(OS_MACOS)
// Valid from: 4 KB to 8 MB (typical range)
// Default is usually: 128 KB
// Maximum is controlled by: net.inet.tcp.sendspace and net.inet.tcp.recvspace
// Interactive applications should use: 128 KB
// High-performance applications should use: 1 MB to 8 MB
#define LARGE_SOCK_SIZE (4 * 1024 * 1024)

#elif defined(OS_WINDOWS)
// Valid from: 8 KB to 16 MB (typical range)
// Default is usually: 8 KB to 64 KB
// Maximum is controlled by: registry keys such as TcpWindowSize
// Interactive applications should use: 64 KB to 128 KB
// High-performance applications should use: 1 MB to 16 MB
#define LARGE_SOCK_SIZE (8 * 1024 * 1024)

#else
// Valid from: 4 KB to platform-dependent maximum
// Default is usually: 64 KB to 256 KB
// Interactive applications should use: 128 KB to 256 KB
// High-performance applications should use: 1 MB to platform-dependent maximum
#define LARGE_SOCK_SIZE (1 * 1024 * 1024)
#endif

// Returns -1 for errors, current buffer size if successful
int sock_enlarge_rcv_buf(int fd) {
    int ret = -1;
    int bs = LARGE_SOCK_SIZE;
    int current_bs = 0;
    socklen_t optlen = sizeof(current_bs);

    // Get the current receive buffer size
    if (getsockopt(fd, SOL_SOCKET, SO_RCVBUF, &current_bs, &optlen) == 0) {
        // Set the buffer size only if it's smaller than the desired size
        if (current_bs < bs) {
            if(setsockopt(fd, SOL_SOCKET, SO_RCVBUF, &bs, sizeof(bs)) != 0)
                return -1;

            // Re-check the buffer size after attempting to set it
            if (getsockopt(fd, SOL_SOCKET, SO_RCVBUF, &current_bs, &optlen) == 0)
                ret = current_bs;
        }
        else {
            // Current buffer size is already large enough
            ret = current_bs;
        }
    }

    return ret;
}

// Returns -1 for errors, current buffer size if successful
int sock_enlarge_snd_buf(int fd) {
    int ret = -1;
    int bs = LARGE_SOCK_SIZE;
    int current_bs = 0;
    socklen_t optlen = sizeof(current_bs);

    // Get the current send buffer size
    if (getsockopt(fd, SOL_SOCKET, SO_SNDBUF, &current_bs, &optlen) == 0) {
        // Set the buffer size only if it's smaller than the desired size
        if (current_bs < bs) {
            if(setsockopt(fd, SOL_SOCKET, SO_SNDBUF, &bs, sizeof(bs)) != 0)
                return -1;

            // Re-check the buffer size after attempting to set it
            if (getsockopt(fd, SOL_SOCKET, SO_SNDBUF, &current_bs, &optlen) == 0)
                ret = current_bs;
        }
        else {
            // Current buffer size is already large enough
            ret = current_bs;
        }
    }

    return ret;
}

// returns -1 for errors, 0 if cork is unset, 1 if cork is set
int sock_setcork(int fd __maybe_unused, bool cork __maybe_unused) {
    int rc = -1;

#ifdef TCP_CORK
    int tcp_cork = (cork) ? 1 : 0;
    socklen_t optlen = sizeof(tcp_cork);

    if(setsockopt(fd, IPPROTO_TCP, TCP_CORK, &tcp_cork, optlen) == 0) {
        // setting was successful, return the intended state
        rc = cork ? 1 : 0;
    }
    else if(getsockopt(fd, IPPROTO_TCP, TCP_CORK, &tcp_cork, &optlen) == 0) {
        // return the current state since retrieval is successful
        rc = tcp_cork ? 1 : 0;
    }
#endif

    return rc;
}

// Returns -1 for errors, 0 if O_NONBLOCK is unset, 1 if O_NONBLOCK is set
int sock_setnonblock(int fd, bool nonblock) {
    int rc = -1;
    int flags = fcntl(fd, F_GETFL);

    if (flags < 0) {
        // Failed to get current flags
        return -1;
    }

    int new_flags = nonblock ? (flags | O_NONBLOCK) : (flags & ~O_NONBLOCK);

    if (fcntl(fd, F_SETFL, new_flags) == 0) {
        // Setting was successful, return the intended state
        rc = nonblock ? 1 : 0;
    } else {
        // If setting failed, return the current state
        flags = fcntl(fd, F_GETFL);
        if (flags >= 0)
            rc = (flags & O_NONBLOCK) ? 1 : 0;
    }

    return rc;
}

// Returns -1 for errors, 0 if SO_REUSEADDR is unset, 1 if SO_REUSEADDR is set
int sock_setreuse_addr(int fd, bool reuse) {
    int rc = -1;
    int reuse_val = reuse ? 1 : 0;
    socklen_t optlen = sizeof(reuse_val);

    // Attempt to set SO_REUSEADDR
    if (setsockopt(fd, SOL_SOCKET, SO_REUSEADDR, &reuse_val, optlen) == 0) {
        // Setting was successful, return the intended state
        rc = reuse ? 1 : 0;
    } else {
        // If setting failed, attempt to retrieve the current state
        if (getsockopt(fd, SOL_SOCKET, SO_REUSEADDR, &reuse_val, &optlen) == 0) {
            // Return the current state
            rc = reuse_val ? 1 : 0;
        }
    }

    return rc;
}

// Returns -1 for errors, 0 if SO_REUSEPORT is unset, 1 if SO_REUSEPORT is set
int sock_setreuse_port(int fd __maybe_unused, bool reuse __maybe_unused) {
    int rc = -1;

#ifdef SO_REUSEPORT
    int reuse_val = reuse ? 1 : 0;
    socklen_t optlen = sizeof(reuse_val);

    // Attempt to set SO_REUSEPORT
    if (setsockopt(fd, SOL_SOCKET, SO_REUSEPORT, &reuse_val, optlen) == 0) {
        // Setting was successful, return the intended state
        rc = reuse ? 1 : 0;
    } else if (errno != ENOPROTOOPT) {
        // If setting failed for a reason other than unsupported option, check the current state
        if (getsockopt(fd, SOL_SOCKET, SO_REUSEPORT, &reuse_val, &optlen) == 0) {
            // Return the current state
            rc = reuse_val ? 1 : 0;
        }
    }
#else
    // SO_REUSEPORT is not supported
    errno = ENOPROTOOPT;
#endif

    return rc;
}

// Returns -1 for errors, 0 if FD_CLOEXEC is unset, 1 if FD_CLOEXEC is set
int sock_setcloexec(int fd, bool cloexec) {
    int rc = -1;

    // Get current file descriptor flags
    int flags = fcntl(fd, F_GETFD);
    if (flags == -1)
        return -1; // Error retrieving flags

    int new_flags = cloexec ? (flags | FD_CLOEXEC) : (flags & ~FD_CLOEXEC);

    // Set the FD_CLOEXEC flag as requested
    if (fcntl(fd, F_SETFD, new_flags) == 0) {
        // Setting was successful, return the intended state
        rc = cloexec ? 1 : 0;
    } else {
        // If setting failed, return the current state
        flags = fcntl(fd, F_GETFD);
        if (flags != -1) {
            rc = (flags & FD_CLOEXEC) ? 1 : 0;
        }
    }

    return rc;
}

// Returns -1 for errors, 0 if TCP_DEFER_ACCEPT is unset, 1 if TCP_DEFER_ACCEPT is set
int sock_set_tcp_defer_accept(int fd __maybe_unused, bool defer __maybe_unused) {
#ifdef TCP_DEFER_ACCEPT
    // Check if the file descriptor is a socket
    if (!fd_is_socket(fd))
        return 0; // Not a socket

    int rc = -1;
    int timeout = defer ? 5 : 0; // Set timeout to 5 seconds for enabling, 0 to disable
    socklen_t optlen = sizeof(timeout);

    // Attempt to set TCP_DEFER_ACCEPT
    if (setsockopt(fd, IPPROTO_TCP, TCP_DEFER_ACCEPT, &timeout, optlen) == 0) {
        // Setting was successful, return the intended state
        rc = defer ? 1 : 0;
    } else if (errno != EINVAL && errno != ENOPROTOOPT) {
        // If setting failed and it's not because of invalid option or unsupported protocol
        // Check the current state
        if (getsockopt(fd, IPPROTO_TCP, TCP_DEFER_ACCEPT, &timeout, &optlen) == 0) {
            rc = timeout > 0 ? 1 : 0;
        }
    }

    return rc;
#else
    // TCP_DEFER_ACCEPT not supported
    errno = ENOPROTOOPT;
    return -1;
#endif
}

// --------------------------------------------------------------------------------------------------------------------
// helpers to send/receive data in one call, in blocking mode, with a timeout

// returns: -1 = thread cancelled, 0 = proceed to read/write, 1 = time exceeded, 2 = error on fd
// timeout parameter can be zero to wait forever
inline int wait_on_socket_or_cancel_with_timeout(
    NETDATA_SSL *ssl,
    int fd, int timeout_ms, short int poll_events, short int *revents) {
    struct pollfd pfd = {
        .fd = fd,
        .events = poll_events,
        .revents = 0,
    };

    bool forever = (timeout_ms <= 0);

    while (timeout_ms > 0 || forever) {
        if(nd_thread_signaled_to_cancel()) {
            errno = ECANCELED;
            return -1;
        }

        if(poll_events == POLLIN && ssl && SSL_connection(ssl) && netdata_ssl_has_pending(ssl))
            return 0;

        const int wait_ms = (timeout_ms >= ND_CHECK_CANCELLABILITY_WHILE_WAITING_EVERY_MS || forever) ?
                                            ND_CHECK_CANCELLABILITY_WHILE_WAITING_EVERY_MS : timeout_ms;

        errno_clear();

        // check every wait_ms
        const int ret = poll(&pfd, 1, wait_ms);

        if(revents)
            *revents = pfd.revents;

        if(ret == -1) {
            // poll failed

            if(errno == EINTR || errno == EAGAIN)
                continue;

            return 2;
        }

        if(ret == 0) {
            // timeout
            if(!forever)
                timeout_ms -= wait_ms;
            continue;
        }

        if(pfd.revents & poll_events)
            return 0;

        // all other errors
        return 2;
    }

    errno = ETIMEDOUT;
    return 1;
}

ssize_t send_timeout(NETDATA_SSL *ssl, int sockfd, void *buf, size_t len, int flags, time_t timeout) {

    switch(wait_on_socket_or_cancel_with_timeout(ssl, sockfd, timeout * 1000, POLLOUT, NULL)) {
        case 0: // data are waiting
            break;

        case 1: // timeout
            return 0;

        default:
        case -1: // thread cancelled
        case 2:  // error on socket
            return -1;
    }

    if(ssl->conn) {
        if (SSL_connection(ssl))
            return netdata_ssl_write(ssl, buf, len);

        else {
            nd_log(NDLS_DAEMON, NDLP_ERR,
                   "cannot write to SSL connection - connection is not ready.");

            return -1;
        }
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
int connection_allowed(int fd, char *client_ip, char *client_host, size_t hostsize, SIMPLE_PATTERN *access_list,
                       const char *patname, int allow_dns)
{
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

            nd_log(NDLS_DAEMON, NDLP_ERR,
                   "Incoming %s on '%s' does not match a numeric pattern, and host could not be resolved (err=%s)",
                   patname, client_ip, gai_strerror(err));

            if (hostsize >= 8)
                strcpy(client_host,"UNKNOWN");
            return 0;
        }
        struct addrinfo *addr_infos = NULL;
        if (getaddrinfo(client_host, NULL, NULL, &addr_infos) !=0 ) {
            nd_log(NDLS_DAEMON, NDLP_ERR,
                   "LISTENER: cannot validate hostname '%s' from '%s' by resolving it",
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
            if (!strcmp(client_ip, address)) {
                validated = 1;
                break;
            }
            scan = scan->ai_next;
        }
        if (!validated) {
            nd_log(NDLS_DAEMON, NDLP_ERR,
                   "LISTENER: Cannot validate '%s' as ip of '%s', not listed in DNS",
                   client_ip, client_host);

            if (hostsize >= 8)
                strcpy(client_host,"UNKNOWN");
        }
        if (addr_infos!=NULL)
            freeaddrinfo(addr_infos);
    }
    if (!simple_pattern_matches(access_list, client_host))
        return 0;

    return 1;
}

// --------------------------------------------------------------------------------------------------------------------
// accept_socket() - accept a socket and store client IP and port
int accept_socket(int fd, int flags, char *client_ip, size_t ipsize, char *client_port, size_t portsize,
                  char *client_host, size_t hostsize, SIMPLE_PATTERN *access_list, int allow_dns) {
    struct sockaddr_storage sadr;
    socklen_t addrlen = sizeof(sadr);

    int nfd = accept4(fd, (struct sockaddr *)&sadr, &addrlen, flags | DEFAULT_SOCKET_FLAGS);
    if (likely(nfd >= 0)) {
        if (getnameinfo((struct sockaddr *)&sadr, addrlen, client_ip, (socklen_t)ipsize,
                        client_port, (socklen_t)portsize, NI_NUMERICHOST | NI_NUMERICSERV) != 0) {

            nd_log(NDLS_DAEMON, NDLP_ERR,
                   "LISTENER: cannot getnameinfo() on received client connection.");

            strncpyz(client_ip, "UNKNOWN", ipsize);
            strncpyz(client_port, "UNKNOWN", portsize);
        }
        if (!strcmp(client_ip, "127.0.0.1") || !strcmp(client_ip, "::1")) {
            strncpyz(client_ip, "localhost", ipsize);
        }
        sock_setcloexec(nfd, true);

#ifdef __FreeBSD__
        if(((struct sockaddr *)&sadr)->sa_family == AF_LOCAL)
            strncpyz(client_ip, "localhost", ipsize);
#endif

        client_ip[ipsize - 1] = '\0';
        client_port[portsize - 1] = '\0';

        switch (((struct sockaddr *)&sadr)->sa_family) {
            case AF_UNIX:
                // netdata_log_debug(D_LISTENER, "New UNIX domain web client from %s on socket %d.", client_ip, fd);
                // set the port - certain versions of libc return garbage on unix sockets
                strncpyz(client_port, "UNIX", portsize);
                break;

            case AF_INET:
                // netdata_log_debug(D_LISTENER, "New IPv4 web client from %s port %s on socket %d.", client_ip, client_port, fd);
                break;

            case AF_INET6:
                if (strncmp(client_ip, "::ffff:", 7) == 0) {
                    memmove(client_ip, &client_ip[7], strlen(&client_ip[7]) + 1);
                    // netdata_log_debug(D_LISTENER, "New IPv4 web client from %s port %s on socket %d.", client_ip, client_port, fd);
                }
                // else
                //    netdata_log_debug(D_LISTENER, "New IPv6 web client from %s port %s on socket %d.", client_ip, client_port, fd);
                break;

            default:
                // netdata_log_debug(D_LISTENER, "New UNKNOWN web client from %s port %s on socket %d.", client_ip, client_port, fd);
                break;
        }
        if (!connection_allowed(nfd, client_ip, client_host, hostsize, access_list, "connection", allow_dns)) {
            errno_clear();
            nd_log(NDLS_DAEMON, NDLP_WARNING,
                   "Permission denied for client '%s', port '%s'",
                   client_ip, client_port);

            close(nfd);
            nfd = -1;
            errno = EPERM;
        }
    }
#ifdef HAVE_ACCEPT4
    else if (errno == ENOSYS)
        nd_log(NDLS_DAEMON, NDLP_ERR,
               "Netdata has been compiled with the assumption that the system has the accept4() call, but it is not here. "
               "Recompile netdata like this: ./configure --disable-accept4 ...");
#endif

    return nfd;
}
