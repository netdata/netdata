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
    switch (query->type) {
        case HTTP_API_V2:
            freez(query->data.http_api_v2.payload);
            if (query->data.http_api_v2.query != query->dedup_id)
                freez(query->data.http_api_v2.query);
            break;

        default:
            break;
    }

    freez(query->dedup_id);
    freez(query->callback_topic);
    freez(query->msg_id);
    freez(query);
}
