// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_HTTP_HEADER_H
#define NETDATA_HTTP_HEADER_H

#include "web_api.h"

struct web_client;
char *http_header_parse_line(struct web_client *w, char *s);

#endif //NETDATA_HTTP_HEADER_H
