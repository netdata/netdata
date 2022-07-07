
#ifndef ARRAYALLOC_H
#define ARRAYALLOC_H 1

#include "../libnetdata.h"

typedef struct arrayalloc {
    size_t element_size;
    size_t elements;
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
        struct arrayalloc_page *first_page;
        struct arrayalloc_page *last_page;
    } internal;
} ARAL;

extern ARAL *arrayalloc_create(size_t element_size, size_t elements, const char *filename, char **cache_dir);
extern void *arrayalloc_mallocz(ARAL *ar);
extern void arrayalloc_freez(ARAL *ar, void *ptr);

#endif // ARRAYALLOC_H
