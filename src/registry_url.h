#ifndef NETDATA_REGISTRY_URL_H
#define NETDATA_REGISTRY_URL_H

/**
 * @file registry_url.h
 * @brief Contains data structures and methods of a url in the registry.
 */

#include "registry_internals.h"

// ----------------------------------------------------------------------------
/// \brief URL structures.
///
/// Save memory by de-duplicating URLs
/// so instead of storing URLs all over the place
/// we store them here and we keep pointers elsewhere
struct registry_url {
    avl avl;        ///< The index
    uint32_t hash;  ///< the index hash

    uint32_t links; ///< the number of links to this URL - when none is left, we free it

    uint16_t len;   ///< the length of the URL in bytes
    char url[1];    ///< the URL - dynamically allocated to more size
};
typedef struct registry_url REGISTRY_URL; ///< An URL structure

// REGISTRY_URL INDEX

/**
 * Compare REGISTRY_URL `a` with REGISTRY_URL `b`.
 *
 * @param a REGISTRY_URL.
 * @param b REGISTRY_URL.
 * @return -1, 0, 1 if `a` is less than, equals, greater than `b`.
 */
extern int registry_url_compare(void *a, void *b);
/**
 * Delete a REGISTRY_URL from the index.
 *
 * @param u REGISTRY_URL to delete.
 * @return `u` or NULL if not found.
 */
extern REGISTRY_URL *registry_url_index_del(REGISTRY_URL *u) WARNUNUSED;
/**
 * Insert a REGISTRY_URL into the index.
 *
 * @param u REGISTRY_URL to delete.
 * @return `u` or REGISTRY_URL equal to `u`.
 */
extern REGISTRY_URL *registry_url_index_add(REGISTRY_URL *u) NEVERNULL WARNUNUSED;

// REGISTRY_URL MANAGEMENT
/**
 * Get a REGISTRY_URL from the index. If not present add it.
 *
 * @param url of REGISTRY_URL.
 * @param urllen Length of `url`
 * @return REGISTRY_URL
 */
extern REGISTRY_URL *registry_url_get(const char *url, size_t urllen) NEVERNULL;
/**
 * Add a link to registry_url.
 *
 * @param u REGISTRY_URL.
 */
extern void registry_url_link(REGISTRY_URL *u);
/**
 * Remove a link to registry_url.
 *
 * This may free REGISTRY_URL.
 *
 * @param u REGISTRY_URL.
 */
extern void registry_url_unlink(REGISTRY_URL *u);

#endif //NETDATA_REGISTRY_URL_H
