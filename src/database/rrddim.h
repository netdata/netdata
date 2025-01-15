// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_RRDDIM_H
#define NETDATA_RRDDIM_H

#include "libnetdata/libnetdata.h"
#include "rrd-algorithm.h"

typedef struct rrddim RRDDIM;
typedef struct rrddim_acquired RRDDIM_ACQUIRED;
typedef struct ml_dimension rrd_ml_dimension_t;
typedef struct rrdmetric_acquired RRDMETRIC_ACQUIRED;

#include "rrdset.h"

// options are permanent configuration options (no atomics to alter/access them)
typedef enum __attribute__ ((__packed__)) rrddim_options {
    RRDDIM_OPTION_NONE                              = 0,
    RRDDIM_OPTION_HIDDEN                            = (1 << 0), // this dimension will not be offered to callers
    RRDDIM_OPTION_DONT_DETECT_RESETS_OR_OVERFLOWS   = (1 << 1), // do not offer RESET or OVERFLOW info to callers
    RRDDIM_OPTION_BACKFILLED_HIGH_TIERS             = (1 << 2), // when set, we have backfilled higher tiers
    RRDDIM_OPTION_UPDATED                           = (1 << 3), // single-threaded collector updated flag

    // this is 8-bit
} RRDDIM_OPTIONS;

#define rrddim_option_check(rd, option) ((rd)->collector.options & (option))
#define rrddim_option_set(rd, option)   (rd)->collector.options |= (option)
#define rrddim_option_clear(rd, option) (rd)->collector.options &= ~(option)

// flags are runtime changing status flags (atomics are required to alter/access them)
typedef enum __attribute__ ((__packed__)) rrddim_flags {
    RRDDIM_FLAG_NONE                            = 0,

    RRDDIM_FLAG_OBSOLETE                        = (1 << 0),  // this is marked by the collector/module as obsolete
    // No new values have been collected for this dimension since agent start, or it was marked RRDDIM_FLAG_OBSOLETE at
    // least rrdset_free_obsolete_time seconds ago.

    RRDDIM_FLAG_ARCHIVED                        = (1 << 1),
    RRDDIM_FLAG_METADATA_UPDATE                 = (1 << 2),  // Metadata needs to go to the database

    RRDDIM_FLAG_META_HIDDEN                     = (1 << 3),  // Status of hidden option in the metadata database
    RRDDIM_FLAG_ML_MODEL_LOAD                   = (1 << 4),  // Do ML LOAD for this dimension

    // this is 8 bit
} RRDDIM_FLAGS;

#define rrddim_flag_get(rd)                         atomic_flags_get(&((rd)->flags))
#define rrddim_flag_check(rd, flag)                 atomic_flags_check(&((rd)->flags), flag)
#define rrddim_flag_set(rd, flag)                   atomic_flags_set(&((rd)->flags), flag)
#define rrddim_flag_clear(rd, flag)                 atomic_flags_clear(&((rd)->flags), flag)
#define rrddim_flag_set_and_clear(rd, set, clear)   atomic_flags_set_and_clear(&((rd)->flags), set, clear)

struct rrddim {
    UUIDMAP_ID uuid;

    // ------------------------------------------------------------------------
    // dimension definition

    STRING *id;                                     // the id of this dimension (for internal identification)
    STRING *name;                                   // the name of this dimension (as presented to user)

    RRD_ALGORITHM algorithm;                        // the algorithm that is applied to add new collected values
    RRD_DB_MODE rrd_memory_mode;                // the memory mode for this dimension
    RRDDIM_FLAGS flags;                             // run time changing status flags

    int32_t multiplier;                             // the multiplier of the collected values
    int32_t divisor;                                // the divider of the collected values

    // ------------------------------------------------------------------------
    // operational state members

    struct rrdset *rrdset;
    rrd_ml_dimension_t *ml_dimension;               // machine learning data about this dimension

    struct {
        RRDMETRIC_ACQUIRED *rrdmetric;              // the rrdmetric of this dimension
        bool collected;
    } rrdcontexts;

#ifdef NETDATA_LOG_COLLECTION_ERRORS
    usec_t rrddim_store_metric_last_ut;             // the timestamp we last called rrddim_store_metric()
    size_t rrddim_store_metric_count;               // the rrddim_store_metric() counter
    const char *rrddim_store_metric_last_caller;    // the name of the function that last called rrddim_store_metric()
#endif

    // ------------------------------------------------------------------------
    // db mode RAM, ALLOC, NONE specifics
    // TODO - they should be managed by storage engine
    //        (RRDDIM_DB_STATE ptr to an undefined structure, and a call to clean this up during destruction)

    struct {
        size_t memsize;                             // the memory allocated for this dimension (without RRDDIM)
        storage_number *data;                       // the array of values
    } db;

    // ------------------------------------------------------------------------
    // streaming

    struct {
        struct {
            uint32_t sent_version;
            uint32_t dim_slot;
        } snd;
    } stream;

    // ------------------------------------------------------------------------
    // data collection members

    struct {
        RRDDIM_OPTIONS options;                         // permanent configuration options

        uint32_t counter;                               // the number of times we added values to this rrddim

        collected_number collected_value;               // the current value, as collected - resets to 0 after being used
        collected_number collected_value_max;           // the absolute maximum of the collected value
        collected_number last_collected_value;          // the last value that was collected, after being processed

        struct timeval last_collected_time;             // when was this dimension last updated
                                            // this is actual date time we updated the last_collected_value
                                            // THIS IS DIFFERENT FROM THE SAME MEMBER OF RRDSET

        NETDATA_DOUBLE calculated_value;                // the current calculated value, after applying the algorithm - resets to zero after being used
        NETDATA_DOUBLE last_calculated_value;           // the last calculated value processed

        NETDATA_DOUBLE last_stored_value;               // the last value as stored in the database (after interpolation)
    } collector;

    // ------------------------------------------------------------------------

    struct rrddim_tier tiers[];                         // our tiers of databases
};

size_t rrddim_size(void);

#define rrddim_id(rd) string2str((rd)->id)
#define rrddim_name(rd) string2str((rd) ->name)

#define rrddim_check_updated(rd) ((rd)->collector.options & RRDDIM_OPTION_UPDATED)
#define rrddim_set_updated(rd) (rd)->collector.options |= RRDDIM_OPTION_UPDATED
#define rrddim_clear_updated(rd) (rd)->collector.options &= ~RRDDIM_OPTION_UPDATED

#define rrddim_foreach_read(rd, st) \
    dfe_start_read((st)->rrddim_root_index, rd)

#define rrddim_foreach_done(rd) \
    dfe_done(rd)

struct pluginsd_rrddim {
    RRDDIM_ACQUIRED *rda;
    RRDDIM *rd;
    const char *id;
};

static inline uint32_t rrddim_metadata_version(RRDDIM *rd) {
    // the metadata version of the dimension, is the version of the chart
    return rrdset_metadata_version(rd->rrdset);
}

static inline uint32_t rrddim_metadata_upstream_version(RRDDIM *rd) {
    return __atomic_load_n(&rd->stream.snd.sent_version, __ATOMIC_RELAXED);
}

void rrddim_metadata_updated(RRDDIM *rd);

static inline void rrddim_metadata_exposed_upstream(RRDDIM *rd, uint32_t version) {
    __atomic_store_n(&rd->stream.snd.sent_version, version, __ATOMIC_RELAXED);
}

static inline void rrddim_metadata_exposed_upstream_clear(RRDDIM *rd) {
    __atomic_store_n(&rd->stream.snd.sent_version, 0, __ATOMIC_RELAXED);
}

static inline bool rrddim_check_upstream_exposed(RRDDIM *rd) {
    return rrddim_metadata_upstream_version(rd) != 0;
}

// the collector sets the exposed flag, but anyone can remove it
// still, it can be removed, after the collector has finished
// so, it is safe to check it without atomics
static inline bool rrddim_check_upstream_exposed_collector(RRDDIM *rd) {
    return rd->rrdset->version == rd->stream.snd.sent_version;
}

void rrddim_index_init(RRDSET *st);
void rrddim_index_destroy(RRDSET *st);

time_t rrddim_first_entry_s(RRDDIM *rd);
time_t rrddim_first_entry_s_of_tier(RRDDIM *rd, size_t tier);
time_t rrddim_last_entry_s(RRDDIM *rd);
time_t rrddim_last_entry_s_of_tier(RRDDIM *rd, size_t tier);

RRDDIM *rrddim_add_custom(RRDSET *st
                          , const char *id
                          , const char *name
                          , collected_number multiplier
                          , collected_number divisor
                          , RRD_ALGORITHM algorithm
                          , RRD_DB_MODE memory_mode
);

#define rrddim_add(st, id, name, multiplier, divisor, algorithm) \
    rrddim_add_custom(st, id, name, multiplier, divisor, algorithm, (st)->rrd_memory_mode)

int rrddim_reset_name(RRDSET *st, RRDDIM *rd, const char *name);
int rrddim_set_algorithm(RRDSET *st, RRDDIM *rd, RRD_ALGORITHM algorithm);
int rrddim_set_multiplier(RRDSET *st, RRDDIM *rd, int32_t multiplier);
int rrddim_set_divisor(RRDSET *st, RRDDIM *rd, int32_t divisor);

RRDDIM *rrddim_find(RRDSET *st, const char *id);
RRDDIM_ACQUIRED *rrddim_find_and_acquire(RRDSET *st, const char *id);
RRDDIM *rrddim_acquired_to_rrddim(RRDDIM_ACQUIRED *rda);
void rrddim_acquired_release(RRDDIM_ACQUIRED *rda);
RRDDIM *rrddim_find_active(RRDSET *st, const char *id);

int rrddim_hide(RRDSET *st, const char *id);
int rrddim_unhide(RRDSET *st, const char *id);

void rrddim_is_obsolete___safe_from_collector_thread(RRDSET *st, RRDDIM *rd);
void rrddim_isnot_obsolete___safe_from_collector_thread(RRDSET *st, RRDDIM *rd);

collected_number rrddim_timed_set_by_pointer(RRDSET *st, RRDDIM *rd, struct timeval collected_time, collected_number value);
collected_number rrddim_set_by_pointer(RRDSET *st, RRDDIM *rd, collected_number value);
collected_number rrddim_set(RRDSET *st, const char *id, collected_number value);

bool rrddim_finalize_collection_and_check_retention(RRDDIM *rd);

void rrddim_free(RRDSET *st, RRDDIM *rd);

static inline double rrddim_get_last_stored_value(RRDDIM *rd_dim, double *max_value, double div) {
    if (!rd_dim)
        return NAN;

    if (isnan(div) || div == 0.0)
        div = 1.0;

    double value = rd_dim->collector.last_stored_value / div;
    value = ABS(value);

    *max_value = MAX(*max_value, value);

    return value;
}

#endif //NETDATA_RRDDIM_H
