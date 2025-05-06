// SPDX-License-Identifier: GPL-3.0-or-later

#include "daemon/common.h"
#include "websocket-internal.h"

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
    worker_is_busy(WORKERS_WEBSOCKET_SOCK_RECEIVE);

    if (!wsc || !wsc->in_buffer.data || wsc->sock.fd < 0)
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
    websocket_dump_debug(wsc, buffer, bytes_read, "READ %zd bytes", bytes_read);

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

// Actually write data to the client socket
ssize_t websocket_write_data(WS_CLIENT *wsc) {
    worker_is_busy(WORKERS_WEBSOCKET_SOCK_SEND);

    if (!wsc || !wsc->out_buffer.data || wsc->sock.fd < 0)
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
    websocket_dump_debug(wsc, data, data_length, "WRITE %zu bytes", data_length);

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

// Handle socket takeover from web client - similar to stream_receiver_takeover_web_connection
void websocket_takeover_web_connection(struct web_client *w, WS_CLIENT *wsc) {
    // Set the file descriptor and ssl from the web client
    wsc->sock.fd = w->fd;
    wsc->sock.ssl = w->ssl;

    w->ssl = NETDATA_SSL_UNSET_CONNECTION;

    WEB_CLIENT_IS_DEAD(w);

    if(web_server_mode == WEB_SERVER_MODE_STATIC_THREADED) {
        web_client_flag_set(w, WEB_CLIENT_FLAG_DONT_CLOSE_SOCKET);
    }
    else {
        w->fd = -1;
    }

    // Clear web client buffer
    buffer_flush(w->response.data);
}
