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
    unsigned long long client_id;

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
                .spinlock = SPINLOCK_INITIALIZER,
                .head = NULL,
                .count = 0,
                .reused = 0,
                .allocated = 0,
        },
        .avail = {
                .spinlock = SPINLOCK_INITIALIZER,
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

    spinlock_lock(&web_clients_cache.avail.spinlock);
    w = web_clients_cache.avail.head;
    while(w) {
        t = w;
        w = w->cache.next;
        web_client_free(t);
    }
    web_clients_cache.avail.head = NULL;
    web_clients_cache.avail.count = 0;
    spinlock_unlock(&web_clients_cache.avail.spinlock);

// DO NOT FREE THEM IF THEY ARE USED
//    spinlock_lock(&web_clients_cache.used.spinlock);
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
//    spinlock_unlock(&web_clients_cache.used.spinlock);
}

struct web_client *web_client_get_from_cache(void) {
    spinlock_lock(&web_clients_cache.avail.spinlock);
    struct web_client *w = web_clients_cache.avail.head;
    if(w) {
        // get it from avail
        DOUBLE_LINKED_LIST_REMOVE_ITEM_UNSAFE(web_clients_cache.avail.head, w, cache.prev, cache.next);
        web_clients_cache.avail.count--;

        spinlock_unlock(&web_clients_cache.avail.spinlock);
        web_client_reuse_from_cache(w);
        spinlock_lock(&web_clients_cache.used.spinlock);

        web_clients_cache.used.reused++;
    }
    else {
        spinlock_unlock(&web_clients_cache.avail.spinlock);
        w = web_client_create(&netdata_buffers_statistics.buffers_web);
        spinlock_lock(&web_clients_cache.used.spinlock);

        w->id = __atomic_add_fetch(&web_clients_cache.client_id, 1, __ATOMIC_RELAXED);
        web_clients_cache.used.allocated++;
    }

    // link it to used web clients
    DOUBLE_LINKED_LIST_PREPEND_ITEM_UNSAFE(web_clients_cache.used.head, w, cache.prev, cache.next);
    web_clients_cache.used.count++;
    spinlock_unlock(&web_clients_cache.used.spinlock);

    // initialize it
    w->use_count++;
    w->port_acl = HTTP_ACL_NONE;
    w->acl = HTTP_ACL_NONE;
    w->mode = HTTP_REQUEST_MODE_GET;
    web_client_reset_permissions(w);
    memset(w->transaction, 0, sizeof(w->transaction));
    memset(&w->auth, 0, sizeof(w->auth));

    return w;
}

void web_client_release_to_cache(struct web_client *w) {
    netdata_ssl_close(&w->ssl);

    // unlink it from the used
    spinlock_lock(&web_clients_cache.used.spinlock);
    DOUBLE_LINKED_LIST_REMOVE_ITEM_UNSAFE(web_clients_cache.used.head, w, cache.prev, cache.next);
    ssize_t used_count = (ssize_t)--web_clients_cache.used.count;
    spinlock_unlock(&web_clients_cache.used.spinlock);

    spinlock_lock(&web_clients_cache.avail.spinlock);
    if(w->use_count > 100 || (used_count > 0 && web_clients_cache.avail.count >= 2 * (size_t)used_count) || (used_count <= 10 && web_clients_cache.avail.count >= 20)) {
        spinlock_unlock(&web_clients_cache.avail.spinlock);

        // we have too many of them - free it
        web_client_free(w);
    }
    else {
        // link it to the avail
        DOUBLE_LINKED_LIST_PREPEND_ITEM_UNSAFE(web_clients_cache.avail.head, w, cache.prev, cache.next);
        web_clients_cache.avail.count++;
        spinlock_unlock(&web_clients_cache.avail.spinlock);
    }
}
