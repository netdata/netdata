// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_API_V1_INFO_FIELDS_H
#define NETDATA_API_V1_INFO_FIELDS_H 1

#include "libnetdata/libnetdata.h"

// The agent-wide fields of /api/v1/info: build features, release channel, notification methods,
// exporting, API usage counters and the localhost chart/metric counts.

typedef enum {
    INFO_FIELDS_HIT_PROMETHEUS = 0,
    INFO_FIELDS_HIT_SHELL,
    INFO_FIELDS_HIT_JSON,
    INFO_FIELDS_HIT_DASHBOARD,

    // terminator
    INFO_FIELDS_HIT_MAX,
} INFO_FIELDS_HIT;

// Count one use of an API surface (while telemetry is enabled, saturating at 255).
void info_fields_count_hit(INFO_FIELDS_HIT hit);

// Build the immutable build-features string. Call once on the main thread, after system info detection.
void info_fields_buildinfo_init(void);

// One-shot static thread: run `alarm-notify.sh dump_methods` once and cache its output.
void info_fields_notification_methods_main(void *ptr);

// Add the fields to an /api/v1/info response.
void info_fields_to_json(BUFFER *wb);

#endif //NETDATA_API_V1_INFO_FIELDS_H
