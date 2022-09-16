// SPDX-License-Identifier: GPL-3.0-or-later

#include "aclk_query_queue.h"

#include "aclk_contexts_api.h"

void aclk_send_contexts_snapshot(contexts_snapshot_t data)
{
    aclk_query_t query = aclk_query_new(PROTO_BIN_MESSAGE);
    query->data.bin_payload.topic = ACLK_TOPICID_CTXS_SNAPSHOT;
    query->data.bin_payload.payload = contexts_snapshot_2bin(data, &query->data.bin_payload.size);
    query->data.bin_payload.msg_name = "ContextsSnapshot";
    QUEUE_IF_PAYLOAD_PRESENT(query);
}

void aclk_send_contexts_updated(contexts_updated_t data)
{
    aclk_query_t query = aclk_query_new(PROTO_BIN_MESSAGE);
    query->data.bin_payload.topic = ACLK_TOPICID_CTXS_UPDATED;
    query->data.bin_payload.payload = contexts_updated_2bin(data, &query->data.bin_payload.size);
    query->data.bin_payload.msg_name = "ContextsUpdated";
    QUEUE_IF_PAYLOAD_PRESENT(query);
}
