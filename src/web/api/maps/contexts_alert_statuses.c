// SPDX-License-Identifier: GPL-3.0-or-later

#include "contexts_alert_statuses.h"

static struct {
    const char *name;
    uint32_t hash;
    CONTEXTS_ALERT_STATUS value;
} contexts_alert_status[] = {
    {"uninitialized"    , 0    , CONTEXT_ALERT_UNINITIALIZED}
    , {"undefined"        , 0    , CONTEXT_ALERT_UNDEFINED}
    , {"clear"            , 0    , CONTEXT_ALERT_CLEAR}
    , {"raised"           , 0    , CONTEXT_ALERT_RAISED}
    , {"active"           , 0    , CONTEXT_ALERT_RAISED}
    , {"warning"          , 0    , CONTEXT_ALERT_WARNING}
    , {"critical"         , 0    , CONTEXT_ALERT_CRITICAL}
    , {NULL               , 0    , 0}
};

CONTEXTS_ALERT_STATUS contexts_alert_status_str_to_id(char *o) {
    CONTEXTS_ALERT_STATUS ret = 0;
    char *tok;

    while(o && *o && (tok = strsep_skip_consecutive_separators(&o, ", |"))) {
        if(!*tok) continue;

        uint32_t hash = simple_hash(tok);
        int i;
        for(i = 0; contexts_alert_status[i].name ; i++) {
            if (unlikely(hash == contexts_alert_status[i].hash && !strcmp(tok, contexts_alert_status[i].name))) {
                ret |= contexts_alert_status[i].value;
                break;
            }
        }
    }

    return ret;
}

void contexts_alerts_status_to_buffer_json_array(BUFFER *wb, const char *key,
    CONTEXTS_ALERT_STATUS options) {
    buffer_json_member_add_array(wb, key);

    CONTEXTS_ALERT_STATUS used = 0; // to prevent adding duplicates
    for(int i = 0; contexts_alert_status[i].name ; i++) {
        if (unlikely((contexts_alert_status[i].value & options) && !(contexts_alert_status[i].value & used))) {
            const char *name = contexts_alert_status[i].name;
            used |= contexts_alert_status[i].value;

            buffer_json_add_array_item_string(wb, name);
        }
    }

    buffer_json_array_close(wb);
}

void contexts_alert_statuses_init(void) {
    for(size_t i = 0; contexts_alert_status[i].name ; i++)
        contexts_alert_status[i].hash = simple_hash(contexts_alert_status[i].name);
}
