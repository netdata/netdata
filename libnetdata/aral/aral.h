
#ifndef ARAL_H
#define ARAL_H 1

#include "../libnetdata.h"

#define ARAL_MAX_NAME 23

typedef struct aral ARAL;

struct aral_statistics {
    struct {
        size_t allocations;
        size_t allocated_bytes;
    } structures;

    struct {
        size_t allocations;
        size_t allocated_bytes;
        size_t used_bytes;
    } malloc;

    struct {
        size_t allocations;
        size_t allocated_bytes;
        size_t used_bytes;
    } mmap;
};

ARAL *aral_create(const char *name, size_t element_size, size_t initial_page_elements, size_t max_page_size,
                  struct aral_statistics *stats, const char *filename, char **cache_dir, bool mmap, bool lockless);
size_t aral_element_size(ARAL *ar);
size_t aral_overhead(ARAL *ar);
size_t aral_structures(ARAL *ar);
struct aral_statistics *aral_statistics(ARAL *ar);
size_t aral_structures_from_stats(struct aral_statistics *stats);
size_t aral_overhead_from_stats(struct aral_statistics *stats);

ARAL *aral_by_size_acquire(size_t size);
void aral_by_size_release(ARAL *ar);
size_t aral_by_size_structures(void);
size_t aral_by_size_overhead(void);
struct aral_statistics *aral_by_size_statistics(void);

int aral_unittest(size_t elements);

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
