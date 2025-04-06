// SPDX-License-Identifier: GPL-3.0-or-later

#include "database/rrd.h"
#include "web/api/queries/rrdr.h"
#include "health_prototypes.h"

#ifndef NETDATA_RRDCALC_H
#define NETDATA_RRDCALC_H 1

// calculated variables (defined in health configuration)
// These aggregate time-series data at fixed intervals
// (defined in their update_every member below)
// They increase the overhead of netdata.
//
// These calculations are stored under RRDHOST.
// Then are also linked to RRDSET (of course only when a
// matching chart is found).

typedef enum rrdcalc_status {
    RRDCALC_STATUS_REMOVED       = -2,
    RRDCALC_STATUS_UNDEFINED     = -1,
    RRDCALC_STATUS_UNINITIALIZED =  0,
    RRDCALC_STATUS_CLEAR         =  1,
    RRDCALC_STATUS_RAISED        =  2, // DO NOT CHANGE THESE NUMBERS
    RRDCALC_STATUS_WARNING       =  3, // DO NOT CHANGE THESE NUMBERS
    RRDCALC_STATUS_CRITICAL      =  4, // DO NOT CHANGE THESE NUMBERS
} RRDCALC_STATUS;

typedef enum {
    RRDCALC_FLAG_DB_ERROR                   = (1 << 0),
    RRDCALC_FLAG_DB_NAN                     = (1 << 1),
    // RRDCALC_FLAG_DB_STALE                   = (1 << 2),
    RRDCALC_FLAG_CALC_ERROR                 = (1 << 3),
    RRDCALC_FLAG_WARN_ERROR                 = (1 << 4),
    RRDCALC_FLAG_CRIT_ERROR                 = (1 << 5),
    RRDCALC_FLAG_RUNNABLE                   = (1 << 6),
    RRDCALC_FLAG_DISABLED                   = (1 << 7),
    RRDCALC_FLAG_SILENCED                   = (1 << 8),
    RRDCALC_FLAG_RUN_ONCE                   = (1 << 9),
} RRDCALC_FLAGS;
void rrdcalc_flags_to_json_array(BUFFER *wb, const char *key, RRDCALC_FLAGS flags);

#define RRDCALC_ALL_OPTIONS_EXCLUDING_THE_RRDR_ONES (RRDCALC_OPTION_NO_CLEAR_NOTIFICATION)

struct rrdcalc {
    uint32_t id;                    // the unique id of this alarm
    uint32_t next_event_id;         // the next event id that will be used for this alarm

    STRING *key;                    // the unique key in the host's rrdcalc_root_index
    STRING *chart;                  // the chart id this should be linked to

    struct rrd_alert_match match;
    struct rrd_alert_config config;

    // ------------------------------------------------------------------------
    // runtime information

    STRING *summary;       // the original summary field before any variable replacement
    STRING *info;          // the original info field before any variable replacement

    RRDCALC_STATUS old_status;      // the old status of the alarm
    RRDCALC_STATUS status;          // the current status of the alarm

    NETDATA_DOUBLE value;           // the current value of the alarm
    NETDATA_DOUBLE old_value;       // the previous value of the alarm
    NETDATA_DOUBLE last_status_change_value; // the value at the last status change

    RRDCALC_FLAGS run_flags;        // check RRDCALC_FLAG_*

    time_t last_updated;            // the last update timestamp of the alarm
    time_t next_update;             // the next update timestamp of the alarm
    time_t last_status_change;      // the timestamp of the last time this alarm changed status
    time_t last_repeat;             // the last time the alarm got repeated
    uint32_t times_repeat;          // number of times the alarm got repeated

    time_t db_after;                // the first timestamp evaluated by the db lookup
    time_t db_before;               // the last timestamp evaluated by the db lookup

    time_t delay_up_to_timestamp;   // the timestamp up to which we should delay notifications
    int delay_up_current;           // the current up notification delay duration
    int delay_down_current;         // the current down notification delay duration
    int delay_last;                 // the last delay we used

    // ------------------------------------------------------------------------
    // the chart this alarm it is linked to

    size_t labels_version;
    struct rrdset *rrdset;

    struct rrdcalc *next;
    struct rrdcalc *prev;
};

#define rrdcalc_name(rc) string2str((rc)->config.name)
#define rrdcalc_chart_name(rc) string2str((rc)->chart)
#define rrdcalc_exec(rc) string2str((rc)->config.exec)
#define rrdcalc_recipient(rc) string2str((rc)->config.recipient)
#define rrdcalc_classification(rc) string2str((rc)->config.classification)
#define rrdcalc_component(rc) string2str((rc)->config.component)
#define rrdcalc_type(rc) string2str((rc)->config.type)
#define rrdcalc_source(rc) string2str((rc)->config.source)
#define rrdcalc_units(rc) string2str((rc)->config.units)
#define rrdcalc_dimensions(rc) string2str((rc)->config.dimensions)

#define foreach_rrdcalc_in_rrdhost_write(host, rc) \
    dfe_start_write((host)->rrdcalc_root_index, rc) \

#define foreach_rrdcalc_in_rrdhost_read(host, rc) \
    dfe_start_read((host)->rrdcalc_root_index, rc) \

#define foreach_rrdcalc_in_rrdhost_reentrant(host, rc) \
    dfe_start_reentrant((host)->rrdcalc_root_index, rc)

#define foreach_rrdcalc_in_rrdhost_done(rc) \
    dfe_done(rc)

#define RRDCALC_HAS_DB_LOOKUP(rc) ((rc)->config.after)

void rrdcalc_update_info_using_rrdset_labels(RRDCALC *rc);

const RRDCALC_ACQUIRED *rrdcalc_from_rrdset_get(RRDSET *st, const char *alert_name);
void rrdcalc_from_rrdset_release(RRDSET *st, const RRDCALC_ACQUIRED *rca);
RRDCALC *rrdcalc_acquired_to_rrdcalc(const RRDCALC_ACQUIRED *rca);

const char *rrdcalc_status2string(RRDCALC_STATUS status);

uint32_t rrdcalc_get_unique_id(RRDHOST *host, STRING *chart, STRING *name, uint32_t *next_event_id, nd_uuid_t *config_hash_id);

static inline int rrdcalc_isrepeating(RRDCALC *rc) {
    if (unlikely(rc->config.warn_repeat_every > 0 || rc->config.crit_repeat_every > 0)) {
        return 1;
    }
    return 0;
}

void rrdcalc_unlink_and_delete_all_rrdset_alerts(RRDSET *st);
void rrdcalc_delete_all(RRDHOST *host);

void rrdcalc_rrdhost_index_init(RRDHOST *host);
void rrdcalc_rrdhost_index_destroy(RRDHOST *host);

void rrdcalc_unlink_and_delete(RRDHOST *host, RRDCALC *rc, bool having_ll_wrlock);

#define RRDCALC_VAR_MAX 100
#define RRDCALC_VAR_FAMILY "${family}"
#define RRDCALC_VAR_LABEL "${label:"
#define RRDCALC_VAR_LABEL_LEN (sizeof(RRDCALC_VAR_LABEL)-1)

void rrdcalc_child_disconnected(RRDHOST *host);

#endif //NETDATA_RRDCALC_H
