// SPDX-License-Identifier: GPL-3.0-or-later

#include "api_v1_calls.h"

int api_v1_metric_correlations(RRDHOST *host, struct web_client *w, char *url) {
    return web_client_api_request_weights(host, w, url, default_metric_correlations_method, WEIGHTS_FORMAT_CHARTS, 1);
}

int api_v1_weights(RRDHOST *host, struct web_client *w, char *url) {
    return web_client_api_request_weights(host, w, url, WEIGHTS_METHOD_ANOMALY_RATE, WEIGHTS_FORMAT_CONTEXTS, 1);
}
