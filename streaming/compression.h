#include "rrdpush.h"

#ifndef NETDATA_RRDPUSH_COMPRESSION_H
#define NETDATA_RRDPUSH_COMPRESSION_H 1

#ifdef ENABLE_RRDPUSH_COMPRESSION

// signature MUST end with a newline
typedef uint32_t rrdpush_signature_t;
#define RRDPUSH_COMPRESSION_SIGNATURE ((rrdpush_signature_t)('z' | 0x80) | (0x80 << 8) | (0x80 << 16) | ('\n' << 24))
#define RRDPUSH_COMPRESSION_SIGNATURE_MASK ((rrdpush_signature_t)0xff | (0x80 << 8) | (0x80 << 16) | (0xff << 24))
#define RRDPUSH_COMPRESSION_SIGNATURE_SIZE sizeof(rrdpush_signature_t)

static inline uint32_t rrdpush_compress_encode_signature(size_t compressed_data_size) {
    uint32_t len = ((compressed_data_size & 0x7f) | 0x80 | (((compressed_data_size & (0x7f << 7)) << 1) | 0x8000)) << 8;
    return len | RRDPUSH_COMPRESSION_SIGNATURE;
}

typedef enum {
    COMPRESSION_ALGORITHM_NONE = 0,
    COMPRESSION_ALGORITHM_LZ4,
    COMPRESSION_ALGORITHM_ZSTD
} compression_algorithm_t;

struct compression_ring_buffer {
    const char *data;
    size_t size;
    size_t read_pos;
    size_t write_pos;
};

struct compressor_state {
    bool initialized;
    compression_algorithm_t algorithm;

    struct compression_ring_buffer input;
    struct compression_ring_buffer output;

    void *stream;
};

void rrdpush_compressor_init(struct compressor_state *state);
void rrdpush_compressor_destroy(struct compressor_state *state);
size_t rrdpush_compress(struct compressor_state *state, const char *data, size_t size, const char **out);

struct decompressor_state {
    bool initialized;
    compression_algorithm_t algorithm;
    size_t signature_size;
    size_t total_compressed;
    size_t total_uncompressed;
    size_t packet_count;

    struct compression_ring_buffer output;

    void *stream;
};

void rrdpush_decompressor_destroy(struct decompressor_state *state);
void rrdpush_decompressor_init(struct decompressor_state *state);
size_t rrdpush_decompress(struct decompressor_state *state, const char *compressed_data, size_t compressed_size);

static inline size_t rrdpush_decompress_decode_signature(const char *data, size_t data_size) {
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
    if(unlikely(state->output.read_pos != state->output.write_pos))
        fatal("RRDPUSH DECOMPRESS: asked to decompress new data, while there are unread data in the decompression buffer!");

    return rrdpush_decompress_decode_signature(header, header_size);
}

static inline size_t rrdpush_decompressed_bytes_in_buffer(struct decompressor_state *state) {
    if(unlikely(state->output.read_pos > state->output.write_pos))
        fatal("RRDPUSH DECOMPRESS: invalid read/write stream positions");

    return state->output.write_pos - state->output.read_pos;
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

    memcpy(dst, state->output.data + state->output.read_pos, bytes_to_return);
    state->output.read_pos += bytes_to_return;

    if(unlikely(state->output.read_pos > state->output.write_pos))
        fatal("RRDPUSH DECOMPRESS: invalid read/write stream positions");

    return bytes_to_return;
}

#endif // ENABLE_RRDPUSH_COMPRESSION
#endif // NETDATA_RRDPUSH_COMPRESSION_H 1
