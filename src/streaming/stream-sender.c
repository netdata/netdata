// SPDX-License-Identifier: GPL-3.0-or-later

#include "stream-thread.h"
#include "stream-sender-internals.h"

static void stream_sender_move_running_to_connector_or_remove(struct stream_thread *sth, struct sender_state *s, STREAM_HANDSHAKE reason, bool reconnect);

// --------------------------------------------------------------------------------------------------------------------

static void stream_sender_charts_and_replication_reset(struct sender_state *s) {
    // stop all replication commands inflight
    replication_sender_delete_pending_requests(s);

    // reset the state of all charts
    RRDSET *st;
    rrdset_foreach_read(st, s->host) {
        rrdset_flag_clear(st, RRDSET_FLAG_SENDER_REPLICATION_IN_PROGRESS);
        rrdset_flag_set(st, RRDSET_FLAG_SENDER_REPLICATION_FINISHED);

        st->stream.snd.resync_time_s = 0;

        RRDDIM *rd;
        rrddim_foreach_read(rd, st)
            rrddim_metadata_exposed_upstream_clear(rd);
        rrddim_foreach_done(rd);

        rrdset_metadata_updated(st);
    }
    rrdset_foreach_done(st);

    rrdhost_sender_replicating_charts_zero(s->host);
    stream_sender_replicating_charts_zero(s);
}

// --------------------------------------------------------------------------------------------------------------------

void stream_sender_on_connect(struct sender_state *s) {
    nd_log(NDLS_DAEMON, NDLP_DEBUG,
           "STREAM SEND [%s]: running on-connect hooks...",
           rrdhost_hostname(s->host));

    rrdhost_flag_set(s->host, RRDHOST_FLAG_STREAM_SENDER_CONNECTED);

    stream_sender_charts_and_replication_reset(s);

    stream_sender_lock(s);
    stream_circular_buffer_flush_unsafe(s->sbuf);
    stream_sender_unlock(s);

    s->thread.last_traffic_ut = now_monotonic_usec();
    s->rbuf.read_len = 0;
}

static void stream_sender_on_ready_to_dispatch(struct sender_state *s) {
    nd_log(NDLS_DAEMON, NDLP_DEBUG,
           "STREAM SEND [%s]: running ready-to-dispatch hooks...",
           rrdhost_hostname(s->host));

    // set this flag before sending any data, or the data will not be sent
    rrdhost_flag_set(s->host, RRDHOST_FLAG_STREAM_SENDER_READY_4_METRICS);

    stream_sender_execute_commands_cleanup(s);
    stream_sender_send_custom_host_variables(s->host);
    stream_path_send_to_parent(s->host);
    stream_sender_send_claimed_id(s->host);
    stream_send_host_labels(s->host);
    stream_send_global_functions(s->host);
}

static void stream_sender_on_disconnect(struct sender_state *s) {
    nd_log(NDLS_DAEMON, NDLP_DEBUG,
           "STREAM SEND [%s]: running on-disconnect hooks...",
           rrdhost_hostname(s->host));

    stream_sender_lock(s);
    stream_circular_buffer_flush_unsafe(s->sbuf);
    stream_sender_unlock(s);

    stream_sender_execute_commands_cleanup(s);
    stream_sender_charts_and_replication_reset(s);
    stream_sender_clear_parent_claim_id(s->host);
    stream_receiver_send_node_and_claim_id_to_child(s->host);
    stream_path_parent_disconnected(s->host);
}

// --------------------------------------------------------------------------------------------------------------------

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

    buffer_strcat(wb, nd_sock_is_ssl(&state->sock) ? "https" : "http");
    return true;
}

static bool stream_sender_log_dst_ip(BUFFER *wb, void *ptr) {
    struct sender_state *state = ptr;
    if(!state || state->sock.fd == -1)
        return false;

    SOCKET_PEERS peers = nd_sock_socket_peers(&state->sock);
    buffer_strcat(wb, peers.peer.ip);
    return true;
}

static bool stream_sender_log_dst_port(BUFFER *wb, void *ptr) {
    struct sender_state *state = ptr;
    if(!state || state->sock.fd == -1)
        return false;

    SOCKET_PEERS peers = nd_sock_socket_peers(&state->sock);
    buffer_print_uint64(wb, peers.peer.port);
    return true;
}

// --------------------------------------------------------------------------------------------------------------------

static void stream_sender_thread_data_reset_unsafe(struct sender_state *s) {
    memset(s->thread.bytes_sent_by_type, 0, sizeof(s->thread.bytes_sent_by_type));

    s->thread.bytes_uncompressed = 0;
    s->thread.bytes_compressed = 0;
    s->thread.sends = 0;
    s->thread.bytes_sent = 0;

    stream_circular_buffer_flush_unsafe(s->sbuf);
    replication_recalculate_buffer_used_ratio_unsafe(s);
}

static void stream_sender_thread_data_sent_data_unsafe(struct sender_state *s, uint64_t bytes_sent) {
    s->thread.sends++;
    s->thread.bytes_sent += bytes_sent;
    replication_recalculate_buffer_used_ratio_unsafe(s);
}

void stream_sender_thread_data_added_data_unsafe(struct sender_state *s, STREAM_TRAFFIC_TYPE type, uint64_t bytes_compressed, uint64_t bytes_uncompressed) {
    // calculate the statistics for our dispatcher
    s->thread.bytes_sent_by_type[type] += bytes_compressed;
    s->thread.bytes_uncompressed += bytes_uncompressed;
    s->thread.bytes_compressed += bytes_compressed;
    replication_recalculate_buffer_used_ratio_unsafe(s);
}

// --------------------------------------------------------------------------------------------------------------------
// opcodes

void stream_sender_handle_op(struct stream_thread *sth, struct sender_state *s, struct stream_opcode *msg) {
    ND_LOG_STACK lgs[] = {
        ND_LOG_FIELD_STR(NDF_NIDL_NODE, s->host->hostname),
        ND_LOG_FIELD_CB(NDF_DST_IP, stream_sender_log_dst_ip, s),
        ND_LOG_FIELD_CB(NDF_DST_PORT, stream_sender_log_dst_port, s),
        ND_LOG_FIELD_CB(NDF_DST_TRANSPORT, stream_sender_log_transport, s),
        ND_LOG_FIELD_CB(NDF_SRC_CAPABILITIES, stream_sender_log_capabilities, s),
        ND_LOG_FIELD_UUID(NDF_MESSAGE_ID, &streaming_to_parent_msgid),
        ND_LOG_FIELD_END(),
    };
    ND_LOG_STACK_PUSH(lgs);

    if(msg->opcode & STREAM_OPCODE_SENDER_BUFFER_OVERFLOW) {
        worker_is_busy(WORKER_SENDER_JOB_DISCONNECT_OVERFLOW);
        errno_clear();
        stream_sender_lock(s);
        STREAM_CIRCULAR_BUFFER_STATS stats = stream_circular_buffer_stats_unsafe(s->sbuf);
        stream_sender_unlock(s);
        nd_log(NDLS_DAEMON, NDLP_ERR,
               "STREAM SEND[%zu] %s [to %s]: send buffer is full (buffer size %zu, max %zu, used %zu, available %zu). "
               "Restarting connection.",
               sth->id, rrdhost_hostname(s->host), s->connected_to,
               stats.bytes_size, stats.bytes_max_size, stats.bytes_outstanding, stats.bytes_available);

        stream_sender_move_running_to_connector_or_remove(
            sth, s, STREAM_HANDSHAKE_DISCONNECT_NOT_SUFFICIENT_SENDER_SEND_BUFFER, true);
        return;
    }

    if(msg->opcode & STREAM_OPCODE_SENDER_STOP_RECEIVER_LEFT) {
        worker_is_busy(WORKER_SENDER_JOB_DISCONNECT_RECEIVER_LEFT);
        stream_sender_move_running_to_connector_or_remove(
            sth, s, STREAM_HANDSHAKE_DISCONNECT_RECEIVER_LEFT, false);
        return;
    }

    if(msg->opcode & STREAM_OPCODE_SENDER_RECONNECT_WITHOUT_COMPRESSION) {
        worker_is_busy(WORKER_SENDER_JOB_DISCONNECT_COMPRESSION_ERROR);
        errno_clear();
        nd_log(NDLS_DAEMON, NDLP_ERR,
               "STREAM SEND[%zu] %s [send to %s]: restarting connection without compression.",
               sth->id, rrdhost_hostname(s->host), s->connected_to);

        stream_sender_move_running_to_connector_or_remove(
            sth, s, STREAM_HANDSHAKE_DISCONNECT_NOT_SUFFICIENT_SENDER_COMPRESSION_FAILED, true);
        return;
    }

    if(msg->opcode & STREAM_OPCODE_SENDER_STOP_HOST_CLEANUP) {
        worker_is_busy(WORKER_SENDER_JOB_DISCONNECT_HOST_CLEANUP);
        stream_sender_move_running_to_connector_or_remove(
            sth, s, STREAM_HANDSHAKE_DISCONNECT_HOST_CLEANUP, false);
        return;
    }

    nd_log(NDLS_DAEMON, NDLP_ERR,
           "STREAM SEND[%zu]: invalid msg id %u", sth->id, (unsigned)msg->opcode);
}


// --------------------------------------------------------------------------------------------------------------------

void stream_sender_move_queue_to_running_unsafe(struct stream_thread *sth) {
    internal_fatal(sth->tid != gettid_cached(), "Function %s() should only be used by the dispatcher thread", __FUNCTION__ );

    // process the queue
    Word_t idx = 0;
    for(struct sender_state *s = SENDERS_FIRST(&sth->queue.senders, &idx);
         s;
         s = SENDERS_NEXT(&sth->queue.senders, &idx)) {
        worker_is_busy(WORKER_STREAM_JOB_DEQUEUE);

        SENDERS_DEL(&sth->queue.senders, (Word_t)s);

        ND_LOG_STACK lgs[] = {
            ND_LOG_FIELD_STR(NDF_NIDL_NODE, s->host->hostname),
            ND_LOG_FIELD_UUID(NDF_MESSAGE_ID, &streaming_to_parent_msgid),
            ND_LOG_FIELD_END(),
        };
        ND_LOG_STACK_PUSH(lgs);

        nd_log(NDLS_DAEMON, NDLP_DEBUG,
               "STREAM SEND[%zu] [%s]: moving host from dispatcher queue to dispatcher running...",
               sth->id, rrdhost_hostname(s->host));

        internal_fatal(SENDERS_GET(&sth->snd.senders, (Word_t)s) != NULL, "Sender already exists in senders list");
        SENDERS_SET(&sth->snd.senders, (Word_t)s, s);

        stream_sender_lock(s);
        s->thread.meta.type = POLLFD_TYPE_SENDER;
        s->thread.meta.s = s;
        if(!nd_poll_add(sth->run.ndpl, s->sock.fd, ND_POLL_READ, &s->thread.meta))
            internal_fatal(true, "Failed to add sender socket to nd_poll()");

        s->thread.msg.thread_slot = (int32_t)sth->id;
        s->thread.msg.session = os_random32();
        s->thread.msg.sender = s;

        s->host->stream.snd.status.tid = gettid_cached();
        s->host->stream.snd.status.connections++;
        s->last_state_since_t = now_realtime_sec();

        stream_sender_thread_data_reset_unsafe(s);
        stream_sender_unlock(s);

        stream_sender_on_ready_to_dispatch(s);
    }
}

void stream_sender_remove(struct sender_state *s) {
    // THIS FUNCTION IS USED BY THE CONNECTOR TOO
    // when it gives up on a certain node

    nd_log(NDLS_DAEMON, NDLP_NOTICE,
           "STREAM SEND [%s]: streaming sender removed host: %s",
           rrdhost_hostname(s->host), stream_handshake_error_to_string(s->exit.reason));

    stream_sender_lock(s);

    __atomic_store_n(&s->exit.shutdown, false, __ATOMIC_RELAXED);
    rrdhost_flag_clear(s->host,
        RRDHOST_FLAG_STREAM_SENDER_ADDED | RRDHOST_FLAG_STREAM_SENDER_CONNECTED |
            RRDHOST_FLAG_STREAM_SENDER_READY_4_METRICS);

    s->last_state_since_t = now_realtime_sec();
    stream_parent_set_disconnect_reason(s->host->stream.snd.parents.current, s->exit.reason, s->last_state_since_t);
    s->connector.id = -1;

    stream_sender_unlock(s);

    rrdhost_stream_parents_reset(s->host, STREAM_HANDSHAKE_EXITING);

#ifdef NETDATA_LOG_STREAM_SENDER
    if (s->stream_log_fp) {
        fclose(s->stream_log_fp);
        s->stream_log_fp = NULL;
    }
#endif
}

static void stream_sender_move_running_to_connector_or_remove(struct stream_thread *sth, struct sender_state *s, STREAM_HANDSHAKE reason, bool reconnect) {
    internal_fatal(sth->tid != gettid_cached(), "Function %s() should only be used by the dispatcher thread", __FUNCTION__ );

    internal_fatal(SENDERS_GET(&sth->snd.senders, (Word_t)s) == NULL, "Sender to be removed is not in the list of senders");
    SENDERS_DEL(&sth->snd.senders, (Word_t)s);
    if(!nd_poll_del(sth->run.ndpl, s->sock.fd))
        internal_fatal(true, "Failed to remove sender socket from nd_poll()");

    // clear this flag asap, to stop other threads from pushing metrics for this node
    rrdhost_flag_clear(s->host, RRDHOST_FLAG_STREAM_SENDER_CONNECTED | RRDHOST_FLAG_STREAM_SENDER_READY_4_METRICS);

    // clear these asap, to make sender_commit() stop processing data for this host
    stream_sender_lock(s);

    s->thread.msg.session = 0;
    s->thread.msg.sender = NULL;

    s->host->stream.snd.status.tid = 0;
    stream_sender_unlock(s);

    nd_log(NDLS_DAEMON, NDLP_NOTICE,
           "STREAM SEND [%s]: sender disconnected from parent, reason: %s",
           rrdhost_hostname(s->host), stream_handshake_error_to_string(reason));

    nd_sock_close(&s->sock);

    stream_parent_set_disconnect_reason(s->host->stream.snd.parents.current, reason, now_realtime_sec());
    stream_sender_on_disconnect(s);

    bool should_remove = !reconnect || stream_connector_is_signaled_to_stop(s);

    stream_thread_node_removed(s->host);

    if (should_remove)
        stream_sender_remove(s);
    else
        stream_connector_requeue(s);
}

void stream_sender_check_all_nodes_from_poll(struct stream_thread *sth, usec_t now_ut) {
    internal_fatal(sth->tid != gettid_cached(), "Function %s() should only be used by the dispatcher thread", __FUNCTION__ );

    size_t bytes_uncompressed = 0;
    size_t bytes_compressed = 0;
    NETDATA_DOUBLE overall_buffer_ratio = 0.0;

    Word_t idx = 0;
    for(struct sender_state *s = SENDERS_FIRST(&sth->snd.senders, &idx);
         s;
         s = SENDERS_NEXT(&sth->snd.senders, &idx)) {

        stream_sender_lock(s);
        STREAM_CIRCULAR_BUFFER_STATS stats = stream_circular_buffer_stats_unsafe(s->sbuf);
        stream_sender_unlock(s);

        if (stats.buffer_ratio > overall_buffer_ratio)
            overall_buffer_ratio = stats.buffer_ratio;

        if(unlikely(s->thread.last_traffic_ut + stream_send.parents.timeout_s * USEC_PER_SEC < now_ut &&
                     !stream_sender_pending_replication_requests(s) &&
                     !stream_sender_replicating_charts(s)
                         )) {

            ND_LOG_STACK lgs[] = {
                ND_LOG_FIELD_STR(NDF_NIDL_NODE, s->host->hostname),
                ND_LOG_FIELD_CB(NDF_DST_IP, stream_sender_log_dst_ip, s),
                ND_LOG_FIELD_CB(NDF_DST_PORT, stream_sender_log_dst_port, s),
                ND_LOG_FIELD_CB(NDF_DST_TRANSPORT, stream_sender_log_transport, s),
                ND_LOG_FIELD_CB(NDF_SRC_CAPABILITIES, stream_sender_log_capabilities, s),
                ND_LOG_FIELD_UUID(NDF_MESSAGE_ID, &streaming_to_parent_msgid),
                ND_LOG_FIELD_END(),
            };
            ND_LOG_STACK_PUSH(lgs);

            worker_is_busy(WORKER_SENDER_JOB_DISCONNECT_TIMEOUT);

            char since[RFC3339_MAX_LENGTH];
            rfc3339_datetime_ut(since, sizeof(since), s->thread.last_traffic_ut, 2, false);

            char pending[64];
            size_snprintf(pending, sizeof(pending), stats.bytes_outstanding, "B", false);

            nd_log(NDLS_DAEMON, NDLP_ERR,
                   "STREAM SEND[%zu] %s [send to %s]: could not send data for %ld seconds - closing connection - "
                   "we have sent %zu bytes in %zu operations, it is idle since: %s, and we have %s pending to send "
                   "(buffer is used %.2f%%).",
                   sth->id, rrdhost_hostname(s->host), s->connected_to, stream_send.parents.timeout_s,
                   s->thread.bytes_sent, s->thread.sends, since, pending, stats.buffer_ratio);

            stream_sender_move_running_to_connector_or_remove(sth, s, STREAM_HANDSHAKE_DISCONNECT_SOCKET_TIMEOUT, true);
            continue;
        }

        bytes_compressed += s->thread.bytes_compressed;
        bytes_uncompressed += s->thread.bytes_uncompressed;

        if(!nd_poll_upd(sth->run.ndpl, s->sock.fd, ND_POLL_READ | (stats.bytes_outstanding ? ND_POLL_WRITE : 0), &s->thread.meta))
            nd_log(NDLS_DAEMON, NDLP_ERR,
                   "STREAM SEND[%zu] %s [send to %s]: failed to update nd_poll().",
                   sth->id, rrdhost_hostname(s->host), s->connected_to);
    }

    if (bytes_compressed && bytes_uncompressed) {
        NETDATA_DOUBLE compression_ratio = 100.0 - ((NETDATA_DOUBLE)bytes_compressed * 100.0 / (NETDATA_DOUBLE)bytes_uncompressed);
        worker_set_metric(WORKER_SENDER_JOB_BYTES_COMPRESSION_RATIO, compression_ratio);
    }

    worker_set_metric(WORKER_SENDER_JOB_BYTES_UNCOMPRESSED, (NETDATA_DOUBLE)bytes_uncompressed);
    worker_set_metric(WORKER_SENDER_JOB_BYTES_COMPRESSED, (NETDATA_DOUBLE)bytes_compressed);
    worker_set_metric(WORKER_SENDER_JOB_BUFFER_RATIO, overall_buffer_ratio);
}

void stream_sender_process_poll_events(struct stream_thread *sth, struct sender_state *s, nd_poll_event_t events, usec_t now_ut) {
    internal_fatal(sth->tid != gettid_cached(), "Function %s() should only be used by the dispatcher thread", __FUNCTION__ );

    ND_LOG_STACK lgs[] = {
        ND_LOG_FIELD_STR(NDF_NIDL_NODE, s->host->hostname),
        ND_LOG_FIELD_CB(NDF_DST_IP, stream_sender_log_dst_ip, s),
        ND_LOG_FIELD_CB(NDF_DST_PORT, stream_sender_log_dst_port, s),
        ND_LOG_FIELD_CB(NDF_DST_TRANSPORT, stream_sender_log_transport, s),
        ND_LOG_FIELD_CB(NDF_SRC_CAPABILITIES, stream_sender_log_capabilities, s),
        ND_LOG_FIELD_UUID(NDF_MESSAGE_ID, &streaming_to_parent_msgid),
        ND_LOG_FIELD_END(),
    };
    ND_LOG_STACK_PUSH(lgs);

    if(unlikely(events & ND_POLL_ERROR)) {
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
               "STREAM SEND[%zu] %s [send to %s]: %s restarting connection - %zu bytes transmitted.",
               sth->id, rrdhost_hostname(s->host), s->connected_to, error, s->thread.bytes_sent);

        stream_sender_move_running_to_connector_or_remove(sth, s, STREAM_HANDSHAKE_DISCONNECT_SOCKET_ERROR, true);
        return;
    }

    if(events & ND_POLL_WRITE) {
        // we can send data on this socket

        worker_is_busy(WORKER_STREAM_JOB_SOCKET_SEND);

        bool disconnect = false;
        stream_sender_lock(s);
        {
            char *chunk;
            size_t outstanding = stream_circular_buffer_get_unsafe(s->sbuf, &chunk);
            ssize_t bytes = nd_sock_send_nowait(&s->sock, chunk, outstanding);
            if (likely(bytes > 0)) {
                stream_circular_buffer_del_unsafe(s->sbuf, bytes);
                stream_sender_thread_data_sent_data_unsafe(s, bytes);
                s->thread.last_traffic_ut = now_ut;
                sth->snd.bytes_sent += bytes;

                STREAM_CIRCULAR_BUFFER_STATS stats = stream_circular_buffer_stats_unsafe(s->sbuf);
                if(!stats.bytes_outstanding) {
                    // we sent them all - remove ND_POLL_WRITE
                    if(!nd_poll_upd(sth->run.ndpl, s->sock.fd, ND_POLL_READ, &s->thread.meta))
                        nd_log(NDLS_DAEMON, NDLP_ERR,
                               "STREAM SEND[%zu] %s [send to %s]: failed to update nd_poll().",
                               sth->id, rrdhost_hostname(s->host), s->connected_to);

                    // recreate the circular buffer if we have to
                    stream_circular_buffer_recreate_timed_unsafe(s->sbuf, now_ut, false);
                }
            }
            else if (bytes < 0 && errno != EWOULDBLOCK && errno != EAGAIN && errno != EINTR)
                disconnect = true;
        }
        stream_sender_unlock(s);

        if(disconnect) {
            worker_is_busy(WORKER_SENDER_JOB_DISCONNECT_SEND_ERROR);
            nd_log(NDLS_DAEMON, NDLP_ERR,
                   "STREAM SEND[%zu] %s [send to %s]: failed to send metrics - restarting connection - "
                   "we have sent %zu bytes on this connection.",
                   sth->id, rrdhost_hostname(s->host), s->connected_to, s->thread.bytes_sent);
            stream_sender_move_running_to_connector_or_remove(
                sth, s, STREAM_HANDSHAKE_DISCONNECT_SOCKET_WRITE_FAILED, true);
            return;
        }
    }

    if(events & ND_POLL_READ) {
        // we can receive data from this socket

        worker_is_busy(WORKER_STREAM_JOB_SOCKET_RECEIVE);
        ssize_t bytes = nd_sock_revc_nowait(&s->sock, s->rbuf.b + s->rbuf.read_len, sizeof(s->rbuf.b) - s->rbuf.read_len - 1);
        if (bytes > 0) {
            s->rbuf.read_len += bytes;
            s->thread.last_traffic_ut = now_ut;
            sth->snd.bytes_received += bytes;
        }
        else if (bytes == 0 || errno == ECONNRESET) {
            worker_is_busy(WORKER_SENDER_JOB_DISCONNECT_PARENT_CLOSED);
            nd_log(NDLS_DAEMON, NDLP_ERR,
                   "STREAM SEND[%zu] %s [send to %s]: connection (fd %d) closed by far end.",
                   sth->id, rrdhost_hostname(s->host), s->connected_to, s->sock.fd);
            stream_sender_move_running_to_connector_or_remove(
                sth, s, STREAM_HANDSHAKE_DISCONNECT_SOCKET_CLOSED_BY_PARENT, true);
            return;
        }
        else if (bytes < 0 && errno != EWOULDBLOCK && errno != EAGAIN && errno != EINTR) {
            worker_is_busy(WORKER_SENDER_JOB_DISCONNECT_RECEIVE_ERROR);
            nd_log(NDLS_DAEMON, NDLP_ERR,
                   "STREAM SEND[%zu] %s [send to %s]: error during receive (%zd, on fd %d) - restarting connection.",
                   sth->id, rrdhost_hostname(s->host), s->connected_to, bytes, s->sock.fd);
            stream_sender_move_running_to_connector_or_remove(
                sth, s, STREAM_HANDSHAKE_DISCONNECT_SOCKET_READ_FAILED, true);
            return;
        }
    }

    if(unlikely(s->rbuf.read_len)) {
        worker_is_busy(WORKER_SENDER_JOB_EXECUTE);
        stream_sender_execute_commands(s);
    }
}

void stream_sender_cleanup(struct stream_thread *sth) {
    // stop all hosts
    Word_t idx = 0;
    for(struct sender_state *s = SENDERS_FIRST(&sth->snd.senders, &idx);
         s;
         s = SENDERS_NEXT(&sth->snd.senders, &idx)) {

        ND_LOG_STACK lgs[] = {
            ND_LOG_FIELD_STR(NDF_NIDL_NODE, s->host->hostname),
            ND_LOG_FIELD_CB(NDF_DST_IP, stream_sender_log_dst_ip, s),
            ND_LOG_FIELD_CB(NDF_DST_PORT, stream_sender_log_dst_port, s),
            ND_LOG_FIELD_CB(NDF_DST_TRANSPORT, stream_sender_log_transport, s),
            ND_LOG_FIELD_CB(NDF_SRC_CAPABILITIES, stream_sender_log_capabilities, s),
            ND_LOG_FIELD_UUID(NDF_MESSAGE_ID, &streaming_to_parent_msgid),
            ND_LOG_FIELD_END(),
        };
        ND_LOG_STACK_PUSH(lgs);

        stream_sender_move_running_to_connector_or_remove(sth, s, STREAM_HANDSHAKE_DISCONNECT_SHUTDOWN, false);
    }

    // cleanup
    SENDERS_FREE(&sth->snd.senders, NULL);
}

