// SPDX-License-Identifier: GPL-3.0-or-later

#include "compression.h"

#ifdef ENABLE_RRDPUSH_COMPRESSION

#include "compression_gzip.h"

#ifdef ENABLE_LZ4
#include "compression_lz4.h"
#endif

#ifdef ENABLE_ZSTD
#include "compression_zstd.h"
#endif

// ----------------------------------------------------------------------------
// compressor public API

void rrdpush_compressor_init(struct compressor_state *state) {
    switch(state->algorithm) {
#ifdef ENABLE_ZSTD
        case COMPRESSION_ALGORITHM_ZSTD:
            rrdpush_compressor_init_zstd(state);
            break;
#endif

#ifdef ENABLE_LZ4
        case COMPRESSION_ALGORITHM_LZ4:
            rrdpush_compressor_init_lz4(state);
            break;
#endif

        default:
        case COMPRESSION_ALGORITHM_GZIP:
            rrdpush_compressor_init_gzip(state);
            break;
    }

    simple_ring_buffer_reset(&state->input);
    simple_ring_buffer_reset(&state->output);
}

void rrdpush_compressor_destroy(struct compressor_state *state) {
    switch(state->algorithm) {
#ifdef ENABLE_ZSTD
        case COMPRESSION_ALGORITHM_ZSTD:
            rrdpush_compressor_destroy_zstd(state);
            break;
#endif

#ifdef ENABLE_LZ4
        case COMPRESSION_ALGORITHM_LZ4:
            rrdpush_compressor_destroy_lz4(state);
            break;
#endif

        default:
        case COMPRESSION_ALGORITHM_GZIP:
            rrdpush_compressor_destroy_gzip(state);
            break;
    }

    state->initialized = false;

    simple_ring_buffer_destroy(&state->input);
    simple_ring_buffer_destroy(&state->output);
}

size_t rrdpush_compress(struct compressor_state *state, const char *data, size_t size, const char **out) {
    switch(state->algorithm) {
#ifdef ENABLE_ZSTD
        case COMPRESSION_ALGORITHM_ZSTD:
            return rrdpush_compress_zstd(state, data, size, out);
#endif

#ifdef ENABLE_LZ4
        case COMPRESSION_ALGORITHM_LZ4:
            return rrdpush_compress_lz4(state, data, size, out);
#endif

        default:
        case COMPRESSION_ALGORITHM_GZIP:
            return rrdpush_compress_gzip(state, data, size, out);
    }
}

// ----------------------------------------------------------------------------
// decompressor public API

void rrdpush_decompressor_destroy(struct decompressor_state *state) {
    if(unlikely(!state->initialized))
        return;

    switch(state->algorithm) {
#ifdef ENABLE_ZSTD
        case COMPRESSION_ALGORITHM_ZSTD:
            rrdpush_decompressor_destroy_zstd(state);
            break;
#endif

#ifdef ENABLE_LZ4
        case COMPRESSION_ALGORITHM_LZ4:
            rrdpush_decompressor_destroy_lz4(state);
            break;
#endif

        default:
        case COMPRESSION_ALGORITHM_GZIP:
            rrdpush_decompressor_destroy_gzip(state);
            break;
    }

    simple_ring_buffer_destroy(&state->output);

    state->initialized = false;
}

void rrdpush_decompressor_init(struct decompressor_state *state) {
    switch(state->algorithm) {
#ifdef ENABLE_ZSTD
        case COMPRESSION_ALGORITHM_ZSTD:
            rrdpush_decompressor_init_zstd(state);
            break;
#endif

#ifdef ENABLE_LZ4
        case COMPRESSION_ALGORITHM_LZ4:
            rrdpush_decompressor_init_lz4(state);
            break;
#endif

        default:
        case COMPRESSION_ALGORITHM_GZIP:
            rrdpush_decompressor_init_gzip(state);
            break;
    }

    state->signature_size = RRDPUSH_COMPRESSION_SIGNATURE_SIZE;
    simple_ring_buffer_reset(&state->output);
}

size_t rrdpush_decompress(struct decompressor_state *state, const char *compressed_data, size_t compressed_size) {
    switch(state->algorithm) {
#ifdef ENABLE_ZSTD
        case COMPRESSION_ALGORITHM_ZSTD:
            return rrdpush_decompress_zstd(state, compressed_data, compressed_size);
#endif

#ifdef ENABLE_LZ4
        case COMPRESSION_ALGORITHM_LZ4:
            return rrdpush_decompress_lz4(state, compressed_data, compressed_size);
#endif

        default:
        case COMPRESSION_ALGORITHM_GZIP:
            return rrdpush_decompress_gzip(state, compressed_data, compressed_size);
    }
}

#endif
