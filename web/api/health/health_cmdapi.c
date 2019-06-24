//
// Created by Christopher on 11/12/18.
//

#include "health_cmdapi.h"

/**
 * Free Silencers
 *
 * Clean the silencer structure
 *
 * @param t is the structure that will be cleaned.
 */
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
        health_silencers2json_entry(wb, HEALTH_FAMILIES_KEY, silencer->families, j);
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
 * Write the sliencer buffer to a file.
 * @param wb
 */
void health_silencers2file(BUFFER *wb) {
    if (wb->len == 0) return;

    FILE *fd = fopen(silencers_filename, "wb");
    if(fd) {
        size_t written = (size_t)fprintf(fd, "%s", wb->buffer) ;
        if (written == wb->len ) {
            info("Silencer changes written to %s", silencers_filename);
        }
        fclose(fd);
        return;
    }
    error("Silencer changes could not be written to %s. Error %s", silencers_filename, strerror(errno));
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
    int ret = 400;
    (void) host;

    BUFFER *wb = w->response.data;
    buffer_flush(wb);
    wb->contenttype = CT_TEXT_PLAIN;

    buffer_flush(w->response.data);

    //Local instance of the silencer
    SILENCER *silencer = NULL;
    int config_changed = 1;

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
                    //In this "if" we are working with the global silencers.
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
                        w->response.data->contenttype = CT_APPLICATION_JSON;
                        health_silencers2json(wb);
                        config_changed=0;
                    }
                } else {
                    //In this else we work with local silencer
                    silencer = health_silencers_addparam(silencer,key,value);
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
            ret = 200;
        }
    }
    w->response.data = wb;
    buffer_no_cacheable(w->response.data);
    if (ret == 200 && config_changed) {
        BUFFER *jsonb = buffer_create(200);
        health_silencers2json(jsonb);
        health_silencers2file(jsonb);
    }
    return ret;
}
