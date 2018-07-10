// SPDX-License-Identifier: GPL-3.0+
#ifndef NETDATA_REGISTRY_URL_H
#define NETDATA_REGISTRY_URL_H

#include "registry_internals.h"

// ----------------------------------------------------------------------------
// URL structures
// Save memory by de-duplicating URLs
// so instead of storing URLs all over the place
// we store them here and we keep pointers elsewhere

struct registry_url {
    avl avl;
    uint32_t hash;  // the index hash

    uint32_t links; // the number of links to this URL - when none is left, we free it

    uint16_t len;   // the length of the URL in bytes
    char url[1];    // the URL - dynamically allocated to more size
};
typedef struct registry_url REGISTRY_URL;

// REGISTRY_URL INDEX
extern int registry_url_compare(void *a, void *b);
extern REGISTRY_URL *registry_url_index_del(REGISTRY_URL *u) WARNUNUSED;
extern REGISTRY_URL *registry_url_index_add(REGISTRY_URL *u) NEVERNULL WARNUNUSED;

// REGISTRY_URL MANAGEMENT
extern REGISTRY_URL *registry_url_get(const char *url, size_t urllen) NEVERNULL;
extern void registry_url_link(REGISTRY_URL *u);
extern void registry_url_unlink(REGISTRY_URL *u);

#endif //NETDATA_REGISTRY_URL_H
