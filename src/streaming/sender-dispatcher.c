// SPDX-License-Identifier: GPL-3.0-or-later

#include "sender-internals.h"

static void stream_sender_cbuffer_recreate_timed_unsafe(struct sender_state *s, time_t now_s, bool force) {
    static __thread time_t last_reset_time_s = 0;

    if(!force && now_s - last_reset_time_s < 300)
        return;

    last_reset_time_s = now_s;

    s->sbuf.recreates++; // we increase even if we don't do it, to have sender_start() recreate its buffers

    if(s->sbuf.cb && s->sbuf.cb->size > CBUFFER_INITIAL_SIZE) {
        size_t max = s->sbuf.cb->max_size;
        cbuffer_free(s->sbuf.cb);
        s->sbuf.cb = cbuffer_new(CBUFFER_INITIAL_SIZE, max, &netdata_buffers_statistics.cbuffers_streaming);
    }
}

static void rrdpush_sender_cbuffer_flush(RRDHOST *host) {
    rrdpush_sender_set_flush_time(host->sender);

    sender_lock(host->sender);

    // flush the output buffer from any data it may have
    cbuffer_flush(host->sender->sbuf.cb);
    stream_sender_cbuffer_recreate_timed_unsafe(host->sender, now_monotonic_sec(), true);

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
    nd_log(NDLS_DAEMON, NDLP_DEBUG,
           "STREAM [dispatchX] [%s]: running on-connect hooks...",
           rrdhost_hostname(s->host));

    rrdhost_flag_set(s->host, RRDHOST_FLAG_RRDPUSH_SENDER_CONNECTED);

    rrdpush_sender_charts_and_replication_reset(s);
    rrdpush_sender_cbuffer_flush(s->host);

    s->last_traffic_seen_t = now_monotonic_sec();
    s->flags &= ~SENDER_FLAG_OVERFLOW;
    s->rbuf.read_len = 0;
    s->sbuf.cb->read = 0;
    s->sbuf.cb->write = 0;
    s->send_attempts = 0;
}

static void stream_sender_on_ready_to_dispatch(struct sender_state *s) {
    nd_log(NDLS_DAEMON, NDLP_DEBUG,
           "STREAM [dispatchX] [%s]: running ready-to-dispatch hooks...",
           rrdhost_hostname(s->host));

    // set this flag before sending any data, or the data will not be sent
    rrdhost_flag_set(s->host, RRDHOST_FLAG_RRDPUSH_SENDER_READY_4_METRICS);

    rrdpush_sender_execute_commands_cleanup(s);
    rrdpush_sender_thread_send_custom_host_variables(s->host);
    stream_path_send_to_parent(s->host);
    rrdpush_sender_send_claimed_id(s->host);
    rrdpush_send_host_labels(s->host);
    rrdpush_send_global_functions(s->host);
}

static void stream_sender_on_disconnect(struct sender_state *s) {
    nd_log(NDLS_DAEMON, NDLP_DEBUG,
           "STREAM [dispatchX] [%s]: running on-disconnect hooks...",
           rrdhost_hostname(s->host));

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
    pid_t tid;
    ND_THREAD *thread;

    struct {
        SPINLOCK spinlock; // ensure a single writer at a time
        int fds[2];

        // ----

        size_t residual_bytes;      // partial pipe reads are tracked here
        size_t size;
        struct pipe_msg *messages;
    } pipe;

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
        struct pollfd *pollfds;         // the array to pass to poll()
        struct sender_state **senders;  // the array of senders (may have nulls in it)
    } run;

    struct {
        usec_t next_full_ut;
        size_t messages;
    } ops;
};

static void stream_sender_dispatcher_move_running_to_connector_or_remove(struct dispatcher *dp, size_t slot, STREAM_HANDSHAKE reason, bool reconnect);

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
    s->dispatcher.bytes_outstanding = cbuffer_next_unsafe(s->sbuf.cb, NULL);
    s->dispatcher.bytes_available = cbuffer_available_size_unsafe(s->sbuf.cb);
    s->dispatcher.buffer_ratio = (NETDATA_DOUBLE)(s->sbuf.cb->max_size -  s->dispatcher.bytes_available) * 100.0 / (NETDATA_DOUBLE)s->sbuf.cb->max_size;
    replication_recalculate_buffer_used_ratio_unsafe(s);
}

void stream_sender_update_dispatcher_added_data_unsafe(struct sender_state *s, uint64_t bytes_compressed, uint64_t bytes_uncompressed) {
    // calculate the statistics for our dispatcher
    s->dispatcher.bytes_uncompressed += bytes_uncompressed;
    s->dispatcher.bytes_compressed += bytes_compressed;
    s->dispatcher.bytes_outstanding = cbuffer_next_unsafe(s->sbuf.cb, NULL);
    s->dispatcher.bytes_available = cbuffer_available_size_unsafe(s->sbuf.cb);
    s->dispatcher.buffer_ratio = (NETDATA_DOUBLE)(s->sbuf.cb->max_size -  s->dispatcher.bytes_available) * 100.0 / (NETDATA_DOUBLE)s->sbuf.cb->max_size;
    replication_recalculate_buffer_used_ratio_unsafe(s);
}

void stream_sender_reconnect(struct sender_state *s) {
    struct pipe_msg msg = s->dispatcher.pollfd;
    msg.msg = SENDER_MSG_RECONNECT;
    stream_sender_send_msg_to_dispatcher(s, msg);
}

// --------------------------------------------------------------------------------------------------------------------
// pipe messages

void stream_sender_send_msg_to_dispatcher(struct sender_state *s, struct pipe_msg msg) {
    if (!msg.slot || !msg.magic) return;

    struct dispatcher *dp = stream_sender_dispatcher(s);

    // don't send a message to ourselves
    if (dp->tid == gettid_cached()) return;

    // ensure one writer at a time
    spinlock_lock(&dp->pipe.spinlock);

    int pipe_fd = dp->pipe.fds[PIPE_WRITE];
    if (pipe_fd != -1) {
        ssize_t total_written = 0;
        ssize_t bytes_to_write = sizeof(msg);
        const char *msg_ptr = (const char *)&msg;

        while (total_written < bytes_to_write) {
            ssize_t written = write(pipe_fd, msg_ptr + total_written, bytes_to_write - total_written);

            if (written > 0)
                total_written += written;

            else if (written == -1) {
                if (errno == EINTR)
                    continue; // Interrupted by a signal, retry

                else if (errno == EAGAIN || errno == EWOULDBLOCK) {
                    // pipe is full
                    nd_log_limit_static_global_var(erl, 1, 1 * USEC_PER_MS);
                    nd_log_limit(&erl, NDLS_DAEMON, NDLP_ERR,
                                 "STREAM %s [send]: pipe full, cannot write to internal pipe. Retrying.",
                                 rrdhost_hostname(s->host));
                    continue;
                }
                else {
                    // Other errors
                    nd_log_limit_static_global_var(erl, 1, 1 * USEC_PER_MS);
                    nd_log_limit(&erl, NDLS_DAEMON, NDLP_ERR,
                                 "STREAM %s [send]: cannot write to internal pipe. Error: %s",
                                 rrdhost_hostname(s->host), strerror(errno));
                    break;
                }
            }
        }

        if (total_written < bytes_to_write) {
            nd_log(NDLS_DAEMON, NDLP_ERR,
                   "STREAM %s [send]: partial write, could not write a complete message to internal pipe.",
                   rrdhost_hostname(s->host));
        }
    }

    spinlock_unlock(&dp->pipe.spinlock);
}

static void stream_sender_dispatcher_read_pipe_messages(struct dispatcher *dp) {
    char *buffer_start = (char *)dp->pipe.messages;
    size_t message_size = sizeof(struct pipe_msg);
    size_t max_read_size = message_size * dp->pipe.size;

    char *read_position = buffer_start + dp->pipe.residual_bytes;
    size_t bytes_available = max_read_size - dp->pipe.residual_bytes;

    ssize_t bytes_read = read(dp->pipe.fds[PIPE_READ], read_position, bytes_available);
    if(bytes_read <= 0) {
        if (bytes_read < 0 && errno != EAGAIN && errno != EINTR)
            nd_log(NDLS_DAEMON, NDLP_ERR,
                   "STREAM [dispatch%d]: pipe read error", dp->id);
        return;
    }

    size_t total_bytes = dp->pipe.residual_bytes + bytes_read;
    size_t full_messages = total_bytes / message_size;
    dp->pipe.residual_bytes = total_bytes % message_size;

    dp->ops.messages += full_messages;

    for (size_t i = 0; i < full_messages; i++) {
        struct pipe_msg *msg = &dp->pipe.messages[i];

        if (msg->slot > 0 &&
            msg->slot < dp->run.used &&
            msg->id == dp->id &&
            dp->run.senders[msg->slot] &&
            dp->run.senders[msg->slot]->dispatcher.pollfd.magic == msg->magic) {

            // Process the message
            switch (msg->msg) {
                case SENDER_MSG_INTERACTIVE:
                    dp->run.senders[msg->slot]->dispatcher.interactive = true;
                    break;

                case SENDER_MSG_RECONNECT:
                    stream_sender_dispatcher_move_running_to_connector_or_remove(dp, msg->slot, 0, true);
                    break;

                case SENDER_MSG_STOP:
                    stream_sender_dispatcher_move_running_to_connector_or_remove(dp, msg->slot, 0, false);
                    break;

                case SENDER_MSG_NONE:
                    break;

                default:
                    nd_log(NDLS_DAEMON, NDLP_ERR,
                           "STREAM [dispatch%d]: invalid msg id %u", dp->id, (unsigned)msg->msg);
                    break;
            }
        }
        else {
            nd_log(NDLS_DAEMON, NDLP_ERR,
                   "STREAM [dispatch%d]: invalid slot %" PRIu32 " read from pipe", dp->id, msg->slot);
        }
    }

    if (dp->pipe.residual_bytes > 0) {
        // move the last partial message to the beginning for next call
        memmove(buffer_start, buffer_start + (full_messages * message_size), dp->pipe.residual_bytes);
    }
}

// --------------------------------------------------------------------------------------------------------------------

static void stream_sender_dispatcher_realloc_arrays_unsafe(struct dispatcher *dp, size_t slot) {
    if(slot >= dp->run.size) {
        size_t new_size = dp->run.size > 0 ? dp->run.size * 2 : 8;
        dp->run.pollfds = reallocz(dp->run.pollfds, new_size * sizeof(*dp->run.pollfds));
        dp->run.senders = reallocz(dp->run.senders, new_size * sizeof(*dp->run.senders));
        dp->run.size = new_size;
        dp->run.used = slot + 1;

        // slot zero is always our pipe
        dp->run.pollfds[0] = (struct pollfd) {
            .fd = dp->pipe.fds[PIPE_READ],
            .events = POLLIN,
            .revents = 0,
        };
        dp->run.senders[0] = NULL;
    }
    else if(slot >= dp->run.used)
        dp->run.used = slot + 1;
}

void stream_sender_dispatcher_add_to_queue(struct sender_state *s) {
    struct dispatcher *dp = stream_sender_dispatcher(s);

    nd_log(NDLS_DAEMON, NDLP_DEBUG,
           "STREAM [dispatch%d] [%s]: moving host to dispatcher queue...",
           dp->id, rrdhost_hostname(s->host));

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
        size_t slot = dp->run.used > 0 ? dp->run.used : 1;

        // find an empty slot
        for(size_t i = first_slot; i < slot && i < dp->run.used ;i++) {
            if(!dp->run.senders[i]) {
                slot = i;
                break;
            }
        }

        stream_sender_dispatcher_realloc_arrays_unsafe(dp, slot);

        nd_log(NDLS_DAEMON, NDLP_DEBUG,
               "STREAM [dispatch%d] [%s]: moving host from dispatcher queue to dispatcher running slot %zu...",
               dp->id, rrdhost_hostname(s->host), slot);

        dp->run.senders[slot] = s;
        dp->run.pollfds[slot] = (struct pollfd){
            .fd = s->sock.fd,
            .events = POLLIN,
            .revents = 0,
        };

        sender_lock(s);
        s->dispatcher.pollfd.id = dp->id;
        s->dispatcher.pollfd.slot = slot;
        s->dispatcher.pollfd.magic = os_random32();
        s->host->stream.snd.status.connections++;
        s->last_state_since_t = now_realtime_sec();

        // reset the bytes we have sent for this session
        memset(s->sent_bytes_on_this_connection_per_type, 0, sizeof(s->sent_bytes_on_this_connection_per_type));

        stream_sender_update_dispatcher_reset_unsafe(s);
        sender_unlock(s);

        stream_sender_on_ready_to_dispatch(s);

        first_slot = slot + 1;
    }
    spinlock_unlock(&dp->queue.spinlock);
}

static void stream_sender_dispatcher_move_running_to_connector_or_remove(struct dispatcher *dp, size_t slot, STREAM_HANDSHAKE reason, bool reconnect) {
    dp->run.pollfds[slot] = (struct pollfd) {
        .fd = -1,
        .events = 0,
        .revents = 0,
    };

    if(slot == dp->run.used - 1)
        dp->run.used--;

    struct sender_state *s = dp->run.senders[slot];
    if(!s) {
        nd_log(NDLS_DAEMON, NDLP_ERR,
               "STREAM [dispatch%d] [unknown]: tried to remove host from slot %zu (reconnect = %s), but it is empty!",
               dp->id, slot, reconnect ? "true" : "false");
        return;
    }

    // clear this flag asap, to stop other threads from pushing metrics for this node
    rrdhost_flag_clear(s->host, RRDHOST_FLAG_RRDPUSH_SENDER_CONNECTED | RRDHOST_FLAG_RRDPUSH_SENDER_READY_4_METRICS);

    // clear these asap, to make sender_commit() stop processing data for this host
    sender_lock(s);
    s->dispatcher.pollfd.slot = 0;
    s->dispatcher.pollfd.magic = 0;
    sender_unlock(s);

    nd_sock_close(&s->sock);
    dp->run.senders[slot] = NULL;

    stream_parent_set_disconnect_reason(s->host->stream.snd.parents.current, reason, now_realtime_sec());
    stream_sender_on_disconnect(s);

    if (!reconnect || stream_sender_is_signaled_to_stop(s))
        stream_sender_connector_remove_unlinked(s);
    else
        stream_sender_connector_requeue(s);
}

static int set_pipe_size(int pipe_fd, int new_size) {
    int default_size = new_size;
    int result = new_size;

#ifdef F_GETPIPE_SZ
    // get the current size of the pipe
    result = fcntl(pipe_fd, F_GETPIPE_SZ);
    if(result > 0)
        default_size = result;
#endif

#ifdef F_SETPIPE_SZ
    // set the new size to the pipe
    if(result <= new_size) {
        result = fcntl(pipe_fd, F_SETPIPE_SZ, new_size);
        if (result <= 0)
            return default_size;
    }
#endif

    // we return either:
    // 1. the new_size (after setting it)
    // 2. the current size (if we can't set it, but we can read it)
    // 3. the new_size (without setting it, when we can't read the current size)
    return result;  // Returns the new pipe size
}

void stream_sender_dispatcher_prepare(struct dispatcher *dp) {
    usec_t now_ut = now_monotonic_usec();
    time_t now_s = (time_t)(now_ut / USEC_PER_SEC);

    bool do_all = false;
    if(now_ut >= dp->ops.next_full_ut) {
        dp->ops.next_full_ut = now_ut + 50 * USEC_PER_MS;
        do_all = true;
    }

    size_t bytes_uncompressed = 0;
    size_t bytes_compressed = 0;
    NETDATA_DOUBLE buffer_ratio = 0.0;
    size_t nodes = 0;
    size_t slots_empty = 0;

    for(size_t slot = 1; slot < dp->run.used ; slot++) {
        struct sender_state *s = dp->run.senders[slot];
        if(!s) {
            slots_empty++;
            continue;
        }

        nodes++;

        // the default for all nodes
        dp->run.pollfds[slot].events = POLLIN;
        dp->run.pollfds[slot].revents = 0;

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

            stream_sender_dispatcher_move_running_to_connector_or_remove(dp, slot, STREAM_HANDSHAKE_DISCONNECT_SOCKET_READ_TIMEOUT, true);
            continue;
        }

        sender_lock(s);
        bytes_compressed += s->dispatcher.bytes_compressed;
        bytes_uncompressed += s->dispatcher.bytes_uncompressed;
        uint64_t outstanding = s->dispatcher.bytes_outstanding;
        if (s->dispatcher.buffer_ratio > buffer_ratio) buffer_ratio = s->dispatcher.buffer_ratio;
        sender_unlock(s);

        if(outstanding)
            dp->run.pollfds[slot].events |= POLLOUT;
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
        worker_set_metric(WORKER_SENDER_DISPATHCER_JOB_MESSAGES, (NETDATA_DOUBLE)dp->ops.messages);
    }
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

    if(pipe(dp->pipe.fds) != 0) {
        nd_log(NDLS_DAEMON, NDLP_ERR, "STREAM [dispatch%d]: cannot create required pipe.", dp->id);
        dp->pipe.fds[PIPE_READ] = -1;
        dp->pipe.fds[PIPE_WRITE] = -1;
        return NULL;
    }

    dp->tid = gettid_cached();

    dp->pipe.size = set_pipe_size(dp->pipe.fds[PIPE_READ], 16384 * sizeof(struct pipe_msg)) / sizeof(struct pipe_msg);
    dp->pipe.messages = mallocz(dp->pipe.size * sizeof(struct pipe_msg));

    usec_t now_ut = now_monotonic_usec();
    dp->ops.next_full_ut = now_ut;

    while(!nd_thread_signaled_to_cancel() && service_running(SERVICE_STREAMING)) {
        worker_is_busy(WORKER_SENDER_DISPATCHER_JOB_LIST);

        // move any pending hosts in the inbound queue, to the running list
        stream_sender_dispatcher_move_queue_to_running(dp);

        // prepare dp->pollfd.array
        stream_sender_dispatcher_prepare(dp);

        worker_is_idle();
        dp->run.pollfds[0].revents = 0;

        // wait for data - timeout is in milliseconds
        int poll_rc = poll(dp->run.pollfds, dp->run.used, 50);

        if (poll_rc == 0 || ((poll_rc == -1) && (errno == EAGAIN || errno == EINTR)))
            // timed out - just loop again
            continue;

        if(unlikely(poll_rc == -1)) {
            // poll() returned an error
            worker_is_busy(WORKER_SENDER_DISPATCHER_JOB_POLL_ERROR);
            nd_log_limit_static_thread_var(erl, 1, 1 * USEC_PER_MS);
            nd_log_limit(&erl, NDLS_DAEMON, NDLP_ERR, "STREAM [dispatch%d] poll() returned error", dp->id);
            continue;
        }

        time_t now_s = now_monotonic_sec();

        // If the collector woke us up then empty the pipe to remove the signal
        if(dp->run.pollfds[0].revents) {
            short revents = dp->run.pollfds[0].revents;

            if (likely(revents & (POLLIN | POLLPRI))) {
                worker_is_busy(WORKER_SENDER_DISPATCHER_JOB_PIPE_READ);
                stream_sender_dispatcher_read_pipe_messages(dp);
            }
            else if(unlikely(revents & (POLLERR|POLLHUP|POLLNVAL))) {
                // we have errors on this pipe
                nd_log(NDLS_DAEMON, NDLP_ERR, "STREAM [dispatch%d]: got errors on pipe - exiting to be restarted.", dp->id);
                break; // exit the dispatcher thread
            }
        }

        size_t replay_entries = 0;
        size_t bytes_received = 0;
        size_t bytes_sent = 0;

        for(size_t slot = 1; slot < dp->run.used ; slot++) {
            struct sender_state *s = dp->run.senders[slot];
            if(!s || !dp->run.pollfds[slot].revents)
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
                       dp->id, rrdhost_hostname(s->host), s->connected_to, s->sbuf.cb->size, s->sent_bytes_on_this_connection);
                stream_sender_dispatcher_move_running_to_connector_or_remove(dp, slot, STREAM_HANDSHAKE_DISCONNECT_NOT_SUFFICIENT_READ_BUFFER, true);
                continue;
            }

            short revents = dp->run.pollfds[slot].revents;

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

                stream_sender_dispatcher_move_running_to_connector_or_remove(dp, slot, STREAM_HANDSHAKE_DISCONNECT_SOCKET_ERROR, true);
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
                    size_t outstanding = cbuffer_next_unsafe(s->sbuf.cb, &chunk);
                    ssize_t bytes = nd_sock_send_nowait(&s->sock, chunk, outstanding);
                    if (likely(bytes > 0)) {
                        cbuffer_remove_unsafe(s->sbuf.cb, bytes);
                        stream_sender_update_dispatcher_sent_data_unsafe(s, bytes);
                        s->last_traffic_seen_t = now_s;
                        bytes_sent += bytes;

                        if(!s->dispatcher.bytes_outstanding) {
                            // we sent them all, remove the interactive flag
                            s->dispatcher.interactive = false;
                            s->dispatcher.interactive_sent = false;

                            // recreate the circular buffer if we have to
                            stream_sender_cbuffer_recreate_timed_unsafe(s, now_s, false);
                        }
                        else if(s->dispatcher.bytes_outstanding > s->dispatcher.bytes_available) {
                            // at 50% turn on the interactive flag
                            s->dispatcher.interactive = true;
                            s->dispatcher.interactive_sent = true;
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
                    stream_sender_dispatcher_move_running_to_connector_or_remove(dp, slot, STREAM_HANDSHAKE_DISCONNECT_SOCKET_WRITE_FAILED, true);
                    continue;
                }
            }

            if(revents & POLLIN) {
                // we can receive data from this socket

                worker_is_busy(WORKER_SENDER_DISPATCHER_JOB_SOCKET_RECEIVE);
                ssize_t bytes = nd_sock_revc_nowait(&s->sock, s->rbuf.b + s->rbuf.read_len, sizeof(s->rbuf.b) - s->rbuf.read_len - 1);
                if (bytes > 0) {
                    s->rbuf.read_len += bytes;
                    s->last_traffic_seen_t = now_s;
                    bytes_received += bytes;
                }
                else if (bytes == 0 || errno == ECONNRESET) {
                    worker_is_busy(WORKER_SENDER_DISPATCHER_JOB_DISCONNECT_PARENT_CLOSED);
                    nd_log(NDLS_DAEMON, NDLP_ERR,
                           "STREAM [dispatch%d] %s [send to %s]: connection (fd %d) closed by far end.",
                           dp->id, rrdhost_hostname(s->host), s->connected_to, s->sock.fd);
                    stream_sender_dispatcher_move_running_to_connector_or_remove(dp, slot, STREAM_HANDSHAKE_DISCONNECT_SOCKET_CLOSED_BY_PARENT, true);
                    continue;
                }
                else if (bytes < 0 && errno != EWOULDBLOCK && errno != EAGAIN && errno != EINTR) {
                    worker_is_busy(WORKER_SENDER_DISPATCHER_JOB_DISCONNECT_RECEIVE_ERROR);
                    nd_log(NDLS_DAEMON, NDLP_ERR,
                           "STREAM [dispatch%d] %s [send to %s]: error during receive (%zd, on fd %d) - restarting connection.",
                           dp->id, rrdhost_hostname(s->host), s->connected_to, bytes, s->sock.fd);
                    stream_sender_dispatcher_move_running_to_connector_or_remove(dp, slot, STREAM_HANDSHAKE_DISCONNECT_SOCKET_READ_FAILED, true);
                    continue;
                }
            }

            if(unlikely(s->rbuf.read_len)) {
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
    for(size_t slot = 1; slot < dp->run.used ;slot++) {
        struct sender_state *s = dp->run.senders[slot];
        if(!s) continue;

        stream_sender_dispatcher_move_running_to_connector_or_remove(dp, slot, STREAM_HANDSHAKE_DISCONNECT_SHUTDOWN, false);
    }

    // cleanup
    freez(dp->run.pollfds);
    freez(dp->run.senders);
    memset(&dp->run, 0, sizeof(dp->run));

    freez(dp->pipe.messages);
    dp->pipe.messages = NULL;
    dp->pipe.size = 0;

    close(dp->pipe.fds[PIPE_READ]);
    close(dp->pipe.fds[PIPE_WRITE]);
    dp->pipe.fds[PIPE_READ] = -1;
    dp->pipe.fds[PIPE_WRITE] = -1;

    dp->thread = NULL;
    dp->tid = 0;

    return NULL;
}

static bool stream_sender_dispatcher_init(struct sender_state *s) {
    static SPINLOCK spinlock = NETDATA_SPINLOCK_INITIALIZER;
    if(!s) return false;

    struct dispatcher *dp = stream_sender_dispatcher(s);

    spinlock_lock(&spinlock);

    if(dp->thread == NULL) {
        dp->pipe.fds[PIPE_READ] = -1;
        dp->pipe.fds[PIPE_WRITE] = -1;
        spinlock_init(&dp->pipe.spinlock);
        spinlock_init(&dp->queue.spinlock);
        dp->run.used = 0;

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

    // initialize first the dispatcher, to have its spinlocks and pipes initialized
    // before the connector attempts to use them
    bool dispatcher_running = stream_sender_dispatcher_init(host->sender);
    bool connector_running =  stream_sender_connector_init();

    if(dispatcher_running && connector_running) {
        rrdhost_stream_parent_ssl_init(host->sender);
        stream_sender_connector_add_unlinked(host->sender);
    }
}
