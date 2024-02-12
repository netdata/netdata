// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_WEB_API_V1_H
#define NETDATA_WEB_API_V1_H 1

#include "web_api.h"

struct web_client;

CONTEXTS_V2_OPTIONS web_client_api_request_v2_context_options(char *o);
CONTEXTS_V2_ALERT_STATUS web_client_api_request_v2_alert_status(char *o);
void web_client_api_request_v2_contexts_options_to_buffer_json_array(BUFFER *wb, const char *key, CONTEXTS_V2_OPTIONS options);
void web_client_api_request_v2_contexts_alerts_status_to_buffer_json_array(BUFFER *wb, const char *key, CONTEXTS_V2_ALERT_STATUS options);

RRDR_OPTIONS rrdr_options_parse(char *o);
RRDR_OPTIONS rrdr_options_parse_one(const char *o);

void rrdr_options_to_buffer_json_array(BUFFER *wb, const char *key, RRDR_OPTIONS options);
void web_client_api_request_v1_data_options_to_string(char *buf, size_t size, RRDR_OPTIONS options);

uint32_t web_client_api_request_v1_data_format(char *name);
uint32_t web_client_api_request_v1_data_google_format(char *name);

int web_client_api_request_v1_alarms(RRDHOST *host, struct web_client *w, char *url);
int web_client_api_request_v1_alarms_values(RRDHOST *host, struct web_client *w, char *url);
int web_client_api_request_v1_alarm_log(RRDHOST *host, struct web_client *w, char *url);
int web_client_api_request_single_chart(RRDHOST *host, struct web_client *w, char *url, void callback(RRDSET *st, BUFFER *buf));
int web_client_api_request_v1_alarm_variables(RRDHOST *host, struct web_client *w, char *url);
int web_client_api_request_v1_alarm_count(RRDHOST *host, struct web_client *w, char *url);
int web_client_api_request_v1_charts(RRDHOST *host, struct web_client *w, char *url);
int web_client_api_request_v1_chart(RRDHOST *host, struct web_client *w, char *url);
int web_client_api_request_v1_registry(RRDHOST *host, struct web_client *w, char *url);
int web_client_api_request_v1_info(RRDHOST *host, struct web_client *w, char *url);
int web_client_api_request_v1(RRDHOST *host, struct web_client *w, char *url_path_endpoint);
int web_client_api_request_v1_info_fill_buffer(RRDHOST *host, BUFFER *wb);

void web_client_api_v1_init(void);
void web_client_api_v1_management_init(void);

void host_labels2json(RRDHOST *host, BUFFER *wb, const char *key);
void web_client_api_request_v1_info_summary_alarm_statuses(RRDHOST *host, BUFFER *wb, const char *key);

void web_client_source2buffer(struct web_client *w, BUFFER *source);

extern char *api_secret;

#endif //NETDATA_WEB_API_V1_H
