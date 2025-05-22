// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_CONTEXTS_OPTIONS_H
#define NETDATA_CONTEXTS_OPTIONS_H

#include "libnetdata/libnetdata.h"

typedef enum contexts_options {
    CONTEXTS_OPTION_MINIFY                      = (1 << 0), // remove JSON spaces and newlines from JSON output
    CONTEXTS_OPTION_DEBUG                       = (1 << 1), // show the request
    CONTEXTS_OPTION_CONFIGURATIONS              = (1 << 2), // include alert configurations (used by /api/v2/alert_transitions)
    CONTEXTS_OPTION_INSTANCES                   = (1 << 3), // include alert/context instances (used by /api/v2/alerts)
    CONTEXTS_OPTION_VALUES                      = (1 << 4), // include alert latest values (used by /api/v2/alerts)
    CONTEXTS_OPTION_SUMMARY                     = (1 << 5), // include alerts summary counters (used by /api/v2/alerts)
    CONTEXTS_OPTION_MCP                         = (1 << 6), // MCP output format
    CONTEXTS_OPTION_DIMENSIONS                  = (1 << 7), // include context dimensions
    CONTEXTS_OPTION_LABELS                      = (1 << 8), // include context labels
    CONTEXTS_OPTION_PRIORITIES                  = (1 << 9), // include context priorities
    CONTEXTS_OPTION_TITLES                      = (1 << 10), // include context titles
} CONTEXTS_OPTIONS;

CONTEXTS_OPTIONS contexts_options_str_to_id(char *o);
void contexts_options_to_buffer_json_array(BUFFER *wb, const char *key, CONTEXTS_OPTIONS options);

void contexts_options_init(void);

#endif //NETDATA_CONTEXTS_OPTIONS_H
