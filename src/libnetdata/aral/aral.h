
#ifndef ARAL_H
#define ARAL_H 1

#include "../libnetdata.h"

#define ARAL_MAX_NAME 23

typedef struct aral ARAL;

struct aral_page_type_stats {
    PAD64(size_t) allocations;
    PAD64(size_t) allocated_bytes;
    PAD64(size_t) used_bytes;
    PAD64(size_t) padding_bytes;
};

struct aral_statistics {
    struct {
        PAD64(size_t) allocations;
        PAD64(size_t) allocated_bytes;
    } structures;

    struct aral_page_type_stats malloc;
    struct aral_page_type_stats mmap;
};

// --------------------------------------------------------------------------------------------------------------------

const char *aral_name(ARAL *ar);

ARAL *aral_create(const char *name, size_t element_size, size_t initial_page_elements, size_t max_page_size,
                  struct aral_statistics *stats, const char *filename, const char **cache_dir,
                  bool mmap, bool lockless, bool dont_dump);

// --------------------------------------------------------------------------------------------------------------------

// return the size of the element, as requested
size_t aral_requested_element_size(ARAL *ar);

// return the exact memory footprint of the elements
size_t aral_actual_element_size(ARAL *ar);

// --------------------------------------------------------------------------------------------------------------------

size_t aral_optimal_malloc_page_size(void);
void aral_optimal_malloc_page_size_set(size_t size);

// --------------------------------------------------------------------------------------------------------------------

/*
 *
 * The total memory used by ARAL is:
 *
 *    total = structures + used + free + padding
 *
 * or
 *
 *    total = structures + allocated + padding
 *
 * always:
 *
 *    allocated = used + free
 *
 * Hints:
 *  - allocated, used and free are about the requested element size.
 *  - structures includes the extension of the elements for the metadata aral needs.
 *  - padding is lost due to alignment requirements
 *
 */

size_t aral_structures_bytes(ARAL *ar);
size_t aral_free_bytes(ARAL *ar);
size_t aral_used_bytes(ARAL *ar);
size_t aral_padding_bytes(ARAL *ar);

struct aral_statistics *aral_get_statistics(ARAL *ar);

size_t aral_structures_bytes_from_stats(struct aral_statistics *stats);
size_t aral_free_bytes_from_stats(struct aral_statistics *stats);
size_t aral_used_bytes_from_stats(struct aral_statistics *stats);
size_t aral_padding_bytes_from_stats(struct aral_statistics *stats);

// --------------------------------------------------------------------------------------------------------------------

ARAL *aral_by_size_acquire(size_t size);
void aral_by_size_release(ARAL *ar);

size_t aral_by_size_structures_bytes(void);
size_t aral_by_size_free_bytes(void);
size_t aral_by_size_used_bytes(void);
size_t aral_by_size_padding_bytes(void);

struct aral_statistics *aral_by_size_statistics(void);

// --------------------------------------------------------------------------------------------------------------------

#ifdef NETDATA_TRACE_ALLOCATIONS

#define aral_callocz(ar) aral_callocz_internal(ar, false, __FILE__, __FUNCTION__, __LINE__)
#define aral_callocz_marked(ar) aral_callocz_internal(ar, true, __FILE__, __FUNCTION__, __LINE__)
#define aral_mallocz(ar) aral_mallocz_internal(ar, false, __FILE__, __FUNCTION__, __LINE__)
#define aral_mallocz_marked(ar) aral_mallocz_internal(ar, true, __FILE__, __FUNCTION__, __LINE__)
#define aral_freez(ar, ptr) aral_freez_internal(ar, ptr, __FILE__, __FUNCTION__, __LINE__)
#define aral_destroy(ar) aral_destroy_internal(ar, __FILE__, __FUNCTION__, __LINE__)

void *aral_callocz_internal(ARAL *ar, bool marked, const char *file, const char *function, size_t line);
void *aral_mallocz_internal(ARAL *ar, bool marked, const char *file, const char *function, size_t line);
void aral_freez_internal(ARAL *ar, void *ptr, const char *file, const char *function, size_t line);
void aral_destroy_internal(ARAL *ar, const char *file, const char *function, size_t line);

#else // NETDATA_TRACE_ALLOCATIONS

#define aral_mallocz(ar) aral_mallocz_internal(ar, false)
#define aral_mallocz_marked(ar) aral_mallocz_internal(ar, true)
#define aral_callocz(ar) aral_callocz_internal(ar, false)
#define aral_callocz_marked(ar) aral_callocz_internal(ar, true)
#define aral_freez(ar, ptr) aral_freez_internal(ar, ptr)
#define aral_destroy(ar) aral_destroy_internal(ar)


void *aral_callocz_internal(ARAL *ar, bool marked);
void *aral_mallocz_internal(ARAL *ar, bool marked);
void aral_freez_internal(ARAL *ar, void *ptr);
void aral_destroy_internal(ARAL *ar);

void aral_unmark_allocation(ARAL *ar, void *ptr);

// --------------------------------------------------------------------------------------------------------------------

int aral_unittest(size_t elements);

#endif // NETDATA_TRACE_ALLOCATIONS

#endif // ARAL_H
