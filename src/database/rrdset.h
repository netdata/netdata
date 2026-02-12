// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_RRDSET_H
#define NETDATA_RRDSET_H

#include "libnetdata/libnetdata.h"

typedef struct rrdset RRDSET;
typedef struct rrdset_acquired RRDSET_ACQUIRED;
typedef struct ml_chart rrd_ml_chart_t;

#include "rrdset-type.h"
#include "rrdlabels.h"
#include "rrd-database-mode.h"

// --------------------------------------------------------------------------------------------------------------------

struct rrdhost;
struct rrdcalc;
struct pluginsd_rrddim;
struct rrdinstance_acquired;
struct rrdcontext_acquired;
struct storage_alignment;

// --------------------------------------------------------------------------------------------------------------------

// use this for configuration flags, not for state control
// flags are set/unset in a manner that is not thread safe
// and may lead to missing information.
typedef enum __attribute__ ((__packed__)) rrdset_flags {
    RRDSET_FLAG_DEBUG                            = (1 << 2),  // enables or disables debugging for a chart
    RRDSET_FLAG_OBSOLETE                         = (1 << 3),  // this is marked by the collector/module as obsolete
    RRDSET_FLAG_EXPORTING_SEND                   = (1 << 4),  // if set, this chart should be sent to Prometheus web API and external databases
    RRDSET_FLAG_EXPORTING_IGNORE                 = (1 << 5),  // if set, this chart should not be sent to Prometheus web API and external databases

    RRDSET_FLAG_UPSTREAM_SEND                    = (1 << 6),  // if set, this chart should be sent upstream (streaming)
    RRDSET_FLAG_UPSTREAM_IGNORE                  = (1 << 7),  // if set, this chart should not be sent upstream (streaming)

    RRDSET_FLAG_STORE_FIRST                      = (1 << 8),  // if set, do not eliminate the first collection during interpolation
    RRDSET_FLAG_HETEROGENEOUS                    = (1 << 9),  // if set, the chart is not homogeneous (dimensions in it have multiple algorithms, multipliers or dividers)
    RRDSET_FLAG_HOMOGENEOUS_CHECK                = (1 << 10), // if set, the chart should be checked to determine if the dimensions are homogeneous
    RRDSET_FLAG_HIDDEN                           = (1 << 11), // if set, do not show this chart on the dashboard, but use it for exporting
    RRDSET_FLAG_SYNC_CLOCK                       = (1 << 12), // if set, microseconds on next data collection will be ignored (the chart will be synced to now)
    RRDSET_FLAG_OBSOLETE_DIMENSIONS              = (1 << 13), // this is marked by the collector/module when a chart has obsolete dimensions

    RRDSET_FLAG_METADATA_UPDATE                  = (1 << 14), // Mark that metadata needs to be stored
    RRDSET_FLAG_ANOMALY_DETECTION                = (1 << 15), // flag to identify anomaly detection charts.
    RRDSET_FLAG_INDEXED_ID                       = (1 << 16), // the rrdset is indexed by its id
    RRDSET_FLAG_INDEXED_NAME                     = (1 << 17), // the rrdset is indexed by its name

    RRDSET_FLAG_PENDING_HEALTH_INITIALIZATION    = (1 << 18),

    RRDSET_FLAG_SENDER_REPLICATION_IN_PROGRESS   = (1 << 19), // the sending side has replication in progress
    RRDSET_FLAG_SENDER_REPLICATION_FINISHED      = (1 << 20), // the sending side has completed replication
    RRDSET_FLAG_RECEIVER_REPLICATION_IN_PROGRESS = (1 << 21), // the receiving side has replication in progress
    RRDSET_FLAG_RECEIVER_REPLICATION_FINISHED    = (1 << 22), // the receiving side has completed replication

    RRDSET_FLAG_BACKFILLED_HIGH_TIERS            = (1 << 23), // we have backfilled this chart

    RRDSET_FLAG_UPSTREAM_SEND_VARIABLES          = (1 << 24), // a custom variable has been updated and needs to be exposed to parent

    RRDSET_FLAG_COLLECTION_FINISHED              = (1 << 25), // when set, data collection is not available for this chart

    RRDSET_FLAG_HAS_RRDCALC_LINKED               = (1 << 26), // this chart has at least one rrdcal linked
} RRDSET_FLAGS;

// --------------------------------------------------------------------------------------------------------------------
// flags

#define rrdset_flag_get(st)                         atomic_flags_get(&((st)->flags))
#define rrdset_flag_check(st, flag)                 atomic_flags_check(&((st)->flags), flag)
#define rrdset_flag_set(st, flag)                   atomic_flags_set(&((st)->flags), flag)
#define rrdset_flag_clear(st, flag)                 atomic_flags_clear(&((st)->flags), flag)
#define rrdset_flag_set_and_clear(st, set, clear)   atomic_flags_set_and_clear(&((st)->flags), set, clear)

#define rrdset_is_replicating(st) (rrdset_flag_check(st, RRDSET_FLAG_SENDER_REPLICATION_IN_PROGRESS|RRDSET_FLAG_RECEIVER_REPLICATION_IN_PROGRESS) \
    && !rrdset_flag_check(st, RRDSET_FLAG_SENDER_REPLICATION_FINISHED|RRDSET_FLAG_RECEIVER_REPLICATION_FINISHED))

#define rrdset_is_discoverable(st) (rrdset_is_replicating(st) || !rrdset_flag_check(st, RRDSET_FLAG_OBSOLETE))

// --------------------------------------------------------------------------------------------------------------------

struct rrdset {
    nd_uuid_t chart_uuid;                             // the global UUID for this chart

    // ------------------------------------------------------------------------
    // chart configuration

    struct {
        STRING *type;                               // the type of {type}.{id}
        STRING *id;                                 // the id of {type}.{id}
        STRING *name;                               // the name of {type}.{name}
    } parts;

    STRING *id;                                     // the unique ID of the rrdset as {type}.{id}
    STRING *name;                                   // the unique name of the rrdset as {type}.{name}
    STRING *family;                                 // grouping sets under the same family
    STRING *title;                                  // title shown to user
    STRING *units;                                  // units of measurement
    STRING *context;                                // the template of this data set
    STRING *plugin_name;                            // the name of the plugin that generated this
    STRING *module_name;                            // the name of the plugin module that generated this

    int32_t priority;                               // the sorting priority of this chart
    int32_t update_every;                           // data collection frequency

    RRDLABELS *rrdlabels;                           // chart labels

    uint32_t version;                               // the metadata version (auto-increment)

    RRDSET_TYPE chart_type;                         // line, area, stacked

    // ------------------------------------------------------------------------
    // operational state members

    RRD_DB_MODE rrd_memory_mode;                    // the db mode of this rrdset
    uint16_t collection_modulo;                     // tier1/2 spread over time
    RRDSET_FLAGS flags;                             // flags

    pid_t collector_tid;

    DICTIONARY *rrddim_root_index;                  // dimensions index

    rrd_ml_chart_t *ml_chart;

    struct storage_alignment *smg[RRD_STORAGE_TIERS];

    // ------------------------------------------------------------------------
    // linking to siblings and parents

    struct rrdhost *rrdhost;                            // pointer to RRDHOST this chart belongs to

    struct {
        struct rrdinstance_acquired *rrdinstance;              // the rrdinstance of this chart
        struct rrdcontext_acquired *rrdcontext;                // the rrdcontext this chart belongs to
        bool collected;
    } rrdcontexts;

    // ------------------------------------------------------------------------
    // data collection members

    SPINLOCK data_collection_lock;

    uint32_t counter;                               // the number of times we added values to this database
    uint32_t counter_done;                          // the number of times rrdset_done() has been called

    time_t last_accessed_time_s;                    // the last time this RRDSET has been accessed
    usec_t usec_since_last_update;                  // the time in microseconds since the last collection of data

    struct timeval last_updated;                    // when this data set was last updated (updated every time the rrd_stats_done() function)
    struct timeval last_collected_time;             // when did this data set last collected values

    size_t rrdlabels_last_saved_version;

    DICTIONARY *functions_view;                     // collector functions this rrdset supports, can be NULL

    // ------------------------------------------------------------------------
    // data collection - streaming to parents, temp variables

    struct {
        struct {
            uint32_t sent_version;
            uint32_t chart_slot;
            uint32_t dim_last_slot_used;
#ifdef REPLICATION_TRACKING
            REPLAY_WHO who;
#endif
            time_t resync_time_s;                   // the timestamp up to which we should resync clock upstream
        } snd;

        struct {
#ifdef REPLICATION_TRACKING
            REPLAY_WHO who;
#endif
        } rcv;
    } stream;

    // ------------------------------------------------------------------------
    // db mode SAVE, MAP specifics
    // TODO - they should be managed by storage engine
    //        (RRDSET_DB_STATE ptr to an undefined structure, and a call to clean this up during destruction)

    struct {
        int32_t entries;                                // total number of entries in the data set
        int32_t current_entry;                          // the entry that is currently being updated
        // it goes around in a round-robin fashion
    } db;

    // ------------------------------------------------------------------------
    // exporting to 3rd party time-series members
    // TODO - they should be managed by exporting engine
    //        (RRDSET_EXPORTING_STATE ptr to an undefined structure, and a call to clean this up during destruction)

    RRDSET_FLAGS *exporting_flags;                  // array of flags for exporting connector instances

    // ------------------------------------------------------------------------
    // health monitoring members
    // TODO - they should be managed by health
    //        (RRDSET_HEALTH_STATE ptr to an undefined structure, and a call to clean this up during destruction)

    NETDATA_DOUBLE green;                           // green threshold for this chart
    NETDATA_DOUBLE red;                             // red threshold for this chart

    DICTIONARY *rrdvars;                            // RRDVAR index for this chart

    struct {
        RW_SPINLOCK spinlock;                       // protection for RRDCALC *base
        struct rrdcalc *base;                       // double linked list of RRDCALC related to this RRDSET
    } alerts;

    struct {
        SPINLOCK spinlock; // used only for cleanup
        pid_t collector_tid;
        bool dims_with_slots;
        bool set;
        uint32_t pos;
        int32_t last_slot;
        uint32_t size;
        struct pluginsd_rrddim *prd_array;
    } pluginsd;

#ifdef NETDATA_LOG_REPLICATION_REQUESTS
    struct {
        bool log_next_data_collection;
        bool start_streaming;
        time_t after;
        time_t before;
    } replay;
#endif // NETDATA_LOG_REPLICATION_REQUESTS

    // Replication stuck detection - outside debug flag for production safety
    uint8_t replication_empty_response_count;  // track consecutive empty responses

    SPINLOCK destroy_lock;
};

// --------------------------------------------------------------------------------------------------------------------

#define rrdset_plugin_name(st) string2str((st)->plugin_name)
#define rrdset_module_name(st) string2str((st)->module_name)
#define rrdset_units(st) string2str((st)->units)
#define rrdset_parts_type(st) string2str((st)->parts.type)
#define rrdset_family(st) string2str((st)->family)
#define rrdset_title(st) string2str((st)->title)
#define rrdset_context(st) string2str((st)->context)
#define rrdset_name(st) string2str((st)->name)
#define rrdset_id(st) string2str((st)->id)

// --------------------------------------------------------------------------------------------------------------------

static inline uint32_t rrdset_metadata_version(RRDSET *st) {
    return __atomic_load_n(&st->version, __ATOMIC_RELAXED);
}

static inline uint32_t rrdset_metadata_upstream_version(RRDSET *st) {
    return __atomic_load_n(&st->stream.snd.sent_version, __ATOMIC_RELAXED);
}

static inline void rrdset_metadata_exposed_upstream(RRDSET *st, uint32_t version) {
    __atomic_store_n(&st->stream.snd.sent_version, version, __ATOMIC_RELAXED);
}

static inline bool rrdset_check_upstream_exposed(RRDSET *st) {
    return rrdset_metadata_version(st) == rrdset_metadata_upstream_version(st);
}

// --------------------------------------------------------------------------------------------------------------------
// these loop macros make sure the linked list is accessed with the right lock

#define rrdset_foreach_read(st, host) \
    dfe_start_read((host)->rrdset_root_index, st)

#define rrdset_foreach_write(st, host) \
    dfe_start_write((host)->rrdset_root_index, st)

#define rrdset_foreach_reentrant(st, host) \
    dfe_start_reentrant((host)->rrdset_root_index, st)

#define rrdset_foreach_done(st) \
    dfe_done(st)

#define rrdset_number_of_dimensions(st) \
    dictionary_entries((st)->rrddim_root_index)

// --------------------------------------------------------------------------------------------------------------------

void rrdset_metadata_updated(RRDSET *st);

// --------------------------------------------------------------------------------------------------------------------

#ifdef NETDATA_INTERNAL_CHECKS
#define rrdset_debug(st, fmt, args...) do { \
        if(unlikely(debug_flags & D_RRD_STATS && rrdset_flag_check(st, RRDSET_FLAG_DEBUG))) \
            netdata_logger(NDLS_DEBUG, NDLP_DEBUG, __FILE__, __FUNCTION__, __LINE__, "%s: " fmt, rrdset_name(st), ##args); \
    } while(0)
#else
#define rrdset_debug(st, fmt, args...) debug_dummy()
#endif

// --------------------------------------------------------------------------------------------------------------------

void rrdset_update_heterogeneous_flag(RRDSET *st);

void rrdset_is_obsolete___safe_from_collector_thread(RRDSET *st);
void rrdset_isnot_obsolete___safe_from_collector_thread(RRDSET *st);

// checks if the RRDSET should be offered to viewers
#define rrdset_is_available_for_viewers(st) ( \
    !rrdset_flag_check(st, RRDSET_FLAG_HIDDEN) && \
    !rrdset_flag_check(st, RRDSET_FLAG_OBSOLETE) && \
    rrdset_number_of_dimensions(st) &&        \
    (st)->rrd_memory_mode != RRD_DB_MODE_NONE \
 )

#define rrdset_is_available_for_exporting_and_alarms(st) ( \
    !rrdset_flag_check(st, RRDSET_FLAG_OBSOLETE) &&        \
    rrdset_number_of_dimensions(st)                        \
 )

time_t rrdset_first_entry_s(RRDSET *st);
time_t rrdset_first_entry_s_of_tier(RRDSET *st, size_t tier);
time_t rrdset_last_entry_s(RRDSET *st);
time_t rrdset_last_entry_s_of_tier(RRDSET *st, size_t tier);

void rrdset_get_retention_of_tier_for_collected_chart(RRDSET *st, time_t *first_time_s, time_t *last_time_s, time_t now_s, size_t tier);

void rrdset_update_rrdlabels(RRDSET *st, RRDLABELS *new_rrdlabels);

#include "rrdset-index-name.h"
#include "rrdset-index-id.h"
#include "rrdset-slots.h"
#include "rrdset-collection.h"

#endif //NETDATA_RRDSET_H
