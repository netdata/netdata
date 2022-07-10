// SPDX-License-Identifier: GPL-3.0-or-later
#ifndef ACLK_CHARTS_H
#define ACLK_CHARTS_H

#include "../daemon/common.h"
#include "schema-wrappers/schema_wrappers.h"

void aclk_chart_inst_update(char **payloads, size_t *payload_sizes, struct aclk_message_position *new_positions);
void aclk_chart_dim_update(char **payloads, size_t *payload_sizes, struct aclk_message_position *new_positions);
void aclk_chart_inst_and_dim_update(char **payloads, size_t *payload_sizes, int *is_dim, struct aclk_message_position *new_positions, uint64_t batch_id);

void aclk_chart_config_updated(struct chart_config_updated *config_list, int list_size);

void aclk_chart_reset(chart_reset_t reset);

void aclk_retention_updated(struct retention_updated *data);

void aclk_update_node_info(struct update_node_info *info);

void aclk_update_node_collectors(struct update_node_collectors *collectors);

#endif /* ACLK_CHARTS_H */
