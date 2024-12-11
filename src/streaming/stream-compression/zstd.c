// SPDX-License-Identifier: GPL-3.0-or-later

#include "zstd.h"

#ifdef ENABLE_ZSTD
#include <zstd.h>

void stream_compressor_init_zstd(struct compressor_state *state) {
    if(!state->initialized) {
        state->initialized = true;
        state->stream = ZSTD_createCStream();

        if(state->level < 1)
            state->level = 1;

        if(state->level > ZSTD_maxCLevel())
            state->level = ZSTD_maxCLevel();

        size_t ret = ZSTD_initCStream(state->stream, state->level);
        if(ZSTD_isError(ret))
            netdata_log_error("STREAM_COMPRESS: ZSTD_initCStream() returned error: %s", ZSTD_getErrorName(ret));

        // ZSTD_CCtx_setParameter(state->stream, ZSTD_c_compressionLevel, 1);
        // ZSTD_CCtx_setParameter(state->stream, ZSTD_c_strategy, ZSTD_fast);
    }
}

void stream_compressor_destroy_zstd(struct compressor_state *state) {
    if(state->stream) {
        ZSTD_freeCStream(state->stream);
        state->stream = NULL;
    }
}

size_t stream_compress_zstd(struct compressor_state *state, const char *data, size_t size, const char **out) {
    if(unlikely(!state || !size || !out))
        return 0;

    ZSTD_inBuffer inBuffer = {
            .pos = 0,
            .size = size,
            .src = data,
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
        netdata_log_error("STREAM_COMPRESS: ZSTD_compressStream() return error: %s", ZSTD_getErrorName(ret));
        return 0;
    }

    if(inBuffer.pos < inBuffer.size) {
        netdata_log_error("STREAM_COMPRESS: ZSTD_compressStream() left unprocessed input (source payload %zu bytes, consumed %zu bytes)",
                          inBuffer.size, inBuffer.pos);
        return 0;
    }

    if(outBuffer.pos == 0) {
        // ZSTD needs more input to flush the output, so let's flush it manually
        ret = ZSTD_flushStream(state->stream, &outBuffer);

        if(ZSTD_isError(ret)) {
            netdata_log_error("STREAM_COMPRESS: ZSTD_flushStream() return error: %s", ZSTD_getErrorName(ret));
            return 0;
        }

        if(outBuffer.pos == 0) {
            netdata_log_error("STREAM_COMPRESS: ZSTD_compressStream() returned zero compressed bytes "
                              "(source is %zu bytes, output buffer can fit %zu bytes) "
                              , size, outBuffer.size);
            return 0;
        }
    }

    state->sender_locked.total_compressions++;
    state->sender_locked.total_uncompressed += size;
    state->sender_locked.total_compressed += outBuffer.pos;

    // return values
    *out = state->output.data;
    return outBuffer.pos;
}

void stream_decompressor_init_zstd(struct decompressor_state *state) {
    if(!state->initialized) {
        state->initialized = true;
        state->stream = ZSTD_createDStream();

        size_t ret = ZSTD_initDStream(state->stream);
        if(ZSTD_isError(ret))
            netdata_log_error("STREAM_DECOMPRESS: ZSTD_initDStream() returned error: %s", ZSTD_getErrorName(ret));

        simple_ring_buffer_make_room(&state->output, MAX(COMPRESSION_MAX_CHUNK, ZSTD_DStreamOutSize()));
    }
}

void stream_decompressor_destroy_zstd(struct decompressor_state *state) {
    if (state->stream) {
        ZSTD_freeDStream(state->stream);
        state->stream = NULL;
    }
}

size_t stream_decompress_zstd(struct decompressor_state *state, const char *compressed_data, size_t compressed_size) {
    if (unlikely(!state || !compressed_data || !compressed_size))
        return 0;

    // The state.output ring buffer is always EMPTY at this point,
    // meaning that (state->output.read_pos == state->output.write_pos)
    // However, THEY ARE NOT ZERO.

    ZSTD_inBuffer inBuffer = {
            .pos = 0,
            .size = compressed_size,
            .src = compressed_data,
    };

    ZSTD_outBuffer outBuffer = {
            .pos = 0,
            .dst = (char *)state->output.data,
            .size = state->output.size,
    };

    size_t ret = ZSTD_decompressStream(
            state->stream
            , &outBuffer
            , &inBuffer);

    if(ZSTD_isError(ret)) {
        netdata_log_error("STREAM_DECOMPRESS: ZSTD_decompressStream() return error: %s", ZSTD_getErrorName(ret));
        return 0;
    }

    if(inBuffer.pos < inBuffer.size)
        fatal("STREAM_DECOMPRESS: ZSTD ZSTD_decompressStream() decompressed %zu bytes, "
              "but %zu bytes of compressed data remain",
              inBuffer.pos, inBuffer.size);

    size_t decompressed_size = outBuffer.pos;

    state->output.read_pos = 0;
    state->output.write_pos = outBuffer.pos;

    // statistics
    state->total_compressed += compressed_size;
    state->total_uncompressed += decompressed_size;
    state->total_compressions++;

    return decompressed_size;
}

#endif // ENABLE_ZSTD
