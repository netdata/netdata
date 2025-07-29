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

// max mapped file size
#define ARAL_MAX_PAGE_SIZE_MMAP (1ULL * 1024 * 1024 * 1024)

// max malloc size
// optimal at current versions of libc is up to 256k
// ideal to have the same overhead as libc is 4k
#define ARAL_MAX_PAGE_SIZE_MALLOC (64ULL * 1024)

// in malloc mode, when the page is bigger than this
// use anonymous private mmap pages
#define ARAL_MALLOC_USE_MMAP_ABOVE (16ULL * 1024)

// do not allocate pages smaller than this
#define ARAL_MIN_PAGE_SIZE (16ULL * 1024)

#define ARAL_PAGE_INCOMING_PARTITIONS 4 // up to 32 (32-bits bitmap)

typedef struct aral_free {
    size_t size;
    struct aral_free *next;
} ARAL_FREE;

typedef struct aral_page {
    REFCOUNT refcount;

    const char *filename;
    uint8_t *data;

    bool started_marked;
    bool mapped;
    uint32_t size;                      // the allocation size of the page
    uint32_t max_elements;              // the number of elements that can fit on this page
    uint64_t elements_segmented;        // fast path for acquiring new elements in this page

    struct {
        bool marked;
        struct aral_page **head_ptr;
        struct aral_page *prev;         // the prev page on the list
        struct aral_page *next;         // the next page on the list
    } aral_lock;

    struct {
        SPINLOCK spinlock;
        uint32_t used_elements;         // the number of used elements on this page
        uint32_t free_elements;         // the number of free elements on this page
        uint32_t marked_elements;
        char pad[32];
    } page_lock;

    struct {
        SPINLOCK spinlock;
        ARAL_FREE *list;
        char pad[40];
    } available;

    struct {
        SPINLOCK spinlock;
        ARAL_FREE *list;
        char pad[48];
    } incoming[ARAL_PAGE_INCOMING_PARTITIONS];

    uint32_t incoming_partition_bitmap; // atomic

} ARAL_PAGE;

typedef enum {
    ARAL_LOCKLESS           = (1 << 0),
    ARAL_ALLOCATED_STATS    = (1 << 1),
    ARAL_DONT_DUMP          = (1 << 2),
} ARAL_OPTIONS;

struct aral_ops {
    struct {
        PAD64(size_t) allocators; // the number of threads currently trying to allocate memory
        PAD64(size_t) deallocators; // the number of threads currently trying to deallocate memory
        PAD64(bool) last_allocated_page; // stability detector, true when was last allocated
    } atomic;

    struct {
        SPINLOCK spinlock;
        size_t allocating_elements;     // currently allocating elements
        size_t allocation_size;         // current / next allocation size
    } adders;
};

struct aral {
    struct {
        SPINLOCK spinlock;
        size_t file_number;             // for mmap

        ARAL_PAGE *pages_free;          // pages with free items
        ARAL_PAGE *pages_full;          // pages that are completely full

        ARAL_PAGE *pages_marked_free;   // pages with marked items and free slots
        ARAL_PAGE *pages_marked_full;   // pages with marked items completely full
    } aral_lock;

    struct {
        char name[ARAL_MAX_NAME + 1];

        ARAL_OPTIONS options;

        size_t element_size;            // calculated to take into account ARAL overheads
        size_t element_ptr_offset;      // calculated
        size_t system_page_size;        // calculated

        size_t initial_page_elements;
        size_t requested_element_size;
        size_t requested_max_page_size;

        size_t min_required_page_size;

        struct {
            bool enabled;
            const char *filename;
            const char **cache_dir;
        } mmap;
    } config;

    struct {
        PAD64(size_t) user_malloc_operations;
        PAD64(size_t) user_free_operations;
    } atomic;

    struct aral_ops ops[2];

    struct aral_statistics *stats;
};

#define mark_to_idx(marked) (marked ? 1 : 0)
#define aral_pages_head_free(ar, marked) (marked ? &ar->aral_lock.pages_marked_free : &ar->aral_lock.pages_free)
#define aral_pages_head_full(ar, marked) (marked ? &ar->aral_lock.pages_marked_full : &ar->aral_lock.pages_full)

static size_t aral_max_allocation_size(ARAL *ar);

static inline bool aral_malloc_use_mmap(ARAL *ar __maybe_unused, size_t size) {
    unsigned long long mmap_limit = os_mmap_limit();

    if(mmap_limit > 256 * 1000 && size >= ARAL_MALLOC_USE_MMAP_ABOVE)
        return true;

    return false;
}

const char *aral_name(ARAL *ar) {
    return ar->config.name;
}

static ALWAYS_INLINE void aral_element_given(ARAL *ar, ARAL_PAGE *page) {
    if(ar->config.mmap.enabled || page->mapped)
        __atomic_add_fetch(&ar->stats->mmap.used_bytes, ar->config.requested_element_size, __ATOMIC_RELAXED);
    else
        __atomic_add_fetch(&ar->stats->malloc.used_bytes, ar->config.requested_element_size, __ATOMIC_RELAXED);
}

static ALWAYS_INLINE void aral_element_returned(ARAL *ar, ARAL_PAGE *page) {
    if(ar->config.mmap.enabled || page->mapped)
        __atomic_sub_fetch(&ar->stats->mmap.used_bytes, ar->config.requested_element_size, __ATOMIC_RELAXED);
    else
        __atomic_sub_fetch(&ar->stats->malloc.used_bytes, ar->config.requested_element_size, __ATOMIC_RELAXED);
}

size_t aral_structures_bytes_from_stats(struct aral_statistics *stats) {
    if(!stats) return 0;
    return __atomic_load_n(&stats->structures.allocated_bytes, __ATOMIC_RELAXED);
}

size_t aral_free_bytes_from_stats(struct aral_statistics *stats) {
    if(!stats) return 0;

    size_t allocated = __atomic_load_n(&stats->malloc.allocated_bytes, __ATOMIC_RELAXED) +
                       __atomic_load_n(&stats->mmap.allocated_bytes, __ATOMIC_RELAXED);

    size_t used = __atomic_load_n(&stats->malloc.used_bytes, __ATOMIC_RELAXED) +
                  __atomic_load_n(&stats->mmap.used_bytes, __ATOMIC_RELAXED);

    return (allocated > used) ? allocated - used : 0;
}

size_t aral_used_bytes_from_stats(struct aral_statistics *stats) {
    size_t used = __atomic_load_n(&stats->malloc.used_bytes, __ATOMIC_RELAXED) +
                  __atomic_load_n(&stats->mmap.used_bytes, __ATOMIC_RELAXED);
    return used;
}

size_t aral_padding_bytes_from_stats(struct aral_statistics *stats) {
    size_t padding = __atomic_load_n(&stats->malloc.padding_bytes, __ATOMIC_RELAXED) +
                     __atomic_load_n(&stats->mmap.padding_bytes, __ATOMIC_RELAXED);
    return padding;
}

size_t aral_used_bytes(ARAL *ar) {
    return aral_used_bytes_from_stats(ar->stats);
}

size_t aral_free_bytes(ARAL *ar) {
    return aral_free_bytes_from_stats(ar->stats);
}

size_t aral_structures_bytes(ARAL *ar) {
    return aral_structures_bytes_from_stats(ar->stats);
}

size_t aral_padding_bytes(ARAL *ar) {
    return aral_padding_bytes_from_stats(ar->stats);
}

size_t aral_free_structures_padding_from_stats(struct aral_statistics *stats) {
    return aral_free_bytes_from_stats(stats) + aral_structures_bytes_from_stats(stats) + aral_padding_bytes_from_stats(stats);
}

struct aral_statistics *aral_get_statistics(ARAL *ar) {
    return ar->stats;
}

static ALWAYS_INLINE void aral_lock_with_trace(ARAL *ar, const char *func) {
    if(likely(!(ar->config.options & ARAL_LOCKLESS)))
        spinlock_lock_with_trace(&ar->aral_lock.spinlock, func);
}

static ALWAYS_INLINE void aral_unlock_with_trace(ARAL *ar, const char *func) {
    if(likely(!(ar->config.options & ARAL_LOCKLESS)))
        spinlock_unlock_with_trace(&ar->aral_lock.spinlock, func);
}

#define aral_lock(ar) aral_lock_with_trace(ar, __FUNCTION__)
#define aral_unlock(ar) aral_unlock_with_trace(ar, __FUNCTION__)

static ALWAYS_INLINE void aral_page_lock(ARAL *ar, ARAL_PAGE *page) {
    if(likely(!(ar->config.options & ARAL_LOCKLESS)))
        spinlock_lock(&page->page_lock.spinlock);
}

static ALWAYS_INLINE void aral_page_unlock(ARAL *ar, ARAL_PAGE *page) {
    if(likely(!(ar->config.options & ARAL_LOCKLESS)))
        spinlock_unlock(&page->page_lock.spinlock);
}

static ALWAYS_INLINE void aral_page_available_lock(ARAL *ar, ARAL_PAGE *page) {
    if(likely(!(ar->config.options & ARAL_LOCKLESS)))
        spinlock_lock(&page->available.spinlock);
}

static ALWAYS_INLINE void aral_page_available_unlock(ARAL *ar, ARAL_PAGE *page) {
    if(likely(!(ar->config.options & ARAL_LOCKLESS)))
        spinlock_unlock(&page->available.spinlock);
}

static ALWAYS_INLINE bool aral_page_incoming_trylock(ARAL *ar, ARAL_PAGE *page, size_t partition) {
    if(likely(!(ar->config.options & ARAL_LOCKLESS)))
        return spinlock_trylock(&page->incoming[partition].spinlock);

    return true;
}

static ALWAYS_INLINE void aral_page_incoming_lock(ARAL *ar, ARAL_PAGE *page, size_t partition) {
    if(likely(!(ar->config.options & ARAL_LOCKLESS)))
        spinlock_lock(&page->incoming[partition].spinlock);
}

static ALWAYS_INLINE void aral_page_incoming_unlock(ARAL *ar, ARAL_PAGE *page, size_t partition) {
    if(likely(!(ar->config.options & ARAL_LOCKLESS)))
        spinlock_unlock(&page->incoming[partition].spinlock);
}

static ALWAYS_INLINE bool aral_adders_trylock(ARAL *ar, bool marked) {
    if(likely(!(ar->config.options & ARAL_LOCKLESS))) {
        size_t idx = mark_to_idx(marked);
        return spinlock_trylock(&ar->ops[idx].adders.spinlock);
    }

    return true;
}

static ALWAYS_INLINE void aral_adders_lock(ARAL *ar, bool marked) {
    if(likely(!(ar->config.options & ARAL_LOCKLESS))) {
        size_t idx = mark_to_idx(marked);
        spinlock_lock(&ar->ops[idx].adders.spinlock);
    }
}

static ALWAYS_INLINE void aral_adders_unlock(ARAL *ar, bool marked) {
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

// --------------------------------------------------------------------------------------------------------------------

#ifdef NETDATA_ARAL_INTERNAL_CHECKS
struct free_space {
    size_t pages;
    size_t pages_with_free_elements;
    size_t max_free_elements_on_a_page;
    size_t free_elements;
    size_t max_page_elements;
    ARAL_PAGE *p, *lp;
};

static inline struct free_space check_free_space___aral_lock_needed(ARAL *ar, ARAL_PAGE *my_page, bool marked) {
    struct free_space f = { 0 };

    f.max_page_elements = aral_max_allocation_size(ar) / ar->config.element_size;
    for(f.p = *aral_pages_head_free(ar, marked); f.p ; f.lp = f.p, f.p = f.p->aral_lock.next) {
        f.pages++;
        internal_fatal(!f.p->aral_lock.free_elements, "page is in the free list, but does not have any elements free");
        internal_fatal(f.p->marked != marked, "page is in the wrong mark list");

        if(f.p != my_page && f.max_free_elements_on_a_page < f.p->aral_lock.free_elements)
            f.max_free_elements_on_a_page = f.p->aral_lock.free_elements;

        f.free_elements += f.p->aral_lock.free_elements;
        f.pages_with_free_elements++;
    }

    for(f.p = *aral_pages_head_full(ar, marked); f.p ; f.lp = f.p, f.p = f.p->aral_lock.next) {
        f.pages++;
        internal_fatal(f.p->aral_lock.free_elements, "found page with free items in a full page");
        internal_fatal(f.p->marked != marked, "page is in the wrong mark list");
    }

    return f;
}
static inline bool is_page_in_list(ARAL_PAGE *head, ARAL_PAGE  *page) {
    for(ARAL_PAGE *p = head; p ; p = p->aral_lock.next)
        if(p == page) return true;
    return false;
}
#else

#define is_page_in_list(head, page) true

#endif


// --------------------------------------------------------------------------------------------------------------------
// find the page a pointer belongs to

#ifdef NETDATA_ARAL_INTERNAL_CHECKS
static inline ARAL_PAGE *find_page_with_allocation_internal_check(ARAL *ar, void *ptr, bool marked) {
    aral_lock(ar);

    uintptr_t seeking = (uintptr_t)ptr;
    ARAL_PAGE *page;

    for (page = *aral_pages_head_full(ar, marked); page; page = page->aral_lock.next) {
        if (unlikely(seeking >= (uintptr_t)page->data && seeking < (uintptr_t)page->data + page->size))
            break;
    }

    if(!page) {
        for(page = *aral_pages_head_free(ar, marked); page ; page = page->aral_lock.next) {
            if(unlikely(seeking >= (uintptr_t)page->data && seeking < (uintptr_t)page->data + page->size))
                break;
        }
    }

    aral_unlock(ar);

    return page;
}
#endif

// --------------------------------------------------------------------------------------------------------------------
// Tagging the pointer with the 'marked' flag

// Retrieving the pointer and the 'marked' flag
static ALWAYS_INLINE ARAL_PAGE *aral_get_page_pointer_after_element___do_NOT_have_aral_lock(ARAL *ar, void *ptr, bool *marked) {
    uint8_t *data = ptr;
    uintptr_t *page_ptr = (uintptr_t *)&data[ar->config.element_ptr_offset];
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

    internal_fatal((uintptr_t)page % SYSTEM_REQUIRED_ALIGNMENT != 0, "Pointer is not aligned properly");

    return page;
}

static ALWAYS_INLINE void aral_set_page_pointer_after_element___do_NOT_have_aral_lock(ARAL *ar, void *page, void *ptr, bool marked) {
    uint8_t *data = ptr;
    uintptr_t *page_ptr = (uintptr_t *)&data[ar->config.element_ptr_offset];
    uintptr_t tagged_page = (uintptr_t)page;  // Cast the pointer to an integer
    if (marked) tagged_page |= 1; // Set the LSB to 1 if 'marked' is true
    __atomic_store_n(page_ptr, tagged_page, __ATOMIC_RELEASE);  // Atomically store the tagged pointer
}

// --------------------------------------------------------------------------------------------------------------------
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

// --------------------------------------------------------------------------------------------------------------------
// page size management

static ALWAYS_INLINE size_t aral_element_slot_size(size_t requested_element_size, bool usable) {
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

static ALWAYS_INLINE size_t aral_elements_in_page_size(ARAL *ar, size_t page_size) {
    if(ar->config.mmap.enabled)
        return page_size / ar->config.element_size;

    size_t aral_page_size = memory_alignment(sizeof(ARAL_PAGE), SYSTEM_REQUIRED_ALIGNMENT);
    size_t remaining = page_size - aral_page_size;
    return remaining / ar->config.element_size;
}

static ALWAYS_INLINE size_t aral_next_allocation_size___adders_lock_needed(ARAL *ar, bool marked) {
    size_t idx = mark_to_idx(marked);
    size_t size = ar->ops[idx].adders.allocation_size;

    bool last_allocated_page = __atomic_load_n(&ar->ops[idx].atomic.last_allocated_page, __ATOMIC_RELAXED);
    if(last_allocated_page) {
        // we are growing, double the size

        size *= 2;

        size_t max = aral_max_allocation_size(ar);
        if(size > max)
            size = max;
        ar->ops[idx].adders.allocation_size = size;
    }

    if(!ar->config.mmap.enabled && aral_malloc_use_mmap(ar, size)) {
        // when doing malloc, don't allocate entire pages, but only what needed
        size =
            aral_elements_in_page_size(ar, size) * ar->config.element_size +
            memory_alignment(sizeof(ARAL_PAGE), SYSTEM_REQUIRED_ALIGNMENT);
    }

    __atomic_store_n(&ar->ops[idx].atomic.last_allocated_page, true, __ATOMIC_RELAXED);

    return size;
}

// --------------------------------------------------------------------------------------------------------------------

static ARAL_PAGE *aral_create_page___no_lock_needed(ARAL *ar, size_t size TRACE_ALLOCATIONS_FUNCTION_DEFINITION_PARAMS) {
    struct aral_page_type_stats *stats;
    ARAL_PAGE *page;

    size_t total_size = size;

    if(ar->config.mmap.enabled) {
        page = callocz(1, sizeof(ARAL_PAGE));
        ar->aral_lock.file_number++;

        char filename[FILENAME_MAX + 1];
        snprintfz(filename, FILENAME_MAX, "%s/array_alloc.mmap/%s.%zu", *ar->config.mmap.cache_dir, ar->config.mmap.filename, ar->aral_lock.file_number);
        page->filename = strdupz(filename);
        page->mapped = true;

        page->data =
            nd_mmap_advanced(page->filename, size, MAP_SHARED, 0, false, ar->config.options & ARAL_DONT_DUMP, NULL);
        if (unlikely(!page->data))
            out_of_memory(__FUNCTION__, size, page->filename);

        total_size = size + sizeof(ARAL_PAGE);
        stats = &ar->stats->mmap;
    }
#ifdef NETDATA_TRACE_ALLOCATIONS
    else {
        page = callocz(1, sizeof(ARAL_PAGE));
        page->data = mallocz_int(size TRACE_ALLOCATIONS_FUNCTION_CALL_PARAMS);
        page->mapped = false;
        __atomic_add_fetch(&ar->stats->malloc.allocations, 1, __ATOMIC_RELAXED);
        __atomic_add_fetch(&ar->stats->malloc.allocated_bytes, size, __ATOMIC_RELAXED);
    }
#else
    else {
        size_t ARAL_PAGE_size = memory_alignment(sizeof(ARAL_PAGE), SYSTEM_REQUIRED_ALIGNMENT);

        if (aral_malloc_use_mmap(ar, size)) {
            bool mapped;
            uint8_t *ptr =
                nd_mmap_advanced(NULL, size, MAP_ANONYMOUS | MAP_PRIVATE, 1, false, ar->config.options & ARAL_DONT_DUMP, NULL);
            if (ptr) {
                mapped = true;
                stats = &ar->stats->mmap;
            }
            else {
                ptr = mallocz(size);
                mapped = false;
                stats = &ar->stats->malloc;
            }
            page = (ARAL_PAGE *)ptr;
            memset(page, 0, ARAL_PAGE_size);
            page->data = &ptr[ARAL_PAGE_size];
            page->mapped = mapped;
        }
        else {
            uint8_t *ptr = mallocz(size);
            page = (ARAL_PAGE *)ptr;
            memset(page, 0, ARAL_PAGE_size);
            page->data = &ptr[ARAL_PAGE_size];
            page->mapped = false;

            stats = &ar->stats->malloc;
        }
    }
#endif

    spinlock_init(&page->available.spinlock);

    for(size_t p = 0; p < ARAL_PAGE_INCOMING_PARTITIONS ;p++)
        spinlock_init(&page->incoming[p].spinlock);

    page->size = size;
    page->max_elements = aral_elements_in_page_size(ar, page->size);
    page->page_lock.free_elements = page->max_elements;
    spinlock_init(&page->page_lock.spinlock);
    page->refcount = 1;

    size_t structures_size = sizeof(ARAL_PAGE) + page->max_elements * sizeof(void *);
    size_t data_size = page->max_elements * ar->config.requested_element_size;
    size_t padding_size = total_size - data_size - structures_size;

    __atomic_add_fetch(&stats->allocations, 1, __ATOMIC_RELAXED);
    __atomic_add_fetch(&stats->allocated_bytes, data_size, __ATOMIC_RELAXED);
    __atomic_add_fetch(&stats->padding_bytes, padding_size, __ATOMIC_RELAXED);

    __atomic_add_fetch(&ar->stats->structures.allocations, 1, __ATOMIC_RELAXED);
    __atomic_add_fetch(&ar->stats->structures.allocated_bytes, structures_size, __ATOMIC_RELAXED);

    // Initialize elements_segmented last with RELEASE
    __atomic_store_n(&page->elements_segmented, 0, __ATOMIC_RELEASE);

    return page;
}

static void aral_del_page___no_lock_needed(ARAL *ar, ARAL_PAGE *page TRACE_ALLOCATIONS_FUNCTION_DEFINITION_PARAMS) {
    size_t idx = mark_to_idx(page->started_marked);
    __atomic_store_n(&ar->ops[idx].atomic.last_allocated_page, false, __ATOMIC_RELAXED);

    struct aral_page_type_stats *stats;
    size_t max_elements = page->max_elements;
    size_t size = page->size;
    size_t total_size = size;

    // free it
    if (ar->config.mmap.enabled) {
        stats = &ar->stats->mmap;
        total_size = size + sizeof(ARAL_PAGE);

        nd_munmap(page->data, page->size);

        if (unlikely(unlink(page->filename) == 1))
            netdata_log_error("Cannot delete file '%s'", page->filename);

        freez((void *)page->filename);
        freez(page);
    }
    else {
#ifdef NETDATA_TRACE_ALLOCATIONS
        __atomic_sub_fetch(&ar->stats->malloc.allocations, 1, __ATOMIC_RELAXED);
        __atomic_sub_fetch(&ar->stats->malloc.allocated_bytes, page->size - sizeof(ARAL_PAGE), __ATOMIC_RELAXED);

        freez_int(page->data TRACE_ALLOCATIONS_FUNCTION_CALL_PARAMS);
        freez(page);
#else
        if(page->mapped) {
            stats = &ar->stats->mmap;
            nd_munmap(page, page->size);
        }
        else {
            stats = &ar->stats->malloc;
            freez(page);
        }
#endif
    }

    size_t structures_size = sizeof(ARAL_PAGE) + max_elements * sizeof(void *);
    size_t data_size = max_elements * ar->config.requested_element_size;
    size_t padding_size = total_size - data_size - structures_size;

    __atomic_sub_fetch(&stats->allocations, 1, __ATOMIC_RELAXED);
    __atomic_sub_fetch(&stats->allocated_bytes, data_size, __ATOMIC_RELAXED);
    __atomic_sub_fetch(&stats->padding_bytes, padding_size, __ATOMIC_RELAXED);

    __atomic_sub_fetch(&ar->stats->structures.allocations, 1, __ATOMIC_RELAXED);
    __atomic_sub_fetch(&ar->stats->structures.allocated_bytes, structures_size, __ATOMIC_RELAXED);
}

ALWAYS_INLINE WARNUNUSED
static bool aral_page_acquire(ARAL_PAGE *page) {
    REFCOUNT rf = __atomic_add_fetch(&page->refcount, 1, __ATOMIC_ACQUIRE);
    if(rf <= 0)
        return false;

    if(rf > (REFCOUNT)page->max_elements) {
        __atomic_sub_fetch(&page->refcount, 1, __ATOMIC_RELAXED);
        return false;
    }

    return true;
}

ALWAYS_INLINE WARNUNUSED
static ARAL_PAGE *aral_acquire_first_page(ARAL *ar, bool marked) {
    aral_lock(ar);

    ARAL_PAGE **head_ptr_free = aral_pages_head_free(ar, marked);
    ARAL_PAGE *page = *head_ptr_free;

    if(page && !aral_page_acquire(page))
        page = NULL;

    aral_unlock(ar);
    return page;
}

ALWAYS_INLINE WARNUNUSED
static bool aral_page_release(ARAL_PAGE *page) {
    REFCOUNT rf = __atomic_sub_fetch(&page->refcount, 1, __ATOMIC_RELEASE);
    if(rf == 0) {
        REFCOUNT expected = rf;
        REFCOUNT desired = REFCOUNT_DELETED;
        if (__atomic_compare_exchange_n(&page->refcount, &expected, desired, false, __ATOMIC_ACQUIRE, __ATOMIC_RELAXED))
            return true;
    }

    return false;
}

static ALWAYS_INLINE ARAL_PAGE *aral_get_first_page_with_a_free_slot(ARAL *ar, bool marked TRACE_ALLOCATIONS_FUNCTION_DEFINITION_PARAMS) {
    size_t idx = mark_to_idx(marked);
    __atomic_add_fetch(&ar->ops[idx].atomic.allocators, 1, __ATOMIC_RELAXED);

#ifdef NETDATA_ARAL_INTERNAL_CHECKS
    // bool added = false;
    struct free_space f1, f2;
#endif

    ARAL_PAGE *page;
    while(!(page = aral_acquire_first_page(ar, marked))) {
#ifdef NETDATA_ARAL_INTERNAL_CHECKS
        f1 = check_free_space___aral_lock_needed(ar, NULL, marked);
#endif

        bool can_add = false;
        size_t page_allocation_size = 0;
        if(aral_adders_trylock(ar, marked)) {
            // we can add a page - let's see it is really needed
            size_t threads_currently_allocating = __atomic_load_n(&ar->ops[idx].atomic.allocators, __ATOMIC_RELAXED);
            size_t threads_currently_deallocating = __atomic_load_n(&ar->ops[idx].atomic.deallocators, __ATOMIC_RELAXED);

            // we will allocate a page, only if the number of elements required is more than the
            // sum of all new allocations under their way plus the pages currently being deallocated
            if(ar->ops[idx].adders.allocating_elements + threads_currently_deallocating < threads_currently_allocating) {
                can_add = true;
                page_allocation_size = aral_next_allocation_size___adders_lock_needed(ar, marked);
                ar->ops[idx].adders.allocating_elements += aral_elements_in_page_size(ar, page_allocation_size);
            }
            aral_adders_unlock(ar, marked);
        }

        if(can_add) {
            page = aral_create_page___no_lock_needed(ar, page_allocation_size TRACE_ALLOCATIONS_FUNCTION_CALL_PARAMS);
            page->aral_lock.marked = page->started_marked = marked;

            ARAL_PAGE **head_ptr_free = aral_pages_head_free(ar, marked);
            aral_lock(ar);
            DOUBLE_LINKED_LIST_APPEND_ITEM_UNSAFE(*head_ptr_free, page, aral_lock.prev, aral_lock.next);
            page->aral_lock.head_ptr = head_ptr_free;
            aral_unlock(ar);

            //#ifdef NETDATA_ARAL_INTERNAL_CHECKS
            //            added = true;
            //#endif

            aral_adders_lock(ar, marked);
            ar->ops[idx].adders.allocating_elements -= aral_elements_in_page_size(ar, page_allocation_size);
            aral_adders_unlock(ar, marked);

            // we have a page that is all empty
            // break the loop
            break;
        }
        else {
            // let the adders/deallocators do it
            sched_yield();
            tinysleep();
        }
    }

    // we have a page
    // it is acquired
    // and aral is NOT locked

    //#ifdef NETDATA_ARAL_INTERNAL_CHECKS
    //    if(added) {
    //        f2 = check_free_space___aral_lock_needed(ar, page, marked);
    //        internal_fatal(f2.failed, "hey!");
    //    }
    //#endif

    internal_fatal(!page,
                   "ARAL: '%s' failed to find a page with a free element",
                   ar->config.name);

    aral_page_lock(ar, page);

    internal_fatal(!page->page_lock.free_elements,
                   "ARAL: '%s' selected page does not have a free slot in it",
                   ar->config.name);

    internal_fatal(page->max_elements != page->page_lock.used_elements + page->page_lock.free_elements,
                   "ARAL: '%s' page element counters do not match, "
                   "page says it can handle %zu elements, "
                   "but there are %zu used and %zu free items, "
                   "total %zu items",
                   ar->config.name,
                   (size_t)page->max_elements,
                   (size_t)page->page_lock.used_elements, (size_t)page->page_lock.free_elements,
                   (size_t)page->page_lock.used_elements + (size_t)page->page_lock.free_elements);

    internal_fatal(page->page_lock.marked_elements > page->page_lock.used_elements,
                   "page has more marked elements than the used ones");

    page->page_lock.used_elements++;
    page->page_lock.free_elements--;

    if(marked)
        page->page_lock.marked_elements++;

    if(unlikely(page->page_lock.used_elements == page->max_elements)) {
        aral_lock(ar);
        ARAL_PAGE **head_ptr_full = aral_pages_head_full(ar, marked);
        internal_fatal(!is_page_in_list(*page->aral_lock.head_ptr, page), "Page is not in this list");
        DOUBLE_LINKED_LIST_REMOVE_ITEM_UNSAFE(*page->aral_lock.head_ptr, page, aral_lock.prev, aral_lock.next);
        DOUBLE_LINKED_LIST_APPEND_ITEM_UNSAFE(*head_ptr_full, page, aral_lock.prev, aral_lock.next);
        page->aral_lock.head_ptr = head_ptr_full;
        aral_unlock(ar);
    }

    aral_page_unlock(ar, page);

    __atomic_sub_fetch(&ar->ops[idx].atomic.allocators, 1, __ATOMIC_RELAXED);
    __atomic_add_fetch(&ar->atomic.user_malloc_operations, 1, __ATOMIC_RELAXED);

    return page;
}

static ALWAYS_INLINE void *aral_get_free_slot___no_lock_required(ARAL *ar, ARAL_PAGE *page, bool marked) {
    // Try fast path first
    uint64_t slot = __atomic_fetch_add(&page->elements_segmented, 1, __ATOMIC_ACQUIRE);
    if (slot < page->max_elements) {
        // Fast path - we got a valid slot
        uint8_t *data = page->data + (slot * ar->config.element_size);

        // Set the page pointer after the element
        aral_set_page_pointer_after_element___do_NOT_have_aral_lock(ar, page, data, marked);

        aral_element_given(ar, page);
        return data;
    }

    // Fall back to existing mechanism for reused memory
    aral_page_available_lock(ar, page);

    while(!page->available.list) {
        uint32_t bitmap = __atomic_load_n(&page->incoming_partition_bitmap, __ATOMIC_RELAXED);
        if (!bitmap)
            fatal("ARAL: bitmap of incoming free elements cannot be empty at this point");

        while(bitmap) {
            size_t partition = __builtin_ffs((int)bitmap) - 1;
            //        for(partition = 0; partition < ARAL_PAGE_INCOMING_PARTITIONS ; partition++) {
            //            if (bitmap & (1U << partition))
            //                break;
            //        }

            if (partition >= ARAL_PAGE_INCOMING_PARTITIONS)
                fatal("ARAL: partition %zu must be smaller than %d", partition, ARAL_PAGE_INCOMING_PARTITIONS);

            if (aral_page_incoming_trylock(ar, page, partition)) {
                page->available.list = page->incoming[partition].list;
                page->incoming[partition].list = NULL;
                __atomic_fetch_and(&page->incoming_partition_bitmap, ~(1U << partition), __ATOMIC_RELAXED);
                aral_page_incoming_unlock(ar, page, partition);
                break;
            }
            else
                bitmap &= ~(1U << partition);
        }
    }

    ARAL_FREE *found_fr = page->available.list;
    internal_fatal(!found_fr, "ARAL: '%s' incoming free list, cannot be NULL.", ar->config.name);
    page->available.list = found_fr->next;

    aral_page_available_unlock(ar, page);

    // Set the page pointer after the element
    aral_set_page_pointer_after_element___do_NOT_have_aral_lock(ar, page, found_fr, marked);

    aral_element_given(ar, page);

    return found_fr;
}

static inline void aral_add_free_slot___no_lock_required(ARAL *ar, ARAL_PAGE *page, void *ptr) {
    ARAL_FREE *fr = (ARAL_FREE *)ptr;
    fr->size = ar->config.element_size;

    // use the slot id of the item to be freed to determine the partition number
    size_t start = (((uint8_t *)ptr - page->data) / ar->config.element_size) % ARAL_PAGE_INCOMING_PARTITIONS;

    while (true) {
        for (size_t partition = start; partition < ARAL_PAGE_INCOMING_PARTITIONS; partition++) {
            if (aral_page_incoming_trylock(ar, page, partition)) {
                fr->next = page->incoming[partition].list;
                page->incoming[partition].list = fr;
                __atomic_fetch_or(&page->incoming_partition_bitmap, 1U << partition, __ATOMIC_RELAXED);
                aral_page_incoming_unlock(ar, page, partition);
                return;
            }
        }

        start = 0;
    }
}

ALWAYS_INLINE void *aral_callocz_internal(ARAL *ar, bool marked TRACE_ALLOCATIONS_FUNCTION_DEFINITION_PARAMS) {
    void *r = aral_mallocz_internal(ar, marked TRACE_ALLOCATIONS_FUNCTION_CALL_PARAMS);
    memset(r, 0, ar->config.requested_element_size);
    return r;
}

void *aral_mallocz_internal(ARAL *ar, bool marked TRACE_ALLOCATIONS_FUNCTION_DEFINITION_PARAMS) {
#if defined(FSANITIZE_ADDRESS)
    if(ar->stats) {
        __atomic_add_fetch(&ar->stats->malloc.allocations, 1, __ATOMIC_RELAXED);
        __atomic_add_fetch(&ar->stats->malloc.allocated_bytes, ar->config.requested_element_size, __ATOMIC_RELAXED);
        __atomic_add_fetch(&ar->stats->malloc.used_bytes, ar->config.requested_element_size, __ATOMIC_RELAXED);
    }
    return mallocz(ar->config.requested_element_size);
#endif

    // reserve a slot on a free page
    ARAL_PAGE *page = aral_get_first_page_with_a_free_slot(ar, marked TRACE_ALLOCATIONS_FUNCTION_CALL_PARAMS);
    // the page returned has reserved a slot for us

    void *data = aral_get_free_slot___no_lock_required(ar, page, marked);

    internal_fatal((uintptr_t)data % SYSTEM_REQUIRED_ALIGNMENT != 0, "Pointer is not aligned properly");

    return data;
}

void aral_unmark_allocation(ARAL *ar, void *ptr) {
#if defined(FSANITIZE_ADDRESS)
    return;
#endif

    if(unlikely(!ptr)) return;

    // get the page pointer
    bool marked;
    ARAL_PAGE *page = aral_get_page_pointer_after_element___do_NOT_have_aral_lock(ar, ptr, &marked);

    internal_fatal(!marked, "This allocation does is not marked");

    if(marked)
        aral_set_page_pointer_after_element___do_NOT_have_aral_lock(ar, page, ptr, false);

    aral_page_lock(ar, page);
    internal_fatal(marked && !page->page_lock.marked_elements, "Marked counter going negative.");
    bool unmark = marked && --page->page_lock.marked_elements == 0 && page->page_lock.used_elements;

    if(unmark) {
        aral_lock(ar);
        internal_fatal(!is_page_in_list(*page->aral_lock.head_ptr, page), "Page is not in this list");

        ARAL_PAGE **head_ptr_to = (page->page_lock.free_elements) ? aral_pages_head_free(ar, false) : aral_pages_head_full(ar, false);
        if(page->aral_lock.head_ptr != head_ptr_to) {
            internal_fatal(!is_page_in_list(*page->aral_lock.head_ptr, page), "Page is not in this list");
            DOUBLE_LINKED_LIST_REMOVE_ITEM_UNSAFE(*page->aral_lock.head_ptr, page, aral_lock.prev, aral_lock.next);
            DOUBLE_LINKED_LIST_APPEND_ITEM_UNSAFE(*head_ptr_to, page, aral_lock.prev, aral_lock.next);
            page->aral_lock.head_ptr = head_ptr_to;
            page->aral_lock.marked = false;
        }

        internal_fatal(page->page_lock.marked_elements > page->page_lock.used_elements,
                       "page has more marked elements than the used ones");
        aral_unlock(ar);
    }

    aral_page_unlock(ar, page);
}

void aral_freez_internal(ARAL *ar, void *ptr TRACE_ALLOCATIONS_FUNCTION_DEFINITION_PARAMS) {
#if defined(FSANITIZE_ADDRESS)
    if(ptr && ar->stats) {
        __atomic_sub_fetch(&ar->stats->malloc.allocations, 1, __ATOMIC_RELAXED);
        __atomic_sub_fetch(&ar->stats->malloc.allocated_bytes, ar->config.requested_element_size, __ATOMIC_RELAXED);
        __atomic_sub_fetch(&ar->stats->malloc.used_bytes, ar->config.requested_element_size, __ATOMIC_RELAXED);
    }
    freez(ptr);
    return;
#endif

    if(unlikely(!ptr)) return;

    // get the page pointer
    bool marked;
    ARAL_PAGE *page = aral_get_page_pointer_after_element___do_NOT_have_aral_lock(ar, ptr, &marked);

    size_t idx = mark_to_idx(marked);
    __atomic_add_fetch(&ar->ops[idx].atomic.deallocators, 1, __ATOMIC_RELAXED);

    // make this element available
    aral_add_free_slot___no_lock_required(ar, page, ptr);

    // statistic, outside the lock
    aral_element_returned(ar, page);
    __atomic_add_fetch(&ar->atomic.user_free_operations, 1, __ATOMIC_RELAXED);

    aral_page_lock(ar, page);
    internal_fatal(!page->page_lock.used_elements,
                   "ARAL: '%s' pointer %p is inside a page without any active allocations.",
                   ar->config.name, ptr);

    internal_fatal(page->max_elements != page->page_lock.used_elements + page->page_lock.free_elements,
                   "ARAL: '%s' page element counters do not match, "
                   "page says it can handle %zu elements, "
                   "but there are %zu used and %zu free items, "
                   "total %zu items",
                   ar->config.name,
                   (size_t)page->max_elements,
                   (size_t)page->page_lock.used_elements, (size_t)page->page_lock.free_elements,
                   (size_t)page->page_lock.used_elements + (size_t)page->page_lock.free_elements
    );

    page->page_lock.used_elements--;
    page->page_lock.free_elements++;

    internal_fatal(marked && !page->page_lock.marked_elements, "Marked counter going negative.");
    bool unmark = marked && --page->page_lock.marked_elements == 0 && page->page_lock.used_elements;

    internal_fatal(page->max_elements != page->page_lock.used_elements + page->page_lock.free_elements,
                   "ARAL: '%s' page element counters do not match, "
                   "page says it can handle %zu elements, "
                   "but there are %zu used and %zu free items, "
                   "total %zu items",
                   ar->config.name,
                   (size_t)page->max_elements,
                   (size_t)page->page_lock.used_elements, (size_t)page->page_lock.free_elements,
                   (size_t)page->page_lock.used_elements + (size_t)page->page_lock.free_elements);

    internal_fatal(page->page_lock.marked_elements > page->page_lock.used_elements,
                   "page has more marked elements than the used ones");

    // release it
    if(unlikely(aral_page_release(page))) {
        internal_fatal(page->page_lock.used_elements, "page has used elements but has been acquired for deletion");
        internal_fatal(page->page_lock.marked_elements, "page has marked elements but not used ones");

        aral_lock(ar);
        internal_fatal(!is_page_in_list(*page->aral_lock.head_ptr, page), "Page is not in this list");

        if(*page->aral_lock.head_ptr != page || page->aral_lock.prev != page || page->aral_lock.next != NULL) {
            // there are more pages with free items  - delete it
            DOUBLE_LINKED_LIST_REMOVE_ITEM_UNSAFE(*page->aral_lock.head_ptr, page, aral_lock.prev, aral_lock.next);
            aral_unlock(ar);
            aral_page_unlock(ar, page);
            __atomic_sub_fetch(&ar->ops[idx].atomic.deallocators, 1, __ATOMIC_RELAXED);
            aral_del_page___no_lock_needed(ar, page TRACE_ALLOCATIONS_FUNCTION_CALL_PARAMS);
            return;
        }

        // this is the last page with free items - keep it
        page->available.list = NULL;
        page->incoming_partition_bitmap = 0;
        for(size_t p = 0; p < ARAL_PAGE_INCOMING_PARTITIONS; p++)
            page->incoming[p].list = NULL;

        __atomic_store_n(&page->elements_segmented, 0, __ATOMIC_RELEASE);
        __atomic_store_n(&page->refcount, 0, __ATOMIC_RELAXED);
        aral_unlock(ar);
    }
    else if(unlikely(unmark)) {
        aral_lock(ar);

        ARAL_PAGE **head_ptr_to = aral_pages_head_free(ar, false);
        if(page->aral_lock.head_ptr != head_ptr_to) {
            internal_fatal(!is_page_in_list(*page->aral_lock.head_ptr, page), "Page is not in this list");
            DOUBLE_LINKED_LIST_REMOVE_ITEM_UNSAFE(*page->aral_lock.head_ptr, page, aral_lock.prev, aral_lock.next);
            DOUBLE_LINKED_LIST_APPEND_ITEM_UNSAFE(*head_ptr_to, page, aral_lock.prev, aral_lock.next);
            page->aral_lock.head_ptr = head_ptr_to;
            page->aral_lock.marked = false;
        }
        aral_unlock(ar);
    }
    else if(unlikely(page->page_lock.used_elements == page->max_elements - 1)) {
        aral_lock(ar);
        ARAL_PAGE **head_ptr_to = aral_pages_head_free(ar, page->aral_lock.marked);
        if(page->aral_lock.head_ptr != head_ptr_to) {
            internal_fatal(!is_page_in_list(*page->aral_lock.head_ptr, page), "Page is not in this list");
            DOUBLE_LINKED_LIST_REMOVE_ITEM_UNSAFE(*page->aral_lock.head_ptr, page, aral_lock.prev, aral_lock.next);
            DOUBLE_LINKED_LIST_APPEND_ITEM_UNSAFE(*head_ptr_to, page, aral_lock.prev, aral_lock.next);
            page->aral_lock.head_ptr = head_ptr_to;
        }
        aral_unlock(ar);
    }

    aral_page_unlock(ar, page);
    __atomic_sub_fetch(&ar->ops[idx].atomic.deallocators, 1, __ATOMIC_RELAXED);
}

void aral_destroy_internal(ARAL *ar TRACE_ALLOCATIONS_FUNCTION_DEFINITION_PARAMS) {
    aral_lock(ar);

    ARAL_PAGE **head_ptr = aral_pages_head_free(ar, false);
    ARAL_PAGE *page;
    while((page = *head_ptr)) {
        DOUBLE_LINKED_LIST_REMOVE_ITEM_UNSAFE(*head_ptr, page, aral_lock.prev, aral_lock.next);
        aral_del_page___no_lock_needed(ar, page TRACE_ALLOCATIONS_FUNCTION_CALL_PARAMS);
    }

    head_ptr = aral_pages_head_free(ar, true);
    while((page = *head_ptr)) {
        DOUBLE_LINKED_LIST_REMOVE_ITEM_UNSAFE(*head_ptr, page, aral_lock.prev, aral_lock.next);
        aral_del_page___no_lock_needed(ar, page TRACE_ALLOCATIONS_FUNCTION_CALL_PARAMS);
    }

    head_ptr = aral_pages_head_full(ar, false);
    while((page = *head_ptr)) {
        DOUBLE_LINKED_LIST_REMOVE_ITEM_UNSAFE(*head_ptr, page, aral_lock.prev, aral_lock.next);
        aral_del_page___no_lock_needed(ar, page TRACE_ALLOCATIONS_FUNCTION_CALL_PARAMS);
    }

    head_ptr = aral_pages_head_full(ar, true);
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

static size_t aral_max_page_size_malloc = ARAL_MAX_PAGE_SIZE_MALLOC;
size_t aral_optimal_malloc_page_size(void) {
    return aral_max_page_size_malloc;
}

void aral_optimal_malloc_page_size_set(size_t size) {
    aral_max_page_size_malloc = size < ARAL_MIN_PAGE_SIZE ? ARAL_MIN_PAGE_SIZE : size;
}

static size_t aral_requested_max_page_size(ARAL *ar) {
    if(!ar->config.requested_max_page_size)
        return ar->config.mmap.enabled ? ARAL_MAX_PAGE_SIZE_MMAP : aral_optimal_malloc_page_size();
    else
        return ar->config.requested_max_page_size;
}

static size_t aral_max_allocation_size(ARAL *ar) {
    size_t size = memory_alignment(aral_requested_max_page_size(ar), ar->config.system_page_size);
    if(size < ar->config.min_required_page_size)
        size = ar->config.min_required_page_size;

    return size;
}

ARAL *aral_create(const char *name, size_t element_size, size_t initial_page_elements, size_t max_page_size,
                  struct aral_statistics *stats, const char *filename, const char **cache_dir,
                  bool mmap, bool lockless, bool dont_dump) {
    ARAL *ar = callocz(1, sizeof(ARAL));
    ar->config.options = ((lockless) ? ARAL_LOCKLESS : 0) | ((dont_dump) ? ARAL_DONT_DUMP : 0);
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

    // ----------------------------------------------------------------------------------------------------------------
    // disable mmap if the directories are not given

    if(ar->config.mmap.enabled && (!ar->config.mmap.cache_dir || !*ar->config.mmap.cache_dir)) {
        netdata_log_error("ARAL: '%s' mmap cache directory is not configured properly, disabling mmap.", ar->config.name);
        ar->config.mmap.enabled = false;
        internal_fatal(true, "ARAL: '%s' mmap cache directory is not configured properly", ar->config.name);
    }

    // ----------------------------------------------------------------------------------------------------------------
    // calculate element size, after adding our pointer

    ar->config.element_size = aral_element_slot_size(ar->config.requested_element_size, false);

    // we write the page pointer just after each element
    ar->config.element_ptr_offset = ar->config.element_size - sizeof(uintptr_t);

    if(ar->config.requested_element_size + sizeof(uintptr_t) > ar->config.element_size)
        fatal("ARAL: '%s' failed to calculate properly page_ptr_offset: "
              "element size %zu, sizeof(uintptr_t) %zu, natural alignment %zu, "
              "final element size %zu, page_ptr_offset %zu",
              ar->config.name, ar->config.requested_element_size, sizeof(uintptr_t),
              SYSTEM_REQUIRED_ALIGNMENT,
              ar->config.element_size, ar->config.element_ptr_offset);

    // ----------------------------------------------------------------------------------------------------------------
    // calculate allocation sizes

    ar->config.system_page_size = os_get_system_page_size();

    if (ar->config.initial_page_elements < 2)
        ar->config.initial_page_elements = 2;

    // find the minimum page size we will use
    ar->config.min_required_page_size = memory_alignment(sizeof(ARAL_PAGE), SYSTEM_REQUIRED_ALIGNMENT) + 2 * ar->config.element_size;

    if(ar->config.min_required_page_size < ARAL_MIN_PAGE_SIZE)
        ar->config.min_required_page_size = ARAL_MIN_PAGE_SIZE;

    ar->config.min_required_page_size = memory_alignment(ar->config.min_required_page_size, ar->config.system_page_size);

    // set the starting allocation size for both marked and unmarked partitions
    ar->ops[0].adders.allocation_size = ar->ops[1].adders.allocation_size = ar->config.min_required_page_size;

    // ----------------------------------------------------------------------------------------------------------------

    ar->aral_lock.pages_free = NULL;
    ar->aral_lock.pages_marked_free = NULL;
    ar->aral_lock.file_number = 0;

    // ----------------------------------------------------------------------------------------------------------------

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
                   , aral_max_allocation_size(ar) / ar->config.element_size
                   , aral_max_allocation_size(ar),  ar->config.requested_max_page_size
    );

    __atomic_add_fetch(&ar->stats->structures.allocations, 1, __ATOMIC_RELAXED);
    __atomic_add_fetch(&ar->stats->structures.allocated_bytes, sizeof(ARAL), __ATOMIC_RELAXED);
    return ar;
}

// --------------------------------------------------------------------------------------------------------------------
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

size_t aral_by_size_structures_bytes(void) {
    return aral_structures_bytes_from_stats(&aral_by_size_globals.shared_statistics);
}

size_t aral_by_size_free_bytes(void) {
    return aral_free_bytes_from_stats(&aral_by_size_globals.shared_statistics);
}

size_t aral_by_size_used_bytes(void) {
    return aral_used_bytes_from_stats(&aral_by_size_globals.shared_statistics);
}

size_t aral_by_size_padding_bytes(void) {
    return aral_padding_bytes_from_stats(&aral_by_size_globals.shared_statistics);
}

ARAL *aral_by_size_acquire(size_t size) {
    spinlock_lock(&aral_by_size_globals.spinlock);

    ARAL *ar = NULL;

    if(size <= ARAL_BY_SIZE_MAX_SIZE && aral_by_size_globals.array[size].ar) {
        ar = aral_by_size_globals.array[size].ar;
        aral_by_size_globals.array[size].refcount++;

        internal_fatal(
            aral_requested_element_size(ar) != size, "ARAL BY SIZE: aral has size %zu but we want %zu",
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
                         NULL, NULL, false, false, false);

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

// --------------------------------------------------------------------------------------------------------------------
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

static void aral_test_thread(void *ptr) {
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

        if (auc->single_threaded && ar->aral_lock.pages_free && ar->aral_lock.pages_free->page_lock.used_elements) {
            fprintf(stderr, "\n\nARAL leftovers detected (1)\n\n");
            __atomic_add_fetch(&auc->errors, 1, __ATOMIC_RELAXED);
        }

        if(!auc->single_threaded && __atomic_load_n(&auc->stop, __ATOMIC_RELAXED))
            break;

        for (size_t i = 0; i < elements; i++) {
            pointers[i] = unittest_aral_malloc(ar, marked);
        }

        size_t max_page_elements = aral_elements_in_page_size(ar, aral_max_allocation_size(ar));
        size_t increment = elements / max_page_elements;
        for (size_t all = increment; all <= elements / 2; all += increment) {

            size_t to_free = (all % max_page_elements) + 1;
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

        if (auc->single_threaded && ar->aral_lock.pages_free && ar->aral_lock.pages_free->page_lock.used_elements) {
            fprintf(stderr, "\n\nARAL leftovers detected (2)\n\n");
            __atomic_add_fetch(&auc->errors, 1, __ATOMIC_RELAXED);
        }

    } while(!auc->single_threaded && !__atomic_load_n(&auc->stop, __ATOMIC_RELAXED));

    freez(pointers);
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
                          NULL, false, false, false),
            .elements = elements,
            .errors = 0,
    };

    usec_t started_ut = now_monotonic_usec();
    ND_THREAD *thread_ptrs[threads];

    for(size_t i = 0; i < threads ; i++) {
        char tag[ND_THREAD_TAG_MAX + 1];
        snprintfz(tag, ND_THREAD_TAG_MAX, "TH[%zu]", i);
        thread_ptrs[i] = nd_thread_create(tag, NETDATA_THREAD_OPTION_DONT_LOG, aral_test_thread, &auc);
    }

    size_t malloc_done = 0;
    size_t free_done = 0;
    size_t countdown = seconds;
    while(countdown-- > 0) {
        sleep_usec(1 * USEC_PER_SEC);
        size_t m = __atomic_load_n(&auc.ar->atomic.user_malloc_operations, __ATOMIC_RELAXED);
        size_t f = __atomic_load_n(&auc.ar->atomic.user_free_operations, __ATOMIC_RELAXED);
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

    if (auc.ar->aral_lock.pages_free && auc.ar->aral_lock.pages_free->page_lock.used_elements) {
        fprintf(stderr, "\n\nARAL leftovers detected (3)\n\n");
        __atomic_add_fetch(&auc.errors, 1, __ATOMIC_RELAXED);
    }

    netdata_log_info("ARAL: did %zu malloc, %zu free, "
         "using %zu threads, in %"PRIu64" usecs",
         __atomic_load_n(&auc.ar->atomic.user_malloc_operations, __ATOMIC_RELAXED),
         __atomic_load_n(&auc.ar->atomic.user_free_operations, __ATOMIC_RELAXED),
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
                          false, false, false),
            .elements = elements,
            .errors = 0,
    };

    aral_test_thread(&auc);

    aral_destroy(auc.ar);

    int errors = aral_stress_test(2, elements, 10);

    return auc.errors + errors;
}
