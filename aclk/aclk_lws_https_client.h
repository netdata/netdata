// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_LWS_HTTPS_CLIENT_H
#define NETDATA_LWS_HTTPS_CLIENT_H

#include "../daemon/common.h"
#include "libnetdata/libnetdata.h"

#define DATAMAXLEN 1024*16

#ifdef ACLK_LWS_HTTPS_CLIENT_INTERNAL
#define ACLK_CONTENT_TYPE_JSON "application/json"
#define SEND_HTTPS_REQUEST_TIMEOUT 30
#endif

int aclk_send_https_request(char *method, char *host, char *port, char *url, char *b, size_t b_size, char *payload);

#endif /* NETDATA_LWS_HTTPS_CLIENT_H */
