#include "rrdpush.h"

#ifndef NETDATA_RRDPUSH_COMPRESSION_H
#define NETDATA_RRDPUSH_COMPRESSION_H 1

#ifdef ENABLE_RRDPUSH_COMPRESSION

// signature MUST end with a newline
#define RRDPUSH_COMPRESSION_SIGNATURE ((uint32_t)('z' | 0x80) | (0x80 << 8) | (0x80 << 16) | ('\n' << 24))
#define RRDPUSH_COMPRESSION_SIGNATURE_MASK ((uint32_t)0xff | (0x80 << 8) | (0x80 << 16) | (0xff << 24))
#define RRDPUSH_COMPRESSION_SIGNATURE_SIZE 4

struct compressor_state {
    bool initialized;
    char *compression_result_buffer;
    size_t compression_result_buffer_size;
    struct {
        void *lz4_stream;
        char *input_ring_buffer;
        size_t input_ring_buffer_size;
        size_t input_ring_buffer_pos;
    } stream;
    size_t (*compress)(struct compressor_state *state, const char *data, size_t size, char **buffer);
    void (*destroy)(struct compressor_state **state);
};

void rrdpush_compressor_reset(struct compressor_state *state);
void rrdpush_compressor_destroy(struct compressor_state *state);
size_t rrdpush_compress(struct compressor_state *state, const char *data, size_t size, char **out);

struct decompressor_state {
    bool initialized;
    size_t signature_size;
    size_t total_compressed;
    size_t total_uncompressed;
    size_t packet_count;
    struct {
        void *lz4_stream;
        char *buffer;
        size_t size;
        size_t write_at;
        size_t read_at;
    } stream;
};

void rrdpush_decompressor_destroy(struct decompressor_state *state);
void rrdpush_decompressor_reset(struct decompressor_state *state);
size_t rrdpush_decompress(struct decompressor_state *state, const char *compressed_data, size_t compressed_size);

static inline size_t rrdpush_decompress_decode_header(const char *data, size_t data_size) {
    if (unlikely(!data || !data_size))
        return 0;

    if (unlikely(data_size != RRDPUSH_COMPRESSION_SIGNATURE_SIZE))
        return 0;

    uint32_t sign = *(uint32_t *)data;
    if (unlikely((sign & RRDPUSH_COMPRESSION_SIGNATURE_MASK) != RRDPUSH_COMPRESSION_SIGNATURE))
        return 0;

    size_t length = ((sign >> 8) & 0x7f) | ((sign >> 9) & (0x7f << 7));
    return length;
}

static inline size_t rrdpush_decompressor_start(struct decompressor_state *state, const char *header, size_t header_size) {
    if(unlikely(state->stream.read_at != state->stream.write_at))
        fatal("RRDPUSH DECOMPRESS: asked to decompress new data, while there are unread data in the decompression buffer!");

    return rrdpush_decompress_decode_header(header, header_size);
}

static inline size_t rrdpush_decompressed_bytes_in_buffer(struct decompressor_state *state) {
    if(unlikely(state->stream.read_at > state->stream.write_at))
        fatal("RRDPUSH DECOMPRESS: invalid read/write stream positions");

    return state->stream.write_at - state->stream.read_at;
}

static inline size_t rrdpush_decompressor_get(struct decompressor_state *state, char *dst, size_t size) {
    if (unlikely(!state || !size || !dst))
        return 0;

    size_t remaining = rrdpush_decompressed_bytes_in_buffer(state);

    if(unlikely(!remaining))
        return 0;

    size_t bytes_to_return = size;
    if(bytes_to_return > remaining)
        bytes_to_return = remaining;

    memcpy(dst, state->stream.buffer + state->stream.read_at, bytes_to_return);
    state->stream.read_at += bytes_to_return;

    if(unlikely(state->stream.read_at > state->stream.write_at))
        fatal("RRDPUSH DECOMPRESS: invalid read/write stream positions");

    return bytes_to_return;
}

#endif // ENABLE_RRDPUSH_COMPRESSION
#endif // NETDATA_RRDPUSH_COMPRESSION_H 1
