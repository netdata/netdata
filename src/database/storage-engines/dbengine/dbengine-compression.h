// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_DBENGINE_COMPRESSION_H
#define NETDATA_DBENGINE_COMPRESSION_H

struct dbengine_engine;

uint8_t dbengine_default_compression(void);

bool dbengine_valid_compression_algorithm(uint8_t algorithm);

size_t dbengine_max_compressed_size(size_t uncompressed_size, uint8_t algorithm);
size_t dbengine_compress(void *payload, size_t uncompressed_size, uint8_t algorithm);

// engine: whose log sink hears about a failure; NULL for netdata's logger
size_t dbengine_decompress(struct dbengine_engine *engine, void *dst, void *src, size_t dst_size, size_t src_size,
                           uint8_t algorithm);

#endif //NETDATA_DBENGINE_COMPRESSION_H
