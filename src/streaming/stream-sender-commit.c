// SPDX-License-Identifier: GPL-3.0-or-later

#include "stream-thread.h"

#define SENDER_BUFFER_ADAPT_TO_TIMES_MAX_SIZE 3

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
    struct sender_op msg;

    char *src = (char *)buffer_tostring(wb);
    size_t src_len = buffer_strlen(wb);

    if (unlikely(!src || !src_len))
        return;

    size_t total_uncompressed_len = src_len;
    size_t total_compressed_len = 0;

    sender_lock(s);

    // copy the sequence number of sender buffer recreates, while having our lock
    if(commit)
        commit->sender_recreates = s->sbuf.recreates;

    if (!s->thread.msg.session) {
        // the dispatcher is not there anymore - ignore these data
        sender_unlock(s);
        if(commit)
            sender_buffer_destroy(commit);
        return;
    }

    if (unlikely(s->sbuf.cb->max_size < (src_len + 1) * SENDER_BUFFER_ADAPT_TO_TIMES_MAX_SIZE)) {
        // adaptive sizing of the circular buffer is needed to get this.

        nd_log(
            NDLS_DAEMON,
            NDLP_NOTICE,
            "STREAM %s [send to %s]: max buffer size of %zu is too small "
            "for a data message of size %zu. Increasing the max buffer size "
            "to %d times the max data message size.",
            rrdhost_hostname(s->host),
            s->connected_to,
            s->sbuf.cb->max_size,
            buffer_strlen(wb) + 1,
            SENDER_BUFFER_ADAPT_TO_TIMES_MAX_SIZE);

        s->sbuf.cb->max_size = (src_len + 1) * SENDER_BUFFER_ADAPT_TO_TIMES_MAX_SIZE;
    }

#ifdef NETDATA_LOG_STREAM_SENDER
    if (type == STREAM_TRAFFIC_TYPE_METADATA) {
        if (!s->stream_log_fp) {
            char filename[FILENAME_MAX + 1];
            snprintfz(
                filename, FILENAME_MAX, "/tmp/stream-sender-%s.txt", s->host ? rrdhost_hostname(s->host) : "unknown");

            s->stream_log_fp = fopen(filename, "w");
        }

        fprintf(
            s->stream_log_fp,
            "\n--- SEND MESSAGE START: %s ----\n"
            "%s"
            "--- SEND MESSAGE END ----------------------------------------\n",
            rrdhost_hostname(s->host),
            src);
    }
#endif

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
            size_t dst_len = rrdpush_compress(&s->compressor, src, size_to_compress, &dst);
            if (!dst_len) {
                nd_log(NDLS_DAEMON, NDLP_ERR,
                    "STREAM %s [send to %s]: COMPRESSION failed. Resetting compressor and re-trying",
                    rrdhost_hostname(s->host), s->connected_to);

                rrdpush_compression_initialize(s);
                dst_len = rrdpush_compress(&s->compressor, src, size_to_compress, &dst);
                if (!dst_len)
                    goto compression_failed_with_lock;
            }

            rrdpush_signature_t signature = rrdpush_compress_encode_signature(dst_len);

#ifdef NETDATA_INTERNAL_CHECKS
            // check if reversing the signature provides the same length
            size_t decoded_dst_len = rrdpush_decompress_decode_signature((const char *)&signature, sizeof(signature));
            if (decoded_dst_len != dst_len)
                fatal(
                    "RRDPUSH COMPRESSION: invalid signature, original payload %zu bytes, "
                    "compressed payload length %zu bytes, but signature says payload is %zu bytes",
                    size_to_compress, dst_len, decoded_dst_len);
#endif

            total_compressed_len += dst_len + sizeof(signature);

            if (cbuffer_add_unsafe(s->sbuf.cb, (const char *)&signature, sizeof(signature)) ||
                cbuffer_add_unsafe(s->sbuf.cb, dst, dst_len))
                goto overflow_with_lock;

            src = src + size_to_compress;
            src_len -= size_to_compress;
        }
    }
    else {
        // uncompressed traffic

        total_compressed_len = src_len;

        if (cbuffer_add_unsafe(s->sbuf.cb, src, src_len))
            goto overflow_with_lock;
    }

    // update s->dispatcher entries
    bool enable_sending = s->thread.bytes_outstanding == 0;
    stream_sender_thread_data_added_data_unsafe(s, type, total_compressed_len, total_uncompressed_len);

    if (enable_sending)
        msg = s->thread.msg;

    sender_unlock(s);

    if (enable_sending) {
        msg.op = SENDER_MSG_ENABLE_SENDING;
        stream_sender_send_msg_to_dispatcher(s, msg);
    }

    return;

overflow_with_lock: {
        size_t buffer_size = s->sbuf.cb->size;
        size_t buffer_max_size = s->sbuf.cb->max_size;
        size_t buffer_available = cbuffer_available_size_unsafe(s->sbuf.cb);
        msg = s->thread.msg;
        sender_unlock(s);
        msg.op = SENDER_MSG_RECONNECT_OVERFLOW;
        stream_sender_send_msg_to_dispatcher(s, msg);
        nd_log(NDLS_DAEMON, NDLP_ERR,
               "STREAM %s [send to %s]: buffer overflow while adding %zu bytes (buffer size %zu, max size %zu, available %zu). "
               "Restarting connection.",
               rrdhost_hostname(s->host), s->connected_to,
               total_compressed_len, buffer_size, buffer_max_size, buffer_available);
        return;
    }

compression_failed_with_lock: {
        rrdpush_compression_deactivate(s);
        msg = s->thread.msg;
        sender_unlock(s);
        msg.op = SENDER_MSG_RECONNECT_WITHOUT_COMPRESSION;
        stream_sender_send_msg_to_dispatcher(s, msg);
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
