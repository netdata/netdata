// SPDX-License-Identifier: GPL-3.0-or-later

#include "contexts_options.h"

static struct {
    const char *name;
    uint32_t hash;
    CONTEXTS_OPTIONS value;
} contexts_options[] = {
    {"minify"           , 0    , CONTEXTS_OPTION_MINIFY}
    , {"debug"            , 0    , CONTEXTS_OPTION_DEBUG}
    , {"config"           , 0    , CONTEXTS_OPTION_ALERTS_WITH_CONFIGURATIONS}
    , {"instances"        , 0    , CONTEXTS_OPTION_ALERTS_WITH_INSTANCES}
    , {"values"           , 0    , CONTEXTS_OPTION_ALERTS_WITH_VALUES}
    , {"summary"          , 0    , CONTEXTS_OPTION_ALERTS_WITH_SUMMARY}
    , {NULL               , 0    , 0}
};

CONTEXTS_OPTIONS contexts_options_str_to_id(char *o) {
    CONTEXTS_OPTIONS ret = 0;
    char *tok;

    while(o && *o && (tok = strsep_skip_consecutive_separators(&o, ", |"))) {
        if(!*tok) continue;

        uint32_t hash = simple_hash(tok);
        int i;
        for(i = 0; contexts_options[i].name ; i++) {
            if (unlikely(hash == contexts_options[i].hash && !strcmp(tok, contexts_options[i].name))) {
                ret |= contexts_options[i].value;
                break;
            }
        }
    }

    return ret;
}

void contexts_options_to_buffer_json_array(BUFFER *wb, const char *key, CONTEXTS_OPTIONS options) {
    buffer_json_member_add_array(wb, key);

    CONTEXTS_OPTIONS used = 0; // to prevent adding duplicates
    for(int i = 0; contexts_options[i].name ; i++) {
        if (unlikely((contexts_options[i].value & options) && !(contexts_options[i].value & used))) {
            const char *name = contexts_options[i].name;
            used |= contexts_options[i].value;

            buffer_json_add_array_item_string(wb, name);
        }
    }

    buffer_json_array_close(wb);
}

void contexts_options_init(void) {
    for(size_t i = 0; contexts_options[i].name ; i++)
        contexts_options[i].hash = simple_hash(contexts_options[i].name);
}
