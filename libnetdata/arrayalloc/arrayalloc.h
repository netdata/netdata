
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
        netdata_mutex_t mutex;
        struct arrayalloc_page *pages;
    } internal;
} ARAL;

ARAL *arrayalloc_create(size_t element_size, size_t elements, const char *filename, char **cache_dir, bool mmap);
int aral_unittest(size_t elements);

#ifdef NETDATA_TRACE_ALLOCATIONS

#define arrayalloc_mallocz(ar) arrayalloc_mallocz_int(ar, __FILE__, __FUNCTION__, __LINE__)
#define arrayalloc_freez(ar, ptr) arrayalloc_freez_int(ar, ptr, __FILE__, __FUNCTION__, __LINE__)

void *arrayalloc_mallocz_int(ARAL *ar, const char *file, const char *function, size_t line);
void arrayalloc_freez_int(ARAL *ar, void *ptr, const char *file, const char *function, size_t line);

#else // NETDATA_TRACE_ALLOCATIONS

void *arrayalloc_mallocz(ARAL *ar);
void arrayalloc_freez(ARAL *ar, void *ptr);

#endif // NETDATA_TRACE_ALLOCATIONS

#endif // ARRAYALLOC_H
