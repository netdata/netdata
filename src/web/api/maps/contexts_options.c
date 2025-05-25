// SPDX-License-Identifier: GPL-3.0-or-later

#include "contexts_options.h"

static struct {
    const char *name;
    uint32_t hash;
    CONTEXTS_OPTIONS value;
} contexts_options[] = {
    {"minify"           , 0    , CONTEXTS_OPTION_MINIFY}
    , {"debug"            , 0    , CONTEXTS_OPTION_DEBUG}
    , {"config"           , 0    , CONTEXTS_OPTION_CONFIGURATIONS}
    , {"instances"        , 0    , CONTEXTS_OPTION_INSTANCES}
    , {"values"           , 0    , CONTEXTS_OPTION_VALUES}
    , {"summary"          , 0    , CONTEXTS_OPTION_SUMMARY}
    , {"mcp"              , 0    , CONTEXTS_OPTION_MCP}
    , {"dimensions"       , 0    , CONTEXTS_OPTION_DIMENSIONS}
    , {"labels"           , 0    , CONTEXTS_OPTION_LABELS}
    , {"priorities"       , 0    , CONTEXTS_OPTION_PRIORITIES}
    , {"titles"           , 0    , CONTEXTS_OPTION_TITLES}
    , {"retention"        , 0    , CONTEXTS_OPTION_RETENTION}
    , {"liveness"         , 0    , CONTEXTS_OPTION_LIVENESS}
    , {"family"           , 0    , CONTEXTS_OPTION_FAMILY}
    , {"units"            , 0    , CONTEXTS_OPTION_UNITS}
    , {"rfc3339"          , 0    , CONTEXTS_OPTION_RFC3339}
    , {"long-json-keys"   , 0    , CONTEXTS_OPTION_JSON_LONG_KEYS}
    , {"long-keys"        , 0    , CONTEXTS_OPTION_JSON_LONG_KEYS}
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

// Map RRDR_OPTIONS to CONTEXTS_OPTIONS for options that are common between both
ALWAYS_INLINE
CONTEXTS_OPTIONS rrdr_options_to_contexts_options(RRDR_OPTIONS rrdr_options) {
    CONTEXTS_OPTIONS contexts_options = 0;
    
    if(rrdr_options & RRDR_OPTION_MINIFY)
        contexts_options |= CONTEXTS_OPTION_MINIFY;
    
    if(rrdr_options & RRDR_OPTION_DEBUG)
        contexts_options |= CONTEXTS_OPTION_DEBUG;
    
    if(rrdr_options & RRDR_OPTION_RFC3339)
        contexts_options |= CONTEXTS_OPTION_RFC3339;
    
    if(rrdr_options & RRDR_OPTION_LONG_JSON_KEYS)
        contexts_options |= CONTEXTS_OPTION_JSON_LONG_KEYS;
    
    return contexts_options;
}
