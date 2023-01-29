#include "../libnetdata.h"
#include "aral.h"

#ifdef NETDATA_TRACE_ALLOCATIONS
#define TRACE_ALLOCATIONS_FUNCTION_DEFINITION_PARAMS , const char *file, const char *function, size_t line
#define TRACE_ALLOCATIONS_FUNCTION_CALL_PARAMS , file, function, line
#else
#define TRACE_ALLOCATIONS_FUNCTION_DEFINITION_PARAMS
#define TRACE_ALLOCATIONS_FUNCTION_CALL_PARAMS
#endif

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

    uint32_t max_elements;          // the number of elements that can fit on this page

    struct {
        uint32_t used_elements;         // the number of used elements on this page
        uint32_t free_elements;         // the number of free elements on this page
    } aral_lock;

    struct {
        SPINLOCK spinlock;
        ARAL_FREE *list;
    } free;

    struct aral_page *prev; // the prev page on the list
    struct aral_page *next; // the next page on the list
} ARAL_PAGE;

struct aral {
    struct {
        char name[ARAL_MAX_NAME + 1];

        bool lockless;
        bool defragment;

        size_t element_size;            // calculated to take into account ARAL overheads
        size_t max_allocation_size;          // calculated in bytes
        size_t page_ptr_offset;         // calculated
        size_t natural_page_size;       // calculated

        size_t requested_element_size;
        size_t initial_page_elements;
        size_t max_page_elements;

        struct {
            bool enabled;
            const char *filename;
            char **cache_dir;
        } mmap;
    } config;

    struct {
        SPINLOCK spinlock;
        size_t file_number;             // for mmap
        size_t pages_version;
        size_t pages_version_last_checked;
        struct aral_page *pages;        // linked list of pages

        size_t user_malloc_operations;
        size_t user_free_operations;
        size_t defragment_operations;
        size_t defragment_linked_list_traversals;
    } aral_lock;

    struct {
        SPINLOCK spinlock;
        size_t allocation_size;         // current allocation size
    } adders;

    struct {
    } atomic;
};

struct {
    struct {
        struct {
            size_t allocations;
            size_t allocated;
        } structures;

        struct {
            size_t allocations;
            size_t allocated;
            size_t used;
        } malloc;

        struct {
            size_t allocations;
            size_t allocated;
            size_t used;
        } mmap;
    } atomic;
} aral_globals = {};

void aral_get_size_statistics(size_t *structures, size_t *malloc_allocated, size_t *malloc_used, size_t *mmap_allocated, size_t *mmap_used) {
    *structures = __atomic_load_n(&aral_globals.atomic.structures.allocated, __ATOMIC_RELAXED);
    *malloc_allocated = __atomic_load_n(&aral_globals.atomic.malloc.allocated, __ATOMIC_RELAXED);
    *malloc_used = __atomic_load_n(&aral_globals.atomic.malloc.used, __ATOMIC_RELAXED);
    *mmap_allocated = __atomic_load_n(&aral_globals.atomic.mmap.allocated, __ATOMIC_RELAXED);
    *mmap_used = __atomic_load_n(&aral_globals.atomic.mmap.used, __ATOMIC_RELAXED);
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
    if(likely(!ar->config.lockless))
        netdata_spinlock_lock(&ar->aral_lock.spinlock);
}

static inline void aral_unlock(ARAL *ar) {
    if(likely(!ar->config.lockless))
        netdata_spinlock_unlock(&ar->aral_lock.spinlock);
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

    for(page = ar->aral_lock.pages; page ; page = page->next) {
        if(unlikely(seeking >= (uintptr_t)page->data && seeking < (uintptr_t)page->data + page->size))
            break;
    }

    aral_unlock(ar);

    return page;
}
#endif

// ----------------------------------------------------------------------------
// find a page with a free slot (there shouldn't be any)

//static inline ARAL_PAGE *find_page_with_free_slots_internal_check(ARAL *ar) {
//    ARAL_PAGE *page;
//
//    for(page = ar->unsafe.pages; page ; page = page->next) {
//        if(page->free_list)
//            break;
//
//        internal_fatal(page->size - page->used_elements * ar->config.element_size >= ar->config.element_size,
//                       "ARAL: '%s' a page is marked full, but it is not!", ar->config.name);
//
//        internal_fatal(page->size < page->used_elements * ar->config.element_size,
//                       "ARAL: '%s' a page has been overflown!", ar->config.name);
//    }
//
//    return page;
//}

static ARAL_PAGE *aral_add_page___adders_lock_needed(ARAL *ar TRACE_ALLOCATIONS_FUNCTION_DEFINITION_PARAMS) {
    ARAL_PAGE *page = callocz(1, sizeof(ARAL_PAGE));
    netdata_spinlock_init(&page->free.spinlock);
    page->size = ar->adders.allocation_size;

    if(page->size > ar->config.max_allocation_size)
        page->size = ar->config.max_allocation_size;
    else
        ar->adders.allocation_size = aral_align_alloc_size(ar, (uint64_t)ar->adders.allocation_size * 4 / 3);

    page->max_elements = page->aral_lock.free_elements = page->size / ar->config.element_size;

    __atomic_add_fetch(&aral_globals.atomic.structures.allocations, 1, __ATOMIC_RELAXED);
    __atomic_add_fetch(&aral_globals.atomic.structures.allocated, sizeof(ARAL_PAGE), __ATOMIC_RELAXED);

    if(unlikely(ar->config.mmap.enabled)) {
        ar->aral_lock.file_number++;
        char filename[FILENAME_MAX + 1];
        snprintfz(filename, FILENAME_MAX, "%s/array_alloc.mmap/%s.%zu", *ar->config.mmap.cache_dir, ar->config.mmap.filename, ar->aral_lock.file_number);
        page->filename = strdupz(filename);
        page->data = netdata_mmap(page->filename, page->size, MAP_SHARED, 0, false, NULL);
        if (unlikely(!page->data))
            fatal("ARAL: '%s' cannot allocate aral buffer of size %zu on filename '%s'",
                  ar->config.name, page->size, page->filename);
        __atomic_add_fetch(&aral_globals.atomic.mmap.allocations, 1, __ATOMIC_RELAXED);
        __atomic_add_fetch(&aral_globals.atomic.mmap.allocated, page->size, __ATOMIC_RELAXED);
    }
    else {
#ifdef NETDATA_TRACE_ALLOCATIONS
        page->data = mallocz_int(page->size TRACE_ALLOCATIONS_FUNCTION_CALL_PARAMS);
#else
        page->data = mallocz(page->size);
#endif
        __atomic_add_fetch(&aral_globals.atomic.malloc.allocations, 1, __ATOMIC_RELAXED);
        __atomic_add_fetch(&aral_globals.atomic.malloc.allocated, page->size, __ATOMIC_RELAXED);
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

        __atomic_sub_fetch(&aral_globals.atomic.mmap.allocations, 1, __ATOMIC_RELAXED);
        __atomic_sub_fetch(&aral_globals.atomic.mmap.allocated, page->size, __ATOMIC_RELAXED);
    }
    else {
#ifdef NETDATA_TRACE_ALLOCATIONS
        freez_int(page->data TRACE_ALLOCATIONS_FUNCTION_CALL_PARAMS);
#else
        freez(page->data);
#endif
        __atomic_sub_fetch(&aral_globals.atomic.malloc.allocations, 1, __ATOMIC_RELAXED);
        __atomic_sub_fetch(&aral_globals.atomic.malloc.allocated, page->size, __ATOMIC_RELAXED);
    }

    freez(page);

    __atomic_sub_fetch(&aral_globals.atomic.structures.allocations, 1, __ATOMIC_RELAXED);
    __atomic_sub_fetch(&aral_globals.atomic.structures.allocated, sizeof(ARAL_PAGE), __ATOMIC_RELAXED);
}

static inline ARAL_PAGE *aral_acquire_a_free_slot(ARAL *ar TRACE_ALLOCATIONS_FUNCTION_DEFINITION_PARAMS) {
    aral_lock(ar);

    ARAL_PAGE *page = ar->aral_lock.pages;

    while(!page || !page->aral_lock.free_elements) {
        aral_unlock(ar);

        if(netdata_spinlock_trylock(&ar->adders.spinlock)) {
            page = aral_add_page___adders_lock_needed(ar TRACE_ALLOCATIONS_FUNCTION_CALL_PARAMS);

            aral_lock(ar);
            DOUBLE_LINKED_LIST_PREPEND_ITEM_UNSAFE(ar->aral_lock.pages, page, prev, next);
            ar->aral_lock.pages_version++;

            netdata_spinlock_unlock(&ar->adders.spinlock);
            break;
        }
        else {
            aral_lock(ar);
            page = ar->aral_lock.pages;
        }
    }

    // we have a page
    // and aral locked

    if (page->aral_lock.free_elements > 1 &&
        ar->aral_lock.pages_version_last_checked != ar->aral_lock.pages_version) {

        ARAL_PAGE *selected = page;
        ARAL_PAGE *next = page->next;
        size_t countdown = 3;

        while (next && next->aral_lock.free_elements && countdown-- > 0) {
            if (next->aral_lock.free_elements < page->aral_lock.free_elements)
                selected = next;

            next = next->next;
        }

        if(selected != page) {
            DOUBLE_LINKED_LIST_REMOVE_ITEM_UNSAFE(ar->aral_lock.pages, selected, prev, next);
            DOUBLE_LINKED_LIST_PREPEND_ITEM_UNSAFE(ar->aral_lock.pages, selected, prev, next);
            page = selected;
        }

        ar->aral_lock.pages_version_last_checked = ar->aral_lock.pages_version;
    }

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
        DOUBLE_LINKED_LIST_REMOVE_ITEM_UNSAFE(ar->aral_lock.pages, page, prev, next);
        DOUBLE_LINKED_LIST_APPEND_ITEM_UNSAFE(ar->aral_lock.pages, page, prev, next);
        ar->aral_lock.pages_version++;
    }

    aral_unlock(ar);

    return page;
}

void *aral_mallocz_internal(ARAL *ar TRACE_ALLOCATIONS_FUNCTION_DEFINITION_PARAMS) {

    ARAL_PAGE *page = aral_acquire_a_free_slot(ar TRACE_ALLOCATIONS_FUNCTION_CALL_PARAMS);

    netdata_spinlock_lock(&page->free.spinlock);

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

    netdata_spinlock_unlock(&page->free.spinlock);

    // put the page pointer after the element
    uint8_t *data = (uint8_t *)found_fr;
    ARAL_PAGE **page_ptr = (ARAL_PAGE **)&data[ar->config.page_ptr_offset];
    *page_ptr = page;

    if(unlikely(ar->config.mmap.enabled))
        __atomic_add_fetch(&aral_globals.atomic.mmap.used, ar->config.element_size, __ATOMIC_RELAXED);
    else
        __atomic_add_fetch(&aral_globals.atomic.malloc.used, ar->config.element_size, __ATOMIC_RELAXED);

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

static inline void aral_move_page_with_free_list___aral_lock_needed(ARAL *ar, ARAL_PAGE *page) {
    if(unlikely(page == ar->aral_lock.pages))
        // we are the first already
        return;

    ar->aral_lock.pages_version++;

    if (!ar->config.defragment ||
        page->aral_lock.free_elements == 1 ||
        page->aral_lock.free_elements <= ar->aral_lock.pages->aral_lock.free_elements) {
        // speed-up: move this page first
        DOUBLE_LINKED_LIST_REMOVE_ITEM_UNSAFE(ar->aral_lock.pages, page, prev, next);
        DOUBLE_LINKED_LIST_PREPEND_ITEM_UNSAFE(ar->aral_lock.pages, page, prev, next);
        return;
    }

    ar->aral_lock.pages_version_last_checked = ar->aral_lock.pages_version;

    ARAL_PAGE *tmp;

    int action = 0; (void)action;
    size_t move_later = 0, move_earlier = 0;

    for(tmp = page->next ;
        tmp && tmp->aral_lock.free_elements && tmp->aral_lock.free_elements < page->aral_lock.free_elements ;
        tmp = tmp->next)
        move_later++;

    if(!tmp && page->next) {
        DOUBLE_LINKED_LIST_REMOVE_ITEM_UNSAFE(ar->aral_lock.pages, page, prev, next);
        DOUBLE_LINKED_LIST_APPEND_ITEM_UNSAFE(ar->aral_lock.pages, page, prev, next);
        action = 1;
    }
    else if(tmp != page->next) {
        DOUBLE_LINKED_LIST_REMOVE_ITEM_UNSAFE(ar->aral_lock.pages, page, prev, next);
        DOUBLE_LINKED_LIST_INSERT_ITEM_BEFORE_UNSAFE(ar->aral_lock.pages, tmp, page, prev, next);
        action = 2;
    }
    else {
        for(tmp = (page == ar->aral_lock.pages) ? NULL : page->prev ;
            tmp && (!tmp->aral_lock.free_elements || tmp->aral_lock.free_elements > page->aral_lock.free_elements);
            tmp = (tmp == ar->aral_lock.pages) ? NULL : tmp->prev)
            move_earlier++;

        if(!tmp) {
            DOUBLE_LINKED_LIST_REMOVE_ITEM_UNSAFE(ar->aral_lock.pages, page, prev, next);
            DOUBLE_LINKED_LIST_PREPEND_ITEM_UNSAFE(ar->aral_lock.pages, page, prev, next);
            action = 3;
        }
        else if(tmp != page->prev){
            DOUBLE_LINKED_LIST_REMOVE_ITEM_UNSAFE(ar->aral_lock.pages, page, prev, next);
            DOUBLE_LINKED_LIST_INSERT_ITEM_AFTER_UNSAFE(ar->aral_lock.pages, tmp, page, prev, next);
            action = 4;
        }
    }

    ar->aral_lock.defragment_operations++;
    ar->aral_lock.defragment_linked_list_traversals += move_earlier + move_later;

    internal_fatal(page->next && page->next->aral_lock.free_elements && page->next->aral_lock.free_elements < page->aral_lock.free_elements,
                   "ARAL: '%s' item should be later in the list", ar->config.name);

    internal_fatal(page != ar->aral_lock.pages && (!page->prev->aral_lock.free_elements || page->prev->aral_lock.free_elements > page->aral_lock.free_elements),
                   "ARAL: '%s' item should be earlier in the list", ar->config.name);
}

void aral_freez_internal(ARAL *ar, void *ptr TRACE_ALLOCATIONS_FUNCTION_DEFINITION_PARAMS) {
    if(unlikely(!ptr)) return;

    // get the page pointer
    ARAL_PAGE *page = aral_ptr_to_page___must_NOT_have_aral_lock(ar, ptr);

    if(unlikely(ar->config.mmap.enabled))
        __atomic_sub_fetch(&aral_globals.atomic.mmap.used, ar->config.element_size, __ATOMIC_RELAXED);
    else
        __atomic_sub_fetch(&aral_globals.atomic.malloc.used, ar->config.element_size, __ATOMIC_RELAXED);

    // make this element available
    ARAL_FREE *fr = (ARAL_FREE *)ptr;
    fr->size = ar->config.element_size;

    netdata_spinlock_lock(&page->free.spinlock);
    fr->next = page->free.list;
    page->free.list = fr;
    netdata_spinlock_unlock(&page->free.spinlock);

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
        DOUBLE_LINKED_LIST_REMOVE_ITEM_UNSAFE(ar->aral_lock.pages, page, prev, next);
        ar->aral_lock.pages_version++;
        aral_unlock(ar);
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
        DOUBLE_LINKED_LIST_REMOVE_ITEM_UNSAFE(ar->aral_lock.pages, page, prev, next);
        aral_del_page___no_lock_needed(ar, page TRACE_ALLOCATIONS_FUNCTION_CALL_PARAMS);
    }

    aral_unlock(ar);
    freez(ar);
}

ARAL *aral_create(const char *name, size_t element_size, size_t initial_page_elements, size_t max_page_elements, const char *filename, char **cache_dir, bool mmap, bool lockless) {
    ARAL *ar = callocz(1, sizeof(ARAL));
    ar->config.requested_element_size = element_size;
    ar->config.initial_page_elements = initial_page_elements;
    ar->config.max_page_elements = max_page_elements;
    ar->config.mmap.filename = filename;
    ar->config.mmap.cache_dir = cache_dir;
    ar->config.mmap.enabled = mmap;
    ar->config.lockless = lockless;
    ar->config.defragment = false;
    strncpyz(ar->config.name, name, ARAL_MAX_NAME);
    netdata_spinlock_init(&ar->aral_lock.spinlock);

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
                   "max elements per page %zu (requested %zu), "
                   "max page size %zu bytes, "
                   , ar->config.name
                   , ar->config.element_size, ar->config.requested_element_size
                   , ar->adders.allocation_size / ar->config.element_size, ar->config.initial_page_elements
                   , ar->config.max_allocation_size / ar->config.element_size, ar->config.max_page_elements
                   , ar->config.max_allocation_size
    );

    __atomic_add_fetch(&aral_globals.atomic.structures.allocations, 1, __ATOMIC_RELAXED);
    __atomic_add_fetch(&aral_globals.atomic.structures.allocated, sizeof(ARAL), __ATOMIC_RELAXED);
    return ar;
}

// ----------------------------------------------------------------------------
// unittest

int aral_unittest(size_t elements) {
    char *cache_dir = "/tmp/";
    ARAL *ar = aral_create("aral-test", 20, 10, 1024, "test-aral", &cache_dir, false, false);

    void *pointers[elements];

    for(size_t i = 0; i < elements ;i++) {
        pointers[i] = aral_mallocz(ar);
    }

    for(size_t div = 5; div >= 2 ;div--) {
        for (size_t i = 0; i < elements / div; i++) {
            aral_freez(ar, pointers[i]);
        }

        for (size_t i = 0; i < elements / div; i++) {
            pointers[i] = aral_mallocz(ar);
        }
    }

    for(size_t step = 50; step >= 10 ;step -= 10) {
        for (size_t i = 0; i < elements; i += step) {
            aral_freez(ar, pointers[i]);
        }

        for (size_t i = 0; i < elements; i += step) {
            pointers[i] = aral_mallocz(ar);
        }
    }

    for(size_t i = 0; i < elements ;i++) {
        aral_freez(ar, pointers[i]);
    }

    if(ar->aral_lock.pages) {
        fprintf(stderr, "ARAL leftovers detected (1)");
        return 1;
    }

    size_t ops = 0; (void)ops;
    size_t increment = elements / 10;
    size_t allocated = 0;
    for(size_t all = increment; all <= elements ; all += increment) {

        for(; allocated < all ; allocated++) {
            pointers[allocated] = aral_mallocz(ar);
            ops++;
        }

        size_t to_free = now_realtime_usec() % all;
        size_t free_list[to_free];
        for(size_t i = 0; i < to_free ;i++) {
            size_t pos;
            do {
                pos = now_realtime_usec() % all;
            } while(!pointers[pos]);

            aral_freez(ar, pointers[pos]);
            pointers[pos] = NULL;
            free_list[i] = pos;
            ops++;
        }

        for(size_t i = 0; i < to_free ;i++) {
            size_t pos = free_list[i];
            pointers[pos] = aral_mallocz(ar);
            ops++;
        }
    }

    for(size_t i = 0; i < allocated - 1 ;i++) {
        aral_freez(ar, pointers[i]);
        ops++;
    }

    aral_freez(ar, pointers[allocated - 1]);

    if(ar->aral_lock.pages) {
        fprintf(stderr, "ARAL leftovers detected (2)");
        return 1;
    }

    return 0;
}
