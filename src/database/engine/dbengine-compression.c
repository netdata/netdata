// SPDX-License-Identifier: GPL-3.0-or-later

#include "rrdengine.h"
#include "dbengine-compression.h"

bool dbengine_valid_compression_algorithm(uint8_t algorithm) {
    switch(algorithm) {
        case RRDENG_COMPRESSION_NONE:
        case RRDENG_COMPRESSION_LZ4:
            return true;

        default:
            return false;
    }
}

size_t dbengine_max_compressed_size(size_t uncompressed_size, uint8_t algorithm) {
    switch(algorithm) {
        case RRDENG_COMPRESSION_LZ4:
            fatal_assert(uncompressed_size < LZ4_MAX_INPUT_SIZE);
            return LZ4_compressBound((int)uncompressed_size);

        case RRDENG_COMPRESSION_NONE:
            return uncompressed_size;

        default:
            fatal("DBENGINE: unknown compression algorithm %u", algorithm);
    }
}

size_t dbengine_compress(void *payload, size_t uncompressed_size, uint8_t algorithm) {
    // the result should be stored in the payload
    // the caller must have called dbengine_max_compressed_size() to make sure the
    // payload is big enough to fit the max size needed.

    switch(algorithm) {
        case RRDENG_COMPRESSION_LZ4:
            {
                size_t max_compressed_size = dbengine_max_compressed_size(uncompressed_size, algorithm);
                struct extent_buffer *eb = extent_buffer_get(max_compressed_size);
                void *compressed_buf = eb->data;

                size_t compressed_size =
                    LZ4_compress_default(payload, compressed_buf, (int)uncompressed_size, (int)max_compressed_size);

                memcpy(payload, compressed_buf, compressed_size);
                extent_buffer_release(eb);
                return compressed_size;
            }

        case RRDENG_COMPRESSION_NONE:
            return uncompressed_size;

        default:
            fatal("DBENGINE: unknown compression algorithm %u", algorithm);
    }
}

size_t dbengine_decompress(void *dst, void *src, size_t dst_size, size_t src_size, uint8_t algorithm) {
    switch(algorithm) {
        case RRDENG_COMPRESSION_LZ4:
            return LZ4_decompress_safe(src, dst, (int)src_size, (int)dst_size);

        case RRDENG_COMPRESSION_NONE:
            fatal("DBENGINE: %s() should not be called for uncompressed pages", __FUNCTION__ );

        default:
            fatal("DBENGINE: unknown compression algorithm %u", algorithm);
    }
}
