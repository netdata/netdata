#ifndef ONEWAYALLOC_H
#define ONEWAYALLOC_H 1

#include "../libnetdata.h"

typedef void ONEWAYALLOC;

ONEWAYALLOC *onewayalloc_create(size_t size_hint);
void onewayalloc_destroy(ONEWAYALLOC *owa);

void *onewayalloc_mallocz(ONEWAYALLOC *owa, size_t size);
void *onewayalloc_callocz(ONEWAYALLOC *owa, size_t nmemb, size_t size);
char *onewayalloc_strdupz(ONEWAYALLOC *owa, const char *s);
void *onewayalloc_memdupz(ONEWAYALLOC *owa, const void *src, size_t size);
void onewayalloc_freez(ONEWAYALLOC *owa, const void *ptr);

void *onewayalloc_doublesize(ONEWAYALLOC *owa, const void *src, size_t oldsize);

size_t onewayalloc_allocated_memory(void);

#endif // ONEWAYALLOC_H
