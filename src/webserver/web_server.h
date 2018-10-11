// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_WEB_SERVER_H
#define NETDATA_WEB_SERVER_H 1

#include "../common.h"
#include "web_client.h"

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

extern SIMPLE_PATTERN *web_allow_connections_from;
extern SIMPLE_PATTERN *web_allow_dashboard_from;
extern SIMPLE_PATTERN *web_allow_registry_from;
extern SIMPLE_PATTERN *web_allow_badges_from;
extern SIMPLE_PATTERN *web_allow_streaming_from;
extern SIMPLE_PATTERN *web_allow_netdataconf_from;

extern WEB_SERVER_MODE web_server_mode;

extern WEB_SERVER_MODE web_server_mode_id(const char *mode);
extern const char *web_server_mode_name(WEB_SERVER_MODE id);

extern void *socket_listen_main_multi_threaded(void *ptr);
extern void *socket_listen_main_single_threaded(void *ptr);
extern void *socket_listen_main_static_threaded(void *ptr);
extern int api_listen_sockets_setup(void);

#define DEFAULT_TIMEOUT_TO_RECEIVE_FIRST_WEB_REQUEST 60
#define DEFAULT_DISCONNECT_IDLE_WEB_CLIENTS_AFTER_SECONDS 60
extern int web_client_timeout;
extern int web_client_first_request_timeout;
extern long web_client_streaming_rate_t;

#endif /* NETDATA_WEB_SERVER_H */
