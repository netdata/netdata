// SPDX-License-Identifier: GPL-3.0+
#include "common.h"

// this file includes 3 web servers:
//
// 1. single-threaded, based on select()
// 2. multi-threaded, based on poll() that spawns threads to handle the requests, based on select()
// 3. static-threaded, based on poll() using a fixed number of threads (configured at netdata.conf)

WEB_SERVER_MODE web_server_mode = WEB_SERVER_MODE_STATIC_THREADED;

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
// API sockets

static LISTEN_SOCKETS api_sockets = {
        .config_section  = CONFIG_SECTION_WEB,
        .default_bind_to = "*",
        .default_port    = API_LISTEN_PORT,
        .backlog         = API_LISTEN_BACKLOG
};

int api_listen_sockets_setup(void) {
    int socks = listen_sockets_setup(&api_sockets);

    if(!socks)
        fatal("LISTENER: Cannot listen on any API socket. Exiting...");

    return socks;
}


// --------------------------------------------------------------------------------------
// access lists

SIMPLE_PATTERN *web_allow_connections_from = NULL;
SIMPLE_PATTERN *web_allow_streaming_from = NULL;
SIMPLE_PATTERN *web_allow_netdataconf_from = NULL;

// WEB_CLIENT_ACL
SIMPLE_PATTERN *web_allow_dashboard_from = NULL;
SIMPLE_PATTERN *web_allow_registry_from = NULL;
SIMPLE_PATTERN *web_allow_badges_from = NULL;

static void web_client_update_acl_matches(struct web_client *w) {
    w->acl = WEB_CLIENT_ACL_NONE;

    if(!web_allow_dashboard_from || simple_pattern_matches(web_allow_dashboard_from, w->client_ip))
        w->acl |= WEB_CLIENT_ACL_DASHBOARD;

    if(!web_allow_registry_from || simple_pattern_matches(web_allow_registry_from, w->client_ip))
        w->acl |= WEB_CLIENT_ACL_REGISTRY;

    if(!web_allow_badges_from || simple_pattern_matches(web_allow_badges_from, w->client_ip))
        w->acl |= WEB_CLIENT_ACL_BADGE;
}


// --------------------------------------------------------------------------------------

static void log_connection(struct web_client *w, const char *msg) {
    log_access("%llu: %d '[%s]:%s' '%s'", w->id, gettid(), w->client_ip, w->client_port, msg);
}

// ----------------------------------------------------------------------------
// allocate and free web_clients

static void web_client_zero(struct web_client *w) {
    // zero everything about it - but keep the buffers

    // remember the pointers to the buffers
    BUFFER *b1 = w->response.data;
    BUFFER *b2 = w->response.header;
    BUFFER *b3 = w->response.header_output;

    // empty the buffers
    buffer_flush(b1);
    buffer_flush(b2);
    buffer_flush(b3);

    freez(w->user_agent);

    // zero everything
    memset(w, 0, sizeof(struct web_client));

    // restore the pointers of the buffers
    w->response.data = b1;
    w->response.header = b2;
    w->response.header_output = b3;
}

static void web_client_free(struct web_client *w) {
    buffer_free(w->response.header_output);
    buffer_free(w->response.header);
    buffer_free(w->response.data);
    freez(w->user_agent);
    freez(w);
}

static struct web_client *web_client_alloc(void) {
    struct web_client *w = callocz(1, sizeof(struct web_client));
    w->response.data = buffer_create(NETDATA_WEB_RESPONSE_INITIAL_SIZE);
    w->response.header = buffer_create(NETDATA_WEB_RESPONSE_HEADER_SIZE);
    w->response.header_output = buffer_create(NETDATA_WEB_RESPONSE_HEADER_SIZE);
    return w;
}

// ----------------------------------------------------------------------------
// web clients caching

// When clients connect and disconnect, avoid allocating and releasing memory.
// Instead, when new clients get connected, reuse any memory previously allocated
// for serving web clients that are now disconnected.

// The size of the cache is adaptive. It caches the structures of 2x
// the number of currently connected clients.

// Comments per server:
// SINGLE-THREADED : 1 cache is maintained
// MULTI-THREADED  : 1 cache is maintained
// STATIC-THREADED : 1 cache for each thred of the web server

struct clients_cache {
    pid_t pid;

    struct web_client *used;    // the structures of the currently connected clients
    size_t used_count;          // the count the currently connected clients

    struct web_client *avail;   // the cached structures, available for future clients
    size_t avail_count;         // the number of cached structures

    size_t reused;              // the number of re-uses
    size_t allocated;           // the number of allocations
};

static __thread struct clients_cache web_clients_cache = {
        .pid = 0,
        .used = NULL,
        .used_count = 0,
        .avail = NULL,
        .avail_count = 0,
        .allocated = 0,
        .reused = 0
};

static inline void web_client_cache_verify(int force) {
#ifdef NETDATA_INTERNAL_CHECKS
    static __thread size_t count = 0;
    count++;

    if(unlikely(force || count > 1000)) {
        count = 0;

        struct web_client *w;
        size_t used = 0, avail = 0;
        for(w = web_clients_cache.used; w ; w = w->next) used++;
        for(w = web_clients_cache.avail; w ; w = w->next) avail++;

        info("web_client_cache has %zu (%zu) used and %zu (%zu) available clients, allocated %zu, reused %zu (hit %zu%%)."
             , used, web_clients_cache.used_count
             , avail, web_clients_cache.avail_count
             , web_clients_cache.allocated
             , web_clients_cache.reused
             , (web_clients_cache.allocated + web_clients_cache.reused)?(web_clients_cache.reused * 100 / (web_clients_cache.allocated + web_clients_cache.reused)):0
        );
    }
#else
    if(unlikely(force)) {
        info("web_client_cache has %zu used and %zu available clients, allocated %zu, reused %zu (hit %zu%%)."
             , web_clients_cache.used_count
             , web_clients_cache.avail_count
             , web_clients_cache.allocated
             , web_clients_cache.reused
             , (web_clients_cache.allocated + web_clients_cache.reused)?(web_clients_cache.reused * 100 / (web_clients_cache.allocated + web_clients_cache.reused)):0
            );
    }
#endif
}

// destroy the cache and free all the memory it uses
static void web_client_cache_destroy(void) {
#ifdef NETDATA_INTERNAL_CHECKS
    if(unlikely(web_clients_cache.pid != 0 && web_clients_cache.pid != gettid()))
        error("Oops! wrong thread accessing the cache. Expected %d, found %d", (int)web_clients_cache.pid, (int)gettid());

    web_client_cache_verify(1);
#endif

    netdata_thread_disable_cancelability();

    struct web_client *w, *t;

    w = web_clients_cache.used;
    while(w) {
        t = w;
        w = w->next;
        web_client_free(t);
    }
    web_clients_cache.used = NULL;
    web_clients_cache.used_count = 0;

    w = web_clients_cache.avail;
    while(w) {
        t = w;
        w = w->next;
        web_client_free(t);
    }
    web_clients_cache.avail = NULL;
    web_clients_cache.avail_count = 0;

    netdata_thread_enable_cancelability();
}

static struct web_client *web_client_get_from_cache_or_allocate() {

#ifdef NETDATA_INTERNAL_CHECKS
    if(unlikely(web_clients_cache.pid == 0))
        web_clients_cache.pid = gettid();

    if(unlikely(web_clients_cache.pid != 0 && web_clients_cache.pid != gettid()))
        error("Oops! wrong thread accessing the cache. Expected %d, found %d", (int)web_clients_cache.pid, (int)gettid());
#endif

    netdata_thread_disable_cancelability();

    struct web_client *w = web_clients_cache.avail;

    if(w) {
        // get it from avail
        if (w == web_clients_cache.avail) web_clients_cache.avail = w->next;
        if(w->prev) w->prev->next = w->next;
        if(w->next) w->next->prev = w->prev;
        web_clients_cache.avail_count--;
        web_client_zero(w);
        web_clients_cache.reused++;
    }
    else {
        // allocate it
        w = web_client_alloc();
        web_clients_cache.allocated++;
    }

    // link it to used web clients
    if (web_clients_cache.used) web_clients_cache.used->prev = w;
    w->next = web_clients_cache.used;
    w->prev = NULL;
    web_clients_cache.used = w;
    web_clients_cache.used_count++;

    // initialize it
    w->id = web_client_connected();
    w->mode = WEB_CLIENT_MODE_NORMAL;

    netdata_thread_enable_cancelability();

    return w;
}

static void web_client_release(struct web_client *w) {
#ifdef NETDATA_INTERNAL_CHECKS
    if(unlikely(web_clients_cache.pid != 0 && web_clients_cache.pid != gettid()))
        error("Oops! wrong thread accessing the cache. Expected %d, found %d", (int)web_clients_cache.pid, (int)gettid());

    if(unlikely(w->running))
        error("%llu: releasing web client from %s port %s, but it still running.", w->id, w->client_ip, w->client_port);
#endif

    debug(D_WEB_CLIENT_ACCESS, "%llu: Closing web client from %s port %s.", w->id, w->client_ip, w->client_port);

    log_connection(w, "DISCONNECTED");
    web_client_request_done(w);
    web_client_disconnected();

    netdata_thread_disable_cancelability();

    if(web_server_mode != WEB_SERVER_MODE_STATIC_THREADED) {
        if (w->ifd != -1) close(w->ifd);
        if (w->ofd != -1 && w->ofd != w->ifd) close(w->ofd);
        w->ifd = w->ofd = -1;
    }

    // unlink it from the used
    if (w == web_clients_cache.used) web_clients_cache.used = w->next;
    if(w->prev) w->prev->next = w->next;
    if(w->next) w->next->prev = w->prev;
    web_clients_cache.used_count--;

    if(web_clients_cache.avail_count >= 2 * web_clients_cache.used_count) {
        // we have too many of them - free it
        web_client_free(w);
    }
    else {
        // link it to the avail
        if (web_clients_cache.avail) web_clients_cache.avail->prev = w;
        w->next = web_clients_cache.avail;
        w->prev = NULL;
        web_clients_cache.avail = w;
        web_clients_cache.avail_count++;
    }

    netdata_thread_enable_cancelability();
}


// ----------------------------------------------------------------------------
// high level web clients connection management

static void web_client_initialize_connection(struct web_client *w) {
    int flag = 1;

    if(unlikely(web_client_check_tcp(w) && setsockopt(w->ifd, IPPROTO_TCP, TCP_NODELAY, (char *) &flag, sizeof(int)) != 0))
        debug(D_WEB_CLIENT, "%llu: failed to enable TCP_NODELAY on socket fd %d.", w->id, w->ifd);

    flag = 1;
    if(unlikely(setsockopt(w->ifd, SOL_SOCKET, SO_KEEPALIVE, (char *) &flag, sizeof(int)) != 0))
        debug(D_WEB_CLIENT, "%llu: failed to enable SO_KEEPALIVE on socket fd %d.", w->id, w->ifd);

    web_client_update_acl_matches(w);

    w->origin[0] = '*'; w->origin[1] = '\0';
    w->cookie1[0] = '\0'; w->cookie2[0] = '\0';
    freez(w->user_agent); w->user_agent = NULL;

    web_client_enable_wait_receive(w);

    log_connection(w, "CONNECTED");

    web_client_cache_verify(0);
}

static struct web_client *web_client_create_on_fd(int fd, const char *client_ip, const char *client_port) {
    struct web_client *w;

    w = web_client_get_from_cache_or_allocate();
    w->ifd = w->ofd = fd;

    strncpyz(w->client_ip, client_ip, sizeof(w->client_ip) - 1);
    strncpyz(w->client_port, client_port, sizeof(w->client_port) - 1);

    if(unlikely(!*w->client_ip))   strcpy(w->client_ip,   "-");
    if(unlikely(!*w->client_port)) strcpy(w->client_port, "-");

    web_client_initialize_connection(w);
    return(w);
}

static struct web_client *web_client_create_on_listenfd(int listener) {
    struct web_client *w;

    w = web_client_get_from_cache_or_allocate();
    w->ifd = w->ofd = accept_socket(listener, SOCK_NONBLOCK, w->client_ip, sizeof(w->client_ip), w->client_port, sizeof(w->client_port), web_allow_connections_from);

    if(unlikely(!*w->client_ip))   strcpy(w->client_ip,   "-");
    if(unlikely(!*w->client_port)) strcpy(w->client_port, "-");

    if (w->ifd == -1) {
        if(errno == EPERM)
            log_connection(w, "ACCESS DENIED");
        else {
            log_connection(w, "CONNECTION FAILED");
            error("%llu: Failed to accept new incoming connection.", w->id);
        }

        web_client_release(w);
        return NULL;
    }

    web_client_initialize_connection(w);
    return(w);
}


// --------------------------------------------------------------------------------------
// the thread of a single client - for the MULTI-THREADED web server

// 1. waits for input and output, using async I/O
// 2. it processes HTTP requests
// 3. it generates HTTP responses
// 4. it copies data from input to output if mode is FILECOPY

int web_client_timeout = DEFAULT_DISCONNECT_IDLE_WEB_CLIENTS_AFTER_SECONDS;
int web_client_first_request_timeout = DEFAULT_TIMEOUT_TO_RECEIVE_FIRST_WEB_REQUEST;
long web_client_streaming_rate_t = 0L;

static void multi_threaded_web_client_worker_main_cleanup(void *ptr) {
    struct web_client *w = ptr;
    WEB_CLIENT_IS_DEAD(w);
    w->running = 0;
}

static void *multi_threaded_web_client_worker_main(void *ptr) {
    netdata_thread_cleanup_push(multi_threaded_web_client_worker_main_cleanup, ptr);

            struct web_client *w = ptr;
            w->running = 1;

            struct pollfd fds[2], *ifd, *ofd;
            int retval, timeout_ms;
            nfds_t fdmax = 0;

            while(!netdata_exit) {
                if(unlikely(web_client_check_dead(w))) {
                    debug(D_WEB_CLIENT, "%llu: client is dead.", w->id);
                    break;
                }
                else if(unlikely(!web_client_has_wait_receive(w) && !web_client_has_wait_send(w))) {
                    debug(D_WEB_CLIENT, "%llu: client is not set for neither receiving nor sending data.", w->id);
                    break;
                }

                if(unlikely(w->ifd < 0 || w->ofd < 0)) {
                    error("%llu: invalid file descriptor, ifd = %d, ofd = %d (required 0 <= fd", w->id, w->ifd, w->ofd);
                    break;
                }

                if(w->ifd == w->ofd) {
                    fds[0].fd = w->ifd;
                    fds[0].events = 0;
                    fds[0].revents = 0;

                    if(web_client_has_wait_receive(w)) fds[0].events |= POLLIN;
                    if(web_client_has_wait_send(w))    fds[0].events |= POLLOUT;

                    fds[1].fd = -1;
                    fds[1].events = 0;
                    fds[1].revents = 0;

                    ifd = ofd = &fds[0];

                    fdmax = 1;
                }
                else {
                    fds[0].fd = w->ifd;
                    fds[0].events = 0;
                    fds[0].revents = 0;
                    if(web_client_has_wait_receive(w)) fds[0].events |= POLLIN;
                    ifd = &fds[0];

                    fds[1].fd = w->ofd;
                    fds[1].events = 0;
                    fds[1].revents = 0;
                    if(web_client_has_wait_send(w))    fds[1].events |= POLLOUT;
                    ofd = &fds[1];

                    fdmax = 2;
                }

                debug(D_WEB_CLIENT, "%llu: Waiting socket async I/O for %s %s", w->id, web_client_has_wait_receive(w)?"INPUT":"", web_client_has_wait_send(w)?"OUTPUT":"");
                errno = 0;
                timeout_ms = web_client_timeout * 1000;
                retval = poll(fds, fdmax, timeout_ms);

                if(unlikely(netdata_exit)) break;

                if(unlikely(retval == -1)) {
                    if(errno == EAGAIN || errno == EINTR) {
                        debug(D_WEB_CLIENT, "%llu: EAGAIN received.", w->id);
                        continue;
                    }

                    debug(D_WEB_CLIENT, "%llu: LISTENER: poll() failed (input fd = %d, output fd = %d). Closing client.", w->id, w->ifd, w->ofd);
                    break;
                }
                else if(unlikely(!retval)) {
                    debug(D_WEB_CLIENT, "%llu: Timeout while waiting socket async I/O for %s %s", w->id, web_client_has_wait_receive(w)?"INPUT":"", web_client_has_wait_send(w)?"OUTPUT":"");
                    break;
                }

                if(unlikely(netdata_exit)) break;

                int used = 0;
                if(web_client_has_wait_send(w) && ofd->revents & POLLOUT) {
                    used++;
                    if(web_client_send(w) < 0) {
                        debug(D_WEB_CLIENT, "%llu: Cannot send data to client. Closing client.", w->id);
                        break;
                    }
                }

                if(unlikely(netdata_exit)) break;

                if(web_client_has_wait_receive(w) && (ifd->revents & POLLIN || ifd->revents & POLLPRI)) {
                    used++;
                    if(web_client_receive(w) < 0) {
                        debug(D_WEB_CLIENT, "%llu: Cannot receive data from client. Closing client.", w->id);
                        break;
                    }

                    if(w->mode == WEB_CLIENT_MODE_NORMAL) {
                        debug(D_WEB_CLIENT, "%llu: Attempting to process received data.", w->id);
                        web_client_process_request(w);

                        // if the sockets are closed, may have transferred this client
                        // to plugins.d
                        if(unlikely(w->mode == WEB_CLIENT_MODE_STREAM))
                            break;
                    }
                }

                if(unlikely(!used)) {
                    debug(D_WEB_CLIENT_ACCESS, "%llu: Received error on socket.", w->id);
                    break;
                }
            }

            if(w->mode != WEB_CLIENT_MODE_STREAM)
                log_connection(w, "DISCONNECTED");

            web_client_request_done(w);

            debug(D_WEB_CLIENT, "%llu: done...", w->id);

            // close the sockets/files now
            // to free file descriptors
            if(w->ifd == w->ofd) {
                if(w->ifd != -1) close(w->ifd);
            }
            else {
                if(w->ifd != -1) close(w->ifd);
                if(w->ofd != -1) close(w->ofd);
            }
            w->ifd = -1;
            w->ofd = -1;

    netdata_thread_cleanup_pop(1);
    return NULL;
}

// --------------------------------------------------------------------------------------
// the main socket listener - MULTI-THREADED

// 1. it accepts new incoming requests on our port
// 2. creates a new web_client for each connection received
// 3. spawns a new netdata_thread to serve the client (this is optimal for keep-alive clients)
// 4. cleans up old web_clients that their netdata_threads have been exited

static void web_client_multi_threaded_web_server_release_clients(void) {
    struct web_client *w;
    for(w = web_clients_cache.used; w ; ) {
        if(unlikely(!w->running && web_client_check_dead(w))) {
            struct web_client *t = w->next;
            web_client_release(w);
            w = t;
        }
        else
            w = w->next;
    }
}

static void web_client_multi_threaded_web_server_stop_all_threads(void) {
    struct web_client *w;

    int found = 1, max = 2 * USEC_PER_SEC, step = 50000;
    for(w = web_clients_cache.used; w ; w = w->next) {
        if(w->running) {
            found++;
            info("stopping web client %s, id %llu", w->client_ip, w->id);
            netdata_thread_cancel(w->thread);
        }
    }

    while(found && max > 0) {
        max -= step;
        info("Waiting %d web threads to finish...", found);
        sleep_usec(step);
        found = 0;
        for(w = web_clients_cache.used; w ; w = w->next)
            if(w->running) found++;
    }

    if(found)
        error("%d web threads are taking too long to finish. Giving up.", found);
}

static struct pollfd *socket_listen_main_multi_threaded_fds = NULL;

static void socket_listen_main_multi_threaded_cleanup(void *data) {
    struct netdata_static_thread *static_thread = (struct netdata_static_thread *)data;
    static_thread->enabled = NETDATA_MAIN_THREAD_EXITING;

    info("cleaning up...");

    info("releasing allocated memory...");
    freez(socket_listen_main_multi_threaded_fds);

    info("closing all sockets...");
    listen_sockets_close(&api_sockets);

    info("stopping all running web server threads...");
    web_client_multi_threaded_web_server_stop_all_threads();

    info("freeing web clients cache...");
    web_client_cache_destroy();

    info("cleanup completed.");
    static_thread->enabled = NETDATA_MAIN_THREAD_EXITED;
}

#define CLEANUP_EVERY_EVENTS 60
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

    int timeout_ms = 1 * 1000;

    while(!netdata_exit) {

        // debug(D_WEB_CLIENT, "LISTENER: Waiting...");
        retval = poll(socket_listen_main_multi_threaded_fds, api_sockets.opened, timeout_ms);

        if(unlikely(retval == -1)) {
            error("LISTENER: poll() failed.");
            continue;
        }
        else if(unlikely(!retval)) {
            debug(D_WEB_CLIENT, "LISTENER: poll() timeout.");
            counter++;
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

                if(api_sockets.fds_families[i] == AF_UNIX)
                    web_client_set_unix(w);
                else
                    web_client_set_tcp(w);

                char tag[NETDATA_THREAD_TAG_MAX + 1];
                snprintfz(tag, NETDATA_THREAD_TAG_MAX, "WEB_CLIENT[%llu,[%s]:%s]", w->id, w->client_ip, w->client_port);

                w->running = 1;
                if(netdata_thread_create(&w->thread, tag, NETDATA_THREAD_OPTION_DONT_LOG, multi_threaded_web_client_worker_main, w) != 0) {
                    w->running = 0;
                    web_client_release(w);
                }
            }
        }

        counter++;
        if(counter > CLEANUP_EVERY_EVENTS) {
            counter = 0;
            web_client_multi_threaded_web_server_release_clients();
        }
    }

    netdata_thread_cleanup_pop(1);
    return NULL;
}


// --------------------------------------------------------------------------------------
// the main socket listener - SINGLE-THREADED

struct web_client *single_threaded_clients[FD_SETSIZE];

static inline int single_threaded_link_client(struct web_client *w, fd_set *ifds, fd_set *ofds, fd_set *efds, int *max) {
    if(unlikely(web_client_check_dead(w) || (!web_client_has_wait_receive(w) && !web_client_has_wait_send(w)))) {
        return 1;
    }

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

    if(unlikely(web_client_check_dead(w) || (!web_client_has_wait_receive(w) && !web_client_has_wait_send(w)))) {
        return 1;
    }

    return 0;
}

static void socket_listen_main_single_threaded_cleanup(void *data) {
    struct netdata_static_thread *static_thread = (struct netdata_static_thread *)data;
    static_thread->enabled = NETDATA_MAIN_THREAD_EXITING;

    info("closing all sockets...");
    listen_sockets_close(&api_sockets);

    info("freeing web clients cache...");
    web_client_cache_destroy();

    info("cleanup completed.");
    static_thread->enabled = NETDATA_MAIN_THREAD_EXITED;
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
                        web_client_release(w);
                    }
                }
            }

            for(i = 0 ; i <= (size_t)fdmax ; i++) {
                if(likely(!FD_ISSET(i, &rifds) && !FD_ISSET(i, &rofds) && !FD_ISSET(i, &refds)))
                    continue;

                w = single_threaded_clients[i];
                if(unlikely(!w)) {
                    // error("no client on slot %zu", i);
                    continue;
                }

                if(unlikely(single_threaded_unlink_client(w, &ifds, &ofds, &efds) != 0)) {
                    // error("failed to unlink client %zu", i);
                    web_client_release(w);
                    continue;
                }

                if (unlikely(FD_ISSET(w->ifd, &refds) || FD_ISSET(w->ofd, &refds))) {
                    // error("no input on client %zu", i);
                    web_client_release(w);
                    continue;
                }

                if (unlikely(web_client_has_wait_receive(w) && FD_ISSET(w->ifd, &rifds))) {
                    if (unlikely(web_client_receive(w) < 0)) {
                        // error("cannot read from client %zu", i);
                        web_client_release(w);
                        continue;
                    }

                    if (w->mode != WEB_CLIENT_MODE_FILECOPY) {
                        debug(D_WEB_CLIENT, "%llu: Processing received data.", w->id);
                        web_client_process_request(w);
                    }
                }

                if (unlikely(web_client_has_wait_send(w) && FD_ISSET(w->ofd, &rofds))) {
                    if (unlikely(web_client_send(w) < 0)) {
                        // error("cannot send data to client %zu", i);
                        debug(D_WEB_CLIENT, "%llu: Cannot send data to client. Closing client.", w->id);
                        web_client_release(w);
                        continue;
                    }
                }

                if(unlikely(single_threaded_link_client(w, &ifds, &ofds, &efds, &fdmax) != 0)) {
                    // error("failed to link client %zu", i);
                    web_client_release(w);
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

    int id;
    int running;

    size_t max_sockets;

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
    if(unlikely(web_client_check_dead(w) || (!web_client_has_wait_receive(w) && !web_client_has_wait_send(w))))
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
        web_client_release(w);
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
        POLLJOB *p = pi->p;                                        // our POLLJOB
        POLLINFO *wpi = pollinfo_from_slot(p, w->pollinfo_slot);  // POLLINFO of the client socket

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
        POLLINFO *fpi = pollinfo_from_slot(pi->p, w->pollinfo_filecopy_slot);  // POLLINFO of the client socket
        debug(D_WEB_CLIENT, "%llu: THE CLIENT WILL BE FRED BY READING FILE JOB ON FD %d", w->id, fpi->fd);
    }
    else {
        if(web_client_flag_check(w, WEB_CLIENT_FLAG_DONT_CLOSE_SOCKET))
            pi->flags |= POLLINFO_FLAG_DONT_CLOSE;

        debug(D_WEB_CLIENT, "%llu: CLOSING CLIENT FD %d", w->id, pi->fd);
        web_client_release(w);
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

                if(fpi)
                    w->pollinfo_filecopy_slot = fpi->slot;
                else {
                    error("Failed to add filecopy fd. Closing client.");
                    return -1;
                }
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

static void web_server_tmr_callback(void *timer_data) {
    worker_private = (struct web_server_static_threaded_worker *)timer_data;

    static __thread RRDSET *st = NULL;
    static __thread RRDDIM *rd_user = NULL, *rd_system = NULL;

    if(unlikely(!st)) {
        char id[100 + 1];
        char title[100 + 1];

        snprintfz(id, 100, "web_thread%d_cpu", worker_private->id + 1);
        snprintfz(title, 100, "NetData web server thread No %d CPU usage", worker_private->id + 1);

        st = rrdset_create_localhost(
                "netdata"
                , id
                , NULL
                , "web"
                , "netdata.web_cpu"
                , title
                , "milliseconds/s"
                , "web"
                , "stats"
                , 132000 + worker_private->id
                , default_rrd_update_every
                , RRDSET_TYPE_STACKED
        );

        rd_user   = rrddim_add(st, "user", NULL, 1, 1000, RRD_ALGORITHM_INCREMENTAL);
        rd_system = rrddim_add(st, "system", NULL, 1, 1000, RRD_ALGORITHM_INCREMENTAL);
    }
    else
        rrdset_next(st);

    struct rusage rusage;
    getrusage(RUSAGE_THREAD, &rusage);
    rrddim_set_by_pointer(st, rd_user, rusage.ru_utime.tv_sec * 1000000ULL + rusage.ru_utime.tv_usec);
    rrddim_set_by_pointer(st, rd_system, rusage.ru_stime.tv_sec * 1000000ULL + rusage.ru_stime.tv_usec);
    rrdset_done(st);
}

// ----------------------------------------------------------------------------
// web server worker thread

static void socket_listen_main_static_threaded_worker_cleanup(void *ptr) {
    worker_private = (struct web_server_static_threaded_worker *)ptr;

    info("freeing local web clients cache...");
    web_client_cache_destroy();

    info("stopped after %zu connects, %zu disconnects (max concurrent %zu), %zu receptions and %zu sends",
            worker_private->connected,
            worker_private->disconnected,
            worker_private->max_concurrent,
            worker_private->receptions,
            worker_private->sends
    );

    worker_private->running = 0;
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
                        , web_server_tmr_callback
                        , web_allow_connections_from
                        , NULL
                        , web_client_first_request_timeout
                        , web_client_timeout
                        , default_rrd_update_every * 1000 // timer_milliseconds
                        , ptr // timer_data
                        , worker_private->max_sockets
            );

    netdata_thread_cleanup_pop(1);
    return NULL;
}


// ----------------------------------------------------------------------------
// web server main thread - also becomes a worker

static void socket_listen_main_static_threaded_cleanup(void *ptr) {
    struct netdata_static_thread *static_thread = (struct netdata_static_thread *)ptr;
    static_thread->enabled = NETDATA_MAIN_THREAD_EXITING;

    int i, found = 0, max = 2 * USEC_PER_SEC, step = 50000;

    // we start from 1, - 0 is self
    for(i = 1; i < static_threaded_workers_count; i++) {
        if(static_workers_private_data[i].running) {
            found++;
            info("stopping worker %d", i + 1);
            netdata_thread_cancel(static_workers_private_data[i].thread);
        }
        else
            info("found stopped worker %d", i + 1);
    }

    while(found && max > 0) {
        max -= step;
        info("Waiting %d static web threads to finish...", found);
        sleep_usec(step);
        found = 0;

        // we start from 1, - 0 is self
        for(i = 1; i < static_threaded_workers_count; i++) {
            if (static_workers_private_data[i].running)
                found++;
        }
    }

    if(found)
        error("%d static web threads are taking too long to finish. Giving up.", found);

    info("closing all web server sockets...");
    listen_sockets_close(&api_sockets);

    info("all static web threads stopped.");
    static_thread->enabled = NETDATA_MAIN_THREAD_EXITED;
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

            size_t max_sockets = (size_t)config_get_number(CONFIG_SECTION_WEB, "web server max sockets", (long long int)(rlimit_nofile.rlim_cur / 2));

            static_workers_private_data = callocz((size_t)static_threaded_workers_count, sizeof(struct web_server_static_threaded_worker));

            web_server_is_multithreaded = (static_threaded_workers_count > 1);

            int i;
            for(i = 1; i < static_threaded_workers_count; i++) {
                static_workers_private_data[i].id = i;
                static_workers_private_data[i].max_sockets = max_sockets / static_threaded_workers_count;

                char tag[50 + 1];
                snprintfz(tag, 50, "WEB_SERVER[static%d]", i+1);

                info("starting worker %d", i+1);
                netdata_thread_create(&static_workers_private_data[i].thread, tag, NETDATA_THREAD_OPTION_DEFAULT, socket_listen_main_static_threaded_worker, (void *)&static_workers_private_data[i]);
            }

            // and the main one
            static_workers_private_data[0].max_sockets = max_sockets / static_threaded_workers_count;
            socket_listen_main_static_threaded_worker((void *)&static_workers_private_data[0]);

    netdata_thread_cleanup_pop(1);
    return NULL;
}
