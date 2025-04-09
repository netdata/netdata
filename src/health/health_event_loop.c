// SPDX-License-Identifier: GPL-3.0-or-later

#include "health.h"
#include "health_internals.h"
#include "health-alert-entry.h"

void health_host_run(RRDHOST *host);
void health_host_cleanup(RRDHOST *host);
void health_host_run_later(RRDHOST *host, uint64_t delay);
void health_host_initialize(RRDHOST *host);
void health_host_maintenance(RRDHOST *host);
void health_run_jobs();
static void host_health_timer_cb(uv_timer_t *handle);

#define HEALTH_HOST_MAINTENANCE_INTERVAL (3600)     // Cleanup host alert transitions (in seconds)

#define MAX_WORKER_DATA (256)

#define COMPUTE_DURATION(var_name, unit, start, end)      \
    char var_name[64];                                    \
    duration_snprintf(var_name, sizeof(var_name),         \
                      (int64_t)((end) - (start)), unit, true)

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

static void perform_repeated_alarm(RRDHOST *host, RRDCALC *rc,  struct health_raised_summary *hrm, time_t now)
{
    worker_is_busy(UV_EVENT_HEALTH_JOB_ALARM_LOG_ENTRY);
    rc->last_repeat = now;

    if (likely(rc->times_repeat < UINT32_MAX))
        rc->times_repeat++;

    ALARM_ENTRY *ae = health_create_alarm_entry(
        host,
        rc,
        now,
        now - rc->last_status_change,
        rc->old_value,
        rc->value,
        rc->old_status,
        rc->status,
        rc->delay_last,
        (((rc->config.alert_action_options & ALERT_ACTION_OPTION_NO_CLEAR_NOTIFICATION) ?
              HEALTH_ENTRY_FLAG_NO_CLEAR_NOTIFICATION :
              0) |
         ((rc->run_flags & RRDCALC_FLAG_SILENCED) ? HEALTH_ENTRY_FLAG_SILENCED : 0) |
         (rrdcalc_isrepeating(rc) ? HEALTH_ENTRY_FLAG_IS_REPEATING : 0)));

    health_log_alert(host, ae);
    ae->last_repeat = rc->last_repeat;
    if (!(rc->run_flags & RRDCALC_FLAG_RUN_ONCE) && rc->status == RRDCALC_STATUS_CLEAR) {
        ae->flags |= HEALTH_ENTRY_RUN_ONCE;
    }
    rc->run_flags |= RRDCALC_FLAG_RUN_ONCE;
    health_send_notification(host, ae, hrm);
    netdata_log_debug(D_HEALTH, "Notification sent for the repeating alarm %u.", ae->alarm_id);
    health_alarm_wait_for_execution(ae);
    health_queue_ae_deletion(host, ae);
    worker_is_idle();
}

void do_rc_status_change(RRDHOST *host, RRDCALC *rc, RRDCALC_STATUS status, time_t now)
{
    worker_is_busy(UV_EVENT_HEALTH_JOB_ALARM_LOG_ENTRY);
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

    delay = (status > rc->status) ? rc->delay_up_current : rc->delay_down_current;

    // COMMENTED: because we do need to send raising alarms
    // if (now + delay < rc->delay_up_to_timestamp)
    //      delay = (int)(rc->delay_up_to_timestamp - now);

    rc->delay_last = delay;
    rc->delay_up_to_timestamp = now + delay;

    ALARM_ENTRY *ae = health_create_alarm_entry(
        host,
        rc,
        now,
        now - rc->last_status_change,
        rc->old_value,
        rc->value,
        rc->status,
        status,
        rc->delay_last,
        (((rc->config.alert_action_options & ALERT_ACTION_OPTION_NO_CLEAR_NOTIFICATION) ?
              HEALTH_ENTRY_FLAG_NO_CLEAR_NOTIFICATION :
              0) |
         ((rc->run_flags & RRDCALC_FLAG_SILENCED) ? HEALTH_ENTRY_FLAG_SILENCED : 0) |
         (rrdcalc_isrepeating(rc) ? HEALTH_ENTRY_FLAG_IS_REPEATING : 0)));

    health_log_alert(host, ae);
    health_alarm_log_add_entry(host, ae);

    nd_log_daemon(
        NDLP_DEBUG,
        "[%s]: Alert event for [%s.%s], value [%s], status [%s].",
        rrdhost_hostname(host),
        ae_chart_id(ae),
        ae_name(ae),
        ae_new_value_string(ae),
        rrdcalc_status2string(ae->new_status));

    rc->last_status_change_value = rc->value;
    rc->last_status_change = now;
    rc->old_status = rc->status;
    rc->status = status;

    if (unlikely(rrdcalc_isrepeating(rc))) {
        rc->last_repeat = now;
        if (rc->status == RRDCALC_STATUS_CLEAR)
            rc->run_flags |= RRDCALC_FLAG_RUN_ONCE;
    }
}

static RRDCALC_STATUS decide_alert_status(RRDCALC_STATUS warning_status, RRDCALC_STATUS critical_status)
{
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

    return status;
}

static void create_removed_event_for_rc(RRDHOST *host, RRDCALC *rc, time_t now)
{
    // create an alert removed event if the chart is obsolete and
    // has stopped being collected for 60 seconds
    if (unlikely(
            rc->rrdset && rc->status != RRDCALC_STATUS_REMOVED && rrdset_flag_check(rc->rrdset, RRDSET_FLAG_OBSOLETE) &&
            now > (rc->rrdset->last_collected_time.tv_sec + 60))) {
        if (!rrdcalc_isrepeating(rc)) {
            worker_is_busy(UV_EVENT_HEALTH_JOB_ALARM_LOG_ENTRY);
            time_t now_tmp = now_realtime_sec();

            ALARM_ENTRY *ae = health_create_alarm_entry(
                host,
                rc,
                now_tmp,
                now_tmp - rc->last_status_change,
                rc->value,
                NAN,
                rc->status,
                RRDCALC_STATUS_REMOVED,
                0,
                rrdcalc_isrepeating(rc) ? HEALTH_ENTRY_FLAG_IS_REPEATING : 0);

            health_log_alert(host, ae);
            health_alarm_log_add_entry(host, ae);

            rc->old_status = rc->status;
            rc->status = RRDCALC_STATUS_REMOVED;
            rc->last_status_change = now_tmp;
            rc->last_status_change_value = rc->value;
            rc->last_updated = now_tmp;
            rc->value = NAN;
        }
    }
}

static void health_database_lookup_for_rc(RRDHOST *host __maybe_unused, RRDCALC *rc)
{
    worker_is_busy(UV_EVENT_HEALTH_JOB_DB_QUERY);

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
            snprintfz(group_options_buf, sizeof(group_options_buf) - 1,
                      NETDATA_DOUBLE_FORMAT_AUTO,
                      rc->config.time_group_value);
            break;

        case RRDR_GROUPING_COUNTIF:
            snprintfz(group_options_buf, sizeof(group_options_buf) - 1,
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

    if (unlikely(ret != HTTP_RESP_OK)) {
        // database lookup failed
        rc->value = NAN;
        rc->run_flags |= RRDCALC_FLAG_DB_ERROR;

        netdata_log_debug(D_HEALTH, "Health on host '%s', alarm '%s.%s': database lookup returned error %d",
                          rrdhost_hostname(host), rrdcalc_chart_name(rc), rrdcalc_name(rc), ret);
    } else
        rc->run_flags &= ~RRDCALC_FLAG_DB_ERROR;

    if (unlikely(value_is_null)) {
        // collected value is null
        rc->value = NAN;
        rc->run_flags |= RRDCALC_FLAG_DB_NAN;

        netdata_log_debug(D_HEALTH,
                          "Health on host '%s', alarm '%s.%s': database lookup returned empty value (possibly value is not collected yet)",
                          rrdhost_hostname(host), rrdcalc_chart_name(rc), rrdcalc_name(rc));
    } else
        rc->run_flags &= ~RRDCALC_FLAG_DB_NAN;

    netdata_log_debug(D_HEALTH, "Health on host '%s', alarm '%s.%s': database lookup gave value " NETDATA_DOUBLE_FORMAT,
                      rrdhost_hostname(host), rrdcalc_chart_name(rc), rrdcalc_name(rc), rc->value);
    worker_is_idle();
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


static void health_execute_delayed_initializations(RRDHOST *host) {
    RRDSET *st;

    if (!rrdhost_flag_check(host, RRDHOST_FLAG_PENDING_HEALTH_INITIALIZATION)) return;
    rrdhost_flag_clear(host, RRDHOST_FLAG_PENDING_HEALTH_INITIALIZATION);

    worker_is_busy(UV_EVENT_HEALTH_JOB_DELAYED_INIT_RRDSET);

    rrdset_foreach_reentrant(st, host) {
        if(!rrdset_flag_check(st, RRDSET_FLAG_PENDING_HEALTH_INITIALIZATION)) continue;
        rrdset_flag_clear(st, RRDSET_FLAG_PENDING_HEALTH_INITIALIZATION);

        health_prototype_alerts_for_rrdset_incrementally(st);
    }
    rrdset_foreach_done(st);

    worker_is_idle();
}

static void health_initialize_rrdhost(RRDHOST *host) {

    if (!host->health.enabled || rrdhost_flag_check(host, RRDHOST_FLAG_INITIALIZED_HEALTH))
        return;

    host->health_log.max = health_globals.config.health_log_entries_max;
    host->health_log.health_log_retention_s = health_globals.config.health_log_retention_s;
    host->health.default_exec = string_dup(health_globals.config.default_exec);
    host->health.default_recipient = string_dup(health_globals.config.default_recipient);
    host->health.use_summary_for_notifications = health_globals.config.use_summary_for_notifications;

    host->health_log.next_log_id = get_uint32_id();
    host->health_log.next_alarm_id = 0;

    sql_health_alarm_log_load(host);
    rw_spinlock_init(&host->health_log.spinlock);
    rrdhost_flag_set(host, RRDHOST_FLAG_INITIALIZED_HEALTH);

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
        worker_is_idle();
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

    worker_is_idle();
}

static void process_repeating_alarms(RRDHOST *host, time_t now, struct health_raised_summary *hrm)
{
    RRDCALC *rc;

    foreach_rrdcalc_in_rrdhost_read(host, rc) {

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

        if (unlikely(repeat_every > 0 && (rc->last_repeat + repeat_every) <= now))
            perform_repeated_alarm(host, rc, hrm, now);
    }
    foreach_rrdcalc_in_rrdhost_done(rc);
}

// returns the number of runnable alerts
static void health_event_loop_for_host(RRDHOST *host, time_t now, time_t *next_run)
{
    size_t runnable = 0;

    if(unlikely(!rrdhost_should_run_health(host)))
        return;

    rrdhost_set_health_evloop_iteration(host);

    if (unlikely(
            !rrdhost_flag_check(host, RRDHOST_FLAG_INITIALIZED_HEALTH) ||
            rrdhost_flag_check(host, RRDHOST_FLAG_PENDING_HEALTH_INITIALIZATION))) {
        // Dont run again, initialization will reschedule us
        *next_run = -1;
        return;
    }

    // wait until cleanup of obsolete charts on children is complete
    if (host != localhost) {
        if (unlikely(host->stream.rcv.status.check_obsolete)) {
            nd_log_daemon(NDLP_DEBUG, "[%s]: Waiting for chart obsoletion check.", rrdhost_hostname(host));
            return;
        }
    }

    worker_is_busy(UV_EVENT_HEALTH_JOB_HOST_LOCK);
    {
        struct aclk_sync_cfg_t *aclk_host_config = __atomic_load_n(&host->aclk_host_config, __ATOMIC_RELAXED);
        if (aclk_host_config && aclk_host_config->send_snapshot == 2)
            return;
    }

    // the first loop is to lookup values from the db
    RRDCALC *rc;
    foreach_rrdcalc_in_rrdhost_read(host, rc) {

        rrdcalc_update_info_using_rrdset_labels(rc);

        if (health_silencers_update_disabled_silenced(host, rc))
            continue;

        // Create REMOVED event if needed
        create_removed_event_for_rc(host, rc, now);

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

        if (unlikely(RRDCALC_HAS_DB_LOOKUP(rc)))
            health_database_lookup_for_rc(host, rc);

        // ------------------------------------------------------------
        // if there is calculation expression, run it
        do_eval_expression(rc, rc->config.calculation, "calculation", UV_EVENT_HEALTH_JOB_CALC_EVAL, RRDCALC_FLAG_CALC_ERROR, NULL, &rc->value);
    }
    foreach_rrdcalc_in_rrdhost_done(rc);

    struct health_raised_summary *hrm = NULL;

    if (unlikely(runnable)) {
        foreach_rrdcalc_in_rrdhost_read(host, rc) {

            if (unlikely(!(rc->run_flags & RRDCALC_FLAG_RUNNABLE) || rc->run_flags & RRDCALC_FLAG_DISABLED))
                continue;

            RRDCALC_STATUS warning_status = RRDCALC_STATUS_UNDEFINED;
            RRDCALC_STATUS critical_status = RRDCALC_STATUS_UNDEFINED;

            do_eval_expression(rc, rc->config.warning, "warning", UV_EVENT_HEALTH_JOB_WARNING_EVAL, RRDCALC_FLAG_WARN_ERROR, &warning_status, NULL);
            do_eval_expression(rc, rc->config.critical, "critical", UV_EVENT_HEALTH_JOB_CRITICAL_EVAL, RRDCALC_FLAG_CRIT_ERROR, &critical_status, NULL);

            // --------------------------------------------------------
            // decide the final alert status
            RRDCALC_STATUS status = decide_alert_status(warning_status, critical_status);

            // --------------------------------------------------------
            // check if the new status and the old differ

            if (status != rc->status) {
                do_rc_status_change(host, rc, status, now);
            }

            rc->last_updated = now;
            rc->next_update = now + rc->config.update_every;

            *next_run = MIN(*next_run, rc->next_update);
        }
        foreach_rrdcalc_in_rrdhost_done(rc);

        hrm = alerts_raised_summary_create(host);
        alerts_raised_summary_populate(hrm);

        // process repeating alarms
        process_repeating_alarms(host, now, hrm);
    }

    // execute notifications
    // and cleanup

    if (hrm) {
        worker_is_busy(UV_EVENT_HEALTH_JOB_ALARM_LOG_PROCESS);
        health_alarm_log_process_to_send_notifications(host, hrm);
        worker_is_idle();
        alerts_raised_summary_free(hrm);
    }

    // Store all transitions
    Word_t Index = 0;
    bool first = true;
    Pvoid_t *Pvalue;
    while ((Pvalue = JudyLFirstThenNext(host->health.JudyL_ae, &Index, &first))) {
        ALARM_ENTRY *ae = *Pvalue;
        sql_health_alarm_log_save(host, ae);
    }
    (void) JudyLFreeArray(&host->health.JudyL_ae, PJE0);

    // Delete AE as needed
    Index = 0;
    first = true;
    while ((Pvalue = JudyLFirstThenNext(host->health.JudyL_del_ae, &Index, &first))) {
        ALARM_ENTRY *ae = *Pvalue;
        health_alarm_log_free_one_nochecks_nounlink(ae);
    }
    (void)JudyLFreeArray(&host->health.JudyL_del_ae, PJE0);

    struct aclk_sync_cfg_t *aclk_host_config = __atomic_load_n(&host->aclk_host_config, __ATOMIC_RELAXED);
    if (aclk_host_config && aclk_host_config->send_snapshot == 1) {
        aclk_host_config->send_snapshot = 2;
        rrdhost_flag_set(host, RRDHOST_FLAG_ACLK_STREAM_ALERTS);
    } else {
        worker_is_busy(UV_EVENT_HEALTH_JOB_ALARM_LOG_QUEUE);

        if (process_alert_pending_queue(host))
            rrdhost_flag_set(host, RRDHOST_FLAG_ACLK_STREAM_ALERTS);
    }

    worker_is_idle();
}

// UV health event loop

enum health_opcode {
    HEALTH_NOOP = 0,
    HEALTH_HOST_INIT,
    HEALTH_HOST_RUN,
    HEALTH_HOST_RUN_LATER,
    HEALTH_HOST_REGISTER,
    HEALTH_HOST_UNREGISTER,
    HEALTH_RUN_JOBS,
    HEALTH_HOST_CLEANUP,
    HEALTH_HOST_MAINTENANCE,
    HEALTH_PAUSE,
    HEALTH_RESUME,
    HEALTH_SHUTDOWN,

    // leave this last
    HEALTH_MAX_ENUMERATIONS_DEFINED
};

struct health_cmd {
    enum health_opcode opcode;
    void *param[2];
    struct health_cmd *prev, *next;
};

typedef enum health_job_type_t {
    HEALTH_JOB_HOST_RUN,
    HEALTH_JOB_HOST_INIT,
    HEALTH_JOB_HOST_MAINT,
    HEALTH_JOB_HOST_CALC_CLEANUP,
    //
    HEALTH_JOB_MAX,
} health_job_type_t;

struct health_config_s {
    uv_thread_t thread;
    uv_loop_t loop;
    uv_timer_t timer_req;
    uv_timer_t timer_ae;
    uv_async_t async;
    bool paused;
    SPINLOCK cmd_queue_lock;
    struct health_cmd *cmd_base;
    struct job_list_t *job_list[HEALTH_JOB_MAX];
    ARAL *ar;
} health_config_s = { 0 };

static struct health_cmd health_deq_cmd(void)
{
    struct health_cmd ret = { 0 };
    struct health_cmd *to_free = NULL;

    spinlock_lock(&health_config_s.cmd_queue_lock);
    if(health_config_s.cmd_base) {
        struct health_cmd *t = health_config_s.cmd_base;
        DOUBLE_LINKED_LIST_REMOVE_ITEM_UNSAFE(health_config_s.cmd_base, t, prev, next);
        ret = *t;
        to_free = t;
    }
    else {
        ret.opcode = HEALTH_NOOP;
    }
    spinlock_unlock(&health_config_s.cmd_queue_lock);
    aral_freez(health_config_s.ar, to_free);

    return ret;
}

static void health_enq_cmd(struct health_cmd *cmd)
{
    struct health_cmd *t = aral_mallocz(health_config_s.ar);
    *t = *cmd;
    t->prev = t->next = NULL;

    spinlock_lock(&health_config_s.cmd_queue_lock);
    DOUBLE_LINKED_LIST_APPEND_ITEM_UNSAFE(health_config_s.cmd_base, t, prev, next);
    spinlock_unlock(&health_config_s.cmd_queue_lock);

    (void) uv_async_send(&health_config_s.async);
}

struct job_list_t {
    health_job_type_t job_type;
    int pending;
    int running;
    int max_threads;
    Pvoid_t JudyL;
    Word_t count;
};

typedef struct worker_data {
    uv_work_t request;
    void *payload;
    time_t next_run;
    health_job_type_t job_type;
    struct health_config_s *config;
} worker_data_t;

typedef struct {
    worker_data_t workers[MAX_WORKER_DATA];  // Preallocated worker data pool
    int free_stack[MAX_WORKER_DATA];  // Stack of available worker data indices
    int top;  // Stack pointer
} WorkerPool;

WorkerPool worker_pool;

// Initialize the worker pool
void init_worker_pool(WorkerPool *pool) {
    for (int i = 0; i < MAX_WORKER_DATA; i++) {
        pool->free_stack[i] = i;  // Fill the stack with indices
    }
    pool->top = MAX_WORKER_DATA;  // All workers are initially free
}

// Get a worker (reuse if available, NULL if pool exhausted)
worker_data_t *get_worker(WorkerPool *pool) {
    if (pool->top == 0) {
        return NULL;  // Pool exhausted
    }
    int index = pool->free_stack[--pool->top];  // Pop from stack
    return &pool->workers[index];
}

// Return a worker for reuse
void return_worker(WorkerPool *pool, worker_data_t *worker) {
    int index = worker - pool->workers;  // Calculate index
    if (index < 0 || index >= MAX_WORKER_DATA) {
        return;  // Invalid worker (should not happen)
    }
    pool->free_stack[pool->top++] = index;  // Push index back to stack
}


static void async_cb(uv_async_t *handle)
{
    uv_stop(handle->loop);
    uv_update_time(handle->loop);
}

static void timer_cb(uv_timer_t *handle)
{
    uv_stop(handle->loop);
    uv_update_time(handle->loop);
    health_run_jobs();
}

static void after_host_rrdcalc_cleanup_job(uv_work_t *req, int status __maybe_unused)
{
    worker_data_t *data = req->data;
    RRDHOST *host = data->payload;
    HEALTH *host_health = &host->health;
    struct health_config_s *config = data->config;
    config->job_list[data->job_type]->running--;
    host_health->rrdcalc_cleanup_running = false;
    host_health->job_running = false;
    (void) uv_timer_stop(&host_health->timer);
    return_worker(&worker_pool, data);
}

static void host_rrdcalc_cleanup_job(uv_work_t *req)
{
    register_libuv_worker_jobs();
    worker_data_t *data = req->data;
    RRDHOST *host = data->payload;
    worker_is_busy(UV_EVENT_HOST_CALC_CLEANUP);

    rrdcalc_child_disconnected(host);

    worker_is_idle();
}

static void after_host_health_maintenance_job(uv_work_t *req, int status __maybe_unused)
{
    worker_data_t *data = req->data;
    struct health_config_s *config = data->config;
    config->job_list[data->job_type]->running--;
    RRDHOST *host = data->payload;
    return_worker(&worker_pool, data);
    HEALTH *host_health = &host->health;
    host_health->job_running = false;
    health_host_run(host);
}

static void host_health_maintenance_job(uv_work_t *req)
{
    register_libuv_worker_jobs();
    worker_data_t *data = req->data;
    RRDHOST *host = data->payload;
    worker_is_busy(UV_EVENT_HEALTH_LOG_CLEANUP);

    sql_health_alarm_log_cleanup(host);

//    (void) db_execute(db_health,"DELETE FROM health_log WHERE host_id NOT IN (SELECT host_id FROM host)");
//    (void) db_execute(db_health,"DELETE FROM health_log_detail WHERE health_log_id NOT IN (SELECT health_log_id FROM health_log)");
//    (void) db_execute(db_aclk,"DELETE FROM alert_version WHERE health_log_id NOT IN (SELECT health_log_id FROM health_log)");

    worker_is_idle();
}

static void after_host_initialize_alerts_job(uv_work_t *req, int status __maybe_unused)
{
    worker_data_t *data = req->data;
    struct health_config_s *config = data->config;
    config->job_list[data->job_type]->running--;
    RRDHOST *host = data->payload;
    HEALTH *host_health = &host->health;
    host_health->job_running = false;
    health_host_run(host);
//    if (config->job_list[data->job_type]->pending)
//        health_host_initialize(NULL);
    return_worker(&worker_pool, data);
}

static void host_initialize_alerts_job(uv_work_t *req)
{
    register_libuv_worker_jobs();

    worker_is_busy(UV_EVENT_HOST_HEALTH_INIT);
    worker_data_t *data =  req->data;
    RRDHOST *host = data->payload;

    usec_t start_ut = now_realtime_usec();
    health_initialize_rrdhost(host);
    health_execute_delayed_initializations(host);
    usec_t end_ut = now_realtime_usec();
    COMPUTE_DURATION(report_duration, "us", start_ut, end_ut);
    netdata_log_debug(D_HEALTH, "Alerts initialized for \"%s\" in %s", rrdhost_hostname(host), report_duration);
    worker_is_idle();
}

static void after_host_evaluate_alerts_job(uv_work_t *req, int status __maybe_unused)
{
    worker_data_t *data = req->data;
    struct health_config_s *config = data->config;
    config->job_list[data->job_type]->running--;

    RRDHOST *host = data->payload;
    HEALTH *host_health = &host->health;
    host_health->job_running = false;

    time_t next_run = data->next_run;
    return_worker(&worker_pool, data);

    if (host_health->rrdcalc_cleanup_running) {
        host_health->rrdcalc_cleanup_running = false;
        health_host_cleanup(host);
        return;
    }

    // initialization needed?
    if (next_run == -1) {
        health_host_initialize(host);
        return;
    }

    time_t now = now_realtime_sec();
    // Lets see if we need to do maintenace
    if (now - host->health.last_maintenance > HEALTH_HOST_MAINTENANCE_INTERVAL) {
        host->health.last_maintenance = now;
        health_host_maintenance(host);
        return;
    }

    int64_t delay = next_run - now_realtime_sec();

    int rc = uv_timer_start(
        &host_health->timer,
        host_health_timer_cb,
        delay > 0 ? delay * MSEC_PER_SEC : 0,
        health_globals.config.run_at_least_every_seconds * MSEC_PER_SEC);

    if (rc) {
        if (delay <= 0)
            health_host_run(host);
        else
            health_host_run_later(host, delay * MSEC_PER_SEC);
    }
}

static void host_evaluate_alerts_job(uv_work_t *req)
{
    register_libuv_worker_jobs();

    worker_data_t *data = req->data;
    RRDHOST *host = data->payload;
    HEALTH *host_health = &host->health;
    struct health_config_s *config = data->config;

    usec_t start_ut = now_realtime_usec();
    time_t now = start_ut / USEC_PER_SEC;

    time_t delay_up_to = (host->health.delay_up_to && host->health.delay_up_to > now) ? host->health.delay_up_to : 0;

    if (host->health.apply_hibernation_delay) {
        host->health.apply_hibernation_delay = false;
        nd_log_daemon(NDLP_DEBUG,
                      "[%s]: Postponing health checks for %" PRId32 " seconds.",
                      rrdhost_hostname(host),
                      health_globals.config.postpone_alarms_during_hibernation_for_seconds);
        data->next_run = now + health_globals.config.postpone_alarms_during_hibernation_for_seconds;
        data->next_run = MAX(data->next_run, delay_up_to);
        return;
    }

    if (delay_up_to) {
        data->next_run = delay_up_to;
        return;
    }

    host->health.delay_up_to = 0;

    data->next_run = (start_ut / USEC_PER_SEC) + health_globals.config.run_at_least_every_seconds;

    if (config->paused) {
        nd_log_daemon(NDLP_INFO, "HEALTH: Health checks are paused for %s", rrdhost_hostname(host));
        return;
        }

    // Just reschedule
    if (!stream_control_health_should_be_running()) {
        nd_log_daemon(NDLP_INFO, "HEALTH: Health checks are paused for %s", rrdhost_hostname(host));
        return;
    }

    if (unlikely(silencers->all_alarms && silencers->stype == STYPE_DISABLE_ALARMS))
        return;

    worker_is_busy(UV_EVENT_HOST_HEALTH_RUN);
    health_event_loop_for_host(host, now_realtime_sec(), &data->next_run);

    host_health->last_runtime  = now_realtime_usec() - start_ut;
    COMPUTE_DURATION(report_duration, "us", 0, host_health->last_runtime);
    netdata_log_debug(D_HEALTH, "Alerts evaluated for \"%s\" in %s", rrdhost_hostname(host), report_duration);
    worker_is_idle();
}

struct {
    uv_work_cb work_cb;
    uv_after_work_cb after_work_cb;
} job_functions[HEALTH_JOB_MAX] = {
    [HEALTH_JOB_HOST_INIT] = {host_initialize_alerts_job, after_host_initialize_alerts_job},
    [HEALTH_JOB_HOST_RUN] = {host_evaluate_alerts_job, after_host_evaluate_alerts_job},
    [HEALTH_JOB_HOST_MAINT] = {host_health_maintenance_job, after_host_health_maintenance_job},
    [HEALTH_JOB_HOST_CALC_CLEANUP] = {host_rrdcalc_cleanup_job, after_host_rrdcalc_cleanup_job},
};

// Return true if submitted to worker
static bool send_job_to_worker(struct health_config_s *config, struct job_list_t *job, RRDHOST *host)
{
    HEALTH *host_health = &host->health;
    if (host_health->job_running) {
        nd_log_daemon(NDLP_INFO, "HEALTH: Job already running for %s", rrdhost_hostname(host));
        return false;
    }

    worker_data_t *data = get_worker(&worker_pool);
    if (!data)
        return false;

    data->request.data = data;
    data->config = config;
    data->payload = host;
    data->job_type = job->job_type;
    job->running++;

    host_health->job_running = true;
    nd_log_daemon(NDLP_INFO, "HEALTH: Running job %u for %s", job->job_type, rrdhost_hostname(host));
    internal_fatal(job->job_type >= HEALTH_JOB_MAX, "Invalid job type %d", job->job_type);
    int rc = uv_queue_work(&config->loop, &data->request, job_functions[job->job_type].work_cb, job_functions[job->job_type].after_work_cb);
    if (rc) {
        job->running--;
        return_worker(&worker_pool, data);
    }
    return (rc == 0);
}

static void add_job(struct job_list_t *job, RRDHOST *host)
{
    Pvoid_t *Pvalue = JudyLIns(&job->JudyL, ++job->count, PJE0);
    if (Pvalue != PJERR) {
        *Pvalue = host;
        job->pending++;
    } else
        nd_log_daemon(NDLP_ERR, "Failed to add job");
}

static Pvoid_t *get_job(struct job_list_t *job, Word_t *Index)
{
    Pvoid_t *Pvalue = JudyLFirst(job->JudyL, Index, PJE0);
    return Pvalue;
}

static void del_job(struct job_list_t *job, Word_t Index)
{
    job->pending--;
    (void)JudyLDel(&job->JudyL, Index, PJE0);
}

static void schedule_job_to_run(struct health_config_s *config, health_job_type_t job_type, RRDHOST *host)
{
    Pvoid_t *Pvalue;
    RRDHOST *host_in_queue;

    struct job_list_t *job = config->job_list[job_type];
    int max_threads = job->max_threads;
    bool too_busy = (job->running >= max_threads);

    // If we are busy and it's just a ping to run, leave
    if (too_busy && !host)
        return;

    // if we are busy (we have a job) store it and leave
    if (too_busy) {
        add_job(job, host);
        return;
    }

    // Here: we are not busy
    // If we have health job to run for a host
    // if we dont, it was a ping from the callback

    // Lets try to queue as many of the pending jobs
    bool submitted = true;
    int loop = max_threads - job->running;
    while (submitted && loop-- > 0 && job->pending && job->running < max_threads) {
        Word_t Index = 0;
        Pvalue = get_job(job, &Index);
        if (Pvalue == NULL)
            break;
        host_in_queue = *Pvalue;
        del_job(job, Index);

        // Send it to worker, increase running
        submitted = send_job_to_worker(config, job, host_in_queue);
        // if it was scheduled in worker, remove it from pending
        if (!submitted)
            add_job(job, host_in_queue);
    }

    // Was it just a ping to run? leave
    if (!host)
        return;

    too_busy = (job->pending > 0 || job->running >= max_threads);
    // We have a host, if not busy lets run it
    if (!too_busy)
        submitted = send_job_to_worker(config, job, host);

    // We were either busy, or failed to start worker, schedule for later
    if (too_busy || !submitted)
        add_job(job, host);
}

// HEALTH CLEANUP

static void close_callback(uv_handle_t *handle, void *data __maybe_unused)
{
    if (handle->type == UV_TIMER) {
        uv_timer_stop((uv_timer_t *)handle);
    }
    uv_close(handle, NULL);  // Automatically close and free the handle
}

static void host_health_timer_cb(uv_timer_t *handle)
{
    RRDHOST *host = handle->data;
    struct health_config_s *config = handle->loop->data;
    // Queue command to run health
    HEALTH *host_health = &host->health;
    if (host_health->job_running) {
        nd_log_daemon(NDLP_INFO, "HEALTH: Job already running for %s", rrdhost_hostname(host));
        return;
    }
    schedule_job_to_run(config, HEALTH_JOB_HOST_RUN, host);
}

#define MAX_HEALTH_BATCH_COMMANDS (16)

#define TIMER_INITIAL_PERIOD_MS (2000)
#define TIMER_REPEAT_PERIOD_MS (2000)

static void health_ev_loop(void *arg)
{
    struct health_config_s *config = arg;
    uv_thread_set_name_np("HEALTH");

    config->ar = aral_by_size_acquire(sizeof(struct health_cmd));

    worker_register("HEALTH");

    service_register(SERVICE_THREAD_TYPE_EVENT_LOOP, NULL, NULL, NULL, true);

    worker_register_job_name(HEALTH_NOOP,  "noop");
    worker_register_job_name(HEALTH_HOST_REGISTER, "host health register");
    worker_register_job_name(HEALTH_HOST_UNREGISTER, "host health unregister");
    worker_register_job_name(HEALTH_HOST_RUN,  "host health evaluate");
    worker_register_job_name(HEALTH_HOST_RUN_LATER,  "host health evaluate");
    worker_register_job_name(HEALTH_HOST_INIT, "host health init");
    worker_register_job_name(HEALTH_RUN_JOBS, "host health run jobs");
    worker_register_job_name(HEALTH_PAUSE, "health paused");
    worker_register_job_name(HEALTH_RESUME, "health resumed");

    uv_loop_t *loop = &config->loop;
    loop->data = config;
    fatal_assert(0 == uv_loop_init(loop));
    fatal_assert(0 == uv_async_init(loop, &config->async, async_cb));

    fatal_assert(0 == uv_timer_init(loop, &config->timer_req));
    config->timer_req.data = config;

    fatal_assert(0 == uv_timer_start(&config->timer_req, timer_cb, TIMER_INITIAL_PERIOD_MS, TIMER_REPEAT_PERIOD_MS));

    int max_thread_count = netdata_conf_health_threads();
    int maint_max_thread_count = (max_thread_count * 25 / 100);
    if (maint_max_thread_count < 1)
        maint_max_thread_count = 1;
    netdata_log_info("Starting health with %d threads for alert evaluations and 3x%d threads for other tasks",
                     max_thread_count, maint_max_thread_count);

    unsigned cmd_batch_size;
    RRDHOST *host;

    for (int i = 0; i < HEALTH_JOB_MAX; i++) {
        config->job_list[i] = callocz(1, sizeof(struct job_list_t));
        config->job_list[i]->job_type = i;
        config->job_list[i]->max_threads = (i == HEALTH_JOB_HOST_RUN) ? max_thread_count : maint_max_thread_count;
    }

    init_worker_pool(&worker_pool);
    health_register_host(localhost, localhost->health.delay_up_to);
    HEALTH *host_health;
    bool is_shutdown = false;
    uint64_t schedule_time;

    while (likely(false == is_shutdown)) {
        enum health_opcode opcode;
        worker_is_idle();
        uv_run(loop, UV_RUN_DEFAULT);

        /* wait for commands */
        cmd_batch_size = 0;
        do {
            if (unlikely(cmd_batch_size >= MAX_HEALTH_BATCH_COMMANDS))
                break;

            struct health_cmd cmd = health_deq_cmd();

            ++cmd_batch_size;
            opcode = cmd.opcode;

            if(likely(opcode != HEALTH_NOOP))
                worker_is_busy(opcode);

            switch (opcode) {
                case HEALTH_NOOP:
                    /* the command queue was empty, do nothing */
                    break;
                case HEALTH_HOST_REGISTER:
                    host = cmd.param[0];
                    schedule_time = (uint64_t)(uintptr_t)cmd.param[1];
                    host_health = &host->health;

                    if (!host_health->timer_initialized) {
                        int rc = uv_timer_init(loop, &host_health->timer);
                        if (!rc) {
                            host_health->timer_initialized = true;
                            host_health->timer.data = host;
                            host_health->timer.loop = loop;
                        }
                    }
                    if (host_health->timer_initialized) {
                        int rc = uv_timer_start(
                            &host_health->timer,
                            host_health_timer_cb,
                            schedule_time,
                            health_globals.config.run_at_least_every_seconds * MSEC_PER_SEC);
                        if (!rc) {
                            nd_log_daemon(NDLP_INFO, "Host \"%s\" is now registered for health monitoring", rrdhost_hostname(host));
                            break;
                        }
                    }
                    nd_log_daemon(
                        NDLP_ERR, "Failed to register host \"%s\" for health monitoring", rrdhost_hostname(host));
                    break;

                case HEALTH_HOST_UNREGISTER:
                    host = cmd.param[0];
                    bool rrdcalc_cleanup = (bool)(uintptr_t)cmd.param[1];
                    host_health = &host->health;

                    if (!host_health->timer_initialized)
                        break;

                    if (false == rrdcalc_cleanup) {
                        if (uv_is_active((uv_handle_t *)&host_health->timer)) {
                            uv_timer_stop(&host_health->timer);
                            netdata_log_debug(
                                D_HEALTH, "Host \"%s\" is now unregistered from health", rrdhost_hostname(host));
                            nd_log_daemon(NDLP_INFO, "Host \"%s\" is now unregistered from health without cleanup", rrdhost_hostname(host));
                        }
                        break;
                    }

                    host_health->rrdcalc_cleanup_running = true;
                    nd_log_daemon(NDLP_INFO, "Host \"%s\" is now unregistered from health -- cleanup will run", rrdhost_hostname(host));
                    break;

                case HEALTH_HOST_RUN_LATER:
                    host = cmd.param[0];
                    schedule_time = (uint64_t)(uintptr_t)cmd.param[1];
                    host_health = &host->health;

                    int rc = uv_timer_start(
                        &host_health->timer,
                        host_health_timer_cb,
                        schedule_time,
                        health_globals.config.run_at_least_every_seconds * MSEC_PER_SEC);
                    if (rc)
                        nd_log_daemon(NDLP_ERR, "Failed to schedule host \"%s\" for health monitoring", rrdhost_hostname(host));
                    break;

                case HEALTH_HOST_CLEANUP:
                    host = cmd.param[0];
                    nd_log_daemon(NDLP_INFO, "Host \"%s\" is now scheduled for cleanup", rrdhost_hostname(host));
                    schedule_job_to_run(config, HEALTH_JOB_HOST_CALC_CLEANUP, host);
                    break;

                case HEALTH_HOST_INIT:
                    host = cmd.param[0];
                    if (host) {
                        nd_log_daemon(NDLP_INFO, "Host \"%s\" is now scheduled for health initialization", rrdhost_hostname(host));
                        schedule_job_to_run(config, HEALTH_JOB_HOST_INIT, host);
                    }
                    else
                        schedule_job_to_run(config, HEALTH_JOB_HOST_INIT, NULL);
                    break;

                case HEALTH_HOST_RUN:
                    host = cmd.param[0];
                    nd_log_daemon(NDLP_INFO, "Host \"%s\" is now scheduled for health evaluation", rrdhost_hostname(host));
                    schedule_job_to_run(config, HEALTH_JOB_HOST_RUN, host);
                    break;


                case HEALTH_RUN_JOBS:
                    schedule_job_to_run(config, HEALTH_JOB_HOST_INIT, NULL);
                    schedule_job_to_run(config, HEALTH_JOB_HOST_RUN, NULL);
                    schedule_job_to_run(config, HEALTH_JOB_HOST_MAINT, NULL);
                    schedule_job_to_run(config, HEALTH_JOB_HOST_CALC_CLEANUP, NULL);
                    break;

                case HEALTH_HOST_MAINTENANCE:
                    host = cmd.param[0];
                    nd_log_daemon(NDLP_INFO, "Host \"%s\" is now scheduled for health maintenance", rrdhost_hostname(host));
                    schedule_job_to_run(config, HEALTH_JOB_HOST_MAINT, host);
                    break;

                case HEALTH_PAUSE:
                    config->paused = true;
                    break;

                case HEALTH_RESUME:
                    config->paused = false;
                    break;

                case HEALTH_SHUTDOWN:
                    is_shutdown = true;
                    break;
                default:
                    break;
            }
        } while (opcode != HEALTH_NOOP);
    }

    if (!uv_timer_stop(&config->timer_req))
        uv_close((uv_handle_t *)&config->timer_req, NULL);

    uv_close((uv_handle_t *)&config->async, NULL);
    uv_run(loop, UV_RUN_NOWAIT);

    uv_walk(loop, (uv_walk_cb) close_callback, NULL);
    uv_run(loop, UV_RUN_NOWAIT);

    (void) uv_loop_close(loop);

    for (int i = 0; i < HEALTH_JOB_MAX; i++)
        freez(config->job_list[i]);

    aral_by_size_release(config->ar);

    worker_unregister();
    service_exits();
    netdata_log_info("HEALTH: Shutdown completed");
}

static inline void queue_health_cmd(enum health_opcode opcode, const void *param0, const void *param1)
{
    struct health_cmd cmd;
    cmd.opcode = opcode;
    cmd.param[0] = (void *) param0;
    cmd.param[1] = (void *) param1;
    health_enq_cmd(&cmd);
}

// Public
// Run health for a host
void health_host_run(RRDHOST *host)
{
    queue_health_cmd(HEALTH_HOST_RUN, host, NULL);
}

void health_host_run_later(RRDHOST *host, uint64_t delay)
{
    queue_health_cmd(HEALTH_HOST_RUN_LATER, host, (void *)(uintptr_t)delay);
}

void health_host_initialize(RRDHOST *host)
{
    queue_health_cmd(HEALTH_HOST_INIT, host, NULL);
}

void health_event_loop_init(void)
{
    memset(&health_config_s, 0, sizeof(health_config_s));
    fatal_assert(0 == uv_thread_create(&health_config_s.thread, health_ev_loop, &health_config_s));
}

void health_register_host(RRDHOST *host, time_t run_at)
{
    netdata_log_debug(D_HEALTH, "Host \"%s\" is registered for health monitoring", rrdhost_hostname(host));
    host->health.apply_hibernation_delay = check_if_resumed_from_suspension();
    uint64_t delay = run_at ? run_at - now_realtime_sec() : 0;
    if (delay <= 0)
        delay = 0;
    else
        delay *= USEC_PER_MS;
    queue_health_cmd(HEALTH_HOST_REGISTER, host, (void *)(uintptr_t)delay);
}

void health_unregister_host(RRDHOST *host, bool rrdcalc_cleanup)
{
    // This should run a cleanup for the host
    queue_health_cmd(HEALTH_HOST_UNREGISTER, host, (void *)(uintptr_t)rrdcalc_cleanup);
}

void health_host_maintenance(RRDHOST *host)
{
    queue_health_cmd(HEALTH_HOST_MAINTENANCE, host, NULL);
}

void health_run_jobs()
{
    queue_health_cmd(HEALTH_RUN_JOBS, NULL, NULL);
}

void health_host_cleanup(RRDHOST *host)
{
    queue_health_cmd(HEALTH_HOST_CLEANUP, host, NULL);
}

void health_pause()
{
    queue_health_cmd(HEALTH_PAUSE, NULL, NULL);
}

void health_resume()
{
    queue_health_cmd(HEALTH_RESUME, NULL, NULL);
}

void health_shutdown()
{
    queue_health_cmd(HEALTH_SHUTDOWN, NULL, NULL);
}

void health_schedule_ae_save(RRDHOST *host, ALARM_ENTRY *ae)
{
    Pvoid_t *Pvalue = JudyLIns(&host->health.JudyL_ae, ++host->health.count, PJE0);
    if (Pvalue)
        *Pvalue = (void *)ae;
}

void health_queue_ae_deletion(RRDHOST *host, ALARM_ENTRY *ae)
{
    Pvoid_t *Pvalue = JudyLIns(&host->health.JudyL_del_ae, ++host->health.delete_count, PJE0);
    if (Pvalue)
        *Pvalue = (void *)ae;
}
