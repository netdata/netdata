// SPDX-License-Identifier: GPL-3.0-or-later

#define NETDATA_HEALTH_INTERNALS
#include "rrd.h"

// ----------------------------------------------------------------------------
// RRDCALCTEMPLATE management

/**
 * RRDCALC TEMPLATE LINK MATCHING
 *
 * Link a template for a specific chart.
 *
 * @param st is the chart where the alarm will be attached.
 */
void rrdcalctemplate_link_matching(RRDSET *st) {
    RRDHOST *host = st->rrdhost;
    RRDCALCTEMPLATE *rt;

    for(rt = host->templates; rt ; rt = rt->next) {
        if(rt->hash_context == st->hash_context && !strcmp(rt->context, st->context)
           && (!rt->family_pattern || simple_pattern_matches(rt->family_pattern, st->family))) {
            fprintf(stderr,"KILLME creating chart from template: %s %s\n",st->id,(rt->foreachdim)?rt->foreachdim:"nothing");
            //IT IS NECESSRY TO BRING THE SAME STRUCTURE USED IN THE FILE health_config.c (line 583)
            RRDCALC *rc = rrdcalc_create_from_template(host, rt, st->id);
            if(unlikely(!rc))
                info("Health tried to create alarm from template '%s' on chart '%s' of host '%s', but it failed", rt->name, st->id, host->hostname);

#ifdef NETDATA_INTERNAL_CHECKS
            else if(rc->rrdset != st)
                error("Health alarm '%s.%s' should be linked to chart '%s', but it is not", rc->chart?rc->chart:"NOCHART", rc->name, st->id);
#endif
        }
    }
}

/**
 * Template free
 *
 * After the template to be unlinked from the caller, this function
 * cleans the heap.
 *
 * @param rt the template to be cleaned.
 */
inline void rrdcalctemplate_free(RRDCALCTEMPLATE *rt) {
    if(unlikely(!rt)) return;

    expression_free(rt->calculation);
    expression_free(rt->warning);
    expression_free(rt->critical);

    freez(rt->family_match);
    simple_pattern_free(rt->family_pattern);

    freez(rt->name);
    freez(rt->exec);
    freez(rt->recipient);
    freez(rt->context);
    freez(rt->source);
    freez(rt->units);
    freez(rt->info);
    freez(rt->dimensions);
    freez(rt->foreachdim);
    freez(rt);
}

/**
 * RRDCALC Templace Unlink and Free
 *
 * Unlink the template from Host and call rrdcalctemplate_free
 *
 * @param host the structure with the template links
 * @param rt the template that will be unlink and cleaned.
 */
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
