// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef LIBNETDATA_PARSER_SPLITTER_H
#define LIBNETDATA_PARSER_SPLITTER_H

#include "../libnetdata.h"

static inline int is_space_config(char c)
{
    switch (c) {
        case ' ': case '\t': case '\r': case '\n': case ',':
            return 1;
        default:
            return 0;
    }
}

inline int is_space_pluginsd(char c) {
    switch(c) {
        case ' ': case '\t': case '\r': case '\n': case '=':
            return 1;
        default:
            return 0;
    }
}

inline int is_space_rrd2json_label(char c) {
    switch (c) {
        case ',': case '|':
            return 1;
        default:
            return 0;
    }
}

// Define the proper macros to generate the three different functions that
// we need for splitting lines of words for config, pluginsd and rrd2json_label

#define CUSTOM_IS_SPACE_FUNC is_space_config
#define SPLITTER_FUNC        split_quoted_words_by_is_space_config
#include "splitter.inc.h"

#define CUSTOM_IS_SPACE_FUNC is_space_pluginsd
#define SPLITTER_FUNC        split_quoted_words_by_is_space_pluginsd
#include "splitter.inc.h"

#define CUSTOM_IS_SPACE_FUNC is_space_rrd2json_label
#define SPLITTER_FUNC        split_quoted_words_by_is_space_rrd2json_label
#include "splitter.inc.h"

#endif /* LIBNETDATA_PARSER_SPLITTER_H */
