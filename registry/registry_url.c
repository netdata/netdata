// SPDX-License-Identifier: GPL-3.0-or-later

#include "../daemon/common.h"
#include "registry_internals.h"

// ----------------------------------------------------------------------------
// REGISTRY_URL

int registry_url_compare(void *a, void *b) {
    if(((REGISTRY_URL *)a)->hash < ((REGISTRY_URL *)b)->hash) return -1;
    else if(((REGISTRY_URL *)a)->hash > ((REGISTRY_URL *)b)->hash) return 1;
    else return strcmp(((REGISTRY_URL *)a)->url, ((REGISTRY_URL *)b)->url);
}

inline REGISTRY_URL *registry_url_index_add(REGISTRY_URL *u) {
    return (REGISTRY_URL *)avl_insert(&(registry.registry_urls_root_index), (avl *)(u));
}

inline REGISTRY_URL *registry_url_index_del(REGISTRY_URL *u) {
    return (REGISTRY_URL *)avl_remove(&(registry.registry_urls_root_index), (avl *)(u));
}

REGISTRY_URL *registry_url_get(const char *url, size_t urllen) {
    // protection from too big URLs
    if(urllen > registry.max_url_length)
        urllen = registry.max_url_length;

    debug(D_REGISTRY, "Registry: registry_url_get('%s', %zu)", url, urllen);

    char buf[sizeof(REGISTRY_URL) + urllen]; // no need for +1, 1 is already in REGISTRY_URL
    REGISTRY_URL *n = (REGISTRY_URL *)&buf[0];
    n->len = (uint16_t)urllen;
    strncpyz(n->url, url, n->len);
    n->hash = simple_hash(n->url);

    REGISTRY_URL *u = (REGISTRY_URL *)avl_search(&(registry.registry_urls_root_index), (avl *)n);
    if(!u) {
        debug(D_REGISTRY, "Registry: registry_url_get('%s', %zu): allocating %zu bytes", url, urllen, sizeof(REGISTRY_URL) + urllen);
        u = callocz(1, sizeof(REGISTRY_URL) + urllen); // no need for +1, 1 is already in REGISTRY_URL

        // a simple strcpy() should do the job
        // but I prefer to be safe, since the caller specified urllen
        u->len = (uint16_t)urllen;
        strncpyz(u->url, url, u->len);
        u->links = 0;
        u->hash = simple_hash(u->url);

        registry.urls_memory += sizeof(REGISTRY_URL) + urllen; // no need for +1, 1 is already in REGISTRY_URL

        debug(D_REGISTRY, "Registry: registry_url_get('%s'): indexing it", url);
        n = registry_url_index_add(u);
        if(n != u) {
            error("INTERNAL ERROR: registry_url_get(): url '%s' already exists in the registry as '%s'", u->url, n->url);
            freez(u);
            u = n;
        }
        else
            registry.urls_count++;
    }

    return u;
}

void registry_url_link(REGISTRY_URL *u) {
    u->links++;
    debug(D_REGISTRY, "Registry: registry_url_link('%s'): URL has now %u links", u->url, u->links);
}

void registry_url_unlink(REGISTRY_URL *u) {
    u->links--;
    if(!u->links) {
        debug(D_REGISTRY, "Registry: registry_url_unlink('%s'): No more links for this URL", u->url);
        REGISTRY_URL *n = registry_url_index_del(u);
        if(!n) {
            error("INTERNAL ERROR: registry_url_unlink('%s'): cannot find url in index", u->url);
        }
        else {
            if(n != u) {
                error("INTERNAL ERROR: registry_url_unlink('%s'): deleted different url '%s'", u->url, n->url);
            }

            registry.urls_memory -= sizeof(REGISTRY_URL) + n->len; // no need for +1, 1 is already in REGISTRY_URL
            freez(n);
        }
    }
    else
        debug(D_REGISTRY, "Registry: registry_url_unlink('%s'): URL has %u links left", u->url, u->links);
}
