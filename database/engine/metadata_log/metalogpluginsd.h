// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_METALOGPLUGINSD_H
#define NETDATA_METALOGPLUGINSD_H

#include "collectors/plugins.d/pluginsd_parser.h"
#include "collectors/plugins.d/plugins_d.h"
#include "parser/parser.h"

struct metalog_pluginsd_state {
    struct metalog_instance *ctx;
    uuid_t uuid;
    uuid_t host_uuid;
    uuid_t chart_uuid;
    uint8_t skip_record; /* skip this record due to errors in parsing */
    struct metadata_logfile *metalogfile; /* current metadata log file being replayed */
};

void metalog_pluginsd_state_init(struct metalog_pluginsd_state *state, struct metalog_instance *ctx);
PARSER_RC metalog_pluginsd_dimension_action(void *user, RRDSET *st, char *id, char *name, char *algorithm,
                                            long multiplier, long divisor, char *options,
                                            RRD_ALGORITHM algorithm_type);
PARSER_RC metalog_pluginsd_guid_action(void *user, uuid_t *uuid);
PARSER_RC metalog_pluginsd_context_action(void *user, uuid_t *uuid);
PARSER_RC metalog_pluginsd_tombstone_action(void *user, uuid_t *uuid);
PARSER_RC metalog_pluginsd_host(char **words, void *user, PLUGINSD_ACTION  *plugins_action);
PARSER_RC metalog_pluginsd_host_action(void *user, char *machine_guid, char *hostname, char *registry_hostname, int update_every, char *os, char *timezone, char *tags);

#endif /* NETDATA_METALOGPLUGINSD_H */
