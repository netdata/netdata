// SPDX-License-Identifier: GPL-3.0-or-later

#include "daemon/common.h"
#include "websocket-compression.h"
#include "websocket-internal.h"

// Check if zlib is available and working
bool websocket_compression_supported(void) {
    // Simple version check for zlib
    const char *version = zlibVersion();
    if (!version || version[0] == '\0')
        return false;
    
    return true;
}

// Initialize compression for a WebSocket client
bool websocket_compression_init(struct websocket_server_client *wsc, const char *extensions) {
    if (!wsc)
        return false;
    
    // Clean up any existing compression resources to prevent leaks
    websocket_compression_cleanup(wsc);
    
    // Initialize compression context to default values
    wsc->compression.type = WS_COMPRESS_NONE;
    wsc->compression.enabled = false;
    wsc->compression.client_context_takeover = true;
    wsc->compression.server_context_takeover = true;
    wsc->compression.window_bits = WS_COMPRESS_WINDOW_BITS;
    wsc->compression.compression_level = WS_COMPRESS_DEFAULT_LEVEL;
    wsc->compression.deflate_stream = NULL;
    wsc->compression.inflate_stream = NULL;
    
    // If extensions is NULL or empty, compression is disabled
    if (!extensions || *extensions == '\0')
        return true;
    
    // Check if "permessage-deflate" is requested
    if (strstr(extensions, "permessage-deflate") != NULL) {
        // Parse extension parameters
        char extension_copy[1024];
        strncpy(extension_copy, extensions, sizeof(extension_copy) - 1);
        extension_copy[sizeof(extension_copy) - 1] = '\0';
        
        char *token, *saveptr;
        token = strtok_r(extension_copy, ",", &saveptr);
        
        while (token) {
            // Trim leading/trailing spaces
            char *ext = token;
            while (*ext && isspace(*ext)) ext++;
            char *end = ext + strlen(ext) - 1;
            while (end > ext && isspace(*end)) *end-- = '\0';
            
            // Check if this is permessage-deflate extension
            if (strncmp(ext, "permessage-deflate", 18) == 0) {
                wsc->compression.type = WS_COMPRESS_DEFLATE;
                wsc->compression.enabled = true;
                
                // Parse parameters
                char *params = ext + 18;
                if (*params == ';') {
                    params++;
                    
                    char *param, *param_saveptr;
                    param = strtok_r(params, ";", &param_saveptr);
                    
                    while (param) {
                        // Trim leading/trailing spaces
                        while (*param && isspace(*param)) param++;
                        end = param + strlen(param) - 1;
                        while (end > param && isspace(*end)) *end-- = '\0';
                        
                        // Client no context takeover
                        if (strcmp(param, "client_no_context_takeover") == 0) {
                            wsc->compression.client_context_takeover = false;
                        }
                        // Server no context takeover
                        else if (strcmp(param, "server_no_context_takeover") == 0) {
                            wsc->compression.server_context_takeover = false;
                        }
                        // Client max window bits
                        else if (strncmp(param, "client_max_window_bits", 22) == 0) {
                            // Just acknowledge, we'll use our defaults
                        }
                        // Server max window bits
                        else if (strncmp(param, "server_max_window_bits=", 23) == 0) {
                            int bits = atoi(param + 23);
                            if (bits >= 8 && bits <= 15) {
                                wsc->compression.window_bits = bits;
                            }
                        }
                        
                        param = strtok_r(NULL, ";", &param_saveptr);
                    }
                }
                
                break;  // Found and parsed permessage-deflate
            }
            
            token = strtok_r(NULL, ",", &saveptr);
        }
    }
    
    // Initialize zlib contexts if compression is enabled
    if (wsc->compression.enabled) {
        // Initialize deflate (compression) context
        wsc->compression.deflate_stream = mallocz(sizeof(z_stream));
        wsc->compression.deflate_stream->zalloc = Z_NULL;
        wsc->compression.deflate_stream->zfree = Z_NULL;
        wsc->compression.deflate_stream->opaque = Z_NULL;
        
        // Initialize with negative window bits for raw deflate (no zlib/gzip header)
        int ret = deflateInit2(
            wsc->compression.deflate_stream,
            wsc->compression.compression_level,
            Z_DEFLATED,
            -wsc->compression.window_bits,
            WS_COMPRESS_MEMLEVEL,
            Z_DEFAULT_STRATEGY
        );
        
        if (ret != Z_OK) {
            websocket_error(wsc, "Failed to initialize deflate context: %s (%d)", 
                       zError(ret), ret);
            freez(wsc->compression.deflate_stream);
            wsc->compression.deflate_stream = NULL;
            wsc->compression.enabled = false;
            return false;
        }
        
        // Initialize inflate (decompression) context using our helper function
        if (!websocket_decompression_init(wsc)) {
            websocket_error(wsc, "Failed to initialize inflate context");
            deflateEnd(wsc->compression.deflate_stream);
            freez(wsc->compression.deflate_stream);
            wsc->compression.deflate_stream = NULL;
            wsc->compression.enabled = false;
            return false;
        }
        
        websocket_debug(wsc, "Compression enabled (permessage-deflate, window bits: %d)",
                   wsc->compression.window_bits);
    }
    
    return true;
}

// Clean up compression resources for a WebSocket client
void websocket_compression_cleanup(struct websocket_server_client *wsc) {
    if (!wsc)
        return;

    // Aggressively clean up all compression resources regardless of state
    websocket_debug(wsc, "Cleaning up compression resources");

    // Clean up deflate context
    if (wsc->compression.deflate_stream) {
        websocket_debug(wsc, "Cleaning up deflate stream");

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

            // Even on error, we'll continue with cleanup to avoid leaks
        }

        // Free the stream structure
        freez(wsc->compression.deflate_stream);
        wsc->compression.deflate_stream = NULL;
    }

    // Clean up inflate context using our helper function
    websocket_decompression_cleanup(wsc);

    // Reset all compression state
    wsc->compression.type = WS_COMPRESS_NONE;
    wsc->compression.enabled = false;
    wsc->compression.client_context_takeover = true;
    wsc->compression.server_context_takeover = true;
    wsc->compression.window_bits = WS_COMPRESS_WINDOW_BITS;
    wsc->compression.compression_level = WS_COMPRESS_DEFAULT_LEVEL;

    websocket_debug(wsc, "Compression resources cleaned up");
}

// Generate the extension string for the WebSocket handshake response
char *websocket_compression_negotiate(struct websocket_server_client *wsc, const char *requested_extensions) {
    if (!wsc || !requested_extensions || !*requested_extensions)
        return NULL;
    
    // Initialize compression with the requested extensions
    if (!websocket_compression_init(wsc, requested_extensions))
        return NULL;
    
    // If compression wasn't enabled, return NULL
    if (!wsc->compression.enabled)
        return NULL;
    
    // Build the response extension string
    BUFFER *wb = buffer_create(256, NULL);
    
    switch (wsc->compression.type) {
        case WS_COMPRESS_DEFLATE:
            buffer_strcat(wb, "permessage-deflate");
            
            // Add parameters if different from defaults
            if (!wsc->compression.client_context_takeover)
                buffer_strcat(wb, "; client_no_context_takeover");
            
            if (!wsc->compression.server_context_takeover)
                buffer_strcat(wb, "; server_no_context_takeover");
            
            if (wsc->compression.window_bits != WS_COMPRESS_WINDOW_BITS)
                buffer_sprintf(wb, "; server_max_window_bits=%d", wsc->compression.window_bits);
            break;
            
        default:
            // Unsupported compression type
            buffer_free(wb);
            return NULL;
    }
    
    char *result = strdupz(buffer_tostring(wb));
    buffer_free(wb);
    return result;
}

// Clean up decompression resources for a client's inflate stream
void websocket_decompression_cleanup(WS_CLIENT *wsc) {
    if (!wsc || !wsc->compression.inflate_stream)
        return;

    websocket_debug(wsc, "Cleaning up inflate stream");

    // End the current inflate stream and free its resources
    inflateEnd(wsc->compression.inflate_stream);
    freez(wsc->compression.inflate_stream);
    wsc->compression.inflate_stream = NULL;
}

// Initialize decompression resources for a client
bool websocket_decompression_init(WS_CLIENT *wsc) {
    if (!wsc)
        return false;

    // Create a new inflate stream
    wsc->compression.inflate_stream = mallocz(sizeof(z_stream));
    wsc->compression.inflate_stream->zalloc = Z_NULL;
    wsc->compression.inflate_stream->zfree = Z_NULL;
    wsc->compression.inflate_stream->opaque = Z_NULL;

    // Initialize with negative window bits for raw deflate (no zlib/gzip header)
    int init_ret = inflateInit2(
        wsc->compression.inflate_stream,
        -wsc->compression.window_bits
    );

    if (init_ret != Z_OK) {
        websocket_error(wsc, "Failed to initialize inflate stream: %s (%d)",
                       zError(init_ret), init_ret);
        freez(wsc->compression.inflate_stream);
        wsc->compression.inflate_stream = NULL;
        return false;
    }

    websocket_debug(wsc, "Inflate stream initialized successfully");
    return true;
}

// Reset decompression resources for a client - calls cleanup and init
bool websocket_decompression_reset(WS_CLIENT *wsc) {
    websocket_decompression_cleanup(wsc);
    return websocket_decompression_init(wsc);
}

// Decompress a client's message from payload to u_payload
bool websocket_client_decompress_message(WS_CLIENT *wsc) {
    if (!wsc || !wsc->is_compressed || !wsc->compression.enabled)
        return false;

    if (wsb_is_empty(&wsc->payload)) {
        websocket_debug(wsc, "Empty compressed message");
        wsb_reset(&wsc->u_payload);
        wsb_null_terminate(&wsc->u_payload);
        return true;
    }

    websocket_debug(wsc, "Decompressing message (%zu bytes)", wsb_length(&wsc->payload));

    z_stream *z_stream = wsc->compression.inflate_stream;
    wsb_reset(&wsc->u_payload);

    // Per RFC 7692, we need to append 4 bytes (00 00 FF FF) to the compressed data
    // to ensure the inflate operation completes
    static const unsigned char trailer[4] = {0x00, 0x00, 0xFF, 0xFF};
    wsb_append_padding(&wsc->payload, trailer, 4);

    // Handle the inflate stream based on context takeover settings
    if (!wsc->compression.client_context_takeover) {
        websocket_debug(wsc, "No context takeover - resetting inflate stream");

        int reset_ret = inflateReset(z_stream);
        if (reset_ret != Z_OK) {
            websocket_error(wsc, "Failed to reset inflate stream for no context takeover: %s (%d)",
                            zError(reset_ret), reset_ret);

            // On reset failure, recreate the inflate stream
            if (!websocket_decompression_reset(wsc))
                return false;
        }
        else {
            // Reset Adler32 checksum to its initial value (1) for a clean dictionary state
            z_stream->adler = 1;
            websocket_debug(wsc, "Inflate stream reset complete with adler set to 1");
        }
    }
    else {
        // With client_context_takeover enabled, we preserve the inflate state
        // This improves compression efficiency for similar messages
        websocket_debug(wsc, "Context takeover, preserving inflate state (total_in=%lu, total_out=%lu)",
                        z_stream->total_in, z_stream->total_out);
    }

    z_stream->next_in = (Bytef *)wsb_data(&wsc->payload);
    z_stream->avail_in = wsb_length(&wsc->payload) + 4;
    z_stream->next_out = (Bytef *)wsb_data(&wsc->u_payload);
    z_stream->avail_out = wsb_size(&wsc->u_payload);

    // Decompress with loop for multiple buffer expansions if needed
    int ret = Z_MEM_ERROR;
    bool success = false;
    int retries = 24;
    size_t wanted_size = MAX(wsb_size(&wsc->u_payload), wsb_length(&wsc->payload) * 2);
    do {
        wsb_resize(&wsc->u_payload, wanted_size);

        // Position next_out to point to the end of the currently decompressed data
        z_stream->next_out = (Bytef *)wsb_data(&wsc->u_payload) + wsb_length(&wsc->u_payload);

        // Only make the newly available space available to zlib
        z_stream->avail_out = wsb_size(&wsc->u_payload) - wsb_length(&wsc->u_payload);

        // Try to decompress
        ret = inflate(z_stream, Z_SYNC_FLUSH);

        websocket_debug(wsc, "inflate() returned %d (%s), "
                             "avail_in=%u, avail_out=%u, total_in=%lu, total_out=%lu",
                        ret, zError(ret),
                        z_stream->avail_in,
                        z_stream->avail_out,
                        z_stream->total_in,
                        z_stream->total_out);

        success = ret == Z_STREAM_END || (ret == Z_OK && z_stream->avail_in == 0 && z_stream->avail_out > 0);

        // Update the buffer's length to include the newly written data
        wsb_set_length(&wsc->u_payload, wsb_size(&wsc->u_payload) - z_stream->avail_out);

        // Check if we need more output space
        if (!success && (ret == Z_BUF_ERROR || ret == Z_OK)) {
            wanted_size = MIN(wanted_size * 2, WEBSOCKET_MAX_UNCOMPRESSED_SIZE);
            if (wanted_size == WEBSOCKET_MAX_UNCOMPRESSED_SIZE && wanted_size == wsb_size(&wsc->u_payload))
                break; // we cannot resize more
        }
    } while (!success && retries-- > 0);

    if(!success) {
        // Decompression failed
        websocket_error(wsc, "Decompression failed: %s (ret = %d, avail_in = %u)", zError(ret), ret, z_stream->avail_in);
        wsb_reset(&wsc->u_payload);
        websocket_decompression_reset(wsc);
        return false;
    }

    // Log successful decompression with detailed information
    websocket_debug(wsc, "Successfully decompressed %zu bytes to %zu bytes (ratio: %.2fx)",
                    wsb_length(&wsc->payload), wsb_length(&wsc->u_payload),
                    (double)wsb_length(&wsc->u_payload) / (double)wsb_length(&wsc->payload));

    // Show a preview of the decompressed data
    websocket_dump_debug(wsc, wsb_data(&wsc->u_payload), wsb_length(&wsc->u_payload), "Decompressed data preview");

    return true;
}
