// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_API_V3_CALLS_H
#define NETDATA_API_V3_CALLS_H

#include "../web_api_v3.h"

int api_v3_settings(RRDHOST *host, struct web_client *w, char *url);

#endif //NETDATA_API_V3_CALLS_H
