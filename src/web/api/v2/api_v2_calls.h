// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_API_V2_CALLS_H
#define NETDATA_API_V2_CALLS_H

#include "../web_api_v2.h"

int api_v2_info(RRDHOST *host, struct web_client *w, char *url);

int api_v2_data(RRDHOST *host, struct web_client *w, char *url);
int api_v2_weights(RRDHOST *host, struct web_client *w, char *url);

int api_v2_alert_config(RRDHOST *host, struct web_client *w, char *url);

int api_v2_contexts_internal(RRDHOST *host, struct web_client *w, char *url, CONTEXTS_V2_MODE mode);
int api_v2_contexts(RRDHOST *host, struct web_client *w, char *url);
int api_v2_alert_transitions(RRDHOST *host, struct web_client *w, char *url);
int api_v2_alerts(RRDHOST *host, struct web_client *w, char *url);
int api_v2_functions(RRDHOST *host, struct web_client *w, char *url);
int api_v2_versions(RRDHOST *host, struct web_client *w, char *url);
int api_v2_q(RRDHOST *host, struct web_client *w, char *url);
int api_v2_nodes(RRDHOST *host, struct web_client *w, char *url);
int api_v2_node_instances(RRDHOST *host, struct web_client *w, char *url);

int api_v2_claim(RRDHOST *host, struct web_client *w, char *url);
int api_v3_claim(RRDHOST *host, struct web_client *w, char *url);

int api_v2_webrtc(RRDHOST *host, struct web_client *w, char *url);

int api_v2_progress(RRDHOST *host, struct web_client *w, char *url);

int api_v2_bearer_get_token(RRDHOST *host, struct web_client *w, char *url);
int bearer_get_token_json_response(BUFFER *wb, RRDHOST *host, const char *claim_id, const char *machine_guid, const char *node_id, HTTP_USER_ROLE user_role, HTTP_ACCESS access, nd_uuid_t cloud_account_id, const char *client_name);
int api_v2_bearer_protection(RRDHOST *host, struct web_client *w, char *url);

#endif //NETDATA_API_V2_CALLS_H
