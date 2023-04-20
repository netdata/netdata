// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_WEB_CLIENT_CACHE_H
#define NETDATA_WEB_CLIENT_CACHE_H

#include "libnetdata/libnetdata.h"
#include "web_client.h"

void web_client_release_to_cache(struct web_client *w);
struct web_client *web_client_get_from_cache(void);
void web_client_cache_destroy(void);

#include "web_server.h"

#endif //NETDATA_WEB_CLIENT_CACHE_H
