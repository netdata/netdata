// SPDX-License-Identifier: GPL-3.0-or-later

#include "web_client.h"
#include "web/websocket/websocket.h"
#include "web/mcp/adapters/mcp-http.h"
#include "web/mcp/adapters/mcp-sse.h"

// this is an async I/O implementation of the web server request parser
// it is used by all netdata web servers

int respect_web_browser_do_not_track_policy = 0;
const char *web_x_frame_options = NULL;

int web_enable_gzip = 1, web_gzip_level = 3, web_gzip_strategy = Z_DEFAULT_STRATEGY;

void web_client_set_conn_tcp(struct web_client *w) {
    web_client_flags_clear_conn(w);
    web_client_flag_set(w, WEB_CLIENT_FLAG_CONN_TCP);
}

void web_client_set_conn_unix(struct web_client *w) {
    web_client_flags_clear_conn(w);
    web_client_flag_set(w, WEB_CLIENT_FLAG_CONN_UNIX);
}

void web_client_set_conn_cloud(struct web_client *w) {
    web_client_flags_clear_conn(w);
    web_client_flag_set(w, WEB_CLIENT_FLAG_CONN_CLOUD);
}

void web_client_set_conn_webrtc(struct web_client *w) {
    web_client_flags_clear_conn(w);
    web_client_flag_set(w, WEB_CLIENT_FLAG_CONN_WEBRTC);
}

void web_client_reset_permissions(struct web_client *w) {
    w->user_auth.method = USER_AUTH_METHOD_NONE;
    w->user_auth.access = HTTP_ACCESS_NONE;
    w->user_auth.user_role = HTTP_USER_ROLE_NONE;
    web_client_clear_mcp_preview_key(w);
}

void web_client_set_permissions(struct web_client *w, HTTP_ACCESS access, HTTP_USER_ROLE role, USER_AUTH_METHOD type) {
    web_client_reset_permissions(w);
    w->user_auth.method = type;
    w->user_auth.access = access;
    w->user_auth.user_role = role;
}

inline int web_client_permission_denied_acl(struct web_client *w) {
    w->response.data->content_type = CT_TEXT_PLAIN;
    buffer_flush(w->response.data);
    buffer_strcat(w->response.data, "You need to be authorized to access this resource");
    w->response.code = HTTP_RESP_UNAVAILABLE_FOR_LEGAL_REASONS;
    return HTTP_RESP_UNAVAILABLE_FOR_LEGAL_REASONS;
}

inline int web_client_permission_denied(struct web_client *w) {
    w->response.data->content_type = CT_TEXT_PLAIN;
    buffer_flush(w->response.data);

    if(w->user_auth.access & HTTP_ACCESS_SIGNED_ID)
        buffer_strcat(w->response.data,
                      "You don't have enough permissions to access this resource");
    else
        buffer_strcat(w->response.data,
                      "You need to be authorized to access this resource");

    w->response.code = HTTP_ACCESS_PERMISSION_DENIED_HTTP_CODE(w->user_auth.access);
    return w->response.code;
}

inline int web_client_service_unavailable(struct web_client *w) {
    w->response.data->content_type = CT_TEXT_PLAIN;
    buffer_flush(w->response.data);
    buffer_strcat(w->response.data, "This service is currently unavailable.");
    w->response.code = HTTP_RESP_SERVICE_UNAVAILABLE;
    return HTTP_RESP_SERVICE_UNAVAILABLE;
}

static inline int bad_request_multiple_dashboard_versions(struct web_client *w) {
    w->response.data->content_type = CT_TEXT_PLAIN;
    buffer_flush(w->response.data);
    buffer_strcat(w->response.data, "Multiple dashboard versions given at the URL.");
    w->response.code = HTTP_RESP_BAD_REQUEST;
    return HTTP_RESP_BAD_REQUEST;
}

static inline void web_client_enable_wait_from_ssl(struct web_client *w) {
    if (w->ssl.ssl_errno == SSL_ERROR_WANT_READ)
        web_client_enable_ssl_wait_receive(w);
    else if (w->ssl.ssl_errno == SSL_ERROR_WANT_WRITE)
        web_client_enable_ssl_wait_send(w);
    else {
        web_client_disable_ssl_wait_receive(w);
        web_client_disable_ssl_wait_send(w);
    }
}

static inline char *strip_control_characters(char *url) {
    if(!url) return "";

    for(char *s = url; *s ;s++)
        if(iscntrl((uint8_t)*s)) *s = ' ';

    return url;
}

static void web_client_reset_allocations(struct web_client *w, bool free_all) {

    if(free_all) {
        // the web client is to be destroyed

        buffer_free(w->url_as_received);
        w->url_as_received = NULL;

        buffer_free(w->url_path_decoded);
        w->url_path_decoded = NULL;

        buffer_free(w->url_query_string_decoded);
        w->url_query_string_decoded = NULL;

        buffer_free(w->response.header_output);
        w->response.header_output = NULL;

        buffer_free(w->response.header);
        w->response.header = NULL;

        buffer_free(w->response.data);
        w->response.data = NULL;

        buffer_free(w->payload);
        w->payload = NULL;
    }
    else {
        // the web client is to be re-used

        buffer_reset(w->url_as_received);
        buffer_reset(w->url_path_decoded);
        buffer_reset(w->url_query_string_decoded);

        buffer_reset(w->response.header_output);
        buffer_reset(w->response.header);
        buffer_reset(w->response.data);

        if(w->payload)
            buffer_reset(w->payload);

        // to add more items here,
        // web_client_reuse_from_cache() needs to be adjusted to maintain them
    }

    freez(w->server_host);
    w->server_host = NULL;

    freez(w->forwarded_host);
    w->forwarded_host = NULL;

    freez(w->origin);
    w->origin = NULL;

    freez(w->user_agent);
    w->user_agent = NULL;

    freez(w->auth_bearer_token);
    w->auth_bearer_token = NULL;
    
    // Free WebSocket resources
    freez(w->websocket.key);
    w->websocket.key = NULL;
    
    w->websocket.ext_flags = WS_EXTENSION_NONE;
    w->websocket.protocol = WS_PROTOCOL_DEFAULT;
    w->websocket.client_max_window_bits = 0;
    w->websocket.server_max_window_bits = 0;

    // if we had enabled compression, release it
    if(w->response.zinitialized) {
        deflateEnd(&w->response.zstream);
        w->response.zsent = 0;
        w->response.zhave = 0;
        w->response.zstream.avail_in = 0;
        w->response.zstream.avail_out = 0;
        w->response.zstream.total_in = 0;
        w->response.zstream.total_out = 0;
        w->response.zinitialized = false;
        web_client_flag_clear(w, WEB_CLIENT_CHUNKED_TRANSFER);
    }

    memset(w->transaction, 0, sizeof(w->transaction));
    memset(&w->auth, 0, sizeof(w->auth));
    memset(&w->user_auth, 0, sizeof(w->user_auth));

    web_client_reset_permissions(w);
    web_client_flag_clear(w, WEB_CLIENT_ENCODING_GZIP|WEB_CLIENT_ENCODING_DEFLATE);
    web_client_flag_clear(w, WEB_CLIENT_FLAG_ACCEPT_JSON |
                             WEB_CLIENT_FLAG_ACCEPT_SSE |
                             WEB_CLIENT_FLAG_ACCEPT_TEXT);
    web_client_reset_path_flags(w);
}

void web_client_log_completed_request(struct web_client *w, bool update_web_stats) {
    struct timeval tv;
    now_monotonic_high_precision_timeval(&tv);

    size_t size = w->response.data->len;
    size_t sent = w->response.zoutput ? (size_t)w->response.zstream.total_out : size;

    usec_t prep_ut = w->timings.tv_ready.tv_sec ? dt_usec(&w->timings.tv_ready, &w->timings.tv_in) : 0;
    usec_t sent_ut = w->timings.tv_ready.tv_sec ? dt_usec(&tv, &w->timings.tv_ready) : 0;
    usec_t total_ut = dt_usec(&tv, &w->timings.tv_in);
    strip_control_characters((char *)buffer_tostring(w->url_as_received));

    ND_LOG_STACK lgs[] = {
            ND_LOG_FIELD_U64(NDF_CONNECTION_ID, w->id),
            ND_LOG_FIELD_UUID(NDF_TRANSACTION_ID, &w->transaction),
            ND_LOG_FIELD_TXT(NDF_NIDL_NODE, w->client_host),
            ND_LOG_FIELD_TXT(NDF_REQUEST_METHOD, HTTP_REQUEST_MODE_2str(w->mode)),
            ND_LOG_FIELD_BFR(NDF_REQUEST, w->url_as_received),
            ND_LOG_FIELD_U64(NDF_RESPONSE_CODE, w->response.code),
            ND_LOG_FIELD_U64(NDF_RESPONSE_SENT_BYTES, sent),
            ND_LOG_FIELD_U64(NDF_RESPONSE_SIZE_BYTES, size),
            ND_LOG_FIELD_U64(NDF_RESPONSE_PREPARATION_TIME_USEC, prep_ut),
            ND_LOG_FIELD_U64(NDF_RESPONSE_SENT_TIME_USEC, sent_ut),
            ND_LOG_FIELD_U64(NDF_RESPONSE_TOTAL_TIME_USEC, total_ut),
            ND_LOG_FIELD_TXT(NDF_SRC_IP, w->user_auth.client_ip),
            ND_LOG_FIELD_TXT(NDF_SRC_PORT, w->client_port),
            ND_LOG_FIELD_TXT(NDF_SRC_FORWARDED_FOR, w->user_auth.forwarded_for),
            ND_LOG_FIELD_UUID(NDF_ACCOUNT_ID, &w->user_auth.cloud_account_id.uuid),
            ND_LOG_FIELD_TXT(NDF_USER_NAME, w->user_auth.client_name),
            ND_LOG_FIELD_TXT(NDF_USER_ROLE, http_id2user_role(w->user_auth.user_role)),
            ND_LOG_FIELD_CB(NDF_USER_ACCESS, log_cb_http_access_to_hex, &w->user_auth.access),
            ND_LOG_FIELD_END(),
    };
    ND_LOG_STACK_PUSH(lgs);

    ND_LOG_FIELD_PRIORITY prio = NDLP_INFO;
    if(w->response.code >= 500)
        prio = NDLP_EMERG;
    else if(w->response.code >= 400)
        prio = NDLP_WARNING;
    else if(w->response.code >= 300)
        prio = NDLP_NOTICE;

    // cleanup progress
    if(web_client_flag_check(w, WEB_CLIENT_FLAG_PROGRESS_TRACKING)) {
        web_client_flag_clear(w, WEB_CLIENT_FLAG_PROGRESS_TRACKING);
        query_progress_finished(&w->transaction, 0, w->response.code, total_ut, size, sent);
    }

    // access log
    if(likely(buffer_strlen(w->url_as_received))) {
        nd_log(NDLS_ACCESS, prio, NULL);

        if(update_web_stats)
            pulse_web_request_completed(
                dt_usec(&tv, &w->timings.tv_in), w->statistics.received_bytes, w->statistics.sent_bytes, size, sent);
    }
}

void web_client_request_done(struct web_client *w) {
    sock_setcork(w->fd, false);

    netdata_log_debug(D_WEB_CLIENT, "%llu: Resetting client.", w->id);

    web_client_log_completed_request(w, true);
    web_client_reset_allocations(w, false);

    w->mode = HTTP_REQUEST_MODE_GET;

    web_client_disable_donottrack(w);
    web_client_disable_tracking_required(w);
    web_client_disable_keepalive(w);

    w->header_parse_tries = 0;
    w->header_parse_last_size = 0;

    web_client_enable_wait_receive(w);
    web_client_disable_wait_send(w);

    w->response.has_cookies = false;
    w->response.sent = 0;
    w->response.code = 0;
    w->response.zoutput = false;

    w->statistics.received_bytes = 0;
    w->statistics.sent_bytes = 0;
}

static int append_slash_to_url_and_redirect(struct web_client *w) {
    // this function returns a relative redirect
    // it finds the last path component on the URL and just appends / to it
    //
    // So, if the URL is:
    //
    //        /path/to/file?query_string
    //
    // It adds a Location header like this:
    //
    //       Location: file/?query_string\r\n
    //
    // The web browser already knows that it is inside /path/to/
    // so it converts the path to /path/to/file/ and executes the
    // request again.

    buffer_strcat(w->response.header, "Location: ");
    const char *b = buffer_tostring(w->url_as_received);
    const char *q = strchr(b, '?');
    if(q && q > b) {
        const char *e = q - 1;
        while(e > b && *e != '/') e--;
        if(*e == '/') e++;

        size_t len = q - e;
        buffer_strncat(w->response.header, e, len);
        buffer_strncat(w->response.header, "/", 1);
        buffer_strcat(w->response.header, q);
    }
    else {
        const char *e = &b[buffer_strlen(w->url_as_received) - 1];
        while(e > b && *e != '/') e--;
        if(*e == '/') e++;

        buffer_strcat(w->response.header, e);
        buffer_strncat(w->response.header, "/", 1);
    }

    buffer_strncat(w->response.header, "\r\n", 2);

    w->response.data->content_type = CT_TEXT_HTML;
    buffer_flush(w->response.data);
    buffer_strcat(w->response.data,
                  "<!DOCTYPE html><html>"
                  "<body onload=\"window.location.href = window.location.origin + window.location.pathname + '/' + window.location.search + window.location.hash\">"
                  "Redirecting. In case your browser does not support redirection, please click "
                  "<a onclick=\"window.location.href = window.location.origin + window.location.pathname + '/' + window.location.search + window.location.hash\">here</a>."
                  "</body></html>");
    return HTTP_RESP_MOVED_PERM;
}

// Work around a bug in the CMocka library by removing this function during testing.
#ifndef REMOVE_MYSENDFILE

static inline int dashboard_version(struct web_client *w) {
    if(!web_client_flag_check(w, WEB_CLIENT_FLAG_PATH_WITH_VERSION))
        return -1;

    if(web_client_flag_check(w, WEB_CLIENT_FLAG_PATH_IS_V3))
        return 3;
    if(web_client_flag_check(w, WEB_CLIENT_FLAG_PATH_IS_V2))
        return 2;
    if(web_client_flag_check(w, WEB_CLIENT_FLAG_PATH_IS_V1))
        return 1;
    if(web_client_flag_check(w, WEB_CLIENT_FLAG_PATH_IS_V0))
        return 0;

    return -1;
}

static bool find_filename_to_serve(const char *filename, char *dst, size_t dst_len, struct stat *statbuf, struct web_client *w, bool *is_dir) {
    int d_version = dashboard_version(w);
    bool has_extension = web_client_flag_check(w, WEB_CLIENT_FLAG_PATH_HAS_FILE_EXTENSION);

    int fallback = 0;

    if(has_extension) {
        if(d_version == -1)
            snprintfz(dst, dst_len, "%s/%s", netdata_configured_web_dir, filename);
        else {
            // check if the filename or directory exists
            // fallback to the same path without the dashboard version otherwise
            snprintfz(dst, dst_len, "%s/v%d/%s", netdata_configured_web_dir, d_version, filename);
            fallback = 1;
        }
    }
    else if(d_version != -1) {
        if(filename && *filename) {
            // check if the filename exists
            // fallback to /vN/index.html otherwise
            snprintfz(dst, dst_len, "%s/%s", netdata_configured_web_dir, filename);
            fallback = 2;
        }
        else {
            if(filename && *filename)
                web_client_flag_set(w, WEB_CLIENT_FLAG_PATH_HAS_TRAILING_SLASH);
            snprintfz(dst, dst_len, "%s/v%d", netdata_configured_web_dir, d_version);
        }
    }
    else {
        // check if filename exists
        // this is needed to serve {filename}/index.html, in case a user puts a html file into a directory
        // fallback to /index.html otherwise
        snprintfz(dst, dst_len, "%s/%s", netdata_configured_web_dir, filename);
        fallback = 3;
    }

    if (stat(dst, statbuf) != 0) {
        if(fallback == 1) {
            snprintfz(dst, dst_len, "%s/%s", netdata_configured_web_dir, filename);
            if (stat(dst, statbuf) != 0)
                return false;
        }
        else if(fallback == 2) {
            if(filename && *filename)
                web_client_flag_set(w, WEB_CLIENT_FLAG_PATH_HAS_TRAILING_SLASH);
            snprintfz(dst, dst_len, "%s/v%d", netdata_configured_web_dir, d_version);
            if (stat(dst, statbuf) != 0)
                return false;
        }
        else if(fallback == 3) {
            if(filename && *filename)
                web_client_flag_set(w, WEB_CLIENT_FLAG_PATH_HAS_TRAILING_SLASH);
            snprintfz(dst, dst_len, "%s", netdata_configured_web_dir);
            if (stat(dst, statbuf) != 0)
                return false;
        }
        else
            return false;
    }

    if((statbuf->st_mode & S_IFMT) == S_IFDIR) {
        size_t len = strlen(dst);
        if(len > dst_len - 11)
            return false;

        strncpyz(&dst[len], "/index.html", dst_len - len);

        if (stat(dst, statbuf) != 0)
            return false;

        *is_dir = true;
    }

    return true;
}

static int web_server_static_file(struct web_client *w, char *filename) {
    netdata_log_debug(D_WEB_CLIENT, "%llu: Looking for file '%s/%s'", w->id, netdata_configured_web_dir, filename);

    if(!http_can_access_dashboard(w))
        return web_client_permission_denied_acl(w);

    // skip leading slashes
    while (*filename == '/') filename++;

    // if the filename contains "strange" characters, refuse to serve it
    char *s;
    for(s = filename; *s ;s++) {
        if( !isalnum((uint8_t)*s) && *s != '/' && *s != '.' && *s != '-' && *s != '_') {
            netdata_log_debug(D_WEB_CLIENT_ACCESS, "%llu: File '%s' is not acceptable.", w->id, filename);
            w->response.data->content_type = CT_TEXT_HTML;
            buffer_sprintf(w->response.data, "Filename contains invalid characters: ");
            buffer_strcat_htmlescape(w->response.data, filename);
            return HTTP_RESP_BAD_REQUEST;
        }
    }

    // if the filename contains a double dot refuse to serve it
    if(strstr(filename, "..") != 0) {
        netdata_log_debug(D_WEB_CLIENT_ACCESS, "%llu: File '%s' is not acceptable.", w->id, filename);
        w->response.data->content_type = CT_TEXT_HTML;
        buffer_strcat(w->response.data, "Relative filenames are not supported: ");
        buffer_strcat_htmlescape(w->response.data, filename);
        return HTTP_RESP_BAD_REQUEST;
    }

    // find the physical file on disk
    bool is_dir = false;
    char web_filename[FILENAME_MAX + 1];
    struct stat statbuf;
    if(!find_filename_to_serve(filename, web_filename, FILENAME_MAX, &statbuf, w, &is_dir)) {
        w->response.data->content_type = CT_TEXT_HTML;
        buffer_strcat(w->response.data, "File does not exist, or is not accessible: ");
        buffer_strcat_htmlescape(w->response.data, filename);
        return HTTP_RESP_NOT_FOUND;
    }

    if(is_dir && !web_client_flag_check(w, WEB_CLIENT_FLAG_PATH_HAS_TRAILING_SLASH))
        return append_slash_to_url_and_redirect(w);

    buffer_flush(w->response.data);
    buffer_need_bytes(w->response.data, (size_t)statbuf.st_size);
    w->response.data->len = (size_t)statbuf.st_size;

    // open the file
    int fd = open(web_filename, O_RDONLY | O_CLOEXEC);

    // read the file
    if(fd != -1 && read(fd, w->response.data->buffer, statbuf.st_size) != statbuf.st_size) {
        // cannot read the whole file
        nd_log(NDLS_DAEMON, NDLP_ERR, "Web server failed to read file '%s'", web_filename);
        close(fd);
        fd = -1;
    }

    // check for failures
    if(fd == -1) {
        buffer_flush(w->response.data);

        if(errno == EBUSY || errno == EAGAIN) {
            netdata_log_error("%llu: File '%s' is busy, sending 307 Moved Temporarily to force retry.", w->id, web_filename);
            w->response.data->content_type = CT_TEXT_HTML;
            buffer_sprintf(w->response.header, "Location: /%s\r\n", filename);
            buffer_strcat(w->response.data, "File is currently busy, please try again later: ");
            buffer_strcat_htmlescape(w->response.data, filename);
            return HTTP_RESP_REDIR_TEMP;
        }
        else {
            netdata_log_error("%llu: Cannot open file '%s'.", w->id, web_filename);
            w->response.data->content_type = CT_TEXT_HTML;
            buffer_strcat(w->response.data, "Cannot open file: ");
            buffer_strcat_htmlescape(w->response.data, filename);
            return HTTP_RESP_NOT_FOUND;
        }
    }
    else
        close(fd);

    w->response.data->content_type = contenttype_for_filename(web_filename);
    netdata_log_debug(D_WEB_CLIENT_ACCESS, "%llu: Sending file '%s' (%"PRId64" bytes, fd %d).", w->id, web_filename, (int64_t)statbuf.st_size, w->fd);

    w->mode = HTTP_REQUEST_MODE_GET;
    web_client_enable_wait_send(w);
    web_client_disable_wait_receive(w);

#ifdef __APPLE__
    w->response.data->date = statbuf.st_mtimespec.tv_sec;
#else
    w->response.data->date = statbuf.st_mtim.tv_sec;
#endif
    w->response.data->expires = now_realtime_sec() + 86400;

    buffer_cacheable(w->response.data);

    return HTTP_RESP_OK;
}
#endif

static inline int check_host_and_call(RRDHOST *host, struct web_client *w, char *url, int (*func)(RRDHOST *, struct web_client *, char *)) {
    return func(host, w, url);
}

int web_client_api_request(RRDHOST *host, struct web_client *w, char *url_path_fragment) {
    ND_LOG_STACK lgs[] = {
            ND_LOG_FIELD_TXT(NDF_SRC_IP, w->user_auth.client_ip),
            ND_LOG_FIELD_TXT(NDF_SRC_PORT, w->client_port),
            ND_LOG_FIELD_TXT(NDF_SRC_FORWARDED_HOST, w->forwarded_host),
            ND_LOG_FIELD_TXT(NDF_SRC_FORWARDED_FOR, w->user_auth.forwarded_for),
            ND_LOG_FIELD_TXT(NDF_NIDL_NODE, w->client_host),
            ND_LOG_FIELD_TXT(NDF_REQUEST_METHOD, HTTP_REQUEST_MODE_2str(w->mode)),
            ND_LOG_FIELD_BFR(NDF_REQUEST, w->url_as_received),
            ND_LOG_FIELD_U64(NDF_CONNECTION_ID, w->id),
            ND_LOG_FIELD_UUID(NDF_TRANSACTION_ID, &w->transaction),
            ND_LOG_FIELD_UUID(NDF_ACCOUNT_ID, &w->user_auth.cloud_account_id.uuid),
            ND_LOG_FIELD_TXT(NDF_USER_NAME, w->user_auth.client_name),
            ND_LOG_FIELD_TXT(NDF_USER_ROLE, http_id2user_role(w->user_auth.user_role)),
            ND_LOG_FIELD_CB(NDF_USER_ACCESS, log_cb_http_access_to_hex, &w->user_auth.access),
            ND_LOG_FIELD_END(),
    };
    ND_LOG_STACK_PUSH(lgs);

    if(!web_client_flag_check(w, WEB_CLIENT_FLAG_PROGRESS_TRACKING)) {
        web_client_flag_set(w, WEB_CLIENT_FLAG_PROGRESS_TRACKING);
        query_progress_start_or_update(&w->transaction, 0, w->mode, w->acl,
                                       buffer_tostring(w->url_as_received),
                                       w->payload,
                                       w->user_auth.forwarded_for[0] ? w->user_auth.forwarded_for : w->user_auth.client_ip);
    }

    // get the api version
    char *tok = strsep_skip_consecutive_separators(&url_path_fragment, "/");
    if(tok && *tok) {
        if(strcmp(tok, "v3") == 0)
            return web_client_api_request_v3(host, w, url_path_fragment);
        else if(strcmp(tok, "v2") == 0)
            return web_client_api_request_v2(host, w, url_path_fragment);
        else if(strcmp(tok, "v1") == 0)
            return web_client_api_request_v1(host, w, url_path_fragment);
        else {
            buffer_flush(w->response.data);
            w->response.data->content_type = CT_TEXT_HTML;
            buffer_strcat(w->response.data, "Unsupported API version: ");
            buffer_strcat_htmlescape(w->response.data, tok);
            return HTTP_RESP_NOT_FOUND;
        }
    }
    else {
        buffer_flush(w->response.data);
        buffer_sprintf(w->response.data, "Which API version?");
        return HTTP_RESP_BAD_REQUEST;
    }
}


/**
 * Valid Method
 *
 * Netdata accepts only three methods, including one of these three(STREAM) is an internal method.
 *
 * @param w is the structure with the client request
 * @param s is the start string to parse
 *
 * @return it returns the next address to parse case the method is valid and NULL otherwise.
 */
static inline char *web_client_valid_method(struct web_client *w, char *s) {
    // is is a valid request?
    if(!strncmp(s, "GET ", 4)) {
        s = &s[4];
        w->mode = HTTP_REQUEST_MODE_GET;
    }
    else if(!strncmp(s, "OPTIONS ", 8)) {
        s = &s[8];
        w->mode = HTTP_REQUEST_MODE_OPTIONS;
    }
    else if(!strncmp(s, "POST ", 5)) {
        s = &s[5];
        w->mode = HTTP_REQUEST_MODE_POST;
    }
    else if(!strncmp(s, "PUT ", 4)) {
        s = &s[4];
        w->mode = HTTP_REQUEST_MODE_PUT;
    }
    else if(!strncmp(s, "DELETE ", 7)) {
        s = &s[7];
        w->mode = HTTP_REQUEST_MODE_DELETE;
    }
    else if(!strncmp(s, "STREAM ", 7)) {
        s = &s[7];

        if (!SSL_connection(&w->ssl) && http_is_using_ssl_force(w)) {
            w->header_parse_tries = 0;
            w->header_parse_last_size = 0;
            web_client_disable_wait_receive(w);

            char hostname[256];
            char *copyme = strstr(s,"hostname=");
            if ( copyme ){
                copyme += 9;
                char *end = strchr(copyme,'&');
                if(end){
                    size_t length = MIN(255, end - copyme);
                    memcpy(hostname,copyme,length);
                    hostname[length] = 0X00;
                }
                else{
                    memcpy(hostname,"not available",13);
                    hostname[13] = 0x00;
                }
            }
            else{
                memcpy(hostname,"not available",13);
                hostname[13] = 0x00;
            }
            netdata_log_error("The server is configured to always use encrypted connections, please enable the SSL on child with hostname '%s'.",hostname);
            s = NULL;
        }

        w->mode = HTTP_REQUEST_MODE_STREAM;
    }
    else {
        s = NULL;
    }

    return s;
}

/**
 * Request validate
 *
 * @param w is the structure with the client request
 *
 * @return It returns HTTP_VALIDATION_OK on success and another code present
 *          in the enum HTTP_VALIDATION otherwise.
 */
HTTP_VALIDATION http_request_validate(struct web_client *w) {
    char *s = (char *)buffer_tostring(w->response.data), *encoded_url = NULL;

    size_t last_pos = w->header_parse_last_size;

    w->header_parse_tries++;
    w->header_parse_last_size = buffer_strlen(w->response.data);

    int is_it_valid;
    if(w->header_parse_tries > 1) {
        if(last_pos > 4) last_pos -= 4; // allow searching for \r\n\r\n
        else last_pos = 0;

        if(w->header_parse_last_size <= last_pos)
            last_pos = 0;

        is_it_valid = url_is_request_complete_and_extract_payload(s, &s[last_pos],
                                                                  w->header_parse_last_size, &w->payload);

        if(!is_it_valid) {
            if(w->header_parse_tries > HTTP_REQ_MAX_HEADER_FETCH_TRIES) {
                netdata_log_info("Disabling slow client after %zu attempts to read the request (%zu bytes received)", w->header_parse_tries, buffer_strlen(w->response.data));
                w->header_parse_tries = 0;
                w->header_parse_last_size = 0;
                web_client_disable_wait_receive(w);
                return HTTP_VALIDATION_TOO_MANY_READ_RETRIES;
            }

            return HTTP_VALIDATION_INCOMPLETE;
        }

        is_it_valid = 1;
    } else {
        last_pos = w->header_parse_last_size;
        is_it_valid =
            url_is_request_complete_and_extract_payload(s, &s[last_pos], w->header_parse_last_size, &w->payload);
    }

    s = web_client_valid_method(w, s);
    if (!s) {
        w->header_parse_tries = 0;
        w->header_parse_last_size = 0;
        web_client_disable_wait_receive(w);

        return HTTP_VALIDATION_NOT_SUPPORTED;
    } else if (!is_it_valid) {
        web_client_enable_wait_receive(w);
        return HTTP_VALIDATION_INCOMPLETE;
    }

    //After the method we have the path and query string together
    encoded_url = s;

    //we search for the position where we have " HTTP/", because it finishes the user request
    s = url_find_protocol(s);

    // incomplete requests
    if(unlikely(!*s)) {
        web_client_enable_wait_receive(w);
        return HTTP_VALIDATION_INCOMPLETE;
    }

    // we have the end of encoded_url - remember it
    char *ue = s;

    // make sure we have complete request
    // complete requests contain: \r\n\r\n
    while(*s) {
        // find a line feed
        while(*s && *s++ != '\r');

        // did we reach the end?
        if(unlikely(!*s)) break;

        // is it \r\n ?
        if(likely(*s++ == '\n')) {

            // is it again \r\n ? (header end)
            if(unlikely(*s == '\r' && s[1] == '\n')) {
                // a valid complete HTTP request found

                char c = *ue;
                *ue = '\0';
                web_client_decode_path_and_query_string(w, encoded_url);
                *ue = c;

                if ( (web_client_check_conn_tcp(w)) && (netdata_ssl_web_server_ctx) ) {
                    if (!w->ssl.conn && (http_is_using_ssl_force(w) || http_is_using_ssl_default(w)) && (w->mode != HTTP_REQUEST_MODE_STREAM)) {
                        w->header_parse_tries = 0;
                        w->header_parse_last_size = 0;
                        web_client_disable_wait_receive(w);
                        return HTTP_VALIDATION_REDIRECT;
                    }
                }

                w->header_parse_tries = 0;
                w->header_parse_last_size = 0;
                web_client_disable_wait_receive(w);
                return HTTP_VALIDATION_OK;
            }

            // another header line
            s = http_header_parse_line(w, s);
        }
    }

    // incomplete request
    web_client_enable_wait_receive(w);
    return HTTP_VALIDATION_INCOMPLETE;
}

static inline ssize_t web_client_send_data(struct web_client *w,const void *buf,size_t len, int flags)
{
    do {
        errno_clear();

        ssize_t bytes;
        if ((web_client_check_conn_tcp(w)) && (netdata_ssl_web_server_ctx)) {
            if (SSL_connection(&w->ssl)) {
                bytes = netdata_ssl_write(&w->ssl, buf, len);
                web_client_enable_wait_from_ssl(w);
            } else
                bytes = send(w->fd, buf, len, flags);
        } else if (web_client_check_conn_tcp(w) || web_client_check_conn_unix(w))
            bytes = send(w->fd, buf, len, flags);
        else
            bytes = -999;

        if(bytes < 0 && errno == EAGAIN) {
            tinysleep();
            continue;
        }

        return bytes;
    } while(true);
}

void web_client_build_http_header(struct web_client *w) {
    if(unlikely(w->response.code != HTTP_RESP_OK))
        buffer_no_cacheable(w->response.data);

    if(unlikely(!w->response.data->date))
        w->response.data->date = now_realtime_sec();

    // set a proper expiration date, if not already set
    if(unlikely(!w->response.data->expires))
        w->response.data->expires = w->response.data->date +
                ((w->response.data->options & WB_CONTENT_NO_CACHEABLE) ? 0 : 86400);

    // prepare the HTTP response header
    netdata_log_debug(D_WEB_CLIENT, "%llu: Generating HTTP header with response %d.", w->id, w->response.code);

    const char *code_msg = http_response_code2string(w->response.code);

    // prepare the last modified and expiration dates
    char rfc7231_date[RFC7231_MAX_LENGTH], rfc7231_expires[RFC7231_MAX_LENGTH];
    rfc7231_datetime(rfc7231_date, sizeof(rfc7231_date), w->response.data->date);
    rfc7231_datetime(rfc7231_expires, sizeof(rfc7231_expires), w->response.data->expires);

    if (w->response.code == HTTP_RESP_HTTPS_UPGRADE) {
        buffer_sprintf(w->response.header_output,
                       "HTTP/1.1 %d %s\r\n"
                       "Location: https://%s%s\r\n",
                       w->response.code, code_msg,
                       w->server_host ? w->server_host : "",
                       buffer_tostring(w->url_as_received));
        w->response.code = HTTP_RESP_MOVED_PERM;
    }
    else {
        buffer_sprintf(w->response.header_output,
                       "HTTP/1.1 %d %s\r\n"
                       "Connection: %s\r\n"
                       "Server: Netdata Embedded HTTP Server %s\r\n"
                       "Access-Control-Allow-Origin: %s\r\n"
                       "Access-Control-Allow-Credentials: true\r\n"
                       "Date: %s\r\n",
                       w->response.code,
                       code_msg,
                       web_client_has_keepalive(w)?"keep-alive":"close",
                       NETDATA_VERSION,
                       w->origin ? w->origin : "*",
                       rfc7231_date);

        http_header_content_type(w->response.header_output, w->response.data->content_type);
    }

    if(unlikely(web_x_frame_options))
        buffer_sprintf(w->response.header_output, "X-Frame-Options: %s\r\n", web_x_frame_options);

    if(w->response.has_cookies) {
        if(respect_web_browser_do_not_track_policy)
            buffer_sprintf(w->response.header_output,
                           "Tk: T;cookies\r\n");
    }
    else {
        if(respect_web_browser_do_not_track_policy) {
            if(web_client_has_tracking_required(w))
                buffer_sprintf(w->response.header_output,
                               "Tk: T;cookies\r\n");
            else
                buffer_sprintf(w->response.header_output,
                               "Tk: N\r\n");
        }
    }

    if(w->mode == HTTP_REQUEST_MODE_OPTIONS) {
        buffer_strcat(w->response.header_output,
                "Access-Control-Allow-Methods: GET, POST, OPTIONS\r\n"
                        "Access-Control-Allow-Headers: accept, x-requested-with, origin, content-type, cookie, pragma, cache-control, x-auth-token, x-netdata-auth, x-transaction-id\r\n"
                        "Access-Control-Max-Age: 1209600\r\n" // 86400 * 14
        );
    }
    else {
        buffer_sprintf(w->response.header_output,
                "Cache-Control: %s\r\n"
                        "Expires: %s\r\n",
                (w->response.data->options & WB_CONTENT_NO_CACHEABLE)?"no-cache, no-store, must-revalidate\r\nPragma: no-cache":"public",
                rfc7231_expires);
    }

    // copy a possibly available custom header
    if(unlikely(buffer_strlen(w->response.header)))
        buffer_strcat(w->response.header_output, buffer_tostring(w->response.header));

    // headers related to the transfer method
    // HTTP 304 Not Modified MUST NOT include Transfer-Encoding or Content-Encoding per RFC 7232 Section 4.1
    if(w->response.code != HTTP_RESP_NOT_MODIFIED) {
        if(likely(w->response.zoutput))
            buffer_strcat(w->response.header_output, "Content-Encoding: gzip\r\n");

        if(likely(w->flags & WEB_CLIENT_CHUNKED_TRANSFER))
            buffer_strcat(w->response.header_output, "Transfer-Encoding: chunked\r\n");
    }
    
    // Content-Length header handling
    if(w->response.code == HTTP_RESP_NOT_MODIFIED) {
        // For 304 Not Modified, always send Content-Length: 0 per RFC 7232 Section 4.1
        buffer_strcat(w->response.header_output, "Content-Length: 0\r\n");
    }
    else if(!(w->flags & WEB_CLIENT_CHUNKED_TRANSFER)) {
        // For non-chunked responses, send Content-Length if we know it
        if(likely(w->response.data->len)) {
            // we know the content length, put it
            buffer_sprintf(w->response.header_output, "Content-Length: %zu\r\n", (size_t)w->response.data->len);
        }
        else {
            // we don't know the content length, disable keep-alive
            web_client_disable_keepalive(w);
        }
    }

    char uuid[UUID_COMPACT_STR_LEN];
    uuid_unparse_lower_compact(w->transaction, uuid);
    buffer_sprintf(w->response.header_output,
                   "X-Transaction-ID: %s\r\n", uuid);

    // end of HTTP header
    buffer_strcat(w->response.header_output, "\r\n");
}

static inline void web_client_send_http_header(struct web_client *w) {
    // For WebSocket handshake, the header is already fully prepared in websocket_handle_handshake
    // For standard HTTP responses, we need to build the header
    if (w->response.code != HTTP_RESP_WEBSOCKET_HANDSHAKE) {
        web_client_build_http_header(w);
    }

    // sent the HTTP header
    netdata_log_debug(D_WEB_DATA, "%llu: Sending response HTTP header of size %zu: '%s'"
          , w->id
          , buffer_strlen(w->response.header_output)
          , buffer_tostring(w->response.header_output)
    );

    sock_setcork(w->fd, true);

    size_t count = 0;
    ssize_t bytes;

    if ( (web_client_check_conn_tcp(w)) && (netdata_ssl_web_server_ctx) ) {
        if (SSL_connection(&w->ssl)) {
            bytes = netdata_ssl_write(&w->ssl, buffer_tostring(w->response.header_output), buffer_strlen(w->response.header_output));
            web_client_enable_wait_from_ssl(w);
        }
        else {
            while((bytes = send(w->fd, buffer_tostring(w->response.header_output), buffer_strlen(w->response.header_output), 0)) == -1) {
                count++;

                if(count > 100 || (errno != EAGAIN && errno != EWOULDBLOCK)) {
                    netdata_log_error("Cannot send HTTP headers to web client.");
                    break;
                }
            }
        }
    }
    else if(web_client_check_conn_tcp(w) || web_client_check_conn_unix(w)) {
        while((bytes = send(w->fd, buffer_tostring(w->response.header_output), buffer_strlen(w->response.header_output), 0)) == -1) {
            count++;

            if(count > 100 || (errno != EAGAIN && errno != EWOULDBLOCK)) {
                netdata_log_error("Cannot send HTTP headers to web client.");
                break;
            }
        }
    }
    else
        bytes = -999;

    if(bytes != (ssize_t) buffer_strlen(w->response.header_output)) {
        if(bytes > 0)
            w->statistics.sent_bytes += bytes;

        if (bytes < 0) {
            netdata_log_error("HTTP headers failed to be sent (I sent %zu bytes but the system sent %zd bytes). Closing web client."
                  , buffer_strlen(w->response.header_output)
                  , bytes);

            WEB_CLIENT_IS_DEAD(w);
            return;
        }
    }
    else
        w->statistics.sent_bytes += bytes;
}

static inline int web_client_switch_host(RRDHOST *host, struct web_client *w, char *url, bool nodeid, int (*func)(RRDHOST *, struct web_client *, char *)) {
    static uint32_t hash_localhost = 0;

    if(unlikely(!hash_localhost)) {
        hash_localhost = simple_hash("localhost");
    }

    if(host != localhost) {
        buffer_flush(w->response.data);
        buffer_strcat(w->response.data, "Nesting of hosts is not allowed.");
        return HTTP_RESP_BAD_REQUEST;
    }

    char *tok = strsep_skip_consecutive_separators(&url, "/");
    if(tok && *tok) {
        netdata_log_debug(D_WEB_CLIENT, "%llu: Searching for host with name '%s'.", w->id, tok);

        if(nodeid) {
            host = rrdhost_find_by_node_id(tok);
            if(!host) {
                host = rrdhost_find_by_guid(tok);
                if (!host)
                    host = rrdhost_find_by_hostname(tok);
            }
        }
        else {
            host = rrdhost_find_by_guid(tok);
            if(!host) {
                host = rrdhost_find_by_node_id(tok);
                if (!host)
                    host = rrdhost_find_by_hostname(tok);
            }
        }

        if(!host) {
            // we didn't find it, but it may be a uuid case mismatch for MACHINE_GUID
            // so, recreate the machine guid in lower-case.
            nd_uuid_t uuid;
            char txt[UUID_STR_LEN];
            if (uuid_parse(tok, uuid) == 0) {
                uuid_unparse_lower(uuid, txt);
                host = rrdhost_find_by_guid(txt);
            }
        }

        if (host) {
            if(!url)
                //no delim found
                return append_slash_to_url_and_redirect(w);

            size_t len = strlen(url) + 2;
            char buf[len];
            buf[0] = '/';
            strcpy(&buf[1], url);
            buf[len - 1] = '\0';

            buffer_flush(w->url_path_decoded);
            buffer_strcat(w->url_path_decoded, buf);
            return func(host, w, buf);
        }
    }

    buffer_flush(w->response.data);
    w->response.data->content_type = CT_TEXT_HTML;
    buffer_strcat(w->response.data, "This netdata does not maintain a database for host: ");
    buffer_strcat_htmlescape(w->response.data, tok?tok:"");
    return HTTP_RESP_NOT_FOUND;
}

int web_client_api_request_with_node_selection(RRDHOST *host, struct web_client *w, char *decoded_url_path) {
    // entry point for all API requests

    ND_LOG_STACK lgs[] = {
            ND_LOG_FIELD_TXT(NDF_REQUEST_METHOD, HTTP_REQUEST_MODE_2str(w->mode)),
            ND_LOG_FIELD_BFR(NDF_REQUEST, w->url_as_received),
            ND_LOG_FIELD_U64(NDF_CONNECTION_ID, w->id),
            ND_LOG_FIELD_UUID(NDF_TRANSACTION_ID, &w->transaction),
            ND_LOG_FIELD_UUID(NDF_ACCOUNT_ID, &w->user_auth.cloud_account_id.uuid),
            ND_LOG_FIELD_TXT(NDF_USER_NAME, w->user_auth.client_name),
            ND_LOG_FIELD_TXT(NDF_USER_ROLE, http_id2user_role(w->user_auth.user_role)),
            ND_LOG_FIELD_CB(NDF_USER_ACCESS, log_cb_http_access_to_hex, &w->user_auth.access),
            ND_LOG_FIELD_END(),
    };
    ND_LOG_STACK_PUSH(lgs);

    // give a new transaction id to the request
    if(uuid_is_null(w->transaction))
        uuid_generate_random(w->transaction);

    static uint32_t
            hash_api = 0,
            hash_host = 0,
            hash_node = 0;

    if(unlikely(!hash_api)) {
        hash_api = simple_hash("api");
        hash_host = simple_hash("host");
        hash_node = simple_hash("node");
    }

    char *tok = strsep_skip_consecutive_separators(&decoded_url_path, "/?");
    if(likely(tok && *tok)) {
        uint32_t hash = simple_hash(tok);

        if(unlikely(hash == hash_api && strcmp(tok, "api") == 0)) {
            // current API
            netdata_log_debug(D_WEB_CLIENT_ACCESS, "%llu: API request ...", w->id);
            return check_host_and_call(host, w, decoded_url_path, web_client_api_request);
        }
        else if(unlikely((hash == hash_host && strcmp(tok, "host") == 0) || (hash == hash_node && strcmp(tok, "node") == 0))) {
            // host switching
            netdata_log_debug(D_WEB_CLIENT_ACCESS, "%llu: host switch request ...", w->id);
            return web_client_switch_host(host, w, decoded_url_path, hash == hash_node, web_client_api_request_with_node_selection);
        }
    }

    buffer_flush(w->response.data);
    buffer_strcat(w->response.data, "Unknown API endpoint.");
    w->response.data->content_type = CT_TEXT_HTML;
    return HTTP_RESP_NOT_FOUND;
}

static inline int web_client_process_url(RRDHOST *host, struct web_client *w, char *decoded_url_path) {
    if(unlikely(!service_running(ABILITY_WEB_REQUESTS)))
        return web_client_service_unavailable(w);

    static uint32_t
            hash_api = 0,
            hash_netdata_conf = 0,
            hash_host = 0,
            hash_node = 0,
            hash_v0 = 0,
            hash_v1 = 0,
            hash_v2 = 0,
            hash_v3 = 0,
            hash_mcp = 0,
            hash_sse = 0;

#ifdef NETDATA_INTERNAL_CHECKS
    static uint32_t hash_exit = 0, hash_debug = 0, hash_mirror = 0;
#endif

    if(unlikely(!hash_api)) {
        hash_api = simple_hash("api");
        hash_netdata_conf = simple_hash("netdata.conf");
        hash_host = simple_hash("host");
        hash_node = simple_hash("node");
        hash_v0 = simple_hash("v0");
        hash_v1 = simple_hash("v1");
        hash_v2 = simple_hash("v2");
        hash_v3 = simple_hash("v3");
        hash_mcp = simple_hash("mcp");
        hash_sse = simple_hash("sse");
#ifdef NETDATA_INTERNAL_CHECKS
        hash_exit = simple_hash("exit");
        hash_debug = simple_hash("debug");
        hash_mirror = simple_hash("mirror");
#endif
    }

    // keep a copy of the decoded path, in case we need to serve it as a filename
    char filename[FILENAME_MAX + 1];
    strncpyz(filename, decoded_url_path ? decoded_url_path : "", FILENAME_MAX);

    char *tok = strsep_skip_consecutive_separators(&decoded_url_path, "/?");
    if(likely(tok && *tok)) {
        uint32_t hash = simple_hash(tok);
        netdata_log_debug(D_WEB_CLIENT, "%llu: Processing command '%s'.", w->id, tok);

        if(likely(hash == hash_api && strcmp(tok, "api") == 0)) {                           // current API
            netdata_log_debug(D_WEB_CLIENT_ACCESS, "%llu: API request ...", w->id);
            return check_host_and_call(host, w, decoded_url_path, web_client_api_request);
        }
        else if(likely(hash == hash_mcp && strcmp(tok, "mcp") == 0)) {
            if(unlikely(!http_can_access_dashboard(w)))
                return web_client_permission_denied_acl(w);
            return mcp_http_handle_request(host, w);
        }
        else if(likely(hash == hash_sse && strcmp(tok, "sse") == 0)) {
            if(unlikely(!http_can_access_dashboard(w)))
                return web_client_permission_denied_acl(w);
            return mcp_sse_handle_request(host, w);
        }
        else if(unlikely((hash == hash_host && strcmp(tok, "host") == 0) || (hash == hash_node && strcmp(tok, "node") == 0))) { // host switching
            netdata_log_debug(D_WEB_CLIENT_ACCESS, "%llu: host switch request ...", w->id);
            return web_client_switch_host(host, w, decoded_url_path, hash == hash_node, web_client_process_url);
        }
        else if(unlikely(hash == hash_v3 && strcmp(tok, "v3") == 0)) {
            if(web_client_flag_check(w, WEB_CLIENT_FLAG_PATH_WITH_VERSION))
                return bad_request_multiple_dashboard_versions(w);
            web_client_flag_set(w, WEB_CLIENT_FLAG_PATH_IS_V3);
            return web_client_process_url(host, w, decoded_url_path);
        }
        else if(unlikely(hash == hash_v2 && strcmp(tok, "v2") == 0)) {
            if(web_client_flag_check(w, WEB_CLIENT_FLAG_PATH_WITH_VERSION))
                return bad_request_multiple_dashboard_versions(w);
            web_client_flag_set(w, WEB_CLIENT_FLAG_PATH_IS_V2);
            return web_client_process_url(host, w, decoded_url_path);
        }
        else if(unlikely(hash == hash_v1 && strcmp(tok, "v1") == 0)) {
            if(web_client_flag_check(w, WEB_CLIENT_FLAG_PATH_WITH_VERSION))
                return bad_request_multiple_dashboard_versions(w);
            web_client_flag_set(w, WEB_CLIENT_FLAG_PATH_IS_V1);
            return web_client_process_url(host, w, decoded_url_path);
        }
        else if(unlikely(hash == hash_v0 && strcmp(tok, "v0") == 0)) {
            if(web_client_flag_check(w, WEB_CLIENT_FLAG_PATH_WITH_VERSION))
                return bad_request_multiple_dashboard_versions(w);
            web_client_flag_set(w, WEB_CLIENT_FLAG_PATH_IS_V0);
            return web_client_process_url(host, w, decoded_url_path);
        }
        else if(unlikely(hash == hash_netdata_conf && strcmp(tok, "netdata.conf") == 0)) {    // netdata.conf
            if(unlikely(!http_can_access_netdataconf(w)))
                return web_client_permission_denied_acl(w);

            netdata_log_debug(D_WEB_CLIENT_ACCESS, "%llu: generating netdata.conf ...", w->id);
            w->response.data->content_type = CT_TEXT_PLAIN;
            buffer_flush(w->response.data);

            inicfg_generate(&netdata_config, w->response.data, 0, true);
            return HTTP_RESP_OK;
        }
#ifdef NETDATA_INTERNAL_CHECKS
        else if(unlikely(hash == hash_exit && strcmp(tok, "exit") == 0)) {
            if(unlikely(!http_can_access_netdataconf(w)))
                return web_client_permission_denied_acl(w);

            w->response.data->content_type = CT_TEXT_PLAIN;
            buffer_flush(w->response.data);

            if(!exit_initiated_get())
                buffer_strcat(w->response.data, "ok, will do...");
            else
                buffer_strcat(w->response.data, "I am doing it already");

            netdata_log_error("web request to exit received.");
            netdata_exit_gracefully(EXIT_REASON_API_QUIT, true);
            return HTTP_RESP_OK;
        }
        else if(unlikely(hash == hash_debug && strcmp(tok, "debug") == 0)) {
            if(unlikely(!http_can_access_netdataconf(w)))
                return web_client_permission_denied_acl(w);

            buffer_flush(w->response.data);

            // get the name of the data to show
            tok = strsep_skip_consecutive_separators(&decoded_url_path, "&");
            if(tok && *tok) {
                netdata_log_debug(D_WEB_CLIENT, "%llu: Searching for RRD data with name '%s'.", w->id, tok);

                // do we have such a data set?
                RRDSET *st = rrdset_find_byname(host, tok);
                if(!st) st = rrdset_find(host, tok, false);
                if(!st) {
                    w->response.data->content_type = CT_TEXT_HTML;
                    buffer_strcat(w->response.data, "Chart is not found: ");
                    buffer_strcat_htmlescape(w->response.data, tok);
                    netdata_log_debug(D_WEB_CLIENT_ACCESS, "%llu: %s is not found.", w->id, tok);
                    return HTTP_RESP_NOT_FOUND;
                }

                if(rrdset_flag_check(st, RRDSET_FLAG_DEBUG))
                    rrdset_flag_clear(st, RRDSET_FLAG_DEBUG);
                else
                    rrdset_flag_set(st, RRDSET_FLAG_DEBUG);

                w->response.data->content_type = CT_TEXT_HTML;
                buffer_sprintf(w->response.data, "Chart has now debug %s: ", rrdset_flag_check(st, RRDSET_FLAG_DEBUG)?"enabled":"disabled");
                buffer_strcat_htmlescape(w->response.data, tok);
                netdata_log_debug(D_WEB_CLIENT_ACCESS, "%llu: debug for %s is %s.", w->id, tok, rrdset_flag_check(st, RRDSET_FLAG_DEBUG)?"enabled":"disabled");
                return HTTP_RESP_OK;
            }

            buffer_flush(w->response.data);
            buffer_strcat(w->response.data, "debug which chart?\r\n");
            return HTTP_RESP_BAD_REQUEST;
        }
        else if(unlikely(hash == hash_mirror && strcmp(tok, "mirror") == 0)) {
            if(unlikely(!http_can_access_netdataconf(w)))
                return web_client_permission_denied_acl(w);

            netdata_log_debug(D_WEB_CLIENT_ACCESS, "%llu: Mirroring...", w->id);

            // replace the zero bytes with spaces
            buffer_char_replace(w->response.data, '\0', ' ');

            // just leave the buffer as-is
            // it will be copied back to the client

            return HTTP_RESP_OK;
        }
#endif  /* NETDATA_INTERNAL_CHECKS */
    }

    buffer_flush(w->response.data);
    return web_server_static_file(w, filename);
}

static bool web_server_log_transport(BUFFER *wb, void *ptr) {
    struct web_client *w = ptr;
    if(!w)
        return false;

    buffer_strcat(wb, SSL_connection(&w->ssl) ? "https" : "http");
    return true;
}

void web_client_process_request_from_web_server(struct web_client *w) {
    // entry point for web server requests

    ND_LOG_STACK lgs[] = {
            ND_LOG_FIELD_CB(NDF_SRC_TRANSPORT, web_server_log_transport, w),
            ND_LOG_FIELD_TXT(NDF_SRC_IP, w->user_auth.client_ip),
            ND_LOG_FIELD_TXT(NDF_SRC_PORT, w->client_port),
            ND_LOG_FIELD_TXT(NDF_SRC_FORWARDED_HOST, w->forwarded_host),
            ND_LOG_FIELD_TXT(NDF_SRC_FORWARDED_FOR, w->user_auth.forwarded_for),
            ND_LOG_FIELD_TXT(NDF_NIDL_NODE, w->client_host),
            ND_LOG_FIELD_TXT(NDF_REQUEST_METHOD, HTTP_REQUEST_MODE_2str(w->mode)),
            ND_LOG_FIELD_BFR(NDF_REQUEST, w->url_as_received),
            ND_LOG_FIELD_U64(NDF_CONNECTION_ID, w->id),
            ND_LOG_FIELD_UUID(NDF_TRANSACTION_ID, &w->transaction),
            ND_LOG_FIELD_UUID(NDF_ACCOUNT_ID, &w->user_auth.cloud_account_id.uuid),
            ND_LOG_FIELD_TXT(NDF_USER_NAME, w->user_auth.client_name),
            ND_LOG_FIELD_TXT(NDF_USER_ROLE, http_id2user_role(w->user_auth.user_role)),
            ND_LOG_FIELD_CB(NDF_USER_ACCESS, log_cb_http_access_to_hex, &w->user_auth.access),
            ND_LOG_FIELD_END(),
    };
    ND_LOG_STACK_PUSH(lgs);

    // give a new transaction id to the request
    if(uuid_is_null(w->transaction))
        uuid_generate_random(w->transaction);

    // start timing us
    web_client_timeout_checkpoint_init(w);

    switch(http_request_validate(w)) {
        case HTTP_VALIDATION_OK:
            if(!web_client_flag_check(w, WEB_CLIENT_FLAG_PROGRESS_TRACKING)) {
                web_client_flag_set(w, WEB_CLIENT_FLAG_PROGRESS_TRACKING);
                query_progress_start_or_update(&w->transaction, 0, w->mode, w->acl,
                                               buffer_tostring(w->url_as_received),
                                               w->payload,
                                               w->user_auth.forwarded_for[0] ? w->user_auth.forwarded_for : w->user_auth.client_ip);
            }

            // Check if this is a WebSocket upgrade request
            // The full WebSocket handshake detection will happen in the header parsing,
            // but we need to set the initial mode to GET for processing to continue
            if (w->mode == HTTP_REQUEST_MODE_GET && web_client_has_websocket_handshake(w) && web_client_is_websocket(w)) {
                w->mode = HTTP_REQUEST_MODE_WEBSOCKET;
                netdata_log_debug(D_WEB_CLIENT, "%llu: Detected WebSocket handshake request", w->id);
            }

            switch(w->mode) {
                case HTTP_REQUEST_MODE_STREAM:
                    if(unlikely(!http_can_access_stream(w))) {
                        web_client_permission_denied_acl(w);
                        return;
                    }

                    w->response.code = stream_receiver_accept_connection(
                        w, (char *)buffer_tostring(w->url_query_string_decoded));
                    return;
                
                case HTTP_REQUEST_MODE_WEBSOCKET:
                    if(unlikely(!http_can_access_dashboard(w))) {
                        web_client_permission_denied_acl(w);
                        return;
                    }
                    
                    // Handle WebSocket handshake - this will take over the socket
                    // similar to how stream_receiver_accept_connection works
                    w->response.code = websocket_handle_handshake(w);
                    
                    // After this point the socket has been taken over
                    // No need to send a response as the WebSocket handler
                    // has already sent the handshake response
                    return;

                case HTTP_REQUEST_MODE_OPTIONS:
                    if(unlikely(
                            !http_can_access_dashboard(w) &&
                            !http_can_access_registry(w) &&
                            !http_can_access_badges(w) &&
                            !http_can_access_mgmt(w) &&
                            !http_can_access_netdataconf(w)
                    )) {
                        web_client_permission_denied_acl(w);
                        break;
                    }

                    w->response.data->content_type = CT_TEXT_PLAIN;
                    buffer_flush(w->response.data);
                    buffer_strcat(w->response.data, "OK");
                    w->response.code = HTTP_RESP_OK;
                    break;

                case HTTP_REQUEST_MODE_POST:
                case HTTP_REQUEST_MODE_GET:
                case HTTP_REQUEST_MODE_PUT:
                case HTTP_REQUEST_MODE_DELETE:
                    if(unlikely(
                            !http_can_access_dashboard(w) &&
                            !http_can_access_registry(w) &&
                            !http_can_access_badges(w) &&
                            !http_can_access_mgmt(w) &&
                            !http_can_access_netdataconf(w)
                    )) {
                        web_client_permission_denied_acl(w);
                        break;
                    }

                    web_client_reset_path_flags(w);

                    // find if the URL path has a filename extension
                    char path[FILENAME_MAX + 1];
                    strncpyz(path, buffer_tostring(w->url_path_decoded), FILENAME_MAX);
                    char *s = path, *e = path;

                    // remove the query string and find the last char
                    for (; *e ; e++) {
                        if (*e == '?')
                            break;
                    }

                    if(e == s || (*(e - 1) == '/'))
                        web_client_flag_set(w, WEB_CLIENT_FLAG_PATH_HAS_TRAILING_SLASH);

                    // check if there is a filename extension
                    while (--e > s) {
                        if (*e == '/')
                            break;
                        if(*e == '.') {
                            web_client_flag_set(w, WEB_CLIENT_FLAG_PATH_HAS_FILE_EXTENSION);
                            break;
                        }
                    }

                    w->response.code = (short)web_client_process_url(localhost, w, path);
                    break;

                default:
                    web_client_permission_denied_acl(w);
                    return;
            }
            break;

        case HTTP_VALIDATION_INCOMPLETE:
            if(w->response.data->len > NETDATA_WEB_REQUEST_MAX_SIZE) {
                buffer_flush(w->url_as_received);
                buffer_strcat(w->url_as_received, "too big request");

                netdata_log_debug(D_WEB_CLIENT_ACCESS, "%llu: Received request is too big (%zu bytes).", w->id, (size_t)w->response.data->len);

                size_t len = w->response.data->len;
                buffer_flush(w->response.data);
                buffer_sprintf(w->response.data, "Received request is too big  (received %zu bytes, max is %zu bytes).\r\n", len, (size_t)NETDATA_WEB_REQUEST_MAX_SIZE);
                w->response.code = HTTP_RESP_BAD_REQUEST;
            }
            else {
                // wait for more data
                // set to normal to prevent web_server_rcv_callback
                // from going into stream mode
                if (w->mode == HTTP_REQUEST_MODE_STREAM || w->mode == HTTP_REQUEST_MODE_WEBSOCKET)
                    w->mode = HTTP_REQUEST_MODE_GET;
                return;
            }
            break;

        case HTTP_VALIDATION_REDIRECT:
        {
            buffer_flush(w->response.data);
            w->response.data->content_type = CT_TEXT_HTML;
            buffer_strcat(w->response.data,
                          "<!DOCTYPE html><!-- SPDX-License-Identifier: GPL-3.0-or-later --><html>"
                          "<body onload=\"window.location.href ='https://'+ window.location.hostname +"
                          " ':' + window.location.port + window.location.pathname + window.location.search\">"
                          "Redirecting to safety connection, case your browser does not support redirection, please"
                          " click <a onclick=\"window.location.href ='https://'+ window.location.hostname + ':' "
                          " + window.location.port + window.location.pathname + window.location.search\">here</a>."
                          "</body></html>");
            w->response.code = HTTP_RESP_HTTPS_UPGRADE;
            break;
        }

        case HTTP_VALIDATION_MALFORMED_URL:
            netdata_log_debug(D_WEB_CLIENT_ACCESS, "%llu: Malformed URL '%s'.", w->id, w->response.data->buffer);

            buffer_flush(w->response.data);
            buffer_strcat(w->response.data, "Malformed URL...\r\n");
            w->response.code = HTTP_RESP_BAD_REQUEST;
            break;
        case HTTP_VALIDATION_TOO_MANY_READ_RETRIES:
            netdata_log_debug(D_WEB_CLIENT_ACCESS, "%llu: Too many retries to read request '%s'.", w->id, w->response.data->buffer);

            buffer_flush(w->response.data);
            buffer_strcat(w->response.data, "Too many retries to read request.\r\n");
            w->response.code = HTTP_RESP_BAD_REQUEST;
            break;
        case HTTP_VALIDATION_NOT_SUPPORTED:
            netdata_log_debug(D_WEB_CLIENT_ACCESS, "%llu: HTTP method requested is not supported '%s'.", w->id, w->response.data->buffer);

            buffer_flush(w->response.data);
            buffer_strcat(w->response.data, "HTTP method requested is not supported...\r\n");
            w->response.code = HTTP_RESP_BAD_REQUEST;
            break;
    }

    // keep track of the processing time
    web_client_timeout_checkpoint_response_ready(w, NULL);

    w->response.sent = 0;

    web_client_send_http_header(w);

    // enable sending immediately if we have data
    if(w->response.data->len) web_client_enable_wait_send(w);
    else web_client_disable_wait_send(w);

    switch(w->mode) {
        case HTTP_REQUEST_MODE_STREAM:
            netdata_log_debug(D_WEB_CLIENT, "%llu: STREAM done.", w->id);
            break;

        case HTTP_REQUEST_MODE_WEBSOCKET:
            netdata_log_debug(D_WEB_CLIENT, "%llu: Done preparing the WEBSOCKET response..", w->id);
            break;

        case HTTP_REQUEST_MODE_OPTIONS:
            netdata_log_debug(D_WEB_CLIENT,
                "%llu: Done preparing the OPTIONS response. Sending data (%zu bytes) to client.",
                w->id, (size_t)w->response.data->len);
            break;

        case HTTP_REQUEST_MODE_POST:
        case HTTP_REQUEST_MODE_GET:
        case HTTP_REQUEST_MODE_PUT:
        case HTTP_REQUEST_MODE_DELETE:
            netdata_log_debug(D_WEB_CLIENT,
                "%llu: Done preparing the response. Sending data (%zu bytes) to client.",
                w->id, (size_t)w->response.data->len);
            break;

        default:
            fatal("%llu: Unknown client mode %u.", w->id, w->mode);
            break;
    }
}

ssize_t web_client_send_chunk_header(struct web_client *w, size_t len)
{
    netdata_log_debug(D_DEFLATE, "%llu: OPEN CHUNK of %zu bytes (hex: %zx).", w->id, len, len);
    char buf[24];
    ssize_t bytes;
    bytes = (ssize_t)sprintf(buf, "%zX\r\n", len);
    buf[bytes] = 0x00;

    bytes = web_client_send_data(w,buf,strlen(buf),0);
    if(bytes > 0) {
        netdata_log_debug(D_DEFLATE, "%llu: Sent chunk header %zd bytes.", w->id, bytes);
        w->statistics.sent_bytes += bytes;
    }

    else if(bytes == 0) {
        netdata_log_debug(D_WEB_CLIENT, "%llu: Did not send chunk header to the client.", w->id);
    }
    else {
        netdata_log_debug(D_WEB_CLIENT, "%llu: Failed to send chunk header to client.", w->id);
        WEB_CLIENT_IS_DEAD(w);
    }

    return bytes;
}

ssize_t web_client_send_chunk_close(struct web_client *w)
{
    //debug(D_DEFLATE, "%llu: CLOSE CHUNK.", w->id);

    ssize_t bytes;
    bytes = web_client_send_data(w,"\r\n",2,0);
    if(bytes > 0) {
        netdata_log_debug(D_DEFLATE, "%llu: Sent chunk suffix %zd bytes.", w->id, bytes);
        w->statistics.sent_bytes += bytes;
    }

    else if(bytes == 0) {
        netdata_log_debug(D_WEB_CLIENT, "%llu: Did not send chunk suffix to the client.", w->id);
    }
    else {
        netdata_log_debug(D_WEB_CLIENT, "%llu: Failed to send chunk suffix to client.", w->id);
        WEB_CLIENT_IS_DEAD(w);
    }

    return bytes;
}

ssize_t web_client_send_chunk_finalize(struct web_client *w)
{
    //debug(D_DEFLATE, "%llu: FINALIZE CHUNK.", w->id);

    ssize_t bytes;
    bytes = web_client_send_data(w,"\r\n0\r\n\r\n",7,0);
    if(bytes > 0) {
        netdata_log_debug(D_DEFLATE, "%llu: Sent chunk suffix %zd bytes.", w->id, bytes);
        w->statistics.sent_bytes += bytes;
    }

    else if(bytes == 0) {
        netdata_log_debug(D_WEB_CLIENT, "%llu: Did not send chunk finalize suffix to the client.", w->id);
    }
    else {
        netdata_log_debug(D_WEB_CLIENT, "%llu: Failed to send chunk finalize suffix to client.", w->id);
        WEB_CLIENT_IS_DEAD(w);
    }

    return bytes;
}

ssize_t web_client_send_deflate(struct web_client *w)
{
    ssize_t len = 0, t = 0;

    // when using compression,
    // w->response.sent is the amount of bytes passed through compression

    netdata_log_debug(D_DEFLATE,
        "%llu: web_client_send_deflate(): w->response.data->len = %zu, w->response.sent = %zu, w->response.zhave = %zu, w->response.zsent = %zu, w->response.zstream.avail_in = %u, w->response.zstream.avail_out = %u, w->response.zstream.total_in = %lu, w->response.zstream.total_out = %lu.",
        w->id, (size_t)w->response.data->len, w->response.sent, w->response.zhave, w->response.zsent, w->response.zstream.avail_in, w->response.zstream.avail_out, w->response.zstream.total_in, w->response.zstream.total_out);

    if(w->response.data->len - w->response.sent == 0 && w->response.zstream.avail_in == 0 && w->response.zhave == w->response.zsent && w->response.zstream.avail_out != 0) {
        // there is nothing to send

        netdata_log_debug(D_WEB_CLIENT, "%llu: Out of output data.", w->id);

        // finalize the chunk
        if(w->response.sent != 0) {
            t = web_client_send_chunk_finalize(w);
            if(t < 0) return t;
        }

        if(unlikely(!web_client_has_keepalive(w))) {
            netdata_log_debug(D_WEB_CLIENT, "%llu: Closing (keep-alive is not enabled). %zu bytes sent.", w->id, w->response.sent);
            WEB_CLIENT_IS_DEAD(w);
            return t;
        }

        // reset the client
        web_client_request_done(w);
        netdata_log_debug(D_WEB_CLIENT, "%llu: Done sending all data on socket.", w->id);
        return t;
    }

    if(w->response.zhave == w->response.zsent) {
        // compress more input data

        // close the previous open chunk
        if(w->response.sent != 0) {
            t = web_client_send_chunk_close(w);
            if(t < 0) return t;
        }

        netdata_log_debug(D_DEFLATE, "%llu: Compressing %zu new bytes starting from %zu (and %u left behind).", w->id, (w->response.data->len - w->response.sent), w->response.sent, w->response.zstream.avail_in);

        // give the compressor all the data not passed through the compressor yet
        if(w->response.data->len > w->response.sent) {
            w->response.zstream.next_in = (Bytef *)&w->response.data->buffer[w->response.sent - w->response.zstream.avail_in];
            w->response.zstream.avail_in += (uInt) (w->response.data->len - w->response.sent);
        }

        // reset the compressor output buffer
        w->response.zstream.next_out = w->response.zbuffer;
        w->response.zstream.avail_out = NETDATA_WEB_RESPONSE_ZLIB_CHUNK_SIZE;

        // ask for FINISH if we have all the input
        int flush = Z_SYNC_FLUSH;
        if((w->mode == HTTP_REQUEST_MODE_GET ||
             w->mode == HTTP_REQUEST_MODE_POST ||
             w->mode == HTTP_REQUEST_MODE_PUT ||
             w->mode == HTTP_REQUEST_MODE_DELETE)) {
            flush = Z_FINISH;
            netdata_log_debug(D_DEFLATE, "%llu: Requesting Z_FINISH, if possible.", w->id);
        }
        else {
            netdata_log_debug(D_DEFLATE, "%llu: Requesting Z_SYNC_FLUSH.", w->id);
        }

        // compress
        if(deflate(&w->response.zstream, flush) == Z_STREAM_ERROR) {
            netdata_log_error("%llu: Compression failed. Closing down client.", w->id);
            web_client_request_done(w);
            return(-1);
        }

        w->response.zhave = NETDATA_WEB_RESPONSE_ZLIB_CHUNK_SIZE - w->response.zstream.avail_out;
        w->response.zsent = 0;

        // keep track of the bytes passed through the compressor
        w->response.sent = w->response.data->len;

        netdata_log_debug(D_DEFLATE, "%llu: Compression produced %zu bytes.", w->id, w->response.zhave);

        // open a new chunk
        ssize_t t2 = web_client_send_chunk_header(w, w->response.zhave);
        if(t2 < 0) return t2;
        t += t2;
    }

    netdata_log_debug(D_WEB_CLIENT, "%llu: Sending %zu bytes of data (+%zd of chunk header).", w->id, w->response.zhave - w->response.zsent, t);

    len = web_client_send_data(w,&w->response.zbuffer[w->response.zsent], (size_t) (w->response.zhave - w->response.zsent), MSG_DONTWAIT);
    if(len > 0) {
        w->statistics.sent_bytes += len;
        w->response.zsent += len;
        len += t;
        netdata_log_debug(D_WEB_CLIENT, "%llu: Sent %zd bytes.", w->id, len);
    }
    else if(len == 0) {
        netdata_log_debug(D_WEB_CLIENT, "%llu: Did not send any bytes to the client (zhave = %zu, zsent = %zu, need to send = %zu).",
            w->id, w->response.zhave, w->response.zsent, w->response.zhave - w->response.zsent);

    }
    else {
        netdata_log_debug(D_WEB_CLIENT, "%llu: Failed to send data to client.", w->id);
        WEB_CLIENT_IS_DEAD(w);
    }

    return(len);
}

ssize_t web_client_send(struct web_client *w) {
    if(likely(w->response.zoutput)) return web_client_send_deflate(w);

    ssize_t bytes;

    if(unlikely(w->response.data->len - w->response.sent == 0)) {
        // there is nothing to send

        netdata_log_debug(D_WEB_CLIENT, "%llu: Out of output data.", w->id);

        // there can be two cases for this
        // A. we have done everything
        // B. we temporarily have nothing to send, waiting for the buffer to be filled by ifd

        if(unlikely(!web_client_has_keepalive(w))) {
            netdata_log_debug(D_WEB_CLIENT, "%llu: Closing (keep-alive is not enabled). %zu bytes sent.", w->id, w->response.sent);
            WEB_CLIENT_IS_DEAD(w);
            return 0;
        }

        web_client_request_done(w);
        netdata_log_debug(D_WEB_CLIENT, "%llu: Done sending all data on socket. Waiting for next request on the same socket.", w->id);
        return 0;
    }

    bytes = web_client_send_data(w,&w->response.data->buffer[w->response.sent], w->response.data->len - w->response.sent, MSG_DONTWAIT);
    if(likely(bytes > 0)) {
        w->statistics.sent_bytes += bytes;
        w->response.sent += bytes;
        netdata_log_debug(D_WEB_CLIENT, "%llu: Sent %zd bytes.", w->id, bytes);
    }
    else if(likely(bytes == 0)) {
        netdata_log_debug(D_WEB_CLIENT, "%llu: Did not send any bytes to the client.", w->id);
    }
    else {
        netdata_log_debug(D_WEB_CLIENT, "%llu: Failed to send data to client.", w->id);
        WEB_CLIENT_IS_DEAD(w);
    }

    return(bytes);
}

ssize_t web_client_receive(struct web_client *w) {
    ssize_t bytes;

    // do we have any space for more data?
    buffer_need_bytes(w->response.data, NETDATA_WEB_REQUEST_INITIAL_SIZE);

    ssize_t left = (ssize_t)(w->response.data->size - w->response.data->len);

    errno_clear();

    if ( (web_client_check_conn_tcp(w)) && (netdata_ssl_web_server_ctx) ) {
        if (SSL_connection(&w->ssl)) {
            bytes = netdata_ssl_read(&w->ssl, &w->response.data->buffer[w->response.data->len], (size_t) (left - 1));
            web_client_enable_wait_from_ssl(w);
        }
        else {
            bytes = recv(w->fd, &w->response.data->buffer[w->response.data->len], (size_t) (left - 1), MSG_DONTWAIT);
        }
    }
    else if(web_client_check_conn_tcp(w) || web_client_check_conn_unix(w)) {
        bytes = recv(w->fd, &w->response.data->buffer[w->response.data->len], (size_t) (left - 1), MSG_DONTWAIT);
    }
    else // other connection methods
        bytes = -1;

    if(likely(bytes > 0)) {
        w->statistics.received_bytes += bytes;

        size_t old = w->response.data->len;
        (void)old;

        w->response.data->len += bytes;
        w->response.data->buffer[w->response.data->len] = '\0';

        netdata_log_debug(D_WEB_CLIENT, "%llu: Received %zd bytes.", w->id, bytes);
        netdata_log_debug(D_WEB_DATA, "%llu: Received data: '%s'.", w->id, &w->response.data->buffer[old]);
    }
    else if(unlikely(bytes < 0 && (errno == EAGAIN || errno == EWOULDBLOCK || errno == EINTR))) {
        web_client_enable_wait_receive(w);
        return 0;
    }
    else if (bytes < 0) {
        netdata_log_debug(D_WEB_CLIENT, "%llu: receive data failed.", w->id);
        WEB_CLIENT_IS_DEAD(w);
    } else
        netdata_log_debug(D_WEB_CLIENT, "%llu: Received %zd bytes.", w->id, bytes);

    return(bytes);
}

void web_client_decode_path_and_query_string(struct web_client *w, const char *path_and_query_string) {
    char buffer[NETDATA_WEB_REQUEST_URL_SIZE + 2];
    buffer[0] = '\0';

    buffer_flush(w->url_path_decoded);
    buffer_flush(w->url_query_string_decoded);

    if(buffer_strlen(w->url_as_received) == 0)
        // do not overwrite this if it is already filled
        buffer_strcat(w->url_as_received, path_and_query_string);

    if(w->mode == HTTP_REQUEST_MODE_STREAM) {
        // in stream mode, there is no path

        url_decode_r(buffer, path_and_query_string, NETDATA_WEB_REQUEST_URL_SIZE + 1);

        buffer[NETDATA_WEB_REQUEST_URL_SIZE + 1] = '\0';
        buffer_strcat(w->url_query_string_decoded, buffer);
    }
    else {
        // in non-stream mode, there is a path
        // FIXME - the way this is implemented, query string params never accept the symbol &, not even encoded as %26
        // To support the symbol & in query string params, we need to turn the url_query_string_decoded into a
        // dictionary and decode each of the parameters individually.
        // OR: in url_query_string_decoded use as separator a control character that cannot appear in the URL.

        url_decode_r(buffer, path_and_query_string, NETDATA_WEB_REQUEST_URL_SIZE + 1);

        char *question_mark_start = strchr(buffer, '?');
        if (question_mark_start) {
            buffer_strcat(w->url_query_string_decoded, question_mark_start);
            char c = *question_mark_start;
            *question_mark_start = '\0';
            buffer_strcat(w->url_path_decoded, buffer);
            *question_mark_start = c;
        } else {
            buffer_strcat(w->url_query_string_decoded, "");
            buffer_strcat(w->url_path_decoded, buffer);
        }
    }
}

void web_client_reuse_from_cache(struct web_client *w) {
    // zero everything about it - but keep the buffers

    web_client_reset_allocations(w, false);

    // remember the pointers to the buffers
    BUFFER *b1 = w->response.data;
    BUFFER *b2 = w->response.header;
    BUFFER *b3 = w->response.header_output;
    BUFFER *b4 = w->url_path_decoded;
    BUFFER *b5 = w->url_as_received;
    BUFFER *b6 = w->url_query_string_decoded;
    BUFFER *b7 = w->payload;

    NETDATA_SSL ssl = w->ssl;

    size_t use_count = w->use_count;
    size_t *statistics_memory_accounting = w->statistics.memory_accounting;

    // zero everything
    memset(w, 0, sizeof(struct web_client));

    w->fd = -1;
    w->statistics.memory_accounting = statistics_memory_accounting;
    w->use_count = use_count;

    w->ssl = ssl;

    // restore the pointers of the buffers
    w->response.data = b1;
    w->response.header = b2;
    w->response.header_output = b3;
    w->url_path_decoded = b4;
    w->url_as_received = b5;
    w->url_query_string_decoded = b6;
    w->payload = b7;
}

struct web_client *web_client_create(size_t *statistics_memory_accounting) {
    struct web_client *w = (struct web_client *)callocz(1, sizeof(struct web_client));

    w->ssl = NETDATA_SSL_UNSET_CONNECTION;

    w->use_count = 1;
    w->statistics.memory_accounting = statistics_memory_accounting;

    w->url_as_received = buffer_create(NETDATA_WEB_DECODED_URL_INITIAL_SIZE, w->statistics.memory_accounting);
    w->url_path_decoded = buffer_create(NETDATA_WEB_DECODED_URL_INITIAL_SIZE, w->statistics.memory_accounting);
    w->url_query_string_decoded = buffer_create(NETDATA_WEB_DECODED_URL_INITIAL_SIZE, w->statistics.memory_accounting);
    w->response.data = buffer_create(NETDATA_WEB_RESPONSE_INITIAL_SIZE, w->statistics.memory_accounting);
    w->response.header = buffer_create(NETDATA_WEB_RESPONSE_HEADER_INITIAL_SIZE, w->statistics.memory_accounting);
    w->response.header_output = buffer_create(NETDATA_WEB_RESPONSE_HEADER_INITIAL_SIZE, w->statistics.memory_accounting);

    __atomic_add_fetch(w->statistics.memory_accounting, sizeof(struct web_client), __ATOMIC_RELAXED);

    return w;
}

void web_client_free(struct web_client *w) {
    netdata_ssl_close(&w->ssl);

    web_client_reset_allocations(w, true);

    __atomic_sub_fetch(w->statistics.memory_accounting, sizeof(struct web_client), __ATOMIC_RELAXED);
    freez(w);
}

inline void web_client_timeout_checkpoint_init(struct web_client *w) {
    now_monotonic_high_precision_timeval(&w->timings.tv_in);
}

inline void web_client_timeout_checkpoint_set(struct web_client *w, int timeout_ms) {
    w->timings.timeout_ut = timeout_ms * USEC_PER_MS;

    if(!w->timings.tv_in.tv_sec)
        web_client_timeout_checkpoint_init(w);

    if(!w->timings.tv_timeout_last_checkpoint.tv_sec)
        w->timings.tv_timeout_last_checkpoint = w->timings.tv_in;
}

inline usec_t web_client_timeout_checkpoint(struct web_client *w) {
    struct timeval now;
    now_monotonic_high_precision_timeval(&now);

    if (!w->timings.tv_timeout_last_checkpoint.tv_sec)
        w->timings.tv_timeout_last_checkpoint = w->timings.tv_in;

    usec_t since_last_check_ut = dt_usec(&w->timings.tv_timeout_last_checkpoint, &now);

    w->timings.tv_timeout_last_checkpoint = now;

    return since_last_check_ut;
}

inline usec_t web_client_timeout_checkpoint_response_ready(struct web_client *w, usec_t *usec_since_last_checkpoint) {
    usec_t since_last_check_ut = web_client_timeout_checkpoint(w);
    if(usec_since_last_checkpoint)
        *usec_since_last_checkpoint = since_last_check_ut;

    w->timings.tv_ready = w->timings.tv_timeout_last_checkpoint;

    // return the total time of the query
    return dt_usec(&w->timings.tv_in, &w->timings.tv_ready);
}

inline bool web_client_timeout_checkpoint_and_check(struct web_client *w, usec_t *usec_since_last_checkpoint) {

    usec_t since_last_check_ut = web_client_timeout_checkpoint(w);
    if(usec_since_last_checkpoint)
        *usec_since_last_checkpoint = since_last_check_ut;

    if(!w->timings.timeout_ut)
        return false;

    usec_t since_reception_ut = dt_usec(&w->timings.tv_in, &w->timings.tv_timeout_last_checkpoint);
    if (since_reception_ut >= w->timings.timeout_ut) {
        buffer_flush(w->response.data);
        buffer_strcat(w->response.data, "Query timeout exceeded");
        w->response.code = HTTP_RESP_GATEWAY_TIMEOUT;
        return true;
    }

    return false;
}
