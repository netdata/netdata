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

void stream_sender_cbuffer_recreate_timed(struct sender_state *s, time_t now_s, bool have_mutex, bool force) {
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
    stream_sender_update_dispatcher_reset_unsafe(host->sender);

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

static void rrdpush_sender_on_disconnect(RRDHOST *host) {
    // we have been connected to this parent - let's cleanup

    rrdpush_sender_charts_and_replication_reset(host);

    // clear the parent's claim id
    rrdpush_sender_clear_parent_claim_id(host);
    rrdpush_receiver_send_node_and_claim_id_to_child(host);
    stream_path_parent_disconnected(host);
}

static bool rrdhost_set_sender(RRDHOST *host) {
    if(unlikely(!host->sender)) return false;

    bool ret = false;
    sender_lock(host->sender);
    if(!host->sender->magic) {
        rrdhost_flag_clear(host, RRDHOST_FLAG_RRDPUSH_SENDER_CONNECTED | RRDHOST_FLAG_RRDPUSH_SENDER_READY_4_METRICS);
        rrdhost_flag_set(host, RRDHOST_FLAG_RRDPUSH_SENDER_SPAWN);
        host->stream.snd.status.connections++;
        host->sender->magic = stream_sender_magic(host->sender);
        host->sender->last_state_since_t = now_realtime_sec();
        host->sender->exit.reason = STREAM_HANDSHAKE_NEVER;
        ret = true;
    }
    sender_unlock(host->sender);

    rrdhost_stream_parents_reset(host, STREAM_HANDSHAKE_PREPARING);

    return ret;
}

static void rrdhost_clear_sender(RRDHOST *host) {
    if(unlikely(!host->sender)) return;

    sender_lock(host->sender);

    if(host->sender->magic == stream_sender_magic(host->sender)) {
        host->sender->magic = 0;
        __atomic_store_n(&host->sender->exit.shutdown, false, __ATOMIC_RELAXED);
        rrdhost_flag_clear(host, RRDHOST_FLAG_RRDPUSH_SENDER_SPAWN | RRDHOST_FLAG_RRDPUSH_SENDER_CONNECTED | RRDHOST_FLAG_RRDPUSH_SENDER_READY_4_METRICS);
        host->sender->last_state_since_t = now_realtime_sec();
        stream_parent_set_disconnect_reason(
            host->stream.snd.parents.current, host->sender->exit.reason, host->sender->last_state_since_t);
    }

    rrdhost_stream_parents_reset(host, STREAM_HANDSHAKE_EXITING);

    sender_unlock(host->sender);

#ifdef NETDATA_LOG_STREAM_SENDER
    if (s->stream_log_fp) {
        fclose(s->stream_log_fp);
        s->stream_log_fp = NULL;
    }
#endif
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

static struct {
    struct {
        ND_THREAD *thread;
        struct completion completion;

        struct {
            // the incoming queue of the connector thread
            // all other threads leave new senders here, to be connected to their parents
            SPINLOCK spinlock;
            struct sender_state *ll;
        } queue;
    } connector;

    struct {
        uint64_t magic;
        pid_t tid;
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
    } dispatcher;
} sender = {
    .connector = {
        .queue = {
            .spinlock = NETDATA_SPINLOCK_INITIALIZER,
            .ll = NULL,
        },
    },
    .dispatcher = {
        .pipe_fds = { -1, -1 },
        .queue = {
            .spinlock = NETDATA_SPINLOCK_INITIALIZER,
            .ll = NULL,
        },
    }
};

void stream_sender_cancel_threads(void) {
    nd_thread_signal_cancel(sender.connector.thread);
    nd_thread_signal_cancel(sender.dispatcher.thread);
}

void stream_sender_update_dispatcher_reset_unsafe(struct sender_state *s) {
    s->sent_bytes_on_this_connection = 0;
    s->dispatcher.bytes_uncompressed = 0;
    s->dispatcher.bytes_compressed = 0;
    s->dispatcher.bytes_outstanding = 0;
    s->dispatcher.bytes_available = 0;
    s->dispatcher.buffer_ratio = 0.0;
    replication_recalculate_buffer_used_ratio_unsafe(s);
}

void stream_sender_update_dispatcher_sent_data_unsafe(struct sender_state *s, uint64_t bytes_sent) {
    s->sent_bytes_on_this_connection += bytes_sent;
    s->sent_bytes += bytes_sent;
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

int sender_write_pipe_fd(struct sender_state *s __maybe_unused) {
    return sender.dispatcher.pipe_fds[PIPE_WRITE];
}

uint64_t stream_sender_magic(struct sender_state *s __maybe_unused) {
    return sender.dispatcher.magic;
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

    spinlock_lock(&sender.connector.queue.spinlock);
    DOUBLE_LINKED_LIST_APPEND_ITEM_UNSAFE(sender.connector.queue.ll, s, prev, next);
    spinlock_unlock(&sender.connector.queue.spinlock);

    completion_mark_complete_a_job(&sender.connector.completion);
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
    spinlock_lock(&sender.dispatcher.queue.spinlock);
    stream_sender_dispatcher_realloc_arrays_unsafe(0); // our pipe
    while(sender.dispatcher.queue.ll) {
        worker_is_busy(WORKER_SENDER_DISPATCHER_JOB_DEQUEUE);

        struct sender_state *s = sender.dispatcher.queue.ll;
        DOUBLE_LINKED_LIST_REMOVE_ITEM_UNSAFE(sender.dispatcher.queue.ll, s, prev, next);

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

        s->dispatcher.pollfd.slot = slot;
        s->dispatcher.pollfd.magic = os_random32();

        first_slot = slot + 1;
    }
    spinlock_unlock(&sender.dispatcher.queue.spinlock);
}

static void stream_sender_dispatcher_move_running_to_connector_or_remove(size_t slot, bool reconnect) {
    sender.dispatcher.pollfd.array[slot] = (struct pollfd) {
        .fd = -1,
        .events = 0,
        .revents = 0,
    };

    struct sender_state *s = sender.dispatcher.pollfd.running[slot];
    if(!s) return;

    sender.dispatcher.pollfd.running[slot] = NULL;
    s->dispatcher.pollfd.slot = 0;

    sender_lock(s);
    {
        rrdpush_sender_thread_close_socket(s);
        rrdpush_sender_on_disconnect(s->host);
        rrdpush_sender_execute_commands_cleanup(s);
    }
    sender_unlock(s);

    if (!reconnect || stream_sender_is_signaled_to_stop(s))
        rrdhost_clear_sender(s->host);
    else
        stream_sender_connector_add(s);
}

void *stream_sender_connector_thread(void *ptr __maybe_unused) {
    worker_register("STREAMCNT");
    worker_register_job_name(WORKER_SENDER_CONNECTOR_JOB_CONNECTING, "connect");
    worker_register_job_name(WORKER_SENDER_CONNECTOR_JOB_CONNECTED, "connected");
    worker_register_job_name(WORKER_SENDER_CONNECTOR_JOB_DISCONNECT_BAD_HANDSHAKE, "bad handshake");
    worker_register_job_name(WORKER_SENDER_CONNECTOR_JOB_DISCONNECT_TIMEOUT, "timeout");
    worker_register_job_name(WORKER_SENDER_CONNECTOR_JOB_DISCONNECT_CANT_UPGRADE_CONNECTION, "cant upgrade");

    worker_register_job_custom_metric(WORKER_SENDER_CONNECTOR_JOB_QUEUED_NODES, "queued nodes", "nodes", WORKER_METRIC_ABSOLUTE);
    worker_register_job_custom_metric(WORKER_SENDER_CONNECTOR_JOB_CONNECTED_NODES, "connected nodes", "nodes", WORKER_METRIC_ABSOLUTE);
    worker_register_job_custom_metric(WORKER_SENDER_CONNECTOR_JOB_FAILED_NODES, "failed nodes", "nodes", WORKER_METRIC_ABSOLUTE);
    worker_register_job_custom_metric(WORKER_SENDER_CONNECTOR_JOB_CANCELLED_NODES, "cancelled nodes", "nodes", WORKER_METRIC_ABSOLUTE);

    unsigned job_id = 0;

    while(!nd_thread_signaled_to_cancel() && service_running(SERVICE_STREAMING)) {
        worker_is_idle();
        job_id = completion_wait_for_a_job_with_timeout(&sender.connector.completion, job_id, 1);
        size_t nodes = 0, connected_nodes = 0, failed_nodes = 0, cancelled_nodes = 0;

        spinlock_lock(&sender.connector.queue.spinlock);
        struct sender_state *next;
        for(struct sender_state *s = sender.connector.queue.ll; s ; s = next) {
            next = s->next;
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

            if(stream_sender_is_signaled_to_stop(s)) {
                cancelled_nodes++;
                DOUBLE_LINKED_LIST_REMOVE_ITEM_UNSAFE(sender.connector.queue.ll, s, prev, next);
                rrdhost_clear_sender(s->host);
                continue;
            }

            spinlock_unlock(&sender.connector.queue.spinlock);
            worker_is_busy(WORKER_SENDER_CONNECTOR_JOB_CONNECTING);
            bool move_to_dispatcher = stream_sender_connect_to_parent(s);
            spinlock_lock(&sender.connector.queue.spinlock);

            if(move_to_dispatcher) {
                connected_nodes++;
                worker_is_busy(WORKER_SENDER_CONNECTOR_JOB_CONNECTED);
                DOUBLE_LINKED_LIST_REMOVE_ITEM_UNSAFE(sender.connector.queue.ll, s, prev, next);
                spinlock_unlock(&sender.connector.queue.spinlock);

                spinlock_lock(&sender.dispatcher.queue.spinlock);
                DOUBLE_LINKED_LIST_APPEND_ITEM_UNSAFE(sender.dispatcher.queue.ll, s, prev, next);
                spinlock_unlock(&sender.dispatcher.queue.spinlock);

                spinlock_lock(&sender.connector.queue.spinlock);
            }
            else
                failed_nodes++;

            worker_is_idle();
        }
        spinlock_unlock(&sender.connector.queue.spinlock);

        worker_set_metric(WORKER_SENDER_CONNECTOR_JOB_QUEUED_NODES, (NETDATA_DOUBLE)nodes);
        worker_set_metric(WORKER_SENDER_CONNECTOR_JOB_CONNECTED_NODES, (NETDATA_DOUBLE)connected_nodes);
        worker_set_metric(WORKER_SENDER_CONNECTOR_JOB_FAILED_NODES, (NETDATA_DOUBLE)failed_nodes);
        worker_set_metric(WORKER_SENDER_CONNECTOR_JOB_CANCELLED_NODES, (NETDATA_DOUBLE)cancelled_nodes);
    }

    return NULL;
}

void *stream_sender_dispacther_thread(void *ptr __maybe_unused) {
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

    sender.dispatcher.tid = gettid_cached();

    if(pipe(sender.dispatcher.pipe_fds) != 0) {
        netdata_log_error("STREAM dispatcher: cannot create required pipe.");
        sender.dispatcher.pipe_fds[PIPE_READ] = -1;
        sender.dispatcher.pipe_fds[PIPE_WRITE] = -1;
        return NULL;
    }

    size_t pipe_messages_size = 32768;
    struct pipe_msg *pipe_messages_buf = mallocz(pipe_messages_size * sizeof(*pipe_messages_buf));

    usec_t now_ut = now_monotonic_usec();
    usec_t next_all_ut = now_ut;
    uint64_t messages = 0;

    while(!nd_thread_signaled_to_cancel() && service_running(SERVICE_STREAMING)) {
        worker_is_busy(WORKER_SENDER_DISPATCHER_JOB_LIST);

        stream_sender_dispatcher_move_queue_to_running();

        now_ut = now_monotonic_usec();
        time_t now_s = (time_t)(now_ut / USEC_PER_SEC);

        bool do_all = false;
        if(next_all_ut < now_ut) {
            next_all_ut += now_ut + 50 * USEC_PER_MS;
            do_all = true;
        }

        size_t bytes_uncompressed = 0;
        size_t bytes_compressed = 0;
        NETDATA_DOUBLE buffer_ratio = 0.0;

        size_t nodes = 0;
        for(size_t slot = 1; slot < sender.dispatcher.pollfd.used ; slot++) {
            struct sender_state *s = sender.dispatcher.pollfd.running[slot];
            if(!s) continue;

            sender.dispatcher.pollfd.array[slot].events = POLLIN;
            sender.dispatcher.pollfd.array[slot].fd = s->sock.fd;
            sender.dispatcher.pollfd.array[slot].revents = 0;

            if(!do_all && !s->dispatcher.interactive)
                continue;

            nodes++;

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
                netdata_log_error("STREAM dispatcher %s [send to %s]: could not send metrics for %ld seconds - closing connection - "
                                  "we have sent %zu bytes on this connection via %zu send attempts.",
                                  rrdhost_hostname(s->host), s->connected_to, stream_send.parents.timeout_s,
                                  s->sent_bytes_on_this_connection, s->send_attempts);
                stream_sender_dispatcher_move_running_to_connector_or_remove(slot, true);
                continue;
            }

            sender_lock(s);
            bytes_compressed += s->dispatcher.bytes_compressed;
            bytes_uncompressed += s->dispatcher.bytes_uncompressed;
            uint64_t outstanding = s->dispatcher.bytes_outstanding;
            if (s->dispatcher.buffer_ratio > buffer_ratio) buffer_ratio = s->dispatcher.buffer_ratio;
            sender_unlock(s);

            if(outstanding)
                sender.dispatcher.pollfd.array[slot].events |= POLLOUT;
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

        sender.dispatcher.pollfd.array[0] = (struct pollfd) {
            .fd = sender.dispatcher.pipe_fds[PIPE_READ],
            .events = POLLIN,
            .revents = 0,
        };

        worker_is_idle();
        int poll_rc = poll(sender.dispatcher.pollfd.array, sender.dispatcher.pollfd.used, 50); // timeout in milliseconds

        if (poll_rc == 0 || ((poll_rc == -1) && (errno == EAGAIN || errno == EINTR)))
            // timeout
            continue;

        if(unlikely(poll_rc == -1)) {
            // poll() error
            worker_is_busy(WORKER_SENDER_DISPATCHER_JOB_POLL_ERROR);
            nd_log_limit_static_thread_var(erl, 1, 1 * USEC_PER_MS);
            nd_log_limit(&erl, NDLS_DAEMON, NDLP_ERR, "poll() returned error");
            continue;
        }

        now_s = now_monotonic_sec();

        // If the collector woke us up then empty the pipe to remove the signal
        if(sender.dispatcher.pollfd.array[0].revents) {
            short revents = sender.dispatcher.pollfd.array[0].revents;

            if (likely(revents & (POLLIN | POLLPRI))) {
                worker_is_busy(WORKER_SENDER_DISPATCHER_JOB_PIPE_READ);
                ssize_t bytes = read(sender.dispatcher.pollfd.array[0].fd, pipe_messages_buf, sizeof(*pipe_messages_buf) * pipe_messages_size);
                if(bytes > 0) {
                    size_t count = bytes / sizeof(*pipe_messages_buf);
                    if(bytes % count)
                        netdata_log_error("STREAM dispatcher: received partial number of messages from pipe by %zu bytes", bytes % count);

                    messages += count;

                    for(size_t i = 0; i < count ; i++) {
                        struct pipe_msg *msg = &pipe_messages_buf[i];
                        if (msg->slot > 0 && msg->slot < sender.dispatcher.pollfd.used &&
                            sender.dispatcher.pollfd.running[msg->slot]->dispatcher.pollfd.magic == msg->magic) {
                            switch (msg->msg) {
                                case SENDER_MSG_INTERACTIVE:
                                    sender.dispatcher.pollfd.running[msg->slot]->dispatcher.interactive = true;
                                    break;

                                case SENDER_MSG_RECONNECT:
                                    stream_sender_dispatcher_move_running_to_connector_or_remove(msg->slot, true);
                                    break;

                                case SENDER_MSG_STOP:
                                    stream_sender_dispatcher_move_running_to_connector_or_remove(msg->slot, false);
                                    break;

                                default:
                                    netdata_log_error("STREAM dispatcher: invalid msg id %u", (unsigned)msg->msg);
                                    break;
                            }
                        } else
                            netdata_log_error("STREAM dispatcher: invalid slot %" PRIu32 " read from pipe", msg->slot);
                    }
                }
            }
            else if(unlikely(revents & (POLLERR|POLLHUP|POLLNVAL))) {
                // we have errors on this pipe

                close(sender.dispatcher.pipe_fds[PIPE_READ]); sender.dispatcher.pipe_fds[PIPE_READ] = -1;
                close(sender.dispatcher.pipe_fds[PIPE_WRITE]); sender.dispatcher.pipe_fds[PIPE_WRITE] = -1;
                if(pipe(sender.dispatcher.pipe_fds) != 0) {
                    netdata_log_error("STREAM dispatcher: cannot create required pipe.");
                    break; // exit the dispatcher thread
                }
                else
                    netdata_log_error("STREAM dispatcher: restarted internal pipe.");
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

            if(unlikely(s->flags & SENDER_FLAG_OVERFLOW)) {
                worker_is_busy(WORKER_SENDER_DISPATCHER_JOB_DISCONNECT_OVERFLOW);
                errno_clear();
                netdata_log_error("STREAM dispatcher %s [send to %s]: buffer full (allocated %zu bytes) after sending %zu bytes. Restarting connection",
                                  rrdhost_hostname(s->host), s->connected_to, s->buffer->size, s->sent_bytes_on_this_connection);
                stream_sender_dispatcher_move_running_to_connector_or_remove(slot, true);
                continue;
            }

            short revents = sender.dispatcher.pollfd.array[slot].revents;

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
                netdata_log_error("STREAM dispatcher %s [send to %s]: restarting connection: %s - %zu bytes transmitted.",
                                  rrdhost_hostname(s->host), s->connected_to, error, s->sent_bytes_on_this_connection);
                stream_sender_dispatcher_move_running_to_connector_or_remove(slot, true);
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
                    netdata_log_error("STREAM dispatcher %s [send to %s]: failed to send metrics - closing connection - we have sent %zu bytes on this connection.",
                                      rrdhost_hostname(s->host), s->connected_to, s->sent_bytes_on_this_connection);
                    stream_sender_dispatcher_move_running_to_connector_or_remove(slot, true);
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
                    netdata_log_error("STREAM dispatcher %s [send to %s]: connection (fd %d) closed by far end.",
                                      rrdhost_hostname(s->host), s->connected_to, s->sock.fd);
                    stream_sender_dispatcher_move_running_to_connector_or_remove(slot, true);
                    continue;
                }
                else if (bytes < 0 && errno != EWOULDBLOCK && errno != EAGAIN && errno != EINTR) {
                    worker_is_busy(WORKER_SENDER_DISPATCHER_JOB_DISCONNECT_RECEIVE_ERROR);
                    netdata_log_error("STREAM dispatcher %s [send to %s]: error during receive (%zd, on fd %d) - closing connection.",
                                      rrdhost_hostname(s->host), s->connected_to, bytes, s->sock.fd);
                    stream_sender_dispatcher_move_running_to_connector_or_remove(slot, true);
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
    while(sender.dispatcher.queue.ll)
        stream_sender_dispatcher_move_queue_to_running();

    // stop all hosts
    for(size_t slot = 1; slot < sender.dispatcher.pollfd.used ;slot++) {
        struct sender_state *s = sender.dispatcher.pollfd.running[slot];
        if(!s) continue;

        stream_sender_dispatcher_move_running_to_connector_or_remove(slot, false);
    }

    // cleanup
    freez(sender.dispatcher.pollfd.array);
    freez(sender.dispatcher.pollfd.running);
    memset(&sender.dispatcher.pollfd, 0, sizeof(sender.dispatcher.pollfd));

    freez(pipe_messages_buf);
    close(sender.dispatcher.pipe_fds[PIPE_READ]); sender.dispatcher.pipe_fds[PIPE_READ] = -1;
    close(sender.dispatcher.pipe_fds[PIPE_WRITE]); sender.dispatcher.pipe_fds[PIPE_WRITE] = -1;

    return NULL;
}

void stream_sender_start_host_routing(RRDHOST *host) {
    static SPINLOCK spinlock = NETDATA_SPINLOCK_INITIALIZER;
    static bool dispatcher_running = false;
    static bool connector_running = false;

    spinlock_lock(&spinlock);
    sender_lock(host->sender);

    if(!rrdhost_flag_check(host, RRDHOST_FLAG_RRDPUSH_SENDER_SPAWN)) {
        sender.dispatcher.magic = 1 + os_random64();

        if(!dispatcher_running && !connector_running)
            completion_init(&sender.connector.completion);

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
