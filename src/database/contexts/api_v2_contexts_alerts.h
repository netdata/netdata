// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_API_V2_CONTEXTS_ALERTS_H
#define NETDATA_API_V2_CONTEXTS_ALERTS_H

#include "internal.h"
#include "api_v2_contexts.h"

struct alert_transitions_callback_data {
    struct rrdcontext_to_json_v2_data *ctl;
    BUFFER *wb;
    bool debug;
    bool only_one_config;

    struct {
        SIMPLE_PATTERN *pattern;
        DICTIONARY *dict;
    } facets[ATF_TOTAL_ENTRIES];

    uint32_t max_items_to_return;
    uint32_t items_to_return;

    uint32_t items_evaluated;
    uint32_t items_matched;


    struct sql_alert_transition_fixed_size *base; // double linked list - last item is base->prev
    struct sql_alert_transition_fixed_size *last_added; // the last item added, not the last of the list

    struct {
        size_t first;
        size_t skips_before;
        size_t skips_after;
        size_t backwards;
        size_t forwards;
        size_t prepend;
        size_t append;
        size_t shifts;
    } operations;

    uint32_t configs_added;
};

void contexts_v2_alerts_to_json(BUFFER *wb, struct rrdcontext_to_json_v2_data *ctl, bool debug);
bool rrdcontext_matches_alert(struct rrdcontext_to_json_v2_data *ctl, RRDCONTEXT *rc);
void contexts_v2_alert_config_to_json_from_sql_alert_config_data(struct sql_alert_config_data *t, void *data);
void contexts_v2_alert_transitions_to_json(BUFFER *wb, struct rrdcontext_to_json_v2_data *ctl, bool debug);

bool rrdcontexts_v2_init_alert_dictionaries(struct rrdcontext_to_json_v2_data *ctl, struct api_v2_contexts_request *req);
void rrdcontexts_v2_alerts_cleanup(struct rrdcontext_to_json_v2_data *ctl);

#endif //NETDATA_API_V2_CONTEXTS_ALERTS_H
