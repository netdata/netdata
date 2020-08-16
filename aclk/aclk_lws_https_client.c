// SPDX-License-Identifier: GPL-3.0-or-later

#define ACLK_LWS_HTTPS_CLIENT_INTERNAL
#include "aclk_lws_https_client.h"

#include "aclk_common.h"

#include "aclk_lws_wss_client.h"

#define SMALL_BUFFER 16

struct simple_hcc_data {
    char *data;
    size_t data_size;
    char *payload;
    int response_code;
    int done;
};

static int simple_https_client_callback(struct lws *wsi, enum lws_callback_reasons reason, void *user, void *in, size_t len)
{
    UNUSED(user);
    int n;
    char *ptr;
    char buffer[SMALL_BUFFER];
    struct simple_hcc_data *perconn_data = lws_get_opaque_user_data(wsi);

    switch (reason) {
    case LWS_CALLBACK_RECEIVE_CLIENT_HTTP_READ:
        debug(D_ACLK, "LWS_CALLBACK_RECEIVE_CLIENT_HTTP_READ");
        return 0;
    case LWS_CALLBACK_RECEIVE_CLIENT_HTTP:
        debug(D_ACLK, "LWS_CALLBACK_RECEIVE_CLIENT_HTTP");
        if(!perconn_data) {
            error("Missing Per Connect Data");
            return -1;
        }
        ptr = perconn_data->data;
        n = perconn_data->data_size - 1;
        if (lws_http_client_read(wsi, &ptr, &n) < 0)
            return -1;
        ptr[n] = '\0';
        return 0;
    case LWS_CALLBACK_WSI_DESTROY:
        debug(D_ACLK, "LWS_CALLBACK_WSI_DESTROY");
        if(perconn_data)
            perconn_data->done = 1;
        return 0;
    case LWS_CALLBACK_ESTABLISHED_CLIENT_HTTP:
        debug(D_ACLK, "LWS_CALLBACK_ESTABLISHED_CLIENT_HTTP");
        if(perconn_data)
            perconn_data->response_code = lws_http_client_http_response(wsi);
        return 0;
    case LWS_CALLBACK_CLOSED_CLIENT_HTTP:
        debug(D_ACLK, "LWS_CALLBACK_CLOSED_CLIENT_HTTP");
        return 0;
    case LWS_CALLBACK_OPENSSL_LOAD_EXTRA_CLIENT_VERIFY_CERTS:
        debug(D_ACLK, "LWS_CALLBACK_OPENSSL_LOAD_EXTRA_CLIENT_VERIFY_CERTS");
        return 0;
    case LWS_CALLBACK_CLIENT_APPEND_HANDSHAKE_HEADER:
        debug(D_ACLK, "LWS_CALLBACK_CLIENT_APPEND_HANDSHAKE_HEADER");
        if(perconn_data && perconn_data->payload) {
            unsigned char **p = (unsigned char **)in, *end = (*p) + len;
            snprintfz(buffer, SMALL_BUFFER, "%zu", strlen(perconn_data->payload));
            if (lws_add_http_header_by_token(wsi,
                    WSI_TOKEN_HTTP_CONTENT_LENGTH,
                    (unsigned char *)buffer, strlen(buffer), p, end))
                return -1;
            if (lws_add_http_header_by_token(wsi,
                    WSI_TOKEN_HTTP_CONTENT_TYPE,
                    (unsigned char *)ACLK_CONTENT_TYPE_JSON,
                    strlen(ACLK_CONTENT_TYPE_JSON), p, end))
                return -1;
            lws_client_http_body_pending(wsi, 1);
            lws_callback_on_writable(wsi);
        }
        return 0;
    case LWS_CALLBACK_CLIENT_HTTP_WRITEABLE:
        debug(D_ACLK, "LWS_CALLBACK_CLIENT_HTTP_WRITEABLE");
        if(perconn_data && perconn_data->payload) {
            n = strlen(perconn_data->payload);
            if(perconn_data->data_size < (size_t)LWS_PRE + n + 1) {
                error("Buffer given is not big enough");
                return 1;
            }

            memcpy(&perconn_data->data[LWS_PRE], perconn_data->payload, n);
            if(n != lws_write(wsi, (unsigned char*)&perconn_data->data[LWS_PRE], n, LWS_WRITE_HTTP)) {
                error("lws_write error");
                perconn_data->data[0] = 0;
                return 1;
            }
            lws_client_http_body_pending(wsi, 0);
            // clean for subsequent reply read
            perconn_data->data[0] = 0;
        }
        return 0;
    case LWS_CALLBACK_CLIENT_HTTP_BIND_PROTOCOL:
        debug(D_ACLK, "LWS_CALLBACK_CLIENT_HTTP_BIND_PROTOCOL");
        return 0;
    case LWS_CALLBACK_WSI_CREATE:
        debug(D_ACLK, "LWS_CALLBACK_WSI_CREATE");
        return 0;
    case LWS_CALLBACK_PROTOCOL_INIT:
        debug(D_ACLK, "LWS_CALLBACK_PROTOCOL_INIT");
        return 0;
    case LWS_CALLBACK_CLIENT_HTTP_DROP_PROTOCOL:
        debug(D_ACLK, "LWS_CALLBACK_CLIENT_HTTP_DROP_PROTOCOL");
        return 0;
    case LWS_CALLBACK_SERVER_NEW_CLIENT_INSTANTIATED:
        debug(D_ACLK, "LWS_CALLBACK_SERVER_NEW_CLIENT_INSTANTIATED");
        return 0;
    case LWS_CALLBACK_GET_THREAD_ID:
        debug(D_ACLK, "LWS_CALLBACK_GET_THREAD_ID");
        return 0;
    case LWS_CALLBACK_EVENT_WAIT_CANCELLED:
        debug(D_ACLK, "LWS_CALLBACK_EVENT_WAIT_CANCELLED");
        return 0;
    case LWS_CALLBACK_OPENSSL_PERFORM_SERVER_CERT_VERIFICATION:
        debug(D_ACLK, "LWS_CALLBACK_OPENSSL_PERFORM_SERVER_CERT_VERIFICATION");
        return 0;
    case LWS_CALLBACK_CLIENT_FILTER_PRE_ESTABLISH:
        debug(D_ACLK, "LWS_CALLBACK_CLIENT_FILTER_PRE_ESTABLISH");
        return 0;
    default:
        debug(D_ACLK, "Unknown callback %d", (int)reason);
        return 0;
    }
}

static const struct lws_protocols protocols[] = {
    {
        "http",
        simple_https_client_callback,
        0,
        0,
        0,
        0,
        0
    },
    { NULL, NULL, 0, 0, 0, 0, 0 }
};

static void simple_hcc_log_divert(int level, const char *line)
{
    UNUSED(level);
    error("Libwebsockets: %s", line);
}

int aclk_send_https_request(char *method, char *host, char *port, char *url, char *b, size_t b_size, char *payload)
{
    info("%s %s", __func__, method);

    struct lws_context_creation_info info;
    struct lws_client_connect_info i;
    struct lws_context *context;

    struct simple_hcc_data *data = callocz(1, sizeof(struct simple_hcc_data));
    data->data = b;
    data->data[0] = 0;
    data->data_size = b_size;
    data->payload = payload;

    int n = 0;
    time_t timestamp;

    struct lws_vhost *vhost;

    memset(&info, 0, sizeof info);

    info.options = LWS_SERVER_OPTION_DO_SSL_GLOBAL_INIT;
    info.port = CONTEXT_PORT_NO_LISTEN;
    info.protocols = protocols;


    context = lws_create_context(&info);
    if (!context) {
        error("Error creating LWS context");
        freez(data);
        return 1;
    }

    lws_set_log_level(LLL_ERR | LLL_WARN, simple_hcc_log_divert);

    lws_service(context, 0);

    memset(&i, 0, sizeof i); /* otherwise uninitialized garbage */
    i.context = context;

#ifdef ACLK_SSL_ALLOW_SELF_SIGNED
    i.ssl_connection = LCCSCF_USE_SSL | LCCSCF_ALLOW_SELFSIGNED | LCCSCF_SKIP_SERVER_CERT_HOSTNAME_CHECK | LCCSCF_ALLOW_INSECURE;
    info("Disabling SSL certificate checks");
#else
    i.ssl_connection = LCCSCF_USE_SSL;
#endif
#if defined(HAVE_X509_VERIFY_PARAM_set1_host) && HAVE_X509_VERIFY_PARAM_set1_host == 0
#warning DISABLING SSL HOSTNAME VALIDATION BECAUSE IT IS NOT AVAILABLE ON THIS SYSTEM.
    i.ssl_connection |= LCCSCF_SKIP_SERVER_CERT_HOSTNAME_CHECK;
#endif

    i.port = atoi(port);
    i.address = host;
    i.path = url;

    i.host = i.address;
    i.origin = i.address;
    i.method = method;
    i.opaque_user_data = data;
    i.alpn = "http/1.1";

    i.protocol = protocols[0].name;

    vhost = lws_get_vhost_by_name(context, "default");
    if(!vhost)
        fatal("Could not find the default LWS vhost.");

    //set up proxy
    aclk_wss_set_proxy(vhost);

    lws_client_connect_via_info(&i);

    // libwebsockets handle connection timeouts already
    // this adds additional safety in case of bug in LWS
    timestamp = now_monotonic_sec();
    while( n >= 0 && !data->done && !netdata_exit) {
        n = lws_service(context, 0);
        if( now_monotonic_sec() - timestamp > SEND_HTTPS_REQUEST_TIMEOUT ) {
            data->data[0] = 0;
            data->done = 1;
            error("Servicing LWS took too long.");
        }
    }

    lws_context_destroy(context);

    n = data->response_code;

    freez(data);
    return (n < 200 || n >= 300);
}
