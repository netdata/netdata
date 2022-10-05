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
} PARSER_USER_OBJECT;

extern PARSER_RC pluginsd_variable_action(void *user, RRDHOST *host, RRDSET *st, char *name, int global, NETDATA_DOUBLE value);
extern PARSER_RC pluginsd_dimension_action(void *user, RRDSET *st, char *id, char *name, char *algorithm,
                                           long multiplier, long divisor, char *options, RRD_ALGORITHM algorithm_type);
extern PARSER_RC pluginsd_label_action(void *user, char *key, char *value, RRDLABEL_SRC source);
extern PARSER_RC pluginsd_overwrite_action(void *user, RRDHOST *host, DICTIONARY *new_host_labels);

extern PARSER_RC pluginsd_function(char **words, void *user, PLUGINSD_ACTION  *plugins_action);
extern PARSER_RC pluginsd_function_result_begin(char **words, void *user, PLUGINSD_ACTION  *plugins_action);
extern void inflight_functions_init(PARSER *parser);

#endif //NETDATA_PLUGINSD_PARSER_H
