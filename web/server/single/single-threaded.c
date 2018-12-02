// SPDX-License-Identifier: GPL-3.0-or-later

#define WEB_SERVER_INTERNALS 1
#include "single-threaded.h"

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


