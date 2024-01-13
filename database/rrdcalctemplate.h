// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_RRDCALCTEMPLATE_H
#define NETDATA_RRDCALCTEMPLATE_H 1

#include "rrd.h"

// RRDCALCTEMPLATE
// these are to be applied to charts found dynamically
// based on their context.
struct rrdcalctemplate {
    uuid_t config_hash_id;

    STRING *context;

    struct rrd_alert_match match;
    struct rrd_alert_config config;

    struct rrdcalctemplate *next;
    struct rrdcalctemplate *prev;
};

#define foreach_rrdcalctemplate_read(host, rt) \
    dfe_start_read((host)->rrdcalctemplate_root_index, rt)

#define foreach_rrdcalctemplate_done(rt) \
    dfe_done(rt)

#define rrdcalctemplate_name(rt) string2str((rt)->config.name)
#define rrdcalctemplate_exec(rt) string2str((rt)->config.exec)
#define rrdcalctemplate_recipient(rt) string2str((rt)->config.recipient)
#define rrdcalctemplate_classification(rt) string2str((rt)->config.classification)
#define rrdcalctemplate_component(rt) string2str((rt)->config.component)
#define rrdcalctemplate_type(rt) string2str((rt)->config.type)
#define rrdcalctemplate_plugin_match(rt) string2str((rt)->match.plugin)
#define rrdcalctemplate_module_match(rt) string2str((rt)->match.module)
#define rrdcalctemplate_charts_match(rt) string2str((rt)->match.charts)
#define rrdcalctemplate_units(rt) string2str((rt)->config.units)
#define rrdcalctemplate_summary(rt) string2str((rt)->config.summary)
#define rrdcalctemplate_info(rt) string2str((rt)->config.info)
#define rrdcalctemplate_source(rt) string2str((rt)->config.source)
#define rrdcalctemplate_dimensions(rt) string2str((rt)->config.dimensions)
#define rrdcalctemplate_foreachdim(rt) string2str((rt)->config.foreach_dimension)
#define rrdcalctemplate_host_labels(rt) string2str((rt)->match.host_labels)
#define rrdcalctemplate_chart_labels(rt) string2str((rt)->match.chart_labels)

#define RRDCALCTEMPLATE_HAS_DB_LOOKUP(rt) ((rt)->config.after)

void rrdcalctemplate_link_matching_templates_to_rrdset(RRDSET *st);

void rrdcalctemplate_free_unused_rrdcalctemplate_loaded_from_config(RRDCALCTEMPLATE *rt);
void rrdcalctemplate_delete_all(RRDHOST *host);
void rrdcalctemplate_add_from_config(RRDHOST *host, RRDCALCTEMPLATE *rt);

void rrdcalctemplate_check_conditions_and_link(RRDCALCTEMPLATE *rt, RRDSET *st, RRDHOST *host);

bool rrdcalctemplate_check_rrdset_conditions(RRDCALCTEMPLATE *rt, RRDSET *st, RRDHOST *host);
void rrdcalctemplate_check_rrddim_conditions_and_link(RRDCALCTEMPLATE *rt, RRDSET *st, RRDDIM *rd, RRDHOST *host);


void rrdcalctemplate_index_init(RRDHOST *host);
void rrdcalctemplate_index_destroy(RRDHOST *host);

#endif //NETDATA_RRDCALCTEMPLATE_H
