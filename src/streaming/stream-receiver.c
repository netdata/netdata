// SPDX-License-Identifier: GPL-3.0-or-later

#include "stream.h"
#include "stream-thread.h"
#include "stream-receiver-internals.h"

#ifdef NETDATA_LOG_STREAM_RECEIVER
void stream_receiver_log_payload(struct receiver_state *rpt, const char *payload, STREAM_TRAFFIC_TYPE type __maybe_unused, bool inbound) {
    if (!rpt || type != STREAM_TRAFFIC_TYPE_REPLICATION) return; // not a streaming parser

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

    fflush(rpt->log.fp);
    spinlock_unlock(&rpt->log.spinlock);
}
#endif

// help the IDE identify use after free
#define stream_receiver_remove(sth, rpt, reason) do {           \
        stream_receiver_remove_internal(sth, rpt, reason);      \
        (rpt) = NULL;                                           \
} while(0)

static void stream_receiver_remove_internal(struct stream_thread *sth, struct receiver_state *rpt, STREAM_HANDSHAKE reason);

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

ALWAYS_INLINE
static ssize_t write_stream(struct receiver_state *r, char* buffer, size_t size) {
    if(unlikely(!size)) {
        internal_error(true, "%s() asked to read zero bytes", __FUNCTION__);
        errno_clear();
        return -2;
    }

    ssize_t bytes_written = nd_sock_send_nowait(&r->sock, buffer, size);
    return bytes_written;
}

ALWAYS_INLINE
static ssize_t read_stream(struct receiver_state *r, char* buffer, size_t size) {
    if(unlikely(!size)) {
        internal_error(true, "%s() asked to read zero bytes", __FUNCTION__);
        errno_clear();
        return -2;
    }

    ssize_t bytes_read = nd_sock_revc_nowait(&r->sock, buffer, size);
    return bytes_read;
}

// --------------------------------------------------------------------------------------------------------------------

ALWAYS_INLINE
static ssize_t receiver_read_uncompressed(struct receiver_state *r) {
    internal_fatal(r->thread.uncompressed.read_buffer[r->thread.uncompressed.read_len] != '\0',
                   "%s: read_buffer does not start with zero #2", __FUNCTION__ );

    ssize_t bytes = read_stream(r, r->thread.uncompressed.read_buffer + r->thread.uncompressed.read_len, sizeof(r->thread.uncompressed.read_buffer) - r->thread.uncompressed.read_len - 1);
    if(bytes > 0) {
        worker_set_metric(WORKER_RECEIVER_JOB_BYTES_READ, (NETDATA_DOUBLE)bytes);
        worker_set_metric(WORKER_RECEIVER_JOB_BYTES_UNCOMPRESSED, (NETDATA_DOUBLE)bytes);

        r->thread.uncompressed.read_len += bytes;
        r->thread.uncompressed.read_buffer[r->thread.uncompressed.read_len] = '\0';
        pulse_stream_received_bytes(bytes);
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

ALWAYS_INLINE_HOT_FLATTEN
static decompressor_status_t receiver_feed_decompressor(struct receiver_state *r) {
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

ALWAYS_INLINE_HOT_FLATTEN
static decompressor_status_t receiver_get_decompressed(struct receiver_state *r) {
    if (unlikely(!stream_decompressed_bytes_in_buffer(&r->thread.compressed.decompressor)))
        return DECOMPRESS_NEED_MORE_DATA;

    size_t available = sizeof(r->thread.uncompressed.read_buffer) - r->thread.uncompressed.read_len - 1;
    if (likely(available)) {
        size_t len = stream_decompressor_get(
            &r->thread.compressed.decompressor, r->thread.uncompressed.read_buffer + r->thread.uncompressed.read_len, available);
        if (unlikely(!len)) {
            internal_error(true, "decompressor returned zero length #1");
            return DECOMPRESS_FAILED;
        }

        r->thread.uncompressed.read_len += (int)len;
        r->thread.uncompressed.read_buffer[r->thread.uncompressed.read_len] = '\0';
    }
    else {
        internal_fatal(true, "The line to read is too big! Already have %zd bytes in read_buffer.", r->thread.uncompressed.read_len);
        return DECOMPRESS_FAILED;
    }

    return DECOMPRESS_OK;
}

ALWAYS_INLINE_HOT_FLATTEN
static ssize_t receiver_read_compressed(struct receiver_state *r) {

    internal_fatal(r->thread.uncompressed.read_buffer[r->thread.uncompressed.read_len] != '\0',
                   "%s: read_buffer does not start with zero #2", __FUNCTION__ );

    ssize_t bytes = read_stream(r, r->thread.compressed.buf + r->thread.compressed.used,
                                 r->thread.compressed.size - r->thread.compressed.used);

    if(bytes > 0) {
        r->thread.compressed.used += bytes;
        worker_set_metric(WORKER_RECEIVER_JOB_BYTES_READ, (NETDATA_DOUBLE)bytes);
        pulse_stream_received_bytes(bytes);
    }

    return bytes;
}

// --------------------------------------------------------------------------------------------------------------------

static STREAM_HANDSHAKE receiver_set_exit_reason(struct receiver_state *rpt, STREAM_HANDSHAKE reason, bool force) {
    if(force || !rpt->exit.reason)
        rpt->exit.reason = reason;

    return rpt->exit.reason;
}

ALWAYS_INLINE
static bool receiver_should_stop(struct receiver_state *rpt) {
    if(unlikely(__atomic_load_n(&rpt->exit.shutdown, __ATOMIC_ACQUIRE))) {
        receiver_set_exit_reason(rpt, STREAM_HANDSHAKE_DISCONNECT_SIGNALED_TO_STOP, false);
        return true;
    }

    return false;
}

// --------------------------------------------------------------------------------------------------------------------

ALWAYS_INLINE
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

        stream_receiver_remove(sth, rpt, STREAM_HANDSHAKE_DISCONNECT_BUFFER_OVERFLOW);
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
    msg.reason = 0;

    size_t size = strlen(txt);
    ssize_t rc = (ssize_t)size;
    if(!stream_circular_buffer_add_unsafe(scb, txt, size, size, type, true)) {
        // should never happen, because of autoscaling
        msg.opcode = STREAM_OPCODE_RECEIVER_BUFFER_OVERFLOW;
        msg.reason = STREAM_HANDSHAKE_DISCONNECT_BUFFER_OVERFLOW;
        rc = -1;
    }
    else {
        stream_receiver_log_payload(rpt, txt, type, false);

        if(was_empty) {
            msg.opcode = STREAM_OPCODE_RECEIVER_POLLOUT;
            msg.reason = 0;
        }
    }

    spinlock_unlock(&rpt->thread.send_to_child.spinlock);

    if(msg.opcode != STREAM_OPCODE_NONE)
        stream_receiver_send_opcode(rpt, msg);

    return rc;
}

// --------------------------------------------------------------------------------------------------------------------

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

    __atomic_store_n(&rpt->host->stream.rcv.status.tid, gettid_cached(), __ATOMIC_RELAXED);
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

    rpt->thread.wanted = ND_POLL_READ;
    if(!nd_poll_add(sth->run.ndpl, rpt->sock.fd, rpt->thread.wanted, &rpt->thread.meta))
        nd_log(NDLS_DAEMON, NDLP_ERR,
               "STREAM RCV[%zu] '%s' [from [%s]:%s]:"
               "Failed to add receiver socket to nd_poll()",
               sth->id, rrdhost_hostname(rpt->host), rpt->remote_ip, rpt->remote_port);

    rpt->thread.compressed.start = 0;
    rpt->thread.compressed.used = 0;
    rpt->thread.compressed.enabled = stream_decompression_initialize(rpt);
    buffered_reader_init(&rpt->thread.uncompressed);

    rpt->thread.line_buffer = buffer_create(sizeof(rpt->thread.uncompressed.read_buffer), NULL);

    // help preferred_sender_buffer() select the right buffer
    rpt->host->stream.snd.commit.receiver_tid = gettid_cached();

    rpt->replication.last_progress_ut = now_monotonic_usec();

    PARSER *parser = NULL;
    {
        char buf[CONFIG_MAX_NAME];
        snprintfz(buf, sizeof(buf),  "[%s]:%s", rpt->remote_ip, rpt->remote_port);
        string_freez(rpt->thread.cd.id);
        rpt->thread.cd.id = string_strdupz(buf);

        string_freez(rpt->thread.cd.filename);
        rpt->thread.cd.filename = NULL;

        string_freez(rpt->thread.cd.fullfilename);
        rpt->thread.cd.fullfilename = NULL;

        string_freez(rpt->thread.cd.cmd);
        rpt->thread.cd.cmd = NULL;

        rpt->thread.cd.update_every = (int)nd_profile.update_every;
        spinlock_init(&rpt->thread.cd.unsafe.spinlock);
        rpt->thread.cd.unsafe.running = true;
        rpt->thread.cd.unsafe.enabled = true;
        rpt->thread.cd.started_t = now_realtime_sec();

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

    if(stream_receive.replication.enabled)
        pulse_host_status(rpt->host, PULSE_HOST_STATUS_RCV_REPLICATION_WAIT, 0);
    else
        pulse_host_status(rpt->host, PULSE_HOST_STATUS_RCV_RUNNING, 0);

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

static void stream_receiver_remove_internal(struct stream_thread *sth, struct receiver_state *rpt, STREAM_HANDSHAKE reason) {
    internal_fatal(sth->tid != gettid_cached(), "Function %s() should only be used by the dispatcher thread", __FUNCTION__ );

    receiver_set_exit_reason(rpt, reason, false);

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
    size_t count = 0;
    if(parser)
        count = parser->user.data_collections_count;

    errno_clear();
    nd_log(NDLS_DAEMON, NDLP_ERR,
           "STREAM RCV[%zu] '%s' [from [%s]:%s]: "
           "receiver disconnected (after %zu received messages): %s"
           , sth->id
           , rpt->hostname ? rpt->hostname : "-"
           , rpt->remote_ip ? rpt->remote_ip : "-"
           , rpt->remote_port ? rpt->remote_port : "-"
           , count
           , stream_handshake_error_to_string(reason));

    internal_fatal(META_GET(&sth->run.meta, (Word_t)&rpt->thread.meta) == NULL,
                   "Receiver to be removed is not found in the list of receivers");

    META_DEL(&sth->run.meta, (Word_t)&rpt->thread.meta);

    rpt->thread.wanted = 0;
    if(!nd_poll_del(sth->run.ndpl, rpt->sock.fd))
        nd_log(NDLS_DAEMON, NDLP_ERR, "Failed to delete receiver socket from nd_poll()");

    __atomic_store_n(&rpt->host->stream.rcv.status.tid, 0, __ATOMIC_RELAXED);

    // make sure send_to_plugin() will not write any data to the socket (or wait for it to finish)
    if(parser) {
        spinlock_lock(&parser->writer.spinlock);
        parser->fd_input = -1;
        parser->fd_output = -1;
        parser->sock = NULL;
        spinlock_unlock(&parser->writer.spinlock);

        parser->user.v2.stream_buffer.wb = NULL;
    }

    stream_thread_node_removed(rpt->host);
    pulse_host_status(rpt->host, PULSE_HOST_STATUS_RCV_OFFLINE, reason);

    // set a default exit reason, if not set
    receiver_set_exit_reason(rpt, reason, false);

    // in case we are connected to netdata cloud,
    // we inform cloud that a child got disconnected
    uint64_t total_reboot = rrdhost_stream_path_total_reboot_time_ms(rpt->host);
    schedule_node_state_update(rpt->host, MIN((total_reboot * MAX_CHILD_DISC_TOLERANCE), MAX_CHILD_DISC_DELAY));

    rrdhost_clear_receiver(rpt, reason);
    rrdhost_set_is_parent_label();

    stream_receiver_free(rpt);
    // DO NOT USE rpt after this point
}

static bool stream_receiver_dequeue_senders(struct stream_thread *sth, struct receiver_state *rpt, usec_t now_ut) {
    internal_fatal(sth->tid != gettid_cached(), "Function %s() should only be used by the dispatcher thread", __FUNCTION__);

    // re-check if we need to send data after reading - if we do, try now
    if(rpt->thread.wanted & ND_POLL_WRITE) {
        worker_is_busy(WORKER_STREAM_JOB_SOCKET_SEND);
        if(!stream_receiver_send_data(sth, rpt, now_ut, false))
            return false;
    }

    if(rpt->host->sender &&                                         // the host has a sender
        rpt->host->stream.snd.status.tid == gettid_cached() &&      // the sender is mine
        (rpt->host->sender->thread.wanted & ND_POLL_WRITE)) {       // the sender needs to send data
        // we return true even if this fais,
        // so that we will not disconnect the receiver because the sender failed
        stream_sender_send_data(sth, rpt->host->sender, now_ut, false);
    }

    return true;
}

static ssize_t
stream_receive_and_process(struct stream_thread *sth, struct receiver_state *rpt, PARSER *parser, usec_t now_ut __maybe_unused, bool *removed) {
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

                        while (buffered_reader_next_line(&rpt->thread.uncompressed, rpt->thread.line_buffer)) {
                            if (unlikely(parser_action(parser, rpt->thread.line_buffer->buffer))) {
                                stream_receiver_remove(sth, rpt, STREAM_HANDSHAKE_RCV_DISCONNECT_PARSER_FAILED);
                                *removed = true;
                                return -1;
                            }

                            rpt->thread.line_buffer->len = 0;
                            rpt->thread.line_buffer->buffer[0] = '\0';
                        }
                    }
                    else if (decompress_rc == DECOMPRESS_NEED_MORE_DATA)
                        break;

                    else {
                        stream_receiver_remove(sth, rpt, STREAM_HANDSHAKE_RCV_DECOMPRESSION_FAILED);
                        *removed = true;
                        return -1;
                    }
                }
            }
            else if (feed_rc == DECOMPRESS_NEED_MORE_DATA)
                break;
            else {
                stream_receiver_remove(sth, rpt, STREAM_HANDSHAKE_RCV_DECOMPRESSION_FAILED);
                *removed = true;
                return -1;
            }
        }

        if(receiver_should_stop(rpt)) {
            stream_receiver_remove(sth, rpt, STREAM_HANDSHAKE_DISCONNECT_SIGNALED_TO_STOP);
            *removed = true;
            return -1;
        }
    }
    else {
        rc = receiver_read_uncompressed(rpt);
        if(rc <= 0)
            return rc;

        while(buffered_reader_next_line(&rpt->thread.uncompressed, rpt->thread.line_buffer)) {
            if(unlikely(parser_action(parser, rpt->thread.line_buffer->buffer))) {
                stream_receiver_remove(sth, rpt, STREAM_HANDSHAKE_RCV_DISCONNECT_PARSER_FAILED);
                *removed = true;
                return -1;
            }

            rpt->thread.line_buffer->len = 0;
            rpt->thread.line_buffer->buffer[0] = '\0';
        }
    }

    return rc;
}

bool stream_receiver_send_data(struct stream_thread *sth, struct receiver_state *rpt, usec_t now_ut, bool process_opcodes_and_enable_removal) {
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
            pulse_stream_sent_bytes(rc);
            rpt->thread.last_traffic_ut = now_ut;
            stream_circular_buffer_del_unsafe(scb, rc, now_ut);
            if (!stats->bytes_outstanding) {
                rpt->thread.wanted = ND_POLL_READ;
                if (!nd_poll_upd(sth->run.ndpl, rpt->sock.fd, rpt->thread.wanted))
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
            STREAM_HANDSHAKE reason;

            if(status == EVLOOP_STATUS_SOCKET_ERROR) {
                worker_is_busy(WORKER_STREAM_JOB_DISCONNECT_SEND_ERROR);
                reason = STREAM_HANDSHAKE_DISCONNECT_SOCKET_WRITE_FAILED;
            }
            else /* if(status == EVLOOP_STATUS_SOCKET_CLOSED) */ {
                worker_is_busy(WORKER_STREAM_JOB_DISCONNECT_REMOTE_CLOSED);
                reason = STREAM_HANDSHAKE_DISCONNECT_SOCKET_CLOSED_BY_REMOTE;
            }

            nd_log(NDLS_DAEMON, NDLP_ERR,
                   "STREAM RCV[%zu] '%s' [from [%s]:%s]: %s (%zd, on fd %d) - closing receiver connection - "
                   "we have sent %zu bytes in %zu operations.",
                   sth->id, rrdhost_hostname(rpt->host), rpt->remote_ip, rpt->remote_port,
                   stream_handshake_error_to_string(reason), rc, rpt->sock.fd, stats->bytes_sent, stats->sends);

            if(process_opcodes_and_enable_removal) {
                // this is not executed from the opcode handling mechanism
                // so we can safely remove the receiver.
                stream_receiver_remove(sth, rpt, reason);
            }
            else {
                receiver_set_exit_reason(rpt, reason, false);

                // protection against this case:
                //
                // 1. receiver gets a replication request
                // 2. parser processes the request
                // 3. parser decides to send back a message to the child (REPLAY_CHART)
                // 4. send_to_child appends the data to the sending circular buffer
                // 5. send_to_child sends opcode to enable sending
                // 6. opcode bypasses the signal and runs this function inline to dispatch immediately
                // 7. sending fails (child disconnected)
                // 8. receiver is removed
                //
                // Point 2 above crashes. The parser is no longer there (freed at point 7)
                // and there is no way for point 2 to know...
            }
        }
        else if(process_opcodes_and_enable_removal &&
                 status == EVLOOP_STATUS_CONTINUE &&
                 stream_thread_process_opcodes(sth, &rpt->thread.meta))
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

    size_t count = 1; // how many reads to do per host, before moving to the next host
    EVLOOP_STATUS status = EVLOOP_STATUS_CONTINUE;
    while(status == EVLOOP_STATUS_CONTINUE && count-- > 0) {
        bool removed = false;
        ssize_t rc = stream_receive_and_process(sth, rpt, parser, now_ut, &removed);
        if(unlikely(removed))
            status = EVLOOP_STATUS_PARSER_FAILED;

        else if (likely(rc > 0)) {
            rpt->thread.last_traffic_ut = now_ut;

            if(!stream_receiver_dequeue_senders(sth, rpt, now_ut))
                status = EVLOOP_STATUS_SOCKET_ERROR;
        }
        else if (rc == 0 || errno == ECONNRESET) {
            status = EVLOOP_STATUS_SOCKET_CLOSED;
        }
        else if (rc < 0) {
            if ((errno == EWOULDBLOCK || errno == EAGAIN || errno == EINTR))
                status = EVLOOP_STATUS_SOCKET_FULL;
            else
                status = EVLOOP_STATUS_SOCKET_ERROR;
        }

        if(status == EVLOOP_STATUS_SOCKET_ERROR || status == EVLOOP_STATUS_SOCKET_CLOSED) {
            STREAM_HANDSHAKE reason;

            if(status == EVLOOP_STATUS_SOCKET_ERROR) {
                worker_is_busy(WORKER_STREAM_JOB_DISCONNECT_RECEIVE_ERROR);
                reason = STREAM_HANDSHAKE_DISCONNECT_SOCKET_READ_FAILED;
            }
            else /* if(status == EVLOOP_STATUS_SOCKET_CLOSED) */ {
                worker_is_busy(WORKER_STREAM_JOB_DISCONNECT_REMOTE_CLOSED);
                reason = STREAM_HANDSHAKE_DISCONNECT_SOCKET_CLOSED_BY_REMOTE;
            }

            nd_log(NDLS_DAEMON, NDLP_ERR,
                   "STREAM RCV[%zu] '%s' [from [%s]:%s]: %s (fd %d) - closing receiver connection.",
                   sth->id, rrdhost_hostname(rpt->host), rpt->remote_ip, rpt->remote_port,
                   stream_handshake_error_to_string(reason), rpt->sock.fd);

            stream_receiver_remove(sth, rpt, reason);
            break;
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
        stream_receiver_remove(sth, rpt, STREAM_HANDSHAKE_DISCONNECT_SIGNALED_TO_STOP);
        return false;
    }

    if (unlikely(events & (ND_POLL_ERROR | ND_POLL_HUP | ND_POLL_INVALID))) {
        // we have errors on this socket

        worker_is_busy(WORKER_STREAM_JOB_DISCONNECT_SOCKET_ERROR);

        STREAM_HANDSHAKE reason = events & ND_POLL_HUP ? STREAM_HANDSHAKE_DISCONNECT_SOCKET_CLOSED_BY_REMOTE : STREAM_HANDSHAKE_DISCONNECT_SOCKET_ERROR;

        nd_log(NDLS_DAEMON, NDLP_ERR,
               "STREAM RCV[%zu] '%s' [from [%s]:%s]: %s - closing connection",
               sth->id, rrdhost_hostname(rpt->host), rpt->remote_ip, rpt->remote_port,
               stream_handshake_error_to_string(reason));

        stream_receiver_remove(sth, rpt, reason);
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

        // Probe socket to detect dead connections (e.g., from TCP keepalive)
        // Uses nd_sock_peek_nowait() which handles both SSL and plain TCP:
        // - For SSL: uses SSL_peek() to avoid corrupting SSL state
        // - For plain TCP: uses recv(MSG_PEEK | MSG_DONTWAIT)
        char probe_byte;
        ssize_t probe_rc = nd_sock_peek_nowait(&rpt->sock, &probe_byte, 1);
        if (probe_rc == 0) {
            // Connection closed gracefully by remote
            ND_LOG_STACK lgs[] = {
                ND_LOG_FIELD_TXT(NDF_SRC_IP, rpt->remote_ip),
                ND_LOG_FIELD_TXT(NDF_SRC_PORT, rpt->remote_port),
                ND_LOG_FIELD_TXT(NDF_NIDL_NODE, rpt->hostname),
                ND_LOG_FIELD_CB(NDF_SRC_TRANSPORT, stream_receiver_log_transport, rpt),
                ND_LOG_FIELD_CB(NDF_SRC_CAPABILITIES, stream_receiver_log_capabilities, rpt),
                ND_LOG_FIELD_END(),
            };
            ND_LOG_STACK_PUSH(lgs);

            worker_is_busy(WORKER_STREAM_JOB_DISCONNECT_REMOTE_CLOSED);
            nd_log(NDLS_DAEMON, NDLP_ERR,
                   "STREAM RCV[%zu] '%s' [from %s]: socket closed by remote - closing connection",
                   sth->id, rrdhost_hostname(rpt->host), rpt->remote_ip);

            stream_receiver_remove(sth, rpt, STREAM_HANDSHAKE_DISCONNECT_SOCKET_CLOSED_BY_REMOTE);
            continue;
        }
        if (probe_rc < 0 && errno != EAGAIN && errno != EWOULDBLOCK && errno != ECONNRESET) {
            // Socket error detected (keepalive timeout, etc.)
            // Save errno immediately as subsequent calls may modify it
            int saved_errno = errno;

            ND_LOG_STACK lgs[] = {
                ND_LOG_FIELD_TXT(NDF_SRC_IP, rpt->remote_ip),
                ND_LOG_FIELD_TXT(NDF_SRC_PORT, rpt->remote_port),
                ND_LOG_FIELD_TXT(NDF_NIDL_NODE, rpt->hostname),
                ND_LOG_FIELD_CB(NDF_SRC_TRANSPORT, stream_receiver_log_transport, rpt),
                ND_LOG_FIELD_CB(NDF_SRC_CAPABILITIES, stream_receiver_log_capabilities, rpt),
                ND_LOG_FIELD_END(),
            };
            ND_LOG_STACK_PUSH(lgs);

            worker_is_busy(WORKER_STREAM_JOB_DISCONNECT_SOCKET_ERROR);
            nd_log(NDLS_DAEMON, NDLP_ERR,
                   "STREAM RCV[%zu] '%s' [from %s]: socket error detected: %s - closing connection",
                   sth->id, rrdhost_hostname(rpt->host), rpt->remote_ip, strerror(saved_errno));

            stream_receiver_remove(sth, rpt, STREAM_HANDSHAKE_DISCONNECT_SOCKET_ERROR);
            continue;
        }
        // probe_rc > 0: data available (normal)
        // probe_rc < 0 with EAGAIN/EWOULDBLOCK: no data but connection alive

        spinlock_lock(&rpt->thread.send_to_child.spinlock);
        STREAM_CIRCULAR_BUFFER_STATS stats = *stream_circular_buffer_stats_unsafe(rpt->thread.send_to_child.scb);
        spinlock_unlock(&rpt->thread.send_to_child.spinlock);

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

            worker_is_busy(WORKER_STREAM_JOB_DISCONNECT_TIMEOUT);

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

            stream_receiver_remove(sth, rpt, STREAM_HANDSHAKE_DISCONNECT_TIMEOUT);
            continue;
        }

        nd_poll_event_t wanted = ND_POLL_READ | (stats.bytes_outstanding ? ND_POLL_WRITE : 0);
        if(unlikely(rpt->thread.wanted != wanted)) {
//            nd_log(NDLS_DAEMON, NDLP_DEBUG,
//                   "STREAM RCV[%zu] '%s' [from %s]: nd_poll() wanted events mismatch.",
//                   sth->id, rrdhost_hostname(rpt->host), rpt->remote_ip);

            rpt->thread.wanted = wanted;
            if(!nd_poll_upd(sth->run.ndpl, rpt->sock.fd, rpt->thread.wanted))
                nd_log(NDLS_DAEMON, NDLP_ERR,
                       "STREAM RCV[%zu] '%s' [from %s]: failed to update nd_poll().",
                       sth->id, rrdhost_hostname(rpt->host), rpt->remote_ip);
        }
    }
}

static bool stream_receiver_did_replication_progress(struct receiver_state *rpt) {
    RRDHOST *host = rpt->host;

    size_t host_counter_sum =
        __atomic_load_n(&host->stream.rcv.status.replication.counter_in, __ATOMIC_RELAXED) +
        __atomic_load_n(&host->stream.rcv.status.replication.counter_out, __ATOMIC_RELAXED);

    if(rpt->replication.last_counter_sum != host_counter_sum) {
        // there has been some progress
        rpt->replication.last_counter_sum = host_counter_sum;
        rpt->replication.last_progress_ut = now_monotonic_usec();
        return true;
    }

    if(!host_counter_sum)
        // we have not started yet
        return true;

    if(__atomic_load_n(&host->stream.rcv.status.replication.backfill_pending, __ATOMIC_RELAXED))
        // we still have requests to execute
        return true;

    return (now_monotonic_usec() - rpt->replication.last_progress_ut < 10ULL * 60 * USEC_PER_SEC);
}

void stream_receiver_replication_check_from_poll(struct stream_thread *sth, usec_t now_ut __maybe_unused) {
    internal_fatal(sth->tid != gettid_cached(), "Function %s() should only be used by the dispatcher thread", __FUNCTION__);

    Word_t idx = 0;
    for(struct pollfd_meta *m = META_FIRST(&sth->run.meta, &idx);
         m;
         m = META_NEXT(&sth->run.meta, &idx)) {
        if (m->type != POLLFD_TYPE_RECEIVER) continue;
        struct receiver_state *rpt = m->rpt;
        RRDHOST *host = rpt->host;


        if(stream_receiver_did_replication_progress(rpt)) {
            rpt->replication.last_checked_ut = 0;
            continue;
        }

        if(rpt->replication.last_checked_ut == rpt->replication.last_progress_ut)
            continue;

        size_t stalled = 0, finished = 0;
        RRDSET *st;
        rrdset_foreach_read(st, rpt->host) {
            RRDSET_FLAGS st_flags = rrdset_flag_get(st);
            if(st_flags & RRDSET_FLAG_OBSOLETE)
                continue;

            if(st_flags & RRDSET_FLAG_RECEIVER_REPLICATION_FINISHED) {
                finished++;
                continue;
            }

            nd_log(NDLS_DAEMON, NDLP_DEBUG,
                   "STREAM RCV[%zu] '%s' [from %s]: REPLICATION EXCEPTIONS: instance '%s' %s replication yet.",
                   sth->id, rrdhost_hostname(host), rpt->remote_ip,
                   rrdset_id(st),
                   (st_flags & RRDSET_FLAG_RECEIVER_REPLICATION_IN_PROGRESS) ? "has not finished" : "has not started");

            stalled++;
        }
        rrdset_foreach_done(st);

        if(stalled && !stream_receiver_did_replication_progress(rpt)) {
            nd_log(NDLS_DAEMON, NDLP_WARNING,
                   "STREAM RCV[%zu] '%s' [from %s]: REPLICATION EXCEPTIONS SUMMARY: node has %zu stalled replication requests (%zu finished). "
                   "We have requested %u and got replies for %u replication commands. "
                   "Disconnecting node to restore streaming.",
                   sth->id, rrdhost_hostname(rpt->host), rpt->remote_ip,
                   stalled, finished,
                   __atomic_load_n(&host->stream.rcv.status.replication.counter_out, __ATOMIC_RELAXED),
                   __atomic_load_n(&host->stream.rcv.status.replication.counter_in, __ATOMIC_RELAXED));

            stream_receiver_remove(sth, rpt, STREAM_HANDSHAKE_DISCONNECT_REPLICATION_STALLED);
            continue;
        }

        rpt->replication.last_checked_ut = rpt->replication.last_progress_ut;
    }
}

void stream_receiver_cleanup(struct stream_thread *sth) {
    Word_t idx = 0;
    for(struct pollfd_meta *m = META_FIRST(&sth->run.meta, &idx);
         m;
         m = META_NEXT(&sth->run.meta, &idx)) {
        if (m->type != POLLFD_TYPE_RECEIVER) continue;
        struct receiver_state *rpt = m->rpt;
        receiver_set_exit_reason(rpt, STREAM_HANDSHAKE_DISCONNECT_SHUTDOWN, true);
        stream_receiver_remove(sth, rpt, STREAM_HANDSHAKE_DISCONNECT_SHUTDOWN);
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
               "STREAM REPLAY ERROR: receiver replication instances counter should be zero, but it is %u"
               " - resetting it to zero",
               rrdhost_receiver_replicating_charts(host));

        rrdhost_receiver_replicating_charts_zero(host);
    }

    __atomic_store_n(&host->stream.rcv.status.replication.counter_in, 0, __ATOMIC_RELAXED);
    __atomic_store_n(&host->stream.rcv.status.replication.counter_out, 0, __ATOMIC_RELAXED);
    __atomic_store_n(&host->stream.rcv.status.replication.backfill_pending, 0, __ATOMIC_RELAXED);
}

bool rrdhost_set_receiver(RRDHOST *host, struct receiver_state *rpt) {
    bool signal_rrdcontext = false;
    bool set_this = false;

    rrdhost_receiver_lock(host);

    if (!host->receiver) {
        object_state_activate(&host->state_id);

        rrdhost_flag_clear(host, RRDHOST_FLAG_ORPHAN);
        rrdhost_set_health_evloop_iteration(host);

        host->stream.rcv.status.connections++;
        streaming_receiver_connected();

        host->receiver = rpt;
        rpt->host = host;

        host->stream.rcv.status.reason = (STREAM_HANDSHAKE)rpt->capabilities;
        rpt->exit.reason = 0;
        __atomic_store_n(&rpt->exit.shutdown, false, __ATOMIC_RELEASE);
        host->stream.rcv.status.last_connected = now_realtime_sec();
        host->stream.rcv.status.last_disconnected = 0;

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

        stream_parents_host_reset(host, STREAM_HANDSHAKE_SP_PREPARING);

        set_this = true;
    }

    rrdhost_receiver_unlock(host);

    if(signal_rrdcontext)
        rrdcontext_host_child_connected(host);

    if(set_this)
        ml_host_start(host);

    return set_this;
}

void rrdhost_clear_receiver(struct receiver_state *rpt, STREAM_HANDSHAKE reason) {
    RRDHOST *host = rpt->host;
    if(!host) return;

    rrdhost_receiver_lock(host);
    {
        // Make sure that we detach this thread and don't kill a freshly arriving receiver

        if (host->receiver == rpt) {
            rrdhost_flag_clear(host, RRDHOST_FLAG_COLLECTOR_ONLINE);

            rrdhost_receiver_unlock(host);
            {
                // this will wait until all workers finish
                object_state_deactivate(&host->state_id);

                // run all these without having the receiver lock

                rrdhost_set_health_evloop_iteration(host);
                ml_host_stop(host);
                stream_path_child_disconnected(host);
                stream_sender_signal_to_stop_and_wait(host, reason, false);
                rrdcontext_host_child_disconnected(host);

                if (rpt->config.health.enabled)
                    rrdcalc_child_disconnected(host);

                stream_parents_host_reset(host, reason);
            }
            rrdhost_receiver_lock(host);

            // now we have the lock again

            stream_receiver_replication_reset(host);
            streaming_receiver_disconnected();

            host->stream.rcv.status.reason = rpt->exit.reason;
            rpt->exit.reason = 0;
            __atomic_store_n(&rpt->exit.shutdown, false, __ATOMIC_RELEASE);
            host->stream.rcv.status.last_connected = 0;
            host->stream.rcv.status.last_disconnected = now_realtime_sec();
            host->health.enabled = false;

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

    struct receiver_state *rpt = host->receiver;

    if(rpt) {
        if(!__atomic_load_n(&rpt->exit.shutdown, __ATOMIC_ACQUIRE)) {
            receiver_set_exit_reason(rpt, reason, true);
            __atomic_store_n(&rpt->exit.shutdown, true, __ATOMIC_RELEASE);
            shutdown(rpt->sock.fd, SHUT_RDWR);
        }

        int count = 2000;
        while (host->receiver == rpt && count-- > 0) {
            rrdhost_receiver_unlock(host);

            // let the lock for the receiver thread to exit
            sleep_usec(1 * USEC_PER_MS);

            rrdhost_receiver_lock(host);
        }

        if(host->receiver == rpt)
            netdata_log_error("STREAM RCV[x] '%s' [from [%s]:%s]: "
                              "streaming thread takes too long to stop, giving up..."
                              , rrdhost_hostname(host)
                                  , rpt->remote_ip, rpt->remote_port);
        else
            ret = true;
    }
    else
        ret = true;

    rrdhost_receiver_unlock(host);

    return ret;
}
