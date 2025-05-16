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

/**
 * @brief Create and send a WebSocket frame
 * 
 * Creates a WebSocket frame with the given payload and sends it.
 * This function directly creates a frame with the specified FIN and RSV1 bits.
 * 
 * @param wsc WebSocket client
 * @param payload Payload data to send
 * @param payload_len Length of the payload
 * @param opcode WebSocket opcode (text, binary, continuation, etc.)
 * @param compressed Whether the payload is already compressed (RSV1 bit)
 * @param final Whether this is the final frame in a message (FIN bit)
 * @return Number of bytes sent or -1 on error
 */
static int websocket_protocol_send_frame(
    WS_CLIENT *wsc, const char *payload, size_t payload_len,
    WEBSOCKET_OPCODE opcode, bool compressed, bool final) {

    if(!wsc)
        return -1;

    const char *disconnect_msg = "";

    if (wsc->sock.fd < 0) {
        disconnect_msg = "Client not connected";
        goto abnormal_disconnect;
    }
    
    // Validate parameters based on WebSocket protocol
    if (websocket_frame_is_control_opcode(opcode) && payload_len > 125) {
        disconnect_msg = "Control frame payload too large";
        goto abnormal_disconnect;
    }
    
    // Control frames must not be fragmented
    if (websocket_frame_is_control_opcode(opcode) && !final) {
        disconnect_msg = "Control frames cannot be fragmented";
        goto abnormal_disconnect;
    }
    
    // Control frames cannot be compressed
    if (websocket_frame_is_control_opcode(opcode) && compressed) {
        disconnect_msg = "Control frames cannot be compressed";
        goto abnormal_disconnect;
    }
    
    // For compressed payloads, we already did the compression outside this function
    // so we don't need to compress again, just use the provided payload directly
    size_t final_payload_len = payload_len;

    // Determine header size based on payload length
    size_t header_size = select_header_size(final_payload_len);

    // Calculate frame size
    size_t frame_size = header_size + final_payload_len;

    // Reserve space in the circular buffer for the entire frame
    unsigned char *header_dst = (unsigned char *)cbuffer_reserve_unsafe(&wsc->out_buffer, frame_size);
    if (!header_dst) {
        disconnect_msg = "Buffer full - too much outgoing data";
        goto abnormal_disconnect;
    }

    // The payload will be written directly after the header in our reserved buffer
    char *payload_dst = (char *)(header_dst + header_size);

    // Copy payload data to our buffer
    if (payload && payload_len > 0) {
        memcpy(payload_dst, payload, payload_len);
    }

    // Write the header
    // First byte: FIN bit, RSV1 bit (compression), and opcode
    // Only set FIN bit if this is the final frame
    // Only set RSV1 if this frame uses compression
    header_dst[0] = (final ? 0x80 : 0) | (compressed ? 0x40 : 0) | (opcode & 0x0F);

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
                    "TX FRAME: OPCODE=0x%x (%s), FIN=%s, RSV1=%d, RSV2=%d, RSV3=%d, MASK=%s, LEN=%d, "
                    "PAYLOAD_LEN=%zu, HEADER_SIZE=%zu, FRAME_SIZE=%zu, MASK=%02x%02x%02x%02x",
                    header.opcode,
                    WEBSOCKET_OPCODE_2str(opcode),
                    header.fin ? "True" : "False", header.rsv1, header.rsv2, header.rsv3,
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

/**
 * @brief Send a potentially large payload with automatic fragmentation
 * 
 * This function handles large message fragmentation to ensure browser compatibility.
 * It compresses the message if requested and then splits it into fragments
 * that are smaller than WS_MAX_OUTGOING_FRAME_SIZE.
 * 
 * @param wsc WebSocket client
 * @param payload Payload data to send
 * @param payload_len Length of the payload
 * @param opcode WebSocket opcode (text, binary)
 * @param use_compression Whether to attempt compression
 * @return Total number of bytes sent or -1 on error
 */
int websocket_protocol_send_payload(
    WS_CLIENT *wsc, const char *payload, size_t payload_len,
    WEBSOCKET_OPCODE opcode, bool use_compression) {
    
    if (!wsc || wsc->sock.fd < 0)
        return -1;
    
    // Control frames should never use this function as they can't be fragmented
    if (websocket_frame_is_control_opcode(opcode) || !payload || !payload_len) {
        return websocket_protocol_send_frame(wsc, payload, payload_len, opcode, false, true);
    }
    
    // Attempt compression if requested and conditions are met
    bool compressed = false;
    const char *data_to_send = payload;
    size_t data_len = payload_len;
    
    if (use_compression) {
        compressed = websocket_client_compress_message(wsc, payload, payload_len);
        if (compressed) {
            data_to_send = wsb_data(&wsc->c_payload);
            data_len = wsb_length(&wsc->c_payload);
            
            websocket_debug(wsc, "Using compressed payload for transmission (%zu -> %zu bytes)",
                           payload_len, data_len);
        }
    }
    
    // Check if we need to fragment the message
    if (data_len <= wsc->max_outbound_frame_size) {
        // Small enough to send in a single frame
        return websocket_protocol_send_frame(wsc, data_to_send, data_len, opcode, compressed, true);
    }
    
    // We need to fragment the message
    websocket_debug(wsc, "Fragmenting large message (%zu bytes) into frames of max %zu bytes",
                   data_len, wsc->max_outbound_frame_size);
    
    size_t total_sent = 0;
    size_t bytes_remaining = data_len;
    size_t offset = 0;
    bool first_frame = true;
    
    // Send fragments
    while (bytes_remaining > 0) {
        // Determine size for this fragment
        size_t fragment_size = MIN(bytes_remaining, wsc->max_outbound_frame_size);
        bool is_final = (fragment_size == bytes_remaining);
        
        // First frame uses original opcode, subsequent frames use continuation
        WEBSOCKET_OPCODE frame_opcode = first_frame ? opcode : WS_OPCODE_CONTINUATION;
        
        // Compression flag is only set on the first frame
        bool frame_compressed = compressed && first_frame;
        
        // Send this fragment
        int result = websocket_protocol_send_frame(
            wsc, 
            data_to_send + offset, 
            fragment_size, 
            frame_opcode, 
            frame_compressed,
            is_final
        );
        
        if (result < 0) {
            // Failed to send frame
            websocket_error(wsc, "Failed to send message fragment at offset %zu", offset);
            return -1;
        }
        
        // Update counters for next iteration
        total_sent += result;
        offset += fragment_size;
        bytes_remaining -= fragment_size;
        first_frame = false;
    }
    
    websocket_debug(wsc, "Successfully sent fragmented message in multiple frames, total bytes: %zu", total_sent);
    return total_sent;
}

/**
 * @brief Send a text message with automatic fragmentation
 * 
 * This function sends a WebSocket text message with automatic compression and fragmentation.
 * 
 * @param wsc WebSocket client
 * @param text Text to send
 * @return Number of bytes sent or -1 on error
 */
int websocket_protocol_send_text(WS_CLIENT *wsc, const char *text) {
    if (!wsc)
        return -1;

    size_t text_len = strlen(text);

    websocket_debug(wsc, "Sending text message, length=%zu", text_len);

    // Dump text message for debugging
    websocket_dump_debug(wsc, text, text_len, "TX TEXT MSG");

    // Enable compression for text messages by default, with automatic fragmentation
    return websocket_protocol_send_payload(wsc, text, text_len, WS_OPCODE_TEXT, true);
}

/**
 * @brief Send a binary message with automatic fragmentation
 * 
 * This function sends a WebSocket binary message with automatic compression and fragmentation.
 * 
 * @param wsc WebSocket client
 * @param data Binary data to send
 * @param length Length of the binary data
 * @return Number of bytes sent or -1 on error
 */
int websocket_protocol_send_binary(WS_CLIENT *wsc, const void *data, size_t length) {
    if (!wsc)
        return -1;

    websocket_debug(wsc, "Sending binary message, length=%zu", length);

    // Dump binary message for debugging
    websocket_dump_debug(wsc, data, length, "TX BIN MSG");

    // Enable compression for binary messages by default, with automatic fragmentation
    return websocket_protocol_send_payload(wsc, data, length, WS_OPCODE_BINARY, true);
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

    // Send close frame (never compressed, always final)
    int result = websocket_protocol_send_frame(wsc, payload, payload_len, WS_OPCODE_CLOSE, false, true);

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
    
    // Send ping frame (never compressed, always final)
    return websocket_protocol_send_frame(wsc, data, length, WS_OPCODE_PING, false, true);
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
    
    // Send pong frame (never compressed, always final)
    return websocket_protocol_send_frame(wsc, data, length, WS_OPCODE_PONG, false, true);
}
