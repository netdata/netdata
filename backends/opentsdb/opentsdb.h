// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_BACKEND_OPENTSDB_H
#define NETDATA_BACKEND_OPENTSDB_H

#include "backends/backends.h"

extern int backends_format_dimension_collected_opentsdb_telnet(
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

extern int backends_format_dimension_stored_opentsdb_telnet(
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

extern int process_opentsdb_response(BUFFER *b);

int backends_format_dimension_collected_opentsdb_http(
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

int backends_format_dimension_stored_opentsdb_http(
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

#endif //NETDATA_BACKEND_OPENTSDB_H
