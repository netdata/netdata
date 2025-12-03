// SPDX-License-Identifier: GPL-3.0-or-later

#include "daemon/daemon-service.h"
#include "websocket-internal.h"

static void websocket_thread_client_socket_error(WEBSOCKET_THREAD *wth, WS_CLIENT *wsc, const char *reason) {
    internal_fatal(wth->tid != gettid_cached(), "Function %s() should only be used by the websocket thread", __FUNCTION__ );

    worker_is_busy(WORKERS_WEBSOCKET_SOCK_ERROR);

    websocket_debug(wsc, reason);

    // Send command to remove the client
    // Note: on_disconnect will be called in websocket_thread_remove_client
    websocket_thread_send_command(wth, WEBSOCKET_THREAD_CMD_REMOVE_CLIENT, wsc->id);
}

// Add a client to a thread's poll
static bool websocket_thread_add_client(WEBSOCKET_THREAD *wth, WS_CLIENT *wsc) {
    internal_fatal(wth->tid != gettid_cached(), "Function %s() should only be used by the websocket thread", __FUNCTION__ );

    // Initialize compression with the parsed options
    websocket_compression_init(wsc);
    websocket_decompression_init(wsc);

    // Add client to the poll - use the socket fd directly
    bool added = nd_poll_add(wth->ndpl, wsc->sock.fd, ND_POLL_READ, wsc);
    if(!added) {
        websocket_error(wsc, "Failed to add client to poll");
        return false;
    }

    // Add client to the thread's client list
    DOUBLE_LINKED_LIST_APPEND_ITEM_UNSAFE(wth->clients, wsc, prev, next);

    return true;
}

static void websocket_thread_remove_client(WEBSOCKET_THREAD *wth, WS_CLIENT *wsc, bool have_lock) {
    internal_fatal(wth->tid != gettid_cached(), "Function %s() should only be used by the websocket thread", __FUNCTION__ );

    // Notify the protocol handler that the client is being disconnected
    if (wsc->on_disconnect) {
        websocket_debug(wsc, "Calling on_disconnect callback for protocol %s", WEBSOCKET_PROTOCOL_2str(wsc->protocol));
        wsc->on_disconnect(wsc);
    }

    // send a close frame (it won't do it if not allowed by the protocol)
    websocket_protocol_send_close(wsc, WS_CLOSE_NORMAL, "Connection closed by server");

    // If already in a closing state, just flush any pending data
    websocket_write_data(wsc);

    // Remove client from the poll - use socket fd directly
    bool removed = nd_poll_del(wth->ndpl, wsc->sock.fd);
    if(!removed) {
        websocket_debug(wsc, "Failed to remove client %zu from poll", wsc->id);
    }

    websocket_decompression_cleanup(wsc);
    websocket_compression_cleanup(wsc);

    // Remove client from the thread's client list
    DOUBLE_LINKED_LIST_REMOVE_ITEM_UNSAFE(wth->clients, wsc, prev, next);

    // Lock the thread clients
    if(!have_lock)
        spinlock_lock(&wth->clients_spinlock);

    if(wth->clients_current > 0)
        wth->clients_current--;

    // Release the thread clients lock
    if(!have_lock)
        spinlock_unlock(&wth->clients_spinlock);

    websocket_debug(wsc, "Removed and resources freed", wth->id, wsc->id);
    websocket_client_free(wsc);
}

// Update a client's poll event flags
bool websocket_thread_update_client_poll_flags(WS_CLIENT *wsc) {
    if(!wsc || !wsc->wth || wsc->sock.fd < 0)
        return false;

    internal_fatal(wsc->wth->tid != gettid_cached(), "Function %s() should only be used by the websocket thread", __FUNCTION__ );

    nd_poll_event_t events = wsc->flush_and_remove_client ? 0 : ND_POLL_READ;
    if(cbuffer_next_unsafe(&wsc->out_buffer, NULL) > 0)
        events |= ND_POLL_WRITE;

    // Update poll events
    bool updated = nd_poll_upd(wsc->wth->ndpl, wsc->sock.fd, events);
    if(!updated)
        websocket_error(wsc, "Failed to update poll events for client");

    return updated;
}

struct pipe_header {
    uint8_t cmd;
    union {
        uint32_t id;
        uint32_t len;
    };
};

// Send command to a thread
bool websocket_thread_send_command(WEBSOCKET_THREAD *wth, uint8_t cmd, uint32_t id) {
    if(!wth || wth->cmd.pipe[PIPE_WRITE] == -1) {
        netdata_log_error("WEBSOCKET[%zu]: Failed to send command - pipe is not initialized", wth ? wth->id : 0);
        return false;
    }

    // Prepare command
    struct pipe_header header = {
        .cmd = cmd,
        .id = id,
    };

    // Lock command pipe for writing
    spinlock_lock(&wth->spinlock);

    // Write command header
    ssize_t bytes = write(wth->cmd.pipe[PIPE_WRITE], &header, sizeof(header));
    if(bytes != sizeof(header)) {
        netdata_log_error("WEBSOCKET[%zu]: Failed to write command header to pipe", wth->id);
        spinlock_unlock(&wth->spinlock);
        return false;
    }

    // Release command pipe
    spinlock_unlock(&wth->spinlock);

    return true;
}

bool websocket_thread_send_broadcast(WEBSOCKET_THREAD *wth, WEBSOCKET_OPCODE opcode, const char *message) {
    if(!wth || wth->cmd.pipe[PIPE_WRITE] == -1) {
        netdata_log_error("WEBSOCKET[%zu]: Failed to send command - pipe is not initialized", wth ? wth->id : 0);
        return false;
    }

    uint32_t message_len = strlen(message);

    // Prepare command
    struct pipe_header header = {
        .cmd = WEBSOCKET_THREAD_CMD_BROADCAST,
        .len = sizeof(opcode) + message_len,
    };

    // Lock command pipe for writing
    spinlock_lock(&wth->spinlock);

    // Write command header
    ssize_t bytes = write(wth->cmd.pipe[PIPE_WRITE], &header, sizeof(header.cmd));
    if(bytes != sizeof(header.cmd)) {
        netdata_log_error("WEBSOCKET[%zu]: Failed to write command header to pipe", wth->id);
        spinlock_unlock(&wth->spinlock);
        return false;
    }

    // Write the opcode
    bytes = write(wth->cmd.pipe[PIPE_WRITE], &opcode, sizeof(opcode));
    if(bytes != sizeof(opcode)) {
        netdata_log_error("WEBSOCKET[%zu]: Failed to write broadcast opcode to pipe", wth->id);
        spinlock_unlock(&wth->spinlock);
        return false;
    }

    // Write the message
    bytes = write(wth->cmd.pipe[PIPE_WRITE], message, message_len);;
    if(bytes != message_len) {
        netdata_log_error("WEBSOCKET[%zu]: Failed to write broadcast message to pipe", wth->id);
        spinlock_unlock(&wth->spinlock);
        return false;
    }

    // Release command pipe
    spinlock_unlock(&wth->spinlock);

    return true;
}

static ssize_t read_pipe_block(int fd, void *buffer, size_t size) {
    char *buf = buffer;
    ssize_t total_read = 0;

    while (total_read < (ssize_t) size) {
        ssize_t bytes = read(fd, buf + total_read, size - total_read);

        if (bytes < 0) {
            if (errno == EAGAIN || errno == EWOULDBLOCK) {
                // Non-blocking case, return what we've read so far
                return total_read;
            }

            // Real error occurred
            return -1;

        }
        else if (bytes == 0)
            return total_read;

        total_read += bytes;
    }

    return total_read;
}

// Process a thread's command pipe
static void websocket_thread_process_commands(WEBSOCKET_THREAD *wth) {
    internal_fatal(wth->tid != gettid_cached(), "Function %s() should only be used by the websocket thread", __FUNCTION__ );

    struct pipe_header header;

    // Read all available commands
    for(;;) {
        // Read command header

        worker_is_busy(WORKERS_WEBSOCKET_CMD_READ);

        ssize_t bytes = read_pipe_block(wth->cmd.pipe[PIPE_READ], &header, sizeof(header));
        if(bytes <= 0) {
            if(bytes < 0 && errno != EAGAIN && errno != EWOULDBLOCK) {
                netdata_log_error("WEBSOCKET[%zu]: Failed to read command header from pipe", wth->id);
            }
            break;
        }

        if(bytes != sizeof(header)) {
            netdata_log_error("WEBSOCKET[%zu]: Read partial command header (%zd/%zu bytes)", wth->id, bytes, sizeof(header));
            break;
        }

        // Process command
        switch(header.cmd) {
            case WEBSOCKET_THREAD_CMD_EXIT:
                worker_is_busy(WORKERS_WEBSOCKET_CMD_EXIT);
                netdata_log_info("WEBSOCKET[%zu] received exit command", wth->id);
                return;

            case WEBSOCKET_THREAD_CMD_ADD_CLIENT: {
                worker_is_busy(WORKERS_WEBSOCKET_CMD_ADD);
                WS_CLIENT *wsc = websocket_client_find_by_id(header.id);
                if(!wsc) {
                    netdata_log_error("WEBSOCKET[%zu]: Client %u not found for add command", wth->id, header.id);
                    continue;
                }
                internal_fatal(wsc->wth != wth, "Client %u added to wrong thread", header.id);
                wsc->wth = wth;
                if (websocket_thread_add_client(wth, wsc)) {
                    // Call the on_connect callback if provided to notify protocol handler of new client
                    if (wsc->on_connect) {
                        websocket_debug(wsc, "Calling on_connect callback for protocol %s", WEBSOCKET_PROTOCOL_2str(wsc->protocol));
                        wsc->on_connect(wsc);
                    }
                }
                break;
            }

            case WEBSOCKET_THREAD_CMD_REMOVE_CLIENT: {
                worker_is_busy(WORKERS_WEBSOCKET_CMD_DEL);
                WS_CLIENT *wsc = websocket_client_find_by_id(header.id);
                if(!wsc) {
                    netdata_log_error("WEBSOCKET[%zu]: Client %u not found for remove command", wth->id, header.id);
                    continue;
                }

                websocket_thread_remove_client(wth, wsc, false);
                break;
            }

            case WEBSOCKET_THREAD_CMD_BROADCAST: {
                worker_is_busy(WORKERS_WEBSOCKET_CMD_BROADCAST);

                WEBSOCKET_OPCODE opcode;
                bytes = read_pipe_block(wth->cmd.pipe[PIPE_READ], &opcode, sizeof(opcode));
                if(bytes != sizeof(opcode)) {
                    netdata_log_error("WEBSOCKET[%zu]: Failed to read broadcast opcode from pipe", wth->id);
                    continue;
                }

                uint32_t message_len = header.len - sizeof(opcode);
                char message[message_len + 1];
                bytes = read_pipe_block(wth->cmd.pipe[PIPE_READ], message, message_len);
                if(bytes != message_len) {
                    netdata_log_error("WEBSOCKET[%zu]: Failed to read broadcast message from pipe", wth->id);
                    continue;
                }

                // Ensure we have the complete message
                if(header.len != sizeof(WEBSOCKET_OPCODE) + message_len) {
                    netdata_log_error("WEBSOCKET[%zu]: Broadcast command size mismatch", wth->id);
                    continue;
                }
                
                // Send to all clients in this thread
                spinlock_lock(&wth->clients_spinlock);

                WS_CLIENT *wsc = wth->clients;
                while(wsc) {
                    if(wsc->state == WS_STATE_OPEN) {
                        websocket_send_message(wsc, message, message_len, opcode);
                    }
                    wsc = wsc->next;
                }
                
                spinlock_unlock(&wth->clients_spinlock);
                break;
            }

            default:
                worker_is_busy(WORKERS_WEBSOCKET_CMD_UNKNOWN);
                netdata_log_error("WEBSOCKET[%zu]: Unknown command %u", wth->id, header.cmd);
                break;
        }
    }
}

// Thread main function
void websocket_thread(void *ptr) {
    WEBSOCKET_THREAD *wth = (WEBSOCKET_THREAD *)ptr;
    wth->tid = gettid_uncached();

    worker_register("WEBSOCKET");
    worker_register_job_name(WORKERS_WEBSOCKET_POLL, "poll");
    worker_register_job_name(WORKERS_WEBSOCKET_CMD_READ, "cmd read");
    worker_register_job_name(WORKERS_WEBSOCKET_CMD_EXIT, "cmd exit");
    worker_register_job_name(WORKERS_WEBSOCKET_CMD_ADD, "cmd add");
    worker_register_job_name(WORKERS_WEBSOCKET_CMD_DEL, "cmd del");
    worker_register_job_name(WORKERS_WEBSOCKET_CMD_BROADCAST, "cmd bcast");
    worker_register_job_name(WORKERS_WEBSOCKET_CMD_UNKNOWN, "cmd unknown");
    worker_register_job_name(WORKERS_WEBSOCKET_SOCK_RECEIVE, "ws rcv");
    worker_register_job_name(WORKERS_WEBSOCKET_SOCK_SEND, "ws snd");
    worker_register_job_name(WORKERS_WEBSOCKET_SOCK_ERROR, "ws err");
    worker_register_job_name(WORKERS_WEBSOCKET_CLIENT_TIMEOUT, "client timeout");
    worker_register_job_name(WORKERS_WEBSOCKET_SEND_PING, "send ping");
    worker_register_job_name(WORKERS_WEBSOCKET_CLIENT_STUCK, "client stuck");
    worker_register_job_name(WORKERS_WEBSOCKET_INCOMPLETE_FRAME, "incomplete frame");
    worker_register_job_name(WORKERS_WEBSOCKET_COMPLETE_FRAME, "complete frame");
    worker_register_job_name(WORKERS_WEBSOCKET_MESSAGE, "message");
    worker_register_job_name(WORKERS_WEBSOCKET_MSG_PING, "rx ping");
    worker_register_job_name(WORKERS_WEBSOCKET_MSG_PONG, "rx pong");
    worker_register_job_name(WORKERS_WEBSOCKET_MSG_CLOSE, "rx close");
    worker_register_job_name(WORKERS_WEBSOCKET_MSG_INVALID, "rx invalid");

    time_t last_cleanup = now_monotonic_sec();

    // Main thread loop
    while(service_running(SERVICE_STREAMING) && !nd_thread_signaled_to_cancel()) {

        worker_is_idle();

        // Poll for events
        nd_poll_result_t ev;
        int rc = nd_poll_wait(wth->ndpl, 100, &ev); // 100ms timeout

        worker_is_busy(WORKERS_WEBSOCKET_POLL);

        if(rc < 0) {
            if(errno == EAGAIN || errno == EINTR)
                continue;

            netdata_log_error("WEBSOCKET[%zu]: Poll error: %s", wth->id, strerror(errno));
            break;
        }

        // Process poll events
        if(rc > 0) {
            // Handle command pipe
            if(ev.data == &wth->cmd) {
                if(ev.events & ND_POLL_READ) {
                    // Read and process commands
                    websocket_thread_process_commands(wth);
                }
                continue;
            }

            // Handle client events
            WS_CLIENT *wsc = (WS_CLIENT *)ev.data;
            if(!wsc) {
                netdata_log_error("WEBSOCKET[%zu]: Poll event with NULL client data", wth->id);
                continue;
            }

            // Push client connection info to log stack for all subsequent logs
            ND_LOG_STACK lgs[] = {
                ND_LOG_FIELD_U64(NDF_CONNECTION_ID, wsc->id),
                ND_LOG_FIELD_TXT(NDF_SRC_IP, wsc->client_ip),
                ND_LOG_FIELD_TXT(NDF_SRC_PORT, wsc->client_port),
                ND_LOG_FIELD_END(),
            };
            ND_LOG_STACK_PUSH(lgs);

            // Check for errors
            if(ev.events & ND_POLL_HUP) {
                websocket_thread_client_socket_error(wth, wsc, "Client hangup");
                continue;
            }
            if(ev.events & ND_POLL_ERROR) {
                websocket_thread_client_socket_error(wth, wsc, "Socket error");
                continue;
            }

            // Process read events
            if(ev.events & ND_POLL_READ) {
                if(websocket_receive_data(wsc) < 0) {
                    websocket_thread_client_socket_error(wth, wsc, "Failed to receive data");
                    continue;
                }
            }

            // Process write events
            if(ev.events & ND_POLL_WRITE) {
                if(websocket_write_data(wsc) < 0) {
                    websocket_thread_client_socket_error(wth, wsc, "Failed to send data");
                    continue;
                }

                // Check if this client is waiting to be closed after flushing outgoing data
                if(wsc->flush_and_remove_client && cbuffer_used_size_unsafe(&wsc->out_buffer) == 0) {
                    // All data flushed - remove client
                    websocket_thread_remove_client(wth, wsc, false);
                }
            }
        }

        worker_is_idle();

        // Periodic cleanup and health checks (every 30 seconds)
        time_t now = now_monotonic_sec();
        if(now - last_cleanup > 30) {
            // Iterate through all clients in this thread
            spinlock_lock(&wth->clients_spinlock);

            WS_CLIENT *wsc = wth->clients;
            while(wsc) {
                WS_CLIENT *next = wsc->next; // Save next in case we remove this client

                if(wsc->state == WS_STATE_OPEN) {
                    // Check if client is idle (no activity for over WS_IDLE_CHECK_INTERVAL seconds)
                    if(now - wsc->last_activity_t > WS_IDLE_CHECK_INTERVAL) {
                        // Client is idle - send a ping to check if it's still alive
                        worker_is_busy(WORKERS_WEBSOCKET_SEND_PING);
                        websocket_protocol_send_ping(wsc, NULL, 0);

                        // If no activity for over WS_INACTIVITY_TIMEOUT seconds, consider it dead
                        if(now - wsc->last_activity_t > WS_INACTIVITY_TIMEOUT) {
                            worker_is_busy(WORKERS_WEBSOCKET_CLIENT_TIMEOUT);
                            websocket_error(wsc, "Client timed out (no activity for over %d minutes)", WS_INACTIVITY_TIMEOUT / 60);
                            websocket_protocol_exception(wsc, WS_CLOSE_GOING_AWAY, "Timeout - no activity");
                        }
                    }
                    // For normal clients, send periodic pings (every WS_PERIODIC_PING_INTERVAL seconds)
                    else if(now - wsc->last_activity_t > WS_PERIODIC_PING_INTERVAL) {
                        worker_is_busy(WORKERS_WEBSOCKET_SEND_PING);
                        websocket_protocol_send_ping(wsc, NULL, 0);
                    }
                }
                else if(wsc->state == WS_STATE_CLOSING_SERVER || wsc->state == WS_STATE_CLOSING_CLIENT) {
                    // If a client is in any CLOSING state for more than WS_CLOSING_STATE_TIMEOUT seconds, force close it
                    if(now - wsc->last_activity_t > WS_CLOSING_STATE_TIMEOUT) {
                        worker_is_busy(WORKERS_WEBSOCKET_CLIENT_STUCK);
                        websocket_error(wsc, "Forcing close (stuck in %s state)",
                                        wsc->state == WS_STATE_CLOSING_SERVER ? "CLOSING_SERVER" : "CLOSING_CLIENT");
                        websocket_thread_send_command(wth, WEBSOCKET_THREAD_CMD_REMOVE_CLIENT, wsc->id);
                    }
                }

                wsc = next;
            }

            spinlock_unlock(&wth->clients_spinlock);

            last_cleanup = now;
        }
    }

    netdata_log_info("WEBSOCKET[%zu] exiting", wth->id);

    // Clean up any remaining clients
    spinlock_lock(&wth->clients_spinlock);

    // Close all clients in this thread
    WS_CLIENT *wsc = wth->clients;

    // SHUTDOWN MODE: Fast cleanup with timeouts to avoid blocking
    usec_t cleanup_start = now_monotonic_usec();
    usec_t max_cleanup_time = 5 * USEC_PER_SEC;
    size_t clients_closed = 0, clients_skipped = 0;

    while(wsc) {
        WS_CLIENT *next = wsc->next;

        // Cache elapsed time to avoid repeated syscalls
        usec_t elapsed = now_monotonic_usec() - cleanup_start;

        if(elapsed < max_cleanup_time && wsc->sock.fd >= 0) {
            bool skip_client = false;

            // Ensure socket is non-blocking
            int flags = fcntl(wsc->sock.fd, F_GETFL, 0);
            if(flags >= 0 && !(flags & O_NONBLOCK)) {
                if(fcntl(wsc->sock.fd, F_SETFL, flags | O_NONBLOCK) < 0) {
                    websocket_debug(wsc, "Failed to set O_NONBLOCK during shutdown: %s", strerror(errno));
                    skip_client = true;
                }
            }

            if(!skip_client) {
                // Set send timeout: 100ms max per client
                struct timeval timeout = { .tv_sec = 0, .tv_usec = 100000 };
                if(setsockopt(wsc->sock.fd, SOL_SOCKET, SO_SNDTIMEO, &timeout, sizeof(timeout)) < 0) {
                    websocket_debug(wsc, "Failed to set SO_SNDTIMEO during shutdown: %s (continuing)", strerror(errno));
                    // Non-critical, continue anyway
                }

                // Try to send close frame (won't block due to O_NONBLOCK + SO_SNDTIMEO)
                websocket_protocol_send_close(wsc, WS_CLOSE_GOING_AWAY, "Server shutting down");
                ssize_t written = websocket_write_data(wsc);
                if(written < 0) {
                    websocket_debug(wsc, "Failed to send close frame during shutdown");
                }
                clients_closed++;
            }
            else {
                clients_skipped++;
            }
        }
        else {
            clients_skipped++;
        }

        websocket_thread_remove_client(wth, wsc, true);
        wsc = next;
    }

    netdata_log_info("WEBSOCKET[%zu] shutdown cleanup complete: %zu clients closed gracefully, %zu skipped",
                     wth->id, clients_closed, clients_skipped);

    if(clients_skipped > 0 && clients_skipped > clients_closed) {
        nd_log_daemon(NDLP_WARNING, "WEBSOCKET[%zu] skipped more clients (%zu) than closed gracefully (%zu) - "
                            "possible network issues or timeout reached",
                            wth->id, clients_skipped, clients_closed);
    }

    // Reset thread's client list
    wth->clients = NULL;
    wth->clients_current = 0;

    spinlock_unlock(&wth->clients_spinlock);

    // Cleanup poll resources
    if(wth->ndpl) {
        nd_poll_destroy(wth->ndpl);
        wth->ndpl = NULL;
    }

    // Cleanup command pipe
    if(wth->cmd.pipe[PIPE_READ] != -1) {
        close(wth->cmd.pipe[PIPE_READ]);
        wth->cmd.pipe[PIPE_READ] = -1;
    }

    if(wth->cmd.pipe[PIPE_WRITE] != -1) {
        close(wth->cmd.pipe[PIPE_WRITE]);
        wth->cmd.pipe[PIPE_WRITE] = -1;
    }

    // Mark thread as not running
    spinlock_lock(&wth->spinlock);
    wth->running = false;
    spinlock_unlock(&wth->spinlock);
}
