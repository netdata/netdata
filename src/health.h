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

typedef struct rrdvar {
    avl avl;

    char *name;
    uint32_t hash;

    calculated_number *value;

    time_t last_updated;
} RRDVAR;

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

#endif //NETDATA_HEALTH_H
