#include "compression.h"

#ifdef ENABLE_RRDPUSH_COMPRESSION

#include "lz4.h"
#include <zstd.h>

// ----------------------------------------------------------------------------
// ring buffers

static inline void ring_buffer_reset(struct compression_ring_buffer *b) {
    b->read_pos = b->write_pos = 0;
}

static inline void ring_buffer_make_room(struct compression_ring_buffer *b, size_t size) {
    if(b->write_pos + size > b->size) {
        if(!b->size)
            b->size = COMPRESSION_MAX_MSG_SIZE;
        else
            b->size *= 2;

        if(b->write_pos + size > b->size)
            b->size += size;

        b->data = reallocz((void *)b->data, b->size);
    }
}

static inline void ring_buffer_append_data(struct compression_ring_buffer *b, const void *data, size_t size) {
    ring_buffer_make_room(b, size);
    memcpy((void *)(b->data + b->write_pos), data, size);
    b->write_pos += size;
}

static inline void ring_buffer_destroy(struct compression_ring_buffer *b) {
    freez((void *)b->data);
    b->data = NULL;
    b->read_pos = b->write_pos = b->size = 0;
}

// ----------------------------------------------------------------------------
// compressor functions

static inline void rrdpush_compressor_init_lz4(struct compressor_state *state) {
    if(!state->initialized) {
        state->initialized = true;
        state->stream = LZ4_createStream();

        // LZ4 needs access to the last 64KB of source data
        // so, we keep twice the size of each message
        ring_buffer_make_room(&state->input, 65536 + COMPRESSION_MAX_MSG_SIZE * 2);
    }
}

#ifdef ENABLE_ZSTD
static inline void rrdpush_compressor_init_zstd(struct compressor_state *state) {
    if(!state->initialized) {
        state->initialized = true;
        state->stream = ZSTD_createCStream();

        ring_buffer_make_room(&state->input, COMPRESSION_MAX_MSG_SIZE);

        // ZSTD_CCtx_setParameter(state->stream.zstd.ctx, ZSTD_c_compressionLevel, ZSTD_CLEVEL_DEFAULT);
    }
}
#endif

static inline void rrdpush_compressor_destroy_lz4(struct compressor_state *state) {
    if (state->stream) {
        LZ4_freeStream(state->stream);
        state->stream = NULL;
    }
}

#ifdef ENABLE_ZSTD
static inline void rrdpush_compressor_destroy_zstd(struct compressor_state *state) {
    if(state->stream) {
        ZSTD_freeCStream(state->stream);
        state->stream = NULL;
    }
}
#endif

/*
 * Compress the given block of data
 * Compressed data will remain in the internal buffer until the next invocation
 * Return the size of compressed data block as result and the pointer to internal buffer using the last argument
 * or 0 in case of error
 */
static inline size_t rrdpush_compress_lz4(struct compressor_state *state, const char *data, size_t size, const char **out) {
    if(unlikely(!state || !size || !out))
        return 0;

    // we need to keep the last 64K of our previous source data
    // as they were in the ring buffer

    ring_buffer_make_room(&state->output, LZ4_COMPRESSBOUND(size));

    if(state->input.write_pos + size > state->input.size)
        // the input buffer cannot fit out data, restart from zero
        ring_buffer_reset(&state->input);

    ring_buffer_append_data(&state->input, data, size);

    long int compressed_data_size = LZ4_compress_fast_continue(
            state->stream,
            state->input.data + state->input.read_pos,
            (char *)state->output.data,
            (int)(state->input.write_pos - state->input.read_pos),
            (int)state->output.size,
            1);

    if (compressed_data_size < 0) {
        netdata_log_error("Data compression error: %ld", compressed_data_size);
        return 0;
    }

    state->input.read_pos = state->input.write_pos;

    *out = state->output.data;
    return compressed_data_size;
}

#ifdef ENABLE_ZSTD
static inline size_t rrdpush_compress_zstd(struct compressor_state *state, const char *data, size_t size, const char **out) {
    if(unlikely(!state || !size || !out))
        return 0;

    ring_buffer_append_data(&state->input, data, size);

    ZSTD_inBuffer inBuffer = {
            .pos = state->input.read_pos,
            .size = state->input.write_pos,
            .src = state->input.data,
    };

    ring_buffer_make_room(&state->output, ZSTD_compressBound(inBuffer.size - inBuffer.pos));

    ZSTD_outBuffer outBuffer = {
            .pos = 0,
            .size = state->output.size,
            .dst = (void *)state->output.data,
    };

    // compress
    size_t ret = ZSTD_compressStream(state->stream,
                                            &outBuffer,
                                            &inBuffer);

    // error handling
    if(ZSTD_isError(ret)) {
        netdata_log_error("STREAM: ZSTD_compressStream() return error: %s", ZSTD_getErrorName(ret));
        return 0;
    }

    state->input.read_pos = inBuffer.pos;

    if(state->input.read_pos >= state->input.write_pos) {
        state->input.read_pos = 0;
        state->input.write_pos = 0;
    }

    // return values
    *out = state->output.data;
    return outBuffer.pos;
}
#endif

// ----------------------------------------------------------------------------
// decompressor functions

/*
 * Decompress the compressed data in the internal buffer
 * Return the size of uncompressed data or 0 for error
 */
static inline size_t rrdpush_decompress_lz4(struct decompressor_state *state, const char *compressed_data, size_t compressed_size) {
    if (unlikely(!state || !compressed_data || !compressed_size))
        return 0;

    if(unlikely(state->output.read_pos != state->output.write_pos))
        fatal("RRDPUSH_DECOMPRESS: asked to decompress new data, while there are unread data in the decompression buffer!");

    if (unlikely(state->output.write_pos + COMPRESSION_MAX_MSG_SIZE > state->output.size))
        // the input buffer cannot fit out data, restart from zero
        ring_buffer_reset(&state->output);

    long int decompressed_size = LZ4_decompress_safe_continue(
            state->stream
            , compressed_data
            , (char *)(state->output.data + state->output.write_pos)
            , (int)compressed_size
            , (int)(state->output.size - state->output.write_pos)
            );

    if (unlikely(decompressed_size < 0)) {
        netdata_log_error("RRDPUSH DECOMPRESS: decompressor returned negative decompressed bytes: %ld", decompressed_size);
        return 0;
    }

    if(unlikely(decompressed_size + state->output.write_pos > state->output.size))
        fatal("RRDPUSH DECOMPRESS: decompressor overflown the stream_buffer. size: %zu, pos: %zu, added: %ld, "
              "exceeding the buffer by %zu"
              , state->output.size
              , state->output.write_pos
              , decompressed_size
              , (size_t)(state->output.write_pos + decompressed_size - state->output.size)
              );

    state->output.write_pos += decompressed_size;

    // statistics
    state->total_compressed += compressed_size + RRDPUSH_COMPRESSION_SIGNATURE_SIZE;
    state->total_uncompressed += decompressed_size;
    state->packet_count++;

    return decompressed_size;
}

#ifdef ENABLE_ZSTD
static inline size_t rrdpush_decompress_zstd(struct decompressor_state *state, const char *compressed_data, size_t compressed_size) {
    if (unlikely(!state || !compressed_data || !compressed_size))
        return 0;

    if(unlikely(state->output.read_pos != state->output.write_pos))
        fatal("RRDPUSH_DECOMPRESS: asked to decompress new data, while there are unread data in the decompression buffer!");

    ring_buffer_reset(&state->output);

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
        fatal("RRDPUSH DECOMPRESS: we should have decompressed all of it, but compressed data remain");

    size_t decompressed_size = outBuffer.pos - state->output.write_pos;
    state->output.write_pos = outBuffer.pos;

    // statistics
    state->total_compressed += compressed_size + RRDPUSH_COMPRESSION_SIGNATURE_SIZE;
    state->total_uncompressed += decompressed_size;
    state->packet_count++;

    return decompressed_size;
}
#endif

static inline void rrdpush_decompressor_init_lz4(struct decompressor_state *state) {
    if(!state->initialized) {
        state->initialized = true;
        state->stream = LZ4_createStreamDecode();
        ring_buffer_make_room(&state->output, 65536 + COMPRESSION_MAX_MSG_SIZE * 2);
    }
}

#ifdef ENABLE_ZSTD
static inline void rrdpush_decompressor_init_zstd(struct decompressor_state *state) {
    if(!state->initialized) {
        state->initialized = true;
        state->stream = ZSTD_createDStream();
        ring_buffer_make_room(&state->output, COMPRESSION_MAX_MSG_SIZE);
    }
}
#endif

static inline void rrdpush_decompressor_destroy_lz4(struct decompressor_state *state) {
    if (state->stream) {
        LZ4_freeStreamDecode(state->stream);
        state->stream = NULL;
    }
}

#ifdef ENABLE_ZSTD
static inline void rrdpush_decompressor_destroy_zstd(struct decompressor_state *state) {
    if (state->stream) {
        ZSTD_freeDStream(state->stream);
        state->stream = NULL;
    }
}
#endif

// ----------------------------------------------------------------------------
// compressor public API

void rrdpush_compressor_init(struct compressor_state *state) {
    switch(state->algorithm) {
        default:
        case COMPRESSION_ALGORITHM_LZ4:
            rrdpush_compressor_init_lz4(state);
            break;

#ifdef ENABLE_ZSTD
        case COMPRESSION_ALGORITHM_ZSTD:
            rrdpush_compressor_init_zstd(state);
            break;
#endif
    }

    ring_buffer_reset(&state->input);
    ring_buffer_reset(&state->output);
}

void rrdpush_compressor_destroy(struct compressor_state *state) {
    switch(state->algorithm) {
        default:
        case COMPRESSION_ALGORITHM_LZ4:
            rrdpush_compressor_destroy_lz4(state);
            break;

#ifdef ENABLE_ZSTD
        case COMPRESSION_ALGORITHM_ZSTD:
            rrdpush_compressor_destroy_zstd(state);
            break;
#endif
    }

    state->initialized = false;

    ring_buffer_destroy(&state->input);
    ring_buffer_destroy(&state->output);
}

size_t rrdpush_compress(struct compressor_state *state, const char *data, size_t size, const char **out) {
    switch(state->algorithm) {
        default:
        case COMPRESSION_ALGORITHM_LZ4:
            return rrdpush_compress_lz4(state, data, size, out);

#ifdef ENABLE_ZSTD
        case COMPRESSION_ALGORITHM_ZSTD:
            return rrdpush_compress_zstd(state, data, size, out);
#endif
    }
}

// ----------------------------------------------------------------------------
// decompressor public API

void rrdpush_decompressor_destroy(struct decompressor_state *state) {
    if(unlikely(!state->initialized))
        return;

    switch(state->algorithm) {
        default:
        case COMPRESSION_ALGORITHM_LZ4:
            rrdpush_decompressor_destroy_lz4(state);
            break;

#ifdef ENABLE_ZSTD
        case COMPRESSION_ALGORITHM_ZSTD:
            rrdpush_decompressor_destroy_zstd(state);
            break;
#endif
    }

    ring_buffer_destroy(&state->output);

    state->initialized = false;
}

void rrdpush_decompressor_init(struct decompressor_state *state) {
    switch(state->algorithm) {
        default:
        case COMPRESSION_ALGORITHM_LZ4:
            rrdpush_decompressor_init_lz4(state);
            break;

#ifdef ENABLE_ZSTD
        case COMPRESSION_ALGORITHM_ZSTD:
            rrdpush_decompressor_init_zstd(state);
            break;
#endif
    }

    state->signature_size = RRDPUSH_COMPRESSION_SIGNATURE_SIZE;
    ring_buffer_reset(&state->output);
}

size_t rrdpush_decompress(struct decompressor_state *state, const char *compressed_data, size_t compressed_size) {
    switch(state->algorithm) {
        default:
        case COMPRESSION_ALGORITHM_LZ4:
            return rrdpush_decompress_lz4(state, compressed_data, compressed_size);
            break;

#ifdef ENABLE_ZSTD
        case COMPRESSION_ALGORITHM_ZSTD:
            return rrdpush_decompress_zstd(state, compressed_data, compressed_size);
            break;
#endif
    }
}

#endif
