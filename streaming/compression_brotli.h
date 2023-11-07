// SPDX-License-Identifier: GPL-3.0-or-later

#include "compression.h"

#ifndef NETDATA_STREAMING_COMPRESSION_BROTLI_H
#define NETDATA_STREAMING_COMPRESSION_BROTLI_H

void rrdpush_compressor_init_brotli(struct compressor_state *state);
void rrdpush_compressor_destroy_brotli(struct compressor_state *state);
size_t rrdpush_compress_brotli(struct compressor_state *state, const char *data, size_t size, const char **out);
size_t rrdpush_decompress_brotli(struct decompressor_state *state, const char *compressed_data, size_t compressed_size);
void rrdpush_decompressor_init_brotli(struct decompressor_state *state);
void rrdpush_decompressor_destroy_brotli(struct decompressor_state *state);

#endif //NETDATA_STREAMING_COMPRESSION_BROTLI_H
