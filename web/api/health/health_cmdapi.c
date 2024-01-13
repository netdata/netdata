// SPDX-License-Identifier: GPL-3.0-or-later
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
    netdata_log_debug(D_HEALTH, "HEALTH command API: Freeing silencer %s:%s:%s:%s", t->alarms,
          t->charts, t->contexts, t->hosts);
    simple_pattern_free(t->alarms_pattern);
    simple_pattern_free(t->charts_pattern);
    simple_pattern_free(t->contexts_pattern);
    simple_pattern_free(t->hosts_pattern);
    freez(t->alarms);
    freez(t->charts);
    freez(t->contexts);
    freez(t->hosts);
    freez(t);
    return;
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

    int config_changed = 1;

    if (!w->auth_bearer_token) {
        buffer_strcat(wb, HEALTH_CMDAPI_MSG_AUTHERROR);
        ret = HTTP_RESP_FORBIDDEN;
    } else {
        netdata_log_debug(D_HEALTH, "HEALTH command API: Comparing secret '%s' to '%s'", w->auth_bearer_token, api_secret);
        if (strcmp(w->auth_bearer_token, api_secret))
        {
            buffer_strcat(wb, HEALTH_CMDAPI_MSG_AUTHERROR);
            ret = HTTP_RESP_FORBIDDEN;
        }
        else
        {
            SILENCER *silencer = NULL;

            while (url)
            {
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
                    silencer = health_silencer_add_param(silencer, key, value);
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
