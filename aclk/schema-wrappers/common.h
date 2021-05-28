// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef ACLK_SCHEMA_WRAPPERS_COMMON_H
#define ACLK_SCHEMA_WRAPPERS_COMMON_H

#include <google/protobuf/timestamp.pb.h>

void set_google_timestamp_from_timeval(struct timeval tv, google::protobuf::Timestamp *ts);

#endif /* ACLK_SCHEMA_WRAPPERS_COMMON_H */
