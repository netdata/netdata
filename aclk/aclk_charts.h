// SPDX-License-Identifier: GPL-3.0-or-later
#ifndef ACLK_CHARTS_H
#define ACLK_CHARTS_H

#include "../daemon/common.h"
#include "schema-wrappers/schema_wrappers.h"

void aclk_charts_and_dims_update(charts_and_dims_updated_t *update);

void aclk_chart_inst_update(char **payloads, size_t *payload_sizes, struct aclk_message_position *new_positions);
void aclk_chart_dim_update(char **payloads, size_t *payload_sizes, struct aclk_message_position *new_positions);

void aclk_chart_config_updated(struct chart_config_updated *config_list, int list_size);

#endif /* ACLK_CHARTS_H */
