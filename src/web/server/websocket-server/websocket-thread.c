// SPDX-License-Identifier: GPL-3.0-or-later

#include "daemon/common.h"
#include "websocket-internal.h"
#include "poll.h"

// Global array of WebSocket threads
WEBSOCKET_THREAD websocket_threads[WEBSOCKET_MAX_THREADS];

// Initialize WebSocket thread system
void websocket_threads_init(void) {
    for(size_t i = 0; i < WEBSOCKET_MAX_THREADS; i++) {
        websocket_threads[i].id = i;
        websocket_threads[i].thread = NULL;
        websocket_threads[i].running = false;
        spinlock_init(&websocket_threads[i].spinlock);
        websocket_threads[i].clients_current = 0;
        spinlock_init(&websocket_threads[i].clients_spinlock);
        websocket_threads[i].clients = NULL;
        websocket_threads[i].ndpl = NULL;
        websocket_threads[i].cmd.pipe[PIPE_READ] = -1;
        websocket_threads[i].cmd.pipe[PIPE_WRITE] = -1;
    }
}

static void websocket_thread_remove_client(WEBSOCKET_THREAD *wth, WS_CLIENT *wsc) {
    // First remove from the poll to prevent further events
    websocket_thread_remove_client_from_poll(wth, wsc);

    // send a close frame (it won't do it if not allowed by the protocol)
    websocket_protocol_send_close(wsc, WS_CLOSE_NORMAL, "Connection closed by server");

    // If already in a closing state, just flush any pending data
    websocket_write_data(wsc);

    websocket_debug(wsc, "Removed and resources freed", wth->id, wsc->id);
    websocket_client_free(wsc);
}

// Find the thread with the minimum client load and atomically increment its count
NEVERNULL
static WEBSOCKET_THREAD *websocket_thread_get_min_load(void) {
    // Static spinlock to protect the critical section of thread selection
    static SPINLOCK assign_spinlock = SPINLOCK_INITIALIZER;
    size_t slot = 0;
    
    // Critical section: find thread with minimum load and increment its count atomically
    spinlock_lock(&assign_spinlock);
    
    // Find the minimum load thread
    size_t min_clients = websocket_threads[0].clients_current;
    
    for(size_t i = 1; i < WEBSOCKET_MAX_THREADS; i++) {
        // Check if this thread has fewer clients
        if(websocket_threads[i].clients_current < min_clients) {
            min_clients = websocket_threads[i].clients_current;
            slot = i;
        }
    }
    
    // Preemptively increment the client count to prevent race conditions
    // This ensures concurrent client assignments will be properly distributed
    websocket_threads[slot].clients_current++;
    
    spinlock_unlock(&assign_spinlock);
    
    return &websocket_threads[slot];
}

// Initialize a thread's poll
static bool websocket_thread_init_poll(WEBSOCKET_THREAD *wth) {
    if(!wth) return false;

    // Create poll instance
    if(!wth->ndpl) {
        wth->ndpl = nd_poll_create();
        if (!wth->ndpl) {
            netdata_log_error("WEBSOCKET[%zu]: Failed to create poll", wth->id);
            goto cleanup;
        }
    }

    // Create command pipe
    if(wth->cmd.pipe[PIPE_READ] == -1 || wth->cmd.pipe[PIPE_WRITE] == -1) {
        if (pipe(wth->cmd.pipe) == -1) {
            netdata_log_error("WEBSOCKET[%zu]: Failed to create command pipe: %s", wth->id, strerror(errno));
            goto cleanup;
        }

        // Set pipe to non-blocking
        if(fcntl(wth->cmd.pipe[PIPE_READ], F_SETFL, O_NONBLOCK) == -1) {
            netdata_log_error("WEBSOCKET[%zu]: Failed to set command pipe to non-blocking: %s", wth->id, strerror(errno));
            goto cleanup;
        }

        // Add command pipe to poll
        bool added = nd_poll_add(wth->ndpl, wth->cmd.pipe[PIPE_READ], ND_POLL_READ, &wth->cmd);
        if(!added) {
            netdata_log_error("WEBSOCKET[%zu]: Failed to add command pipe to poll", wth->id);
            goto cleanup;
        }
    }

    return true;

cleanup:
    if(wth->cmd.pipe[PIPE_READ] != -1) {
        close(wth->cmd.pipe[PIPE_READ]);
        wth->cmd.pipe[PIPE_READ] = -1;
    }
    if(wth->cmd.pipe[PIPE_WRITE] != -1) {
        close(wth->cmd.pipe[PIPE_WRITE]);
        wth->cmd.pipe[PIPE_WRITE] = -1;
    }
    if(wth->ndpl) {
        nd_poll_destroy(wth->ndpl);
        wth->ndpl = NULL;
    }
    return false;
}

static void websocket_thread_client_socket_error(WEBSOCKET_THREAD *wth, WS_CLIENT *wsc, const char *reason) {
    worker_is_busy(WORKERS_WEBSOCKET_SOCK_ERROR);

    websocket_debug(wsc, reason);

    // Handle the disconnection more gracefully
    if(wsc->state != WS_STATE_CLOSED) {
        // Call the close handler if registered
        if(wsc->on_close) {
            wsc->on_close(wsc, WS_CLOSE_GOING_AWAY, reason);
        }

        // Update client state
        wsc->state = WS_STATE_CLOSED;
    }

    // Send command to remove the client
    websocket_thread_send_command(wth, WEBSOCKET_THREAD_CMD_REMOVE_CLIENT, &wsc->id, sizeof(wsc->id));
}

// Thread main function
void *websocket_thread(void *ptr) {
    WEBSOCKET_THREAD *wth = (WEBSOCKET_THREAD *)ptr;
    
    if(!wth) {
        netdata_log_error("WEBSOCKET: Thread started with NULL argument");
        return NULL;
    }

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
                    websocket_thread_remove_client(wth, wsc);
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
                    // Check if client is idle (no activity for over 120 seconds)
                    if(now - wsc->last_activity_t > 120) {
                        // Client is idle - send a ping to check if it's still alive
                        worker_is_busy(WORKERS_WEBSOCKET_SEND_PING);
                        websocket_protocol_send_ping(wsc, NULL, 0);
                        
                        // If no activity for over 300 seconds (5 minutes), consider it dead
                        if(now - wsc->last_activity_t > 300) {
                            worker_is_busy(WORKERS_WEBSOCKET_CLIENT_TIMEOUT);
                            websocket_error(wsc, "Client timed out (no activity for over 5 minutes)");
                            websocket_protocol_exception(wsc, WS_CLOSE_GOING_AWAY, "Timeout - no activity");
                        }
                    }
                    // For normal clients, send periodic pings (every 60 seconds)
                    else if(now - wsc->last_activity_t > 60) {
                        worker_is_busy(WORKERS_WEBSOCKET_SEND_PING);
                        websocket_protocol_send_ping(wsc, NULL, 0);
                    }
                }
                else if(wsc->state == WS_STATE_CLOSING_SERVER || wsc->state == WS_STATE_CLOSING_CLIENT) {
                    // If a client is in any CLOSING state for more than 5 seconds, force close it
                    if(now - wsc->last_activity_t > 5) {
                        worker_is_busy(WORKERS_WEBSOCKET_CLIENT_STUCK);
                        websocket_error(wsc, "Forcing close (stuck in %s state)",
                            wsc->state == WS_STATE_CLOSING_SERVER ? "CLOSING_SERVER" : "CLOSING_CLIENT");
                        websocket_thread_send_command(wth, WEBSOCKET_THREAD_CMD_REMOVE_CLIENT, &wsc->id, sizeof(wsc->id));
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
    while(wsc) {
        WS_CLIENT *next = wsc->next;
        
        netdata_log_info("WEBSOCKET[%zu]: Closing client %zu during thread shutdown", wth->id, wsc->id);
        
        // First remove from the poll to prevent further events
        if (wsc->sock.fd >= 0) {
            bool removed = nd_poll_del(wth->ndpl, wsc->sock.fd);
            if (!removed) {
                netdata_log_error("WEBSOCKET[%zu]: Failed to remove client %zu from poll during shutdown", wth->id, wsc->id);
            }
        }
        
        // If client is still connected and not in CLOSED state, send close frame
        if(wsc->sock.fd >= 0 && wsc->state != WS_STATE_CLOSED) {
            // Try to send close frame (ignoring errors)
            websocket_protocol_exception(wsc, WS_CLOSE_GOING_AWAY, "Server shutting down");
            websocket_write_data(wsc);
        }

        websocket_client_free(wsc);
        wsc = next;
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

    return NULL;
}

// Assign a client to a thread
WEBSOCKET_THREAD *websocket_thread_assign_client(WS_CLIENT *wsc) {
    if(!wsc)
        return NULL;

    // Get the thread with the minimum load 
    // Note: client count is already atomically incremented inside this function
    WEBSOCKET_THREAD *wth = websocket_thread_get_min_load();

    // Lock the thread for initialization
    spinlock_lock(&wth->spinlock);

    // Start the thread if not running
    if(!wth->thread) {
        // Initialize poll
        if(!websocket_thread_init_poll(wth)) {
            spinlock_unlock(&wth->spinlock);
            netdata_log_error("WEBSOCKET[%zu]: Failed to initialize poll", wth->id);
            goto undo;
        }

        char thread_name[32];
        snprintf(thread_name, sizeof(thread_name), "WEBSOCK[%zu]", wth->id);
        wth->thread = nd_thread_create(thread_name, NETDATA_THREAD_OPTION_DEFAULT, websocket_thread, wth);
        wth->running = true;
    }

    // Release the thread lock
    spinlock_unlock(&wth->spinlock);

    // Send command to add client
    if(!websocket_thread_send_command(wth, WEBSOCKET_THREAD_CMD_ADD_CLIENT, &wsc->id, sizeof(wsc->id))) {
        netdata_log_error("WEBSOCKET[%zu]: Failed to send add client command", wth->id);
        goto undo;
    }

    // Link thread to client
    wsc->wth = wth;

    return wth;

undo:
    // Roll back the client count increment since assignment failed
    if(wth) {
        spinlock_lock(&wth->clients_spinlock);
        if (wth->clients_current > 0)
            wth->clients_current--;
        spinlock_unlock(&wth->clients_spinlock);
    }

    return NULL;
}

// Add a client to a thread's poll
bool websocket_thread_add_client_to_poll(WEBSOCKET_THREAD *wth, WS_CLIENT *wsc) {
    if(!wth || !wsc || wsc->sock.fd < 0)
        return false;

    // Lock the thread poll
    spinlock_lock(&wth->spinlock);

    // Add client to the poll - use the socket fd directly
    bool added = nd_poll_add(wth->ndpl, wsc->sock.fd, ND_POLL_READ, wsc);
    if(!added) {
        netdata_log_error("WEBSOCKET[%zu]: Failed to add client %zu to poll", wth->id, wsc->id);
        spinlock_unlock(&wth->spinlock);
        return false;
    }

    // Release the thread poll lock
    spinlock_unlock(&wth->spinlock);

    // Add client to the thread's client list
    websocket_thread_enqueue_client(wth, wsc);

    return true;
}

// Remove a client from a thread's poll
void websocket_thread_remove_client_from_poll(WEBSOCKET_THREAD *wth, WS_CLIENT *wsc) {
    if(!wth || !wsc || wsc->sock.fd < 0)
        return;

    // Lock the thread poll
    spinlock_lock(&wth->spinlock);

    // Remove client from the poll - use socket fd directly
    bool removed = nd_poll_del(wth->ndpl, wsc->sock.fd);
    if(!removed) {
        netdata_log_error("WEBSOCKET[%zu]: Failed to remove client %zu from poll", wth->id, wsc->id);
    }

    // Release the thread poll lock
    spinlock_unlock(&wth->spinlock);

    // Remove client from the thread's client list
    websocket_thread_dequeue_client(wth, wsc);
}

// Thread client enqueue function
void websocket_thread_enqueue_client(WEBSOCKET_THREAD *wth, WS_CLIENT *wsc) {
    if(!wth || !wsc)
        return;

    // Lock the thread clients
    spinlock_lock(&wth->clients_spinlock);

    // Add client to the thread's client list using the double linked list macro
    DOUBLE_LINKED_LIST_APPEND_ITEM_UNSAFE(wth->clients, wsc, prev, next);

    // Note: clients_current is already incremented in websocket_thread_assign_client
    // to ensure proper load distribution in concurrent scenarios

    // Release the thread clients lock
    spinlock_unlock(&wth->clients_spinlock);
}

// Thread client dequeue function
void websocket_thread_dequeue_client(WEBSOCKET_THREAD *wth, WS_CLIENT *wsc) {
    if(!wth || !wsc)
        return;

    // Lock the thread clients
    spinlock_lock(&wth->clients_spinlock);

    // Remove client from the thread's client list using the double linked list macro
    DOUBLE_LINKED_LIST_REMOVE_ITEM_UNSAFE(wth->clients, wsc, prev, next);

    if(wth->clients_current > 0)
        wth->clients_current--;

    // Release the thread clients lock
    spinlock_unlock(&wth->clients_spinlock);
}

// Update a client's poll event flags
bool websocket_thread_update_client_poll_flags(WS_CLIENT *wsc) {
    if(!wsc || !wsc->wth || wsc->sock.fd < 0)
        return false;

    nd_poll_event_t events = wsc->flush_and_remove_client ? 0 : ND_POLL_READ;
    if(cbuffer_next_unsafe(&wsc->out_buffer, NULL) > 0)
        events |= ND_POLL_WRITE;

    // Update poll events
    bool updated = nd_poll_upd(wsc->wth->ndpl, wsc->sock.fd, events);
    if(!updated) {
        netdata_log_error("WEBSOCKET[%zu]: Failed to update poll events for client %zu", wsc->wth->id, wsc->id);
    }

    return updated;
}

// Send command to a thread
bool websocket_thread_send_command(WEBSOCKET_THREAD *wth, uint8_t cmd, void *data, size_t data_len) {
    if(!wth || wth->cmd.pipe[PIPE_WRITE] == -1) {
        netdata_log_error("WEBSOCKET[%zu]: Failed to send command - pipe is not initialized", wth->id);
        return false;
    }

    // Prepare command
    struct {
        uint8_t cmd;
        size_t len;
        char data[sizeof(size_t)]; // Variable size, just a placeholder
    } __attribute__((packed)) message;

    message.cmd = cmd;
    message.len = data_len;

    // Lock command pipe for writing
    spinlock_lock(&wth->spinlock);

    // Write command header
    ssize_t bytes = write(wth->cmd.pipe[PIPE_WRITE], &message, sizeof(message.cmd) + sizeof(message.len));
    if(bytes != sizeof(message.cmd) + sizeof(message.len)) {
        netdata_log_error("WEBSOCKET[%zu]: Failed to write command header", wth->id);
        spinlock_unlock(&wth->spinlock);
        return false;
    }

    // Write command data
    if(data && data_len > 0) {
        bytes = write(wth->cmd.pipe[PIPE_WRITE], data, data_len);
        if(bytes != (ssize_t)data_len) {
            netdata_log_error("WEBSOCKET[%zu]: Failed to write command data", wth->id);
            spinlock_unlock(&wth->spinlock);
            return false;
        }
    }

    // Release command pipe
    spinlock_unlock(&wth->spinlock);

    return true;
}

// Process a thread's command pipe
void websocket_thread_process_commands(WEBSOCKET_THREAD *wth) {
    if(!wth || wth->cmd.pipe[PIPE_READ] == -1)
        return;

    struct {
        uint8_t cmd;
        size_t len;
    } __attribute__((packed)) header;

    char buffer[1024]; // Buffer for command data

    // Read all available commands
    for(;;) {
        // Read command header

        worker_is_busy(WORKERS_WEBSOCKET_CMD_READ);

        ssize_t bytes = read(wth->cmd.pipe[PIPE_READ], &header, sizeof(header));
        if(bytes <= 0) {
            if(bytes < 0 && errno != EAGAIN && errno != EWOULDBLOCK) {
                netdata_log_error("WEBSOCKET[%zu]: Failed to read command header from pipe: %s", wth->id, strerror(errno));
            }
            break;
        }

        if(bytes != sizeof(header)) {
            netdata_log_error("WEBSOCKET[%zu]: Read partial command header (%zd/%zu bytes)", wth->id, bytes, sizeof(header));
            break;
        }

        // Read command data
        if(header.len > 0) {
            if(header.len > sizeof(buffer)) {
                netdata_log_error("WEBSOCKET[%zu]: Command data too large (%zu bytes)", wth->id, header.len);
                // Skip the data
                size_t to_skip = header.len;
                while(to_skip > 0) {
                    size_t chunk = to_skip > sizeof(buffer) ? sizeof(buffer) : to_skip;
                    bytes = read(wth->cmd.pipe[PIPE_READ], buffer, chunk);
                    if(bytes <= 0)
                        break;
                    to_skip -= bytes;
                }
                continue;
            }

            bytes = read(wth->cmd.pipe[PIPE_READ], buffer, header.len);
            if(bytes < 0) {
                if(errno != EAGAIN && errno != EWOULDBLOCK) {
                    netdata_log_error("WEBSOCKET[%zu]: Failed to read command data from pipe: %s", wth->id, strerror(errno));
                }
                break;
            }
            else if(bytes != (ssize_t)header.len) {
                netdata_log_error("WEBSOCKET[%zu]: Read partial command data (%zd/%zu bytes)", wth->id, bytes, header.len);
                break;
            }
        }

        // Process command
        switch(header.cmd) {
            case WEBSOCKET_THREAD_CMD_EXIT:
                worker_is_busy(WORKERS_WEBSOCKET_CMD_EXIT);
                netdata_log_info("WEBSOCKET[%zu] received exit command", wth->id);
                return;

            case WEBSOCKET_THREAD_CMD_ADD_CLIENT: {
                worker_is_busy(WORKERS_WEBSOCKET_CMD_ADD);
                if(header.len != sizeof(size_t)) {
                    netdata_log_error("WEBSOCKET[%zu]: Invalid add client command size (%zu != %zu)", wth->id, header.len, sizeof(size_t));
                    continue;
                }
                size_t client_id = *(size_t *)buffer;
                WS_CLIENT *wsc = websocket_client_find_by_id(client_id);
                if(!wsc) {
                    netdata_log_error("WEBSOCKET[%zu]: Client %zu not found for add command", wth->id, client_id);
                    continue;
                }
                websocket_thread_add_client_to_poll(wth, wsc);
                break;
            }

            case WEBSOCKET_THREAD_CMD_REMOVE_CLIENT: {
                worker_is_busy(WORKERS_WEBSOCKET_CMD_DEL);

                if(header.len != sizeof(size_t)) {
                    netdata_log_error("WEBSOCKET[%zu]: Invalid remove client command size (%zu != %zu)", wth->id, header.len, sizeof(size_t));
                    continue;
                }

                size_t client_id = *(size_t *)buffer;
                WS_CLIENT *wsc = websocket_client_find_by_id(client_id);
                if(!wsc) {
                    netdata_log_error("WEBSOCKET[%zu]: Client %zu not found for remove command", wth->id, client_id);
                    continue;
                }

                websocket_thread_remove_client(wth, wsc);
                break;
            }

            case WEBSOCKET_THREAD_CMD_BROADCAST: {
                worker_is_busy(WORKERS_WEBSOCKET_CMD_BROADCAST);

                if(header.len < sizeof(size_t) + sizeof(WEBSOCKET_OPCODE)) {
                    netdata_log_error("WEBSOCKET[%zu]: Invalid broadcast command size", wth->id);
                    continue;
                }
                
                // Extract broadcast parameters
                size_t offset = 0;
                
                // Extract message length
                size_t message_len;
                memcpy(&message_len, buffer + offset, sizeof(size_t));
                offset += sizeof(size_t);
                
                // Extract opcode
                WEBSOCKET_OPCODE opcode;
                memcpy(&opcode, buffer + offset, sizeof(WEBSOCKET_OPCODE));
                offset += sizeof(WEBSOCKET_OPCODE);
                
                // Ensure we have the complete message
                if(header.len != sizeof(size_t) + sizeof(WEBSOCKET_OPCODE) + message_len) {
                    netdata_log_error("WEBSOCKET[%zu]: Broadcast command size mismatch", wth->id);
                    continue;
                }
                
                // Extract message
                const char *message = buffer + offset;
                
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

// Cancel all WebSocket threads
void websocket_threads_join(void) {
    for(size_t i = 0; i < WEBSOCKET_MAX_THREADS; i++) {
        if(websocket_threads[i].thread) {
            // Send exit command
            websocket_thread_send_command(&websocket_threads[i], WEBSOCKET_THREAD_CMD_EXIT, NULL, 0);
            
            // Signal thread to cancel
            nd_thread_signal_cancel(websocket_threads[i].thread);
        }
    }

    // Wait for all threads to exit
    for(size_t i = 0; i < WEBSOCKET_MAX_THREADS; i++) {
        if(websocket_threads[i].thread) {
            nd_thread_join(websocket_threads[i].thread);
            websocket_threads[i].thread = NULL;
            websocket_threads[i].running = false;
        }
    }
}

// Get thread by ID
WEBSOCKET_THREAD *websocket_thread_by_id(size_t id) {
    if(id >= WEBSOCKET_MAX_THREADS)
        return NULL;
    return &websocket_threads[id];
}