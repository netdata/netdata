// SPDX-License-Identifier: GPL-3.0-or-later

#include "compression_zstd.h"

#ifdef ENABLE_ZSTD
#include <zstd.h>

#ifndef ZSTD_CLEVEL_DEFAULT
#  define ZSTD_CLEVEL_DEFAULT 3
#endif

void rrdpush_compressor_init_zstd(struct compressor_state *state) {
    if(!state->initialized) {
        state->initialized = true;
        state->stream = ZSTD_createCStream();

        size_t ret = ZSTD_initCStream(state->stream, ZSTD_CLEVEL_DEFAULT);
        if(ZSTD_isError(ret))
            netdata_log_error("STREAM: ZSTD_initCStream() returned error: %s", ZSTD_getErrorName(ret));

        simple_ring_buffer_make_room(&state->input, MAX(COMPRESSION_MAX_CHUNK, ZSTD_CStreamInSize()));

        // ZSTD_CCtx_setParameter(state->stream, ZSTD_c_compressionLevel, 1);
        // ZSTD_CCtx_setParameter(state->stream, ZSTD_c_strategy, ZSTD_fast);
    }
}

void rrdpush_compressor_destroy_zstd(struct compressor_state *state) {
    if(state->stream) {
        ZSTD_freeCStream(state->stream);
        state->stream = NULL;
    }
}

size_t rrdpush_compress_zstd(struct compressor_state *state, const char *data, size_t size, const char **out) {
    if(unlikely(!state || !size || !out))
        return 0;

    simple_ring_buffer_append_data(&state->input, data, size);

    ZSTD_inBuffer inBuffer = {
            .pos = state->input.read_pos,
            .size = state->input.write_pos,
            .src = state->input.data,
    };

    size_t wanted_size = MAX(ZSTD_compressBound(inBuffer.size - inBuffer.pos), ZSTD_CStreamOutSize());
    simple_ring_buffer_make_room(&state->output, wanted_size);

    ZSTD_outBuffer outBuffer = {
            .pos = 0,
            .size = state->output.size,
            .dst = (void *)state->output.data,
    };

    // compress
    size_t ret = ZSTD_compressStream(state->stream, &outBuffer, &inBuffer);

    // error handling
    if(ZSTD_isError(ret)) {
        netdata_log_error("STREAM: ZSTD_compressStream() return error: %s", ZSTD_getErrorName(ret));
        return 0;
    }

    if(outBuffer.pos == 0) {
        // ZSTD needs more input to flush the output, so let's flush it manually
        ret = ZSTD_flushStream(state->stream, &outBuffer);

        if(ZSTD_isError(ret)) {
            netdata_log_error("STREAM: ZSTD_flushStream() return error: %s", ZSTD_getErrorName(ret));
            return 0;
        }

        if(outBuffer.pos == 0) {
            netdata_log_error("STREAM: ZSTD_compressStream() returned zero compressed bytes "
                              "(source is %zu bytes, output buffer can fit %zu bytes) "
                              , size, outBuffer.size);
            return 0;
        }
    }

    state->input.read_pos = inBuffer.pos;

    if(state->input.read_pos >= state->input.write_pos)
        simple_ring_buffer_reset(&state->input);

    state->sender_locked.total_compressions++;
    state->sender_locked.total_uncompressed += size;
    state->sender_locked.total_compressed += outBuffer.pos;

    // return values
    *out = state->output.data;
    return outBuffer.pos;
}


size_t rrdpush_decompress_zstd(struct decompressor_state *state, const char *compressed_data, size_t compressed_size) {
    if (unlikely(!state || !compressed_data || !compressed_size))
        return 0;

    if(unlikely(state->output.read_pos != state->output.write_pos))
        fatal("RRDPUSH_DECOMPRESS: ZSTD asked to decompress new data, while there are unread data in the decompression buffer!");

    simple_ring_buffer_reset(&state->output);

    ZSTD_inBuffer inBuffer = {
            .pos = 0,
            .size = compressed_size,
            .src = compressed_data,
    };

    ZSTD_outBuffer outBuffer = {
            .pos = state->output.write_pos,
            .dst = (char *)state->output.data,
            .size = state->output.size,
    };

    size_t ret = ZSTD_decompressStream(
            state->stream
            , &outBuffer
            , &inBuffer);

    if(ZSTD_isError(ret)) {
        netdata_log_error("STREAM: ZSTD_decompressStream() return error: %s", ZSTD_getErrorName(ret));
        return 0;
    }

    if(inBuffer.pos < inBuffer.size)
        fatal("RRDPUSH DECOMPRESS: ZSTD ZSTD_decompressStream() decompressed %zu bytes, "
              "but %zu bytes of compressed data remain",
              inBuffer.pos, inBuffer.size);

    size_t decompressed_size = outBuffer.pos - state->output.write_pos;
    state->output.write_pos = outBuffer.pos;

    // statistics
    state->total_compressed += compressed_size + RRDPUSH_COMPRESSION_SIGNATURE_SIZE;
    state->total_uncompressed += decompressed_size;
    state->packet_count++;

    return decompressed_size;
}

void rrdpush_decompressor_init_zstd(struct decompressor_state *state) {
    if(!state->initialized) {
        state->initialized = true;
        state->stream = ZSTD_createDStream();

        size_t ret = ZSTD_initDStream(state->stream);
        if(ZSTD_isError(ret))
            netdata_log_error("STREAM: ZSTD_initDStream() returned error: %s", ZSTD_getErrorName(ret));

        simple_ring_buffer_make_room(&state->output, MAX(COMPRESSION_MAX_CHUNK, ZSTD_DStreamOutSize()));
    }
}

void rrdpush_decompressor_destroy_zstd(struct decompressor_state *state) {
    if (state->stream) {
        ZSTD_freeDStream(state->stream);
        state->stream = NULL;
    }
}

#endif // ENABLE_ZSTD
