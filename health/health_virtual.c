// SPDX-License-Identifier: GPL-3.0-or-later

#include "health.h"

void health_virtual_run(RRDHOST *host, BUFFER *wb, char *chart, char *context, char *lookup, char *calc, char *warn, char *crit) {
    RRDCALC *rcv = callocz(1, sizeof(RRDCALC));

    health_config_setup_rc_from_api(host, rcv, chart, context, lookup, calc, warn, crit);

    if (unlikely(RRDCALC_HAS_DB_LOOKUP(rcv))) {

        /* time_t old_db_timestamp = rc->db_before; */
        int value_is_null = 0;

        int ret = rrdset2value_api_v1(rcv->rrdset, NULL, &rcv->value, rrdcalc_dimensions(rcv), 1,
                                      rcv->after, rcv->before, rcv->group, NULL,
                                      0, rcv->options,
                                      &rcv->db_after,&rcv->db_before,
                                      NULL, NULL, NULL,
                                      &value_is_null, NULL, 0, 0,
                                      QUERY_SOURCE_HEALTH, STORAGE_PRIORITY_LOW);

        if (unlikely(ret != 200)) {
            // database lookup failed
            rcv->value = NAN;
            //rcv->run_flags |= RRDCALC_FLAG_DB_ERROR;

            log_health("Health on host '%s', alarm '%s.%s': database lookup returned error %d",
                  rrdhost_hostname(host), rrdcalc_chart_name(rcv), rrdcalc_name(rcv), ret
                  );
        } else
            rcv->run_flags &= ~RRDCALC_FLAG_DB_ERROR;

        if (unlikely(value_is_null)) {
            // collected value is null
            rcv->value = NAN;
            rcv->run_flags |= RRDCALC_FLAG_DB_NAN;

            log_health(
                  "Health on host '%s', alarm '%s.%s': database lookup returned empty value (possibly value is not collected yet)",
                  rrdhost_hostname(host), rrdcalc_chart_name(rcv), rrdcalc_name(rcv)
                  );
        } else
            rcv->run_flags &= ~RRDCALC_FLAG_DB_NAN;

        log_health("Health on host '%s', alarm '%s.%s': database lookup gave value " NETDATA_DOUBLE_FORMAT,
              rrdhost_hostname(host), rrdcalc_chart_name(rcv), rrdcalc_name(rcv), rcv->value
              );

        buffer_sprintf(wb, "%s\n\t\t\"%s\": %0.5" NETDATA_DOUBLE_MODIFIER, /*helper->counter?",":*/"", "db_lookup", (NETDATA_DOUBLE)rcv->value);
    }

    if (unlikely(rcv->calculation)) {

        if (unlikely(!expression_evaluate(rcv->calculation))) {
            // calculation failed
            rcv->value = NAN;
            rcv->run_flags |= RRDCALC_FLAG_CALC_ERROR;

            log_health("Health on host '%s', alarm '%s.%s': expression '%s' failed: %s",
                  rrdhost_hostname(host), rrdcalc_chart_name(rcv), rrdcalc_name(rcv),
                  rcv->calculation->parsed_as, buffer_tostring(rcv->calculation->error_msg)
                  );
        } else {
            rcv->run_flags &= ~RRDCALC_FLAG_CALC_ERROR;

            log_health("Health on host '%s', alarm '%s.%s': expression '%s' gave value "
                  NETDATA_DOUBLE_FORMAT
                  ": %s (source: %s)", rrdhost_hostname(host), rrdcalc_chart_name(rcv), rrdcalc_name(rcv),
                  rcv->calculation->parsed_as, rcv->calculation->result,
                  buffer_tostring(rcv->calculation->error_msg), rrdcalc_source(rcv)
                  );

            rcv->value = rcv->calculation->result;
            buffer_sprintf(wb, "%s\n\t\t\"%s\": %0.5" NETDATA_DOUBLE_MODIFIER, /*helper->counter?",":*/",", "calc", (NETDATA_DOUBLE)rcv->value);
        }
    }

    if (likely(rcv->warning)) {

        if (unlikely(!expression_evaluate(rcv->warning))) {
            // calculation failed
            rcv->run_flags |= RRDCALC_FLAG_WARN_ERROR;

        } else {
            rcv->run_flags &= ~RRDCALC_FLAG_WARN_ERROR;
            buffer_sprintf(wb, "%s\n\t\t\"%s\": %0.5" NETDATA_DOUBLE_MODIFIER, /*helper->counter?",":*/",", "warning", (NETDATA_DOUBLE)rcv->warning->result);
        }
    }

}
