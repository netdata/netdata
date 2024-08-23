// SPDX-License-Identifier: GPL-3.0-or-later

#include "datasource_formats.h"

static struct {
    const char *name;
    uint32_t hash;
    DATASOURCE_FORMAT value;
} google_data_formats[] = {
    // this is not an error - when Google requests json, it expects javascript
    // https://developers.google.com/chart/interactive/docs/dev/implementing_data_source#responseformat
    {"json",      0, DATASOURCE_DATATABLE_JSONP}
    , {"html",      0, DATASOURCE_HTML}
    , {"csv",       0, DATASOURCE_CSV}
    , {"tsv-excel", 0, DATASOURCE_TSV}

    // terminator
    , {NULL,        0, 0}
};

inline DATASOURCE_FORMAT google_data_format_str_to_id(char *name) {
    uint32_t hash = simple_hash(name);
    int i;

    for(i = 0; google_data_formats[i].name ; i++) {
        if (unlikely(hash == google_data_formats[i].hash && !strcmp(name, google_data_formats[i].name))) {
            return google_data_formats[i].value;
        }
    }

    return DATASOURCE_JSON;
}

// --------------------------------------------------------------------------------------------------------------------

static struct {
    const char *name;
    uint32_t hash;
    DATASOURCE_FORMAT value;
} datasource_formats[] = {
    {  "datatable"     , 0 , DATASOURCE_DATATABLE_JSON}
    , {"datasource"    , 0 , DATASOURCE_DATATABLE_JSONP}
    , {"json"          , 0 , DATASOURCE_JSON}
    , {"json2"         , 0 , DATASOURCE_JSON2}
    , {"jsonp"         , 0 , DATASOURCE_JSONP}
    , {"ssv"           , 0 , DATASOURCE_SSV}
    , {"csv"           , 0 , DATASOURCE_CSV}
    , {"tsv"           , 0 , DATASOURCE_TSV}
    , {"tsv-excel"     , 0 , DATASOURCE_TSV}
    , {"html"          , 0 , DATASOURCE_HTML}
    , {"array"        , 0 , DATASOURCE_JS_ARRAY}
    , {"ssvcomma"     , 0 , DATASOURCE_SSV_COMMA}
    , {"csvjsonarray" , 0 , DATASOURCE_CSV_JSON_ARRAY}
    , {"markdown"     , 0 , DATASOURCE_CSV_MARKDOWN}

    // terminator
    , {NULL, 0, 0}
};

DATASOURCE_FORMAT datasource_format_str_to_id(char *name) {
    uint32_t hash = simple_hash(name);
    int i;

    for(i = 0; datasource_formats[i].name ; i++) {
        if (unlikely(hash == datasource_formats[i].hash && !strcmp(name, datasource_formats[i].name))) {
            return datasource_formats[i].value;
        }
    }

    return DATASOURCE_JSON;
}

const char *rrdr_format_to_string(DATASOURCE_FORMAT format) {
    for(size_t i = 0; datasource_formats[i].name ;i++)
        if(unlikely(datasource_formats[i].value == format))
            return datasource_formats[i].name;

    return "unknown";
}

// --------------------------------------------------------------------------------------------------------------------

void datasource_formats_init(void) {
    for(size_t i = 0; datasource_formats[i].name ; i++)
        datasource_formats[i].hash = simple_hash(datasource_formats[i].name);

    for(size_t i = 0; google_data_formats[i].name ; i++)
        google_data_formats[i].hash = simple_hash(google_data_formats[i].name);
}
