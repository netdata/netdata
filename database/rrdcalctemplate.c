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
void rrdcalctemplate_check_conditions_and_link(RRDCALCTEMPLATE *rt, RRDSET *st, RRDHOST *host) {
    if(rt->hash_context != st->hash_context || strcmp(rt->context, st->context) != 0)
        return;

    if (rt->charts_pattern && !simple_pattern_matches(rt->charts_pattern, st->name))
        return;

    if (rt->family_pattern && !simple_pattern_matches(rt->family_pattern, rrdset_family(st)))
        return;

    if (rt->module_pattern && !simple_pattern_matches(rt->module_pattern, rrdset_module_name(st)))
        return;

    if (rt->plugin_pattern && !simple_pattern_matches(rt->plugin_pattern, rrdset_plugin_name(st)))
        return;

    if(host->host_labels && rt->host_labels_pattern && !rrdlabels_match_simple_pattern_parsed(host->host_labels, rt->host_labels_pattern, '='))
        return;

    RRDCALC *rc = rrdcalc_create_from_template(host, rt, st->id);
    if (unlikely(!rc))
        info("Health tried to create alarm from template '%s' on chart '%s' of host '%s', but it failed", rt->name, st->id, host->hostname);
#ifdef NETDATA_INTERNAL_CHECKS
    else if (rc->rrdset != st && !rc->foreachdim) //When we have a template with foreadhdim, the child will be added to the index late
        error("Health alarm '%s.%s' should be linked to chart '%s', but it is not", rc->chart ? rc->chart : "NOCHART", rc->name, st->id);
#endif
}

void rrdcalctemplate_link_matching(RRDSET *st) {
    RRDHOST *host = st->rrdhost;
    RRDCALCTEMPLATE *rt;

    for(rt = host->templates; rt ; rt = rt->next)
        rrdcalctemplate_check_conditions_and_link(rt, st, host);

    for(rt = host->alarms_template_with_foreach; rt ; rt = rt->next)
        rrdcalctemplate_check_conditions_and_link(rt, st, host);
}

inline void rrdcalctemplate_free(RRDCALCTEMPLATE *rt) {
    if(unlikely(!rt)) return;

    expression_free(rt->calculation);
    expression_free(rt->warning);
    expression_free(rt->critical);

    freez(rt->family_match);
    simple_pattern_free(rt->family_pattern);

    freez(rt->plugin_match);
    simple_pattern_free(rt->plugin_pattern);

    freez(rt->module_match);
    simple_pattern_free(rt->module_pattern);

    freez(rt->charts_match);
    simple_pattern_free(rt->charts_pattern);

    freez(rt->name);
    freez(rt->exec);
    freez(rt->recipient);
    freez(rt->classification);
    freez(rt->component);
    freez(rt->type);
    freez(rt->context);
    freez(rt->source);
    freez(rt->units);
    freez(rt->info);
    freez(rt->dimensions);
    freez(rt->foreachdim);
    freez(rt->host_labels);
    simple_pattern_free(rt->spdim);
    simple_pattern_free(rt->host_labels_pattern);
    freez(rt);
}

inline void rrdcalctemplate_unlink_and_free(RRDHOST *host, RRDCALCTEMPLATE *rt) {
    if(unlikely(!rt)) return;

    debug(D_HEALTH, "Health removing template '%s' of host '%s'", rt->name, host->hostname);

    if(host->templates == rt) {
        host->templates = rt->next;
    }
    else {
        RRDCALCTEMPLATE *t;
        for (t = host->templates; t && t->next != rt; t = t->next ) ;
        if(t) {
            t->next = rt->next;
            rt->next = NULL;
        }
        else
            error("Cannot find RRDCALCTEMPLATE '%s' linked in host '%s'", rt->name, host->hostname);
    }

    rrdcalctemplate_free(rt);
}
