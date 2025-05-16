// SPDX-License-Identifier: GPL-3.0-or-later

#include "websocket-internal.h"

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

    // Decompress with loop for multiple buffer expansions if needed
    int ret = Z_MEM_ERROR;
    bool success = false;
    int retries = 24;
    size_t wanted_size = MAX(wsb_size(&wsc->u_payload), wsb_length(&wsc->payload) * 2);
    do {
        wsb_resize(&wsc->u_payload, wanted_size);

        // Position next_out to point to the end of the currently decompressed data
        zstrm->next_out = (Bytef *)wsb_data(&wsc->u_payload) + wsb_length(&wsc->u_payload);

        // Only make the newly available space available to zlib
        zstrm->avail_out = wsb_size(&wsc->u_payload) - wsb_length(&wsc->u_payload);

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

        // Update the buffer's length to include the newly written data
        wsb_set_length(&wsc->u_payload, wsb_size(&wsc->u_payload) - zstrm->avail_out);

        // Check if we need more output space
        if (!success && (ret == Z_BUF_ERROR || ret == Z_OK)) {
            wanted_size = MIN(wanted_size * 2, WS_MAX_DECOMPRESSED_SIZE);
            if (wanted_size == WS_MAX_DECOMPRESSED_SIZE && wanted_size == wsb_size(&wsc->u_payload))
                break; // we cannot resize more
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
