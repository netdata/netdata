// SPDX-License-Identifier: GPL-3.0-or-later

#include "gzip.h"
#include <zlib.h>

void stream_compressor_init_gzip(struct compressor_state *state) {
    if (!state->initialized) {
        state->initialized = true;

        // Initialize deflate stream
        z_stream *strm = state->stream = (z_stream *) mallocz(sizeof(z_stream));
        strm->zalloc = Z_NULL;
        strm->zfree = Z_NULL;
        strm->opaque = Z_NULL;

        if(state->level < Z_BEST_SPEED)
            state->level = Z_BEST_SPEED;

        if(state->level > Z_BEST_COMPRESSION)
            state->level = Z_BEST_COMPRESSION;

        // int r = deflateInit2(strm, Z_BEST_COMPRESSION, Z_DEFLATED, 15 + 16, 8, Z_DEFAULT_STRATEGY);
        int r = deflateInit2(strm, state->level, Z_DEFLATED, 15 + 16, 8, Z_DEFAULT_STRATEGY);
        if (r != Z_OK) {
            netdata_log_error("STREAM_COMPRESS: Failed to initialize deflate with error: %d", r);
            freez(state->stream);
            state->initialized = false;
            return;
        }

    }
}

void stream_compressor_destroy_gzip(struct compressor_state *state) {
    if (state->stream) {
        deflateEnd(state->stream);
        freez(state->stream);
        state->stream = NULL;
    }
}

size_t stream_compress_gzip(struct compressor_state *state, const char *data, size_t size, const char **out) {
    if (unlikely(!state || !size || !out))
        return 0;

    simple_ring_buffer_make_room(&state->output, deflateBound(state->stream, size));

    z_stream *strm = state->stream;
    strm->avail_in = (uInt)size;
    strm->next_in = (Bytef *)data;
    strm->avail_out = (uInt)state->output.size;
    strm->next_out = (Bytef *)state->output.data;

    int ret = deflate(strm, Z_SYNC_FLUSH);
    if (ret != Z_OK && ret != Z_STREAM_END) {
        netdata_log_error("STREAM_COMPRESS: deflate() failed with error %d", ret);
        return 0;
    }

    if(strm->avail_in != 0) {
        netdata_log_error("STREAM_COMPRESS: deflate() did not use all the input buffer, %u bytes out of %zu remain",
                          strm->avail_in, size);
        return 0;
    }

    if(strm->avail_out == 0) {
        netdata_log_error("STREAM_COMPRESS: deflate() needs a bigger output buffer than the one we provided "
                          "(output buffer %zu bytes, compressed payload %zu bytes)",
                          state->output.size, size);
        return 0;
    }

    size_t compressed_data_size = state->output.size - strm->avail_out;

    if(compressed_data_size == 0) {
        netdata_log_error("STREAM_COMPRESS: deflate() did not produce any output "
                          "(output buffer %zu bytes, compressed payload %zu bytes)",
                          state->output.size, size);
        return 0;
    }

    state->sender_locked.total_compressions++;
    state->sender_locked.total_uncompressed += size;
    state->sender_locked.total_compressed += compressed_data_size;

    *out = state->output.data;
    return compressed_data_size;
}

void stream_decompressor_init_gzip(struct decompressor_state *state) {
    if (!state->initialized) {
        state->initialized = true;

        // Initialize inflate stream
        z_stream *strm = state->stream = (z_stream *)mallocz(sizeof(z_stream));
        strm->zalloc = Z_NULL;
        strm->zfree = Z_NULL;
        strm->opaque = Z_NULL;

        int r = inflateInit2(strm, 15 + 16);
        if (r != Z_OK) {
            netdata_log_error("STREAM_DECOMPRESS: Failed to initialize inflateInit2() with error: %d", r);
            freez(state->stream);
            state->initialized = false;
            return;
        }

        simple_ring_buffer_make_room(&state->output, COMPRESSION_MAX_CHUNK);
    }
}

void stream_decompressor_destroy_gzip(struct decompressor_state *state) {
    if (state->stream) {
        inflateEnd(state->stream);
        freez(state->stream);
        state->stream = NULL;
    }
}

size_t stream_decompress_gzip(struct decompressor_state *state, const char *compressed_data, size_t compressed_size) {
    if (unlikely(!state || !compressed_data || !compressed_size))
        return 0;

    // The state.output ring buffer is always EMPTY at this point,
    // meaning that (state->output.read_pos == state->output.write_pos)
    // However, THEY ARE NOT ZERO.

    z_stream *strm = state->stream;
    strm->avail_in = (uInt)compressed_size;
    strm->next_in = (Bytef *)compressed_data;
    strm->avail_out = (uInt)state->output.size;
    strm->next_out = (Bytef *)state->output.data;

    int ret = inflate(strm, Z_SYNC_FLUSH);
    if (ret != Z_STREAM_END && ret != Z_OK) {
        netdata_log_error("STREAM_DECOMPRESS: inflate() failed with error %d", ret);
        return 0;
    }

    if(strm->avail_in != 0) {
        netdata_log_error("STREAM_DECOMPRESS: inflate() did not use all compressed data we provided "
                          "(compressed payload %zu bytes, remaining to be uncompressed %u)"
                          , compressed_size, strm->avail_in);
        return 0;
    }

    if(strm->avail_out == 0) {
        netdata_log_error("STREAM_DECOMPRESS: inflate() needs a bigger output buffer than the one we provided "
                          "(compressed payload %zu bytes, output buffer size %zu bytes)"
                          , compressed_size, state->output.size);
        return 0;
    }

    size_t decompressed_size = state->output.size - strm->avail_out;

    state->output.read_pos = 0;
    state->output.write_pos = decompressed_size;

    state->total_compressed += compressed_size;
    state->total_uncompressed += decompressed_size;
    state->total_compressions++;

    return decompressed_size;
}
