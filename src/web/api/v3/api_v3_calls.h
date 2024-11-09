// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_API_V3_CALLS_H
#define NETDATA_API_V3_CALLS_H

#include "../web_api_v3.h"

int api_v2_contexts_internal(RRDHOST *host, struct web_client *w, char *url, CONTEXTS_V2_MODE mode);
#define api_v3_contexts_internal(host, w, url, mode) api_v2_contexts_internal(host, w, url, mode)

int api_v3_settings(RRDHOST *host, struct web_client *w, char *url);
int api_v3_me(RRDHOST *host, struct web_client *w, char *url);
int api_v3_stream_info(RRDHOST *host __maybe_unused, struct web_client *w, char *url);
int api_v3_stream_path(RRDHOST *host __maybe_unused, struct web_client *w, char *url);

#endif //NETDATA_API_V3_CALLS_H
