#include "aclk_lws_wss_client.h"

#include "libnetdata/libnetdata.h"
#include "../daemon/common.h"
#include "aclk_common.h"

static int aclk_lws_wss_callback(struct lws *wsi, enum lws_callback_reasons reason, void *user, void *in, size_t len);

struct aclk_lws_wss_perconnect_data {
    int todo;
};

struct lws_wss_packet_buffer {
    unsigned char *data;
    size_t data_size;
    struct lws_wss_packet_buffer *next;
};

static struct aclk_lws_wss_engine_instance *engine_instance = NULL;

static inline struct lws_wss_packet_buffer *lws_wss_packet_buffer_new(void *data, size_t size)
{
    struct lws_wss_packet_buffer *new = callocz(1, sizeof(struct lws_wss_packet_buffer));
    if (data) {
        new->data = mallocz(LWS_PRE + size);
        memcpy(new->data + LWS_PRE, data, size);
        new->data_size = size;
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
                                                    sizeof(struct aclk_lws_wss_perconnect_data), 0, 0, 0, 0 },
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
    struct lws_context_creation_info info;

    if (unlikely(!lws_logging_initialized)) {
        lws_set_log_level(LLL_ERR | LLL_WARN, aclk_lws_wss_log_divert);
        lws_logging_initialized = 1;
    }

    if (!target_hostname)
        return 1;

    engine_instance = callocz(1, sizeof(struct aclk_lws_wss_engine_instance));

    engine_instance->host = target_hostname;
    engine_instance->port = target_port;

    memset(&info, 0, sizeof(struct lws_context_creation_info));
    info.options = LWS_SERVER_OPTION_DO_SSL_GLOBAL_INIT;
    info.port = CONTEXT_PORT_NO_LISTEN;
    info.protocols = protocols;

    engine_instance->lws_context = lws_create_context(&info);
    if (!engine_instance->lws_context)
        goto failure_cleanup_2;

    aclk_lws_mutex_init(&engine_instance->write_buf_mutex);
    aclk_lws_mutex_init(&engine_instance->read_buf_mutex);

    engine_instance->read_ringbuffer = lws_ring_create(1, ACLK_LWS_WSS_RECV_BUFF_SIZE_BYTES, NULL);
    if (!engine_instance->read_ringbuffer)
        goto failure_cleanup;

    return 0;

failure_cleanup:
    lws_context_destroy(engine_instance->lws_context);
failure_cleanup_2:
    freez(engine_instance);
    return 1;
}

void aclk_lws_wss_client_destroy(struct aclk_lws_wss_engine_instance *engine_instance)
{
    if (engine_instance == NULL)
        return;
    lws_context_destroy(engine_instance->lws_context);
    engine_instance->lws_context = NULL;
    engine_instance->lws_wsi = NULL;

    aclk_lws_wss_clear_io_buffers(engine_instance);

#ifdef ACLK_LWS_MOSQUITTO_IO_CALLS_MULTITHREADED
    pthread_mutex_destroy(&engine_instance->write_buf_mutex);
    pthread_mutex_destroy(&engine_instance->read_buf_mutex);
#endif
}

static int _aclk_wss_set_socks(struct lws_vhost *vhost, const char *socks)
{
	char *proxy = strstr(socks, ACLK_PROXY_PROTO_ADDR_SEPARATOR);

	if(!proxy)
		return -1;

	proxy += strlen(ACLK_PROXY_PROTO_ADDR_SEPARATOR);

	if(!*proxy)
		return -1;

	return lws_set_socks(vhost, proxy);
}

// helper function to censor user&password
// for logging purposes
static void safe_log_proxy_censor(char *proxy) {
    size_t length = strlen(proxy);
    char *auth = proxy+length-1;
    char *cur;

    while( (auth >= proxy) && (*auth != '@') )
        auth--;

    //if not found or @ is first char do nothing
    if(auth<=proxy)
        return;

    cur = strstr(proxy, ACLK_PROXY_PROTO_ADDR_SEPARATOR);
    if(!cur)
        cur = proxy;
    else
        cur += strlen(ACLK_PROXY_PROTO_ADDR_SEPARATOR);

    while(cur < auth) {
        *cur='X';
        cur++;
    }
}

static inline void safe_log_proxy_error(char *str, const char *proxy) {
    char *log = strdupz(proxy);
    safe_log_proxy_censor(log);
    error("%s Provided Value:\"%s\"", str, log);
    freez(log);
}

static inline int check_socks_enviroment(const char **proxy) {
    char *tmp = getenv("socks_proxy");

    if(!tmp)
        return 1;

    if(aclk_verify_proxy(tmp) == PROXY_TYPE_SOCKS5) {
        *proxy = tmp;
        return 0;
    }

    safe_log_proxy_error("Environment var \"socks_proxy\" defined but of unknown format. Supported syntax: \"socks5[h]://[user:pass@]host:ip\".", tmp);
    return 1;
}

static const char *aclk_lws_wss_get_proxy_setting(ACLK_PROXY_TYPE *type) {
    const char *proxy = config_get(CONFIG_SECTION_ACLK, ACLK_PROXY_CONFIG_VAR, ACLK_PROXY_ENV);
    *type = PROXY_DISABLED;

    if(strcmp(proxy, "none") == 0)
        return proxy;

    if(strcmp(proxy, ACLK_PROXY_ENV) == 0) {
        if(check_socks_enviroment(&proxy) == 0)
            *type = PROXY_TYPE_SOCKS5;
        return proxy;
    }

    *type = aclk_verify_proxy(proxy);
    if(*type == PROXY_TYPE_UNKNOWN) {
        *type = PROXY_DISABLED;
        safe_log_proxy_error("Config var \"" ACLK_PROXY_CONFIG_VAR "\" defined but of unknown format. Supported syntax: \"socks5[h]://[user:pass@]host:ip\".", proxy);
    }

    return proxy;
}

// Return code indicates if connection attempt has started async.
int aclk_lws_wss_connect(char *host, int port)
{
    struct lws_client_connect_info i;
    struct lws_vhost *vhost;
    static const char *proxy = NULL;
    static ACLK_PROXY_TYPE proxy_type = PROXY_NOT_SET;
    char *log;

    if (!engine_instance) {
        return aclk_lws_wss_client_init(host, port);
        // PROTOCOL_INIT callback will call again.
    }

    if(proxy_type == PROXY_NOT_SET)
        proxy = aclk_lws_wss_get_proxy_setting(&proxy_type);

    if (engine_instance->lws_wsi) {
        error("Already Connected. Only one connection supported at a time.");
        return 0;
    }

    // from LWS docu:
    // If option LWS_SERVER_OPTION_EXPLICIT_VHOSTS is given, no vhost is
    // created; you're expected to create your own vhosts afterwards using
    // lws_create_vhost().  Otherwise a vhost named "default" is also created
    // using the information in the vhost-related members, for compatibility.
    vhost = lws_get_vhost_by_name(engine_instance->lws_context, "default");
    if(!vhost)
        fatal("Could not find the default LWS vhost.");

    memset(&i, 0, sizeof(i));
    i.context = engine_instance->lws_context;
    i.port = engine_instance->port;
    i.address = engine_instance->host;
    i.path = "/mqtt";
    i.host = engine_instance->host;
    i.protocol = "mqtt";

    switch (proxy_type) {
    case PROXY_DISABLED:
        lws_set_socks(vhost, ":");
        lws_set_proxy(vhost, ":");
        break;
    case PROXY_TYPE_SOCKS5:
        log = strdupz(proxy);
        safe_log_proxy_censor(log);
        info("Connecting using SOCKS5 proxy:\"%s\"", log);
        freez(log);
        if(_aclk_wss_set_socks(vhost, proxy))
            error("LWS failed to accept socks proxy.");
        break;
    default:
        error("The proxy could not be set. Unknown proxy type.");
    }

#ifdef ACLK_SSL_ALLOW_SELF_SIGNED
    i.ssl_connection = LCCSCF_USE_SSL | LCCSCF_ALLOW_SELFSIGNED | LCCSCF_SKIP_SERVER_CERT_HOSTNAME_CHECK;
    info("Disabling SSL certificate checks");
#else
    i.ssl_connection = LCCSCF_USE_SSL;
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
        default:
            // Not using an internal buffer here for thread-safety with unknown calling context.
            error("Unknown LWS callback %u", reason);
            return "unknown";
    }
}
static int aclk_lws_wss_callback(struct lws *wsi, enum lws_callback_reasons reason, void *user, void *in, size_t len)
{
    UNUSED(user);
    struct lws_wss_packet_buffer *data;
    int retval = 0;

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
            data = lws_wss_packet_buffer_pop(&engine_instance->write_buffer_head);
            if (likely(data)) {
                lws_write(wsi, data->data + LWS_PRE, data->data_size, LWS_WRITE_BINARY);
                lws_wss_packet_buffer_free(data);
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
            aclk_lws_wss_clear_io_buffers(engine_instance);
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

abort:
    aclk_lws_mutex_unlock(&engine_instance->read_buf_mutex);
    return data_to_be_read;
}

void aclk_lws_wss_service_loop()
{
    if (engine_instance)
        lws_service(engine_instance->lws_context, 0);
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
