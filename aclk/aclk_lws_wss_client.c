// SPDX-License-Identifier: GPL-3.0-or-later

#include "aclk_lws_wss_client.h"

#include "libnetdata/libnetdata.h"
#include "../daemon/common.h"
#include "aclk_common.h"
#include "aclk_stats.h"

extern int aclk_shutting_down;

static int aclk_lws_wss_callback(struct lws *wsi, enum lws_callback_reasons reason, void *user, void *in, size_t len);

struct aclk_lws_wss_perconnect_data {
    int todo;
};

static struct aclk_lws_wss_engine_instance *engine_instance = NULL;

void lws_wss_check_queues(size_t *write_len, size_t *write_len_bytes, size_t *read_len)
{
    if (write_len != NULL && write_len_bytes != NULL)
    {
        *write_len = 0;
        *write_len_bytes = 0;
        if (engine_instance != NULL)
        {
            aclk_lws_mutex_lock(&engine_instance->write_buf_mutex);

            struct lws_wss_packet_buffer *write_b;
            size_t w,wb;
            for(w=0, wb=0, write_b = engine_instance->write_buffer_head; write_b != NULL; write_b = write_b->next)
            {
                w++;
                wb += write_b->data_size - write_b->written;
            }
            *write_len = w;
            *write_len_bytes = wb;
            aclk_lws_mutex_unlock(&engine_instance->write_buf_mutex);
        }
    }
    else if (write_len != NULL)
    {
        *write_len = 0;
        if (engine_instance != NULL)
        {
            aclk_lws_mutex_lock(&engine_instance->write_buf_mutex);

            struct lws_wss_packet_buffer *write_b;
            size_t w;
            for(w=0, write_b = engine_instance->write_buffer_head; write_b != NULL; write_b = write_b->next)
                w++;
            *write_len = w;
            aclk_lws_mutex_unlock(&engine_instance->write_buf_mutex);
        }
    }
    if (read_len != NULL)
    {
        *read_len = 0;
        if (engine_instance != NULL)
        {
            aclk_lws_mutex_lock(&engine_instance->read_buf_mutex);
            *read_len = lws_ring_get_count_waiting_elements(engine_instance->read_ringbuffer, NULL);
            aclk_lws_mutex_unlock(&engine_instance->read_buf_mutex);
        }
    }
}

static inline struct lws_wss_packet_buffer *lws_wss_packet_buffer_new(void *data, size_t size)
{
    struct lws_wss_packet_buffer *new = callocz(1, sizeof(struct lws_wss_packet_buffer));
    if (data) {
        new->data = mallocz(LWS_PRE + size);
        memcpy(new->data + LWS_PRE, data, size);
        new->data_size = size;
        new->written = 0;
    }
    return new;
}

static inline void lws_wss_packet_buffer_append(struct lws_wss_packet_buffer **list, struct lws_wss_packet_buffer *item)
{
    struct lws_wss_packet_buffer *tail = *list;
    if (!*list) {
        *list = item;
        return;
    }
    while (tail->next) {
        tail = tail->next;
    }
    tail->next = item;
}

static inline struct lws_wss_packet_buffer *lws_wss_packet_buffer_pop(struct lws_wss_packet_buffer **list)
{
    struct lws_wss_packet_buffer *ret = *list;
    if (ret != NULL)
        *list = ret->next;

    return ret;
}

static inline void lws_wss_packet_buffer_free(struct lws_wss_packet_buffer *item)
{
    freez(item->data);
    freez(item);
}

static inline void _aclk_lws_wss_read_buffer_clear(struct lws_ring *ringbuffer)
{
    size_t elems = lws_ring_get_count_waiting_elements(ringbuffer, NULL);
    if (elems > 0)
        lws_ring_consume(ringbuffer, NULL, NULL, elems);
}

static inline void _aclk_lws_wss_write_buffer_clear(struct lws_wss_packet_buffer **list)
{
    struct lws_wss_packet_buffer *i;
    while ((i = lws_wss_packet_buffer_pop(list)) != NULL) {
        lws_wss_packet_buffer_free(i);
    }
    *list = NULL;
}

static inline void aclk_lws_wss_clear_io_buffers()
{
    aclk_lws_mutex_lock(&engine_instance->read_buf_mutex);
    _aclk_lws_wss_read_buffer_clear(engine_instance->read_ringbuffer);
    aclk_lws_mutex_unlock(&engine_instance->read_buf_mutex);
    aclk_lws_mutex_lock(&engine_instance->write_buf_mutex);
    _aclk_lws_wss_write_buffer_clear(&engine_instance->write_buffer_head);
    aclk_lws_mutex_unlock(&engine_instance->write_buf_mutex);
}

static const struct lws_protocols protocols[] = { { "aclk-wss", aclk_lws_wss_callback,
                                                    sizeof(struct aclk_lws_wss_perconnect_data), 32768*4, 0, 0, 32768*4 },
                                                  { NULL, NULL, 0, 0, 0, 0, 0 } };

static void aclk_lws_wss_log_divert(int level, const char *line)
{
    switch (level) {
        case LLL_ERR:
            error("Libwebsockets Error: %s", line);
            break;
        case LLL_WARN:
            debug(D_ACLK, "Libwebsockets Warn: %s", line);
            break;
        default:
            error("Libwebsockets try to log with unknown log level (%d), msg: %s", level, line);
    }
}

static int aclk_lws_wss_client_init( char *target_hostname, int target_port)
{
    static int lws_logging_initialized = 0;

    if (unlikely(!lws_logging_initialized)) {
        lws_set_log_level(LLL_ERR | LLL_WARN, aclk_lws_wss_log_divert);
        lws_logging_initialized = 1;
    }

    if (!target_hostname)
        return 1;

    engine_instance = callocz(1, sizeof(struct aclk_lws_wss_engine_instance));

    engine_instance->host = target_hostname;
    engine_instance->port = target_port;


    aclk_lws_mutex_init(&engine_instance->write_buf_mutex);
    aclk_lws_mutex_init(&engine_instance->read_buf_mutex);

    engine_instance->read_ringbuffer = lws_ring_create(1, ACLK_LWS_WSS_RECV_BUFF_SIZE_BYTES, NULL);
    if (!engine_instance->read_ringbuffer)
        goto failure_cleanup;

    return 0;

failure_cleanup:
    freez(engine_instance);
    return 1;
}

void aclk_lws_wss_destroy_context()
{
    if (!engine_instance)
        return;
    if (!engine_instance->lws_context)
        return;
    lws_context_destroy(engine_instance->lws_context);
    engine_instance->lws_context = NULL;
}


void aclk_lws_wss_client_destroy()
{
    if (engine_instance == NULL)
        return;

    aclk_lws_wss_destroy_context();
    engine_instance->lws_wsi = NULL;

    aclk_lws_wss_clear_io_buffers();

#ifdef ACLK_LWS_MOSQUITTO_IO_CALLS_MULTITHREADED
    pthread_mutex_destroy(&engine_instance->write_buf_mutex);
    pthread_mutex_destroy(&engine_instance->read_buf_mutex);
#endif
}

#ifdef LWS_WITH_SOCKS5
static int aclk_wss_set_socks(struct lws_vhost *vhost, const char *socks)
{
    char *proxy = strstr(socks, ACLK_PROXY_PROTO_ADDR_SEPARATOR);

    if (!proxy)
        return -1;

    proxy += strlen(ACLK_PROXY_PROTO_ADDR_SEPARATOR);

    if (!*proxy)
        return -1;

    return lws_set_socks(vhost, proxy);
}
#endif

void aclk_wss_set_proxy(struct lws_vhost *vhost)
{
    const char *proxy;
    ACLK_PROXY_TYPE proxy_type;
    char *log;

    proxy = aclk_get_proxy(&proxy_type);

#ifdef LWS_WITH_SOCKS5
    lws_set_socks(vhost, ":");
#endif
    lws_set_proxy(vhost, ":");

    if (proxy_type == PROXY_TYPE_UNKNOWN) {
        error("Unknown proxy type");
        return;
    }

    if (proxy_type == PROXY_TYPE_SOCKS5 || proxy_type == PROXY_TYPE_HTTP) {
        log = strdupz(proxy);
        safe_log_proxy_censor(log);
        info("Connecting using %s proxy:\"%s\"", aclk_proxy_type_to_s(&proxy_type), log);
        freez(log);
    }
    if (proxy_type == PROXY_TYPE_SOCKS5) {
#ifdef LWS_WITH_SOCKS5
        if (aclk_wss_set_socks(vhost, proxy))
            error("LWS failed to accept socks proxy.");
        return;
#else
        fatal("We have no SOCKS5 support but we made it here. Programming error!");
#endif
    }
    if (proxy_type == PROXY_TYPE_HTTP) {
        if (lws_set_proxy(vhost, proxy))
            error("LWS failed to accept http proxy.");
        return;
    }
    if (proxy_type != PROXY_DISABLED)
        error("Unknown proxy type");
}

// Return code indicates if connection attempt has started async.
int aclk_lws_wss_connect(char *host, int port)
{
    struct lws_client_connect_info i;
    struct lws_vhost *vhost;
    int n;

    if (!engine_instance) {
        if (aclk_lws_wss_client_init(host, port))
            return 1;       // Propagate failure
    }

    if (!engine_instance->lws_context)
    {
        // First time through (on this connection), create the context
        struct lws_context_creation_info info;
        memset(&info, 0, sizeof(struct lws_context_creation_info));
        info.options = LWS_SERVER_OPTION_DO_SSL_GLOBAL_INIT;
        info.port = CONTEXT_PORT_NO_LISTEN;
        info.protocols = protocols;
        engine_instance->lws_context = lws_create_context(&info);
        if (!engine_instance->lws_context)
        {
            error("Failed to create lws_context, ACLK will not function");
            return 1;
        }
        return 0;
        // PROTOCOL_INIT callback will call again.
    }

    for (n = 0; n < ACLK_LWS_CALLBACK_HISTORY; n++)
        engine_instance->lws_callback_history[n] = 0;

    if (engine_instance->lws_wsi) {
        error("Already Connected. Only one connection supported at a time.");
        return 0;
    }

    memset(&i, 0, sizeof(i));
    i.context = engine_instance->lws_context;
    i.port = engine_instance->port;
    i.address = engine_instance->host;
    i.path = "/mqtt";
    i.host = engine_instance->host;
    i.protocol = "mqtt";

    // from LWS docu:
    // If option LWS_SERVER_OPTION_EXPLICIT_VHOSTS is given, no vhost is
    // created; you're expected to create your own vhosts afterwards using
    // lws_create_vhost().  Otherwise a vhost named "default" is also created
    // using the information in the vhost-related members, for compatibility.
    vhost = lws_get_vhost_by_name(engine_instance->lws_context, "default");
    if(!vhost)
        fatal("Could not find the default LWS vhost.");

    aclk_wss_set_proxy(vhost);

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
    lws_client_connect_via_info(&i);
    return 0;
}

static inline int received_data_to_ringbuff(struct lws_ring *buffer, void *data, size_t len)
{
    if (lws_ring_insert(buffer, data, len) != len) {
        error("ACLK_LWS_WSS_CLIENT: receive buffer full. Closing connection to prevent flooding.");
        return 0;
    }
    return 1;
}

static const char *aclk_lws_callback_name(enum lws_callback_reasons reason)
{
    switch (reason) {
        case LWS_CALLBACK_CLIENT_WRITEABLE:
            return "LWS_CALLBACK_CLIENT_WRITEABLE";
        case LWS_CALLBACK_CLIENT_RECEIVE:
            return "LWS_CALLBACK_CLIENT_RECEIVE";
        case LWS_CALLBACK_PROTOCOL_INIT:
            return "LWS_CALLBACK_PROTOCOL_INIT";
        case LWS_CALLBACK_SERVER_NEW_CLIENT_INSTANTIATED:
            return "LWS_CALLBACK_SERVER_NEW_CLIENT_INSTANTIATED";
        case LWS_CALLBACK_USER:
            return "LWS_CALLBACK_USER";
        case LWS_CALLBACK_CLIENT_CONNECTION_ERROR:
            return "LWS_CALLBACK_CLIENT_CONNECTION_ERROR";
        case LWS_CALLBACK_CLIENT_CLOSED:
            return "LWS_CALLBACK_CLIENT_CLOSED";
        case LWS_CALLBACK_WS_PEER_INITIATED_CLOSE:
            return "LWS_CALLBACK_WS_PEER_INITIATED_CLOSE";
        case LWS_CALLBACK_WSI_DESTROY:
            return "LWS_CALLBACK_WSI_DESTROY";
        case LWS_CALLBACK_CLIENT_ESTABLISHED:
            return "LWS_CALLBACK_CLIENT_ESTABLISHED";
        case LWS_CALLBACK_OPENSSL_PERFORM_SERVER_CERT_VERIFICATION:
            return "LWS_CALLBACK_OPENSSL_PERFORM_SERVER_CERT_VERIFICATION";
        case LWS_CALLBACK_EVENT_WAIT_CANCELLED:
            return "LWS_CALLBACK_EVENT_WAIT_CANCELLED";
        default:
            // Not using an internal buffer here for thread-safety with unknown calling context.
            error("Unknown LWS callback %u", reason);
            return "unknown";
    }
}

void aclk_lws_wss_fail_report()
{
    int i;
    int anything_to_send = 0;
    BUFFER *buf;

    if (netdata_anonymous_statistics_enabled <= 0)
        return;

    // guess - most of the callback will be 1-99 + ',' + \0
    buf = buffer_create((ACLK_LWS_CALLBACK_HISTORY * 2) + 10);

    for (i = 0; i < ACLK_LWS_CALLBACK_HISTORY; i++)
        if (engine_instance->lws_callback_history[i]) {
            buffer_sprintf(buf, "%s%d", (i ? "," : ""), engine_instance->lws_callback_history[i]);
            anything_to_send = 1;
        }

    if (anything_to_send)
        send_statistics("ACLK_CONN_FAIL", "FAIL", buffer_tostring(buf));

    buffer_free(buf);
}

static int aclk_lws_wss_callback(struct lws *wsi, enum lws_callback_reasons reason, void *user, void *in, size_t len)
{
    UNUSED(user);
    struct lws_wss_packet_buffer *data;
    int retval = 0;
    static int lws_shutting_down = 0;
    int i;

    for (i = ACLK_LWS_CALLBACK_HISTORY - 1; i > 0; i--)
        engine_instance->lws_callback_history[i] = engine_instance->lws_callback_history[i - 1];
    engine_instance->lws_callback_history[0] = (int)reason;

    if (unlikely(aclk_shutting_down && !lws_shutting_down)) {
            lws_shutting_down = 1;
            retval = -1;
            engine_instance->upstream_reconnect_request = 0;
    }

    // Callback servicing is forced when we are closed from above.
    if (engine_instance->upstream_reconnect_request) {
        error("Closing lws connectino due to libmosquitto error.");
        char *upstream_connection_error = "MQTT protocol error. Closing underlying wss connection.";
        lws_close_reason(
            wsi, LWS_CLOSE_STATUS_PROTOCOL_ERR, (unsigned char *)upstream_connection_error,
            strlen(upstream_connection_error));
        retval = -1;
        engine_instance->upstream_reconnect_request = 0;
    }

    // Don't log to info - volume is proportional to message flow on ACLK.
    switch (reason) {
        case LWS_CALLBACK_CLIENT_WRITEABLE:
            aclk_lws_mutex_lock(&engine_instance->write_buf_mutex);
            data = engine_instance->write_buffer_head;
            if (likely(data)) {
                size_t bytes_left = data->data_size - data->written;
                if ( bytes_left > FRAGMENT_SIZE)
                    bytes_left = FRAGMENT_SIZE;
                int n = lws_write(wsi, data->data + LWS_PRE + data->written, bytes_left, LWS_WRITE_BINARY);
                if (n>=0) {
                    data->written += n;
                    if (aclk_stats_enabled) {
                        ACLK_STATS_LOCK;
                        aclk_metrics_per_sample.write_q_consumed += n;
                        ACLK_STATS_UNLOCK;
                    }
                }
                //error("lws_write(req=%u,written=%u) %zu of %zu",bytes_left, rc, data->written,data->data_size,rc);
                if (data->written == data->data_size)
                {
                    lws_wss_packet_buffer_pop(&engine_instance->write_buffer_head);
                    lws_wss_packet_buffer_free(data);
                }
                if (engine_instance->write_buffer_head)
                    lws_callback_on_writable(engine_instance->lws_wsi);
            }
            aclk_lws_mutex_unlock(&engine_instance->write_buf_mutex);
            return retval;

        case LWS_CALLBACK_CLIENT_RECEIVE:
            aclk_lws_mutex_lock(&engine_instance->read_buf_mutex);
            if (!received_data_to_ringbuff(engine_instance->read_ringbuffer, in, len))
                retval = 1;
            aclk_lws_mutex_unlock(&engine_instance->read_buf_mutex);
            if (aclk_stats_enabled) {
                ACLK_STATS_LOCK;
                aclk_metrics_per_sample.read_q_added += len;
                ACLK_STATS_UNLOCK;
            }

            // to future myself -> do not call this while read lock is active as it will eventually
            // want to acquire same lock later in aclk_lws_wss_client_read() function
            aclk_lws_connection_data_received();
            return retval;

        case LWS_CALLBACK_WSI_CREATE:
        case LWS_CALLBACK_CLIENT_FILTER_PRE_ESTABLISH:
        case LWS_CALLBACK_CLIENT_APPEND_HANDSHAKE_HEADER:
        case LWS_CALLBACK_OPENSSL_LOAD_EXTRA_CLIENT_VERIFY_CERTS:
        case LWS_CALLBACK_GET_THREAD_ID: // ?
        case LWS_CALLBACK_EVENT_WAIT_CANCELLED:
        case LWS_CALLBACK_OPENSSL_PERFORM_SERVER_CERT_VERIFICATION:
            // Expected and safe to ignore.
            debug(D_ACLK, "Ignoring expected callback from LWS: %s", aclk_lws_callback_name(reason));
            return retval;

        default:
            // Pass to next switch, this case removes compiler warnings.
            break;
    }
    // Log to info - volume is proportional to connection attempts.
    info("Processing callback %s", aclk_lws_callback_name(reason));
    switch (reason) {
        case LWS_CALLBACK_PROTOCOL_INIT:
            aclk_lws_wss_connect(engine_instance->host, engine_instance->port); // Makes the outgoing connection
            break;
        case LWS_CALLBACK_SERVER_NEW_CLIENT_INSTANTIATED:
            if (engine_instance->lws_wsi != NULL && engine_instance->lws_wsi != wsi)
                error("Multiple connections on same WSI? %p vs %p", engine_instance->lws_wsi, wsi);
            engine_instance->lws_wsi = wsi;
            break;
        case LWS_CALLBACK_CLIENT_CONNECTION_ERROR:
            error(
                "Could not connect MQTT over WSS server \"%s:%d\". LwsReason:\"%s\"", engine_instance->host,
                engine_instance->port, (in ? (char *)in : "not given"));
            // Fall-through
        case LWS_CALLBACK_CLIENT_CLOSED:
        case LWS_CALLBACK_WS_PEER_INITIATED_CLOSE:
            engine_instance->lws_wsi = NULL; // inside libwebsockets lws_close_free_wsi is called after callback
            aclk_lws_connection_closed();
            return -1;                       // the callback response is ignored, hope the above remains true
        case LWS_CALLBACK_WSI_DESTROY:
            aclk_lws_wss_clear_io_buffers();
            if (!engine_instance->websocket_connection_up)
                aclk_lws_wss_fail_report();
            engine_instance->lws_wsi = NULL;
            engine_instance->websocket_connection_up = 0;
            aclk_lws_connection_closed();
            break;
        case LWS_CALLBACK_CLIENT_ESTABLISHED:
            engine_instance->websocket_connection_up = 1;
            aclk_lws_connection_established(engine_instance->host, engine_instance->port);
            break;

        default:
            error("Unexpected callback from libwebsockets %s", aclk_lws_callback_name(reason));
            break;
    }
    return retval; //0-OK, other connection should be closed!
}

int aclk_lws_wss_client_write(void *buf, size_t count)
{
    if (engine_instance && engine_instance->lws_wsi && engine_instance->websocket_connection_up) {
        aclk_lws_mutex_lock(&engine_instance->write_buf_mutex);
        lws_wss_packet_buffer_append(&engine_instance->write_buffer_head, lws_wss_packet_buffer_new(buf, count));
        aclk_lws_mutex_unlock(&engine_instance->write_buf_mutex);

        if (aclk_stats_enabled) {
            ACLK_STATS_LOCK;
            aclk_metrics_per_sample.write_q_added += count;
            ACLK_STATS_UNLOCK;
        }

        lws_callback_on_writable(engine_instance->lws_wsi);
        return count;
    }
    return 0;
}

int aclk_lws_wss_client_read(void *buf, size_t count)
{
    size_t data_to_be_read = count;

    aclk_lws_mutex_lock(&engine_instance->read_buf_mutex);
    size_t readable_byte_count = lws_ring_get_count_waiting_elements(engine_instance->read_ringbuffer, NULL);
    if (unlikely(readable_byte_count == 0)) {
        errno = EAGAIN;
        data_to_be_read = -1;
        goto abort;
    }

    if (readable_byte_count < data_to_be_read)
        data_to_be_read = readable_byte_count;

    data_to_be_read = lws_ring_consume(engine_instance->read_ringbuffer, NULL, buf, data_to_be_read);
    if (data_to_be_read == readable_byte_count)
        engine_instance->data_to_read = 0;

    if (aclk_stats_enabled) {
        ACLK_STATS_LOCK;
        aclk_metrics_per_sample.read_q_consumed += data_to_be_read;
        ACLK_STATS_UNLOCK;
    }

abort:
    aclk_lws_mutex_unlock(&engine_instance->read_buf_mutex);
    return data_to_be_read;
}

void aclk_lws_wss_service_loop()
{
    if (engine_instance)
    {
        /*if (engine_instance->lws_wsi) {
            lws_cancel_service(engine_instance->lws_context);
            lws_callback_on_writable(engine_instance->lws_wsi);
        }*/
        lws_service(engine_instance->lws_context, 0);
    }
}

// in case the MQTT connection disconnect while lws transport is still operational
// we should drop connection and reconnect
// this function should be called when that happens to notify lws of that situation
void aclk_lws_wss_mqtt_layer_disconect_notif()
{
    if (!engine_instance)
        return;
    if (engine_instance->lws_wsi && engine_instance->websocket_connection_up) {
        engine_instance->upstream_reconnect_request = 1;
        lws_callback_on_writable(
            engine_instance->lws_wsi); //here we just do it to ensure we get callback called from lws, we don't need any actual data to be written.
    }
}
