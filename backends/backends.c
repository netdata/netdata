// SPDX-License-Identifier: GPL-3.0-or-later

#define BACKENDS_INTERNALS
#include "backends.h"

// ----------------------------------------------------------------------------
// How backends work in netdata:
//
// 1. There is an independent thread that runs at the required interval
//    (for example, once every 10 seconds)
//
// 2. Every time it wakes, it calls the backend formatting functions to build
//    a buffer of data. This is a very fast, memory only operation.
//
// 3. If the buffer already includes data, the new data are appended.
//    If the buffer becomes too big, because the data cannot be sent, a
//    log is written and the buffer is discarded.
//
// 4. Then it tries to send all the data. It blocks until all the data are sent
//    or the socket returns an error.
//    If the time required for this is above the interval, it starts skipping
//    intervals, but the calculated values include the entire database, without
//    gaps (it remembers the timestamps and continues from where it stopped).
//
// 5. repeats the above forever.
//

const char *global_backend_prefix = "netdata";
int global_backend_update_every = 10;
BACKEND_OPTIONS global_backend_options = BACKEND_SOURCE_DATA_AVERAGE | BACKEND_OPTION_SEND_NAMES;

// ----------------------------------------------------------------------------
// helper functions for backends

size_t backend_name_copy(char *d, const char *s, size_t usable) {
    size_t n;

    for(n = 0; *s && n < usable ; d++, s++, n++) {
        char c = *s;

        if(c != '.' && !isalnum(c)) *d = '_';
        else *d = c;
    }
    *d = '\0';

    return n;
}

// calculate the SUM or AVERAGE of a dimension, for any timeframe
// may return NAN if the database does not have any value in the give timeframe

calculated_number backend_calculate_value_from_stored_data(
          RRDSET *st                // the chart
        , RRDDIM *rd                // the dimension
        , time_t after              // the start timestamp
        , time_t before             // the end timestamp
        , BACKEND_OPTIONS backend_options // BACKEND_SOURCE_* bitmap
        , time_t *first_timestamp   // the first point of the database used in this response
        , time_t *last_timestamp    // the timestamp that should be reported to backend
) {
    RRDHOST *host = st->rrdhost;
    (void)host;

    // find the edges of the rrd database for this chart
    time_t first_t = rd->state->query_ops.oldest_time(rd);
    time_t last_t  = rd->state->query_ops.latest_time(rd);
    time_t update_every = st->update_every;
    struct rrddim_query_handle handle;
    storage_number n;

    // step back a little, to make sure we have complete data collection
    // for all metrics
    after  -= update_every * 2;
    before -= update_every * 2;

    // align the time-frame
    after  = after  - (after  % update_every);
    before = before - (before % update_every);

    // for before, loose another iteration
    // the latest point will be reported the next time
    before -= update_every;

    if(unlikely(after > before))
        // this can happen when update_every > before - after
        after = before;

    if(unlikely(after < first_t))
        after = first_t;

    if(unlikely(before > last_t))
        before = last_t;

    if(unlikely(before < first_t || after > last_t)) {
        // the chart has not been updated in the wanted timeframe
        debug(D_BACKEND, "BACKEND: %s.%s.%s: aligned timeframe %lu to %lu is outside the chart's database range %lu to %lu",
              host->hostname, st->id, rd->id,
              (unsigned long)after, (unsigned long)before,
              (unsigned long)first_t, (unsigned long)last_t
        );
        return NAN;
    }

    *first_timestamp = after;
    *last_timestamp = before;

    size_t counter = 0;
    calculated_number sum = 0;

/*
    long    start_at_slot = rrdset_time2slot(st, before),
            stop_at_slot  = rrdset_time2slot(st, after),
            slot, stop_now = 0;

    for(slot = start_at_slot; !stop_now ; slot--) {

        if(unlikely(slot < 0)) slot = st->entries - 1;
        if(unlikely(slot == stop_at_slot)) stop_now = 1;

        storage_number n = rd->values[slot];

        if(unlikely(!does_storage_number_exist(n))) {
            // not collected
            continue;
        }

        calculated_number value = unpack_storage_number(n);
        sum += value;

        counter++;
    }
*/
    for(rd->state->query_ops.init(rd, &handle, after, before) ; !rd->state->query_ops.is_finished(&handle) ; ) {
        n = rd->state->query_ops.next_metric(&handle);

        if(unlikely(!does_storage_number_exist(n))) {
            // not collected
            continue;
        }

        calculated_number value = unpack_storage_number(n);
        sum += value;

        counter++;
    }
    rd->state->query_ops.finalize(&handle);
    if(unlikely(!counter)) {
        debug(D_BACKEND, "BACKEND: %s.%s.%s: no values stored in database for range %lu to %lu",
              host->hostname, st->id, rd->id,
              (unsigned long)after, (unsigned long)before
        );
        return NAN;
    }

    if(unlikely(BACKEND_OPTIONS_DATA_SOURCE(backend_options) == BACKEND_SOURCE_DATA_SUM))
        return sum;

    return sum / (calculated_number)counter;
}


// discard a response received by a backend
// after logging a simple of it to error.log

int discard_response(BUFFER *b, const char *backend) {
    char sample[1024];
    const char *s = buffer_tostring(b);
    char *d = sample, *e = &sample[sizeof(sample) - 1];

    for(; *s && d < e ;s++) {
        char c = *s;
        if(unlikely(!isprint(c))) c = ' ';
        *d++ = c;
    }
    *d = '\0';

    info("BACKEND: received %zu bytes from %s backend. Ignoring them. Sample: '%s'", buffer_strlen(b), backend, sample);
    buffer_flush(b);
    return 0;
}


// ----------------------------------------------------------------------------
// the backend thread

static SIMPLE_PATTERN *charts_pattern = NULL;
static SIMPLE_PATTERN *hosts_pattern = NULL;

inline int backends_can_send_rrdset(BACKEND_OPTIONS backend_options, RRDSET *st) {
    RRDHOST *host = st->rrdhost;
    (void)host;

    if(unlikely(rrdset_flag_check(st, RRDSET_FLAG_BACKEND_IGNORE)))
        return 0;

    if(unlikely(!rrdset_flag_check(st, RRDSET_FLAG_BACKEND_SEND))) {
        // we have not checked this chart
        if(simple_pattern_matches(charts_pattern, st->id) || simple_pattern_matches(charts_pattern, st->name))
            rrdset_flag_set(st, RRDSET_FLAG_BACKEND_SEND);
        else {
            rrdset_flag_set(st, RRDSET_FLAG_BACKEND_IGNORE);
            debug(D_BACKEND, "BACKEND: not sending chart '%s' of host '%s', because it is disabled for backends.", st->id, host->hostname);
            return 0;
        }
    }

    if(unlikely(!rrdset_is_available_for_backends(st))) {
        debug(D_BACKEND, "BACKEND: not sending chart '%s' of host '%s', because it is not available for backends.", st->id, host->hostname);
        return 0;
    }

    if(unlikely(st->rrd_memory_mode == RRD_MEMORY_MODE_NONE && !(BACKEND_OPTIONS_DATA_SOURCE(backend_options) == BACKEND_SOURCE_DATA_AS_COLLECTED))) {
        debug(D_BACKEND, "BACKEND: not sending chart '%s' of host '%s' because its memory mode is '%s' and the backend requires database access.", st->id, host->hostname, rrd_memory_mode_name(host->rrd_memory_mode));
        return 0;
    }

    return 1;
}

inline BACKEND_OPTIONS backend_parse_data_source(const char *source, BACKEND_OPTIONS backend_options) {
    if(!strcmp(source, "raw") || !strcmp(source, "as collected") || !strcmp(source, "as-collected") || !strcmp(source, "as_collected") || !strcmp(source, "ascollected")) {
        backend_options |= BACKEND_SOURCE_DATA_AS_COLLECTED;
        backend_options &= ~(BACKEND_OPTIONS_SOURCE_BITS ^ BACKEND_SOURCE_DATA_AS_COLLECTED);
    }
    else if(!strcmp(source, "average")) {
        backend_options |= BACKEND_SOURCE_DATA_AVERAGE;
        backend_options &= ~(BACKEND_OPTIONS_SOURCE_BITS ^ BACKEND_SOURCE_DATA_AVERAGE);
    }
    else if(!strcmp(source, "sum") || !strcmp(source, "volume")) {
        backend_options |= BACKEND_SOURCE_DATA_SUM;
        backend_options &= ~(BACKEND_OPTIONS_SOURCE_BITS ^ BACKEND_SOURCE_DATA_SUM);
    }
    else {
        error("BACKEND: invalid data source method '%s'.", source);
    }

    return backend_options;
}

static void backends_main_cleanup(void *ptr) {
    struct netdata_static_thread *static_thread = (struct netdata_static_thread *)ptr;
    static_thread->enabled = NETDATA_MAIN_THREAD_EXITING;

    info("cleaning up...");

    static_thread->enabled = NETDATA_MAIN_THREAD_EXITED;
}

void *backends_main(void *ptr) {
    netdata_thread_cleanup_push(backends_main_cleanup, ptr);

    int default_port = 0;
    int sock = -1;
    BUFFER *b = buffer_create(1), *response = buffer_create(1);
    int (*backend_request_formatter)(BUFFER *, const char *, RRDHOST *, const char *, RRDSET *, RRDDIM *, time_t, time_t, BACKEND_OPTIONS) = NULL;
    int (*backend_response_checker)(BUFFER *) = NULL;

#if HAVE_KINESIS
    int do_kinesis = 0;
    char *kinesis_auth_key_id = NULL, *kinesis_secure_key = NULL, *kinesis_stream_name = NULL;
#endif

#if ENABLE_PROMETHEUS_REMOTE_WRITE
    int do_prometheus_remote_write = 0;
    BUFFER *http_request_header = buffer_create(1);
#endif


    // ------------------------------------------------------------------------
    // collect configuration options

    struct timeval timeout = {
            .tv_sec = 0,
            .tv_usec = 0
    };
    int enabled                 = config_get_boolean(CONFIG_SECTION_BACKEND, "enabled", 0);
    const char *source          = config_get(CONFIG_SECTION_BACKEND, "data source", "average");
    const char *type            = config_get(CONFIG_SECTION_BACKEND, "type", "graphite");
    const char *destination     = config_get(CONFIG_SECTION_BACKEND, "destination", "localhost");
    global_backend_prefix       = config_get(CONFIG_SECTION_BACKEND, "prefix", "netdata");
    const char *hostname        = config_get(CONFIG_SECTION_BACKEND, "hostname", localhost->hostname);
    global_backend_update_every = (int)config_get_number(CONFIG_SECTION_BACKEND, "update every", global_backend_update_every);
    int buffer_on_failures      = (int)config_get_number(CONFIG_SECTION_BACKEND, "buffer on failures", 10);
    long timeoutms              = config_get_number(CONFIG_SECTION_BACKEND, "timeout ms", global_backend_update_every * 2 * 1000);

    if(config_get_boolean(CONFIG_SECTION_BACKEND, "send names instead of ids", (global_backend_options & BACKEND_OPTION_SEND_NAMES)))
        global_backend_options |= BACKEND_OPTION_SEND_NAMES;
    else
        global_backend_options &= ~BACKEND_OPTION_SEND_NAMES;

    charts_pattern = simple_pattern_create(config_get(CONFIG_SECTION_BACKEND, "send charts matching", "*"), NULL, SIMPLE_PATTERN_EXACT);
    hosts_pattern  = simple_pattern_create(config_get(CONFIG_SECTION_BACKEND, "send hosts matching", "localhost *"), NULL, SIMPLE_PATTERN_EXACT);

#if ENABLE_PROMETHEUS_REMOTE_WRITE
    const char *remote_write_path = config_get(CONFIG_SECTION_BACKEND, "remote write URL path", "/receive");
#endif

    // ------------------------------------------------------------------------
    // validate configuration options
    // and prepare for sending data to our backend

    global_backend_options = backend_parse_data_source(source, global_backend_options);

    if(timeoutms < 1) {
        error("BACKEND: invalid timeout %ld ms given. Assuming %d ms.", timeoutms, global_backend_update_every * 2 * 1000);
        timeoutms = global_backend_update_every * 2 * 1000;
    }
    timeout.tv_sec  = (timeoutms * 1000) / 1000000;
    timeout.tv_usec = (timeoutms * 1000) % 1000000;

    if(!enabled || global_backend_update_every < 1)
        goto cleanup;

    // ------------------------------------------------------------------------
    // select the backend type

    if(!strcmp(type, "graphite") || !strcmp(type, "graphite:plaintext")) {

        default_port = 2003;
        backend_response_checker = process_graphite_response;

        if(BACKEND_OPTIONS_DATA_SOURCE(global_backend_options) == BACKEND_SOURCE_DATA_AS_COLLECTED)
            backend_request_formatter = format_dimension_collected_graphite_plaintext;
        else
            backend_request_formatter = format_dimension_stored_graphite_plaintext;

    }
    else if(!strcmp(type, "opentsdb") || !strcmp(type, "opentsdb:telnet")) {

        default_port = 4242;
        backend_response_checker = process_opentsdb_response;

        if(BACKEND_OPTIONS_DATA_SOURCE(global_backend_options) == BACKEND_SOURCE_DATA_AS_COLLECTED)
            backend_request_formatter = format_dimension_collected_opentsdb_telnet;
        else
            backend_request_formatter = format_dimension_stored_opentsdb_telnet;

    }
    else if (!strcmp(type, "json") || !strcmp(type, "json:plaintext")) {

        default_port = 5448;
        backend_response_checker = process_json_response;

        if (BACKEND_OPTIONS_DATA_SOURCE(global_backend_options) == BACKEND_SOURCE_DATA_AS_COLLECTED)
            backend_request_formatter = format_dimension_collected_json_plaintext;
        else
            backend_request_formatter = format_dimension_stored_json_plaintext;

    }
    else if (!strcmp(type, "kinesis") || !strcmp(type, "kinesis:plaintext")) {
#if HAVE_KINESIS
        do_kinesis = 1;

        if(unlikely(read_kinesis_conf(netdata_configured_user_config_dir, &kinesis_auth_key_id, &kinesis_secure_key, &kinesis_stream_name))) {
            error("BACKEND: kinesis backend type is set but cannot read its configuration from %s/aws_kinesis.conf", netdata_configured_user_config_dir);
            goto cleanup;
        }

        kinesis_init(destination, kinesis_auth_key_id, kinesis_secure_key, timeout.tv_sec * 1000 + timeout.tv_usec / 1000);

        backend_response_checker = process_json_response;
        if (BACKEND_OPTIONS_DATA_SOURCE(global_backend_options) == BACKEND_SOURCE_DATA_AS_COLLECTED)
            backend_request_formatter = format_dimension_collected_json_plaintext;
        else
            backend_request_formatter = format_dimension_stored_json_plaintext;
#else
        error("AWS Kinesis support isn't compiled");
#endif /* HAVE_KINESIS */
    }
    else if (!strcmp(type, "prometheus_remote_write")) {
#if ENABLE_PROMETHEUS_REMOTE_WRITE
        do_prometheus_remote_write = 1;

        backend_response_checker = process_prometheus_remote_write_response;

        init_write_request();
#else
        error("Prometheus remote write support isn't compiled");
#endif /* ENABLE_PROMETHEUS_REMOTE_WRITE */
    }
    else {
        error("BACKEND: Unknown backend type '%s'", type);
        goto cleanup;
    }

#if ENABLE_PROMETHEUS_REMOTE_WRITE
    if((backend_request_formatter == NULL && !do_prometheus_remote_write) || backend_response_checker == NULL) {
#else
    if(backend_request_formatter == NULL || backend_response_checker == NULL) {
#endif
        error("BACKEND: backend is misconfigured - disabling it.");
        goto cleanup;
    }


    // ------------------------------------------------------------------------
    // prepare the charts for monitoring the backend operation

    struct rusage thread;

    collected_number
            chart_buffered_metrics = 0,
            chart_lost_metrics = 0,
            chart_sent_metrics = 0,
            chart_buffered_bytes = 0,
            chart_received_bytes = 0,
            chart_sent_bytes = 0,
            chart_receptions = 0,
            chart_transmission_successes = 0,
            chart_transmission_failures = 0,
            chart_data_lost_events = 0,
            chart_lost_bytes = 0,
            chart_backend_reconnects = 0;
            // chart_backend_latency = 0;

    RRDSET *chart_metrics = rrdset_create_localhost("netdata", "backend_metrics", NULL, "backend", NULL, "Netdata Buffered Metrics", "metrics", "backends", NULL, 130600, global_backend_update_every, RRDSET_TYPE_LINE);
    rrddim_add(chart_metrics, "buffered", NULL,  1, 1, RRD_ALGORITHM_ABSOLUTE);
    rrddim_add(chart_metrics, "lost",     NULL,  1, 1, RRD_ALGORITHM_ABSOLUTE);
    rrddim_add(chart_metrics, "sent",     NULL,  1, 1, RRD_ALGORITHM_ABSOLUTE);

    RRDSET *chart_bytes = rrdset_create_localhost("netdata", "backend_bytes", NULL, "backend", NULL, "Netdata Backend Data Size", "KiB", "backends", NULL, 130610, global_backend_update_every, RRDSET_TYPE_AREA);
    rrddim_add(chart_bytes, "buffered", NULL, 1, 1024, RRD_ALGORITHM_ABSOLUTE);
    rrddim_add(chart_bytes, "lost",     NULL, 1, 1024, RRD_ALGORITHM_ABSOLUTE);
    rrddim_add(chart_bytes, "sent",     NULL, 1, 1024, RRD_ALGORITHM_ABSOLUTE);
    rrddim_add(chart_bytes, "received", NULL, 1, 1024, RRD_ALGORITHM_ABSOLUTE);

    RRDSET *chart_ops = rrdset_create_localhost("netdata", "backend_ops", NULL, "backend", NULL, "Netdata Backend Operations", "operations", "backends", NULL, 130630, global_backend_update_every, RRDSET_TYPE_LINE);
    rrddim_add(chart_ops, "write",     NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
    rrddim_add(chart_ops, "discard",   NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
    rrddim_add(chart_ops, "reconnect", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
    rrddim_add(chart_ops, "failure",   NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
    rrddim_add(chart_ops, "read",      NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);

    /*
     * this is misleading - we can only measure the time we need to send data
     * this time is not related to the time required for the data to travel to
     * the backend database and the time that server needed to process them
     *
     * issue #1432 and https://www.softlab.ntua.gr/facilities/documentation/unix/unix-socket-faq/unix-socket-faq-2.html
     *
    RRDSET *chart_latency = rrdset_create_localhost("netdata", "backend_latency", NULL, "backend", NULL, "Netdata Backend Latency", "ms", "backends", NULL, 130620, global_backend_update_every, RRDSET_TYPE_AREA);
    rrddim_add(chart_latency, "latency",   NULL,  1, 1000, RRD_ALGORITHM_ABSOLUTE);
    */

    RRDSET *chart_rusage = rrdset_create_localhost("netdata", "backend_thread_cpu", NULL, "backend", NULL, "NetData Backend Thread CPU usage", "milliseconds/s", "backends", NULL, 130630, global_backend_update_every, RRDSET_TYPE_STACKED);
    rrddim_add(chart_rusage, "user",   NULL, 1, 1000, RRD_ALGORITHM_INCREMENTAL);
    rrddim_add(chart_rusage, "system", NULL, 1, 1000, RRD_ALGORITHM_INCREMENTAL);


    // ------------------------------------------------------------------------
    // prepare the backend main loop

    info("BACKEND: configured ('%s' on '%s' sending '%s' data, every %d seconds, as host '%s', with prefix '%s')", type, destination, source, global_backend_update_every, hostname, global_backend_prefix);

    usec_t step_ut = global_backend_update_every * USEC_PER_SEC;
    time_t after = now_realtime_sec();
    int failures = 0;
    heartbeat_t hb;
    heartbeat_init(&hb);

    while(!netdata_exit) {

        // ------------------------------------------------------------------------
        // Wait for the next iteration point.

        heartbeat_next(&hb, step_ut);
        time_t before = now_realtime_sec();
        debug(D_BACKEND, "BACKEND: preparing buffer for timeframe %lu to %lu", (unsigned long)after, (unsigned long)before);

        // ------------------------------------------------------------------------
        // add to the buffer the data we need to send to the backend

        netdata_thread_disable_cancelability();

        size_t count_hosts = 0;
        size_t count_charts_total = 0;
        size_t count_dims_total = 0;

#if ENABLE_PROMETHEUS_REMOTE_WRITE
        clear_write_request();
#endif
        rrd_rdlock();
        RRDHOST *host;
        rrdhost_foreach_read(host) {
            if(unlikely(!rrdhost_flag_check(host, RRDHOST_FLAG_BACKEND_SEND|RRDHOST_FLAG_BACKEND_DONT_SEND))) {
                char *name = (host == localhost)?"localhost":host->hostname;
                if (!hosts_pattern || simple_pattern_matches(hosts_pattern, name)) {
                    rrdhost_flag_set(host, RRDHOST_FLAG_BACKEND_SEND);
                    info("enabled backend for host '%s'", name);
                }
                else {
                    rrdhost_flag_set(host, RRDHOST_FLAG_BACKEND_DONT_SEND);
                    info("disabled backend for host '%s'", name);
                }
            }

            if(unlikely(!rrdhost_flag_check(host, RRDHOST_FLAG_BACKEND_SEND)))
                continue;

            rrdhost_rdlock(host);

            count_hosts++;
            size_t count_charts = 0;
            size_t count_dims = 0;
            size_t count_dims_skipped = 0;

            const char *__hostname = (host == localhost)?hostname:host->hostname;

#if ENABLE_PROMETHEUS_REMOTE_WRITE
            if(do_prometheus_remote_write) {
                rrd_stats_remote_write_allmetrics_prometheus(
                    host
                    , __hostname
                    , global_backend_prefix
                    , global_backend_options
                    , after
                    , before
                    , &count_charts
                    , &count_dims
                    , &count_dims_skipped
                );
                chart_buffered_metrics += count_dims;
            }
            else
#endif
            {
                RRDSET *st;
                rrdset_foreach_read(st, host) {
                    if(likely(backends_can_send_rrdset(global_backend_options, st))) {
                        rrdset_rdlock(st);

                        count_charts++;

                        RRDDIM *rd;
                        rrddim_foreach_read(rd, st) {
                            if (likely(rd->last_collected_time.tv_sec >= after)) {
                                chart_buffered_metrics += backend_request_formatter(b, global_backend_prefix, host, __hostname, st, rd, after, before, global_backend_options);
                                count_dims++;
                            }
                            else {
                                debug(D_BACKEND, "BACKEND: not sending dimension '%s' of chart '%s' from host '%s', its last data collection (%lu) is not within our timeframe (%lu to %lu)", rd->id, st->id, __hostname, (unsigned long)rd->last_collected_time.tv_sec, (unsigned long)after, (unsigned long)before);
                                count_dims_skipped++;
                            }
                        }

                        rrdset_unlock(st);
                    }
                }
            }

            debug(D_BACKEND, "BACKEND: sending host '%s', metrics of %zu dimensions, of %zu charts. Skipped %zu dimensions.", __hostname, count_dims, count_charts, count_dims_skipped);
            count_charts_total += count_charts;
            count_dims_total += count_dims;

            rrdhost_unlock(host);
        }
        rrd_unlock();

        netdata_thread_enable_cancelability();

        debug(D_BACKEND, "BACKEND: buffer has %zu bytes, added metrics for %zu dimensions, of %zu charts, from %zu hosts", buffer_strlen(b), count_dims_total, count_charts_total, count_hosts);

        // ------------------------------------------------------------------------

        chart_buffered_bytes = (collected_number)buffer_strlen(b);

        // reset the monitoring chart counters
        chart_received_bytes =
        chart_sent_bytes =
        chart_sent_metrics =
        chart_lost_metrics =
        chart_receptions =
        chart_transmission_successes =
        chart_transmission_failures =
        chart_data_lost_events =
        chart_lost_bytes =
        chart_backend_reconnects = 0;
        // chart_backend_latency = 0;

        if(unlikely(netdata_exit)) break;

        //fprintf(stderr, "\nBACKEND BEGIN:\n%s\nBACKEND END\n", buffer_tostring(b));
        //fprintf(stderr, "after = %lu, before = %lu\n", after, before);

        // prepare for the next iteration
        // to add incrementally data to buffer
        after = before;

#if HAVE_KINESIS
        if(do_kinesis) {
            unsigned long long partition_key_seq = 0;

            size_t buffer_len = buffer_strlen(b);
            size_t sent = 0;

            while(sent < buffer_len) {
                char partition_key[KINESIS_PARTITION_KEY_MAX + 1];
                snprintf(partition_key, KINESIS_PARTITION_KEY_MAX, "netdata_%llu", partition_key_seq++);
                size_t partition_key_len = strnlen(partition_key, KINESIS_PARTITION_KEY_MAX);

                const char *first_char = buffer_tostring(b) + sent;

                size_t record_len = 0;

                // split buffer into chunks of maximum allowed size
                if(buffer_len - sent < KINESIS_RECORD_MAX - partition_key_len) {
                    record_len = buffer_len - sent;
                }
                else {
                    record_len = KINESIS_RECORD_MAX - partition_key_len;
                    while(*(first_char + record_len) != '\n' && record_len) record_len--;
                }

                char error_message[ERROR_LINE_MAX + 1] = "";

                debug(D_BACKEND, "BACKEND: kinesis_put_record(): dest = %s, id = %s, key = %s, stream = %s, partition_key = %s, \
                      buffer = %zu, record = %zu", destination, kinesis_auth_key_id, kinesis_secure_key, kinesis_stream_name,
                      partition_key, buffer_len, record_len);

                kinesis_put_record(kinesis_stream_name, partition_key, first_char, record_len);

                sent += record_len;
                chart_transmission_successes++;

                size_t sent_bytes = 0, lost_bytes = 0;

                if(unlikely(kinesis_get_result(error_message, &sent_bytes, &lost_bytes))) {
                    // oops! we couldn't send (all or some of the) data
                    error("BACKEND: %s", error_message);
                    error("BACKEND: failed to write data to database backend '%s'. Willing to write %zu bytes, wrote %zu bytes.",
                          destination, sent_bytes, sent_bytes - lost_bytes);

                    chart_transmission_failures++;
                    chart_data_lost_events++;
                    chart_lost_bytes += lost_bytes;

                    // estimate the number of lost metrics
                    chart_lost_metrics += (collected_number)(chart_buffered_metrics
                                          * (buffer_len && (lost_bytes > buffer_len) ? (double)lost_bytes / buffer_len : 1));

                    break;
                }
                else {
                    chart_receptions++;
                }

                if(unlikely(netdata_exit)) break;
            }

            chart_sent_bytes += sent;
            if(likely(sent == buffer_len))
                chart_sent_metrics = chart_buffered_metrics;

            buffer_flush(b);
        }
        else {
#else
        {
#endif /* HAVE_KINESIS */

            // ------------------------------------------------------------------------
            // if we are connected, receive a response, without blocking

            if(likely(sock != -1)) {
                errno = 0;

                // loop through to collect all data
                while(sock != -1 && errno != EWOULDBLOCK) {
                    buffer_need_bytes(response, 4096);

                    ssize_t r = recv(sock, &response->buffer[response->len], response->size - response->len, MSG_DONTWAIT);
                    if(likely(r > 0)) {
                        // we received some data
                        response->len += r;
                        chart_received_bytes += r;
                        chart_receptions++;
                    }
                    else if(r == 0) {
                        error("BACKEND: '%s' closed the socket", destination);
                        close(sock);
                        sock = -1;
                    }
                    else {
                        // failed to receive data
                        if(errno != EAGAIN && errno != EWOULDBLOCK) {
                            error("BACKEND: cannot receive data from backend '%s'.", destination);
                        }
                    }
                }

                // if we received data, process them
                if(buffer_strlen(response))
                    backend_response_checker(response);
            }

            // ------------------------------------------------------------------------
            // if we are not connected, connect to a backend server

            if(unlikely(sock == -1)) {
                // usec_t start_ut = now_monotonic_usec();
                size_t reconnects = 0;

                sock = connect_to_one_of(destination, default_port, &timeout, &reconnects, NULL, 0);

                chart_backend_reconnects += reconnects;
                // chart_backend_latency += now_monotonic_usec() - start_ut;
            }

            if(unlikely(netdata_exit)) break;

            // ------------------------------------------------------------------------
            // if we are connected, send our buffer to the backend server

            if(likely(sock != -1)) {
                size_t len = buffer_strlen(b);
                // usec_t start_ut = now_monotonic_usec();
                int flags = 0;
    #ifdef MSG_NOSIGNAL
                flags += MSG_NOSIGNAL;
    #endif

#if ENABLE_PROMETHEUS_REMOTE_WRITE
                if(do_prometheus_remote_write) {
                    size_t data_size = get_write_request_size();

                    if(unlikely(!data_size)) {
                        error("BACKEND: write request size is out of range");
                        continue;
                    }

                    buffer_flush(b);
                    buffer_need_bytes(b, data_size);
                    if(unlikely(pack_write_request(b->buffer, &data_size))) {
                        error("BACKEND: cannot pack write request");
                        continue;
                    }
                    b->len = data_size;
                    chart_buffered_bytes = (collected_number)buffer_strlen(b);

                    buffer_flush(http_request_header);
                    buffer_sprintf(http_request_header,
                                    "POST %s HTTP/1.1\r\n"
                                    "Host: %s\r\n"
                                    "Accept: */*\r\n"
                                    "Content-Length: %zu\r\n"
                                    "Content-Type: application/x-www-form-urlencoded\r\n\r\n",
                                    remote_write_path,
                                    hostname,
                                    data_size
                    );

                    len = buffer_strlen(http_request_header);
                    send(sock, buffer_tostring(http_request_header), len, flags);

                    len = data_size;
                }
#endif

                ssize_t written = send(sock, buffer_tostring(b), len, flags);
                // chart_backend_latency += now_monotonic_usec() - start_ut;
                if(written != -1 && (size_t)written == len) {
                    // we sent the data successfully
                    chart_transmission_successes++;
                    chart_sent_bytes += written;
                    chart_sent_metrics = chart_buffered_metrics;

                    // reset the failures count
                    failures = 0;

                    // empty the buffer
                    buffer_flush(b);
                }
                else {
                    // oops! we couldn't send (all or some of the) data
                    error("BACKEND: failed to write data to database backend '%s'. Willing to write %zu bytes, wrote %zd bytes. Will re-connect.", destination, len, written);
                    chart_transmission_failures++;

                    if(written != -1)
                        chart_sent_bytes += written;

                    // increment the counter we check for data loss
                    failures++;

                    // close the socket - we will re-open it next time
                    close(sock);
                    sock = -1;
                }
            }
            else {
                error("BACKEND: failed to update database backend '%s'", destination);
                chart_transmission_failures++;

                // increment the counter we check for data loss
                failures++;
            }
        }


#if ENABLE_PROMETHEUS_REMOTE_WRITE
        if(failures) {
            (void) buffer_on_failures;
            failures = 0;
            chart_lost_bytes = chart_buffered_bytes = get_write_request_size(); // estimated write request size
            chart_data_lost_events++;
            chart_lost_metrics = chart_buffered_metrics;
        }
#else
        if(failures > buffer_on_failures) {
            // too bad! we are going to lose data
            chart_lost_bytes += buffer_strlen(b);
            error("BACKEND: reached %d backend failures. Flushing buffers to protect this host - this results in data loss on back-end server '%s'", failures, destination);
            buffer_flush(b);
            failures = 0;
            chart_data_lost_events++;
            chart_lost_metrics = chart_buffered_metrics;
        }
#endif /* ENABLE_PROMETHEUS_REMOTE_WRITE */

        if(unlikely(netdata_exit)) break;

        // ------------------------------------------------------------------------
        // update the monitoring charts

        if(likely(chart_ops->counter_done)) rrdset_next(chart_ops);
        rrddim_set(chart_ops, "read",         chart_receptions);
        rrddim_set(chart_ops, "write",        chart_transmission_successes);
        rrddim_set(chart_ops, "discard",      chart_data_lost_events);
        rrddim_set(chart_ops, "failure",      chart_transmission_failures);
        rrddim_set(chart_ops, "reconnect",    chart_backend_reconnects);
        rrdset_done(chart_ops);

        if(likely(chart_metrics->counter_done)) rrdset_next(chart_metrics);
        rrddim_set(chart_metrics, "buffered", chart_buffered_metrics);
        rrddim_set(chart_metrics, "lost",     chart_lost_metrics);
        rrddim_set(chart_metrics, "sent",     chart_sent_metrics);
        rrdset_done(chart_metrics);

        if(likely(chart_bytes->counter_done)) rrdset_next(chart_bytes);
        rrddim_set(chart_bytes, "buffered",   chart_buffered_bytes);
        rrddim_set(chart_bytes, "lost",       chart_lost_bytes);
        rrddim_set(chart_bytes, "sent",       chart_sent_bytes);
        rrddim_set(chart_bytes, "received",   chart_received_bytes);
        rrdset_done(chart_bytes);

        /*
        if(likely(chart_latency->counter_done)) rrdset_next(chart_latency);
        rrddim_set(chart_latency, "latency",  chart_backend_latency);
        rrdset_done(chart_latency);
        */

        getrusage(RUSAGE_THREAD, &thread);
        if(likely(chart_rusage->counter_done)) rrdset_next(chart_rusage);
        rrddim_set(chart_rusage, "user",   thread.ru_utime.tv_sec * 1000000ULL + thread.ru_utime.tv_usec);
        rrddim_set(chart_rusage, "system", thread.ru_stime.tv_sec * 1000000ULL + thread.ru_stime.tv_usec);
        rrdset_done(chart_rusage);

        if(likely(buffer_strlen(b) == 0))
            chart_buffered_metrics = 0;

        if(unlikely(netdata_exit)) break;
    }

cleanup:
#if HAVE_KINESIS
    if(do_kinesis) {
        kinesis_shutdown();
        freez(kinesis_auth_key_id);
        freez(kinesis_secure_key);
        freez(kinesis_stream_name);
    }
#endif

#if ENABLE_PROMETHEUS_REMOTE_WRITE
    if(do_prometheus_remote_write) {
        buffer_free(http_request_header);
        protocol_buffers_shutdown();
    }
#endif

    if(sock != -1)
        close(sock);

    buffer_free(b);
    buffer_free(response);

    netdata_thread_cleanup_pop(1);
    return NULL;
}
