// SPDX-License-Identifier: GPL-3.0-or-later

#define BACKENDS_INTERNALS
#include "graphite.h"

// ----------------------------------------------------------------------------
// graphite backend

int format_dimension_collected_graphite_plaintext(
        BUFFER *b                   // the buffer to write data to
        , const char *prefix        // the prefix to use
        , RRDHOST *host             // the host this chart comes from
        , const char *hostname      // the hostname (to override host->hostname)
        , RRDSET *st                // the chart
        , RRDDIM *rd                // the dimension
        , usec_t after_usec         // the start timestamp
        , usec_t before_usec        // the end timestamp
        , BACKEND_OPTIONS backend_options // BACKEND_SOURCE_* bitmap
) {
    (void)host;
    (void)after_usec;
    (void)before_usec;

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

int format_dimension_stored_graphite_plaintext(
        BUFFER *b                   // the buffer to write data to
        , const char *prefix        // the prefix to use
        , RRDHOST *host             // the host this chart comes from
        , const char *hostname      // the hostname (to override host->hostname)
        , RRDSET *st                // the chart
        , RRDDIM *rd                // the dimension
        , usec_t after_usec         // the start timestamp
        , usec_t before_usec        // the end timestamp
        , BACKEND_OPTIONS backend_options // BACKEND_SOURCE_* bitmap
) {
    (void)host;

    char chart_name[RRD_ID_LENGTH_MAX + 1];
    char dimension_name[RRD_ID_LENGTH_MAX + 1];
    backend_name_copy(chart_name, (backend_options & BACKEND_OPTION_SEND_NAMES && st->name)?st->name:st->id, RRD_ID_LENGTH_MAX);
    backend_name_copy(dimension_name, (backend_options & BACKEND_OPTION_SEND_NAMES && rd->name)?rd->name:rd->id, RRD_ID_LENGTH_MAX);

    usec_t first_usec = after_usec, last_usec = before_usec;
    calculated_number value = backend_calculate_value_from_stored_data(st, rd, after_usec, before_usec, backend_options, &first_usec, &last_usec);

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
                , last_usec / USEC_PER_SEC
        );

        return 1;
    }
    return 0;
}

int process_graphite_response(BUFFER *b) {
    return discard_response(b, "graphite");
}


