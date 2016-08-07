#ifndef NETDATA_HEALTH_H
#define NETDATA_HEALTH_H

extern int rrdvar_compare(void *a, void *b);

/*
 * RRDVAR
 * a variable
 *
 * There are 4 scopes: local (chart), context, host and global variables
 *
 * Standard global variables:
 *  $now
 *
 * Standard host variables:
 *  - none -
 *
 * Standard context variables:
 *  - none -
 *
 * Standard local variables:
 *  $last_updated
 *  $last_collected_value
 *  $last_value
 *
 */

#define RRDVAR_TYPE_CALCULATED 1
#define RRDVAR_TYPE_TIME_T     2
#define RRDVAR_TYPE_COLLECTED  3
#define RRDVAR_TYPE_TOTAL      4

// the variables as stored in the variables indexes
typedef struct rrdvar {
    avl avl;

    char *name;
    uint32_t hash;

    int type;
    void *value;

    time_t last_updated;
} RRDVAR;

// variables linked to charts
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


// variables linked to dimensions
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

typedef struct rrdcalc {
    avl avl;

    int group;          // grouping method: average, max, etc.
    int before;         // ending point in time-series
    int after;          // starting point in time-series
    int update_every;   // update frequency for the calculation

    const char *name;
    calculated_number value;

    RRDVAR *local;
    RRDVAR *context;
    RRDVAR *host;

    struct rrdcalc *next;
    struct rrdcalc *prev;
} RRDCALC;

#include "rrd.h"

extern void rrdsetvar_rename_all(RRDSET *st);
extern RRDSETVAR *rrdsetvar_create(RRDSET *st, const char *variable, int type, void *value, uint32_t options);
extern void rrdsetvar_free(RRDSETVAR *rs);

extern void rrddimvar_rename_all(RRDDIM *rd);
extern RRDDIMVAR *rrddimvar_create(RRDDIM *rd, int type, const char *prefix, const char *suffix, void *value, uint32_t options);
extern void rrddimvar_free(RRDDIMVAR *rs);

#endif //NETDATA_HEALTH_H
