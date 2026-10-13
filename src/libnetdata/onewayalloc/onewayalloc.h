#ifndef ONEWAYALLOC_H
#define ONEWAYALLOC_H 1

#include "../libnetdata.h"

typedef void ONEWAYALLOC;

ONEWAYALLOC *onewayalloc_create(size_t size_hint);
void onewayalloc_destroy(ONEWAYALLOC *owa);

// Reset invalidates all allocations and retains every page for reuse.
// The allocation cursor is rewound in O(1); retained overflow pages are
// rewound lazily as needed. Only a fitting unused page is moved after
// the current page; skipped pages remain available to later allocations.
// Capacity is released only by destroy, so the caller owns the lifetime
// of the arena's high-water footprint.
//
// Important: reset does NOT zero retained pages. Subsequent
// onewayalloc_mallocz() calls may return memory that still holds bytes
// from the previous burst — this matches the mallocz() contract (the "z"
// means "fatal on failure", not "zeroed"). Callers that need zeroed
// memory must use onewayalloc_callocz() just as they would against a
// freshly-created arena; the reset path does not relax that contract.
// Under ASAN, allocations bypass the arena and callers must free them
// individually with onewayalloc_freez(); reset remains a no-op.
void onewayalloc_reset(ONEWAYALLOC *owa);

size_t onewayalloc_mul_or_fatal(size_t nmemb, size_t size, const char *context);
size_t onewayalloc_mul3_or_fatal(size_t nmemb1, size_t nmemb2, size_t size, const char *context);

void *onewayalloc_mallocz(ONEWAYALLOC *owa, size_t size);
void *onewayalloc_callocz(ONEWAYALLOC *owa, size_t nmemb, size_t size);
char *onewayalloc_strdupz(ONEWAYALLOC *owa, const char *s);
void *onewayalloc_memdupz(ONEWAYALLOC *owa, const void *src, size_t size);
void onewayalloc_freez(ONEWAYALLOC *owa, const void *ptr);

void *onewayalloc_doublesize(ONEWAYALLOC *owa, const void *src, size_t oldsize);

size_t onewayalloc_allocated_memory(void);

#endif // ONEWAYALLOC_H
