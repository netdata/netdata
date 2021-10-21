// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_BACKEND_JSON_H
#define NETDATA_BACKEND_JSON_H

#include "backends/backends.h"

extern int backends_format_dimension_collected_json_plaintext(
          BUFFER *b                 // the buffer to write data to
        , const char *prefix        // the prefix to use
        , RRDHOST *host             // the host this chart comes from
        , const char *hostname      // the hostname (to override host->hostname)
        , RRDSET *st                // the chart
        , RRDDIM *rd                // the dimension
        , time_t after              // the start timestamp
        , time_t before             // the end timestamp
        , BACKEND_OPTIONS backend_options // BACKEND_SOURCE_* bitmap
);

extern int backends_format_dimension_stored_json_plaintext(
          BUFFER *b                 // the buffer to write data to
        , const char *prefix        // the prefix to use
        , RRDHOST *host             // the host this chart comes from
        , const char *hostname      // the hostname (to override host->hostname)
        , RRDSET *st                // the chart
        , RRDDIM *rd                // the dimension
        , time_t after              // the start timestamp
        , time_t before             // the end timestamp
        , BACKEND_OPTIONS backend_options // BACKEND_SOURCE_* bitmap
);

extern int process_json_response(BUFFER *b);

#endif //NETDATA_BACKEND_JSON_H
