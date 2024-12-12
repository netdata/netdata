// SPDX-License-Identifier: GPL-3.0-or-later

#include "stream.h"
#include "stream-thread.h"
#include "stream-receiver-internals.h"
#include "web/server/h2o/http_server.h"

static void stream_receiver_remove(struct stream_thread *sth, struct receiver_state *rpt, const char *why);

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

static inline ssize_t write_stream(struct receiver_state *r, char* buffer, size_t size) {
    if(unlikely(!size)) {
        internal_error(true, "%s() asked to read zero bytes", __FUNCTION__);
        errno_clear();
        return -2;
    }

#ifdef ENABLE_H2O
    if (is_h2o_rrdpush(r)) {
        if(nd_thread_signaled_to_cancel()) {
            errno_clear();
            return -3;
        }

        return (ssize_t)h2o_stream_write(r->h2o_ctx, buffer, size);
    }
#endif

    ssize_t bytes_written = nd_sock_send_nowait(&r->sock, buffer, size);
    return bytes_written;
}

static inline ssize_t read_stream(struct receiver_state *r, char* buffer, size_t size) {
    if(unlikely(!size)) {
        internal_error(true, "%s() asked to read zero bytes", __FUNCTION__);
        errno_clear();
        return -2;
    }

#ifdef ENABLE_H2O
    if (is_h2o_rrdpush(r)) {
        if(nd_thread_signaled_to_cancel()) {
            errno_clear();
            return -3;
        }

        return (ssize_t)h2o_stream_read(r->h2o_ctx, buffer, size);
    }
#endif

    ssize_t bytes_read = nd_sock_revc_nowait(&r->sock, buffer, size);
    return bytes_read;
}

// --------------------------------------------------------------------------------------------------------------------

static inline ssize_t receiver_read_uncompressed(struct receiver_state *r) {
    internal_fatal(r->reader.read_buffer[r->reader.read_len] != '\0',
                   "%s: read_buffer does not start with zero #2", __FUNCTION__ );

    ssize_t bytes = read_stream(r, r->reader.read_buffer + r->reader.read_len, sizeof(r->reader.read_buffer) - r->reader.read_len - 1);
    if(bytes > 0) {
        worker_set_metric(WORKER_RECEIVER_JOB_BYTES_READ, (NETDATA_DOUBLE)bytes);
        worker_set_metric(WORKER_RECEIVER_JOB_BYTES_UNCOMPRESSED, (NETDATA_DOUBLE)bytes);

        r->reader.read_len += bytes;
        r->reader.read_buffer[r->reader.read_len] = '\0';
    }

    return bytes;
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
        nd_log(NDLS_DAEMON, NDLP_ERR,
               "STREAM RCV[x] '%s' [from [%s]:%s]: multiplexed uncompressed data in compressed stream!",
               rrdhost_hostname(r->host), r->client_ip, r->client_port);
        return DECOMPRESS_FAILED;
    }

    if(unlikely(compressed_message_size > COMPRESSION_MAX_MSG_SIZE)) {
        nd_log(NDLS_DAEMON, NDLP_ERR,
               "STREAM RCV[x] '%s' [from [%s]:%s]: received a compressed message of %zu bytes, "
               "which is bigger than the max compressed message "
               "size supported of %zu. Ignoring message.",
               rrdhost_hostname(r->host), r->client_ip, r->client_port,
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
        nd_log(NDLS_DAEMON, NDLP_ERR,
               "STREAM RCV[x] '%s' [from [%s]:%s]: no bytes to decompress.",
               rrdhost_hostname(r->host), r->client_ip, r->client_port);
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

static inline ssize_t receiver_read_compressed(struct receiver_state *r) {

    internal_fatal(r->reader.read_buffer[r->reader.read_len] != '\0',
                   "%s: read_buffer does not start with zero #2", __FUNCTION__ );

    ssize_t bytes_read = read_stream(r, r->thread.compressed.buf + r->thread.compressed.used,
                                 r->thread.compressed.size - r->thread.compressed.used);

    if(bytes_read > 0) {
        r->thread.compressed.used += bytes_read;
        worker_set_metric(WORKER_RECEIVER_JOB_BYTES_READ, (NETDATA_DOUBLE)bytes_read);
    }

    return bytes_read;
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

void stream_receiver_handle_op(struct stream_thread *sth, struct receiver_state *rpt, struct stream_opcode *msg) {
    ND_LOG_STACK lgs[] = {
        ND_LOG_FIELD_STR(NDF_NIDL_NODE, rpt->host->hostname),
        ND_LOG_FIELD_TXT(NDF_SRC_IP, rpt->client_ip),
        ND_LOG_FIELD_TXT(NDF_SRC_PORT, rpt->client_port),
        ND_LOG_FIELD_CB(NDF_SRC_TRANSPORT, stream_receiver_log_transport, rpt),
        ND_LOG_FIELD_CB(NDF_SRC_CAPABILITIES, stream_receiver_log_capabilities, rpt),
        ND_LOG_FIELD_UUID(NDF_MESSAGE_ID, &streaming_to_parent_msgid),
        ND_LOG_FIELD_END(),
    };
    ND_LOG_STACK_PUSH(lgs);

    if(msg->opcode & STREAM_OPCODE_RECEIVER_BUFFER_OVERFLOW) {
        worker_is_busy(WORKER_SENDER_JOB_DISCONNECT_OVERFLOW);
        errno_clear();
        spinlock_lock(&rpt->thread.send_to_child.spinlock);
        // copy the statistics
        STREAM_CIRCULAR_BUFFER_STATS stats = *stream_circular_buffer_stats_unsafe(rpt->thread.send_to_child.scb);
        spinlock_unlock(&rpt->thread.send_to_child.spinlock);
        nd_log(NDLS_DAEMON, NDLP_ERR,
               "STREAM RCV[%zu] '%s' [from [%s]:%s]: send buffer is full (buffer size %u, max %u, used %u, available %u). "
               "Restarting connection.",
               sth->id, rrdhost_hostname(rpt->host), rpt->client_ip, rpt->client_port,
               stats.bytes_size, stats.bytes_max_size, stats.bytes_outstanding, stats.bytes_available);

        stream_receiver_remove(sth, rpt, "receiver send buffer overflow");
        return;
    }

    nd_log(NDLS_DAEMON, NDLP_ERR,
           "STREAM RCV[%zu]: invalid msg id %u", sth->id, (unsigned)msg->opcode);
}

static ssize_t send_to_child(const char *txt, void *data, STREAM_TRAFFIC_TYPE type) {
    struct receiver_state *rpt = data;
    if(!rpt || rpt->thread.meta.type != POLLFD_TYPE_RECEIVER || !rpt->thread.send_to_child.scb)
        return 0;

    spinlock_lock(&rpt->thread.send_to_child.spinlock);
    STREAM_CIRCULAR_BUFFER *scb = rpt->thread.send_to_child.scb;
    STREAM_CIRCULAR_BUFFER_STATS *stats = stream_circular_buffer_stats_unsafe(scb);
    bool was_empty = stats->bytes_outstanding == 0;
    struct stream_opcode msg = rpt->thread.send_to_child.msg;
    msg.opcode = STREAM_OPCODE_NONE;

    size_t size = strlen(txt);
    ssize_t rc = (ssize_t)size;
    if(!stream_circular_buffer_add_unsafe(scb, txt, size, size, type, true)) {
        // should never happen, because of autoscaling
        msg.opcode = STREAM_OPCODE_RECEIVER_BUFFER_OVERFLOW;
        rc = -1;
    }
    else if(was_empty)
        msg.opcode = STREAM_OPCODE_RECEIVER_POLLOUT;

    spinlock_unlock(&rpt->thread.send_to_child.spinlock);

    if(msg.opcode != STREAM_OPCODE_NONE)
        stream_receiver_send_opcode(rpt, msg);

    return rc;
}

static void streaming_parser_init(struct receiver_state *rpt) {
    rpt->thread.cd = (struct plugind){
        .update_every = default_rrd_update_every,
        .unsafe = {
            .spinlock = SPINLOCK_INITIALIZER,
            .running = true,
            .enabled = true,
        },
        .started_t = now_realtime_sec(),
    };

    // put the client IP and port into the buffers used by plugins.d
    {
        char buf[CONFIG_MAX_NAME];
        snprintfz(buf, sizeof(buf),  "[%s]:%s", rpt->client_ip, rpt->client_port);
        string_freez(rpt->thread.cd.id);
        rpt->thread.cd.id = string_strdupz(buf);

        string_freez(rpt->thread.cd.filename);
        rpt->thread.cd.filename = string_strdupz(buf);

        string_freez(rpt->thread.cd.fullfilename);
        rpt->thread.cd.fullfilename = string_strdupz(buf);

        string_freez(rpt->thread.cd.cmd);
        rpt->thread.cd.cmd = string_strdupz(buf);
    }

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
        parser->send_to_plugin_data = rpt;
        parser->send_to_plugin_cb = send_to_child;
    }

#ifdef ENABLE_H2O
    parser->h2o_ctx = rpt->h2o_ctx;
#endif

    pluginsd_keywords_init(parser, PARSER_INIT_STREAMING);

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

static void stream_receive_log_database_gap(struct receiver_state *rpt) {
    RRDHOST *host = rpt->host;

    time_t now = now_realtime_sec();
    time_t last_db_entry = 0;
    rrdhost_retention(host, now, false, NULL, &last_db_entry);

    if(now < last_db_entry)
        last_db_entry = now;

    char buf[128];
    duration_snprintf(buf, sizeof(buf), now - last_db_entry, "s", true);
    nd_log(NDLS_DAEMON, NDLP_NOTICE,
           "STREAM RCV '%s' [from [%s]:%s]: node connected; last sample in the database %s ago",
           rrdhost_hostname(host), rpt->client_ip, rpt->client_port, buf);
}

void stream_receiver_move_to_running_unsafe(struct stream_thread *sth, struct receiver_state *rpt) {
    internal_fatal(sth->tid != gettid_cached(), "Function %s() should only be used by the dispatcher thread", __FUNCTION__ );

    worker_is_busy(WORKER_STREAM_JOB_DEQUEUE);

    ND_LOG_STACK lgs[] = {
        ND_LOG_FIELD_STR(NDF_NIDL_NODE, rpt->host->hostname),
        ND_LOG_FIELD_UUID(NDF_MESSAGE_ID, &streaming_to_parent_msgid),
        ND_LOG_FIELD_END(),
    };
    ND_LOG_STACK_PUSH(lgs);

    nd_log(NDLS_DAEMON, NDLP_DEBUG,
           "STREAM RCV[%zu] '%s' [from [%s]:%s]: moving host from receiver queue to receiver running...",
           sth->id, rrdhost_hostname(rpt->host), rpt->client_ip, rpt->client_port);

    rpt->host->stream.rcv.status.tid = gettid_cached();
    rpt->thread.meta.type = POLLFD_TYPE_RECEIVER;
    rpt->thread.meta.rpt = rpt;

    spinlock_lock(&rpt->thread.send_to_child.spinlock);
    rpt->thread.send_to_child.scb = stream_circular_buffer_create();
    rpt->thread.send_to_child.msg.thread_slot = (int32_t)sth->id;
    rpt->thread.send_to_child.msg.session = os_random32();
    rpt->thread.send_to_child.msg.meta = &rpt->thread.meta;
    spinlock_unlock(&rpt->thread.send_to_child.spinlock);

    internal_fatal(META_GET(&sth->run.meta, (Word_t)&rpt->thread.meta) != NULL, "Receiver to be added is already in the list of receivers");
    META_SET(&sth->run.meta, (Word_t)&rpt->thread.meta, &rpt->thread.meta);

    if(sock_setnonblock(rpt->sock.fd) < 0)
        nd_log(NDLS_DAEMON, NDLP_ERR,
               "STREAM RCV '%s' [from [%s]:%s]: cannot set the non-blocking flag from socket %d",
               rrdhost_hostname(rpt->host), rpt->client_ip, rpt->client_port, rpt->sock.fd);

    if(!nd_poll_add(sth->run.ndpl, rpt->sock.fd, ND_POLL_READ, &rpt->thread.meta))
        nd_log(NDLS_DAEMON, NDLP_ERR,
               "STREAM RCV[%zu] '%s' [from [%s]:%s]:"
               "Failed to add receiver socket to nd_poll()",
               sth->id, rrdhost_hostname(rpt->host), rpt->client_ip, rpt->client_port);

    stream_receive_log_database_gap(rpt);

    // keep this last, since it sends commands back to the child
    streaming_parser_init(rpt);
}

void stream_receiver_move_entire_queue_to_running_unsafe(struct stream_thread *sth) {
    internal_fatal(sth->tid != gettid_cached(), "Function %s() should only be used by the dispatcher thread", __FUNCTION__ );

    // process the queue
    Word_t idx = 0;
    for(struct receiver_state *rpt = RECEIVERS_FIRST(&sth->queue.receivers, &idx);
         rpt;
         rpt = RECEIVERS_NEXT(&sth->queue.receivers, &idx)) {
        RECEIVERS_DEL(&sth->queue.receivers, idx);
        stream_receiver_move_to_running_unsafe(sth, rpt);
    }
}

static void stream_receiver_remove(struct stream_thread *sth, struct receiver_state *rpt, const char *why) {
    internal_fatal(sth->tid != gettid_cached(), "Function %s() should only be used by the dispatcher thread", __FUNCTION__ );

    errno_clear();
    nd_log(NDLS_DAEMON, NDLP_ERR,
           "STREAM RCV[%zu] '%s' [from [%s]:%s]: "
           "receiver disconnected: %s"
           , sth->id
           , rpt->hostname ? rpt->hostname : "-"
           , rpt->client_ip ? rpt->client_ip : "-"
           , rpt->client_port ? rpt->client_port : "-"
           , why ? why : "");

    internal_fatal(META_GET(&sth->run.meta, (Word_t)&rpt->thread.meta) == NULL, "Receiver to be removed is not found in the list of receivers");
    META_DEL(&sth->run.meta, (Word_t)&rpt->thread.meta);

    if(!nd_poll_del(sth->run.ndpl, rpt->sock.fd))
        nd_log(NDLS_DAEMON, NDLP_ERR, "Failed to delete receiver socket from nd_poll()");

    rpt->host->stream.rcv.status.tid = 0;

    spinlock_lock(&rpt->thread.send_to_child.spinlock);
    rpt->thread.send_to_child.msg.session = 0;
    rpt->thread.send_to_child.msg.meta = NULL;
    stream_circular_buffer_destroy(rpt->thread.send_to_child.scb);
    rpt->thread.send_to_child.scb = NULL;
    spinlock_unlock(&rpt->thread.send_to_child.spinlock);

    stream_thread_node_removed(rpt->host);

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
        snprintfz(msg, sizeof(msg) - 1, "receiver disconnected (completed %zu updates)", count);
        stream_receiver_log_status(rpt, msg, STREAM_STATUS_DISCONNECTED, NDLP_WARNING);
    }

    // in case we are connected to netdata cloud,
    // we inform cloud that a child got disconnected
    uint64_t total_reboot = rrdhost_stream_path_total_reboot_time_ms(rpt->host);
    schedule_node_state_update(rpt->host, MIN((total_reboot * MAX_CHILD_DISC_TOLERANCE), MAX_CHILD_DISC_DELAY));

    rrdhost_clear_receiver(rpt);
    rrdhost_set_is_parent_label();

    stream_receiver_free(rpt);
    // DO NOT USE rpt after this point
}

static ssize_t
stream_receive_and_process(struct stream_thread *sth, struct receiver_state *rpt, PARSER *parser, bool *removed) {
    internal_fatal(sth->tid != gettid_cached(), "Function %s() should only be used by the dispatcher thread", __FUNCTION__);

    ssize_t rc;
    if(rpt->thread.compressed.enabled) {
        rc = receiver_read_compressed(rpt);
        if(unlikely(rc <= 0))
            return rc;

        while(!nd_thread_signaled_to_cancel() && service_running(SERVICE_STREAMING) && !receiver_should_stop(rpt)) {
            worker_is_busy(WORKER_STREAM_JOB_DECOMPRESS);

            // feed the decompressor with the new data we just read
            decompressor_status_t feed_rc = receiver_feed_decompressor(rpt);

            if(likely(feed_rc == DECOMPRESS_OK)) {
                while (true) {
                    // feed our uncompressed data buffer with new data
                    decompressor_status_t decompress_rc = receiver_get_decompressed(rpt);

                    if (likely(decompress_rc == DECOMPRESS_OK)) {
                        // loop through all the complete lines found in the uncompressed buffer

                        while (buffered_reader_next_line(&rpt->reader, rpt->thread.buffer)) {
                            if (unlikely(parser_action(parser, rpt->thread.buffer->buffer))) {
                                receiver_set_exit_reason(rpt, STREAM_HANDSHAKE_DISCONNECT_PARSER_FAILED, false);
                                stream_receiver_remove(sth, rpt, "parser action failed");
                                *removed = true;
                                return -1;
                            }

                            rpt->thread.buffer->len = 0;
                            rpt->thread.buffer->buffer[0] = '\0';
                        }
                    }
                    else if (decompress_rc == DECOMPRESS_NEED_MORE_DATA)
                        break;

                    else {
                        receiver_set_exit_reason(rpt, STREAM_HANDSHAKE_DISCONNECT_PARSER_FAILED, false);
                        stream_receiver_remove(sth, rpt, "receiver decompressor failed");
                        *removed = true;
                        return -1;
                    }
                }
            }
            else if (feed_rc == DECOMPRESS_NEED_MORE_DATA)
                break;
            else {
                receiver_set_exit_reason(rpt, STREAM_HANDSHAKE_DISCONNECT_PARSER_FAILED, false);
                stream_receiver_remove(sth, rpt, "receiver compressed data invalid");
                *removed = true;
                return -1;
            }
        }

        if(receiver_should_stop(rpt)) {
            receiver_set_exit_reason(rpt, rpt->exit.reason, false);
            stream_receiver_remove(sth, rpt, "received stop signal");
            *removed = true;
            return -1;
        }
    }
    else {
        rc = receiver_read_uncompressed(rpt);
        if(rc <= 0) return rc;

        while(buffered_reader_next_line(&rpt->reader, rpt->thread.buffer)) {
            if(unlikely(parser_action(parser, rpt->thread.buffer->buffer))) {
                receiver_set_exit_reason(rpt, STREAM_HANDSHAKE_DISCONNECT_PARSER_FAILED, false);
                stream_receiver_remove(sth, rpt, "parser action failed");
                *removed = true;
                return -1;
            }

            rpt->thread.buffer->len = 0;
            rpt->thread.buffer->buffer[0] = '\0';
        }
    }

    return rc;
}

// process poll() events for streaming receivers
// returns true when the receiver is still there, false if it removed it
bool stream_receive_process_poll_events(struct stream_thread *sth, struct receiver_state *rpt, nd_poll_event_t events, usec_t now_ut)
{
    internal_fatal(sth->tid != gettid_cached(), "Function %s() should only be used by the dispatcher thread", __FUNCTION__);

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

    if (receiver_should_stop(rpt)) {
        receiver_set_exit_reason(rpt, rpt->exit.reason, false);
        stream_receiver_remove(sth, rpt, "received stop signal");
        return false;
    }

    if (unlikely(events & (ND_POLL_ERROR | ND_POLL_HUP | ND_POLL_INVALID))) {
        // we have errors on this socket

        worker_is_busy(WORKER_STREAM_JOB_SOCKET_ERROR);

        char *error = "unknown error";

        if (events & ND_POLL_ERROR)
            error = "socket reports errors";
        else if (events & ND_POLL_HUP)
            error = "connection closed by remote end (HUP)";
        else if (events & ND_POLL_INVALID)
            error = "connection is invalid";

        worker_is_busy(WORKER_SENDER_JOB_DISCONNECT_SOCKET_ERROR);

        nd_log(NDLS_DAEMON, NDLP_ERR,
               "STREAM RCV[%zu] '%s' [from [%s]:%s]: %s - closing connection",
               sth->id, rrdhost_hostname(rpt->host), rpt->client_ip, rpt->client_port, error);

        receiver_set_exit_reason(rpt, STREAM_HANDSHAKE_DISCONNECT_SOCKET_ERROR, false);
        stream_receiver_remove(sth, rpt, error);
        return false;
    }

    if (events & ND_POLL_WRITE) {
        worker_is_busy(WORKER_STREAM_JOB_SOCKET_SEND);

        bool stop = false;
        while(!stop) {
            if (spinlock_trylock(&rpt->thread.send_to_child.spinlock)) {
                const char *disconnect_reason = NULL;
                STREAM_HANDSHAKE reason;

                char *chunk;
                STREAM_CIRCULAR_BUFFER *scb = rpt->thread.send_to_child.scb;
                STREAM_CIRCULAR_BUFFER_STATS *stats = stream_circular_buffer_stats_unsafe(scb);
                size_t outstanding = stream_circular_buffer_get_unsafe(scb, &chunk);
                ssize_t rc = write_stream(rpt, chunk, outstanding);
                if (likely(rc > 0)) {
                    stream_circular_buffer_del_unsafe(scb, rc);
                    if (!stats->bytes_outstanding) {
                        if (!nd_poll_upd(sth->run.ndpl, rpt->sock.fd, ND_POLL_READ, &rpt->thread.meta))
                            nd_log(NDLS_DAEMON, NDLP_ERR,
                                   "STREAM RCV[%zu] '%s' [from [%s]:%s]: cannot update nd_poll()",
                                   sth->id, rrdhost_hostname(rpt->host), rpt->client_ip, rpt->client_port);

                        // recreate the circular buffer if we have to
                        stream_circular_buffer_recreate_timed_unsafe(rpt->thread.send_to_child.scb, now_ut, false);
                        stop = true;
                    }
                    else if(stream_thread_process_opcodes(sth, &rpt->thread.meta))
                        stop = true;
                }
                else if (rc == 0 || errno == ECONNRESET) {
                    disconnect_reason = "socket reports EOF (closed by child)";
                    reason = STREAM_HANDSHAKE_DISCONNECT_SOCKET_CLOSED_BY_REMOTE_END;
                }
                else if (rc < 0) {
                    if (errno == EWOULDBLOCK || errno == EAGAIN || errno == EINTR)
                        // will try later
                        stop = true;
                    else {
                        disconnect_reason = "socket reports error while writing";
                        reason = STREAM_HANDSHAKE_DISCONNECT_SOCKET_WRITE_FAILED;
                    }
                }
                spinlock_unlock(&rpt->thread.send_to_child.spinlock);

                if (disconnect_reason) {
                    worker_is_busy(WORKER_SENDER_JOB_DISCONNECT_SEND_ERROR);
                    nd_log(NDLS_DAEMON, NDLP_ERR,
                           "STREAM RCV[%zu] '%s' [from [%s]:%s]: %s (%zd, on fd %d) - closing connection - "
                           "we have sent %zu bytes in %zu operations.",
                           sth->id, rrdhost_hostname(rpt->host), rpt->client_ip, rpt->client_port,
                           disconnect_reason, rc, rpt->sock.fd, stats->bytes_sent, stats->sends);

                    receiver_set_exit_reason(rpt, reason, false);
                    stream_receiver_remove(sth, rpt, disconnect_reason);
                    return false;
                }
            }
            else
                break;
        }
    }

    if (!(events & ND_POLL_READ))
        return true;

    // we can receive data from this socket

    worker_is_busy(WORKER_STREAM_JOB_SOCKET_RECEIVE);
    bool removed = false, stop = false;
    size_t iterations = 0;
    while(!removed && !stop && iterations++ < MAX_IO_ITERATIONS_PER_EVENT) {
        ssize_t rc = stream_receive_and_process(sth, rpt, parser, &removed);
        if (likely(rc > 0)) {
            rpt->last_msg_t = (time_t)(now_ut / USEC_PER_SEC);

            if(stream_thread_process_opcodes(sth, &rpt->thread.meta))
                stop = true;
        }
        else if (rc == 0 || errno == ECONNRESET) {
            worker_is_busy(WORKER_SENDER_JOB_DISCONNECT_REMOTE_CLOSED);
            nd_log(NDLS_DAEMON, NDLP_ERR,
                   "STREAM RCV[%zu] '%s' [from [%s]:%s]: socket %d reports EOF (closed by child).",
                   sth->id, rrdhost_hostname(rpt->host), rpt->client_ip, rpt->client_port, rpt->sock.fd);
            receiver_set_exit_reason(rpt, STREAM_HANDSHAKE_DISCONNECT_SOCKET_CLOSED_BY_REMOTE_END, false);
            stream_receiver_remove(sth, rpt, "socket reports EOF (closed by child)");
            return false;
        }
        else if (rc < 0) {
            if(removed)
                return false;

            else if ((errno == EWOULDBLOCK || errno == EAGAIN || errno == EINTR))
                // will try later
                stop = true;
            else {
                worker_is_busy(WORKER_SENDER_JOB_DISCONNECT_RECEIVE_ERROR);
                nd_log(NDLS_DAEMON, NDLP_ERR,
                       "STREAM RCV[%zu] '%s' [from [%s]:%s]: error during receive (%zd, on fd %d) - closing connection.",
                       sth->id, rrdhost_hostname(rpt->host), rpt->client_ip, rpt->client_port, rc, rpt->sock.fd);
                receiver_set_exit_reason(rpt, STREAM_HANDSHAKE_DISCONNECT_SOCKET_READ_FAILED, false);
                stream_receiver_remove(sth, rpt, "error during receive");
                return false;
            }
        }
    }

    return !removed;
}

void stream_receiver_cleanup(struct stream_thread *sth) {
    Word_t idx = 0;
    for(struct pollfd_meta *m = META_FIRST(&sth->run.meta, &idx);
         m;
         m = META_NEXT(&sth->run.meta, &idx)) {
        if (m->type != POLLFD_TYPE_RECEIVER) continue;
        struct receiver_state *rpt = m->rpt;
        stream_receiver_remove(sth, rpt, "shutdown");
    }
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
        rrdhost_state_id_increment(host);

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
                       "STREAM RCV '%s' [from [%s]:%s]: "
                       "Postponing health checks for %" PRId64 " seconds, because it was just connected.",
                       rrdhost_hostname(host), rpt->client_ip, rpt->client_port,
                       (int64_t) rpt->config.health.delay);
            }
        }

        host->health_log.health_log_retention_s = rpt->config.health.history;

//         this is a test
//        if(rpt->hops <= host->sender->hops)
//            stream_sender_thread_stop(host, "HOPS MISMATCH", false);

        signal_rrdcontext = true;
        stream_receiver_replication_reset(host);

        rrdhost_flag_set(rpt->host, RRDHOST_FLAG_COLLECTOR_ONLINE);
        aclk_queue_node_info(rpt->host, true);

        rrdhost_stream_parents_reset(host, STREAM_HANDSHAKE_PREPARING);

        set_this = true;
    }

    rrdhost_receiver_unlock(host);

    if(signal_rrdcontext)
        rrdcontext_host_child_connected(host);

    if(set_this)
        ml_host_start(host);

    return set_this;
}

void rrdhost_clear_receiver(struct receiver_state *rpt) {
    RRDHOST *host = rpt->host;
    if(!host) return;

    rrdhost_receiver_lock(host);
    {
        // Make sure that we detach this thread and don't kill a freshly arriving receiver

        if (host->receiver == rpt) {
            rrdhost_state_id_increment(host);
            rrdhost_flag_clear(host, RRDHOST_FLAG_COLLECTOR_ONLINE);
            rrdhost_receiver_unlock(host);
            {
                // run all these without having the receiver lock

                ml_host_stop(host);
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
        netdata_log_error("STREAM RCV[x] '%s' [from [%s]:%s]: "
              "streaming thread takes too long to stop, giving up..."
              , rrdhost_hostname(host)
              , host->receiver->client_ip, host->receiver->client_port);
    else
        ret = true;

    rrdhost_receiver_unlock(host);

    return ret;
}
