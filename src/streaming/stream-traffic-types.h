// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_STREAM_TRAFFIC_TYPES_H
#define NETDATA_STREAM_TRAFFIC_TYPES_H

#ifdef __cplusplus
extern "C" {
#endif

typedef enum __attribute__((packed)) {
    STREAM_TRAFFIC_TYPE_REPLICATION = 0,
    STREAM_TRAFFIC_TYPE_FUNCTIONS,
    STREAM_TRAFFIC_TYPE_METADATA,
    STREAM_TRAFFIC_TYPE_DATA,

    // terminator
    STREAM_TRAFFIC_TYPE_MAX,
} STREAM_TRAFFIC_TYPE;

#ifdef __cplusplus
}
#endif

#endif //NETDATA_STREAM_TRAFFIC_TYPES_H
