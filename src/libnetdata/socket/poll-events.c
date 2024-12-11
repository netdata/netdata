// SPDX-License-Identifier: GPL-3.0-or-later

#include "libnetdata/libnetdata.h"

// poll() based listener
// this should be the fastest possible listener for up to 100 sockets
// above 100, an epoll() interface is needed on Linux

#define POLL_FDS_INCREASE_STEP 10

inline POLLINFO *poll_add_fd(POLLJOB *p
                             , int fd
                             , int socktype
                             , HTTP_ACL port_acl
                             , uint32_t flags
                             , const char *client_ip
                             , const char *client_port
                             , const char *client_host
                             , void *(*add_callback)(POLLINFO * /*pi*/, short int * /*events*/, void * /*data*/)
                                 , void  (*del_callback)(POLLINFO * /*pi*/)
                                 , int   (*rcv_callback)(POLLINFO * /*pi*/, short int * /*events*/)
                                 , int   (*snd_callback)(POLLINFO * /*pi*/, short int * /*events*/)
                                 , void *data
) {
    if(unlikely(fd < 0)) return NULL;

    //if(p->limit && p->used >= p->limit) {
    //    nd_log(NDLS_DAEMON, NDLP_WARNING, "Max sockets limit reached (%zu sockets), dropping connection", p->used);
    //    close(fd);
    //    return NULL;
    //}

    if(unlikely(!p->first_free)) {
        size_t new_slots = p->slots + POLL_FDS_INCREASE_STEP;

        p->fds = reallocz(p->fds, sizeof(struct pollfd) * new_slots);
        p->inf = reallocz(p->inf, sizeof(POLLINFO) * new_slots);

        // reset all the newly added slots
        ssize_t i;
        for(i = new_slots - 1; i >= (ssize_t)p->slots ; i--) {
            p->fds[i].fd = -1;
            p->fds[i].events = 0;
            p->fds[i].revents = 0;

            p->inf[i].p = p;
            p->inf[i].slot = (size_t)i;
            p->inf[i].flags = 0;
            p->inf[i].socktype = -1;
            p->inf[i].port_acl = -1;

            p->inf[i].client_ip = NULL;
            p->inf[i].client_port = NULL;
            p->inf[i].client_host = NULL;
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

    struct pollfd *pf = &p->fds[pi->slot];
    pf->fd = fd;
    pf->events = POLLIN;
    pf->revents = 0;

    pi->fd = fd;
    pi->p = p;
    pi->socktype = socktype;
    pi->port_acl = port_acl;
    pi->flags = flags;
    pi->next = NULL;
    pi->client_ip   = strdupz(client_ip);
    pi->client_port = strdupz(client_port);
    pi->client_host = strdupz(client_host);

    pi->del_callback = del_callback;
    pi->rcv_callback = rcv_callback;
    pi->snd_callback = snd_callback;

    pi->connected_t = now_boottime_sec();
    pi->last_received_t = 0;
    pi->last_sent_t = 0;
    pi->last_sent_t = 0;
    pi->recv_count = 0;
    pi->send_count = 0;

    p->used++;
    if(unlikely(pi->slot > p->max))
        p->max = pi->slot;

    if(pi->flags & POLLINFO_FLAG_CLIENT_SOCKET) {
        pi->data = add_callback(pi, &pf->events, data);
    }

    if(pi->flags & POLLINFO_FLAG_SERVER_SOCKET) {
        p->min = pi->slot;
    }

    return pi;
}

inline void poll_close_fd(POLLINFO *pi) {
    POLLJOB *p = pi->p;

    struct pollfd *pf = &p->fds[pi->slot];

    if(unlikely(pf->fd == -1)) return;

    if(pi->flags & POLLINFO_FLAG_CLIENT_SOCKET) {
        pi->del_callback(pi);

        if(likely(!(pi->flags & POLLINFO_FLAG_DONT_CLOSE))) {
            if(close(pf->fd) == -1)
                nd_log(NDLS_DAEMON, NDLP_ERR,
                       "Failed to close() poll_events() socket %d",
                       pf->fd);
        }
    }

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

    freez(pi->client_host);
    pi->client_host = NULL;

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
}

void *poll_default_add_callback(POLLINFO *pi, short int *events, void *data) {
    (void)pi;
    (void)events;
    (void)data;

    return NULL;
}

void poll_default_del_callback(POLLINFO *pi) {
    if(pi->data)
        nd_log(NDLS_DAEMON, NDLP_ERR,
               "POLLFD: internal error: del_callback_default() called with data pointer - possible memory leak");
}

int poll_default_rcv_callback(POLLINFO *pi, short int *events) {
    *events |= POLLIN;

    char buffer[1024 + 1];

    ssize_t rc;
    do {
        rc = recv(pi->fd, buffer, 1024, MSG_DONTWAIT);
        if (rc < 0) {
            // read failed
            if (errno != EWOULDBLOCK && errno != EAGAIN) {
                nd_log(NDLS_DAEMON, NDLP_ERR,
                       "POLLFD: poll_default_rcv_callback(): recv() failed with %zd.",
                       rc);

                return -1;
            }
        } else if (rc) {
            // data received
            nd_log(NDLS_DAEMON, NDLP_WARNING,
                   "POLLFD: internal error: poll_default_rcv_callback() is discarding %zd bytes received on socket %d",
                   rc, pi->fd);
        }
    } while (rc != -1);

    return 0;
}

int poll_default_snd_callback(POLLINFO *pi, short int *events) {
    *events &= ~POLLOUT;

    nd_log(NDLS_DAEMON, NDLP_WARNING,
           "POLLFD: internal error: poll_default_snd_callback(): nothing to send on socket %d",
           pi->fd);

    return 0;
}

void poll_default_tmr_callback(void *timer_data) {
    (void)timer_data;
}

static void poll_events_cleanup(void *pptr) {
    POLLJOB *p = CLEANUP_FUNCTION_GET_PTR(pptr);
    if(!p) return;

    for(size_t i = 0 ; i <= p->max ; i++) {
        POLLINFO *pi = &p->inf[i];
        poll_close_fd(pi);
    }

    freez(p->fds);
    freez(p->inf);
}

static int poll_process_error(POLLINFO *pi, struct pollfd *pf, short int revents) {
    ND_LOG_STACK lgs[] = {
        ND_LOG_FIELD_TXT(NDF_SRC_IP, pi->client_ip),
        ND_LOG_FIELD_TXT(NDF_SRC_PORT, pi->client_port),
        ND_LOG_FIELD_END(),
    };
    ND_LOG_STACK_PUSH(lgs);

    nd_log(NDLS_DAEMON, NDLP_DEBUG,
           "POLLFD: LISTENER: received %s %s %s on socket at slot %zu (fd %d) client '%s' port '%s' expecting %s %s %s, having %s %s %s"
           , revents & POLLERR  ? "POLLERR" : ""
           , revents & POLLHUP  ? "POLLHUP" : ""
           , revents & POLLNVAL ? "POLLNVAL" : ""
           , pi->slot
           , pi->fd
           , pi->client_ip ? pi->client_ip : "<undefined-ip>"
           , pi->client_port ? pi->client_port : "<undefined-port>"
           , pf->events & POLLIN ? "POLLIN" : "", pf->events & POLLOUT ? "POLLOUT" : "", pf->events & POLLPRI ? "POLLPRI" : ""
           , revents & POLLIN ? "POLLIN" : "", revents & POLLOUT ? "POLLOUT" : "", revents & POLLPRI ? "POLLPRI" : ""
    );

    pf->events = 0;
    poll_close_fd(pi);
    return 1;
}

static inline int poll_process_send(POLLJOB *p, POLLINFO *pi, struct pollfd *pf, time_t now) {
    pi->last_sent_t = now;
    pi->send_count++;

    pf->events = 0;

    // remember the slot, in case we need to close it later
    // the callback may manipulate the socket list and our pf and pi pointers may be invalid after that call
    size_t slot = pi->slot;

    if (unlikely(pi->snd_callback(pi, &pf->events) == -1))
        poll_close_fd(&p->inf[slot]);

    // IMPORTANT:
    // pf and pi may be invalid below this point, they may have been reallocated.

    return 1;
}

static inline int poll_process_tcp_read(POLLJOB *p, POLLINFO *pi, struct pollfd *pf, time_t now) {
    pi->last_received_t = now;
    pi->recv_count++;

    pf->events = 0;

    // remember the slot, in case we need to close it later
    // the callback may manipulate the socket list and our pf and pi pointers may be invalid after that call
    size_t slot = pi->slot;

    if (pi->rcv_callback(pi, &pf->events) == -1)
        poll_close_fd(&p->inf[slot]);

    // IMPORTANT:
    // pf and pi may be invalid below this point, they may have been reallocated.

    return 1;
}

static inline int poll_process_udp_read(POLLINFO *pi, struct pollfd *pf, time_t now __maybe_unused) {
    pi->last_received_t = now;
    pi->recv_count++;

    // TODO: access_list is not applied to UDP
    // but checking the access list on every UDP packet will destroy
    // performance, especially for statsd.

    pf->events = 0;
    if(pi->rcv_callback(pi, &pf->events) == -1)
        return 0;

    // IMPORTANT:
    // pf and pi may be invalid below this point, they may have been reallocated.

    return 1;
}

static int poll_process_new_tcp_connection(POLLJOB *p, POLLINFO *pi, struct pollfd *pf, time_t now) {
    pi->last_received_t = now;
    pi->recv_count++;

    char client_ip[INET6_ADDRSTRLEN] = "";
    char client_port[NI_MAXSERV] = "";
    char client_host[NI_MAXHOST] = "";

#ifdef SOCK_NONBLOCK
    int flags = SOCK_NONBLOCK;
#else
    int flags = 0;
#endif

    int nfd = accept_socket(
        pf->fd, flags,
        client_ip, INET6_ADDRSTRLEN, client_port,NI_MAXSERV, client_host, NI_MAXHOST,
        p->access_list, p->allow_dns
    );

#ifndef SOCK_NONBLOCK
    if (nfd > 0) {
        int flags = fcntl(nfd, F_GETFL);
        (void)fcntl(nfd, F_SETFL, flags| O_NONBLOCK);
    }
#endif

    if (unlikely(nfd < 0)) {
        // accept failed

        if(unlikely(errno == EMFILE)) {
            nd_log_limit_static_global_var(erl, 10, 1000);
            nd_log_limit(&erl, NDLS_DAEMON, NDLP_ERR,
                         "POLLFD: LISTENER: too many open files - used by this thread %zu, max for this thread %zu",
                         p->used, p->limit);
        }
        else if(unlikely(errno != EWOULDBLOCK && errno != EAGAIN))
            nd_log(NDLS_DAEMON, NDLP_ERR,
                   "POLLFD: LISTENER: accept() failed.");

    }
    else {
        // accept ok

        poll_add_fd(p
                    , nfd
                    , SOCK_STREAM
                    , pi->port_acl
                    , POLLINFO_FLAG_CLIENT_SOCKET
                    , client_ip
                    , client_port
                    , client_host
                    , p->add_callback
                    , p->del_callback
                    , p->rcv_callback
                    , p->snd_callback
                    , NULL
        );

        // IMPORTANT:
        // pf and pi may be invalid below this point, they may have been reallocated.

        return 1;
    }

    return 0;
}

void poll_events(LISTEN_SOCKETS *sockets
                 , void *(*add_callback)(POLLINFO * /*pi*/, short int * /*events*/, void * /*data*/)
                     , void  (*del_callback)(POLLINFO * /*pi*/)
                     , int   (*rcv_callback)(POLLINFO * /*pi*/, short int * /*events*/)
                     , int   (*snd_callback)(POLLINFO * /*pi*/, short int * /*events*/)
                     , void  (*tmr_callback)(void * /*timer_data*/)
                     , bool  (*check_to_stop_callback)(void)
                     , SIMPLE_PATTERN *access_list
                 , int allow_dns
                 , void *data
                 , time_t tcp_request_timeout_seconds
                 , time_t tcp_idle_timeout_seconds
                 , time_t timer_milliseconds
                 , void *timer_data
                 , size_t max_tcp_sockets
) {
    if(!sockets || !sockets->opened) {
        nd_log(NDLS_DAEMON, NDLP_ERR,
               "POLLFD: internal error: no listening sockets are opened");
        return;
    }

    if(timer_milliseconds <= 0) timer_milliseconds = 0;

    int retval;

    POLLJOB p = {
        .slots = 0,
        .used = 0,
        .max = 0,
        .limit = max_tcp_sockets,
        .fds = NULL,
        .inf = NULL,
        .first_free = NULL,

        .complete_request_timeout = tcp_request_timeout_seconds,
        .idle_timeout = tcp_idle_timeout_seconds,
        .checks_every = (tcp_idle_timeout_seconds / 3) + 1,

        .access_list = access_list,
        .allow_dns   = allow_dns,

        .timer_milliseconds = timer_milliseconds,
        .timer_data = timer_data,

        .add_callback = add_callback?add_callback:poll_default_add_callback,
        .del_callback = del_callback?del_callback:poll_default_del_callback,
        .rcv_callback = rcv_callback?rcv_callback:poll_default_rcv_callback,
        .snd_callback = snd_callback?snd_callback:poll_default_snd_callback,
        .tmr_callback = tmr_callback?tmr_callback:poll_default_tmr_callback
    };

    size_t i;
    for(i = 0; i < sockets->opened ;i++) {

        POLLINFO *pi = poll_add_fd(&p
                                   , sockets->fds[i]
                                   , sockets->fds_types[i]
                                   , sockets->fds_acl_flags[i]
                                   , POLLINFO_FLAG_SERVER_SOCKET
                                   , (sockets->fds_names[i])?sockets->fds_names[i]:"UNKNOWN"
                                   , ""
                                   , ""
                                   , p.add_callback
                                   , p.del_callback
                                   , p.rcv_callback
                                   , p.snd_callback
                                   , NULL
        );

        pi->data = data;
        nd_log(NDLS_DAEMON, NDLP_DEBUG,
               "POLLFD: LISTENER: listening on '%s'",
               (sockets->fds_names[i])?sockets->fds_names[i]:"UNKNOWN");
    }

    int listen_sockets_active = 1;

    time_t last_check = now_boottime_sec();

    usec_t timer_usec = timer_milliseconds * USEC_PER_MS;
    usec_t now_usec = 0, next_timer_usec = 0, last_timer_usec = 0;
    (void)last_timer_usec;

    if(unlikely(timer_usec)) {
        now_usec = now_boottime_usec();
        next_timer_usec = now_usec - (now_usec % timer_usec) + timer_usec;
    }

    CLEANUP_FUNCTION_REGISTER(poll_events_cleanup) cleanup_ptr = &p;

    while(!check_to_stop_callback() && !nd_thread_signaled_to_cancel()) {
        if(unlikely(timer_usec)) {
            now_usec = now_boottime_usec();

            if(unlikely(timer_usec && now_usec >= next_timer_usec)) {
                last_timer_usec = now_usec;
                p.tmr_callback(p.timer_data);
                now_usec = now_boottime_usec();
                next_timer_usec = now_usec - (now_usec % timer_usec) + timer_usec;
            }
        }

        // enable or disable the TCP listening sockets, based on the current number of sockets used and the limit set
        if((listen_sockets_active && (p.limit && p.used >= p.limit)) || (!listen_sockets_active && (!p.limit || p.used < p.limit))) {
            listen_sockets_active = !listen_sockets_active;

            nd_log(NDLS_DAEMON, NDLP_DEBUG,
                   "%s listening sockets (used TCP sockets %zu, max allowed for this worker %zu)",
                   (listen_sockets_active)?"ENABLING":"DISABLING", p.used, p.limit);

            for (i = 0; i <= p.max; i++) {
                if(p.inf[i].flags & POLLINFO_FLAG_SERVER_SOCKET && p.inf[i].socktype == SOCK_STREAM) {
                    p.fds[i].events = (short int) ((listen_sockets_active) ? POLLIN : 0);
                }
            }
        }

        retval = poll(p.fds, p.max + 1, ND_CHECK_CANCELLABILITY_WHILE_WAITING_EVERY_MS);
        time_t now = now_boottime_sec();

        if(unlikely(retval == -1)) {
            nd_log(NDLS_DAEMON, NDLP_ERR,
                   "POLLFD: LISTENER: poll() failed while waiting on %zu sockets.",
                   p.max + 1);

            break;
        }
        else if(unlikely(!retval)) {
            // timeout
            ;
        }
        else {
            POLLINFO *pi;
            struct pollfd *pf;
            size_t idx, processed = 0;
            short int revents;

            // keep fast lookup arrays per function
            // to avoid looping through the entire list every time
            size_t sends[p.max + 1], sends_max = 0;
            size_t reads[p.max + 1], reads_max = 0;
            size_t conns[p.max + 1], conns_max = 0;
            size_t udprd[p.max + 1], udprd_max = 0;

            for (i = 0; i <= p.max; i++) {
                pi = &p.inf[i];
                pf = &p.fds[i];
                revents = pf->revents;

                if(unlikely(revents == 0 || pf->fd == -1))
                    continue;

                if (unlikely(revents & (POLLERR|POLLHUP|POLLNVAL))) {
                    // something is wrong to one of our sockets

                    pf->revents = 0;
                    processed += poll_process_error(pi, pf, revents);
                }
                else if (likely(revents & POLLOUT)) {
                    // a client is ready to receive data

                    sends[sends_max++] = i;
                }
                else if (likely(revents & (POLLIN|POLLPRI))) {
                    if (pi->flags & POLLINFO_FLAG_CLIENT_SOCKET) {
                        // a client sent data to us

                        reads[reads_max++] = i;
                    }
                    else if (pi->flags & POLLINFO_FLAG_SERVER_SOCKET) {
                        // something is coming to our server sockets

                        if(pi->socktype == SOCK_DGRAM) {
                            // UDP receive, directly on our listening socket

                            udprd[udprd_max++] = i;
                        }
                        else if(pi->socktype == SOCK_STREAM) {
                            // new TCP connection

                            conns[conns_max++] = i;
                        }
                        else
                            nd_log(NDLS_DAEMON, NDLP_ERR,
                                   "POLLFD: LISTENER: server slot %zu (fd %d) connection from %s port %s using unhandled socket type %d."
                                   , i
                                   , pi->fd
                                   , pi->client_ip ? pi->client_ip : "<undefined-ip>"
                                   , pi->client_port ? pi->client_port : "<undefined-port>"
                                   , pi->socktype
                            );
                    }
                    else
                        nd_log(NDLS_DAEMON, NDLP_ERR,
                               "POLLFD: LISTENER: client slot %zu (fd %d) data from %s port %s using flags %08X is neither client nor server."
                               , i
                               , pi->fd
                               , pi->client_ip ? pi->client_ip : "<undefined-ip>"
                               , pi->client_port ? pi->client_port : "<undefined-port>"
                               , pi->flags
                        );
                }
                else
                    nd_log(NDLS_DAEMON, NDLP_ERR,
                           "POLLFD: LISTENER: socket slot %zu (fd %d) client %s port %s unhandled event id %d."
                           , i
                           , pi->fd
                           , pi->client_ip ? pi->client_ip : "<undefined-ip>"
                           , pi->client_port ? pi->client_port : "<undefined-port>"
                           , revents
                    );
            }

            // process sends
            for (idx = 0; idx < sends_max; idx++) {
                i = sends[idx];
                pi = &p.inf[i];
                pf = &p.fds[i];
                pf->revents = 0;
                processed += poll_process_send(&p, pi, pf, now);
            }

            // process UDP reads
            for (idx = 0; idx < udprd_max; idx++) {
                i = udprd[idx];
                pi = &p.inf[i];
                pf = &p.fds[i];
                pf->revents = 0;
                processed += poll_process_udp_read(pi, pf, now);
            }

            // process TCP reads
            for (idx = 0; idx < reads_max; idx++) {
                i = reads[idx];
                pi = &p.inf[i];
                pf = &p.fds[i];
                pf->revents = 0;
                processed += poll_process_tcp_read(&p, pi, pf, now);
            }

            if(!processed && (!p.limit || p.used < p.limit)) {
                // nothing processed above (rcv, snd) and we have room for another TCP connection
                // so, accept one TCP connection
                for (idx = 0; idx < conns_max; idx++) {
                    i = conns[idx];
                    pi = &p.inf[i];
                    pf = &p.fds[i];
                    pf->revents = 0;
                    if (poll_process_new_tcp_connection(&p, pi, pf, now))
                        break;
                }
            }
        }

        if(unlikely(p.checks_every > 0 && now - last_check > p.checks_every)) {
            last_check = now;

            // cleanup old sockets
            for(i = 0; i <= p.max; i++) {
                POLLINFO *pi = &p.inf[i];

                if(likely(pi->flags & POLLINFO_FLAG_CLIENT_SOCKET)) {
                    if (unlikely(pi->send_count == 0 && p.complete_request_timeout > 0 && (now - pi->connected_t) >= p.complete_request_timeout)) {
                        nd_log(NDLS_DAEMON, NDLP_DEBUG,
                               "POLLFD: LISTENER: client slot %zu (fd %d) from %s port %s has not sent a complete request in %zu seconds - closing it. "
                               , i
                               , pi->fd
                               , pi->client_ip ? pi->client_ip : "<undefined-ip>"
                               , pi->client_port ? pi->client_port : "<undefined-port>"
                               , (size_t) p.complete_request_timeout
                        );
                        poll_close_fd(pi);
                    }
                    else if(unlikely(pi->recv_count && p.idle_timeout > 0 && now - ((pi->last_received_t > pi->last_sent_t) ? pi->last_received_t : pi->last_sent_t) >= p.idle_timeout )) {
                        nd_log(NDLS_DAEMON, NDLP_DEBUG,
                               "POLLFD: LISTENER: client slot %zu (fd %d) from %s port %s is idle for more than %zu seconds - closing it. "
                               , i
                               , pi->fd
                               , pi->client_ip ? pi->client_ip : "<undefined-ip>"
                               , pi->client_port ? pi->client_port : "<undefined-port>"
                               , (size_t) p.idle_timeout
                        );
                        poll_close_fd(pi);
                    }
                }
            }
        }
    }
}
