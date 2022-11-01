// SPDX-License-Identifier: GPL-3.0-or-later

#include "rrdpush.h"
#include "parser/parser.h"

#define WORKER_SENDER_JOB_CONNECT                    0
#define WORKER_SENDER_JOB_PIPE_READ                  1
#define WORKER_SENDER_JOB_SOCKET_RECEIVE             2
#define WORKER_SENDER_JOB_EXECUTE                    3
#define WORKER_SENDER_JOB_SOCKET_SEND                4
#define WORKER_SENDER_JOB_DISCONNECT_BAD_HANDSHAKE   5
#define WORKER_SENDER_JOB_DISCONNECT_OVERFLOW        6
#define WORKER_SENDER_JOB_DISCONNECT_TIMEOUT         7
#define WORKER_SENDER_JOB_DISCONNECT_POLL_ERROR      8
#define WORKER_SENDER_JOB_DISCONNECT_SOCKER_ERROR    9
#define WORKER_SENDER_JOB_DISCONNECT_SSL_ERROR      10
#define WORKER_SENDER_JOB_DISCONNECT_PARENT_CLOSED  11
#define WORKER_SENDER_JOB_DISCONNECT_RECEIVE_ERROR  12
#define WORKER_SENDER_JOB_DISCONNECT_SEND_ERROR     13
#define WORKER_SENDER_JOB_DISCONNECT_NO_COMPRESSION 14
#define WORKER_SENDER_JOB_BUFFER_RATIO              15
#define WORKER_SENDER_JOB_BYTES_RECEIVED            16
#define WORKER_SENDER_JOB_BYTES_SENT                17

#if WORKER_UTILIZATION_MAX_JOB_TYPES < 18
#error WORKER_UTILIZATION_MAX_JOB_TYPES has to be at least 18
#endif

extern struct config stream_config;
extern int netdata_use_ssl_on_stream;
extern char *netdata_ssl_ca_path;
extern char *netdata_ssl_ca_file;

static __thread BUFFER *sender_thread_buffer = NULL;
static __thread bool sender_thread_buffer_used = false;

void sender_thread_buffer_free(void) {
    if(sender_thread_buffer) {
        buffer_free(sender_thread_buffer);
        sender_thread_buffer = NULL;
    }
}

// Collector thread starting a transmission
BUFFER *sender_start(struct sender_state *s __maybe_unused) {
    if(!sender_thread_buffer)
        sender_thread_buffer = buffer_create(1024);

    if(sender_thread_buffer_used)
        fatal("STREAMING: thread buffer is used multiple times concurrently.");

    sender_thread_buffer_used = true;
    buffer_flush(sender_thread_buffer);
    return sender_thread_buffer;
}

void sender_cancel(struct sender_state *s __maybe_unused) {
    sender_thread_buffer_used = false;
}

static inline void rrdpush_sender_thread_close_socket(RRDHOST *host);

#ifdef ENABLE_COMPRESSION
/*
* In case of stream compression buffer oveflow
* Inform the user through the error log file and 
* deactivate compression by downgrading the stream protocol.
*/
static inline void deactivate_compression(struct sender_state *s) {
    worker_is_busy(WORKER_SENDER_JOB_DISCONNECT_NO_COMPRESSION);
    error("STREAM_COMPRESSION: Compression returned error, disabling it.");
    s->flags &= ~SENDER_FLAG_COMPRESSION;
    error("STREAM %s [send to %s]: Restarting connection without compression.", rrdhost_hostname(s->host), s->connected_to);
    rrdpush_sender_thread_close_socket(s->host);
}
#endif

// Collector thread finishing a transmission
void sender_commit(struct sender_state *s, BUFFER *wb) {

    if(unlikely(wb != sender_thread_buffer))
        fatal("STREAMING: sender is trying to commit a buffer that is not this thread's buffer.");

    if(unlikely(!sender_thread_buffer_used))
        fatal("STREAMING: sender is committing a buffer twice.");

    sender_thread_buffer_used = false;

    char *src = (char *)buffer_tostring(wb);
    size_t src_len = buffer_strlen(wb);

    if(unlikely(!src || !src_len))
        return;

    netdata_mutex_lock(&s->mutex);

#ifdef ENABLE_COMPRESSION
    if (s->flags & SENDER_FLAG_COMPRESSION && s->compressor) {
        while(src_len) {
            size_t size_to_compress = src_len;

            if(size_to_compress > COMPRESSION_MAX_MSG_SIZE) {
                // we need to find the last newline
                // so that the decompressor will have a whole line to work with

                const char *t = &src[COMPRESSION_MAX_MSG_SIZE - 1];
                while(t-- > src)
                    if(*t == '\n')
                        break;

                if(t == src)
                    size_to_compress = COMPRESSION_MAX_MSG_SIZE;
                else
                    size_to_compress = t - src + 1;
            }

            char *dst;
            size_t dst_len = s->compressor->compress(s->compressor, src, size_to_compress, &dst);
            if (!dst_len) {
                error("STREAM %s [send to %s]: compression failed. Resetting compressor and re-trying",
                      rrdhost_hostname(s->host), s->connected_to);

                s->compressor->reset(s->compressor);
                dst_len = s->compressor->compress(s->compressor, src, size_to_compress, &dst);
                if(!dst_len) {
                    error("STREAM %s [send to %s]: compression failed again. Deactivating compression",
                          rrdhost_hostname(s->host), s->connected_to);

                    deactivate_compression(s);
                    netdata_mutex_unlock(&s->mutex);
                    return;
                }
            }

            if(cbuffer_add_unsafe(s->host->sender->buffer, dst, dst_len))
                s->flags |= SENDER_FLAG_OVERFLOW;

            src = src + size_to_compress;
            src_len -= size_to_compress;
        }
    }
    else if(cbuffer_add_unsafe(s->host->sender->buffer, src, src_len))
        s->flags |= SENDER_FLAG_OVERFLOW;
#else
    if(cbuffer_add_unsafe(s->host->sender->buffer, src, src_len))
        s->flags |= SENDER_FLAG_OVERFLOW;
#endif

    netdata_mutex_unlock(&s->mutex);
    rrdpush_signal_sender_to_wake_up(s);
}


static inline void rrdpush_sender_thread_close_socket(RRDHOST *host) {
    rrdhost_flag_clear(host, RRDHOST_FLAG_RRDPUSH_SENDER_READY_4_METRICS);
    rrdhost_flag_clear(host, RRDHOST_FLAG_RRDPUSH_SENDER_CONNECTED);

    if(host->sender->rrdpush_sender_socket != -1) {
        close(host->sender->rrdpush_sender_socket);
        host->sender->rrdpush_sender_socket = -1;
    }
}

static inline void rrdpush_sender_add_host_variable_to_buffer(BUFFER *wb, const RRDVAR_ACQUIRED *rva) {
    buffer_sprintf(
            wb
            , "VARIABLE HOST %s = " NETDATA_DOUBLE_FORMAT "\n"
            , rrdvar_name(rva)
            , rrdvar2number(rva)
    );

    debug(D_STREAM, "RRDVAR pushed HOST VARIABLE %s = " NETDATA_DOUBLE_FORMAT, rrdvar_name(rva), rrdvar2number(rva));
}

void rrdpush_sender_send_this_host_variable_now(RRDHOST *host, const RRDVAR_ACQUIRED *rva) {
    if(rrdhost_can_send_definitions_to_parent(host)) {
        BUFFER *wb = sender_start(host->sender);
        rrdpush_sender_add_host_variable_to_buffer(wb, rva);
        sender_commit(host->sender, wb);
    }
}

struct custom_host_variables_callback {
    BUFFER *wb;
};

static int rrdpush_sender_thread_custom_host_variables_callback(const DICTIONARY_ITEM *item __maybe_unused, void *rrdvar_ptr __maybe_unused, void *struct_ptr) {
    const RRDVAR_ACQUIRED *rv = (const RRDVAR_ACQUIRED *)item;
    struct custom_host_variables_callback *tmp = struct_ptr;
    BUFFER *wb = tmp->wb;

    if(unlikely(rrdvar_flags(rv) & RRDVAR_FLAG_CUSTOM_HOST_VAR && rrdvar_type(rv) == RRDVAR_TYPE_CALCULATED)) {
        rrdpush_sender_add_host_variable_to_buffer(wb, rv);
        return 1;
    }
    return 0;
}

static void rrdpush_sender_thread_send_custom_host_variables(RRDHOST *host) {
    if(rrdhost_can_send_definitions_to_parent(host)) {
        BUFFER *wb = sender_start(host->sender);
        struct custom_host_variables_callback tmp = {
            .wb = wb
        };
        int ret = rrdvar_walkthrough_read(host->rrdvars, rrdpush_sender_thread_custom_host_variables_callback, &tmp);
        (void)ret;
        sender_commit(host->sender, wb);

        debug(D_STREAM, "RRDVAR sent %d VARIABLES", ret);
    }
}

// resets all the chart, so that their definitions
// will be resent to the central netdata
static void rrdpush_sender_thread_reset_all_charts(RRDHOST *host) {
    RRDSET *st;
    rrdset_foreach_read(st, host) {
        rrdset_flag_clear(st, RRDSET_FLAG_UPSTREAM_EXPOSED);

        st->upstream_resync_time = 0;

        RRDDIM *rd;
        rrddim_foreach_read(rd, st)
            rd->exposed = 0;
        rrddim_foreach_done(rd);
    }
    rrdset_foreach_done(st);
}

static inline void rrdpush_sender_thread_data_flush(RRDHOST *host) {
    netdata_mutex_lock(&host->sender->mutex);
    cbuffer_flush(host->sender->buffer);
    netdata_mutex_unlock(&host->sender->mutex);

    rrdpush_sender_thread_reset_all_charts(host);
    rrdpush_sender_thread_send_custom_host_variables(host);
}

void rrdpush_encode_variable(stream_encoded_t *se, RRDHOST *host)
{
    se->os_name = (host->system_info->host_os_name)?url_encode(host->system_info->host_os_name):"";
    se->os_id = (host->system_info->host_os_id)?url_encode(host->system_info->host_os_id):"";
    se->os_version = (host->system_info->host_os_version)?url_encode(host->system_info->host_os_version):"";
    se->kernel_name = (host->system_info->kernel_name)?url_encode(host->system_info->kernel_name):"";
    se->kernel_version = (host->system_info->kernel_version)?url_encode(host->system_info->kernel_version):"";
}

void rrdpush_clean_encoded(stream_encoded_t *se)
{
    if (se->os_name)
        freez(se->os_name);

    if (se->os_id)
        freez(se->os_id);

    if (se->os_version)
        freez(se->os_version);

    if (se->kernel_name)
        freez(se->kernel_name);

    if (se->kernel_version)
        freez(se->kernel_version);
}

struct {
    const char *response;
    size_t length;
    int32_t version;
    bool dynamic;
    const char *error;
    int worker_job_id;
    time_t postpone_reconnect_seconds;
} stream_responses[] = {
    {
        .response = START_STREAMING_PROMPT_VN,
        .length = sizeof(START_STREAMING_PROMPT_VN) - 1,
        .version = STREAM_HANDSHAKE_OK_V3, // and above
        .dynamic = true,                 // dynamic = we will parse the version / capabilities
        .error = NULL,
        .worker_job_id = 0,
        .postpone_reconnect_seconds = 0,
    },
    {
        .response = START_STREAMING_PROMPT_V2,
        .length = sizeof(START_STREAMING_PROMPT_V2) - 1,
        .version = STREAM_HANDSHAKE_OK_V2,
        .dynamic = false,
        .error = NULL,
        .worker_job_id = 0,
        .postpone_reconnect_seconds = 0,
    },
    {
        .response = START_STREAMING_PROMPT_V1,
        .length = sizeof(START_STREAMING_PROMPT_V1) - 1,
        .version = STREAM_HANDSHAKE_OK_V1,
        .dynamic = false,
        .error = NULL,
        .worker_job_id = 0,
        .postpone_reconnect_seconds = 0,
    },
    {
        .response = START_STREAMING_ERROR_SAME_LOCALHOST,
        .length = sizeof(START_STREAMING_ERROR_SAME_LOCALHOST) - 1,
        .version = STREAM_HANDSHAKE_ERROR_LOCALHOST,
        .dynamic = false,
        .error = "remote server rejected this stream, the host we are trying to stream is its localhost",
        .worker_job_id = WORKER_SENDER_JOB_DISCONNECT_BAD_HANDSHAKE,
        .postpone_reconnect_seconds = 60 * 60, // the IP may change, try it every hour
    },
    {
        .response = START_STREAMING_ERROR_ALREADY_STREAMING,
        .length = sizeof(START_STREAMING_ERROR_ALREADY_STREAMING) - 1,
        .version = STREAM_HANDSHAKE_ERROR_ALREADY_CONNECTED,
        .dynamic = false,
        .error = "remote server rejected this stream, the host we are trying to stream is already streamed to it",
        .worker_job_id = WORKER_SENDER_JOB_DISCONNECT_BAD_HANDSHAKE,
        .postpone_reconnect_seconds = 1 * 60, // 1 minute
    },
    {
        .response = START_STREAMING_ERROR_NOT_PERMITTED,
        .length = sizeof(START_STREAMING_ERROR_NOT_PERMITTED) - 1,
        .version = STREAM_HANDSHAKE_ERROR_DENIED,
        .dynamic = false,
        .error = "remote server denied access, probably we don't have the right API key?",
        .worker_job_id = WORKER_SENDER_JOB_DISCONNECT_BAD_HANDSHAKE,
        .postpone_reconnect_seconds = 1 * 60, // 1 minute
    },

    // terminator
    {
        .response = NULL,
        .length = 0,
        .version = STREAM_HANDSHAKE_ERROR_BAD_HANDSHAKE,
        .dynamic = false,
        .error = "remote node response is not understood, is it Netdata?",
        .worker_job_id = WORKER_SENDER_JOB_DISCONNECT_BAD_HANDSHAKE,
        .postpone_reconnect_seconds = 1 * 60, // 1 minute
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
    const char *error = stream_responses[i].error;
    int worker_job_id = stream_responses[i].worker_job_id;
    time_t delay = stream_responses[i].postpone_reconnect_seconds;

    if(version >= STREAM_HANDSHAKE_OK_V1) {
        host->destination->last_error = NULL;
        host->destination->last_handshake = version;
        host->destination->postpone_reconnection_until = 0;
        s->capabilities = convert_stream_version_to_capabilities(version);
        return true;
    }

    error("STREAM %s [send to %s]: %s.", rrdhost_hostname(host), s->connected_to, error);

    worker_is_busy(worker_job_id);
    rrdpush_sender_thread_close_socket(host);
    host->destination->last_error = error;
    host->destination->last_handshake = version;
    host->destination->postpone_reconnection_until = now_realtime_sec() + delay;
    return false;
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
        error("STREAM %s [send to %s]: could not connect to parent node at this time.", rrdhost_hostname(host), host->rrdpush_send_destination);
        return false;
    }

    info("STREAM %s [send to %s]: initializing communication...", rrdhost_hostname(host), s->connected_to);

#ifdef ENABLE_HTTPS
    if(netdata_ssl_client_ctx){
        host->sender->ssl.flags = NETDATA_SSL_START;
        if (!host->sender->ssl.conn){
            host->sender->ssl.conn = SSL_new(netdata_ssl_client_ctx);
            if(!host->sender->ssl.conn){
                error("Failed to allocate SSL structure.");
                host->sender->ssl.flags = NETDATA_SSL_NO_HANDSHAKE;
            }
        }
        else{
            SSL_clear(host->sender->ssl.conn);
        }

        if (host->sender->ssl.conn)
        {
            if (SSL_set_fd(host->sender->ssl.conn, s->rrdpush_sender_socket) != 1) {
                error("Failed to set the socket to the SSL on socket fd %d.", s->rrdpush_sender_socket);
                host->sender->ssl.flags = NETDATA_SSL_NO_HANDSHAKE;
            } else{
                host->sender->ssl.flags = NETDATA_SSL_HANDSHAKE_COMPLETE;
            }
        }
    }
    else {
        host->sender->ssl.flags = NETDATA_SSL_NO_HANDSHAKE;
    }
#endif

    // reset our capabilities to default
    s->capabilities = STREAM_OUR_CAPABILITIES;

#ifdef  ENABLE_COMPRESSION
    // If we don't want compression, remove it from our capabilities
    if(!(s->flags & SENDER_FLAG_COMPRESSION) && stream_has_capability(s, STREAM_CAP_COMPRESSION))
        s->capabilities &= ~STREAM_CAP_COMPRESSION;
#endif  // ENABLE_COMPRESSION

    /* TODO: During the implementation of #7265 switch the set of variables to HOST_* and CONTAINER_* if the
             version negotiation resulted in a high enough version.
    */
    stream_encoded_t se;
    rrdpush_encode_variable(&se, host);

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
                 " HTTP/1.1\r\n"
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
                 , host->system_info->hops + 1
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

#ifdef ENABLE_HTTPS
    if (!host->sender->ssl.flags) {
        ERR_clear_error();
        SSL_set_connect_state(host->sender->ssl.conn);
        int err = SSL_connect(host->sender->ssl.conn);
        if (err != 1){
            err = SSL_get_error(host->sender->ssl.conn, err);
            error("SSL cannot connect with the server:  %s ",ERR_error_string((long)SSL_get_error(host->sender->ssl.conn,err),NULL));
            if (netdata_use_ssl_on_stream == NETDATA_SSL_FORCE) {
                worker_is_busy(WORKER_SENDER_JOB_DISCONNECT_SSL_ERROR);
                rrdpush_sender_thread_close_socket(host);
                host->destination->last_error = "SSL error";
                host->destination->last_handshake = STREAM_HANDSHAKE_ERROR_SSL_ERROR;
                host->destination->postpone_reconnection_until = now_realtime_sec() + 5 * 60;
                return false;
            }
            else {
                host->sender->ssl.flags = NETDATA_SSL_NO_HANDSHAKE;
            }
        }
        else {
            if (netdata_use_ssl_on_stream == NETDATA_SSL_FORCE) {
                if (netdata_ssl_validate_server == NETDATA_SSL_VALID_CERTIFICATE) {
                    if ( security_test_certificate(host->sender->ssl.conn)) {
                        worker_is_busy(WORKER_SENDER_JOB_DISCONNECT_SSL_ERROR);
                        error("Closing the stream connection, because the server SSL certificate is not valid.");
                        rrdpush_sender_thread_close_socket(host);
                        host->destination->last_error = "invalid SSL certificate";
                        host->destination->last_handshake = STREAM_HANDSHAKE_ERROR_INVALID_CERTIFICATE;
                        host->destination->postpone_reconnection_until = now_realtime_sec() + 5 * 60;
                        return false;
                    }
                }
            }
        }
    }
#endif

    ssize_t bytes;

    bytes = send_timeout(
#ifdef ENABLE_HTTPS
        &host->sender->ssl,
#endif
        s->rrdpush_sender_socket,
        http,
        strlen(http),
        0,
        timeout);

    if(bytes <= 0) { // timeout is 0
        worker_is_busy(WORKER_SENDER_JOB_DISCONNECT_TIMEOUT);
        rrdpush_sender_thread_close_socket(host);
        error("STREAM %s [send to %s]: failed to send HTTP header to remote netdata.", rrdhost_hostname(host), s->connected_to);
        host->destination->last_error = "timeout while sending request";
        host->destination->last_handshake = STREAM_HANDSHAKE_ERROR_SEND_TIMEOUT;
        host->destination->postpone_reconnection_until = now_realtime_sec() + 1 * 60;
        return false;
    }

    info("STREAM %s [send to %s]: waiting response from remote netdata...", rrdhost_hostname(host), s->connected_to);

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
        worker_is_busy(WORKER_SENDER_JOB_DISCONNECT_TIMEOUT);
        rrdpush_sender_thread_close_socket(host);
        error("STREAM %s [send to %s]: remote netdata does not respond.", rrdhost_hostname(host), s->connected_to);
        host->destination->last_error = "timeout while expecting first response";
        host->destination->last_handshake = STREAM_HANDSHAKE_ERROR_RECEIVE_TIMEOUT;
        host->destination->postpone_reconnection_until = now_realtime_sec() + 30;
        return false;
    }

    http[bytes] = '\0';
    debug(D_STREAM, "Response to sender from far end: %s", http);
    if(!rrdpush_sender_validate_response(host, s, http, bytes))
        return false;

#ifdef ENABLE_COMPRESSION
    // if the stream does not have compression capability,
    // shut it down for us too.
    // FIXME - this means that if there are multiple parents and one of them does not support compression
    //         we are going to shut it down for all of them eventually...
    if(!stream_has_capability(s, STREAM_CAP_COMPRESSION))
        s->flags &= ~SENDER_FLAG_COMPRESSION;

    if(s->flags & SENDER_FLAG_COMPRESSION) {
        if(s->compressor)
            s->compressor->reset(s->compressor);
    }
    else
        info("STREAM %s [send to %s]: compression is disabled on this connection.", rrdhost_hostname(host), s->connected_to);

#endif  //ENABLE_COMPRESSION

    log_sender_capabilities(s);

    if(sock_setnonblock(s->rrdpush_sender_socket) < 0)
        error("STREAM %s [send to %s]: cannot set non-blocking mode for socket.", rrdhost_hostname(host), s->connected_to);

    if(sock_enlarge_out(s->rrdpush_sender_socket) < 0)
        error("STREAM %s [send to %s]: cannot enlarge the socket buffer.", rrdhost_hostname(host), s->connected_to);

    debug(D_STREAM, "STREAM: Connected on fd %d...", s->rrdpush_sender_socket);

    return true;
}

static bool attempt_to_connect(struct sender_state *state)
{
    state->send_attempts = 0;

    if(rrdpush_sender_thread_connect_to_parent(state->host, state->default_port, state->timeout, state)) {
        state->last_sent_t = now_monotonic_sec();

        // reset the buffer, to properly send charts and metrics
        rrdpush_sender_thread_data_flush(state->host);

        // send from the beginning
        state->begin = 0;

        // make sure the next reconnection will be immediate
        state->not_connected_loops = 0;

        // reset the bytes we have sent for this session
        state->sent_bytes_on_this_connection = 0;

        // let the data collection threads know we are ready
        rrdhost_flag_set(state->host, RRDHOST_FLAG_RRDPUSH_SENDER_CONNECTED);

        return true;
    }

    // we couldn't connect

    // increase the failed connections counter
    state->not_connected_loops++;

    // reset the number of bytes sent
    state->sent_bytes_on_this_connection = 0;

    // slow re-connection on repeating errors
    sleep_usec(USEC_PER_SEC * state->reconnect_delay); // seconds

    return false;
}

// TCP window is open and we have data to transmit.
static ssize_t attempt_to_send(struct sender_state *s) {
    ssize_t ret = 0;

#ifdef NETDATA_INTERNAL_CHECKS
    struct circular_buffer *cb = s->buffer;
#endif

    netdata_mutex_lock(&s->mutex);
    char *chunk;
    size_t outstanding = cbuffer_next_unsafe(s->buffer, &chunk);
    debug(D_STREAM, "STREAM: Sending data. Buffer r=%zu w=%zu s=%zu, next chunk=%zu", cb->read, cb->write, cb->size, outstanding);

#ifdef ENABLE_HTTPS
    SSL *conn = s->host->sender->ssl.conn ;
    if(conn && s->host->sender->ssl.flags == NETDATA_SSL_HANDSHAKE_COMPLETE)
        ret = SSL_write(conn, chunk, outstanding);
    else
        ret = send(s->rrdpush_sender_socket, chunk, outstanding, MSG_DONTWAIT);
#else
    ret = send(s->rrdpush_sender_socket, chunk, outstanding, MSG_DONTWAIT);
#endif

    if (likely(ret > 0)) {
        cbuffer_remove_unsafe(s->buffer, ret);
        s->sent_bytes_on_this_connection += ret;
        s->sent_bytes += ret;
        debug(D_STREAM, "STREAM %s [send to %s]: Sent %zd bytes", rrdhost_hostname(s->host), s->connected_to, ret);
        s->last_sent_t = now_monotonic_sec();
    }
    else if (ret == -1 && (errno == EAGAIN || errno == EINTR || errno == EWOULDBLOCK))
        debug(D_STREAM, "STREAM %s [send to %s]: unavailable after polling POLLOUT", rrdhost_hostname(s->host), s->connected_to);
    else if (ret == -1) {
        worker_is_busy(WORKER_SENDER_JOB_DISCONNECT_SEND_ERROR);
        debug(D_STREAM, "STREAM: Send failed - closing socket...");
        error("STREAM %s [send to %s]: failed to send metrics - closing connection - we have sent %zu bytes on this connection.",  rrdhost_hostname(s->host), s->connected_to, s->sent_bytes_on_this_connection);
        rrdpush_sender_thread_close_socket(s->host);
    }
    else
        debug(D_STREAM, "STREAM: send() returned 0 -> no error but no transmission");

    netdata_mutex_unlock(&s->mutex);

    return ret;
}

static ssize_t attempt_read(struct sender_state *s) {
    ssize_t ret = 0;

#ifdef ENABLE_HTTPS
    if (s->host->sender->ssl.conn && s->host->sender->ssl.flags == NETDATA_SSL_HANDSHAKE_COMPLETE) {
        ERR_clear_error();
        int desired = sizeof(s->read_buffer) - s->read_len - 1;
        ret = SSL_read(s->host->sender->ssl.conn, s->read_buffer, desired);
        if (ret > 0 ) {
            s->read_len += ret;
            return ret;
        }
        int sslerrno = SSL_get_error(s->host->sender->ssl.conn, desired);
        if (sslerrno == SSL_ERROR_WANT_READ || sslerrno == SSL_ERROR_WANT_WRITE)
            return ret;

        worker_is_busy(WORKER_SENDER_JOB_DISCONNECT_SSL_ERROR);
        u_long err;
        char buf[256];
        while ((err = ERR_get_error()) != 0) {
            ERR_error_string_n(err, buf, sizeof(buf));
            error("STREAM %s [send to %s] SSL error: %s", rrdhost_hostname(s->host), s->connected_to, buf);
        }
        rrdpush_sender_thread_close_socket(s->host);
        return ret;
    }
#endif
    ret = recv(s->rrdpush_sender_socket, s->read_buffer + s->read_len, sizeof(s->read_buffer) - s->read_len - 1,MSG_DONTWAIT);
    if (ret > 0) {
        s->read_len += ret;
        return ret;
    }

    if (ret < 0 && (errno == EAGAIN || errno == EWOULDBLOCK || errno == EINTR))
        return ret;

    if (ret == 0 || errno == ECONNRESET) {
        worker_is_busy(WORKER_SENDER_JOB_DISCONNECT_PARENT_CLOSED);
        error("STREAM %s [send to %s]: connection closed by far end.", rrdhost_hostname(s->host), s->connected_to);
    }
    else {
        worker_is_busy(WORKER_SENDER_JOB_DISCONNECT_RECEIVE_ERROR);
        error("STREAM %s [send to %s]: error during receive (%zd) - closing connection.", rrdhost_hostname(s->host), s->connected_to, ret);
    }
    rrdpush_sender_thread_close_socket(s->host);

    return ret;
}

struct inflight_stream_function {
    struct sender_state *sender;
    STRING *transaction;
    usec_t received_ut;
};

void stream_execute_function_callback(BUFFER *func_wb, int code, void *data) {
    struct inflight_stream_function *tmp = data;

    struct sender_state *s = tmp->sender;

    if(rrdhost_can_send_definitions_to_parent(s->host)) {
        BUFFER *wb = sender_start(s);

        pluginsd_function_result_begin_to_buffer(wb
                                                 , string2str(tmp->transaction)
                                                 , code
                                                 , functions_content_type_to_format(func_wb->contenttype)
                                                 , func_wb->expires);

        buffer_fast_strcat(wb, buffer_tostring(func_wb), buffer_strlen(func_wb));
        pluginsd_function_result_end_to_buffer(wb);

        sender_commit(s, wb);

        internal_error(true, "STREAM %s [send to %s] FUNCTION transaction %s sending back response (%zu bytes, %llu usec).",
                       rrdhost_hostname(s->host), s->connected_to,
                       string2str(tmp->transaction),
                       buffer_strlen(func_wb),
                       now_realtime_usec() - tmp->received_ut);
    }
    string_freez(tmp->transaction);
    buffer_free(func_wb);
    freez(tmp);
}

// This is just a placeholder until the gap filling state machine is inserted
void execute_commands(struct sender_state *s) {
    char *start = s->read_buffer, *end = &s->read_buffer[s->read_len], *newline;
    *end = 0;
    while( start < end && (newline = strchr(start, '\n')) ) {
        *newline = '\0';

        log_access("STREAM: %d from '%s' for host '%s': %s",
                   gettid(), s->connected_to, rrdhost_hostname(s->host), start);

        internal_error(true, "STREAM %s [send to %s] received command over connection: %s", rrdhost_hostname(s->host), s->connected_to, start);

        char *words[PLUGINSD_MAX_WORDS] = { NULL };
        pluginsd_split_words(start, words, PLUGINSD_MAX_WORDS, NULL, NULL, 0);

        if(words[0] && strcmp(words[0], PLUGINSD_KEYWORD_FUNCTION) == 0) {
            char *transaction = words[1];
            char *timeout_s = words[2];
            char *function = words[3];

            if(!transaction || !*transaction || !timeout_s || !*timeout_s || !function || !*function) {
                error("STREAM %s [send to %s] %s execution command is incomplete (transaction = '%s', timeout = '%s', function = '%s'). Ignoring it.",
                      rrdhost_hostname(s->host), s->connected_to,
                      words[0],
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
                BUFFER *wb = buffer_create(PLUGINSD_LINE_MAX + 1);

                int code = rrd_call_function_async(s->host, wb, timeout, function, stream_execute_function_callback, tmp);
                if(code != HTTP_RESP_OK) {
                    rrd_call_function_error(wb, "Failed to route request to collector", code);
                    stream_execute_function_callback(wb, code, tmp);
                }
            }
        }
        else
            error("STREAM %s [send to %s] received unknown command over connection: %s", rrdhost_hostname(s->host), s->connected_to, words[0]?words[0]:"(unset)");

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
    struct sender_state *sender_state;
    RRDHOST *host;
    DICTFE dictfe;
    enum {
        SENDING_DEFINITIONS_RESTART,
        SENDING_DEFINITIONS_CONTINUE,
        SENDING_DEFINITIONS_DONE,
    } sending_definitions_status;
    char *pipe_buffer;
};

static size_t cbuffer_available_bytes_with_lock(struct rrdpush_sender_thread_data *thread_data) {
    netdata_mutex_lock(&thread_data->sender_state->mutex);
    size_t outstanding = cbuffer_available_size_unsafe(thread_data->sender_state->host->sender->buffer);
    netdata_mutex_unlock(&thread_data->sender_state->mutex);
    return outstanding;
}

static void rrdpush_queue_incremental_definitions(struct rrdpush_sender_thread_data *thread_data) {

    while(rrdhost_flag_check(thread_data->host, RRDHOST_FLAG_RRDPUSH_SENDER_CONNECTED)
           && thread_data->sending_definitions_status != SENDING_DEFINITIONS_DONE
           && cbuffer_available_bytes_with_lock(thread_data) > (thread_data->sender_state->buffer->max_size / 2)) {

        if(thread_data->sending_definitions_status == SENDING_DEFINITIONS_RESTART)
            info("STREAM %s [send to %s]: sending metric definitions...", rrdhost_hostname(thread_data->host), thread_data->sender_state->connected_to);

        bool more_defs_available = rrdpush_incremental_transmission_of_chart_definitions(
            thread_data->sender_state->host, &thread_data->dictfe,
            thread_data->sending_definitions_status == SENDING_DEFINITIONS_RESTART, false);

        if (unlikely(!more_defs_available)) {
            thread_data->sending_definitions_status = SENDING_DEFINITIONS_DONE;
            info("STREAM %s [send to %s]: sending metric definitions finished.", rrdhost_hostname(thread_data->host), thread_data->sender_state->connected_to);
        }
        else
            thread_data->sending_definitions_status = SENDING_DEFINITIONS_CONTINUE;
    }
}

static bool rrdpush_sender_pipe_close(RRDHOST *host, int *pipe_fds, bool reopen) {
    static netdata_mutex_t mutex = NETDATA_MUTEX_INITIALIZER;

    bool ret = true;

    netdata_mutex_lock(&mutex);

    int new_pipe_fds[2];
    if(reopen) {
        if(pipe(new_pipe_fds) != 0) {
            error("STREAM %s [send]: cannot create required pipe.", rrdhost_hostname(host));
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
        error("STREAM %s [send]: cannot write to internal pipe.", rrdhost_hostname(host));
        rrdpush_sender_pipe_close(host, s->rrdpush_sender_pipe, true);
    }
}

static void rrdpush_sender_thread_cleanup_callback(void *ptr) {
    struct rrdpush_sender_thread_data *data = ptr;
    worker_unregister();

    RRDHOST *host = data->host;

    rrdpush_incremental_transmission_of_chart_definitions(host, &data->dictfe, false, true);

    netdata_mutex_lock(&host->sender->mutex);

    info("STREAM %s [send]: sending thread cleans up...", rrdhost_hostname(host));

    rrdpush_sender_thread_close_socket(host);
    rrdpush_sender_pipe_close(host, host->sender->rrdpush_sender_pipe, false);

    if(!rrdhost_flag_check(host, RRDHOST_FLAG_RRDPUSH_SENDER_JOIN)) {
        info("STREAM %s [send]: sending thread detaches itself.", rrdhost_hostname(host));
        netdata_thread_detach(netdata_thread_self());
    }

    rrdhost_flag_clear(host, RRDHOST_FLAG_RRDPUSH_SENDER_SPAWN);

    info("STREAM %s [send]: sending thread now exits.", rrdhost_hostname(host));

    netdata_mutex_unlock(&host->sender->mutex);

    freez(data->pipe_buffer);
    freez(data);
}

void sender_init(RRDHOST *parent)
{
    if (parent->sender)
        return;

    parent->sender = callocz(1, sizeof(*parent->sender));
    parent->sender->host = parent;
    parent->sender->buffer = cbuffer_new(1024, 1024*1024);
    parent->sender->capabilities = STREAM_OUR_CAPABILITIES;

    parent->sender->rrdpush_sender_pipe[PIPE_READ] = -1;
    parent->sender->rrdpush_sender_pipe[PIPE_WRITE] = -1;
    parent->sender->rrdpush_sender_socket  = -1;

#ifdef ENABLE_COMPRESSION
    if(default_compression_enabled) {
        parent->sender->flags |= SENDER_FLAG_COMPRESSION;
        parent->sender->compressor = create_compressor();
    }
#endif

    netdata_mutex_init(&parent->sender->mutex);
}

void *rrdpush_sender_thread(void *ptr) {
    worker_register("STREAMSND");
    worker_register_job_name(WORKER_SENDER_JOB_CONNECT, "connect");
    worker_register_job_name(WORKER_SENDER_JOB_PIPE_READ, "pipe read");
    worker_register_job_name(WORKER_SENDER_JOB_SOCKET_RECEIVE, "receive");
    worker_register_job_name(WORKER_SENDER_JOB_EXECUTE, "execute");
    worker_register_job_name(WORKER_SENDER_JOB_SOCKET_SEND, "send");

    // disconnection reasons
    worker_register_job_name(WORKER_SENDER_JOB_DISCONNECT_TIMEOUT, "disconnect timeout");
    worker_register_job_name(WORKER_SENDER_JOB_DISCONNECT_POLL_ERROR, "disconnect poll error");
    worker_register_job_name(WORKER_SENDER_JOB_DISCONNECT_SOCKER_ERROR, "disconnect socket error");
    worker_register_job_name(WORKER_SENDER_JOB_DISCONNECT_OVERFLOW, "disconnect overflow");
    worker_register_job_name(WORKER_SENDER_JOB_DISCONNECT_SSL_ERROR, "disconnect ssl error");
    worker_register_job_name(WORKER_SENDER_JOB_DISCONNECT_PARENT_CLOSED, "disconnect parent closed");
    worker_register_job_name(WORKER_SENDER_JOB_DISCONNECT_RECEIVE_ERROR, "disconnect receive error");
    worker_register_job_name(WORKER_SENDER_JOB_DISCONNECT_SEND_ERROR, "disconnect send error");
    worker_register_job_name(WORKER_SENDER_JOB_DISCONNECT_NO_COMPRESSION, "disconnect no compression");
    worker_register_job_name(WORKER_SENDER_JOB_DISCONNECT_BAD_HANDSHAKE, "disconnect bad handshake");

    worker_register_job_custom_metric(WORKER_SENDER_JOB_BUFFER_RATIO, "used buffer ratio", "%", WORKER_METRIC_ABSOLUTE);
    worker_register_job_custom_metric(WORKER_SENDER_JOB_BYTES_RECEIVED, "bytes received", "bytes/s", WORKER_METRIC_INCREMENTAL);
    worker_register_job_custom_metric(WORKER_SENDER_JOB_BYTES_SENT, "bytes sent", "bytes/s", WORKER_METRIC_INCREMENTAL);

    struct sender_state *s = ptr;
    s->tid = gettid();

    if(!rrdhost_has_rrdpush_sender_enabled(s->host) || !s->host->rrdpush_send_destination ||
       !*s->host->rrdpush_send_destination || !s->host->rrdpush_send_api_key ||
       !*s->host->rrdpush_send_api_key) {
        error("STREAM %s [send]: thread created (task id %d), but host has streaming disabled.",
              rrdhost_hostname(s->host), s->tid);
        return NULL;
    }

#ifdef ENABLE_HTTPS
    if (netdata_use_ssl_on_stream & NETDATA_SSL_FORCE ){
        security_start_ssl(NETDATA_SSL_CONTEXT_STREAMING);
        ssl_security_location_for_context(netdata_ssl_client_ctx, netdata_ssl_ca_file, netdata_ssl_ca_path);
    }
#endif

    info("STREAM %s [send]: thread created (task id %d)", rrdhost_hostname(s->host), s->tid);

    s->timeout = (int)appconfig_get_number(
        &stream_config, CONFIG_SECTION_STREAM, "timeout seconds", 60);

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
        error("STREAM %s [send]: cannot create inter-thread communication pipe. Disabling streaming.",
              rrdhost_hostname(s->host));
        return NULL;
    }

    struct rrdpush_sender_thread_data *thread_data = callocz(1, sizeof(struct rrdpush_sender_thread_data));
    thread_data->pipe_buffer = mallocz(pipe_buffer_size);
    thread_data->sender_state = s;
    thread_data->host = s->host;
    thread_data->sending_definitions_status = SENDING_DEFINITIONS_RESTART;

    // reset our cleanup flags
    rrdhost_flag_clear(s->host, RRDHOST_FLAG_RRDPUSH_SENDER_JOIN);

    netdata_thread_cleanup_push(rrdpush_sender_thread_cleanup_callback, thread_data);

    for(; rrdhost_has_rrdpush_sender_enabled(s->host) && !netdata_exit ;) {
        // check for outstanding cancellation requests
        netdata_thread_testcancel();

        // The connection attempt blocks (after which we use the socket in nonblocking)
        if(unlikely(s->rrdpush_sender_socket == -1)) {
            worker_is_busy(WORKER_SENDER_JOB_CONNECT);
            thread_data->sending_definitions_status = SENDING_DEFINITIONS_RESTART;
            rrdhost_flag_clear(s->host, RRDHOST_FLAG_RRDPUSH_SENDER_READY_4_METRICS);
            s->flags &= ~SENDER_FLAG_OVERFLOW;
            s->read_len = 0;
            s->buffer->read = 0;
            s->buffer->write = 0;

            if(unlikely(!attempt_to_connect(s)))
                continue;

            if (stream_has_capability(s, STREAM_CAP_GAP_FILLING)) {
                time_t now = now_realtime_sec();
                BUFFER *wb = sender_start(s);
                buffer_sprintf(wb, "TIMESTAMP %"PRId64"", (int64_t)now);
                sender_commit(s, wb);
            }

            rrdpush_claimed_id(s->host);
            rrdpush_send_host_labels(s->host);

            // TO PUSH METRICS WITH DEFINITIONS:
            //if(unlikely(s->rrdpush_sender_socket != -1 && __atomic_load_n(&s->host->rrdpush_sender_connected, __ATOMIC_SEQ_CST))) {
            //    thread_data->sending_definitions_status = SENDING_DEFINITIONS_DONE;
            //    rrdhost_flag_set(s->host, RRDHOST_FLAG_STREAM_COLLECTED_METRICS);
            //}

            continue;
        }

        // If the TCP window never opened then something is wrong, restart connection
        if(unlikely(now_monotonic_sec() - s->last_sent_t > s->timeout)) {
            worker_is_busy(WORKER_SENDER_JOB_DISCONNECT_TIMEOUT);
            error("STREAM %s [send to %s]: could not send metrics for %d seconds - closing connection - we have sent %zu bytes on this connection via %zu send attempts.", rrdhost_hostname(s->host), s->connected_to, s->timeout, s->sent_bytes_on_this_connection, s->send_attempts);
            rrdpush_sender_thread_close_socket(s->host);
            continue;
        }

        if(unlikely(thread_data->sending_definitions_status != SENDING_DEFINITIONS_DONE))
            rrdpush_queue_incremental_definitions(thread_data);

        netdata_mutex_lock(&s->mutex);
        size_t outstanding = cbuffer_next_unsafe(s->host->sender->buffer, NULL);
        size_t available = cbuffer_available_size_unsafe(s->host->sender->buffer);
        netdata_mutex_unlock(&s->mutex);

        worker_set_metric(WORKER_SENDER_JOB_BUFFER_RATIO, (NETDATA_DOUBLE)(s->host->sender->buffer->max_size - available) * 100.0 / (NETDATA_DOUBLE)s->host->sender->buffer->max_size);

        if(outstanding)
            s->send_attempts++;
        else {
            if(unlikely(thread_data->sending_definitions_status == SENDING_DEFINITIONS_DONE
                         && rrdhost_flag_check(s->host, RRDHOST_FLAG_RRDPUSH_SENDER_CONNECTED)
                         && !rrdhost_flag_check(s->host, RRDHOST_FLAG_RRDPUSH_SENDER_READY_4_METRICS)
                             )) {
                // let the data collection threads know we are ready to push metrics
                rrdhost_flag_set(s->host, RRDHOST_FLAG_RRDPUSH_SENDER_READY_4_METRICS);
                info("STREAM %s [send to %s]: enabling metrics streaming...", rrdhost_hostname(s->host), s->connected_to);
            }
        }

        if(unlikely(s->rrdpush_sender_pipe[PIPE_READ] == -1)) {
            if(!rrdpush_sender_pipe_close(s->host, s->rrdpush_sender_pipe, true)) {
                error("STREAM %s [send]: cannot create inter-thread communication pipe. Disabling streaming.",
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
        int poll_rc = poll(fds, 2, 1000);

        debug(D_STREAM, "STREAM: poll() finished collector=%d socket=%d (current chunk %zu bytes)...",
              fds[Collector].revents, fds[Socket].revents, outstanding);

        if(unlikely(netdata_exit)) break;

        internal_error(fds[Collector].fd != s->rrdpush_sender_pipe[PIPE_READ],
            "STREAM %s [send to %s]: pipe changed after poll().", rrdhost_hostname(s->host), s->connected_to);

        internal_error(fds[Socket].fd != s->rrdpush_sender_socket,
            "STREAM %s [send to %s]: socket changed after poll().", rrdhost_hostname(s->host), s->connected_to);

        // Spurious wake-ups without error - loop again
        if (poll_rc == 0 || ((poll_rc == -1) && (errno == EAGAIN || errno == EINTR))) {
            debug(D_STREAM, "Spurious wakeup");
            continue;
        }

        // Only errors from poll() are internal, but try restarting the connection
        if(unlikely(poll_rc == -1)) {
            worker_is_busy(WORKER_SENDER_JOB_DISCONNECT_POLL_ERROR);
            error("STREAM %s [send to %s]: failed to poll(). Closing socket.", rrdhost_hostname(s->host), s->connected_to);
            rrdpush_sender_pipe_close(s->host, s->rrdpush_sender_pipe, true);
            rrdpush_sender_thread_close_socket(s->host);
            continue;
        }

         // If we have data and have seen the TCP window open then try to close it by a transmission.
        if(likely(outstanding && (fds[Socket].revents & POLLOUT))) {
            worker_is_busy(WORKER_SENDER_JOB_SOCKET_SEND);
            ssize_t bytes = attempt_to_send(s);
            if(bytes > 0)
                worker_set_metric(WORKER_SENDER_JOB_BYTES_SENT, bytes);
        }

        // If the collector woke us up then empty the pipe to remove the signal
        if (fds[Collector].revents & (POLLIN|POLLPRI)) {
            worker_is_busy(WORKER_SENDER_JOB_PIPE_READ);
            debug(D_STREAM, "STREAM: Data added to send buffer (current buffer chunk %zu bytes)...", outstanding);

            if (read(fds[Collector].fd, thread_data->pipe_buffer, pipe_buffer_size) == -1)
                error("STREAM %s [send to %s]: cannot read from internal pipe.", rrdhost_hostname(s->host), s->connected_to);
        }

        // Read as much as possible to fill the buffer, split into full lines for execution.
        if (fds[Socket].revents & POLLIN) {
            worker_is_busy(WORKER_SENDER_JOB_SOCKET_RECEIVE);
            ssize_t bytes = attempt_read(s);
            if(bytes > 0)
                worker_set_metric(WORKER_SENDER_JOB_BYTES_RECEIVED, bytes);
        }

        if(unlikely(s->read_len)) {
            worker_is_busy(WORKER_SENDER_JOB_EXECUTE);
            execute_commands(s);
        }

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
                error("STREAM %s [send to %s]: restarting internal pipe: %s.",
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
                worker_is_busy(WORKER_SENDER_JOB_DISCONNECT_SOCKER_ERROR);
                error("STREAM %s [send to %s]: restarting connection: %s - %zu bytes transmitted.",
                      rrdhost_hostname(s->host), s->connected_to, error, s->sent_bytes_on_this_connection);
                rrdpush_sender_thread_close_socket(s->host);
            }
        }

        // protection from overflow
        if(unlikely(s->flags & SENDER_FLAG_OVERFLOW)) {
            worker_is_busy(WORKER_SENDER_JOB_DISCONNECT_OVERFLOW);
            errno = 0;
            error("STREAM %s [send to %s]: buffer full (allocated %zu bytes) after sending %zu bytes. Restarting connection",
                  rrdhost_hostname(s->host), s->connected_to, s->buffer->size, s->sent_bytes_on_this_connection);
            rrdpush_sender_thread_close_socket(s->host);
        }
    }

    netdata_thread_cleanup_pop(1);
    return NULL;
}
