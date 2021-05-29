// SPDX-License-Identifier: GPL-3.0-or-later

#include "common.h"

void set_google_timestamp_from_timeval(struct timeval tv, google::protobuf::Timestamp *ts)
{
    ts->set_nanos(tv.tv_usec/1000);
    ts->set_seconds(tv.tv_sec);
}
