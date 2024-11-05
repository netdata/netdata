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

    if(SSL_connection(&s->ssl))
        ret = netdata_ssl_write(&s->ssl, chunk, outstanding);
    else
        ret = send(s->rrdpush_sender_socket, chunk, outstanding, MSG_DONTWAIT);

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
    ssize_t ret;

    if (SSL_connection(&s->ssl))
        ret = netdata_ssl_read(&s->ssl, s->read_buffer + s->read_len, sizeof(s->read_buffer) - s->read_len - 1);
    else
        ret = recv(s->rrdpush_sender_socket, s->read_buffer + s->read_len, sizeof(s->read_buffer) - s->read_len - 1,MSG_DONTWAIT);

    if (ret > 0) {
        s->read_len += ret;
        return ret;
    }

    if (ret < 0 && (errno == EAGAIN || errno == EWOULDBLOCK || errno == EINTR))
        return ret;

    if (SSL_connection(&s->ssl))
        worker_is_busy(WORKER_SENDER_JOB_DISCONNECT_SSL_ERROR);
    else if (ret == 0 || errno == ECONNRESET) {
        worker_is_busy(WORKER_SENDER_JOB_DISCONNECT_PARENT_CLOSED);
        netdata_log_error("STREAM %s [send to %s]: connection closed by far end.", rrdhost_hostname(s->host), s->connected_to);
    }
    else {
        worker_is_busy(WORKER_SENDER_JOB_DISCONNECT_RECEIVE_ERROR);
        netdata_log_error("STREAM %s [send to %s]: error during receive (%zd) - closing connection.", rrdhost_hostname(s->host), s->connected_to, ret);
    }

    rrdpush_sender_thread_close_socket(s);

    return ret;
}

static bool rrdpush_sender_pipe_close(RRDHOST *host, int *pipe_fds, bool reopen) {
    static netdata_mutex_t mutex = NETDATA_MUTEX_INITIALIZER;

    bool ret = true;

    netdata_mutex_lock(&mutex);

    int new_pipe_fds[2];
    if(reopen) {
        if(pipe(new_pipe_fds) != 0) {
            netdata_log_error("STREAM %s [send]: cannot create required pipe.", rrdhost_hostname(host));
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
    if(unlikely(s->tid == gettid_cached()))
        return;

    RRDHOST *host = s->host;

    int pipe_fd = s->rrdpush_sender_pipe[PIPE_WRITE];

    // signal the sender there are more data
    if (pipe_fd != -1 && write(pipe_fd, " ", 1) == -1) {
        netdata_log_error("STREAM %s [send]: cannot write to internal pipe.", rrdhost_hostname(host));
        rrdpush_sender_pipe_close(host, s->rrdpush_sender_pipe, true);
    }
}

static bool rrdhost_set_sender(RRDHOST *host) {
    if(unlikely(!host->sender)) return false;

    bool ret = false;
    sender_lock(host->sender);
    if(!host->sender->tid) {
        rrdhost_flag_clear(host, RRDHOST_FLAG_RRDPUSH_SENDER_CONNECTED | RRDHOST_FLAG_RRDPUSH_SENDER_READY_4_METRICS);
        rrdhost_flag_set(host, RRDHOST_FLAG_RRDPUSH_SENDER_SPAWN);
        host->rrdpush_sender_connection_counter++;
        host->sender->tid = gettid_cached();
        host->sender->last_state_since_t = now_realtime_sec();
        host->sender->exit.reason = STREAM_HANDSHAKE_NEVER;
        ret = true;
    }
    sender_unlock(host->sender);

    rrdpush_reset_destinations_postpone_time(host);

    return ret;
}

static void rrdhost_clear_sender___while_having_sender_mutex(RRDHOST *host) {
    if(unlikely(!host->sender)) return;

    if(host->sender->tid == gettid_cached()) {
        host->sender->tid = 0;
        host->sender->exit.shutdown = false;
        rrdhost_flag_clear(host, RRDHOST_FLAG_RRDPUSH_SENDER_SPAWN | RRDHOST_FLAG_RRDPUSH_SENDER_CONNECTED | RRDHOST_FLAG_RRDPUSH_SENDER_READY_4_METRICS);
        host->sender->last_state_since_t = now_realtime_sec();
        rrdpush_destination_set_disconnect_reason(host->destination, host->sender->exit.reason, host->sender->last_state_since_t);
    }

    rrdpush_reset_destinations_postpone_time(host);
}

bool rrdhost_sender_should_exit(struct sender_state *s) {
    if(unlikely(nd_thread_signaled_to_cancel())) {
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

    buffer_strcat(wb, SSL_connection(&state->ssl) ? "https" : "http");
    return true;
}

static bool stream_sender_log_dst_ip(BUFFER *wb, void *ptr) {
    struct sender_state *state = ptr;
    if(!state || state->rrdpush_sender_socket == -1)
        return false;

    SOCKET_PEERS peers = socket_peers(state->rrdpush_sender_socket);
    buffer_strcat(wb, peers.peer.ip);
    return true;
}

static bool stream_sender_log_dst_port(BUFFER *wb, void *ptr) {
    struct sender_state *state = ptr;
    if(!state || state->rrdpush_sender_socket == -1)
        return false;

    SOCKET_PEERS peers = socket_peers(state->rrdpush_sender_socket);
    buffer_print_uint64(wb, peers.peer.port);
    return true;
}

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

    if(!rrdhost_has_rrdpush_sender_enabled(s->host) || !s->host->rrdpush.send.destination ||
       !*s->host->rrdpush.send.destination || !s->host->rrdpush.send.api_key ||
       !*s->host->rrdpush.send.api_key) {
        netdata_log_error("STREAM %s [send]: thread created (task id %d), but host has streaming disabled.",
              rrdhost_hostname(s->host), gettid_cached());
        return NULL;
    }

    if(!rrdhost_set_sender(s->host)) {
        netdata_log_error("STREAM %s [send]: thread created (task id %d), but there is another sender running for this host.",
              rrdhost_hostname(s->host), gettid_cached());
        return NULL;
    }

    rrdpush_sender_ssl_init(s->host);

    netdata_log_info("STREAM %s [send]: thread created (task id %d)", rrdhost_hostname(s->host), gettid_cached());

    s->timeout = (int)appconfig_get_duration_seconds(
        &stream_config, CONFIG_SECTION_STREAM, "timeout", 600);

    s->default_port = (int)appconfig_get_number(
        &stream_config, CONFIG_SECTION_STREAM, "default port", 19999);

    s->buffer->max_size = (size_t)appconfig_get_number(
        &stream_config, CONFIG_SECTION_STREAM, "buffer size bytes", 1024 * 1024 * 10);

    s->reconnect_delay = (unsigned int)appconfig_get_duration_seconds(
        &stream_config, CONFIG_SECTION_STREAM, "reconnect delay", 5);

    stream_conf_initial_clock_resync_iterations = (unsigned int)appconfig_get_number(
        &stream_config, CONFIG_SECTION_STREAM,
        "initial clock resync iterations",
        stream_conf_initial_clock_resync_iterations); // TODO: REMOVE FOR SLEW / GAPFILLING

    s->parent_using_h2o = appconfig_get_boolean(
        &stream_config, CONFIG_SECTION_STREAM, "parent using h2o", false);

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
        if(unlikely(s->rrdpush_sender_socket == -1)) {
            if(was_connected)
                rrdpush_sender_on_disconnect(s->host);

            was_connected = rrdpush_sender_connect(s);
            now_s = s->last_traffic_seen_t;
            continue;
        }

        if(iterations % 1000 == 0)
            now_s = now_monotonic_sec();

        // If the TCP window never opened then something is wrong, restart connection
        if(unlikely(now_s - s->last_traffic_seen_t > s->timeout &&
            !rrdpush_sender_pending_replication_requests(s) &&
            !rrdpush_sender_replicating_charts(s)
        )) {
            worker_is_busy(WORKER_SENDER_JOB_DISCONNECT_TIMEOUT);
            netdata_log_error("STREAM %s [send to %s]: could not send metrics for %d seconds - closing connection - we have sent %zu bytes on this connection via %zu send attempts.", rrdhost_hostname(s->host), s->connected_to, s->timeout, s->sent_bytes_on_this_connection, s->send_attempts);
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
                .fd = s->rrdpush_sender_socket,
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

        internal_error(fds[Socket].fd != s->rrdpush_sender_socket,
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

void rrdpush_sender_thread_spawn(RRDHOST *host) {
    sender_lock(host->sender);

    if(!rrdhost_flag_check(host, RRDHOST_FLAG_RRDPUSH_SENDER_SPAWN)) {
        char tag[NETDATA_THREAD_TAG_MAX + 1];
        snprintfz(tag, NETDATA_THREAD_TAG_MAX, THREAD_TAG_STREAM_SENDER "[%s]", rrdhost_hostname(host));

        host->rrdpush_sender_thread = nd_thread_create(tag, NETDATA_THREAD_OPTION_DEFAULT,
                                                       rrdpush_sender_thread, (void *)host->sender);
        if(!host->rrdpush_sender_thread)
            nd_log_daemon(NDLP_ERR, "STREAM %s [send]: failed to create new thread for client.", rrdhost_hostname(host));
        else
            rrdhost_flag_set(host, RRDHOST_FLAG_RRDPUSH_SENDER_SPAWN);
    }

    sender_unlock(host->sender);
}
