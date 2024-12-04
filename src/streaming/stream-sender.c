// SPDX-License-Identifier: GPL-3.0-or-later

#include "stream-thread.h"
#include "stream-sender-internals.h"

static void stream_sender_move_running_to_connector_or_remove(struct stream_thread *sth, struct sender_state *s, STREAM_HANDSHAKE reason, bool reconnect);

// --------------------------------------------------------------------------------------------------------------------

static void stream_sender_cbuffer_recreate_timed_unsafe(struct sender_state *s, time_t now_s, bool force) {
    static __thread time_t last_reset_time_s = 0;

    if(!force && now_s - last_reset_time_s < 300)
        return;

    last_reset_time_s = now_s;

    s->sbuf.recreates++; // we increase even if we don't do it, to have sender_start() recreate its buffers

    if(s->sbuf.cb && s->sbuf.cb->size > CBUFFER_INITIAL_SIZE) {
        cbuffer_free(s->sbuf.cb);
        s->sbuf.cb = cbuffer_new(CBUFFER_INITIAL_SIZE, stream_send.buffer_max_size, &netdata_buffers_statistics.cbuffers_streaming);
    }
}

static void rrdpush_sender_cbuffer_flush(RRDHOST *host) {
    stream_sender_set_flush_time(host->sender);

    stream_sender_lock(host->sender);

    // flush the output buffer from any data it may have
    cbuffer_flush(host->sender->sbuf.cb);
    stream_sender_cbuffer_recreate_timed_unsafe(host->sender, now_monotonic_sec(), true);

    stream_sender_unlock(host->sender);
}

// --------------------------------------------------------------------------------------------------------------------

static void rrdpush_sender_charts_and_replication_reset(struct sender_state *s) {
    stream_sender_set_flush_time(s);

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
           "STREAM [dispatchX] [%s]: running on-connect hooks...",
           rrdhost_hostname(s->host));

    rrdhost_flag_set(s->host, RRDHOST_FLAG_STREAM_SENDER_CONNECTED);

    rrdpush_sender_charts_and_replication_reset(s);
    rrdpush_sender_cbuffer_flush(s->host);

    s->last_traffic_seen_t = now_monotonic_sec();
    s->rbuf.read_len = 0;
    s->sbuf.cb->read = 0;
    s->sbuf.cb->write = 0;
}

static void stream_sender_on_ready_to_dispatch(struct sender_state *s) {
    nd_log(NDLS_DAEMON, NDLP_DEBUG,
           "STREAM [dispatchX] [%s]: running ready-to-dispatch hooks...",
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
           "STREAM [dispatchX] [%s]: running on-disconnect hooks...",
           rrdhost_hostname(s->host));

    stream_sender_execute_commands_cleanup(s);
    rrdpush_sender_charts_and_replication_reset(s);
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
    s->thread.bytes_outstanding = 0;
    s->thread.bytes_available = 0;
    s->thread.buffer_ratio = 0.0;
    s->thread.sends = 0;
    s->thread.bytes_sent = 0;
    replication_recalculate_buffer_used_ratio_unsafe(s);
}

static void stream_sender_thread_data_sent_data_unsafe(struct sender_state *s, uint64_t bytes_sent) {
    s->thread.sends++;
    s->thread.bytes_sent += bytes_sent;
    s->thread.bytes_outstanding = cbuffer_next_unsafe(s->sbuf.cb, NULL);
    s->thread.bytes_available = cbuffer_available_size_unsafe(s->sbuf.cb);
    s->thread.buffer_ratio = (NETDATA_DOUBLE)(s->sbuf.cb->max_size -  s->thread.bytes_available) * 100.0 / (NETDATA_DOUBLE)s->sbuf.cb->max_size;
    replication_recalculate_buffer_used_ratio_unsafe(s);
}

void stream_sender_thread_data_added_data_unsafe(struct sender_state *s, STREAM_TRAFFIC_TYPE type, uint64_t bytes_compressed, uint64_t bytes_uncompressed) {
    // calculate the statistics for our dispatcher
    s->thread.bytes_sent_by_type[type] += bytes_compressed;

    s->thread.bytes_uncompressed += bytes_uncompressed;
    s->thread.bytes_compressed += bytes_compressed;
    s->thread.bytes_outstanding = cbuffer_next_unsafe(s->sbuf.cb, NULL);
    s->thread.bytes_available = cbuffer_available_size_unsafe(s->sbuf.cb);
    s->thread.buffer_ratio = (NETDATA_DOUBLE)(s->sbuf.cb->max_size -  s->thread.bytes_available) * 100.0 / (NETDATA_DOUBLE)s->sbuf.cb->max_size;
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
        size_t buffer_size = s->sbuf.cb->size;
        size_t buffer_max_size = s->sbuf.cb->max_size;
        size_t buffer_available = cbuffer_available_size_unsafe(s->sbuf.cb);
        stream_sender_unlock(s);
        nd_log(NDLS_DAEMON, NDLP_ERR,
               "STREAM[%zu] %s [send to %s]: send buffer is full (buffer size %zu, max %zu, available %zu). "
               "Restarting connection.",
               sth->id, rrdhost_hostname(s->host), s->connected_to,
               buffer_size, buffer_max_size, buffer_available);

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
               "STREAM[%zu] %s [send to %s]: restarting connection without compression.",
               sth->id, rrdhost_hostname(s->host), s->connected_to);

        stream_sender_move_running_to_connector_or_remove(
            sth, s, STREAM_HANDSHAKE_DISCONNECT_NOT_SUFFICIENT_SENDER_READ_BUFFER, true);
        return;
    }

    if(msg->opcode & STREAM_OPCODE_SENDER_STOP_HOST_CLEANUP) {
        worker_is_busy(WORKER_SENDER_JOB_DISCONNECT_HOST_CLEANUP);
        stream_sender_move_running_to_connector_or_remove(
            sth, s, STREAM_HANDSHAKE_DISCONNECT_HOST_CLEANUP, false);
        return;
    }

    nd_log(NDLS_DAEMON, NDLP_ERR,
           "STREAM[%zu]: invalid msg id %u", sth->id, (unsigned)msg->opcode);
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
               "STREAM[%zu] [%s]: moving host from dispatcher queue to dispatcher running...",
               sth->id, rrdhost_hostname(s->host));

        internal_fatal(SENDERS_GET(&sth->snd.senders, (Word_t)s) != NULL, "Sender already exists in senders list");
        SENDERS_SET(&sth->snd.senders, (Word_t)s, s);

        stream_sender_lock(s);
        s->thread.pfd = stream_thread_pollfd_get(sth, s->sock.fd, POLLFD_TYPE_SENDER, NULL, s);

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
           "STREAM [connector] [%s]: streaming sender removed host: %s",
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
    stream_thread_pollfd_release(sth, s->thread.pfd);

    // clear this flag asap, to stop other threads from pushing metrics for this node
    rrdhost_flag_clear(s->host, RRDHOST_FLAG_STREAM_SENDER_CONNECTED | RRDHOST_FLAG_STREAM_SENDER_READY_4_METRICS);

    // clear these asap, to make sender_commit() stop processing data for this host
    stream_sender_lock(s);
    s->thread.pfd = PFD_EMPTY;

    s->thread.msg.session = 0;
    s->thread.msg.sender = NULL;

    s->host->stream.snd.status.tid = 0;
    stream_sender_unlock(s);

    nd_log(NDLS_DAEMON, NDLP_NOTICE,
           "STREAM [dispatcher] [%s]: disconnected from parent, reason: %s",
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

void stream_sender_check_all_nodes_from_poll(struct stream_thread *sth) {
    internal_fatal(sth->tid != gettid_cached(), "Function %s() should only be used by the dispatcher thread", __FUNCTION__ );

    usec_t now_ut = now_monotonic_usec();
    time_t now_s = (time_t)(now_ut / USEC_PER_SEC);

    size_t bytes_uncompressed = 0;
    size_t bytes_compressed = 0;
    NETDATA_DOUBLE buffer_ratio = 0.0;

    Word_t idx = 0;
    for(struct sender_state *s = SENDERS_FIRST(&sth->snd.senders, &idx);
         s;
         s = SENDERS_NEXT(&sth->snd.senders, &idx)) {
        // the default for all nodes
        struct pollfd *pfd = pfd_validate(sth, s->thread.pfd);
        pfd->events = POLLIN;
        pfd->revents = 0;

        // If the TCP window never opened, then something is wrong, restart connection
        if(unlikely(now_s - s->last_traffic_seen_t > stream_send.parents.timeout_s &&
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

            nd_log(NDLS_DAEMON, NDLP_ERR,
                   "STREAM[%zu] %s [send to %s]: could not send metrics for %ld seconds - closing connection - "
                   "we have sent %zu bytes on this connection via %zu send attempts.",
                   sth->id, rrdhost_hostname(s->host), s->connected_to, stream_send.parents.timeout_s,
                   s->thread.bytes_sent, s->thread.sends);

            stream_sender_move_running_to_connector_or_remove(sth, s, STREAM_HANDSHAKE_DISCONNECT_SOCKET_READ_TIMEOUT, true);
            continue;
        }

        stream_sender_lock(s);
        {
            bytes_compressed += s->thread.bytes_compressed;
            bytes_uncompressed += s->thread.bytes_uncompressed;
            uint64_t outstanding = s->thread.bytes_outstanding;
            if (s->thread.buffer_ratio > buffer_ratio)
                buffer_ratio = s->thread.buffer_ratio;

            if (outstanding)
                pfd->events |= POLLOUT;
        }
        stream_sender_unlock(s);
    }

    if (bytes_compressed && bytes_uncompressed) {
        NETDATA_DOUBLE compression_ratio = 100.0 - ((NETDATA_DOUBLE)bytes_compressed * 100.0 / (NETDATA_DOUBLE)bytes_uncompressed);
        worker_set_metric(WORKER_SENDER_JOB_BYTES_COMPRESSION_RATIO, compression_ratio);
    }

    worker_set_metric(WORKER_SENDER_JOB_BYTES_UNCOMPRESSED, (NETDATA_DOUBLE)bytes_uncompressed);
    worker_set_metric(WORKER_SENDER_JOB_BYTES_COMPRESSED, (NETDATA_DOUBLE)bytes_compressed);
    worker_set_metric(WORKER_SENDER_JOB_BUFFER_RATIO, buffer_ratio);
}

void stream_sender_process_poll_events(struct stream_thread *sth, struct sender_state *s, short revents, time_t now_s) {
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

    if(unlikely(revents & (POLLERR|POLLHUP|POLLNVAL))) {
        // we have errors on this socket

        worker_is_busy(WORKER_STREAM_JOB_SOCKET_ERROR);

        char *error = "unknown error";

        if (revents & POLLERR)
            error = "socket reports errors (POLLERR)";
        else if (revents & POLLHUP)
            error = "connection closed by remote end (POLLHUP)";
        else if (revents & POLLNVAL)
            error = "connection is invalid (POLLNVAL)";

        worker_is_busy(WORKER_SENDER_JOB_DISCONNECT_SOCKET_ERROR);

        nd_log(NDLS_DAEMON, NDLP_ERR,
               "STREAM[%zu] %s [send to %s]: %s restarting connection - %zu bytes transmitted.",
               sth->id, rrdhost_hostname(s->host), s->connected_to, error, s->thread.bytes_sent);

        stream_sender_move_running_to_connector_or_remove(sth, s, STREAM_HANDSHAKE_DISCONNECT_SOCKET_ERROR, true);
        return;
    }

    if(revents & POLLOUT) {
        // we can send data on this socket

        worker_is_busy(WORKER_STREAM_JOB_SOCKET_SEND);

        bool disconnect = false;
        stream_sender_lock(s);
        {
            char *chunk;
            size_t outstanding = cbuffer_next_unsafe(s->sbuf.cb, &chunk);
            ssize_t bytes = nd_sock_send_nowait(&s->sock, chunk, outstanding);
            if (likely(bytes > 0)) {
                cbuffer_remove_unsafe(s->sbuf.cb, bytes);
                stream_sender_thread_data_sent_data_unsafe(s, bytes);
                s->last_traffic_seen_t = now_s;
                sth->snd.bytes_sent += bytes;

                if(!s->thread.bytes_outstanding) {
                    // we sent them all - remove POLLOUT
                    struct pollfd *pfd = pfd_validate(sth, s->thread.pfd);
                    pfd->events &= ~(POLLOUT);

                    // recreate the circular buffer if we have to
                    stream_sender_cbuffer_recreate_timed_unsafe(s, now_s, false);
                }
            }
            else if (bytes < 0 && errno != EWOULDBLOCK && errno != EAGAIN && errno != EINTR)
                disconnect = true;
        }
        stream_sender_unlock(s);

        if(disconnect) {
            worker_is_busy(WORKER_SENDER_JOB_DISCONNECT_SEND_ERROR);
            nd_log(NDLS_DAEMON, NDLP_ERR,
                   "STREAM[%zu] %s [send to %s]: failed to send metrics - restarting connection - "
                   "we have sent %zu bytes on this connection.",
                   sth->id, rrdhost_hostname(s->host), s->connected_to, s->thread.bytes_sent);
            stream_sender_move_running_to_connector_or_remove(
                sth, s, STREAM_HANDSHAKE_DISCONNECT_SOCKET_WRITE_FAILED, true);
            return;
        }
    }

    if(revents & POLLIN) {
        // we can receive data from this socket

        worker_is_busy(WORKER_STREAM_JOB_SOCKET_RECEIVE);
        ssize_t bytes = nd_sock_revc_nowait(&s->sock, s->rbuf.b + s->rbuf.read_len, sizeof(s->rbuf.b) - s->rbuf.read_len - 1);
        if (bytes > 0) {
            s->rbuf.read_len += bytes;
            s->last_traffic_seen_t = now_s;
            sth->snd.bytes_received += bytes;
        }
        else if (bytes == 0 || errno == ECONNRESET) {
            worker_is_busy(WORKER_SENDER_JOB_DISCONNECT_PARENT_CLOSED);
            nd_log(NDLS_DAEMON, NDLP_ERR,
                   "STREAM[%zu] %s [send to %s]: connection (fd %d) closed by far end.",
                   sth->id, rrdhost_hostname(s->host), s->connected_to, s->sock.fd);
            stream_sender_move_running_to_connector_or_remove(
                sth, s, STREAM_HANDSHAKE_DISCONNECT_SOCKET_CLOSED_BY_PARENT, true);
            return;
        }
        else if (bytes < 0 && errno != EWOULDBLOCK && errno != EAGAIN && errno != EINTR) {
            worker_is_busy(WORKER_SENDER_JOB_DISCONNECT_RECEIVE_ERROR);
            nd_log(NDLS_DAEMON, NDLP_ERR,
                   "STREAM[%zu] %s [send to %s]: error during receive (%zd, on fd %d) - restarting connection.",
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

