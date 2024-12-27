// SPDX-License-Identifier: GPL-3.0-or-later

#include "stream.h"
#include "stream-thread.h"
#include "stream-receiver-internals.h"
#include "web/server/h2o/http_server.h"

#ifdef NETDATA_LOG_STREAM_RECEIVER
void stream_receiver_log_payload(struct receiver_state *rpt, const char *payload, STREAM_TRAFFIC_TYPE type __maybe_unused, bool inbound) {
    if (!rpt) return; // not a streaming parser

    spinlock_lock(&rpt->log.spinlock);

    if (!rpt->log.fp) {
        char filename[FILENAME_MAX + 1];
        snprintfz(
            filename, FILENAME_MAX, "/tmp/stream-receiver-%s.txt", rpt->host ? rrdhost_hostname(rpt->host) : "unknown");

        rpt->log.fp = fopen(filename, "w");

        // Align first_call to wall clock time
        clock_gettime(CLOCK_REALTIME, &rpt->log.first_call);
        rpt->log.first_call.tv_nsec = 0; // Align to the start of the second
    }

    if (rpt->log.fp) {
        struct timespec now;
        clock_gettime(CLOCK_REALTIME, &now);

        time_t elapsed_sec = now.tv_sec - rpt->log.first_call.tv_sec;
        long elapsed_nsec = now.tv_nsec - rpt->log.first_call.tv_nsec;

        if (elapsed_nsec < 0) {
            elapsed_sec--;
            elapsed_nsec += 1000000000;
        }

        uint16_t days = elapsed_sec / 86400;
        uint8_t hours = (elapsed_sec % 86400) / 3600;
        uint8_t minutes = (elapsed_sec % 3600) / 60;
        uint8_t seconds = elapsed_sec % 60;
        uint16_t milliseconds = elapsed_nsec / 1000000;

        char prefix[30];
        snprintf(prefix, sizeof(prefix), "%03ud.%02u:%02u:%02u.%03u ",
                 days, hours, minutes, seconds, milliseconds);

        const char *line_start = payload;
        const char *line_end;

        while (line_start && *line_start) {
            line_end = strchr(line_start, '\n');
            if (line_end) {
                fprintf(rpt->log.fp, "%s%s%.*s\n", prefix, inbound ? "> " : "< ", (int)(line_end - line_start), line_start);
                line_start = line_end + 1;
            } else {
                fprintf(rpt->log.fp, "%s%s%s\n", prefix, inbound ? "> " : "< ", line_start);
                break;
            }
        }
    }

    // fflush(rpt->log.fp);
    spinlock_unlock(&rpt->log.spinlock);
}
#endif

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
               rrdhost_hostname(r->host), r->remote_ip, r->remote_port);
        return DECOMPRESS_FAILED;
    }

    if(unlikely(compressed_message_size > COMPRESSION_MAX_MSG_SIZE)) {
        nd_log(NDLS_DAEMON, NDLP_ERR,
               "STREAM RCV[x] '%s' [from [%s]:%s]: received a compressed message of %zu bytes, "
               "which is bigger than the max compressed message "
               "size supported of %zu. Ignoring message.",
               rrdhost_hostname(r->host), r->remote_ip, r->remote_port,
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
               rrdhost_hostname(r->host), r->remote_ip, r->remote_port);
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
        ND_LOG_FIELD_TXT(NDF_SRC_IP, rpt->remote_ip),
        ND_LOG_FIELD_TXT(NDF_SRC_PORT, rpt->remote_port),
        ND_LOG_FIELD_CB(NDF_SRC_TRANSPORT, stream_receiver_log_transport, rpt),
        ND_LOG_FIELD_CB(NDF_SRC_CAPABILITIES, stream_receiver_log_capabilities, rpt),
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
               sth->id, rrdhost_hostname(rpt->host), rpt->remote_ip, rpt->remote_port,
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
    else {
        stream_receiver_log_payload(rpt, txt, type, false);

        if(was_empty)
            msg.opcode = STREAM_OPCODE_RECEIVER_POLLOUT;
    }

    spinlock_unlock(&rpt->thread.send_to_child.spinlock);

    if(msg.opcode != STREAM_OPCODE_NONE)
        stream_receiver_send_opcode(rpt, msg);

    return rc;
}

// --------------------------------------------------------------------------------------------------------------------

static void stream_receive_log_database_gap(struct receiver_state *rpt) {
    RRDHOST *host = rpt->host;

    time_t now = now_realtime_sec();
    time_t last_db_entry = 0;
    rrdhost_retention(host, now, false, NULL, &last_db_entry);

    if(now < last_db_entry)
        last_db_entry = now;

    if(!last_db_entry) {
        nd_log(NDLS_DAEMON, NDLP_NOTICE,
               "STREAM RCV '%s' [from [%s]:%s]: node connected; for the first time!",
               rrdhost_hostname(host), rpt->remote_ip, rpt->remote_port);
    }
    else {
        char buf[128];
        duration_snprintf(buf, sizeof(buf), now - last_db_entry, "s", true);
        nd_log(NDLS_DAEMON, NDLP_NOTICE,
               "STREAM RCV '%s' [from [%s]:%s]: node connected; last sample in the database %s ago",
               rrdhost_hostname(host), rpt->remote_ip, rpt->remote_port, buf);
    }
}

void stream_receiver_move_to_running_unsafe(struct stream_thread *sth, struct receiver_state *rpt) {
    internal_fatal(sth->tid != gettid_cached(), "Function %s() should only be used by the dispatcher thread", __FUNCTION__ );

    worker_is_busy(WORKER_STREAM_JOB_DEQUEUE);

    ND_LOG_STACK lgs[] = {
        ND_LOG_FIELD_STR(NDF_NIDL_NODE, rpt->host->hostname),
        ND_LOG_FIELD_UUID(NDF_MESSAGE_ID, &streaming_from_child_msgid),
        ND_LOG_FIELD_END(),
    };
    ND_LOG_STACK_PUSH(lgs);

    nd_log(NDLS_DAEMON, NDLP_DEBUG,
           "STREAM RCV[%zu] '%s' [from [%s]:%s]: moving host from receiver queue to receiver running...",
           sth->id, rrdhost_hostname(rpt->host), rpt->remote_ip, rpt->remote_port);

    sock_setcloexec(rpt->sock.fd, true);
    sock_enlarge_rcv_buf(rpt->sock.fd);
    sock_enlarge_snd_buf(rpt->sock.fd);
    sock_setcork(rpt->sock.fd, false);
    if(sock_setnonblock(rpt->sock.fd, true) != 1)
        nd_log(NDLS_DAEMON, NDLP_ERR,
               "STREAM RCV '%s' [from [%s]:%s]: failed to set non-blocking mode on socket %d",
               rrdhost_hostname(rpt->host), rpt->remote_ip, rpt->remote_port, rpt->sock.fd);

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

    if(!nd_poll_add(sth->run.ndpl, rpt->sock.fd, ND_POLL_READ, &rpt->thread.meta))
        nd_log(NDLS_DAEMON, NDLP_ERR,
               "STREAM RCV[%zu] '%s' [from [%s]:%s]:"
               "Failed to add receiver socket to nd_poll()",
               sth->id, rrdhost_hostname(rpt->host), rpt->remote_ip, rpt->remote_port);

    // put the client IP and port into the buffers used by plugins.d
    {
        char buf[CONFIG_MAX_NAME];
        snprintfz(buf, sizeof(buf),  "[%s]:%s", rpt->remote_ip, rpt->remote_port);
        string_freez(rpt->thread.cd.id);
        rpt->thread.cd.id = string_strdupz(buf);

        string_freez(rpt->thread.cd.filename);
        rpt->thread.cd.filename = string_strdupz(buf);

        string_freez(rpt->thread.cd.fullfilename);
        rpt->thread.cd.fullfilename = string_strdupz(buf);

        string_freez(rpt->thread.cd.cmd);
        rpt->thread.cd.cmd = string_strdupz(buf);
    }

    rpt->thread.compressed.start = 0;
    rpt->thread.compressed.used = 0;
    rpt->thread.compressed.enabled = stream_decompression_initialize(rpt);
    buffered_reader_init(&rpt->reader);

    rpt->thread.buffer = buffer_create(sizeof(rpt->reader.read_buffer), NULL);

    // help preferred_sender_buffer() select the right buffer
    rpt->host->stream.snd.commit.receiver_tid = gettid_cached();

    PARSER *parser = NULL;
    {
        rpt->thread.cd = (struct plugind){
            .update_every = nd_profile.update_every,
            .unsafe = {
                .spinlock = SPINLOCK_INITIALIZER,
                .running = true,
                .enabled = true,
            },
            .started_t = now_realtime_sec(),
        };

        PARSER_USER_OBJECT user = {
            .enabled = plugin_is_enabled(&rpt->thread.cd),
            .host = rpt->host,
            .opaque = rpt,
            .cd = &rpt->thread.cd,
            .trust_durations = 1,
            .capabilities = rpt->capabilities,
#ifdef NETDATA_LOG_STREAM_RECEIVER
            .rpt = rpt,
#endif
        };

        parser = parser_init(&user, -1, -1, PARSER_INPUT_SPLIT, &rpt->sock);
        parser->send_to_plugin_data = rpt;
        parser->send_to_plugin_cb = send_to_child;

        pluginsd_keywords_init(parser, PARSER_INIT_STREAMING);

        __atomic_store_n(&rpt->thread.parser, parser, __ATOMIC_RELAXED);
    }

#ifdef ENABLE_H2O
    parser->h2o_ctx = rpt->h2o_ctx;
#endif

    stream_receive_log_database_gap(rpt);
    rrdhost_state_connected(rpt->host);

    // keep this last - it needs everything ready since to sends data to the child
    stream_receiver_send_node_and_claim_id_to_child(rpt->host);
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

    ND_LOG_STACK lgs[] = {
        ND_LOG_FIELD_STR(NDF_NIDL_NODE, rpt->host->hostname),
        ND_LOG_FIELD_TXT(NDF_SRC_IP, rpt->remote_ip),
        ND_LOG_FIELD_TXT(NDF_SRC_PORT, rpt->remote_port),
        ND_LOG_FIELD_CB(NDF_SRC_TRANSPORT, stream_receiver_log_transport, rpt),
        ND_LOG_FIELD_CB(NDF_SRC_CAPABILITIES, stream_receiver_log_capabilities, rpt),
        ND_LOG_FIELD_UUID(NDF_MESSAGE_ID, &streaming_from_child_msgid),
        ND_LOG_FIELD_END(),
    };
    ND_LOG_STACK_PUSH(lgs);

    PARSER *parser = __atomic_load_n(&rpt->thread.parser, __ATOMIC_RELAXED);
    size_t count = parser ? parser->user.data_collections_count : 0;

    errno_clear();
    nd_log(NDLS_DAEMON, NDLP_ERR,
           "STREAM RCV[%zu] '%s' [from [%s]:%s]: "
           "receiver disconnected (after %zu received messages): %s"
           , sth->id
           , rpt->hostname ? rpt->hostname : "-"
           , rpt->remote_ip ? rpt->remote_ip : "-"
           , rpt->remote_port ? rpt->remote_port : "-"
           , count
           , why ? why : "");

    rrdhost_state_disconnected(rpt->host);

    internal_fatal(META_GET(&sth->run.meta, (Word_t)&rpt->thread.meta) == NULL, "Receiver to be removed is not found in the list of receivers");
    META_DEL(&sth->run.meta, (Word_t)&rpt->thread.meta);

    if(!nd_poll_del(sth->run.ndpl, rpt->sock.fd, &rpt->thread.meta))
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

    if(parser) {
        parser->user.v2.stream_buffer.wb = NULL;

        // make sure send_to_plugin() will not write any data to the socket
        spinlock_lock(&parser->writer.spinlock);
        parser->fd_input = -1;
        parser->fd_output = -1;
        parser->sock = NULL;
        spinlock_unlock(&parser->writer.spinlock);
    }

    // the parser stopped
    receiver_set_exit_reason(rpt, STREAM_HANDSHAKE_DISCONNECT_PARSER_EXIT, false);

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
    *removed = false;

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
        if(rc <= 0)
            return rc;

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

bool stream_receiver_send_data(struct stream_thread *sth, struct receiver_state *rpt, usec_t now_ut, bool process_opcodes) {
    internal_fatal(sth->tid != gettid_cached(), "Function %s() should only be used by the dispatcher thread", __FUNCTION__ );

    EVLOOP_STATUS status = EVLOOP_STATUS_CONTINUE;
    while(status == EVLOOP_STATUS_CONTINUE) {
        if (!spinlock_trylock(&rpt->thread.send_to_child.spinlock)) {
            status = EVLOOP_STATUS_CANT_GET_LOCK;
            break;
        }

        char *chunk;
        STREAM_CIRCULAR_BUFFER *scb = rpt->thread.send_to_child.scb;
        STREAM_CIRCULAR_BUFFER_STATS *stats = stream_circular_buffer_stats_unsafe(scb);
        size_t outstanding = stream_circular_buffer_get_unsafe(scb, &chunk);

        if(!outstanding) {
            status = EVLOOP_STATUS_NO_MORE_DATA;
            spinlock_unlock(&rpt->thread.send_to_child.spinlock);
            continue;
        }

        ssize_t rc = write_stream(rpt, chunk, outstanding);
        if (likely(rc > 0)) {
            rpt->thread.last_traffic_ut = now_ut;
            stream_circular_buffer_del_unsafe(scb, rc, now_ut);
            if (!stats->bytes_outstanding) {
                if (!nd_poll_upd(sth->run.ndpl, rpt->sock.fd, ND_POLL_READ, &rpt->thread.meta))
                    nd_log(NDLS_DAEMON, NDLP_ERR,
                           "STREAM RCV[%zu] '%s' [from [%s]:%s]: cannot update nd_poll()",
                           sth->id, rrdhost_hostname(rpt->host), rpt->remote_ip, rpt->remote_port);

                // recreate the circular buffer if we have to
                stream_circular_buffer_recreate_timed_unsafe(rpt->thread.send_to_child.scb, now_ut, false);
                status = EVLOOP_STATUS_NO_MORE_DATA;
            }
        }
        else if (rc == 0 || errno == ECONNRESET)
            status = EVLOOP_STATUS_SOCKET_CLOSED;

        else if (rc < 0) {
            if (errno == EWOULDBLOCK || errno == EAGAIN || errno == EINTR)
                status = EVLOOP_STATUS_SOCKET_FULL;
            else
                status = EVLOOP_STATUS_SOCKET_ERROR;
        }

        spinlock_unlock(&rpt->thread.send_to_child.spinlock);

        if (status == EVLOOP_STATUS_SOCKET_ERROR || status == EVLOOP_STATUS_SOCKET_CLOSED) {
            const char *disconnect_reason;
            STREAM_HANDSHAKE reason;

            if(status == EVLOOP_STATUS_SOCKET_ERROR) {
                worker_is_busy(WORKER_STREAM_JOB_DISCONNECT_SEND_ERROR);
                disconnect_reason = "socket reports error while writing";
                reason = STREAM_HANDSHAKE_DISCONNECT_SOCKET_WRITE_FAILED;
            }
            else /* if(status == EVLOOP_STATUS_SOCKET_CLOSED) */ {
                worker_is_busy(WORKER_STREAM_JOB_DISCONNECT_REMOTE_CLOSED);
                disconnect_reason = "socket reports EOF (closed by child)";
                reason = STREAM_HANDSHAKE_DISCONNECT_SOCKET_CLOSED_BY_REMOTE_END;
            }

            nd_log(NDLS_DAEMON, NDLP_ERR,
                   "STREAM RCV[%zu] '%s' [from [%s]:%s]: %s (%zd, on fd %d) - closing receiver connection - "
                   "we have sent %zu bytes in %zu operations.",
                   sth->id, rrdhost_hostname(rpt->host), rpt->remote_ip, rpt->remote_port,
                   disconnect_reason, rc, rpt->sock.fd, stats->bytes_sent, stats->sends);

            receiver_set_exit_reason(rpt, reason, false);
            stream_receiver_remove(sth, rpt, disconnect_reason);
        }
        else if(process_opcodes && status == EVLOOP_STATUS_CONTINUE && stream_thread_process_opcodes(sth, &rpt->thread.meta))
            status = EVLOOP_STATUS_OPCODE_ON_ME;
    }

    return EVLOOP_STATUS_STILL_ALIVE(status);
}

bool stream_receiver_receive_data(struct stream_thread *sth, struct receiver_state *rpt, usec_t now_ut, bool process_opcodes) {
    internal_fatal(sth->tid != gettid_cached(), "Function %s() should only be used by the dispatcher thread", __FUNCTION__ );

    PARSER *parser = __atomic_load_n(&rpt->thread.parser, __ATOMIC_RELAXED);
    ND_LOG_STACK lgs[] = {
        ND_LOG_FIELD_CB(NDF_REQUEST, line_splitter_reconstruct_line, &parser->line),
        ND_LOG_FIELD_CB(NDF_NIDL_NODE, parser_reconstruct_node, parser),
        ND_LOG_FIELD_CB(NDF_NIDL_INSTANCE, parser_reconstruct_instance, parser),
        ND_LOG_FIELD_CB(NDF_NIDL_CONTEXT, parser_reconstruct_context, parser),
        ND_LOG_FIELD_END(),
    };
    ND_LOG_STACK_PUSH(lgs);

    EVLOOP_STATUS status = EVLOOP_STATUS_CONTINUE;
    while(status == EVLOOP_STATUS_CONTINUE) {
        bool removed = false;
        ssize_t rc = stream_receive_and_process(sth, rpt, parser, &removed);
        if(unlikely(removed))
            status = EVLOOP_STATUS_PARSER_FAILED;

        else if (likely(rc > 0))
            rpt->thread.last_traffic_ut = now_ut;

        else if (rc == 0 || errno == ECONNRESET)
            status = EVLOOP_STATUS_SOCKET_CLOSED;

        else if (rc < 0) {
            if ((errno == EWOULDBLOCK || errno == EAGAIN || errno == EINTR))
                status = EVLOOP_STATUS_SOCKET_FULL;
            else
                status = EVLOOP_STATUS_SOCKET_ERROR;
        }

        if(status == EVLOOP_STATUS_SOCKET_ERROR || status == EVLOOP_STATUS_SOCKET_CLOSED) {
            const char *disconnect_reason;
            STREAM_HANDSHAKE reason;

            if(status == EVLOOP_STATUS_SOCKET_ERROR) {
                worker_is_busy(WORKER_STREAM_JOB_DISCONNECT_RECEIVE_ERROR);
                reason = STREAM_HANDSHAKE_DISCONNECT_SOCKET_READ_FAILED;
                disconnect_reason = "error during receive";
            }
            else /* if(status == EVLOOP_STATUS_SOCKET_CLOSED) */ {
                worker_is_busy(WORKER_STREAM_JOB_DISCONNECT_REMOTE_CLOSED);
                reason = STREAM_HANDSHAKE_DISCONNECT_SOCKET_CLOSED_BY_REMOTE_END;
                disconnect_reason = "socket reports EOF (closed by child)";
            }

            nd_log(NDLS_DAEMON, NDLP_ERR,
                   "STREAM RCV[%zu] '%s' [from [%s]:%s]: %s (fd %d) - closing receiver connection.",
                   sth->id, rrdhost_hostname(rpt->host), rpt->remote_ip, rpt->remote_port, disconnect_reason, rpt->sock.fd);

            receiver_set_exit_reason(rpt, reason, false);
            stream_receiver_remove(sth, rpt, disconnect_reason);
        }
        else if(status == EVLOOP_STATUS_CONTINUE && process_opcodes && stream_thread_process_opcodes(sth, &rpt->thread.meta))
            status = EVLOOP_STATUS_OPCODE_ON_ME;
    }

    return EVLOOP_STATUS_STILL_ALIVE(status);
}

// process poll() events for streaming receivers
// returns true when the receiver is still there, false if it removed it
bool stream_receive_process_poll_events(struct stream_thread *sth, struct receiver_state *rpt, nd_poll_event_t events, usec_t now_ut) {
    internal_fatal(sth->tid != gettid_cached(), "Function %s() should only be used by the dispatcher thread", __FUNCTION__);

    ND_LOG_STACK lgs[] = {
        ND_LOG_FIELD_TXT(NDF_SRC_IP, rpt->remote_ip),
        ND_LOG_FIELD_TXT(NDF_SRC_PORT, rpt->remote_port),
        ND_LOG_FIELD_TXT(NDF_NIDL_NODE, rpt->hostname),
        ND_LOG_FIELD_CB(NDF_SRC_TRANSPORT, stream_receiver_log_transport, rpt),
        ND_LOG_FIELD_CB(NDF_SRC_CAPABILITIES, stream_receiver_log_capabilities, rpt),
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
               sth->id, rrdhost_hostname(rpt->host), rpt->remote_ip, rpt->remote_port, error);

        receiver_set_exit_reason(rpt, STREAM_HANDSHAKE_DISCONNECT_SOCKET_ERROR, false);
        stream_receiver_remove(sth, rpt, error);
        return false;
    }

    if (events & ND_POLL_WRITE) {
        worker_is_busy(WORKER_STREAM_JOB_SOCKET_SEND);
        if(!stream_receiver_send_data(sth, rpt, now_ut, true))
            return false;
    }

    if (events & ND_POLL_READ) {
        worker_is_busy(WORKER_STREAM_JOB_SOCKET_RECEIVE);
        if(!stream_receiver_receive_data(sth, rpt, now_ut, true))
            return false;
    }

    return true;
}

void stream_receiver_check_all_nodes_from_poll(struct stream_thread *sth, usec_t now_ut) {
    internal_fatal(sth->tid != gettid_cached(), "Function %s() should only be used by the dispatcher thread", __FUNCTION__ );

    NETDATA_DOUBLE overall_buffer_ratio = 0.0;

    Word_t idx = 0;
    for(struct pollfd_meta *m = META_FIRST(&sth->run.meta, &idx);
         m;
         m = META_NEXT(&sth->run.meta, &idx)) {
        if (m->type != POLLFD_TYPE_RECEIVER) continue;
        struct receiver_state *rpt = m->rpt;

        spinlock_lock(&rpt->thread.send_to_child.spinlock);
        STREAM_CIRCULAR_BUFFER_STATS stats = *stream_circular_buffer_stats_unsafe(rpt->thread.send_to_child.scb);
        spinlock_lock(&rpt->thread.send_to_child.spinlock);

        if (stats.buffer_ratio > overall_buffer_ratio)
            overall_buffer_ratio = stats.buffer_ratio;

        time_t timeout_s = 600;
        if(unlikely(rpt->thread.last_traffic_ut + timeout_s * USEC_PER_SEC < now_ut &&
                     !rrdhost_receiver_replicating_charts(rpt->host))) {

            ND_LOG_STACK lgs[] = {
                ND_LOG_FIELD_TXT(NDF_SRC_IP, rpt->remote_ip),
                ND_LOG_FIELD_TXT(NDF_SRC_PORT, rpt->remote_port),
                ND_LOG_FIELD_TXT(NDF_NIDL_NODE, rpt->hostname),
                ND_LOG_FIELD_CB(NDF_SRC_TRANSPORT, stream_receiver_log_transport, rpt),
                ND_LOG_FIELD_CB(NDF_SRC_CAPABILITIES, stream_receiver_log_capabilities, rpt),
                ND_LOG_FIELD_END(),
            };
            ND_LOG_STACK_PUSH(lgs);

            worker_is_busy(WORKER_SENDER_JOB_DISCONNECT_TIMEOUT);

            char duration[RFC3339_MAX_LENGTH];
            duration_snprintf(duration, sizeof(duration), (int64_t)(now_monotonic_usec() - rpt->thread.last_traffic_ut), "us", true);

            char pending[64] = "0";
            if(stats.bytes_outstanding)
                size_snprintf(pending, sizeof(pending), stats.bytes_outstanding, "B", false);

            nd_log(NDLS_DAEMON, NDLP_ERR,
                   "STREAM RCV[%zu] '%s' [from %s]: there was not traffic for %ld seconds - closing connection - "
                   "we have sent %zu bytes in %zu operations, it is idle for %s, and we have %s pending to send "
                   "(buffer is used %.2f%%).",
                   sth->id, rrdhost_hostname(rpt->host), rpt->remote_ip, timeout_s,
                   stats.bytes_sent, stats.sends, duration, pending, stats.buffer_ratio);

            receiver_set_exit_reason(rpt, STREAM_HANDSHAKE_DISCONNECT_SOCKET_TIMEOUT, false);
            stream_receiver_remove(sth, rpt, "timeout");
            continue;
        }

        if(!nd_poll_upd(sth->run.ndpl, rpt->sock.fd, ND_POLL_READ | (stats.bytes_outstanding ? ND_POLL_WRITE : 0), &rpt->thread.meta))
            nd_log(NDLS_DAEMON, NDLP_ERR,
                   "STREAM RCV[%zu] '%s' [from %s]: failed to update nd_poll().",
                   sth->id, rrdhost_hostname(rpt->host), rpt->remote_ip);

    }
}

void stream_receiver_cleanup(struct stream_thread *sth) {
    Word_t idx = 0;
    for(struct pollfd_meta *m = META_FIRST(&sth->run.meta, &idx);
         m;
         m = META_NEXT(&sth->run.meta, &idx)) {
        if (m->type != POLLFD_TYPE_RECEIVER) continue;
        struct receiver_state *rpt = m->rpt;
        receiver_set_exit_reason(rpt, STREAM_HANDSHAKE_DISCONNECT_SHUTDOWN, false);
        stream_receiver_remove(sth, rpt, "shutdown");
    }
}

static void stream_receiver_replication_reset(RRDHOST *host) {
    RRDSET *st;
    rrdset_foreach_read(st, host) {
        RRDSET_FLAGS old = rrdset_flag_set_and_clear(st, RRDSET_FLAG_RECEIVER_REPLICATION_FINISHED, RRDSET_FLAG_RECEIVER_REPLICATION_IN_PROGRESS);
        if(!(old & RRDSET_FLAG_RECEIVER_REPLICATION_FINISHED))
            rrdhost_receiver_replicating_charts_minus_one(host);

#ifdef REPLICATION_TRACKING
        st->stream.rcv.who = REPLAY_WHO_UNKNOWN;
#endif
    }
    rrdset_foreach_done(st);

    if(rrdhost_receiver_replicating_charts(host) != 0) {
        nd_log(NDLS_DAEMON, NDLP_WARNING,
               "STREAM REPLAY ERROR: receiver replication instances counter should be zero, but it is %zu"
               " - resetting it to zero",
               rrdhost_receiver_replicating_charts(host));

        rrdhost_receiver_replicating_charts_zero(host);
    }
}

bool rrdhost_set_receiver(RRDHOST *host, struct receiver_state *rpt) {
    bool signal_rrdcontext = false;
    bool set_this = false;

    rrdhost_receiver_lock(host);

    if (!host->receiver) {
        rrdhost_flag_clear(host, RRDHOST_FLAG_ORPHAN);
        rrdhost_set_health_evloop_iteration(host);

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
                       rrdhost_hostname(host), rpt->remote_ip, rpt->remote_port,
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
            rrdhost_flag_clear(host, RRDHOST_FLAG_COLLECTOR_ONLINE);

            rrdhost_receiver_unlock(host);
            {
                // run all these without having the receiver lock

                rrdhost_set_health_evloop_iteration(host);
                ml_host_stop(host);
                stream_path_child_disconnected(host);
                stream_sender_signal_to_stop_and_wait(host, STREAM_HANDSHAKE_DISCONNECT_RECEIVER_LEFT, false);
                rrdcontext_host_child_disconnected(host);

                if (rpt->config.health.enabled)
                    rrdcalc_child_disconnected(host);

                rrdhost_stream_parents_reset(host, STREAM_HANDSHAKE_DISCONNECT_RECEIVER_LEFT);
            }
            rrdhost_receiver_lock(host);

            // now we have the lock again

            stream_receiver_replication_reset(host);
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
              , host->receiver->remote_ip, host->receiver->remote_port);
    else
        ret = true;

    rrdhost_receiver_unlock(host);

    return ret;
}
