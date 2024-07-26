// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_WEB_API_V1_H
#define NETDATA_WEB_API_V1_H 1

#include "web_api.h"

struct web_client;

#include "maps/rrdr_options.h"

CONTEXTS_V2_OPTIONS web_client_api_request_v2_context_options(char *o);
CONTEXTS_V2_ALERT_STATUS web_client_api_request_v2_alert_status(char *o);
void web_client_api_request_v2_contexts_options_to_buffer_json_array(BUFFER *wb, const char *key, CONTEXTS_V2_OPTIONS options);
void web_client_api_request_v2_contexts_alerts_status_to_buffer_json_array(BUFFER *wb, const char *key, CONTEXTS_V2_ALERT_STATUS options);

RRDR_OPTIONS rrdr_options_parse(char *o);
RRDR_OPTIONS rrdr_options_parse_one(const char *o);

uint32_t web_client_api_request_vX_data_format(char *name);
uint32_t web_client_api_request_vX_data_google_format(char *name);

int web_client_api_request_v1(RRDHOST *host, struct web_client *w, char *url_path_endpoint);

void nd_web_api_init(void);
void web_client_api_v1_management_init(void);

void web_client_api_request_vX_source_to_buffer(struct web_client *w, BUFFER *source);

extern char *api_secret;

#endif //NETDATA_WEB_API_V1_H
