#ifndef NETDATA_HEALTH_H
#define NETDATA_HEALTH_H

extern int default_health_enabled;

extern int rrdvar_compare(void *a, void *b);

typedef enum rrdvar_type {
    RRDVAR_TYPE_CALCULATED              = 1,
    RRDVAR_TYPE_TIME_T                  = 2,
    RRDVAR_TYPE_COLLECTED               = 3,
    RRDVAR_TYPE_TOTAL                   = 4,
    RRDVAR_TYPE_INT                     = 5,
    RRDVAR_TYPE_CALCULATED_ALLOCATED    = 6  // a custom variable, allocate on purpose (ie. not inherited from charts)
} RRDVAR_TYPE;

// the variables as stored in the variables indexes
// there are 3 indexes:
// 1. at each chart   (RRDSET.rrdvar_root_index)
// 2. at each context (RRDFAMILY.rrdvar_root_index)
// 3. at each host    (RRDHOST.rrdvar_root_index)
typedef struct rrdvar {
    avl avl;

    char *name;
    uint32_t hash;

    RRDVAR_TYPE type;
    void *value;

    time_t last_updated;
} RRDVAR;

// variables linked to charts
// We link variables to point to the values that are already
// calculated / processed by the normal data collection process
// This means, there will be no speed penalty for using
// these variables

typedef enum rrdvar_options {
    RRDVAR_OPTION_DEFAULT    = (0 << 0)
    // future use
} RRDVAR_OPTIONS;

typedef struct rrdsetvar {
    char *key_fullid;               // chart type.chart id.variable
    char *key_fullname;             // chart type.chart name.variable
    char *variable;                 // variable

    RRDVAR_TYPE type;
    void *value;

    RRDVAR_OPTIONS options;

    RRDVAR *var_local;
    RRDVAR *var_family;
    RRDVAR *var_host;
    RRDVAR *var_family_name;
    RRDVAR *var_host_name;

    struct rrdset *rrdset;

    struct rrdsetvar *next;
} RRDSETVAR;


// variables linked to individual dimensions
// We link variables to point the values that are already
// calculated / processed by the normal data collection process
// This means, there will be no speed penalty for using
// these variables
typedef struct rrddimvar {
    char *prefix;
    char *suffix;

    char *key_id;                   // dimension id
    char *key_name;                 // dimension name
    char *key_contextid;            // context + dimension id
    char *key_contextname;          // context + dimension name
    char *key_fullidid;             // chart type.chart id + dimension id
    char *key_fullidname;           // chart type.chart id + dimension name
    char *key_fullnameid;           // chart type.chart name + dimension id
    char *key_fullnamename;         // chart type.chart name + dimension name

    RRDVAR_TYPE type;
    void *value;

    RRDVAR_OPTIONS options;

    RRDVAR *var_local_id;
    RRDVAR *var_local_name;

    RRDVAR *var_family_id;
    RRDVAR *var_family_name;
    RRDVAR *var_family_contextid;
    RRDVAR *var_family_contextname;

    RRDVAR *var_host_chartidid;
    RRDVAR *var_host_chartidname;
    RRDVAR *var_host_chartnameid;
    RRDVAR *var_host_chartnamename;

    struct rrddim *rrddim;

    struct rrddimvar *next;
} RRDDIMVAR;

// calculated variables (defined in health configuration)
// These aggregate time-series data at fixed intervals
// (defined in their update_every member below)
// They increase the overhead of netdata.
//
// These calculations are allocated and linked (->next)
// under RRDHOST.
// Then are also linked to RRDSET (of course only when the
// chart is found, via ->rrdset_next and ->rrdset_prev).
// This double-linked list is maintained sorted at all times
// having as RRDSET.calculations the RRDCALC to be processed
// next.

#define RRDCALC_FLAG_DB_ERROR              0x00000001
#define RRDCALC_FLAG_DB_NAN                0x00000002
/* #define RRDCALC_FLAG_DB_STALE           0x00000004 */
#define RRDCALC_FLAG_CALC_ERROR            0x00000008
#define RRDCALC_FLAG_WARN_ERROR            0x00000010
#define RRDCALC_FLAG_CRIT_ERROR            0x00000020
#define RRDCALC_FLAG_RUNNABLE              0x00000040
#define RRDCALC_FLAG_NO_CLEAR_NOTIFICATION 0x80000000

typedef struct rrdcalc {
    uint32_t id;                    // the unique id of this alarm
    uint32_t next_event_id;         // the next event id that will be used for this alarm

    char *name;                     // the name of this alarm
    uint32_t hash;      

    char *exec;                     // the command to execute when this alarm switches state
    char *recipient;                // the recipient of the alarm (the first parameter to exec)

    char *chart;                    // the chart id this should be linked to
    uint32_t hash_chart;

    char *source;                   // the source of this alarm
    char *units;                    // the units of the alarm
    char *info;                     // a short description of the alarm

    int update_every;               // update frequency for the alarm

    // the red and green threshold of this alarm (to be set to the chart)
    calculated_number green;
    calculated_number red;

    // ------------------------------------------------------------------------
    // database lookup settings

    char *dimensions;               // the chart dimensions
    int group;                      // grouping method: average, max, etc.
    int before;                     // ending point in time-series
    int after;                      // starting point in time-series
    uint32_t options;               // calculation options

    // ------------------------------------------------------------------------
    // expressions related to the alarm

    EVAL_EXPRESSION *calculation;   // expression to calculate the value of the alarm
    EVAL_EXPRESSION *warning;       // expression to check the warning condition
    EVAL_EXPRESSION *critical;      // expression to check the critical condition

    // ------------------------------------------------------------------------
    // notification delay settings

    int delay_up_duration;         // duration to delay notifications when alarm raises
    int delay_down_duration;       // duration to delay notifications when alarm lowers
    int delay_max_duration;        // the absolute max delay to apply to this alarm
    float delay_multiplier;        // multiplier for all delays when alarms switch status
                                   // while now < delay_up_to

    // ------------------------------------------------------------------------
    // runtime information

    RRDCALC_STATUS status;          // the current status of the alarm

    calculated_number value;        // the current value of the alarm
    calculated_number old_value;    // the previous value of the alarm

    uint32_t rrdcalc_flags;         // check RRDCALC_FLAG_*

    time_t last_updated;            // the last update timestamp of the alarm
    time_t next_update;             // the next update timestamp of the alarm
    time_t last_status_change;      // the timestamp of the last time this alarm changed status

    time_t db_after;                // the first timestamp evaluated by the db lookup
    time_t db_before;               // the last timestamp evaluated by the db lookup

    time_t delay_up_to_timestamp;   // the timestamp up to which we should delay notifications
    int delay_up_current;           // the current up notification delay duration
    int delay_down_current;         // the current down notification delay duration
    int delay_last;                 // the last delay we used

    // ------------------------------------------------------------------------
    // variables this alarm exposes to the rest of the alarms

    RRDVAR *local;
    RRDVAR *family;
    RRDVAR *hostid;
    RRDVAR *hostname;

    // ------------------------------------------------------------------------
    // the chart this alarm it is linked to

    struct rrdset *rrdset;

    // linking of this alarm on its chart
    struct rrdcalc *rrdset_next;
    struct rrdcalc *rrdset_prev;

    struct rrdcalc *next;
} RRDCALC;

#define RRDCALC_HAS_DB_LOOKUP(rc) ((rc)->after)

// RRDCALCTEMPLATE
// these are to be applied to charts found dynamically
// based on their context.
typedef struct rrdcalctemplate {
    char *name;
    uint32_t hash_name;

    char *exec;
    char *recipient;

    char *context;
    uint32_t hash_context;

    char *family_match;
    SIMPLE_PATTERN *family_pattern;

    char *source;                   // the source of this alarm
    char *units;                    // the units of the alarm
    char *info;                     // a short description of the alarm

    int update_every;               // update frequency for the alarm

    // the red and green threshold of this alarm (to be set to the chart)
    calculated_number green;
    calculated_number red;

    // ------------------------------------------------------------------------
    // database lookup settings

    char *dimensions;               // the chart dimensions
    int group;                      // grouping method: average, max, etc.
    int before;                     // ending point in time-series
    int after;                      // starting point in time-series
    uint32_t options;               // calculation options

    // ------------------------------------------------------------------------
    // notification delay settings

    int delay_up_duration;         // duration to delay notifications when alarm raises
    int delay_down_duration;       // duration to delay notifications when alarm lowers
    int delay_max_duration;        // the absolute max delay to apply to this alarm
    float delay_multiplier;        // multiplier for all delays when alarms switch status

    // ------------------------------------------------------------------------
    // expressions related to the alarm

    EVAL_EXPRESSION *calculation;
    EVAL_EXPRESSION *warning;
    EVAL_EXPRESSION *critical;

    struct rrdcalctemplate *next;
} RRDCALCTEMPLATE;

#define RRDCALCTEMPLATE_HAS_CALCULATION(rt) ((rt)->after)

#define HEALTH_ENTRY_FLAG_PROCESSED             0x00000001
#define HEALTH_ENTRY_FLAG_UPDATED               0x00000002
#define HEALTH_ENTRY_FLAG_EXEC_RUN              0x00000004
#define HEALTH_ENTRY_FLAG_EXEC_FAILED           0x00000008
#define HEALTH_ENTRY_FLAG_SAVED                 0x10000000
#define HEALTH_ENTRY_FLAG_NO_CLEAR_NOTIFICATION 0x80000000

typedef struct alarm_entry {
    uint32_t unique_id;
    uint32_t alarm_id;
    uint32_t alarm_event_id;

    time_t when;
    time_t duration;
    time_t non_clear_duration;

    char *name;
    uint32_t hash_name;

    char *chart;
    uint32_t hash_chart;

    char *family;

    char *exec;
    char *recipient;
    time_t exec_run_timestamp;
    int exec_code;

    char *source;
    char *units;
    char *info;

    calculated_number old_value;
    calculated_number new_value;

    char *old_value_string;
    char *new_value_string;

    RRDCALC_STATUS old_status;
    RRDCALC_STATUS new_status;

    uint32_t flags;

    int delay;
    time_t delay_up_to_timestamp;

    uint32_t updated_by_id;
    uint32_t updates_id;
    
    struct alarm_entry *next;
} ALARM_ENTRY;

typedef struct alarm_log {
    uint32_t next_log_id;
    uint32_t next_alarm_id;
    unsigned int count;
    unsigned int max;
    ALARM_ENTRY *alarms;
    netdata_rwlock_t alarm_log_rwlock;
} ALARM_LOG;

#include "rrd.h"

extern void rrdsetvar_rename_all(RRDSET *st);
extern RRDSETVAR *rrdsetvar_create(RRDSET *st, const char *variable, RRDVAR_TYPE type, void *value, RRDVAR_OPTIONS options);
extern void rrdsetvar_free(RRDSETVAR *rs);

extern void rrddimvar_rename_all(RRDDIM *rd);
extern RRDDIMVAR *rrddimvar_create(RRDDIM *rd, RRDVAR_TYPE type, const char *prefix, const char *suffix, void *value, RRDVAR_OPTIONS options);
extern void rrddimvar_free(RRDDIMVAR *rs);

extern void rrdsetcalc_link_matching(RRDSET *st);
extern void rrdsetcalc_unlink(RRDCALC *rc);
extern void rrdcalctemplate_link_matching(RRDSET *st);
extern RRDCALC *rrdcalc_find(RRDSET *st, const char *name);

extern void health_init(void);
extern void *health_main(void *ptr);

extern void health_reload(void);

extern int health_variable_lookup(const char *variable, uint32_t hash, RRDCALC *rc, calculated_number *result);
extern void health_alarms2json(RRDHOST *host, BUFFER *wb, int all);
extern void health_alarm_log2json(RRDHOST *host, BUFFER *wb, uint32_t after);

void health_api_v1_chart_variables2json(RRDSET *st, BUFFER *buf);

extern RRDVAR *rrdvar_custom_host_variable_create(RRDHOST *host, const char *name);
extern void rrdvar_custom_host_variable_set(RRDHOST *host, RRDVAR *rv, calculated_number value);

extern RRDVAR *rrdvar_custom_chart_variable_create(RRDSET *st, const char *name);
extern void rrdvar_custom_chart_variable_set(RRDSET *st, RRDVAR *rv, calculated_number value);

extern void rrdvar_free_remaining_variables(RRDHOST *host, avl_tree_lock *tree_lock);

extern const char *rrdcalc_status2string(RRDCALC_STATUS status);


extern int health_alarm_log_open(RRDHOST *host);
extern void health_alarm_log_close(RRDHOST *host);
extern void health_log_rotate(RRDHOST *host);
extern void health_alarm_log_save(RRDHOST *host, ALARM_ENTRY *ae);
extern ssize_t health_alarm_log_read(RRDHOST *host, FILE *fp, const char *filename);
extern void health_alarm_log_load(RRDHOST *host);
extern void health_alarm_log(
        RRDHOST *host,
        uint32_t alarm_id,
        uint32_t alarm_event_id,
        time_t when,
        const char *name,
        const char *chart,
        const char *family,
        const char *exec,
        const char *recipient,
        time_t duration,
        calculated_number old_value,
        calculated_number new_value,
        RRDCALC_STATUS old_status,
        RRDCALC_STATUS new_status,
        const char *source,
        const char *units,
        const char *info,
        int delay,
        uint32_t flags
);

extern void health_readdir(RRDHOST *host, const char *path);
extern char *health_config_dir(void);
extern void health_reload_host(RRDHOST *host);
extern void health_alarm_log_free(RRDHOST *host);

extern void rrdcalc_free(RRDCALC *rc);
extern void rrdcalc_unlink_and_free(RRDHOST *host, RRDCALC *rc);

extern void rrdcalctemplate_free(RRDCALCTEMPLATE *rt);
extern void rrdcalctemplate_unlink_and_free(RRDHOST *host, RRDCALCTEMPLATE *rt);

extern int  rrdvar_callback_for_all_variables(RRDHOST *host, int (*callback)(void *rrdvar, void *data), void *data);

#ifdef NETDATA_HEALTH_INTERNALS
#define RRDVAR_MAX_LENGTH 1024

extern int rrdcalc_exists(RRDHOST *host, const char *chart, const char *name, uint32_t hash_chart, uint32_t hash_name);
extern uint32_t rrdcalc_get_unique_id(RRDHOST *host, const char *chart, const char *name, uint32_t *next_event_id);
extern int rrdvar_fix_name(char *variable);

extern RRDCALC *rrdcalc_create(RRDHOST *host, RRDCALCTEMPLATE *rt, const char *chart);
extern void rrdcalc_create_part2(RRDHOST *host, RRDCALC *rc);

extern RRDVAR *rrdvar_create_and_index(const char *scope, avl_tree_lock *tree, const char *name, RRDVAR_TYPE type, void *value);
extern void rrdvar_free(RRDHOST *host, avl_tree_lock *tree, RRDVAR *rv);

extern void health_alarm_log_free_one_nochecks_nounlink(ALARM_ENTRY *ae);

#endif // NETDATA_HEALTH_INTERNALS

#endif //NETDATA_HEALTH_H
