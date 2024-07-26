// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_WEB_API_V3_H
#define NETDATA_WEB_API_V3_H

#include "web_api.h"

struct web_client;

int web_client_api_request_v3(RRDHOST *host, struct web_client *w, char *url_path_endpoint);

#endif //NETDATA_WEB_API_V3_H
