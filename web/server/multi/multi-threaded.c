// SPDX-License-Identifier: GPL-3.0-or-later

#define WEB_SERVER_INTERNALS 1
#include "multi-threaded.h"

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
                web_server_log_connection(w, "DISCONNECTED");

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

    int found = 1;
    usec_t max = 2 * USEC_PER_SEC, step = 50000;
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


