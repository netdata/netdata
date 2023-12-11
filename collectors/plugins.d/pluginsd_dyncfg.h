// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_PLUGINSD_DYNCFG_H
#define NETDATA_PLUGINSD_DYNCFG_H

#include "pluginsd_internals.h"

PARSER_RC pluginsd_register_plugin(char **words, size_t num_words, PARSER *parser);
PARSER_RC pluginsd_register_module(char **words, size_t num_words, PARSER *parser);
PARSER_RC pluginsd_register_job(char **words, size_t num_words, PARSER *parser);
PARSER_RC pluginsd_dyncfg_reset(char **words, size_t num_words, PARSER *parser);
PARSER_RC pluginsd_job_status(char **words, size_t num_words, PARSER *parser);
PARSER_RC pluginsd_delete_job(char **words, size_t num_words, PARSER *parser);

void pluginsd_dyncfg_cleanup(PARSER *parser);

#endif //NETDATA_PLUGINSD_DYNCFG_H
