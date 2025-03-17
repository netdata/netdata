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
} page_raw_t;

typedef struct {
    gorilla_writer_t *writer;
    uint16_t num_buffers;
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

            uint32_t total_entries = gorilla_buffer_patch((void *) pg->raw.data);
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
            aral_freez(pgd_alloc_globals.aral_pgd[pg->partition], pg);
            pg = PGD_EMPTY;
            break;
    }

    return pg;
}

void pgd_free(PGD *pg) {
    if (!pg || pg == PGD_EMPTY)
        return;

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

                pg->raw.data = NULL;
                pg->raw.size = 0;
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
