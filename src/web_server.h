#ifndef NETDATA_WEB_SERVER_H
#define NETDATA_WEB_SERVER_H 1

#define WEB_PATH_FILE               "file"
#define WEB_PATH_DATA               "data"
#define WEB_PATH_DATASOURCE         "datasource"
#define WEB_PATH_GRAPH              "graph"

#ifndef API_LISTEN_PORT
#define API_LISTEN_PORT 19999
#endif

#ifndef API_LISTEN_BACKLOG
#define API_LISTEN_BACKLOG 4096
#endif

typedef enum web_server_mode {
    WEB_SERVER_MODE_SINGLE_THREADED,
    WEB_SERVER_MODE_STATIC_THREADED,
    WEB_SERVER_MODE_MULTI_THREADED,
    WEB_SERVER_MODE_NONE
} WEB_SERVER_MODE;

extern WEB_SERVER_MODE web_server_mode;
extern int web_server_is_multithreaded;

extern WEB_SERVER_MODE web_server_mode_id(const char *mode);
extern const char *web_server_mode_name(WEB_SERVER_MODE id);

extern void *socket_listen_main_multi_threaded(void *ptr);
extern void *socket_listen_main_single_threaded(void *ptr);
extern void *socket_listen_main_static_threaded(void *ptr);
extern int api_listen_sockets_setup(void);

#endif /* NETDATA_WEB_SERVER_H */
