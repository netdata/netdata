#include "common.h"

static LISTEN_SOCKETS api_sockets = {
        .config_section  = CONFIG_SECTION_WEB,
        .default_bind_to = "*",
        .default_port    = API_LISTEN_PORT,
        .backlog         = API_LISTEN_BACKLOG
};

WEB_SERVER_MODE web_server_mode = WEB_SERVER_MODE_STATIC_THREADED;

// --------------------------------------------------------------------------------------

#ifdef NETDATA_INTERNAL_CHECKS
static void log_allocations(void)
{
#ifdef HAVE_C_MALLINFO
    static int heap = 0, used = 0, mmap = 0;

    struct mallinfo mi;

    mi = mallinfo();
    if(mi.uordblks > used) {
        info("Allocated memory: used %d KB (+%d B), mmap %d KB (+%d B), heap %d KB (+%d B).",
            mi.uordblks / 1024,
            mi.uordblks - used,
            mi.hblkhd / 1024,
            mi.hblkhd - mmap,
            mi.arena / 1024,
            mi.arena - heap);

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

// --------------------------------------------------------------------------------------

WEB_SERVER_MODE web_server_mode_id(const char *mode) {
    if(!strcmp(mode, "none"))
        return WEB_SERVER_MODE_NONE;
    else if(!strcmp(mode, "single") || !strcmp(mode, "single-threaded"))
        return WEB_SERVER_MODE_SINGLE_THREADED;
    else if(!strcmp(mode, "static") || !strcmp(mode, "static-threaded"))
        return WEB_SERVER_MODE_STATIC_THREADED;
    else // if(!strcmp(mode, "multi") || !strcmp(mode, "multi-threaded"))
        return WEB_SERVER_MODE_MULTI_THREADED;
}

const char *web_server_mode_name(WEB_SERVER_MODE id) {
    switch(id) {
        case WEB_SERVER_MODE_NONE:
            return "none";

        case WEB_SERVER_MODE_SINGLE_THREADED:
            return "single-threaded";

        case WEB_SERVER_MODE_STATIC_THREADED:
            return "static-threaded";

        default:
        case WEB_SERVER_MODE_MULTI_THREADED:
            return "multi-threaded";
    }
}

// --------------------------------------------------------------------------------------

int api_listen_sockets_setup(void) {
    int socks = listen_sockets_setup(&api_sockets);

    if(!socks)
        fatal("LISTENER: Cannot listen on any API socket. Exiting...");

    return socks;
}

// --------------------------------------------------------------------------------------
// the main socket listener - MULTI-THREADED

// maintain a linked list of web clients - for the web servers that need it
static struct web_client *web_clients = NULL;

static void web_client_link(struct web_client *w) {
    if (web_clients) web_clients->prev = w;
    w->next = web_clients;
    web_clients = w;
}

static struct web_client *web_client_unlink(struct web_client *w) {
    struct web_client *n = w->next;
    if (w == web_clients) web_clients = n;

    if(w->prev) w->prev->next = w->next;
    if(w->next) w->next->prev = w->prev;

    return n;
}

static inline void multi_threaded_cleanup_web_clients(void) {
    struct web_client *w;

    for (w = web_clients; w;) {
        if (web_client_check_obsolete(w)) {
            debug(D_WEB_CLIENT, "%llu: Removing client.", w->id);
            struct web_client *t = web_client_unlink(w);
            web_client_free(w);
            w = t;

#ifdef NETDATA_INTERNAL_CHECKS
            log_allocations();
#endif
        }
        else w = w->next;
    }
}

// 1. it accepts new incoming requests on our port
// 2. creates a new web_client for each connection received
// 3. spawns a new netdata_thread to serve the client (this is optimal for keep-alive clients)
// 4. cleans up old web_clients that their netdata_threads have been exited

#define CLEANUP_EVERY_EVENTS 100

static struct pollfd *socket_listen_main_multi_threaded_fds = NULL;

static void socket_listen_main_multi_threaded_cleanup(void *data) {
    struct netdata_static_thread *static_thread = (struct netdata_static_thread *)data;
    if(static_thread->enabled) {
        static_thread->enabled = 0;

        info("cleaning up...");

        info("releasing allocated memory...");
        freez(socket_listen_main_multi_threaded_fds);

        info("closing all sockets...");
        listen_sockets_close(&api_sockets);

        info("cleanup completed.");
    }

    struct web_client *w;
    for(w = web_clients; w ; w = w->next) {
        if(!web_client_check_obsolete(w)) {
            WEB_CLIENT_IS_OBSOLETE(w);
            info("Stopping web client %s, id %llu", w->client_ip, w->id);
            netdata_thread_cancel(w->thread);
        }
    }
}

void *socket_listen_main_multi_threaded(void *ptr) {
    netdata_thread_cleanup_push(socket_listen_main_multi_threaded_cleanup, ptr);

    web_server_mode = WEB_SERVER_MODE_MULTI_THREADED;
    web_server_is_multithreaded = 1;

    struct web_client *w;
    int retval, counter = 0;

    if(!api_sockets.opened)
        fatal("LISTENER: No sockets to listen to.");

    socket_listen_main_multi_threaded_fds = callocz(sizeof(struct pollfd), api_sockets.opened);

    size_t i;
    for(i = 0; i < api_sockets.opened ;i++) {
        socket_listen_main_multi_threaded_fds[i].fd = api_sockets.fds[i];
        socket_listen_main_multi_threaded_fds[i].events = POLLIN;
        socket_listen_main_multi_threaded_fds[i].revents = 0;

        info("Listening on '%s'", (api_sockets.fds_names[i])?api_sockets.fds_names[i]:"UNKNOWN");
    }

    int timeout = 10 * 1000;

    while(!netdata_exit) {

        // debug(D_WEB_CLIENT, "LISTENER: Waiting...");
        retval = poll(socket_listen_main_multi_threaded_fds, api_sockets.opened, timeout);

        if(unlikely(retval == -1)) {
            error("LISTENER: poll() failed.");
            continue;
        }
        else if(unlikely(!retval)) {
            debug(D_WEB_CLIENT, "LISTENER: select() timeout.");
            counter = 0;
            multi_threaded_cleanup_web_clients();
            continue;
        }

        for(i = 0 ; i < api_sockets.opened ; i++) {
            short int revents = socket_listen_main_multi_threaded_fds[i].revents;

            // check for new incoming connections
            if(revents & POLLIN || revents & POLLPRI) {
                socket_listen_main_multi_threaded_fds[i].revents = 0;

                w = web_client_create_on_listenfd(socket_listen_main_multi_threaded_fds[i].fd);
                if(unlikely(!w)) {
                    // no need for error log - web_client_create_on_listenfd already logged the error
                    continue;
                }
                web_client_link(w);

                if(api_sockets.fds_families[i] == AF_UNIX)
                    web_client_set_unix(w);
                else
                    web_client_set_tcp(w);

                char tag[NETDATA_THREAD_TAG_MAX + 1];
                snprintfz(tag, NETDATA_THREAD_TAG_MAX, "WEB_CLIENT[%llu,[%s]:%s]", w->id, w->client_ip, w->client_port);

                if(netdata_thread_create(&w->thread, tag, NETDATA_THREAD_OPTION_DONT_LOG, web_client_main, w) != 0)
                    WEB_CLIENT_IS_OBSOLETE(w);
            }
        }

        // cleanup unused clients
        counter++;
        if(counter >= CLEANUP_EVERY_EVENTS) {
            counter = 0;
            multi_threaded_cleanup_web_clients();
        }
    }

    netdata_thread_cleanup_pop(1);
    return NULL;
}

// --------------------------------------------------------------------------------------
// the main socket listener - SINGLE-THREADED

struct web_client *single_threaded_clients[FD_SETSIZE];

static inline int single_threaded_link_client(struct web_client *w, fd_set *ifds, fd_set *ofds, fd_set *efds, int *max) {
    if(unlikely(web_client_check_obsolete(w) || web_client_check_dead(w) || (!web_client_has_wait_receive(w) && !web_client_has_wait_send(w))))
        return 1;

    if(unlikely(w->ifd < 0 || w->ifd >= (int)FD_SETSIZE || w->ofd < 0 || w->ofd >= (int)FD_SETSIZE)) {
        error("%llu: invalid file descriptor, ifd = %d, ofd = %d (required 0 <= fd < FD_SETSIZE (%d)", w->id, w->ifd, w->ofd, (int)FD_SETSIZE);
        return 1;
    }

    FD_SET(w->ifd, efds);
    if(unlikely(*max < w->ifd)) *max = w->ifd;

    if(unlikely(w->ifd != w->ofd)) {
        if(*max < w->ofd) *max = w->ofd;
        FD_SET(w->ofd, efds);
    }

    if(web_client_has_wait_receive(w)) FD_SET(w->ifd, ifds);
    if(web_client_has_wait_send(w))    FD_SET(w->ofd, ofds);

    single_threaded_clients[w->ifd] = w;
    single_threaded_clients[w->ofd] = w;

    return 0;
}

static inline int single_threaded_unlink_client(struct web_client *w, fd_set *ifds, fd_set *ofds, fd_set *efds) {
    FD_CLR(w->ifd, efds);
    if(unlikely(w->ifd != w->ofd)) FD_CLR(w->ofd, efds);

    if(web_client_has_wait_receive(w)) FD_CLR(w->ifd, ifds);
    if(web_client_has_wait_send(w))    FD_CLR(w->ofd, ofds);

    single_threaded_clients[w->ifd] = NULL;
    single_threaded_clients[w->ofd] = NULL;

    if(unlikely(web_client_check_obsolete(w) || web_client_check_dead(w) || (!web_client_has_wait_receive(w) && !web_client_has_wait_send(w))))
        return 1;

    return 0;
}

static void socket_listen_main_single_threaded_cleanup(void *data) {
    struct netdata_static_thread *static_thread = (struct netdata_static_thread *)data;
    if(static_thread->enabled) {
        static_thread->enabled = 0;

        info("cleaning up...");

        info("closing all sockets...");
        listen_sockets_close(&api_sockets);

        info("cleanup completed.");
        debug(D_WEB_CLIENT, "LISTENER: exit!");
    }
}

void *socket_listen_main_single_threaded(void *ptr) {
    netdata_thread_cleanup_push(socket_listen_main_single_threaded_cleanup, ptr);
    web_server_mode = WEB_SERVER_MODE_SINGLE_THREADED;
    web_server_is_multithreaded = 0;

    struct web_client *w;

    if(!api_sockets.opened)
        fatal("LISTENER: no listen sockets available.");

    size_t i;
    for(i = 0; i < (size_t)FD_SETSIZE ; i++)
        single_threaded_clients[i] = NULL;

    fd_set ifds, ofds, efds, rifds, rofds, refds;
    FD_ZERO (&ifds);
    FD_ZERO (&ofds);
    FD_ZERO (&efds);
    int fdmax = 0;

    for(i = 0; i < api_sockets.opened ; i++) {
        if (api_sockets.fds[i] < 0 || api_sockets.fds[i] >= (int)FD_SETSIZE)
            fatal("LISTENER: Listen socket %d is not ready, or invalid.", api_sockets.fds[i]);

        info("Listening on '%s'", (api_sockets.fds_names[i])?api_sockets.fds_names[i]:"UNKNOWN");

        FD_SET(api_sockets.fds[i], &ifds);
        FD_SET(api_sockets.fds[i], &efds);
        if(fdmax < api_sockets.fds[i])
            fdmax = api_sockets.fds[i];
    }

    while(!netdata_exit) {
        debug(D_WEB_CLIENT_ACCESS, "LISTENER: single threaded web server waiting (fdmax = %d)...", fdmax);

        struct timeval tv = { .tv_sec = 1, .tv_usec = 0 };
        rifds = ifds;
        rofds = ofds;
        refds = efds;
        int retval = select(fdmax+1, &rifds, &rofds, &refds, &tv);

        if(unlikely(retval == -1)) {
            error("LISTENER: select() failed.");
            continue;
        }
        else if(likely(retval)) {
            debug(D_WEB_CLIENT_ACCESS, "LISTENER: got something.");

            for(i = 0; i < api_sockets.opened ; i++) {
                if (FD_ISSET(api_sockets.fds[i], &rifds)) {
                    debug(D_WEB_CLIENT_ACCESS, "LISTENER: new connection.");
                    w = web_client_create_on_listenfd(api_sockets.fds[i]);
                    if(unlikely(!w))
                        continue;

                    if(api_sockets.fds_families[i] == AF_UNIX)
                        web_client_set_unix(w);
                    else
                        web_client_set_tcp(w);

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

                if (unlikely(web_client_has_wait_receive(w) && FD_ISSET(w->ifd, &rifds))) {
                    if (unlikely(web_client_receive(w) < 0)) {
                        web_client_free(w);
                        continue;
                    }

                    if (w->mode != WEB_CLIENT_MODE_FILECOPY) {
                        debug(D_WEB_CLIENT, "%llu: Processing received data.", w->id);
                        web_client_process_request(w);
                    }
                }

                if (unlikely(web_client_has_wait_send(w) && FD_ISSET(w->ofd, &rofds))) {
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
        }
    }

    netdata_thread_cleanup_pop(1);
    return NULL;
}

// --------------------------------------------------------------------------------------
// the main socket listener - STATIC-THREADED

struct web_server_static_threaded_worker {
    netdata_thread_t thread;

    int running;

    volatile size_t connected;
    volatile size_t disconnected;
    volatile size_t receptions;
    volatile size_t sends;
    volatile size_t max_concurrent;

    volatile size_t files_read;
    volatile size_t file_reads;
};

static long long static_threaded_workers_count = 1;
static struct web_server_static_threaded_worker *static_workers_private_data = NULL;
static __thread struct web_server_static_threaded_worker *worker_private = NULL;

// ----------------------------------------------------------------------------

static inline int web_server_check_client_status(struct web_client *w) {
    if(unlikely(web_client_check_obsolete(w) || web_client_check_dead(w) || (!web_client_has_wait_receive(w) && !web_client_has_wait_send(w))))
        return -1;

    return 0;
}

// ----------------------------------------------------------------------------
// web server files

static void *web_server_file_add_callback(POLLINFO *pi, short int *events, void *data) {
    struct web_client *w = (struct web_client *)data;

    worker_private->files_read++;

    debug(D_WEB_CLIENT, "%llu: ADDED FILE READ ON FD %d", w->id, pi->fd);
    *events = POLLIN;
    pi->data = w;
    return w;
}

static void web_werver_file_del_callback(POLLINFO *pi) {
    struct web_client *w = (struct web_client *)pi->data;
    debug(D_WEB_CLIENT, "%llu: RELEASE FILE READ ON FD %d", w->id, pi->fd);

    w->pollinfo_filecopy_slot = 0;

    if(unlikely(!w->pollinfo_slot)) {
        debug(D_WEB_CLIENT, "%llu: CROSS WEB CLIENT CLEANUP (iFD %d, oFD %d)", w->id, pi->fd, w->ofd);
        web_client_free(w);
    }
}

static int web_server_file_read_callback(POLLINFO *pi, short int *events) {
    struct web_client *w = (struct web_client *)pi->data;

    // if there is no POLLINFO linked to this, it means the client disconnected
    // stop the file reading too
    if(unlikely(!w->pollinfo_slot)) {
        debug(D_WEB_CLIENT, "%llu: PREVENTED ATTEMPT TO READ FILE ON FD %d, ON CLOSED WEB CLIENT", w->id, pi->fd);
        return -1;
    }

    if(unlikely(w->mode != WEB_CLIENT_MODE_FILECOPY || w->ifd == w->ofd)) {
        debug(D_WEB_CLIENT, "%llu: PREVENTED ATTEMPT TO READ FILE ON FD %d, ON NON-FILECOPY WEB CLIENT", w->id, pi->fd);
        return -1;
    }

    debug(D_WEB_CLIENT, "%llu: READING FILE ON FD %d", w->id, pi->fd);

    worker_private->file_reads++;
    ssize_t ret = unlikely(web_client_read_file(w));

    if(likely(web_client_has_wait_send(w))) {
        POLLJOB *p = pi->p;                         // our POLLJOB
        POLLINFO *wpi = &p->inf[w->pollinfo_slot];  // POLLINFO of the client socket

        debug(D_WEB_CLIENT, "%llu: SIGNALING W TO SEND (iFD %d, oFD %d)", w->id, pi->fd, wpi->fd);
        p->fds[wpi->slot].events |= POLLOUT;
    }

    if(unlikely(ret <= 0 || w->ifd == w->ofd)) {
        debug(D_WEB_CLIENT, "%llu: DONE READING FILE ON FD %d", w->id, pi->fd);
        return -1;
    }

    *events = POLLIN;
    return 0;
}

static int web_server_file_write_callback(POLLINFO *pi, short int *events) {
    (void)pi;
    (void)events;

    error("Writing to web files is not supported!");

    return -1;
}

// ----------------------------------------------------------------------------
// web server clients

static void *web_server_add_callback(POLLINFO *pi, short int *events, void *data) {
    (void)data;

    worker_private->connected++;

    size_t concurrent = worker_private->connected - worker_private->disconnected;
    if(unlikely(concurrent > worker_private->max_concurrent))
        worker_private->max_concurrent = concurrent;

    *events = POLLIN;

    debug(D_WEB_CLIENT_ACCESS, "LISTENER on %d: new connection.", pi->fd);
    struct web_client *w = web_client_create_on_fd(pi->fd, pi->client_ip, pi->client_port);
    w->pollinfo_slot = pi->slot;

    if(unlikely(pi->socktype == AF_UNIX))
        web_client_set_unix(w);
    else
        web_client_set_tcp(w);

    debug(D_WEB_CLIENT, "%llu: ADDED CLIENT FD %d", w->id, pi->fd);
    return w;
}

// TCP client disconnected
static void web_server_del_callback(POLLINFO *pi) {
    worker_private->disconnected++;

    struct web_client *w = (struct web_client *)pi->data;

    w->pollinfo_slot = 0;
    if(unlikely(w->pollinfo_filecopy_slot)) {
        POLLJOB *p = pi->p;                                  // our POLLJOB
        POLLINFO *fpi = &p->inf[w->pollinfo_filecopy_slot];  // POLLINFO of the client socket
        debug(D_WEB_CLIENT, "%llu: THE CLIENT WILL BE FRED BY READING FILE JOB ON FD %d", w->id, fpi->fd);
    }
    else {
        debug(D_WEB_CLIENT, "%llu: CLOSING CLIENT FD %d", w->id, pi->fd);
        web_client_free(w);
    }
}

static int web_server_rcv_callback(POLLINFO *pi, short int *events) {
    worker_private->receptions++;

    struct web_client *w = (struct web_client *)pi->data;
    int fd = pi->fd;

    if(unlikely(web_client_receive(w) < 0))
        return -1;

    debug(D_WEB_CLIENT, "%llu: processing received data on fd %d.", w->id, fd);
    web_client_process_request(w);

    if(unlikely(w->mode == WEB_CLIENT_MODE_FILECOPY)) {
        if(w->pollinfo_filecopy_slot == 0) {
            debug(D_WEB_CLIENT, "%llu: FILECOPY DETECTED ON FD %d", w->id, pi->fd);

            if (unlikely(w->ifd != -1 && w->ifd != w->ofd && w->ifd != fd)) {
                // add a new socket to poll_events, with the same
                debug(D_WEB_CLIENT, "%llu: CREATING FILECOPY SLOT ON FD %d", w->id, pi->fd);

                POLLINFO *fpi = poll_add_fd(
                        pi->p
                        , w->ifd
                        , 0
                        , POLLINFO_FLAG_CLIENT_SOCKET
                        , "FILENAME"
                        , ""
                        , web_server_file_add_callback
                        , web_werver_file_del_callback
                        , web_server_file_read_callback
                        , web_server_file_write_callback
                        , (void *) w
                );

                w->pollinfo_filecopy_slot = fpi->slot;
            }
        }
    }
    else {
        if(unlikely(w->ifd == fd && web_client_has_wait_receive(w)))
            *events |= POLLIN;
    }

    if(unlikely(w->ofd == fd && web_client_has_wait_send(w)))
        *events |= POLLOUT;

    return web_server_check_client_status(w);
}

static int web_server_snd_callback(POLLINFO *pi, short int *events) {
    worker_private->sends++;

    struct web_client *w = (struct web_client *)pi->data;
    int fd = pi->fd;

    debug(D_WEB_CLIENT, "%llu: sending data on fd %d.", w->id, fd);

    if(unlikely(web_client_send(w) < 0))
        return -1;

    if(unlikely(w->ifd == fd && web_client_has_wait_receive(w)))
        *events |= POLLIN;

    if(unlikely(w->ofd == fd && web_client_has_wait_send(w)))
        *events |= POLLOUT;

    return web_server_check_client_status(w);
}


// ----------------------------------------------------------------------------
// web server worker thread

static void socket_listen_main_static_threaded_worker_cleanup(void *ptr) {
    worker_private = (struct web_server_static_threaded_worker *)ptr;
    worker_private->running = 0;

    info("stopped after %zu connects, %zu disconnects (max concurrent %zu), %zu receptions and %zu sends",
            worker_private->connected,
            worker_private->disconnected,
            worker_private->max_concurrent,
            worker_private->receptions,
            worker_private->sends
    );
}

void *socket_listen_main_static_threaded_worker(void *ptr) {
    worker_private = (struct web_server_static_threaded_worker *)ptr;
    worker_private->running = 1;

    netdata_thread_cleanup_push(socket_listen_main_static_threaded_worker_cleanup, ptr);

            poll_events(&api_sockets
                        , web_server_add_callback
                        , web_server_del_callback
                        , web_server_rcv_callback
                        , web_server_snd_callback
                        , web_allow_connections_from
                        , NULL
            );

    netdata_thread_cleanup_pop(1);
    return NULL;
}


// ----------------------------------------------------------------------------
// web server main thread - also becomes a worker

static void socket_listen_main_static_threaded_cleanup(void *ptr) {
    struct netdata_static_thread *static_thread = (struct netdata_static_thread *)ptr;
    if(static_thread->enabled) {
        static_thread->enabled = 0;

        int i;
        for(i = 1; i < static_threaded_workers_count; i++) {
            if(static_workers_private_data[i].running) {
                info("stopping worker %d", i + 1);
                netdata_thread_cancel(static_workers_private_data[i].thread);
            }
            else
                info("found stopped worker %d", i + 1);
        }

        info("closing all web server sockets...");
        listen_sockets_close(&api_sockets);
    }
}

void *socket_listen_main_static_threaded(void *ptr) {
    netdata_thread_cleanup_push(socket_listen_main_static_threaded_cleanup, ptr);
            web_server_mode = WEB_SERVER_MODE_STATIC_THREADED;

            if(!api_sockets.opened)
                fatal("LISTENER: no listen sockets available.");

            // 6 threads is the optimal value
            // since 6 are the parallel connections browsers will do
            // so, if the machine has more CPUs, avoid using resources unnecessarily
            int def_thread_count = (processors > 6)?6:processors;

            static_threaded_workers_count = config_get_number(CONFIG_SECTION_WEB, "web server threads", def_thread_count);
            if(static_threaded_workers_count < 1) static_threaded_workers_count = 1;

            static_workers_private_data = callocz((size_t)static_threaded_workers_count, sizeof(struct web_server_static_threaded_worker));

            web_server_is_multithreaded = (static_threaded_workers_count > 1);

            int i;
            for(i = 1; i < static_threaded_workers_count; i++) {
                char tag[50 + 1];
                snprintfz(tag, 50, "WEB_SERVER[static%d]", i+1);

                info("starting worker %d", i+1);
                netdata_thread_create(&static_workers_private_data[i].thread, tag, NETDATA_THREAD_OPTION_DEFAULT, socket_listen_main_static_threaded_worker, (void *)&static_workers_private_data[i]);
            }

            // and the main one
            socket_listen_main_static_threaded_worker((void *)&static_workers_private_data[0]);

    netdata_thread_cleanup_pop(1);
    return NULL;
}
