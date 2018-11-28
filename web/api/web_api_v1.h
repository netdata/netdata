// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_WEB_API_V1_H
#define NETDATA_WEB_API_V1_H 1

#include "daemon/common.h"
#include "web/api/badges/web_buffer_svg.h"
#include "web/api/formatters/rrd2json.h"

extern uint32_t web_client_api_request_v1_data_options(char *o);
extern uint32_t web_client_api_request_v1_data_format(char *name);
extern uint32_t web_client_api_request_v1_data_google_format(char *name);

extern int web_client_api_request_v1_alarms(RRDHOST *host, struct web_client *w, char *url);
extern int web_client_api_request_v1_alarm_log(RRDHOST *host, struct web_client *w, char *url);
extern int web_client_api_request_single_chart(RRDHOST *host, struct web_client *w, char *url, void callback(RRDSET *st, BUFFER *buf));
extern int web_client_api_request_v1_alarm_variables(RRDHOST *host, struct web_client *w, char *url);
extern int web_client_api_request_v1_charts(RRDHOST *host, struct web_client *w, char *url);
extern int web_client_api_request_v1_chart(RRDHOST *host, struct web_client *w, char *url);
extern int web_client_api_request_v1_data(RRDHOST *host, struct web_client *w, char *url);
extern int web_client_api_request_v1_registry(RRDHOST *host, struct web_client *w, char *url);
extern int web_client_api_request_v1(RRDHOST *host, struct web_client *w, char *url);

extern void web_client_api_v1_init(void);

#endif //NETDATA_WEB_API_V1_H
