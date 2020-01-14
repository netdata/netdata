// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_BACKENDS_H
#define NETDATA_BACKENDS_H 1

#include "daemon/common.h"

typedef enum backend_options {
    BACKEND_OPTION_NONE              = 0,

    BACKEND_SOURCE_DATA_AS_COLLECTED = (1 << 0),
    BACKEND_SOURCE_DATA_AVERAGE      = (1 << 1),
    BACKEND_SOURCE_DATA_SUM          = (1 << 2),

    BACKEND_OPTION_SEND_NAMES        = (1 << 16)
} BACKEND_OPTIONS;

typedef enum backend_types {
    BACKEND_TYPE_UNKNOWN,                   // Invalid type
    BACKEND_TYPE_GRAPHITE,                  // Send plain text to Graphite
    BACKEND_TYPE_OPENTSDB_USING_TELNET,     // Send data to OpenTSDB using telnet API
    BACKEND_TYPE_OPENTSDB_USING_HTTP,       // Send data to OpenTSDB using HTTP API
    BACKEND_TYPE_JSON,                      // Stores the data using JSON.
    BACKEND_TYPE_PROMETHEUS,                // The user selected to use Prometheus backend
    BACKEND_TYPE_KINESIS,                   // Send message to AWS Kinesis
    BACKEND_TYPE_MONGODB,                   // Send data to MongoDB collection
    BACKEND_TYPE_NUM                        // Number of backend types
} BACKEND_TYPE;

#ifdef ENABLE_EXPORTING
#include "exporting/exporting_engine.h"
#endif

typedef int (**backend_response_checker_t)(BUFFER *);
typedef int (**backend_request_formatter_t)(BUFFER *, const char *, RRDHOST *, const char *, RRDSET *, RRDDIM *, time_t, time_t, BACKEND_OPTIONS);

#define BACKEND_OPTIONS_SOURCE_BITS (BACKEND_SOURCE_DATA_AS_COLLECTED|BACKEND_SOURCE_DATA_AVERAGE|BACKEND_SOURCE_DATA_SUM)
#define BACKEND_OPTIONS_DATA_SOURCE(backend_options) (backend_options & BACKEND_OPTIONS_SOURCE_BITS)

extern int global_backend_update_every;
extern BACKEND_OPTIONS global_backend_options;
extern const char *global_backend_prefix;

extern void *backends_main(void *ptr);
BACKEND_TYPE backend_select_type(const char *type);

extern BACKEND_OPTIONS backend_parse_data_source(const char *source, BACKEND_OPTIONS backend_options);

#ifdef BACKENDS_INTERNALS

extern int backends_can_send_rrdset(BACKEND_OPTIONS backend_options, RRDSET *st);
extern calculated_number backend_calculate_value_from_stored_data(
        RRDSET *st                  // the chart
        , RRDDIM *rd                // the dimension
        , time_t after              // the start timestamp
        , time_t before             // the end timestamp
        , BACKEND_OPTIONS backend_options  // BACKEND_SOURCE_* bitmap
        , time_t *first_timestamp   // the timestamp of the first point used in this response
        , time_t *last_timestamp    // the timestamp that should be reported to backend
);

extern size_t backend_name_copy(char *d, const char *s, size_t usable);
extern int discard_response(BUFFER *b, const char *backend);

static inline char *strip_quotes(char *str) {
    if(*str == '"' || *str == '\'') {
        char *s;

        str++;

        s = str;
        while(*s) s++;
        if(s != str) s--;

        if(*s == '"' || *s == '\'') *s = '\0';
    }

    return str;
}

#endif // BACKENDS_INTERNALS

#include "backends/prometheus/backend_prometheus.h"
#include "backends/graphite/graphite.h"
#include "backends/json/json.h"
#include "backends/opentsdb/opentsdb.h"

#if HAVE_KINESIS
#include "backends/aws_kinesis/aws_kinesis.h"
#endif

#if ENABLE_PROMETHEUS_REMOTE_WRITE
#include "backends/prometheus/remote_write/remote_write.h"
#endif

#if HAVE_MONGOC
#include "backends/mongodb/mongodb.h"
#endif

#endif /* NETDATA_BACKENDS_H */
