// SPDX-License-Identifier: GPL-3.0-or-later
#include "aclk_charts.h"

#include "aclk_query_queue.h"

void aclk_chart_dim_update(charts_and_dims_updated_t *update) {
    aclk_query_t query = aclk_query_new(CHART_DIM_UPDATE);
    query->data.chart_dim_update = update;
    aclk_queue_query(query);
}

void aclk_chart_inst_update(char **payloads, size_t *payload_sizes, struct aclk_message_position *new_positions)
{
    aclk_query_t query = aclk_query_new(CHART_DIM_UPDATE_BIN);
    query->data.bin_payload.payload = generate_charts_updated(&query->data.bin_payload.size, payloads, payload_sizes, new_positions);
    if (query->data.bin_payload.payload)
        aclk_queue_query(query);
}
