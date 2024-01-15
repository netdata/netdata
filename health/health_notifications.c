// SPDX-License-Identifier: GPL-3.0-or-later

#include "health_internals.h"

#define ACTIVE_ALARMS_LIST_EXAMINE 500
#define ACTIVE_ALARMS_LIST 15

// the queue of executed alarm notifications that haven't been waited for yet
static struct {
    ALARM_ENTRY *head; // oldest
    ALARM_ENTRY *tail; // latest
} alarm_notifications_in_progress = {NULL, NULL};

typedef struct active_alerts {
    char *name;
    time_t last_status_change;
    RRDCALC_STATUS status;
} active_alerts_t;

static inline int compare_active_alerts(const void * a, const void * b) {
    active_alerts_t *active_alerts_a = (active_alerts_t *)a;
    active_alerts_t *active_alerts_b = (active_alerts_t *)b;

    return (int) ( active_alerts_b->last_status_change - active_alerts_a->last_status_change );
}

void health_alarm_wait_for_execution(ALARM_ENTRY *ae) {
    if (!(ae->flags & HEALTH_ENTRY_FLAG_EXEC_IN_PROGRESS))
        return;

    spawn_wait_cmd(ae->exec_spawn_serial, &ae->exec_code, &ae->exec_run_timestamp);
    netdata_log_debug(D_HEALTH, "done executing command - returned with code %d", ae->exec_code);
    ae->flags &= ~HEALTH_ENTRY_FLAG_EXEC_IN_PROGRESS;

    if(ae->exec_code != 0)
        ae->flags |= HEALTH_ENTRY_FLAG_EXEC_FAILED;

    unlink_alarm_notify_in_progress(ae);
}

void wait_for_all_notifications_to_finish_before_allowing_health_to_be_cleaned_up(void) {
    ALARM_ENTRY *ae;
    while (NULL != (ae = alarm_notifications_in_progress.head)) {
        if(unlikely(!service_running(SERVICE_HEALTH)))
            break;

        health_alarm_wait_for_execution(ae);
    }
}

void unlink_alarm_notify_in_progress(ALARM_ENTRY *ae)
{
    struct alarm_entry *prev = ae->prev_in_progress;
    struct alarm_entry *next = ae->next_in_progress;

    if (NULL != prev) {
        prev->next_in_progress = next;
    }
    if (NULL != next) {
        next->prev_in_progress = prev;
    }
    if (ae == alarm_notifications_in_progress.head) {
        alarm_notifications_in_progress.head = next;
    }
    if (ae == alarm_notifications_in_progress.tail) {
        alarm_notifications_in_progress.tail = prev;
    }
}

static inline void enqueue_alarm_notify_in_progress(ALARM_ENTRY *ae)
{
    ae->prev_in_progress = NULL;
    ae->next_in_progress = NULL;

    if (NULL != alarm_notifications_in_progress.tail) {
        ae->prev_in_progress = alarm_notifications_in_progress.tail;
        alarm_notifications_in_progress.tail->next_in_progress = ae;
    }
    if (NULL == alarm_notifications_in_progress.head) {
        alarm_notifications_in_progress.head = ae;
    }
    alarm_notifications_in_progress.tail = ae;

}

static bool prepare_command(BUFFER *wb,
                            const char *exec,
                            const char *recipient,
                            const char *registry_hostname,
                            uint32_t unique_id,
                            uint32_t alarm_id,
                            uint32_t alarm_event_id,
                            uint32_t when,
                            const char *alert_name,
                            const char *alert_chart_name,
                            const char *new_status,
                            const char *old_status,
                            NETDATA_DOUBLE new_value,
                            NETDATA_DOUBLE old_value,
                            const char *alert_source,
                            uint32_t duration,
                            uint32_t non_clear_duration,
                            const char *alert_units,
                            const char *alert_info,
                            const char *new_value_string,
                            const char *old_value_string,
                            const char *source,
                            const char *error_msg,
                            int n_warn,
                            int n_crit,
                            const char *warn_alarms,
                            const char *crit_alarms,
                            const char *classification,
                            const char *edit_command,
                            const char *machine_guid,
                            uuid_t *transition_id,
                            const char *summary,
                            const char *context,
                            const char *component,
                            const char *type
) {
    char buf[8192];
    size_t n = sizeof(buf) - 1;

    buffer_strcat(wb, "exec");

    if (!sanitize_command_argument_string(buf, exec, n))
        return false;
    buffer_sprintf(wb, " '%s'", buf);

    if (!sanitize_command_argument_string(buf, recipient, n))
        return false;
    buffer_sprintf(wb, " '%s'", buf);

    if (!sanitize_command_argument_string(buf, registry_hostname, n))
        return false;
    buffer_sprintf(wb, " '%s'", buf);

    buffer_sprintf(wb, " '%u'", unique_id);

    buffer_sprintf(wb, " '%u'", alarm_id);

    buffer_sprintf(wb, " '%u'", alarm_event_id);

    buffer_sprintf(wb, " '%u'", when);

    if (!sanitize_command_argument_string(buf, alert_name, n))
        return false;
    buffer_sprintf(wb, " '%s'", buf);

    if (!sanitize_command_argument_string(buf, alert_chart_name, n))
        return false;
    buffer_sprintf(wb, " '%s'", buf);

    if (!sanitize_command_argument_string(buf, new_status, n))
        return false;
    buffer_sprintf(wb, " '%s'", buf);

    if (!sanitize_command_argument_string(buf, old_status, n))
        return false;
    buffer_sprintf(wb, " '%s'", buf);

    buffer_sprintf(wb, " '" NETDATA_DOUBLE_FORMAT_ZERO "'", new_value);

    buffer_sprintf(wb, " '" NETDATA_DOUBLE_FORMAT_ZERO "'", old_value);

    if (!sanitize_command_argument_string(buf, alert_source, n))
        return false;
    buffer_sprintf(wb, " '%s'", buf);

    buffer_sprintf(wb, " '%u'", duration);

    buffer_sprintf(wb, " '%u'", non_clear_duration);

    if (!sanitize_command_argument_string(buf, alert_units, n))
        return false;
    buffer_sprintf(wb, " '%s'", buf);

    if (!sanitize_command_argument_string(buf, alert_info, n))
        return false;
    buffer_sprintf(wb, " '%s'", buf);

    if (!sanitize_command_argument_string(buf, new_value_string, n))
        return false;
    buffer_sprintf(wb, " '%s'", buf);

    if (!sanitize_command_argument_string(buf, old_value_string, n))
        return false;
    buffer_sprintf(wb, " '%s'", buf);

    if (!sanitize_command_argument_string(buf, source, n))
        return false;
    buffer_sprintf(wb, " '%s'", buf);

    if (!sanitize_command_argument_string(buf, error_msg, n))
        return false;
    buffer_sprintf(wb, " '%s'", buf);

    buffer_sprintf(wb, " '%d'", n_warn);

    buffer_sprintf(wb, " '%d'", n_crit);

    if (!sanitize_command_argument_string(buf, warn_alarms, n))
        return false;
    buffer_sprintf(wb, " '%s'", buf);

    if (!sanitize_command_argument_string(buf, crit_alarms, n))
        return false;
    buffer_sprintf(wb, " '%s'", buf);

    if (!sanitize_command_argument_string(buf, classification, n))
        return false;
    buffer_sprintf(wb, " '%s'", buf);

    if (!sanitize_command_argument_string(buf, edit_command, n))
        return false;
    buffer_sprintf(wb, " '%s'", buf);

    if (!sanitize_command_argument_string(buf, machine_guid, n))
        return false;
    buffer_sprintf(wb, " '%s'", buf);

    char tr_id[UUID_STR_LEN];
    uuid_unparse_lower(*transition_id, tr_id);
    if (!sanitize_command_argument_string(buf, tr_id, n))
        return false;
    buffer_sprintf(wb, " '%s'", buf);

    if (!sanitize_command_argument_string(buf, summary, n))
        return false;
    buffer_sprintf(wb, " '%s'", buf);

    if (!sanitize_command_argument_string(buf, context, n))
        return false;
    buffer_sprintf(wb, " '%s'", buf);

    if (!sanitize_command_argument_string(buf, component, n))
        return false;
    buffer_sprintf(wb, " '%s'", buf);

    if (!sanitize_command_argument_string(buf, type, n))
        return false;
    buffer_sprintf(wb, " '%s'", buf);

    return true;
}

void health_alarm_execute(RRDHOST *host, ALARM_ENTRY *ae) {
    ae->flags |= HEALTH_ENTRY_FLAG_PROCESSED;

    if(unlikely(ae->new_status < RRDCALC_STATUS_CLEAR)) {
        // do not send notifications for internal statuses
        netdata_log_debug(D_HEALTH, "Health not sending notification for alarm '%s.%s' status %s (internal statuses)", ae_chart_id(ae), ae_name(ae), rrdcalc_status2string(ae->new_status));
        goto done;
    }

    if(unlikely(ae->new_status <= RRDCALC_STATUS_CLEAR && (ae->flags & HEALTH_ENTRY_FLAG_NO_CLEAR_NOTIFICATION))) {
        // do not send notifications for disabled statuses

        nd_log(NDLS_DAEMON, NDLP_DEBUG,
               "[%s]: Health not sending notification for alarm '%s.%s' status %s (it has no-clear-notification enabled)",
               rrdhost_hostname(host), ae_chart_id(ae), ae_name(ae), rrdcalc_status2string(ae->new_status));

        // mark it as run, so that we will send the same alarm if it happens again
        goto done;
    }

    // find the previous notification for the same alarm
    // which we have run the exec script
    // exception: alarms with HEALTH_ENTRY_FLAG_NO_CLEAR_NOTIFICATION set
    RRDCALC_STATUS last_executed_status = -3;
    if(likely(!(ae->flags & HEALTH_ENTRY_FLAG_NO_CLEAR_NOTIFICATION))) {
        int ret = sql_health_get_last_executed_event(host, ae, &last_executed_status);

        if (likely(ret == 1)) {
            // we have executed this alarm notification in the past
            if(last_executed_status == ae->new_status && !(ae->flags & HEALTH_ENTRY_FLAG_IS_REPEATING)) {
                // don't send the notification for the same status again
                nd_log(NDLS_DAEMON, NDLP_DEBUG,
                       "[%s]: Health not sending again notification for alarm '%s.%s' status %s",
                       rrdhost_hostname(host), ae_chart_id(ae), ae_name(ae),
                       rrdcalc_status2string(ae->new_status));
                goto done;
            }
        }
        else {
            // we have not executed this alarm notification in the past
            // so, don't send CLEAR notifications
            if(unlikely(ae->new_status == RRDCALC_STATUS_CLEAR)) {
                if((!(ae->flags & HEALTH_ENTRY_RUN_ONCE)) || (ae->flags & HEALTH_ENTRY_RUN_ONCE && ae->old_status < RRDCALC_STATUS_RAISED) ) {
                    netdata_log_debug(D_HEALTH, "Health not sending notification for first initialization of alarm '%s.%s' status %s"
                                      , ae_chart_id(ae), ae_name(ae), rrdcalc_status2string(ae->new_status));
                    goto done;
                }
            }
        }
    }

    // Check if alarm notifications are silenced
    if (ae->flags & HEALTH_ENTRY_FLAG_SILENCED) {
        nd_log(NDLS_DAEMON, NDLP_DEBUG,
               "[%s]: Health not sending notification for alarm '%s.%s' status %s "
               "(command API has disabled notifications)",
               rrdhost_hostname(host), ae_chart_id(ae), ae_name(ae), rrdcalc_status2string(ae->new_status));
        goto done;
    }

    nd_log(NDLS_DAEMON, NDLP_DEBUG,
           "[%s]: Sending notification for alarm '%s.%s' status %s.",
           rrdhost_hostname(host), ae_chart_id(ae), ae_name(ae), rrdcalc_status2string(ae->new_status));

    const char *exec      = (ae->exec)      ? ae_exec(ae)      : string2str(host->health.health_default_exec);
    const char *recipient = (ae->recipient) ? ae_recipient(ae) : string2str(host->health.health_default_recipient);

    int n_warn=0, n_crit=0;
    RRDCALC *rc;
    EVAL_EXPRESSION *expr=NULL;
    BUFFER *warn_alarms, *crit_alarms;
    active_alerts_t *active_alerts = callocz(ACTIVE_ALARMS_LIST_EXAMINE, sizeof(active_alerts_t));

    warn_alarms = buffer_create(NETDATA_WEB_RESPONSE_INITIAL_SIZE, &netdata_buffers_statistics.buffers_health);
    crit_alarms = buffer_create(NETDATA_WEB_RESPONSE_INITIAL_SIZE, &netdata_buffers_statistics.buffers_health);

    foreach_rrdcalc_in_rrdhost_read(host, rc) {
        if(unlikely(!rc->rrdset || !rc->rrdset->last_collected_time.tv_sec))
            continue;

        if(unlikely((n_warn + n_crit) >= ACTIVE_ALARMS_LIST_EXAMINE))
            break;

        if (unlikely(rc->status == RRDCALC_STATUS_WARNING)) {
            if (likely(ae->alarm_id != rc->id) || likely(ae->alarm_event_id != rc->next_event_id - 1)) {
                active_alerts[n_warn+n_crit].name = (char *)rrdcalc_name(rc);
                active_alerts[n_warn+n_crit].last_status_change = rc->last_status_change;
                active_alerts[n_warn+n_crit].status = rc->status;
                n_warn++;
            } else if (ae->alarm_id == rc->id)
                expr = rc->config.warning;
        } else if (unlikely(rc->status == RRDCALC_STATUS_CRITICAL)) {
            if (likely(ae->alarm_id != rc->id) || likely(ae->alarm_event_id != rc->next_event_id - 1)) {
                active_alerts[n_warn+n_crit].name = (char *)rrdcalc_name(rc);
                active_alerts[n_warn+n_crit].last_status_change = rc->last_status_change;
                active_alerts[n_warn+n_crit].status = rc->status;
                n_crit++;
            } else if (ae->alarm_id == rc->id)
                expr = rc->config.critical;
        } else if (unlikely(rc->status == RRDCALC_STATUS_CLEAR)) {
            if (ae->alarm_id == rc->id)
                expr = rc->config.warning;
        }
    }
    foreach_rrdcalc_in_rrdhost_done(rc);

    if (n_warn+n_crit>1)
        qsort (active_alerts, n_warn+n_crit, sizeof(active_alerts_t), compare_active_alerts);

    int count_w = 0, count_c = 0;
    while (count_w + count_c < n_warn + n_crit && count_w + count_c < ACTIVE_ALARMS_LIST) {
        if (active_alerts[count_w+count_c].status == RRDCALC_STATUS_WARNING) {
            if (count_w)
                buffer_strcat(warn_alarms, ",");
            buffer_strcat(warn_alarms, active_alerts[count_w+count_c].name);
            buffer_strcat(warn_alarms, "=");
            buffer_snprintf(warn_alarms, 11, "%"PRId64"", (int64_t)active_alerts[count_w+count_c].last_status_change);
            count_w++;
        }
        else if (active_alerts[count_w+count_c].status == RRDCALC_STATUS_CRITICAL) {
            if (count_c)
                buffer_strcat(crit_alarms, ",");
            buffer_strcat(crit_alarms, active_alerts[count_w+count_c].name);
            buffer_strcat(crit_alarms, "=");
            buffer_snprintf(crit_alarms, 11, "%"PRId64"", (int64_t)active_alerts[count_w+count_c].last_status_change);
            count_c++;
        }
    }

    char *edit_command = ae->source ? health_edit_command_from_source(ae_source(ae)) : strdupz("UNKNOWN=0=UNKNOWN");

    BUFFER *wb = buffer_create(8192, &netdata_buffers_statistics.buffers_health);
    bool ok = prepare_command(wb,
                              exec,
                              recipient,
                              rrdhost_registry_hostname(host),
                              ae->unique_id,
                              ae->alarm_id,
                              ae->alarm_event_id,
                              (unsigned long)ae->when,
                              ae_name(ae),
                              ae->chart?ae_chart_id(ae):"NOCHART",
                              rrdcalc_status2string(ae->new_status),
                              rrdcalc_status2string(ae->old_status),
                              ae->new_value,
                              ae->old_value,
                              ae->source?ae_source(ae):"UNKNOWN",
                              (uint32_t)ae->duration,
                              (ae->flags & HEALTH_ENTRY_FLAG_IS_REPEATING && ae->new_status >= RRDCALC_STATUS_WARNING) ? (uint32_t)ae->duration : (uint32_t)ae->non_clear_duration,
                              ae_units(ae),
                              ae_info(ae),
                              ae_new_value_string(ae),
                              ae_old_value_string(ae),
                              (expr && expr->source)?string2str(expr->source):"NOSOURCE",
                              (expr && expr->error_msg)?buffer_tostring(expr->error_msg):"NOERRMSG",
                              n_warn,
                              n_crit,
                              buffer_tostring(warn_alarms),
                              buffer_tostring(crit_alarms),
                              ae->classification?ae_classification(ae):"Unknown",
                              edit_command,
                              host->machine_guid,
                              &ae->transition_id,
                              host->health.use_summary_for_notifications && ae->summary?ae_summary(ae):ae_name(ae),
                              string2str(ae->chart_context),
                              string2str(ae->component),
                              string2str(ae->type)
    );

    const char *command_to_run = buffer_tostring(wb);
    if (ok) {
        ae->flags |= HEALTH_ENTRY_FLAG_EXEC_RUN;
        ae->exec_run_timestamp = now_realtime_sec(); /* will be updated by real time after spawning */

        netdata_log_debug(D_HEALTH, "executing command '%s'", command_to_run);
        ae->flags |= HEALTH_ENTRY_FLAG_EXEC_IN_PROGRESS;
        ae->exec_spawn_serial = spawn_enq_cmd(command_to_run);
        enqueue_alarm_notify_in_progress(ae);
        health_alarm_log_save(host, ae);
    } else {
        netdata_log_error("Failed to format command arguments");
    }

    buffer_free(wb);
    freez(edit_command);
    buffer_free(warn_alarms);
    buffer_free(crit_alarms);
    freez(active_alerts);

    return; //health_alarm_wait_for_execution
done:
    health_alarm_log_save(host, ae);
}

void health_send_notification(RRDHOST *host, ALARM_ENTRY *ae) {
    netdata_log_debug(D_HEALTH, "Health alarm '%s.%s' = " NETDATA_DOUBLE_FORMAT_AUTO " - changed status from %s to %s",
                      ae->chart?ae_chart_id(ae):"NOCHART", ae_name(ae),
                      ae->new_value,
                      rrdcalc_status2string(ae->old_status),
                      rrdcalc_status2string(ae->new_status)
    );

    health_alarm_execute(host, ae);
}

bool health_alarm_log_get_global_id_and_transition_id_for_rrdcalc(RRDCALC *rc, usec_t *global_id, uuid_t *transitions_id) {
    if(!rc->rrdset)
        return false;

    RRDHOST *host = rc->rrdset->rrdhost;

    rw_spinlock_read_lock(&host->health_log.spinlock);

    ALARM_ENTRY *ae;
    for(ae = host->health_log.alarms; ae ; ae = ae->next) {
        if(unlikely(ae->alarm_id == rc->id))
            break;
    }

    if(ae) {
        *global_id = ae->global_id;
        uuid_copy(*transitions_id, ae->transition_id);
    }
    else {
        *global_id = 0;
        uuid_clear(*transitions_id);
    }

    rw_spinlock_read_unlock(&host->health_log.spinlock);

    return ae != NULL;
}

void health_alarm_log_process_to_send_notifications(RRDHOST *host) {
    uint32_t first_waiting = (host->health_log.alarms)?host->health_log.alarms->unique_id:0;
    time_t now = now_realtime_sec();

    rw_spinlock_read_lock(&host->health_log.spinlock);

    ALARM_ENTRY *ae;
    for(ae = host->health_log.alarms; ae && ae->unique_id >= host->health_last_processed_id; ae = ae->next) {
        if(unlikely(
                !(ae->flags & HEALTH_ENTRY_FLAG_PROCESSED) &&
                !(ae->flags & HEALTH_ENTRY_FLAG_UPDATED)
                    )) {
            if(unlikely(ae->unique_id < first_waiting))
                first_waiting = ae->unique_id;

            if(likely(now >= ae->delay_up_to_timestamp))
                health_send_notification(host, ae);
        }
    }

    rw_spinlock_read_unlock(&host->health_log.spinlock);

    // remember this for the next iteration
    host->health_last_processed_id = first_waiting;

    //delete those that are updated, no in progress execution, and is not repeating
    rw_spinlock_write_lock(&host->health_log.spinlock);

    ALARM_ENTRY *prev = NULL, *next = NULL;
    for(ae = host->health_log.alarms; ae ; ae = next) {
        next = ae->next; // set it here, for the next iteration

        if((likely(!(ae->flags & HEALTH_ENTRY_FLAG_IS_REPEATING)) &&
             (ae->flags & HEALTH_ENTRY_FLAG_UPDATED) &&
             (ae->flags & HEALTH_ENTRY_FLAG_SAVED) &&
             !(ae->flags & HEALTH_ENTRY_FLAG_EXEC_IN_PROGRESS))
            ||
            ((ae->new_status == RRDCALC_STATUS_REMOVED) &&
             (ae->flags & HEALTH_ENTRY_FLAG_SAVED) &&
             (ae->when + 86400 < now_realtime_sec())))
        {

            if(host->health_log.alarms == ae) {
                host->health_log.alarms = next;
                // prev is also NULL here
            }
            else {
                prev->next = next;
                // prev should not be touched here - we need it for the next iteration
                // because we may have to also remove the next item
            }

            health_alarm_log_free_one_nochecks_nounlink(ae);
        }
        else
            prev = ae;
    }

    rw_spinlock_write_unlock(&host->health_log.spinlock);
}
