// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_PLUGINSD_PARSER_H
#define NETDATA_PLUGINSD_PARSER_H

#include "parser/parser.h"

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
    void *private; // the user can set this for private use

    struct {
        time_t start_time;
        time_t end_time;

        usec_t start_time_ut;
        usec_t end_time_ut;
    } replay;
} PARSER_USER_OBJECT;

PARSER_RC pluginsd_function(char **words, size_t num_words, void *user);
PARSER_RC pluginsd_function_result_begin(char **words, size_t num_words, void *user);
void inflight_functions_init(PARSER *parser);
#endif //NETDATA_PLUGINSD_PARSER_H
