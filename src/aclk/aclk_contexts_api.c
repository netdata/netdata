// SPDX-License-Identifier: GPL-3.0-or-later

#include "aclk_query_queue.h"

#include "aclk_contexts_api.h"

void aclk_send_contexts_snapshot(contexts_snapshot_t data)
{
    aclk_query_t query = aclk_query_new(CTX_SEND_SNAPSHOT);
    query->data.bin_payload.topic = ACLK_TOPICID_CTXS_SNAPSHOT;
    query->data.bin_payload.payload = contexts_snapshot_2bin(data, &query->data.bin_payload.size);
    query->data.bin_payload.msg_name = "ContextsSnapshot";
    QUEUE_IF_PAYLOAD_PRESENT(query);
}

void aclk_send_contexts_updated(contexts_updated_t data)
{
    aclk_query_t query = aclk_query_new(CTX_SEND_SNAPSHOT_UPD);
    query->data.bin_payload.topic = ACLK_TOPICID_CTXS_UPDATED;
    query->data.bin_payload.payload = contexts_updated_2bin(data, &query->data.bin_payload.size);
    query->data.bin_payload.msg_name = "ContextsUpdated";
    QUEUE_IF_PAYLOAD_PRESENT(query);
}

void aclk_update_node_collectors(struct update_node_collectors *collectors)
{
    aclk_query_t query = aclk_query_new(UPDATE_NODE_COLLECTORS);
    query->data.bin_payload.topic = ACLK_TOPICID_NODE_COLLECTORS;
    query->data.bin_payload.payload = generate_update_node_collectors_message(&query->data.bin_payload.size, collectors);
    query->data.bin_payload.msg_name = "UpdateNodeCollectors";
    QUEUE_IF_PAYLOAD_PRESENT(query);
}

void aclk_update_node_info(struct update_node_info *info)
{
    aclk_query_t query = aclk_query_new(UPDATE_NODE_INFO);
    query->data.bin_payload.topic = ACLK_TOPICID_NODE_INFO;
    query->data.bin_payload.payload = generate_update_node_info_message(&query->data.bin_payload.size, info);
    query->data.bin_payload.msg_name = "UpdateNodeInfo";
    QUEUE_IF_PAYLOAD_PRESENT(query);
}
