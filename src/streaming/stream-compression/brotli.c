// SPDX-License-Identifier: GPL-3.0-or-later

#include "brotli.h"

#ifdef ENABLE_BROTLI
#include <brotli/encode.h>
#include <brotli/decode.h>

void stream_compressor_init_brotli(struct compressor_state *state) {
    if (!state->initialized) {
        state->initialized = true;
        state->stream = BrotliEncoderCreateInstance(NULL, NULL, NULL);

        if (state->level < BROTLI_MIN_QUALITY) {
            state->level = BROTLI_MIN_QUALITY;
        } else if (state->level > BROTLI_MAX_QUALITY) {
            state->level = BROTLI_MAX_QUALITY;
        }

        BrotliEncoderSetParameter(state->stream, BROTLI_PARAM_QUALITY, state->level);
    }
}

void stream_compressor_destroy_brotli(struct compressor_state *state) {
    if (state->stream) {
        BrotliEncoderDestroyInstance(state->stream);
        state->stream = NULL;
    }
}

size_t stream_compress_brotli(struct compressor_state *state, const char *data, size_t size, const char **out) {
    if (unlikely(!state || !size || !out))
        return 0;

    simple_ring_buffer_make_room(&state->output, MAX(BrotliEncoderMaxCompressedSize(size), COMPRESSION_MAX_CHUNK));

    size_t available_out = state->output.size;

    size_t available_in = size;
    const uint8_t *next_in = (const uint8_t *)data;
    uint8_t *next_out = (uint8_t *)state->output.data;

    if (!BrotliEncoderCompressStream(state->stream, BROTLI_OPERATION_FLUSH, &available_in, &next_in, &available_out, &next_out, NULL)) {
        netdata_log_error("STREAM_COMPRESS: Brotli compression failed.");
        return 0;
    }

    if(available_in != 0) {
        netdata_log_error("STREAM_COMPRESS: BrotliEncoderCompressStream() did not use all the input buffer, %zu bytes out of %zu remain",
                available_in, size);
        return 0;
    }

    size_t compressed_size = state->output.size - available_out;
    if(available_out == 0) {
        netdata_log_error("STREAM_COMPRESS: BrotliEncoderCompressStream() needs a bigger output buffer than the one we provided "
                          "(output buffer %zu bytes, compressed payload %zu bytes)",
                state->output.size, size);
        return 0;
    }

    if(compressed_size == 0) {
        netdata_log_error("STREAM_COMPRESS: BrotliEncoderCompressStream() did not produce any output from the input provided "
                          "(input buffer %zu bytes)",
                size);
        return 0;
    }

    state->sender_locked.total_compressions++;
    state->sender_locked.total_uncompressed += size - available_in;
    state->sender_locked.total_compressed += compressed_size;

    *out = state->output.data;
    return compressed_size;
}

void stream_decompressor_init_brotli(struct decompressor_state *state) {
    if (!state->initialized) {
        state->initialized = true;
        state->stream = BrotliDecoderCreateInstance(NULL, NULL, NULL);

        simple_ring_buffer_make_room(&state->output, COMPRESSION_MAX_CHUNK);
    }
}

void stream_decompressor_destroy_brotli(struct decompressor_state *state) {
    if (state->stream) {
        BrotliDecoderDestroyInstance(state->stream);
        state->stream = NULL;
    }
}

size_t stream_decompress_brotli(struct decompressor_state *state, const char *compressed_data, size_t compressed_size) {
    if (unlikely(!state || !compressed_data || !compressed_size))
        return 0;

    // The state.output ring buffer is always EMPTY at this point,
    // meaning that (state->output.read_pos == state->output.write_pos)
    // However, THEY ARE NOT ZERO.

    size_t available_out = state->output.size;
    size_t available_in = compressed_size;
    const uint8_t *next_in = (const uint8_t *)compressed_data;
    uint8_t *next_out = (uint8_t *)state->output.data;

    if (BrotliDecoderDecompressStream(state->stream, &available_in, &next_in, &available_out, &next_out, NULL) == BROTLI_DECODER_RESULT_ERROR) {
        netdata_log_error("STREAM_DECOMPRESS: Brotli decompression failed.");
        return 0;
    }

    if(available_in != 0) {
        netdata_log_error("STREAM_DECOMPRESS: BrotliDecoderDecompressStream() did not use all the input buffer, %zu bytes out of %zu remain",
                          available_in, compressed_size);
        return 0;
    }

    size_t decompressed_size = state->output.size - available_out;
    if(available_out == 0) {
        netdata_log_error("STREAM_DECOMPRESS: BrotliDecoderDecompressStream() needs a bigger output buffer than the one we provided "
                          "(output buffer %zu bytes, compressed payload %zu bytes)",
                          state->output.size, compressed_size);
        return 0;
    }

    if(decompressed_size == 0) {
        netdata_log_error("STREAM_DECOMPRESS: BrotliDecoderDecompressStream() did not produce any output from the input provided "
                          "(input buffer %zu bytes)",
                          compressed_size);
        return 0;
    }

    state->output.read_pos = 0;
    state->output.write_pos = decompressed_size;

    state->total_compressed += compressed_size - available_in;
    state->total_uncompressed += decompressed_size;
    state->total_compressions++;

    return decompressed_size;
}

#endif // ENABLE_BROTLI
