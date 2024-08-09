// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_DATASOURCE_FORMATS_H
#define NETDATA_DATASOURCE_FORMATS_H

#include "libnetdata/libnetdata.h"

// type of JSON generations
typedef enum {
    DATASOURCE_JSON = 0,
    DATASOURCE_DATATABLE_JSON,
    DATASOURCE_DATATABLE_JSONP,
    DATASOURCE_SSV,
    DATASOURCE_CSV,
    DATASOURCE_JSONP,
    DATASOURCE_TSV,
    DATASOURCE_HTML,
    DATASOURCE_JS_ARRAY,
    DATASOURCE_SSV_COMMA,
    DATASOURCE_CSV_JSON_ARRAY,
    DATASOURCE_CSV_MARKDOWN,
    DATASOURCE_JSON2,
} DATASOURCE_FORMAT;

DATASOURCE_FORMAT datasource_format_str_to_id(char *name);
const char *rrdr_format_to_string(DATASOURCE_FORMAT format);

DATASOURCE_FORMAT google_data_format_str_to_id(char *name);

void datasource_formats_init(void);

#endif //NETDATA_DATASOURCE_FORMATS_H
