#include "common.h"

int listen_backlog = LISTEN_BACKLOG;
size_t listen_fds_count = 0;
int listen_fds[MAX_LISTEN_FDS] = { [0 ... 99] = -1 };
char *listen_fds_names[MAX_LISTEN_FDS] = { [0 ... 99] = NULL };
int listen_port = LISTEN_PORT;
int web_server_mode = WEB_SERVER_MODE_MULTI_THREADED;

static int shown_server_socket_error = 0;

#ifdef NETDATA_INTERNAL_CHECKS
static void log_allocations(void)
{
#ifdef HAVE_C_MALLINFO
    static int heap = 0, used = 0, mmap = 0;

    struct mallinfo mi;

    mi = mallinfo();
    if(mi.uordblks > used) {
        int clients = 0;
        struct web_client *w;
        for(w = web_clients; w ; w = w->next) clients++;

        info("Allocated memory: used %d KB (+%d B), mmap %d KB (+%d B), heap %d KB (+%d B). %d web clients connected.",
            mi.uordblks / 1024,
            mi.uordblks - used,
            mi.hblkhd / 1024,
            mi.hblkhd - mmap,
            mi.arena / 1024,
            mi.arena - heap,
            clients);

        used = mi.uordblks;
        heap = mi.arena;
        mmap = mi.hblkhd;
    }
#else /* ! HAVE_C_MALLINFO */
    ;
#endif /* ! HAVE_C_MALLINFO */

#ifdef has_jemalloc
    malloc_stats_print(NULL, NULL, NULL);
#endif
}
#endif /* NETDATA_INTERNAL_CHECKS */

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

int create_listen_socket4(const char *ip, int port, int listen_backlog) {
    int sock;
    int sockopt = 1;

    debug(D_LISTENER, "IPv4 creating new listening socket on ip '%s' port %d", ip, port);

    sock = socket(AF_INET, SOCK_STREAM, 0);
    if(sock < 0) {
        error("IPv4 socket() on ip '%s' port %d failed.", ip, port);
        shown_server_socket_error = 1;
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
        shown_server_socket_error = 1;
        close(sock);
        return -1;
    }

    if(bind (sock, (struct sockaddr *) &name, sizeof (name)) < 0) {
        close(sock);
        error("IPv4 bind() on ip '%s' port %d failed.", ip, port);
        shown_server_socket_error = 1;
        return -1;
    }

    if(listen(sock, listen_backlog) < 0) {
        close(sock);
        error("IPv4 listen() on ip '%s' port %d failed.", ip, port);
        shown_server_socket_error = 1;
        return -1;
    }

    debug(D_LISTENER, "Listening on IPv4 ip '%s' port %d", ip, port);
    return sock;
}

int create_listen_socket6(const char *ip, int port, int listen_backlog) {
    int sock = -1;
    int sockopt = 1;
    int ipv6only = 1;

    debug(D_LISTENER, "IPv6 creating new listening socket on ip '%s' port %d", ip, port);

    sock = socket(AF_INET6, SOCK_STREAM, 0);
    if (sock < 0) {
        error("IPv6 socket() on ip '%s' port %d failed.", ip, port);
        shown_server_socket_error = 1;
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

    int ret = inet_pton(AF_INET6, ip, (void *)&name.sin6_addr.s6_addr);
    if(ret != 1) {
        error("Failed to convert IP '%s' to a valid IPv6 address.", ip);
        shown_server_socket_error = 1;
        close(sock);
        return -1;
    }

    name.sin6_scope_id = 0;

    if (bind (sock, (struct sockaddr *) &name, sizeof (name)) < 0) {
        close(sock);
        error("IPv6 bind() on ip '%s' port %d failed.", ip, port);
        shown_server_socket_error = 1;
        return -1;
    }

    if (listen(sock, listen_backlog) < 0) {
        close(sock);
        error("IPv6 listen() on ip '%s' port %d failed.", ip, port);
        shown_server_socket_error = 1;
        return -1;
    }

    debug(D_LISTENER, "Listening on IPv6 ip '%s' port %d", ip, port);
    return sock;
}

static inline int add_listen_socket(int fd, const char *ip, int port) {
    if(listen_fds_count >= MAX_LISTEN_FDS) {
        error("Too many listening sockets. Failed to add listening socket at ip '%s' port %d", ip, port);
        shown_server_socket_error = 1;
        close(fd);
        return -1;
    }

    listen_fds[listen_fds_count] = fd;

    char buffer[100 + 1];
    snprintfz(buffer, 100, "[%s]:%d", ip, port);
    listen_fds_names[listen_fds_count] = strdupz(buffer);

    listen_fds_count++;
    return 0;
}

int is_listen_socket(int fd) {
    size_t i;
    for(i = 0; i < listen_fds_count ;i++)
        if(listen_fds[i] == fd) return 1;

    return 0;
}

static inline void close_listen_sockets(void) {
    size_t i;
    for(i = 0; i < listen_fds_count ;i++) {
        close(listen_fds[i]);
        listen_fds[i] = -1;

        freez(listen_fds_names[i]);
        listen_fds_names[i] = NULL;
    }

    listen_fds_count = 0;
}

static inline int bind_to_one(const char *definition, int default_port, int listen_backlog) {
    int added = 0;
    struct addrinfo hints;
    struct addrinfo *result = NULL, *rp = NULL;

    char buffer[strlen(definition) + 1];
    strcpy(buffer, definition);

    char buffer2[10 + 1];
    snprintfz(buffer2, 10, "%d", default_port);

    char *ip = buffer, *port = buffer2;

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
        while(*e && *e != ':') e++;
    }

    if(*e == ':') {
        port = e + 1;
        *e = '\0';
    }

    if(!*ip || *ip == '*' || !strcmp(ip, "any") || !strcmp(ip, "all"))
        ip = NULL;
    if(!*port)
        port = buffer2;

    memset(&hints, 0, sizeof(struct addrinfo));
    hints.ai_family = AF_UNSPEC;    /* Allow IPv4 or IPv6 */
    hints.ai_socktype = SOCK_DGRAM; /* Datagram socket */
    hints.ai_flags = AI_PASSIVE;    /* For wildcard IP address */
    hints.ai_protocol = 0;          /* Any protocol */
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
                fd = create_listen_socket4(rip, rport, listen_backlog);
                break;
            }

            case AF_INET6: {
                struct sockaddr_in6 *sin6 = (struct sockaddr_in6 *) rp->ai_addr;
                inet_ntop(AF_INET6, &sin6->sin6_addr, rip, INET6_ADDRSTRLEN);
                rport = ntohs(sin6->sin6_port);
                fd = create_listen_socket6(rip, rport, listen_backlog);
                break;
            }
        }

        if (fd == -1)
            error("Cannot bind to ip '%s', port %d", rip, rport);
        else {
            add_listen_socket(fd, rip, rport);
            added++;
        }
    }

    freeaddrinfo(result);

    return added;
}

int create_listen_sockets(void) {
    shown_server_socket_error = 0;

    listen_backlog = (int) config_get_number("global", "http port listen backlog", LISTEN_BACKLOG);

    if(config_exists("global", "bind socket to IP") && !config_exists("global", "bind to"))
        config_rename("global", "bind socket to IP", "bind to");

    if(config_exists("global", "port") && !config_exists("global", "default port"))
        config_rename("global", "port", "default port");

    listen_port = (int) config_get_number("global", "default port", LISTEN_PORT);
    if(listen_port < 1 || listen_port > 65535) {
        error("Invalid listen port %d given. Defaulting to %d.", listen_port, LISTEN_PORT);
        listen_port = (int) config_set_number("global", "default port", LISTEN_PORT);
    }
    debug(D_OPTIONS, "Default listen port set to %d.", listen_port);

    char *s = config_get("global", "bind to", "*");
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
        bind_to_one(buf, listen_port, listen_backlog);

        s = e;
    }

    if(!listen_fds_count)
        fatal("Cannot listen on any socket. Exiting...");
    else if(shown_server_socket_error) {
        size_t i;
        for(i = 0; i < listen_fds_count ;i++)
            info("Listen socket %s opened.", listen_fds_names[i]);
    }

    return (int)listen_fds_count;
}

// --------------------------------------------------------------------------------------
// the main socket listener

static inline void cleanup_web_clients(void) {
    struct web_client *w;

    for (w = web_clients; w;) {
        if (w->obsolete) {
            debug(D_WEB_CLIENT, "%llu: Removing client.", w->id);
            // pthread_cancel(w->thread);
            // pthread_join(w->thread, NULL);
            w = web_client_free(w);
#ifdef NETDATA_INTERNAL_CHECKS
            log_allocations();
#endif
        }
        else w = w->next;
    }
}

// 1. it accepts new incoming requests on our port
// 2. creates a new web_client for each connection received
// 3. spawns a new pthread to serve the client (this is optimal for keep-alive clients)
// 4. cleans up old web_clients that their pthreads have been exited

#define CLEANUP_EVERY_EVENTS 100

void *socket_listen_main_multi_threaded(void *ptr) {
    (void)ptr;

    web_server_mode = WEB_SERVER_MODE_MULTI_THREADED;
    info("Multi-threaded WEB SERVER thread created with task id %d", gettid());

    struct web_client *w;
    int retval, counter = 0;

    if(pthread_setcanceltype(PTHREAD_CANCEL_DEFERRED, NULL) != 0)
        error("Cannot set pthread cancel type to DEFERRED.");

    if(pthread_setcancelstate(PTHREAD_CANCEL_ENABLE, NULL) != 0)
        error("Cannot set pthread cancel state to ENABLE.");

    if(!listen_fds_count)
        fatal("LISTENER: No sockets to listen to.");

    struct pollfd *fds = callocz(sizeof(struct pollfd), listen_fds_count);

    size_t i;
    for(i = 0; i < listen_fds_count ;i++) {
        fds[i].fd = listen_fds[i];
        fds[i].events = POLLIN;
        fds[i].revents = 0;

        info("Listening on '%s'", (listen_fds_names[i])?listen_fds_names[i]:"UNKNOWN");
    }

    int timeout = 10 * 1000;

    for(;;) {
        // debug(D_WEB_CLIENT, "LISTENER: Waiting...");
        retval = poll(fds, listen_fds_count, timeout);

        if(unlikely(retval == -1)) {
            error("LISTENER: poll() failed.");
            continue;
        }
        else if(unlikely(!retval)) {
            debug(D_WEB_CLIENT, "LISTENER: select() timeout.");
            counter = 0;
            cleanup_web_clients();
            continue;
        }

        for(i = 0 ; i < listen_fds_count ; i++) {
            short int revents = fds[i].revents;

            // check for new incoming connections
            if(revents & POLLIN || revents & POLLPRI) {
                fds[i].revents = 0;

                w = web_client_create(fds[i].fd);
                if(unlikely(!w)) {
                    // no need for error log - web_client_create already logged the error
                    continue;
                }

                if(pthread_create(&w->thread, NULL, web_client_main, w) != 0) {
                    error("%llu: failed to create new thread for web client.", w->id);
                    w->obsolete = 1;
                }
                else if(pthread_detach(w->thread) != 0) {
                    error("%llu: Cannot request detach of newly created web client thread.", w->id);
                    w->obsolete = 1;
                }
            }
        }

        // cleanup unused clients
        counter++;
        if(counter >= CLEANUP_EVERY_EVENTS) {
            counter = 0;
            cleanup_web_clients();
        }
    }

    debug(D_WEB_CLIENT, "LISTENER: exit!");
    close_listen_sockets();

    return NULL;
}

struct web_client *single_threaded_clients[FD_SETSIZE];

static inline int single_threaded_link_client(struct web_client *w, fd_set *ifds, fd_set *ofds, fd_set *efds, int *max) {
    if(unlikely(w->obsolete || w->dead || (!w->wait_receive && !w->wait_send)))
        return 1;

    if(unlikely(w->ifd < 0 || w->ifd >= FD_SETSIZE || w->ofd < 0 || w->ofd >= FD_SETSIZE)) {
        error("%llu: invalid file descriptor, ifd = %d, ofd = %d (required 0 <= fd < FD_SETSIZE (%d)", w->id, w->ifd, w->ofd, FD_SETSIZE);
        return 1;
    }

    FD_SET(w->ifd, efds);
    if(unlikely(*max < w->ifd)) *max = w->ifd;

    if(unlikely(w->ifd != w->ofd)) {
        if(*max < w->ofd) *max = w->ofd;
        FD_SET(w->ofd, efds);
    }

    if(w->wait_receive) FD_SET(w->ifd, ifds);
    if(w->wait_send)    FD_SET(w->ofd, ofds);

    single_threaded_clients[w->ifd] = w;
    single_threaded_clients[w->ofd] = w;

    return 0;
}

static inline int single_threaded_unlink_client(struct web_client *w, fd_set *ifds, fd_set *ofds, fd_set *efds) {
    FD_CLR(w->ifd, efds);
    if(unlikely(w->ifd != w->ofd)) FD_CLR(w->ofd, efds);

    if(w->wait_receive) FD_CLR(w->ifd, ifds);
    if(w->wait_send)    FD_CLR(w->ofd, ofds);

    single_threaded_clients[w->ifd] = NULL;
    single_threaded_clients[w->ofd] = NULL;

    if(unlikely(w->obsolete || w->dead || (!w->wait_receive && !w->wait_send)))
        return 1;

    return 0;
}

void *socket_listen_main_single_threaded(void *ptr) {
    (void)ptr;

    web_server_mode = WEB_SERVER_MODE_SINGLE_THREADED;

    info("Single-threaded WEB SERVER thread created with task id %d", gettid());

    struct web_client *w;
    int retval;

    if(pthread_setcanceltype(PTHREAD_CANCEL_DEFERRED, NULL) != 0)
        error("Cannot set pthread cancel type to DEFERRED.");

    if(pthread_setcancelstate(PTHREAD_CANCEL_ENABLE, NULL) != 0)
        error("Cannot set pthread cancel state to ENABLE.");

    if(!listen_fds_count)
        fatal("LISTENER: no listen sockets available.");

    size_t i;
    for(i = 0; i < FD_SETSIZE ; i++)
        single_threaded_clients[i] = NULL;

    fd_set ifds, ofds, efds, rifds, rofds, refds;
    FD_ZERO (&ifds);
    FD_ZERO (&ofds);
    FD_ZERO (&efds);
    int fdmax = 0;

    for(i = 0; i < listen_fds_count ; i++) {
        if (listen_fds[i] < 0 || listen_fds[i] >= FD_SETSIZE)
            fatal("LISTENER: Listen socket %d is not ready, or invalid.", listen_fds[i]);

        info("Listening on '%s'", (listen_fds_names[i])?listen_fds_names[i]:"UNKNOWN");

        FD_SET(listen_fds[i], &ifds);
        FD_SET(listen_fds[i], &efds);
        if(fdmax < listen_fds[i])
            fdmax = listen_fds[i];
    }

    for(;;) {
        debug(D_WEB_CLIENT_ACCESS, "LISTENER: single threaded web server waiting (fdmax = %d)...", fdmax);

        struct timeval tv = { .tv_sec = 1, .tv_usec = 0 };
        rifds = ifds;
        rofds = ofds;
        refds = efds;
        retval = select(fdmax+1, &rifds, &rofds, &refds, &tv);

        if(unlikely(retval == -1)) {
            error("LISTENER: select() failed.");
            continue;
        }
        else if(likely(retval)) {
            debug(D_WEB_CLIENT_ACCESS, "LISTENER: got something.");

            for(i = 0; i < listen_fds_count ; i++) {
                if (FD_ISSET(listen_fds[i], &rifds)) {
                    debug(D_WEB_CLIENT_ACCESS, "LISTENER: new connection.");
                    w = web_client_create(listen_fds[i]);
                    if (single_threaded_link_client(w, &ifds, &ofds, &ifds, &fdmax) != 0) {
                        web_client_free(w);
                    }
                }
            }

            for(i = 0 ; i <= (size_t)fdmax ; i++) {
                if(likely(!FD_ISSET(i, &rifds) && !FD_ISSET(i, &rofds) && !FD_ISSET(i, &refds)))
                    continue;

                w = single_threaded_clients[i];
                if(unlikely(!w))
                    continue;

                if(unlikely(single_threaded_unlink_client(w, &ifds, &ofds, &efds) != 0)) {
                    web_client_free(w);
                    continue;
                }

                if (unlikely(FD_ISSET(w->ifd, &refds) || FD_ISSET(w->ofd, &refds))) {
                    web_client_free(w);
                    continue;
                }

                if (unlikely(w->wait_receive && FD_ISSET(w->ifd, &rifds))) {
                    if (unlikely(web_client_receive(w) < 0)) {
                        web_client_free(w);
                        continue;
                    }

                    if (w->mode != WEB_CLIENT_MODE_FILECOPY) {
                        debug(D_WEB_CLIENT, "%llu: Processing received data.", w->id);
                        web_client_process(w);
                    }
                }

                if (unlikely(w->wait_send && FD_ISSET(w->ofd, &rofds))) {
                    if (unlikely(web_client_send(w) < 0)) {
                        debug(D_WEB_CLIENT, "%llu: Cannot send data to client. Closing client.", w->id);
                        web_client_free(w);
                        continue;
                    }
                }

                if(unlikely(single_threaded_link_client(w, &ifds, &ofds, &efds, &fdmax) != 0)) {
                    web_client_free(w);
                }
            }
        }
        else {
            debug(D_WEB_CLIENT_ACCESS, "LISTENER: single threaded web server timeout.");
#ifdef NETDATA_INTERNAL_CHECKS
            log_allocations();
#endif
        }
    }

    debug(D_WEB_CLIENT, "LISTENER: exit!");
    close_listen_sockets();
    return NULL;
}
