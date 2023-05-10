// SPDX-License-Identifier: GPL-3.0-or-later

#define WEB_SERVER_INTERNALS 1
#include "web_client_cache.h"

// ----------------------------------------------------------------------------
// allocate and free web_clients

// ----------------------------------------------------------------------------
// web clients caching

// When clients connect and disconnect, avoid allocating and releasing memory.
// Instead, when new clients get connected, reuse any memory previously allocated
// for serving web clients that are now disconnected.

// The size of the cache is adaptive. It caches the structures of 2x
// the number of currently connected clients.

static struct clients_cache {
    struct {
        SPINLOCK spinlock;
        struct web_client *head;    // the structures of the currently connected clients
        size_t count;               // the count the currently connected clients

        size_t allocated;           // the number of allocations
        size_t reused;              // the number of re-uses
    } used;

    struct {
        SPINLOCK spinlock;
        struct web_client *head;    // the cached structures, available for future clients
        size_t count;               // the number of cached structures
    } avail;
} web_clients_cache = {
        .used = {
                .spinlock = NETDATA_SPINLOCK_INITIALIZER,
                .head = NULL,
                .count = 0,
                .reused = 0,
                .allocated = 0,
        },
        .avail = {
                .spinlock = NETDATA_SPINLOCK_INITIALIZER,
                .head = NULL,
                .count = 0,
        },
};

// destroy the cache and free all the memory it uses
void web_client_cache_destroy(void) {
    internal_error(true, "web_client_cache has %zu used and %zu available clients, allocated %zu, reused %zu (hit %zu%%)."
        , web_clients_cache.used.count
        , web_clients_cache.avail.count
        , web_clients_cache.used.allocated
        , web_clients_cache.used.reused
        , (web_clients_cache.used.allocated + web_clients_cache.used.reused)?(web_clients_cache.used.reused * 100 / (web_clients_cache.used.allocated + web_clients_cache.used.reused)):0
        );

    struct web_client *w, *t;

    netdata_spinlock_lock(&web_clients_cache.avail.spinlock);
    w = web_clients_cache.avail.head;
    while(w) {
        t = w;
        w = w->cache.next;
        web_client_free(t);
    }
    web_clients_cache.avail.head = NULL;
    web_clients_cache.avail.count = 0;
    netdata_spinlock_unlock(&web_clients_cache.avail.spinlock);

// DO NOT FREE THEM IF THEY ARE USED
//    netdata_spinlock_lock(&web_clients_cache.used.spinlock);
//    w = web_clients_cache.used.head;
//    while(w) {
//        t = w;
//        w = w->next;
//        web_client_free(t);
//    }
//    web_clients_cache.used.head = NULL;
//    web_clients_cache.used.count = 0;
//    web_clients_cache.used.reused = 0;
//    web_clients_cache.used.allocated = 0;
//    netdata_spinlock_unlock(&web_clients_cache.used.spinlock);
}

struct web_client *web_client_get_from_cache(void) {
    netdata_spinlock_lock(&web_clients_cache.avail.spinlock);
    struct web_client *w = web_clients_cache.avail.head;
    if(w) {
        // get it from avail
        DOUBLE_LINKED_LIST_REMOVE_ITEM_UNSAFE(web_clients_cache.avail.head, w, cache.prev, cache.next);
        web_clients_cache.avail.count--;
        netdata_spinlock_unlock(&web_clients_cache.avail.spinlock);

        web_client_zero(w);

        netdata_spinlock_lock(&web_clients_cache.used.spinlock);
        web_clients_cache.used.reused++;
    }
    else {
        netdata_spinlock_unlock(&web_clients_cache.avail.spinlock);

        // allocate it
        w = web_client_create(&netdata_buffers_statistics.buffers_web);

#ifdef ENABLE_HTTPS
        w->ssl.flags = NETDATA_SSL_START;
        debug(D_WEB_CLIENT_ACCESS,"Starting SSL structure with (w->ssl = NULL, w->accepted = %u)", w->ssl.flags);
#endif

        netdata_spinlock_lock(&web_clients_cache.used.spinlock);
        web_clients_cache.used.allocated++;
    }

    // link it to used web clients
    DOUBLE_LINKED_LIST_PREPEND_ITEM_UNSAFE(web_clients_cache.used.head, w, cache.prev, cache.next);
    web_clients_cache.used.count++;
    netdata_spinlock_unlock(&web_clients_cache.used.spinlock);

    // initialize it
    w->use_count++;
    w->id = global_statistics_web_client_connected();
    w->mode = WEB_CLIENT_MODE_GET;

    return w;
}

void web_client_release_to_cache(struct web_client *w) {
    // unlink it from the used
    netdata_spinlock_lock(&web_clients_cache.used.spinlock);
    DOUBLE_LINKED_LIST_REMOVE_ITEM_UNSAFE(web_clients_cache.used.head, w, cache.prev, cache.next);
    ssize_t used_count = (ssize_t)--web_clients_cache.used.count;
    netdata_spinlock_unlock(&web_clients_cache.used.spinlock);

    netdata_spinlock_lock(&web_clients_cache.avail.spinlock);
    if(w->use_count > 100 || (used_count > 0 && web_clients_cache.avail.count >= 2 * (size_t)used_count) || (used_count <= 10 && web_clients_cache.avail.count >= 20)) {
        netdata_spinlock_unlock(&web_clients_cache.avail.spinlock);

        // we have too many of them - free it
        web_client_free(w);
    }
    else {
        // link it to the avail
        DOUBLE_LINKED_LIST_PREPEND_ITEM_UNSAFE(web_clients_cache.avail.head, w, cache.prev, cache.next);
        web_clients_cache.avail.count++;
        netdata_spinlock_unlock(&web_clients_cache.avail.spinlock);
    }
}
