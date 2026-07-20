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
        size_t reserved;            // accepted for caching, currently being reset
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
                .reserved = 0,
        },
};

// destroy the cache and free all the memory it uses
void web_client_cache_destroy(void) {
    internal_error(true, "web_client_cache has %zu used, %zu available, and %zu reserved clients, allocated %zu, reused %zu (hit %zu%%)."
        , web_clients_cache.used.count
        , web_clients_cache.avail.count
        , web_clients_cache.avail.reserved
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
    web_client_clear_mcp_preview_key(w);
    memset(w->transaction, 0, sizeof(w->transaction));
    memset(w->mcp_session_id, 0, sizeof(w->mcp_session_id));
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
    size_t available = web_clients_cache.avail.count + web_clients_cache.avail.reserved;
    bool discard = w->use_count > 100 ||
                   (used_count > 0 && available >= 2 * (size_t)used_count) ||
                   (used_count <= 10 && available >= 20);
    if(!discard)
        web_clients_cache.avail.reserved++;
    spinlock_unlock(&web_clients_cache.avail.spinlock);

    if(discard) {
        // we have too many of them - free it
        web_client_free(w);
        return;
    }

    // Do not park request-sized allocations until another client needs this slot.
    web_client_reset_allocations_for_reuse(w);

    spinlock_lock(&web_clients_cache.avail.spinlock);
    if(unlikely(!web_clients_cache.avail.reserved))
        fatal("WEB CLIENT CACHE: publishing a client without a reservation");
    web_clients_cache.avail.reserved--;
    DOUBLE_LINKED_LIST_PREPEND_ITEM_UNSAFE(web_clients_cache.avail.head, w, cache.prev, cache.next);
    web_clients_cache.avail.count++;
    spinlock_unlock(&web_clients_cache.avail.spinlock);
}

static int web_client_cache_unittest_grow_allocations(struct web_client *w) {
    int errors = 0;
    BUFFER *buffers[] = {
        w->url_as_received,
        w->url_for_logging,
        w->url_path_decoded,
        w->url_query_string_decoded,
        w->response.header_output,
        w->response.header,
        w->response.data,
    };

    for(size_t i = 0; i < _countof(buffers); i++) {
        buffer_need_bytes(buffers[i], NETDATA_WEB_CLIENT_CACHE_MAX_BUFFER_SIZE + 1);
        if(buffers[i]->size <= NETDATA_WEB_CLIENT_CACHE_MAX_BUFFER_SIZE)
            errors++;
    }

    if(!w->payload)
        w->payload = buffer_create(0, w->statistics.memory_accounting);
    buffer_need_bytes(w->payload, NETDATA_WEB_CLIENT_CACHE_MAX_BUFFER_SIZE + 1);
    if(w->payload->size <= NETDATA_WEB_CLIENT_CACHE_MAX_BUFFER_SIZE)
        errors++;

    return errors;
}

int web_client_cache_unittest(void) {
    int errors = 0;
    struct web_client *w = web_client_get_from_cache();

    errors += web_client_cache_unittest_grow_allocations(w);
    web_client_release_to_cache(w);

    bool removed = false;
    spinlock_lock(&web_clients_cache.avail.spinlock);
    if(web_clients_cache.avail.reserved)
        errors++;

    struct web_client *cached;
    for(cached = web_clients_cache.avail.head; cached && cached != w; cached = cached->cache.next) {
        ;
    }

    if(!cached)
        errors++;
    else {
        BUFFER *cached_buffers[] = {
            w->url_as_received,
            w->url_for_logging,
            w->url_path_decoded,
            w->url_query_string_decoded,
            w->response.header_output,
            w->response.header,
            w->response.data,
        };

        for(size_t i = 0; i < _countof(cached_buffers); i++) {
            if(cached_buffers[i]->size > NETDATA_WEB_CLIENT_CACHE_MAX_BUFFER_SIZE)
                errors++;
        }

        if(w->payload)
            errors++;

        DOUBLE_LINKED_LIST_REMOVE_ITEM_UNSAFE(web_clients_cache.avail.head, w, cache.prev, cache.next);
        web_clients_cache.avail.count--;
        removed = true;
    }
    spinlock_unlock(&web_clients_cache.avail.spinlock);

    if(removed)
        web_client_free(w);

    w = web_client_get_from_cache();
    errors += web_client_cache_unittest_grow_allocations(w);
    w->use_count = 101;
    uintptr_t discarded_address = (uintptr_t)w;

    spinlock_lock(&web_clients_cache.avail.spinlock);
    size_t available_before_discard = web_clients_cache.avail.count;
    spinlock_unlock(&web_clients_cache.avail.spinlock);

    web_client_release_to_cache(w);

    spinlock_lock(&web_clients_cache.avail.spinlock);
    if(web_clients_cache.avail.count != available_before_discard || web_clients_cache.avail.reserved)
        errors++;

    for(cached = web_clients_cache.avail.head; cached; cached = cached->cache.next) {
        if((uintptr_t)cached == discarded_address) {
            errors++;
            break;
        }
    }
    spinlock_unlock(&web_clients_cache.avail.spinlock);

    if(errors)
        fprintf(stderr, "WEB CLIENT CACHE: %d test(s) failed\n", errors);

    return errors;
}
