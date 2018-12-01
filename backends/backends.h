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

#define BACKEND_OPTIONS_SOURCE_BITS (BACKEND_SOURCE_DATA_AS_COLLECTED|BACKEND_SOURCE_DATA_AVERAGE|BACKEND_SOURCE_DATA_SUM)
#define BACKEND_OPTIONS_DATA_SOURCE(backend_options) (backend_options & BACKEND_OPTIONS_SOURCE_BITS)

extern usec_t global_backend_update_every_usec;
extern BACKEND_OPTIONS global_backend_options;
extern const char *global_backend_prefix;

extern void *backends_main(void *ptr);

extern BACKEND_OPTIONS backend_parse_data_source(const char *source, BACKEND_OPTIONS backend_options);

#ifdef BACKENDS_INTERNALS

extern int backends_can_send_rrdset(BACKEND_OPTIONS backend_options, RRDSET *st);
extern calculated_number backend_calculate_value_from_stored_data(
        RRDSET *st                  // the chart
        , RRDDIM *rd                // the dimension
        , usec_t after_usec              // the start timestamp
        , usec_t before_usec             // the end timestamp
        , BACKEND_OPTIONS backend_options  // BACKEND_SOURCE_* bitmap
        , usec_t *first_timestamp_usec   // the timestamp of the first point used in this response
        , usec_t *last_timestamp_usec    // the timestamp that should be reported to backend
);

extern size_t backend_name_copy(char *d, const char *s, size_t usable);
extern int discard_response(BUFFER *b, const char *backend);

#endif // BACKENDS_INTERNALS

#include "backends/prometheus/backend_prometheus.h"
#include "backends/graphite/graphite.h"
#include "backends/json/json.h"
#include "backends/opentsdb/opentsdb.h"

#endif /* NETDATA_BACKENDS_H */
