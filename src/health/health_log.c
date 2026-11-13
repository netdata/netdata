// SPDX-License-Identifier: GPL-3.0-or-later

#include "health.h"
#include "health-alert-entry.h"

// ----------------------------------------------------------------------------
// ARAL memory management for ALARM_ENTRY structures

static struct {
    ARAL *ar;
} health_alarm_entry_globals = {
    .ar = NULL,
};

void health_alarm_entry_aral_init(void) {
    health_alarm_entry_globals.ar = aral_create(
            "health-alarm-entry",
            sizeof(ALARM_ENTRY),
            0,
            0,
            NULL,
            NULL, NULL, false, false, true
    );

    pulse_aral_register(health_alarm_entry_globals.ar, "health_log");
}

struct aral_statistics *health_alarm_entry_aral_stats(void) {
    return aral_get_statistics(health_alarm_entry_globals.ar);
}

ALARM_ENTRY *health_alarm_entry_create(void) {
    ALARM_ENTRY *ae = aral_mallocz(health_alarm_entry_globals.ar);
    memset(ae, 0, sizeof(ALARM_ENTRY));
    return ae;
}

void health_alarm_entry_destroy(ALARM_ENTRY *ae) {
    aral_freez(health_alarm_entry_globals.ar, ae);
}

// ----------------------------------------------------------------------------
extern __thread bool is_health_thread;

static inline void health_alarm_entry_assign_unique_id_unsafe(RRDHOST *host, ALARM_ENTRY *ae) {
    if(unlikely(!ae->unique_id))
        ae->unique_id = host->health_log.next_log_id++;
}

void health_alarm_entry_assign_unique_id(RRDHOST *host, ALARM_ENTRY *ae) {
    if(!host || !ae)
        return;

    rw_spinlock_write_lock(&host->health_log.spinlock);
    health_alarm_entry_assign_unique_id_unsafe(host, ae);
    rw_spinlock_write_unlock(&host->health_log.spinlock);
}

static inline void health_alarm_log_insert_entry_unsafe(RRDHOST *host, ALARM_ENTRY *ae) {
    ALARM_ENTRY *t;

    for(t = host->health_log.alarms ; t && t->unique_id > ae->unique_id ; t = t->next)
        ;

    DOUBLE_LINKED_LIST_INSERT_ITEM_BEFORE_UNSAFE(host->health_log.alarms, t, ae, prev, next);
}

inline void health_alarm_log_save(RRDHOST *host, ALARM_ENTRY *ae, bool async)
{
    if (async) {
        bool queued = metadata_queue_ae_save(host, ae);
        if (!queued && is_health_thread && service_running(SERVICE_HEALTH))
            sql_health_alarm_log_save(host, ae);
    } else
        sql_health_alarm_log_save(host, ae);
}

void health_log_alert_transition_with_trace(RRDHOST *host, ALARM_ENTRY *ae, int line, const char *file, const char *function) {
    if(!host || !ae) return;
    
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

    errno_clear();

    ND_LOG_FIELD_PRIORITY priority = NDLP_INFO;
    const char *emoji = "🌀";
    const char *transitioned = (ae->old_status < ae->new_status) ? "raised" : "lowered";

    switch(ae->new_status) {
        case RRDCALC_STATUS_UNDEFINED:
            emoji = "❓";
            if(ae->old_status >= RRDCALC_STATUS_CLEAR)
                priority = NDLP_NOTICE;
            else
                priority = NDLP_DEBUG;
            break;

        default:
        case RRDCALC_STATUS_UNINITIALIZED:
            emoji = "⏳";
            priority = NDLP_DEBUG;
            break;

        case RRDCALC_STATUS_REMOVED:
            emoji = "🚫";
            priority = NDLP_DEBUG;
            break;

        case RRDCALC_STATUS_CLEAR:
            if(ae->old_status == RRDCALC_STATUS_UNINITIALIZED) {
                emoji = "💚";
                priority = NDLP_DEBUG;
            }
            else if(ae->old_status <= RRDCALC_STATUS_CLEAR) {
                emoji = "✅";
                priority = NDLP_INFO;
            }
            else {
                emoji = "💚";
                priority = NDLP_NOTICE;
            }
            break;

        case RRDCALC_STATUS_WARNING:
            if(ae->old_status <= RRDCALC_STATUS_WARNING) {
                emoji = "⚠️";
                priority = NDLP_WARNING;
            }
            else {
                emoji = "🔶";
                priority = NDLP_INFO;
            }
            break;

        case RRDCALC_STATUS_CRITICAL:
            emoji = "🔴";
            priority = NDLP_CRIT;
            break;
    }

    netdata_logger(NDLS_HEALTH, priority, file, function, line,
                   "ALERT '%s' of '%s' on node '%s', %s from %s to %s.\n"
                   "%s %s on %s.\n"
                   "%s:%s:%s value got from %f %s, to %f %s.",
                   string2str(ae->name), string2str(ae->chart), string2str(host->hostname),
                   transitioned, rrdcalc_status2string(ae->old_status), rrdcalc_status2string(ae->new_status),
                   emoji, string2str(ae->info), string2str(host->hostname),
                   string2str(host->hostname), string2str(ae->chart), string2str(ae->name),
                   ae->old_value, string2str(ae->units),
                   ae->new_value, string2str(ae->units));
}

// ----------------------------------------------------------------------------
// health alarm log management

inline ALARM_ENTRY* health_create_alarm_entry(
    RRDHOST *host __maybe_unused,
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
    STRING *summary = NULL;
    STRING *info = NULL;
    rrdcalc_runtime_strings_acquire(rc, &summary, &info);

    if (duration < 0)
        duration = 0;

    ALARM_ENTRY *ae = health_alarm_entry_create();
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

    ae->alarm_id = alarm_id;
    ae->alarm_event_id = alarm_event_id;
    ae->when = when;
    ae->old_value = old_value;
    ae->new_value = new_value;

    char value_string[100 + 1];
    ae->old_value_string = string_strdupz(format_value_and_unit(value_string, 100, ae->old_value, ae_units(ae), -1));
    ae->new_value_string = string_strdupz(format_value_and_unit(value_string, 100, ae->new_value, ae_units(ae), -1));

    ae->summary = summary;
    ae->info = info;
    ae->old_status = old_status;
    ae->new_status = new_status;
    ae->duration = duration;
    ae->delay = delay;
    ae->delay_up_to_timestamp = nd_time_t_add_saturating(when, delay);
    ae->flags |= flags;

    ae->last_repeat = 0;
    ae->pending_save_count = 0;

    if(ae->old_status == RRDCALC_STATUS_WARNING || ae->old_status == RRDCALC_STATUS_CRITICAL)
        ae->non_clear_duration += ae->duration;

    return ae;
}

// Link ae into the health log and mark the previous transition of the same alert as updated by it. Returns that
// previous entry, or NULL. Both entries are pinned (pending_save_count) before the lock is released: the previous
// entry becomes UPDATED here, and retention cleanup frees old UPDATED entries that are not pinned, so without the pin
// it could be freed before it is saved. health_alarm_log_save_pinned() saves and unpins them.
static ALARM_ENTRY *health_alarm_log_link_entry(RRDHOST *host, ALARM_ENTRY *ae)
{
    __atomic_add_fetch(&host->health_transitions, 1, __ATOMIC_RELAXED);

    ALARM_ENTRY *update_ae = NULL;

    rw_spinlock_write_lock(&host->health_log.spinlock);

    health_alarm_entry_assign_unique_id_unsafe(host, ae);
    health_alarm_log_insert_entry_unsafe(host, ae);

    // Match the previous older entry for this alert. The list is ordered by
    // descending unique_id, so entries before ae are newer and must not be
    // marked as updated by ae.
    for(ALARM_ENTRY *t = ae->next ; t ; t = t->next) {
        if(t->alarm_id == ae->alarm_id) {
            if(!atomic_flags_check(&t->flags, HEALTH_ENTRY_FLAG_UPDATED) && !t->updated_by_id) {
                // t is published: the notification pass and the savers read its fields without the health-log
                // lock. updated_by_id is stored before the flag that announces it (release).
                __atomic_store_n(&t->updated_by_id, ae->unique_id, __ATOMIC_RELAXED);
                atomic_flags_set(&t->flags, HEALTH_ENTRY_FLAG_UPDATED);
                ae->updates_id = t->unique_id;

                if((t->new_status == RRDCALC_STATUS_WARNING || t->new_status == RRDCALC_STATUS_CRITICAL) &&
                   (t->old_status == RRDCALC_STATUS_WARNING || t->old_status == RRDCALC_STATUS_CRITICAL))
                    ae->non_clear_duration =
                        nd_time_t_add_saturating(ae->non_clear_duration, t->non_clear_duration);

                update_ae = t;
            }

            // no need to continue
            break;
        }
    }

    __atomic_add_fetch(&ae->pending_save_count, 1, __ATOMIC_RELAXED);
    if(update_ae)
        __atomic_add_fetch(&update_ae->pending_save_count, 1, __ATOMIC_RELAXED);

    rw_spinlock_write_unlock(&host->health_log.spinlock);

    netdata_log_debug(D_HEALTH, "Health adding alarm log entry with id: %u", ae->unique_id);

    return update_ae;
}

// Save a linked transition after the previous one it updated, and drop the pins health_alarm_log_link_entry() took.
// async queues the saves to the metadata worker, which pins each entry again until it is saved.
static void health_alarm_log_save_pinned(RRDHOST *host, ALARM_ENTRY *update_ae, ALARM_ENTRY *ae, bool async)
{
    if (update_ae) {
        health_alarm_log_save(host, update_ae, async);
        // release: the save happens before whoever frees the entry sees the pin drop
        __atomic_sub_fetch(&update_ae->pending_save_count, 1, __ATOMIC_RELEASE);
    }

    health_alarm_log_save(host, ae, async);
    __atomic_sub_fetch(&ae->pending_save_count, 1, __ATOMIC_RELEASE);
}

// Add a transition, log it, and queue its save (and its predecessor's) to the metadata worker. For callers that hold
// dictionary locks: nothing here waits for SQLite. The entry is logged here, while it is pinned: once the pin drops,
// the metadata worker can save it and the notification pass can free it, so callers must not use ae afterwards.
inline void health_alarm_log_add_entry(RRDHOST *host, ALARM_ENTRY *ae)
{
    ALARM_ENTRY *update_ae = health_alarm_log_link_entry(host, ae);
    health_log_alert(host, ae);
    health_alarm_log_save_pinned(host, update_ae, ae, true);
}

// Add a transition now and save it later, synchronously, with health_alarm_log_save_linked(): for the health loop,
// which creates transitions while it walks the alert dictionary and must not wait for SQLite under its lock.
// Returns the previous transition it updated (or NULL); both stay pinned until saved.
ALARM_ENTRY *health_alarm_log_add_entry_linked(RRDHOST *host, ALARM_ENTRY *ae)
{
    return health_alarm_log_link_entry(host, ae);
}

// Save a transition added with health_alarm_log_add_entry_linked(), after the previous one it updated, and unpin both.
void health_alarm_log_save_linked(RRDHOST *host, ALARM_ENTRY *update_ae, ALARM_ENTRY *ae)
{
    health_alarm_log_save_pinned(host, update_ae, ae, false);
}

inline void health_alarm_log_free_one_nochecks_nounlink(ALARM_ENTRY *ae) {
    // acquire: pairs with the release that drops each pin, so the pin holder's last use happens before the free
    if(__atomic_load_n(&ae->pending_save_count, __ATOMIC_ACQUIRE))
        metadata_queue_ae_deletion(ae);
    else {
        string_freez(ae->name);
        string_freez(ae->chart);
        string_freez(ae->chart_name);
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
        string_freez(ae->summary);

        if(ae->popen_instance) {
            spawn_popen_kill(ae->popen_instance, 0);
            ae->popen_instance = NULL;
        }

        if(ae->next || ae->prev)
            fatal("HEALTH: alarm entry to delete is still linked!");

//        if(ae->prev_in_progress || ae->next_in_progress)
//            fatal("HEALTH: alarm entry to be delete is linked in progress!");

        health_alarm_entry_destroy(ae);
    }
}

inline void health_alarm_log_free(RRDHOST *host) {
    rw_spinlock_write_lock(&host->health_log.spinlock);

    while(host->health_log.alarms) {
        ALARM_ENTRY *ae = host->health_log.alarms;
        DOUBLE_LINKED_LIST_REMOVE_ITEM_UNSAFE(host->health_log.alarms, ae, prev, next);
        health_alarm_log_free_one_nochecks_nounlink(ae);
    }

    rw_spinlock_write_unlock(&host->health_log.spinlock);
}

// Clean up old alarm entries from memory based on retention settings
void health_alarm_log_cleanup(RRDHOST *host) {
    if(!host->health_log.alarms)
        return;

    if (!rw_spinlock_trywrite_lock(&host->health_log.spinlock)) {
        // If we can't get the lock, just return
        return;
    }

    time_t now = now_realtime_sec();
    time_t retention = host->health_log.health_log_retention_s;

    ALARM_ENTRY *ae = host->health_log.alarms;
    while(ae) {
        // Check if entry is old enough to be deleted
        if(nd_time_t_add_compare(now, -(intmax_t)retention, ae->when) > 0 &&
           // Only remove entries that have been processed/updated, and whose notification is not running: the
           // in-progress queue keeps the pointer until health_alarm_wait_for_execution()
           (atomic_flags_get(&ae->flags) & (HEALTH_ENTRY_FLAG_UPDATED | HEALTH_ENTRY_FLAG_EXEC_IN_PROGRESS)) == HEALTH_ENTRY_FLAG_UPDATED &&
           __atomic_load_n(&ae->pending_save_count, __ATOMIC_ACQUIRE) == 0) { // Only remove entries not pending save
            
            // Remove from linked list
            ALARM_ENTRY *next = ae->next;
            DOUBLE_LINKED_LIST_REMOVE_ITEM_UNSAFE(host->health_log.alarms, ae, prev, next);

            ALARM_ENTRY *to_free = ae;
            ae = next;
            
            // Free memory
            health_alarm_log_free_one_nochecks_nounlink(to_free);
        }
        else
            ae = ae->next;
    }
    
    rw_spinlock_write_unlock(&host->health_log.spinlock);
}
