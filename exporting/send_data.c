// SPDX-License-Identifier: GPL-3.0-or-later

#include "exporting_engine.h"

/**
 * Discard response
 *
 * Discards a response received by an exporting connector instance after logging a sample of it to error.log
 *
 * @param buffer buffer with response data.
 * @param instance an instance data structure.
 * @return Always returns 0.
 */
int exporting_discard_response(BUFFER *buffer, struct instance *instance) {
    char sample[1024];
    const char *s = buffer_tostring(buffer);
    char *d = sample, *e = &sample[sizeof(sample) - 1];

    for(; *s && d < e ;s++) {
        char c = *s;
        if(unlikely(!isprint(c))) c = ' ';
        *d++ = c;
    }
    *d = '\0';

    info(
        "EXPORTING: received %zu bytes from %s connector instance. Ignoring them. Sample: '%s'",
        buffer_strlen(buffer),
        instance->config.name,
        sample);
    buffer_flush(buffer);
    return 0;
}

/**
 * Receive response
 *
 * @param sock communication socket.
 * @param instance an instance data structure.
 */
void simple_connector_receive_response(int *sock, struct instance *instance)
{
    static BUFFER *response = NULL;
    if (!response)
        response = buffer_create(1);

    struct stats *stats = &instance->stats;
#ifdef ENABLE_HTTPS
    uint32_t options = (uint32_t)instance->config.options;
    struct simple_connector_data *connector_specific_data = instance->connector_specific_data;

    if (options & EXPORTING_OPTION_USE_TLS)
        ERR_clear_error();
#endif

    errno = 0;

    // loop through to collect all data
    while (*sock != -1 && errno != EWOULDBLOCK) {
        buffer_need_bytes(response, 4096);

        ssize_t r;
#ifdef ENABLE_HTTPS
        if (instance->config.type == EXPORTING_CONNECTOR_TYPE_OPENTSDB_USING_HTTP &&
            options & EXPORTING_OPTION_USE_TLS &&
            connector_specific_data->conn &&
            connector_specific_data->flags == NETDATA_SSL_HANDSHAKE_COMPLETE) {
            r = (ssize_t)SSL_read(connector_specific_data->conn,
                                  &response->buffer[response->len],
                                  (int) (response->size - response->len));

            if (likely(r > 0)) {
                // we received some data
                response->len += r;
                stats->received_bytes += r;
                stats->receptions++;
                continue;
            } else {
                int sslerrno = SSL_get_error(connector_specific_data->conn, (int) r);
                u_long sslerr = ERR_get_error();
                char buf[256];
                switch (sslerrno) {
                    case SSL_ERROR_WANT_READ:
                    case SSL_ERROR_WANT_WRITE:
                        goto endloop;
                    default:
                        ERR_error_string_n(sslerr, buf, sizeof(buf));
                        error("SSL error (%s)",
                              ERR_error_string((long)SSL_get_error(connector_specific_data->conn, (int)r), NULL));
                        goto endloop;
                }
            }
        } else {
            r = recv(*sock, &response->buffer[response->len], response->size - response->len, MSG_DONTWAIT);
        }
#else
        r = recv(*sock, &response->buffer[response->len], response->size - response->len, MSG_DONTWAIT);
#endif
        if (likely(r > 0)) {
            // we received some data
            response->len += r;
            stats->received_bytes += r;
            stats->receptions++;
        } else if (r == 0) {
            error("EXPORTING: '%s' closed the socket", instance->config.destination);
            close(*sock);
            *sock = -1;
        } else {
            // failed to receive data
            if (errno != EAGAIN && errno != EWOULDBLOCK) {
                error("EXPORTING: cannot receive data from '%s'.", instance->config.destination);
            }
        }

#ifdef UNIT_TESTING
        break;
#endif
    }
#ifdef ENABLE_HTTPS
endloop:
#endif

    // if we received data, process them
    if (buffer_strlen(response))
        instance->check_response(response, instance);
}

/**
 * Send buffer to a server
 *
 * @param sock communication socket.
 * @param failures the number of communication failures.
 * @param instance an instance data structure.
 */
void simple_connector_send_buffer(
    int *sock, int *failures, struct instance *instance, BUFFER *header, BUFFER *buffer, size_t buffered_metrics)
{
    int flags = 0;
#ifdef MSG_NOSIGNAL
    flags += MSG_NOSIGNAL;
#endif

#ifdef ENABLE_HTTPS
    uint32_t options = (uint32_t)instance->config.options;
    struct simple_connector_data *connector_specific_data = instance->connector_specific_data;

    if (options & EXPORTING_OPTION_USE_TLS)
        ERR_clear_error();
#endif

    struct stats *stats = &instance->stats;
    ssize_t written = -1;
    size_t header_len = buffer_strlen(header);
    size_t buffer_len = buffer_strlen(buffer);

#ifdef ENABLE_HTTPS
    if (instance->config.type == EXPORTING_CONNECTOR_TYPE_OPENTSDB_USING_HTTP &&
        options & EXPORTING_OPTION_USE_TLS &&
        connector_specific_data->conn &&
        connector_specific_data->flags == NETDATA_SSL_HANDSHAKE_COMPLETE) {
        written = (ssize_t)SSL_write(connector_specific_data->conn, buffer_tostring(header), header_len);
        written += (ssize_t)SSL_write(connector_specific_data->conn, buffer_tostring(buffer), buffer_len);
    } else {
        written = send(*sock, buffer_tostring(header), header_len, flags);
        written += send(*sock, buffer_tostring(buffer), buffer_len, flags);
    }
#else
    written = send(*sock, buffer_tostring(header), header_len, flags);
    written += send(*sock, buffer_tostring(buffer), buffer_len, flags);
#endif

    if(written != -1 && (size_t)written == (header_len + buffer_len)) {
        // we sent the data successfully
        stats->transmission_successes++;
        stats->sent_bytes += written;
        stats->sent_metrics = buffered_metrics;

        // reset the failures count
        *failures = 0;

        // empty the buffer
        buffer_flush(buffer);
    }
    else {
        // oops! we couldn't send (all or some of the) data
        error(
            "EXPORTING: failed to write data to '%s'. Willing to write %zu bytes, wrote %zd bytes. Will re-connect.",
            instance->config.destination,
            buffer_len,
            written);
        stats->transmission_failures++;

        if(written != -1)
            stats->sent_bytes += written;

        // increment the counter we check for data loss
        (*failures)++;

        // close the socket - we will re-open it next time
        close(*sock);
        *sock = -1;
    }
}

/**
 * Simple connector worker
 *
 * Runs in a separate thread for every instance.
 *
 * @param instance_p an instance data structure.
 */
void simple_connector_worker(void *instance_p)
{
    struct instance *instance = (struct instance*)instance_p;

#ifdef ENABLE_HTTPS
    uint32_t options = (uint32_t)instance->config.options;
    struct simple_connector_data *connector_specific_data = instance->connector_specific_data;

    if (options & EXPORTING_OPTION_USE_TLS)
        ERR_clear_error();
#endif
    struct simple_connector_config *connector_specific_config = instance->config.connector_specific_config;

    int sock = -1;
    struct timeval timeout = {.tv_sec = (instance->config.timeoutms * 1000) / 1000000,
                              .tv_usec = (instance->config.timeoutms * 1000) % 1000000};
    int failures = 0;

    BUFFER *empty_header = buffer_create(0);
    BUFFER *empty_buffer = buffer_create(0);

    while(!instance->engine->exit) {
        struct stats *stats = &instance->stats;

        uv_mutex_lock(&instance->mutex);
        if (!connector_specific_data->first_buffer->buffer ||
            !buffer_strlen(connector_specific_data->first_buffer->buffer)) {
            while (!instance->data_is_ready)
                uv_cond_wait(&instance->cond_var, &instance->mutex);
            instance->data_is_ready = 0;
        }

        if (unlikely(instance->engine->exit)) {
            uv_mutex_unlock(&instance->mutex);
            break;
        }

        // reset the monitoring chart counters
        stats->received_bytes =
        stats->sent_bytes =
        stats->sent_metrics =
        stats->lost_metrics =
        stats->receptions =
        stats->transmission_successes =
        stats->transmission_failures =
        stats->data_lost_events =
        stats->lost_bytes =
        stats->reconnects = 0;

        BUFFER *header = connector_specific_data->first_buffer->header;
        BUFFER *buffer = connector_specific_data->first_buffer->buffer;
        size_t buffered_metrics = connector_specific_data->first_buffer->buffered_metrics;
        size_t buffered_bytes = buffer_strlen(buffer);

        connector_specific_data->first_buffer->header = empty_header;
        empty_header = header;
        connector_specific_data->first_buffer->buffer = empty_buffer;
        empty_buffer = buffer;
        connector_specific_data->first_buffer->buffered_metrics = 0;
        connector_specific_data->first_buffer = connector_specific_data->first_buffer->next;

        uv_mutex_unlock(&instance->mutex);

        // ------------------------------------------------------------------------
        // if we are connected, receive a response, without blocking

        if(likely(sock != -1))
            simple_connector_receive_response(&sock, instance);

        // ------------------------------------------------------------------------
        // if we are not connected, connect to a data collecting server

        if(unlikely(sock == -1)) {
            size_t reconnects = 0;

            sock = connect_to_one_of(
                instance->config.destination,
                connector_specific_config->default_port,
                &timeout,
                &reconnects,
                NULL,
                0);
#ifdef ENABLE_HTTPS
            if(instance->config.type == EXPORTING_CONNECTOR_TYPE_OPENTSDB_USING_HTTP && sock != -1) {
                if (netdata_exporting_ctx) {
                    if ( sock_delnonblock(sock) < 0 )
                        error("Exporting cannot remove the non-blocking flag from socket %d", sock);

                    if (connector_specific_data->conn == NULL) {
                        connector_specific_data->conn = SSL_new(netdata_exporting_ctx);
                        if (connector_specific_data->conn == NULL) {
                            error("Failed to allocate SSL structure to socket %d.", sock);
                            connector_specific_data->flags = NETDATA_SSL_NO_HANDSHAKE;
                        }
                    } else {
                        SSL_clear(connector_specific_data->conn);
                    }

                    if (connector_specific_data->conn) {
                        if (SSL_set_fd(connector_specific_data->conn, sock) != 1) {
                            error("Failed to set the socket to the SSL on socket fd %d.", sock);
                            connector_specific_data->flags = NETDATA_SSL_NO_HANDSHAKE;
                        } else {
                            connector_specific_data->flags = NETDATA_SSL_HANDSHAKE_COMPLETE;
                            SSL_set_connect_state(connector_specific_data->conn);
                            int err = SSL_connect(connector_specific_data->conn);
                            if (err != 1) {
                                err = SSL_get_error(connector_specific_data->conn, err);
                                error("SSL cannot connect with the server:  %s ",
                                      ERR_error_string((long)SSL_get_error(connector_specific_data->conn, err), NULL));
                                connector_specific_data->flags = NETDATA_SSL_NO_HANDSHAKE;
                            } else {
                                info("Exporting established a SSL connection.");

                                struct timeval tv;
                                tv.tv_sec = timeout.tv_sec /4;
                                tv.tv_usec = 0;

                                if (!tv.tv_sec)
                                    tv.tv_sec = 2;

                                if (setsockopt(sock, SOL_SOCKET, SO_RCVTIMEO, (const char*)&tv, sizeof(tv)))
                                    error("Cannot set timeout to socket %d, this can block communication", sock);

                            }
                        }
                    }
                }

            }
#endif

            stats->reconnects += reconnects;
        }

        if(unlikely(instance->engine->exit)) break;

        // ------------------------------------------------------------------------
        // if we are connected, send our buffer to the data collecting server

        if (likely(sock != -1)) {
            simple_connector_send_buffer(&sock, &failures, instance, header, buffer, buffered_metrics);
        } else {
            error("EXPORTING: failed to update '%s'", instance->config.destination);
            stats->transmission_failures++;

            // increment the counter we check for data loss
            failures++;
        }

        if (failures > instance->config.buffer_on_failures) {
            stats->lost_bytes += buffer_strlen(buffer);
            error(
                "EXPORTING: connector instance %s reached %d exporting failures. "
                "Flushing buffers to protect this host - this results in data loss on server '%s'",
                instance->config.name, failures, instance->config.destination);
            buffer_flush(buffer);
            failures = 0;
            stats->data_lost_events++;
            stats->lost_metrics = buffered_metrics;
        }

        if (unlikely(instance->engine->exit))
            break;

        uv_mutex_lock(&instance->mutex);

        stats->buffered_metrics = buffered_metrics;

        send_internal_metrics(instance);

        connector_specific_data->total_buffered_metrics -= buffered_metrics;

        stats->buffered_metrics = 0;
        stats->buffered_bytes -= buffered_bytes;

        uv_mutex_unlock(&instance->mutex);

#ifdef UNIT_TESTING
        return;
#endif
    }

#if ENABLE_PROMETHEUS_REMOTE_WRITE
    if (instance->config.type == EXPORTING_CONNECTOR_TYPE_PROMETHEUS_REMOTE_WRITE)
        clean_prometheus_remote_write(instance);
#endif

    simple_connector_cleanup(instance);
}
