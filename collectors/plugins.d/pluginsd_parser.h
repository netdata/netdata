// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_PLUGINSD_PARSER_H
#define NETDATA_PLUGINSD_PARSER_H

#include "../../parser/parser.h"


typedef struct parser_user_object {
    PARSER  *parser;
    RRDSET *st;
    RRDHOST *host;
    void    *opaque;
    struct plugind *cd;
    int trust_durations;
    struct label *new_labels;
    size_t count;
    int enabled;
} PARSER_USER_OBJECT;

PARSER_RC pluginsd_set_action(void *user, RRDSET *st, RRDDIM *rd, long long int value);
PARSER_RC pluginsd_flush_action(void *user, RRDSET *st);
PARSER_RC pluginsd_begin_action(void *user, RRDSET *st, usec_t microseconds, int trust_durations);
PARSER_RC pluginsd_end_action(void *user, RRDSET *st);
PARSER_RC pluginsd_chart_action(void *user, char *type, char *id, char *name, char *family, char *context, char *title, char *units, char *plugin,
           char *module, int priority, int update_every, RRDSET_TYPE chart_type, char *options);
PARSER_RC pluginsd_disable_action(void *user);
PARSER_RC pluginsd_variable_action(void *user, RRDHOST *host, RRDSET *st, char *name, int global, calculated_number value);
PARSER_RC pluginsd_dimension_action(void *user, RRDSET *st, char *id, char *name, char *algorithm, long multiplier, long divisor, char *options,
                                    RRD_ALGORITHM algorithm_type);
PARSER_RC pluginsd_label_action(void *user, char *key, char *value, LABEL_SOURCE source);
PARSER_RC pluginsd_overwrite_action(void *user, RRDHOST *host, struct label *new_labels);

#endif //NETDATA_PLUGINSD_PARSER_H
