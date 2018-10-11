// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_SOCKET_H
#define NETDATA_SOCKET_H

#include "libnetdata.h"

#ifndef MAX_LISTEN_FDS
#define MAX_LISTEN_FDS 50
#endif

typedef struct listen_sockets {
    const char *config_section;         // the netdata configuration section to read settings from
    const char *default_bind_to;        // the default bind to configuration string
    uint16_t default_port;              // the default port to use
    int backlog;                        // the default listen backlog to use

    size_t opened;                      // the number of sockets opened
    size_t failed;                      // the number of sockets attempted to open, but failed
    int fds[MAX_LISTEN_FDS];            // the open sockets
    char *fds_names[MAX_LISTEN_FDS];    // descriptions for the open sockets
    int fds_types[MAX_LISTEN_FDS];      // the socktype for the open sockets (SOCK_STREAM, SOCK_DGRAM)
    int fds_families[MAX_LISTEN_FDS];   // the family of the open sockets (AF_UNIX, AF_INET, AF_INET6)
} LISTEN_SOCKETS;

extern char *strdup_client_description(int family, const char *protocol, const char *ip, uint16_t port);

extern int listen_sockets_setup(LISTEN_SOCKETS *sockets);
extern void listen_sockets_close(LISTEN_SOCKETS *sockets);

extern int connect_to_this(const char *definition, int default_port, struct timeval *timeout);
extern int connect_to_one_of(const char *destination, int default_port, struct timeval *timeout, size_t *reconnects_counter, char *connected_to, size_t connected_to_size);

extern ssize_t recv_timeout(int sockfd, void *buf, size_t len, int flags, int timeout);
extern ssize_t send_timeout(int sockfd, void *buf, size_t len, int flags, int timeout);

extern int sock_setnonblock(int fd);
extern int sock_delnonblock(int fd);
extern int sock_setreuse(int fd, int reuse);
extern int sock_setreuse_port(int fd, int reuse);
extern int sock_enlarge_in(int fd);
extern int sock_enlarge_out(int fd);

extern int accept_socket(int fd, int flags, char *client_ip, size_t ipsize, char *client_port, size_t portsize, SIMPLE_PATTERN *access_list);

#ifndef HAVE_ACCEPT4
extern int accept4(int sock, struct sockaddr *addr, socklen_t *addrlen, int flags);

#ifndef SOCK_NONBLOCK
#define SOCK_NONBLOCK 00004000
#endif  /* #ifndef SOCK_NONBLOCK */

#ifndef SOCK_CLOEXEC
#define SOCK_CLOEXEC 02000000
#endif /* #ifndef SOCK_CLOEXEC */

#endif /* #ifndef HAVE_ACCEPT4 */


// ----------------------------------------------------------------------------
// poll() based listener

#define POLLINFO_FLAG_SERVER_SOCKET 0x00000001
#define POLLINFO_FLAG_CLIENT_SOCKET 0x00000002
#define POLLINFO_FLAG_DONT_CLOSE    0x00000004

typedef struct poll POLLJOB;

typedef struct pollinfo {
    POLLJOB *p;             // the parent
    size_t slot;            // the slot id

    int fd;                 // the file descriptor
    int socktype;           // the client socket type
    char *client_ip;        // the connected client IP
    char *client_port;      // the connected client port

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

    void *(*add_callback)(POLLINFO *pi, short int *events, void *data);
    void  (*del_callback)(POLLINFO *pi);
    int   (*rcv_callback)(POLLINFO *pi, short int *events);
    int   (*snd_callback)(POLLINFO *pi, short int *events);
    void  (*tmr_callback)(void *timer_data);
};

#define pollinfo_from_slot(p, slot) (&((p)->inf[(slot)]))

extern int poll_default_snd_callback(POLLINFO *pi, short int *events);
extern int poll_default_rcv_callback(POLLINFO *pi, short int *events);
extern void poll_default_del_callback(POLLINFO *pi);
extern void *poll_default_add_callback(POLLINFO *pi, short int *events, void *data);

extern POLLINFO *poll_add_fd(POLLJOB *p
                             , int fd
                             , int socktype
                             , uint32_t flags
                             , const char *client_ip
                             , const char *client_port
                             , void *(*add_callback)(POLLINFO *pi, short int *events, void *data)
                             , void  (*del_callback)(POLLINFO *pi)
                             , int   (*rcv_callback)(POLLINFO *pi, short int *events)
                             , int   (*snd_callback)(POLLINFO *pi, short int *events)
                             , void *data
);
extern void poll_close_fd(POLLINFO *pi);

extern void poll_events(LISTEN_SOCKETS *sockets
        , void *(*add_callback)(POLLINFO *pi, short int *events, void *data)
        , void  (*del_callback)(POLLINFO *pi)
        , int   (*rcv_callback)(POLLINFO *pi, short int *events)
        , int   (*snd_callback)(POLLINFO *pi, short int *events)
        , void  (*tmr_callback)(void *timer_data)
        , SIMPLE_PATTERN *access_list
        , void *data
        , time_t tcp_request_timeout_seconds
        , time_t tcp_idle_timeout_seconds
        , time_t timer_milliseconds
        , void *timer_data
        , size_t max_tcp_sockets
);

#endif //NETDATA_SOCKET_H
