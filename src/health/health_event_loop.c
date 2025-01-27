// SPDX-License-Identifier: GPL-3.0-or-later

#include "health.h"
#include "health_internals.h"
#include "health-alert-entry.h"

#define WORKER_HEALTH_JOB_RRD_LOCK              0
#define WORKER_HEALTH_JOB_HOST_LOCK             1
#define WORKER_HEALTH_JOB_DB_QUERY              2
#define WORKER_HEALTH_JOB_CALC_EVAL             3
#define WORKER_HEALTH_JOB_WARNING_EVAL          4
#define WORKER_HEALTH_JOB_CRITICAL_EVAL         5
#define WORKER_HEALTH_JOB_ALARM_LOG_ENTRY       6
#define WORKER_HEALTH_JOB_ALARM_LOG_PROCESS     7
#define WORKER_HEALTH_JOB_ALARM_LOG_QUEUE       8
#define WORKER_HEALTH_JOB_WAIT_EXEC             9
#define WORKER_HEALTH_JOB_DELAYED_INIT_RRDSET   10
#define WORKER_HEALTH_JOB_DELAYED_INIT_RRDDIM   11

#if WORKER_UTILIZATION_MAX_JOB_TYPES < 10
#error WORKER_UTILIZATION_MAX_JOB_TYPES has to be at least 10
#endif

static uint64_t health_evloop_iteration = 0;

uint64_t health_evloop_current_iteration(void) {
    return __atomic_load_n(&health_evloop_iteration, __ATOMIC_RELAXED);
}

uint64_t rrdhost_health_evloop_last_iteration(RRDHOST *host) {
    return __atomic_load_n(&host->health.evloop_iteration, __ATOMIC_RELAXED);
}

void rrdhost_set_health_evloop_iteration(RRDHOST *host) {
    __atomic_store_n(&host->health.evloop_iteration,
                     health_evloop_current_iteration(), __ATOMIC_RELAXED);
}

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
            netdata_log_debug(D_HEALTH,
                "Health not examining alarm '%s.%s' yet (not enough data yet - we need %lu but got %lu - %lu).",
                rrdcalc_chart_name(rc),
                rrdcalc_name(rc),
                (unsigned long) needed,
                (unsigned long) first,
                (unsigned long) last);
            return 0;
        }
    }

    return 1;
}

static void health_sleep(time_t next_run, uint64_t loop __maybe_unused) {
    time_t now = now_realtime_sec();
    if(now < next_run) {
        worker_is_idle();
        netdata_log_debug(D_HEALTH, "Health monitoring iteration no %llu done. Next iteration in %d secs",
                          (unsigned long long)loop, (int) (next_run - now));
        while (now < next_run && service_running(SERVICE_HEALTH)) {
            sleep_usec(USEC_PER_SEC);
            now = now_realtime_sec();
        }
    }
    else {
        netdata_log_debug(D_HEALTH, "Health monitoring iteration no %llu done. Next iteration now",
                          (unsigned long long)loop);
    }
}

static void health_execute_delayed_initializations(RRDHOST *host) {
    health_plugin_init();

    RRDSET *st;

    if (!rrdhost_flag_check(host, RRDHOST_FLAG_PENDING_HEALTH_INITIALIZATION)) return;
    rrdhost_flag_clear(host, RRDHOST_FLAG_PENDING_HEALTH_INITIALIZATION);

    rrdset_foreach_reentrant(st, host) {
        if(!rrdset_flag_check(st, RRDSET_FLAG_PENDING_HEALTH_INITIALIZATION)) continue;
        rrdset_flag_clear(st, RRDSET_FLAG_PENDING_HEALTH_INITIALIZATION);

        worker_is_busy(WORKER_HEALTH_JOB_DELAYED_INIT_RRDSET);
        health_prototype_alerts_for_rrdset_incrementally(st);
    }
    rrdset_foreach_done(st);
}

static void health_initialize_rrdhost(RRDHOST *host) {
    health_plugin_init();

    if(!host->health.enabled ||
        rrdhost_flag_check(host, RRDHOST_FLAG_INITIALIZED_HEALTH) ||
        !service_running(SERVICE_HEALTH))
        return;

    rrdhost_flag_set(host, RRDHOST_FLAG_INITIALIZED_HEALTH);

    host->health_log.max = health_globals.config.health_log_entries_max;
    host->health_log.health_log_retention_s = health_globals.config.health_log_retention_s;
    host->health.default_exec = string_dup(health_globals.config.default_exec);
    host->health.default_recipient = string_dup(health_globals.config.default_recipient);
    host->health.use_summary_for_notifications = health_globals.config.use_summary_for_notifications;

    host->health_log.next_log_id = get_uint32_id();
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

static void do_eval_expression(
    RRDCALC *rc,
    EVAL_EXPRESSION *expression,
    const char *expression_type __maybe_unused,
    size_t job_type,
    RRDCALC_FLAGS error_type,
    RRDCALC_STATUS *calc_status,
    NETDATA_DOUBLE *result)
{
    if (!expression || (!calc_status && !result))
        return;

    worker_is_busy(job_type);

    if (unlikely(!expression_evaluate(expression))) {
        // calculation failed
        rc->run_flags |= error_type;
        if (result)
            *result = NAN;

        netdata_log_debug(D_HEALTH,
                          "Health on host '%s', alarm '%s.%s': %s expression failed with error: %s",
                          rrdhost_hostname(rc->rrdset->rrdhost), rrdcalc_chart_name(rc), rrdcalc_name(rc), expression_type,
                          expression_error_msg(expression)
        );
        return;
    }
    rc->run_flags &= ~error_type;
    netdata_log_debug(D_HEALTH,
                      "Health on host '%s', alarm '%s.%s': %s expression gave value "
                      NETDATA_DOUBLE_FORMAT ": %s (source: %s)",
                      rrdhost_hostname(rc->rrdset->rrdhost),
                      rrdcalc_chart_name(rc),
                      rrdcalc_name(rc),
                      expression_type,
                      expression_result(expression),
                      expression_error_msg(expression),
                      rrdcalc_source(rc));
    if (calc_status)
        *calc_status = rrdcalc_value2status(expression_result(expression));
    else
        *result = expression_result(expression);
}

// returns the number of runnable alerts
static void health_event_loop_for_host(RRDHOST *host, bool apply_hibernation_delay, time_t now, time_t *next_run) {
    size_t runnable = 0;

    if(unlikely(!rrdhost_should_run_health(host)))
        return;

    rrdhost_set_health_evloop_iteration(host);

    //#define rrdhost_pending_alert_transitions(host) (__atomic_load_n(&((host)->aclk_config.alert_transition.pending), __ATOMIC_RELAXED))

    if (unlikely(__atomic_load_n(&host->health.pending_transitions, __ATOMIC_RELAXED))) {
        nd_log(NDLS_DAEMON, NDLP_DEBUG,
               "Host \"%s\" has pending alert transitions to save, postponing health checks",
               rrdhost_hostname(host));
        return;
    }

    if (unlikely(!rrdhost_flag_check(host, RRDHOST_FLAG_INITIALIZED_HEALTH)))
        health_initialize_rrdhost(host);

    health_execute_delayed_initializations(host);

    if (unlikely(apply_hibernation_delay)) {
        nd_log(NDLS_DAEMON, NDLP_DEBUG,
               "[%s]: Postponing health checks for %"PRId32" seconds.",
               rrdhost_hostname(host),
               health_globals.config.postpone_alarms_during_hibernation_for_seconds);

        host->health.delay_up_to =
            now + health_globals.config.postpone_alarms_during_hibernation_for_seconds;
    }

    if (unlikely(host->health.delay_up_to)) {
        if (unlikely(now < host->health.delay_up_to))
            return;

        nd_log(NDLS_DAEMON, NDLP_DEBUG,
               "[%s]: Resuming health checks after delay.",
               rrdhost_hostname(host));

        host->health.delay_up_to = 0;
    }

    // wait until cleanup of obsolete charts on children is complete
    if (host != localhost) {
        if (unlikely(host->stream.rcv.status.check_obsolete)) {
            nd_log(NDLS_DAEMON, NDLP_DEBUG,
                   "[%s]: Waiting for chart obsoletion check.",
                   rrdhost_hostname(host));
            return;
        }
    }

    worker_is_busy(WORKER_HEALTH_JOB_HOST_LOCK);
    {
        struct aclk_sync_cfg_t *wc = host->aclk_config;
        if (wc && wc->send_snapshot == 2)
            return;
    }

    // the first loop is to lookup values from the db
    RRDCALC *rc;
    foreach_rrdcalc_in_rrdhost_read(host, rc) {
        if(unlikely(!service_running(SERVICE_HEALTH) || !rrdhost_should_run_health(host)))
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
                    health_alarm_log_add_entry(host, ae, false);
                    rc->old_status = rc->status;
                    rc->status = RRDCALC_STATUS_REMOVED;
                    rc->last_status_change = now_tmp;
                    rc->last_status_change_value = rc->value;
                    rc->last_updated = now_tmp;
                    rc->value = NAN;
                }
            }
        }

        if (unlikely(!rrdcalc_isrunnable(rc, now, next_run))) {
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

            char group_options_buf[100];
            const char *group_options = group_options_buf;
            switch(rc->config.time_group) {
                default:
                    group_options = NULL;
                    break;

                case RRDR_GROUPING_PERCENTILE:
                case RRDR_GROUPING_TRIMMED_MEAN:
                case RRDR_GROUPING_TRIMMED_MEDIAN:
                    snprintfz(group_options_buf, sizeof(group_options_buf),
                              NETDATA_DOUBLE_FORMAT_AUTO,
                              rc->config.time_group_value);
                    break;

                case RRDR_GROUPING_COUNTIF:
                    snprintfz(group_options_buf, sizeof(group_options_buf),
                              "%s" NETDATA_DOUBLE_FORMAT_AUTO,
                              alerts_group_conditions_id2txt(rc->config.time_group_condition),
                              rc->config.time_group_value);
                    break;
            }

            int ret = rrdset2value_api_v1(rc->rrdset, NULL, &rc->value, rrdcalc_dimensions(rc), 1,
                                          rc->config.after, rc->config.before, rc->config.time_group, group_options,
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

        do_eval_expression(rc, rc->config.calculation, "calculation", WORKER_HEALTH_JOB_CALC_EVAL, RRDCALC_FLAG_CALC_ERROR, NULL, &rc->value);
    }
    foreach_rrdcalc_in_rrdhost_done(rc);

    struct health_raised_summary *hrm = alerts_raised_summary_create(host);

    if (unlikely(runnable && service_running(SERVICE_HEALTH))) {
        foreach_rrdcalc_in_rrdhost_read(host, rc) {
            if(unlikely(!service_running(SERVICE_HEALTH) || !rrdhost_should_run_health(host)))
                break;

            if (unlikely(!(rc->run_flags & RRDCALC_FLAG_RUNNABLE)))
                continue;

            if (rc->run_flags & RRDCALC_FLAG_DISABLED) {
                continue;
            }
            RRDCALC_STATUS warning_status = RRDCALC_STATUS_UNDEFINED;
            RRDCALC_STATUS critical_status = RRDCALC_STATUS_UNDEFINED;

            do_eval_expression(rc, rc->config.warning, "warning", WORKER_HEALTH_JOB_WARNING_EVAL, RRDCALC_FLAG_WARN_ERROR, &warning_status, NULL);
            do_eval_expression(rc, rc->config.critical, "critical", WORKER_HEALTH_JOB_CRITICAL_EVAL, RRDCALC_FLAG_CRIT_ERROR, &critical_status, NULL);

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
                health_alarm_log_add_entry(host, ae, false);

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

            if (*next_run > rc->next_update)
                *next_run = rc->next_update;
        }
        foreach_rrdcalc_in_rrdhost_done(rc);

        alerts_raised_summary_populate(hrm);

        // process repeating alarms
        foreach_rrdcalc_in_rrdhost_read(host, rc) {
            if(unlikely(!service_running(SERVICE_HEALTH) || !rrdhost_should_run_health(host)))
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

    if(unlikely(!service_running(SERVICE_HEALTH) || !rrdhost_should_run_health(host))) {
        alerts_raised_summary_free(hrm);
        return;
    }

    // execute notifications
    // and cleanup

    worker_is_busy(WORKER_HEALTH_JOB_ALARM_LOG_PROCESS);
    health_alarm_log_process_to_send_notifications(host, hrm);
    alerts_raised_summary_free(hrm);

    int32_t pending = __atomic_load_n(&host->health.pending_transitions, __ATOMIC_RELAXED);
    if (pending)
        commit_alert_transitions(host);

    if (!__atomic_load_n(&host->health.pending_transitions, __ATOMIC_RELAXED)) {
        struct aclk_sync_cfg_t *wc = host->aclk_config;
        if (wc && wc->send_snapshot == 1) {
            wc->send_snapshot = 2;
            rrdhost_flag_set(host, RRDHOST_FLAG_ACLK_STREAM_ALERTS);
        } else {
            worker_is_busy(WORKER_HEALTH_JOB_ALARM_LOG_QUEUE);
            if (process_alert_pending_queue(host))
                rrdhost_flag_set(host, RRDHOST_FLAG_ACLK_STREAM_ALERTS);
        }
    }
    worker_is_idle();
}

__thread bool is_health_thread = false;
static void health_event_loop(void) {

    is_health_thread = true;
    while(service_running(SERVICE_HEALTH)) {
        if(!stream_control_health_should_be_running()) {
            worker_is_idle();
            stream_control_throttle();
            continue;
        }

        time_t now = now_realtime_sec();
        bool apply_hibernation_delay = false;
        time_t next_run = now + health_globals.config.run_at_least_every_seconds;

        if (unlikely(check_if_resumed_from_suspension())) {
            apply_hibernation_delay = true;

            nd_log(NDLS_DAEMON, NDLP_NOTICE,
                   "Postponing alarm checks for %"PRId32" seconds, "
                   "because it seems that the system was just resumed from suspension.",
                   (int32_t)health_globals.config.postpone_alarms_during_hibernation_for_seconds);
            schedule_node_state_update(localhost, 0);
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
        uint64_t loop = __atomic_add_fetch(&health_evloop_iteration, 1, __ATOMIC_RELAXED);

        RRDHOST *host;
        dfe_start_reentrant(rrdhost_root_index, host) {
            if(unlikely(!service_running(SERVICE_HEALTH)))
                break;

            health_event_loop_for_host(host, apply_hibernation_delay, now, &next_run);
        }
        dfe_done(host);

        if(unlikely(!service_running(SERVICE_HEALTH)))
            break;

        // wait for all notifications to finish before allowing health to be cleaned up
        worker_is_busy(WORKER_HEALTH_JOB_WAIT_EXEC);
        wait_for_all_notifications_to_finish_before_allowing_health_to_be_cleaned_up();
        worker_is_idle();
        
        health_sleep(next_run, loop);
    } // forever
}


static void health_main_cleanup(void *pptr) {
    struct netdata_static_thread *static_thread = CLEANUP_FUNCTION_GET_PTR(pptr);
    if(!static_thread) return;

    worker_unregister();
    static_thread->enabled = NETDATA_MAIN_THREAD_EXITING;
    static_thread->enabled = NETDATA_MAIN_THREAD_EXITED;

    nd_log(NDLS_DAEMON, NDLP_DEBUG, "Health thread ended.");
}

void *health_main(void *ptr) {
    worker_register("HEALTH");
    worker_register_job_name(WORKER_HEALTH_JOB_RRD_LOCK, "rrd lock");
    worker_register_job_name(WORKER_HEALTH_JOB_HOST_LOCK, "host lock");
    worker_register_job_name(WORKER_HEALTH_JOB_DB_QUERY, "db lookup");
    worker_register_job_name(WORKER_HEALTH_JOB_CALC_EVAL, "calc eval");
    worker_register_job_name(WORKER_HEALTH_JOB_WARNING_EVAL, "warning eval");
    worker_register_job_name(WORKER_HEALTH_JOB_CRITICAL_EVAL, "critical eval");
    worker_register_job_name(WORKER_HEALTH_JOB_ALARM_LOG_ENTRY, "alert log entry");
    worker_register_job_name(WORKER_HEALTH_JOB_ALARM_LOG_PROCESS, "alert log process");
    worker_register_job_name(WORKER_HEALTH_JOB_ALARM_LOG_QUEUE, "alert log queue");
    worker_register_job_name(WORKER_HEALTH_JOB_WAIT_EXEC, "alert wait exec");
    worker_register_job_name(WORKER_HEALTH_JOB_DELAYED_INIT_RRDSET, "rrdset init");
    worker_register_job_name(WORKER_HEALTH_JOB_DELAYED_INIT_RRDDIM, "rrddim init");

    CLEANUP_FUNCTION_REGISTER(health_main_cleanup) cleanup_ptr = ptr;
    health_event_loop();
    return NULL;
}
