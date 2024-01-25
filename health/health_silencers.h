// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_HEALTH_SILENCERS_H
#define NETDATA_HEALTH_SILENCERS_H

#include "health.h"

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

SILENCER *create_silencer(void);
int health_silencers_json_read_callback(JSON_ENTRY *e);
void health_silencers_add(SILENCER *silencer);
SILENCER * health_silencers_addparam(SILENCER *silencer, char *key, char *value);
int health_initialize_global_silencers();

void free_silencers(SILENCER *t);

struct web_client;
int web_client_api_request_v1_mgmt_health(RRDHOST *host, struct web_client *w, char *url);

const char *health_silencers_filename(void);
void health_set_silencers_filename(void);
void health_silencers_init(void);
SILENCE_TYPE health_silencers_check_silenced(RRDCALC *rc, const char *host);
int health_silencers_update_disabled_silenced(RRDHOST *host, RRDCALC *rc);

#endif //NETDATA_HEALTH_SILENCERS_H
