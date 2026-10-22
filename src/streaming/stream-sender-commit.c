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

    pid_t receiver_tid = __atomic_load_n(&commit->receiver_tid, __ATOMIC_RELAXED);
    if(unlikely(receiver_tid && receiver_tid != gettid_cached()))
        fatal("STREAM SND '%s' [to %s]: thread buffer is reserved for tid %d, but it used by thread %d function '%s()'.",
              rrdhost_hostname(s->host), s->remote_ip,
              receiver_tid, gettid_cached(), func ? func : "(null)");

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

// compress src into the circular buffer as one or more framed messages
static SENDER_COMPRESS_RC sender_compress_to_scb_unsafe(struct sender_state *s, const char *src, size_t src_len, STREAM_TRAFFIC_TYPE type) {
    while (src_len) {
        size_t size_to_compress = src_len;

        if (unlikely(size_to_compress > COMPRESSION_MAX_MSG_SIZE)) {
            if (stream_has_capability(s, STREAM_CAP_BINARY))
                size_to_compress = COMPRESSION_MAX_MSG_SIZE;
            else {
                // we need to find the last newline
                // so that the decompressor will have a whole line to work with

                const char *t = &src[COMPRESSION_MAX_MSG_SIZE];
                while (t > src) {
                    t--;
                    if (unlikely(*t == '\n'))
                        break;
                }

                if (t <= src)
                    size_to_compress = COMPRESSION_MAX_MSG_SIZE;
                else
                    size_to_compress = t - src + 1;
            }
        }

        const char *dst;
        size_t dst_len = stream_compress(&s->thread.compressor, src, size_to_compress, &dst);
        if (!dst_len) {
            nd_log(NDLS_DAEMON, NDLP_ERR,
                "STREAM SND '%s' [to %s]: COMPRESSION failed. Resetting compressor and re-trying",
                rrdhost_hostname(s->host), s->remote_ip);

            if (!stream_compression_initialize(s))
                return SENDER_COMPRESS_FAILED;

            dst_len = stream_compress(&s->thread.compressor, src, size_to_compress, &dst);
            if (!dst_len)
                return SENDER_COMPRESS_FAILED;
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
            return SENDER_COMPRESS_OVERFLOW;

        src = src + size_to_compress;
        src_len -= size_to_compress;
    }

    return SENDER_COMPRESS_OK;
}

// Compress all the text committed since the last call into as few frames as possible.
// Committers only queue text (see sender_buffer_commit()), so a frame carries many chart updates
// instead of one, which compresses better and costs one compression call instead of one per commit.
SENDER_COMPRESS_RC sender_pending_compress_unsafe(struct sender_state *s) {
    BUFFER *wb = s->thread.pending.wb;
    if (!wb || !buffer_strlen(wb))
        return SENDER_COMPRESS_OK;

    SENDER_COMPRESS_RC rc = sender_compress_to_scb_unsafe(s, buffer_tostring(wb), buffer_strlen(wb), s->thread.pending.type);
    buffer_flush(wb);
    return rc;
}

bool sender_pending_has_data_unsafe(struct sender_state *s) {
    return s->thread.pending.wb && buffer_strlen(s->thread.pending.wb);
}

void sender_pending_flush_unsafe(struct sender_state *s) {
    if (s->thread.pending.wb)
        buffer_flush(s->thread.pending.wb);
}

void sender_pending_free(struct sender_state *s) {
    buffer_free(s->thread.pending.wb);
    s->thread.pending.wb = NULL;
}

// Queue a committed message for compression. The queue is compressed here only when needed
// to keep it at about one frame; otherwise the dispatcher compresses it before sending.
SENDER_COMPRESS_RC sender_pending_add_unsafe(struct sender_state *s, const char *src, size_t src_len, STREAM_TRAFFIC_TYPE type) {
    SENDER_COMPRESS_RC rc;

    // the circular buffer accounts bytes per traffic type, so a frame carries one type only
    if (unlikely(sender_pending_has_data_unsafe(s) && s->thread.pending.type != type)) {
        rc = sender_pending_compress_unsafe(s);
        if (rc != SENDER_COMPRESS_OK)
            return rc;
    }

    if (unlikely(src_len >= SENDER_PENDING_MAX_SIZE)) {
        // large messages (like function responses) go to the circular buffer directly, after
        // whatever is already queued, so the queue never grows (and stays allocated) beyond a frame
        rc = sender_pending_compress_unsafe(s);
        if (rc != SENDER_COMPRESS_OK)
            return rc;

        return sender_compress_to_scb_unsafe(s, src, src_len, type);
    }

    if (unlikely(!s->thread.pending.wb))
        s->thread.pending.wb = buffer_create(COMPRESSION_MAX_MSG_SIZE, &netdata_buffers_statistics.buffers_streaming);

    s->thread.pending.type = type;
    buffer_memcat(s->thread.pending.wb, src, src_len);

    if (buffer_strlen(s->thread.pending.wb) >= SENDER_PENDING_MAX_SIZE)
        return sender_pending_compress_unsafe(s);

    return SENDER_COMPRESS_OK;
}

// Report a failure of sender_pending_compress_unsafe() (or an overflow of uncompressed data) to the
// dispatcher and release the sender lock and waitq. The dispatcher restarts the connection.
void sender_compress_failed_and_unlock(struct sender_state *s, SENDER_COMPRESS_RC rc) {
    struct stream_opcode msg;

    if (rc == SENDER_COMPRESS_OVERFLOW) {
        STREAM_CIRCULAR_BUFFER_STATS stats_snapshot = *stream_circular_buffer_stats_unsafe(s->scb);
        msg = s->thread.msg;
        stream_sender_unlock(s);
        waitq_release(&s->waitq);
        msg.opcode = STREAM_OPCODE_SENDER_BUFFER_OVERFLOW;
        msg.reason = STREAM_HANDSHAKE_DISCONNECT_BUFFER_OVERFLOW;
        stream_sender_send_opcode(s, msg);
        nd_log_limit_static_global_var(erl, 1, 0);
        nd_log_limit(&erl, NDLS_DAEMON, NDLP_ERR,
                     "STREAM SND '%s' [to %s]: buffer overflow (buffer size %zu, max size %zu, available %zu). "
                     "Restarting connection.",
                     rrdhost_hostname(s->host), s->remote_ip,
                     stats_snapshot.bytes_size, stats_snapshot.bytes_max_size, stats_snapshot.bytes_available);
    }
    else {
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
               "STREAM SND '%s' [to %s]: Increased max buffer size to %zu (message size %zu).",
               rrdhost_hostname(s->host), s->remote_ip, stats->bytes_max_size, src_len + 1);
    }

    stream_sender_log_payload(s, wb, type, false);

    // if there are data already queued, we don't need to send an opcode
    bool enable_sending = stats->bytes_outstanding == 0 && !sender_pending_has_data_unsafe(s);

    SENDER_COMPRESS_RC rc = SENDER_COMPRESS_OK;
    if (s->thread.compressor.initialized) {
        // compressed traffic: queue the text; the dispatcher compresses everything queued
        // in one frame right before sending (stream_sender_send_data())
        if(rrdhost_is_this_a_stream_thread(s->host))
            worker_is_busy(WORKER_STREAM_JOB_COMPRESS);

        rc = sender_pending_add_unsafe(s, src, src_len, type);
    }
    else {
        // uncompressed traffic

        if (!stream_circular_buffer_add_unsafe(s->scb, src, src_len,
                                               src_len, type, false))
            rc = SENDER_COMPRESS_OVERFLOW;
    }

    if (unlikely(rc != SENDER_COMPRESS_OK)) {
        sender_compress_failed_and_unlock(s, rc);
        return;
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
        is_receiver = __atomic_load_n(&commit->receiver_tid, __ATOMIC_RELAXED) == gettid_cached();
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

// ----------------------------------------------------------------------------
// unittest: queued commits compressed in batches must decompress to the exact committed text,
// in frames that fit the protocol limits and (for non-binary parents) end on a line boundary

void unittest_generate_message(BUFFER *wb, time_t now_s, size_t counter);

static int unittest_sender_pending_one(compression_algorithm_t algorithm, const char *name, STREAM_CAPABILITIES caps) {
    int errors = 0;

    struct sender_state *s = callocz(1, sizeof(*s));
    s->capabilities = caps;
    s->scb = stream_circular_buffer_create();
    s->thread.compressor.algorithm = algorithm;
    s->thread.compressor.level = stream_send.compression.levels[algorithm];
    stream_compressor_init(&s->thread.compressor);

    struct decompressor_state dctx = { .algorithm = algorithm, };
    stream_decompressor_init(&dctx);

    BUFFER *expected = buffer_create(0, NULL);
    BUFFER *got = buffer_create(0, NULL);
    BUFFER *msg = buffer_create(0, NULL);
    BUFFER *wire = buffer_create(0, NULL);
    time_t now_s = now_realtime_sec();
    size_t commits = 0, frames = 0;

    for(size_t i = 0; i < 2000; i++) {
        // a commit: one chart update, sometimes a large message spanning several frames,
        // and every few commits a different traffic type that must not share a frame with the data
        buffer_flush(msg);
        STREAM_TRAFFIC_TYPE type = STREAM_TRAFFIC_TYPE_DATA;
        if(i % 7 == 3) {
            type = STREAM_TRAFFIC_TYPE_REPLICATION;
            for(size_t l = 0; l < 1 + i % 5; l++)
                buffer_sprintf(msg, "RTEST %zu %zu\n", i, l);
        }
        else {
            size_t target = (i % 500 == 499) ? COMPRESSION_MAX_MSG_SIZE * 3 : 1 + os_random32() % 2000;
            while(buffer_strlen(msg) < target)
                unittest_generate_message(msg, now_s, i);
        }

        buffer_memcat(expected, buffer_tostring(msg), buffer_strlen(msg));
        commits++;

        // what sender_buffer_commit() does with compression enabled
        SENDER_COMPRESS_RC rc = sender_pending_add_unsafe(s, buffer_tostring(msg), buffer_strlen(msg), type);

        // what the dispatcher does whenever it wakes up
        if(rc == SENDER_COMPRESS_OK && os_random32() % 8 == 0)
            rc = sender_pending_compress_unsafe(s);

        if(rc != SENDER_COMPRESS_OK) {
            fprintf(stderr, "%s: compression failed at commit %zu\n", name, i);
            errors++;
            goto cleanup;
        }

        if(s->thread.pending.wb && buffer_strlen(s->thread.pending.wb) >= SENDER_PENDING_MAX_SIZE) {
            fprintf(stderr, "%s: the queue holds %zu bytes after commit %zu\n", name, buffer_strlen(s->thread.pending.wb), i);
            errors++;
            goto cleanup;
        }

        // drain the circular buffer, like sending does
        char *chunk;
        size_t len;
        while((len = stream_circular_buffer_get_unsafe(s->scb, &chunk))) {
            buffer_memcat(wire, chunk, len);
            stream_circular_buffer_del_unsafe(s->scb, len, now_monotonic_usec());
        }
    }

    if(sender_pending_compress_unsafe(s) != SENDER_COMPRESS_OK) {
        fprintf(stderr, "%s: final compression failed\n", name);
        errors++;
        goto cleanup;
    }

    {
        char *chunk;
        size_t len;
        while((len = stream_circular_buffer_get_unsafe(s->scb, &chunk))) {
            buffer_memcat(wire, chunk, len);
            stream_circular_buffer_del_unsafe(s->scb, len, now_monotonic_usec());
        }
    }

    // decode the frames, like the receiver does
    const char *w = buffer_tostring(wire);
    size_t wlen = buffer_strlen(wire), pos = 0;
    while(pos < wlen) {
        if(wlen - pos < STREAM_COMPRESSION_SIGNATURE_SIZE) {
            fprintf(stderr, "%s: truncated signature at byte %zu\n", name, pos);
            errors++;
            goto cleanup;
        }

        size_t clen = stream_decompressor_start(&dctx, &w[pos], STREAM_COMPRESSION_SIGNATURE_SIZE);
        pos += STREAM_COMPRESSION_SIGNATURE_SIZE;
        if(!clen || clen > wlen - pos) {
            fprintf(stderr, "%s: invalid frame length %zu at byte %zu\n", name, clen, pos);
            errors++;
            goto cleanup;
        }

        size_t dlen = stream_decompress(&dctx, &w[pos], clen);
        pos += clen;
        frames++;

        if(!dlen || dlen > COMPRESSION_MAX_MSG_SIZE) {
            fprintf(stderr, "%s: frame %zu decompressed to %zu bytes\n", name, frames, dlen);
            errors++;
            goto cleanup;
        }

        char tmp[COMPRESSION_MAX_MSG_SIZE + 1];
        size_t n = stream_decompressor_get(&dctx, tmp, sizeof(tmp));
        if(n != dlen) {
            fprintf(stderr, "%s: frame %zu returned %zu of %zu bytes\n", name, frames, n, dlen);
            errors++;
            goto cleanup;
        }

        tmp[n] = '\0';
        if(strstr(tmp, "RTEST") && strstr(tmp, PLUGINSD_KEYWORD_SET_V2)) {
            fprintf(stderr, "%s: frame %zu mixes traffic types\n", name, frames);
            errors++;
            goto cleanup;
        }

        if(!(caps & STREAM_CAP_BINARY) && tmp[n - 1] != '\n') {
            fprintf(stderr, "%s: frame %zu does not end on a line boundary\n", name, frames);
            errors++;
            goto cleanup;
        }

        buffer_memcat(got, tmp, n);
    }

    if(buffer_strlen(got) != buffer_strlen(expected) ||
        memcmp(buffer_tostring(got), buffer_tostring(expected), buffer_strlen(expected)) != 0) {
        fprintf(stderr, "%s: decompressed %zu bytes do not match the %zu committed bytes\n",
                name, buffer_strlen(got), buffer_strlen(expected));
        errors++;
    }
    else if(frames >= commits) {
        fprintf(stderr, "%s: %zu frames for %zu commits - nothing was batched\n", name, frames, commits);
        errors++;
    }

cleanup:
    fprintf(stderr, "Sender batching with %s%s: %s (%zu commits, %zu frames, %zu raw bytes, %zu wire bytes)\n",
            name, (caps & STREAM_CAP_BINARY) ? " (binary)" : "", errors ? "FAILED" : "OK",
            commits, frames, buffer_strlen(expected), buffer_strlen(wire));

    buffer_free(expected);
    buffer_free(got);
    buffer_free(msg);
    buffer_free(wire);
    sender_pending_free(s);
    stream_compressor_destroy(&s->thread.compressor);
    stream_decompressor_destroy(&dctx);
    stream_circular_buffer_destroy(s->scb);
    freez(s);

    return errors;
}

int unittest_stream_sender_pending(void) {
    int errors = 0;

#ifdef ENABLE_ZSTD
    errors += unittest_sender_pending_one(COMPRESSION_ALGORITHM_ZSTD, "ZSTD", STREAM_CAP_ZSTD);
    errors += unittest_sender_pending_one(COMPRESSION_ALGORITHM_ZSTD, "ZSTD", STREAM_CAP_ZSTD | STREAM_CAP_BINARY);
#endif
#ifdef ENABLE_LZ4
    errors += unittest_sender_pending_one(COMPRESSION_ALGORITHM_LZ4, "LZ4", STREAM_CAP_LZ4);
#endif
#ifdef ENABLE_BROTLI
    errors += unittest_sender_pending_one(COMPRESSION_ALGORITHM_BROTLI, "BROTLI", STREAM_CAP_BROTLI);
#endif
    errors += unittest_sender_pending_one(COMPRESSION_ALGORITHM_GZIP, "GZIP", STREAM_CAP_GZIP);

    return errors;
}
