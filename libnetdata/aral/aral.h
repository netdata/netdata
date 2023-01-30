
#ifndef ARAL_H
#define ARAL_H 1

#include "../libnetdata.h"

#define ARAL_MAX_NAME 23

typedef struct aral ARAL;

ARAL *aral_create(const char *name, size_t element_size, size_t initial_page_elements, size_t max_page_elements, const char *filename, char **cache_dir, bool mmap, bool lockless);
int aral_unittest(size_t elements);
void aral_get_size_statistics(size_t *structures, size_t *malloc_allocated, size_t *malloc_used, size_t *mmap_allocated, size_t *mmap_used);

#ifdef NETDATA_TRACE_ALLOCATIONS

#define aral_mallocz(ar) aral_mallocz_internal(ar, __FILE__, __FUNCTION__, __LINE__)
#define aral_freez(ar, ptr) aral_freez_internal(ar, ptr, __FILE__, __FUNCTION__, __LINE__)
#define aral_destroy(ar) aral_destroy_internal(ar, __FILE__, __FUNCTION__, __LINE__)

void *aral_mallocz_internal(ARAL *ar, const char *file, const char *function, size_t line);
void aral_freez_internal(ARAL *ar, void *ptr, const char *file, const char *function, size_t line);
void aral_destroy_internal(ARAL *ar, const char *file, const char *function, size_t line);

#else // NETDATA_TRACE_ALLOCATIONS

#define aral_mallocz(ar) aral_mallocz_internal(ar)
#define aral_freez(ar, ptr) aral_freez_internal(ar, ptr)
#define aral_destroy(ar) aral_destroy_internal(ar)

void *aral_mallocz_internal(ARAL *ar);
void aral_freez_internal(ARAL *ar, void *ptr);
void aral_destroy_internal(ARAL *ar);

#endif // NETDATA_TRACE_ALLOCATIONS

#endif // ARAL_H
