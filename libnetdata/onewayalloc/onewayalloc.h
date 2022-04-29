#ifndef ONEWAYALLOC_H
#define ONEWAYALLOC_H 1

#include "../libnetdata.h"

typedef void ONEWAYALLOC;

extern ONEWAYALLOC *onewayalloc_create(size_t size_hint);
extern void onewayalloc_destroy(ONEWAYALLOC *owa);

extern void *onewayalloc_mallocz(ONEWAYALLOC *owa, size_t size);
extern char *onewayalloc_strdupz(ONEWAYALLOC *owa, const char *s);
extern void *onewayalloc_memdupz(ONEWAYALLOC *owa, const void *src, size_t size);

#endif // ONEWAYALLOC_H
