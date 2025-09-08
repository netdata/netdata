// SPDX-License-Identifier: GPL-3.0-or-later

#include "exporting_engine.h"

/**
 * Check if TLS is enabled in the configuration
 *
 * @param type buffer with response data.
 * @param options an instance data structure.
 * @return Returns 1 if TLS should be enabled, 0 otherwise.
 */
static int exporting_tls_is_enabled(EXPORTING_CONNECTOR_TYPE type __maybe_unused, EXPORTING_OPTIONS options __maybe_unused)
{

    return (type == EXPORTING_CONNECTOR_TYPE_GRAPHITE_HTTP ||
            type == EXPORTING_CONNECTOR_TYPE_JSON_HTTP ||
            type == EXPORTING_CONNECTOR_TYPE_OPENTSDB_HTTP ||
            type == EXPORTING_CONNECTOR_TYPE_PROMETHEUS_REMOTE_WRITE) &&
           options & EXPORTING_OPTION_USE_TLS;
}

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
#ifdef NETDATA_INTERNAL_CHECKS
    char sample[1024];
    const char *s = buffer_tostring(buffer);
    char *d = sample, *e = &sample[sizeof(sample) - 1];

    for(; *s && d < e ;s++) {
        char c = *s;
        if(unlikely(!isprint(c))) c = ' ';
        *d++ = c;
    }
    *d = '\0';

    netdata_log_debug(D_EXPORTING,
                      "EXPORTING: received %zu bytes from %s connector instance. Ignoring them. Sample: '%s'",
                      buffer_strlen(buffer),
                      instance->config.name,
                      sample);
#else
    UNUSED(instance);
#endif /* NETDATA_INTERNAL_CHECKS */

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
        response = buffer_create(4096, &netdata_buffers_statistics.buffers_exporters);

    struct stats *stats = &instance->stats;
    uint32_t options = (uint32_t)instance->config.options;
    struct simple_connector_data *connector_specific_data = instance->connector_specific_data;

    if (options & EXPORTING_OPTION_USE_TLS)
        ERR_clear_error();

    errno_clear();

    // loop through to collect all data
    while (*sock != -1 && errno != EWOULDBLOCK) {
        ssize_t r;
        if (SSL_connection(&connector_specific_data->ssl))
            r = netdata_ssl_read(&connector_specific_data->ssl, &response->buffer[response->len],
                                 (int) (response->size - response->len));
        else
            r = recv(*sock, &response->buffer[response->len], response->size - response->len, MSG_DONTWAIT);

        if (likely(r > 0)) {
            // we received some data
            response->len += r;
            stats->received_bytes += r;
            stats->receptions++;
        }
        else if (r == 0) {
            netdata_log_error("EXPORTING: '%s' closed the socket", instance->config.destination);
            close(*sock);
            *sock = -1;
        }
        else {
            // failed to receive data
            if (errno != EAGAIN && errno != EWOULDBLOCK) {
                netdata_log_error("EXPORTING: cannot receive data from '%s'.", instance->config.destination);
                close(*sock);
                *sock = -1;
            }
        }

#ifdef UNIT_TESTING
        break;
#endif
    }

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

    // Safety check to prevent NULL pointer crashes, but don't allocate new memory
    if (unlikely(!buffer || !header)) {
        netdata_log_error("EXPORTING: NULL %s passed to simple_connector_send_buffer for instance %s", 
                          (!buffer && !header) ? "buffer and header" : (!buffer ? "buffer" : "header"),
                          instance->config.name ? instance->config.name : "unknown");
        (*failures)++;
        return;
    }

    uint32_t options = (uint32_t)instance->config.options;
    struct simple_connector_data *connector_specific_data = instance->connector_specific_data;

    if (options & EXPORTING_OPTION_USE_TLS)
        ERR_clear_error();

    struct stats *stats = &instance->stats;
    ssize_t header_sent_bytes = 0;
    ssize_t buffer_sent_bytes = 0;
    size_t header_len = buffer_strlen(header);
    size_t buffer_len = buffer_strlen(buffer);

    if (SSL_connection(&connector_specific_data->ssl)) {

        if (header_len)
            header_sent_bytes = netdata_ssl_write(&connector_specific_data->ssl, buffer_tostring(header), header_len);

        if ((size_t)header_sent_bytes == header_len)
            buffer_sent_bytes = netdata_ssl_write(&connector_specific_data->ssl, buffer_tostring(buffer), buffer_len);

    }
    else {
        if (header_len)
            header_sent_bytes = send(*sock, buffer_tostring(header), header_len, flags);
        if ((size_t)header_sent_bytes == header_len)
            buffer_sent_bytes = send(*sock, buffer_tostring(buffer), buffer_len, flags);
    }

    if ((size_t)buffer_sent_bytes == buffer_len) {
        // we sent the data successfully
        stats->transmission_successes++;
        stats->sent_metrics += buffered_metrics;
        stats->sent_bytes += buffer_sent_bytes;

        // reset the failures count
        *failures = 0;

        // empty the buffer
        buffer_flush(buffer);
    } else {
        // oops! we couldn't send (all or some of the) data
        netdata_log_error(
            "EXPORTING: failed to write data to '%s'. Willing to write %zu bytes, wrote %zd bytes. Will re-connect.",
            instance->config.destination,
            buffer_len,
            buffer_sent_bytes);
        stats->transmission_failures++;

        if(buffer_sent_bytes != -1)
            stats->sent_bytes += buffer_sent_bytes;

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
    struct simple_connector_data *connector_specific_data = instance->connector_specific_data;

    // Thread name is set during creation

    uint32_t options = (uint32_t)instance->config.options;

    if (options & EXPORTING_OPTION_USE_TLS)
        ERR_clear_error();

    struct simple_connector_config *connector_specific_config = instance->config.connector_specific_config;

    int sock = -1;
    struct timeval timeout = { .tv_sec = (instance->config.timeoutms * 1000) / 1000000,
                               .tv_usec = (instance->config.timeoutms * 1000) % 1000000 };
    int failures = 0;

    while (!instance->engine->exit) {
        struct stats *stats = &instance->stats;
        int send_stats = 0;

        if (instance->data_is_ready)
            send_stats = 1;

        netdata_mutex_lock(&instance->mutex);
        if (!connector_specific_data->first_buffer->used || failures) {
            while (!instance->data_is_ready)
                netdata_cond_wait(&instance->cond_var, &instance->mutex);
            instance->data_is_ready = 0;
            send_stats = 1;
        }

        if (unlikely(instance->engine->exit)) {
            netdata_mutex_unlock(&instance->mutex);
            break;
        }

        // ------------------------------------------------------------------------
        // detach buffer

        size_t buffered_metrics;

        if (!connector_specific_data->previous_buffer ||
            (connector_specific_data->previous_buffer == connector_specific_data->first_buffer &&
             connector_specific_data->first_buffer->used == 1)) {
            BUFFER *header, *buffer;

            header = connector_specific_data->first_buffer->header;
            buffer = connector_specific_data->first_buffer->buffer;
            connector_specific_data->buffered_metrics = connector_specific_data->first_buffer->buffered_metrics;
            connector_specific_data->buffered_bytes = connector_specific_data->first_buffer->buffered_bytes;

            buffered_metrics = connector_specific_data->buffered_metrics;

            buffer_flush(connector_specific_data->header);
            connector_specific_data->first_buffer->header = connector_specific_data->header;
            connector_specific_data->header = header;

            buffer_flush(connector_specific_data->buffer);
            connector_specific_data->first_buffer->buffer = connector_specific_data->buffer;
            connector_specific_data->buffer = buffer;
        } else {
            buffered_metrics = connector_specific_data->buffered_metrics;
        }

        netdata_mutex_unlock(&instance->mutex);

        // ------------------------------------------------------------------------
        // if we are connected, receive a response, without blocking

        if (likely(sock != -1))
            simple_connector_receive_response(&sock, instance);

        // ------------------------------------------------------------------------
        // if we are not connected, connect to a data collecting server

        if (unlikely(sock == -1)) {
            size_t reconnects = 0;

            sock = connect_to_one_of_urls(
                instance->config.destination,
                connector_specific_config->default_port,
                &timeout,
                &reconnects,
                connector_specific_data->connected_to,
                CONNECTED_TO_MAX);

            if (exporting_tls_is_enabled(instance->config.type, options) && sock != -1) {
                if (netdata_ssl_exporting_ctx) {
                    if (sock_setnonblock(sock, false) != 0)
                        netdata_log_error("Exporting cannot remove the non-blocking flag from socket %d", sock);

                    if(netdata_ssl_open(&connector_specific_data->ssl, netdata_ssl_exporting_ctx, sock)) {
                        if(netdata_ssl_connect(&connector_specific_data->ssl)) {
                            netdata_log_info("Exporting established a SSL connection.");

                            struct timeval tv;
                            tv.tv_sec = timeout.tv_sec / 4;
                            tv.tv_usec = 0;

                            if (!tv.tv_sec)
                                tv.tv_sec = 2;

                            if (setsockopt(sock, SOL_SOCKET, SO_RCVTIMEO, (const char *)&tv, sizeof(tv)))
                                netdata_log_error("Cannot set timeout to socket %d, this can block communication", sock);
                        }
                    }
                }
            }

            stats->reconnects += reconnects;
        }

        if (unlikely(instance->engine->exit))
            break;

        // ------------------------------------------------------------------------
        // if we are connected, send our buffer to the data collecting server

        failures = 0;

        if (likely(sock != -1)) {
            simple_connector_send_buffer(
                &sock,
                &failures,
                instance,
                connector_specific_data->header,
                connector_specific_data->buffer,
                buffered_metrics);
        } else {
            netdata_log_error("EXPORTING: failed to update '%s'", instance->config.destination);
            stats->transmission_failures++;

            // increment the counter we check for data loss
            failures++;
        }

        if (!failures) {
            connector_specific_data->first_buffer->buffered_metrics =
                connector_specific_data->first_buffer->buffered_bytes = connector_specific_data->first_buffer->used = 0;
            connector_specific_data->first_buffer = connector_specific_data->first_buffer->next;
        }

        if (unlikely(instance->engine->exit))
            break;

        if (send_stats) {
            netdata_mutex_lock(&instance->mutex);

            stats->buffered_metrics = connector_specific_data->total_buffered_metrics;

            send_internal_metrics(instance);

            stats->buffered_metrics = 0;

            // reset the internal monitoring chart counters
            connector_specific_data->total_buffered_metrics =
            stats->buffered_bytes =
            stats->receptions =
            stats->received_bytes =
            stats->sent_metrics =
            stats->sent_bytes =
            stats->transmission_successes =
            stats->transmission_failures =
            stats->reconnects =
            stats->data_lost_events =
            stats->lost_metrics =
            stats->lost_bytes = 0;

            netdata_mutex_unlock(&instance->mutex);
        }

#ifdef UNIT_TESTING
        return;
#endif
    }

#ifdef ENABLE_PROMETHEUS_REMOTE_WRITE
    if (instance->config.type == EXPORTING_CONNECTOR_TYPE_PROMETHEUS_REMOTE_WRITE)
        clean_prometheus_remote_write(instance);
#endif

    simple_connector_cleanup(instance);
}
