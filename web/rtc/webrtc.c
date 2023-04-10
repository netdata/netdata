// SPDX-License-Identifier: GPL-3.0-or-later

#include "webrtc.h"

#include "../server/web_client.h"

#ifdef HAVE_LIBDATACHANNEL

#include "rtc/rtc.h"

static void webrtc_log(rtcLogLevel level, const char *message) {
    switch(level) {
        case RTC_LOG_NONE:
            break;

        case RTC_LOG_WARNING:
        case RTC_LOG_ERROR:
        case RTC_LOG_FATAL:
            error("WEBRTC: %s", message);
            break;

        case RTC_LOG_INFO:
            info("WEBRTC: %s", message);
            break;

        default:
        case RTC_LOG_DEBUG:
        case RTC_LOG_VERBOSE:
            internal_error(true, "WEBRTC: %s", message);
            break;

    }
}

void webrtc_initialize() {
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
    rtcCleanup();
}

typedef struct webrtc_datachannel {
    int dc;
    char *label;
    struct webrtc_connection *conn;
    bool open;

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

static struct {
    struct {
        SPINLOCK spinlock;
        WEBRTC_CONN *head;
    } unsafe;

} webrtc_base = {
        .unsafe = {
                .spinlock = NETDATA_SPINLOCK_INITIALIZER,
                .head = NULL,
        },
};

// ----------------------------------------------------------------------------
// webrtc data channel

static void myOpenCallback(int id, void *user_ptr) {
    WEBRTC_DC *chan = user_ptr;
    internal_fatal(chan->dc != id, "WEBRTC[%d],DC[%d]: dc mismatch, expected %d, got %d", chan->conn->pc, chan->dc, chan->dc, id);

    internal_error(true, "WEBRTC[%d],DC[%d]: data channel opened.", chan->conn->pc, chan->dc);
    chan->open = true;
}

static void myClosedCallback(int id, void *user_ptr) {
    WEBRTC_DC *chan = user_ptr;
    internal_fatal(chan->dc != id, "WEBRTC[%d],DC[%d]: dc mismatch, expected %d, got %d", chan->conn->pc, chan->dc, chan->dc, id);

    chan->open = false;
    internal_error(true, "WEBRTC[%d],DC[%d]: data channel closed.", chan->conn->pc, chan->dc);

    netdata_spinlock_lock(&chan->conn->channels.spinlock);
    DOUBLE_LINKED_LIST_REMOVE_ITEM_UNSAFE(chan->conn->channels.head, chan, link.prev, link.next);
    netdata_spinlock_unlock(&chan->conn->channels.spinlock);

    freez(chan->label);
    freez(chan);
}

static void myErrorCallback(int id, const char *error, void *user_ptr) {
    WEBRTC_DC *chan = user_ptr;
    internal_fatal(chan->dc != id, "WEBRTC[%d],DC[%d]: dc mismatch, expected %d, got %d", chan->conn->pc, chan->dc, chan->dc, id);

    error("WEBRTC[%d],DC[%d]: ERROR: '%s'", chan->conn->pc, chan->dc, error);
}

static void myMessageCallback(int id, const char *message, int size, void *user_ptr) {
    WEBRTC_DC *chan = user_ptr;
    internal_fatal(chan->dc != id, "WEBRTC[%d],DC[%d]: dc mismatch, expected %d, got %d", chan->conn->pc, chan->dc, chan->dc, id);

    internal_fatal(!chan->open, "WEBRTC[%d],DC[%d]: received message on closed channel", chan->conn->pc, chan->dc);

    bool binary = (size >= 0);
    if(size < 0)
        size = -size;

    BUFFER *wb = buffer_create(size + 1, NULL);
    buffer_strncat(wb, message, size);

    info("WEBRTC[%d],DC[%d]: received %s message of length %d: '%s'", chan->conn->pc, chan->dc, binary ? "binary" : "text", size, buffer_tostring(wb));

    buffer_strcat(wb, " to you too!");
    rtcSendMessage(id, buffer_tostring(wb), -(int)buffer_strlen(wb));

    info("WEBRTC[%d],DC[%d]: sent message of length %d: '%s'", chan->conn->pc, chan->dc, (int)buffer_strlen(wb), buffer_tostring(wb));
    buffer_free(wb);
}

//#define WEBRTC_MAX_REQUEST_SIZE 65536
//
//static void myAvailableCallback(int id, void *user_ptr) {
//    WEBRTC_DC *chan = user_ptr;
//    internal_fatal(chan->dc != id, "WEBRTC[%d],DC[%d]: dc mismatch, expected %d, got %d", chan->conn->pc, chan->dc, chan->dc, id);
//
//    internal_fatal(!chan->open, "WEBRTC[%d],DC[%d]: received message on closed channel", chan->conn->pc, chan->dc);
//
//    int size = WEBRTC_MAX_REQUEST_SIZE;
//    char buffer[WEBRTC_MAX_REQUEST_SIZE];
//    while(rtcReceiveMessage(id, buffer, &size) == RTC_ERR_SUCCESS) {
//        if(size < 0)
//            size = -size;
//
//        BUFFER *wb = buffer_create(size, NULL);
//        buffer_strncat(wb, buffer, size);
//
//        info("WEBRTC[%d],DC[%d]: received message of length %d: '%s'", chan->conn->pc, chan->dc, size, buffer_tostring(wb));
//
//        buffer_strcat(wb, " to you too!");
//        rtcSendMessage(id, buffer_tostring(wb), buffer_strlen(wb));
//
//        buffer_free(wb);
//    }
//}

static void myDataChannelCallback(int pc, int dc, void *user_ptr) {
    WEBRTC_CONN *conn = user_ptr;
    internal_fatal(conn->pc != pc, "WEBRTC[%d]: pc mismatch, expected %d, got %d", conn->pc, conn->pc, pc);

    WEBRTC_DC *chan = callocz(1, sizeof(WEBRTC_DC));
    chan->dc = dc;
    chan->conn = conn;

    netdata_spinlock_lock(&conn->channels.spinlock);
    DOUBLE_LINKED_LIST_APPEND_ITEM_UNSAFE(conn->channels.head, chan, link.prev, link.next);
    netdata_spinlock_unlock(&conn->channels.spinlock);

    rtcSetUserPointer(dc, chan);

    char label[1024 + 1];
    rtcGetDataChannelLabel(dc, label, 1024);
    label[1024] = '\0';

    chan->label = strdupz(label);

    if(rtcSetOpenCallback(dc, myOpenCallback) != RTC_ERR_SUCCESS)
        error("WEBRTC[%d],DC[%d]: rtcSetOpenCallback() failed.", conn->pc, chan->dc);

    if(rtcSetClosedCallback(dc, myClosedCallback) != RTC_ERR_SUCCESS)
        error("WEBRTC[%d],DC[%d]: rtcSetClosedCallback() failed.", conn->pc, chan->dc);

    if(rtcSetErrorCallback(dc, myErrorCallback) != RTC_ERR_SUCCESS)
        error("WEBRTC[%d],DC[%d]: rtcSetErrorCallback() failed.", conn->pc, chan->dc);

    if(rtcSetMessageCallback(dc, myMessageCallback) != RTC_ERR_SUCCESS)
        error("WEBRTC[%d],DC[%d]: rtcSetMessageCallback() failed.", conn->pc, chan->dc);

//    if(rtcSetAvailableCallback(dc, myAvailableCallback) != RTC_ERR_SUCCESS)
//        error("WEBRTC[%d],DC[%d]: rtcSetAvailableCallback() failed.", conn->pc, chan->dc);

    internal_error(true, "WEBRTC[%d],DC[%d]: new data channel with label '%s'", chan->conn->pc, chan->dc, chan->label);
}

// ----------------------------------------------------------------------------
// webrtc connection

static void webrtc_destroy_connection_unsafe(WEBRTC_CONN *conn) {
    if(conn->state == RTC_CLOSED) {
        size_t channels = 0;

        netdata_spinlock_lock(&conn->channels.spinlock);
        WEBRTC_DC *chan = conn->channels.head;
        while (chan) {
            WEBRTC_DC *chan_next = chan->link.next;

            // do not close channels here
            // they will dead-lock on conn->channels.spinlock

            channels++;
            chan = chan_next;
        }
        netdata_spinlock_unlock(&conn->channels.spinlock);

        if(!channels) {
            internal_error(true, "WEBRTC[%d]: destroying connection", conn->pc);
            DOUBLE_LINKED_LIST_REMOVE_ITEM_UNSAFE(webrtc_base.unsafe.head, conn, link.prev, link.next);
            freez(conn->config.iceServers);
            freez(conn);
        }
        else {
            internal_error(true, "WEBRTC[%d]: not destroying closed connection because it has %zu channels running", conn->pc, channels);
        }
    }
}

static void cleanupConnections() {
    netdata_spinlock_lock(&webrtc_base.unsafe.spinlock);
    WEBRTC_CONN *conn = webrtc_base.unsafe.head;
    while(conn) {
        WEBRTC_CONN *conn_next = conn->link.next;
        webrtc_destroy_connection_unsafe(conn);
        conn = conn_next;
    }
    netdata_spinlock_unlock(&webrtc_base.unsafe.spinlock);
}

static WEBRTC_CONN *webrtc_create_connection(int iceServersCount) {
    WEBRTC_CONN *conn = callocz(1, sizeof(WEBRTC_CONN));

    if(iceServersCount)
        conn->config.iceServers = callocz(iceServersCount, sizeof(char *));

    netdata_spinlock_init(&conn->response.spinlock);
    netdata_spinlock_init(&conn->channels.spinlock);

    netdata_spinlock_lock(&webrtc_base.unsafe.spinlock);
    DOUBLE_LINKED_LIST_APPEND_ITEM_UNSAFE(webrtc_base.unsafe.head, conn, link.prev, link.next);
    netdata_spinlock_unlock(&webrtc_base.unsafe.spinlock);
    return conn;
}

static void myDescriptionCallback(int pc __maybe_unused, const char *sdp, const char *type, void *user_ptr) {
    WEBRTC_CONN *conn = user_ptr;
    internal_fatal(conn->pc != pc, "WEBRTC[%d]: pc mismatch, expected %d, got %d", conn->pc, conn->pc, pc);

    internal_error(true, "WEBRTC[%d]: local description type '%s': %s", conn->pc, type, sdp);
    netdata_spinlock_lock(&conn->response.spinlock);
    if(!conn->response.candidates) {
        buffer_json_member_add_string(conn->response.wb, "sdp", sdp);
        buffer_json_member_add_string(conn->response.wb, "type", type);
        conn->response.sdp = true;
    }
    netdata_spinlock_unlock(&conn->response.spinlock);
}

static void myCandidateCallback(int pc __maybe_unused, const char *cand, const char *mid __maybe_unused, void *user_ptr) {
    WEBRTC_CONN *conn = user_ptr;
    internal_fatal(conn->pc != pc, "WEBRTC[%d]: pc mismatch, expected %d, got %d", conn->pc, conn->pc, pc);

    netdata_spinlock_lock(&conn->response.spinlock);
    if(!conn->response.candidates) {
        buffer_json_member_add_array(conn->response.wb, "candidates");
        conn->response.candidates = true;
    }

    internal_error(true, "WEBRTC[%d]: local candidate '%s', mid '%s'", conn->pc, cand, mid);
    buffer_json_add_array_item_string(conn->response.wb, cand);
    netdata_spinlock_unlock(&conn->response.spinlock);
}

static void myStateChangeCallback(int pc __maybe_unused, rtcState state, void *user_ptr) {
    WEBRTC_CONN *conn = user_ptr;
    internal_fatal(conn->pc != pc, "WEBRTC[%d]: pc mismatch, expected %d, got %d", conn->pc, conn->pc, pc);

    conn->state = state;

    switch(state) {
        case RTC_NEW:
            internal_error(true, "WEBRTC[%d]: new connection...", conn->pc);
            break;

        case RTC_CONNECTING:
            internal_error(true, "WEBRTC[%d]: connecting...", conn->pc);
            break;

        case RTC_CONNECTED:
            internal_error(true, "WEBRTC[%d]: connected!", conn->pc);
            break;

        case RTC_DISCONNECTED:
            internal_error(true, "WEBRTC[%d]: disconnected.", conn->pc);
            break;

        case RTC_FAILED:
            internal_error(true, "WEBRTC[%d]: failed.", conn->pc);
            break;

        case RTC_CLOSED:
            internal_error(true, "WEBRTC[%d]: closed.", conn->pc);
            netdata_spinlock_lock(&webrtc_base.unsafe.spinlock);
            webrtc_destroy_connection_unsafe(conn);
            netdata_spinlock_unlock(&webrtc_base.unsafe.spinlock);
            break;
    }
}

static void myGatheringStateCallback(int pc __maybe_unused, rtcGatheringState state, void *user_ptr) {
    WEBRTC_CONN *conn = user_ptr;
    internal_fatal(conn->pc != pc, "WEBRTC[%d]: pc mismatch, expected %d, got %d", conn->pc, conn->pc, pc);

    conn->gathering_state = state;

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
    cleanupConnections();

    buffer_flush(wb);
    buffer_json_initialize(wb, "\"", "\"", 0, true, false);
    wb->content_type = CT_APPLICATION_JSON;

    if(!sdp || !*sdp) {
        buffer_strcat(wb, "No SDP message posted with the request");
        wb->content_type = CT_TEXT_PLAIN;
        return HTTP_RESP_BAD_REQUEST;
    }

    int iceServersCount = 1;
    WEBRTC_CONN *conn = webrtc_create_connection(iceServersCount);
    conn->response.wb = wb;

    // Format:
    // [("stun"|"turn"|"turns") (":"|"://")][username ":" password "@"]hostname[":" port]["?transport=" ("udp"|"tcp"|"tls")]
    //
    // Note transports TCP and TLS are only available for a TURN server with libnice as ICE backend and govern only the
    // TURN control connection, meaning relaying is always performed over UDP.
    //
    // If the username or password of an URI contains reserved special characters, they must be percent-encoded.
    // In particular, ":" must be encoded as "%3A" and "@" must by encoded as "%40".
    if(iceServersCount)
        conn->config.iceServers[iceServersCount - 1] = "stun://stun.l.google.com:19302";

    conn->config.iceServersCount = iceServersCount;

    conn->config.proxyServer = NULL; // [("http"|"socks5") (":"|"://")][username ":" password "@"]hostname["    :" port]
    conn->config.bindAddress = NULL;
    conn->config.certificateType = RTC_CERTIFICATE_DEFAULT;
    conn->config.iceTransportPolicy = RTC_TRANSPORT_POLICY_ALL;
    conn->config.enableIceTcp = true; // libnice only
    conn->config.enableIceUdpMux = true; // libjuice only
    conn->config.disableAutoNegotiation = false;
    conn->config.forceMediaTransport = false;
    conn->config.portRangeBegin = 0; // 0 means automatic
    conn->config.portRangeEnd = 0; // 0 means automatic
    conn->config.mtu = 0; // <= 0 means automatic
    conn->config.maxMessageSize = 5 * 1024 * 1024; // <= 0 means default

    conn->pc = rtcCreatePeerConnection(&conn->config);
    rtcSetUserPointer(conn->pc, conn);

    if(rtcSetLocalDescriptionCallback(conn->pc, myDescriptionCallback) != RTC_ERR_SUCCESS)
        error("WEBRTC[%d]: rtcSetLocalDescriptionCallback() failed", conn->pc);

    if(rtcSetLocalCandidateCallback(conn->pc, myCandidateCallback) != RTC_ERR_SUCCESS)
        error("WEBRTC[%d]: rtcSetLocalCandidateCallback() failed", conn->pc);

    if(rtcSetStateChangeCallback(conn->pc, myStateChangeCallback) != RTC_ERR_SUCCESS)
        error("WEBRTC[%d]: rtcSetStateChangeCallback() failed", conn->pc);

    if(rtcSetGatheringStateChangeCallback(conn->pc, myGatheringStateCallback) != RTC_ERR_SUCCESS)
        error("WEBRTC[%d]: rtcSetGatheringStateChangeCallback() failed", conn->pc);

    if(rtcSetDataChannelCallback(conn->pc, myDataChannelCallback) != RTC_ERR_SUCCESS)
        error("WEBRTC[%d]: rtcSetDataChannelCallback() failed", conn->pc);

    // initialize the handshake
    internal_error(true, "WEBRTC[%d]: setting remote sdp: %s", conn->pc, sdp);
    if(rtcSetRemoteDescription(conn->pc, sdp, "offer") != RTC_ERR_SUCCESS)
        error("WEBRTC[%d]: rtcSetRemoteDescription() failed", conn->pc);

    // initiate the handshake process
    if(conn->config.disableAutoNegotiation) {
        if(rtcSetLocalDescription(conn->pc, NULL) != RTC_ERR_SUCCESS)
            error("WEBRTC[%d]: rtcSetLocalDescription() failed", conn->pc);
    }

    bool logged = false;
    while(conn->gathering_state != RTC_GATHERING_COMPLETE) {
        if(!logged) {
            logged = true;
            internal_error(true, "WEBRTC[%d]: Waiting for gathering to complete", conn->pc);
        }
        usleep(1000);
    }

    if(logged)
        internal_error(true, "WEBRTC[%d]: Gathering finished, our answer is ready", conn->pc);

    internal_fatal(!conn->response.sdp, "WEBRTC[%d]: response does not have an SDP: %s", conn->pc, buffer_tostring(conn->response.wb));
    internal_fatal(!conn->response.candidates, "WEBRTC[%d]: response does not have candidates: %s", conn->pc, buffer_tostring(conn->response.wb));

    buffer_json_finalize(wb);

    return HTTP_RESP_OK;
}

#else // ! HAVE_LIBDATACHANNEL

void webrtc_initialize() {
    ;
}

int webrtc_new_connection(const char *sdp __maybe_unused, BUFFER *wb) {
    buffer_flush(wb);
    buffer_strcat(wb, "WEBRTC is not enabled on this server");
    wb->content_type = CT_TEXT_PLAIN;
    return HTTP_RESP_BAD_REQUEST;
}

void webrtc_close_all_connections() {
    ;
}

#endif // ! HAVE_LIBDATACHANNEL
