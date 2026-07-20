// SPDX-License-Identifier: GPL-3.0-or-later

#include "webrtc.h"

#include "../server/web_client.h"
#include "../server/web_client_cache.h"

#ifdef HAVE_LIBDATACHANNEL

#include <limits.h>

#include "rtc/rtc.h"

#if defined(ENABLE_LZ4)
#include <lz4.h>
#endif

#define WEBRTC_OUR_MAX_MESSAGE_SIZE (5 * 1024 * 1024)
#define WEBRTC_DEFAULT_REMOTE_MAX_MESSAGE_SIZE (65536)
#define WEBRTC_COMPRESSED_HEADER_SIZE 200
#define WEBRTC_REQUEST_PARSER_PADDING 10

static void webrtc_log(rtcLogLevel level, const char *message) {
    switch(level) {
        case RTC_LOG_NONE:
            break;

        case RTC_LOG_WARNING:
        case RTC_LOG_ERROR:
        case RTC_LOG_FATAL:
            netdata_log_error("WEBRTC: %s", message);
            break;

        case RTC_LOG_INFO:
            netdata_log_info("WEBRTC: %s", message);
            break;

        default:
        case RTC_LOG_DEBUG:
        case RTC_LOG_VERBOSE:
            internal_error(true, "WEBRTC: %s", message);
            break;

    }
}

typedef struct webrtc_datachannel {
    int dc;
    char *label;
    struct webrtc_connection *conn;

    bool open; // atomic

    struct {
        struct webrtc_datachannel *prev;
        struct webrtc_datachannel *next;
    } link;
} WEBRTC_DC;

typedef struct webrtc_connection {
    int pc;
    rtcConfiguration config;
    rtcState state;
    rtcGatheringState gathering_state;

    size_t max_message_size;
    size_t local_max_message_size;
    size_t remote_max_message_size;

    struct {
        SPINLOCK spinlock;
        BUFFER *wb;
        bool sdp;
        bool candidates;
    } response;

    struct {
        SPINLOCK spinlock;
        WEBRTC_DC *head;
    } channels;

    struct {
        struct webrtc_connection *prev;
        struct webrtc_connection *next;
    } link;
} WEBRTC_CONN;

#define WEBRTC_MAX_ICE_SERVERS 100

static struct {
    bool enabled;
    char *iceServers[WEBRTC_MAX_ICE_SERVERS];
    int iceServersCount;
    const char *proxyServer;
    const char *bindAddress;

    struct {
        SPINLOCK spinlock;
        WEBRTC_CONN *head;
    } unsafe;

} webrtc_base = {
#ifdef NETDATA_INTERNAL_CHECKS
        .enabled = true,
#else
        .enabled = false,
#endif
        .iceServers = {
                // Format:
                // [("stun"|"turn"|"turns") (":"|"://")][username ":" password "@"]hostname[":" port]["?transport=" ("udp"|"tcp"|"tls")]
                //
                // Note transports TCP and TLS are only available for a TURN server with libnice as ICE backend and govern only the
                // TURN control connection, meaning relaying is always performed over UDP.
                //
                // If the username or password of a URI contains reserved special characters, they must be percent-encoded.
                // In particular, ":" must be encoded as "%3A" and "@" must by encoded as "%40".

                "stun://stun.l.google.com:19302",
                NULL, // terminator
        },
        .iceServersCount = 1,
        .proxyServer = NULL, // [("http"|"socks5") (":"|"://")][username ":" password "@"]hostname["    :" port]
        .bindAddress = NULL,
        .unsafe = {
                .spinlock = SPINLOCK_INITIALIZER,
                .head = NULL,
        },
};

static inline bool webrtc_dc_is_open(WEBRTC_DC *chan) {
    return __atomic_load_n(&chan->open, __ATOMIC_RELAXED);
}

static inline rtcState webrtc_conn_state(WEBRTC_CONN *conn) {
    return __atomic_load_n(&conn->state, __ATOMIC_RELAXED);
}

static inline void webrtc_conn_set_state(WEBRTC_CONN *conn, rtcState state) {
    __atomic_store_n(&conn->state, state, __ATOMIC_RELAXED);
}

static inline rtcGatheringState webrtc_conn_gathering_state(WEBRTC_CONN *conn) {
    return __atomic_load_n(&conn->gathering_state, __ATOMIC_RELAXED);
}

static inline void webrtc_conn_set_gathering_state(WEBRTC_CONN *conn, rtcGatheringState state) {
    __atomic_store_n(&conn->gathering_state, state, __ATOMIC_RELAXED);
}

static void cleanupConnections(void);

static void webrtc_config_ice_servers(void) {
    BUFFER *wb = buffer_create(0, NULL);

    int i;
    for(i = 0; i < WEBRTC_MAX_ICE_SERVERS ;i++) {
        if (webrtc_base.iceServers[i]) {
            if (buffer_strlen(wb))
                buffer_strcat(wb, " ");

            internal_error(true, "WEBRTC: default ice server No %d is: '%s'", i, webrtc_base.iceServers[i]);
            buffer_strcat(wb, webrtc_base.iceServers[i]);
        }
        else
            break;
    }
    webrtc_base.iceServersCount = i;
    internal_error(true, "WEBRTC: there are %d default ice servers: '%s'", webrtc_base.iceServersCount, buffer_tostring(wb));

    const char *servers = inicfg_get(&netdata_config, CONFIG_SECTION_WEBRTC, "ice servers", buffer_tostring(wb));

    webrtc_base.iceServersCount = 0;
    char tmp[strlen(servers) + 1];
    strcpy(tmp, servers);
    char *s = tmp, *e;
    while(*s) {
        if(isspace((uint8_t)*s))
            s++;

        e = s;
        while(*e && !isspace((uint8_t)*e))
            e++;

        if(s != e && webrtc_base.iceServersCount < WEBRTC_MAX_ICE_SERVERS) {
            char old = *e;
            *e = '\0';
            internal_error(true, "WEBRTC: ice server No %d is: '%s'", webrtc_base.iceServersCount, s);
            webrtc_base.iceServers[webrtc_base.iceServersCount++] = strdupz(s);
            *e = old;
        }

        if(*e)
            s = e + 1;
        else
            break;
    }

    buffer_free(wb);
}

void webrtc_initialize() {
    webrtc_base.enabled = inicfg_get_boolean(&netdata_config, CONFIG_SECTION_WEBRTC, "enabled", webrtc_base.enabled);
    internal_error(true, "WEBRTC: is %s", webrtc_base.enabled ? "enabled" : "disabled");

    webrtc_config_ice_servers();

    webrtc_base.proxyServer = inicfg_get(&netdata_config, CONFIG_SECTION_WEBRTC, "proxy server", webrtc_base.proxyServer ? webrtc_base.proxyServer : "");
    if(!webrtc_base.proxyServer || !*webrtc_base.proxyServer)
        webrtc_base.proxyServer = NULL;

    internal_error(true, "WEBRTC: proxy server is: '%s'", webrtc_base.proxyServer ? webrtc_base.proxyServer : "");

    webrtc_base.bindAddress = inicfg_get(&netdata_config, CONFIG_SECTION_WEBRTC, "bind address", webrtc_base.bindAddress ? webrtc_base.bindAddress : "");
    if(!webrtc_base.bindAddress || !*webrtc_base.bindAddress)
        webrtc_base.bindAddress = NULL;

    internal_error(true, "WEBRTC: bind address is: '%s'", webrtc_base.bindAddress ? webrtc_base.bindAddress : "");

    if(!webrtc_base.enabled)
        return;

    rtcLogLevel level;
#ifdef NETDATA_INTERNAL_CHECKS
    level = RTC_LOG_INFO;
#else
    level = RTC_LOG_WARNING;
#endif

    rtcInitLogger(level, webrtc_log);
    rtcPreload();
}

void webrtc_close_all_connections() {
    if(!webrtc_base.enabled)
        return;

    rtcCleanup();
}

size_t find_max_message_size_in_sdp(const char *sdp) {
    char *s = strstr(sdp, "a=max-message-size:");
    if(s)
        return str2ul(&s[19]);

    return WEBRTC_DEFAULT_REMOTE_MAX_MESSAGE_SIZE;
}

// ----------------------------------------------------------------------------
// execute web API requests

static bool web_client_stop_callback(struct web_client *w __maybe_unused, void *data) {
    WEBRTC_DC *chan = data;
    return !webrtc_dc_is_open(chan);
}

static size_t webrtc_send_in_chunks(WEBRTC_DC *chan, const char *data, size_t size, int code, const char *message_type, HTTP_CONTENT_TYPE content_type, size_t max_message_size, bool binary) {
    size_t sent_bytes = 0;
    size_t chunk = 0;
    size_t total_chunks = size / max_message_size;
    if(total_chunks * max_message_size < size)
        total_chunks++;

    char *send_buffer = mallocz(chan->conn->max_message_size);

    char *s = (char *)data;
    size_t remaining = size;
    while(remaining > 0) {
        chunk++;

        size_t message_size = MIN(remaining, max_message_size);

        int len = snprintfz(send_buffer, WEBRTC_COMPRESSED_HEADER_SIZE, "%d %s %zu %zu %zu %s\r\n",
                            code,
                            message_type,
                            message_size,
                            chunk,
                            total_chunks,
                            content_type_id2string(content_type)
        );

        internal_fatal((size_t)len != strlen(send_buffer), "WEBRTC compressed header line mismatch");
        internal_fatal(len + message_size > chan->conn->max_message_size, "WEBRTC message exceeds max message size");

        memcpy(&send_buffer[len], s, message_size);

        int total_message_size = (int)(len + message_size);
        sent_bytes += total_message_size;

        if(!binary)
            total_message_size = -total_message_size;

        if(rtcSendMessage(chan->dc, send_buffer, total_message_size) != RTC_ERR_SUCCESS)
            netdata_log_error("WEBRTC[%d],DC[%d]: failed to send LZ4 chunk %zu of %zu", chan->conn->pc, chan->dc, chunk, total_chunks);
        else
            internal_error(true, "WEBRTC[%d],DC[%d]: sent chunk %zu of %zu, size %zu (total %d)",
                           chan->conn->pc, chan->dc, chunk, total_chunks, message_size, total_message_size);

        s = s + message_size;
        remaining -= message_size;
    }

    internal_fatal(chunk != total_chunks, "WEBRTC number of compressed chunks mismatch");

    freez(send_buffer);
    return sent_bytes;
}

static void webrtc_execute_api_request(WEBRTC_DC *chan, const char *request, size_t size, bool binary) {
    ND_LOG_STACK lgs[] = {
            ND_LOG_FIELD_TXT(NDF_SRC_TRANSPORT, "webrtc"),
            ND_LOG_FIELD_END(),
    };
    ND_LOG_STACK_PUSH(lgs);

    internal_error(true, "WEBRTC[%d],DC[%d]: got request of size %zu and type %s.",
                   chan->conn->pc, chan->dc, size, binary ? "binary" : "text");

    struct web_client *w = web_client_get_from_cache();
    w->statistics.received_bytes = size;
    w->interrupt.callback = web_client_stop_callback;
    w->interrupt.callback_data = chan;
    web_client_set_conn_webrtc(w);

    w->port_acl = HTTP_ACL_WEBRTC | HTTP_ACL_ALL_FEATURES;
    w->acl = w->port_acl;

    char *path = (char *)request;
    if(strncmp(request, "POST ", 5) == 0) {
        w->mode = HTTP_REQUEST_MODE_POST;
        path += 10;
    }
    else if(strncmp(request, "GET ", 4) == 0) {
        w->mode = HTTP_REQUEST_MODE_GET;
        path += 4;
    }

    web_client_timeout_checkpoint_set(w, 0);
    size_t path_offset = (size_t)(path - request);
    HTTP_VALIDATION validation = path_offset > size ? HTTP_VALIDATION_MALFORMED_URL :
        web_client_decode_path_and_query_string(w, path, size - path_offset);
    if(unlikely(validation != HTTP_VALIDATION_OK)) {
        buffer_flush(w->response.data);
        if(validation == HTTP_VALIDATION_URL_TOO_LONG) {
            buffer_strcat(w->response.data, "Request target is too long.");
        }
        else {
            buffer_strcat(w->response.data, "Malformed URL.");
        }
        w->response.code = http_validation_error_to_response_code(validation);
    }
    else {
        path = (char *)buffer_tostring(w->url_path_decoded);
        w->response.code = (short)web_client_api_request_with_node_selection(localhost, w, path);
    }
    web_client_timeout_checkpoint_response_ready(w, NULL);

    size_t sent_bytes = 0;
    size_t response_size = buffer_strlen(w->response.data);

    bool send_plain = true;
    size_t max_message_size = chan->conn->max_message_size - WEBRTC_COMPRESSED_HEADER_SIZE;

    if(!webrtc_dc_is_open(chan)) {
        internal_error(true, "WEBRTC[%d],DC[%d]: ignoring API response on closed data channel.", chan->conn->pc, chan->dc);
        goto cleanup;
    }
    else {
        internal_error(true, "WEBRTC[%d],DC[%d]: prepared response with code %d, size %zu.",
                       chan->conn->pc, chan->dc, w->response.code, response_size);
    }

#if defined(ENABLE_LZ4)
    if(response_size <= LZ4_MAX_INPUT_SIZE) {
        int max_compressed_size = LZ4_compressBound((int)response_size);
        char *compressed = mallocz(max_compressed_size);

        int compressed_size = LZ4_compress_default(buffer_tostring(w->response.data), compressed,
                                                   (int)response_size, max_compressed_size);

        if(compressed_size > 0) {
            send_plain = false;
            sent_bytes = webrtc_send_in_chunks(chan, compressed, compressed_size,
                                               w->response.code, "LZ4", w->response.data->content_type,
                                               max_message_size, true);
        }
        freez(compressed);
    }
#endif

    if(send_plain)
        sent_bytes = webrtc_send_in_chunks(chan, buffer_tostring(w->response.data), buffer_strlen(w->response.data),
                              w->response.code, "PLAIN", w->response.data->content_type,
                              max_message_size, false);

    w->statistics.sent_bytes = sent_bytes;

cleanup:
    web_client_log_completed_request(w, false);
    web_client_release_to_cache(w);
}

// ----------------------------------------------------------------------------
// webrtc data channel

static void myOpenCallback(int id __maybe_unused, void *user_ptr) {
    webrtc_set_thread_name();

    WEBRTC_DC *chan = user_ptr;
    internal_fatal(chan->dc != id, "WEBRTC[%d],DC[%d]: dc mismatch, expected %d, got %d", chan->conn->pc, chan->dc, chan->dc, id);

    nd_log(NDLS_ACCESS, NDLP_DEBUG, "WEBRTC[%d],DC[%d]: %d DATA CHANNEL '%s' OPEN", chan->conn->pc, chan->dc, gettid_cached(), chan->label);
    internal_error(true, "WEBRTC[%d],DC[%d]: data channel opened.", chan->conn->pc, chan->dc);
    __atomic_store_n(&chan->open, true, __ATOMIC_RELAXED);
}

static void myClosedCallback(int id __maybe_unused, void *user_ptr) {
    webrtc_set_thread_name();

    WEBRTC_DC *chan = user_ptr;
    WEBRTC_CONN *conn = chan->conn;
    int pc = conn->pc;
    int dc = chan->dc;
    const char *label = chan->label;

    internal_fatal(dc != id, "WEBRTC[%d],DC[%d]: dc mismatch, expected %d, got %d", pc, dc, dc, id);

    __atomic_store_n(&chan->open, false, __ATOMIC_RELAXED);
    internal_error(true, "WEBRTC[%d],DC[%d]: data channel closed.", pc, dc);

    if(rtcDeleteDataChannel(dc) != RTC_ERR_SUCCESS)
        netdata_log_error("WEBRTC[%d],DC[%d]: rtcDeleteDataChannel() failed.", pc, dc);

    spinlock_lock(&conn->channels.spinlock);
    DOUBLE_LINKED_LIST_REMOVE_ITEM_UNSAFE(conn->channels.head, chan, link.prev, link.next);
    spinlock_unlock(&conn->channels.spinlock);

    nd_log(NDLS_ACCESS, NDLP_DEBUG, "WEBRTC[%d],DC[%d]: %d DATA CHANNEL '%s' CLOSED", pc, dc, gettid_cached(), label);

    freez(chan->label);
    freez(chan);

    cleanupConnections();
}

static void myErrorCallback(int id __maybe_unused, const char *error, void *user_ptr) {
    webrtc_set_thread_name();

    WEBRTC_DC *chan = user_ptr;
    internal_fatal(chan->dc != id, "WEBRTC[%d],DC[%d]: dc mismatch, expected %d, got %d", chan->conn->pc, chan->dc, chan->dc, id);

    netdata_log_error("WEBRTC[%d],DC[%d]: ERROR: '%s'", chan->conn->pc, chan->dc, error);
}

static void myMessageCallback(int id __maybe_unused, const char *message, int size, void *user_ptr) {
    webrtc_set_thread_name();

    WEBRTC_DC *chan = user_ptr;
    internal_fatal(chan->dc != id, "WEBRTC[%d],DC[%d]: dc mismatch, expected %d, got %d", chan->conn->pc, chan->dc, chan->dc, id);
    internal_fatal(!webrtc_dc_is_open(chan), "WEBRTC[%d],DC[%d]: received message on closed channel", chan->conn->pc, chan->dc);

    bool binary = (size >= 0);
    if(size == INT_MIN) {
        netdata_log_error("WEBRTC[%d],DC[%d]: invalid message size.", chan->conn->pc, chan->dc);
        return;
    }

    if(size < 0)
        size = -size;

    if(unlikely(!message)) {
        netdata_log_error("WEBRTC[%d],DC[%d]: received NULL message.", chan->conn->pc, chan->dc);
        return;
    }

    size_t request_size = (size_t)size;
    CLEAN_CHAR_P *request = mallocz(request_size + 1 + WEBRTC_REQUEST_PARSER_PADDING);
    memcpy(request, message, request_size);
    memset(&request[request_size], 0, 1 + WEBRTC_REQUEST_PARSER_PADDING);

    webrtc_execute_api_request(chan, request, request_size, binary);
}

static bool webrtc_conn_is_linked_unsafe(WEBRTC_CONN *conn) {
    for(WEBRTC_CONN *t = webrtc_base.unsafe.head; t ;t = t->link.next) {
        if(t == conn)
            return true;
    }

    return false;
}

//#define WEBRTC_MAX_REQUEST_SIZE 65536
//
//static void myAvailableCallback(int id, void *user_ptr) {
//    webrtc_set_thread_name();
//
//    WEBRTC_DC *chan = user_ptr;
//    internal_fatal(chan->dc != id, "WEBRTC[%d],DC[%d]: dc mismatch, expected %d, got %d", chan->conn->pc, chan->dc, chan->dc, id);
//
//    internal_fatal(!chan->open, "WEBRTC[%d],DC[%d]: received message on closed channel", chan->conn->pc, chan->dc);
//
//    int size = WEBRTC_MAX_REQUEST_SIZE;
//    char buffer[WEBRTC_MAX_REQUEST_SIZE];
//    while(rtcReceiveMessage(id, buffer, &size) == RTC_ERR_SUCCESS) {
//        bool binary = (size >= 0);
//        if(size < 0)
//            size = -size;
//
//        webrtc_execute_api_request(chan, message, size, binary);
//    }
//}

static void myDataChannelCallback(int pc, int dc, void *user_ptr) {
    webrtc_set_thread_name();

    WEBRTC_CONN *conn = user_ptr;
    WEBRTC_DC *chan = callocz(1, sizeof(WEBRTC_DC));
    chan->dc = dc;
    chan->conn = conn;

    spinlock_lock(&webrtc_base.unsafe.spinlock);
    if(unlikely(!webrtc_conn_is_linked_unsafe(conn) || webrtc_conn_state(conn) == RTC_CLOSED)) {
        spinlock_unlock(&webrtc_base.unsafe.spinlock);

        internal_error(true, "WEBRTC[%d],DC[%d]: ignoring data channel for closed connection.", pc, dc);
        freez(chan);
        if(rtcDeleteDataChannel(dc) != RTC_ERR_SUCCESS)
            netdata_log_error("WEBRTC[%d],DC[%d]: rtcDeleteDataChannel() failed.", pc, dc);

        return;
    }

    internal_fatal(conn->pc != pc, "WEBRTC[%d]: pc mismatch, expected %d, got %d", conn->pc, conn->pc, pc);

    spinlock_lock(&conn->channels.spinlock);
    DOUBLE_LINKED_LIST_APPEND_ITEM_UNSAFE(conn->channels.head, chan, link.prev, link.next);
    spinlock_unlock(&conn->channels.spinlock);
    spinlock_unlock(&webrtc_base.unsafe.spinlock);

    rtcSetUserPointer(dc, chan);

    char label[1024 + 1];
    rtcGetDataChannelLabel(dc, label, 1024);
    label[1024] = '\0';

    chan->label = strdupz(label);

    if(rtcSetOpenCallback(dc, myOpenCallback) != RTC_ERR_SUCCESS)
        netdata_log_error("WEBRTC[%d],DC[%d]: rtcSetOpenCallback() failed.", conn->pc, chan->dc);

    if(rtcSetClosedCallback(dc, myClosedCallback) != RTC_ERR_SUCCESS)
        netdata_log_error("WEBRTC[%d],DC[%d]: rtcSetClosedCallback() failed.", conn->pc, chan->dc);

    if(rtcSetErrorCallback(dc, myErrorCallback) != RTC_ERR_SUCCESS)
        netdata_log_error("WEBRTC[%d],DC[%d]: rtcSetErrorCallback() failed.", conn->pc, chan->dc);

    if(rtcSetMessageCallback(dc, myMessageCallback) != RTC_ERR_SUCCESS)
        netdata_log_error("WEBRTC[%d],DC[%d]: rtcSetMessageCallback() failed.", conn->pc, chan->dc);

//    if(rtcSetAvailableCallback(dc, myAvailableCallback) != RTC_ERR_SUCCESS)
//        netdata_log_error("WEBRTC[%d],DC[%d]: rtcSetAvailableCallback() failed.", conn->pc, chan->dc);

    internal_error(true, "WEBRTC[%d],DC[%d]: new data channel with label '%s'", chan->conn->pc, chan->dc, chan->label);
}

// ----------------------------------------------------------------------------
// webrtc connection

static WEBRTC_CONN *webrtc_connection_to_destroy_unsafe(void) {
    WEBRTC_CONN *conn = webrtc_base.unsafe.head;

    while(conn) {
        WEBRTC_CONN *next = conn->link.next;

        if(webrtc_conn_state(conn) != RTC_CLOSED) {
            conn = next;
            continue;
        }

        spinlock_lock(&conn->channels.spinlock);
        WEBRTC_DC *chan = conn->channels.head;
        spinlock_unlock(&conn->channels.spinlock);

        if(!chan) {
            DOUBLE_LINKED_LIST_REMOVE_ITEM_UNSAFE(webrtc_base.unsafe.head, conn, link.prev, link.next);
            return conn;
        }

        internal_error(true, "WEBRTC[%d]: not destroying closed connection because it has data channels running", conn->pc);
        conn = next;
    }

    return NULL;
}

static void webrtc_delete_connection(WEBRTC_CONN *conn) {
    internal_error(true, "WEBRTC[%d]: destroying connection", conn->pc);

    if(rtcDeletePeerConnection(conn->pc) != RTC_ERR_SUCCESS)
        netdata_log_error("WEBRTC[%d]: rtcDeletePeerConnection() failed.", conn->pc);

    freez(conn);
}

static void cleanupConnections(void) {
    while(true) {
        spinlock_lock(&webrtc_base.unsafe.spinlock);
        WEBRTC_CONN *conn = webrtc_connection_to_destroy_unsafe();
        spinlock_unlock(&webrtc_base.unsafe.spinlock);

        if(!conn)
            break;

        webrtc_delete_connection(conn);
    }
}

static WEBRTC_CONN * webrtc_create_connection(void) {
    WEBRTC_CONN *conn = callocz(1, sizeof(WEBRTC_CONN));

    spinlock_init(&conn->response.spinlock);
    spinlock_init(&conn->channels.spinlock);

    spinlock_lock(&webrtc_base.unsafe.spinlock);
    DOUBLE_LINKED_LIST_APPEND_ITEM_UNSAFE(webrtc_base.unsafe.head, conn, link.prev, link.next);
    spinlock_unlock(&webrtc_base.unsafe.spinlock);
    return conn;
}

static void myDescriptionCallback(int pc __maybe_unused, const char *sdp, const char *type, void *user_ptr) {
    webrtc_set_thread_name();

    WEBRTC_CONN *conn = user_ptr;
    internal_fatal(conn->pc != pc, "WEBRTC[%d]: pc mismatch, expected %d, got %d", conn->pc, conn->pc, pc);

    internal_error(true, "WEBRTC[%d]: local description type '%s': %s", conn->pc, type, sdp);
    spinlock_lock(&conn->response.spinlock);
    if(conn->response.wb && !conn->response.candidates) {
        buffer_json_member_add_string(conn->response.wb, "sdp", sdp);
        buffer_json_member_add_string(conn->response.wb, "type", type);
        conn->response.sdp = true;
    }
    spinlock_unlock(&conn->response.spinlock);

    conn->local_max_message_size = find_max_message_size_in_sdp(sdp);
}

static void myCandidateCallback(int pc __maybe_unused, const char *cand, const char *mid __maybe_unused, void *user_ptr) {
    webrtc_set_thread_name();

    WEBRTC_CONN *conn = user_ptr;
    internal_fatal(conn->pc != pc, "WEBRTC[%d]: pc mismatch, expected %d, got %d", conn->pc, conn->pc, pc);

    spinlock_lock(&conn->response.spinlock);
    internal_error(true, "WEBRTC[%d]: local candidate '%s', mid '%s'", conn->pc, cand, mid);
    if(conn->response.wb) {
        if(!conn->response.candidates) {
            buffer_json_member_add_array(conn->response.wb, "candidates");
            conn->response.candidates = true;
        }

        buffer_json_add_array_item_string(conn->response.wb, cand);
    }
    spinlock_unlock(&conn->response.spinlock);
}

static void myStateChangeCallback(int pc __maybe_unused, rtcState state, void *user_ptr) {
    webrtc_set_thread_name();

    WEBRTC_CONN *conn = user_ptr;
    internal_fatal(conn->pc != pc, "WEBRTC[%d]: pc mismatch, expected %d, got %d", conn->pc, conn->pc, pc);

    webrtc_conn_set_state(conn, state);

    switch(state) {
        case RTC_NEW:
            internal_error(true, "WEBRTC[%d]: new connection...", conn->pc);
            break;

        case RTC_CONNECTING:
            nd_log(NDLS_ACCESS, NDLP_DEBUG, "WEBRTC[%d]: %d CONNECTING", conn->pc, gettid_cached());
            internal_error(true, "WEBRTC[%d]: connecting...", conn->pc);
            break;

        case RTC_CONNECTED:
            nd_log(NDLS_ACCESS, NDLP_DEBUG, "WEBRTC[%d]: %d CONNECTED", conn->pc, gettid_cached());
            internal_error(true, "WEBRTC[%d]: connected!", conn->pc);
            break;

        case RTC_DISCONNECTED:
            nd_log(NDLS_ACCESS, NDLP_DEBUG, "WEBRTC[%d]: %d DISCONNECTED", conn->pc, gettid_cached());
            internal_error(true, "WEBRTC[%d]: disconnected.", conn->pc);
            break;

        case RTC_FAILED:
            nd_log(NDLS_ACCESS, NDLP_DEBUG, "WEBRTC[%d]: %d CONNECTION FAILED", conn->pc, gettid_cached());
            internal_error(true, "WEBRTC[%d]: failed.", conn->pc);
            break;

        case RTC_CLOSED:
            nd_log(NDLS_ACCESS, NDLP_DEBUG, "WEBRTC[%d]: %d CONNECTION CLOSED", conn->pc, gettid_cached());
            internal_error(true, "WEBRTC[%d]: closed.", conn->pc);
            cleanupConnections();
            break;
    }
}

static void myGatheringStateCallback(int pc __maybe_unused, rtcGatheringState state, void *user_ptr) {
    webrtc_set_thread_name();

    WEBRTC_CONN *conn = user_ptr;
    internal_fatal(conn->pc != pc, "WEBRTC[%d]: pc mismatch, expected %d, got %d", conn->pc, conn->pc, pc);

    webrtc_conn_set_gathering_state(conn, state);

    switch(state) {
        case RTC_GATHERING_NEW:
            internal_error(true, "WEBRTC[%d]: gathering...", conn->pc);
            break;

        case RTC_GATHERING_INPROGRESS:
            internal_error(true, "WEBRTC[%d]: gathering in progress...", conn->pc);
            break;

        case RTC_GATHERING_COMPLETE:
            internal_error(true, "WEBRTC[%d]: gathering complete!", conn->pc);
            break;
    }
}

int webrtc_new_connection(const char *sdp, BUFFER *wb) {
    if(unlikely(!webrtc_base.enabled)) {
        buffer_flush(wb);
        buffer_strcat(wb, "WebRTC is not enabled on this agent.");
        wb->content_type = CT_TEXT_PLAIN;
        return HTTP_RESP_BAD_REQUEST;
    }

    cleanupConnections();

    if(unlikely(!sdp || !*sdp)) {
        buffer_flush(wb);
        buffer_strcat(wb, "No SDP message posted with the request");
        wb->content_type = CT_TEXT_PLAIN;
        return HTTP_RESP_BAD_REQUEST;
    }

    buffer_flush(wb);
    buffer_json_initialize(wb, "\"", "\"", 0, true, BUFFER_JSON_OPTIONS_DEFAULT);
    wb->content_type = CT_APPLICATION_JSON;

    WEBRTC_CONN *conn = webrtc_create_connection();
    conn->response.wb = wb;
    conn->max_message_size = WEBRTC_DEFAULT_REMOTE_MAX_MESSAGE_SIZE;
    conn->local_max_message_size = WEBRTC_OUR_MAX_MESSAGE_SIZE;
    conn->remote_max_message_size = find_max_message_size_in_sdp(sdp);

    conn->config.iceServers = (const char **)webrtc_base.iceServers;
    conn->config.iceServersCount = webrtc_base.iceServersCount;
    conn->config.proxyServer = webrtc_base.proxyServer;
    conn->config.bindAddress = webrtc_base.bindAddress;
    conn->config.certificateType = RTC_CERTIFICATE_DEFAULT;
    conn->config.iceTransportPolicy = RTC_TRANSPORT_POLICY_ALL;
    conn->config.enableIceTcp = true; // libnice only
    conn->config.enableIceUdpMux = true; // libjuice only
    conn->config.disableAutoNegotiation = false;
    conn->config.forceMediaTransport = false;
    conn->config.portRangeBegin = 0; // 0 means automatic
    conn->config.portRangeEnd = 0; // 0 means automatic
    conn->config.mtu = 0; // <= 0 means automatic
    conn->config.maxMessageSize = WEBRTC_OUR_MAX_MESSAGE_SIZE; // <= 0 means default

    conn->pc = rtcCreatePeerConnection(&conn->config);
    rtcSetUserPointer(conn->pc, conn);

    if(rtcSetLocalDescriptionCallback(conn->pc, myDescriptionCallback) != RTC_ERR_SUCCESS)
        netdata_log_error("WEBRTC[%d]: rtcSetLocalDescriptionCallback() failed", conn->pc);

    if(rtcSetLocalCandidateCallback(conn->pc, myCandidateCallback) != RTC_ERR_SUCCESS)
        netdata_log_error("WEBRTC[%d]: rtcSetLocalCandidateCallback() failed", conn->pc);

    if(rtcSetStateChangeCallback(conn->pc, myStateChangeCallback) != RTC_ERR_SUCCESS)
        netdata_log_error("WEBRTC[%d]: rtcSetStateChangeCallback() failed", conn->pc);

    if(rtcSetGatheringStateChangeCallback(conn->pc, myGatheringStateCallback) != RTC_ERR_SUCCESS)
        netdata_log_error("WEBRTC[%d]: rtcSetGatheringStateChangeCallback() failed", conn->pc);

    if(rtcSetDataChannelCallback(conn->pc, myDataChannelCallback) != RTC_ERR_SUCCESS)
        netdata_log_error("WEBRTC[%d]: rtcSetDataChannelCallback() failed", conn->pc);

    // initialize the handshake
    internal_error(true, "WEBRTC[%d]: setting remote sdp: %s", conn->pc, sdp);
    if(rtcSetRemoteDescription(conn->pc, sdp, "offer") != RTC_ERR_SUCCESS)
        netdata_log_error("WEBRTC[%d]: rtcSetRemoteDescription() failed", conn->pc);

    // initiate the handshake process
    if(conn->config.disableAutoNegotiation) {
        if(rtcSetLocalDescription(conn->pc, NULL) != RTC_ERR_SUCCESS)
            netdata_log_error("WEBRTC[%d]: rtcSetLocalDescription() failed", conn->pc);
    }

    bool logged = false;
    while(webrtc_conn_gathering_state(conn) != RTC_GATHERING_COMPLETE) {
        if(!logged) {
            logged = true;
            internal_error(true, "WEBRTC[%d]: Waiting for gathering to complete", conn->pc);
        }
        sleep_usec(1000);
    }

    if(logged)
        internal_error(true, "WEBRTC[%d]: Gathering finished, our answer is ready", conn->pc);

    conn->max_message_size = MIN(conn->local_max_message_size, conn->remote_max_message_size);
    if(conn->max_message_size <= WEBRTC_COMPRESSED_HEADER_SIZE)
        conn->max_message_size = WEBRTC_COMPRESSED_HEADER_SIZE + 1;

    spinlock_lock(&conn->response.spinlock);
    internal_fatal(!conn->response.sdp, "WEBRTC[%d]: response does not have an SDP: %s", conn->pc, buffer_tostring(conn->response.wb));
    internal_fatal(!conn->response.candidates, "WEBRTC[%d]: response does not have candidates: %s", conn->pc, buffer_tostring(conn->response.wb));
    buffer_json_finalize(wb);
    conn->response.wb = NULL;
    spinlock_unlock(&conn->response.spinlock);

    return HTTP_RESP_OK;
}

#else // ! HAVE_LIBDATACHANNEL

void webrtc_initialize() {
    ;
}

int webrtc_new_connection(const char *sdp __maybe_unused, BUFFER *wb) {
    buffer_flush(wb);
    buffer_strcat(wb, "WEBRTC is not available on this server");
    wb->content_type = CT_TEXT_PLAIN;
    return HTTP_RESP_BAD_REQUEST;
}

void webrtc_close_all_connections() {
    ;
}

#endif // ! HAVE_LIBDATACHANNEL
