// SPDX-License-Identifier: GPL-3.0-or-later

#include "health.h"

// ----------------------------------------------------------------------------

inline void health_alarm_log_save(RRDHOST *host, ALARM_ENTRY *ae) {
    sql_health_alarm_log_save(host, ae);
}


void health_log_alert_transition_with_trace(RRDHOST *host, ALARM_ENTRY *ae, int line, const char *file, const char *function) {
    ND_LOG_STACK lgs[] = {
            ND_LOG_FIELD_UUID(NDF_MESSAGE_ID, &health_alert_transition_msgid),
            ND_LOG_FIELD_STR(NDF_NIDL_NODE, host->hostname),
            ND_LOG_FIELD_STR(NDF_NIDL_INSTANCE, ae->chart_name),
            ND_LOG_FIELD_STR(NDF_NIDL_CONTEXT, ae->chart_context),
            ND_LOG_FIELD_U64(NDF_ALERT_ID, ae->alarm_id),
            ND_LOG_FIELD_U64(NDF_ALERT_UNIQUE_ID, ae->unique_id),
            ND_LOG_FIELD_U64(NDF_ALERT_EVENT_ID, ae->alarm_event_id),
            ND_LOG_FIELD_UUID(NDF_ALERT_CONFIG_HASH, &ae->config_hash_id),
            ND_LOG_FIELD_UUID(NDF_ALERT_TRANSITION_ID, &ae->transition_id),
            ND_LOG_FIELD_STR(NDF_ALERT_NAME, ae->name),
            ND_LOG_FIELD_STR(NDF_ALERT_CLASS, ae->classification),
            ND_LOG_FIELD_STR(NDF_ALERT_COMPONENT, ae->component),
            ND_LOG_FIELD_STR(NDF_ALERT_TYPE, ae->type),
            ND_LOG_FIELD_STR(NDF_ALERT_EXEC, ae->exec),
            ND_LOG_FIELD_STR(NDF_ALERT_RECIPIENT, ae->recipient),
            ND_LOG_FIELD_STR(NDF_ALERT_SOURCE, ae->exec),
            ND_LOG_FIELD_STR(NDF_ALERT_UNITS, ae->units),
            ND_LOG_FIELD_STR(NDF_ALERT_SUMMARY, ae->summary),
            ND_LOG_FIELD_STR(NDF_ALERT_INFO, ae->info),
            ND_LOG_FIELD_DBL(NDF_ALERT_VALUE, ae->new_value),
            ND_LOG_FIELD_DBL(NDF_ALERT_VALUE_OLD, ae->old_value),
            ND_LOG_FIELD_TXT(NDF_ALERT_STATUS, rrdcalc_status2string(ae->new_status)),
            ND_LOG_FIELD_TXT(NDF_ALERT_STATUS_OLD, rrdcalc_status2string(ae->old_status)),
            ND_LOG_FIELD_I64(NDF_ALERT_DURATION, ae->duration),
            ND_LOG_FIELD_I64(NDF_RESPONSE_CODE, ae->exec_code),
            ND_LOG_FIELD_U64(NDF_ALERT_NOTIFICATION_REALTIME_USEC, ae->delay_up_to_timestamp * USEC_PER_SEC),
            ND_LOG_FIELD_END(),
    };
    ND_LOG_STACK_PUSH(lgs);

    errno = 0;

    ND_LOG_FIELD_PRIORITY priority = NDLP_INFO;

    switch(ae->new_status) {
        case RRDCALC_STATUS_UNDEFINED:
            if(ae->old_status >= RRDCALC_STATUS_CLEAR)
                priority = NDLP_NOTICE;
            else
                priority = NDLP_DEBUG;
            break;

        default:
        case RRDCALC_STATUS_UNINITIALIZED:
        case RRDCALC_STATUS_REMOVED:
            priority = NDLP_DEBUG;
            break;

        case RRDCALC_STATUS_CLEAR:
            priority = NDLP_INFO;
            break;

        case RRDCALC_STATUS_WARNING:
            if(ae->old_status < RRDCALC_STATUS_WARNING)
                priority = NDLP_WARNING;
            break;

        case RRDCALC_STATUS_CRITICAL:
            if(ae->old_status < RRDCALC_STATUS_CRITICAL)
                priority = NDLP_CRIT;
            break;
    }

    netdata_logger(NDLS_HEALTH, priority, file, function, line,
           "ALERT '%s' of instance '%s' on node '%s', transitioned from %s to %s",
           string2str(ae->name), string2str(ae->chart), string2str(host->hostname),
           rrdcalc_status2string(ae->old_status), rrdcalc_status2string(ae->new_status)
           );
}

// ----------------------------------------------------------------------------
// health alarm log management

inline ALARM_ENTRY* health_create_alarm_entry(
    RRDHOST *host,
    RRDCALC *rc,
    time_t when,
    time_t duration,
    NETDATA_DOUBLE old_value,
    NETDATA_DOUBLE new_value,
    RRDCALC_STATUS old_status,
    RRDCALC_STATUS new_status,
    int delay,
    HEALTH_ENTRY_FLAGS flags
) {
    uint32_t alarm_id = rc->id;
    uint32_t alarm_event_id = rc->next_event_id++;
    STRING *name = rc->config.name;
    STRING *chart = rc->rrdset->id;
    STRING *chart_context = rc->rrdset->context;
    STRING *chart_name = rc->rrdset->name;
    STRING *class = rc->config.classification;
    STRING *component = rc->config.component;
    STRING *type = rc->config.type;
    STRING *exec = rc->config.exec;
    STRING *recipient = rc->config.recipient;
    STRING *source = rc->config.source;
    STRING *units = rc->config.units;
    STRING *summary = rc->summary;
    STRING *info = rc->info;

    netdata_log_debug(D_HEALTH, "Health adding alarm log entry with id: %u", host->health_log.next_log_id);

    ALARM_ENTRY *ae = callocz(1, sizeof(ALARM_ENTRY));
    ae->name = string_dup(name);
    ae->chart = string_dup(chart);
    ae->chart_context = string_dup(chart_context);
    ae->chart_name = string_dup(chart_name);

    uuid_copy(ae->config_hash_id, rc->config.hash_id);

    uuid_generate_random(ae->transition_id);
    ae->global_id = now_realtime_usec();

    ae->classification = string_dup(class);
    ae->component = string_dup(component);
    ae->type = string_dup(type);
    ae->exec = string_dup(exec);
    ae->recipient = string_dup(recipient);
    ae->source = string_dup(source);
    ae->units = string_dup(units);

    ae->unique_id = host->health_log.next_log_id++;
    ae->alarm_id = alarm_id;
    ae->alarm_event_id = alarm_event_id;
    ae->when = when;
    ae->old_value = old_value;
    ae->new_value = new_value;

    char value_string[100 + 1];
    ae->old_value_string = string_strdupz(format_value_and_unit(value_string, 100, ae->old_value, ae_units(ae), -1));
    ae->new_value_string = string_strdupz(format_value_and_unit(value_string, 100, ae->new_value, ae_units(ae), -1));

    ae->summary = string_dup(summary);
    ae->info = string_dup(info);
    ae->old_status = old_status;
    ae->new_status = new_status;
    ae->duration = duration;
    ae->delay = delay;
    ae->delay_up_to_timestamp = when + delay;
    ae->flags |= flags;

    ae->last_repeat = 0;

    if(ae->old_status == RRDCALC_STATUS_WARNING || ae->old_status == RRDCALC_STATUS_CRITICAL)
        ae->non_clear_duration += ae->duration;

    return ae;
}

inline void health_alarm_log_add_entry(
        RRDHOST *host,
        ALARM_ENTRY *ae
) {
    netdata_log_debug(D_HEALTH, "Health adding alarm log entry with id: %u", ae->unique_id);

    __atomic_add_fetch(&host->health_transitions, 1, __ATOMIC_RELAXED);

    // link it
    rw_spinlock_write_lock(&host->health_log.spinlock);
    ae->next = host->health_log.alarms;
    host->health_log.alarms = ae;
    host->health_log.count++;
    rw_spinlock_write_unlock(&host->health_log.spinlock);

    // match previous alarms
    rw_spinlock_read_lock(&host->health_log.spinlock);
    ALARM_ENTRY *t;
    for(t = host->health_log.alarms ; t ; t = t->next) {
        if(t != ae && t->alarm_id == ae->alarm_id) {
            if(!(t->flags & HEALTH_ENTRY_FLAG_UPDATED) && !t->updated_by_id) {
                t->flags |= HEALTH_ENTRY_FLAG_UPDATED;
                t->updated_by_id = ae->unique_id;
                ae->updates_id = t->unique_id;

                if((t->new_status == RRDCALC_STATUS_WARNING || t->new_status == RRDCALC_STATUS_CRITICAL) &&
                   (t->old_status == RRDCALC_STATUS_WARNING || t->old_status == RRDCALC_STATUS_CRITICAL))
                    ae->non_clear_duration += t->non_clear_duration;

                health_alarm_log_save(host, t);
            }

            // no need to continue
            break;
        }
    }
    rw_spinlock_read_unlock(&host->health_log.spinlock);

    health_alarm_log_save(host, ae);
}

inline void health_alarm_log_free_one_nochecks_nounlink(ALARM_ENTRY *ae) {
    string_freez(ae->name);
    string_freez(ae->chart);
    string_freez(ae->chart_context);
    string_freez(ae->classification);
    string_freez(ae->component);
    string_freez(ae->type);
    string_freez(ae->exec);
    string_freez(ae->recipient);
    string_freez(ae->source);
    string_freez(ae->units);
    string_freez(ae->info);
    string_freez(ae->old_value_string);
    string_freez(ae->new_value_string);
    freez(ae);
}

inline void health_alarm_log_free(RRDHOST *host) {
    rw_spinlock_write_lock(&host->health_log.spinlock);

    ALARM_ENTRY *ae;
    while((ae = host->health_log.alarms)) {
        host->health_log.alarms = ae->next;
        health_alarm_log_free_one_nochecks_nounlink(ae);
    }

    rw_spinlock_write_unlock(&host->health_log.spinlock);
}
