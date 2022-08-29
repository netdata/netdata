// SPDX-License-Identifier: GPL-3.0-or-later

#include "health.h"

unsigned int default_health_enabled = 1;
char *silencers_filename;

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

static inline void unlink_alarm_notify_in_progress(ALARM_ENTRY *ae)
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
// ----------------------------------------------------------------------------
// health initialization

/**
 * User Config directory
 *
 * Get the config directory for health and return it.
 *
 * @return a pointer to the user config directory
 */
inline char *health_user_config_dir(void) {
    char buffer[FILENAME_MAX + 1];
    snprintfz(buffer, FILENAME_MAX, "%s/health.d", netdata_configured_user_config_dir);
    return config_get(CONFIG_SECTION_DIRECTORIES, "health config", buffer);
}

/**
 * Stock Config Directory
 *
 * Get the Stock config directory and return it.
 *
 * @return a pointer to the stock config directory.
 */
inline char *health_stock_config_dir(void) {
    char buffer[FILENAME_MAX + 1];
    snprintfz(buffer, FILENAME_MAX, "%s/health.d", netdata_configured_stock_config_dir);
    return config_get(CONFIG_SECTION_DIRECTORIES, "stock health config", buffer);
}

/**
 * Silencers init
 *
 * Function used to initialize the silencer structure.
 */
static void health_silencers_init(void) {
    FILE *fd = fopen(silencers_filename, "r");
    if (fd) {
        fseek(fd, 0 , SEEK_END);
        off_t length = (off_t) ftell(fd);
        fseek(fd, 0 , SEEK_SET);

        if (length > 0 && length < HEALTH_SILENCERS_MAX_FILE_LEN) {
            char *str = mallocz((length+1)* sizeof(char));
            if(str) {
                size_t copied;
                copied = fread(str, sizeof(char), length, fd);
                if (copied == (length* sizeof(char))) {
                    str[length] = 0x00;
                    json_parse(str, NULL, health_silencers_json_read_callback);
                    info("Parsed health silencers file %s", silencers_filename);
                } else {
                    error("Cannot read the data from health silencers file %s", silencers_filename);
                }
                freez(str);
            }
        } else {
            error(
                "Health silencers file %s has the size %" PRId64 " that is out of range[ 1 , %d ]. Aborting read.",
                silencers_filename,
                (int64_t)length,
                HEALTH_SILENCERS_MAX_FILE_LEN);
        }
        fclose(fd);
    } else {
        info("Cannot open the file %s, so Netdata will work with the default health configuration.",silencers_filename);
    }
}

/**
 * Health Init
 *
 * Initialize the health thread.
 */
void health_init(void) {
    debug(D_HEALTH, "Health configuration initializing");

    if(!(default_health_enabled = (unsigned int)config_get_boolean(CONFIG_SECTION_HEALTH, "enabled", default_health_enabled))) {
        debug(D_HEALTH, "Health is disabled.");
        return;
    }

    health_silencers_init();
}

// ----------------------------------------------------------------------------
// re-load health configuration

/**
 * Reload host
 *
 * Reload configuration for a specific host.
 *
 * @param host the structure of the host that the function will reload the configuration.
 */
static void health_reload_host(RRDHOST *host) {
    if(unlikely(!host->health_enabled))
        return;

    char *user_path = health_user_config_dir();
    char *stock_path = health_stock_config_dir();

    // free all running alarms
    rrdhost_wrlock(host);

    while(host->templates)
        rrdcalctemplate_unlink_and_free(host, host->templates);

    RRDCALCTEMPLATE *rt,*next;
    for(rt = host->alarms_template_with_foreach; rt ; rt = next) {
        next = rt->next;
        rrdcalctemplate_free(rt);
    }
    host->alarms_template_with_foreach = NULL;

    while(host->alarms)
        rrdcalc_unlink_and_free(host, host->alarms);

    RRDCALC *rc,*nc;
    for(rc = host->alarms_with_foreach; rc ; rc = nc) {
        nc = rc->next;
        rrdcalc_free(rc);
    }
    host->alarms_with_foreach = NULL;

    rrdhost_unlock(host);

    // invalidate all previous entries in the alarm log
    ALARM_ENTRY *t;
    for(t = host->health_log.alarms ; t ; t = t->next) {
        if(t->new_status != RRDCALC_STATUS_REMOVED)
            t->flags |= HEALTH_ENTRY_FLAG_UPDATED;
    }

    rrdhost_rdlock(host);
    // reset all thresholds to all charts
    RRDSET *st;
    rrdset_foreach_read(st, host) {
        st->green = NAN;
        st->red = NAN;
    }
    rrdhost_unlock(host);

    // load the new alarms
    rrdhost_wrlock(host);
    health_readdir(host, user_path, stock_path, NULL);

    //Discard alarms with labels that do not apply to host
    rrdcalc_labels_unlink_alarm_from_host(host);

    // link the loaded alarms to their charts
    RRDDIM *rd;
    rrdset_foreach_write(st, host) {
        if (rrdset_flag_check(st, RRDSET_FLAG_ARCHIVED))
            continue;
        rrdsetcalc_link_matching(st);
        rrdcalctemplate_link_matching(st);

        //This loop must be the last, because ` rrdcalctemplate_link_matching` will create alarms related to it.
        rrdset_rdlock(st);
        rrddim_foreach_read(rd, st) {
            rrdcalc_link_to_rrddim(rd, st, host);
        }
        rrdset_unlock(st);
    }

    rrdhost_unlock(host);
}

/**
 * Reload
 *
 * Reload the host configuration for all hosts.
 */
void health_reload(void) {
    sql_refresh_hashes();

    rrd_rdlock();

    RRDHOST *host;
    rrdhost_foreach_read(host)
        health_reload_host(host);

    rrd_unlock();
#ifdef ENABLE_ACLK
    if (netdata_cloud_setting) {
        aclk_alert_reloaded = 1;
    }
#endif
}

// ----------------------------------------------------------------------------
// health main thread and friends

static inline RRDCALC_STATUS rrdcalc_value2status(NETDATA_DOUBLE n) {
    if(isnan(n) || isinf(n)) return RRDCALC_STATUS_UNDEFINED;
    if(n) return RRDCALC_STATUS_RAISED;
    return RRDCALC_STATUS_CLEAR;
}

#define ALARM_EXEC_COMMAND_LENGTH 8192
#define ACTIVE_ALARMS_LIST_EXAMINE 500
#define ACTIVE_ALARMS_LIST 15

static inline int compare_active_alerts(const void * a, const void * b) {
    active_alerts_t *active_alerts_a = (active_alerts_t *)a;
    active_alerts_t *active_alerts_b = (active_alerts_t *)b;

    return ( active_alerts_b->last_status_change - active_alerts_a->last_status_change );
}

static inline void health_alarm_execute(RRDHOST *host, ALARM_ENTRY *ae) {
    ae->flags |= HEALTH_ENTRY_FLAG_PROCESSED;

    if(unlikely(ae->new_status < RRDCALC_STATUS_CLEAR)) {
        // do not send notifications for internal statuses
        debug(D_HEALTH, "Health not sending notification for alarm '%s.%s' status %s (internal statuses)", ae_chart_name(ae), ae_name(ae), rrdcalc_status2string(ae->new_status));
        goto done;
    }

    if(unlikely(ae->new_status <= RRDCALC_STATUS_CLEAR && (ae->flags & HEALTH_ENTRY_FLAG_NO_CLEAR_NOTIFICATION))) {
        // do not send notifications for disabled statuses
        debug(D_HEALTH, "Health not sending notification for alarm '%s.%s' status %s (it has no-clear-notification enabled)", ae_chart_name(ae), ae_name(ae), rrdcalc_status2string(ae->new_status));
        // mark it as run, so that we will send the same alarm if it happens again
        goto done;
    }

    // find the previous notification for the same alarm
    // which we have run the exec script
    // exception: alarms with HEALTH_ENTRY_FLAG_NO_CLEAR_NOTIFICATION set
    if(likely(!(ae->flags & HEALTH_ENTRY_FLAG_NO_CLEAR_NOTIFICATION))) {
        uint32_t id = ae->alarm_id;
        ALARM_ENTRY *t;
        for(t = ae->next; t ; t = t->next) {
            if(t->alarm_id == id && t->flags & HEALTH_ENTRY_FLAG_EXEC_RUN)
                break;
        }

        if(likely(t)) {
            // we have executed this alarm notification in the past
            if(t && t->new_status == ae->new_status) {
                // don't send the notification for the same status again
                debug(D_HEALTH, "Health not sending again notification for alarm '%s.%s' status %s", ae_chart_name(ae), ae_name(ae)
                      , rrdcalc_status2string(ae->new_status));
                goto done;
            }
        }
        else {
            // we have not executed this alarm notification in the past
            // so, don't send CLEAR notifications
            if(unlikely(ae->new_status == RRDCALC_STATUS_CLEAR)) {
                if((!(ae->flags & HEALTH_ENTRY_RUN_ONCE)) || (ae->flags & HEALTH_ENTRY_RUN_ONCE && ae->old_status < RRDCALC_STATUS_RAISED) ) {
                    debug(D_HEALTH, "Health not sending notification for first initialization of alarm '%s.%s' status %s"
                    , ae_chart_name(ae), ae_name(ae), rrdcalc_status2string(ae->new_status));
                    goto done;
                }
            }
        }
    }

    // Check if alarm notifications are silenced
    if (ae->flags & HEALTH_ENTRY_FLAG_SILENCED) {
        info("Health not sending notification for alarm '%s.%s' status %s (command API has disabled notifications)", ae_chart_name(ae), ae_name(ae), rrdcalc_status2string(ae->new_status));
        goto done;
    }

    static char command_to_run[ALARM_EXEC_COMMAND_LENGTH + 1];

    const char *exec      = (ae->exec)      ? ae_exec(ae)      : string2str(host->health_default_exec);
    const char *recipient = (ae->recipient) ? ae_recipient(ae) : string2str(host->health_default_recipient);

    int n_warn=0, n_crit=0;
    RRDCALC *rc;
    EVAL_EXPRESSION *expr=NULL;
    BUFFER *warn_alarms, *crit_alarms;
    active_alerts_t *active_alerts = callocz(ACTIVE_ALARMS_LIST_EXAMINE, sizeof(active_alerts_t));

    warn_alarms = buffer_create(NETDATA_WEB_RESPONSE_INITIAL_SIZE);
    crit_alarms = buffer_create(NETDATA_WEB_RESPONSE_INITIAL_SIZE);

    for(rc = host->alarms; rc && (n_warn + n_crit) < ACTIVE_ALARMS_LIST_EXAMINE ; rc = rc->next) {
        if(unlikely(!rc->rrdset || !rc->rrdset->last_collected_time.tv_sec))
            continue;

        if (unlikely(rc->status == RRDCALC_STATUS_WARNING)) {
            if (likely(ae->alarm_id != rc->id) || likely(ae->alarm_event_id != rc->next_event_id - 1)) {
                active_alerts[n_warn+n_crit].name = (char *)rrdcalc_name(rc);
                active_alerts[n_warn+n_crit].last_status_change = rc->last_status_change;
                active_alerts[n_warn+n_crit].status = rc->status;
                n_warn++;
            } else if (ae->alarm_id == rc->id)
                expr = rc->warning;
        } else if (unlikely(rc->status == RRDCALC_STATUS_CRITICAL)) {
            if (likely(ae->alarm_id != rc->id) || likely(ae->alarm_event_id != rc->next_event_id - 1)) {
                active_alerts[n_warn+n_crit].name = (char *)rrdcalc_name(rc);
                active_alerts[n_warn+n_crit].last_status_change = rc->last_status_change;
                active_alerts[n_warn+n_crit].status = rc->status;
                n_crit++;
            } else if (ae->alarm_id == rc->id)
                expr = rc->critical;
        } else if (unlikely(rc->status == RRDCALC_STATUS_CLEAR)) {
            if (ae->alarm_id == rc->id)
                expr = rc->warning;
        }
    }

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

    snprintfz(command_to_run, ALARM_EXEC_COMMAND_LENGTH, "exec %s '%s' '%s' '%u' '%u' '%u' '%lu' '%s' '%s' '%s' '%s' '%s' '" NETDATA_DOUBLE_FORMAT_ZERO
        "' '" NETDATA_DOUBLE_FORMAT_ZERO
        "' '%s' '%u' '%u' '%s' '%s' '%s' '%s' '%s' '%s' '%d' '%d' '%s' '%s' '%s' '%s' '%s'",
              exec,
              recipient,
              rrdhost_registry_hostname(host),
              ae->unique_id,
              ae->alarm_id,
              ae->alarm_event_id,
              (unsigned long)ae->when,
              ae_name(ae),
              ae->chart?ae_chart_name(ae):"NOCHART",
              ae->family?ae_family(ae):"NOFAMILY",
              rrdcalc_status2string(ae->new_status),
              rrdcalc_status2string(ae->old_status),
              ae->new_value,
              ae->old_value,
              ae->source?ae_source(ae):"UNKNOWN",
              (uint32_t)ae->duration,
              (uint32_t)ae->non_clear_duration,
              ae_units(ae),
              ae_info(ae),
              ae_new_value_string(ae),
              ae_old_value_string(ae),
              (expr && expr->source)?expr->source:"NOSOURCE",
              (expr && expr->error_msg)?buffer_tostring(expr->error_msg):"NOERRMSG",
              n_warn,
              n_crit,
              buffer_tostring(warn_alarms),
              buffer_tostring(crit_alarms),
              ae->classification?ae_classification(ae):"Unknown",
              edit_command,
              host != localhost ? host->machine_guid:""
    );

    ae->flags |= HEALTH_ENTRY_FLAG_EXEC_RUN;
    ae->exec_run_timestamp = now_realtime_sec(); /* will be updated by real time after spawning */

    debug(D_HEALTH, "executing command '%s'", command_to_run);
    ae->flags |= HEALTH_ENTRY_FLAG_EXEC_IN_PROGRESS;
    ae->exec_spawn_serial = spawn_enq_cmd(command_to_run);
    enqueue_alarm_notify_in_progress(ae);

    freez(edit_command);
    buffer_free(warn_alarms);
    buffer_free(crit_alarms);
    freez(active_alerts);

    return; //health_alarm_wait_for_execution
done:
    health_alarm_log_save(host, ae);
}

static inline void health_alarm_wait_for_execution(ALARM_ENTRY *ae) {
    if (!(ae->flags & HEALTH_ENTRY_FLAG_EXEC_IN_PROGRESS))
        return;

    spawn_wait_cmd(ae->exec_spawn_serial, &ae->exec_code, &ae->exec_run_timestamp);
    debug(D_HEALTH, "done executing command - returned with code %d", ae->exec_code);
    ae->flags &= ~HEALTH_ENTRY_FLAG_EXEC_IN_PROGRESS;

    if(ae->exec_code != 0)
        ae->flags |= HEALTH_ENTRY_FLAG_EXEC_FAILED;

    unlink_alarm_notify_in_progress(ae);
}

static inline void health_process_notifications(RRDHOST *host, ALARM_ENTRY *ae) {
    debug(D_HEALTH, "Health alarm '%s.%s' = " NETDATA_DOUBLE_FORMAT_AUTO " - changed status from %s to %s",
         ae->chart?ae_chart_name(ae):"NOCHART", ae_name(ae),
         ae->new_value,
         rrdcalc_status2string(ae->old_status),
         rrdcalc_status2string(ae->new_status)
    );

    health_alarm_execute(host, ae);
}

static inline void health_alarm_log_process(RRDHOST *host) {
    uint32_t first_waiting = (host->health_log.alarms)?host->health_log.alarms->unique_id:0;
    time_t now = now_realtime_sec();

    netdata_rwlock_rdlock(&host->health_log.alarm_log_rwlock);

    ALARM_ENTRY *ae;
    for(ae = host->health_log.alarms; ae && ae->unique_id >= host->health_last_processed_id; ae = ae->next) {
        if(likely(!alarm_entry_isrepeating(host, ae))) {
            if(unlikely(
                    !(ae->flags & HEALTH_ENTRY_FLAG_PROCESSED) &&
                    !(ae->flags & HEALTH_ENTRY_FLAG_UPDATED)
            )) {
                if(unlikely(ae->unique_id < first_waiting))
                    first_waiting = ae->unique_id;

                if(likely(now >= ae->delay_up_to_timestamp))
                    health_process_notifications(host, ae);
            }
        }
    }

    // remember this for the next iteration
    host->health_last_processed_id = first_waiting;

    bool cleanup_excess_log_entries = host->health_log.count > host->health_log.max;

    netdata_rwlock_unlock(&host->health_log.alarm_log_rwlock);

    if (!cleanup_excess_log_entries)
        return;

    // cleanup excess entries in the log
    netdata_rwlock_wrlock(&host->health_log.alarm_log_rwlock);

    ALARM_ENTRY *last = NULL;
    unsigned int count = host->health_log.max * 2 / 3;
    for(ae = host->health_log.alarms; ae && count ; count--, last = ae, ae = ae->next) ;

    if(ae && last && last->next == ae)
        last->next = NULL;
    else
        ae = NULL;

    while(ae) {
        debug(D_HEALTH, "Health removing alarm log entry with id: %u", ae->unique_id);

        ALARM_ENTRY *t = ae->next;

        if(likely(!alarm_entry_isrepeating(host, ae))) {
            health_alarm_wait_for_execution(ae);
            health_alarm_log_free_one_nochecks_nounlink(ae);
            host->health_log.count--;
        }

        ae = t;
    }

    netdata_rwlock_unlock(&host->health_log.alarm_log_rwlock);
}

static inline int rrdcalc_isrunnable(RRDCALC *rc, time_t now, time_t *next_run) {
    if(unlikely(!rc->rrdset)) {
        debug(D_HEALTH, "Health not running alarm '%s.%s'. It is not linked to a chart.", rrdcalc_chart_name(rc), rrdcalc_name(rc));
        return 0;
    }

    if(unlikely(rc->next_update > now)) {
        if (unlikely(*next_run > rc->next_update)) {
            // update the next_run time of the main loop
            // to run this alarm precisely the time required
            *next_run = rc->next_update;
        }

        debug(D_HEALTH, "Health not examining alarm '%s.%s' yet (will do in %d secs).", rrdcalc_chart_name(rc), rrdcalc_name(rc), (int) (rc->next_update - now));
        return 0;
    }

    if(unlikely(!rc->update_every)) {
        debug(D_HEALTH, "Health not running alarm '%s.%s'. It does not have an update frequency", rrdcalc_chart_name(rc), rrdcalc_name(rc));
        return 0;
    }

    if(unlikely(rrdset_flag_check(rc->rrdset, RRDSET_FLAG_OBSOLETE))) {
        debug(D_HEALTH, "Health not running alarm '%s.%s'. The chart has been marked as obsolete", rrdcalc_chart_name(rc), rrdcalc_name(rc));
        return 0;
    }

    if(unlikely(rrdset_flag_check(rc->rrdset, RRDSET_FLAG_ARCHIVED))) {
        debug(D_HEALTH, "Health not running alarm '%s.%s'. The chart has been marked as archived", rrdcalc_chart_name(rc), rrdcalc_name(rc));
        return 0;
    }

    if(unlikely(!rc->rrdset->last_collected_time.tv_sec || rc->rrdset->counter_done < 2)) {
        debug(D_HEALTH, "Health not running alarm '%s.%s'. Chart is not fully collected yet.", rrdcalc_chart_name(rc), rrdcalc_name(rc));
        return 0;
    }

    int update_every = rc->rrdset->update_every;
    rrdset_rdlock(rc->rrdset);
    time_t first = rrdset_first_entry_t_nolock(rc->rrdset);
    time_t last = rrdset_last_entry_t_nolock(rc->rrdset);
    rrdset_unlock(rc->rrdset);

    if(unlikely(now + update_every < first /* || now - update_every > last */)) {
        debug(D_HEALTH
              , "Health not examining alarm '%s.%s' yet (wanted time is out of bounds - we need %lu but got %lu - %lu)."
              , rrdcalc_chart_name(rc), rrdcalc_name(rc), (unsigned long) now, (unsigned long) first
              , (unsigned long) last);
        return 0;
    }

    if(RRDCALC_HAS_DB_LOOKUP(rc)) {
        time_t needed = now + rc->before + rc->after;

        if(needed + update_every < first || needed - update_every > last) {
            debug(D_HEALTH
                  , "Health not examining alarm '%s.%s' yet (not enough data yet - we need %lu but got %lu - %lu)."
                  , rrdcalc_chart_name(rc), rrdcalc_name(rc), (unsigned long) needed, (unsigned long) first
                  , (unsigned long) last);
            return 0;
        }
    }

    return 1;
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

static void health_main_cleanup(void *ptr) {
    worker_unregister();

    struct netdata_static_thread *static_thread = (struct netdata_static_thread *)ptr;
    static_thread->enabled = NETDATA_MAIN_THREAD_EXITING;

    info("cleaning up...");

    static_thread->enabled = NETDATA_MAIN_THREAD_EXITED;
}

static SILENCE_TYPE check_silenced(RRDCALC *rc, const char *host, SILENCERS *silencers) {
    SILENCER *s;
    debug(D_HEALTH, "Checking if alarm was silenced via the command API. Alarm info name:%s context:%s chart:%s host:%s family:%s",
          rrdcalc_name(rc), (rc->rrdset)?rrdset_context(rc->rrdset):"", rrdcalc_chart_name(rc), host, (rc->rrdset)?rrdset_family(rc->rrdset):"");

    for (s = silencers->silencers; s!=NULL; s=s->next){
        if (
                (!s->alarms_pattern || (rc->name && s->alarms_pattern && simple_pattern_matches(s->alarms_pattern, rrdcalc_name(rc)))) &&
                (!s->contexts_pattern || (rc->rrdset && rc->rrdset->context && s->contexts_pattern && simple_pattern_matches(s->contexts_pattern, rrdset_context(rc->rrdset)))) &&
                (!s->hosts_pattern || (host && s->hosts_pattern && simple_pattern_matches(s->hosts_pattern,host))) &&
                (!s->charts_pattern || (rc->chart && s->charts_pattern && simple_pattern_matches(s->charts_pattern, rrdcalc_chart_name(rc)))) &&
                (!s->families_pattern || (rc->rrdset && rc->rrdset->family && s->families_pattern && simple_pattern_matches(s->families_pattern, rrdset_family(rc->rrdset))))
                ) {
            debug(D_HEALTH, "Alarm matches command API silence entry %s:%s:%s:%s:%s", s->alarms,s->charts, s->contexts, s->hosts, s->families);
            if (unlikely(silencers->stype == STYPE_NONE)) {
                debug(D_HEALTH, "Alarm %s matched a silence entry, but no SILENCE or DISABLE command was issued via the command API. The match has no effect.", rrdcalc_name(rc));
            } else {
                debug(D_HEALTH, "Alarm %s via the command API - name:%s context:%s chart:%s host:%s family:%s"
                        , (silencers->stype == STYPE_DISABLE_ALARMS)?"Disabled":"Silenced"
                        , rrdcalc_name(rc)
                        , (rc->rrdset)?rrdset_context(rc->rrdset):""
                        , rrdcalc_chart_name(rc)
                        , host
                        , (rc->rrdset)?rrdset_family(rc->rrdset):""
                        );
            }
            return silencers->stype;
        }
    }
    return STYPE_NONE;
}

/**
 * Update Disabled Silenced
 *
 * Update the variable rrdcalc_flags of the structure RRDCALC according with the values of the host structure
 *
 * @param host structure that contains information about the host monitored.
 * @param rc structure with information about the alarm
 *
 * @return It returns 1 case rrdcalc_flags is DISABLED or 0 otherwise
 */
static int update_disabled_silenced(RRDHOST *host, RRDCALC *rc) {
    uint32_t rrdcalc_flags_old = rc->rrdcalc_flags;
    // Clear the flags
    rc->rrdcalc_flags &= ~(RRDCALC_FLAG_DISABLED | RRDCALC_FLAG_SILENCED);
    if (unlikely(silencers->all_alarms)) {
        if (silencers->stype == STYPE_DISABLE_ALARMS) rc->rrdcalc_flags |= RRDCALC_FLAG_DISABLED;
        else if (silencers->stype == STYPE_SILENCE_NOTIFICATIONS) rc->rrdcalc_flags |= RRDCALC_FLAG_SILENCED;
    } else {
        SILENCE_TYPE st = check_silenced(rc, rrdhost_hostname(host), silencers);
        if (st == STYPE_DISABLE_ALARMS) rc->rrdcalc_flags |= RRDCALC_FLAG_DISABLED;
        else if (st == STYPE_SILENCE_NOTIFICATIONS) rc->rrdcalc_flags |= RRDCALC_FLAG_SILENCED;
    }

    if (rrdcalc_flags_old != rc->rrdcalc_flags) {
        info("Alarm silencing changed for host '%s' alarm '%s': Disabled %s->%s Silenced %s->%s",
             rrdhost_hostname(host),
             rrdcalc_name(rc),
             (rrdcalc_flags_old & RRDCALC_FLAG_DISABLED)?"true":"false",
             (rc->rrdcalc_flags & RRDCALC_FLAG_DISABLED)?"true":"false",
             (rrdcalc_flags_old & RRDCALC_FLAG_SILENCED)?"true":"false",
             (rc->rrdcalc_flags & RRDCALC_FLAG_SILENCED)?"true":"false"
        );
    }
    if (rc->rrdcalc_flags & RRDCALC_FLAG_DISABLED)
        return 1;
    else
        return 0;
}

// Create alarms for dimensions that have been added to charts
// since the previous iteration.
static void init_pending_foreach_alarms(RRDHOST *host) {
    RRDSET *st;
    RRDDIM *rd;

    if (!rrdhost_flag_check(host, RRDHOST_FLAG_PENDING_FOREACH_ALARMS))
        return;

    rrdhost_wrlock(host);

    rrdset_foreach_write(st, host) {
        if (!rrdset_flag_check(st, RRDSET_FLAG_PENDING_FOREACH_ALARMS))
            continue;

        rrdset_rdlock(st);

        rrddim_foreach_read(rd, st) {
            if (!rrddim_flag_check(rd, RRDDIM_FLAG_PENDING_FOREACH_ALARM))
                continue;

            rrdcalc_link_to_rrddim(rd, st, host);

            rrddim_flag_clear(rd, RRDDIM_FLAG_PENDING_FOREACH_ALARM);
        }

        rrdset_flag_clear(st, RRDSET_FLAG_PENDING_FOREACH_ALARMS);
        rrdset_unlock(st);
    }

    rrdhost_flag_clear(host, RRDHOST_FLAG_PENDING_FOREACH_ALARMS);
    rrdhost_unlock(host);
}

/**
 * Health Main
 *
 * The main thread of the health system. In this function all the alarms will be processed.
 *
 * @param ptr is a pointer to the netdata_static_thread structure.
 *
 * @return It always returns NULL
 */

#define WORKER_HEALTH_JOB_RRD_LOCK           0
#define WORKER_HEALTH_JOB_HOST_LOCK          1
#define WORKER_HEALTH_JOB_DB_QUERY           2
#define WORKER_HEALTH_JOB_CALC_EVAL          3
#define WORKER_HEALTH_JOB_WARNING_EVAL       4
#define WORKER_HEALTH_JOB_CRITICAL_EVAL      5
#define WORKER_HEALTH_JOB_ALARM_LOG_ENTRY    6
#define WORKER_HEALTH_JOB_ALARM_LOG_PROCESS  7

#if WORKER_UTILIZATION_MAX_JOB_TYPES < 8
#error WORKER_UTILIZATION_MAX_JOB_TYPES has to be at least 8
#endif

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

    netdata_thread_cleanup_push(health_main_cleanup, ptr);

    int min_run_every = (int)config_get_number(CONFIG_SECTION_HEALTH, "run at least every seconds", 10);
    if(min_run_every < 1) min_run_every = 1;

    int cleanup_sql_every_loop = 7200 / min_run_every;

    time_t now                = now_realtime_sec();
    time_t hibernation_delay  = config_get_number(CONFIG_SECTION_HEALTH, "postpone alarms during hibernation for seconds", 60);

    rrdcalc_labels_unlink();

    unsigned int loop = 0;
#ifdef ENABLE_ACLK
    unsigned int marked_aclk_reload_loop = 0;
#endif
    while(!netdata_exit) {
        loop++;
        debug(D_HEALTH, "Health monitoring iteration no %u started", loop);

        int runnable = 0, apply_hibernation_delay = 0;
        time_t next_run = now + min_run_every;
        RRDCALC *rc;

        if (unlikely(check_if_resumed_from_suspension())) {
            apply_hibernation_delay = 1;

            info(
                "Postponing alarm checks for %"PRId64" seconds, "
                "because it seems that the system was just resumed from suspension.",
                (int64_t)hibernation_delay);
        }

        if (unlikely(silencers->all_alarms && silencers->stype == STYPE_DISABLE_ALARMS)) {
            static int logged=0;
            if (!logged) {
                info("Skipping health checks, because all alarms are disabled via a %s command.",
                     HEALTH_CMDAPI_CMD_DISABLEALL);
                logged = 1;
            }
        }

#ifdef ENABLE_ACLK
        if (aclk_alert_reloaded && !marked_aclk_reload_loop)
            marked_aclk_reload_loop = loop;
#endif

        worker_is_busy(WORKER_HEALTH_JOB_RRD_LOCK);
        rrd_rdlock();

        RRDHOST *host;
        rrdhost_foreach_read(host) {
            if (unlikely(!host->health_enabled))
                continue;

            if (unlikely(apply_hibernation_delay)) {
                info(
                    "Postponing health checks for %"PRId64" seconds, on host '%s'.",
                    (int64_t)hibernation_delay,
                    rrdhost_hostname(host));

                host->health_delay_up_to = now + hibernation_delay;
            }

            if (unlikely(host->health_delay_up_to)) {
                if (unlikely(now < host->health_delay_up_to))
                    continue;

                info("Resuming health checks on host '%s'.", rrdhost_hostname(host));
                host->health_delay_up_to = 0;
            }

            // wait until cleanup of obsolete charts on children is complete
            if (host != localhost)
                if (unlikely(host->trigger_chart_obsoletion_check == 1))
                    continue;

            if(likely(!host->health_log_fp) && (loop == 1 || loop % cleanup_sql_every_loop == 0))
                sql_health_alarm_log_cleanup(host);

            init_pending_foreach_alarms(host);

            worker_is_busy(WORKER_HEALTH_JOB_HOST_LOCK);
            rrdhost_rdlock(host);

            // the first loop is to lookup values from the db
            for (rc = host->alarms; rc; rc = rc->next) {

                if (update_disabled_silenced(host, rc))
                    continue;

                // create an alert removed event if the chart is obsolete and
                // has stopped being collected for 60 seconds
                if (unlikely(rc->rrdset && rc->status != RRDCALC_STATUS_REMOVED &&
                             rrdset_flag_check(rc->rrdset, RRDSET_FLAG_OBSOLETE) &&
                             now > (rc->rrdset->last_collected_time.tv_sec + 60))) {
                    if (!rrdcalc_isrepeating(rc)) {
                        worker_is_busy(WORKER_HEALTH_JOB_ALARM_LOG_ENTRY);
                        time_t now = now_realtime_sec();

                        ALARM_ENTRY *ae = health_create_alarm_entry(
                            host,
                            rc->id,
                            rc->next_event_id++,
                            rc->config_hash_id,
                            now,
                            rc->name,
                            rc->rrdset->id,
                            rc->rrdset->context,
                            rc->rrdset->family,
                            rc->classification,
                            rc->component,
                            rc->type,
                            rc->exec,
                            rc->recipient,
                            now - rc->last_status_change,
                            rc->value,
                            NAN,
                            rc->status,
                            RRDCALC_STATUS_REMOVED,
                            rc->source,
                            rc->units,
                            rc->info,
                            0,
                            0);

                        if (ae) {
                            health_alarm_log(host, ae);
                            rc->old_status = rc->status;
                            rc->status = RRDCALC_STATUS_REMOVED;
                            rc->last_status_change = now;
                            rc->last_updated = now;
                            rc->value = NAN;
#ifdef ENABLE_ACLK
                            if (netdata_cloud_setting && likely(!aclk_alert_reloaded))
                                sql_queue_alarm_to_aclk(host, ae, 1);
#endif
                        }
                    }
                }

                if (unlikely(!rrdcalc_isrunnable(rc, now, &next_run))) {
                    if (unlikely(rc->rrdcalc_flags & RRDCALC_FLAG_RUNNABLE))
                        rc->rrdcalc_flags &= ~RRDCALC_FLAG_RUNNABLE;
                    continue;
                }

                runnable++;
                rc->old_value = rc->value;
                rc->rrdcalc_flags |= RRDCALC_FLAG_RUNNABLE;

                // ------------------------------------------------------------
                // if there is database lookup, do it

                if (unlikely(RRDCALC_HAS_DB_LOOKUP(rc))) {
                    worker_is_busy(WORKER_HEALTH_JOB_DB_QUERY);

                    /* time_t old_db_timestamp = rc->db_before; */
                    int value_is_null = 0;

                    int ret = rrdset2value_api_v1(rc->rrdset, NULL, &rc->value, rrdcalc_dimensions(rc), 1,
                                                  rc->after, rc->before, rc->group, NULL,
                                                  0, rc->options,
                                                  &rc->db_after,&rc->db_before,
                                                  NULL, NULL, NULL,
                                                  &value_is_null, NULL, 0, 0);

                    if (unlikely(ret != 200)) {
                        // database lookup failed
                        rc->value = NAN;
                        rc->rrdcalc_flags |= RRDCALC_FLAG_DB_ERROR;

                        debug(D_HEALTH, "Health on host '%s', alarm '%s.%s': database lookup returned error %d",
                              rrdhost_hostname(host), rrdcalc_chart_name(rc), rrdcalc_name(rc), ret
                        );
                    } else
                        rc->rrdcalc_flags &= ~RRDCALC_FLAG_DB_ERROR;

                    /* - RRDCALC_FLAG_DB_STALE not currently used
                    if (unlikely(old_db_timestamp == rc->db_before)) {
                        // database is stale

                        debug(D_HEALTH, "Health on host '%s', alarm '%s.%s': database is stale", host->hostname, rc->chart?rc->chart:"NOCHART", rc->name);

                        if (unlikely(!(rc->rrdcalc_flags & RRDCALC_FLAG_DB_STALE))) {
                            rc->rrdcalc_flags |= RRDCALC_FLAG_DB_STALE;
                            error("Health on host '%s', alarm '%s.%s': database is stale", host->hostname, rc->chart?rc->chart:"NOCHART", rc->name);
                        }
                    }
                    else if (unlikely(rc->rrdcalc_flags & RRDCALC_FLAG_DB_STALE))
                        rc->rrdcalc_flags &= ~RRDCALC_FLAG_DB_STALE;
                    */

                    if (unlikely(value_is_null)) {
                        // collected value is null
                        rc->value = NAN;
                        rc->rrdcalc_flags |= RRDCALC_FLAG_DB_NAN;

                        debug(D_HEALTH,
                              "Health on host '%s', alarm '%s.%s': database lookup returned empty value (possibly value is not collected yet)",
                              rrdhost_hostname(host), rrdcalc_chart_name(rc), rrdcalc_name(rc)
                        );
                    } else
                        rc->rrdcalc_flags &= ~RRDCALC_FLAG_DB_NAN;

                    debug(D_HEALTH, "Health on host '%s', alarm '%s.%s': database lookup gave value " NETDATA_DOUBLE_FORMAT,
                          rrdhost_hostname(host), rrdcalc_chart_name(rc), rrdcalc_name(rc), rc->value
                    );
                }

                // ------------------------------------------------------------
                // if there is calculation expression, run it

                if (unlikely(rc->calculation)) {
                    worker_is_busy(WORKER_HEALTH_JOB_CALC_EVAL);

                    if (unlikely(!expression_evaluate(rc->calculation))) {
                        // calculation failed
                        rc->value = NAN;
                        rc->rrdcalc_flags |= RRDCALC_FLAG_CALC_ERROR;

                        debug(D_HEALTH, "Health on host '%s', alarm '%s.%s': expression '%s' failed: %s",
                              rrdhost_hostname(host), rrdcalc_chart_name(rc), rrdcalc_name(rc),
                              rc->calculation->parsed_as, buffer_tostring(rc->calculation->error_msg)
                        );
                    } else {
                        rc->rrdcalc_flags &= ~RRDCALC_FLAG_CALC_ERROR;

                        debug(D_HEALTH, "Health on host '%s', alarm '%s.%s': expression '%s' gave value "
                              NETDATA_DOUBLE_FORMAT
                              ": %s (source: %s)", rrdhost_hostname(host), rrdcalc_chart_name(rc), rrdcalc_name(rc),
                              rc->calculation->parsed_as, rc->calculation->result,
                              buffer_tostring(rc->calculation->error_msg), rrdcalc_source(rc)
                        );

                        rc->value = rc->calculation->result;

                        if (rc->local) rc->local->last_updated = now;
                        if (rc->family) rc->family->last_updated = now;
                        if (rc->hostid) rc->hostid->last_updated = now;
                        if (rc->hostname) rc->hostname->last_updated = now;
                    }
                }
            }

            rrdhost_unlock(host);

            if (unlikely(runnable && !netdata_exit)) {
                rrdhost_rdlock(host);

                for (rc = host->alarms; rc; rc = rc->next) {
                    if (unlikely(!(rc->rrdcalc_flags & RRDCALC_FLAG_RUNNABLE)))
                        continue;

                    if (rc->rrdcalc_flags & RRDCALC_FLAG_DISABLED) {
                        continue;
                    }
                    RRDCALC_STATUS warning_status = RRDCALC_STATUS_UNDEFINED;
                    RRDCALC_STATUS critical_status = RRDCALC_STATUS_UNDEFINED;

                    // --------------------------------------------------------
                    // check the warning expression

                    if (likely(rc->warning)) {
                        worker_is_busy(WORKER_HEALTH_JOB_WARNING_EVAL);

                        if (unlikely(!expression_evaluate(rc->warning))) {
                            // calculation failed
                            rc->rrdcalc_flags |= RRDCALC_FLAG_WARN_ERROR;

                            debug(D_HEALTH,
                                  "Health on host '%s', alarm '%s.%s': warning expression failed with error: %s",
                                  rrdhost_hostname(host), rrdcalc_chart_name(rc), rrdcalc_name(rc),
                                  buffer_tostring(rc->warning->error_msg)
                            );
                        } else {
                            rc->rrdcalc_flags &= ~RRDCALC_FLAG_WARN_ERROR;
                            debug(D_HEALTH, "Health on host '%s', alarm '%s.%s': warning expression gave value "
                                  NETDATA_DOUBLE_FORMAT
                                  ": %s (source: %s)", rrdhost_hostname(host), rrdcalc_chart_name(rc),
                                  rrdcalc_name(rc), rc->warning->result, buffer_tostring(rc->warning->error_msg), rrdcalc_source(rc)
                            );
                            warning_status = rrdcalc_value2status(rc->warning->result);
                        }
                    }

                    // --------------------------------------------------------
                    // check the critical expression

                    if (likely(rc->critical)) {
                        worker_is_busy(WORKER_HEALTH_JOB_CRITICAL_EVAL);

                        if (unlikely(!expression_evaluate(rc->critical))) {
                            // calculation failed
                            rc->rrdcalc_flags |= RRDCALC_FLAG_CRIT_ERROR;

                            debug(D_HEALTH,
                                  "Health on host '%s', alarm '%s.%s': critical expression failed with error: %s",
                                  rrdhost_hostname(host), rrdcalc_chart_name(rc), rrdcalc_name(rc),
                                  buffer_tostring(rc->critical->error_msg)
                            );
                        } else {
                            rc->rrdcalc_flags &= ~RRDCALC_FLAG_CRIT_ERROR;
                            debug(D_HEALTH, "Health on host '%s', alarm '%s.%s': critical expression gave value "
                                  NETDATA_DOUBLE_FORMAT
                                  ": %s (source: %s)", rrdhost_hostname(host), rrdcalc_chart_name(rc),
                                  rrdcalc_name(rc), rc->critical->result, buffer_tostring(rc->critical->error_msg),
                                  rrdcalc_source(rc)
                            );
                            critical_status = rrdcalc_value2status(rc->critical->result);
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
                        int delay = 0;

                        // apply trigger hysteresis

                        if (now > rc->delay_up_to_timestamp) {
                            rc->delay_up_current = rc->delay_up_duration;
                            rc->delay_down_current = rc->delay_down_duration;
                            rc->delay_last = 0;
                            rc->delay_up_to_timestamp = 0;
                        } else {
                            rc->delay_up_current = (int) (rc->delay_up_current * rc->delay_multiplier);
                            if (rc->delay_up_current > rc->delay_max_duration)
                                rc->delay_up_current = rc->delay_max_duration;

                            rc->delay_down_current = (int) (rc->delay_down_current * rc->delay_multiplier);
                            if (rc->delay_down_current > rc->delay_max_duration)
                                rc->delay_down_current = rc->delay_max_duration;
                        }

                        if (status > rc->status)
                            delay = rc->delay_up_current;
                        else
                            delay = rc->delay_down_current;

                        // COMMENTED: because we do need to send raising alarms
                        // if(now + delay < rc->delay_up_to_timestamp)
                        //      delay = (int)(rc->delay_up_to_timestamp - now);

                        rc->delay_last = delay;
                        rc->delay_up_to_timestamp = now + delay;


                        ALARM_ENTRY *ae = health_create_alarm_entry(
                            host,
                            rc->id,
                            rc->next_event_id++,
                            rc->config_hash_id,
                            now,
                            rc->name,
                            rc->rrdset->id,
                            rc->rrdset->context,
                            rc->rrdset->family,
                            rc->classification,
                            rc->component,
                            rc->type,
                            rc->exec,
                            rc->recipient,
                            now - rc->last_status_change,
                            rc->old_value,
                            rc->value,
                            rc->status,
                            status,
                            rc->source,
                            rc->units,
                            rc->info,
                            rc->delay_last,
                            (
                                ((rc->options & RRDCALC_FLAG_NO_CLEAR_NOTIFICATION)? HEALTH_ENTRY_FLAG_NO_CLEAR_NOTIFICATION : 0) |
                                ((rc->rrdcalc_flags & RRDCALC_FLAG_SILENCED)? HEALTH_ENTRY_FLAG_SILENCED : 0)
                                )
                        );

                        health_alarm_log(host, ae);

                        rc->last_status_change = now;
                        rc->old_status = rc->status;
                        rc->status = status;
                    }

                    rc->last_updated = now;
                    rc->next_update = now + rc->update_every;

                    if (next_run > rc->next_update)
                        next_run = rc->next_update;
                }

                // process repeating alarms
                RRDCALC *rc;
                for(rc = host->alarms; rc ; rc = rc->next) {
                    int repeat_every = 0;
                    if(unlikely(rrdcalc_isrepeating(rc) && rc->delay_up_to_timestamp <= now)) {
                        if(unlikely(rc->status == RRDCALC_STATUS_WARNING)) {
                            rc->rrdcalc_flags &= ~RRDCALC_FLAG_RUN_ONCE;
                            repeat_every = rc->warn_repeat_every;
                        } else if(unlikely(rc->status == RRDCALC_STATUS_CRITICAL)) {
                            rc->rrdcalc_flags &= ~RRDCALC_FLAG_RUN_ONCE;
                            repeat_every = rc->crit_repeat_every;
                        } else if(unlikely(rc->status == RRDCALC_STATUS_CLEAR)) {
                            if(!(rc->rrdcalc_flags & RRDCALC_FLAG_RUN_ONCE)) {
                                if(rc->old_status == RRDCALC_STATUS_CRITICAL) {
                                    repeat_every = 1;
                                } else if (rc->old_status == RRDCALC_STATUS_WARNING) {
                                    repeat_every = 1;
                                }
                            }
                        }
                    } else {
                        continue;
                    }

                    if(unlikely(repeat_every > 0 && (rc->last_repeat + repeat_every) <= now)) {
                        worker_is_busy(WORKER_HEALTH_JOB_ALARM_LOG_ENTRY);
                        rc->last_repeat = now;
                        if (likely(rc->times_repeat < UINT32_MAX)) rc->times_repeat++;

                        ALARM_ENTRY *ae = health_create_alarm_entry(
                            host,
                            rc->id,
                            rc->next_event_id++,
                            rc->config_hash_id,
                            now,
                            rc->name,
                            rc->rrdset->id,
                            rc->rrdset->context,
                            rc->rrdset->family,
                            rc->classification,
                            rc->component,
                            rc->type,
                            rc->exec,
                            rc->recipient,
                            now - rc->last_status_change,
                            rc->old_value,
                            rc->value,
                            rc->old_status,
                            rc->status,
                            rc->source,
                            rc->units,
                            rc->info,
                            rc->delay_last,
                            (
                                ((rc->options & RRDCALC_FLAG_NO_CLEAR_NOTIFICATION)? HEALTH_ENTRY_FLAG_NO_CLEAR_NOTIFICATION : 0) |
                                ((rc->rrdcalc_flags & RRDCALC_FLAG_SILENCED)? HEALTH_ENTRY_FLAG_SILENCED : 0)
                                )
                        );

                        ae->last_repeat = rc->last_repeat;
                        if (!(rc->rrdcalc_flags & RRDCALC_FLAG_RUN_ONCE) && rc->status == RRDCALC_STATUS_CLEAR) {
                            ae->flags |= HEALTH_ENTRY_RUN_ONCE;
                        }
                        rc->rrdcalc_flags |= RRDCALC_FLAG_RUN_ONCE;
                        health_process_notifications(host, ae);
                        debug(D_HEALTH, "Notification sent for the repeating alarm %u.", ae->alarm_id);
                        health_alarm_wait_for_execution(ae);
                        health_alarm_log_free_one_nochecks_nounlink(ae);
                    }
                }

                rrdhost_unlock(host);
            }

            if (unlikely(netdata_exit))
                break;

            // execute notifications
            // and cleanup
            worker_is_busy(WORKER_HEALTH_JOB_ALARM_LOG_PROCESS);
            health_alarm_log_process(host);

            if (unlikely(netdata_exit)) {
                // wait for all notifications to finish before allowing health to be cleaned up
                ALARM_ENTRY *ae;
                while (NULL != (ae = alarm_notifications_in_progress.head)) {
                    health_alarm_wait_for_execution(ae);
                }
                break;
            }

        } /* rrdhost_foreach */

        // wait for all notifications to finish before allowing health to be cleaned up
        ALARM_ENTRY *ae;
        while (NULL != (ae = alarm_notifications_in_progress.head)) {
            health_alarm_wait_for_execution(ae);
        }

#ifdef ENABLE_ACLK
        if (netdata_cloud_setting && unlikely(aclk_alert_reloaded) && loop > (marked_aclk_reload_loop + 2)) {
                rrdhost_foreach_read(host) {
                    if (unlikely(!host->health_enabled))
                        continue;
                    sql_queue_removed_alerts_to_aclk(host);
                }
                aclk_alert_reloaded = 0;
                marked_aclk_reload_loop = 0;
            }
#endif

        rrd_unlock();

        if(unlikely(netdata_exit))
            break;

        now = now_realtime_sec();
        if(now < next_run) {
            worker_is_idle();
            debug(D_HEALTH, "Health monitoring iteration no %u done. Next iteration in %d secs", loop, (int) (next_run - now));
            sleep_usec(USEC_PER_SEC * (usec_t) (next_run - now));
            now = now_realtime_sec();
        }
        else
            debug(D_HEALTH, "Health monitoring iteration no %u done. Next iteration now", loop);

    } // forever

    netdata_thread_cleanup_pop(1);
    return NULL;
}
