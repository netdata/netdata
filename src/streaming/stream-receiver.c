// SPDX-License-Identifier: GPL-3.0-or-later

#include "stream.h"
#include "stream-thread.h"
#include "stream-receiver-internals.h"
#include "web/server/h2o/http_server.h"

// When a child disconnects this is the maximum we will wait
// before we update the cloud that the child is offline
#define MAX_CHILD_DISC_DELAY (30000)
#define MAX_CHILD_DISC_TOLERANCE (125 / 100)

static uint32_t streaming_connected_receivers = 0;

bool plugin_is_enabled(struct plugind *cd);

uint32_t stream_receivers_currently_connected(void) {
    return __atomic_load_n(&streaming_connected_receivers, __ATOMIC_RELAXED);
}

static void streaming_receiver_connected(void) {
    __atomic_add_fetch(&streaming_connected_receivers, 1, __ATOMIC_RELAXED);
}

static void streaming_receiver_disconnected(void) {
    __atomic_sub_fetch(&streaming_connected_receivers, 1, __ATOMIC_RELAXED);
}

// --------------------------------------------------------------------------------------------------------------------

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
        bytes_read = nd_sock_read(&r->sock, buffer, size);

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
            return STREAM_HANDSHAKE_DISCONNECT_NOT_SUFFICIENT_SENDER_READ_BUFFER;

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

// --------------------------------------------------------------------------------------------------------------------

static inline bool receiver_read_uncompressed(struct receiver_state *r, STREAM_HANDSHAKE *reason) {
    internal_fatal(r->reader.read_buffer[r->reader.read_len] != '\0',
                   "%s: read_buffer does not start with zero #2", __FUNCTION__ );

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

typedef enum {
    DECOMPRESS_NEED_MORE_DATA,
    DECOMPRESS_FAILED,
    DECOMPRESS_OK,
} decompressor_status_t;

static inline void receiver_move_compressed(struct receiver_state *r) {
    size_t remaining = r->thread.compressed.used - r->thread.compressed.start;
    if(remaining > 0) {
        memmove(r->thread.compressed.buf, r->thread.compressed.buf + r->thread.compressed.start, remaining);
        r->thread.compressed.start = 0;
        r->thread.compressed.used = remaining;
    }
    else {
        r->thread.compressed.start = 0;
        r->thread.compressed.used = 0;
    }
}

static inline decompressor_status_t receiver_feed_decompressor(struct receiver_state *r) {
    char *buf = r->thread.compressed.buf;
    size_t start = r->thread.compressed.start;
    size_t signature_size = r->thread.compressed.decompressor.signature_size;
    size_t used = r->thread.compressed.used;

    if(start + signature_size > used) {
        // incomplete header, we need to wait for more data
        receiver_move_compressed(r);
        return DECOMPRESS_NEED_MORE_DATA;
    }

    size_t compressed_message_size =
        stream_decompressor_start(&r->thread.compressed.decompressor, buf + start, signature_size);

    if (unlikely(!compressed_message_size)) {
        nd_log(NDLS_DAEMON, NDLP_ERR, "multiplexed uncompressed data in compressed stream!");
        return DECOMPRESS_FAILED;
    }

    if(unlikely(compressed_message_size > COMPRESSION_MAX_MSG_SIZE)) {
        nd_log(NDLS_DAEMON, NDLP_ERR,
               "received a compressed message of %zu bytes, which is bigger than the max compressed message "
               "size supported of %zu. Ignoring message.",
               compressed_message_size, (size_t)COMPRESSION_MAX_MSG_SIZE);
        return DECOMPRESS_FAILED;
    }

    if(start + signature_size + compressed_message_size > used) {
        // incomplete compressed message, we need to wait for more data
        receiver_move_compressed(r);
        return DECOMPRESS_NEED_MORE_DATA;
    }

    size_t bytes_to_parse =
        stream_decompress(&r->thread.compressed.decompressor, buf + start + signature_size, compressed_message_size);

    if (unlikely(!bytes_to_parse)) {
        nd_log(NDLS_DAEMON, NDLP_ERR, "no bytes to parse.");
        return DECOMPRESS_FAILED;
    }

    worker_set_metric(WORKER_RECEIVER_JOB_BYTES_UNCOMPRESSED, (NETDATA_DOUBLE)bytes_to_parse);

    // move the header to the next message
    r->thread.compressed.start += signature_size + compressed_message_size;

    return DECOMPRESS_OK;
}

static inline decompressor_status_t receiver_get_decompressed(struct receiver_state *r) {
    if (unlikely(!stream_decompressed_bytes_in_buffer(&r->thread.compressed.decompressor)))
        return DECOMPRESS_NEED_MORE_DATA;

    size_t available = sizeof(r->reader.read_buffer) - r->reader.read_len - 1;
    if (likely(available)) {
        size_t len = stream_decompressor_get(
            &r->thread.compressed.decompressor, r->reader.read_buffer + r->reader.read_len, available);
        if (unlikely(!len)) {
            internal_error(true, "decompressor returned zero length #1");
            return DECOMPRESS_FAILED;
        }

        r->reader.read_len += (int)len;
        r->reader.read_buffer[r->reader.read_len] = '\0';
    }
    else {
        internal_fatal(true, "The line to read is too big! Already have %zd bytes in read_buffer.", r->reader.read_len);
        return DECOMPRESS_FAILED;
    }

    return DECOMPRESS_OK;
}

static inline bool receiver_read_compressed(struct receiver_state *r, STREAM_HANDSHAKE *reason) {

    internal_fatal(r->reader.read_buffer[r->reader.read_len] != '\0',
                   "%s: read_buffer does not start with zero #2", __FUNCTION__ );

    int bytes_read = read_stream(r, r->thread.compressed.buf + r->thread.compressed.used,
                                 sizeof(r->thread.compressed.buf) - r->thread.compressed.used);

    if (unlikely(bytes_read <= 0)) {
        *reason = read_stream_error_to_reason(bytes_read);
        return false;
    }

    r->thread.compressed.used += bytes_read;
    worker_set_metric(WORKER_RECEIVER_JOB_BYTES_READ, (NETDATA_DOUBLE)bytes_read);

    return true;
}

// --------------------------------------------------------------------------------------------------------------------

static void receiver_set_exit_reason(struct receiver_state *rpt, STREAM_HANDSHAKE reason, bool force) {
    if(force || !rpt->exit.reason)
        rpt->exit.reason = reason;
}

static inline bool receiver_should_stop(struct receiver_state *rpt) {
    if(unlikely(__atomic_load_n(&rpt->exit.shutdown, __ATOMIC_RELAXED))) {
        receiver_set_exit_reason(rpt, STREAM_HANDSHAKE_DISCONNECT_SHUTDOWN, false);
        return true;
    }

    return false;
}

// --------------------------------------------------------------------------------------------------------------------

static void streaming_parser_init(struct receiver_state *rpt) {
    rpt->thread.cd = (struct plugind){
        .update_every = default_rrd_update_every,
        .unsafe = {
            .spinlock = NETDATA_SPINLOCK_INITIALIZER,
            .running = true,
            .enabled = true,
        },
        .started_t = now_realtime_sec(),
    };

    // put the client IP and port into the buffers used by plugins.d
    snprintfz(rpt->thread.cd.id,           CONFIG_MAX_NAME,  "%s:%s", rpt->client_ip, rpt->client_port);
    snprintfz(rpt->thread.cd.filename,     FILENAME_MAX,     "%s:%s", rpt->client_ip, rpt->client_port);
    snprintfz(rpt->thread.cd.fullfilename, FILENAME_MAX,     "%s:%s", rpt->client_ip, rpt->client_port);
    snprintfz(rpt->thread.cd.cmd,          PLUGINSD_CMD_MAX, "%s:%s", rpt->client_ip, rpt->client_port);

    PARSER *parser = NULL;
    {
        PARSER_USER_OBJECT user = {
            .enabled = plugin_is_enabled(&rpt->thread.cd),
            .host = rpt->host,
            .opaque = rpt,
            .cd = &rpt->thread.cd,
            .trust_durations = 1,
            .capabilities = rpt->capabilities,
        };

        parser = parser_init(&user, -1, -1, PARSER_INPUT_SPLIT, &rpt->sock);
    }

#ifdef ENABLE_H2O
    parser->h2o_ctx = rpt->h2o_ctx;
#endif

    pluginsd_keywords_init(parser, PARSER_INIT_STREAMING);

    rrd_collector_started();

    rpt->thread.compressed.start = 0;
    rpt->thread.compressed.used = 0;
    rpt->thread.compressed.enabled = stream_decompression_initialize(rpt);
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

    __atomic_store_n(&rpt->thread.parser, parser, __ATOMIC_RELAXED);
    stream_receiver_send_node_and_claim_id_to_child(rpt->host);

    rpt->thread.buffer = buffer_create(sizeof(rpt->reader.read_buffer), NULL);

    // help rrdset_push_metric_initialize() select the right buffer
    rpt->host->stream.snd.commit.receiver_tid = gettid_cached();
}

// --------------------------------------------------------------------------------------------------------------------

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

    buffer_strcat(wb, nd_sock_is_ssl(&rpt->sock) ? "https" : "http");
    return true;
}

// --------------------------------------------------------------------------------------------------------------------

void stream_receiver_move_queue_to_running_unsafe(struct stream_thread *sth) {
    internal_fatal(sth->tid != gettid_cached(), "Function %s() should only be used by the dispatcher thread", __FUNCTION__ );

    // process the queue
    Word_t idx = 0;
    for(struct receiver_state *rpt = RECEIVERS_FIRST(&sth->queue.receivers, &idx);
         rpt;
         rpt = RECEIVERS_NEXT(&sth->queue.receivers, &idx)) {
        worker_is_busy(WORKER_STREAM_JOB_DEQUEUE);

        RECEIVERS_DEL(&sth->queue.receivers, (Word_t)rpt);

        ND_LOG_STACK lgs[] = {
            ND_LOG_FIELD_STR(NDF_NIDL_NODE, rpt->host->hostname),
            ND_LOG_FIELD_UUID(NDF_MESSAGE_ID, &streaming_to_parent_msgid),
            ND_LOG_FIELD_END(),
        };
        ND_LOG_STACK_PUSH(lgs);

        nd_log(NDLS_DAEMON, NDLP_DEBUG,
               "STREAM RECEIVE[%zu] [%s]: moving host from receiver queue to receiver running...",
               sth->id, rrdhost_hostname(rpt->host));

        internal_fatal(RECEIVERS_GET(&sth->rcv.receivers, (Word_t)rpt) != NULL, "Receiver to be added is already in the list of receivers");
        RECEIVERS_SET(&sth->rcv.receivers, (Word_t)rpt, rpt);

        streaming_parser_init(rpt);

        rpt->host->stream.rcv.status.tid = gettid_cached();
        rpt->thread.pfd = stream_thread_pollfd_get(sth, rpt->sock.fd, POLLFD_TYPE_RECEIVER, rpt, NULL);
    }
}

static void stream_receiver_on_disconnect(struct stream_thread *sth __maybe_unused, struct receiver_state *rpt) {
    internal_fatal(sth->tid != gettid_cached(), "Function %s() should only be used by the dispatcher thread", __FUNCTION__ );
    if(!rpt) return;

    buffer_free(rpt->thread.buffer);
    rpt->thread.buffer = NULL;

    size_t count = 0;
    PARSER *parser = __atomic_load_n(&rpt->thread.parser, __ATOMIC_RELAXED);
    if(parser) {
        parser->user.v2.stream_buffer.wb = NULL;

        // make sure send_to_plugin() will not write any data to the socket
        spinlock_lock(&parser->writer.spinlock);
        parser->fd_input = -1;
        parser->fd_output = -1;
        parser->sock = NULL;
        spinlock_unlock(&parser->writer.spinlock);

        count = parser->user.data_collections_count;
    }

    // the parser stopped
    receiver_set_exit_reason(rpt, STREAM_HANDSHAKE_DISCONNECT_PARSER_EXIT, false);

    {
        char msg[100 + 1];
        snprintfz(msg, sizeof(msg) - 1, "disconnected (completed %zu updates)", count);
        stream_receiver_log_status(rpt, msg, STREAM_STATUS_DISCONNECTED, NDLP_WARNING);
    }

    // in case we have cloud connection we inform cloud
    // a child disconnected
    uint64_t total_reboot = rrdhost_stream_path_total_reboot_time_ms(rpt->host);
    schedule_node_state_update(rpt->host, MIN((total_reboot * MAX_CHILD_DISC_TOLERANCE), MAX_CHILD_DISC_DELAY));

    rrdhost_clear_receiver(rpt);
    rrdhost_set_is_parent_label();
    stream_receiver_free(rpt);
}

static void stream_receiver_remove(struct stream_thread *sth, struct receiver_state *rpt, const char *why) {
    internal_fatal(sth->tid != gettid_cached(), "Function %s() should only be used by the dispatcher thread", __FUNCTION__ );

    nd_log(NDLS_DAEMON, NDLP_ERR,
           "STREAM RECEIVE[%zu] '%s' [from [%s]:%s]: "
           "receiver disconnected: %s"
           , sth->id
           , rpt->hostname ? rpt->hostname : "-"
           , rpt->client_ip ? rpt->client_ip : "-"
           , rpt->client_port ? rpt->client_port : "-"
           , why ? why : "");

    internal_fatal(RECEIVERS_GET(&sth->rcv.receivers, (Word_t)rpt) == NULL, "Receiver to be removed is not found in the list of receivers");
    RECEIVERS_DEL(&sth->rcv.receivers, (Word_t)rpt);
    stream_thread_pollfd_release(sth, rpt->thread.pfd);

    rpt->thread.pfd = PFD_EMPTY;
    rpt->host->stream.rcv.status.tid = 0;

    stream_thread_node_removed(rpt->host);

    stream_receiver_on_disconnect(sth, rpt);
    // DO NOT USE rpt after this point
}

// process poll() events for streaming receivers
void stream_receive_process_poll_events(struct stream_thread *sth, struct receiver_state *rpt, short revents __maybe_unused, time_t now_s) {
        PARSER *parser = __atomic_load_n(&rpt->thread.parser, __ATOMIC_RELAXED);
        ND_LOG_STACK lgs[] = {
            ND_LOG_FIELD_TXT(NDF_SRC_IP, rpt->client_ip),
            ND_LOG_FIELD_TXT(NDF_SRC_PORT, rpt->client_port),
            ND_LOG_FIELD_TXT(NDF_NIDL_NODE, rpt->hostname),
            ND_LOG_FIELD_CB(NDF_SRC_TRANSPORT, stream_receiver_log_transport, rpt),
            ND_LOG_FIELD_CB(NDF_SRC_CAPABILITIES, stream_receiver_log_capabilities, rpt),
            ND_LOG_FIELD_CB(NDF_REQUEST, line_splitter_reconstruct_line, &parser->line),
            ND_LOG_FIELD_CB(NDF_NIDL_NODE, parser_reconstruct_node, parser),
            ND_LOG_FIELD_CB(NDF_NIDL_INSTANCE, parser_reconstruct_instance, parser),
            ND_LOG_FIELD_CB(NDF_NIDL_CONTEXT, parser_reconstruct_context, parser),
            ND_LOG_FIELD_END(),
        };
        ND_LOG_STACK_PUSH(lgs);

        if(receiver_should_stop(rpt)) {
            receiver_set_exit_reason(rpt, rpt->exit.reason, false);
            stream_receiver_remove(sth, rpt, "received stop signal");
            return;
        }

        rpt->last_msg_t = now_s;

        if(rpt->thread.compressed.enabled) {
            worker_is_busy(WORKER_STREAM_JOB_SOCKET_RECEIVE);

            STREAM_HANDSHAKE reason = STREAM_HANDSHAKE_DISCONNECT_UNKNOWN_SOCKET_READ_ERROR;
            if(unlikely(!receiver_read_compressed(rpt, &reason))) {
                worker_is_busy(WORKER_STREAM_JOB_SOCKET_ERROR);
                receiver_set_exit_reason(rpt, reason, false);
                stream_receiver_remove(sth, rpt, "socket read error");
                return;
            }

            bool node_removed = false;
            while(!node_removed && !nd_thread_signaled_to_cancel() && service_running(SERVICE_STREAMING) && !receiver_should_stop(rpt)) {
                worker_is_busy(WORKER_STREAM_JOB_DECOMPRESS);

                // feed the decompressor with the new data we just read
                decompressor_status_t feed = receiver_feed_decompressor(rpt);

                if(likely(feed == DECOMPRESS_OK)) {
                    while (!node_removed) {
                        // feed our uncompressed data buffer with new data
                        decompressor_status_t rc = receiver_get_decompressed(rpt);

                        if (likely(rc == DECOMPRESS_OK)) {
                            // loop through all the complete lines found in the uncompressed buffer

                            while (buffered_reader_next_line(&rpt->reader, rpt->thread.buffer)) {
                                if (unlikely(parser_action(parser, rpt->thread.buffer->buffer))) {
                                    receiver_set_exit_reason(rpt, STREAM_HANDSHAKE_DISCONNECT_PARSER_FAILED, false);
                                    stream_receiver_remove(sth, rpt, "parser failed");
                                    node_removed = true;
                                    break;
                                }

                                rpt->thread.buffer->len = 0;
                                rpt->thread.buffer->buffer[0] = '\0';
                            }
                        }
                        else if (rc == DECOMPRESS_NEED_MORE_DATA)
                            break;

                        else {
                            receiver_set_exit_reason(rpt, STREAM_HANDSHAKE_DISCONNECT_PARSER_FAILED, false);
                            stream_receiver_remove(sth, rpt, "decompressor failed");
                            node_removed = true;
                            break;
                        }
                    }
                }
                else if (feed == DECOMPRESS_NEED_MORE_DATA)
                    break;
                else {
                    receiver_set_exit_reason(rpt, STREAM_HANDSHAKE_DISCONNECT_PARSER_FAILED, false);
                    stream_receiver_remove(sth, rpt, "compressed data invalid");
                    node_removed = true;
                    break;
                }
            }

            if(!node_removed && receiver_should_stop(rpt)) {
                receiver_set_exit_reason(rpt, rpt->exit.reason, false);
                stream_receiver_remove(sth, rpt, "received stop signal");
                return;
            }
        }
        else {
            worker_is_busy(WORKER_STREAM_JOB_SOCKET_RECEIVE);

            STREAM_HANDSHAKE reason = STREAM_HANDSHAKE_DISCONNECT_UNKNOWN_SOCKET_READ_ERROR;
            if(unlikely(!receiver_read_uncompressed(rpt, &reason))) {
                worker_is_busy(WORKER_STREAM_JOB_SOCKET_ERROR);
                receiver_set_exit_reason(rpt, reason, false);
                stream_receiver_remove(sth, rpt, "socker read error");
                return;
            }

            while(buffered_reader_next_line(&rpt->reader, rpt->thread.buffer)) {
                if(unlikely(parser_action(parser, rpt->thread.buffer->buffer))) {
                    receiver_set_exit_reason(rpt, STREAM_HANDSHAKE_DISCONNECT_PARSER_FAILED, false);
                    stream_receiver_remove(sth, rpt, "parser failed");
                    break;
                }

                rpt->thread.buffer->len = 0;
                rpt->thread.buffer->buffer[0] = '\0';
            }
        }
}

void stream_receiver_cleanup(struct stream_thread *sth) {
    Word_t idx = 0;
    for(struct receiver_state *rpt = RECEIVERS_FIRST(&sth->rcv.receivers, &idx);
         rpt;
         rpt = RECEIVERS_NEXT(&sth->rcv.receivers, &idx))
        stream_receiver_remove(sth, rpt, "shutdown");

    RECEIVERS_FREE(&sth->rcv.receivers, NULL);
}

static void stream_receiver_replication_reset(RRDHOST *host) {
    RRDSET *st;
    rrdset_foreach_read(st, host) {
        rrdset_flag_clear(st, RRDSET_FLAG_RECEIVER_REPLICATION_IN_PROGRESS);
        rrdset_flag_set(st, RRDSET_FLAG_RECEIVER_REPLICATION_FINISHED);
    }
    rrdset_foreach_done(st);
    rrdhost_receiver_replicating_charts_zero(host);
}

bool rrdhost_set_receiver(RRDHOST *host, struct receiver_state *rpt) {
    bool signal_rrdcontext = false;
    bool set_this = false;

    rrdhost_receiver_lock(host);

    if (!host->receiver) {
        rrdhost_flag_clear(host, RRDHOST_FLAG_ORPHAN);

        host->stream.rcv.status.connections++;
        streaming_receiver_connected();

        host->receiver = rpt;
        rpt->host = host;

        __atomic_store_n(&rpt->exit.shutdown, false, __ATOMIC_RELAXED);
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
//            stream_sender_thread_stop(host, "HOPS MISMATCH", false);

        signal_rrdcontext = true;
        stream_receiver_replication_reset(host);

        rrdhost_flag_clear(rpt->host, RRDHOST_FLAG_STREAM_RECEIVER_DISCONNECTED);
        aclk_queue_node_info(rpt->host, true);

        rrdhost_stream_parents_reset(host, STREAM_HANDSHAKE_PREPARING);

        set_this = true;
    }

    rrdhost_receiver_unlock(host);

    if(signal_rrdcontext)
        rrdcontext_host_child_connected(host);

    return set_this;
}

void rrdhost_clear_receiver(struct receiver_state *rpt) {
    RRDHOST *host = rpt->host;
    if(!host) return;

    rrdhost_receiver_lock(host);
    {
        // Make sure that we detach this thread and don't kill a freshly arriving receiver

        if (host->receiver == rpt) {
            rrdhost_flag_set(host, RRDHOST_FLAG_STREAM_RECEIVER_DISCONNECTED);
            rrdhost_receiver_unlock(host);
            {
                // run all these without having the receiver lock

                stream_path_child_disconnected(host);
                stream_sender_signal_to_stop_and_wait(host, STREAM_HANDSHAKE_DISCONNECT_RECEIVER_LEFT, false);
                stream_receiver_replication_reset(host);
                rrdcontext_host_child_disconnected(host);

                if (rpt->config.health.enabled)
                    rrdcalc_child_disconnected(host);

                rrdhost_stream_parents_reset(host, STREAM_HANDSHAKE_DISCONNECT_RECEIVER_LEFT);
            }
            rrdhost_receiver_lock(host);

            // now we have the lock again

            streaming_receiver_disconnected();

            __atomic_store_n(&host->receiver->exit.shutdown, false, __ATOMIC_RELAXED);
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
    pluginsd_process_cleanup(rpt->thread.parser);
    __atomic_store_n(&rpt->thread.parser, NULL, __ATOMIC_RELAXED);

    rrdhost_receiver_unlock(host);
}

bool stream_receiver_signal_to_stop_and_wait(RRDHOST *host, STREAM_HANDSHAKE reason) {
    bool ret = false;

    rrdhost_receiver_lock(host);

    if(host->receiver) {
        if(!__atomic_load_n(&host->receiver->exit.shutdown, __ATOMIC_RELAXED)) {
            __atomic_store_n(&host->receiver->exit.shutdown, true, __ATOMIC_RELAXED);
            receiver_set_exit_reason(host->receiver, reason, true);
            shutdown(host->receiver->sock.fd, SHUT_RDWR);
        }
    }

    int count = 2000;
    while (host->receiver && count-- > 0) {
        rrdhost_receiver_unlock(host);

        // let the lock for the receiver thread to exit
        sleep_usec(1 * USEC_PER_MS);

        rrdhost_receiver_lock(host);
    }

    if(host->receiver)
        netdata_log_error("STREAM RECEIVE[x] '%s' [from [%s]:%s]: "
              "streaming thread takes too long to stop, giving up..."
              , rrdhost_hostname(host)
              , host->receiver->client_ip, host->receiver->client_port);
    else
        ret = true;

    rrdhost_receiver_unlock(host);

    return ret;
}
