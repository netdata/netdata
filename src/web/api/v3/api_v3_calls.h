
#ifndef NETDATA_API_V3_CALLS_H
#define NETDATA_API_V3_CALLS_H

#include "../web_api_v3.h"

int web_client_api_request_v3_info(RRDHOST *host, struct web_client *w, char *url);

int web_client_api_request_v3_config(RRDHOST *host, struct web_client *w, char *url);

int web_client_api_request_v3_data(RRDHOST *host, struct web_client *w, char *url);
int web_client_api_request_v3_weights(RRDHOST *host, struct web_client *w, char *url);

int web_client_api_request_v3_alert_config(RRDHOST *host, struct web_client *w, char *url);

int web_client_api_request_v3_contexts_internal(RRDHOST *host, struct web_client *w, char *url, CONTEXTS_V2_MODE mode);
int web_client_api_request_v3_contexts(RRDHOST *host, struct web_client *w, char *url);
int web_client_api_request_v3_alert_transitions(RRDHOST *host, struct web_client *w, char *url);
int web_client_api_request_v3_alerts(RRDHOST *host, struct web_client *w, char *url);
int web_client_api_request_v3_functions(RRDHOST *host, struct web_client *w, char *url);
int web_client_api_request_v3_versions(RRDHOST *host, struct web_client *w, char *url);
int web_client_api_request_v3_q(RRDHOST *host, struct web_client *w, char *url);
int web_client_api_request_v3_nodes(RRDHOST *host, struct web_client *w, char *url);
int web_client_api_request_v3_node_instances(RRDHOST *host, struct web_client *w, char *url);

int web_client_api_request_v3_function(RRDHOST *host, struct web_client *w, char *url);
int web_client_api_request_v1_functions(RRDHOST *host, struct web_client *w, char *url);

int web_client_api_request_v3_badge(RRDHOST *host, struct web_client *w, char *url);
int web_client_api_request_v3_ilove(RRDHOST *host, struct web_client *w, char *url);

int web_client_api_request_v3_claim(RRDHOST *host, struct web_client *w, char *url);

int web_client_api_request_v3_webrtc(RRDHOST *host, struct web_client *w, char *url);

int web_client_api_request_v3_progress(RRDHOST *host, struct web_client *w, char *url);

int web_client_api_request_v3_allmetrics(RRDHOST *host, struct web_client *w, char *url);

int api_v3_bearer_token(RRDHOST *host, struct web_client *w, char *url);
int api_v3_bearer_protection(RRDHOST *host, struct web_client *w, char *url);

static inline void web_client_progress_functions_update(void *data, size_t done, size_t all) {
    // handle progress updates from the plugin
    struct web_client *w = data;
    query_progress_functions_update(&w->transaction, done, all);
}

#endif //NETDATA_API_V3_CALLS_H
