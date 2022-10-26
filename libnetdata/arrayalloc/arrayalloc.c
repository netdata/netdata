#include "../libnetdata.h"
#include "arrayalloc.h"
#include "daemon/common.h"

// max file size
#define ARAL_MAX_PAGE_SIZE_MMAP (1*1024*1024*1024)

// max malloc size
// optimal at current versions of libc is up to 256k
// ideal to have the same overhead as libc is 4k
#define ARAL_MAX_PAGE_SIZE_MALLOC (64*1024)

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

#define ARAL_NATURAL_ALIGNMENT  (sizeof(uintptr_t) * 2)
static inline size_t natural_alignment(size_t size, size_t alignment) {
    if(unlikely(size % alignment))
        size = size + alignment - (size % alignment);

    return size;
}

static void arrayalloc_delete_leftover_files(const char *path, const char *required_prefix) {
    DIR *dir = opendir(path);
    if(!dir) return;

    char fullpath[FILENAME_MAX + 1];
    size_t len = strlen(required_prefix);

    struct dirent *de = NULL;
    while((de = readdir(dir))) {
        if(de->d_type == DT_DIR)
            continue;

        if(strncmp(de->d_name, required_prefix, len) != 0)
            continue;

        snprintfz(fullpath, FILENAME_MAX, "%s/%s", path, de->d_name);
        info("ARRAYALLOC: removing left-over file '%s'", fullpath);
        if(unlikely(unlink(fullpath) == -1))
            error("Cannot delete file '%s'", fullpath);
    }

    closedir(dir);
}

// ----------------------------------------------------------------------------
// arrayalloc_init()

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

        // we need to add a page pointer after the element
        // so, first align the element size to the pointer size
        ar->internal.element_size = natural_alignment(ar->requested_element_size, sizeof(uintptr_t));

        // then add the size of a pointer to it
        ar->internal.element_size += sizeof(uintptr_t);

        // make sure it is at least what we need for an ARAL_FREE slot
        if (ar->internal.element_size < sizeof(ARAL_FREE))
            ar->internal.element_size = sizeof(ARAL_FREE);

        // and finally align it to the natural alignment
        ar->internal.element_size = natural_alignment(ar->internal.element_size, ARAL_NATURAL_ALIGNMENT);

        // we write the page pointer just after each element
        ar->internal.page_ptr_offset = ar->internal.element_size - sizeof(uintptr_t);

        if(ar->requested_element_size + sizeof(uintptr_t) > ar->internal.element_size)
            fatal("ARRAYALLOC: failed to calculate properly page_ptr_offset: element size %zu, sizeof(uintptr_t) %zu, natural alignment %zu, final element size %zu, page_ptr_offset %zu",
                  ar->requested_element_size, sizeof(uintptr_t), ARAL_NATURAL_ALIGNMENT, ar->internal.element_size, ar->internal.page_ptr_offset);

        //info("ARRAYALLOC: element size %zu, sizeof(uintptr_t) %zu, natural alignment %zu, final element size %zu, page_ptr_offset %zu",
        //      ar->element_size, sizeof(uintptr_t), ARAL_NATURAL_ALIGNMENT, ar->internal.element_size, ar->internal.page_ptr_offset);

        if (ar->initial_elements < 10)
            ar->initial_elements = 10;

        ar->internal.mmap = (ar->use_mmap && ar->cache_dir && *ar->cache_dir) ? true : false;
        ar->internal.max_alloc_size = ar->internal.mmap ? ARAL_MAX_PAGE_SIZE_MMAP : ARAL_MAX_PAGE_SIZE_MALLOC;

        if(ar->internal.max_alloc_size % ar->internal.natural_page_size)
            ar->internal.max_alloc_size += ar->internal.natural_page_size - (ar->internal.max_alloc_size % ar->internal.natural_page_size) ;

        if(ar->internal.max_alloc_size % ar->internal.element_size)
            ar->internal.max_alloc_size -= ar->internal.max_alloc_size % ar->internal.element_size;

        ar->internal.pages = NULL;
        ar->internal.allocation_multiplier = 1;
        ar->internal.file_number = 0;

        if(ar->internal.mmap) {
            char directory_name[FILENAME_MAX + 1];
            snprintfz(directory_name, FILENAME_MAX, "%s/array_alloc.mmap", *ar->cache_dir);
            int r = mkdir(directory_name, 0775);
            if (r != 0 && errno != EEXIST)
                fatal("Cannot create directory '%s'", directory_name);

            char filename[FILENAME_MAX + 1];
            snprintfz(filename, FILENAME_MAX, "%s.", ar->filename);
            arrayalloc_delete_leftover_files(directory_name, filename);
        }

        ar->internal.initialized = true;
    }

    netdata_mutex_unlock(&mutex);
}

// ----------------------------------------------------------------------------
// check a free slot

#ifdef NETDATA_INTERNAL_CHECKS
static inline void arrayalloc_free_validate_internal_check(ARAL *ar, ARAL_FREE *fr) {
    if(fr->size < ar->internal.element_size)
        fatal("ARRAYALLOC: free item of size %zu, less than the expected element size %zu", fr->size, ar->internal.element_size);

    if(fr->size % ar->internal.element_size)
        fatal("ARRAYALLOC: free item of size %zu is not multiple to element size %zu", fr->size, ar->internal.element_size);
}
#else
#define arrayalloc_free_validate_internal_check(ar, fr) debug_dummy()
#endif

// ----------------------------------------------------------------------------
// find the page a pointer belongs to

#ifdef NETDATA_INTERNAL_CHECKS
static inline ARAL_PAGE *find_page_with_allocation_internal_check(ARAL *ar, void *ptr) {
    uintptr_t seeking = (uintptr_t)ptr;
    ARAL_PAGE *page;

    for(page = ar->internal.pages; page ; page = page->next) {
        if(unlikely(seeking >= (uintptr_t)page->data && seeking < (uintptr_t)page->data + page->size))
            break;
    }

    return page;
}
#endif

// ----------------------------------------------------------------------------
// find a page with a free slot (there shouldn't be any)

#ifdef NETDATA_INTERNAL_CHECKS
static inline ARAL_PAGE *find_page_with_free_slots_internal_check(ARAL *ar) {
    ARAL_PAGE *page;

    for(page = ar->internal.pages; page ; page = page->next) {
        if(page->free_list)
            break;

        internal_fatal(page->size - page->used_elements * ar->internal.element_size >= ar->internal.element_size,
                       "ARRAYALLOC: a page is marked full, but it is not!");

        internal_fatal(page->size < page->used_elements * ar->internal.element_size,
                       "ARRAYALLOC: a page has been overflown!");
    }

    return page;
}
#endif

#ifdef NETDATA_TRACE_ALLOCATIONS
static void arrayalloc_add_page(ARAL *ar, const char *file, const char *function, size_t line) {
#else
static void arrayalloc_add_page(ARAL *ar) {
#endif
    if(unlikely(!ar->internal.initialized))
        arrayalloc_init(ar);

    ARAL_PAGE *page = callocz(1, sizeof(ARAL_PAGE));
    page->size = ar->initial_elements * ar->internal.element_size * ar->internal.allocation_multiplier;
    if(page->size > ar->internal.max_alloc_size)
        page->size = ar->internal.max_alloc_size;
    else
        ar->internal.allocation_multiplier *= 2;

    if(ar->internal.mmap) {
        ar->internal.file_number++;
        char filename[FILENAME_MAX + 1];
        snprintfz(filename, FILENAME_MAX, "%s/array_alloc.mmap/%s.%zu", *ar->cache_dir, ar->filename, ar->internal.file_number);
        page->filename = strdupz(filename);
        page->data = netdata_mmap(page->filename, page->size, MAP_SHARED, 0, false);
        if (unlikely(!page->data))
            fatal("Cannot allocate arrayalloc buffer of size %zu on filename '%s'", page->size, page->filename);
    }
    else {
#ifdef NETDATA_TRACE_ALLOCATIONS
        page->data = mallocz_int(page->size, file, function, line);
#else
        page->data = mallocz(page->size);
#endif
    }

    // link the free space to its page
    ARAL_FREE *fr = (ARAL_FREE *)page->data;
    fr->size = page->size;
    fr->page = page;
    fr->next = NULL;
    page->free_list = fr;

    // link the new page at the front of the list of pages
    DOUBLE_LINKED_LIST_PREPEND_UNSAFE(ar->internal.pages, page, prev, next);

    arrayalloc_free_validate_internal_check(ar, fr);
}

static void arrayalloc_lock(ARAL *ar) {
    if(!ar->internal.lockless)
        netdata_mutex_lock(&ar->internal.mutex);
}

static void arrayalloc_unlock(ARAL *ar) {
    if(!ar->internal.lockless)
        netdata_mutex_unlock(&ar->internal.mutex);
}

ARAL *arrayalloc_create(size_t element_size, size_t elements, const char *filename, char **cache_dir, bool mmap) {
    ARAL *ar = callocz(1, sizeof(ARAL));
    ar->requested_element_size = element_size;
    ar->initial_elements = elements;
    ar->filename = filename;
    ar->cache_dir = cache_dir;
    ar->use_mmap = mmap;
    return ar;
}

#ifdef NETDATA_TRACE_ALLOCATIONS
void *arrayalloc_mallocz_int(ARAL *ar, const char *file, const char *function, size_t line) {
#else
void *arrayalloc_mallocz(ARAL *ar) {
#endif
    if(unlikely(!ar->internal.initialized))
        arrayalloc_init(ar);

    arrayalloc_lock(ar);

    if(unlikely(!ar->internal.pages || !ar->internal.pages->free_list)) {
            internal_fatal(find_page_with_free_slots_internal_check(ar) != NULL,
                           "ARRAYALLOC: first page does not have any free slots, but there is another that has!");

#ifdef NETDATA_TRACE_ALLOCATIONS
        arrayalloc_add_page(ar, file, function, line);
#else
        arrayalloc_add_page(ar);
#endif
    }

    ARAL_PAGE *page = ar->internal.pages;
    ARAL_FREE *found_fr = page->free_list;

    internal_fatal(!found_fr,
                   "ARRAYALLOC: free item to use, cannot be NULL.");

    internal_fatal(found_fr->size < ar->internal.element_size,
                   "ARRAYALLOC: free item size %zu, cannot be smaller than %zu",
                   found_fr->size, ar->internal.element_size);

    if(unlikely(found_fr->size - ar->internal.element_size < ar->internal.element_size)) {
        // we can use the entire free space entry

        page->free_list = found_fr->next;

        if(unlikely(!page->free_list)) {
            // we are done with this page
            // move the full page last
            // so that pages with free items remain first in the list
            DOUBLE_LINKED_LIST_REMOVE_UNSAFE(ar->internal.pages, page, prev, next);
            DOUBLE_LINKED_LIST_APPEND_UNSAFE(ar->internal.pages, page, prev, next);
        }
    }
    else {
        // we can split the free space entry

        uint8_t *data = (uint8_t *)found_fr;
        ARAL_FREE *fr = (ARAL_FREE *)&data[ar->internal.element_size];
        fr->page = page;
        fr->size = found_fr->size - ar->internal.element_size;

        // link the free slot first in the page
        fr->next = found_fr->next;
        page->free_list = fr;

        arrayalloc_free_validate_internal_check(ar, fr);
    }

    page->used_elements++;

    // put the page pointer after the element
    uint8_t *data = (uint8_t *)found_fr;
    ARAL_PAGE **page_ptr = (ARAL_PAGE **)&data[ar->internal.page_ptr_offset];
    *page_ptr = page;

    arrayalloc_unlock(ar);
    return (void *)found_fr;
}

#ifdef NETDATA_TRACE_ALLOCATIONS
void arrayalloc_freez_int(ARAL *ar, void *ptr, const char *file, const char *function, size_t line) {
#else
void arrayalloc_freez(ARAL *ar, void *ptr) {
#endif
    if(unlikely(!ptr)) return;
    arrayalloc_lock(ar);

    // get the page pointer
    ARAL_PAGE *page;
    {
        uint8_t *data = (uint8_t *)ptr;
        ARAL_PAGE **page_ptr = (ARAL_PAGE **)&data[ar->internal.page_ptr_offset];
        page = *page_ptr;

#ifdef NETDATA_INTERNAL_CHECKS
        // make it NULL so that we will fail on double free
        // do not enable this on production, because the MMAP file
        // will need to be saved again!
        *page_ptr = NULL;
#endif
    }

#ifdef NETDATA_ARRAYALLOC_INTERNAL_CHECKS
    {
        // find the page ptr belongs
        ARAL_PAGE *page2 = find_page_with_allocation_internal_check(ar, ptr);

        if(unlikely(page != page2))
            fatal("ARRAYALLOC: page pointers do not match!");

        if (unlikely(!page2))
            fatal("ARRAYALLOC: free of pointer %p is not in arrayalloc address space.", ptr);
    }
#endif

    if(unlikely(!page))
        fatal("ARRAYALLOC: possible corruption or double free of pointer %p", ptr);

    if (unlikely(!page->used_elements))
        fatal("ARRAYALLOC: free of pointer %p is inside a page without any active allocations.", ptr);

    page->used_elements--;

    // make this element available
    ARAL_FREE *fr = (ARAL_FREE *)ptr;
    fr->page = page;
    fr->size = ar->internal.element_size;
    fr->next = page->free_list;
    page->free_list = fr;

    // if the page is empty, release it
    if(!page->used_elements) {
        DOUBLE_LINKED_LIST_REMOVE_UNSAFE(ar->internal.pages, page, prev, next);

        // free it
        if(ar->internal.mmap) {
            netdata_munmap(page->data, page->size);
            if (unlikely(unlink(page->filename) == 1))
                error("Cannot delete file '%s'", page->filename);
            freez((void *)page->filename);
        }
        else {
#ifdef NETDATA_TRACE_ALLOCATIONS
            freez_int(page->data, file, function, line);
#else
            freez(page->data);
#endif
        }

        freez(page);
    }
    else if(page != ar->internal.pages) {
        // move the page with free item first
        // so that the next allocation will use this page
        DOUBLE_LINKED_LIST_REMOVE_UNSAFE(ar->internal.pages, page, prev, next);
        DOUBLE_LINKED_LIST_PREPEND_UNSAFE(ar->internal.pages, page, prev, next);
    }

    arrayalloc_unlock(ar);
}

int aral_unittest(size_t elements) {
    char *cache_dir = "/tmp/";
    ARAL *ar = arrayalloc_create(20, 10, "test-aral", &cache_dir, false);

    void *pointers[elements];

    for(size_t i = 0; i < elements ;i++) {
        pointers[i] = arrayalloc_mallocz(ar);
    }

    for(size_t div = 5; div >= 2 ;div--) {
        for (size_t i = 0; i < elements / div; i++) {
            arrayalloc_freez(ar, pointers[i]);
        }

        for (size_t i = 0; i < elements / div; i++) {
            pointers[i] = arrayalloc_mallocz(ar);
        }
    }

    for(size_t step = 50; step >= 10 ;step -= 10) {
        for (size_t i = 0; i < elements; i += step) {
            arrayalloc_freez(ar, pointers[i]);
        }

        for (size_t i = 0; i < elements; i += step) {
            pointers[i] = arrayalloc_mallocz(ar);
        }
    }

    for(size_t i = 0; i < elements ;i++) {
        arrayalloc_freez(ar, pointers[i]);
    }

    if(ar->internal.pages) {
        fprintf(stderr, "ARAL leftovers detected (1)");
        return 1;
    }

    size_t ops = 0;
    size_t increment = elements / 10;
    size_t allocated = 0;
    for(size_t all = increment; all <= elements ; all += increment) {

        for(; allocated < all ; allocated++) {
            pointers[allocated] = arrayalloc_mallocz(ar);
            ops++;
        }

        size_t to_free = now_realtime_usec() % all;
        size_t free_list[to_free];
        for(size_t i = 0; i < to_free ;i++) {
            size_t pos;
            do {
                pos = now_realtime_usec() % all;
            } while(!pointers[pos]);

            arrayalloc_freez(ar, pointers[pos]);
            pointers[pos] = NULL;
            free_list[i] = pos;
            ops++;
        }

        for(size_t i = 0; i < to_free ;i++) {
            size_t pos = free_list[i];
            pointers[pos] = arrayalloc_mallocz(ar);
            ops++;
        }
    }

    for(size_t i = 0; i < allocated - 1 ;i++) {
        arrayalloc_freez(ar, pointers[i]);
        ops++;
    }

    arrayalloc_freez(ar, pointers[allocated - 1]);

    if(ar->internal.pages) {
        fprintf(stderr, "ARAL leftovers detected (2)");
        return 1;
    }

    return 0;
}
