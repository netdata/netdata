// SPDX-License-Identifier: GPL-3.0-or-later

#include "schema_wrapper_utils.h"

void set_google_timestamp_from_timeval(struct timeval tv, google::protobuf::Timestamp *ts)
{
    ts->set_nanos(tv.tv_usec*1000);
    ts->set_seconds(tv.tv_sec);
}

void set_timeval_from_google_timestamp(const google::protobuf::Timestamp &ts, struct timeval *tv)
{
    tv->tv_sec = ts.seconds();
    tv->tv_usec = ts.nanos()/1000;
}

int label_add_to_map_callback(const char *name, const char *value, RRDLABEL_SRC ls __maybe_unused, void *data)
{
    auto map = (google::protobuf::Map<std::string, std::string> *)data;
    map->insert({name, value});
    return 1;
}
