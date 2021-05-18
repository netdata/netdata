// SPDX-License-Identifier: GPL-3.0-or-later
#include "aclk_charts.h"

#include "aclk_query_queue.h"

void aclk_chart_dim_update(charts_and_dims_updated_t *update) {
    aclk_query_t query = aclk_query_new(CHART_DIM_UPDATE);
    query->data.chart_dim_update = update;
    aclk_queue_query(query);
}
