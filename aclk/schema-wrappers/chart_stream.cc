// SPDX-License-Identifier: GPL-3.0-or-later

#include "../aclk-schemas/proto/chart/v1/stream.pb.h"
#include "chart_stream.h"

#include <sys/time.h>
#include <stdlib.h>

stream_charts_and_dims_t parse_stream_charts_and_dims(const char *data, size_t len)
{
    chart::v1::StreamChartsAndDimensions msg;
    stream_charts_and_dims_t res = { .claim_id = NULL, .node_id = NULL };

    if (!msg.ParseFromArray(data, len))
        return res;

    res.node_id = strdup(msg.node_id().c_str());
    res.claim_id = strdup(msg.claim_id().c_str());
    res.seq_id = msg.sequence_id();
    res.batch_id = msg.batch_id();
    res.seq_id_created_at.tv_usec = msg.seq_id_created_at().nanos() / 1000;
    res.seq_id_created_at.tv_sec = msg.seq_id_created_at().seconds();

    return res;
}
