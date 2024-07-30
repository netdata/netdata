
#ifndef NETDATA_API_V2_CALLS_H
#define NETDATA_API_V2_CALLS_H

#include "../web_api_v3.h"

int api_v2_info(RRDHOST *host, struct web_client *w, char *url);

int api_v2_config(RRDHOST *host, struct web_client *w, char *url);

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

int api_v2_ilove(RRDHOST *host, struct web_client *w, char *url);

int api_v2_claim(RRDHOST *host, struct web_client *w, char *url);

int api_v2_webrtc(RRDHOST *host, struct web_client *w, char *url);

int api_v2_progress(RRDHOST *host, struct web_client *w, char *url);

int api_v2_bearer_token(RRDHOST *host, struct web_client *w, char *url);
int api_v2_bearer_protection(RRDHOST *host, struct web_client *w, char *url);

#endif //NETDATA_API_V2_CALLS_H
