// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_PLUGINSD_PARSER_H
#define NETDATA_PLUGINSD_PARSER_H

#include "parser/parser.h"

typedef struct replication_object {
    // From REPLAY_RRDSET_BEGIN
    RRDHOST *host;
    RRDSET *st;

    // From REPLAY_RRDSET_HEADER
    RRDDIM *replay_dimensions[PLUGINSD_MAX_WORDS];

    // This flag is initialized to true when we replay a new chart.
    // It's set to false if/when an error occurs, so that we can proceed
    // by asking the child to continue by enabling streaming
    bool ok;
} REPLICATION_OBJECT;

typedef struct parser_user_object {
    PARSER  *parser;
    RRDSET *st;
    RRDHOST *host;
    void    *opaque;
    struct plugind *cd;
    int trust_durations;
    DICTIONARY *new_host_labels;
    DICTIONARY *chart_rrdlabels_linked_temporarily;
    size_t count;
    int enabled;
    uint8_t st_exists;
    uint8_t host_exists;
    struct replication_object repl_object;
    void *private; // the user can set this for private use
} PARSER_USER_OBJECT;

PARSER_RC pluginsd_function(char **words, size_t num_words, void *user, PLUGINSD_ACTION  *plugins_action);
PARSER_RC pluginsd_function_result_begin(char **words, size_t num_words, void *user, PLUGINSD_ACTION  *plugins_action);
void inflight_functions_init(PARSER *parser);

#endif //NETDATA_PLUGINSD_PARSER_H
