// SPDX-License-Identifier: GPL-3.0-or-later

#include "websocket-internal.h"

// --------------------------------------------------------------------------------------------------------------------
// reading from the socket

static inline bool cbuffer_has_enough_data_for_next_frame(WS_CLIENT *wsc) {
    return wsc->next_frame_size > 0 &&
           cbuffer_used_size_unsafe(&wsc->in_buffer) >= wsc->next_frame_size;
}

static inline bool cbuffer_next_frame_is_fragmented(WS_CLIENT *wsc) {
    return cbuffer_has_enough_data_for_next_frame(wsc) &&
           cbuffer_next_unsafe(&wsc->in_buffer,  NULL) < wsc->next_frame_size;
}

static ssize_t websocket_received_data_process(WS_CLIENT *wsc, ssize_t bytes_read) {
    if(cbuffer_next_frame_is_fragmented(wsc))
        cbuffer_ensure_unwrapped_size(&wsc->in_buffer, wsc->next_frame_size);

    char *buffer_pos;
    size_t contiguous_input = cbuffer_next_unsafe(&wsc->in_buffer, &buffer_pos);

    // Now we have contiguous data for processing
    ssize_t bytes_consumed = websocket_protocol_got_data(wsc, buffer_pos, contiguous_input);
    if (bytes_consumed < 0) {
        if (bytes_consumed < -1) {
            bytes_consumed = -bytes_consumed;
            cbuffer_remove_unsafe(&wsc->in_buffer, bytes_consumed);
        }

        websocket_error(wsc, "Failed to process received data");
        return -1;
    }

    // Check if bytes_processed is 0 but this was a successful call
    // This means we have an incomplete frame and need to keep the entire buffer
    if (bytes_consumed == 0) {
        websocket_debug(
            wsc, "Incomplete frame detected - keeping all %zu bytes in buffer for next read", contiguous_input);
        return bytes_read; // Return the bytes read so caller knows we made progress
    }

    // We've processed some data - remove it from the circular buffer
    cbuffer_remove_unsafe(&wsc->in_buffer, bytes_consumed);

    return bytes_consumed;
}

// Process incoming WebSocket data
ssize_t websocket_receive_data(WS_CLIENT *wsc) {
    internal_fatal(wsc->wth->tid != gettid_cached(), "Function %s() should only be used by the websocket thread", __FUNCTION__ );

    worker_is_busy(WORKERS_WEBSOCKET_SOCK_RECEIVE);

    if (!wsc->in_buffer.data || wsc->sock.fd < 0)
        return -1;

    size_t available_space = WEBSOCKET_RECEIVE_BUFFER_SIZE;
    if(wsc->next_frame_size > 0) {
        size_t used_space = cbuffer_used_size_unsafe(&wsc->in_buffer);
        if(used_space < wsc->next_frame_size) {
            size_t missing_for_next_frame = wsc->next_frame_size - used_space;
            available_space = MAX(missing_for_next_frame, WEBSOCKET_RECEIVE_BUFFER_SIZE);
        }
    }

    char *buffer = cbuffer_reserve_unsafe(&wsc->in_buffer, available_space);
    if(!buffer) {
        websocket_error(wsc, "Not enough space to read %zu bytes", available_space);
        return -1;
    }

    // Read data from socket into temporary buffer using ND_SOCK
    ssize_t bytes_read = nd_sock_read(&wsc->sock, buffer, available_space, 0);

    if (bytes_read <= 0) {
        if (bytes_read == 0) {
            // Connection closed
            websocket_debug(wsc, "Client closed connection");
            return -1;
        }

        if (errno == EAGAIN || errno == EWOULDBLOCK)
            return 0; // No data available right now

        websocket_error(wsc, "Failed to read from client: %s", strerror(errno));
        return -1;
    }

    if (bytes_read > (ssize_t)available_space) {
        websocket_error(wsc, "Received more data (%zd) than available space in buffer (%zd)",
                        bytes_read, available_space);
        return -1;
    }

    cbuffer_commit_reserved_unsafe(&wsc->in_buffer, bytes_read);

    // Update last activity time
    wsc->last_activity_t = now_monotonic_sec();

    // Dump the received data for debugging
    websocket_dump_debug(wsc, buffer, bytes_read, "RX SOCK %zd bytes", bytes_read);

    if(wsc->next_frame_size  == 0 || cbuffer_has_enough_data_for_next_frame(wsc)) {
        // we don't know the next frame size
        // or, we know it and we have all the data for it

        // process the received data
        if(websocket_received_data_process(wsc, bytes_read) < 0)
            return -1;

        // we may still have wrapped data in the circular buffer that can satisfy the entire next frame
        if(cbuffer_next_frame_is_fragmented(wsc)) {
            // we have enough data to process this frame, no need to wait for more input
            if(websocket_received_data_process(wsc, bytes_read) < 0)
                return -1;
        }
    }

    // Return the number of bytes we processed from this read
    // Even if bytes_processed is 0, we still read data which will be processed later
    return bytes_read;
}

// --------------------------------------------------------------------------------------------------------------------

// Validate a WebSocket close code according to RFC 6455
bool websocket_validate_close_code(uint16_t code) {
    // 1000-2999 are reserved for the WebSocket protocol
    // 3000-3999 are reserved for use by libraries, frameworks, and applications
    // 4000-4999 are reserved for private use

    // Check if code is in valid ranges
    if ((code >= 1000 && code <= 1011) || // Protocol-defined codes
        (code >= 3000 && code <= 4999))   // Application/library/private codes
    {
        // Codes 1004, 1005, and 1006 must not be used in a Close frame by an endpoint
        if (code != WS_CLOSE_RESERVED && code != WS_CLOSE_NO_STATUS && code != WS_CLOSE_ABNORMAL)
            return true;
    }

    return false;
}

// Centralized function to handle WebSocket protocol exceptions
void websocket_protocol_exception(WS_CLIENT *wsc, WEBSOCKET_CLOSE_CODE reason_code, const char *reason_txt) {
    if (!wsc) return;

    websocket_error(wsc, "Protocol exception: %s (code: %d, %s)",
                   reason_txt, reason_code, WEBSOCKET_CLOSE_CODE_2str(reason_code));

    // Always send a close frame with the reason
    websocket_protocol_send_close(wsc, reason_code, reason_txt);

    // Update state based on current state
    if (wsc->state == WS_STATE_OPEN) {
        // We're initiating the close - transition to server-initiated closing
        wsc->state = WS_STATE_CLOSING_SERVER;
    }
    else if (wsc->state == WS_STATE_CLOSING_CLIENT || wsc->state == WS_STATE_CLOSING_SERVER) {
        // Already in closing state, nothing to do
        websocket_debug(wsc, "Protocol exception during closing state %s",
                       WEBSOCKET_STATE_2str(wsc->state));
    }
    else {
        // For any other state, move straight to CLOSED
        wsc->state = WS_STATE_CLOSED;
    }

    // For severe protocol errors, force immediate disconnection
    if (reason_code == WS_CLOSE_PROTOCOL_ERROR ||
        reason_code == WS_CLOSE_POLICY_VIOLATION ||
        reason_code == WS_CLOSE_INVALID_PAYLOAD) {

        websocket_info(wsc, "Forcing immediate disconnection due to protocol exception");

        // First try to flush outgoing data to send the close frame
        websocket_write_data(wsc);

        // Remove client from thread
        if (wsc->wth) {
            websocket_thread_send_command(wsc->wth, WEBSOCKET_THREAD_CMD_REMOVE_CLIENT, wsc->id);
        }
    }
}

#define WS_ALLOW_USE       ( 1)
#define WS_ALLOW_DISCARD   ( 0)
#define WS_ALLOW_ERROR     (-1)

// Centralized function to check if a frame is allowed based on connection state
static int websocket_is_frame_allowed(WS_CLIENT *wsc, const WEBSOCKET_FRAME_HEADER *header) {
    if (!wsc || !header)
        return WS_ALLOW_ERROR;

    bool is_control = websocket_frame_is_control_opcode(header->opcode);

    // Check state-based restrictions
    switch (wsc->state) {
        case WS_STATE_OPEN:
            // In OPEN state, all frames are allowed
            return WS_ALLOW_USE;

        case WS_STATE_CLOSING_SERVER:
            // When server initiated closing, only control frames are allowed
            if (!is_control) {
                websocket_debug(wsc, "Non-control frame rejected in CLOSING_SERVER state");
                return WS_ALLOW_DISCARD;
            }
            return WS_ALLOW_USE;

        case WS_STATE_CLOSING_CLIENT:
            // When client initiated closing, we shouldn't process any further frames
            // All frames in this state should be silently ignored
            websocket_debug(wsc, "Frame rejected in CLOSING_CLIENT state (will be silently ignored)");
            return WS_ALLOW_DISCARD;

        case WS_STATE_CLOSED:
            // In CLOSED state, no frames should be processed
            websocket_debug(wsc, "Frame rejected in CLOSED state");
            return WS_ALLOW_DISCARD;

        case WS_STATE_HANDSHAKE:
            // In HANDSHAKE state, no WebSocket frames should be processed yet
            websocket_debug(wsc, "Frame rejected in HANDSHAKE state");
            return WS_ALLOW_ERROR;

        default:
            // Unknown state - reject frame
            websocket_debug(wsc, "Frame rejected in unknown state: %d", wsc->state);
            return WS_ALLOW_DISCARD;
    }
}

// Helper function to handle frame header parsing
bool websocket_protocol_parse_header_from_buffer(const char *buffer, size_t length,
                                                      WEBSOCKET_FRAME_HEADER *header) {
    if (!buffer || !header || length < 2) {
        websocket_debug(NULL, "We need at least 2 bytes to parse a header: buffer=%p, length=%zu", buffer, length);
        return false;
    }

    // Get first byte - contains FIN bit, RSV bits, and opcode
    unsigned char byte1 = (unsigned char)buffer[0];
    header->fin = (byte1 & WS_FIN) ? 1 : 0;
    header->rsv1 = (byte1 & WS_RSV1) ? 1 : 0;
    header->rsv2 = (byte1 & (WS_RSV1 >> 1)) ? 1 : 0;
    header->rsv3 = (byte1 & (WS_RSV1 >> 2)) ? 1 : 0;
    header->opcode = byte1 & 0x0F;
    
    // Get second byte - contains MASK bit and initial length
    unsigned char byte2 = (unsigned char)buffer[1];
    header->mask = (byte2 & WS_MASK) ? 1 : 0;
    header->len = byte2 & 0x7F;
    
    // Calculate header size and payload length based on length field
    header->header_size = 2; // Start with 2 bytes for the basic header
    
    // Determine payload length
    if (header->len < 126) {
        header->payload_length = header->len;
    } 
    else if (header->len == 126) {
        // 16-bit length
        if (length < 4) {
            websocket_debug(NULL, "We need at least 4 bytes to parse this header: buffer=%p, length=%zu", buffer, length);
            return false; // Not enough data
        }
        
        header->payload_length = ((uint64_t)((unsigned char)buffer[2]) << 8) | ((uint64_t)((unsigned char)buffer[3]));
        header->header_size += 2;
    } 
    else if (header->len == 127) {
        // 64-bit length
        if (length < 10) {
            websocket_debug(NULL, "We need at least 10 bytes to parse this header: buffer=%p, length=%zu", buffer, length);
            return false; // Not enough data
        }
        
        header->payload_length =
            ((uint64_t)((unsigned char)buffer[2]) << 56) | 
            ((uint64_t)((unsigned char)buffer[3]) << 48) | 
            ((uint64_t)((unsigned char)buffer[4]) << 40) | 
            ((uint64_t)((unsigned char)buffer[5]) << 32) | 
            ((uint64_t)((unsigned char)buffer[6]) << 24) | 
            ((uint64_t)((unsigned char)buffer[7]) << 16) | 
            ((uint64_t)((unsigned char)buffer[8]) << 8) | 
            ((uint64_t)((unsigned char)buffer[9]));
        header->header_size += 8;
    }
    
    // Read masking key if frame is masked
    if (header->mask) {
        if (length < header->header_size + 4) return false; // Not enough data
        
        // Copy masking key
        memcpy(header->mask_key, buffer + header->header_size, 4);
        header->header_size += 4;
    } else {
        // Clear mask key if not masked
        memset(header->mask_key, 0, 4);
    }

    header->payload = (void *)&buffer[header->header_size];
    header->frame_size = header->header_size + header->payload_length;
    
    return true;
}

// Validate a parsed frame header according to WebSocket protocol rules
static bool websocket_protocol_validate_header(
    WS_CLIENT *wsc, WEBSOCKET_FRAME_HEADER *header,
                                            uint64_t payload_length, bool in_fragment_sequence) {
    if (!wsc || !header)
        return false;

    // Check RSV bits - must be 0 unless extensions are negotiated
    if (header->rsv2 || header->rsv3) {
        websocket_error(wsc, "Invalid frame: RSV2 or RSV3 bits set");
        websocket_protocol_exception(wsc, WS_CLOSE_PROTOCOL_ERROR, "RSV2 or RSV3 bits set");
        return false;
    }

    // RSV1 is only valid if compression is enabled
    if (header->rsv1 && (!wsc->compression.enabled)) {
        websocket_error(wsc, "Invalid frame: RSV1 bit set but compression not enabled");
        websocket_protocol_exception(wsc, WS_CLOSE_PROTOCOL_ERROR, "RSV1 bit set without compression");
        return false;
    }

    // For continuation frames in a compressed message, RSV1 must be 0 per RFC 7692
    // Continuation frames for a compressed message must not have RSV1 set
    if (header->opcode == WS_OPCODE_CONTINUATION && in_fragment_sequence && header->rsv1) {
        websocket_error(wsc, "Invalid frame: Continuation frame should not have RSV1 bit set");
        websocket_protocol_exception(wsc, WS_CLOSE_PROTOCOL_ERROR, "RSV1 bit set on continuation frame");
        return false;
    }

    // Check opcode validity
    switch (header->opcode) {
        case WS_OPCODE_CONTINUATION:
            // Continuation frames must be in a fragment sequence
            if (!in_fragment_sequence) {
                websocket_error(wsc, "Invalid frame: Continuation frame without initial frame");
                websocket_protocol_exception(wsc, WS_CLOSE_PROTOCOL_ERROR, "Continuation frame without initial frame");
                return false;
            }
            break;

        case WS_OPCODE_TEXT:
        case WS_OPCODE_BINARY:
            // New data frames cannot start inside a fragment sequence
            if (in_fragment_sequence) {
                websocket_error(wsc, "Invalid frame: New data frame during fragmented message");
                websocket_protocol_exception(wsc, WS_CLOSE_PROTOCOL_ERROR, "New data frame during fragmented message");
                return false;
            }
            break;

        case WS_OPCODE_CLOSE:
        case WS_OPCODE_PING:
        case WS_OPCODE_PONG:
            // Control frames must not be fragmented
            if (!header->fin) {
                websocket_error(wsc, "Invalid frame: Fragmented control frame");
                websocket_protocol_exception(wsc, WS_CLOSE_PROTOCOL_ERROR, "Fragmented control frame");
                return false;
            }

            // Control frames must have payload ≤ 125 bytes
            if (payload_length > 125) {
                websocket_error(wsc, "Invalid frame: Control frame payload too large (%llu bytes)",
                           (unsigned long long)payload_length);
                websocket_protocol_exception(wsc, WS_CLOSE_PROTOCOL_ERROR, "Control frame payload too large");
                return false;
            }
            break;

        default:
            // Unknown opcode
            websocket_error(wsc, "Invalid frame: Unknown opcode: 0x%x", (unsigned int)header->opcode);
            websocket_protocol_exception(wsc, WS_CLOSE_PROTOCOL_ERROR, "Unknown opcode");
            return false;
    }

    // Validate payload length against limits
    if (payload_length > (uint64_t)WS_MAX_INCOMING_FRAME_SIZE) {
        websocket_error(wsc, "Invalid frame: Payload too large (%llu bytes)",
                   (unsigned long long)payload_length);
        websocket_protocol_exception(wsc, WS_CLOSE_MESSAGE_TOO_BIG, "Frame payload too large");
        return false;
    }

    // All checks passed
    return true;
}

// Process a control frame directly without creating a message structure
static bool websocket_protocol_process_control_message(
    WS_CLIENT *wsc, WEBSOCKET_OPCODE opcode,
                                                    char *payload, size_t payload_length,
                                                    bool is_masked, const unsigned char *mask_key) {
    websocket_debug(wsc, "Processing control frame opcode=0x%x, payload_length=%zu, is_masked=%d, connection state=%d",
                  opcode, payload_length, is_masked, wsc->state);

    // If payload is masked, unmask it first
    if (is_masked && mask_key && payload && payload_length > 0)
        websocket_unmask(payload, payload, payload_length, mask_key);

    switch (opcode) {
        case WS_OPCODE_CLOSE: {
            worker_is_busy(WORKERS_WEBSOCKET_MSG_CLOSE);

            uint16_t code = WS_CLOSE_NORMAL;
            char reason[124];
            reason[0] = '\0';  // Initialize reason string

            // Check for malformed CLOSE frame payload
            if (payload && payload_length == 1) {
                websocket_error(wsc, "Invalid CLOSE frame payload length: 1 byte (must be 0 or >= 2 bytes)");

                // This is a protocol violation - handle it consistently through the protocol exception mechanism
                websocket_protocol_exception(wsc, WS_CLOSE_PROTOCOL_ERROR, "Invalid payload length");

                // Return true since we handled the message
                return true;
            }
            // Parse close code if present
            else if (payload && payload_length >= 2) {
                code = ((uint16_t)((unsigned char)payload[0]) << 8) |
                       ((uint16_t)((unsigned char)payload[1]));

                // Validate close code
                if (!websocket_validate_close_code(code)) {
                    websocket_error(wsc, "Invalid close code: %u", code);

                    // This is a protocol violation - handle it through the protocol exception mechanism
                    // This will send a close frame with 1002 (protocol error) and set the appropriate state
                    websocket_protocol_exception(wsc, WS_CLOSE_PROTOCOL_ERROR, "Invalid close code");

                    // Return true since we handled the message
                    return true;
                }
                // Check UTF-8 validity of the reason text
                else if (payload_length > 2) {
                    // RFC 6455 requires all control frame payloads (including close reasons) to be valid UTF-8
                    if (!websocket_validate_utf8(payload + 2, payload_length - 2)) {
                        websocket_error(wsc, "Invalid UTF-8 in close frame reason");
                        code = WS_CLOSE_INVALID_PAYLOAD;  // 1007 - Invalid frame payload data
                        strncpyz(reason, "Invalid UTF-8 in close reason", sizeof(reason) - 1);
                    }
                    else {
                        // Valid UTF-8, copy the reason
                        strncpyz(reason, payload + 2, MIN(payload_length - 2, sizeof(reason) - 1));
                    }
                }
            }

            // Different handling based on connection state
            if (wsc->state == WS_STATE_OPEN) {
                // This is the initial CLOSE from client - respond with our own CLOSE
                websocket_debug(wsc, "Received initial CLOSE frame from client, responding with CLOSE");

                // Send close frame in response
                websocket_protocol_send_close(wsc, code, reason);

                wsc->state = WS_STATE_CLOSING_CLIENT;
                wsc->flush_and_remove_client = true;

                // IMPORTANT: do not call websocket_write_data() here
                // because it prevents wsc->flush_and_remove_client from removing the client!
            }
            else if (wsc->state == WS_STATE_CLOSING_SERVER) {
                // We initiated the closing handshake and now received client's response
                // This completes the closing handshake
                websocket_debug(wsc, "Closing handshake complete - received client's CLOSE response to our close");

                // Ensure immediate removal from thread/poll
                if (wsc->wth) {
                    websocket_info(wsc, "Closing TCP connection after completed handshake (server initiated)");
                    websocket_thread_send_command(wsc->wth, WEBSOCKET_THREAD_CMD_REMOVE_CLIENT, wsc->id);
                }

                // The RFC requires us to close the TCP connection immediately now
                wsc->state = WS_STATE_CLOSED;
            }
            else if (wsc->state == WS_STATE_CLOSING_CLIENT) {
                // Client already sent a CLOSE, and we responded, but they sent another CLOSE
                // This is not strictly according to protocol, but we'll handle it gracefully
                websocket_debug(wsc, "Received another CLOSE frame while in client-initiated closing state");

                // Remove client from thread
                if (wsc->wth) {
                    websocket_info(wsc, "Closing TCP connection (duplicate close from client)");
                    websocket_thread_send_command(wsc->wth, WEBSOCKET_THREAD_CMD_REMOVE_CLIENT, wsc->id);
                }

                // Move to CLOSED state and close the connection
                wsc->state = WS_STATE_CLOSED;
            }
            else {
                // Already in CLOSED state - ignore
                websocket_debug(wsc, "Ignoring CLOSE frame - connection already in CLOSED state");
            }
            return true;
        }

        case WS_OPCODE_PING:
            worker_is_busy(WORKERS_WEBSOCKET_MSG_PING);

            // If we're in CLOSING or CLOSED state, decide how to handle PING based on state
            if (wsc->state == WS_STATE_CLOSING_SERVER || wsc->state == WS_STATE_CLOSING_CLIENT || wsc->state == WS_STATE_CLOSED) {
                // When we initiated closing, we should still respond to control frames
                if (wsc->state == WS_STATE_CLOSING_SERVER) {
                    websocket_debug(wsc, "Received PING during server-initiated closing, responding with PONG");
                    return websocket_protocol_send_pong(wsc, payload, payload_length) > 0;
                }

                // For client-initiated closing or CLOSED, we should ignore control frames
                websocket_debug(wsc, "Ignoring PING frame - connection in %s state",
                             wsc->state == WS_STATE_CLOSING_CLIENT ? "client closing" : "closed");
                return true; // Successfully processed (by ignoring)
            }

            // Ping frame - respond with pong
            websocket_debug(wsc, "Received PING frame with %zu bytes, responding with PONG", payload_length);

            // Send pong with the same payload
            return websocket_protocol_send_pong(wsc, payload, payload_length) > 0;

        case WS_OPCODE_PONG:
            worker_is_busy(WORKERS_WEBSOCKET_MSG_PONG);

            // If we're in CLOSING or CLOSED state, decide how to handle PONG
            if (wsc->state == WS_STATE_CLOSING_SERVER || wsc->state == WS_STATE_CLOSING_CLIENT || wsc->state == WS_STATE_CLOSED) {
                // We can safely ignore PONG frames in any closing or closed state
                websocket_debug(wsc, "Ignoring PONG frame - connection in %s state",
                             wsc->state == WS_STATE_CLOSING_SERVER ? "server closing" :
                             wsc->state == WS_STATE_CLOSING_CLIENT ? "client closing" : "closed");
                return true; // Successfully processed (by ignoring)
            }

            // Pong frame - update last activity time
            websocket_debug(wsc, "Received PONG frame, updating last activity time");
            wsc->last_activity_t = now_monotonic_sec();
            return true;

        default:
            worker_is_busy(WORKERS_WEBSOCKET_MSG_INVALID);
            break;
    }

    websocket_error(wsc, "Unknown control opcode: %d", opcode);
    return false;
}

// Parse a frame from a buffer and append it to the current message if applicable.
// Returns one of the following:
// - WS_FRAME_ERROR: An error occurred, connection should be closed
// - WS_FRAME_NEED_MORE_DATA: More data is needed to complete the frame
// - WS_FRAME_COMPLETE: Frame was successfully parsed and handled, but is not a complete message yet
// - WS_FRAME_MESSAGE_READY: Message is ready for processing
static WEBSOCKET_FRAME_RESULT
websocket_protocol_consume_frame(WS_CLIENT *wsc, char *data, size_t length, ssize_t *bytes_processed) {
    if (!wsc || !data || !length || !bytes_processed)
        return WS_FRAME_ERROR;
        
    size_t bytes = *bytes_processed = 0;

    // Local variables for frame processing
    WEBSOCKET_FRAME_HEADER header = { 0 };

    // Step 1: Parse the frame header
    if (!websocket_protocol_parse_header_from_buffer(data, length, &header)) {
        websocket_debug(wsc, "Not enough data to parse a complete header: bytes available = %zu",
                        length);
        return WS_FRAME_NEED_MORE_DATA;
    }
    
    if(header.frame_size > wsc->max_message_size)
        wsc->max_message_size = header.frame_size;

    // Check if we have enough data for the complete frame (header + payload)
    // If not, don't consume any bytes and wait for more data
    if (bytes + header.frame_size > length) {

        // let the circular buffer know how much data we need
        wsc->next_frame_size = header.frame_size;

        worker_is_busy(WORKERS_WEBSOCKET_INCOMPLETE_FRAME);
        websocket_debug(wsc,
                        "RX FRAME INCOMPLETE (need %zu bytes more): OPCODE=0x%x, FIN=%s, RSV1=%d, RSV2=%d, RSV3=%d, MASK=%s, LEN=%d, "
                        "PAYLOAD_LEN=%zu, HEADER_SIZE=%zu, FRAME_SIZE=%zu, MASK=%02x%02x%02x%02x, bytes available = %zu",
                        (bytes + header.frame_size) - length,
                        header.opcode, header.fin ? "True" : "False", header.rsv1, header.rsv2, header.rsv3,
                        header.mask ? "True" : "False", header.len,
                        header.payload_length, header.header_size, header.frame_size,
                        header.mask_key[0], header.mask_key[1], header.mask_key[2], header.mask_key[3],
                        length);

        return WS_FRAME_NEED_MORE_DATA;
    }
    wsc->next_frame_size = 0; // reset it, since we have enough data now
    
    worker_is_busy(WORKERS_WEBSOCKET_COMPLETE_FRAME);

    // Log detailed header information for debugging
    websocket_debug(wsc,
                    "RX FRAME: OPCODE=0x%x, FIN=%s, RSV1=%d, RSV2=%d, RSV3=%d, MASK=%s, LEN=%d, "
                    "PAYLOAD_LEN=%zu, HEADER_SIZE=%zu, FRAME_SIZE=%zu, MASK=%02x%02x%02x%02x",
                    header.opcode, header.fin ? "True" : "False", header.rsv1, header.rsv2, header.rsv3,
                    header.mask ? "True" : "False", header.len,
                    header.payload_length, header.header_size, header.frame_size,
                    header.mask_key[0], header.mask_key[1], header.mask_key[2], header.mask_key[3]);

    // Check for invalid RSV bits
    if (header.rsv2 || header.rsv3 || (header.rsv1 && !wsc->compression.enabled)) {
        const char *reason = header.rsv2 ? "RSV2 bit set" :
                            (header.rsv3 ? "RSV3 bit set" : "RSV1 bit set without compression");

        // Handle protocol exception in a centralized way
        websocket_protocol_exception(wsc, WS_CLOSE_PROTOCOL_ERROR, reason);
        return WS_FRAME_ERROR;
    }

    // Check if this frame is allowed in the current connection state
    switch(websocket_is_frame_allowed(wsc, &header)) {
        case WS_ALLOW_USE:
            break;

        case WS_ALLOW_DISCARD:
            // we have already logged in websocket_is_frame_allowed()
            return WS_FRAME_COMPLETE;

        default:
        case WS_ALLOW_ERROR: {
            char reason[128];

            snprintf(reason, sizeof(reason), "Frame not allowed in %s state",
                     wsc->state == WS_STATE_CLOSING_SERVER ? "server closing" :
                     wsc->state == WS_STATE_CLOSING_CLIENT ? "client closing" :
                     "current");

            websocket_protocol_exception(wsc, WS_CLOSE_PROTOCOL_ERROR, reason);
            return WS_FRAME_ERROR;
        }
    }

    // Step 2: Validate the frame header
    if (!websocket_protocol_validate_header(wsc, &header, header.payload_length, !wsc->message_complete)) {
        // Invalid frame - websocket_protocol_validate_header sent a close frame
        // but we should handle the connection closing consistently
        websocket_protocol_exception(wsc, WS_CLOSE_PROTOCOL_ERROR, "Invalid frame header");
        return WS_FRAME_ERROR;
    }

    // Advance past the header
    bytes += header.header_size;
    
    if (websocket_frame_is_control_opcode(header.opcode)) {
        // Handle control frames (PING, PONG, CLOSE) directly
        websocket_debug(wsc, "Handling control frame: opcode=0x%x, payload_length=%zu",
                        (unsigned)header.opcode, header.payload_length);

        // Process the control frame with optional payload
        char *payload = (header.payload_length > 0) ? (data + bytes) : NULL;

        // Process control message directly without creating a message object
        if (!websocket_protocol_process_control_message(
                wsc, (WEBSOCKET_OPCODE)header.opcode,
                payload, header.payload_length,
                header.mask ? true : false,
                header.mask_key)) {
            websocket_error(wsc, "Failed to process control frame");
            return WS_FRAME_ERROR;
        }

        // Update bytes processed
        if (header.payload_length > 0)
            bytes += (size_t)header.payload_length;

        *bytes_processed = bytes;

        return WS_FRAME_COMPLETE; // Return COMPLETE so we continue processing other frames
    }

    // For non-control frames (text, binary, continuation), check if connection is closing
    if (wsc->state == WS_STATE_CLOSING_SERVER || wsc->state == WS_STATE_CLOSING_CLIENT || wsc->state == WS_STATE_CLOSED) {
        // Per RFC 6455, once the closing handshake is started, we should ignore non-control frames
        websocket_debug(wsc, "Ignoring non-control frame (opcode=0x%x) - connection in %s state",
                    header.opcode,
                    wsc->state == WS_STATE_CLOSING_SERVER ? "server closing" :
                    wsc->state == WS_STATE_CLOSING_CLIENT ? "client closing" : "closed");

        // Consume the frame bytes but don't process it
        bytes += header.header_size + header.payload_length;
        *bytes_processed = bytes;

        return WS_FRAME_COMPLETE;
    }

    // Step 3: Handle the frame based on its opcode
    if (header.opcode == WS_OPCODE_CONTINUATION) {
        // This is a continuation frame - need an existing message in progress
        if (wsc->message_complete) {
            websocket_error(wsc, "Received continuation frame with no message in progress");
            websocket_protocol_exception(wsc, WS_CLOSE_PROTOCOL_ERROR, "Continuation frame without initial frame");
            return WS_FRAME_ERROR;
        }

        // If it's a zero-length frame, we don't need to append any data
        if (header.payload_length == 0) {
            // For zero-length non-final frames, just update and continue
            if (!header.fin) {
                // Non-final zero-length frame
                websocket_debug(wsc, "Zero-length non-final continuation frame");
                *bytes_processed = bytes;
                wsc->frame_id++;
                return WS_FRAME_COMPLETE;
            }

            // Final zero-length frame - mark message as complete
            wsc->message_complete = true;
            *bytes_processed = bytes;
            return WS_FRAME_MESSAGE_READY;
        }

        // The message buffer length is updated as we append frame data
    }
    else {
        if(!header.payload_length) {
            websocket_debug(wsc, "Received data frame with zero-length payload (fin=%d)", header.fin);

            // Initialize the client's message state for a new message
            websocket_client_message_reset(wsc);
            wsc->opcode = (WEBSOCKET_OPCODE)header.opcode;
            wsc->is_compressed = header.rsv1 ? true : false;

            // The most important part: for fragmented messages (non-final frames),
            // we must set message_complete to false, regardless of payload length
            wsc->message_complete = header.fin;

            // Buffer length is already 0 after reset
            wsc->frame_id = 0;

            // Check if this is a final frame
            if (header.fin) {
                // Final frame - message is already marked as complete
                *bytes_processed = bytes;
                return WS_FRAME_MESSAGE_READY;
            } else {
                // Non-final frame - continue to next frame
                *bytes_processed = bytes;
                wsc->frame_id++;
                return WS_FRAME_COMPLETE;
            }
        }

        // This is a new data frame (TEXT or BINARY)
        // If we have an existing message in progress, it's an error
        if (!wsc->message_complete) {
            websocket_error(wsc, "Received new data frame while another message is in progress");
            websocket_protocol_exception(wsc, WS_CLOSE_PROTOCOL_ERROR, "New data frame during fragmented message");
            return WS_FRAME_ERROR;
        }

        // Initialize the client's message state for a new message
        websocket_client_message_reset(wsc);
        wsc->opcode = (WEBSOCKET_OPCODE)header.opcode;
        wsc->is_compressed = header.rsv1 ? true : false;

        // For fragmented messages (non-final frames), we must set message_complete to false
        // This needs to be consistently done for both empty and non-empty frames
        wsc->message_complete = header.fin;

        // Buffer length will be updated when we append the payload data
        wsc->frame_id = 0;
    }

    // Step 4: Append payload data to the message

    if (header.payload_length > 0) {
        char *src = header.payload;

        if (header.mask) {
            // Payload is masked - need to unmask it first
            websocket_debug(wsc, "Unmasking and appending payload data at position %zu (key=%02x%02x%02x%02x)",
                wsb_length(&wsc->payload),
                            header.mask_key[0], header.mask_key[1], header.mask_key[2], header.mask_key[3]);

            // Use the new helper function to unmask and append the data in one step
            wsb_unmask_and_append(&wsc->payload, src, header.payload_length, header.mask_key);
        }
        else {
            // Payload is not masked - can directly append
            websocket_debug(wsc, "Appending unmasked payload data at position %zu", wsb_length(&wsc->payload));

            // Append unmasked data directly
            wsb_append(&wsc->payload, src, header.payload_length);
        }

        // Dump payload for debugging
        size_t buffer_length = wsb_length(&wsc->payload);
        websocket_dump_debug(wsc,
            wsb_data(&wsc->payload) + (buffer_length - header.payload_length),
                            header.payload_length,
                            "RX FRAME PAYLOAD");
    }

    // Step 5: At this point, we know we've processed a complete frame
    wsc->frame_id++;

    bytes += header.payload_length;
    *bytes_processed = bytes;

    // If this is a final frame, mark the message as complete
    if (header.fin)
        return WS_FRAME_MESSAGE_READY;

    // Non-final frame, message is incomplete - move to next frame
    return WS_FRAME_COMPLETE;
}

// Process incoming data from the WebSocket client
// This function's job is to:
// 1. Consume frames from the input buffer 
// 2. Build messages
// 3. Process complete messages
ssize_t websocket_protocol_got_data(WS_CLIENT *wsc, char *data, size_t length) {
    if (!wsc || !data || !length)
        return -1;
    
    // Keep processing frames until we can't process any more
    size_t processed = 0;
    while (processed < length) {
        // Try to consume one complete frame
        ssize_t consumed = 0;
        WEBSOCKET_FRAME_RESULT result = websocket_protocol_consume_frame(wsc, data + processed, length - processed, &consumed);

        websocket_debug(wsc, "Frame processing result: %d, processed: %zu/%zu", result, consumed, length);
        
        // Safety check to ensure we always move forward in the buffer
        if (consumed == 0 && result != WS_FRAME_NEED_MORE_DATA && result != WS_FRAME_ERROR) {
            // If we're processing a frame but not consuming bytes, we might be stuck
            websocket_error(wsc, "Protocol processing stalled - consumed 0 bytes but not waiting for more data (%d)", (int)result);
            return -(ssize_t)processed;
        }
        
        switch (result) {
            case WS_FRAME_ERROR:
                // Error occurred during frame processing
                websocket_error(wsc, "Error processing WebSocket frame");
                return processed ? -(ssize_t)processed : -1;

            case WS_FRAME_NEED_MORE_DATA:
                // Need more data to complete the current frame
                websocket_debug(wsc, "Need more data to complete the current frame");
                return (ssize_t)processed;

            case WS_FRAME_COMPLETE:
                // Frame was processed successfully, but more frames are needed for a complete message
                websocket_debug(wsc, "Frame complete, but message not yet complete");
                processed += consumed;
                continue;

            case WS_FRAME_MESSAGE_READY:
                worker_is_busy(WORKERS_WEBSOCKET_MESSAGE);
                processed += consumed;

                wsc->message_complete = true;
                if (!websocket_client_process_message(wsc))
                    websocket_error(wsc, "Failed to process completed message");

                continue;
        }
    }
    
    return (ssize_t)processed;
}

