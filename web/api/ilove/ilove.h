// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_WEB_API_ILOVE_H
#define NETDATA_WEB_API_ILOVE_H 1

#include "libnetdata/libnetdata.h"
#include "web/server/web_client.h"

int web_client_api_request_v2_ilove(RRDHOST *host, struct web_client *w, char *url);

#include "web/api/web_api_v1.h"

#endif /* NETDATA_WEB_API_ILOVE_H */
