// SPDX-License-Identifier: GPL-3.0-or-later
#include "aclk_charts_api.h"

#include "aclk_query_queue.h"

#include "aclk.h"

#define CHART_DIM_UPDATE_NAME "ChartsAndDimensionsUpdated"

void aclk_chart_inst_update(char **payloads, size_t *payload_sizes, struct aclk_message_position *new_positions)
{
    GENERATE_AND_SEND_PAYLOAD(ACLK_TOPICID_CHART_DIMS, CHART_DIM_UPDATE_NAME, generate_charts_updated, payloads, payload_sizes, new_positions);
}

void aclk_chart_dim_update(char **payloads, size_t *payload_sizes, struct aclk_message_position *new_positions)
{
    GENERATE_AND_SEND_PAYLOAD(ACLK_TOPICID_CHART_DIMS, CHART_DIM_UPDATE_NAME, generate_chart_dimensions_updated, payloads, payload_sizes, new_positions);
}

void aclk_chart_inst_and_dim_update(char **payloads, size_t *payload_sizes, int *is_dim, struct aclk_message_position *new_positions, uint64_t batch_id)
{
    GENERATE_AND_SEND_PAYLOAD(ACLK_TOPICID_CHART_DIMS, CHART_DIM_UPDATE_NAME, generate_charts_and_dimensions_updated, payloads, payload_sizes, is_dim, new_positions, batch_id);
}

void aclk_chart_config_updated(struct chart_config_updated *config_list, int list_size)
{
    GENERATE_AND_SEND_PAYLOAD(ACLK_TOPICID_CHART_CONFIGS_UPDATED, "ChartConfigsUpdated", generate_chart_configs_updated, config_list, list_size);
}

void aclk_chart_reset(chart_reset_t reset)
{
    GENERATE_AND_SEND_PAYLOAD(ACLK_TOPICID_CHART_RESET, "ResetChartMessages", generate_reset_chart_messages, reset);
}

void aclk_retention_updated(struct retention_updated *data)
{
    GENERATE_AND_SEND_PAYLOAD(ACLK_TOPICID_RETENTION_UPDATED, "RetentionUpdated", generate_retention_updated, data);
}

void aclk_update_node_info(struct update_node_info *info)
{
    GENERATE_AND_SEND_PAYLOAD(ACLK_TOPICID_NODE_INFO, "UpdateNodeInfo", generate_update_node_info_message, info);
}

void aclk_update_node_collectors(struct update_node_collectors *collectors)
{
    GENERATE_AND_SEND_PAYLOAD(ACLK_TOPICID_NODE_COLLECTORS, "UpdateNodeCollectors", generate_update_node_collectors_message, collectors);
}
