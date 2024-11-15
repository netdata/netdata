// SPDX-License-Identifier: GPL-3.0-or-later

#include "sender-internals.h"

static void stream_sender_cbuffer_recreate_timed(struct sender_state *s, time_t now_s, bool have_mutex, bool force) {
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
    stream_sender_cbuffer_recreate_timed(host->sender, now_monotonic_sec(), true, true);

    sender_unlock(host->sender);
}

static void rrdpush_sender_charts_and_replication_reset(struct sender_state *s) {
    rrdpush_sender_set_flush_time(s);

    // stop all replication commands inflight
    replication_sender_delete_pending_requests(s);

    // reset the state of all charts
    RRDSET *st;
    rrdset_foreach_read(st, s->host) {
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

    rrdhost_sender_replicating_charts_zero(s->host);
    rrdpush_sender_replicating_charts_zero(s);
}

void stream_sender_on_connect(struct sender_state *s) {
    rrdhost_flag_set(s->host, RRDHOST_FLAG_RRDPUSH_SENDER_CONNECTED);

    rrdpush_sender_charts_and_replication_reset(s);
    rrdpush_sender_cbuffer_flush(s->host);

    s->last_traffic_seen_t = now_monotonic_sec();
    s->flags &= ~SENDER_FLAG_OVERFLOW;
    s->read_len = 0;
    s->buffer->read = 0;
    s->buffer->write = 0;
    s->send_attempts = 0;
}

static void stream_sender_on_ready_to_dispatch(struct sender_state *s) {
    rrdhost_flag_set(s->host, RRDHOST_FLAG_RRDPUSH_SENDER_READY_4_METRICS);

    rrdpush_sender_execute_commands_cleanup(s);
    rrdpush_sender_thread_send_custom_host_variables(s->host);
    stream_path_send_to_parent(s->host);
    rrdpush_sender_send_claimed_id(s->host);
    rrdpush_send_host_labels(s->host);
    rrdpush_send_global_functions(s->host);
}

static void stream_sender_on_disconnect(struct sender_state *s) {
    rrdhost_flag_clear(s->host, RRDHOST_FLAG_RRDPUSH_SENDER_CONNECTED | RRDHOST_FLAG_RRDPUSH_SENDER_READY_4_METRICS);

    nd_sock_close(&s->sock);
    rrdpush_sender_execute_commands_cleanup(s);
    rrdpush_sender_charts_and_replication_reset(s);
    rrdpush_sender_clear_parent_claim_id(s->host);
    rrdpush_receiver_send_node_and_claim_id_to_child(s->host);
    stream_path_parent_disconnected(s->host);
}

bool stream_sender_is_signaled_to_stop(struct sender_state *s) {
    return __atomic_load_n(&s->exit.shutdown, __ATOMIC_RELAXED);
}

bool stream_sender_is_host_stopped(struct sender_state *s) {
    if(unlikely(stream_sender_is_signaled_to_stop(s))) {
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

// --------------------------------------------------------------------------------------------------------------------

#define MAX_DISPATCHERS 1

struct dispatcher {
    int id;
    int pipe_fds[2];
    ND_THREAD *thread;

    struct {
        // the incoming queue of the dispatcher thread
        // the connector thread leaves the connected senders in this list, for the dispatcher to pick them up
        SPINLOCK spinlock;
        struct sender_state *ll;
    } queue;

    struct {
        // private fields for the dispatcher thread only - DO NOT USE ON OTHER THREADS
        size_t used;
        size_t size;
        struct pollfd *array;
        struct sender_state **running;
    } pollfd;
};

static struct {
    struct dispatcher dispatcher[MAX_DISPATCHERS];
} dispatcher_globals = { 0 };

void stream_sender_cancel_threads(void) {
    stream_sender_connector_cancel_threads();

    for(int id = 0; id < MAX_DISPATCHERS ;id++)
        nd_thread_signal_cancel(dispatcher_globals.dispatcher[id].thread);
}

static struct dispatcher *stream_sender_dispatcher(struct sender_state *s) {
    if(s->dispatcher.id < 0 || s->dispatcher.id >= MAX_DISPATCHERS)
        s->dispatcher.id = 0;

    return &dispatcher_globals.dispatcher[s->dispatcher.id];
}

static void stream_sender_update_dispatcher_reset_unsafe(struct sender_state *s) {
    s->sent_bytes_on_this_connection = 0;
    s->dispatcher.bytes_uncompressed = 0;
    s->dispatcher.bytes_compressed = 0;
    s->dispatcher.bytes_outstanding = 0;
    s->dispatcher.bytes_available = 0;
    s->dispatcher.buffer_ratio = 0.0;
    replication_recalculate_buffer_used_ratio_unsafe(s);
}

static void stream_sender_update_dispatcher_sent_data_unsafe(struct sender_state *s, uint64_t bytes_sent) {
    s->sent_bytes_on_this_connection += bytes_sent;
    s->dispatcher.bytes_outstanding = cbuffer_next_unsafe(s->buffer, NULL);
    s->dispatcher.bytes_available = cbuffer_available_size_unsafe(s->buffer);
    s->dispatcher.buffer_ratio = (NETDATA_DOUBLE)(s->buffer->max_size -  s->dispatcher.bytes_available) * 100.0 / (NETDATA_DOUBLE)s->buffer->max_size;
    replication_recalculate_buffer_used_ratio_unsafe(s);
}

void stream_sender_update_dispatcher_added_data_unsafe(struct sender_state *s, uint64_t bytes_compressed, uint64_t bytes_uncompressed) {
    // calculate the statistics for our dispatcher
    s->dispatcher.bytes_uncompressed += bytes_uncompressed;
    s->dispatcher.bytes_compressed += bytes_compressed;
    s->dispatcher.bytes_outstanding = cbuffer_next_unsafe(s->buffer, NULL);
    s->dispatcher.bytes_available = cbuffer_available_size_unsafe(s->buffer);
    s->dispatcher.buffer_ratio = (NETDATA_DOUBLE)(s->buffer->max_size -  s->dispatcher.bytes_available) * 100.0 / (NETDATA_DOUBLE)s->buffer->max_size;
    replication_recalculate_buffer_used_ratio_unsafe(s);
}

void stream_sender_reconnect(struct sender_state *s) {
    struct pipe_msg msg = s->dispatcher.pollfd;
    msg.msg = SENDER_MSG_RECONNECT;
    stream_sender_send_msg_to_dispatcher(s, msg);
}

void stream_sender_send_msg_to_dispatcher(struct sender_state *s, struct pipe_msg msg) {
    if(!msg.slot || !msg.magic) return;

    int pipe_fd = sender_write_pipe_fd(s);
    if (pipe_fd != -1 && write(pipe_fd, &msg, sizeof(msg)) != sizeof(msg))
        netdata_log_error("STREAM %s [send]: cannot write to internal pipe.",
                          rrdhost_hostname(s->host));
}

int sender_write_pipe_fd(struct sender_state *s) {
    struct dispatcher *dp = stream_sender_dispatcher(s);
    return dp->pipe_fds[PIPE_WRITE];
}

static void stream_sender_connector_add_unlinked(struct sender_state *s) {
    ND_LOG_STACK lgs[] = {
        ND_LOG_FIELD_STR(NDF_NIDL_NODE, s->host->hostname),
        ND_LOG_FIELD_END(),
    };
    ND_LOG_STACK_PUSH(lgs);

    // multiple threads may come here - only one should be able to pass through
    sender_lock(s);
    if(!rrdhost_has_rrdpush_sender_enabled(s->host) || !s->host->stream.snd.destination || !s->host->stream.snd.api_key) {
        nd_log(NDLS_DAEMON, NDLP_ERR, "STREAM %s [send]: host has streaming disabled - not sending data to a parent.",
                          rrdhost_hostname(s->host));
        sender_unlock(s);
        return;
    }
    if(rrdhost_flag_check(s->host, RRDHOST_FLAG_RRDPUSH_SENDER_ADDED)) {
        sender_unlock(s);
        return;
    }
    rrdhost_flag_set(s->host, RRDHOST_FLAG_RRDPUSH_SENDER_ADDED);
    rrdhost_flag_clear(s->host, RRDHOST_FLAG_RRDPUSH_SENDER_CONNECTED | RRDHOST_FLAG_RRDPUSH_SENDER_READY_4_METRICS);
    sender_unlock(s);

    nd_log(NDLS_DAEMON, NDLP_DEBUG, "STREAM connector [%s]: adding host", rrdhost_hostname(s->host));

    nd_sock_close(&s->sock);
    s->buffer->max_size = stream_send.buffer_max_size;
    s->parent_using_h2o = stream_send.parents.h2o;

    // do not have the dispatcher lock when calling this
    stream_sender_connector_add_to_queue(s);
}

static void stream_sender_dispatcher_realloc_arrays_unsafe(struct dispatcher *dp, size_t slot) {
    if(slot >= dp->pollfd.size) {
        size_t new_size = dp->pollfd.size > 0 ? dp->pollfd.size * 2 : 8;
        dp->pollfd.array = reallocz(dp->pollfd.array, new_size * sizeof(*dp->pollfd.array));
        dp->pollfd.running = reallocz(dp->pollfd.running, new_size * sizeof(*dp->pollfd.running));
        dp->pollfd.size = new_size;
        dp->pollfd.used = slot + 1;
    }
    else if(slot >= dp->pollfd.used)
        dp->pollfd.used = slot + 1;
}

void stream_sender_dispatcher_add_to_queue(struct sender_state *s) {
    struct dispatcher *dp = stream_sender_dispatcher(s);

    spinlock_lock(&dp->queue.spinlock);
    DOUBLE_LINKED_LIST_APPEND_ITEM_UNSAFE(dp->queue.ll, s, prev, next);
    spinlock_unlock(&dp->queue.spinlock);
}

static void stream_sender_dispatcher_move_queue_to_running(struct dispatcher *dp) {
    size_t first_slot = 1;

    // process the queue
    spinlock_lock(&dp->queue.spinlock);
    stream_sender_dispatcher_realloc_arrays_unsafe(dp, 0); // our pipe
    while(dp->queue.ll) {
        worker_is_busy(WORKER_SENDER_DISPATCHER_JOB_DEQUEUE);

        struct sender_state *s = dp->queue.ll;
        DOUBLE_LINKED_LIST_REMOVE_ITEM_UNSAFE(dp->queue.ll, s, prev, next);

        // slot 0 is our pipe
        size_t slot = dp->pollfd.used > 0 ? dp->pollfd.used : 1;

        // find an empty slot
        for(size_t i = first_slot; i < slot && i < dp->pollfd.used ;i++) {
            if(!dp->pollfd.running[i]) {
                slot = i;
                break;
            }
        }

        stream_sender_dispatcher_realloc_arrays_unsafe(dp, slot);

        dp->pollfd.running[slot] = s;
        dp->pollfd.array[slot] = (struct pollfd){
            .fd = s->sock.fd,
            .events = POLLIN,
            .revents = 0,
        };

        sender_lock(s);
        s->dispatcher.pollfd.slot = slot;
        s->dispatcher.pollfd.magic = os_random32();
        s->host->stream.snd.status.connections++;
        s->last_state_since_t = now_realtime_sec();
        stream_sender_update_dispatcher_reset_unsafe(s);

        // reset the bytes we have sent for this session
        memset(s->sent_bytes_on_this_connection_per_type, 0, sizeof(s->sent_bytes_on_this_connection_per_type));
        sender_unlock(s);

        stream_sender_on_ready_to_dispatch(s);

        first_slot = slot + 1;
    }
    spinlock_unlock(&dp->queue.spinlock);
}

static void stream_sender_dispatcher_move_running_to_connector_or_remove(struct dispatcher *dp, size_t slot, bool reconnect) {
    dp->pollfd.array[slot] = (struct pollfd) {
        .fd = -1,
        .events = 0,
        .revents = 0,
    };

    struct sender_state *s = dp->pollfd.running[slot];
    if(!s) return;

    stream_sender_on_disconnect(s);

    dp->pollfd.running[slot] = NULL;
    s->dispatcher.pollfd.slot = 0;

    if (!reconnect || stream_sender_is_signaled_to_stop(s))
        stream_sender_connector_remove_unlinked(s);
    else
        stream_sender_connector_add_unlinked(s);
}

void *stream_sender_dispacther_thread(void *ptr) {
    struct dispatcher *dp = ptr;

    worker_register("STREAMSND");
    worker_register_job_name(WORKER_SENDER_DISPATCHER_JOB_LIST, "list");
    worker_register_job_name(WORKER_SENDER_DISPATCHER_JOB_DEQUEUE, "dequeue");
    worker_register_job_name(WORKER_SENDER_DISPATCHER_JOB_POLL_ERROR, "disconnect poll error");
    worker_register_job_name(WORKER_SENDER_DISPATCHER_JOB_PIPE_READ, "pipe read");
    worker_register_job_name(WORKER_SENDER_DISPATCHER_JOB_SOCKET_RECEIVE, "receive");
    worker_register_job_name(WORKER_SENDER_DISPATCHER_JOB_SOCKET_SEND, "send");
    worker_register_job_name(WORKER_SENDER_DISPATCHER_JOB_EXECUTE, "execute");

    // disconnection reasons
    worker_register_job_name(WORKER_SENDER_DISPATCHER_JOB_DISCONNECT_OVERFLOW, "disconnect overflow");
    worker_register_job_name(WORKER_SENDER_DISPATCHER_JOB_DISCONNECT_TIMEOUT, "disconnect timeout");
    worker_register_job_name(WORKER_SENDER_DISPATCHER_JOB_DISCONNECT_SOCKET_ERROR, "disconnect socket error");
    worker_register_job_name(WORKER_SENDER_DISPATCHER_JOB_DISCONNECT_PARENT_CLOSED, "disconnect parent closed");
    worker_register_job_name(WORKER_SENDER_DISPATCHER_JOB_DISCONNECT_RECEIVE_ERROR, "disconnect receive error");
    worker_register_job_name(WORKER_SENDER_DISPATCHER_JOB_DISCONNECT_SEND_ERROR, "disconnect send error");

    worker_register_job_name(WORKER_SENDER_DISPATCHER_JOB_REPLAY_REQUEST, "replay request");
    worker_register_job_name(WORKER_SENDER_DISPATCHER_JOB_FUNCTION_REQUEST, "function");

    worker_register_job_custom_metric(WORKER_SENDER_DISPATCHER_JOB_NODES, "nodes", "nodes", WORKER_METRIC_ABSOLUTE);
    worker_register_job_custom_metric(WORKER_SENDER_DISPATCHER_JOB_BUFFER_RATIO, "used buffer ratio", "%", WORKER_METRIC_ABSOLUTE);
    worker_register_job_custom_metric(WORKER_SENDER_DISPATCHER_JOB_BYTES_RECEIVED, "bytes received", "bytes/s", WORKER_METRIC_INCREMENT);
    worker_register_job_custom_metric(WORKER_SENDER_DISPATCHER_JOB_BYTES_SENT, "bytes sent", "bytes/s", WORKER_METRIC_INCREMENT);
    worker_register_job_custom_metric(WORKER_SENDER_DISPATCHER_JOB_BYTES_COMPRESSED, "bytes compressed", "bytes/s", WORKER_METRIC_INCREMENTAL_TOTAL);
    worker_register_job_custom_metric(WORKER_SENDER_DISPATCHER_JOB_BYTES_UNCOMPRESSED, "bytes uncompressed", "bytes/s", WORKER_METRIC_INCREMENTAL_TOTAL);
    worker_register_job_custom_metric(WORKER_SENDER_DISPATCHER_JOB_BYTES_COMPRESSION_RATIO, "cumulative compression savings ratio", "%", WORKER_METRIC_ABSOLUTE);
    worker_register_job_custom_metric(WORKER_SENDER_DISPATHCER_JOB_REPLAY_DICT_SIZE, "replication dict entries", "entries", WORKER_METRIC_ABSOLUTE);
    worker_register_job_custom_metric(WORKER_SENDER_DISPATHCER_JOB_MESSAGES, "pipe messages received", "messages", WORKER_METRIC_INCREMENT);

    if(pipe(dp->pipe_fds) != 0) {
        nd_log(NDLS_DAEMON, NDLP_ERR, "STREAM [dispatch%d]: cannot create required pipe.", dp->id);
        dp->pipe_fds[PIPE_READ] = -1;
        dp->pipe_fds[PIPE_WRITE] = -1;
        return NULL;
    }

    size_t pipe_messages_size = 4096;
    struct pipe_msg *pipe_messages_buf = mallocz(pipe_messages_size * sizeof(*pipe_messages_buf));

    usec_t now_ut = now_monotonic_usec();
    usec_t next_all_ut = now_ut;
    uint64_t messages = 0;

    while(!nd_thread_signaled_to_cancel() && service_running(SERVICE_STREAMING)) {
        worker_is_busy(WORKER_SENDER_DISPATCHER_JOB_LIST);

        stream_sender_dispatcher_move_queue_to_running(dp);

        now_ut = now_monotonic_usec();
        time_t now_s = (time_t)(now_ut / USEC_PER_SEC);

        bool do_all = false;
        if(now_ut >= next_all_ut) {
            next_all_ut = now_ut + 50 * USEC_PER_MS;
            do_all = true;
        }

        size_t bytes_uncompressed = 0;
        size_t bytes_compressed = 0;
        NETDATA_DOUBLE buffer_ratio = 0.0;

        size_t nodes = 0;
        for(size_t slot = 1; slot < dp->pollfd.used ; slot++) {
            struct sender_state *s = dp->pollfd.running[slot];
            if(!s) continue;

            nodes++;

            dp->pollfd.array[slot].events = POLLIN;
            dp->pollfd.array[slot].fd = s->sock.fd;
            dp->pollfd.array[slot].revents = 0;

            if(!do_all && !s->dispatcher.interactive)
                continue;

            // If the TCP window never opened then something is wrong, restart connection
            if(unlikely(do_all &&
                         now_s - s->last_traffic_seen_t > stream_send.parents.timeout_s &&
                         !rrdpush_sender_pending_replication_requests(s) &&
                         !rrdpush_sender_replicating_charts(s)
                             )) {

                ND_LOG_STACK lgs[] = {
                    ND_LOG_FIELD_STR(NDF_NIDL_NODE, s->host->hostname),
                    ND_LOG_FIELD_CB(NDF_DST_IP, stream_sender_log_dst_ip, s),
                    ND_LOG_FIELD_CB(NDF_DST_PORT, stream_sender_log_dst_port, s),
                    ND_LOG_FIELD_CB(NDF_DST_TRANSPORT, stream_sender_log_transport, s),
                    ND_LOG_FIELD_CB(NDF_SRC_CAPABILITIES, stream_sender_log_capabilities, s),
                    ND_LOG_FIELD_END(),
                };
                ND_LOG_STACK_PUSH(lgs);

                worker_is_busy(WORKER_SENDER_DISPATCHER_JOB_DISCONNECT_TIMEOUT);

                nd_log(NDLS_DAEMON, NDLP_ERR,
                       "STREAM [dispatch%d] %s [send to %s]: could not send metrics for %ld seconds - closing connection - "
                       "we have sent %zu bytes on this connection via %zu send attempts.",
                       dp->id, rrdhost_hostname(s->host), s->connected_to, stream_send.parents.timeout_s,
                       s->sent_bytes_on_this_connection, s->send_attempts);

                stream_sender_dispatcher_move_running_to_connector_or_remove(dp, slot, true);
                continue;
            }

            sender_lock(s);
            bytes_compressed += s->dispatcher.bytes_compressed;
            bytes_uncompressed += s->dispatcher.bytes_uncompressed;
            uint64_t outstanding = s->dispatcher.bytes_outstanding;
            if (s->dispatcher.buffer_ratio > buffer_ratio) buffer_ratio = s->dispatcher.buffer_ratio;
            sender_unlock(s);

            if(outstanding)
                dp->pollfd.array[slot].events |= POLLOUT;
        }

        if(do_all) {
            if (bytes_compressed && bytes_uncompressed) {
                NETDATA_DOUBLE compression_ratio = 100.0 - ((NETDATA_DOUBLE)bytes_compressed * 100.0 / (NETDATA_DOUBLE)bytes_uncompressed);
                worker_set_metric(WORKER_SENDER_DISPATCHER_JOB_BYTES_COMPRESSION_RATIO, compression_ratio);
            }

            worker_set_metric(WORKER_SENDER_DISPATCHER_JOB_NODES, (NETDATA_DOUBLE)nodes);
            worker_set_metric(WORKER_SENDER_DISPATCHER_JOB_BYTES_UNCOMPRESSED, (NETDATA_DOUBLE)bytes_uncompressed);
            worker_set_metric(WORKER_SENDER_DISPATCHER_JOB_BYTES_COMPRESSED, (NETDATA_DOUBLE)bytes_compressed);
            worker_set_metric(WORKER_SENDER_DISPATCHER_JOB_BUFFER_RATIO, buffer_ratio);
            worker_set_metric(WORKER_SENDER_DISPATHCER_JOB_MESSAGES, (NETDATA_DOUBLE)messages);
        }

        dp->pollfd.array[0] = (struct pollfd) {
            .fd = dp->pipe_fds[PIPE_READ],
            .events = POLLIN,
            .revents = 0,
        };

        worker_is_idle();
        int poll_rc = poll(
            dp->pollfd.array,
            dp->pollfd.used, 50); // timeout in milliseconds

        if (poll_rc == 0 || ((poll_rc == -1) && (errno == EAGAIN || errno == EINTR)))
            // timeout
            continue;

        if(unlikely(poll_rc == -1)) {
            // poll() error
            worker_is_busy(WORKER_SENDER_DISPATCHER_JOB_POLL_ERROR);
            nd_log_limit_static_thread_var(erl, 1, 1 * USEC_PER_MS);
            nd_log_limit(&erl, NDLS_DAEMON, NDLP_ERR, "STREAM [dispatch%d] poll() returned error", dp->id);
            continue;
        }

        now_s = now_monotonic_sec();

        // If the collector woke us up then empty the pipe to remove the signal
        if(dp->pollfd.array[0].revents) {
            short revents = dp->pollfd.array[0].revents;

            if (likely(revents & (POLLIN | POLLPRI))) {
                worker_is_busy(WORKER_SENDER_DISPATCHER_JOB_PIPE_READ);
                ssize_t bytes = read(
                    dp->pollfd.array[0].fd, pipe_messages_buf, sizeof(*pipe_messages_buf) * pipe_messages_size);
                if(bytes > 0) {
                    size_t count = bytes / sizeof(*pipe_messages_buf);
                    if(bytes % count) {
                        nd_log(NDLS_DAEMON, NDLP_ERR,
                               "STREAM [dispatch%d]: received partial number of messages from pipe by %zu bytes",
                               dp->id, bytes % count);
                    }

                    messages += count;

                    for(size_t i = 0; i < count ; i++) {
                        struct pipe_msg *msg = &pipe_messages_buf[i];
                        if (msg->slot > 0 && msg->slot < dp->pollfd.used &&
                            dp->pollfd.running[msg->slot]->dispatcher.pollfd.magic == msg->magic) {
                            switch (msg->msg) {
                                case SENDER_MSG_INTERACTIVE:
                                    dp->pollfd.running[msg->slot]->dispatcher.interactive = true;
                                    break;

                                case SENDER_MSG_RECONNECT:
                                    stream_sender_dispatcher_move_running_to_connector_or_remove(dp, msg->slot, true);
                                    break;

                                case SENDER_MSG_STOP:
                                    stream_sender_dispatcher_move_running_to_connector_or_remove(dp, msg->slot, false);
                                    break;

                                default:
                                    nd_log(NDLS_DAEMON, NDLP_ERR, "STREAM [dispatch%d]: invalid msg id %u",
                                           dp->id, (unsigned)msg->msg);
                                    break;
                            }
                        }
                        else {
                            nd_log(NDLS_DAEMON, NDLP_ERR,
                                   "STREAM [dispatch%d]: invalid slot %" PRIu32 " read from pipe", dp->id, msg->slot);
                        }
                    }
                }
            }
            else if(unlikely(revents & (POLLERR|POLLHUP|POLLNVAL))) {
                // we have errors on this pipe

                close(dp->pipe_fds[PIPE_READ]);
                dp->pipe_fds[PIPE_READ] = -1;
                close(dp->pipe_fds[PIPE_WRITE]);
                dp->pipe_fds[PIPE_WRITE] = -1;
                if(pipe(dp->pipe_fds) != 0) {
                    nd_log(NDLS_DAEMON, NDLP_ERR, "STREAM [dispatch%d]: cannot create required pipe.", dp->id);
                    break; // exit the dispatcher thread
                }
                else
                    nd_log(NDLS_DAEMON, NDLP_ERR, "STREAM [dispatch%d]: restarted internal pipe.", dp->id);
            }
        }

        size_t replay_entries = 0;
        size_t bytes_received = 0;
        size_t bytes_sent = 0;

        for(size_t slot = 1; slot < dp->pollfd.used ; slot++) {
            struct sender_state *s = dp->pollfd.running[slot];
            if(!s || !dp->pollfd.array[slot].revents)
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

            if(unlikely(s->flags & SENDER_FLAG_OVERFLOW)) {
                worker_is_busy(WORKER_SENDER_DISPATCHER_JOB_DISCONNECT_OVERFLOW);
                errno_clear();
                nd_log(NDLS_DAEMON, NDLP_ERR,
                       "STREAM [dispatch%d] %s [send to %s]: buffer full (allocated %zu bytes) after sending %zu bytes. "
                       "Restarting connection.",
                       dp->id, rrdhost_hostname(s->host), s->connected_to, s->buffer->size, s->sent_bytes_on_this_connection);
                stream_sender_dispatcher_move_running_to_connector_or_remove(dp, slot, true);
                continue;
            }

            short revents = dp->pollfd.array[slot].revents;

            if(unlikely(revents & (POLLERR|POLLHUP|POLLNVAL))) {
                // we have errors on this socket

                char *error = "unknown error";

                if (revents & POLLERR)
                    error = "socket reports errors (POLLERR)";
                else if (revents & POLLHUP)
                    error = "connection closed by remote end (POLLHUP)";
                else if (revents & POLLNVAL)
                    error = "connection is invalid (POLLNVAL)";

                worker_is_busy(WORKER_SENDER_DISPATCHER_JOB_DISCONNECT_SOCKET_ERROR);

                nd_log(NDLS_DAEMON, NDLP_ERR,
                       "STREAM [dispatch%d] %s [send to %s]: %s restarting connection - %zu bytes transmitted.",
                       dp->id, rrdhost_hostname(s->host), s->connected_to, error, s->sent_bytes_on_this_connection);

                stream_sender_dispatcher_move_running_to_connector_or_remove(dp, slot, true);
                continue;
            }

            if(revents & POLLOUT) {
                // we can send data on this socket

                worker_is_busy(WORKER_SENDER_DISPATCHER_JOB_SOCKET_SEND);
                s->send_attempts++;
                bool disconnect = false;

                sender_lock(s);
                {
                    char *chunk;
                    size_t outstanding = cbuffer_next_unsafe(s->buffer, &chunk);
                    ssize_t bytes = nd_sock_send_nowait(&s->sock, chunk, outstanding);
                    if (likely(bytes > 0)) {
                        cbuffer_remove_unsafe(s->buffer, bytes);
                        stream_sender_update_dispatcher_sent_data_unsafe(s, bytes);
                        s->last_traffic_seen_t = now_s;
                        bytes_sent += bytes;

                        if(!s->dispatcher.bytes_outstanding) {
                            s->dispatcher.interactive = false;
                            s->dispatcher.interactive_sent = false;
                        }
                    }
                    else if (bytes < 0 && errno != EWOULDBLOCK && errno != EAGAIN && errno != EINTR)
                        disconnect = true;
                }
                sender_unlock(s);

                if(disconnect) {
                    worker_is_busy(WORKER_SENDER_DISPATCHER_JOB_DISCONNECT_SEND_ERROR);
                    nd_log(NDLS_DAEMON, NDLP_ERR,
                           "STREAM [dispatch%d] %s [send to %s]: failed to send metrics - restarting connection - we have sent %zu bytes on this connection.",
                           dp->id, rrdhost_hostname(s->host), s->connected_to, s->sent_bytes_on_this_connection);
                    stream_sender_dispatcher_move_running_to_connector_or_remove(dp, slot, true);
                    continue;
                }
            }

            if(revents & POLLIN) {
                // we can receive data from this socket

                worker_is_busy(WORKER_SENDER_DISPATCHER_JOB_SOCKET_RECEIVE);
                ssize_t bytes = nd_sock_revc_nowait(&s->sock, s->read_buffer + s->read_len, sizeof(s->read_buffer) - s->read_len - 1);
                if (bytes > 0) {
                    s->read_len += bytes;
                    s->last_traffic_seen_t = now_s;
                    bytes_received += bytes;
                }
                else if (bytes == 0 || errno == ECONNRESET) {
                    worker_is_busy(WORKER_SENDER_DISPATCHER_JOB_DISCONNECT_PARENT_CLOSED);
                    nd_log(NDLS_DAEMON, NDLP_ERR,
                           "STREAM [dispatch%d] %s [send to %s]: connection (fd %d) closed by far end.",
                           dp->id, rrdhost_hostname(s->host), s->connected_to, s->sock.fd);
                    stream_sender_dispatcher_move_running_to_connector_or_remove(dp, slot, true);
                    continue;
                }
                else if (bytes < 0 && errno != EWOULDBLOCK && errno != EAGAIN && errno != EINTR) {
                    worker_is_busy(WORKER_SENDER_DISPATCHER_JOB_DISCONNECT_RECEIVE_ERROR);
                    nd_log(NDLS_DAEMON, NDLP_ERR,
                           "STREAM [dispatch%d] %s [send to %s]: error during receive (%zd, on fd %d) - restarting connection.",
                           dp->id, rrdhost_hostname(s->host), s->connected_to, bytes, s->sock.fd);
                    stream_sender_dispatcher_move_running_to_connector_or_remove(dp, slot, true);
                    continue;
                }
            }

            if(unlikely(s->read_len)) {
                worker_is_busy(WORKER_SENDER_DISPATCHER_JOB_EXECUTE);
                rrdpush_sender_execute_commands(s);
            }

            replay_entries += dictionary_entries(s->replication.requests);
        }

        worker_set_metric(WORKER_SENDER_DISPATCHER_JOB_BYTES_RECEIVED, (NETDATA_DOUBLE)bytes_received);
        worker_set_metric(WORKER_SENDER_DISPATCHER_JOB_BYTES_SENT, (NETDATA_DOUBLE)bytes_sent);
        worker_set_metric(WORKER_SENDER_DISPATHCER_JOB_REPLAY_DICT_SIZE, (NETDATA_DOUBLE)replay_entries);
    }

    // dequeue
    while(dp->queue.ll)
        stream_sender_dispatcher_move_queue_to_running(dp);

    // stop all hosts
    for(size_t slot = 1; slot < dp->pollfd.used ;slot++) {
        struct sender_state *s = dp->pollfd.running[slot];
        if(!s) continue;

        stream_sender_dispatcher_move_running_to_connector_or_remove(dp, slot, false);
    }

    // cleanup
    freez(dp->pollfd.array);
    freez(dp->pollfd.running);
    memset(&dp->pollfd, 0, sizeof(dp->pollfd));

    freez(pipe_messages_buf);
    close(dp->pipe_fds[PIPE_READ]);
    dp->pipe_fds[PIPE_READ] = -1;
    close(dp->pipe_fds[PIPE_WRITE]);
    dp->pipe_fds[PIPE_WRITE] = -1;

    dp->thread = NULL;

    return NULL;
}

static bool stream_sender_dispatcher_init(struct sender_state *s) {
    static SPINLOCK spinlock = NETDATA_SPINLOCK_INITIALIZER;
    if(!s) return false;

    struct dispatcher *dp = stream_sender_dispatcher(s);

    spinlock_lock(&spinlock);

    if(dp->thread == NULL) {
        dp->pipe_fds[PIPE_READ] = -1;
        dp->pipe_fds[PIPE_WRITE] = -1;

        char tag[NETDATA_THREAD_TAG_MAX + 1];
        snprintfz(tag, NETDATA_THREAD_TAG_MAX, THREAD_TAG_STREAM_SENDER "-DP" "[%d]", dp->id);

        dp->thread = nd_thread_create(tag, NETDATA_THREAD_OPTION_DEFAULT, stream_sender_dispacther_thread, dp);
        if (!dp->thread)
            nd_log_daemon(NDLP_ERR, "STREAM [dispatch%d]: failed to create new thread for client.", dp->id);
    }

    spinlock_unlock(&spinlock);

    return dp->thread != NULL;
}

void stream_sender_start_host_routing(RRDHOST *host) {
    host->sender->dispatcher.id = (int)os_random(MAX_DISPATCHERS);
    bool connector_running =  stream_sender_connector_init();
    bool dispatcher_running = stream_sender_dispatcher_init(host->sender);

    if(dispatcher_running && connector_running) {
        rrdhost_stream_parent_ssl_init(host->sender);
        stream_sender_connector_add_unlinked(host->sender);
    }
}
