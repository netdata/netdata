#ifndef NETDATA_WEB_SERVER_H
#define NETDATA_WEB_SERVER_H 1

#define WEB_PATH_FILE               "file"
#define WEB_PATH_DATA               "data"
#define WEB_PATH_DATASOURCE         "datasource"
#define WEB_PATH_GRAPH              "graph"

#define LISTEN_PORT 19999
#define LISTEN_BACKLOG 100

#ifndef MAX_LISTEN_FDS
#define MAX_LISTEN_FDS 100
#endif

typedef enum web_server_mode {
    WEB_SERVER_MODE_SINGLE_THREADED,
    WEB_SERVER_MODE_MULTI_THREADED,
    WEB_SERVER_MODE_NONE
} WEB_SERVER_MODE;

extern WEB_SERVER_MODE web_server_mode;

extern WEB_SERVER_MODE web_server_mode_id(const char *mode);
extern const char *web_server_mode_name(WEB_SERVER_MODE id);


extern void *socket_listen_main_multi_threaded(void *ptr);
extern void *socket_listen_main_single_threaded(void *ptr);
extern int create_listen_sockets(void);
extern int is_listen_socket(int fd);

#ifndef HAVE_ACCEPT4
extern int accept4(int sock, struct sockaddr *addr, socklen_t *addrlen, int flags);

#ifndef SOCK_NONBLOCK
#define SOCK_NONBLOCK 00004000
#endif  /* #ifndef SOCK_NONBLOCK */

#ifndef SOCK_CLOEXEC
#define SOCK_CLOEXEC 02000000
#endif /* #ifndef SOCK_CLOEXEC */

#endif /* #ifndef HAVE_ACCEPT4 */

#endif /* NETDATA_WEB_SERVER_H */
