// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef DBENGINE_PAGE_H
#define DBENGINE_PAGE_H

#include "libnetdata/libnetdata.h"

typedef struct pgd_cursor {
    struct pgd *pgd;
    uint32_t position;
} PGDC;

#include "rrdengine.h"

typedef struct pgd PGD;

#define PGD_EMPTY (PGD *)(-1)

void pgd_init(void);

PGD *pgd_create(uint8_t type, uint32_t slots);
PGD *pgd_create_and_copy(uint8_t type, void *base, uint32_t size);

void pgd_free(PGD *pg);

void pgd_append_point(PGD *pg,
                      usec_t point_in_time_ut,
                      NETDATA_DOUBLE n,
                      NETDATA_DOUBLE min_value,
                      NETDATA_DOUBLE max_value,
                      uint16_t count,
                      uint16_t anomaly_count,
                      SN_FLAGS flags,
                      uint32_t expected_slot);

bool pgd_is_empty(PGD *pg);

uint32_t pgd_length(PGD *pg);
uint32_t pgd_footprint(PGD *pg);

void *pgd_raw_data_pointer(PGD *pg);
uint32_t pgd_type(PGD *pg);
uint32_t pgd_slots(PGD *pg);

typedef enum __attribute__((packed)) {
    PGD_CURSOR_OK,
    PGD_CURSOR_NO_MORE_DATA,
} PGD_CURSOR_RESULT;

static inline void pgdc_seek(PGDC *pgdc) {
    ;
}

static inline void pgdc_reset(PGDC *pgdc, PGD *pgd, uint32_t position) {
    pgdc->pgd = pgd;
    pgdc->position = position;

    if(pgd && pgd != PGD_EMPTY)
        pgdc_seek(pgdc);
}

#define pgdc_clear(pgdc) pgdc_reset(pgdc, NULL, UINT32_MAX)

static inline bool pgdc_get_next_point(PGDC *pgdc, uint32_t expected_position, STORAGE_POINT *sp) {
    if(!pgdc->pgd || pgdc->pgd == PGD_EMPTY || pgdc->position >= pgd_slots(pgdc->pgd)) {
        storage_point_empty(*sp, sp->start_time_s, sp->end_time_s);
        return false;
    }

    internal_fatal(pgdc->position != expected_position, "Wrong expected cursor position");

    switch(pgd_type(pgdc->pgd)) {
        case PAGE_METRICS: {
            storage_number *array = (storage_number *)pgd_raw_data_pointer(pgdc->pgd);
            storage_number n = array[pgdc->position++];
            sp->min = sp->max = sp->sum = unpack_storage_number(n);
            sp->flags = (SN_FLAGS)(n & SN_USER_FLAGS);
            sp->count = 1;
            sp->anomaly_count = is_storage_number_anomalous(n) ? 1 : 0;
            return true;
        }
        break;

        case PAGE_TIER: {
            storage_number_tier1_t *array = (storage_number_tier1_t *)pgd_raw_data_pointer(pgdc->pgd);
            storage_number_tier1_t n = array[pgdc->position++];
            sp->flags = n.anomaly_count ? SN_FLAG_NONE : SN_FLAG_NOT_ANOMALOUS;
            sp->count = n.count;
            sp->anomaly_count = n.anomaly_count;
            sp->min = n.min_value;
            sp->max = n.max_value;
            sp->sum = n.sum_value;
            return true;
        }
        break;

            // we don't know this page type
        default: {
            static bool logged = false;
            if(!logged) {
                netdata_log_error("DBENGINE: unknown page type %d found. Cannot decode it. Ignoring its metrics.", pgd_type(pgdc->pgd));
                logged = true;
            }
            storage_point_empty(*sp, sp->start_time_s, sp->end_time_s);
            return false;
        }
        break;
    }
}

#endif // DBENGINE_PAGE_H
