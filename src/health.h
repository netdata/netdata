#ifndef NETDATA_HEALTH_H
#define NETDATA_HEALTH_H

extern int health_enabled;

extern int rrdvar_compare(void *a, void *b);

#define RRDVAR_TYPE_CALCULATED 1
#define RRDVAR_TYPE_TIME_T     2
#define RRDVAR_TYPE_COLLECTED  3
#define RRDVAR_TYPE_TOTAL      4

// the variables as stored in the variables indexes
// there are 3 indexes:
// 1. at each chart   (RRDSET.variables_root_index)
// 2. at each context (RRDCONTEXT.variables_root_index)
// 3. at each host    (RRDHOST.variables_root_index)
typedef struct rrdvar {
    avl avl;

    char *name;
    uint32_t hash;

    int type;
    void *value;

    time_t last_updated;
} RRDVAR;

// variables linked to charts
// We link variables to point to the values that are already
// calculated / processed by the normal data collection process
// This means, there will be no speed penalty for using
// these variables
typedef struct rrdsetvar {
    char *fullid;               // chart type.chart id.variable
    uint32_t hash_fullid;

    char *fullname;             // chart type.chart name.variable
    uint32_t hash_fullname;

    char *variable;             // variable
    uint32_t hash_variable;

    int type;
    void *value;

    uint32_t options;

    RRDVAR *local;
    RRDVAR *context;
    RRDVAR *host;
    RRDVAR *context_name;
    RRDVAR *host_name;

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

    char *id;                   // dimension id
    uint32_t hash;

    char *name;                 // dimension name
    uint32_t hash_name;

    char *fullidid;             // chart type.chart id.dimension id
    uint32_t hash_fullidid;

    char *fullidname;           // chart type.chart id.dimension name
    uint32_t hash_fullidname;

    char *fullnameid;           // chart type.chart name.dimension id
    uint32_t hash_fullnameid;

    char *fullnamename;         // chart type.chart name.dimension name
    uint32_t hash_fullnamename;

    int type;
    void *value;

    uint32_t options;

    RRDVAR *local_id;
    RRDVAR *local_name;

    RRDVAR *context_id;
    RRDVAR *context_name;

    RRDVAR *context_fullidid;
    RRDVAR *context_fullidname;
    RRDVAR *context_fullnameid;
    RRDVAR *context_fullnamename;

    RRDVAR *host_fullidid;
    RRDVAR *host_fullidname;
    RRDVAR *host_fullnameid;
    RRDVAR *host_fullnamename;

    struct rrddim *rrddim;

    struct rrddimvar *next;
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
typedef struct rrdcalc {
    char *name;
    uint32_t hash;

    char *exec;

    char *chart;        // the chart id this should be linked to
    uint32_t hash_chart;

    char *source;       // the source of this calculation

    char *dimensions;   // the chart dimensions

    int group;          // grouping method: average, max, etc.
    int before;         // ending point in time-series
    int after;          // starting point in time-series
    uint32_t options;   // calculation options
    int update_every;   // update frequency for the calculation

    time_t last_updated;
    time_t next_update;

    EVAL_EXPRESSION *calculation;
    EVAL_EXPRESSION *warning;
    EVAL_EXPRESSION *critical;

    calculated_number value;

    calculated_number green;
    calculated_number red;

    RRDVAR *local;
    RRDVAR *context;
    RRDVAR *host;

    struct rrdset *rrdset;
    struct rrdcalc *rrdset_next;
    struct rrdcalc *rrdset_prev;

    struct rrdcalc *next;
} RRDCALC;

#define RRDCALC_HAS_CALCULATION(rc) ((rc)->after)

// RRDCALCTEMPLATE
// these are to be applied to charts found dynamically
// based on their context.
typedef struct rrdcalctemplate {
    char *name;
    uint32_t hash_name;

    char *exec;

    char *context;
    uint32_t hash_context;

    char *source;       // the source of this template

    char *dimensions;

    int group;          // grouping method: average, max, etc.
    int before;         // ending point in time-series
    int after;          // starting point in time-series
    uint32_t options;   // calculation options
    int update_every;   // update frequency for the calculation

    EVAL_EXPRESSION *calculation;
    EVAL_EXPRESSION *warning;
    EVAL_EXPRESSION *critical;

    calculated_number green;
    calculated_number red;

    struct rrdcalctemplate *next;
} RRDCALCTEMPLATE;

#define RRDCALCTEMPLATE_HAS_CALCULATION(rt) ((rt)->after)


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

extern void health_init(void);

#endif //NETDATA_HEALTH_H
