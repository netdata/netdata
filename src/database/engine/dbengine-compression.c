// SPDX-License-Identifier: GPL-3.0-or-later

#include "rrdengine.h"
#include "dbengine-compression.h"

#ifdef ENABLE_LZ4
#include <lz4.h>
#endif

#ifdef ENABLE_ZSTD
#include <zstd.h>
#define DBENGINE_ZSTD_DEFAULT_COMPRESSION_LEVEL 3
#endif

uint8_t dbengine_default_compression(void) {

#ifdef ENABLE_ZSTD
    return RRDENG_COMPRESSION_ZSTD;
#endif

#ifdef ENABLE_LZ4
    return RRDENG_COMPRESSION_LZ4;
#endif

    return RRDENG_COMPRESSION_NONE;
}

bool dbengine_valid_compression_algorithm(uint8_t algorithm) {
    switch(algorithm) {
        case RRDENG_COMPRESSION_NONE:

#ifdef ENABLE_LZ4
        case RRDENG_COMPRESSION_LZ4:
#endif

#ifdef ENABLE_ZSTD
        case RRDENG_COMPRESSION_ZSTD:
#endif

            return true;

        default:
            return false;
    }
}

size_t dbengine_max_compressed_size(size_t uncompressed_size, uint8_t algorithm) {
    switch(algorithm) {
#ifdef ENABLE_LZ4
        case RRDENG_COMPRESSION_LZ4:
            fatal_assert(uncompressed_size < LZ4_MAX_INPUT_SIZE);
            return LZ4_compressBound((int)uncompressed_size);
#endif

#ifdef ENABLE_ZSTD
        case RRDENG_COMPRESSION_ZSTD:
            return ZSTD_compressBound(uncompressed_size);
#endif

        case RRDENG_COMPRESSION_NONE:
            return uncompressed_size;

        default: {
            fatal("DBENGINE: unknown compression algorithm %u", algorithm);
            //we will never reach this point, but we have warnings from compiler
            return 0;
        }
    }
}

size_t dbengine_compress(void *payload, size_t uncompressed_size, uint8_t algorithm) {
    // the result should be stored in the payload
    // the caller must have called dbengine_max_compressed_size() to make sure the
    // payload is big enough to fit the max size needed.

    switch(algorithm) {
#ifdef ENABLE_LZ4
        case RRDENG_COMPRESSION_LZ4: {
            size_t max_compressed_size = dbengine_max_compressed_size(uncompressed_size, algorithm);
            struct extent_buffer *eb = extent_buffer_get(max_compressed_size);
            void *compressed_buf = eb->data;

            size_t compressed_size =
                LZ4_compress_default(payload, compressed_buf, (int)uncompressed_size, (int)max_compressed_size);

            if(compressed_size > 0 && compressed_size < uncompressed_size)
                memcpy(payload, compressed_buf, compressed_size);
            else
                compressed_size = 0;

            extent_buffer_release(eb);
            return compressed_size;
        }
#endif

#ifdef ENABLE_ZSTD
        case RRDENG_COMPRESSION_ZSTD: {
            size_t max_compressed_size = dbengine_max_compressed_size(uncompressed_size, algorithm);
            struct extent_buffer *eb = extent_buffer_get(max_compressed_size);
            void *compressed_buf = eb->data;

            size_t compressed_size = ZSTD_compress(compressed_buf, max_compressed_size, payload, uncompressed_size,
                                                   DBENGINE_ZSTD_DEFAULT_COMPRESSION_LEVEL);

            if (ZSTD_isError(compressed_size)) {
                internal_fatal(true, "DBENGINE: ZSTD compression error %s", ZSTD_getErrorName(compressed_size));
                compressed_size = 0;
            }

            if(compressed_size > 0 && compressed_size < uncompressed_size)
                memcpy(payload, compressed_buf, compressed_size);
            else
                compressed_size = 0;

            extent_buffer_release(eb);
            return compressed_size;
        }
#endif

        case RRDENG_COMPRESSION_NONE:
            return 0;

        default: {
            fatal("DBENGINE: unknown compression algorithm %u", algorithm);
            //we will never reach this point, but we have warnings from compiler
            return 0;
        }
    }
}

size_t dbengine_decompress(void *dst, void *src, size_t dst_size, size_t src_size, uint8_t algorithm) {
    switch(algorithm) {

#ifdef ENABLE_LZ4
        case RRDENG_COMPRESSION_LZ4: {
            int rc = LZ4_decompress_safe(src, dst, (int)src_size, (int)dst_size);
            if(rc < 0) {
                nd_log(NDLS_DAEMON, NDLP_ERR, "DBENGINE: ZSTD decompression error %d", rc);
                rc = 0;
            }

            return rc;
        }
#endif

#ifdef ENABLE_ZSTD
        case RRDENG_COMPRESSION_ZSTD: {
            size_t decompressed_size = ZSTD_decompress(dst, dst_size, src, src_size);

            if (ZSTD_isError(decompressed_size)) {
                nd_log(NDLS_DAEMON, NDLP_ERR, "DBENGINE: ZSTD decompression error %s",
                       ZSTD_getErrorName(decompressed_size));

                decompressed_size = 0;
            }

            return decompressed_size;
        }
#endif

        case RRDENG_COMPRESSION_NONE:
            internal_fatal(true, "DBENGINE: %s() should not be called for uncompressed pages", __FUNCTION__ );
            return 0;

        default:
            internal_fatal(true, "DBENGINE: unknown compression algorithm %u", algorithm);
            return 0;
    }
}
