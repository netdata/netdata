// SPDX-License-Identifier: GPL-3.0-or-later

#define NETDATA_HEALTH_INTERNALS
#include "rrd.h"

// ----------------------------------------------------------------------------
// RRDCALCTEMPLATE management
/**
 * RRDCALC TEMPLATE LINK MATCHING
 *
 * @param rt is the template used to create the chart.
 * @param st is the chart where the alarm will be attached.
 */

static char *rrdcalc_alert_name_with_dimension(const char *name, size_t namelen, const char *dim, size_t dimlen) {
    char *newname,*move;

    newname = mallocz(namelen + dimlen + 2);
    move = newname;
    memcpy(move, name, namelen);
    move += namelen;

    *move++ = '_';
    memcpy(move, dim, dimlen);
    move += dimlen;
    *move = '\0';

    return newname;
}

bool rrdcalctemplate_check_rrdset_conditions(RRDCALCTEMPLATE *rt, RRDSET *st, RRDHOST *host) {
    if(rt->context != st->context)
        return false;

    if(rt->foreach_dimension_pattern && !rrdset_number_of_dimensions(st))
        return false;

    if (rt->charts_pattern && !simple_pattern_matches(rt->charts_pattern, rrdset_name(st)) && !simple_pattern_matches(rt->charts_pattern, rrdset_id(st)))
        return false;

    if (rt->family_pattern && !simple_pattern_matches(rt->family_pattern, rrdset_family(st)))
        return false;

    if (rt->module_pattern && !simple_pattern_matches(rt->module_pattern, rrdset_module_name(st)))
        return false;

    if (rt->plugin_pattern && !simple_pattern_matches(rt->plugin_pattern, rrdset_plugin_name(st)))
        return false;

    if(host->rrdlabels && rt->host_labels_pattern && !rrdlabels_match_simple_pattern_parsed(host->rrdlabels, rt->host_labels_pattern, '='))
        return false;

    return true;
}

void rrdcalctemplate_check_rrddim_conditions_and_link(RRDCALCTEMPLATE *rt, RRDSET *st, RRDDIM *rd, RRDHOST *host) {
    if (simple_pattern_matches(rt->foreach_dimension_pattern, rrddim_id(rd)) || simple_pattern_matches(rt->foreach_dimension_pattern, rrddim_name(rd))) {
        char *overwrite_alert_name = rrdcalc_alert_name_with_dimension(
            rrdcalctemplate_name(rt), string_strlen(rt->name), rrddim_name(rd), string_strlen(rd->name));
        rrdcalc_add_from_rrdcalctemplate(host, rt, st, overwrite_alert_name, rrddim_name(rd));
        freez(overwrite_alert_name);
    }
}

void rrdcalctemplate_check_conditions_and_link(RRDCALCTEMPLATE *rt, RRDSET *st, RRDHOST *host) {
    if(!rrdcalctemplate_check_rrdset_conditions(rt, st, host))
        return;

    if(!rt->foreach_dimension_pattern) {
        rrdcalc_add_from_rrdcalctemplate(host, rt, st, NULL, NULL);
        return;
    }

    RRDDIM *rd;
    rrddim_foreach_read(rd, st) {
        rrdcalctemplate_check_rrddim_conditions_and_link(rt, st, rd, host);
    }
    rrddim_foreach_done(rd);
}

void rrdcalctemplate_link_matching(RRDSET *st) {
    RRDHOST *host = st->rrdhost;
    RRDCALCTEMPLATE *rt;

    foreach_rrdcalctemplate_in_rrdhost(host, rt)
        rrdcalctemplate_check_conditions_and_link(rt, st, host);
}

inline void rrdcalctemplate_free(RRDCALCTEMPLATE *rt) {
    if(unlikely(!rt)) return;

    expression_free(rt->calculation);
    expression_free(rt->warning);
    expression_free(rt->critical);

    string_freez(rt->family_match);
    simple_pattern_free(rt->family_pattern);

    string_freez(rt->plugin_match);
    simple_pattern_free(rt->plugin_pattern);

    string_freez(rt->module_match);
    simple_pattern_free(rt->module_pattern);

    string_freez(rt->charts_match);
    simple_pattern_free(rt->charts_pattern);

    string_freez(rt->name);
    string_freez(rt->exec);
    string_freez(rt->recipient);
    string_freez(rt->classification);
    string_freez(rt->component);
    string_freez(rt->type);
    string_freez(rt->context);
    string_freez(rt->source);
    string_freez(rt->units);
    string_freez(rt->info);
    string_freez(rt->dimensions);
    string_freez(rt->foreach_dimension);
    string_freez(rt->host_labels);
    simple_pattern_free(rt->foreach_dimension_pattern);
    simple_pattern_free(rt->host_labels_pattern);
    freez(rt);
}

inline void rrdcalctemplate_unlink_and_free(RRDHOST *host, RRDCALCTEMPLATE *rt) {
    if(unlikely(!rt)) return;

    debug(D_HEALTH, "Health removing template '%s' of host '%s'", rrdcalctemplate_name(rt), rrdhost_hostname(host));

    DOUBLE_LINKED_LIST_REMOVE_UNSAFE(host->alarms_templates, rt, prev, next);

    rrdcalctemplate_free(rt);
}
