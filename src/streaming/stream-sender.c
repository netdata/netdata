// SPDX-License-Identifier: GPL-3.0-or-later

#include "stream-thread.h"
#include "stream-sender-internals.h"
#include "stream-replication-sender.h"

static void stream_sender_move_running_to_connector_or_remove(struct stream_thread *sth, struct sender_state *s, STREAM_HANDSHAKE reason, STREAM_HANDSHAKE receiver_reason, bool reconnect);

// --------------------------------------------------------------------------------------------------------------------

#ifdef NETDATA_LOG_STREAM_SENDER
void stream_sender_log_payload(struct sender_state *s, BUFFER *payload, STREAM_TRAFFIC_TYPE type __maybe_unused, bool inbound) {
    spinlock_lock(&s->log.spinlock);

    if (!s->log.fp) {
        char filename[FILENAME_MAX + 1];
        snprintfz(
            filename, FILENAME_MAX, "/tmp/stream-sender-%s.txt", s->host ? rrdhost_hostname(s->host) : "unknown");

        s->log.fp = fopen(filename, "w");

        // Align first_call to wall clock time
        clock_gettime(CLOCK_REALTIME, &s->log.first_call);
        s->log.first_call.tv_nsec = 0; // Align to the start of the second
    }

    if (s->log.fp) {
        struct timespec now;
        clock_gettime(CLOCK_REALTIME, &now);

        time_t elapsed_sec = now.tv_sec - s->log.first_call.tv_sec;
        long elapsed_nsec = now.tv_nsec - s->log.first_call.tv_nsec;

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

        const char *line_start = buffer_tostring(payload);
        const char *line_end;

        while (line_start && *line_start) {
            line_end = strchr(line_start, '\n');
            if (line_end) {
                fprintf(s->log.fp, "%s%s%.*s\n", prefix, inbound ? "> " : "< ", (int)(line_end - line_start), line_start);
                line_start = line_end + 1;
            } else {
                fprintf(s->log.fp, "%s%s%s\n", prefix, inbound ? "> " : "< ", line_start);
                break;
            }
        }
    }

    // fflush(s->log.fp);
    spinlock_unlock(&s->log.spinlock);
}
#endif

// --------------------------------------------------------------------------------------------------------------------

void stream_sender_charts_and_replication_reset(struct sender_state *s) {
    // stop all replication commands inflight
    replication_sender_delete_pending_requests(s);

    // reset the state of all charts
    RRDSET *st;
    rrdset_foreach_read(st, s->host) {
        RRDSET_FLAGS old = rrdset_flag_set_and_clear(st, RRDSET_FLAG_SENDER_REPLICATION_FINISHED, RRDSET_FLAG_SENDER_REPLICATION_IN_PROGRESS);
        if(!(old & RRDSET_FLAG_SENDER_REPLICATION_FINISHED))
            rrdhost_sender_replicating_charts_minus_one(st->rrdhost);

#ifdef REPLICATION_TRACKING
        st->stream.snd.who = REPLAY_WHO_UNKNOWN;
#endif

        st->stream.snd.resync_time_s = 0;

        RRDDIM *rd;
        rrddim_foreach_read(rd, st)
            rrddim_metadata_exposed_upstream_clear(rd);
        rrddim_foreach_done(rd);

        rrdset_metadata_updated(st);
    }
    rrdset_foreach_done(st);

    if(rrdhost_sender_replicating_charts(s->host) != 0) {
        nd_log(NDLS_DAEMON, NDLP_WARNING,
               "STREAM REPLAY ERROR: sender replicating instances counter should be zero, but it is %u"
               " - resetting it to zero",
               rrdhost_sender_replicating_charts(s->host));

        rrdhost_sender_replicating_charts_zero(s->host);
    }

    stream_sender_replicating_charts_zero(s);

    __atomic_store_n(&s->host->stream.snd.status.replication.counter_in, 0, __ATOMIC_RELAXED);
    __atomic_store_n(&s->host->stream.snd.status.replication.counter_out, 0, __ATOMIC_RELAXED);
}

// --------------------------------------------------------------------------------------------------------------------

static void stream_sender_on_connect_and_disconnect(struct sender_state *s) {
    stream_sender_execute_commands_cleanup(s);
    stream_sender_charts_and_replication_reset(s);

    stream_sender_lock(s);
    stream_circular_buffer_flush_unsafe(s->scb, stream_send.buffer_max_size);
    stream_sender_unlock(s);
}

void stream_sender_on_connect(struct sender_state *s) {
    nd_log(NDLS_DAEMON, NDLP_DEBUG,
           "STREAM SND [%s]: running on-connect hooks...",
           rrdhost_hostname(s->host));

    rrdhost_flag_set(s->host, RRDHOST_FLAG_STREAM_SENDER_CONNECTED);

    stream_sender_on_connect_and_disconnect(s);

    s->thread.last_traffic_ut = now_monotonic_usec();

    freez(s->thread.rbuf.b);
    s->thread.rbuf.size = PLUGINSD_LINE_MAX + 1;
    s->thread.rbuf.b = mallocz(s->thread.rbuf.size);
    s->thread.rbuf.b[0] = '\0';
    s->thread.rbuf.read_len = 0;
}

static void stream_sender_on_ready_to_dispatch(struct sender_state *s) {
    nd_log(NDLS_DAEMON, NDLP_DEBUG,
           "STREAM SND '%s': running ready-to-dispatch hooks...",
           rrdhost_hostname(s->host));

    // set this flag before sending any data, or the data will not be sent
    rrdhost_flag_set(s->host, RRDHOST_FLAG_STREAM_SENDER_READY_4_METRICS);

    // send our global metadata to the parent
    stream_sender_send_custom_host_variables(s->host);
    stream_path_send_to_parent(s->host);
    stream_sender_send_claimed_id(s->host);
    stream_send_host_labels(s->host);
    stream_send_global_functions(s->host);
}

void stream_sender_on_disconnect(struct sender_state *s) {
    nd_log(NDLS_DAEMON, NDLP_DEBUG,
           "STREAM SND '%s': running on-disconnect hooks...",
           rrdhost_hostname(s->host));

    stream_sender_on_connect_and_disconnect(s);

    // update the child (the receiver side) for this parent
    stream_path_parent_disconnected(s->host);
    stream_receiver_send_node_and_claim_id_to_child(s->host);

    freez(s->thread.rbuf.b);
    s->thread.rbuf.size = 0;
    s->thread.rbuf.b = NULL;
    s->thread.rbuf.read_len = 0;
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
// opcodes

ALWAYS_INLINE
void stream_sender_handle_op(struct stream_thread *sth, struct sender_state *s, struct stream_opcode *msg) {
    ND_LOG_STACK lgs[] = {
        ND_LOG_FIELD_STR(NDF_NIDL_NODE, s->host->hostname),
        ND_LOG_FIELD_CB(NDF_DST_IP, stream_sender_log_dst_ip, s),
        ND_LOG_FIELD_CB(NDF_DST_PORT, stream_sender_log_dst_port, s),
        ND_LOG_FIELD_CB(NDF_DST_TRANSPORT, stream_sender_log_transport, s),
        ND_LOG_FIELD_CB(NDF_DST_CAPABILITIES, stream_sender_log_capabilities, s),
        ND_LOG_FIELD_END(),
    };
    ND_LOG_STACK_PUSH(lgs);

    if(msg->opcode & STREAM_OPCODE_SENDER_BUFFER_OVERFLOW) {
        worker_is_busy(WORKER_SENDER_JOB_DISCONNECT_OVERFLOW);
        errno_clear();
        stream_sender_lock(s);
        // copy the statistics
        STREAM_CIRCULAR_BUFFER_STATS stats = *stream_circular_buffer_stats_unsafe(s->scb);
        stream_sender_unlock(s);
        nd_log(NDLS_DAEMON, NDLP_ERR,
               "STREAM SND[%zu] '%s' [to %s]: send buffer is full (buffer size %u, max %u, used %u, available %u). "
               "Restarting connection.",
               sth->id, rrdhost_hostname(s->host), s->remote_ip,
               stats.bytes_size, stats.bytes_max_size, stats.bytes_outstanding, stats.bytes_available);

        stream_sender_move_running_to_connector_or_remove(
            sth, s, STREAM_HANDSHAKE_DISCONNECT_BUFFER_OVERFLOW, 0, true);
        return;
    }

    if(msg->opcode & STREAM_OPCODE_SENDER_STOP_RECEIVER_LEFT) {
        worker_is_busy(WORKER_SENDER_JOB_DISCONNECT_RECEIVER_LEFT);
        stream_sender_move_running_to_connector_or_remove(
            sth, s, STREAM_HANDSHAKE_SND_DISCONNECT_RECEIVER_LEFT, msg->reason, false);

        // at this point we also have access to the receiver exit reason as msg->reason

        return;
    }

    if(msg->opcode & STREAM_OPCODE_SENDER_RECONNECT_WITHOUT_COMPRESSION) {
        worker_is_busy(WORKER_SENDER_JOB_DISCONNECT_COMPRESSION_ERROR);
        errno_clear();
        nd_log(NDLS_DAEMON, NDLP_ERR,
               "STREAM SND[%zu] '%s' [to %s]: restarting connection without compression.",
               sth->id, rrdhost_hostname(s->host), s->remote_ip);

        stream_sender_move_running_to_connector_or_remove(
            sth, s, STREAM_HANDSHAKE_SND_DISCONNECT_COMPRESSION_FAILED, 0, true);
        return;
    }

    if(msg->opcode & STREAM_OPCODE_SENDER_STOP_HOST_CLEANUP) {
        worker_is_busy(WORKER_SENDER_JOB_DISCONNECT_HOST_CLEANUP);
        stream_sender_move_running_to_connector_or_remove(
            sth, s, STREAM_HANDSHAKE_SND_DISCONNECT_HOST_CLEANUP, 0, false);
        return;
    }

    nd_log(NDLS_DAEMON, NDLP_ERR,
           "STREAM SND[%zu]: invalid msg id %u", sth->id, (unsigned)msg->opcode);
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

        SENDERS_DEL(&sth->queue.senders, idx);

        ND_LOG_STACK lgs[] = {
            ND_LOG_FIELD_STR(NDF_NIDL_NODE, s->host->hostname),
            ND_LOG_FIELD_CB(NDF_DST_IP, stream_sender_log_dst_ip, s),
            ND_LOG_FIELD_CB(NDF_DST_PORT, stream_sender_log_dst_port, s),
            ND_LOG_FIELD_CB(NDF_DST_TRANSPORT, stream_sender_log_transport, s),
            ND_LOG_FIELD_CB(NDF_DST_CAPABILITIES, stream_sender_log_capabilities, s),
            ND_LOG_FIELD_END(),
        };
        ND_LOG_STACK_PUSH(lgs);

        nd_log(NDLS_DAEMON, NDLP_DEBUG,
               "STREAM SND[%zu] '%s' [to %s]: moving host from dispatcher queue to dispatcher running...",
               sth->id, rrdhost_hostname(s->host), s->remote_ip);

        if(sock_setnonblock(s->sock.fd, true) != 1)
            nd_log(NDLS_DAEMON, NDLP_DEBUG,
                   "STREAM SND[%zu] '%s' [to %s]: failed to set non-blocking mode on socket %d",
                   sth->id, rrdhost_hostname(s->host), s->remote_ip, s->sock.fd);

        sock_setcloexec(s->sock.fd, true);
        sock_enlarge_rcv_buf(s->sock.fd);
        sock_enlarge_snd_buf(s->sock.fd);
        sock_setcork(s->sock.fd, false);

        stream_sender_lock(s);
        s->thread.meta.type = POLLFD_TYPE_SENDER;
        s->thread.meta.s = s;

        s->thread.msg.thread_slot = (int32_t)sth->id;
        s->thread.msg.session = os_random32();
        s->thread.msg.meta = &s->thread.meta;

        __atomic_store_n(&s->host->stream.snd.status.tid, gettid_cached(), __ATOMIC_RELAXED);
        s->host->stream.snd.status.connections++;
        s->last_state_since_t = now_realtime_sec();

        s->replication.last_progress_ut = now_monotonic_usec();

        stream_circular_buffer_flush_unsafe(s->scb, stream_send.buffer_max_size);
        replication_sender_recalculate_buffer_used_ratio_unsafe(s);
        stream_sender_unlock(s);

        internal_fatal(META_GET(&sth->run.meta, (Word_t)&s->thread.meta) != NULL, "Sender already exists in meta list");
        META_SET(&sth->run.meta, (Word_t)&s->thread.meta, &s->thread.meta);

        s->thread.wanted = ND_POLL_READ;
        if(!nd_poll_add(sth->run.ndpl, s->sock.fd, s->thread.wanted, &s->thread.meta))
            nd_log(NDLS_DAEMON, NDLP_ERR,
                   "STREAM SND[%zu] '%s' [to %s]: failed to add sender socket to nd_poll()",
                   sth->id, rrdhost_hostname(s->host), s->remote_ip);

        stream_sender_on_ready_to_dispatch(s);

        pulse_host_status(s->host, PULSE_HOST_STATUS_SND_RUNNING, 0);
    }
}

void stream_sender_remove(struct sender_state *s, STREAM_HANDSHAKE reason) {
    // THIS FUNCTION IS USED BY THE CONNECTOR TOO
    // when it gives up on a certain node

    stream_sender_lock(s);

    if(reason == STREAM_HANDSHAKE_DISCONNECT_SIGNALED_TO_STOP && s->exit.reason)
        reason = s->exit.reason;

    s->exit.reason = 0;

    __atomic_store_n(&s->exit.shutdown, false, __ATOMIC_RELAXED);
    rrdhost_flag_clear(s->host,
                       RRDHOST_FLAG_STREAM_SENDER_ADDED | RRDHOST_FLAG_STREAM_SENDER_CONNECTED |
                           RRDHOST_FLAG_STREAM_SENDER_READY_4_METRICS);

    s->last_state_since_t = now_realtime_sec();
    stream_parent_set_host_disconnect_reason(s->host, reason, s->last_state_since_t);
    s->connector.id = -1;

    stream_sender_unlock(s);

    stream_parents_host_reset(s->host, reason);

#ifdef NETDATA_LOG_STREAM_SENDER
    spinlock_lock(&s->log.spinlock);
    if (s->log.fp) {
        fclose(s->log.fp);
        s->log.fp = NULL;
    }
    buffer_free(s->log.received);
    s->log.received = NULL;
    spinlock_unlock(&s->log.spinlock);
#endif
}

static void stream_sender_log_disconnection(struct stream_thread *sth, struct sender_state *s, STREAM_HANDSHAKE reason, STREAM_HANDSHAKE receiver_reason) {
    ND_LOG_STACK lgs[] = {
        ND_LOG_FIELD_UUID(NDF_MESSAGE_ID, &streaming_to_parent_msgid),
        ND_LOG_FIELD_END(),
    };
    ND_LOG_STACK_PUSH(lgs);

    if(reason == STREAM_HANDSHAKE_SND_DISCONNECT_RECEIVER_LEFT && receiver_reason)
        nd_log(NDLS_DAEMON, NDLP_NOTICE,
               "STREAM SND[%zu] '%s' [to %s]: sender disconnected from parent, reason: %s (receiver left due to: %s)",
               sth->id, rrdhost_hostname(s->host), s->remote_ip,
               stream_handshake_error_to_string(reason),
               stream_handshake_error_to_string(receiver_reason));
    else
        nd_log(NDLS_DAEMON, NDLP_NOTICE,
               "STREAM SND[%zu] '%s' [to %s]: sender disconnected from parent, reason: %s",
               sth->id, rrdhost_hostname(s->host), s->remote_ip, stream_handshake_error_to_string(reason));
}

static void stream_sender_move_running_to_connector_or_remove(struct stream_thread *sth, struct sender_state *s, STREAM_HANDSHAKE reason, STREAM_HANDSHAKE receiver_reason, bool reconnect) {
    internal_fatal(sth->tid != gettid_cached(), "Function %s() should only be used by the dispatcher thread", __FUNCTION__ );

    ND_LOG_STACK lgs[] = {
        ND_LOG_FIELD_STR(NDF_NIDL_NODE, s->host->hostname),
        ND_LOG_FIELD_CB(NDF_DST_IP, stream_sender_log_dst_ip, s),
        ND_LOG_FIELD_CB(NDF_DST_PORT, stream_sender_log_dst_port, s),
        ND_LOG_FIELD_CB(NDF_DST_TRANSPORT, stream_sender_log_transport, s),
        ND_LOG_FIELD_CB(NDF_DST_CAPABILITIES, stream_sender_log_capabilities, s),
        ND_LOG_FIELD_END(),
    };
    ND_LOG_STACK_PUSH(lgs);

    internal_fatal(META_GET(&sth->run.meta, (Word_t)&s->thread.meta) == NULL, "Sender to be removed is not in the list of senders");
    META_DEL(&sth->run.meta, (Word_t)&s->thread.meta);

    s->thread.wanted = 0;
    if(!nd_poll_del(sth->run.ndpl, s->sock.fd))
        nd_log(NDLS_DAEMON, NDLP_ERR,
               "STREAM SND[%zu] '%s' [to %s]: failed to delete sender socket from nd_poll()",
               sth->id, rrdhost_hostname(s->host), s->remote_ip);

    // clear this flag asap, to stop other threads from pushing metrics for this node
    rrdhost_flag_clear(s->host, RRDHOST_FLAG_STREAM_SENDER_CONNECTED | RRDHOST_FLAG_STREAM_SENDER_READY_4_METRICS);

    // clear these asap, to make sender_commit() stop processing data for this host
    stream_sender_lock(s);

    if(reason == STREAM_HANDSHAKE_DISCONNECT_SIGNALED_TO_STOP && s->exit.reason)
        reason = s->exit.reason;

    s->exit.reason = reason;
    s->thread.msg.session = 0;
    s->thread.msg.meta = NULL;

    __atomic_store_n(&s->host->stream.snd.status.tid, 0, __ATOMIC_RELAXED);
    stream_sender_unlock(s);

    stream_sender_log_disconnection(sth, s, reason, receiver_reason);

    nd_sock_close(&s->sock);

    stream_parent_set_host_disconnect_reason(s->host, reason, now_realtime_sec());
    stream_sender_clear_parent_claim_id(s->host);
    sender_host_buffer_free(s->host);

    pulse_host_status(s->host, PULSE_HOST_STATUS_SND_OFFLINE, reason);

    stream_thread_node_removed(s->host);

    stream_connector_requeue(
        s, reconnect && !stream_connector_is_signaled_to_stop(s) ? STRCNT_CMD_CONNECT : STRCNT_CMD_REMOVE);
}

void stream_sender_check_all_nodes_from_poll(struct stream_thread *sth, usec_t now_ut) {
    internal_fatal(sth->tid != gettid_cached(), "Function %s() should only be used by the dispatcher thread", __FUNCTION__ );

    size_t bytes_uncompressed = 0;
    size_t bytes_compressed = 0;
    NETDATA_DOUBLE overall_buffer_ratio = 0.0;

    Word_t idx = 0;
    for(struct pollfd_meta *m = META_FIRST(&sth->run.meta, &idx);
         m;
         m = META_NEXT(&sth->run.meta, &idx)) {
        if(m->type != POLLFD_TYPE_SENDER) continue;
        struct sender_state *s = m->s;

        stream_sender_lock(s);
        // copy the statistics
        STREAM_CIRCULAR_BUFFER_STATS stats = *stream_circular_buffer_stats_unsafe(s->scb);
        stream_sender_unlock(s);

        if (stats.buffer_ratio > overall_buffer_ratio)
            overall_buffer_ratio = stats.buffer_ratio;

        if(unlikely(stats.bytes_outstanding &&
                     s->thread.last_traffic_ut + stream_send.parents.timeout_s * USEC_PER_SEC < now_ut &&
                     !stream_sender_pending_replication_requests(s) &&
                     !stream_sender_replicating_charts(s))) {

            ND_LOG_STACK lgs[] = {
                ND_LOG_FIELD_STR(NDF_NIDL_NODE, s->host->hostname),
                ND_LOG_FIELD_CB(NDF_DST_IP, stream_sender_log_dst_ip, s),
                ND_LOG_FIELD_CB(NDF_DST_PORT, stream_sender_log_dst_port, s),
                ND_LOG_FIELD_CB(NDF_DST_TRANSPORT, stream_sender_log_transport, s),
                ND_LOG_FIELD_CB(NDF_DST_CAPABILITIES, stream_sender_log_capabilities, s),
                ND_LOG_FIELD_END(),
            };
            ND_LOG_STACK_PUSH(lgs);

            worker_is_busy(WORKER_STREAM_JOB_DISCONNECT_TIMEOUT);

            char duration[RFC3339_MAX_LENGTH];
            duration_snprintf(duration, sizeof(duration), (int64_t)(now_monotonic_usec() - s->thread.last_traffic_ut), "us", true);

            char pending[64] = "0";
            if(stats.bytes_outstanding)
                size_snprintf(pending, sizeof(pending), stats.bytes_outstanding, "B", false);

            nd_log(NDLS_DAEMON, NDLP_ERR,
                   "STREAM SND[%zu] '%s' [to %s]: there was not traffic for %ld seconds - closing connection - "
                   "we have sent %zu bytes in %zu operations, it is idle for %s, and we have %s pending to send "
                   "(buffer is used %.2f%%).",
                   sth->id, rrdhost_hostname(s->host), s->remote_ip, stream_send.parents.timeout_s,
                   stats.bytes_sent, stats.sends,
                   duration, pending, stats.buffer_ratio);

            stream_sender_move_running_to_connector_or_remove(sth, s, STREAM_HANDSHAKE_DISCONNECT_TIMEOUT, 0, true);
            continue;
        }

        bytes_compressed += stats.bytes_added;
        bytes_uncompressed += stats.bytes_uncompressed;

        nd_poll_event_t wanted = ND_POLL_READ | (stats.bytes_outstanding ? ND_POLL_WRITE : 0);
        if(unlikely(s->thread.wanted != wanted)) {
//            nd_log(NDLS_DAEMON, NDLP_DEBUG,
//                   "STREAM SND[%zu] '%s' [to %s]: nd_poll() wanted events mismatch.",
//                   sth->id, rrdhost_hostname(s->host), s->remote_ip);

            s->thread.wanted = wanted;
            if(!nd_poll_upd(sth->run.ndpl, s->sock.fd, s->thread.wanted))
                nd_log(NDLS_DAEMON, NDLP_ERR,
                       "STREAM SND[%zu] '%s' [to %s]: failed to update nd_poll().",
                       sth->id, rrdhost_hostname(s->host), s->remote_ip);
        }
    }

    if (bytes_compressed && bytes_uncompressed) {
        NETDATA_DOUBLE compression_ratio = 100.0 - ((NETDATA_DOUBLE)bytes_compressed * 100.0 / (NETDATA_DOUBLE)bytes_uncompressed);
        worker_set_metric(WORKER_SENDER_JOB_BYTES_COMPRESSION_RATIO, compression_ratio);
    }

    worker_set_metric(WORKER_SENDER_JOB_BYTES_UNCOMPRESSED, (NETDATA_DOUBLE)bytes_uncompressed);
    worker_set_metric(WORKER_SENDER_JOB_BYTES_COMPRESSED, (NETDATA_DOUBLE)bytes_compressed);
    worker_set_metric(WORKER_SENDER_JOB_BUFFER_RATIO, overall_buffer_ratio);
}

static bool stream_sender_did_replication_progress(struct sender_state *s) {
    RRDHOST *host = s->host;

    size_t host_counter_sum =
        __atomic_load_n(&host->stream.snd.status.replication.counter_in, __ATOMIC_RELAXED) +
        __atomic_load_n(&host->stream.snd.status.replication.counter_out, __ATOMIC_RELAXED);

    if(s->replication.last_counter_sum != host_counter_sum) {
        // there has been some progress
        s->replication.last_counter_sum = host_counter_sum;
        s->replication.last_progress_ut = now_monotonic_usec();
        return true;
    }

    if(!host_counter_sum)
        // we have not started yet
        return true;

    if(dictionary_entries(s->replication.requests))
        // we still have requests to execute
        return true;

    return (now_monotonic_usec() - s->replication.last_progress_ut < 10ULL * 60 * USEC_PER_SEC);
}

void stream_sender_replication_check_from_poll(struct stream_thread *sth, usec_t now_ut __maybe_unused) {
    internal_fatal(sth->tid != gettid_cached(), "Function %s() should only be used by the dispatcher thread", __FUNCTION__);

    Word_t idx = 0;
    for(struct pollfd_meta *m = META_FIRST(&sth->run.meta, &idx);
         m;
         m = META_NEXT(&sth->run.meta, &idx)) {
        if (m->type != POLLFD_TYPE_SENDER) continue;
        struct sender_state *s = m->s;
        RRDHOST *host = s->host;

        if(stream_sender_did_replication_progress(s)) {
            s->replication.last_checked_ut = 0;
            continue;
        }

        if(s->replication.last_checked_ut == s->replication.last_progress_ut)
            continue;

        ND_LOG_STACK lgs[] = {
            ND_LOG_FIELD_STR(NDF_NIDL_NODE, host->hostname),
            ND_LOG_FIELD_CB(NDF_DST_IP, stream_sender_log_dst_ip, s),
            ND_LOG_FIELD_CB(NDF_DST_PORT, stream_sender_log_dst_port, s),
            ND_LOG_FIELD_CB(NDF_DST_TRANSPORT, stream_sender_log_transport, s),
            ND_LOG_FIELD_CB(NDF_DST_CAPABILITIES, stream_sender_log_capabilities, s),
            ND_LOG_FIELD_END(),
        };
        ND_LOG_STACK_PUSH(lgs);

        size_t stalled = 0, finished = 0;
        RRDSET *st;
        rrdset_foreach_read(st, host) {
            RRDSET_FLAGS st_flags = rrdset_flag_get(st);
            if(st_flags & (RRDSET_FLAG_OBSOLETE | RRDSET_FLAG_UPSTREAM_IGNORE))
                continue;

            if(st_flags & RRDSET_FLAG_SENDER_REPLICATION_FINISHED) {
                finished++;
                continue;
            }

            nd_log(NDLS_DAEMON, NDLP_DEBUG,
                   "STREAM SND[%zu] '%s' [to %s]: REPLICATION STALLED: instance '%s' %s replication yet.",
                   sth->id, rrdhost_hostname(host), s->remote_ip,
                   rrdset_id(st),
                   (st_flags & RRDSET_FLAG_SENDER_REPLICATION_IN_PROGRESS) ? "has not finished" : "has not started");

            stalled++;
        }
        rrdset_foreach_done(st);

        if(stalled && !stream_sender_did_replication_progress(s)) {
            nd_log(NDLS_DAEMON, NDLP_ERR,
                   "STREAM SND[%zu] '%s' [to %s]: REPLICATION EXCEPTIONS SUMMARY: node has %zu stalled replication requests (%zu completed)."
                   "We have received %u and sent %u replication commands. "
                   "Disconnecting node to restore streaming.",
                   sth->id, rrdhost_hostname(s->host), s->remote_ip,
                   stalled, finished,
                   __atomic_load_n(&host->stream.snd.status.replication.counter_in, __ATOMIC_RELAXED),
                   __atomic_load_n(&host->stream.snd.status.replication.counter_out, __ATOMIC_RELAXED));

            stream_sender_move_running_to_connector_or_remove(sth, s, STREAM_HANDSHAKE_DISCONNECT_REPLICATION_STALLED, 0, true);
        }

        s->replication.last_checked_ut = s->replication.last_progress_ut;
    }
}

bool stream_sender_send_data(struct stream_thread *sth, struct sender_state *s, usec_t now_ut, bool process_opcodes_and_enable_removal) {
    internal_fatal(sth->tid != gettid_cached(), "Function %s() should only be used by the dispatcher thread", __FUNCTION__ );

    EVLOOP_STATUS status = EVLOOP_STATUS_CONTINUE;
    while(status == EVLOOP_STATUS_CONTINUE) {
        waitq_acquire(&s->waitq, WAITQ_PRIO_URGENT);
        stream_sender_lock(s);

        STREAM_CIRCULAR_BUFFER_STATS *stats = stream_circular_buffer_stats_unsafe(s->scb);
        char *chunk;
        size_t outstanding = stream_circular_buffer_get_unsafe(s->scb, &chunk);

        if(!outstanding) {
            status = EVLOOP_STATUS_NO_MORE_DATA;
            stream_sender_unlock(s);
            waitq_release(&s->waitq);
            continue;
        }

        ssize_t rc = nd_sock_send_nowait(&s->sock, chunk, outstanding);
        if (likely(rc > 0)) {
            pulse_stream_sent_bytes(rc);
            stream_circular_buffer_del_unsafe(s->scb, rc, now_ut);
            replication_sender_recalculate_buffer_used_ratio_unsafe(s);
            s->thread.last_traffic_ut = now_ut;
            sth->snd.bytes_sent += rc;

            if (!stats->bytes_outstanding) {
                // we sent them all - remove ND_POLL_WRITE
                s->thread.wanted = ND_POLL_READ;
                if (!nd_poll_upd(sth->run.ndpl, s->sock.fd, s->thread.wanted))
                    nd_log(NDLS_DAEMON, NDLP_ERR,
                           "STREAM SND[%zu] '%s' [to %s]: failed to update nd_poll().",
                           sth->id, rrdhost_hostname(s->host), s->remote_ip);

                // recreate the circular buffer if we have to
                stream_circular_buffer_recreate_timed_unsafe(s->scb, now_ut, false);
                status = EVLOOP_STATUS_NO_MORE_DATA;
            }
        }
        else if (rc == 0 || errno == ECONNRESET)
            status = EVLOOP_STATUS_SOCKET_CLOSED;

        else if (rc < 0) {
            if(errno == EWOULDBLOCK || errno == EAGAIN || errno == EINTR)
                status = EVLOOP_STATUS_SOCKET_FULL;
            else
                status = EVLOOP_STATUS_SOCKET_ERROR;
        }
        stream_sender_unlock(s);
        waitq_release(&s->waitq);

        if (status == EVLOOP_STATUS_SOCKET_ERROR || status == EVLOOP_STATUS_SOCKET_CLOSED) {
            const char *disconnect_reason = NULL;
            STREAM_HANDSHAKE reason;

            if(status == EVLOOP_STATUS_SOCKET_ERROR) {
                worker_is_busy(WORKER_STREAM_JOB_DISCONNECT_SEND_ERROR);
                disconnect_reason = "socket reports error while writing";
                reason = STREAM_HANDSHAKE_DISCONNECT_SOCKET_WRITE_FAILED;
            }
            else /* if(status == EVLOOP_STATUS_SOCKET_CLOSED) */ {
                worker_is_busy(WORKER_STREAM_JOB_DISCONNECT_REMOTE_CLOSED);
                disconnect_reason = "socket reports EOF (closed by parent)";
                reason = STREAM_HANDSHAKE_DISCONNECT_SOCKET_CLOSED_BY_REMOTE;
            }

            nd_log(NDLS_DAEMON, NDLP_ERR,
                   "STREAM SND[%zu] '%s' [to %s]: %s (%zd, on fd %d) - restarting connection - "
                   "we have sent %zu bytes in %zu operations.",
                   sth->id, rrdhost_hostname(s->host), s->remote_ip, disconnect_reason, rc, s->sock.fd,
                   stats->bytes_sent, stats->sends);

            if(process_opcodes_and_enable_removal) {
                // this is not executed from the opcode handling mechanism
                // so we can safely remove the sender
                stream_sender_move_running_to_connector_or_remove(sth, s, reason, 0, true);
            }
            else {
                // protection against this case:
                //
                // 1. sender gets a function request
                // 2. sender executes the request
                // 3. response is immediately available
                // 4. sender_commit() appends the data to the sending circular buffer
                // 5. sender_commit() sends opcode to enable sending
                // 6. opcode bypasses the signal and runs this function inline to dispatch immediately
                // 7. sending fails (remote disconnected)
                // 8. sender is removed
                //
                // Point 2 above crashes. The sender is no longer there (freed at point 8)
                // and there is no way for point 2 to know...
            }
        }
        else if(process_opcodes_and_enable_removal &&
                 status == EVLOOP_STATUS_CONTINUE &&
                 stream_thread_process_opcodes(sth, &s->thread.meta))
            status = EVLOOP_STATUS_OPCODE_ON_ME;
    }

    return EVLOOP_STATUS_STILL_ALIVE(status);
}

bool stream_sender_receive_data(struct stream_thread *sth, struct sender_state *s, usec_t now_ut, bool process_opcodes) {
    EVLOOP_STATUS status = EVLOOP_STATUS_CONTINUE;
    while(status == EVLOOP_STATUS_CONTINUE) {
        ssize_t rc = nd_sock_revc_nowait(&s->sock, s->thread.rbuf.b + s->thread.rbuf.read_len, s->thread.rbuf.size - s->thread.rbuf.read_len - 1);
        if (likely(rc > 0)) {
            s->thread.rbuf.read_len += rc;

            s->thread.last_traffic_ut = now_ut;
            sth->snd.bytes_received += rc;
            pulse_stream_received_bytes(rc);

            worker_is_busy(WORKER_SENDER_JOB_EXECUTE);
            stream_sender_execute_commands(s);
        }
        else if (rc == 0 || errno == ECONNRESET)
            status = EVLOOP_STATUS_SOCKET_CLOSED;

        else if (rc < 0) {
            if(errno == EWOULDBLOCK || errno == EAGAIN || errno == EINTR)
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
                reason = STREAM_HANDSHAKE_DISCONNECT_SOCKET_CLOSED_BY_REMOTE;
                disconnect_reason = "socket reports EOF (closed by parent)";
            }

            nd_log(NDLS_DAEMON, NDLP_ERR,
                   "STREAM SND[%zu] '%s' [to %s]: %s (fd %d) - restarting sender connection.",
                   sth->id, rrdhost_hostname(s->host), s->remote_ip, disconnect_reason, s->sock.fd);

            stream_sender_move_running_to_connector_or_remove(
                sth, s, reason, 0, true);
        }
        else if(status == EVLOOP_STATUS_CONTINUE && process_opcodes && stream_thread_process_opcodes(sth, &s->thread.meta))
            status = EVLOOP_STATUS_OPCODE_ON_ME;
    }

    return EVLOOP_STATUS_STILL_ALIVE(status);
}

// process poll() events for streaming senders
// returns true when the sender is still there, false if it removed it
bool stream_sender_process_poll_events(struct stream_thread *sth, struct sender_state *s, nd_poll_event_t events, usec_t now_ut) {
    internal_fatal(sth->tid != gettid_cached(), "Function %s() should only be used by the dispatcher thread", __FUNCTION__ );

    ND_LOG_STACK lgs[] = {
        ND_LOG_FIELD_STR(NDF_NIDL_NODE, s->host->hostname),
        ND_LOG_FIELD_CB(NDF_DST_IP, stream_sender_log_dst_ip, s),
        ND_LOG_FIELD_CB(NDF_DST_PORT, stream_sender_log_dst_port, s),
        ND_LOG_FIELD_CB(NDF_DST_TRANSPORT, stream_sender_log_transport, s),
        ND_LOG_FIELD_CB(NDF_DST_CAPABILITIES, stream_sender_log_capabilities, s),
        ND_LOG_FIELD_END(),
    };
    ND_LOG_STACK_PUSH(lgs);

    if(unlikely(events & (ND_POLL_ERROR|ND_POLL_HUP|ND_POLL_INVALID))) {
        // we have errors on this socket

        char *error = "unknown error";

        if (events & ND_POLL_ERROR)
            error = "socket reports errors";
        else if (events & ND_POLL_HUP)
            error = "connection closed by remote end (HUP)";
        else if (events & ND_POLL_INVALID)
            error = "connection is invalid";

        worker_is_busy(WORKER_STREAM_JOB_DISCONNECT_SOCKET_ERROR);

        stream_sender_lock(s);
        // copy the statistics
        STREAM_CIRCULAR_BUFFER_STATS stats = *stream_circular_buffer_stats_unsafe(s->scb);
        stream_sender_unlock(s);

        nd_log(NDLS_DAEMON, NDLP_ERR,
               "STREAM SND[%zu] '%s' [to %s]: %s restarting connection - %zu bytes transmitted in %zu operations.",
               sth->id, rrdhost_hostname(s->host), s->remote_ip, error, stats.bytes_sent, stats.sends);

        stream_sender_move_running_to_connector_or_remove(sth, s, STREAM_HANDSHAKE_DISCONNECT_SOCKET_ERROR, 0, true);
        return false;
    }

    if(events & ND_POLL_READ) {
        worker_is_busy(WORKER_STREAM_JOB_SOCKET_RECEIVE);
        if(!stream_sender_receive_data(sth, s, now_ut, true))
            return false;
    }

    if(events & ND_POLL_WRITE) {
        worker_is_busy(WORKER_STREAM_JOB_SOCKET_SEND);
        if(!stream_sender_send_data(sth, s, now_ut, true))
            return false;
    }

    return true;
}

void stream_sender_cleanup(struct stream_thread *sth) {
    // stop all hosts
    Word_t idx = 0;
    for(struct pollfd_meta *m = META_FIRST(&sth->run.meta, &idx);
         m;
         m = META_NEXT(&sth->run.meta, &idx)) {
        if(m->type != POLLFD_TYPE_SENDER) continue;
        struct sender_state *s = m->s;

        s->exit.reason = STREAM_HANDSHAKE_DISCONNECT_SHUTDOWN;
        s->exit.shutdown = true;
        stream_sender_move_running_to_connector_or_remove(sth, s, STREAM_HANDSHAKE_DISCONNECT_SHUTDOWN, 0, false);
    }
}
