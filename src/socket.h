#ifndef NETDATA_SOCKET_H
#define NETDATA_SOCKET_H

#ifndef MAX_LISTEN_FDS
#define MAX_LISTEN_FDS 50
#endif

typedef struct listen_sockets {
    const char *config_section;         // the netdata configuration section to read settings from
    int default_port;                   // the default port to use
    int backlog;                        // the default listen backlog to use

    size_t opened;                      // the number of sockets opened
    size_t failed;                      // the number of sockets attempted to open, but failed
    int fds[MAX_LISTEN_FDS];            // the open sockets
    char *fds_names[MAX_LISTEN_FDS];    // descriptions for the open sockets
} LISTEN_SOCKETS;

extern int listen_sockets_setup(LISTEN_SOCKETS *sockets);
extern void listen_sockets_close(LISTEN_SOCKETS *sockets);

extern int connect_to(const char *definition, int default_port, struct timeval *timeout);
extern int connect_to_one_of(const char *destination, int default_port, struct timeval *timeout, size_t *reconnects_counter, char *connected_to, size_t connected_to_size);

extern ssize_t recv_timeout(int sockfd, void *buf, size_t len, int flags, int timeout);
extern ssize_t send_timeout(int sockfd, void *buf, size_t len, int flags, int timeout);

#ifndef HAVE_ACCEPT4
extern int accept4(int sock, struct sockaddr *addr, socklen_t *addrlen, int flags);

#ifndef SOCK_NONBLOCK
#define SOCK_NONBLOCK 00004000
#endif  /* #ifndef SOCK_NONBLOCK */

#ifndef SOCK_CLOEXEC
#define SOCK_CLOEXEC 02000000
#endif /* #ifndef SOCK_CLOEXEC */

#endif /* #ifndef HAVE_ACCEPT4 */

#endif //NETDATA_SOCKET_H
