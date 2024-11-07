// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_API_V3_CALLS_H
#define NETDATA_API_V3_CALLS_H

#include "../web_api_v3.h"

int api_v3_settings(RRDHOST *host, struct web_client *w, char *url);
int api_v3_me(RRDHOST *host, struct web_client *w, char *url);
int api_v3_stream_info(RRDHOST *host __maybe_unused, struct web_client *w, char *url __maybe_unused);

#endif //NETDATA_API_V3_CALLS_H
