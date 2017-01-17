#ifndef NETDATA_HEALTH_H
#define NETDATA_HEALTH_H

extern int health_enabled;

extern int rrdvar_compare(void *a, void *b);

#define RRDVAR_TYPE_CALCULATED              1
#define RRDVAR_TYPE_TIME_T                  2
#define RRDVAR_TYPE_COLLECTED               3
#define RRDVAR_TYPE_TOTAL                   4
#define RRDVAR_TYPE_INT                     5
#define RRDVAR_TYPE_CALCULATED_ALLOCATED    6


/// \brief the variables as stored in the variables indexes.
///
/// there are 3 indexes:
/// 1. at each chart   (RRDSET.variables_root_index)
/// 2. at each context (RRDFAMILY.variables_root_index)
/// 3. at each host    (RRDHOST.variables_root_index)
typedef struct rrdvar {
    avl avl;       ///< the tree

    char *name;    ///< variable name
    uint32_t hash; ///< hash of `name`

    int type;    ///< RRDVAR_TYPE_* 
    void *value; ///< the value

    time_t last_updated; ///< last updated timestamp
} RRDVAR;

/// \brief variables linked to charts.
///
/// We link variables to point to the values that are already
/// calculated / processed by the normal data collection process
/// This means, there will be no speed penalty for using
/// these variables
typedef struct rrdsetvar {
    char *key_fullid;           ///< chart type.chart id.variable
    char *key_fullname;         ///< chart type.chart name.variable
    char *variable;             ///< variable

    int type;    ///< RRDVAR_TYPE_*
    void *value; ///< the value

    uint32_t options; ///< \deprecated Not used.

    RRDVAR *var_local;       ///< local var
    RRDVAR *var_family;      ///< var family
    RRDVAR *var_host;        ///< host of var
    RRDVAR *var_family_name; ///< family name
    RRDVAR *var_host_name;   ///< host name

    struct rrdset *rrdset;   ///< set the variable is mapped to

    struct rrdsetvar *next;  ///< next variable in the list
} RRDSETVAR;


/// \brief variables linked to individual dimensions
///
/// We link variables to point the values that are already
/// calculated / processed by the normal data collection process
/// This means, there will be no speed penalty for using
/// these variables
typedef struct rrddimvar {
    char *prefix; ///< prefix bevore name
    char *suffix; ///< suffix after name

    char *key_id;                   ///< dimension id
    char *key_name;                 ///< dimension name
    char *key_contextid;            ///< context + dimension id
    char *key_contextname;          ///< context + dimension name
    char *key_fullidid;             ///< chart type.chart id + dimension id
    char *key_fullidname;           ///< chart type.chart id + dimension name
    char *key_fullnameid;           ///< chart type.chart name + dimension id
    char *key_fullnamename;         ///< chart type.chart name + dimension name

    int type;    ///< RRDVAR_TYPE_*
    void *value; ///< variable value

    uint32_t options; ///< \deprecated

    RRDVAR *var_local_id;   ///< local id
    RRDVAR *var_local_name; ///< local name

    RRDVAR *var_family_id;          ///< family id
    RRDVAR *var_family_name;        ///< family name
    RRDVAR *var_family_contextid;   ///< family context id
    RRDVAR *var_family_contextname; ///< family context name

    RRDVAR *var_host_chartidid;     ///< host chart id
    RRDVAR *var_host_chartidname;   ///< host chart id name
    RRDVAR *var_host_chartnameid;   ///< host chart name id
    RRDVAR *var_host_chartnamename; ///< host chart name name

    struct rrddim *rrddim; ///< dimension mapped to

    struct rrddimvar *next; ///< next dimension variable in the list
} RRDDIMVAR;

// calculated variables (defined in health configuration)
// These aggregate time-series data at fixed intervals
// (defined in their update_every member below)
// These increase the overhead of netdata.
//
// These calculations are allocated and linked (->next)
// under RRDHOST.
// Then are also linked to RRDSET (of course only when the
// chart is found, via ->rrdset_next and ->rrdset_prev).
// This double-linked list is maintained sorted at all times
// having as RRDSET.calculations the RRDCALC to be processed
// next.

#define RRDCALC_STATUS_REMOVED       -2
#define RRDCALC_STATUS_UNDEFINED     -1
#define RRDCALC_STATUS_UNINITIALIZED  0
#define RRDCALC_STATUS_CLEAR          1
#define RRDCALC_STATUS_RAISED         2
#define RRDCALC_STATUS_WARNING        3
#define RRDCALC_STATUS_CRITICAL       4

#define RRDCALC_FLAG_DB_ERROR              0x00000001
#define RRDCALC_FLAG_DB_NAN                0x00000002
/* #define RRDCALC_FLAG_DB_STALE           0x00000004 */
#define RRDCALC_FLAG_CALC_ERROR            0x00000008
#define RRDCALC_FLAG_WARN_ERROR            0x00000010
#define RRDCALC_FLAG_CRIT_ERROR            0x00000020
#define RRDCALC_FLAG_RUNNABLE              0x00000040
#define RRDCALC_FLAG_NO_CLEAR_NOTIFICATION 0x80000000

/** One alarm */
typedef struct rrdcalc {
    uint32_t id;                    ///< the unique id of this alarm
    uint32_t next_event_id;         ///< the next event id that will be used for this alar/<m

    char *name;                     ///< the name of this alarm
    uint32_t hash;                  ///< hash of `name`

    char *exec;                     ///< the command to execute when this alarm switches state
    char *recipient;                ///< the recipient of the alarm (the first parameter to exec)

    char *chart;                    ///< the chart id this should be linked to
    uint32_t hash_chart;            ///< hash of `chart`

    char *source;                   ///< the source of this alarm
    char *units;                    ///< the units of the alarm
    char *info;                     ///< a short description of the alarm

    int update_every;               ///< update frequency for the alarm

    /// the red threshold of this alarm (to be set to the chart)
    calculated_number green;
    /// the green threshold of this alarm (to be set to the chart)
    calculated_number red;

    // ------------------------------------------------------------------------
    // database lookup settings

    char *dimensions;               ///< the chart dimensions
    int group;                      ///< grouping method: average, max, etc.
    int before;                     ///< ending point in time-series
    int after;                      ///< starting point in time-series
    uint32_t options;               ///< calculation options

    // ------------------------------------------------------------------------
    // expressions related to the alarm

    EVAL_EXPRESSION *calculation;   ///< expression to calculate the value of the alarm
    EVAL_EXPRESSION *warning;       ///< expression to check the warning condition
    EVAL_EXPRESSION *critical;      ///< expression to check the critical condition

    // ------------------------------------------------------------------------
    // notification delay settings

    int delay_up_duration;         ///< duration to delay notifications when alarm raises
    int delay_down_duration;       ///< duration to delay notifications when alarm lowers
    int delay_max_duration;        ///< the absolute max delay to apply to this alarm
    float delay_multiplier;        ///< multiplier for all delays when alarms switch status
                                   ///< while now < delay_up_to

    // ------------------------------------------------------------------------
    // runtime information

    int status;                     ///< the current status of the alarm

    calculated_number value;        ///< the current value of the alarm
    calculated_number old_value;    ///< the previous value of the alarm

    uint32_t rrdcalc_flags;         ///< check RRDCALC_FLAG_*

    time_t last_updated;            ///< the last update timestamp of the alarm
    time_t next_update;             ///< the next update timestamp of the alarm
    time_t last_status_change;      ///< the timestamp of the last time this alarm changed status

    time_t db_after;                ///< the first timestamp evaluated by the db lookup
    time_t db_before;               ///< the last timestamp evaluated by the db lookup

    time_t delay_up_to_timestamp;   ///< the timestamp up to which we should delay notifications
    int delay_up_current;           ///< the current up notification delay duration
    int delay_down_current;         ///< the current down notification delay duration
    int delay_last;                 ///< the last delay we used

    // ------------------------------------------------------------------------
    // variables this alarm exposes to the rest of the alarms

    RRDVAR *local;    ///< exposed to the rest of the alarms
    RRDVAR *family;   ///< exposed to the rest of the alarms
    RRDVAR *hostid;   ///< exposed to the rest of the alarms
    RRDVAR *hostname; ///< exposed to the rest of the alarms

    // ------------------------------------------------------------------------
    // the chart this alarm it is linked to

    struct rrdset *rrdset; ///< set this is linked to

    /// linking of this alarm on its chart
    struct rrdcalc *rrdset_next;
    /// linking of this alarm on its chart
    struct rrdcalc *rrdset_prev;

    struct rrdcalc *next; ///< next in the list
} RRDCALC;

#define RRDCALC_HAS_DB_LOOKUP(rc) ((rc)->after)

/// RRDCALCTEMPLATE
/// these are to be applied to charts found dynamically
/// based on their context.
typedef struct rrdcalctemplate {
    char *name;         ///< name of the template
    uint32_t hash_name; ///< hash of `name`

    char *exec;      ///< Line of the alarm template. (a script)
    char *recipient; ///< The `to` line of the alarm template

    char *context;         ///< the `on` line of the alarm template
    uint32_t hash_context; ///< hash of `context`

    char *family_match;             ///< the `famailies` line of the alarm template (a simple-pattern)
    SIMPLE_PATTERN *family_pattern; ///< parsed pattern to generate `family_match`

    char *source;                   ///< the source of this alarm
    char *units;                    ///< the units of the alarm
    char *info;                     ///< a short description of the alarm

    int update_every;               ///< update frequency for the alarm

    /// the green threshold of this alarm (to be set to the chart)
    calculated_number green;
    /// the red threshold of this alarm (to be set to the chart)
    calculated_number red;

    // ------------------------------------------------------------------------
    // database lookup settings

    char *dimensions;               ///< the chart dimensions
    int group;                      ///< grouping method: average, max, etc.
    int before;                     ///< ending point in time-series
    int after;                      ///< starting point in time-series
    uint32_t options;               ///< calculation options

    // ------------------------------------------------------------------------
    // notification delay settings

    int delay_up_duration;         ///< duration to delay notifications when alarm raises
    int delay_down_duration;       ///< duration to delay notifications when alarm lowers
    int delay_max_duration;        ///< the absolute max delay to apply to this alarm
    float delay_multiplier;        ///< multiplier for all delays when alarms switch status

    // ------------------------------------------------------------------------
    // expressions related to the alarm

    EVAL_EXPRESSION *calculation; ///< calculation expression
    EVAL_EXPRESSION *warning;     ///< warning expression
    EVAL_EXPRESSION *critical;    ///< critical expression

    struct rrdcalctemplate *next; ///< next template in the list
} RRDCALCTEMPLATE;

#define RRDCALCTEMPLATE_HAS_CALCULATION(rt) ((rt)->after)

#define HEALTH_ENTRY_FLAG_PROCESSED             0x00000001
#define HEALTH_ENTRY_FLAG_UPDATED               0x00000002
#define HEALTH_ENTRY_FLAG_EXEC_RUN              0x00000004
#define HEALTH_ENTRY_FLAG_EXEC_FAILED           0x00000008
#define HEALTH_ENTRY_FLAG_SAVED                 0x10000000
#define HEALTH_ENTRY_FLAG_NO_CLEAR_NOTIFICATION 0x80000000

/** List of alarm_entry */
typedef struct alarm_entry {
    uint32_t unique_id;      ///< Uniqe id.
    uint32_t alarm_id;       ///< id of alarm
    uint32_t alarm_event_id; ///< id of event

    time_t when;               ///< timestamp when occoured
    time_t duration;           ///< duration time
    time_t non_clear_duration; ///< approximate duration time

    char *name;         ///< the name
    uint32_t hash_name; ///< hash of `name`

    char *chart;         ///< name of chart
    uint32_t hash_chart; ///< hash of chart

    char *family; ///< link to family

    char *exec;                ///< Script to run when the alarm switches state.
    char *recipient;           ///< First parameter passed to `exec`
    time_t exec_run_timestamp; ///< timestamp when `exec` run
    int exec_code;             ///< exit code of `exec`

    char *source; ///< source description of the alarm that generated this alarm. (i.e. `LINE@FILE`)
    char *units;  ///< how to interpret the values
    char *info;   ///< description of the alarm

    calculated_number old_value; ///< value before
    calculated_number new_value; ///< value after

    char *old_value_string; ///< string representation of `old_value`
    char *new_value_string; ///< string representation of `new_value`

    int old_status; ///< integer representation of `old_value`
    int new_status; ///< integer representation of `new_value`

    uint32_t flags; ///< HEALTH_ENTRY_FLAG_*

    int delay;                    ///< "delay to run the script" as was calculated when the alarm entry was created.
    time_t delay_up_to_timestamp; ///< "delay to run the script" expressed as an absolute timestamp.

    uint32_t updated_by_id; ///< id of the alarm entry that updated this alarm entry
    uint32_t updates_id;    ///< \brief id of the alarm entry that has been updated by this alarm entry.
                            ///< Tthe one that was cancelled because of this alarm entry.
    
    struct alarm_entry *next; ///< next entry in the list
} ALARM_ENTRY;

/** Log of alarms */
typedef struct alarm_log {
    uint32_t next_log_id;   ///< next ID of log
    uint32_t next_alarm_id; ///< next alarm
    unsigned int count;     ///< number appeard
    unsigned int max;       ///< maximal value
    ALARM_ENTRY *alarms;    ///< alarms which are logged
    pthread_rwlock_t alarm_log_rwlock; ///< lock to synchronize access to this
} ALARM_LOG;

#include "rrd.h"

extern void rrdsetvar_rename_all(RRDSET *st);
extern RRDSETVAR *rrdsetvar_create(RRDSET *st, const char *variable, int type, void *value, uint32_t options);
extern void rrdsetvar_free(RRDSETVAR *rs);

extern void rrddimvar_rename_all(RRDDIM *rd);
extern RRDDIMVAR *rrddimvar_create(RRDDIM *rd, int type, const char *prefix, const char *suffix, void *value, uint32_t options);
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
extern void rrdvar_custom_host_variable_destroy(RRDHOST *host, const char *name);
extern void rrdvar_custom_host_variable_set(RRDVAR *rv, calculated_number value);

extern const char *rrdcalc_status2string(int status);

#endif //NETDATA_HEALTH_H
