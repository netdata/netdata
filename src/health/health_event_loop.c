// SPDX-License-Identifier: GPL-3.0-or-later

#include "health.h"
#include "health_internals.h"

#define WORKER_HEALTH_JOB_RRD_LOCK              0
#define WORKER_HEALTH_JOB_HOST_LOCK             1
#define WORKER_HEALTH_JOB_DB_QUERY              2
#define WORKER_HEALTH_JOB_CALC_EVAL             3
#define WORKER_HEALTH_JOB_WARNING_EVAL          4
#define WORKER_HEALTH_JOB_CRITICAL_EVAL         5
#define WORKER_HEALTH_JOB_ALARM_LOG_ENTRY       6
#define WORKER_HEALTH_JOB_ALARM_LOG_PROCESS     7
#define WORKER_HEALTH_JOB_DELAYED_INIT_RRDSET   8
#define WORKER_HEALTH_JOB_DELAYED_INIT_RRDDIM   9

#if WORKER_UTILIZATION_MAX_JOB_TYPES < 10
#error WORKER_UTILIZATION_MAX_JOB_TYPES has to be at least 10
#endif

// ----------------------------------------------------------------------------
// health main thread and friends

static inline RRDCALC_STATUS rrdcalc_value2status(NETDATA_DOUBLE n) {
    if(isnan(n) || isinf(n)) return RRDCALC_STATUS_UNDEFINED;
    if(n) return RRDCALC_STATUS_RAISED;
    return RRDCALC_STATUS_CLEAR;
}

static inline int rrdcalc_isrunnable(RRDCALC *rc, time_t now, time_t *next_run) {
    if(unlikely(!rc->rrdset)) {
        netdata_log_debug(D_HEALTH, "Health not running alarm '%s.%s'. It is not linked to a chart.", rrdcalc_chart_name(rc), rrdcalc_name(rc));
        return 0;
    }

    if(unlikely(rc->next_update > now)) {
        if (unlikely(*next_run > rc->next_update)) {
            // update the next_run time of the main loop
            // to run this alarm precisely the time required
            *next_run = rc->next_update;
        }

        netdata_log_debug(D_HEALTH, "Health not examining alarm '%s.%s' yet (will do in %d secs).", rrdcalc_chart_name(rc), rrdcalc_name(rc), (int) (rc->next_update - now));
        return 0;
    }

    if(unlikely(!rc->config.update_every)) {
        netdata_log_debug(D_HEALTH, "Health not running alarm '%s.%s'. It does not have an update frequency", rrdcalc_chart_name(rc), rrdcalc_name(rc));
        return 0;
    }

    if(unlikely(rrdset_flag_check(rc->rrdset, RRDSET_FLAG_OBSOLETE))) {
        netdata_log_debug(D_HEALTH, "Health not running alarm '%s.%s'. The chart has been marked as obsolete", rrdcalc_chart_name(rc), rrdcalc_name(rc));
        return 0;
    }

    if(unlikely(!rc->rrdset->last_collected_time.tv_sec || rc->rrdset->counter_done < 2)) {
        netdata_log_debug(D_HEALTH, "Health not running alarm '%s.%s'. Chart is not fully collected yet.", rrdcalc_chart_name(rc), rrdcalc_name(rc));
        return 0;
    }

    int update_every = rc->rrdset->update_every;
    time_t first = rrdset_first_entry_s(rc->rrdset);
    time_t last = rrdset_last_entry_s(rc->rrdset);

    if(unlikely(now + update_every < first /* || now - update_every > last */)) {
        netdata_log_debug(D_HEALTH
                          , "Health not examining alarm '%s.%s' yet (wanted time is out of bounds - we need %lu but got %lu - %lu)."
                          , rrdcalc_chart_name(rc), rrdcalc_name(rc), (unsigned long) now, (unsigned long) first
                          , (unsigned long) last);
        return 0;
    }

    if(RRDCALC_HAS_DB_LOOKUP(rc)) {
        time_t needed = now + rc->config.before + rc->config.after;

        if(needed + update_every < first || needed - update_every > last) {
            netdata_log_debug(D_HEALTH
                              , "Health not examining alarm '%s.%s' yet (not enough data yet - we need %lu but got %lu - %lu)."
                              , rrdcalc_chart_name(rc), rrdcalc_name(rc), (unsigned long) needed, (unsigned long) first
                              , (unsigned long) last);
            return 0;
        }
    }

    return 1;
}

static void health_sleep(time_t next_run, unsigned int loop __maybe_unused) {
    time_t now = now_realtime_sec();
    if(now < next_run) {
        worker_is_idle();
        netdata_log_debug(D_HEALTH, "Health monitoring iteration no %u done. Next iteration in %d secs", loop, (int) (next_run - now));
        while (now < next_run && service_running(SERVICE_HEALTH)) {
            sleep_usec(USEC_PER_SEC);
            now = now_realtime_sec();
        }
    }
    else {
        netdata_log_debug(D_HEALTH, "Health monitoring iteration no %u done. Next iteration now", loop);
    }
}

static void sql_health_postpone_queue_removed(RRDHOST *host __maybe_unused) {
#ifdef ENABLE_ACLK
    if (netdata_cloud_enabled) {
        struct aclk_sync_cfg_t *wc = host->aclk_config;
        if (unlikely(!wc)) {
            return;
        }

        if (wc->alert_queue_removed >= 1) {
            wc->alert_queue_removed+=6;
        }
    }
#endif
}

static void health_execute_delayed_initializations(RRDHOST *host) {
    health_plugin_init();

    RRDSET *st;
    bool must_postpone = false;

    if (!rrdhost_flag_check(host, RRDHOST_FLAG_PENDING_HEALTH_INITIALIZATION)) return;
    rrdhost_flag_clear(host, RRDHOST_FLAG_PENDING_HEALTH_INITIALIZATION);

    rrdset_foreach_reentrant(st, host) {
        if(!rrdset_flag_check(st, RRDSET_FLAG_PENDING_HEALTH_INITIALIZATION)) continue;
        rrdset_flag_clear(st, RRDSET_FLAG_PENDING_HEALTH_INITIALIZATION);

        worker_is_busy(WORKER_HEALTH_JOB_DELAYED_INIT_RRDSET);
        health_prototype_alerts_for_rrdset_incrementally(st);
        must_postpone = true;
    }
    rrdset_foreach_done(st);
    if (must_postpone)
        sql_health_postpone_queue_removed(host);
}

static void health_initialize_rrdhost(RRDHOST *host) {
    health_plugin_init();

    if(!host->health.health_enabled ||
        rrdhost_flag_check(host, RRDHOST_FLAG_INITIALIZED_HEALTH) ||
        !service_running(SERVICE_HEALTH))
        return;

    rrdhost_flag_set(host, RRDHOST_FLAG_INITIALIZED_HEALTH);

    host->health.health_default_warn_repeat_every = health_globals.config.default_warn_repeat_every;
    host->health.health_default_crit_repeat_every = health_globals.config.default_crit_repeat_every;
    host->health_log.max = health_globals.config.health_log_entries_max;
    host->health_log.health_log_history = health_globals.config.health_log_history;
    host->health.health_default_exec = string_dup(health_globals.config.default_exec);
    host->health.health_default_recipient = string_dup(health_globals.config.default_recipient);
    host->health.use_summary_for_notifications = health_globals.config.use_summary_for_notifications;

    host->health_log.next_log_id = (uint32_t)now_realtime_sec();
    host->health_log.next_alarm_id = 0;

    rw_spinlock_init(&host->health_log.spinlock);
    sql_health_alarm_log_load(host);
    health_apply_prototypes_to_host(host);
}

static inline int check_if_resumed_from_suspension(void) {
    static usec_t last_realtime = 0, last_monotonic = 0;
    usec_t realtime = now_realtime_usec(), monotonic = now_monotonic_usec();
    int ret = 0;

    // detect if monotonic and realtime have twice the difference
    // in which case we assume the system was just waken from hibernation

    if(last_realtime && last_monotonic && realtime - last_realtime > 2 * (monotonic - last_monotonic))
        ret = 1;

    last_realtime = realtime;
    last_monotonic = monotonic;

    return ret;
}

static void health_event_loop(void) {
    bool health_running_logged = false;

    unsigned int loop = 0;

    while(service_running(SERVICE_HEALTH)) {
        loop++;
        netdata_log_debug(D_HEALTH, "Health monitoring iteration no %u started", loop);

        time_t now = now_realtime_sec();
        int runnable = 0, apply_hibernation_delay = 0;
        time_t next_run = now + health_globals.config.run_at_least_every_seconds;
        RRDCALC *rc;
        RRDHOST *host;

        if (unlikely(check_if_resumed_from_suspension())) {
            apply_hibernation_delay = 1;

            nd_log(NDLS_DAEMON, NDLP_NOTICE,
                   "Postponing alarm checks for %"PRId32" seconds, "
                   "because it seems that the system was just resumed from suspension.",
                   (int32_t)health_globals.config.postpone_alarms_during_hibernation_for_seconds);
        }

        if (unlikely(silencers->all_alarms && silencers->stype == STYPE_DISABLE_ALARMS)) {
            static int logged=0;
            if (!logged) {
                nd_log(NDLS_DAEMON, NDLP_DEBUG,
                       "Skipping health checks, because all alarms are disabled via API command.");
                logged = 1;
            }
        }

        worker_is_busy(WORKER_HEALTH_JOB_RRD_LOCK);
        dfe_start_reentrant(rrdhost_root_index, host) {

            if(unlikely(!service_running(SERVICE_HEALTH)))
                break;

            if (unlikely(!host->health.health_enabled))
                continue;

            if (unlikely(!rrdhost_flag_check(host, RRDHOST_FLAG_INITIALIZED_HEALTH)))
                health_initialize_rrdhost(host);

            health_execute_delayed_initializations(host);

            if (unlikely(apply_hibernation_delay)) {
                nd_log(NDLS_DAEMON, NDLP_DEBUG,
                       "[%s]: Postponing health checks for %"PRId32" seconds.",
                       rrdhost_hostname(host),
                       health_globals.config.postpone_alarms_during_hibernation_for_seconds);

                host->health.health_delay_up_to =
                    now + health_globals.config.postpone_alarms_during_hibernation_for_seconds;
            }

            if (unlikely(host->health.health_delay_up_to)) {
                if (unlikely(now < host->health.health_delay_up_to)) {
                    continue;
                }

                nd_log(NDLS_DAEMON, NDLP_DEBUG,
                       "[%s]: Resuming health checks after delay.",
                       rrdhost_hostname(host));

                host->health.health_delay_up_to = 0;
            }

            // wait until cleanup of obsolete charts on children is complete
            if (host != localhost) {
                if (unlikely(host->trigger_chart_obsoletion_check == 1)) {

                    nd_log(NDLS_DAEMON, NDLP_DEBUG,
                           "[%s]: Waiting for chart obsoletion check.",
                           rrdhost_hostname(host));

                    continue;
                }
            }

            if (!health_running_logged) {
                nd_log(NDLS_DAEMON, NDLP_DEBUG,
                       "[%s]: Health is running.",
                       rrdhost_hostname(host));

                health_running_logged = true;
            }

            worker_is_busy(WORKER_HEALTH_JOB_HOST_LOCK);

            // the first loop is to lookup values from the db
            foreach_rrdcalc_in_rrdhost_read(host, rc) {

                if(unlikely(!service_running(SERVICE_HEALTH)))
                    break;

                rrdcalc_update_info_using_rrdset_labels(rc);

                if (health_silencers_update_disabled_silenced(host, rc))
                    continue;

                // create an alert removed event if the chart is obsolete and
                // has stopped being collected for 60 seconds
                if (unlikely(rc->rrdset && rc->status != RRDCALC_STATUS_REMOVED &&
                             rrdset_flag_check(rc->rrdset, RRDSET_FLAG_OBSOLETE) &&
                             now > (rc->rrdset->last_collected_time.tv_sec + 60))) {
                    if (!rrdcalc_isrepeating(rc)) {
                        worker_is_busy(WORKER_HEALTH_JOB_ALARM_LOG_ENTRY);
                        time_t now_tmp = now_realtime_sec();

                        ALARM_ENTRY *ae =
                            health_create_alarm_entry(
                                host,
                                rc,
                                now_tmp,
                                now_tmp - rc->last_status_change,
                                rc->value,
                                NAN,
                                rc->status,
                                RRDCALC_STATUS_REMOVED,
                                0,
                                rrdcalc_isrepeating(rc)?HEALTH_ENTRY_FLAG_IS_REPEATING:0);

                        if (ae) {
                            health_log_alert(host, ae);
                            health_alarm_log_add_entry(host, ae);
                            rc->old_status = rc->status;
                            rc->status = RRDCALC_STATUS_REMOVED;
                            rc->last_status_change = now_tmp;
                            rc->last_status_change_value = rc->value;
                            rc->last_updated = now_tmp;
                            rc->value = NAN;

#ifdef ENABLE_ACLK
                            if (netdata_cloud_enabled)
                                sql_queue_alarm_to_aclk(host, ae, true);
#endif
                        }
                    }
                }

                if (unlikely(!rrdcalc_isrunnable(rc, now, &next_run))) {
                    if (unlikely(rc->run_flags & RRDCALC_FLAG_RUNNABLE))
                        rc->run_flags &= ~RRDCALC_FLAG_RUNNABLE;
                    continue;
                }

                runnable++;
                rc->old_value = rc->value;
                rc->run_flags |= RRDCALC_FLAG_RUNNABLE;

                // ------------------------------------------------------------
                // if there is database lookup, do it

                if (unlikely(RRDCALC_HAS_DB_LOOKUP(rc))) {
                    worker_is_busy(WORKER_HEALTH_JOB_DB_QUERY);

                    /* time_t old_db_timestamp = rc->db_before; */
                    int value_is_null = 0;

                    int ret = rrdset2value_api_v1(rc->rrdset, NULL, &rc->value, rrdcalc_dimensions(rc), 1,
                                                  rc->config.after, rc->config.before, rc->config.group, NULL,
                                                  0, rc->config.options | RRDR_OPTION_SELECTED_TIER,
                                                  &rc->db_after,&rc->db_before,
                                                  NULL, NULL, NULL,
                                                  &value_is_null, NULL, 0, 0,
                                                  QUERY_SOURCE_HEALTH, STORAGE_PRIORITY_SYNCHRONOUS);

                    if (unlikely(ret != 200)) {
                        // database lookup failed
                        rc->value = NAN;
                        rc->run_flags |= RRDCALC_FLAG_DB_ERROR;

                        netdata_log_debug(D_HEALTH, "Health on host '%s', alarm '%s.%s': database lookup returned error %d",
                                          rrdhost_hostname(host), rrdcalc_chart_name(rc), rrdcalc_name(rc), ret
                        );
                    } else
                        rc->run_flags &= ~RRDCALC_FLAG_DB_ERROR;

                    if (unlikely(value_is_null)) {
                        // collected value is null
                        rc->value = NAN;
                        rc->run_flags |= RRDCALC_FLAG_DB_NAN;

                        netdata_log_debug(D_HEALTH,
                                          "Health on host '%s', alarm '%s.%s': database lookup returned empty value (possibly value is not collected yet)",
                                          rrdhost_hostname(host), rrdcalc_chart_name(rc), rrdcalc_name(rc)
                        );
                    } else
                        rc->run_flags &= ~RRDCALC_FLAG_DB_NAN;

                    netdata_log_debug(D_HEALTH, "Health on host '%s', alarm '%s.%s': database lookup gave value " NETDATA_DOUBLE_FORMAT,
                                      rrdhost_hostname(host), rrdcalc_chart_name(rc), rrdcalc_name(rc), rc->value
                    );
                }

                // ------------------------------------------------------------
                // if there is calculation expression, run it

                if (unlikely(rc->config.calculation)) {
                    worker_is_busy(WORKER_HEALTH_JOB_CALC_EVAL);

                    if (unlikely(!expression_evaluate(rc->config.calculation))) {
                        // calculation failed
                        rc->value = NAN;
                        rc->run_flags |= RRDCALC_FLAG_CALC_ERROR;

                        netdata_log_debug(
                            D_HEALTH, "Health on host '%s', alarm '%s.%s': expression '%s' failed: %s",
                            rrdhost_hostname(host), rrdcalc_chart_name(rc), rrdcalc_name(rc),
                            expression_parsed_as(rc->config.calculation), expression_error_msg(rc->config.calculation)
                        );
                    }
                    else {
                        rc->run_flags &= ~RRDCALC_FLAG_CALC_ERROR;

                        netdata_log_debug(
                            D_HEALTH, "Health on host '%s', alarm '%s.%s': expression '%s' gave value "
                            NETDATA_DOUBLE_FORMAT": %s (source: %s)",
                            rrdhost_hostname(host), rrdcalc_chart_name(rc), rrdcalc_name(rc),
                            expression_parsed_as(rc->config.calculation),
                            expression_result(rc->config.calculation),
                            expression_error_msg(rc->config.calculation),
                            rrdcalc_source(rc)
                        );

                        rc->value = expression_result(rc->config.calculation);
                    }
                }
            }
            foreach_rrdcalc_in_rrdhost_done(rc);

            struct health_raised_summary *hrm = alerts_raised_summary_create(host);

            if (unlikely(runnable && service_running(SERVICE_HEALTH))) {
                foreach_rrdcalc_in_rrdhost_read(host, rc) {
                    if(unlikely(!service_running(SERVICE_HEALTH)))
                        break;

                    if (unlikely(!(rc->run_flags & RRDCALC_FLAG_RUNNABLE)))
                        continue;

                    if (rc->run_flags & RRDCALC_FLAG_DISABLED) {
                        continue;
                    }
                    RRDCALC_STATUS warning_status = RRDCALC_STATUS_UNDEFINED;
                    RRDCALC_STATUS critical_status = RRDCALC_STATUS_UNDEFINED;

                    // --------------------------------------------------------
                    // check the warning expression

                    if (likely(rc->config.warning)) {
                        worker_is_busy(WORKER_HEALTH_JOB_WARNING_EVAL);

                        if (unlikely(!expression_evaluate(rc->config.warning))) {
                            // calculation failed
                            rc->run_flags |= RRDCALC_FLAG_WARN_ERROR;

                            netdata_log_debug(D_HEALTH,
                                              "Health on host '%s', alarm '%s.%s': warning expression failed with error: %s",
                                              rrdhost_hostname(host), rrdcalc_chart_name(rc), rrdcalc_name(rc),
                                              expression_error_msg(rc->config.warning)
                            );
                        } else {
                            rc->run_flags &= ~RRDCALC_FLAG_WARN_ERROR;
                            netdata_log_debug(D_HEALTH,
                                              "Health on host '%s', alarm '%s.%s': warning expression gave value "
                                              NETDATA_DOUBLE_FORMAT ": %s (source: %s)",
                                              rrdhost_hostname(host),
                                              rrdcalc_chart_name(rc),
                                              rrdcalc_name(rc),
                                              expression_result(rc->config.warning),
                                              expression_error_msg(rc->config.warning),
                                              rrdcalc_source(rc)
                            );
                            warning_status = rrdcalc_value2status(expression_result(rc->config.warning));
                        }
                    }

                    // --------------------------------------------------------
                    // check the critical expression

                    if (likely(rc->config.critical)) {
                        worker_is_busy(WORKER_HEALTH_JOB_CRITICAL_EVAL);

                        if (unlikely(!expression_evaluate(rc->config.critical))) {
                            // calculation failed
                            rc->run_flags |= RRDCALC_FLAG_CRIT_ERROR;

                            netdata_log_debug(D_HEALTH,
                                              "Health on host '%s', alarm '%s.%s': critical expression failed with error: %s",
                                              rrdhost_hostname(host), rrdcalc_chart_name(rc), rrdcalc_name(rc),
                                              expression_error_msg(rc->config.critical)
                            );
                        } else {
                            rc->run_flags &= ~RRDCALC_FLAG_CRIT_ERROR;
                            netdata_log_debug(D_HEALTH,
                                              "Health on host '%s', alarm '%s.%s': critical expression gave value "
                                              NETDATA_DOUBLE_FORMAT ": %s (source: %s)",
                                              rrdhost_hostname(host), rrdcalc_chart_name(rc), rrdcalc_name(rc),
                                              expression_result(rc->config.critical),
                                              expression_error_msg(rc->config.critical),
                                              rrdcalc_source(rc)
                            );
                            critical_status = rrdcalc_value2status(expression_result(rc->config.critical));
                        }
                    }

                    // --------------------------------------------------------
                    // decide the final alarm status

                    RRDCALC_STATUS status = RRDCALC_STATUS_UNDEFINED;

                    switch (warning_status) {
                        case RRDCALC_STATUS_CLEAR:
                            status = RRDCALC_STATUS_CLEAR;
                            break;

                        case RRDCALC_STATUS_RAISED:
                            status = RRDCALC_STATUS_WARNING;
                            break;

                        default:
                            break;
                    }

                    switch (critical_status) {
                        case RRDCALC_STATUS_CLEAR:
                            if (status == RRDCALC_STATUS_UNDEFINED)
                                status = RRDCALC_STATUS_CLEAR;
                            break;

                        case RRDCALC_STATUS_RAISED:
                            status = RRDCALC_STATUS_CRITICAL;
                            break;

                        default:
                            break;
                    }

                    // --------------------------------------------------------
                    // check if the new status and the old differ

                    if (status != rc->status) {

                        worker_is_busy(WORKER_HEALTH_JOB_ALARM_LOG_ENTRY);
                        int delay;

                        // apply trigger hysteresis

                        if (now > rc->delay_up_to_timestamp) {
                            rc->delay_up_current = rc->config.delay_up_duration;
                            rc->delay_down_current = rc->config.delay_down_duration;
                            rc->delay_last = 0;
                            rc->delay_up_to_timestamp = 0;
                        } else {
                            rc->delay_up_current = (int)((float)rc->delay_up_current * rc->config.delay_multiplier);
                            if (rc->delay_up_current > rc->config.delay_max_duration)
                                rc->delay_up_current = rc->config.delay_max_duration;

                            rc->delay_down_current = (int)((float)rc->delay_down_current * rc->config.delay_multiplier);
                            if (rc->delay_down_current > rc->config.delay_max_duration)
                                rc->delay_down_current = rc->config.delay_max_duration;
                        }

                        if (status > rc->status)
                            delay = rc->delay_up_current;
                        else
                            delay = rc->delay_down_current;

                        // COMMENTED: because we do need to send raising alarms
                        // if (now + delay < rc->delay_up_to_timestamp)
                        //      delay = (int)(rc->delay_up_to_timestamp - now);

                        rc->delay_last = delay;
                        rc->delay_up_to_timestamp = now + delay;

                        ALARM_ENTRY *ae =
                            health_create_alarm_entry(
                                host,
                                rc,
                                now,
                                now - rc->last_status_change,
                                rc->old_value,
                                rc->value,
                                rc->status,
                                status,
                                rc->delay_last,
                                (
                                    ((rc->config.alert_action_options & ALERT_ACTION_OPTION_NO_CLEAR_NOTIFICATION)? HEALTH_ENTRY_FLAG_NO_CLEAR_NOTIFICATION : 0) |
                                    ((rc->run_flags & RRDCALC_FLAG_SILENCED)? HEALTH_ENTRY_FLAG_SILENCED : 0) |
                                    (rrdcalc_isrepeating(rc)?HEALTH_ENTRY_FLAG_IS_REPEATING:0)
                                        )
                            );

                        health_log_alert(host, ae);
                        health_alarm_log_add_entry(host, ae);

                        nd_log(NDLS_DAEMON, NDLP_DEBUG,
                               "[%s]: Alert event for [%s.%s], value [%s], status [%s].",
                               rrdhost_hostname(host), ae_chart_id(ae), ae_name(ae), ae_new_value_string(ae),
                               rrdcalc_status2string(ae->new_status));

                        rc->last_status_change_value = rc->value;
                        rc->last_status_change = now;
                        rc->old_status = rc->status;
                        rc->status = status;

                        if(unlikely(rrdcalc_isrepeating(rc))) {
                            rc->last_repeat = now;
                            if (rc->status == RRDCALC_STATUS_CLEAR)
                                rc->run_flags |= RRDCALC_FLAG_RUN_ONCE;
                        }
                    }

                    rc->last_updated = now;
                    rc->next_update = now + rc->config.update_every;

                    if (next_run > rc->next_update)
                        next_run = rc->next_update;
                }
                foreach_rrdcalc_in_rrdhost_done(rc);

                alerts_raised_summary_populate(hrm);

                // process repeating alarms
                foreach_rrdcalc_in_rrdhost_read(host, rc) {
                    if(unlikely(!service_running(SERVICE_HEALTH)))
                        break;

                    int repeat_every = 0;
                    if(unlikely(rrdcalc_isrepeating(rc) && rc->delay_up_to_timestamp <= now)) {
                        if(unlikely(rc->status == RRDCALC_STATUS_WARNING)) {
                            rc->run_flags &= ~RRDCALC_FLAG_RUN_ONCE;
                            repeat_every = (int)rc->config.warn_repeat_every;
                        }
                        else if(unlikely(rc->status == RRDCALC_STATUS_CRITICAL)) {
                            rc->run_flags &= ~RRDCALC_FLAG_RUN_ONCE;
                            repeat_every = (int)rc->config.crit_repeat_every;
                        }
                        else if(unlikely(rc->status == RRDCALC_STATUS_CLEAR)) {
                            if(!(rc->run_flags & RRDCALC_FLAG_RUN_ONCE) &&
                                (rc->old_status == RRDCALC_STATUS_CRITICAL || rc->old_status == RRDCALC_STATUS_WARNING))
                                repeat_every = 1;
                        }
                    }
                    else
                        continue;

                    if(unlikely(repeat_every > 0 && (rc->last_repeat + repeat_every) <= now)) {
                        worker_is_busy(WORKER_HEALTH_JOB_ALARM_LOG_ENTRY);
                        rc->last_repeat = now;
                        if (likely(rc->times_repeat < UINT32_MAX)) rc->times_repeat++;
                        ALARM_ENTRY *ae =
                            health_create_alarm_entry(
                                host,
                                rc,
                                now,
                                now - rc->last_status_change,
                                rc->old_value,
                                rc->value,
                                rc->old_status,
                                rc->status,
                                rc->delay_last,
                                (
                                    ((rc->config.alert_action_options & ALERT_ACTION_OPTION_NO_CLEAR_NOTIFICATION)? HEALTH_ENTRY_FLAG_NO_CLEAR_NOTIFICATION : 0) |
                                    ((rc->run_flags & RRDCALC_FLAG_SILENCED)? HEALTH_ENTRY_FLAG_SILENCED : 0) |
                                    (rrdcalc_isrepeating(rc)?HEALTH_ENTRY_FLAG_IS_REPEATING:0)
                                        )
                            );

                        health_log_alert(host, ae);
                        ae->last_repeat = rc->last_repeat;
                        if (!(rc->run_flags & RRDCALC_FLAG_RUN_ONCE) && rc->status == RRDCALC_STATUS_CLEAR) {
                            ae->flags |= HEALTH_ENTRY_RUN_ONCE;
                        }
                        rc->run_flags |= RRDCALC_FLAG_RUN_ONCE;
                        health_send_notification(host, ae, hrm);
                        netdata_log_debug(D_HEALTH, "Notification sent for the repeating alarm %u.", ae->alarm_id);
                        health_alarm_wait_for_execution(ae);
                        health_alarm_log_free_one_nochecks_nounlink(ae);
                    }
                }
                foreach_rrdcalc_in_rrdhost_done(rc);
            }

            if (unlikely(!service_running(SERVICE_HEALTH)))
                break;

            // execute notifications
            // and cleanup

            worker_is_busy(WORKER_HEALTH_JOB_ALARM_LOG_PROCESS);
            health_alarm_log_process_to_send_notifications(host, hrm);
            alerts_raised_summary_free(hrm);

            if (unlikely(!service_running(SERVICE_HEALTH))) {
                // wait for all notifications to finish before allowing health to be cleaned up
                wait_for_all_notifications_to_finish_before_allowing_health_to_be_cleaned_up();
                break;
            }
#ifdef ENABLE_ACLK
            if (netdata_cloud_enabled) {
                struct aclk_sync_cfg_t *wc = host->aclk_config;
                if (unlikely(!wc))
                    continue;

                if (wc->alert_queue_removed == 1) {
                    sql_queue_removed_alerts_to_aclk(host);
                } else if (wc->alert_queue_removed > 1) {
                    wc->alert_queue_removed--;
                }

                if (wc->alert_checkpoint_req == 1) {
                    aclk_push_alarm_checkpoint(host);
                } else if (wc->alert_checkpoint_req > 1) {
                    wc->alert_checkpoint_req--;
                }
            }
#endif
        }
        dfe_done(host);

        // wait for all notifications to finish before allowing health to be cleaned up
        wait_for_all_notifications_to_finish_before_allowing_health_to_be_cleaned_up();

        if(unlikely(!service_running(SERVICE_HEALTH)))
            break;

        health_sleep(next_run, loop);

    } // forever
}


static void health_main_cleanup(void *ptr) {
    worker_unregister();

    struct netdata_static_thread *static_thread = (struct netdata_static_thread *)ptr;
    static_thread->enabled = NETDATA_MAIN_THREAD_EXITING;
    netdata_log_info("cleaning up...");
    static_thread->enabled = NETDATA_MAIN_THREAD_EXITED;

    nd_log(NDLS_DAEMON, NDLP_DEBUG,
           "Health thread ended.");
}

void *health_main(void *ptr) {
    worker_register("HEALTH");
    worker_register_job_name(WORKER_HEALTH_JOB_RRD_LOCK, "rrd lock");
    worker_register_job_name(WORKER_HEALTH_JOB_HOST_LOCK, "host lock");
    worker_register_job_name(WORKER_HEALTH_JOB_DB_QUERY, "db lookup");
    worker_register_job_name(WORKER_HEALTH_JOB_CALC_EVAL, "calc eval");
    worker_register_job_name(WORKER_HEALTH_JOB_WARNING_EVAL, "warning eval");
    worker_register_job_name(WORKER_HEALTH_JOB_CRITICAL_EVAL, "critical eval");
    worker_register_job_name(WORKER_HEALTH_JOB_ALARM_LOG_ENTRY, "alarm log entry");
    worker_register_job_name(WORKER_HEALTH_JOB_ALARM_LOG_PROCESS, "alarm log process");
    worker_register_job_name(WORKER_HEALTH_JOB_DELAYED_INIT_RRDSET, "rrdset init");
    worker_register_job_name(WORKER_HEALTH_JOB_DELAYED_INIT_RRDDIM, "rrddim init");

    netdata_thread_cleanup_push(health_main_cleanup, ptr);
    {
        health_event_loop();
    }
    netdata_thread_cleanup_pop(1);
    return NULL;
}
