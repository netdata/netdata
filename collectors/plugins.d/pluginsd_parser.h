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
    struct label *new_labels;
    struct label *chart_labels;
    size_t count;
    int enabled;
    uint8_t st_exists;
    uint8_t host_exists;
    void *private; // the user can set this for private use
} PARSER_USER_OBJECT;

extern PARSER_RC pluginsd_set_action(void *user, RRDSET *st, RRDDIM *rd, long long int value);
extern PARSER_RC pluginsd_flush_action(void *user, RRDSET *st);
extern PARSER_RC pluginsd_begin_action(void *user, RRDSET *st, usec_t microseconds, int trust_durations);
extern PARSER_RC pluginsd_end_action(void *user, RRDSET *st);
extern PARSER_RC pluginsd_chart_action(void *user, char *type, char *id, char *name, char *family, char *context,
                                       char *title, char *units, char *plugin, char *module, int priority,
                                       int update_every, RRDSET_TYPE chart_type, char *options);
extern PARSER_RC pluginsd_disable_action(void *user);
extern PARSER_RC pluginsd_variable_action(void *user, RRDHOST *host, RRDSET *st, char *name, int global,
                                          calculated_number value);
extern PARSER_RC pluginsd_dimension_action(void *user, RRDSET *st, char *id, char *name, char *algorithm,
                                           long multiplier, long divisor, char *options, RRD_ALGORITHM algorithm_type);
extern PARSER_RC pluginsd_label_action(void *user, char *key, char *value, LABEL_SOURCE source);
extern PARSER_RC pluginsd_overwrite_action(void *user, RRDHOST *host, struct label *new_labels);
extern PARSER_RC pluginsd_clabel_commit_action(void *user, RRDHOST *host, struct label *new_labels);
extern PARSER_RC pluginsd_clabel_action(void *user, char *key, char *value, LABEL_SOURCE source);


#endif //NETDATA_PLUGINSD_PARSER_H
