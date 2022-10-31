// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef REPLICATION_H
#define REPLICATION_H

#include "daemon/common.h"

bool replicate_chart_response(RRDHOST *rh, RRDSET *rs, bool start_streaming, time_t after, time_t before);

typedef int (*send_command)(const char *txt, void *data);

bool replicate_chart_request(send_command callback, void *callback_data,
                             RRDHOST *rh, RRDSET *rs,
                             time_t first_entry_child, time_t last_entry_child,
                             time_t response_first_start_time, time_t response_last_end_time);

#endif /* REPLICATION_H */
