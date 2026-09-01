// SPDX-License-Identifier: GPL-3.0-or-later

#include "page.h"

#include "libnetdata/libnetdata.h"

typedef enum __attribute__((packed)) {
    PAGE_OPTION_ALL_VALUES_EMPTY    = (1 << 0),
    PAGE_OPTION_ARAL_MARKED         = (1 << 1),
    PAGE_OPTION_ARAL_UNMARKED       = (1 << 2),
} PAGE_OPTIONS;

typedef enum __attribute__((packed)) {
    PGD_STATE_CREATED_FROM_COLLECTOR        = (1 << 0),
    PGD_STATE_CREATED_FROM_DISK             = (1 << 1),
    PGD_STATE_SCHEDULED_FOR_FLUSHING        = (1 << 2),
    PGD_STATE_FLUSHED_TO_DISK               = (1 << 3),
} PGD_STATES;

typedef struct {
    uint8_t *data;
    uint16_t size;
    uint16_t freed_mark;        // PGD lifetime guard - see below
} page_raw_t;

typedef struct {
    gorilla_writer_t *writer;
    uint16_t num_buffers;
    uint16_t freed_mark;        // PGD lifetime guard - same offset as page_raw_t's
} page_gorilla_t;

struct pgd {
    // the used number of slots in the page
    uint16_t used;

    // the total number of slots available in the page
    uint16_t slots;

    // the page type
    uint8_t type;

    // the partition this pgd was allocated from
    uint8_t partition;

    // options related to the page
    PAGE_OPTIONS options;

    PGD_STATES states;

    union {
        page_raw_t raw;
        page_gorilla_t gorilla;
    };
};

// ----------------------------------------------------------------------------
// PGD lifetime guard
//
// aral_freez() writes ARAL_FREE { size_t size; struct aral_free *next; } over the FIRST
// 16 BYTES of the element it frees (src/libnetdata/aral/aral.c). On a 24-byte PGD that
// clobbers used, slots, type, partition, options, states and raw.data, and leaves bytes
// 16..23 untouched. The clobbered values are not random: type reads back as 0, which is
// RRDENG_PAGE_TYPE_ARRAY_32BIT, and partition reads back as 0.
//
// So a second pgd_free() on an already freed PGD passes every check it has, takes the
// ARRAY branch even when the PGD was a GORILLA page, and calls
//
//     aral_freez(pgd-<stale raw.size>-0, <the free-list 'next' pointer>)
//
// which either fatals inside ARAL naming an arena and a pointer that have nothing to do
// with the bug, or - when 'next' happens to be NULL - corrupts state silently, because
// aral_freez(ar, NULL) is a no-op.
//
// We therefore mark a PGD as freed in the 8 bytes ARAL provably cannot reach, and check
// that mark on entry. Same approach as SPINLOCK_TRACKED: record the actor inline in the
// object's own memory, in production, on one narrowly scoped hot object.
//
// Under FSANITIZE_ADDRESS the pool is bypassed (aral_freez() delegates to freez()), so
// the mark is never read back and ASAN catches the double free directly.

// The guard has to survive what aral_freez() writes into a freed element:
// ARAL_FREE { size_t size; struct aral_free *next; } - two pointer-sized words at offset
// 0, on every ABI. The asserts below are what actually enforce that.
//
// freed_mark is a declared member of BOTH union arms at the same offset, not punned
// padding, and it is a uint16 so it fits in padding those arms already had - so
// sizeof(struct pgd) is unchanged on LP64 (24) and on ILP32 (16), and PGD does not move
// aral size class on either. A member appended after the union instead would have cost
// +8 bytes per PGD on LP64, because sizeof(page_raw_t) is already pointer-padded to 16.
//
// The ABIs differ, and it matters:
//
//   LP64  ARAL_FREE is 16 bytes and covers used/slots/type/partition/options/states AND
//         raw.data. type reads back as 0 == RRDENG_PAGE_TYPE_ARRAY_32BIT and partition as
//         0, so a second pgd_free() takes the ARRAY branch even for a GORILLA page and
//         frees the free-list `next` pointer into the arena picked by the stale raw.size.
//   ILP32 ARAL_FREE is 8 bytes and covers only used/slots/type/partition/options/states.
//         raw.data SURVIVES, so a second pgd_free() frees the real data buffer again, and
//         partition is a byte of the `next` pointer - i.e. an out-of-range index into
//         aral_pgd[] once internal_fatal() is compiled out.
//
// Either way the resulting fatal describes the wrong thing, which is why this guard exists.
// 8 bits of magic is enough: freed_mark is a declared member that nothing but this guard
// ever writes, so it only has to tell "cleared by pgd_alloc()" from "freed". The low byte
// carries the PGD_FREE_SITE.
#define PGD_FREED_MAGIC      0x9D00U
#define PGD_FREED_MAGIC_MASK 0xFF00U

_Static_assert(offsetof(struct pgd, raw.freed_mark) >= 2 * sizeof(void *),
               "freed_mark must sit past ARAL_FREE{size_t,pointer}, which aral_freez() "
               "writes over the start of every freed element");
_Static_assert(offsetof(struct pgd, raw.freed_mark) == offsetof(struct pgd, gorilla.freed_mark),
               "both union arms must place freed_mark at the same offset");
_Static_assert(offsetof(struct pgd, raw.freed_mark) >= offsetof(struct pgd, raw.size) + sizeof(((page_raw_t *)0)->size),
               "freed_mark must not overlay page_raw_t.size");
_Static_assert(offsetof(struct pgd, gorilla.freed_mark) >= offsetof(struct pgd, gorilla.num_buffers) + sizeof(((page_gorilla_t *)0)->num_buffers),
               "freed_mark must not overlay page_gorilla_t.num_buffers");

#if UINTPTR_MAX == 0xFFFFFFFFFFFFFFFFu
// The guard must stay free on the platform that carries the fleet: it has to fit in
// padding struct pgd already had, so PGD does not change aral size class.
_Static_assert(sizeof(PGD) == 24, "struct pgd grew on LP64 - the freed mark is no longer free");
#elif UINTPTR_MAX == 0xFFFFFFFFu
// Same requirement on ILP32, where the comments above claim 16 - assert it instead of
// only documenting it, so a layout change cannot move PGD's aral size class unnoticed.
_Static_assert(sizeof(PGD) == 16, "struct pgd grew on ILP32 - the freed mark is no longer free");
#endif

static ALWAYS_INLINE bool pgd_mark_is_freed(uint16_t mark) {
    return (mark & PGD_FREED_MAGIC_MASK) == PGD_FREED_MAGIC;
}

static const char *pgd_free_site_name(PGD_FREE_SITE site) {
    switch(site) {
        case PGD_FREE_SITE_MAIN_CACHE_EVICT:     return "main cache eviction (main_cache_free_clean_page_callback)";
        case PGD_FREE_SITE_EXTENT_INSERT_LOST:   return "extent population, lost the main cache insert race";
        case PGD_FREE_SITE_DISK_GORILLA_INVALID: return "invalid gorilla chain loaded from disk";
        case PGD_FREE_SITE_CREATE_BAD_TYPE:      return "unknown page type while creating";
        case PGD_FREE_SITE_UNITTEST:             return "unit test teardown";
        default:                                 return "unknown";
    }
}

// ----------------------------------------------------------------------------
// PGD use-after-free guard
//
// The double-free check above only fires on the SECOND pgd_free(). It says nothing about
// the far more common shape in the fleet: a PGD that is freed once, and then USED - by a
// collector still appending points to it, or by the flush path still reading it. That is
// what produces 21 of the 25 pgd_fatal events / 30 days, and it produces them with the
// wrong story, because every field pgd_fatal prints has already been overwritten by ARAL's
// free-list header:
//
//     collection on page not created from a collector - pgd: { type: ARRAY_32BIT, used: 0,
//     slots: 0, partition: 0, state: 0, options: 0 }
//
// Nothing in that line is this PGD's. `state: 0` is why the "not created from a collector"
// check tripped, and `type/used/slots/partition` are ARAL_FREE{size_t,pointer}. Reading it
// as a page-state bug sends the investigation to the collector, not to the freer.
//
// So: check the mark BEFORE any other field is read, and name the site that freed it. Same
// mark, same cost as the double-free check - one 16-bit relaxed load on a cacheline the
// caller is about to touch anyway.
//
// Guarded: pgd_append_point(), pgd_disk_footprint(), pgd_buffer_memory_footprint(),
// pgd_copy_to_extent(), pgdc_reset(). pgd_fatal() reports the verdict for everything else.
// NOT guarded, deliberately: pgd_type(), pgd_is_empty(), pgd_slots_used(), pgd_capacity(),
// pgd_memory_footprint() and pgdc_get_next_point(). The first five only read scalars that
// pgd_fatal() already labels; the last is per-point in the query hot path, and pgdc_reset()
// guards the cursor at setup. A freed PGD reaching pgdc_get_next_point() still derefs
// raw.data (== ARAL's free-list `next` on LP64) and SIGSEGVs without a story - a known gap,
// not a regression: nothing here removes a check that existed before.
//
// Two more boundaries this detector cannot cross, both inherent to an in-object mark:
//
//   Recycling (ABA). pgd_alloc() clears the mark, so once ARAL hands the slot to a new PGD
//   a stale pointer to the old one reads as live and the operation silently proceeds
//   against the new page. The mark catches a use-after-free only while the slot is still
//   free; it cannot catch one after reuse.
//
//   Unmapping. aral_freez_internal() deletes a fully-freed ARAL page at runtime when
//   another page still has free slots (aral.c:1265 -> aral_del_page___no_lock_needed() ->
//   nd_munmap()/freez()). If that happens between the free and the guarded use, the mark
//   load itself faults. Strictly no worse than before - the old code dereferenced the same
//   unmapped page - but "fatal with a story" is best-effort, not a guarantee.
//
// On exit: this check does NOT honour exit_initiated_get(), unlike its neighbours in
// pgd_append_point(). Be precise about what that trades, because it differs by ABI. `states`
// sits at offset 7 of struct pgd, and ARAL_FREE{size_t,pointer} covers bytes 0..15 on LP64
// and 0..7 on ILP32:
//
//   LP64  offset 7 is the TOP byte of ARAL_FREE.size, so it always reads 0 for any real
//         element size. The old code therefore always reached the
//         !(states & CREATED_FROM_COLLECTOR) check below and returned 0 during exit - it
//         never wrote into the freed PGD. Here we trade a clean shutdown for detection.
//   ILP32 offset 7 is the TOP byte of ARAL_FREE.next, which is not reliably 0. The old code
//         could pass the state checks and write into freed memory. Here we prevent that.
//
// So the honest claim is narrower than "writing into freed memory cannot be a race": on the
// ABI that carries the fleet this converts a silent, harmless exit into a reported fatal.
// That is the intended trade - a latent eviction/collector lifetime race is worth a crash
// event - but it is a deliberate behaviour change, not a free win.

static void pgd_fatal_use_after_free(const PGD *pg, uint16_t mark, const char *what, const char *func) {
    fatal("DBENGINE: PGD use after free - %s on pgd %p in '%s', but that PGD was already "
          "freed by: %s. Do not trust any other field of it: its first %zu bytes now hold "
          "ARAL's free-list header, which is why it reports type %u, partition %u, used %u, "
          "slots %u, states %u.",
          what, (const void *)pg, func ? func : "(unknown)",
          pgd_free_site_name((PGD_FREE_SITE)(mark & 0xFFU)),
          (size_t)(2 * sizeof(void *)),
          (unsigned)pg->type, (unsigned)pg->partition,
          (unsigned)pg->used, (unsigned)pg->slots, (unsigned)pg->states);
}

// Fatals if pg has already been freed; returns normally otherwise. NULL and PGD_EMPTY are
// neither freed nor dereferenceable, so callers that already tolerate them keep doing so.
#define pgd_check_alive(pg, what) pgd_check_alive_with_trace(pg, what, __FUNCTION__)
static ALWAYS_INLINE void pgd_check_alive_with_trace(const PGD *pg, const char *what, const char *func) {
    if(!pg || pg == PGD_EMPTY)
        return;

    uint16_t mark = __atomic_load_n(&pg->raw.freed_mark, __ATOMIC_RELAXED);
    if(unlikely(pgd_mark_is_freed(mark)))
        pgd_fatal_use_after_free(pg, mark, what, func);
}

// Claim the PGD for freeing, and return what was there before.
//
// This is an exchange, taken on ENTRY to pgd_free(), not a store on the way out. Two
// threads racing to free the same PGD would both pass a check-then-set guard and both run
// the teardown; with an exchange exactly one wins, and the loser gets the winner's mark
// back and reports it.
static ALWAYS_INLINE uint16_t pgd_claim_freed(PGD *pg, PGD_FREE_SITE site) {
    return __atomic_exchange_n(&pg->raw.freed_mark,
                               (uint16_t)(PGD_FREED_MAGIC | ((uint16_t)site & 0xFFU)),
                               __ATOMIC_ACQ_REL);
}

// aral does not zero recycled slots, so a PGD handed back out still carries the mark of
// its previous life. Clearing it on allocation is what keeps the guard free of false
// positives.
static ALWAYS_INLINE void pgd_clear_freed(PGD *pg) {
    __atomic_store_n(&pg->raw.freed_mark, 0, __ATOMIC_RELAXED);
}

static PRINTFLIKE(2, 3) void pgd_fatal(const PGD *pg, const char *fmt, ...) {
    BUFFER *wb = buffer_create(0, NULL);

    va_list args;
    va_start(args, fmt);
    buffer_vsprintf(wb, fmt, args);
    va_end(args);

    buffer_strcat(wb, " - pgd: { ");

    {
        buffer_strcat(wb, "type: ");
        bool added = false;

        if (pg->type == RRDENG_PAGE_TYPE_ARRAY_32BIT) {
            buffer_sprintf(wb, "%s", "ARRAY_32BIT");
            added = true;
        }

        if (pg->type == RRDENG_PAGE_TYPE_ARRAY_TIER1) {
            buffer_sprintf(wb, added ? "|%s" : "%s", "ARRAY_TIER1");
            added = true;
        }

        if (pg->type == RRDENG_PAGE_TYPE_GORILLA_32BIT) {
            buffer_sprintf(wb, added ? "|%s" : "%s", "GORILLA_32BIT");
            added = true;
        }

        if (!added) {
            int type = pg->type;
            buffer_sprintf(wb, "%d", type);
        }
    }

    {
        int used = pg->used;
        int slots = pg->slots;
        int partition = pg->partition;
        buffer_sprintf(wb, ", used: %d, slots: %d, partition: %d", used, slots, partition);
    }

    {
        buffer_strcat(wb, ", state: ");
        bool added = false;

        if (pg->states == PGD_STATE_CREATED_FROM_COLLECTOR) {
            buffer_sprintf(wb, "%s", "CREATED_FROM_COLLECTOR");
            added = true;
        }

        if (pg->states == PGD_STATE_CREATED_FROM_DISK) {
            buffer_sprintf(wb, added ? "|%s" : "%s", "CREATED_FROM_DISK");
            added = true;
        }

        if (pg->states == PGD_STATE_SCHEDULED_FOR_FLUSHING) {
            buffer_sprintf(wb, added ? "|%s" : "%s", "SCHEDULED_FOR_FLUSHING");
            added = true;
        }

        if (pg->states == PGD_STATE_FLUSHED_TO_DISK) {
            buffer_sprintf(wb, added ? "|%s" : "%s", "FLUSHED_TO_DISK");
            added = true;
        }

        if (!added) {
            int state = pg->states;
            buffer_sprintf(wb, "%d", state);
        }
    }

    {
        buffer_strcat(wb, ", options: ");
        bool added = false;

        if (pg->options & PAGE_OPTION_ALL_VALUES_EMPTY) {
            buffer_sprintf(wb, "%s", "ALL_VALUES_EMPTY");
            added = true;
        }

        if (pg->options & PAGE_OPTION_ARAL_MARKED) {
            buffer_sprintf(wb, added ? "|%s" : "%s", "ARAL_MARKED");
            added = true;
        }

        if (pg->options & PAGE_OPTION_ARAL_UNMARKED) {
            buffer_sprintf(wb, added ? "|%s" : "%s", "ARAL_UNMARKED");
            added = true;
        }

        if (!added) {
            int options = pg->options;
            buffer_sprintf(wb, "%d", options);
        }
    }

    {
        // The lifetime verdict has to come last and be unmissable: if this PGD is already
        // freed, every field printed above belongs to ARAL's free-list header, not to it.
        uint16_t mark = __atomic_load_n(&pg->raw.freed_mark, __ATOMIC_RELAXED);
        if(unlikely(pgd_mark_is_freed(mark)))
            buffer_sprintf(wb, ", lifetime: ALREADY FREED BY '%s' - IGNORE EVERY FIELD "
                               "ABOVE, they are ARAL's free-list header",
                           pgd_free_site_name((PGD_FREE_SITE)(mark & 0xFFU)));
        else
            buffer_strcat(wb, ", lifetime: live");
    }

    buffer_strcat(wb, " }");

    fatal("%s", buffer_tostring(wb));
}

// ----------------------------------------------------------------------------
// memory management

// deduplicate aral sizes, if the delta is below this number of bytes
#define ARAL_TOLERANCE_TO_DEDUP 7

// max, we use as many as the cpu cores
// cannot be bigger than 256, due to struct pgd->partition (uint8_t)
#define PGD_ARAL_PARTITIONS_MAX 256

struct {
    int64_t padding_used;
    size_t partitions;

    size_t sizeof_pgd;
    size_t sizeof_gorilla_writer_t;
    size_t sizeof_gorilla_buffer_32bit;

    ARAL *aral_pgd[PGD_ARAL_PARTITIONS_MAX];
    ARAL *aral_gorilla_buffer[PGD_ARAL_PARTITIONS_MAX];
    ARAL *aral_gorilla_writer[PGD_ARAL_PARTITIONS_MAX];
} pgd_alloc_globals = { 0 };

#if RRD_STORAGE_TIERS != 5
#error "You need to update the slots reserved for storage tiers"
#endif

static struct aral_statistics pgd_aral_statistics = { 0 };

static size_t aral_sizes_delta;
static size_t aral_sizes_count;
static size_t aral_sizes[] = {
//    // leave space for the storage tier page sizes
    [RRD_STORAGE_TIERS - 5] = 0,
    [RRD_STORAGE_TIERS - 4] = 0,
    [RRD_STORAGE_TIERS - 3] = 0,
    [RRD_STORAGE_TIERS - 2] = 0,
    [RRD_STORAGE_TIERS - 1] = 0,

    // gorilla buffer sizes
    RRDENG_GORILLA_32BIT_BUFFER_SIZE,
    RRDENG_GORILLA_32BIT_BUFFER_SIZE * 2,
    RRDENG_GORILLA_32BIT_BUFFER_SIZE * 3,
    RRDENG_GORILLA_32BIT_BUFFER_SIZE * 4,

    // our structures
    sizeof(gorilla_writer_t),
    sizeof(PGD),

    // per 512B
    512, 1024, 1536, 2048, 5 * 512, 6 * 512, 7 * 512, 8 * 512, /* 9 * 512, */

    // per 1KiB
//    5 * 1024, 6 * 1024, 7 * 1024, 8 * 1024, 9 * 1024, 10 * 1024, 11 * 1024,
//    12 * 1024, 13 * 1024, 14 * 1024, 15 * 1024, 16 * 1024, 17 * 1024, 18 * 1024,
//    19 * 1024, 20 * 1024, 21 * 1024, 22 * 1024, 23 * 1024, 24 * 1024, 25 * 1024,
//    26 * 1024, 27 * 1024, 28 * 1024, 29 * 1024, 30 * 1024, 31 * 1024, 32 * 1024,

    // test to see if 4KiB has less overheads than 1KiB
    8 * 1024, 12 * 1024, 16 * 1024, 20 * 1024, 24 * 1024, 28 * 1024, 32 * 1024,

    // per 4KiB
    36 * 1024, 40 * 1024, 44 * 1024, 48 * 1024, 52 * 1024, 56 * 1024, 60 * 1024,
    64 * 1024, 68 * 1024, 72 * 1024, 76 * 1024, 80 * 1024, 84 * 1024, 88 * 1024,
    92 * 1024, 96 * 1024, 100 * 1024, 104 * 1024, 108 * 1024, 112 * 1024, 116 * 1024,
    120 * 1024, 124 * 1024, 128 * 1024,
};
static ARAL **arals = NULL;

#define arals_slot(slot, partition) ((partition) * aral_sizes_count + (slot))
static ARAL *pgd_get_aral_by_size_and_partition(size_t size, size_t partition);

size_t pgd_padding_bytes(void) {
    int64_t x = __atomic_load_n(&pgd_alloc_globals.padding_used, __ATOMIC_RELAXED);
    return (x > 0) ? x : 0;
}

struct aral_statistics *pgd_aral_stats(void) {
    return &pgd_aral_statistics;
}

int aral_size_sort_compare(const void *a, const void *b) {
    size_t size_a = *(const size_t *)a;
    size_t size_b = *(const size_t *)b;
    return (size_a > size_b) - (size_a < size_b);
}

void pgd_init_arals(void) {
    size_t partitions = netdata_conf_cpus();
    if(partitions < 4) partitions = 4;
    if(partitions > PGD_ARAL_PARTITIONS_MAX) partitions = PGD_ARAL_PARTITIONS_MAX;
    pgd_alloc_globals.partitions = partitions;

    aral_sizes_count = _countof(aral_sizes);

    for(size_t i = 0; i < RRD_STORAGE_TIERS ;i++)
        aral_sizes[i] = tier_page_size[i];

    if(!netdata_conf_is_parent()) {
        // this agent is not a parent
        // do not use ARAL for sizes above 4KiB
        for(size_t i = RRD_STORAGE_TIERS ; i < _countof(aral_sizes) ;i++) {
            if(aral_sizes[i] > 4096)
                aral_sizes[i] = 0;
        }
    }

    size_t max_delta = 0;
    for(size_t i = 0; i < aral_sizes_count ;i++) {
        size_t wanted = aral_sizes[i];
        size_t usable = aral_sizes[i]; /* aral_allocation_slot_size(wanted, true);*/
        internal_fatal(usable < wanted, "usable cannot be less than wanted");
        if(usable > wanted && usable - wanted > max_delta)
            max_delta = usable - wanted;

        aral_sizes[i] = usable;
    }
    aral_sizes_delta = max_delta + ARAL_TOLERANCE_TO_DEDUP;

    // sort the array
    qsort(aral_sizes, aral_sizes_count, sizeof(size_t), aral_size_sort_compare);

    // deduplicate (with some tolerance)
    size_t unique_count = 1;
    for (size_t i = 1; i < aral_sizes_count; ++i) {
        if (aral_sizes[i] > aral_sizes[unique_count - 1] + aral_sizes_delta)
            aral_sizes[unique_count++] = aral_sizes[i];
        else
            aral_sizes[unique_count - 1] = aral_sizes[i];
    }
    aral_sizes_count = unique_count;

    // clear the rest
    for(size_t i = unique_count; i < _countof(aral_sizes) ;i++)
        aral_sizes[i] = 0;

    // allocate all the arals
    arals = callocz(aral_sizes_count * pgd_alloc_globals.partitions, sizeof(ARAL *));
    for(size_t slot = 0; slot < aral_sizes_count ; slot++) {
        for(size_t partition = 0; partition < pgd_alloc_globals.partitions; partition++) {

            if(partition > 0 && aral_sizes[slot] > 128) {
                // do not create partitions for sizes above 128 bytes
                // use the first partition for all of them
                arals[arals_slot(slot, partition)] = arals[arals_slot(slot, 0)];
                continue;
            }

            char buf[32];
            snprintfz(buf, sizeof(buf), "pgd-%zu-%zu", aral_sizes[slot], partition);

            arals[arals_slot(slot, partition)] = aral_create(
                buf,
                aral_sizes[slot],
                0,
                0,
                &pgd_aral_statistics,
                NULL, NULL, false, false, true);
        }
    }

    for(size_t p = 0; p < pgd_alloc_globals.partitions ;p++) {
        pgd_alloc_globals.aral_pgd[p] = pgd_get_aral_by_size_and_partition(sizeof(PGD), p);
        pgd_alloc_globals.aral_gorilla_writer[p] = pgd_get_aral_by_size_and_partition(sizeof(gorilla_writer_t), p);
        pgd_alloc_globals.aral_gorilla_buffer[p] = pgd_get_aral_by_size_and_partition(RRDENG_GORILLA_32BIT_BUFFER_SIZE, p);

        internal_fatal(!pgd_alloc_globals.aral_pgd[p] ||
                       !pgd_alloc_globals.aral_gorilla_writer[p] ||
                       !pgd_alloc_globals.aral_gorilla_buffer[p]
                       , "required PGD aral sizes not found");
    }

    pgd_alloc_globals.sizeof_pgd = aral_actual_element_size(pgd_alloc_globals.aral_pgd[0]);
    pgd_alloc_globals.sizeof_gorilla_writer_t = aral_actual_element_size(pgd_alloc_globals.aral_gorilla_writer[0]);
    pgd_alloc_globals.sizeof_gorilla_buffer_32bit = aral_actual_element_size(pgd_alloc_globals.aral_gorilla_buffer[0]);

    pulse_aral_register_statistics(&pgd_aral_statistics, "pgd");
}

static ARAL *pgd_get_aral_by_size_and_partition(size_t size, size_t partition) {
    internal_fatal(partition >= pgd_alloc_globals.partitions, "Wrong partition %zu", partition);

    size_t slot;

    if (size <= aral_sizes[0])
        slot = 0;

    else if (size > aral_sizes[aral_sizes_count - 1])
        return NULL;

    else {
        // binary search for the smallest size >= requested size
        size_t low = 0, high = aral_sizes_count - 1;
        while (low < high) {
            size_t mid = low + (high - low) / 2;
            if (aral_sizes[mid] >= size)
                high = mid;
            else
                low = mid + 1;
        }
        slot = low; // This is the smallest index where aral_sizes[slot] >= size
    }
    internal_fatal(slot >= aral_sizes_count || aral_sizes[slot] < size, "Invalid PGD size binary search");

    ARAL *ar = arals[arals_slot(slot, partition)];
    internal_fatal(!ar || aral_requested_element_size(ar) < size, "Invalid PGD aral lookup");
    return ar;
}

static ALWAYS_INLINE gorilla_writer_t *pgd_gorilla_writer_alloc(size_t partition) {
    internal_fatal(partition >= pgd_alloc_globals.partitions, "invalid gorilla writer partition %zu", partition);
    return aral_mallocz_marked(pgd_alloc_globals.aral_gorilla_writer[partition]);
}

static ALWAYS_INLINE gorilla_buffer_t *pgd_gorilla_buffer_alloc(size_t partition) {
    internal_fatal(partition >= pgd_alloc_globals.partitions, "invalid gorilla buffer partition %zu", partition);
    return aral_mallocz_marked(pgd_alloc_globals.aral_gorilla_buffer[partition]);
}

static ALWAYS_INLINE PGD *pgd_alloc(bool for_collector) {
    size_t partition = gettid_cached() % pgd_alloc_globals.partitions;
    PGD *pgd;

    if(for_collector)
        pgd = aral_mallocz_marked(pgd_alloc_globals.aral_pgd[partition]);
    else
        pgd = aral_mallocz(pgd_alloc_globals.aral_pgd[partition]);

    pgd->partition = partition;

    // the slot may be recycled from a previously freed PGD, which still carries its mark
    pgd_clear_freed(pgd);

    return pgd;
}

static ALWAYS_INLINE void *pgd_data_alloc(size_t size, size_t partition, bool for_collector) {
    ARAL *ar = pgd_get_aral_by_size_and_partition(size, partition);
    if(ar) {
        int64_t padding = (int64_t)aral_requested_element_size(ar) - (int64_t)size;
        __atomic_add_fetch(&pgd_alloc_globals.padding_used, padding, __ATOMIC_RELAXED);

        if(for_collector)
            return aral_mallocz_marked(ar);
        else
            return aral_mallocz(ar);
    }
    else
        return mallocz(size);
}

static ALWAYS_INLINE void pgd_data_free(void *page, size_t size, size_t partition) {
    ARAL *ar = pgd_get_aral_by_size_and_partition(size, partition);
    if(ar) {
        int64_t padding = (int64_t)aral_requested_element_size(ar) - (int64_t)size;
        __atomic_sub_fetch(&pgd_alloc_globals.padding_used, padding, __ATOMIC_RELAXED);

        aral_freez(ar, page);
    }
    else
        freez(page);
    timing_dbengine_evict_step(TIMING_STEP_DBENGINE_EVICT_FREE_MAIN_PGD_TIER1_ARAL);
}

static ALWAYS_INLINE void pgd_data_unmark(void *page, size_t size, size_t partition) {
    if(!page) return;

    ARAL *ar = pgd_get_aral_by_size_and_partition(size, partition);
    if(ar)
        aral_unmark_allocation(ar, page);
}

static size_t pgd_data_footprint(size_t size, size_t partition) {
    ARAL *ar = pgd_get_aral_by_size_and_partition(size, partition);
    if(ar)
        return aral_actual_element_size(ar);
    else
        return size;
}

// ----------------------------------------------------------------------------

ALWAYS_INLINE void *dbengine_extent_alloc(size_t size) {
    return pgd_data_alloc(size, 0, false);
}

ALWAYS_INLINE void dbengine_extent_free(void *extent, size_t size) {
    pgd_data_free(extent, size, 0);
}

// ----------------------------------------------------------------------------
// management api

ALWAYS_INLINE PGD *pgd_create(uint8_t type, uint32_t slots) {

    PGD *pg = pgd_alloc(true); // this is malloc'd !
    pg->type = type;
    pg->states = PGD_STATE_CREATED_FROM_COLLECTOR;
    pg->options = PAGE_OPTION_ALL_VALUES_EMPTY | PAGE_OPTION_ARAL_MARKED;

    pg->used = 0;
    pg->slots = slots;

    switch (type) {
        case RRDENG_PAGE_TYPE_GORILLA_32BIT: {
            internal_fatal(slots == 1,
                      "DBENGINE: invalid number of slots (%u) or page type (%u)", slots, type);

            // allocate new gorilla writer
            pg->gorilla.writer = pgd_gorilla_writer_alloc(pg->partition);

            // allocate new gorilla buffer
            gorilla_buffer_t *gbuf = pgd_gorilla_buffer_alloc(pg->partition);
            memset(gbuf, 0, RRDENG_GORILLA_32BIT_BUFFER_SIZE);
            pulse_gorilla_hot_buffer_added();

            *pg->gorilla.writer = gorilla_writer_init(gbuf, RRDENG_GORILLA_32BIT_BUFFER_SLOTS);
            pg->gorilla.num_buffers = 1;

            break;
        }

        case RRDENG_PAGE_TYPE_ARRAY_32BIT:
        case RRDENG_PAGE_TYPE_ARRAY_TIER1: {
            uint32_t size = slots * page_type_size[type];

            internal_fatal(!size || slots == 1,
                           "DBENGINE: invalid number of slots (%u) or page type (%u)", slots, type);

            pg->raw.size = size;
            pg->raw.data = pgd_data_alloc(size, pg->partition, true);
            break;
        }

        default:
            netdata_log_error("%s() - Unknown page type: %uc", __FUNCTION__, type);
            pgd_claim_freed(pg, PGD_FREE_SITE_CREATE_BAD_TYPE);
            aral_freez(pgd_alloc_globals.aral_pgd[pg->partition], pg);
            pg = PGD_EMPTY;
            break;
    }

    return pg;
}

ALWAYS_INLINE PGD *pgd_create_from_disk_data(uint8_t type, void *base, uint32_t size) {

    if (!size || size < page_type_size[type])
        return PGD_EMPTY;

    PGD *pg = pgd_alloc(false); // this is malloc'd !
    pg->type = type;
    pg->states = PGD_STATE_CREATED_FROM_DISK;
    pg->options = PAGE_OPTION_ARAL_UNMARKED;

    switch (type)
    {
        case RRDENG_PAGE_TYPE_GORILLA_32BIT:
            internal_fatal(size == 0, "Asked to create page with 0 data!!!");
            internal_fatal(size % sizeof(uint32_t), "Unaligned gorilla buffer size");
            internal_fatal(size % RRDENG_GORILLA_32BIT_BUFFER_SIZE, "Expected size to be a multiple of %zu-bytes",
                RRDENG_GORILLA_32BIT_BUFFER_SIZE);

            pg->raw.data = (void *)pgd_data_alloc(size, pg->partition, false);
            pg->raw.size = size;

            memcpy(pg->raw.data, base, pg->raw.size);

            uint32_t total_entries = 0;
            if(unlikely(!gorilla_buffer_patch((void *) pg->raw.data,
                                              size / RRDENG_GORILLA_32BIT_BUFFER_SIZE,
                                              &total_entries))) {
                netdata_log_error("DBENGINE: invalid gorilla disk page chain.");
                pgd_free(pg, PGD_FREE_SITE_DISK_GORILLA_INVALID);
                pg = PGD_EMPTY;
                break;
            }
            pg->used = total_entries;
            pg->slots = pg->used;
            break;

        case RRDENG_PAGE_TYPE_ARRAY_32BIT:
        case RRDENG_PAGE_TYPE_ARRAY_TIER1:
            pg->used = size / page_type_size[type];
            pg->slots = pg->used;

            pg->raw.size = size;
            pg->raw.data = pgd_data_alloc(size, pg->partition, false);
            memcpy(pg->raw.data, base, size);
            break;

        default:
            netdata_log_error("%s() - Unknown page type: %uc", __FUNCTION__, type);
            pgd_claim_freed(pg, PGD_FREE_SITE_CREATE_BAD_TYPE);
            aral_freez(pgd_alloc_globals.aral_pgd[pg->partition], pg);
            pg = PGD_EMPTY;
            break;
    }

    return pg;
}

void pgd_free_with_trace(PGD *pg, PGD_FREE_SITE site, const char *func) {
    if (!pg || pg == PGD_EMPTY)
        return;

    // Claim the PGD before trusting ANY field of it. On an already freed PGD every field
    // below offset 2*sizeof(void*) has been overwritten by ARAL's free-list header, and on
    // LP64 the values it leaves behind (type 0 == ARRAY_32BIT, partition 0) are precisely
    // the ones that make the rest of this function proceed as if nothing were wrong.
    // Read before claiming. On the path this guard exists for - a PGD that is already
    // freed - we then fatal without writing to it at all. The exchange still happens on
    // the normal path, so two threads racing to free one live PGD cannot both win.
    uint16_t previous = __atomic_load_n(&pg->raw.freed_mark, __ATOMIC_ACQUIRE);
    if (likely(!pgd_mark_is_freed(previous)))
        previous = pgd_claim_freed(pg, site);

    if (unlikely(pgd_mark_is_freed(previous))) {
        fatal("DBENGINE: PGD double free - pgd %p was first freed by: %s; "
              "it is now being freed again by: %s (in '%s'). "
              "Its first %zu bytes now hold ARAL's free-list header, so the values it "
              "reports (type %u, partition %u, used %u, slots %u, data %p) belong to that "
              "header, not to this PGD.",
              (void *)pg,
              pgd_free_site_name((PGD_FREE_SITE)(previous & 0xFFU)),
              pgd_free_site_name(site), func ? func : "(unknown)",
              (size_t)(2 * sizeof(void *)),
              (unsigned)pg->type, (unsigned)pg->partition,
              (unsigned)pg->used, (unsigned)pg->slots, pg->raw.data);
    }

    internal_fatal(pg->partition >= pgd_alloc_globals.partitions,
                   "PGD partition is invalid %u", pg->partition);

    switch (pg->type)
    {
        case RRDENG_PAGE_TYPE_GORILLA_32BIT: {
            if (pg->states & PGD_STATE_CREATED_FROM_DISK)
            {
                internal_fatal(pg->raw.data == NULL, "Tried to free gorilla PGD loaded from disk with NULL data");

                pgd_data_free(pg->raw.data, pg->raw.size, pg->partition);
                timing_dbengine_evict_step(TIMING_STEP_DBENGINE_EVICT_FREE_MAIN_PGD_ARAL);
            }
            else if ((pg->states & PGD_STATE_CREATED_FROM_COLLECTOR) ||
                     (pg->states & PGD_STATE_SCHEDULED_FOR_FLUSHING) ||
                     (pg->states & PGD_STATE_FLUSHED_TO_DISK))
            {
                internal_fatal(pg->gorilla.writer == NULL,
                               "PGD does not have an active gorilla writer");

                internal_fatal(pg->gorilla.num_buffers == 0,
                               "PGD does not have any gorilla buffers allocated");

                while (true) {
                    gorilla_buffer_t *gbuf = gorilla_writer_drop_head_buffer(pg->gorilla.writer);
                    if (!gbuf)
                        break;
                    aral_freez(pgd_alloc_globals.aral_gorilla_buffer[pg->partition], gbuf);
                    pg->gorilla.num_buffers -= 1;
                }

                timing_dbengine_evict_step(TIMING_STEP_DBENGINE_EVICT_FREE_MAIN_PGD_GLIVE);

                internal_fatal(pg->gorilla.num_buffers != 0,
                               "Could not free all gorilla writer buffers");

                aral_freez(pgd_alloc_globals.aral_gorilla_writer[pg->partition], pg->gorilla.writer);
                pg->gorilla.writer = NULL;

                timing_dbengine_evict_step(TIMING_STEP_DBENGINE_EVICT_FREE_MAIN_PGD_GWORKER);
            } else {
                fatal("pgd_free() called on gorilla page with unsupported state");
                // TODO: should we support any other states?
                // if (!(pg->states & PGD_STATE_FLUSHED_TO_DISK))
                //     fatal("pgd_free() is not supported yet for pages flushed to disk");
            }

            break;
        }

        case RRDENG_PAGE_TYPE_ARRAY_32BIT:
        case RRDENG_PAGE_TYPE_ARRAY_TIER1:
            pgd_data_free(pg->raw.data, pg->raw.size, pg->partition);
            break;

        default:
            netdata_log_error("%s() - Unknown page type: %uc", __FUNCTION__, pg->type);
            break;
    }

    timing_dbengine_evict_step(TIMING_STEP_DBENGINE_EVICT_FREE_MAIN_PGD_DATA);

    aral_freez(pgd_alloc_globals.aral_pgd[pg->partition], pg);

    timing_dbengine_evict_step(TIMING_STEP_DBENGINE_EVICT_FREE_MAIN_PGD_ARAL);
}

static void pgd_aral_unmark(PGD *pg) {
    if (!pg ||
        pg == PGD_EMPTY ||
        (pg->options & PAGE_OPTION_ARAL_UNMARKED) ||
        !(pg->options & PAGE_OPTION_ARAL_MARKED))
        return;

    internal_fatal(pg->partition >= pgd_alloc_globals.partitions,
                   "PGD partition is invalid %u", pg->partition);

    switch (pg->type)
    {
        case RRDENG_PAGE_TYPE_GORILLA_32BIT: {
            if (pg->states & PGD_STATE_CREATED_FROM_DISK)
                pgd_data_unmark(pg->raw.data, pg->raw.size, pg->partition);

            else if ((pg->states & PGD_STATE_CREATED_FROM_COLLECTOR) ||
                     (pg->states & PGD_STATE_SCHEDULED_FOR_FLUSHING) ||
                     (pg->states & PGD_STATE_FLUSHED_TO_DISK))
            {
                internal_fatal(pg->gorilla.writer == NULL, "PGD does not have an active gorilla writer");
                internal_fatal(pg->gorilla.num_buffers == 0, "PGD does not have any gorilla buffers allocated");

                gorilla_writer_aral_unmark(pg->gorilla.writer, pgd_alloc_globals.aral_gorilla_buffer[pg->partition]);
                aral_unmark_allocation(pgd_alloc_globals.aral_gorilla_writer[pg->partition], pg->gorilla.writer);
            }
            else {
                fatal("pgd_free() called on gorilla page with unsupported state");
                // TODO: should we support any other states?
                // if (!(pg->states & PGD_STATE_FLUSHED_TO_DISK))
                //     fatal("pgd_free() is not supported yet for pages flushed to disk");
            }

            break;
        }

        case RRDENG_PAGE_TYPE_ARRAY_32BIT:
        case RRDENG_PAGE_TYPE_ARRAY_TIER1:
            pgd_data_unmark(pg->raw.data, pg->raw.size, pg->partition);
            break;

        default:
            netdata_log_error("%s() - Unknown page type: %uc", __FUNCTION__, pg->type);
            break;
    }

    aral_unmark_allocation(pgd_alloc_globals.aral_pgd[pg->partition], pg);

    // make sure we will not do this again
    pg->options |= PAGE_OPTION_ARAL_UNMARKED;
}

// ----------------------------------------------------------------------------
// utility functions

ALWAYS_INLINE uint32_t pgd_type(PGD *pg)
{
    return pg->type;
}

ALWAYS_INLINE bool pgd_is_empty(PGD *pg)
{
    if (!pg)
        return true;

    if (pg == PGD_EMPTY)
        return true;

    if (pg->used == 0)
        return true;

    if (pg->options & PAGE_OPTION_ALL_VALUES_EMPTY)
        return true;

    return false;
}

ALWAYS_INLINE uint32_t pgd_slots_used(PGD *pg)
{
    if (!pg)
        return 0;

    if (pg == PGD_EMPTY)
        return 0;

    return pg->used;
}

ALWAYS_INLINE uint32_t pgd_capacity(PGD *pg) {
    if (!pg)
        return 0;

    if (pg == PGD_EMPTY)
        return 0;

    return pg->slots;
}

// return the overall memory footprint of the page, including all its structures and overheads
ALWAYS_INLINE uint32_t pgd_memory_footprint(PGD *pg)
{
    if (!pg)
        return 0;

    if (pg == PGD_EMPTY)
        return 0;

    size_t footprint = pgd_alloc_globals.sizeof_pgd;

    switch (pg->type) {
        case RRDENG_PAGE_TYPE_GORILLA_32BIT: {
            if (pg->states & PGD_STATE_CREATED_FROM_DISK)
                footprint += pgd_data_footprint(pg->raw.size, pg->partition);

            else {
                footprint += pgd_alloc_globals.sizeof_gorilla_writer_t;
                footprint += pg->gorilla.num_buffers * pgd_alloc_globals.sizeof_gorilla_buffer_32bit;
            }
            break;
        }

        case RRDENG_PAGE_TYPE_ARRAY_32BIT:
        case RRDENG_PAGE_TYPE_ARRAY_TIER1:
            footprint += pgd_data_footprint(pg->raw.size, pg->partition);
            break;

        default:
            netdata_log_error("%s() - Unknown page type: %uc", __FUNCTION__, pg->type);
            break;
    }

    return footprint;
}

// return the nominal buffer size depending on the page type - used by the PGC histogram
uint32_t pgd_buffer_memory_footprint(PGD *pg)
{
    pgd_check_alive(pg, "buffer memory footprint");

    if (!pg)
        return 0;

    if (pg == PGD_EMPTY)
        return 0;

    size_t footprint = 0;

    switch (pg->type) {
        case RRDENG_PAGE_TYPE_GORILLA_32BIT: {
            if (pg->states & PGD_STATE_CREATED_FROM_DISK)
                footprint = pg->raw.size;

            else
                footprint = pg->gorilla.num_buffers * RRDENG_GORILLA_32BIT_BUFFER_SIZE;
            break;
        }

        case RRDENG_PAGE_TYPE_ARRAY_32BIT:
        case RRDENG_PAGE_TYPE_ARRAY_TIER1:
            footprint = pg->raw.size;
            break;

        default:
            netdata_log_error("%s() - Unknown page type: %uc", __FUNCTION__, pg->type);
            break;
    }

    return footprint;
}

uint32_t pgd_disk_footprint(PGD *pg)
{
    // before pgd_aral_unmark() below dereferences pg->gorilla.writer, which sits at offset 8
    // and is therefore clobbered on a freed PGD on LP64 (ARAL_FREE covers bytes 0..15). On
    // ILP32 it covers only 0..7, so writer survives there and the deref hits the real, freed
    // gorilla writer instead - equally fatal, differently shaped.
    pgd_check_alive(pg, "disk footprint (flush)");

    if (!pgd_slots_used(pg))
        return 0;

    size_t size = 0;

    // since the page is ready for flushing, let's unmark its pages to ARAL
    pgd_aral_unmark(pg);

    switch (pg->type) {
        case RRDENG_PAGE_TYPE_GORILLA_32BIT: {
            if (pg->states & PGD_STATE_CREATED_FROM_COLLECTOR ||
                pg->states & PGD_STATE_SCHEDULED_FOR_FLUSHING ||
                pg->states & PGD_STATE_FLUSHED_TO_DISK)
            {
                internal_fatal(!pg->gorilla.writer,
                               "pgd_disk_footprint() not implemented for NULL gorilla writers");

                internal_fatal(pg->gorilla.num_buffers == 0,
                               "Gorilla writer does not have any buffers");

                size = pg->gorilla.num_buffers * RRDENG_GORILLA_32BIT_BUFFER_SIZE;

                if (pg->states & PGD_STATE_CREATED_FROM_COLLECTOR)
                    pulse_gorilla_tier0_page_flush(
                        gorilla_writer_actual_nbytes(pg->gorilla.writer),
                        gorilla_writer_optimal_nbytes(pg->gorilla.writer),
                        tier_page_size[0]);

            } else if (pg->states & PGD_STATE_CREATED_FROM_DISK) {
                size = pg->raw.size;
            } else {
                fatal("Asked disk footprint on unknown page state");
            }

            break;
        }

        case RRDENG_PAGE_TYPE_ARRAY_32BIT:
        case RRDENG_PAGE_TYPE_ARRAY_TIER1: {
            uint32_t used_size = pg->used * page_type_size[pg->type];
            internal_fatal(used_size > pg->raw.size, "Wrong disk footprint page size");
            size = used_size;

            break;
        }

        default:
            netdata_log_error("%s() - Unknown page type: %uc", __FUNCTION__, pg->type);
            break;
    }

    internal_fatal(pg->states & PGD_STATE_CREATED_FROM_DISK,
                   "Disk footprint asked for page created from disk.");

    pg->states = PGD_STATE_SCHEDULED_FOR_FLUSHING;
    return size;
}

void pgd_copy_to_extent(PGD *pg, uint8_t *dst, uint32_t dst_size)
{
    // a freed PGD here would serialize ARAL's free-list header into the extent
    pgd_check_alive(pg, "copy to extent (flush)");

    internal_fatal(pgd_disk_footprint(pg) != dst_size, "Wrong disk footprint size requested (need %u, available %u)",
                   pgd_disk_footprint(pg), dst_size);

    switch (pg->type) {
        case RRDENG_PAGE_TYPE_GORILLA_32BIT: {
            if ((pg->states & PGD_STATE_SCHEDULED_FOR_FLUSHING) == 0)
                fatal("Copying to extent is supported only for PGDs that are scheduled for flushing.");

            internal_fatal(!pg->gorilla.writer,
                           "pgd_copy_to_extent() not implemented for NULL gorilla writers");

            internal_fatal(pg->gorilla.num_buffers == 0,
                           "pgd_copy_to_extent() gorilla writer does not have any buffers");

            bool ok = gorilla_writer_serialize(pg->gorilla.writer, dst, dst_size);
            UNUSED(ok);
            internal_fatal(!ok,
                           "pgd_copy_to_extent() tried to serialize pg=%p, gw=%p (with dst_size=%u bytes, num_buffers=%u)",
                           pg, pg->gorilla.writer, dst_size, pg->gorilla.num_buffers);
            break;
        }

        case RRDENG_PAGE_TYPE_ARRAY_32BIT:
        case RRDENG_PAGE_TYPE_ARRAY_TIER1:
            memcpy(dst, pg->raw.data, dst_size);
            break;

        default:
            netdata_log_error("%s() - Unknown page type: %uc", __FUNCTION__, pg->type);
            break;
    }

    pg->states = PGD_STATE_FLUSHED_TO_DISK;
}

// ----------------------------------------------------------------------------
// data collection

// returns additional memory that may have been allocated to store this point
ALWAYS_INLINE_HOT_FLATTEN
size_t pgd_append_point(
    PGD *pg,
    usec_t point_in_time_ut __maybe_unused,
    NETDATA_DOUBLE n,
    NETDATA_DOUBLE min_value,
    NETDATA_DOUBLE max_value,
    uint16_t count,
    uint16_t anomaly_count,
    SN_FLAGS flags,
    uint32_t expected_slot)
{
    // before pg->states, which a freed PGD no longer owns
    pgd_check_alive(pg, "data collection");

    if (pg->states & PGD_STATE_SCHEDULED_FOR_FLUSHING) {
        if(exit_initiated_get() == EXIT_REASON_NONE)
            pgd_fatal(pg, "Data collection on page already scheduled for flushing");
        else
            return 0;
    }

    if (!(pg->states & PGD_STATE_CREATED_FROM_COLLECTOR)) {
        if(exit_initiated_get() == EXIT_REASON_NONE)
            pgd_fatal(pg, "DBENGINE: collection on page not created from a collector");
        else
            return 0;
    }

    if (unlikely(pg->used != expected_slot))
        pgd_fatal(pg, "DBENGINE: page is not aligned to expected slot (used %u, expected %u)",
              pg->used, expected_slot);

    if (unlikely(pg->used >= pg->slots))
        pgd_fatal(pg, "DBENGINE: attempted to write beyond page size (page type %u, slots %u, used %u)",
              pg->type, pg->slots, pg->used /* FIXME:, pg->size */);

    switch (pg->type) {
        case RRDENG_PAGE_TYPE_GORILLA_32BIT: {
            pg->used++;
            storage_number t = pack_storage_number(n, flags);

            if ((pg->options & PAGE_OPTION_ALL_VALUES_EMPTY) && does_storage_number_exist(t))
                pg->options &= ~PAGE_OPTION_ALL_VALUES_EMPTY;

            bool ok = gorilla_writer_write(pg->gorilla.writer, t);
            if (!ok) {
                gorilla_buffer_t *new_buffer = pgd_gorilla_buffer_alloc(pg->partition);
                memset(new_buffer, 0, RRDENG_GORILLA_32BIT_BUFFER_SIZE);

                gorilla_writer_add_buffer(pg->gorilla.writer, new_buffer, RRDENG_GORILLA_32BIT_BUFFER_SLOTS);
                pg->gorilla.num_buffers += 1;
                pulse_gorilla_hot_buffer_added();

                ok = gorilla_writer_write(pg->gorilla.writer, t);
                internal_fatal(ok == false, "Failed to writer value in newly allocated gorilla buffer.");

                return RRDENG_GORILLA_32BIT_BUFFER_SIZE;
            }

            break;
        }
        case RRDENG_PAGE_TYPE_ARRAY_TIER1: {
            storage_number_tier1_t *tier12_metric_data = (storage_number_tier1_t *)pg->raw.data;
            storage_number_tier1_t t;
            t.sum_value = (float) n;
            t.min_value = (float) min_value;
            t.max_value = (float) max_value;
            t.anomaly_count = anomaly_count;
            t.count = count;
            tier12_metric_data[pg->used++] = t;

            if ((pg->options & PAGE_OPTION_ALL_VALUES_EMPTY) && fpclassify(n) != FP_NAN)
                pg->options &= ~PAGE_OPTION_ALL_VALUES_EMPTY;

            break;
        }
        case RRDENG_PAGE_TYPE_ARRAY_32BIT: {
            storage_number *tier0_metric_data = (storage_number *)pg->raw.data;
            storage_number t = pack_storage_number(n, flags);
            tier0_metric_data[pg->used++] = t;

            if ((pg->options & PAGE_OPTION_ALL_VALUES_EMPTY) && does_storage_number_exist(t))
                pg->options &= ~PAGE_OPTION_ALL_VALUES_EMPTY;

            break;
        }
        default:
            netdata_log_error("%s() - Unknown page type: %uc", __FUNCTION__, pg->type);
            break;
    }

    return 0;
}

// ----------------------------------------------------------------------------
// querying with cursor

static void pgdc_seek(PGDC *pgdc, uint32_t position)
{
    PGD *pg = pgdc->pgd;

    switch (pg->type) {
        case RRDENG_PAGE_TYPE_GORILLA_32BIT: {
            if (pg->states & PGD_STATE_CREATED_FROM_DISK) {
                pgdc->slots = pgdc->pgd->slots;
                pgdc->gr = gorilla_reader_init((void *) pg->raw.data);
            } else {
                if (!(pg->states & PGD_STATE_CREATED_FROM_COLLECTOR) &&
                    !(pg->states & PGD_STATE_SCHEDULED_FOR_FLUSHING) &&
                    !(pg->states & PGD_STATE_FLUSHED_TO_DISK))
                    pgd_fatal(pg, "pgdc_seek() currently is not supported for pages created from disk.");

                if (!pg->gorilla.writer)
                    pgd_fatal(pg, "Seeking from a page without an active gorilla writer is not supported (yet).");

                pgdc->slots = gorilla_writer_entries(pg->gorilla.writer);
                pgdc->gr = gorilla_writer_get_reader(pg->gorilla.writer);
            }

            if (position > pgdc->slots)
                position = pgdc->slots;

            for (uint32_t i = 0; i != position; i++) {
                uint32_t value;

                bool ok = gorilla_reader_read(&pgdc->gr, &value);
                if (!ok) {
                    // this is fine, the reader will return empty points
                    break;
                }
            }

            break;
        }

        case RRDENG_PAGE_TYPE_ARRAY_32BIT:
        case RRDENG_PAGE_TYPE_ARRAY_TIER1:
            pgdc->slots = pgdc->pgd->used;
            break;

        default:
            netdata_log_error("%s() - Unknown page type: %uc", __FUNCTION__, pg->type);
            break;
    }
}

void pgdc_reset(PGDC *pgdc, PGD *pgd, uint32_t position)
{
    // pgd might be null and position equal to UINT32_MAX
    pgd_check_alive(pgd, "query cursor reset");

    pgdc->pgd = pgd;
    pgdc->position = position;

    if (!pgd)
        return;

    if (pgd == PGD_EMPTY)
        return;

    if (position == UINT32_MAX)
        return;

    pgdc_seek(pgdc, position);
}

ALWAYS_INLINE_HOT_FLATTEN
bool pgdc_get_next_point(PGDC *pgdc, uint32_t expected_position __maybe_unused, STORAGE_POINT *sp)
{
    if (!pgdc->pgd || pgdc->pgd == PGD_EMPTY || pgdc->position >= pgdc->slots)
    {
        storage_point_empty(*sp, sp->start_time_s, sp->end_time_s);
        return false;
    }

    internal_fatal(pgdc->position != expected_position, "Wrong expected cursor position");

    switch (pgdc->pgd->type)
    {
        case RRDENG_PAGE_TYPE_GORILLA_32BIT: {
            pgdc->position++;

            uint32_t n = 666666666;
            bool ok = gorilla_reader_read(&pgdc->gr, &n);

            if (ok) {
                sp->min = sp->max = sp->sum = unpack_storage_number(n);
                sp->flags = (SN_FLAGS)(n & SN_USER_FLAGS);
                sp->count = 1;
                sp->anomaly_count = is_storage_number_anomalous(n) ? 1 : 0;
            } else {
                storage_point_empty(*sp, sp->start_time_s, sp->end_time_s);
            }

            return ok;
        }
        case RRDENG_PAGE_TYPE_ARRAY_TIER1: {
            storage_number_tier1_t *array = (storage_number_tier1_t *) pgdc->pgd->raw.data;
            storage_number_tier1_t n = array[pgdc->position++];

            sp->flags = n.anomaly_count ? SN_FLAG_NONE : SN_FLAG_NOT_ANOMALOUS;
            sp->count = n.count;
            sp->anomaly_count = n.anomaly_count;
            sp->min = n.min_value;
            sp->max = n.max_value;
            sp->sum = n.sum_value;

            return true;
        }
        case RRDENG_PAGE_TYPE_ARRAY_32BIT: {
            storage_number *array = (storage_number *) pgdc->pgd->raw.data;
            storage_number n = array[pgdc->position++];

            sp->min = sp->max = sp->sum = unpack_storage_number(n);
            sp->flags = (SN_FLAGS)(n & SN_USER_FLAGS);
            sp->count = 1;
            sp->anomaly_count = is_storage_number_anomalous(n) ? 1 : 0;

            return true;
        }
        default: {
            static bool logged = false;
            if (!logged)
            {
                netdata_log_error("DBENGINE: unknown page type %"PRIu32" found. Cannot decode it. Ignoring its metrics.",
                                  pgd_type(pgdc->pgd));
                logged = true;
            }

            storage_point_empty(*sp, sp->start_time_s, sp->end_time_s);
            return false;
        }
    }
}

// ----------------------------------------------------------------------------
// unittest

// The PGD lifetime guard rests on an assumption that lives in ANOTHER module: that
// aral_freez() overwrites exactly the first 16 bytes of the element with
// ARAL_FREE { size_t size; struct aral_free *next; }, leaving bytes 16..23 - where we put
// the freed mark - alone. This test pins that assumption, so a future change to ARAL's
// free-list layout fails here loudly instead of silently disarming the guard in
// production.
//
// Skipped under FSANITIZE_ADDRESS: there aral_freez() delegates to freez(), so reading a
// freed PGD back is a genuine use-after-free that ASAN traps first, and ASAN already
// catches PGD double frees directly.
int pgd_unittest(void) {
#if defined(FSANITIZE_ADDRESS)
    fprintf(stderr, "PGD: skipping lifetime-guard test under ASAN (aral bypasses its pool)\n");
    return 0;
#else
    int errors = 0;

    // pgd_init_arals() sorts and de-duplicates the static aral_sizes[] table in place, so
    // it is not idempotent. Only initialise if nothing has yet.
    if(!pgd_alloc_globals.partitions)
        pgd_init_arals();

    // ------------------------------------------------------------------------------------
    // The cross-module assumption, checked deterministically.
    //
    // What can silently disarm the production guard is ARAL changing where it writes its
    // free-list header: freed_mark must stay outside it. That claim is about ARAL, not about
    // PGD, so it does not need a real PGD - and checking it on a PRIVATE aral makes it
    // immune to whatever state the shared pgd arals are in. A fresh aral has exactly one
    // page, and aral_freez_internal() keeps the last page with free items (aral.c:1275)
    // instead of deleting it, so reading the element back is safe by construction. The
    // second allocation keeps used_elements > 0 as well, so the page cannot even become a
    // deletion candidate.
    //
    // This runs unconditionally. The PGD-level checks further down depend on winning an
    // allocation race for two co-located slots and may be skipped; this one never is.
    {
        struct aral_statistics selftest_stats = { 0 };
        ARAL *ar = aral_create("pgd-lifetime-selftest", sizeof(PGD), 0, 0,
                               &selftest_stats, NULL, NULL, false, false, true);
        if(!ar) {
            fprintf(stderr, "PGD: cannot create the selftest aral\n");
            errors++;
        }
        else {
            void *keep = aral_mallocz(ar);      // holds the page, never freed before the read
            void *e = aral_mallocz(ar);

            const size_t mark_off = offsetof(struct pgd, raw.freed_mark);
            uint16_t *mark = (uint16_t *)((uint8_t *)e + mark_off);
            uint8_t *head = (uint8_t *)e;

            *mark = PGD_FREED_MAGIC | 0x5AU;
            memset(head, 0, 2 * sizeof(void *));

            aral_freez(ar, e);

            // ARAL must have written its free-list header over the head of the element...
            bool header_written = false;
            for(size_t i = 0; i < 2 * sizeof(void *) ;i++)
                if(head[i]) { header_written = true; break; }

            if(!header_written) {
                fprintf(stderr, "PGD: aral_freez() no longer writes a free-list header over the "
                                "start of a freed element - re-check ARAL_FREE in aral.c\n");
                errors++;
            }

            // ...and it must NOT have reached the freed mark.
            if(*mark != (uint16_t)(PGD_FREED_MAGIC | 0x5AU)) {
                fprintf(stderr, "PGD: aral_freez() now overwrites offset %zu, where the freed "
                                "mark lives - the lifetime guard is SILENTLY DISARMED\n", mark_off);
                errors++;
            }

            aral_freez(ar, keep);
        }
    }

    // This test deliberately reads `pg` back AFTER freeing it, so its page must not be
    // deletable: aral_freez_internal() deletes a page that has become FULLY free whenever
    // another page still has free items (aral.c:1265; the delete is gated on
    // used_elements == 0). Keeping a live element ON THE SAME PAGE prevents that.
    //
    // "Same aral" is not enough. pgd_alloc() picks its aral by gettid_cached() % partitions,
    // so every allocation here shares one aral, but aral serves recycled free-list slots
    // before fresh ones and those are scattered across pages - and a fresh page starts as
    // soon as the current one is exhausted. So co-location has to be PROVEN, not assumed:
    // two elements exactly one slot apart are necessarily on the same page, because distinct
    // pages are separate mallocs each carrying its own ARAL_PAGE header.
    //
    // Probe until such a pair appears. Recycled slots are scattered, but the free list is
    // finite: once it is drained, aral hands out consecutive slots from a page's tail, so a
    // bounded probe finds an adjacent pair. All probes stay allocated until the end, which
    // also keeps every page they touch alive.
    //
    // pgd_unittest() runs at position 33 of -W unittest, well after test_dbengine() has
    // exercised these arals, so this is the realistic case - not the standalone -W pgdtest
    // one, where the aral is fresh and any two allocations are trivially adjacent. Note CI
    // only ever runs -W unittest.
    //
    // The hazard if this were wrong: these arals are created with mmap=false (page.c:518),
    // so a deleted page is freez()'d, not nd_munmap()'d - a stale read would be a heap
    // use-after-free rather than a fault. Silent, and invisible to ASAN because this whole
    // test is skipped there. That is why it is proven rather than assumed.
    PGD *probe[64];
    size_t probes = 0;
    PGD *pg = NULL;          // the one under test: proven to share a page with another probe
    size_t slot = 0;

    while(probes < _countof(probe)) {
        PGD *p = pgd_create(RRDENG_PAGE_TYPE_ARRAY_TIER1, 16);
        if(!p || p == PGD_EMPTY)
            break;

        if(!slot)
            slot = aral_actual_element_size(pgd_alloc_globals.aral_pgd[p->partition]);

        // is this one adjacent to any probe already taken?
        for(size_t i = 0; i < probes ;i++) {
            uintptr_t a = (uintptr_t)p, b = (uintptr_t)probe[i];
            if((a > b ? a - b : b - a) == (uintptr_t)slot) {
                // probe[i] is on p's page and stays allocated until the end of the test,
                // which is what keeps that page's used_elements > 0 while we read p back
                pg = p;
                break;
            }
        }

        probe[probes++] = p;

        if(pg)
            break;
    }

    if(!probes) {
        fprintf(stderr, "PGD: cannot create a page to test with\n");
        return 1;
    }

    if(!pg) {
        // The probe bound is a heuristic, not a proof: aral serves a page's virgin slots
        // sequentially (the elements_segmented fast path) and only then falls back to the
        // recycled list, so how many allocations it takes to see two co-located slots depends
        // on how scattered that list is. A long enough scattered list would exhaust the probe.
        //
        // That must NOT fail the build - it would be a false failure on a healthy tree. It is
        // safe to skip because the assumption that can silently disarm the guard was already
        // verified above, deterministically, on a private aral. What is lost here is only the
        // end-to-end check that pgd_free() stamps the mark on a real PGD.
        fprintf(stderr, "PGD: no two PGDs landed on one aral page in %zu probes - skipping the "
                        "freed-PGD read-back (the ARAL layout assumption was still verified)\n",
                probes);
    }
    else {
        // a live PGD must never look freed
        if(pgd_mark_is_freed(__atomic_load_n(&pg->raw.freed_mark, __ATOMIC_RELAXED))) {
            fprintf(stderr, "PGD: a freshly created PGD is already marked as freed\n");
            errors++;
        }

        pgd_free(pg, PGD_FREE_SITE_UNITTEST);

        // Reading pg back is deliberate, and safe because a probe provably holds its page.
        // Everything below 2*sizeof(void*) now belongs to aral's free-list header.
        //
        // This is the detector's own contract: a second free must see the FIRST freer's site.
        // We exercise the exact claim the fatal path reads, rather than calling pgd_free()
        // again - that would fatal and take this process down with it.
        uint16_t previous = pgd_claim_freed(pg, PGD_FREE_SITE_CREATE_BAD_TYPE);
        if(!pgd_mark_is_freed(previous)) {
            fprintf(stderr, "PGD: a freed PGD does not carry the freed mark - the guard is disarmed\n");
            errors++;
        }
        else if((PGD_FREE_SITE)(previous & 0xFFU) != PGD_FREE_SITE_UNITTEST) {
            fprintf(stderr, "PGD: the freed mark reports the wrong first-free site (%s) - "
                            "the site is the whole point of the mark\n",
                    pgd_free_site_name((PGD_FREE_SITE)(previous & 0xFFU)));
            errors++;
        }
    }

    // The use-after-free guard reads the SAME mark the double-free guard writes, and the
    // exchange above already proved pgd_free() left it set with the right site - so there is
    // nothing further to assert about a freed PGD here. What is left to pin is the guard's
    // behaviour on the inputs it must NOT flag. Calling a real entry point
    // (pgd_append_point() etc.) is not an option - it would fatal by design and take this
    // process down.
    //
    // NULL and PGD_EMPTY must never be reported as freed: pgd_check_alive() returns early
    // for both, and callers rely on that. If this ever regressed it would fatal on every
    // empty page in the fleet, so it is worth a test even though it looks trivial.
    pgd_check_alive(NULL, "unittest: NULL must be accepted");
    pgd_check_alive(PGD_EMPTY, "unittest: PGD_EMPTY must be accepted");

    // Everything below reads or writes the freed `pg`, so it runs only when a co-located
    // probe was proven above (pg is NULL otherwise, and that already counted an error).
    if(pg) {
        // pgd_alloc() must clear the mark, or every FIRST free of a recycled slot would be
        // misreported as a double free, AND every first USE of it as a use-after-free. Test
        // the clear directly - which slot aral hands back next is not ours to predict.
        pgd_clear_freed(pg);

        // a cleared mark must make the use guard quiet again
        pgd_check_alive(pg, "unittest: a cleared mark must read as live");
        if(pgd_mark_is_freed(__atomic_load_n(&pg->raw.freed_mark, __ATOMIC_RELAXED))) {
            fprintf(stderr, "PGD: clearing the freed mark did not clear it\n");
            errors++;
        }

#if UINTPTR_MAX == 0xFFFFFFFFFFFFFFFFu
        // Pin the LP64 mechanism this guard exists to describe: aral's 16-byte free-list
        // header makes a freed PGD read back as ARRAY_32BIT in partition 0, which is what
        // lets a second pgd_free() misroute silently. The guard is correct either way, but
        // if this stops holding, the comments in this file and the fatal's wording are stale.
        if(pg->type != RRDENG_PAGE_TYPE_ARRAY_32BIT || pg->partition != 0) {
            fprintf(stderr,
                    "PGD: aral's free-list header no longer makes a freed PGD read back as "
                    "ARRAY_32BIT/partition 0 (type %u, partition %u) - re-check ARAL_FREE in "
                    "aral.c against struct pgd\n",
                    (unsigned)pg->type, (unsigned)pg->partition);
            errors++;
        }
#endif
    }

    // every read of the freed `pg` is done - the probes may go back to the aral now.
    // pg was freed by the test itself, so skip it here and never free it twice.
    for(size_t i = 0; i < probes ;i++) {
        if(probe[i] != pg)
            pgd_free(probe[i], PGD_FREE_SITE_UNITTEST);
    }

    if(errors)
        fprintf(stderr, "PGD: lifetime-guard test FAILED with %d error(s)\n", errors);
    else
        fprintf(stderr, "PGD: lifetime-guard test PASSED\n");

    return errors;
#endif
}
