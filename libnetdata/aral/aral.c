#include "../libnetdata.h"
#include "aral.h"

#ifdef NETDATA_TRACE_ALLOCATIONS
#define TRACE_ALLOCATIONS_FUNCTION_DEFINITION_PARAMS , const char *file, const char *function, size_t line
#define TRACE_ALLOCATIONS_FUNCTION_CALL_PARAMS , file, function, line
#else
#define TRACE_ALLOCATIONS_FUNCTION_DEFINITION_PARAMS
#define TRACE_ALLOCATIONS_FUNCTION_CALL_PARAMS
#endif

#define ARAL_FREE_PAGES_DELTA_TO_REARRANGE_LIST 5

// max file size
#define ARAL_MAX_PAGE_SIZE_MMAP (1*1024*1024*1024)

// max malloc size
// optimal at current versions of libc is up to 256k
// ideal to have the same overhead as libc is 4k
#define ARAL_MAX_PAGE_SIZE_MALLOC (65*1024)

typedef struct aral_free {
    size_t size;
    struct aral_free *next;
} ARAL_FREE;

typedef struct aral_page {
    size_t size;                    // the allocation size of the page
    const char *filename;
    uint8_t *data;

    uint32_t free_elements_to_move_first;
    uint32_t max_elements;          // the number of elements that can fit on this page

    struct {
        uint32_t used_elements;         // the number of used elements on this page
        uint32_t free_elements;         // the number of free elements on this page

        struct aral_page *prev; // the prev page on the list
        struct aral_page *next; // the next page on the list
    } aral_lock;

    struct {
        SPINLOCK spinlock;
        ARAL_FREE *list;
    } free;

} ARAL_PAGE;

typedef enum {
    ARAL_LOCKLESS = (1 << 0),
    ARAL_DEFRAGMENT = (1 << 1),
    ARAL_ALLOCATED_STATS = (1 << 2),
} ARAL_OPTIONS;

struct aral {
    struct {
        char name[ARAL_MAX_NAME + 1];

        ARAL_OPTIONS options;

        size_t element_size;            // calculated to take into account ARAL overheads
        size_t max_allocation_size;     // calculated in bytes
        size_t max_page_elements;       // calculated
        size_t page_ptr_offset;         // calculated
        size_t natural_page_size;       // calculated

        size_t initial_page_elements;
        size_t requested_element_size;
        size_t requested_max_page_size;

        struct {
            bool enabled;
            const char *filename;
            char **cache_dir;
        } mmap;
    } config;

    struct {
        SPINLOCK spinlock;
        size_t file_number;             // for mmap
        struct aral_page *pages;        // linked list of pages

        size_t user_malloc_operations;
        size_t user_free_operations;
        size_t defragment_operations;
        size_t defragment_linked_list_traversals;
    } aral_lock;

    struct {
        SPINLOCK spinlock;
        size_t allocating_elements;     // currently allocating elements
        size_t allocation_size;         // current / next allocation size
    } adders;

    struct {
        size_t allocators;              // the number of threads currently trying to allocate memory
    } atomic;

    struct aral_statistics *stats;
};

size_t aral_structures_from_stats(struct aral_statistics *stats) {
    return __atomic_load_n(&stats->structures.allocated_bytes, __ATOMIC_RELAXED);
}

size_t aral_overhead_from_stats(struct aral_statistics *stats) {
    return __atomic_load_n(&stats->malloc.allocated_bytes, __ATOMIC_RELAXED) -
           __atomic_load_n(&stats->malloc.used_bytes, __ATOMIC_RELAXED);
}

size_t aral_overhead(ARAL *ar) {
    return aral_overhead_from_stats(ar->stats);
}

size_t aral_structures(ARAL *ar) {
    return aral_structures_from_stats(ar->stats);
}

struct aral_statistics *aral_statistics(ARAL *ar) {
    return ar->stats;
}

#define ARAL_NATURAL_ALIGNMENT  (sizeof(uintptr_t) * 2)
static inline size_t natural_alignment(size_t size, size_t alignment) {
    if(unlikely(size % alignment))
        size = size + alignment - (size % alignment);

    return size;
}

static size_t aral_align_alloc_size(ARAL *ar, uint64_t size) {
    if(size % ar->config.natural_page_size)
        size += ar->config.natural_page_size - (size % ar->config.natural_page_size) ;

    if(size % ar->config.element_size)
        size -= size % ar->config.element_size;

    return size;
}

static inline void aral_lock(ARAL *ar) {
    if(likely(!(ar->config.options & ARAL_LOCKLESS)))
        netdata_spinlock_lock(&ar->aral_lock.spinlock);
}

static inline void aral_unlock(ARAL *ar) {
    if(likely(!(ar->config.options & ARAL_LOCKLESS)))
        netdata_spinlock_unlock(&ar->aral_lock.spinlock);
}

static inline void aral_page_free_lock(ARAL *ar, ARAL_PAGE *page) {
    if(likely(!(ar->config.options & ARAL_LOCKLESS)))
        netdata_spinlock_lock(&page->free.spinlock);
}

static inline void aral_page_free_unlock(ARAL *ar, ARAL_PAGE *page) {
    if(likely(!(ar->config.options & ARAL_LOCKLESS)))
        netdata_spinlock_unlock(&page->free.spinlock);
}

static inline bool aral_adders_trylock(ARAL *ar) {
    if(likely(!(ar->config.options & ARAL_LOCKLESS)))
        return netdata_spinlock_trylock(&ar->adders.spinlock);

    return true;
}

static inline void aral_adders_lock(ARAL *ar) {
    if(likely(!(ar->config.options & ARAL_LOCKLESS)))
        netdata_spinlock_lock(&ar->adders.spinlock);
}

static inline void aral_adders_unlock(ARAL *ar) {
    if(likely(!(ar->config.options & ARAL_LOCKLESS)))
        netdata_spinlock_unlock(&ar->adders.spinlock);
}

static void aral_delete_leftover_files(const char *name, const char *path, const char *required_prefix) {
    DIR *dir = opendir(path);
    if(!dir) return;

    char full_path[FILENAME_MAX + 1];
    size_t len = strlen(required_prefix);

    struct dirent *de = NULL;
    while((de = readdir(dir))) {
        if(de->d_type == DT_DIR)
            continue;

        if(strncmp(de->d_name, required_prefix, len) != 0)
            continue;

        snprintfz(full_path, FILENAME_MAX, "%s/%s", path, de->d_name);
        info("ARAL: '%s' removing left-over file '%s'", name, full_path);
        if(unlikely(unlink(full_path) == -1))
            error("ARAL: '%s' cannot delete file '%s'", name, full_path);
    }

    closedir(dir);
}

// ----------------------------------------------------------------------------
// check a free slot

#ifdef NETDATA_INTERNAL_CHECKS
static inline void aral_free_validate_internal_check(ARAL *ar, ARAL_FREE *fr) {
    if(unlikely(fr->size < ar->config.element_size))
        fatal("ARAL: '%s' free item of size %zu, less than the expected element size %zu",
              ar->config.name, fr->size, ar->config.element_size);

    if(unlikely(fr->size % ar->config.element_size))
        fatal("ARAL: '%s' free item of size %zu is not multiple to element size %zu",
              ar->config.name, fr->size, ar->config.element_size);
}
#else
#define aral_free_validate_internal_check(ar, fr) debug_dummy()
#endif

// ----------------------------------------------------------------------------
// find the page a pointer belongs to

#ifdef NETDATA_INTERNAL_CHECKS
static inline ARAL_PAGE *find_page_with_allocation_internal_check(ARAL *ar, void *ptr) {
    aral_lock(ar);

    uintptr_t seeking = (uintptr_t)ptr;
    ARAL_PAGE *page;

    for(page = ar->aral_lock.pages; page ; page = page->aral_lock.next) {
        if(unlikely(seeking >= (uintptr_t)page->data && seeking < (uintptr_t)page->data + page->size))
            break;
    }

    aral_unlock(ar);

    return page;
}
#endif

// ----------------------------------------------------------------------------
// find a page with a free slot (there shouldn't be any)

#ifdef NETDATA_ARAL_INTERNAL_CHECKS
static inline ARAL_PAGE *find_page_with_free_slots_internal_check___with_aral_lock(ARAL *ar) {
    ARAL_PAGE *page;

    for(page = ar->aral_lock.pages; page ; page = page->next) {
        if(page->aral_lock.free_elements)
            break;

        internal_fatal(page->size - page->aral_lock.used_elements * ar->config.element_size >= ar->config.element_size,
                       "ARAL: '%s' a page is marked full, but it is not!", ar->config.name);

        internal_fatal(page->size < page->aral_lock.used_elements * ar->config.element_size,
                       "ARAL: '%s' a page has been overflown!", ar->config.name);
    }

    return page;
}
#endif

size_t aral_next_allocation_size___adders_lock_needed(ARAL *ar) {
    size_t size = ar->adders.allocation_size;

    if(size > ar->config.max_allocation_size)
        size = ar->config.max_allocation_size;
    else
        ar->adders.allocation_size = aral_align_alloc_size(ar, (uint64_t)ar->adders.allocation_size * 2);

    return size;
}

static ARAL_PAGE *aral_create_page___no_lock_needed(ARAL *ar, size_t size TRACE_ALLOCATIONS_FUNCTION_DEFINITION_PARAMS) {
    ARAL_PAGE *page = callocz(1, sizeof(ARAL_PAGE));
    netdata_spinlock_init(&page->free.spinlock);
    page->size = size;
    page->max_elements = page->size / ar->config.element_size;
    page->aral_lock.free_elements = page->max_elements;
    page->free_elements_to_move_first = page->max_elements / 4;
    if(unlikely(page->free_elements_to_move_first < 1))
        page->free_elements_to_move_first = 1;

    __atomic_add_fetch(&ar->stats->structures.allocations, 1, __ATOMIC_RELAXED);
    __atomic_add_fetch(&ar->stats->structures.allocated_bytes, sizeof(ARAL_PAGE), __ATOMIC_RELAXED);

    if(unlikely(ar->config.mmap.enabled)) {
        ar->aral_lock.file_number++;
        char filename[FILENAME_MAX + 1];
        snprintfz(filename, FILENAME_MAX, "%s/array_alloc.mmap/%s.%zu", *ar->config.mmap.cache_dir, ar->config.mmap.filename, ar->aral_lock.file_number);
        page->filename = strdupz(filename);
        page->data = netdata_mmap(page->filename, page->size, MAP_SHARED, 0, false, NULL);
        if (unlikely(!page->data))
            fatal("ARAL: '%s' cannot allocate aral buffer of size %zu on filename '%s'",
                  ar->config.name, page->size, page->filename);
        __atomic_add_fetch(&ar->stats->mmap.allocations, 1, __ATOMIC_RELAXED);
        __atomic_add_fetch(&ar->stats->mmap.allocated_bytes, page->size, __ATOMIC_RELAXED);
    }
    else {
#ifdef NETDATA_TRACE_ALLOCATIONS
        page->data = mallocz_int(page->size TRACE_ALLOCATIONS_FUNCTION_CALL_PARAMS);
#else
        page->data = mallocz(page->size);
#endif
        __atomic_add_fetch(&ar->stats->malloc.allocations, 1, __ATOMIC_RELAXED);
        __atomic_add_fetch(&ar->stats->malloc.allocated_bytes, page->size, __ATOMIC_RELAXED);
    }

    // link the free space to its page
    ARAL_FREE *fr = (ARAL_FREE *)page->data;
    fr->size = page->size;
    fr->next = NULL;
    page->free.list = fr;

    aral_free_validate_internal_check(ar, fr);

    return page;
}

void aral_del_page___no_lock_needed(ARAL *ar, ARAL_PAGE *page TRACE_ALLOCATIONS_FUNCTION_DEFINITION_PARAMS) {

    // free it
    if (ar->config.mmap.enabled) {
        netdata_munmap(page->data, page->size);

        if (unlikely(unlink(page->filename) == 1))
            error("Cannot delete file '%s'", page->filename);

        freez((void *)page->filename);

        __atomic_sub_fetch(&ar->stats->mmap.allocations, 1, __ATOMIC_RELAXED);
        __atomic_sub_fetch(&ar->stats->mmap.allocated_bytes, page->size, __ATOMIC_RELAXED);
    }
    else {
#ifdef NETDATA_TRACE_ALLOCATIONS
        freez_int(page->data TRACE_ALLOCATIONS_FUNCTION_CALL_PARAMS);
#else
        freez(page->data);
#endif
        __atomic_sub_fetch(&ar->stats->malloc.allocations, 1, __ATOMIC_RELAXED);
        __atomic_sub_fetch(&ar->stats->malloc.allocated_bytes, page->size, __ATOMIC_RELAXED);
    }

    freez(page);

    __atomic_sub_fetch(&ar->stats->structures.allocations, 1, __ATOMIC_RELAXED);
    __atomic_sub_fetch(&ar->stats->structures.allocated_bytes, sizeof(ARAL_PAGE), __ATOMIC_RELAXED);
}

static inline void aral_insert_not_linked_page_with_free_items_to_proper_position___aral_lock_needed(ARAL *ar, ARAL_PAGE *page) {
    ARAL_PAGE *first = ar->aral_lock.pages;

    if (page->aral_lock.free_elements <= page->free_elements_to_move_first ||
        !first ||
        !first->aral_lock.free_elements ||
        page->aral_lock.free_elements <= first->aral_lock.free_elements + ARAL_FREE_PAGES_DELTA_TO_REARRANGE_LIST) {
        // first position
        DOUBLE_LINKED_LIST_PREPEND_ITEM_UNSAFE(ar->aral_lock.pages, page, aral_lock.prev, aral_lock.next);
    }
    else {
        ARAL_PAGE *second = first->aral_lock.next;

        if (!second ||
            !second->aral_lock.free_elements ||
            page->aral_lock.free_elements <= second->aral_lock.free_elements)
            // second position
            DOUBLE_LINKED_LIST_INSERT_ITEM_AFTER_UNSAFE(ar->aral_lock.pages, first, page, aral_lock.prev, aral_lock.next);
        else
            // third position
            DOUBLE_LINKED_LIST_INSERT_ITEM_AFTER_UNSAFE(ar->aral_lock.pages, second, page, aral_lock.prev, aral_lock.next);
    }
}

static inline ARAL_PAGE *aral_acquire_a_free_slot(ARAL *ar TRACE_ALLOCATIONS_FUNCTION_DEFINITION_PARAMS) {
    __atomic_add_fetch(&ar->atomic.allocators, 1, __ATOMIC_RELAXED);
    aral_lock(ar);

    ARAL_PAGE *page = ar->aral_lock.pages;

    while(!page || !page->aral_lock.free_elements) {
#ifdef NETDATA_ARAL_INTERNAL_CHECKS
        internal_fatal(find_page_with_free_slots_internal_check___with_aral_lock(ar), "ARAL: '%s' found page with free slot!", ar->config.name);
#endif
        aral_unlock(ar);

        if(aral_adders_trylock(ar)) {
            if(ar->adders.allocating_elements < __atomic_load_n(&ar->atomic.allocators, __ATOMIC_RELAXED)) {

                size_t size = aral_next_allocation_size___adders_lock_needed(ar);
                ar->adders.allocating_elements += size / ar->config.element_size;
                aral_adders_unlock(ar);

                page = aral_create_page___no_lock_needed(ar, size TRACE_ALLOCATIONS_FUNCTION_CALL_PARAMS);

                aral_lock(ar);
                aral_insert_not_linked_page_with_free_items_to_proper_position___aral_lock_needed(ar, page);

                aral_adders_lock(ar);
                ar->adders.allocating_elements -= size / ar->config.element_size;
                aral_adders_unlock(ar);

                // we have a page that is all empty
                // and only aral_lock() is held, so
                // break the loop
                break;
            }

            aral_adders_unlock(ar);
        }

        aral_lock(ar);
        page = ar->aral_lock.pages;
    }

    __atomic_sub_fetch(&ar->atomic.allocators, 1, __ATOMIC_RELAXED);

    // we have a page
    // and aral locked

    {
        ARAL_PAGE *first = ar->aral_lock.pages;
        ARAL_PAGE *second = first->aral_lock.next;

        if (!second ||
            !second->aral_lock.free_elements ||
            first->aral_lock.free_elements <= second->aral_lock.free_elements + ARAL_FREE_PAGES_DELTA_TO_REARRANGE_LIST)
            page = first;
        else {
            DOUBLE_LINKED_LIST_REMOVE_ITEM_UNSAFE(ar->aral_lock.pages, second, aral_lock.prev, aral_lock.next);
            DOUBLE_LINKED_LIST_PREPEND_ITEM_UNSAFE(ar->aral_lock.pages, second, aral_lock.prev, aral_lock.next);
            page = second;
        }
    }

    internal_fatal(!page || !page->aral_lock.free_elements,
                   "ARAL: '%s' selected page does not have a free slot in it",
                   ar->config.name);

    internal_fatal(page->max_elements != page->aral_lock.used_elements + page->aral_lock.free_elements,
                   "ARAL: '%s' page element counters do not match, "
                   "page says it can handle %zu elements, "
                   "but there are %zu used and %zu free items, "
                   "total %zu items",
                   ar->config.name,
                   (size_t)page->max_elements,
                   (size_t)page->aral_lock.used_elements, (size_t)page->aral_lock.free_elements,
                   (size_t)page->aral_lock.used_elements + (size_t)page->aral_lock.free_elements
    );

    ar->aral_lock.user_malloc_operations++;

    // acquire a slot for the caller
    page->aral_lock.used_elements++;
    if(--page->aral_lock.free_elements == 0) {
        // we are done with this page
        // move the full page last
        // so that pages with free items remain first in the list
        DOUBLE_LINKED_LIST_REMOVE_ITEM_UNSAFE(ar->aral_lock.pages, page, aral_lock.prev, aral_lock.next);
        DOUBLE_LINKED_LIST_APPEND_ITEM_UNSAFE(ar->aral_lock.pages, page, aral_lock.prev, aral_lock.next);
    }

    aral_unlock(ar);

    return page;
}

void *aral_mallocz_internal(ARAL *ar TRACE_ALLOCATIONS_FUNCTION_DEFINITION_PARAMS) {
#ifdef FSANITIZE_ADDRESS
    return mallocz(ar->config.requested_element_size);
#endif

    ARAL_PAGE *page = aral_acquire_a_free_slot(ar TRACE_ALLOCATIONS_FUNCTION_CALL_PARAMS);

    aral_page_free_lock(ar, page);

    internal_fatal(!page->free.list,
                   "ARAL: '%s' free item to use, cannot be NULL.", ar->config.name);

    internal_fatal(page->free.list->size < ar->config.element_size,
                   "ARAL: '%s' free item size %zu, cannot be smaller than %zu",
                   ar->config.name, page->free.list->size, ar->config.element_size);

    ARAL_FREE *found_fr = page->free.list;

    // check if the remaining size (after we use this slot) is not enough for another element
    if(unlikely(found_fr->size - ar->config.element_size < ar->config.element_size)) {
        // we can use the entire free space entry

        page->free.list = found_fr->next;
    }
    else {
        // we can split the free space entry

        uint8_t *data = (uint8_t *)found_fr;
        ARAL_FREE *fr = (ARAL_FREE *)&data[ar->config.element_size];
        fr->size = found_fr->size - ar->config.element_size;

        // link the free slot first in the page
        fr->next = found_fr->next;
        page->free.list = fr;

        aral_free_validate_internal_check(ar, fr);
    }

    aral_page_free_unlock(ar, page);

    // put the page pointer after the element
    uint8_t *data = (uint8_t *)found_fr;
    ARAL_PAGE **page_ptr = (ARAL_PAGE **)&data[ar->config.page_ptr_offset];
    *page_ptr = page;

    if(unlikely(ar->config.mmap.enabled))
        __atomic_add_fetch(&ar->stats->mmap.used_bytes, ar->config.element_size, __ATOMIC_RELAXED);
    else
        __atomic_add_fetch(&ar->stats->malloc.used_bytes, ar->config.element_size, __ATOMIC_RELAXED);

    return (void *)found_fr;
}

static inline ARAL_PAGE *aral_ptr_to_page___must_NOT_have_aral_lock(ARAL *ar, void *ptr) {
    // given a data pointer we returned before,
    // find the ARAL_PAGE it belongs to

    uint8_t *data = (uint8_t *)ptr;
    ARAL_PAGE **page_ptr = (ARAL_PAGE **)&data[ar->config.page_ptr_offset];
    ARAL_PAGE *page = *page_ptr;

#ifdef NETDATA_INTERNAL_CHECKS
    // make it NULL so that we will fail on double free
    // do not enable this on production, because the MMAP file
    // will need to be saved again!
    *page_ptr = NULL;
#endif

#ifdef NETDATA_ARAL_INTERNAL_CHECKS
    {
        // find the page ptr belongs
        ARAL_PAGE *page2 = find_page_with_allocation_internal_check(ar, ptr);

        internal_fatal(page != page2,
                       "ARAL: '%s' page pointers do not match!",
                       ar->name);

        internal_fatal(!page2,
                       "ARAL: '%s' free of pointer %p is not in ARAL address space.",
                       ar->name, ptr);
    }
#endif

    internal_fatal(!page,
                   "ARAL: '%s' possible corruption or double free of pointer %p",
                   ar->config.name, ptr);

    return page;
}

static void aral_defrag_sorted_page_position___aral_lock_needed(ARAL *ar, ARAL_PAGE *page) {
    ARAL_PAGE *tmp;

    int action = 0; (void)action;
    size_t move_later = 0, move_earlier = 0;

    for(tmp = page->aral_lock.next ;
        tmp && tmp->aral_lock.free_elements && tmp->aral_lock.free_elements < page->aral_lock.free_elements ;
        tmp = tmp->aral_lock.next)
        move_later++;

    if(!tmp && page->aral_lock.next) {
        DOUBLE_LINKED_LIST_REMOVE_ITEM_UNSAFE(ar->aral_lock.pages, page, aral_lock.prev, aral_lock.next);
        DOUBLE_LINKED_LIST_APPEND_ITEM_UNSAFE(ar->aral_lock.pages, page, aral_lock.prev, aral_lock.next);
        action = 1;
    }
    else if(tmp != page->aral_lock.next) {
        DOUBLE_LINKED_LIST_REMOVE_ITEM_UNSAFE(ar->aral_lock.pages, page, aral_lock.prev, aral_lock.next);
        DOUBLE_LINKED_LIST_INSERT_ITEM_BEFORE_UNSAFE(ar->aral_lock.pages, tmp, page, aral_lock.prev, aral_lock.next);
        action = 2;
    }
    else {
        for(tmp = (page == ar->aral_lock.pages) ? NULL : page->aral_lock.prev ;
            tmp && (!tmp->aral_lock.free_elements || tmp->aral_lock.free_elements > page->aral_lock.free_elements);
            tmp = (tmp == ar->aral_lock.pages) ? NULL : tmp->aral_lock.prev)
            move_earlier++;

        if(!tmp) {
            DOUBLE_LINKED_LIST_REMOVE_ITEM_UNSAFE(ar->aral_lock.pages, page, aral_lock.prev, aral_lock.next);
            DOUBLE_LINKED_LIST_PREPEND_ITEM_UNSAFE(ar->aral_lock.pages, page, aral_lock.prev, aral_lock.next);
            action = 3;
        }
        else if(tmp != page->aral_lock.prev){
            DOUBLE_LINKED_LIST_REMOVE_ITEM_UNSAFE(ar->aral_lock.pages, page, aral_lock.prev, aral_lock.next);
            DOUBLE_LINKED_LIST_INSERT_ITEM_AFTER_UNSAFE(ar->aral_lock.pages, tmp, page, aral_lock.prev, aral_lock.next);
            action = 4;
        }
    }

    ar->aral_lock.defragment_operations++;
    ar->aral_lock.defragment_linked_list_traversals += move_earlier + move_later;

    internal_fatal(page->aral_lock.next && page->aral_lock.next->aral_lock.free_elements && page->aral_lock.next->aral_lock.free_elements < page->aral_lock.free_elements,
                   "ARAL: '%s' item should be later in the list", ar->config.name);

    internal_fatal(page != ar->aral_lock.pages && (!page->aral_lock.prev->aral_lock.free_elements || page->aral_lock.prev->aral_lock.free_elements > page->aral_lock.free_elements),
                   "ARAL: '%s' item should be earlier in the list", ar->config.name);
}

static inline void aral_move_page_with_free_list___aral_lock_needed(ARAL *ar, ARAL_PAGE *page) {
    if(unlikely(page == ar->aral_lock.pages))
        // we are the first already
        return;

    if(likely(!(ar->config.options & ARAL_DEFRAGMENT))) {
        DOUBLE_LINKED_LIST_REMOVE_ITEM_UNSAFE(ar->aral_lock.pages, page, aral_lock.prev, aral_lock.next);
        aral_insert_not_linked_page_with_free_items_to_proper_position___aral_lock_needed(ar, page);
    }
    else
        aral_defrag_sorted_page_position___aral_lock_needed(ar, page);
}

void aral_freez_internal(ARAL *ar, void *ptr TRACE_ALLOCATIONS_FUNCTION_DEFINITION_PARAMS) {
#ifdef FSANITIZE_ADDRESS
    freez(ptr);
    return;
#endif

    if(unlikely(!ptr)) return;

    // get the page pointer
    ARAL_PAGE *page = aral_ptr_to_page___must_NOT_have_aral_lock(ar, ptr);

    if(unlikely(ar->config.mmap.enabled))
        __atomic_sub_fetch(&ar->stats->mmap.used_bytes, ar->config.element_size, __ATOMIC_RELAXED);
    else
        __atomic_sub_fetch(&ar->stats->malloc.used_bytes, ar->config.element_size, __ATOMIC_RELAXED);

    // make this element available
    ARAL_FREE *fr = (ARAL_FREE *)ptr;
    fr->size = ar->config.element_size;

    aral_page_free_lock(ar, page);
    fr->next = page->free.list;
    page->free.list = fr;
    aral_page_free_unlock(ar, page);

    aral_lock(ar);

    internal_fatal(!page->aral_lock.used_elements,
                   "ARAL: '%s' pointer %p is inside a page without any active allocations.",
                   ar->config.name, ptr);

    internal_fatal(page->max_elements != page->aral_lock.used_elements + page->aral_lock.free_elements,
                   "ARAL: '%s' page element counters do not match, "
                   "page says it can handle %zu elements, "
                   "but there are %zu used and %zu free items, "
                   "total %zu items",
                   ar->config.name,
                   (size_t)page->max_elements,
                   (size_t)page->aral_lock.used_elements, (size_t)page->aral_lock.free_elements,
                   (size_t)page->aral_lock.used_elements + (size_t)page->aral_lock.free_elements
    );

    page->aral_lock.used_elements--;
    page->aral_lock.free_elements++;

    ar->aral_lock.user_free_operations++;

    // if the page is empty, release it
    if(unlikely(!page->aral_lock.used_elements)) {
        bool is_this_page_the_last_one = ar->aral_lock.pages == page && !page->aral_lock.next;

        if(!is_this_page_the_last_one)
            DOUBLE_LINKED_LIST_REMOVE_ITEM_UNSAFE(ar->aral_lock.pages, page, aral_lock.prev, aral_lock.next);

        aral_unlock(ar);

        if(!is_this_page_the_last_one)
            aral_del_page___no_lock_needed(ar, page TRACE_ALLOCATIONS_FUNCTION_CALL_PARAMS);
    }
    else {
        aral_move_page_with_free_list___aral_lock_needed(ar, page);
        aral_unlock(ar);
    }
}

void aral_destroy_internal(ARAL *ar TRACE_ALLOCATIONS_FUNCTION_DEFINITION_PARAMS) {
    aral_lock(ar);

    ARAL_PAGE *page;
    while((page = ar->aral_lock.pages)) {
        DOUBLE_LINKED_LIST_REMOVE_ITEM_UNSAFE(ar->aral_lock.pages, page, aral_lock.prev, aral_lock.next);
        aral_del_page___no_lock_needed(ar, page TRACE_ALLOCATIONS_FUNCTION_CALL_PARAMS);
    }

    aral_unlock(ar);

    if(ar->config.options & ARAL_ALLOCATED_STATS)
        freez(ar->stats);

    freez(ar);
}

size_t aral_element_size(ARAL *ar) {
    return ar->config.requested_element_size;
}

ARAL *aral_create(const char *name, size_t element_size, size_t initial_page_elements, size_t max_page_size,
                  struct aral_statistics *stats, const char *filename, char **cache_dir, bool mmap, bool lockless) {
    ARAL *ar = callocz(1, sizeof(ARAL));
    ar->config.options = (lockless) ? ARAL_LOCKLESS : 0;
    ar->config.requested_element_size = element_size;
    ar->config.initial_page_elements = initial_page_elements;
    ar->config.requested_max_page_size = max_page_size;
    ar->config.mmap.filename = filename;
    ar->config.mmap.cache_dir = cache_dir;
    ar->config.mmap.enabled = mmap;
    strncpyz(ar->config.name, name, ARAL_MAX_NAME);
    netdata_spinlock_init(&ar->aral_lock.spinlock);

    if(stats) {
        ar->stats = stats;
        ar->config.options &= ~ARAL_ALLOCATED_STATS;
    }
    else {
        ar->stats = callocz(1, sizeof(struct aral_statistics));
        ar->config.options |= ARAL_ALLOCATED_STATS;
    }

    long int page_size = sysconf(_SC_PAGE_SIZE);
    if (unlikely(page_size == -1))
        ar->config.natural_page_size = 4096;
    else
        ar->config.natural_page_size = page_size;

    // we need to add a page pointer after the element
    // so, first align the element size to the pointer size
    ar->config.element_size = natural_alignment(ar->config.requested_element_size, sizeof(uintptr_t));

    // then add the size of a pointer to it
    ar->config.element_size += sizeof(uintptr_t);

    // make sure it is at least what we need for an ARAL_FREE slot
    if (ar->config.element_size < sizeof(ARAL_FREE))
        ar->config.element_size = sizeof(ARAL_FREE);

    // and finally align it to the natural alignment
    ar->config.element_size = natural_alignment(ar->config.element_size, ARAL_NATURAL_ALIGNMENT);

    ar->config.max_page_elements = ar->config.requested_max_page_size / ar->config.element_size;

    // we write the page pointer just after each element
    ar->config.page_ptr_offset = ar->config.element_size - sizeof(uintptr_t);

    if(ar->config.requested_element_size + sizeof(uintptr_t) > ar->config.element_size)
        fatal("ARAL: '%s' failed to calculate properly page_ptr_offset: "
              "element size %zu, sizeof(uintptr_t) %zu, natural alignment %zu, "
              "final element size %zu, page_ptr_offset %zu",
              ar->config.name, ar->config.requested_element_size, sizeof(uintptr_t), ARAL_NATURAL_ALIGNMENT,
              ar->config.element_size, ar->config.page_ptr_offset);

    //info("ARAL: element size %zu, sizeof(uintptr_t) %zu, natural alignment %zu, final element size %zu, page_ptr_offset %zu",
    //      ar->element_size, sizeof(uintptr_t), ARAL_NATURAL_ALIGNMENT, ar->internal.element_size, ar->internal.page_ptr_offset);


    if (ar->config.initial_page_elements < 2)
        ar->config.initial_page_elements = 2;

    if(ar->config.mmap.enabled && (!ar->config.mmap.cache_dir || !*ar->config.mmap.cache_dir)) {
        error("ARAL: '%s' mmap cache directory is not configured properly, disabling mmap.", ar->config.name);
        ar->config.mmap.enabled = false;
        internal_fatal(true, "ARAL: '%s' mmap cache directory is not configured properly", ar->config.name);
    }

    uint64_t max_alloc_size;
    if(!ar->config.max_page_elements)
        max_alloc_size = ar->config.mmap.enabled ? ARAL_MAX_PAGE_SIZE_MMAP : ARAL_MAX_PAGE_SIZE_MALLOC;
    else
        max_alloc_size = ar->config.max_page_elements * ar->config.element_size;

    ar->config.max_allocation_size = aral_align_alloc_size(ar, max_alloc_size);
    ar->adders.allocation_size = aral_align_alloc_size(ar, (uint64_t)ar->config.element_size * ar->config.initial_page_elements);
    ar->aral_lock.pages = NULL;
    ar->aral_lock.file_number = 0;

    if(ar->config.mmap.enabled) {
        char directory_name[FILENAME_MAX + 1];
        snprintfz(directory_name, FILENAME_MAX, "%s/array_alloc.mmap", *ar->config.mmap.cache_dir);
        int r = mkdir(directory_name, 0775);
        if (r != 0 && errno != EEXIST)
            fatal("Cannot create directory '%s'", directory_name);

        char file[FILENAME_MAX + 1];
        snprintfz(file, FILENAME_MAX, "%s.", ar->config.mmap.filename);
        aral_delete_leftover_files(ar->config.name, directory_name, file);
    }

    internal_error(true,
                   "ARAL: '%s' "
                   "element size %zu (requested %zu bytes), "
                   "min elements per page %zu (requested %zu), "
                   "max elements per page %zu, "
                   "max page size %zu bytes (requested %zu) "
                   , ar->config.name
                   , ar->config.element_size, ar->config.requested_element_size
                   , ar->adders.allocation_size / ar->config.element_size, ar->config.initial_page_elements
                   , ar->config.max_allocation_size / ar->config.element_size
                   , ar->config.max_allocation_size,  ar->config.requested_max_page_size
    );

    __atomic_add_fetch(&ar->stats->structures.allocations, 1, __ATOMIC_RELAXED);
    __atomic_add_fetch(&ar->stats->structures.allocated_bytes, sizeof(ARAL), __ATOMIC_RELAXED);
    return ar;
}

// ----------------------------------------------------------------------------
// global aral caching

#define ARAL_BY_SIZE_MAX_SIZE 1024

struct aral_by_size {
    ARAL *ar;
    int32_t refcount;
};

struct {
    struct aral_statistics shared_statistics;
    SPINLOCK spinlock;
    struct aral_by_size array[ARAL_BY_SIZE_MAX_SIZE + 1];
} aral_by_size_globals = {};

struct aral_statistics *aral_by_size_statistics(void) {
    return &aral_by_size_globals.shared_statistics;
}

size_t aral_by_size_structures(void) {
    return aral_structures_from_stats(&aral_by_size_globals.shared_statistics);
}

size_t aral_by_size_overhead(void) {
    return aral_overhead_from_stats(&aral_by_size_globals.shared_statistics);
}

ARAL *aral_by_size_acquire(size_t size) {
    netdata_spinlock_lock(&aral_by_size_globals.spinlock);

    ARAL *ar = NULL;

    if(size <= ARAL_BY_SIZE_MAX_SIZE && aral_by_size_globals.array[size].ar) {
        ar = aral_by_size_globals.array[size].ar;
        aral_by_size_globals.array[size].refcount++;

        internal_fatal(aral_element_size(ar) != size, "DICTIONARY: aral has size %zu but we want %zu",
                       aral_element_size(ar), size);
    }

    if(!ar) {
        char buf[30 + 1];
        snprintf(buf, 30, "size-%zu", size);
        ar = aral_create(buf,
                         size,
                         0,
                         65536 * ((size / 150) + 1),
                         &aral_by_size_globals.shared_statistics,
                         NULL, NULL, false, false);

        if(size <= ARAL_BY_SIZE_MAX_SIZE) {
            aral_by_size_globals.array[size].ar = ar;
            aral_by_size_globals.array[size].refcount = 1;
        }
    }

    netdata_spinlock_unlock(&aral_by_size_globals.spinlock);

    return ar;
}

void aral_by_size_release(ARAL *ar) {
    size_t size = aral_element_size(ar);

    if(size <= ARAL_BY_SIZE_MAX_SIZE) {
        netdata_spinlock_lock(&aral_by_size_globals.spinlock);

        internal_fatal(aral_by_size_globals.array[size].ar != ar,
                       "ARAL BY SIZE: aral pointers do not match");

        if(aral_by_size_globals.array[size].refcount <= 0)
            fatal("ARAL BY SIZE: double release detected");

        aral_by_size_globals.array[size].refcount--;
//        if(!aral_by_size_globals.array[size].refcount) {
//            aral_destroy(aral_by_size_globals.array[size].ar);
//            aral_by_size_globals.array[size].ar = NULL;
//        }

        netdata_spinlock_unlock(&aral_by_size_globals.spinlock);
    }
    else
        aral_destroy(ar);
}

// ----------------------------------------------------------------------------
// unittest

struct aral_unittest_config {
    bool single_threaded;
    bool stop;
    ARAL *ar;
    size_t elements;
    size_t threads;
    int errors;
};

static void *aral_test_thread(void *ptr) {
    struct aral_unittest_config *auc = ptr;
    ARAL *ar = auc->ar;
    size_t elements = auc->elements;

    void **pointers = callocz(elements, sizeof(void *));

    do {
        for (size_t i = 0; i < elements; i++) {
            pointers[i] = aral_mallocz(ar);
        }

        for (size_t div = 5; div >= 2; div--) {
            for (size_t i = 0; i < elements / div; i++) {
                aral_freez(ar, pointers[i]);
                pointers[i] = NULL;
            }

            for (size_t i = 0; i < elements / div; i++) {
                pointers[i] = aral_mallocz(ar);
            }
        }

        for (size_t step = 50; step >= 10; step -= 10) {
            for (size_t i = 0; i < elements; i += step) {
                aral_freez(ar, pointers[i]);
                pointers[i] = NULL;
            }

            for (size_t i = 0; i < elements; i += step) {
                pointers[i] = aral_mallocz(ar);
            }
        }

        for (size_t i = 0; i < elements; i++) {
            aral_freez(ar, pointers[i]);
            pointers[i] = NULL;
        }

        if (auc->single_threaded && ar->aral_lock.pages && ar->aral_lock.pages->aral_lock.used_elements) {
            fprintf(stderr, "\n\nARAL leftovers detected (1)\n\n");
            __atomic_add_fetch(&auc->errors, 1, __ATOMIC_RELAXED);
        }

        if(!auc->single_threaded && __atomic_load_n(&auc->stop, __ATOMIC_RELAXED))
            break;

        for (size_t i = 0; i < elements; i++) {
            pointers[i] = aral_mallocz(ar);
        }

        size_t increment = elements / ar->config.max_page_elements;
        for (size_t all = increment; all <= elements / 2; all += increment) {

            size_t to_free = (all % ar->config.max_page_elements) + 1;
            size_t step = elements / to_free;
            if(!step) step = 1;

            // fprintf(stderr, "all %zu, to free %zu, step %zu\n", all, to_free, step);

            size_t free_list[to_free];
            for (size_t i = 0; i < to_free; i++) {
                size_t pos = step * i;
                aral_freez(ar, pointers[pos]);
                pointers[pos] = NULL;
                free_list[i] = pos;
            }

            for (size_t i = 0; i < to_free; i++) {
                size_t pos = free_list[i];
                pointers[pos] = aral_mallocz(ar);
            }
        }

        for (size_t i = 0; i < elements; i++) {
            aral_freez(ar, pointers[i]);
            pointers[i] = NULL;
        }

        if (auc->single_threaded && ar->aral_lock.pages && ar->aral_lock.pages->aral_lock.used_elements) {
            fprintf(stderr, "\n\nARAL leftovers detected (2)\n\n");
            __atomic_add_fetch(&auc->errors, 1, __ATOMIC_RELAXED);
        }

    } while(!auc->single_threaded && !__atomic_load_n(&auc->stop, __ATOMIC_RELAXED));

    freez(pointers);

    return ptr;
}

int aral_stress_test(size_t threads, size_t elements, size_t seconds) {
    fprintf(stderr, "Running stress test of %zu threads, with %zu elements each, for %zu seconds...\n",
            threads, elements, seconds);

    struct aral_unittest_config auc = {
            .single_threaded = false,
            .threads = threads,
            .ar = aral_create("aral-stress-test", 20, 0, 8192, NULL, "aral-stress-test", NULL, false, false),
            .elements = elements,
            .errors = 0,
    };

    usec_t started_ut = now_monotonic_usec();
    netdata_thread_t thread_ptrs[threads];

    for(size_t i = 0; i < threads ; i++) {
        char tag[NETDATA_THREAD_NAME_MAX + 1];
        snprintfz(tag, NETDATA_THREAD_NAME_MAX, "TH[%zu]", i);
        netdata_thread_create(&thread_ptrs[i], tag,
                              NETDATA_THREAD_OPTION_JOINABLE | NETDATA_THREAD_OPTION_DONT_LOG,
                              aral_test_thread, &auc);
    }

    size_t malloc_done = 0;
    size_t free_done = 0;
    size_t countdown = seconds;
    while(countdown-- > 0) {
        sleep_usec(1 * USEC_PER_SEC);
        aral_lock(auc.ar);
        size_t m = auc.ar->aral_lock.user_malloc_operations;
        size_t f = auc.ar->aral_lock.user_free_operations;
        aral_unlock(auc.ar);
        fprintf(stderr, "ARAL executes %0.2f M malloc and %0.2f M free operations/s\n",
                (double)(m - malloc_done) / 1000000.0, (double)(f - free_done) / 1000000.0);
        malloc_done = m;
        free_done = f;
    }

    __atomic_store_n(&auc.stop, true, __ATOMIC_RELAXED);

//    fprintf(stderr, "Cancelling the threads...\n");
//    for(size_t i = 0; i < threads ; i++) {
//        netdata_thread_cancel(thread_ptrs[i]);
//    }

    fprintf(stderr, "Waiting the threads to finish...\n");
    for(size_t i = 0; i < threads ; i++) {
        netdata_thread_join(thread_ptrs[i], NULL);
    }

    usec_t ended_ut = now_monotonic_usec();

    if (auc.ar->aral_lock.pages && auc.ar->aral_lock.pages->aral_lock.used_elements) {
        fprintf(stderr, "\n\nARAL leftovers detected (3)\n\n");
        __atomic_add_fetch(&auc.errors, 1, __ATOMIC_RELAXED);
    }

    info("ARAL: did %zu malloc, %zu free, "
         "using %zu threads, in %llu usecs",
         auc.ar->aral_lock.user_malloc_operations,
         auc.ar->aral_lock.user_free_operations,
         threads,
         ended_ut - started_ut);

    aral_destroy(auc.ar);

    return auc.errors;
}

int aral_unittest(size_t elements) {
    char *cache_dir = "/tmp/";

    struct aral_unittest_config auc = {
            .single_threaded = true,
            .threads = 1,
            .ar = aral_create("aral-test", 20, 0, 8192, NULL, "aral-test", &cache_dir, false, false),
            .elements = elements,
            .errors = 0,
    };

    aral_test_thread(&auc);

    aral_destroy(auc.ar);

    int errors = aral_stress_test(2, elements, 5);

    return auc.errors + errors;
}
