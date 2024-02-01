// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef SCHEMA_WRAPPER_UTILS_H
#define SCHEMA_WRAPPER_UTILS_H

#include "database/rrd.h"

#include <sys/time.h>
#include <google/protobuf/timestamp.pb.h>
#include <google/protobuf/map.h>

#if GOOGLE_PROTOBUF_VERSION < 3001000
#define PROTO_COMPAT_MSG_SIZE(msg) (size_t)msg.ByteSize()
#define PROTO_COMPAT_MSG_SIZE_PTR(msg) (size_t)msg->ByteSize()
#else
#define PROTO_COMPAT_MSG_SIZE(msg) msg.ByteSizeLong()
#define PROTO_COMPAT_MSG_SIZE_PTR(msg) msg->ByteSizeLong()
#endif

void set_google_timestamp_from_timeval(struct timeval tv, google::protobuf::Timestamp *ts);
void set_timeval_from_google_timestamp(const google::protobuf::Timestamp &ts, struct timeval *tv);
int label_add_to_map_callback(const char *name, const char *value, RRDLABEL_SRC ls, void *data);

#endif /* SCHEMA_WRAPPER_UTILS_H */
