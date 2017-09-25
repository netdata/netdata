#ifndef NETDATA_SOCKET_H
#define NETDATA_SOCKET_H

#ifndef MAX_LISTEN_FDS
#define MAX_LISTEN_FDS 50
#endif

typedef struct listen_sockets {
    const char *config_section;         // the netdata configuration section to read settings from
    const char *default_bind_to;        // the default bind to configuration string
    int default_port;                   // the default port to use
    int backlog;                        // the default listen backlog to use

    size_t opened;                      // the number of sockets opened
    size_t failed;                      // the number of sockets attempted to open, but failed
    int fds[MAX_LISTEN_FDS];            // the open sockets
    char *fds_names[MAX_LISTEN_FDS];    // descriptions for the open sockets
    int fds_types[MAX_LISTEN_FDS];      // the socktype for the open sockets (SOCK_STREAM, SOCK_DGRAM)
    int fds_families[MAX_LISTEN_FDS];   // the family of the open sockets (AF_UNIX, AF_INET, AF_INET6)
} LISTEN_SOCKETS;

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


extern void poll_events(LISTEN_SOCKETS *sockets
        , void *(*add_callback)(int fd, int socktype, short int *events)
        , void  (*del_callback)(int fd, int socktype, void *data)
        , int   (*rcv_callback)(int fd, int socktype, void *data, short int *events)
        , int   (*snd_callback)(int fd, int socktype, void *data, short int *events)
        , SIMPLE_PATTERN *access_list
        , void *data
);

#endif //NETDATA_SOCKET_H
