// SPDX-License-Identifier: GPL-3.0-or-later

#include "aclk_query_queue.h"

aclk_query_t aclk_query_new(aclk_query_type_t type)
{
    aclk_query_t query = callocz(1, sizeof(struct aclk_query));
    query->type = type;
    return query;
}

void aclk_query_free(aclk_query_t query)
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
        case CTX_STOP_STREAMING:
            cmd = query->data.payload;
            freez(cmd->claim_id);
            freez(cmd->node_id);
            freez(cmd);
            break;
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
    freez(query);
}
