// SPDX-License-Identifier: GPL-3.0-or-later

#define WEB_SERVER_INTERNALS 1
#include "web_server.h"

WEB_SERVER_MODE web_server_mode = WEB_SERVER_MODE_STATIC_THREADED;

// --------------------------------------------------------------------------------------

WEB_SERVER_MODE web_server_mode_id(const char *mode) {
    if(!strcmp(mode, "none"))
        return WEB_SERVER_MODE_NONE;
    else
        return WEB_SERVER_MODE_STATIC_THREADED;

}

const char *web_server_mode_name(WEB_SERVER_MODE id) {
    switch(id) {
        case WEB_SERVER_MODE_NONE:
            return "none";
        default:
        case WEB_SERVER_MODE_STATIC_THREADED:
            return "static-threaded";
    }
}

// --------------------------------------------------------------------------------------
// API sockets

LISTEN_SOCKETS api_sockets = {
		.config          = &netdata_config,
		.config_section  = CONFIG_SECTION_WEB,
		.default_bind_to = "*",
		.default_port    = API_LISTEN_PORT,
		.backlog         = API_LISTEN_BACKLOG
};

// Return the default port to listen on.
uint16_t web_server_get_default_port(void) {
    return api_sockets.default_port;
}

void debug_sockets() {
	BUFFER *wb = buffer_create(256 * sizeof(char), NULL);
	int i;

	for(i = 0 ; i < (int)api_sockets.opened ; i++) {
		buffer_strcat(wb, (api_sockets.fds_acl_flags[i] & HTTP_ACL_NOCHECK) ? "NONE " : "");
		buffer_strcat(wb, (api_sockets.fds_acl_flags[i] & HTTP_ACL_DASHBOARD) ? "dashboard " : "");
		buffer_strcat(wb, (api_sockets.fds_acl_flags[i] & HTTP_ACL_REGISTRY) ? "registry " : "");
		buffer_strcat(wb, (api_sockets.fds_acl_flags[i] & HTTP_ACL_BADGES) ? "badges " : "");
		buffer_strcat(wb, (api_sockets.fds_acl_flags[i] & HTTP_ACL_MANAGEMENT) ? "management " : "");
		buffer_strcat(wb, (api_sockets.fds_acl_flags[i] & HTTP_ACL_STREAMING) ? "streaming " : "");
		buffer_strcat(wb, (api_sockets.fds_acl_flags[i] & HTTP_ACL_NETDATACONF) ? "netdata.conf " : "");
        netdata_log_debug(D_WEB_CLIENT, "Socket fd %d name '%s' acl_flags: %s",
			  i,
			  api_sockets.fds_names[i],
			  buffer_tostring(wb));
		buffer_reset(wb);
	}
	buffer_free(wb);
}

void web_server_listen_sockets_setup(void) {
    errno_clear();

	int socks = listen_sockets_setup(&api_sockets);
	if(!socks) {
        exit_initiated_add(EXIT_REASON_ALREADY_RUNNING);
        daemon_status_file_update_status(DAEMON_STATUS_NONE);
        fatal("Cannot setup listen port(s). Is Netdata already running?");
    }

	if(unlikely(debug_flags & D_WEB_CLIENT))
		debug_sockets();

    websocket_initialize();
}


// --------------------------------------------------------------------------------------
// access lists

SIMPLE_PATTERN *web_allow_connections_from = NULL;
int web_allow_connections_dns;

// WEB_CLIENT_ACL
SIMPLE_PATTERN *web_allow_dashboard_from = NULL;
int             web_allow_dashboard_dns;
SIMPLE_PATTERN *web_allow_registry_from = NULL;
int             web_allow_registry_dns;
SIMPLE_PATTERN *web_allow_badges_from = NULL;
int             web_allow_badges_dns;
SIMPLE_PATTERN *web_allow_mgmt_from = NULL;
int             web_allow_mgmt_dns;
SIMPLE_PATTERN *web_allow_streaming_from = NULL;
int             web_allow_streaming_dns;
SIMPLE_PATTERN *web_allow_netdataconf_from = NULL;
int             web_allow_netdataconf_dns;

void web_client_update_acl_matches(struct web_client *w) {
    w->acl = HTTP_ACL_TRANSPORTS;

    if(!(w->port_acl & HTTP_ACL_TRANSPORTS_WITHOUT_CLIENT_IP_VALIDATION)) {
        if (!web_allow_dashboard_from ||
            connection_allowed(w->fd, w->user_auth.client_ip, w->client_host, sizeof(w->client_host),
                               web_allow_dashboard_from, "dashboard", web_allow_dashboard_dns))
            w->acl |= HTTP_ACL_DASHBOARD;

        if (!web_allow_registry_from ||
            connection_allowed(w->fd, w->user_auth.client_ip, w->client_host, sizeof(w->client_host),
                               web_allow_registry_from, "registry", web_allow_registry_dns))
            w->acl |= HTTP_ACL_REGISTRY;

        if (!web_allow_badges_from ||
            connection_allowed(w->fd, w->user_auth.client_ip, w->client_host, sizeof(w->client_host),
                               web_allow_badges_from, "badges", web_allow_badges_dns))
            w->acl |= HTTP_ACL_BADGES;

        if (!web_allow_mgmt_from ||
            connection_allowed(w->fd, w->user_auth.client_ip, w->client_host, sizeof(w->client_host),
                               web_allow_mgmt_from, "management", web_allow_mgmt_dns))
            w->acl |= HTTP_ACL_MANAGEMENT;

        if (!web_allow_streaming_from ||
            connection_allowed(w->fd, w->user_auth.client_ip, w->client_host, sizeof(w->client_host),
                               web_allow_streaming_from, "streaming", web_allow_streaming_dns))
            w->acl |= HTTP_ACL_STREAMING;

        if (!web_allow_netdataconf_from ||
           connection_allowed(w->fd, w->user_auth.client_ip, w->client_host, sizeof(w->client_host),
                              web_allow_netdataconf_from, "netdata.conf", web_allow_netdataconf_dns))
            w->acl |= HTTP_ACL_NETDATACONF;
    }

    w->acl &= w->port_acl;
}


// --------------------------------------------------------------------------------------

void web_server_log_connection(struct web_client *w, const char *msg) {
    ND_LOG_STACK lgs[] = {
            ND_LOG_FIELD_U64(NDF_CONNECTION_ID, w->id),
            ND_LOG_FIELD_TXT(NDF_SRC_TRANSPORT, SSL_connection(&w->ssl) ? "https" : "http"),
            ND_LOG_FIELD_TXT(NDF_SRC_IP, w->user_auth.client_ip),
            ND_LOG_FIELD_TXT(NDF_SRC_PORT, w->client_port),
            ND_LOG_FIELD_TXT(NDF_SRC_FORWARDED_HOST, w->forwarded_host),
            ND_LOG_FIELD_TXT(NDF_SRC_FORWARDED_FOR, w->user_auth.forwarded_for),
            ND_LOG_FIELD_END(),
    };
    ND_LOG_STACK_PUSH(lgs);

    nd_log(NDLS_ACCESS, NDLP_DEBUG, "[%s]:%s %s", w->user_auth.client_ip, w->client_port, msg);
}
