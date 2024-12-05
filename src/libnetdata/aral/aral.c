#include "../libnetdata.h"
#include "aral.h"

// #define NETDATA_ARAL_INTERNAL_CHECKS 1

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

// we don't need alignof(max_align_t) for normal C structures
// alignof(uintptr_r) is sufficient for our use cases
// #define SYSTEM_REQUIRED_ALIGNMENT (alignof(max_align_t))
#define SYSTEM_REQUIRED_ALIGNMENT (alignof(uintptr_t))

typedef struct aral_free {
    size_t size;
    struct aral_free *next;
} ARAL_FREE;

typedef struct aral_page {
    bool marked;
    uint32_t size;                      // the allocation size of the page
    const char *filename;
    uint8_t *data;

    uint32_t free_elements_to_move_first;
    uint32_t max_elements;              // the number of elements that can fit on this page

    struct {
        uint32_t used_elements;         // the number of used elements on this page
        uint32_t free_elements;         // the number of free elements on this page
        uint32_t marked_elements;

        struct aral_page *prev;         // the prev page on the list
        struct aral_page *next;         // the next page on the list
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

struct aral_ops {
    struct {
        alignas(64) size_t allocators; // the number of threads currently trying to allocate memory
        alignas(64) size_t deallocators; // the number of threads currently trying to deallocate memory
    } atomic;

    struct {
        alignas(64) SPINLOCK spinlock;
        size_t allocating_elements;     // currently allocating elements
        size_t allocation_size;         // current / next allocation size
    } adders;
};

struct aral {

    struct {
        char name[ARAL_MAX_NAME + 1];

        ARAL_OPTIONS options;

        size_t element_size;            // calculated to take into account ARAL overheads
        size_t max_allocation_size;     // calculated in bytes
        size_t max_page_elements;       // calculated
        size_t page_ptr_offset;         // calculated
        size_t system_page_size;        // calculated

        size_t initial_page_elements;
        size_t requested_element_size;
        size_t requested_max_page_size;

        struct {
            bool enabled;
            const char *filename;
            const char **cache_dir;
        } mmap;
    } config;

    struct {
        alignas(64) SPINLOCK spinlock;
        size_t file_number;             // for mmap
        ARAL_PAGE *pages;        // linked list of pages
        ARAL_PAGE *pages_marked;

        size_t user_malloc_operations;
        size_t user_free_operations;
        size_t defragment_operations;
        size_t defragment_linked_list_traversals;
    } aral_lock;

    struct aral_ops ops[2];

    struct aral_statistics *stats;
};

#define mark_to_idx(marked) (marked ? 1 : 0)
#define aral_pages_head(ar, marked) (marked ? &ar->aral_lock.pages_marked : &ar->aral_lock.pages)

static void aral_defrag_sorted_page_position___aral_lock_needed(ARAL *ar, ARAL_PAGE *page);

const char *aral_name(ARAL *ar) {
    return ar->config.name;
}

size_t aral_structures_from_stats(struct aral_statistics *stats) {
    if(!stats) return 0;
    return __atomic_load_n(&stats->structures.allocated_bytes, __ATOMIC_RELAXED);
}

size_t aral_overhead_from_stats(struct aral_statistics *stats) {
    if(!stats) return 0;

    size_t allocated = __atomic_load_n(&stats->malloc.allocated_bytes, __ATOMIC_RELAXED) +
                       __atomic_load_n(&stats->mmap.allocated_bytes, __ATOMIC_RELAXED);

    size_t used = __atomic_load_n(&stats->malloc.used_bytes, __ATOMIC_RELAXED) +
                  __atomic_load_n(&stats->mmap.used_bytes, __ATOMIC_RELAXED);

    if(allocated > used) return allocated - used;
    return allocated;
}

size_t aral_used_bytes_from_stats(struct aral_statistics *stats) {
    size_t used = __atomic_load_n(&stats->malloc.used_bytes, __ATOMIC_RELAXED) +
                  __atomic_load_n(&stats->mmap.used_bytes, __ATOMIC_RELAXED);

    return used;
}

size_t aral_overhead(ARAL *ar) {
    return aral_overhead_from_stats(ar->stats);
}

size_t aral_structures(ARAL *ar) {
    return aral_structures_from_stats(ar->stats);
}

struct aral_statistics *aral_get_statistics(ARAL *ar) {
    return ar->stats;
}

static inline size_t memory_alignment(size_t size, size_t alignment) {
    // return (size + alignment - 1) & ~(alignment - 1); // assumees alignment is power of 2
    return ((size + alignment - 1) / alignment) * alignment;
}

static size_t aral_align_alloc_size(ARAL *ar, uint64_t size) {
    size = memory_alignment(size, ar->config.system_page_size);

    if(size % ar->config.element_size)
        size -= size % ar->config.element_size;

    return size;
}

static inline void aral_lock(ARAL *ar) {
    if(likely(!(ar->config.options & ARAL_LOCKLESS)))
        spinlock_lock(&ar->aral_lock.spinlock);
}

static inline void aral_unlock(ARAL *ar) {
    if(likely(!(ar->config.options & ARAL_LOCKLESS)))
        spinlock_unlock(&ar->aral_lock.spinlock);
}

static inline void aral_page_free_lock(ARAL *ar, ARAL_PAGE *page) {
    if(likely(!(ar->config.options & ARAL_LOCKLESS)))
        spinlock_lock(&page->free.spinlock);
}

static inline void aral_page_free_unlock(ARAL *ar, ARAL_PAGE *page) {
    if(likely(!(ar->config.options & ARAL_LOCKLESS)))
        spinlock_unlock(&page->free.spinlock);
}

static inline bool aral_adders_trylock(ARAL *ar, bool marked) {
    if(likely(!(ar->config.options & ARAL_LOCKLESS))) {
        size_t idx = mark_to_idx(marked);
        return spinlock_trylock(&ar->ops[idx].adders.spinlock);
    }

    return true;
}

static inline void aral_adders_lock(ARAL *ar, bool marked) {
    if(likely(!(ar->config.options & ARAL_LOCKLESS))) {
        size_t idx = mark_to_idx(marked);
        spinlock_lock(&ar->ops[idx].adders.spinlock);
    }
}

static inline void aral_adders_unlock(ARAL *ar, bool marked) {
    if(likely(!(ar->config.options & ARAL_LOCKLESS))) {
        size_t idx = mark_to_idx(marked);
        spinlock_unlock(&ar->ops[idx].adders.spinlock);
    }
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
        netdata_log_info("ARAL: '%s' removing left-over file '%s'", name, full_path);
        if(unlikely(unlink(full_path) == -1))
            netdata_log_error("ARAL: '%s' cannot delete file '%s'", name, full_path);
    }

    closedir(dir);
}

// ----------------------------------------------------------------------------
// find the page a pointer belongs to

#ifdef NETDATA_ARAL_INTERNAL_CHECKS
static inline ARAL_PAGE *find_page_with_allocation_internal_check(ARAL *ar, void *ptr, bool marked) {
    aral_lock(ar);

    uintptr_t seeking = (uintptr_t)ptr;
    ARAL_PAGE *page;

    for(page = *aral_pages_head(ar, marked); page ; page = page->aral_lock.next) {
        if(unlikely(seeking >= (uintptr_t)page->data && seeking < (uintptr_t)page->data + page->size))
            break;
    }

    aral_unlock(ar);

    return page;
}
#endif

// ----------------------------------------------------------------------------
// Tagging the pointer with the 'marked' flag

// Retrieving the pointer and the 'marked' flag
static ARAL_PAGE *aral_get_page_pointer_after_element___do_NOT_have_aral_lock(ARAL *ar, void *ptr, bool *marked) {
    uint8_t *data = ptr;
    uintptr_t *page_ptr = (uintptr_t *)&data[ar->config.page_ptr_offset];
    uintptr_t tagged_page = __atomic_load_n(page_ptr, __ATOMIC_ACQUIRE);  // Atomically load the tagged pointer
    *marked = (tagged_page & 1) != 0;  // Extract the LSB as the 'marked' flag
    ARAL_PAGE *page = (ARAL_PAGE *)(tagged_page & ~1);  // Mask out the LSB to get the original pointer

    internal_fatal(!page,
                   "ARAL: '%s' possible corruption or double free of pointer %p",
                   ar->config.name, ptr);

#ifdef NETDATA_ARAL_INTERNAL_CHECKS
    {
        // find the page ptr belongs
        ARAL_PAGE *page2 = find_page_with_allocation_internal_check(ar, ptr, *marked);
        if(!page2) {
            page2 = find_page_with_allocation_internal_check(ar, ptr, !(*marked));
            internal_fatal(page2 && (*marked) && !page2->marked, "ARAL: '%s' page pointer is in different mark index",
                           ar->config.name);
        }

        internal_fatal(page != page2,
                       "ARAL: '%s' page pointers do not match!",
                       ar->config.name);

        internal_fatal(!page2,
                       "ARAL: '%s' free of pointer %p is not in ARAL address space.",
                       ar->config.name, ptr);
    }
#endif

    return page;
}

static void aral_set_page_pointer_after_element___do_NOT_have_aral_lock(ARAL *ar, void *page, void *ptr, bool marked) {
    uint8_t *data = ptr;
    uintptr_t *page_ptr = (uintptr_t *)&data[ar->config.page_ptr_offset];
    uintptr_t tagged_page = (uintptr_t)page;  // Cast the pointer to an integer
    if (marked) tagged_page |= 1; // Set the LSB to 1 if 'marked' is true
    __atomic_store_n(page_ptr, tagged_page, __ATOMIC_RELEASE);  // Atomically store the tagged pointer
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

size_t aral_next_allocation_size___adders_lock_needed(ARAL *ar, bool marked) {
    size_t idx = mark_to_idx(marked);
    size_t size = ar->ops[idx].adders.allocation_size;

    if(size > ar->config.max_allocation_size)
        size = ar->config.max_allocation_size;
    else
        ar->ops[idx].adders.allocation_size = aral_align_alloc_size(ar, (uint64_t)ar->ops[idx].adders.allocation_size * 2);

    return size;
}

static ARAL_PAGE *aral_create_page___no_lock_needed(ARAL *ar, size_t size TRACE_ALLOCATIONS_FUNCTION_DEFINITION_PARAMS) {
    ARAL_PAGE *page = callocz(1, sizeof(ARAL_PAGE));
    spinlock_init(&page->free.spinlock);
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
            fatal("ARAL: '%s' cannot allocate aral buffer of size %u on filename '%s'",
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
            netdata_log_error("Cannot delete file '%s'", page->filename);

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
    internal_fatal(!page, "page cannot be NULL in this function");

    ARAL_PAGE **head_ptr = aral_pages_head(ar, page->marked);
    ARAL_PAGE *first = *head_ptr;

    if (page->aral_lock.free_elements <= page->free_elements_to_move_first ||
        !first ||
        !first->aral_lock.free_elements ||
        page->aral_lock.free_elements <= first->aral_lock.free_elements + ARAL_FREE_PAGES_DELTA_TO_REARRANGE_LIST) {
        // first position
        DOUBLE_LINKED_LIST_PREPEND_ITEM_UNSAFE(*head_ptr, page, aral_lock.prev, aral_lock.next);
    }
    else {
        ARAL_PAGE *second = first->aral_lock.next;

        if (!second ||
            !second->aral_lock.free_elements ||
            page->aral_lock.free_elements <= second->aral_lock.free_elements)
            // second position
            DOUBLE_LINKED_LIST_INSERT_ITEM_AFTER_UNSAFE(*head_ptr, first, page, aral_lock.prev, aral_lock.next);
        else
            // third position
            DOUBLE_LINKED_LIST_INSERT_ITEM_AFTER_UNSAFE(*head_ptr, second, page, aral_lock.prev, aral_lock.next);
    }
}

#ifdef NETDATA_ARAL_INTERNAL_CHECKS
struct free_space {
    size_t pages;
    size_t pages_with_free_elements;
    size_t max_free_elements_on_a_page;
    size_t free_elements;
    size_t max_page_elements;
    bool last_had_free;
    bool failed;
    ARAL_PAGE *p, *lp;
};

static inline struct free_space check_free_space___aral_lock_needed(ARAL *ar, ARAL_PAGE *my_page, bool marked) {
    struct free_space f = { 0 };

    f.max_page_elements = ar->config.max_allocation_size / ar->config.element_size;
    f.last_had_free = true;
    for(f.p = *aral_pages_head(ar, marked); f.p ; f.lp = f.p, f.p = f.p->aral_lock.next) {
        f.pages++;
        if(f.p->aral_lock.free_elements) {
            if(f.p != my_page && f.max_free_elements_on_a_page < f.p->aral_lock.free_elements)
                f.max_free_elements_on_a_page = f.p->aral_lock.free_elements;

            f.free_elements += f.p->aral_lock.free_elements;
            f.pages_with_free_elements++;
            internal_fatal(!f.last_had_free, "found page with free items after a full page");
        }
        else
            f.last_had_free = false;
    }

    f.failed = f.free_elements >= f.max_page_elements * 3;
    return f;
}
#endif

static inline ARAL_PAGE *aral_get_first_page_with_a_free_slot(ARAL *ar, bool marked TRACE_ALLOCATIONS_FUNCTION_DEFINITION_PARAMS) {
    size_t idx = mark_to_idx(marked);
    __atomic_add_fetch(&ar->ops[idx].atomic.allocators, 1, __ATOMIC_RELAXED);
    aral_lock(ar);

    ARAL_PAGE **head_ptr = aral_pages_head(ar, marked);
    ARAL_PAGE *page = *head_ptr;

#ifdef NETDATA_ARAL_INTERNAL_CHECKS
    // bool added = false;
    struct free_space f1, f2;
#endif

    while(!page || !page->aral_lock.free_elements) {
        internal_fatal(page && page->aral_lock.next && page->aral_lock.next->aral_lock.free_elements, "hey!");

#ifdef NETDATA_ARAL_INTERNAL_CHECKS
        f1 = check_free_space___aral_lock_needed(ar, NULL, marked);
        internal_fatal(f1.failed || f1.free_elements, "hey!");
#endif

        size_t page_allocation_size = 0;
        bool can_add = false;
        if(aral_adders_trylock(ar, marked)) {
            // we can add a page - let's see it is really needed
            size_t threads_currently_allocating = __atomic_load_n(&ar->ops[idx].atomic.allocators, __ATOMIC_RELAXED);
            size_t threads_currently_deallocating = __atomic_load_n(&ar->ops[idx].atomic.deallocators, __ATOMIC_RELAXED);

            // we will allocate a page, only if the number of elements required is more than the
            // sum of all new allocations under their way plus the pages currently being deallocated
            if(ar->ops[idx].adders.allocating_elements + threads_currently_deallocating < threads_currently_allocating) {
                can_add = true;
                page_allocation_size = aral_next_allocation_size___adders_lock_needed(ar, marked);
                ar->ops[idx].adders.allocating_elements += page_allocation_size / ar->config.element_size;
            }
            aral_adders_unlock(ar, marked);
        }
        aral_unlock(ar);

        if(can_add) {
            page = aral_create_page___no_lock_needed(ar, page_allocation_size TRACE_ALLOCATIONS_FUNCTION_CALL_PARAMS);
            page->marked = marked;

            aral_lock(ar);

            aral_insert_not_linked_page_with_free_items_to_proper_position___aral_lock_needed(ar, page);

//#ifdef NETDATA_ARAL_INTERNAL_CHECKS
//            added = true;
//#endif

            aral_adders_lock(ar, marked);
            ar->ops[idx].adders.allocating_elements -= page_allocation_size / ar->config.element_size;
            aral_adders_unlock(ar, marked);

            // we have a page that is all empty
            // and only aral_lock() is held, so
            // break the loop
            break;
        }
        else {
            // let the adders/deallocators do it
            // tinysleep();
            sched_yield();

            aral_lock(ar);
            page = *head_ptr;
        }
    }

    // we have a page
    // and aral locked

    internal_fatal(marked && !page->marked, "ARAL: requested a marked page, but the page found is not marked");

//#ifdef NETDATA_ARAL_INTERNAL_CHECKS
//    if(added) {
//        f2 = check_free_space___aral_lock_needed(ar, page, marked);
//        internal_fatal(f2.failed, "hey!");
//    }
//#endif

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
    page->aral_lock.free_elements--;

    if(marked)
        page->aral_lock.marked_elements++;

    internal_fatal(page->aral_lock.marked_elements > page->aral_lock.used_elements,
                   "page has more marked elements than the used ones");

    if(page->aral_lock.free_elements == 0) {
        if((*head_ptr)->aral_lock.prev != page) {
            // we are done with this page
            // move the full page last
            // so that pages with free items remain first in the list
            DOUBLE_LINKED_LIST_REMOVE_ITEM_UNSAFE(*head_ptr, page, aral_lock.prev, aral_lock.next);
            DOUBLE_LINKED_LIST_APPEND_ITEM_UNSAFE(*head_ptr, page, aral_lock.prev, aral_lock.next);
        }
    }
    else if(page->aral_lock.free_elements == 1) {
        if((*head_ptr) != page) {
            // we are done with this page
            // move the full page last
            // so that pages with free items remain first in the list
            DOUBLE_LINKED_LIST_REMOVE_ITEM_UNSAFE(*head_ptr, page, aral_lock.prev, aral_lock.next);
            DOUBLE_LINKED_LIST_PREPEND_ITEM_UNSAFE(*head_ptr, page, aral_lock.prev, aral_lock.next);
        }
    }

    __atomic_sub_fetch(&ar->ops[idx].atomic.allocators, 1, __ATOMIC_RELAXED);
    aral_unlock(ar);

    return page;
}

void *aral_callocz_internal(ARAL *ar, bool marked TRACE_ALLOCATIONS_FUNCTION_DEFINITION_PARAMS) {
    void *r = aral_mallocz_internal(ar, marked TRACE_ALLOCATIONS_FUNCTION_CALL_PARAMS);
    memset(r, 0, ar->config.requested_element_size);
    return r;
}

void *aral_mallocz_internal(ARAL *ar, bool marked TRACE_ALLOCATIONS_FUNCTION_DEFINITION_PARAMS) {
#if defined(FSANITIZE_ADDRESS)
    return mallocz(ar->config.requested_element_size);
#endif

    // reserve a slot on a free page
    ARAL_PAGE *page = aral_get_first_page_with_a_free_slot(ar, marked TRACE_ALLOCATIONS_FUNCTION_CALL_PARAMS);
    // the page returned has reserved a slot for us

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
    aral_set_page_pointer_after_element___do_NOT_have_aral_lock(ar, page, found_fr, marked);

    if(unlikely(ar->config.mmap.enabled))
        __atomic_add_fetch(&ar->stats->mmap.used_bytes, ar->config.element_size, __ATOMIC_RELAXED);
    else
        __atomic_add_fetch(&ar->stats->malloc.used_bytes, ar->config.element_size, __ATOMIC_RELAXED);

    return (void *)found_fr;
}

static void aral_defrag_sorted_page_position___aral_lock_needed(ARAL *ar, ARAL_PAGE *page) {
    internal_fatal(!page, "page cannot be NULL in this function");

    ARAL_PAGE **head_ptr = aral_pages_head(ar, page->marked);
    ARAL_PAGE *tmp;

    int action = 0; (void)action;
    size_t move_later = 0, move_earlier = 0;

    for(tmp = page->aral_lock.next ;
        tmp && tmp->aral_lock.free_elements && tmp->aral_lock.free_elements < page->aral_lock.free_elements ;
        tmp = tmp->aral_lock.next)
        move_later++;

    if(!tmp && page->aral_lock.next) {
        DOUBLE_LINKED_LIST_REMOVE_ITEM_UNSAFE(*head_ptr, page, aral_lock.prev, aral_lock.next);
        DOUBLE_LINKED_LIST_APPEND_ITEM_UNSAFE(*head_ptr, page, aral_lock.prev, aral_lock.next);
        action = 1;
    }
    else if(tmp != page->aral_lock.next) {
        DOUBLE_LINKED_LIST_REMOVE_ITEM_UNSAFE(*head_ptr, page, aral_lock.prev, aral_lock.next);
        DOUBLE_LINKED_LIST_INSERT_ITEM_BEFORE_UNSAFE(*head_ptr, tmp, page, aral_lock.prev, aral_lock.next);
        action = 2;
    }
    else {
        for(tmp = (page == *head_ptr) ? NULL : page->aral_lock.prev ;
            tmp && tmp != page && (!tmp->aral_lock.free_elements || tmp->aral_lock.free_elements > page->aral_lock.free_elements);
            tmp = (tmp == *head_ptr) ? NULL : tmp->aral_lock.prev)
            move_earlier++;

        if(!tmp) {
            DOUBLE_LINKED_LIST_REMOVE_ITEM_UNSAFE(*head_ptr, page, aral_lock.prev, aral_lock.next);
            DOUBLE_LINKED_LIST_PREPEND_ITEM_UNSAFE(*head_ptr, page, aral_lock.prev, aral_lock.next);
            action = 3;
        }
        else if(tmp != page->aral_lock.prev){
            DOUBLE_LINKED_LIST_REMOVE_ITEM_UNSAFE(*head_ptr, page, aral_lock.prev, aral_lock.next);
            DOUBLE_LINKED_LIST_INSERT_ITEM_AFTER_UNSAFE(*head_ptr, tmp, page, aral_lock.prev, aral_lock.next);
            action = 4;
        }
    }

    ar->aral_lock.defragment_operations++;
    ar->aral_lock.defragment_linked_list_traversals += move_earlier + move_later;

    internal_fatal(page->aral_lock.next && page->aral_lock.next->aral_lock.free_elements && page->aral_lock.next->aral_lock.free_elements < page->aral_lock.free_elements,
                   "ARAL: '%s' item should be later in the list", ar->config.name);

    internal_fatal(page != *head_ptr && (!page->aral_lock.prev->aral_lock.free_elements || page->aral_lock.prev->aral_lock.free_elements > page->aral_lock.free_elements),
                   "ARAL: '%s' item should be earlier in the list", ar->config.name);
}

// returns true if it moved the page to the unmarked list
static bool aral_remove_marked_allocation___aral_lock_needed(ARAL *ar, ARAL_PAGE *page) {
    internal_fatal(!page->aral_lock.marked_elements, "marked elements refcount found zero");

    bool rc = false;
    page->aral_lock.marked_elements--;
    if (!page->aral_lock.marked_elements && page->aral_lock.used_elements) {
        DOUBLE_LINKED_LIST_REMOVE_ITEM_UNSAFE(ar->aral_lock.pages_marked, page, aral_lock.prev, aral_lock.next);
        if(page->aral_lock.free_elements)
            DOUBLE_LINKED_LIST_PREPEND_ITEM_UNSAFE(ar->aral_lock.pages, page, aral_lock.prev, aral_lock.next);
        else
            DOUBLE_LINKED_LIST_APPEND_ITEM_UNSAFE(ar->aral_lock.pages, page, aral_lock.prev, aral_lock.next);
        page->marked = false;
        rc = true;
    }

    internal_fatal(page->aral_lock.marked_elements > page->aral_lock.used_elements,
                   "page has more marked elements than the used ones");

    return rc;
}

void aral_unmark_allocation(ARAL *ar, void *ptr) {
    if(unlikely(!ptr)) return;

    // get the page pointer
    bool marked;
    ARAL_PAGE *page = aral_get_page_pointer_after_element___do_NOT_have_aral_lock(ar, ptr, &marked);

    internal_fatal(!page->marked, "This allocation does not belong to a marked page");
    internal_fatal(!marked, "This allocation does is not marked");

    if(marked)
        aral_set_page_pointer_after_element___do_NOT_have_aral_lock(ar, page, ptr, false);

    if(marked && page->marked) {
        aral_lock(ar);
        aral_remove_marked_allocation___aral_lock_needed(ar, page);
        aral_unlock(ar);
    }
}

void aral_freez_internal(ARAL *ar, void *ptr TRACE_ALLOCATIONS_FUNCTION_DEFINITION_PARAMS) {
#if defined(FSANITIZE_ADDRESS)
    freez(ptr);
    return;
#endif

    if(unlikely(!ptr)) return;

    if(unlikely(ar->config.mmap.enabled))
        __atomic_sub_fetch(&ar->stats->mmap.used_bytes, ar->config.element_size, __ATOMIC_RELAXED);
    else
        __atomic_sub_fetch(&ar->stats->malloc.used_bytes, ar->config.element_size, __ATOMIC_RELAXED);

    // get the page pointer
    bool marked;
    ARAL_PAGE *page = aral_get_page_pointer_after_element___do_NOT_have_aral_lock(ar, ptr, &marked);

    size_t idx = mark_to_idx(marked);
    __atomic_add_fetch(&ar->ops[idx].atomic.deallocators, 1, __ATOMIC_RELAXED);

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

    internal_fatal(marked && !page->marked, "ARAL: found a marked element on a non-marked page");

    if(marked && page->marked) {
        // IMPORTANT: the page may be moved between pages and marked pages linked-lists
        // DO NOT GET THE HEAD BEFORE THIS CALL
        aral_remove_marked_allocation___aral_lock_needed(ar, page);
    }

    ARAL_PAGE **head_ptr = aral_pages_head(ar, page->marked);

    internal_fatal(page->aral_lock.marked_elements > page->aral_lock.used_elements,
                   "page has more marked elements than the used ones");

    // if the page is empty, release it
    if(unlikely(!page->aral_lock.used_elements)) {
        internal_fatal(page->aral_lock.marked_elements, "page has marked elements but not used ones");

        bool is_this_page_the_last_one = *head_ptr == page && !page->aral_lock.next;

        if(!is_this_page_the_last_one)
            DOUBLE_LINKED_LIST_REMOVE_ITEM_UNSAFE(*head_ptr, page, aral_lock.prev, aral_lock.next);

        __atomic_sub_fetch(&ar->ops[idx].atomic.deallocators, 1, __ATOMIC_RELAXED);
        aral_unlock(ar);

        if(!is_this_page_the_last_one)
            aral_del_page___no_lock_needed(ar, page TRACE_ALLOCATIONS_FUNCTION_CALL_PARAMS);

        return;
    }

    if(*head_ptr != page) {
        if (page->aral_lock.free_elements == 1) {
            // we are done with this page - move the full page last,
            // so that pages with free items remain first in the list
            DOUBLE_LINKED_LIST_REMOVE_ITEM_UNSAFE(*head_ptr, page, aral_lock.prev, aral_lock.next);
            DOUBLE_LINKED_LIST_PREPEND_ITEM_UNSAFE(*head_ptr, page, aral_lock.prev, aral_lock.next);
        }
        else {
            if (likely(!(ar->config.options & ARAL_DEFRAGMENT))) {
                DOUBLE_LINKED_LIST_REMOVE_ITEM_UNSAFE(*head_ptr, page, aral_lock.prev, aral_lock.next);
                aral_insert_not_linked_page_with_free_items_to_proper_position___aral_lock_needed(ar, page);
            }
            else
                aral_defrag_sorted_page_position___aral_lock_needed(ar, page);
        }
    }

    __atomic_sub_fetch(&ar->ops[idx].atomic.deallocators, 1, __ATOMIC_RELAXED);
    aral_unlock(ar);
}

void aral_destroy_internal(ARAL *ar TRACE_ALLOCATIONS_FUNCTION_DEFINITION_PARAMS) {
    aral_lock(ar);

    ARAL_PAGE **head_ptr = aral_pages_head(ar, false);
    ARAL_PAGE *page;
    while((page = *head_ptr)) {
        DOUBLE_LINKED_LIST_REMOVE_ITEM_UNSAFE(*head_ptr, page, aral_lock.prev, aral_lock.next);
        aral_del_page___no_lock_needed(ar, page TRACE_ALLOCATIONS_FUNCTION_CALL_PARAMS);
    }

    head_ptr = aral_pages_head(ar, true);
    while((page = *head_ptr)) {
        DOUBLE_LINKED_LIST_REMOVE_ITEM_UNSAFE(*head_ptr, page, aral_lock.prev, aral_lock.next);
        aral_del_page___no_lock_needed(ar, page TRACE_ALLOCATIONS_FUNCTION_CALL_PARAMS);
    }

    aral_unlock(ar);

    if(ar->config.options & ARAL_ALLOCATED_STATS)
        freez(ar->stats);

    freez(ar);
}

size_t aral_requested_element_size(ARAL *ar) {
    return ar->config.requested_element_size;
}

size_t aral_actual_element_size(ARAL *ar) {
    return ar->config.element_size;
}

static size_t aral_allocation_slot_size(size_t requested_element_size, bool usable) {
    // we need to add a page pointer after the element
    // so, first align the element size to the pointer size
    size_t element_size = memory_alignment(requested_element_size, sizeof(uintptr_t));

    // then add the size of a pointer to it
    element_size += sizeof(uintptr_t);

    // make sure it is at least what we need for an ARAL_FREE slot
    if (element_size < sizeof(ARAL_FREE))
        element_size = sizeof(ARAL_FREE);

    // and finally align it to the natural alignment
    element_size = memory_alignment(element_size, SYSTEM_REQUIRED_ALIGNMENT);

    if(usable)
        return element_size - sizeof(uintptr_t);

    return element_size;
}

size_t aral_optimal_page_size(void) {
    // for allocations above 128KiB, glibc gives them back to the system
    // immediately when freed.
    return 256 * 1024;
}

static void optimal_max_page_size(ARAL *ar) {
    if(ar->config.requested_max_page_size)
        return;

    ar->config.requested_max_page_size = aral_optimal_page_size();
}

ARAL *aral_create(const char *name, size_t element_size, size_t initial_page_elements, size_t max_page_size,
                  struct aral_statistics *stats, const char *filename, const char **cache_dir, bool mmap, bool lockless) {
    ARAL *ar = callocz(1, sizeof(ARAL));
    ar->config.options = ((lockless) ? ARAL_LOCKLESS : 0);
    ar->config.requested_element_size = element_size;
    ar->config.initial_page_elements = initial_page_elements;
    ar->config.requested_max_page_size = max_page_size;
    ar->config.mmap.filename = filename;
    ar->config.mmap.cache_dir = cache_dir;
    ar->config.mmap.enabled = mmap;
    strncpyz(ar->config.name, name, ARAL_MAX_NAME);
    spinlock_init(&ar->aral_lock.spinlock);
    spinlock_init(&ar->ops[0].adders.spinlock);
    spinlock_init(&ar->ops[1].adders.spinlock);

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
        ar->config.system_page_size = 4096;
    else
        ar->config.system_page_size = page_size;

    ar->config.element_size = aral_allocation_slot_size(ar->config.requested_element_size, false);
    optimal_max_page_size(ar);

    ar->config.max_page_elements = ar->config.requested_max_page_size / ar->config.element_size;

    // we write the page pointer just after each element
    ar->config.page_ptr_offset = ar->config.element_size - sizeof(uintptr_t);

    if(ar->config.requested_element_size + sizeof(uintptr_t) > ar->config.element_size)
        fatal("ARAL: '%s' failed to calculate properly page_ptr_offset: "
              "element size %zu, sizeof(uintptr_t) %zu, natural alignment %zu, "
              "final element size %zu, page_ptr_offset %zu",
              ar->config.name, ar->config.requested_element_size, sizeof(uintptr_t),
              SYSTEM_REQUIRED_ALIGNMENT,
              ar->config.element_size, ar->config.page_ptr_offset);

    //netdata_log_info("ARAL: element size %zu, sizeof(uintptr_t) %zu, natural alignment %zu, final element size %zu, page_ptr_offset %zu",
    //      ar->element_size, sizeof(uintptr_t), ARAL_NATURAL_ALIGNMENT, ar->internal.element_size, ar->internal.page_ptr_offset);


    if (ar->config.initial_page_elements < 2)
        ar->config.initial_page_elements = 2;

    if(ar->config.mmap.enabled && (!ar->config.mmap.cache_dir || !*ar->config.mmap.cache_dir)) {
        netdata_log_error("ARAL: '%s' mmap cache directory is not configured properly, disabling mmap.", ar->config.name);
        ar->config.mmap.enabled = false;
        internal_fatal(true, "ARAL: '%s' mmap cache directory is not configured properly", ar->config.name);
    }

    uint64_t max_alloc_size;
    if(!ar->config.max_page_elements)
        max_alloc_size = ar->config.mmap.enabled ? ARAL_MAX_PAGE_SIZE_MMAP : ARAL_MAX_PAGE_SIZE_MALLOC;
    else
        max_alloc_size = ar->config.max_page_elements * ar->config.element_size;

    ar->config.max_allocation_size = aral_align_alloc_size(ar, max_alloc_size);
    ar->ops[0].adders.allocation_size =
        ar->ops[1].adders.allocation_size =
        aral_align_alloc_size(ar, (uint64_t)ar->config.element_size * ar->config.initial_page_elements);
    ar->aral_lock.pages = NULL;
    ar->aral_lock.pages_marked = NULL;
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

    errno_clear();
    internal_error(true,
                   "ARAL: '%s' "
                   "element size %zu (requested %zu bytes), "
                   "min elements per page %zu (requested %zu), "
                   "max elements per page %zu, "
                   "max page size %zu bytes (requested %zu) "
                   , ar->config.name
                   , ar->config.element_size, ar->config.requested_element_size
                   , ar->ops[0].adders.allocation_size / ar->config.element_size, ar->config.initial_page_elements
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

size_t aral_by_size_used_bytes(void) {
    return aral_used_bytes_from_stats(&aral_by_size_globals.shared_statistics);
}

ARAL *aral_by_size_acquire(size_t size) {
    spinlock_lock(&aral_by_size_globals.spinlock);

    ARAL *ar = NULL;

    if(size <= ARAL_BY_SIZE_MAX_SIZE && aral_by_size_globals.array[size].ar) {
        ar = aral_by_size_globals.array[size].ar;
        aral_by_size_globals.array[size].refcount++;

        internal_fatal(
            aral_requested_element_size(ar) != size, "DICTIONARY: aral has size %zu but we want %zu",
            aral_requested_element_size(ar), size);
    }

    if(!ar) {
        char buf[30 + 1];
        snprintf(buf, 30, "size-%zu", size);
        ar = aral_create(buf,
                         size,
                         0,
                         0,
                         &aral_by_size_globals.shared_statistics,
                         NULL, NULL, false, false);

        if(size <= ARAL_BY_SIZE_MAX_SIZE) {
            aral_by_size_globals.array[size].ar = ar;
            aral_by_size_globals.array[size].refcount = 1;
        }
    }

    spinlock_unlock(&aral_by_size_globals.spinlock);

    return ar;
}

void aral_by_size_release(ARAL *ar) {
    size_t size = aral_requested_element_size(ar);

    if(size <= ARAL_BY_SIZE_MAX_SIZE) {
        spinlock_lock(&aral_by_size_globals.spinlock);

        internal_fatal(aral_by_size_globals.array[size].ar != ar,
                       "ARAL BY SIZE: aral pointers do not match");

        if(aral_by_size_globals.array[size].refcount <= 0)
            fatal("ARAL BY SIZE: double release detected");

        aral_by_size_globals.array[size].refcount--;
//        if(!aral_by_size_globals.array[size].refcount) {
//            aral_destroy(aral_by_size_globals.array[size].ar);
//            aral_by_size_globals.array[size].ar = NULL;
//        }

        spinlock_unlock(&aral_by_size_globals.spinlock);
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

struct aral_unittest_entry {
    char TXT[27];
    char txt[27];
    char nnn[10];
};

#define UNITTEST_ITEM (struct aral_unittest_entry){ \
    .TXT = "ABCDEFGHIJKLMNOPQRSTUVWXYZ",            \
    .txt = "abcdefghijklmnopqrstuvwxyz",            \
    .nnn = "123456789",                             \
}

static inline struct aral_unittest_entry *unittest_aral_malloc(ARAL *ar, bool marked) {
    struct aral_unittest_entry *t;
    if(marked)
        t = aral_mallocz_marked(ar);
    else
        t = aral_mallocz(ar);

    *t = UNITTEST_ITEM;
    return t;
}

static void *aral_test_thread(void *ptr) {
    struct aral_unittest_config *auc = ptr;
    ARAL *ar = auc->ar;
    size_t elements = auc->elements;

    bool marked = os_random(2);
    struct aral_unittest_entry **pointers = callocz(elements, sizeof(struct aral_unittest_entry *));

    size_t iterations = 0;
    do {
        iterations++;

        for (size_t i = 0; i < elements; i++) {
            pointers[i] = unittest_aral_malloc(ar, marked);
        }

        if(marked) {
            for (size_t i = 0; i < elements; i++) {
                aral_unmark_allocation(ar, pointers[i]);
            }
        }

        for (size_t div = 5; div >= 2; div--) {
            for (size_t i = 0; i < elements / div; i++) {
                aral_freez(ar, pointers[i]);
                pointers[i] = NULL;
            }

            for (size_t i = 0; i < elements / div; i++) {
                pointers[i] = unittest_aral_malloc(ar, marked);
            }
        }

        for (size_t step = 50; step >= 10; step -= 10) {
            for (size_t i = 0; i < elements; i += step) {
                aral_freez(ar, pointers[i]);
                pointers[i] = NULL;
            }

            for (size_t i = 0; i < elements; i += step) {
                pointers[i] = unittest_aral_malloc(ar, marked);
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
            pointers[i] = unittest_aral_malloc(ar, marked);
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
                pointers[pos] = unittest_aral_malloc(ar, marked);
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
            .ar = aral_create("aral-stress-test",
                          sizeof(struct aral_unittest_entry),
                          0,
                          16384,
                          NULL,
                          "aral-stress-test",
                          NULL, false, false),
            .elements = elements,
            .errors = 0,
    };

    usec_t started_ut = now_monotonic_usec();
    ND_THREAD *thread_ptrs[threads];

    for(size_t i = 0; i < threads ; i++) {
        char tag[ND_THREAD_TAG_MAX + 1];
        snprintfz(tag, ND_THREAD_TAG_MAX, "TH[%zu]", i);
        thread_ptrs[i] = nd_thread_create(
            tag,
            NETDATA_THREAD_OPTION_JOINABLE | NETDATA_THREAD_OPTION_DONT_LOG,
            aral_test_thread,
            &auc);
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
//        nd_thread_signal_cancel(thread_ptrs[i]);
//    }

    fprintf(stderr, "Waiting the threads to finish...\n");
    for(size_t i = 0; i < threads ; i++) {
        nd_thread_join(thread_ptrs[i]);
    }

    usec_t ended_ut = now_monotonic_usec();

    if (auc.ar->aral_lock.pages && auc.ar->aral_lock.pages->aral_lock.used_elements) {
        fprintf(stderr, "\n\nARAL leftovers detected (3)\n\n");
        __atomic_add_fetch(&auc.errors, 1, __ATOMIC_RELAXED);
    }

    netdata_log_info("ARAL: did %zu malloc, %zu free, "
         "using %zu threads, in %"PRIu64" usecs",
         auc.ar->aral_lock.user_malloc_operations,
         auc.ar->aral_lock.user_free_operations,
         threads,
         ended_ut - started_ut);

    aral_destroy(auc.ar);

    return auc.errors;
}

int aral_unittest(size_t elements) {
    const char *cache_dir = "/tmp/";

    struct aral_unittest_config auc = {
            .single_threaded = true,
            .threads = 1,
            .ar = aral_create("aral-test",
                          sizeof(struct aral_unittest_entry),
                          0,
                          65536,
                          NULL,
                          "aral-test",
                          &cache_dir,
                          false,
                          false),
            .elements = elements,
            .errors = 0,
    };

    aral_test_thread(&auc);

    aral_destroy(auc.ar);

    int errors = aral_stress_test(2, elements, 5);

    return auc.errors + errors;
}
