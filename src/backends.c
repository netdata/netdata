#include "common.h"

#define BACKEND_SOURCE_DATA_AS_COLLECTED 0x00000001
#define BACKEND_SOURCE_DATA_AVERAGE      0x00000002
#define BACKEND_SOURCE_DATA_SUM          0x00000004

int connect_to_socket4(const char *ip, int port, struct timeval *timeout) {
    int sock;

    debug(D_LISTENER, "IPv4 connecting to ip '%s' port %d", ip, port);

    sock = socket(AF_INET, SOCK_STREAM, 0);
    if(sock < 0) {
        error("IPv4 socket() on ip '%s' port %d failed.", ip, port);
        return -1;
    }

    if(setsockopt(sock, SOL_SOCKET, SO_SNDTIMEO, (char *)timeout, sizeof(struct timeval)) < 0)
        error("Failed to set timeout on the socket to ip '%s' port %d", ip, port);

    struct sockaddr_in name;
    memset(&name, 0, sizeof(struct sockaddr_in));
    name.sin_family = AF_INET;
    name.sin_port = htons(port);

    int ret = inet_pton(AF_INET, ip, (void *)&name.sin_addr.s_addr);
    if(ret != 1) {
        error("Failed to convert '%s' to a valid IPv4 address.", ip);
        close(sock);
        return -1;
    }

    if(connect(sock, (struct sockaddr *) &name, sizeof(name)) < 0) {
        close(sock);
        error("IPv4 failed to connect to '%s', port %d", ip, port);
        return -1;
    }

    debug(D_LISTENER, "Connected to IPv4 ip '%s' port %d", ip, port);
    return sock;
}

int connect_to_socket6(const char *ip, int port, struct timeval *timeout) {
    int sock = -1;
    int ipv6only = 1;

    debug(D_LISTENER, "IPv6 connecting to ip '%s' port %d", ip, port);

    sock = socket(AF_INET6, SOCK_STREAM, 0);
    if (sock < 0) {
        error("IPv6 socket() on ip '%s' port %d failed.", ip, port);
        return -1;
    }

    if(setsockopt(sock, SOL_SOCKET, SO_SNDTIMEO, (char *)timeout, sizeof(struct timeval)) < 0)
        error("Failed to set timeout on the socket to ip '%s' port %d", ip, port);

    /* IPv6 only */
    if(setsockopt(sock, IPPROTO_IPV6, IPV6_V6ONLY, (void*)&ipv6only, sizeof(ipv6only)) != 0)
        error("Cannot set IPV6_V6ONLY on ip '%s' port's %d.", ip, port);

    struct sockaddr_in6 name;
    memset(&name, 0, sizeof(struct sockaddr_in6));
    name.sin6_family = AF_INET6;
    name.sin6_port = htons ((uint16_t) port);

    int ret = inet_pton(AF_INET6, ip, (void *)&name.sin6_addr.s6_addr);
    if(ret != 1) {
        error("Failed to convert IP '%s' to a valid IPv6 address.", ip);
        close(sock);
        return -1;
    }

    name.sin6_scope_id = 0;

    if(connect(sock, (struct sockaddr *)&name, sizeof(name)) < 0) {
        close(sock);
        error("IPv6 failed to connect to '%s', port %d", ip, port);
        return -1;
    }

    debug(D_LISTENER, "Connected to IPv6 ip '%s' port %d", ip, port);
    return sock;
}


static inline int connect_to_one(const char *definition, int default_port, struct timeval *timeout) {
    struct addrinfo hints;
    struct addrinfo *result = NULL, *rp = NULL;

    char buffer[strlen(definition) + 1];
    strcpy(buffer, definition);

    char buffer2[10 + 1];
    snprintfz(buffer2, 10, "%d", default_port);

    char *ip = buffer, *port = buffer2;

    char *e = ip;
    if(*e == '[') {
        e = ++ip;
        while(*e && *e != ']') e++;
        if(*e == ']') {
            *e = '\0';
            e++;
        }
    }
    else {
        while(*e && *e != ':') e++;
    }

    if(*e == ':') {
        port = e + 1;
        *e = '\0';
    }

    if(!*ip)
        return -1;

    if(!*port)
        port = buffer2;

    memset(&hints, 0, sizeof(struct addrinfo));
    hints.ai_family = AF_UNSPEC;    /* Allow IPv4 or IPv6 */
    hints.ai_socktype = SOCK_DGRAM; /* Datagram socket */
    hints.ai_flags = AI_PASSIVE;    /* For wildcard IP address */
    hints.ai_protocol = 0;          /* Any protocol */
    hints.ai_canonname = NULL;
    hints.ai_addr = NULL;
    hints.ai_next = NULL;

    int r = getaddrinfo(ip, port, &hints, &result);
    if (r != 0) {
        error("Cannot resolve host '%s', port '%s': %s\n", ip, port, gai_strerror(r));
        return -1;
    }

    int fd = -1;
    for (rp = result; rp != NULL && fd == -1; rp = rp->ai_next) {
        char rip[INET_ADDRSTRLEN + INET6_ADDRSTRLEN] = "INVALID";
        int rport;

        switch (rp->ai_addr->sa_family) {
            case AF_INET: {
                struct sockaddr_in *sin = (struct sockaddr_in *) rp->ai_addr;
                inet_ntop(AF_INET, &sin->sin_addr, rip, INET_ADDRSTRLEN);
                rport = ntohs(sin->sin_port);
                fd = connect_to_socket4(rip, rport, timeout);
                break;
            }

            case AF_INET6: {
                struct sockaddr_in6 *sin6 = (struct sockaddr_in6 *) rp->ai_addr;
                inet_ntop(AF_INET6, &sin6->sin6_addr, rip, INET6_ADDRSTRLEN);
                rport = ntohs(sin6->sin6_port);
                fd = connect_to_socket6(rip, rport, timeout);
                break;
            }
        }
    }

    freeaddrinfo(result);

    return fd;
}

static inline calculated_number backend_calculate_value_from_stored_data(RRDSET *st, RRDDIM *rd, time_t after, time_t before, uint32_t options) {
    time_t first_t = rrdset_first_entry_t(st);
    time_t last_t = rrdset_last_entry_t(st);

    if(unlikely(before - after < st->update_every && after != after - after % st->update_every))
        // when st->update_every is bigger than the frequency we send data to backend
        // skip the iterations that are not aligned to the database
        return NAN;

    // align the time-frame
    // for 'after' also skip the first value by adding st->update_every
    after  = after  - after  % st->update_every + st->update_every;
    before = before - before % st->update_every;

    if(unlikely(after < first_t))
        after = first_t;

    if(unlikely(after > before))
        // this can happen when the st->update_every > before - after
        before = after;

    if(unlikely(before > last_t))
        before = last_t;

    size_t counter = 0;
    calculated_number sum = 0;

    long    start_at_slot = rrdset_time2slot(st, before),
            stop_at_slot  = rrdset_time2slot(st, after),
            slot, stop_now = 0;

    for(slot = start_at_slot; !stop_now ; slot--) {
        if(unlikely(slot < 0)) slot = st->entries - 1;
        if(unlikely(slot == stop_at_slot)) stop_now = 1;

        storage_number n = rd->values[slot];
        if(unlikely(!does_storage_number_exist(n))) continue;

        calculated_number value = unpack_storage_number(n);
        sum += value;
        counter++;
    }

    if(unlikely(!counter))
        return NAN;

    if(unlikely(options & BACKEND_SOURCE_DATA_SUM))
        return sum;

    return sum / (calculated_number)counter;
}

static inline int format_dimension_collected_graphite_plaintext(BUFFER *b, const char *prefix, RRDHOST *host, const char *hostname, RRDSET *st, RRDDIM *rd, time_t after, time_t before, uint32_t options) {
    (void)host;
    (void)after;
    (void)before;
    (void)options;
    buffer_sprintf(b, "%s.%s.%s.%s " COLLECTED_NUMBER_FORMAT " %u\n", prefix, hostname, st->id, rd->id, rd->last_collected_value, (uint32_t)rd->last_collected_time.tv_sec);
    return 1;
}

static inline int format_dimension_stored_graphite_plaintext(BUFFER *b, const char *prefix, RRDHOST *host, const char *hostname, RRDSET *st, RRDDIM *rd, time_t after, time_t before, uint32_t options) {
    (void)host;
    calculated_number value = backend_calculate_value_from_stored_data(st, rd, after, before, options);
    if(!isnan(value)) {
        buffer_sprintf(b, "%s.%s.%s.%s " CALCULATED_NUMBER_FORMAT " %u\n", prefix, hostname, st->id, rd->id, value, (uint32_t) before);
        return 1;
    }
    return 0;
}

static inline int format_dimension_collected_opentsdb_telnet(BUFFER *b, const char *prefix, RRDHOST *host, const char *hostname, RRDSET *st, RRDDIM *rd, time_t after, time_t before, uint32_t options) {
    (void)host;
    (void)after;
    (void)before;
    (void)options;
    buffer_sprintf(b, "put %s.%s.%s %u " COLLECTED_NUMBER_FORMAT " host=%s\n", prefix, st->id, rd->id, (uint32_t)rd->last_collected_time.tv_sec, rd->last_collected_value, hostname);
    return 1;
}

static inline int format_dimension_stored_opentsdb_telnet(BUFFER *b, const char *prefix, RRDHOST *host, const char *hostname, RRDSET *st, RRDDIM *rd, time_t after, time_t before, uint32_t options) {
    (void)host;
    calculated_number value = backend_calculate_value_from_stored_data(st, rd, after, before, options);
    if(!isnan(value)) {
        buffer_sprintf(b, "put %s.%s.%s %u " CALCULATED_NUMBER_FORMAT " host=%s\n", prefix, st->id, rd->id, (uint32_t) before, value, hostname);
        return 1;
    }
    return 0;
}

void *backends_main(void *ptr) {
    (void)ptr;

    BUFFER *b = buffer_create(1);
    int (*formatter)(BUFFER *b, const char *prefix, RRDHOST *host, const char *hostname, RRDSET *st, RRDDIM *rd, time_t after, time_t before, uint32_t options);

    info("BACKEND thread created with task id %d", gettid());

    if(pthread_setcanceltype(PTHREAD_CANCEL_DEFERRED, NULL) != 0)
        error("Cannot set pthread cancel type to DEFERRED.");

    if(pthread_setcancelstate(PTHREAD_CANCEL_ENABLE, NULL) != 0)
        error("Cannot set pthread cancel state to ENABLE.");

    // ------------------------------------------------------------------------
    // collect configuration options

    struct timeval timeout = {
            .tv_sec = 0,
            .tv_usec = 0
    };
    int default_port = 0;
    int sock = -1;
    uint32_t options;
    int enabled = config_get_boolean("backend", "enabled", 0);
    const char *source = config_get("backend", "data source", "average");
    const char *type = config_get("backend", "type", "graphite");
    const char *destination = config_get("backend", "destination", "localhost");
    const char *prefix = config_get("backend", "prefix", "netdata");
    const char *hostname = config_get("backend", "hostname", localhost.hostname);
    int frequency = (int)config_get_number("backend", "update every", 10);
    int buffer_on_failures = (int)config_get_number("backend", "buffer on failures", 10);
    long timeoutms = config_get_number("backend", "timeout ms", frequency * 2 * 1000);

    // ------------------------------------------------------------------------
    // validate configuration options
    // and prepare for sending data to our backend
    if(!enabled || frequency < 1)
        goto cleanup;

    if(!strcmp(source, "as collected")) {
        options = BACKEND_SOURCE_DATA_AS_COLLECTED;
    }
    else if(!strcmp(source, "average")) {
        options = BACKEND_SOURCE_DATA_AVERAGE;
    }
    else if(!strcmp(source, "sum") || !strcmp(source, "volume")) {
        options = BACKEND_SOURCE_DATA_SUM;
    }
    else {
        error("Invalid data source method '%s' for backend given. Disabling backed.", source);
        goto cleanup;
    }

    if(!strcmp(type, "graphite") || !strcmp(type, "graphite:plaintext")) {
        default_port = 2003;
        if(options == BACKEND_SOURCE_DATA_AS_COLLECTED)
            formatter = format_dimension_collected_graphite_plaintext;
        else
            formatter = format_dimension_stored_graphite_plaintext;
    }
    else if(!strcmp(type, "opentsdb") || !strcmp(type, "opentsdb:telnet")) {
        default_port = 4242;
        if(options == BACKEND_SOURCE_DATA_AS_COLLECTED)
            formatter = format_dimension_collected_opentsdb_telnet;
        else
            formatter = format_dimension_stored_opentsdb_telnet;
    }
    else {
        error("Unknown backend type '%s'", type);
        goto cleanup;
    }

    if(timeoutms < 1) {
        error("BACKED invalid timeout %ld ms given. Assuming %d ms.", timeoutms, frequency * 2 * 1000);
        timeoutms = frequency * 2 * 1000;
    }
    timeout.tv_sec  = (timeoutms * 1000) / 1000000;
    timeout.tv_usec = (timeoutms * 1000) % 1000000;

    // ------------------------------------------------------------------------
    // prepare the charts for monitoring the backend

    struct rusage thread;

    collected_number
            chart_buffered_metrics = 0,
            chart_lost_metrics = 0,
            chart_sent_metrics = 0,
            chart_buffered_bytes = 0,
            chart_sent_bytes = 0,
            chart_transmission_successes = 0,
            chart_transmission_failures = 0,
            chart_data_lost_events = 0,
            chart_lost_bytes = 0,
            chart_backend_reconnects = 0,
            chart_backend_latency = 0;

    RRDSET *chart_metrics = rrdset_find("netdata.backend_metrics");
    if(!chart_metrics) {
        chart_metrics = rrdset_create("netdata", "backend_metrics", NULL, "backend", NULL, "Netdata Buffered Metrics", "metrics", 130600, frequency, RRDSET_TYPE_LINE);
        rrddim_add(chart_metrics, "buffered", NULL,   1, 1, RRDDIM_ABSOLUTE);
        rrddim_add(chart_metrics, "lost",     NULL,   1, 1, RRDDIM_ABSOLUTE);
        rrddim_add(chart_metrics, "sent",     NULL,   1, 1, RRDDIM_ABSOLUTE);
    }

    RRDSET *chart_bytes = rrdset_find("netdata.backend_bytes");
    if(!chart_bytes) {
        chart_bytes = rrdset_create("netdata", "backend_bytes", NULL, "backend", NULL, "Netdata Backend Data Size", "KB", 130610, frequency, RRDSET_TYPE_AREA);
        rrddim_add(chart_bytes, "buffered", NULL,  1, 1024, RRDDIM_ABSOLUTE);
        rrddim_add(chart_bytes, "lost",     NULL,  1, 1024, RRDDIM_ABSOLUTE);
        rrddim_add(chart_bytes, "sent",     NULL,  1, 1024, RRDDIM_ABSOLUTE);
    }

    RRDSET *chart_ops = rrdset_find("netdata.backend_ops");
    if(!chart_ops) {
        chart_ops = rrdset_create("netdata", "backend_ops", NULL, "backend", NULL, "Netdata Backend Operations", "operations", 130630, frequency, RRDSET_TYPE_LINE);
        rrddim_add(chart_ops, "write",     NULL,  1, 1, RRDDIM_ABSOLUTE);
        rrddim_add(chart_ops, "discard",   NULL,  1, 1, RRDDIM_ABSOLUTE);
        rrddim_add(chart_ops, "reconnect", NULL,  1, 1, RRDDIM_ABSOLUTE);
        rrddim_add(chart_ops, "failure",   NULL,  1, 1, RRDDIM_ABSOLUTE);
    }

    RRDSET *chart_latency = rrdset_find("netdata.backend_latency");
    if(!chart_latency) {
        chart_latency = rrdset_create("netdata", "backend_latency", NULL, "backend", NULL, "Netdata Backend Latency", "ms", 130620, frequency, RRDSET_TYPE_AREA);
        rrddim_add(chart_latency, "latency",   NULL,  1, 1000, RRDDIM_ABSOLUTE);
    }

    RRDSET *chart_rusage = rrdset_find("netdata.backend_thread_cpu");
    if(!chart_rusage) {
        chart_rusage = rrdset_create("netdata", "backend_thread_cpu", NULL, "backend", NULL, "NetData Backend Thread CPU usage", "milliseconds/s", 130630, frequency, RRDSET_TYPE_STACKED);
        rrddim_add(chart_rusage, "user",   NULL, 1, 1000, RRDDIM_INCREMENTAL);
        rrddim_add(chart_rusage, "system", NULL, 1, 1000, RRDDIM_INCREMENTAL);
    }

    // ------------------------------------------------------------------------
    // prepare the backend main loop

    info("BACKEND configured ('%s' on '%s' sending '%s' data, every %d seconds, as host '%s', with prefix '%s')", type, destination, source, frequency, hostname, prefix);

    usec_t step_ut = frequency * USEC_PER_SEC;
    usec_t random_ut = now_realtime_usec() % (step_ut / 2);
    time_t before = (time_t)((now_realtime_usec() - step_ut) / USEC_PER_SEC);
    time_t after = before;
    int failures = 0;

    for(;;) {
        // ------------------------------------------------------------------------
        // wait for the next iteration point

        usec_t now_ut = now_realtime_usec();
        usec_t next_ut = now_ut - (now_ut % step_ut) + step_ut;
        before = (time_t)(next_ut / USEC_PER_SEC);

        // add a little delay (1/4 of the step) plus some randomness
        next_ut += (step_ut / 4) + random_ut;

        while(now_ut < next_ut) {
            sleep_usec(next_ut - now_ut);
            now_ut = now_realtime_usec();
        }

        // ------------------------------------------------------------------------
        // add to the buffer the data we need to send to the backend

        RRDSET *st;
        int pthreadoldcancelstate;

        if(unlikely(pthread_setcancelstate(PTHREAD_CANCEL_DISABLE, &pthreadoldcancelstate) != 0))
            error("Cannot set pthread cancel state to DISABLE.");

        rrdhost_rdlock(&localhost);
        for(st = localhost.rrdset_root; st ;st = st->next) {
            pthread_rwlock_rdlock(&st->rwlock);

            RRDDIM *rd;
            for(rd = st->dimensions; rd ;rd = rd->next) {
                if(rd->last_collected_time.tv_sec >= after)
                    chart_buffered_metrics += formatter(b, prefix, &localhost, hostname, st, rd, after, before, options);
            }

            pthread_rwlock_unlock(&st->rwlock);
        }
        rrdhost_unlock(&localhost);

        if(unlikely(pthread_setcancelstate(pthreadoldcancelstate, NULL) != 0))
            error("Cannot set pthread cancel state to RESTORE (%d).", pthreadoldcancelstate);

        chart_buffered_bytes = (collected_number)buffer_strlen(b);

        // reset the monitoring chart counters
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

        // ------------------------------------------------------------------------
        // connect to a backend server

        if(unlikely(sock == -1)) {
            usec_t start_ut = now_realtime_usec();
            const char *s = destination;
            while(*s) {
                const char *e = s;

                // skip separators, moving both s(tart) and e(nd)
                while(isspace(*e) || *e == ',') s = ++e;

                // move e(nd) to the first separator
                while(*e && !isspace(*e) && *e != ',') e++;

                // is there anything?
                if(!*s || s == e) break;

                char buf[e - s + 1];
                strncpyz(buf, s, e - s);
                chart_backend_reconnects++;
                sock = connect_to_one(buf, default_port, &timeout);
                if(sock != -1) break;
                s = e;
            }
            chart_backend_latency += now_realtime_usec() - start_ut;
        }

        if(unlikely(netdata_exit)) break;

        // ------------------------------------------------------------------------
        // send our buffer to the backend server

        if(likely(sock != -1)) {
            size_t len = buffer_strlen(b);
            usec_t start_ut = now_realtime_usec();
            int flags = 0;
#ifdef MSG_NOSIGNAL
            flags += MSG_NOSIGNAL;
#endif
            ssize_t written = send(sock, buffer_tostring(b), len, flags);
            chart_backend_latency += now_realtime_usec() - start_ut;
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
                error("Failed to write data to database backend '%s'. Willing to write %zu bytes, wrote %zd bytes. Will re-connect.", destination, len, written);
                chart_transmission_failures++;

                if(written != -1)
                    chart_sent_bytes += written;

                // increment the counter we check for data loss
                failures++;

                // close the socket - we will re-open it next time
                close(sock);
                sock = -1;
            }

            // either the buffer is empty
            // or is holding the data we couldn't send
            // so, make sure the next iteration will continue
            // from where we are now
            after = before;
        }
        else {
            error("Failed to update database backend '%s'", destination);
            chart_transmission_failures++;

            // increment the counter we check for data loss
            failures++;
        }

        if(failures > buffer_on_failures) {
            // too bad! we are going to lose data
            chart_lost_bytes += buffer_strlen(b);
            error("Reached %d backend failures. Flushing buffers to protect this host - this results in data loss on back-end server '%s'", failures, destination);
            buffer_flush(b);
            failures = 0;
            chart_data_lost_events++;
            chart_lost_metrics = chart_buffered_metrics;
        }

        if(unlikely(netdata_exit)) break;

        // ------------------------------------------------------------------------
        // update the monitoring charts

        if(chart_ops->counter_done) rrdset_next(chart_ops);
        rrddim_set(chart_ops, "write",        chart_transmission_successes);
        rrddim_set(chart_ops, "discard",      chart_data_lost_events);
        rrddim_set(chart_ops, "failure",      chart_transmission_failures);
        rrddim_set(chart_ops, "reconnect",    chart_backend_reconnects);
        rrdset_done(chart_ops);

        if(chart_metrics->counter_done) rrdset_next(chart_metrics);
        rrddim_set(chart_metrics, "buffered", chart_buffered_metrics);
        rrddim_set(chart_metrics, "lost",     chart_lost_metrics);
        rrddim_set(chart_metrics, "sent",     chart_sent_metrics);
        rrdset_done(chart_metrics);

        if(chart_bytes->counter_done) rrdset_next(chart_bytes);
        rrddim_set(chart_bytes, "buffered",   chart_buffered_bytes);
        rrddim_set(chart_bytes, "lost",       chart_lost_bytes);
        rrddim_set(chart_bytes, "sent",       chart_sent_bytes);
        rrdset_done(chart_bytes);

        if(chart_latency->counter_done) rrdset_next(chart_latency);
        rrddim_set(chart_latency, "latency",  chart_backend_latency);
        rrdset_done(chart_latency);

        getrusage(RUSAGE_THREAD, &thread);
        if(chart_rusage->counter_done) rrdset_next(chart_rusage);
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

    info("BACKEND thread exiting");

    pthread_exit(NULL);
    return NULL;
}
