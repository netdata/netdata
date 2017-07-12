#include "common.h"

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

const char *backend_prefix = "netdata";
int backend_send_names = 1;
int backend_update_every = 10;
uint32_t backend_options = BACKEND_SOURCE_DATA_AVERAGE;

// ----------------------------------------------------------------------------
// helper functions for backends

static inline size_t backend_name_copy(char *d, const char *s, size_t usable) {
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

inline calculated_number backend_calculate_value_from_stored_data(
          RRDSET *st                // the chart
        , RRDDIM *rd                // the dimension
        , time_t after              // the start timestamp
        , time_t before             // the end timestamp
        , uint32_t options          // BACKEND_SOURCE_* bitmap
        , time_t *first_timestamp   // the first point of the database used in this response
        , time_t *last_timestamp    // the timestamp that should be reported to backend
) {
    // find the edges of the rrd database for this chart
    time_t first_t = rrdset_first_entry_t(st);
    time_t last_t  = rrdset_last_entry_t(st);
    time_t update_every = st->update_every;

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
              st->rrdhost->hostname, st->id, rd->id,
              (unsigned long)after, (unsigned long)before,
              (unsigned long)first_t, (unsigned long)last_t
        );
        return NAN;
    }

    *first_timestamp = after;
    *last_timestamp = before;

    size_t counter = 0;
    calculated_number sum = 0;

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

    if(unlikely(!counter)) {
        debug(D_BACKEND, "BACKEND: %s.%s.%s: no values stored in database for range %lu to %lu",
              st->rrdhost->hostname, st->id, rd->id,
              (unsigned long)after, (unsigned long)before
        );
        return NAN;
    }

    if(unlikely((options & BACKEND_SOURCE_BITS) == BACKEND_SOURCE_DATA_SUM))
        return sum;

    return sum / (calculated_number)counter;
}


// discard a response received by a backend
// after logging a simple of it to error.log

static inline int discard_response(BUFFER *b, const char *backend) {
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
// graphite backend

static inline int format_dimension_collected_graphite_plaintext(
          BUFFER *b                 // the buffer to write data to
        , const char *prefix        // the prefix to use
        , RRDHOST *host             // the host this chart comes from
        , const char *hostname      // the hostname (to override host->hostname)
        , RRDSET *st                // the chart
        , RRDDIM *rd                // the dimension
        , time_t after              // the start timestamp
        , time_t before             // the end timestamp
        , uint32_t options          // BACKEND_SOURCE_* bitmap
) {
    (void)host;
    (void)after;
    (void)before;
    (void)options;

    char chart_name[RRD_ID_LENGTH_MAX + 1];
    char dimension_name[RRD_ID_LENGTH_MAX + 1];
    backend_name_copy(chart_name, (backend_send_names && st->name)?st->name:st->id, RRD_ID_LENGTH_MAX);
    backend_name_copy(dimension_name, (backend_send_names && rd->name)?rd->name:rd->id, RRD_ID_LENGTH_MAX);

    buffer_sprintf(
            b
            , "%s.%s.%s.%s " COLLECTED_NUMBER_FORMAT " %u\n"
            , prefix
            , hostname
            , chart_name
            , dimension_name
            , rd->last_collected_value
            , (uint32_t)rd->last_collected_time.tv_sec
    );

    return 1;
}

static inline int format_dimension_stored_graphite_plaintext(
          BUFFER *b                 // the buffer to write data to
        , const char *prefix        // the prefix to use
        , RRDHOST *host             // the host this chart comes from
        , const char *hostname      // the hostname (to override host->hostname)
        , RRDSET *st                // the chart
        , RRDDIM *rd                // the dimension
        , time_t after              // the start timestamp
        , time_t before             // the end timestamp
        , uint32_t options          // BACKEND_SOURCE_* bitmap
) {
    (void)host;

    char chart_name[RRD_ID_LENGTH_MAX + 1];
    char dimension_name[RRD_ID_LENGTH_MAX + 1];
    backend_name_copy(chart_name, (backend_send_names && st->name)?st->name:st->id, RRD_ID_LENGTH_MAX);
    backend_name_copy(dimension_name, (backend_send_names && rd->name)?rd->name:rd->id, RRD_ID_LENGTH_MAX);

    time_t first_t = after, last_t = before;
    calculated_number value = backend_calculate_value_from_stored_data(st, rd, after, before, options, &first_t, &last_t);

    if(!isnan(value)) {

        buffer_sprintf(
                b
                , "%s.%s.%s.%s " CALCULATED_NUMBER_FORMAT " %u\n"
                , prefix
                , hostname
                , chart_name
                , dimension_name
                , value
                , (uint32_t) last_t
        );

        return 1;
    }
    return 0;
}

static inline int process_graphite_response(BUFFER *b) {
    return discard_response(b, "graphite");
}


// ----------------------------------------------------------------------------
// opentsdb backend

static inline int format_dimension_collected_opentsdb_telnet(
          BUFFER *b                 // the buffer to write data to
        , const char *prefix        // the prefix to use
        , RRDHOST *host             // the host this chart comes from
        , const char *hostname      // the hostname (to override host->hostname)
        , RRDSET *st                // the chart
        , RRDDIM *rd                // the dimension
        , time_t after              // the start timestamp
        , time_t before             // the end timestamp
        , uint32_t options          // BACKEND_SOURCE_* bitmap
) {
    (void)host;
    (void)after;
    (void)before;
    (void)options;

    char chart_name[RRD_ID_LENGTH_MAX + 1];
    char dimension_name[RRD_ID_LENGTH_MAX + 1];
    backend_name_copy(chart_name, (backend_send_names && st->name)?st->name:st->id, RRD_ID_LENGTH_MAX);
    backend_name_copy(dimension_name, (backend_send_names && rd->name)?rd->name:rd->id, RRD_ID_LENGTH_MAX);

    buffer_sprintf(
            b
            , "put %s.%s.%s %u " COLLECTED_NUMBER_FORMAT " host=%s%s%s\n"
            , prefix
            , chart_name
            , dimension_name
            , (uint32_t)rd->last_collected_time.tv_sec
            , rd->last_collected_value
            , hostname
            , (host->tags)?" ":""
            , (host->tags)?host->tags:""
    );

    return 1;
}

static inline int format_dimension_stored_opentsdb_telnet(
          BUFFER *b                 // the buffer to write data to
        , const char *prefix        // the prefix to use
        , RRDHOST *host             // the host this chart comes from
        , const char *hostname      // the hostname (to override host->hostname)
        , RRDSET *st                // the chart
        , RRDDIM *rd                // the dimension
        , time_t after              // the start timestamp
        , time_t before             // the end timestamp
        , uint32_t options          // BACKEND_SOURCE_* bitmap
) {
    (void)host;

    time_t first_t = after, last_t = before;
    calculated_number value = backend_calculate_value_from_stored_data(st, rd, after, before, options, &first_t, &last_t);

    char chart_name[RRD_ID_LENGTH_MAX + 1];
    char dimension_name[RRD_ID_LENGTH_MAX + 1];
    backend_name_copy(chart_name, (backend_send_names && st->name)?st->name:st->id, RRD_ID_LENGTH_MAX);
    backend_name_copy(dimension_name, (backend_send_names && rd->name)?rd->name:rd->id, RRD_ID_LENGTH_MAX);

    if(!isnan(value)) {

        buffer_sprintf(
                b
                , "put %s.%s.%s %u " CALCULATED_NUMBER_FORMAT " host=%s%s%s\n"
                , prefix
                , chart_name
                , dimension_name
                , (uint32_t) last_t
                , value
                , hostname
                , (host->tags)?" ":""
                , (host->tags)?host->tags:""
        );

        return 1;
    }
    return 0;
}

static inline int process_opentsdb_response(BUFFER *b) {
    return discard_response(b, "opentsdb");
}


// ----------------------------------------------------------------------------
// json backend

static inline int format_dimension_collected_json_plaintext(
          BUFFER *b                 // the buffer to write data to
        , const char *prefix        // the prefix to use
        , RRDHOST *host             // the host this chart comes from
        , const char *hostname      // the hostname (to override host->hostname)
        , RRDSET *st                // the chart
        , RRDDIM *rd                // the dimension
        , time_t after              // the start timestamp
        , time_t before             // the end timestamp
        , uint32_t options          // BACKEND_SOURCE_* bitmap
) {
    (void)host;
    (void)after;
    (void)before;
    (void)options;

    buffer_sprintf(b, "{"
        "\"prefix\":\"%s\","
        "\"hostname\":\"%s\","

        "\"chart_id\":\"%s\","
        "\"chart_name\":\"%s\","
        "\"chart_family\":\"%s\","
        "\"chart_context\": \"%s\","
        "\"chart_type\":\"%s\","
        "\"units\": \"%s\","

        "\"id\":\"%s\","
        "\"name\":\"%s\","
        "\"value\":" COLLECTED_NUMBER_FORMAT ","

        "\"timestamp\": %u}\n", 
            prefix,
            hostname,

            st->id,
            st->name,
            st->family,
            st->context,
            st->type,
            st->units,

            rd->id,
            rd->name,
            rd->last_collected_value,

            (uint32_t)rd->last_collected_time.tv_sec
    );

    return 1;
}

static inline int format_dimension_stored_json_plaintext(
          BUFFER *b                 // the buffer to write data to
        , const char *prefix        // the prefix to use
        , RRDHOST *host             // the host this chart comes from
        , const char *hostname      // the hostname (to override host->hostname)
        , RRDSET *st                // the chart
        , RRDDIM *rd                // the dimension
        , time_t after              // the start timestamp
        , time_t before             // the end timestamp
        , uint32_t options          // BACKEND_SOURCE_* bitmap
) {
    (void)host;

    time_t first_t = after, last_t = before;
    calculated_number value = backend_calculate_value_from_stored_data(st, rd, after, before, options, &first_t, &last_t);

    if(!isnan(value)) {
        buffer_sprintf(b, "{"
            "\"prefix\":\"%s\","
            "\"hostname\":\"%s\","

            "\"chart_id\":\"%s\","
            "\"chart_name\":\"%s\","
            "\"chart_family\":\"%s\","
            "\"chart_context\": \"%s\","
            "\"chart_type\":\"%s\","
            "\"units\": \"%s\","

            "\"id\":\"%s\","
            "\"name\":\"%s\","
            "\"value\":" CALCULATED_NUMBER_FORMAT ","

            "\"timestamp\": %u}\n", 
                prefix,
                hostname,
                
                st->id,
                st->name,
                st->family,
                st->context,
                st->type,
                st->units,

                rd->id,
                rd->name,
                value, 
                
                (uint32_t) last_t
        );
        
        return 1;
    }
    return 0;
}

static inline int process_json_response(BUFFER *b) {
    return discard_response(b, "json");
}


// ----------------------------------------------------------------------------
// the backend thread

static SIMPLE_PATTERN *charts_pattern = NULL;

inline int backends_can_send_rrdset(uint32_t options, RRDSET *st) {
    if(unlikely(rrdset_flag_check(st, RRDSET_FLAG_BACKEND_IGNORE)))
        return 0;

    if(unlikely(!rrdset_flag_check(st, RRDSET_FLAG_BACKEND_SEND))) {
        // we have not checked this chart
        if(simple_pattern_matches(charts_pattern, st->id) || simple_pattern_matches(charts_pattern, st->name))
            rrdset_flag_set(st, RRDSET_FLAG_BACKEND_SEND);
        else {
            rrdset_flag_set(st, RRDSET_FLAG_BACKEND_IGNORE);
            debug(D_BACKEND, "BACKEND: not sending chart '%s' of host '%s', because it is disabled for backends.", st->id, st->rrdhost->hostname);
            return 0;
        }
    }

    if(unlikely(!rrdset_is_available_for_backends(st))) {
        debug(D_BACKEND, "BACKEND: not sending chart '%s' of host '%s', because it is not available for backends.", st->id, st->rrdhost->hostname);
        return 0;
    }

    if(unlikely(st->rrd_memory_mode == RRD_MEMORY_MODE_NONE && !((options & BACKEND_SOURCE_BITS) == BACKEND_SOURCE_DATA_AS_COLLECTED))) {
        debug(D_BACKEND, "BACKEND: not sending chart '%s' of host '%s' because its memory mode is '%s' and the backend requires database access.", st->id, st->rrdhost->hostname, rrd_memory_mode_name(st->rrdhost->rrd_memory_mode));
        return 0;
    }

    return 1;
}

inline uint32_t backend_parse_data_source(const char *source, uint32_t mode) {
    if(!strcmp(source, "raw") || !strcmp(source, "as collected") || !strcmp(source, "as-collected") || !strcmp(source, "as_collected") || !strcmp(source, "ascollected")) {
        mode |= BACKEND_SOURCE_DATA_AS_COLLECTED;
        mode &= ~(BACKEND_SOURCE_BITS ^ BACKEND_SOURCE_DATA_AS_COLLECTED);
    }
    else if(!strcmp(source, "average")) {
        mode |= BACKEND_SOURCE_DATA_AVERAGE;
        mode &= ~(BACKEND_SOURCE_BITS ^ BACKEND_SOURCE_DATA_AVERAGE);
    }
    else if(!strcmp(source, "sum") || !strcmp(source, "volume")) {
        mode |= BACKEND_SOURCE_DATA_SUM;
        mode &= ~(BACKEND_SOURCE_BITS ^ BACKEND_SOURCE_DATA_SUM);
    }
    else {
        error("BACKEND: invalid data source method '%s'.", source);
    }

    return mode;
}

void *backends_main(void *ptr) {
    int default_port = 0;
    int sock = -1;
    struct netdata_static_thread *static_thread = (struct netdata_static_thread *)ptr;

    BUFFER *b = buffer_create(1), *response = buffer_create(1);
    int (*backend_request_formatter)(BUFFER *, const char *, RRDHOST *, const char *, RRDSET *, RRDDIM *, time_t, time_t, uint32_t) = NULL;
    int (*backend_response_checker)(BUFFER *) = NULL;

    info("BACKEND: thread created with task id %d", gettid());

    if(pthread_setcanceltype(PTHREAD_CANCEL_DEFERRED, NULL) != 0)
        error("BACKEND: cannot set pthread cancel type to DEFERRED.");

    if(pthread_setcancelstate(PTHREAD_CANCEL_ENABLE, NULL) != 0)
        error("BACKEND: cannot set pthread cancel state to ENABLE.");

    // ------------------------------------------------------------------------
    // collect configuration options

    struct timeval timeout = {
            .tv_sec = 0,
            .tv_usec = 0
    };
    int enabled             = config_get_boolean(CONFIG_SECTION_BACKEND, "enabled", 0);
    const char *source      = config_get(CONFIG_SECTION_BACKEND, "data source", "average");
    const char *type        = config_get(CONFIG_SECTION_BACKEND, "type", "graphite");
    const char *destination = config_get(CONFIG_SECTION_BACKEND, "destination", "localhost");
    backend_prefix          = config_get(CONFIG_SECTION_BACKEND, "prefix", "netdata");
    const char *hostname    = config_get(CONFIG_SECTION_BACKEND, "hostname", localhost->hostname);
    backend_update_every    = (int)config_get_number(CONFIG_SECTION_BACKEND, "update every", backend_update_every);
    int buffer_on_failures  = (int)config_get_number(CONFIG_SECTION_BACKEND, "buffer on failures", 10);
    long timeoutms          = config_get_number(CONFIG_SECTION_BACKEND, "timeout ms", backend_update_every * 2 * 1000);
    backend_send_names      = config_get_boolean(CONFIG_SECTION_BACKEND, "send names instead of ids", backend_send_names);

    charts_pattern = simple_pattern_create(config_get(CONFIG_SECTION_BACKEND, "send charts matching", "*"), SIMPLE_PATTERN_EXACT);


    // ------------------------------------------------------------------------
    // validate configuration options
    // and prepare for sending data to our backend

    backend_options = backend_parse_data_source(source, backend_options);

    if(timeoutms < 1) {
        error("BACKEND: invalid timeout %ld ms given. Assuming %d ms.", timeoutms, backend_update_every * 2 * 1000);
        timeoutms = backend_update_every * 2 * 1000;
    }
    timeout.tv_sec  = (timeoutms * 1000) / 1000000;
    timeout.tv_usec = (timeoutms * 1000) % 1000000;

    if(!enabled || backend_update_every < 1)
        goto cleanup;

    // ------------------------------------------------------------------------
    // select the backend type

    if(!strcmp(type, "graphite") || !strcmp(type, "graphite:plaintext")) {

        default_port = 2003;
        backend_response_checker = process_graphite_response;

        if((backend_options & BACKEND_SOURCE_BITS) == BACKEND_SOURCE_DATA_AS_COLLECTED)
            backend_request_formatter = format_dimension_collected_graphite_plaintext;
        else
            backend_request_formatter = format_dimension_stored_graphite_plaintext;

    }
    else if(!strcmp(type, "opentsdb") || !strcmp(type, "opentsdb:telnet")) {

        default_port = 4242;
        backend_response_checker = process_opentsdb_response;

        if((backend_options & BACKEND_SOURCE_BITS) == BACKEND_SOURCE_DATA_AS_COLLECTED)
            backend_request_formatter = format_dimension_collected_opentsdb_telnet;
        else
            backend_request_formatter = format_dimension_stored_opentsdb_telnet;

    }
    else if (!strcmp(type, "json") || !strcmp(type, "json:plaintext")) {

        default_port = 5448;
        backend_response_checker = process_json_response;

        if ((backend_options & BACKEND_SOURCE_BITS) == BACKEND_SOURCE_DATA_AS_COLLECTED)
            backend_request_formatter = format_dimension_collected_json_plaintext;
        else
            backend_request_formatter = format_dimension_stored_json_plaintext;

    }
    else {
        error("BACKEND: Unknown backend type '%s'", type);
        goto cleanup;
    }

    if(backend_request_formatter == NULL || backend_response_checker == NULL) {
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
            chart_backend_reconnects = 0,
            chart_backend_latency = 0;

    RRDSET *chart_metrics = rrdset_create_localhost("netdata", "backend_metrics", NULL, "backend", NULL, "Netdata Buffered Metrics", "metrics", 130600, backend_update_every, RRDSET_TYPE_LINE);
    rrddim_add(chart_metrics, "buffered", NULL,  1, 1, RRD_ALGORITHM_ABSOLUTE);
    rrddim_add(chart_metrics, "lost",     NULL,  1, 1, RRD_ALGORITHM_ABSOLUTE);
    rrddim_add(chart_metrics, "sent",     NULL,  1, 1, RRD_ALGORITHM_ABSOLUTE);

    RRDSET *chart_bytes = rrdset_create_localhost("netdata", "backend_bytes", NULL, "backend", NULL, "Netdata Backend Data Size", "KB", 130610, backend_update_every, RRDSET_TYPE_AREA);
    rrddim_add(chart_bytes, "buffered", NULL, 1, 1024, RRD_ALGORITHM_ABSOLUTE);
    rrddim_add(chart_bytes, "lost",     NULL, 1, 1024, RRD_ALGORITHM_ABSOLUTE);
    rrddim_add(chart_bytes, "sent",     NULL, 1, 1024, RRD_ALGORITHM_ABSOLUTE);
    rrddim_add(chart_bytes, "received", NULL, 1, 1024, RRD_ALGORITHM_ABSOLUTE);

    RRDSET *chart_ops = rrdset_create_localhost("netdata", "backend_ops", NULL, "backend", NULL, "Netdata Backend Operations", "operations", 130630, backend_update_every, RRDSET_TYPE_LINE);
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
    RRDSET *chart_latency = rrdset_create_localhost("netdata", "backend_latency", NULL, "backend", NULL, "Netdata Backend Latency", "ms", 130620, backend_update_every, RRDSET_TYPE_AREA);
    rrddim_add(chart_latency, "latency",   NULL,  1, 1000, RRD_ALGORITHM_ABSOLUTE);
    */

    RRDSET *chart_rusage = rrdset_create_localhost("netdata", "backend_thread_cpu", NULL, "backend", NULL, "NetData Backend Thread CPU usage", "milliseconds/s", 130630, backend_update_every, RRDSET_TYPE_STACKED);
    rrddim_add(chart_rusage, "user",   NULL, 1, 1000, RRD_ALGORITHM_INCREMENTAL);
    rrddim_add(chart_rusage, "system", NULL, 1, 1000, RRD_ALGORITHM_INCREMENTAL);


    // ------------------------------------------------------------------------
    // prepare the backend main loop

    info("BACKEND: configured ('%s' on '%s' sending '%s' data, every %d seconds, as host '%s', with prefix '%s')", type, destination, source, backend_update_every, hostname, backend_prefix);

    usec_t step_ut = backend_update_every * USEC_PER_SEC;
    time_t after = now_realtime_sec();
    int failures = 0;
    heartbeat_t hb;
    heartbeat_init(&hb);

    for(;;) {

        // ------------------------------------------------------------------------
        // Wait for the next iteration point.

        heartbeat_next(&hb, step_ut);
        time_t before = now_realtime_sec();
        debug(D_BACKEND, "BACKEND: preparing buffer for timeframe %lu to %lu", (unsigned long)after, (unsigned long)before);

        // ------------------------------------------------------------------------
        // add to the buffer the data we need to send to the backend

        int pthreadoldcancelstate;

        if(unlikely(pthread_setcancelstate(PTHREAD_CANCEL_DISABLE, &pthreadoldcancelstate) != 0))
            error("BACKEND: cannot set pthread cancel state to DISABLE.");

        size_t count_hosts = 0;
        size_t count_charts_total = 0;
        size_t count_dims_total = 0;

        rrd_rdlock();
        RRDHOST *host;
        rrdhost_foreach_read(host) {
            rrdhost_rdlock(host);

            count_hosts++;
            size_t count_charts = 0;
            size_t count_dims = 0;
            size_t count_dims_skipped = 0;

            const char *__hostname = (host == localhost)?hostname:host->hostname;

            RRDSET *st;
            rrdset_foreach_read(st, host) {
                if(likely(backends_can_send_rrdset(backend_options, st))) {
                    rrdset_rdlock(st);

                    count_charts++;

                    RRDDIM *rd;
                    rrddim_foreach_read(rd, st) {
                        if (likely(rd->last_collected_time.tv_sec >= after)) {
                            chart_buffered_metrics += backend_request_formatter(b, backend_prefix, host, __hostname, st, rd, after, before, backend_options);
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

            debug(D_BACKEND, "BACKEND: sending host '%s', metrics of %zu dimensions, of %zu charts. Skipped %zu dimensions.", __hostname, count_dims, count_charts, count_dims_skipped);
            count_charts_total += count_charts;
            count_dims_total += count_dims;

            rrdhost_unlock(host);
        }
        rrd_unlock();

        debug(D_BACKEND, "BACKEND: buffer has %zu bytes, added metrics for %zu dimensions, of %zu charts, from %zu hosts", buffer_strlen(b), count_dims_total, count_charts_total, count_hosts);

        if(unlikely(pthread_setcancelstate(pthreadoldcancelstate, NULL) != 0))
            error("BACKEND: cannot set pthread cancel state to RESTORE (%d).", pthreadoldcancelstate);

        // ------------------------------------------------------------------------

        chart_buffered_bytes = (collected_number)buffer_strlen(b);

        // reset the monitoring chart counters
        chart_received_bytes =
        chart_sent_bytes =
        chart_sent_metrics =
        chart_lost_metrics =
        chart_transmission_successes =
        chart_transmission_failures =
        chart_data_lost_events =
        chart_lost_bytes =
        chart_backend_reconnects =
        chart_backend_latency = 0;

        if(unlikely(netdata_exit)) break;

        //fprintf(stderr, "\nBACKEND BEGIN:\n%s\nBACKEND END\n", buffer_tostring(b)); // FIXME
        //fprintf(stderr, "after = %lu, before = %lu\n", after, before);

        // prepare for the next iteration
        // to add incrementally data to buffer
        after = before;

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
            usec_t start_ut = now_monotonic_usec();
            size_t reconnects = 0;

            sock = connect_to_one_of(destination, default_port, &timeout, &reconnects, NULL, 0);

            chart_backend_reconnects += reconnects;
            chart_backend_latency += now_monotonic_usec() - start_ut;
        }

        if(unlikely(netdata_exit)) break;

        // ------------------------------------------------------------------------
        // if we are connected, send our buffer to the backend server

        if(likely(sock != -1)) {
            size_t len = buffer_strlen(b);
            usec_t start_ut = now_monotonic_usec();
            int flags = 0;
#ifdef MSG_NOSIGNAL
            flags += MSG_NOSIGNAL;
#endif

            ssize_t written = send(sock, buffer_tostring(b), len, flags);
            chart_backend_latency += now_monotonic_usec() - start_ut;
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

        if(failures > buffer_on_failures) {
            // too bad! we are going to lose data
            chart_lost_bytes += buffer_strlen(b);
            error("BACKEND: reached %d backend failures. Flushing buffers to protect this host - this results in data loss on back-end server '%s'", failures, destination);
            buffer_flush(b);
            failures = 0;
            chart_data_lost_events++;
            chart_lost_metrics = chart_buffered_metrics;
        }

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
    if(sock != -1)
        close(sock);

    buffer_free(b);
    buffer_free(response);

    info("BACKEND: thread exiting");

    static_thread->enabled = 0;
    pthread_exit(NULL);
    return NULL;
}
