#include "rrdpush.h"

#ifdef ENABLE_COMPRESSION
#include "lz4.h"

#define STREAM_COMPRESSION_MSG "STREAM_COMPRESSION"

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
            info("%s: Compressor Reset", STREAM_COMPRESSION_MSG);
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

    if(unlikely(size > LZ4_MAX_MSG_SIZE)) {
        error("%s: Compression Failed - Message size %lu above compression buffer limit: %d", STREAM_COMPRESSION_MSG, (long unsigned int) size, LZ4_MAX_MSG_SIZE);
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
    if(unlikely(state->data->input_ring_buffer_pos >= state->data->input_ring_buffer_size - LZ4_MAX_MSG_SIZE))
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
    state->data->input_ring_buffer_size = LZ4_DECODER_RING_BUFFER_SIZE(LZ4_MAX_MSG_SIZE * 2);
    state->data->input_ring_buffer = callocz(1, state->data->input_ring_buffer_size);
    state->compression_result_buffer_size = 0;
    state->reset(state);
    debug(D_STREAM, "%s: Initialize streaming compression!", STREAM_COMPRESSION_MSG);
    return state;
}

/*
 * LZ4 streaming API decompressor specific data
 */
struct decompressor_data {
    LZ4_streamDecode_t *stream;
    char *stream_buffer;
    size_t stream_buffer_size;
    size_t stream_buffer_pos;
};

/*
 * Reset decompressor state for a new stream
 */
static void lz4_decompressor_reset(struct decompressor_state *state)
{
    if (state->data) {
        if (state->data->stream)
           LZ4_setStreamDecode(state->data->stream, NULL, 0);
        state->data->stream_buffer_pos = 0;
        state->buffer_len = 0;
        state->out_buffer_len = 0;
    }
}

/*
 * Destroy decompressor state and all related data
 */
static void lz4_decompressor_destroy(struct decompressor_state **state)
{
    if (state && *state) {
        struct decompressor_state *s = *state;
        if (s->data) {
            debug(D_STREAM, "%s: Destroying decompressor.", STREAM_COMPRESSION_MSG);
            if (s->data->stream)
                LZ4_freeStreamDecode(s->data->stream);
            freez(s->data->stream_buffer);
            freez(s->data);
        }
        freez(s->buffer);
        freez(s);
        *state = NULL;
    }
}

static size_t decode_compress_header(const char *data, size_t data_size)
{
    if (!data || !data_size)
        return 0;
    if (data_size < SIGNATURE_SIZE)
        return 0;
    uint32_t sign = *(uint32_t *)data;
    if ((sign & SIGNATURE_MASK) != SIGNATURE)
        return 0;
    size_t length = ((sign >> 8) & 0x7f) | ((sign >> 9) & (0x7f << 7));
    return length;
}

/*
 * Check input data for the compression header
 * Return the size of compressed data or 0 for uncompressed data
 */
size_t is_compressed_data(const char *data, size_t data_size)
{
    return decode_compress_header(data, data_size);
}

/*
 * Start the collection of compressed data in an internal buffer
 * Return the size of compressed data or 0 for uncompressed data
 */
static size_t lz4_decompressor_start(struct decompressor_state *state, const char *header, size_t header_size)
{
    size_t length = decode_compress_header(header, header_size);
    if (!length)
        return 0;

    if (!state->buffer) {
        state->buffer = mallocz(length);
        state->buffer_size = length;
    } else if (state->buffer_size < length) {
        state->buffer = reallocz(state->buffer, length);
        state->buffer_size = length;
    }
    state->buffer_len = length;
    state->buffer_pos = 0;
    state->out_buffer_pos = 0;
    state->out_buffer_len = 0;
    return length;
}

/*
 * Add a chunk of compressed data to the internal buffer
 * Return the current size of compressed data or 0 for error
 */
static size_t lz4_decompressor_put(struct decompressor_state *state, const char *data, size_t size)
{
    if (!state || !size || !data)
        return 0;
    if (!state->buffer)
        fatal("STREAM: No decompressor buffer allocated");

    if (state->buffer_pos + size > state->buffer_len) {
        error("STREAM: Decompressor buffer overflow %lu + %lu > %lu",
                    (long unsigned int) state->buffer_pos, (long unsigned int) size,
                    (long unsigned int) state->buffer_len);
        size = state->buffer_len - state->buffer_pos;
    }
    memcpy(state->buffer + state->buffer_pos, data, size);
    state->buffer_pos += size;
    return state->buffer_pos;
}

static size_t saving_percent(size_t comp_len, size_t src_len)
{
    if (comp_len > src_len)
        comp_len = src_len;
    if (!src_len)
        return 0;
    return 100 - comp_len * 100 / src_len;
}

/*
 * Decompress the compressed data in the internal buffer
 * Return the size of uncompressed data or 0 for error
 */
static size_t lz4_decompressor_decompress(struct decompressor_state *state)
{
    if (!state)
        return 0;
    if (!state->buffer) {
        error("%s: No decompressor buffer allocated", STREAM_COMPRESSION_MSG);
        return 0;
    }
    
    long int decompressed_size = LZ4_decompress_safe_continue(state->data->stream, state->buffer,
            state->data->stream_buffer + state->data->stream_buffer_pos,
            state->buffer_len, state->data->stream_buffer_size - state->data->stream_buffer_pos);
    if (decompressed_size < 0) {
        error("%s: Decompressor error %ld", STREAM_COMPRESSION_MSG, decompressed_size);
        return 0;
    }

    state->out_buffer = state->data->stream_buffer + state->data->stream_buffer_pos;
    state->data->stream_buffer_pos += decompressed_size;
    if (state->data->stream_buffer_pos >= state->data->stream_buffer_size - LZ4_MAX_MSG_SIZE)
        state->data->stream_buffer_pos = 0;
    state->out_buffer_len = decompressed_size;
    state->out_buffer_pos = 0;

    // Some compression statistics
    size_t old_avg_saving = saving_percent(state->total_compressed, state->total_uncompressed);
    size_t old_avg_size = state->packet_count ? state->total_uncompressed / state->packet_count : 0;

    state->total_compressed += state->buffer_len + SIGNATURE_SIZE;
    state->total_uncompressed += decompressed_size;
    state->packet_count++;

    size_t saving = saving_percent(state->buffer_len, decompressed_size);
    size_t avg_saving = saving_percent(state->total_compressed, state->total_uncompressed);
    size_t avg_size = state->total_uncompressed / state->packet_count;

    (void)saving;

    if (old_avg_saving != avg_saving || old_avg_size != avg_size){
        debug(D_STREAM, "%s: Saving: %lu%% (avg. %lu%%), avg.size: %lu", STREAM_COMPRESSION_MSG,
              (long unsigned int) saving, (long unsigned int) avg_saving, (long unsigned int) avg_size);
    }
    return decompressed_size;
}

/*
 * Return the size of uncompressed data left in the internal buffer or 0 for error
 */
static size_t lz4_decompressor_decompressed_bytes_in_buffer(struct decompressor_state *state)
{
    return state->out_buffer_len ?
        state->out_buffer_len - state->out_buffer_pos : 0;
}

/*
 * Fill the buffer provided with uncompressed data from the internal buffer
 * Return the size of uncompressed data copied or 0 for error
 */
static size_t lz4_decompressor_get(struct decompressor_state *state, char *data, size_t size)
{
    if (!state || !size || !data)
        return 0;
    if (!state->out_buffer)
        fatal("%s: No decompressor output buffer allocated", STREAM_COMPRESSION_MSG);
    if (state->out_buffer_pos + size > state->out_buffer_len)
        size = state->out_buffer_len - state->out_buffer_pos;
    
    char *p = state->out_buffer + state->out_buffer_pos, *endp = p + size, *last_lf = NULL;
    for (; p < endp; ++p)
        if (*p == '\n' || *p == 0)
            last_lf = p;
    if (last_lf)
        size = last_lf + 1 - (state->out_buffer + state->out_buffer_pos);

    memcpy(data, state->out_buffer + state->out_buffer_pos, size);
    state->out_buffer_pos += size;
    return size;
}

/*
 * Create and initialize decompressor state
 * Return the pointer to decompressor_state structure created
 */
struct decompressor_state *create_decompressor()
{
    struct decompressor_state *state = callocz(1, sizeof(struct decompressor_state));
    state->reset = lz4_decompressor_reset;
    state->start = lz4_decompressor_start;
    state->put = lz4_decompressor_put;
    state->decompress = lz4_decompressor_decompress;
    state->get = lz4_decompressor_get;
    state->decompressed_bytes_in_buffer = lz4_decompressor_decompressed_bytes_in_buffer;
    state->destroy = lz4_decompressor_destroy;

    state->data = callocz(1, sizeof(struct decompressor_data));
    fatal_assert(state->data);
    state->data->stream = LZ4_createStreamDecode();
    state->data->stream_buffer_size = LZ4_decoderRingBufferSize(LZ4_MAX_MSG_SIZE);
    state->data->stream_buffer = mallocz(state->data->stream_buffer_size);
    fatal_assert(state->data->stream_buffer);
    state->reset(state);
    debug(D_STREAM, "%s: Initialize streaming decompression!", STREAM_COMPRESSION_MSG);
    return state;
}
#endif
