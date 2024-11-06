// SPDX-License-Identifier: GPL-3.0-or-later

#include "rrdpush.h"
#include "web/server/h2o/http_server.h"

// When a child disconnects this is the maximum we will wait
// before we update the cloud that the child is offline
#define MAX_CHILD_DISC_DELAY (30000)
#define MAX_CHILD_DISC_TOLERANCE (125 / 100)

static uint32_t streaming_connected_receivers = 0;

uint32_t stream_currently_connected_receivers(void) {
    return __atomic_load_n(&streaming_connected_receivers, __ATOMIC_RELAXED);
}

static void streaming_receiver_connected(void) {
    __atomic_add_fetch(&streaming_connected_receivers, 1, __ATOMIC_RELAXED);
}

static void streaming_receiver_disconnected(void) {
    __atomic_sub_fetch(&streaming_connected_receivers, 1, __ATOMIC_RELAXED);
}

void receiver_state_free(struct receiver_state *rpt) {
    netdata_ssl_close(&rpt->ssl);

    if(rpt->fd != -1) {
        internal_error(true, "closing socket...");
        close(rpt->fd);
    }

    rrdpush_decompressor_destroy(&rpt->decompressor);

    if(rpt->system_info)
        rrdhost_system_info_free(rpt->system_info);

    __atomic_sub_fetch(&netdata_buffers_statistics.rrdhost_receivers, sizeof(*rpt), __ATOMIC_RELAXED);

    freez(rpt->key);
    freez(rpt->hostname);
    freez(rpt->registry_hostname);
    freez(rpt->machine_guid);
    freez(rpt->os);
    freez(rpt->timezone);
    freez(rpt->abbrev_timezone);
    freez(rpt->client_ip);
    freez(rpt->client_port);
    freez(rpt->program_name);
    freez(rpt->program_version);

    string_freez(rpt->config.send.api_key);
    string_freez(rpt->config.send.parents);
    string_freez(rpt->config.send.charts_matching);

    freez(rpt);
}

#include "plugins.d/pluginsd_parser.h"

// IMPORTANT: to add workers, you have to edit WORKER_PARSER_FIRST_JOB accordingly
#define WORKER_RECEIVER_JOB_BYTES_READ (WORKER_PARSER_FIRST_JOB - 1)
#define WORKER_RECEIVER_JOB_BYTES_UNCOMPRESSED (WORKER_PARSER_FIRST_JOB - 2)

// this has to be the same at parser.h
#define WORKER_RECEIVER_JOB_REPLICATION_COMPLETION (WORKER_PARSER_FIRST_JOB - 3)

#if WORKER_PARSER_FIRST_JOB < 1
#error The define WORKER_PARSER_FIRST_JOB needs to be at least 1
#endif

static inline int read_stream(struct receiver_state *r, char* buffer, size_t size) {
    if(unlikely(!size)) {
        internal_error(true, "%s() asked to read zero bytes", __FUNCTION__);
        return 0;
    }

#ifdef ENABLE_H2O
    if (is_h2o_rrdpush(r)) {
        if(nd_thread_signaled_to_cancel())
            return -4;

        return (int)h2o_stream_read(r->h2o_ctx, buffer, size);
    }
#endif

    int tries = 100;
    ssize_t bytes_read;

    do {
        errno_clear();

        switch(wait_on_socket_or_cancel_with_timeout(
        &r->ssl,
        r->fd, 0, POLLIN, NULL))
        {
            case 0: // data are waiting
                break;

            case 1: // timeout reached
                netdata_log_error("STREAM: %s(): timeout while waiting for data on socket!", __FUNCTION__);
                return -3;

            case -1: // thread cancelled
                netdata_log_error("STREAM: %s(): thread has been cancelled timeout while waiting for data on socket!", __FUNCTION__);
                return -4;

            default:
            case 2: // error on socket
                netdata_log_error("STREAM: %s() socket error!", __FUNCTION__);
                return -2;
        }

        if (SSL_connection(&r->ssl))
            bytes_read = netdata_ssl_read(&r->ssl, buffer, size);
        else
            bytes_read = read(r->fd, buffer, size);

    } while(bytes_read < 0 && errno == EINTR && tries--);

    if((bytes_read == 0 || bytes_read == -1) && (errno == EAGAIN || errno == EWOULDBLOCK || errno == EINPROGRESS)) {
        netdata_log_error("STREAM: %s(): timeout while waiting for data on socket!", __FUNCTION__);
        bytes_read = -3;
    }
    else if (bytes_read == 0) {
        netdata_log_error("STREAM: %s(): EOF while reading data from socket!", __FUNCTION__);
        bytes_read = -1;
    }
    else if (bytes_read < 0) {
        netdata_log_error("STREAM: %s() failed to read from socket!", __FUNCTION__);
        bytes_read = -2;
    }

    return (int)bytes_read;
}

static inline STREAM_HANDSHAKE read_stream_error_to_reason(int code) {
    if(code > 0)
        return 0;

    switch(code) {
        case 0:
            // asked to read zero bytes
            return STREAM_HANDSHAKE_DISCONNECT_NOT_SUFFICIENT_READ_BUFFER;

        case -1:
            // EOF
            return STREAM_HANDSHAKE_DISCONNECT_SOCKET_EOF;

        case -2:
            // failed to read
            return STREAM_HANDSHAKE_DISCONNECT_SOCKET_READ_FAILED;

        case -3:
            // timeout
            return STREAM_HANDSHAKE_DISCONNECT_SOCKET_READ_TIMEOUT;

        case -4:
            // the thread is cancelled
            return STREAM_HANDSHAKE_DISCONNECT_SHUTDOWN;

        default:
            // anything else
            return STREAM_HANDSHAKE_DISCONNECT_UNKNOWN_SOCKET_READ_ERROR;
    }
}

static inline bool receiver_read_uncompressed(struct receiver_state *r, STREAM_HANDSHAKE *reason) {
#ifdef NETDATA_INTERNAL_CHECKS
    if(r->reader.read_buffer[r->reader.read_len] != '\0')
        fatal("%s(): read_buffer does not start with zero", __FUNCTION__ );
#endif

    int bytes_read = read_stream(r, r->reader.read_buffer + r->reader.read_len, sizeof(r->reader.read_buffer) - r->reader.read_len - 1);
    if(unlikely(bytes_read <= 0)) {
        *reason = read_stream_error_to_reason(bytes_read);
        return false;
    }

    worker_set_metric(WORKER_RECEIVER_JOB_BYTES_READ, (NETDATA_DOUBLE)bytes_read);
    worker_set_metric(WORKER_RECEIVER_JOB_BYTES_UNCOMPRESSED, (NETDATA_DOUBLE)bytes_read);

    r->reader.read_len += bytes_read;
    r->reader.read_buffer[r->reader.read_len] = '\0';

    return true;
}

static inline bool receiver_read_compressed(struct receiver_state *r, STREAM_HANDSHAKE *reason) {

    internal_fatal(r->reader.read_buffer[r->reader.read_len] != '\0',
                   "%s: read_buffer does not start with zero #2", __FUNCTION__ );

    // first use any available uncompressed data
    if (likely(rrdpush_decompressed_bytes_in_buffer(&r->decompressor))) {
        size_t available = sizeof(r->reader.read_buffer) - r->reader.read_len - 1;
        if (likely(available)) {
            size_t len = rrdpush_decompressor_get(&r->decompressor, r->reader.read_buffer + r->reader.read_len, available);
            if (unlikely(!len)) {
                internal_error(true, "decompressor returned zero length #1");
                return false;
            }

            r->reader.read_len += (int)len;
            r->reader.read_buffer[r->reader.read_len] = '\0';
        }
        else
            internal_fatal(true, "The line to read is too big! Already have %zd bytes in read_buffer.", r->reader.read_len);

        return true;
    }

    // no decompressed data available
    // read the compression signature of the next block

    if(unlikely(r->reader.read_len + r->decompressor.signature_size > sizeof(r->reader.read_buffer) - 1)) {
        internal_error(true, "The last incomplete line does not leave enough room for the next compression header! "
                             "Already have %zd bytes in read_buffer.", r->reader.read_len);
        return false;
    }

    // read the compression signature from the stream
    // we have to do a loop here, because read_stream() may return less than the data we need
    int bytes_read = 0;
    do {
        int ret = read_stream(r, r->reader.read_buffer + r->reader.read_len + bytes_read, r->decompressor.signature_size - bytes_read);
        if (unlikely(ret <= 0)) {
            *reason = read_stream_error_to_reason(ret);
            return false;
        }

        bytes_read += ret;
    } while(unlikely(bytes_read < (int)r->decompressor.signature_size));

    worker_set_metric(WORKER_RECEIVER_JOB_BYTES_READ, (NETDATA_DOUBLE)bytes_read);

    if(unlikely(bytes_read != (int)r->decompressor.signature_size))
        fatal("read %d bytes, but expected compression signature of size %zu", bytes_read, r->decompressor.signature_size);

    size_t compressed_message_size = rrdpush_decompressor_start(&r->decompressor, r->reader.read_buffer + r->reader.read_len, bytes_read);
    if (unlikely(!compressed_message_size)) {
        internal_error(true, "multiplexed uncompressed data in compressed stream!");
        r->reader.read_len += bytes_read;
        r->reader.read_buffer[r->reader.read_len] = '\0';
        return true;
    }

    if(unlikely(compressed_message_size > COMPRESSION_MAX_MSG_SIZE)) {
        netdata_log_error("received a compressed message of %zu bytes, which is bigger than the max compressed message size supported of %zu. Ignoring message.",
              compressed_message_size, (size_t)COMPRESSION_MAX_MSG_SIZE);
        return false;
    }

    // delete compression header from our read buffer
    r->reader.read_buffer[r->reader.read_len] = '\0';

    // Read the entire compressed block of compressed data
    char compressed[compressed_message_size];
    size_t compressed_bytes_read = 0;
    do {
        size_t start = compressed_bytes_read;
        size_t remaining = compressed_message_size - start;

        int last_read_bytes = read_stream(r, &compressed[start], remaining);
        if (unlikely(last_read_bytes <= 0)) {
            *reason = read_stream_error_to_reason(last_read_bytes);
            return false;
        }

        compressed_bytes_read += last_read_bytes;

    } while(unlikely(compressed_message_size > compressed_bytes_read));

    worker_set_metric(WORKER_RECEIVER_JOB_BYTES_READ, (NETDATA_DOUBLE)compressed_bytes_read);

    // decompress the compressed block
    size_t bytes_to_parse = rrdpush_decompress(&r->decompressor, compressed, compressed_bytes_read);
    if (unlikely(!bytes_to_parse)) {
        internal_error(true, "no bytes to parse.");
        return false;
    }

    worker_set_metric(WORKER_RECEIVER_JOB_BYTES_UNCOMPRESSED, (NETDATA_DOUBLE)bytes_to_parse);

    // fill read buffer with decompressed data
    size_t len = (int) rrdpush_decompressor_get(&r->decompressor, r->reader.read_buffer + r->reader.read_len, sizeof(r->reader.read_buffer) - r->reader.read_len - 1);
    if (unlikely(!len)) {
        internal_error(true, "decompressor returned zero length #2");
        return false;
    }
    r->reader.read_len += (int)len;
    r->reader.read_buffer[r->reader.read_len] = '\0';

    return true;
}

bool plugin_is_enabled(struct plugind *cd);

static void receiver_set_exit_reason(struct receiver_state *rpt, STREAM_HANDSHAKE reason, bool force) {
    if(force || !rpt->exit.reason)
        rpt->exit.reason = reason;
}

static inline bool receiver_should_stop(struct receiver_state *rpt) {
    static __thread size_t counter = 0;

    if(nd_thread_signaled_to_cancel()) {
        receiver_set_exit_reason(rpt, STREAM_HANDSHAKE_DISCONNECT_SHUTDOWN, false);
        return true;
    }

    if(unlikely(rpt->exit.shutdown)) {
        receiver_set_exit_reason(rpt, STREAM_HANDSHAKE_DISCONNECT_SHUTDOWN, false);
        return true;
    }

    if(unlikely(!service_running(SERVICE_STREAMING))) {
        receiver_set_exit_reason(rpt, STREAM_HANDSHAKE_DISCONNECT_NETDATA_EXIT, false);
        return true;
    }

    if(unlikely((counter++ % 1000) == 0))
        rpt->last_msg_t = now_monotonic_sec();

    return false;
}

static size_t streaming_parser(struct receiver_state *rpt, struct plugind *cd, int fd, void *ssl) {
    size_t result = 0;

    PARSER *parser = NULL;
    {
        PARSER_USER_OBJECT user = {
                .enabled = plugin_is_enabled(cd),
                .host = rpt->host,
                .opaque = rpt,
                .cd = cd,
                .trust_durations = 1,
                .capabilities = rpt->capabilities,
        };

        parser = parser_init(&user, fd, fd, PARSER_INPUT_SPLIT, ssl);
    }

#ifdef ENABLE_H2O
    parser->h2o_ctx = rpt->h2o_ctx;
#endif

    pluginsd_keywords_init(parser, PARSER_INIT_STREAMING);

    rrd_collector_started();

    bool compressed_connection = rrdpush_decompression_initialize(rpt);
    buffered_reader_init(&rpt->reader);

#ifdef NETDATA_LOG_STREAM_RECEIVE
    {
        char filename[FILENAME_MAX + 1];
        snprintfz(filename, FILENAME_MAX, "/tmp/stream-receiver-%s.txt", rpt->host ? rrdhost_hostname(
                        rpt->host) : "unknown"
                 );
        parser->user.stream_log_fp = fopen(filename, "w");
        parser->user.stream_log_repertoire = PARSER_REP_METADATA;
    }
#endif

    CLEAN_BUFFER *buffer = buffer_create(sizeof(rpt->reader.read_buffer), NULL);

    ND_LOG_STACK lgs[] = {
            ND_LOG_FIELD_CB(NDF_REQUEST, line_splitter_reconstruct_line, &parser->line),
            ND_LOG_FIELD_CB(NDF_NIDL_NODE, parser_reconstruct_node, parser),
            ND_LOG_FIELD_CB(NDF_NIDL_INSTANCE, parser_reconstruct_instance, parser),
            ND_LOG_FIELD_CB(NDF_NIDL_CONTEXT, parser_reconstruct_context, parser),
            ND_LOG_FIELD_END(),
    };
    ND_LOG_STACK_PUSH(lgs);

    __atomic_store_n(&rpt->parser, parser, __ATOMIC_RELAXED);
    rrdpush_receiver_send_node_and_claim_id_to_child(rpt->host);

    while(!receiver_should_stop(rpt)) {

        if(!buffered_reader_next_line(&rpt->reader, buffer)) {
            STREAM_HANDSHAKE reason = STREAM_HANDSHAKE_DISCONNECT_UNKNOWN_SOCKET_READ_ERROR;

            bool have_new_data = compressed_connection ? receiver_read_compressed(rpt, &reason)
                                                       : receiver_read_uncompressed(rpt, &reason);

            if(unlikely(!have_new_data)) {
                receiver_set_exit_reason(rpt, reason, false);
                break;
            }

            continue;
        }

        if(unlikely(parser_action(parser, buffer->buffer))) {
            receiver_set_exit_reason(rpt, STREAM_HANDSHAKE_DISCONNECT_PARSER_FAILED, false);
            break;
        }

        buffer->len = 0;
        buffer->buffer[0] = '\0';
    }

    // make sure send_to_plugin() will not write any data to the socket
    spinlock_lock(&parser->writer.spinlock);
    parser->fd_output = -1;
    parser->ssl_output = NULL;
    spinlock_unlock(&parser->writer.spinlock);

    result = parser->user.data_collections_count;
    return result;
}

static void rrdpush_receiver_replication_reset(RRDHOST *host) {
    RRDSET *st;
    rrdset_foreach_read(st, host) {
        rrdset_flag_clear(st, RRDSET_FLAG_RECEIVER_REPLICATION_IN_PROGRESS);
        rrdset_flag_set(st, RRDSET_FLAG_RECEIVER_REPLICATION_FINISHED);
    }
    rrdset_foreach_done(st);
    rrdhost_receiver_replicating_charts_zero(host);
}

static bool rrdhost_set_receiver(RRDHOST *host, struct receiver_state *rpt) {
    bool signal_rrdcontext = false;
    bool set_this = false;

    spinlock_lock(&host->receiver_lock);

    if (!host->receiver) {
        rrdhost_flag_clear(host, RRDHOST_FLAG_ORPHAN);

        host->stream.rcv.status.connections++;
        streaming_receiver_connected();

        host->receiver = rpt;
        rpt->host = host;

        host->stream.rcv.status.last_connected = now_realtime_sec();
        host->stream.rcv.status.last_disconnected = 0;
        host->stream.rcv.status.last_chart = 0;
        host->stream.rcv.status.check_obsolete = true;

        if (rpt->config.health.enabled != CONFIG_BOOLEAN_NO) {
            if (rpt->config.health.delay > 0) {
                host->health.delay_up_to = now_realtime_sec() + rpt->config.health.delay;
                nd_log(NDLS_DAEMON, NDLP_DEBUG,
                       "[%s]: Postponing health checks for %" PRId64 " seconds, because it was just connected.",
                       rrdhost_hostname(host),
                       (int64_t) rpt->config.health.delay);
            }
        }

        host->health_log.health_log_retention_s = rpt->config.health.history;

//         this is a test
//        if(rpt->hops <= host->sender->hops)
//            rrdpush_sender_thread_stop(host, "HOPS MISMATCH", false);

        signal_rrdcontext = true;
        rrdpush_receiver_replication_reset(host);

        rrdhost_flag_clear(rpt->host, RRDHOST_FLAG_RRDPUSH_RECEIVER_DISCONNECTED);
        aclk_queue_node_info(rpt->host, true);

        rrdhost_stream_parent_reset_postpone_time(host);

        set_this = true;
    }

    spinlock_unlock(&host->receiver_lock);

    if(signal_rrdcontext)
        rrdcontext_host_child_connected(host);

    return set_this;
}

static void rrdhost_clear_receiver(struct receiver_state *rpt) {
    RRDHOST *host = rpt->host;
    if(!host) return;

    spinlock_lock(&host->receiver_lock);
    {
        // Make sure that we detach this thread and don't kill a freshly arriving receiver

        if (host->receiver == rpt) {
            spinlock_unlock(&host->receiver_lock);
            {
                // run all these without having the receiver lock

                stream_path_child_disconnected(host);
                rrdpush_sender_thread_stop(host, STREAM_HANDSHAKE_DISCONNECT_RECEIVER_LEFT, false);
                rrdpush_receiver_replication_reset(host);
                rrdcontext_host_child_disconnected(host);

                if (rpt->config.health.enabled)
                    rrdcalc_child_disconnected(host);

                rrdhost_stream_parent_reset_postpone_time(host);
            }
            spinlock_lock(&host->receiver_lock);

            // now we have the lock again

            streaming_receiver_disconnected();
            rrdhost_flag_set(rpt->host, RRDHOST_FLAG_RRDPUSH_RECEIVER_DISCONNECTED);

            host->stream.rcv.status.check_obsolete = false;
            host->stream.rcv.status.last_connected = 0;
            host->stream.rcv.status.last_disconnected = now_realtime_sec();
            host->health.enabled = false;

            host->stream.rcv.status.exit_reason = rpt->exit.reason;
            rrdhost_flag_set(host, RRDHOST_FLAG_ORPHAN);
            host->receiver = NULL;
        }
    }

    // this must be cleared with the receiver lock
    pluginsd_process_cleanup(rpt->parser);
    __atomic_store_n(&rpt->parser, NULL, __ATOMIC_RELAXED);

    spinlock_unlock(&host->receiver_lock);
}

bool stop_streaming_receiver(RRDHOST *host, STREAM_HANDSHAKE reason) {
    bool ret = false;

    spinlock_lock(&host->receiver_lock);

    if(host->receiver) {
        if(!host->receiver->exit.shutdown) {
            host->receiver->exit.shutdown = true;
            receiver_set_exit_reason(host->receiver, reason, true);
            shutdown(host->receiver->fd, SHUT_RDWR);
        }

        nd_thread_signal_cancel(host->receiver->thread);
    }

    int count = 2000;
    while (host->receiver && count-- > 0) {
        spinlock_unlock(&host->receiver_lock);

        // let the lock for the receiver thread to exit
        sleep_usec(1 * USEC_PER_MS);

        spinlock_lock(&host->receiver_lock);
    }

    if(host->receiver)
        netdata_log_error("STREAM '%s' [receive from [%s]:%s]: "
              "thread %d takes too long to stop, giving up..."
        , rrdhost_hostname(host)
        , host->receiver->client_ip, host->receiver->client_port
        , host->receiver->tid);
    else
        ret = true;

    spinlock_unlock(&host->receiver_lock);

    return ret;
}

static void rrdpush_send_error_on_taken_over_connection(struct receiver_state *rpt, const char *msg) {
    (void) send_timeout(
            &rpt->ssl,
            rpt->fd,
            (char *)msg,
            strlen(msg),
            0,
            5);
}

static void rrdpush_receive_log_status(struct receiver_state *rpt, const char *msg, const char *status, ND_LOG_FIELD_PRIORITY priority) {
    // this function may be called BEFORE we spawn the receiver thread
    // so, we need to add the fields again (it does not harm)
    ND_LOG_STACK lgs[] = {
            ND_LOG_FIELD_TXT(NDF_SRC_IP, rpt->client_ip),
            ND_LOG_FIELD_TXT(NDF_SRC_PORT, rpt->client_port),
            ND_LOG_FIELD_TXT(NDF_NIDL_NODE, (rpt->hostname && *rpt->hostname) ? rpt->hostname : ""),
            ND_LOG_FIELD_TXT(NDF_RESPONSE_CODE, status),
            ND_LOG_FIELD_UUID(NDF_MESSAGE_ID, &streaming_from_child_msgid),
            ND_LOG_FIELD_END(),
    };
    ND_LOG_STACK_PUSH(lgs);

    nd_log(NDLS_ACCESS, priority, "api_key:'%s' machine_guid:'%s' msg:'%s'"
                       , (rpt->key && *rpt->key)? rpt->key : ""
                       , (rpt->machine_guid && *rpt->machine_guid) ? rpt->machine_guid : ""
                       , msg);

    nd_log(NDLS_DAEMON, priority, "STREAM_RECEIVER for '%s': %s %s%s%s"
                     , (rpt->hostname && *rpt->hostname) ? rpt->hostname : ""
                     , msg
                     , rpt->exit.reason != STREAM_HANDSHAKE_NEVER?" (":""
                     , stream_handshake_error_to_string(rpt->exit.reason)
                     , rpt->exit.reason != STREAM_HANDSHAKE_NEVER?")":""
                     );
}

static void rrdpush_receive(struct receiver_state *rpt) {
    stream_conf_receiver_config(rpt, &rpt->config, rpt->key, rpt->machine_guid);

    // find the host for this receiver
    {
        // this will also update the host with our system_info
        RRDHOST *host = rrdhost_find_or_create(
            rpt->hostname,
            rpt->registry_hostname,
            rpt->machine_guid,
            rpt->os,
            rpt->timezone,
            rpt->abbrev_timezone,
            rpt->utc_offset,
            rpt->program_name,
            rpt->program_version,
            rpt->config.update_every,
            rpt->config.history,
            rpt->config.mode,
            rpt->config.health.enabled != CONFIG_BOOLEAN_NO,
            rpt->config.send.enabled && rpt->config.send.parents && rpt->config.send.api_key,
            rpt->config.send.parents,
            rpt->config.send.api_key,
            rpt->config.send.charts_matching,
            rpt->config.replication.enabled,
            rpt->config.replication.period,
            rpt->config.replication.step,
            rpt->system_info,
            0);

        if(!host) {
            rrdpush_receive_log_status(
                    rpt,"failed to find/create host structure, rejecting connection",
                    RRDPUSH_STATUS_INTERNAL_SERVER_ERROR, NDLP_ERR);

            rrdpush_send_error_on_taken_over_connection(rpt, START_STREAMING_ERROR_INTERNAL_ERROR);
            goto cleanup;
        }

        if (unlikely(rrdhost_flag_check(host, RRDHOST_FLAG_PENDING_CONTEXT_LOAD))) {
            rrdpush_receive_log_status(
                    rpt, "host is initializing, retry later",
                    RRDPUSH_STATUS_INITIALIZATION_IN_PROGRESS, NDLP_NOTICE);

            rrdpush_send_error_on_taken_over_connection(rpt, START_STREAMING_ERROR_INITIALIZATION);
            goto cleanup;
        }

        // system_info has been consumed by the host structure
        rpt->system_info = NULL;

        if(!rrdhost_set_receiver(host, rpt)) {
            rrdpush_receive_log_status(
                    rpt, "host is already served by another receiver",
                    RRDPUSH_STATUS_DUPLICATE_RECEIVER, NDLP_INFO);

            rrdpush_send_error_on_taken_over_connection(rpt, START_STREAMING_ERROR_ALREADY_STREAMING);
            goto cleanup;
        }
    }

#ifdef NETDATA_INTERNAL_CHECKS
    netdata_log_info("STREAM '%s' [receive from [%s]:%s]: "
         "client willing to stream metrics for host '%s' with machine_guid '%s': "
         "update every = %d, history = %d, memory mode = %s, health %s,%s"
         , rpt->hostname
         , rpt->client_ip
         , rpt->client_port
         , rrdhost_hostname(rpt->host)
         , rpt->host->machine_guid
         , rpt->host->rrd_update_every
         , rpt->host->rrd_history_entries
         , rrd_memory_mode_name(rpt->host->rrd_memory_mode)
         , (rpt->config.health.enabled == CONFIG_BOOLEAN_NO)?"disabled":((rpt->config.health.enabled == CONFIG_BOOLEAN_YES)?"enabled":"auto")
         , (rpt->ssl.conn != NULL) ? " SSL," : ""
    );
#endif // NETDATA_INTERNAL_CHECKS


    struct plugind cd = {
            .update_every = default_rrd_update_every,
            .unsafe = {
                    .spinlock = NETDATA_SPINLOCK_INITIALIZER,
                    .running = true,
                    .enabled = true,
            },
            .started_t = now_realtime_sec(),
    };

    // put the client IP and port into the buffers used by plugins.d
    snprintfz(cd.id,           CONFIG_MAX_NAME,  "%s:%s", rpt->client_ip, rpt->client_port);
    snprintfz(cd.filename,     FILENAME_MAX,     "%s:%s", rpt->client_ip, rpt->client_port);
    snprintfz(cd.fullfilename, FILENAME_MAX,     "%s:%s", rpt->client_ip, rpt->client_port);
    snprintfz(cd.cmd,          PLUGINSD_CMD_MAX, "%s:%s", rpt->client_ip, rpt->client_port);

    rrdpush_select_receiver_compression_algorithm(rpt);

    {
        // netdata_log_info("STREAM %s [receive from [%s]:%s]: initializing communication...", rrdhost_hostname(rpt->host), rpt->client_ip, rpt->client_port);
        char initial_response[HTTP_HEADER_SIZE];
        if (stream_has_capability(rpt, STREAM_CAP_VCAPS)) {
            log_receiver_capabilities(rpt);
            sprintf(initial_response, "%s%u", START_STREAMING_PROMPT_VN, rpt->capabilities);
        }
        else if (stream_has_capability(rpt, STREAM_CAP_VN)) {
            log_receiver_capabilities(rpt);
            sprintf(initial_response, "%s%d", START_STREAMING_PROMPT_VN, stream_capabilities_to_vn(rpt->capabilities));
        }
        else if (stream_has_capability(rpt, STREAM_CAP_V2)) {
            log_receiver_capabilities(rpt);
            sprintf(initial_response, "%s", START_STREAMING_PROMPT_V2);
        }
        else { // stream_has_capability(rpt, STREAM_CAP_V1)
            log_receiver_capabilities(rpt);
            sprintf(initial_response, "%s", START_STREAMING_PROMPT_V1);
        }

        netdata_log_debug(D_STREAM, "Initial response to %s: %s", rpt->client_ip, initial_response);
#ifdef ENABLE_H2O
        if (is_h2o_rrdpush(rpt)) {
            h2o_stream_write(rpt->h2o_ctx, initial_response, strlen(initial_response));
        } else {
#endif
            ssize_t bytes_sent = send_timeout(
                    &rpt->ssl,
                    rpt->fd, initial_response, strlen(initial_response), 0, 60);

            if(bytes_sent != (ssize_t)strlen(initial_response)) {
                internal_error(true, "Cannot send response, got %zd bytes, expecting %zu bytes", bytes_sent, strlen(initial_response));
                rrdpush_receive_log_status(
                        rpt, "cannot reply back, dropping connection",
                        RRDPUSH_STATUS_CANT_REPLY, NDLP_ERR);
                goto cleanup;
            }
#ifdef ENABLE_H2O
        }
#endif
    }

#ifdef ENABLE_H2O
    unless_h2o_rrdpush(rpt)
#endif
    {
        // remove the non-blocking flag from the socket
        if(sock_delnonblock(rpt->fd) < 0)
            netdata_log_error("STREAM '%s' [receive from [%s]:%s]: "
                  "cannot remove the non-blocking flag from socket %d"
                  , rrdhost_hostname(rpt->host)
                  , rpt->client_ip, rpt->client_port
                  , rpt->fd);

        struct timeval timeout;
        timeout.tv_sec = 600;
        timeout.tv_usec = 0;
        if (unlikely(setsockopt(rpt->fd, SOL_SOCKET, SO_RCVTIMEO, &timeout, sizeof timeout) != 0))
            netdata_log_error("STREAM '%s' [receive from [%s]:%s]: "
                  "cannot set timeout for socket %d"
                  , rrdhost_hostname(rpt->host)
                  , rpt->client_ip, rpt->client_port
                  , rpt->fd);
    }

    rrdpush_receive_log_status(
            rpt, "connected and ready to receive data",
            RRDPUSH_STATUS_CONNECTED, NDLP_INFO);

    // in case we have cloud connection we inform cloud
    // new child connected
    schedule_node_state_update(rpt->host, 300);
    rrdhost_set_is_parent_label();

    if (rpt->config.ephemeral)
        rrdhost_option_set(rpt->host, RRDHOST_OPTION_EPHEMERAL_HOST);

    // let it reconnect to parent immediately
    rrdhost_stream_parent_reset_postpone_time(rpt->host);

    // receive data
    size_t count = streaming_parser(rpt, &cd, rpt->fd, (rpt->ssl.conn) ? &rpt->ssl : NULL);

    // the parser stopped
    receiver_set_exit_reason(rpt, STREAM_HANDSHAKE_DISCONNECT_PARSER_EXIT, false);

    {
        char msg[100 + 1];
        snprintfz(msg, sizeof(msg) - 1, "disconnected (completed %zu updates)", count);
        rrdpush_receive_log_status(rpt, msg, RRDPUSH_STATUS_DISCONNECTED, NDLP_WARNING);
    }

    // in case we have cloud connection we inform cloud
    // a child disconnected
    STREAM_PATH tmp = rrdhost_stream_path_fetch(rpt->host);
    uint64_t total_reboot = (tmp.start_time_ms + tmp.shutdown_time_ms);
    schedule_node_state_update(rpt->host, MIN((total_reboot * MAX_CHILD_DISC_TOLERANCE), MAX_CHILD_DISC_DELAY));

cleanup:
    ;
}

static bool stream_receiver_log_capabilities(BUFFER *wb, void *ptr) {
    struct receiver_state *rpt = ptr;
    if(!rpt)
        return false;

    stream_capabilities_to_string(wb, rpt->capabilities);
    return true;
}

static bool stream_receiver_log_transport(BUFFER *wb, void *ptr) {
    struct receiver_state *rpt = ptr;
    if(!rpt)
        return false;

    buffer_strcat(wb, SSL_connection(&rpt->ssl) ? "https" : "http");
    return true;
}

void *rrdpush_receiver_thread(void *ptr) {
    worker_register("STREAMRCV");

    worker_register_job_custom_metric(WORKER_RECEIVER_JOB_BYTES_READ,
                                      "received bytes", "bytes/s",
                                      WORKER_METRIC_INCREMENT);

    worker_register_job_custom_metric(WORKER_RECEIVER_JOB_BYTES_UNCOMPRESSED,
                                      "uncompressed bytes", "bytes/s",
                                      WORKER_METRIC_INCREMENT);

    worker_register_job_custom_metric(WORKER_RECEIVER_JOB_REPLICATION_COMPLETION,
                                      "replication completion", "%",
                                      WORKER_METRIC_ABSOLUTE);

    struct receiver_state *rpt = (struct receiver_state *) ptr;
    rpt->tid = gettid_cached();

    ND_LOG_STACK lgs[] = {
            ND_LOG_FIELD_TXT(NDF_SRC_IP, rpt->client_ip),
            ND_LOG_FIELD_TXT(NDF_SRC_PORT, rpt->client_port),
            ND_LOG_FIELD_TXT(NDF_NIDL_NODE, rpt->hostname),
            ND_LOG_FIELD_CB(NDF_SRC_TRANSPORT, stream_receiver_log_transport, rpt),
            ND_LOG_FIELD_CB(NDF_SRC_CAPABILITIES, stream_receiver_log_capabilities, rpt),
            ND_LOG_FIELD_END(),
    };
    ND_LOG_STACK_PUSH(lgs);

    netdata_log_info("STREAM %s [%s]:%s: receive thread started", rpt->hostname, rpt->client_ip
                     , rpt->client_port);

    rrdpush_receive(rpt);

    netdata_log_info("STREAM '%s' [receive from [%s]:%s]: "
                     "receive thread ended (task id %d)"
                     , rpt->hostname ? rpt->hostname : "-"
                     , rpt->client_ip ? rpt->client_ip : "-", rpt->client_port ? rpt->client_port : "-", gettid_cached());

    worker_unregister();
    rrdhost_clear_receiver(rpt);
    rrdhost_set_is_parent_label();
    receiver_state_free(rpt);
    return NULL;
}

int rrdpush_receiver_permission_denied(struct web_client *w) {
    // we always respond with the same message and error code
    // to prevent an attacker from gaining info about the error
    buffer_flush(w->response.data);
    buffer_strcat(w->response.data, START_STREAMING_ERROR_NOT_PERMITTED);
    return HTTP_RESP_UNAUTHORIZED;
}

int rrdpush_receiver_too_busy_now(struct web_client *w) {
    // we always respond with the same message and error code
    // to prevent an attacker from gaining info about the error
    buffer_flush(w->response.data);
    buffer_strcat(w->response.data, START_STREAMING_ERROR_BUSY_TRY_LATER);
    return HTTP_RESP_SERVICE_UNAVAILABLE;
}

static void rrdpush_receiver_takeover_web_connection(struct web_client *w, struct receiver_state *rpt) {
    rpt->fd                = w->ifd;

    rpt->ssl.conn          = w->ssl.conn;
    rpt->ssl.state         = w->ssl.state;

    w->ssl = NETDATA_SSL_UNSET_CONNECTION;

    WEB_CLIENT_IS_DEAD(w);

    if(web_server_mode == WEB_SERVER_MODE_STATIC_THREADED) {
        web_client_flag_set(w, WEB_CLIENT_FLAG_DONT_CLOSE_SOCKET);
    }
    else {
        if(w->ifd == w->ofd)
            w->ifd = w->ofd = -1;
        else
            w->ifd = -1;
    }

    buffer_flush(w->response.data);
}

int rrdpush_receiver_thread_spawn(struct web_client *w, char *decoded_query_string, void *h2o_ctx __maybe_unused) {

    if(!service_running(ABILITY_STREAMING_CONNECTIONS))
        return rrdpush_receiver_too_busy_now(w);

    struct receiver_state *rpt = callocz(1, sizeof(*rpt));
    rpt->connected_since_s = now_realtime_sec();
    rpt->last_msg_t = now_monotonic_sec();
    rpt->hops = 1;

    rpt->capabilities = STREAM_CAP_INVALID;

#ifdef ENABLE_H2O
    rpt->h2o_ctx = h2o_ctx;
#endif

    __atomic_add_fetch(&netdata_buffers_statistics.rrdhost_receivers, sizeof(*rpt), __ATOMIC_RELAXED);
    __atomic_add_fetch(&netdata_buffers_statistics.rrdhost_allocations_size, sizeof(struct rrdhost_system_info), __ATOMIC_RELAXED);

    rpt->system_info = callocz(1, sizeof(struct rrdhost_system_info));
    rpt->system_info->hops = rpt->hops;

    rpt->fd                = -1;
    rpt->client_ip         = strdupz(w->client_ip);
    rpt->client_port       = strdupz(w->client_port);

    rpt->ssl = NETDATA_SSL_UNSET_CONNECTION;

    rpt->config.update_every = default_rrd_update_every;

    // parse the parameters and fill rpt and rpt->system_info

    while(decoded_query_string) {
        char *value = strsep_skip_consecutive_separators(&decoded_query_string, "&");
        if(!value || !*value) continue;

        char *name = strsep_skip_consecutive_separators(&value, "=");
        if(!name || !*name) continue;
        if(!value || !*value) continue;

        if(!strcmp(name, "key") && !rpt->key)
            rpt->key = strdupz(value);

        else if(!strcmp(name, "hostname") && !rpt->hostname)
            rpt->hostname = strdupz(value);

        else if(!strcmp(name, "registry_hostname") && !rpt->registry_hostname)
            rpt->registry_hostname = strdupz(value);

        else if(!strcmp(name, "machine_guid") && !rpt->machine_guid)
            rpt->machine_guid = strdupz(value);

        else if(!strcmp(name, "update_every"))
            rpt->config.update_every = (int)strtoul(value, NULL, 0);

        else if(!strcmp(name, "os") && !rpt->os)
            rpt->os = strdupz(value);

        else if(!strcmp(name, "timezone") && !rpt->timezone)
            rpt->timezone = strdupz(value);

        else if(!strcmp(name, "abbrev_timezone") && !rpt->abbrev_timezone)
            rpt->abbrev_timezone = strdupz(value);

        else if(!strcmp(name, "utc_offset"))
            rpt->utc_offset = (int32_t)strtol(value, NULL, 0);

        else if(!strcmp(name, "hops"))
            rpt->hops = rpt->system_info->hops = (uint16_t) strtoul(value, NULL, 0);

        else if(!strcmp(name, "ml_capable"))
            rpt->system_info->ml_capable = strtoul(value, NULL, 0);

        else if(!strcmp(name, "ml_enabled"))
            rpt->system_info->ml_enabled = strtoul(value, NULL, 0);

        else if(!strcmp(name, "mc_version"))
            rpt->system_info->mc_version = strtoul(value, NULL, 0);

        else if(!strcmp(name, "ver") && (rpt->capabilities & STREAM_CAP_INVALID))
            rpt->capabilities = convert_stream_version_to_capabilities(strtoul(value, NULL, 0), NULL, false);

        else {
            // An old Netdata child does not have a compatible streaming protocol, map to something sane.
            if (!strcmp(name, "NETDATA_SYSTEM_OS_NAME"))
                name = "NETDATA_HOST_OS_NAME";

            else if (!strcmp(name, "NETDATA_SYSTEM_OS_ID"))
                name = "NETDATA_HOST_OS_ID";

            else if (!strcmp(name, "NETDATA_SYSTEM_OS_ID_LIKE"))
                name = "NETDATA_HOST_OS_ID_LIKE";

            else if (!strcmp(name, "NETDATA_SYSTEM_OS_VERSION"))
                name = "NETDATA_HOST_OS_VERSION";

            else if (!strcmp(name, "NETDATA_SYSTEM_OS_VERSION_ID"))
                name = "NETDATA_HOST_OS_VERSION_ID";

            else if (!strcmp(name, "NETDATA_SYSTEM_OS_DETECTION"))
                name = "NETDATA_HOST_OS_DETECTION";

            else if(!strcmp(name, "NETDATA_PROTOCOL_VERSION") && (rpt->capabilities & STREAM_CAP_INVALID))
                rpt->capabilities = convert_stream_version_to_capabilities(1, NULL, false);

            if (unlikely(rrdhost_set_system_info_variable(rpt->system_info, name, value))) {
                nd_log_daemon(NDLP_NOTICE, "STREAM '%s' [receive from [%s]:%s]: "
                                           "request has parameter '%s' = '%s', which is not used."
                              , (rpt->hostname && *rpt->hostname) ? rpt->hostname : "-"
                              , rpt->client_ip, rpt->client_port
                              , name, value);
            }
        }
    }

    if (rpt->capabilities & STREAM_CAP_INVALID)
        // no version is supplied, assume version 0;
        rpt->capabilities = convert_stream_version_to_capabilities(0, NULL, false);

    // find the program name and version
    if(w->user_agent && w->user_agent[0]) {
        char *t = strchr(w->user_agent, '/');
        if(t && *t) {
            *t = '\0';
            t++;
        }

        rpt->program_name = strdupz(w->user_agent);
        if(t && *t) rpt->program_version = strdupz(t);
    }

    // check if we should accept this connection

    if(!rpt->key || !*rpt->key) {
        rrdpush_receive_log_status(
            rpt, "request without an API key, rejecting connection",
            RRDPUSH_STATUS_NO_API_KEY, NDLP_WARNING);

        receiver_state_free(rpt);
        return rrdpush_receiver_permission_denied(w);
    }

    if(!rpt->hostname || !*rpt->hostname) {
        rrdpush_receive_log_status(
            rpt, "request without a hostname, rejecting connection",
            RRDPUSH_STATUS_NO_HOSTNAME, NDLP_WARNING);

        receiver_state_free(rpt);
        return rrdpush_receiver_permission_denied(w);
    }

    if(!rpt->registry_hostname)
        rpt->registry_hostname = strdupz(rpt->hostname);

    if(!rpt->machine_guid || !*rpt->machine_guid) {
        rrdpush_receive_log_status(
            rpt, "request without a machine GUID, rejecting connection",
            RRDPUSH_STATUS_NO_MACHINE_GUID, NDLP_WARNING);

        receiver_state_free(rpt);
        return rrdpush_receiver_permission_denied(w);
    }

    {
        char buf[GUID_LEN + 1];

        if (regenerate_guid(rpt->key, buf) == -1) {
            rrdpush_receive_log_status(
                rpt, "API key is not a valid UUID (use the command uuidgen to generate one)",
                RRDPUSH_STATUS_INVALID_API_KEY, NDLP_WARNING);

            receiver_state_free(rpt);
            return rrdpush_receiver_permission_denied(w);
        }

        if (regenerate_guid(rpt->machine_guid, buf) == -1) {
            rrdpush_receive_log_status(
                rpt, "machine GUID is not a valid UUID",
                RRDPUSH_STATUS_INVALID_MACHINE_GUID, NDLP_WARNING);

            receiver_state_free(rpt);
            return rrdpush_receiver_permission_denied(w);
        }
    }

    if(!stream_conf_is_key_type(rpt->key, "api")) {
        rrdpush_receive_log_status(
            rpt, "API key is a machine GUID",
            RRDPUSH_STATUS_INVALID_API_KEY, NDLP_WARNING);

        receiver_state_free(rpt);
        return rrdpush_receiver_permission_denied(w);
    }

    // the default for api keys is false, so that users
    // have to enable them manually
    if(!stream_conf_api_key_is_enabled(rpt->key, false)) {
        rrdpush_receive_log_status(
            rpt, "API key is not enabled",
            RRDPUSH_STATUS_API_KEY_DISABLED, NDLP_WARNING);

        receiver_state_free(rpt);
        return rrdpush_receiver_permission_denied(w);
    }

    if(!stream_conf_api_key_allows_client(rpt->key, w->client_ip)) {
        rrdpush_receive_log_status(
            rpt, "API key is not allowed from this IP",
            RRDPUSH_STATUS_NOT_ALLOWED_IP, NDLP_WARNING);

        receiver_state_free(rpt);
        return rrdpush_receiver_permission_denied(w);
    }

    if (!stream_conf_is_key_type(rpt->machine_guid, "machine")) {
        rrdpush_receive_log_status(
            rpt, "machine GUID is an API key",
            RRDPUSH_STATUS_INVALID_MACHINE_GUID, NDLP_WARNING);

        receiver_state_free(rpt);
        return rrdpush_receiver_permission_denied(w);
    }

    // the default for machine guids is true, so that users do not
    // have to enable them manually
    if(!stream_conf_api_key_is_enabled(rpt->machine_guid, true)) {
        rrdpush_receive_log_status(
            rpt, "machine GUID is not enabled",
            RRDPUSH_STATUS_MACHINE_GUID_DISABLED, NDLP_WARNING);

        receiver_state_free(rpt);
        return rrdpush_receiver_permission_denied(w);
    }

    if(!stream_conf_api_key_allows_client(rpt->machine_guid, w->client_ip)) {
        rrdpush_receive_log_status(
            rpt, "machine GUID is not allowed from this IP",
            RRDPUSH_STATUS_NOT_ALLOWED_IP, NDLP_WARNING);

        receiver_state_free(rpt);
        return rrdpush_receiver_permission_denied(w);
    }

    if (strcmp(rpt->machine_guid, localhost->machine_guid) == 0) {
        rrdpush_receiver_takeover_web_connection(w, rpt);

        rrdpush_receive_log_status(
            rpt, "machine GUID is my own",
            RRDPUSH_STATUS_LOCALHOST, NDLP_DEBUG);

        char initial_response[HTTP_HEADER_SIZE + 1];
        snprintfz(initial_response, HTTP_HEADER_SIZE, "%s", START_STREAMING_ERROR_SAME_LOCALHOST);

        if(send_timeout(&rpt->ssl, rpt->fd, initial_response, strlen(initial_response), 0, 60) !=
            (ssize_t)strlen(initial_response)) {

            nd_log_daemon(NDLP_ERR, "STREAM '%s' [receive from [%s]:%s]: "
                                    "failed to reply."
                          , rpt->hostname
                          , rpt->client_ip, rpt->client_port
            );
        }

        receiver_state_free(rpt);
        return HTTP_RESP_OK;
    }

    if(unlikely(web_client_streaming_rate_t > 0)) {
        static SPINLOCK spinlock = NETDATA_SPINLOCK_INITIALIZER;
        static time_t last_stream_accepted_t = 0;

        time_t now = now_realtime_sec();
        spinlock_lock(&spinlock);

        if(unlikely(last_stream_accepted_t == 0))
            last_stream_accepted_t = now;

        if(now - last_stream_accepted_t < web_client_streaming_rate_t) {
            spinlock_unlock(&spinlock);

            char msg[100 + 1];
            snprintfz(msg, sizeof(msg) - 1,
                      "rate limit, will accept new connection in %ld secs",
                      (long)(web_client_streaming_rate_t - (now - last_stream_accepted_t)));

            rrdpush_receive_log_status(
                rpt, msg,
                RRDPUSH_STATUS_RATE_LIMIT, NDLP_NOTICE);

            receiver_state_free(rpt);
            return rrdpush_receiver_too_busy_now(w);
        }

        last_stream_accepted_t = now;
        spinlock_unlock(&spinlock);
    }

    /*
     * Quick path for rejecting multiple connections. The lock taken is fine-grained - it only protects the receiver
     * pointer within the host (if a host exists). This protects against multiple concurrent web requests hitting
     * separate threads within the web-server and landing here. The lock guards the thread-shutdown sequence that
     * detaches the receiver from the host. If the host is being created (first time-access) then we also use the
     * lock to prevent race-hazard (two threads try to create the host concurrently, one wins and the other does a
     * lookup to the now-attached structure).
     */

    {
        time_t age = 0;
        bool receiver_stale = false;
        bool receiver_working = false;

        rrd_rdlock();
        RRDHOST *host = rrdhost_find_by_guid(rpt->machine_guid);
        if (unlikely(host && rrdhost_flag_check(host, RRDHOST_FLAG_ARCHIVED))) /* Ignore archived hosts. */
            host = NULL;

        if (host) {
            spinlock_lock(&host->receiver_lock);
            if (host->receiver) {
                age = now_monotonic_sec() - host->receiver->last_msg_t;

                if (age < 30)
                    receiver_working = true;
                else
                    receiver_stale = true;
            }
            spinlock_unlock(&host->receiver_lock);
        }
        rrd_rdunlock();

        if (receiver_stale && stop_streaming_receiver(host, STREAM_HANDSHAKE_DISCONNECT_STALE_RECEIVER)) {
            // we stopped the receiver
            // we can proceed with this connection
            receiver_stale = false;

            nd_log_daemon(NDLP_NOTICE, "STREAM '%s' [receive from [%s]:%s]: "
                                       "stopped previous stale receiver to accept this one."
                          , rpt->hostname
                          , rpt->client_ip, rpt->client_port
            );
        }

        if (receiver_working || receiver_stale) {
            // another receiver is already connected
            // try again later

            char msg[200 + 1];
            snprintfz(msg, sizeof(msg) - 1,
                      "multiple connections for same host, "
                      "old connection was last used %ld secs ago%s",
                      age, receiver_stale ? " (signaled old receiver to stop)" : " (new connection not accepted)");

            rrdpush_receive_log_status(
                rpt, msg,
                RRDPUSH_STATUS_ALREADY_CONNECTED, NDLP_DEBUG);

            // Have not set WEB_CLIENT_FLAG_DONT_CLOSE_SOCKET - caller should clean up
            buffer_flush(w->response.data);
            buffer_strcat(w->response.data, START_STREAMING_ERROR_ALREADY_STREAMING);
            receiver_state_free(rpt);
            return HTTP_RESP_CONFLICT;
        }
    }

    rrdpush_receiver_takeover_web_connection(w, rpt);

    char tag[NETDATA_THREAD_TAG_MAX + 1];
    snprintfz(tag, NETDATA_THREAD_TAG_MAX, THREAD_TAG_STREAM_RECEIVER "[%s]", rpt->hostname);
    tag[NETDATA_THREAD_TAG_MAX] = '\0';

    rpt->thread = nd_thread_create(tag, NETDATA_THREAD_OPTION_DEFAULT, rrdpush_receiver_thread, (void *)rpt);
    if(!rpt->thread) {
        rrdpush_receive_log_status(
            rpt, "can't create receiver thread",
            RRDPUSH_STATUS_INTERNAL_SERVER_ERROR, NDLP_ERR);

        buffer_flush(w->response.data);
        buffer_strcat(w->response.data, "Can't handle this request");
        receiver_state_free(rpt);
        return HTTP_RESP_INTERNAL_SERVER_ERROR;
    }

    // prevent the caller from closing the streaming socket
    return HTTP_RESP_OK;
}
