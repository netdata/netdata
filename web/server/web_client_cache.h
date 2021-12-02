// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_WEB_CLIENT_CACHE_H
#define NETDATA_WEB_CLIENT_CACHE_H

#include "libnetdata/libnetdata.h"
#include "web_client.h"

struct clients_cache {
    pid_t pid;

    struct web_client *used;    // the structures of the currently connected clients
    size_t used_count;          // the count the currently connected clients

    struct web_client *avail;   // the cached structures, available for future clients
    size_t avail_count;         // the number of cached structures

    size_t reused;              // the number of re-uses
    size_t allocated;           // the number of allocations
};

extern __thread struct clients_cache web_clients_cache;

extern void web_client_release(struct web_client *w);
extern struct web_client *web_client_get_from_cache_or_allocate();
extern void web_client_cache_destroy(void);
extern void web_client_cache_verify(int force);

#include "web_server.h"

#endif //NETDATA_WEB_CLIENT_CACHE_H
