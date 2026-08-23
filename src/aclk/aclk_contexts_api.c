// SPDX-License-Identifier: GPL-3.0-or-later

#include "aclk_query_queue.h"

#include "aclk_contexts_api.h"

void aclk_send_contexts_snapshot(contexts_snapshot_t data)
{
    aclk_query_t *query = aclk_query_new(CTX_SEND_SNAPSHOT);
    query->data.bin_payload.topic = ACLK_TOPICID_CTXS_SNAPSHOT;
    query->data.bin_payload.payload = contexts_snapshot_2bin(data, &query->data.bin_payload.size);
    query->data.bin_payload.msg_name = "ContextsSnapshot";
    QUEUE_IF_PAYLOAD_PRESENT(query);
}

void aclk_send_contexts_updated(contexts_updated_t data)
{
    aclk_query_t *query = aclk_query_new(CTX_SEND_SNAPSHOT_UPD);
    query->data.bin_payload.topic = ACLK_TOPICID_CTXS_UPDATED;
    query->data.bin_payload.payload = contexts_updated_2bin(data, &query->data.bin_payload.size);
    query->data.bin_payload.msg_name = "ContextsUpdated";
    QUEUE_IF_PAYLOAD_PRESENT(query);
}

void aclk_update_node_collectors(struct update_node_collectors *collectors)
{
    aclk_query_t *query = aclk_query_new(UPDATE_NODE_COLLECTORS);
    query->data.bin_payload.topic = ACLK_TOPICID_NODE_COLLECTORS;
    query->data.bin_payload.payload = generate_update_node_collectors_message(&query->data.bin_payload.size, collectors);
    query->data.bin_payload.msg_name = "UpdateNodeCollectors";
    QUEUE_IF_PAYLOAD_PRESENT(query);
}

void aclk_update_node_info(struct update_node_info *info, struct aclk_sync_completion *sync_completion)
{
    aclk_query_t *query = aclk_query_new(UPDATE_NODE_INFO);
    query->sync_completion = sync_completion;
    query->data.bin_payload.topic = ACLK_TOPICID_NODE_INFO;
    query->data.bin_payload.payload = generate_update_node_info_message(&query->data.bin_payload.size, info);
    query->data.bin_payload.msg_name = "UpdateNodeInfo";
    QUEUE_IF_PAYLOAD_PRESENT(query);
}

// The caller has already recorded this enqueue under `token` on the host identified by
// `machine_guid`. They are carried on the query so that a message which never reaches the mqtt layer
// can have that record invalidated again - see aclk_node_manifest_publish_result().
void aclk_update_node_instance_manifest(
    struct update_node_instance_manifest *manifest,
    const char *machine_guid,
    uint64_t token)
{
    aclk_query_t *query = aclk_query_new(UPDATE_NODE_MANIFEST);
    strncpyz(query->manifest.machine_guid, machine_guid, sizeof(query->manifest.machine_guid) - 1);
    query->manifest.token = token;
    query->data.bin_payload.topic = ACLK_TOPICID_NODE_MANIFEST;
    query->data.bin_payload.payload =
        generate_update_node_instance_manifest_message(&query->data.bin_payload.size, manifest);
    query->data.bin_payload.msg_name = "UpdateNodeInstanceManifest";
    QUEUE_IF_PAYLOAD_PRESENT(query);
}
