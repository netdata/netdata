// SPDX-License-Identifier: GPL-3.0-or-later

#include "stream-thread.h"
#include "stream-replication-sender.h"

static __thread struct sender_buffer commit___thread = { 0 };

void sender_buffer_destroy(struct sender_buffer *commit) {
    buffer_free(commit->wb);
    commit->wb = NULL;
    commit->used = false;
    commit->reused = 0;
    commit->our_recreates = 0;
    commit->sender_recreates = 0;
    commit->last_function = NULL;
}

void sender_thread_buffer_free(void) {
    sender_buffer_destroy(&commit___thread);
}

void sender_host_buffer_free(RRDHOST *host) {
    sender_buffer_destroy(&host->stream.snd.commit);
}

// Collector thread starting a transmission
static BUFFER *sender_commit_start_with_trace(struct sender_state *s, struct sender_buffer *commit, size_t default_size, const char *func) {
    if(unlikely(commit->used))
        fatal("STREAM SND '%s' [to %s]: thread buffer is used multiple times concurrently (%u). "
              "It is already being used by '%s()', and now is called by '%s()'",
              rrdhost_hostname(s->host), s->remote_ip,
              (unsigned)commit->used,
              commit->last_function ? commit->last_function : "(null)",
              func ? func : "(null)");

    if(unlikely(commit->receiver_tid && commit->receiver_tid != gettid_cached()))
        fatal("STREAM SND '%s' [to %s]: thread buffer is reserved for tid %d, but it used by thread %d function '%s()'.",
              rrdhost_hostname(s->host), s->remote_ip,
              commit->receiver_tid, gettid_cached(), func ? func : "(null)");

    if(unlikely(commit->wb &&
                 commit->wb->size > default_size &&
                 commit->our_recreates != commit->sender_recreates)) {
        buffer_free(commit->wb);
        commit->wb = NULL;
    }

    if(unlikely(!commit->wb)) {
        commit->wb = buffer_create(default_size, &netdata_buffers_statistics.buffers_streaming);
        commit->our_recreates = commit->sender_recreates;
    }

    commit->used = true;

    if(!commit->reused)
        buffer_flush(commit->wb);

    return commit->wb;
}

BUFFER *sender_thread_buffer_with_trace(struct sender_state *s, size_t default_size, const char *func) {
    return sender_commit_start_with_trace(s, &commit___thread, default_size, func);
}

BUFFER *sender_host_buffer_with_trace(struct rrdhost *host, const char *func) {
    return sender_commit_start_with_trace(host->sender, &host->stream.snd.commit, HOST_THREAD_BUFFER_INITIAL_SIZE, func);
}

// Collector thread finishing a transmission
void sender_buffer_commit(struct sender_state *s, BUFFER *wb, struct sender_buffer *commit, STREAM_TRAFFIC_TYPE type) {
    struct stream_opcode msg;

    char *src = (char *)buffer_tostring(wb);
    size_t src_len = buffer_strlen(wb);

    if (unlikely(!src || !src_len))
        return;

    waitq_acquire(&s->waitq, (rrdhost_is_this_a_stream_thread(s->host)) ? WAITQ_PRIO_HIGH : WAITQ_PRIO_NORMAL);
    stream_sender_lock(s);

    // copy the sequence number of sender buffer recreates, while having our lock
    STREAM_CIRCULAR_BUFFER_STATS *stats = stream_circular_buffer_stats_unsafe(s->scb);
    if(commit)
        commit->sender_recreates = stats->recreates;

    if (!s->thread.msg.session) {
        // the dispatcher is not there anymore - ignore these data

        if(commit)
            sender_buffer_destroy(commit);

        stream_sender_unlock(s);
        waitq_release(&s->waitq);
        return;
    }

    if (unlikely(stream_circular_buffer_set_max_size_unsafe(
            s->scb, src_len * STREAM_CIRCULAR_BUFFER_ADAPT_TO_TIMES_MAX_SIZE, false))) {
        // adaptive sizing of the circular buffer
        nd_log(NDLS_DAEMON, NDLP_NOTICE,
               "STREAM SND '%s' [to %s]: Increased max buffer size to %u (message size %zu).",
               rrdhost_hostname(s->host), s->remote_ip, stats->bytes_max_size, src_len + 1);
    }

    stream_sender_log_payload(s, wb, type, false);

    // if there are data already in the buffer, we don't need to send an opcode
    bool enable_sending = stats->bytes_outstanding == 0;

    if (s->thread.compressor.initialized) {
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
            size_t dst_len = stream_compress(&s->thread.compressor, src, size_to_compress, &dst);
            if (!dst_len) {
                nd_log(NDLS_DAEMON, NDLP_ERR,
                    "STREAM SND '%s' [to %s]: COMPRESSION failed. Resetting compressor and re-trying",
                    rrdhost_hostname(s->host), s->remote_ip);

                stream_compression_initialize(s);
                dst_len = stream_compress(&s->thread.compressor, src, size_to_compress, &dst);
                if (!dst_len)
                    goto compression_failed_with_lock;
            }

            stream_compression_signature_t signature = stream_compress_encode_signature(dst_len);

#ifdef NETDATA_INTERNAL_CHECKS
            // check if reversing the signature provides the same length
            size_t decoded_dst_len = stream_decompress_decode_signature((const char *)&signature, sizeof(signature));
            if (decoded_dst_len != dst_len)
                fatal(
                    "STREAM SND '%s' [to %s]: invalid signature, original payload %zu bytes, "
                    "compressed payload length %zu bytes, but signature says payload is %zu bytes",
                    rrdhost_hostname(s->host), s->remote_ip,
                    size_to_compress, dst_len, decoded_dst_len);
#endif

            if (!stream_circular_buffer_add_unsafe(s->scb, (const char *)&signature, sizeof(signature),
                                                   sizeof(signature), type, false) ||
                !stream_circular_buffer_add_unsafe(s->scb, dst, dst_len,
                                                   size_to_compress, type, false))
                goto overflow_with_lock;

            src = src + size_to_compress;
            src_len -= size_to_compress;
        }
    }
    else {
        // uncompressed traffic

        if (!stream_circular_buffer_add_unsafe(s->scb, src, src_len,
                                               src_len, type, false))
            goto overflow_with_lock;
    }

    replication_sender_recalculate_buffer_used_ratio_unsafe(s);

    if (enable_sending)
        msg = s->thread.msg;

    stream_sender_unlock(s);
    waitq_release(&s->waitq);

    if (enable_sending) {
        msg.opcode = STREAM_OPCODE_SENDER_POLLOUT;
        msg.reason = 0;
        stream_sender_send_opcode(s, msg);
    }

    return;

overflow_with_lock: {
        msg = s->thread.msg;
        stream_sender_unlock(s);
        waitq_release(&s->waitq);
        msg.opcode = STREAM_OPCODE_SENDER_BUFFER_OVERFLOW;
        msg.reason = STREAM_HANDSHAKE_DISCONNECT_BUFFER_OVERFLOW;
        stream_sender_send_opcode(s, msg);
        nd_log_limit_static_global_var(erl, 1, 0);
        nd_log_limit(&erl, NDLS_DAEMON, NDLP_ERR,
                     "STREAM SND '%s' [to %s]: buffer overflow (buffer size %u, max size %u, available %u). "
                     "Restarting connection.",
                     rrdhost_hostname(s->host), s->remote_ip,
                     stats->bytes_size, stats->bytes_max_size, stats->bytes_available);
        return;
    }

compression_failed_with_lock: {
        stream_compression_deactivate(s);
        msg = s->thread.msg;
        stream_sender_unlock(s);
        waitq_release(&s->waitq);
        msg.opcode = STREAM_OPCODE_SENDER_RECONNECT_WITHOUT_COMPRESSION;
        msg.reason = STREAM_HANDSHAKE_SND_DISCONNECT_COMPRESSION_FAILED;
        stream_sender_send_opcode(s, msg);
        nd_log_limit_static_global_var(erl, 1, 0);
        nd_log_limit(&erl, NDLS_DAEMON, NDLP_ERR,
                     "STREAM SND '%s' [to %s]: COMPRESSION failed (twice). "
                     "Deactivating compression and restarting connection.",
                     rrdhost_hostname(s->host), s->remote_ip);
    }
}

void sender_thread_commit_with_trace(struct sender_state *s, BUFFER *wb, STREAM_TRAFFIC_TYPE type, const char *func) {
    struct sender_buffer *commit;
    bool is_receiver;

    if(unlikely(wb == commit___thread.wb)) {
        commit = &commit___thread;
        is_receiver = false;
    }
    else {
        commit = &s->host->stream.snd.commit;
        is_receiver = commit->receiver_tid == gettid_cached();
    }

    if (unlikely(wb != commit->wb))
        fatal("STREAM SND '%s' [to %s]: function '%s()' is trying to commit an unknown commit buffer.",
              rrdhost_hostname(s->host), s->remote_ip, func);

    if (unlikely(!commit->used))
        fatal("STREAM SND '%s' [to %s]: function '%s()' is committing a sender buffer twice.",
              rrdhost_hostname(s->host), s->remote_ip, func);

    if(!is_receiver ||
        type != STREAM_TRAFFIC_TYPE_DATA ||
        commit->reused >= 100 ||
        buffer_strlen(wb) >= COMPRESSION_MAX_MSG_SIZE * 2 / 3) {
        sender_buffer_commit(s, wb, commit, type);
        commit->reused = 0;
    }
    else
        commit->reused++;

    commit->used = false;
    commit->last_function = NULL;
}
