// SPDX-License-Identifier: GPL-3.0-or-later

#include "libnetdata/libnetdata.h"

static inline void poll_process_updated_events(POLLINFO *pi) {
    if(pi->events != pi->events_we_wait_for) {
        if(!nd_poll_upd(pi->p->ndpl, pi->fd, pi->events))
            nd_log(NDLS_DAEMON, NDLP_ERR, "Failed to update socket %d to nd_poll", pi->fd);
        pi->events_we_wait_for = pi->events;
    }
}

// poll() based listener
// this should be the fastest possible listener for up to 100 sockets
// above 100, an epoll() interface is needed on Linux

POLLINFO *poll_add_fd(POLLJOB *p
                      , int fd
                      , int socktype
                      , HTTP_ACL port_acl
                      , uint32_t flags
                      , const char *client_ip
                      , const char *client_port
                      , const char *client_host
                      , poll_events_add_callback_t add_callback
                      , poll_events_del_callback_t del_callback
                      , poll_events_rcv_callback_t rcv_callback
                      , poll_events_snd_callback_t snd_callback
                      , void *data
) {
    if(unlikely(fd < 0)) return NULL;

    //if(p->limit && p->used >= p->limit) {
    //    nd_log(NDLS_DAEMON, NDLP_WARNING, "Max sockets limit reached (%zu sockets), dropping connection", p->used);
    //    close(fd);
    //    return NULL;
    //}

    POLLINFO *pi = callocz(1, sizeof(*pi));

    pi->fd = fd;
    pi->events = ND_POLL_READ;
    pi->p = p;
    pi->socktype = socktype;
    pi->port_acl = port_acl;
    pi->flags = flags;
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

    if(pi->flags & POLLINFO_FLAG_CLIENT_SOCKET)
        pi->data = add_callback(pi, &pi->events, data);

    DOUBLE_LINKED_LIST_APPEND_ITEM_UNSAFE(p->ll, pi, prev, next);
    pi->events_we_wait_for = pi->events;
    if(!nd_poll_add(pi->p->ndpl, pi->fd, pi->events, pi))
        nd_log(NDLS_DAEMON, NDLP_ERR, "Failed to add socket %d to nd_poll", pi->fd);

    return pi;
}

static inline void poll_close_fd(POLLINFO *pi, const char *func) {
    POLLJOB *p = pi->p;

    DOUBLE_LINKED_LIST_REMOVE_ITEM_UNSAFE(p->ll, pi, prev, next);
    if(!nd_poll_del(p->ndpl, pi->fd))
        // this is ok, if the socket is already closed
        nd_log(NDLS_DAEMON, NDLP_DEBUG,
               "Failed to delete socket %d from nd_poll() - called from %s() - is the socket already closed?",
               pi->fd, func);

    if(pi->flags & POLLINFO_FLAG_CLIENT_SOCKET) {
        pi->del_callback(pi);

        if(likely(!(pi->flags & POLLINFO_FLAG_DONT_CLOSE))) {
            if(close(pi->fd) == -1)
                nd_log(NDLS_DAEMON, NDLP_ERR,
                       "Failed to close() poll_events() socket %d",
                       pi->fd);
        }
    }

    freez(pi->client_ip);
    freez(pi->client_port);
    freez(pi->client_host);
    freez(pi);

    p->used--;
}

void *poll_default_add_callback(POLLINFO *pi __maybe_unused, nd_poll_event_t *events __maybe_unused, void *data __maybe_unused) {
    return NULL;
}

void poll_default_del_callback(POLLINFO *pi) {
    if(pi->data)
        nd_log(NDLS_DAEMON, NDLP_ERR,
               "POLLFD: internal error: del_callback_default() called with data pointer - possible memory leak");
}

int poll_default_rcv_callback(POLLINFO *pi, nd_poll_event_t *events) {
    *events |= ND_POLL_READ;

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

int poll_default_snd_callback(POLLINFO *pi, nd_poll_event_t *events) {
    *events &= ~ND_POLL_WRITE;

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

    while(p->ll) {
        POLLINFO *pi = p->ll;
        pi->flags &= ~(POLLINFO_FLAG_DONT_CLOSE);
        poll_close_fd(pi, __FUNCTION__ );
    }

    nd_poll_destroy(p->ndpl);
    p->ndpl = NULL;
}

static int poll_process_error(POLLINFO *pi, nd_poll_event_t revents) {
    ND_LOG_STACK lgs[] = {
        ND_LOG_FIELD_TXT(NDF_SRC_IP, pi->client_ip),
        ND_LOG_FIELD_TXT(NDF_SRC_PORT, pi->client_port),
        ND_LOG_FIELD_END(),
    };
    ND_LOG_STACK_PUSH(lgs);

    nd_log(NDLS_DAEMON, NDLP_DEBUG,
           "POLLFD: LISTENER: received %s %s %s on socket %d client '%s' port '%s' expecting %s %s, having %s %s"
           , revents & ND_POLL_ERROR  ? "ERROR" : ""
           , revents & ND_POLL_HUP  ? "HUP" : ""
           , revents & ND_POLL_INVALID ? "INVALID" : ""
           , pi->fd
           , pi->client_ip ? pi->client_ip : "<undefined-ip>"
           , pi->client_port ? pi->client_port : "<undefined-port>"
           , pi->events & ND_POLL_READ ? "READ" : "", pi->events & ND_POLL_WRITE ? "WRITE" : ""
           , revents & ND_POLL_READ ? "READ" : "", revents & ND_POLL_WRITE ? "WRITE" : ""
    );

    poll_close_fd(pi, __FUNCTION__ );
    return 1;
}

static inline int poll_process_send(POLLINFO *pi, time_t now) {
    pi->last_sent_t = now;
    pi->send_count++;

    pi->events = 0;

    if (unlikely(pi->snd_callback(pi, &pi->events) == -1))
        poll_close_fd(pi, __FUNCTION__ );
    else
        poll_process_updated_events(pi);

    return 1;
}

static inline int poll_process_tcp_read(POLLINFO *pi, time_t now) {
    pi->last_received_t = now;
    pi->recv_count++;

    pi->events = 0;

    if (pi->rcv_callback(pi, &pi->events) == -1)
        poll_close_fd(pi, __FUNCTION__ );
    else
        poll_process_updated_events(pi);

    return 1;
}

static inline int poll_process_udp_read(POLLINFO *pi, time_t now __maybe_unused) {
    pi->last_received_t = now;
    pi->recv_count++;

    // TODO: access_list is not applied to UDP
    // but checking the access list on every UDP packet will destroy
    // performance, especially for statsd.

    pi->events = 0;

    if(pi->rcv_callback(pi, &pi->events) == -1)
        return 0;
    else {
        poll_process_updated_events(pi);
        return 1;
    }
}

static int poll_process_new_tcp_connection(POLLINFO *pi, time_t now) {
    POLLJOB *p = pi->p;

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
        pi->fd, flags,
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
    else if(is_socket_closed(nfd))
        close(nfd);

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

        return 1;
    }

    return 0;
}

void poll_events(LISTEN_SOCKETS *sockets
                 , poll_events_add_callback_t add_callback
                 , poll_events_del_callback_t del_callback
                 , poll_events_rcv_callback_t rcv_callback
                 , poll_events_snd_callback_t snd_callback
                 , poll_events_tmr_callback_t tmr_callback
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
        .ndpl = nd_poll_create(),
        .used = 0,
        .limit = max_tcp_sockets,

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

            for(POLLINFO *pi = p.ll; pi ; pi = pi->next) {
                if((pi->flags & POLLINFO_FLAG_SERVER_SOCKET) && pi->socktype == SOCK_STREAM) {
                    pi->events =  (short int) ((listen_sockets_active) ? ND_POLL_READ : 0);
                    poll_process_updated_events(pi);
                }
            }
        }

        nd_poll_result_t result;
        retval = nd_poll_wait(p.ndpl, ND_CHECK_CANCELLABILITY_WHILE_WAITING_EVERY_MS, &result);
        time_t now = now_boottime_sec();

        if(unlikely(retval == -1)) {
            nd_log(NDLS_DAEMON, NDLP_ERR, "POLLFD: LISTENER: nd_poll_wait() failed.");
            break;
        }
        else if(unlikely(!retval)) {
            // timeout
            ;
        }
        else {
            POLLINFO *pi = (POLLINFO *)result.data;

            if(result.events & (ND_POLL_HUP | ND_POLL_INVALID | ND_POLL_ERROR))
                poll_process_error(pi, result.events);

            else if(result.events & ND_POLL_WRITE) {
                poll_process_send(pi, now);
            }

            else if(result.events & ND_POLL_READ) {
                if (pi->flags & POLLINFO_FLAG_CLIENT_SOCKET) {
                    if (pi->socktype == SOCK_DGRAM)
                        poll_process_udp_read(pi, now);
                    else if (pi->socktype == SOCK_STREAM)
                        poll_process_tcp_read(pi, now);
                    else {
                        nd_log(NDLS_DAEMON, NDLP_ERR,
                               "POLLFD: LISTENER: server slot %zu (fd %d) connection from %s port %s using unhandled socket type %d.",
                               i,
                               pi->fd,
                               pi->client_ip ? pi->client_ip : "<undefined-ip>",
                               pi->client_port ? pi->client_port : "<undefined-port>",
                               pi->socktype);

                        poll_close_fd(pi, "poll_events1");
                    }
                }
                else if (pi->flags & POLLINFO_FLAG_SERVER_SOCKET) {
                    if(pi->socktype == SOCK_DGRAM)
                        poll_process_udp_read(pi, now);

                    else if(pi->socktype == SOCK_STREAM) {
                        if (!p.limit || p.used < p.limit)
                            poll_process_new_tcp_connection(pi, now);
                    }
                    else {
                        nd_log(NDLS_DAEMON, NDLP_ERR,
                               "POLLFD: LISTENER: server slot %zu (fd %d) connection from %s port %s using unhandled socket type %d.",
                               i,
                               pi->fd,
                               pi->client_ip ? pi->client_ip : "<undefined-ip>",
                               pi->client_port ? pi->client_port : "<undefined-port>",
                               pi->socktype);

                        poll_close_fd(pi, "poll_events2");
                    }
                }
                else {
                    nd_log(NDLS_DAEMON, NDLP_ERR,
                           "POLLFD: LISTENER: client slot %zu (fd %d) data from %s port %s using flags %08X is neither client nor server."
                           , i
                           , pi->fd
                           , pi->client_ip ? pi->client_ip : "<undefined-ip>"
                           , pi->client_port ? pi->client_port : "<undefined-port>"
                           , pi->flags
                    );

                    poll_close_fd(pi, "poll_events3");
                }
            }
            else {
                nd_log(NDLS_DAEMON, NDLP_ERR,
                       "POLLFD: LISTENER: socket slot %zu (fd %d) client %s port %s unhandled event id %d."
                       , i
                       , pi->fd
                       , pi->client_ip ? pi->client_ip : "<undefined-ip>"
                       , pi->client_port ? pi->client_port : "<undefined-port>"
                       , (int)result.events
                );

                poll_close_fd(pi, "poll_events4");
            }
        }

        if(unlikely(p.checks_every > 0 && now - last_check > p.checks_every)) {
            last_check = now;

            // cleanup old sockets
            POLLINFO *pi, *next = NULL;
            for(pi = p.ll; pi ; pi = next) {
                next = pi->next;

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
                        poll_close_fd(pi, "poll_events4");
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
                        poll_close_fd(pi, "poll_events5");
                    }
                }
            }
        }
    }
}
