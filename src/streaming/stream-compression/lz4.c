// SPDX-License-Identifier: GPL-3.0-or-later

#include "libnetdata/libnetdata.h"
#include "lz4.h"

#ifdef ENABLE_LZ4
#include <lz4.h>

// ----------------------------------------------------------------------------
// compress

void stream_compressor_init_lz4(struct compressor_state *state) {
    if(!state->initialized) {
        state->initialized = true;
        state->stream = LZ4_createStream();

        // LZ4 needs access to the last 64KB of source data
        // so, we keep twice the size of each message
        simple_ring_buffer_make_room(&state->input, 65536 + COMPRESSION_MAX_CHUNK * 2);
    }
}

void stream_compressor_destroy_lz4(struct compressor_state *state) {
    if (state->stream) {
        LZ4_freeStream(state->stream);
        state->stream = NULL;
    }
}

/*
 * Compress the given block of data
 * Compressed data will remain in the internal buffer until the next invocation
 * Return the size of compressed data block as result and the pointer to internal buffer using the last argument
 * or 0 in case of error
 */
size_t stream_compress_lz4(struct compressor_state *state, const char *data, size_t size, const char **out) {
    if(unlikely(!state || !size || !out))
        return 0;

    // we need to keep the last 64K of our previous source data
    // as they were in the ring buffer

    simple_ring_buffer_make_room(&state->output, LZ4_COMPRESSBOUND(size));

    if(state->input.write_pos + size > state->input.size)
        // the input buffer cannot fit out data, restart from zero
        simple_ring_buffer_reset(&state->input);

    simple_ring_buffer_append_data(&state->input, data, size);

    long int compressed_data_size = LZ4_compress_fast_continue(
            state->stream,
            state->input.data + state->input.read_pos,
            (char *)state->output.data,
            (int)(state->input.write_pos - state->input.read_pos),
            (int)state->output.size,
            state->level);

    if (compressed_data_size <= 0) {
        netdata_log_error("STREAM_COMPRESS: LZ4_compress_fast_continue() returned %ld "
                          "(source is %zu bytes, output buffer can fit %zu bytes)",
                compressed_data_size, size, state->output.size);
        return 0;
    }

    state->input.read_pos = state->input.write_pos;

    state->sender_locked.total_compressions++;
    state->sender_locked.total_uncompressed += size;
    state->sender_locked.total_compressed += compressed_data_size;

    *out = state->output.data;
    return compressed_data_size;
}

// ----------------------------------------------------------------------------
// decompress

void stream_decompressor_init_lz4(struct decompressor_state *state) {
    if(!state->initialized) {
        state->initialized = true;
        state->stream = LZ4_createStreamDecode();
        simple_ring_buffer_make_room(&state->output, 65536 + COMPRESSION_MAX_CHUNK * 2);
    }
}

void stream_decompressor_destroy_lz4(struct decompressor_state *state) {
    if (state->stream) {
        LZ4_freeStreamDecode(state->stream);
        state->stream = NULL;
    }
}

/*
 * Decompress the compressed data in the internal buffer
 * Return the size of uncompressed data or 0 for error
 */
size_t stream_decompress_lz4(struct decompressor_state *state, const char *compressed_data, size_t compressed_size) {
    if (unlikely(!state || !compressed_data || !compressed_size))
        return 0;

    // The state.output ring buffer is always EMPTY at this point,
    // meaning that (state->output.read_pos == state->output.write_pos)
    // However, THEY ARE NOT ZERO.

    if (unlikely(state->output.write_pos + COMPRESSION_MAX_CHUNK > state->output.size))
        // the input buffer cannot fit out data, restart from zero
        simple_ring_buffer_reset(&state->output);

    long int decompressed_size = LZ4_decompress_safe_continue(
            state->stream
            , compressed_data
            , (char *)(state->output.data + state->output.write_pos)
            , (int)compressed_size
            , (int)(state->output.size - state->output.write_pos)
            );

    if (unlikely(decompressed_size < 0)) {
        netdata_log_error("STREAM_DECOMPRESS: LZ4_decompress_safe_continue() returned negative value: %ld "
                          "(compressed chunk is %zu bytes)"
                          , decompressed_size, compressed_size);
        return 0;
    }

    if(unlikely(decompressed_size + state->output.write_pos > state->output.size))
        fatal("STREAM_DECOMPRESS: LZ4_decompress_safe_continue() overflown the stream_buffer "
              "(size: %zu, pos: %zu, added: %ld, exceeding the buffer by %zu)"
              , state->output.size
              , state->output.write_pos
              , decompressed_size
              , (size_t)(state->output.write_pos + decompressed_size - state->output.size)
             );

    state->output.write_pos += decompressed_size;

    // statistics
    state->total_compressed += compressed_size;
    state->total_uncompressed += decompressed_size;
    state->total_compressions++;

    return decompressed_size;
}

#endif // ENABLE_LZ4
