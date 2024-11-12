// SPDX-License-Identifier: GPL-3.0-or-later

#include "sender-internals.h"

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

void rrdpush_sender_cbuffer_recreate_timed(struct sender_state *s, time_t now_s, bool have_mutex, bool force) {
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

void rrdpush_sender_on_connect(RRDHOST *host) {
    rrdpush_sender_cbuffer_flush(host);
    rrdpush_sender_charts_and_replication_reset(host);
}

void rrdpush_sender_after_connect(RRDHOST *host) {
    rrdpush_sender_thread_send_custom_host_variables(host);
}

static void rrdpush_sender_on_disconnect(RRDHOST *host) {
    // we have been connected to this parent - let's cleanup

    rrdpush_sender_charts_and_replication_reset(host);

    // clear the parent's claim id
    rrdpush_sender_clear_parent_claim_id(host);
    rrdpush_receiver_send_node_and_claim_id_to_child(host);
    stream_path_parent_disconnected(host);
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

    ret = nd_sock_send_nowait(&s->sock, chunk, outstanding);
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
        rrdpush_sender_thread_close_socket(s);
    }
    else
        netdata_log_debug(D_STREAM, "STREAM: send() returned 0 -> no error but no transmission");

    replication_recalculate_buffer_used_ratio_unsafe(s);
    sender_unlock(s);

    return ret;
}

static ssize_t attempt_read(struct sender_state *s) {
    ssize_t ret = nd_sock_revc_nowait(&s->sock, s->read_buffer + s->read_len, sizeof(s->read_buffer) - s->read_len - 1);

    if (ret > 0) {
        s->read_len += ret;
        return ret;
    }

    if (ret < 0 && (errno == EAGAIN || errno == EWOULDBLOCK || errno == EINTR))
        return ret;

    if (nd_sock_is_ssl(&s->sock))
        worker_is_busy(WORKER_SENDER_JOB_DISCONNECT_SSL_ERROR);
    else if (ret == 0 || errno == ECONNRESET) {
        worker_is_busy(WORKER_SENDER_JOB_DISCONNECT_PARENT_CLOSED);
        netdata_log_error("STREAM %s [send to %s]: connection (fd %d) closed by far end.",
                          rrdhost_hostname(s->host), s->connected_to, s->sock.fd);
    }
    else {
        worker_is_busy(WORKER_SENDER_JOB_DISCONNECT_RECEIVE_ERROR);
        netdata_log_error("STREAM %s [send to %s]: error during receive (%zd, on fd %d) - closing connection.",
                          rrdhost_hostname(s->host), s->connected_to, ret, s->sock.fd);
    }

    rrdpush_sender_thread_close_socket(s);

    return ret;
}

void rrdpush_signal_sender_to_wake_up(struct sender_state *s) {
    if(unlikely(sender_tid(s->host) == gettid_cached()))
        return;

    RRDHOST *host = s->host;

    int pipe_fd = sender_write_pipe_fd(s);

    // signal the sender there are more data
    if (pipe_fd != -1 && write(pipe_fd, " ", 1) == -1)
        netdata_log_error("STREAM %s [send]: cannot write to internal pipe.", rrdhost_hostname(host));
}

static bool rrdhost_set_sender(RRDHOST *host) {
    if(unlikely(!host->sender)) return false;

    bool ret = false;
    sender_lock(host->sender);
    if(!host->sender->sender_magic) {
        rrdhost_flag_clear(host, RRDHOST_FLAG_RRDPUSH_SENDER_CONNECTED | RRDHOST_FLAG_RRDPUSH_SENDER_READY_4_METRICS);
        rrdhost_flag_set(host, RRDHOST_FLAG_RRDPUSH_SENDER_SPAWN);
        host->stream.snd.status.connections++;
        host->sender->sender_magic = sender_magic(host);
        host->sender->last_state_since_t = now_realtime_sec();
        host->sender->exit.reason = STREAM_HANDSHAKE_NEVER;
        ret = true;
    }
    sender_unlock(host->sender);

    rrdhost_stream_parents_reset(host, STREAM_HANDSHAKE_PREPARING);

    return ret;
}

static void rrdhost_clear_sender(RRDHOST *host, bool having_sender_lock) {
    if(unlikely(!host->sender)) return;

    if(!having_sender_lock)
        sender_lock(host->sender);

    if(host->sender->sender_magic == sender_magic(host)) {
        host->sender->sender_magic = 0;
        host->sender->exit.shutdown = false;
        rrdhost_flag_clear(host, RRDHOST_FLAG_RRDPUSH_SENDER_SPAWN | RRDHOST_FLAG_RRDPUSH_SENDER_CONNECTED | RRDHOST_FLAG_RRDPUSH_SENDER_READY_4_METRICS);
        host->sender->last_state_since_t = now_realtime_sec();
        stream_parent_set_disconnect_reason(
            host->stream.snd.parents.current, host->sender->exit.reason, host->sender->last_state_since_t);
    }

    rrdhost_stream_parents_reset(host, STREAM_HANDSHAKE_EXITING);

    if(!having_sender_lock)
        sender_unlock(host->sender);

#ifdef NETDATA_LOG_STREAM_SENDER
    if (s->stream_log_fp) {
        fclose(s->stream_log_fp);
        s->stream_log_fp = NULL;
    }
#endif
}

static bool rrdhost_is_sender_stopped(struct sender_state *s) {
    return __atomic_load_n(&s->stop, __ATOMIC_RELAXED);
}

bool rrdhost_sender_should_exit(struct sender_state *s) {
    if(unlikely(rrdhost_is_sender_stopped(s))) {
        if(!s->exit.reason)
            s->exit.reason = STREAM_HANDSHAKE_DISCONNECT_SHUTDOWN;
        return true;
    }

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

/*
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

    if(!rrdhost_has_rrdpush_sender_enabled(s->host) || !s->host->stream.snd.destination || !s->host->stream.snd.api_key) {
        netdata_log_error("STREAM %s [send]: thread created (task id %d), but host has streaming disabled.",
                          rrdhost_hostname(s->host), gettid_cached());
        return NULL;
    }

    if(!rrdhost_set_sender(s->host)) {
        netdata_log_error("STREAM %s [send]: thread created (task id %d), but there is another sender running for this host.",
              rrdhost_hostname(s->host), gettid_cached());
        return NULL;
    }

    rrdhost_stream_parent_ssl_init(s);

    netdata_log_info("STREAM %s [send]: thread created (task id %d)", rrdhost_hostname(s->host), gettid_cached());

    s->buffer->max_size = stream_send.buffer_max_size;
    s->parent_using_h2o = stream_send.parents.h2o;

    // initialize rrdpush globals
    rrdhost_flag_clear(s->host, RRDHOST_FLAG_RRDPUSH_SENDER_CONNECTED | RRDHOST_FLAG_RRDPUSH_SENDER_READY_4_METRICS);

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

    char *pipe_buffer = mallocz(pipe_buffer_size);

    bool was_connected = false;
    size_t iterations = 0;
    time_t now_s = now_monotonic_sec();
    while(!rrdhost_sender_should_exit(s)) {
        iterations++;

        // The connection attempt blocks (after which we use the socket in nonblocking)
        if(unlikely(s->sock.fd == -1)) {
            if(was_connected)
                rrdpush_sender_on_disconnect(s->host);

            was_connected = rrdpush_sender_connect(s);
            now_s = s->last_traffic_seen_t;
            continue;
        }

        if(iterations % 1000 == 0)
            now_s = now_monotonic_sec();

        // If the TCP window never opened then something is wrong, restart connection
        if(unlikely(now_s - s->last_traffic_seen_t > stream_send.parents.timeout_s &&
            !rrdpush_sender_pending_replication_requests(s) &&
            !rrdpush_sender_replicating_charts(s)
        )) {
            worker_is_busy(WORKER_SENDER_JOB_DISCONNECT_TIMEOUT);
            netdata_log_error("STREAM %s [send to %s]: could not send metrics for %ld seconds - closing connection - "
                              "we have sent %zu bytes on this connection via %zu send attempts.",
                              rrdhost_hostname(s->host), s->connected_to, stream_send.parents.timeout_s,
                              s->sent_bytes_on_this_connection, s->send_attempts);
            rrdpush_sender_thread_close_socket(s);
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
                netdata_log_error("STREAM %s [send]: cannot create inter-thread communication pipe. "
                                  "Disabling streaming.", rrdhost_hostname(s->host));
                rrdpush_sender_thread_close_socket(s);
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
                .fd = s->sock.fd,
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

        internal_error(fds[Socket].fd != s->sock.fd,
            "STREAM %s [send to %s]: socket changed after poll().", rrdhost_hostname(s->host), s->connected_to);

        // Spurious wake-ups without error - loop again
        if (poll_rc == 0 || ((poll_rc == -1) && (errno == EAGAIN || errno == EINTR))) {
            netdata_log_debug(D_STREAM, "Spurious wakeup");
            now_s = now_monotonic_sec();
            continue;
        }

        // Only errors from poll() are internal, but try restarting the connection
        if(unlikely(poll_rc == -1)) {
            worker_is_busy(WORKER_SENDER_JOB_DISCONNECT_POLL_ERROR);
            netdata_log_error("STREAM %s [send to %s]: failed to poll(). Closing socket.", rrdhost_hostname(s->host), s->connected_to);
            rrdpush_sender_pipe_close(s->host, s->rrdpush_sender_pipe, true);
            rrdpush_sender_thread_close_socket(s);
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

            if (read(fds[Collector].fd, pipe_buffer, pipe_buffer_size) == -1)
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
            rrdpush_sender_execute_commands(s);

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
                rrdpush_sender_thread_close_socket(s);
            }
        }

        // protection from overflow
        if(unlikely(s->flags & SENDER_FLAG_OVERFLOW)) {
            worker_is_busy(WORKER_SENDER_JOB_DISCONNECT_OVERFLOW);
            errno_clear();
            netdata_log_error("STREAM %s [send to %s]: buffer full (allocated %zu bytes) after sending %zu bytes. Restarting connection",
                              rrdhost_hostname(s->host), s->connected_to, s->buffer->size, s->sent_bytes_on_this_connection);
            rrdpush_sender_thread_close_socket(s);
        }

        worker_set_metric(WORKER_SENDER_JOB_REPLAY_DICT_SIZE, (NETDATA_DOUBLE) dictionary_entries(s->replication.requests));
    }

    if(was_connected)
        rrdpush_sender_on_disconnect(s->host);

    netdata_log_info("STREAM %s [send]: sending thread exits %s",
                     rrdhost_hostname(s->host),
                     s->exit.reason != STREAM_HANDSHAKE_NEVER ? stream_handshake_error_to_string(s->exit.reason) : "");

    sender_lock(s);
    {
        rrdpush_sender_thread_close_socket(s);
        rrdpush_sender_pipe_close(s->host, s->rrdpush_sender_pipe, false);
        rrdpush_sender_execute_commands_cleanup(s);

        rrdhost_clear_sender___while_having_sender_mutex(s->host);

#ifdef NETDATA_LOG_STREAM_SENDER
        if (s->stream_log_fp) {
            fclose(s->stream_log_fp);
            s->stream_log_fp = NULL;
        }
#endif
    }
    sender_unlock(s);

    freez(pipe_buffer);
    worker_unregister();

    return NULL;
}
*/

static struct {
    struct {
        ND_THREAD *thread;
        SPINLOCK spinlock;
        size_t count;
        struct sender_state *queue;
        struct completion completion;
    } connector;

    struct {
        uint64_t magic;
        pid_t tid;

        int pipe_fds[2];
        ND_THREAD *thread;
        SPINLOCK spinlock;
        size_t count;
        struct sender_state *queue;

        struct {
            size_t used;
            size_t size;
            struct pollfd *array;
            struct sender_state **running;
        } pollfd;
    } dispatcher;
} sender = {
    .dispatcher = {
        .pipe_fds = { -1, -1 },
    }
};

void stream_sender_cancel_threads(void) {
    nd_thread_signal_cancel(sender.connector.thread);
    nd_thread_signal_cancel(sender.dispatcher.thread);
}

int sender_write_pipe_fd(struct sender_state *s __maybe_unused) {
    return sender.dispatcher.pipe_fds[PIPE_WRITE];
}

uint64_t sender_magic(RRDHOST *host __maybe_unused) {
    return sender.dispatcher.magic;
}

pid_t sender_tid(RRDHOST *host __maybe_unused) {
    return sender.dispatcher.tid;
}

void stream_sender_connector_add(struct sender_state *s) {
    ND_LOG_STACK lgs[] = {
        ND_LOG_FIELD_STR(NDF_NIDL_NODE, s->host->hostname),
        ND_LOG_FIELD_CB(NDF_DST_IP, stream_sender_log_dst_ip, s),
        ND_LOG_FIELD_CB(NDF_DST_PORT, stream_sender_log_dst_port, s),
        ND_LOG_FIELD_CB(NDF_DST_TRANSPORT, stream_sender_log_transport, s),
        ND_LOG_FIELD_CB(NDF_SRC_CAPABILITIES, stream_sender_log_capabilities, s),
        ND_LOG_FIELD_END(),
    };
    ND_LOG_STACK_PUSH(lgs);

    if(!rrdhost_has_rrdpush_sender_enabled(s->host) || !s->host->stream.snd.destination || !s->host->stream.snd.api_key) {
        netdata_log_error("STREAM %s [send]: thread created (task id %d), but host has streaming disabled.",
                          rrdhost_hostname(s->host), gettid_cached());
        return;
    }

    rrdhost_stream_parent_ssl_init(s);

    s->buffer->max_size = stream_send.buffer_max_size;
    s->parent_using_h2o = stream_send.parents.h2o;

    rrdhost_flag_clear(s->host, RRDHOST_FLAG_RRDPUSH_SENDER_CONNECTED | RRDHOST_FLAG_RRDPUSH_SENDER_READY_4_METRICS);

    spinlock_lock(&sender.connector.spinlock);
    DOUBLE_LINKED_LIST_APPEND_ITEM_UNSAFE(sender.connector.queue, s, prev, next);
    sender.connector.count++;
    spinlock_unlock(&sender.connector.spinlock);

    completion_mark_complete_a_job(&sender.connector.completion);
}

static void stream_sender_dispatcher_workers(void) {
    worker_register("STREAMSND");
    worker_register_job_name(WORKER_SENDER_JOB_CONNECT, "connect");
    worker_register_job_name(WORKER_SENDER_JOB_CONNECTED, "connected");
    worker_register_job_name(WORKER_SENDER_JOB_DEQUEUE, "dequeue");
    worker_register_job_name(WORKER_SENDER_JOB_LIST, "list");
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
    worker_register_job_name(WORKER_SENDER_JOB_DISCONNECT_STOPPED, "disconnect stopped");

    worker_register_job_name(WORKER_SENDER_JOB_REPLAY_REQUEST, "replay request");
    worker_register_job_name(WORKER_SENDER_JOB_FUNCTION_REQUEST, "function");

    worker_register_job_custom_metric(WORKER_SENDER_JOB_NODES, "nodes", "nodes", WORKER_METRIC_ABSOLUTE);
    worker_register_job_custom_metric(WORKER_SENDER_JOB_BUFFER_RATIO, "used buffer ratio", "%", WORKER_METRIC_ABSOLUTE);
    worker_register_job_custom_metric(WORKER_SENDER_JOB_BYTES_RECEIVED, "bytes received", "bytes/s", WORKER_METRIC_INCREMENT);
    worker_register_job_custom_metric(WORKER_SENDER_JOB_BYTES_SENT, "bytes sent", "bytes/s", WORKER_METRIC_INCREMENT);
    worker_register_job_custom_metric(WORKER_SENDER_JOB_BYTES_COMPRESSED, "bytes compressed", "bytes/s", WORKER_METRIC_INCREMENTAL_TOTAL);
    worker_register_job_custom_metric(WORKER_SENDER_JOB_BYTES_UNCOMPRESSED, "bytes uncompressed", "bytes/s", WORKER_METRIC_INCREMENTAL_TOTAL);
    worker_register_job_custom_metric(WORKER_SENDER_JOB_BYTES_COMPRESSION_RATIO, "cumulative compression savings ratio", "%", WORKER_METRIC_ABSOLUTE);
    worker_register_job_custom_metric(WORKER_SENDER_JOB_REPLAY_DICT_SIZE, "replication dict entries", "entries", WORKER_METRIC_ABSOLUTE);
}

void *stream_sender_connector_thread(void *ptr __maybe_unused) {
    // stream_sender_dispatcher_workers();

    unsigned job_id = 0;

    while(!nd_thread_signaled_to_cancel() && service_running(SERVICE_STREAMING)) {
        worker_is_idle();
        job_id = completion_wait_for_a_job_with_timeout(&sender.connector.completion, job_id, 1);

        spinlock_lock(&sender.connector.spinlock);
        struct sender_state *next;
        for(struct sender_state *s = sender.connector.queue; s ; s = next) {
            next = s->next;

            ND_LOG_STACK lgs[] = {
                ND_LOG_FIELD_STR(NDF_NIDL_NODE, s->host->hostname),
                ND_LOG_FIELD_CB(NDF_DST_IP, stream_sender_log_dst_ip, s),
                ND_LOG_FIELD_CB(NDF_DST_PORT, stream_sender_log_dst_port, s),
                ND_LOG_FIELD_CB(NDF_DST_TRANSPORT, stream_sender_log_transport, s),
                ND_LOG_FIELD_CB(NDF_SRC_CAPABILITIES, stream_sender_log_capabilities, s),
                ND_LOG_FIELD_END(),
            };
            ND_LOG_STACK_PUSH(lgs);

            if(rrdhost_is_sender_stopped(s)) {
                DOUBLE_LINKED_LIST_REMOVE_ITEM_UNSAFE(sender.connector.queue, s, prev, next);
                rrdhost_clear_sender(s->host, false);
                continue;
            }

            spinlock_unlock(&sender.connector.spinlock);
            // worker_is_busy(WORKER_SENDER_JOB_CONNECT);
            bool move_to_dispatcher = rrdpush_sender_connect(s);
            spinlock_lock(&sender.connector.spinlock);

            if(move_to_dispatcher) {
                // worker_is_busy(WORKER_SENDER_JOB_CONNECTED);
                DOUBLE_LINKED_LIST_REMOVE_ITEM_UNSAFE(sender.connector.queue, s, prev, next);
                spinlock_unlock(&sender.connector.spinlock);

                spinlock_lock(&sender.dispatcher.spinlock);
                DOUBLE_LINKED_LIST_APPEND_ITEM_UNSAFE(sender.dispatcher.queue, s, prev, next);
                spinlock_unlock(&sender.dispatcher.spinlock);

                spinlock_lock(&sender.connector.spinlock);
            }

            // worker_is_idle();
        }
        spinlock_unlock(&sender.connector.spinlock);
    }

    return NULL;
}

static void stream_sender_dispatcher_realloc_arrays_unsafe(size_t slot) {
    if(slot >= sender.dispatcher.pollfd.size) {
        size_t new_size = sender.dispatcher.pollfd.size > 0 ? sender.dispatcher.pollfd.size * 2 : 8;
        sender.dispatcher.pollfd.array = reallocz(sender.dispatcher.pollfd.array, new_size * sizeof(*sender.dispatcher.pollfd.array));
        sender.dispatcher.pollfd.running = reallocz(sender.dispatcher.pollfd.running, new_size * sizeof(*sender.dispatcher.pollfd.running));
        sender.dispatcher.pollfd.size = new_size;
        sender.dispatcher.pollfd.used = slot + 1;
    }
    else if(slot >= sender.dispatcher.pollfd.used)
        sender.dispatcher.pollfd.used = slot + 1;
}

static void stream_sender_dispatcher_move_queue_to_running(void) {
    size_t first_slot = 1;

    // process the queue
    spinlock_lock(&sender.dispatcher.spinlock);
    stream_sender_dispatcher_realloc_arrays_unsafe(0); // our pipe
    while(sender.dispatcher.queue) {
        worker_is_busy(WORKER_SENDER_JOB_DEQUEUE);

        struct sender_state *s = sender.dispatcher.queue;
        DOUBLE_LINKED_LIST_REMOVE_ITEM_UNSAFE(sender.dispatcher.queue, s, prev, next);

        // slot 0 is our pipe
        size_t slot = sender.dispatcher.pollfd.used > 0 ? sender.dispatcher.pollfd.used : 1;

        // find an empty slot
        for(size_t i = first_slot; i < slot && i < sender.dispatcher.pollfd.used ;i++) {
            if(!sender.dispatcher.pollfd.running[i]) {
                slot = i;
                break;
            }
        }

        stream_sender_dispatcher_realloc_arrays_unsafe(slot);

        sender.dispatcher.pollfd.running[slot] = s;
        sender.dispatcher.pollfd.array[slot] = (struct pollfd){
            .fd = s->sock.fd,
            .events = POLLIN,
            .revents = 0,
        };

        first_slot = slot + 1;
    }
    spinlock_unlock(&sender.dispatcher.spinlock);
}

static void stream_sender_dispatcher_move_running_to_connector(size_t slot) {
    sender.dispatcher.pollfd.array[slot] = (struct pollfd) {
        .fd = -1,
        .events = 0,
        .revents = 0,
    };

    struct sender_state *s = sender.dispatcher.pollfd.running[slot];
    if(!s) return;

    sender.dispatcher.pollfd.running[slot] = NULL;

    sender_lock(s);
    {
        rrdpush_sender_thread_close_socket(s);
        rrdpush_sender_on_disconnect(s->host);
        rrdpush_sender_execute_commands_cleanup(s);
    }
    sender_unlock(s);

    if (rrdhost_is_sender_stopped(s))
        rrdhost_clear_sender(s->host, false);
    else
        stream_sender_connector_add(s);
}

void *stream_sender_dispacther_thread(void *ptr __maybe_unused) {
    stream_sender_dispatcher_workers();

    sender.dispatcher.tid = gettid_cached();

    size_t pipe_buffer_size = 10 * 1024;
    char *pipe_buffer = mallocz(pipe_buffer_size);
    if(pipe(sender.dispatcher.pipe_fds) != 0) {
        netdata_log_error("STREAM dispatcher: cannot create required pipe.");
        sender.dispatcher.pipe_fds[PIPE_READ] = -1;
        sender.dispatcher.pipe_fds[PIPE_WRITE] = -1;
        return NULL;
    }

    while(!nd_thread_signaled_to_cancel() && service_running(SERVICE_STREAMING)) {
        stream_sender_dispatcher_move_queue_to_running();

        time_t now_s = now_monotonic_sec();

        size_t bytes_uncompressed = 0;
        size_t bytes_compressed = 0;
        NETDATA_DOUBLE buffer_ratio = 0.0;

        size_t nodes = 0;
        for(size_t slot = 1; slot < sender.dispatcher.pollfd.used ; slot++) {
            struct sender_state *s = sender.dispatcher.pollfd.running[slot];
            if(!s) continue;

            worker_is_busy(WORKER_SENDER_JOB_LIST);
            nodes++;

            ND_LOG_STACK lgs[] = {
                ND_LOG_FIELD_STR(NDF_NIDL_NODE, s->host->hostname),
                ND_LOG_FIELD_CB(NDF_DST_IP, stream_sender_log_dst_ip, s),
                ND_LOG_FIELD_CB(NDF_DST_PORT, stream_sender_log_dst_port, s),
                ND_LOG_FIELD_CB(NDF_DST_TRANSPORT, stream_sender_log_transport, s),
                ND_LOG_FIELD_CB(NDF_SRC_CAPABILITIES, stream_sender_log_capabilities, s),
                ND_LOG_FIELD_END(),
            };
            ND_LOG_STACK_PUSH(lgs);

            if(s->sock.fd == -1 || rrdhost_is_sender_stopped(s) || rrdhost_sender_should_exit(s)) {
                worker_is_busy(WORKER_SENDER_JOB_DISCONNECT_STOPPED);
                stream_sender_dispatcher_move_running_to_connector(slot);
                continue;
            }

            // If the TCP window never opened then something is wrong, restart connection
            if(unlikely(now_s - s->last_traffic_seen_t > stream_send.parents.timeout_s &&
                         !rrdpush_sender_pending_replication_requests(s) &&
                         !rrdpush_sender_replicating_charts(s)
                             )) {
                worker_is_busy(WORKER_SENDER_JOB_DISCONNECT_TIMEOUT);
                netdata_log_error("STREAM %s [send to %s]: could not send metrics for %ld seconds - closing connection - "
                                  "we have sent %zu bytes on this connection via %zu send attempts.",
                                  rrdhost_hostname(s->host), s->connected_to, stream_send.parents.timeout_s,
                                  s->sent_bytes_on_this_connection, s->send_attempts);
                stream_sender_dispatcher_move_running_to_connector(slot);
                continue;
            }

            sender_lock(s);
            size_t outstanding = cbuffer_next_unsafe(s->buffer, NULL);
            size_t available = cbuffer_available_size_unsafe(s->buffer);
            if (unlikely(!outstanding)) {
                rrdpush_sender_pipe_clear_pending_data(s);
                rrdpush_sender_cbuffer_recreate_timed(s, now_s, true, false);
            }
            if(s->buffer->max_size) {
                NETDATA_DOUBLE ratio = (NETDATA_DOUBLE)(s->buffer->max_size - available) * 100.0 / (NETDATA_DOUBLE)s->buffer->max_size;
                if (ratio > buffer_ratio) buffer_ratio = ratio;
            }

            if(s->compressor.initialized) {
                bytes_uncompressed += s->compressor.sender_locked.total_uncompressed;
                bytes_compressed += s->compressor.sender_locked.total_compressed + s->compressor.sender_locked.total_compressions * sizeof(rrdpush_signature_t);
            }
            sender_unlock(s);

            if(outstanding) {
                sender.dispatcher.pollfd.array[slot].events = POLLIN | POLLOUT;
                s->send_attempts++;
            }
            else
                sender.dispatcher.pollfd.array[slot].events = POLLIN;

            sender.dispatcher.pollfd.array[slot].fd = s->sock.fd;
            sender.dispatcher.pollfd.array[slot].revents = 0;
        }

        if(bytes_compressed && bytes_uncompressed) {
            NETDATA_DOUBLE compression_ratio = 100.0 - ((NETDATA_DOUBLE)bytes_compressed * 100.0 / (NETDATA_DOUBLE)bytes_uncompressed);
            worker_set_metric(WORKER_SENDER_JOB_BYTES_COMPRESSION_RATIO, compression_ratio);
        }

        worker_set_metric(WORKER_SENDER_JOB_NODES, (NETDATA_DOUBLE)nodes);
        worker_set_metric(WORKER_SENDER_JOB_BYTES_UNCOMPRESSED, (NETDATA_DOUBLE)bytes_uncompressed);
        worker_set_metric(WORKER_SENDER_JOB_BYTES_COMPRESSED, (NETDATA_DOUBLE)bytes_compressed);
        worker_set_metric(WORKER_SENDER_JOB_BUFFER_RATIO, buffer_ratio);

        worker_is_idle();

        sender.dispatcher.pollfd.array[0] = (struct pollfd) {
            .fd = sender.dispatcher.pipe_fds[PIPE_READ],
            .events = POLLIN,
            .revents = 0,
        };

        // timeout in milliseconds
        int poll_rc = poll(sender.dispatcher.pollfd.array, sender.dispatcher.pollfd.used, 50);

        // Spurious wake-ups without error - loop again
        if (poll_rc == 0 || ((poll_rc == -1) && (errno == EAGAIN || errno == EINTR)))
            continue;

        if(unlikely(poll_rc == -1)) {
            worker_is_busy(WORKER_SENDER_JOB_DISCONNECT_POLL_ERROR);
            nd_log_limit_static_thread_var(erl, 1, 1 * USEC_PER_MS);
            nd_log_limit(&erl, NDLS_DAEMON, NDLP_ERR, "poll() returned error");
            continue;
        }

        now_s = now_monotonic_sec();

        // If the collector woke us up then empty the pipe to remove the signal
        if(sender.dispatcher.pollfd.array[0].revents) {
            short revents = sender.dispatcher.pollfd.array[0].revents;

            if (revents & (POLLIN | POLLPRI)) {
                worker_is_busy(WORKER_SENDER_JOB_PIPE_READ);
                if (read(sender.dispatcher.pollfd.array[0].fd, pipe_buffer, pipe_buffer_size) == -1)
                    netdata_log_error("STREAM dispatcher: cannot read from internal pipe.");
            }
            else if(revents & (POLLERR|POLLHUP|POLLNVAL)) {
                char *error = NULL;

                if (revents & POLLERR)
                    error = "pipe reports errors (POLLERR)";
                else if (revents & POLLHUP)
                    error = "pipe closed (POLLHUP)";
                else if (revents & POLLNVAL)
                    error = "pipe is invalid (POLLNVAL)";

                if(error) {
                    close(sender.dispatcher.pipe_fds[PIPE_READ]); sender.dispatcher.pipe_fds[PIPE_READ] = -1;
                    close(sender.dispatcher.pipe_fds[PIPE_WRITE]); sender.dispatcher.pipe_fds[PIPE_WRITE] = -1;
                    if(pipe(sender.dispatcher.pipe_fds) != 0)
                        netdata_log_error("STREAM dispatcher: cannot create required pipe.");
                    else
                        netdata_log_error("STREAM dispatcher: restarted internal pipe.");
                }
            }
        }

        size_t replay_entries = 0;
        size_t bytes_received = 0;
        size_t bytes_sent = 0;

        for(size_t slot = 1; slot < sender.dispatcher.pollfd.used ; slot++) {
            struct sender_state *s = sender.dispatcher.pollfd.running[slot];
            if(!s || !sender.dispatcher.pollfd.array[slot].revents)
                continue;

            ND_LOG_STACK lgs[] = {
                ND_LOG_FIELD_STR(NDF_NIDL_NODE, s->host->hostname),
                ND_LOG_FIELD_CB(NDF_DST_IP, stream_sender_log_dst_ip, s),
                ND_LOG_FIELD_CB(NDF_DST_PORT, stream_sender_log_dst_port, s),
                ND_LOG_FIELD_CB(NDF_DST_TRANSPORT, stream_sender_log_transport, s),
                ND_LOG_FIELD_CB(NDF_SRC_CAPABILITIES, stream_sender_log_capabilities, s),
                ND_LOG_FIELD_END(),
            };
            ND_LOG_STACK_PUSH(lgs);

            short revents = sender.dispatcher.pollfd.array[slot].revents;

            if(unlikely(revents & (POLLERR|POLLHUP|POLLNVAL))) {
                char *error = NULL;

                if (revents & POLLERR)
                    error = "socket reports errors (POLLERR)";
                else if (revents & POLLHUP)
                    error = "connection closed by remote end (POLLHUP)";
                else if (revents & POLLNVAL)
                    error = "connection is invalid (POLLNVAL)";

                if(error) {
                    worker_is_busy(WORKER_SENDER_JOB_DISCONNECT_SOCKET_ERROR);
                    netdata_log_error("STREAM %s [send to %s]: restarting connection: %s - %zu bytes transmitted.",
                                      rrdhost_hostname(s->host), s->connected_to, error, s->sent_bytes_on_this_connection);

                    stream_sender_dispatcher_move_running_to_connector(slot);
                    continue;
                }
            }

            if(revents & POLLOUT) {
                worker_is_busy(WORKER_SENDER_JOB_SOCKET_SEND);
                ssize_t bytes = attempt_to_send(s);
                if(bytes > 0) {
                    s->last_traffic_seen_t = now_s;
                    bytes_sent += bytes;
                }
            }

            if(revents & POLLIN) {
                worker_is_busy(WORKER_SENDER_JOB_SOCKET_RECEIVE);
                ssize_t bytes = attempt_read(s);
                if(bytes > 0) {
                    s->last_traffic_seen_t = now_s;
                    bytes_received += bytes;
                }
            }

            if(unlikely(s->read_len)) {
                worker_is_busy(WORKER_SENDER_JOB_EXECUTE);
                rrdpush_sender_execute_commands(s);
            }

            replay_entries += dictionary_entries(s->replication.requests);
        }

        worker_set_metric(WORKER_SENDER_JOB_BYTES_RECEIVED, (NETDATA_DOUBLE)bytes_received);
        worker_set_metric(WORKER_SENDER_JOB_BYTES_SENT, (NETDATA_DOUBLE)bytes_sent);
        worker_set_metric(WORKER_SENDER_JOB_REPLAY_DICT_SIZE, (NETDATA_DOUBLE)replay_entries);
    }

    // dequeue
    while(sender.dispatcher.queue)
        stream_sender_dispatcher_move_queue_to_running();

    // stop all hosts
    for(size_t slot = 0; slot < sender.dispatcher.pollfd.used ;slot++) {
        struct sender_state *s = sender.dispatcher.pollfd.running[slot];
        if(!s) continue;

        __atomic_store_n(&s->stop, true, __ATOMIC_RELAXED);
        stream_sender_dispatcher_move_running_to_connector(slot);
    }

    // cleanup
    freez(pipe_buffer);
    close(sender.dispatcher.pipe_fds[PIPE_READ]); sender.dispatcher.pipe_fds[PIPE_READ] = -1;
    close(sender.dispatcher.pipe_fds[PIPE_WRITE]); sender.dispatcher.pipe_fds[PIPE_WRITE] = -1;

    return NULL;
}

void rrdpush_sender_thread_spawn(RRDHOST *host) {
    static SPINLOCK spinlock = NETDATA_SPINLOCK_INITIALIZER;
    static bool dispatcher_running = false;
    static bool connector_running = false;

    spinlock_lock(&spinlock);
    sender_lock(host->sender);

    if(!rrdhost_flag_check(host, RRDHOST_FLAG_RRDPUSH_SENDER_SPAWN)) {
        sender.dispatcher.magic = 1 + os_random64();

        if(!dispatcher_running) {
            int id = 0;
            char tag[NETDATA_THREAD_TAG_MAX + 1];
            snprintfz(tag, NETDATA_THREAD_TAG_MAX, THREAD_TAG_STREAM_SENDER "-DP" "[%d]", id);

            sender.dispatcher.thread = nd_thread_create(tag, NETDATA_THREAD_OPTION_DEFAULT, stream_sender_dispacther_thread, &id);
            if (!sender.dispatcher.thread)
                nd_log_daemon(NDLP_ERR, "STREAM %s [send]: failed to create new thread for client.", rrdhost_hostname(host));
            else
                dispatcher_running = true;
        }

        if(!connector_running) {
            int id = 0;
            char tag[NETDATA_THREAD_TAG_MAX + 1];
            snprintfz(tag, NETDATA_THREAD_TAG_MAX, THREAD_TAG_STREAM_SENDER "-CN" "[%d]", id);

            sender.connector.thread = nd_thread_create(tag, NETDATA_THREAD_OPTION_DEFAULT, stream_sender_connector_thread, &id);
            if (!sender.connector.thread)
                nd_log_daemon(NDLP_ERR, "STREAM %s [send]: failed to create new thread for client.", rrdhost_hostname(host));
            else
                connector_running = true;
        }
    }

    sender_unlock(host->sender);

    if(dispatcher_running && connector_running) {
        rrdhost_flag_set(host, RRDHOST_FLAG_RRDPUSH_SENDER_SPAWN);

        if(!rrdhost_set_sender(host)) {
            netdata_log_error("STREAM %s [send]: sender is already occupied.", rrdhost_hostname(host));
            return;
        }

        stream_sender_connector_add(host->sender);
    }

    spinlock_unlock(&spinlock);
}

void *rrdpush_sender_thread(void *ptr __maybe_unused) {
    if(!localhost) return NULL;

    rrdpush_sender_thread_spawn(localhost);
    return NULL;
}
