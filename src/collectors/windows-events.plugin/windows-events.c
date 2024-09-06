// SPDX-License-Identifier: GPL-3.0-or-later

#include "libnetdata/libnetdata.h"
#include "libnetdata/required_dummies.h"

#include "windows-events.h"

typedef enum {
    WEVTS_NONE               = 0,
    WEVTS_ALL                = (1 << 0),
} WEVT_SOURCE_TYPE;

#define WEVT_FUNCTION_DESCRIPTION    "View, search and analyze Microsoft Windows events."
#define WEVT_FUNCTION_NAME           "windows-events"

// functions needed by LQS
static WEVT_SOURCE_TYPE wevt_internal_source_type(const char *value);

// structures needed by LQS
struct lqs_extension {};

// prepare LQS
#define LQS_FUNCTION_NAME           WEVT_FUNCTION_NAME
#define LQS_FUNCTION_DESCRIPTION    WEVT_FUNCTION_DESCRIPTION
#define LQS_DEFAULT_ITEMS_PER_QUERY 200
#define LQS_DEFAULT_ITEMS_SAMPLING  1000000
#define LQS_SOURCE_TYPE             WEVT_SOURCE_TYPE
#define LQS_SOURCE_TYPE_ALL         WEVTS_ALL
#define LQS_SOURCE_TYPE_NONE        WEVTS_NONE
#define LQS_FUNCTION_GET_INTERNAL_SOURCE_TYPE(value) wevt_internal_source_type(value)
#define LQS_FUNCTION_SOURCE_TO_JSON_ARRAY(wb) wevt_sources_to_json_array(wb)
#include "libnetdata/facets/logs_query_status.h"

static WEVT_SOURCE_TYPE wevt_internal_source_type(const char *value) {
    if(strcmp(value, "all") == 0)
        return WEVTS_ALL;

    return WEVTS_NONE;
}

int main(void) {
    CLEAN_BUFFER *wb = buffer_create(0, NULL);
    buffer_json_initialize(wb, "\"", "\"", 0, true, BUFFER_JSON_OPTIONS_DEFAULT);
    buffer_json_member_add_array(wb, "sources");
    {
        wevt_sources_to_json_array(wb);
    }
    buffer_json_array_close(wb); // sources
    buffer_json_finalize(wb);

    printf("%s\n", buffer_tostring(wb));
    return 0;
}
