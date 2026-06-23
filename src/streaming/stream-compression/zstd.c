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

    if(inBuffer.pos < inBuffer.size) {
        // The peer sent a frame that decompresses to more than our output buffer.
        // A conforming sender never does this (it compresses in <= COMPRESSION_MAX_MSG_SIZE
        // plaintext chunks), but a malicious/compromised peer is not bound by that.
        // Return an error (like the gzip path and the ZSTD_isError branch above) so the
        // caller fails the stream connection, instead of fatal()-ing the whole Agent.
        netdata_log_error("STREAM_DECOMPRESS: ZSTD_decompressStream() consumed only %zu of %zu compressed bytes "
                          "after filling the %zu-byte output buffer (frame decompresses to more than the buffer); "
                          "failing the connection",
                          inBuffer.pos, inBuffer.size, outBuffer.size);
        return 0;
    }

    size_t decompressed_size = outBuffer.pos;

    state->output.read_pos = 0;
    state->output.write_pos = outBuffer.pos;

    // statistics
    state->total_compressed += compressed_size;
    state->total_uncompressed += decompressed_size;
    state->total_compressions++;

    return decompressed_size;
}

// Regression test for the decompression-bomb guard: a malicious peer can send one small
// ZSTD frame that decompresses to far more than the receiver's fixed output buffer.
// stream_decompress_zstd() must reject it (return 0), NOT fatal()/abort the agent.
// (Pre-fix this aborted the process, so a regression makes this test crash, not soft-fail.)
//
// Each sub-case uses a fresh decompressor_state, mirroring production: on a decompress
// failure the receiver tears down the connection, so a destroyed state is never reused.
int unittest_stream_decompress_bomb_zstd(void) {
    fprintf(stderr, "\nTesting ZSTD decompression-bomb guard\n");
    int errors = 0;

    // 2MB of zeros: decompresses to far more than the ~128KB output buffer, but the
    // compressed frame is only a few dozen bytes (a conforming sender never produces this).
    size_t plain_len = 2 * 1024 * 1024;
    char *plain = callocz(1, plain_len);
    size_t bound = ZSTD_compressBound(plain_len);
    char *comp = mallocz(bound);
    size_t comp_len = ZSTD_compress(comp, bound, plain, plain_len, 1);
    if (ZSTD_isError(comp_len)) {
        fprintf(stderr, "  FAILED: ZSTD_compress() error: %s\n", ZSTD_getErrorName(comp_len));
        freez(plain); freez(comp);
        return 1;
    }

    // (1) the bomb must be rejected gracefully (return 0), not fatal()
    {
        struct decompressor_state dctx = { .initialized = false, .algorithm = COMPRESSION_ALGORITHM_ZSTD };
        stream_decompressor_init(&dctx);
        size_t out = stream_decompress(&dctx, comp, comp_len);
        if (out != 0) {
            fprintf(stderr, "  FAILED: oversized frame (%zuB compressed) accepted, decompressed %zuB "
                            "(expected rejection)\n", comp_len, out);
            errors++;
        }
        else
            fprintf(stderr, "  OK: oversized frame (%zuB compressed) rejected gracefully, no fatal()\n", comp_len);
        stream_decompressor_destroy(&dctx);
    }

    // (2) a normal frame still decompresses correctly (fresh decompressor, as a new
    //     connection would get) - the guard must not break legitimate traffic.
    {
        const char *msg = "the quick brown fox jumps over the lazy dog";
        size_t msg_len = strlen(msg);
        size_t cbound = ZSTD_compressBound(msg_len);
        char *cmsg = mallocz(cbound);
        size_t cmsg_len = ZSTD_compress(cmsg, cbound, msg, msg_len, 1);
        if (ZSTD_isError(cmsg_len)) {
            fprintf(stderr, "  FAILED: ZSTD_compress() error: %s\n", ZSTD_getErrorName(cmsg_len));
            freez(cmsg); freez(plain); freez(comp);
            return errors + 1;
        }

        struct decompressor_state dctx = { .initialized = false, .algorithm = COMPRESSION_ALGORITHM_ZSTD };
        stream_decompressor_init(&dctx);
        size_t dlen = stream_decompress(&dctx, cmsg, cmsg_len);
        if (dlen != msg_len || memcmp(&dctx.output.data[dctx.output.read_pos], msg, msg_len) != 0) {
            fprintf(stderr, "  FAILED: normal frame did not round-trip (got %zuB, expected %zuB)\n", dlen, msg_len);
            errors++;
        }
        else
            fprintf(stderr, "  OK: normal frame round-trips\n");
        stream_decompressor_destroy(&dctx);
        freez(cmsg);
    }

    freez(plain); freez(comp);
    return errors;
}

#endif // ENABLE_ZSTD
