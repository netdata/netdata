// SPDX-License-Identifier: GPL-3.0-or-later

#define BACKENDS_INTERNALS
#include "opentsdb.h"

// ----------------------------------------------------------------------------
// opentsdb backend

int format_dimension_collected_opentsdb_telnet(
        BUFFER *b                 // the buffer to write data to
        , const char *prefix        // the prefix to use
        , RRDHOST *host             // the host this chart comes from
        , const char *hostname      // the hostname (to override host->hostname)
        , RRDSET *st                // the chart
        , RRDDIM *rd                // the dimension
        , time_t after              // the start timestamp
        , time_t before             // the end timestamp
        , BACKEND_OPTIONS backend_options // BACKEND_SOURCE_* bitmap
) {
    (void)host;
    (void)after;
    (void)before;

    char chart_name[RRD_ID_LENGTH_MAX + 1];
    char dimension_name[RRD_ID_LENGTH_MAX + 1];
    backend_name_copy(chart_name, (backend_options & BACKEND_OPTION_SEND_NAMES && st->name)?st->name:st->id, RRD_ID_LENGTH_MAX);
    backend_name_copy(dimension_name, (backend_options & BACKEND_OPTION_SEND_NAMES && rd->name)?rd->name:rd->id, RRD_ID_LENGTH_MAX);

    buffer_sprintf(
            b
            , "put %s.%s.%s %llu " COLLECTED_NUMBER_FORMAT " host=%s%s%s\n"
            , prefix
            , chart_name
            , dimension_name
            , (unsigned long long)rd->last_collected_time.tv_sec
            , rd->last_collected_value
            , hostname
            , (host->tags)?" ":""
            , (host->tags)?host->tags:""
    );

    return 1;
}

int format_dimension_stored_opentsdb_telnet(
        BUFFER *b                 // the buffer to write data to
        , const char *prefix        // the prefix to use
        , RRDHOST *host             // the host this chart comes from
        , const char *hostname      // the hostname (to override host->hostname)
        , RRDSET *st                // the chart
        , RRDDIM *rd                // the dimension
        , time_t after              // the start timestamp
        , time_t before             // the end timestamp
        , BACKEND_OPTIONS backend_options // BACKEND_SOURCE_* bitmap
) {
    (void)host;

    time_t first_t = after, last_t = before;
    calculated_number value = backend_calculate_value_from_stored_data(st, rd, after, before, backend_options, &first_t, &last_t);

    char chart_name[RRD_ID_LENGTH_MAX + 1];
    char dimension_name[RRD_ID_LENGTH_MAX + 1];
    backend_name_copy(chart_name, (backend_options & BACKEND_OPTION_SEND_NAMES && st->name)?st->name:st->id, RRD_ID_LENGTH_MAX);
    backend_name_copy(dimension_name, (backend_options & BACKEND_OPTION_SEND_NAMES && rd->name)?rd->name:rd->id, RRD_ID_LENGTH_MAX);

    if(!isnan(value)) {

        buffer_sprintf(
                b
                , "put %s.%s.%s %llu " CALCULATED_NUMBER_FORMAT " host=%s%s%s\n"
                , prefix
                , chart_name
                , dimension_name
                , (unsigned long long) last_t
                , value
                , hostname
                , (host->tags)?" ":""
                , (host->tags)?host->tags:""
        );

        return 1;
    }
    return 0;
}

int process_opentsdb_response(BUFFER *b) {
    return discard_response(b, "opentsdb");
}

int format_dimension_collected_opentsdb_http(
        BUFFER *b                 // the buffer to write data to
        , const char *prefix        // the prefix to use
        , RRDHOST *host             // the host this chart comes from
        , const char *hostname      // the hostname (to override host->hostname)
        , RRDSET *st                // the chart
        , RRDDIM *rd                // the dimension
        , time_t after              // the start timestamp
        , time_t before             // the end timestamp
        , BACKEND_OPTIONS backend_options // BACKEND_SOURCE_* bitmap
) {
    (void)host;
    (void)after;
    (void)before;

    char protocol[8];
    memcpy(protocol,"http",4);
    int end = 4;
#ifdef ENABLE_HTTPS
    if ( netdata_srv_ctx ) {
        protocol[end++] = 's';
    }
#endif
    protocol[end] = 0x00;

    char message[1024];

    int length =  snprintfz(message, sizeof(message),"{ }");

    buffer_sprintf(
            b
            , "POST %s://%s/api/put HTTP/1.1\r\n"
              "Host: %s\r\n"
              "Content-Type: application/json\r\n"
              "Content-Length: %d\r\n"
              "\r\n"
              "\"timestamp\": %llu}\n"
            , protocol
            , hostname
            , hostname
            , length
            , (unsigned long long) rd->last_collected_time.tv_sec);

    return 1;
}


int format_dimension_stored_opentsdb_http(
        BUFFER *b                 // the buffer to write data to
        , const char *prefix        // the prefix to use
        , RRDHOST *host             // the host this chart comes from
        , const char *hostname      // the hostname (to override host->hostname)
        , RRDSET *st                // the chart
        , RRDDIM *rd                // the dimension
        , time_t after              // the start timestamp
        , time_t before             // the end timestamp
        , BACKEND_OPTIONS backend_options // BACKEND_SOURCE_* bitmap
) {
    (void)host;

    time_t first_t = after, last_t = before;
    calculated_number value = backend_calculate_value_from_stored_data(st, rd, after, before, backend_options, &first_t, &last_t);

    if(!isnan(value)) {
        const char *tags_pre = "", *tags_post = "", *tags = host->tags;
        if (!tags) tags = "";

        if (*tags) {
            if (*tags == '{' || *tags == '[' || *tags == '"') {
                tags_pre = "\"host_tags\":";
                tags_post = ",";
            } else {
                tags_pre = "\"host_tags\":\"";
                tags_post = "\",";
            }
        }

        char protocol[8];
        memcpy(protocol,"http",4);
        int end = 4;
#ifdef ENABLE_HTTPS
        if ( netdata_srv_ctx ) {
            protocol[end++] = 's';
        }
#endif
        protocol[end] = 0x00;

        buffer_sprintf(
                b
                , "POST %s://%s/api/put HTTP/1.1\r\n"
                  "Host: %s\r\n"
                  "Content-Type: application/json\r\n"
                  "Content-Length: %llu\r\n"
                  "\r\n"
                  "{"
                  "\"prefix\":\"%s\","
                  "\"hostname\":\"%s\","
                  "%s%s%s"

                  "\"chart_id\":\"%s\","
                  "\"chart_name\":\"%s\","
                  "\"chart_family\":\"%s\","
                  "\"chart_context\": \"%s\","
                  "\"chart_type\":\"%s\","
                  "\"units\": \"%s\","

                  "\"id\":\"%s\","
                  "\"name\":\"%s\","
                  "\"value\":" COLLECTED_NUMBER_FORMAT ","

                  "\"timestamp\": %llu}\n",
                 protocol
                , hostname
                , hostname
                , prefix
                , hostname
                , tags_pre, tags, tags_post
                , st->id
                , st->name
                , st->family
                , st->context
                , st->type
                , st->units
                , rd->id
                , rd->name
                , rd->last_collected_value);

        return 1;
    }

    return 0;
}