// SPDX-License-Identifier: GPL-3.0-or-later
#include "aclk_charts_api.h"

#include "aclk_query_queue.h"

#define CHART_DIM_UPDATE_NAME "ChartsAndDimensionsUpdated"

void aclk_chart_inst_update(char **payloads, size_t *payload_sizes, struct aclk_message_position *new_positions)
{
    aclk_query_t query = aclk_query_new(CHART_DIMS_UPDATE);
    query->data.bin_payload.payload = generate_charts_updated(&query->data.bin_payload.size, payloads, payload_sizes, new_positions);
    query->data.bin_payload.msg_name = CHART_DIM_UPDATE_NAME;
    if (query->data.bin_payload.payload)
        aclk_queue_query(query);
}

void aclk_chart_dim_update(char **payloads, size_t *payload_sizes, struct aclk_message_position *new_positions)
{
    aclk_query_t query = aclk_query_new(CHART_DIMS_UPDATE);
    query->data.bin_payload.payload = generate_chart_dimensions_updated(&query->data.bin_payload.size, payloads, payload_sizes, new_positions);
    query->data.bin_payload.msg_name = CHART_DIM_UPDATE_NAME;
    if (query->data.bin_payload.payload)
        aclk_queue_query(query);
}

void aclk_chart_inst_and_dim_update(char **payloads, size_t *payload_sizes, int *is_dim, struct aclk_message_position *new_positions, uint64_t batch_id)
{
    aclk_query_t query = aclk_query_new(CHART_DIMS_UPDATE);
    query->data.bin_payload.payload = generate_charts_and_dimensions_updated(&query->data.bin_payload.size, payloads, payload_sizes, is_dim, new_positions, batch_id);
    query->data.bin_payload.msg_name = CHART_DIM_UPDATE_NAME;
    if (query->data.bin_payload.payload)
        aclk_queue_query(query);
}

void aclk_chart_config_updated(struct chart_config_updated *config_list, int list_size)
{
    aclk_query_t query = aclk_query_new(CHART_CONFIG_UPDATED);
    query->data.bin_payload.payload = generate_chart_configs_updated(&query->data.bin_payload.size, config_list, list_size);
    query->data.bin_payload.msg_name = "ChartConfigsUpdated";
    if (query->data.bin_payload.payload)
        aclk_queue_query(query);
}

void aclk_chart_reset(chart_reset_t reset)
{
    aclk_query_t query = aclk_query_new(CHART_RESET);
    query->data.bin_payload.payload = generate_reset_chart_messages(&query->data.bin_payload.size, reset);
    query->data.bin_payload.msg_name = "ResetChartMessages";
    if (query->data.bin_payload.payload)
        aclk_queue_query(query);
}

void aclk_retention_updated(struct retention_updated *data)
{
    aclk_query_t query = aclk_query_new(RETENTION_UPDATED);
    query->data.bin_payload.topic = ACLK_TOPICID_RETENTION_UPDATED;
    query->data.bin_payload.payload = generate_retention_updated(&query->data.bin_payload.size, data);
    query->data.bin_payload.msg_name = "RetentionUpdated";
    if (query->data.bin_payload.payload)
        aclk_queue_query(query);
}
