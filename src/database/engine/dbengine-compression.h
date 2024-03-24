// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_DBENGINE_COMPRESSION_H
#define NETDATA_DBENGINE_COMPRESSION_H

uint8_t dbengine_default_compression(void);

bool dbengine_valid_compression_algorithm(uint8_t algorithm);

size_t dbengine_max_compressed_size(size_t uncompressed_size, uint8_t algorithm);
size_t dbengine_compress(void *payload, size_t uncompressed_size, uint8_t algorithm);

size_t dbengine_decompress(void *dst, void *src, size_t dst_size, size_t src_size, uint8_t algorithm);

#endif //NETDATA_DBENGINE_COMPRESSION_H
