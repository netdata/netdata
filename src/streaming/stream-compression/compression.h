// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_STREAM_COMPRESSION_H
#define NETDATA_STREAM_COMPRESSION_H 1

#include "libnetdata/libnetdata.h"

// signature MUST end with a newline

#if COMPRESSION_MAX_MSG_SIZE >= (COMPRESSION_MAX_CHUNK - COMPRESSION_MAX_OVERHEAD)
#error "COMPRESSION_MAX_MSG_SIZE >= (COMPRESSION_MAX_CHUNK - COMPRESSION_MAX_OVERHEAD)"
#endif

typedef uint32_t stream_compression_signature_t;
#define STREAM_COMPRESSION_SIGNATURE ((stream_compression_signature_t)('z' | 0x80) | (0x80 << 8) | (0x80 << 16) | ('\n' << 24))
#define STREAM_COMPRESSION_SIGNATURE_MASK ((stream_compression_signature_t) 0xffU | (0x80U << 8) | (0x80U << 16) | (0xffU << 24))
#define STREAM_COMPRESSION_SIGNATURE_SIZE sizeof(stream_compression_signature_t)

static inline stream_compression_signature_t stream_compress_encode_signature(size_t compressed_data_size) {
    stream_compression_signature_t len = ((compressed_data_size & 0x7f) | 0x80 | (((compressed_data_size & (0x7f << 7)) << 1) | 0x8000)) << 8;
    return len | STREAM_COMPRESSION_SIGNATURE;
}

typedef enum {
    COMPRESSION_ALGORITHM_NONE  = 0,
    COMPRESSION_ALGORITHM_ZSTD,
    COMPRESSION_ALGORITHM_LZ4,
    COMPRESSION_ALGORITHM_GZIP,
    COMPRESSION_ALGORITHM_BROTLI,

    // terminator
    COMPRESSION_ALGORITHM_MAX,
} compression_algorithm_t;

// this defines the order the algorithms will be selected by the receiver (parent)
#define STREAM_COMPRESSION_ALGORITHMS_ORDER "zstd lz4 brotli gzip"

// ----------------------------------------------------------------------------

typedef struct simple_ring_buffer {
    const char *data;
    size_t size;
    size_t read_pos;
    size_t write_pos;
} SIMPLE_RING_BUFFER;

static inline void simple_ring_buffer_reset(SIMPLE_RING_BUFFER *b) {
    b->read_pos = b->write_pos = 0;
}

static inline void simple_ring_buffer_make_room(SIMPLE_RING_BUFFER *b, size_t size) {
    if(b->write_pos + size > b->size) {
        if(!b->size)
            b->size = COMPRESSION_MAX_CHUNK;
        else
            b->size *= 2;

        if(b->write_pos + size > b->size)
            b->size += size;

        b->data = (const char *)reallocz((void *)b->data, b->size);
    }
}

static inline void simple_ring_buffer_append_data(SIMPLE_RING_BUFFER *b, const void *data, size_t size) {
    simple_ring_buffer_make_room(b, size);
    memcpy((void *)(b->data + b->write_pos), data, size);
    b->write_pos += size;
}

static inline void simple_ring_buffer_destroy(SIMPLE_RING_BUFFER *b) {
    freez((void *)b->data);
    b->data = NULL;
    b->read_pos = b->write_pos = b->size = 0;
}

// ----------------------------------------------------------------------------

struct compressor_state {
    bool initialized;
    compression_algorithm_t algorithm;

    SIMPLE_RING_BUFFER input;
    SIMPLE_RING_BUFFER output;

    int level;
    void *stream;

    struct {
        size_t total_compressed;
        size_t total_uncompressed;
        size_t total_compressions;
    } sender_locked;
};

void stream_compressor_init(struct compressor_state *state);
void stream_compressor_destroy(struct compressor_state *state);
size_t stream_compress(struct compressor_state *state, const char *data, size_t size, const char **out);

// ----------------------------------------------------------------------------

struct decompressor_state {
    bool initialized;
    compression_algorithm_t algorithm;
    size_t signature_size;

    size_t total_compressed;
    size_t total_uncompressed;
    size_t total_compressions;

    SIMPLE_RING_BUFFER output;

    void *stream;
};

void stream_decompressor_destroy(struct decompressor_state *state);
void stream_decompressor_init(struct decompressor_state *state);
size_t stream_decompress(struct decompressor_state *state, const char *compressed_data, size_t compressed_size);

static inline size_t stream_decompress_decode_signature(const char *data, size_t data_size) {
    if (unlikely(!data || !data_size))
        return 0;

    if (unlikely(data_size != STREAM_COMPRESSION_SIGNATURE_SIZE))
        return 0;

    stream_compression_signature_t sign;
    memcpy(&sign, data, sizeof(stream_compression_signature_t)); // Safe copy to aligned variable
    // stream_compression_signature_t sign = *(stream_compression_signature_t *)data;

    if (unlikely((sign & STREAM_COMPRESSION_SIGNATURE_MASK) != STREAM_COMPRESSION_SIGNATURE))
        return 0;

    size_t length = ((sign >> 8) & 0x7f) | ((sign >> 9) & (0x7f << 7));
    return length;
}

static inline size_t stream_decompressor_start(struct decompressor_state *state, const char *header, size_t header_size) {
    if(unlikely(state->output.read_pos != state->output.write_pos))
        fatal("STREAM_DECOMPRESS: asked to decompress new data, while there are unread data in the decompression buffer!");

    return stream_decompress_decode_signature(header, header_size);
}

static inline size_t stream_decompressed_bytes_in_buffer(struct decompressor_state *state) {
    if(unlikely(state->output.read_pos > state->output.write_pos))
        fatal("STREAM_DECOMPRESS: invalid read/write stream positions");

    return state->output.write_pos - state->output.read_pos;
}

static inline size_t stream_decompressor_get(struct decompressor_state *state, char *dst, size_t size) {
    if (unlikely(!state || !size || !dst))
        return 0;

    size_t remaining = stream_decompressed_bytes_in_buffer(state);

    if(unlikely(!remaining))
        return 0;

    size_t bytes_to_return = size;
    if(bytes_to_return > remaining)
        bytes_to_return = remaining;

    memcpy(dst, state->output.data + state->output.read_pos, bytes_to_return);
    state->output.read_pos += bytes_to_return;

    if(unlikely(state->output.read_pos > state->output.write_pos))
        fatal("STREAM_DECOMPRESS: invalid read/write stream positions");

    return bytes_to_return;
}

// ----------------------------------------------------------------------------

struct sender_state;
struct receiver_state;
struct stream_receiver_config;

bool stream_compression_initialize(struct sender_state *s);
bool stream_decompression_initialize(struct receiver_state *rpt);
void stream_parse_compression_order(struct stream_receiver_config *config, const char *order);
void stream_select_receiver_compression_algorithm(struct receiver_state *rpt);
void stream_compression_deactivate(struct sender_state *s);

#endif // NETDATA_STREAM_COMPRESSION_H 1
