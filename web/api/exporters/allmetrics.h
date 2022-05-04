// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_API_ALLMETRICS_H
#define NETDATA_API_ALLMETRICS_H

#include "web/api/formatters/rrd2json.h"
#include "shell/allmetrics_shell.h"
#include "web/server/web_client.h"

extern int web_client_api_request_v1_allmetrics(RRDHOST *host, struct web_client *w, char *url);

#endif //NETDATA_API_ALLMETRICS_H
