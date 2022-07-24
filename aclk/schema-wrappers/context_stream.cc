// SPDX-License-Identifier: GPL-3.0-or-later

#include "proto/context/v1/stream.pb.h"

#include "context_stream.h"

#include "libnetdata/libnetdata.h"

struct stop_streaming_ctxs *parse_stop_streaming_ctxs(const char *data, size_t len)
{
    context::v1::StopStreamingContexts msg;

    struct stop_streaming_ctxs *res;

    if (!msg.ParseFromArray(data, len))
        return NULL;

    res = (struct stop_streaming_ctxs *)callocz(1, sizeof(struct stop_streaming_ctxs));

    res->claim_id = strdupz(msg.claim_id().c_str());
    res->node_id = strdupz(msg.node_id().c_str());

    return res;
}

struct ctxs_checkpoint *parse_ctxs_checkpoint(const char *data, size_t len)
{
    context::v1::ContextsCheckpoint msg;

    struct ctxs_checkpoint *res;

    if (!msg.ParseFromArray(data, len))
        return NULL;

    res = (struct ctxs_checkpoint *)callocz(1, sizeof(struct ctxs_checkpoint));

    res->claim_id = strdupz(msg.claim_id().c_str());
    res->node_id = strdupz(msg.node_id().c_str());
    res->version_hash = msg.version_hash();

    return res;
}
