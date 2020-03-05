// SPDX-License-Identifier: GPL-3.0-or-later

#define BACKENDS_INTERNALS
#include "json.h"

// ----------------------------------------------------------------------------
// json backend

int backends_format_dimension_collected_json_plaintext(
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

int backends_format_dimension_stored_json_plaintext(
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

                (unsigned long long) last_t
        );

        return 1;
    }
    return 0;
}

int process_json_response(BUFFER *b) {
    return discard_response(b, "json");
}


