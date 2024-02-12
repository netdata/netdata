// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_WEB_API_V2_H
#define NETDATA_WEB_API_V2_H 1

#include "web_api.h"

struct web_client;

int web_client_api_request_v2(RRDHOST *host, struct web_client *w, char *url_path_endpoint);

#endif //NETDATA_WEB_API_V2_H
