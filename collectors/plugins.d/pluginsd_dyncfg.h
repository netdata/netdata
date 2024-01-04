// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_PLUGINSD_DYNCFG_H
#define NETDATA_PLUGINSD_DYNCFG_H

#include "pluginsd_internals.h"

PARSER_RC pluginsd_config(char **words, size_t num_words, PARSER *parser);
PARSER_RC pluginsd_dyncfg_noop(char **words, size_t num_words, PARSER *parser);

#endif //NETDATA_PLUGINSD_DYNCFG_H
