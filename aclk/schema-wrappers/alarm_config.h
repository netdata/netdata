// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef ACLK_SCHEMA_WRAPPER_ALARM_CONFIG_H
#define ACLK_SCHEMA_WRAPPER_ALARM_CONFIG_H

#include <stdlib.h>
#include <stdint.h>

#ifdef __cplusplus
extern "C" {
#endif

struct aclk_alarm_configuration {
    char *alarm;
    char *tmpl;
    char *on_chart;
    
    char *classification;
    char *type;
    char *component;
        
    char *os;
    char *hosts;
    char *plugin;
    char *module;
    char *charts;
    char *lookup;
    char *every;
    char *units;

    char *green;
    char *red;

    char *calculation_expr;
    char *warning_expr;
    char *critical_expr;
    
    char *recipient;
    char *exec;
    char *delay;
    char *repeat;
    char *info;
    char *options;
    char *host_labels;

    int32_t p_db_lookup_after;
    int32_t p_db_lookup_before;
    char *p_db_lookup_dimensions;
    char *p_db_lookup_method;
    char *p_db_lookup_options;
    int32_t p_update_every;

    char *chart_labels;
    char *summary;
};

void destroy_aclk_alarm_configuration(struct aclk_alarm_configuration *cfg);

struct provide_alarm_configuration {
    char *cfg_hash;
    struct aclk_alarm_configuration cfg;
};

char *generate_provide_alarm_configuration(size_t *len, struct provide_alarm_configuration *data);
char *parse_send_alarm_configuration(const char *data, size_t len);

#ifdef __cplusplus
}
#endif

#endif /* ACLK_SCHEMA_WRAPPER_ALARM_CONFIG_H */
