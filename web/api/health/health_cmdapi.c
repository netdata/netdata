//
// Created by christopher on 11/12/18.
//

#include "health_cmdapi.h"


static SILENCER *create_silencer(void) {
    SILENCER *t = callocz(1, sizeof(SILENCER));
    debug(D_HEALTH, "HEALTH command API: Created empty silencer");

    return t;
}

void free_silencers(SILENCER *t) {
    if (!t) return;
    if (t->next) free_silencers(t->next);
    debug(D_HEALTH, "HEALTH command API: Freeing silencer %s:%s:%s:%s:%s", t->alarms,
          t->charts, t->contexts, t->hosts, t->families);
    simple_pattern_free(t->alarms_pattern);
    simple_pattern_free(t->charts_pattern);
    simple_pattern_free(t->contexts_pattern);
    simple_pattern_free(t->hosts_pattern);
    simple_pattern_free(t->families_pattern);
    freez(t->alarms);
    freez(t->charts);
    freez(t->contexts);
    freez(t->hosts);
    freez(t->families);
    freez(t);
    return;
}

int health_silencers2json_entry(BUFFER *wb, char* var, char* val, int hasprev) {
    if (val) {
        buffer_sprintf(wb, "%s\n\t\t\t\"%s\": \"%s\"", (hasprev)?",":"", var, val);
        return 1;
    } else {
        return hasprev;
    }
}

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
        health_silencers2json_entry(wb, HEALTH_FAMILIES_KEY, silencer->families, j);
        j=0;
        buffer_strcat(wb, "\n\t\t}");
        i++;
    }
    if(likely(i)) buffer_strcat(wb, "\n\t");
    buffer_strcat(wb, "]\n}\n");
}

int web_client_api_request_v1_mgmt_health(RRDHOST *host, struct web_client *w, char *url) {
    int ret = 400;
    (void) host;



    BUFFER *wb = w->response.data;
    buffer_flush(wb);
    wb->contenttype = CT_TEXT_PLAIN;

    buffer_flush(w->response.data);

    static uint32_t
            hash_alarm = 0,
            hash_template = 0,
            hash_chart = 0,
            hash_context = 0,
            hash_host = 0,
            hash_families = 0;

    if (unlikely(!hash_alarm)) {
        hash_alarm = simple_uhash(HEALTH_ALARM_KEY);
        hash_template = simple_uhash(HEALTH_TEMPLATE_KEY);
        hash_chart = simple_uhash(HEALTH_CHART_KEY);
        hash_context = simple_uhash(HEALTH_CONTEXT_KEY);
        hash_host = simple_uhash(HEALTH_HOST_KEY);
        hash_families = simple_uhash(HEALTH_FAMILIES_KEY);
    }

    SILENCER *silencer = NULL;

    if (!w->auth_bearer_token) {
        buffer_strcat(wb, HEALTH_CMDAPI_MSG_AUTHERROR);
        ret = 403;
    } else {
        debug(D_HEALTH, "HEALTH command API: Comparing secret '%s' to '%s'", w->auth_bearer_token, api_secret);
        if (strcmp(w->auth_bearer_token, api_secret)) {
            buffer_strcat(wb, HEALTH_CMDAPI_MSG_AUTHERROR);
            ret = 403;
        } else {
            while (url) {
                char *value = mystrsep(&url, "&");
                if (!value || !*value) continue;

                char *key = mystrsep(&value, "=");
                if (!key || !*key) continue;
                if (!value || !*value) continue;

                debug(D_WEB_CLIENT, "%llu: API v1 health query param '%s' with value '%s'", w->id, key, value);

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
                        health_silencers2json(wb);
                    }
                } else {
                    uint32_t hash = simple_uhash(key);
                    if (unlikely(silencer == NULL)) {
                        if (
                                (hash == hash_alarm && !strcasecmp(key, HEALTH_ALARM_KEY)) ||
                                (hash == hash_template && !strcasecmp(key, HEALTH_TEMPLATE_KEY)) ||
                                (hash == hash_chart && !strcasecmp(key, HEALTH_CHART_KEY)) ||
                                (hash == hash_context && !strcasecmp(key, HEALTH_CONTEXT_KEY)) ||
                                (hash == hash_host && !strcasecmp(key, HEALTH_HOST_KEY)) ||
                                (hash == hash_families && !strcasecmp(key, HEALTH_FAMILIES_KEY))
                                ) {
                            silencer = create_silencer();
                        }
                    }

                    if (hash == hash_alarm && !strcasecmp(key, HEALTH_ALARM_KEY)) {
                        silencer->alarms = strdupz(value);
                        silencer->alarms_pattern = simple_pattern_create(silencer->alarms, NULL, SIMPLE_PATTERN_EXACT);
                    } else if (hash == hash_chart && !strcasecmp(key, HEALTH_CHART_KEY)) {
                        silencer->charts = strdupz(value);
                        silencer->charts_pattern = simple_pattern_create(silencer->charts, NULL, SIMPLE_PATTERN_EXACT);
                    } else if (hash == hash_context && !strcasecmp(key, HEALTH_CONTEXT_KEY)) {
                        silencer->contexts = strdupz(value);
                        silencer->contexts_pattern = simple_pattern_create(silencer->contexts, NULL, SIMPLE_PATTERN_EXACT);
                    } else if (hash == hash_host && !strcasecmp(key, HEALTH_HOST_KEY)) {
                        silencer->hosts = strdupz(value);
                        silencer->hosts_pattern = simple_pattern_create(silencer->hosts, NULL, SIMPLE_PATTERN_EXACT);
                    } else if (hash == hash_families && !strcasecmp(key, HEALTH_FAMILIES_KEY)) {
                        silencer->families = strdupz(value);
                        silencer->families_pattern = simple_pattern_create(silencer->families, NULL, SIMPLE_PATTERN_EXACT);
                    } else {
                        buffer_strcat(wb, HEALTH_CMDAPI_MSG_INVALID_KEY);
                    }
                }

            }
            if (likely(silencer)) {
                // Add the created instance to the linked list in silencers
                silencer->next = silencers->silencers;
                silencers->silencers = silencer;
                debug(D_HEALTH, "HEALTH command API: Added silencer %s:%s:%s:%s:%s", silencer->alarms,
                      silencer->charts, silencer->contexts, silencer->hosts, silencer->families
                );
                buffer_strcat(wb, HEALTH_CMDAPI_MSG_ADDED);
                if (silencers->stype == STYPE_NONE) {
                    buffer_strcat(wb, HEALTH_CMDAPI_MSG_STYPEWARNING);
                }
            }
            if (unlikely(silencers->stype != STYPE_NONE && !silencers->all_alarms && !silencers->silencers)) {
                buffer_strcat(wb, HEALTH_CMDAPI_MSG_NOSELECTORWARNING);
            }
            ret = 200;
        }
    }
    w->response.data = wb;
    buffer_no_cacheable(w->response.data);
    return ret;
}
