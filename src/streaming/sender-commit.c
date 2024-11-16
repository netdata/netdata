// SPDX-License-Identifier: GPL-3.0-or-later

#include "sender-internals.h"

#define SENDER_BUFFER_ADAPT_TO_TIMES_MAX_SIZE 3

static __thread BUFFER *sender_thread_buffer = NULL;
static __thread bool sender_thread_buffer_used = false;
static __thread size_t sender_thread_buffer_our_recreates = 0;    // the sender sequence, when we created this buffer
static __thread size_t sender_thread_buffer_sender_recreates = 0; // sender_commit() copies this, while having the sender lock

void sender_thread_buffer_free(void) {
    buffer_free(sender_thread_buffer);
    sender_thread_buffer = NULL;
    sender_thread_buffer_used = false;
    sender_thread_buffer_our_recreates = 0;
    sender_thread_buffer_sender_recreates = 0;
}

// Collector thread starting a transmission
BUFFER *sender_start(struct sender_state *s __maybe_unused) {
    if(unlikely(sender_thread_buffer_used))
        fatal("STREAMING: thread buffer is used multiple times concurrently.");

    if(unlikely(sender_thread_buffer &&
                 sender_thread_buffer->size > THREAD_BUFFER_INITIAL_SIZE &&
                 sender_thread_buffer_our_recreates != sender_thread_buffer_sender_recreates)) {
        buffer_free(sender_thread_buffer);
        sender_thread_buffer = NULL;
    }

    if(unlikely(!sender_thread_buffer)) {
        sender_thread_buffer = buffer_create(THREAD_BUFFER_INITIAL_SIZE, &netdata_buffers_statistics.buffers_streaming);
        sender_thread_buffer_our_recreates = sender_thread_buffer_sender_recreates;
    }

    sender_thread_buffer_used = true;
    buffer_flush(sender_thread_buffer);
    return sender_thread_buffer;
}

// Collector thread finishing a transmission
void sender_commit(struct sender_state *s, BUFFER *wb, STREAM_TRAFFIC_TYPE type) {
    struct sender_op msg;

    if (unlikely(wb != sender_thread_buffer))
        fatal("STREAMING: sender is trying to commit a buffer that is not this thread's buffer.");

    if (unlikely(!sender_thread_buffer_used))
        fatal("STREAMING: sender is committing a buffer twice.");

    sender_thread_buffer_used = false;

    char *src = (char *)buffer_tostring(wb);
    size_t src_len = buffer_strlen(wb);

    if (unlikely(!src || !src_len))
        return;

    size_t total_uncompressed_len = src_len;
    size_t total_compressed_len = 0;

    sender_lock(s);

    // copy the sequence number of sender buffer recreates, while having our lock
    sender_thread_buffer_sender_recreates = s->sbuf.recreates;

    if (s->dispatcher.msg.slot == 0 || s->dispatcher.msg.magic == 0) {
        // the dispatcher is not there anymore - ignore these data
        sender_unlock(s);
        sender_thread_buffer_free(); // free the thread data
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
                if (!dst_len) {
                    nd_log(NDLS_DAEMON, NDLP_ERR,
                        "STREAM %s [send to %s]: COMPRESSION failed again. Deactivating compression",
                        rrdhost_hostname(s->host), s->connected_to);

                    rrdpush_compression_deactivate(s);
                    goto compression_failed_with_lock;
                }
            }

            rrdpush_signature_t signature = rrdpush_compress_encode_signature(dst_len);

#ifdef NETDATA_INTERNAL_CHECKS
            // check if reversing the signature provides the same length
            size_t decoded_dst_len = rrdpush_decompress_decode_signature((const char *)&signature, sizeof(signature));
            if (decoded_dst_len != dst_len)
                fatal(
                    "RRDPUSH COMPRESSION: invalid signature, original payload %zu bytes, "
                    "compressed payload length %zu bytes, but signature says payload is %zu bytes",
                    size_to_compress,
                    dst_len,
                    decoded_dst_len);
#endif

            if (cbuffer_add_unsafe(s->sbuf.cb, (const char *)&signature, sizeof(signature)) ||
                cbuffer_add_unsafe(s->sbuf.cb, dst, dst_len))
                goto overflow_with_lock;

            total_compressed_len += dst_len + sizeof(signature);

            src = src + size_to_compress;
            src_len -= size_to_compress;
        }
    }
    else {
        // uncompressed traffic

        if (cbuffer_add_unsafe(s->sbuf.cb, src, src_len))
            goto overflow_with_lock;

        total_compressed_len = total_uncompressed_len;
    }

    // update s->dispatcher entries
    bool enable_sending = s->dispatcher.bytes_outstanding == 0;
    stream_sender_update_dispatcher_added_data_unsafe(s, type, total_compressed_len, total_uncompressed_len);

    if (enable_sending)
        msg = s->dispatcher.msg;

    sender_unlock(s);

    if (enable_sending) {
        msg.op = SENDER_MSG_ENABLE_SENDING;
        stream_sender_send_msg_to_dispatcher(s, msg);
    }

    return;

overflow_with_lock:
    msg = s->dispatcher.msg;
    sender_unlock(s);
    msg.op = SENDER_MSG_RECONNECT_OVERFLOW;
    stream_sender_send_msg_to_dispatcher(s, msg);
    return;

compression_failed_with_lock:
    msg = s->dispatcher.msg;
    sender_unlock(s);
    msg.op = SENDER_MSG_RECONNECT_WITHOUT_COMPRESSION;
    stream_sender_send_msg_to_dispatcher(s, msg);
    return;
}
