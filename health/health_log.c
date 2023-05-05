// SPDX-License-Identifier: GPL-3.0-or-later

#include "health.h"

// ----------------------------------------------------------------------------

inline void health_alarm_log_save(RRDHOST *host, ALARM_ENTRY *ae) {
    sql_health_alarm_log_save(host, ae);
}

static inline STRING *alarm_entry_replace_variables(const char *line, RRDCALC *rc) {
    if (!line || !*line)
        return NULL;

    size_t pos = 0;
    char *temp = strdupz(line);
    char var[ALARM_ENTRY_VAR_MAX];
    char *m;

    while ((m = strchr(temp + pos, '$')) && *(m+1) == '{') {
        int i = 0;
        char *e = m;
        while (*e) {
            var[i++] = *e;

            if (*e == '}' || i == ALARM_ENTRY_VAR_MAX - 1)
                break;

            e++;
        }

        var[i] = '\0';
        pos = m - temp + 1;

        if (!strncmp(var, ALARM_ENTRY_VAR_VALUE, ALARM_ENTRY_VAR_VALUE_LEN)) {
            char value_val[ALARM_ENTRY_VAR_MAX + ALARM_ENTRY_VAR_VALUE_LEN + 1] = { 0 };
            strcpy(value_val, var+ALARM_ENTRY_VAR_VALUE_LEN);
            value_val[i - ALARM_ENTRY_VAR_VALUE_LEN - 1] = '\0';

            NETDATA_DOUBLE n;
            if (health_variable_lookup(string_strdupz(value_val), rc, &n)) {
                if(isnan(n) || isinf(n)) {
                    char *buf = find_and_replace(temp, var, "null", m);
                    freez(temp);
                    temp = buf;
                } else {
                    char val_string[100];
                    snprintfz(val_string, 99, "" NETDATA_DOUBLE_FORMAT "", n);
                    char *buf = find_and_replace(temp, var, val_string, m);
                    freez(temp);
                    temp = buf;
                }
            }
        }
    }

    STRING *ret = string_strdupz(temp);
    freez(temp);

    return ret;
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
    STRING *chart_name,
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
    STRING *summary,
    STRING *info,
    int delay,
    HEALTH_ENTRY_FLAGS flags,
    RRDCALC *rc
) {
    netdata_log_debug(D_HEALTH, "Health adding alarm log entry with id: %u", host->health_log.next_log_id);

    ALARM_ENTRY *ae = callocz(1, sizeof(ALARM_ENTRY));
    ae->name = string_dup(name);
    ae->chart = string_dup(chart);
    ae->chart_context = string_dup(chart_context);
    ae->chart_name = string_dup(chart_name);

    uuid_copy(ae->config_hash_id, *((uuid_t *) config_hash_id));

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

    ae->info = alarm_entry_replace_variables(string2str(info), rc);
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
