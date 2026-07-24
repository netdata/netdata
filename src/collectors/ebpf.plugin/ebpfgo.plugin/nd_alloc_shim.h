// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_ND_ALLOC_SHIM_H
#define NETDATA_ND_ALLOC_SHIM_H 1

/* Provide callocz / mallocz / reallocz / freez for translation units that are
 * compiled via CGo and therefore cannot pull in the full libnetdata header
 * chain (nd-mallocz.h requires common.h which requires autoconf outputs).
 *
 * When NETDATA_ND_MALLOCZ_H is defined the real libnetdata versions are
 * already visible — do nothing so this shim does not shadow them.
 *
 * OOM semantics are identical to the libnetdata variants: abort the process
 * rather than return NULL. */

#ifndef NETDATA_ND_MALLOCZ_H

#include <stdio.h>
#include <stdlib.h>

static inline void *callocz(size_t nmemb, size_t size) {
    void *p = calloc(nmemb, size);
    if (!p && nmemb && size) { fprintf(stderr, "callocz: out of memory\n"); abort(); }
    return p;
}

static inline void *mallocz(size_t size) {
    void *p = malloc(size);
    if (!p && size) { fprintf(stderr, "mallocz: out of memory\n"); abort(); }
    return p;
}

static inline void *reallocz(void *ptr, size_t size) {
    void *p = realloc(ptr, size);
    if (!p && size) { fprintf(stderr, "reallocz: out of memory\n"); abort(); }
    return p;
}

static inline void freez(void *ptr) {
    free(ptr);
}

#endif /* !NETDATA_ND_MALLOCZ_H */

#endif /* NETDATA_ND_ALLOC_SHIM_H */
