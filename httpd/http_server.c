// SPDX-License-Identifier: GPL-3.0-or-later

#include "daemon/common.h"
#include "http_server.h"
#include "h2o.h"

#include "h2o_utils.h"

static h2o_globalconf_t config;
static h2o_context_t ctx;
static h2o_accept_ctx_t accept_ctx;

#define CONTENT_JSON_UTF8 H2O_STRLIT("application/json; charset=utf-8")
#define CONTENT_TEXT_UTF8 H2O_STRLIT("text/plain; charset=utf-8")
#define NBUF_INITIAL_SIZE_RESP (4096)
#define API_V1_PREFIX "/api/v1/"
#define HOST_SELECT_PREFIX "/host/"

#define HTTPD_CONFIG_SECTION "httpd"
#define HTTPD_ENABLED_DEFAULT false

static void on_accept(h2o_socket_t *listener, const char *err)
{
    h2o_socket_t *sock;

    if (err != NULL) {
        return;
    }

    if ((sock = h2o_evloop_socket_accept(listener)) == NULL)
        return;
    h2o_accept(&accept_ctx, sock);
}

static int create_listener(const char *ip, int port)
{
    struct sockaddr_in addr;
    int fd, reuseaddr_flag = 1;
    h2o_socket_t *sock;

    memset(&addr, 0, sizeof(addr));
    addr.sin_family = AF_INET;
    addr.sin_addr.s_addr = inet_addr(ip);
    addr.sin_port = htons(port);

    if ((fd = socket(AF_INET, SOCK_STREAM, 0)) == -1 ||
        setsockopt(fd, SOL_SOCKET, SO_REUSEADDR, &reuseaddr_flag, sizeof(reuseaddr_flag)) != 0 ||
        bind(fd, (struct sockaddr *)&addr, sizeof(addr)) != 0 || listen(fd, SOMAXCONN) != 0) {
        return -1;
    }

    sock = h2o_evloop_socket_create(ctx.loop, fd, H2O_SOCKET_FLAG_DONT_READ);
    h2o_socket_read_start(sock, on_accept);

    return 0;
}

static int ssl_init()
{
    if (!config_get_boolean(HTTPD_CONFIG_SECTION, "ssl", false))
        return 0;

    char default_fn[FILENAME_MAX + 1];

    snprintfz(default_fn,  FILENAME_MAX, "%s/ssl/key.pem",  netdata_configured_user_config_dir);
    const char *key_fn  = config_get(HTTPD_CONFIG_SECTION, "ssl key", default_fn);

    snprintfz(default_fn, FILENAME_MAX, "%s/ssl/cert.pem", netdata_configured_user_config_dir);
    const char *cert_fn = config_get(HTTPD_CONFIG_SECTION, "ssl certificate",  default_fn);

#if OPENSSL_VERSION_NUMBER < OPENSSL_VERSION_110
    accept_ctx.ssl_ctx = SSL_CTX_new(SSLv23_server_method());
#else
    accept_ctx.ssl_ctx = SSL_CTX_new(TLS_server_method());
#endif

    SSL_CTX_set_options(accept_ctx.ssl_ctx, SSL_OP_NO_SSLv2);

    /* load certificate and private key */
    if (SSL_CTX_use_PrivateKey_file(accept_ctx.ssl_ctx, key_fn, SSL_FILETYPE_PEM) != 1) {
        error("Could not load server key from \"%s\"", key_fn);
        return -1;
    }
    if (SSL_CTX_use_certificate_file(accept_ctx.ssl_ctx, cert_fn, SSL_FILETYPE_PEM) != 1) {
        error("Could not load certificate from \"%s\"", cert_fn);
        return -1;
    }

    h2o_ssl_register_alpn_protocols(accept_ctx.ssl_ctx, h2o_http2_alpn_protocols);

    info("SSL support enabled");

    return 0;
}

// I did not find a way to do wildcard paths to make common handler for urls like:
// /api/v1/info
// /host/child/api/v1/info
// /host/uuid/api/v1/info
// ideally we could do something like "/*/api/v1/info" subscription
// so we do it "manually" here with uberhandler
static inline int _netdata_uberhandler(h2o_req_t *req, RRDHOST **host)
{
    if (!h2o_memis(req->method.base, req->method.len, H2O_STRLIT("GET")))
        return -1;

    static h2o_generator_t generator = { NULL, NULL };

    h2o_iovec_t norm_path = req->path_normalized;

    if (norm_path.len > strlen(HOST_SELECT_PREFIX) && !memcmp(norm_path.base, HOST_SELECT_PREFIX, strlen(HOST_SELECT_PREFIX))) {
        h2o_iovec_t host_id; // host_id can be either and UUID or a hostname of the child

        norm_path.base += strlen(HOST_SELECT_PREFIX);
        norm_path.len -= strlen(HOST_SELECT_PREFIX);

        host_id = norm_path;

        size_t end_loc = h2o_strstr(host_id.base, host_id.len, "/", 1);
        if (end_loc != SIZE_MAX) {
            host_id.len = end_loc;
            norm_path.base += end_loc;
            norm_path.len -= end_loc;
        }

        char *c_host_id = iovec_to_cstr(&host_id);
        *host = rrdhost_find_by_hostname(c_host_id);
        if (!*host)
            *host = rrdhost_find_by_guid(c_host_id);
        if (!*host) {
            req->res.status = HTTP_RESP_BAD_REQUEST;
            req->res.reason = "Wrong host id";
            h2o_send_inline(req, H2O_STRLIT("Host id provided was not found!\n"));
            freez(c_host_id);
            return 0;
        }
        freez(c_host_id);

        // we have to rewrite URL here in case this is not an api call
        // so that the subsequent file upload handler can send the correct
        // files to the client
        // if this is not an API call we will abort this handler later
        // and let the internal serve file handler of h2o care for things

        if (end_loc == SIZE_MAX) {
            req->path.len = 1;
            req->path_normalized.len = 1;
        } else {
            size_t offset = norm_path.base - req->path_normalized.base;
            req->path.len -= offset;
            req->path.base += offset;
            req->query_at -= offset;
            req->path_normalized.len -= offset;
            req->path_normalized.base += offset;
        }
    }

    // workaround for a dashboard bug which causes sometimes urls like
    // "//api/v1/info" to be caled instead of "/api/v1/info"
    if (norm_path.len > 2 &&
        norm_path.base[0] == '/' &&
        norm_path.base[1] == '/' ) {
            norm_path.base++;
            norm_path.len--;
    }

    size_t api_loc = h2o_strstr(norm_path.base, norm_path.len, H2O_STRLIT(API_V1_PREFIX));
    if (api_loc == SIZE_MAX)
        return 1;

    h2o_iovec_t api_command = norm_path;
    api_command.base += api_loc + strlen(API_V1_PREFIX);
    api_command.len -= api_loc + strlen(API_V1_PREFIX);

    if (!api_command.len)
        return 1;

    // this (emulating struct web_client) is a hack and will be removed
    // in future PRs but needs bigger changes in old http_api_v1
    // we need to make the web_client_api_request_v1 to be web server
    // agnostic and remove the old webservers dependency creep into the
    // individual response generators and thus remove the need to "emulate"
    // the old webserver calling this function here and in ACLK
    struct web_client w;
    w.response.data = buffer_create(NBUF_INITIAL_SIZE_RESP, NULL);
    w.response.header = buffer_create(NBUF_INITIAL_SIZE_RESP, NULL);
    w.url_query_string_decoded = buffer_create(NBUF_INITIAL_SIZE_RESP, NULL);
    w.acl = WEB_CLIENT_ACL_DASHBOARD;

    char *path_c_str = iovec_to_cstr(&api_command);
    char *path_unescaped = url_unescape(path_c_str);
    freez(path_c_str);

    IF_HAS_URL_PARAMS(req) {
        h2o_iovec_t query_params = URL_PARAMS_IOVEC_INIT_WITH_QUESTIONMARK(req);
        char *query_c_str = iovec_to_cstr(&query_params);
        char *query_unescaped = url_unescape(query_c_str);
        freez(query_c_str);
        buffer_strcat(w.url_query_string_decoded, query_unescaped);
        freez(query_unescaped);
    }

    web_client_api_request_v1(*host, &w, path_unescaped);
    freez(path_unescaped);

    h2o_iovec_t body = buffer_to_h2o_iovec(w.response.data);

    // we move msg body to req->pool managed memory as it has to
    // live until whole response has been encrypted and sent
    // when req is finished memory will be freed with the pool
    void *managed = h2o_mem_alloc_shared(&req->pool, body.len, NULL);
    memcpy(managed, body.base, body.len);
    body.base = managed;

    req->res.status = HTTP_RESP_OK;
    req->res.reason = "OK";
    if (w.response.data->content_type == CT_APPLICATION_JSON)
        h2o_add_header(&req->pool, &req->res.headers, H2O_TOKEN_CONTENT_TYPE, NULL, CONTENT_JSON_UTF8);
    else
        h2o_add_header(&req->pool, &req->res.headers, H2O_TOKEN_CONTENT_TYPE, NULL, CONTENT_TEXT_UTF8);
    h2o_start_response(req, &generator);
    h2o_send(req, &body, 1, H2O_SEND_STATE_FINAL);

    buffer_free(w.response.data);
    buffer_free(w.response.header);
    buffer_free(w.url_query_string_decoded);

    return 0;
}

static int netdata_uberhandler(h2o_handler_t *self, h2o_req_t *req)
{
    UNUSED(self);
    RRDHOST *host = localhost;

    int ret = _netdata_uberhandler(req, &host);

    char host_uuid_str[UUID_STR_LEN];
    uuid_unparse_lower(host->host_uuid, host_uuid_str);

    if (!ret) {
        log_access("HTTPD OK method: " PRINTF_H2O_IOVEC_FMT
                   ", path: " PRINTF_H2O_IOVEC_FMT
                   ", as host: %s"
                   ", response: %d",
                   PRINTF_H2O_IOVEC(&req->method),
                   PRINTF_H2O_IOVEC(&req->input.path),
                   host == localhost ? "localhost" : host_uuid_str,
                   req->res.status);
    } else {
        log_access("HTTPD %d"
                   " method: " PRINTF_H2O_IOVEC_FMT
                   ", path: " PRINTF_H2O_IOVEC_FMT
                   ", forwarding to file handler as path: " PRINTF_H2O_IOVEC_FMT,
                   ret,
                   PRINTF_H2O_IOVEC(&req->method),
                   PRINTF_H2O_IOVEC(&req->input.path),
                   PRINTF_H2O_IOVEC(&req->path));
    }

    return ret;
}

static int hdl_netdata_conf(h2o_handler_t *self, h2o_req_t *req)
{
    UNUSED(self);
    if (!h2o_memis(req->method.base, req->method.len, H2O_STRLIT("GET")))
        return -1;

    BUFFER *buf = buffer_create(NBUF_INITIAL_SIZE_RESP, NULL);
    config_generate(buf, 0);

    void *managed = h2o_mem_alloc_shared(&req->pool, buf->len, NULL);
    memcpy(managed, buf->buffer, buf->len);

    req->res.status = HTTP_RESP_OK;
    req->res.reason = "OK";
    h2o_add_header(&req->pool, &req->res.headers, H2O_TOKEN_CONTENT_TYPE, NULL, CONTENT_TEXT_UTF8);
    h2o_send_inline(req, managed, buf->len);
    buffer_free(buf);

    return 0;
}

#define POLL_INTERVAL 100

void *httpd_main(void *ptr) {
    struct netdata_static_thread *static_thread = (struct netdata_static_thread *)ptr;

    h2o_pathconf_t *pathconf;
    h2o_hostconf_t *hostconf;

    netdata_thread_disable_cancelability();

    const char *bind_addr = config_get(HTTPD_CONFIG_SECTION, "bind to", "127.0.0.1");
    int bind_port = config_get_number(HTTPD_CONFIG_SECTION, "port", 19998);

    h2o_config_init(&config);
    hostconf = h2o_config_register_host(&config, h2o_iovec_init(H2O_STRLIT("default")), bind_port);

    pathconf = h2o_config_register_path(hostconf, "/netdata.conf", 0);
    h2o_handler_t *handler = h2o_create_handler(pathconf, sizeof(*handler));
    handler->on_req = hdl_netdata_conf;

    pathconf = h2o_config_register_path(hostconf, "/", 0);
    handler = h2o_create_handler(pathconf, sizeof(*handler));
    handler->on_req = netdata_uberhandler;
    h2o_file_register(pathconf, netdata_configured_web_dir, NULL, NULL, H2O_FILE_FLAG_SEND_COMPRESSED);

    h2o_context_init(&ctx, h2o_evloop_create(), &config);

    if(ssl_init()) {
        error_report("SSL was requested but could not be properly initialized. Aborting.");
        return NULL;
    }

    accept_ctx.ctx = &ctx;
    accept_ctx.hosts = config.hosts;

    if (create_listener(bind_addr, bind_port) != 0) {
        error("failed to create listener %s:%d", bind_addr, bind_port);
        return NULL;
    }

    while (service_running(SERVICE_HTTPD)) {
        int rc = h2o_evloop_run(ctx.loop, POLL_INTERVAL);
        if (rc < 0 && errno != EINTR) {
            error("h2o_evloop_run returned (%d) with errno other than EINTR. Aborting", rc);
            break;
        }
    } 

    static_thread->enabled = NETDATA_MAIN_THREAD_EXITED;
    return NULL;
}

int httpd_is_enabled() {
    return config_get_boolean(HTTPD_CONFIG_SECTION, "enabled", HTTPD_ENABLED_DEFAULT);
}
