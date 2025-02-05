// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_STREAM_REPLICATION_RECEIVER_H
#define NETDATA_STREAM_REPLICATION_RECEIVER_H

#include "libnetdata/libnetdata.h"
#include "stream-traffic-types.h"

#ifdef __cplusplus
extern "C" {
#endif

struct parser;
struct rrdhost;
struct rrdset;

typedef ssize_t (*send_command)(const char *txt, struct parser *parser, STREAM_TRAFFIC_TYPE type);
bool replicate_chart_request(send_command callback, struct parser *parser,
                             struct rrdhost *rh, struct rrdset *rs,
                             time_t child_first_entry, time_t child_last_entry, time_t child_wall_clock_time,
                             time_t response_first_start_time, time_t response_last_end_time);

bool stream_parse_enable_streaming(const char *start_streaming_txt);

#ifdef __cplusplus
}
#endif

#endif //NETDATA_STREAM_REPLICATION_RECEIVER_H
