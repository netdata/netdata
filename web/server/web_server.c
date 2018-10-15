// SPDX-License-Identifier: GPL-3.0-or-later

#define WEB_SERVER_INTERNALS 1
#include "web_server.h"

// this file includes 3 web servers:
//
// 1. single-threaded, based on select()
// 2. multi-threaded, based on poll() that spawns threads to handle the requests, based on select()
// 3. static-threaded, based on poll() using a fixed number of threads (configured at netdata.conf)

WEB_SERVER_MODE web_server_mode = WEB_SERVER_MODE_STATIC_THREADED;

// --------------------------------------------------------------------------------------

WEB_SERVER_MODE web_server_mode_id(const char *mode) {
    if(!strcmp(mode, "none"))
        return WEB_SERVER_MODE_NONE;
    else if(!strcmp(mode, "single") || !strcmp(mode, "single-threaded"))
        return WEB_SERVER_MODE_SINGLE_THREADED;
    else if(!strcmp(mode, "static") || !strcmp(mode, "static-threaded"))
        return WEB_SERVER_MODE_STATIC_THREADED;
    else // if(!strcmp(mode, "multi") || !strcmp(mode, "multi-threaded"))
        return WEB_SERVER_MODE_MULTI_THREADED;
}

const char *web_server_mode_name(WEB_SERVER_MODE id) {
    switch(id) {
        case WEB_SERVER_MODE_NONE:
            return "none";

        case WEB_SERVER_MODE_SINGLE_THREADED:
            return "single-threaded";

        case WEB_SERVER_MODE_STATIC_THREADED:
            return "static-threaded";

        default:
        case WEB_SERVER_MODE_MULTI_THREADED:
            return "multi-threaded";
    }
}

// --------------------------------------------------------------------------------------
// API sockets

LISTEN_SOCKETS api_sockets = {
        .config_section  = CONFIG_SECTION_WEB,
        .default_bind_to = "*",
        .default_port    = API_LISTEN_PORT,
        .backlog         = API_LISTEN_BACKLOG
};

int api_listen_sockets_setup(void) {
    int socks = listen_sockets_setup(&api_sockets);

    if(!socks)
        fatal("LISTENER: Cannot listen on any API socket. Exiting...");

    return socks;
}


// --------------------------------------------------------------------------------------
// access lists

SIMPLE_PATTERN *web_allow_connections_from = NULL;
SIMPLE_PATTERN *web_allow_streaming_from = NULL;
SIMPLE_PATTERN *web_allow_netdataconf_from = NULL;

// WEB_CLIENT_ACL
SIMPLE_PATTERN *web_allow_dashboard_from = NULL;
SIMPLE_PATTERN *web_allow_registry_from = NULL;
SIMPLE_PATTERN *web_allow_badges_from = NULL;

void web_client_update_acl_matches(struct web_client *w) {
    w->acl = WEB_CLIENT_ACL_NONE;

    if(!web_allow_dashboard_from || simple_pattern_matches(web_allow_dashboard_from, w->client_ip))
        w->acl |= WEB_CLIENT_ACL_DASHBOARD;

    if(!web_allow_registry_from || simple_pattern_matches(web_allow_registry_from, w->client_ip))
        w->acl |= WEB_CLIENT_ACL_REGISTRY;

    if(!web_allow_badges_from || simple_pattern_matches(web_allow_badges_from, w->client_ip))
        w->acl |= WEB_CLIENT_ACL_BADGE;
}


// --------------------------------------------------------------------------------------

void web_server_log_connection(struct web_client *w, const char *msg) {
    log_access("%llu: %d '[%s]:%s' '%s'", w->id, gettid(), w->client_ip, w->client_port, msg);
}

// --------------------------------------------------------------------------------------

void web_client_initialize_connection(struct web_client *w) {
    int flag = 1;

    if(unlikely(web_client_check_tcp(w) && setsockopt(w->ifd, IPPROTO_TCP, TCP_NODELAY, (char *) &flag, sizeof(int)) != 0))
        debug(D_WEB_CLIENT, "%llu: failed to enable TCP_NODELAY on socket fd %d.", w->id, w->ifd);

    flag = 1;
    if(unlikely(setsockopt(w->ifd, SOL_SOCKET, SO_KEEPALIVE, (char *) &flag, sizeof(int)) != 0))
        debug(D_WEB_CLIENT, "%llu: failed to enable SO_KEEPALIVE on socket fd %d.", w->id, w->ifd);

    web_client_update_acl_matches(w);

    w->origin[0] = '*'; w->origin[1] = '\0';
    w->cookie1[0] = '\0'; w->cookie2[0] = '\0';
    freez(w->user_agent); w->user_agent = NULL;

    web_client_enable_wait_receive(w);

    web_server_log_connection(w, "CONNECTED");

    web_client_cache_verify(0);
}

struct web_client *web_client_create_on_listenfd(int listener) {
    struct web_client *w;

    w = web_client_get_from_cache_or_allocate();
    w->ifd = w->ofd = accept_socket(listener, SOCK_NONBLOCK, w->client_ip, sizeof(w->client_ip), w->client_port, sizeof(w->client_port), web_allow_connections_from);

    if(unlikely(!*w->client_ip))   strcpy(w->client_ip,   "-");
    if(unlikely(!*w->client_port)) strcpy(w->client_port, "-");

    if (w->ifd == -1) {
        if(errno == EPERM)
            web_server_log_connection(w, "ACCESS DENIED");
        else {
            web_server_log_connection(w, "CONNECTION FAILED");
            error("%llu: Failed to accept new incoming connection.", w->id);
        }

        web_client_release(w);
        return NULL;
    }

    web_client_initialize_connection(w);
    return(w);
}

