// SPDX-License-Identifier: GPL-3.0-or-later

#include "stream-thread.h"
#include "replication.h"

static __thread struct sender_buffer commit___thread = { 0 };

void sender_buffer_destroy(struct sender_buffer *commit) {
    buffer_free(commit->wb);
    commit->wb = NULL;
    commit->used = false;
    commit->our_recreates = 0;
    commit->sender_recreates = 0;
    commit->last_function = NULL;
}

void sender_commit_thread_buffer_free(void) {
    sender_buffer_destroy(&commit___thread);
}

// Collector thread starting a transmission
BUFFER *sender_commit_start_with_trace(struct sender_state *s __maybe_unused, struct sender_buffer *commit, const char *func) {
    if(unlikely(commit->used))
        fatal("STREAMING: thread buffer is used multiple times concurrently (%u). "
              "It is already being used by '%s()', and now is called by '%s()'",
              (unsigned)commit->used,
              commit->last_function ? commit->last_function : "(null)",
              func ? func : "(null)");

    if(unlikely(commit->receiver_tid && commit->receiver_tid != gettid_cached()))
        fatal("STREAMING: thread buffer is reserved for tid %d, but it used by thread %d function '%s()'.",
              commit->receiver_tid, gettid_cached(), func ? func : "(null)");

    if(unlikely(commit->wb &&
                 commit->wb->size > THREAD_BUFFER_INITIAL_SIZE &&
                 commit->our_recreates != commit->sender_recreates)) {
        buffer_free(commit->wb);
        commit->wb = NULL;
    }

    if(unlikely(!commit->wb)) {
        commit->wb = buffer_create(THREAD_BUFFER_INITIAL_SIZE, &netdata_buffers_statistics.buffers_streaming);
        commit->our_recreates = commit->sender_recreates;
    }

    commit->used = true;
    buffer_flush(commit->wb);
    return commit->wb;
}

BUFFER *sender_thread_buffer_with_trace(struct sender_state *s __maybe_unused, const char *func) {
    return sender_commit_start_with_trace(s, &commit___thread, func);
}

BUFFER *sender_host_buffer_with_trace(struct rrdhost *host, const char *func) {
    return sender_commit_start_with_trace(host->sender, &host->stream.snd.commit, func);
}

// Collector thread finishing a transmission
void sender_buffer_commit(struct sender_state *s, BUFFER *wb, struct sender_buffer *commit, STREAM_TRAFFIC_TYPE type) {
    struct stream_opcode msg;

    char *src = (char *)buffer_tostring(wb);
    size_t src_len = buffer_strlen(wb);

    if (unlikely(!src || !src_len))
        return;

    stream_sender_lock(s);

    // copy the sequence number of sender buffer recreates, while having our lock
    STREAM_CIRCULAR_BUFFER_STATS *stats = stream_circular_buffer_stats_unsafe(s->scb);
    if(commit)
        commit->sender_recreates = stats->recreates;

    if (!s->thread.msg.session) {
        // the dispatcher is not there anymore - ignore these data
        stream_sender_unlock(s);
        if(commit)
            sender_buffer_destroy(commit);
        return;
    }

    if (unlikely(stream_circular_buffer_set_max_size_unsafe(s->scb, src_len, false))) {
        // adaptive sizing of the circular buffer
        nd_log(NDLS_DAEMON, NDLP_NOTICE,
               "STREAM SEND %s [to %s]: Increased max buffer size to %u (message size %zu).",
               rrdhost_hostname(s->host), s->connected_to, stats->bytes_max_size, buffer_strlen(wb) + 1);
    }

    stream_sender_log_payload(s, wb, type, false);

    if (s->compressor.initialized) {
        // compressed traffic
        if(rrdhost_is_this_a_stream_thread(s->host))
            worker_is_busy(WORKER_STREAM_JOB_COMPRESS);

        while (src_len) {
            size_t size_to_compress = src_len;

            if (unlikely(size_to_compress > COMPRESSION_MAX_MSG_SIZE)) {
                if (stream_has_capability(s, STREAM_CAP_BINARY))
                    size_to_compress = COMPRESSION_MAX_MSG_SIZE;
                else {
                    if (size_to_compress > COMPRESSION_MAX_MSG_SIZE) {
                        // we need to find the last newline
                        // so that the decompressor will have a whole line to work with

                        const char *t = &src[COMPRESSION_MAX_MSG_SIZE];
                        while (--t >= src)
                            if (unlikely(*t == '\n'))
                                break;

                        if (t <= src)
                            size_to_compress = COMPRESSION_MAX_MSG_SIZE;
                        else
                            size_to_compress = t - src + 1;
                    }
                }
            }

            const char *dst;
            size_t dst_len = stream_compress(&s->compressor, src, size_to_compress, &dst);
            if (!dst_len) {
                nd_log(NDLS_DAEMON, NDLP_ERR,
                    "STREAM %s [send to %s]: COMPRESSION failed. Resetting compressor and re-trying",
                    rrdhost_hostname(s->host), s->connected_to);

                stream_compression_initialize(s);
                dst_len = stream_compress(&s->compressor, src, size_to_compress, &dst);
                if (!dst_len)
                    goto compression_failed_with_lock;
            }

            stream_compression_signature_t signature = stream_compress_encode_signature(dst_len);

#ifdef NETDATA_INTERNAL_CHECKS
            // check if reversing the signature provides the same length
            size_t decoded_dst_len = stream_decompress_decode_signature((const char *)&signature, sizeof(signature));
            if (decoded_dst_len != dst_len)
                fatal(
                    "STREAM COMPRESSION: invalid signature, original payload %zu bytes, "
                    "compressed payload length %zu bytes, but signature says payload is %zu bytes",
                    size_to_compress, dst_len, decoded_dst_len);
#endif

            if (!stream_circular_buffer_add_unsafe(s->scb, (const char *)&signature, sizeof(signature), sizeof(signature), type) ||
                !stream_circular_buffer_add_unsafe(s->scb, dst, dst_len, size_to_compress, type))
                goto overflow_with_lock;

            src = src + size_to_compress;
            src_len -= size_to_compress;
        }
    }
    else {
        // uncompressed traffic

        if (!stream_circular_buffer_add_unsafe(s->scb, src, src_len, src_len, type))
            goto overflow_with_lock;
    }

    bool enable_sending = stats->bytes_outstanding == 0;
    replication_recalculate_buffer_used_ratio_unsafe(s);

    if (enable_sending)
        msg = s->thread.msg;

    stream_sender_unlock(s);

    if (enable_sending) {
        msg.opcode = STREAM_OPCODE_SENDER_POLLOUT;
        stream_sender_send_opcode(s, msg);
    }

    return;

overflow_with_lock: {
        msg = s->thread.msg;
        stream_sender_unlock(s);
        msg.opcode = STREAM_OPCODE_SENDER_BUFFER_OVERFLOW;
        stream_sender_send_opcode(s, msg);
        nd_log(NDLS_DAEMON, NDLP_ERR,
               "STREAM %s [send to %s]: buffer overflow (buffer size %u, max size %u, used %u, available %u). "
               "Restarting connection.",
               rrdhost_hostname(s->host), s->connected_to,
               stats->bytes_size, stats->bytes_max_size, stats->bytes_outstanding, stats->bytes_available);
        return;
    }

compression_failed_with_lock: {
        stream_compression_deactivate(s);
        msg = s->thread.msg;
        stream_sender_unlock(s);
        msg.opcode = STREAM_OPCODE_SENDER_RECONNECT_WITHOUT_COMPRESSION;
        stream_sender_send_opcode(s, msg);
        nd_log(NDLS_DAEMON, NDLP_ERR,
               "STREAM %s [send to %s]: COMPRESSION failed (twice). Deactivating compression and restarting connection.",
               rrdhost_hostname(s->host), s->connected_to);
    }
}

void sender_thread_commit(struct sender_state *s, BUFFER *wb, STREAM_TRAFFIC_TYPE type, const char *func) {
    struct sender_buffer *commit = (wb == commit___thread.wb) ? & commit___thread : &s->host->stream.snd.commit;

    if (unlikely(wb != commit->wb))
        fatal("STREAMING: function '%s()' is trying to commit an unknown commit buffer.", func);

    if (unlikely(!commit->used))
        fatal("STREAMING: function '%s()' is committing a sender buffer twice.", func);

    commit->used = false;
    commit->last_function = NULL;

    sender_buffer_commit(s, wb, commit, type);
}
