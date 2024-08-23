// SPDX-License-Identifier: GPL-3.0-or-later

#include "rrdr_options.h"

static struct {
    const char *name;
    uint32_t hash;
    RRDR_OPTIONS value;
} rrdr_options[] = {
    {  "nonzero"           , 0    , RRDR_OPTION_NONZERO}
    , {"flip"              , 0    , RRDR_OPTION_REVERSED}
    , {"reversed"          , 0    , RRDR_OPTION_REVERSED}
    , {"reverse"           , 0    , RRDR_OPTION_REVERSED}
    , {"jsonwrap"          , 0    , RRDR_OPTION_JSON_WRAP}
    , {"min2max"           , 0    , RRDR_OPTION_DIMS_MIN2MAX}   // rrdr2value() only
    , {"average"           , 0    , RRDR_OPTION_DIMS_AVERAGE}   // rrdr2value() only
    , {"min"               , 0    , RRDR_OPTION_DIMS_MIN}       // rrdr2value() only
    , {"max"               , 0    , RRDR_OPTION_DIMS_MAX}       // rrdr2value() only
    , {"ms"                , 0    , RRDR_OPTION_MILLISECONDS}
    , {"milliseconds"      , 0    , RRDR_OPTION_MILLISECONDS}
    , {"absolute"          , 0    , RRDR_OPTION_ABSOLUTE}
    , {"abs"               , 0    , RRDR_OPTION_ABSOLUTE}
    , {"absolute_sum"      , 0    , RRDR_OPTION_ABSOLUTE}
    , {"absolute-sum"      , 0    , RRDR_OPTION_ABSOLUTE}
    , {"display_absolute"  , 0    , RRDR_OPTION_DISPLAY_ABS}
    , {"display-absolute"  , 0    , RRDR_OPTION_DISPLAY_ABS}
    , {"seconds"           , 0    , RRDR_OPTION_SECONDS}
    , {"null2zero"         , 0    , RRDR_OPTION_NULL2ZERO}
    , {"objectrows"        , 0    , RRDR_OPTION_OBJECTSROWS}
    , {"google_json"       , 0    , RRDR_OPTION_GOOGLE_JSON}
    , {"google-json"       , 0    , RRDR_OPTION_GOOGLE_JSON}
    , {"percentage"        , 0    , RRDR_OPTION_PERCENTAGE}
    , {"unaligned"         , 0    , RRDR_OPTION_NOT_ALIGNED}
    , {"match_ids"         , 0    , RRDR_OPTION_MATCH_IDS}
    , {"match-ids"         , 0    , RRDR_OPTION_MATCH_IDS}
    , {"match_names"       , 0    , RRDR_OPTION_MATCH_NAMES}
    , {"match-names"       , 0    , RRDR_OPTION_MATCH_NAMES}
    , {"anomaly-bit"       , 0    , RRDR_OPTION_ANOMALY_BIT}
    , {"selected-tier"     , 0    , RRDR_OPTION_SELECTED_TIER}
    , {"raw"               , 0    , RRDR_OPTION_RETURN_RAW}
    , {"jw-anomaly-rates"  , 0    , RRDR_OPTION_RETURN_JWAR}
    , {"natural-points"    , 0    , RRDR_OPTION_NATURAL_POINTS}
    , {"virtual-points"    , 0    , RRDR_OPTION_VIRTUAL_POINTS}
    , {"all-dimensions"    , 0    , RRDR_OPTION_ALL_DIMENSIONS}
    , {"details"           , 0    , RRDR_OPTION_SHOW_DETAILS}
    , {"debug"             , 0    , RRDR_OPTION_DEBUG}
    , {"plan"              , 0    , RRDR_OPTION_DEBUG}
    , {"minify"            , 0    , RRDR_OPTION_MINIFY}
    , {"group-by-labels"   , 0    , RRDR_OPTION_GROUP_BY_LABELS}
    , {"label-quotes"      , 0    , RRDR_OPTION_LABEL_QUOTES}
    , {NULL                , 0    , 0}
};

RRDR_OPTIONS rrdr_options_parse_one(const char *o) {
    RRDR_OPTIONS ret = 0;

    if(!o || !*o) return ret;

    uint32_t hash = simple_hash(o);
    int i;
    for(i = 0; rrdr_options[i].name ; i++) {
        if (unlikely(hash == rrdr_options[i].hash && !strcmp(o, rrdr_options[i].name))) {
            ret |= rrdr_options[i].value;
            break;
        }
    }

    return ret;
}

RRDR_OPTIONS rrdr_options_parse(char *o) {
    RRDR_OPTIONS ret = 0;
    char *tok;

    while(o && *o && (tok = strsep_skip_consecutive_separators(&o, ", |"))) {
        if(!*tok) continue;
        ret |= rrdr_options_parse_one(tok);
    }

    return ret;
}

void rrdr_options_to_buffer_json_array(BUFFER *wb, const char *key, RRDR_OPTIONS options) {
    buffer_json_member_add_array(wb, key);

    RRDR_OPTIONS used = 0; // to prevent adding duplicates
    for(int i = 0; rrdr_options[i].name ; i++) {
        if (unlikely((rrdr_options[i].value & options) && !(rrdr_options[i].value & used))) {
            const char *name = rrdr_options[i].name;
            used |= rrdr_options[i].value;

            buffer_json_add_array_item_string(wb, name);
        }
    }

    buffer_json_array_close(wb);
}

void rrdr_options_to_buffer(BUFFER *wb, RRDR_OPTIONS options) {
    RRDR_OPTIONS used = 0; // to prevent adding duplicates
    size_t added = 0;
    for(int i = 0; rrdr_options[i].name ; i++) {
        if (unlikely((rrdr_options[i].value & options) && !(rrdr_options[i].value & used))) {
            const char *name = rrdr_options[i].name;
            used |= rrdr_options[i].value;

            if(added++) buffer_strcat(wb, " ");
            buffer_strcat(wb, name);
        }
    }
}

void web_client_api_request_data_vX_options_to_string(char *buf, size_t size, RRDR_OPTIONS options) {
    char *write = buf;
    char *end = &buf[size - 1];

    RRDR_OPTIONS used = 0; // to prevent adding duplicates
    int added = 0;
    for(int i = 0; rrdr_options[i].name ; i++) {
        if (unlikely((rrdr_options[i].value & options) && !(rrdr_options[i].value & used))) {
            const char *name = rrdr_options[i].name;
            used |= rrdr_options[i].value;

            if(added && write < end)
                *write++ = ',';

            while(*name && write < end)
                *write++ = *name++;

            added++;
        }
    }
    *write = *end = '\0';
}

void rrdr_options_init(void) {
    for(size_t i = 0; rrdr_options[i].name ; i++)
        rrdr_options[i].hash = simple_hash(rrdr_options[i].name);
}
