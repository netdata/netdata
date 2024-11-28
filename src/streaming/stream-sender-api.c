// SPDX-License-Identifier: GPL-3.0-or-later

#include "stream-sender-internals.h"

bool stream_sender_has_capabilities(struct rrdhost *host, STREAM_CAPABILITIES capabilities) {
    return host && stream_has_capability(host->sender, capabilities);
}

bool stream_sender_is_connected_with_ssl(struct rrdhost *host) {
    return host && rrdhost_can_send_metadata_to_parent(host) && nd_sock_is_ssl(&host->sender->sock);
}

bool stream_sender_has_compression(struct rrdhost *host) {
    return host && host->sender && host->sender->compressor.initialized;
}

void stream_sender_structures_init(struct rrdhost *host) {
    if (host->sender) return;

    host->sender = callocz(1, sizeof(*host->sender));
    __atomic_add_fetch(&netdata_buffers_statistics.rrdhost_senders, sizeof(*host->sender), __ATOMIC_RELAXED);

    host->sender->connector.id = -1;
    host->sender->thread.slot = -1;
    host->sender->host = host;
    host->sender->sbuf.cb = cbuffer_new(CBUFFER_INITIAL_SIZE, CBUFFER_INITIAL_MAX_SIZE, &netdata_buffers_statistics.cbuffers_streaming);
    host->sender->capabilities = stream_our_capabilities(host, true);

    nd_sock_init(&host->sender->sock, netdata_ssl_streaming_sender_ctx, netdata_ssl_validate_certificate_sender);
    host->sender->disabled_capabilities = STREAM_CAP_NONE;

    if(!stream_send.compression.enabled)
        host->sender->disabled_capabilities |= STREAM_CAP_COMPRESSIONS_AVAILABLE;

    spinlock_init(&host->sender->spinlock);
    replication_init_sender(host->sender);
}

void stream_sender_structures_free(struct rrdhost *host) {
    rrdhost_option_clear(host, RRDHOST_OPTION_SENDER_ENABLED);

    if (unlikely(!host->sender)) return;

    // stop a possibly running thread
    stream_sender_signal_to_stop_and_wait(host, STREAM_HANDSHAKE_DISCONNECT_HOST_CLEANUP, true);
    cbuffer_free(host->sender->sbuf.cb);

    rrdpush_compressor_destroy(&host->sender->compressor);

    replication_cleanup_sender(host->sender);

    __atomic_sub_fetch(&netdata_buffers_statistics.rrdhost_senders, sizeof(*host->sender), __ATOMIC_RELAXED);

    freez(host->sender);
    host->sender = NULL;
    rrdhost_flag_clear(host, RRDHOST_FLAG_RRDPUSH_SENDER_INITIALIZED);
}

void stream_sender_start_host(struct rrdhost *host) {
    internal_fatal(!rrdhost_has_rrdpush_sender_enabled(host),
                   "Host '%s' does not have streaming enabled, but %s() was called",
                   rrdhost_hostname(host), __FUNCTION__);

    stream_sender_add_to_connector_queue(host);
}

void *stream_sender_start_localhost(void *ptr __maybe_unused) {
    if(!localhost) return NULL;
    stream_sender_start_host(localhost);
    return NULL;
}

// Either the receiver lost the connection or the host is being destroyed.
// The sender mutex guards thread creation, any spurious data is wiped on reconnection.
void stream_sender_signal_to_stop_and_wait(struct rrdhost *host, STREAM_HANDSHAKE reason, bool wait) {
    if (!host->sender)
        return;

    sender_lock(host->sender);

    if(rrdhost_flag_check(host, RRDHOST_FLAG_RRDPUSH_SENDER_ADDED)) {
        __atomic_store_n(&host->sender->exit.shutdown, true, __ATOMIC_RELAXED);
        host->sender->exit.reason = reason;
    }

    struct sender_op msg = host->sender->thread.msg;
    sender_unlock(host->sender);

    if(reason == STREAM_HANDSHAKE_DISCONNECT_RECEIVER_LEFT)
        msg.op = SENDER_MSG_STOP_RECEIVER_LEFT;
    else
        msg.op = SENDER_MSG_STOP_HOST_CLEANUP;
    stream_sender_send_msg_to_dispatcher(host->sender, msg);

    while(wait && rrdhost_flag_check(host, RRDHOST_FLAG_RRDPUSH_SENDER_ADDED))
        sleep_usec(10 * USEC_PER_MS);
}
