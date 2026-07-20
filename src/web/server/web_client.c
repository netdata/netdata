// SPDX-License-Identifier: GPL-3.0-or-later

#include "web_client.h"
#include "web_server.h"
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

static inline void web_client_reset_zchunk_send_state(struct web_client *w) {
    w->response.zchunk_header_len = 0;
    w->response.zchunk_header_sent = 0;
    w->response.zchunk_suffix_sent = 0;
    w->response.zchunk_finalize_sent = 0;
    w->response.zchunk_header[0] = '\0';
}

static inline char *strip_control_characters(char *url) {
    if(!url) return "";

    for(char *s = url; *s ;s++)
        if(iscntrl((uint8_t)*s)) *s = ' ';

    return url;
}

static inline void web_client_reset_or_recreate_buffer(
    BUFFER **wb, size_t initial_size, size_t cache_max_size, size_t *statistics)
{
    if((*wb)->size > cache_max_size) {
        buffer_free(*wb);
        *wb = buffer_create(initial_size, statistics);
    }
    else
        buffer_reset(*wb);
}

static void web_client_reset_allocations(struct web_client *w, bool free_all) {

    if(free_all) {
        // the web client is to be destroyed

        buffer_free(w->url_as_received);
        w->url_as_received = NULL;

        buffer_free(w->url_for_logging);
        w->url_for_logging = NULL;

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

        web_client_reset_or_recreate_buffer(&w->url_as_received,
                                            NETDATA_WEB_DECODED_URL_INITIAL_SIZE,
                                            NETDATA_WEB_CLIENT_CACHE_MAX_BUFFER_SIZE,
                                            w->statistics.memory_accounting);
        web_client_reset_or_recreate_buffer(&w->url_for_logging,
                                            NETDATA_WEB_DECODED_URL_INITIAL_SIZE,
                                            NETDATA_WEB_CLIENT_CACHE_MAX_BUFFER_SIZE,
                                            w->statistics.memory_accounting);
        web_client_reset_or_recreate_buffer(&w->url_path_decoded,
                                            NETDATA_WEB_DECODED_URL_INITIAL_SIZE,
                                            NETDATA_WEB_CLIENT_CACHE_MAX_BUFFER_SIZE,
                                            w->statistics.memory_accounting);
        web_client_reset_or_recreate_buffer(&w->url_query_string_decoded,
                                            NETDATA_WEB_DECODED_URL_INITIAL_SIZE,
                                            NETDATA_WEB_CLIENT_CACHE_MAX_BUFFER_SIZE,
                                            w->statistics.memory_accounting);

        web_client_reset_or_recreate_buffer(&w->response.header_output,
                                            NETDATA_WEB_RESPONSE_HEADER_INITIAL_SIZE,
                                            NETDATA_WEB_CLIENT_CACHE_MAX_BUFFER_SIZE,
                                            w->statistics.memory_accounting);
        web_client_reset_or_recreate_buffer(&w->response.header,
                                            NETDATA_WEB_RESPONSE_HEADER_INITIAL_SIZE,
                                            NETDATA_WEB_CLIENT_CACHE_MAX_BUFFER_SIZE,
                                            w->statistics.memory_accounting);
        web_client_reset_or_recreate_buffer(&w->response.data,
                                            NETDATA_WEB_RESPONSE_INITIAL_SIZE,
                                            NETDATA_WEB_CLIENT_CACHE_MAX_BUFFER_SIZE,
                                            w->statistics.memory_accounting);

        if(w->payload) {
            if(w->payload->size > NETDATA_WEB_CLIENT_CACHE_MAX_BUFFER_SIZE) {
                buffer_free(w->payload);
                w->payload = NULL;
            }
            else
                buffer_reset(w->payload);
        }

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

    memset(w->mcp_session_id, 0, sizeof(w->mcp_session_id));

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
        web_client_reset_zchunk_send_state(w);
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
    web_client_clear_mcp_preview_key(w);
    web_client_flag_clear(w, WEB_CLIENT_ENCODING_GZIP|WEB_CLIENT_ENCODING_DEFLATE);
    web_client_flag_clear(w, WEB_CLIENT_FLAG_ACCEPT_JSON |
                             WEB_CLIENT_FLAG_ACCEPT_SSE |
                             WEB_CLIENT_FLAG_ACCEPT_TEXT);
    web_client_reset_path_flags(w);
}

void web_client_reset_allocations_for_reuse(struct web_client *w) {
    web_client_reset_allocations(w, false);
}

void web_client_log_completed_request(struct web_client *w, bool update_web_stats) {
    struct timeval tv;
    now_monotonic_high_precision_timeval(&tv);

    size_t size = w->response.data->len;
    size_t sent = w->response.zoutput ? (size_t)w->response.zstream.total_out : size;

    usec_t prep_ut = w->timings.tv_ready.tv_sec ? dt_usec(&w->timings.tv_ready, &w->timings.tv_in) : 0;
    usec_t sent_ut = w->timings.tv_ready.tv_sec ? dt_usec(&tv, &w->timings.tv_ready) : 0;
    usec_t total_ut = dt_usec(&tv, &w->timings.tv_in);
    strip_control_characters((char *)buffer_tostring(w->url_for_logging));

    ND_LOG_STACK lgs[] = {
            ND_LOG_FIELD_U64(NDF_CONNECTION_ID, w->id),
            ND_LOG_FIELD_UUID(NDF_TRANSACTION_ID, &w->transaction),
            ND_LOG_FIELD_TXT(NDF_NIDL_NODE, w->client_host),
            ND_LOG_FIELD_TXT(NDF_REQUEST_METHOD, HTTP_REQUEST_MODE_2str(w->mode)),
            ND_LOG_FIELD_BFR(NDF_REQUEST, w->url_for_logging),
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
    if(likely(buffer_strlen(w->url_for_logging))) {
        nd_log(NDLS_ACCESS, prio, NULL);

        if(update_web_stats)
            pulse_web_request_completed(
                dt_usec(&tv, &w->timings.tv_in), w->statistics.received_bytes, w->statistics.sent_bytes, size, sent);
    }
}

static inline void web_client_request_parser_reset(struct web_client *w) {
    w->header_parse_last_size = 0;
    w->request_header_length = 0;
    w->request_content_length = 0;
    w->request_content_type = CT_TEXT_PLAIN;
    w->request_content_length_valid = false;
    w->request_too_large = false;
}

void web_client_request_done(struct web_client *w) {
    sock_setcork(w->fd, false);

    netdata_log_debug(D_WEB_CLIENT, "%llu: Resetting client.", w->id);

    web_client_log_completed_request(w, true);
    web_client_reset_allocations_for_reuse(w);

    w->mode = HTTP_REQUEST_MODE_GET;

    web_client_disable_donottrack(w);
    web_client_disable_tracking_required(w);
    web_client_disable_keepalive(w);

    // Clear URL-derived flags between requests. PATH_IS_MCP is re-set
    // during the next URL decode; clearing it here makes sure a keepalive
    // connection cannot carry the previous request's classification into
    // a new request that fails before URL decoding runs (e.g., malformed
    // request line, unsupported method).
    web_client_flag_clear(w, WEB_CLIENT_FLAG_PATH_IS_MCP);

    web_client_request_parser_reset(w);
    w->request_ingress_started_ut = 0;

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
    size_t url_len = buffer_strlen(w->url_as_received);
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
        const char *e = b + url_len;
        if(e > b) e--;
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
    char web_path[FILENAME_MAX];
    snprintfz(web_path, sizeof(web_path), "%s/%s", netdata_configured_web_dir, filename);
#if defined(OS_WINDOWS)
    char display_path[FILENAME_MAX];
    netdata_log_debug(D_WEB_CLIENT, "%llu: Looking for file '%s'", w->id,
                      os_translate_path(display_path, web_path, sizeof(display_path)));
#else
    netdata_log_debug(D_WEB_CLIENT, "%llu: Looking for file '%s'", w->id, web_path);
#endif

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

    // Avoid open-time effects for known special objects; O_NONBLOCK covers FIFO replacement races.
    int fd = -1;
    if(!S_ISREG(statbuf.st_mode))
        errno = EINVAL;
    else
        fd = open(web_filename, O_RDONLY | O_CLOEXEC | O_NONBLOCK);

    if(fd != -1 && fstat(fd, &statbuf) != 0) {
        int saved_errno = errno;
        close(fd);
        errno = saved_errno;
        fd = -1;
    }

    if(fd != -1 && !S_ISREG(statbuf.st_mode)) {
        close(fd);
        errno = EINVAL;
        fd = -1;
    }

    if(fd != -1 && unlikely(statbuf.st_size < 0 || (uintmax_t)statbuf.st_size > UINT32_MAX - 2)) {
        close(fd);
        errno = EFBIG;
        fd = -1;
    }

    if(fd != -1) {
        size_t file_size = (size_t)statbuf.st_size;

        buffer_flush(w->response.data);
        buffer_need_bytes(w->response.data, file_size + 1);

        size_t bytes_read = 0;
        while(bytes_read < file_size) {
            size_t bytes_to_read = file_size - bytes_read;
            if(bytes_to_read > (size_t)SSIZE_MAX)
                bytes_to_read = (size_t)SSIZE_MAX;

            ssize_t r = read(fd, &w->response.data->buffer[bytes_read], bytes_to_read);
            if(likely(r > 0)) {
                bytes_read += (size_t)r;
                continue;
            }

            if(unlikely(r == -1 && errno == EINTR))
                continue;

            if(r == 0)
                errno = EIO;

            // cannot read the whole file
            nd_log(NDLS_DAEMON, NDLP_ERR, "Web server failed to read file '%s'", web_filename);
            int saved_errno = errno;
            close(fd);
            errno = saved_errno;
            fd = -1;
            break;
        }

        if(fd != -1) {
            w->response.data->len = bytes_read;
            w->response.data->buffer[w->response.data->len] = '\0';
        }
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
            ND_LOG_FIELD_BFR(NDF_REQUEST, w->url_for_logging),
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
                                       buffer_strlen(w->url_as_received),
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


typedef struct web_client_http_method {
    const char *text;
    size_t length;
    HTTP_REQUEST_MODE mode;
} WEB_CLIENT_HTTP_METHOD;

static const WEB_CLIENT_HTTP_METHOD web_client_http_methods[] = {
    { "GET ",     4, HTTP_REQUEST_MODE_GET },
    { "OPTIONS ", 8, HTTP_REQUEST_MODE_OPTIONS },
    { "POST ",    5, HTTP_REQUEST_MODE_POST },
    { "PUT ",     4, HTTP_REQUEST_MODE_PUT },
    { "DELETE ",  7, HTTP_REQUEST_MODE_DELETE },
    { "STREAM ",  7, HTTP_REQUEST_MODE_STREAM },
};

static inline void web_client_request_validation_end(struct web_client *w) {
    web_client_request_parser_reset(w);
    web_client_disable_wait_receive(w);
}

static inline char *web_client_valid_method(
    struct web_client *w, char *s, size_t length, bool *incomplete)
{
    *incomplete = false;

    const WEB_CLIENT_HTTP_METHOD *matched = NULL;
    for(size_t i = 0; i < _countof(web_client_http_methods); i++) {
        const WEB_CLIENT_HTTP_METHOD *method = &web_client_http_methods[i];
        size_t comparable = MIN(length, method->length);
        if(memcmp(s, method->text, comparable) != 0)
            continue;

        if(length < method->length) {
            *incomplete = true;
            continue;
        }

        matched = method;
        s += method->length;
        w->mode = method->mode;
        break;
    }

    if(!matched)
        return NULL;

    if(matched->mode == HTTP_REQUEST_MODE_STREAM) {
        if (!SSL_connection(&w->ssl) && http_is_using_ssl_force(w)) {
            web_client_request_validation_end(w);

            char hostname[256];
            char *copyme = strstr(s,"hostname=");
            if ( copyme ){
                copyme += 9;
                char *end = strchr(copyme,'&');
                if(end){
                    size_t hostname_length = MIN(255, end - copyme);
                    memcpy(hostname, copyme, hostname_length);
                    hostname[hostname_length] = 0X00;
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
    }

    return s;
}

static const char *web_client_request_target_start(const char *request, size_t length) {
    for(size_t i = 0; i < _countof(web_client_http_methods); i++) {
        const WEB_CLIENT_HTTP_METHOD *method = &web_client_http_methods[i];
        if(length >= method->length && !memcmp(request, method->text, method->length))
            return request + method->length;
    }

    return NULL;
}

static size_t web_client_request_target_for_logging(const char *request, size_t length) {
    const char *target = web_client_request_target_start(request, length);
    if(!target)
        return 0;

    const char *request_end = request + length;
    const char *request_line_end = memchr(target, '\r', (size_t)(request_end - target));
    if(!request_line_end)
        request_line_end = request_end;

    const char *protocol = url_find_protocol((char *)target, request_line_end);
    if(protocol < request_line_end && *protocol)
        return (size_t)(protocol - target);

    return (size_t)(request_line_end - target);
}

static size_t web_client_find_header_end(const char *request, size_t length, size_t previous_length) {
    size_t start = MIN(length, previous_length);
    start = start > 3 ? start - 3 : 0;

    for(size_t i = start; i + 3 < length; i++) {
        if(request[i] == '\r' && request[i + 1] == '\n' &&
           request[i + 2] == '\r' && request[i + 3] == '\n')
            return i + 4;
    }

    return 0;
}

static const char *web_client_find_header_value(
    const char *request, size_t header_length, const char *name, size_t name_length, const char **value_end)
{
    const char *header_end = request + header_length;
    const char *line = request;
    bool request_line = true;

    while(line + 1 < header_end) {
        const char *line_end = line;
        while(line_end + 1 < header_end && (line_end[0] != '\r' || line_end[1] != '\n'))
            line_end++;
        if(line_end + 1 >= header_end)
            break;

        if(!request_line && line_end == line)
            break;

        if(!request_line && (size_t)(line_end - line) >= name_length &&
           strncasecmp(line, name, name_length) == 0) {
            *value_end = line_end;
            return line + name_length;
        }

        request_line = false;
        line = line_end + 2;
    }

    return NULL;
}

static void web_client_parse_content_metadata(struct web_client *w, const char *request) {
    w->request_content_length_valid = false;
    w->request_content_type = CT_TEXT_PLAIN;

    const char content_length_header[] = "Content-Length: ";
    const char *value_end = NULL;
    const char *cl = web_client_find_header_value(
        request, w->request_header_length, content_length_header, sizeof(content_length_header) - 1, &value_end);
    if(!cl)
        return;

    while(cl < value_end && (*cl == ' ' || *cl == '\t'))
        cl++;
    if(cl >= value_end || !isdigit((uint8_t)*cl))
        return;

    size_t content_length = 0;
    while(cl < value_end && isdigit((uint8_t)*cl)) {
        size_t digit = (size_t)(*cl - '0');
        if(content_length > (SIZE_MAX - digit) / 10)
            return;
        content_length = content_length * 10 + digit;
        cl++;
    }

    while(cl < value_end && (*cl == ' ' || *cl == '\t'))
        cl++;
    if(cl != value_end)
        return;

    w->request_content_length = content_length;
    w->request_content_length_valid = true;

    const char content_type_header[] = "Content-Type: ";
    value_end = NULL;
    const char *ct = web_client_find_header_value(
        request, w->request_header_length, content_type_header, sizeof(content_type_header) - 1, &value_end);
    if(!ct)
        return;

    while(ct < value_end && isspace((uint8_t)*ct))
        ct++;
    const char *ct_end = ct;
    while(ct_end < value_end && !isspace((uint8_t)*ct_end) && *ct_end != ';')
        ct_end++;

    size_t content_type_length = (size_t)(ct_end - ct);
    if(!content_type_length)
        return;

    CLEAN_CHAR_P *content_type = mallocz(content_type_length + 1);
    memcpy(content_type, ct, content_type_length);
    content_type[content_type_length] = '\0';
    w->request_content_type = content_type_string2id(content_type);
}

static bool web_client_request_complete_and_extract_payload(
    struct web_client *w, const char *request, size_t length, size_t previous_length)
{
    if(!w->request_header_length) {
        w->request_header_length = web_client_find_header_end(request, length, previous_length);
        if(!w->request_header_length)
            return false;

        if(w->mode == HTTP_REQUEST_MODE_POST || w->mode == HTTP_REQUEST_MODE_PUT)
            web_client_parse_content_metadata(w, request);
    }

    if(w->mode != HTTP_REQUEST_MODE_POST && w->mode != HTTP_REQUEST_MODE_PUT)
        return true;

    if(!w->request_content_length_valid)
        return false;

    size_t payload_length = length - w->request_header_length;
    if(payload_length != w->request_content_length)
        return false;

    if(!w->payload)
        w->payload = buffer_create(payload_length + 1, NULL);

    buffer_contents_replace(w->payload, &request[w->request_header_length], payload_length);
    w->payload->content_type = w->request_content_type;
    return true;
}

static bool web_client_request_ingress_timed_out(struct web_client *w) {
    usec_t now_ut = now_boottime_usec();
    if(!w->request_ingress_started_ut)
        w->request_ingress_started_ut = now_ut;

    if(web_client_first_request_timeout <= 0)
        return false;

    usec_t timeout_ut = (usec_t)web_client_first_request_timeout * USEC_PER_SEC;
    return now_ut - w->request_ingress_started_ut >= timeout_ut;
}

/**
 * Request validate
 *
 * Production callers must enforce web_client_request_size_validation() at their ingress boundary first.
 * This parser intentionally accepts every request admitted by its caller.
 *
 * @param w is the structure with the client request
 *
 * @return It returns HTTP_VALIDATION_OK on success and another code present
 *          in the enum HTTP_VALIDATION otherwise.
 */
HTTP_VALIDATION http_request_validate(struct web_client *w) {
    char *request = (char *)buffer_tostring(w->response.data), *s = request, *encoded_url = NULL;

    if(unlikely(web_client_request_ingress_timed_out(w))) {
        web_client_request_validation_end(w);
        return HTTP_VALIDATION_REQUEST_TIMEOUT;
    }

    size_t previous_length = w->header_parse_last_size;
    size_t request_length = buffer_strlen(w->response.data);
    w->header_parse_last_size = request_length;
    char *request_end = request + request_length;

    bool method_incomplete = false;
    s = web_client_valid_method(w, s, (size_t)(request_end - s), &method_incomplete);
    if (!s) {
        if(method_incomplete && !web_client_find_header_end(request, request_length, previous_length)) {
            web_client_enable_wait_receive(w);
            return HTTP_VALIDATION_INCOMPLETE;
        }

        web_client_request_validation_end(w);
        return HTTP_VALIDATION_NOT_SUPPORTED;
    }

    if(!web_client_request_complete_and_extract_payload(w, request, request_length, previous_length)) {
        web_client_enable_wait_receive(w);
        return HTTP_VALIDATION_INCOMPLETE;
    }

    //After the method we have the path and query string together
    encoded_url = s;

    //we search for the position where we have " HTTP/", because it finishes the user request
    char *request_line_end = s;
    while(request_line_end < request_end && *request_line_end && *request_line_end != '\r')
        request_line_end++;

    s = url_find_protocol(s, request_line_end);

    // incomplete requests
    if(unlikely(s >= request_line_end || !*s)) {
        web_client_enable_wait_receive(w);
        return HTTP_VALIDATION_INCOMPLETE;
    }

    // we have the end of encoded_url - remember it
    char *ue = s;
    size_t encoded_url_length = (size_t)(ue - encoded_url);

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

                HTTP_VALIDATION decode_status =
                    web_client_decode_path_and_query_string(w, encoded_url, encoded_url_length);
                if(unlikely(decode_status != HTTP_VALIDATION_OK)) {
                    web_client_request_validation_end(w);
                    return decode_status;
                }

                if ( (web_client_check_conn_tcp(w)) && (netdata_ssl_web_server_ctx) ) {
                    if (!w->ssl.conn && (http_is_using_ssl_force(w) || http_is_using_ssl_default(w)) && (w->mode != HTTP_REQUEST_MODE_STREAM)) {
                        web_client_request_validation_end(w);
                        return HTTP_VALIDATION_REDIRECT;
                    }
                }

                web_client_request_validation_end(w);
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

static inline void web_client_prepare_request_summary(struct web_client *w, const char *fallback) {
    if(!buffer_strlen(w->url_for_logging)) {
        const char *request = buffer_tostring(w->response.data);
        size_t request_length = buffer_strlen(w->response.data);
        const char *target = web_client_request_target_start(request, request_length);
        size_t target_length = web_client_request_target_for_logging(request, request_length);

        if(target && target_length)
            buffer_content_summary(w->url_for_logging, target, target_length);
        else
            buffer_sprintf(w->url_for_logging, "%s (request_bytes=%zu)", fallback, request_length);
    }

    strip_control_characters((char *)buffer_tostring(w->url_for_logging));
}

static inline ssize_t web_client_send_data(struct web_client *w,const void *buf,size_t len, int flags)
{
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

    if(bytes < 0 && (errno == EAGAIN || errno == EWOULDBLOCK)) {
        web_client_enable_wait_send(w);
        return 0;
    }

    return bytes;
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

    // MCP-specific CORS: widen the allowlist + advertise exposed
    // headers only for MCP transport endpoints (/mcp, /sse). The flag
    // is set once during URL decoding; see WEB_CLIENT_FLAG_PATH_IS_MCP.
    bool is_mcp_path = web_client_flag_check(w, WEB_CLIENT_FLAG_PATH_IS_MCP);

    if(is_mcp_path && w->mode != HTTP_REQUEST_MODE_OPTIONS)
        buffer_strcat(w->response.header_output,
                      "Access-Control-Expose-Headers: Mcp-Session-Id\r\n");

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
        // Methods, max-age, and the base header allowlist are identical
        // for every OPTIONS preflight. MCP preflights append the extra
        // request headers the SDK uses: mcp-protocol-version,
        // mcp-session-id, last-event-id (SSE resumption), authorization
        // (bearer tokens). DELETE is *not* advertised — the MCP handlers
        // currently return 405 for it, and advertising it would let the
        // preflight succeed only for the real request to fail. Add DELETE
        // here when the handlers learn session teardown.
        buffer_strcat(w->response.header_output,
                "Access-Control-Allow-Methods: GET, POST, OPTIONS\r\n"
                        "Access-Control-Allow-Headers: accept, x-requested-with, origin, content-type, cookie, pragma, cache-control, x-auth-token, x-netdata-auth, x-transaction-id");

        if(is_mcp_path)
            buffer_strcat(w->response.header_output,
                          ", authorization, mcp-protocol-version, mcp-session-id, last-event-id");

        buffer_strcat(w->response.header_output,
                      "\r\n"
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
    if(likely(w->response.zoutput))
        buffer_strcat(w->response.header_output, "Content-Encoding: gzip\r\n");

    if(likely(w->flags & WEB_CLIENT_CHUNKED_TRANSFER))
        buffer_strcat(w->response.header_output, "Transfer-Encoding: chunked\r\n");
    else {
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

struct web_client_url_hashes {
    uint32_t api;
    uint32_t host;
    uint32_t node;
    uint32_t netdata_conf;
    uint32_t v0;
    uint32_t v1;
    uint32_t v2;
    uint32_t v3;
    uint32_t mcp;
    uint32_t sse;
#ifdef NETDATA_INTERNAL_CHECKS
    uint32_t exit;
    uint32_t debug;
    uint32_t mirror;
#endif
};

static struct web_client_url_hashes web_client_url_hashes = { 0 };
static SPINLOCK web_client_url_hashes_spinlock = SPINLOCK_INITIALIZER;
static bool web_client_url_hashes_initialized = false;

static inline const struct web_client_url_hashes *web_client_url_hashes_get(void) {
    if(likely(__atomic_load_n(&web_client_url_hashes_initialized, __ATOMIC_ACQUIRE)))
        return &web_client_url_hashes;

    spinlock_lock(&web_client_url_hashes_spinlock);

    if(unlikely(!__atomic_load_n(&web_client_url_hashes_initialized, __ATOMIC_ACQUIRE))) {
        web_client_url_hashes.api = simple_hash("api");
        web_client_url_hashes.host = simple_hash("host");
        web_client_url_hashes.node = simple_hash("node");
        web_client_url_hashes.netdata_conf = simple_hash("netdata.conf");
        web_client_url_hashes.v0 = simple_hash("v0");
        web_client_url_hashes.v1 = simple_hash("v1");
        web_client_url_hashes.v2 = simple_hash("v2");
        web_client_url_hashes.v3 = simple_hash("v3");
        web_client_url_hashes.mcp = simple_hash("mcp");
        web_client_url_hashes.sse = simple_hash("sse");
#ifdef NETDATA_INTERNAL_CHECKS
        web_client_url_hashes.exit = simple_hash("exit");
        web_client_url_hashes.debug = simple_hash("debug");
        web_client_url_hashes.mirror = simple_hash("mirror");
#endif
        __atomic_store_n(&web_client_url_hashes_initialized, true, __ATOMIC_RELEASE);
    }

    spinlock_unlock(&web_client_url_hashes_spinlock);

    return &web_client_url_hashes;
}

static inline int web_client_switch_host(RRDHOST *host, struct web_client *w, char *url, bool nodeid, int (*func)(RRDHOST *, struct web_client *, char *)) {
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

            buffer_flush(w->url_path_decoded);
            buffer_strcat(w->url_path_decoded, "/");
            buffer_strcat(w->url_path_decoded, url);
            char *mutable_path = strdupz(buffer_tostring(w->url_path_decoded));
            int rc = func(host, w, mutable_path);
            freez(mutable_path);
            return rc;
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
            ND_LOG_FIELD_BFR(NDF_REQUEST, w->url_for_logging),
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

    const struct web_client_url_hashes *url_hashes = web_client_url_hashes_get();

    char *tok = strsep_skip_consecutive_separators(&decoded_url_path, "/?");
    if(likely(tok && *tok)) {
        uint32_t hash = simple_hash(tok);

        if(unlikely(hash == url_hashes->api && strcmp(tok, "api") == 0)) {
            // current API
            netdata_log_debug(D_WEB_CLIENT_ACCESS, "%llu: API request ...", w->id);
            return check_host_and_call(host, w, decoded_url_path, web_client_api_request);
        }
        else if(unlikely((hash == url_hashes->host && strcmp(tok, "host") == 0) || (hash == url_hashes->node && strcmp(tok, "node") == 0))) {
            // host switching
            netdata_log_debug(D_WEB_CLIENT_ACCESS, "%llu: host switch request ...", w->id);
            return web_client_switch_host(host, w, decoded_url_path, hash == url_hashes->node, web_client_api_request_with_node_selection);
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

    const struct web_client_url_hashes *url_hashes = web_client_url_hashes_get();

    // keep a copy of the decoded path, in case we need to serve it as a filename
    char filename[FILENAME_MAX + 1];
    strncpyz(filename, decoded_url_path ? decoded_url_path : "", FILENAME_MAX);

    char *tok = strsep_skip_consecutive_separators(&decoded_url_path, "/?");
    if(likely(tok && *tok)) {
        uint32_t hash = simple_hash(tok);
        netdata_log_debug(D_WEB_CLIENT, "%llu: Processing command '%s'.", w->id, tok);

        if(likely(hash == url_hashes->api && strcmp(tok, "api") == 0)) {                           // current API
            netdata_log_debug(D_WEB_CLIENT_ACCESS, "%llu: API request ...", w->id);
            return check_host_and_call(host, w, decoded_url_path, web_client_api_request);
        }
        else if(likely(hash == url_hashes->mcp && strcmp(tok, "mcp") == 0)) {
            if(unlikely(!http_can_access_mcp(w)))
                return web_client_permission_denied_acl(w);
            return mcp_http_handle_request(host, w);
        }
        else if(likely(hash == url_hashes->sse && strcmp(tok, "sse") == 0)) {
            if(unlikely(!http_can_access_mcp(w)))
                return web_client_permission_denied_acl(w);
            return mcp_sse_handle_request(host, w);
        }
        else if(unlikely((hash == url_hashes->host && strcmp(tok, "host") == 0) || (hash == url_hashes->node && strcmp(tok, "node") == 0))) { // host switching
            netdata_log_debug(D_WEB_CLIENT_ACCESS, "%llu: host switch request ...", w->id);
            return web_client_switch_host(host, w, decoded_url_path, hash == url_hashes->node, web_client_process_url);
        }
        else if(unlikely(hash == url_hashes->v3 && strcmp(tok, "v3") == 0)) {
            if(web_client_flag_check(w, WEB_CLIENT_FLAG_PATH_WITH_VERSION))
                return bad_request_multiple_dashboard_versions(w);
            web_client_flag_set(w, WEB_CLIENT_FLAG_PATH_IS_V3);
            return web_client_process_url(host, w, decoded_url_path);
        }
        else if(unlikely(hash == url_hashes->v2 && strcmp(tok, "v2") == 0)) {
            if(web_client_flag_check(w, WEB_CLIENT_FLAG_PATH_WITH_VERSION))
                return bad_request_multiple_dashboard_versions(w);
            web_client_flag_set(w, WEB_CLIENT_FLAG_PATH_IS_V2);
            return web_client_process_url(host, w, decoded_url_path);
        }
        else if(unlikely(hash == url_hashes->v1 && strcmp(tok, "v1") == 0)) {
            if(web_client_flag_check(w, WEB_CLIENT_FLAG_PATH_WITH_VERSION))
                return bad_request_multiple_dashboard_versions(w);
            web_client_flag_set(w, WEB_CLIENT_FLAG_PATH_IS_V1);
            return web_client_process_url(host, w, decoded_url_path);
        }
        else if(unlikely(hash == url_hashes->v0 && strcmp(tok, "v0") == 0)) {
            if(web_client_flag_check(w, WEB_CLIENT_FLAG_PATH_WITH_VERSION))
                return bad_request_multiple_dashboard_versions(w);
            web_client_flag_set(w, WEB_CLIENT_FLAG_PATH_IS_V0);
            return web_client_process_url(host, w, decoded_url_path);
        }
        else if(unlikely(hash == url_hashes->netdata_conf && strcmp(tok, "netdata.conf") == 0)) {    // netdata.conf
            if(unlikely(!http_can_access_netdataconf(w)))
                return web_client_permission_denied_acl(w);

            netdata_log_debug(D_WEB_CLIENT_ACCESS, "%llu: generating netdata.conf ...", w->id);
            w->response.data->content_type = CT_TEXT_PLAIN;
            buffer_flush(w->response.data);

            inicfg_generate(&netdata_config, w->response.data, 0, true);
            return HTTP_RESP_OK;
        }
#ifdef NETDATA_INTERNAL_CHECKS
        else if(unlikely(hash == url_hashes->exit && strcmp(tok, "exit") == 0)) {
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
        else if(unlikely(hash == url_hashes->debug && strcmp(tok, "debug") == 0)) {
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
        else if(unlikely(hash == url_hashes->mirror && strcmp(tok, "mirror") == 0)) {
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
            ND_LOG_FIELD_BFR(NDF_REQUEST, w->url_for_logging),
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

    HTTP_VALIDATION validation;
    if(unlikely(w->request_too_large)) {
        validation = HTTP_VALIDATION_REQUEST_TOO_LARGE;
        web_client_request_validation_end(w);
    }
    else
        validation = http_request_validate(w);

    switch(validation) {
        case HTTP_VALIDATION_OK:
            if(!web_client_flag_check(w, WEB_CLIENT_FLAG_PROGRESS_TRACKING)) {
                web_client_flag_set(w, WEB_CLIENT_FLAG_PROGRESS_TRACKING);
                query_progress_start_or_update(&w->transaction, 0, w->mode, w->acl,
                                               buffer_tostring(w->url_as_received),
                                               buffer_strlen(w->url_as_received),
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
                    // Coarse gate: allow handshake only for clients that can access at least one WebSocket surface.
                    // websocket_handle_handshake() performs the protocol-specific ACL check (MCP vs dashboard).
                    if(unlikely(!http_can_access_dashboard(w) && !http_can_access_mcp(w))) {
                        web_client_permission_denied_acl(w);
                        return;
                    }

                    w->response.code = websocket_handle_handshake(w);

                    if(w->response.code == HTTP_RESP_WEBSOCKET_HANDSHAKE)
                        // socket taken over successfully, handshake response already sent
                        return;

                    // handshake failed - fall through to send the HTTP error response
                    break;

                case HTTP_REQUEST_MODE_OPTIONS: {
                    // Path-aware coarse pre-filter:
                    // MCP ACL is accepted only for MCP endpoints (/mcp, /sse), not as generic API access.
                    // Reuse the canonical classification set at URL-decode time (see
                    // WEB_CLIENT_FLAG_PATH_IS_MCP) so the prefilter cannot drift from the dispatcher.
                    bool mcp_route_requested = web_client_flag_check(w, WEB_CLIENT_FLAG_PATH_IS_MCP);
                    if(unlikely(
                            !http_can_access_dashboard(w) &&
                            !http_can_access_registry(w) &&
                            !http_can_access_badges(w) &&
                            !http_can_access_mgmt(w) &&
                            !http_can_access_netdataconf(w) &&
                            !(mcp_route_requested && http_can_access_mcp(w))
                    )) {
                        web_client_permission_denied_acl(w);
                        break;
                    }

                    w->response.data->content_type = CT_TEXT_PLAIN;
                    buffer_flush(w->response.data);
                    buffer_strcat(w->response.data, "OK");
                    w->response.code = HTTP_RESP_OK;
                    break;
                }

                case HTTP_REQUEST_MODE_POST:
                case HTTP_REQUEST_MODE_GET:
                case HTTP_REQUEST_MODE_PUT:
                case HTTP_REQUEST_MODE_DELETE: {
                    // Path-aware coarse pre-filter:
                    // MCP ACL may open only MCP routes, while all other routes still require their own ACL surface.
                    // Reuse the canonical classification set at URL-decode time (see
                    // WEB_CLIENT_FLAG_PATH_IS_MCP) so the prefilter cannot drift from the dispatcher.
                    bool mcp_route_requested = web_client_flag_check(w, WEB_CLIENT_FLAG_PATH_IS_MCP);
                    if(unlikely(
                            !http_can_access_dashboard(w) &&
                            !http_can_access_registry(w) &&
                            !http_can_access_badges(w) &&
                            !http_can_access_mgmt(w) &&
                            !http_can_access_netdataconf(w) &&
                            !(mcp_route_requested && http_can_access_mcp(w))
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
                    while (e > s + 1) {
                        e--;
                        if (*e == '/')
                            break;
                        if(*e == '.') {
                            web_client_flag_set(w, WEB_CLIENT_FLAG_PATH_HAS_FILE_EXTENSION);
                            break;
                        }
                    }

                    w->response.code = (short)web_client_process_url(localhost, w, path);
                    break;
                }

                default:
                    web_client_permission_denied_acl(w);
                    return;
            }
            break;

        case HTTP_VALIDATION_INCOMPLETE:
            // wait for more data
            // set to normal to prevent web_server_rcv_callback
            // from going into stream mode
            if (w->mode == HTTP_REQUEST_MODE_STREAM || w->mode == HTTP_REQUEST_MODE_WEBSOCKET)
                w->mode = HTTP_REQUEST_MODE_GET;
            return;

        case HTTP_VALIDATION_REQUEST_TOO_LARGE: {
            size_t request_length = buffer_strlen(w->response.data);
            web_client_prepare_request_summary(w, "request too large");
            buffer_flush(w->url_as_received);
            buffer_strcat(w->url_as_received, "request too large");
            buffer_flush(w->response.data);
            buffer_sprintf(w->response.data,
                           "Request is too large (received at least %zu bytes, maximum is %zu bytes).\r\n",
                           request_length,
                           (size_t)NETDATA_WEB_REQUEST_MAX_SIZE);
            w->response.code = http_validation_error_to_response_code(HTTP_VALIDATION_REQUEST_TOO_LARGE);
            break;
        }

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
            web_client_prepare_request_summary(w, "malformed request target");
            netdata_log_debug(
                D_WEB_CLIENT_ACCESS, "%llu: Malformed URL '%s'.", w->id, buffer_tostring(w->url_for_logging));

            buffer_flush(w->response.data);
            buffer_strcat(w->response.data, "Malformed URL...\r\n");
            w->response.code = http_validation_error_to_response_code(HTTP_VALIDATION_MALFORMED_URL);
            break;
        case HTTP_VALIDATION_REQUEST_TIMEOUT:
            web_client_prepare_request_summary(w, "request timeout");
            netdata_log_debug(
                D_WEB_CLIENT_ACCESS,
                "%llu: Timed out while receiving request '%s'.",
                w->id,
                buffer_tostring(w->url_for_logging));

            buffer_flush(w->response.data);
            buffer_strcat(w->response.data, "Request timeout while receiving request.\r\n");
            w->response.code = HTTP_RESP_REQUEST_TIMEOUT;
            break;
        case HTTP_VALIDATION_NOT_SUPPORTED:
            web_client_prepare_request_summary(w, "unsupported request method");
            netdata_log_debug(
                D_WEB_CLIENT_ACCESS,
                "%llu: HTTP method requested is not supported '%s'.",
                w->id,
                buffer_tostring(w->url_for_logging));

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

static inline ssize_t web_client_send_chunk_frame(struct web_client *w, const char *buf, size_t len, size_t *sent)
{
    ssize_t total = 0;

    while(*sent < len) {
        ssize_t bytes = web_client_send_data(w, &buf[*sent], len - *sent, 0);
        if(bytes > 0) {
            *sent += bytes;
            total += bytes;
            w->statistics.sent_bytes += bytes;
        }
        else if(bytes == 0) {
            web_client_enable_wait_send(w);
            return total;
        }
        else
            return bytes;
    }

    return total;
}

ssize_t web_client_send_chunk_header(struct web_client *w, size_t len)
{
    netdata_log_debug(D_DEFLATE, "%llu: OPEN CHUNK of %zu bytes (hex: %zx).", w->id, len, len);
    if(w->response.zchunk_header_len == 0) {
        int bytes = snprintf(w->response.zchunk_header, sizeof(w->response.zchunk_header), "%zX\r\n", len);
        if(bytes < 0 || (size_t)bytes >= sizeof(w->response.zchunk_header)) {
            netdata_log_debug(D_WEB_CLIENT, "%llu: Failed to prepare chunk header.", w->id);
            WEB_CLIENT_IS_DEAD(w);
            return -1;
        }
        w->response.zchunk_header_len = (size_t)bytes;
        w->response.zchunk_header_sent = 0;
    }

    ssize_t bytes = web_client_send_chunk_frame(
        w, w->response.zchunk_header, w->response.zchunk_header_len, &w->response.zchunk_header_sent);
    if(bytes > 0) {
        netdata_log_debug(D_DEFLATE, "%llu: Sent chunk header %zd bytes.", w->id, bytes);
        if(w->response.zchunk_header_sent == w->response.zchunk_header_len) {
            w->response.zchunk_header_len = 0;
            w->response.zchunk_header_sent = 0;
            w->response.zchunk_header[0] = '\0';
        }
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
    bytes = web_client_send_chunk_frame(w, "\r\n", 2, &w->response.zchunk_suffix_sent);
    if(bytes > 0) {
        netdata_log_debug(D_DEFLATE, "%llu: Sent chunk suffix %zd bytes.", w->id, bytes);
        if(w->response.zchunk_suffix_sent == 2)
            w->response.zchunk_suffix_sent = 0;
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
    bytes = web_client_send_chunk_frame(w, "\r\n0\r\n\r\n", 7, &w->response.zchunk_finalize_sent);
    if(bytes > 0) {
        netdata_log_debug(D_DEFLATE, "%llu: Sent chunk suffix %zd bytes.", w->id, bytes);
        if(w->response.zchunk_finalize_sent == 7)
            w->response.zchunk_finalize_sent = 0;
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
    bool completed_pending_suffix = false;

    // when using compression,
    // w->response.sent is the amount of bytes passed through compression

    netdata_log_debug(D_DEFLATE,
        "%llu: web_client_send_deflate(): w->response.data->len = %zu, w->response.sent = %zu, w->response.zhave = %zu, w->response.zsent = %zu, w->response.zstream.avail_in = %u, w->response.zstream.avail_out = %u, w->response.zstream.total_in = %lu, w->response.zstream.total_out = %lu.",
        w->id, (size_t)w->response.data->len, w->response.sent, w->response.zhave, w->response.zsent, w->response.zstream.avail_in, w->response.zstream.avail_out, w->response.zstream.total_in, w->response.zstream.total_out);

    if(w->response.zchunk_suffix_sent) {
        t = web_client_send_chunk_close(w);
        if(t <= 0 || w->response.zchunk_suffix_sent)
            return t;

        completed_pending_suffix = true;
    }

    if(w->response.zchunk_header_len) {
        t = web_client_send_chunk_header(w, w->response.zhave);
        if(t < 0 || w->response.zchunk_header_len)
            return t;
    }

    if(w->response.data->len - w->response.sent == 0 && w->response.zstream.avail_in == 0 && w->response.zhave == w->response.zsent && w->response.zstream.avail_out != 0) {
        // there is nothing to send

        netdata_log_debug(D_WEB_CLIENT, "%llu: Out of output data.", w->id);

        // finalize the chunk
        if(w->response.sent != 0) {
            // A pending inter-chunk suffix is also the prefix of the final marker.
            if(completed_pending_suffix && w->response.zchunk_finalize_sent == 0)
                w->response.zchunk_finalize_sent = 2;

            ssize_t t2 = web_client_send_chunk_finalize(w);
            if(t2 < 0)
                return t2;

            t += t2;
            if(t2 == 0 || w->response.zchunk_finalize_sent)
                return t;
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
        // Skip this if the pending suffix was completed at function entry.
        if(w->response.sent != 0 && !completed_pending_suffix) {
            t = web_client_send_chunk_close(w);
            if(t <= 0 || w->response.zchunk_suffix_sent)
                return t;
        }

        netdata_log_debug(D_DEFLATE, "%llu: Compressing %zu new bytes starting from %zu (and %u left behind).", w->id, (w->response.data->len - w->response.sent), w->response.sent, w->response.zstream.avail_in);

        // give the compressor all the data not passed through the compressor yet
        if(w->response.data->len > w->response.sent) {
            if(unlikely((size_t)w->response.zstream.avail_in > w->response.sent)) {
                netdata_log_error(
                    "%llu: Compression input state is inconsistent (sent %zu, avail_in %u). Closing down client.",
                    w->id, w->response.sent, w->response.zstream.avail_in);
                web_client_request_done(w);
                return -1;
            }

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
        if(w->response.zchunk_header_len)
            return t;
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

        len += t;
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
    size_t request_length = buffer_strlen(w->response.data);
    if(unlikely(request_length >= NETDATA_WEB_REQUEST_MAX_SIZE + 1)) {
        w->request_too_large = true;
        return 0;
    }

    size_t boundary_bytes_left = NETDATA_WEB_REQUEST_MAX_SIZE + 1 - request_length;
    size_t desired_free = MIN(boundary_bytes_left, (size_t)NETDATA_WEB_REQUEST_INITIAL_SIZE);

    // do we have any space for more data?
    buffer_need_bytes(w->response.data, desired_free);

    ssize_t left = (ssize_t)(w->response.data->size - w->response.data->len);
    size_t bytes_to_read = MIN((size_t)(left - 1), boundary_bytes_left);

    errno_clear();

    if ( (web_client_check_conn_tcp(w)) && (netdata_ssl_web_server_ctx) ) {
        if (SSL_connection(&w->ssl)) {
            bytes = netdata_ssl_read(&w->ssl, &w->response.data->buffer[w->response.data->len], bytes_to_read);
            web_client_enable_wait_from_ssl(w);
        }
        else {
            bytes = recv(w->fd, &w->response.data->buffer[w->response.data->len], bytes_to_read, MSG_DONTWAIT);
        }
    }
    else if(web_client_check_conn_tcp(w) || web_client_check_conn_unix(w)) {
        bytes = recv(w->fd, &w->response.data->buffer[w->response.data->len], bytes_to_read, MSG_DONTWAIT);
    }
    else // other connection methods
        bytes = -1;

    if(likely(bytes > 0)) {
        w->statistics.received_bytes += bytes;

        if(unlikely(!w->request_ingress_started_ut && !buffer_strlen(w->response.data)))
            w->request_ingress_started_ut = now_boottime_usec();

        size_t old = w->response.data->len;
        (void)old;

        w->response.data->len += bytes;
        w->response.data->buffer[w->response.data->len] = '\0';

        if(unlikely(web_client_request_size_validation(w->response.data->len) != HTTP_VALIDATION_OK))
            w->request_too_large = true;

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

// Production callers must enforce web_client_request_size_validation() before invoking this decoder.
HTTP_VALIDATION web_client_decode_path_and_query_string(struct web_client *w, const char *path_and_query_string, size_t length) {
    buffer_flush(w->url_path_decoded);
    buffer_flush(w->url_query_string_decoded);

    if(buffer_strlen(w->url_as_received) == 0) {
        // Do not overwrite these if they are already filled.
        buffer_contents_replace(w->url_as_received, path_and_query_string, length);
        buffer_content_summary(w->url_for_logging, path_and_query_string, length);
    }

    CLEAN_CHAR_P *decoded = mallocz(length + 1);
    size_t decoded_length = 0;
    URL_DECODE_STATUS decode_status =
        url_decode_r_len(decoded, length + 1, path_and_query_string, length, &decoded_length);

    if(unlikely(decode_status != URL_DECODE_OK))
        return HTTP_VALIDATION_MALFORMED_URL;

    // PATH_IS_MCP is a function of the URL alone; clear and re-derive on
    // every decode so keepalived connections reusing the same web_client
    // for a different URL see a fresh value.
    web_client_flag_clear(w, WEB_CLIENT_FLAG_PATH_IS_MCP);

    if(w->mode == HTTP_REQUEST_MODE_STREAM) {
        // in stream mode, there is no path
        buffer_contents_replace(w->url_query_string_decoded, decoded, decoded_length);
    }
    else {
        // in non-stream mode, there is a path
        // FIXME - the way this is implemented, query string params never accept the symbol &, not even encoded as %26
        // To support the symbol & in query string params, we need to turn the url_query_string_decoded into a
        // dictionary and decode each of the parameters individually.
        // OR: in url_query_string_decoded use as separator a control character that cannot appear in the URL.

        char *question_mark_start = memchr(decoded, '?', decoded_length);
        if (question_mark_start) {
            size_t path_length = (size_t)(question_mark_start - decoded);
            buffer_contents_replace(w->url_query_string_decoded, question_mark_start, decoded_length - path_length);
            buffer_contents_replace(w->url_path_decoded, decoded, path_length);
        } else {
            buffer_contents_replace(w->url_path_decoded, decoded, decoded_length);
        }

        // Classify path: set PATH_IS_MCP when the URL addresses one of
        // Netdata's MCP transport endpoints (/mcp or /sse, and their
        // subpaths). Done here — at URL-decoding time — so the flag is
        // available later both to the URL dispatcher and to the response
        // header builder, including for OPTIONS preflights which bypass
        // the dispatcher. Matching requires a path-segment boundary so a
        // hypothetical /mcpfoo does not leak through.
        const char *decoded_path = buffer_tostring(w->url_path_decoded);
        size_t decoded_path_len = buffer_strlen(w->url_path_decoded);
        if(decoded_path_len >= 4
           && (memcmp(decoded_path, "/mcp", 4) == 0 || memcmp(decoded_path, "/sse", 4) == 0)
           && (decoded_path_len == 4 || decoded_path[4] == '/'))
            web_client_flag_set(w, WEB_CLIENT_FLAG_PATH_IS_MCP);
    }

    return HTTP_VALIDATION_OK;
}

void web_client_reuse_from_cache(struct web_client *w) {
    // zero everything about it - but keep the buffers

    web_client_reset_allocations_for_reuse(w);

    // remember the pointers to the buffers
    BUFFER *b1 = w->response.data;
    BUFFER *b2 = w->response.header;
    BUFFER *b3 = w->response.header_output;
    BUFFER *b4 = w->url_path_decoded;
    BUFFER *b5 = w->url_as_received;
    BUFFER *b6 = w->url_query_string_decoded;
    BUFFER *b7 = w->payload;
    BUFFER *b8 = w->url_for_logging;

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
    w->url_for_logging = b8;
}

struct web_client *web_client_create(size_t *statistics_memory_accounting) {
    struct web_client *w = (struct web_client *)callocz(1, sizeof(struct web_client));

    w->ssl = NETDATA_SSL_UNSET_CONNECTION;

    w->use_count = 1;
    w->statistics.memory_accounting = statistics_memory_accounting;

    w->url_as_received = buffer_create(NETDATA_WEB_DECODED_URL_INITIAL_SIZE, w->statistics.memory_accounting);
    w->url_for_logging = buffer_create(NETDATA_WEB_DECODED_URL_INITIAL_SIZE, w->statistics.memory_accounting);
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

static void web_client_unittest_prepare_request(struct web_client *w, size_t request_length) {
    static const char prefix[] = "GET ";
    static const char suffix[] = " HTTP/1.1\r\n\r\n";
    internal_fatal(
        request_length < sizeof(prefix) - 1 + sizeof(suffix) - 1 + 1,
        "WEB REQUEST unittest size is too small");

    buffer_flush(w->response.data);
    buffer_strcat(w->response.data, prefix);

    size_t target_length = request_length - (sizeof(prefix) - 1) - (sizeof(suffix) - 1);
    buffer_need_bytes(w->response.data, target_length + sizeof(suffix));
    char *target = &w->response.data->buffer[w->response.data->len];
    memset(target, 'a', target_length);
    target[0] = '/';
    w->response.data->len += target_length;
    w->response.data->buffer[w->response.data->len] = '\0';
    buffer_strcat(w->response.data, suffix);
}

int web_client_request_unittest(void) {
    int errors = poll_events_unittest() + web_client_cache_unittest();
    size_t memory_accounting = 0;
    struct web_client *w = web_client_create(&memory_accounting);
    w->fd = -1;

    if(web_client_request_size_validation(NETDATA_WEB_REQUEST_MAX_SIZE - 1) != HTTP_VALIDATION_OK ||
       web_client_request_size_validation(NETDATA_WEB_REQUEST_MAX_SIZE) != HTTP_VALIDATION_OK ||
       web_client_request_size_validation(NETDATA_WEB_REQUEST_MAX_SIZE + 1) !=
           HTTP_VALIDATION_REQUEST_TOO_LARGE ||
       http_validation_error_to_response_code(HTTP_VALIDATION_REQUEST_TOO_LARGE) != HTTP_RESP_CONTENT_TOO_LONG ||
       http_validation_error_to_response_code(HTTP_VALIDATION_REQUEST_TIMEOUT) != HTTP_RESP_REQUEST_TIMEOUT ||
       http_validation_error_to_response_code(HTTP_VALIDATION_MALFORMED_URL) != HTTP_RESP_BAD_REQUEST)
        errors++;

    web_client_unittest_prepare_request(w, NETDATA_WEB_REQUEST_MAX_SIZE);
    if(http_request_validate(w) != HTTP_VALIDATION_OK ||
       buffer_strlen(w->response.data) != NETDATA_WEB_REQUEST_MAX_SIZE ||
       buffer_strlen(w->url_as_received) >= NETDATA_WEB_REQUEST_MAX_SIZE)
        errors++;

    // Boundary policy is intentionally absent from the parser and decoder.
    web_client_reuse_from_cache(w);
    web_client_unittest_prepare_request(w, NETDATA_WEB_REQUEST_MAX_SIZE + 1);
    if(http_request_validate(w) != HTTP_VALIDATION_OK ||
       buffer_strlen(w->response.data) != NETDATA_WEB_REQUEST_MAX_SIZE + 1)
        errors++;

    web_client_reuse_from_cache(w);
    static const char post_request[] =
        "POST /api/v3/test HTTP/1.1\r\n"
        "Content-Length: 13\r\n"
        "Content-Type: application/json; charset=utf-8\r\n\r\n"
        "payload-value";
    static const char put_request[] =
        "PUT /api/v3/test HTTP/1.1\r\n"
        "Content-Length: 13\r\n"
        "Content-Type: application/json; charset=utf-8\r\n\r\n"
        "payload-value";
    static const struct {
        const char *request;
        size_t length;
    } body_requests[] = {
        { post_request, sizeof(post_request) - 1 },
        { put_request, sizeof(put_request) - 1 },
    };
    for(size_t r = 0; r < _countof(body_requests); r++) {
        for(size_t split = 1; split < body_requests[r].length; split++) {
            web_client_reuse_from_cache(w);
            buffer_strncat(w->response.data, body_requests[r].request, split);
            if(http_request_validate(w) != HTTP_VALIDATION_INCOMPLETE) {
                errors++;
                continue;
            }

            buffer_strncat(
                w->response.data, &body_requests[r].request[split], body_requests[r].length - split);
            if(http_request_validate(w) != HTTP_VALIDATION_OK || !w->payload ||
               buffer_strlen(w->payload) != 13 || strcmp(buffer_tostring(w->payload), "payload-value") ||
               w->payload->content_type != CT_APPLICATION_JSON)
                errors++;
        }
    }

    web_client_reuse_from_cache(w);
    for(size_t i = 0; i < sizeof(post_request) - 1; i++) {
        buffer_strncat(w->response.data, &post_request[i], 1);
        HTTP_VALIDATION expected =
            i + 1 == sizeof(post_request) - 1 ? HTTP_VALIDATION_OK : HTTP_VALIDATION_INCOMPLETE;
        if(http_request_validate(w) != expected) {
            errors++;
            break;
        }
    }
    if(!w->payload || strcmp(buffer_tostring(w->payload), "payload-value"))
        errors++;

    // Once POST headers are complete, body fragments use only stored offsets and lengths.
    web_client_reuse_from_cache(w);
    static const char post_headers[] =
        "POST /api HTTP/1.1\r\nContent-Length: 5\r\n\r\n";
    buffer_strcat(w->response.data, post_headers);
    if(http_request_validate(w) != HTTP_VALIDATION_INCOMPLETE ||
       w->request_header_length != sizeof(post_headers) - 1 ||
       !w->request_content_length_valid || w->request_content_length != 5 ||
       w->request_content_type != CT_TEXT_PLAIN)
        errors++;

    for(size_t i = 0; i < sizeof("hello") - 1; i++) {
        size_t header_length = w->request_header_length;
        buffer_strncat(w->response.data, &"hello"[i], 1);
        HTTP_VALIDATION expected = i + 1 == sizeof("hello") - 1 ? HTTP_VALIDATION_OK : HTTP_VALIDATION_INCOMPLETE;
        if(http_request_validate(w) != expected ||
           (expected == HTTP_VALIDATION_INCOMPLETE && w->request_header_length != header_length))
            errors++;
    }
    if(!w->payload || strcmp(buffer_tostring(w->payload), "hello") ||
       w->payload->content_type != CT_TEXT_PLAIN)
        errors++;

    // Header names must match at the beginning of an actual header line.
    web_client_reuse_from_cache(w);
    buffer_strcat(
        w->response.data,
        "POST /?Content-Length:%2099 HTTP/1.1\r\nX-Content-Length: 99\r\ncontent-length: 0\r\n\r\n");
    if(http_request_validate(w) != HTTP_VALIDATION_OK || !w->payload || buffer_strlen(w->payload))
        errors++;

    // Missing or invalid Content-Length is latched and is not reparsed for later body fragments.
    static const char *invalid_content_lengths[] = {
        "POST / HTTP/1.1\r\n\r\n",
        "POST / HTTP/1.1\r\nContent-Length: invalid\r\n\r\n",
        "POST / HTTP/1.1\r\nContent-Length: -1\r\n\r\n",
        "POST / HTTP/1.1\r\nContent-Length: 184467440737095516160\r\n\r\n",
    };
    for(size_t i = 0; i < _countof(invalid_content_lengths); i++) {
        web_client_reuse_from_cache(w);
        buffer_strcat(w->response.data, invalid_content_lengths[i]);
        if(http_request_validate(w) != HTTP_VALIDATION_INCOMPLETE ||
           !w->request_header_length || w->request_content_length_valid)
            errors++;
        size_t header_length = w->request_header_length;
        buffer_strcat(w->response.data, "body");
        if(http_request_validate(w) != HTTP_VALIDATION_INCOMPLETE ||
           w->request_header_length != header_length || w->request_content_length_valid)
            errors++;
    }

    web_client_reuse_from_cache(w);
    buffer_strcat(w->response.data, "POST / HTTP/1.1\r\nContent-Length: 0\r\n\r\nx");
    if(http_request_validate(w) != HTTP_VALIDATION_INCOMPLETE ||
       !w->request_content_length_valid || w->request_content_length)
        errors++;

    web_client_reuse_from_cache(w);
    buffer_strcat(w->response.data, "PUT / HTTP/1.1\r\nContent-Length: 1048576\r\n\r\n");
    if(http_request_validate(w) != HTTP_VALIDATION_INCOMPLETE ||
       !w->request_content_length_valid || w->request_content_length != NETDATA_WEB_REQUEST_MAX_SIZE)
        errors++;

    for(size_t m = 0; m < _countof(web_client_http_methods); m++) {
        const WEB_CLIENT_HTTP_METHOD *method = &web_client_http_methods[m];
        for(size_t split = 1; split < method->length; split++) {
            web_client_reuse_from_cache(w);
            buffer_strncat(w->response.data, method->text, split);
            if(http_request_validate(w) != HTTP_VALIDATION_INCOMPLETE || !web_client_has_wait_receive(w)) {
                errors++;
                continue;
            }

            buffer_strcat(w->response.data, &method->text[split]);
            if(method->mode == HTTP_REQUEST_MODE_POST || method->mode == HTTP_REQUEST_MODE_PUT)
                buffer_strcat(w->response.data, "/ HTTP/1.1\r\nContent-Length: 0\r\n\r\n");
            else
                buffer_strcat(w->response.data, "/ HTTP/1.1\r\n\r\n");

            if(http_request_validate(w) != HTTP_VALIDATION_OK || web_client_has_wait_receive(w))
                errors++;
        }
    }

    web_client_reuse_from_cache(w);
    buffer_need_bytes(w->response.data, NETDATA_WEB_REQUEST_MAX_SIZE + 2);
    memset(w->response.data->buffer, 'a', NETDATA_WEB_REQUEST_MAX_SIZE);
    memcpy(w->response.data->buffer, "GET /", 5);
    w->response.data->len = NETDATA_WEB_REQUEST_MAX_SIZE;
    w->response.data->buffer[w->response.data->len] = '\0';
    if(http_request_validate(w) != HTTP_VALIDATION_INCOMPLETE)
        errors++;

#ifndef OS_WINDOWS
    int sockets[2];
    int response_peer_fd = -1;
    if(socketpair(AF_UNIX, SOCK_STREAM, 0, sockets) != 0)
        errors++;
    else {
        w->fd = sockets[0];
        response_peer_fd = sockets[1];
        web_client_set_conn_unix(w);
        if(send(sockets[1], "xy", 2, 0) != 2 || web_client_receive(w) != 1 ||
           buffer_strlen(w->response.data) != NETDATA_WEB_REQUEST_MAX_SIZE + 1 || !w->request_too_large)
            errors++;
    }
#else
    buffer_strcat(w->response.data, "x");
    w->request_too_large =
        web_client_request_size_validation(buffer_strlen(w->response.data)) != HTTP_VALIDATION_OK;
#endif

    web_client_process_request_from_web_server(w);
    if(w->response.code != HTTP_RESP_CONTENT_TOO_LONG ||
       !strstr(buffer_tostring(w->response.data), "received at least 1048577 bytes") ||
       w->request_too_large || web_client_has_wait_receive(w))
        errors++;

#ifndef OS_WINDOWS
    if(response_peer_fd != -1) {
        close(w->fd);
        close(response_peer_fd);
        w->fd = -1;
    }
#endif

    web_client_reuse_from_cache(w);
    int saved_ingress_timeout = web_client_first_request_timeout;
    web_client_first_request_timeout = 1;
    buffer_strcat(
        w->response.data,
        "GET /partial HTTP/1.1\r\nAuthorization: Bearer sentinel-auth-value\r\n");
    if(http_request_validate(w) != HTTP_VALIDATION_INCOMPLETE)
        errors++;
    w->request_ingress_started_ut -= 2 * USEC_PER_SEC;
    if(http_request_validate(w) != HTTP_VALIDATION_REQUEST_TIMEOUT || web_client_has_wait_receive(w))
        errors++;
    web_client_prepare_request_summary(w, "request timeout");
    if(!strstr(buffer_tostring(w->url_for_logging), "/partial") ||
       strstr(buffer_tostring(w->url_for_logging), "sentinel-auth-value") ||
       strstr(buffer_tostring(w->url_for_logging), "Authorization"))
        errors++;
    web_client_first_request_timeout = saved_ingress_timeout;

    web_client_reuse_from_cache(w);
    buffer_strcat(w->response.data, "GET /api?x=%GG HTTP/1.1\r\n\r\n");
    if(http_request_validate(w) != HTTP_VALIDATION_MALFORMED_URL || web_client_has_wait_receive(w))
        errors++;

    web_client_reuse_from_cache(w);
    buffer_strcat(
        w->response.data,
        "BOGUS / HTTP/1.1\r\nAuthorization: Bearer sentinel-auth-value\r\n"
        "Cookie: session=sentinel-cookie-value\r\n\r\n");
    if(http_request_validate(w) != HTTP_VALIDATION_NOT_SUPPORTED ||
       web_client_has_wait_receive(w))
        errors++;
    web_client_prepare_request_summary(w, "unsupported request method");
    if(!strstr(buffer_tostring(w->url_for_logging), "unsupported request method") ||
       strstr(buffer_tostring(w->url_for_logging), "sentinel-auth-value") ||
       strstr(buffer_tostring(w->url_for_logging), "sentinel-cookie-value") ||
       strstr(buffer_tostring(w->url_for_logging), "Authorization") ||
       strstr(buffer_tostring(w->url_for_logging), "Cookie"))
        errors++;

    web_client_reuse_from_cache(w);
    buffer_strcat(
        w->response.data,
        "POST /safe-target HTTP/1.1\r\nAuthorization: Bearer sentinel-auth-value\r\n"
        "Content-Length: 19\r\n\r\nsentinel-body-value");
    web_client_prepare_request_summary(w, "request too large");
    if(!strstr(buffer_tostring(w->url_for_logging), "/safe-target") ||
       strstr(buffer_tostring(w->url_for_logging), "sentinel-auth-value") ||
       strstr(buffer_tostring(w->url_for_logging), "sentinel-body-value") ||
       strstr(buffer_tostring(w->url_for_logging), "Authorization"))
        errors++;

    w->header_parse_last_size = 11;
    w->request_header_length = 22;
    w->request_content_length = 33;
    w->request_content_type = CT_APPLICATION_JSON;
    w->request_content_length_valid = true;
    w->request_too_large = true;
    w->request_ingress_started_ut = 44;
    w->fd = -1;
    web_client_request_done(w);
    if(w->header_parse_last_size || w->request_header_length || w->request_content_length ||
       w->request_content_type != CT_TEXT_PLAIN || w->request_content_length_valid ||
       w->request_too_large || w->request_ingress_started_ut)
        errors++;
    buffer_strcat(w->response.data, "GET /after-post HTTP/1.1\r\n\r\n");
    if(http_request_validate(w) != HTTP_VALIDATION_OK || w->mode != HTTP_REQUEST_MODE_GET ||
       strcmp(buffer_tostring(w->url_path_decoded), "/after-post"))
        errors++;

    w->header_parse_last_size = 11;
    w->request_header_length = 22;
    w->request_content_length = 33;
    w->request_content_type = CT_APPLICATION_JSON;
    w->request_content_length_valid = true;
    w->request_too_large = true;
    w->request_ingress_started_ut = 44;
    web_client_reuse_from_cache(w);
    if(w->header_parse_last_size || w->request_header_length || w->request_content_length ||
       w->request_content_type != CT_NONE || w->request_content_length_valid ||
       w->request_too_large || w->request_ingress_started_ut)
        errors++;

    web_client_reuse_from_cache(w);
    if(web_client_decode_path_and_query_string(w, "/api?x=1", 8) != HTTP_VALIDATION_OK ||
       strcmp(buffer_tostring(w->url_path_decoded), "/api") != 0 ||
       strcmp(buffer_tostring(w->url_query_string_decoded), "?x=1") != 0)
        errors++;

    web_client_reuse_from_cache(w);
    if(web_client_decode_path_and_query_string(w, "/api?x=%GG", 10) != HTTP_VALIDATION_MALFORMED_URL)
        errors++;

    web_client_reuse_from_cache(w);
    const char embedded_nul[] = { '/', 'a', 'p', 'i', '\0', 'x' };
    if(web_client_decode_path_and_query_string(w, embedded_nul, sizeof(embedded_nul)) !=
       HTTP_VALIDATION_MALFORMED_URL)
        errors++;

    web_client_free(w);
    if(memory_accounting)
        errors++;

    if(errors)
        fprintf(stderr, "WEB REQUEST: %d test(s) failed\n", errors);

    return errors;
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
