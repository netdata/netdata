// SPDX-License-Identifier: GPL-3.0-or-later

#include "websocket-internal.h"

// --------------------------------------------------------------------------------------------------------------------
// writing to the socket

// Actually write data to the client socket
ssize_t websocket_write_data(WS_CLIENT *wsc) {
    internal_fatal(wsc->wth->tid != gettid_cached(), "Function %s() should only be used by the websocket thread", __FUNCTION__ );

    worker_is_busy(WORKERS_WEBSOCKET_SOCK_SEND);

    if (!wsc->out_buffer.data || wsc->sock.fd < 0)
        return -1;

    ssize_t bytes_written = 0;

    // Let cbuffer_next_unsafe determine if there's data to write
    // This correctly handles the circular buffer wrap-around cases

    // Get data to write from circular buffer
    char *data;
    size_t data_length = cbuffer_next_unsafe(&wsc->out_buffer, &data);
    if (data_length == 0)
        goto done;

    // Dump the data being written for debugging
    websocket_dump_debug(wsc, data, data_length, "TX SOCK %zu bytes", data_length);

    // In the websocket thread we want non-blocking behavior
    // Use nd_sock_write with a single retry
    bytes_written = nd_sock_write(&wsc->sock, data, data_length, 1); // 1 retry for non-blocking write

    if (bytes_written < 0) {
        websocket_error(wsc, "Failed to write to client: %s", strerror(errno));
        goto done;
    }

    // Remove written bytes from circular buffer
    if (bytes_written > 0)
        cbuffer_remove_unsafe(&wsc->out_buffer, bytes_written);

done:
    websocket_thread_update_client_poll_flags(wsc);
    return bytes_written;
}

// --------------------------------------------------------------------------------------------------------------------

static inline size_t select_header_size(size_t payload_len) {
    if (payload_len < 126)
        return 2;
    else if (payload_len <= 65535)
        return 4;
    else
        return 10;
}

// Create and send a WebSocket frame
int websocket_protocol_send_frame(
    WS_CLIENT *wsc, const char *payload, size_t payload_len,
    WEBSOCKET_OPCODE opcode, bool use_compression) {

    if(!wsc)
        return -1;

    const char *disconnect_msg = "";
    z_stream *zstrm = wsc->compression.deflate_stream;

    if (wsc->sock.fd < 0) {
        disconnect_msg = "Client not connected";
        goto abnormal_disconnect;
    }
    
    // Validate parameters based on WebSocket protocol
    if (websocket_frame_is_control_opcode(opcode) && payload_len > 125) {
        disconnect_msg = "Control frame payload too large";
        goto abnormal_disconnect;
    }
    
    // Check if we should actually use compression
    bool compress = !websocket_frame_is_control_opcode(opcode) &&
                    use_compression &&
                    payload && payload_len &&
                    wsc->compression.enabled &&
                    zstrm &&
                    payload_len >= WS_COMPRESS_MIN_SIZE;

    // Calculate maximum possible compressed size using deflateBound
    size_t max_compressed_size = payload_len;
    if (compress) {
        // Use deflateBound to accurately calculate maximum possible size
        max_compressed_size = deflateBound(zstrm, payload_len);

        // Add 4 bytes for Z_SYNC_FLUSH trailer (will be removed later)
        max_compressed_size += 4;

        // Ensure the destination can fit the uncompressed data too
        max_compressed_size = MAX(payload_len, max_compressed_size);
    }

    // Determine header size based on maximum potential size
    size_t header_size = select_header_size(max_compressed_size);

    // Calculate maximum potential frame size
    size_t max_frame_size = header_size + max_compressed_size;

    // Reserve space in the circular buffer for the entire frame
    unsigned char *header_dst = (unsigned char *)cbuffer_reserve_unsafe(&wsc->out_buffer, max_frame_size);
    if (!header_dst) {
        disconnect_msg = "Buffer full - too much outgoing data";
        goto abnormal_disconnect;
    }

    // The payload will be written directly after the header in our reserved buffer
    char *payload_dst = (char *)(header_dst + header_size);
    size_t final_payload_len = 0;

    if (compress) {
        // Setup temporary parameters for the compressor to use
        // We'll use the space after our header for the compressed data
        zstrm->next_in = (Bytef *)payload;
        zstrm->avail_in = payload_len;
        zstrm->next_out = (Bytef *)payload_dst;
        zstrm->avail_out = max_compressed_size;
        zstrm->total_in = 0;
        zstrm->total_out = 0;

        // Compress with sync flush to ensure data is flushed
        int ret = deflate(zstrm, Z_SYNC_FLUSH);

        bool success = false;
        if (ret == Z_STREAM_END || (ret == Z_OK && zstrm->avail_in == 0 && zstrm->avail_out > 0))
            success = true;
        else if (ret == Z_OK && zstrm->avail_in == 0 && zstrm->avail_out == 0) {
            unsigned pending = Z_NULL;
            int bits = Z_NULL;
            if(deflatePending(zstrm, &pending, &bits) == Z_OK &&
                (pending == Z_NULL && bits == Z_NULL))
                success = true;
        }

        uInt avail_in = zstrm->avail_in;
        uInt avail_out = zstrm->avail_out;
        uLong total_in = zstrm->total_in;
        uLong total_out = zstrm->total_out;

        // we are done - reset the stream to avoid heap-use-after-free issues later
        if (deflateReset(zstrm) != Z_OK) {
            disconnect_msg = "Deflate reset failed";
            goto abnormal_disconnect;
        }

        // Calculate compressed size if successful
        if (!success || total_out <= 4) {
            // Compression failed
            websocket_error(wsc, "Compression failed: %s "
                                 "(ret = %d, avail_in = %u, avail_out = %u, total_in = %lu, total_out = %lu) - "
                                 "sending uncompressed payload",
                            zError(ret), ret, avail_in, avail_out, total_in, total_out);
            compress = false;
        }
        else {
            // As per RFC 7692, remove trailing 4 bytes (00 00 FF FF) from Z_SYNC_FLUSH
            final_payload_len = max_compressed_size - avail_out - 4;

            websocket_debug(wsc, "Compressed payload from %zu to %zu bytes (%.1f%%)",
                            payload_len, final_payload_len,
                            (double)final_payload_len * 100.0 / (double)payload_len);

            // we may have selected a bigger header size than needed
            // so we need to move the payload to the right place
            size_t optimal_header_size = select_header_size(final_payload_len);
            if(optimal_header_size < header_size) {
                char *dst = (char *)header_dst + optimal_header_size;
                char *src = payload_dst;
                memmove(dst, src, final_payload_len);
                payload_dst = dst;
                header_size = optimal_header_size;
            }
        }

        // ensure all pointer values are NULL, so that there is no trace back to this compression
        zstrm->next_in = NULL;
        zstrm->avail_in = 0;
        zstrm->next_out = NULL;
        zstrm->avail_out = 0;
        zstrm->total_in = 0;
        zstrm->total_out = 0;
    }

    if(!compress && payload && payload_len > 0) {
        memcpy(payload_dst, payload, payload_len);
        final_payload_len = payload_len;
    }

    // Write the header
    // First byte: FIN(1) + RSV1(compress) + RSV2(0) + RSV3(0) + OPCODE(4)
    header_dst[0] = 0x80 | (compress ? 0x40 : 0) | (opcode & 0x0F);

    // Write payload length with the appropriate format
    switch(header_size) {
        case 2:
            header_dst[1] = final_payload_len & 0x7F;
            break;

        case 4:
            header_dst[1] = 126;
            header_dst[2] = (final_payload_len >> 8) & 0xFF;
            header_dst[3] = final_payload_len & 0xFF;
            break;

        case 10:
            header_dst[1] = 127;
            header_dst[2] = (final_payload_len >> 56) & 0xFF;
            header_dst[3] = (final_payload_len >> 48) & 0xFF;
            header_dst[4] = (final_payload_len >> 40) & 0xFF;
            header_dst[5] = (final_payload_len >> 32) & 0xFF;
            header_dst[6] = (final_payload_len >> 24) & 0xFF;
            header_dst[7] = (final_payload_len >> 16) & 0xFF;
            header_dst[8] = (final_payload_len >> 8) & 0xFF;
            header_dst[9] = final_payload_len & 0xFF;
            break;

        default:
            // impossible case - added to avoid compiler warning
            disconnect_msg = "Invalid header size";
            goto abnormal_disconnect;
    }

    // Commit the final frame size (header + payload)
    size_t final_frame_size = header_size + final_payload_len;
    cbuffer_commit_reserved_unsafe(&wsc->out_buffer, final_frame_size);

#ifdef NETDATA_INTERNAL_CHECKS
    // Log frame being sent with detailed format matching the received frame logging
    WEBSOCKET_FRAME_HEADER header;
    if(!websocket_protocol_parse_header_from_buffer((const char *)header_dst, header_size, &header)) {
        disconnect_msg = "Failed to parse the header we generated";
        goto abnormal_disconnect;
    }

    websocket_debug(wsc,
                    "TX FRAME: OPCODE=0x%x, FIN=%s, RSV1=%d, RSV2=%d, RSV3=%d, MASK=%s, LEN=%d, "
                    "PAYLOAD_LEN=%zu, HEADER_SIZE=%zu, FRAME_SIZE=%zu, MASK=%02x%02x%02x%02x",
                    header.opcode, header.fin ? "True" : "False", header.rsv1, header.rsv2, header.rsv3,
                    header.mask ? "True" : "False", header.len,
                    header.payload_length, header.header_size, header.frame_size,
                    header.mask_key[0], header.mask_key[1], header.mask_key[2], header.mask_key[3]);
#endif

    // Make sure the client's poll flags include WRITE
    websocket_thread_update_client_poll_flags(wsc);

    return (int)final_frame_size;

abnormal_disconnect:
    websocket_error(wsc, "triggering abnormal disconnect: %s", disconnect_msg);
    websocket_thread_send_command(wsc->wth, WEBSOCKET_THREAD_CMD_REMOVE_CLIENT, wsc->id);
    return -1;

//graceful_disconnect:
//    // the current implementation does not support graceful disconnect - so we do an abnormal one
//    websocket_error(wsc, "triggering graceful disconnect: %s", disconnect_msg);
//    websocket_thread_send_command(wsc->wth, WEBSOCKET_THREAD_CMD_REMOVE_CLIENT, wsc->id);
//    return -1;
}

// Send a text message
int websocket_protocol_send_text(WS_CLIENT *wsc, const char *text) {
    if (!wsc)
        return -1;

    // Special handling for null or empty text message
    if (!text || text[0] == '\0') {
        websocket_debug(wsc, "Sending empty text message");

        // Use an empty buffer for zero-length text messages
        static const char empty_data[1] = {0};
        return websocket_protocol_send_frame(wsc, empty_data, 0, WS_OPCODE_TEXT, false);
    }

    size_t text_len = strlen(text);

    websocket_debug(wsc, "Sending text message, length=%zu", text_len);

    // Dump text message for debugging
    websocket_dump_debug(wsc, text, text_len, "TX TEXT MSG");

    // Enable compression for text messages by default
    return websocket_protocol_send_frame(wsc, text, text_len, WS_OPCODE_TEXT, true);
}

// Send a binary message
int websocket_protocol_send_binary(WS_CLIENT *wsc, const void *data, size_t length) {
    if (!wsc)
        return -1;

    // Special handling for empty binary message
    if (!data || length == 0) {
        websocket_debug(wsc, "Sending empty binary message");

        // Use an empty buffer for zero-length binary messages
        static const char empty_data[1] = {0};
        return websocket_protocol_send_frame(wsc, empty_data, 0, WS_OPCODE_BINARY, false);
    }

    websocket_debug(wsc, "Sending binary message, length=%zu", length);

    // Dump binary message for debugging
    websocket_dump_debug(wsc, data, length, "TX BIN MSG");

    return websocket_protocol_send_frame(wsc, data, length, WS_OPCODE_BINARY, true);
}

// Send a close frame
int websocket_protocol_send_close(WS_CLIENT *wsc, WEBSOCKET_CLOSE_CODE code, const char *reason) {
    if (!wsc || wsc->sock.fd < 0)
        return -1;

    // Only send a close frame if we're in a valid state to do so
    // Per RFC 6455: An endpoint MUST NOT send any more data frames after sending a Close frame
    // CLOSING_CLIENT means we already responded to a client's close and shouldn't send another
    if (wsc->state == WS_STATE_CLOSED ||
        wsc->state == WS_STATE_CLOSING_SERVER ||
        wsc->state == WS_STATE_CLOSING_CLIENT)
        return -1;

    // Validate close code
    if (!websocket_validate_close_code((uint16_t)code)) {
        websocket_error(wsc, "Invalid close code: %d (%s)", code, WEBSOCKET_CLOSE_CODE_2str(code));
        code = WS_CLOSE_PROTOCOL_ERROR;
        reason = "Invalid close code";
    }

    // Prepare close payload: 2-byte code + optional reason text
    size_t reason_len = reason ? strlen(reason) : 0;

    // Control frames max size is 125 bytes
    if (reason_len > 123) {
        websocket_error(wsc, "Close frame payload too large: %zu bytes (max 123)", reason_len);
        reason_len = 123; // Truncate reason to fit
    }

    // Use stack buffer for close frame payload (max 125 bytes per RFC 6455)
    size_t payload_len = 2 + reason_len;
    char payload[payload_len];

    // Set status code in network byte order (big-endian)
    uint16_t code_value = (uint16_t)code;
    payload[0] = (code_value >> 8) & 0xFF;
    payload[1] = code & 0xFF;

    // Add reason if provided (truncate if necessary)
    if (reason && reason_len > 0)
        memcpy(payload + 2, reason, reason_len);

    // Call the close handler if registered - this is used to inject a message on close if needed
    if(wsc->on_close)
        wsc->on_close(wsc, WS_CLOSE_GOING_AWAY, reason);

    // Send close frame (never compressed)
    int result = websocket_protocol_send_frame(wsc, payload, payload_len, WS_OPCODE_CLOSE, false);

    return result;
}

// Send a ping frame
int websocket_protocol_send_ping(WS_CLIENT *wsc, const char *data, size_t length) {
    if (!wsc)
        return -1;
    
    // Control frames max size is 125 bytes
    if (length > 125) {
        websocket_error(wsc, "Ping frame payload too large: %zu bytes (max: 125)",
                   length);
        return -1;
    }
    
    // If no data provided, use empty ping
    if (!data || length == 0) {
        data = "";
        length = 0;
    }
    
    // Send ping frame (never compressed)
    return websocket_protocol_send_frame(wsc, data, length, WS_OPCODE_PING, false);
}

// Send a pong frame
int websocket_protocol_send_pong(WS_CLIENT *wsc, const char *data, size_t length) {
    if (!wsc)
        return -1;
    
    // Control frames max size is 125 bytes
    if (length > 125) {
        websocket_error(wsc, "Pong frame payload too large: %zu bytes (max: 125)",
                   length);
        return -1;
    }
    
    // If no data provided, use empty pong
    if (!data || length == 0) {
        data = "";
        length = 0;
    }
    
    // Send pong frame (never compressed)
    return websocket_protocol_send_frame(wsc, data, length, WS_OPCODE_PONG, false);
}
