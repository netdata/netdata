// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_PLUGINSD_PARSER_H
#define NETDATA_PLUGINSD_PARSER_H

#include "../../incremental_parser/incremental_parser.h"


typedef struct parser_user_object {
    INCREMENTAL_PARSER  *parser;

    RRDSET *st;
    RRDHOST *host;
    struct plugind *cd;
    int trust_durations;

    struct label *new_labels;
    size_t count;
    int enabled;
} PARSER_USER_OBJECT;

#endif //NETDATA_PLUGINSD_PARSER_H
