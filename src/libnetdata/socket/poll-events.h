// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_POLL_EVENTS_H
#define NETDATA_POLL_EVENTS_H

#include "nd-poll.h"

#define POLLINFO_FLAG_SERVER_SOCKET 0x00000001
#define POLLINFO_FLAG_CLIENT_SOCKET 0x00000002
#define POLLINFO_FLAG_DONT_CLOSE    0x00000004

typedef struct poll POLLJOB;
typedef struct pollinfo POLLINFO;

typedef void *(*poll_events_add_callback_t)(POLLINFO *pi, nd_poll_event_t *events, void *data);
typedef void (*poll_events_del_callback_t)(POLLINFO *pi);
typedef int (*poll_events_rcv_callback_t)(POLLINFO *pi, nd_poll_event_t *events);
typedef int (*poll_events_snd_callback_t)(POLLINFO *pi, nd_poll_event_t *events);
typedef void (*poll_events_tmr_callback_t)(void *timer_data);

struct pollinfo {
    POLLJOB *p;             // the parent

    int fd;                 // the file descriptor
    int socktype;           // the client socket type
    HTTP_ACL port_acl; // the access lists permitted on this web server port (it's -1 for client sockets)
    char *client_ip;         // Max INET6_ADDRSTRLEN bytes
    char *client_port;       // Max NI_MAXSERV bytes
    char *client_host;       // Max NI_MAXHOST bytes

    nd_poll_event_t events;
    nd_poll_event_t events_we_wait_for;

    time_t connected_t;     // the time the socket connected
    time_t last_received_t; // the time the socket last received data
    time_t last_sent_t;     // the time the socket last sent data

    size_t recv_count;      // the number of times the socket was ready for inbound traffic
    size_t send_count;      // the number of times the socket was ready for outbound traffic

    uint32_t flags;         // internal flags

    // callbacks for this socket
    poll_events_del_callback_t del_callback;
    poll_events_rcv_callback_t rcv_callback;
    poll_events_snd_callback_t snd_callback;

    // the user data
    void *data;

    struct pollinfo *prev, *next;
};

struct poll {
    nd_poll_t *ndpl;
    POLLINFO *ll;

    size_t used;
    size_t limit;

    time_t complete_request_timeout;
    time_t idle_timeout;
    time_t checks_every;

    time_t timer_milliseconds;
    void *timer_data;

    SIMPLE_PATTERN *access_list;
    int allow_dns;

    poll_events_add_callback_t add_callback;
    poll_events_del_callback_t del_callback;
    poll_events_rcv_callback_t rcv_callback;
    poll_events_snd_callback_t snd_callback;
    poll_events_tmr_callback_t tmr_callback;
};

int poll_default_snd_callback(POLLINFO *pi, nd_poll_event_t *events);
int poll_default_rcv_callback(POLLINFO *pi, nd_poll_event_t *events);
void poll_default_del_callback(POLLINFO *pi);
void *poll_default_add_callback(POLLINFO *pi, nd_poll_event_t *events, void *data);

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
);

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
);

#endif //NETDATA_POLL_EVENTS_H
