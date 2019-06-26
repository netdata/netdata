#ifndef NETDATA_HEALTH_LIB
# define NETDATA_HEALTH_LIB 1

# include "../libnetdata.h"

typedef struct silencer {
    char *alarms;
    SIMPLE_PATTERN *alarms_pattern;

    char *hosts;
    SIMPLE_PATTERN *hosts_pattern;

    char *contexts;
    SIMPLE_PATTERN *contexts_pattern;

    char *charts;
    SIMPLE_PATTERN *charts_pattern;

    char *families;
    SIMPLE_PATTERN *families_pattern;

    struct silencer *next;
} SILENCER;

#endif
