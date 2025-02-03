// SPDX-License-Identifier: GPL-3.0-or-later

#include "health_internals.h"

#define HEALTH_CMDAPI_CMD_SILENCEALL "SILENCE ALL"
#define HEALTH_CMDAPI_CMD_DISABLEALL "DISABLE ALL"
#define HEALTH_CMDAPI_CMD_SILENCE "SILENCE"
#define HEALTH_CMDAPI_CMD_DISABLE "DISABLE"
#define HEALTH_CMDAPI_CMD_RESET "RESET"
#define HEALTH_CMDAPI_CMD_LIST "LIST"

#define HEALTH_CMDAPI_MSG_AUTHERROR "Auth Error\n"
#define HEALTH_CMDAPI_MSG_SILENCEALL "All alarm notifications are silenced\n"
#define HEALTH_CMDAPI_MSG_DISABLEALL "All health checks are disabled\n"
#define HEALTH_CMDAPI_MSG_RESET "All health checks and notifications are enabled\n"
#define HEALTH_CMDAPI_MSG_DISABLE "Health checks disabled for alarms matching the selectors\n"
#define HEALTH_CMDAPI_MSG_SILENCE "Alarm notifications silenced for alarms matching the selectors\n"
#define HEALTH_CMDAPI_MSG_ADDED "Alarm selector added\n"
#define HEALTH_CMDAPI_MSG_STYPEWARNING "WARNING: Added alarm selector to silence/disable alarms without a SILENCE or DISABLE command.\n"
#define HEALTH_CMDAPI_MSG_NOSELECTORWARNING "WARNING: SILENCE or DISABLE command is ineffective without defining any alarm selectors.\n"

SILENCERS *silencers;

/**
 * Create Silencer
 *
 * Allocate a new silencer to Netdata.
 *
 * @return It returns the address off the silencer on success and NULL otherwise
 */
SILENCER *create_silencer(void) {
    SILENCER *t = callocz(1, sizeof(SILENCER));
    netdata_log_debug(D_HEALTH, "HEALTH command API: Created empty silencer");

    return t;
}

/**
 * Health Silencers add
 *
 * Add more one silencer to the list of silencers.
 *
 * @param silencer
 */
void health_silencers_add(SILENCER *silencer) {
    // Add the created instance to the linked list in silencers
    silencer->next = silencers->silencers;
    silencers->silencers = silencer;
    netdata_log_debug(
        D_HEALTH,
        "HEALTH command API: Added silencer %s:%s:%s:%s",
        silencer->alarms,
        silencer->charts,
        silencer->contexts,
        silencer->hosts);
}

/**
 * Silencers Add Parameter
 *
 * Create a new silencer and adjust the variables
 *
 * @param silencer a pointer to the silencer that will be adjusted
 * @param key the key value sent by client
 * @param value the value sent to the key
 *
 * @return It returns the silencer configured on success and NULL otherwise
 */
SILENCER *health_silencers_addparam(SILENCER *silencer, char *key, char *value) {
    static uint32_t
        hash_alarm = 0,
        hash_template = 0,
        hash_chart = 0,
        hash_context = 0,
        hash_host = 0;

    if (unlikely(!hash_alarm)) {
        hash_alarm = simple_uhash(HEALTH_ALARM_KEY);
        hash_template = simple_uhash(HEALTH_TEMPLATE_KEY);
        hash_chart = simple_uhash(HEALTH_CHART_KEY);
        hash_context = simple_uhash(HEALTH_CONTEXT_KEY);
        hash_host = simple_uhash(HEALTH_HOST_KEY);
    }

    uint32_t hash = simple_uhash(key);
    if (unlikely(silencer == NULL)) {
        if (
            (hash == hash_alarm && !strcasecmp(key, HEALTH_ALARM_KEY)) ||
            (hash == hash_template && !strcasecmp(key, HEALTH_TEMPLATE_KEY)) ||
            (hash == hash_chart && !strcasecmp(key, HEALTH_CHART_KEY)) ||
            (hash == hash_context && !strcasecmp(key, HEALTH_CONTEXT_KEY)) ||
            (hash == hash_host && !strcasecmp(key, HEALTH_HOST_KEY))
        ) {
            silencer = create_silencer();
        }
    }

    if (hash == hash_alarm && !strcasecmp(key, HEALTH_ALARM_KEY)) {
        silencer->alarms = strdupz(value);
        silencer->alarms_pattern = simple_pattern_create(silencer->alarms, NULL, SIMPLE_PATTERN_EXACT, true);
    } else if (hash == hash_chart && !strcasecmp(key, HEALTH_CHART_KEY)) {
        silencer->charts = strdupz(value);
        silencer->charts_pattern = simple_pattern_create(silencer->charts, NULL, SIMPLE_PATTERN_EXACT, true);
    } else if (hash == hash_context && !strcasecmp(key, HEALTH_CONTEXT_KEY)) {
        silencer->contexts = strdupz(value);
        silencer->contexts_pattern = simple_pattern_create(silencer->contexts, NULL, SIMPLE_PATTERN_EXACT, true);
    } else if (hash == hash_host && !strcasecmp(key, HEALTH_HOST_KEY)) {
        silencer->hosts = strdupz(value);
        silencer->hosts_pattern = simple_pattern_create(silencer->hosts, NULL, SIMPLE_PATTERN_EXACT, true);
    }

    return silencer;
}

/**
 * JSON Read Callback
 *
 * Callback called by netdata to create the silencer.
 *
 * @param e the main json structure
 *
 * @return It always return 0.
 */
int health_silencers_json_read_callback(JSON_ENTRY *e)
{
    switch(e->type) {
        case JSON_OBJECT:
#ifndef ENABLE_JSONC
            e->callback_function = health_silencers_json_read_callback;
            if(strcmp(e->name,"")) {
                // init silencer
                netdata_log_debug(D_HEALTH, "JSON: Got object with a name, initializing new silencer for %s",e->name);
#endif
                e->callback_data = create_silencer();
                if(e->callback_data) {
                    health_silencers_add(e->callback_data);
                }
#ifndef ENABLE_JSONC
            }
#endif
            break;

        case JSON_ARRAY:
            e->callback_function = health_silencers_json_read_callback;
            break;

        case JSON_STRING:
            if(!strcmp(e->name,"type")) {
                netdata_log_debug(D_HEALTH, "JSON: Processing type=%s",e->data.string);
                if (!strcmp(e->data.string,"SILENCE")) silencers->stype = STYPE_SILENCE_NOTIFICATIONS;
                else if (!strcmp(e->data.string,"DISABLE")) silencers->stype = STYPE_DISABLE_ALARMS;
            } else {
                netdata_log_debug(D_HEALTH, "JSON: Adding %s=%s", e->name, e->data.string);
                if (e->callback_data)
                    (void)health_silencers_addparam(e->callback_data, e->name, e->data.string);
            }
            break;

        case JSON_BOOLEAN:
            netdata_log_debug(D_HEALTH, "JSON: Processing all_alarms");
            silencers->all_alarms=e->data.boolean?1:0;
            break;

        case JSON_NUMBER:
        case JSON_NULL:
            break;
    }

    return 0;
}

/**
 * Initialize Global Silencers
 *
 * Initialize the silencer  for the whole netdata system.
 *
 * @return It returns 0 on success and -1 otherwise
 */
int health_initialize_global_silencers() {
    silencers = mallocz(sizeof(SILENCERS));
    silencers->all_alarms = 0;
    silencers->stype = STYPE_NONE;
    silencers->silencers = NULL;

    return 0;
}

// ----------------------------------------------------------------------------

/**
 * Free Silencers
 *
 * Clean the silencer structure
 *
 * @param t is the structure that will be cleaned.
 */
void free_silencers(SILENCER *t) {
    if (!t) return;

    while(t) {
        SILENCER *next = t->next;

        simple_pattern_free(t->alarms_pattern);
        simple_pattern_free(t->charts_pattern);
        simple_pattern_free(t->contexts_pattern);
        simple_pattern_free(t->hosts_pattern);
        freez(t->alarms);
        freez(t->charts);
        freez(t->contexts);
        freez(t->hosts);
        freez(t);

        t = next;
    }
}

/**
 * Silencers to JSON Entry
 *
 * Fill the buffer with the other values given.
 *
 * @param wb a pointer to the output buffer
 * @param var the json variable
 * @param val the json value
 * @param hasprev has it a previous value?
 *
 * @return
 */
int health_silencers2json_entry(BUFFER *wb, char* var, char* val, int hasprev) {
    if (val) {
        buffer_sprintf(wb, "%s\n\t\t\t\"%s\": \"%s\"", (hasprev)?",":"", var, val);
        return 1;
    } else {
        return hasprev;
    }
}

/**
 * Silencer to JSON
 *
 * Write the silencer values using JSON format inside a buffer.
 *
 * @param wb is the buffer to write the silencers.
 */
void health_silencers2json(BUFFER *wb) {
    buffer_sprintf(wb, "{\n\t\"all\": %s,"
                       "\n\t\"type\": \"%s\","
                       "\n\t\"silencers\": [",
                   (silencers->all_alarms)?"true":"false",
                   (silencers->stype == STYPE_NONE)?"None":((silencers->stype == STYPE_DISABLE_ALARMS)?"DISABLE":"SILENCE"));

    SILENCER *silencer;
    int i = 0, j = 0;
    for(silencer = silencers->silencers; silencer ; silencer = silencer->next) {
        if(likely(i)) buffer_strcat(wb, ",");
        buffer_strcat(wb, "\n\t\t{");
        j=health_silencers2json_entry(wb, HEALTH_ALARM_KEY, silencer->alarms, j);
        j=health_silencers2json_entry(wb, HEALTH_CHART_KEY, silencer->charts, j);
        j=health_silencers2json_entry(wb, HEALTH_CONTEXT_KEY, silencer->contexts, j);
        j=health_silencers2json_entry(wb, HEALTH_HOST_KEY, silencer->hosts, j);
        j=0;
        buffer_strcat(wb, "\n\t\t}");
        i++;
    }
    if(likely(i)) buffer_strcat(wb, "\n\t");
    buffer_strcat(wb, "]\n}\n");
}


/**
 * Silencer to FILE
 *
 * Write the silencer buffer to a file.
 * @param wb
 */
void health_silencers2file(BUFFER *wb) {
    if (wb->len == 0) return;

    FILE *fd = fopen(health_silencers_filename(), "wb");
    if(fd) {
        size_t written = (size_t)fprintf(fd, "%s", wb->buffer) ;
        if (written == wb->len ) {
            netdata_log_info("Silencer changes written to %s", health_silencers_filename());
        }
        fclose(fd);
        return;
    }
    netdata_log_error("Silencer changes could not be written to %s. Error %s", health_silencers_filename(), strerror(errno));
}

/**
 * Request V1 MGMT Health
 *
 * Function called by api to management the health.
 *
 * @param host main structure with client information!
 * @param w is the structure with all information of the client request.
 * @param url is the url that netdata is working
 *
 * @return It returns 200 on success and another code otherwise.
 */
int web_client_api_request_v1_mgmt_health(RRDHOST *host, struct web_client *w, char *url) {
    int ret;
    (void) host;

    BUFFER *wb = w->response.data;
    buffer_flush(wb);
    wb->content_type = CT_TEXT_PLAIN;

    buffer_flush(w->response.data);

    //Local instance of the silencer
    SILENCER *silencer = NULL;
    int config_changed = 1;

    if (!w->auth_bearer_token) {
        buffer_strcat(wb, HEALTH_CMDAPI_MSG_AUTHERROR);
        ret = HTTP_RESP_FORBIDDEN;
    } else {
        netdata_log_debug(D_HEALTH, "HEALTH command API: Comparing secret '%s' to '%s'", w->auth_bearer_token, api_secret);
        if (strcmp(w->auth_bearer_token, api_secret) != 0) {
            buffer_strcat(wb, HEALTH_CMDAPI_MSG_AUTHERROR);
            ret = HTTP_RESP_FORBIDDEN;
        } else {
            while (url) {
                char *value = strsep_skip_consecutive_separators(&url, "&");
                if (!value || !*value) continue;

                char *key = strsep_skip_consecutive_separators(&value, "=");
                if (!key || !*key) continue;
                if (!value || !*value) continue;

                netdata_log_debug(D_WEB_CLIENT, "%llu: API v1 health query param '%s' with value '%s'", w->id, key, value);

                // name and value are now the parameters
                if (!strcmp(key, "cmd")) {
                    if (!strcmp(value, HEALTH_CMDAPI_CMD_SILENCEALL)) {
                        silencers->all_alarms = 1;
                        silencers->stype = STYPE_SILENCE_NOTIFICATIONS;
                        buffer_strcat(wb, HEALTH_CMDAPI_MSG_SILENCEALL);
                    } else if (!strcmp(value, HEALTH_CMDAPI_CMD_DISABLEALL)) {
                        silencers->all_alarms = 1;
                        silencers->stype = STYPE_DISABLE_ALARMS;
                        buffer_strcat(wb, HEALTH_CMDAPI_MSG_DISABLEALL);
                    } else if (!strcmp(value, HEALTH_CMDAPI_CMD_SILENCE)) {
                        silencers->stype = STYPE_SILENCE_NOTIFICATIONS;
                        buffer_strcat(wb, HEALTH_CMDAPI_MSG_SILENCE);
                    } else if (!strcmp(value, HEALTH_CMDAPI_CMD_DISABLE)) {
                        silencers->stype = STYPE_DISABLE_ALARMS;
                        buffer_strcat(wb, HEALTH_CMDAPI_MSG_DISABLE);
                    } else if (!strcmp(value, HEALTH_CMDAPI_CMD_RESET)) {
                        silencers->all_alarms = 0;
                        silencers->stype = STYPE_NONE;
                        free_silencers(silencers->silencers);
                        silencers->silencers = NULL;
                        buffer_strcat(wb, HEALTH_CMDAPI_MSG_RESET);
                    } else if (!strcmp(value, HEALTH_CMDAPI_CMD_LIST)) {
                        w->response.data->content_type = CT_APPLICATION_JSON;
                        health_silencers2json(wb);
                        config_changed=0;
                    }
                } else {
                    silencer = health_silencers_addparam(silencer, key, value);
                }
            }

            if (likely(silencer)) {
                health_silencers_add(silencer);
                buffer_strcat(wb, HEALTH_CMDAPI_MSG_ADDED);
                if (silencers->stype == STYPE_NONE) {
                    buffer_strcat(wb, HEALTH_CMDAPI_MSG_STYPEWARNING);
                }
            }
            if (unlikely(silencers->stype != STYPE_NONE && !silencers->all_alarms && !silencers->silencers)) {
                buffer_strcat(wb, HEALTH_CMDAPI_MSG_NOSELECTORWARNING);
            }
            ret = HTTP_RESP_OK;
        }
    }
    w->response.data = wb;
    buffer_no_cacheable(w->response.data);
    if (ret == HTTP_RESP_OK && config_changed) {
        BUFFER *jsonb = buffer_create(200, &netdata_buffers_statistics.buffers_health);
        health_silencers2json(jsonb);
        health_silencers2file(jsonb);
        buffer_free(jsonb);
    }

    return ret;
}

// ----------------------------------------------------------------------------

const char *health_silencers_filename(void) {
    return string2str(health_globals.config.silencers_filename);
}

void health_set_silencers_filename(void) {
    char filename[FILENAME_MAX + 1];
    snprintfz(filename, FILENAME_MAX, "%s/health.silencers.json", netdata_configured_varlib_dir);

    health_globals.config.silencers_filename =
        string_strdupz(inicfg_get(&netdata_config, CONFIG_SECTION_HEALTH, "silencers file", filename));
}

void health_silencers_init(void) {
    FILE *fd = fopen(health_silencers_filename(), "r");
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
                    netdata_log_info("Parsed health silencers file %s", health_silencers_filename());
                } else {
                    netdata_log_error("Cannot read the data from health silencers file %s", health_silencers_filename());
                }
                freez(str);
            }
        } else {
            netdata_log_error("Health silencers file %s has the size %" PRId64 " that is out of range[ 1 , %d ]. Aborting read.",
                              health_silencers_filename(),
                              (int64_t)length,
                              HEALTH_SILENCERS_MAX_FILE_LEN);
        }
        fclose(fd);
    } else {
        netdata_log_info("Cannot open the file %s, so Netdata will work with the default health configuration.",
                         health_silencers_filename());
    }
}

SILENCE_TYPE health_silencers_check_silenced(RRDCALC *rc, const char *host) {
    SILENCER *s;

    for (s = silencers->silencers; s!=NULL; s=s->next){
        if (
            (!s->alarms_pattern || (rc->config.name && s->alarms_pattern && simple_pattern_matches_string(s->alarms_pattern, rc->config.name))) &&
            (!s->contexts_pattern || (rc->rrdset && rc->rrdset->context && s->contexts_pattern && simple_pattern_matches_string(s->contexts_pattern, rc->rrdset->context))) &&
            (!s->hosts_pattern || (host && s->hosts_pattern && simple_pattern_matches(s->hosts_pattern, host))) &&
            (!s->charts_pattern || (rc->chart && s->charts_pattern && simple_pattern_matches_string(s->charts_pattern, rc->chart)))
        ) {
            netdata_log_debug(D_HEALTH, "Alarm matches command API silence entry %s:%s:%s:%s", s->alarms,s->charts, s->contexts, s->hosts);
            if (unlikely(silencers->stype == STYPE_NONE)) {
                netdata_log_debug(D_HEALTH, "Alarm %s matched a silence entry, but no SILENCE or DISABLE command was issued via the command API. The match has no effect.", rrdcalc_name(rc));
            } else {
                netdata_log_debug(D_HEALTH, "Alarm %s via the command API - name:%s context:%s chart:%s host:%s"
                                  , (silencers->stype == STYPE_DISABLE_ALARMS)?"Disabled":"Silenced"
                                  , rrdcalc_name(rc)
                                      , (rc->rrdset)?rrdset_context(rc->rrdset):""
                                  , rrdcalc_chart_name(rc)
                                      , host
                );
            }
            return silencers->stype;
        }
    }
    return STYPE_NONE;
}

int health_silencers_update_disabled_silenced(RRDHOST *host, RRDCALC *rc) {
    uint32_t rrdcalc_flags_old = rc->run_flags;
    // Clear the flags
    rc->run_flags &= ~(RRDCALC_FLAG_DISABLED | RRDCALC_FLAG_SILENCED);
    if (unlikely(silencers->all_alarms)) {
        if (silencers->stype == STYPE_DISABLE_ALARMS) rc->run_flags |= RRDCALC_FLAG_DISABLED;
        else if (silencers->stype == STYPE_SILENCE_NOTIFICATIONS) rc->run_flags |= RRDCALC_FLAG_SILENCED;
    } else {
        SILENCE_TYPE st = health_silencers_check_silenced(rc, rrdhost_hostname(host));
        if (st == STYPE_DISABLE_ALARMS) rc->run_flags |= RRDCALC_FLAG_DISABLED;
        else if (st == STYPE_SILENCE_NOTIFICATIONS) rc->run_flags |= RRDCALC_FLAG_SILENCED;
    }

    if (rrdcalc_flags_old != rc->run_flags) {
        netdata_log_info(
            "Alarm silencing changed for host '%s' alarm '%s': Disabled %s->%s Silenced %s->%s",
            rrdhost_hostname(host),
            rrdcalc_name(rc),
            (rrdcalc_flags_old & RRDCALC_FLAG_DISABLED) ? "true" : "false",
            (rc->run_flags & RRDCALC_FLAG_DISABLED) ? "true" : "false",
            (rrdcalc_flags_old & RRDCALC_FLAG_SILENCED) ? "true" : "false",
            (rc->run_flags & RRDCALC_FLAG_SILENCED) ? "true" : "false");
    }
    if (rc->run_flags & RRDCALC_FLAG_DISABLED)
        return 1;
    else
        return 0;
}
