// SPDX-License-Identifier: GPL-3.0-or-later

/** @file rrd_api_stats.h
 *  @brief Incudes the structure and function definitions
 *         for logs management stats charts.
 */

#ifndef RRD_API_STATS_H_
#define RRD_API_STATS_H_

#include "daemon/common.h"

struct File_info;

#include "../file_info.h"

void stats_charts_init(void);
void stats_charts_update(uv_timer_t *handle);

#endif // RRD_API_STATS_H_