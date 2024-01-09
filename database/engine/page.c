// SPDX-License-Identifier: GPL-3.0-or-later

#include "page.h"

#include "libnetdata/libnetdata.h"

typedef enum __attribute__((packed)) {
    PAGE_OPTION_ALL_VALUES_EMPTY    = (1 << 0),
} PAGE_OPTIONS;

typedef enum __attribute__((packed)) {
    PGD_STATE_CREATED_FROM_COLLECTOR        = (1 << 0),
    PGD_STATE_CREATED_FROM_DISK             = (1 << 1),
    PGD_STATE_SCHEDULED_FOR_FLUSHING        = (1 << 2),
    PGD_STATE_FLUSHED_TO_DISK               = (1 << 3),
} PGD_STATES;

typedef struct {
    uint8_t *data;
    uint32_t size;
} page_raw_t;


typedef struct {
    size_t num_buffers;
    gorilla_writer_t *writer;
    int aral_index;
} page_gorilla_t;

struct pgd {
    // the page type
    uint8_t type;

   // options related to the page
    PAGE_OPTIONS options;

    PGD_STATES states;

    // the uses number of slots in the page
    uint32_t used;

    // the total number of slots available in the page
    uint32_t slots;

    union {
        page_raw_t raw;
        page_gorilla_t gorilla;
    };
};

// ----------------------------------------------------------------------------
// memory management

struct {
    ARAL *aral_pgd;
    ARAL *aral_data[RRD_STORAGE_TIERS];
    ARAL *aral_gorilla_buffer[4];
    ARAL *aral_gorilla_writer[4];
} pgd_alloc_globals = {};

static ARAL *pgd_aral_data_lookup(size_t size)
{
    for (size_t tier = 0; tier < storage_tiers; tier++)
        if (size == tier_page_size[tier])
            return pgd_alloc_globals.aral_data[tier];

    return NULL;
}

void pgd_init_arals(void)
{
    // pgd aral
    {
        char buf[20 + 1];
        snprintfz(buf, sizeof(buf) - 1, "pgd");

        // FIXME: add stats
        pgd_alloc_globals.aral_pgd = aral_create(
                buf,
                sizeof(struct pgd),
                64,
                512 * (sizeof(struct pgd)),
                pgc_aral_statistics(),
                NULL, NULL, false, false);
    }

    // tier page aral
    {
        for (size_t i = storage_tiers; i > 0 ;i--)
        {
            size_t tier = storage_tiers - i;

            char buf[20 + 1];
            snprintfz(buf, sizeof(buf) - 1, "tier%zu-pages", tier);

            pgd_alloc_globals.aral_data[tier] = aral_create(
                    buf,
                    tier_page_size[tier],
                    64,
                    512 * (tier_page_size[tier]),
                    pgc_aral_statistics(),
                    NULL, NULL, false, false);
        }
    }

    // gorilla buffers aral
    for (size_t i = 0; i != 4; i++) {
        char buf[20 + 1];
        snprintfz(buf, sizeof(buf) - 1, "gbuffer-%zu", i);

        // FIXME: add stats
        pgd_alloc_globals.aral_gorilla_buffer[i] = aral_create(
                buf,
                GORILLA_BUFFER_SIZE,
                64,
                512 * GORILLA_BUFFER_SIZE,
                pgc_aral_statistics(),
                NULL, NULL, false, false);
    }

    // gorilla writers aral
    for (size_t i = 0; i != 4; i++) {
        char buf[20 + 1];
        snprintfz(buf, sizeof(buf) - 1, "gwriter-%zu", i);

        // FIXME: add stats
        pgd_alloc_globals.aral_gorilla_writer[i] = aral_create(
                buf,
                sizeof(gorilla_writer_t),
                64,
                512 * sizeof(gorilla_writer_t),
                pgc_aral_statistics(),
                NULL, NULL, false, false);
    }
}

static void *pgd_data_aral_alloc(size_t size)
{
    ARAL *ar = pgd_aral_data_lookup(size);
    if (!ar)
        return mallocz(size);
    else
        return aral_mallocz(ar);
}

static void pgd_data_aral_free(void *page, size_t size)
{
    ARAL *ar = pgd_aral_data_lookup(size);
    if (!ar)
        freez(page);
    else
        aral_freez(ar, page);
}

// ----------------------------------------------------------------------------
// management api

PGD *pgd_create(uint8_t type, uint32_t slots)
{
    PGD *pg = aral_mallocz(pgd_alloc_globals.aral_pgd);
    pg->type = type;
    pg->used = 0;
    pg->slots = slots;
    pg->options = PAGE_OPTION_ALL_VALUES_EMPTY;
    pg->states = PGD_STATE_CREATED_FROM_COLLECTOR;

    switch (type) {
        case PAGE_METRICS:
        case PAGE_TIER: {
            uint32_t size = slots * page_type_size[type];

            internal_fatal(!size || slots == 1,
                      "DBENGINE: invalid number of slots (%u) or page type (%u)", slots, type);

            pg->raw.size = size;
            pg->raw.data = pgd_data_aral_alloc(size);
            break;
        }
        case PAGE_GORILLA_METRICS: {
            internal_fatal(slots == 1,
                      "DBENGINE: invalid number of slots (%u) or page type (%u)", slots, type);

            pg->slots = 8 * GORILLA_BUFFER_SLOTS;

            // allocate new gorilla writer
            pg->gorilla.aral_index = gettid() % 4;
            pg->gorilla.writer = aral_mallocz(pgd_alloc_globals.aral_gorilla_writer[pg->gorilla.aral_index]);

            // allocate new gorilla buffer
            gorilla_buffer_t *gbuf = aral_mallocz(pgd_alloc_globals.aral_gorilla_buffer[pg->gorilla.aral_index]);
            memset(gbuf, 0, GORILLA_BUFFER_SIZE);
            global_statistics_gorilla_buffer_add_hot();

            *pg->gorilla.writer = gorilla_writer_init(gbuf, GORILLA_BUFFER_SLOTS);
            pg->gorilla.num_buffers = 1;

            break;
        }
        default:
            netdata_log_error("%s() - Unknown page type: %uc", __FUNCTION__, type);
            aral_freez(pgd_alloc_globals.aral_pgd, pg);
            pg = PGD_EMPTY;
            break;
    }

    return pg;
}

PGD *pgd_create_from_disk_data(uint8_t type, void *base, uint32_t size)
{
    if (!size)
        return PGD_EMPTY;

    if (size < page_type_size[type])
        return PGD_EMPTY;

    PGD *pg = aral_mallocz(pgd_alloc_globals.aral_pgd);

    pg->type = type;
    pg->states = PGD_STATE_CREATED_FROM_DISK;
    pg->options = ~PAGE_OPTION_ALL_VALUES_EMPTY;

    switch (type)
    {
        case PAGE_METRICS:
        case PAGE_TIER:
            pg->raw.size = size;
            pg->used = size / page_type_size[type];
            pg->slots = pg->used;

            pg->raw.data = pgd_data_aral_alloc(size);
            memcpy(pg->raw.data, base, size);
            break;
        case PAGE_GORILLA_METRICS:
            internal_fatal(size == 0, "Asked to create page with 0 data!!!");
            internal_fatal(size % sizeof(uint32_t), "Unaligned gorilla buffer size");
            internal_fatal(size % GORILLA_BUFFER_SIZE, "Expected size to be a multiple of %zu-bytes", GORILLA_BUFFER_SIZE);

            pg->raw.data = mallocz(size);
            pg->raw.size = size;

            // TODO: rm this
            memset(pg->raw.data, 0, size);
            memcpy(pg->raw.data, base, size);

            uint32_t total_entries = gorilla_buffer_patch((void *) pg->raw.data);

            pg->used = total_entries;
            pg->slots = pg->used;
            break;
        default:
            netdata_log_error("%s() - Unknown page type: %uc", __FUNCTION__, type);
            aral_freez(pgd_alloc_globals.aral_pgd, pg);
            pg = PGD_EMPTY;
            break;
    }

    return pg;
}

void pgd_free(PGD *pg)
{
    if (!pg)
        return;

    if (pg == PGD_EMPTY)
        return;

    switch (pg->type)
    {
        case PAGE_METRICS:
        case PAGE_TIER:
            pgd_data_aral_free(pg->raw.data, pg->raw.size);
            break;
        case PAGE_GORILLA_METRICS: {
            if (pg->states & PGD_STATE_CREATED_FROM_DISK)
            {
                internal_fatal(pg->raw.data == NULL, "Tried to free gorilla PGD loaded from disk with NULL data");
                freez(pg->raw.data);
                pg->raw.data = NULL;
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
                    aral_freez(pgd_alloc_globals.aral_gorilla_buffer[pg->gorilla.aral_index], gbuf);
                    pg->gorilla.num_buffers -= 1;
                }

                internal_fatal(pg->gorilla.num_buffers != 0,
                               "Could not free all gorilla writer buffers");

                aral_freez(pgd_alloc_globals.aral_gorilla_writer[pg->gorilla.aral_index], pg->gorilla.writer);
                pg->gorilla.writer = NULL;
            } else {
                fatal("pgd_free() called on gorilla page with unsupported state");
                // TODO: should we support any other states?
                // if (!(pg->states & PGD_STATE_FLUSHED_TO_DISK))
                //     fatal("pgd_free() is not supported yet for pages flushed to disk");
            }

            break;
        }
        default:
            netdata_log_error("%s() - Unknown page type: %uc", __FUNCTION__, pg->type);
            break;
    }

    aral_freez(pgd_alloc_globals.aral_pgd, pg);
}

// ----------------------------------------------------------------------------
// utility functions

uint32_t pgd_type(PGD *pg)
{
    return pg->type;
}

bool pgd_is_empty(PGD *pg)
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

uint32_t pgd_slots_used(PGD *pg)
{
    if (!pg)
        return 0;

    if (pg == PGD_EMPTY)
        return 0;

    return pg->used;
}

uint32_t pgd_memory_footprint(PGD *pg)
{
    if (!pg)
        return 0;

    if (pg == PGD_EMPTY)
        return 0;

    size_t footprint = 0;
    switch (pg->type) {
        case PAGE_METRICS:
        case PAGE_TIER:
            footprint = sizeof(PGD) + pg->raw.size;
            break;
        case PAGE_GORILLA_METRICS: {
            if (pg->states & PGD_STATE_CREATED_FROM_DISK)
                footprint = sizeof(PGD) + pg->raw.size;
            else
                footprint = sizeof(PGD) + sizeof(gorilla_writer_t) + (pg->gorilla.num_buffers * GORILLA_BUFFER_SIZE);

            break;
        }
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

    switch (pg->type) {
        case PAGE_METRICS:
        case PAGE_TIER: {
            uint32_t used_size = pg->used * page_type_size[pg->type];
            internal_fatal(used_size > pg->raw.size, "Wrong disk footprint page size");
            size = used_size;

            break;
        }
        case PAGE_GORILLA_METRICS: {
            if (pg->states & PGD_STATE_CREATED_FROM_COLLECTOR ||
                pg->states & PGD_STATE_SCHEDULED_FOR_FLUSHING ||
                pg->states & PGD_STATE_FLUSHED_TO_DISK)
            {
                internal_fatal(!pg->gorilla.writer,
                               "pgd_disk_footprint() not implemented for NULL gorilla writers");

                internal_fatal(pg->gorilla.num_buffers == 0,
                               "Gorilla writer does not have any buffers");

                size = pg->gorilla.num_buffers * GORILLA_BUFFER_SIZE;

                if (pg->states & PGD_STATE_CREATED_FROM_COLLECTOR) {
                    global_statistics_tier0_disk_compressed_bytes(gorilla_writer_nbytes(pg->gorilla.writer));
                    global_statistics_tier0_disk_uncompressed_bytes(gorilla_writer_entries(pg->gorilla.writer) * sizeof(storage_number));
                }
            } else if (pg->states & PGD_STATE_CREATED_FROM_DISK) {
                size = pg->raw.size;
            } else {
                fatal("Asked disk footprint on unknown page state");
            }

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
        case PAGE_METRICS:
        case PAGE_TIER:
            memcpy(dst, pg->raw.data, dst_size);
            break;
        case PAGE_GORILLA_METRICS: {
            if ((pg->states & PGD_STATE_SCHEDULED_FOR_FLUSHING) == 0)
                fatal("Copying to extent is supported only for PGDs that are scheduled for flushing.");

            internal_fatal(!pg->gorilla.writer,
                           "pgd_copy_to_extent() not implemented for NULL gorilla writers");

            internal_fatal(pg->gorilla.num_buffers == 0,
                           "pgd_copy_to_extent() gorilla writer does not have any buffers");

            bool ok = gorilla_writer_serialize(pg->gorilla.writer, dst, dst_size);
            UNUSED(ok);
            internal_fatal(!ok,
                           "pgd_copy_to_extent() tried to serialize pg=%p, gw=%p (with dst_size=%u bytes, num_buffers=%zu)",
                           pg, pg->gorilla.writer, dst_size, pg->gorilla.num_buffers);
            break;
        }
        default:
            netdata_log_error("%s() - Unknown page type: %uc", __FUNCTION__, pg->type);
            break;
    }

    pg->states = PGD_STATE_FLUSHED_TO_DISK;
}

// ----------------------------------------------------------------------------
// data collection

void pgd_append_point(PGD *pg,
                      usec_t point_in_time_ut __maybe_unused,
                      NETDATA_DOUBLE n,
                      NETDATA_DOUBLE min_value,
                      NETDATA_DOUBLE max_value,
                      uint16_t count,
                      uint16_t anomaly_count,
                      SN_FLAGS flags,
                      uint32_t expected_slot)
{
    if (unlikely(pg->used >= pg->slots))
        fatal("DBENGINE: attempted to write beyond page size (page type %u, slots %u, used %u)",
              pg->type, pg->slots, pg->used /* FIXME:, pg->size */);

    if (unlikely(pg->used != expected_slot))
        fatal("DBENGINE: page is not aligned to expected slot (used %u, expected %u)",
              pg->used, expected_slot);

    if (!(pg->states & PGD_STATE_CREATED_FROM_COLLECTOR))
        fatal("DBENGINE: collection on page not created from a collector");

    if (pg->states & PGD_STATE_SCHEDULED_FOR_FLUSHING)
        fatal("Data collection on page already scheduled for flushing");

    switch (pg->type) {
        case PAGE_METRICS: {
            storage_number *tier0_metric_data = (storage_number *)pg->raw.data;
            storage_number t = pack_storage_number(n, flags);
            tier0_metric_data[pg->used++] = t;

            if ((pg->options & PAGE_OPTION_ALL_VALUES_EMPTY) && does_storage_number_exist(t))
                pg->options &= ~PAGE_OPTION_ALL_VALUES_EMPTY;

            break;
        }
        case PAGE_TIER: {
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
        case PAGE_GORILLA_METRICS: {
            pg->used++;
            storage_number t = pack_storage_number(n, flags);

            if ((pg->options & PAGE_OPTION_ALL_VALUES_EMPTY) && does_storage_number_exist(t))
                pg->options &= ~PAGE_OPTION_ALL_VALUES_EMPTY;

            bool ok = gorilla_writer_write(pg->gorilla.writer, t);
            if (!ok) {
                gorilla_buffer_t *new_buffer = aral_mallocz(pgd_alloc_globals.aral_gorilla_buffer[pg->gorilla.aral_index]);
                memset(new_buffer, 0, GORILLA_BUFFER_SIZE);

                gorilla_writer_add_buffer(pg->gorilla.writer, new_buffer, GORILLA_BUFFER_SLOTS);
                pg->gorilla.num_buffers += 1;
                global_statistics_gorilla_buffer_add_hot();

                ok = gorilla_writer_write(pg->gorilla.writer, t);
                internal_fatal(ok == false, "Failed to writer value in newly allocated gorilla buffer.");
            }
            break;
        }
        default:
            netdata_log_error("%s() - Unknown page type: %uc", __FUNCTION__, pg->type);
            break;
    }
}

// ----------------------------------------------------------------------------
// querying with cursor

static void pgdc_seek(PGDC *pgdc, uint32_t position)
{
    PGD *pg = pgdc->pgd;

    switch (pg->type) {
        case PAGE_METRICS:
        case PAGE_TIER:
            pgdc->slots = pgdc->pgd->used;
            break;
        case PAGE_GORILLA_METRICS: {
            if (pg->states & PGD_STATE_CREATED_FROM_DISK) {
                pgdc->slots = pgdc->pgd->slots;
                pgdc->gr = gorilla_reader_init((void *) pg->raw.data);
            } else {
                if (!(pg->states & PGD_STATE_CREATED_FROM_COLLECTOR) &&
                    !(pg->states & PGD_STATE_SCHEDULED_FOR_FLUSHING) &&
                    !(pg->states & PGD_STATE_FLUSHED_TO_DISK))
                    fatal("pgdc_seek() currently is not supported for pages created from disk.");

                if (!pg->gorilla.writer)
                    fatal("Seeking from a page without an active gorilla writer is not supported (yet).");

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
        case PAGE_METRICS: {
            storage_number *array = (storage_number *) pgdc->pgd->raw.data;
            storage_number n = array[pgdc->position++];

            sp->min = sp->max = sp->sum = unpack_storage_number(n);
            sp->flags = (SN_FLAGS)(n & SN_USER_FLAGS);
            sp->count = 1;
            sp->anomaly_count = is_storage_number_anomalous(n) ? 1 : 0;

            return true;
        }
        case PAGE_TIER: {
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
        case PAGE_GORILLA_METRICS: {
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
