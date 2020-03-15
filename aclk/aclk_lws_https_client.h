// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_LWS_HTTPS_CLIENT_H
#define NETDATA_LWS_HTTPS_CLIENT_H

#include "../daemon/common.h"
#include "libnetdata/libnetdata.h"

#ifdef ACLK_LWS_HTTPS_CLIENT_INTERNAL
#define ACLK_CONTENT_TYPE_JSON "application/json"
#define SEND_HTTPS_REQUEST_TIMEOUT 30
#endif

int send_https_request(char *method, char *host, char *port, char *url, BUFFER *b, char *payload);

#endif /* NETDATA_LWS_HTTPS_CLIENT_H */
