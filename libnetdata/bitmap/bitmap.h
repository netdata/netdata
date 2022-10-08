#ifndef LIBNETDATA_BITMAP_H
#define LIBNETDATA_BITMAP_H

#include "../libnetdata.h"

#ifdef __cplusplus
extern "C" {
#endif

typedef void * bitmap_t;

bitmap_t bitmap_new(size_t capacity);
void bitmap_delete(bitmap_t bitmap);

void bitmap_set(bitmap_t bitmap, size_t index, bool value);
bool bitmap_get(bitmap_t bitmap, size_t index);

#ifdef __cplusplus
}
#endif

#endif /* LIBNETDATA_BITMAP_H */
