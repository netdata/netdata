// SPDX-License-Identifier: GPL-3.0-or-later

#include "setup.h"
#include "handshake.h"

static const size_t HANDSHAKE_PROTOCOL_INITIAL_RESPONSE_SIZE = 1024;

static void rrdpush_set_flags_to_newest_stream(RRDHOST *host) {
    rrdhost_flag_set(host, RRDHOST_FLAG_STREAM_LABELS_UPDATE);
    rrdhost_flag_clear(host, RRDHOST_FLAG_STREAM_LABELS_STOP);
}

static long int parse_stream_version_for_errors(const char *http)
{
    if (!memcmp(http, START_STREAMING_ERROR_SAME_LOCALHOST, sizeof(START_STREAMING_ERROR_SAME_LOCALHOST)))
        return -2;
    else if (!memcmp(http, START_STREAMING_ERROR_ALREADY_STREAMING, sizeof(START_STREAMING_ERROR_ALREADY_STREAMING)))
        return -3;
    else if (!memcmp(http, START_STREAMING_ERROR_NOT_PERMITTED, sizeof(START_STREAMING_ERROR_NOT_PERMITTED)))
        return -4;
    else
        return -1;
}

static long int parse_stream_version(RRDHOST *host, const char *http)
{
    long int stream_version = -1;
    int answer = -1;
    char *stream_version_start = strchr(http, '=');
    if (stream_version_start) {
        stream_version_start++;
        stream_version = strtol(stream_version_start, NULL, 10);
        answer = memcmp(http, START_STREAMING_PROMPT_VN, (size_t)(stream_version_start - http));
        if (!answer) {
            rrdpush_set_flags_to_newest_stream(host);
        }
    } else {
        answer = memcmp(http, START_STREAMING_PROMPT_V2, strlen(START_STREAMING_PROMPT_V2));
        if (!answer) {
            stream_version = 1;
            rrdpush_set_flags_to_newest_stream(host);
        } else {
            answer = memcmp(http, START_STREAMING_PROMPT, strlen(START_STREAMING_PROMPT));
            if (!answer) {
                stream_version = 0;
                rrdhost_flag_set(host, RRDHOST_FLAG_STREAM_LABELS_STOP);
                rrdhost_flag_clear(host, RRDHOST_FLAG_STREAM_LABELS_UPDATE);
            }
            else {
                stream_version = parse_stream_version_for_errors(http);
            }
        }
    }
    return stream_version;
}

static inline struct netdata_ssl *sender_ssl(struct sender_state *ss) {
    return &ss->host->ssl;
}

static inline int sender_sockfd(struct sender_state *ss) {
    return ss->host->rrdpush_sender_socket;
}

static inline RRDHOST *receiver_host(struct receiver_state *rs) {
    return rs->host;
}

static inline struct netdata_ssl *receiver_ssl(struct receiver_state *rs) {
    return &rs->ssl;
}

static inline int receiver_sockfd(struct receiver_state *rs) {
    return rs->fd;
}

#define sender_error(sender, fmt, args...) do { \
    size_t len = 1024; \
    char newfmt[len]; \
    memset(newfmt, 0, len); \
    const char *connected_to = s->connected_to; \
    snprintfz(newfmt, len, " [send to %%s]: %s", fmt); \
    error(newfmt, connected_to, ##args); \
} while(0);

#define receiver_error(receiver, fmt, args...) do { \
    size_t len = 1024; \
    char newfmt[len]; \
    memset(newfmt, 0, len); \
    snprintfz(newfmt, len, " [recv from %%s (%%s:%%s)]: %s", fmt); \
    error(newfmt, receiver_host(receiver), receiver->client_ip, receiver->client_port, ##args); \
} while(0);

static size_t dummy_tcp_data(struct netdata_ssl *ssl, int sockfd, time_t timeout,
                           size_t processed, size_t expected, bool recv)
{
    if (processed > expected)
        return ~0;

    ssize_t remaining = expected - processed;
    if (!remaining)
        return 0;

    char *buf = callocz(sizeof(char), remaining);
    if (!buf)
        return remaining;

    if (recv)
        remaining = recv_exact(ssl, sockfd, buf, remaining, 0, timeout);
    else
        remaining = send_exact(ssl, sockfd, buf, remaining, 0, timeout);

    freez(buf);

    return remaining;
}

static bool drain_dummy_tcp_data(struct sender_state *ss, time_t timeout,
                                 size_t received, size_t expected)
{
    size_t n = dummy_tcp_data(sender_ssl(ss), sender_sockfd(ss),
                              timeout, received, expected, /* recv: */ true);
    if (n)
        error("Could not drain tcp data (recv'd=%zu, expected=%zu, remaining=%zu)",
              received, expected, n);
    return n == 0;
}

static bool fill_dummy_tcp_data(struct receiver_state *rs, time_t timeout,
                                size_t written, size_t expected)
{
    size_t n = dummy_tcp_data(receiver_ssl(rs), receiver_sockfd(rs),
                              timeout, written, expected, /* recv: */ false);
    if (n)
        error("Could not drain tcp data (sent=%zu, expected=%zu, remaining=%zu)",
              written, expected, n);
    return n == 0;
}

bool protocol_setup_on_sender(struct sender_state *s, int timeout) {
    RRDHOST *host = s->host;

#ifdef ENABLE_HANDSHAKE
    bool enable_handshake_protocol = localhost->system_info->handshake_enabled &&
                                     host->system_info->handshake_enabled;
#else
    bool enable_handshake_protocol = false;
#endif

    char http[HTTP_HEADER_SIZE + 1];
    ssize_t received;

    received = recv_timeout(sender_ssl(s), sender_sockfd(s), http, HTTP_HEADER_SIZE, 0, timeout);
    if(received == -1) {
        worker_is_busy(WORKER_SENDER_JOB_DISCONNECT_TIMEOUT);
        sender_error(s, "remote netdata does not respond");
        rrdpush_sender_thread_close_socket(host);
        return false;
    }

    http[received] = '\0';
    debug(D_STREAM, "Response to sender from far end: %s", http);

    if (enable_handshake_protocol && strstr(http, HANDSHAKE_PROTOCOL_PROMPT)) {
        bool ok = drain_dummy_tcp_data(s, timeout, received, HANDSHAKE_PROTOCOL_INITIAL_RESPONSE_SIZE);
        if (!ok) {
            worker_is_busy(WORKER_SENDER_JOB_DISCONNECT_TIMEOUT);
            sender_error(s, "handshake protocol initialization failed.");
            rrdpush_sender_thread_close_socket(host);
            return false;
        }

        ok = sender_handshake_start(s);
        if (!ok) {
            worker_is_busy(WORKER_SENDER_JOB_DISCONNECT_TIMEOUT);
            sender_error(s, "handshake protocol initialization failed.");
            rrdpush_sender_thread_close_socket(host);
            return false;
        }
    }

    int32_t version = (int32_t)parse_stream_version(host, http);
    if(version == -1) {
        worker_is_busy(WORKER_SENDER_JOB_DISCONNECT_BAD_HANDSHAKE);
        sender_error(s, "server is not replying properly (is it a netdata?).");
        rrdpush_sender_thread_close_socket(host);
        return false;
    }
    else if(version == -2) {
        sender_error(s, "remote server is localhost");
        rrdpush_sender_thread_close_socket(host);
        host->destination->disabled_because_of_localhost = 1;
        return false;
    }
    else if(version == -3) {
        sender_error(s, "remote server already receives metrics for host '%s'", rrdhost_hostname(host));
        rrdpush_sender_thread_close_socket(host);
        host->destination->disabled_already_streaming = now_realtime_sec();
        return false;
    }
    else if(version == -4) {
        sender_error(s, "remote server denied access for [%s].", rrdhost_hostname(host));
        rrdpush_sender_thread_close_socket(host);
        if (host->destination->next)
            host->destination->disabled_because_of_denied_access = 1;
        return false;
    }
    s->version = version;

    return true;
}

bool protocol_setup_on_receiver(struct receiver_state *rpt) {
#ifdef ENABLE_HANDSHAKE
    bool enable_handshake_protocol = localhost->system_info->handshake_enabled &&
                                     rpt->system_info->handshake_enabled;
#else
    bool enable_handshake_protocol = false;
#endif

    info("STREAM %s [receive from [%s]:%s]: initializing communication...", rrdhost_hostname(rpt->host), rpt->client_ip, rpt->client_port);

    char initial_response[HTTP_HEADER_SIZE];
    memset(initial_response, 0, HTTP_HEADER_SIZE);

    if (rpt->stream_version > 1) {
        if(rpt->stream_version >= STREAM_VERSION_COMPRESSION){
#ifdef ENABLE_COMPRESSION
            if(!rpt->rrdpush_compression)
                rpt->stream_version = STREAM_VERSION_CLABELS;
#else
            if(STREAMING_PROTOCOL_CURRENT_VERSION < rpt->stream_version)
                rpt->stream_version =  STREAMING_PROTOCOL_CURRENT_VERSION;
#endif
        }
        info("STREAM %s [receive from [%s]:%s]: Netdata is using the stream version %u.", rrdhost_hostname(rpt->host), rpt->client_ip, rpt->client_port, rpt->stream_version);
        sprintf(initial_response, "%s%u", START_STREAMING_PROMPT_VN, rpt->stream_version);
    } else if (rpt->stream_version == 1) {
        info("STREAM %s [receive from [%s]:%s]: Netdata is using the stream version %u.", rrdhost_hostname(rpt->host), rpt->client_ip, rpt->client_port, rpt->stream_version);
        sprintf(initial_response, "%s", START_STREAMING_PROMPT_V2);
    } else {
        info("STREAM %s [receive from [%s]:%s]: Netdata is using first stream protocol.", rrdhost_hostname(rpt->host), rpt->client_ip, rpt->client_port);
        sprintf(initial_response, "%s", START_STREAMING_PROMPT);
    }

    if (enable_handshake_protocol) {
        char handshake_response[HTTP_HEADER_SIZE];
        memset(handshake_response, 0, HTTP_HEADER_SIZE);

        sprintf(handshake_response, "%s&%s", initial_response, HANDSHAKE_PROTOCOL_PROMPT);
        memcpy(initial_response, handshake_response, HTTP_HEADER_SIZE);
    }

    debug(D_STREAM, "Initial response to %s: %s", rpt->client_ip, initial_response);

#ifdef ENABLE_HTTPS
    rpt->host->stream_ssl.conn = rpt->ssl.conn;
    rpt->host->stream_ssl.flags = rpt->ssl.flags;
#endif

    time_t timeout = 60;
    ssize_t sent = strlen(initial_response);

    if (send_timeout(&rpt->ssl, rpt->fd, initial_response, strlen(initial_response), 0, timeout) != sent) {
        log_stream_connection(rpt->client_ip, rpt->client_port, rpt->key, rpt->host->machine_guid, rrdhost_hostname(rpt->host), "FAILED - CANNOT REPLY");
        error("STREAM %s [receive from [%s]:%s]: cannot send ready command.", rrdhost_hostname(rpt->host), rpt->client_ip, rpt->client_port);
        close(rpt->fd);
        return false;
    }
    receiver_error(rpt, "sent initial response");

    if (enable_handshake_protocol) {
        bool ok = fill_dummy_tcp_data(rpt, timeout, sent, HANDSHAKE_PROTOCOL_INITIAL_RESPONSE_SIZE);
        if (!ok) {
            fatal("Could not fill all data");
            return false;
        }

        return receiver_handshake_start(rpt);
    }

    return true;
}
