// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_BACKEND_KINESIS_H
#define NETDATA_BACKEND_KINESIS_H

#include "backends/backends.h"
#include "aws_kinesis_put_record.h"

#define KINESIS_PARTITION_KEY_MAX 256
#define KINESIS_RECORD_MAX 1024 * 1024

extern int read_kinesis_conf(const char *path, char **auth_key_id_p, char **secure_key_p, char **stream_name_p);

extern int format_dimension_collected_kinesis_plaintext(
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

extern int format_dimension_stored_kinesis_plaintext(
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

extern int process_kinesis_response(BUFFER *b);

#endif //NETDATA_BACKEND_KINESIS_H
