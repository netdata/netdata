// SPDX-License-Identifier: GPL-3.0-or-later

#include "stream-sender-internals.h"
#include "stream-replication-sender.h"

bool stream_sender_has_capabilities(struct rrdhost *host, STREAM_CAPABILITIES capabilities) {
    return host && stream_has_capability(host->sender, capabilities);
}

bool stream_sender_is_connected_with_ssl(struct rrdhost *host) {
    return host && rrdhost_can_stream_metadata_to_parent(host) && nd_sock_is_ssl(&host->sender->sock);
}

bool stream_sender_has_compression(struct rrdhost *host) {
    return host && host->sender && host->sender->thread.compressor.initialized;
}

void stream_sender_structures_init(RRDHOST *host, bool stream, STRING *parents, STRING *api_key, STRING *send_charts_matching) {
    if(rrdhost_flag_check(host, RRDHOST_FLAG_STREAM_SENDER_INITIALIZED))
        return;

    if(!stream || !parents || !api_key) {
        rrdhost_option_clear(host, RRDHOST_OPTION_SENDER_ENABLED);
        return;
    }

    rrdhost_flag_set(host, RRDHOST_FLAG_STREAM_SENDER_INITIALIZED);

    if (host->sender) return;

    host->sender = callocz(1, sizeof(*host->sender));
    __atomic_add_fetch(&netdata_buffers_statistics.rrdhost_senders, sizeof(*host->sender), __ATOMIC_RELAXED);

    host->sender->connector.id = -1;
    host->sender->host = host;
    host->sender->scb = stream_circular_buffer_create();
    waitq_init(&host->sender->waitq);
    host->sender->capabilities = stream_our_capabilities(host, true);

    nd_sock_init(&host->sender->sock, netdata_ssl_streaming_sender_ctx, netdata_ssl_validate_certificate_sender);
    host->sender->disabled_capabilities = STREAM_CAP_NONE;

    if(!stream_send.compression.enabled)
        host->sender->disabled_capabilities |= STREAM_CAP_COMPRESSIONS_AVAILABLE;

    spinlock_init(&host->sender->spinlock);
    replication_sender_init(host->sender);

    // gracefully swap destination
    if(host->stream.snd.destination != parents) {
        STRING *t = string_dup(parents);
        SWAP(host->stream.snd.destination, t);
        string_freez(t);
    }
    rrdhost_stream_parents_update_from_destination(host);

    // gracefully swap api_key
    if(host->stream.snd.api_key != api_key) {
        STRING *t = string_dup(api_key);
        SWAP(host->stream.snd.api_key, t);
        string_freez(t);
    }

    // gracefully swap send_charts_matching
    {
        SIMPLE_PATTERN *t = simple_pattern_create(
            string2str(send_charts_matching), NULL, SIMPLE_PATTERN_EXACT, true);
        SWAP(host->stream.snd.charts_matching, t);
        simple_pattern_free(t);
    }

    rrdhost_option_set(host, RRDHOST_OPTION_SENDER_ENABLED);
}

void stream_sender_structures_free(struct rrdhost *host) {
    rrdhost_option_clear(host, RRDHOST_OPTION_SENDER_ENABLED);

    if (unlikely(!host->sender)) return;

    // stop a possibly running thread
    stream_sender_signal_to_stop_and_wait(host, STREAM_HANDSHAKE_SND_DISCONNECT_HOST_CLEANUP, true);
    stream_circular_buffer_destroy(host->sender->scb);
    host->sender->scb = NULL;
    waitq_destroy(&host->sender->waitq);
    stream_compressor_destroy(&host->sender->thread.compressor);

    replication_sender_cleanup(host->sender);

    __atomic_sub_fetch(&netdata_buffers_statistics.rrdhost_senders, sizeof(*host->sender), __ATOMIC_RELAXED);

    freez(host->sender);
    host->sender = NULL;

    sender_host_buffer_free(host);

    rrdhost_flag_clear(host, RRDHOST_FLAG_STREAM_SENDER_INITIALIZED);
}

void stream_sender_start_host(struct rrdhost *host) {
    internal_fatal(!rrdhost_has_stream_sender_enabled(host),
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

    stream_sender_lock(host->sender);

    if(rrdhost_flag_check(host, RRDHOST_FLAG_STREAM_SENDER_ADDED)) {
        __atomic_store_n(&host->sender->exit.shutdown, true, __ATOMIC_RELAXED);
        host->sender->exit.reason = reason;
    }

    struct stream_opcode msg = host->sender->thread.msg;
    stream_sender_unlock(host->sender);

    if(reason == STREAM_HANDSHAKE_SND_DISCONNECT_HOST_CLEANUP)
        msg.opcode = STREAM_OPCODE_SENDER_STOP_HOST_CLEANUP;
    else
        msg.opcode = STREAM_OPCODE_SENDER_STOP_RECEIVER_LEFT;
    msg.reason = reason;

    stream_sender_send_opcode(host->sender, msg);

    while(wait && rrdhost_flag_check(host, RRDHOST_FLAG_STREAM_SENDER_ADDED)) {
        sleep_usec(10 * USEC_PER_MS);
        stream_connector_remove_host(host);
    }
}
