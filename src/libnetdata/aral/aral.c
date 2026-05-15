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

#ifdef NETDATA_INTERNAL_CHECKS
struct aral_race_unittest_hook {
    ARAL *ar;
    ARAL_PAGE *page;
    struct aral_unittest_entry *forced_entry;
    bool enabled;
    bool first_allocator_waiting;
    bool release_first_allocator;
    bool first_allocator_claimed;
    bool page_force_fully_used;
};

static struct aral_race_unittest_hook aral_race_unittest_hook = { 0 };

static ALWAYS_INLINE void aral_unittest_wait_for_race_window(ARAL *ar, ARAL_PAGE *page) {
    if(unlikely(__atomic_load_n(&aral_race_unittest_hook.enabled, __ATOMIC_RELAXED) &&
                aral_race_unittest_hook.ar == ar)) {
        bool expected = false;
        if(__atomic_compare_exchange_n(&aral_race_unittest_hook.first_allocator_claimed, &expected, true, false,
                                       __ATOMIC_ACQ_REL, __ATOMIC_RELAXED)) {
            aral_race_unittest_hook.page = page;
            __atomic_store_n(&aral_race_unittest_hook.first_allocator_waiting, true, __ATOMIC_RELEASE);
            while(!__atomic_load_n(&aral_race_unittest_hook.release_first_allocator, __ATOMIC_ACQUIRE))
                tinysleep();
        }
    }
}

// Pause-point hook for the unmark/freez state-machine concurrency tests.
// Distinct from aral_race_unittest_hook above; fires only on a (ar, ptr, stage)
// match so the two test families do not interfere.
enum aral_concurrency_race_stage {
    ARAL_CONCURRENCY_RACE_NONE = 0,
    ARAL_CONCURRENCY_RACE_UNMARK_BEFORE_CAS,   // unmark entry, before reading the trailer
    ARAL_CONCURRENCY_RACE_UNMARK_AFTER_CAS,    // unmark after CAS to UNMARKING, before page_lock
    ARAL_CONCURRENCY_RACE_FREEZ_BEFORE_CLAIM,  // freez entry, before atomic-exchange
};

struct aral_concurrency_race_hook {
    ARAL *ar;
    void *target_ptr;
    enum aral_concurrency_race_stage stage;
    bool enabled;
    bool waiting;
    bool release;
};

static struct aral_concurrency_race_hook aral_concurrency_race_hook = { 0 };

// Counter incremented every time aral_claim_page_pointer_after_element___wait_for_unmark
// enters the cold path (i.e. observed UNMARKING in the trailer). Tests poll
// this to verify they actually exercised the cold path before completing.
static size_t aral_freez_unmarking_observed_count = 0;

// Arm the concurrency race hook for a (ar, ptr, stage) match.
// Sets the match fields first, then publishes `enabled = true` with a
// release-store. Paired with the acquire-load in aral_concurrency_race_pause,
// this guarantees a reader that observes enabled==true sees fully-published
// match fields. Without this ordering the compiler is free to publish
// `enabled` before the other fields, causing the pause to miss.
static inline void aral_concurrency_race_hook_arm(ARAL *ar, void *target_ptr,
                                                  enum aral_concurrency_race_stage stage) {
    aral_concurrency_race_hook.ar = ar;
    aral_concurrency_race_hook.target_ptr = target_ptr;
    aral_concurrency_race_hook.stage = stage;
    aral_concurrency_race_hook.waiting = false;
    aral_concurrency_race_hook.release = false;
    __atomic_store_n(&aral_concurrency_race_hook.enabled, true, __ATOMIC_RELEASE);
}

// Reset the hook. Callers should only invoke this after every concurrent
// reader is known to have stopped (i.e. all racing threads have been joined).
static inline void aral_concurrency_race_hook_reset(void) {
    __atomic_store_n(&aral_concurrency_race_hook.enabled, false, __ATOMIC_RELEASE);
    aral_concurrency_race_hook = (struct aral_concurrency_race_hook){ 0 };
}

static ALWAYS_INLINE void aral_concurrency_race_pause(ARAL *ar, void *ptr, enum aral_concurrency_race_stage stage) {
    // Acquire-load on `enabled` pairs with the release-store in
    // aral_concurrency_race_hook_arm so that if we observe enabled==true,
    // the other match fields are fully published.
    if(unlikely(__atomic_load_n(&aral_concurrency_race_hook.enabled, __ATOMIC_ACQUIRE) &&
                aral_concurrency_race_hook.ar == ar &&
                aral_concurrency_race_hook.target_ptr == ptr &&
                aral_concurrency_race_hook.stage == stage)) {
        __atomic_store_n(&aral_concurrency_race_hook.waiting, true, __ATOMIC_RELEASE);
        while(!__atomic_load_n(&aral_concurrency_race_hook.release, __ATOMIC_ACQUIRE))
            tinysleep();
    }
}
#endif

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
//
// Trailer state machine (the low bits of the per-slot trailer word):
//   0                                       = freed / on the free list
//   page                                    = allocated, unmarked
//   page | ARAL_TRAILER_MARKED              = allocated, marked
//   page | ARAL_TRAILER_UNMARKING           = allocated, unmark in progress
//                                             (page counters not yet updated)
//
// Page pointers must be aligned such that the low 2 bits are always 0,
// otherwise the tag bits would collide with real address bits. The static
// assert below enforces this at compile time on every supported platform.
//
// The UNMARKING state is set by aral_unmark_allocation() before it takes the
// page lock to decrement page->page_lock.marked_elements, and is cleared
// after the decrement is complete (still under page lock). While UNMARKING
// is visible, aral_freez_internal() observes it and waits, so:
//   - the slot's refcount contribution remains in place, keeping the page
//     alive across our trailer transition (no UAF on aral_page_lock())
//   - freez never sees an unmarked trailer with a stale marked_elements
//     counter (no spurious "marked > used" assertion)

#define ARAL_TRAILER_MARKED       ((uintptr_t)0x1)
#define ARAL_TRAILER_UNMARKING    ((uintptr_t)0x2)
#define ARAL_TRAILER_TAG_MASK     (ARAL_TRAILER_MARKED | ARAL_TRAILER_UNMARKING)

_Static_assert((SYSTEM_REQUIRED_ALIGNMENT & ARAL_TRAILER_TAG_MASK) == 0,
               "ARAL trailer tag bits collide with page pointer alignment");

static ALWAYS_INLINE ARAL_PAGE *aral_decode_page_pointer_after_element___do_NOT_have_aral_lock(ARAL *ar, void *ptr, uintptr_t tagged_page, bool *marked) {
    *marked = (tagged_page & ARAL_TRAILER_MARKED) != 0;
    ARAL_PAGE *page = (ARAL_PAGE *)(tagged_page & ~ARAL_TRAILER_TAG_MASK);

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

// Retrieving the pointer and the 'marked' flag
static ALWAYS_INLINE ARAL_PAGE *aral_get_page_pointer_after_element___do_NOT_have_aral_lock(ARAL *ar, void *ptr, bool *marked) {
    uint8_t *data = ptr;
    uintptr_t *page_ptr = (uintptr_t *)&data[ar->config.element_ptr_offset];
    uintptr_t tagged_page = __atomic_load_n(page_ptr, __ATOMIC_ACQUIRE);  // Atomically load the tagged pointer

    return aral_decode_page_pointer_after_element___do_NOT_have_aral_lock(ar, ptr, tagged_page, marked);
}

// Atomically claims an allocated slot for freeing.
//
// Hot path: single atomic-exchange to 0 (same primitive as before the unmark
// protocol existed - no extra load, no CAS).
//
// Cold path: if the exchange returned a value with UNMARKING set, we
// accidentally claimed a slot mid-unmark. Best-effort restore the UNMARKING
// state so unmark can finish, yield briefly, and retry. The restore CAS only
// succeeds if the trailer is still 0 (i.e. nothing else touched it since our
// exchange); if it fails, the next iteration's exchange picks up whatever
// settled value the other writer left behind.
//
// Returns NULL on a concurrent double-free or stale free.
static ALWAYS_INLINE ARAL_PAGE *aral_claim_page_pointer_after_element___wait_for_unmark(ARAL *ar, void *ptr, bool *marked) {
    uint8_t *data = ptr;
    uintptr_t *page_ptr = (uintptr_t *)&data[ar->config.element_ptr_offset];

    while(true) {
        uintptr_t prior = __atomic_exchange_n(page_ptr, 0, __ATOMIC_ACQ_REL);

        if(unlikely(!prior)) {
            *marked = false;
            return NULL;
        }

        if(likely(!(prior & ARAL_TRAILER_UNMARKING)))
            return aral_decode_page_pointer_after_element___do_NOT_have_aral_lock(ar, ptr, prior, marked);

        // Cold path: we exchanged with the UNMARKING transition. Put it back
        // (only succeeds if the trailer is still 0) and retry once unmark has
        // had a chance to publish the final state.
        uintptr_t zero = 0;
        __atomic_compare_exchange_n(page_ptr, &zero, prior,
                                    false, __ATOMIC_RELEASE, __ATOMIC_RELAXED);
#ifdef NETDATA_INTERNAL_CHECKS
        __atomic_add_fetch(&aral_freez_unmarking_observed_count, 1, __ATOMIC_RELAXED);
#endif
        tinysleep();
    }
}

static ALWAYS_INLINE void aral_set_page_pointer_after_element___do_NOT_have_aral_lock(ARAL *ar, void *page, void *ptr, bool marked) {
    uint8_t *data = ptr;
    uintptr_t *page_ptr = (uintptr_t *)&data[ar->config.element_ptr_offset];
    uintptr_t tagged_page = (uintptr_t)page;
    if (marked) tagged_page |= ARAL_TRAILER_MARKED;
    __atomic_store_n(page_ptr, tagged_page, __ATOMIC_RELEASE);
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
    if(rf <= 0) {
        __atomic_sub_fetch(&page->refcount, 1, __ATOMIC_RELAXED);
        return false;
    }

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

    ARAL_PAGE *page = NULL;

retry_acquisition:

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

#ifdef NETDATA_INTERNAL_CHECKS
    aral_unittest_wait_for_race_window(ar, page);
#endif

    aral_page_lock(ar, page);

    if(unlikely(!page->page_lock.free_elements)) {
        aral_page_unlock(ar, page);
        bool deleted = aral_page_release(page);
        (void)deleted;
        page = NULL;
        goto retry_acquisition;
    }

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
        uint32_t bitmap = __atomic_load_n(&page->incoming_partition_bitmap, __ATOMIC_ACQUIRE);
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
                __atomic_fetch_and(&page->incoming_partition_bitmap, ~(1U << partition), __ATOMIC_RELEASE);
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
                __atomic_fetch_or(&page->incoming_partition_bitmap, 1U << partition, __ATOMIC_RELEASE);
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

#ifdef NETDATA_INTERNAL_CHECKS
    aral_concurrency_race_pause(ar, ptr, ARAL_CONCURRENCY_RACE_UNMARK_BEFORE_CAS);
#endif

    uint8_t *data = ptr;
    uintptr_t *page_ptr = (uintptr_t *)&data[ar->config.element_ptr_offset];

    // Stage 1: claim the unmark transition.
    // CAS the trailer from (page, MARKED) to (page, UNMARKING). On failure
    // (slot was freed, already unmarked, another unmark won), bail without
    // touching counters.
    //
    // Holding the UNMARKING state has two crucial effects:
    //   - aral_freez_internal observes UNMARKING and waits, so the slot's
    //     refcount contribution stays in place and the page cannot be
    //     destroyed under us.
    //   - freez never observes the slot as unmarked while marked_elements
    //     is still high, so the "marked > used" invariant is preserved.
    uintptr_t initial = __atomic_load_n(page_ptr, __ATOMIC_ACQUIRE);
    if((initial & ARAL_TRAILER_TAG_MASK) != ARAL_TRAILER_MARKED)
        return;
    uintptr_t desired = (initial & ~ARAL_TRAILER_TAG_MASK) | ARAL_TRAILER_UNMARKING;
    if(!__atomic_compare_exchange_n(page_ptr, &initial, desired,
                                    false, __ATOMIC_ACQ_REL, __ATOMIC_ACQUIRE))
        return;

#ifdef NETDATA_INTERNAL_CHECKS
    aral_concurrency_race_pause(ar, ptr, ARAL_CONCURRENCY_RACE_UNMARK_AFTER_CAS);
#endif

    // Stage 2: under page_lock, decrement counters and update page lists.
    bool was_marked;
    ARAL_PAGE *page = aral_decode_page_pointer_after_element___do_NOT_have_aral_lock(ar, ptr, initial, &was_marked);
    (void)was_marked;

    aral_page_lock(ar, page);
    internal_fatal(!page->page_lock.marked_elements, "Marked counter going negative.");
    bool unmark = (--page->page_lock.marked_elements == 0) && page->page_lock.used_elements;

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

    // Stage 3: publish the final UNMARKED state.
    // Atomic store transitions the trailer from (page, UNMARKING) to (page, UNMARKED).
    // Done under page_lock so any waiting freez observes a settled trailer
    // only after our counter update is committed.
    __atomic_store_n(page_ptr, (uintptr_t)page, __ATOMIC_RELEASE);

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

#ifdef NETDATA_INTERNAL_CHECKS
    aral_concurrency_race_pause(ar, ptr, ARAL_CONCURRENCY_RACE_FREEZ_BEFORE_CLAIM);
#endif

    // Atomically claim the trailer:
    //  - On a concurrent double-free or stale-free, the loser observes a
    //    NULL pointer and fatal()s here.
    //  - If aral_unmark_allocation has CAS'd the trailer to UNMARKING, we
    //    wait until it publishes the final UNMARKED state, so we never see
    //    the slot as unmarked while marked_elements is still high.
    bool marked;
    ARAL_PAGE *page = aral_claim_page_pointer_after_element___wait_for_unmark(ar, ptr, &marked);
    if(unlikely(!page))
        fatal("ARAL: '%s' double free, stale free, or corrupted pointer %p", ar->config.name, ptr);

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
        // Reset elements_segmented first to prevent new fast-path allocations
        __atomic_store_n(&page->elements_segmented, 0, __ATOMIC_RELEASE);

        // Clear available list under its lock
        aral_page_available_lock(ar, page);
        page->available.list = NULL;
        aral_page_available_unlock(ar, page);

        // Clear incoming partition lists under their respective locks
        // to synchronize with allocators in aral_get_free_slot___no_lock_required
        for(size_t p = 0; p < ARAL_PAGE_INCOMING_PARTITIONS; p++) {
            aral_page_incoming_lock(ar, page, p);
            page->incoming[p].list = NULL;
            aral_page_incoming_unlock(ar, page, p);
        }

        // Clear bitmap last with atomic operation to ensure visibility
        __atomic_store_n(&page->incoming_partition_bitmap, 0, __ATOMIC_RELEASE);
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

#ifdef NETDATA_INTERNAL_CHECKS

struct aral_race_unittest_allocator {
    ARAL *ar;
    struct aral_unittest_entry *entry;
};
static bool aral_unittest_wait_for_flag(bool *flag, usec_t timeout_ut) {
    usec_t started_ut = now_monotonic_usec();

    while(!__atomic_load_n(flag, __ATOMIC_ACQUIRE)) {
        if(now_monotonic_usec() - started_ut > timeout_ut)
            return false;

        tinysleep();
    }

    return true;
}

static void aral_race_unittest_allocator_thread(void *ptr) {
    struct aral_race_unittest_allocator *ctx = ptr;
    ctx->entry = unittest_aral_malloc(ctx->ar, false);
}

static struct aral_unittest_entry *aral_race_unittest_force_page_full(ARAL *ar, ARAL_PAGE *page) {
    struct aral_unittest_entry *entry;
    aral_page_lock(ar, page);

    internal_fatal(!page->page_lock.free_elements,
                   "ARAL race unittest: target page unexpectedly has no free elements");

    uint64_t slot = __atomic_fetch_add(&page->elements_segmented, 1, __ATOMIC_ACQUIRE);
    internal_fatal(slot >= page->max_elements,
                   "ARAL race unittest: failed to reserve the last free slot");

    entry = (struct aral_unittest_entry *)(page->data + (slot * ar->config.element_size));
    aral_set_page_pointer_after_element___do_NOT_have_aral_lock(ar, page, entry, false);
    *entry = UNITTEST_ITEM;
    aral_element_given(ar, page);

    page->page_lock.used_elements++;
    page->page_lock.free_elements--;

    REFCOUNT rf = __atomic_add_fetch(&page->refcount, 1, __ATOMIC_RELAXED);
    internal_fatal(rf < (REFCOUNT)page->max_elements || rf > (REFCOUNT)page->max_elements + 1,
                   "ARAL race unittest: invalid forced refcount %d for max_elements %u",
                   rf, page->max_elements);

    aral_lock(ar);
    if(page->aral_lock.head_ptr == aral_pages_head_free(ar, false)) {
        DOUBLE_LINKED_LIST_REMOVE_ITEM_UNSAFE(*page->aral_lock.head_ptr, page, aral_lock.prev, aral_lock.next);

        ARAL_PAGE **head_ptr_full = aral_pages_head_full(ar, false);
        DOUBLE_LINKED_LIST_APPEND_ITEM_UNSAFE(*head_ptr_full, page, aral_lock.prev, aral_lock.next);
        page->aral_lock.head_ptr = head_ptr_full;
    }
    aral_unlock(ar);

    aral_race_unittest_hook.forced_entry = entry;
    __atomic_store_n(&aral_race_unittest_hook.page_force_fully_used, true, __ATOMIC_RELEASE);

    aral_page_unlock(ar, page);
    return entry;
}

static int aral_detect_acquire_to_page_lock_race(void) {
    int errors = 0;
    bool allocator_entry_marked = false;
    ARAL_PAGE *allocator_page = NULL;
    ARAL *ar = aral_create("aral-race-test",
                           sizeof(struct aral_unittest_entry),
                           0,
                           0,
                           NULL,
                           "aral-race-test",
                           NULL, false, false, false);

    size_t page_elements = aral_elements_in_page_size(ar, ar->ops[0].adders.allocation_size);
    struct aral_unittest_entry **filled = callocz(page_elements, sizeof(*filled));
    struct aral_race_unittest_allocator allocator = {
        .ar = ar,
        .entry = NULL,
    };

    for(size_t i = 0; i < page_elements - 1; i++)
        filled[i] = unittest_aral_malloc(ar, false);

    aral_race_unittest_hook = (struct aral_race_unittest_hook) {
        .ar = ar,
        .enabled = true,
    };

    ND_THREAD *thread = nd_thread_create("ARALRACE", NETDATA_THREAD_OPTION_DONT_LOG,
                                         aral_race_unittest_allocator_thread, &allocator);

    if(!thread) {
        fprintf(stderr, "ARAL race unittest: failed to create allocator thread.\n");
        errors++;
    }

    if(thread && !aral_unittest_wait_for_flag(&aral_race_unittest_hook.first_allocator_waiting, 5 * USEC_PER_SEC)) {
        fprintf(stderr, "ARAL race unittest: timed out waiting for the first allocator to pause.\n");
        errors++;
    }
    else if(thread) {
        if(!aral_race_unittest_hook.page) {
            fprintf(stderr, "ARAL race unittest: paused allocator did not publish its target page.\n");
            errors++;
        }
        else
            aral_race_unittest_force_page_full(ar, aral_race_unittest_hook.page);
    }

    __atomic_store_n(&aral_race_unittest_hook.release_first_allocator, true, __ATOMIC_RELEASE);
    if(thread)
        nd_thread_join(thread);
    __atomic_store_n(&aral_race_unittest_hook.enabled, false, __ATOMIC_RELEASE);

    if(!allocator.entry) {
        fprintf(stderr, "ARAL race unittest: paused allocator failed to complete its allocation.\n");
        errors++;
    }
    else
        allocator_page = aral_get_page_pointer_after_element___do_NOT_have_aral_lock(ar, allocator.entry, &allocator_entry_marked);

    (void)allocator_entry_marked;

    if(ar->aral_lock.pages_full == NULL) {
        fprintf(stderr, "ARAL race unittest: expected the original page to become full during the race.\n");
        errors++;
    }

    if(!__atomic_load_n(&aral_race_unittest_hook.page_force_fully_used, __ATOMIC_ACQUIRE)) {
        fprintf(stderr, "ARAL race unittest: failed to force the page into the fully-used state.\n");
        errors++;
    }

    if(errors == 0 && allocator_page == aral_race_unittest_hook.page) {
        fprintf(stderr, "ARAL race unittest: allocator retried on the forced-full page instead of a new page.\n");
        errors++;
    }

    if(aral_race_unittest_hook.forced_entry)
        aral_freez(ar, aral_race_unittest_hook.forced_entry);

    if(allocator.entry)
        aral_freez(ar, allocator.entry);

    for(size_t i = 0; i < page_elements - 1; i++) {
        if(filled[i])
            aral_freez(ar, filled[i]);
    }

    freez(filled);
    aral_destroy(ar);
    aral_race_unittest_hook = (struct aral_race_unittest_hook) { 0 };

    return errors;
}

// --------------------------------------------------------------------------------------------------------------------
// Concurrency tests for the unmark / freez state machine.
//
// Each scenario uses the aral_concurrency_race_hook to deterministically pause
// one operation at a known point so a second operation can race it. Tests 4-6
// drive both sides through the trailer transition concurrently and verify
// that page counters end consistent and no assertion fires; tests 1-3 are
// bug-independent sanity checks for the unmark / freez API contract.

struct aral_concurrency_test_args {
    ARAL *ar;
    void *ptr;
};

static void aral_concurrency_test_unmark_thread(void *arg) {
    struct aral_concurrency_test_args *a = arg;
    aral_unmark_allocation(a->ar, a->ptr);
}

static void aral_concurrency_test_freez_thread(void *arg) {
    struct aral_concurrency_test_args *a = arg;
    aral_freez(a->ar, a->ptr);
}

static ARAL_PAGE *aral_concurrency_test_decode(ARAL *ar, void *ptr, bool *marked) {
    uint8_t *data = ptr;
    uintptr_t *page_ptr = (uintptr_t *)&data[ar->config.element_ptr_offset];
    uintptr_t tagged = __atomic_load_n(page_ptr, __ATOMIC_ACQUIRE);
    return aral_decode_page_pointer_after_element___do_NOT_have_aral_lock(ar, ptr, tagged, marked);
}

// Common setup for tests that need a marked guard and a marked test entry on
// the same page. Captures used/marked counters at setup time so each test can
// verify the expected delta after its scenario runs.
struct aral_concurrency_test_fixture {
    ARAL *ar;
    struct aral_unittest_entry *guard;
    struct aral_unittest_entry *entry;
    ARAL_PAGE *page;
    uint32_t used0;
    uint32_t marked0;
};

// Initialize the fixture. Returns true on success. On failure (guard/entry
// landed on different pages), the aral and allocations are torn down and
// false is returned; the caller should bubble up an error.
static bool aral_concurrency_test_fixture_init(struct aral_concurrency_test_fixture *f, const char *name) {
    f->ar = aral_create(name, sizeof(struct aral_unittest_entry),
                        0, 0, NULL, name, NULL, false, false, false);

    // Guard is allocated marked so it lives on the same (marked) page as the
    // test entry. Marked and unmarked allocations use separate page lists.
    f->guard = aral_mallocz_marked(f->ar);
    *f->guard = UNITTEST_ITEM;

    f->entry = aral_mallocz_marked(f->ar);
    *f->entry = UNITTEST_ITEM;

    bool m = false;
    f->page = aral_concurrency_test_decode(f->ar, f->entry, &m);
    bool gm = false;
    ARAL_PAGE *guard_page = aral_concurrency_test_decode(f->ar, f->guard, &gm);
    if(guard_page != f->page) {
        fprintf(stderr, "    setup: guard and entry are not on the same page (guard=%p entry=%p)\n",
                (void*)guard_page, (void*)f->page);
        aral_freez(f->ar, f->entry);
        aral_freez(f->ar, f->guard);
        aral_destroy(f->ar);
        return false;
    }
    f->used0 = f->page->page_lock.used_elements;
    f->marked0 = f->page->page_lock.marked_elements;
    return true;
}

// Teardown when the test entry was already freed by the scenario.
// Frees the guard, destroys the aral, resets the concurrency hook.
static void aral_concurrency_test_fixture_teardown(struct aral_concurrency_test_fixture *f) {
    aral_freez(f->ar, f->guard);
    aral_destroy(f->ar);
    aral_concurrency_race_hook_reset();
}

// Teardown when the test entry is still allocated (e.g. failure path before
// the scenario could free it). Frees both entry and guard.
static void aral_concurrency_test_fixture_teardown_with_entry(struct aral_concurrency_test_fixture *f) {
    aral_freez(f->ar, f->entry);
    aral_freez(f->ar, f->guard);
    aral_destroy(f->ar);
    aral_concurrency_race_hook_reset();
}

// Verify the page counters reflect exactly one freez of the test entry:
// used and marked each decremented by 1. Returns the error count to add.
static int aral_concurrency_test_check_counters_after_one_freez(struct aral_concurrency_test_fixture *f) {
    int errors = 0;
    if(f->page->page_lock.used_elements != f->used0 - 1) {
        fprintf(stderr, "    used_elements: %u -> %u (expected %u)\n",
                f->used0, f->page->page_lock.used_elements, f->used0 - 1);
        errors++;
    }
    if(f->page->page_lock.marked_elements != f->marked0 - 1) {
        fprintf(stderr, "    marked_elements: %u -> %u (expected %u)\n",
                f->marked0, f->page->page_lock.marked_elements, f->marked0 - 1);
        errors++;
    }
    return errors;
}

// Test 1: clean unmark on a marked allocation. No concurrency.
// Expects: marked counter -1, used counter unchanged, slot trailer becomes UNMARKED.
static int aral_concurrency_test_clean_unmark(void) {
    int errors = 0;
    fprintf(stderr, "  test 1: clean unmark on a marked allocation\n");

    ARAL *ar = aral_create("aral-conc-1", sizeof(struct aral_unittest_entry),
                           0, 0, NULL, "aral-conc-1", NULL, false, false, false);

    struct aral_unittest_entry *entry = aral_mallocz_marked(ar);
    *entry = UNITTEST_ITEM;

    bool m = false;
    ARAL_PAGE *page = aral_concurrency_test_decode(ar, entry, &m);
    if(!m) { fprintf(stderr, "    setup: not marked\n"); errors++; }
    uint32_t used0 = page->page_lock.used_elements;
    uint32_t marked0 = page->page_lock.marked_elements;

    aral_unmark_allocation(ar, entry);

    bool m_after = true;
    ARAL_PAGE *p_after = aral_concurrency_test_decode(ar, entry, &m_after);
    if(p_after != page || m_after) {
        fprintf(stderr, "    trailer state wrong after unmark (page=%p marked=%d)\n", (void*)p_after, (int)m_after);
        errors++;
    }
    if(page->page_lock.used_elements != used0) {
        fprintf(stderr, "    used_elements changed (%u -> %u)\n", used0, page->page_lock.used_elements);
        errors++;
    }
    if(page->page_lock.marked_elements != marked0 - 1) {
        fprintf(stderr, "    marked_elements: %u -> %u (expected %u)\n",
                marked0, page->page_lock.marked_elements, marked0 - 1);
        errors++;
    }

    aral_freez(ar, entry);
    aral_destroy(ar);
    return errors;
}

// Test 2: unmark on an already-unmarked allocation. Should bail without changing counters.
static int aral_concurrency_test_unmark_on_unmarked(void) {
    int errors = 0;
    fprintf(stderr, "  test 2: unmark on an already-unmarked allocation (should bail)\n");

    ARAL *ar = aral_create("aral-conc-2", sizeof(struct aral_unittest_entry),
                           0, 0, NULL, "aral-conc-2", NULL, false, false, false);

    // allocate UNMARKED (regular mallocz)
    struct aral_unittest_entry *entry = aral_mallocz(ar);
    *entry = UNITTEST_ITEM;

    bool m = true;
    ARAL_PAGE *page = aral_concurrency_test_decode(ar, entry, &m);
    if(m) { fprintf(stderr, "    setup: unexpectedly marked\n"); errors++; }
    uint32_t used0 = page->page_lock.used_elements;
    uint32_t marked0 = page->page_lock.marked_elements;

    aral_unmark_allocation(ar, entry);

    if(page->page_lock.used_elements != used0 || page->page_lock.marked_elements != marked0) {
        fprintf(stderr, "    counters changed after no-op unmark (used %u->%u, marked %u->%u)\n",
                used0, page->page_lock.used_elements, marked0, page->page_lock.marked_elements);
        errors++;
    }

    aral_freez(ar, entry);
    aral_destroy(ar);
    return errors;
}

// Test 3: clean freez on a marked allocation. No concurrency.
// Expects: used -1, marked -1, slot returns to free pool.
//
// A marked guard is kept alive on the same page so the page is not destroyed
// (or otherwise has its counters reset) by freezing the only allocation.
static int aral_concurrency_test_clean_freez_marked(void) {
    int errors = 0;
    fprintf(stderr, "  test 3: clean freez of a marked allocation\n");

    struct aral_concurrency_test_fixture f;
    if(!aral_concurrency_test_fixture_init(&f, "aral-conc-3"))
        return 1;

    aral_freez(f.ar, f.entry);

    errors += aral_concurrency_test_check_counters_after_one_freez(&f);

    aral_concurrency_test_fixture_teardown(&f);
    return errors;
}

// Test 4: race - freez wins claim before unmark gets to its CAS.
// Pause unmark at entry, run freez to completion, release unmark.
// On master (no fix): unmark loads trailer=0, decode produces page=NULL,
//   triggers "possible corruption or double free" internal_fatal under
//   NETDATA_INTERNAL_CHECKS, OR aral_set_page_pointer with NULL page,
//   then aral_page_lock(NULL) SEGV.
// On v2: unmark loads 0, the (initial & TAG_MASK) != MARKED check bails
//   without touching counters or pointers.
// Either outcome is captured: master crashes, v2 passes counter checks.
//
// A guard allocation is kept alive on the same page so the page is never
// destroyed mid-test (refcount stays > 0), making it safe to read page
// counters after the racing freez completes.
static int aral_concurrency_test_freez_wins(void) {
    int errors = 0;
    fprintf(stderr, "  test 4: race - freez wins claim before unmark CAS\n");

    struct aral_concurrency_test_fixture f;
    if(!aral_concurrency_test_fixture_init(&f, "aral-conc-4"))
        return 1;

    aral_concurrency_race_hook_arm(f.ar, f.entry, ARAL_CONCURRENCY_RACE_UNMARK_BEFORE_CAS);

    struct aral_concurrency_test_args ctx = { f.ar, f.entry };
    ND_THREAD *unmark_thread = nd_thread_create("UNMARK", NETDATA_THREAD_OPTION_DONT_LOG,
                                                aral_concurrency_test_unmark_thread, &ctx);
    if(!unmark_thread) {
        fprintf(stderr, "    failed to create unmark thread\n");
        aral_concurrency_test_fixture_teardown_with_entry(&f);
        return errors + 1;
    }

    if(!aral_unittest_wait_for_flag(&aral_concurrency_race_hook.waiting, 5 * USEC_PER_SEC)) {
        fprintf(stderr, "    unmark thread did not pause\n");
        errors++;
    }

    // The hook is armed for an UNMARK_* stage, so the freez we are about to
    // run will not match (its only pause point is FREEZ_BEFORE_CLAIM). Just
    // freez and let unmark resume on the release flag below.
    aral_freez(f.ar, f.entry);

    __atomic_store_n(&aral_concurrency_race_hook.release, true, __ATOMIC_RELEASE);
    nd_thread_join(unmark_thread);

    if(f.page->page_lock.used_elements != f.used0 - 1) {
        fprintf(stderr, "    used_elements: %u -> %u (expected %u)\n",
                f.used0, f.page->page_lock.used_elements, f.used0 - 1);
        errors++;
    }
    if(f.page->page_lock.marked_elements != f.marked0 - 1) {
        fprintf(stderr, "    marked_elements: %u -> %u (expected %u; %u would mean unmark double-decremented)\n",
                f.marked0, f.page->page_lock.marked_elements, f.marked0 - 1, f.marked0 - 2);
        errors++;
    }

    aral_concurrency_test_fixture_teardown(&f);
    return errors;
}

// Tests 5 and 6: race - unmark wins the trailer transition, then freez runs.
// Pause unmark AFTER its trailer transition (UNMARKING on v2, UNMARKED on
// master) but BEFORE the page-lock counter update. Run freez concurrently:
// on v2 it should observe UNMARKING, restore, and spin until unmark publishes
// the final state; on master it observes UNMARKED, captures marked=false,
// then races on page_lock with unmark.
//
// Master under NETDATA_INTERNAL_CHECKS: if freez wins page_lock first, the
// "marked > used" assertion at aral_freez_internal fires (or the deletion
// path's "page has marked elements but not used ones" fires). This is the
// exact race the v2 protocol closes.
//
// v2: counters end consistent: used -1, marked -1.
//
// Test 5 sleeps 10ms before releasing unmark to let freez settle into its
// cold-path spin; test 6 releases immediately to also catch regressions in
// the very first iteration of the cold path.
static int aral_concurrency_test_unmark_wins_impl(const char *aral_name, usec_t grace_us) {
    int errors = 0;

    struct aral_concurrency_test_fixture f;
    if(!aral_concurrency_test_fixture_init(&f, aral_name))
        return 1;

    aral_concurrency_race_hook_arm(f.ar, f.entry, ARAL_CONCURRENCY_RACE_UNMARK_AFTER_CAS);

    struct aral_concurrency_test_args ctx = { f.ar, f.entry };
    ND_THREAD *unmark_thread = nd_thread_create("UNMARK", NETDATA_THREAD_OPTION_DONT_LOG,
                                                aral_concurrency_test_unmark_thread, &ctx);
    if(!unmark_thread) {
        fprintf(stderr, "    failed to create unmark thread\n");
        aral_concurrency_test_fixture_teardown_with_entry(&f);
        return errors + 1;
    }

    if(!aral_unittest_wait_for_flag(&aral_concurrency_race_hook.waiting, 5 * USEC_PER_SEC)) {
        fprintf(stderr, "    unmark thread did not pause after trailer transition\n");
        errors++;
        __atomic_store_n(&aral_concurrency_race_hook.release, true, __ATOMIC_RELEASE);
        nd_thread_join(unmark_thread);
        aral_concurrency_test_fixture_teardown_with_entry(&f);
        return errors;
    }

    // Hook is armed for UNMARK_AFTER_CAS - freez's FREEZ_BEFORE_CLAIM pause
    // point will not match, so the freez thread proceeds without pausing.
    ND_THREAD *freez_thread = nd_thread_create("FREEZ", NETDATA_THREAD_OPTION_DONT_LOG,
                                               aral_concurrency_test_freez_thread, &ctx);
    if(!freez_thread) {
        fprintf(stderr, "    failed to create freez thread\n");
        __atomic_store_n(&aral_concurrency_race_hook.release, true, __ATOMIC_RELEASE);
        nd_thread_join(unmark_thread);
        // unmark completed - it transitioned the slot to UNMARKED but did not
        // free it. We must free entry too.
        aral_concurrency_test_fixture_teardown_with_entry(&f);
        return errors + 1;
    }

    if(grace_us > 0)
        sleep_usec(grace_us);

    __atomic_store_n(&aral_concurrency_race_hook.release, true, __ATOMIC_RELEASE);
    nd_thread_join(unmark_thread);
    nd_thread_join(freez_thread);

    errors += aral_concurrency_test_check_counters_after_one_freez(&f);

    aral_concurrency_test_fixture_teardown(&f);
    return errors;
}

static int aral_concurrency_test_unmark_wins_transition(void) {
    fprintf(stderr, "  test 5: race - unmark transitions trailer, freez races counter update\n");
    return aral_concurrency_test_unmark_wins_impl("aral-conc-5", 10 * USEC_PER_MS);
}

static int aral_concurrency_test_unmark_wins_no_grace(void) {
    fprintf(stderr, "  test 6: race - same as test 5, no grace period before release\n");
    return aral_concurrency_test_unmark_wins_impl("aral-conc-6", 0);
}

// Test 7: unmark of the last marked element on a page triggers a list move
// from the marked-pages list to the unmarked-pages list. This is the
// "if(unmark)" branch of aral_unmark_allocation that takes aral_lock and
// moves the page between linked lists.
static int aral_concurrency_test_unmark_last_marked_on_page(void) {
    int errors = 0;
    fprintf(stderr, "  test 7: unmark of last marked element triggers list move\n");

    ARAL *ar = aral_create("aral-conc-7", sizeof(struct aral_unittest_entry),
                           0, 0, NULL, "aral-conc-7", NULL, false, false, false);

    // Single marked allocation. After unmarking, marked_elements drops to 0
    // while used_elements stays at 1 (unmarking does not free the slot), which
    // triggers the unmark branch in aral_unmark_allocation that takes
    // aral_lock and moves the page from the marked-pages list to the
    // unmarked-pages list.
    struct aral_unittest_entry *m1 = aral_mallocz_marked(ar);
    *m1 = UNITTEST_ITEM;

    bool m = false;
    ARAL_PAGE *marked_page = aral_concurrency_test_decode(ar, m1, &m);
    if(!m) { fprintf(stderr, "    setup: not marked\n"); errors++; }
    if(!marked_page->aral_lock.marked) {
        fprintf(stderr, "    setup: page not on marked list before unmark\n");
        errors++;
    }

    aral_unmark_allocation(ar, m1);

    if(marked_page->page_lock.marked_elements != 0) {
        fprintf(stderr, "    marked_elements not 0 after unmark of last marked: %u\n",
                marked_page->page_lock.marked_elements);
        errors++;
    }
    if(marked_page->aral_lock.marked) {
        fprintf(stderr, "    page still on marked list after unmarking last marked\n");
        errors++;
    }

    aral_freez(ar, m1);
    aral_destroy(ar);
    return errors;
}

// Test 8: coordinated per-pointer race stress.
// For every pointer in the pool, deterministically force the racing window:
//   1. Arm the hook to pause unmark at the UNMARKING transition.
//   2. Spawn an unmark thread - it CAS's to UNMARKING and pauses at the hook.
//   3. Spawn a freez thread on the same pointer - it observes UNMARKING and
//      enters the cold path (restore + retry) of the claim helper.
//   4. Release unmark - it finishes its counter update and publishes UNMARKED.
//   5. Freez's retry exchange picks up UNMARKED, claims, decrements only used.
//   6. Both threads complete; the slot is fully freed.
//
// This exercises every interesting transition for every pointer:
//   - unmark wins the trailer transition
//   - freez observes UNMARKING (cold path of the claim helper)
//   - the published UNMARKED value lets freez proceed
// Plus, because the pool spans multiple pages, the freezes near the end of
// each page exercise the "last marked element triggers list move" branch and
// the page deletion path.
//
// Final state is verified via aral_used_bytes(): must be 0 after every slot
// is freed.
static int aral_concurrency_test_stress(void) {
    int errors = 0;
    const size_t pool_size = 256;  // large enough to span multiple pages
    fprintf(stderr, "  test 8: coordinated race stress - %zu pointers, deterministic UNMARKING for each\n", pool_size);

    ARAL *ar = aral_create("aral-conc-stress", sizeof(struct aral_unittest_entry),
                           0, 0, NULL, "aral-conc-stress", NULL, false, false, false);

    struct aral_unittest_entry **pool = callocz(pool_size, sizeof(*pool));
    for(size_t i = 0; i < pool_size; i++) {
        pool[i] = aral_mallocz_marked(ar);
        *pool[i] = UNITTEST_ITEM;
    }

    size_t cold_path_hits = 0;
    size_t i;
    for(i = 0; i < pool_size; i++) {
        // Arm the hook to pause unmark just after its CAS to UNMARKING.
        aral_concurrency_race_hook_arm(ar, pool[i], ARAL_CONCURRENCY_RACE_UNMARK_AFTER_CAS);

        struct aral_concurrency_test_args ctx = { ar, pool[i] };

        ND_THREAD *unmark_thread = nd_thread_create("UNMARK", NETDATA_THREAD_OPTION_DONT_LOG,
                                                    aral_concurrency_test_unmark_thread, &ctx);
        if(!unmark_thread) {
            fprintf(stderr, "    iter %zu: failed to create unmark thread\n", i);
            errors++;
            break;
        }

        if(!aral_unittest_wait_for_flag(&aral_concurrency_race_hook.waiting, 5 * USEC_PER_SEC)) {
            fprintf(stderr, "    iter %zu: unmark did not reach pause\n", i);
            errors++;
            __atomic_store_n(&aral_concurrency_race_hook.release, true, __ATOMIC_RELEASE);
            nd_thread_join(unmark_thread);
            break;
        }

        // Hook is armed for UNMARK_AFTER_CAS - freez's FREEZ_BEFORE_CLAIM
        // pause point will not match, so the freez we are about to spawn
        // proceeds without pausing.

        // Snapshot the UNMARKING-cold-path counter before spawning freez.
        size_t cold_before = __atomic_load_n(&aral_freez_unmarking_observed_count, __ATOMIC_ACQUIRE);

        ND_THREAD *freez_thread = nd_thread_create("FREEZ", NETDATA_THREAD_OPTION_DONT_LOG,
                                                   aral_concurrency_test_freez_thread, &ctx);
        if(!freez_thread) {
            fprintf(stderr, "    iter %zu: failed to create freez thread\n", i);
            errors++;
            __atomic_store_n(&aral_concurrency_race_hook.release, true, __ATOMIC_RELEASE);
            nd_thread_join(unmark_thread);
            break;
        }

        // Wait until freez actually enters the UNMARKING cold path, i.e. its
        // exchange returned a value with the UNMARKING bit set. Without this
        // synchronization the test could release unmark before freez has
        // even reached the exchange, never exercising the cold path we are
        // here to validate.
        usec_t deadline = now_monotonic_usec() + 5 * USEC_PER_SEC;
        while(__atomic_load_n(&aral_freez_unmarking_observed_count, __ATOMIC_ACQUIRE) == cold_before) {
            if(now_monotonic_usec() > deadline) {
                fprintf(stderr, "    iter %zu: freez did not observe UNMARKING within timeout\n", i);
                errors++;
                break;
            }
            tinysleep();
        }
        if(__atomic_load_n(&aral_freez_unmarking_observed_count, __ATOMIC_ACQUIRE) > cold_before)
            cold_path_hits++;

        // Now release unmark; freez (still spinning in its cold path) will
        // observe the published UNMARKED and proceed.
        __atomic_store_n(&aral_concurrency_race_hook.release, true, __ATOMIC_RELEASE);

        nd_thread_join(unmark_thread);
        nd_thread_join(freez_thread);

        // Reset the hook for the next iteration.
        aral_concurrency_race_hook_reset();
    }

    // Defensive reset: any early break above could have left the hook armed.
    aral_concurrency_race_hook_reset();

    // Free pool entries that were not freed by a successful loop iteration.
    // On normal completion i == pool_size and this loop is empty. On early
    // break, pool[i] may be in any allocated state (MARKED or UNMARKED) but
    // is always still allocated; aral_freez handles both.
    for(size_t j = i; j < pool_size; j++)
        aral_freez(ar, pool[j]);

    if(cold_path_hits != pool_size) {
        fprintf(stderr, "    cold path was exercised %zu/%zu times (expected all)\n",
                cold_path_hits, pool_size);
        errors++;
    }

    if(aral_used_bytes(ar) != 0) {
        fprintf(stderr, "    aral has %zu used bytes after stress test (expected 0)\n",
                aral_used_bytes(ar));
        errors++;
    }

    freez(pool);
    aral_destroy(ar);
    return errors;
}

int aral_unittest_concurrency(void) {
#if defined(FSANITIZE_ADDRESS)
    // Under address sanitizer ARAL is bypassed entirely: mallocz/callocz/
    // freez delegate straight to glibc and aral_unmark_allocation() is a
    // no-op. There is no trailer protocol to test, so skip cleanly.
    fprintf(stderr, "ARAL concurrency tests: SKIPPED (ARAL is disabled under FSANITIZE_ADDRESS)\n");
    return 0;
#else
    fprintf(stderr, "Running ARAL concurrency tests (unmark/freez state machine)...\n");
    int errors = 0;
    errors += aral_concurrency_test_clean_unmark();
    errors += aral_concurrency_test_unmark_on_unmarked();
    errors += aral_concurrency_test_clean_freez_marked();
    errors += aral_concurrency_test_freez_wins();
    errors += aral_concurrency_test_unmark_wins_transition();
    errors += aral_concurrency_test_unmark_wins_no_grace();
    errors += aral_concurrency_test_unmark_last_marked_on_page();
    errors += aral_concurrency_test_stress();
    fprintf(stderr, "ARAL concurrency tests: %s (%d errors)\n",
            errors ? "FAILED" : "PASSED", errors);
    return errors;
#endif
}

#endif

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

            size_t *free_list = mallocz(to_free * sizeof(*free_list));
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

            freez(free_list);
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
    ND_THREAD **thread_ptrs = callocz(threads, sizeof(*thread_ptrs));

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

    freez(thread_ptrs);

    usec_t ended_ut = now_monotonic_usec();

    if (auc.ar->aral_lock.pages_free && auc.ar->aral_lock.pages_free->page_lock.used_elements) {
        fprintf(stderr, "\n\nARAL leftovers detected (3)\n\n");
        __atomic_add_fetch(&auc.errors, 1, __ATOMIC_RELAXED);
    }

    fprintf(stderr, "ARAL: did %zu malloc, %zu free, using %zu threads, in %"PRIu64" usecs\n",
            __atomic_load_n(&auc.ar->atomic.user_malloc_operations, __ATOMIC_RELAXED),
            __atomic_load_n(&auc.ar->atomic.user_free_operations, __ATOMIC_RELAXED),
            threads,
            ended_ut - started_ut);

    aral_destroy(auc.ar);

    return auc.errors;
}

int aral_unittest(size_t elements) {
    const char *cache_dir = "/tmp/";
#ifdef NETDATA_INTERNAL_CHECKS
    int errors = aral_detect_acquire_to_page_lock_race();

    if(errors) {
        fprintf(stderr, "ARAL unittest: FAILED (%d errors)\n", errors);
        return errors;
    }
#else
    int errors = 0;
#endif

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

    errors += aral_stress_test(2, elements, 10);

    int total_errors = auc.errors + errors;
    fprintf(stderr, "ARAL unittest: %s (%d errors)\n", total_errors ? "FAILED" : "PASSED", total_errors);

    return total_errors;
}
