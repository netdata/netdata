// SPDX-License-Identifier: GPL-3.0-or-later

#include "health.h"
#include "libnetdata/http/http_defs.h"

static void rcv_free(RRDCALC *rcv) {
    if(unlikely(!rcv)) return;

    expression_free(rcv->calculation);
    expression_free(rcv->warning);
    expression_free(rcv->critical);

    string_freez(rcv->key);
    string_freez(rcv->name);
    string_freez(rcv->chart);
    string_freez(rcv->dimensions);
    string_freez(rcv->foreach_dimension);
    string_freez(rcv->units);

    simple_pattern_free(rcv->foreach_dimension_pattern);
}

void health_virtual_run(RRDHOST *host, BUFFER *wb, RRDCALC *rcv, time_t at) {
    bool vraised_warn = false;
    bool vraised_crit = false;

    buffer_json_add_array_item_object(wb);
    buffer_json_member_add_time_t(wb, "when", at + 1); //small adjustment to match health's results

    if (unlikely(RRDCALC_HAS_DB_LOOKUP(rcv))) {
        int value_is_null = 0;
        time_t before = at;
        time_t after = before + rcv->after + 1;

        int ret = rrdset2value_api_v1(rcv->rrdset, NULL, &rcv->value, rrdcalc_dimensions(rcv), 1,
                                      after, before, rcv->group, NULL,
                                      0, rcv->options | RRDR_OPTION_SELECTED_TIER,
                                      &rcv->db_after,&rcv->db_before,
                                      NULL, NULL, NULL,
                                      &value_is_null, NULL, 0, 0,
                                      QUERY_SOURCE_HEALTH, STORAGE_PRIORITY_LOW);

        if (unlikely(ret != HTTP_RESP_OK)) {
            // database lookup failed
            rcv->value = NAN;
            //rcv->run_flags |= RRDCALC_FLAG_DB_ERROR;

            netdata_log_debug(D_HEALTH, "Health (virtual) on host '%s', alarm '%s.%s': database lookup returned error %d",
                  rrdhost_hostname(host), rrdcalc_chart_name(rcv), rrdcalc_name(rcv), ret
                  );
        } else
            rcv->run_flags &= ~RRDCALC_FLAG_DB_ERROR;

        if (unlikely(value_is_null)) {
            // collected value is null
            rcv->value = NAN;
            rcv->run_flags |= RRDCALC_FLAG_DB_NAN;

            netdata_log_debug(D_HEALTH,
                  "Health (virtual) on host '%s', alarm '%s.%s': database lookup returned empty value (possibly value is not collected yet)",
                  rrdhost_hostname(host), rrdcalc_chart_name(rcv), rrdcalc_name(rcv)
                  );
        } else
            rcv->run_flags &= ~RRDCALC_FLAG_DB_NAN;

        netdata_log_debug(D_HEALTH, "Health (virtual) on host '%s', alarm '%s.%s': database lookup gave value " NETDATA_DOUBLE_FORMAT,
              rrdhost_hostname(host), rrdcalc_chart_name(rcv), rrdcalc_name(rcv), rcv->value
              );

        buffer_json_member_add_double(wb, "db_lookup", rcv->value);
    }

    if (unlikely(rcv->calculation)) {
        rcv->calculation->value_at = at;

        if (unlikely(!expression_evaluate(rcv->calculation))) {
            // calculation failed
            rcv->value = NAN;
            rcv->run_flags |= RRDCALC_FLAG_CALC_ERROR;

            netdata_log_debug(D_HEALTH, "Health (virtual) on host '%s', alarm '%s.%s': expression '%s' failed: %s",
                  rrdhost_hostname(host), rrdcalc_chart_name(rcv), rrdcalc_name(rcv),
                  rcv->calculation->parsed_as, buffer_tostring(rcv->calculation->error_msg)
                  );
            buffer_json_member_add_string(wb, "calc_error", buffer_tostring(rcv->calculation->error_msg));
        } else {
            rcv->run_flags &= ~RRDCALC_FLAG_CALC_ERROR;

            netdata_log_debug(D_HEALTH, "Health (virtual) on host '%s', alarm '%s.%s': expression '%s' gave value "
                  NETDATA_DOUBLE_FORMAT
                  ": %s (source: %s)", rrdhost_hostname(host), rrdcalc_chart_name(rcv), rrdcalc_name(rcv),
                  rcv->calculation->parsed_as, rcv->calculation->result,
                  buffer_tostring(rcv->calculation->error_msg), rrdcalc_source(rcv)
                  );

            rcv->value = rcv->calculation->result;
            buffer_json_member_add_double(wb, "calc", rcv->value);
        }
    }

    if (likely(rcv->warning)) {
        rcv->warning->value_at = at;

        if (unlikely(!expression_evaluate(rcv->warning))) {
            // calculation failed
            rcv->run_flags |= RRDCALC_FLAG_WARN_ERROR;
        } else {
            rcv->run_flags &= ~RRDCALC_FLAG_WARN_ERROR;
            buffer_json_member_add_double(wb, "warn", rcv->warning->result);
            if (rcv->warning->result) {
                rcv->status = RRDCALC_STATUS_WARNING;
                vraised_warn = true;
            }
        }
    }

    if (likely(rcv->critical)) {
        rcv->critical->value_at = at;

        if (unlikely(!expression_evaluate(rcv->critical))) {
            // calculation failed
            rcv->run_flags |= RRDCALC_FLAG_CRIT_ERROR;
        } else {
            rcv->run_flags &= ~RRDCALC_FLAG_CRIT_ERROR;
            buffer_json_member_add_double(wb, "crit", rcv->critical->result);
            if (rcv->critical->result) {
                rcv->status = RRDCALC_STATUS_CRITICAL;
                vraised_crit = true;
            }
        }
    }

    if (!vraised_warn && !vraised_crit)
        rcv->status = RRDCALC_STATUS_CLEAR;

    buffer_json_object_close(wb);
}

void health_virtual(RRDHOST *host, BUFFER *wb, struct health_virtual *hv, int min_run_every) {
    DICTIONARY *dict_rcvs = NULL;
    dict_rcvs = dictionary_create(DICT_OPTION_SINGLE_THREADED | DICT_OPTION_VALUE_LINK_DONT_CLONE);

    {
        buffer_json_member_add_object(wb, "configuration");
        health_config_setup_rc_from_api(wb, host, dict_rcvs, hv);
        buffer_json_object_close(wb);
    }

    RRDCALC *rcv;
    dfe_start_read(dict_rcvs, rcv) {
        buffer_json_member_add_array(wb, string2str(rcv->chart));

        //small adjustment to match health's results
        time_t now      = now_realtime_sec();
        time_t at       = hv->after  ? hv->after - 1  : now;
        time_t before   = hv->before ? hv->before - 1 : now;

        while (at <= before) {
            health_virtual_run(host, wb, rcv, at);
            at += min_run_every;
        }

        buffer_json_array_close(wb);
        rcv_free(rcv);
        freez(rcv);
    }
    dfe_done(rcv);
    dictionary_destroy(dict_rcvs);
}
