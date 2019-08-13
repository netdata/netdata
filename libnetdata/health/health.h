#ifndef NETDATA_HEALTH_LIB
# define NETDATA_HEALTH_LIB 1

# include "../libnetdata.h"

#define HEALTH_ALARM_KEY "alarm"
#define HEALTH_TEMPLATE_KEY "template"
#define HEALTH_CONTEXT_KEY "context"
#define HEALTH_CHART_KEY "chart"
#define HEALTH_HOST_KEY "hosts"
#define HEALTH_OS_KEY "os"
#define HEALTH_FAMILIES_KEY "families"
#define HEALTH_LOOKUP_KEY "lookup"
#define HEALTH_CALC_KEY "calc"

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

typedef enum silence_type {
    STYPE_NONE,
    STYPE_DISABLE_ALARMS,
    STYPE_SILENCE_NOTIFICATIONS
} SILENCE_TYPE;

typedef struct silencers {
    int all_alarms;
    SILENCE_TYPE stype;
    SILENCER *silencers;
} SILENCERS;

SILENCERS *silencers;

extern SILENCER *create_silencer(void);
extern int health_silencers_json_read_callback(JSON_ENTRY *e);
extern void health_silencers_add(SILENCER *silencer);
extern SILENCER * health_silencers_addparam(SILENCER *silencer, char *key, char *value);
extern int health_initialize_global_silencers();

#endif
