// SPDX-License-Identifier: GPL-3.0-or-later

#include "health.h"
#include "health_internals.h"

#define HEALTH_HISTORY_VALUE_RESOLUTION 10000
#define HEALTH_HISTORY_CHART_TYPE "netdata.health.alert"

static void rrdcalc_history_chart_create(RRDCALC *rc) {
    if(rc->history.rrdset)
        return;

    char id[RRD_ID_LENGTH_MAX + 1];
    snprintfz(id, sizeof(id), "%s.%s.%s",
              string2str(rc->config.name),
              string2str(rc->rrdset->id),
              string2str(rc->rrdset->rrdhost->hostname));

    rc->history.rrdset = rrdset_find_bytype_localhost(HEALTH_HISTORY_CHART_TYPE, id);
    if(!rc->history.rrdset) {
        rc->history.rrdset = rrdset_create_localhost(
            HEALTH_HISTORY_CHART_TYPE,
            id,
            NULL,
            "alert",
            "netdata.health.alert",
            "Alert History",
            "state",
            "health",
            "alert",
            9999999,
            rc->config.update_every,
            RRDSET_TYPE_LINE
            );

        rrdlabels_add(rc->history.rrdset->rrdlabels, "alert", string2str(rc->config.name), RRDLABEL_SRC_AUTO);
        rrdlabels_add(rc->history.rrdset->rrdlabels, "context", string2str(rc->rrdset->context), RRDLABEL_SRC_AUTO);
        rrdlabels_add(rc->history.rrdset->rrdlabels, "instance", string2str(rc->rrdset->id), RRDLABEL_SRC_AUTO);
        rrdlabels_add(rc->history.rrdset->rrdlabels, "host", string2str(rc->rrdset->rrdhost->hostname), RRDLABEL_SRC_AUTO);

        rrdset_flag_set(rc->history.rrdset, RRDSET_FLAG_EXPORTING_IGNORE|RRDSET_FLAG_UPSTREAM_IGNORE|RRDSET_FLAG_STORE_FIRST|RRDSET_FLAG_HIDDEN);
    }
    else
        rrdset_set_update_every_s(rc->history.rrdset, rc->config.update_every);

    rc->history.value = rrddim_find(rc->history.rrdset, "value");
    if(!rc->history.value)
        rc->history.value = rrddim_add(rc->history.rrdset, "value", NULL, 1, HEALTH_HISTORY_VALUE_RESOLUTION, RRD_ALGORITHM_ABSOLUTE);

    rc->history.undefined = rrddim_find(rc->history.rrdset, "undefined");
    if(!rc->history.undefined)
        rc->history.undefined = rrddim_add(rc->history.rrdset, "undefined", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);

    rc->history.clear = rrddim_find(rc->history.rrdset, "clear");
    if(!rc->history.clear)
        rc->history.clear = rrddim_add(rc->history.rrdset, "clear", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);

    rc->history.warning = rrddim_find(rc->history.rrdset, "warning");
    if(!rc->history.warning)
        rc->history.warning = rrddim_add(rc->history.rrdset, "warning", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);

    rc->history.critical = rrddim_find(rc->history.rrdset, "critical");
    if(!rc->history.critical)
        rc->history.critical = rrddim_add(rc->history.rrdset, "critical", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
}

void rrdcalc_history_chart_update(RRDCALC *rc) {
    if(!rc->history.rrdset)
        rrdcalc_history_chart_create(rc);

    collected_number undefined = 0, clear = 0, warning = 0, critical = 0;

    switch(rc->status) {
        case RRDCALC_STATUS_REMOVED:
            rrdcalc_history_chart_destroy(rc);
            return;

        default:
        case RRDCALC_STATUS_UNINITIALIZED:
            return;

        case RRDCALC_STATUS_UNDEFINED:
            undefined = 1;
            break;

        case RRDCALC_STATUS_CLEAR:
            clear = 1;
            break;

        case RRDCALC_STATUS_WARNING:
            warning = 1;
            break;

        case RRDCALC_STATUS_CRITICAL:
            critical = 1;
    }

    rrdset_next(rc->history.rrdset);

    rrddim_set_by_pointer(rc->history.rrdset, rc->history.value, (collected_number)(rc->value * HEALTH_HISTORY_VALUE_RESOLUTION));
    rrddim_set_by_pointer(rc->history.rrdset, rc->history.undefined, undefined);
    rrddim_set_by_pointer(rc->history.rrdset, rc->history.clear, clear);
    rrddim_set_by_pointer(rc->history.rrdset, rc->history.warning, warning);
    rrddim_set_by_pointer(rc->history.rrdset, rc->history.critical, critical);

    rrdset_done(rc->history.rrdset);
}

void rrdcalc_history_chart_destroy(RRDCALC *rc) {
    if(rc->history.rrdset)
        rrdset_is_obsolete___safe_from_collector_thread(rc->history.rrdset);

    rc->history.rrdset = NULL;
    rc->history.value = NULL;
    rc->history.undefined = NULL;
    rc->history.clear = NULL;
    rc->history.warning = NULL;
    rc->history.critical = NULL;
}
