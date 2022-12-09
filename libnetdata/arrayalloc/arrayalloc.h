
#ifndef ARRAYALLOC_H
#define ARRAYALLOC_H 1

#include "../libnetdata.h"

typedef struct arrayalloc {
    size_t requested_element_size;
    size_t initial_elements;
    const char *filename;
    char **cache_dir;
    bool use_mmap;

    // private members - do not touch
    struct {
        bool mmap;
        bool lockless;
        bool initialized;
        size_t element_size;
        size_t page_ptr_offset;
        size_t file_number;
        size_t natural_page_size;
        size_t allocation_multiplier;
        size_t max_alloc_size;
        SPINLOCK spinlock;
        struct arrayalloc_page *pages;
    } internal;
} ARAL;

ARAL *arrayalloc_create(size_t element_size, size_t elements, const char *filename, char **cache_dir, bool mmap, bool lockless);
int aral_unittest(size_t elements);

#ifdef NETDATA_TRACE_ALLOCATIONS

#define arrayalloc_mallocz(ar) arrayalloc_mallocz_internal(ar, __FILE__, __FUNCTION__, __LINE__)
#define arrayalloc_freez(ar, ptr) arrayalloc_freez_internal(ar, ptr, __FILE__, __FUNCTION__, __LINE__)
#define arrayalloc_destroy(ar) arrayalloc_destroy_internal(ar, __FILE__, __FUNCTION__, __LINE__)

void *arrayalloc_mallocz_internal(ARAL *ar, const char *file, const char *function, size_t line);
void arrayalloc_freez_internal(ARAL *ar, void *ptr, const char *file, const char *function, size_t line);
void arrayalloc_destroy_internal(ARAL *ar, const char *file, const char *function, size_t line);

#else // NETDATA_TRACE_ALLOCATIONS

#define arrayalloc_mallocz(ar) arrayalloc_mallocz_internal(ar)
#define arrayalloc_freez(ar, ptr) arrayalloc_freez_internal(ar, ptr)
#define arrayalloc_destroy(ar) arrayalloc_destroy_internal(ar)

void *arrayalloc_mallocz_internal(ARAL *ar);
void arrayalloc_freez_internal(ARAL *ar, void *ptr);
void arrayalloc_destroy_internal(ARAL *ar);

#endif // NETDATA_TRACE_ALLOCATIONS

#endif // ARRAYALLOC_H
