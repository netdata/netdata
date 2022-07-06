#include "../libnetdata.h"
#include "arrayalloc.h"
#include "daemon/common.h"

// 1GB max file size
#define ARAL_MAX_PAGE_SIZE_MMAP (1*1024*1024*1024)

// 5MB max malloc size
#define ARAL_MAX_PAGE_SIZE_MALLOC (5*1024*1024)

typedef struct arrayalloc_free {
    size_t size;
    struct arrayalloc_page *page;
    struct arrayalloc_free *next;
} ARAL_FREE;

typedef struct arrayalloc_page {
    const char *filename;
    size_t size;                  // the total size of the page
    size_t used_elements;         // the total number of used elements on this page
    uint8_t *data;
    ARAL_FREE *free_list;
    struct arrayalloc_page *prev; // the prev page on the list
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

        ar->internal.element_size = ar->element_size;
        if (ar->internal.element_size < sizeof(ARAL_FREE))
            ar->internal.element_size = sizeof(ARAL_FREE);

        ar->internal.element_size = natural_alignment(ar->element_size);

        if (ar->elements < 1000)
            ar->elements = 1000;

        ar->internal.mmap = (ar->use_mmap && ar->cache_dir && *ar->cache_dir) ? true : false;
        ar->internal.max_alloc_size = natural_alignment(ar->internal.mmap ? ARAL_MAX_PAGE_SIZE_MMAP : ARAL_MAX_PAGE_SIZE_MALLOC);

        if(ar->internal.max_alloc_size % ar->internal.element_size)
            ar->internal.max_alloc_size -= ar->internal.max_alloc_size % ar->internal.element_size;

        ar->internal.first_page = NULL;
        ar->internal.last_page = NULL;
        ar->internal.allocation_multiplier = 1;
        ar->internal.file_number = 0;

        if(ar->internal.mmap) {
            char filename[FILENAME_MAX + 1];
            snprintfz(filename, FILENAME_MAX, "%s/array_alloc.mmap", *ar->cache_dir);
            int r = mkdir(filename, 0775);
            if (r != 0 && errno != EEXIST)
                fatal("Cannot create directory '%s'", filename);
        }

        ar->internal.initialized = true;
    }

    netdata_mutex_unlock(&mutex);
}

static void arrayalloc_free_checks(ARAL *ar, ARAL_FREE *fr) {
    if(fr->size < ar->internal.element_size)
        fatal("ARRAYALLOC: free item of size %zu, less than the expected element size %zu", fr->size, ar->internal.element_size);

    if(fr->size % ar->internal.element_size)
        fatal("ARRAYALLOC: free item of size %zu is not multiple to element size %zu", fr->size, ar->internal.element_size);
}

void unlink_page(ARAL *ar, ARAL_PAGE *page) {
    if(unlikely(!page)) return;

    if(page->next)
        page->next->prev = page->prev;

    if(page->prev)
        page->prev->next = page->next;

    if(page == ar->internal.first_page)
        ar->internal.first_page = page->next;

    if(page == ar->internal.last_page)
        ar->internal.last_page = page->prev;
}

ARAL_PAGE *unlink_first_page(ARAL *ar) {
    ARAL_PAGE *page = ar->internal.first_page;
    unlink_page(ar, page);
    return page;
}

void link_page_first(ARAL *ar, ARAL_PAGE *page) {
    page->prev = NULL;
    page->next = ar->internal.first_page;
    if(page->next) page->next->prev = page;

    ar->internal.first_page = page;

    if(!ar->internal.last_page)
        ar->internal.last_page = page;
}

void link_page_last(ARAL *ar, ARAL_PAGE *page) {
    page->next = NULL;
    page->prev = ar->internal.last_page;
    if(page->prev) page->prev->next = page;

    ar->internal.last_page = page;

    if(!ar->internal.first_page)
        ar->internal.first_page = page;
}

ARAL_PAGE *find_page_with_allocation(ARAL *ar, void *ptr) {
    size_t seeking = (size_t)ptr;
    ARAL_PAGE *page;

    for(page = ar->internal.first_page; page ; page = page->next) {
        if(unlikely(seeking >= (size_t)page->data && seeking < (size_t)page->data + page->size))
            break;
    }

    return page;
}

static void arrayalloc_increase(ARAL *ar) {
    if(unlikely(!ar->internal.initialized))
        arrayalloc_init(ar);

    ARAL_PAGE *page = callocz(1, sizeof(ARAL_PAGE));
    page->size = ar->elements * ar->internal.element_size * ar->internal.allocation_multiplier;
    if(page->size > ar->internal.max_alloc_size)
        page->size = ar->internal.max_alloc_size;
    else
        ar->internal.allocation_multiplier *= 2;

    if(ar->internal.mmap) {
        ar->internal.file_number++;
        char filename[FILENAME_MAX + 1];
        snprintfz(filename, FILENAME_MAX, "%s/array_alloc.mmap/%s.%zu", *ar->cache_dir, ar->filename, ar->internal.file_number);
        page->filename = strdupz(filename);
        page->data = netdata_mmap(page->filename, page->size, MAP_SHARED, 0);
        if (unlikely(!page->data))
            fatal("Cannot allocate arrayalloc buffer of size %zu on filename '%s'", page->size, page->filename);
    }
    else
        page->data = mallocz(page->size);

    // link the free space to its page
    ARAL_FREE *fr = (ARAL_FREE *)page->data;
    fr->size = page->size;
    fr->page = page;
    fr->next = NULL;
    page->free_list = fr;

    // link the new page at the front of the list of pages
    link_page_first(ar, page);

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

    if(unlikely(!ar->internal.first_page || !ar->internal.first_page->free_list))
        arrayalloc_increase(ar);

    ARAL_PAGE *page = ar->internal.first_page;
    ARAL_FREE *fr = page->free_list;

    if(unlikely(!fr))
        fatal("ARRAYALLOC: free item cannot be NULL.");

    if(unlikely(fr->size < ar->internal.element_size))
        fatal("ARRAYALLOC: free item size %zu is smaller than %zu", fr->size, ar->internal.element_size);

    if(fr->size - ar->internal.element_size <= ar->internal.element_size) {
        // we are done with this page
        page->free_list = NULL;

        if(page != ar->internal.last_page) {
            unlink_page(ar, page);
            link_page_last(ar, page);
        }
    }
    else {
        uint8_t *data = (uint8_t *)fr;
        ARAL_FREE *fr2 = (ARAL_FREE *)&data[ar->internal.element_size];
        fr2->page = fr->page;
        fr2->size = fr->size - ar->internal.element_size;
        fr2->next = fr->next;
        page->free_list = fr2;

        arrayalloc_free_checks(ar, fr2);
    }

    fr->page->used_elements++;

    arrayalloc_unlock(ar);
    return (void *)fr;
}

void arrayalloc_freez(ARAL *ar, void *ptr) {
    if(!ptr) return;
    arrayalloc_lock(ar);

    // find the page ptr belongs
    ARAL_PAGE *page = find_page_with_allocation(ar, ptr);

    if(unlikely(!page))
        fatal("ARRAYALLOC: free of pointer %p is not in arrayalloc address space.", ptr);

    if(unlikely(!page->used_elements))
        fatal("ARRAYALLOC: free of pointer %p is inside an already free page.", ptr);

    page->used_elements--;

    // make this element available
    ARAL_FREE *fr = (ARAL_FREE *)ptr;
    fr->page = page;
    fr->size = ar->internal.element_size;
    fr->next = page->free_list;
    page->free_list = fr;

    // if the page is empty, release it
    if(!page->used_elements) {
        unlink_page(ar, page);

        // free it
        if(ar->internal.mmap) {
            munmap(page->data, page->size);
            unlink(page->filename);
            freez((void *)page->filename);
        }
        else
            freez(page->data);

        freez(page);
    }
    else if(page != ar->internal.first_page) {
        unlink_page(ar, page);
        link_page_first(ar, page);
    }

    arrayalloc_unlock(ar);
}
