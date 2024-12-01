// SPDX-License-Identifier: GPL-3.0-or-later

#include "compression.h"

#ifndef NETDATA_STREAMING_COMPRESSION_LZ4_H
#define NETDATA_STREAMING_COMPRESSION_LZ4_H

#ifdef ENABLE_LZ4

void stream_compressor_init_lz4(struct compressor_state *state);
void stream_compressor_destroy_lz4(struct compressor_state *state);
size_t stream_compress_lz4(struct compressor_state *state, const char *data, size_t size, const char **out);
size_t stream_decompress_lz4(struct decompressor_state *state, const char *compressed_data, size_t compressed_size);
void stream_decompressor_init_lz4(struct decompressor_state *state);
void stream_decompressor_destroy_lz4(struct decompressor_state *state);

#endif // ENABLE_LZ4

#endif //NETDATA_STREAMING_COMPRESSION_LZ4_H
