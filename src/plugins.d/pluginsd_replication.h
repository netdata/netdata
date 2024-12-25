// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_PLUGINSD_REPLICATION_H
#define NETDATA_PLUGINSD_REPLICATION_H

#include "pluginsd_internals.h"

PARSER_RC pluginsd_replay_begin(char **words, size_t num_words, PARSER *parser);
PARSER_RC pluginsd_replay_set(char **words, size_t num_words, PARSER *parser);
PARSER_RC pluginsd_replay_rrddim_collection_state(char **words, size_t num_words, PARSER *parser);
PARSER_RC pluginsd_replay_rrdset_collection_state(char **words, size_t num_words, PARSER *parser);
PARSER_RC pluginsd_replay_end(char **words, size_t num_words, PARSER *parser);
PARSER_RC pluginsd_chart_definition_end(char **words, size_t num_words, PARSER *parser);

#endif //NETDATA_PLUGINSD_REPLICATION_H
