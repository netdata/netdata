#include "../libnetdata.h"
#include "arrayalloc.h"
#include "daemon/common.h"

typedef struct arrayalloc_free {
    size_t size;
    struct arrayalloc_free *next;
} ARAL_FREE;

typedef struct arrayalloc_page {
    size_t size;                  // the total size of the page
    uint8_t *data;
    struct arrayalloc_page *next; // the next page on the list
} ARAL_PAGE;

ARAL *arrayalloc_create(size_t element_size, size_t elements, const char *filename, char **cache_dir) {
    ARAL *ar = callocz(1, sizeof(ARAL));
    ar->element_size = element_size;
    ar->elements = elements;
    ar->filename = filename;
    ar->cache_dir = cache_dir;
    return ar;
}

#define ARAL_NATURAL_ALIGNMENT  (sizeof(uintptr_t) * 2)
static inline size_t natural_alignment(size_t size) {
    if(unlikely(size % ARAL_NATURAL_ALIGNMENT))
        size = size + ARAL_NATURAL_ALIGNMENT - (size % ARAL_NATURAL_ALIGNMENT);

    return size;
}

static void arrayalloc_init(ARAL *ar) {
    static netdata_mutex_t mutex = NETDATA_MUTEX_INITIALIZER;
    netdata_mutex_lock(&mutex);

    if(!ar->internal.initialized) {
        netdata_mutex_init(&ar->internal.mutex);

        long int page_size = sysconf(_SC_PAGE_SIZE);
        if (unlikely(page_size == -1))
            ar->internal.natural_page_size = 4096;
        else
            ar->internal.natural_page_size = page_size;

        if (ar->element_size < sizeof(ARAL_FREE))
            ar->element_size = sizeof(ARAL_FREE);

        ar->element_size = natural_alignment(ar->element_size);

        if (ar->elements < 1000)
            ar->elements = 1000;

        ar->internal.pages = NULL;
        ar->internal.free_list = NULL;
        ar->internal.allocation_multiplier = 1;
        ar->internal.file_number = 0;
        ar->internal.total_allocated = 0;
        ar->internal.initialized = true;

        char filename[FILENAME_MAX + 1];
        snprintfz(filename, FILENAME_MAX, "%s/array_alloc.mmap", *ar->cache_dir);
        int r = mkdir(filename, 0775);
        if (r != 0 && errno != EEXIST)
            fatal("Cannot create directory '%s'", filename);

    }

    netdata_mutex_unlock(&mutex);
}

static void arrayalloc_free_checks(ARAL *ar, ARAL_FREE *fr) {
    internal_error(fr->size < ar->element_size, "ARRAYALLOC: free item of size %zu, less than the expected element size %zu", fr->size, ar->element_size);
    internal_error(fr->size % ar->element_size, "ARRAYALLOC: free item of size %zu is not multiple to element size %zu", fr->size, ar->element_size);
    ;
}

static void arrayalloc_increase(ARAL *ar) {
    if(unlikely(!ar->internal.initialized))
        arrayalloc_init(ar);

    ar->internal.file_number++;
    char filename[FILENAME_MAX + 1];

    ARAL_PAGE *page = mallocz(sizeof(ARAL_PAGE));
    page->size = ar->elements * ar->element_size * ar->internal.allocation_multiplier;
    ar->internal.allocation_multiplier *= 2;

    snprintfz(filename, FILENAME_MAX, "%s/array_alloc.mmap/%s.%zu", *ar->cache_dir, ar->filename, ar->internal.file_number);
    page->data = netdata_mmap(filename, page->size, MAP_SHARED, 0);
    if(unlikely(!page->data)) fatal("Cannot allocate arrayalloc buffer of size %zu on filename '%s'", page->size, filename);

    ARAL_FREE *fr = (ARAL_FREE *)page->data;
    fr->size = page->size;
    fr->next = ar->internal.free_list;
    ar->internal.free_list = fr;

    // link the new page
    page->next = ar->internal.pages;
    ar->internal.pages = page;

    arrayalloc_free_checks(ar, fr);
}


static void arrayalloc_lock(ARAL *ar) {
    if(!ar->internal.lockless)
        netdata_mutex_lock(&ar->internal.mutex);
}

static void arrayalloc_unlock(ARAL *ar) {
    if(!ar->internal.lockless)
        netdata_mutex_unlock(&ar->internal.mutex);
}

void *arrayalloc_mallocz(ARAL *ar) {
    arrayalloc_lock(ar);

    if(unlikely(!ar->internal.free_list))
        arrayalloc_increase(ar);

    ARAL_FREE *fr = ar->internal.free_list;

    internal_error(fr->size < ar->element_size, "ARRAYALLOC: free item size is too small %zu", fr->size);

    if(fr->size - ar->element_size <= ar->element_size) {
        // we are done with this space
        ar->internal.free_list = fr->next;
    }
    else {
        uint8_t *data = (uint8_t *)fr;
        ARAL_FREE *fr2 = (ARAL_FREE *)&data[ar->element_size];
        fr2->size = fr->size - ar->element_size;
        fr2->next = fr->next;

        ar->internal.free_list = fr2;
        arrayalloc_free_checks(ar, fr2);
    }

    arrayalloc_unlock(ar);
    return (void *)fr;
}

void arrayalloc_freez(ARAL *ar, void *ptr) {
    if(!ptr) return;
    arrayalloc_lock(ar);
    ARAL_FREE *fr = (ARAL_FREE *)ptr;
    fr->size = ar->element_size;
    fr->next = ar->internal.free_list;
    ar->internal.free_list = fr;
    arrayalloc_unlock(ar);
}
