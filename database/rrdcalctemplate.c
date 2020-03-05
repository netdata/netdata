// SPDX-License-Identifier: GPL-3.0-or-later

#define NETDATA_HEALTH_INTERNALS
#include "rrd.h"

// ----------------------------------------------------------------------------

static int rrdcalctemplate_is_there_label_restriction(RRDCALCTEMPLATE *rt,  RRDHOST *host) {
    if(!rt->labels)
        return 0;

    errno = 0;
    struct label *move = host->labels;
    char cmp[CONFIG_FILE_LINE_MAX+1];

    int ret;
    if(move) {
        rrdhost_check_rdlock(host);
        netdata_rwlock_rdlock(&host->labels_rwlock);
        while(move) {
            snprintfz(cmp, CONFIG_FILE_LINE_MAX, "%s=%s", move->key, move->value);
            if (simple_pattern_matches(rt->splabels, move->key) ||
                simple_pattern_matches(rt->splabels, cmp)) {
                break;
            }
            move = move->next;
        }
        netdata_rwlock_unlock(&host->labels_rwlock);

        if(!move) {
            error("Health template '%s' cannot be applied, because the host %s does not have the label(s) '%s'",
                   rt->name,
                   host->hostname,
                   rt->labels
            );
            ret = 1;
        } else {
            ret = 0;
        }
    } else {
        ret =0;
    }

    return ret;
}

// RRDCALCTEMPLATE management
/**
 * RRDCALC TEMPLATE LINK MATCHING
 *
 * @param rt is the template used to create the chart.
 * @param st is the chart where the alarm will be attached.
 */
void rrdcalctemplate_link_matching_test(RRDCALCTEMPLATE *rt, RRDSET *st, RRDHOST *host ) {
    if(rt->hash_context == st->hash_context && !strcmp(rt->context, st->context)
       && (!rt->family_pattern || simple_pattern_matches(rt->family_pattern, st->family))) {
        if (!rrdcalctemplate_is_there_label_restriction(rt, host)) {
            RRDCALC *rc = rrdcalc_create_from_template(host, rt, st->id);
            if (unlikely(!rc))
                info("Health tried to create alarm from template '%s' on chart '%s' of host '%s', but it failed",
                     rt->name, st->id, host->hostname);
#ifdef NETDATA_INTERNAL_CHECKS
            else if (rc->rrdset != st &&
                     !rc->foreachdim) //When we have a template with foreadhdim, the child will be added to the index late
                error("Health alarm '%s.%s' should be linked to chart '%s', but it is not",
                      rc->chart ? rc->chart : "NOCHART", rc->name, st->id);
#endif
        }
    }
}

void rrdcalctemplate_link_matching(RRDSET *st) {
    RRDHOST *host = st->rrdhost;
    RRDCALCTEMPLATE *rt;

    for(rt = host->templates; rt ; rt = rt->next) {
        rrdcalctemplate_link_matching_test(rt, st, host);
    }

    for(rt = host->alarms_template_with_foreach; rt ; rt = rt->next) {
        rrdcalctemplate_link_matching_test(rt, st, host);
    }
}

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
    freez(rt->labels);
    simple_pattern_free(rt->spdim);
    simple_pattern_free(rt->splabels);
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
