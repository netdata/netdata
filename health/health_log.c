// SPDX-License-Identifier: GPL-3.0-or-later

#include "health.h"

// ----------------------------------------------------------------------------
// health alarm log load/save
// no need for locking - only one thread is reading / writing the alarms log

inline int health_alarm_log_open(RRDHOST *host) {
    if(host->health_log_fp)
        fclose(host->health_log_fp);

    host->health_log_fp = fopen(host->health_log_filename, "a");

    if(host->health_log_fp) {
        if (setvbuf(host->health_log_fp, NULL, _IOLBF, 0) != 0)
            error("HEALTH [%s]: cannot set line buffering on health log file '%s'.", host->hostname, host->health_log_filename);
        return 0;
    }

    error("HEALTH [%s]: cannot open health log file '%s'. Health data will be lost in case of netdata or server crash.", host->hostname, host->health_log_filename);
    return -1;
}

static inline void health_alarm_log_close(RRDHOST *host) {
    if(host->health_log_fp) {
        fclose(host->health_log_fp);
        host->health_log_fp = NULL;
    }
}

static inline void health_log_rotate(RRDHOST *host) {
    static size_t rotate_every = 0;

    if(unlikely(rotate_every == 0)) {
        rotate_every = (size_t)config_get_number(CONFIG_SECTION_HEALTH, "rotate log every lines", 2000);
        if(rotate_every < 100) rotate_every = 100;
    }

    if(unlikely(host->health_log_entries_written > rotate_every)) {
        if(unlikely(host->health_log_fp)) {
            health_alarm_log_close(host);

            char old_filename[FILENAME_MAX + 1];
            snprintfz(old_filename, FILENAME_MAX, "%s.old", host->health_log_filename);

            if(unlink(old_filename) == -1 && errno != ENOENT)
                error("HEALTH [%s]: cannot remove old alarms log file '%s'", host->hostname, old_filename);

            if(link(host->health_log_filename, old_filename) == -1 && errno != ENOENT)
                error("HEALTH [%s]: cannot move file '%s' to '%s'.", host->hostname, host->health_log_filename, old_filename);

            if(unlink(host->health_log_filename) == -1 && errno != ENOENT)
                error("HEALTH [%s]: cannot remove old alarms log file '%s'", host->hostname, host->health_log_filename);

            // open it with truncate
            host->health_log_fp = fopen(host->health_log_filename, "w");

            if(host->health_log_fp)
                fclose(host->health_log_fp);
            else
                error("HEALTH [%s]: cannot truncate health log '%s'", host->hostname, host->health_log_filename);

            host->health_log_fp = NULL;

            host->health_log_entries_written = 0;
            health_alarm_log_open(host);
        }
    }
}

inline void health_label_log_save(RRDHOST *host) {
    health_log_rotate(host);

    if(unlikely(host->health_log_fp)) {
        BUFFER *wb = buffer_create(1024);
        rrdhost_check_rdlock(host);
        netdata_rwlock_rdlock(&host->labels.labels_rwlock);
        struct label *l=localhost->labels.head;
        while (l != NULL) {
            buffer_sprintf(wb,"%s=%s\t ", l->key, l->value);
            l = l->next;
        }
        netdata_rwlock_unlock(&host->labels.labels_rwlock);

        char *write = (char *) buffer_tostring(wb) ;

        write[wb->len-2] = '\n';
        write[wb->len-1] = '\0';

        if (unlikely(fprintf(host->health_log_fp, "L\t%s"
                , write
        ) < 0))
            error("HEALTH [%s]: failed to save alarm log entry to '%s'. Health data may be lost in case of abnormal restart.",
                  host->hostname, host->health_log_filename);
        else {
            host->health_log_entries_written++;
        }

        buffer_free(wb);
    }
}

inline void health_alarm_log_save(RRDHOST *host, ALARM_ENTRY *ae) {
    health_log_rotate(host);
    if(unlikely(host->health_log_fp)) {
        if(unlikely(fprintf(host->health_log_fp
                            , "%c\t%s"
                        "\t%08x\t%08x\t%08x\t%08x\t%08x"
                        "\t%08x\t%08x\t%08x"
                        "\t%08x\t%08x\t%08x"
                        "\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s"
                        "\t%d\t%d\t%d\t%d"
                        "\t" CALCULATED_NUMBER_FORMAT_AUTO "\t" CALCULATED_NUMBER_FORMAT_AUTO
                        "\t%016"PRIx64""
                        "\t%s\t%s\t%s"
                        "\n"
                            , (ae->flags & HEALTH_ENTRY_FLAG_SAVED)?'U':'A'
                            , host->hostname

                            , ae->unique_id
                            , ae->alarm_id
                            , ae->alarm_event_id
                            , ae->updated_by_id
                            , ae->updates_id

                            , (uint32_t)ae->when
                            , (uint32_t)ae->duration
                            , (uint32_t)ae->non_clear_duration
                            , (uint32_t)ae->flags
                            , (uint32_t)ae->exec_run_timestamp
                            , (uint32_t)ae->delay_up_to_timestamp

                            , (ae->name)?ae->name:""
                            , (ae->chart)?ae->chart:""
                            , (ae->family)?ae->family:""
                            , (ae->exec)?ae->exec:""
                            , (ae->recipient)?ae->recipient:""
                            , (ae->source)?ae->source:""
                            , (ae->units)?ae->units:""
                            , (ae->info)?ae->info:""

                            , ae->exec_code
                            , ae->new_status
                            , ae->old_status
                            , ae->delay

                            , ae->new_value
                            , ae->old_value
                            , (uint64_t)ae->last_repeat
                            , (ae->classification)?ae->classification:"Unknown"
                            , (ae->component)?ae->component:"Unknown"
                            , (ae->type)?ae->type:"Unknown"
        ) < 0))
            error("HEALTH [%s]: failed to save alarm log entry to '%s'. Health data may be lost in case of abnormal restart.", host->hostname, host->health_log_filename);
        else {
            ae->flags |= HEALTH_ENTRY_FLAG_SAVED;
            host->health_log_entries_written++;
        }
    }else
        sql_health_alarm_log_save(host, ae);

#ifdef ENABLE_ACLK
    if (netdata_cloud_setting) {
        sql_queue_alarm_to_aclk(host, ae);
    }
#endif
}

static uint32_t is_valid_alarm_id(RRDHOST *host, const char *chart, const char *name, uint32_t alarm_id)
{
    uint32_t hash_chart = simple_hash(chart);
    uint32_t hash_name = simple_hash(name);

    ALARM_ENTRY *ae;
    for(ae = host->health_log.alarms; ae ;ae = ae->next) {
        if (unlikely(
                ae->alarm_id == alarm_id && (!(ae->hash_name == hash_name && ae->hash_chart == hash_chart &&
                                               !strcmp(name, ae->name) && !strcmp(chart, ae->chart))))) {
            return 0;
        }
    }
    return 1;
}

static inline ssize_t health_alarm_log_read(RRDHOST *host, FILE *fp, const char *filename) {
    errno = 0;

    char *s, *buf = mallocz(65536 + 1);
    size_t line = 0, len = 0;
    ssize_t loaded = 0, updated = 0, errored = 0, duplicate = 0;

    netdata_rwlock_rdlock(&host->health_log.alarm_log_rwlock);

    while((s = fgets_trim_len(buf, 65536, fp, &len))) {
        host->health_log_entries_written++;
        line++;

        int max_entries = 33, entries = 0;
        char *pointers[max_entries];

        pointers[entries++] = s++;
        while(*s) {
            if(unlikely(*s == '\t')) {
                *s = '\0';
                pointers[entries++] = ++s;
                if(entries >= max_entries) {
                    error("HEALTH [%s]: line %zu of file '%s' has more than %d entries. Ignoring excessive entries.", host->hostname, line, filename, max_entries);
                    break;
                }
            }
            else s++;
        }

        if(likely(*pointers[0] == 'L'))
            continue;

        if(likely(*pointers[0] == 'U' || *pointers[0] == 'A')) {
            ALARM_ENTRY *ae = NULL;

            if(entries < 27) {
                error("HEALTH [%s]: line %zu of file '%s' should have at least 27 entries, but it has %d. Ignoring it.", host->hostname, line, filename, entries);
                errored++;
                continue;
            }

            // check that we have valid ids
            uint32_t unique_id = (uint32_t)strtoul(pointers[2], NULL, 16);
            if(!unique_id) {
                error("HEALTH [%s]: line %zu of file '%s' states alarm entry with invalid unique id %u (%s). Ignoring it.", host->hostname, line, filename, unique_id, pointers[2]);
                errored++;
                continue;
            }

            uint32_t alarm_id = (uint32_t)strtoul(pointers[3], NULL, 16);
            if(!alarm_id) {
                error("HEALTH [%s]: line %zu of file '%s' states alarm entry for invalid alarm id %u (%s). Ignoring it.", host->hostname, line, filename, alarm_id, pointers[3]);
                errored++;
                continue;
            }

            // Check if we got last_repeat field
            time_t last_repeat = 0;
            if(entries > 27) {
                char* alarm_name = pointers[13];
                last_repeat = (time_t)strtoul(pointers[27], NULL, 16);

                RRDCALC *rc = alarm_max_last_repeat(host, alarm_name,simple_hash(alarm_name));
                if (!rc) {
                    for(rc = host->alarms; rc ; rc = rc->next) {
                        RRDCALC *rdcmp  = (RRDCALC *) avl_insert_lock(&(host)->alarms_idx_name, (avl_t *)rc);
                        if(rdcmp != rc) {
                            error("Cannot insert the alarm index ID using log %s", rc->name);
                        }
                    }

                    rc = alarm_max_last_repeat(host, alarm_name,simple_hash(alarm_name));
                }

                if(unlikely(rc)) {
                    if (rrdcalc_isrepeating(rc)) {
                        rc->last_repeat = last_repeat;
                        // We iterate through repeating alarm entries only to
                        // find the latest last_repeat timestamp. Otherwise,
                        // there is no need to keep them in memory.
                        continue;
                    }
                }
            }

            if(unlikely(*pointers[0] == 'A')) {
                // make sure it is properly numbered
                if(unlikely(host->health_log.alarms && unique_id < host->health_log.alarms->unique_id)) {
                    error( "HEALTH [%s]: line %zu of file '%s' has alarm log entry %u in wrong order. Ignoring it."
                           , host->hostname, line, filename, unique_id);
                    errored++;
                    continue;
                }

                ae = callocz(1, sizeof(ALARM_ENTRY));
            }
            else if(unlikely(*pointers[0] == 'U')) {
                // find the original
                for(ae = host->health_log.alarms; ae ; ae = ae->next) {
                    if(unlikely(unique_id == ae->unique_id)) {
                        if(unlikely(*pointers[0] == 'A')) {
                            error("HEALTH [%s]: line %zu of file '%s' adds duplicate alarm log entry %u. Using the later."
                            , host->hostname, line, filename, unique_id);
                            *pointers[0] = 'U';
                            duplicate++;
                        }
                        break;
                    }
                    else if(unlikely(unique_id > ae->unique_id)) {
                        // no need to continue
                        // the linked list is sorted
                        ae = NULL;
                        break;
                    }
                }
            }

            // if not found, skip this line
            if(unlikely(!ae)) {
                // error("HEALTH [%s]: line %zu of file '%s' updates alarm log entry with unique id %u, but it is not found.", host->hostname, line, filename, unique_id);
                continue;
            }

            // check for a possible host mismatch
            //if(strcmp(pointers[1], host->hostname))
            //    error("HEALTH [%s]: line %zu of file '%s' provides an alarm for host '%s' but this is named '%s'.", host->hostname, line, filename, pointers[1], host->hostname);

            ae->unique_id               = unique_id;
            if (!is_valid_alarm_id(host, pointers[14], pointers[13], alarm_id))
                alarm_id = rrdcalc_get_unique_id(host, pointers[14], pointers[13], NULL);
            ae->alarm_id                = alarm_id;
            ae->alarm_event_id          = (uint32_t)strtoul(pointers[4], NULL, 16);
            ae->updated_by_id           = (uint32_t)strtoul(pointers[5], NULL, 16);
            ae->updates_id              = (uint32_t)strtoul(pointers[6], NULL, 16);

            ae->when                    = (uint32_t)strtoul(pointers[7], NULL, 16);
            ae->duration                = (uint32_t)strtoul(pointers[8], NULL, 16);
            ae->non_clear_duration      = (uint32_t)strtoul(pointers[9], NULL, 16);

            ae->flags                   = (uint32_t)strtoul(pointers[10], NULL, 16);
            ae->flags |= HEALTH_ENTRY_FLAG_SAVED;

            ae->exec_run_timestamp      = (uint32_t)strtoul(pointers[11], NULL, 16);
            ae->delay_up_to_timestamp   = (uint32_t)strtoul(pointers[12], NULL, 16);

            freez(ae->name);
            ae->name = strdupz(pointers[13]);
            ae->hash_name = simple_hash(ae->name);

            freez(ae->chart);
            ae->chart = strdupz(pointers[14]);
            ae->hash_chart = simple_hash(ae->chart);

            freez(ae->family);
            ae->family = strdupz(pointers[15]);

            freez(ae->exec);
            ae->exec = strdupz(pointers[16]);
            if(!*ae->exec) { freez(ae->exec); ae->exec = NULL; }

            freez(ae->recipient);
            ae->recipient = strdupz(pointers[17]);
            if(!*ae->recipient) { freez(ae->recipient); ae->recipient = NULL; }

            freez(ae->source);
            ae->source = strdupz(pointers[18]);
            if(!*ae->source) { freez(ae->source); ae->source = NULL; }

            freez(ae->units);
            ae->units = strdupz(pointers[19]);
            if(!*ae->units) { freez(ae->units); ae->units = NULL; }

            freez(ae->info);
            ae->info = strdupz(pointers[20]);
            if(!*ae->info) { freez(ae->info); ae->info = NULL; }

            ae->exec_code   = str2i(pointers[21]);
            ae->new_status  = str2i(pointers[22]);
            ae->old_status  = str2i(pointers[23]);
            ae->delay       = str2i(pointers[24]);

            ae->new_value   = str2l(pointers[25]);
            ae->old_value   = str2l(pointers[26]);

            ae->last_repeat = last_repeat;

            if (likely(entries > 30)) {
                freez(ae->classification);
                ae->classification = strdupz(pointers[28]);
                if(!*ae->classification) { freez(ae->classification); ae->classification = NULL; }

                freez(ae->component);
                ae->component = strdupz(pointers[29]);
                if(!*ae->component) { freez(ae->component); ae->component = NULL; }

                freez(ae->type);
                ae->type = strdupz(pointers[30]);
                if(!*ae->type) { freez(ae->type); ae->type = NULL; }
            }

            char value_string[100 + 1];
            freez(ae->old_value_string);
            freez(ae->new_value_string);
            ae->old_value_string = strdupz(format_value_and_unit(value_string, 100, ae->old_value, ae->units, -1));
            ae->new_value_string = strdupz(format_value_and_unit(value_string, 100, ae->new_value, ae->units, -1));

            // add it to host if not already there
            if(unlikely(*pointers[0] == 'A')) {
                ae->next = host->health_log.alarms;
                host->health_log.alarms = ae;
                sql_health_alarm_log_insert(host, ae);
                loaded++;
            }
            else {
                sql_health_alarm_log_update(host, ae);
                updated++;
            }

            if(unlikely(ae->unique_id > host->health_max_unique_id))
                host->health_max_unique_id = ae->unique_id;

            if(unlikely(ae->alarm_id >= host->health_max_alarm_id))
                host->health_max_alarm_id = ae->alarm_id;
        }
        else {
            error("HEALTH [%s]: line %zu of file '%s' is invalid (unrecognized entry type '%s').", host->hostname, line, filename, pointers[0]);
            errored++;
        }
    }

    netdata_rwlock_unlock(&host->health_log.alarm_log_rwlock);

    freez(buf);

    if(!host->health_max_unique_id) host->health_max_unique_id = (uint32_t)now_realtime_sec();
    if(!host->health_max_alarm_id)  host->health_max_alarm_id  = (uint32_t)now_realtime_sec();

    host->health_log.next_log_id = host->health_max_unique_id + 1;
    if (unlikely(!host->health_log.next_alarm_id || host->health_log.next_alarm_id <= host->health_max_alarm_id))
        host->health_log.next_alarm_id = host->health_max_alarm_id + 1;

    debug(D_HEALTH, "HEALTH [%s]: loaded file '%s' with %zd new alarm entries, updated %zd alarms, errors %zd entries, duplicate %zd", host->hostname, filename, loaded, updated, errored, duplicate);
    return loaded;
}

inline void health_alarm_log_load(RRDHOST *host) {
    health_alarm_log_close(host);

    char filename[FILENAME_MAX + 1];
    snprintfz(filename, FILENAME_MAX, "%s.old", host->health_log_filename);
    FILE *fp = fopen(filename, "r");
    if(!fp)
        error("HEALTH [%s]: cannot open health file: %s", host->hostname, filename);
    else {
        health_alarm_log_read(host, fp, filename);
        fclose(fp);
    }

    host->health_log_entries_written = 0;
    fp = fopen(host->health_log_filename, "r");
    if(!fp)
        error("HEALTH [%s]: cannot open health file: %s", host->hostname, host->health_log_filename);
    else {
        health_alarm_log_read(host, fp, host->health_log_filename);
        fclose(fp);
    }
}


// ----------------------------------------------------------------------------
// health alarm log management

inline ALARM_ENTRY* health_create_alarm_entry(
        RRDHOST *host,
        uint32_t alarm_id,
        uint32_t alarm_event_id,
        uuid_t config_hash_id,
        time_t when,
        const char *name,
        const char *chart,
        const char *family,
        const char *class,
        const char *component,
        const char *type,
        const char *exec,
        const char *recipient,
        time_t duration,
        calculated_number old_value,
        calculated_number new_value,
        RRDCALC_STATUS old_status,
        RRDCALC_STATUS new_status,
        const char *source,
        const char *units,
        const char *info,
        int delay,
        uint32_t flags
) {
    debug(D_HEALTH, "Health adding alarm log entry with id: %u", host->health_log.next_log_id);

    ALARM_ENTRY *ae = callocz(1, sizeof(ALARM_ENTRY));
    ae->name = strdupz(name);
    ae->hash_name = simple_hash(ae->name);

    if(chart) {
        ae->chart = strdupz(chart);
        ae->hash_chart = simple_hash(ae->chart);
    }

    uuid_copy(ae->config_hash_id, *((uuid_t *) config_hash_id));

    if(family)
        ae->family = strdupz(family);

    if (class)
        ae->classification = strdupz(class);

    if (component)
        ae->component = strdupz(component);

    if (type)
        ae->type = strdupz(type);

    if(exec) ae->exec = strdupz(exec);
    if(recipient) ae->recipient = strdupz(recipient);
    if(source) ae->source = strdupz(source);
    if(units) ae->units = strdupz(units);

    ae->unique_id = host->health_log.next_log_id++;
    ae->alarm_id = alarm_id;
    ae->alarm_event_id = alarm_event_id;
    ae->when = when;
    ae->old_value = old_value;
    ae->new_value = new_value;

    char value_string[100 + 1];
    ae->old_value_string = strdupz(format_value_and_unit(value_string, 100, ae->old_value, ae->units, -1));
    ae->new_value_string = strdupz(format_value_and_unit(value_string, 100, ae->new_value, ae->units, -1));

    char *replaced_info = NULL;
    if (likely(info)) {
        char *m;
        replaced_info = strdupz(info);
        size_t pos = 0;
        while ((m = strstr(replaced_info + pos, "$family"))) {
            char *buf = NULL;
            pos = m - replaced_info;
            buf = find_and_replace(replaced_info, "$family", (ae->family) ? ae->family : "", m);
            freez(replaced_info);
            replaced_info = strdupz(buf);
            freez(buf);
        }
    }

    if(replaced_info) ae->info = strdupz(replaced_info);
    freez(replaced_info);

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

inline void health_alarm_log(
        RRDHOST *host,
        ALARM_ENTRY *ae
) {
    debug(D_HEALTH, "Health adding alarm log entry with id: %u", ae->unique_id);

    if(unlikely(alarm_entry_isrepeating(host, ae))) {
        error("Repeating alarms cannot be added to host's alarm log entries. It seems somewhere in the logic, API is being misused. Alarm id: %u", ae->alarm_id);
        return;
    }
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
    freez(ae->name);
    freez(ae->chart);
    freez(ae->family);
    freez(ae->classification);
    freez(ae->component);
    freez(ae->type);
    freez(ae->exec);
    freez(ae->recipient);
    freez(ae->source);
    freez(ae->units);
    freez(ae->info);
    freez(ae->old_value_string);
    freez(ae->new_value_string);
    freez(ae);
}

inline void health_alarm_log_free(RRDHOST *host) {
    rrdhost_check_wrlock(host);

    netdata_rwlock_wrlock(&host->health_log.alarm_log_rwlock);

    ALARM_ENTRY *ae;
    while((ae = host->health_log.alarms)) {
        host->health_log.alarms = ae->next;
        health_alarm_log_free_one_nochecks_nounlink(ae);
    }

    netdata_rwlock_unlock(&host->health_log.alarm_log_rwlock);
}
