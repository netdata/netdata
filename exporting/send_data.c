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

    errno = 0;

    // loop through to collect all data
    while (*sock != -1 && errno != EWOULDBLOCK) {
        buffer_need_bytes(response, 4096);

        ssize_t r;
        r = recv(*sock, &response->buffer[response->len], response->size - response->len, MSG_DONTWAIT);
        if (likely(r > 0)) {
            // we received some data
            response->len += r;
            stats->chart_received_bytes += r;
            stats->chart_receptions++;
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
void simple_connector_send_buffer(int *sock, int *failures, struct instance *instance)
{
    BUFFER *buffer = (BUFFER *)instance->buffer;
    size_t len = buffer_strlen(buffer);

    int flags = 0;
#ifdef MSG_NOSIGNAL
    flags += MSG_NOSIGNAL;
#endif

    struct stats *stats = &instance->stats;

    int ret = 0;
    if (instance->send_header)
        ret = instance->send_header(sock, instance);

    ssize_t written = -1;

    if (!ret)
        written = send(*sock, buffer_tostring(buffer), len, flags);

    if(written != -1 && (size_t)written == len) {
        // we sent the data successfully
        stats->chart_transmission_successes++;
        stats->chart_sent_bytes += written;
        stats->chart_sent_metrics = stats->chart_buffered_metrics;

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
            len,
            written);
        stats->chart_transmission_failures++;

        if(written != -1)
            stats->chart_sent_bytes += written;

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

    struct simple_connector_config *connector_specific_config = instance->config.connector_specific_config;
    struct stats *stats = &instance->stats;

    int sock = -1;
    struct timeval timeout = {.tv_sec = (instance->config.timeoutms * 1000) / 1000000,
                              .tv_usec = (instance->config.timeoutms * 1000) % 1000000};
    int failures = 0;

    while(!netdata_exit) {
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
            stats->chart_reconnects += reconnects;
        }

        if(unlikely(netdata_exit)) break;

        // ------------------------------------------------------------------------
        // if we are connected, send our buffer to the data collecting server

        uv_mutex_lock(&instance->mutex);
        uv_cond_wait(&instance->cond_var, &instance->mutex);

        if (likely(sock != -1)) {
            simple_connector_send_buffer(&sock, &failures, instance);
        } else {
            error("EXPORTING: failed to update '%s'", instance->config.destination);
            stats->chart_transmission_failures++;

            // increment the counter we check for data loss
            failures++;
        }

        uv_mutex_unlock(&instance->mutex);

#ifdef UNIT_TESTING
        break;
#endif
    }
}
