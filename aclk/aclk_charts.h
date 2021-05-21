// SPDX-License-Identifier: GPL-3.0-or-later
#ifndef ACLK_CHARTS_H
#define ACLK_CHARTS_H

#include "../daemon/common.h"

void aclk_charts_and_dims_update(charts_and_dims_updated_t *update);

void aclk_chart_inst_update(char **payloads, size_t *payload_sizes, struct aclk_message_position *new_positions);
void aclk_chart_dim_update(char **payloads, size_t *payload_sizes, struct aclk_message_position *new_positions);

#endif /* ACLK_CHARTS_H */
