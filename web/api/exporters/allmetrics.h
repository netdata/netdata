// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_API_ALLMETRICS_H
#define NETDATA_API_ALLMETRICS_H

#include "web/api/formatters/rrd2json.h"
#include "shell/allmetrics_shell.h"
#include "web/server/web_client.h"

extern int web_client_api_request_v1_allmetrics(RRDHOST *host, struct web_client *w, char *url);

struct allmetrics_filter; // declared in rrd.h

int lock_and_update_allmetrics_filter(struct allmetrics_filter **filter_p, const char *filter_string);
void unlock_allmetrics_filter(struct allmetrics_filter *filter, int filter_changed);
int chart_is_filtered_out(RRDSET *st, struct allmetrics_filter *filter, int filter_changed, int filter_type);

#endif //NETDATA_API_ALLMETRICS_H
