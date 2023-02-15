// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_PLUGINSD_PARSER_H
#define NETDATA_PLUGINSD_PARSER_H

#include "daemon/common.h"

typedef enum __attribute__ ((__packed__)) {
    PARSER_INIT_PLUGINSD        = (1 << 1),
    PARSER_INIT_STREAMING       = (1 << 2),
} PLUGINSD_KEYWORDS;

typedef struct parser_user_object {
    PARSER  *parser;
    RRDSET *st;
    RRDHOST *host;
    void    *opaque;
    struct plugind *cd;
    int trust_durations;
    DICTIONARY *new_host_labels;
    DICTIONARY *chart_rrdlabels_linked_temporarily;
    size_t data_collections_count;
    int enabled;

    STREAM_CAPABILITIES capabilities; // receiver capabilities

    struct {
        bool parsing_host;
        uuid_t machine_guid;
        char machine_guid_str[UUID_STR_LEN];
        STRING *hostname;
        DICTIONARY *rrdlabels;
    } host_define;

    struct parser_user_object_replay {
        time_t start_time;
        time_t end_time;

        usec_t start_time_ut;
        usec_t end_time_ut;

        time_t wall_clock_time;

        bool rset_enabled;
    } replay;

    struct parser_user_object_v2 {
        bool locked_data_collection;
        RRDSET_STREAM_BUFFER stream_buffer; // sender capabilities in this
        time_t update_every;
        time_t end_time;
        time_t wall_clock_time;
        bool ml_locked;
    } v2;
} PARSER_USER_OBJECT;

PARSER_RC pluginsd_function(char **words, size_t num_words, void *user);
PARSER_RC pluginsd_function_result_begin(char **words, size_t num_words, void *user);
void inflight_functions_init(PARSER *parser);
void pluginsd_keywords_init(PARSER *parser, PLUGINSD_KEYWORDS types);

#endif //NETDATA_PLUGINSD_PARSER_H
