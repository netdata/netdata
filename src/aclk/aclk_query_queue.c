// SPDX-License-Identifier: GPL-3.0-or-later

#include "aclk_query_queue.h"

#define MAX_QUERY_ENTRIES (512)

struct {
    aclk_query_t query_workers[MAX_QUERY_ENTRIES];
    int free_stack[MAX_QUERY_ENTRIES];
    int top;
    SPINLOCK spinlock;
} queryPool;

// Initialize the query pool
__attribute__((constructor)) void init_query_pool()
{
    spinlock_init(&queryPool.spinlock);
    for (int i = 0; i < MAX_QUERY_ENTRIES; i++) {
        queryPool.free_stack[i] = i;
        queryPool.query_workers[i].allocated = false;
    }
    queryPool.top = MAX_QUERY_ENTRIES;
}

static aclk_query_t *get_query()
{
    spinlock_lock(&queryPool.spinlock);
    if (queryPool.top == 0) {
        spinlock_unlock(&queryPool.spinlock);
        aclk_query_t *query = callocz(1, sizeof(aclk_query_t));
        query->allocated = true;
        return query;
    }
    int index = queryPool.free_stack[--queryPool.top];
    memset(&queryPool.query_workers[index], 0, sizeof(aclk_query_t));
    spinlock_unlock(&queryPool.spinlock);
    return &queryPool.query_workers[index];
}

static void return_query(aclk_query_t *query)
{
    if (unlikely(query->allocated)) {
       freez(query);
       return;
    }
    spinlock_lock(&queryPool.spinlock);
    int index = (int) (query - queryPool.query_workers);
    if (index < 0 || index >= MAX_QUERY_ENTRIES) {
        spinlock_unlock(&queryPool.spinlock);
        return;  // Invalid (should not happen)
    }
    queryPool.free_stack[queryPool.top++] = index;
    memset(query, 0, sizeof(aclk_query_t));
    spinlock_unlock(&queryPool.spinlock);
}

aclk_query_t *aclk_query_new(aclk_query_type_t type)
{
    aclk_query_t *query = get_query();
    query->type = type;
    now_monotonic_high_precision_timeval(&query->created_tv);
    return query;
}

void aclk_query_free(aclk_query_t *query)
{
    struct ctxs_checkpoint *cmd;
    switch (query->type) {
        case HTTP_API_V2:
            freez(query->data.http_api_v2.payload);
            if (query->data.http_api_v2.query != query->dedup_id)
                freez(query->data.http_api_v2.query);
            break;
        case ALERT_START_STREAMING:
            freez(query->data.node_id);
            break;
        case ALERT_CHECKPOINT:
            freez(query->data.node_id);
            freez(query->claim_id);
            break;
        case CREATE_NODE_INSTANCE:
            freez(query->data.node_id);
            freez(query->machine_guid);
            break;
        // keep following cases together
        case CTX_STOP_STREAMING:
        case CTX_CHECKPOINT:
            cmd = query->data.payload;
            freez(cmd->claim_id);
            freez(cmd->node_id);
            freez(cmd);
            break;

        default:
            break;
    }

    freez(query->dedup_id);
    freez(query->callback_topic);
    freez(query->msg_id);
    return_query(query);
}
