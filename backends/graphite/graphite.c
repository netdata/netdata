// SPDX-License-Identifier: GPL-3.0-or-later

#define BACKENDS_INTERNALS
#include "graphite.h"

// ----------------------------------------------------------------------------
// graphite backend

int backends_format_dimension_collected_graphite_plaintext(
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
            , "%s.%s.%s.%s%s%s " COLLECTED_NUMBER_FORMAT " %llu\n"
            , prefix
            , hostname
            , chart_name
            , dimension_name
            , (host->tags)?";":""
            , (host->tags)?host->tags:""
            , rd->last_collected_value
            , (unsigned long long)rd->last_collected_time.tv_sec
    );

    return 1;
}

int backends_format_dimension_stored_graphite_plaintext(
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

    char chart_name[RRD_ID_LENGTH_MAX + 1];
    char dimension_name[RRD_ID_LENGTH_MAX + 1];
    backend_name_copy(chart_name, (backend_options & BACKEND_OPTION_SEND_NAMES && st->name)?st->name:st->id, RRD_ID_LENGTH_MAX);
    backend_name_copy(dimension_name, (backend_options & BACKEND_OPTION_SEND_NAMES && rd->name)?rd->name:rd->id, RRD_ID_LENGTH_MAX);

    time_t first_t = after, last_t = before;
    calculated_number value = backend_calculate_value_from_stored_data(st, rd, after, before, backend_options, &first_t, &last_t);

    if(!isnan(value)) {

        buffer_sprintf(
                b
                , "%s.%s.%s.%s%s%s " CALCULATED_NUMBER_FORMAT " %llu\n"
                , prefix
                , hostname
                , chart_name
                , dimension_name
                , (host->tags)?";":""
                , (host->tags)?host->tags:""
                , value
                , (unsigned long long) last_t
        );

        return 1;
    }
    return 0;
}

int process_graphite_response(BUFFER *b) {
    return discard_response(b, "graphite");
}


