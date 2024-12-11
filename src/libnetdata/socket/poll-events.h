// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_POLL_EVENTS_H
#define NETDATA_POLL_EVENTS_H

#define POLLINFO_FLAG_SERVER_SOCKET 0x00000001
#define POLLINFO_FLAG_CLIENT_SOCKET 0x00000002
#define POLLINFO_FLAG_DONT_CLOSE    0x00000004

typedef struct poll POLLJOB;

typedef struct pollinfo {
    POLLJOB *p;             // the parent
    size_t slot;            // the slot id

    int fd;                 // the file descriptor
    int socktype;           // the client socket type
    HTTP_ACL port_acl; // the access lists permitted on this web server port (it's -1 for client sockets)
    char *client_ip;         // Max INET6_ADDRSTRLEN bytes
    char *client_port;       // Max NI_MAXSERV bytes
    char *client_host;       // Max NI_MAXHOST bytes

    time_t connected_t;     // the time the socket connected
    time_t last_received_t; // the time the socket last received data
    time_t last_sent_t;     // the time the socket last sent data

    size_t recv_count;      // the number of times the socket was ready for inbound traffic
    size_t send_count;      // the number of times the socket was ready for outbound traffic

    uint32_t flags;         // internal flags

    // callbacks for this socket
    void  (*del_callback)(struct pollinfo *pi);
    int   (*rcv_callback)(struct pollinfo *pi, short int *events);
    int   (*snd_callback)(struct pollinfo *pi, short int *events);

    // the user data
    void *data;

    // linking of free pollinfo structures
    // for quickly finding the next available
    // this is like a stack, it grows and shrinks
    // (with gaps - lower empty slots are preferred)
    struct pollinfo *next;
} POLLINFO;

struct poll {
    size_t slots;
    size_t used;
    size_t min;
    size_t max;

    size_t limit;

    time_t complete_request_timeout;
    time_t idle_timeout;
    time_t checks_every;

    time_t timer_milliseconds;
    void *timer_data;

    struct pollfd *fds;
    struct pollinfo *inf;
    struct pollinfo *first_free;

    SIMPLE_PATTERN *access_list;
    int allow_dns;

    void *(*add_callback)(POLLINFO *pi, short int *events, void *data);
    void  (*del_callback)(POLLINFO *pi);
    int   (*rcv_callback)(POLLINFO *pi, short int *events);
    int   (*snd_callback)(POLLINFO *pi, short int *events);
    void  (*tmr_callback)(void *timer_data);
};

#define pollinfo_from_slot(p, slot) (&((p)->inf[(slot)]))

int poll_default_snd_callback(POLLINFO *pi, short int *events);
int poll_default_rcv_callback(POLLINFO *pi, short int *events);
void poll_default_del_callback(POLLINFO *pi);
void *poll_default_add_callback(POLLINFO *pi, short int *events, void *data);

POLLINFO *poll_add_fd(POLLJOB *p
                      , int fd
                      , int socktype
                      , HTTP_ACL port_acl
                      , uint32_t flags
                      , const char *client_ip
                      , const char *client_port
                      , const char *client_host
                      , void *(*add_callback)(POLLINFO *pi, short int *events, void *data)
                          , void  (*del_callback)(POLLINFO *pi)
                          , int   (*rcv_callback)(POLLINFO *pi, short int *events)
                          , int   (*snd_callback)(POLLINFO *pi, short int *events)
                          , void *data
);
void poll_close_fd(POLLINFO *pi);

void poll_events(LISTEN_SOCKETS *sockets
                 , void *(*add_callback)(POLLINFO *pi, short int *events, void *data)
                     , void  (*del_callback)(POLLINFO *pi)
                     , int   (*rcv_callback)(POLLINFO *pi, short int *events)
                     , int   (*snd_callback)(POLLINFO *pi, short int *events)
                     , void  (*tmr_callback)(void *timer_data)
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
