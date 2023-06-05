// SPDX-License-Identifier: GPL-3.0-or-later

#include "health.h"

// ----------------------------------------------------------------------------

inline void health_alarm_log_save(RRDHOST *host, ALARM_ENTRY *ae) {
    sql_health_alarm_log_save(host, ae);
}

// ----------------------------------------------------------------------------
// health alarm log management

inline ALARM_ENTRY* health_create_alarm_entry(
    RRDHOST *host,
    uint32_t alarm_id,
    uint32_t alarm_event_id,
    const uuid_t config_hash_id,
    time_t when,
    STRING *name,
    STRING *chart,
    STRING *chart_context,
    STRING *family,
    STRING *class,
    STRING *component,
    STRING *type,
    STRING *exec,
    STRING *recipient,
    time_t duration,
    NETDATA_DOUBLE old_value,
    NETDATA_DOUBLE new_value,
    RRDCALC_STATUS old_status,
    RRDCALC_STATUS new_status,
    STRING *source,
    STRING *units,
    STRING *info,
    int delay,
    uint32_t flags
) {
    debug(D_HEALTH, "Health adding alarm log entry with id: %u", host->health_log.next_log_id);

    ALARM_ENTRY *ae = callocz(1, sizeof(ALARM_ENTRY));
    ae->name = string_dup(name);
    ae->chart = string_dup(chart);
    ae->chart_context = string_dup(chart_context);

    uuid_copy(ae->config_hash_id, *((uuid_t *) config_hash_id));

    uuid_generate_random(ae->transition_id);

    ae->family = string_dup(family);
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
    debug(D_HEALTH, "Health adding alarm log entry with id: %u", ae->unique_id);

    __atomic_add_fetch(&host->health_transitions, 1, __ATOMIC_RELAXED);

    // link it
    netdata_rwlock_wrlock(&host->health_log.alarm_log_rwlock);
    ae->next = host->health_log.alarms;
    host->health_log.alarms = ae;
    host->health_log.count++;
    netdata_rwlock_unlock(&host->health_log.alarm_log_rwlock);

    // match previous alarms
    netdata_rwlock_rdlock(&host->health_log.alarm_log_rwlock);
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
    netdata_rwlock_unlock(&host->health_log.alarm_log_rwlock);

    health_alarm_log_save(host, ae);
}

inline void health_alarm_log_free_one_nochecks_nounlink(ALARM_ENTRY *ae) {
    string_freez(ae->name);
    string_freez(ae->chart);
    string_freez(ae->chart_context);
    string_freez(ae->family);
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
    netdata_rwlock_wrlock(&host->health_log.alarm_log_rwlock);

    ALARM_ENTRY *ae;
    while((ae = host->health_log.alarms)) {
        host->health_log.alarms = ae->next;
        health_alarm_log_free_one_nochecks_nounlink(ae);
    }

    netdata_rwlock_unlock(&host->health_log.alarm_log_rwlock);
}
