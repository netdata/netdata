
#ifndef ARRAYALLOC_H
#define ARRAYALLOC_H 1

#include "../libnetdata.h"

typedef struct arrayalloc {
    size_t element_size;
    size_t elements;
    const char *filename;
    char **cache_dir;
    struct {
        bool lockless;
        bool initialized;
        size_t file_number;
        size_t natural_page_size;
        size_t allocation_multiplier;
        size_t total_allocated;
        netdata_mutex_t mutex;
        struct arrayalloc_page *pages;
        void *free_list;
    } internal;
} ARAL;

extern ARAL *arrayalloc_create(size_t element_size, size_t elements, const char *filename, char **cache_dir);
extern void *arrayalloc_mallocz(ARAL *ar);
extern void arrayalloc_freez(ARAL *ar, void *ptr);

#endif // ARRAYALLOC_H
