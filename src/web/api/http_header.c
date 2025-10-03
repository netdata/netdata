// SPDX-License-Identifier: GPL-3.0-or-later

#include "http_header.h"

#include <string.h>
#include <strings.h>

static void web_client_enable_deflate(struct web_client *w, bool gzip) {
    if(gzip)
        web_client_flag_set(w, WEB_CLIENT_ENCODING_GZIP);
    else
        web_client_flag_set(w, WEB_CLIENT_ENCODING_DEFLATE);

    if(!web_client_check_conn_unix(w) && !web_client_check_conn_tcp(w) && !web_client_check_conn_cloud(w))
        return;

    if(unlikely(w->response.zinitialized)) {
        // compression has already been initialized for this client.
        return;
    }

    if(unlikely(w->response.sent)) {
        netdata_log_error("%llu: Cannot enable compression in the middle of a conversation.", w->id);
        return;
    }

    w->response.zstream.zalloc = Z_NULL;
    w->response.zstream.zfree = Z_NULL;
    w->response.zstream.opaque = Z_NULL;

    w->response.zstream.next_in = (Bytef *)w->response.data->buffer;
    w->response.zstream.avail_in = 0;
    w->response.zstream.total_in = 0;

    w->response.zstream.next_out = w->response.zbuffer;
    w->response.zstream.avail_out = 0;
    w->response.zstream.total_out = 0;

    w->response.zstream.zalloc = Z_NULL;
    w->response.zstream.zfree = Z_NULL;
    w->response.zstream.opaque = Z_NULL;

    // Select GZIP compression: windowbits = 15 + 16 = 31
    if(deflateInit2(&w->response.zstream, web_gzip_level, Z_DEFLATED, 15 + ((gzip)?16:0), 8, web_gzip_strategy) != Z_OK) {
        netdata_log_error("%llu: Failed to initialize zlib. Proceeding without compression.", w->id);
        return;
    }

    w->response.zsent = 0;
    w->response.zoutput = true;
    w->response.zinitialized = true;

    if(!web_client_check_conn_cloud(w))
        // cloud sends the entire response at once, not in chunks
        web_client_flag_set(w, WEB_CLIENT_CHUNKED_TRANSFER);

    netdata_log_debug(D_DEFLATE, "%llu: Initialized compression.", w->id);
}

static void http_header_origin(struct web_client *w, const char *v, size_t len __maybe_unused) {
    freez(w->origin);
    w->origin = strdupz(v);
}

static void http_header_connection(struct web_client *w, const char *v, size_t len __maybe_unused) {
    if(strcasestr(v, "keep-alive"))
        web_client_enable_keepalive(w);
    
    // Check for WebSocket upgrade request
    if(strcasestr(v, "upgrade"))
        web_client_set_websocket_handshake(w);
}

static void http_header_dnt(struct web_client *w, const char *v, size_t len __maybe_unused) {
    if(respect_web_browser_do_not_track_policy) {
        if (*v == '0') web_client_disable_donottrack(w);
        else if (*v == '1') web_client_enable_donottrack(w);
    }
}

static void http_header_user_agent(struct web_client *w, const char *v, size_t len __maybe_unused) {
    if(w->mode == HTTP_REQUEST_MODE_STREAM) {
        freez(w->user_agent);
        w->user_agent = strdupz(v);
    }
}

static void http_header_accept(struct web_client *w, const char *v, size_t len __maybe_unused) {
    web_client_flag_clear(w, WEB_CLIENT_FLAG_ACCEPT_JSON |
                             WEB_CLIENT_FLAG_ACCEPT_SSE |
                             WEB_CLIENT_FLAG_ACCEPT_TEXT);

    for (const char *p = v; p && *p; ) {
        while (*p == ' ' || *p == '\t' || *p == ',') p++;
        if (!*p)
            break;

        const char *start = p;
        while (*p && *p != ',' && *p != ';')
            p++;
        size_t length = (size_t)(p - start);

        while (*p && *p != ',')
            p++;

        if (length == 0)
            continue;

        if (length >= strlen("application/json") &&
            strncasecmp(start, "application/json", strlen("application/json")) == 0) {
            web_client_flag_set(w, WEB_CLIENT_FLAG_ACCEPT_JSON);
        }
        else if (length >= strlen("text/event-stream") &&
                 strncasecmp(start, "text/event-stream", strlen("text/event-stream")) == 0) {
            web_client_flag_set(w, WEB_CLIENT_FLAG_ACCEPT_SSE);
        }
        else if (length >= strlen("text/plain") &&
                 strncasecmp(start, "text/plain", strlen("text/plain")) == 0) {
            web_client_flag_set(w, WEB_CLIENT_FLAG_ACCEPT_TEXT);
        }
    }
}

static void http_header_x_auth_token(struct web_client *w, const char *v, size_t len __maybe_unused) {
    freez(w->auth_bearer_token);
    w->auth_bearer_token = strdupz(v);
}

static void http_header_host(struct web_client *w, const char *v, size_t len) {
    char buffer[NI_MAXHOST];
    strncpyz(buffer, v, (len < sizeof(buffer) - 1 ? len : sizeof(buffer) - 1));
    freez(w->server_host);
    w->server_host = strdupz(buffer);
}

static void http_header_accept_encoding(struct web_client *w, const char *v, size_t len __maybe_unused) {
    if(web_enable_gzip) {
        if(strcasestr(v, "gzip"))
            web_client_enable_deflate(w, true);

        // does not seem to work
        // else if(strcasestr(v, "deflate"))
        //  web_client_enable_deflate(w, 0);
    }
}

static void http_header_x_forwarded_host(struct web_client *w, const char *v, size_t len) {
    char buffer[NI_MAXHOST];
    strncpyz(buffer, v, (len < sizeof(buffer) - 1 ? len : sizeof(buffer) - 1));
    freez(w->forwarded_host);
    w->forwarded_host = strdupz(buffer);
}

static void http_header_x_forwarded_for(struct web_client *w, const char *v, size_t len) {
    if(len)
        strncpyz(w->user_auth.forwarded_for, v,
                 (len < sizeof(w->user_auth.forwarded_for) - 1 ? len : sizeof(w->user_auth.forwarded_for) - 1));
}

static void http_header_x_transaction_id(struct web_client *w, const char *v, size_t len) {
    char buffer[UUID_STR_LEN * 2];
    strncpyz(buffer, v, (len < sizeof(buffer) - 1 ? len : sizeof(buffer) - 1));
    (void) uuid_parse_flexi(buffer, w->transaction); // will not alter w->transaction if it fails
}

static void http_header_x_netdata_account_id(struct web_client *w, const char *v, size_t len) {
    if(web_client_flag_check(w, WEB_CLIENT_FLAG_CONN_CLOUD) && w->acl & HTTP_ACL_ACLK) {
        char buffer[UUID_STR_LEN * 2];
        strncpyz(buffer, v, (len < sizeof(buffer) - 1 ? len : sizeof(buffer) - 1));
        (void) uuid_parse_flexi(buffer, w->user_auth.cloud_account_id.uuid); // will not alter w->cloud_account_id if it fails
    }
}

static void http_header_x_netdata_role(struct web_client *w, const char *v, size_t len) {
    if(web_client_flag_check(w, WEB_CLIENT_FLAG_CONN_CLOUD) && w->acl & HTTP_ACL_ACLK) {
        char buffer[100];
        strncpyz(buffer, v, (len < sizeof(buffer) - 1 ? len : sizeof(buffer) - 1));
        if (strcasecmp(buffer, "admin") == 0)
            w->user_auth.user_role = HTTP_USER_ROLE_ADMIN;
        else if(strcasecmp(buffer, "manager") == 0)
            w->user_auth.user_role = HTTP_USER_ROLE_MANAGER;
        else if(strcasecmp(buffer, "troubleshooter") == 0)
            w->user_auth.user_role = HTTP_USER_ROLE_TROUBLESHOOTER;
        else if(strcasecmp(buffer, "observer") == 0)
            w->user_auth.user_role = HTTP_USER_ROLE_OBSERVER;
        else if(strcasecmp(buffer, "member") == 0)
            w->user_auth.user_role = HTTP_USER_ROLE_MEMBER;
        else if(strcasecmp(buffer, "billing") == 0)
            w->user_auth.user_role = HTTP_USER_ROLE_BILLING;
        else
            w->user_auth.user_role = HTTP_USER_ROLE_MEMBER;
    }
}

static void http_header_x_netdata_permissions(struct web_client *w, const char *v, size_t len __maybe_unused) {
    if(web_client_flag_check(w, WEB_CLIENT_FLAG_CONN_CLOUD) && w->acl & HTTP_ACL_ACLK) {
        HTTP_ACCESS access = http_access_from_hex(v);
        web_client_set_permissions(w, access, w->user_auth.user_role, USER_AUTH_METHOD_CLOUD);
    }
}

static void http_header_x_netdata_user_name(struct web_client *w, const char *v, size_t len) {
    if(web_client_flag_check(w, WEB_CLIENT_FLAG_CONN_CLOUD) && w->acl & HTTP_ACL_ACLK) {
        strncpyz(w->user_auth.client_name, v, (len < sizeof(w->user_auth.client_name) - 1 ? len : sizeof(w->user_auth.client_name) - 1));
    }
}

static void http_header_x_netdata_auth(struct web_client *w, const char *v, size_t len __maybe_unused) {
    if(web_client_flag_check(w, WEB_CLIENT_FLAG_CONN_CLOUD) && w->acl & HTTP_ACL_ACLK)
        // we don't need authorization bearer when the request comes from netdata cloud
        return;

    if(strncasecmp(v, "Bearer ", 7) == 0) {
        v = &v[7];
        while(*v && isspace((uint8_t)*v)) v++;
        web_client_bearer_token_auth(w, v);
    }
}

// Handle WebSocket-specific headers
static void http_header_upgrade(struct web_client *w, const char *v, size_t len __maybe_unused) {
    if(strcasecmp(v, "websocket") == 0) {
        web_client_set_websocket(w);
    }
}

static void http_header_sec_websocket_key(struct web_client *w, const char *v, size_t len __maybe_unused) {
    // Store the websocket key for later use in the handshake
    freez(w->websocket.key);
    w->websocket.key = strdupz(v);
}

static void http_header_sec_websocket_version(struct web_client *w, const char *v, size_t len __maybe_unused) {
    // We only support version 13, which will be checked during handshake
    // No need to store this as we only accept one version
    if(strcmp(v, "13") != 0) {
        netdata_log_debug(D_WEB_CLIENT, "%llu: WebSocket version %s not supported, only version 13 is supported", w->id, v);
        web_client_clear_websocket(w);
    }
}

static void http_header_sec_websocket_protocol(struct web_client *w, const char *v, size_t len __maybe_unused) {
    // Store the requested protocols for later evaluation during handshake
    w->websocket.protocol = WEBSOCKET_PROTOCOL_2id(v);
}

static void http_header_sec_websocket_extensions(struct web_client *w, const char *v, size_t len __maybe_unused) {
    // Reset extension flags
    w->websocket.ext_flags = WS_EXTENSION_NONE;

    // Check if "permessage-deflate" is requested
    if (strstr(v, "permessage-deflate") != NULL) {
        // Parse extension parameters
        char extension_copy[1024];
        strncpyz(extension_copy, v, sizeof(extension_copy) - 1);

        char *token, *saveptr;
        token = strtok_r(extension_copy, ",", &saveptr);

        while (token) {
            // Trim leading/trailing spaces
            char *ext = token;
            while (*ext && isspace(*ext)) ext++;
            char *end = ext + strlen(ext) - 1;
            while (end > ext && isspace(*end)) *end-- = '\0';

            // Check if this is permessage-deflate extension
            if (strncmp(ext, "permessage-deflate", 18) == 0) {
                w->websocket.ext_flags |= WS_EXTENSION_PERMESSAGE_DEFLATE;

                // Parse parameters
                char *params = ext + 18;
                if (*params == ';') {
                    params++;

                    char *param, *param_saveptr;
                    param = strtok_r(params, ";", &param_saveptr);

                    while (param) {
                        // Trim leading/trailing spaces
                        while (*param && isspace(*param)) param++;
                        end = param + strlen(param) - 1;
                        while (end > param && isspace(*end)) *end-- = '\0';

                        // Client no context takeover
                        if (strcmp(param, "client_no_context_takeover") == 0)
                            w->websocket.ext_flags |= WS_EXTENSION_CLIENT_NO_CONTEXT_TAKEOVER;

                        // Server no context takeover
                        else if (strcmp(param, "server_no_context_takeover") == 0)
                            w->websocket.ext_flags |= WS_EXTENSION_SERVER_NO_CONTEXT_TAKEOVER;

                        // Server max window bits
                        else if (strncmp(param, "server_max_window_bits=", 23) == 0) {
                            w->websocket.server_max_window_bits = str2u(param + 23);
                            if(w->websocket.server_max_window_bits >= 8 && w->websocket.server_max_window_bits <= 15)
                                w->websocket.ext_flags |= WS_EXTENSION_SERVER_MAX_WINDOW_BITS;
                        }
                        // Server max window bits without value
                        else if (strcmp(param, "server_max_window_bits") == 0) {
                            w->websocket.ext_flags |= WS_EXTENSION_SERVER_MAX_WINDOW_BITS;
                            w->websocket.server_max_window_bits = 0; // Default
                        }

                        // Client max window bits with value
                        else if (strncmp(param, "client_max_window_bits=", 23) == 0) {
                            w->websocket.client_max_window_bits = str2u(param + 23);
                            if(w->websocket.client_max_window_bits >= 8 && w->websocket.client_max_window_bits <= 15)
                                w->websocket.ext_flags |= WS_EXTENSION_CLIENT_MAX_WINDOW_BITS;
                        }
                        // Client max window bits without value
                        else if (strcmp(param, "client_max_window_bits") == 0) {
                            w->websocket.ext_flags |= WS_EXTENSION_CLIENT_MAX_WINDOW_BITS;
                            w->websocket.client_max_window_bits = 0; // Default
                        }

                        param = strtok_r(NULL, ";", &param_saveptr);
                    }
                }

                break;  // Found and parsed permessage-deflate
            }

            token = strtok_r(NULL, ",", &saveptr);
        }

        netdata_log_debug(D_WEB_CLIENT, "%llu: Client requested WebSocket extensions: %s, "
                                        "enabled flags: %u, client_max_window_bits: %u, server_max_window_bits: %u",
                          w->id, v, w->websocket.ext_flags,
                          w->websocket.client_max_window_bits,
                          w->websocket.server_max_window_bits);
    }
}

struct {
    uint32_t hash;
    const char *key;
    void (*cb)(struct web_client *w, const char *value, size_t value_len);
} supported_headers[] = {
    { .hash = 0, .key = "Origin",                .cb = http_header_origin },
    { .hash = 0, .key = "Connection",            .cb = http_header_connection },
    { .hash = 0, .key = "DNT",                   .cb = http_header_dnt },
    { .hash = 0, .key = "User-Agent",            .cb = http_header_user_agent},
    { .hash = 0, .key = "Accept",                .cb = http_header_accept },
    { .hash = 0, .key = "X-Auth-Token",          .cb = http_header_x_auth_token },
    { .hash = 0, .key = "Host",                  .cb = http_header_host },
    { .hash = 0, .key = "Accept-Encoding",       .cb = http_header_accept_encoding },
    { .hash = 0, .key = "X-Forwarded-Host",      .cb = http_header_x_forwarded_host },
    { .hash = 0, .key = "X-Forwarded-For",       .cb = http_header_x_forwarded_for },
    { .hash = 0, .key = "X-Transaction-Id",      .cb = http_header_x_transaction_id },
    { .hash = 0, .key = "X-Netdata-Account-Id",  .cb = http_header_x_netdata_account_id },
    { .hash = 0, .key = "X-Netdata-Role",        .cb = http_header_x_netdata_role },
    { .hash = 0, .key = "X-Netdata-Permissions", .cb = http_header_x_netdata_permissions },
    { .hash = 0, .key = "X-Netdata-User-Name",   .cb = http_header_x_netdata_user_name },
    { .hash = 0, .key = "X-Netdata-Auth",        .cb = http_header_x_netdata_auth },

    // WebSocket headers
    { .hash = 0, .key = "Upgrade",               .cb = http_header_upgrade },
    { .hash = 0, .key = "Sec-WebSocket-Key",     .cb = http_header_sec_websocket_key },
    { .hash = 0, .key = "Sec-WebSocket-Version", .cb = http_header_sec_websocket_version },
    { .hash = 0, .key = "Sec-WebSocket-Protocol",.cb = http_header_sec_websocket_protocol },
    { .hash = 0, .key = "Sec-WebSocket-Extensions",.cb = http_header_sec_websocket_extensions },

    // for historical reasons.
    // there are a few nightly versions of netdata UI that incorrectly use this instead of X-Netdata-Auth
    { .hash = 0, .key = "Authorization",        .cb = http_header_x_netdata_auth },

    // terminator
    { .hash = 0, .key = NULL, .cb = NULL }
};

char *http_header_parse_line(struct web_client *w, char *s) {
    if(unlikely(!supported_headers[0].hash)) {
        // initialize the hashes, the first time it runs

        for(size_t i = 0; supported_headers[i].key ;i++)
            supported_headers[i].hash = simple_uhash(supported_headers[i].key);
    }

    char *e = s;

    // find the colon
    while(*e && *e != ':') e++;
    if(!*e) return e;

    // get the name
    *e = '\0';

    // find the value
    char *v = e + 1, *ve;

    // skip leading spaces from value
    while(*v == ' ') v++;
    ve = v;

    // find the \r
    while(*ve && *ve != '\r') ve++;
    if(!*ve || ve[1] != '\n') {
        *e = ':';
        return ve;
    }

    // terminate the value
    *ve = '\0';

    uint32_t hash = simple_uhash(s);

    for(size_t i = 0; supported_headers[i].key ;i++) {
        if(likely(hash != supported_headers[i].hash || strcasecmp(s, supported_headers[i].key) != 0))
            continue;

        supported_headers[i].cb(w, v, ve - v);
        break;
    }

    *e = ':';
    *ve = '\r';
    return ve;
}
