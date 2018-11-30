// SPDX-License-Identifier: GPL-3.0-or-later

#define BACKENDS_INTERNALS
#include "json.h"

// ----------------------------------------------------------------------------
// json backend

int format_dimension_collected_json_plaintext(
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
    (void)backend_options;

    const char *tags_pre = "", *tags_post = "", *tags = host->tags;
    if(!tags) tags = "";

    if(*tags) {
        if(*tags == '{' || *tags == '[' || *tags == '"') {
            tags_pre = "\"host_tags\":";
            tags_post = ",";
        }
        else {
            tags_pre = "\"host_tags\":\"";
            tags_post = "\",";
        }
    }

    buffer_sprintf(b, "{"
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
            prefix,
            hostname,
            tags_pre, tags, tags_post,

            st->id,
            st->name,
            st->family,
            st->context,
            st->type,
            st->units,

            rd->id,
            rd->name,
            rd->last_collected_value,

            (unsigned long long) rd->last_collected_time.tv_sec
    );

    return 1;
}

int format_dimension_stored_json_plaintext(
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

    usec_t first_usec = after_usec, last_usec = before_usec;
    calculated_number value = backend_calculate_value_from_stored_data(st, rd, after_usec, before_usec, backend_options, &first_usec, &last_usec);

    if(!isnan(value)) {
        const char *tags_pre = "", *tags_post = "", *tags = host->tags;
        if(!tags) tags = "";

        if(*tags) {
            if(*tags == '{' || *tags == '[' || *tags == '"') {
                tags_pre = "\"host_tags\":";
                tags_post = ",";
            }
            else {
                tags_pre = "\"host_tags\":\"";
                tags_post = "\",";
            }
        }

        buffer_sprintf(b, "{"
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
                          "\"value\":" CALCULATED_NUMBER_FORMAT ","

                          "\"timestamp\": %llu}\n",
                prefix,
                hostname,
                tags_pre, tags, tags_post,

                st->id,
                st->name,
                st->family,
                st->context,
                st->type,
                st->units,

                rd->id,
                rd->name,
                value,

                last_usec / USEC_PER_SEC
        );

        return 1;
    }
    return 0;
}

int process_json_response(BUFFER *b) {
    return discard_response(b, "json");
}


