// SPDX-License-Identifier: GPL-3.0-or-later

#include "aclk_query_queue.h"

#include "aclk_contexts_api.h"

#include "aclk.h"

void aclk_send_contexts_snapshot(contexts_snapshot_t data)
{
    GENERATE_AND_SEND_PAYLOAD(ACLK_TOPICID_CTXS_SNAPSHOT, "ContextsSnapshot", contexts_snapshot_2bin, data);
}

void aclk_send_contexts_updated(contexts_updated_t data)
{
    GENERATE_AND_SEND_PAYLOAD(ACLK_TOPICID_CTXS_UPDATED, "ContextsUpdated", contexts_updated_2bin, data);
}
