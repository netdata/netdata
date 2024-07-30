// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_WEB_API_V1_H
#define NETDATA_WEB_API_V1_H 1

#include "web_api.h"

int web_client_api_request_v1(RRDHOST *host, struct web_client *w, char *url_path_endpoint);

extern char *api_secret;

#endif //NETDATA_WEB_API_V1_H
