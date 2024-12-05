// SPDX-License-Identifier: GPL-3.0-or-later

#include "compression.h"

#ifndef NETDATA_STREAMING_COMPRESSION_ZSTD_H
#define NETDATA_STREAMING_COMPRESSION_ZSTD_H

#ifdef ENABLE_ZSTD

void stream_compressor_init_zstd(struct compressor_state *state);
void stream_compressor_destroy_zstd(struct compressor_state *state);
size_t stream_compress_zstd(struct compressor_state *state, const char *data, size_t size, const char **out);
size_t stream_decompress_zstd(struct decompressor_state *state, const char *compressed_data, size_t compressed_size);
void stream_decompressor_init_zstd(struct decompressor_state *state);
void stream_decompressor_destroy_zstd(struct decompressor_state *state);

#endif // ENABLE_ZSTD

#endif //NETDATA_STREAMING_COMPRESSION_ZSTD_H
