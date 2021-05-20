// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef ACLK_SCHEMA_WRAPPER_CHART_CONFIG_H
#define ACLK_SCHEMA_WRAPPER_CHART_CONFIG_H

#include <stdlib.h>

#ifdef __cplusplus
extern "C" {
#endif

struct update_chart_config {
    char *claim_id;
    char *node_id;
    char **hashes;
};

void destroy_update_chart_config(struct update_chart_config *cfg);

struct update_chart_config parse_update_chart_config(const char *data, size_t len);

#ifdef __cplusplus
}
#endif

#endif /* ACLK_SCHEMA_WRAPPER_CHART_CONFIG_H */
