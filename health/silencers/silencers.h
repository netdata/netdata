// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_SILENCERS_H
#define NETDATA_SILENCERS_H

#include "libnetdata/libnetdata.h"

#ifdef __cplusplus
extern "C" {
#endif
    
typedef struct silencer {
    char *alarms;
    SIMPLE_PATTERN *alarms_pattern;

    char *hosts;
    SIMPLE_PATTERN *hosts_pattern;

    char *contexts;
    SIMPLE_PATTERN *contexts_pattern;

    char *charts;
    SIMPLE_PATTERN *charts_pattern;

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

extern SILENCERS *silencers;
extern char *silencers_filename;

void health_silencers_add(SILENCER *silencer);
SILENCER * health_silencer_add_param(SILENCER *silencer, char *key, char *value);
int health_initialize_global_silencers();
void health_silencers_init(void);
void health_silencers2file(BUFFER *wb);
void health_silencers2json(BUFFER *wb);

bool load_health_silencers(const char *path);

#ifdef __cplusplus
}
#endif

#endif /* NETDATA_SILENCERS_H */
