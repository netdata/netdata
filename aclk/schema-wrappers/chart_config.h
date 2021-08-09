// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef ACLK_SCHEMA_WRAPPER_CHART_CONFIG_H
#define ACLK_SCHEMA_WRAPPER_CHART_CONFIG_H

#include <stdlib.h>

#include "database/rrd.h"

#ifdef __cplusplus
extern "C" {
#endif

struct update_chart_config {
    char *claim_id;
    char *node_id;
    char **hashes;
};

enum chart_config_chart_type {
    LINE,
    AREA,
    STACKED
};

struct chart_config_updated {
    char *type;
    char *family;
    char *context;
    char *title;
    uint64_t priority;
    char *plugin;
    char *module;
    RRDSET_TYPE chart_type;
    char *units;
    char *config_hash;
};

void destroy_update_chart_config(struct update_chart_config *cfg);
void destroy_chart_config_updated(struct chart_config_updated *cfg);

struct update_chart_config parse_update_chart_config(const char *data, size_t len);

char *generate_chart_configs_updated(size_t *len, const struct chart_config_updated *config_list, int list_size);

#ifdef __cplusplus
}
#endif

#endif /* ACLK_SCHEMA_WRAPPER_CHART_CONFIG_H */
