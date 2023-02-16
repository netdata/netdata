#include "rrdpush.h"

#ifdef ENABLE_COMPRESSION
#include "lz4.h"

#define STREAM_COMPRESSION_MSG "STREAM_COMPRESSION"

// signature MUST end with a newline
#define SIGNATURE ((uint32_t)('z' | 0x80) | (0x80 << 8) | (0x80 << 16) | ('\n' << 24))
#define SIGNATURE_MASK ((uint32_t)0xff | (0x80 << 8) | (0x80 << 16) | (0xff << 24))
#define SIGNATURE_SIZE 4


/*
 * LZ4 streaming API compressor specific data
 */
struct compressor_data {
    LZ4_stream_t *stream;
    char *input_ring_buffer;
    size_t input_ring_buffer_size;
    size_t input_ring_buffer_pos;
};


/*
 * Reset compressor state for a new stream
 */
static void lz4_compressor_reset(struct compressor_state *state)
{
    if (state->data) {
        if (state->data->stream) {
            LZ4_resetStream_fast(state->data->stream);            
            internal_error(true, "%s: compressor reset", STREAM_COMPRESSION_MSG);
        }
        state->data->input_ring_buffer_pos = 0;
    }
}

/*
 * Destroy compressor state and all related data
 */
static void lz4_compressor_destroy(struct compressor_state **state)
{
    if (state && *state) {
        struct compressor_state *s = *state;
        if (s->data) {
            if (s->data->stream)
                LZ4_freeStream(s->data->stream);
            freez(s->data->input_ring_buffer);
            freez(s->data);
        }
        freez(s->compression_result_buffer);
        freez(s);
        *state = NULL;
        debug(D_STREAM, "%s: Compressor Destroyed.", STREAM_COMPRESSION_MSG);
    }
}

/*
 * Compress the given block of data
 * Compressed data will remain in the internal buffer until the next invocation
 * Return the size of compressed data block as result and the pointer to internal buffer using the last argument
 * or 0 in case of error
 */
static size_t lz4_compressor_compress(struct compressor_state *state, const char *data, size_t size, char **out)
{
    if(unlikely(!state || !size || !out))
        return 0;

    if(unlikely(size > COMPRESSION_MAX_MSG_SIZE)) {
        error("%s: Compression Failed - Message size %lu above compression buffer limit: %d", STREAM_COMPRESSION_MSG, (long unsigned int)size, COMPRESSION_MAX_MSG_SIZE);
        return 0;
    }

    size_t max_dst_size = LZ4_COMPRESSBOUND(size);
    size_t data_size = max_dst_size + SIGNATURE_SIZE;

    if (!state->compression_result_buffer) {
        state->compression_result_buffer = mallocz(data_size);
        state->compression_result_buffer_size = data_size;
    }
    else if(unlikely(state->compression_result_buffer_size < data_size)) {
        state->compression_result_buffer = reallocz(state->compression_result_buffer, data_size);
        state->compression_result_buffer_size = data_size;
    }

    // the ring buffer always has space for LZ4_MAX_MSG_SIZE
    memcpy(state->data->input_ring_buffer + state->data->input_ring_buffer_pos, data, size);

    // this call needs the last 64K of our previous data
    // they are available in the ring buffer
    long int compressed_data_size = LZ4_compress_fast_continue(
        state->data->stream,
        state->data->input_ring_buffer + state->data->input_ring_buffer_pos,
        state->compression_result_buffer + SIGNATURE_SIZE,
        size,
        max_dst_size,
        1);

    if (compressed_data_size < 0) {
        error("Data compression error: %ld", compressed_data_size);
        return 0;
    }

    // update the next writing position of the ring buffer
    state->data->input_ring_buffer_pos += size;
    if(unlikely(state->data->input_ring_buffer_pos >= state->data->input_ring_buffer_size - COMPRESSION_MAX_MSG_SIZE))
        state->data->input_ring_buffer_pos = 0;

    // update the signature header
    uint32_t len = ((compressed_data_size & 0x7f) | 0x80 | (((compressed_data_size & (0x7f << 7)) << 1) | 0x8000)) << 8;
    *(uint32_t *)state->compression_result_buffer = len | SIGNATURE;
    *out = state->compression_result_buffer;
    debug(D_STREAM, "%s: Compressed data header: %ld", STREAM_COMPRESSION_MSG, compressed_data_size);
    return compressed_data_size + SIGNATURE_SIZE;
}

/*
 * Create and initialize compressor state
 * Return the pointer to compressor_state structure created
 */
struct compressor_state *create_compressor()
{
    struct compressor_state *state = callocz(1, sizeof(struct compressor_state));

    state->reset = lz4_compressor_reset;
    state->compress = lz4_compressor_compress;
    state->destroy = lz4_compressor_destroy;

    state->data = callocz(1, sizeof(struct compressor_data));
    state->data->stream = LZ4_createStream();
    state->data->input_ring_buffer_size = LZ4_DECODER_RING_BUFFER_SIZE(COMPRESSION_MAX_MSG_SIZE * 2);
    state->data->input_ring_buffer = callocz(1, state->data->input_ring_buffer_size);
    state->compression_result_buffer_size = 0;
    state->reset(state);
    debug(D_STREAM, "%s: Initialize streaming compression!", STREAM_COMPRESSION_MSG);
    return state;
}

/*
 * LZ4 streaming API decompressor specific data
 */
struct decompressor_stream {
    LZ4_streamDecode_t *lz4_stream;
    char *buffer;
    size_t size;
    size_t write_at;
    size_t read_at;
};

/*
 * Reset decompressor state for a new stream
 */
static void lz4_decompressor_reset(struct decompressor_state *state)
{
    if (state->stream) {
        if (state->stream->lz4_stream)
           LZ4_setStreamDecode(state->stream->lz4_stream, NULL, 0);

        state->stream->write_at = 0;
        state->stream->read_at = 0;
    }
}

/*
 * Destroy decompressor state and all related data
 */
static void lz4_decompressor_destroy(struct decompressor_state **state)
{
    if (state && *state) {
        struct decompressor_state *s = *state;
        if (s->stream) {
            debug(D_STREAM, "%s: Destroying decompressor.", STREAM_COMPRESSION_MSG);
            if (s->stream->lz4_stream)
                LZ4_freeStreamDecode(s->stream->lz4_stream);
            freez(s->stream->buffer);
            freez(s->stream);
        }
        freez(s);
        *state = NULL;
    }
}

static size_t decode_compress_header(const char *data, size_t data_size) {
    if (unlikely(!data || !data_size))
        return 0;

    if (unlikely(data_size != SIGNATURE_SIZE))
        return 0;

    uint32_t sign = *(uint32_t *)data;
    if (unlikely((sign & SIGNATURE_MASK) != SIGNATURE))
        return 0;

    size_t length = ((sign >> 8) & 0x7f) | ((sign >> 9) & (0x7f << 7));
    return length;
}

/*
 * Start the collection of compressed data in an internal buffer
 * Return the size of compressed data or 0 for uncompressed data
 */
static size_t lz4_decompressor_start(struct decompressor_state *state __maybe_unused, const char *header, size_t header_size) {
    if(unlikely(state->stream->read_at != state->stream->write_at))
        fatal("%s: asked to decompress new data, while there are unread data in the decompression buffer!"
        , STREAM_COMPRESSION_MSG);

    return decode_compress_header(header, header_size);
}

/*
 * Decompress the compressed data in the internal buffer
 * Return the size of uncompressed data or 0 for error
 */
static size_t lz4_decompressor_decompress(struct decompressor_state *state, const char *compressed_data, size_t compressed_size) {
    if (unlikely(!state || !compressed_data || !compressed_size))
        return 0;

    if(unlikely(state->stream->read_at != state->stream->write_at))
        fatal("%s: asked to decompress new data, while there are unread data in the decompression buffer!"
              , STREAM_COMPRESSION_MSG);

    if (unlikely(state->stream->write_at >= state->stream->size / 2)) {
        state->stream->write_at = 0;
        state->stream->read_at = 0;
    }

    long int decompressed_size = LZ4_decompress_safe_continue(
            state->stream->lz4_stream
            , compressed_data
            , state->stream->buffer + state->stream->write_at
            , (int)compressed_size
            , (int)(state->stream->size - state->stream->write_at)
            );

    if (unlikely(decompressed_size < 0)) {
        error("%s: decompressor returned negative decompressed bytes: %ld", STREAM_COMPRESSION_MSG, decompressed_size);
        return 0;
    }

    if(unlikely(decompressed_size + state->stream->write_at > state->stream->size))
        fatal("%s: decompressor overflown the stream_buffer. size: %zu, pos: %zu, added: %ld, exceeding the buffer by %zu"
              , STREAM_COMPRESSION_MSG
              , state->stream->size
              , state->stream->write_at
              , decompressed_size
              , (size_t)(state->stream->write_at + decompressed_size - state->stream->size)
              );

    state->stream->write_at += decompressed_size;

    // statistics
    state->total_compressed += compressed_size + SIGNATURE_SIZE;
    state->total_uncompressed += decompressed_size;
    state->packet_count++;

    return decompressed_size;
}

/*
 * Return the size of uncompressed data left in the internal buffer or 0 for error
 */
static size_t lz4_decompressor_decompressed_bytes_in_buffer(struct decompressor_state *state) {
    if(unlikely(state->stream->read_at > state->stream->write_at))
        fatal("%s: invalid read/write stream positions"
        , STREAM_COMPRESSION_MSG);

    return state->stream->write_at - state->stream->read_at;
}

/*
 * Fill the buffer provided with uncompressed data from the internal buffer
 * Return the size of uncompressed data copied or 0 for error
 */
static size_t lz4_decompressor_get(struct decompressor_state *state, char *dst, size_t size) {
    if (unlikely(!state || !size || !dst))
        return 0;

    size_t remaining = lz4_decompressor_decompressed_bytes_in_buffer(state);
    if(unlikely(!remaining))
        return 0;

    size_t bytes_to_return = size;
    if(bytes_to_return > remaining)
        bytes_to_return = remaining;

    memcpy(dst, state->stream->buffer + state->stream->read_at, bytes_to_return);
    state->stream->read_at += bytes_to_return;

    if(unlikely(state->stream->read_at > state->stream->write_at))
        fatal("%s: invalid read/write stream positions"
        , STREAM_COMPRESSION_MSG);

    return bytes_to_return;
}

/*
 * Create and initialize decompressor state
 * Return the pointer to decompressor_state structure created
 */
struct decompressor_state *create_decompressor()
{
    struct decompressor_state *state = callocz(1, sizeof(struct decompressor_state));
    state->signature_size = SIGNATURE_SIZE;
    state->reset = lz4_decompressor_reset;
    state->start = lz4_decompressor_start;
    state->decompress = lz4_decompressor_decompress;
    state->get = lz4_decompressor_get;
    state->decompressed_bytes_in_buffer = lz4_decompressor_decompressed_bytes_in_buffer;
    state->destroy = lz4_decompressor_destroy;

    state->stream = callocz(1, sizeof(struct decompressor_stream));
    fatal_assert(state->stream);
    state->stream->lz4_stream = LZ4_createStreamDecode();
    state->stream->size = LZ4_decoderRingBufferSize(COMPRESSION_MAX_MSG_SIZE) * 2;
    state->stream->buffer = mallocz(state->stream->size);
    fatal_assert(state->stream->buffer);
    state->reset(state);
    debug(D_STREAM, "%s: Initialize streaming decompression!", STREAM_COMPRESSION_MSG);
    return state;
}
#endif
