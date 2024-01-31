// SPDX-License-Identifier: GPL-3.0-or-later

#include "rrdpush.h"
#include "common.h"
#include "aclk/https_client.h"

#define WORKER_SENDER_JOB_CONNECT                                0
#define WORKER_SENDER_JOB_PIPE_READ                              1
#define WORKER_SENDER_JOB_SOCKET_RECEIVE                         2
#define WORKER_SENDER_JOB_EXECUTE                                3
#define WORKER_SENDER_JOB_SOCKET_SEND                            4
#define WORKER_SENDER_JOB_DISCONNECT_BAD_HANDSHAKE               5
#define WORKER_SENDER_JOB_DISCONNECT_OVERFLOW                    6
#define WORKER_SENDER_JOB_DISCONNECT_TIMEOUT                     7
#define WORKER_SENDER_JOB_DISCONNECT_POLL_ERROR                  8
#define WORKER_SENDER_JOB_DISCONNECT_SOCKET_ERROR                9
#define WORKER_SENDER_JOB_DISCONNECT_SSL_ERROR                  10
#define WORKER_SENDER_JOB_DISCONNECT_PARENT_CLOSED              11
#define WORKER_SENDER_JOB_DISCONNECT_RECEIVE_ERROR              12
#define WORKER_SENDER_JOB_DISCONNECT_SEND_ERROR                 13
#define WORKER_SENDER_JOB_DISCONNECT_NO_COMPRESSION             14
#define WORKER_SENDER_JOB_BUFFER_RATIO                          15
#define WORKER_SENDER_JOB_BYTES_RECEIVED                        16
#define WORKER_SENDER_JOB_BYTES_SENT                            17
#define WORKER_SENDER_JOB_BYTES_COMPRESSED                      18
#define WORKER_SENDER_JOB_BYTES_UNCOMPRESSED                    19
#define WORKER_SENDER_JOB_BYTES_COMPRESSION_RATIO               20
#define WORKER_SENDER_JOB_REPLAY_REQUEST                        21
#define WORKER_SENDER_JOB_FUNCTION_REQUEST                      22
#define WORKER_SENDER_JOB_REPLAY_DICT_SIZE                      23
#define WORKER_SENDER_JOB_DISCONNECT_CANT_UPGRADE_CONNECTION    24

#if WORKER_UTILIZATION_MAX_JOB_TYPES < 25
#error WORKER_UTILIZATION_MAX_JOB_TYPES has to be at least 25
#endif

extern struct config stream_config;
extern char *netdata_ssl_ca_path;
extern char *netdata_ssl_ca_file;

static __thread BUFFER *sender_thread_buffer = NULL;
static __thread bool sender_thread_buffer_used = false;
static __thread time_t sender_thread_buffer_last_reset_s = 0;

void sender_thread_buffer_free(void) {
    buffer_free(sender_thread_buffer);
    sender_thread_buffer = NULL;
    sender_thread_buffer_used = false;
}

// Collector thread starting a transmission
BUFFER *sender_start(struct sender_state *s) {
    if(unlikely(sender_thread_buffer_used))
        fatal("STREAMING: thread buffer is used multiple times concurrently.");

    if(unlikely(rrdpush_sender_last_buffer_recreate_get(s) > sender_thread_buffer_last_reset_s)) {
        if(unlikely(sender_thread_buffer && sender_thread_buffer->size > THREAD_BUFFER_INITIAL_SIZE)) {
            buffer_free(sender_thread_buffer);
            sender_thread_buffer = NULL;
        }
    }

    if(unlikely(!sender_thread_buffer)) {
        sender_thread_buffer = buffer_create(THREAD_BUFFER_INITIAL_SIZE, &netdata_buffers_statistics.buffers_streaming);
        sender_thread_buffer_last_reset_s = rrdpush_sender_last_buffer_recreate_get(s);
    }

    sender_thread_buffer_used = true;
    buffer_flush(sender_thread_buffer);
    return sender_thread_buffer;
}

static inline void rrdpush_sender_thread_close_socket(RRDHOST *host);

#define SENDER_BUFFER_ADAPT_TO_TIMES_MAX_SIZE 3

// Collector thread finishing a transmission
void sender_commit(struct sender_state *s, BUFFER *wb, STREAM_TRAFFIC_TYPE type) {

    if(unlikely(wb != sender_thread_buffer))
        fatal("STREAMING: sender is trying to commit a buffer that is not this thread's buffer.");

    if(unlikely(!sender_thread_buffer_used))
        fatal("STREAMING: sender is committing a buffer twice.");

    sender_thread_buffer_used = false;

    char *src = (char *)buffer_tostring(wb);
    size_t src_len = buffer_strlen(wb);

    if(unlikely(!src || !src_len))
        return;

    sender_lock(s);

#ifdef NETDATA_LOG_STREAM_SENDER
    if(type == STREAM_TRAFFIC_TYPE_METADATA) {
        if(!s->stream_log_fp) {
            char filename[FILENAME_MAX + 1];
            snprintfz(filename, FILENAME_MAX, "/tmp/stream-sender-%s.txt", s->host ? rrdhost_hostname(s->host) : "unknown");

            s->stream_log_fp = fopen(filename, "w");
        }

        fprintf(s->stream_log_fp, "\n--- SEND MESSAGE START: %s ----\n"
                    "%s"
                    "--- SEND MESSAGE END ----------------------------------------\n"
                , rrdhost_hostname(s->host), src
               );
    }
#endif

    if(unlikely(s->buffer->max_size < (src_len + 1) * SENDER_BUFFER_ADAPT_TO_TIMES_MAX_SIZE)) {
        netdata_log_info("STREAM %s [send to %s]: max buffer size of %zu is too small for a data message of size %zu. Increasing the max buffer size to %d times the max data message size.",
              rrdhost_hostname(s->host), s->connected_to, s->buffer->max_size, buffer_strlen(wb) + 1, SENDER_BUFFER_ADAPT_TO_TIMES_MAX_SIZE);

        s->buffer->max_size = (src_len + 1) * SENDER_BUFFER_ADAPT_TO_TIMES_MAX_SIZE;
    }

    if (s->compressor.initialized) {
        while(src_len) {
            size_t size_to_compress = src_len;

            if(unlikely(size_to_compress > COMPRESSION_MAX_MSG_SIZE)) {
                if (stream_has_capability(s, STREAM_CAP_BINARY))
                    size_to_compress = COMPRESSION_MAX_MSG_SIZE;
                else {
                    if (size_to_compress > COMPRESSION_MAX_MSG_SIZE) {
                        // we need to find the last newline
                        // so that the decompressor will have a whole line to work with

                        const char *t = &src[COMPRESSION_MAX_MSG_SIZE];
                        while (--t >= src)
                            if (unlikely(*t == '\n'))
                                break;

                        if (t <= src) {
                            size_to_compress = COMPRESSION_MAX_MSG_SIZE;
                        } else
                            size_to_compress = t - src + 1;
                    }
                }
            }

            const char *dst;
            size_t dst_len = rrdpush_compress(&s->compressor, src, size_to_compress, &dst);
            if (!dst_len) {
                netdata_log_error("STREAM %s [send to %s]: COMPRESSION failed. Resetting compressor and re-trying",
                      rrdhost_hostname(s->host), s->connected_to);

                rrdpush_compression_initialize(s);
                dst_len = rrdpush_compress(&s->compressor, src, size_to_compress, &dst);
                if(!dst_len) {
                    netdata_log_error("STREAM %s [send to %s]: COMPRESSION failed again. Deactivating compression",
                          rrdhost_hostname(s->host), s->connected_to);

                    worker_is_busy(WORKER_SENDER_JOB_DISCONNECT_NO_COMPRESSION);
                    rrdpush_compression_deactivate(s);
                    rrdpush_sender_thread_close_socket(s->host);
                    sender_unlock(s);
                    return;
                }
            }

            rrdpush_signature_t signature = rrdpush_compress_encode_signature(dst_len);

#ifdef NETDATA_INTERNAL_CHECKS
            // check if reversing the signature provides the same length
            size_t decoded_dst_len = rrdpush_decompress_decode_signature((const char *)&signature, sizeof(signature));
            if(decoded_dst_len != dst_len)
                fatal("RRDPUSH COMPRESSION: invalid signature, original payload %zu bytes, "
                      "compressed payload length %zu bytes, but signature says payload is %zu bytes",
                      size_to_compress, dst_len, decoded_dst_len);
#endif

            if(cbuffer_add_unsafe(s->buffer, (const char *)&signature, sizeof(signature)))
                s->flags |= SENDER_FLAG_OVERFLOW;
            else {
                if(cbuffer_add_unsafe(s->buffer, dst, dst_len))
                    s->flags |= SENDER_FLAG_OVERFLOW;
                else
                    s->sent_bytes_on_this_connection_per_type[type] += dst_len + sizeof(signature);
            }

            src = src + size_to_compress;
            src_len -= size_to_compress;
        }
    }
    else if(cbuffer_add_unsafe(s->buffer, src, src_len))
        s->flags |= SENDER_FLAG_OVERFLOW;
    else
        s->sent_bytes_on_this_connection_per_type[type] += src_len;

    replication_recalculate_buffer_used_ratio_unsafe(s);

    bool signal_sender = false;
    if(!rrdpush_sender_pipe_has_pending_data(s)) {
        rrdpush_sender_pipe_set_pending_data(s);
        signal_sender = true;
    }

    sender_unlock(s);

    if(signal_sender && (!stream_has_capability(s, STREAM_CAP_INTERPOLATED) || type != STREAM_TRAFFIC_TYPE_DATA))
        rrdpush_signal_sender_to_wake_up(s);
}

static inline void rrdpush_sender_add_host_variable_to_buffer(BUFFER *wb, const RRDVAR_ACQUIRED *rva) {
    buffer_sprintf(
            wb
            , "VARIABLE HOST %s = " NETDATA_DOUBLE_FORMAT "\n"
            , rrdvar_name(rva)
            , rrdvar2number(rva)
    );

    netdata_log_debug(D_STREAM, "RRDVAR pushed HOST VARIABLE %s = " NETDATA_DOUBLE_FORMAT, rrdvar_name(rva), rrdvar2number(rva));
}

void rrdpush_sender_send_this_host_variable_now(RRDHOST *host, const RRDVAR_ACQUIRED *rva) {
    if(rrdhost_can_send_definitions_to_parent(host)) {
        BUFFER *wb = sender_start(host->sender);
        rrdpush_sender_add_host_variable_to_buffer(wb, rva);
        sender_commit(host->sender, wb, STREAM_TRAFFIC_TYPE_METADATA);
        sender_thread_buffer_free();
    }
}

struct custom_host_variables_callback {
    BUFFER *wb;
};

static int rrdpush_sender_thread_custom_host_variables_callback(const DICTIONARY_ITEM *item __maybe_unused, void *rrdvar_ptr __maybe_unused, void *struct_ptr) {
    const RRDVAR_ACQUIRED *rv = (const RRDVAR_ACQUIRED *)item;
    struct custom_host_variables_callback *tmp = struct_ptr;
    BUFFER *wb = tmp->wb;

    rrdpush_sender_add_host_variable_to_buffer(wb, rv);
    return 1;
}

static void rrdpush_sender_thread_send_custom_host_variables(RRDHOST *host) {
    if(rrdhost_can_send_definitions_to_parent(host)) {
        BUFFER *wb = sender_start(host->sender);
        struct custom_host_variables_callback tmp = {
            .wb = wb
        };
        int ret = rrdvar_walkthrough_read(host->rrdvars, rrdpush_sender_thread_custom_host_variables_callback, &tmp);
        (void)ret;
        sender_commit(host->sender, wb, STREAM_TRAFFIC_TYPE_METADATA);
        sender_thread_buffer_free();

        netdata_log_debug(D_STREAM, "RRDVAR sent %d VARIABLES", ret);
    }
}

// resets all the chart, so that their definitions
// will be resent to the central netdata
static void rrdpush_sender_thread_reset_all_charts(RRDHOST *host) {
    RRDSET *st;
    rrdset_foreach_read(st, host) {
        rrdset_flag_clear(st, RRDSET_FLAG_SENDER_REPLICATION_IN_PROGRESS);
        rrdset_flag_set(st, RRDSET_FLAG_SENDER_REPLICATION_FINISHED);

        st->rrdpush.sender.resync_time_s = 0;

        RRDDIM *rd;
        rrddim_foreach_read(rd, st)
            rrddim_metadata_exposed_upstream_clear(rd);
        rrddim_foreach_done(rd);

        rrdset_metadata_updated(st);
    }
    rrdset_foreach_done(st);

    rrdhost_sender_replicating_charts_zero(host);
}

static void rrdpush_sender_cbuffer_recreate_timed(struct sender_state *s, time_t now_s, bool have_mutex, bool force) {
    static __thread time_t last_reset_time_s = 0;

    if(!force && now_s - last_reset_time_s < 300)
        return;

    if(!have_mutex)
        sender_lock(s);

    rrdpush_sender_last_buffer_recreate_set(s, now_s);
    last_reset_time_s = now_s;

    if(s->buffer && s->buffer->size > CBUFFER_INITIAL_SIZE) {
        size_t max = s->buffer->max_size;
        cbuffer_free(s->buffer);
        s->buffer = cbuffer_new(CBUFFER_INITIAL_SIZE, max, &netdata_buffers_statistics.cbuffers_streaming);
    }

    sender_thread_buffer_free();

    if(!have_mutex)
        sender_unlock(s);
}

static void rrdpush_sender_cbuffer_flush(RRDHOST *host) {
    rrdpush_sender_set_flush_time(host->sender);

    sender_lock(host->sender);

    // flush the output buffer from any data it may have
    cbuffer_flush(host->sender->buffer);
    rrdpush_sender_cbuffer_recreate_timed(host->sender, now_monotonic_sec(), true, true);
    replication_recalculate_buffer_used_ratio_unsafe(host->sender);

    sender_unlock(host->sender);
}

static void rrdpush_sender_charts_and_replication_reset(RRDHOST *host) {
    rrdpush_sender_set_flush_time(host->sender);

    // stop all replication commands inflight
    replication_sender_delete_pending_requests(host->sender);

    // reset the state of all charts
    rrdpush_sender_thread_reset_all_charts(host);

    rrdpush_sender_replicating_charts_zero(host->sender);
}

static void rrdpush_sender_on_connect(RRDHOST *host) {
    rrdpush_sender_cbuffer_flush(host);
    rrdpush_sender_charts_and_replication_reset(host);
}

static void rrdpush_sender_after_connect(RRDHOST *host) {
    rrdpush_sender_thread_send_custom_host_variables(host);
}

static inline void rrdpush_sender_thread_close_socket(RRDHOST *host) {
#ifdef ENABLE_HTTPS
    netdata_ssl_close(&host->sender->ssl);
#endif

    if(host->sender->rrdpush_sender_socket != -1) {
        close(host->sender->rrdpush_sender_socket);
        host->sender->rrdpush_sender_socket = -1;
    }

    rrdhost_flag_clear(host, RRDHOST_FLAG_RRDPUSH_SENDER_READY_4_METRICS);
    rrdhost_flag_clear(host, RRDHOST_FLAG_RRDPUSH_SENDER_CONNECTED);

    // do not flush the circular buffer here
    // this function is called sometimes with the mutex lock, sometimes without the lock
    rrdpush_sender_charts_and_replication_reset(host);
}

void rrdpush_encode_variable(stream_encoded_t *se, RRDHOST *host) {
    se->os_name = (host->system_info->host_os_name)?url_encode(host->system_info->host_os_name):strdupz("");
    se->os_id = (host->system_info->host_os_id)?url_encode(host->system_info->host_os_id):strdupz("");
    se->os_version = (host->system_info->host_os_version)?url_encode(host->system_info->host_os_version):strdupz("");
    se->kernel_name = (host->system_info->kernel_name)?url_encode(host->system_info->kernel_name):strdupz("");
    se->kernel_version = (host->system_info->kernel_version)?url_encode(host->system_info->kernel_version):strdupz("");
}

void rrdpush_clean_encoded(stream_encoded_t *se) {
    if (se->os_name) {
        freez(se->os_name);
        se->os_name = NULL;
    }

    if (se->os_id) {
        freez(se->os_id);
        se->os_id = NULL;
    }

    if (se->os_version) {
        freez(se->os_version);
        se->os_version = NULL;
    }

    if (se->kernel_name) {
        freez(se->kernel_name);
        se->kernel_name = NULL;
    }

    if (se->kernel_version) {
        freez(se->kernel_version);
        se->kernel_version = NULL;
    }
}

struct {
    const char *response;
    const char *status;
    size_t length;
    int32_t version;
    bool dynamic;
    const char *error;
    int worker_job_id;
    int postpone_reconnect_seconds;
    ND_LOG_FIELD_PRIORITY priority;
} stream_responses[] = {
    {
        .response = START_STREAMING_PROMPT_VN,
        .length = sizeof(START_STREAMING_PROMPT_VN) - 1,
        .status = RRDPUSH_STATUS_CONNECTED,
        .version = STREAM_HANDSHAKE_OK_V3, // and above
        .dynamic = true,                 // dynamic = we will parse the version / capabilities
        .error = NULL,
        .worker_job_id = 0,
        .postpone_reconnect_seconds = 0,
        .priority = NDLP_INFO,
    },
    {
        .response = START_STREAMING_PROMPT_V2,
        .length = sizeof(START_STREAMING_PROMPT_V2) - 1,
        .status = RRDPUSH_STATUS_CONNECTED,
        .version = STREAM_HANDSHAKE_OK_V2,
        .dynamic = false,
        .error = NULL,
        .worker_job_id = 0,
        .postpone_reconnect_seconds = 0,
        .priority = NDLP_INFO,
    },
    {
        .response = START_STREAMING_PROMPT_V1,
        .length = sizeof(START_STREAMING_PROMPT_V1) - 1,
        .status = RRDPUSH_STATUS_CONNECTED,
        .version = STREAM_HANDSHAKE_OK_V1,
        .dynamic = false,
        .error = NULL,
        .worker_job_id = 0,
        .postpone_reconnect_seconds = 0,
        .priority = NDLP_INFO,
    },
    {
        .response = START_STREAMING_ERROR_SAME_LOCALHOST,
        .length = sizeof(START_STREAMING_ERROR_SAME_LOCALHOST) - 1,
        .status = RRDPUSH_STATUS_LOCALHOST,
        .version = STREAM_HANDSHAKE_ERROR_LOCALHOST,
        .dynamic = false,
        .error = "remote server rejected this stream, the host we are trying to stream is its localhost",
        .worker_job_id = WORKER_SENDER_JOB_DISCONNECT_BAD_HANDSHAKE,
        .postpone_reconnect_seconds = 60 * 60, // the IP may change, try it every hour
        .priority = NDLP_DEBUG,
    },
    {
        .response = START_STREAMING_ERROR_ALREADY_STREAMING,
        .length = sizeof(START_STREAMING_ERROR_ALREADY_STREAMING) - 1,
        .status = RRDPUSH_STATUS_ALREADY_CONNECTED,
        .version = STREAM_HANDSHAKE_ERROR_ALREADY_CONNECTED,
        .dynamic = false,
        .error = "remote server rejected this stream, the host we are trying to stream is already streamed to it",
        .worker_job_id = WORKER_SENDER_JOB_DISCONNECT_BAD_HANDSHAKE,
        .postpone_reconnect_seconds = 2 * 60, // 2 minutes
        .priority = NDLP_DEBUG,
    },
    {
        .response = START_STREAMING_ERROR_NOT_PERMITTED,
        .length = sizeof(START_STREAMING_ERROR_NOT_PERMITTED) - 1,
        .status = RRDPUSH_STATUS_PERMISSION_DENIED,
        .version = STREAM_HANDSHAKE_ERROR_DENIED,
        .dynamic = false,
        .error = "remote server denied access, probably we don't have the right API key?",
        .worker_job_id = WORKER_SENDER_JOB_DISCONNECT_BAD_HANDSHAKE,
        .postpone_reconnect_seconds = 1 * 60, // 1 minute
        .priority = NDLP_ERR,
    },
    {
        .response = START_STREAMING_ERROR_BUSY_TRY_LATER,
        .length = sizeof(START_STREAMING_ERROR_BUSY_TRY_LATER) - 1,
        .status = RRDPUSH_STATUS_RATE_LIMIT,
        .version = STREAM_HANDSHAKE_BUSY_TRY_LATER,
        .dynamic = false,
        .error = "remote server is currently busy, we should try later",
        .worker_job_id = WORKER_SENDER_JOB_DISCONNECT_BAD_HANDSHAKE,
        .postpone_reconnect_seconds = 2 * 60, // 2 minutes
        .priority = NDLP_NOTICE,
    },
    {
        .response = START_STREAMING_ERROR_INTERNAL_ERROR,
        .length = sizeof(START_STREAMING_ERROR_INTERNAL_ERROR) - 1,
        .status = RRDPUSH_STATUS_INTERNAL_SERVER_ERROR,
        .version = STREAM_HANDSHAKE_INTERNAL_ERROR,
        .dynamic = false,
        .error = "remote server is encountered an internal error, we should try later",
        .worker_job_id = WORKER_SENDER_JOB_DISCONNECT_BAD_HANDSHAKE,
        .postpone_reconnect_seconds = 5 * 60, // 5 minutes
        .priority = NDLP_CRIT,
    },
    {
        .response = START_STREAMING_ERROR_INITIALIZATION,
        .length = sizeof(START_STREAMING_ERROR_INITIALIZATION) - 1,
        .status = RRDPUSH_STATUS_INITIALIZATION_IN_PROGRESS,
        .version = STREAM_HANDSHAKE_INITIALIZATION,
        .dynamic = false,
        .error = "remote server is initializing, we should try later",
        .worker_job_id = WORKER_SENDER_JOB_DISCONNECT_BAD_HANDSHAKE,
        .postpone_reconnect_seconds = 2 * 60, // 2 minute
        .priority = NDLP_NOTICE,
    },

    // terminator
    {
        .response = NULL,
        .length = 0,
        .status = RRDPUSH_STATUS_BAD_HANDSHAKE,
        .version = STREAM_HANDSHAKE_ERROR_BAD_HANDSHAKE,
        .dynamic = false,
        .error = "remote node response is not understood, is it Netdata?",
        .worker_job_id = WORKER_SENDER_JOB_DISCONNECT_BAD_HANDSHAKE,
        .postpone_reconnect_seconds = 1 * 60, // 1 minute
        .priority = NDLP_ERR,
    }
};

static inline bool rrdpush_sender_validate_response(RRDHOST *host, struct sender_state *s, char *http, size_t http_length) {
    int32_t version = STREAM_HANDSHAKE_ERROR_BAD_HANDSHAKE;

    int i;
    for(i = 0; stream_responses[i].response ; i++) {
        if(stream_responses[i].dynamic &&
            http_length > stream_responses[i].length && http_length < (stream_responses[i].length + 30) &&
            strncmp(http, stream_responses[i].response, stream_responses[i].length) == 0) {

            version = str2i(&http[stream_responses[i].length]);
            break;
        }
        else if(http_length == stream_responses[i].length && strcmp(http, stream_responses[i].response) == 0) {
            version = stream_responses[i].version;

            break;
        }
    }

    if(version >= STREAM_HANDSHAKE_OK_V1) {
        host->destination->reason = version;
        host->destination->postpone_reconnection_until = now_realtime_sec() + s->reconnect_delay;
        s->capabilities = convert_stream_version_to_capabilities(version, host, true);
        return true;
    }

    ND_LOG_FIELD_PRIORITY priority = stream_responses[i].priority;
    const char *error = stream_responses[i].error;
    const char *status = stream_responses[i].status;
    int worker_job_id = stream_responses[i].worker_job_id;
    int delay = stream_responses[i].postpone_reconnect_seconds;

    worker_is_busy(worker_job_id);
    rrdpush_sender_thread_close_socket(host);
    host->destination->reason = version;
    host->destination->postpone_reconnection_until = now_realtime_sec() + delay;

    ND_LOG_STACK lgs[] = {
            ND_LOG_FIELD_TXT(NDF_RESPONSE_CODE, status),
            ND_LOG_FIELD_END(),
    };
    ND_LOG_STACK_PUSH(lgs);

    char buf[RFC3339_MAX_LENGTH];
    rfc3339_datetime_ut(buf, sizeof(buf), host->destination->postpone_reconnection_until * USEC_PER_SEC, 0, false);

    nd_log(NDLS_DAEMON, priority,
           "STREAM %s [send to %s]: %s - will retry in %d secs, at %s",
           rrdhost_hostname(host), s->connected_to, error, delay, buf);

    return false;
}

unsigned char alpn_proto_list[] = {
    18, 'n', 'e', 't', 'd', 'a', 't', 'a', '_', 's', 't', 'r', 'e', 'a', 'm', '/', '2', '.', '0',
    8, 'h', 't', 't', 'p', '/', '1', '.', '1'
};

#define CONN_UPGRADE_VAL "upgrade"

static bool rrdpush_sender_connect_ssl(struct sender_state *s __maybe_unused) {
#ifdef ENABLE_HTTPS
    RRDHOST *host = s->host;
    bool ssl_required = host->destination && host->destination->ssl;

    netdata_ssl_close(&host->sender->ssl);

    if(!ssl_required)
        return true;

    if (netdata_ssl_open_ext(&host->sender->ssl, netdata_ssl_streaming_sender_ctx, s->rrdpush_sender_socket, alpn_proto_list, sizeof(alpn_proto_list))) {
        if(!netdata_ssl_connect(&host->sender->ssl)) {
            // couldn't connect

            ND_LOG_STACK lgs[] = {
                    ND_LOG_FIELD_TXT(NDF_RESPONSE_CODE, RRDPUSH_STATUS_SSL_ERROR),
                    ND_LOG_FIELD_END(),
            };
            ND_LOG_STACK_PUSH(lgs);

            worker_is_busy(WORKER_SENDER_JOB_DISCONNECT_SSL_ERROR);
            rrdpush_sender_thread_close_socket(host);
            host->destination->reason = STREAM_HANDSHAKE_ERROR_SSL_ERROR;
            host->destination->postpone_reconnection_until = now_realtime_sec() + 5 * 60;
            return false;
        }

        if (netdata_ssl_validate_certificate_sender &&
            security_test_certificate(host->sender->ssl.conn)) {
            // certificate is not valid

            ND_LOG_STACK lgs[] = {
                    ND_LOG_FIELD_TXT(NDF_RESPONSE_CODE, RRDPUSH_STATUS_INVALID_SSL_CERTIFICATE),
                    ND_LOG_FIELD_END(),
            };
            ND_LOG_STACK_PUSH(lgs);

            worker_is_busy(WORKER_SENDER_JOB_DISCONNECT_SSL_ERROR);
            netdata_log_error("SSL: closing the stream connection, because the server SSL certificate is not valid.");
            rrdpush_sender_thread_close_socket(host);
            host->destination->reason = STREAM_HANDSHAKE_ERROR_INVALID_CERTIFICATE;
            host->destination->postpone_reconnection_until = now_realtime_sec() + 5 * 60;
            return false;
        }

        return true;
    }

    ND_LOG_STACK lgs[] = {
            ND_LOG_FIELD_TXT(NDF_RESPONSE_CODE, RRDPUSH_STATUS_CANT_ESTABLISH_SSL_CONNECTION),
            ND_LOG_FIELD_END(),
    };
    ND_LOG_STACK_PUSH(lgs);

    netdata_log_error("SSL: failed to establish connection.");
    return false;

#else
    // SSL is not enabled
    return true;
#endif
}

static int rrdpush_http_upgrade_prelude(RRDHOST *host, struct sender_state *s) {

    char http[HTTP_HEADER_SIZE + 1];
    snprintfz(http, HTTP_HEADER_SIZE,
            "GET " NETDATA_STREAM_URL HTTP_1_1 HTTP_ENDL
            "Upgrade: " NETDATA_STREAM_PROTO_NAME HTTP_ENDL
            "Connection: Upgrade"
            HTTP_HDR_END);

    ssize_t bytes = send_timeout(
#ifdef ENABLE_HTTPS
        &host->sender->ssl,
#endif
        s->rrdpush_sender_socket,
        http,
        strlen(http),
        0,
        1000);

    bytes = recv_timeout(
#ifdef ENABLE_HTTPS
        &host->sender->ssl,
#endif
        s->rrdpush_sender_socket,
        http,
        HTTP_HEADER_SIZE,
        0,
        1000);

    if (bytes <= 0) {
        error_report("Error reading from remote");
        return 1;
    }

    rbuf_t buf = rbuf_create(bytes);
    rbuf_push(buf, http, bytes);

    http_parse_ctx ctx;
    http_parse_ctx_create(&ctx);
    ctx.flags |= HTTP_PARSE_FLAG_DONT_WAIT_FOR_CONTENT;

    int rc;
//    while((rc = parse_http_response(buf, &ctx)) == HTTP_PARSE_NEED_MORE_DATA);
    rc = parse_http_response(buf, &ctx);

    if (rc != HTTP_PARSE_SUCCESS) {
        error_report("Failed to parse HTTP response sent. (%d)", rc);
        goto err_cleanup;
    }
    if (ctx.http_code == HTTP_RESP_MOVED_PERM) {
        const char *hdr = get_http_header_by_name(&ctx, "location");
        if (hdr) 
            error_report("HTTP response is %d Moved Permanently (location: \"%s\") instead of expected %d Switching Protocols.", ctx.http_code, hdr, HTTP_RESP_SWITCH_PROTO);
        else
            error_report("HTTP response is %d instead of expected %d Switching Protocols.", ctx.http_code, HTTP_RESP_SWITCH_PROTO);
        goto err_cleanup;
    }
    if (ctx.http_code == HTTP_RESP_NOT_FOUND) {
        error_report("HTTP response is %d instead of expected %d Switching Protocols. Parent version too old.", ctx.http_code, HTTP_RESP_SWITCH_PROTO);
        // TODO set some flag here that will signify parent is older version
        // and to try connection without rrdpush_http_upgrade_prelude next time
        goto err_cleanup;
    }
    if (ctx.http_code != HTTP_RESP_SWITCH_PROTO) {
        error_report("HTTP response is %d instead of expected %d Switching Protocols", ctx.http_code, HTTP_RESP_SWITCH_PROTO);
        goto err_cleanup;
    }

    const char *hdr = get_http_header_by_name(&ctx, "connection");
    if (!hdr) {
        error_report("Missing \"connection\" header in reply");
        goto err_cleanup;
    }
    if (strncmp(hdr, CONN_UPGRADE_VAL, strlen(CONN_UPGRADE_VAL))) {
        error_report("Expected \"connection: " CONN_UPGRADE_VAL "\"");
        goto err_cleanup;
    }

    hdr = get_http_header_by_name(&ctx, "upgrade");
    if (!hdr) {
        error_report("Missing \"upgrade\" header in reply");
        goto err_cleanup;
    }
    if (strncmp(hdr, NETDATA_STREAM_PROTO_NAME, strlen(NETDATA_STREAM_PROTO_NAME))) {
        error_report("Expected \"upgrade: " NETDATA_STREAM_PROTO_NAME "\"");
        goto err_cleanup;
    }

    netdata_log_debug(D_STREAM, "Stream sender upgrade to \"" NETDATA_STREAM_PROTO_NAME "\" successful");
    rbuf_free(buf);
    http_parse_ctx_destroy(&ctx);
    return 0;
err_cleanup:
    rbuf_free(buf);
    http_parse_ctx_destroy(&ctx);
    return 1;
}

static bool rrdpush_sender_thread_connect_to_parent(RRDHOST *host, int default_port, int timeout, struct sender_state *s) {

    struct timeval tv = {
            .tv_sec = timeout,
            .tv_usec = 0
    };

    // make sure the socket is closed
    rrdpush_sender_thread_close_socket(host);

    s->rrdpush_sender_socket = connect_to_one_of_destinations(
              host
            , default_port
            , &tv
            , &s->reconnects_counter
            , s->connected_to
            , sizeof(s->connected_to)-1
            , &host->destination
    );

    if(unlikely(s->rrdpush_sender_socket == -1)) {
        // netdata_log_error("STREAM %s [send to %s]: could not connect to parent node at this time.", rrdhost_hostname(host), host->rrdpush_send_destination);
        return false;
    }

    // netdata_log_info("STREAM %s [send to %s]: initializing communication...", rrdhost_hostname(host), s->connected_to);

    // reset our capabilities to default
    s->capabilities = stream_our_capabilities(host, true);

    /* TODO: During the implementation of #7265 switch the set of variables to HOST_* and CONTAINER_* if the
             version negotiation resulted in a high enough version.
    */
    stream_encoded_t se;
    rrdpush_encode_variable(&se, host);

    host->sender->hops = host->system_info->hops + 1;

    char http[HTTP_HEADER_SIZE + 1];
    int eol = snprintfz(http, HTTP_HEADER_SIZE,
            "STREAM "
                 "key=%s"
                 "&hostname=%s"
                 "&registry_hostname=%s"
                 "&machine_guid=%s"
                 "&update_every=%d"
                 "&os=%s"
                 "&timezone=%s"
                 "&abbrev_timezone=%s"
                 "&utc_offset=%d"
                 "&hops=%d"
                 "&ml_capable=%d"
                 "&ml_enabled=%d"
                 "&mc_version=%d"
                 "&tags=%s"
                 "&ver=%u"
                 "&NETDATA_INSTANCE_CLOUD_TYPE=%s"
                 "&NETDATA_INSTANCE_CLOUD_INSTANCE_TYPE=%s"
                 "&NETDATA_INSTANCE_CLOUD_INSTANCE_REGION=%s"
                 "&NETDATA_SYSTEM_OS_NAME=%s"
                 "&NETDATA_SYSTEM_OS_ID=%s"
                 "&NETDATA_SYSTEM_OS_ID_LIKE=%s"
                 "&NETDATA_SYSTEM_OS_VERSION=%s"
                 "&NETDATA_SYSTEM_OS_VERSION_ID=%s"
                 "&NETDATA_SYSTEM_OS_DETECTION=%s"
                 "&NETDATA_HOST_IS_K8S_NODE=%s"
                 "&NETDATA_SYSTEM_KERNEL_NAME=%s"
                 "&NETDATA_SYSTEM_KERNEL_VERSION=%s"
                 "&NETDATA_SYSTEM_ARCHITECTURE=%s"
                 "&NETDATA_SYSTEM_VIRTUALIZATION=%s"
                 "&NETDATA_SYSTEM_VIRT_DETECTION=%s"
                 "&NETDATA_SYSTEM_CONTAINER=%s"
                 "&NETDATA_SYSTEM_CONTAINER_DETECTION=%s"
                 "&NETDATA_CONTAINER_OS_NAME=%s"
                 "&NETDATA_CONTAINER_OS_ID=%s"
                 "&NETDATA_CONTAINER_OS_ID_LIKE=%s"
                 "&NETDATA_CONTAINER_OS_VERSION=%s"
                 "&NETDATA_CONTAINER_OS_VERSION_ID=%s"
                 "&NETDATA_CONTAINER_OS_DETECTION=%s"
                 "&NETDATA_SYSTEM_CPU_LOGICAL_CPU_COUNT=%s"
                 "&NETDATA_SYSTEM_CPU_FREQ=%s"
                 "&NETDATA_SYSTEM_TOTAL_RAM=%s"
                 "&NETDATA_SYSTEM_TOTAL_DISK_SIZE=%s"
                 "&NETDATA_PROTOCOL_VERSION=%s"
                 HTTP_1_1 HTTP_ENDL
                 "User-Agent: %s/%s\r\n"
                 "Accept: */*\r\n\r\n"
                 , host->rrdpush_send_api_key
                 , rrdhost_hostname(host)
                 , rrdhost_registry_hostname(host)
                 , host->machine_guid
                 , default_rrd_update_every
                 , rrdhost_os(host)
                 , rrdhost_timezone(host)
                 , rrdhost_abbrev_timezone(host)
                 , host->utc_offset
                 , host->sender->hops
                 , host->system_info->ml_capable
                 , host->system_info->ml_enabled
                 , host->system_info->mc_version
                 , rrdhost_tags(host)
                 , s->capabilities
                 , (host->system_info->cloud_provider_type) ? host->system_info->cloud_provider_type : ""
                 , (host->system_info->cloud_instance_type) ? host->system_info->cloud_instance_type : ""
                 , (host->system_info->cloud_instance_region) ? host->system_info->cloud_instance_region : ""
                 , se.os_name
                 , se.os_id
                 , (host->system_info->host_os_id_like) ? host->system_info->host_os_id_like : ""
                 , se.os_version
                 , (host->system_info->host_os_version_id) ? host->system_info->host_os_version_id : ""
                 , (host->system_info->host_os_detection) ? host->system_info->host_os_detection : ""
                 , (host->system_info->is_k8s_node) ? host->system_info->is_k8s_node : ""
                 , se.kernel_name
                 , se.kernel_version
                 , (host->system_info->architecture) ? host->system_info->architecture : ""
                 , (host->system_info->virtualization) ? host->system_info->virtualization : ""
                 , (host->system_info->virt_detection) ? host->system_info->virt_detection : ""
                 , (host->system_info->container) ? host->system_info->container : ""
                 , (host->system_info->container_detection) ? host->system_info->container_detection : ""
                 , (host->system_info->container_os_name) ? host->system_info->container_os_name : ""
                 , (host->system_info->container_os_id) ? host->system_info->container_os_id : ""
                 , (host->system_info->container_os_id_like) ? host->system_info->container_os_id_like : ""
                 , (host->system_info->container_os_version) ? host->system_info->container_os_version : ""
                 , (host->system_info->container_os_version_id) ? host->system_info->container_os_version_id : ""
                 , (host->system_info->container_os_detection) ? host->system_info->container_os_detection : ""
                 , (host->system_info->host_cores) ? host->system_info->host_cores : ""
                 , (host->system_info->host_cpu_freq) ? host->system_info->host_cpu_freq : ""
                 , (host->system_info->host_ram_total) ? host->system_info->host_ram_total : ""
                 , (host->system_info->host_disk_space) ? host->system_info->host_disk_space : ""
                 , STREAMING_PROTOCOL_VERSION
                 , rrdhost_program_name(host)
                 , rrdhost_program_version(host)
                 );
    http[eol] = 0x00;
    rrdpush_clean_encoded(&se);

    if(!rrdpush_sender_connect_ssl(s))
        return false;

    if (s->parent_using_h2o && rrdpush_http_upgrade_prelude(host, s)) {
        ND_LOG_STACK lgs[] = {
                ND_LOG_FIELD_TXT(NDF_RESPONSE_CODE, RRDPUSH_STATUS_CANT_UPGRADE_CONNECTION),
                ND_LOG_FIELD_END(),
        };
        ND_LOG_STACK_PUSH(lgs);

        worker_is_busy(WORKER_SENDER_JOB_DISCONNECT_CANT_UPGRADE_CONNECTION);
        rrdpush_sender_thread_close_socket(host);
        host->destination->reason = STREAM_HANDSHAKE_ERROR_HTTP_UPGRADE;
        host->destination->postpone_reconnection_until = now_realtime_sec() + 1 * 60;
        return false;
    }
    
    ssize_t bytes, len = (ssize_t)strlen(http);

    bytes = send_timeout(
#ifdef ENABLE_HTTPS
        &host->sender->ssl,
#endif
        s->rrdpush_sender_socket,
        http,
        len,
        0,
        timeout);

    if(bytes <= 0) { // timeout is 0
        ND_LOG_STACK lgs[] = {
                ND_LOG_FIELD_TXT(NDF_RESPONSE_CODE, RRDPUSH_STATUS_TIMEOUT),
                ND_LOG_FIELD_END(),
        };
        ND_LOG_STACK_PUSH(lgs);

        worker_is_busy(WORKER_SENDER_JOB_DISCONNECT_TIMEOUT);
        rrdpush_sender_thread_close_socket(host);

        nd_log(NDLS_DAEMON, NDLP_ERR,
               "STREAM %s [send to %s]: failed to send HTTP header to remote netdata.",
               rrdhost_hostname(host), s->connected_to);

        host->destination->reason = STREAM_HANDSHAKE_ERROR_SEND_TIMEOUT;
        host->destination->postpone_reconnection_until = now_realtime_sec() + 1 * 60;
        return false;
    }

    bytes = recv_timeout(
#ifdef ENABLE_HTTPS
        &host->sender->ssl,
#endif
        s->rrdpush_sender_socket,
        http,
        HTTP_HEADER_SIZE,
        0,
        timeout);

    if(bytes <= 0) { // timeout is 0
        ND_LOG_STACK lgs[] = {
                ND_LOG_FIELD_TXT(NDF_RESPONSE_CODE, RRDPUSH_STATUS_TIMEOUT),
                ND_LOG_FIELD_END(),
        };
        ND_LOG_STACK_PUSH(lgs);

        worker_is_busy(WORKER_SENDER_JOB_DISCONNECT_TIMEOUT);
        rrdpush_sender_thread_close_socket(host);

        nd_log(NDLS_DAEMON, NDLP_ERR,
               "STREAM %s [send to %s]: remote netdata does not respond.",
               rrdhost_hostname(host), s->connected_to);

        host->destination->reason = STREAM_HANDSHAKE_ERROR_RECEIVE_TIMEOUT;
        host->destination->postpone_reconnection_until = now_realtime_sec() + 30;
        return false;
    }

    if(sock_setnonblock(s->rrdpush_sender_socket) < 0)
        nd_log(NDLS_DAEMON, NDLP_WARNING,
               "STREAM %s [send to %s]: cannot set non-blocking mode for socket.",
               rrdhost_hostname(host), s->connected_to);

    if(sock_enlarge_out(s->rrdpush_sender_socket) < 0)
        nd_log(NDLS_DAEMON, NDLP_WARNING,
               "STREAM %s [send to %s]: cannot enlarge the socket buffer.",
               rrdhost_hostname(host), s->connected_to);

    http[bytes] = '\0';
    if(!rrdpush_sender_validate_response(host, s, http, bytes))
        return false;

    rrdpush_compression_initialize(s);

    log_sender_capabilities(s);

    ND_LOG_STACK lgs[] = {
            ND_LOG_FIELD_TXT(NDF_RESPONSE_CODE, RRDPUSH_STATUS_CONNECTED),
            ND_LOG_FIELD_END(),
    };
    ND_LOG_STACK_PUSH(lgs);

    nd_log(NDLS_DAEMON, NDLP_DEBUG,
           "STREAM %s: connected to %s...",
           rrdhost_hostname(host), s->connected_to);

    return true;
}

static bool attempt_to_connect(struct sender_state *state) {
    ND_LOG_STACK lgs[] = {
            ND_LOG_FIELD_UUID(NDF_MESSAGE_ID, &streaming_to_parent_msgid),
            ND_LOG_FIELD_END(),
    };
    ND_LOG_STACK_PUSH(lgs);

    state->send_attempts = 0;

    // reset the bytes we have sent for this session
    state->sent_bytes_on_this_connection = 0;
    memset(state->sent_bytes_on_this_connection_per_type, 0, sizeof(state->sent_bytes_on_this_connection_per_type));

    if(rrdpush_sender_thread_connect_to_parent(state->host, state->default_port, state->timeout, state)) {
        // reset the buffer, to properly send charts and metrics
        rrdpush_sender_on_connect(state->host);

        // send from the beginning
        state->begin = 0;

        // make sure the next reconnection will be immediate
        state->not_connected_loops = 0;

        // let the data collection threads know we are ready
        rrdhost_flag_set(state->host, RRDHOST_FLAG_RRDPUSH_SENDER_CONNECTED);

        rrdpush_sender_after_connect(state->host);

        return true;
    }

    // we couldn't connect

    // increase the failed connections counter
    state->not_connected_loops++;

    // slow re-connection on repeating errors
    usec_t now_ut = now_monotonic_usec();
    usec_t end_ut = now_ut + USEC_PER_SEC * state->reconnect_delay;
    while(now_ut < end_ut) {
        netdata_thread_testcancel();
        sleep_usec(500 * USEC_PER_MS); // seconds
        now_ut = now_monotonic_usec();
    }

    return false;
}

// TCP window is open, and we have data to transmit.
static ssize_t attempt_to_send(struct sender_state *s) {
    ssize_t ret;

#ifdef NETDATA_INTERNAL_CHECKS
    struct circular_buffer *cb = s->buffer;
#endif

    sender_lock(s);
    char *chunk;
    size_t outstanding = cbuffer_next_unsafe(s->buffer, &chunk);
    netdata_log_debug(D_STREAM, "STREAM: Sending data. Buffer r=%zu w=%zu s=%zu, next chunk=%zu", cb->read, cb->write, cb->size, outstanding);

#ifdef ENABLE_HTTPS
    if(SSL_connection(&s->ssl))
        ret = netdata_ssl_write(&s->ssl, chunk, outstanding);
    else
        ret = send(s->rrdpush_sender_socket, chunk, outstanding, MSG_DONTWAIT);
#else
    ret = send(s->rrdpush_sender_socket, chunk, outstanding, MSG_DONTWAIT);
#endif

    if (likely(ret > 0)) {
        cbuffer_remove_unsafe(s->buffer, ret);
        s->sent_bytes_on_this_connection += ret;
        s->sent_bytes += ret;
        netdata_log_debug(D_STREAM, "STREAM %s [send to %s]: Sent %zd bytes", rrdhost_hostname(s->host), s->connected_to, ret);
    }
    else if (ret == -1 && (errno == EAGAIN || errno == EINTR || errno == EWOULDBLOCK))
        netdata_log_debug(D_STREAM, "STREAM %s [send to %s]: unavailable after polling POLLOUT", rrdhost_hostname(s->host), s->connected_to);
    else if (ret == -1) {
        worker_is_busy(WORKER_SENDER_JOB_DISCONNECT_SEND_ERROR);
        netdata_log_debug(D_STREAM, "STREAM: Send failed - closing socket...");
        netdata_log_error("STREAM %s [send to %s]: failed to send metrics - closing connection - we have sent %zu bytes on this connection.",  rrdhost_hostname(s->host), s->connected_to, s->sent_bytes_on_this_connection);
        rrdpush_sender_thread_close_socket(s->host);
    }
    else
        netdata_log_debug(D_STREAM, "STREAM: send() returned 0 -> no error but no transmission");

    replication_recalculate_buffer_used_ratio_unsafe(s);
    sender_unlock(s);

    return ret;
}

static ssize_t attempt_read(struct sender_state *s) {
    ssize_t ret;

#ifdef ENABLE_HTTPS
    if (SSL_connection(&s->ssl))
        ret = netdata_ssl_read(&s->ssl, s->read_buffer + s->read_len, sizeof(s->read_buffer) - s->read_len - 1);
    else
        ret = recv(s->rrdpush_sender_socket, s->read_buffer + s->read_len, sizeof(s->read_buffer) - s->read_len - 1,MSG_DONTWAIT);
#else
    ret = recv(s->rrdpush_sender_socket, s->read_buffer + s->read_len, sizeof(s->read_buffer) - s->read_len - 1,MSG_DONTWAIT);
#endif

    if (ret > 0) {
        s->read_len += ret;
        return ret;
    }

    if (ret < 0 && (errno == EAGAIN || errno == EWOULDBLOCK || errno == EINTR))
        return ret;

#ifdef ENABLE_HTTPS
    if (SSL_connection(&s->ssl))
        worker_is_busy(WORKER_SENDER_JOB_DISCONNECT_SSL_ERROR);
    else
#endif

    if (ret == 0 || errno == ECONNRESET) {
        worker_is_busy(WORKER_SENDER_JOB_DISCONNECT_PARENT_CLOSED);
        netdata_log_error("STREAM %s [send to %s]: connection closed by far end.", rrdhost_hostname(s->host), s->connected_to);
    }
    else {
        worker_is_busy(WORKER_SENDER_JOB_DISCONNECT_RECEIVE_ERROR);
        netdata_log_error("STREAM %s [send to %s]: error during receive (%zd) - closing connection.", rrdhost_hostname(s->host), s->connected_to, ret);
    }

    rrdpush_sender_thread_close_socket(s->host);

    return ret;
}

struct inflight_stream_function {
    struct sender_state *sender;
    STRING *transaction;
    usec_t received_ut;
};

static void stream_execute_function_callback(BUFFER *func_wb, int code, void *data) {
    struct inflight_stream_function *tmp = data;
    struct sender_state *s = tmp->sender;

    if(rrdhost_can_send_definitions_to_parent(s->host)) {
        BUFFER *wb = sender_start(s);

        pluginsd_function_result_begin_to_buffer(wb
                                                 , string2str(tmp->transaction)
                                                 , code
                                                 , content_type_id2string(func_wb->content_type)
                                                 , func_wb->expires);

        buffer_fast_strcat(wb, buffer_tostring(func_wb), buffer_strlen(func_wb));
        pluginsd_function_result_end_to_buffer(wb);

        sender_commit(s, wb, STREAM_TRAFFIC_TYPE_FUNCTIONS);
        sender_thread_buffer_free();

        internal_error(true, "STREAM %s [send to %s] FUNCTION transaction %s sending back response (%zu bytes, %"PRIu64" usec).",
                       rrdhost_hostname(s->host), s->connected_to,
                       string2str(tmp->transaction),
                       buffer_strlen(func_wb),
                       now_realtime_usec() - tmp->received_ut);
    }

    string_freez(tmp->transaction);
    buffer_free(func_wb);
    freez(tmp);
}

static void stream_execute_function_progress_callback(void *data, size_t done, size_t all) {
    struct inflight_stream_function *tmp = data;
    struct sender_state *s = tmp->sender;

    if(rrdhost_can_send_definitions_to_parent(s->host)) {
        BUFFER *wb = sender_start(s);

        buffer_sprintf(wb, PLUGINSD_KEYWORD_FUNCTION_PROGRESS " '%s' %zu %zu\n",
                       string2str(tmp->transaction), done, all);

        sender_commit(s, wb, STREAM_TRAFFIC_TYPE_FUNCTIONS);
    }
}

static void execute_commands_function(struct sender_state *s, const char *command, const char *transaction, const char *timeout_s, const char *function, BUFFER *payload, const char *access, const char *source) {
    worker_is_busy(WORKER_SENDER_JOB_FUNCTION_REQUEST);
    nd_log(NDLS_ACCESS, NDLP_INFO, NULL);

    if(!transaction || !*transaction || !timeout_s || !*timeout_s || !function || !*function) {
        netdata_log_error("STREAM %s [send to %s] %s execution command is incomplete (transaction = '%s', timeout = '%s', function = '%s'). Ignoring it.",
                          rrdhost_hostname(s->host), s->connected_to,
                          command,
                          transaction?transaction:"(unset)",
                          timeout_s?timeout_s:"(unset)",
                          function?function:"(unset)");
    }
    else {
        int timeout = str2i(timeout_s);
        if(timeout <= 0) timeout = PLUGINS_FUNCTIONS_TIMEOUT_DEFAULT;

        struct inflight_stream_function *tmp = callocz(1, sizeof(struct inflight_stream_function));
        tmp->received_ut = now_realtime_usec();
        tmp->sender = s;
        tmp->transaction = string_strdupz(transaction);
        BUFFER *wb = buffer_create(1024, &netdata_buffers_statistics.buffers_functions);

        int code = rrd_function_run(s->host, wb, timeout,
                                    http_access_from_hex_mapping_old_roles(access), function, false, transaction,
                                    stream_execute_function_callback, tmp,
                                    stream_has_capability(s, STREAM_CAP_PROGRESS) ? stream_execute_function_progress_callback : NULL,
                                    stream_has_capability(s, STREAM_CAP_PROGRESS) ? tmp : NULL,
                                    NULL, NULL, payload, source);

        if(code != HTTP_RESP_OK) {
            if (!buffer_strlen(wb))
                rrd_call_function_error(wb, "Failed to route request to collector", code);
        }
    }
}

static void cleanup_intercepting_input(struct sender_state *s) {
    freez((void *)s->functions.transaction);
    freez((void *)s->functions.timeout_s);
    freez((void *)s->functions.function);
    freez((void *)s->functions.access);
    freez((void *)s->functions.source);
    buffer_free(s->functions.payload);

    s->functions.transaction = NULL;
    s->functions.timeout_s = NULL;
    s->functions.function = NULL;
    s->functions.payload = NULL;
    s->functions.access = NULL;
    s->functions.source = NULL;
    s->functions.intercept_input = false;
}

static void execute_commands_cleanup(struct sender_state *s) {
    cleanup_intercepting_input(s);
}

// This is just a placeholder until the gap filling state machine is inserted
void execute_commands(struct sender_state *s) {
    worker_is_busy(WORKER_SENDER_JOB_EXECUTE);

    ND_LOG_STACK lgs[] = {
            ND_LOG_FIELD_CB(NDF_REQUEST, line_splitter_reconstruct_line, &s->line),
            ND_LOG_FIELD_END(),
    };
    ND_LOG_STACK_PUSH(lgs);

    char *start = s->read_buffer, *end = &s->read_buffer[s->read_len], *newline;
    *end = 0;
    while( start < end && (newline = strchr(start, '\n')) ) {
        s->line.count++;

        if(s->functions.intercept_input) {
            if(strcmp(start, PLUGINSD_CALL_FUNCTION_PAYLOAD_END "\n") == 0) {
                execute_commands_function(s,
                    PLUGINSD_CALL_FUNCTION_PAYLOAD_END,
                                          s->functions.transaction, s->functions.timeout_s,
                                          s->functions.function, s->functions.payload,
                                          s->functions.access, s->functions.source);

                cleanup_intercepting_input(s);
            }
            else
                buffer_strcat(s->functions.payload, start);

            start = newline + 1;
            continue;
        }

        *newline = '\0';
        s->line.num_words = quoted_strings_splitter_pluginsd(start, s->line.words, PLUGINSD_MAX_WORDS);
        const char *command = get_word(s->line.words, s->line.num_words, 0);

        if(command && strcmp(command, PLUGINSD_CALL_FUNCTION) == 0) {
            char *transaction  = get_word(s->line.words, s->line.num_words, 1);
            char *timeout_s    = get_word(s->line.words, s->line.num_words, 2);
            char *function     = get_word(s->line.words, s->line.num_words, 3);
            char *access       = get_word(s->line.words, s->line.num_words, 4);
            char *source       = get_word(s->line.words, s->line.num_words, 5);

            execute_commands_function(s, command, transaction, timeout_s, function, NULL, access, source);
        }
        else if(command && strcmp(command, PLUGINSD_CALL_FUNCTION_PAYLOAD_BEGIN) == 0) {
            char *transaction  = get_word(s->line.words, s->line.num_words, 1);
            char *timeout_s    = get_word(s->line.words, s->line.num_words, 2);
            char *function     = get_word(s->line.words, s->line.num_words, 3);
            char *access       = get_word(s->line.words, s->line.num_words, 4);
            char *source       = get_word(s->line.words, s->line.num_words, 5);
            char *content_type = get_word(s->line.words, s->line.num_words, 6);

            s->functions.transaction = strdupz(transaction ? transaction : "");
            s->functions.timeout_s = strdupz(timeout_s ? timeout_s : "");
            s->functions.function = strdupz(function ? function : "");
            s->functions.access = strdupz(access ? access : "");
            s->functions.source = strdupz(source ? source : "");
            s->functions.payload = buffer_create(0, NULL);
            s->functions.payload->content_type = content_type_string2id(content_type);
            s->functions.intercept_input = true;
        }
        else if(command && strcmp(command, PLUGINSD_CALL_FUNCTION_CANCEL) == 0) {
            worker_is_busy(WORKER_SENDER_JOB_FUNCTION_REQUEST);
            nd_log(NDLS_ACCESS, NDLP_DEBUG, NULL);

            char *transaction = get_word(s->line.words, s->line.num_words, 1);
            if(transaction && *transaction)
                rrd_function_cancel(transaction);
        }
        else if(command && strcmp(command, PLUGINSD_CALL_FUNCTION_PROGRESS) == 0) {
            worker_is_busy(WORKER_SENDER_JOB_FUNCTION_REQUEST);
            nd_log(NDLS_ACCESS, NDLP_DEBUG, NULL);

            char *transaction = get_word(s->line.words, s->line.num_words, 1);
            if(transaction && *transaction)
                rrd_function_progress(transaction);
        }
        else if (command && strcmp(command, PLUGINSD_KEYWORD_REPLAY_CHART) == 0) {
            worker_is_busy(WORKER_SENDER_JOB_REPLAY_REQUEST);
            nd_log(NDLS_ACCESS, NDLP_DEBUG, NULL);

            const char *chart_id = get_word(s->line.words, s->line.num_words, 1);
            const char *start_streaming = get_word(s->line.words, s->line.num_words, 2);
            const char *after = get_word(s->line.words, s->line.num_words, 3);
            const char *before = get_word(s->line.words, s->line.num_words, 4);

            if (!chart_id || !start_streaming || !after || !before) {
                netdata_log_error("STREAM %s [send to %s] %s command is incomplete"
                      " (chart=%s, start_streaming=%s, after=%s, before=%s)",
                      rrdhost_hostname(s->host), s->connected_to,
                      command,
                      chart_id ? chart_id : "(unset)",
                      start_streaming ? start_streaming : "(unset)",
                      after ? after : "(unset)",
                      before ? before : "(unset)");
            }
            else {
                replication_add_request(s, chart_id,
                                        strtoll(after, NULL, 0),
                                        strtoll(before, NULL, 0),
                                        !strcmp(start_streaming, "true")
                                        );
            }
        }
        else {
            netdata_log_error("STREAM %s [send to %s] received unknown command over connection: %s", rrdhost_hostname(s->host), s->connected_to, s->line.words[0]?s->line.words[0]:"(unset)");
        }

        line_splitter_reset(&s->line);
        worker_is_busy(WORKER_SENDER_JOB_EXECUTE);
        start = newline + 1;
    }

    if (start < end) {
        memmove(s->read_buffer, start, end-start);
        s->read_len = end - start;
    }
    else {
        s->read_buffer[0] = '\0';
        s->read_len = 0;
    }
}

struct rrdpush_sender_thread_data {
    RRDHOST *host;
    char *pipe_buffer;
};

static bool rrdpush_sender_pipe_close(RRDHOST *host, int *pipe_fds, bool reopen) {
    static netdata_mutex_t mutex = NETDATA_MUTEX_INITIALIZER;

    bool ret = true;

    netdata_mutex_lock(&mutex);

    int new_pipe_fds[2];
    if(reopen) {
        if(pipe(new_pipe_fds) != 0) {
            netdata_log_error("STREAM %s [send]: cannot create required pipe.", rrdhost_hostname(host));
            new_pipe_fds[PIPE_READ] = -1;
            new_pipe_fds[PIPE_WRITE] = -1;
            ret = false;
        }
    }

    int old_pipe_fds[2];
    old_pipe_fds[PIPE_READ] = pipe_fds[PIPE_READ];
    old_pipe_fds[PIPE_WRITE] = pipe_fds[PIPE_WRITE];

    if(reopen) {
        pipe_fds[PIPE_READ] = new_pipe_fds[PIPE_READ];
        pipe_fds[PIPE_WRITE] = new_pipe_fds[PIPE_WRITE];
    }
    else {
        pipe_fds[PIPE_READ] = -1;
        pipe_fds[PIPE_WRITE] = -1;
    }

    if(old_pipe_fds[PIPE_READ] > 2)
        close(old_pipe_fds[PIPE_READ]);

    if(old_pipe_fds[PIPE_WRITE] > 2)
        close(old_pipe_fds[PIPE_WRITE]);

    netdata_mutex_unlock(&mutex);
    return ret;
}

void rrdpush_signal_sender_to_wake_up(struct sender_state *s) {
    if(unlikely(s->tid == gettid()))
        return;

    RRDHOST *host = s->host;

    int pipe_fd = s->rrdpush_sender_pipe[PIPE_WRITE];

    // signal the sender there are more data
    if (pipe_fd != -1 && write(pipe_fd, " ", 1) == -1) {
        netdata_log_error("STREAM %s [send]: cannot write to internal pipe.", rrdhost_hostname(host));
        rrdpush_sender_pipe_close(host, s->rrdpush_sender_pipe, true);
    }
}

static bool rrdhost_set_sender(RRDHOST *host) {
    if(unlikely(!host->sender)) return false;

    bool ret = false;
    sender_lock(host->sender);
    if(!host->sender->tid) {
        rrdhost_flag_clear(host, RRDHOST_FLAG_RRDPUSH_SENDER_CONNECTED | RRDHOST_FLAG_RRDPUSH_SENDER_READY_4_METRICS);
        rrdhost_flag_set(host, RRDHOST_FLAG_RRDPUSH_SENDER_SPAWN);
        host->rrdpush_sender_connection_counter++;
        host->sender->tid = gettid();
        host->sender->last_state_since_t = now_realtime_sec();
        host->sender->exit.reason = STREAM_HANDSHAKE_NEVER;
        ret = true;
    }
    sender_unlock(host->sender);

    rrdpush_reset_destinations_postpone_time(host);

    return ret;
}

static void rrdhost_clear_sender___while_having_sender_mutex(RRDHOST *host) {
    if(unlikely(!host->sender)) return;

    if(host->sender->tid == gettid()) {
        host->sender->tid = 0;
        host->sender->exit.shutdown = false;
        rrdhost_flag_clear(host, RRDHOST_FLAG_RRDPUSH_SENDER_SPAWN | RRDHOST_FLAG_RRDPUSH_SENDER_CONNECTED | RRDHOST_FLAG_RRDPUSH_SENDER_READY_4_METRICS);
        host->sender->last_state_since_t = now_realtime_sec();
        if(host->destination) {
            host->destination->since = host->sender->last_state_since_t;
            host->destination->reason = host->sender->exit.reason;
        }
    }

    rrdpush_reset_destinations_postpone_time(host);
}

static bool rrdhost_sender_should_exit(struct sender_state *s) {
    // check for outstanding cancellation requests
    netdata_thread_testcancel();

    if(unlikely(!service_running(SERVICE_STREAMING))) {
        if(!s->exit.reason)
            s->exit.reason = STREAM_HANDSHAKE_DISCONNECT_NETDATA_EXIT;
        return true;
    }

    if(unlikely(!rrdhost_has_rrdpush_sender_enabled(s->host))) {
        if(!s->exit.reason)
            s->exit.reason = STREAM_HANDSHAKE_NON_STREAMABLE_HOST;
        return true;
    }

    if(unlikely(s->exit.shutdown)) {
        if(!s->exit.reason)
            s->exit.reason = STREAM_HANDSHAKE_DISCONNECT_SHUTDOWN;
        return true;
    }

    if(unlikely(rrdhost_flag_check(s->host, RRDHOST_FLAG_ORPHAN))) {
        if(!s->exit.reason)
            s->exit.reason = STREAM_HANDSHAKE_DISCONNECT_ORPHAN_HOST;
        return true;
    }

    return false;
}

static void rrdpush_sender_thread_cleanup_callback(void *ptr) {
    struct rrdpush_sender_thread_data *s = ptr;
    worker_unregister();

    RRDHOST *host = s->host;

    sender_lock(host->sender);
    netdata_log_info("STREAM %s [send]: sending thread exits %s",
         rrdhost_hostname(host),
         host->sender->exit.reason != STREAM_HANDSHAKE_NEVER ? stream_handshake_error_to_string(host->sender->exit.reason) : "");

    rrdpush_sender_thread_close_socket(host);
    rrdpush_sender_pipe_close(host, host->sender->rrdpush_sender_pipe, false);
    execute_commands_cleanup(host->sender);

    rrdhost_clear_sender___while_having_sender_mutex(host);

#ifdef NETDATA_LOG_STREAM_SENDER
    if(host->sender->stream_log_fp) {
        fclose(host->sender->stream_log_fp);
        host->sender->stream_log_fp = NULL;
    }
#endif

    sender_unlock(host->sender);

    freez(s->pipe_buffer);
    freez(s);
}

void rrdpush_initialize_ssl_ctx(RRDHOST *host __maybe_unused) {
#ifdef ENABLE_HTTPS
    static SPINLOCK sp = NETDATA_SPINLOCK_INITIALIZER;
    spinlock_lock(&sp);

    if(netdata_ssl_streaming_sender_ctx || !host) {
        spinlock_unlock(&sp);
        return;
    }

    for(struct rrdpush_destinations *d = host->destinations; d ; d = d->next) {
        if (d->ssl) {
            // we need to initialize SSL

            netdata_ssl_initialize_ctx(NETDATA_SSL_STREAMING_SENDER_CTX);
            ssl_security_location_for_context(netdata_ssl_streaming_sender_ctx, netdata_ssl_ca_file, netdata_ssl_ca_path);

            // stop the loop
            break;
        }
    }

    spinlock_unlock(&sp);
#endif
}

static bool stream_sender_log_capabilities(BUFFER *wb, void *ptr) {
    struct sender_state *state = ptr;
    if(!state)
        return false;

    stream_capabilities_to_string(wb, state->capabilities);
    return true;
}

static bool stream_sender_log_transport(BUFFER *wb, void *ptr) {
    struct sender_state *state = ptr;
    if(!state)
        return false;

#ifdef ENABLE_HTTPS
    buffer_strcat(wb, SSL_connection(&state->ssl) ? "https" : "http");
#else
    buffer_strcat(wb, "http");
#endif
    return true;
}

static bool stream_sender_log_dst_ip(BUFFER *wb, void *ptr) {
    struct sender_state *state = ptr;
    if(!state || state->rrdpush_sender_socket == -1)
        return false;

    SOCKET_PEERS peers = socket_peers(state->rrdpush_sender_socket);
    buffer_strcat(wb, peers.peer.ip);
    return true;
}

static bool stream_sender_log_dst_port(BUFFER *wb, void *ptr) {
    struct sender_state *state = ptr;
    if(!state || state->rrdpush_sender_socket == -1)
        return false;

    SOCKET_PEERS peers = socket_peers(state->rrdpush_sender_socket);
    buffer_print_uint64(wb, peers.peer.port);
    return true;
}

void *rrdpush_sender_thread(void *ptr) {
    struct sender_state *s = ptr;

    ND_LOG_STACK lgs[] = {
            ND_LOG_FIELD_STR(NDF_NIDL_NODE, s->host->hostname),
            ND_LOG_FIELD_CB(NDF_DST_IP, stream_sender_log_dst_ip, s),
            ND_LOG_FIELD_CB(NDF_DST_PORT, stream_sender_log_dst_port, s),
            ND_LOG_FIELD_CB(NDF_DST_TRANSPORT, stream_sender_log_transport, s),
            ND_LOG_FIELD_CB(NDF_SRC_CAPABILITIES, stream_sender_log_capabilities, s),
            ND_LOG_FIELD_END(),
    };
    ND_LOG_STACK_PUSH(lgs);

    worker_register("STREAMSND");
    worker_register_job_name(WORKER_SENDER_JOB_CONNECT, "connect");
    worker_register_job_name(WORKER_SENDER_JOB_PIPE_READ, "pipe read");
    worker_register_job_name(WORKER_SENDER_JOB_SOCKET_RECEIVE, "receive");
    worker_register_job_name(WORKER_SENDER_JOB_EXECUTE, "execute");
    worker_register_job_name(WORKER_SENDER_JOB_SOCKET_SEND, "send");

    // disconnection reasons
    worker_register_job_name(WORKER_SENDER_JOB_DISCONNECT_TIMEOUT, "disconnect timeout");
    worker_register_job_name(WORKER_SENDER_JOB_DISCONNECT_POLL_ERROR, "disconnect poll error");
    worker_register_job_name(WORKER_SENDER_JOB_DISCONNECT_SOCKET_ERROR, "disconnect socket error");
    worker_register_job_name(WORKER_SENDER_JOB_DISCONNECT_OVERFLOW, "disconnect overflow");
    worker_register_job_name(WORKER_SENDER_JOB_DISCONNECT_SSL_ERROR, "disconnect ssl error");
    worker_register_job_name(WORKER_SENDER_JOB_DISCONNECT_PARENT_CLOSED, "disconnect parent closed");
    worker_register_job_name(WORKER_SENDER_JOB_DISCONNECT_RECEIVE_ERROR, "disconnect receive error");
    worker_register_job_name(WORKER_SENDER_JOB_DISCONNECT_SEND_ERROR, "disconnect send error");
    worker_register_job_name(WORKER_SENDER_JOB_DISCONNECT_NO_COMPRESSION, "disconnect no compression");
    worker_register_job_name(WORKER_SENDER_JOB_DISCONNECT_BAD_HANDSHAKE, "disconnect bad handshake");
    worker_register_job_name(WORKER_SENDER_JOB_DISCONNECT_CANT_UPGRADE_CONNECTION, "disconnect cant upgrade");

    worker_register_job_name(WORKER_SENDER_JOB_REPLAY_REQUEST, "replay request");
    worker_register_job_name(WORKER_SENDER_JOB_FUNCTION_REQUEST, "function");

    worker_register_job_custom_metric(WORKER_SENDER_JOB_BUFFER_RATIO, "used buffer ratio", "%", WORKER_METRIC_ABSOLUTE);
    worker_register_job_custom_metric(WORKER_SENDER_JOB_BYTES_RECEIVED, "bytes received", "bytes/s", WORKER_METRIC_INCREMENT);
    worker_register_job_custom_metric(WORKER_SENDER_JOB_BYTES_SENT, "bytes sent", "bytes/s", WORKER_METRIC_INCREMENT);
    worker_register_job_custom_metric(WORKER_SENDER_JOB_BYTES_COMPRESSED, "bytes compressed", "bytes/s", WORKER_METRIC_INCREMENTAL_TOTAL);
    worker_register_job_custom_metric(WORKER_SENDER_JOB_BYTES_UNCOMPRESSED, "bytes uncompressed", "bytes/s", WORKER_METRIC_INCREMENTAL_TOTAL);
    worker_register_job_custom_metric(WORKER_SENDER_JOB_BYTES_COMPRESSION_RATIO, "cumulative compression savings ratio", "%", WORKER_METRIC_ABSOLUTE);
    worker_register_job_custom_metric(WORKER_SENDER_JOB_REPLAY_DICT_SIZE, "replication dict entries", "entries", WORKER_METRIC_ABSOLUTE);

    if(!rrdhost_has_rrdpush_sender_enabled(s->host) || !s->host->rrdpush_send_destination ||
       !*s->host->rrdpush_send_destination || !s->host->rrdpush_send_api_key ||
       !*s->host->rrdpush_send_api_key) {
        netdata_log_error("STREAM %s [send]: thread created (task id %d), but host has streaming disabled.",
              rrdhost_hostname(s->host), gettid());
        return NULL;
    }

    if(!rrdhost_set_sender(s->host)) {
        netdata_log_error("STREAM %s [send]: thread created (task id %d), but there is another sender running for this host.",
              rrdhost_hostname(s->host), gettid());
        return NULL;
    }

    rrdpush_initialize_ssl_ctx(s->host);

    netdata_log_info("STREAM %s [send]: thread created (task id %d)", rrdhost_hostname(s->host), gettid());

    s->timeout = (int)appconfig_get_number(
        &stream_config, CONFIG_SECTION_STREAM, "timeout seconds", 600);

    s->default_port = (int)appconfig_get_number(
        &stream_config, CONFIG_SECTION_STREAM, "default port", 19999);

    s->buffer->max_size = (size_t)appconfig_get_number(
        &stream_config, CONFIG_SECTION_STREAM, "buffer size bytes", 1024 * 1024 * 10);

    s->reconnect_delay = (unsigned int)appconfig_get_number(
        &stream_config, CONFIG_SECTION_STREAM, "reconnect delay seconds", 5);

    remote_clock_resync_iterations = (unsigned int)appconfig_get_number(
        &stream_config, CONFIG_SECTION_STREAM,
        "initial clock resync iterations",
        remote_clock_resync_iterations); // TODO: REMOVE FOR SLEW / GAPFILLING

    s->parent_using_h2o = appconfig_get_boolean(
        &stream_config, CONFIG_SECTION_STREAM, "parent using h2o", false);

    // initialize rrdpush globals
    rrdhost_flag_clear(s->host, RRDHOST_FLAG_RRDPUSH_SENDER_READY_4_METRICS);
    rrdhost_flag_clear(s->host, RRDHOST_FLAG_RRDPUSH_SENDER_CONNECTED);

    int pipe_buffer_size = 10 * 1024;
#ifdef F_GETPIPE_SZ
    pipe_buffer_size = fcntl(s->rrdpush_sender_pipe[PIPE_READ], F_GETPIPE_SZ);
#endif
    if(pipe_buffer_size < 10 * 1024)
        pipe_buffer_size = 10 * 1024;

    if(!rrdpush_sender_pipe_close(s->host, s->rrdpush_sender_pipe, true)) {
        netdata_log_error("STREAM %s [send]: cannot create inter-thread communication pipe. Disabling streaming.",
              rrdhost_hostname(s->host));
        return NULL;
    }

    struct rrdpush_sender_thread_data *thread_data = callocz(1, sizeof(struct rrdpush_sender_thread_data));
    thread_data->pipe_buffer = mallocz(pipe_buffer_size);
    thread_data->host = s->host;

    netdata_thread_cleanup_push(rrdpush_sender_thread_cleanup_callback, thread_data);

    size_t iterations = 0;
    time_t now_s = now_monotonic_sec();
    while(!rrdhost_sender_should_exit(s)) {
        iterations++;

        // The connection attempt blocks (after which we use the socket in nonblocking)
        if(unlikely(s->rrdpush_sender_socket == -1)) {
            worker_is_busy(WORKER_SENDER_JOB_CONNECT);

            now_s = now_monotonic_sec();
            rrdpush_sender_cbuffer_recreate_timed(s, now_s, false, true);
            execute_commands_cleanup(s);

            rrdhost_flag_clear(s->host, RRDHOST_FLAG_RRDPUSH_SENDER_READY_4_METRICS);
            s->flags &= ~SENDER_FLAG_OVERFLOW;
            s->read_len = 0;
            s->buffer->read = 0;
            s->buffer->write = 0;

            if(!attempt_to_connect(s))
                continue;

            if(rrdhost_sender_should_exit(s))
                break;

            now_s = s->last_traffic_seen_t = now_monotonic_sec();
            rrdpush_send_claimed_id(s->host);
            rrdpush_send_host_labels(s->host);
            rrdpush_send_global_functions(s->host);
            s->replication.oldest_request_after_t = 0;

            rrdhost_flag_set(s->host, RRDHOST_FLAG_RRDPUSH_SENDER_READY_4_METRICS);

            nd_log(NDLS_DAEMON, NDLP_DEBUG,
                   "STREAM %s [send to %s]: enabling metrics streaming...",
                   rrdhost_hostname(s->host), s->connected_to);

            continue;
        }

        if(iterations % 1000 == 0)
            now_s = now_monotonic_sec();

        // If the TCP window never opened then something is wrong, restart connection
        if(unlikely(now_s - s->last_traffic_seen_t > s->timeout &&
            !rrdpush_sender_pending_replication_requests(s) &&
            !rrdpush_sender_replicating_charts(s)
        )) {
            worker_is_busy(WORKER_SENDER_JOB_DISCONNECT_TIMEOUT);
            netdata_log_error("STREAM %s [send to %s]: could not send metrics for %d seconds - closing connection - we have sent %zu bytes on this connection via %zu send attempts.", rrdhost_hostname(s->host), s->connected_to, s->timeout, s->sent_bytes_on_this_connection, s->send_attempts);
            rrdpush_sender_thread_close_socket(s->host);
            continue;
        }

        sender_lock(s);
        size_t outstanding = cbuffer_next_unsafe(s->buffer, NULL);
        size_t available = cbuffer_available_size_unsafe(s->buffer);
        if (unlikely(!outstanding)) {
            rrdpush_sender_pipe_clear_pending_data(s);
            rrdpush_sender_cbuffer_recreate_timed(s, now_s, true, false);
        }

        if(s->compressor.initialized) {
            size_t bytes_uncompressed = s->compressor.sender_locked.total_uncompressed;
            size_t bytes_compressed = s->compressor.sender_locked.total_compressed + s->compressor.sender_locked.total_compressions * sizeof(rrdpush_signature_t);
            NETDATA_DOUBLE ratio = 100.0 - ((NETDATA_DOUBLE)bytes_compressed * 100.0 / (NETDATA_DOUBLE)bytes_uncompressed);
            worker_set_metric(WORKER_SENDER_JOB_BYTES_UNCOMPRESSED, (NETDATA_DOUBLE)bytes_uncompressed);
            worker_set_metric(WORKER_SENDER_JOB_BYTES_COMPRESSED, (NETDATA_DOUBLE)bytes_compressed);
            worker_set_metric(WORKER_SENDER_JOB_BYTES_COMPRESSION_RATIO, ratio);
        }
        sender_unlock(s);

        worker_set_metric(WORKER_SENDER_JOB_BUFFER_RATIO, (NETDATA_DOUBLE)(s->buffer->max_size - available) * 100.0 / (NETDATA_DOUBLE)s->buffer->max_size);

        if(outstanding)
            s->send_attempts++;

        if(unlikely(s->rrdpush_sender_pipe[PIPE_READ] == -1)) {
            if(!rrdpush_sender_pipe_close(s->host, s->rrdpush_sender_pipe, true)) {
                netdata_log_error("STREAM %s [send]: cannot create inter-thread communication pipe. Disabling streaming.",
                      rrdhost_hostname(s->host));
                rrdpush_sender_thread_close_socket(s->host);
                break;
            }
        }

        worker_is_idle();

        // Wait until buffer opens in the socket or a rrdset_done_push wakes us
        enum {
            Collector = 0,
            Socket    = 1,
        };
        struct pollfd fds[2] = {
            [Collector] = {
                .fd = s->rrdpush_sender_pipe[PIPE_READ],
                .events = POLLIN,
                .revents = 0,
            },
            [Socket] = {
                .fd = s->rrdpush_sender_socket,
                .events = POLLIN | (outstanding ? POLLOUT : 0 ),
                .revents = 0,
            }
        };

        int poll_rc = poll(fds, 2, 50); // timeout in milliseconds

        netdata_log_debug(D_STREAM, "STREAM: poll() finished collector=%d socket=%d (current chunk %zu bytes)...",
              fds[Collector].revents, fds[Socket].revents, outstanding);

        if(unlikely(rrdhost_sender_should_exit(s)))
            break;

        internal_error(fds[Collector].fd != s->rrdpush_sender_pipe[PIPE_READ],
            "STREAM %s [send to %s]: pipe changed after poll().", rrdhost_hostname(s->host), s->connected_to);

        internal_error(fds[Socket].fd != s->rrdpush_sender_socket,
            "STREAM %s [send to %s]: socket changed after poll().", rrdhost_hostname(s->host), s->connected_to);

        // Spurious wake-ups without error - loop again
        if (poll_rc == 0 || ((poll_rc == -1) && (errno == EAGAIN || errno == EINTR))) {
            netdata_thread_testcancel();
            netdata_log_debug(D_STREAM, "Spurious wakeup");
            now_s = now_monotonic_sec();
            continue;
        }

        // Only errors from poll() are internal, but try restarting the connection
        if(unlikely(poll_rc == -1)) {
            worker_is_busy(WORKER_SENDER_JOB_DISCONNECT_POLL_ERROR);
            netdata_log_error("STREAM %s [send to %s]: failed to poll(). Closing socket.", rrdhost_hostname(s->host), s->connected_to);
            rrdpush_sender_pipe_close(s->host, s->rrdpush_sender_pipe, true);
            rrdpush_sender_thread_close_socket(s->host);
            continue;
        }

         // If we have data and have seen the TCP window open then try to close it by a transmission.
        if(likely(outstanding && (fds[Socket].revents & POLLOUT))) {
            worker_is_busy(WORKER_SENDER_JOB_SOCKET_SEND);
            ssize_t bytes = attempt_to_send(s);
            if(bytes > 0) {
                s->last_traffic_seen_t = now_monotonic_sec();
                worker_set_metric(WORKER_SENDER_JOB_BYTES_SENT, (NETDATA_DOUBLE)bytes);
            }
        }

        // If the collector woke us up then empty the pipe to remove the signal
        if (fds[Collector].revents & (POLLIN|POLLPRI)) {
            worker_is_busy(WORKER_SENDER_JOB_PIPE_READ);
            netdata_log_debug(D_STREAM, "STREAM: Data added to send buffer (current buffer chunk %zu bytes)...", outstanding);

            if (read(fds[Collector].fd, thread_data->pipe_buffer, pipe_buffer_size) == -1)
                netdata_log_error("STREAM %s [send to %s]: cannot read from internal pipe.", rrdhost_hostname(s->host), s->connected_to);
        }

        // Read as much as possible to fill the buffer, split into full lines for execution.
        if (fds[Socket].revents & POLLIN) {
            worker_is_busy(WORKER_SENDER_JOB_SOCKET_RECEIVE);
            ssize_t bytes = attempt_read(s);
            if(bytes > 0) {
                s->last_traffic_seen_t = now_monotonic_sec();
                worker_set_metric(WORKER_SENDER_JOB_BYTES_RECEIVED, (NETDATA_DOUBLE)bytes);
            }
        }

        if(unlikely(s->read_len))
            execute_commands(s);

        if(unlikely(fds[Collector].revents & (POLLERR|POLLHUP|POLLNVAL))) {
            char *error = NULL;

            if (unlikely(fds[Collector].revents & POLLERR))
                error = "pipe reports errors (POLLERR)";
            else if (unlikely(fds[Collector].revents & POLLHUP))
                error = "pipe closed (POLLHUP)";
            else if (unlikely(fds[Collector].revents & POLLNVAL))
                error = "pipe is invalid (POLLNVAL)";

            if(error) {
                rrdpush_sender_pipe_close(s->host, s->rrdpush_sender_pipe, true);
                netdata_log_error("STREAM %s [send to %s]: restarting internal pipe: %s.",
                      rrdhost_hostname(s->host), s->connected_to, error);
            }
        }

        if(unlikely(fds[Socket].revents & (POLLERR|POLLHUP|POLLNVAL))) {
            char *error = NULL;

            if (unlikely(fds[Socket].revents & POLLERR))
                error = "socket reports errors (POLLERR)";
            else if (unlikely(fds[Socket].revents & POLLHUP))
                error = "connection closed by remote end (POLLHUP)";
            else if (unlikely(fds[Socket].revents & POLLNVAL))
                error = "connection is invalid (POLLNVAL)";

            if(unlikely(error)) {
                worker_is_busy(WORKER_SENDER_JOB_DISCONNECT_SOCKET_ERROR);
                netdata_log_error("STREAM %s [send to %s]: restarting connection: %s - %zu bytes transmitted.",
                      rrdhost_hostname(s->host), s->connected_to, error, s->sent_bytes_on_this_connection);
                rrdpush_sender_thread_close_socket(s->host);
            }
        }

        // protection from overflow
        if(unlikely(s->flags & SENDER_FLAG_OVERFLOW)) {
            worker_is_busy(WORKER_SENDER_JOB_DISCONNECT_OVERFLOW);
            errno = 0;
            netdata_log_error("STREAM %s [send to %s]: buffer full (allocated %zu bytes) after sending %zu bytes. Restarting connection",
                  rrdhost_hostname(s->host), s->connected_to, s->buffer->size, s->sent_bytes_on_this_connection);
            rrdpush_sender_thread_close_socket(s->host);
        }

        worker_set_metric(WORKER_SENDER_JOB_REPLAY_DICT_SIZE, (NETDATA_DOUBLE) dictionary_entries(s->replication.requests));
    }

    netdata_thread_cleanup_pop(1);
    return NULL;
}
