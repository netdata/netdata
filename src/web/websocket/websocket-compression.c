// SPDX-License-Identifier: GPL-3.0-or-later

#include "websocket-internal.h"

/**
 * @brief Compresses a message into the client's c_payload buffer
 * 
 * This function compresses the given data using the client's deflate stream
 * and stores the result in the client's c_payload buffer.
 * 
 * @param wsc WebSocket client
 * @param data Data to compress
 * @param length Length of the data
 * @return true if compression was successful, false otherwise
 */
bool websocket_client_compress_message(WS_CLIENT *wsc, const char *data, size_t length) {
    if (!wsc || !data || !length || !wsc->compression.enabled || !wsc->compression.deflate_stream)
        return false;
        
    if (length < WS_COMPRESS_MIN_SIZE)
        return false;  // Too small to benefit from compression
        
    // Clear and prepare the compression buffer
    wsb_reset(&wsc->c_payload);
    
    z_stream *zstrm = wsc->compression.deflate_stream;
    
    // Calculate maximum possible compressed size
    uLong max_compressed_size = deflateBound(zstrm, length) + 4;  // +4 for Z_SYNC_FLUSH trailer
    
    // Ensure the buffer has enough capacity
    wsb_need_bytes(&wsc->c_payload, max_compressed_size);

    // Set up the deflate stream
    zstrm->next_in = (Bytef *)data;
    zstrm->avail_in = length;
    zstrm->next_out = (Bytef *)wsb_data(&wsc->c_payload);
    zstrm->avail_out = wsb_size(&wsc->c_payload);
    zstrm->total_in = 0;
    zstrm->total_out = 0;
    
    // Compress with sync flush
    int ret = deflate(zstrm, Z_SYNC_FLUSH);
    
    bool success = false;
    if (ret == Z_STREAM_END || (ret == Z_OK && zstrm->avail_in == 0 && zstrm->avail_out > 0))
        success = true;
    else if (ret == Z_OK && zstrm->avail_in == 0 && zstrm->avail_out == 0) {
        unsigned pending = 0;
        int bits = 0;
        if(deflatePending(zstrm, &pending, &bits) == Z_OK &&
            (pending == 0 && bits == 0))
            success = true;
    }
    
    uLong total_out = zstrm->total_out;
    
    // Reset the stream for future use
    if (deflateReset(zstrm) != Z_OK) {
        websocket_error(wsc, "Deflate reset failed");
        
        // Clear pointers for safety
        zstrm->next_in = NULL;
        zstrm->avail_in = 0;
        zstrm->next_out = NULL;
        zstrm->avail_out = 0;
        zstrm->total_in = 0;
        zstrm->total_out = 0;
        
        return false;
    }
    
    // Clear all pointers for safety
    zstrm->next_in = NULL;
    zstrm->avail_in = 0;
    zstrm->next_out = NULL;
    zstrm->avail_out = 0;
    zstrm->total_in = 0;
    zstrm->total_out = 0;
    
    if (!success || total_out <= 4) {
        // Compression failed or didn't save space
        websocket_debug(wsc, "Compression not beneficial (in=%zu, out=%lu) - not using compression",
                     length, total_out);
        return false;
    }
    
    // As per RFC 7692, remove trailing 4 bytes (00 00 FF FF) from Z_SYNC_FLUSH
    size_t compressed_size = total_out - 4;
    
    // Update the buffer's length
    wsb_set_length(&wsc->c_payload, compressed_size);
    
    websocket_debug(wsc, "Compressed message from %zu to %zu bytes (%.1f%%)",
                 length, compressed_size, (double)compressed_size * 100.0 / (double)length);
    
    return true;
}

// Initialize compression resources using the parsed options
bool websocket_compression_init(WS_CLIENT *wsc) {
    internal_fatal(wsc->wth->tid != gettid_cached(), "Function %s() should only be used by the websocket thread", __FUNCTION__ );

    if (!wsc->compression.enabled) {
        websocket_debug(wsc, "Compression is disabled");
        return false;
    }

    // Initialize deflate (compression) context for server-to-client messages
    wsc->compression.deflate_stream = mallocz(sizeof(z_stream));
    wsc->compression.deflate_stream->zalloc = Z_NULL;
    wsc->compression.deflate_stream->zfree = Z_NULL;
    wsc->compression.deflate_stream->opaque = Z_NULL;

    // Initialize with negative window bits for raw deflate (no zlib/gzip header)
    // Use server_max_window_bits for outgoing (server-to-client) messages
    int ret = deflateInit2(
        wsc->compression.deflate_stream,
        wsc->compression.compression_level,
        Z_DEFLATED,
        -wsc->compression.server_max_window_bits,
        WS_COMPRESS_MEMLEVEL,
        Z_DEFAULT_STRATEGY
    );

    if (ret != Z_OK) {
        websocket_error(wsc, "Failed to initialize deflate context: %s (%d)",
                   zError(ret), ret);
        freez(wsc->compression.deflate_stream);
        wsc->compression.deflate_stream = NULL;
        return false;
    }

    websocket_debug(wsc, "Compression initialized (server window bits: %d)",
                    wsc->compression.server_max_window_bits);

    return true;
}

// Initialize decompression resources for a client
bool websocket_decompression_init(WS_CLIENT *wsc) {
    internal_fatal(wsc->wth->tid != gettid_cached(), "Function %s() should only be used by the websocket thread", __FUNCTION__ );

    if (!wsc->compression.enabled) {
        websocket_debug(wsc, "Decompression is disabled");
        return false;
    }

    // Create a new inflate stream
    wsc->compression.inflate_stream = mallocz(sizeof(z_stream));
    wsc->compression.inflate_stream->zalloc = Z_NULL;
    wsc->compression.inflate_stream->zfree = Z_NULL;
    wsc->compression.inflate_stream->opaque = Z_NULL;

    // Initialize with negative window bits for raw deflate (no zlib/gzip header)
    // Use client_max_window_bits for incoming (client-to-server) messages
    int init_ret = inflateInit2(wsc->compression.inflate_stream, -wsc->compression.client_max_window_bits);

    if (init_ret != Z_OK) {
        websocket_error(wsc, "Failed to initialize inflate stream: %s (%d)",
                        zError(init_ret), init_ret);
        freez(wsc->compression.inflate_stream);
        wsc->compression.inflate_stream = NULL;
        return false;
    }

    websocket_debug(wsc, "Decompression initialized (client window bits: %d)",
                    wsc->compression.client_max_window_bits);

    return true;
}

// Clean up compression resources for a WebSocket client
void websocket_compression_cleanup(WS_CLIENT *wsc) {
    // Clean up deflate context
    if (!wsc->compression.deflate_stream)
        return;

    internal_fatal(wsc->wth->tid != gettid_cached(), "Function %s() should only be used by the websocket thread", __FUNCTION__ );

    // Set up dummy I/O pointers to ensure clean state
    unsigned char dummy_buffer[16] = {0};
    wsc->compression.deflate_stream->next_in = dummy_buffer;
    wsc->compression.deflate_stream->avail_in = 0;
    wsc->compression.deflate_stream->next_out = dummy_buffer;
    wsc->compression.deflate_stream->avail_out = sizeof(dummy_buffer);

    // Always call deflateEnd to release internal zlib resources
    // Don't bother with deflateReset as deflateEnd will clean up properly
    int ret = deflateEnd(wsc->compression.deflate_stream);

    if (ret != Z_OK && ret != Z_DATA_ERROR) {
        // Z_DATA_ERROR can happen in some edge cases, it's not critical here
        // as we're cleaning up anyway
        websocket_debug(wsc, "deflateEnd returned %d: %s", ret, zError(ret));
    }

    // Free the stream structure
    freez(wsc->compression.deflate_stream);
    wsc->compression.deflate_stream = NULL;

    websocket_debug(wsc, "Compression resources cleaned up");
}

// Clean up decompression resources for a client's inflate stream
void websocket_decompression_cleanup(WS_CLIENT *wsc) {
    if (!wsc->compression.inflate_stream)
        return;

    internal_fatal(wsc->wth->tid != gettid_cached(), "Function %s() should only be used by the websocket thread", __FUNCTION__ );

    // End the current inflate stream and free its resources
    inflateEnd(wsc->compression.inflate_stream);
    freez(wsc->compression.inflate_stream);
    wsc->compression.inflate_stream = NULL;

    websocket_debug(wsc, "Decompression resources cleaned up");
}

// Reset compression resources for a client - calls cleanup and init
ALWAYS_INLINE
bool websocket_compression_reset(WS_CLIENT *wsc) {
    websocket_compression_cleanup(wsc);
    return websocket_compression_init(wsc);
}

// Reset decompression resources for a client - calls cleanup and init
ALWAYS_INLINE
bool websocket_decompression_reset(WS_CLIENT *wsc) {
    websocket_decompression_cleanup(wsc);
    return websocket_decompression_init(wsc);
}

// Decompress a client's message from payload to u_payload
bool websocket_client_decompress_message(WS_CLIENT *wsc) {
    internal_fatal(wsc->wth->tid != gettid_cached(), "Function %s() should only be used by the websocket thread", __FUNCTION__ );

    if (!wsc->is_compressed || !wsc->compression.enabled || !wsc->compression.inflate_stream)
        return false;

    if (wsb_is_empty(&wsc->payload)) {
        websocket_debug(wsc, "Empty compressed message");
        wsb_reset(&wsc->u_payload);
        wsb_null_terminate(&wsc->u_payload);
        return true;
    }

    websocket_debug(wsc, "Decompressing message (%zu bytes)", wsb_length(&wsc->payload));

    z_stream *zstrm = wsc->compression.inflate_stream;
    wsb_reset(&wsc->u_payload);

    // Per RFC 7692, we need to append 4 bytes (00 00 FF FF) to the compressed data
    // to ensure the inflate operation completes
    static const unsigned char trailer[4] = {0x00, 0x00, 0xFF, 0xFF};
    wsb_append_padding(&wsc->payload, trailer, 4);

    zstrm->next_in = (Bytef *)wsb_data(&wsc->payload);
    zstrm->avail_in = wsb_length(&wsc->payload) + 4;
    zstrm->next_out = (Bytef *)wsb_data(&wsc->u_payload);
    zstrm->avail_out = wsb_size(&wsc->u_payload);
    zstrm->total_in = 0;
    zstrm->total_out = 0;

    // Decompression-bomb guard: bound the output buffer by a maximum ratio over the
    // compressed input (with a floor for small messages), in addition to the absolute
    // WS_MAX_DECOMPRESSED_SIZE cap. Without this, a ~100-byte payload of repeated bytes
    // can force the loop below to grow the buffer toward the 200MB ceiling (CWE-409).
    size_t compressed_len = wsb_length(&wsc->payload);
    size_t ratio_cap =
        (compressed_len > WS_MAX_DECOMPRESSED_SIZE / WS_MAX_DECOMPRESSION_RATIO)
            ? WS_MAX_DECOMPRESSED_SIZE
            : compressed_len * WS_MAX_DECOMPRESSION_RATIO;
    size_t max_decompressed = MIN(WS_MAX_DECOMPRESSED_SIZE, MAX(ratio_cap, WS_MIN_DECOMPRESSED_SIZE));

    // Give the loop one byte of headroom beyond the limit. These messages are raw
    // deflate (no Z_STREAM_END), so completion is detected by inflate leaving avail_out
    // > 0. The extra byte lets a message that decompresses to exactly max_decompressed
    // be recognized as complete (avail_out stays > 0) instead of being mistaken for an
    // overflow, while anything strictly larger still fills it and is rejected below.
    size_t cap = max_decompressed + 1;

    // Decompress with loop for multiple buffer expansions if needed
    int ret = Z_MEM_ERROR;
    bool success = false;
    int retries = 24;
    size_t wanted_size = MIN(MAX(wsb_size(&wsc->u_payload), compressed_len * 2), cap);
    do {
        wsb_resize(&wsc->u_payload, wanted_size);

        // Position next_out to point to the end of the currently decompressed data
        size_t len_before = wsb_length(&wsc->u_payload);
        zstrm->next_out = (Bytef *)wsb_data(&wsc->u_payload) + len_before;

        // Offer zlib the free space, but never let the decompressed output exceed the cap
        // (max_decompressed + 1 probe byte) - even if the physical buffer is larger. The
        // buffer is grow-only (wsb_resize never shrinks) and may have been enlarged by a
        // previous message on this connection (permessage-deflate context takeover), so
        // capping avail_out here - not the buffer size - is what actually enforces the limit.
        size_t offered = MIN(wsb_size(&wsc->u_payload) - len_before, cap - len_before);
        zstrm->avail_out = offered;

        // Try to decompress
        ret = inflate(zstrm, Z_SYNC_FLUSH);

        websocket_debug(wsc, "inflate() returned %d (%s), "
                             "avail_in=%u, avail_out=%u, total_in=%lu, total_out=%lu",
                        ret, zError(ret),
            zstrm->avail_in, zstrm->avail_out, zstrm->total_in, zstrm->total_out);

        // Handle different return codes from inflate()
        // Z_STREAM_END - Complete decompression success
        // Z_OK - Partial success, all input processed or output buffer full
        // Z_BUF_ERROR - Need more output space

        success = ret == Z_STREAM_END ||
            (zstrm->avail_in == 0 && zstrm->avail_out > 0 && (ret == Z_OK || ret == Z_BUF_ERROR));

        // Update the buffer's length to include the newly written data. We only offered
        // 'offered' bytes, so the bytes written this pass are (offered - avail_out).
        wsb_set_length(&wsc->u_payload, len_before + (offered - zstrm->avail_out));

        // Check if we need more output space
        if (!success && (ret == Z_BUF_ERROR || ret == Z_OK)) {
            if (wsb_length(&wsc->u_payload) > max_decompressed) {
                // We produced more than max_decompressed bytes (the probe byte was used)
                // and the message is still not complete: either a decompression bomb or a
                // message larger than we allow. Reject it (handled by the !success path).
                websocket_error(wsc,
                                "Decompression aborted: %zu compressed bytes expand beyond the %zu byte limit "
                                "(max ratio %llu:1) - possible decompression bomb",
                                compressed_len, max_decompressed, WS_MAX_DECOMPRESSION_RATIO);
                break;
            }
            wanted_size = MIN(wanted_size * 2, cap);
        }
    } while (!success && retries-- > 0);

    if(!success) {
        // Decompression failed
        websocket_error(wsc, "Decompression failed: %s (ret = %d, avail_in = %u)", zError(ret), ret, zstrm->avail_in);
        wsb_reset(&wsc->u_payload);
        websocket_decompression_reset(wsc);
        return false;
    }

    // Log successful decompression with detailed information
    websocket_debug(wsc, "Successfully decompressed %zu bytes to %zu bytes (ratio: %.2fx)",
                    wsb_length(&wsc->payload), wsb_length(&wsc->u_payload),
                    (double)wsb_length(&wsc->u_payload) / (double)wsb_length(&wsc->payload));

    // Show a preview of the decompressed data
    websocket_dump_debug(wsc, wsb_data(&wsc->u_payload), wsb_length(&wsc->u_payload), "RX UNCOMPRESSED PAYLOAD");

    // when client context takeover is disabled, reset the decompressor
    if (!wsc->compression.client_context_takeover) {
        websocket_debug(wsc, "resetting compression");
        if(inflateReset2(zstrm, -wsc->compression.client_max_window_bits) != Z_OK) {
            websocket_debug(wsc, "reset failed, re-initializing compression");
            if (!websocket_decompression_reset(wsc)) {
                websocket_debug(wsc, "re-initializing failed, reporting failure");
                return false;
            }
            zstrm = wsc->compression.inflate_stream;
        }
    }

    zstrm->next_in = NULL;
    zstrm->next_out = NULL;
    zstrm->avail_in = 0;
    zstrm->avail_out = 0;
    zstrm->total_in = 0;
    zstrm->total_out = 0;

    return true;
}

// ---------------------------------------------------------------------------
// Unit test: decompression-bomb guard (CWE-409)
// ---------------------------------------------------------------------------
//
// Exercises websocket_client_decompress_message()'s ratio/floor cap, including the
// grow-only-buffer regression: a previously decompressed large message enlarges
// u_payload (wsb_resize never shrinks), and under permessage-deflate context
// takeover the next tiny message must still be capped - the guard limits avail_out
// per pass, not the physical buffer size.

// Compress one message raw-deflate style and store it in 'out' the way the receiver
// expects it: WITHOUT the trailing 4-byte 00 00 FF FF sync marker (the receiver
// re-appends it). A persistent z_stream lets a sequence of messages share the
// compression window, mirroring the inflate side's client_context_takeover.
static bool ws_test_compress_msg(z_stream *def, const char *in, size_t in_len, WS_BUF *out) {
    wsb_reset(out);
    size_t bound = (size_t)deflateBound(def, (uLong)in_len) + 64;
    wsb_resize(out, bound);

    def->next_in = (Bytef *)in;
    def->avail_in = (uInt)in_len;
    def->next_out = (Bytef *)wsb_data(out);
    def->avail_out = (uInt)bound;

    int ret = deflate(def, Z_SYNC_FLUSH);
    if ((ret != Z_OK && ret != Z_BUF_ERROR) || def->avail_in != 0)
        return false;

    size_t produced = bound - def->avail_out;
    if (produced < 4)
        return false;

    // strip the 00 00 FF FF emitted by Z_SYNC_FLUSH (the receiver re-adds it)
    wsb_set_length(out, produced - 4);
    return true;
}

// Deterministic low-compressibility fill (xorshift32) so a "legit" large message is
// accepted under the ratio cap and grows u_payload. Avoids Math.random()/time().
static void ws_test_fill_incompressible(char *buf, size_t len) {
    uint32_t x = 0x9e3779b9u;
    for (size_t i = 0; i < len; i++) {
        x ^= x << 13; x ^= x >> 17; x ^= x << 5;
        buf[i] = (char)(x & 0xff);
    }
}

// Build a fresh client + matching test compressor. Independent per case so a
// rejection (which resets the inflate stream) cannot desync later cases.
static WS_CLIENT *ws_test_client_create(WEBSOCKET_THREAD *wth, z_stream *def) {
    WS_CLIENT *wsc = callocz(1, sizeof(*wsc));
    wsc->wth = wth;
    wsc->compression = WEBSOCKET_COMPRESSION_DEFAULTS;
    wsc->compression.enabled = true;
    wsc->is_compressed = true;

    if (!websocket_decompression_init(wsc)) {
        freez(wsc);
        return NULL;
    }

    memset(def, 0, sizeof(*def));
    if (deflateInit2(def, Z_DEFAULT_COMPRESSION, Z_DEFLATED,
                     -wsc->compression.client_max_window_bits, 8, Z_DEFAULT_STRATEGY) != Z_OK) {
        websocket_decompression_cleanup(wsc);
        freez(wsc);
        return NULL;
    }
    return wsc;
}

static void ws_test_client_destroy(WS_CLIENT *wsc, z_stream *def) {
    deflateEnd(def);
    websocket_decompression_cleanup(wsc);
    wsb_cleanup(&wsc->payload);
    wsb_cleanup(&wsc->u_payload);
    wsb_cleanup(&wsc->c_payload);
    freez(wsc);
}

int websocket_compression_unittest(void) {
    fprintf(stderr, "\n%s() running...\n", __FUNCTION__);
    int errors = 0;

    WEBSOCKET_THREAD wth = { 0 };
    wth.tid = gettid_cached(); // satisfy the collector-thread internal_fatal in decompress

    // Case 1: a legit small message round-trips correctly (guard must not break normal traffic)
    {
        z_stream def;
        WS_CLIENT *wsc = ws_test_client_create(&wth, &def);
        if (!wsc) { fprintf(stderr, "  FAILED setup (case 1)\n"); return 1; }

        char msg[4096];
        ws_test_fill_incompressible(msg, sizeof(msg)); // < WS_MIN_DECOMPRESSED_SIZE, must pass
        if (!ws_test_compress_msg(&def, msg, sizeof(msg), &wsc->payload)) {
            fprintf(stderr, "  FAILED case1: test compressor error\n"); errors++;
        }
        else if (!websocket_client_decompress_message(wsc)) {
            fprintf(stderr, "  FAILED case1: legit message rejected\n"); errors++;
        }
        else if (wsb_length(&wsc->u_payload) != sizeof(msg) ||
                 memcmp(wsb_data(&wsc->u_payload), msg, sizeof(msg)) != 0) {
            fprintf(stderr, "  FAILED case1: round-trip mismatch (%zu bytes)\n", wsb_length(&wsc->u_payload));
            errors++;
        }
        ws_test_client_destroy(wsc, &def);
    }

    // Case 2: a tiny payload that inflates past the floor cap is rejected (classic bomb)
    {
        z_stream def;
        WS_CLIENT *wsc = ws_test_client_create(&wth, &def);
        if (!wsc) { fprintf(stderr, "  FAILED setup (case 2)\n"); return 1; }

        size_t bomb_len = 4 * 1024 * 1024; // 4MB of zeros -> tiny compressed, cap = 1MB floor
        char *bomb = callocz(1, bomb_len);
        if (!ws_test_compress_msg(&def, bomb, bomb_len, &wsc->payload)) {
            fprintf(stderr, "  FAILED case2: test compressor error\n"); errors++;
        }
        else if (websocket_client_decompress_message(wsc)) {
            fprintf(stderr, "  FAILED case2: %zuB->%zuB bomb accepted (should be rejected)\n",
                    wsb_length(&wsc->payload), wsb_length(&wsc->u_payload));
            errors++;
        }
        freez(bomb);
        ws_test_client_destroy(wsc, &def);
    }

    // Case 3 (regression): a legit large message grows u_payload, then a tiny bomb on the
    // SAME client (context takeover) must STILL be rejected. Pre-fix, the retained large
    // buffer let the bomb complete in one inflate pass and be accepted.
    {
        z_stream def;
        WS_CLIENT *wsc = ws_test_client_create(&wth, &def);
        if (!wsc) { fprintf(stderr, "  FAILED setup (case 3)\n"); return 1; }

        // (a) legit large: ~4MB incompressible -> ratio ~1 -> accepted, grows u_payload
        size_t big_len = 4 * 1024 * 1024;
        char *big = mallocz(big_len);
        ws_test_fill_incompressible(big, big_len);
        bool a_ok = ws_test_compress_msg(&def, big, big_len, &wsc->payload) &&
                    websocket_client_decompress_message(wsc);
        size_t grown = wsb_size(&wsc->u_payload);
        if (!a_ok) {
            fprintf(stderr, "  FAILED case3a: legit large message rejected\n"); errors++;
        }
        else if (grown < big_len) {
            fprintf(stderr, "  FAILED case3a: u_payload did not grow as expected (%zu)\n", grown); errors++;
        }
        freez(big);

        // (b) tiny bomb on the same client: 4MB zeros -> cap 1MB -> must be rejected even
        // though u_payload capacity is already ~4MB.
        if (a_ok) {
            size_t bomb_len = 4 * 1024 * 1024;
            char *bomb = callocz(1, bomb_len);
            if (!ws_test_compress_msg(&def, bomb, bomb_len, &wsc->payload)) {
                fprintf(stderr, "  FAILED case3b: test compressor error\n"); errors++;
            }
            else if (websocket_client_decompress_message(wsc)) {
                fprintf(stderr, "  FAILED case3b: bomb accepted despite retained %zuB buffer "
                                "(grow-only bypass regression)\n", grown);
                errors++;
            }
            freez(bomb);
        }
        ws_test_client_destroy(wsc, &def);
    }

    // Case 4 (boundary): a message that decompresses to EXACTLY the cap must be accepted,
    // not mistaken for an overflow. 1MB of zeros compresses to ~1KB, so the floor applies
    // (max_decompressed == WS_MIN_DECOMPRESSED_SIZE) and the output is exactly at the limit.
    {
        z_stream def;
        WS_CLIENT *wsc = ws_test_client_create(&wth, &def);
        if (!wsc) { fprintf(stderr, "  FAILED setup (case 4)\n"); return 1; }

        size_t exact_len = WS_MIN_DECOMPRESSED_SIZE; // == max_decompressed for this tiny input
        char *exact = callocz(1, exact_len);
        if (!ws_test_compress_msg(&def, exact, exact_len, &wsc->payload)) {
            fprintf(stderr, "  FAILED case4: test compressor error\n"); errors++;
        }
        else if (!websocket_client_decompress_message(wsc)) {
            fprintf(stderr, "  FAILED case4: exact-limit %zuB message falsely rejected\n", exact_len);
            errors++;
        }
        else if (wsb_length(&wsc->u_payload) != exact_len) {
            fprintf(stderr, "  FAILED case4: accepted but length %zu != %zu\n",
                    wsb_length(&wsc->u_payload), exact_len);
            errors++;
        }
        freez(exact);
        ws_test_client_destroy(wsc, &def);
    }

    if (errors)
        fprintf(stderr, "%s() FAILED with %d error(s)\n", __FUNCTION__, errors);
    else
        fprintf(stderr, "%s() passed\n", __FUNCTION__);

    return errors ? 1 : 0;
}
