// SPDX-License-Identifier: GPL-3.0-or-later

#include "sender-internals.h"

static __thread BUFFER *sender_thread_buffer = NULL;
static __thread bool sender_thread_buffer_used = false;
static __thread time_t sender_thread_buffer_last_reset_s = 0;

void sender_thread_buffer_free(void) {
    buffer_free(sender_thread_buffer);
    sender_thread_buffer = NULL;
    sender_thread_buffer_used = false;
    sender_thread_buffer_last_reset_s = 0;
}

// Collector thread starting a transmission
BUFFER *sender_start(struct sender_state *s) {
    if(unlikely(sender_thread_buffer_used))
        fatal("STREAMING: thread buffer is used multiple times concurrently.");

    if(unlikely(rrdpush_sender_last_buffer_recreate_get(s) > sender_thread_buffer_last_reset_s)) {
        if(unlikely(sender_thread_buffer && sender_thread_buffer->size > THREAD_BUFFER_INITIAL_SIZE)) {
            buffer_free(sender_thread_buffer);
            sender_thread_buffer = NULL;
        }
    }

    if(unlikely(!sender_thread_buffer)) {
        sender_thread_buffer = buffer_create(THREAD_BUFFER_INITIAL_SIZE, &netdata_buffers_statistics.buffers_streaming);
        sender_thread_buffer_last_reset_s = rrdpush_sender_last_buffer_recreate_get(s);
    }

    sender_thread_buffer_used = true;
    buffer_flush(sender_thread_buffer);
    return sender_thread_buffer;
}

#define SENDER_BUFFER_ADAPT_TO_TIMES_MAX_SIZE 3

// Collector thread finishing a transmission
void sender_commit(struct sender_state *s, BUFFER *wb, STREAM_TRAFFIC_TYPE type) {

    if(unlikely(wb != sender_thread_buffer))
        fatal("STREAMING: sender is trying to commit a buffer that is not this thread's buffer.");

    if(unlikely(!sender_thread_buffer_used))
        fatal("STREAMING: sender is committing a buffer twice.");

    sender_thread_buffer_used = false;

    char *src = (char *)buffer_tostring(wb);
    size_t src_len = buffer_strlen(wb);

    if(unlikely(!src || !src_len))
        return;

    sender_lock(s);

#ifdef NETDATA_LOG_STREAM_SENDER
    if(type == STREAM_TRAFFIC_TYPE_METADATA) {
        if(!s->stream_log_fp) {
            char filename[FILENAME_MAX + 1];
            snprintfz(filename, FILENAME_MAX, "/tmp/stream-sender-%s.txt", s->host ? rrdhost_hostname(s->host) : "unknown");

            s->stream_log_fp = fopen(filename, "w");
        }

        fprintf(s->stream_log_fp, "\n--- SEND MESSAGE START: %s ----\n"
                                  "%s"
                                  "--- SEND MESSAGE END ----------------------------------------\n"
                , rrdhost_hostname(s->host), src
        );
    }
#endif

    if(unlikely(s->buffer->max_size < (src_len + 1) * SENDER_BUFFER_ADAPT_TO_TIMES_MAX_SIZE)) {
        netdata_log_info("STREAM %s [send to %s]: max buffer size of %zu is too small for a data message of size %zu. Increasing the max buffer size to %d times the max data message size.",
                         rrdhost_hostname(s->host), s->connected_to, s->buffer->max_size, buffer_strlen(wb) + 1, SENDER_BUFFER_ADAPT_TO_TIMES_MAX_SIZE);

        s->buffer->max_size = (src_len + 1) * SENDER_BUFFER_ADAPT_TO_TIMES_MAX_SIZE;
    }

    if (s->compressor.initialized) {
        while(src_len) {
            size_t size_to_compress = src_len;

            if(unlikely(size_to_compress > COMPRESSION_MAX_MSG_SIZE)) {
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

                        if (t <= src) {
                            size_to_compress = COMPRESSION_MAX_MSG_SIZE;
                        } else
                            size_to_compress = t - src + 1;
                    }
                }
            }

            const char *dst;
            size_t dst_len = rrdpush_compress(&s->compressor, src, size_to_compress, &dst);
            if (!dst_len) {
                netdata_log_error("STREAM %s [send to %s]: COMPRESSION failed. Resetting compressor and re-trying",
                                  rrdhost_hostname(s->host), s->connected_to);

                rrdpush_compression_initialize(s);
                dst_len = rrdpush_compress(&s->compressor, src, size_to_compress, &dst);
                if(!dst_len) {
                    netdata_log_error("STREAM %s [send to %s]: COMPRESSION failed again. Deactivating compression",
                                      rrdhost_hostname(s->host), s->connected_to);

                    rrdpush_compression_deactivate(s);
                    rrdpush_sender_thread_close_socket(s);
                    sender_unlock(s);
                    return;
                }
            }

            rrdpush_signature_t signature = rrdpush_compress_encode_signature(dst_len);

#ifdef NETDATA_INTERNAL_CHECKS
            // check if reversing the signature provides the same length
            size_t decoded_dst_len = rrdpush_decompress_decode_signature((const char *)&signature, sizeof(signature));
            if(decoded_dst_len != dst_len)
                fatal("RRDPUSH COMPRESSION: invalid signature, original payload %zu bytes, "
                      "compressed payload length %zu bytes, but signature says payload is %zu bytes",
                      size_to_compress, dst_len, decoded_dst_len);
#endif

            if(cbuffer_add_unsafe(s->buffer, (const char *)&signature, sizeof(signature)))
                s->flags |= SENDER_FLAG_OVERFLOW;
            else {
                if(cbuffer_add_unsafe(s->buffer, dst, dst_len))
                    s->flags |= SENDER_FLAG_OVERFLOW;
                else
                    s->sent_bytes_on_this_connection_per_type[type] += dst_len + sizeof(signature);
            }

            src = src + size_to_compress;
            src_len -= size_to_compress;
        }
    }
    else if(cbuffer_add_unsafe(s->buffer, src, src_len))
        s->flags |= SENDER_FLAG_OVERFLOW;
    else
        s->sent_bytes_on_this_connection_per_type[type] += src_len;

    replication_recalculate_buffer_used_ratio_unsafe(s);

    bool signal_sender = false;
    if(!rrdpush_sender_pipe_has_pending_data(s)) {
        rrdpush_sender_pipe_set_pending_data(s);
        signal_sender = true;
    }

    sender_unlock(s);

    if(signal_sender && (!stream_has_capability(s, STREAM_CAP_INTERPOLATED) || type != STREAM_TRAFFIC_TYPE_DATA))
        stream_sender_dispatcher_wake_up(s);
}
