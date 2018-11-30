// SPDX-License-Identifier: GPL-3.0-or-later

#include "health.h"

unsigned int default_health_enabled = 1;

// ----------------------------------------------------------------------------
// health initialization

inline char *health_user_config_dir(void) {
    char buffer[FILENAME_MAX + 1];
    snprintfz(buffer, FILENAME_MAX, "%s/health.d", netdata_configured_user_config_dir);
    return config_get(CONFIG_SECTION_HEALTH, "health configuration directory", buffer);
}

inline char *health_stock_config_dir(void) {
    char buffer[FILENAME_MAX + 1];
    snprintfz(buffer, FILENAME_MAX, "%s/health.d", netdata_configured_stock_config_dir);
    return config_get(CONFIG_SECTION_HEALTH, "stock health configuration directory", buffer);
}

void health_init(void) {
    debug(D_HEALTH, "Health configuration initializing");

    if(!(default_health_enabled = (unsigned int)config_get_boolean(CONFIG_SECTION_HEALTH, "enabled", default_health_enabled))) {
        debug(D_HEALTH, "Health is disabled.");
        return;
    }
}

// ----------------------------------------------------------------------------
// re-load health configuration

void health_reload_host(RRDHOST *host) {
    if(unlikely(!host->health_enabled))
        return;

    char *user_path = health_user_config_dir();
    char *stock_path = health_stock_config_dir();

    // free all running alarms
    rrdhost_wrlock(host);

    while(host->templates)
        rrdcalctemplate_unlink_and_free(host, host->templates);

    while(host->alarms)
        rrdcalc_unlink_and_free(host, host->alarms);

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

    // link the loaded alarms to their charts
    rrdset_foreach_write(st, host) {
        rrdsetcalc_link_matching(st);
        rrdcalctemplate_link_matching(st);
    }

    rrdhost_unlock(host);
}

void health_reload(void) {

    rrd_rdlock();

    RRDHOST *host;
    rrdhost_foreach_read(host)
        health_reload_host(host);

    rrd_unlock();
}

// ----------------------------------------------------------------------------
// health main thread and friends

static inline RRDCALC_STATUS rrdcalc_value2status(calculated_number n) {
    if(isnan(n) || isinf(n)) return RRDCALC_STATUS_UNDEFINED;
    if(n) return RRDCALC_STATUS_RAISED;
    return RRDCALC_STATUS_CLEAR;
}

#define ALARM_EXEC_COMMAND_LENGTH 8192

static inline void health_alarm_execute(RRDHOST *host, ALARM_ENTRY *ae) {
    ae->flags |= HEALTH_ENTRY_FLAG_PROCESSED;

    if(unlikely(ae->new_status < RRDCALC_STATUS_CLEAR)) {
        // do not send notifications for internal statuses
        debug(D_HEALTH, "Health not sending notification for alarm '%s.%s' status %s (internal statuses)", ae->chart, ae->name, rrdcalc_status2string(ae->new_status));
        goto done;
    }

    if(unlikely(ae->new_status <= RRDCALC_STATUS_CLEAR && (ae->flags & HEALTH_ENTRY_FLAG_NO_CLEAR_NOTIFICATION))) {
        // do not send notifications for disabled statuses
        debug(D_HEALTH, "Health not sending notification for alarm '%s.%s' status %s (it has no-clear-notification enabled)", ae->chart, ae->name, rrdcalc_status2string(ae->new_status));
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
                debug(D_HEALTH, "Health not sending again notification for alarm '%s.%s' status %s", ae->chart, ae->name
                      , rrdcalc_status2string(ae->new_status));
                goto done;
            }
        }
        else {
            // we have not executed this alarm notification in the past
            // so, don't send CLEAR notifications
            if(unlikely(ae->new_status == RRDCALC_STATUS_CLEAR)) {
                debug(D_HEALTH, "Health not sending notification for first initialization of alarm '%s.%s' status %s"
                      , ae->chart, ae->name, rrdcalc_status2string(ae->new_status));
                goto done;
            }
        }
    }

    static char command_to_run[ALARM_EXEC_COMMAND_LENGTH + 1];
    pid_t command_pid;

    const char *exec      = (ae->exec)      ? ae->exec      : host->health_default_exec;
    const char *recipient = (ae->recipient) ? ae->recipient : host->health_default_recipient;

    snprintfz(command_to_run, ALARM_EXEC_COMMAND_LENGTH, "exec %s '%s' '%s' '%u' '%u' '%u' '%lu' '%s' '%s' '%s' '%s' '%s' '" CALCULATED_NUMBER_FORMAT_ZERO "' '" CALCULATED_NUMBER_FORMAT_ZERO "' '%s' '%u' '%u' '%s' '%s' '%s' '%s'",
              exec,
              recipient,
              host->registry_hostname,
              ae->unique_id,
              ae->alarm_id,
              ae->alarm_event_id,
              (unsigned long)ae->when,
              ae->name,
              ae->chart?ae->chart:"NOCAHRT",
              ae->family?ae->family:"NOFAMILY",
              rrdcalc_status2string(ae->new_status),
              rrdcalc_status2string(ae->old_status),
              ae->new_value,
              ae->old_value,
              ae->source?ae->source:"UNKNOWN",
              (uint32_t)ae->duration,
              (uint32_t)ae->non_clear_duration,
              ae->units?ae->units:"",
              ae->info?ae->info:"",
              ae->new_value_string,
              ae->old_value_string
    );

    ae->flags |= HEALTH_ENTRY_FLAG_EXEC_RUN;
    ae->exec_run_timestamp = now_realtime_sec();

    debug(D_HEALTH, "executing command '%s'", command_to_run);
    FILE *fp = mypopen(command_to_run, &command_pid);
    if(!fp) {
        error("HEALTH: Cannot popen(\"%s\", \"r\").", command_to_run);
        goto done;
    }
    debug(D_HEALTH, "HEALTH reading from command (discarding command's output)");
    char buffer[100 + 1];
    while(fgets(buffer, 100, fp) != NULL) ;
    ae->exec_code = mypclose(fp, command_pid);
    debug(D_HEALTH, "done executing command - returned with code %d", ae->exec_code);

    if(ae->exec_code != 0)
        ae->flags |= HEALTH_ENTRY_FLAG_EXEC_FAILED;

done:
    health_alarm_log_save(host, ae);
}

static inline void health_process_notifications(RRDHOST *host, ALARM_ENTRY *ae) {
    debug(D_HEALTH, "Health alarm '%s.%s' = " CALCULATED_NUMBER_FORMAT_AUTO " - changed status from %s to %s",
         ae->chart?ae->chart:"NOCHART", ae->name,
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
    for(ae = host->health_log.alarms; ae && ae->unique_id >= host->health_last_processed_id ; ae = ae->next) {
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

    // remember this for the next iteration
    host->health_last_processed_id = first_waiting;

    netdata_rwlock_unlock(&host->health_log.alarm_log_rwlock);

    if(host->health_log.count <= host->health_log.max)
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

        health_alarm_log_free_one_nochecks_nounlink(ae);

        ae = t;
        host->health_log.count--;
    }

    netdata_rwlock_unlock(&host->health_log.alarm_log_rwlock);
}

static inline int rrdcalc_isrunnable(RRDCALC *rc, usec_t now_usec, usec_t *next_run_usec) {
    if(unlikely(!rc->rrdset)) {
        debug(D_HEALTH, "Health not running alarm '%s.%s'. It is not linked to a chart.", rc->chart?rc->chart:"NOCHART", rc->name);
        return 0;
    }

    if(unlikely(rc->next_update_usec > now_usec)) {
        if (unlikely(*next_run_usec > rc->next_update_usec)) {
            // update the next_run time of the main loop
            // to run this alarm precisely the time required
            *next_run_usec = rc->next_update_usec;
        }

        debug(D_HEALTH, "Health not examining alarm '%s.%s' yet (will do in %llu usec).", rc->chart?rc->chart:"NOCHART", rc->name,  (rc->next_update_usec - now_usec));
        return 0;
    }

    if(unlikely(!rc->update_every_usec)) {
        debug(D_HEALTH, "Health not running alarm '%s.%s'. It does not have an update frequency", rc->chart?rc->chart:"NOCHART", rc->name);
        return 0;
    }

    if(unlikely(rrdset_flag_check(rc->rrdset, RRDSET_FLAG_OBSOLETE))) {
        debug(D_HEALTH, "Health not running alarm '%s.%s'. The chart has been marked as obsolete", rc->chart?rc->chart:"NOCHART", rc->name);
        return 0;
    }

    if(unlikely(!rrdset_flag_check(rc->rrdset, RRDSET_FLAG_ENABLED))) {
        debug(D_HEALTH, "Health not running alarm '%s.%s'. The chart is not enabled", rc->chart?rc->chart:"NOCHART", rc->name);
        return 0;
    }

    if(unlikely(!rc->rrdset->last_collected_time.tv_sec || rc->rrdset->counter_done < 2)) {
        debug(D_HEALTH, "Health not running alarm '%s.%s'. Chart is not fully collected yet.", rc->chart?rc->chart:"NOCHART", rc->name);
        return 0;
    }

    usec_t update_every_usec = rc->rrdset->update_every_usec;
    usec_t first_usec = rrdset_first_entry_usec(rc->rrdset);
    usec_t last_usec = rrdset_last_entry_usec(rc->rrdset);

    if(unlikely(now_usec + update_every_usec < first_usec /* || now - update_every > last */)) {
        debug(D_HEALTH
              , "Health not examining alarm '%s.%s' yet (wanted time is out of bounds - we need %llu but got %llu - %llu)."
              , rc->chart ? rc->chart : "NOCHART", rc->name, now_usec, first_usec, last_usec);
        return 0;
    }

    if(RRDCALC_HAS_DB_LOOKUP(rc)) {
        usec_t needed_usec = now_usec + rc->before_usec + rc->after_usec;

        if(needed_usec + update_every_usec < first_usec || needed_usec - update_every_usec > last_usec) {
            debug(D_HEALTH
                  , "Health not examining alarm '%s.%s' yet (not enough data yet - we need %llu but got %llu - %llu)."
                  , rc->chart ? rc->chart : "NOCHART", rc->name, needed_usec, first_usec, last_usec);
            return 0;
        }
    }

    return 1;
}

static inline int check_if_resumed_from_suspention(void) {
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
    struct netdata_static_thread *static_thread = (struct netdata_static_thread *)ptr;
    static_thread->enabled = NETDATA_MAIN_THREAD_EXITING;

    info("cleaning up...");

    static_thread->enabled = NETDATA_MAIN_THREAD_EXITED;
}

void *health_main(void *ptr) {
    netdata_thread_cleanup_push(health_main_cleanup, ptr);

    usec_t min_run_every_usec = config_get_usec(CONFIG_SECTION_HEALTH, "run at least every seconds", 10 * USEC_PER_SEC);
    if(min_run_every_usec < USEC_PER_SEC) min_run_every_usec = USEC_PER_SEC;

    usec_t now_usec                = now_realtime_usec();
    usec_t hibernation_delay_usec  = config_get_usec(CONFIG_SECTION_HEALTH, "postpone alarms during hibernation for seconds", 60);

    unsigned int loop = 0;
    while(!netdata_exit) {
        loop++;
        debug(D_HEALTH, "Health monitoring iteration no %u started", loop);

        int runnable = 0, apply_hibernation_delay = 0;
        usec_t next_run_usec = now_usec + min_run_every_usec;
        RRDCALC *rc;

        if(unlikely(check_if_resumed_from_suspention())) {
            apply_hibernation_delay = 1;

            info("Postponing alarm checks for %llu usec, because it seems that the system was just resumed from suspension."
            , hibernation_delay_usec
            );
        }

        rrd_rdlock();

        RRDHOST *host;
        rrdhost_foreach_read(host) {
            if(unlikely(!host->health_enabled))
                continue;

            if(unlikely(apply_hibernation_delay)) {

                info("Postponing health checks for %llu usec, on host '%s'."
                     , hibernation_delay_usec
                     , host->hostname
                );

                host->health_delay_up_to_usec = now_usec + hibernation_delay_usec;
            }

            if(unlikely(host->health_delay_up_to_usec)) {
                if(unlikely(now_usec < host->health_delay_up_to_usec))
                    continue;

                info("Resuming health checks on host '%s'.", host->hostname);
                host->health_delay_up_to_usec = 0;
            }

            rrdhost_rdlock(host);

            // the first loop is to lookup values from the db
            for(rc = host->alarms; rc; rc = rc->next) {
                if(unlikely(!rrdcalc_isrunnable(rc, now_usec, &next_run_usec))) {
                    if(unlikely(rc->rrdcalc_flags & RRDCALC_FLAG_RUNNABLE))
                        rc->rrdcalc_flags &= ~RRDCALC_FLAG_RUNNABLE;
                    continue;
                }

                runnable++;
                rc->old_value = rc->value;
                rc->rrdcalc_flags |= RRDCALC_FLAG_RUNNABLE;

                // ------------------------------------------------------------
                // if there is database lookup, do it

                if(unlikely(RRDCALC_HAS_DB_LOOKUP(rc))) {
                    /* time_t old_db_timestamp = rc->db_before; */
                    int value_is_null = 0;

                    int ret = rrdset2value_api_v1(rc->rrdset
                                                  , NULL
                                                  , &rc->value
                                                  , rc->dimensions
                                                  , 1
                                                  , rc->after_usec
                                                  , rc->before_usec
                                                  , rc->group
                                                  , 0
                                                  , rc->options
                                                  , &rc->db_after_usec
                                                  , &rc->db_before_usec
                                                  , &value_is_null
                    );

                    if(unlikely(ret != 200)) {
                        // database lookup failed
                        rc->value = NAN;
                        rc->rrdcalc_flags |= RRDCALC_FLAG_DB_ERROR;

                        debug(D_HEALTH
                              , "Health on host '%s', alarm '%s.%s': database lookup returned error %d"
                              , host->hostname
                              , rc->chart ? rc->chart : "NOCHART"
                              , rc->name
                              , ret
                        );
                    }
                    else
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

                    if(unlikely(value_is_null)) {
                        // collected value is null
                        rc->value = NAN;
                        rc->rrdcalc_flags |= RRDCALC_FLAG_DB_NAN;

                        debug(D_HEALTH
                              , "Health on host '%s', alarm '%s.%s': database lookup returned empty value (possibly value is not collected yet)"
                              , host->hostname
                              , rc->chart ? rc->chart : "NOCHART"
                              , rc->name
                        );
                    }
                    else
                        rc->rrdcalc_flags &= ~RRDCALC_FLAG_DB_NAN;

                    debug(D_HEALTH
                          , "Health on host '%s', alarm '%s.%s': database lookup gave value " CALCULATED_NUMBER_FORMAT
                          , host->hostname
                          , rc->chart ? rc->chart : "NOCHART"
                          , rc->name
                          , rc->value
                    );
                }

                // ------------------------------------------------------------
                // if there is calculation expression, run it

                if(unlikely(rc->calculation)) {
                    if(unlikely(!expression_evaluate(rc->calculation))) {
                        // calculation failed
                        rc->value = NAN;
                        rc->rrdcalc_flags |= RRDCALC_FLAG_CALC_ERROR;

                        debug(D_HEALTH
                              , "Health on host '%s', alarm '%s.%s': expression '%s' failed: %s"
                              , host->hostname
                              , rc->chart ? rc->chart : "NOCHART"
                              , rc->name
                              , rc->calculation->parsed_as
                              , buffer_tostring(rc->calculation->error_msg)
                        );
                    }
                    else {
                        rc->rrdcalc_flags &= ~RRDCALC_FLAG_CALC_ERROR;

                        debug(D_HEALTH, "Health on host '%s', alarm '%s.%s': expression '%s' gave value " CALCULATED_NUMBER_FORMAT ": %s (source: %s)"
                              , host->hostname
                              , rc->chart ? rc->chart : "NOCHART"
                              , rc->name
                              , rc->calculation->parsed_as
                              , rc->calculation->result
                              , buffer_tostring(rc->calculation->error_msg)
                              , rc->source
                        );

                        rc->value = rc->calculation->result;

                        if(rc->local) rc->local->last_updated_usec = now_usec;
                        if(rc->family) rc->family->last_updated_usec = now_usec;
                        if(rc->hostid) rc->hostid->last_updated_usec = now_usec;
                        if(rc->hostname) rc->hostname->last_updated_usec = now_usec;
                    }
                }
            }
            rrdhost_unlock(host);

            if(unlikely(runnable && !netdata_exit)) {
                rrdhost_rdlock(host);

                for(rc = host->alarms; rc; rc = rc->next) {
                    if(unlikely(!(rc->rrdcalc_flags & RRDCALC_FLAG_RUNNABLE)))
                        continue;

                    RRDCALC_STATUS warning_status  = RRDCALC_STATUS_UNDEFINED;
                    RRDCALC_STATUS critical_status = RRDCALC_STATUS_UNDEFINED;

                    // --------------------------------------------------------
                    // check the warning expression

                    if(likely(rc->warning)) {
                        if(unlikely(!expression_evaluate(rc->warning))) {
                            // calculation failed
                            rc->rrdcalc_flags |= RRDCALC_FLAG_WARN_ERROR;

                            debug(D_HEALTH
                                  , "Health on host '%s', alarm '%s.%s': warning expression failed with error: %s"
                                  , host->hostname
                                  , rc->chart ? rc->chart : "NOCHART"
                                  , rc->name
                                  , buffer_tostring(rc->warning->error_msg)
                            );
                        }
                        else {
                            rc->rrdcalc_flags &= ~RRDCALC_FLAG_WARN_ERROR;
                            debug(D_HEALTH
                                  , "Health on host '%s', alarm '%s.%s': warning expression gave value " CALCULATED_NUMBER_FORMAT ": %s (source: %s)"
                                  , host->hostname
                                  , rc->chart ? rc->chart : "NOCHART"
                                  , rc->name
                                  , rc->warning->result
                                  , buffer_tostring(rc->warning->error_msg)
                                  , rc->source
                            );
                            warning_status = rrdcalc_value2status(rc->warning->result);
                        }
                    }

                    // --------------------------------------------------------
                    // check the critical expression

                    if(likely(rc->critical)) {
                        if(unlikely(!expression_evaluate(rc->critical))) {
                            // calculation failed
                            rc->rrdcalc_flags |= RRDCALC_FLAG_CRIT_ERROR;

                            debug(D_HEALTH
                                  , "Health on host '%s', alarm '%s.%s': critical expression failed with error: %s"
                                  , host->hostname
                                  , rc->chart ? rc->chart : "NOCHART"
                                  , rc->name
                                  , buffer_tostring(rc->critical->error_msg)
                            );
                        }
                        else {
                            rc->rrdcalc_flags &= ~RRDCALC_FLAG_CRIT_ERROR;
                            debug(D_HEALTH
                                  , "Health on host '%s', alarm '%s.%s': critical expression gave value " CALCULATED_NUMBER_FORMAT ": %s (source: %s)"
                                  , host->hostname
                                  , rc->chart ? rc->chart : "NOCHART"
                                  , rc->name
                                  , rc->critical->result
                                  , buffer_tostring(rc->critical->error_msg)
                                  , rc->source
                            );
                            critical_status = rrdcalc_value2status(rc->critical->result);
                        }
                    }

                    // --------------------------------------------------------
                    // decide the final alarm status

                    RRDCALC_STATUS status = RRDCALC_STATUS_UNDEFINED;

                    switch(warning_status) {
                        case RRDCALC_STATUS_CLEAR:
                            status = RRDCALC_STATUS_CLEAR;
                            break;

                        case RRDCALC_STATUS_RAISED:
                            status = RRDCALC_STATUS_WARNING;
                            break;

                        default:
                            break;
                    }

                    switch(critical_status) {
                        case RRDCALC_STATUS_CLEAR:
                            if(status == RRDCALC_STATUS_UNDEFINED)
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

                    if(status != rc->status) {
                        int delay = 0;

                        // apply trigger hysteresis

                        if(now_usec > rc->delay_up_to_timestamp_usec) {
                            rc->delay_up_current_usec = rc->delay_up_usec;
                            rc->delay_down_current_usec = rc->delay_down_usec;
                            rc->delay_last_usec = 0;
                            rc->delay_up_to_timestamp_usec = 0;
                        }
                        else {
                            rc->delay_up_current_usec = (int) (rc->delay_up_current_usec * rc->delay_multiplier);
                            if(rc->delay_up_current_usec > rc->delay_max_usec)
                                rc->delay_up_current_usec = rc->delay_max_usec;

                            rc->delay_down_current_usec = (int) (rc->delay_down_current_usec * rc->delay_multiplier);
                            if(rc->delay_down_current_usec > rc->delay_max_usec)
                                rc->delay_down_current_usec = rc->delay_max_usec;
                        }

                        if(status > rc->status)
                            delay = rc->delay_up_current_usec;
                        else
                            delay = rc->delay_down_current_usec;

                        // COMMENTED: because we do need to send raising alarms
                        // if(now + delay < rc->delay_up_to_timestamp)
                        //    delay = (int)(rc->delay_up_to_timestamp - now);

                        rc->delay_last_usec = delay;
                        rc->delay_up_to_timestamp_usec = now_usec + delay;

                        // add the alarm into the log

                        health_alarm_log(
                                host
                                , rc->id
                                , rc->next_event_id++
                                , now_usec
                                , rc->name
                                , rc->rrdset->id
                                , rc->rrdset->family
                                , rc->exec
                                , rc->recipient
                                , now_usec - rc->last_status_change_usec
                                , rc->old_value
                                , rc->value
                                , rc->status
                                , status
                                , rc->source
                                , rc->units
                                , rc->info
                                , rc->delay_last_usec
                                , (rc->options & RRDCALC_FLAG_NO_CLEAR_NOTIFICATION) ? HEALTH_ENTRY_FLAG_NO_CLEAR_NOTIFICATION : 0
                        );

                        rc->last_status_change_usec = now_usec;
                        rc->status = status;
                    }

                    rc->last_updated_usec = now_usec;
                    rc->next_update_usec = now_usec + rc->update_every_usec;

                    if(next_run_usec > rc->next_update_usec)
                        next_run_usec = rc->next_update_usec;
                }

                rrdhost_unlock(host);
            }

            if(unlikely(netdata_exit))
                break;

            // execute notifications
            // and cleanup
            health_alarm_log_process(host);

            if(unlikely(netdata_exit))
                break;

        } /* rrdhost_foreach */

        rrd_unlock();

        if(unlikely(netdata_exit))
            break;

        now_usec = now_realtime_usec();
        if(now_usec < next_run_usec) {
            debug(D_HEALTH, "Health monitoring iteration no %u done. Next iteration in %llu usec", loop, (next_run_usec - now_usec));
            sleep_usec(next_run_usec - now_usec);
            now_usec = now_realtime_usec();
        }
        else
            debug(D_HEALTH, "Health monitoring iteration no %u done. Next iteration now", loop);

    } // forever

    netdata_thread_cleanup_pop(1);
    return NULL;
}
