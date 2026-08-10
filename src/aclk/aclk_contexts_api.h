// SPDX-License-Identifier: GPL-3.0-or-later
#ifndef ACLK_CONTEXTS_API_H
#define ACLK_CONTEXTS_API_H

#include "schema-wrappers/schema_wrappers.h"

// Forward declaration - defined in aclk_query_queue.h
struct aclk_sync_completion;

void aclk_send_contexts_snapshot(contexts_snapshot_t data);
void aclk_send_contexts_updated(contexts_updated_t data);
void aclk_update_node_collectors(struct update_node_collectors *collectors);
void aclk_update_node_info(struct update_node_info *info, struct aclk_sync_completion *sync_completion);
void aclk_update_node_instance_manifest(
    struct update_node_instance_manifest *manifest,
    const char *machine_guid,
    uint64_t suppression_key,
    uint64_t token);

#endif /* ACLK_CONTEXTS_API_H */

