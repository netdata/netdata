// SPDX-License-Identifier: GPL-3.0-or-later

#include "rrdpush.h"

#ifdef ENABLE_RRDPUSH_COMPRESSION
#include "lz4.h"

#define STREAM_COMPRESSION_MSG "STREAM_COMPRESSION"

/*
 * Reset compressor state for a new stream
 */
void rrdpush_compressor_reset(struct compressor_state *state) {
    if(!state->initialized) {
        state->initialized = true;

        state->stream.lz4_stream = LZ4_createStream();
        state->stream.input_ring_buffer_size = LZ4_DECODER_RING_BUFFER_SIZE(COMPRESSION_MAX_MSG_SIZE * 2);
        state->stream.input_ring_buffer = callocz(1, state->stream.input_ring_buffer_size);
        state->compression_result_buffer_size = 0;
    }

    LZ4_resetStream_fast(state->stream.lz4_stream);

    state->stream.input_ring_buffer_pos = 0;
}

/*
 * Destroy compressor state and all related data
 */
void rrdpush_compressor_destroy(struct compressor_state *state) {
    if (state->stream.lz4_stream) {
        LZ4_freeStream(state->stream.lz4_stream);
        state->stream.lz4_stream = NULL;
    }

    freez(state->stream.input_ring_buffer);
    state->stream.input_ring_buffer = NULL;

    freez(state->compression_result_buffer);
    state->compression_result_buffer = NULL;

    state->initialized = false;
}

/*
 * Compress the given block of data
 * Compressed data will remain in the internal buffer until the next invocation
 * Return the size of compressed data block as result and the pointer to internal buffer using the last argument
 * or 0 in case of error
 */
size_t rrdpush_compress(struct compressor_state *state, const char *data, size_t size, char **out) {
    if(unlikely(!state || !size || !out))
        return 0;

    if(unlikely(size > COMPRESSION_MAX_MSG_SIZE)) {
        netdata_log_error("RRDPUSH COMPRESS: Compression Failed - Message size %lu above compression buffer limit: %d",
              (long unsigned int)size, COMPRESSION_MAX_MSG_SIZE);
        return 0;
    }

    size_t max_dst_size = LZ4_COMPRESSBOUND(size);
    size_t data_size = max_dst_size + RRDPUSH_COMPRESSION_SIGNATURE_SIZE;

    if (!state->compression_result_buffer) {
        state->compression_result_buffer = mallocz(data_size);
        state->compression_result_buffer_size = data_size;
    }
    else if(unlikely(state->compression_result_buffer_size < data_size)) {
        state->compression_result_buffer = reallocz(state->compression_result_buffer, data_size);
        state->compression_result_buffer_size = data_size;
    }

    // the ring buffer always has space for LZ4_MAX_MSG_SIZE
    memcpy(state->stream.input_ring_buffer + state->stream.input_ring_buffer_pos, data, size);

    // this call needs the last 64K of our previous data
    // they are available in the ring buffer
    long int compressed_data_size = LZ4_compress_fast_continue(
        state->stream.lz4_stream,
        state->stream.input_ring_buffer + state->stream.input_ring_buffer_pos,
        state->compression_result_buffer + RRDPUSH_COMPRESSION_SIGNATURE_SIZE,
        (int)size,
        (int)max_dst_size,
        1);

    if (compressed_data_size < 0) {
        netdata_log_error("Data compression error: %ld", compressed_data_size);
        return 0;
    }

    // update the next writing position of the ring buffer
    state->stream.input_ring_buffer_pos += size;
    if(unlikely(state->stream.input_ring_buffer_pos >= state->stream.input_ring_buffer_size - COMPRESSION_MAX_MSG_SIZE))
        state->stream.input_ring_buffer_pos = 0;

    // update the signature header
    uint32_t len = ((compressed_data_size & 0x7f) | 0x80 | (((compressed_data_size & (0x7f << 7)) << 1) | 0x8000)) << 8;
    *(uint32_t *)state->compression_result_buffer = len | RRDPUSH_COMPRESSION_SIGNATURE;
    *out = state->compression_result_buffer;
    netdata_log_debug(D_STREAM, "%s: Compressed data header: %ld", STREAM_COMPRESSION_MSG, compressed_data_size);
    return compressed_data_size + RRDPUSH_COMPRESSION_SIGNATURE_SIZE;
}

/*
 * Decompress the compressed data in the internal buffer
 * Return the size of uncompressed data or 0 for error
 */
size_t rrdpush_decompress(struct decompressor_state *state, const char *compressed_data, size_t compressed_size) {
    if (unlikely(!state || !compressed_data || !compressed_size))
        return 0;

    if(unlikely(state->stream.read_at != state->stream.write_at))
        fatal("RRDPUSH_DECOMPRESS: asked to decompress new data, while there are unread data in the decompression buffer!");

    if (unlikely(state->stream.write_at >= state->stream.size / 2)) {
        state->stream.write_at = 0;
        state->stream.read_at = 0;
    }

    long int decompressed_size = LZ4_decompress_safe_continue(
            state->stream.lz4_stream
            , compressed_data
            , state->stream.buffer + state->stream.write_at
            , (int)compressed_size
            , (int)(state->stream.size - state->stream.write_at)
            );

    if (unlikely(decompressed_size < 0)) {
        netdata_log_error("RRDPUSH DECOMPRESS: decompressor returned negative decompressed bytes: %ld", decompressed_size);
        return 0;
    }

    if(unlikely(decompressed_size + state->stream.write_at > state->stream.size))
        fatal("RRDPUSH DECOMPRESS: decompressor overflown the stream_buffer. size: %zu, pos: %zu, added: %ld, "
              "exceeding the buffer by %zu"
              , state->stream.size
              , state->stream.write_at
              , decompressed_size
              , (size_t)(state->stream.write_at + decompressed_size - state->stream.size)
              );

    state->stream.write_at += decompressed_size;

    // statistics
    state->total_compressed += compressed_size + RRDPUSH_COMPRESSION_SIGNATURE_SIZE;
    state->total_uncompressed += decompressed_size;
    state->packet_count++;

    return decompressed_size;
}

void rrdpush_decompressor_reset(struct decompressor_state *state) {
    if(!state->initialized) {
        state->initialized = true;
        state->stream.lz4_stream = LZ4_createStreamDecode();
        state->stream.size = LZ4_decoderRingBufferSize(COMPRESSION_MAX_MSG_SIZE) * 2;
        state->stream.buffer = mallocz(state->stream.size);
    }

    LZ4_setStreamDecode(state->stream.lz4_stream, NULL, 0);

    state->signature_size = RRDPUSH_COMPRESSION_SIGNATURE_SIZE;
    state->stream.write_at = 0;
    state->stream.read_at = 0;
}

void rrdpush_decompressor_destroy(struct decompressor_state *state) {
    if(unlikely(!state->initialized))
        return;

    if (state->stream.lz4_stream) {
        LZ4_freeStreamDecode(state->stream.lz4_stream);
        state->stream.lz4_stream = NULL;
    }

    freez(state->stream.buffer);
    state->stream.buffer = NULL;

    state->initialized = false;
}

#endif
