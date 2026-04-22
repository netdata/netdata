#ifndef ONEWAYALLOC_H
#define ONEWAYALLOC_H 1

#include "../libnetdata.h"

typedef void ONEWAYALLOC;

ONEWAYALLOC *onewayalloc_create(size_t size_hint);
void onewayalloc_destroy(ONEWAYALLOC *owa);

// Reset the arena to an empty state without destroying it. Frees every
// page except the head, and rolls the head page's bump pointer back to
// its initial offset so the arena is reusable. Intended for callers that
// do many short bursts of allocations back-to-back (e.g. evaluating all
// the alerts of one host) and want to amortise the mmap/munmap cost of
// create/destroy across the whole burst.
//
// Important: reset does NOT zero the retained head page. Subsequent
// onewayalloc_mallocz() calls may return memory that still holds bytes
// from the previous burst — this matches the mallocz() contract (the "z"
// means "fatal on failure", not "zeroed"). Callers that need zeroed
// memory must use onewayalloc_callocz() just as they would against a
// freshly-created arena; the reset path does not relax that contract.
void onewayalloc_reset(ONEWAYALLOC *owa);

void *onewayalloc_mallocz(ONEWAYALLOC *owa, size_t size);
void *onewayalloc_callocz(ONEWAYALLOC *owa, size_t nmemb, size_t size);
char *onewayalloc_strdupz(ONEWAYALLOC *owa, const char *s);
void *onewayalloc_memdupz(ONEWAYALLOC *owa, const void *src, size_t size);
void onewayalloc_freez(ONEWAYALLOC *owa, const void *ptr);

void *onewayalloc_doublesize(ONEWAYALLOC *owa, const void *src, size_t oldsize);

size_t onewayalloc_allocated_memory(void);

#endif // ONEWAYALLOC_H
